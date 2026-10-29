# Final frozen evaluation

Code revision `7962054f99fb` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T03:13:29.672027Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.6758 | 0.6567 – 0.6949 |
| crc_recall_no_review | 0.08 | 0.0649 – 0.0951 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9648 | 0.947 – 0.9826 |
| kpi_truth | 0.3258 | 0.3131 – 0.3385 |
| kpi_wasl | 0.3156 | 0.3022 – 0.329 |
| kpi_eid_only | 0.1216 | 0.1089 – 0.1343 |
| kpi_abs_error_wasl | 0.0102 | 0.008 – 0.0124 |
| kpi_abs_error_eid_only | 0.2042 | 0.1935 – 0.2149 |
| linkage_pair_f1 | 0.978 | 0.974 – 0.982 |
| reviewer_h_per_1k | 29.3832 | 28.1311 – 30.6353 |
| review_items_per_100_loops | 83.781 | 81.5004 – 86.0616 |
| throughput_ev_s | 354.9971 | 336.3205 – 373.6737 |
| review_items_per_100_arab | 86.1738 | 83.5532 – 88.7944 |
| review_items_per_100_filipino | 79.3308 | 70.2884 – 88.3732 |
| review_items_per_100_south_asian | 83.9818 | 80.1636 – 87.8 |
| review_items_per_100_western | 77.814 | 69.2374 – 86.3906 |
| reviewer_h_per_1k_arab | 30.1976 | 29.0858 – 31.3094 |
| reviewer_h_per_1k_filipino | 28.213 | 23.9611 – 32.4649 |
| reviewer_h_per_1k_south_asian | 28.992 | 27.7199 – 30.2641 |
| reviewer_h_per_1k_western | 27.9328 | 24.055 – 31.8106 |
| review_load_ratio_max_min_origin | 1.147 | 1.0072 – 1.2869 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1945 | 0.1890% |
| cervical_false_closures | 0 | 1001 | 0.3680% |
| wrong_patient_released | 0 | 10191 | 0.0360% |
| released_without_granted_consent | 0 | 5527 | 0.0670% |
| duplicate_or_lost_outputs | 0 | 125075 | 0.0030% |
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
| crc_recall | 0.6372 | 0.6175 – 0.6569 |
| crc_recall_no_review | 0.0866 | 0.0648 – 0.1084 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9142 | 0.9035 – 0.9249 |
| kpi_truth | 0.3252 | 0.3098 – 0.3406 |
| kpi_wasl | 0.2946 | 0.2787 – 0.3105 |
| kpi_eid_only | 0.1318 | 0.1197 – 0.1439 |
| kpi_abs_error_wasl | 0.0306 | 0.0279 – 0.0333 |
| kpi_abs_error_eid_only | 0.1934 | 0.1814 – 0.2054 |
| linkage_pair_f1 | 0.9424 | 0.9382 – 0.9466 |
| reviewer_h_per_1k | 26.838 | 25.4415 – 28.2345 |
| review_items_per_100_loops | 76.0382 | 73.2744 – 78.802 |
| throughput_ev_s | 412.7412 | 204.2467 – 621.2358 |
| review_items_per_100_arab | 80.5542 | 77.3002 – 83.8082 |
| review_items_per_100_filipino | 72.551 | 65.6356 – 79.4664 |
| review_items_per_100_south_asian | 72.3536 | 68.6304 – 76.0768 |
| review_items_per_100_western | 68.6972 | 62.0144 – 75.38 |
| reviewer_h_per_1k_arab | 28.4372 | 26.5874 – 30.287 |
| reviewer_h_per_1k_filipino | 25.5326 | 23.781 – 27.2842 |
| reviewer_h_per_1k_south_asian | 25.0652 | 23.9496 – 26.1808 |
| reviewer_h_per_1k_western | 25.086 | 21.6775 – 28.4945 |
| review_load_ratio_max_min_origin | 1.2053 | 1.1405 – 1.2701 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1837 | 0.2010% |
| cervical_false_closures | 0 | 969 | 0.3800% |
| wrong_patient_released | 0 | 9421 | 0.0390% |
| released_without_granted_consent | 0 | 5558 | 0.0660% |
| duplicate_or_lost_outputs | 0 | 121678 | 0.0030% |
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
