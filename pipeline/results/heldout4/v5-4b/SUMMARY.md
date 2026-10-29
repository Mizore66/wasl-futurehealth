# Final frozen evaluation

Code revision `8916058cd8a5` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T05:25:28.726039Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8174 | 0.7841 – 0.8507 |
| crc_recall_no_review | 0.0952 | 0.085 – 0.1054 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9726 | 0.9532 – 0.992 |
| kpi_truth | 0.3254 | 0.3123 – 0.3385 |
| kpi_wasl | 0.3152 | 0.3023 – 0.3281 |
| kpi_eid_only | 0.1216 | 0.1114 – 0.1318 |
| kpi_abs_error_wasl | 0.0102 | 0.0044 – 0.016 |
| kpi_abs_error_eid_only | 0.2038 | 0.1916 – 0.216 |
| linkage_pair_f1 | 0.9784 | 0.976 – 0.9808 |
| reviewer_h_per_1k | 32.06 | 30.5399 – 33.5801 |
| review_items_per_100_loops | 88.619 | 83.8398 – 93.3982 |
| throughput_ev_s | 291.5513 | 259.9858 – 323.1168 |
| review_items_per_100_arab | 90.9766 | 87.7896 – 94.1636 |
| review_items_per_100_filipino | 84.9464 | 77.8207 – 92.0721 |
| review_items_per_100_south_asian | 88.3274 | 78.6111 – 98.0437 |
| review_items_per_100_western | 83.231 | 78.2598 – 88.2022 |
| reviewer_h_per_1k_arab | 32.809 | 31.6764 – 33.9416 |
| reviewer_h_per_1k_filipino | 30.7734 | 28.0015 – 33.5453 |
| reviewer_h_per_1k_south_asian | 31.927 | 28.2602 – 35.5938 |
| reviewer_h_per_1k_western | 30.5072 | 27.9355 – 33.0789 |
| review_load_ratio_max_min_origin | 1.1276 | 1.0718 – 1.1834 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2338 | 0.1580% |
| cervical_false_closures | 0 | 1009 | 0.3650% |
| wrong_patient_released | 0 | 10051 | 0.0370% |
| released_without_granted_consent | 0 | 6863 | 0.0540% |
| duplicate_or_lost_outputs | 0 | 127517 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153354 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 116 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7592 | 0.738 – 0.7804 |
| crc_recall_no_review | 0.0902 | 0.0772 – 0.1032 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9162 | 0.9087 – 0.9237 |
| kpi_truth | 0.3242 | 0.3096 – 0.3388 |
| kpi_wasl | 0.2932 | 0.2768 – 0.3096 |
| kpi_eid_only | 0.126 | 0.1136 – 0.1384 |
| kpi_abs_error_wasl | 0.031 | 0.0264 – 0.0356 |
| kpi_abs_error_eid_only | 0.1982 | 0.182 – 0.2144 |
| linkage_pair_f1 | 0.9444 | 0.9413 – 0.9475 |
| reviewer_h_per_1k | 29.3136 | 28.3027 – 30.3245 |
| review_items_per_100_loops | 80.4858 | 77.8476 – 83.124 |
| throughput_ev_s | 379.6559 | 199.1568 – 560.155 |
| review_items_per_100_arab | 85.0264 | 81.8014 – 88.2514 |
| review_items_per_100_filipino | 74.8898 | 68.7514 – 81.0282 |
| review_items_per_100_south_asian | 74.9696 | 67.9392 – 82.0 |
| review_items_per_100_western | 77.644 | 71.3984 – 83.8896 |
| reviewer_h_per_1k_arab | 30.8744 | 29.0329 – 32.7159 |
| reviewer_h_per_1k_filipino | 26.4298 | 23.702 – 29.1576 |
| reviewer_h_per_1k_south_asian | 27.474 | 25.173 – 29.775 |
| reviewer_h_per_1k_western | 28.8518 | 27.657 – 30.0466 |
| review_load_ratio_max_min_origin | 1.1977 | 1.1377 – 1.2576 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2140 | 0.1720% |
| cervical_false_closures | 0 | 971 | 0.3790% |
| wrong_patient_released | 0 | 9359 | 0.0390% |
| released_without_granted_consent | 0 | 6834 | 0.0540% |
| duplicate_or_lost_outputs | 0 | 124483 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153569 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 107 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 3 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
