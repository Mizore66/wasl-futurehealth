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


if __name__ == "__main__":
    unittest.main()
