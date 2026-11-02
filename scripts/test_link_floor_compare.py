import copy
import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import link_floor_compare as lfc

HERE = os.path.dirname(os.path.abspath(__file__))
SCRIPT = os.path.join(HERE, "link_floor_experiment.sh")


def _rb(p):
    with open(p, "rb") as h:
        return h.read()


def _wj(p, o):
    with open(p, "w") as h:
        json.dump(o, h)


def arm(floor, tp, truth, **kw):
    a = {"floor": floor, "tp": tp, "truth_closed": truth, "false_closures": 0, "wrong_released_total": 0,
         "wrong_released_by_origin": {"a": 0, "b": 0}, "replay_mismatch": 0, "analysis_errors": 0,
         "trace_error": 0, "false_merge_pairs": 0, "impure_clusters": 0, "cross_person_edges": 0}
    a.update(kw)
    return a


def report(tp=5, truth=10, **kw):
    r = {"programmes_live": {"crc": {"tp": tp, "truth_closed": truth, "false_closures": 0}},
         "outreach_live": {"wrong_patient_released": 0, "by_origin": {"a": {"wrong_released": 0}}},
         "live_vs_replay_final_mismatch": 0,
         "linkage": {"false_merge_pairs": 1, "impure_clusters": 2, "cross_person_edges_by_basis": {"eid": 1, "human": 2}},
         "reviewer": {"by_kind": {"all": {"items": 7, "sim_hours": 1.5}},
                      "by_programme": {"crc": {"items": 7, "sim_hours": 1.5}},
                      "by_origin": {"a": {"items": 7, "sim_hours": 1.5}}}}
    r.update(kw)
    return r


FUNNEL = {"truth_closed": 10, "analysis_errors": 0, "categories": {"attached_or_closed": 9}}
LOAD = {"items": 7, "distinct_pairs": 6, "unanswered": 1}


