# Final frozen evaluation

Code revision `b11d4ffc72f6` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-28T19:54:49.995673Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 0.991 | 0.9887 – 0.9933 |
| crc_recall | 0.9136 | 0.8946 – 0.9326 |
| crc_recall_no_review | 0.1246 | 0.1153 – 0.1339 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9718 | 0.9472 – 0.9964 |
| kpi_truth | 0.3236 | 0.3081 – 0.3391 |
| kpi_wasl | 0.3118 | 0.298 – 0.3256 |
| kpi_eid_only | 0.1198 | 0.1121 – 0.1275 |
| kpi_abs_error_wasl | 0.0118 | 0.009 – 0.0146 |
| kpi_abs_error_eid_only | 0.2038 | 0.185 – 0.2226 |
| linkage_pair_f1 | 0.9772 | 0.973 – 0.9814 |
| reviewer_h_per_1k | 32.3536 | 32.1593 – 32.5479 |
| review_items_per_100_loops | 89.9524 | 88.3771 – 91.5277 |
| throughput_ev_s | 600.5567 | 546.0175 – 655.096 |
| review_items_per_100_arab | 93.4838 | 89.4157 – 97.5519 |
| review_items_per_100_filipino | 86.6438 | 79.6182 – 93.6694 |
| review_items_per_100_south_asian | 88.6258 | 85.5969 – 91.6547 |
| review_items_per_100_western | 82.2428 | 77.2062 – 87.2794 |
| reviewer_h_per_1k_arab | 33.5328 | 32.6519 – 34.4137 |
| reviewer_h_per_1k_filipino | 31.8796 | 28.7657 – 34.9935 |
| reviewer_h_per_1k_south_asian | 31.7632 | 30.9793 – 32.5471 |
| reviewer_h_per_1k_western | 29.6404 | 26.8137 – 32.4671 |
| review_load_ratio_max_min_origin | 1.1538 | 1.0408 – 1.2669 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 24 | 2664 | 1.3380% |
| cervical_false_closures | 0 | 1008 | 0.3650% |
| wrong_patient_released | 0 | 10130 | 0.0360% |
| released_without_granted_consent | 0 | 5786 | 0.0640% |
| duplicate_or_lost_outputs | 0 | 126304 | 0.0030% |
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
| crc_precision | 0.9896 | 0.9821 – 0.9971 |
| crc_recall | 0.855 | 0.833 – 0.877 |
| crc_recall_no_review | 0.1142 | 0.0898 – 0.1386 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9172 | 0.9107 – 0.9237 |
| kpi_truth | 0.3256 | 0.315 – 0.3362 |
| kpi_wasl | 0.2952 | 0.2863 – 0.3041 |
| kpi_eid_only | 0.1286 | 0.1205 – 0.1367 |
| kpi_abs_error_wasl | 0.0304 | 0.0259 – 0.0349 |
| kpi_abs_error_eid_only | 0.197 | 0.1872 – 0.2068 |
| linkage_pair_f1 | 0.9426 | 0.9367 – 0.9485 |
| reviewer_h_per_1k | 29.632 | 28.6492 – 30.6148 |
| review_items_per_100_loops | 81.562 | 80.3742 – 82.7498 |
| throughput_ev_s | 624.781 | 565.6381 – 683.9239 |
| review_items_per_100_arab | 84.9932 | 82.3383 – 87.6481 |
| review_items_per_100_filipino | 81.1524 | 74.6235 – 87.6813 |
| review_items_per_100_south_asian | 78.988 | 73.7678 – 84.2082 |
| review_items_per_100_western | 74.1398 | 70.1402 – 78.1394 |
| reviewer_h_per_1k_arab | 30.4694 | 28.9394 – 31.9994 |
| reviewer_h_per_1k_filipino | 29.5846 | 26.4108 – 32.7584 |
| reviewer_h_per_1k_south_asian | 29.3296 | 26.505 – 32.1542 |
| reviewer_h_per_1k_western | 27.2386 | 26.6656 – 27.8116 |
| review_load_ratio_max_min_origin | 1.1631 | 1.0748 – 1.2515 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 26 | 2512 | 1.5130% |
| cervical_false_closures | 0 | 972 | 0.3790% |
| wrong_patient_released | 0 | 9481 | 0.0390% |
| released_without_granted_consent | 0 | 5610 | 0.0660% |
| duplicate_or_lost_outputs | 0 | 122331 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153424 | – |
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
