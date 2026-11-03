import copy
import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import pairfeat_compare as pfc

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
SCRIPT = os.path.join(HERE, "pairfeat_experiment.sh")
BASE_FILES = ("encoder_weights.json", "lgb_stream.json", "iso.json", "iso_v7.json")
CAND_FILES = ("encoder_pairfeat1.json", "lgb_stream_pairfeat1.json", "iso_pairfeat1.json", "iso_v7_pairfeat1.json")
CAND2_FILES = ("encoder_pairfeat2.json", "lgb_stream_pairfeat2.json", "iso_pairfeat2.json", "iso_v7_pairfeat2.json")
FILES = {"baseline": BASE_FILES, "pairfeat1": CAND_FILES, "pairfeat2": CAND2_FILES}
DEV_SEEDS = {"pairfeat1": [307, 311, 313], "pairfeat2": [317, 331, 337]}
P1 = "pairfeat1"  # the candidate of every pre-existing (pairfeat1) fixture
HEAD = "a" * 40
BIN_SHA = "b" * 64


def _rb(p):
    with open(p, "rb") as h:
        return h.read()


def _sha(p):
    return hashlib.sha256(_rb(p)).hexdigest()


def _wj(p, o):
    with open(p, "w") as h:
        json.dump(o, h)


def _rj(p):
    with open(p) as h:
        return json.load(h)


def arm(name, tp, truth, distinct_pairs=100, **kw):
    a = {"arm": name, "modelset": "" if name == "baseline" else name, "tp": tp, "truth_closed": truth,
         "false_closures": 0, "wrong_released_total": 0, "wrong_released_by_origin": {"a": 0, "b": 0},
         "replay_mismatch": 0, "analysis_errors": 0, "trace_error": 0,
         "false_merge_pairs": 0, "impure_clusters": 0, "cross_person_edges": 0,
         "reviewer": {"items": 10, "hours": 1.0, "distinct_pairs": distinct_pairs, "unanswered": 0, "link_items": 10,
                      "wrong_final_signoffs": 0}}
    a.update(kw)
    return a


def report(tp=5, truth=10, signoffs=0, **kw):
    r = {"programmes_live": {"crc": {"pred_closed": tp, "truth_closed": truth, "false_closures": 0,
                                     "recall": round(tp / truth, 3)}},
         "outreach_live": {"wrong_patient_released": 0, "by_origin": {"a": {"wrong_released": 0}}},
         "live_vs_replay_final_mismatch": 0,
         "linkage": {"false_merge_pairs": 1, "impure_clusters": 2, "cross_person_edges_by_basis": {"eid": 1, "human": 2}},
         "reviewer": {"by_kind": {"all": {"items": 7, "sim_hours": 1.5, "wrong_final_signoffs": signoffs}},
                      "by_programme": {"crc": {"items": 7, "sim_hours": 1.5}},
                      "by_origin": {"a": {"items": 7, "sim_hours": 1.5}}}}
    r.update(kw)
    return r


def funnel(tp=5, truth=10, attached=None, **cats):
    """Consistent funnel: attached_or_closed = tp + 1 (one attached-not-closed), remaining misses below review."""
    att = tp + 1 if attached is None else attached
    c = {"attached_or_closed": att}
    if cats:
        c.update(cats)
    else:
        c["candidate_below_review"] = truth - att
    return {"truth_closed": truth, "analysis_errors": 0, "categories": c}


LOAD = {"items": 7, "distinct_pairs": 6, "unanswered": 1}


def chain(files, tag):
    return {f: hashlib.sha256((tag + f).encode()).hexdigest() for f in files}


class Constants(unittest.TestCase):
    def test_frozen_constants(self):
        self.assertEqual(pfc.modes(P1), {
            "dev-clean": {"split": "dev", "seeds": [307, 311, 313], "rerr": "0", "errungraded": "0"},
            "dev-stress": {"split": "dev", "seeds": [307, 311, 313], "rerr": "0.04", "errungraded": "0.25"},
            "validation-clean": {"split": "eval", "seeds": [401, 409, 419, 421, 431], "rerr": "0", "errungraded": "0"},
        })
        self.assertEqual(pfc.FLOOR, "0.01")
        self.assertEqual(pfc.MIN_GAIN_PP, {"selection": 2.0, "validation": 1.0})
        self.assertEqual(pfc.MAX_REVIEW_GROWTH, 0.10)
        self.assertEqual(pfc.arms(P1), ("baseline", "pairfeat1"))
        self.assertEqual({a: pfc.CHAIN_FILES[a] for a in pfc.arms(P1)}, {"baseline": BASE_FILES, "pairfeat1": CAND_FILES})
        self.assertEqual(pfc.POLICY, "v7strict")


class Gates(unittest.TestCase):
    def test_frozen_gates_and_workload(self):
        base = arm("baseline", 500, 600, distinct_pairs=100)
        self.assertTrue(pfc.gate(base, arm("pairfeat1", 512, 600, distinct_pairs=110), "selection")["pass"])
        self.assertFalse(pfc.gate(base, arm("pairfeat1", 511, 600, distinct_pairs=110), "selection")["pass"])
        self.assertFalse(pfc.gate(base, arm("pairfeat1", 512, 600, distinct_pairs=111), "selection")["pass"])
        self.assertTrue(pfc.gate(base, arm("pairfeat1", 506, 600, distinct_pairs=105), "validation")["pass"])
        self.assertFalse(pfc.gate(base, arm("pairfeat1", 505, 600, distinct_pairs=105), "validation")["pass"])
        self.assertTrue(pfc.gate(base, arm("pairfeat1", 500, 600, distinct_pairs=105), "stress")["pass"])
        self.assertFalse(pfc.gate(base, arm("pairfeat1", 499, 600, distinct_pairs=105), "stress")["pass"])

    def test_exact_thresholds_reported(self):
        base = arm("baseline", 500, 600)
        g = pfc.gate(base, arm("pairfeat1", 512, 600), "selection")
        self.assertEqual((g["gain_pp"], g["min_gain_pp"]), (2.0, 2.0))
        self.assertEqual(pfc.gate(base, arm("pairfeat1", 506, 600), "validation")["min_gain_pp"], 1.0)
        self.assertEqual(pfc.gate(base, arm("pairfeat1", 500, 600), "stress")["min_gain_pp"], 0.0)
        with self.assertRaises(ValueError):
            pfc.gate(base, arm("pairfeat1", 512, 600), "bogus")

    def test_every_safety_field_in_both_arms(self):
        fields = [("false_closures", 1), ("wrong_released_total", 1), ("wrong_released_by_origin", {"a": 1, "b": 0}),
                  ("replay_mismatch", 1), ("analysis_errors", 1), ("trace_error", 1)]
        for kind in ("selection", "validation", "stress"):
            for k, v in fields:
                good_b, good_c = arm("baseline", 500, 600), arm("pairfeat1", 530, 600)
                self.assertTrue(pfc.gate(good_b, good_c, kind)["pass"])
                self.assertFalse(pfc.gate(good_b, arm("pairfeat1", 530, 600, **{k: v}), kind)["pass"], (kind, k, "cand"))
                g = pfc.gate(arm("baseline", 500, 600, **{k: v}), good_c, kind)
                self.assertFalse(g["pass"], (kind, k, "base"))
                self.assertTrue(any(r.startswith("baseline") for r in g["reasons"]), g["reasons"])

    def test_every_contamination_increase(self):
        for kind in ("selection", "validation", "stress"):
            b = arm("baseline", 500, 600, false_merge_pairs=1, impure_clusters=1, cross_person_edges=2)
            for k in pfc.CONTAM:
                c = arm("pairfeat1", 530, 600, false_merge_pairs=1, impure_clusters=1, cross_person_edges=2)
                c[k] += 1
                self.assertFalse(pfc.gate(b, c, kind)["pass"], (kind, k))
            c = arm("pairfeat1", 530, 600, false_merge_pairs=0, impure_clusters=1, cross_person_edges=2)
            self.assertTrue(pfc.gate(b, c, kind)["pass"], kind)

    def test_truth_denominator_mismatch(self):
        for kind in ("selection", "validation", "stress"):
            g = pfc.gate(arm("baseline", 500, 600), arm("pairfeat1", 530, 601), kind)
            self.assertFalse(g["pass"])
            self.assertIn("truth_closed differs between arms", g["reasons"])
        self.assertFalse(pfc.gate(arm("baseline", 0, 0), arm("pairfeat1", 0, 0), "stress")["pass"])

    def test_workload_growth_and_zero_baseline(self):
        for kind in ("selection", "validation", "stress"):
            b = arm("baseline", 500, 600, distinct_pairs=0)
            self.assertTrue(pfc.gate(b, arm("pairfeat1", 530, 600, distinct_pairs=0), kind)["pass"], kind)
            self.assertFalse(pfc.gate(b, arm("pairfeat1", 530, 600, distinct_pairs=1), kind)["pass"], kind)
            b = arm("baseline", 500, 600, distinct_pairs=10)
            self.assertTrue(pfc.gate(b, arm("pairfeat1", 530, 600, distinct_pairs=11), kind)["pass"], kind)
            self.assertFalse(pfc.gate(b, arm("pairfeat1", 530, 600, distinct_pairs=12), kind)["pass"], kind)
            # fewer distinct pairs is fine
            self.assertTrue(pfc.gate(b, arm("pairfeat1", 530, 600, distinct_pairs=3), kind)["pass"], kind)

    def test_mislabeled_aggregate_arms_fail(self):
        self.assertFalse(pfc.gate(arm("pairfeat1", 500, 600), arm("pairfeat1", 530, 600), "selection")["pass"])
        self.assertFalse(pfc.gate(arm("baseline", 500, 600), arm("baseline", 530, 600), "selection")["pass"])

    def test_selection_record(self):
        s = pfc.select(arm("baseline", 500, 600), arm("pairfeat1", 512, 600))
        self.assertEqual(s["selected_modelset"], "pairfeat1")
        self.assertTrue(s["pass"])
        s = pfc.select(arm("baseline", 500, 600), arm("pairfeat1", 511, 600))
        self.assertIsNone(s["selected_modelset"])
        self.assertFalse(s["pass"])


class Lanes(unittest.TestCase):
    def test_residual_partition(self):
        m = pfc.lane_metrics(1, report(tp=5, truth=10), funnel(5, 10), {}, LOAD)
        self.assertEqual(m["residual"], {"candidate_not_generated": 0, "candidate_below_review": 4,
                                         "candidate_rejected_demographic": 0, "candidate_routed_review": 0,
                                         "linked_report_not_attached": 0, "attached_not_closed": 1})
        self.assertEqual(m["categories"]["attached_or_closed"], 6)
        self.assertEqual(set(m["categories"]), set(pfc.FUNNEL_CATEGORIES))
        f = funnel(5, 10, attached=5, candidate_not_generated=1, candidate_below_review=1, candidate_rejected_demographic=1,
                   candidate_routed_review=1, linked_report_not_attached=1)
        m = pfc.lane_metrics(1, report(tp=5, truth=10), f, {}, LOAD)
        self.assertEqual(sum(m["residual"].values()), 5)
        self.assertEqual(m["residual"]["attached_not_closed"], 0)

    def test_residual_rejections(self):
        with self.assertRaises(ValueError):  # attached_or_closed < tp
            pfc.lane_metrics(1, report(tp=5, truth=10), funnel(5, 10, attached=4, candidate_below_review=6), {}, LOAD)
        with self.assertRaises(ValueError):  # categories do not partition misses
            pfc.lane_metrics(1, report(tp=5, truth=10), funnel(5, 10, attached=6, candidate_below_review=3), {}, LOAD)
        with self.assertRaises(ValueError):  # unknown category
            pfc.lane_metrics(1, report(tp=5, truth=10), funnel(5, 10, attached=6, candidate_below_review=3, mystery=1), {}, LOAD)

    def test_reported_outcomes(self):
        m = pfc.lane_metrics(1, report(tp=5, truth=10, signoffs=3), funnel(5, 10), {"link_funnel_error": "x"}, LOAD)
        self.assertEqual(m["reviewer"]["wrong_final_signoffs"], 3)
        self.assertEqual(m["trace_error"], 1)
        self.assertEqual(m["recall"], 0.5)
        self.assertEqual(m["cross_person_edges"], 3)
        r = report()
        del r["reviewer"]["by_kind"]["all"]["wrong_final_signoffs"]
        with self.assertRaises(KeyError):
            pfc.lane_metrics(1, r, funnel(5, 10), {}, LOAD)

    def test_crc_consistency(self):
        def bad(**kw):
            r = report(tp=7, truth=10)
            r["programmes_live"]["crc"].update(kw)
            with self.assertRaises(ValueError):
                pfc.lane_metrics(1, r, funnel(7, 10), {}, LOAD)
        bad(pred_closed=1, false_closures=2, recall=0.0)
        bad(pred_closed=11, false_closures=0, recall=1.0)
        bad(pred_closed=7, false_closures=0, recall=0.8)
        with self.assertRaises(ValueError):
            pfc.lane_metrics(1, report(truth=10), dict(funnel(5, 10), truth_closed=11), {}, LOAD)

    def test_aggregate_intervals(self):
        lanes = [pfc.lane_metrics(s, report(tp=tp, truth=10, signoffs=1), funnel(tp, 10), {}, LOAD)
                 for s, tp in ((307, 5), (311, 6), (313, 7))]
        a = pfc.aggregate_arm("baseline", lanes)
        self.assertEqual((a["tp"], a["truth_closed"]), (18, 30))
        self.assertEqual(a["recall"], 0.6)
        self.assertEqual(a["per_seed_recall"], {"307": 0.5, "311": 0.6, "313": 0.7})
        self.assertAlmostEqual(a["recall_t95"]["mean_pp"], 60.0)
        self.assertAlmostEqual(a["recall_t95"]["half_width_pp"], 4.303 * 10.0 / 3 ** 0.5)
        self.assertEqual(a["reviewer"]["wrong_final_signoffs"], 3)
        self.assertEqual(a["reviewer"]["distinct_pairs"], 18)
        self.assertEqual(a["residual"]["attached_not_closed"], 3)
        self.assertEqual(a["residual"]["candidate_below_review"], 9)
        self.assertEqual(a["categories"]["attached_or_closed"], 21)
        cl = [pfc.lane_metrics(s, report(tp=tp, truth=10), funnel(tp, 10), {}, LOAD)
              for s, tp in ((307, 6), (311, 8), (313, 7))]
        c = pfc.aggregate_arm("pairfeat1", cl)
        p = pfc.paired_gain_interval(a, c)
        self.assertEqual(p["per_seed_gain_pp"], {"307": 10.0, "311": 20.0, "313": 0.0})
        self.assertAlmostEqual(p["mean_pp"], 10.0)
        self.assertAlmostEqual(p["half_width_pp"], 4.303 * 10.0 / 3 ** 0.5)
        c2 = copy.deepcopy(c)
        del c2["per_seed"]["313"]
        with self.assertRaises(ValueError):
            pfc.paired_gain_interval(a, c2)

    def test_t_interval(self):
        m, h = pfc.t_interval([1.0, 2.0, 3.0])
        self.assertEqual(m, 2.0)
        self.assertAlmostEqual(h, 4.303 / 3 ** 0.5)
        m, h = pfc.t_interval([1, 2, 3, 4, 5])
        self.assertAlmostEqual(h, 2.776 * (2.5 ** 0.5) / 5 ** 0.5)
        with self.assertRaises(ValueError):
            pfc.t_interval([1, 2])

    def test_funnel_identity_checks(self):
        pfc.check_funnel_floor({"config": {"review_floor": 0.01}})
        for f in ({}, {"config": None}, {"config": {}}, {"config": {"review_floor": 0.2}}, {"config": {"review_floor": "0.01"}}):
            with self.assertRaises(ValueError):
                pfc.check_funnel_floor(f)
        pfc.check_funnel_modelset({"config": {"modelset": ""}}, "baseline")
        pfc.check_funnel_modelset({"config": {"modelset": "pairfeat1"}}, "pairfeat1")
        for f, a in (({"config": {}}, "baseline"), ({"config": {"modelset": None}}, "baseline"),
                     ({"config": {"modelset": "pairfeat1"}}, "baseline"), ({"config": {"modelset": ""}}, "pairfeat1"),
                     ({"config": {"modelset": "v3"}}, "pairfeat1"), ({}, "pairfeat1")):
            with self.assertRaises(ValueError):
                pfc.check_funnel_modelset(f, a)
        sc = {"person_split": "dev", "reviewer_error": 0.0, "day15_ungraded_prep_reviewer_error": 0.0}
        pfc.check_funnel_scope({"scope": sc}, "dev-clean")
        for mode in ("dev-stress", "validation-clean"):
            with self.assertRaises(ValueError):
                pfc.check_funnel_scope({"scope": sc}, mode)

    def test_review_load(self):
        q = [{"item": "a", "kinds": ["link"], "pair": ["x", "y"]}, {"item": "b", "kinds": ["link"], "pair": ["y", "x"]},
             {"item": "c", "kinds": ["link"], "pair": ["p", "q"]},
             {"item": "d", "kinds": ["link"], "pair": ["m", "n"], "reason": "second_identifier"},
             {"item": "e", "kinds": ["other"], "pair": ["u", "v"]}]
        self.assertEqual(pfc.review_load(q, [{"item": "a"}]), {"items": 3, "distinct_pairs": 2, "unanswered": 1})


class Guard(unittest.TestCase):
    ROOT = "/repo"
    BIN = "/repo/bin/wsi7"
    O = "/data/exp/arm/v2-s307"

    def cmds(self, mode="dev-clean", arm_="baseline", seed=307):  # pairfeat1 experiment
        return pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, mode, arm_)

    def test_lane_commands_pass_guard_both_arms(self):
        for mode in pfc.MODES:
            seed = pfc.modes(P1)[mode]["seeds"][0]
            for a in pfc.arms(P1):
                names = [n for n, _, _ in self.cmds(mode, a, seed)]
                self.assertEqual(names, ["gen", "sim", "augment", "orchestrate", "audit", "report7", "classifier"])
                for _, argv, _ in self.cmds(mode, a, seed):
                    pfc.guard(argv, mode, a, self.BIN, P1)

    def test_cohort_identical_and_modelset_only_on_model_commands(self):
        b = {n: argv for n, argv, _ in self.cmds("dev-clean", "baseline")}
        c = {n: argv for n, argv, _ in self.cmds("dev-clean", "pairfeat1")}
        for n in ("gen", "sim", "augment", "classifier"):
            self.assertEqual(b[n], c[n], n)
            self.assertNotIn("-modelset", c[n])
        for n in ("orchestrate", "audit", "report7"):
            self.assertNotIn("-modelset", b[n])
            i = c[n].index("-modelset")
            self.assertEqual(c[n][i + 1], "pairfeat1")
            self.assertEqual(c[n].count("-modelset"), 1)
            without = c[n][:i] + c[n][i + 2:]
            self.assertEqual(without, b[n], n)
            for argv in (b[n], c[n]):
                self.assertEqual(argv[argv.index("-linkfloor") + 1], "0.01")
                self.assertEqual(argv[argv.index("-policy") + 1], "v7strict")

    def mutate(self, name, mode="dev-clean", arm_="baseline", fn=None):
        for n, argv, _ in self.cmds(mode, arm_):
            if n == name:
                return fn(list(argv))
        raise AssertionError(name)

    def rejects(self, argv, mode="dev-clean", arm_="baseline"):
        with self.assertRaises(pfc.GuardError, msg=str(argv)):
            pfc.guard(argv, mode, arm_, self.BIN, P1)

    def setv(self, argv, flag, v):
        argv[argv.index(flag) + 1] = v
        return argv

    def test_candidate_v7ltt_rejected(self):
        for n in ("orchestrate", "audit", "report7"):
            for a in pfc.arms(P1):
                self.rejects(self.mutate(n, arm_=a, fn=lambda x: self.setv(x, "-policy", "v7ltt")), arm_=a)
        self.rejects(self.mutate("sim", arm_="pairfeat1", fn=lambda x: x + ["-policy", "v7ltt"]), arm_="pairfeat1")
        self.rejects(self.mutate("orchestrate", arm_="pairfeat1", fn=lambda x: self.setv(x, "-policy", "v6")), arm_="pairfeat1")

    def test_modelset_placement(self):
        for n in ("gen", "sim"):
            for a in pfc.arms(P1):
                self.rejects(self.mutate(n, arm_=a, fn=lambda x: x + ["-modelset", "pairfeat1"]), arm_=a)
        self.rejects(self.mutate("augment", arm_="pairfeat1", fn=lambda x: x + ["-modelset", "pairfeat1"]), arm_="pairfeat1")
        for n in ("orchestrate", "audit", "report7"):
            self.rejects(self.mutate(n, arm_="baseline", fn=lambda x: x + ["-modelset", "pairfeat1"]))
            self.rejects(self.mutate(n, arm_="baseline", fn=lambda x: x + ["-modelset", ""]))

            def drop(x):
                i = x.index("-modelset")
                return x[:i] + x[i + 2:]
            self.rejects(self.mutate(n, arm_="pairfeat1", fn=drop), arm_="pairfeat1")
            self.rejects(self.mutate(n, arm_="pairfeat1", fn=lambda x: self.setv(x, "-modelset", "v3")), arm_="pairfeat1")
            self.rejects(self.mutate(n, arm_="pairfeat1", fn=lambda x: x + ["-modelset", "pairfeat1"]), arm_="pairfeat1")
            self.rejects(self.mutate(n, arm_="pairfeat1", fn=lambda x: x + ["-modelset=pairfeat1"]), arm_="pairfeat1")

    def test_floor_injection(self):
        for n in ("orchestrate", "audit", "report7"):
            for a in pfc.arms(P1):
                self.rejects(self.mutate(n, arm_=a, fn=lambda x: self.setv(x, "-linkfloor", "0.2")), arm_=a)
                self.rejects(self.mutate(n, arm_=a, fn=lambda x: x + ["-linkfloor", "0.01"]), arm_=a)
                self.rejects(self.mutate(n, arm_=a, fn=lambda x: x + ["-linkfloor=0.2"]), arm_=a)
                self.rejects(self.mutate(n, arm_=a, fn=lambda x: x + ["--linkfloor", "0.2"]), arm_=a)

                def drop(x):
                    i = x.index("-linkfloor")
                    return x[:i] + x[i + 2:]
                self.rejects(self.mutate(n, arm_=a, fn=drop), arm_=a)
        self.rejects(self.mutate("sim", fn=lambda x: x + ["-linkfloor", "0.01"]))
        self.rejects(self.mutate("orchestrate", fn=lambda x: x + ["-linkfloor"]))

    def test_protected_settings(self):
        cases = [
            ("orchestrate", lambda x: self.setv(x, "-reader", "live")),
            ("audit", lambda x: self.setv(x, "-image", "on")),
            ("orchestrate", lambda x: self.setv(x, "-chaos", "1")),
            ("orchestrate", lambda x: self.setv(x, "-subkill", "0")),
            ("orchestrate", lambda x: self.setv(x, "-source", "hl7")),
            ("orchestrate", lambda x: self.setv(x, "-ann", "brute")),
            ("orchestrate", lambda x: self.setv(x, "-rseed", "311")),
            ("orchestrate", lambda x: self.setv(x, "-rerr", "0.04")),
            ("report7", lambda x: self.setv(x, "-errungraded", "0.25")),
            ("gen", lambda x: self.setv(x, "-seed", "101")),
            ("gen", lambda x: self.setv(x, "-split", "eval")),
            ("gen", lambda x: self.setv(x, "-noise", "v1")),
            ("gen", lambda x: self.setv(x, "-lung", "5")),
            ("gen", lambda x: self.setv(x, "-loops", "601")),
            ("gen", lambda x: x + ["-persons", "50"]),
            ("sim", lambda x: x + ["-split", "dev"]),
            ("sim", lambda x: x + ["-wasl", "/models/frozen"]),
            ("audit", lambda x: x + ["-run", "/x/heldout/run"]),
            ("classifier", lambda x: x[:-3] + ["eval", "0", "0"]),
        ]
        for n, fn in cases:
            self.rejects(self.mutate(n, fn=fn))
        self.rejects(["/bin/sh", "-c", "true"])
        self.rejects([self.BIN, "tamper", "-data", "x"])
        self.rejects(["python3", "-I", "/repo/scripts/other.py"])
        self.rejects(["python3", "/repo/pipeline/augment_cohort.py", "a", "b", "c"])
        with self.assertRaises(pfc.GuardError):
            pfc.guard(self.cmds()[0][1], "bogus", "baseline", self.BIN, P1)
        with self.assertRaises(pfc.GuardError):
            pfc.guard(self.cmds()[0][1], "dev-clean", "pairfeat2", self.BIN, P1)


class Repo:
    """Throwaway repo layout for preflight/freeze unit tests (never the real models)."""

    def __init__(self, base, cand=P1):
        self.cand = cand  # the one candidate this experiment parent serves
        self.root = os.path.join(base, "repo")
        os.makedirs(os.path.join(self.root, "models"))
        os.makedirs(os.path.join(self.root, "scripts"))
        for f in BASE_FILES + CAND_FILES + CAND2_FILES:
            with open(os.path.join(self.root, "models", f), "w") as h:
                h.write("model " + f)
        # each candidate encoder is an exact copy of the default encoder
        for c in ("pairfeat1", "pairfeat2"):
            with open(os.path.join(self.root, "models", "encoder_%s.json" % c), "wb") as h:
                h.write(_rb(os.path.join(self.root, "models", "encoder_weights.json")))
        for f in ("pairfeat_experiment.sh", "pairfeat_compare.py"):
            with open(os.path.join(self.root, "scripts", f), "w") as h:
                h.write("# " + f)
        self.exp = os.path.join(base, "exp")
        os.makedirs(self.exp)

    def hashes(self, files):
        return {f: _sha(os.path.join(self.root, "models", f)) for f in files}

    def artifact_manifest(self, head=HEAD, **kw):
        m = {"git_head": head, "baseline_chain_sha256": self.hashes(BASE_FILES),
             self.cand + "_chain_sha256": self.hashes(FILES[self.cand]), "fit_seed": 1, "other": {"ignored": True}}
        m.update(kw)
        _wj(os.path.join(self.exp, "artifact-manifest.json"), m)
        return m

    def selection(self, head=HEAD, **kw):
        s = {"selected_modelset": self.cand, "pass": True, "mode": "dev-clean", "seeds": DEV_SEEDS[self.cand],
             "git_head": head, "baseline_chain_sha256": self.hashes(BASE_FILES),
             self.cand + "_chain_sha256": self.hashes(FILES[self.cand]), "candidate": self.cand}
        s.update(kw)
        _wj(os.path.join(self.exp, "selection.json"), s)
        return s

    def stress(self, head=HEAD, **kw):
        s = {"pass": True, "mode": "dev-stress", "seeds": DEV_SEEDS[self.cand], "git_head": head,
             "baseline_chain_sha256": self.hashes(BASE_FILES), self.cand + "_chain_sha256": self.hashes(FILES[self.cand]),
             "selection_sha256": _sha(os.path.join(self.exp, "selection.json")), "candidate": self.cand}
        s.update(kw)
        _wj(os.path.join(self.exp, "stress.json"), s)
        return s


