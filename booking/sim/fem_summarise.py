"""Summarise raw_fem.json -> fem_summary.json: the female-list access gap with and without reservation.

Gap = (<=31-day KPI for everyone who does not require a female endoscopist) - (KPI for women who do), paired per draw.
Medians [2.5, 97.5] over 150 parameter draws (0-149); the reservation setting was chosen on draws 150-189.
Run: python sim.py 150 fem '<cfg in booking/RESULTS.md>' && python fem_summarise.py
"""
import json, os
import numpy as np

D = os.path.dirname(os.path.abspath(__file__))
dr = json.load(open(os.path.join(D, "raw_fem.json")))["draws"]


def ci(x):
    x = 100 * np.array(x, float)
    return [round(float(np.median(x)), 1), round(float(np.percentile(x, 2.5)), 1), round(float(np.percentile(x, 97.5)), 1)]


def arms(get):
    out = {}
    for name, rows in get.items():
        base = rows["c"]
        out[name] = {}
        for arm, r in rows.items():
            gap = [a["g_not_hard_pref_d3"] - a["g_women_hard_pref_d3"] for a in r]
            gap2 = [a["g_not_hard_pref_d2"] - a["g_women_hard_pref_d2"] for a in r]
            out[name][arm] = {
                "kpi_women_requiring_female_list": ci([a["g_women_hard_pref_d3"] for a in r]),
                "kpi_everyone_else": ci([a["g_not_hard_pref_d3"] for a in r]),
                "gap_kpi_pts": ci(gap), "P(gap<=5pts)": round(float(np.mean(np.array(gap) <= 0.05)), 3),
                "gap_referred_pts": ci(gap2),
                "all_fit_pos_31d": ci([a["d1_31"] for a in r]),
                "paired_all_fit_pos_31d_vs_c_pts": ci([a["d1_31"] - b["d1_31"] for a, b in zip(r, base)]),
                "paired_everyone_else_kpi_vs_c_pts": ci([a["g_not_hard_pref_d3"] - b["g_not_hard_pref_d3"] for a, b in zip(r, base)]),
            }
    return out


get = {"f_share 0.15-0.30 (main)": {a: [d["main"][a] for d in dr] for a in dr[0]["main"]}}
for f in ("0.3", "0.4"):
    get[f"f_share {f}"] = {a: [d["sweep"][f"f_share={f}|{a}@1.5"] for d in dr] for a in ("c", "c_res7", "c_fem")}
out = {"n_draws": len(dr), "capacity_slots_per_fit_pos": 1.5, "arms": arms(get)}
json.dump(out, open(os.path.join(D, "fem_summary.json"), "w"), indent=1)
for name, rows in out["arms"].items():
    for arm, r in rows.items():
        print(f"{name:26s} {arm:7s} hard {r['kpi_women_requiring_female_list']} else {r['kpi_everyone_else']} gap {r['gap_kpi_pts']} P<=5 {r['P(gap<=5pts)']} all31 Δ {r['paired_all_fit_pos_31d_vs_c_pts']}")
