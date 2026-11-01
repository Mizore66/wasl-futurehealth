"""Paired comparison of the booking optimisers on materialised pipeline runs (SIMULATED capacity).

usage: python booking/sim/lookahead_eval.py <out_dir> <run_dir> [<run_dir> ...] [--seeds 2026,99] [--jobs 4] [--grid main|ablation]

For every run dir (a pipeline run: out_log.ndjson + fhir_log.ndjson), calendar seed, capacity condition and stress
test, it runs booking/suggest.py's stage in-process once per optimiser on the SAME open cases, synthetic calendar and
request days, and records the existing booking metrics. Writes <out_dir>/raw.json (one record per run) and
<out_dir>/summary.json + summary.md (means over replicates and paired differences vs the default myopic engine).
Capacity is synthetic (booking/suggest.py calendar); every number here is simulated decision support.
Needs a built bin/wsi7 (scripts/demo.sh build) for the lookahead's Go min-cost-flow solver.
"""
from __future__ import annotations
import argparse, collections, contextlib, io, json, os, sys
from concurrent.futures import ProcessPoolExecutor
import numpy as np

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
import suggest  # noqa: E402

CONDITIONS = {"adequate": ["--slots-per-fit", "1.2"], "misallocated": ["--slots-per-fit", "1.2", "--misallocated"],
              "short_overall": ["--slots-per-fit", "0.8"]}
STRESS = {"base": [], "bursty": ["--arrivals", "bursty"],
          "scarce_female": ["--female-share", "0.10", "--colpo-female-share", "0.20"],
          "urgent_mix": ["--crc-urgent-share", "0.2"]}
OPTIMISERS = {"myopic": [], "greedy": ["--optimiser", "greedy"], "lookahead": ["--optimiser", "lookahead"],
              "oracle": ["--optimiser", "lookahead", "--forecast", "oracle"]}
ABLATION = {"lookahead_g0.5": ["--optimiser", "lookahead", "--gamma", "0.5"],
            "lookahead_g1.0": ["--optimiser", "lookahead", "--gamma", "1.0"],
            "lookahead_L14": ["--optimiser", "lookahead", "--lookahead-days", "14"],
            "lookahead_L56": ["--optimiser", "lookahead", "--lookahead-days", "56"],
            "lookahead_noprotect": ["--optimiser", "lookahead", "--female-protect", "0"],
            "myopic_noprotect": ["--female-protect", "0"]}


def _q(xs, q):
    return float(np.percentile(xs, q)) if xs else None


def share(xs, ok):
    return round(sum(1 for c in xs if ok(c)) / len(xs), 4) if xs else None


def metrics(res):
    crc = [c for c in res["cases"].values() if c["procedure"] == "colonoscopy"]
    cer = [c for c in res["cases"].values() if c["procedure"] == "colposcopy"]
    ok = lambda c: c["status"] == "suggested" and c["booking"]["within_deadline"]
    sug = [c for c in crc if c["status"] == "suggested"]
    waits = [c["wait_days"] for c in sug]
    late = [c["booking"]["day"] - c["booking"]["deadline"] for c in sug if not c["booking"]["within_deadline"]]
    hard = [c for c in crc if c["constraints"]["female_endoscopist"] == "hard"]
    other = [c for c in crc if c["constraints"]["female_endoscopist"] != "hard"]
    urg = [c for c in crc if c.get("target_days") == 14]
    m = {"crc_cases": len(crc), "within_target_share": share(crc, ok), "no_slot": len(crc) - len(sug),
         "cross_region": sum(1 for c in sug if c["cross_region"]),
         "overbooked": sum(1 for c in sug if c["booking"]["overbooked"]),
         "slots": res["capacity"]["slots"],
         "slots_unused": res["capacity"]["slots"] - (len(sug) - sum(1 for c in sug if c["booking"]["overbooked"])),
         "median_wait": _q(waits, 50), "p90_wait": _q(waits, 90), "max_wait": max(waits) if waits else None,
         "late_n": len(late), "late_mean_days": float(np.mean(late)) if late else 0.0,
         "female_hard_share": share(hard, ok), "female_other_share": share(other, ok)}
    m["female_gap_pp"] = (round(100 * (m["female_other_share"] - m["female_hard_share"]), 2)
                          if hard and other else None)
    if urg:
        us = [c for c in urg if c["status"] == "suggested"]
        m.update(urgent_n=len(urg), urgent_within_share=share(urg, ok), urgent_median_wait=_q([c["wait_days"] for c in us], 50),
                 urgent_p90_wait=_q([c["wait_days"] for c in us], 90))
    cok = lambda c: c["status"] == "suggested" and c["within_window"]
    first = [c for c in cer if not c["repeat_after_unsatisfactory"]]
    curg = [c for c in first if c["window_days"][1] - c["window_days"][0] <= 14]
    cus = [c for c in curg if c["status"] == "suggested"]
    chard = [c for c in cer if c["constraints"]["female_endoscopist"] == "hard"]
    m.update(cerv_cases=len(cer), cerv_within_share=share(cer, cok), cerv_no_slot=sum(1 for c in cer if c["status"] != "suggested"),
             cerv_urgent_n=len(curg), cerv_urgent_within_share=share(curg, cok),
             cerv_urgent_median_wait=_q([c["wait_days"] for c in cus], 50), cerv_urgent_p90_wait=_q([c["wait_days"] for c in cus], 90),
             cerv_repeat_within_share=share([c for c in cer if c["repeat_after_unsatisfactory"]], cok),
             cerv_female_hard_within_share=share(chard, cok),
             cerv_cross_region=sum(1 for c in cer if c["status"] == "suggested" and c["cross_region"]))
    return m


