"""SIMULATION, not a measurement. FIT+ -> colonoscopy in Abu Dhabi under three booking policies.

Every parameter is tagged: [PMC8343888] SEHA AHS 2015-16 (BMC Cancer 2021), [PEERJ22] Yilmaz & Kocyigit PeerJ 2022
(endoscopy no-show, n=5,938), [GIE16] nurse call OR 0.67, [NOSHOW-PRIOR] prior no-show OR 3.8 (1.7-8.5),
[KAMANI21] Muslim women 52.6% prefer female endoscopist (GIE 2021), [MALAY] 67.7% prefer, 1/3 would decline,
[JAMAIM26] automated referral RR 1.68 (1.09-2.60), [DOH-CRC] DoH CRC standard, [BRUNEI] Ramadan endoscopy
workload -34% (upper GI), or 'assumed'.

Patient pools (Table 5 of [PMC8343888], 1,429 FIT+), Dirichlet over the counts:
  engaged 528 (436 done + 92 scheduled-not-done) | no-result 133 | clinically excluded 160 (counselled /
  known GI disease / known CRC) | refused 112 | non-engaged 496 (referred never attended 360 + others 136).
Excluded and refusers never get a colonoscopy in any arm (booking cannot fix refusal).
No-result + non-engaged are 'convertible': with automated booking (b, c) a fraction phi attends; phi is solved from
the [JAMAIM26] RR draw. The rest are booked but never show (they waste slots, which is what overbooking absorbs).
"""
from __future__ import annotations
import sys, os, math, json, heapq
import numpy as np

D = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(D))
from booking.model import Prefs, Event, NoShowModel
from booking.calendar import Calendar
from booking.engine import BookingEngine, Weights

REG = ["AD", "AA", "DH"]
REG_SHARE = np.array([0.55, 0.35, 0.10])        # assumed (approx. population split)
UNITS = {"AD": 3, "AA": 2, "DH": 1}              # assumed
LIST_CAP = 4                                     # FIT+ slots per endoscopy list, assumed
# day 0 = 2027-01-01 (Friday). Ramadan / Eid windows approximate (astronomical calendar).
RAMADAN = [(38, 66), (392, 420), (746, 774), (1100, 1129)]
HOLIDAYS = set()
for a, b in RAMADAN:
    HOLIDAYS |= {b + 1, b + 2, b + 3}
for e in (135, 490, 843, 1197):
    HOLIDAYS |= {e, e + 1, e + 2, e + 3}
T_ARR = 730          # arrivals over 2 years; measure year 2 (days 365-729) after a 1-year warm-up
T_END = T_ARR + 365  # follow-up 365 d
RATE = 1000 / 365    # FIT+ per day -> 1,000 per year


from booking.engine import Weights as _W
# c_aggr: overbooking penalty kappa=0.6 (a collision priced at ~60 days of delay) with eps=0.10; reported as an
# operator trade-off, chosen AFTER seeing kappa_check.json (so it is a sensitivity row, not the pre-registered arm)
ABLATIONS = {"c_aggr": {"w": _W(kappa_overbook=0.6), "eps": 0.10}, "c_greedy": {"solver": "greedy"}, "c_noob": {"eps": 0.0}, "c_norebook": {"max_rebooks": 0},
             "c_nonurse": {"nurse_threshold": 2.0},
             # female-list reservation: c_res7 = fixed 7-day window (ablation); c_fem = demand-sized protection, factor 4
             # (chosen on design draws 150-189, reported on 0-149)
             "c_res7": {"reserve_days": 7}, "c_fem": {"protect": 4.0}}


def in_ramadan(d):
    return any(a <= d <= b for a, b in RAMADAN)


