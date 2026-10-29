# Final frozen evaluation

Code revision `8916058cd8a5` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T05:15:29.797785Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8766 | 0.8583 – 0.8949 |
| crc_recall_no_review | 0.1746 | 0.1626 – 0.1866 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9728 | 0.9525 – 0.9931 |
| kpi_truth | 0.3254 | 0.3123 – 0.3385 |
| kpi_wasl | 0.3152 | 0.3023 – 0.3281 |
| kpi_eid_only | 0.1216 | 0.1114 – 0.1318 |
| kpi_abs_error_wasl | 0.0102 | 0.0044 – 0.016 |
| kpi_abs_error_eid_only | 0.2038 | 0.1916 – 0.216 |
| linkage_pair_f1 | 0.9782 | 0.9748 – 0.9816 |
| reviewer_h_per_1k | 29.3208 | 27.9325 – 30.7091 |
| review_items_per_100_loops | 84.1238 | 80.112 – 88.1356 |
| throughput_ev_s | 506.3361 | 468.1254 – 544.5468 |
| review_items_per_100_arab | 86.4364 | 83.8847 – 88.9881 |
| review_items_per_100_filipino | 81.0514 | 76.8515 – 85.2513 |
| review_items_per_100_south_asian | 84.6466 | 74.3805 – 94.9127 |
| review_items_per_100_western | 77.0334 | 73.531 – 80.5358 |
| reviewer_h_per_1k_arab | 30.121 | 28.9378 – 31.3042 |
| reviewer_h_per_1k_filipino | 27.9076 | 26.0184 – 29.7968 |
| reviewer_h_per_1k_south_asian | 29.2952 | 25.4803 – 33.1101 |
| reviewer_h_per_1k_western | 27.4322 | 25.019 – 29.8454 |
| review_load_ratio_max_min_origin | 1.1609 | 1.1097 – 1.2122 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2507 | 0.1470% |
| cervical_false_closures | 0 | 1009 | 0.3650% |
| wrong_patient_released | 0 | 10095 | 0.0370% |
| released_without_granted_consent | 0 | 5716 | 0.0650% |
| duplicate_or_lost_outputs | 0 | 124738 | 0.0030% |
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
| crc_recall | 0.8054 | 0.7834 – 0.8274 |
| crc_recall_no_review | 0.1682 | 0.1418 – 0.1946 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9142 | 0.9042 – 0.9242 |
| kpi_truth | 0.3242 | 0.3096 – 0.3388 |
| kpi_wasl | 0.2932 | 0.2768 – 0.3096 |
| kpi_eid_only | 0.126 | 0.1136 – 0.1384 |
| kpi_abs_error_wasl | 0.031 | 0.0264 – 0.0356 |
| kpi_abs_error_eid_only | 0.1982 | 0.182 – 0.2144 |
| linkage_pair_f1 | 0.9444 | 0.9413 – 0.9475 |
| reviewer_h_per_1k | 26.3384 | 25.654 – 27.0228 |
| review_items_per_100_loops | 75.1618 | 73.13 – 77.1936 |
| throughput_ev_s | 518.4838 | 442.0325 – 594.9351 |
| review_items_per_100_arab | 78.9118 | 75.7219 – 82.1017 |
| review_items_per_100_filipino | 70.8052 | 63.9809 – 77.6295 |
| review_items_per_100_south_asian | 70.7224 | 66.0803 – 75.3645 |
| review_items_per_100_western | 72.4676 | 64.6285 – 80.3067 |
| reviewer_h_per_1k_arab | 27.2942 | 25.7712 – 28.8172 |
| reviewer_h_per_1k_filipino | 24.0566 | 21.8874 – 26.2258 |
| reviewer_h_per_1k_south_asian | 25.3416 | 23.5385 – 27.1447 |
| reviewer_h_per_1k_western | 26.1682 | 23.6432 – 28.6932 |
| review_load_ratio_max_min_origin | 1.1745 | 1.0797 – 1.2693 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2270 | 0.1620% |
| cervical_false_closures | 0 | 969 | 0.3800% |
| wrong_patient_released | 0 | 9363 | 0.0390% |
| released_without_granted_consent | 0 | 6076 | 0.0610% |
| duplicate_or_lost_outputs | 0 | 121643 | 0.0030% |
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
