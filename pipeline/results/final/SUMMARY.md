# Final frozen evaluation

Code revision `d3a1e64236ad` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-31T16:51:32.761837Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8422 | 0.8265 – 0.8579 |
| crc_recall_no_review | 0.325 | 0.2991 – 0.3509 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9682 | 0.948 – 0.9884 |
| kpi_truth | 0.323 | 0.3097 – 0.3363 |
| kpi_wasl | 0.3126 | 0.2988 – 0.3264 |
| kpi_eid_only | 0.1284 | 0.1197 – 0.1371 |
| kpi_abs_error_wasl | 0.0104 | 0.0078 – 0.013 |
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
| linkage_pair_f1 | 0.9748 | 0.9734 – 0.9762 |
| reviewer_h_per_1k | 14.3444 | 14.003 – 14.6858 |
| review_items_per_100_loops | 41.5824 | 41.1731 – 41.9917 |
| throughput_ev_s | 513.6797 | 432.0152 – 595.3442 |
| review_items_per_100_arab | 41.4666 | 40.2373 – 42.6959 |
| review_items_per_100_filipino | 40.305 | 37.7098 – 42.9002 |
| review_items_per_100_south_asian | 42.6208 | 39.2255 – 46.0161 |
| review_items_per_100_western | 41.096 | 39.2421 – 42.9499 |
| reviewer_h_per_1k_arab | 14.2572 | 13.8599 – 14.6545 |
| reviewer_h_per_1k_filipino | 14.0788 | 12.8047 – 15.3529 |
| reviewer_h_per_1k_south_asian | 14.63 | 13.1405 – 16.1195 |
| reviewer_h_per_1k_western | 14.3522 | 13.2552 – 15.4492 |
| review_load_ratio_max_min_origin | 1.1176 | 1.0554 – 1.1798 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3155 | 0.1170% |
| cervical_false_closures | 0 | 1405 | 0.2620% |
| wrong_patient_released | 0 | 15158 | 0.0240% |
| released_without_granted_consent | 0 | 9939 | 0.0370% |
| duplicate_or_lost_outputs | 0 | 202977 | 0.0020% |
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