def draw_params(rng):
    """One draw of the uncertain world."""
    pools = rng.dirichlet([528, 133, 160, 112, 496])
    rr_ref = math.exp(rng.normal(math.log(1.68), (math.log(2.60) - math.log(1.09)) / 3.92))
    conv = pools[1] + pools[4]
    # phi: share of convertible patients who attend once booked automatically, solved so that b's completion is
    # ~0.305*RR [JAMAIM26], assuming 90% of the engaged pool completes under automated booking (assumed) and
    # convertible attenders show at ~85% (assumed)
    phi = float(np.clip((0.305 * rr_ref - 0.9 * pools[0]) / (conv * 0.85), 0, 1))
    return dict(
        pools=pools, rr_ref=rr_ref, phi=phi,
        med_target=72 * math.exp(rng.normal(0, 0.10)),          # [PMC8343888] median 72 d
        iqr_target=114.25 * math.exp(rng.normal(0, 0.10)),      # [PMC8343888] IQR 114.25 d
        ns_target=rng.beta(92, 436),                            # [PMC8343888] 92/528 scheduled-not-done
        or_lead=math.exp(rng.normal(math.log(1.006), (math.log(1.008) - math.log(1.004)) / 3.92)),  # [PEERJ22]
        or_prior=math.exp(rng.normal(math.log(3.8), (math.log(8.5) - math.log(1.7)) / 3.92)),       # [NOSHOW-PRIOR]
        or_nurse=math.exp(rng.normal(math.log(0.67), (math.log(0.91) - math.log(0.50)) / 3.92)),    # [GIE16]
        or_soft=rng.uniform(1.2, 2.0),           # assumed
        p_hard=rng.uniform(0.5, 0.9),            # assumed: hard-preference woman booked with a male endoscopist
        p_ramadan=rng.uniform(0.3, 0.7),         # assumed: fasting patient booked inside Ramadan
        p_pref=rng.uniform(0.45, 0.70),          # [KAMANI21] 52.6%, [MALAY] 67.7%
        hard_given_pref=rng.uniform(0.2, 0.4),   # [MALAY] ~1/3 would decline
        f_share=rng.uniform(0.15, 0.30),         # assumed share of lists with a female endoscopist
        ram_cap=rng.uniform(0.6, 0.8),           # assumed, cf. [BRUNEI] -34% workload
        prep=int(rng.integers(3, 8)),            # assumed bowel-prep instruction lead 3-7 d
        ref_lat=rng.uniform(3, 14),              # assumed GP->GI automated referral latency median (policy b)
        r_self=rng.uniform(0.2, 0.5),            # assumed P(self-rebook after a no-show), policies a/b
        switch=rng.uniform(0.3, 0.7),            # assumed P(hard-pref woman asks for a female list), a/b
    )


def make_cohort(rng, P):
    n_days = T_ARR
    counts = rng.poisson(RATE, n_days)
    arr = np.repeat(np.arange(n_days), counts)
    n = len(arr)
    female = rng.random(n) < 0.696                      # [PMC8343888] 69.6% of FIT+ were women
    pref = female & (rng.random(n) < P["p_pref"])
    hard = pref & (rng.random(n) < P["hard_given_pref"])
    pool = rng.choice(5, n, p=P["pools"])               # 0 engaged 1 no-result 2 excluded 3 refused 4 non-engaged
    return dict(
        n=n, arr=arr, female=female, fem_pref=np.where(hard, 2, np.where(pref, 1, 0)),
        region=rng.choice(3, n, p=REG_SHARE),
        arabic=rng.random(n) < 0.30,                    # assumed
        after=rng.random(n) < 0.25,                     # assumed needs Saturday/after-hours
        fasting=rng.random(n) < 0.60,                   # assumed avoids Ramadan procedures
        travel=rng.random(n) < 0.30,                    # assumed willing to travel to another region
        pool=pool, u_conv=rng.random(n), z_delay=rng.normal(size=n), z_ref=rng.normal(size=n),
        z_rebook=rng.normal(size=(n, 8)), u_show=rng.random((n, 10)), u_switch=rng.random(n), u_self=rng.random((n, 8)),
    )


def make_calendar(rng, P, c):
    days, unit, region, cap, fem, ara, aft, ram = [], [], [], [], [], [], [], []
    for r, name in enumerate(REG):
        lists_per_week = c * 1000 * REG_SHARE[r] / 52 / LIST_CAP
        rate = lists_per_week / 6
        acc = rng.random()
        k = 0
        for d in range(T_END + 200):
            wd = (d + 4) % 7          # 0=Mon .. 6=Sun ; day0 is a Friday
            if wd == 6 or d in HOLIDAYS:
                continue
            acc += rate
            while acc >= 1:
                acc -= 1
                if in_ramadan(d) and rng.random() > P["ram_cap"]:
                    continue
                days.append(d); unit.append(f"{name}-{k % UNITS[name] + 1}"); region.append(name)
                cap.append(LIST_CAP); fem.append(rng.random() < P["f_share"]); ara.append(rng.random() < 0.5)
                aft.append(wd == 5); ram.append(in_ramadan(d)); k += 1
    return Calendar(days, unit, region, cap, fem, ara, aft, ram)


