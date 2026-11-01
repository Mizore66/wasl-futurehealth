# Learn-then-Test closure gate

Calibration: 6 cohorts from the persons3 `cal` split (disjoint from eval), v2 noise. α_fc = 0.001, α_wp = 0.001, δ = 0.05 (δ/2 per risk). p-values: exact binomial tail (HB shown for reference).

All programmes (scripts/ltt_cohorts.sh WASL_LTT_ALL=1): loops over the cohorts by programme: crc 4950, cervical 1980, lung_nodule 3900, diabetic_retinopathy 3900, breast 96. Lung, DR and breast run with the frozen evaluation's switches (-lungsurv, -drnext, -refreject, -xreflink). The pooled certificate below covers a loop drawn from this programme mix; the per-programme tables say how much each programme carries on its own.

**Cal-split capacity and cohort design.** The cal split has 3,588 persons: 2,797 men, 779 women (21.7%) and 12 unknown sex. Each seed contributes CRC 825, cervical 330, lung 650, DR 650 and breast 16 loops, plus 283 distractors. Breast is the binding constraint: after allocating the other loops, capacity probes found only 18 eligible women aged 40-90 with no other loop in the tightest seed; 16 breast loops plus the required 2 distractors therefore fit every seed. Larger CRC/cervical cohorts left too few eligible women. These sizes were fixed before the all-programme LTT sweep; breast supports the pooled certificate but is far too small for a programme-specific zero-event certificate.


## with review: loosest certified λ = 0.5; operating λ̂ = **0.995**

| τ_close | loops | false closures | wrong-patient msg loops | recall | review items /100 | p_fc | p_wp | certified | smallest certifiable α_fc |
|---|---|---|---|---|---|---|---|---|---|
| 0.999 | 14826 | 0 | 0 | 0.9268 | 48.7 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.995 | 14826 | 0 | 0 | 0.9326 | 34.88 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.99 | 14826 | 0 | 0 | 0.9326 | 34.88 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.98 | 14826 | 1 | 0 | 0.9364 | 29.56 | 5.72e-06 | 3.61e-07 | yes | 3.76e-04 |
| 0.95 | 14826 | 1 | 0 | 0.9373 | 28.69 | 5.72e-06 | 3.61e-07 | yes | 3.76e-04 |
| 0.9 | 14826 | 1 | 0 | 0.938 | 27.06 | 5.72e-06 | 3.61e-07 | yes | 3.76e-04 |
| 0.8 | 14826 | 1 | 0 | 0.9383 | 26.86 | 5.72e-06 | 3.61e-07 | yes | 3.76e-04 |
| 0.7 | 14826 | 1 | 0 | 0.9389 | 26.21 | 5.72e-06 | 3.61e-07 | yes | 3.76e-04 |
| 0.5 | 14826 | 1 | 0 | 0.9389 | 26.21 | 5.72e-06 | 3.61e-07 | yes | 3.76e-04 |

