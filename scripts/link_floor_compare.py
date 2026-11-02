"""Aggregate analyzer for the CRC review-floor paired experiment (development lanes only select; never reads validation paths).

Selection rule, fixed before any result: among arms passing the gate prefer 0.05 over 0.01 (less reviewer exposure);
select none if neither passes. CRC closure recall is report7 programmes_live.crc.tp / truth_closed, NOT funnel attached_or_closed.
"unanswered" = a link review item (kinds contains "link", no "reason") with no reviewer decision recorded by item id or by pair.
Small-n caveat: t intervals with 3 (dev) or 5 (validation) seeds are wide and only indicative.
"""
import json
import math
import os
import statistics
import sys

MODES = {
    "dev-clean": {"split": "dev", "seeds": [101, 103, 107], "rerr": "0", "errungraded": "0"},
    "dev-stress": {"split": "dev", "seeds": [101, 103, 107], "rerr": "0.04", "errungraded": "0.25"},
    "validation-clean": {"split": "eval", "seeds": [211, 223, 227, 229, 233], "rerr": "0", "errungraded": "0"},
}
FLOORS = ("0.2", "0.05", "0.01")
MIN_GAIN_PP = 1.0
T_CRIT = {3: 4.303, 5: 2.776}  # two-sided 95%, df = n-1
PREFERENCE = (0.05, 0.01)  # conservative first
SAFETY = ("false_closures", "wrong_released_total", "replay_mismatch", "analysis_errors", "trace_error")
CONTAM = ("false_merge_pairs", "impure_clusters", "cross_person_edges")


def t_interval(values):
    n = len(values)
    if n not in T_CRIT:
        raise ValueError("t critical value defined only for n=3 or n=5")
    m = statistics.mean(values)
    return m, T_CRIT[n] * statistics.stdev(values) / math.sqrt(n)


def _req(d, *path):
    for k in path:
        d = d[k]  # KeyError on absence is intended: never substitute zero
    return d


def check_funnel_floor(funnel, floor):
    """The funnel config must record review_floor exactly equal to the arm floor; absence or mismatch raises."""
    cfg = funnel.get("config")
    if not isinstance(cfg, dict) or "review_floor" not in cfg:
        raise ValueError("funnel config.review_floor missing")
    v = cfg["review_floor"]
    if isinstance(v, bool) or not isinstance(v, (int, float)) or v != float(floor):
        raise ValueError("funnel review_floor %r != configured %s" % (v, floor))


def review_load(queue, decisions):
    """Link-review burden from review_queue.json items and reviewer decision records (answered by item or by pair)."""
    items = [i for i in queue if "link" in (i.get("kinds") or []) and not i.get("reason")]
    key = lambda i: tuple(sorted(i["pair"]))
    answered_items = {d["item"] for d in decisions}
    answered_pairs = {key(i) for i in items if i["item"] in answered_items}
    un = sum(1 for i in items if i["item"] not in answered_items and key(i) not in answered_pairs)
    return {"items": len(items), "distinct_pairs": len({key(i) for i in items}), "unanswered": un}


def lane_metrics(seed, report7, funnel, status, load):
    crc = _req(report7, "programmes_live", "crc")
    tp, truth = _req(crc, "tp"), _req(crc, "truth_closed")
    if _req(funnel, "truth_closed") != truth:
        raise ValueError("funnel/CRC truth_closed mismatch")
    lk = _req(report7, "linkage")
    org = _req(report7, "outreach_live", "by_origin")
    rv = _req(report7, "reviewer")
    allk = _req(rv, "by_kind", "all")
    return {
        "seed": seed, "tp": tp, "truth_closed": truth,
        "attach_rate_attached_or_closed": _req(funnel, "categories").get("attached_or_closed", 0),
        "false_closures": _req(crc, "false_closures"),
        "wrong_released_total": _req(report7, "outreach_live", "wrong_patient_released"),
        "wrong_released_by_origin": {k: _req(v, "wrong_released") for k, v in sorted(org.items())},
        "replay_mismatch": _req(report7, "live_vs_replay_final_mismatch"),
        "analysis_errors": _req(funnel, "analysis_errors"),
        "trace_error": 1 if status.get("link_funnel_error") else 0,
        "false_merge_pairs": _req(lk, "false_merge_pairs"),
        "impure_clusters": _req(lk, "impure_clusters"),
        "cross_person_edges": sum(_req(lk, "cross_person_edges_by_basis").values()),
        "reviewer": {"items": _req(allk, "items"), "hours": _req(allk, "sim_hours"),
                     "distinct_pairs": _req(load, "distinct_pairs"), "unanswered": _req(load, "unanswered"),
                     "link_items": _req(load, "items"),
                     "by_programme": _req(rv, "by_programme"), "by_origin": _req(rv, "by_origin")},
    }


