# Final frozen evaluation

Code revision `a3ad83a5431a` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T19:02:39.250134Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7612 | 0.7344 – 0.788 |
| crc_recall_no_review | 0.3054 | 0.27 – 0.3408 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.97 | 0.9518 – 0.9882 |
| kpi_truth | 0.3362 | 0.3216 – 0.3508 |
| kpi_wasl | 0.326 | 0.3114 – 0.3406 |
| kpi_eid_only | 0.1278 | 0.1193 – 0.1363 |
| kpi_abs_error_wasl | 0.0102 | 0.0068 – 0.0136 |
| kpi_abs_error_eid_only | 0.2084 | 0.1923 – 0.2245 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9842 | 0.9713 – 0.9971 |
| lung_state_accuracy | 0.9934 | 0.9882 – 0.9986 |
| lung_escalation_recall | 0.977 | 0.9132 – 1.0408 |
| linkage_pair_f1 | 0.9772 | 0.9736 – 0.9808 |
| reviewer_h_per_1k | 19.0802 | 18.4445 – 19.7159 |
| review_items_per_100_loops | 58.7242 | 56.3776 – 61.0708 |
| throughput_ev_s | 515.437 | 459.7819 – 571.0921 |
| review_items_per_100_arab | 60.4602 | 56.0848 – 64.8356 |
| review_items_per_100_filipino | 60.8536 | 58.1609 – 63.5463 |
| review_items_per_100_south_asian | 58.1684 | 55.2285 – 61.1083 |
| review_items_per_100_western | 52.6054 | 48.9706 – 56.2402 |
| reviewer_h_per_1k_arab | 19.5652 | 18.6463 – 20.4841 |
| reviewer_h_per_1k_filipino | 20.2172 | 18.7457 – 21.6887 |
| reviewer_h_per_1k_south_asian | 18.9086 | 17.5216 – 20.2956 |
| reviewer_h_per_1k_western | 17.0684 | 15.7612 – 18.3756 |
| review_load_ratio_max_min_origin | 1.1932 | 1.1054 – 1.2811 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2185 | 0.1690% |
| cervical_false_closures | 0 | 1023 | 0.3600% |
| wrong_patient_released | 0 | 12421 | 0.0300% |
| released_without_granted_consent | 0 | 7278 | 0.0510% |
| duplicate_or_lost_outputs | 0 | 141181 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 173842 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 114 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 1 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 5 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1506 | 0.2450% |
| handoff_false_closures | 0 | 66 | 5.4360% |

Review load by name origin: pooled over seeds {'arab': 60.4, 'filipino': 60.8, 'south_asian': 58.2, 'western': 52.6} items per 100 loops, max/min **1.1545**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1149 and 95th percentile 1.2442 (mean over seeds); 1 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0502 and 95th percentile 1.097; the observed pooled 1.1545 has p = 0.002.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |
|---|---|
| handoff_acted_state_correct | 206 |
| handoff_closed | 66 |
| handoff_packets_all_required_fields | 222 |
| handoff_packets_matched | 206 |
| handoff_packets_missed | 17 |
| handoff_packets_pipeline_only | 16 |
| handoff_truth_acted_by_act_by | 100 |
| handoff_truth_closed_after_handoff | 84 |
| handoff_truth_packets | 223 |
| handoff_truth_referrer_acted | 182 |
| handoff_truth_urgent | 181 |
| lung_workup_pred_awaiting_tissue | 4 |
| lung_workup_pred_closed_by_ct | 60 |
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
| crc_recall | 0.703 | 0.6799 – 0.7261 |
| crc_recall_no_review | 0.3156 | 0.2984 – 0.3328 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.914 | 0.9018 – 0.9262 |
| kpi_truth | 0.322 | 0.3077 – 0.3363 |
| kpi_wasl | 0.2932 | 0.2839 – 0.3025 |
| kpi_eid_only | 0.1312 | 0.119 – 0.1434 |
| kpi_abs_error_wasl | 0.0288 | 0.0225 – 0.0351 |
| kpi_abs_error_eid_only | 0.1908 | 0.1738 – 0.2078 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9488 | 0.9401 – 0.9575 |
| lung_state_accuracy | 0.9736 | 0.9717 – 0.9755 |
| lung_escalation_recall | 0.8768 | 0.8298 – 0.9238 |
| linkage_pair_f1 | 0.9396 | 0.9373 – 0.9419 |
| reviewer_h_per_1k | 17.1786 | 16.5726 – 17.7846 |
| review_items_per_100_loops | 52.0484 | 50.7951 – 53.3017 |
| throughput_ev_s | 520.98 | 399.5985 – 642.3615 |
| review_items_per_100_arab | 54.046 | 51.7719 – 56.3201 |
| review_items_per_100_filipino | 49.6308 | 44.6842 – 54.5774 |
| review_items_per_100_south_asian | 51.1704 | 49.5357 – 52.8051 |
| review_items_per_100_western | 47.9288 | 43.0477 – 52.8099 |
| reviewer_h_per_1k_arab | 17.8286 | 16.3659 – 19.2913 |
| reviewer_h_per_1k_filipino | 16.3944 | 14.5631 – 18.2257 |
| reviewer_h_per_1k_south_asian | 16.9572 | 15.9705 – 17.9439 |
| reviewer_h_per_1k_western | 15.7354 | 14.3566 – 17.1142 |
| review_load_ratio_max_min_origin | 1.1784 | 1.0478 – 1.3091 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2061 | 0.1790% |
| cervical_false_closures | 0 | 987 | 0.3730% |
| wrong_patient_released | 0 | 11685 | 0.0320% |
| released_without_granted_consent | 0 | 7138 | 0.0520% |
| duplicate_or_lost_outputs | 0 | 137134 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 173929 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 110 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1463 | 0.2520% |
| handoff_false_closures | 0 | 44 | 8.0420% |

Review load by name origin: pooled over seeds {'arab': 54.0, 'filipino': 49.6, 'south_asian': 51.2, 'western': 48.0} items per 100 loops, max/min **1.1264**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1296 and 95th percentile 1.2677 (mean over seeds); 1 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0552 and 95th percentile 1.1094; the observed pooled 1.1264 has p = 0.02.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |
|---|---|
| handoff_acted_state_correct | 174 |
| handoff_closed | 44 |
| handoff_packets_all_required_fields | 198 |
| handoff_packets_matched | 176 |
| handoff_packets_missed | 34 |
| handoff_packets_pipeline_only | 22 |
| handoff_truth_acted_by_act_by | 95 |
| handoff_truth_closed_after_handoff | 62 |
| handoff_truth_packets | 210 |
| handoff_truth_referrer_acted | 149 |
| handoff_truth_urgent | 148 |
| lung_workup_pred_awaiting_tissue | 10 |
| lung_workup_pred_closed_by_ct | 61 |
| lung_workup_pred_closed_by_pet | 52 |
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
