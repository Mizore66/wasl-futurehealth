"""Stage 6: booking as SIMULATED decision support for a materialised Wasl run (not live scheduling).

usage: python booking/suggest.py <run_dir> [--slots-per-fit 1.2] [--seed 2026] [--misallocated]

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
"""
from __future__ import annotations
import json, os, sys, argparse, collections, hashlib
import numpy as np
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from booking.model import Prefs, Event, NoShowModel
from booking.calendar import Calendar
from booking.engine import BookingEngine
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


def wd(xs):
    return round(sum(1 for c in xs if c["status"] == "suggested" and c["booking"]["within_deadline"]) / len(xs), 3) if xs else None


REPEAT_WINDOW = (42, 84)   # DoH cervical standard 10.6.4: unsatisfactory colposcopy -> repeat in 6-12 weeks
COLPO_FEMALE_SHARE = 0.5   # assumed share of colposcopy clinics run by a female clinician [A]


def cervical(last, sr, a, rng):
    """Cervical loops that still need a colposcopy: a repeat 6-12 weeks after an unsatisfactory exam (DoH 10.6.4), or a
    first colposcopy inside the loop's DoH window (App 5). Same engine, SYNTHETIC colposcopy-clinic calendar."""
    need = [(k, d) for k, d in sorted(last.items()) if d.get("prog") == "cervical" and d["state"] in NEEDS_SLOT]
    # same slot density per day as the CRC calendar (slots_per_fit over its 95-day window), over 160 days so repeats fit
    cal, _ = calendar(len(need), a.slots_per_fit * 160 / 95, rng, a.misallocated, f_share=COLPO_FEMALE_SHARE, days=160)
    eng = BookingEngine(cal, NoShowModel(), eps=0.05, prep_lead=1, protect=a.female_protect or None)
    meta, by_day = {}, collections.defaultdict(list)
    for k, d in need:
        demo = sr.get(k, {})
        u = stable_u(k, "fem")
        prefs = Prefs(female_endoscopist="hard" if u < 0.25 else ("soft" if u < 0.6 else "none"),
                      arabic_only=demo.get("lang") == "ar" and stable_u(k, "ar") < 0.3, region=PROV_REGION.get(d["prov"], "AD"),
                      travel_ok=stable_u(k, "travel") < 0.3, prior_no_show=d["state"] == "no_show")
        day = int(stable_u(k, "arr") * 60)
        repeat = d["state"] == "inadequate"
        lo, hi = (day + REPEAT_WINDOW[0], day + REPEAT_WINDOW[1]) if repeat else (day, day + int(d.get("deadline") or 56))
        meta[k] = (d, prefs, day, repeat, lo, hi)
        by_day[day].append(Event("result_accepted", k, day, prefs, hi, lo))   # booked when the result arrives
    out = {}
    for day in range(0, 200):
        for ev in by_day.get(day, []):
            eng.submit(ev)
        for b in eng.step(day):
            out[b.loop_id] = b
        if day > max(by_day, default=0) and not eng.pending:
            break
    cases = {}
    for k, (d, prefs, day, repeat, lo, hi) in meta.items():
        base = {"procedure": "colposcopy", "repeat_after_unsatisfactory": repeat, "requested_day": day, "window_days": [lo, hi],
                "rule": "doh:cervical 10.6.4 repeat 6-12 wk" if repeat else "doh:cervical App 5 colposcopy window",
                "region": prefs.region, "constraints": prefs.__dict__, "state_at_run_end": d["state"]}
        b = out.get(k)
        if b is None:
            cases[k] = {"status": "no_slot_within_horizon", "reason": "capacity", **base}
        else:
            cases[k] = {"status": "suggested", "booking": b.to_json(), "within_window": lo <= b.day <= hi,
                        "cross_region": b.unit.split("-")[0] != prefs.region, "wait_days": b.day - day, **base}
    sugg = [c for c in cases.values() if c["status"] == "suggested"]
    within = sum(1 for c in sugg if c["within_window"])
    return {"cases": cases, "summary": {
        "cases": len(cases), "repeat_after_unsatisfactory": sum(1 for c in cases.values() if c["repeat_after_unsatisfactory"]),
        "suggested": len(sugg), "no_slot": len(cases) - len(sugg), "within_window": within,
        "within_window_share": round(within / len(cases), 3) if cases else None,
        "repeat_window_days": list(REPEAT_WINDOW), "colposcopy_female_clinic_share_assumed": COLPO_FEMALE_SHARE,
        "note": "Synthetic colposcopy-clinic calendar at the same slots-per-case assumption; decision support only."}}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("run")
    ap.add_argument("--slots-per-fit", type=float, default=1.2, help="assumed endoscopy capacity (slots per open FIT+ case)")
    ap.add_argument("--seed", type=int, default=2026)
    ap.add_argument("--misallocated", action="store_true", help="put capacity in the wrong regions (pooling scenario)")
    ap.add_argument("--female-protect", type=float, default=FEMALE_PROTECT, help="female-list protection factor (0 = off)")
    a = ap.parse_args()
    last, sr = load(a.run)
    rng = np.random.default_rng(a.seed)
    open_cases = [(k, d) for k, d in sorted(last.items()) if d.get("prog") == "crc" and d["state"] in NEEDS_SLOT]
    as_of = 0  # suggestions are made from the start of an empty synthetic calendar window
    cal, share = calendar(len(open_cases), a.slots_per_fit, rng, a.misallocated)
    for _ in range(90):
        pass
    eng = BookingEngine(cal, NoShowModel(), eps=0.05, prep_lead=5, protect=a.female_protect or None)  # female-list protection
    meta = {}
    for k, d in open_cases:
        demo = sr.get(k, {})
        region = PROV_REGION.get(d["prov"], "AD")
        female = demo.get("gender") == "female"
        fem_pref = "hard" if female and stable_u(k, "fem") < 0.25 else ("soft" if female and stable_u(k, "fem") < 0.6 else "none")
        prefs = Prefs(female_endoscopist=fem_pref, arabic_only=demo.get("lang") == "ar" and stable_u(k, "ar") < 0.3,
                      region=region, travel_ok=stable_u(k, "travel") < (0.6 if a.misallocated else 0.3),
                      prior_no_show=d["state"] == "no_show")
        day = int(as_of + stable_u(k, "arr") * 60)  # spread requests over 60 days
        meta[k] = (d, prefs, day)
    by_day = collections.defaultdict(list)
    for k, (_, prefs, day) in meta.items():
        by_day[day].append(Event("result_accepted", k, day, prefs, day + 31))
    out = {}
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
    cases, unbooked = {}, 0
    reg_demand, reg_booked_out = collections.Counter(), collections.Counter()
    for k, (d, prefs, day) in meta.items():
        b = out.get(k)
        reg_demand[prefs.region] += 1
        lv = levers.assess(k)
        if b is None:
            unbooked += 1
            cases[k] = {"status": "no_slot_within_horizon", "procedure": "colonoscopy", "requested_day": day, "region": prefs.region,
                        "constraints": prefs.__dict__, "reason": "capacity", "state_at_run_end": d["state"], **lv}
            continue
        ru = b.unit.split("-")[0]
        if ru != prefs.region:
            reg_booked_out[prefs.region] += 1
        cases[k] = {"status": "suggested", "procedure": "colonoscopy", "requested_day": day, "region": prefs.region, "constraints": prefs.__dict__,
                    "state_at_run_end": d["state"], "booking": b.to_json(), "cross_region": ru != prefs.region,
                    "wait_days": b.day - day, **lv}
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
    json.dump(res, open(os.path.join(a.run, "booking.json"), "w"), indent=1)
    s = res["summary"]; c = res["capacity"]
    print(f"booking (simulated): {n} open CRC cases; capacity {c['condition']} ({c['slots']} slots, {c['slots_per_case']}/case); "
          f"suggested {s['suggested']}, within 31 d {s['within_31d']} ({s['within_31d_share']}), cross-region {s['cross_region']}, "
          f"no slot {s['no_slot']}; pooling relevant: {c['pooling_relevant']}; pathway {res['levers']['pathway']}, "
          f"prep risk {res['levers']['prep_risk_band']}")
    cs = res["cervical"]
    print(f"booking (simulated): {cs['cases']} cervical cases needing colposcopy ({cs['repeat_after_unsatisfactory']} repeats after "
          f"an unsatisfactory exam); suggested {cs['suggested']}, inside the DoH window {cs['within_window']} ({cs['within_window_share']})")


if __name__ == "__main__":
    main()
