#!/usr/bin/env python3
"""Tests for future final-evaluation manifest annotation provenance."""
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / "scripts"))
try:
    import final_manifest
except ImportError:
    final_manifest = None

LIMITATION = "not independent human double-labelling"


class AnnotationProvenance(unittest.TestCase):
    def setUp(self):
        self.assertIsNotNone(final_manifest, "future manifest generator is not implemented")
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)

    def tearDown(self):
        if hasattr(self, "tmp"):
            self.tmp.cleanup()

    def write_reports(self, text):
        path = self.root / "reports.json"
        path.write_text(text, encoding="utf-8")
        return path

    def test_verified_metadata_drives_manifest_wording(self):
        got = final_manifest.annotation_provenance(ROOT / "data/evidence/reports43.json")
        self.assertEqual("clinically_verified", got["status"])
        self.assertIn("clinically verified model-assisted", got["labelling"])
        self.assertIn(LIMITATION, got["labelling"])
        self.assertEqual("synthetic cohorts, reviewers, capacity", got["limitations"][0])
        self.assertIn("clinically verified model-assisted answer key", got["limitations"][1])
        self.assertIn(LIMITATION, got["limitations"][1])
        self.assertNotIn("no clinician labels", " ".join(got["limitations"]))
        self.assertNotIn("LLM-labelled", " ".join(got["limitations"]))

    def test_unverified_metadata_never_claims_clinical_verification(self):
        path = self.write_reports(json.dumps({"verification": {"status": "unverified"}}))
        got = final_manifest.annotation_provenance(path)
        self.assertEqual("unverified", got["status"])
        self.assertIn("annotation status: unverified", got["labelling"])
        self.assertNotIn("clinically verified", got["labelling"])
        self.assertIn("answer key annotation status: unverified", got["limitations"][1])
        self.assertIn(LIMITATION, got["limitations"][1])

    def test_contradictory_unverified_double_labelling_fails_closed(self):
        path = self.write_reports(json.dumps({"verification": {
            "status": "unverified", "human_double_labelled": True,
        }}))
        with self.assertRaises(ValueError):
            final_manifest.annotation_provenance(path)

    def test_unsupported_annotation_status_fails_closed(self):
        path = self.write_reports(json.dumps({"verification": {
            "status": "independently_human_double_labelled", "human_double_labelled": True,
        }}))
        with self.assertRaises(ValueError):
            final_manifest.annotation_provenance(path)

    def test_malformed_metadata_fails_closed(self):
        malformed = [
            "{not json",
            "[]",
            "{}",
            '{"verification": []}',
            '{"verification": {}}',
            '{"verification": {"status": null}}',
            '{"verification": {"status": 7}}',
            '{"verification": {"status": "clinically_verified", "human_double_labelled": true}}',
            '{"verification": {"status": "clinically_verified", "human_double_labelled": false, "method": "clinician checked"}}',
        ]
        for text in malformed:
            with self.subTest(text=text):
                with self.assertRaises(ValueError):
                    final_manifest.annotation_provenance(self.write_reports(text))


class ManifestGeneration(unittest.TestCase):
    def setUp(self):
        self.assertIsNotNone(final_manifest, "future manifest generator is not implemented")

    def test_generated_manifest_uses_current_verification_without_changing_seeds(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "manifest.json"
            env = os.environ.copy()
            env.update({
                "REPORTS": str(ROOT / "data/evidence/reports43.json"),
                "MODELS": str(ROOT / "models"),
                "LUNG": "1088",
                "DR": "1200",
                "BREAST": "70",
                "LDCT": "0",
                "IFHAS": "0",
                "CRC": "2040",
                "CERV": "816",
                "PERSONS": "0",
            })
            subprocess.run(
                [sys.executable, "-I", str(ROOT / "scripts/final_manifest.py"),
                 str(ROOT), str(out), "11", "23"],
                check=True, capture_output=True, text=True, env=env,
            )
            got = json.loads(out.read_text(encoding="utf-8"))
        self.assertEqual("clinically_verified", got["annotation_status"])
        self.assertEqual([11, 23], got["seeds"])
        self.assertIn(LIMITATION, got["labelling"])
        self.assertIn(LIMITATION, got["limitations"][2])
        self.assertNotIn("no clinician labels", " ".join(got["limitations"]))
        self.assertNotIn("LLM-labelled", " ".join(got["limitations"]))


if __name__ == "__main__":
    unittest.main()
