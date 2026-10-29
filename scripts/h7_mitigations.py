"""Aggregate the H7 mitigation sweeps (one `wsi7 sweep -sweepmit <set>` output per set) into
pipeline/results/h7-mitigations/h7_mitigations_summary.json.
Usage: python3 scripts/h7_mitigations.py <out.json> <sweep_double.json> <sweep_blind.json> ... [label=<sweep.json> ...]
A label=path argument files that sweep's rows under the label (parameter-sensitivity runs) instead of its -sweepmit set.
Reviewer error rates and correlations are assumed stress-test values, not measured clinician rates."""
import json, sys
from collections import defaultdict

out, ins = sys.argv[1], sys.argv[2:]
rows = []
for a in ins:
    label, _, path = a.rpartition("=") if "=" in a else ("", "", a)
    for r in json.load(open(path)):
        if label and "mitigation" in r:
            r["mitigation"] = label
        rows.append(r)
ref = next(r for r in rows if r.get("double") == "no_review")
cells = defaultdict(list)
for r in rows:
    if "mitigation" in r:
        cells[(r["mitigation"], r["err"], r["rho"])].append(r)

AKEYS = ["audited", "wrong_before", "disagreed", "fixed", "introduced", "missed"]
summary = []
for (mit, err, rho), rs in sorted(cells.items(), key=lambda kv: (kv[0][1], kv[0][2], kv[0][0])):
    rs.sort(key=lambda r: r["seed"])
    n = len(rs)
    audit = {k: sum(r["audit"][k] for r in rs) for k in AKEYS}
    summary.append({
        "mitigation": mit, "err": err, "rho": rho, "effective_rho": rs[0]["effective_rho"], "seeds": [r["seed"] for r in rs],
        "false_closures": sum(r["false_closures"] for r in rs), "false_closures_by_seed": [r["false_closures"] for r in rs],
        "closed": sum(r["closed_pred"] for r in rs), "recall_mean": round(sum(r["recall"] for r in rs) / n, 3),
        "reads": sum(r["reads"] for r in rs), "reviewer_h_per_1k": round(sum(r["reviewer_hours_per_1k_loops"] for r in rs) / n, 1),
        "audit": audit,
    })

# determinism check: at rho=0 a blinded or rotated read has nothing to remove, so those cells must equal the plain double read
by = {(s["mitigation"], s["err"], s["rho"]): s for s in summary}
same0 = all(by[(m, e, 0)]["false_closures_by_seed"] == by[("double", e, 0)]["false_closures_by_seed"]
            and by[(m, e, 0)]["closed"] == by[("double", e, 0)]["closed"] and by[(m, e, 0)]["reads"] == by[("double", e, 0)]["reads"]
            for m in ("blind", "rotate") for (mm, e, r) in by if mm == "double" and r == 0 and (m, e, 0) in by)

json.dump({
    "cohort": "ltt5 cal-s101 (synthetic)", "policy": "v7strict", "double": "merge+close", "reviewer_seeds": sorted({r["seed"] for r in rows if "seed" in r}),
    "note": "err and rho are assumed stress-test values, not measured clinician rates; mitigation parameters in SUMMARY.md",
    "check_rho0_blind_rotate_equal_double": same0,
    "no_review_reference": {"false_closures": ref["false_closures"], "closed": ref["closed_pred"], "recall": ref["recall"]},
    "rows": summary,
}, open(out, "w"), indent=1)
print("rows", len(summary), "rho0 check", same0)
