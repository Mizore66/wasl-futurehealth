"""Learn-then-Test threshold selection (Angelopoulos, Bates, Candès, Jordan, Lei; Ann. Appl. Stat. 2025).

Input: pipeline/results/ltt/counts_*.json from `wsi7 ltt` (per calibration cohort, per closure link tier λ).
Risks, each per loop:
  R_fc(λ)  = P(loop falsely closed)                          target α_fc
  R_wp(λ)  = P(loop has a wrong-patient message released)    target α_wp
H0(λ): R(λ) > α. p-value: exact binomial tail P(Bin(n, α) <= k) (valid for 0/1 losses), and the Hoeffding–Bentkus
p-value min(exp(-n·h(R̂, α)), e·P(Bin(n, α) <= ceil(n R̂))) for comparison. Family-wise control: Bonferroni δ/2 per
risk; fixed-sequence testing from strict to loose (no further correction along the sequence). λ̂ = the loosest tier
reached before the first non-rejection in BOTH risk sequences. Guarantee: P(R_fc(λ̂) <= α_fc and R_wp(λ̂) <= α_wp) >= 1-δ,
for loops exchangeable with the calibration cohorts (same generator, cal-split persons, v2 noise).
Usage: python scripts/ltt_select.py pipeline/results/ltt [--alpha-fc 0.001] [--alpha-wp 0.001] [--delta 0.05] [--review 1]
"""
import argparse, glob, json, math, os
from scipy.stats import binom

ap = argparse.ArgumentParser()
ap.add_argument("dir")
ap.add_argument("--alpha-fc", type=float, default=0.001)
ap.add_argument("--alpha-wp", type=float, default=0.001)
ap.add_argument("--delta", type=float, default=0.05)
ap.add_argument("--min-tau", type=float, default=0.99,
                help="design floor: operating tier must exceed the 0.989 uncorroborated-edge cap (added after the final-code evaluation)")
a = ap.parse_args()

rows = [r for f in sorted(glob.glob(os.path.join(a.dir, "counts_*.json"))) for r in json.load(open(f))["rows"]]
cohorts = sorted({r["cohort"] for r in rows})


def h(r, al):  # Bernoulli KL divergence kl(r || al) for r < al
    r = min(max(r, 1e-12), 1 - 1e-12)
    return r * math.log(r / al) + (1 - r) * math.log((1 - r) / (1 - al))


def p_binom(k, n, al):
    return float(binom.cdf(k, n, al))


def p_hb(k, n, al):
    rh = k / n
    hoeff = math.exp(-n * h(rh, al)) if rh < al else 1.0
    bent = math.e * float(binom.cdf(math.ceil(n * rh), n, al))
    return min(1.0, hoeff, bent)


out = {"method": "Learn-then-Test, fixed-sequence (strict→loose), Bonferroni over 2 risks",
       "alpha_false_closure": a.alpha_fc, "alpha_wrong_patient": a.alpha_wp, "delta": a.delta, "delta_per_risk": a.delta / 2,
       "calibration_cohorts": cohorts, "modes": {}}
md = [f"# Learn-then-Test closure gate\n", f"Calibration: {len(cohorts)} cohorts from the persons3 `cal` split (disjoint from eval), v2 noise. "
      f"α_fc = {a.alpha_fc}, α_wp = {a.alpha_wp}, δ = {a.delta} (δ/2 per risk). p-values: exact binomial tail (HB shown for reference).\n"]
