# Final frozen evaluation

Code revision `3eb4d03e6db2` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T07:35:04.171199Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.9168 | 0.9123 – 0.9213 |
| crc_recall_no_review | 0.1904 | 0.1705 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9676 | 0.9524 – 0.9828 |
| kpi_truth | 0.3312 | 0.3022 – 0.3602 |
| kpi_wasl | 0.3212 | 0.293 – 0.3494 |
| kpi_eid_only | 0.1238 | 0.1093 – 0.1383 |
| kpi_abs_error_wasl | 0.01 | 0.0074 – 0.0126 |
| kpi_abs_error_eid_only | 0.2074 | 0.1857 – 0.2291 |
| linkage_pair_f1 | 0.9786 | 0.9736 – 0.9836 |
| reviewer_h_per_1k | 28.8524 | 27.745 – 29.9598 |
| review_items_per_100_loops | 83.5524 | 80.5198 – 86.585 |
| throughput_ev_s | 577.6372 | 536.6586 – 618.6158 |
| review_items_per_100_arab | 86.0504 | 81.3329 – 90.7679 |
| review_items_per_100_filipino | 74.9258 | 73.9135 – 75.9381 |
| review_items_per_100_south_asian | 84.1094 | 81.8613 – 86.3575 |
| review_items_per_100_western | 79.1704 | 75.0257 – 83.3151 |
| reviewer_h_per_1k_arab | 29.849 | 28.1207 – 31.5773 |
| reviewer_h_per_1k_filipino | 26.639 | 24.6591 – 28.6189 |
| reviewer_h_per_1k_south_asian | 28.7148 | 27.8501 – 29.5795 |
| reviewer_h_per_1k_western | 26.9704 | 24.9646 – 28.9762 |
| review_load_ratio_max_min_origin | 1.1547 | 1.0888 – 1.2206 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2614 | 0.1410% |
| cervical_false_closures | 0 | 1004 | 0.3670% |
| wrong_patient_released | 0 | 10089 | 0.0370% |
| released_without_granted_consent | 0 | 5567 | 0.0660% |
| duplicate_or_lost_outputs | 0 | 124401 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153646 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 114 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8504 | 0.8296 – 0.8712 |
| crc_recall_no_review | 0.1946 | 0.1789 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9152 | 0.906 – 0.9244 |
| kpi_truth | 0.3266 | 0.3071 – 0.3461 |
| kpi_wasl | 0.2916 | 0.2742 – 0.309 |
| kpi_eid_only | 0.1212 | 0.1107 – 0.1317 |
| kpi_abs_error_wasl | 0.035 | 0.026 – 0.044 |
| kpi_abs_error_eid_only | 0.2054 | 0.1954 – 0.2154 |
| linkage_pair_f1 | 0.9422 | 0.9376 – 0.9468 |
| reviewer_h_per_1k | 25.821 | 24.9614 – 26.6806 |
| review_items_per_100_loops | 74.695 | 72.3922 – 76.9978 |
| throughput_ev_s | 556.7544 | 471.5113 – 641.9975 |
| review_items_per_100_arab | 77.7636 | 76.1139 – 79.4133 |
| review_items_per_100_filipino | 67.978 | 61.4621 – 74.4939 |
| review_items_per_100_south_asian | 74.3876 | 68.4229 – 80.3523 |
| review_items_per_100_western | 68.5802 | 65.3031 – 71.8573 |
| reviewer_h_per_1k_arab | 26.864 | 26.231 – 27.497 |
| reviewer_h_per_1k_filipino | 23.292 | 21.7902 – 24.7938 |
| reviewer_h_per_1k_south_asian | 25.6978 | 22.7871 – 28.6085 |
| reviewer_h_per_1k_western | 23.9368 | 22.6707 – 25.2029 |
| review_load_ratio_max_min_origin | 1.1754 | 1.1107 – 1.2401 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2451 | 0.1500% |
| cervical_false_closures | 0 | 970 | 0.3800% |
| wrong_patient_released | 0 | 9389 | 0.0390% |
| released_without_granted_consent | 0 | 5903 | 0.0620% |
| duplicate_or_lost_outputs | 0 | 121529 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153692 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 111 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 1 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