**Per programme at the operating tier τ 0.995** (counts summed over the 6 cohorts; each programme's own resolution: lung `closed`/`no_followup_needed` plus a falsely closed surveillance-CT child, DR `closed`/`no_referral_needed` plus `closed` on a referable loop, breast `closed`/`no_recall_needed` plus `closed` on a recall loop). Alone: binomial test at δ/2 = 0.025 per risk (needs n ≥ 3688 with 0 events); all 5 programmes at once: δ/(2·5) = 0.0050 per risk (needs n ≥ 5296).

| programme | loops | false closures | wrong-patient msg loops | recall | review items /100 | p_fc | p_wp | n ≥ 3688 | certified alone | certified jointly | smallest certifiable α_fc |
|---|---|---|---|---|---|---|---|---|---|---|---|
| crc | 4950 | 0 | 0 | 0.7965 | 60.95 | 7.07e-03 | 7.07e-03 | yes | yes | no | 7.45e-04 |
| cervical | 1980 | 0 | 0 | 0.9125 | 51.26 | 1.38e-01 | 1.38e-01 | no | no | no | 1.86e-03 |
| lung_nodule | 3900 | 0 | 0 | 0.9539 | 9.0 | 2.02e-02 | 2.02e-02 | yes | yes | no | 9.45e-04 |
| diabetic_retinopathy | 3900 | 0 | 0 | 0.9962 | 13.26 | 2.02e-02 | 2.02e-02 | yes | yes | no | 9.45e-04 |
| breast | 96 | 0 | 0 | 1.0 | 5.21 | 9.08e-01 | 9.08e-01 | no | no | no | 3.77e-02 |

False closures / wrong-patient message loops by programme at every tier (with review):

| τ_close | crc | cervical | lung_nodule | diabetic_retinopathy | breast |
|---|---|---|---|---|---|
| 0.999 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.995 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.99 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.98 | 1 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.95 | 1 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.9 | 1 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.8 | 1 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.7 | 1 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.5 | 1 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |

## no review: loosest certified λ = 0.5; operating λ̂ = **0.995**

| τ_close | loops | false closures | wrong-patient msg loops | recall | review items /100 | p_fc | p_wp | certified | smallest certifiable α_fc |
|---|---|---|---|---|---|---|---|---|---|
| 0.999 | 14826 | 0 | 0 | 0.6146 | 0.0 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.995 | 14826 | 0 | 0 | 0.701 | 0.0 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.99 | 14826 | 0 | 0 | 0.701 | 0.0 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.98 | 14826 | 0 | 0 | 0.7583 | 0.0 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.95 | 14826 | 0 | 0 | 0.7672 | 0.0 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.9 | 14826 | 0 | 0 | 0.7809 | 0.0 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.8 | 14826 | 0 | 0 | 0.7828 | 0.0 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.7 | 14826 | 0 | 0 | 0.7897 | 0.0 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |
| 0.5 | 14826 | 0 | 0 | 0.7897 | 0.0 | 3.61e-07 | 3.61e-07 | yes | 2.49e-04 |

**Per programme at the operating tier τ 0.995** (counts summed over the 6 cohorts; each programme's own resolution: lung `closed`/`no_followup_needed` plus a falsely closed surveillance-CT child, DR `closed`/`no_referral_needed` plus `closed` on a referable loop, breast `closed`/`no_recall_needed` plus `closed` on a recall loop). Alone: binomial test at δ/2 = 0.025 per risk (needs n ≥ 3688 with 0 events); all 5 programmes at once: δ/(2·5) = 0.0050 per risk (needs n ≥ 5296).

| programme | loops | false closures | wrong-patient msg loops | recall | review items /100 | p_fc | p_wp | n ≥ 3688 | certified alone | certified jointly | smallest certifiable α_fc |
|---|---|---|---|---|---|---|---|---|---|---|---|
| crc | 4950 | 0 | 0 | 0.3215 | 0.0 | 7.07e-03 | 7.07e-03 | yes | yes | no | 7.45e-04 |
| cervical | 1980 | 0 | 0 | 0.5653 | 0.0 | 1.38e-01 | 1.38e-01 | no | no | no | 1.86e-03 |
| lung_nodule | 3900 | 0 | 0 | 0.8041 | 0.0 | 2.02e-02 | 2.02e-02 | yes | yes | no | 9.45e-04 |
| diabetic_retinopathy | 3900 | 0 | 0 | 0.8733 | 0.0 | 2.02e-02 | 2.02e-02 | yes | yes | no | 9.45e-04 |
| breast | 96 | 0 | 0 | 0.957 | 0.0 | 9.08e-01 | 9.08e-01 | no | no | no | 3.77e-02 |

False closures / wrong-patient message loops by programme at every tier (no review):

| τ_close | crc | cervical | lung_nodule | diabetic_retinopathy | breast |
|---|---|---|---|---|---|
| 0.999 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.995 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.99 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.98 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.95 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.9 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.8 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.7 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 0.5 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |

With 0 observed errors, certifying α = 0.001 at δ/2 = 0.025 needs n ≥ 3688 calibration loops.

**Design floor:** the operating tier must be ≥ 0.99, above the 0.989 cap on uncorroborated model edges, so a name-only link cannot auto-close a loop. This floor was added after the final-code evaluation found one wrong-patient closure at the cap under τ 0.98; it is a disclosed post-hoc change.

**Operating rule (pre-declared):** FWER control means every certified tier satisfies both risk bounds simultaneously with probability ≥ 1−δ, so any choice inside the certified set keeps the guarantee. Wasl uses the strictest certified tier whose recall is within 0.5 pp of the best certified recall: looser tiers that buy <0.5 pp recall for observed false closures are not worth it.

**Scope of the guarantee:** it holds for loops exchangeable with the calibration cohorts (same synthetic generator, cal-split persons, v2 noise, 4% reviewer error, merge+close double review). It is not a guarantee on real Abu Dhabi data: the pilot must re-run this calibration on locally adjudicated loops (≈3k per stratum).
