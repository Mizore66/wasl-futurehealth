"""Fresh4 harness tests. Fixture texts only; no corpus, reader, model or integrated run."""
import copy
import os
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "fresh2_eval"))
import fresh2 as F  # noqa: E402
import fresh4 as X  # noqa: E402


def item(iid, kind, text="Synthetic fixture text."):
    programme = "ldct" if kind.startswith("ldct") else "ifhas"
    ans = {"id": iid, "kind": kind, "programme": programme, "text": text}
    if kind == "ifhas-screen":
        ans["coded"] = "8480-6|130|mm[Hg];8462-4|80|mm[Hg]"
    return ans


def corpus():
    items = []
    for prefix, kind in (("f4l-", "ldct-screen"), ("f4l-", "ldct-workup"),
                         ("f4i-", "ifhas-followup"), ("f4i-", "ifhas-screen")):
        start = 1 if kind in ("ldct-screen", "ifhas-followup") else 26
        for n in range(start, start + 25):
            items.append(item(f"{prefix}{n:03d}", kind, f"Unique synthetic {prefix}{n:03d}."))
    return {"schema": F.CORPUS_SCHEMA,
            "provenance": {"authoring": "model", "author_id": "fixture-author",
                           "author_saw_reader_code": False, "created": "2026-11-09"},
            "items": items}


def labels(c, lid):
    rows = []
    for it in c["items"]:
        label = {"ldct-screen": "LR2", "ldct-workup": "wk_incomplete",
                 "ifhas-followup": "bp=pending", "ifhas-screen": "bp=N;glyc=X;lipid=X;weight=X"}[it["kind"]]
        row = {"id": it["id"], "label": label}
        if it["kind"] == "ifhas-screen":
            row["coded_label"] = "bp=N;glyc=X;lipid=X;weight=X"
        rows.append(row)
    return {"schema": F.LABELS_SCHEMA, "corpus_sha256": F.canon_sha(c),
            "labeller": {"id": lid, "kind": "model", "model": "fixture", "model_family": lid,
                         "prompt_sha256": "x", "run_date": "2026-11-09", "guide_sha256": F.guide_sha256(),
                         "session_isolated": True, "saw_reader_implementation": False,
                         "saw_other_labels": False, "clinician_verified": False}, "labels": rows}


class CorpusAndBlindInput(unittest.TestCase):
    def test_exact_coverage_and_novel_text_pass(self):
        with X.fresh4_environment():
            self.assertEqual(X.validate_fresh4_corpus(corpus(), set()), [])

    def test_missing_adversarial_block_is_rejected(self):
        c = corpus()
        c["items"] = c["items"][:-1]
        with X.fresh4_environment():
            errs = X.validate_fresh4_corpus(c, set())
        self.assertTrue(any("f4i-050" in e for e in errs))

    def test_blind_input_excludes_all_provenance(self):
        blind = X.make_blind_input(corpus())
        self.assertNotIn("provenance", blind)
        self.assertNotIn("key", blind)
        self.assertEqual(len(blind["items"]), 100)
        self.assertEqual(set(blind["items"][0]), {"id", "kind", "programme", "text"})


class Adjudication(unittest.TestCase):
    def setUp(self):
        self.c = corpus()
        self.a = labels(self.c, "fixture-a")
        self.b = labels(self.c, "fixture-b")
        self.b["labels"][0]["label"] = "LR3"
        ab, bb = F.dumps(self.a).encode(), F.dumps(self.b).encode()
        with X.fresh4_environment():
            self.key, _ = F.build_key(self.c, self.a, self.b, ab, bb)
        self.adj = {"schema": X.ADJUDICATION_SCHEMA, "corpus_sha256": F.canon_sha(self.c),
                    "adjudicator": {"id": "fixture-adjudicator", "session_isolated": True,
                                    "saw_reader_outputs": False, "saw_scores": False},
                    "items": [{"id": "f4l-001", "resolution": "adjudicated", "key": {"label": "LR2"},
                               "rationale": "The fixture text supports the guide category.",
                               "uncertainty": "low"}]}

    def test_disagreement_input_has_only_disagreements_and_both_labels(self):
        d = X.make_disagreement_input(self.c, self.key)
        self.assertEqual(len(d["items"]), 1)
        self.assertEqual(d["items"][0]["id"], "f4l-001")
        self.assertEqual(d["items"][0]["labels"]["fixture-a"], {"label": "LR2"})
        self.assertNotIn("reader", d)

    def test_apply_valid_adjudication_preserves_provenance(self):
        out = X.apply_adjudication(self.key, self.adj)
        row = out["items"][0]
        self.assertEqual(row["status"], "adjudicated")
        self.assertEqual(row["adjudication"]["uncertainty"], "low")
        self.assertEqual(out["counts"]["adjudicated"], 1)
        self.assertEqual(out["counts"]["scoreable"], 100)

    def test_adjudicator_cannot_change_an_agreed_item(self):
        self.adj["items"][0]["id"] = "f4l-002"
        with self.assertRaises(F.HarnessError):
            X.apply_adjudication(self.key, self.adj)

    def test_unresolved_requires_no_key_and_stays_unscored(self):
        row = self.adj["items"][0]
        row.update(resolution="unresolved", key={"label": "LR2"}, uncertainty="high")
        with self.assertRaises(F.HarnessError):
            X.apply_adjudication(self.key, self.adj)
        row["key"] = None
        out = X.apply_adjudication(self.key, self.adj)
        self.assertEqual(out["items"][0]["status"], "unadjudicated")
        self.assertEqual(out["counts"]["scoreable"], 99)


