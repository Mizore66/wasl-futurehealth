# Final frozen evaluation

Code revision `1c6dcbcb9f48` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T06:34:36.256711Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 0.9976 | 0.9945 – 1.0007 |
| crc_recall | 0.7902 | 0.7746 – 0.8058 |
| crc_recall_no_review | 0.0972 | 0.0884 – 0.106 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9688 | 0.9472 – 0.9904 |
| kpi_truth | 0.3148 | 0.2953 – 0.3343 |
| kpi_wasl | 0.3042 | 0.2868 – 0.3216 |
| kpi_eid_only | 0.118 | 0.1009 – 0.1351 |
| kpi_abs_error_wasl | 0.0106 | 0.0079 – 0.0133 |
| kpi_abs_error_eid_only | 0.1968 | 0.1772 – 0.2164 |
| linkage_pair_f1 | 0.9774 | 0.9734 – 0.9814 |
| reviewer_h_per_1k | 29.9882 | 29.3419 – 30.6345 |
| review_items_per_100_loops | 84.7426 | 82.4038 – 87.0814 |
| throughput_ev_s | 531.1799 | 457.5401 – 604.8197 |
| review_items_per_100_arab | 87.6852 | 82.1181 – 93.2523 |
| review_items_per_100_filipino | 78.4162 | 75.5531 – 81.2793 |
| review_items_per_100_south_asian | 84.84 | 82.003 – 87.677 |
| review_items_per_100_western | 78.3722 | 74.499 – 82.2454 |
| reviewer_h_per_1k_arab | 31.015 | 28.8264 – 33.2036 |
| reviewer_h_per_1k_filipino | 27.5734 | 24.8083 – 30.3385 |
| reviewer_h_per_1k_south_asian | 30.3146 | 28.3918 – 32.2374 |
| reviewer_h_per_1k_western | 27.3732 | 26.0895 – 28.6569 |
| review_load_ratio_max_min_origin | 1.1478 | 1.0553 – 1.2402 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 5 | 2129 | 0.5470% |
| cervical_false_closures | 0 | 1005 | 0.3660% |
| wrong_patient_released | 0 | 10034 | 0.0370% |
| released_without_granted_consent | 0 | 5646 | 0.0650% |
| duplicate_or_lost_outputs | 0 | 124342 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 152877 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 112 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 0.9968 | 0.9948 – 0.9988 |
| crc_recall | 0.7264 | 0.6935 – 0.7593 |
| crc_recall_no_review | 0.0828 | 0.076 – 0.0896 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9182 | 0.9074 – 0.929 |
| kpi_truth | 0.326 | 0.3178 – 0.3342 |
| kpi_wasl | 0.2892 | 0.2793 – 0.2991 |
| kpi_eid_only | 0.1248 | 0.1175 – 0.1321 |
| kpi_abs_error_wasl | 0.0368 | 0.0279 – 0.0457 |
| kpi_abs_error_eid_only | 0.2012 | 0.1937 – 0.2087 |
| linkage_pair_f1 | 0.942 | 0.9392 – 0.9448 |
| reviewer_h_per_1k | 27.8058 | 26.7986 – 28.813 |
| review_items_per_100_loops | 77.4858 | 73.965 – 81.0066 |
| throughput_ev_s | 638.0907 | 563.3395 – 712.8419 |
| review_items_per_100_arab | 79.504 | 74.6382 – 84.3698 |
| review_items_per_100_filipino | 75.1782 | 70.0639 – 80.2925 |
| review_items_per_100_south_asian | 77.7738 | 75.0609 – 80.4867 |
| review_items_per_100_western | 71.2544 | 65.5477 – 76.9611 |
| reviewer_h_per_1k_arab | 28.6842 | 26.6305 – 30.7379 |
| reviewer_h_per_1k_filipino | 27.498 | 25.6228 – 29.3732 |
| reviewer_h_per_1k_south_asian | 27.7796 | 25.9733 – 29.5859 |
| reviewer_h_per_1k_western | 24.9204 | 22.8474 – 26.9934 |
| review_load_ratio_max_min_origin | 1.1314 | 1.0676 – 1.1953 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 7 | 2109 | 0.6830% |
| cervical_false_closures | 0 | 973 | 0.3780% |
| wrong_patient_released | 0 | 9349 | 0.0390% |
| released_without_granted_consent | 0 | 5476 | 0.0670% |
| duplicate_or_lost_outputs | 0 | 121113 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153558 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 112 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 1 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
