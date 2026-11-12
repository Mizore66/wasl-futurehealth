#!/usr/bin/env python3
"""Build and verify Wasl supply-chain evidence using only the Python stdlib."""

from __future__ import annotations

import copy
import argparse
import base64
import datetime
import hashlib
import json
import platform
from pathlib import Path
import re
import subprocess
import sys
import urllib.parse
import uuid


def _npm_integrity(lockfile: Path) -> dict[tuple[str, str], str]:
    """Read package tarball integrity values from a pnpm v9 lockfile."""
    result: dict[tuple[str, str], str] = {}
    in_packages = False
    current: tuple[str, str] | None = None
    for line in lockfile.read_text(encoding="utf-8").splitlines():
        if line == "packages:":
            in_packages = True
            continue
        if in_packages and line and not line.startswith(" "):
            break
        if not in_packages:
            continue
        package = re.match(r"^  (\S.*?):\s*$", line)
        if package:
            key = package.group(1)
            if len(key) >= 2 and key[0] == key[-1] and key[0] in "'\"":
                key = key[1:-1]
            name, separator, version = key.rpartition("@")
            if not separator or not name or not version:
                raise ValueError(f"Unsupported pnpm package key: {key}")
            current = (name, version)
            continue
        integrity = re.match(r"^    resolution: \{integrity: sha512-([^,}]+)", line)
        if integrity and current:
            result[current] = integrity.group(1)
    if not result:
        raise ValueError(f"No pnpm package integrity values found in {lockfile}")
    return result


def _go_module_hashes(go_sum: Path) -> dict[tuple[str, str], str]:
    result: dict[tuple[str, str], str] = {}
    for line in go_sum.read_text(encoding="utf-8").splitlines():
        fields = line.split()
        if len(fields) != 3:
            continue
        name, version, module_hash = fields
        if not version.endswith("/go.mod"):
            result[(name, version)] = module_hash
    if not result:
        raise ValueError(f"No Go module hashes found in {go_sum}")
    return result


def _properties(component: dict) -> list[dict[str, str]]:
    return component.setdefault("properties", [])


def _add_property(component: dict, name: str, value: str) -> None:
    properties = _properties(component)
    if any(prop["name"] == name for prop in properties):
        raise ValueError(f"Duplicate SBOM property: {name}")
    properties.append({"name": name, "value": value})


def _docker_components(root: Path) -> tuple[list[dict], list[dict]]:
    libraries: list[dict] = []
    containers: list[dict] = []
    dockerfiles = [root / "Dockerfile", root / "deploy" / "nats" / "Dockerfile"]
    for dockerfile in dockerfiles:
        text = dockerfile.read_text(encoding="utf-8")
        args = dict(re.findall(r"^ARG ([A-Z0-9_]+)=([^\s]+)\s*$", text, re.MULTILINE))
        relative = dockerfile.relative_to(root).as_posix()
        for line_number, line in enumerate(text.splitlines(), start=1):
            match = re.match(r"^FROM\s+(?:--platform=\S+\s+)?([^\s]+)(?:\s+AS\s+([^\s]+))?", line)
            if not match:
                continue
            reference = match.group(1)
            if reference == "scratch":
                continue  # Empty bundle stage, not an image
            # ARG defaults may reference earlier ARGs (PYTHON_IMAGE uses PYTHON_VERSION); resolve until stable.
            for _ in range(len(args) + 1):
                resolved = reference
                for key, value in args.items():
                    resolved = resolved.replace("${" + key + "}", value)
                if resolved == reference:
                    break
                reference = resolved
            name, separator, version = reference.rpartition(":")
            if not separator:
                name, version = reference, "latest"
            stage = match.group(2)
            component = {
                "bom-ref": f"container:{relative}:{line_number}:{reference}",
                "type": "container",
                "name": name.rsplit("/", 1)[-1],
                "version": version,
                "purl": f"pkg:docker/library/{name}@{version}",
                "properties": [
                    {"name": "wasl:declared-in", "value": f"{relative}:{line_number}"},
                    {"name": "wasl:image-role", "value": "build" if stage else "runtime"},
                    {"name": "wasl:reference", "value": reference},
                ],
            }
            containers.append(component)
        if relative == "Dockerfile":
            for name, version in re.findall(r'"([A-Za-z0-9_.-]+)==([A-Za-z0-9_.-]+)"', text):
                if name == "pip":
                    continue  # Build-only installer; the Dockerfile removes it before the runtime stage is complete.
                libraries.append({
                    "bom-ref": f"pkg:pypi/{name}@{version}",
                    "type": "library",
                    "name": name,
                    "version": version,
                    "purl": f"pkg:pypi/{name}@{version}",
                    "scope": "required",
                    "properties": [
                        {"name": "wasl:declared-in", "value": relative},
                        {"name": "wasl:inventory-note", "value": "Python runtime declaration; use the image SBOM for installed artifacts and licenses"},
                    ],
                })
    return libraries, containers