class Preflight(unittest.TestCase):
    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.r = Repo(self.td.name)

    def tearDown(self):
        self.td.cleanup()

    def pf(self, mode, a, head=HEAD):
        return pfc.preflight(mode, a, self.r.exp, self.r.root, head)

    def refused(self, mode, a, head=HEAD, msg=None):
        with self.assertRaises(pfc.PreflightError) as cm:
            self.pf(mode, a, head)
        if msg:
            self.assertIn(msg, str(cm.exception))

    def test_dev_clean(self):
        self.refused("dev-clean", "baseline", msg="artifact-manifest.json")
        self.r.artifact_manifest()
        self.assertEqual(self.pf("dev-clean", "baseline"), self.r.hashes(BASE_FILES))
        self.assertEqual(self.pf("dev-clean", "pairfeat1"), self.r.hashes(CAND_FILES))
        self.refused("dev-clean", "baseline", head="c" * 40, msg="git_head")
        for a, files in (("baseline", BASE_FILES), ("pairfeat1", CAND_FILES)):
            for f in files:
                p = os.path.join(self.r.root, "models", f)
                orig = _rb(p)
                with open(p, "ab") as h:
                    h.write(b"x")
                self.refused("dev-clean", a, msg=f)
                with open(p, "wb") as h:
                    h.write(orig)
        os.remove(os.path.join(self.r.root, "models", "iso_v7_pairfeat1.json"))
        self.refused("dev-clean", "pairfeat1")
        self.pf("dev-clean", "baseline")

    def test_malformed_artifact_manifest(self):
        good = self.r.artifact_manifest()
        bad = [dict(good, git_head="abc"), dict(good, baseline_chain_sha256={}),
               dict(good, pairfeat1_chain_sha256=dict(good["pairfeat1_chain_sha256"], extra="0" * 64)),
               dict(good, baseline_chain_sha256=good["pairfeat1_chain_sha256"])]
        for m in bad:
            _wj(os.path.join(self.r.exp, "artifact-manifest.json"), m)
            for a in pfc.arms(P1):
                self.refused("dev-clean", a)
        os.remove(os.path.join(self.r.exp, "artifact-manifest.json"))
        os.symlink(os.path.join(self.r.root, "models", "iso.json"), os.path.join(self.r.exp, "artifact-manifest.json"))
        self.refused("dev-clean", "baseline")

    def test_dev_stress_needs_passing_selection(self):
        self.r.artifact_manifest()
        self.refused("dev-stress", "baseline", msg="selection.json")
        for kw in ({"selected_modelset": None, "pass": False}, {"pass": False}, {"selected_modelset": None},
                   {"git_head": "c" * 40}, {"mode": "dev-stress"}, {"seeds": [101, 103, 107]},
                   {"pairfeat1_chain_sha256": chain(CAND_FILES, "x")}, {"baseline_chain_sha256": chain(BASE_FILES, "x")}):
            self.r.selection(**kw)
            for a in pfc.arms(P1):
                self.refused("dev-stress", a)
        self.r.selection()
        self.assertEqual(self.pf("dev-stress", "pairfeat1"), self.r.hashes(CAND_FILES))
        self.pf("dev-stress", "baseline")

    def upstream(self):
        self.r.artifact_manifest()
        self.r.selection()
        self.r.stress()

    def good_vm(self, **kw):
        ex = lambda n: _sha(os.path.join(self.r.exp, n))
        vm = {"git_head": HEAD, "mode": "validation-clean", "seeds": [401, 409, 419, 421, 431], "selected_modelset": "pairfeat1",
              "baseline_chain_sha256": self.r.hashes(BASE_FILES), "pairfeat1_chain_sha256": self.r.hashes(CAND_FILES),
              "runner_sha256": _sha(os.path.join(self.r.root, "scripts", "pairfeat_experiment.sh")),
              "analyzer_sha256": _sha(os.path.join(self.r.root, "scripts", "pairfeat_compare.py")),
              "settings": pfc.expected_settings("validation-clean", P1), "candidate": P1,
              "artifact_manifest_sha256": ex("artifact-manifest.json"), "selection_sha256": ex("selection.json"),
              "stress_sha256": ex("stress.json")}
        vm.update(kw)
        return vm

    def test_validation_manifest_fields(self):
        self.refused("validation-clean", "baseline", msg="validation-manifest.json")
        self.upstream()
        vp = os.path.join(self.r.exp, "validation-manifest.json")
        _wj(vp, self.good_vm())
        self.assertEqual(self.pf("validation-clean", "pairfeat1"), self.r.hashes(CAND_FILES))
        self.assertEqual(self.pf("validation-clean", "baseline"), self.r.hashes(BASE_FILES))
        bad = [{"git_head": "c" * 40}, {"seeds": [401, 409, 419, 421]}, {"seeds": [307, 311, 313]},
               {"runner_sha256": "0" * 64}, {"analyzer_sha256": "0" * 64}, {"selected_modelset": None},
               {"settings": dict(pfc.expected_settings("validation-clean", P1), linkfloor="0.2")},
               {"settings": pfc.expected_settings("dev-clean", P1)}, {"mode": "dev-clean"},
               {"pairfeat1_chain_sha256": {"encoder_pairfeat1.json": "0" * 64}},
               {"pairfeat1_chain_sha256": chain(CAND_FILES, "x")}, {"baseline_chain_sha256": chain(BASE_FILES, "x")},
               {"artifact_manifest_sha256": "0" * 64}, {"selection_sha256": "0" * 64}, {"stress_sha256": "0" * 64}]
        for kw in bad:
            _wj(vp, self.good_vm(**kw))
            for a in pfc.arms(P1):
                self.refused("validation-clean", a)
        for k in ("artifact_manifest_sha256", "selection_sha256", "stress_sha256"):
            vm = self.good_vm()
            del vm[k]
            _wj(vp, vm)
            self.refused("validation-clean", "baseline", msg=k)
        _wj(vp, self.good_vm())
        with open(os.path.join(self.r.root, "scripts", "pairfeat_compare.py"), "a") as h:
            h.write("tampered")
        self.refused("validation-clean", "baseline", msg="analyzer")

    def test_validation_lane_bound_to_own_model_files(self):
        self.upstream()
        vp = os.path.join(self.r.exp, "validation-manifest.json")
        _wj(vp, self.good_vm())
        p = os.path.join(self.r.root, "models", "lgb_stream_pairfeat1.json")
        with open(p, "a") as h:
            h.write("refit")
        self.refused("validation-clean", "pairfeat1", msg="lgb_stream_pairfeat1.json")
        self.pf("validation-clean", "baseline")

    def test_hand_written_manifest_cannot_bypass_freeze(self):
        vp = os.path.join(self.r.exp, "validation-manifest.json")
        # no upstream files at all: a hand-written manifest with plausible hashes is refused
        _wj(vp, {
            "git_head": HEAD, "mode": "validation-clean", "seeds": [401, 409, 419, 421, 431], "selected_modelset": "pairfeat1",
            "baseline_chain_sha256": self.r.hashes(BASE_FILES), "pairfeat1_chain_sha256": self.r.hashes(CAND_FILES),
            "runner_sha256": _sha(os.path.join(self.r.root, "scripts", "pairfeat_experiment.sh")),
            "analyzer_sha256": _sha(os.path.join(self.r.root, "scripts", "pairfeat_compare.py")),
            "settings": pfc.expected_settings("validation-clean", P1), "candidate": P1,
            "artifact_manifest_sha256": "1" * 64, "selection_sha256": "2" * 64, "stress_sha256": "3" * 64})
        for a in pfc.arms(P1):
            self.refused("validation-clean", a, msg="artifact-manifest.json")
        self.upstream()
        good = self.good_vm()
        for name in ("artifact-manifest.json", "selection.json", "stress.json"):
            os.rename(os.path.join(self.r.exp, name), os.path.join(self.r.exp, name + ".bak"))
            _wj(vp, good)
            self.refused("validation-clean", "baseline", msg=name)
            os.rename(os.path.join(self.r.exp, name + ".bak"), os.path.join(self.r.exp, name))
        # selection or stress rewritten as failing, with the manifest re-hashed to match: still refused (no freeze bypass)
        for writer, kw in ((self.r.selection, {"selected_modelset": None, "pass": False}), (self.r.stress, {"pass": False}),
                           (self.r.artifact_manifest, {"git_head": "c" * 40})):
            self.upstream()
            writer(**kw)
            if writer is self.r.selection:  # keep stress consistent with the rewritten selection
                self.r.stress()
            _wj(vp, self.good_vm())
            for a in pfc.arms(P1):
                self.refused("validation-clean", a)
        # stress computed against a different selection than the current one
        self.upstream()
        self.r.selection(extra=1)
        _wj(vp, self.good_vm())
        self.refused("validation-clean", "baseline", msg="stress.json")
        self.upstream()
        _wj(vp, self.good_vm())
        self.pf("validation-clean", "baseline")

    def test_invalid_mode_or_arm(self):
        self.r.artifact_manifest()
        for mode, a in (("bogus", "baseline"), ("dev-clean", "v3"), ("dev-clean", ""), ("validation", "baseline")):
            self.refused(mode, a)


class Freeze(unittest.TestCase):
    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.r = Repo(self.td.name)
        self.vp = os.path.join(self.r.exp, "validation-manifest.json")

    def tearDown(self):
        self.td.cleanup()

    def freeze(self, head=HEAD):
        return pfc.freeze_validation(self.r.exp, self.r.root, head)

    def refused(self, head=HEAD):
        with self.assertRaises(pfc.PreflightError):
            self.freeze(head)
        self.assertFalse(os.path.exists(self.vp))

    def test_freeze_requires_selection_and_stress(self):
        self.refused()
        self.r.artifact_manifest()
        self.refused()
        self.r.selection(selected_modelset=None, **{"pass": False})
        self.refused()
        self.r.selection()
        self.refused()  # no stress result yet
        self.r.stress(**{"pass": False})
        self.refused()
        for kw in ({"git_head": "c" * 40}, {"mode": "dev-clean"}, {"selection_sha256": "0" * 64},
                   {"pairfeat1_chain_sha256": chain(CAND_FILES, "x")}):
            self.r.stress(**kw)
            self.refused()
        self.r.stress()
        self.refused(head="c" * 40)
        p = os.path.join(self.r.root, "models", "iso_pairfeat1.json")
        orig = _rb(p)
        with open(p, "ab") as h:
            h.write(b"x")
        self.refused()
        with open(p, "wb") as h:
            h.write(orig)
        vm = self.freeze()
        self.assertEqual(_rj(self.vp), vm)
        self.assertEqual(vm["seeds"], [401, 409, 419, 421, 431])
        self.assertEqual(vm["git_head"], HEAD)
        self.assertEqual(vm["selected_modelset"], "pairfeat1")
        self.assertEqual(vm["baseline_chain_sha256"], self.r.hashes(BASE_FILES))
        self.assertEqual(vm["pairfeat1_chain_sha256"], self.r.hashes(CAND_FILES))
        self.assertEqual(vm["runner_sha256"], _sha(os.path.join(self.r.root, "scripts", "pairfeat_experiment.sh")))
        self.assertEqual(vm["analyzer_sha256"], _sha(os.path.join(self.r.root, "scripts", "pairfeat_compare.py")))
        self.assertEqual(vm["settings"], pfc.expected_settings("validation-clean", P1))
        self.assertEqual(vm["settings"]["linkfloor"], "0.01")
        self.assertEqual(vm["settings"]["policy"], "v7strict")
        for k in ("artifact_manifest_sha256", "selection_sha256", "stress_sha256"):
            self.assertEqual(len(vm[k]), 64)
        # the frozen manifest now authorises validation preflight, and is write-once
        pfc.preflight("validation-clean", "pairfeat1", self.r.exp, self.r.root, HEAD)
        with self.assertRaises(pfc.PreflightError):
            self.freeze()


class CLIFixture:
    """Synthetic experiment parent serving candidate CAND (no clinical lane, no build)."""
    CAND = P1

    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.exp = os.path.join(self.td.name, "exp")
        os.makedirs(self.exp)
        self.maps = {"baseline": chain(BASE_FILES, "b"), self.CAND: chain(FILES[self.CAND], "c")}
        _wj(os.path.join(self.exp, "artifact-manifest.json"),
            {"git_head": HEAD, "baseline_chain_sha256": self.maps["baseline"], self.CAND + "_chain_sha256": self.maps[self.CAND],
             "ignored": 1})

    def tearDown(self):
        self.td.cleanup()

    def make_arm(self, a, tp, mode="dev-clean", head=HEAD, binary=BIN_SHA, cfg_modelset=None, cfg_floor=0.01,
                 scope_mode=None, chain_map=None, build=True, name=None, build_arm=None, build_cand=None, seeds_of=None):
        m = pfc.modes(seeds_of or self.CAND)[mode]
        sm = pfc.MODES[scope_mode or mode]
        scope = {"person_split": sm["split"], "reviewer_error": float(sm["rerr"]),
                 "day15_ungraded_prep_reviewer_error": float(sm["errungraded"]), "noise": "v2"}
        ms = pfc.MODELSET[a] if cfg_modelset is None else cfg_modelset
        root = os.path.join(self.exp, name or (mode + "-" + a))
        for sd in m["seeds"]:
            d = os.path.join(root, "v2-s%d" % sd)
            os.makedirs(os.path.join(d, "run/main"))
            _wj(d + "/funnel.json", dict(funnel(tp, 10), scope=scope, config={"review_floor": cfg_floor, "modelset": ms}))
            _wj(d + "/run/main/report7.json", report(tp=tp))
            _wj(d + "/run/main/review_queue.json", [])
            _wj(d + "/run/main/status-linker.json", {})
        if build:
            ba = build_arm or a
            _wj(root + "/build.json", {"git_head": head, "binary_sha256": binary, "floor": 0.01, "mode": mode, "arm": ba,
                                       "modelset": pfc.MODELSET[ba], "candidate": build_cand or self.CAND,
                                       "model_chain_sha256": self.maps[a] if chain_map is None else chain_map})
        return root

    def run_cli(self, mode, b, c, out=None, cand=None):
        return pfc.main([mode, out or os.path.join(self.exp, mode + "-summary.json"), "baseline=" + b,
                         (cand or self.CAND) + "=" + c])

    def assert_rejected(self, b, c, mode="dev-clean", cand=None):
        with self.assertRaises((ValueError, KeyError, OSError)):
            self.run_cli(mode, b, c, cand=cand)
        self.assertFalse(os.path.exists(os.path.join(self.exp, "selection.json")))


