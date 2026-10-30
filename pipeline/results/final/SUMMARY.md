# Final frozen evaluation

Code revision `3caa8a194f72` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-30T09:26:46.044275Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8098 | 0.7881 – 0.8315 |
| crc_recall_no_review | 0.325 | 0.2991 – 0.3509 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9666 | 0.9488 – 0.9844 |
| kpi_truth | 0.3232 | 0.3099 – 0.3365 |
| kpi_wasl | 0.313 | 0.2992 – 0.3268 |
| kpi_eid_only | 0.1284 | 0.1197 – 0.1371 |
| kpi_abs_error_wasl | 0.0102 | 0.0074 – 0.013 |
| kpi_abs_error_eid_only | 0.1948 | 0.1849 – 0.2047 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9862 | 0.9803 – 0.9921 |
| lung_state_accuracy | 0.9942 | 0.992 – 0.9964 |
| lung_escalation_recall | 0.9914 | 0.9766 – 1.0062 |
| linkage_pair_f1 | 0.9754 | 0.972 – 0.9788 |
| reviewer_h_per_1k | 17.2152 | 16.7448 – 17.6856 |
| review_items_per_100_loops | 52.9208 | 52.1727 – 53.6689 |
| throughput_ev_s | 571.2355 | 477.48 – 664.9911 |
| review_items_per_100_arab | 53.8318 | 52.8473 – 54.8163 |
| review_items_per_100_filipino | 50.0352 | 45.7003 – 54.3701 |
| review_items_per_100_south_asian | 52.9164 | 48.9117 – 56.9211 |
| review_items_per_100_western | 51.5508 | 49.1683 – 53.9333 |
| reviewer_h_per_1k_arab | 17.4456 | 17.2014 – 17.6898 |
| reviewer_h_per_1k_filipino | 16.3732 | 14.372 – 18.3744 |
| reviewer_h_per_1k_south_asian | 17.286 | 15.9543 – 18.6177 |
| reviewer_h_per_1k_western | 16.8348 | 15.3962 – 18.2734 |
| review_load_ratio_max_min_origin | 1.1284 | 1.0618 – 1.1951 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3034 | 0.1220% |
| cervical_false_closures | 0 | 1403 | 0.2630% |
| wrong_patient_released | 0 | 17809 | 0.0210% |
| released_without_granted_consent | 0 | 11197 | 0.0330% |
| duplicate_or_lost_outputs | 0 | 192137 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 233838 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 117 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 2026 | 0.1820% |
| handoff_false_closures | 0 | 85 | 4.2470% |

Review load by name origin: pooled over seeds {'arab': 53.8, 'filipino': 50.0, 'south_asian': 52.9, 'western': 51.5} items per 100 loops, max/min **1.0773**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1052 and 95th percentile 1.2157 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0469 and 95th percentile 1.0899; the observed pooled 1.0773 has p = 0.122.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |
|---|---|
| handoff_acted_state_correct | 330 |
| handoff_closed | 85 |
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
| crc_recall | 0.7466 | 0.738 – 0.7552 |
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
| linkage_pair_f1 | 0.9396 | 0.9362 – 0.943 |
| reviewer_h_per_1k | 15.4692 | 14.6231 – 16.3153 |
| review_items_per_100_loops | 47.2974 | 45.2368 – 49.358 |
| throughput_ev_s | 623.4576 | 493.0911 – 753.824 |
| review_items_per_100_arab | 47.5554 | 45.8271 – 49.2837 |
| review_items_per_100_filipino | 47.6638 | 38.4977 – 56.8299 |
| review_items_per_100_south_asian | 48.2598 | 41.6084 – 54.9112 |
| review_items_per_100_western | 44.5384 | 42.9292 – 46.1476 |
| reviewer_h_per_1k_arab | 15.5062 | 14.9635 – 16.0489 |
| reviewer_h_per_1k_filipino | 15.7946 | 12.7725 – 18.8167 |
| reviewer_h_per_1k_south_asian | 15.7502 | 13.4818 – 18.0186 |
| reviewer_h_per_1k_western | 14.6706 | 13.5472 – 15.794 |
| review_load_ratio_max_min_origin | 1.2136 | 1.0864 – 1.3408 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2886 | 0.1280% |
| cervical_false_closures | 0 | 1300 | 0.2830% |
| wrong_patient_released | 0 | 16743 | 0.0220% |
| released_without_granted_consent | 0 | 10620 | 0.0350% |
| duplicate_or_lost_outputs | 0 | 187349 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 234762 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 120 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1899 | 0.1940% |
| handoff_false_closures | 0 | 77 | 4.6780% |

Review load by name origin: pooled over seeds {'arab': 47.5, 'filipino': 47.8, 'south_asian': 48.3, 'western': 44.5} items per 100 loops, max/min **1.0843**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1136 and 95th percentile 1.2377 (mean over seeds); 2 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0511 and 95th percentile 1.1018; the observed pooled 1.0843 has p = 0.105.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |
|---|---|
| handoff_acted_state_correct | 296 |
| handoff_closed | 77 |
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
