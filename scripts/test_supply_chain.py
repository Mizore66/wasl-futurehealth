import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "scripts" / "supply_chain.py"
SPEC = importlib.util.spec_from_file_location("supply_chain", MODULE_PATH)
supply_chain = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(supply_chain)


class AugmentSbomTests(unittest.TestCase):
    def setUp(self):
        self.raw = {
            "$schema": "http://cyclonedx.org/schema/bom-1.6.schema.json",
            "bomFormat": "CycloneDX",
            "specVersion": "1.6",
            "serialNumber": "urn:uuid:random",
            "version": 1,
            "metadata": {
                "timestamp": "2026-11-09T10:00:00Z",
                "component": {"bom-ref": "root", "name": "checkout", "type": "application"},
            },
            "components": [],
        }
        npm = supply_chain._npm_integrity(ROOT / "web" / "pnpm-lock.yaml")
        go = supply_chain._go_module_hashes(ROOT / "pipeline" / "go.sum")
        for (name, version) in sorted(npm):
            purl_name = "%40" + name[1:] if name.startswith("@") else name
            self.raw["components"].append({
                "bom-ref": "npm-minimatch" if name == "minimatch" else f"pkg:npm/{purl_name}@{version}",
                "type": "library", "name": name.rsplit("/", 1)[-1], "version": version,
                "purl": f"pkg:npm/{purl_name}@{version}",
            })
        for (name, version) in sorted(go):
            self.raw["components"].append({
                "bom-ref": "go-crypto" if name == "golang.org/x/crypto" else f"pkg:golang/{name}@{version}",
                "type": "library", "name": name, "version": version,
                "purl": f"pkg:golang/{name}@{version}",
            })

    def augment(self):
        return supply_chain.augment_sbom(
            copy.deepcopy(self.raw),
            ROOT,
            revision="a" * 40,
            committed_at="2026-11-09T01:02:03Z",
            dirty=False,
        )

    def test_binds_source_provenance_and_replaces_volatile_identity(self):
        bom = self.augment()
        self.assertEqual(bom["metadata"]["timestamp"], "2026-11-09T01:02:03Z")
        self.assertEqual(bom["metadata"]["component"]["version"], "a" * 40)
        properties = {p["name"]: p["value"] for p in bom["metadata"]["properties"]}
        self.assertEqual(properties["wasl:source-revision"], "a" * 40)
        self.assertEqual(properties["wasl:source-tree"], "clean")
        self.assertEqual(properties["wasl:inventory-scope"], "source; not a built container SBOM")
        self.assertEqual(bom, self.augment())

    def test_adds_npm_artifact_hashes_and_go_module_checksums(self):
        components = {c["bom-ref"]: c for c in self.augment()["components"]}
        npm_hash = components["npm-minimatch"]["hashes"][0]
        self.assertEqual(npm_hash["alg"], "SHA-512")
        self.assertRegex(npm_hash["content"], r"^[0-9a-f]{128}$")
        go_properties = components["go-crypto"].get("properties", [])
        self.assertTrue(any(p["name"] == "wasl:go-sum-module-hash" for p in go_properties))

    def test_includes_docker_python_and_all_base_images(self):
        bom = self.augment()
        libraries = {(c["name"], c["version"]) for c in bom["components"] if c["type"] == "library"}
        self.assertIn(("numpy", "2.3.5"), libraries)
        self.assertIn(("scipy", "1.16.3"), libraries)
        self.assertNotIn("pip", {name for name, _ in libraries})
        containers = {c["name"]: c for c in bom["components"] if c["type"] == "container"}
        self.assertEqual(
            set(containers),
            {"golang", "python", "nats"},
        )
        self.assertEqual(containers["nats"]["version"], "2.12.6-alpine")
        self.assertIn("pkg:docker/", containers["python"]["purl"])
        self.assertEqual(containers["python"]["version"], "3.12-alpine")

    def test_normalizes_scanner_root_uuids_throughout_dependency_graph(self):
        raw = copy.deepcopy(self.raw)
        raw["metadata"]["component"]["bom-ref"] = "8ce651a0-8004-4192-9437-724f77b4146b"
        raw["components"].append({
            "bom-ref": "170fa53f-e2f2-4abc-82cb-3c00ada272cd",
            "type": "application", "name": "web/pnpm-lock.yaml",
        })
        raw["dependencies"] = [
            {"ref": "8ce651a0-8004-4192-9437-724f77b4146b", "dependsOn": ["170fa53f-e2f2-4abc-82cb-3c00ada272cd"]},
            {"ref": "170fa53f-e2f2-4abc-82cb-3c00ada272cd", "dependsOn": ["npm-minimatch"]},
        ]
        bom = supply_chain.augment_sbom(
            raw, ROOT, revision="a" * 40,
            committed_at="2026-11-09T01:02:03Z", dirty=False,
        )
        self.assertEqual(bom["metadata"]["component"]["bom-ref"], "wasl:source-root")
        self.assertEqual(next(c for c in bom["components"] if c.get("name") == "web/pnpm-lock.yaml")["bom-ref"], "wasl:manifest:web/pnpm-lock.yaml")
        dependencies = {d["ref"]: d for d in bom["dependencies"]}
        self.assertEqual(dependencies["wasl:source-root"]["dependsOn"], ["wasl:manifest:web/pnpm-lock.yaml"])
        self.assertEqual(dependencies["wasl:manifest:web/pnpm-lock.yaml"]["dependsOn"], ["npm-minimatch"])
        self.assertEqual([c["bom-ref"] for c in bom["components"]], sorted(c["bom-ref"] for c in bom["components"]))
        self.assertEqual([d["ref"] for d in bom["dependencies"]], sorted(d["ref"] for d in bom["dependencies"]))
        for dependency in bom["dependencies"]:
            self.assertEqual(dependency["dependsOn"], sorted(dependency["dependsOn"]))

    def test_refuses_a_dirty_tree_as_clean_release_evidence(self):
        with self.assertRaisesRegex(ValueError, "dirty"):
            supply_chain.augment_sbom(
                copy.deepcopy(self.raw), ROOT, revision="a" * 40,
                committed_at="2026-11-09T01:02:03Z", dirty=True,
            )