class Truth:
    def __init__(self, P, intercept_p):
        self.m = NoShowModel(p0=intercept_p, or_lead=P["or_lead"], or_prior=P["or_prior"], or_soft=P["or_soft"],
                             or_nurse=P["or_nurse"], p_ramadan=P["p_ramadan"])
        self.P = P

    def p(self, C, i, lead, cal, s, prior, nurse):
        fp = C["fem_pref"][i]
        f = bool(cal.female[s])
        p = float(self.m.p(lead, prior, fp == 1 and not f, C["arabic"][i] and not cal.arabic[s],
                           C["after"][i] and not cal.after[s], 1, nurse,
                           C["fasting"][i] and cal.ramadan[s], REG[C["region"][i]] != cal.region[s]))
        if fp == 2 and not f:
            p = max(p, self.P["p_hard"])
        return p


def attends(C, i, policy, P):
    pool = C["pool"][i]
    if pool == 0:
        return True
    if pool in (1, 4) and policy != "a":
        return C["u_conv"][i] < P["phi"]
    return False


def bookable(C, i, policy):
    pool = C["pool"][i]
    return pool == 0 if policy == "a" else pool in (0, 1, 4)


def simulate(policy, P, C, cal, truth, med_p=None, sig_p=None, eps=0.05, engine_kw=None):
    """Returns per-patient colonoscopy day (nan if none) and diagnostics."""
    n = C["n"]
    colo = np.full(n, np.nan)
    first_appt = np.full(n, np.nan)
    attempts = np.zeros(n, int)
    first_ns = []
    st = dict(collisions=0, bumped=0, escalated=0, overbooked=0, nurse_calls=0, bookings=0)
    req = []  # heap of (day, i) booking requests for a/b
    attend = np.array([attends(C, i, policy, P) for i in range(n)])
    for i in range(n):
        if not bookable(C, i, policy):
            continue
        if policy == "a":
            d = C["arr"][i] + math.exp(math.log(med_p) + sig_p * C["z_delay"][i])
            heapq.heappush(req, (int(round(d)), i))
        elif policy.startswith("b"):
            d = C["arr"][i] + 1 + math.exp(math.log(P["ref_lat"]) + 0.5 * C["z_ref"][i])
            heapq.heappush(req, (int(round(d)), i))
    eng = None
    if policy.startswith("c"):
        kw = dict(engine_kw or {})
        kw.update(ABLATIONS.get(policy, {}))
        eng = BookingEngine(cal, NoShowModel(), prep_lead=P["prep"], **{"eps": eps, **kw})
    if eng is not None:
        arrivals = {}
        for i in range(n):
            if bookable(C, i, "c"):
                arrivals.setdefault(int(C["arr"][i]) + 1, []).append(i)   # result accepted next day
    by_day = {}   # session day -> list of sessions with bookings (implicit through cal.day)
    booked_at = {}  # (s, pid) -> booking day, nurse flag
    s_ptr = 0
    for day in range(0, T_END + 1):
        if policy in ("a", "b", "b_pref"):
            while req and req[0][0] <= day:
                _, i = heapq.heappop(req)
                earliest = day + P["prep"]
                if policy == "b_pref":   # first-available but honouring hard female / Ramadan preferences
                    s = cal.first_free(earliest, REG[C["region"][i]], C["fem_pref"][i] == 2, bool(C["fasting"][i]))
                else:
                    need_f = (C["fem_pref"][i] == 2) and (C["u_switch"][i] < P["switch"])
                    s = cal.first_free(earliest, REG[C["region"][i]], need_f)
                if s < 0:
                    continue
                cal.add(s, i, 0.0)
                if np.isnan(first_appt[i]):
                    first_appt[i] = cal.day[s]
                booked_at[(s, i)] = (day, False)
                st["bookings"] += 1
        else:
            for i in arrivals.get(day, []):
                fp = C["fem_pref"][i]
                pr = Prefs(female_endoscopist=["none", "soft", "hard"][fp], arabic_only=bool(C["arabic"][i]),
                           region=REG[C["region"][i]], travel_ok=bool(C["travel"][i]), after_hours=bool(C["after"][i]),
                           avoid_ramadan=bool(C["fasting"][i]))
                eng.submit(Event("result_accepted", str(i), day, pr, int(C["arr"][i]) + 31))
            for b in eng.step(day):
                i = int(b.loop_id)
                if np.isnan(first_appt[i]):
                    first_appt[i] = b.day
                booked_at[(b.session_id, i)] = (day, b.nurse_call)
                st["bookings"] += 1; st["overbooked"] += b.overbooked; st["nurse_calls"] += b.nurse_call
            if day % 30 == 29:
                eng.recalibrate()
        # realise today's sessions
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
                if eng is not None and k == 0:
                    pred = cal.booked[s][pid]
                    eng.observe(pred, show)
                if k == 0:
                    first_ns.append(not show)
                if show:
                    shows.append(i)
                else:
                    attempts[i] += 1
                    cal.remove(s, pid)
                    if eng is not None:
                        eng.submit(Event("no_show", str(i), day))
                    elif policy.startswith("b") and attempts[i] < 8 and C["u_self"][i, attempts[i]] < P["r_self"]:
                        # status quo (a): Table 5 counts 'scheduled, not done' as final -> no rebooking
                        d = day + math.exp(math.log(21) + 0.5 * C["z_rebook"][i, attempts[i]])  # assumed median 21 d
                        heapq.heappush(req, (int(round(d)), i))
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
                colo[i] = day
    if eng is not None:
        st["escalated"] = len(eng.escalated)
    st["first_attempt_no_show"] = float(np.mean(first_ns)) if first_ns else float("nan")
    st["first_appt"] = first_appt
    return colo, st


