"""Multi-period lookahead booking (SIMULATED decision support; off by default).

The default engine (engine.BookingEngine, "myopic") books each day's pending patients optimally *for that day*: today's
patients take the earliest suitable slots even when a patient expected tomorrow (an urgent window, a woman who requires
a female endoscopist, a patient who cannot travel) will need exactly those slots. In our tests it ties greedy.

LookaheadEngine is a rolling-horizon optimiser. Every booking day it solves ONE min-cost flow over

    rows    = today's real pending patients (supply 1)
            + forecast future arrivals, aggregated by class (region, requires a female list, travel ok, earliest offset,
              deadline offset) and by future 7-day block (supply = forecast count; "phantom" demand)
    columns = the real calendar's sessions over the booking horizon (free slots, held female-list slots, convex
              overbook copies), shared by real and phantom rows
    unassigned arc per row (same penalty as the default engine)

with the default engine's cost per (row, session) (wait days, late penalties, soft-preference, travel and no-show
terms) and the same hard constraints (female list, region/travel, Ramadan, prep lead, female-list protection).
Phantom rows' costs are scaled by gamma < 1 (a forecast is uncertain; it also breaks ties in favour of real
patients). Only the real rows' assignments are committed; phantoms are discarded and re-forecast the next day.
Committed suggestions are never moved (a clinician confirms them).

Forecast: "trailing" (deployable) = per-class arrival rate over the last `window` days; "oracle" (NOT deployable,
upper bound) = the true future arrival counts per class and day, without identities.

The flow is solved by `wsi7 mcf` (pipeline/mcf.go, pure Go successive shortest paths; exact). With zero phantom rows
the problem is exactly the default engine's batch assignment (tests/test_lookahead.py checks equal optimum cost).
"""
from __future__ import annotations
import json
import math
import os
import subprocess
import collections
import numpy as np

from .model import Prefs, Event
from .engine import BookingEngine, _Pending

SCALE = 1000          # costs are sent to the Go solver as integers (milli-days)


def _repo_root():
    return os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


