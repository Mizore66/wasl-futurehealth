"""Stage 6: booking as SIMULATED decision support for a materialised Wasl run (not live scheduling).

usage: python booking/suggest.py <run_dir> [--slots-per-fit 1.2] [--seed 2026] [--misallocated] [--optimiser myopic|greedy|lookahead]

Reads the run's final decisions (out_log.ndjson) + referral demographics (fhir_log.ndjson), takes every CRC loop that is
still open at the end of the run and would need a colonoscopy slot (and every cervical loop that needs a colposcopy,
including the DoH 10.6.4 repeat 6-12 weeks after an unsatisfactory exam), and feeds them to the real booking engine
(booking/engine.py: constraint-safe batch assignment, overbooking, nurse calls) on a SYNTHETIC endoscopy calendar.
Writes <run_dir>/booking.json (schema "wasl-booking/2"; every wasl-booking/1 field is kept): calendar summary, capacity
condition, per-case suggestion, whether network pooling is relevant (only when capacity is geographically misallocated),
and per case the queue-lever suggestions (booking/booking/levers.py): `pathway` (direct-to-procedure vs GI clinic, from
SYNTHETIC App 9 fields) and `prep_risk` (Dik 2015 published score, cut-off >=2, suggested prep intervention; any regimen
change requires a clinician order).
The optimiser does NOT beat greedy in our tests; we make no optimisation claim (see booking/RESULTS.md).
`--optimiser lookahead` (off by default) swaps in the multi-period rolling-horizon optimiser (booking/booking/lookahead.py,
solved by `wsi7 mcf`); the stress-test flags (--arrivals bursty, --female-share, --colpo-female-share,
--crc-urgent-share) are for booking/sim/lookahead_eval.py. The default remains myopic; deterministic probability
rounding and integer-cost assignment apply, with reviewed golden changes to probabilities and two equal-cost slots.
"""
from __future__ import annotations
import json, os, sys, argparse, collections, hashlib
import numpy as np
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from booking.model import Prefs, Event, NoShowModel
from booking.calendar import Calendar
from booking.engine import BookingEngine
from booking.lookahead import LookaheadEngine
from booking import levers

REG = ["AD", "AA", "DH"]
UNITS = {"AD": 3, "AA": 2, "DH": 1}
LIST_CAP = 4
# provider -> region (cohort providers are all in Abu Dhabi city / Al Ain; assumed, synthetic)
PROV_REGION = {"ahs-mushrif": "AD", "medlab": "AA", "skmc-endo": "AD", "noor-endo": "AA", "corniche-wh": "AD"}
NEEDS_SLOT = {"open", "no_show", "inadequate", "referral_rejected"}  # still need (another) colonoscopy


def load(run):
    last = {}
    for l in open(os.path.join(run, "out_log.ndjson")):
        d = json.loads(l)["data"].get("dec")
        if d:
            last[d["loop"]] = d
    sr = {}
    for l in open(os.path.join(run, "fhir_log.ndjson")):
        m = json.loads(l)
        if not m["subj"].endswith(".res"):
            continue
        r = m["data"] if isinstance(m["data"], dict) else json.loads(m["data"])
        if r.get("resourceType") != "ServiceRequest":
            continue
        prov = m["subj"].split(".")[1]
        pt = (r.get("contained") or [{}])[0]
        lang = ""
        for c in pt.get("communication") or []:
            lang = ((c.get("language") or {}).get("coding") or [{}])[0].get("code", "")
        sr[f"{prov}/ServiceRequest/{r['id']}"] = {"gender": pt.get("gender"), "lang": lang}
    return last, sr


def stable_u(key, salt):
    return int(hashlib.sha256(f"{salt}|{key}".encode()).hexdigest()[:12], 16) / 16 ** 12


def calendar(n_fit, slots_per_fit, rng, misallocated, f_share=0.25, start=0, days=95):
    share = np.array([0.40, 0.50, 0.10]) if misallocated else np.array([0.55, 0.35, 0.10])
    dd, un, rg, cp, fe, ar, af, ra = [], [], [], [], [], [], [], []
    for r, name in enumerate(REG):
        lists_per_day = slots_per_fit * n_fit * share[r] / (days * 6 / 7) / LIST_CAP  # 6 working days a week
        acc, k = rng.random(), 0
        for d in range(start, start + days):  # requests arrive over 60 d + 31 d deadline window (+ prep lead)
            if (d + 4) % 7 == 6:
                continue
            acc += lists_per_day
            while acc >= 1:
                acc -= 1
                dd.append(d); un.append(f"{name}-{k % UNITS[name] + 1}"); rg.append(name); cp.append(LIST_CAP)
                fe.append(rng.random() < f_share); ar.append(rng.random() < 0.5); af.append((d + 4) % 7 == 5); ra.append(False); k += 1
    return Calendar(dd, un, rg, cp, fe, ar, af, ra), share


