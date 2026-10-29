# Final frozen evaluation

Code revision `7962054f99fb` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T03:39:03.392769Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.9122 | 0.894 – 0.9304 |
| crc_recall_no_review | 0.266 | 0.2412 – 0.2908 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9718 | 0.9494 – 0.9942 |
| kpi_truth | 0.3236 | 0.3081 – 0.3391 |
| kpi_wasl | 0.3124 | 0.2991 – 0.3257 |
| kpi_eid_only | 0.1198 | 0.1121 – 0.1275 |
| kpi_abs_error_wasl | 0.0112 | 0.0084 – 0.014 |
| kpi_abs_error_eid_only | 0.2038 | 0.185 – 0.2226 |
| linkage_pair_f1 | 0.978 | 0.9739 – 0.9821 |
| reviewer_h_per_1k | 28.3644 | 28.1903 – 28.5385 |
| review_items_per_100_loops | 81.5712 | 79.8409 – 83.3015 |
| throughput_ev_s | 263.7952 | 221.3381 – 306.2523 |
| review_items_per_100_arab | 84.8964 | 81.0663 – 88.7265 |
| review_items_per_100_filipino | 78.5112 | 71.1131 – 85.9093 |
| review_items_per_100_south_asian | 80.9954 | 77.2277 – 84.7631 |
| review_items_per_100_western | 73.1378 | 68.6619 – 77.6137 |
| reviewer_h_per_1k_arab | 29.5552 | 28.5688 – 30.5416 |
| reviewer_h_per_1k_filipino | 27.7226 | 24.4658 – 30.9794 |
| reviewer_h_per_1k_south_asian | 28.0824 | 26.7369 – 29.4279 |
| reviewer_h_per_1k_western | 25.1932 | 22.5917 – 27.7947 |
| review_load_ratio_max_min_origin | 1.1874 | 1.079 – 1.2958 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2636 | 0.1400% |
| cervical_false_closures | 0 | 1008 | 0.3650% |
| wrong_patient_released | 0 | 10088 | 0.0370% |
| released_without_granted_consent | 0 | 7223 | 0.0510% |
| duplicate_or_lost_outputs | 0 | 126790 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153402 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 113 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 4 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8548 | 0.838 – 0.8716 |
| crc_recall_no_review | 0.2562 | 0.2293 – 0.2831 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9182 | 0.9114 – 0.925 |
| kpi_truth | 0.3256 | 0.315 – 0.3362 |
| kpi_wasl | 0.2954 | 0.2866 – 0.3042 |
| kpi_eid_only | 0.1286 | 0.1205 – 0.1367 |
| kpi_abs_error_wasl | 0.0302 | 0.0256 – 0.0348 |
| kpi_abs_error_eid_only | 0.197 | 0.1872 – 0.2068 |
| linkage_pair_f1 | 0.9426 | 0.9367 – 0.9485 |
| reviewer_h_per_1k | 26.0024 | 25.0216 – 26.9832 |
| review_items_per_100_loops | 74.057 | 72.6745 – 75.4395 |
| throughput_ev_s | 295.5008 | 131.7415 – 459.26 |
| review_items_per_100_arab | 77.3834 | 74.1534 – 80.6134 |
| review_items_per_100_filipino | 72.152 | 65.3569 – 78.9471 |
| review_items_per_100_south_asian | 71.6196 | 66.2306 – 77.0086 |
| review_items_per_100_western | 67.6862 | 65.7516 – 69.6208 |
| reviewer_h_per_1k_arab | 26.8548 | 25.1068 – 28.6028 |
| reviewer_h_per_1k_filipino | 25.419 | 22.2353 – 28.6027 |
| reviewer_h_per_1k_south_asian | 25.7242 | 22.9592 – 28.4892 |
| reviewer_h_per_1k_western | 23.827 | 22.6383 – 25.0157 |
| review_load_ratio_max_min_origin | 1.1653 | 1.101 – 1.2297 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2485 | 0.1480% |
| cervical_false_closures | 0 | 973 | 0.3780% |
| wrong_patient_released | 0 | 9472 | 0.0390% |
| released_without_granted_consent | 0 | 5880 | 0.0630% |
| duplicate_or_lost_outputs | 0 | 121582 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153424 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 111 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 2 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
