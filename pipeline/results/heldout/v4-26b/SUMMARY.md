# Final frozen evaluation

Code revision `7962054f99fb` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T03:13:29.687044Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.817 | 0.8032 – 0.8308 |
| crc_recall_no_review | 0.1924 | 0.1844 – 0.2004 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9668 | 0.9475 – 0.9861 |
| kpi_truth | 0.3258 | 0.3131 – 0.3385 |
| kpi_wasl | 0.3156 | 0.3022 – 0.329 |
| kpi_eid_only | 0.1216 | 0.1089 – 0.1343 |
| kpi_abs_error_wasl | 0.0102 | 0.008 – 0.0124 |
| kpi_abs_error_eid_only | 0.2042 | 0.1935 – 0.2149 |
| linkage_pair_f1 | 0.9784 | 0.9742 – 0.9826 |
| reviewer_h_per_1k | 27.286 | 26.0975 – 28.4745 |
| review_items_per_100_loops | 80.1524 | 77.9367 – 82.3681 |
| throughput_ev_s | 349.8871 | 322.5516 – 377.2225 |
| review_items_per_100_arab | 82.115 | 80.3163 – 83.9137 |
| review_items_per_100_filipino | 75.4008 | 66.8182 – 83.9834 |
| review_items_per_100_south_asian | 80.8956 | 76.4298 – 85.3614 |
| review_items_per_100_western | 74.8636 | 69.7117 – 80.0155 |
| reviewer_h_per_1k_arab | 27.8724 | 26.9629 – 28.7819 |
| reviewer_h_per_1k_filipino | 25.832 | 21.8176 – 29.8464 |
| reviewer_h_per_1k_south_asian | 27.0356 | 25.3928 – 28.6784 |
| reviewer_h_per_1k_western | 26.5348 | 23.9516 – 29.118 |
| review_load_ratio_max_min_origin | 1.1341 | 1.0405 – 1.2277 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2352 | 0.1570% |
| cervical_false_closures | 0 | 1003 | 0.3670% |
| wrong_patient_released | 0 | 10186 | 0.0360% |
| released_without_granted_consent | 0 | 6027 | 0.0610% |
| duplicate_or_lost_outputs | 0 | 125355 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153442 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 112 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 4 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.757 | 0.7438 – 0.7702 |
| crc_recall_no_review | 0.1942 | 0.1623 – 0.2261 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9142 | 0.9033 – 0.9251 |
| kpi_truth | 0.3252 | 0.3098 – 0.3406 |
| kpi_wasl | 0.2946 | 0.2787 – 0.3105 |
| kpi_eid_only | 0.1318 | 0.1197 – 0.1439 |
| kpi_abs_error_wasl | 0.0306 | 0.0279 – 0.0333 |
| kpi_abs_error_eid_only | 0.1934 | 0.1814 – 0.2054 |
| linkage_pair_f1 | 0.9422 | 0.9381 – 0.9463 |
| reviewer_h_per_1k | 25.0312 | 23.856 – 26.2064 |
| review_items_per_100_loops | 72.9236 | 70.4484 – 75.3988 |
| throughput_ev_s | 385.4475 | 273.4541 – 497.441 |
| review_items_per_100_arab | 77.128 | 75.0221 – 79.2339 |
| review_items_per_100_filipino | 67.0294 | 62.396 – 71.6628 |
| review_items_per_100_south_asian | 69.5838 | 65.8208 – 73.3468 |
| review_items_per_100_western | 67.5338 | 61.0831 – 73.9845 |
| reviewer_h_per_1k_arab | 26.4324 | 25.4841 – 27.3807 |
| reviewer_h_per_1k_filipino | 23.1894 | 20.9011 – 25.4777 |
| reviewer_h_per_1k_south_asian | 23.4536 | 22.217 – 24.6902 |
| reviewer_h_per_1k_western | 23.9444 | 20.6321 – 27.2567 |
| review_load_ratio_max_min_origin | 1.1772 | 1.1123 – 1.2421 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2182 | 0.1690% |
| cervical_false_closures | 0 | 969 | 0.3800% |
| wrong_patient_released | 0 | 9423 | 0.0390% |
| released_without_granted_consent | 0 | 5554 | 0.0660% |
| duplicate_or_lost_outputs | 0 | 121077 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153841 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 114 | – |
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