class Analyzer(unittest.TestCase):
    def test_pooled_recall_arithmetic(self):
        l1 = lfc.lane_metrics(1, report(tp=5, truth=10), FUNNEL, {}, LOAD)
        l2 = lfc.lane_metrics(2, report(tp=1, truth=90), dict(FUNNEL, truth_closed=90), {}, LOAD)
        a = lfc.aggregate_arm(0.2, [l1, l2])
        self.assertEqual((a["tp"], a["truth_closed"]), (6, 100))
        self.assertEqual(a["recall"], 0.06)
        self.assertEqual(a["cross_person_edges"], 6)
        self.assertEqual(a["false_merge_pairs"], 2)
        self.assertEqual(a["reviewer"]["items"], 14)
        self.assertEqual(a["reviewer"]["distinct_pairs"], 12)
        self.assertEqual(a["per_seed"]["1"]["tp"], 5)

    def test_missing_required_field_fails_loudly(self):
        r = report()
        del r["reviewer"]["by_kind"]["all"]["sim_hours"]
        with self.assertRaises(KeyError):
            lfc.lane_metrics(1, r, FUNNEL, {}, LOAD)
        with self.assertRaises(KeyError):
            lfc.lane_metrics(1, report(), FUNNEL, {}, {"items": 1})

    def test_truth_mismatch_with_funnel_fails(self):
        with self.assertRaises(ValueError):
            lfc.lane_metrics(1, report(truth=10), dict(FUNNEL, truth_closed=11), {}, LOAD)

    def test_funnel_floor_required_and_exact(self):
        lfc.check_funnel_floor({"config": {"review_floor": 0.05}}, "0.05")
        for f in ({}, {"config": None}, {"config": {}}, {"config": {"review_floor": 0.2}},
                  {"config": {"floor": 0.05}}, {"config": {"review_floor": "0.05"}}):
            with self.assertRaises(ValueError):
                lfc.check_funnel_floor(f, "0.05")

    def test_trace_error_recorded(self):
        m = lfc.lane_metrics(1, report(), FUNNEL, {"link_funnel_error": "x"}, LOAD)
        self.assertEqual(m["trace_error"], 1)

    def test_exact_one_pp_passes(self):
        r = lfc.select_arm(arm(0.2, 500, 600), [arm(0.05, 506, 600)])
        self.assertEqual(r["selected_floor"], 0.05)

    def test_below_gate_fails(self):
        r = lfc.select_arm(arm(0.2, 500, 600), [arm(0.05, 505, 600), arm(0.01, 505, 600)])
        self.assertIsNone(r["selected_floor"])
        self.assertTrue(r["arms"]["0.05"]["reasons"])

    def test_selects_conservative_when_both_pass(self):
        r = lfc.select_arm(arm(0.2, 500, 600), [arm(0.01, 510, 600), arm(0.05, 507, 600)])
        self.assertEqual(r["selected_floor"], 0.05)

    def test_selects_primary_when_only_primary_passes(self):
        r = lfc.select_arm(arm(0.2, 500, 600), [arm(0.05, 503, 600), arm(0.01, 510, 600)])
        self.assertEqual(r["selected_floor"], 0.01)

    def test_each_rejection_field(self):
        base = arm(0.2, 500, 600, cross_person_edges=2)
        for k, v in [("false_closures", 1), ("wrong_released_total", 1), ("wrong_released_by_origin", {"a": 1, "b": 0}),
                     ("replay_mismatch", 1), ("analysis_errors", 1), ("trace_error", 1)]:
            c = arm(0.01, 520, 600, cross_person_edges=2, **{k: v})
            self.assertIsNone(lfc.select_arm(base, [c])["selected_floor"], k)

    def test_contamination_increase_rejects(self):
        b = arm(0.2, 500, 600, false_merge_pairs=1, impure_clusters=1, cross_person_edges=2)
        for k in ("false_merge_pairs", "impure_clusters", "cross_person_edges"):
            c = arm(0.01, 510, 600, false_merge_pairs=1, impure_clusters=1, cross_person_edges=2)
            c[k] += 1
            self.assertIsNone(lfc.select_arm(b, [c])["selected_floor"], k)
        c = arm(0.01, 510, 600, false_merge_pairs=0, impure_clusters=1, cross_person_edges=2)
        self.assertEqual(lfc.select_arm(b, [c])["selected_floor"], 0.01)

    def test_baseline_unsafe_or_truth_mismatch_selects_none(self):
        self.assertIsNone(lfc.select_arm(arm(0.2, 500, 600, false_closures=1), [arm(0.01, 520, 600)])["selected_floor"])
        self.assertIsNone(lfc.select_arm(arm(0.2, 500, 600), [arm(0.01, 520, 601)])["selected_floor"])

    def test_stress_and_validation_gates(self):
        b = arm(0.2, 500, 600)
        self.assertTrue(lfc.gate(b, arm(0.05, 500, 600), "stress")["pass"])
        self.assertFalse(lfc.gate(b, arm(0.05, 500, 600, false_closures=1), "stress")["pass"])
        self.assertFalse(lfc.gate(b, arm(0.05, 500, 600), "validation")["pass"])
        self.assertTrue(lfc.gate(b, arm(0.05, 501, 600), "validation")["pass"])

    def test_t_interval(self):
        m, h = lfc.t_interval([1.0, 2.0, 3.0])
        self.assertEqual(m, 2.0)
        self.assertAlmostEqual(h, 4.303 * 1.0 / 3 ** 0.5)
        m, h = lfc.t_interval([1, 2, 3, 4, 5])
        self.assertAlmostEqual(h, 2.776 * (2.5 ** 0.5) / 5 ** 0.5)
        with self.assertRaises(ValueError):
            lfc.t_interval([1, 2])