Review load by name origin: pooled over seeds {'arab': 41.5, 'filipino': 40.3, 'south_asian': 42.6, 'western': 41.1} items per 100 loops, max/min **1.0583**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1069 and 95th percentile 1.2175 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0458 and 95th percentile 1.0926; the observed pooled 1.0583 has p = 0.292.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.025, 'filipino': 0.932, 'south_asian': 0.97, 'western': 1.002}, max/min **1.0997** (no-bias 95th percentile 1.0853, p = 0.014; labels shuffled within seed and programme).
- cervical: {'arab': 61.3, 'filipino': 56.5, 'south_asian': 56.3, 'western': 58.8} items per 100 loops, max/min **1.0884** (no-bias 95th percentile 1.1803, p = 0.532).
- crc: {'arab': 68.6, 'filipino': 61.1, 'south_asian': 65.0, 'western': 66.5} items per 100 loops, max/min **1.123** (no-bias 95th percentile 1.099, p = 0.017).
- diabetic_retinopathy: {'arab': 14.0, 'filipino': 14.2, 'south_asian': 12.6, 'western': 15.7} items per 100 loops, max/min **1.2488** (no-bias 95th percentile 1.4041, p = 0.291).
- lung_nodule: {'arab': 11.6, 'filipino': 10.9, 'south_asian': 12.8, 'western': 10.9} items per 100 loops, max/min **1.1752** (no-bias 95th percentile 1.4858, p = 0.661).
CRC loops by age band: {'40-59': 64.5, '60-79': 67.9, '80+': 68.2, '<40': 66.8} review items per 100 loops, max/min **1.0581** (no-bias 95th percentile 1.0831, p = 0.225); recall {'40-59': 0.818, '60-79': 0.851, '80+': 0.843, '<40': 0.859}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 65.9, 'M': 68.6} review items per 100 loops, max/min **1.0411** (no-bias 95th percentile 1.0457, p = 0.089); recall {'F': 0.843, 'M': 0.84}; false closures {'F': 0, 'M': 0}.

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
| crc_recall | 0.7756 | 0.7615 – 0.7897 |
| crc_recall_no_review | 0.3196 | 0.2957 – 0.3435 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9072 | 0.8823 – 0.9321 |
| kpi_truth | 0.329 | 0.3066 – 0.3514 |
| kpi_wasl | 0.297 | 0.2713 – 0.3227 |
| kpi_eid_only | 0.1224 | 0.1141 – 0.1307 |
| kpi_abs_error_wasl | 0.032 | 0.0251 – 0.0389 |
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
| linkage_pair_f1 | 0.9376 | 0.935 – 0.9402 |
| reviewer_h_per_1k | 12.9274 | 12.1034 – 13.7514 |
| review_items_per_100_loops | 37.259 | 35.6283 – 38.8897 |
| throughput_ev_s | 590.7927 | 363.6775 – 817.9079 |
| review_items_per_100_arab | 36.5758 | 35.4297 – 37.7219 |
| review_items_per_100_filipino | 39.135 | 33.4184 – 44.8516 |
| review_items_per_100_south_asian | 39.1714 | 33.7608 – 44.582 |
| review_items_per_100_western | 35.5658 | 34.6965 – 36.4351 |
| reviewer_h_per_1k_arab | 12.7098 | 12.1299 – 13.2897 |
| reviewer_h_per_1k_filipino | 13.5674 | 11.1716 – 15.9632 |
| reviewer_h_per_1k_south_asian | 13.4778 | 11.559 – 15.3966 |
| reviewer_h_per_1k_western | 12.4608 | 11.9283 – 12.9933 |
| review_load_ratio_max_min_origin | 1.1816 | 1.0249 – 1.3383 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2999 | 0.1230% |
| cervical_false_closures | 0 | 1305 | 0.2820% |
| wrong_patient_released | 0 | 14334 | 0.0260% |
| released_without_granted_consent | 0 | 9482 | 0.0390% |
| duplicate_or_lost_outputs | 0 | 198294 | 0.0020% |
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

Review load by name origin: pooled over seeds {'arab': 36.6, 'filipino': 39.2, 'south_asian': 39.2, 'western': 35.6} items per 100 loops, max/min **1.1031**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1156 and 95th percentile 1.2404 (mean over seeds); 2 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0492 and 95th percentile 1.0977; the observed pooled 1.1031 has p = 0.041.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.011, 'filipino': 1.002, 'south_asian': 0.991, 'western': 0.974}, max/min **1.0371** (no-bias 95th percentile 1.088, p = 0.635; labels shuffled within seed and programme).
- cervical: {'arab': 51.9, 'filipino': 53.0, 'south_asian': 51.0, 'western': 47.8} items per 100 loops, max/min **1.1106** (no-bias 95th percentile 1.2027, p = 0.372).
- crc: {'arab': 61.2, 'filipino': 57.6, 'south_asian': 60.0, 'western': 60.5} items per 100 loops, max/min **1.063** (no-bias 95th percentile 1.1149, p = 0.375).
- diabetic_retinopathy: {'arab': 12.9, 'filipino': 16.3, 'south_asian': 12.9, 'western': 13.7} items per 100 loops, max/min **1.2677** (no-bias 95th percentile 1.4053, p = 0.248).
- lung_nodule: {'arab': 10.1, 'filipino': 11.6, 'south_asian': 9.7, 'western': 7.5} items per 100 loops, max/min **1.5502** (no-bias 95th percentile 1.5671, p = 0.06).
CRC loops by age band: {'40-59': 58.4, '60-79': 62.1, '80+': 61.2, '<40': 60.6} review items per 100 loops, max/min **1.0631** (no-bias 95th percentile 1.0879, p = 0.234); recall {'40-59': 0.771, '60-79': 0.782, '80+': 0.792, '<40': 0.763}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 60.8, 'M': 59.8} review items per 100 loops, max/min **1.0161** (no-bias 95th percentile 1.049, p = 0.546); recall {'F': 0.773, 'M': 0.783}; false closures {'F': 0, 'M': 0}.

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
