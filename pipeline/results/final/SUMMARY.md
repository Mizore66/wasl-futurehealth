# Final frozen evaluation

Code revision `4cafa26e4d64` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-31T13:27:56.078422Z

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
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9976 | 0.9959 – 0.9993 |
| dr_referral_recall | 0.9768 | 0.9561 – 0.9975 |
| dr_state_accuracy | 0.9978 | 0.9968 – 0.9988 |
| dr_triage_accuracy | 0.9988 | 0.9978 – 0.9998 |
| dr_overdue_accuracy | 0.9984 | 0.997 – 0.9998 |
| dr_exam_within_deadline_truth | 0.3514 | 0.2929 – 0.4099 |
| dr_exam_within_deadline_wasl | 0.343 | 0.2811 – 0.4049 |
| linkage_pair_f1 | 0.9744 | 0.973 – 0.9758 |
| reviewer_h_per_1k | 14.7398 | 14.3931 – 15.0865 |
| review_items_per_100_loops | 42.9124 | 42.5042 – 43.3206 |
| throughput_ev_s | 543.396 | 491.6547 – 595.1374 |
| review_items_per_100_arab | 42.8722 | 41.3308 – 44.4136 |
| review_items_per_100_filipino | 41.6756 | 38.1681 – 45.1831 |
| review_items_per_100_south_asian | 44.0636 | 41.0283 – 47.0989 |
| review_items_per_100_western | 41.9656 | 39.9464 – 43.9848 |
| reviewer_h_per_1k_arab | 14.6944 | 14.2126 – 15.1762 |
| reviewer_h_per_1k_filipino | 14.4414 | 12.742 – 16.1408 |
| reviewer_h_per_1k_south_asian | 15.0506 | 13.6767 – 16.4245 |
| reviewer_h_per_1k_western | 14.5832 | 13.3814 – 15.785 |
| review_load_ratio_max_min_origin | 1.1249 | 1.0675 – 1.1823 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3153 | 0.1170% |
| cervical_false_closures | 0 | 1402 | 0.2630% |
| wrong_patient_released | 0 | 15179 | 0.0240% |
| released_without_granted_consent | 0 | 9954 | 0.0370% |
| duplicate_or_lost_outputs | 0 | 203000 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 253148 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 117 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 2026 | 0.1820% |
| dr_false_closures | 0 | 5621 | 0.0660% |
| dr_referral_false_closures | 0 | 254 | 1.4420% |
| handoff_false_closures | 0 | 88 | 4.1050% |

Review load by name origin: pooled over seeds {'arab': 42.9, 'filipino': 41.6, 'south_asian': 44.1, 'western': 42.0} items per 100 loops, max/min **1.0584**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1061 and 95th percentile 1.2133 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0456 and 95th percentile 1.0924; the observed pooled 1.0584 has p = 0.284.
CRC loops by age band: {'40-59': 66.2, '60-79': 69.8, '80+': 69.5, '<40': 68.6} review items per 100 loops, max/min **1.0547** (no-bias 95th percentile 1.082, p = 0.263); recall {'40-59': 0.817, '60-79': 0.851, '80+': 0.842, '<40': 0.859}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 68.2, 'M': 68.9} review items per 100 loops, max/min **1.0112** (no-bias 95th percentile 1.0451, p = 0.634); recall {'F': 0.842, 'M': 0.84}; false closures {'F': 0, 'M': 0}.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off, DR readers) | Count |
|---|---|
| dr_reader_dr-clinic_agree | 1087 |
| dr_reader_dr-clinic_agree_but_wrong | 0 |
| dr_reader_dr-clinic_coded | 0 |
| dr_reader_dr-clinic_instances | 1102 |
| dr_reader_dr-screen_agree | 5513 |
| dr_reader_dr-screen_agree_but_wrong | 0 |
| dr_reader_dr-screen_coded | 3724 |
| dr_reader_dr-screen_instances | 6210 |
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
| crc_recall | 0.7746 | 0.7607 – 0.7885 |
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
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9938 | 0.9894 – 0.9982 |
| dr_referral_recall | 0.8802 | 0.792 – 0.9684 |
| dr_state_accuracy | 0.9938 | 0.9892 – 0.9984 |
| dr_triage_accuracy | 0.999 | 0.9978 – 1.0002 |
| dr_overdue_accuracy | 0.995 | 0.9914 – 0.9986 |
| dr_exam_within_deadline_truth | 0.4178 | 0.325 – 0.5106 |
| dr_exam_within_deadline_wasl | 0.3678 | 0.2769 – 0.4587 |
| linkage_pair_f1 | 0.937 | 0.9344 – 0.9396 |
| reviewer_h_per_1k | 13.3132 | 12.4501 – 14.1763 |
| review_items_per_100_loops | 38.5652 | 36.8195 – 40.3109 |
| throughput_ev_s | 627.8433 | 417.1747 – 838.5118 |
| review_items_per_100_arab | 37.9884 | 36.7229 – 39.2539 |
| review_items_per_100_filipino | 40.719 | 34.4711 – 46.9669 |
| review_items_per_100_south_asian | 40.1982 | 34.7078 – 45.6886 |
| review_items_per_100_western | 36.7638 | 35.7014 – 37.8262 |
| reviewer_h_per_1k_arab | 13.1412 | 12.538 – 13.7444 |
| reviewer_h_per_1k_filipino | 14.031 | 11.4459 – 16.6161 |
| reviewer_h_per_1k_south_asian | 13.766 | 11.785 – 15.747 |
| reviewer_h_per_1k_western | 12.7868 | 12.1243 – 13.4493 |
| review_load_ratio_max_min_origin | 1.1877 | 1.0369 – 1.3385 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2995 | 0.1230% |
| cervical_false_closures | 0 | 1300 | 0.2830% |
| wrong_patient_released | 0 | 14343 | 0.0260% |
| released_without_granted_consent | 0 | 9438 | 0.0390% |
| duplicate_or_lost_outputs | 0 | 198213 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 254055 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 117 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1899 | 0.1940% |
| dr_false_closures | 0 | 5608 | 0.0660% |
| dr_referral_false_closures | 0 | 214 | 1.7090% |
| handoff_false_closures | 0 | 78 | 4.6190% |

Review load by name origin: pooled over seeds {'arab': 38.0, 'filipino': 40.8, 'south_asian': 40.3, 'western': 36.8} items per 100 loops, max/min **1.1097**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1143 and 95th percentile 1.2371 (mean over seeds); 2 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0475 and 95th percentile 1.0987; the observed pooled 1.1097 has p = 0.023.
CRC loops by age band: {'40-59': 60.5, '60-79': 63.8, '80+': 62.6, '<40': 62.2} review items per 100 loops, max/min **1.055** (no-bias 95th percentile 1.0869, p = 0.341); recall {'40-59': 0.77, '60-79': 0.781, '80+': 0.791, '<40': 0.763}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 63.2, 'M': 59.9} review items per 100 loops, max/min **1.0538** (no-bias 95th percentile 1.0489, p = 0.028); recall {'F': 0.771, 'M': 0.783}; false closures {'F': 0, 'M': 0}.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off, DR readers) | Count |
|---|---|
| dr_reader_dr-clinic_agree | 1068 |
| dr_reader_dr-clinic_agree_but_wrong | 0 |
| dr_reader_dr-clinic_coded | 0 |
| dr_reader_dr-clinic_instances | 1090 |
| dr_reader_dr-screen_agree | 5523 |
| dr_reader_dr-screen_agree_but_wrong | 0 |
| dr_reader_dr-screen_coded | 3746 |
| dr_reader_dr-screen_instances | 6207 |
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
