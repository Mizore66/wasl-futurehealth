# Final frozen evaluation

Code revision `eea6f26b7fe1` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-30T05:12:08.664706Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7616 | 0.7346 – 0.7886 |
| crc_recall_no_review | 0.3114 | 0.2757 – 0.3471 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.97 | 0.9518 – 0.9882 |
| kpi_truth | 0.3362 | 0.3216 – 0.3508 |
| kpi_wasl | 0.3262 | 0.3113 – 0.3411 |
| kpi_eid_only | 0.1278 | 0.1193 – 0.1363 |
| kpi_abs_error_wasl | 0.01 | 0.0067 – 0.0133 |
| kpi_abs_error_eid_only | 0.2084 | 0.1923 – 0.2245 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9842 | 0.9713 – 0.9971 |
| lung_state_accuracy | 0.9934 | 0.9882 – 0.9986 |
| lung_escalation_recall | 0.977 | 0.9132 – 1.0408 |
| linkage_pair_f1 | 0.9774 | 0.9742 – 0.9806 |
| reviewer_h_per_1k | 17.2518 | 16.5857 – 17.9179 |
| review_items_per_100_loops | 52.545 | 50.1097 – 54.9803 |
| throughput_ev_s | 617.4974 | 573.1669 – 661.828 |
| review_items_per_100_arab | 54.2006 | 49.9364 – 58.4648 |
| review_items_per_100_filipino | 54.3314 | 52.041 – 56.6218 |
| review_items_per_100_south_asian | 51.6524 | 49.0922 – 54.2126 |
| review_items_per_100_western | 47.4524 | 44.6283 – 50.2765 |
| reviewer_h_per_1k_arab | 17.7128 | 16.8736 – 18.552 |
| reviewer_h_per_1k_filipino | 18.3526 | 16.989 – 19.7162 |
| reviewer_h_per_1k_south_asian | 16.9724 | 15.5254 – 18.4194 |
| reviewer_h_per_1k_western | 15.5298 | 14.5274 – 16.5322 |
| review_load_ratio_max_min_origin | 1.1876 | 1.1131 – 1.2621 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2187 | 0.1690% |
| cervical_false_closures | 0 | 1023 | 0.3600% |
| wrong_patient_released | 0 | 12399 | 0.0300% |
| released_without_granted_consent | 0 | 8071 | 0.0460% |
| duplicate_or_lost_outputs | 0 | 140075 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 173884 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 114 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 1 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 5 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1506 | 0.2450% |
| handoff_false_closures | 0 | 67 | 5.3570% |

Review load by name origin: pooled over seeds {'arab': 54.2, 'filipino': 54.3, 'south_asian': 51.6, 'western': 47.5} items per 100 loops, max/min **1.1435**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1252 and 95th percentile 1.2602 (mean over seeds); 1 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0539 and 95th percentile 1.1055; the observed pooled 1.1435 has p = 0.005.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |
|---|---|
| handoff_acted_state_correct | 229 |
| handoff_closed | 67 |
| handoff_packets_all_required_fields | 229 |
| handoff_packets_matched | 229 |
| handoff_packets_missed | 1 |
| handoff_truth_acted_by_act_by | 105 |
| handoff_truth_closed_after_handoff | 85 |
| handoff_truth_packets | 230 |
| handoff_truth_referrer_acted | 188 |
| handoff_truth_urgent | 187 |
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
| cervical_recall | 0.9148 | 0.9015 – 0.9281 |
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
| reviewer_h_per_1k | 15.731 | 15.0956 – 16.3664 |
| review_items_per_100_loops | 47.2412 | 45.648 – 48.8344 |
| throughput_ev_s | 587.9935 | 470.6389 – 705.348 |
| review_items_per_100_arab | 49.0566 | 46.765 – 51.3482 |
| review_items_per_100_filipino | 44.4442 | 41.87 – 47.0184 |
| review_items_per_100_south_asian | 46.7204 | 44.8667 – 48.5741 |
| review_items_per_100_western | 43.398 | 39.4303 – 47.3657 |
| reviewer_h_per_1k_arab | 16.3508 | 14.9742 – 17.7274 |
| reviewer_h_per_1k_filipino | 14.7518 | 13.738 – 15.7656 |
| reviewer_h_per_1k_south_asian | 15.6572 | 14.884 – 16.4304 |
| reviewer_h_per_1k_western | 14.2632 | 13.2392 – 15.2872 |
| review_load_ratio_max_min_origin | 1.1661 | 1.0975 – 1.2347 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2063 | 0.1790% |
| cervical_false_closures | 0 | 988 | 0.3730% |
| wrong_patient_released | 0 | 11682 | 0.0320% |
| released_without_granted_consent | 0 | 7539 | 0.0490% |
| duplicate_or_lost_outputs | 0 | 136053 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 173977 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 110 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1463 | 0.2520% |
| handoff_false_closures | 0 | 45 | 7.8710% |

Review load by name origin: pooled over seeds {'arab': 49.1, 'filipino': 44.4, 'south_asian': 46.7, 'western': 43.4} items per 100 loops, max/min **1.1298**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1372 and 95th percentile 1.2888 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0585 and 95th percentile 1.1146; the observed pooled 1.1298 has p = 0.022.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |
|---|---|
| handoff_acted_state_correct | 220 |
| handoff_closed | 45 |
| handoff_packets_all_required_fields | 220 |
| handoff_packets_matched | 220 |
| handoff_packets_missed | 3 |
| handoff_truth_acted_by_act_by | 99 |
| handoff_truth_closed_after_handoff | 62 |
| handoff_truth_packets | 223 |
| handoff_truth_referrer_acted | 155 |
| handoff_truth_urgent | 157 |
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
