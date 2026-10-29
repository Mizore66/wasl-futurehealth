# Final frozen evaluation

Code revision `e25296f925a0` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-28T22:25:39.014388Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 0.991 | 0.986 – 0.996 |
| crc_recall | 0.8752 | 0.8543 – 0.8961 |
| crc_recall_no_review | 0.2358 | 0.2078 – 0.2638 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9718 | 0.9472 – 0.9964 |
| kpi_truth | 0.3236 | 0.3081 – 0.3391 |
| kpi_wasl | 0.312 | 0.2986 – 0.3254 |
| kpi_eid_only | 0.1198 | 0.1121 – 0.1275 |
| kpi_abs_error_wasl | 0.0116 | 0.0086 – 0.0146 |
| kpi_abs_error_eid_only | 0.2038 | 0.185 – 0.2226 |
| linkage_pair_f1 | 0.9774 | 0.9732 – 0.9816 |
| reviewer_h_per_1k | 27.5992 | 26.956 – 28.2424 |
| review_items_per_100_loops | 81.1522 | 78.998 – 83.3064 |
| throughput_ev_s | 590.375 | 541.8262 – 638.9239 |
| review_items_per_100_arab | 84.7322 | 80.5287 – 88.9357 |
| review_items_per_100_filipino | 77.342 | 71.0004 – 83.6836 |
| review_items_per_100_south_asian | 79.9516 | 77.1365 – 82.7667 |
| review_items_per_100_western | 73.3052 | 68.8395 – 77.7709 |
| reviewer_h_per_1k_arab | 28.8172 | 27.6429 – 29.9915 |
| reviewer_h_per_1k_filipino | 26.9648 | 24.1786 – 29.751 |
| reviewer_h_per_1k_south_asian | 27.1116 | 26.4722 – 27.751 |
| reviewer_h_per_1k_western | 24.6466 | 21.7916 – 27.5016 |
| review_load_ratio_max_min_origin | 1.1773 | 1.0604 – 1.2941 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 23 | 2553 | 1.3490% |
| cervical_false_closures | 0 | 1008 | 0.3650% |
| wrong_patient_released | 0 | 10131 | 0.0360% |
| released_without_granted_consent | 0 | 5611 | 0.0660% |
| duplicate_or_lost_outputs | 0 | 123917 | 0.0030% |
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
| crc_precision | 0.988 | 0.983 – 0.993 |
| crc_recall | 0.82 | 0.8034 – 0.8366 |
| crc_recall_no_review | 0.2404 | 0.2086 – 0.2722 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9172 | 0.9107 – 0.9237 |
| kpi_truth | 0.3256 | 0.315 – 0.3362 |
| kpi_wasl | 0.2954 | 0.2863 – 0.3045 |
| kpi_eid_only | 0.1286 | 0.1205 – 0.1367 |
| kpi_abs_error_wasl | 0.0302 | 0.0256 – 0.0348 |
| kpi_abs_error_eid_only | 0.197 | 0.1872 – 0.2068 |
| linkage_pair_f1 | 0.9428 | 0.9366 – 0.949 |
| reviewer_h_per_1k | 25.0658 | 24.2061 – 25.9255 |
| review_items_per_100_loops | 72.9428 | 71.6353 – 74.2503 |
| throughput_ev_s | 596.6638 | 486.9997 – 706.3279 |
| review_items_per_100_arab | 76.421 | 73.6234 – 79.2186 |
| review_items_per_100_filipino | 70.8986 | 63.742 – 78.0552 |
| review_items_per_100_south_asian | 70.9114 | 66.3393 – 75.4835 |
| review_items_per_100_western | 65.4548 | 62.1529 – 68.7567 |
| reviewer_h_per_1k_arab | 26.058 | 24.6234 – 27.4926 |
| reviewer_h_per_1k_filipino | 24.1224 | 20.6495 – 27.5953 |
| reviewer_h_per_1k_south_asian | 24.9776 | 22.4846 – 27.4706 |
| reviewer_h_per_1k_western | 22.3334 | 20.9695 – 23.6973 |
| review_load_ratio_max_min_origin | 1.1881 | 1.1163 – 1.2598 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 29 | 2413 | 1.7210% |
| cervical_false_closures | 0 | 972 | 0.3790% |
| wrong_patient_released | 0 | 9488 | 0.0390% |
| released_without_granted_consent | 0 | 5496 | 0.0670% |
| duplicate_or_lost_outputs | 0 | 120358 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153424 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 1 | 111 | – |
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