class CLI(CLIFixture, unittest.TestCase):
    """Analyzer end-to-end on synthetic lane files (no clinical lane, no build)."""

    def test_dev_clean_selects_on_pass_only(self):
        self.assertEqual(self.run_cli("dev-clean", self.make_arm("baseline", 5), self.make_arm("pairfeat1", 6)), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual(sel["selected_modelset"], "pairfeat1")
        self.assertEqual(sel["git_head"], HEAD)
        self.assertEqual(sel["baseline_chain_sha256"], self.maps["baseline"])
        self.assertEqual(sel["pairfeat1_chain_sha256"], self.maps["pairfeat1"])
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        self.assertIn("paired_gain_t95", s)
        self.assertIn("residual", s["arms"]["pairfeat1"])
        # selection.json is write-once
        with self.assertRaises((ValueError, OSError)):
            self.run_cli("dev-clean", os.path.join(self.exp, "dev-clean-baseline"), os.path.join(self.exp, "dev-clean-pairfeat1"),
                         out=os.path.join(self.exp, "again.json"))

    def test_dev_clean_failure_writes_null_selection(self):
        self.assertEqual(self.run_cli("dev-clean", self.make_arm("baseline", 5), self.make_arm("pairfeat1", 5)), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertIsNone(sel["selected_modelset"])
        self.assertFalse(sel["pass"])

    def test_mislabeled_funnel_modelset_floor_scope(self):
        b = self.make_arm("baseline", 5)
        self.assert_rejected(b, self.make_arm("pairfeat1", 6, cfg_modelset="", name="c1"))
        self.assert_rejected(self.make_arm("baseline", 5, cfg_modelset="pairfeat1", name="b1"), self.make_arm("pairfeat1", 6, name="c2"))
        self.assert_rejected(b, self.make_arm("pairfeat1", 6, cfg_floor=0.2, name="c3"))
        self.assert_rejected(self.make_arm("baseline", 5, cfg_floor=0.2, name="b2"), self.make_arm("pairfeat1", 6, name="c4"))
        self.assert_rejected(b, self.make_arm("pairfeat1", 6, scope_mode="dev-stress", name="c5"))

    def test_build_identity_between_arms(self):
        b = self.make_arm("baseline", 5)
        self.assert_rejected(b, self.make_arm("pairfeat1", 6, head="c" * 40, name="c1"))
        self.assert_rejected(b, self.make_arm("pairfeat1", 6, binary="d" * 64, name="c2"))
        self.assert_rejected(b, self.make_arm("pairfeat1", 6, build=False, name="c3"))
        self.assert_rejected(b, self.make_arm("pairfeat1", 6, build_arm="baseline", chain_map=self.maps["pairfeat1"], name="c4"))
        # both arms agree with each other but not with the artifact manifest head
        self.assert_rejected(self.make_arm("baseline", 5, head="e" * 40, name="b1"), self.make_arm("pairfeat1", 6, head="e" * 40, name="c5"))

    def test_artifact_hashes_must_match_manifest_per_arm(self):
        b = self.make_arm("baseline", 5)
        c = self.make_arm("pairfeat1", 6)
        self.assert_rejected(self.make_arm("baseline", 5, chain_map=chain(BASE_FILES, "x"), name="b1"), c)
        self.assert_rejected(b, self.make_arm("pairfeat1", 6, chain_map=chain(CAND_FILES, "x"), name="c1"))
        # an arm carrying the other arm's chain is rejected (baseline and candidate hashes are intentionally different)
        self.assert_rejected(self.make_arm("baseline", 5, chain_map=self.maps["pairfeat1"], name="b2"), c)
        self.assert_rejected(b, self.make_arm("pairfeat1", 6, chain_map=self.maps["baseline"], name="c2"))
        os.remove(os.path.join(self.exp, "artifact-manifest.json"))
        self.assert_rejected(b, c)

    def test_bad_arm_arguments(self):
        b = self.make_arm("baseline", 5)
        out = os.path.join(self.exp, "s.json")
        self.assertEqual(pfc.main(["dev-clean", out, "baseline=" + b]), 2)
        self.assertEqual(pfc.main(["dev-clean", out, "baseline=" + b, "baseline=" + b]), 2)
        self.assertEqual(pfc.main(["dev-clean", out, "baseline=" + b, "v3=" + b]), 2)
        self.assertEqual(pfc.main(["bogus", out, "baseline=" + b, "pairfeat1=" + b]), 2)
        self.assertEqual(pfc.main(["dev-clean", os.path.join(self.exp, "selection.json"), "baseline=" + b, "pairfeat1=" + b]), 2)

    def test_dev_stress_requires_selection_and_writes_stress(self):
        b = self.make_arm("baseline", 5, mode="dev-stress")
        c = self.make_arm("pairfeat1", 5, mode="dev-stress")
        with self.assertRaises(ValueError):
            self.run_cli("dev-stress", b, c)
        sel = {"selected_modelset": "pairfeat1", "pass": True, "mode": "dev-clean", "seeds": [307, 311, 313], "git_head": HEAD,
               "baseline_chain_sha256": self.maps["baseline"], "pairfeat1_chain_sha256": self.maps["pairfeat1"], "candidate": P1}
        _wj(os.path.join(self.exp, "selection.json"), dict(sel, selected_modelset=None, **{"pass": False}))
        with self.assertRaises(ValueError):
            self.run_cli("dev-stress", b, c)
        _wj(os.path.join(self.exp, "selection.json"), sel)
        self.assertEqual(self.run_cli("dev-stress", b, c), 0)
        st = _rj(os.path.join(self.exp, "stress.json"))
        self.assertTrue(st["pass"])
        self.assertEqual(st["mode"], "dev-stress")
        self.assertEqual(st["selection_sha256"], _sha(os.path.join(self.exp, "selection.json")))
        self.assertEqual(st["pairfeat1_chain_sha256"], self.maps["pairfeat1"])

    def test_dev_stress_regression_fails(self):
        sel = {"selected_modelset": "pairfeat1", "pass": True, "mode": "dev-clean", "seeds": [307, 311, 313], "git_head": HEAD,
               "baseline_chain_sha256": self.maps["baseline"], "pairfeat1_chain_sha256": self.maps["pairfeat1"], "candidate": P1}
        _wj(os.path.join(self.exp, "selection.json"), sel)
        self.assertEqual(self.run_cli("dev-stress", self.make_arm("baseline", 6, mode="dev-stress"),
                                      self.make_arm("pairfeat1", 5, mode="dev-stress")), 0)
        self.assertFalse(_rj(os.path.join(self.exp, "stress.json"))["pass"])

    def test_validation_analysis_requires_verified_frozen_manifest(self):
        r = Repo(os.path.join(self.td.name, "v"))
        self.exp = r.exp
        self.maps = {"baseline": r.hashes(BASE_FILES), "pairfeat1": r.hashes(CAND_FILES)}
        b = self.make_arm("baseline", 5, mode="validation-clean")
        c = self.make_arm("pairfeat1", 6, mode="validation-clean")
        out = os.path.join(r.exp, "validation-clean-summary.json")
        run = lambda head=HEAD: pfc.analyse("validation-clean", out, {"baseline": b, "pairfeat1": c}, repo=r.root, head=head)
        with self.assertRaises(OSError):
            run()  # no validation manifest
        r.artifact_manifest()
        r.selection()
        r.stress()
        vm = pfc.freeze_validation(r.exp, r.root, HEAD)
        vp = os.path.join(r.exp, "validation-manifest.json")
        bad = [{"runner_sha256": "1" * 64}, {"analyzer_sha256": "2" * 64}, {"git_head": "c" * 40},
               {"settings": dict(pfc.expected_settings("validation-clean", P1), policy="v7ltt")},
               {"settings": dict(pfc.expected_settings("validation-clean", P1), linkfloor="0.2")},
               {"artifact_manifest_sha256": "0" * 64}, {"selection_sha256": "0" * 64}, {"stress_sha256": "0" * 64},
               {"pairfeat1_chain_sha256": chain(CAND_FILES, "x")}, {"seeds": [307, 311, 313]}]
        for kw in bad:
            _wj(vp, dict(vm, **kw))
            with self.assertRaises((ValueError, OSError), msg=str(kw)):
                run()
            self.assertFalse(os.path.exists(out))
        _wj(vp, vm)
        with self.assertRaises(ValueError):
            run(head="c" * 40)  # current HEAD moved since freeze
        sel = _rj(os.path.join(r.exp, "selection.json"))
        r.selection(selected_modelset=None, **{"pass": False})  # upstream selection no longer the frozen one
        with self.assertRaises(ValueError):
            run()
        _wj(os.path.join(r.exp, "selection.json"), sel)
        st = _rj(os.path.join(r.exp, "stress.json"))
        _wj(os.path.join(r.exp, "stress.json"), dict(st, **{"pass": False}))
        with self.assertRaises(ValueError):
            run()
        _wj(os.path.join(r.exp, "stress.json"), st)
        self.assertFalse(os.path.exists(out))
        res = run()
        self.assertTrue(res["gate"]["pass"])
        self.assertEqual(res["gate"]["kind"], "validation")
        self.assertEqual(_rj(out)["gate"], res["gate"])
        # the headline validation verdict is write-once
        before = _rb(out)
        with self.assertRaises(FileExistsError):
            run()
        self.assertEqual(_rb(out), before)


class RunnerFixture:
    """Shell runner helpers: every case exits before any build (preflight refusals or PAIRFEAT_PREFLIGHT_ONLY)."""

    def run_sh(self, *args, preflight_only=False):
        env = dict(os.environ, HOME=self.home)
        env.pop("PAIRFEAT_PREFLIGHT_ONLY", None)
        if preflight_only:
            env["PAIRFEAT_PREFLIGHT_ONLY"] = "1"
        return subprocess.run(["bash", SCRIPT] + list(args), env=env, capture_output=True, text=True, timeout=60)

    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.home = os.path.realpath(self.td.name)
        self.exp = os.path.join(self.home, "wasl-ml-data", "exp")
        os.makedirs(self.exp)
        self.root = os.path.join(self.exp, "arm")
        self.head = subprocess.run(["git", "rev-parse", "HEAD"], cwd=HERE, capture_output=True, text=True).stdout.strip()

    def tearDown(self):
        self.td.cleanup()

    def assert_refused(self, r, msg=None):
        self.assertEqual(r.returncode, 2, r.stdout + r.stderr)
        if msg:
            self.assertIn(msg, r.stderr)
        self.assertFalse(os.path.exists(self.root))

    def real_artifact_manifest(self, cand=P1, **kw):
        m = {"git_head": self.head,
             "baseline_chain_sha256": {f: _sha(os.path.join(REPO, "models", f)) for f in BASE_FILES},
             cand + "_chain_sha256": chain(FILES[cand], "c")}
        m.update(kw)
        _wj(os.path.join(self.exp, "artifact-manifest.json"), m)


class Runner(RunnerFixture, unittest.TestCase):
    """Shell runner: every case exits before any build (preflight refusals or PAIRFEAT_PREFLIGHT_ONLY)."""

    def test_invalid_args(self):
        for args in [(), (self.root,), (self.root, "dev-clean"), (self.root, "bogus", "baseline"),
                     (self.root, "dev-clean", "v3"), (self.root, "dev-clean", "Baseline"), (self.root, "dev-clean", ""),
                     (self.root, "dev-clean", "pairfeat1", "0.2"), (self.root, "dev-clean", "baseline", "-linkfloor", "0.2"),
                     (self.root, "dev-clean", "0.01"), (self.root, "dev-clean", "pairfeat1 -linkfloor 0.2"),
                     (self.root, "validation", "baseline"), ("", "dev-clean", "baseline"),
                     (self.root + "/../arm", "dev-clean", "baseline"), ("/tmp/outside", "dev-clean", "baseline"),
                     (os.path.join(self.home, "elsewhere", "arm"), "dev-clean", "baseline")]:
            self.assert_refused(self.run_sh(*args))

    def test_existing_root_rejected(self):
        os.mkdir(self.root)
        r = self.run_sh(self.root, "dev-clean", "baseline")
        self.assertEqual(r.returncode, 2)
        self.assertIn("already exists", r.stderr)

    def test_dev_clean_requires_artifact_manifest(self):
        self.assert_refused(self.run_sh(self.root, "dev-clean", "baseline"), "artifact-manifest.json")
        self.real_artifact_manifest(git_head="0" * 40)
        self.assert_refused(self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True), "git_head")
        self.real_artifact_manifest(baseline_chain_sha256=chain(BASE_FILES, "x"))
        self.assert_refused(self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True), "encoder_weights.json")
        self.real_artifact_manifest()
        # candidate artifacts are absent from the real models/ dir (or differ from the fake map): refused before any build
        self.assert_refused(self.run_sh(self.root, "dev-clean", "pairfeat1", preflight_only=True))

    def test_preflight_only_passes_without_creating_root(self):
        self.real_artifact_manifest()
        r = self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("preflight ok", r.stdout)
        self.assertFalse(os.path.exists(self.root))

    def test_dev_stress_requires_passing_selection(self):
        self.real_artifact_manifest()
        self.assert_refused(self.run_sh(self.root, "dev-stress", "baseline", preflight_only=True), "selection.json")
        sp = os.path.join(self.exp, "selection.json")
        _wj(sp, {"selected_modelset": None, "pass": False})
        self.assert_refused(self.run_sh(self.root, "dev-stress", "baseline", preflight_only=True))
        sel = {"selected_modelset": "pairfeat1", "pass": True, "mode": "dev-clean", "seeds": [307, 311, 313], "git_head": self.head,
               "baseline_chain_sha256": {f: _sha(os.path.join(REPO, "models", f)) for f in BASE_FILES},
               "pairfeat1_chain_sha256": chain(CAND_FILES, "c"), "candidate": P1}
        _wj(sp, dict(sel, git_head="0" * 40))
        self.assert_refused(self.run_sh(self.root, "dev-stress", "baseline", preflight_only=True))
        _wj(sp, sel)
        r = self.run_sh(self.root, "dev-stress", "baseline", preflight_only=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertFalse(os.path.exists(self.root))

    def test_validation_requires_frozen_manifest(self):
        self.real_artifact_manifest()
        self.assert_refused(self.run_sh(self.root, "validation-clean", "baseline", preflight_only=True), "validation-manifest.json")
        am = _rj(os.path.join(self.exp, "artifact-manifest.json"))
        ident = {"git_head": self.head, "baseline_chain_sha256": am["baseline_chain_sha256"],
                 "pairfeat1_chain_sha256": am["pairfeat1_chain_sha256"], "candidate": P1}
        sp, tp = os.path.join(self.exp, "selection.json"), os.path.join(self.exp, "stress.json")
        _wj(sp, dict(ident, selected_modelset="pairfeat1", mode="dev-clean", seeds=[307, 311, 313], **{"pass": True}))
        _wj(tp, dict(ident, mode="dev-stress", seeds=[307, 311, 313], selection_sha256=_sha(sp), **{"pass": True}))
        ex = lambda n: _sha(os.path.join(self.exp, n))
        vm = dict(ident, mode="validation-clean", seeds=[401, 409, 419, 421, 431], selected_modelset="pairfeat1",
                  runner_sha256=_sha(SCRIPT), analyzer_sha256=_sha(os.path.join(HERE, "pairfeat_compare.py")),
                  settings=pfc.expected_settings("validation-clean", P1), artifact_manifest_sha256=ex("artifact-manifest.json"),
                  selection_sha256=ex("selection.json"), stress_sha256=ex("stress.json"))
        vp = os.path.join(self.exp, "validation-manifest.json")
        for kw in ({"git_head": "0" * 40}, {"seeds": [211, 223, 227, 229, 233]}, {"runner_sha256": "0" * 64},
                   {"analyzer_sha256": "0" * 64}, {"baseline_chain_sha256": chain(BASE_FILES, "x")},
                   {"selection_sha256": "0" * 64}, {"stress_sha256": "0" * 64}, {"artifact_manifest_sha256": "0" * 64}):
            _wj(vp, dict(vm, **kw))
            self.assert_refused(self.run_sh(self.root, "validation-clean", "baseline", preflight_only=True))
        # a hand-written manifest is refused once the stress result it hashes is a failure
        st = _rj(tp)
        _wj(tp, dict(st, **{"pass": False}))
        _wj(vp, dict(vm, stress_sha256=ex("stress.json")))
        self.assert_refused(self.run_sh(self.root, "validation-clean", "baseline", preflight_only=True), "stress.json")
        _wj(tp, st)
        _wj(vp, vm)
        r = self.run_sh(self.root, "validation-clean", "baseline", preflight_only=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertFalse(os.path.exists(self.root))

    def test_runner_structure(self):
        with open(SCRIPT) as h:
            text = h.read()
        build = text.index('"$ROOT/scripts/demo.sh" build')
        po = text.index("${PAIRFEAT_PREFLIGHT_ONLY:-}")
        self.assertLess(text.index('pairfeat_compare.py" preflight'), po)
        self.assertLess(po, text.index("status --porcelain --untracked-files=all"))
        self.assertLess(text.index("status --porcelain --untracked-files=all"), build)
        self.assertLess(build, text.index('"model_chain_sha256"'))
        self.assertLess(text.index('"model_chain_sha256"'), text.index("for seed in seeds:"))
        self.assertIn("pfc.guard(", text)
        self.assertIn("pfc.lane_commands(", text)
        self.assertNotIn("link_floor_compare", text)



# ================================================================ candidate set: pairfeat1 | pairfeat2

P2 = "pairfeat2"


class Candidates(unittest.TestCase):
    def test_candidate_set_and_seeds(self):
        self.assertEqual(pfc.CANDIDATES, {"pairfeat1": {"dev_seeds": [307, 311, 313], "review_gate": "total"},
                                         "pairfeat2": {"dev_seeds": [317, 331, 337], "review_gate": "total"},
                                         "pairfeat3": {"dev_seeds": [347, 353, 367], "review_gate": "total"},
                                         "pairfeat4": {"dev_seeds": [433, 449, 461], "review_gate": "wrong_pair"},
                                         "pairfeat5": {"dev_seeds": [521, 541, 547], "review_gate": "wrong_pair_cap",
                                                       "report_xscript_cap": True},
                                         "r1review": {"dev_seeds": [571, 577, 587], "validation_seeds": [631, 641, 643, 647, 653],
                                                      "review_gate": "wrong_pair_cap", "report_xscript_cap": True,
                                                      "chain": "pairfeat5", "reference": "pairfeat5",
                                                      "extra_flags": ["-r1review"], "lane_image": True,
                                                      "baseline_gating": False}})
        self.assertEqual(pfc.VALIDATION_SEEDS, [401, 409, 419, 421, 431])
        self.assertEqual(pfc.ALLOWED_ARMS, ("baseline", "pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4",
                                                  "pairfeat5", "r1review"))
        self.assertEqual(pfc.modes(P2), {
            "dev-clean": {"split": "dev", "seeds": [317, 331, 337], "rerr": "0", "errungraded": "0"},
            "dev-stress": {"split": "dev", "seeds": [317, 331, 337], "rerr": "0.04", "errungraded": "0.25"},
            "validation-clean": {"split": "eval", "seeds": [401, 409, 419, 421, 431], "rerr": "0", "errungraded": "0"},
        })
        self.assertEqual(pfc.arms(P2), ("baseline", "pairfeat2"))
        self.assertEqual(pfc.CHAIN_FILES[P2], CAND2_FILES)
        self.assertEqual(pfc.CHAIN_KEY[P2], "pairfeat2_chain_sha256")
        self.assertEqual(pfc.MODELSET, {"baseline": "", "pairfeat1": "pairfeat1", "pairfeat2": "pairfeat2",
                                        "pairfeat3": "pairfeat3", "pairfeat4": "pairfeat4",
                                        "pairfeat5": "pairfeat5", "r1review": "pairfeat5"})
        for bad in ("baseline", "pairfeat6", "", None):
            with self.assertRaises(ValueError):
                pfc.mode_settings("dev-clean", bad)
            with self.assertRaises(ValueError):
                pfc.arms(bad)
            with self.assertRaises(ValueError):
                pfc.expected_settings("validation-clean", bad)

    def test_expected_settings_per_candidate(self):
        # pairfeat1 validation settings are byte-for-byte what the single-candidate harness froze
        self.assertEqual(pfc.expected_settings("validation-clean", P1), {
            "mode": "validation-clean", "split": "eval", "seeds": [401, 409, 419, 421, 431], "rerr": "0", "errungraded": "0",
            "linkfloor": "0.01", "policy": "v7strict", "nats_kill": False, "cohort": "crc-only", "dual": True,
            "candidate_modelset": "pairfeat1", "candidate_modelset_commands": ["orchestrate", "audit", "report7"],
            "arms": ["baseline", "pairfeat1"], "arms_sequential": True,
            "reader": "cached", "image": "off", "source": "fhir", "chaos": "0", "subkill": "1", "ann": "hnsw", "noise": "v2",
            "loops": "600", "distract": "200", "rdouble": "merge+close", "preps": "3", "threads": "8"})
        v2 = pfc.expected_settings("validation-clean", P2)
        self.assertEqual(v2["seeds"], [401, 409, 419, 421, 431])
        self.assertEqual((v2["candidate_modelset"], v2["arms"]), ("pairfeat2", ["baseline", "pairfeat2"]))
        self.assertEqual({k: v for k, v in v2.items() if k not in ("candidate_modelset", "arms")},
                         {k: v for k, v in pfc.expected_settings("validation-clean", P1).items()
                          if k not in ("candidate_modelset", "arms")})
        self.assertEqual(pfc.expected_settings("dev-clean", P2)["seeds"], [317, 331, 337])
        self.assertEqual(pfc.expected_settings("dev-stress", P1)["seeds"], [307, 311, 313])

    def test_infer_candidate(self):
        b = chain(BASE_FILES, "b")
        ok = {"git_head": HEAD, "baseline_chain_sha256": b, "pairfeat2_chain_sha256": chain(CAND2_FILES, "c"),
              "training_manifest_sha256": "1" * 64, "csv_sha256": {"x.csv": "2" * 64}, "parity": {}}
        self.assertEqual(pfc.infer_candidate(ok, "artifact-manifest.json"), P2)
        self.assertEqual(pfc.infer_candidate(dict(b=1, baseline_chain_sha256=b, pairfeat1_chain_sha256={}), "x"), P1)
        for bad in ({"baseline_chain_sha256": b},
                    dict(ok, pairfeat1_chain_sha256=chain(CAND_FILES, "c")),
                    {"baseline_chain_sha256": b, "pairfeat6_chain_sha256": {}},
                    {"baseline_chain_sha256": b, "_chain_sha256": {}},
                    {"baseline_chain_sha256": b, "Pairfeat2_chain_sha256": {}}, [], None):
            with self.assertRaises(pfc.PreflightError, msg=str(bad)):
                pfc.infer_candidate(bad, "artifact-manifest.json")


class GuardPairfeat2(unittest.TestCase):
    ROOT = "/repo"
    BIN = "/repo/bin/wsi7"
    O = "/data/exp/arm/v2-s317"

    def cmds(self, mode="dev-clean", arm_="baseline", seed=317):
        return pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, mode, arm_)

    def rejects(self, argv, mode="dev-clean", arm_="baseline", cand=P2):
        with self.assertRaises(pfc.GuardError, msg=str(argv)):
            pfc.guard(argv, mode, arm_, self.BIN, cand)

    def test_seeds_follow_the_candidate(self):
        for mode in pfc.MODES:
            for seed in pfc.modes(P2)[mode]["seeds"]:
                for a in pfc.arms(P2):
                    for _, argv, _ in self.cmds(mode, a, seed):
                        pfc.guard(argv, mode, a, self.BIN, P2)
        # pairfeat1 development seeds are not authorised in a pairfeat2 experiment, and vice versa (both arms)
        for mode in ("dev-clean", "dev-stress"):
            for a in pfc.arms(P2):
                for n, argv, _ in self.cmds(mode, a, 307):
                    if n in ("gen", "orchestrate"):
                        self.rejects(argv, mode, a)
            for n, argv, _ in self.cmds(mode, "baseline", 317):
                if n in ("gen", "orchestrate"):
                    self.rejects(argv, mode, "baseline", cand=P1)
        # validation seeds are shared
        for a in ("baseline",):
            for _, argv, _ in self.cmds("validation-clean", a, 401):
                pfc.guard(argv, "validation-clean", a, self.BIN, P1)
                pfc.guard(argv, "validation-clean", a, self.BIN, P2)

    def test_modelset_only_on_model_commands(self):
        b = {n: argv for n, argv, _ in self.cmds("dev-clean", "baseline")}
        c = {n: argv for n, argv, _ in self.cmds("dev-clean", P2)}
        for n in ("gen", "sim", "augment", "classifier"):
            self.assertEqual(b[n], c[n], n)
            self.assertNotIn("-modelset", c[n])
        for n in ("orchestrate", "audit", "report7"):
            self.assertNotIn("-modelset", b[n])
            i = c[n].index("-modelset")
            self.assertEqual(c[n][i + 1], "pairfeat2")
            self.assertEqual(c[n].count("-modelset"), 1)
            self.assertEqual(c[n][:i] + c[n][i + 2:], b[n], n)
            self.assertEqual(c[n][c[n].index("-linkfloor") + 1], "0.01")
            self.assertEqual(c[n][c[n].index("-policy") + 1], "v7strict")
            self.rejects(self.setv(list(c[n]), "-modelset", "pairfeat1"), arm_=P2)
            self.rejects(self.setv(list(c[n]), "-policy", "v7ltt"), arm_=P2)
            self.rejects(list(b[n]) + ["-modelset", "pairfeat2"])
        for n in ("gen", "sim"):
            self.rejects(list(c[n]) + ["-modelset", "pairfeat2"], arm_=P2)

    def setv(self, argv, flag, v):
        argv[argv.index(flag) + 1] = v
        return argv

    def test_arm_must_be_the_experiment_candidate(self):
        p1 = {n: argv for n, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, self.O, 307, "dev-clean", P1)}
        with self.assertRaises(pfc.GuardError):
            pfc.guard(p1["orchestrate"], "dev-clean", P1, self.BIN, P2)
        with self.assertRaises(pfc.GuardError):
            pfc.guard(self.cmds()[0][1], "dev-clean", P2, self.BIN, P1)
        for bad in ("pairfeat6", "baseline", "", None):
            with self.assertRaises(pfc.GuardError):
                pfc.guard(self.cmds()[0][1], "dev-clean", "baseline", self.BIN, bad)

    def test_funnel_modelset(self):
        pfc.check_funnel_modelset({"config": {"modelset": "pairfeat2"}}, P2)
        pfc.check_funnel_modelset({"config": {"modelset": ""}}, "baseline")
        for f, a in (({"config": {"modelset": "pairfeat1"}}, P2), ({"config": {"modelset": ""}}, P2),
                     ({"config": {"modelset": "pairfeat2"}}, "baseline"), ({"config": {"modelset": "pairfeat2"}}, P1),
                     ({"config": {}}, P2), ({"config": {"modelset": "pairfeat2"}}, "pairfeat6")):
            with self.assertRaises(ValueError):
                pfc.check_funnel_modelset(f, a)

    def test_gate_and_selection_record(self):
        base = pfc_arm = arm("baseline", 500, 600)
        g = pfc.gate(base, arm(P2, 512, 600, distinct_pairs=110), "selection")
        self.assertTrue(g["pass"])
        self.assertEqual(g["distinct_pairs"], {"baseline": 100, "pairfeat2": 110, "max_allowed": 110.0})
        self.assertFalse(pfc.gate(base, arm(P2, 511, 600), "selection")["pass"])
        self.assertFalse(pfc.gate(arm(P2, 500, 600), arm(P2, 530, 600), "selection")["pass"])
        self.assertFalse(pfc.gate(base, arm("pairfeat6", 530, 600), "selection")["pass"])
        s = pfc.select(pfc_arm, arm(P2, 512, 600))
        self.assertEqual((s["selected_modelset"], s["candidate"], s["pass"]), ("pairfeat2", "pairfeat2", True))
        self.assertIn("pairfeat2_recall", s)
        s = pfc.select(pfc_arm, arm(P2, 511, 600))
        self.assertEqual((s["selected_modelset"], s["candidate"], s["pass"]), (None, "pairfeat2", False))
        self.assertEqual(pfc.select(pfc_arm, arm(P1, 512, 600))["candidate"], "pairfeat1")


class PreflightPairfeat2(unittest.TestCase):
    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.r = Repo(self.td.name, cand=P2)
        self.ap = os.path.join(self.r.exp, "artifact-manifest.json")
        self.vp = os.path.join(self.r.exp, "validation-manifest.json")

    def tearDown(self):
        self.td.cleanup()

    def pf(self, mode, a, head=HEAD):
        return pfc.preflight(mode, a, self.r.exp, self.r.root, head)

    def refused(self, mode, a, msg=None):
        with self.assertRaises(pfc.PreflightError) as cm:
            self.pf(mode, a)
        if msg:
            self.assertIn(msg, str(cm.exception))

    def test_dev_clean_infers_candidate(self):
        good = self.r.artifact_manifest()
        self.assertEqual(self.pf("dev-clean", "baseline"), self.r.hashes(BASE_FILES))
        self.assertEqual(self.pf("dev-clean", P2), self.r.hashes(CAND2_FILES))
        self.assertEqual(pfc.preflight_identity("dev-clean", "baseline", self.r.exp, self.r.root, HEAD),
                         (P2, self.r.hashes(BASE_FILES)))
        self.refused("dev-clean", P1, msg="pairfeat2")  # an arm the parent does not serve
        for f in CAND2_FILES:
            p = os.path.join(self.r.root, "models", f)
            orig = _rb(p)
            with open(p, "ab") as h:
                h.write(b"x")
            self.refused("dev-clean", P2, msg=f)
            self.pf("dev-clean", "baseline")
            with open(p, "wb") as h:
                h.write(orig)
        # zero, multiple or unknown candidate keys refuse every arm
        for m in ({k: v for k, v in good.items() if k != "pairfeat2_chain_sha256"},
                  dict(good, pairfeat1_chain_sha256=self.r.hashes(CAND_FILES)),
                  dict({k: v for k, v in good.items() if k != "pairfeat2_chain_sha256"}, pairfeat6_chain_sha256={})):
            _wj(self.ap, m)
            for a in pfc.ALLOWED_ARMS:
                self.refused("dev-clean", a)
                self.refused("dev-stress", a)
        _wj(self.ap, dict(good, pairfeat2_chain_sha256=self.r.hashes(CAND_FILES)))  # pairfeat1 files under pairfeat2 key
        self.refused("dev-clean", P2)

    def test_dev_stress_selection_must_record_candidate(self):
        self.r.artifact_manifest()
        self.refused("dev-stress", "baseline", msg="selection.json")
        p1map = self.r.hashes(CAND_FILES)
        for kw in ({"candidate": P1}, {"selected_modelset": P1}, {"candidate": None}, {"seeds": [307, 311, 313]},
                   {"pairfeat1_chain_sha256": p1map}, {"pairfeat2_chain_sha256": chain(CAND2_FILES, "x")},
                   {"selected_modelset": None, "pass": False}):
            self.r.selection(**kw)
            for a in pfc.arms(P2):
                self.refused("dev-stress", a)
        s = self.r.selection()
        del s["candidate"]
        _wj(os.path.join(self.r.exp, "selection.json"), s)
        self.refused("dev-stress", "baseline", msg="candidate")
        # a pairfeat1 selection (same layout, pairfeat1 key) in a pairfeat2 parent
        _wj(os.path.join(self.r.exp, "selection.json"),
            {"selected_modelset": P1, "pass": True, "mode": "dev-clean", "seeds": [307, 311, 313], "git_head": HEAD,
             "baseline_chain_sha256": self.r.hashes(BASE_FILES), "pairfeat1_chain_sha256": p1map, "candidate": P1})
        self.refused("dev-stress", "baseline")
        self.r.selection()
        self.assertEqual(self.pf("dev-stress", P2), self.r.hashes(CAND2_FILES))
        self.pf("dev-stress", "baseline")

    def test_freeze_and_validation(self):
        self.r.artifact_manifest()
        self.r.selection()
        for kw in ({"candidate": P1}, {"seeds": [307, 311, 313]}, {"pairfeat1_chain_sha256": self.r.hashes(CAND_FILES)}):
            self.r.stress(**kw)
            with self.assertRaises(pfc.PreflightError):
                pfc.freeze_validation(self.r.exp, self.r.root, HEAD)
            self.assertFalse(os.path.exists(self.vp))
        st = self.r.stress()
        del st["candidate"]
        _wj(os.path.join(self.r.exp, "stress.json"), st)
        with self.assertRaises(pfc.PreflightError):
            pfc.freeze_validation(self.r.exp, self.r.root, HEAD)
        self.r.stress()
        vm = pfc.freeze_validation(self.r.exp, self.r.root, HEAD)
        self.assertEqual(_rj(self.vp), vm)
        self.assertEqual((vm["candidate"], vm["selected_modelset"]), ("pairfeat2", "pairfeat2"))
        self.assertEqual(vm["seeds"], [401, 409, 419, 421, 431])
        self.assertEqual(vm["settings"], pfc.expected_settings("validation-clean", P2))
        self.assertEqual(vm["pairfeat2_chain_sha256"], self.r.hashes(CAND2_FILES))
        self.assertEqual(vm["baseline_chain_sha256"], self.r.hashes(BASE_FILES))
        self.assertNotIn("pairfeat1_chain_sha256", vm)
        self.assertEqual(self.pf("validation-clean", P2), self.r.hashes(CAND2_FILES))
        self.assertEqual(self.pf("validation-clean", "baseline"), self.r.hashes(BASE_FILES))
        self.refused("validation-clean", P1)
        with self.assertRaises(pfc.PreflightError):
            pfc.freeze_validation(self.r.exp, self.r.root, HEAD)  # write-once
        for kw in ({"candidate": P1}, {"candidate": None}, {"selected_modelset": P1},
                   {"settings": pfc.expected_settings("validation-clean", P1)},
                   {"pairfeat1_chain_sha256": self.r.hashes(CAND_FILES)}, {"seeds": [317, 331, 337]}):
            _wj(self.vp, dict(vm, **kw))
            for a in pfc.ALLOWED_ARMS:
                self.refused("validation-clean", a)
        # the parent's artifact manifest rewritten to serve pairfeat1, frozen manifest re-hashed to match: refused
        _wj(self.vp, vm)
        am = _rj(self.ap)
        swapped = {k: v for k, v in am.items() if k != "pairfeat2_chain_sha256"}
        swapped["pairfeat1_chain_sha256"] = self.r.hashes(CAND_FILES)
        _wj(self.ap, swapped)
        _wj(self.vp, dict(vm, artifact_manifest_sha256=_sha(self.ap)))
        for a in pfc.ALLOWED_ARMS:
            self.refused("validation-clean", a)
        _wj(self.ap, am)
        _wj(self.vp, vm)
        self.pf("validation-clean", P2)


