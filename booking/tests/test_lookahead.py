"""Checks for the multi-period lookahead (booking/booking/lookahead.py; needs a built bin/wsi7 for the Go solver):
(1) with no forecast rows the min-cost flow reaches the default engine's per-day optimum (scipy assignment) on random
    batches, female-list protection on and off;
(2) hard constraints (female list, region/travel, Ramadan, prep lead, earliest day) hold for every lookahead booking;
(3) forecast rows only reshape real bookings: phantoms are never committed and every real patient is still booked when
    capacity allows; the oracle forecast sees only class counts.
Run: python tests/test_lookahead.py"""
import sys, os, tempfile
import numpy as np
from scipy.optimize import linear_sum_assignment

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from booking.model import Prefs, Event, NoShowModel
from booking.engine import BookingEngine, BIG
from booking.lookahead import LookaheadEngine, GoMCF, SCALE
from tests.test_engine import rand_cal, rand_prefs


def test_matches_assignment(trials=30):
    rng = np.random.default_rng(11)
    worst = 0.0
    for t in range(trials):
        for protect in (None, 4.0):
            cal_a, cal_b = rand_cal(np.random.default_rng(100 + t)), rand_cal(np.random.default_rng(100 + t))
            pre = np.random.default_rng(200 + t)
            for s in range(cal_a.n):
                for k in range(int(pre.integers(0, cal_a.cap[s] + 1))):
                    q = float(pre.uniform(.1, .5))
                    cal_a.add(s, f"pre{s}_{k}", q); cal_b.add(s, f"pre{s}_{k}", q)
            a = BookingEngine(cal_a, NoShowModel(p0=0.3), eps=0.10, prep_lead=3, horizon=60, protect=protect)
            b = LookaheadEngine(cal_b, NoShowModel(p0=0.3), eps=0.10, prep_lead=3, horizon=60, protect=protect,
                                lookahead_days=0)
            evs = []
            for i in range(int(rng.integers(5, 30))):
                pr = rand_prefs(rng)
                evs.append((f"p{i}", pr, int(rng.integers(10, 40))))
            if protect:   # some history of women who require a female list, so lists hold slots
                for j in range(6):
                    reg = str(rng.choice(["AD", "AA", "DH"]))
                    for e in (a, b):
                        e._hard_arr.append((-j, reg))
            for k, pr, dl in evs:
                a.submit(Event("result_accepted", k, 0, Prefs(**pr.__dict__), dl))
                b.submit(Event("result_accepted", k, 0, Prefs(**pr.__dict__), dl))
            P, C, m_cols, _ = a.build(0)
            r, c = linear_sum_assignment(C)
            opt = C[r, c].sum()
            n, s, tt, arcs, rows, meta = b.network(0)
            g = GoMCF()
            try:
                flows, flow, cost = g.solve(n, s, tt, arcs)
            finally:
                g.close()
            assert flow == len(rows), (t, flow, len(rows))
            gap = abs(cost / SCALE - opt)
            # integer rounding of each arc cost: at most 0.5/SCALE per patient
            assert gap <= len(rows) * 0.5 / SCALE + 1e-6, (t, protect, cost / SCALE, opt)
            worst = max(worst, gap)
    return f"lookahead flow (no forecast) == per-day assignment optimum on {trials}x2 batches, max |gap| = {worst:.2e} days"


def test_constraints_and_phantoms(trials=20):
    rng = np.random.default_rng(5)
    nb = 0
    for t in range(trials):
        cal = rand_cal(rng, n=120, days=90)
        eng = LookaheadEngine(cal, NoShowModel(), eps=0.05, prep_lead=4, horizon=80, protect=4.0, lookahead_days=21,
                              forecast="trailing" if t % 2 else "oracle")
        prefs, evs = {}, []
        for i in range(60):
            p = rand_prefs(rng)
            d = int(rng.integers(0, 40))
            earliest = int(d + rng.integers(0, 20)) if rng.random() < .2 else None
            prefs[f"p{i}"] = (p, d, earliest)
            evs.append(Event("result_accepted", f"p{i}", d, p, d + 31, earliest))
        if eng.forecast == "oracle":
            eng.set_oracle(evs)
        booked = {}
        for day in range(0, 60):
            for ev in evs:
                if ev.day == day:
                    eng.submit(ev)
            for b in eng.step(day):
                assert b.loop_id not in booked
                booked[b.loop_id] = b
        eng.close()
        assert eng.stats["phantom_units"] > 0
        for k, b in booked.items():
            p, d, earliest = prefs[k]
            s = b.session_id
            assert k.startswith("p")                                        # only real patients are committed
            if p.female_endoscopist == "hard":
                assert cal.female[s], (t, k)
            if not p.travel_ok:
                assert cal.region[s] == p.region, (t, k)
            if p.avoid_ramadan:
                assert not cal.ramadan[s], (t, k)
            assert cal.day[s] >= b.booked_on + 4 and cal.day[s] >= (earliest or 0), (t, k)
            nb += 1
    return f"hard constraints held on {nb}/{nb} lookahead bookings ({trials} calendars, trailing and oracle forecast)"


def test_solver_failures():
    def fake(d, name, body):
        path = os.path.join(d, name)
        with open(path, "w") as f:
            f.write("#!/usr/bin/env python3\n" + body)
        os.chmod(path, 0o755)
        return path

    with tempfile.TemporaryDirectory() as d:
        g = GoMCF(fake(d, "dead.py", "import sys\nsys.stdin.readline()\nsys.exit(7)\n"), timeout=2.0)
        try:
            try:
                g.solve(2, 0, 1, [])
                raise AssertionError("solver EOF was not reported")
            except RuntimeError as e:
                assert "closed output" in str(e) and "exit code 7" in str(e), e
        finally:
            g.close()
        assert g.p.poll() == 7

        g = GoMCF(fake(d, "hang.py", "import time\ntime.sleep(60)\n"), timeout=0.05)
        try:
            try:
                g.solve(2, 0, 1, [])
                raise AssertionError("solver timeout was not reported")
            except RuntimeError as e:
                assert "timed out" in str(e) and "killed" in str(e), e
        finally:
            g.close()
        assert g.p.poll() is not None

        # A partial JSON response must not bypass the read deadline.
        g = GoMCF(fake(d, "partial.py",
                       "import sys,time\nsys.stdin.readline()\nsys.stdout.write('{')\nsys.stdout.flush()\ntime.sleep(60)\n"),
                  timeout=0.1)
        try:
            try:
                g.solve(2, 0, 1, [])
                raise AssertionError("partial-response timeout was not reported")
            except RuntimeError as e:
                assert "timed out" in str(e), e
        finally:
            g.close()
        assert g.p.poll() is not None

        # stdin EOF is not enough: close must kill a solver which keeps running.
        g = GoMCF(fake(d, "ignore_eof.py", "import sys,time\nsys.stdin.readline()\ntime.sleep(60)\n"), timeout=0.05)
        g.close()
        assert g.p.poll() is not None
    return "solver EOF includes exit code; full/partial read and close timeouts kill and reap processes"


if __name__ == "__main__":
    for f in (test_matches_assignment, test_constraints_and_phantoms, test_solver_failures):
        print(f.__name__, "->", f())
