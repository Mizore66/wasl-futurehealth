# Final frozen evaluation

Code revision `315a9147e9bd` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-28T23:09:26.058618Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8804 | 0.864 – 0.8968 |
| crc_recall_no_review | 0.3362 | 0.3176 – 0.3548 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9726 | 0.9501 – 0.9951 |
| kpi_truth | 0.3236 | 0.3081 – 0.3391 |
| kpi_wasl | 0.3124 | 0.2991 – 0.3257 |
| kpi_eid_only | 0.1198 | 0.1121 – 0.1275 |
| kpi_abs_error_wasl | 0.0112 | 0.0084 – 0.014 |
| kpi_abs_error_eid_only | 0.2038 | 0.185 – 0.2226 |
| linkage_pair_f1 | 0.9776 | 0.9735 – 0.9817 |
| reviewer_h_per_1k | 25.0836 | 24.6812 – 25.486 |
| review_items_per_100_loops | 75.505 | 73.3175 – 77.6925 |
| throughput_ev_s | 323.4282 | 312.3946 – 334.4618 |
| review_items_per_100_arab | 78.709 | 74.6139 – 82.8041 |
| review_items_per_100_filipino | 72.1772 | 64.4955 – 79.8589 |
| review_items_per_100_south_asian | 74.6014 | 71.9862 – 77.2166 |
| review_items_per_100_western | 68.1892 | 64.9807 – 71.3977 |
| reviewer_h_per_1k_arab | 26.1574 | 25.4124 – 26.9024 |
| reviewer_h_per_1k_filipino | 24.5456 | 20.8219 – 28.2693 |
| reviewer_h_per_1k_south_asian | 24.6354 | 24.0776 – 25.1932 |
| reviewer_h_per_1k_western | 22.5238 | 20.2872 – 24.7604 |
| review_load_ratio_max_min_origin | 1.1788 | 1.0782 – 1.2793 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2544 | 0.1450% |
| cervical_false_closures | 0 | 1009 | 0.3650% |
| wrong_patient_released | 0 | 10130 | 0.0360% |
| released_without_granted_consent | 0 | 5992 | 0.0620% |
| duplicate_or_lost_outputs | 0 | 124331 | 0.0030% |
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
| crc_recall | 0.8248 | 0.8088 – 0.8408 |
| crc_recall_no_review | 0.3284 | 0.2917 – 0.3651 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9172 | 0.9107 – 0.9237 |
| kpi_truth | 0.3256 | 0.315 – 0.3362 |
| kpi_wasl | 0.2952 | 0.2863 – 0.3041 |
| kpi_eid_only | 0.1286 | 0.1205 – 0.1367 |
| kpi_abs_error_wasl | 0.0304 | 0.0259 – 0.0349 |
| kpi_abs_error_eid_only | 0.197 | 0.1872 – 0.2068 |
| linkage_pair_f1 | 0.9426 | 0.9367 – 0.9485 |
| reviewer_h_per_1k | 22.7818 | 21.9293 – 23.6343 |
| review_items_per_100_loops | 67.8476 | 66.4254 – 69.2698 |
| throughput_ev_s | 357.9485 | 242.1541 – 473.7429 |
| review_items_per_100_arab | 71.2118 | 68.145 – 74.2786 |
| review_items_per_100_filipino | 65.9782 | 60.3377 – 71.6187 |
| review_items_per_100_south_asian | 65.3682 | 60.5598 – 70.1766 |
| review_items_per_100_western | 61.4028 | 59.5173 – 63.2883 |
| reviewer_h_per_1k_arab | 23.6892 | 22.2537 – 25.1247 |
| reviewer_h_per_1k_filipino | 22.1422 | 19.5509 – 24.7335 |
| reviewer_h_per_1k_south_asian | 22.4018 | 20.1849 – 24.6187 |
| reviewer_h_per_1k_western | 20.6348 | 19.6164 – 21.6532 |
| review_load_ratio_max_min_origin | 1.1784 | 1.1262 – 1.2306 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2398 | 0.1540% |
| cervical_false_closures | 0 | 972 | 0.3790% |
| wrong_patient_released | 0 | 9487 | 0.0390% |
| released_without_granted_consent | 0 | 5311 | 0.0690% |
| duplicate_or_lost_outputs | 0 | 119237 | 0.0030% |
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