class CLIPairfeat2(CLIFixture, unittest.TestCase):
    CAND = P2

    def test_dev_clean_selects_pairfeat2(self):
        self.assertEqual(self.run_cli("dev-clean", self.make_arm("baseline", 5), self.make_arm(P2, 6)), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual((sel["selected_modelset"], sel["candidate"], sel["pass"]), ("pairfeat2", "pairfeat2", True))
        self.assertEqual(sel["seeds"], [317, 331, 337])
        self.assertEqual(sel["pairfeat2_chain_sha256"], self.maps[P2])
        self.assertEqual(sel["baseline_chain_sha256"], self.maps["baseline"])
        self.assertNotIn("pairfeat1_chain_sha256", sel)
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        self.assertEqual((s["candidate"], s["seeds"]), ("pairfeat2", [317, 331, 337]))
        self.assertEqual(sorted(s["arms"]), ["baseline", "pairfeat2"])
        self.assertEqual(sorted(s["arms"]["baseline"]["per_seed"]), ["317", "331", "337"])

    def test_dev_clean_failure_writes_null_selection(self):
        self.assertEqual(self.run_cli("dev-clean", self.make_arm("baseline", 5), self.make_arm(P2, 5)), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual((sel["selected_modelset"], sel["candidate"], sel["pass"]), (None, "pairfeat2", False))

    def test_wrong_candidate_or_identity_rejected(self):
        b = self.make_arm("baseline", 5)
        c = self.make_arm(P2, 6)
        # a pairfeat1-labelled arm cannot be analysed against a pairfeat2 parent
        self.assert_rejected(b, c, cand=P1)
        self.assert_rejected(b, self.make_arm(P2, 6, cfg_modelset="pairfeat1", name="c1"))
        self.assert_rejected(b, self.make_arm(P2, 6, cfg_modelset="", name="c2"))
        self.assert_rejected(self.make_arm("baseline", 5, cfg_modelset="pairfeat2", name="b1"), c)
        self.assert_rejected(self.make_arm("baseline", 5, build_cand=P1, name="b2"), c)
        self.assert_rejected(b, self.make_arm(P2, 6, build_cand=P1, name="c3"))
        self.assert_rejected(b, self.make_arm(P2, 6, chain_map=chain(CAND2_FILES, "x"), name="c4"))
        # lanes run on pairfeat1 development seeds are not this experiment's lanes
        self.assert_rejected(self.make_arm("baseline", 5, seeds_of=P1, name="b3"), self.make_arm(P2, 6, seeds_of=P1, name="c5"))
        out = os.path.join(self.exp, "s.json")
        self.assertEqual(pfc.main(["dev-clean", out, "pairfeat1=" + b, "pairfeat2=" + c]), 2)
        self.assertEqual(pfc.main(["dev-clean", out, "pairfeat2=" + c, "pairfeat2=" + c]), 2)
        self.assertEqual(pfc.main(["dev-clean", out, "baseline=" + b, "pairfeat6=" + c]), 2)
        am = _rj(os.path.join(self.exp, "artifact-manifest.json"))
        for m in (dict(am, pairfeat1_chain_sha256=chain(CAND_FILES, "c")),
                  {k: v for k, v in am.items() if k != "pairfeat2_chain_sha256"}):
            _wj(os.path.join(self.exp, "artifact-manifest.json"), m)
            self.assert_rejected(b, c)
        _wj(os.path.join(self.exp, "artifact-manifest.json"), am)
        self.assertEqual(self.run_cli("dev-clean", b, c), 0)

    def test_dev_stress_and_validation_end_to_end(self):
        r = Repo(os.path.join(self.td.name, "v"), cand=P2)
        self.exp = r.exp
        self.maps = {"baseline": r.hashes(BASE_FILES), P2: r.hashes(CAND2_FILES)}
        r.artifact_manifest()
        b = self.make_arm("baseline", 5, mode="dev-stress")
        c = self.make_arm(P2, 5, mode="dev-stress")
        with self.assertRaises(ValueError):
            self.run_cli("dev-stress", b, c)  # no selection
        sel = r.selection(candidate=P1)
        with self.assertRaises(ValueError):
            self.run_cli("dev-stress", b, c)
        r.selection()
        self.assertEqual(self.run_cli("dev-stress", b, c), 0)
        st = _rj(os.path.join(r.exp, "stress.json"))
        self.assertEqual((st["pass"], st["candidate"], st["seeds"]), (True, "pairfeat2", [317, 331, 337]))
        self.assertEqual(st["selection_sha256"], _sha(os.path.join(r.exp, "selection.json")))
        self.assertNotIn("pairfeat1_chain_sha256", st)
        vm = pfc.freeze_validation(r.exp, r.root, HEAD)
        self.assertEqual(vm["candidate"], "pairfeat2")
        vb = self.make_arm("baseline", 5, mode="validation-clean")
        vc = self.make_arm(P2, 6, mode="validation-clean")
        out = os.path.join(r.exp, "validation-clean-summary.json")
        run = lambda roots: pfc.analyse("validation-clean", out, roots, repo=r.root, head=HEAD)
        with self.assertRaises(ValueError):
            run({"baseline": vb, "pairfeat1": vc})
        vp = os.path.join(r.exp, "validation-manifest.json")
        _wj(vp, dict(vm, candidate=P1))
        with self.assertRaises(ValueError):
            run({"baseline": vb, P2: vc})
        _wj(vp, vm)
        self.assertFalse(os.path.exists(out))
        res = run({"baseline": vb, P2: vc})
        self.assertTrue(res["gate"]["pass"])
        self.assertEqual((res["candidate"], res["seeds"]), ("pairfeat2", [401, 409, 419, 421, 431]))
        with self.assertRaises(FileExistsError):
            run({"baseline": vb, P2: vc})
        del sel


class RunnerPairfeat2(RunnerFixture, unittest.TestCase):
    """Shell runner for a pairfeat2 experiment parent; every case exits before any build."""

    def test_pairfeat2_parent_baseline_uses_pairfeat2_seeds(self):
        self.real_artifact_manifest(cand=P2)
        for mode in ("dev-clean",):
            r = self.run_sh(self.root, mode, "baseline", preflight_only=True)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn("candidate pairfeat2", r.stdout)
            self.assertIn("seeds 317 331 337", r.stdout)
            self.assertFalse(os.path.exists(self.root))
        # candidate artifacts differ from the fake map: refused before any build
        self.assert_refused(self.run_sh(self.root, "dev-clean", P2, preflight_only=True))
        # the pairfeat1 arm is not served by a pairfeat2 parent
        self.assert_refused(self.run_sh(self.root, "dev-clean", P1, preflight_only=True), "pairfeat2")

    def test_pairfeat1_parent_still_uses_pairfeat1_seeds(self):
        self.real_artifact_manifest()
        r = self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("candidate pairfeat1", r.stdout)
        self.assertIn("seeds 307 311 313", r.stdout)
        self.assert_refused(self.run_sh(self.root, "dev-clean", P2, preflight_only=True), "pairfeat1")

    def test_zero_or_multiple_candidate_keys_refused(self):
        base = {f: _sha(os.path.join(REPO, "models", f)) for f in BASE_FILES}
        ap = os.path.join(self.exp, "artifact-manifest.json")
        for m in ({"git_head": self.head, "baseline_chain_sha256": base},
                  {"git_head": self.head, "baseline_chain_sha256": base, "pairfeat1_chain_sha256": chain(CAND_FILES, "c"),
                   "pairfeat2_chain_sha256": chain(CAND2_FILES, "c")},
                  {"git_head": self.head, "baseline_chain_sha256": base, "pairfeat6_chain_sha256": chain(CAND2_FILES, "c")}):
            _wj(ap, m)
            for a in ("baseline", P1, P2):
                self.assert_refused(self.run_sh(self.root, "dev-clean", a, preflight_only=True), "candidate")
        self.assert_refused(self.run_sh(self.root, "dev-clean", "pairfeat6"))

    def test_pairfeat2_dev_stress_and_validation(self):
        self.real_artifact_manifest(cand=P2)
        am = _rj(os.path.join(self.exp, "artifact-manifest.json"))
        ident = {"git_head": self.head, "baseline_chain_sha256": am["baseline_chain_sha256"],
                 "pairfeat2_chain_sha256": am["pairfeat2_chain_sha256"], "candidate": P2}
        sp, tp = os.path.join(self.exp, "selection.json"), os.path.join(self.exp, "stress.json")
        sel = dict(ident, selected_modelset=P2, mode="dev-clean", seeds=[317, 331, 337], **{"pass": True})
        for bad in (dict(sel, candidate=P1), dict(sel, seeds=[307, 311, 313]), dict(sel, selected_modelset=P1)):
            _wj(sp, bad)
            self.assert_refused(self.run_sh(self.root, "dev-stress", "baseline", preflight_only=True))
        _wj(sp, sel)
        r = self.run_sh(self.root, "dev-stress", "baseline", preflight_only=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("seeds 317 331 337", r.stdout)
        _wj(tp, dict(ident, mode="dev-stress", seeds=[317, 331, 337], selection_sha256=_sha(sp), **{"pass": True}))
        ex = lambda n: _sha(os.path.join(self.exp, n))
        vm = dict(ident, mode="validation-clean", seeds=[401, 409, 419, 421, 431], selected_modelset=P2,
                  runner_sha256=_sha(SCRIPT), analyzer_sha256=_sha(os.path.join(HERE, "pairfeat_compare.py")),
                  settings=pfc.expected_settings("validation-clean", P2), artifact_manifest_sha256=ex("artifact-manifest.json"),
                  selection_sha256=ex("selection.json"), stress_sha256=ex("stress.json"))
        vp = os.path.join(self.exp, "validation-manifest.json")
        _wj(vp, dict(vm, candidate=P1))
        self.assert_refused(self.run_sh(self.root, "validation-clean", "baseline", preflight_only=True))
        _wj(vp, vm)
        r = self.run_sh(self.root, "validation-clean", "baseline", preflight_only=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("candidate pairfeat2", r.stdout)
        self.assertIn("seeds 401 409 419 421 431", r.stdout)
        self.assertFalse(os.path.exists(self.root))
        self.assert_refused(self.run_sh(self.root, "validation-clean", P1, preflight_only=True))

    def test_runner_seed_table_matches_analyzer(self):
        with open(SCRIPT) as h:
            text = h.read()
        for c, spec in pfc.CANDIDATES.items():
            self.assertIn('%s) DEV_SEEDS="%s"' % (c, " ".join(map(str, spec["dev_seeds"]))), text)
        self.assertIn('VAL_SEEDS="%s"' % " ".join(map(str, pfc.VALIDATION_SEEDS)), text)
        self.assertIn("pfc.guard(argv, mode, arm, bin_, cand)", text)
        self.assertIn('"candidate": cand', text)


# ================================================================ candidate pairfeat3 (namesake negatives)

P3 = "pairfeat3"
CAND3_FILES = ("encoder_pairfeat3.json", "lgb_stream_pairfeat3.json", "iso_pairfeat3.json", "iso_v7_pairfeat3.json")


class Pairfeat3(unittest.TestCase):
    ROOT = "/repo"
    BIN = "/repo/bin/wsi7"
    O = "/data/exp/arm/v2-s347"

    def cmds(self, mode="dev-clean", arm_="baseline", seed=347):
        return pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, mode, arm_)

    def test_seeds_and_chain(self):
        self.assertEqual(pfc.CANDIDATES[P3], {"dev_seeds": [347, 353, 367], "review_gate": "total"})
        self.assertEqual(pfc.arms(P3), ("baseline", P3))
        self.assertEqual(pfc.CHAIN_FILES[P3], CAND3_FILES)
        self.assertEqual(pfc.CHAIN_KEY[P3], "pairfeat3_chain_sha256")
        self.assertEqual(pfc.MODELSET[P3], P3)
        for mode, seeds in (("dev-clean", [347, 353, 367]), ("dev-stress", [347, 353, 367]),
                            ("validation-clean", [401, 409, 419, 421, 431])):
            self.assertEqual(pfc.mode_settings(mode, P3)["seeds"], seeds)
        self.assertEqual(pfc.expected_settings("validation-clean", P3)["candidate_modelset"], P3)

    def test_infer_candidate_ignores_non_candidate_keys(self):
        b = chain(BASE_FILES, "b")
        ok = {"git_head": HEAD, "baseline_chain_sha256": b, "pairfeat3_chain_sha256": chain(CAND3_FILES, "c"),
              "namesake_slice": {"rows": 5}, "plain_cal_diff_at_merge_0_7": 31, "parity": {}}
        self.assertEqual(pfc.infer_candidate(ok, "artifact-manifest.json"), P3)
        for bad in (dict(ok, pairfeat2_chain_sha256=chain(CAND2_FILES, "c")),
                    dict(ok, pairfeat1_chain_sha256=chain(CAND_FILES, "c"))):
            with self.assertRaises(pfc.PreflightError):
                pfc.infer_candidate(bad, "artifact-manifest.json")
        only_extras = {"baseline_chain_sha256": b, "namesake_slice": {}, "plain_cal_diff_at_merge_0_7": 1}
        with self.assertRaises(pfc.PreflightError):
            pfc.infer_candidate(only_extras, "artifact-manifest.json")

    def test_guard_argv(self):
        for mode in pfc.MODES:
            for seed in pfc.modes(P3)[mode]["seeds"]:
                for a in pfc.arms(P3):
                    for _, argv, _ in self.cmds(mode, a, seed):
                        pfc.guard(argv, mode, a, self.BIN, P3)
        c = {n: argv for n, argv, _ in self.cmds("dev-clean", P3)}
        b = {n: argv for n, argv, _ in self.cmds("dev-clean", "baseline")}
        for n, argv in c.items():
            if n in ("orchestrate", "audit", "report7"):
                self.assertEqual(argv[argv.index("-modelset") + 1], P3)
                self.assertEqual(argv.count("-modelset"), 1)
                self.assertNotIn("-modelset", b[n])
                with self.assertRaises(pfc.GuardError):
                    pfc.guard(list(b[n]) + ["-modelset", P3], "dev-clean", "baseline", self.BIN, P3)
            else:
                self.assertNotIn("-modelset", argv)
                with self.assertRaises(pfc.GuardError):
                    pfc.guard(list(argv) + ["-modelset", P3], "dev-clean", P3, self.BIN, P3)
        # earlier candidates' seeds are not authorised here
        for n, argv, _ in self.cmds("dev-clean", "baseline", 317):
            if n in ("gen", "orchestrate"):
                with self.assertRaises(pfc.GuardError):
                    pfc.guard(argv, "dev-clean", "baseline", self.BIN, P3)

    def test_funnel_modelset(self):
        pfc.check_funnel_modelset({"config": {"modelset": P3}}, P3)
        for f, a in (({"config": {"modelset": "pairfeat2"}}, P3), ({"config": {"modelset": ""}}, P3),
                     ({"config": {"modelset": P3}}, "baseline"), ({"config": {"modelset": P3}}, "pairfeat2"),
                     ({"config": {}}, P3)):
            with self.assertRaises(ValueError):
                pfc.check_funnel_modelset(f, a)

    def test_runner_arm_and_seeds(self):
        with open(SCRIPT) as h:
            text = h.read()
        self.assertIn('pairfeat3) DEV_SEEDS="347 353 367";;', text)
        self.assertIn("baseline|pairfeat1|pairfeat2|pairfeat3", text)



# ================================================================ candidate pairfeat4 (symmetric namesakes, wrong-pair gate)

P4 = "pairfeat4"
CAND4_FILES = ("encoder_pairfeat4.json", "lgb_stream_pairfeat4.json", "iso_pairfeat4.json", "iso_v7_pairfeat4.json")
FILES[P3] = CAND3_FILES
FILES[P4] = CAND4_FILES
DEV_SEEDS[P4] = [433, 449, 461]
P4_EXTRAS = {"namesake_slice": {"rows": 5}, "family_namesake_slice": {"rows": 7}, "plain_cal_diff_at_merge_0_7": 19,
             "ns_cal_sidecar_sha256": "3" * 64}
# fixture truth map: r1/r2 are one person, every other registration is its own person
TRUTH = {"r1": "P1", "r2": "P1", "r3": "P3", "r4": "P4", "r5": "P5", "r6": "P6"}


def warm(name, tp, truth, wrong, distinct_pairs=100, hours=1.0, **kw):
    """An aggregate arm carrying wrong_distinct_pairs (as a wrong_pair experiment's lanes do)."""
    a = arm(name, tp, truth, distinct_pairs=distinct_pairs, **kw)
    a["reviewer"] = dict(a["reviewer"], wrong_distinct_pairs=wrong, hours=hours)
    return a


def queue(same=0, wrong=0, dup=True):
    """A review queue with `same` same-person and `wrong` different-person distinct link-review pairs (plus duplicates and
    items the load ignores), and the matching truth map."""
    pk = {}
    q = []
    for i in range(same):
        a, b = "s%d|a" % i, "s%d|b" % i
        pk[a] = pk[b] = "S%d" % i
        q.append({"item": "s%d" % i, "kinds": ["link"], "pair": [a, b]})
    for i in range(wrong):
        a, b = "w%d|a" % i, "w%d|b" % i
        pk[a], pk[b] = "WA%d" % i, "WB%d" % i
        q.append({"item": "w%d" % i, "kinds": ["link"], "pair": [a, b]})
        if dup:
            q.append({"item": "w%d-dup" % i, "kinds": ["link"], "pair": [b, a]})
    return q, pk


class Pairfeat4(unittest.TestCase):
    ROOT = "/repo"
    BIN = "/repo/bin/wsi7"
    O = "/data/exp/arm/v2-s433"

    def cmds(self, mode="dev-clean", arm_="baseline", seed=433):
        return pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, mode, arm_)

    def test_seeds_chain_and_review_gate(self):
        self.assertEqual(pfc.CANDIDATES[P4], {"dev_seeds": [433, 449, 461], "review_gate": "wrong_pair"})
        for c in (P1, P2, P3):
            self.assertEqual(pfc.CANDIDATES[c]["review_gate"], "total")
            self.assertEqual(pfc.review_gate(c), "total")
        self.assertEqual(pfc.review_gate(P4), "wrong_pair")
        for bad in ("baseline", "pairfeat6", "", None):
            with self.assertRaises(ValueError):
                pfc.review_gate(bad)
        self.assertEqual(pfc.arms(P4), ("baseline", P4))
        self.assertEqual(pfc.CHAIN_FILES[P4], CAND4_FILES)
        self.assertEqual(pfc.CHAIN_KEY[P4], "pairfeat4_chain_sha256")
        self.assertEqual(pfc.MODELSET[P4], P4)
        for mode, seeds in (("dev-clean", [433, 449, 461]), ("dev-stress", [433, 449, 461]),
                            ("validation-clean", [401, 409, 419, 421, 431])):
            self.assertEqual(pfc.mode_settings(mode, P4)["seeds"], seeds)
        v4 = pfc.expected_settings("validation-clean", P4)
        self.assertEqual((v4["candidate_modelset"], v4["arms"]), (P4, ["baseline", P4]))
        self.assertEqual({k: v for k, v in v4.items() if k not in ("candidate_modelset", "arms")},
                         {k: v for k, v in pfc.expected_settings("validation-clean", P3).items()
                          if k not in ("candidate_modelset", "arms")})

    def test_infer_candidate_ignores_non_candidate_keys(self):
        b = chain(BASE_FILES, "b")
        ok = dict(P4_EXTRAS, git_head=HEAD, baseline_chain_sha256=b, pairfeat4_chain_sha256=chain(CAND4_FILES, "c"))
        self.assertEqual(pfc.infer_candidate(ok, "artifact-manifest.json"), P4)
        for bad in (dict(ok, pairfeat3_chain_sha256=chain(CAND3_FILES, "c")),
                    dict(ok, pairfeat6_chain_sha256=chain(CAND4_FILES, "c")),
                    dict(P4_EXTRAS, baseline_chain_sha256=b)):
            with self.assertRaises(pfc.PreflightError):
                pfc.infer_candidate(bad, "artifact-manifest.json")

    def test_guard_argv(self):
        for mode in pfc.MODES:
            for seed in pfc.modes(P4)[mode]["seeds"]:
                for a in pfc.arms(P4):
                    for _, argv, _ in self.cmds(mode, a, seed):
                        pfc.guard(argv, mode, a, self.BIN, P4)
        c = {n: argv for n, argv, _ in self.cmds("dev-clean", P4)}
        b = {n: argv for n, argv, _ in self.cmds("dev-clean", "baseline")}
        for n, argv in c.items():
            if n in ("orchestrate", "audit", "report7"):
                self.assertEqual(argv[argv.index("-modelset") + 1], P4)
                self.assertEqual(argv.count("-modelset"), 1)
                i = argv.index("-modelset")
                self.assertEqual(argv[:i] + argv[i + 2:], b[n])
                with self.assertRaises(pfc.GuardError):
                    pfc.guard(list(b[n]) + ["-modelset", P4], "dev-clean", "baseline", self.BIN, P4)
                with self.assertRaises(pfc.GuardError):
                    pfc.guard(self.setv(list(argv), "-modelset", P3), "dev-clean", P4, self.BIN, P4)
                with self.assertRaises(pfc.GuardError):
                    pfc.guard(self.setv(list(argv), "-policy", "v7ltt"), "dev-clean", P4, self.BIN, P4)
            else:
                self.assertEqual(argv, b[n])
                self.assertNotIn("-modelset", argv)
                with self.assertRaises(pfc.GuardError):
                    pfc.guard(list(argv) + ["-modelset", P4], "dev-clean", P4, self.BIN, P4)
        # other candidates' development seeds are not authorised here, and pairfeat4 seeds are not authorised elsewhere
        for seed, other in ((347, P3), (317, P2), (307, P1)):
            for n, argv, _ in self.cmds("dev-clean", "baseline", seed):
                if n in ("gen", "orchestrate"):
                    with self.assertRaises(pfc.GuardError):
                        pfc.guard(argv, "dev-clean", "baseline", self.BIN, P4)
            for n, argv, _ in self.cmds("dev-clean", "baseline", 433):
                if n in ("gen", "orchestrate"):
                    with self.assertRaises(pfc.GuardError):
                        pfc.guard(argv, "dev-clean", "baseline", self.BIN, other)
        with self.assertRaises(pfc.GuardError):
            pfc.guard(c["orchestrate"], "dev-clean", P4, self.BIN, P3)

    def setv(self, argv, flag, v):
        argv[argv.index(flag) + 1] = v
        return argv

    def test_funnel_modelset(self):
        pfc.check_funnel_modelset({"config": {"modelset": P4}}, P4)
        for f, a in (({"config": {"modelset": P3}}, P4), ({"config": {"modelset": ""}}, P4),
                     ({"config": {"modelset": P4}}, "baseline"), ({"config": {"modelset": P4}}, P3),
                     ({"config": {}}, P4)):
            with self.assertRaises(ValueError):
                pfc.check_funnel_modelset(f, a)

    def test_runner_arm_and_seeds(self):
        with open(SCRIPT) as h:
            text = h.read()
        self.assertIn('pairfeat4) DEV_SEEDS="433 449 461";;', text)
        self.assertIn("baseline|pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5) ;;", text)
        self.assertIn("pfc._load_lane(os.path.join(work, \"v2-s%d\" % seed), seed, arm, mode, cand)", text)


class WrongPairLoad(unittest.TestCase):
    Q = [{"item": "a", "kinds": ["link"], "pair": ["r1", "r2"]},                      # same person
         {"item": "b", "kinds": ["link"], "pair": ["r3", "r4"]},                      # different persons
         {"item": "c", "kinds": ["link"], "pair": ["r4", "r3"]},                      # duplicate of b
         {"item": "d", "kinds": ["link"], "pair": ["r5", "r6"], "reason": "second_identifier"},  # not link review
         {"item": "e", "kinds": ["other"], "pair": ["r1", "r5"]},                     # not link review
         {"item": "f", "kinds": ["link"], "pair": ["r6", "r1"]}]                      # different persons

    def test_wrong_distinct_pairs(self):
        self.assertEqual(pfc.review_load(self.Q, [{"item": "a"}], TRUTH),
                         {"items": 4, "distinct_pairs": 3, "unanswered": 3, "wrong_distinct_pairs": 2})
        # without a truth map the load is exactly the total-gate load
        self.assertEqual(pfc.review_load(self.Q, [{"item": "a"}]), {"items": 4, "distinct_pairs": 3, "unanswered": 3})
        self.assertEqual(pfc.review_load([], [], TRUTH), {"items": 0, "distinct_pairs": 0, "unanswered": 0,
                                                          "wrong_distinct_pairs": 0})
        q, pk = queue(same=3, wrong=2)
        self.assertEqual(pfc.review_load(q, [], pk)["wrong_distinct_pairs"], 2)
        self.assertEqual(pfc.review_load(q, [], pk)["distinct_pairs"], 5)

    def test_unmapped_registration_is_an_error(self):
        for bad in ([{"item": "g", "kinds": ["link"], "pair": ["r1", "ghost"]}],
                    [{"item": "g", "kinds": ["link"], "pair": ["ghost", "r1"]}],
                    [{"item": "g", "kinds": ["link"], "pair": ["r1", "rx"]}]):
            with self.assertRaises(ValueError):
                pfc.review_load(self.Q + bad, [], dict(TRUTH, rx=None))
        for pk in ([], {"r1": "P1"}):  # not a map; r2..r6 unmapped
            with self.assertRaises(ValueError):
                pfc.review_load(self.Q, [], pk)
        with self.assertRaises(ValueError):  # a link item must name exactly two registrations
            pfc.review_load([{"item": "g", "kinds": ["link"], "pair": ["r1", "r2", "r3"]}], [], TRUTH)
        # registrations on items the load ignores are not checked
        extra = [{"item": "h", "kinds": ["other"], "pair": ["ghost", "r1"]},
                 {"item": "i", "kinds": ["link"], "pair": ["ghost", "r1"], "reason": "second_identifier"}]
        self.assertEqual(pfc.review_load(self.Q + extra, [], TRUTH)["wrong_distinct_pairs"], 2)

    def test_lane_metrics_and_aggregate(self):
        load = dict(LOAD, wrong_distinct_pairs=2)
        m = pfc.lane_metrics(1, report(), funnel(5, 10), {}, load)
        self.assertEqual(m["reviewer"]["wrong_distinct_pairs"], 2)
        plain = pfc.lane_metrics(1, report(), funnel(5, 10), {}, LOAD)
        self.assertNotIn("wrong_distinct_pairs", plain["reviewer"])
        self.assertEqual(set(plain["reviewer"]), {"items", "hours", "wrong_final_signoffs", "distinct_pairs", "unanswered",
                                                  "link_items", "by_programme", "by_origin"})
        self.assertEqual({k: v for k, v in m["reviewer"].items() if k != "wrong_distinct_pairs"}, plain["reviewer"])
        lanes = [pfc.lane_metrics(s, report(), funnel(5, 10), {}, dict(LOAD, wrong_distinct_pairs=w))
                 for s, w in ((433, 1), (449, 0), (461, 3))]
        a = pfc.aggregate_arm(P4, lanes)
        self.assertEqual(a["reviewer"]["wrong_distinct_pairs"], 4)
        self.assertEqual(a["reviewer"]["distinct_pairs"], 18)
        p = pfc.aggregate_arm("baseline", [pfc.lane_metrics(s, report(), funnel(5, 10), {}, LOAD) for s in (347, 353, 367)])
        self.assertEqual(set(p["reviewer"]), set(pfc.REVIEWER_SUMS))
        with self.assertRaises(ValueError):  # all lanes or none
            pfc.aggregate_arm(P4, lanes[:2] + [pfc.lane_metrics(461, report(), funnel(5, 10), {}, LOAD)])


class WrongPairGate(unittest.TestCase):
    def test_threshold_exactly_ten_percent(self):
        for kind in pfc.GATE_KINDS:
            b = warm("baseline", 500, 600, wrong=100)
            g = pfc.gate(b, warm(P4, 530, 600, wrong=110), kind)
            self.assertTrue(g["pass"], (kind, g["reasons"]))
            self.assertEqual(g["review_gate"], "wrong_pair")
            self.assertEqual(g["wrong_distinct_pairs"], {"baseline": 100, P4: 110, "max_allowed": 110.0})
            g = pfc.gate(b, warm(P4, 530, 600, wrong=111), kind)
            self.assertFalse(g["pass"], kind)
            self.assertEqual(g["reasons"], ["wrong-pair link-review pairs 111 exceed 110.00 (baseline 100 +10%)"])
            b = warm("baseline", 500, 600, wrong=10)
            self.assertTrue(pfc.gate(b, warm(P4, 530, 600, wrong=11), kind)["pass"], kind)
            self.assertFalse(pfc.gate(b, warm(P4, 530, 600, wrong=12), kind)["pass"], kind)
            self.assertTrue(pfc.gate(b, warm(P4, 530, 600, wrong=0), kind)["pass"], kind)

    def test_zero_baseline_allows_one(self):
        for kind in pfc.GATE_KINDS:
            b = warm("baseline", 500, 600, wrong=0)
            for cw, ok in ((0, True), (1, True), (2, False)):
                g = pfc.gate(b, warm(P4, 530, 600, wrong=cw), kind)
                self.assertEqual(g["pass"], ok, (kind, cw, g["reasons"]))
                self.assertEqual(g["wrong_distinct_pairs"]["max_allowed"], 1.0)
            self.assertEqual(pfc.gate(b, warm(P4, 530, 600, wrong=2), kind)["reasons"],
                             ["wrong-pair link-review pairs 2 exceed 1.00 (baseline 0 +10%)"])

    def test_total_overage_alone_does_not_fail_pairfeat4_but_still_fails_pairfeat3(self):
        for kind in pfc.GATE_KINDS:
            b = warm("baseline", 500, 600, wrong=10, distinct_pairs=100, hours=2.0)
            g = pfc.gate(b, warm(P4, 530, 600, wrong=10, distinct_pairs=250, hours=5.0), kind)
            self.assertTrue(g["pass"], (kind, g["reasons"]))
            self.assertEqual(g["distinct_pairs"], {"baseline": 100, P4: 250, "ratio": 2.5, "old_gate_max_allowed": 110.0,
                                                   "gating": False})
            # the same numbers under the pairfeat3 (total) gate: unchanged behaviour and output shape
            g3 = pfc.gate(b, warm(P3, 530, 600, wrong=10, distinct_pairs=250, hours=5.0), kind)
            self.assertFalse(g3["pass"])
            self.assertEqual(g3["reasons"], ["distinct link-review pairs 250 exceed 110.00 (baseline 100 +10%)"])
            self.assertEqual(set(g3), {"pass", "kind", "gain_pp", "min_gain_pp", "distinct_pairs", "reasons"})
            self.assertEqual(g3["distinct_pairs"], {"baseline": 100, P3: 250, "max_allowed": 110.0})
            # pairfeat3 ignores wrong pairs: a wrong-pair overage alone does not fail it
            self.assertTrue(pfc.gate(b, warm(P3, 530, 600, wrong=50, distinct_pairs=110), kind)["pass"], kind)
            self.assertEqual(pfc.gate(b, warm(P3, 530, 600, wrong=50, distinct_pairs=110), kind),
                             pfc.gate(arm("baseline", 500, 600), arm(P3, 530, 600, distinct_pairs=110), kind))

    def test_extra_hours_per_extra_closure(self):
        b = warm("baseline", 500, 600, wrong=10, hours=2.0)
        self.assertEqual(pfc.gate(b, warm(P4, 512, 600, wrong=10, hours=5.0), "selection")["extra_hours_per_extra_closure"],
                         0.25)
        self.assertEqual(pfc.gate(b, warm(P4, 520, 600, wrong=10, hours=1.0), "selection")["extra_hours_per_extra_closure"],
                         -0.05)
        for tp in (500, 490):  # tp does not rise: null
            g = pfc.gate(b, warm(P4, tp, 600, wrong=10, hours=5.0), "stress")
            self.assertIsNone(g["extra_hours_per_extra_closure"])
            self.assertIn("extra_hours_per_extra_closure", g)

    def test_gains_safety_and_contamination_still_apply(self):
        b = warm("baseline", 500, 600, wrong=10)
        self.assertFalse(pfc.gate(b, warm(P4, 511, 600, wrong=10), "selection")["pass"])
        self.assertTrue(pfc.gate(b, warm(P4, 506, 600, wrong=10), "validation")["pass"])
        self.assertFalse(pfc.gate(b, warm(P4, 505, 600, wrong=10), "validation")["pass"])
        self.assertFalse(pfc.gate(b, warm(P4, 499, 600, wrong=10), "stress")["pass"])
        for k in pfc.SAFETY + pfc.CONTAM:
            self.assertFalse(pfc.gate(b, warm(P4, 530, 600, wrong=10, **{k: 1}), "selection")["pass"], k)
        self.assertFalse(pfc.gate(warm("baseline", 500, 600, wrong=10, false_closures=1), warm(P4, 530, 600, wrong=10),
                                  "selection")["pass"])
        self.assertFalse(pfc.gate(b, warm(P4, 530, 601, wrong=10), "selection")["pass"])

    def test_missing_wrong_pairs_is_an_error(self):
        for b, c in ((arm("baseline", 500, 600), warm(P4, 530, 600, wrong=1)),
                     (warm("baseline", 500, 600, wrong=1), arm(P4, 530, 600))):
            with self.assertRaises(ValueError):
                pfc.gate(b, c, "selection")

    def test_selection_record_uses_the_candidate_gate(self):
        b = warm("baseline", 500, 600, wrong=10, distinct_pairs=100)
        s = pfc.select(b, warm(P4, 512, 600, wrong=11, distinct_pairs=300))
        self.assertEqual((s["selected_modelset"], s["candidate"], s["pass"]), (P4, P4, True))
        self.assertEqual(s["gate"]["review_gate"], "wrong_pair")
        self.assertIn("pairfeat4_recall", s)
        s = pfc.select(b, warm(P4, 512, 600, wrong=12, distinct_pairs=100))
        self.assertEqual((s["selected_modelset"], s["pass"]), (None, False))


class CLIPairfeat4(CLIFixture, unittest.TestCase):
    CAND = P4

    def make_arm(self, a, tp, *args, same=4, wrong=2, truth=True, unmapped=False, **kw):
        root = super().make_arm(a, tp, *args, **kw)
        for d in sorted(os.listdir(root)):
            if not d.startswith("v2-s"):
                continue
            q, pk = queue(same=same, wrong=wrong)
            if unmapped:
                q.append({"item": "ghost", "kinds": ["link"], "pair": ["ghost|a", next(iter(pk))]})
            _wj(os.path.join(root, d, "run/main/review_queue.json"), q)
            if truth:
                os.makedirs(os.path.join(root, d, "cohort"))
                _wj(os.path.join(root, d, "cohort/truth.json"), {"pk_qid": pk, "loops": {}})
        return root

    def setUp(self):
        super().setUp()
        _wj(os.path.join(self.exp, "artifact-manifest.json"),
            dict(P4_EXTRAS, git_head=HEAD, baseline_chain_sha256=self.maps["baseline"], pairfeat4_chain_sha256=self.maps[P4]))

    def test_dev_clean_selects_on_wrong_pair_gate(self):
        # candidate reviews many more same-person pairs (total x3) but no more different-person pairs: selected
        self.assertEqual(self.run_cli("dev-clean", self.make_arm("baseline", 5, same=2, wrong=2),
                                      self.make_arm(P4, 6, same=8, wrong=2)), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual((sel["selected_modelset"], sel["candidate"], sel["pass"]), (P4, P4, True))
        self.assertEqual(sel["seeds"], [433, 449, 461])
        self.assertEqual(sel["gate"]["wrong_distinct_pairs"], {"baseline": 6, P4: 6, "max_allowed": 6.6})
        self.assertEqual(sel["gate"]["distinct_pairs"]["ratio"], 2.5)
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        self.assertEqual(s["arms"][P4]["reviewer"]["wrong_distinct_pairs"], 6)
        self.assertEqual(s["arms"]["baseline"]["per_seed"]["433"]["reviewer"]["wrong_distinct_pairs"], 2)

    def test_dev_clean_wrong_pair_overage_fails(self):
        self.assertEqual(self.run_cli("dev-clean", self.make_arm("baseline", 5, wrong=2),
                                      self.make_arm(P4, 6, wrong=3)), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual((sel["selected_modelset"], sel["pass"]), (None, False))
        self.assertEqual(sel["gate"]["reasons"], ["wrong-pair link-review pairs 9 exceed 6.60 (baseline 6 +10%)"])

    def test_truth_required_and_unmapped_registration_rejected(self):
        b = self.make_arm("baseline", 5)
        self.assert_rejected(b, self.make_arm(P4, 6, truth=False, name="c1"))
        self.assert_rejected(b, self.make_arm(P4, 6, unmapped=True, name="c2"))
        self.assert_rejected(self.make_arm("baseline", 5, unmapped=True, name="b1"), self.make_arm(P4, 6, name="c3"))
        self.assertEqual(self.run_cli("dev-clean", b, self.make_arm(P4, 6, name="c4")), 0)

    def test_dev_stress_uses_wrong_pair_gate(self):
        sel = {"selected_modelset": P4, "pass": True, "mode": "dev-clean", "seeds": [433, 449, 461], "git_head": HEAD,
               "baseline_chain_sha256": self.maps["baseline"], "pairfeat4_chain_sha256": self.maps[P4], "candidate": P4}
        _wj(os.path.join(self.exp, "selection.json"), sel)
        self.assertEqual(self.run_cli("dev-stress", self.make_arm("baseline", 5, mode="dev-stress", same=1),
                                      self.make_arm(P4, 5, mode="dev-stress", same=9)), 0)
        st = _rj(os.path.join(self.exp, "stress.json"))
        self.assertTrue(st["pass"], st["gate"]["reasons"])
        self.assertEqual(st["gate"]["review_gate"], "wrong_pair")
        self.assertIsNone(st["gate"]["extra_hours_per_extra_closure"])

    def test_validation_uses_wrong_pair_gate(self):
        r = Repo(os.path.join(self.td.name, "v"), cand=P4)
        for f in CAND4_FILES:
            with open(os.path.join(r.root, "models", f), "w") as h:
                h.write("model " + f)
        self.exp = r.exp
        self.maps = {"baseline": r.hashes(BASE_FILES), P4: r.hashes(CAND4_FILES)}
        r.artifact_manifest(**P4_EXTRAS)
        r.selection()
        r.stress()
        vm = pfc.freeze_validation(r.exp, r.root, HEAD)
        self.assertEqual((vm["candidate"], vm["seeds"]), (P4, [401, 409, 419, 421, 431]))
        self.assertEqual(pfc.preflight("validation-clean", P4, r.exp, r.root, HEAD), r.hashes(CAND4_FILES))
        out = os.path.join(r.exp, "validation-clean-summary.json")
        res = pfc.analyse("validation-clean", out, {"baseline": self.make_arm("baseline", 5, mode="validation-clean", same=1),
                                                    P4: self.make_arm(P4, 6, mode="validation-clean", same=9)},
                          repo=r.root, head=HEAD)
        self.assertTrue(res["gate"]["pass"], res["gate"]["reasons"])
        self.assertEqual(res["gate"]["review_gate"], "wrong_pair")
        self.assertEqual(res["gate"]["wrong_distinct_pairs"], {"baseline": 10, P4: 10, "max_allowed": 11.0})
        self.assertAlmostEqual(res["gate"]["extra_hours_per_extra_closure"], 0.0)
        with self.assertRaises(FileExistsError):
            pfc.analyse("validation-clean", out, {"baseline": os.path.join(r.exp, "validation-clean-baseline"),
                                                  P4: os.path.join(r.exp, "validation-clean-" + P4)}, repo=r.root, head=HEAD)


class CLIPairfeat3Unchanged(CLIFixture, unittest.TestCase):
    """A total-gate experiment never reads cohort/truth.json and emits no wrong-pair fields."""
    CAND = P3

    def test_dev_clean_without_truth(self):
        self.assertEqual(self.run_cli("dev-clean", self.make_arm("baseline", 5), self.make_arm(P3, 6)), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual((sel["selected_modelset"], sel["pass"]), (P3, True))
        self.assertEqual(sel["gate"], {"pass": True, "kind": "selection", "gain_pp": 10.0, "min_gain_pp": 2.0,
                                       "distinct_pairs": {"baseline": 0, P3: 0, "max_allowed": 0.0}, "reasons": []})
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        for a in ("baseline", P3):
            self.assertEqual(set(s["arms"][a]["reviewer"]), set(pfc.REVIEWER_SUMS))
            self.assertNotIn("wrong_distinct_pairs", s["arms"][a]["per_seed"]["347"]["reviewer"])


class RunnerPairfeat4(RunnerFixture, unittest.TestCase):
    def test_pairfeat4_parent_uses_pairfeat4_seeds(self):
        self.real_artifact_manifest(cand=P4, **P4_EXTRAS)
        r = self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("candidate pairfeat4", r.stdout)
        self.assertIn("seeds 433 449 461", r.stdout)
        self.assertFalse(os.path.exists(self.root))
        self.assert_refused(self.run_sh(self.root, "dev-clean", P4, preflight_only=True))  # fake candidate map
        self.assert_refused(self.run_sh(self.root, "dev-clean", P3, preflight_only=True), "pairfeat4")


# ================================================================ candidate pairfeat5 (regularised model, capped-pair report)

P5 = "pairfeat5"
CAND5_FILES = ("encoder_pairfeat5.json", "lgb_stream_pairfeat5.json", "iso_pairfeat5.json", "iso_v7_pairfeat5.json")
FILES[P5] = CAND5_FILES
DEV_SEEDS[P5] = [521, 541, 547]


def trace_lines(recs):
    return "".join(json.dumps(r) + "\n" for r in recs)


def cap_rec(a, b, **kw):
    return dict({"kind": "pair", "a": a, "b": b, "stage": "model", "p": 0.69, "reason": "xscript_cap"}, **kw)


class Pairfeat5(unittest.TestCase):
    ROOT = "/repo"
    BIN = "/repo/bin/wsi7"
    O = "/data/exp/arm/v2-s521"

    def cmds(self, mode="dev-clean", arm_="baseline", seed=521):
        return pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, mode, arm_)

    def test_seeds_chain_and_gate(self):
        self.assertEqual(pfc.CANDIDATES[P5], {"dev_seeds": [521, 541, 547], "review_gate": "wrong_pair_cap",
                                              "report_xscript_cap": True})
        for c in (P1, P2, P3, P4):
            self.assertNotIn("report_xscript_cap", pfc.CANDIDATES[c])
        self.assertEqual(pfc.review_gate(P5), "wrong_pair_cap")
        self.assertEqual(pfc.review_gate(P4), "wrong_pair")
        self.assertEqual(pfc.CAP_WRONG_PER_PREVENTED, 2)
        self.assertEqual(pfc.arms(P5), ("baseline", P5))
        self.assertEqual(pfc.CHAIN_FILES[P5], CAND5_FILES)
        self.assertEqual(pfc.CHAIN_KEY[P5], "pairfeat5_chain_sha256")
        self.assertEqual(pfc.MODELSET[P5], P5)
        for mode, seeds in (("dev-clean", [521, 541, 547]), ("dev-stress", [521, 541, 547]),
                            ("validation-clean", [401, 409, 419, 421, 431])):
            self.assertEqual(pfc.mode_settings(mode, P5)["seeds"], seeds)

    def test_infer_candidate(self):
        b = chain(BASE_FILES, "b")
        ok = {"git_head": HEAD, "baseline_chain_sha256": b, "pairfeat5_chain_sha256": chain(CAND5_FILES, "c")}
        self.assertEqual(pfc.infer_candidate(ok, "artifact-manifest.json"), P5)
        with self.assertRaises(pfc.PreflightError):
            pfc.infer_candidate(dict(ok, pairfeat4_chain_sha256=chain(CAND4_FILES, "c")), "artifact-manifest.json")

    def test_guard_argv(self):
        for mode in pfc.MODES:
            for seed in pfc.modes(P5)[mode]["seeds"]:
                for a in pfc.arms(P5):
                    for _, argv, _ in self.cmds(mode, a, seed):
                        pfc.guard(argv, mode, a, self.BIN, P5)
        c = {n: argv for n, argv, _ in self.cmds("dev-clean", P5)}
        for n in ("orchestrate", "audit", "report7"):
            self.assertEqual(c[n][c[n].index("-modelset") + 1], P5)
        with self.assertRaises(pfc.GuardError):
            pfc.guard(c["orchestrate"], "dev-clean", P5, self.BIN, P4)
        for n, argv, _ in self.cmds("dev-clean", "baseline", 433):
            if n in ("gen", "orchestrate"):
                with self.assertRaises(pfc.GuardError):
                    pfc.guard(argv, "dev-clean", "baseline", self.BIN, P5)

    def test_runner_arm_and_seeds(self):
        with open(SCRIPT) as h:
            text = h.read()
        self.assertIn('pairfeat5) DEV_SEEDS="521 541 547";;', text)
        self.assertIn("baseline|pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5) ;;", text)


class XscriptCount(unittest.TestCase):
    PK = {"r1": "P1", "r2": "P1", "r3": "P3", "r4": "P4", "r5": "P5"}

    def test_counts_distinct_sorted_pairs(self):
        recs = [cap_rec("r1", "r2"), cap_rec("r2", "r1"),                      # same person, duplicate (reversed)
                cap_rec("r3", "r4"), cap_rec("r3", "r4", stage="review"),       # different, duplicate
                cap_rec("r4", "r5"),                                            # different
                cap_rec("r1", "r3", reason="other"),                            # other reason
                {"kind": "pair", "a": "r1", "b": "r5", "stage": "model", "p": 0.9},   # no reason
                {"kind": "cluster", "a": "r1", "b": "r5", "reason": "xscript_cap"},    # not a pair record
                {"kind": "pair", "a": "ghost", "b": "r5", "reason": "other"}]  # unmapped but not capped: ignored
        self.assertEqual(pfc.xscript_cap_counts(trace_lines(recs).splitlines(True), self.PK),
                         {"pairs": 3, "same_person": 1, "different_person": 2})
        self.assertEqual(pfc.xscript_cap_counts([], self.PK), {"pairs": 0, "same_person": 0, "different_person": 0})

    def test_unmapped_capped_registration_is_an_error(self):
        for bad in (cap_rec("r1", "ghost"), cap_rec("ghost", "r1")):
            with self.assertRaises(ValueError):
                pfc.xscript_cap_counts(trace_lines([cap_rec("r1", "r2"), bad]).splitlines(True), self.PK)
        with self.assertRaises(ValueError):
            pfc.xscript_cap_counts(trace_lines([cap_rec("r1", "r2")]).splitlines(True), {"r1": "P1"})

    def test_lane_metrics_and_aggregate(self):
        xs = {"pairs": 3, "same_person": 1, "different_person": 2}
        m = pfc.lane_metrics(1, report(), funnel(5, 10), {}, dict(LOAD, wrong_distinct_pairs=0), xs)
        self.assertEqual(m["xscript_cap"], xs)
        plain = pfc.lane_metrics(1, report(), funnel(5, 10), {}, dict(LOAD, wrong_distinct_pairs=0))
        self.assertNotIn("xscript_cap", plain)
        lanes = [pfc.lane_metrics(s, report(), funnel(5, 10), {}, dict(LOAD, wrong_distinct_pairs=0),
                                  {"pairs": n, "same_person": 1, "different_person": n - 1})
                 for s, n in ((521, 2), (541, 3), (547, 4))]
        self.assertEqual(pfc.aggregate_arm(P5, lanes)["xscript_cap"], {"pairs": 9, "same_person": 3, "different_person": 6})
        with self.assertRaises(ValueError):
            pfc.aggregate_arm(P5, lanes[:2] + [pfc.lane_metrics(547, report(), funnel(5, 10), {},
                                                               dict(LOAD, wrong_distinct_pairs=0))])


class CLIPairfeat5(CLIFixture, unittest.TestCase):
    CAND = P5

    def make_arm(self, a, tp, *args, caps=None, truth=True, unmapped=False, recs=None, fm=None, wrong=1, **kw):
        root = super().make_arm(a, tp, *args, **kw)
        for d in sorted(os.listdir(root)):
            if not d.startswith("v2-s"):
                continue
            q, pk = queue(same=1, wrong=wrong)
            pk.update({"c1": "C1", "c2": "C1", "c3": "C3", "c4": "C4"})
            _wj(os.path.join(root, d, "run/main/review_queue.json"), q)
            os.makedirs(os.path.join(root, d, "cohort"))
            _wj(os.path.join(root, d, "cohort/truth.json"), {"pk_qid": pk, "loops": {}})
            if caps is not None or recs is not None:
                r = recs if recs is not None else [cap_rec("c1", "c2"), cap_rec("c2", "c1"), cap_rec("c3", "c4"),
                                                   cap_rec("c4", "c3", stage="x")]
                if unmapped:
                    r = r + [cap_rec("c1", "ghost")]
                with open(os.path.join(root, d, "run/main/link_funnel.ndjson"), "w") as h:
                    h.write(trace_lines(r))
            if fm is not None:
                rp = os.path.join(root, d, "run/main/report7.json")
                rj = _rj(rp)
                rj["linkage"]["false_merge_pairs"] = fm
                _wj(rp, rj)
        return root

    def setUp(self):
        super().setUp()
        _wj(os.path.join(self.exp, "artifact-manifest.json"),
            {"git_head": HEAD, "baseline_chain_sha256": self.maps["baseline"], "pairfeat5_chain_sha256": self.maps[P5]})

    def test_candidate_reports_caps_baseline_does_not(self):
        self.assertEqual(self.run_cli("dev-clean", self.make_arm("baseline", 5, recs=[]),
                                      self.make_arm(P5, 6, caps=[])), 0)
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        self.assertEqual(s["arms"][P5]["xscript_cap"], {"pairs": 6, "same_person": 3, "different_person": 3})
        self.assertEqual(s["arms"][P5]["per_seed"]["521"]["xscript_cap"], {"pairs": 2, "same_person": 1, "different_person": 1})
        self.assertNotIn("xscript_cap", s["arms"]["baseline"])
        self.assertNotIn("xscript_cap", s["arms"]["baseline"]["per_seed"]["521"])
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertNotIn("xscript_cap", json.dumps(sel["gate"]))
        self.assertEqual(sel["gate"]["review_gate"], "wrong_pair_cap")
        self.assertEqual(sel["gate"]["uncapped_wrong_distinct_pairs"]["baseline"], 3)
        self.assertEqual(s["arms"]["baseline"]["reviewer"]["capped_wrong_distinct_pairs"], 0)
        # fixture: each lane has wrong pair w0 (uncapped) and capped different-person pair c3|c4 (not in the queue)
        self.assertEqual(s["arms"][P5]["reviewer"]["capped_wrong_distinct_pairs"], 0)
        self.assertEqual(s["arms"][P5]["reviewer"]["uncapped_wrong_distinct_pairs"], 3)

    def test_missing_trace_or_unmapped_aborts_before_writes(self):
        b = self.make_arm("baseline", 5, recs=[])
        self.assert_rejected(b, self.make_arm(P5, 6, name="m1"))                              # no trace
        self.assert_rejected(b, self.make_arm(P5, 6, caps=[], unmapped=True, name="m2"))      # unmapped capped reg
        self.assert_rejected(self.make_arm("baseline", 5, name="b0"), self.make_arm(P5, 6, caps=[], name="m4"))  # baseline no trace
        self.assertFalse(os.path.exists(os.path.join(self.exp, "dev-clean-summary.json")))
        self.assertEqual(self.run_cli("dev-clean", b, self.make_arm(P5, 6, caps=[], name="m3")), 0)

    def test_baseline_capped_record_aborts_before_writes(self):
        for recs in ([cap_rec("c1", "c2")], [cap_rec("c3", "c4")]):  # same-person or different-person capped record
            b = self.make_arm("baseline", 5, recs=recs, name="bad%d" % len(os.listdir(self.exp)))
            self.assert_rejected(b, self.make_arm(P5, 6, caps=[], name="c%d" % len(os.listdir(self.exp))))
            self.assertFalse(os.path.exists(os.path.join(self.exp, "dev-clean-summary.json")))

    def test_cap_gate_end_to_end(self):
        # queue pairs w0|a-w0|b ... ; cap the first two wrong pairs in the candidate arm
        capw = [cap_rec("w0|a", "w0|b"), cap_rec("w1|b", "w1|a"), cap_rec("c1", "c2")]
        b = self.make_arm("baseline", 5, recs=[], wrong=2, fm=3)
        c = self.make_arm(P5, 6, recs=capw, wrong=4, fm=1, name="cand-ok")   # wrong 4/lane: 2 capped, 2 uncapped
        self.assertEqual(self.run_cli("dev-clean", b, c), 0)
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        r = s["arms"][P5]["reviewer"]
        self.assertEqual((r["capped_wrong_distinct_pairs"], r["uncapped_wrong_distinct_pairs"]), (6, 6))
        self.assertEqual(s["arms"][P5]["xscript_cap"], {"pairs": 9, "same_person": 3, "different_person": 6})
        g = _rj(os.path.join(self.exp, "selection.json"))["gate"]
        self.assertEqual(g["prevented_false_merges"], 6)
        self.assertEqual(g["capped_wrong_distinct_pairs"], {P5: 6, "max_allowed": 12})
        self.assertEqual(g["uncapped_wrong_distinct_pairs"], {"baseline": 6, P5: 6, "max_allowed": 6.6})
        self.assertEqual(g["capped_same_person_pairs"], 3)
        self.assertEqual(g["reasons"], [])


def cap_arm(name, tp, truth, uncapped, capped, fm, wrong=None, **kw):
    a = arm(name, tp, truth, false_merge_pairs=fm, **kw)
    a["reviewer"] = dict(a["reviewer"], uncapped_wrong_distinct_pairs=uncapped, capped_wrong_distinct_pairs=capped,
                         wrong_distinct_pairs=uncapped + capped if wrong is None else wrong, hours=1.0)
    return a


class CapGate(unittest.TestCase):
    def base(self, wrong=10, fm=5):
        return cap_arm("baseline", 500, 600, wrong, 0, fm)

    def test_cap_passes_exactly_at_two_per_prevented(self):
        for kind in pfc.GATE_KINDS:
            g = pfc.gate(self.base(), cap_arm(P5, 530, 600, 11, 6, 2), kind)  # prevented 3, capped 6 = 2 x 3
            self.assertTrue(g["pass"], (kind, g["reasons"]))
            self.assertEqual(g["review_gate"], "wrong_pair_cap")
            self.assertEqual(g["prevented_false_merges"], 3)
            self.assertEqual(g["capped_wrong_distinct_pairs"], {P5: 6, "max_allowed": 6})
            self.assertEqual(g["uncapped_wrong_distinct_pairs"], {"baseline": 10, P5: 11, "max_allowed": 11.0})
            self.assertIn("extra_hours_per_extra_closure", g)
            self.assertEqual(g["distinct_pairs"]["gating"], False)
            self.assertNotIn("wrong_distinct_pairs", g)

    def test_fails_when_nothing_prevented(self):
        for fm in (5, 6):
            g = pfc.gate(self.base(), cap_arm(P5, 530, 600, 10, 0, fm), "selection")
            self.assertFalse(g["pass"])
            self.assertIn("false_merge_pairs not reduced (prevented %d)" % (5 - fm), g["reasons"])
        # capped record with prevented 0 also trips the 2:1 ratio (max 0)
        g = pfc.gate(self.base(), cap_arm(P5, 530, 600, 10, 1, 5), "selection")
        self.assertEqual(g["reasons"], ["false_merge_pairs not reduced (prevented 0)",
                                        "capped wrong-pair link-review pairs 1 exceed 0 (2 per prevented false merge)"])

    def test_fails_on_two_to_one_ratio(self):
        g = pfc.gate(self.base(), cap_arm(P5, 530, 600, 10, 7, 2), "selection")
        self.assertEqual(g["reasons"], ["capped wrong-pair link-review pairs 7 exceed 6 (2 per prevented false merge)"])

    def test_fails_on_uncapped_growth(self):
        g = pfc.gate(self.base(), cap_arm(P5, 530, 600, 12, 0, 2), "selection")
        self.assertEqual(g["reasons"], ["uncapped wrong-pair link-review pairs 12 exceed 11.00 (baseline 10 +10%)"])
        b0 = self.base(wrong=0)
        self.assertTrue(pfc.gate(b0, cap_arm(P5, 530, 600, 1, 0, 2), "selection")["pass"])
        g = pfc.gate(b0, cap_arm(P5, 530, 600, 2, 0, 2), "selection")
        self.assertEqual(g["reasons"], ["uncapped wrong-pair link-review pairs 2 exceed 1.00 (baseline 0 +10%)"])

    def test_capped_pairs_do_not_count_as_uncapped_growth(self):
        g = pfc.gate(self.base(wrong=7), cap_arm(P5, 530, 600, 7, 6, 2), "selection")  # total wrong 13 > 7.7, prevented 3
        self.assertTrue(g["pass"], g["reasons"])

    def test_other_gates_still_apply_and_missing_fields_error(self):
        self.assertFalse(pfc.gate(self.base(), cap_arm(P5, 505, 600, 10, 0, 2), "selection")["pass"])
        for b, c in ((arm("baseline", 500, 600), cap_arm(P5, 530, 600, 1, 0, 2)),
                     (cap_arm("baseline", 500, 600, 1, 0, 2), arm(P5, 530, 600))):
            with self.assertRaises(ValueError):
                pfc.gate(b, c, "selection")

    def test_pairfeat4_gate_json_unchanged(self):
        b = warm("baseline", 500, 600, wrong=100, distinct_pairs=100, hours=2.0)
        g = pfc.gate(b, warm(P4, 512, 600, wrong=105, distinct_pairs=120, hours=5.0), "selection")
        self.assertEqual(json.dumps(g, sort_keys=True), json.dumps({
            "pass": True, "kind": "selection", "gain_pp": 2.0, "min_gain_pp": 2.0, "review_gate": "wrong_pair",
            "wrong_distinct_pairs": {"baseline": 100, P4: 105, "max_allowed": 110.0},
            "distinct_pairs": {"baseline": 100, P4: 120, "ratio": 1.2, "old_gate_max_allowed": 110.0, "gating": False},
            "extra_hours_per_extra_closure": 0.25, "reasons": []}, sort_keys=True))


class CapLoad(unittest.TestCase):
    def test_review_load_capped_split(self):
        q, pk = queue(same=1, wrong=3)
        capped = {("w0|a", "w0|b"), ("w2|b", "w2|a"[::1])}
        capped = {tuple(sorted(k)) for k in capped} | {("s0|a", "s0|b"), ("zz", "yy")}  # extras outside wrong: ignored
        out = pfc.review_load(q, [], pk, capped)
        self.assertEqual((out["wrong_distinct_pairs"], out["capped_wrong_distinct_pairs"],
                          out["uncapped_wrong_distinct_pairs"]), (3, 2, 1))
        out = pfc.review_load(q, [], pk, set())
        self.assertEqual((out["capped_wrong_distinct_pairs"], out["uncapped_wrong_distinct_pairs"]), (0, 3))
        self.assertNotIn("capped_wrong_distinct_pairs", pfc.review_load(q, [], pk))
        self.assertNotIn("capped_wrong_distinct_pairs", pfc.review_load(q, []))

    def test_xscript_cap_pairs_sets(self):
        pk = XscriptCount.PK
        same, diff = pfc.xscript_cap_pairs(trace_lines([cap_rec("r2", "r1"), cap_rec("r4", "r3"), cap_rec("r3", "r4")]).splitlines(True), pk)
        self.assertEqual((same, diff), ({("r1", "r2")}, {("r3", "r4")}))

    def test_lane_metrics_and_aggregate_carry_cap_fields(self):
        load = dict(LOAD, wrong_distinct_pairs=3, capped_wrong_distinct_pairs=1, uncapped_wrong_distinct_pairs=2)
        m = pfc.lane_metrics(1, report(), funnel(5, 10), {}, load)
        self.assertEqual((m["reviewer"]["capped_wrong_distinct_pairs"], m["reviewer"]["uncapped_wrong_distinct_pairs"]), (1, 2))
        lanes = [pfc.lane_metrics(s, report(), funnel(5, 10), {}, load) for s in (521, 541, 547)]
        r = pfc.aggregate_arm(P5, lanes)["reviewer"]
        self.assertEqual((r["capped_wrong_distinct_pairs"], r["uncapped_wrong_distinct_pairs"]), (3, 6))
        mixed = pfc.lane_metrics(547, report(), funnel(5, 10), {}, dict(LOAD, wrong_distinct_pairs=3))
        with self.assertRaises(ValueError):
            pfc.aggregate_arm(P5, lanes[:2] + [mixed])


class CLIPairfeat4NoXscript(CLIPairfeat4):
    def test_no_xscript_field_for_pairfeat4(self):
        self.assertEqual(self.run_cli("dev-clean", self.make_arm("baseline", 5), self.make_arm(P4, 6)), 0)
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        for a in ("baseline", P4):
            self.assertNotIn("xscript_cap", s["arms"][a])
            self.assertNotIn("xscript_cap", s["arms"][a]["per_seed"]["433"])


class CappedNotQueuedCategory(unittest.TestCase):
    CAT = "candidate_capped_not_queued"

    def fun(self, **cats):
        return funnel(5, 10, attached=5, candidate_below_review=4, **dict(cats))

    def test_pairfeat5_lane_counts_and_partitions(self):
        f = self.fun(**{self.CAT: 1})
        m = pfc.lane_metrics(1, report(tp=5, truth=10), f, {}, dict(LOAD, wrong_distinct_pairs=0), candidate=P5)
        self.assertEqual(m["categories"][self.CAT], 1)
        self.assertEqual(m["residual"][self.CAT], 1)
        self.assertEqual(sum(m["residual"].values()), 5)
        z = pfc.lane_metrics(1, report(tp=5, truth=10), funnel(5, 10), {}, dict(LOAD, wrong_distinct_pairs=0), candidate=P5)
        self.assertEqual((z["categories"][self.CAT], z["residual"][self.CAT]), (0, 0))
        with self.assertRaises(ValueError):  # extra category not balanced by truth
            pfc.lane_metrics(1, report(tp=5, truth=10), self.fun(**{self.CAT: 2}), {}, LOAD, candidate=P5)

    def test_pairfeat5_arms_aggregate_the_category(self):
        lanes = [pfc.lane_metrics(s, report(tp=5, truth=10), self.fun(**{self.CAT: 1}), {},
                                  dict(LOAD, wrong_distinct_pairs=0), candidate=P5) for s in (521, 541, 547)]
        a = pfc.aggregate_arm("baseline", lanes)
        self.assertEqual((a["categories"][self.CAT], a["residual"][self.CAT]), (3, 3))
        self.assertEqual(pfc.aggregate_arm(P5, lanes)["residual"][self.CAT], 3)

    def test_earlier_candidates_unchanged_and_reject_it(self):
        for cand in (None, "pairfeat1", "pairfeat4"):
            kw = {} if cand is None else {"candidate": cand}
            m = pfc.lane_metrics(1, report(tp=5, truth=10), funnel(5, 10), {}, LOAD, **kw)
            plain = pfc.lane_metrics(1, report(tp=5, truth=10), funnel(5, 10), {}, LOAD)
            self.assertEqual(m, plain)
            self.assertNotIn(self.CAT, m["categories"])
            self.assertNotIn(self.CAT, m["residual"])
            with self.assertRaises(ValueError):
                pfc.lane_metrics(1, report(tp=5, truth=10), self.fun(**{self.CAT: 1}), {}, LOAD, **kw)
        lanes = [pfc.lane_metrics(s, report(), funnel(5, 10), {}, dict(LOAD, wrong_distinct_pairs=0), candidate=P4)
                 for s in (433, 449, 461)]
        a = pfc.aggregate_arm(P4, lanes)
        self.assertNotIn(self.CAT, a["categories"])
        self.assertNotIn(self.CAT, a["residual"])



# ================================================================ byte identity of pairfeat1-pairfeat5 (pinned at db5ce5e)

def _digest(obj):
    return hashlib.sha256(json.dumps(obj, sort_keys=True).encode()).hexdigest()


# SHA256 digests of the pairfeat1-pairfeat5 driver tables and analysis outputs computed with the harness at db5ce5e
# (before r1review); any change to these outputs fails here.
GOLDEN = {"driver": "cb1b2d1c913effd8d33bef6ba748298c726e42c564c3db34b172270f3d4201f7",
          "p1-dev-clean": "50d0cea515c9e6b4755aac519b01c2ed804182b516ca540b526cd2164de59684",
          "p4-dev-clean": "518b80b3ebea34deb40a8af85f60425d00830fd727622b0e0044a03b4bf15641",
          "p5-dev-clean": "1262922847b3e19787b9c84ccc864dfc950a43bd0acf9a87682a8c656dac2c6f",
          "p5-dev-stress": "55f5802a5c524dc60e4494669d20a43a54a2a673b8ed318ff73cd265b00732fc"}


class ByteIdentity(unittest.TestCase):
    ROOT = "/repo"
    BIN = "/repo/bin/wsi7"
    O = "/data/exp/arm/v2-s0"

    def driver_tables(self):
        out = {}
        for c in (P1, P2, P3, P4, P5):
            out[c] = {"arms": list(pfc.arms(c)), "modes": pfc.modes(c), "review_gate": pfc.review_gate(c),
                      "settings": {m: pfc.expected_settings(m, c) for m in pfc.MODES},
                      "chain": {a: [pfc.CHAIN_KEY[a], list(pfc.CHAIN_FILES[a]), pfc.MODELSET[a]] for a in pfc.arms(c)},
                      "lanes": {m: {str(s): {a: pfc.lane_commands(self.ROOT, self.BIN, self.O, s, m, a) for a in pfc.arms(c)}
                                    for s in pfc.modes(c)[m]["seeds"]} for m in pfc.MODES}}
        return out

    def outputs(self, case, scenario):
        t = case()
        t.setUp()
        try:
            names = scenario(t)
            return {n: hashlib.sha256(_rb(os.path.join(t.exp, n))).hexdigest() for n in names}
        finally:
            t.tearDown()

    @staticmethod
    def p1(t):
        t.assertEqual(t.run_cli("dev-clean", t.make_arm("baseline", 5), t.make_arm(P1, 6)), 0)
        return ("dev-clean-summary.json", "selection.json")

    @staticmethod
    def p4(t):
        t.assertEqual(t.run_cli("dev-clean", t.make_arm("baseline", 5, same=2, wrong=2), t.make_arm(P4, 6, same=8, wrong=2)), 0)
        return ("dev-clean-summary.json", "selection.json")

    @staticmethod
    def p5(t, stress=False):
        capw = [cap_rec("w0|a", "w0|b"), cap_rec("w1|b", "w1|a"), cap_rec("c1", "c2")]
        t.assertEqual(t.run_cli("dev-clean", t.make_arm("baseline", 5, recs=[], wrong=2, fm=3),
                                t.make_arm(P5, 6, recs=capw, wrong=4, fm=1)), 0)
        if not stress:
            return ("dev-clean-summary.json", "selection.json")
        t.assertEqual(t.run_cli("dev-stress", t.make_arm("baseline", 5, mode="dev-stress", recs=[], wrong=2, fm=3),
                                t.make_arm(P5, 5, mode="dev-stress", recs=capw, wrong=4, fm=1)), 0)
        return ("dev-stress-summary.json", "stress.json")

    def test_pairfeat1_5_driver_tables_unchanged(self):
        self.assertEqual(_digest(self.driver_tables()), GOLDEN["driver"])

    def test_pairfeat1_4_5_analysis_json_unchanged(self):
        got = {"p1-dev-clean": _digest(self.outputs(CLI, self.p1)), "p4-dev-clean": _digest(self.outputs(CLIPairfeat4, self.p4)),
               "p5-dev-clean": _digest(self.outputs(CLIPairfeat5, self.p5)),
               "p5-dev-stress": _digest(self.outputs(CLIPairfeat5, lambda t: self.p5(t, stress=True)))}
        self.assertEqual(got, {k: GOLDEN[k] for k in got})



# ================================================================ candidate r1review (three arms: baseline, pairfeat5 reference, r1review)

R1 = "r1review"
R1_DEV = [571, 577, 587]
R1_VAL = [631, 641, 643, 647, 653]
R1_ENTRY = {"dev_seeds": [571, 577, 587], "validation_seeds": [631, 641, 643, 647, 653], "review_gate": "wrong_pair_cap",
            "report_xscript_cap": True, "chain": "pairfeat5", "reference": "pairfeat5", "extra_flags": ["-r1review"],
            "lane_image": True, "baseline_gating": False}
CU = "completion_undocumented"
EXPERIMENT_JSON = {"candidate": R1, "reference": P5}


class R1reviewCandidate(unittest.TestCase):
    def test_entry_seeds_and_arms(self):
        self.assertEqual(pfc.CANDIDATES[R1], R1_ENTRY)
        self.assertEqual(pfc.arms(R1), ("baseline", P5, R1))
        for c in (P1, P2, P3, P4, P5):
            self.assertEqual(len(pfc.arms(c)), 2)
            self.assertEqual(pfc.mode_settings("validation-clean", c)["seeds"], [401, 409, 419, 421, 431])
        self.assertEqual(pfc.modes(R1), {
            "dev-clean": {"split": "dev", "seeds": R1_DEV, "rerr": "0", "errungraded": "0"},
            "dev-stress": {"split": "dev", "seeds": R1_DEV, "rerr": "0.04", "errungraded": "0.25"},
            "validation-clean": {"split": "eval", "seeds": R1_VAL, "rerr": "0", "errungraded": "0"}})
        self.assertEqual(pfc.review_gate(R1), "wrong_pair_cap")
        self.assertEqual(pfc.MODELSET[R1], P5)
        self.assertEqual(pfc.CHAIN_FILES[R1], CAND5_FILES)
        self.assertEqual(pfc.CHAIN_KEY[R1], "pairfeat5_chain_sha256")
        self.assertEqual(pfc.REF_MIN_GAIN_PP, {"selection": 1.0, "stress": 0.0, "validation": 0.5})
        self.assertEqual(pfc.REF_MAX_HOURS_GROWTH, 0.10)
        self.assertEqual(pfc.EXPERIMENT, "experiment.json")
        self.assertIn(pfc.EXPERIMENT, pfc.RESERVED)

    def test_expected_settings(self):
        v = pfc.expected_settings("validation-clean", R1)
        self.assertEqual((v["seeds"], v["arms"], v["candidate_modelset"]), (R1_VAL, ["baseline", P5, R1], P5))
        self.assertEqual((v["reference"], v["candidate_extra_flags"]), (P5, ["-r1review"]))
        five = pfc.expected_settings("validation-clean", P5)
        self.assertEqual({k: v[k] for k in five if k not in ("seeds", "arms", "candidate_modelset", "image")},
                         {k: five[k] for k in five if k not in ("seeds", "arms", "candidate_modelset", "image")})
        self.assertEqual(set(v) - set(five), {"reference", "candidate_extra_flags", "imgphotos", "imgcache"})
        self.assertEqual((v["image"], v["imgphotos"], v["imgcache"]), ("cached", True, "models/image_verdicts.json"))
        self.assertEqual(five["image"], "off")

    def test_chain_key_inference_never_names_r1review(self):
        b = chain(BASE_FILES, "b")
        ok = {"git_head": HEAD, "baseline_chain_sha256": b, "pairfeat5_chain_sha256": chain(CAND5_FILES, "c")}
        self.assertEqual(pfc.infer_candidate(ok, "artifact-manifest.json"), P5)
        for bad in ({"git_head": HEAD, "baseline_chain_sha256": b, "r1review_chain_sha256": chain(CAND5_FILES, "c")},
                    dict(ok, r1review_chain_sha256=chain(CAND5_FILES, "c"))):
            with self.assertRaises(pfc.PreflightError):
                pfc.infer_candidate(bad, "artifact-manifest.json")


class R1reviewGuard(unittest.TestCase):
    ROOT = "/repo"
    BIN = "/repo/bin/wsi7"
    O = "/data/exp/arm/v2-s571"

    def cmds(self, mode="dev-clean", arm_=R1, seed=571, cand=R1):
        return {n: argv for n, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, mode, arm_, candidate=cand)}

    def rejects(self, argv, arm_=R1, cand=R1, mode="dev-clean"):
        with self.assertRaises(pfc.GuardError, msg=" ".join(argv)):
            pfc.guard(argv, mode, arm_, self.BIN, cand)

    def test_lane_argv_per_arm(self):
        r, f, b = self.cmds(arm_=R1), self.cmds(arm_=P5), self.cmds(arm_="baseline")
        for n in ("gen", "sim", "augment", "classifier"):
            self.assertEqual(r[n], f[n])
            self.assertEqual(r[n], b[n])
            self.assertNotIn("-r1review", r[n])
        for n in ("orchestrate", "audit", "report7"):
            self.assertEqual(r[n], f[n] + ["-r1review"])          # the pairfeat5 lane plus the one flag
            self.assertEqual(r[n][r[n].index("-modelset") + 1], P5)
            self.assertEqual(r[n][r[n].index("-policy") + 1], "v7strict")
            self.assertEqual(r[n].count("-r1review"), 1)
            self.assertNotIn("-r1review", f[n])
            self.assertNotIn("-r1review", b[n])
            self.assertNotIn("-modelset", b[n])

    def test_every_r1review_lane_passes_the_guard(self):
        for mode in pfc.MODES:
            for seed in pfc.modes(R1)[mode]["seeds"]:
                for a in pfc.arms(R1):
                    for _, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, mode, a, candidate=R1):
                        pfc.guard(argv, mode, a, self.BIN, R1)

    def test_flag_only_on_the_r1review_arm(self):
        r, f, b = self.cmds(arm_=R1), self.cmds(arm_=P5), self.cmds(arm_="baseline")
        for n in ("orchestrate", "audit", "report7"):
            self.rejects(f[n] + ["-r1review"], arm_=P5)                       # reference arm
            self.rejects(b[n] + ["-r1review"], arm_="baseline")
            self.rejects([a for a in r[n] if a != "-r1review"], arm_=R1)      # required on the r1review arm
            self.rejects(r[n] + ["-r1review"])                                # repeated
            self.rejects([a for a in r[n] if a != "-r1review"] + ["-r1review=true"])
            self.rejects([a for a in r[n] if a != "-r1review"] + ["-r1review=false"])
            self.rejects(r[n] + ["false"])                                    # a value after the bool flag
            idx = r[n].index("-r1review")
            self.rejects(r[n][:idx] + ["-r1review", "true"] + r[n][idx + 1:])
            self.rejects([a for a in r[n] if a != "-r1review"] + ["--r1review"])
            self.rejects([a for a in r[n] if a != "-r1review"] + ["-R1review"])
            self.rejects([a for a in r[n] if a != "-r1review"] + ["-r1reviewx"])
            self.rejects(f[n] + ["-r1review"], arm_=P5, cand=P5)              # pairfeat5 experiment
            g = list(r[n])
            g[g.index("-modelset") + 1] = R1                                  # the arm's modelset is pairfeat5
            self.rejects(g)
        for n in ("gen", "sim", "augment", "classifier"):
            self.rejects(r[n] + ["-r1review"])
        # the r1review arm is not an arm of a pairfeat5 experiment, and the seeds follow the experiment
        self.rejects(r["orchestrate"], cand=P5)
        self.rejects(self.cmds(arm_=P5, seed=521)["gen"], arm_=P5)
        with self.assertRaises(pfc.GuardError):
            pfc.guard(self.cmds(arm_=P5, seed=571, cand=P5)["gen"], "dev-clean", P5, self.BIN, P5)
        pfc.guard(self.cmds(arm_=P5, seed=521, cand=P5)["gen"], "dev-clean", P5, self.BIN, P5)

    def test_output_paths_may_name_r1review(self):
        o = "/data/agents2/hardening/r1review/dev/r1review/v2-s571"
        for a in pfc.arms(R1):
            for _, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, o, 571, "dev-clean", a, candidate=R1):
                pfc.guard(argv, "dev-clean", a, self.BIN, R1)


class R1reviewPreflight(unittest.TestCase):
    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.r = Repo(self.td.name, cand=P5)
        for f in CAND5_FILES:
            with open(os.path.join(self.r.root, "models", f), "w") as h:
                h.write("model " + f)
        self.ep = os.path.join(self.r.exp, "experiment.json")

    def tearDown(self):
        self.td.cleanup()

    def ident(self, **kw):
        d = {"git_head": HEAD, "baseline_chain_sha256": self.r.hashes(BASE_FILES),
             "pairfeat5_chain_sha256": self.r.hashes(CAND5_FILES), "candidate": R1}
        d.update(kw)
        return d

    def write_selection_and_stress(self):
        sp, tp = os.path.join(self.r.exp, "selection.json"), os.path.join(self.r.exp, "stress.json")
        _wj(sp, self.ident(selected_modelset=R1, mode="dev-clean", seeds=R1_DEV, **{"pass": True}))
        _wj(tp, self.ident(mode="dev-stress", seeds=R1_DEV, selection_sha256=_sha(sp), **{"pass": True}))

    def test_init_experiment(self):
        with self.assertRaises(pfc.PreflightError):
            pfc.init_experiment(self.r.exp, R1)               # no artifact manifest
        self.assertFalse(os.path.exists(self.ep))
        b = self.r.hashes(BASE_FILES)
        ap = os.path.join(self.r.exp, "artifact-manifest.json")
        for m in ({"git_head": HEAD, "baseline_chain_sha256": b, "pairfeat4_chain_sha256": chain(CAND4_FILES, "c")},
                  {"git_head": HEAD, "baseline_chain_sha256": b, "pairfeat5_chain_sha256": self.r.hashes(CAND5_FILES),
                   "pairfeat4_chain_sha256": chain(CAND4_FILES, "c")},
                  {"git_head": HEAD, "baseline_chain_sha256": b, "r1review_chain_sha256": self.r.hashes(CAND5_FILES)},
                  {"git_head": HEAD, "baseline_chain_sha256": b, "pairfeat5_chain_sha256": {"x": "1"}}):
            _wj(ap, m)
            with self.assertRaises(pfc.PreflightError, msg=str(m)):
                pfc.init_experiment(self.r.exp, R1)
            self.assertFalse(os.path.exists(self.ep))
        self.r.artifact_manifest()
        for bad in (P5, "pairfeat6", "", None):
            with self.assertRaises(pfc.PreflightError):
                pfc.init_experiment(self.r.exp, bad)          # only a candidate with a reference arm has one
        self.assertFalse(os.path.exists(self.ep))
        self.assertEqual(pfc.init_experiment(self.r.exp, R1), EXPERIMENT_JSON)
        self.assertEqual(_rj(self.ep), EXPERIMENT_JSON)
        before = _rb(self.ep)
        with self.assertRaises(pfc.PreflightError):
            pfc.init_experiment(self.r.exp, R1)               # write-once
        self.assertEqual(_rb(self.ep), before)

    def test_init_experiment_cli(self):
        self.r.artifact_manifest()
        self.assertEqual(pfc.main(["init-experiment", self.r.exp]), 2)
        self.assertEqual(pfc.main(["init-experiment", self.r.exp, P5]), 2)
        self.assertFalse(os.path.exists(self.ep))
        self.assertEqual(pfc.main(["init-experiment", self.r.exp, R1]), 0)
        self.assertEqual(_rj(self.ep), EXPERIMENT_JSON)
        self.assertEqual(pfc.main(["init-experiment", self.r.exp, R1]), 2)

    def test_inference(self):
        self.r.artifact_manifest()
        # no experiment.json: today's inference (a pairfeat5 parent); the r1review arm is refused
        self.assertEqual(pfc.preflight_identity("dev-clean", "baseline", self.r.exp, self.r.root, HEAD)[0], P5)
        with self.assertRaises(pfc.PreflightError):
            pfc.preflight_identity("dev-clean", R1, self.r.exp, self.r.root, HEAD)
        pfc.init_experiment(self.r.exp, R1)
        for a in pfc.arms(R1):
            cand, ch = pfc.preflight_identity("dev-clean", a, self.r.exp, self.r.root, HEAD)
            self.assertEqual(cand, R1)
            self.assertEqual(ch, self.r.hashes(BASE_FILES if a == "baseline" else CAND5_FILES))
        with self.assertRaises(pfc.PreflightError):
            pfc.preflight_identity("dev-clean", P4, self.r.exp, self.r.root, HEAD)
        # a contradicting or malformed experiment.json is refused for every arm
        good = _rb(self.ep)
        for bad in ({"candidate": R1, "reference": P4}, {"candidate": P5, "reference": P5}, {"candidate": "pairfeat6", "reference": P5},
                    {"candidate": R1}, dict(EXPERIMENT_JSON, extra=1), [], "r1review"):
            _wj(self.ep, bad)
            for a in ("baseline", P5, R1):
                with self.assertRaises(pfc.PreflightError, msg=str(bad)):
                    pfc.preflight_identity("dev-clean", a, self.r.exp, self.r.root, HEAD)
        with open(self.ep, "wb") as h:
            h.write(good)
        # experiment.json naming r1review in a parent whose manifest carries another candidate's chain
        b = self.r.hashes(BASE_FILES)
        _wj(os.path.join(self.r.exp, "artifact-manifest.json"),
            {"git_head": HEAD, "baseline_chain_sha256": b, "pairfeat4_chain_sha256": chain(CAND4_FILES, "c")})
        for a in ("baseline", P4, R1):
            with self.assertRaises(pfc.PreflightError):
                pfc.preflight_identity("dev-clean", a, self.r.exp, self.r.root, HEAD)

    def test_dev_stress_selection_must_record_r1review(self):
        self.r.artifact_manifest()
        pfc.init_experiment(self.r.exp, R1)
        sp = os.path.join(self.r.exp, "selection.json")
        sel = self.ident(selected_modelset=R1, mode="dev-clean", seeds=R1_DEV, **{"pass": True})
        for bad in (dict(sel, selected_modelset=P5), dict(sel, candidate=P5), dict(sel, seeds=[521, 541, 547]),
                    dict(sel, r1review_chain_sha256=sel["pairfeat5_chain_sha256"])):
            _wj(sp, bad)
            with self.assertRaises(pfc.PreflightError, msg=str(bad)):
                pfc.preflight_identity("dev-stress", "baseline", self.r.exp, self.r.root, HEAD)
        _wj(sp, sel)
        self.assertEqual(pfc.preflight_identity("dev-stress", R1, self.r.exp, self.r.root, HEAD)[0], R1)

    def test_freeze_and_validation_preflight(self):
        self.r.artifact_manifest()
        pfc.init_experiment(self.r.exp, R1)
        self.write_selection_and_stress()
        vm = pfc.freeze_validation(self.r.exp, self.r.root, HEAD)
        self.assertEqual((vm["candidate"], vm["selected_modelset"], vm["seeds"]), (R1, R1, R1_VAL))
        self.assertEqual(vm["settings"], pfc.expected_settings("validation-clean", R1))
        self.assertEqual(vm["experiment_sha256"], _sha(self.ep))
        self.assertEqual(vm["pairfeat5_chain_sha256"], self.r.hashes(CAND5_FILES))
        self.assertNotIn("r1review_chain_sha256", vm)
        for a in pfc.arms(R1):
            self.assertEqual(pfc.preflight_identity("validation-clean", a, self.r.exp, self.r.root, HEAD)[0], R1)
        good = _rb(self.ep)
        os.remove(self.ep)
        for a in pfc.arms(R1):
            with self.assertRaises(pfc.PreflightError):
                pfc.preflight_identity("validation-clean", a, self.r.exp, self.r.root, HEAD)
        with open(self.ep, "w") as h:
            json.dump(EXPERIMENT_JSON, h)                     # same content, different bytes than the frozen file
        with self.assertRaises(pfc.PreflightError):
            pfc.preflight_identity("validation-clean", "baseline", self.r.exp, self.r.root, HEAD)
        with open(self.ep, "wb") as h:
            h.write(good)
        pfc.preflight_identity("validation-clean", "baseline", self.r.exp, self.r.root, HEAD)

    def test_pairfeat5_validation_manifest_has_no_experiment_field(self):
        self.r.artifact_manifest()
        self.r.selection()
        self.r.stress()
        vm = pfc.freeze_validation(self.r.exp, self.r.root, HEAD)
        self.assertEqual(vm["candidate"], P5)
        self.assertNotIn("experiment_sha256", vm)
        self.assertNotIn("reference", vm["settings"])


def ref_arm(name, tp, truth=600, fm=2, wrong=10, hours=10.0, **kw):
    a = cap_arm(name, tp, truth, wrong, 0, fm, **kw)
    a["reviewer"]["hours"] = hours
    return a


class ReferenceGate(unittest.TestCase):
    def test_pass(self):
        g = pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 536, hours=11.0), "selection")  # +1.0pp, hours 1.10 x
        self.assertEqual(json.dumps(g, sort_keys=True), json.dumps({
            "pass": True, "kind": "selection", "reference": P5, "gain_pp": 1.0, "min_gain_pp": 1.0,
            "hours": {P5: 10.0, R1: 11.0, "max_allowed": 11.0},
            "wrong_final_signoffs": {P5: 0, R1: 0, "gating": False},
            "not_above_reference": {"false_merge_pairs": {P5: 2, R1: 2}, "impure_clusters": {P5: 0, R1: 0},
                                    "cross_person_edges": {P5: 0, R1: 0}, "wrong_distinct_pairs": {P5: 10, R1: 10}},
            "reasons": []}, sort_keys=True))

    def test_gain_below_minimum(self):
        for kind, ok, bad in (("selection", 536, 535), ("stress", 530, 529), ("validation", 533, 532)):
            self.assertTrue(pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, ok), kind)["pass"], kind)
            g = pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, bad), kind)
            self.assertFalse(g["pass"], kind)
            self.assertEqual(len(g["reasons"]), 1, g["reasons"])
            self.assertIn("gain vs reference", g["reasons"][0])
        g = pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 535), "selection")
        self.assertEqual(g["reasons"], ["gain vs reference 0.8333pp below 1.0pp"])

    def test_contamination_not_above_reference(self):
        for kw, reason in (({"fm": 3}, "false_merge_pairs above reference (3 > 2)"),
                           ({"impure_clusters": 1}, "impure_clusters above reference (1 > 0)"),
                           ({"cross_person_edges": 4}, "cross_person_edges above reference (4 > 0)"),
                           ({"wrong": 11}, "wrong_distinct_pairs above reference (11 > 10)")):
            g = pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 540, **kw), "selection")
            self.assertEqual(g["reasons"], [reason])
        self.assertTrue(pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 540, fm=1, wrong=9), "selection")["pass"])

    def test_hours_above_ten_percent(self):
        g = pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 540, hours=11.01), "selection")
        self.assertEqual(g["reasons"], ["reviewer hours 11.01 exceed 11.00 (reference 10.00 +10%)"])
        self.assertTrue(pfc.reference_gate(ref_arm(P5, 530, hours=0.0), ref_arm(R1, 540, hours=0.0), "selection")["pass"])
        self.assertFalse(pfc.reference_gate(ref_arm(P5, 530, hours=0.0), ref_arm(R1, 540, hours=0.5), "selection")["pass"])

    def test_safety_zero_in_both_arms(self):
        for k in ("false_closures", "wrong_released_total", "replay_mismatch", "analysis_errors", "trace_error"):
            g = pfc.reference_gate(ref_arm(P5, 530, **{k: 1}), ref_arm(R1, 540), "selection")
            self.assertEqual(g["reasons"], ["reference " + k + " nonzero"])
            g = pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 540, **{k: 1}), "selection")
            self.assertEqual(g["reasons"], [k + " nonzero"])
        g = pfc.reference_gate(ref_arm(P5, 530, wrong_released_by_origin={"a": 1}), ref_arm(R1, 540), "selection")
        self.assertEqual(g["reasons"], ["reference wrong_released_by_origin nonzero"])

    def test_wrong_final_signoffs_reported_not_gated(self):
        # Amendment 1: stress lanes carry ~80 wrong final sign-offs per arm by design (4% reviewer error); with zero
        # false closures and wrong releases the reference gate's safety check passes, and the counts are reported
        ref, cand = ref_arm(P5, 530), ref_arm(R1, 540)
        ref["reviewer"]["wrong_final_signoffs"], cand["reviewer"]["wrong_final_signoffs"] = 82, 85
        for kind in pfc.GATE_KINDS:
            g = pfc.reference_gate(ref, cand, kind)
            self.assertTrue(g["pass"], (kind, g["reasons"]))
            self.assertEqual(g["wrong_final_signoffs"], {P5: 82, R1: 85, "gating": False})
        ref["false_closures"] = 1
        self.assertEqual(pfc.reference_gate(ref, cand, "stress")["reasons"], ["reference false_closures nonzero"])

    def test_arms_and_inputs(self):
        self.assertFalse(pfc.reference_gate(ref_arm(P4, 530), ref_arm(R1, 540), "selection")["pass"])
        self.assertFalse(pfc.reference_gate(ref_arm(P5, 530), ref_arm(P5, 540), "selection")["pass"])
        self.assertFalse(pfc.reference_gate(ref_arm("baseline", 530), ref_arm(R1, 540), "selection")["pass"])
        self.assertEqual(pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 540, truth=601), "selection")["reasons"],
                         ["truth_closed differs between reference and candidate"])
        with self.assertRaises(ValueError):
            pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 540), "bogus")
        for r, c in ((arm(P5, 530, 600), ref_arm(R1, 540)), (ref_arm(P5, 530), arm(R1, 540, 600))):
            with self.assertRaises(ValueError):
                pfc.reference_gate(r, c, "selection")       # reviewer.wrong_distinct_pairs is required

    def test_combined_selection(self):
        base = cap_arm("baseline", 500, 600, 10, 0, 5)
        s = pfc.select(base, ref_arm(R1, 540), ref_arm(P5, 530))
        self.assertEqual((s["candidate"], s["reference"], s["selected_modelset"], s["pass"]), (R1, P5, R1, True))
        self.assertTrue(s["gate"]["pass"] and s["reference_gate"]["pass"])
        self.assertEqual(s["gate"]["review_gate"], "wrong_pair_cap")
        self.assertAlmostEqual(s["pairfeat5_recall"], 530 / 600)
        s = pfc.select(base, ref_arm(R1, 535), ref_arm(P5, 530))         # baseline gate passes, reference gate fails
        self.assertEqual((s["selected_modelset"], s["pass"], s["gate"]["pass"], s["reference_gate"]["pass"]), (None, False, True, False))
        s = pfc.select(base, ref_arm(R1, 510), ref_arm(P5, 500))         # baseline gate fails: reported only (amendment 3)
        self.assertEqual((s["selected_modelset"], s["pass"], s["gate"]["pass"], s["reference_gate"]["pass"]), (R1, True, False, True))
        self.assertIs(s["gate"]["gating"], False)
        self.assertNotIn("reference_gate", pfc.select(base, cap_arm(P5, 530, 600, 10, 0, 2)))