def augment_sbom(raw: dict, root: Path, *, revision: str, committed_at: str, dirty: bool) -> dict:
    """Make a Trivy source SBOM deterministic and add declarations Trivy misses."""
    if dirty:
        raise ValueError("Refusing to label a dirty source tree as clean release evidence")
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("A full 40-character source revision is required")
    bom = copy.deepcopy(raw)
    if bom.get("bomFormat") != "CycloneDX" or not isinstance(bom.get("components"), list):
        raise ValueError("Expected a CycloneDX SBOM with a component list")

    npm_hashes = _npm_integrity(root / "web" / "pnpm-lock.yaml")
    go_hashes = _go_module_hashes(root / "pipeline" / "go.sum")
    seen_npm: set[tuple[str, str]] = set()
    seen_go: set[tuple[str, str]] = set()
    for component in bom["components"]:
        purl = component.get("purl", "")
        key = (component.get("name", ""), component.get("version", ""))
        if purl.startswith("pkg:npm/"):
            purl_identity = purl[len("pkg:npm/"):].split("?", 1)[0].split("#", 1)[0]
            purl_name, separator, purl_version = purl_identity.rpartition("@")
            if not separator:
                raise ValueError(f"Invalid npm purl in source SBOM: {purl}")
            key = (urllib.parse.unquote(purl_name), urllib.parse.unquote(purl_version))
        if purl.startswith("pkg:npm/") and key in npm_hashes:
            algorithm, encoded = "SHA-512", npm_hashes[key]
            hashes = component.setdefault("hashes", [])
            if not any(item.get("alg") == algorithm for item in hashes):
                hashes.append({"alg": algorithm, "content": base64.b64decode(encoded, validate=True).hex()})
            seen_npm.add(key)
        elif purl.startswith("pkg:golang/") and key in go_hashes:
            _add_property(component, "wasl:go-sum-module-hash", go_hashes[key])
            seen_go.add(key)

    missing_npm = set(npm_hashes) - seen_npm
    missing_go = set(go_hashes) - seen_go
    if missing_npm or missing_go:
        raise ValueError(
            "Trivy source inventory does not match lockfiles: "
            f"missing npm={sorted(missing_npm)!r}, missing Go={sorted(missing_go)!r}"
        )

    metadata = bom.setdefault("metadata", {})
    root_component = metadata.setdefault("component", {})
    reference_map: dict[str, str] = {}
    old_root_ref = root_component.get("bom-ref")
    if old_root_ref:
        reference_map[old_root_ref] = "wasl:source-root"
    root_component["bom-ref"] = "wasl:source-root"
    uuid_ref = re.compile(r"[0-9a-fA-F-]{36}")
    by_ref = {c.get("bom-ref", ""): c for c in bom["components"]}
    children = {d.get("ref"): d.get("dependsOn", []) for d in bom.get("dependencies", [])}
    # Scanner UUIDs on libraries repeat per manifest (for example two Go modules sharing x/sys). Name each by package
    # identity plus the manifest whose dependency tree reaches it.
    owner: dict[str, str] = {}
    for component in sorted((c for c in bom["components"] if uuid_ref.fullmatch(c.get("bom-ref", ""))
                             and c.get("type") == "application" and c.get("name")), key=lambda c: c["name"]):
        pending = list(children.get(component["bom-ref"], []))
        seen = set()
        while pending:
            child = pending.pop()
            if child in seen:
                continue
            seen.add(child)
            if uuid_ref.fullmatch(child):
                owner.setdefault(child, component["name"])
            pending.extend(children.get(child, []))
    stabilized: set[str] = set()
    for component in bom["components"]:
        reference = component.get("bom-ref", "")
        if uuid_ref.fullmatch(reference):
            if component.get("type") == "application" and component.get("name"):
                replacement = "wasl:manifest:" + component["name"]
            elif component.get("type") == "library" and component.get("purl") and reference in owner:
                replacement = f"wasl:library:{component['purl']}@{owner[reference]}"
            else:
                raise ValueError(f"Cannot stabilize scanner component reference: {reference}")
            if replacement in stabilized:
                raise ValueError(f"Duplicate stabilized component reference: {replacement}")
            stabilized.add(replacement)
            reference_map[reference] = replacement
            component["bom-ref"] = replacement
    for dependency in bom.get("dependencies", []):
        dependency["ref"] = reference_map.get(dependency.get("ref"), dependency.get("ref"))
        dependency["dependsOn"] = [reference_map.get(ref, ref) for ref in dependency.get("dependsOn", [])]

    python_libraries, containers = _docker_components(root)
    existing_refs = {component.get("bom-ref") for component in bom["components"]}
    for component in python_libraries + containers:
        if component["bom-ref"] not in existing_refs:
            bom["components"].append(component)
            existing_refs.add(component["bom-ref"])

    metadata["timestamp"] = committed_at
    root_component.update({"type": "application", "name": "wasl", "version": revision})
    metadata["properties"] = [
        {"name": "wasl:source-revision", "value": revision},
        {"name": "wasl:source-tree", "value": "clean"},
        {"name": "wasl:inventory-scope", "value": "source; not a built container SBOM"},
        {"name": "wasl:container-inventory", "value": "declared base images only; installed OS packages require image scanning"},
        {"name": "wasl:license-discovery-system", "value": platform.system().lower()},
        {"name": "wasl:license-discovery-machine", "value": platform.machine().lower()},
    ]
    bom["components"].sort(key=lambda component: component.get("bom-ref", ""))
    for dependency in bom.get("dependencies", []):
        dependency["dependsOn"] = sorted(dependency.get("dependsOn", []))
    bom.get("dependencies", []).sort(key=lambda dependency: dependency.get("ref", ""))
    bom["serialNumber"] = "urn:uuid:" + str(uuid.uuid5(uuid.NAMESPACE_URL, "wasl-source-sbom:" + revision))
    bom["version"] = 1
    return bom


