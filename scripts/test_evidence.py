"""Checks web/data/evidence.json: contract shape, and that every number traces to the frozen summary, the per-seed
reports, the deployed demo run or a cited parameter. Run: python3 -m unittest scripts/test_evidence.py"""
import glob
import json
import math
import os
import sys
import unittest
from statistics import mean, stdev

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
sys.path.insert(0, HERE)
import evidence  # noqa: E402


def load(p):
    with open(os.path.join(ROOT, p), encoding="utf-8") as f:
        return json.load(f)


def leaves(x, path=""):
    if isinstance(x, dict):
        for k, v in x.items():
            yield from leaves(v, f"{path}.{k}")
    elif isinstance(x, list):
        for i, v in enumerate(x):
            yield from leaves(v, f"{path}[{i}]")
    elif isinstance(x, (int, float)) and not isinstance(x, bool):
        yield path, x


def close(a, b, tol=0.051):
    return a is not None and b is not None and abs(a - b) <= tol


class EvidenceTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.ev = load(evidence.OUT_JSON)
        cls.S = load(evidence.SUMMARY)
        cls.demo = load(evidence.DEMO)["metrics"]["crc_kpi_31d"]
        cls.checked = set()

    def mark(self, path):
        self.checked.add(path)

    def test_0_committed_json_is_current(self):
        files = evidence.outputs()
        self.assertEqual(json.loads(files[evidence.OUT_JSON]), self.ev, "run python3 scripts/evidence.py")

    def test_shape(self):
        ev = self.ev
        rows = [x["row"] for x in ev["impact"]["per_1000"][:5]]
        for r, want in zip(rows, ("Completions seen", "Share of true completions seen", "Patients wrongly flagged", "Worklist size", "Worklist precision")):
            self.assertTrue(r.startswith(want), r)
        self.assertIn("frozen v2", rows[0])
        self.assertEqual(set(ev), {"generated", "impact", "accuracy", "benchmarks", "claims"})
        self.assertEqual(set(ev["generated"]), {"revision", "frozen_at", "source", "generated_by"})
        self.assertEqual(ev["generated"]["revision"], self.S["manifest"]["code_revision"])
        self.assertEqual(ev["generated"]["frozen_at"], self.S["manifest"]["frozen_at"])
        for x in ev["impact"]["per_1000"]:
            self.assertEqual(set(x), {"row", "eid_only", "wasl", "truth", "unit"})
        self.assertEqual(set(ev["impact"]["calibration"]), {"label", "value", "source", "url", "caveat"})
        m = ev["impact"]["modelled"]
        self.assertEqual(set(m), {"label", "scenarios", "tornado", "parameters"})
        self.assertIn("conditional on navigation acting on the corrected list", m["label"])
        self.assertEqual([s["name"] for s in m["scenarios"]], ["worst", "base", "best"])
        for s in m["scenarios"]:
            self.assertEqual(set(s), {"name", "uplift_pp", "ppv", "extra_colonoscopies_per_1000", "cancers_found_per_1000"})
        for t in m["tornado"]:
            self.assertEqual(set(t), {"input", "low", "high", "out_low", "out_high"})
            self.assertLessEqual(t["out_low"], t["out_high"])
        for p in m["parameters"]:
            self.assertEqual(set(p), {"name", "value", "scope", "source", "url"})
            self.assertIn(p["scope"], {"UAE", "INTL", "ASSUMED"})
            if p["scope"] != "ASSUMED":
                self.assertTrue(p["url"].startswith("https://"), p["name"])
        a = ev["accuracy"]
        self.assertEqual(set(a), {"definitions", "linkage", "safety", "worst"})
        terms = " ".join(d["term"] for d in a["definitions"]).lower()
        for t in ("false closure", "false open", "wrong-patient merge", "missed match"):
            self.assertIn(t, terms)
        for x in a["linkage"]:
            self.assertEqual(set(x), {"noise", "metric", "value", "low", "high"})
            self.assertIn(x["noise"], {"v1", "v2"})
        for n in ("v1", "v2"):
            rows = [x["metric"] for x in a["linkage"] if x["noise"] == n]
            self.assertIn("F1", rows[-1], "F1 comes last")
        for x in a["safety"]:
            self.assertEqual(set(x), {"metric", "events", "denominator", "upper_95"})
            self.assertIsInstance(x["events"], int)
        self.assertIn("false closures", a["safety"][0]["metric"])
        for x in ev["claims"]:
            self.assertEqual(set(x), {"id", "claim", "status", "evidence", "reproduce"})
            self.assertIn(x["status"], {"built", "simulated", "designed"})
        self.assertTrue(10 <= len(ev["claims"]) <= 15)
        text = json.dumps(ev).lower()
        self.assertNotIn("odds ratio", text)
        self.assertNotIn("from 11% to 31%", text)

    def test_numbers_trace(self):
        ev, S, N = self.ev, self.S, self.S["by_noise"]
        expect = {}
        # impact.per_1000: 5 rows each for v1 mean, v2 mean, deployed run
        rates = [(N[n]["metrics"]["kpi_eid_only"][0], N[n]["metrics"]["kpi_wasl"][0], N[n]["metrics"]["kpi_truth"][0]) for n in ("v2", "v1")]
        rates.append(tuple(self.demo[k]["kpi_all_referrals"] for k in ("eid_only", "wasl_live", "truth")))
        for b, (e, w, t) in enumerate(rates):
            rows = [(1000 * e, 1000 * w, 1000 * t), (100 * e / t, 100 * w / t, 100), (1000 * (t - e), 1000 * (t - w), 0),
                    (1000 * (1 - e), 1000 * (1 - w), 1000 * (1 - t)), (100 * (1 - t) / (1 - e), 100 * (1 - t) / (1 - w), None)]
            for i, vals in enumerate(rows):
                for k, v in zip(("eid_only", "wasl", "truth"), vals):
                    expect[f".impact.per_1000[{5 * b + i}].{k}"] = v
        self.assertEqual(self.demo["eid_only"]["kpi_all_referrals"], 0.113)  # the deployed-run numbers quoted in the docs
        self.assertEqual(self.demo["wasl_live"]["kpi_all_referrals"], 0.307)
        self.assertEqual(self.demo["truth"]["kpi_all_referrals"], 0.319)
        expect[".impact.calibration.value"] = 0.305  # PARAMETERS[0], SEHA BMC Cancer 2021
        cited_up, cited_ppv = {0, 3, 11}, {0.025, 0.055}  # PARAMETERS[2:6]
        for i, s in enumerate(ev["impact"]["modelled"]["scenarios"]):
            self.assertIn(s["uplift_pp"], cited_up); self.assertIn(s["ppv"], cited_ppv)
            expect[f".impact.modelled.scenarios[{i}].uplift_pp"] = s["uplift_pp"]
            expect[f".impact.modelled.scenarios[{i}].ppv"] = s["ppv"]
            expect[f".impact.modelled.scenarios[{i}].extra_colonoscopies_per_1000"] = s["uplift_pp"] * 10
            expect[f".impact.modelled.scenarios[{i}].cancers_found_per_1000"] = s["uplift_pp"] * 10 * s["ppv"]
        products = {round(u * 10 * p, 2) for u in cited_up - {0} for p in cited_ppv}
        for i, t in enumerate(ev["impact"]["modelled"]["tornado"]):
            for k in ("out_low", "out_high"):
                self.assertIn(t[k], products)
                expect[f".impact.modelled.tornado[{i}].{k}"] = t[k]
        # accuracy.linkage
        seeds = {}
        for n in ("v1", "v2"):
            seeds[n] = [load(os.path.relpath(p, ROOT)) for p in sorted(glob.glob(os.path.join(ROOT, f"pipeline/results/final/{n}-s*/report7.json")))]
            self.assertEqual(len(seeds[n]), len(S["manifest"]["seeds"]))

        def tci(xs):
            m, s = mean(xs), stdev(xs)
            h = evidence.T95[len(xs) - 1] * s / math.sqrt(len(xs))
            return m, m - h, m + h
        for i, x in enumerate(ev["accuracy"]["linkage"]):
            n, met, m = x["noise"], x["metric"], N[x["noise"]]["metrics"]
            if met.startswith("Linkage pairs"):
                key = "pair_precision" if "false-match" in met else "pair_recall"
                v, lo, hi = tci([r["linkage"][key] for r in seeds[n]])
                src = (1 - v, 1 - hi, 1 - lo)
            elif "F1" in met:
                src = m["linkage_pair_f1"]
            elif "review items" in met:
                src = m["review_items_per_100_loops"]
            elif "reviewer hours" in met:
                src = m["reviewer_h_per_1k"]
            else:
                prog = next(p for p in evidence.PROGRAMMES if met.startswith(p[0] + ":"))
                v = m[prog[1] if "false-match" in met else prog[2]]
                src = (1 - v[0], 1 - v[2], 1 - v[1])
            for k, s in zip(("value", "low", "high"), src):
                expect[f".accuracy.linkage[{i}].{k}"] = s
        # accuracy.safety
        for i, x in enumerate(ev["accuracy"]["safety"]):
            n = x["metric"][1:3]
            sp = N[n]["safety_pooled"]
            if "merge pairs" in x["metric"]:
                expect[f".accuracy.safety[{i}].events"] = sum(r["linkage"]["false_merge_pairs"] for r in seeds[n])
                continue
            if "Tamper" in x["metric"]:
                det, att, _ = sp["tamper_detected"]
                src = (att - det, att, None)
            else:
                label = x["metric"][5:]
                key = next(iter([p[3] for p in evidence.PROGRAMMES if label == p[0] + " false closures"] +
                           [k for l, k in evidence.SAFETY_EXTRA if l == label]))
                src = sp[key]
            for k, s in zip(("events", "denominator", "upper_95"), src):
                if s is not None:
                    expect[f".accuracy.safety[{i}].{k}"] = s
        for i, b in enumerate(ev["benchmarks"]):  # copied verbatim from pipeline/results/benchmarks/summary.json
            src = load(evidence.BENCH)[i]
            for k in ("wasl", "baseline"):
                if isinstance(b.get(k), (int, float)):
                    expect[f".benchmarks[{i}].{k}"] = src[k]
        found = dict(leaves(ev))
        for path, v in found.items():
            self.assertIn(path, expect, f"untraced number at {path} = {v}")
            self.assertTrue(close(v, expect[path], 0.051 if "per_1000" in path else 1e-4), f"{path}: {v} != {expect[path]}")

    def test_safety_order_false_closures_first(self):
        for n in ("v1", "v2"):
            rows = [x["metric"] for x in self.ev["accuracy"]["safety"] if x["metric"].startswith(f"[{n}]")]
            k = len(evidence.PROGRAMMES)
            self.assertTrue(all("false closures" in r for r in rows[:k]))

    def test_docs_rendered(self):
        md = evidence.read(os.path.join(ROOT, evidence.OUT_MD))
        for s in ("075abc5ad04a", "2026-11-02T04:33Z", "modelled, conditional on navigation acting on the corrected list",
                  "raises the measured rate toward the true rate", "Tier A", "not applicable: synthetic only".capitalize().split(":")[0]):
            self.assertIn(s, md)
        for f in ("impact_per_1000", "missed_match", "safety_upper", "seed_gap", "tornado"):
            self.assertTrue(os.path.exists(os.path.join(ROOT, evidence.OUT_SVG, f + ".svg")))


if __name__ == "__main__":
    unittest.main()
