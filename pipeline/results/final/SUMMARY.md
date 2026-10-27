# Final frozen evaluation

Code revision `82bceb2c2b97` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-27T22:05:21.182599Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.945 | 0.9326 – 0.9574 |
| crc_recall_no_review | 0.184 | 0.1635 – 0.2045 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9598 | 0.9388 – 0.9808 |
| kpi_truth | 0.3282 | 0.3114 – 0.345 |
| kpi_wasl | 0.3168 | 0.2984 – 0.3352 |
| kpi_eid_only | 0.1274 | 0.1193 – 0.1355 |
| kpi_abs_error_wasl | 0.0114 | 0.0095 – 0.0133 |
| kpi_abs_error_eid_only | 0.2008 | 0.1917 – 0.2099 |
| linkage_pair_f1 | 0.9734 | 0.971 – 0.9758 |
| reviewer_h_per_1k | 25.7394 | 24.9226 – 26.5562 |
| review_items_per_100_loops | 73.9622 | 72.3928 – 75.5316 |
| throughput_ev_s | 754.7204 | 680.1342 – 829.3066 |
| review_items_per_100_arab | 77.5484 | 76.0701 – 79.0267 |
| review_items_per_100_filipino | 70.1138 | 63.9132 – 76.3144 |
| review_items_per_100_south_asian | 68.2606 | 63.7003 – 72.8209 |
| review_items_per_100_western | 71.6334 | 68.9177 – 74.3491 |
| reviewer_h_per_1k_arab | 26.5596 | 25.6779 – 27.4413 |
| reviewer_h_per_1k_filipino | 25.0826 | 23.4429 – 26.7223 |
| reviewer_h_per_1k_south_asian | 23.927 | 22.1586 – 25.6954 |
| reviewer_h_per_1k_western | 25.8426 | 24.9176 – 26.7676 |
| review_load_ratio_max_min_origin | 1.1562 | 1.077 – 1.2353 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2724 | 0.1350% |
| cervical_false_closures | 0 | 1012 | 0.3640% |
| wrong_patient_released | 0 | 4798 | 0.0770% |
| released_without_granted_consent | 0 | 3274 | 0.1130% |
| duplicate_or_lost_outputs | 0 | 101747 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 153118 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 120 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.862 | 0.8576 – 0.8664 |
| crc_recall_no_review | 0.1656 | 0.1368 – 0.1944 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.8872 | 0.8549 – 0.9195 |
| kpi_truth | 0.3218 | 0.3091 – 0.3345 |
| kpi_wasl | 0.2828 | 0.2714 – 0.2942 |
| kpi_eid_only | 0.121 | 0.1089 – 0.1331 |
| kpi_abs_error_wasl | 0.039 | 0.0368 – 0.0412 |
| kpi_abs_error_eid_only | 0.2008 | 0.1877 – 0.2139 |
| linkage_pair_f1 | 0.9308 | 0.9248 – 0.9368 |
| reviewer_h_per_1k | 22.2246 | 21.0128 – 23.4364 |
| review_items_per_100_loops | 63.9526 | 60.5033 – 67.4019 |
| throughput_ev_s | 717.633 | 599.9527 – 835.3134 |
| review_items_per_100_arab | 66.0534 | 62.1277 – 69.9791 |
| review_items_per_100_filipino | 61.9 | 53.5409 – 70.2591 |
| review_items_per_100_south_asian | 63.3988 | 57.6444 – 69.1532 |
| review_items_per_100_western | 58.4062 | 53.7957 – 63.0167 |
| reviewer_h_per_1k_arab | 22.7286 | 21.4791 – 23.9781 |
| reviewer_h_per_1k_filipino | 21.4784 | 18.3206 – 24.6362 |
| reviewer_h_per_1k_south_asian | 22.0866 | 19.8505 – 24.3227 |
| reviewer_h_per_1k_western | 20.9426 | 19.8171 – 22.0681 |
| review_load_ratio_max_min_origin | 1.1808 | 1.093 – 1.2685 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2468 | 0.1490% |
| cervical_false_closures | 0 | 911 | 0.4040% |
| wrong_patient_released | 0 | 4453 | 0.0830% |
| released_without_granted_consent | 0 | 3032 | 0.1220% |
| duplicate_or_lost_outputs | 0 | 99538 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 152539 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 120 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
