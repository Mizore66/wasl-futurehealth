"""Deterministic no-show recovery inputs over the existing synthetic booking calendars.

This projects future released-capacity opportunities; it never represents the missed
provider Appointment as available and never creates a reservation or outreach.
Usage: python booking/recovery.py <run_dir> [--seed 2026] [--slots-per-fit 1.2]
"""
from __future__ import annotations
import argparse, hashlib, json, math, os, sys
import numpy as np

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import suggest


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def digest(value):
    return hashlib.sha256(canonical(value)).hexdigest()


def latest_decisions(run):
    out = {}
    with open(os.path.join(run, "out_log.ndjson")) as f:
        for line in f:
            d = json.loads(line).get("data", {}).get("dec")
            if d:
                out[d["loop"]] = d
    return out


def appointment_sources(run, cases):
    """Keep full provider references; basedOn ties an Appointment to its own referral only."""
    versions = {}
    path = os.path.join(run, "fhir_log.ndjson")
    if not os.path.exists(path):
        return versions
    with open(path) as f:
        for seq, line in enumerate(f):
            msg = json.loads(line)
            if not msg.get("subj", "").endswith(".res"):
                continue
            provider = msg["subj"].split(".")[1]
            resource = msg.get("data")
            if isinstance(resource, str):
                resource = json.loads(resource)
            if not isinstance(resource, dict) or resource.get("resourceType") != "Appointment":
                continue
            ref = f"{provider}/Appointment/{resource.get('id', '')}"
            links = []
            for based in resource.get("basedOn") or []:
                value = based.get("reference", "")
                if value.startswith("ServiceRequest/"):
                    links.append(f"{provider}/{value}")
                display = based.get("display", "")
                if display.startswith("external referral "):
                    links.append(display.removeprefix("external referral "))
            # A resource without an explicit referral link is not safe release provenance.
            for case in set(links):
                if case not in cases:
                    # Synthetic downstream referrals may name the source provider explicitly.
                    matches = [k for k in cases if k.endswith("/" + value) and value]
                    if len(matches) == 1:
                        case = matches[0]
                    else:
                        continue
                old = versions.get(case)
                version = resource.get("_v", 0)
                if old is None or (version, seq) >= old[0]:
                    versions[case] = ((version, seq), ref, resource)
    return versions


def preference(booking_case):
    raw = booking_case.get("constraints") or {}
    allowed = {"female_endoscopist", "arabic_only", "region", "travel_ok", "after_hours", "avoid_ramadan", "prior_no_show"}
    return {k: raw[k] for k in allowed if k in raw}


def waitlist(case_id, decision, booking_case):
    requested = int(booking_case.get("requested_day", decision.get("sr_day", 0)))
    repeat = bool(booking_case.get("repeat_after_unsatisfactory"))
    window = booking_case.get("window_days") or []
    if repeat and len(window) == 2:
        lower, deadline = int(window[0]), int(window[1])
    else:
        lower = 0  # only an unsatisfactory-exam repeat has a lower-bound clinical window
        deadline = int(booking_case.get("booking", {}).get("deadline", requested + int(decision.get("deadline") or 31)))
    return {
        "case_id": case_id, "programme": decision.get("prog"), "state": decision.get("state"),
        "provider": decision.get("prov"), "priority": decision.get("prio", ""),
        "requested_day": requested, "repeat_after_unsatisfactory": repeat, "repeat_window_lower_bound": lower, "deadline_day": deadline,
        "constraints": preference(booking_case), "region": booking_case.get("region", suggest.PROV_REGION.get(decision.get("prov"), "AD")),
        "prep_lead": 1 if decision.get("prog") == "cervical" else 5,
        "existing_suggestion": booking_case.get("booking"), "source_decision_sig": decision.get("sig", ""),
    }


def compatible(session, constraints, region, earliest, as_of):
    if session["day"] < earliest or session["booked"] >= session["capacity"]:
        return False
    if session["region"] != region and not constraints.get("travel_ok", False):
        return False
    if constraints.get("female_endoscopist") == "hard" and not session["female"]:
        return False
    if constraints.get("avoid_ramadan") and session["ramadan"]:
        return False
    # Protected female-list capacity remains protected outside its seven-day release window.
    if session["female"] and constraints.get("female_endoscopist") != "hard" and session["day"] - as_of > 7:
        return False
    return True


