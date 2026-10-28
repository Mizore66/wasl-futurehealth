"""Capacity-aware booking engine (policy c) and the first-available baseline (policy b).

Method: rolling-horizon batch optimisation. Once per day (or per event; a batch of one) the engine
solves an exact min-cost assignment of pending patients to *slot copies* of endoscopy sessions:

  min  sum_ij c_ij x_ij     s.t.  sum_j x_ij = 1 (incl. one private 'unassigned' column per patient),
                                  sum_i x_ij <= 1,  x in {0,1}

Each session s contributes (cap_s - booked_s) regular copies at cost 0 plus up to h_s overbook copies whose
marginal cost is increasing (kappa * marginal rise in P(arrivals > cap), Poisson-binomial). Because the
overbook cost is separable and convex in the copy index, the slot-copy expansion of this transportation
problem is exact, and the assignment matrix is totally unimodular, so the LP relaxation is integral:
Hungarian/Jonker-Volgenant (scipy.optimize.linear_sum_assignment) returns the ILP optimum for the batch.
tests/test_engine.py checks this against an explicit ILP solved by HiGHS (scipy.optimize.milp).

Optimality is per batch (myopic over future arrivals); no claim of optimality over the whole year.
"""
from __future__ import annotations
from dataclasses import dataclass, field
import math
import numpy as np
from scipy.optimize import linear_sum_assignment

from .model import Prefs, Event, Booking, NoShowModel
from .calendar import Calendar, poibin_tail, REGION_IDX

BIG = 1e7


@dataclass
class Weights:  # hand-set, days-equivalent; NOT tuned on simulated outcomes
    late_per_day: float = 2.0      # each day past the 31-day deadline
    late_step: float = 40.0        # missing the deadline at all -> slack prioritisation
    female_soft: float = 7.0
    language: float = 4.0
    after_hours: float = 4.0
    travel: float = 10.0
    p_no_show: float = 20.0        # expected-no-show penalty (favours short lead / matched sessions)
    kappa_overbook: float = 30.0   # per unit of marginal collision probability
    unassigned: float = 1e5


@dataclass
class _Pending:
    loop_id: str
    prefs: Prefs
    deadline: int
    earliest: int
    attempts: int = 0
    prep_lead: int = 5