def cu_items(n, start=0):
    return [{"item": "cu%d" % i, "loop": "L%d" % i, "kinds": ["evidence"], "pair": ["", ""], "report": "R%d" % i, "reason": CU}
            for i in range(start, start + n)]


class CompletionCount(unittest.TestCase):
    def test_counts_items_and_answers(self):
        q = cu_items(5) + [cu_items(1)[0]]                                     # duplicate cu0
        q += [{"item": "p1", "kinds": ["evidence"], "reason": "prep_ungraded_day15", "report": "R9"},
              {"item": "e1", "kinds": ["evidence"], "report": "R8"},
              {"item": "l1", "kinds": ["link"], "pair": ["a", "b"]}]
        dec = [{"item": "cu0", "evidence": "adequate", "wrong": False}, {"item": "cu1", "evidence": "inadequate", "wrong": True},
               {"item": "cu2", "evidence": "histology", "wrong": False}, {"item": "cu3", "evidence": "adequate"},
               {"item": "p1", "evidence": "adequate", "wrong": True}, {"item": "l1", "link": "same"}]
        self.assertEqual(pfc.completion_review(q, dec), {"items": 5, "adequate": 2, "inadequate": 1, "other": 1,
                                                         "unanswered": 1, "wrong": 1})
        self.assertEqual(pfc.completion_review([], []), {"items": 0, "adequate": 0, "inadequate": 0, "other": 0,
                                                         "unanswered": 0, "wrong": 0})
        # completion items never count as link-review load
        self.assertEqual(pfc.review_load(q, dec), pfc.review_load([i for i in q if i.get("reason") != CU], dec))

    def test_lane_metrics_and_aggregate(self):
        cu = {"items": 2, "adequate": 1, "inadequate": 1, "other": 0, "unanswered": 0, "wrong": 0}
        load = dict(LOAD, wrong_distinct_pairs=0)
        m = pfc.lane_metrics(1, report(), funnel(5, 10), {}, load, candidate=R1, completion=cu)
        self.assertEqual(m["completion_undocumented"], cu)
        self.assertNotIn("completion_undocumented", pfc.lane_metrics(1, report(), funnel(5, 10), {}, load, candidate=P5))
        lanes = [pfc.lane_metrics(s, report(), funnel(5, 10), {}, load, candidate=R1, completion=cu) for s in R1_DEV]
        self.assertEqual(pfc.aggregate_arm(R1, lanes)["completion_undocumented"],
                         {"items": 6, "adequate": 3, "inadequate": 3, "other": 0, "unanswered": 0, "wrong": 0})
        self.assertEqual(pfc.aggregate_arm(R1, lanes)["modelset"], P5)
        with self.assertRaises(ValueError):
            pfc.aggregate_arm(R1, lanes[:2] + [pfc.lane_metrics(587, report(), funnel(5, 10), {}, load, candidate=R1)])