def aggregate_arm(floor, lanes):
    a = {"floor": floor, "per_seed": {str(l["seed"]): l for l in lanes}}
    a["tp"] = sum(l["tp"] for l in lanes)
    a["truth_closed"] = sum(l["truth_closed"] for l in lanes)
    a["recall"] = a["tp"] / a["truth_closed"]
    for k in ("false_closures", "wrong_released_total", "replay_mismatch", "analysis_errors", "trace_error") + CONTAM:
        a[k] = sum(l[k] for l in lanes)
    org = {}
    for l in lanes:
        for k, v in l["wrong_released_by_origin"].items():
            org[k] = org.get(k, 0) + v
    a["wrong_released_by_origin"] = org
    a["reviewer"] = {k: sum(l["reviewer"][k] for l in lanes) for k in ("items", "hours", "distinct_pairs", "unanswered", "link_items")}
    return a


def gate(base, cand, kind="selection"):
    """kind: selection (>=1.0pp), stress (no safety/contamination regression, any gain), validation (strict improvement)."""
    reasons = []
    if base["truth_closed"] != cand["truth_closed"]:
        reasons.append("truth_closed differs between arms")
    gain = 100.0 * (cand["tp"] / cand["truth_closed"] - base["tp"] / base["truth_closed"])
    if kind == "selection" and gain < MIN_GAIN_PP - 1e-9:
        reasons.append("gain %.4fpp below %.1fpp" % (gain, MIN_GAIN_PP))
    if kind == "validation" and not gain > 0:
        reasons.append("no recall improvement")
    if kind == "stress" and gain < 0:
        reasons.append("recall regressed")
    for k in SAFETY:
        if cand[k] != 0:
            reasons.append(k + " nonzero")
    if any(v != 0 for v in cand["wrong_released_by_origin"].values()):
        reasons.append("wrong_released_by_origin nonzero")
    for k in SAFETY:
        if base[k] != 0:
            reasons.append("baseline " + k + " nonzero")
    for k in CONTAM:
        if cand[k] > base[k]:
            reasons.append(k + " increased")
    return {"pass": not reasons, "gain_pp": round(gain, 4), "reasons": reasons}


def select_arm(base, cands):
    arms = {}
    for c in cands:
        arms[str(c["floor"])] = gate(base, c)
    sel = None
    for f in PREFERENCE:
        if str(f) in arms and arms[str(f)]["pass"]:
            sel = f
            break
    return {"selected_floor": sel, "arms": arms, "baseline_recall": base["recall"] if "recall" in base else base["tp"] / base["truth_closed"]}


def _load_lane(path, seed, floor=None):
    def j(p):
        with open(os.path.join(path, p)) as h:
            return json.load(h)
    r = j("run/main/report7.json")
    q = j("run/main/review_queue.json")
    f = j("funnel.json")
    if floor is not None:
        check_funnel_floor(f, floor)
    dec = []
    dp = os.path.join(path, "run/main/reviewer_decisions.ndjson")
    if os.path.exists(dp):
        with open(dp) as h:
            dec = [json.loads(x) for x in h if x.strip()]
    return lane_metrics(seed, r, f, j("run/main/status-linker.json"), review_load(q, dec))


def main(argv):
    """link_floor_compare.py MODE OUT_JSON FLOOR=ROOT ...; baseline 0.2 required. Development modes only.
    dev-clean additionally writes selection.json (exactly that name) into the directory of OUT_JSON, which must be the
    experiment parent directory: the lane runner reads it to authorise dev-stress arms."""
    if len(argv) < 3 or argv[0] not in MODES:
        print("usage: link_floor_compare.py MODE OUT_JSON FLOOR=ROOT ...", file=sys.stderr)
        return 2
    mode, out = argv[0], argv[1]
    seeds = MODES[mode]["seeds"]
    arms = {}
    for a in argv[2:]:
        f, root = a.split("=", 1)
        if f not in FLOORS or f in arms:
            print("bad or repeated floor " + f, file=sys.stderr)
            return 2
        arms[f] = aggregate_arm(float(f), [_load_lane(os.path.join(root, "v2-s%d" % s), s) for s in seeds])
    if "0.2" not in arms:
        print("baseline 0.2 required", file=sys.stderr)
        return 2
    base = arms["0.2"]
    cands = [v for k, v in arms.items() if k != "0.2"]
    res = {"mode": mode, "seeds": seeds, "arms": arms,
           "note": "t intervals use n=%d seeds: small-n, indicative only" % len(seeds)}
    if mode == "dev-clean":
        res["selection"] = select_arm(base, cands)
        sel = dict(res["selection"], mode=mode, seeds=seeds)
        with open(os.path.join(os.path.dirname(os.path.abspath(out)), "selection.json"), "w") as h:
            json.dump(sel, h, indent=1, sort_keys=True)
    else:
        kind = "stress" if mode == "dev-stress" else "validation"
        res["gates"] = {str(c["floor"]): gate(base, c, kind) for c in cands}
    with open(out, "w") as h:
        json.dump(res, h, indent=1, sort_keys=True)
    print(json.dumps(res.get("selection", res.get("gates")), sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
