"""Fresh4 amendment 2 tests: ownership-aware I1 gate. Synthetic aggregate fixtures only; no run."""
import copy
import json
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import amendment2 as A  # noqa: E402
import fresh4 as X  # noqa: E402
import test_fresh4 as T  # noqa: E402

V2 = "outcome_or_evidence_ownership_v2"


def summary(v2=True, unsafe=(0, 0)):
    s = T.IntegratedGates.summary(None)
    for noise in ("v1", "v2"):
        block = s["by_noise"][noise]
        if v2:
            block["closure_loss"] = V2
            block["safety_pooled"]["crc_unsafe_closures"] = [unsafe[0], 100, None]
            block["safety_pooled"]["cervical_unsafe_closures"] = [unsafe[1], 100, None]
        else:
            block["closure_loss"] = "outcome_only_v1"
    return s


def gate(gates, gid):
    return next(g for g in gates if g["id"] == gid)


class AmendedI1(unittest.TestCase):
    def test_original_gates_are_unchanged_and_amended_gate_follows_each_i1(self):
        s = summary()
        original = X.integrated_gates(copy.deepcopy(s))
        gates = A.amended_integrated_gates(s)
        stripped = [{k: v for k, v in g.items() if k != "protocol_version"} for g in gates
                    if g["protocol_version"] == "fresh4-original"]
        self.assertEqual(stripped, original)
        ids = [g["id"] for g in gates]
        for noise in ("v1", "v2"):
            self.assertEqual(ids.index(A.AMENDED_I1 % noise), ids.index(A.ORIGINAL_I1 % noise) + 1)
        self.assertTrue(A.passed(gates), gates)
        self.assertEqual(X.FALSE_CLOSURE_METRICS[:2], ["crc_false_closures", "cervical_false_closures"])

    def test_wrong_patient_evidence_fails_amended_gate_while_original_still_passes(self):
        # #193: a truth-closed loop closed on another patient's report has zero false_closures but one unsafe closure.
        gates = A.amended_integrated_gates(summary(unsafe=(1, 0)))
        self.assertEqual(gate(gates, A.ORIGINAL_I1 % "v1")["status"], "pass")
        g = gate(gates, A.AMENDED_I1 % "v1")
        self.assertEqual(g["status"], "fail")
        self.assertIn({"metric": "crc_unsafe_closures", "numerator": 1, "denominator": 100}, g["failures"])
        self.assertEqual(g["protocol_version"], "fresh4-amendment-2")
        self.assertFalse(A.passed(gates))

    def test_cervical_unsafe_closure_fails(self):
        g = gate(A.amended_integrated_gates(summary(unsafe=(0, 2))), A.AMENDED_I1 % "v2")
        self.assertEqual(g["status"], "fail")
        self.assertEqual(g["numerator"], 2)

    def test_outcome_only_aggregate_is_not_evaluable_never_pass(self):
        gates = A.amended_integrated_gates(summary(v2=False))
        g = gate(gates, A.AMENDED_I1 % "v1")
        self.assertEqual(g["status"], "not_evaluable")
        self.assertIn("outcome_only_v1", g["reason"])
        self.assertEqual(gate(gates, A.ORIGINAL_I1 % "v1")["status"], "pass")
        self.assertFalse(A.passed(gates))

    def test_outcome_only_aggregate_with_measured_false_closure_still_fails(self):
        s = summary(v2=False)
        s["by_noise"]["v1"]["safety_pooled"]["ldct_false_closures"] = [1, 50, None]
        self.assertEqual(gate(A.amended_integrated_gates(s), A.AMENDED_I1 % "v1")["status"], "fail")

    def test_v2_aggregate_missing_unsafe_counts_fails(self):
        s = summary()
        del s["by_noise"]["v2"]["safety_pooled"]["cervical_unsafe_closures"]
        g = gate(A.amended_integrated_gates(s), A.AMENDED_I1 % "v2")
        self.assertEqual(g["status"], "fail")
        self.assertIn("missing", [f.get("error") for f in g["failures"]])

    def test_no_summary_is_not_run(self):
        gates = A.amended_integrated_gates(None)
        self.assertEqual({g["status"] for g in gates}, {"not_run"})

    def test_cli_writes_versioned_gate_file_and_refuses_overwrite(self):
        with tempfile.TemporaryDirectory() as d:
            src, out = os.path.join(d, "summary.json"), os.path.join(d, "gates.json")
            json.dump(summary(), open(src, "w"))
            cmd = [sys.executable, os.path.join(HERE, "amendment2.py"), src, "--out", out]
            self.assertEqual(subprocess.run(cmd, capture_output=True).returncode, 0)
            doc = json.load(open(out))
            self.assertEqual(doc["schema"], A.GATE_SCHEMA)
            self.assertEqual(doc["amendment"]["approved_on"], "2026-11-17")
            self.assertNotEqual(subprocess.run(cmd, capture_output=True).returncode, 0)


if __name__ == "__main__":
    unittest.main()
