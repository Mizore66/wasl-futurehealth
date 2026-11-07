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
    def test_kpi_32_0_flagged(self):
        self.mutate("docs/PITCH.md", "| Wasl | 31.9% |", "| Wasl | 32.0% |")
        self.assertIn("kpi-evidence", self.rules())

    def test_unqualified_within_one_point_flagged(self):
        p = self.root / "docs/PITCH.md"
        p.write_text(p.read_text(encoding="utf-8") + "\nWasl is within one percentage point of truth.\n", encoding="utf-8")
        self.assertIn("kpi-evidence", self.rules())

    def test_demo_caveat_required(self):
        self.mutate("docs/DEMO-RECORDING-GUIDE.md", "3.1 under v2", "3 under v2")
        self.assertIn("kpi-evidence", self.rules())

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


README_PENDING = ("The first run found 4 CRC false closures, all one bug. Held-out recall rose. "
                  "The human double-labelling of the 43 reports is still pending ([`docs/labelling/`](docs/labelling/)).")
PITCH_OLD = "0/1,903 lung, 0/5,625 retinopathy and 0/338 breast false closures in the frozen held-out evaluation."
README_OLD = ("In the earlier `5496563` evaluation, dev-noise recall was 0.842 with 0/3,147 CRC, "
              "0/1,403 cervical, 0/2,025 lung and 0/5,621 DR false closures.")