IMAGE_BUILD = {"image": "cached", "imgphotos": True, "image_verdicts_sha256": "e" * 64}
POLICY_BASE = "v7strict+cal=true+guard=true+tau=0.99+linkfloor=0.01"


def lane_policy(a, image="cached"):
    """The decision policy stamp main.go writes for an arm (PolicyName: +models=<set>, then +image=<mode>)."""
    return POLICY_BASE + ("+models=" + pfc.MODELSET[a] if pfc.MODELSET[a] else "") + ("+image=" + image if image else "")


class CLIR1review(CLIFixture, unittest.TestCase):
    CAND = R1

    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.exp = os.path.join(self.td.name, "exp")
        os.makedirs(self.exp)
        c5 = chain(CAND5_FILES, "c")
        self.maps = {"baseline": chain(BASE_FILES, "b"), P5: c5, R1: c5}
        _wj(os.path.join(self.exp, "artifact-manifest.json"),
            {"git_head": HEAD, "baseline_chain_sha256": self.maps["baseline"], "pairfeat5_chain_sha256": c5})
        pfc.init_experiment(self.exp, R1)

    def make_arm(self, a, tp, *args, recs=None, fm=None, wrong=2, hours=None, cu=0, answers=(), flags="auto",
                 image=IMAGE_BUILD, img_reader="auto", policies="auto", out_log=True, r7=None, **kw):
        root = super().make_arm(a, tp, *args, **kw)
        bp = os.path.join(root, "build.json")
        if os.path.exists(bp):
            ba = kw.get("build_arm") or a
            fl = (["-r1review"] if ba == R1 else []) if flags == "auto" else flags
            b = dict(_rj(bp), **image)
            if fl:
                b["extra_flags"] = fl
            _wj(bp, b)
        for d in sorted(os.listdir(root)):
            if not d.startswith("v2-s"):
                continue
            q, pk = queue(same=1, wrong=wrong)
            pk.update({"c1": "C1", "c2": "C1", "c3": "C3", "c4": "C4"})
            q += cu_items(cu) + [{"item": "p1", "kinds": ["evidence"], "reason": "prep_ungraded_day15", "report": "R9"}]
            _wj(os.path.join(root, d, "run/main/review_queue.json"), q)
            with open(os.path.join(root, d, "run/main/reviewer_decisions.ndjson"), "w") as h:
                for i, ans in enumerate(answers):
                    h.write(json.dumps({"item": "cu%d" % i, "evidence": ans, "wrong": False}) + "\n")
                h.write(json.dumps({"item": "p1", "evidence": "adequate", "wrong": False}) + "\n")
            os.makedirs(os.path.join(root, d, "cohort"))
            _wj(os.path.join(root, d, "cohort/truth.json"), {"pk_qid": pk, "loops": {}})
            r = recs if recs is not None else ([] if a == "baseline" else [cap_rec("c1", "c2"), cap_rec("c3", "c4")])
            with open(os.path.join(root, d, "run/main/link_funnel.ndjson"), "w") as h:
                h.write(trace_lines(r))
            rp = os.path.join(root, d, "run/main/report7.json")
            rj = _rj(rp)
            if fm is not None:
                rj["linkage"]["false_merge_pairs"] = fm
            if hours is not None:
                rj["reviewer"]["by_kind"]["all"]["sim_hours"] = hours
            if r7 is not None:
                r7(rj)
            if img_reader == "auto":
                rj["image_reader"] = {"mode": "cached", "model": "landmark-cnn", "colonoscopy_reports_read": 4}
            elif img_reader is not None:
                rj["image_reader"] = img_reader
            _wj(rp, rj)
            if out_log:
                pol = ([lane_policy(kw.get("build_arm") or a)] * 2) if policies == "auto" else policies
                with open(os.path.join(root, d, "run/main/out_log.ndjson"), "w") as h:
                    for i, pv in enumerate(pol):
                        h.write(json.dumps({"seq": 2 * i + 1, "data": {"kind": "decision", "dec": {"id": "d%d" % i, "policy": pv}}}) + "\n")
                        h.write(json.dumps({"seq": 2 * i + 2, "data": {"kind": "review", "item": {"item": "x%d" % i}}}) + "\n")
        return root

    def three(self, mode="dev-clean", b=5, f=6, r=7, **kw):
        return (self.make_arm("baseline", b, mode=mode, fm=3), self.make_arm(P5, f, mode=mode, fm=1),
                self.make_arm(R1, r, mode=mode, fm=1, cu=3, answers=("adequate", "inadequate"), **kw))

    def run3(self, mode, roots, out=None):
        b, f, r = roots
        return pfc.main([mode, out or os.path.join(self.exp, mode + "-summary.json"), "baseline=" + b, "pairfeat5=" + f,
                         "r1review=" + r])

    def test_dev_clean_combined_pass(self):
        self.assertEqual(self.run3("dev-clean", self.three()), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual((sel["candidate"], sel["reference"], sel["selected_modelset"], sel["pass"]), (R1, P5, R1, True))
        self.assertEqual(sel["seeds"], R1_DEV)
        self.assertEqual(sel["pairfeat5_chain_sha256"], self.maps[P5])
        self.assertNotIn("r1review_chain_sha256", sel)
        self.assertEqual(sel["gate"]["review_gate"], "wrong_pair_cap")
        self.assertEqual(sel["gate"]["gain_pp"], 20.0)
        self.assertTrue(sel["reference_gate"]["pass"])
        self.assertEqual(sel["reference_gate"]["gain_pp"], 10.0)
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        self.assertEqual(sorted(s["arms"]), ["baseline", P5, R1])
        self.assertEqual((s["candidate"], s["reference"], s["pass"]), (R1, P5, True))
        self.assertEqual(s["reference_gate"], sel["reference_gate"])
        self.assertIn("paired_gain_vs_reference_t95", s)
        self.assertEqual(s["paired_gain_vs_reference_t95"]["per_seed_gain_pp"], {"571": 10.0, "577": 10.0, "587": 10.0})
        self.assertEqual(s["arms"][R1]["modelset"], P5)
        self.assertEqual(s["arms"][R1]["completion_undocumented"],
                         {"items": 9, "adequate": 3, "inadequate": 3, "other": 0, "unanswered": 3, "wrong": 0})
        self.assertEqual(s["arms"][P5]["completion_undocumented"]["items"], 0)
        self.assertEqual(s["arms"]["baseline"]["completion_undocumented"]["items"], 0)
        for a in (P5, R1):  # capped-pair report and the capped category for both non-baseline arms
            self.assertEqual(s["arms"][a]["xscript_cap"], {"pairs": 6, "same_person": 3, "different_person": 3})
            self.assertIn("candidate_capped_not_queued", s["arms"][a]["categories"])
        self.assertNotIn("xscript_cap", s["arms"]["baseline"])

    def test_reference_gate_failure_blocks_selection(self):
        self.assertEqual(self.run3("dev-clean", self.three(r=6)), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertTrue(sel["gate"]["pass"])
        self.assertEqual(sel["reference_gate"]["reasons"], ["gain vs reference 0.0000pp below 1.0pp"])
        self.assertEqual((sel["selected_modelset"], sel["pass"]), (None, False))

    def test_reference_hours_failure_blocks_selection(self):
        self.assertEqual(self.run3("dev-clean", self.three(hours=1.66)), 0)   # 4.98 h vs 4.5 h (max 4.95)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual(sel["reference_gate"]["reasons"], ["reviewer hours 4.98 exceed 4.95 (reference 4.50 +10%)"])
        self.assertFalse(sel["pass"])

    def test_three_roots_required(self):
        b, f, r = self.three()
        out = os.path.join(self.exp, "s.json")
        for roots in ({"baseline": b, P5: f}, {"baseline": b, R1: r}, {P5: f, R1: r}):
            with self.assertRaises(ValueError, msg=str(roots)):
                pfc.analyse("dev-clean", out, roots)
        self.assertEqual(pfc.main(["dev-clean", out, "baseline=" + b, "pairfeat5=" + f, "r1review=" + r, "pairfeat4=" + f]), 2)
        self.assertEqual(pfc.main(["dev-clean", out, "baseline=" + b, "pairfeat5=" + f, "pairfeat5=" + r]), 2)
        with self.assertRaises(ValueError):
            pfc.main(["dev-clean", out, "baseline=" + b, "pairfeat4=" + f, "r1review=" + r])
        # without experiment.json the parent is a pairfeat5 experiment: three roots are refused
        ep = os.path.join(self.exp, "experiment.json")
        saved = _rb(ep)
        os.remove(ep)
        with self.assertRaises(ValueError):
            self.run3("dev-clean", (b, f, r))
        with open(ep, "wb") as h:
            h.write(saved)
        self.assertFalse(os.path.exists(os.path.join(self.exp, "selection.json")))
        self.assertFalse(os.path.exists(out))

    def test_arm_identity_rejected(self):
        b, f, r = self.three()
        for roots in ((b, f, self.make_arm(R1, 7, flags=[], name="r-noflag")),                   # r1review build without -r1review
                      (b, self.make_arm(P5, 6, flags=["-r1review"], name="f-flag"), r),          # reference built with it
                      (b, f, self.make_arm(R1, 7, cfg_modelset=R1, name="r-ms")),               # funnel modelset must be pairfeat5
                      (b, f, self.make_arm(R1, 7, binary="d" * 64, name="r-bin")),
                      (b, self.make_arm(P5, 6, head="c" * 40, name="f-head"), r),
                      (b, self.make_arm(P5, 6, build_cand=P5, name="f-cand"), r),
                      (self.make_arm("baseline", 5, recs=[cap_rec("c3", "c4")], name="b-cap"), f, r),
                      (b, f, self.make_arm(R1, 7, seeds_of=P5, name="r-seeds"))):
            with self.assertRaises((ValueError, KeyError, OSError)):
                self.run3("dev-clean", roots)
            self.assertFalse(os.path.exists(os.path.join(self.exp, "selection.json")))
        self.assertEqual(self.run3("dev-clean", (b, f, r)), 0)

    def test_pairfeat5_arm_is_a_pairfeat5_lane(self):
        root = self.make_arm(P5, 6, fm=1)
        lane = os.path.join(root, "v2-s571")
        inside = pfc._load_lane(lane, 571, P5, "dev-clean", R1)
        alone = pfc._load_lane(lane, 571, P5, "dev-clean", P5)
        self.assertEqual(inside.pop("completion_undocumented")["items"], 0)
        self.assertEqual(inside, alone)

    def test_dev_stress_freeze_and_validation_end_to_end(self):
        r = Repo(os.path.join(self.td.name, "v"), cand=P5)
        for fn in CAND5_FILES:
            with open(os.path.join(r.root, "models", fn), "w") as h:
                h.write("model " + fn)
        self.exp = r.exp
        self.maps = {"baseline": r.hashes(BASE_FILES), P5: r.hashes(CAND5_FILES), R1: r.hashes(CAND5_FILES)}
        r.artifact_manifest()
        pfc.init_experiment(r.exp, R1)
        st3 = self.three(mode="dev-stress")
        with self.assertRaises(ValueError):
            self.run3("dev-stress", st3)                              # no selection yet
        self.assertEqual(self.run3("dev-clean", self.three()), 0)
        self.assertEqual(self.run3("dev-stress", st3), 0)
        st = _rj(os.path.join(r.exp, "stress.json"))
        self.assertEqual((st["pass"], st["candidate"], st["reference"], st["seeds"]), (True, R1, P5, R1_DEV))
        self.assertTrue(st["gate"]["pass"] and st["reference_gate"]["pass"])
        vm = pfc.freeze_validation(r.exp, r.root, HEAD)
        self.assertEqual((vm["candidate"], vm["seeds"]), (R1, R1_VAL))
        out = os.path.join(r.exp, "validation-clean-summary.json")
        v3 = self.three(mode="validation-clean")
        run = lambda roots: pfc.analyse("validation-clean", out, roots, repo=r.root, head=HEAD)
        with self.assertRaises(ValueError):
            run({"baseline": v3[0], R1: v3[2]})
        self.assertFalse(os.path.exists(out))
        res = run({"baseline": v3[0], P5: v3[1], R1: v3[2]})
        self.assertTrue(res["pass"])
        self.assertTrue(res["gate"]["pass"] and res["reference_gate"]["pass"])
        self.assertEqual((res["reference_gate"]["kind"], res["reference_gate"]["min_gain_pp"]), ("validation", 0.5))
        self.assertEqual((res["candidate"], res["seeds"]), (R1, R1_VAL))
        self.assertEqual(_rj(out)["reference_gate"], res["reference_gate"])
        with self.assertRaises(FileExistsError):
            run({"baseline": v3[0], P5: v3[1], R1: v3[2]})

    def test_stress_reference_failure(self):
        sel = {"selected_modelset": R1, "pass": True, "mode": "dev-clean", "seeds": R1_DEV, "git_head": HEAD,
               "baseline_chain_sha256": self.maps["baseline"], "pairfeat5_chain_sha256": self.maps[P5], "candidate": R1}
        _wj(os.path.join(self.exp, "selection.json"), sel)
        self.assertEqual(self.run3("dev-stress", self.three(mode="dev-stress", f=6, r=5)), 0)
        st = _rj(os.path.join(self.exp, "stress.json"))
        self.assertTrue(st["gate"]["pass"])
        self.assertFalse(st["reference_gate"]["pass"])
        self.assertFalse(st["pass"])


class RunnerR1review(RunnerFixture, unittest.TestCase):
    def init(self):
        return subprocess.run(["python3", "-I", os.path.join(HERE, "pairfeat_compare.py"), "init-experiment", self.exp, R1],
                              capture_output=True, text=True, timeout=60)

    def test_parent_needs_experiment_json(self):
        self.real_artifact_manifest(cand=P5)
        r = self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("candidate pairfeat5", r.stdout)
        self.assert_refused(self.run_sh(self.root, "dev-clean", R1, preflight_only=True))   # no experiment.json
        self.assertEqual(self.init().returncode, 0)
        self.assertEqual(self.init().returncode, 2)                                         # write-once
        for mode in ("dev-clean",):
            r = self.run_sh(self.root, mode, "baseline", preflight_only=True)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn("candidate r1review seeds 571 577 587", r.stdout)
        r = self.run_sh(self.root, "dev-clean", R1, preflight_only=True)
        self.assert_refused(r)                                     # fake pairfeat5 map: refused by preflight, not arm parsing
        self.assertNotIn("invalid arm", r.stderr)
        self.assert_refused(self.run_sh(self.root, "dev-clean", P5, preflight_only=True))
        self.assert_refused(self.run_sh(self.root, "dev-clean", P4, preflight_only=True), "r1review")

    def test_runner_tables(self):
        with open(SCRIPT) as h:
            text = h.read()
        self.assertIn('r1review) DEV_SEEDS="571 577 587"; VAL_SEEDS="631 641 643 647 653"; CHAIN=pairfeat5; REF=pairfeat5;;', text)
        self.assertIn('pairfeat5) DEV_SEEDS="521 541 547";;', text)
        self.assertIn('VAL_SEEDS="401 409 419 421 431"', text)

    def recheck(self, cand, chain_, mode="dev-clean"):
        """Run the runner's independent clean-HEAD manifest re-check (its inline Python) against this parent."""
        with open(SCRIPT) as h:
            lines = h.read().split("\n")
        start = next(i for i, l in enumerate(lines) if l.startswith('python3 -I - "$PARENT" "$MODE" "$HEAD"'))
        end = next(i for i in range(start + 1, len(lines)) if lines[i] == "PY")
        code = "\n".join(lines[start + 2:end])
        return subprocess.run(["python3", "-I", "-", self.exp, mode, self.head, SCRIPT, os.path.join(HERE, "pairfeat_compare.py"),
                               cand, chain_], input=code, capture_output=True, text=True, timeout=60).returncode

    def test_independent_recheck(self):
        self.real_artifact_manifest(cand=P5)
        self.assertEqual(self.recheck(P5, P5), 0)
        self.assertNotEqual(self.recheck(R1, P5), 0)                # no experiment.json
        self.assertNotEqual(self.recheck(R1, R1), 0)
        self.assertEqual(self.init().returncode, 0)
        self.assertEqual(self.recheck(R1, P5), 0)
        self.assertNotEqual(self.recheck(R1, R1), 0)
        self.assertNotEqual(self.recheck(P4, P4), 0)



# ================================================================ r1review experiments: synthetic photos + cached image reader (Amendment 2)

IMGCACHE = "/repo/models/image_verdicts.json"


def strip_image(argv):
    """argv with the r1review experiment's image settings turned back into the image-off lane."""
    out, skip = [], False
    for i, a in enumerate(argv):
        if skip:
            skip = False
            continue
        if a == "-imgphotos":
            continue
        if a == "-imgcache":
            skip = True
            continue
        out.append("off" if i and argv[i - 1] == "-image" else a)
    return out


class R1reviewImageLanes(unittest.TestCase):
    ROOT = "/repo"
    BIN = "/repo/bin/wsi7"
    O = "/data/exp/arm/v2-s571"

    def cmds(self, arm_, cand, mode="dev-clean", seed=571):
        return {n: argv for n, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, mode, arm_, candidate=cand)}

    def rejects(self, argv, arm_, cand, mode="dev-clean"):
        with self.assertRaises(pfc.GuardError, msg=" ".join(argv)):
            pfc.guard(argv, mode, arm_, self.BIN, cand)

    def test_lane_image_entry(self):
        self.assertIs(pfc.CANDIDATES[R1]["lane_image"], True)
        for c in (P1, P2, P3, P4, P5):
            self.assertNotIn("lane_image", pfc.CANDIDATES[c])
            self.assertFalse(pfc.lane_image(c))
        self.assertTrue(pfc.lane_image(R1))
        self.assertEqual(pfc.IMAGE_VERDICTS, "models/image_verdicts.json")

    def test_every_arm_runs_photos_and_cached_image_reader(self):
        arms_ = {a: self.cmds(a, R1) for a in pfc.arms(R1)}
        for a, c in arms_.items():
            g = c["gen"]
            self.assertEqual(g.count("-imgphotos"), 1, a)
            self.assertEqual(g[g.index("-imgcache") + 1], IMGCACHE)
            self.assertNotIn("-image", g)
            for n in ("orchestrate", "audit", "report7"):
                self.assertEqual(c[n][c[n].index("-image") + 1], "cached", (a, n))
                self.assertEqual(c[n][c[n].index("-imgcache") + 1], IMGCACHE, (a, n))
                self.assertEqual(c[n].count("-image"), 1)
                self.assertNotIn("-imgphotos", c[n])
            for n in ("sim", "augment", "classifier"):
                for f in ("-image", "-imgphotos", "-imgcache"):
                    self.assertNotIn(f, c[n], (a, n))
        for n in ("gen", "sim", "augment", "classifier"):  # identical cohort and truth in all three arms
            self.assertEqual(arms_["baseline"][n], arms_[P5][n])
            self.assertEqual(arms_[R1][n], arms_[P5][n])
        for n in ("orchestrate", "audit", "report7"):
            self.assertEqual(arms_[R1][n], arms_[P5][n] + ["-r1review"])
            self.assertNotIn("-modelset", arms_["baseline"][n])

    def test_reference_arm_is_a_pairfeat5_lane_with_the_image_reader(self):
        inside, alone = self.cmds(P5, R1), self.cmds(P5, P5)
        for n in inside:
            self.assertEqual(strip_image(inside[n]), alone[n], n)
        self.assertEqual(self.cmds("baseline", R1)["orchestrate"][-6:], ["-reader", "cached", "-image", "cached", "-imgcache", IMGCACHE])

    def test_default_candidate_keeps_existing_argv(self):
        for a in ("baseline", P1, P4, P5):
            default = {n: argv for n, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, self.O, 521, "dev-clean", a)}
            self.assertEqual(default, self.cmds(a, a if a != "baseline" else P5, seed=521))
            for argv in default.values():
                self.assertNotIn("-imgphotos", argv)
                self.assertNotIn("-imgcache", argv)
        self.assertEqual({n: argv for n, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, self.O, 571, "dev-clean", R1)},
                         self.cmds(R1, R1))

    def test_guard_requires_image_in_r1review_experiments(self):
        for a in pfc.arms(R1):
            c = self.cmds(a, R1)
            for _, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, self.O, 571, "dev-clean", a, candidate=R1):
                pfc.guard(argv, "dev-clean", a, self.BIN, R1)
            g = c["gen"]
            self.rejects([x for x in g if x != "-imgphotos"], a, R1)                      # photos required
            self.rejects(g + ["-imgphotos"], a, R1)                                       # once
            self.rejects(g + ["false"], a, R1)                                            # bare flag (last token)
            i = g.index("-imgcache")
            self.rejects(g[:i] + g[i + 2:], a, R1)                                        # photo pool required
            self.rejects(g[:i + 1] + ["/repo/data/image_verdicts.json"] + g[i + 2:], a, R1)
            self.rejects(g + ["-image", "cached"], a, R1)                                 # reader flags not on gen
            for n in ("orchestrate", "audit", "report7"):
                m = c[n]
                j = m.index("-image")
                for v in ("off", "live", "replay"):
                    self.rejects(m[:j + 1] + [v] + m[j + 2:], a, R1)
                k = m.index("-imgcache")
                self.rejects(m[:k] + m[k + 2:], a, R1)
                self.rejects(m[:k + 1] + ["/elsewhere/image_verdicts.json"] + m[k + 2:], a, R1)
                self.rejects(m + ["-imgphotos"], a, R1)
                self.rejects(m + ["-ImgPhotos"], a, R1)
            for n in ("sim", "augment", "classifier"):
                self.rejects(c[n] + ["-imgcache", IMGCACHE], a, R1)
                self.rejects(c[n] + ["-imgphotos"], a, R1)

    def test_guard_refuses_image_everywhere_else(self):
        for cand in (P1, P4, P5):
            for a in pfc.arms(cand):
                seed = pfc.modes(cand)["dev-clean"]["seeds"][0]
                c = self.cmds(a, cand, seed=seed)
                for argv in c.values():
                    pfc.guard(argv, "dev-clean", a, self.BIN, cand)
                self.rejects(c["gen"] + ["-imgphotos"], a, cand)
                self.rejects(c["gen"] + ["-imgphotos", "-imgcache", IMGCACHE], a, cand)
                for n in ("orchestrate", "audit", "report7"):
                    m = c[n]
                    j = m.index("-image")
                    self.rejects(m[:j + 1] + ["cached"] + m[j + 2:], a, cand)
                    self.rejects(m[:j + 1] + ["cached"] + m[j + 2:] + ["-imgcache", IMGCACHE], a, cand)
                    self.rejects(m + ["-imgcache", IMGCACHE], a, cand)
        # an image-on lane of an r1review experiment is not a lane of a pairfeat5 experiment, and the reverse
        self.rejects(self.cmds(P5, R1, seed=521)["gen"], P5, P5)
        self.rejects(self.cmds(P5, P5, seed=571)["orchestrate"], P5, R1)
        self.rejects(self.cmds(P5, P5, seed=571)["gen"], P5, R1)


