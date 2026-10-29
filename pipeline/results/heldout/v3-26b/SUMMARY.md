# Final frozen evaluation

Code revision `ab6daa2463ae` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T02:46:15.611587Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.6928 | 0.6814 – 0.7042 |
| crc_recall_no_review | 0.1366 | 0.1231 – 0.1501 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9648 | 0.947 – 0.9826 |
| kpi_truth | 0.3258 | 0.3131 – 0.3385 |
| kpi_wasl | 0.3156 | 0.3022 – 0.329 |
| kpi_eid_only | 0.1216 | 0.1089 – 0.1343 |
| kpi_abs_error_wasl | 0.0102 | 0.008 – 0.0124 |
| kpi_abs_error_eid_only | 0.2042 | 0.1935 – 0.2149 |
| linkage_pair_f1 | 0.9782 | 0.9743 – 0.9821 |
| reviewer_h_per_1k | 26.7108 | 25.6059 – 27.8157 |
| review_items_per_100_loops | 79.2664 | 77.1946 – 81.3382 |
| throughput_ev_s | 333.9812 | 325.9624 – 342.0 |
| review_items_per_100_arab | 81.2088 | 78.9014 – 83.5162 |
| review_items_per_100_filipino | 74.4894 | 66.5956 – 82.3832 |
| review_items_per_100_south_asian | 79.9908 | 75.5601 – 84.4215 |
| review_items_per_100_western | 74.143 | 68.9551 – 79.3309 |
| reviewer_h_per_1k_arab | 27.2654 | 26.3864 – 28.1444 |
| reviewer_h_per_1k_filipino | 25.43 | 21.7637 – 29.0963 |
| reviewer_h_per_1k_south_asian | 26.5822 | 24.9701 – 28.1943 |
| reviewer_h_per_1k_western | 25.7732 | 23.2178 – 28.3286 |
| review_load_ratio_max_min_origin | 1.1347 | 1.0406 – 1.2289 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1994 | 0.1850% |
| cervical_false_closures | 0 | 1001 | 0.3680% |
| wrong_patient_released | 0 | 10183 | 0.0360% |
| released_without_granted_consent | 0 | 5813 | 0.0630% |
| duplicate_or_lost_outputs | 0 | 124586 | 0.0030% |
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
| crc_recall | 0.6476 | 0.6268 – 0.6684 |
| crc_recall_no_review | 0.1448 | 0.1142 – 0.1754 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9142 | 0.9035 – 0.9249 |
| kpi_truth | 0.3252 | 0.3098 – 0.3406 |
| kpi_wasl | 0.2946 | 0.2787 – 0.3105 |
| kpi_eid_only | 0.1318 | 0.1197 – 0.1439 |
| kpi_abs_error_wasl | 0.0306 | 0.0279 – 0.0333 |
| kpi_abs_error_eid_only | 0.1934 | 0.1814 – 0.2054 |
| linkage_pair_f1 | 0.9422 | 0.9381 – 0.9463 |
| reviewer_h_per_1k | 24.527 | 23.1085 – 25.9455 |
| review_items_per_100_loops | 71.9904 | 69.0171 – 74.9637 |
| throughput_ev_s | 413.5377 | 211.9033 – 615.1722 |
| review_items_per_100_arab | 76.2022 | 73.5058 – 78.8986 |
| review_items_per_100_filipino | 66.4136 | 59.7509 – 73.0763 |
| review_items_per_100_south_asian | 68.33 | 65.0247 – 71.6353 |
| review_items_per_100_western | 66.947 | 60.4215 – 73.4725 |
| reviewer_h_per_1k_arab | 25.9612 | 24.7354 – 27.187 |
| reviewer_h_per_1k_filipino | 22.5948 | 19.9099 – 25.2797 |
| reviewer_h_per_1k_south_asian | 22.868 | 21.3819 – 24.3541 |
| reviewer_h_per_1k_western | 23.524 | 20.3066 – 26.7414 |
| review_load_ratio_max_min_origin | 1.1851 | 1.1235 – 1.2468 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1867 | 0.1970% |
| cervical_false_closures | 0 | 969 | 0.3800% |
| wrong_patient_released | 0 | 9430 | 0.0390% |
| released_without_granted_consent | 0 | 5298 | 0.0700% |
| duplicate_or_lost_outputs | 0 | 120485 | 0.0030% |
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