def generate(run, seed=2026, slots_per_fit=1.2, booking=None):
    decisions = latest_decisions(run)
    if booking is None:
        with open(os.path.join(run, "booking.json")) as f:
            booking = json.load(f)
    cases = booking.get("cases") or {}
    as_of = int(math.ceil(max((float(d.get("now", 0)) for d in decisions.values()), default=0)))
    appointments = appointment_sources(run, decisions)
    waits = {case_id: waitlist(case_id, decisions[case_id], value)
             for case_id, value in sorted(cases.items())
             if case_id in decisions and decisions[case_id].get("prog") in ("crc", "cervical")}
    slots = []
    used_sessions = {}
    for case_id, decision in sorted(decisions.items()):
        programme = decision.get("prog")
        if programme not in ("crc", "cervical") or decision.get("state") != "no_show":
            continue
        source = appointments.get(case_id)
        if source is None or source[2].get("status") != "noshow":
            continue
        wait = waits.get(case_id)
        if wait is None:
            continue
        _, appointment_ref, appointment = source
        days = 160 if programme == "cervical" else 95
        n_fit = max(1, sum(1 for w in waits.values() if w["programme"] == programme))
        earliest = max(as_of + 1 + wait["prep_lead"], wait["repeat_window_lower_bound"] if wait["repeat_after_unsatisfactory"] else 0)
        matches = []
        # The existing calendar is stochastic and a very small demand can round to zero
        # sessions. Choose a deterministic seed lane with real regular capacity instead
        # of inventing an overbooked slot or changing slots-per-case.
        for attempt in range(128):
            cal, _ = suggest.calendar(n_fit, slots_per_fit * days / 95,
                                      np.random.default_rng(seed + (1 if programme == "cervical" else 0) + attempt * 1009),
                                      False, f_share=suggest.COLPO_FEMALE_SHARE if programme == "cervical" else 0.25,
                                      start=as_of + 1, days=days)
            sessions = []
            for sid in range(cal.n):
                sessions.append({"session_id": f"synthetic-{programme}-{int(cal.day[sid])}-{sid}", "unit": str(cal.unit[sid]),
                                 "region": str(cal.region[sid]), "day": int(cal.day[sid]), "capacity": int(cal.cap[sid]),
                                 "booked": int(cal.nb[sid]), "regular": True, "overbooked": False,
                                 "female": bool(cal.female[sid]), "arabic": bool(cal.arabic[sid]),
                                 "after_hours": bool(cal.after[sid]), "ramadan": bool(cal.ramadan[sid])})
            matches = [s for s in sessions if used_sessions.get(s["session_id"], 0) < s["capacity"]
                       and compatible(s, wait["constraints"], wait["region"], earliest, as_of)]
            if matches:
                break
        if not matches:
            continue
        session = sorted(matches, key=lambda s: (s["day"], s["session_id"]))[0]
        used_sessions[session["session_id"]] = used_sessions.get(session["session_id"], 0) + 1
        source_sig = decision.get("sig", "")
        slot_id = "rel_" + digest({"sourceAppointment": appointment_ref, "sourceDecisionSig": source_sig, "session": session["session_id"]})[:24]
        slot = {"slot_id": slot_id, "programme": programme,
                "procedure": "colonoscopy" if programme == "crc" else "colposcopy", "provider": decision.get("prov"),
                "session_id": session["session_id"], "unit": session["unit"], "region": session["region"], "day": session["day"],
                "capacity": session["capacity"], "regular": True, "overbooked": False,
                "female": session["female"], "arabic": session["arabic"], "after_hours": session["after_hours"], "ramadan": session["ramadan"],
                "source_case": case_id, "source_appointment": appointment_ref, "source_decision_sig": source_sig,
                "source_appointment_version": appointment.get("_v", 0),
                "source_booking_fingerprint": digest(cases[case_id].get("booking")),
                "projection_basis": "no_show_plus_future_synthetic_capacity_window"}
        slot["base_head"] = digest(slot)
        slot["source_fingerprint"] = digest({"appointment": appointment, "decision_sig": source_sig, "base_head": slot["base_head"]})
        slots.append(slot)
    result = {"schema": "wasl-recovery-input/1", "data_mode": "simulated", "decision_support_only": True,
              "canonical_unchanged": True, "as_of_day": as_of,
              "notice": "Simulated released-capacity projection, not the original provider Appointment or a real cancellation.",
              "ranking_policy": ["urgent_priority_first", "absolute_deadline_ascending", "requested_day_ascending", "case_id_ascending"],
              "slots": slots, "waitlist": list(waits.values())}
    result["input_fingerprint"] = digest({k: v for k, v in result.items() if k != "input_fingerprint"})
    return result


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("run")
    parser.add_argument("--seed", type=int, default=2026)
    parser.add_argument("--slots-per-fit", type=float, default=1.2)
    parser.add_argument("--out")
    args = parser.parse_args(argv)
    result = generate(args.run, args.seed, args.slots_per_fit)
    path = args.out or os.path.join(args.run, "recovery.json")
    with open(path, "w") as f:
        json.dump(result, f, indent=1, sort_keys=True)
        f.write("\n")
    print(f"recovery (simulated): {len(result['slots'])} projected released slots; {len(result['waitlist'])} waitlist cases; no booking or message made")
    return result


if __name__ == "__main__":
    main()