class Harness(unittest.TestCase):
    def run_sh(self, home, *args):
        env = dict(os.environ, HOME=home)
        return subprocess.run(["bash", SCRIPT] + list(args), env=env, capture_output=True, text=True, timeout=60)

    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.home = os.path.realpath(self.td.name)
        os.makedirs(os.path.join(self.home, "wasl-ml-data", "exp"))
        self.root = os.path.join(self.home, "wasl-ml-data", "exp", "arm")

    def tearDown(self):
        self.td.cleanup()

    def test_invalid_args_rejected_without_launch(self):
        for args in [(), (self.root,), (self.root, "dev-clean"), (self.root, "bogus", "0.2"),
                     (self.root, "dev-clean", "0.1"), (self.root, "dev-clean", "0.20"), (self.root, "dev-clean", "0"),
                     (self.root, "dev-clean", "0.2", "x"), ("", "dev-clean", "0.2"),
                     (self.root + "/../arm", "dev-clean", "0.2"), ("/tmp/outside", "dev-clean", "0.2")]:
            r = self.run_sh(self.home, *args)
            self.assertEqual(r.returncode, 2, (args, r.stderr))
        self.assertFalse(os.path.exists(self.root))

    def test_existing_root_rejected(self):
        os.mkdir(self.root)
        r = self.run_sh(self.home, self.root, "dev-clean", "0.2")
        self.assertEqual(r.returncode, 2)
        self.assertIn("already exists", r.stderr)

    def test_validation_requires_manifest(self):
        r = self.run_sh(self.home, self.root, "validation-clean", "0.2")
        self.assertEqual(r.returncode, 2)
        self.assertIn("validation-manifest.json", r.stderr)
        self.assertFalse(os.path.exists(self.root))

    def test_validation_manifest_mismatch_rejected(self):
        head = subprocess.run(["git", "rev-parse", "HEAD"], cwd=HERE, capture_output=True, text=True).stdout.strip()
        sha = hashlib.sha256(_rb(os.path.join(HERE, "link_floor_compare.py"))).hexdigest()
        good = {"git_head": head, "selected_floor": 0.05, "analyzer_sha256": sha, "seeds": [211, 223, 227, 229, 233]}
        mp = os.path.join(self.home, "wasl-ml-data", "exp", "validation-manifest.json")
        cases = [dict(good, git_head="0" * 40), dict(good, analyzer_sha256="0" * 64), dict(good, seeds=[211]),
                 dict(good, selected_floor=0.01)]
        for m in cases:
            _wj(mp, m)
            r = self.run_sh(self.home, self.root, "validation-clean", "0.05")
            self.assertEqual(r.returncode, 2, m)
            self.assertFalse(os.path.exists(self.root))
        # a floor that is neither baseline nor the selected arm is refused even with a good manifest
        _wj(mp, good)
        r = self.run_sh(self.home, self.root, "validation-clean", "0.01")
        self.assertEqual(r.returncode, 2)
        self.assertFalse(os.path.exists(self.root))

    def test_dev_stress_requires_selection(self):
        r = self.run_sh(self.home, self.root, "dev-stress", "0.2")
        self.assertEqual(r.returncode, 2)
        self.assertIn("selection.json missing", r.stderr)
        sp = os.path.join(self.home, "wasl-ml-data", "exp", "selection.json")
        _wj(sp, {"selected_floor": None})
        r = self.run_sh(self.home, self.root, "dev-stress", "0.2")
        self.assertEqual(r.returncode, 2)
        self.assertIn("must be 0.2 or the selected arm", r.stderr)
        _wj(sp, {"selected_floor": 0.05})
        r = self.run_sh(self.home, self.root, "dev-stress", "0.01")
        self.assertEqual(r.returncode, 2)
        self.assertIn("must be 0.2 or the selected arm", r.stderr)
        self.assertFalse(os.path.exists(self.root))

    def test_clean_tree_check_is_in_runner(self):
        with open(SCRIPT) as h:
            text = h.read()
        self.assertLess(text.index("status --porcelain --untracked-files=all"), text.index('"$ROOT/scripts/demo.sh" build'))

    def test_review_load(self):
        q = [{"item": "a", "kinds": ["link"], "pair": ["x", "y"]}, {"item": "b", "kinds": ["link"], "pair": ["y", "x"]},
             {"item": "c", "kinds": ["link"], "pair": ["p", "q"]}, {"item": "d", "kinds": ["link"], "pair": ["m", "n"], "reason": "second_identifier"},
             {"item": "e", "kinds": ["other"], "pair": ["u", "v"]}]
        self.assertEqual(lfc.review_load(q, [{"item": "a"}]), {"items": 3, "distinct_pairs": 2, "unanswered": 1})
        self.assertEqual(lfc.review_load(q, []), {"items": 3, "distinct_pairs": 2, "unanswered": 3})

    def test_cli_dev_clean_writes_selection(self):
        out = os.path.join(self.home, "wasl-ml-data", "exp")
        for fl, tp in (("0.2", 5), ("0.05", 6)):
            for sd in (101, 103, 107):
                d = os.path.join(out, "a" + fl, "v2-s%d" % sd)
                os.makedirs(os.path.join(d, "run/main"))
                _wj(d + "/funnel.json", dict(FUNNEL, config={"review_floor": float(fl)}))
                _wj(d + "/run/main/report7.json", report(tp=tp))
                _wj(d + "/run/main/review_queue.json", [])
                _wj(d + "/run/main/status-linker.json", {})
        args = ["dev-clean", out + "/summary-clean.json"] + ["%s=%s/a%s" % (f, out, f) for f in ("0.2", "0.05")]
        self.assertEqual(lfc.main(args), 0)
        with open(out + "/selection.json") as h:
            self.assertEqual(json.load(h)["selected_floor"], 0.05)

    def test_modes_table(self):
        self.assertEqual(lfc.MODES["dev-clean"], {"split": "dev", "seeds": [101, 103, 107], "rerr": "0", "errungraded": "0"})
        self.assertEqual(lfc.MODES["dev-stress"]["rerr"], "0.04")
        self.assertEqual(lfc.MODES["dev-stress"]["errungraded"], "0.25")
        self.assertEqual(lfc.MODES["validation-clean"]["seeds"], [211, 223, 227, 229, 233])
        self.assertEqual(lfc.MODES["validation-clean"]["split"], "eval")


if __name__ == "__main__":
    unittest.main()