def bind_image_sbom(raw: dict, *, image: str, revision: str, committed_at: str) -> dict:
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("A full 40-character source revision is required")
    if not image or not isinstance(raw.get("components"), list):
        raise ValueError("An image reference and CycloneDX component list are required")
    bom = copy.deepcopy(raw)
    metadata = bom.setdefault("metadata", {})
    metadata["timestamp"] = committed_at
    original_component = metadata.get("component", {})
    component = metadata.setdefault("component", {})
    component.update({"type": "container", "name": image, "version": revision})
    metadata["properties"] = [
        {"name": "wasl:source-revision", "value": revision},
        {"name": "wasl:image-reference", "value": image},
        {"name": "wasl:inventory-scope", "value": "built container image"},
    ]
    bom["serialNumber"] = "urn:uuid:" + str(uuid.uuid5(
        uuid.NAMESPACE_URL,
        f"wasl-image-sbom:{revision}:{image}:{original_component.get('bom-ref', '')}",
    ))
    bom["version"] = 1
    return bom


SEVERITIES = ("UNKNOWN", "LOW", "MEDIUM", "HIGH", "CRITICAL")


def _results(raw: dict) -> list[dict]:
    results = raw.get("Results", [])
    if not isinstance(results, list):
        raise ValueError("Trivy Results must be a list; malformed output is not a clean scan")
    return results


def _context(revision: str, committed_at: str, scanner: dict) -> dict:
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("A full 40-character source revision is required")
    return {
        "source": {"revision": revision, "committed_at": committed_at},
        "scanner": scanner,
    }


