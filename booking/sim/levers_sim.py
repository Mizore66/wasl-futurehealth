"""SIMULATION, not a measurement. Queue-lever arms on top of the Wasl booking engine (policy c of sim.py).

Arms (all use the same engine, cohort, calendar and random draws, so differences are paired):
  base   : every patient goes through a GI consult before the colonoscopy is booked (status-quo pathway).
  direct : App 9 nurse checklist call; patients with no clinic flag (ASA>=3, anticoagulation, device; booking/levers.py)
           who complete the call are booked straight into a colonoscopy slot (navigated direct access, the design).
           Clinic-flagged patients keep the consult. Patients who do not complete the call go to a navigator, unbooked.
  direct_ungated : sensitivity arm: every non-flagged patient is booked without the call gate (unnavigated direct access).
  prep   : consult pathway + prep-failure targeting (low band: SMS prep sequence; high band: nurse call + extended
           regimen if a clinician orders it).
  both   : direct + prep.

Calibration of the consult step (tagged):
  - Consult leak [PMC8343888 T5]: 360 of 1,429 FIT+ were "referred to a gastroenterologist but did not attend";
    that is 36.3% of the 993 non-completers and 57.2% of the convertible pool (133 no-result + 496 non-engaged).
    In sim.py convertible patients attend a booked colonoscopy with probability phi. Here, a convertible patient who
    would NOT attend is "lost at consult" with p = 0.572: under the consult pathway that patient is never booked.
  - Direct access removes the consult visit, but a patient lost at consult does not automatically attend
    (unnavigated direct access was slower in one US system, JGIM 2026; direct access was non-inferior, 62.7% vs 57.1%,
    PubMed 38630852). Gated arm: such a patient completes the nurse checklist call with c ~ U(0.4, 0.7) [A], and then
    attends with q ~ U(0.3, 0.6) [A]. Ungated arm: booked regardless; attends with c*q. Booked non-attenders waste
    slots (overbooking absorbs some), which is the honest cost of the lever.
  - Consult delay: lognormal, median 9 d, sigma 0.5 [Banerjea 2017: first test 22 d via clinic vs 13 d straight-to-test].
    Direct pathway: nurse checklist call, median 2 d [A].
Prep failure:
  - Population inadequate-prep rate without levers ~ U(0.12, 0.22) [A]; DoH App 9.5 caps it at 10%.
    Per-patient risk = logistic(intercept + ln(1.6) * Dik 2015 score) [slope A], intercept fitted to the drawn mean.
  - An inadequate exam uses a slot and needs a repeat (rebooked after the prep lead; at most 2 repeats [A]).
    A loop counts as done only on an ADEQUATE colonoscopy.
  - Low band (SMS): relative reduction of inadequate risk ~ U(0.12, 0.38) [A], centred on the pooled SMS trials
    (IJCD 2022, 7 RCTs: adequate 81.7% vs 75.7%, i.e. inadequate 18.3% vs 24.3%, RRR 0.25).
  - High band: relative reduction of inadequate risk ~ U(0.3, 0.7) [A; shrunk from 94% vs 49% adequate, one trial,
    DLD 2023], applied only when the clinician orders the regimen: P(order) ~ U(0.6, 0.9) [A].
  - The SMS no-show effect (RR 0.74) is NOT added: sim.py already assumes appointment reminders in every arm.

Usage (from booking/sim): python levers_sim.py [n_draws=30]  ->  levers_summary.json (raw draws: raw_levers.json)
"""
from __future__ import annotations
import sys, os, math, json
import numpy as np

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.dirname(HERE))
import sim
from booking.model import Prefs, Event, NoShowModel
from booking.engine import BookingEngine
from booking import levers

ARMS = ("base", "direct", "prep", "both", "direct_ungated")
CAPS = (0.8, 1.0, 1.2, 1.5)
ALLOCS = ("adequate", "misallocated")
SKEW = np.array([0.40, 0.50, 0.10])   # misallocated capacity share by region (patients stay 0.55/0.35/0.10), as pool_test.py
CONSULT_LOST_SHARE = 360 / (133 + 496)  # [PMC8343888 T5] = 0.572
MAX_REPEATS = 2


def lever_params(rng):
    return dict(
        c_call=rng.uniform(0.4, 0.7), q_direct=rng.uniform(0.3, 0.6),
        consult_med=9.0, consult_sig=0.5, nurse_med=2.0,
        inad_mean=rng.uniform(0.12, 0.22),
        rrr_sms=rng.uniform(0.12, 0.38),
        rrr_high=rng.uniform(0.3, 0.7),
        p_order=rng.uniform(0.6, 0.9),
    )


