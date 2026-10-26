"""Session ledger: endoscopy lists with FIT+ slots, bookings and overbooking headroom."""
from __future__ import annotations
import numpy as np

REGION_IDX = {"AD": 0, "AA": 1, "DH": 2}


def poibin_tail(ps, cap):
    """P(sum Bernoulli(ps) > cap): exact DP (Poisson-binomial)."""
    dist = np.zeros(len(ps) + 2)
    dist[0] = 1.0
    for q in ps:
        dist[1:] = dist[1:] * (1 - q) + dist[:-1] * q
        dist[0] *= (1 - q)
    return float(dist[cap + 1:].sum())


class Calendar:
    """Columnar session store. Each session = one endoscopy list's FIT+ slots at one unit on one day."""

    def __init__(self, day, unit, region, cap, female, arabic, after_hours, ramadan):
        order = np.lexsort((np.arange(len(day)), day))
        self.day = np.asarray(day)[order]
        self.unit = np.asarray(unit)[order]
        self.region = np.asarray(region)[order]
        self.cap = np.asarray(cap)[order]
        self.female = np.asarray(female, bool)[order]
        self.arabic = np.asarray(arabic, bool)[order]
        self.after = np.asarray(after_hours, bool)[order]
        self.ramadan = np.asarray(ramadan, bool)[order]
        self.n = len(self.day)
        self.booked = [dict() for _ in range(self.n)]  # session -> {pid: engine_predicted_p_absent}
        self.nb = np.zeros(self.n, int)

    def window(self, lo, hi):
        a = np.searchsorted(self.day, lo, "left")
        b = np.searchsorted(self.day, hi, "right")
        return a, b

    def first_free(self, earliest, region, need_female=False, avoid_ramadan=False, span=500):
        a, b = self.window(earliest, earliest + span)
        m = (self.region[a:b] == region) & (self.nb[a:b] < self.cap[a:b])
        if need_female:
            m &= self.female[a:b]
        if avoid_ramadan:
            m &= ~self.ramadan[a:b]
        idx = np.flatnonzero(m)
        return int(a + idx[0]) if len(idx) else -1

    def add(self, s, pid, p_absent=0.0):
        self.booked[s][pid] = p_absent
        self.nb[s] += 1

    def remove(self, s, pid):
        if pid in self.booked[s]:
            del self.booked[s][pid]
            self.nb[s] -= 1

    def set_p(self, s, pid, p):
        if pid in self.booked[s]:
            self.booked[s][pid] = p

    def headroom(self, s, eps, p_new):
        """Max extra bookings k so that P(arrivals > cap) <= eps, new bookings assumed absent w.p. p_new.
        eps=0 -> no overbooking (k = cap - booked)."""
        cap = int(self.cap[s])
        cur = [1 - q for q in self.booked[s].values()]  # show probabilities
        k = max(0, cap - len(cur))
        if eps <= 0:
            return k
        while k < cap:  # never more than double-book
            if poibin_tail(cur + [1 - p_new] * (k + 1), cap) > eps:
                break
            k += 1
        return k
