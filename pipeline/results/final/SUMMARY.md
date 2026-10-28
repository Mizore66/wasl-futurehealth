# Final frozen evaluation

Code revision `cab895f295fc` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-28T02:11:37.720931Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.9466 | 0.9344 – 0.9588 |
| crc_recall_no_review | 0.1956 | 0.1723 – 0.2189 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9616 | 0.9396 – 0.9836 |
| kpi_truth | 0.3282 | 0.3114 – 0.345 |
| kpi_wasl | 0.3172 | 0.2984 – 0.336 |
| kpi_eid_only | 0.1274 | 0.1193 – 0.1355 |
| kpi_abs_error_wasl | 0.011 | 0.0088 – 0.0132 |
| kpi_abs_error_eid_only | 0.2008 | 0.1917 – 0.2099 |
| linkage_pair_f1 | 0.9736 | 0.9713 – 0.9759 |
| reviewer_h_per_1k | 24.9774 | 24.2319 – 25.7229 |
| review_items_per_100_loops | 71.5238 | 69.8614 – 73.1862 |
| throughput_ev_s | 708.7567 | 674.5917 – 742.9218 |
| review_items_per_100_arab | 74.0894 | 71.8033 – 76.3755 |
| review_items_per_100_filipino | 68.8218 | 62.4357 – 75.2079 |
| review_items_per_100_south_asian | 66.3672 | 62.178 – 70.5564 |
| review_items_per_100_western | 71.3932 | 68.4381 – 74.3483 |
| reviewer_h_per_1k_arab | 25.4982 | 24.4209 – 26.5755 |
| reviewer_h_per_1k_filipino | 24.812 | 23.0757 – 26.5483 |
| reviewer_h_per_1k_south_asian | 23.2728 | 21.8028 – 24.7428 |
| reviewer_h_per_1k_western | 25.7324 | 24.7038 – 26.761 |
| review_load_ratio_max_min_origin | 1.145 | 1.0886 – 1.2015 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2728 | 0.1350% |
| cervical_false_closures | 0 | 1014 | 0.3630% |
| wrong_patient_released | 0 | 4904 | 0.0750% |
| released_without_granted_consent | 0 | 3278 | 0.1120% |
| duplicate_or_lost_outputs | 0 | 101413 | 0.0040% |
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
| crc_recall | 0.8644 | 0.8592 – 0.8696 |
| crc_recall_no_review | 0.1838 | 0.1498 – 0.2178 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.8872 | 0.8549 – 0.9195 |
| kpi_truth | 0.3218 | 0.3091 – 0.3345 |
| kpi_wasl | 0.2834 | 0.2723 – 0.2945 |
| kpi_eid_only | 0.121 | 0.1089 – 0.1331 |
| kpi_abs_error_wasl | 0.0384 | 0.036 – 0.0408 |
| kpi_abs_error_eid_only | 0.2008 | 0.1877 – 0.2139 |
| linkage_pair_f1 | 0.9314 | 0.9258 – 0.937 |
| reviewer_h_per_1k | 20.8076 | 19.8425 – 21.7727 |
| review_items_per_100_loops | 59.2954 | 57.1731 – 61.4177 |
| throughput_ev_s | 641.0756 | 549.1128 – 733.0384 |
| review_items_per_100_arab | 60.2736 | 56.9286 – 63.6186 |
| review_items_per_100_filipino | 59.0822 | 52.9024 – 65.262 |
| review_items_per_100_south_asian | 59.1368 | 54.7497 – 63.5239 |
| review_items_per_100_western | 56.1012 | 51.9471 – 60.2553 |
| reviewer_h_per_1k_arab | 20.9726 | 19.8944 – 22.0508 |
| reviewer_h_per_1k_filipino | 20.6882 | 18.1336 – 23.2428 |
| reviewer_h_per_1k_south_asian | 20.8006 | 18.7785 – 22.8227 |
| reviewer_h_per_1k_western | 20.187 | 19.1161 – 21.2579 |
| review_load_ratio_max_min_origin | 1.1431 | 1.0744 – 1.2118 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2475 | 0.1490% |
| cervical_false_closures | 0 | 911 | 0.4040% |
| wrong_patient_released | 0 | 3686 | 0.1000% |
| released_without_granted_consent | 0 | 2650 | 0.1390% |
| duplicate_or_lost_outputs | 0 | 95868 | 0.0040% |
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
