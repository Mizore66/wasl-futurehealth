# Learn-then-Test closure gate

Calibration: 6 cohorts from the persons3 `cal` split (disjoint from eval), v2 noise. α_fc = 0.001, α_wp = 0.001, δ = 0.05 (δ/2 per risk). p-values: exact binomial tail (HB shown for reference).


## with review: loosest certified λ = 0.7; operating λ̂ = **0.98**

| τ_close | loops | false closures | wrong-patient msg loops | recall | review items /100 | p_fc | p_wp | certified | smallest certifiable α_fc |
|---|---|---|---|---|---|---|---|---|---|
| 0.999 | 12600 | 0 | 0 | 0.8575 | 88.47 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.995 | 12600 | 0 | 0 | 0.8671 | 69.83 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.99 | 12600 | 0 | 0 | 0.8671 | 69.83 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.98 | 12600 | 0 | 0 | 0.8744 | 63.83 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.95 | 12600 | 0 | 0 | 0.8744 | 63.83 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.9 | 12600 | 3 | 0 | 0.8774 | 59.87 | 1.43e-03 | 3.35e-06 | yes | 6.96e-04 |
| 0.8 | 12600 | 3 | 0 | 0.8774 | 59.87 | 1.43e-03 | 3.35e-06 | yes | 6.96e-04 |
| 0.7 | 12600 | 3 | 0 | 0.8774 | 59.87 | 1.43e-03 | 3.35e-06 | yes | 6.96e-04 |
| 0.5 | 12600 | 9 | 0 | 0.8792 | 58.66 | 1.94e-01 | 3.35e-06 | no | 1.36e-03 |

## no review: loosest certified λ = 0.5; operating λ̂ = **0.5**

| τ_close | loops | false closures | wrong-patient msg loops | recall | review items /100 | p_fc | p_wp | certified | smallest certifiable α_fc |
|---|---|---|---|---|---|---|---|---|---|
| 0.999 | 12600 | 0 | 0 | 0.1806 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.995 | 12600 | 0 | 0 | 0.2561 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.99 | 12600 | 0 | 0 | 0.2561 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.98 | 12600 | 0 | 0 | 0.3238 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.95 | 12600 | 0 | 0 | 0.3238 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.9 | 12600 | 0 | 0 | 0.3709 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.8 | 12600 | 0 | 0 | 0.3709 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.7 | 12600 | 0 | 0 | 0.3709 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.5 | 12600 | 2 | 0 | 0.3872 | 0.0 | 3.12e-04 | 3.35e-06 | yes | 5.73e-04 |

With 0 observed errors, certifying α = 0.001 at δ/2 = 0.025 needs n ≥ 3688 calibration loops.

**Operating rule (pre-declared):** FWER control means every certified tier satisfies both risk bounds simultaneously with probability ≥ 1−δ, so any choice inside the certified set keeps the guarantee. Wasl uses the strictest certified tier whose recall is within 0.5 pp of the best certified recall: looser tiers that buy <0.5 pp recall for observed false closures are not worth it.

**Scope of the guarantee:** it holds for loops exchangeable with the calibration cohorts (same synthetic generator, cal-split persons, v2 noise, 4% reviewer error, merge+close double review). It is not a guarantee on real Abu Dhabi data: the pilot must re-run this calibration on locally adjudicated loops (≈3k per stratum).