def measures(C, colo, cal, st, P):
    arr = C["arr"]
    m = (arr >= 365) & (arr < 730)
    t = colo[m] - arr[m]
    t = np.where(t <= 365, t, np.nan)
    done = ~np.isnan(t)
    n = m.sum()
    within = done & (t <= 31)
    a, b = cal.window(365, 729)
    slots = cal.cap[a:b].sum()
    performed = np.sum((colo >= 365) & (colo <= 729))
    out = dict(
        n=int(n),
        d1_31=float(within.sum() / n),                    # all FIT+
        d3_31=float(within.sum() / max(1, done.sum())),   # among colonoscopies done (KPI as written)
        d2_31=float(within.sum() / max(1, np.isin(C["pool"][m], [0, 1, 4]).sum())),  # referred, not excluded/refused
        sched_31=float(np.sum((st["first_appt"][m] - arr[m]) <= 31) / max(1, np.isin(C["pool"][m], [0, 1, 4]).sum())),
        completion=float(done.sum() / n),
        median_days=float(np.nanmedian(t)) if done.any() else float("nan"),
        utilisation=float(performed / max(1, slots)),
        collisions_per_1000=st["collisions"] / (C["n"] / 1000),
        bumped_per_1000=st["bumped"] / (C["n"] / 1000),
        overbooked_per_1000=st["overbooked"] / (C["n"] / 1000),
        no_show_first=st["first_attempt_no_show"],
    )
    groups = {
        "women_hard_pref": C["fem_pref"] == 2, "women_soft_pref": C["fem_pref"] == 1,
        "women_no_pref": C["female"] & (C["fem_pref"] == 0), "men": ~C["female"], "not_hard_pref": C["fem_pref"] != 2,
        "al_dhafra": C["region"] == 2, "after_hours": C["after"], "arabic_only": C["arabic"],
    }
    elig = np.isin(C["pool"], [0, 1, 4])[m]
    for g, gm in groups.items():
        gm2 = gm[m]
        dd = done & gm2
        out[f"g_{g}_d3"] = float((within & gm2).sum() / max(1, dd.sum()))
        out[f"g_{g}_d2"] = float((within & gm2).sum() / max(1, (gm2 & elig).sum()))
        out[f"g_{g}_med"] = float(np.nanmedian(t[gm2])) if dd.any() else float("nan")
    return out


