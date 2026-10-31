# Final frozen evaluation

Code revision `dc7fee2bb40a` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-31T04:17:50.417453Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8416 | 0.825 – 0.8582 |
| crc_recall_no_review | 0.325 | 0.2991 – 0.3509 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9666 | 0.9488 – 0.9844 |
| kpi_truth | 0.3232 | 0.3099 – 0.3365 |
| kpi_wasl | 0.3128 | 0.2992 – 0.3264 |
| kpi_eid_only | 0.1284 | 0.1197 – 0.1371 |
| kpi_abs_error_wasl | 0.0104 | 0.0072 – 0.0136 |
| kpi_abs_error_eid_only | 0.1948 | 0.1849 – 0.2047 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9862 | 0.9803 – 0.9921 |
| lung_state_accuracy | 0.9942 | 0.992 – 0.9964 |
| lung_escalation_recall | 0.9914 | 0.9766 – 1.0062 |
| linkage_pair_f1 | 0.975 | 0.9717 – 0.9783 |
| reviewer_h_per_1k | 17.3592 | 16.7358 – 17.9826 |
| review_items_per_100_loops | 53.1946 | 52.291 – 54.0982 |
| throughput_ev_s | 613.9654 | 579.5267 – 648.4041 |
| review_items_per_100_arab | 54.0066 | 52.9487 – 55.0645 |
| review_items_per_100_filipino | 50.0392 | 45.6146 – 54.4638 |
| review_items_per_100_south_asian | 53.4856 | 49.3934 – 57.5778 |
| review_items_per_100_western | 51.855 | 49.5255 – 54.1845 |
| reviewer_h_per_1k_arab | 17.565 | 17.1971 – 17.9329 |
| reviewer_h_per_1k_filipino | 16.354 | 14.2345 – 18.4735 |
| reviewer_h_per_1k_south_asian | 17.572 | 16.0264 – 19.1176 |
| reviewer_h_per_1k_western | 16.9324 | 15.6573 – 18.2075 |
| review_load_ratio_max_min_origin | 1.1323 | 1.0623 – 1.2024 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3153 | 0.1170% |
| cervical_false_closures | 0 | 1403 | 0.2630% |
| wrong_patient_released | 0 | 17802 | 0.0210% |
| released_without_granted_consent | 0 | 11093 | 0.0330% |
| duplicate_or_lost_outputs | 0 | 191901 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 233838 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 117 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 2026 | 0.1820% |
| handoff_false_closures | 0 | 88 | 4.1050% |

Review load by name origin: pooled over seeds {'arab': 54.0, 'filipino': 50.0, 'south_asian': 53.5, 'western': 51.8} items per 100 loops, max/min **1.0808**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1053 and 95th percentile 1.2155 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0469 and 95th percentile 1.0909; the observed pooled 1.0808 has p = 0.098.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |
|---|---|
| handoff_acted_state_correct | 330 |
| handoff_closed | 88 |
| handoff_packets_all_required_fields | 330 |
| handoff_packets_matched | 330 |
| handoff_packets_missed | 3 |
| handoff_truth_acted_by_act_by | 152 |
| handoff_truth_closed_after_handoff | 96 |
| handoff_truth_packets | 333 |
| handoff_truth_referrer_acted | 257 |
| handoff_truth_urgent | 248 |
| lung_workup_pred_awaiting_tissue | 10 |
| lung_workup_pred_closed_by_ct | 94 |
| lung_workup_pred_closed_by_pet | 96 |
| lung_workup_pred_closed_by_tissue | 48 |
| lung_workup_truth_awaiting_tissue | 10 |
| lung_workup_truth_path_ct | 127 |
| lung_workup_truth_path_pet | 158 |
| lung_workup_truth_path_tissue | 48 |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7754 | 0.7623 – 0.7885 |
| crc_recall_no_review | 0.3196 | 0.2957 – 0.3435 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9034 | 0.8808 – 0.926 |
| kpi_truth | 0.3294 | 0.3074 – 0.3514 |
| kpi_wasl | 0.2972 | 0.2718 – 0.3226 |
| kpi_eid_only | 0.1224 | 0.1141 – 0.1307 |
| kpi_abs_error_wasl | 0.0322 | 0.025 – 0.0394 |
| kpi_abs_error_eid_only | 0.207 | 0.1896 – 0.2244 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9498 | 0.9398 – 0.9598 |
| lung_state_accuracy | 0.9774 | 0.9732 – 0.9816 |
| lung_escalation_recall | 0.9198 | 0.8915 – 0.9481 |
| linkage_pair_f1 | 0.9398 | 0.9364 – 0.9432 |
| reviewer_h_per_1k | 15.62 | 14.7393 – 16.5007 |
| review_items_per_100_loops | 47.4948 | 45.472 – 49.5176 |
| throughput_ev_s | 706.4821 | 585.844 – 827.1201 |
| review_items_per_100_arab | 47.8068 | 45.9459 – 49.6677 |
| review_items_per_100_filipino | 47.664 | 38.3232 – 57.0048 |
| review_items_per_100_south_asian | 48.4702 | 42.0695 – 54.8709 |
| review_items_per_100_western | 44.6432 | 43.0475 – 46.2389 |
| reviewer_h_per_1k_arab | 15.7064 | 15.0699 – 16.3429 |
| reviewer_h_per_1k_filipino | 15.748 | 12.5591 – 18.9369 |
| reviewer_h_per_1k_south_asian | 15.8802 | 13.685 – 18.0754 |
| reviewer_h_per_1k_western | 14.7948 | 13.7557 – 15.8339 |
| review_load_ratio_max_min_origin | 1.2222 | 1.0961 – 1.3483 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2997 | 0.1230% |
| cervical_false_closures | 0 | 1300 | 0.2830% |
| wrong_patient_released | 0 | 16767 | 0.0220% |
| released_without_granted_consent | 0 | 10611 | 0.0350% |
| duplicate_or_lost_outputs | 0 | 187203 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 234762 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 120 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1899 | 0.1940% |
| handoff_false_closures | 0 | 78 | 4.6190% |

Review load by name origin: pooled over seeds {'arab': 47.8, 'filipino': 47.8, 'south_asian': 48.5, 'western': 44.6} items per 100 loops, max/min **1.0865**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1129 and 95th percentile 1.2372 (mean over seeds); 3 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0514 and 95th percentile 1.1017; the observed pooled 1.0865 has p = 0.098.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |
|---|---|
| handoff_acted_state_correct | 296 |
| handoff_closed | 78 |
| handoff_packets_all_required_fields | 296 |
| handoff_packets_matched | 296 |
| handoff_packets_missed | 4 |
| handoff_truth_acted_by_act_by | 142 |
| handoff_truth_closed_after_handoff | 102 |
| handoff_truth_packets | 300 |
| handoff_truth_referrer_acted | 228 |
| handoff_truth_urgent | 232 |
| lung_workup_pred_awaiting_tissue | 6 |
| lung_workup_pred_closed_by_ct | 90 |
| lung_workup_pred_closed_by_pet | 81 |
| lung_workup_pred_closed_by_tissue | 51 |
| lung_workup_truth_awaiting_tissue | 6 |
| lung_workup_truth_path_ct | 142 |
| lung_workup_truth_path_pet | 151 |
| lung_workup_truth_path_tissue | 48 |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
