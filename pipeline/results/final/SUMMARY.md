# Final frozen evaluation

Code revision `d61c29a8b051` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-28T09:17:27.203509Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.9462 | 0.939 – 0.9534 |
| crc_recall_no_review | 0.1904 | 0.1705 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9676 | 0.9524 – 0.9828 |
| kpi_truth | 0.3312 | 0.3022 – 0.3602 |
| kpi_wasl | 0.3212 | 0.293 – 0.3494 |
| kpi_eid_only | 0.1238 | 0.1093 – 0.1383 |
| kpi_abs_error_wasl | 0.01 | 0.0074 – 0.0126 |
| kpi_abs_error_eid_only | 0.2074 | 0.1857 – 0.2291 |
| linkage_pair_f1 | 0.9786 | 0.9742 – 0.983 |
| reviewer_h_per_1k | 28.6986 | 27.7299 – 29.6673 |
| review_items_per_100_loops | 84.0668 | 81.1484 – 86.9852 |
| throughput_ev_s | 560.2923 | 522.0601 – 598.5245 |
| review_items_per_100_arab | 86.5344 | 81.824 – 91.2448 |
| review_items_per_100_filipino | 75.6874 | 74.29 – 77.0848 |
| review_items_per_100_south_asian | 84.411 | 81.5646 – 87.2574 |
| review_items_per_100_western | 80.01 | 76.6366 – 83.3834 |
| reviewer_h_per_1k_arab | 29.6584 | 28.0092 – 31.3076 |
| reviewer_h_per_1k_filipino | 26.7268 | 24.561 – 28.8926 |
| reviewer_h_per_1k_south_asian | 28.5172 | 27.5133 – 29.5211 |
| reviewer_h_per_1k_western | 26.8812 | 25.1941 – 28.5683 |
| review_load_ratio_max_min_origin | 1.1468 | 1.0723 – 1.2214 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2698 | 0.1370% |
| cervical_false_closures | 0 | 1004 | 0.3670% |
| wrong_patient_released | 0 | 10090 | 0.0370% |
| released_without_granted_consent | 0 | 5540 | 0.0670% |
| duplicate_or_lost_outputs | 0 | 124601 | 0.0030% |
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
| crc_recall | 0.879 | 0.8622 – 0.8958 |
| crc_recall_no_review | 0.1946 | 0.1789 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9132 | 0.9014 – 0.925 |
| kpi_truth | 0.3266 | 0.3071 – 0.3461 |
| kpi_wasl | 0.2916 | 0.2742 – 0.309 |
| kpi_eid_only | 0.1212 | 0.1107 – 0.1317 |
| kpi_abs_error_wasl | 0.035 | 0.026 – 0.044 |
| kpi_abs_error_eid_only | 0.2054 | 0.1954 – 0.2154 |
| linkage_pair_f1 | 0.942 | 0.9376 – 0.9464 |
| reviewer_h_per_1k | 25.8588 | 25.2242 – 26.4934 |
| review_items_per_100_loops | 75.4 | 73.6628 – 77.1372 |
| throughput_ev_s | 626.7072 | 549.4446 – 703.9699 |
| review_items_per_100_arab | 78.4464 | 77.5152 – 79.3776 |
| review_items_per_100_filipino | 69.2248 | 62.7469 – 75.7027 |
| review_items_per_100_south_asian | 75.188 | 70.1383 – 80.2377 |
| review_items_per_100_western | 68.8596 | 66.4701 – 71.2491 |
| reviewer_h_per_1k_arab | 26.881 | 26.4825 – 27.2795 |
| reviewer_h_per_1k_filipino | 23.5008 | 21.9397 – 25.0619 |
| reviewer_h_per_1k_south_asian | 25.7806 | 23.2599 – 28.3013 |
| reviewer_h_per_1k_western | 23.8566 | 22.5284 – 25.1848 |
| review_load_ratio_max_min_origin | 1.1735 | 1.1161 – 1.2309 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2533 | 0.1460% |
| cervical_false_closures | 0 | 968 | 0.3800% |
| wrong_patient_released | 0 | 9398 | 0.0390% |
| released_without_granted_consent | 0 | 5467 | 0.0670% |
| duplicate_or_lost_outputs | 0 | 121060 | 0.0030% |
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
