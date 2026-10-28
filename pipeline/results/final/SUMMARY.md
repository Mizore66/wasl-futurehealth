# Final frozen evaluation

Code revision `cdeac0a6fbcc` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-28T05:43:09.122606Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.948 | 0.9396 – 0.9564 |
| crc_recall_no_review | 0.19 | 0.1704 – 0.2096 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9716 | 0.9544 – 0.9888 |
| kpi_truth | 0.3312 | 0.3022 – 0.3602 |
| kpi_wasl | 0.3216 | 0.2952 – 0.348 |
| kpi_eid_only | 0.1238 | 0.1093 – 0.1383 |
| kpi_abs_error_wasl | 0.0096 | 0.0058 – 0.0134 |
| kpi_abs_error_eid_only | 0.2074 | 0.1857 – 0.2291 |
| linkage_pair_f1 | 0.9786 | 0.9736 – 0.9836 |
| reviewer_h_per_1k | 25.9538 | 24.9411 – 26.9665 |
| review_items_per_100_loops | 74.8094 | 71.9576 – 77.6612 |
| throughput_ev_s | 716.3953 | 673.8258 – 758.9649 |
| review_items_per_100_arab | 76.4956 | 71.821 – 81.1702 |
| review_items_per_100_filipino | 66.8624 | 64.4895 – 69.2353 |
| review_items_per_100_south_asian | 75.4912 | 72.6942 – 78.2882 |
| review_items_per_100_western | 72.675 | 68.7546 – 76.5954 |
| reviewer_h_per_1k_arab | 26.6686 | 25.0601 – 28.2771 |
| reviewer_h_per_1k_filipino | 24.3202 | 22.1142 – 26.5262 |
| reviewer_h_per_1k_south_asian | 25.8922 | 25.1077 – 26.6767 |
| reviewer_h_per_1k_western | 24.5912 | 22.9468 – 26.2356 |
| review_load_ratio_max_min_origin | 1.1662 | 1.0959 – 1.2365 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2703 | 0.1360% |
| cervical_false_closures | 0 | 1008 | 0.3650% |
| wrong_patient_released | 0 | 5523 | 0.0670% |
| released_without_granted_consent | 0 | 3846 | 0.0960% |
| duplicate_or_lost_outputs | 0 | 100576 | 0.0040% |
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
| crc_recall | 0.8782 | 0.8627 – 0.8937 |
| crc_recall_no_review | 0.1946 | 0.1789 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9114 | 0.9002 – 0.9226 |
| kpi_truth | 0.3266 | 0.3071 – 0.3461 |
| kpi_wasl | 0.2914 | 0.2745 – 0.3083 |
| kpi_eid_only | 0.1212 | 0.1107 – 0.1317 |
| kpi_abs_error_wasl | 0.0352 | 0.0268 – 0.0436 |
| kpi_abs_error_eid_only | 0.2054 | 0.1954 – 0.2154 |
| linkage_pair_f1 | 0.9424 | 0.9376 – 0.9472 |
| reviewer_h_per_1k | 23.212 | 22.0834 – 24.3406 |
| review_items_per_100_loops | 66.638 | 63.7628 – 69.5132 |
| throughput_ev_s | 697.6271 | 631.0375 – 764.2166 |
| review_items_per_100_arab | 68.4292 | 66.5729 – 70.2855 |
| review_items_per_100_filipino | 63.1696 | 57.3443 – 68.9949 |
| review_items_per_100_south_asian | 66.977 | 61.0346 – 72.9194 |
| review_items_per_100_western | 61.917 | 56.3183 – 67.5157 |
| reviewer_h_per_1k_arab | 23.8364 | 23.368 – 24.3048 |
| reviewer_h_per_1k_filipino | 21.6 | 20.12 – 23.08 |
| reviewer_h_per_1k_south_asian | 23.2498 | 20.3713 – 26.1283 |
| reviewer_h_per_1k_western | 21.9518 | 19.431 – 24.4726 |
| review_load_ratio_max_min_origin | 1.1595 | 1.0939 – 1.2251 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2532 | 0.1460% |
| cervical_false_closures | 0 | 966 | 0.3810% |
| wrong_patient_released | 0 | 5493 | 0.0670% |
| released_without_granted_consent | 0 | 3709 | 0.0990% |
| duplicate_or_lost_outputs | 0 | 99254 | 0.0040% |
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
