# Final frozen evaluation

Code revision `7962054f99fb` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T03:39:03.407889Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8792 | 0.8623 – 0.8961 |
| crc_recall_no_review | 0.3278 | 0.3116 – 0.344 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9718 | 0.9472 – 0.9964 |
| kpi_truth | 0.3236 | 0.3081 – 0.3391 |
| kpi_wasl | 0.3122 | 0.2989 – 0.3255 |
| kpi_eid_only | 0.1198 | 0.1121 – 0.1275 |
| kpi_abs_error_wasl | 0.0114 | 0.0085 – 0.0143 |
| kpi_abs_error_eid_only | 0.2038 | 0.185 – 0.2226 |
| linkage_pair_f1 | 0.9774 | 0.9732 – 0.9816 |
| reviewer_h_per_1k | 25.237 | 24.9493 – 25.5247 |
| review_items_per_100_loops | 75.762 | 73.9557 – 77.5683 |
| throughput_ev_s | 263.0156 | 177.0638 – 348.9673 |
| review_items_per_100_arab | 79.1708 | 75.607 – 82.7346 |
| review_items_per_100_filipino | 71.9034 | 65.4624 – 78.3444 |
| review_items_per_100_south_asian | 74.9056 | 72.1479 – 77.6633 |
| review_items_per_100_western | 67.94 | 64.1199 – 71.7601 |
| reviewer_h_per_1k_arab | 26.3716 | 25.7866 – 26.9566 |
| reviewer_h_per_1k_filipino | 24.4324 | 21.3329 – 27.5319 |
| reviewer_h_per_1k_south_asian | 24.826 | 24.1344 – 25.5176 |
| reviewer_h_per_1k_western | 22.5508 | 20.435 – 24.6666 |
| review_load_ratio_max_min_origin | 1.1879 | 1.0841 – 1.2918 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2541 | 0.1450% |
| cervical_false_closures | 0 | 1008 | 0.3650% |
| wrong_patient_released | 0 | 10107 | 0.0360% |
| released_without_granted_consent | 0 | 6328 | 0.0580% |
| duplicate_or_lost_outputs | 0 | 124306 | 0.0030% |
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
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8246 | 0.807 – 0.8422 |
| crc_recall_no_review | 0.3232 | 0.2869 – 0.3595 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9182 | 0.9114 – 0.925 |
| kpi_truth | 0.3256 | 0.315 – 0.3362 |
| kpi_wasl | 0.2954 | 0.2866 – 0.3042 |
| kpi_eid_only | 0.1286 | 0.1205 – 0.1367 |
| kpi_abs_error_wasl | 0.0302 | 0.0256 – 0.0348 |
| kpi_abs_error_eid_only | 0.197 | 0.1872 – 0.2068 |
| linkage_pair_f1 | 0.9426 | 0.9367 – 0.9485 |
| reviewer_h_per_1k | 22.8454 | 21.9333 – 23.7575 |
| review_items_per_100_loops | 67.9332 | 66.2081 – 69.6583 |
| throughput_ev_s | 328.315 | 155.1557 – 501.4743 |
| review_items_per_100_arab | 71.2696 | 67.8036 – 74.7356 |
| review_items_per_100_filipino | 65.884 | 60.3387 – 71.4293 |
| review_items_per_100_south_asian | 65.4796 | 60.9729 – 69.9863 |
| review_items_per_100_western | 61.6584 | 59.423 – 63.8938 |
| reviewer_h_per_1k_arab | 23.7534 | 22.186 – 25.3208 |
| reviewer_h_per_1k_filipino | 22.1064 | 19.4984 – 24.7144 |
| reviewer_h_per_1k_south_asian | 22.5094 | 20.421 – 24.5978 |
| reviewer_h_per_1k_western | 20.6832 | 19.6537 – 21.7127 |
| review_load_ratio_max_min_origin | 1.1701 | 1.1151 – 1.2252 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2397 | 0.1540% |
| cervical_false_closures | 0 | 973 | 0.3780% |
| wrong_patient_released | 0 | 9455 | 0.0390% |
| released_without_granted_consent | 0 | 6372 | 0.0580% |
| duplicate_or_lost_outputs | 0 | 120924 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153424 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 111 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 2 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