class ContainerSecurityTests(unittest.TestCase):
    def test_nats_binary_is_rebuilt_with_patched_toolchain_and_modules(self):
        dockerfile = (ROOT / "deploy" / "nats" / "Dockerfile").read_text()
        self.assertIn("ARG GO_VERSION=1.27.3", dockerfile)
        self.assertIn("FROM --platform=linux/${TARGETARCH} nats:${NATS_VERSION}-alpine", dockerfile)
        self.assertIn("ARG NATS_VERSION=2.12.6", dockerfile)
        self.assertIn("golang.org/x/crypto@v0.58.1-0.20261009181718-2f1f834330b2", dockerfile)
        self.assertIn("golang.org/x/sys@v0.49.0", dockerfile)
        self.assertIn("github.com/klauspost/compress@v1.20.0", dockerfile)
        self.assertIn("COPY --from=build /out/nats-server /usr/local/bin/nats-server", dockerfile)
        self.assertIn("apk upgrade --no-cache", dockerfile)

    def test_python_runtime_removes_unused_bootstrap_toolchain(self):
        dockerfile = (ROOT / "Dockerfile").read_text()
        self.assertIn("ARG GO_VERSION=1.27.3", dockerfile)
        self.assertIn("FROM python:${PYTHON_VERSION}-alpine", dockerfile)
        self.assertIn("apk upgrade --no-cache", dockerfile)
        self.assertIn("apk add --no-cache bash util-linux", dockerfile)
        self.assertIn('"pip==26.3"', dockerfile)
        self.assertIn("pip uninstall -y pip", dockerfile)
        self.assertIn("rm -rf /usr/local/lib/python3.12/ensurepip", dockerfile)


