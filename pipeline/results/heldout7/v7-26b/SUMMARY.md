# Final frozen evaluation

Code revision `3eb4d03e6db2` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T08:06:14.287983Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7794 | 0.758 – 0.8008 |
| crc_recall_no_review | 0.2732 | 0.2454 – 0.301 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9638 | 0.9461 – 0.9815 |
| kpi_truth | 0.3208 | 0.3069 – 0.3347 |
| kpi_wasl | 0.3108 | 0.2961 – 0.3255 |
| kpi_eid_only | 0.1216 | 0.107 – 0.1362 |
| kpi_abs_error_wasl | 0.01 | 0.0085 – 0.0115 |
| kpi_abs_error_eid_only | 0.1992 | 0.1798 – 0.2186 |
| linkage_pair_f1 | 0.9764 | 0.975 – 0.9778 |
| reviewer_h_per_1k | 25.3224 | 24.5142 – 26.1306 |
| review_items_per_100_loops | 74.4286 | 71.7553 – 77.1019 |
| throughput_ev_s | 538.1814 | 497.6833 – 578.6796 |
| review_items_per_100_arab | 76.4046 | 72.2212 – 80.588 |
| review_items_per_100_filipino | 71.6848 | 67.9316 – 75.438 |
| review_items_per_100_south_asian | 75.0176 | 71.1749 – 78.8603 |
| review_items_per_100_western | 68.3874 | 61.3312 – 75.4436 |
| reviewer_h_per_1k_arab | 25.9432 | 24.9059 – 26.9805 |
| reviewer_h_per_1k_filipino | 24.9648 | 21.7442 – 28.1854 |
| reviewer_h_per_1k_south_asian | 25.4318 | 24.1562 – 26.7074 |
| reviewer_h_per_1k_western | 23.2308 | 20.6307 – 25.8309 |
| review_load_ratio_max_min_origin | 1.162 | 1.0687 – 1.2553 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2245 | 0.1640% |
| cervical_false_closures | 0 | 1000 | 0.3680% |
| wrong_patient_released | 0 | 10070 | 0.0370% |
| released_without_granted_consent | 0 | 5576 | 0.0660% |
| duplicate_or_lost_outputs | 0 | 122595 | 0.0030% |
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
| crc_recall | 0.7188 | 0.7068 – 0.7308 |
| crc_recall_no_review | 0.2696 | 0.2485 – 0.2907 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9152 | 0.9018 – 0.9286 |
| kpi_truth | 0.3196 | 0.3012 – 0.338 |
| kpi_wasl | 0.2854 | 0.2671 – 0.3037 |
| kpi_eid_only | 0.118 | 0.113 – 0.123 |
| kpi_abs_error_wasl | 0.0342 | 0.0266 – 0.0418 |
| kpi_abs_error_eid_only | 0.2016 | 0.1857 – 0.2175 |
| linkage_pair_f1 | 0.9412 | 0.9366 – 0.9458 |
| reviewer_h_per_1k | 22.4918 | 21.8915 – 23.0921 |
| review_items_per_100_loops | 66.257 | 65.4544 – 67.0596 |
| throughput_ev_s | 546.7104 | 515.7535 – 577.6674 |
| review_items_per_100_arab | 69.6138 | 68.3142 – 70.9134 |
| review_items_per_100_filipino | 61.8874 | 53.6511 – 70.1237 |
| review_items_per_100_south_asian | 63.594 | 60.0092 – 67.1788 |
| review_items_per_100_western | 61.7184 | 56.3922 – 67.0446 |
| reviewer_h_per_1k_arab | 23.3578 | 22.2761 – 24.4395 |
| reviewer_h_per_1k_filipino | 21.01 | 17.0649 – 24.9551 |
| reviewer_h_per_1k_south_asian | 21.7792 | 20.4704 – 23.088 |
| reviewer_h_per_1k_western | 21.5648 | 19.1739 – 23.9557 |
| review_load_ratio_max_min_origin | 1.2031 | 1.1049 – 1.3014 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2021 | 0.1820% |
| cervical_false_closures | 0 | 970 | 0.3800% |
| wrong_patient_released | 0 | 9377 | 0.0390% |
| released_without_granted_consent | 0 | 5270 | 0.0700% |
| duplicate_or_lost_outputs | 0 | 118579 | 0.0030% |
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