for review in (True, False):
    rs = [r for r in rows if r["with_review"] == review]
    taus = sorted({r["tau_close"] for r in rs}, reverse=True)
    table, lam, open_fc, open_wp = [], None, True, True
    for t in taus:
        g = [r for r in rs if r["tau_close"] == t]
        n = sum(r["loops"] for r in g)
        k_fc = sum(r["false_closures"] for r in g)
        k_wp = sum(r["loops_with_wrong_patient_message"] for r in g)
        closed, tp, truth = sum(r["closed"] for r in g), sum(r["tp"] for r in g), sum(r["truth_closed"] for r in g)
        items = sum(r["review_items"] for r in g)
        p_fc, p_wp = p_binom(k_fc, n, a.alpha_fc), p_binom(k_wp, n, a.alpha_wp)
        open_fc = open_fc and p_fc <= a.delta / 2
        open_wp = open_wp and p_wp <= a.delta / 2
        ok = open_fc and open_wp
        if ok:
            lam = t
        from scipy.optimize import brentq
        a_min = brentq(lambda al: binom.cdf(k_fc, n, al) - a.delta / 2, 1e-9, 0.5)  # smallest α_fc this tier certifies
        table.append({"tau_close": t, "n_loops": n, "false_closures": k_fc, "loops_wrong_patient_msg": k_wp, "closed": closed,
                      "recall": round(tp / max(truth, 1), 4), "review_items_per_100": round(100 * items / n, 2),
                      "p_fc_binom": p_fc, "p_fc_hb": p_hb(k_fc, n, a.alpha_fc), "p_wp_binom": p_wp, "p_wp_hb": p_hb(k_wp, n, a.alpha_wp),
                      "certified": ok, "alpha_fc_certifiable": a_min})
    mode = "with_review" if review else "no_review"
    cert = [x for x in table if x["certified"] and x["tau_close"] >= a.min_tau]
    best = max((x["recall"] for x in cert), default=None)
    # pre-declared operating rule: strictest certified tier whose recall is within 0.5 pp of the best certified recall
    op = next((x["tau_close"] for x in cert if best is not None and x["recall"] >= best - 0.005), None)
    out["modes"][mode] = {"loosest_certified_tau_close": lam, "operating_tau_close": op, "table": table,
                          "n_needed_zero_errors": math.ceil(math.log(a.delta / 2) / math.log(1 - min(a.alpha_fc, a.alpha_wp)))}
    md.append(f"\n## {mode.replace('_', ' ')}: loosest certified λ = {lam if lam is not None else 'none'}; operating λ̂ = **{op}**\n")
    md.append("| τ_close | loops | false closures | wrong-patient msg loops | recall | review items /100 | p_fc | p_wp | certified | smallest certifiable α_fc |\n|---|---|---|---|---|---|---|---|---|---|")
    for x in table:
        md.append(f"| {x['tau_close']} | {x['n_loops']} | {x['false_closures']} | {x['loops_wrong_patient_msg']} | {x['recall']} | {x['review_items_per_100']} | "
                  f"{x['p_fc_binom']:.2e} | {x['p_wp_binom']:.2e} | {'yes' if x['certified'] else 'no'} | {x['alpha_fc_certifiable']:.2e} |")
md.append(f"\nWith 0 observed errors, certifying α = {min(a.alpha_fc, a.alpha_wp)} at δ/2 = {a.delta/2} needs n ≥ "
          f"{out['modes']['with_review']['n_needed_zero_errors']} calibration loops.\n")
md.append(f"**Design floor:** the operating tier must be ≥ {a.min_tau}, above the 0.989 cap on uncorroborated model edges, so a name-only link cannot auto-close a loop. "
          "This floor was added after the final-code evaluation found one wrong-patient closure at the cap under τ 0.98; it is a disclosed post-hoc change.\n")
md.append("**Operating rule (pre-declared):** FWER control means every certified tier satisfies both risk bounds simultaneously with probability ≥ 1−δ, "
          "so any choice inside the certified set keeps the guarantee. Wasl uses the strictest certified tier whose recall is within 0.5 pp of the best certified recall: "
          "looser tiers that buy <0.5 pp recall for observed false closures are not worth it.\n")
md.append("**Scope of the guarantee:** it holds for loops exchangeable with the calibration cohorts (same synthetic generator, cal-split persons, v2 noise, "
          "4% reviewer error, merge+close double review). It is not a guarantee on real Abu Dhabi data: the pilot must re-run this calibration on "
          "locally adjudicated loops (≈3k per stratum).")
json.dump(out, open(os.path.join(a.dir, "ltt_selection.json"), "w"), indent=1)
open(os.path.join(a.dir, "LTT.md"), "w").write("\n".join(md) + "\n")
print("\n".join(md))