FEMALE_PROTECT = 4.0   # demand-sized female-list protection, released 7 days out (booking/RESULTS.md, female-list gap)


BURST_STARTS, BURST_LEN, BURST_SHARE = (5, 20, 35, 50), 4, 0.6   # stress test [A]: 60% of requests in four 4-day surges


def arrival_day(k, a):
    """Request day over 60 days: uniform (default) or bursty (stress test only)."""
    if getattr(a, "arrivals", "uniform") == "bursty" and stable_u(k, "burst") < BURST_SHARE:
        return BURST_STARTS[int(stable_u(k, "bwhich") * len(BURST_STARTS))] + int(stable_u(k, "bday") * BURST_LEN)
    return int(stable_u(k, "arr") * 60)


def make_engine(a, cal, prep_lead):
    """Default: the myopic per-day assignment engine. greedy = its ablation; lookahead = multi-period optimiser."""
    kw = dict(eps=0.05, prep_lead=prep_lead, protect=a.female_protect or None)
    if getattr(a, "optimiser", "myopic") == "lookahead":
        return LookaheadEngine(cal, NoShowModel(), lookahead_days=a.lookahead_days, gamma=a.gamma, forecast=a.forecast, **kw)
    return BookingEngine(cal, NoShowModel(), solver="greedy" if getattr(a, "optimiser", "myopic") == "greedy" else "assignment", **kw)


def wd(xs):
    return round(sum(1 for c in xs if c["status"] == "suggested" and c["booking"]["within_deadline"]) / len(xs), 3) if xs else None


def _canonical(value):
    """Compact key-sorted JSON with the same integral-number normalization as Go's booking hasher."""
    def numbers(v):
        if isinstance(v, float) and np.isfinite(v) and v.is_integer():
            return int(v)
        if isinstance(v, list):
            return [numbers(x) for x in v]
        if isinstance(v, dict):
            return {k: numbers(x) for k, x in v.items()}
        return v
    return json.dumps(numbers(value), sort_keys=True, separators=(",", ":"), ensure_ascii=False).replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026").encode()


def _sha(value):
    return hashlib.sha256(_canonical(value)).hexdigest()


def _eligible_slots(eng, cal, procedure, prefs, requested_day, deadline, earliest, prep_lead):
    """Deterministic synthetic seats from the existing engine's hard-feasibility and no-show rules.

    This is a catalogue, not a second assignment pass: sessions stay in calendar order and every declared
    non-overbooked seat is named. Per-case fields (deadline, lead time and no-show risk) intentionally differ.
    """
    day = requested_day
    lo = max(day + prep_lead, earliest)
    a, b = cal.window(lo, day + eng.H)
    sessions = np.arange(a, b)
    if len(sessions) == 0:
        return []
    m = cal.day[sessions] >= lo
    m &= (cal.region[sessions] == prefs.region) | prefs.travel_ok
    if prefs.female_endoscopist == "hard":
        m &= cal.female[sessions]
    if prefs.avoid_ramadan:
        m &= ~cal.ramadan[sessions]
    sessions = sessions[m]
    # Match BookingEngine.build's bounded candidate window, counting sessions rather than occupied seats because
    # coordinator reservations occupy their own append-only ledger, not the engine's retrospective assignment state.
    sessions = sessions[: eng.K]
    out = []
    for s0 in sessions:
        s = int(s0)
        female, arabic, after, ramadan = (bool(x) for x in (cal.female[s], cal.arabic[s], cal.after[s], cal.ramadan[s]))
        region = str(cal.region[s])
        pred = float(eng._pred(prefs, int(cal.day[s]) - day, female, arabic, after, ramadan, 0, region))
        nurse = pred > eng.nurse_thr
        if nurse:
            pred = float(eng._pred(prefs, int(cal.day[s]) - day, female, arabic, after, ramadan, 1, region))
        for seat in range(1, int(cal.cap[s]) + 1):
            out.append({
                "slot_id": f"sim:{procedure}:{s}:{seat}", "session_id": s, "unit": str(cal.unit[s]),
                "day": int(cal.day[s]), "region": region, "deadline": int(deadline),
                "within_deadline": bool(cal.day[s] <= deadline), "endoscopist_female": female,
                "arabic": arabic, "after_hours": after, "prep_send_day": int(cal.day[s]) - prep_lead,
                "predicted_no_show": pred, "nurse_call": bool(nurse), "cross_region": region != prefs.region,
            })
    return out