class BookingEngine:
    """Event in -> Booking out. submit() queues events; step(day) solves the batch and returns bookings."""

    def __init__(self, cal: Calendar, nsm: NoShowModel | None = None, eps: float = 0.05, horizon: int = 150,
                 prep_lead: int = 5, w: Weights | None = None, nurse_threshold: float = 0.20,
                 max_rebooks: int = 2, max_candidates: int = 40, solver: str = "assignment",
                 reserve_days: int | None = None, protect: float | None = None, protect_window: int = 120):
        self.cal, self.nsm, self.eps, self.H = cal, nsm or NoShowModel(), eps, horizon
        # Female-list reservation (both off by default). Women who require a female endoscopist can only use female
        # lists, so other patients filling those lists early is what pushed them past 31 days.
        #   reserve_days: other patients may book a female list only within reserve_days of today.
        #   protect: demand-sized nesting instead. Each female list holds back ceil(frac * cap) free slots for
        #     women who require one, frac = protect * (recent arrivals of such women in the region per day) /
        #     (female-list slots per day in the region). Held slots are released reserve_days (default 7) out.
        self.reserve_days, self.protect, self.protect_window = reserve_days, protect, protect_window
        if protect is not None and reserve_days is None:
            self.reserve_days = 7
        self._hard_arr: list[tuple[int, str]] = []
        self.prep_lead, self.w, self.nurse_thr = prep_lead, w or Weights(), nurse_threshold
        self.max_rebooks, self.K, self.solver = max_rebooks, max_candidates, solver
        self.pending: dict[str, _Pending] = {}
        self.where: dict[str, int] = {}          # loop -> session
        self.meta: dict[str, _Pending] = {}
        self.escalated: list[str] = []
        self._hcache: dict[int, tuple[int, list[float]]] = {}
        self._obs: list[tuple[float, bool]] = []

    # ---------------- events ----------------
    def submit(self, ev: Event):
        if ev.kind == "result_accepted":
            p = _Pending(ev.loop_id, ev.prefs or Prefs(), ev.deadline if ev.deadline is not None else ev.day + 31,
                         max(ev.day + self.prep_lead, ev.earliest or 0), 0, self.prep_lead)
            self.meta[ev.loop_id] = p
            self.pending[ev.loop_id] = p
            if p.prefs.female_endoscopist == "hard":
                self._hard_arr.append((ev.day, p.prefs.region))
            return
        p = self.meta[ev.loop_id]
        s = self.where.pop(ev.loop_id, None)
        if s is not None:
            self.cal.remove(s, ev.loop_id)
        if ev.kind in ("no_show", "bumped"):
            if ev.kind == "no_show":
                p.attempts += 1
                p.prefs.prior_no_show = True
            if p.attempts > self.max_rebooks:
                self.escalated.append(ev.loop_id)   # hand to a human navigator
                return
            p.earliest = ev.day + p.prep_lead
            self.pending[ev.loop_id] = p
        elif ev.kind == "unconfirmed":
            self.escalated.append(ev.loop_id)
        elif ev.kind == "cancel":           # pre-assessment exclusion or patient refusal: slot released, loop closed
            self.pending.pop(ev.loop_id, None)

    def observe(self, predicted: float, showed_up: bool):
        """Feed back a realised first-attempt outcome; used for online intercept recalibration."""
        self._obs.append((predicted, not showed_up))
        if len(self._obs) > 400:
            self._obs = self._obs[-400:]

    def recalibrate(self, prior_n: int = 30):
        """Shift the no-show logit intercept so mean prediction matches the observed rate (Beta-shrunk to prior)."""
        if len(self._obs) < 20:
            return
        pr = np.array([o[0] for o in self._obs]); y = np.array([o[1] for o in self._obs], float)
        target = (y.sum() + prior_n * pr.mean()) / (len(y) + prior_n)
        lp = np.log(pr / (1 - pr))
        lo, hi = -4.0, 4.0
        for _ in range(40):
            mid = (lo + hi) / 2
            if (1 / (1 + np.exp(-(lp + mid)))).mean() < target:
                lo = mid
            else:
                hi = mid
        z = math.log(self.nsm.p0 / (1 - self.nsm.p0)) + (lo + hi) / 2
        self.nsm.p0 = float(min(0.9, max(0.02, 1 / (1 + math.exp(-z)))))
        self._obs = []
        self._hcache.clear()

    def handle(self, ev: Event) -> list[Booking]:
        """Single-event entry point for the Wasl pipeline: event in, booking(s) out."""
        self.submit(ev)
        return self.step(ev.day)

    # ---------------- solver ----------------
    def _pred(self, pr: Prefs, lead, fem, ara, aft, ram, nurse, reg=None):
        mm_f = (pr.female_endoscopist == "soft") & ~fem
        trav = 0 if reg is None else (reg != pr.region)
        return self.nsm.p(lead, int(pr.prior_no_show), mm_f, pr.arabic_only & ~ara, pr.after_hours & ~aft,
                          1, nurse, pr.avoid_ramadan & ram, trav)

    def _overbook_costs(self, s, p_new, max_extra):
        """Marginal collision probabilities for successive overbook copies of session s, capped by eps."""
        cal = self.cal
        key = (int(cal.nb[s]), round(p_new, 2), max_extra)
        hit = self._hcache.get(s)
        if hit is not None and hit[0] == key:
            return hit[1]
        cap = int(cal.cap[s])
        shows = [1 - q for q in cal.booked[s].values()]
        free = max(0, cap - len(shows))
        shows = shows + [1 - p_new] * free
        prev = poibin_tail(shows, cap)
        out = []
        for _ in range(min(max_extra, cap)):
            shows = shows + [1 - p_new]
            t = poibin_tail(shows, cap)
            if t > self.eps:
                break
            out.append(t - prev)
            prev = t
        self._hcache[s] = (key, out)
        return out

    def build(self, day: int):
        """Returns (patients, cost matrix C [n x (m_cols+n)], m_cols, column->session) for the current batch."""
        cal, w = self.cal, self.w
        pids = sorted(self.pending, key=lambda k: (self.pending[k].deadline, k))
        P = [self.pending[k] for k in pids]
        lo = min(max(day + p.prep_lead, p.earliest) for p in P)
        a, b = cal.window(lo, day + self.H)
        S = np.arange(a, b)
        if len(S) == 0:
            return P, None, 0, None
        # candidate sessions per patient (hard constraints), limited to the first K feasible with room
        cols_s, cols_cost = [], []          # column -> session, column overbook cost
        col_index: dict[int, list[int]] = {}
        cand = []
        for p in P:
            pr = p.prefs
            m = (cal.day[S] >= max(day + p.prep_lead, p.earliest))
            reg = (cal.region[S] == pr.region) | (pr.travel_ok)
            m &= reg
            if pr.female_endoscopist == "hard":
                m &= cal.female[S]
            elif self.reserve_days is not None and self.protect is None:
                m &= ~cal.female[S] | (cal.day[S] <= day + self.reserve_days)
            if pr.avoid_ramadan:
                m &= ~cal.ramadan[S]
            idx = S[m]
            # keep sessions up to the K-th one that has free capacity (full ones before it are overbook candidates)
            free = cal.nb[idx] < cal.cap[idx]
            cut = np.flatnonzero(free)
            if len(cut) > self.K:
                idx = idx[: cut[self.K - 1] + 1]
            cand.append(idx)
        p_mean = 0.0
        for p in P:
            p_mean += self.nsm.p(14, int(p.prefs.prior_no_show), 0, 0, 0, 1, 0, False).item()
        p_mean /= len(P)
        sess_needed = np.unique(np.concatenate(cand)) if cand else np.array([], int)
        n = len(P)
        held = self._held(day, sess_needed) if self.protect is not None else {}
        cols_held = []                      # column is a held female-list slot (women who require one only)
        for s in sess_needed:
            s = int(s)
            free = max(0, int(cal.cap[s] - cal.nb[s]))
            h = held.get(s, 0)
            js = []
            for q in range(min(free, n)):
                js.append(len(cols_s)); cols_s.append(s); cols_cost.append(0.0); cols_held.append(q >= free - h)
            if self.eps > 0 and free < n and cal.day[s] <= day + 45:
                mcs = self._overbook_costs(s, p_mean, n - min(free, n))
                # enforce non-decreasing marginal cost (convexity) so the slot-copy expansion stays exact
                for mc in (np.maximum.accumulate(mcs) if mcs else []):
                    js.append(len(cols_s)); cols_s.append(s); cols_cost.append(w.kappa_overbook * mc * 100)
                    cols_held.append(h > 0)   # overbooking a list that is holding slots would use them up
            col_index[s] = js
        m_cols = len(cols_s)
        C = np.full((n, m_cols + n), BIG)
        for i in range(n):
            C[i, m_cols + i] = w.unassigned
        cols_s_arr = np.array(cols_s, int)
        cols_cost_arr = np.array(cols_cost)
        cols_held_arr = np.array(cols_held, bool)
        for i, (p, idx) in enumerate(zip(P, cand)):
            if len(idx) == 0:
                continue
            pr = p.prefs
            cols = np.concatenate([col_index[int(s)] for s in idx]) if len(idx) else np.array([], int)
            cols = cols.astype(int)
            if len(cols) == 0:
                continue
            ss = cols_s_arr[cols]
            d = cal.day[ss]
            fem, ara, aft, ram = cal.female[ss], cal.arabic[ss], cal.after[ss], cal.ramadan[ss]
            rg = cal.region[ss]
            pred = self._pred(pr, d - day, fem, ara, aft, ram, 0, rg)
            nurse = pred > self.nurse_thr
            pred = np.where(nurse, self._pred(pr, d - day, fem, ara, aft, ram, 1, rg), pred)
            late = np.maximum(0, d - p.deadline)
            cost = ((d - day) + w.late_per_day * late + w.late_step * (late > 0)
                    + w.female_soft * ((pr.female_endoscopist == "soft") & ~fem)
                    + w.language * (pr.arabic_only & ~ara) + w.after_hours * (pr.after_hours & ~aft)
                    + w.travel * (cal.region[ss] != pr.region) + w.p_no_show * pred + cols_cost_arr[cols])
            if pr.female_endoscopist != "hard" and held:
                cost = np.where(cols_held_arr[cols], BIG, cost)
            C[i, cols] = cost
        return P, C, m_cols, cols_s_arr

    def _held(self, day, sess):
        """Free slots each female list in `sess` holds back for women who require a female endoscopist."""
        cal, out = self.cal, {}
        lo = day - self.protect_window
        self._hard_arr = [x for x in self._hard_arr if x[0] >= lo]
        a, b = cal.window(day, day + self.H)
        for reg in np.unique(cal.region[sess]) if len(sess) else []:
            rate = sum(1 for _, r in self._hard_arr if r == reg) / self.protect_window
            fm = cal.female[a:b] & (cal.region[a:b] == reg)
            supply = cal.cap[a:b][fm].sum() / max(1, self.H)
            frac = 1.0 if supply == 0 else min(1.0, self.protect * rate / supply)
            for s in sess:
                s = int(s)
                if cal.female[s] and cal.region[s] == reg and cal.day[s] > day + self.reserve_days:
                    out[s] = int(math.ceil(frac * cal.cap[s]))
        return out

    def step(self, day: int) -> list[Booking]:
        if not self.pending:
            return []
        cal = self.cal
        P, C, m_cols, cols_s_arr = self.build(day)
        if C is None:
            return []
        if self.solver == "greedy":   # ablation: earliest-deadline-first, each patient takes its cheapest free column
            used, r, c = set(), [], []
            for i in range(C.shape[0]):
                order = np.argsort(C[i])
                for j in order:
                    if j not in used:
                        used.add(int(j)); r.append(i); c.append(int(j)); break
        else:
            r, c = linear_sum_assignment(C)
        out = []
        for i, j in zip(r, c):
            if j >= m_cols or C[i, j] >= BIG:
                continue
            p, s = P[i], int(cols_s_arr[j])
            pr = p.prefs
            fem, ara, aft, ram = cal.female[s], cal.arabic[s], cal.after[s], cal.ramadan[s]
            rg = cal.region[s]
            pred = float(self._pred(pr, cal.day[s] - day, fem, ara, aft, ram, 0, rg))
            nurse = pred > self.nurse_thr
            if nurse:
                pred = float(self._pred(pr, cal.day[s] - day, fem, ara, aft, ram, 1, rg))
            over = cal.nb[s] >= cal.cap[s]
            cal.add(s, p.loop_id, pred)
            self.where[p.loop_id] = s
            del self.pending[p.loop_id]
            out.append(Booking(p.loop_id, s, str(cal.unit[s]), int(cal.day[s]), bool(fem), bool(over), pred,
                               bool(nurse), int(cal.day[s]) - p.prep_lead, p.deadline,
                               bool(cal.day[s] <= p.deadline), day))
        return out


class FirstAvailable:
    """Policy (b): automated GI referral, first free slot at the home region, no preferences, no overbooking."""

    def __init__(self, cal: Calendar, prep_lead: int = 5):
        self.cal, self.prep_lead = cal, prep_lead

    def book(self, loop_id, region, earliest, need_female=False):
        s = self.cal.first_free(earliest, region, need_female)
        if s >= 0:
            self.cal.add(s, loop_id, 0.0)
        return s
