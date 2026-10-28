# Learn-then-Test closure gate

Calibration: 6 cohorts from the persons3 `cal` split (disjoint from eval), v2 noise. α_fc = 0.001, α_wp = 0.001, δ = 0.05 (δ/2 per risk). p-values: exact binomial tail (HB shown for reference).


## with review: loosest certified λ = 0.5; operating λ̂ = **0.995**

| τ_close | loops | false closures | wrong-patient msg loops | recall | review items /100 | p_fc | p_wp | certified | smallest certifiable α_fc |
|---|---|---|---|---|---|---|---|---|---|
| 0.999 | 12600 | 0 | 0 | 0.8683 | 108.58 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.995 | 12600 | 0 | 0 | 0.8831 | 82.44 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.99 | 12600 | 0 | 0 | 0.8831 | 82.44 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.98 | 12600 | 0 | 0 | 0.8919 | 77.29 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.95 | 12600 | 0 | 0 | 0.8941 | 76.17 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.9 | 12600 | 1 | 0 | 0.8961 | 74.33 | 4.56e-05 | 3.35e-06 | yes | 4.42e-04 |
| 0.8 | 12600 | 1 | 0 | 0.8963 | 74.1 | 4.56e-05 | 3.35e-06 | yes | 4.42e-04 |
| 0.7 | 12600 | 3 | 0 | 0.8985 | 73.35 | 1.43e-03 | 3.35e-06 | yes | 6.96e-04 |
| 0.5 | 12600 | 3 | 0 | 0.8985 | 73.35 | 1.43e-03 | 3.35e-06 | yes | 6.96e-04 |

## no review: loosest certified λ = 0.5; operating λ̂ = **0.995**

| τ_close | loops | false closures | wrong-patient msg loops | recall | review items /100 | p_fc | p_wp | certified | smallest certifiable α_fc |
|---|---|---|---|---|---|---|---|---|---|
| 0.999 | 12600 | 0 | 0 | 0.1801 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.995 | 12600 | 0 | 0 | 0.2946 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.99 | 12600 | 0 | 0 | 0.2946 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.98 | 12600 | 0 | 0 | 0.3717 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.95 | 12600 | 0 | 0 | 0.3847 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.9 | 12600 | 0 | 0 | 0.4017 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.8 | 12600 | 0 | 0 | 0.4039 | 0.0 | 3.35e-06 | 3.35e-06 | yes | 2.93e-04 |
| 0.7 | 12600 | 2 | 0 | 0.4122 | 0.0 | 3.12e-04 | 3.35e-06 | yes | 5.73e-04 |
| 0.5 | 12600 | 2 | 0 | 0.4122 | 0.0 | 3.12e-04 | 3.35e-06 | yes | 5.73e-04 |

With 0 observed errors, certifying α = 0.001 at δ/2 = 0.025 needs n ≥ 3688 calibration loops.

**Design floor:** the operating tier must be ≥ 0.99, above the 0.989 cap on uncorroborated model edges, so a name-only link cannot auto-close a loop. This floor was added after the final-code evaluation found one wrong-patient closure at the cap under τ 0.98; it is a disclosed post-hoc change.

**Operating rule (pre-declared):** FWER control means every certified tier satisfies both risk bounds simultaneously with probability ≥ 1−δ, so any choice inside the certified set keeps the guarantee. Wasl uses the strictest certified tier whose recall is within 0.5 pp of the best certified recall: looser tiers that buy <0.5 pp recall for observed false closures are not worth it.

**Scope of the guarantee:** it holds for loops exchangeable with the calibration cohorts (same synthetic generator, cal-split persons, v2 noise, 4% reviewer error, merge+close double review). It is not a guarantee on real Abu Dhabi data: the pilot must re-run this calibration on locally adjudicated loops (≈3k per stratum).