def lever_cohort(k, C, L, rng):
    """Per-patient App 9 assessment (levers.py, deterministic per draw/patient) + lever random numbers."""
    n = C["n"]
    route, score, band = np.empty(n, object), np.zeros(n, int), np.empty(n, object)
    for i in range(n):
        a = levers.assess(f"sim{k}-{i}")
        route[i], score[i], band[i] = a["pathway"]["route"], a["prep_risk"]["score"], a["prep_risk"]["band"]
    icpt = levers.fit_intercept(score.tolist(), L["inad_mean"])
    p_inad = np.array([levers.baseline_inadequate_p(int(s), icpt) for s in score])
    return dict(route=route, score=score, band=band, p_inad=p_inad,
                u_lost=rng.random(n), u_direct=rng.random(n), u_call=rng.random(n), z_consult=rng.normal(size=n), z_nurse=rng.normal(size=n),
                u_prep=rng.random((n, MAX_REPEATS + 2)), u_order=rng.random(n))


def p_inadequate(i, arm, X, L):
    p = X["p_inad"][i]
    if arm not in ("prep", "both"):
        return p
    if X["band"][i] == "low":
        return p * (1 - L["rrr_sms"])
    if X["u_order"][i] < L["p_order"]:
        return p * (1 - L["rrr_high"])
    return p


def simulate_arm(arm, P, C, X, L, cal, truth, eps=0.05):
    n = C["n"]
    colo = np.full(n, np.nan)
    attempts = np.zeros(n, int)
    repeats = np.zeros(n, int)
    st = dict(collisions=0, bumped=0, overbooked=0, procedures=0, inadequate=0, consult_lost=0, direct_routed=0, navigator=0)
    direct_arm = arm in ("direct", "both", "direct_ungated")
    gated = arm != "direct_ungated"
    attend = np.zeros(n, bool)
    submit_day = {}
    for i in range(n):
        pool = C["pool"][i]
        if pool not in (0, 1, 4):
            continue
        base_attend = pool == 0 or C["u_conv"][i] < P["phi"]
        lost = (not base_attend) and X["u_lost"][i] < CONSULT_LOST_SHARE
        direct = direct_arm and X["route"][i] == "direct"
        st["direct_routed"] += direct
        if direct:
            called = base_attend or X["u_call"][i] < L["c_call"]
            if gated and not called:
                st["navigator"] += 1
                continue                   # checklist call not completed: navigator follow-up, no slot used
            attend[i] = base_attend or (lost and called and X["u_direct"][i] < L["q_direct"])
            delay = math.exp(math.log(L["nurse_med"]) + 0.5 * X["z_nurse"][i])
        else:
            if lost:
                st["consult_lost"] += 1
                continue                   # never returns from the consult: not booked
            attend[i] = base_attend
            delay = math.exp(math.log(L["consult_med"]) + L["consult_sig"] * X["z_consult"][i])
        submit_day.setdefault(int(C["arr"][i]) + 1 + int(round(delay)), []).append(i)
    eng = BookingEngine(cal, NoShowModel(), prep_lead=P["prep"], eps=eps)
    booked_at = {}
    for day in range(0, sim.T_END + 1):
        for i in submit_day.get(day, []):
            fp = C["fem_pref"][i]
            pr = Prefs(female_endoscopist=["none", "soft", "hard"][fp], arabic_only=bool(C["arabic"][i]),
                       region=sim.REG[C["region"][i]], travel_ok=bool(C["travel"][i]), after_hours=bool(C["after"][i]),
                       avoid_ramadan=bool(C["fasting"][i]))
            eng.submit(Event("result_accepted", str(i), day, pr, int(C["arr"][i]) + 31))
        for b in eng.step(day):
            booked_at[(b.session_id, int(b.loop_id))] = (day, b.nurse_call)
            st["overbooked"] += b.overbooked
        if day % 30 == 29:
            eng.recalibrate()
        a_, b_ = cal.window(day, day)
        for s in range(a_, b_):
            pts = list(cal.booked[s].keys())
            if not pts:
                continue
            shows = []
            for pid in pts:
                i = int(pid)
                bday, nurse = booked_at.pop((s, i), (day, False))
                k = attempts[i]
                p = 1.0 if not attend[i] else truth.p(C, i, day - bday, cal, s, k > 0, nurse)
                show = C["u_show"][i, min(k, 9)] >= p
                if k == 0 and repeats[i] == 0:
                    eng.observe(cal.booked[s][pid], show)
                if show:
                    shows.append(i)
                else:
                    attempts[i] += 1
                    cal.remove(s, pid)
                    eng.submit(Event("no_show", str(i), day))
            capn = int(cal.cap[s])
            if len(shows) > capn:
                st["collisions"] += 1
                order = list(shows)
                C["_rng"].shuffle(order)
                for i in order[capn:]:
                    st["bumped"] += 1
                    cal.remove(s, str(i))
                    eng.submit(Event("bumped", str(i), day))
                shows = order[:capn]
            for i in shows:
                st["procedures"] += 1
                if X["u_prep"][i, repeats[i]] < p_inadequate(i, arm, X, L):
                    st["inadequate"] += 1
                    repeats[i] += 1
                    if repeats[i] <= MAX_REPEATS:   # repeat colonoscopy: needs a new slot after the prep lead
                        eng.where.pop(str(i), None)
                        eng.submit(Event("bumped", str(i), day))
                    continue
                colo[i] = day
    return colo, st