def normalize_vulnerability_report(raw: dict, *, revision: str, committed_at: str, scanner: dict) -> dict:
    findings: list[dict] = []
    for result in _results(raw):
        vulnerabilities = result.get("Vulnerabilities") or []
        if not isinstance(vulnerabilities, list):
            raise ValueError("Trivy Vulnerabilities must be a list or null")
        for vulnerability in vulnerabilities:
            findings.append({
                "id": vulnerability.get("VulnerabilityID", ""),
                "package": vulnerability.get("PkgName", ""),
                "installed_version": vulnerability.get("InstalledVersion", ""),
                "fixed_version": vulnerability.get("FixedVersion", ""),
                "status": vulnerability.get("Status", ""),
                "severity": vulnerability.get("Severity", "UNKNOWN"),
                "title": vulnerability.get("Title", ""),
                "primary_url": vulnerability.get("PrimaryURL", ""),
                "target": result.get("Target", ""),
                "class": result.get("Class", ""),
                "type": result.get("Type", ""),
            })
    findings.sort(key=lambda item: (
        item["target"], item["package"], item["installed_version"], item["id"], item["severity"],
    ))
    by_severity = {severity: 0 for severity in SEVERITIES}
    for finding in findings:
        by_severity[finding["severity"]] = by_severity.get(finding["severity"], 0) + 1
    report = {
        "schema_version": 1,
        **_context(revision, committed_at, scanner),
        "policy": {
            "severities": list(SEVERITIES),
            "include_development_dependencies": True,
            "fail_on": "any reported vulnerability",
            "suppressions": [],
        },
        "summary": {"total": len(findings), "by_severity": by_severity},
        "findings": findings,
        "limitations": [
            "A source scan is not a built-container or deployed-image scan.",
            "A module-level advisory does not prove that the vulnerable code path is reachable.",
        ],
    }
    return report


def vulnerability_exit_code(report: dict) -> int:
    total = report.get("summary", {}).get("total")
    if not isinstance(total, int) or total < 0 or total != len(report.get("findings", [])):
        raise ValueError("Invalid vulnerability summary; refusing to treat it as clean")
    return int(total > 0)


def normalize_license_report(raw: dict, *, revision: str, committed_at: str, scanner: dict) -> dict:
    packages: list[dict] = []
    license_counts: dict[str, int] = {}
    for result in _results(raw):
        raw_packages = result.get("Packages") or []
        if not isinstance(raw_packages, list):
            raise ValueError("Trivy Packages must be a list or null")
        for package in raw_packages:
            if package.get("Relationship") == "root":
                continue
            licenses = package.get("Licenses") or []
            if not isinstance(licenses, list):
                raise ValueError("Trivy Licenses must be a list or null")
            packages.append({
                "target": result.get("Target", ""),
                "name": package.get("Name", ""),
                "version": package.get("Version", ""),
                "purl": package.get("Identifier", {}).get("PURL", ""),
                "development": bool(package.get("Dev", False)),
                "licenses": sorted(licenses),
            })
            for license_name in licenses:
                license_counts[license_name] = license_counts.get(license_name, 0) + 1
    packages.sort(key=lambda item: (item["target"], item["name"], item["version"], item["purl"]))
    with_license = sum(bool(package["licenses"]) for package in packages)
    return {
        "schema_version": 1,
        **_context(revision, committed_at, scanner),
        "scan_environment": {"system": platform.system().lower(), "machine": platform.machine().lower()},
        "summary": {
            "packages": len(packages),
            "with_discovered_license": with_license,
            "without_discovered_license": len(packages) - with_license,
            "license_counts": dict(sorted(license_counts.items())),
        },
        "packages": packages,
        "limitations": [
            "Scanner-discovered license identifiers are evidence, not legal clearance.",
            "An empty licenses list means no license was discovered; it does not mean public domain or no license obligation.",
            "Install frontend dependencies before scanning so local package license metadata can be inspected.",
            "Source reports do not inventory licenses inside built container images; scan release images separately.",
        ],
    }


SOURCE_PATHS = (
    "web/package.json",
    "web/pnpm-lock.yaml",
    "pipeline/go.mod",
    "pipeline/go.sum",
    "Dockerfile",
    "deploy/nats/Dockerfile",
)
EVIDENCE_FILES = ("repository.cdx.json", "vulnerabilities.json", "licenses.json")


def _git(root: Path, *arguments: str) -> str:
    return subprocess.run(
        ["git", "-C", str(root), *arguments], check=True, text=True,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE,
    ).stdout.strip()


def _commit_time(root: Path, revision: str) -> str:
    value = _git(root, "show", "-s", "--format=%cI", revision)
    return datetime.datetime.fromisoformat(value).astimezone(datetime.timezone.utc).isoformat().replace("+00:00", "Z")