class R1reviewImageBuild(unittest.TestCase):
    def build(self, root, arm_, cand, **kw):
        os.makedirs(root, exist_ok=True)
        b = {"git_head": HEAD, "binary_sha256": BIN_SHA, "floor": 0.01, "mode": "dev-clean", "arm": arm_,
             "modelset": pfc.MODELSET[arm_], "candidate": cand, "model_chain_sha256": chain(pfc.CHAIN_FILES[arm_], "c")}
        if arm_ == R1:
            b["extra_flags"] = ["-r1review"]
        b.update(kw)
        _wj(os.path.join(root, "build.json"), {k: v for k, v in b.items() if v is not None})
        return root

    def test_read_build_records_image_setting(self):
        with tempfile.TemporaryDirectory() as td:
            for a in pfc.arms(R1):
                r = self.build(os.path.join(td, a), a, R1, **IMAGE_BUILD)
                self.assertEqual(pfc.read_build(r, a, "dev-clean", R1)["image"], "cached")
                for bad in ({"image": "off"}, {"image": None}, {"imgphotos": False}, {"imgphotos": None},
                            {"image_verdicts_sha256": None}, {"image_verdicts_sha256": "x"}):
                    self.build(r, a, R1, **dict(IMAGE_BUILD, **bad))
                    with self.assertRaises(ValueError, msg=str(bad)):
                        pfc.read_build(r, a, "dev-clean", R1)
            for a in pfc.arms(P5):  # pairfeat5 experiments record no image setting
                r = self.build(os.path.join(td, "p5" + a), a, P5)
                self.assertNotIn("image", pfc.read_build(r, a, "dev-clean", P5))
                self.build(r, a, P5, **IMAGE_BUILD)
                with self.assertRaises(ValueError):
                    pfc.read_build(r, a, "dev-clean", P5)


