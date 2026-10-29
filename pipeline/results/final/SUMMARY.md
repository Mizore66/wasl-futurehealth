# Final frozen evaluation

Code revision `315a9147e9bd` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-28T23:09:26.342761Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.9462 | 0.939 – 0.9534 |
| crc_recall_no_review | 0.1904 | 0.1705 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9676 | 0.9524 – 0.9828 |
| kpi_truth | 0.3312 | 0.3022 – 0.3602 |
| kpi_wasl | 0.3212 | 0.293 – 0.3494 |
| kpi_eid_only | 0.1238 | 0.1093 – 0.1383 |
| kpi_abs_error_wasl | 0.01 | 0.0074 – 0.0126 |
| kpi_abs_error_eid_only | 0.2074 | 0.1857 – 0.2291 |
| linkage_pair_f1 | 0.9784 | 0.9736 – 0.9832 |
| reviewer_h_per_1k | 28.713 | 27.6606 – 29.7654 |
| review_items_per_100_loops | 84.1046 | 80.9356 – 87.2736 |
| throughput_ev_s | 315.0867 | 305.9975 – 324.1758 |
| review_items_per_100_arab | 86.441 | 81.6361 – 91.2459 |
| review_items_per_100_filipino | 75.4658 | 73.8667 – 77.0649 |
| review_items_per_100_south_asian | 84.64 | 80.8469 – 88.4331 |
| review_items_per_100_western | 80.3458 | 76.7411 – 83.9505 |
| reviewer_h_per_1k_arab | 29.6366 | 27.9569 – 31.3163 |
| reviewer_h_per_1k_filipino | 26.6688 | 24.4426 – 28.895 |
| reviewer_h_per_1k_south_asian | 28.5968 | 27.2894 – 29.9042 |
| reviewer_h_per_1k_western | 26.9578 | 25.0865 – 28.8291 |
| review_load_ratio_max_min_origin | 1.1484 | 1.0685 – 1.2283 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2698 | 0.1370% |
| cervical_false_closures | 0 | 1004 | 0.3670% |
| wrong_patient_released | 0 | 10089 | 0.0370% |
| released_without_granted_consent | 0 | 5455 | 0.0680% |
| duplicate_or_lost_outputs | 0 | 124775 | 0.0030% |
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
| crc_recall | 0.879 | 0.8622 – 0.8958 |
| crc_recall_no_review | 0.1946 | 0.1789 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9142 | 0.9031 – 0.9253 |
| kpi_truth | 0.3266 | 0.3071 – 0.3461 |
| kpi_wasl | 0.2914 | 0.2744 – 0.3084 |
| kpi_eid_only | 0.1212 | 0.1107 – 0.1317 |
| kpi_abs_error_wasl | 0.0352 | 0.0265 – 0.0439 |
| kpi_abs_error_eid_only | 0.2054 | 0.1954 – 0.2154 |
| linkage_pair_f1 | 0.942 | 0.9376 – 0.9464 |
| reviewer_h_per_1k | 25.9126 | 25.257 – 26.5682 |
| review_items_per_100_loops | 75.5524 | 73.7655 – 77.3393 |
| throughput_ev_s | 402.6835 | 188.0286 – 617.3383 |
| review_items_per_100_arab | 78.6318 | 77.2155 – 80.0481 |
| review_items_per_100_filipino | 68.9094 | 62.2126 – 75.6062 |
| review_items_per_100_south_asian | 75.4536 | 70.0921 – 80.8151 |
| review_items_per_100_western | 68.9928 | 66.8549 – 71.1307 |
| reviewer_h_per_1k_arab | 26.9406 | 26.47 – 27.4112 |
| reviewer_h_per_1k_filipino | 23.3976 | 21.6982 – 25.097 |
| reviewer_h_per_1k_south_asian | 25.8864 | 23.2684 – 28.5044 |
| reviewer_h_per_1k_western | 23.8994 | 22.6009 – 25.1979 |
| review_load_ratio_max_min_origin | 1.1796 | 1.107 – 1.2523 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2533 | 0.1460% |
| cervical_false_closures | 0 | 969 | 0.3800% |
| wrong_patient_released | 0 | 9408 | 0.0390% |
| released_without_granted_consent | 0 | 5191 | 0.0710% |
| duplicate_or_lost_outputs | 0 | 120721 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153692 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 111 | – |
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