def measures(C, colo, st, cal):
    arr = C["arr"]
    m = (arr >= 365) & (arr < 730)
    t = colo[m] - arr[m]
    t = np.where(t <= 365, t, np.nan)
    done = ~np.isnan(t)
    n = int(m.sum())
    scale = 1000 / C["n"]   # per 1,000 FIT+ (whole cohort; counters are not windowed)
    adequate = max(1, int(np.sum(~np.isnan(colo))))
    return dict(
        d1_31=float((done & (t <= 31)).sum() / n), completion=float(done.sum() / n),
        median_days=float(np.nanmedian(t)) if done.any() else float("nan"),
        procedures_per_1000=st["procedures"] * scale, repeats_per_1000=st["inadequate"] * scale,
        repeats_per_1000_adequate=1000 * st["inadequate"] / adequate,
        inadequate_rate=st["inadequate"] / max(1, st["procedures"]),
        consult_lost_per_1000=st["consult_lost"] * scale, navigator_per_1000=st["navigator"] * scale, direct_share=st["direct_routed"] / max(1, int(np.isin(C["pool"], [0, 1, 4]).sum())),
        collisions_per_1000=st["collisions"] * scale,
    )


def make_cal(seed, P, cap, alloc):
    if alloc == "adequate":
        return sim.make_calendar(np.random.default_rng(seed + 1), P, cap)
    save = sim.REG_SHARE.copy()
    sim.REG_SHARE[:] = SKEW
    try:
        return sim.make_calendar(np.random.default_rng(seed + 1), P, cap)
    finally:
        sim.REG_SHARE[:] = save


def run_draw(k, caps=CAPS, allocs=ALLOCS, arms=ARMS):
    seed = 10_000 + 97 * k
    P = sim.draw_params(np.random.default_rng(seed))
    C = sim.make_cohort(np.random.default_rng(seed + 3), P)
    _, _, p0, _ = sim.calibrate_sq(P, C, seed, 1.2)
    lrng = np.random.default_rng(seed + 11)
    L = lever_params(lrng)
    X = lever_cohort(k, C, L, lrng)
    out = {"k": k, "lever_params": L, "res": {}}
    for alloc in allocs:
        for cap in caps:
            for arm in arms:
                cal = make_cal(seed, P, cap, alloc)
                C["_rng"] = np.random.default_rng(seed + 2)
                colo, st = simulate_arm(arm, P, C, X, L, cal, sim.Truth(P, p0))
                out["res"][f"{alloc}|{cap}|{arm}"] = measures(C, colo, st, cal)
    return out


def _ci(xs):
    xs = np.asarray(xs, float)
    return [round(float(np.mean(xs)), 4), round(float(np.percentile(xs, 2.5)), 4), round(float(np.percentile(xs, 97.5)), 4)]


def summarise(draws):
    keys = sorted(draws[0]["res"])
    S = {"n_draws": len(draws), "interval": "2.5-97.5 percentile over parameter draws; diffs are paired within draw", "cells": {}}
    for key in keys:
        alloc, cap, arm = key.split("|")
        cell = {m: _ci([d["res"][key][m] for d in draws]) for m in draws[0]["res"][key]}
        if arm != "base":
            b = f"{alloc}|{cap}|base"
            cell["diff_d1_31_vs_base_pp"] = _ci([100 * (d["res"][key]["d1_31"] - d["res"][b]["d1_31"]) for d in draws])
            cell["diff_completion_vs_base_pp"] = _ci([100 * (d["res"][key]["completion"] - d["res"][b]["completion"]) for d in draws])
            cell["slots_freed_per_1000_adequate"] = _ci([d["res"][b]["repeats_per_1000_adequate"] - d["res"][key]["repeats_per_1000_adequate"] for d in draws])
            cell["share_draws_better_d1_31"] = round(float(np.mean([d["res"][key]["d1_31"] > d["res"][b]["d1_31"] for d in draws])), 3)
        S["cells"][key] = cell
    return S


if __name__ == "__main__":
    import multiprocessing as mp, time
    N = int(sys.argv[1]) if len(sys.argv) > 1 else 30
    t0 = time.time()
    with mp.Pool(min(8, N)) as pool:
        draws = pool.map(run_draw, range(N))
    json.dump(draws, open(os.path.join(HERE, "raw_levers.json"), "w"))
    S = summarise(draws)
    json.dump(S, open(os.path.join(HERE, "levers_summary.json"), "w"), indent=1)
    print(f"{N} draws in {time.time() - t0:.0f}s")