class GoMCF:
    """One long-lived `wsi7 mcf` process: JSON line in, JSON line out."""

    def __init__(self, binary: str | None = None):
        b = binary or os.environ.get("WASL_MCF_BIN") or os.path.join(_repo_root(), "bin", "wsi7")
        if not os.path.exists(b):
            raise FileNotFoundError(f"lookahead needs the Go solver {b}: run scripts/demo.sh build "
                                    "(or set WASL_MCF_BIN to a built wsi7)")
        self.p = subprocess.Popen([b, "mcf"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True, bufsize=1)

    def solve(self, n: int, s: int, t: int, arcs: list[tuple[int, int, int, int]]):
        self.p.stdin.write(json.dumps({"n": n, "s": s, "t": t, "arcs": arcs}, separators=(",", ":")) + "\n")
        self.p.stdin.flush()
        r = json.loads(self.p.stdout.readline())
        if r.get("error"):
            raise RuntimeError("mcf: " + r["error"])
        return r["flows"], r["flow"], r["cost"]

    def close(self):
        if self.p.poll() is None:
            self.p.stdin.close()
            self.p.wait(timeout=10)


class _Row:
    __slots__ = ("prefs", "earliest", "deadline", "ref", "supply", "scale", "pend")

    def __init__(self, prefs, earliest, deadline, ref, supply, scale, pend=None):
        self.prefs, self.earliest, self.deadline, self.ref = prefs, earliest, deadline, ref
        self.supply, self.scale, self.pend = supply, scale, pend


def class_key(prefs: Prefs, day: int, earliest: int, deadline: int) -> tuple:
    return (prefs.region, prefs.female_endoscopist == "hard", bool(prefs.travel_ok), earliest - day, deadline - day)


class LookaheadEngine(BookingEngine):
    def __init__(self, *args, lookahead_days: int = 28, block: int = 7, gamma: float = 0.8, window: int = 28,
                 forecast: str = "trailing", solver_bin: str | None = None, **kw):
        super().__init__(*args, **kw)
        self.L, self.block, self.gamma, self.window, self.forecast = lookahead_days, block, gamma, window, forecast
        self._arr: list[tuple[int, tuple]] = []          # (day, class) of every first submission
        self._future: dict[int, collections.Counter] = {}  # oracle: day -> Counter(class)
        self._solver_bin = solver_bin
        self._mcf: GoMCF | None = None
        self.stats = collections.Counter()

    # ---------------- forecast ----------------
    def submit(self, ev: Event):
        super().submit(ev)
        if ev.kind == "result_accepted":
            p = self.meta[ev.loop_id]
            self._arr.append((ev.day, class_key(p.prefs, ev.day, p.earliest, p.deadline)))

    def set_oracle(self, events: list[Event]):
        """Upper bound only: give the engine the true future arrival counts per class and day (no identities)."""
        self._future = collections.defaultdict(collections.Counter)
        for ev in events:
            earliest = max(ev.day + self.prep_lead, ev.earliest or 0)
            dl = ev.deadline if ev.deadline is not None else ev.day + 31
            self._future[ev.day][class_key(ev.prefs or Prefs(), ev.day, earliest, dl)] += 1

    def _phantoms(self, day: int) -> list[_Row]:
        rows = []
        if self.L <= 0:
            return rows
        if self.forecast == "oracle":
            for a in range(day + 1, day + 1 + self.L):
                for key, cnt in sorted(self._future.get(a, {}).items()):
                    rows.append(self._phantom_row(key, a, cnt))
            return rows
        if not self._arr:
            return rows
        first = self._arr[0][0]
        span = max(self.block, min(self.window, day - first + 1))
        cnt = collections.Counter(k for d, k in self._arr if day - span < d <= day)
        for b in range(self.L // self.block):
            a = day + 1 + b * self.block + self.block // 2
            for key, c in sorted(cnt.items()):
                q = int(round(c / span * self.block))
                if q > 0:
                    rows.append(self._phantom_row(key, a, q))
        return rows

    def _phantom_row(self, key, a, supply):
        region, hard, travel, lo, hi = key
        pr = Prefs(female_endoscopist="hard" if hard else "none", region=region, travel_ok=travel)
        return _Row(pr, a + lo, a + hi, a, supply, self.gamma)

    # ---------------- solver ----------------
    def _cands(self, S, row):
        cal, pr = self.cal, row.prefs
        m = (cal.day[S] >= row.earliest)
        m &= (cal.region[S] == pr.region) | (pr.travel_ok)
        if pr.female_endoscopist == "hard":
            m &= cal.female[S]
        elif self.reserve_days is not None and self.protect is None:
            m &= ~cal.female[S] | (cal.day[S] <= self._today + self.reserve_days)
        if pr.avoid_ramadan:
            m &= ~cal.ramadan[S]
        idx = S[m]
        free = cal.nb[idx] < cal.cap[idx]
        cut = np.flatnonzero(free)
        if len(cut) > self.K:
            idx = idx[: cut[self.K - 1] + 1]
        return idx

    def _costs(self, row, ss):
        cal, w, pr = self.cal, self.w, row.prefs
        d = cal.day[ss]
        fem, ara, aft, ram, rg = cal.female[ss], cal.arabic[ss], cal.after[ss], cal.ramadan[ss], cal.region[ss]
        pred = self._pred(pr, d - row.ref, fem, ara, aft, ram, 0, rg)
        nurse = pred > self.nurse_thr
        pred = np.where(nurse, self._pred(pr, d - row.ref, fem, ara, aft, ram, 1, rg), pred)
        late = np.maximum(0, d - row.deadline)
        return ((d - row.ref) + w.late_per_day * late + w.late_step * (late > 0)
                + w.female_soft * ((pr.female_endoscopist == "soft") & ~fem)
                + w.language * (pr.arabic_only & ~ara) + w.after_hours * (pr.after_hours & ~aft)
                + w.travel * (rg != pr.region) + w.p_no_show * pred)

    def network(self, day: int):
        """Build the flow network for `day`. Returns (n, s, t, arcs, rows, arc_meta)."""
        cal, w = self.cal, self.w
        self._today = day
        pids = sorted(self.pending, key=lambda k: (self.pending[k].deadline, k))
        real = [_Row(self.pending[k].prefs, max(day + self.pending[k].prep_lead, self.pending[k].earliest),
                     self.pending[k].deadline, day, 1, 1.0, self.pending[k]) for k in pids]
        rows = real + self._phantoms(day)
        lo = min(r.earliest for r in rows)
        a, b = cal.window(lo, day + self.H)
        S = np.arange(a, b)
        cand = [self._cands(S, r) for r in rows]
        N = sum(r.supply for r in rows)
        p_mean = 0.0
        for r in real:
            p_mean += self.nsm.p(14, int(r.prefs.prior_no_show), 0, 0, 0, 1, 0, False).item()
        p_mean /= len(real)
        sess = np.unique(np.concatenate(cand)) if cand else np.array([], int)
        held = self._held(day, sess) if self.protect is not None else {}
        s_node, t_node, nxt = 0, 1, 2 + len(rows)
        arcs, meta = [], []                               # meta: per arc (row index, session) for row->session arcs
        node_open, node_held, node_ob = {}, {}, {}
        for s in sess:
            s = int(s)
            free = max(0, int(cal.cap[s] - cal.nb[s]))
            h = min(held.get(s, 0), free)
            if free - h > 0:
                node_open[s] = nxt; arcs.append((nxt, t_node, free - h, 0)); meta.append(None); nxt += 1
            if h > 0:
                node_held[s] = nxt; arcs.append((nxt, t_node, h, 0)); meta.append(None); nxt += 1
            if self.eps > 0 and free < N and cal.day[s] <= day + 45:
                mcs = self._overbook_costs(s, p_mean, N - min(free, N))
                if mcs:
                    node_ob[s] = (nxt, held.get(s, 0) > 0)
                    for mc in np.maximum.accumulate(mcs):
                        arcs.append((nxt, t_node, 1, int(round(w.kappa_overbook * mc * 100 * SCALE)))); meta.append(None)
                    nxt += 1
        for i, (r, idx) in enumerate(zip(rows, cand)):
            rn = 2 + i
            arcs.append((s_node, rn, r.supply, 0)); meta.append(None)
            arcs.append((rn, t_node, r.supply, int(round(w.unassigned * r.scale * SCALE)))); meta.append(None)
            if len(idx) == 0:
                continue
            hard = r.prefs.female_endoscopist == "hard"
            cost = self._costs(r, idx) * r.scale
            for s, c in zip(idx.tolist(), cost.tolist()):
                ci = int(round(c * SCALE))
                if s in node_open:
                    arcs.append((rn, node_open[s], r.supply, ci)); meta.append((i, s))
                if hard and s in node_held:
                    arcs.append((rn, node_held[s], r.supply, ci)); meta.append((i, s))
                if s in node_ob and (hard or not node_ob[s][1]):
                    arcs.append((rn, node_ob[s][0], r.supply, ci)); meta.append((i, s))
        return nxt, s_node, t_node, arcs, rows, meta

    def step(self, day: int):
        if not self.pending:
            return []
        n, s, t, arcs, rows, meta = self.network(day)
        if self._mcf is None:
            self._mcf = GoMCF(self._solver_bin)
        flows, _, _ = self._mcf.solve(n, s, t, arcs)
        assign = {}
        for f, m in zip(flows, meta):
            if f and m is not None and rows[m[0]].pend is not None:
                assign[m[0]] = m[1]
        self.stats["steps"] += 1
        self.stats["phantom_rows"] += sum(1 for r in rows if r.pend is None)
        self.stats["phantom_units"] += sum(r.supply for r in rows if r.pend is None)
        out = []
        for i, r in enumerate(rows):          # real rows come first, in (deadline, id) order as in the default engine
            if r.pend is None:
                break
            if i in assign:
                out.append(self._commit(day, r.pend, assign[i]))
        return out

    def close(self):
        if self._mcf is not None:
            self._mcf.close()
            self._mcf = None