class ScoringAndGates(unittest.TestCase):
    def raw(self, c):
        rows = []
        for it in c["items"]:
            label = {"ldct-screen": "LR2", "ldct-workup": "wk_incomplete",
                     "ifhas-followup": "bp=pending", "ifhas-screen": "bp=N;glyc=X;lipid=X;weight=X"}[it["kind"]]
            rows.append({"id": it["id"], "kind": it["kind"], "a": label, "b": label,
                         "b_available": True, "agree": True, "label": label})
        return {"schema": F.RAW_SCHEMA, "items": rows, "models": {}}

    def test_scores_keep_agreed_and_adjudicated_denominators_separate(self):
        c = corpus()
        a, b = labels(c, "fixture-a"), labels(c, "fixture-b")
        b["labels"][0]["label"] = "LR3"
        with X.fresh4_environment():
            key, _ = F.build_key(c, a, b, F.dumps(a).encode(), F.dumps(b).encode())
        adj = {"schema": X.ADJUDICATION_SCHEMA, "corpus_sha256": F.canon_sha(c),
               "adjudicator": {"id": "third", "session_isolated": True, "saw_reader_outputs": False, "saw_scores": False},
               "items": [{"id": "f4l-001", "resolution": "adjudicated", "key": {"label": "LR2"},
                          "rationale": "Guide-based fixture resolution.", "uncertainty": "low"}]}
        key = X.apply_adjudication(key, adj)
        score = X.compute_fresh4_scores(key, self.raw(c))
        self.assertEqual(score["by_key_origin"]["agreed"]["agreed"]["totals"]["n"], 99)
        self.assertEqual(score["by_key_origin"]["adjudicated"]["agreed"]["totals"]["n"], 1)
        self.assertEqual(score["pooled"]["agreed"]["totals"]["n"], 100)
        self.assertEqual(score["pooled"]["agreed"]["totals"]["agreed_but_wrong"], 25)

    def test_closing_direction_is_safety_failure(self):
        self.assertEqual(X.direction("ldct-workup", "wk_incomplete", "wk_treatment"), "closing")
        self.assertEqual(X.direction("ifhas-followup", "bp=pending", "bp=managed"), "closing")
        self.assertEqual(X.direction("ifhas-followup", "bp=managed", "bp=pending"), "holding")

    def test_reader_gate_reports_exact_closing_denominator(self):
        score = {"pooled": {"agreed": {"totals": {"n": 100}}}, "raw_items": 100,
                 "wrong_direction": {"closing": 1, "holding": 2, "items": []}}
        gates = X.reader_gates(score, expected_items=100)
        closing = next(g for g in gates if g["id"] == "R2_zero_closing_direction")
        self.assertEqual((closing["status"], closing["numerator"], closing["denominator"]), ("fail", 1, 100))

    def test_missing_reader_output_is_not_run_not_zero(self):
        gates = X.reader_gates(None, expected_items=100)
        self.assertTrue(all(g["status"] == "not_run" for g in gates))


class IntegratedGates(unittest.TestCase):
    def summary(self):
        safety = {
            "crc_false_closures": [0, 100, None], "cervical_false_closures": [0, 100, None],
            "lung_false_closures": [0, 100, None], "dr_false_closures": [0, 100, None],
            "dr_referral_false_closures": [0, 100, None], "breast_false_closures": [0, 100, None],
            "breast_recall_false_closures": [0, 100, None], "ldct_false_closures": [0, 100, None],
            "ldct_action_false_closures": [0, 100, None], "ifhas_false_closures": [0, 100, None],
            "ifhas_followup_false_closures": [0, 100, None], "wrong_patient_released": [0, 100, None],
            "released_without_granted_consent": [0, 100, None], "duplicate_or_lost_outputs": [0, 100, None],
            "ingest_lost_or_duplicated": [0, 100, None], "final_state_mismatch_loops": [0, None, None],
            "forged_with_effect": [0, 95, None], "tamper_detected": [85, 85, None],
            "tamper_false_alarms": [0, 5, None],
        }
        return {"manifest": {"seeds": X.INTEGRATED_SEEDS, "selected_noise_models": ["v1", "v2"]},
                "by_noise": {"v1": {"seeds": 5, "safety": copy.deepcopy(safety)},
                             "v2": {"seeds": 5, "safety": copy.deepcopy(safety)}}}

    def test_integrated_gates_pass_complete_zero_event_evidence(self):
        gates = X.integrated_gates(self.summary())
        self.assertTrue(all(g["status"] == "pass" for g in gates), gates)

    def test_integrated_gate_failure_preserves_count_and_denominator(self):
        s = self.summary()
        s["by_noise"]["v2"]["safety"]["ldct_false_closures"] = [2, 101, None]
        gates = X.integrated_gates(s)
        g = next(g for g in gates if g["id"] == "I1_v2_zero_listed_false_closures")
        self.assertEqual(g["status"], "fail")
        self.assertIn({"metric": "ldct_false_closures", "numerator": 2, "denominator": 101}, g["failures"])

    def test_missing_lane_is_not_silently_passed(self):
        s = self.summary()
        s["by_noise"]["v2"]["seeds"] = 4
        gates = X.integrated_gates(s)
        complete = next(g for g in gates if g["id"] == "I0_all_10_lanes_complete")
        self.assertEqual(complete["status"], "fail")
        self.assertEqual(complete["observed_lanes"], 9)


if __name__ == "__main__":
    unittest.main()
