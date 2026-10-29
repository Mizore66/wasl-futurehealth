# Final frozen evaluation

Code revision `54ff4bacb101` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T08:09:35.236646Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.9178 | 0.9136 – 0.922 |
| crc_recall_no_review | 0.1904 | 0.1705 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9676 | 0.9524 – 0.9828 |
| kpi_truth | 0.3312 | 0.3022 – 0.3602 |
| kpi_wasl | 0.3214 | 0.2928 – 0.35 |
| kpi_eid_only | 0.1238 | 0.1093 – 0.1383 |
| kpi_abs_error_wasl | 0.0098 | 0.0074 – 0.0122 |
| kpi_abs_error_eid_only | 0.2074 | 0.1857 – 0.2291 |
| linkage_pair_f1 | 0.9784 | 0.9736 – 0.9832 |
| reviewer_h_per_1k | 28.3274 | 27.4359 – 29.2189 |
| review_items_per_100_loops | 82.8 | 80.1705 – 85.4295 |
| throughput_ev_s | 527.4239 | 484.6022 – 570.2457 |
| review_items_per_100_arab | 85.2164 | 81.1399 – 89.2929 |
| review_items_per_100_filipino | 74.5068 | 73.3709 – 75.6427 |
| review_items_per_100_south_asian | 83.4652 | 80.6214 – 86.309 |
| review_items_per_100_western | 78.3212 | 74.3741 – 82.2683 |
| reviewer_h_per_1k_arab | 29.2568 | 27.8196 – 30.694 |
| reviewer_h_per_1k_filipino | 26.3106 | 24.1803 – 28.4409 |
| reviewer_h_per_1k_south_asian | 28.2858 | 27.3074 – 29.2642 |
| reviewer_h_per_1k_western | 26.3978 | 24.6739 – 28.1217 |
| review_load_ratio_max_min_origin | 1.1497 | 1.0864 – 1.2131 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2617 | 0.1410% |
| cervical_false_closures | 0 | 1004 | 0.3670% |
| wrong_patient_released | 0 | 10089 | 0.0370% |
| released_without_granted_consent | 0 | 5547 | 0.0660% |
| duplicate_or_lost_outputs | 0 | 124325 | 0.0030% |
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
| crc_recall | 0.8506 | 0.8301 – 0.8711 |
| crc_recall_no_review | 0.1946 | 0.1789 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9142 | 0.9031 – 0.9253 |
| kpi_truth | 0.3266 | 0.3071 – 0.3461 |
| kpi_wasl | 0.2916 | 0.2742 – 0.309 |
| kpi_eid_only | 0.1212 | 0.1107 – 0.1317 |
| kpi_abs_error_wasl | 0.035 | 0.026 – 0.044 |
| kpi_abs_error_eid_only | 0.2054 | 0.1954 – 0.2154 |
| linkage_pair_f1 | 0.942 | 0.9376 – 0.9464 |
| reviewer_h_per_1k | 25.453 | 24.7829 – 26.1231 |
| review_items_per_100_loops | 74.1142 | 72.2295 – 75.9989 |
| throughput_ev_s | 535.968 | 472.472 – 599.464 |
| review_items_per_100_arab | 77.3328 | 76.0209 – 78.6447 |
| review_items_per_100_filipino | 67.3474 | 61.3486 – 73.3462 |
| review_items_per_100_south_asian | 73.5948 | 67.9748 – 79.2148 |
| review_items_per_100_western | 67.8668 | 65.0603 – 70.6733 |
| reviewer_h_per_1k_arab | 26.5402 | 26.0304 – 27.05 |
| reviewer_h_per_1k_filipino | 22.934 | 21.779 – 24.089 |
| reviewer_h_per_1k_south_asian | 25.2034 | 22.5367 – 27.8701 |
| reviewer_h_per_1k_western | 23.6236 | 22.3335 – 24.9137 |
| review_load_ratio_max_min_origin | 1.1783 | 1.1123 – 1.2443 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2452 | 0.1500% |
| cervical_false_closures | 0 | 969 | 0.3800% |
| wrong_patient_released | 0 | 9410 | 0.0390% |
| released_without_granted_consent | 0 | 5315 | 0.0690% |
| duplicate_or_lost_outputs | 0 | 120483 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153692 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 111 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 1 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
