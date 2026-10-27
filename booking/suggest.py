"""Stage 6: booking as SIMULATED decision support for a materialised Wasl run (not live scheduling).

usage: python booking/suggest.py <run_dir> [--slots-per-fit 1.2] [--seed 2026] [--misallocated]

Reads the run's final decisions (out_log.ndjson) + referral demographics (fhir_log.ndjson), takes every CRC loop that is
still open at the end of the run and would need a colonoscopy slot, and feeds them to the real booking engine
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


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("run")
    ap.add_argument("--slots-per-fit", type=float, default=1.2, help="assumed endoscopy capacity (slots per open FIT+ case)")
    ap.add_argument("--seed", type=int, default=2026)
    ap.add_argument("--misallocated", action="store_true", help="put capacity in the wrong regions (pooling scenario)")
    a = ap.parse_args()
    last, sr = load(a.run)
    rng = np.random.default_rng(a.seed)
    open_cases = [(k, d) for k, d in sorted(last.items()) if d.get("prog") == "crc" and d["state"] in NEEDS_SLOT]
    as_of = 0  # suggestions are made from the start of an empty synthetic calendar window
    cal, share = calendar(len(open_cases), a.slots_per_fit, rng, a.misallocated)
    for _ in range(90):
        pass
    eng = BookingEngine(cal, NoShowModel(), eps=0.05, prep_lead=5)
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
            cases[k] = {"status": "no_slot_within_horizon", "requested_day": day, "region": prefs.region,
                        "constraints": prefs.__dict__, "reason": "capacity", "state_at_run_end": d["state"], **lv}
            continue
        ru = b.unit.split("-")[0]
        if ru != prefs.region:
            reg_booked_out[prefs.region] += 1
        cases[k] = {"status": "suggested", "requested_day": day, "region": prefs.region, "constraints": prefs.__dict__,
                    "state_at_run_end": d["state"], "booking": b.to_json(), "cross_region": ru != prefs.region,
                    "wait_days": b.day - day, **lv}
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
                    "median_wait_days": float(np.median([c["wait_days"] for c in sugg])) if sugg else None},
        "levers": {"schema": levers.SCHEMA, "data_mode": "synthetic App 9 fields; Dik 2015 published score weights",
                   "pathway": dict(collections.Counter(c["pathway"]["route"] for c in cases.values())),
                   "prep_risk_band": dict(collections.Counter(c["prep_risk"]["band"] for c in cases.values())),
                   "requires_clinician_order": True,
                   "simulated_effect": "booking/RESULTS.md, Queue levers (simulated)"},
        "cases": cases,
    }
    json.dump(res, open(os.path.join(a.run, "booking.json"), "w"), indent=1)
    s = res["summary"]; c = res["capacity"]
    print(f"booking (simulated): {n} open CRC cases; capacity {c['condition']} ({c['slots']} slots, {c['slots_per_case']}/case); "
          f"suggested {s['suggested']}, within 31 d {s['within_31d']} ({s['within_31d_share']}), cross-region {s['cross_region']}, "
          f"no slot {s['no_slot']}; pooling relevant: {c['pooling_relevant']}; pathway {res['levers']['pathway']}, "
          f"prep risk {res['levers']['prep_risk_band']}")


if __name__ == "__main__":
    main()