class VulnerabilityReportTests(unittest.TestCase):
    def raw_report(self):
        return {
            "SchemaVersion": 2,
            "ReportID": "volatile",
            "CreatedAt": "2026-11-09T10:00:00Z",
            "Results": [
                {
                    "Target": "web/pnpm-lock.yaml",
                    "Class": "lang-pkgs",
                    "Type": "pnpm",
                    "Vulnerabilities": [{
                        "VulnerabilityID": "CVE-2026-118764",
                        "PkgName": "minimatch",
                        "InstalledVersion": "10.2.6",
                        "FixedVersion": "11.0.1",
                        "Status": "fixed",
                        "Severity": "MEDIUM",
                        "Title": "example",
                        "PrimaryURL": "https://example.invalid/advisory",
                    }],
                },
                {
                    "Target": "pipeline/go.mod",
                    "Class": "lang-pkgs",
                    "Type": "gomod",
                    "Vulnerabilities": [{
                        "VulnerabilityID": "GO-2026-5932",
                        "PkgName": "golang.org/x/crypto",
                        "InstalledVersion": "v0.57.0",
                        "FixedVersion": "0.58.1-0.20261009181718-2f1f834330b2",
                        "Severity": "UNKNOWN",
                    }],
                },
            ],
        }

    def normalize(self, raw=None):
        return supply_chain.normalize_vulnerability_report(
            self.raw_report() if raw is None else raw,
            revision="b" * 40,
            committed_at="2026-11-09T01:02:03Z",
            scanner={"name": "trivy", "version": "0.76.2"},
        )

    def test_preserves_medium_and_unknown_findings(self):
        report = self.normalize()
        self.assertEqual(report["summary"]["total"], 2)
        self.assertEqual(report["summary"]["by_severity"]["MEDIUM"], 1)
        self.assertEqual(report["summary"]["by_severity"]["UNKNOWN"], 1)
        self.assertEqual(report["policy"]["fail_on"], "any reported vulnerability")
        self.assertTrue(report["policy"]["include_development_dependencies"])
        self.assertNotIn("ReportID", report)
        self.assertEqual(report["findings"][0]["target"], "pipeline/go.mod")

    def test_any_severity_fails_the_gate(self):
        for index in range(2):
            raw = self.raw_report()
            raw["Results"] = [raw["Results"][index]]
            self.assertEqual(supply_chain.vulnerability_exit_code(self.normalize(raw)), 1)
        self.assertEqual(supply_chain.vulnerability_exit_code(self.normalize({"Results": []})), 0)

    def test_rejects_malformed_results_instead_of_reporting_clean(self):
        with self.assertRaisesRegex(ValueError, "Results"):
            self.normalize({"Results": {}})


class LicenseReportTests(unittest.TestCase):
    def test_reports_discovered_and_missing_licenses(self):
        raw = {"Results": [{
            "Target": "web/pnpm-lock.yaml",
            "Packages": [
                {"Name": "root", "Relationship": "root"},
                {"Name": "zod", "Version": "4.6.7", "Identifier": {"PURL": "pkg:npm/zod@4.6.7"}, "Licenses": ["MIT"]},
                {"Name": "mystery", "Version": "1.0.0", "Identifier": {"PURL": "pkg:npm/mystery@1.0.0"}},
            ],
        }]}
        report = supply_chain.normalize_license_report(
            raw, revision="b" * 40, committed_at="2026-11-09T01:02:03Z",
            scanner={"name": "trivy", "version": "0.76.2"},
        )
        self.assertEqual(report["summary"]["packages"], 2)
        self.assertEqual(report["summary"]["with_discovered_license"], 1)
        self.assertEqual(report["summary"]["without_discovered_license"], 1)
        self.assertEqual(next(p for p in report["packages"] if p["name"] == "mystery")["licenses"], [])
        self.assertIn("not legal clearance", " ".join(report["limitations"]))