def calibrate_sq(P, C, seed, c):
    """Fit the status-quo patient-side delay (median, sigma) and the no-show intercept to the SEHA targets."""
    med_p, sig_p, p0 = P["med_target"] * 0.8, 1.0, P["ns_target"]
    for it in range(6):
        cal = make_calendar(np.random.default_rng(seed + 1), P, c)
        truth = Truth(P, p0)
        C["_rng"] = np.random.default_rng(seed + 2)
        colo, st = simulate("a", P, C, cal, truth, med_p, sig_p)
        t = colo - C["arr"]
        t = t[(~np.isnan(t)) & (t <= 365)]
        med, q1, q3 = np.median(t), np.percentile(t, 25), np.percentile(t, 75)
        ns = st["first_attempt_no_show"]
        med_p *= P["med_target"] / med
        sig_p = float(np.clip(sig_p * ((P["iqr_target"] / P["med_target"]) / ((q3 - q1) / med)) ** 0.8, 0.3, 2.5))
        lg = lambda x: math.log(x / (1 - x))
        z = lg(p0) + (lg(P["ns_target"]) - lg(min(max(ns, 0.01), 0.99)))
        p0 = 1 / (1 + math.exp(-z))
    return med_p, sig_p, p0, dict(median=float(med), iqr=float(q3 - q1), no_show=float(ns),
                                  completion=float(len(t) / C["n"]))


KEEP = ("d1_31", "d2_31", "d3_31", "sched_31", "g_women_hard_pref_d3", "g_women_hard_pref_d2", "g_not_hard_pref_d3",
        "g_not_hard_pref_d2", "median_days", "utilisation", "collisions_per_1000", "bumped_per_1000",
        "overbooked_per_1000", "completion", "no_show_first")


def run_draw(args):
    """cfg: main capacity, policies at main capacity, sweep capacities (b, b_pref, c), param overrides."""
    k, cfg = args
    seed = 10_000 + 97 * k
    rng = np.random.default_rng(seed)
    P = draw_params(rng)
    P.update(cfg.get("override", {}))
    if "phi" in cfg.get("override", {}):
        pass
    C = make_cohort(np.random.default_rng(seed + 3), P)
    med_p, sig_p, p0, calib = calibrate_sq(P, C, seed, cfg["c_main"])
    res = {"k": k, "calib": calib, "main": {}, "sweep": {},
           "params": {k2: float(v) for k2, v in P.items() if np.isscalar(v)}}

    def one(pol, cc, eps=0.05):
        cal = make_calendar(np.random.default_rng(seed + 1), P, cc)
        C["_rng"] = np.random.default_rng(seed + 2)
        colo, st = simulate(pol, P, C, cal, Truth(P, p0), med_p, sig_p, eps=eps)
        return measures(C, colo, cal, st, P)
    for pol in cfg.get("policies", ()):
        res["main"][pol] = one(pol, cfg["c_main"])
    for cc in cfg.get("sweep", ()):
        for pol in cfg.get("sweep_policies", ("b", "c")):
            mm = one(pol, cc)
            res["sweep"][f"{pol}@{cc}"] = {k2: mm[k2] for k2 in KEEP}
    for e in cfg.get("eps", ()):
        mm = one("c", cfg["c_main"], e)
        res["sweep"][f"eps@{e}"] = {k2: mm[k2] for k2 in KEEP}
    for v in cfg.get("variants", ()):
        P2 = dict(P); P2.update(v["override"])
        C2 = make_cohort(np.random.default_rng(seed + 3), P2)
        m2, s2, q2, _ = calibrate_sq(P2, C2, seed, cfg["c_main"])
        for pol, cc in v["runs"]:
            cal = make_calendar(np.random.default_rng(seed + 1), P2, cc)
            C2["_rng"] = np.random.default_rng(seed + 2)
            colo, st = simulate(pol, P2, C2, cal, Truth(P2, q2), m2, s2)
            mm = measures(C2, colo, cal, st, P2)
            res["sweep"][f"{v['name']}|{pol}@{cc}"] = {k2: mm[k2] for k2 in KEEP}
    return res


if __name__ == "__main__":
    import multiprocessing as mp, time
    N, tag = int(sys.argv[1]), sys.argv[2]
    cfg = json.loads(sys.argv[3])
    t0 = time.time()
    with mp.Pool(min(10, N)) as pool:
        out = pool.map(run_draw, [(k, cfg) for k in range(N)])
    json.dump({"cfg": cfg, "draws": out}, open(os.path.join(D, f"raw_{tag}.json"), "w"))
    print(f"{N} draws in {time.time() - t0:.0f}s")