def _version_catalogue(res):
    """Version the generated document without a self-referential calendar hash.

    suggestion_version covers the complete case suggestion (including constraints and eligible slot IDs), excluding
    only that field itself. calendar_version then covers the complete booking document, including suggestion versions,
    excluding only calendar_version itself. The Go loader recomputes both before exposing a reservable slot.
    """
    for case in res["cases"].values():
        unsigned = {k: v for k, v in case.items() if k != "suggestion_version"}
        case["suggestion_version"] = _sha(unsigned)
    unsigned = {k: v for k, v in res.items() if k != "calendar_version"}
    res["calendar_version"] = _sha(unsigned)


def _assign_suggested_seats(cases):
    """Give ordinary suggestions unique deterministic seats in case-ID order; never reserve an overbooked one."""
    used = collections.defaultdict(set)
    for case_id in sorted(cases):
        case = cases[case_id]
        candidates = case.get("eligible_slots") or []
        if case["status"] != "suggested":
            case["reservable"] = False
            continue
        booking = case["booking"]
        if booking["overbooked"]:
            case["reservable"] = False
            case["reservation_explanation"] = "overbooked_suggestion_not_reservable"
            continue
        same_session = [s for s in candidates if s["session_id"] == booking["session_id"]]
        for slot in same_session:
            seat = int(slot["slot_id"].rsplit(":", 1)[1])
            if seat not in used[(case["procedure"], booking["session_id"])]:
                used[(case["procedure"], booking["session_id"])].add(seat)
                case["suggested_slot_id"] = slot["slot_id"]
                case["reservable"] = True
                break
        else:
            case["reservable"] = False
            case["reservation_explanation"] = "suggested_session_has_no_regular_seat"


REPEAT_WINDOW = (42, 84)   # DoH cervical standard 10.6.4: unsatisfactory colposcopy -> repeat in 6-12 weeks
COLPO_FEMALE_SHARE = 0.5   # assumed share of colposcopy clinics run by a female clinician [A]