class CLIR1reviewImage(CLIR1review):
    """The three-arm analysis requires every arm built with the image setting, on one image_verdicts.json."""

    def test_image_build_identity(self):
        b, f, r = self.three()
        for roots in ((b, f, self.make_arm(R1, 7, image={}, name="r-noimg")),
                      (self.make_arm("baseline", 5, fm=3, image={}, name="b-noimg"), f, r),
                      (b, self.make_arm(P5, 6, fm=1, image=dict(IMAGE_BUILD, image="off"), name="f-off"), r),
                      (b, self.make_arm(P5, 6, fm=1, image=dict(IMAGE_BUILD, image_verdicts_sha256="f" * 64), name="f-sha"), r)):
            with self.assertRaises(ValueError):
                self.run3("dev-clean", roots)
            self.assertFalse(os.path.exists(os.path.join(self.exp, "selection.json")))
        self.assertEqual(self.run3("dev-clean", (b, f, r)), 0)
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        self.assertEqual(s["image_verdicts_sha256"], "e" * 64)


class InitExperimentFreshParent(unittest.TestCase):
    def test_refuses_a_parent_with_decisions(self):
        for name in ("selection.json", "stress.json", "validation-manifest.json"):
            with tempfile.TemporaryDirectory() as td:
                r = Repo(td, cand=P5)
                for fn in CAND5_FILES:
                    with open(os.path.join(r.root, "models", fn), "w") as h:
                        h.write("model " + fn)
                r.artifact_manifest()
                _wj(os.path.join(r.exp, name), {"pass": False})
                with self.assertRaises(pfc.PreflightError, msg=name):
                    pfc.init_experiment(r.exp, R1)
                self.assertFalse(os.path.exists(os.path.join(r.exp, "experiment.json")))
                os.remove(os.path.join(r.exp, name))
                pfc.init_experiment(r.exp, R1)


class RunnerR1reviewImage(unittest.TestCase):
    def test_runner_passes_the_candidate_and_records_the_image_setting(self):
        with open(SCRIPT) as h:
            text = h.read()
        self.assertIn("pfc.lane_commands(root, bin_, o, seed, mode, arm, candidate=cand)", text)
        self.assertIn("pfc.image_build(root, cand)", text)
        self.assertIn('"image": pfc.lane_image_mode(cand)', text)



# ================================================================ final-review M1: lane-level evidence for arm identity

class CLIR1reviewLaneEvidence(CLIR1review):
    """Three-arm r1review analysis refuses lanes whose own outputs contradict the arm build.json claims."""

    def refused(self, roots):
        with self.assertRaises(ValueError):
            self.run3("dev-clean", roots)
        self.assertFalse(os.path.exists(os.path.join(self.exp, "selection.json")))

    def test_completion_items_only_in_the_r1review_arm(self):
        b, f, r = self.three()
        self.refused((b, self.make_arm(P5, 6, fm=1, cu=1, name="f-cu"), r))
        self.refused((self.make_arm("baseline", 5, fm=3, cu=2, answers=("adequate",), name="b-cu"), f, r))
        self.assertEqual(self.run3("dev-clean", (b, f, r)), 0)

    def test_report7_image_reader_must_be_cached(self):
        b, f, r = self.three()
        for i, ir in enumerate((None, {"mode": "off"}, {"mode": "replay"}, {}, "cached")):
            self.refused((b, f, self.make_arm(R1, 7, fm=1, img_reader=ir, name="r-img%d" % i)))
        self.refused((self.make_arm("baseline", 5, fm=3, img_reader={"mode": "off"}, name="b-img"), f, r))

    def test_decision_policy_stamps_image_and_modelset(self):
        b, f, r = self.three()
        for i, pol in enumerate(([], [lane_policy(R1, image=None)], [lane_policy(R1), lane_policy(R1, image="off")],
                                 [lane_policy(R1) + "x"], [lane_policy("baseline")], [lane_policy(R1, image="replay")])):
            self.refused((b, f, self.make_arm(R1, 7, fm=1, policies=pol, name="r-pol%d" % i)))
        self.refused((b, f, self.make_arm(R1, 7, fm=1, out_log=False, name="r-nolog")))
        self.refused((self.make_arm("baseline", 5, fm=3, policies=[lane_policy(P5)], name="b-pol"), f, r))
        self.refused((b, self.make_arm(P5, 6, fm=1, policies=[lane_policy("baseline")], name="f-pol"), r))
        self.assertEqual(self.run3("dev-clean", (b, f, r)), 0)

    def test_pairfeat5_experiment_lanes_need_no_image_evidence(self):
        # a pairfeat5 experiment lane (no out_log, no image_reader) still loads exactly as before
        lane = os.path.join(self.make_arm(P5, 6, fm=1, img_reader=None, out_log=False), "v2-s571")
        pfc._load_lane(lane, 571, P5, "dev-clean", P5)
        with self.assertRaises(ValueError):
            pfc._load_lane(lane, 571, P5, "dev-clean", R1)



# ================================================================ amendment 3: r1review gated against pairfeat5 only, fresh seeds

OLD_R1_SEEDS = (557, 563, 569, 601, 607, 613, 617, 619)


class R1reviewAmendment3(unittest.TestCase):
    ROOT = "/repo"
    BIN = "/repo/bin/wsi7"
    O = "/data/exp/arm/v2-s571"

    def test_fresh_seeds_and_retired_list(self):
        self.assertEqual(pfc.mode_settings("dev-clean", R1)["seeds"], [571, 577, 587])
        self.assertEqual(pfc.mode_settings("dev-stress", R1)["seeds"], [571, 577, 587])
        self.assertEqual(pfc.mode_settings("validation-clean", R1)["seeds"], [631, 641, 643, 647, 653])
        self.assertEqual(pfc.RETIRED_SEEDS, OLD_R1_SEEDS)
        for c, spec in pfc.CANDIDATES.items():
            self.assertFalse(set(spec["dev_seeds"] + spec.get("validation_seeds", [])) & set(OLD_R1_SEEDS), c)
        self.assertIs(pfc.baseline_gating(R1), False)
        for c in (P1, P2, P3, P4, P5):
            self.assertIs(pfc.baseline_gating(c), True)

    def test_new_seeds_accepted_old_seeds_refused(self):
        for mode in pfc.MODES:
            for a in pfc.arms(R1):
                for seed in pfc.modes(R1)[mode]["seeds"]:
                    for _, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, mode, a, candidate=R1):
                        pfc.guard(argv, mode, a, self.BIN, R1)
                for seed in OLD_R1_SEEDS:
                    c = {n: argv for n, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, mode, a, candidate=R1)}
                    for n in ("gen", "orchestrate"):
                        with self.assertRaises(pfc.GuardError, msg=(mode, a, seed, n)):
                            pfc.guard(c[n], mode, a, self.BIN, R1)
        # retired seeds are refused in every experiment, whatever its seed table
        for cand in (P1, P5):
            for seed in OLD_R1_SEEDS:
                for n, argv, _ in pfc.lane_commands(self.ROOT, self.BIN, self.O, seed, "dev-clean", cand):
                    if n in ("gen", "orchestrate"):
                        with self.assertRaises(pfc.GuardError) as e:
                            pfc.guard(argv, "dev-clean", cand, self.BIN, cand)
                        self.assertIn("retired", str(e.exception))

    def test_runner_seed_table(self):
        with open(SCRIPT) as h:
            text = h.read()
        self.assertIn('r1review) DEV_SEEDS="571 577 587"; VAL_SEEDS="631 641 643 647 653"; CHAIN=pairfeat5; REF=pairfeat5;;', text)
        table = text.split('case "$CAND" in', 1)[1].split("esac", 1)[0]
        for sd in OLD_R1_SEEDS:
            self.assertNotIn(str(sd), table)

    def test_baseline_gate_reported_not_gating(self):
        base = cap_arm("baseline", 500, 600, 10, 0, 5)
        for kind in pfc.GATE_KINDS:
            g = pfc.gate(base, ref_arm(R1, 540, fm=5), kind)       # prevented 0: the cap gate fails vs baseline
            self.assertFalse(g["pass"])
            self.assertIs(g["gating"], False)
        # pairfeat5 (and every other two-arm candidate) is still gated vs baseline, with unchanged gate JSON
        g5 = pfc.gate(base, cap_arm(P5, 530, 600, 10, 0, 5), "selection")
        self.assertFalse(g5["pass"])
        self.assertNotIn("gating", g5)
        s5 = pfc.select(base, cap_arm(P5, 530, 600, 10, 0, 5))
        self.assertEqual((s5["selected_modelset"], s5["pass"]), (None, False))


class CLIR1reviewAmendment3(CLIR1review):
    def baseline_failing(self, mode="dev-clean", r=7):
        # baseline fm 1 = r1review fm 1: no prevented false merge, so gate 1 (vs baseline) fails; gate 2 passes
        return (self.make_arm("baseline", 5, mode=mode, fm=1), self.make_arm(P5, 6, mode=mode, fm=1),
                self.make_arm(R1, r, mode=mode, fm=1, cu=3, answers=("adequate", "inadequate")))

    def test_dev_clean_passes_when_only_the_baseline_gate_fails(self):
        self.assertEqual(self.run3("dev-clean", self.baseline_failing()), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertFalse(sel["gate"]["pass"])
        self.assertIs(sel["gate"]["gating"], False)
        self.assertIn("false_merge_pairs not reduced (prevented 0)", sel["gate"]["reasons"])
        self.assertTrue(sel["reference_gate"]["pass"])
        self.assertEqual((sel["selected_modelset"], sel["pass"]), (R1, True))
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        self.assertTrue(s["pass"])
        self.assertIs(s["gate"]["gating"], False)

    def test_dev_clean_fails_when_the_reference_gate_fails(self):
        self.assertEqual(self.run3("dev-clean", self.baseline_failing(r=6)), 0)   # gain vs pairfeat5 0pp
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual((sel["selected_modelset"], sel["pass"]), (None, False))

    def test_stress_and_validation_follow_the_reference_gate(self):
        r = Repo(os.path.join(self.td.name, "v"), cand=P5)
        for fn in CAND5_FILES:
            with open(os.path.join(r.root, "models", fn), "w") as h:
                h.write("model " + fn)
        self.exp = r.exp
        self.maps = {"baseline": r.hashes(BASE_FILES), P5: r.hashes(CAND5_FILES), R1: r.hashes(CAND5_FILES)}
        r.artifact_manifest()
        pfc.init_experiment(r.exp, R1)
        self.assertEqual(self.run3("dev-clean", self.baseline_failing()), 0)
        self.assertEqual(self.run3("dev-stress", self.baseline_failing(mode="dev-stress")), 0)
        st = _rj(os.path.join(r.exp, "stress.json"))
        self.assertEqual((st["pass"], st["gate"]["pass"], st["reference_gate"]["pass"]), (True, False, True))
        pfc.freeze_validation(r.exp, r.root, HEAD)
        out = os.path.join(r.exp, "validation-clean-summary.json")
        v = self.baseline_failing(mode="validation-clean")
        res = pfc.analyse("validation-clean", out, {"baseline": v[0], P5: v[1], R1: v[2]}, repo=r.root, head=HEAD)
        self.assertEqual((res["pass"], res["gate"]["pass"], res["reference_gate"]["pass"]), (True, False, True))
        self.assertEqual(res["seeds"], [631, 641, 643, 647, 653])



# ================================================================ amendment 3 fix: SAFETY zero in the baseline arm too

def _bad_false_closure(rj):  # one false closure; tp (pred_closed - false_closures) unchanged
    rj["programmes_live"]["crc"]["false_closures"] = 1
    rj["programmes_live"]["crc"]["pred_closed"] += 1


def _bad_wrong_release(rj):
    rj["outreach_live"]["wrong_patient_released"] = 1


def _bad_by_origin(rj):
    rj["outreach_live"]["by_origin"]["a"]["wrong_released"] = 1


def _bad_replay(rj):
    rj["live_vs_replay_final_mismatch"] = 1


BASELINE_SAFETY_KEYS = pfc.SAFETY + ("wrong_released_by_origin",)


class BaselineSafetyR1review(unittest.TestCase):
    def test_every_baseline_safety_key_blocks_every_kind(self):
        base_ok = cap_arm("baseline", 500, 600, 10, 0, 5)
        for k in BASELINE_SAFETY_KEYS:
            base = cap_arm("baseline", 500, 600, 10, 0, 5, **({k: {"a": 1}} if k == "wrong_released_by_origin" else {k: 1}))
            for kind in pfc.GATE_KINDS:
                rg = pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 540), kind, base=base)
                self.assertFalse(rg["pass"], (k, kind))
                self.assertEqual(rg["reasons"], ["baseline " + k + " nonzero"], (k, kind))
                self.assertTrue(pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 540), kind, base=base_ok)["pass"])
            s = pfc.select(base, ref_arm(R1, 540), ref_arm(P5, 530))
            self.assertEqual((s["pass"], s["selected_modelset"]), (False, None), k)
            self.assertIn("baseline " + k + " nonzero", s["reference_gate"]["reasons"])
        # without a baseline arm the reference gate is the one reviewed before (JSON unchanged)
        self.assertEqual(pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 540), "selection"),
                         pfc.reference_gate(ref_arm(P5, 530), ref_arm(R1, 540), "selection", base=base_ok))


class CLIR1reviewBaselineSafety(CLIR1review):
    BAD = (("false_closures", _bad_false_closure), ("wrong_released_total", _bad_wrong_release),
           ("wrong_released_by_origin", _bad_by_origin), ("replay_mismatch", _bad_replay))

    def arms3(self, mode, bad=None, tag=""):
        # gate 1 fails on purpose (no prevented false merge) so only the baseline safety check can block the pass
        return (self.make_arm("baseline", 5, mode=mode, fm=1, r7=bad, name="%s-b%s" % (mode, tag)),
                self.make_arm(P5, 6, mode=mode, fm=1, name="%s-f%s" % (mode, tag)),
                self.make_arm(R1, 7, mode=mode, fm=1, cu=3, answers=("adequate",), name="%s-r%s" % (mode, tag)))

    def test_dev_clean_refuses_baseline_safety(self):
        for k, bad in self.BAD:
            sp = os.path.join(self.exp, "selection.json")
            if os.path.exists(sp):
                os.remove(sp)
            self.assertEqual(self.run3("dev-clean", self.arms3("dev-clean", bad, k), out=os.path.join(self.exp, k + ".json")), 0)
            sel = _rj(sp)
            self.assertEqual((sel["pass"], sel["selected_modelset"]), (False, None), k)
            self.assertIn("baseline " + k + " nonzero", sel["reference_gate"]["reasons"])

    def setup_repo(self):
        r = Repo(os.path.join(self.td.name, "v"), cand=P5)
        for fn in CAND5_FILES:
            with open(os.path.join(r.root, "models", fn), "w") as h:
                h.write("model " + fn)
        self.exp = r.exp
        self.maps = {"baseline": r.hashes(BASE_FILES), P5: r.hashes(CAND5_FILES), R1: r.hashes(CAND5_FILES)}
        r.artifact_manifest()
        pfc.init_experiment(r.exp, R1)
        return r

    def test_stress_refuses_baseline_safety(self):
        for k, bad in self.BAD:
            with self.subTest(k=k):
                self.td.cleanup()
                self.td = tempfile.TemporaryDirectory()
                self.setup_repo()
                self.assertEqual(self.run3("dev-clean", self.arms3("dev-clean")), 0)
                self.assertTrue(_rj(os.path.join(self.exp, "selection.json"))["pass"])
                self.assertEqual(self.run3("dev-stress", self.arms3("dev-stress", bad)), 0)
                st = _rj(os.path.join(self.exp, "stress.json"))
                self.assertFalse(st["pass"], k)
                self.assertIn("baseline " + k + " nonzero", st["reference_gate"]["reasons"])

    def test_validation_refuses_baseline_safety(self):
        r = self.setup_repo()
        self.assertEqual(self.run3("dev-clean", self.arms3("dev-clean")), 0)
        self.assertEqual(self.run3("dev-stress", self.arms3("dev-stress")), 0)
        pfc.freeze_validation(r.exp, r.root, HEAD)
        for k, bad in self.BAD:
            out = os.path.join(r.exp, "validation-%s.json" % k)
            v = self.arms3("validation-clean", bad, k)
            res = pfc.analyse("validation-clean", out, {"baseline": v[0], P5: v[1], R1: v[2]}, repo=r.root, head=HEAD)
            self.assertFalse(res["pass"], k)
            self.assertIn("baseline " + k + " nonzero", res["reference_gate"]["reasons"])
        v = self.arms3("validation-clean", None, "ok")
        res = pfc.analyse("validation-clean", os.path.join(r.exp, "validation-ok.json"), {"baseline": v[0], P5: v[1], R1: v[2]},
                          repo=r.root, head=HEAD)
        self.assertTrue(res["pass"])


if __name__ == "__main__":
    unittest.main()
