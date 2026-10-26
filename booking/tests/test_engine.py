"""Checks: (1) batch assignment equals the explicit ILP optimum (HiGHS via scipy.milp) on random instances;
(2) hard constraints (female endoscopist, Ramadan, region, prep lead) are never violated;
(3) the Wasl interface round-trips: JSON event in -> booking out -> FHIR Appointment.
Run: python tests/test_engine.py"""
import sys, os, json
import numpy as np
from scipy.optimize import milp, LinearConstraint, Bounds
from scipy.sparse import lil_matrix

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from booking.model import Prefs, Event, NoShowModel
from booking.calendar import Calendar
from booking.engine import BookingEngine, BIG


def rand_cal(rng, n=60, days=50):
    d = np.sort(rng.integers(0, days, n))
    return Calendar(d, [f"U{i%3}" for i in range(n)], rng.choice(["AD", "AA", "DH"], n, p=[.5, .35, .15]),
                    rng.integers(1, 4, n), rng.random(n) < .3, rng.random(n) < .5, rng.random(n) < .2,
                    (d > 30) & (d < 40))


def rand_prefs(rng):
    return Prefs(female_endoscopist=rng.choice(["none", "soft", "hard"], p=[.5, .3, .2]),
                 arabic_only=bool(rng.random() < .3), region=str(rng.choice(["AD", "AA", "DH"])),
                 travel_ok=bool(rng.random() < .3), after_hours=bool(rng.random() < .2),
                 avoid_ramadan=bool(rng.random() < .5), prior_no_show=bool(rng.random() < .2))


def ilp_opt(C):
    n, m = C.shape
    ok = C < BIG
    idx = np.argwhere(ok)
    nv = len(idx)
    A = lil_matrix((n + m, nv))
    for v, (i, j) in enumerate(idx):
        A[i, v] = 1
        A[n + j, v] = 1
    lo = np.r_[np.ones(n), np.zeros(m)]
    hi = np.r_[np.ones(n), np.ones(m)]
    r = milp(C[ok], constraints=LinearConstraint(A.tocsr(), lo, hi), integrality=np.ones(nv), bounds=Bounds(0, 1))
    return r.fun


def test_optimality(trials=40):
    rng = np.random.default_rng(1)
    gaps = []
    for t in range(trials):
        cal = rand_cal(rng)
        # pre-fill some sessions so overbook columns appear
        for s in range(cal.n):
            for k in range(int(rng.integers(0, cal.cap[s] + 1))):
                cal.add(s, f"pre{s}_{k}", float(rng.uniform(.1, .5)))
        eng = BookingEngine(cal, NoShowModel(p0=0.3), eps=0.10, prep_lead=3, horizon=60)
        for i in range(int(rng.integers(5, 30))):
            eng.submit(Event("result_accepted", f"p{i}", 0, rand_prefs(rng), int(rng.integers(10, 40))))
        P, C, m_cols, cols_s = eng.build(0)
        from scipy.optimize import linear_sum_assignment
        r, c = linear_sum_assignment(C)
        lsa = C[r, c].sum()
        ilp = ilp_opt(C)
        gaps.append(abs(lsa - ilp))
        assert abs(lsa - ilp) < 1e-6 * max(1, abs(ilp)), (t, lsa, ilp)
    return f"assignment == ILP (HiGHS) on {trials}/{trials} random batches, max |gap| = {max(gaps):.2e}"


def test_constraints(trials=40):
    rng = np.random.default_rng(2)
    nb = 0
    for t in range(trials):
        cal = rand_cal(rng)
        eng = BookingEngine(cal, NoShowModel(), eps=0.05, prep_lead=4, horizon=60)
        prefs = {}
        for i in range(25):
            p = rand_prefs(rng)
            prefs[f"p{i}"] = p
            eng.submit(Event("result_accepted", f"p{i}", 0, p))
        for b in eng.step(0):
            p, s = prefs[b.loop_id], b.session_id
            nb += 1
            assert b.day >= 4
            if p.female_endoscopist == "hard":
                assert cal.female[s]
            if p.avoid_ramadan:
                assert not cal.ramadan[s]
            if not p.travel_ok:
                assert cal.region[s] == p.region
    return f"hard constraints held on {nb} bookings"


def test_interface():
    rng = np.random.default_rng(3)
    cal = rand_cal(rng)
    eng = BookingEngine(cal, NoShowModel(), prep_lead=4)
    ev = Event.from_json(json.loads(json.dumps({"kind": "result_accepted", "loop_id": "CRC-000123", "day": 0,
                                                "prefs": {"female_endoscopist": "hard", "region": "AD", "travel_ok": True}})))
    out = eng.handle(ev)
    assert len(out) == 1
    b = out[0]
    fh = b.to_fhir_appointment()
    out2 = eng.handle(Event("no_show", "CRC-000123", b.day))
    assert len(out2) == 1 and out2[0].day >= b.day + 4
    return "event->booking->FHIR ok: " + json.dumps(b.to_json()) + " | rebook after no-show -> day " + str(out2[0].day)


if __name__ == "__main__":
    for f in (test_optimality, test_constraints, test_interface):
        print(f.__name__, ":", f())