def cervical(last, sr, a, rng):
    """Cervical loops that still need a colposcopy: a repeat 6-12 weeks after an unsatisfactory exam (DoH 10.6.4), or a
    first colposcopy inside the loop's DoH window (App 5). Same engine, SYNTHETIC colposcopy-clinic calendar."""
    need = [(k, d) for k, d in sorted(last.items()) if d.get("prog") == "cervical" and d["state"] in NEEDS_SLOT]
    # same slot density per day as the CRC calendar (slots_per_fit over its 95-day window), over 160 days so repeats fit
    cal, _ = calendar(len(need), a.slots_per_fit * 160 / 95, rng, a.misallocated, f_share=a.colpo_female_share, days=160)
    eng = make_engine(a, cal, 1)
    meta, by_day = {}, collections.defaultdict(list)
    for k, d in need:
        demo = sr.get(k, {})
        u = stable_u(k, "fem")
        prefs = Prefs(female_endoscopist="hard" if u < 0.25 else ("soft" if u < 0.6 else "none"),
                      arabic_only=demo.get("lang") == "ar" and stable_u(k, "ar") < 0.3, region=PROV_REGION.get(d["prov"], "AD"),
                      travel_ok=stable_u(k, "travel") < 0.3, prior_no_show=d["state"] == "no_show")
        day = arrival_day(k, a)
        repeat = d["state"] == "inadequate"
        lo, hi = (day + REPEAT_WINDOW[0], day + REPEAT_WINDOW[1]) if repeat else (day, day + int(d.get("deadline") or 56))
        meta[k] = (d, prefs, day, repeat, lo, hi)
        by_day[day].append(Event("result_accepted", k, day, prefs, hi, lo))   # booked when the result arrives
    if getattr(a, "forecast", "") == "oracle" and isinstance(eng, LookaheadEngine):
        eng.set_oracle([ev for evs in by_day.values() for ev in evs])
    out = {}
    try:
        for day in range(0, 200):
            for ev in by_day.get(day, []):
                eng.submit(ev)
            for b in eng.step(day):
                out[b.loop_id] = b
            if day > max(by_day, default=0) and not eng.pending:
                break
    finally:
        getattr(eng, "close", lambda: None)()
    cases = {}
    for k, (d, prefs, day, repeat, lo, hi) in meta.items():
        base = {"procedure": "colposcopy", "repeat_after_unsatisfactory": repeat, "requested_day": day, "window_days": [lo, hi],
                "rule": "doh:cervical 10.6.4 repeat 6-12 wk" if repeat else "doh:cervical App 5 colposcopy window",
                "region": prefs.region, "constraints": prefs.__dict__, "state_at_run_end": d["state"]}
        b = out.get(k)
        candidates = _eligible_slots(eng, cal, "colposcopy", prefs, day, hi, max(day + 1, lo), 1)
        if b is None:
            cases[k] = {"status": "no_slot_within_horizon", "reason": "capacity", "eligible_slots": candidates, **base}
        else:
            cases[k] = {"status": "suggested", "booking": b.to_json(), "within_window": lo <= b.day <= hi,
                        "cross_region": b.unit.split("-")[0] != prefs.region, "wait_days": b.day - day,
                        "eligible_slots": candidates, **base}
    sugg = [c for c in cases.values() if c["status"] == "suggested"]
    within = sum(1 for c in sugg if c["within_window"])
    return {"cases": cases, "summary": {
        "cases": len(cases), "repeat_after_unsatisfactory": sum(1 for c in cases.values() if c["repeat_after_unsatisfactory"]),
        "suggested": len(sugg), "no_slot": len(cases) - len(sugg), "within_window": within,
        "within_window_share": round(within / len(cases), 3) if cases else None,
        "repeat_window_days": list(REPEAT_WINDOW), "colposcopy_female_clinic_share_assumed": a.colpo_female_share,
        "note": "Synthetic colposcopy-clinic calendar at the same slots-per-case assumption; decision support only."}}


