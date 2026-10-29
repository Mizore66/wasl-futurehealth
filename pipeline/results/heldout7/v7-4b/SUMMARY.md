# Final frozen evaluation

Code revision `3eb4d03e6db2` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T08:31:43.937654Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.6556 | 0.6282 – 0.683 |
| crc_recall_no_review | 0.2002 | 0.1733 – 0.2271 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9638 | 0.9461 – 0.9815 |
| kpi_truth | 0.3208 | 0.3069 – 0.3347 |
| kpi_wasl | 0.3108 | 0.2961 – 0.3255 |
| kpi_eid_only | 0.1216 | 0.107 – 0.1362 |
| kpi_abs_error_wasl | 0.01 | 0.0085 – 0.0115 |
| kpi_abs_error_eid_only | 0.1992 | 0.1798 – 0.2186 |
| linkage_pair_f1 | 0.9762 | 0.9746 – 0.9778 |
| reviewer_h_per_1k | 25.377 | 24.066 – 26.688 |
| review_items_per_100_loops | 74.8952 | 71.3756 – 78.4148 |
| throughput_ev_s | 559.969 | 540.688 – 579.25 |
| review_items_per_100_arab | 77.1492 | 71.3366 – 82.9618 |
| review_items_per_100_filipino | 72.2484 | 67.8949 – 76.6019 |
| review_items_per_100_south_asian | 74.4692 | 70.9328 – 78.0056 |
| review_items_per_100_western | 69.5626 | 61.8693 – 77.2559 |
| reviewer_h_per_1k_arab | 26.024 | 24.1079 – 27.9401 |
| reviewer_h_per_1k_filipino | 24.904 | 22.1357 – 27.6723 |
| reviewer_h_per_1k_south_asian | 24.953 | 23.9875 – 25.9185 |
| reviewer_h_per_1k_western | 24.1978 | 20.556 – 27.8396 |
| review_load_ratio_max_min_origin | 1.1633 | 1.0744 – 1.2523 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1889 | 0.1950% |
| cervical_false_closures | 0 | 1000 | 0.3680% |
| wrong_patient_released | 0 | 10071 | 0.0370% |
| released_without_granted_consent | 0 | 5503 | 0.0670% |
| duplicate_or_lost_outputs | 0 | 122654 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153365 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 114 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.6116 | 0.5848 – 0.6384 |
| crc_recall_no_review | 0.1904 | 0.1777 – 0.2031 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9144 | 0.8999 – 0.9289 |
| kpi_truth | 0.3196 | 0.3012 – 0.338 |
| kpi_wasl | 0.2856 | 0.2675 – 0.3037 |
| kpi_eid_only | 0.118 | 0.113 – 0.123 |
| kpi_abs_error_wasl | 0.034 | 0.0261 – 0.0419 |
| kpi_abs_error_eid_only | 0.2016 | 0.1857 – 0.2175 |
| linkage_pair_f1 | 0.9414 | 0.9368 – 0.946 |
| reviewer_h_per_1k | 22.9888 | 21.8308 – 24.1468 |
| review_items_per_100_loops | 67.1712 | 65.3591 – 68.9833 |
| throughput_ev_s | 581.0379 | 465.3809 – 696.6948 |
| review_items_per_100_arab | 70.629 | 69.328 – 71.93 |
| review_items_per_100_filipino | 63.6588 | 55.2395 – 72.0781 |
| review_items_per_100_south_asian | 64.3134 | 59.8976 – 68.7292 |
| review_items_per_100_western | 62.1008 | 56.8482 – 67.3534 |
| reviewer_h_per_1k_arab | 23.9448 | 22.9413 – 24.9483 |
| reviewer_h_per_1k_filipino | 22.0416 | 18.0145 – 26.0687 |
| reviewer_h_per_1k_south_asian | 22.0922 | 20.534 – 23.6504 |
| reviewer_h_per_1k_western | 21.7242 | 19.1231 – 24.3253 |
| review_load_ratio_max_min_origin | 1.1988 | 1.1395 – 1.2582 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1720 | 0.2140% |
| cervical_false_closures | 0 | 969 | 0.3800% |
| wrong_patient_released | 0 | 9363 | 0.0390% |
| released_without_granted_consent | 0 | 5407 | 0.0680% |
| duplicate_or_lost_outputs | 0 | 118892 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153249 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 112 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