_CACHE = {}


def job(args):
    run, seed, cond, stress, opt, argv = args
    if run not in _CACHE:
        _CACHE.clear()
        _CACHE[run] = suggest.load(run)
    with contextlib.redirect_stdout(io.StringIO()):
        res = suggest.main([run, "--seed", str(seed)] + argv, preloaded=_CACHE[run], write=False, quiet=True)
    return {"run": os.path.basename(os.path.normpath(run)), "seed": seed, "condition": cond, "stress": stress,
            "optimiser": opt, "condition_reported": res["capacity"]["condition"], **metrics(res)}


KEYS = ["within_target_share", "no_slot", "cross_region", "female_gap_pp", "female_hard_share", "median_wait", "p90_wait",
        "late_mean_days", "slots_unused", "overbooked", "urgent_within_share", "urgent_p90_wait", "cerv_within_share",
        "cerv_urgent_within_share", "cerv_urgent_p90_wait", "cerv_repeat_within_share", "cerv_female_hard_within_share"]


def summarise(recs, base="myopic"):
    by = collections.defaultdict(dict)
    for r in recs:
        by[(r["condition"], r["stress"], r["run"], r["seed"])][r["optimiser"]] = r
    cells = collections.defaultdict(lambda: collections.defaultdict(list))
    for (cond, stress, _, _), opts in by.items():
        for opt, r in opts.items():
            for k in KEYS:
                if r.get(k) is not None:
                    cells[(cond, stress, opt)][k].append(r[k])
                    b = opts.get(base, {}).get(k)
                    if opt != base and b is not None:
                        cells[(cond, stress, opt)]["d_" + k].append(r[k] - b)
    out = {}
    for (cond, stress, opt), d in sorted(cells.items()):
        row = {"n": len(d.get("within_target_share", []))}
        for k, xs in d.items():
            row[k] = round(float(np.mean(xs)), 4)
            if k.startswith("d_"):
                row[k + "_min"], row[k + "_max"] = round(float(min(xs)), 4), round(float(max(xs)), 4)
                row[k + "_pos"], row[k + "_neg"] = sum(1 for x in xs if x > 1e-9), sum(1 for x in xs if x < -1e-9)
        out[f"{cond}|{stress}|{opt}"] = row
    return out


def table(summ):
    cols = [("within_target_share", "CRC ≤target", 3), ("d_within_target_share", "Δ", 3), ("cross_region", "x-region", 0),
            ("female_gap_pp", "fem gap pp", 1), ("p90_wait", "p90 wait", 0), ("late_mean_days", "late mean d", 1),
            ("no_slot", "no slot", 0), ("slots_unused", "unused", 0), ("urgent_within_share", "CRC urgent ≤14", 3),
            ("cerv_within_share", "cerv in window", 3), ("d_cerv_within_share", "Δ", 3),
            ("cerv_urgent_within_share", "cerv 14-d", 3), ("cerv_urgent_p90_wait", "cerv 14-d p90", 0)]
    lines = ["| condition | stress | optimiser | n | " + " | ".join(c[1] for c in cols) + " |",
             "|" + "---|" * (4 + len(cols))]
    for key, r in summ.items():
        cond, stress, opt = key.split("|")
        vals = []
        for k, _, nd in cols:
            v = r.get(k)
            if v is None:
                vals.append("")
            elif k.startswith("d_"):
                vals.append(f"{v:+.{nd}f} ({r.get(k + '_pos', 0)}+/{r.get(k + '_neg', 0)}-)")
            else:
                vals.append(f"{v:.{nd}f}")
        lines.append(f"| {cond} | {stress} | {opt} | {r['n']} | " + " | ".join(vals) + " |")
    return "\n".join(lines)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("out")
    ap.add_argument("runs", nargs="+")
    ap.add_argument("--seeds", default="2026")
    ap.add_argument("--jobs", type=int, default=4)
    ap.add_argument("--grid", choices=["main", "ablation"], default="main")
    a = ap.parse_args()
    os.makedirs(a.out, exist_ok=True)
    opts = OPTIMISERS if a.grid == "main" else {"myopic": [], **ABLATION}
    stresses = STRESS if a.grid == "main" else {"base": [], "scarce_female": STRESS["scarce_female"]}
    jobs = [(run, int(seed), cond, st, opt, CONDITIONS[cond] + stresses[st] + opts[opt])
            for run in a.runs for seed in a.seeds.split(",") for cond in CONDITIONS for st in stresses for opt in opts]
    per_run = len(jobs) // len(a.runs)
    with ProcessPoolExecutor(a.jobs) as ex:   # chunks of one run dir per worker keep its parsed logs cached
        recs = list(ex.map(job, jobs, chunksize=max(1, -(-per_run // a.jobs))))
    json.dump(recs, open(os.path.join(a.out, "raw.json"), "w"), indent=0)
    summ = summarise(recs)
    json.dump(summ, open(os.path.join(a.out, "summary.json"), "w"), indent=1)
    md = table(summ)
    open(os.path.join(a.out, "summary.md"), "w").write(
        "# Booking optimisers, paired on the same cases and calendars (SIMULATED capacity)\n\n"
        f"{len(a.runs)} runs x seeds {a.seeds}; Δ = mean paired difference vs myopic (runs where it was higher+/lower-).\n\n" + md + "\n")
    print(md)


if __name__ == "__main__":
    main()
