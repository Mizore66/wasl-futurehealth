# Final frozen evaluation

Code revision `e4c7553beead` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-30T07:12:52.887451Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8088 | 0.7888 – 0.8288 |
| crc_recall_no_review | 0.3424 | 0.3182 – 0.3666 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9674 | 0.9485 – 0.9863 |
| kpi_truth | 0.3306 | 0.3169 – 0.3443 |
| kpi_wasl | 0.321 | 0.3073 – 0.3347 |
| kpi_eid_only | 0.1294 | 0.1174 – 0.1414 |
| kpi_abs_error_wasl | 0.0096 | 0.007 – 0.0122 |
| kpi_abs_error_eid_only | 0.2012 | 0.1911 – 0.2113 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9842 | 0.9733 – 0.9951 |
| lung_state_accuracy | 0.9934 | 0.989 – 0.9978 |
| lung_escalation_recall | 0.977 | 0.9132 – 1.0408 |
| linkage_pair_f1 | 0.9784 | 0.9765 – 0.9803 |
| reviewer_h_per_1k | 17.4006 | 16.504 – 18.2972 |
| review_items_per_100_loops | 53.2138 | 50.726 – 55.7016 |
| throughput_ev_s | 542.7829 | 503.3645 – 582.2013 |
| review_items_per_100_arab | 55.4426 | 51.7011 – 59.1841 |
| review_items_per_100_filipino | 52.1252 | 47.4478 – 56.8026 |
| review_items_per_100_south_asian | 52.7826 | 47.4416 – 58.1236 |
| review_items_per_100_western | 46.93 | 43.1088 – 50.7512 |
| reviewer_h_per_1k_arab | 18.0552 | 16.6234 – 19.487 |
| reviewer_h_per_1k_filipino | 17.4208 | 16.6283 – 18.2133 |
| reviewer_h_per_1k_south_asian | 17.4034 | 15.9244 – 18.8824 |
| reviewer_h_per_1k_western | 15.1316 | 13.8876 – 16.3756 |
| review_load_ratio_max_min_origin | 1.2353 | 1.1457 – 1.3249 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2287 | 0.1610% |
| cervical_false_closures | 0 | 1020 | 0.3610% |
| wrong_patient_released | 0 | 12267 | 0.0300% |
| released_without_granted_consent | 0 | 7887 | 0.0470% |
| duplicate_or_lost_outputs | 0 | 139464 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 173713 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 111 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1506 | 0.2450% |
| handoff_false_closures | 0 | 76 | 4.7380% |

Review load by name origin: pooled over seeds {'arab': 55.4, 'filipino': 52.1, 'south_asian': 52.8, 'western': 46.9} items per 100 loops, max/min **1.1812**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1215 and 95th percentile 1.2602 (mean over seeds); 2 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0527 and 95th percentile 1.1073; the observed pooled 1.1812 has p = 0.0.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |
|---|---|
| handoff_acted_state_correct | 229 |
| handoff_closed | 76 |
| handoff_packets_all_required_fields | 229 |
| handoff_packets_matched | 229 |
| handoff_packets_missed | 1 |
| handoff_truth_acted_by_act_by | 115 |
| handoff_truth_closed_after_handoff | 90 |
| handoff_truth_packets | 230 |
| handoff_truth_referrer_acted | 181 |
| handoff_truth_urgent | 187 |
| lung_workup_pred_awaiting_tissue | 4 |
| lung_workup_pred_closed_by_ct | 59 |
| lung_workup_pred_closed_by_pet | 71 |
| lung_workup_pred_closed_by_tissue | 36 |
| lung_workup_truth_awaiting_tissue | 4 |
| lung_workup_truth_path_ct | 91 |
| lung_workup_truth_path_pet | 122 |
| lung_workup_truth_path_tissue | 30 |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7368 | 0.7239 – 0.7497 |
| crc_recall_no_review | 0.3164 | 0.2913 – 0.3415 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9168 | 0.9026 – 0.931 |
| kpi_truth | 0.3272 | 0.3009 – 0.3535 |
| kpi_wasl | 0.2936 | 0.2718 – 0.3154 |
| kpi_eid_only | 0.1304 | 0.1194 – 0.1414 |
| kpi_abs_error_wasl | 0.0336 | 0.0253 – 0.0419 |
| kpi_abs_error_eid_only | 0.1968 | 0.1767 – 0.2169 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9486 | 0.9317 – 0.9655 |
| lung_state_accuracy | 0.974 | 0.9704 – 0.9776 |
| lung_escalation_recall | 0.8886 | 0.8353 – 0.9419 |
| linkage_pair_f1 | 0.9406 | 0.938 – 0.9432 |
| reviewer_h_per_1k | 15.8518 | 15.5694 – 16.1342 |
| review_items_per_100_loops | 48.8346 | 47.6586 – 50.0106 |
| throughput_ev_s | 551.2863 | 433.1874 – 669.3851 |
| review_items_per_100_arab | 49.9794 | 49.4027 – 50.5561 |
| review_items_per_100_filipino | 51.8198 | 44.4291 – 59.2105 |
| review_items_per_100_south_asian | 48.6712 | 45.6334 – 51.709 |
| review_items_per_100_western | 43.368 | 36.6558 – 50.0802 |
| reviewer_h_per_1k_arab | 16.1502 | 15.7127 – 16.5877 |
| reviewer_h_per_1k_filipino | 16.7638 | 14.5571 – 18.9705 |
| reviewer_h_per_1k_south_asian | 16.0136 | 14.4831 – 17.5441 |
| reviewer_h_per_1k_western | 14.0134 | 12.2061 – 15.8207 |
| review_load_ratio_max_min_origin | 1.2806 | 1.1101 – 1.4512 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2090 | 0.1760% |
| cervical_false_closures | 0 | 991 | 0.3720% |
| wrong_patient_released | 0 | 11846 | 0.0310% |
| released_without_granted_consent | 0 | 7544 | 0.0490% |
| duplicate_or_lost_outputs | 0 | 136669 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 174842 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 112 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1463 | 0.2520% |
| handoff_false_closures | 0 | 71 | 5.0630% |

Review load by name origin: pooled over seeds {'arab': 50.0, 'filipino': 51.7, 'south_asian': 48.7, 'western': 43.4} items per 100 loops, max/min **1.1932**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.133 and 95th percentile 1.2827 (mean over seeds); 2 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0569 and 95th percentile 1.1115; the observed pooled 1.1932 has p = 0.0.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |
|---|---|
| handoff_acted_state_correct | 233 |
| handoff_closed | 71 |
| handoff_packets_all_required_fields | 233 |
| handoff_packets_matched | 233 |
| handoff_packets_missed | 3 |
| handoff_truth_acted_by_act_by | 119 |
| handoff_truth_closed_after_handoff | 90 |
| handoff_truth_packets | 236 |
| handoff_truth_referrer_acted | 187 |
| handoff_truth_urgent | 170 |
| lung_workup_pred_awaiting_tissue | 10 |
| lung_workup_pred_closed_by_ct | 59 |
| lung_workup_pred_closed_by_pet | 51 |
| lung_workup_pred_closed_by_tissue | 38 |
| lung_workup_truth_awaiting_tissue | 12 |
| lung_workup_truth_path_ct | 88 |
| lung_workup_truth_path_pet | 103 |
| lung_workup_truth_path_tissue | 45 |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