class EvidenceComparisonTests(unittest.TestCase):
    def setUp(self):
        self.tracked = {
            "repository.cdx.json": {
                "metadata": {"properties": [
                    {"name": "wasl:license-discovery-system", "value": "darwin"},
                    {"name": "wasl:license-discovery-machine", "value": "arm64"},
                ]},
                "components": [{"name": "zod", "version": "4.6.7", "licenses": [{"license": {"id": "MIT"}}]}],
            },
            "vulnerabilities.json": {"summary": {"total": 0}, "findings": []},
            "licenses.json": {
                "source": {"revision": "a" * 40}, "scanner": {"name": "trivy", "version": "0.76.2"},
                "scan_environment": {"system": "darwin", "machine": "arm64"},
                "packages": [{"target": "web/pnpm-lock.yaml", "name": "zod", "version": "4.6.7", "purl": "pkg:npm/zod@4.6.7", "development": False, "licenses": ["MIT"]}],
                "limitations": [],
            },
        }

    def test_cross_platform_optional_license_changes_are_comparable(self):
        generated = copy.deepcopy(self.tracked)
        generated["repository.cdx.json"]["metadata"]["properties"][0]["value"] = "linux"
        generated["repository.cdx.json"]["metadata"]["properties"][1]["value"] = "x86_64"
        generated["repository.cdx.json"]["components"][0].pop("licenses")
        generated["licenses.json"]["scan_environment"] = {"system": "linux", "machine": "x86_64"}
        generated["licenses.json"]["packages"][0]["licenses"] = []
        supply_chain.compare_evidence(generated, self.tracked)

    def test_same_platform_license_drift_is_rejected(self):
        generated = copy.deepcopy(self.tracked)
        generated["licenses.json"]["packages"][0]["licenses"] = []
        with self.assertRaisesRegex(ValueError, "licenses.json"):
            supply_chain.compare_evidence(generated, self.tracked)

    def test_cross_platform_package_drift_is_rejected(self):
        generated = copy.deepcopy(self.tracked)
        generated["repository.cdx.json"]["metadata"]["properties"][0]["value"] = "linux"
        generated["licenses.json"]["scan_environment"] = {"system": "linux", "machine": "arm64"}
        generated["licenses.json"]["packages"][0]["version"] = "4.6.8"
        with self.assertRaisesRegex(ValueError, "package identity"):
            supply_chain.compare_evidence(generated, self.tracked)

    def test_vulnerability_drift_is_always_rejected(self):
        generated = copy.deepcopy(self.tracked)
        generated["vulnerabilities.json"]["summary"]["total"] = 1
        with self.assertRaisesRegex(ValueError, "vulnerabilities.json"):
            supply_chain.compare_evidence(generated, self.tracked)


class ImageSbomTests(unittest.TestCase):
    def test_binds_image_reference_and_source_without_source_inventory_claim(self):
        raw = {
            "bomFormat": "CycloneDX", "specVersion": "1.6", "version": 1,
            "serialNumber": "urn:uuid:volatile",
            "metadata": {"timestamp": "volatile", "component": {
                "bom-ref": "image-digest-ref", "name": "old", "type": "container",
                "purl": "pkg:oci/wasl@sha256%3A0123456789abcdef",
            }},
            "components": [{"type": "library", "name": "musl", "version": "1.2.5"}],
        }
        bom = supply_chain.bind_image_sbom(
            raw, image="wasl:test", revision="c" * 40,
            committed_at="2026-11-09T01:02:03Z",
        )
        self.assertEqual(bom["metadata"]["component"]["name"], "wasl:test")
        self.assertEqual(bom["metadata"]["component"]["bom-ref"], "image-digest-ref")
        self.assertIn("sha256", bom["metadata"]["component"]["purl"])
        self.assertEqual(bom["metadata"]["timestamp"], "2026-11-09T01:02:03Z")
        properties = {p["name"]: p["value"] for p in bom["metadata"]["properties"]}
        self.assertEqual(properties["wasl:inventory-scope"], "built container image")
        self.assertEqual(properties["wasl:source-revision"], "c" * 40)
        self.assertEqual(bom["components"], raw["components"])


if __name__ == "__main__":
    unittest.main()
