# Final frozen evaluation

Code revision `7f72a6e6bd0c` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T04:40:54.848163Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.808 | 0.7869 – 0.8291 |
| crc_recall_no_review | 0.1406 | 0.1174 – 0.1638 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9706 | 0.9462 – 0.995 |
| kpi_truth | 0.3172 | 0.2942 – 0.3402 |
| kpi_wasl | 0.3082 | 0.284 – 0.3324 |
| kpi_eid_only | 0.1246 | 0.1163 – 0.1329 |
| kpi_abs_error_wasl | 0.009 | 0.0067 – 0.0113 |
| kpi_abs_error_eid_only | 0.1926 | 0.1759 – 0.2093 |
| linkage_pair_f1 | 0.979 | 0.977 – 0.981 |
| reviewer_h_per_1k | 27.8408 | 27.0608 – 28.6208 |
| review_items_per_100_loops | 82.1906 | 79.957 – 84.4242 |
| throughput_ev_s | 304.677 | 284.2149 – 325.1391 |
| review_items_per_100_arab | 85.0692 | 81.0332 – 89.1052 |
| review_items_per_100_filipino | 80.4774 | 72.5833 – 88.3715 |
| review_items_per_100_south_asian | 80.538 | 75.0892 – 85.9868 |
| review_items_per_100_western | 76.2356 | 70.8313 – 81.6399 |
| reviewer_h_per_1k_arab | 28.3138 | 27.0798 – 29.5478 |
| reviewer_h_per_1k_filipino | 27.9302 | 25.1842 – 30.6762 |
| reviewer_h_per_1k_south_asian | 27.2748 | 24.8377 – 29.7119 |
| reviewer_h_per_1k_western | 27.1614 | 25.4422 – 28.8806 |
| review_load_ratio_max_min_origin | 1.165 | 1.1368 – 1.1932 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2275 | 0.1620% |
| cervical_false_closures | 0 | 1007 | 0.3660% |
| wrong_patient_released | 0 | 10033 | 0.0370% |
| released_without_granted_consent | 0 | 5885 | 0.0630% |
| duplicate_or_lost_outputs | 0 | 125055 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153102 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 110 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 0.9996 | 0.9985 – 1.0007 |
| crc_recall | 0.7312 | 0.7053 – 0.7571 |
| crc_recall_no_review | 0.1464 | 0.1392 – 0.1536 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9074 | 0.8998 – 0.915 |
| kpi_truth | 0.3222 | 0.3077 – 0.3367 |
| kpi_wasl | 0.2884 | 0.2716 – 0.3052 |
| kpi_eid_only | 0.1304 | 0.1224 – 0.1384 |
| kpi_abs_error_wasl | 0.0338 | 0.0273 – 0.0403 |
| kpi_abs_error_eid_only | 0.1918 | 0.1775 – 0.2061 |
| linkage_pair_f1 | 0.9408 | 0.9369 – 0.9447 |
| reviewer_h_per_1k | 25.1912 | 24.1664 – 26.216 |
| review_items_per_100_loops | 72.562 | 70.1152 – 75.0088 |
| throughput_ev_s | 356.2073 | 182.3169 – 530.0977 |
| review_items_per_100_arab | 74.4428 | 71.5835 – 77.3021 |
| review_items_per_100_filipino | 70.2702 | 63.2247 – 77.3157 |
| review_items_per_100_south_asian | 71.8944 | 68.5272 – 75.2616 |
| review_items_per_100_western | 68.5666 | 60.5405 – 76.5927 |
| reviewer_h_per_1k_arab | 25.9122 | 24.5756 – 27.2488 |
| reviewer_h_per_1k_filipino | 24.0452 | 20.7734 – 27.317 |
| reviewer_h_per_1k_south_asian | 25.0528 | 23.0083 – 27.0973 |
| reviewer_h_per_1k_western | 23.6318 | 21.1329 – 26.1307 |
| review_load_ratio_max_min_origin | 1.1575 | 1.0912 – 1.2239 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 1 | 2105 | 0.2640% |
| cervical_false_closures | 0 | 962 | 0.3830% |
| wrong_patient_released | 0 | 9394 | 0.0390% |
| released_without_granted_consent | 0 | 6332 | 0.0580% |
| duplicate_or_lost_outputs | 0 | 122271 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153977 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 104 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
