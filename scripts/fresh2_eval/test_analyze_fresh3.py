"""Tests for the fresh3 post-score analysis (reads the committed frozen data; no model call).
Run: python3 -m unittest discover -s scripts/fresh2_eval -p 'test_*.py'"""
import json, os, unittest

import analyze_fresh3 as A3

REPO = A3.REPO


def rj(rel):
    with open(os.path.join(REPO, rel), encoding="utf-8") as f:
        return json.load(f)


class Fresh3Analysis(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.a = A3.build(REPO)

    def test_counts_and_agreement(self):
        self.assertEqual(self.a["counts"], {"agreed": 93, "items": 100, "unadjudicated": 7})
        self.assertEqual(len(self.a["agreement"]["unadjudicated_ids"]), 7)

    def test_scores_match_stored_original(self):
        stored = rj("data/fresh3/results/score.json")
        self.assertEqual(self.a["scores_by_kind_agreed"], stored["agreed"]["by_kind"])
        self.assertEqual(self.a["scores_totals_agreed"], stored["agreed"]["totals"])
        self.assertTrue(self.a["verification"]["score_json_matches_recomputation"])
        # The history rewrite rebound the manifest and e698476 restored shared evidence plumbing; original scores and reader logic are unchanged.
        self.assertEqual(self.a["verification"]["changed_since_scoring_commit"], ["data/fresh3/FREEZE.json", "pipeline/evidence.go"])

    def test_manifest_rederived_and_scoring_protocol_unchanged(self):
        v = self.a["verification"]
        self.assertTrue(v["manifest_rederived_equal"])
        self.assertEqual(v["check_freeze_after_score_errors"], ["pipeline/evidence.go changed since reader_code_commit"])
        self.assertEqual(v["protected_changed_since_reader_code_commit"], [])

    def test_wrong_items_and_direction(self):
        w = self.a["agreed_but_wrong"]
        self.assertEqual([x["id"] for x in w["items"]], ["f3l-008", "f3l-019", "f3l-048"])
        self.assertEqual((w["closing"], w["holding"]), (0, 3))

    def test_unavailability_exact(self):
        u = self.a["reader_b_unavailable"]
        self.assertEqual([(r["id"], r["kind"]) for r in u["rows"]], [("f3i-003", "ifhas-followup"), ("f3i-013", "ifhas-screen")])
        self.assertEqual(u["ldct"], 0)
        cs = self.a["ifhas_coded_screening_unavailability"]
        self.assertEqual(cs["flag_unavailable"], ["f3i-013"])
        self.assertEqual(cs["placeholder_no_coded_results"], ["f3i-014", "f3i-032", "f3i-046"])

    def test_unavailable_key_items_have_exact_roles(self):
        # Three agreed items have key coded_label 'unavailable'. Only f3i-013 is a match, because B literally returned
        # 'unavailable'. f3i-032 and f3i-046 return an all-X label with b_available=true, so they are misses.
        raw = {r["id"]: r for r in rj("data/fresh3/results/reader_raw.json")["items"]}
        self.assertEqual(raw["f3i-013"]["b"], "unavailable")
        self.assertFalse(raw["f3i-013"]["b_available"])
        for i in ("f3i-032", "f3i-046"):
            self.assertNotEqual(raw[i]["b"], "unavailable")
            self.assertTrue(raw[i]["b_available"])
            self.assertEqual(raw[i]["b"], "bp=X;glyc=X;lipid=X;weight=X")
        note = self.a["ifhas_screening_coded_not_gemma"]["note_b"]
        self.assertIn("22/24; of the 3 items whose key coded_label is unavailable, only f3i-013 counts as a match "
                      "(B returned `unavailable`); f3i-032 and f3i-046 are misses.", note)
        self.assertIn("not a language-model reader", note)

    def test_comparison_headline_only(self):
        c = self.a["comparison_fresh2_to_fresh3"]
        self.assertTrue(c["headline_matches"])
        self.assertEqual(c["fresh3"], {"ifhas_followup_b_unavailable_agreed": 1, "ifhas_followup_agreed_but_wrong": 0, "ldct_workup_agreed_but_wrong": 3})

    def test_committed_files_up_to_date_and_caveats(self):
        self.assertEqual(A3.main(["--check"]), 0)
        with open(os.path.join(REPO, A3.OUT_MD), encoding="utf-8") as f:
            md = f.read()
        for needle in ("model-labelled and not clinician-verified", "not evidence of clinical effectiveness", "93/100"):
            self.assertIn(needle, md)
        self.assertNotIn("0 unsafe", md.replace("no `0 unsafe` claim", ""))


if __name__ == "__main__":
    unittest.main()
