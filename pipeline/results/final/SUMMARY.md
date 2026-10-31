# Final frozen evaluation

Code revision `c2a74e20c86f` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-31T05:55:24.650862Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8416 | 0.825 – 0.8582 |
| crc_recall_no_review | 0.325 | 0.2991 – 0.3509 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9658 | 0.9467 – 0.9849 |
| kpi_truth | 0.323 | 0.3097 – 0.3363 |
| kpi_wasl | 0.3124 | 0.2989 – 0.3259 |
| kpi_eid_only | 0.1284 | 0.1197 – 0.1371 |
| kpi_abs_error_wasl | 0.0106 | 0.0076 – 0.0136 |
| kpi_abs_error_eid_only | 0.1946 | 0.1849 – 0.2043 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9862 | 0.9803 – 0.9921 |
| lung_state_accuracy | 0.9942 | 0.992 – 0.9964 |
| lung_escalation_recall | 0.9914 | 0.9766 – 1.0062 |
| linkage_pair_f1 | 0.975 | 0.9717 – 0.9783 |
| reviewer_h_per_1k | 16.9546 | 16.3399 – 17.5693 |
| review_items_per_100_loops | 51.7546 | 50.8311 – 52.6781 |
| throughput_ev_s | 550.0381 | 519.1785 – 580.8977 |
| review_items_per_100_arab | 52.4966 | 51.1768 – 53.8164 |
| review_items_per_100_filipino | 48.7312 | 44.138 – 53.3244 |
| review_items_per_100_south_asian | 52.2186 | 47.9324 – 56.5048 |
| review_items_per_100_western | 50.2878 | 48.3914 – 52.1842 |
| reviewer_h_per_1k_arab | 17.1344 | 16.7809 – 17.4879 |
| reviewer_h_per_1k_filipino | 16.0364 | 13.9043 – 18.1685 |
| reviewer_h_per_1k_south_asian | 17.219 | 15.5034 – 18.9346 |
| reviewer_h_per_1k_western | 16.4814 | 15.34 – 17.6228 |
| review_load_ratio_max_min_origin | 1.1331 | 1.048 – 1.2181 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3153 | 0.1170% |
| cervical_false_closures | 0 | 1402 | 0.2630% |
| wrong_patient_released | 0 | 14247 | 0.0260% |
| released_without_granted_consent | 0 | 9001 | 0.0410% |
| duplicate_or_lost_outputs | 0 | 185228 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 233836 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 117 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 2026 | 0.1820% |
| handoff_false_closures | 0 | 88 | 4.1050% |

Review load by name origin: pooled over seeds {'arab': 52.5, 'filipino': 48.7, 'south_asian': 52.2, 'western': 50.3} items per 100 loops, max/min **1.0787**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1042 and 95th percentile 1.2181 (mean over seeds); 1 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0462 and 95th percentile 1.0909; the observed pooled 1.0787 has p = 0.109.
CRC loops by age band: {'40-59': 66.2, '60-79': 70.0, '80+': 69.6, '<40': 68.8} review items per 100 loops, max/min **1.0582** (no-bias 95th percentile 1.0832, p = 0.226); recall {'40-59': 0.817, '60-79': 0.851, '80+': 0.842, '<40': 0.859}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 68.3, 'M': 69.0} review items per 100 loops, max/min **1.0092** (no-bias 95th percentile 1.0443, p = 0.673); recall {'F': 0.842, 'M': 0.84}; false closures {'F': 0, 'M': 0}.

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
| handoff_truth_referrer_acted | 256 |
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
| kpi_truth | 0.329 | 0.3066 – 0.3514 |
| kpi_wasl | 0.2968 | 0.2709 – 0.3227 |
| kpi_eid_only | 0.1224 | 0.1141 – 0.1307 |
| kpi_abs_error_wasl | 0.0322 | 0.025 – 0.0394 |
| kpi_abs_error_eid_only | 0.2066 | 0.1889 – 0.2243 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9498 | 0.9398 – 0.9598 |
| lung_state_accuracy | 0.9774 | 0.9732 – 0.9816 |
| lung_escalation_recall | 0.9198 | 0.8915 – 0.9481 |
| linkage_pair_f1 | 0.9396 | 0.9362 – 0.943 |
| reviewer_h_per_1k | 15.2032 | 14.316 – 16.0904 |
| review_items_per_100_loops | 46.1208 | 43.9916 – 48.25 |
| throughput_ev_s | 604.3659 | 502.3405 – 706.3912 |
| review_items_per_100_arab | 46.4742 | 44.3089 – 48.6395 |
| review_items_per_100_filipino | 46.7994 | 38.0852 – 55.5136 |
| review_items_per_100_south_asian | 46.701 | 40.3293 – 53.0727 |
| review_items_per_100_western | 43.444 | 41.9052 – 44.9828 |
| reviewer_h_per_1k_arab | 15.2958 | 14.5372 – 16.0544 |
| reviewer_h_per_1k_filipino | 15.5216 | 12.5998 – 18.4434 |
| reviewer_h_per_1k_south_asian | 15.351 | 13.2229 – 17.4791 |
| reviewer_h_per_1k_western | 14.4274 | 13.3036 – 15.5512 |
| review_load_ratio_max_min_origin | 1.2134 | 1.0965 – 1.3302 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2997 | 0.1230% |
| cervical_false_closures | 0 | 1300 | 0.2830% |
| wrong_patient_released | 0 | 13479 | 0.0270% |
| released_without_granted_consent | 0 | 8678 | 0.0420% |
| duplicate_or_lost_outputs | 0 | 180908 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 234761 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 120 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1899 | 0.1940% |
| handoff_false_closures | 0 | 78 | 4.6190% |

Review load by name origin: pooled over seeds {'arab': 46.5, 'filipino': 46.9, 'south_asian': 46.7, 'western': 43.4} items per 100 loops, max/min **1.0795**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1147 and 95th percentile 1.2379 (mean over seeds); 3 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.051 and 95th percentile 1.1005; the observed pooled 1.0795 has p = 0.13.
CRC loops by age band: {'40-59': 60.3, '60-79': 63.7, '80+': 62.3, '<40': 62.1} review items per 100 loops, max/min **1.0561** (no-bias 95th percentile 1.087, p = 0.325); recall {'40-59': 0.77, '60-79': 0.781, '80+': 0.792, '<40': 0.764}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 62.9, 'M': 59.9} review items per 100 loops, max/min **1.0501** (no-bias 95th percentile 1.0489, p = 0.042); recall {'F': 0.772, 'M': 0.784}; false closures {'F': 0, 'M': 0}.

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
