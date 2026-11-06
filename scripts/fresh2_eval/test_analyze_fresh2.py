"""Tests for the committed labeller input and the post-score analysis.
Run: python3 -m unittest discover -s scripts/fresh2_eval -p 'test_*.py'"""
import hashlib, json, os, unittest

import analyze_fresh2 as A
import fresh2 as F

REPO = A.REPO


def rj(rel):
    with open(os.path.join(REPO, rel), encoding="utf-8") as f:
        return json.load(f)


def rb(rel):
    with open(os.path.join(REPO, rel), "rb") as f:
        return f.read()


class LabellerInput(unittest.TestCase):
    def setUp(self):
        self.inp = rj("data/fresh2/labeller-input.json")
        self.man = rj("data/fresh2/LABELLER-INPUT-MANIFEST.json")
        self.corpus = rj("data/fresh2/corpus.json")

    def test_only_allowed_fields_and_no_metadata(self):
        self.assertEqual(set(self.inp), set(self.man["allowed_top_level_fields"]))
        for banned in ("item_metadata", "provenance", "key", "labels", "tags"):
            self.assertNotIn(banned, self.inp)
        allowed = set(self.man["allowed_item_fields"])
        self.assertEqual(allowed, F.ITEM_FIELDS)
        self.assertEqual(len(self.inp["items"]), 120)
        for it in self.inp["items"]:
            self.assertLessEqual(set(it), allowed, it["id"])
            self.assertTrue({"id", "programme", "kind", "text"} <= set(it))
            self.assertEqual("coded" in it, it["kind"] == "ifhas-screen", it["id"])
        blob = json.dumps(self.inp)
        for needle in ("author_role", "facility_name", "registered_patient", "document_role", "emirates_id"):
            self.assertNotIn(needle, blob)

    def test_items_equal_canonical_corpus_items(self):
        self.assertEqual(self.inp["items"], self.corpus["items"])
        self.assertEqual(self.inp["schema"], self.corpus["schema"])
        self.assertEqual(F.canon_sha(self.corpus), self.man["canonical_corpus_sha256"])

    def test_manifest_hashes(self):
        self.assertEqual(hashlib.sha256(rb("data/fresh2/labeller-input.json")).hexdigest(), self.man["labeller_input_file_sha256"])
        self.assertEqual(F.guide_sha256(), self.man["guide_sha256"])
        self.assertTrue(self.man["declaration_exact_input_supplied"])

    def test_prompt_hashes_reproduce_and_match_label_files(self):
        guide = open(F.GUIDE_PATH, encoding="utf-8").read()
        by_id = {x["id"]: x for x in self.man["labellers"]}
        a, b = by_id["gemini-a"], by_id["kimi-b"]
        pa = (guide + a["output_instruction_suffix"]).encode()
        pb = (b["prefix"] + guide + b["output_instruction_suffix"]).encode()
        for lab, prompt in ((a, pa), (b, pb)):
            self.assertEqual(hashlib.sha256(prompt).hexdigest(), lab["prompt_sha256"])
            lf = rj(lab["label_file"])
            self.assertEqual(lf["labeller"]["prompt_sha256"], lab["prompt_sha256"])
            self.assertEqual(lf["labeller"]["model"], lab["model"])
            self.assertEqual(lf["labeller"]["model_family"], lab["model_family"])
        self.assertNotEqual(a["model_family"], b["model_family"])


class Analysis(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.a = A.build()

    def test_directions(self):
        w = {x["id"]: x for x in self.a["agreed_but_wrong"]["items"]}
        self.assertEqual(sorted(w), ["f2i-031", "f2l-002", "f2l-021", "f2l-029"])
        self.assertEqual(w["f2i-031"]["direction"], "closing")
        self.assertEqual((w["f2i-031"]["key"], w["f2i-031"]["reader_a"], w["f2i-031"]["reader_b"]), ("bp=pending", "bp=managed", "bp=managed"))
        for i in ("f2l-002", "f2l-021", "f2l-029"):
            self.assertEqual(w[i]["direction"], "holding")
        self.assertEqual((self.a["agreed_but_wrong"]["closing"], self.a["agreed_but_wrong"]["holding"]), (1, 3))

    def test_direction_function(self):
        self.assertEqual(A.direction("ifhas-followup", "bp=pending", "bp=managed"), "closing")
        self.assertEqual(A.direction("ifhas-followup", "bp=managed", "bp=pending"), "holding")
        self.assertEqual(A.direction("ifhas-followup", "indeterminate", "bp=not_confirmed"), "closing")
        self.assertEqual(A.direction("ldct-workup", "wk_incomplete", "wk_benign"), "closing")
        self.assertEqual(A.direction("ldct-workup", "wk_surveillance", "wk_incomplete"), "holding")

    def test_screening_reported_separately(self):
        s = self.a["ifhas_screening_coded_not_gemma"]
        self.assertIn("NOT a Gemma", s["reader_b_is"])
        self.assertEqual((s["reader_a_correct"], s["reader_b_coded_matches_key"], s["agreed_n"]), (6, 28, 28))
        self.assertEqual(s["panels_mentioning_fasting"], 27)
        self.assertEqual(s["panels_total"], 30)
        g = self.a["gemma_reader_kinds_agreed_subset"]
        self.assertNotIn("ifhas-screen", g["by_kind"])
        self.assertEqual(g["totals"]["n"], 85)
        self.assertEqual(g["totals"]["reader_b_gemma_correct"], 12 + 24 + 17)

    def test_unavailable_reasons_exact(self):
        u = self.a["reader_b_unavailable"]
        self.assertEqual(u["by_kind_total"], {"ifhas-followup": 11, "ifhas-screen": 2})
        f = u["ifhas_followup"]
        self.assertEqual((f["unavailable_total"], f["cache_miss"], f["cache_error"]), (11, 0, 0))
        self.assertEqual(f["main_quote_not_grounded"], 11)
        self.assertEqual(f["main_quote_not_grounded_only"], 9)
        self.assertEqual(f["main_and_finding_quote_not_grounded"], 2)
        self.assertEqual(u["ldct"], 0)
        self.assertIn("not a correct answer", u["unavailable_is_not_correct"])
        raw_unavailable = {r["id"] for r in rj("data/fresh2/results/reader_raw.json")["items"] if not r["b_available"]}
        self.assertEqual({x["id"] for x in u["rows"]}, raw_unavailable)

    def test_original_scores_recomputed(self):
        self.assertTrue(self.a["verification"]["original_scores"]["score_json_matches_recomputation"])

    def test_manifest_rederived_and_protected_unchanged(self):
        v = self.a["verification"]
        self.assertTrue(v["manifest"]["equal"])
        self.assertEqual(v["manifest"]["check_freeze_after_score_errors"], [])
        self.assertEqual(v["protected_files"]["changed"], [])

    def test_committed_files_match_regeneration(self):
        self.assertEqual(open(os.path.join(REPO, A.OUT_JSON), encoding="utf-8").read(), F.dumps(self.a))
        self.assertEqual(open(os.path.join(REPO, A.OUT_MD), encoding="utf-8").read(), A.render(self.a) + "\n")

    def test_grounding_rule_matches_go(self):
        self.assertTrue(A.grounded("HbA1c was 6.2%", "Today the hba1c was 6.2 % ok"))
        self.assertFalse(A.grounded("", "text"))
        self.assertFalse(A.grounded("advised a dietitian", "no such words"))


if __name__ == "__main__":
    unittest.main()
