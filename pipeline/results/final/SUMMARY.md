# Final frozen evaluation

Code revision `03e430d75026` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-30T02:55:07.414829Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7612 | 0.7333 – 0.7891 |
| crc_recall_no_review | 0.3108 | 0.2749 – 0.3467 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.97 | 0.9518 – 0.9882 |
| kpi_truth | 0.3362 | 0.3216 – 0.3508 |
| kpi_wasl | 0.3262 | 0.3113 – 0.3411 |
| kpi_eid_only | 0.1278 | 0.1193 – 0.1363 |
| kpi_abs_error_wasl | 0.01 | 0.0067 – 0.0133 |
| kpi_abs_error_eid_only | 0.2084 | 0.1923 – 0.2245 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9842 | 0.9713 – 0.9971 |
| lung_state_accuracy | 0.9932 | 0.9877 – 0.9987 |
| lung_escalation_recall | 0.9692 | 0.8837 – 1.0547 |
| linkage_pair_f1 | 0.977 | 0.9734 – 0.9806 |
| reviewer_h_per_1k | 17.253 | 16.5584 – 17.9476 |
| review_items_per_100_loops | 52.5862 | 49.9547 – 55.2177 |
| throughput_ev_s | 574.3955 | 553.0861 – 595.705 |
| review_items_per_100_arab | 54.2702 | 49.8022 – 58.7382 |
| review_items_per_100_filipino | 54.187 | 51.6751 – 56.6989 |
| review_items_per_100_south_asian | 51.7026 | 49.0935 – 54.3117 |
| review_items_per_100_western | 47.4896 | 44.3414 – 50.6378 |
| reviewer_h_per_1k_arab | 17.7104 | 16.8251 – 18.5957 |
| reviewer_h_per_1k_filipino | 18.348 | 16.9195 – 19.7765 |
| reviewer_h_per_1k_south_asian | 16.9644 | 15.5541 – 18.3747 |
| reviewer_h_per_1k_western | 15.5602 | 14.4207 – 16.6997 |
| review_load_ratio_max_min_origin | 1.1885 | 1.1077 – 1.2694 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2185 | 0.1690% |
| cervical_false_closures | 0 | 1023 | 0.3600% |
| wrong_patient_released | 0 | 12409 | 0.0300% |
| released_without_granted_consent | 0 | 8085 | 0.0460% |
| duplicate_or_lost_outputs | 0 | 140024 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 173842 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 114 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 1 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 5 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1506 | 0.2450% |
| handoff_false_closures | 0 | 66 | 5.4360% |

Review load by name origin: pooled over seeds {'arab': 54.2, 'filipino': 54.1, 'south_asian': 51.7, 'western': 47.5} items per 100 loops, max/min **1.1413**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1248 and 95th percentile 1.2613 (mean over seeds); 1 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0543 and 95th percentile 1.1065; the observed pooled 1.1413 has p = 0.005.

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
| crc_recall | 0.7038 | 0.6817 – 0.7259 |
| crc_recall_no_review | 0.3232 | 0.3055 – 0.3409 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.914 | 0.9018 – 0.9262 |
| kpi_truth | 0.322 | 0.3077 – 0.3363 |
| kpi_wasl | 0.2934 | 0.284 – 0.3028 |
| kpi_eid_only | 0.1312 | 0.119 – 0.1434 |
| kpi_abs_error_wasl | 0.0286 | 0.0224 – 0.0348 |
| kpi_abs_error_eid_only | 0.1908 | 0.1738 – 0.2078 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9488 | 0.9401 – 0.9575 |
| lung_state_accuracy | 0.9736 | 0.9717 – 0.9755 |
| lung_escalation_recall | 0.8768 | 0.8298 – 0.9238 |
| linkage_pair_f1 | 0.94 | 0.938 – 0.942 |
| reviewer_h_per_1k | 15.6984 | 15.0586 – 16.3382 |
| review_items_per_100_loops | 47.138 | 45.7137 – 48.5623 |
| throughput_ev_s | 576.0105 | 461.6006 – 690.4204 |
| review_items_per_100_arab | 48.912 | 46.5773 – 51.2467 |
| review_items_per_100_filipino | 44.0458 | 41.5249 – 46.5667 |
| review_items_per_100_south_asian | 46.6656 | 44.8991 – 48.4321 |
| review_items_per_100_western | 43.5338 | 39.5092 – 47.5584 |
| reviewer_h_per_1k_arab | 16.3032 | 14.8744 – 17.732 |
| reviewer_h_per_1k_filipino | 14.6504 | 13.2664 – 16.0344 |
| reviewer_h_per_1k_south_asian | 15.6328 | 14.808 – 16.4576 |
| reviewer_h_per_1k_western | 14.3098 | 13.3073 – 15.3123 |
| review_load_ratio_max_min_origin | 1.1625 | 1.0813 – 1.2437 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2063 | 0.1790% |
| cervical_false_closures | 0 | 987 | 0.3730% |
| wrong_patient_released | 0 | 11683 | 0.0320% |
| released_without_granted_consent | 0 | 7656 | 0.0480% |
| duplicate_or_lost_outputs | 0 | 136075 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 173929 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 110 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1463 | 0.2520% |
| handoff_false_closures | 0 | 44 | 8.0420% |

Review load by name origin: pooled over seeds {'arab': 48.9, 'filipino': 44.0, 'south_asian': 46.6, 'western': 43.6} items per 100 loops, max/min **1.1229**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1375 and 95th percentile 1.2864 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0586 and 95th percentile 1.1137; the observed pooled 1.1229 has p = 0.033.

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
