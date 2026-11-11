"""Production notice-evidence tests (Task 2). Needs scripts/notices.py from Task 1."""
import hashlib
import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SC = ROOT / "supply-chain"
COUNTS = {"direct-gitHead": 47, "tag-candidate": 21, "unresolved-native": 13}


def _manifest():
    return json.loads((SC / "notice-evidence.json").read_text(encoding="utf-8"))


def _lock_pairs():
    pairs = set()
    for line in (ROOT / "web" / "pnpm-lock.yaml").read_text(encoding="utf-8").splitlines():
        m = re.match(r"^  '?((?:@[^/']+/)?[^@'\s]+)@([^':\s(]+)'?:\s*$", line)
        if m:
            pairs.add((m.group(1), m.group(2)))
    return pairs


class ProductionEvidence(unittest.TestCase):
    def setUp(self):
        self.pkgs = _manifest()["packages"]

    def test_81_unique_rows_and_class_totals(self):
        keys = [(p["name"], p["version"]) for p in self.pkgs]
        self.assertEqual(len(keys), 81)
        self.assertEqual(len(set(keys)), 81)
        got = {}
        for p in self.pkgs:
            got[p["evidence_class"]] = got.get(p["evidence_class"], 0) + 1
        self.assertEqual(got, COUNTS)

    def test_rows_exist_at_same_version_in_lockfile(self):
        lock = _lock_pairs()
        for p in self.pkgs:
            self.assertIn((p["name"], p["version"]), lock, p["name"])

    def test_class_membership(self):
        def cls(prefix):
            return {p["evidence_class"] for p in self.pkgs if p["name"].startswith(prefix)}

        self.assertEqual(len([p for p in self.pkgs if p["name"].startswith("@next/")]), 7)
        self.assertEqual(len([p for p in self.pkgs if p["name"].startswith("@rolldown/")]), 14)
        self.assertEqual(cls("@next/"), {"tag-candidate"})
        self.assertEqual(cls("@rolldown/"), {"tag-candidate"})
        unresolved = [p for p in self.pkgs if p["evidence_class"] == "unresolved-native"]
        self.assertEqual(sum("sharp-libvips" in p["name"] for p in unresolved), 9)
        self.assertEqual(sum(p["name"].startswith("@img/sharp-") and "libvips" not in p["name"] for p in unresolved), 4)
        self.assertEqual({p["name"] for p in self.pkgs if p["name"].startswith("@img/")}, {p["name"] for p in unresolved})

    def test_notice_hashes_and_row_shapes(self):
        for p in self.pkgs:
            if p["evidence_class"] == "unresolved-native":
                self.assertEqual(p["notices"], [])
                self.assertIn("No full", p["limitation"])
                self.assertEqual(set(p["source"]), {"url"})
                continue
            self.assertTrue(p["notices"])
            for n in p["notices"]:
                data = (SC / n["path"]).read_bytes()
                data.decode("utf-8")
                self.assertEqual(hashlib.sha256(data).hexdigest(), n["sha256"])
            if p["evidence_class"] == "tag-candidate":
                self.assertRegex(p["source"]["resolved_commit"], r"^[0-9a-f]{40}$")
                self.assertTrue(p["source"]["tag"].startswith("v"))
                self.assertIn("not a direct artifact-to-commit binding", p["qualification"])
            else:
                self.assertRegex(p["source"]["commit"], r"^[0-9a-f]{40}$")

    def test_sharp_wasm32_keeps_mit_limitation(self):
        row = next(p for p in self.pkgs if p["name"] == "@img/sharp-wasm32")
        self.assertEqual(row["declaration"], "Apache-2.0 AND LGPL-3.0-or-later AND MIT")
        self.assertIn("MIT", row["limitation"])

    def test_bundle_matches_renderer_and_wording(self):
        from scripts import notices  # Task 1 API

        manifest = _manifest()
        bundle = (SC / "THIRD-PARTY-NOTICES.txt").read_text(encoding="utf-8")
        self.assertEqual(bundle, notices.render_notice_bundle(manifest, ROOT))
        self.assertIn("does not prove shipment", bundle)
        self.assertIn("UNRESOLVED", bundle)
        self.assertNotIn("legally cleared", bundle)
        for p in self.pkgs:
            if p["evidence_class"] == "tag-candidate":
                self.assertNotIn(f"{p['name']}@{p['version']}\nDeclaration: {p['declaration']}\nEvidence class: direct", bundle)

    def test_production_validator(self):
        from scripts import notices

        notices.validate_notice_manifest(_manifest(), ROOT, expected_counts=COUNTS)


if __name__ == "__main__":
    unittest.main()
