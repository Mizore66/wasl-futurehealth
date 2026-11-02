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
        self.assertEqual(pfc.CANDIDATES, {"pairfeat1": {"dev_seeds": [307, 311, 313]}, "pairfeat2": {"dev_seeds": [317, 331, 337]},
                                         "pairfeat3": {"dev_seeds": [347, 353, 367]}})
        self.assertEqual(pfc.VALIDATION_SEEDS, [401, 409, 419, 421, 431])
        self.assertEqual(pfc.ALLOWED_ARMS, ("baseline", "pairfeat1", "pairfeat2", "pairfeat3"))
        self.assertEqual(pfc.modes(P2), {
            "dev-clean": {"split": "dev", "seeds": [317, 331, 337], "rerr": "0", "errungraded": "0"},
            "dev-stress": {"split": "dev", "seeds": [317, 331, 337], "rerr": "0.04", "errungraded": "0.25"},
            "validation-clean": {"split": "eval", "seeds": [401, 409, 419, 421, 431], "rerr": "0", "errungraded": "0"},
        })
        self.assertEqual(pfc.arms(P2), ("baseline", "pairfeat2"))
        self.assertEqual(pfc.CHAIN_FILES[P2], CAND2_FILES)
        self.assertEqual(pfc.CHAIN_KEY[P2], "pairfeat2_chain_sha256")
        self.assertEqual(pfc.MODELSET, {"baseline": "", "pairfeat1": "pairfeat1", "pairfeat2": "pairfeat2",
                                        "pairfeat3": "pairfeat3"})
        for bad in ("baseline", "pairfeat4", "", None):
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
                    {"baseline_chain_sha256": b, "pairfeat4_chain_sha256": {}},
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
        for bad in ("pairfeat4", "baseline", "", None):
            with self.assertRaises(pfc.GuardError):
                pfc.guard(self.cmds()[0][1], "dev-clean", "baseline", self.BIN, bad)

    def test_funnel_modelset(self):
        pfc.check_funnel_modelset({"config": {"modelset": "pairfeat2"}}, P2)
        pfc.check_funnel_modelset({"config": {"modelset": ""}}, "baseline")
        for f, a in (({"config": {"modelset": "pairfeat1"}}, P2), ({"config": {"modelset": ""}}, P2),
                     ({"config": {"modelset": "pairfeat2"}}, "baseline"), ({"config": {"modelset": "pairfeat2"}}, P1),
                     ({"config": {}}, P2), ({"config": {"modelset": "pairfeat2"}}, "pairfeat4")):
            with self.assertRaises(ValueError):
                pfc.check_funnel_modelset(f, a)

    def test_gate_and_selection_record(self):
        base = pfc_arm = arm("baseline", 500, 600)
        g = pfc.gate(base, arm(P2, 512, 600, distinct_pairs=110), "selection")
        self.assertTrue(g["pass"])
        self.assertEqual(g["distinct_pairs"], {"baseline": 100, "pairfeat2": 110, "max_allowed": 110.0})
        self.assertFalse(pfc.gate(base, arm(P2, 511, 600), "selection")["pass"])
        self.assertFalse(pfc.gate(arm(P2, 500, 600), arm(P2, 530, 600), "selection")["pass"])
        self.assertFalse(pfc.gate(base, arm("pairfeat4", 530, 600), "selection")["pass"])
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
                  dict({k: v for k, v in good.items() if k != "pairfeat2_chain_sha256"}, pairfeat4_chain_sha256={})):
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
        self.assertEqual(pfc.main(["dev-clean", out, "baseline=" + b, "pairfeat4=" + c]), 2)
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
                  {"git_head": self.head, "baseline_chain_sha256": base, "pairfeat4_chain_sha256": chain(CAND2_FILES, "c")}):
            _wj(ap, m)
            for a in ("baseline", P1, P2):
                self.assert_refused(self.run_sh(self.root, "dev-clean", a, preflight_only=True), "candidate")
        self.assert_refused(self.run_sh(self.root, "dev-clean", "pairfeat4"))

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
        self.assertEqual(pfc.CANDIDATES[P3], {"dev_seeds": [347, 353, 367]})
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


if __name__ == "__main__":
    unittest.main()