def main(argv=None, preloaded=None, write=True, quiet=False):
    ap = argparse.ArgumentParser()
    ap.add_argument("run")
    ap.add_argument("--slots-per-fit", type=float, default=1.2, help="assumed endoscopy capacity (slots per open FIT+ case)")
    ap.add_argument("--seed", type=int, default=2026)
    ap.add_argument("--misallocated", action="store_true", help="put capacity in the wrong regions (pooling scenario)")
    ap.add_argument("--female-protect", type=float, default=FEMALE_PROTECT, help="female-list protection factor (0 = off)")
    # multi-period lookahead (booking/booking/lookahead.py) and stress tests; all off by default
    ap.add_argument("--optimiser", choices=["myopic", "greedy", "lookahead"], default="myopic",
                    help="myopic = per-day exact assignment (default); greedy = its ablation; lookahead = rolling horizon")
    ap.add_argument("--lookahead-days", type=int, default=28, help="lookahead: forecast horizon in days")
    ap.add_argument("--gamma", type=float, default=0.8, help="lookahead: cost weight of forecast (phantom) demand")
    ap.add_argument("--forecast", choices=["trailing", "oracle"], default="trailing",
                    help="lookahead: trailing arrival rate (deployable) or true future counts (upper bound only)")
    ap.add_argument("--arrivals", choices=["uniform", "bursty"], default="uniform", help="stress test: request-day pattern")
    ap.add_argument("--female-share", type=float, default=0.25, help="share of endoscopy lists with a female endoscopist [A]")
    ap.add_argument("--colpo-female-share", type=float, default=COLPO_FEMALE_SHARE, help="share of colposcopy clinics run by a female clinician [A]")
    ap.add_argument("--crc-urgent-share", type=float, default=0.0,
                    help="stress test: share of CRC cases given a hypothetical 14-day target instead of 31 days [A]")
    ap.add_argument("--out", default=None, help="output path (default <run>/booking.json)")
    a = ap.parse_args(argv)
    last, sr = preloaded if preloaded is not None else load(a.run)
    rng = np.random.default_rng(a.seed)
    open_cases = [(k, d) for k, d in sorted(last.items()) if d.get("prog") == "crc" and d["state"] in NEEDS_SLOT]
    as_of = 0  # suggestions are made from the start of an empty synthetic calendar window
    cal, share = calendar(len(open_cases), a.slots_per_fit, rng, a.misallocated, f_share=a.female_share)
    eng = make_engine(a, cal, 5)  # female-list protection on by default
    meta = {}
    for k, d in open_cases:
        demo = sr.get(k, {})
        region = PROV_REGION.get(d["prov"], "AD")
        female = demo.get("gender") == "female"
        fem_pref = "hard" if female and stable_u(k, "fem") < 0.25 else ("soft" if female and stable_u(k, "fem") < 0.6 else "none")
        prefs = Prefs(female_endoscopist=fem_pref, arabic_only=demo.get("lang") == "ar" and stable_u(k, "ar") < 0.3,
                      region=region, travel_ok=stable_u(k, "travel") < (0.6 if a.misallocated else 0.3),
                      prior_no_show=d["state"] == "no_show")
        day = as_of + arrival_day(k, a)  # spread requests over 60 days
        meta[k] = (d, prefs, day)
    urgent = {k for k in meta if a.crc_urgent_share > 0 and stable_u(k, "urgent") < a.crc_urgent_share}
    by_day = collections.defaultdict(list)
    for k, (_, prefs, day) in meta.items():
        by_day[day].append(Event("result_accepted", k, day, prefs, day + (14 if k in urgent else 31)))
    if a.forecast == "oracle" and isinstance(eng, LookaheadEngine):
        eng.set_oracle([ev for evs in by_day.values() for ev in evs])
    out = {}
    try:
        for day in sorted(by_day):
            for ev in by_day[day]:
                eng.submit(ev)
            for b in eng.step(day):
                out[b.loop_id] = b
        for day in range(max(by_day) + 1 if by_day else 1, (max(by_day) if by_day else 0) + 90):  # drain the queue
            if not eng.pending:
                break
            for b in eng.step(day):
                out[b.loop_id] = b
    finally:
        getattr(eng, "close", lambda: None)()
    cases, unbooked = {}, 0
    reg_demand, reg_booked_out = collections.Counter(), collections.Counter()
    for k, (d, prefs, day) in meta.items():
        b = out.get(k)
        deadline = day + (14 if k in urgent else 31)
        candidates = _eligible_slots(eng, cal, "colonoscopy", prefs, day, deadline, day + 5, 5)
        reg_demand[prefs.region] += 1
        lv = levers.assess(k)
        if b is None:
            unbooked += 1
            cases[k] = {"status": "no_slot_within_horizon", "procedure": "colonoscopy", "requested_day": day, "region": prefs.region,
                        "constraints": prefs.__dict__, "reason": "capacity", "state_at_run_end": d["state"],
                        "eligible_slots": candidates, **lv}
            if a.crc_urgent_share > 0:
                cases[k]["target_days"] = 14 if k in urgent else 31
            continue
        ru = b.unit.split("-")[0]
        if ru != prefs.region:
            reg_booked_out[prefs.region] += 1
        cases[k] = {"status": "suggested", "procedure": "colonoscopy", "requested_day": day, "region": prefs.region, "constraints": prefs.__dict__,
                    "state_at_run_end": d["state"], "booking": b.to_json(), "cross_region": ru != prefs.region,
                    "wait_days": b.day - day, "eligible_slots": candidates, **lv}
        if a.crc_urgent_share > 0:
            cases[k]["target_days"] = 14 if k in urgent else 31
    cerv = cervical(last, sr, a, rng)
    sugg = [c for c in cases.values() if c["status"] == "suggested"]
    within = sum(1 for c in sugg if c["booking"]["within_deadline"])
    n = len(cases)
    slots = int(cal.cap.sum())
    cap_by_region = {r: int(cal.cap[cal.region == r].sum()) for r in REG}
    dem = {r: reg_demand[r] for r in REG}
    ratio = {r: round(cap_by_region[r] / dem[r], 2) if dem[r] else 0.0 for r in REG}  # 0.0 = no demand in region
    total_ratio = round(slots / n, 2) if n else None
    misalloc = any(dem[r] and ratio[r] < 1.0 for r in REG) and (total_ratio or 0) >= 1.0
    condition = ("short_overall" if (total_ratio or 0) < 1.0 else "misallocated" if misalloc else "adequate")
    res = {
        "schema": "wasl-booking/2", "data_mode": "simulated", "decision_support_only": True,
        "note": "Synthetic endoscopy calendar and constraint/no-show assumptions; not live scheduling. Optimiser ~= greedy (no optimisation claim).",
        "inputs": {"run": os.path.basename(os.path.abspath(a.run)), "seed": a.seed, "slots_per_open_case": a.slots_per_fit,
                   "capacity_share_by_region": dict(zip(REG, map(float, share))), "misallocated_scenario": a.misallocated},
        "capacity": {"condition": condition, "slots": slots, "open_cases_needing_slot": n, "slots_per_case": total_ratio,
                     "by_region": {r: {"slots": cap_by_region[r], "cases": dem[r], "slots_per_case": ratio[r]} for r in REG},
                     "shortfall_slots": max(0, n - slots),
                     "pooling_relevant": condition == "misallocated",
                     "pooling_note": {"misallocated": "slots exist in another region: network pooling (cross-site booking) recovers them",
                                      "short_overall": "network-wide shortfall: pooling only spreads it; needs capacity (DoH) or prep-failure prevention",
                                      "adequate": "no capacity constraint at this demand"}[condition]},
        "summary": {"suggested": len(sugg), "no_slot": unbooked, "within_31d": within,
                    "within_31d_share": round(within / n, 3) if n else None,
                    "cross_region": sum(1 for c in sugg if c["cross_region"]),
                    "overbooked": sum(1 for c in sugg if c["booking"]["overbooked"]),
                    "nurse_call": sum(1 for c in sugg if c["booking"]["nurse_call"]),
                    "median_wait_days": float(np.median([c["wait_days"] for c in sugg])) if sugg else None,
                    "female_list": {"protect": a.female_protect, "release_days": eng.reserve_days,
                                    "within_31d_share_women_requiring_female_list": wd([c for c in cases.values() if c["constraints"]["female_endoscopist"] == "hard"]),
                                    "within_31d_share_everyone_else": wd([c for c in cases.values() if c["constraints"]["female_endoscopist"] != "hard"])}},
        "levers": {"schema": levers.SCHEMA, "data_mode": "synthetic App 9 fields; Dik 2015 published score weights",
                   "pathway": dict(collections.Counter(c["pathway"]["route"] for c in cases.values())),
                   "prep_risk_band": dict(collections.Counter(c["prep_risk"]["band"] for c in cases.values())),
                   "requires_clinician_order": True,
                   "simulated_effect": "booking/RESULTS.md, Queue levers (simulated)"},
        "cases": {**cases, **cerv["cases"]},
        "cervical": cerv["summary"],
    }
    given = {"optimiser": (a.optimiser, "myopic"), "arrivals": (a.arrivals, "uniform"), "female_list_share": (a.female_share, 0.25),
             "colposcopy_female_share": (a.colpo_female_share, COLPO_FEMALE_SHARE), "crc_urgent_share": (a.crc_urgent_share, 0.0)}
    opt = {k: v for k, (v, dflt) in given.items() if v != dflt}
    if a.optimiser == "lookahead":
        opt.update(lookahead_days=a.lookahead_days, gamma=a.gamma, forecast=a.forecast,
                   method="rolling-horizon min-cost flow over real + forecast demand (booking/booking/lookahead.py, wsi7 mcf)")
    if opt:   # only when a non-default optimiser or stress test is on, so the default booking.json is unchanged
        res["inputs"]["optimiser"] = opt
    _assign_suggested_seats(res["cases"])
    _version_catalogue(res)
    if write:
        json.dump(res, open(a.out or os.path.join(a.run, "booking.json"), "w"), indent=1)
    if quiet:
        return res
    s = res["summary"]; c = res["capacity"]
    print(f"booking (simulated): {n} open CRC cases; capacity {c['condition']} ({c['slots']} slots, {c['slots_per_case']}/case); "
          f"suggested {s['suggested']}, within 31 d {s['within_31d']} ({s['within_31d_share']}), cross-region {s['cross_region']}, "
          f"no slot {s['no_slot']}; pooling relevant: {c['pooling_relevant']}; pathway {res['levers']['pathway']}, "
          f"prep risk {res['levers']['prep_risk_band']}")
    cs = res["cervical"]
    print(f"booking (simulated): {cs['cases']} cervical cases needing colposcopy ({cs['repeat_after_unsatisfactory']} repeats after "
          f"an unsatisfactory exam); suggested {cs['suggested']}, inside the DoH window {cs['within_window']} ({cs['within_window_share']})")
    return res


if __name__ == "__main__":
    main()