def resolve_context(root: Path, tracked: Path | None = None) -> dict:
    """Resolve clean current provenance, or validate tracked provenance for regeneration."""
    if _git(root, "status", "--porcelain=v1", "--untracked-files=normal"):
        raise ValueError("Source tree is dirty; commit changes before generating or verifying evidence")
    head = _git(root, "rev-parse", "HEAD")
    if tracked is None:
        revision = head
    else:
        source = _read_json(tracked).get("source", {})
        revision = source.get("revision", "")
        if not re.fullmatch(r"[0-9a-f]{40}", revision):
            raise ValueError("Tracked evidence has no valid source revision")
        subprocess.run(
            ["git", "-C", str(root), "merge-base", "--is-ancestor", revision, head],
            check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
        changed = subprocess.run(
            ["git", "-C", str(root), "diff", "--name-only", revision, head, "--", *SOURCE_PATHS],
            check=True, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        ).stdout.strip()
        if changed:
            raise ValueError(f"Dependency/container declarations changed after tracked evidence: {changed}")
    return {"revision": revision, "committed_at": _commit_time(root, revision)}


def _read_json(path: Path) -> dict:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"Expected a JSON object in {path}")
    return value


def _write_json(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + ".tmp")
    temporary.write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    temporary.replace(path)


def _scanner(raw: dict) -> dict:
    version = raw.get("Version")
    if not isinstance(version, str) or not version:
        raise ValueError("Trivy version output lacks Version")
    return {"name": "trivy", "version": version}


def _canonical_source_sbom(bom: dict) -> dict:
    canonical = copy.deepcopy(bom)
    for component in canonical.get("components", []):
        component.pop("licenses", None)
    properties = canonical.get("metadata", {}).get("properties", [])
    canonical.get("metadata", {})["properties"] = [
        prop for prop in properties
        if prop.get("name") not in {"wasl:license-discovery-system", "wasl:license-discovery-machine"}
    ]
    return canonical


def compare_evidence(generated: dict[str, dict], tracked: dict[str, dict]) -> None:
    if generated["vulnerabilities.json"] != tracked["vulnerabilities.json"]:
        raise ValueError("Tracked evidence is stale: vulnerabilities.json; run scripts/supply-chain.sh generate")
    generated_environment = generated["licenses.json"].get("scan_environment")
    tracked_environment = tracked["licenses.json"].get("scan_environment")
    if generated_environment == tracked_environment:
        for name in EVIDENCE_FILES:
            if generated[name] != tracked[name]:
                raise ValueError(f"Tracked evidence is stale: {name}; run scripts/supply-chain.sh generate")
        return

    if _canonical_source_sbom(generated["repository.cdx.json"]) != _canonical_source_sbom(tracked["repository.cdx.json"]):
        raise ValueError("Tracked source SBOM changed beyond platform-specific license discovery")
    generated_license = generated["licenses.json"]
    tracked_license = tracked["licenses.json"]
    for key in ("schema_version", "source", "scanner", "limitations"):
        if generated_license.get(key) != tracked_license.get(key):
            raise ValueError(f"Tracked licenses.json metadata changed: {key}")
    def identities(report: dict) -> list[tuple]:
        return [
            (p["target"], p["name"], p["version"], p["purl"], p["development"])
            for p in report["packages"]
        ]
    if identities(generated_license) != identities(tracked_license):
        raise ValueError("Tracked licenses.json package identity inventory is stale")
    tracked_packages = {
        (p["target"], p["name"], p["version"], p["purl"], p["development"]): p["licenses"]
        for p in tracked_license["packages"]
    }
    for package in generated_license["packages"]:
        key = (package["target"], package["name"], package["version"], package["purl"], package["development"])
        if package["licenses"] and tracked_packages[key] and package["licenses"] != tracked_packages[key]:
            raise ValueError(f"Conflicting license findings for {key[1]}@{key[2]}")


def build_manifest(directory: Path, context: dict, scanner_raw: dict, *, offline: bool) -> dict:
    scanner = _scanner(scanner_raw)
    database = scanner_raw.get("VulnerabilityDB")
    if not isinstance(database, dict) or not database.get("UpdatedAt"):
        raise ValueError("Trivy provenance lacks vulnerability database metadata")
    artifacts = []
    for name in EVIDENCE_FILES:
        path = directory / name
        artifacts.append({"path": name, "sha256": hashlib.sha256(path.read_bytes()).hexdigest()})
    return {
        "schema_version": 1,
        "source": {"revision": context["revision"], "committed_at": context["committed_at"]},
        "scanner": {**scanner, "vulnerability_database": database},
        "scan_mode": "offline with a pre-seeded database" if offline else "online database refresh permitted",
        "artifacts": artifacts,
        "notes": [
            "Database timestamps and digest provenance belong to this manifest, not the normalized findings.",
            "A cold offline scan requires a pre-seeded Trivy database; this tooling does not claim otherwise.",
        ],
    }


