#!/usr/bin/env python3
"""Tests for the submission-evidence consistency checker (stdlib only)."""
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import check_evidence_consistency as chk  # noqa: E402

REAL = Path(__file__).resolve().parent.parent
FILES = ["README.md", "pipeline/RESULTS.md", "docs/labelling/agreement.json",
         "data/evidence/reports43.json"] + [
    "docs/" + n for n in ("BACKEND-HANDOFF.md", "BRIEF.md", "WRITEUP.md", "PITCH.md",
                          "DEMO-RECORDING-GUIDE.md", "SUBMISSION-DRAFT.md")] + ["docs/labelling/README.md"]


class Fixture(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        for f in FILES:
            (self.root / f).parent.mkdir(parents=True, exist_ok=True)
            shutil.copy(REAL / f, self.root / f)

    def tearDown(self):
        self.tmp.cleanup()

    def mutate(self, rel, old, new, count=1):
        p = self.root / rel
        t = p.read_text(encoding="utf-8")
        self.assertIn(old, t, f"fixture text missing in {rel}")
        p.write_text(t.replace(old, new, count), encoding="utf-8")

    def rules(self):
        return {f.rule for f in chk.check(self.root)}


class RealRepo(unittest.TestCase):
    def test_repository_is_consistent(self):
        found = chk.check(REAL)
        self.assertEqual([], [str(f) for f in found])


class Mutations(Fixture):
    def test_clean_copy_passes(self):
        self.assertEqual(set(), self.rules())

    def test_stale_final_figures_flagged(self):
        p = self.root / "docs/PITCH.md"
        p.write_text(p.read_text(encoding="utf-8") + "\nRecall 0.782, 0/3,025 false closures.\n", encoding="utf-8")
        self.assertIn("stale-figure", self.rules())

    def test_stale_marked_superseded_is_allowed(self):
        p = self.root / "docs/PITCH.md"
        p.write_text(p.read_text(encoding="utf-8") + "\nSuperseded earlier run: 0/3,025 false closures.\n", encoding="utf-8")
        self.assertNotIn("stale-figure", self.rules())

    def test_current_figures_must_match_results(self):
        self.mutate("README.md", "0/16,486", "0/16,487")
        self.assertIn("current-figures", self.rules())

    def test_kappa_wording(self):
        self.mutate("docs/BRIEF.md", "not independent human double-labelling", "independently double-labelled", 1)
        self.assertIn("kappa-wording", self.rules())

    def test_kappa_data(self):
        self.mutate("docs/labelling/agreement.json", '"kappa_ab": 1.0', '"kappa_ab": 0.9')
        self.assertIn("kappa-data", self.rules())

    def test_accuracy_claim(self):
        self.mutate("docs/SUBMISSION-DRAFT.md", "19 correct and 0 unsafe", "18 correct and 1 unsafe")
        self.assertIn("accuracy-claim", self.rules())

    def test_stale_pending_labelling(self):
        p = self.root / "docs/PITCH.md"
        p.write_text(p.read_text(encoding="utf-8") + "\nThe key is LLM-produced until our human double-labelling is done.\n", encoding="utf-8")
        self.assertIn("stale-labelling", self.rules())

    def test_overclaim(self):
        p = self.root / "docs/BRIEF.md"
        p.write_text(p.read_text(encoding="utf-8") + "\nThe reports are clinically validated.\n", encoding="utf-8")
        self.assertIn("overclaim", self.rules())

    def test_negated_claim_allowed(self):
        p = self.root / "docs/BRIEF.md"
        p.write_text(p.read_text(encoding="utf-8") + "\nThe reports are not clinically validated.\n", encoding="utf-8")
        self.assertNotIn("overclaim", self.rules())

    def test_ldct_ifhas_caveat(self):
        self.mutate("docs/BRIEF.md", "non-blind", "blind")
        self.assertIn("ldct-ifhas-caveat", self.rules())

    def test_agent_wording(self):
        self.mutate("docs/DEMO-RECORDING-GUIDE.md", "five fewer closures than unguarded", "more closures")
        self.assertIn("agent-wording", self.rules())

    def test_status_labels(self):
        self.mutate("docs/SUBMISSION-DRAFT.md", "No clinical deployment or patient-outcome improvement is claimed.", "")
        self.assertIn("status-labels", self.rules())


if __name__ == "__main__":
    unittest.main()
