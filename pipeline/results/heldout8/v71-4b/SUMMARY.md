# Final frozen evaluation

Code revision `2cac5690647b` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T09:16:06.647604Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7868 | 0.7777 – 0.7959 |
| crc_recall_no_review | 0.0524 | 0.0441 – 0.0607 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9696 | 0.9543 – 0.9849 |
| kpi_truth | 0.327 | 0.3025 – 0.3515 |
| kpi_wasl | 0.3154 | 0.2913 – 0.3395 |
| kpi_eid_only | 0.1202 | 0.1061 – 0.1343 |
| kpi_abs_error_wasl | 0.0116 | 0.0099 – 0.0133 |
| kpi_abs_error_eid_only | 0.2068 | 0.1926 – 0.221 |
| linkage_pair_f1 | 0.9758 | 0.9721 – 0.9795 |
| reviewer_h_per_1k | 32.426 | 31.4146 – 33.4374 |
| review_items_per_100_loops | 90.5522 | 88.7439 – 92.3605 |
| throughput_ev_s | 611.5963 | 555.7488 – 667.4438 |
| review_items_per_100_arab | 94.3964 | 91.3822 – 97.4106 |
| review_items_per_100_filipino | 85.138 | 75.0412 – 95.2348 |
| review_items_per_100_south_asian | 90.197 | 83.1698 – 97.2242 |
| review_items_per_100_western | 81.3756 | 75.4102 – 87.341 |
| reviewer_h_per_1k_arab | 33.6492 | 32.7103 – 34.5881 |
| reviewer_h_per_1k_filipino | 30.0712 | 25.5898 – 34.5526 |
| reviewer_h_per_1k_south_asian | 32.7026 | 29.545 – 35.8602 |
| reviewer_h_per_1k_western | 29.1458 | 27.6058 – 30.6858 |
| review_load_ratio_max_min_origin | 1.1911 | 1.0966 – 1.2855 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2272 | 0.1620% |
| cervical_false_closures | 0 | 1006 | 0.3660% |
| wrong_patient_released | 0 | 10084 | 0.0370% |
| released_without_granted_consent | 0 | 5599 | 0.0660% |
| duplicate_or_lost_outputs | 0 | 125871 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153052 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 119 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7202 | 0.7056 – 0.7348 |
| crc_recall_no_review | 0.0468 | 0.0369 – 0.0567 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.916 | 0.9054 – 0.9266 |
| kpi_truth | 0.3184 | 0.3082 – 0.3286 |
| kpi_wasl | 0.2874 | 0.2816 – 0.2932 |
| kpi_eid_only | 0.1262 | 0.1193 – 0.1331 |
| kpi_abs_error_wasl | 0.031 | 0.0228 – 0.0392 |
| kpi_abs_error_eid_only | 0.1922 | 0.1831 – 0.2013 |
| linkage_pair_f1 | 0.9426 | 0.9405 – 0.9447 |
| reviewer_h_per_1k | 29.041 | 28.2947 – 29.7873 |
| review_items_per_100_loops | 80.4858 | 79.8668 – 81.1048 |
| throughput_ev_s | 592.8044 | 523.6128 – 661.996 |
| review_items_per_100_arab | 83.1888 | 79.4132 – 86.9644 |
| review_items_per_100_filipino | 77.6962 | 70.8862 – 84.5062 |
| review_items_per_100_south_asian | 77.7986 | 74.1605 – 81.4367 |
| review_items_per_100_western | 77.1626 | 65.161 – 89.1642 |
| reviewer_h_per_1k_arab | 29.7502 | 28.3477 – 31.1527 |
| reviewer_h_per_1k_filipino | 28.8882 | 26.3267 – 31.4497 |
| reviewer_h_per_1k_south_asian | 27.8746 | 26.6752 – 29.074 |
| reviewer_h_per_1k_western | 28.5888 | 23.9779 – 33.1997 |
| review_load_ratio_max_min_origin | 1.1845 | 1.0609 – 1.3081 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2029 | 0.1820% |
| cervical_false_closures | 0 | 971 | 0.3790% |
| wrong_patient_released | 0 | 9449 | 0.0390% |
| released_without_granted_consent | 0 | 5462 | 0.0680% |
| duplicate_or_lost_outputs | 0 | 121989 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153807 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 107 | – |
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
