# PROJECTION, not a measurement. Simulated before/after for FIT+ -> colonoscopy in Abu Dhabi: status quo vs the full Wasl workflow,
# using published effect sizes only. Uncertainty = parameter uncertainty (Monte Carlo over the published CIs + baseline shape).
#
# Status quo (SEHA AHS, Emirati 40-75, 2015-16; BMC Cancer 2021, PMC8343888; cited in research/phase2-pophealth-a.md):
#   1,429 FIT+; 436 (30.5%) had follow-up colonoscopy; median wait from positive result to colonoscopy 72 days.
#   Modelled as a cure mixture: F0(t) = pi0 * LogNormalCDF(t; median m0, sigma); pi0 ~ Beta(436, 993); m0 ~ 72 * LN(0, 0.10);
#   sigma ~ U(0.6, 1.0) (shape not reported; the 31-day share is sensitive to it, so it is sampled, not fixed).
# Effect sizes (risk ratios for follow-up colonoscopy completion), mapped to Wasl components:
#   patient reminders  -> RR 1.32 (95% CI 1.04-1.67)  Chua et al., JAMA Intern Med 2026 (doi 10.1001/jamainternmed.2026.4827)
#        Wasl: approved EN/AR notification + reminder + teach-back with clarification
#   automated GI referral -> RR 1.68 (1.09-2.60)      same meta-analysis
#        Wasl: referral routed to the receiving unit + clinician e-consult on rejection/no answer (re-referral/redirect)
#   outreach + navigation -> 31.4% vs 22.9% at 120 d, adj. diff 8.5 pts (4.8-12.0)  mFOCUS, Atlas et al., JAMA 2023
#        Wasl: named owner per loop, SLA escalations, nurse callback on failed teach-back
# Combination: components overlap, so effects are NOT multiplied. log RR_full = log RR_max + rho * sum(log RR_others),
#   rho ~ U(0, 0.5) (overlap discount; rho=0 means only the strongest component counts). Completion capped at 0.95.
# Timing: proportional hazards on the population survival: S1(t) = S0(t)^HR, HR solved so that F1(120)/F0(120) = RR
#   (mFOCUS and most trials measure completion at ~120 days). This yields time-to-colonoscopy and the 31-day share.
import numpy as np, json, os
from math import log
from scipy.stats import norm
from scipy.optimize import brentq

D = os.path.dirname(os.path.abspath(__file__))
rng = np.random.default_rng(2026)
N = 10000
T = np.arange(0, 366, 0.5)

def lnrr(est, lo, hi, n):
    se = (log(hi) - log(lo)) / 3.92
    return rng.normal(log(est), se, n)

pi0 = rng.beta(436, 993, N)
m0 = 72 * np.exp(rng.normal(0, 0.10, N))
sig = rng.uniform(0.6, 1.0, N)
L_rem = lnrr(1.32, 1.04, 1.67, N)
L_ref = lnrr(1.68, 1.09, 2.60, N)
# mFOCUS: RR from adjusted absolute difference on the 22.9% usual-care base
d_nav = rng.normal(8.5, (12.0 - 4.8) / 3.92, N)
L_nav = np.log((22.9 + d_nav) / 22.9)
rho = rng.uniform(0, 0.5, N)

def combine(Ls):
    M = np.stack(Ls)
    mx = M.max(0)
    return mx + rho * (M.sum(0) - mx)

scen = {
    "navigation_only (mFOCUS)": L_nav,
    "reminders+teachback+navigation": combine([L_rem, L_nav]),
    "full Wasl (reminders+e-consult/auto-referral+navigation)": combine([L_rem, L_ref, L_nav]),
}

def F0(t, i):
    return pi0[i] * norm.cdf((np.log(np.maximum(t, 1e-9)) - np.log(m0[i])) / sig[i])

def summarise(F, label):
    f31, f120 = F(31.0), F(120.0)
    curve = F(T)
    # median days among those completing by day 120
    half = f120 / 2
    med = T[np.searchsorted(curve, half)]
    # mean days to colonoscopy among completers by 120 d (restricted)
    rmst = float(np.trapezoid(1 - curve[T <= 120], T[T <= 120]))  # mean colonoscopy-free days over 0-120 (population, incl. never)
    return f31, f120, (med, rmst)

res = {k: {"f31": [], "f120": [], "med": [], "rmst": [], "rr120": []} for k in ["status_quo"] + list(scen)}
for i in range(N):
    f0 = lambda t, i=i: F0(t, i)
    a31, a120, amed = summarise(f0, "sq")
    for k, v in (("f31", a31), ("f120", a120), ("med", amed[0]), ("rmst", amed[1]), ("rr120", 1.0)):
        res["status_quo"][k].append(float(v))
    for name, L in scen.items():
        target = min(np.exp(L[i]) * a120, 0.95)
        rr = target / a120
        S120 = 1 - a120
        hr = log(1 - target) / log(S120)
        f1 = lambda t, hr=hr, i=i: 1 - (1 - F0(t, i)) ** hr
        b31, b120, bmed = summarise(f1, name)
        for k, v in (("f31", b31), ("f120", b120), ("med", bmed[0]), ("rmst", bmed[1]), ("rr120", rr)):
            res[name][k].append(float(v))

def ci(x, scale=1.0, nd=1):
    x = np.array(x) * scale
    return [round(float(np.median(x)), nd), round(float(np.percentile(x, 2.5)), nd), round(float(np.percentile(x, 97.5)), nd)]

out = {"note": "PROJECTION from published effect sizes, not an observed outcome. Median [95% interval] over 10,000 parameter draws.",
       "per_1000_FIT_positive": {}}
sq31, sq120 = np.array(res["status_quo"]["f31"]), np.array(res["status_quo"]["f120"])
for k, r in res.items():
    f31, f120 = np.array(r["f31"]), np.array(r["f120"])
    out["per_1000_FIT_positive"][k] = {
        "colonoscopy_within_31d_pct": ci(f31, 100),
        "colonoscopy_by_120d_pct": ci(f120, 100),
        "median_days_to_colonoscopy_among_completers_by_120d": ci(r["med"], 1, 0),
        "unresolved_at_120d_per_1000": ci(1 - f120, 1000, 0),
        "rmst_colonoscopy_free_days_0_120": ci(r["rmst"], 1, 1),
        "rmst_days_saved_vs_status_quo": ci(np.array(res["status_quo"]["rmst"]) - np.array(r["rmst"]), 1, 1),
        "additional_within_31d_per_1000_vs_status_quo": ci(f31 - sq31, 1000, 0),
        "fewer_unresolved_at_120d_per_1000_vs_status_quo": ci(f120 - sq120, 1000, 0),
        "effective_rr_at_120d": ci(r["rr120"], 1, 2),
    }
# sensitivity: which assumption moves the full-workflow 31-day share most (Spearman rank correlation)
from scipy.stats import spearmanr
full = "full Wasl (reminders+e-consult/auto-referral+navigation)"
f31 = np.array(res[full]["f31"])
out["sensitivity_spearman_full_31d"] = {k: round(float(spearmanr(v, f31)[0]), 2) for k, v in
    {"pi0_baseline": pi0, "m0_median_72d": m0, "sigma_shape": sig, "rr_reminders": L_rem, "rr_auto_referral": L_ref, "rr_navigation": L_nav, "rho_overlap": rho}.items()}
json.dump(out, open(os.path.join(D, "beforeafter.json"), "w"), indent=1)
print(json.dumps(out, indent=1))
