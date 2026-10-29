# Final frozen evaluation

Code revision `315a9147e9bd` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-28T23:36:07.035159Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.9112 | 0.8929 – 0.9295 |
| crc_recall_no_review | 0.2692 | 0.2468 – 0.2916 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9718 | 0.9472 – 0.9964 |
| kpi_truth | 0.3236 | 0.3081 – 0.3391 |
| kpi_wasl | 0.312 | 0.2986 – 0.3254 |
| kpi_eid_only | 0.1198 | 0.1121 – 0.1275 |
| kpi_abs_error_wasl | 0.0116 | 0.0086 – 0.0146 |
| kpi_abs_error_eid_only | 0.2038 | 0.185 – 0.2226 |
| linkage_pair_f1 | 0.9774 | 0.9732 – 0.9816 |
| reviewer_h_per_1k | 28.4 | 28.1681 – 28.6319 |
| review_items_per_100_loops | 81.8 | 80.1803 – 83.4197 |
| throughput_ev_s | 552.5828 | 443.0741 – 662.0915 |
| review_items_per_100_arab | 85.087 | 81.0073 – 89.1667 |
| review_items_per_100_filipino | 78.8736 | 70.9886 – 86.7586 |
| review_items_per_100_south_asian | 81.2182 | 77.6518 – 84.7846 |
| review_items_per_100_western | 73.4574 | 69.1078 – 77.807 |
| reviewer_h_per_1k_arab | 29.5712 | 28.4896 – 30.6528 |
| reviewer_h_per_1k_filipino | 27.7948 | 24.1847 – 31.4049 |
| reviewer_h_per_1k_south_asian | 28.1308 | 26.8229 – 29.4387 |
| reviewer_h_per_1k_western | 25.2678 | 22.5359 – 27.9997 |
| review_load_ratio_max_min_origin | 1.1892 | 1.0839 – 1.2946 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2633 | 0.1400% |
| cervical_false_closures | 0 | 1008 | 0.3650% |
| wrong_patient_released | 0 | 10132 | 0.0360% |
| released_without_granted_consent | 0 | 5717 | 0.0650% |
| duplicate_or_lost_outputs | 0 | 124501 | 0.0030% |
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
| crc_recall | 0.8556 | 0.8396 – 0.8716 |
| crc_recall_no_review | 0.2588 | 0.2314 – 0.2862 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9172 | 0.9107 – 0.9237 |
| kpi_truth | 0.3256 | 0.315 – 0.3362 |
| kpi_wasl | 0.2954 | 0.2863 – 0.3045 |
| kpi_eid_only | 0.1286 | 0.1205 – 0.1367 |
| kpi_abs_error_wasl | 0.0302 | 0.0256 – 0.0348 |
| kpi_abs_error_eid_only | 0.197 | 0.1872 – 0.2068 |
| linkage_pair_f1 | 0.9426 | 0.9367 – 0.9485 |
| reviewer_h_per_1k | 25.8908 | 24.911 – 26.8706 |
| review_items_per_100_loops | 73.7808 | 72.4651 – 75.0965 |
| throughput_ev_s | 613.9914 | 536.5912 – 691.3916 |
| review_items_per_100_arab | 77.214 | 74.4051 – 80.0229 |
| review_items_per_100_filipino | 72.3514 | 66.0408 – 78.662 |
| review_items_per_100_south_asian | 71.1716 | 66.2336 – 76.1096 |
| review_items_per_100_western | 67.0298 | 64.7532 – 69.3064 |
| reviewer_h_per_1k_arab | 26.7616 | 25.1743 – 28.3489 |
| reviewer_h_per_1k_filipino | 25.5014 | 22.479 – 28.5238 |
| reviewer_h_per_1k_south_asian | 25.5782 | 22.8639 – 28.2925 |
| reviewer_h_per_1k_western | 23.5922 | 22.3131 – 24.8713 |
| review_load_ratio_max_min_origin | 1.1664 | 1.1015 – 1.2313 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2487 | 0.1480% |
| cervical_false_closures | 0 | 972 | 0.3790% |
| wrong_patient_released | 0 | 9483 | 0.0390% |
| released_without_granted_consent | 0 | 5664 | 0.0650% |
| duplicate_or_lost_outputs | 0 | 120891 | 0.0030% |
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
