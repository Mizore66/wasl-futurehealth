"""Summarise raw_main.json -> summary.json (median [2.5, 97.5] percentile over parameter draws)."""
import json, os, sys
import numpy as np

D = os.path.dirname(os.path.abspath(__file__))
raw = json.load(open(os.path.join(D, "raw_main.json")))
dr = raw["draws"]
if os.path.exists(os.path.join(D, "raw_aggr.json")):   # merge the c_aggr sensitivity arm (same seeds / draws)
    ag = {d["k"]: d for d in json.load(open(os.path.join(D, "raw_aggr.json")))["draws"]}
    for d in dr:
        d["main"].update(ag[d["k"]]["main"]); d["sweep"].update(ag[d["k"]]["sweep"])


def ci(xs, scale=100, nd=1):
    x = np.array([v for v in xs if v == v], float) * scale
    return [round(float(np.median(x)), nd), round(float(np.percentile(x, 2.5)), nd), round(float(np.percentile(x, 97.5)), nd)]


out = {"n_draws": len(dr), "calibration_status_quo": {
    "median_days": ci([d["calib"]["median"] for d in dr], 1, 0), "iqr_days": ci([d["calib"]["iqr"] for d in dr], 1, 0),
    "first_no_show_pct": ci([d["calib"]["no_show"] for d in dr]), "completion_pct": ci([d["calib"]["completion"] for d in dr])},
    "main": {}, "paired_c_minus_b_pts": {}, "sweep": {}, "kpi_reach": {}}
for p in dr[0]["main"]:
    M = [d["main"][p] for d in dr]
    row = {}
    for k in M[0]:
        if k == "n":
            continue
        sc = 1 if ("median" in k or "_med" in k or "per_1000" in k) else 100
        row[k] = ci([m[k] for m in M], sc, 1)
    out["main"][p] = row
for k in ("d1_31", "d2_31", "d3_31", "completion", "g_women_hard_pref_d2", "g_al_dhafra_d2"):
    out["paired_c_minus_b_pts"][k] = ci([d["main"]["c"][k] - d["main"]["b"][k] for d in dr])
    out["paired_c_minus_b_pts"][k + "_P(c>b)"] = round(float(np.mean([d["main"]["c"][k] > d["main"]["b"][k] for d in dr])), 3)
keys = sorted(dr[0]["sweep"])
for key in keys:
    S = [d["sweep"][key] for d in dr]
    out["sweep"][key] = {k: ci([s[k] for s in S], 1 if ("median" in k or "per_1000" in k) else 100)
                         for k in ("d1_31", "d2_31", "d3_31", "sched_31", "median_days", "utilisation",
                                   "collisions_per_1000", "completion", "g_women_hard_pref_d3", "g_women_hard_pref_d2")}
# capacity needed per draw to reach >90% (d3 = among colonoscopies done; sched = first appointment within 31 d)
caps = sorted({float(k.split("@")[1]) for k in keys if "|" not in k and not k.startswith("eps")})
out["paired_caggr_minus_b_pts"] = {k: ci([d["main"]["c_aggr"][k] - d["main"]["b"][k] for d in dr]) for k in ("d1_31", "d2_31", "d3_31")}
for pol in ("b", "b_pref", "c", "c_aggr"):
    for metric in ("d3_31", "sched_31", "g_women_hard_pref_d3"):
        need = []
        for d in dr:
            hit = [c for c in caps if d["sweep"].get(f"{pol}@{c}", {}).get(metric, 0) > 0.90]
            need.append(min(hit) if hit else np.inf)
        need = np.array(need)
        fin = need[np.isfinite(need)]
        out["kpi_reach"][f"{pol}:{metric}"] = {
            "share_of_draws_reaching_by_c=" + str(max(caps)): round(float(np.isfinite(need).mean()), 3),
            "median_c_needed": float(np.median(need)) if np.isfinite(np.median(need)) else "not reached",
            "p90_c_needed": float(np.percentile(need, 90)) if np.isfinite(np.percentile(need, 90)) else "not reached",
            "P(reach)_by_c": {str(c): round(float(np.mean(need <= c)), 3) for c in caps}}
json.dump(out, open(os.path.join(D, "summary.json"), "w"), indent=1)
print(json.dumps(out, indent=1))
