# Final frozen evaluation

Code revision `8916058cd8a5` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T05:25:28.710784Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.9468 | 0.9395 – 0.9541 |
| crc_recall_no_review | 0.1904 | 0.1705 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9686 | 0.9511 – 0.9861 |
| kpi_truth | 0.3312 | 0.3022 – 0.3602 |
| kpi_wasl | 0.3212 | 0.293 – 0.3494 |
| kpi_eid_only | 0.1238 | 0.1093 – 0.1383 |
| kpi_abs_error_wasl | 0.01 | 0.0074 – 0.0126 |
| kpi_abs_error_eid_only | 0.2074 | 0.1857 – 0.2291 |
| linkage_pair_f1 | 0.9786 | 0.9736 – 0.9836 |
| reviewer_h_per_1k | 28.6388 | 27.6032 – 29.6744 |
| review_items_per_100_loops | 83.7524 | 80.5763 – 86.9285 |
| throughput_ev_s | 293.5679 | 279.0845 – 308.0514 |
| review_items_per_100_arab | 86.1096 | 81.368 – 90.8512 |
| review_items_per_100_filipino | 75.439 | 74.1745 – 76.7035 |
| review_items_per_100_south_asian | 84.5226 | 80.9266 – 88.1186 |
| review_items_per_100_western | 79.2894 | 75.856 – 82.7228 |
| reviewer_h_per_1k_arab | 29.582 | 27.9472 – 31.2168 |
| reviewer_h_per_1k_filipino | 26.7186 | 24.4169 – 29.0203 |
| reviewer_h_per_1k_south_asian | 28.5942 | 27.3231 – 29.8653 |
| reviewer_h_per_1k_western | 26.6044 | 24.8021 – 28.4067 |
| review_load_ratio_max_min_origin | 1.1447 | 1.0698 – 1.2196 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2700 | 0.1370% |
| cervical_false_closures | 0 | 1005 | 0.3660% |
| wrong_patient_released | 0 | 10048 | 0.0370% |
| released_without_granted_consent | 0 | 6724 | 0.0550% |
| duplicate_or_lost_outputs | 0 | 127008 | 0.0030% |
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
| crc_recall | 0.8794 | 0.863 – 0.8958 |
| crc_recall_no_review | 0.1946 | 0.1789 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9142 | 0.904 – 0.9244 |
| kpi_truth | 0.3266 | 0.3071 – 0.3461 |
| kpi_wasl | 0.2914 | 0.2744 – 0.3084 |
| kpi_eid_only | 0.1212 | 0.1107 – 0.1317 |
| kpi_abs_error_wasl | 0.0352 | 0.0265 – 0.0439 |
| kpi_abs_error_eid_only | 0.2054 | 0.1954 – 0.2154 |
| linkage_pair_f1 | 0.9422 | 0.9371 – 0.9473 |
| reviewer_h_per_1k | 25.8852 | 25.1736 – 26.5968 |
| review_items_per_100_loops | 75.4574 | 73.3174 – 77.5974 |
| throughput_ev_s | 392.9553 | 236.292 – 549.6186 |
| review_items_per_100_arab | 78.561 | 77.0653 – 80.0567 |
| review_items_per_100_filipino | 69.2216 | 61.8993 – 76.5439 |
| review_items_per_100_south_asian | 75.1464 | 69.4556 – 80.8372 |
| review_items_per_100_western | 68.9254 | 66.6238 – 71.227 |
| reviewer_h_per_1k_arab | 26.9268 | 26.3955 – 27.4581 |
| reviewer_h_per_1k_filipino | 23.5284 | 21.7105 – 25.3463 |
| reviewer_h_per_1k_south_asian | 25.7812 | 23.2015 – 28.3609 |
| reviewer_h_per_1k_western | 23.8642 | 22.5808 – 25.1476 |
| review_load_ratio_max_min_origin | 1.1818 | 1.1096 – 1.254 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2534 | 0.1450% |
| cervical_false_closures | 0 | 969 | 0.3800% |
| wrong_patient_released | 0 | 9386 | 0.0390% |
| released_without_granted_consent | 0 | 6285 | 0.0590% |
| duplicate_or_lost_outputs | 0 | 122857 | 0.0030% |
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
