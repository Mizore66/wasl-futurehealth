# Network pooling test: same total capacity, but uneven across regions (AD short, AA surplus).
# Compare travel_ok as-assumed (30% willing) vs network pooling where willingness is raised (60%) and vs no travel (0%).
# Run from booking/sim:  python pool_test.py
import sys, os, json, numpy as np
HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE); os.chdir(HERE)
import sim
skew = np.array([0.40, 0.50, 0.10])   # capacity share by region (patients stay 0.55/0.35/0.10)
orig_make_calendar = sim.make_calendar
def make_cal_skew(rng, P, c):
    save = sim.REG_SHARE.copy(); sim.REG_SHARE[:] = skew
    try: return orig_make_calendar(rng, P, c)
    finally: sim.REG_SHARE[:] = save
orig_cohort = sim.make_cohort
def cohort_travel(p):
    def f(rng, P):
        C = orig_cohort(rng, P)
        C["travel"] = np.random.default_rng(7).random(len(C["travel"])) < p
        return C
    return f
out = {}
N = 30
for cap in (1.0, 1.2):
  for label, skewed, tp in (("balanced_30", False, .30), ("skewed_no_travel", True, 0.0), ("skewed_30", True, .30), ("skewed_pool_60", True, .60)):
    sim.make_calendar = make_cal_skew if skewed else orig_make_calendar
    sim.make_cohort = cohort_travel(tp)
    vals = []
    for k in range(N):
        seed = 10_000 + 97*k; rng = np.random.default_rng(seed); P = sim.draw_params(rng)
        C = sim.make_cohort(np.random.default_rng(seed+3), P)
        mp, sp, p0, _ = sim.calibrate_sq(P, C, seed, cap)
        cal = sim.make_calendar(np.random.default_rng(seed+1), P, cap)
        C["_rng"] = np.random.default_rng(seed+2)
        colo, st = sim.simulate("c", P, C, cal, sim.Truth(P, p0), mp, sp)
        m = sim.measures(C, colo, cal, st, P); vals.append((m["d1_31"], m["d3_31"]))
    v = np.array(vals); out[f"{label}@{cap}"] = [round(float(np.median(v[:,0]))*100,1), round(float(np.median(v[:,1]))*100,1)]
    print(label, cap, out[f"{label}@{cap}"], flush=True)
json.dump(out, open(os.path.join(HERE, "pool_test.json"), "w"), indent=1)