class Round1Findings(Fixture):
    def append(self, rel, text):
        p = self.root / rel
        p.write_text(p.read_text(encoding="utf-8") + "\n" + text + "\n", encoding="utf-8")

    def rules_in(self, rel):
        return {f.rule for f in chk.check(self.root) if f.file == rel}

    # finding 1
    def test_readme_pending_labelling_with_heldout_on_line(self):
        self.append("README.md", README_PENDING)
        self.assertIn("stale-labelling", self.rules())

    def test_readme_pending_labelling_without_heldout(self):
        self.append("README.md", "The human double-labelling of the 43 reports is still pending.")
        self.assertIn("stale-labelling", self.rules())

    def test_heldout_pending_statement_alone_is_allowed(self):
        self.append("README.md", "Fresh held-out corpora have no clinician labels yet.")
        self.assertNotIn("stale-labelling", self.rules())

    # findings 2 and 3
    def test_pitch_old_lung_dr_figures_flagged(self):
        self.append("docs/PITCH.md", PITCH_OLD)
        self.assertIn("stale-figure", self.rules())

    def test_readme_old_dev_figures_flagged(self):
        self.append("README.md", README_OLD)
        self.assertIn("stale-figure", self.rules())

    def test_old_figure_marked_superseded_in_same_sentence_allowed(self):
        self.append("README.md", "The earlier run reported 0/3,147 CRC false closures; it is superseded.")
        self.assertNotIn("stale-figure", self.rules())

    def test_superseded_word_elsewhere_in_line_does_not_exempt(self):
        self.append("README.md", "Run X is superseded by Y. Current result: 0/3,147 CRC false closures.")
        self.assertIn("stale-figure", self.rules())

    def test_numeric_boundary_no_stale_substring_false_positive(self):
        self.append("README.md", "A benign count of 13,025 items and 0.7821 ratio.")
        self.assertNotIn("stale-figure", self.rules())

    def test_numeric_boundary_current_figure_not_substring(self):
        self.mutate("README.md", "0/3,035 CRC", "0/13,035 CRC")
        self.assertIn("current-figures", self.rules())

    def test_wrong_current_value_in_claim_line(self):
        self.append("docs/PITCH.md", "Headline: 0/9,999 wrong-patient messages in the frozen held-out evaluation.")
        self.assertIn("current-figures", self.rules())

    def test_wrong_consent_and_hours_in_claim_line(self):
        self.append("docs/BRIEF.md", "In the frozen held-out evaluation we measured 0/11,000 messages sent without granted consent.")
        self.assertIn("current-figures", self.rules())

    def test_wrong_hours_in_claim_line(self):
        self.append("docs/BRIEF.md", "In the frozen held-out evaluation reviewer time was 9.1 h per 1,000 loops.")
        self.assertIn("current-figures", self.rules())

    def test_wrong_crc_recall_in_claim_line(self):
        self.append("docs/PITCH.md", "CRC precision 1.000, recall 0.700 on held-out noise.")
        self.assertIn("current-figures", self.rules())

    def test_sensitivity_line_not_confused_with_headline(self):
        self.append("docs/BRIEF.md", "Clean-review sensitivity: wrong-patient messages 0/16,534.")
        self.assertNotIn("current-figures", self.rules())

    # finding 4
    def test_writeup_needs_current_row(self):
        p = self.root / "docs/WRITEUP.md"
        t = p.read_text(encoding="utf-8")
        t = "\n".join(l for l in t.splitlines()
                      if not ("075abc5" in l and "supersed" not in l.lower()))
        p.write_text(t, encoding="utf-8")
        self.assertIn("current-figures", self.rules_in("docs/WRITEUP.md"))

    def test_writeup_current_row_with_wrong_figure(self):
        self.mutate("docs/WRITEUP.md", "0/3,035", "0/3,036", 2)
        self.assertIn("current-figures", self.rules_in("docs/WRITEUP.md"))

    # finding 5
    def test_zero_124_without_agreed_but_wrong(self):
        self.mutate("docs/BRIEF.md", "0/124 agreed-but-wrong answers", "0/124 errors")
        self.mutate("docs/BRIEF.md", "current agreed-but-wrong counts", "current counts")
        self.assertIn("ldct-ifhas-caveat", self.rules())

    def test_zero_124_without_nonblind_in_file(self):
        self.mutate("README.md", "non-blind", "informal")
        self.assertIn("ldct-ifhas-caveat", self.rules_in("README.md"))

    def test_ldct_not_independent_wording_removed(self):
        self.mutate("README.md", "not independent reader-generalisation", "good generalisation")
        self.assertIn("ldct-ifhas-caveat", self.rules_in("README.md"))

    def test_kappa_requires_model_assisted(self):
        self.append("docs/BRIEF.md", "\n\nAgreement was κ = 1.000, not independent human double-labelling.")
        self.assertIn("kappa-wording", self.rules())

    def test_missing_doc_file_reported(self):
        (self.root / "docs/PITCH.md").unlink()
        self.assertIn("missing-file", self.rules())

    def test_missing_results_reported(self):
        (self.root / "pipeline/RESULTS.md").unlink()
        self.assertIn("current-figures", self.rules())

    def test_malformed_final_heading(self):
        self.mutate("pipeline/RESULTS.md", "### Frozen seven-programme evaluation at `075abc5`", "### Frozen evaluation", 1)
        self.assertIn("current-figures", self.rules())

    def test_malformed_final_table(self):
        self.mutate("pipeline/RESULTS.md", "Wrong-patient messages / without granted consent", "Wrong patient", 3)
        self.assertIn("current-figures", self.rules())

    def test_malformed_agreement_json(self):
        (self.root / "docs/labelling/agreement.json").write_text("{not json", encoding="utf-8")
        self.assertIn("kappa-data", self.rules())

    def test_missing_agreement_json(self):
        (self.root / "docs/labelling/agreement.json").unlink()
        self.assertIn("kappa-data", self.rules())

    def test_malformed_reports43(self):
        (self.root / "data/evidence/reports43.json").write_text("[]", encoding="utf-8")
        self.assertIn("kappa-data", self.rules())

    def test_per_file_current_figure_coverage(self):
        for rel in ("README.md", "docs/SUBMISSION-DRAFT.md", "docs/BRIEF.md", "docs/PITCH.md"):
            with self.subTest(rel=rel):
                self.setUp()
                p = self.root / rel
                p.write_text(p.read_text(encoding="utf-8").replace("0/3,035", "0/3,036"), encoding="utf-8")
                self.assertIn("current-figures", self.rules_in(rel))

    def test_per_file_stale_and_overclaim_coverage(self):
        for rel in chk.DOCS:
            with self.subTest(rel=rel):
                self.setUp()
                self.append(rel, "Older run: 0/14,451 wrong-patient messages.")
                self.append(rel, "The reports are clinically validated.")
                got = self.rules_in(rel)
                self.assertIn("stale-figure", got)
                self.assertIn("overclaim", got)

    # finding 6
    def test_fairness_is_not_fair_prep(self):
        self.mutate("docs/SUBMISSION-DRAFT.md", "`fair`", "fairness", 1)
        self.mutate("pipeline/RESULTS.md", "`fair` preparation", "fairness preparation", 1)
        self.assertIn("accuracy-claim", self.rules())

    def test_far_negation_does_not_suppress_overclaim(self):
        self.append("docs/BRIEF.md", "We do not hide limits, but the reports are clinically validated.")
        self.assertIn("overclaim", self.rules())

    def test_kappa_one_detected(self):
        self.append("docs/BRIEF.md", "Agreement was kappa 1.0 on all fields.")
        self.assertIn("kappa-wording", self.rules())

    def test_process_report_not_tracked(self):
        import subprocess
        out = subprocess.run(["git", "ls-files", ".superpowers"], cwd=REAL, capture_output=True, text=True).stdout
        self.assertEqual("", out.strip())


if __name__ == "__main__":
    unittest.main()