def _load_context(path: Path) -> dict:
    context = _read_json(path)
    if not re.fullmatch(r"[0-9a-f]{40}", context.get("revision", "")) or not context.get("committed_at"):
        raise ValueError("Invalid source context")
    return context


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    context_parser = subparsers.add_parser("context")
    context_parser.add_argument("--root", type=Path, required=True)
    context_parser.add_argument("--tracked", type=Path)
    context_parser.add_argument("--output", type=Path, required=True)

    for name in ("augment-sbom", "normalize-vulnerabilities", "normalize-licenses"):
        command = subparsers.add_parser(name)
        command.add_argument("--input", type=Path, required=True)
        command.add_argument("--context", type=Path, required=True)
        command.add_argument("--output", type=Path, required=True)
        if name != "augment-sbom":
            command.add_argument("--scanner", type=Path, required=True)

    image_parser = subparsers.add_parser("bind-image-sbom")
    image_parser.add_argument("--input", type=Path, required=True)
    image_parser.add_argument("--context", type=Path, required=True)
    image_parser.add_argument("--image", required=True)
    image_parser.add_argument("--output", type=Path, required=True)

    manifest_parser = subparsers.add_parser("manifest")
    manifest_parser.add_argument("--directory", type=Path, required=True)
    manifest_parser.add_argument("--context", type=Path, required=True)
    manifest_parser.add_argument("--scanner", type=Path, required=True)
    manifest_parser.add_argument("--offline", action="store_true")
    manifest_parser.add_argument("--output", type=Path, required=True)

    gate_parser = subparsers.add_parser("gate")
    gate_parser.add_argument("--input", type=Path, required=True)

    compare_parser = subparsers.add_parser("compare")
    compare_parser.add_argument("--generated", type=Path, required=True)
    compare_parser.add_argument("--tracked", type=Path, required=True)

    args = parser.parse_args(argv)
    try:
        if args.command == "context":
            _write_json(args.output, resolve_context(args.root.resolve(), args.tracked))
        elif args.command == "augment-sbom":
            context = _load_context(args.context)
            root = Path(__file__).resolve().parents[1]
            bom = augment_sbom(
                _read_json(args.input), root,
                revision=context["revision"], committed_at=context["committed_at"], dirty=False,
            )
            _write_json(args.output, bom)
        elif args.command in ("normalize-vulnerabilities", "normalize-licenses"):
            context = _load_context(args.context)
            scanner = _scanner(_read_json(args.scanner))
            normalizer = normalize_vulnerability_report if args.command == "normalize-vulnerabilities" else normalize_license_report
            report = normalizer(
                _read_json(args.input), revision=context["revision"],
                committed_at=context["committed_at"], scanner=scanner,
            )
            _write_json(args.output, report)
        elif args.command == "bind-image-sbom":
            context = _load_context(args.context)
            _write_json(args.output, bind_image_sbom(
                _read_json(args.input), image=args.image, revision=context["revision"],
                committed_at=context["committed_at"],
            ))
        elif args.command == "manifest":
            _write_json(args.output, build_manifest(
                args.directory, _load_context(args.context), _read_json(args.scanner), offline=args.offline,
            ))
        elif args.command == "gate":
            report = _read_json(args.input)
            print(json.dumps(report.get("summary", {}), sort_keys=True))
            return vulnerability_exit_code(report)
        elif args.command == "compare":
            generated = {name: _read_json(args.generated / name) for name in EVIDENCE_FILES}
            tracked = {name: _read_json(args.tracked / name) for name in EVIDENCE_FILES}
            compare_evidence(generated, tracked)
            print("Tracked evidence matches fresh output (platform-specific license discovery canonicalized when needed).")
        return 0
    except (ValueError, OSError, json.JSONDecodeError, subprocess.CalledProcessError) as error:
        print(f"supply-chain error: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
