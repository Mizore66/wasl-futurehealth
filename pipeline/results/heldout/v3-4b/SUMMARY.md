# Final frozen evaluation

Code revision `ab6daa2463ae` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T02:46:15.596424Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.5806 | 0.5546 – 0.6066 |
| crc_recall_no_review | 0.0548 | 0.0428 – 0.0668 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9668 | 0.9475 – 0.9861 |
| kpi_truth | 0.3258 | 0.3131 – 0.3385 |
| kpi_wasl | 0.3156 | 0.3022 – 0.329 |
| kpi_eid_only | 0.1216 | 0.1089 – 0.1343 |
| kpi_abs_error_wasl | 0.0102 | 0.008 – 0.0124 |
| kpi_abs_error_eid_only | 0.2042 | 0.1935 – 0.2149 |
| linkage_pair_f1 | 0.9782 | 0.9743 – 0.9821 |
| reviewer_h_per_1k | 28.4924 | 27.3854 – 29.5994 |
| review_items_per_100_loops | 82.0286 | 79.9301 – 84.1271 |
| throughput_ev_s | 339.1508 | 313.5536 – 364.7479 |
| review_items_per_100_arab | 84.3964 | 81.5134 – 87.2794 |
| review_items_per_100_filipino | 77.5328 | 69.249 – 85.8166 |
| review_items_per_100_south_asian | 82.102 | 78.4818 – 85.7222 |
| review_items_per_100_western | 76.4126 | 69.0087 – 83.8165 |
| reviewer_h_per_1k_arab | 29.3196 | 28.1765 – 30.4627 |
| reviewer_h_per_1k_filipino | 27.346 | 23.5938 – 31.0982 |
| reviewer_h_per_1k_south_asian | 28.025 | 26.9177 – 29.1323 |
| reviewer_h_per_1k_western | 27.1196 | 23.8502 – 30.389 |
| review_load_ratio_max_min_origin | 1.1384 | 1.0111 – 1.2657 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1670 | 0.2210% |
| cervical_false_closures | 0 | 1003 | 0.3670% |
| wrong_patient_released | 0 | 10192 | 0.0360% |
| released_without_granted_consent | 0 | 5873 | 0.0630% |
| duplicate_or_lost_outputs | 0 | 125414 | 0.0030% |
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
| crc_recall | 0.5494 | 0.5294 – 0.5694 |
| crc_recall_no_review | 0.0656 | 0.0486 – 0.0826 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9124 | 0.9046 – 0.9202 |
| kpi_truth | 0.3252 | 0.3098 – 0.3406 |
| kpi_wasl | 0.2946 | 0.2787 – 0.3105 |
| kpi_eid_only | 0.1318 | 0.1197 – 0.1439 |
| kpi_abs_error_wasl | 0.0306 | 0.0279 – 0.0333 |
| kpi_abs_error_eid_only | 0.1934 | 0.1814 – 0.2054 |
| linkage_pair_f1 | 0.9422 | 0.9381 – 0.9463 |
| reviewer_h_per_1k | 26.0668 | 24.4822 – 27.6514 |
| review_items_per_100_loops | 74.5616 | 71.3859 – 77.7373 |
| throughput_ev_s | 394.3771 | 241.8509 – 546.9033 |
| review_items_per_100_arab | 79.141 | 75.6971 – 82.5849 |
| review_items_per_100_filipino | 70.6668 | 64.0224 – 77.3112 |
| review_items_per_100_south_asian | 71.0258 | 67.0193 – 75.0323 |
| review_items_per_100_western | 66.9734 | 59.1267 – 74.8201 |
| reviewer_h_per_1k_arab | 27.7258 | 25.9403 – 29.5113 |
| reviewer_h_per_1k_filipino | 24.4332 | 22.8871 – 25.9793 |
| reviewer_h_per_1k_south_asian | 24.3624 | 22.8096 – 25.9152 |
| reviewer_h_per_1k_western | 24.1768 | 20.4045 – 27.9491 |
| review_load_ratio_max_min_origin | 1.2158 | 1.144 – 1.2875 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1584 | 0.2330% |
| cervical_false_closures | 0 | 967 | 0.3810% |
| wrong_patient_released | 0 | 9432 | 0.0390% |
| released_without_granted_consent | 0 | 5254 | 0.0700% |
| duplicate_or_lost_outputs | 0 | 120854 | 0.0030% |
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
