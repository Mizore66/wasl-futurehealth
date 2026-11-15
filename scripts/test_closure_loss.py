import json
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import closure_loss  # noqa: E402

try:
    import scipy  # noqa: F401
except ImportError:
    scipy = None


def ltt_row(review, tau, unsafe, **extra):
    row = {"cohort": "c1", "with_review": review, "tau_close": tau, "loops": 5000, "closed": 2000, "tp": 2000,
           "false_closures": 0, "truth_closed": 2000, "recall": 1.0, "loops_with_wrong_patient_message": 0,
           "review_items": 10}
    if unsafe is not None:
        row["unsafe_closures"] = unsafe
    row.update(extra)
    return row


def report7(false_closures, unsafe):
    def live(prog):
        c = {"precision": 1.0, "recall": 1.0, "false_closures": false_closures, "closed_pred": 100}
        if unsafe is not None:
            c["unsafe_closures"] = unsafe[prog]
        return c
    kpi = {"kpi_all_referrals": 0.5}
    return {
        "closure": {p: {"wasl_live_with_review": live(p), "wasl_no_review": {"recall": 1.0}} for p in ("crc", "cervical")},
        "crc_kpi_31d": {"wasl_live": kpi, "truth": kpi, "eid_only": kpi},
        "linkage": {"pair_f1": 1.0},
        "reviewer": {"by_kind": {"all": {"reviewer_hours_per_1k_loops": 1.0, "items_per_100_loops": 1.0}},
                     "by_origin": {"o1": {"items_per_100_loops": 1.0, "reviewer_hours_per_1k_loops": 1.0, "items": 1, "loops": 100}}},
        "crash_recovery": {"duplicate_outputs": 0, "expected_outputs": 1, "ingest_lost": 0, "ingest_duplicates": 0,
                           "generated_resources": 1, "final_state_mismatch_loops": 0},
        "outreach_live": {"by_origin": {"o1": {"wrong_released": 0, "intents": 1}}},
        "consent_timing": {},
        "forged": {"forged_events": 0, "forged_with_any_effect": 0},
        "throughput_ev_s": 1.0,
    }


def run_final_aggregate(seeds):
    with tempfile.TemporaryDirectory() as d:
        json.dump({"code_revision": "0" * 40, "policy": "p", "seeds": list(seeds), "source": "s", "frozen_at": "t",
                   "noise_models": {"v1": "n"}, "limitations": []}, open(os.path.join(d, "manifest.json"), "w"))
        for seed, rep in seeds.items():
            os.makedirs(os.path.join(d, f"v1-s{seed}"))
            json.dump(rep, open(os.path.join(d, f"v1-s{seed}", "report7.json"), "w"))
            json.dump({"attacks": []}, open(os.path.join(d, f"v1-s{seed}", "tamper.json"), "w"))
        res = subprocess.run([sys.executable, os.path.join(os.path.dirname(os.path.abspath(__file__)), "final_aggregate.py"), d],
                             capture_output=True, text=True, env=dict(os.environ, PYTHONUTF8="1"))
        if res.returncode:
            return None, res.stderr
        return json.load(open(os.path.join(d, "summary.json")))["by_noise"]["v1"], res.stderr


class ClosureLossTests(unittest.TestCase):
    def test_v2_artifact_uses_ownership_aware_loss(self):
        self.assertEqual(closure_loss.pooled([{"false_closures": 0, "unsafe_closures": 2}, {"false_closures": 1, "unsafe_closures": 1}]),
                         (3, closure_loss.V2))

    def test_historical_artifact_is_labelled_outcome_only(self):
        self.assertEqual(closure_loss.pooled([{"false_closures": 1}]), (1, closure_loss.V1))

    def test_mixed_loss_definitions_are_refused(self):
        with self.assertRaises(ValueError):
            closure_loss.pooled([{"false_closures": 0, "unsafe_closures": 1}, {"false_closures": 0}])

    @unittest.skipIf(scipy is None, "scipy not installed")
    def test_ltt_select_certifies_on_unsafe_closures_not_outcome_only(self):
        with tempfile.TemporaryDirectory() as d:
            rows = [ltt_row(review, tau, unsafe) for review in (True, False) for tau, unsafe in ((0.999, 0), (0.995, 50))]
            json.dump({"closure_loss": closure_loss.V2, "rows": rows}, open(os.path.join(d, "counts_c1.json"), "w"))
            subprocess.run([sys.executable, os.path.join(os.path.dirname(os.path.abspath(__file__)), "ltt_select.py"), d,
                            "--alpha-fc", "0.002", "--min-tau", "0.99"], check=True, capture_output=True,
                            env=dict(os.environ, PYTHONUTF8="1"))
            out = json.load(open(os.path.join(d, "ltt_selection.json")))
        self.assertEqual(out["closure_loss"], closure_loss.V2)
        table = {x["tau_close"]: x for x in out["modes"]["with_review"]["table"]}
        self.assertEqual((table[0.995]["false_closures"], table[0.995]["unsafe_closures"]), (0, 50))
        self.assertTrue(table[0.999]["certified"])
        self.assertFalse(table[0.995]["certified"])
        self.assertEqual(out["modes"]["with_review"]["loosest_certified_tau_close"], 0.999)


    def test_final_aggregate_pools_unsafe_closures_for_v2_artifacts(self):
        out, err = run_final_aggregate({1: report7(0, {"crc": 2, "cervical": 0}), 2: report7(0, {"crc": 1, "cervical": 0})})
        self.assertIsNotNone(out, err)
        self.assertEqual(out["closure_loss"], closure_loss.V2)
        self.assertEqual(out["safety_pooled"]["crc_false_closures"][:2], [0, 200])
        self.assertEqual(out["safety_pooled"]["crc_unsafe_closures"][:2], [3, 200])
        self.assertEqual(out["safety_pooled"]["cervical_unsafe_closures"][:2], [0, 200])

    def test_final_aggregate_labels_v1_artifacts_without_unsafe_counts(self):
        out, err = run_final_aggregate({1: report7(1, None), 2: report7(0, None)})
        self.assertIsNotNone(out, err)
        self.assertEqual(out["closure_loss"], closure_loss.V1)
        self.assertNotIn("crc_unsafe_closures", out["safety_pooled"])
        self.assertNotIn("cervical_unsafe_closures", out["safety_pooled"])

    def test_final_aggregate_refuses_mixed_loss_seeds(self):
        out, err = run_final_aggregate({1: report7(0, {"crc": 1, "cervical": 0}), 2: report7(0, None)})
        self.assertIsNone(out)
        self.assertIn("closure-loss definitions differ", err)

if __name__ == "__main__":
    unittest.main()
