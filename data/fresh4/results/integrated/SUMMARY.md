# Final frozen evaluation

Code revision `bc25c6968ebf` · policy `v7strict` · seeds [1103, 1109, 1117, 1121, 1123] · source `fhir` · held-out persons only · frozen 2026-11-09T07:09:16.962130Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8254 | 0.815 – 0.8358 |
| crc_recall_no_review | 0.313 | 0.2961 – 0.3299 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9676 | 0.9577 – 0.9775 |
| kpi_truth | 0.3284 | 0.3179 – 0.3389 |
| kpi_wasl | 0.3172 | 0.3064 – 0.328 |
| kpi_eid_only | 0.125 | 0.1137 – 0.1363 |
| kpi_abs_error_wasl | 0.0112 | 0.0102 – 0.0122 |
| kpi_abs_error_eid_only | 0.2034 | 0.1934 – 0.2134 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9822 | 0.9784 – 0.986 |
| lung_state_accuracy | 0.992 | 0.9908 – 0.9932 |
| lung_escalation_recall | 0.9728 | 0.9362 – 1.0094 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9986 | 0.9975 – 0.9997 |
| dr_referral_recall | 0.9888 | 0.9757 – 1.0019 |
| dr_state_accuracy | 0.9988 | 0.9982 – 0.9994 |
| dr_triage_accuracy | 0.9992 | 0.9982 – 1.0002 |
| dr_overdue_accuracy | 0.9992 | 0.9986 – 0.9998 |
| dr_exam_within_deadline_truth | 0.3608 | 0.279 – 0.4426 |
| dr_exam_within_deadline_wasl | 0.3566 | 0.2725 – 0.4407 |
| breast_precision | 1 | 1.0 – 1.0 |
| breast_recall | 1 | 1.0 – 1.0 |
| breast_recall_loop_recall | 1 | 1.0 – 1.0 |
| breast_state_accuracy | 1 | 1.0 – 1.0 |
| breast_triage_accuracy | 1 | 1.0 – 1.0 |
| breast_overdue_accuracy | 1 | 1.0 – 1.0 |
| breast_recall_outcome_accuracy | 1 | 1.0 – 1.0 |
| breast_assessment_within_deadline_truth | 0.4256 | 0.1231 – 0.7281 |
| breast_assessment_within_deadline_wasl | 0.4256 | 0.1231 – 0.7281 |
| breast_abnormal_recall_rate_truth | 0.0972 | 0.0268 – 0.1676 |
| breast_abnormal_recall_rate_wasl | 0.0972 | 0.0268 – 0.1676 |
| breast_early_recall_rate_truth | 0.0028 | -0.005 – 0.0106 |
| breast_early_recall_rate_wasl | 0.0028 | -0.005 – 0.0106 |
| ldct_precision | 1 | 1.0 – 1.0 |
| ldct_recall | 1 | 1.0 – 1.0 |
| ldct_action_loop_recall | 1 | 1.0 – 1.0 |
| ldct_state_accuracy | 1 | 1.0 – 1.0 |
| ldct_triage_accuracy | 1 | 1.0 – 1.0 |
| ldct_overdue_accuracy | 1 | 1.0 – 1.0 |
| ldct_action_outcome_accuracy | 1 | 1.0 – 1.0 |
| ldct_next_step_within_clock_truth | 0.5976 | 0.5378 – 0.6574 |
| ldct_next_step_within_clock_wasl | 0.5976 | 0.5378 – 0.6574 |
| ldct_abnormal_recall_rate_truth | 0.1372 | 0.1318 – 0.1426 |
| ldct_abnormal_recall_rate_wasl | 0.1372 | 0.1318 – 0.1426 |
| ldct_cancer_detection_rate_truth | 0.008 | 0.0034 – 0.0126 |
| ldct_cancer_detection_rate_wasl | 0.008 | 0.0034 – 0.0126 |
| linkage_pair_f1 | 0.98 | 0.9777 – 0.9823 |
| reviewer_h_per_1k | 13.1406 | 13.0153 – 13.2659 |
| review_items_per_100_loops | 37.8252 | 36.9024 – 38.748 |
| throughput_ev_s | 352.7336 | 319.7487 – 385.7185 |
| review_items_per_100_arab | 37.4064 | 36.0231 – 38.7897 |
| review_items_per_100_filipino | 39.6888 | 37.7297 – 41.6479 |
| review_items_per_100_south_asian | 39.887 | 38.5427 – 41.2313 |
| review_items_per_100_western | 35.1528 | 34.2291 – 36.0765 |
| reviewer_h_per_1k_arab | 12.9046 | 12.6327 – 13.1765 |
| reviewer_h_per_1k_filipino | 13.8786 | 12.7531 – 15.0041 |
| reviewer_h_per_1k_south_asian | 14.0982 | 13.563 – 14.6334 |
| reviewer_h_per_1k_western | 12.1214 | 11.5348 – 12.708 |
| review_load_ratio_max_min_origin | 1.1541 | 1.1135 – 1.1946 |
| ifhas_abnormal_findings_rate_truth | 0.4022 | 0.383 – 0.4214 |
| ifhas_abnormal_findings_rate_wasl | 0.403 | 0.3836 – 0.4224 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.9662 | 0.9515 – 0.9809 |
| ifhas_followup_within_clock_truth | 0.557 | 0.5347 – 0.5793 |
| ifhas_followup_within_clock_wasl | 0.5392 | 0.5139 – 0.5645 |
| ifhas_overdue_accuracy | 0.9908 | 0.984 – 0.9976 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.9878 | 0.9818 – 0.9938 |
| ifhas_state_accuracy | 0.9892 | 0.9841 – 0.9943 |
| ifhas_triage_accuracy | 0.9992 | 0.9982 – 1.0002 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3231 | 0.1140% |
| cervical_false_closures | 0 | 1364 | 0.2700% |
| wrong_patient_released | 0 | 17377 | 0.0210% |
| released_without_granted_consent | 0 | 11994 | 0.0310% |
| duplicate_or_lost_outputs | 0 | 221992 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 274134 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 114 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 0 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 2031 | 0.1810% |
| lung_surveillance_ct_false_closures | 0 | 31 | 11.2190% |
| lung_surveillance_ct_overdue_flag_correct | 87 | 87 | – |
| dr_false_closures | 0 | 5630 | 0.0660% |
| dr_referral_false_closures | 0 | 277 | 1.3230% |
| breast_false_closures | 0 | 341 | 1.0760% |
| breast_recall_false_closures | 0 | 28 | 12.3440% |
| ldct_false_closures | 0 | 1392 | 0.2650% |
| ldct_action_false_closures | 0 | 104 | 3.4850% |
| ifhas_false_closures | 0 | 3487 | 0.1060% |
| ifhas_followup_false_closures | 0 | 1099 | 0.3350% |
| handoff_false_closures | 0 | 87 | 4.1510% |
| handoff_lung_false_closures | 0 | 11 | 28.4910% |
| handoff_dr_false_closures | 0 | 21 | 16.1100% |

Review load by name origin: pooled over seeds {'arab': 37.4, 'filipino': 39.7, 'south_asian': 39.9, 'western': 35.2} items per 100 loops, max/min **1.1341**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.105 and 95th percentile 1.214 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0468 and 95th percentile 1.0911; the observed pooled 1.1341 has p = 0.001.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.02, 'filipino': 0.979, 'south_asian': 1.002, 'western': 0.938}, max/min **1.087** (no-bias 95th percentile 1.0785, p = 0.025; labels shuffled within seed and programme).
- breast: {'arab': 3.9, 'filipino': 0.0, 'south_asian': 3.1, 'western': 8.3} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.504).
- cervical: {'arab': 62.3, 'filipino': 55.6, 'south_asian': 55.0, 'western': 58.1} items per 100 loops, max/min **1.1337** (no-bias 95th percentile 1.1708, p = 0.157).
- crc: {'arab': 71.1, 'filipino': 70.6, 'south_asian': 71.0, 'western': 63.9} items per 100 loops, max/min **1.1129** (no-bias 95th percentile 1.0981, p = 0.026).
- diabetic_retinopathy: {'arab': 13.8, 'filipino': 15.3, 'south_asian': 14.7, 'western': 14.0} items per 100 loops, max/min **1.1128** (no-bias 95th percentile 1.4056, p = 0.802).
- ifhas: {'arab': 21.4, 'filipino': 17.0, 'south_asian': 20.6, 'western': 20.9} items per 100 loops, max/min **1.2595** (no-bias 95th percentile 1.4823, p = 0.396).
- lung_ldct: {'arab': 2.4, 'filipino': 2.0, 'south_asian': 2.5, 'western': 2.3} items per 100 loops, max/min **1.2082** (no-bias 95th percentile inf, p = 1.0).
- lung_nodule: {'arab': 12.3, 'filipino': 10.6, 'south_asian': 13.9, 'western': 11.1} items per 100 loops, max/min **1.3102** (no-bias 95th percentile 1.5195, p = 0.271).
CRC loops by age band: {'40-59': 67.4, '60-79': 72.1, '80+': 71.5, '<40': 69.5} review items per 100 loops, max/min **1.0689** (no-bias 95th percentile 1.0835, p = 0.118); recall {'40-59': 0.828, '60-79': 0.825, '80+': 0.826, '<40': 0.823}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 69.8, 'M': 70.4} review items per 100 loops, max/min **1.0084** (no-bias 95th percentile 1.0469, p = 0.685); recall {'F': 0.83, 'M': 0.814}; false closures {'F': 0, 'M': 0}.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off, DR, breast and LDCT readers, PET-low surveillance CT) | Count |
|---|---|
| breast_reader_breast-assess_agree | 78 |
| breast_reader_breast-assess_agree_but_wrong | 0 |
| breast_reader_breast-assess_coded | 0 |
| breast_reader_breast-assess_instances | 78 |
| breast_reader_breast-screen_agree | 351 |
| breast_reader_breast-screen_agree_but_wrong | 0 |
| breast_reader_breast-screen_coded | 243 |
| breast_reader_breast-screen_instances | 355 |
| dr_reader_dr-clinic_agree | 1183 |
| dr_reader_dr-clinic_agree_but_wrong | 0 |
| dr_reader_dr-clinic_coded | 0 |
| dr_reader_dr-clinic_instances | 1214 |
| dr_reader_dr-screen_agree | 5525 |
| dr_reader_dr-screen_agree_but_wrong | 0 |
| dr_reader_dr-screen_coded | 3771 |
| dr_reader_dr-screen_instances | 6224 |
| handoff_acted_state_correct | 316 |
| handoff_closed | 87 |
| handoff_dr_acted_state_correct | 40 |
| handoff_dr_closed | 21 |
| handoff_dr_packets_all_required_fields | 40 |
| handoff_dr_packets_matched | 40 |
| handoff_dr_truth_acted_by_act_by | 19 |
| handoff_dr_truth_closed_after_handoff | 21 |
| handoff_dr_truth_packets | 40 |
| handoff_dr_truth_referrer_acted | 33 |
| handoff_dr_truth_urgent | 14 |
| handoff_lung_acted_state_correct | 27 |
| handoff_lung_closed | 11 |
| handoff_lung_packets_all_required_fields | 27 |
| handoff_lung_packets_matched | 27 |
| handoff_lung_packets_missed | 1 |
| handoff_lung_truth_acted_by_act_by | 14 |
| handoff_lung_truth_closed_after_handoff | 12 |
| handoff_lung_truth_packets | 28 |
| handoff_lung_truth_referrer_acted | 21 |
| handoff_lung_truth_urgent | 28 |
| handoff_packets_all_required_fields | 316 |
| handoff_packets_matched | 316 |
| handoff_packets_missed | 1 |
| handoff_truth_acted_by_act_by | 154 |
| handoff_truth_closed_after_handoff | 100 |
| handoff_truth_packets | 317 |
| handoff_truth_referrer_acted | 238 |
| handoff_truth_urgent | 219 |
| ifhas_reader_ifhas-followup_agree | 1751 |
| ifhas_reader_ifhas-followup_agree_but_wrong | 0 |
| ifhas_reader_ifhas-followup_instances | 2186 |
| ifhas_reader_ifhas-screen_agree | 3897 |
| ifhas_reader_ifhas-screen_agree_but_wrong | 0 |
| ifhas_reader_ifhas-screen_instances | 4000 |
| ldct_reader_ldct-screen_agree | 1575 |
| ldct_reader_ldct-screen_agree_but_wrong | 0 |
| ldct_reader_ldct-screen_coded | 972 |
| ldct_reader_ldct-screen_instances | 1591 |
| ldct_reader_ldct-workup_agree | 210 |
| ldct_reader_ldct-workup_agree_but_wrong | 0 |
| ldct_reader_ldct-workup_coded | 0 |
| ldct_reader_ldct-workup_instances | 238 |
| lung_surv_extra_opened | 0 |
| lung_surv_false_closures | 0 |
| lung_surv_missed | 0 |
| lung_surv_obligations | 87 |
| lung_surv_opened | 86 |
| lung_surv_outreach_lung_surveillance_due:released | 2 |
| lung_surv_outreach_lung_surveillance_recall:consent_unknown_hold | 2 |
| lung_surv_outreach_lung_surveillance_recall:consent_withheld | 1 |
| lung_surv_outreach_lung_surveillance_recall:released | 24 |
| lung_surv_overdue_flag_correct | 87 |
| lung_surv_pred_closed | 31 |
| lung_surv_pred_overdue | 27 |
| lung_surv_tp | 31 |
| lung_surv_truth_closed | 31 |
| lung_surv_truth_overdue | 27 |
| lung_workup_pred_awaiting_tissue | 13 |
| lung_workup_pred_closed_by_ct | 94 |
| lung_workup_pred_closed_by_pet | 86 |
| lung_workup_pred_closed_by_tissue | 51 |
| lung_workup_truth_awaiting_tissue | 13 |
| lung_workup_truth_path_ct | 132 |
| lung_workup_truth_path_pet | 162 |
| lung_workup_truth_path_tissue | 55 |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7716 | 0.7574 – 0.7858 |
| crc_recall_no_review | 0.3174 | 0.3103 – 0.3245 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.8974 | 0.8667 – 0.9281 |
| kpi_truth | 0.3276 | 0.3171 – 0.3381 |
| kpi_wasl | 0.292 | 0.2799 – 0.3041 |
| kpi_eid_only | 0.123 | 0.1125 – 0.1335 |
| kpi_abs_error_wasl | 0.0356 | 0.0312 – 0.04 |
| kpi_abs_error_eid_only | 0.2046 | 0.1951 – 0.2141 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9464 | 0.9224 – 0.9704 |
| lung_state_accuracy | 0.9756 | 0.9668 – 0.9844 |
| lung_escalation_recall | 0.8892 | 0.8354 – 0.943 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9948 | 0.9934 – 0.9962 |
| dr_referral_recall | 0.9086 | 0.8726 – 0.9446 |
| dr_state_accuracy | 0.994 | 0.9925 – 0.9955 |
| dr_triage_accuracy | 0.9992 | 0.9982 – 1.0002 |
| dr_overdue_accuracy | 0.995 | 0.9932 – 0.9968 |
| dr_exam_within_deadline_truth | 0.3938 | 0.3456 – 0.442 |
| dr_exam_within_deadline_wasl | 0.3646 | 0.3257 – 0.4035 |
| breast_precision | 1 | 1.0 – 1.0 |
| breast_recall | 0.997 | 0.9887 – 1.0053 |
| breast_recall_loop_recall | 0.96 | 0.849 – 1.071 |
| breast_state_accuracy | 0.9972 | 0.9894 – 1.005 |
| breast_triage_accuracy | 1 | 1.0 – 1.0 |
| breast_overdue_accuracy | 0.9972 | 0.9894 – 1.005 |
| breast_recall_outcome_accuracy | 1 | 1.0 – 1.0 |
| breast_assessment_within_deadline_truth | 0.6812 | 0.2625 – 1.0999 |
| breast_assessment_within_deadline_wasl | 0.6812 | 0.2625 – 1.0999 |
| breast_abnormal_recall_rate_truth | 0.077 | 0.0099 – 0.1441 |
| breast_abnormal_recall_rate_wasl | 0.077 | 0.0099 – 0.1441 |
| breast_early_recall_rate_truth | 0.0028 | -0.005 – 0.0106 |
| breast_early_recall_rate_wasl | 0.0028 | -0.005 – 0.0106 |
| ldct_precision | 1 | 1.0 – 1.0 |
| ldct_recall | 0.9992 | 0.997 – 1.0014 |
| ldct_action_loop_recall | 0.9916 | 0.9683 – 1.0149 |
| ldct_state_accuracy | 0.9994 | 0.9977 – 1.0011 |
| ldct_triage_accuracy | 1 | 1.0 – 1.0 |
| ldct_overdue_accuracy | 0.9994 | 0.9977 – 1.0011 |
| ldct_action_outcome_accuracy | 1 | 1.0 – 1.0 |
| ldct_next_step_within_clock_truth | 0.5552 | 0.465 – 0.6454 |
| ldct_next_step_within_clock_wasl | 0.5494 | 0.4546 – 0.6442 |
| ldct_abnormal_recall_rate_truth | 0.1242 | 0.0983 – 0.1501 |
| ldct_abnormal_recall_rate_wasl | 0.1242 | 0.0983 – 0.1501 |
| ldct_cancer_detection_rate_truth | 0.0074 | 0.003 – 0.0118 |
| ldct_cancer_detection_rate_wasl | 0.0074 | 0.003 – 0.0118 |
| linkage_pair_f1 | 0.9412 | 0.9373 – 0.9451 |
| reviewer_h_per_1k | 11.8162 | 11.3652 – 12.2672 |
| review_items_per_100_loops | 33.5444 | 32.8602 – 34.2286 |
| throughput_ev_s | 451.8858 | 351.85 – 551.9217 |
| review_items_per_100_arab | 33.6602 | 32.62 – 34.7004 |
| review_items_per_100_filipino | 34.4494 | 33.245 – 35.6538 |
| review_items_per_100_south_asian | 34.097 | 31.6758 – 36.5182 |
| review_items_per_100_western | 31.7452 | 29.981 – 33.5094 |
| reviewer_h_per_1k_arab | 11.7766 | 11.1904 – 12.3628 |
| reviewer_h_per_1k_filipino | 12.1246 | 11.6565 – 12.5927 |
| reviewer_h_per_1k_south_asian | 11.931 | 10.9062 – 12.9558 |
| reviewer_h_per_1k_western | 11.598 | 10.6478 – 12.5482 |
| review_load_ratio_max_min_origin | 1.1191 | 1.0736 – 1.1647 |
| ifhas_abnormal_findings_rate_truth | 0.3982 | 0.3756 – 0.4208 |
| ifhas_abnormal_findings_rate_wasl | 0.3984 | 0.3754 – 0.4214 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.9264 | 0.9033 – 0.9495 |
| ifhas_followup_within_clock_truth | 0.5636 | 0.5158 – 0.6114 |
| ifhas_followup_within_clock_wasl | 0.5258 | 0.4696 – 0.582 |
| ifhas_overdue_accuracy | 0.98 | 0.9742 – 0.9858 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.976 | 0.9687 – 0.9833 |
| ifhas_state_accuracy | 0.978 | 0.9713 – 0.9847 |
| ifhas_triage_accuracy | 0.9994 | 0.9983 – 1.0005 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2925 | 0.1260% |
| cervical_false_closures | 0 | 1234 | 0.2980% |
| wrong_patient_released | 0 | 16365 | 0.0230% |
| released_without_granted_consent | 0 | 11548 | 0.0320% |
| duplicate_or_lost_outputs | 0 | 215301 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 273435 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 108 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 5 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1881 | 0.1960% |
| lung_surveillance_ct_false_closures | 0 | 29 | 11.9440% |
| lung_surveillance_ct_overdue_flag_correct | 76 | 82 | – |
| dr_false_closures | 0 | 5607 | 0.0660% |
| dr_referral_false_closures | 0 | 260 | 1.4090% |
| breast_false_closures | 0 | 340 | 1.0790% |
| breast_recall_false_closures | 0 | 21 | 16.1100% |
| ldct_false_closures | 0 | 1413 | 0.2610% |
| ldct_action_false_closures | 0 | 108 | 3.3580% |
| ifhas_false_closures | 0 | 3486 | 0.1060% |
| ifhas_followup_false_closures | 0 | 1079 | 0.3410% |
| handoff_false_closures | 0 | 80 | 4.5060% |
| handoff_lung_false_closures | 0 | 8 | 36.9420% |
| handoff_dr_false_closures | 0 | 9 | 33.6270% |

Review load by name origin: pooled over seeds {'arab': 33.7, 'filipino': 34.4, 'south_asian': 34.1, 'western': 31.8} items per 100 loops, max/min **1.084**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1133 and 95th percentile 1.2339 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0507 and 95th percentile 1.0989; the observed pooled 1.084 has p = 0.122.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.034, 'filipino': 0.963, 'south_asian': 0.96, 'western': 0.966}, max/min **1.0767** (no-bias 95th percentile 1.0889, p = 0.102; labels shuffled within seed and programme).
- breast: {'arab': 4.4, 'filipino': 0.0, 'south_asian': 4.4, 'western': 0.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.673).
- cervical: {'arab': 55.5, 'filipino': 49.3, 'south_asian': 50.0, 'western': 51.0} items per 100 loops, max/min **1.1246** (no-bias 95th percentile 1.1819, p = 0.262).
- crc: {'arab': 64.3, 'filipino': 58.5, 'south_asian': 57.6, 'western': 60.4} items per 100 loops, max/min **1.1159** (no-bias 95th percentile 1.1095, p = 0.035).
- diabetic_retinopathy: {'arab': 14.3, 'filipino': 14.2, 'south_asian': 15.6, 'western': 14.2} items per 100 loops, max/min **1.0985** (no-bias 95th percentile 1.4055, p = 0.843).
- ifhas: {'arab': 18.3, 'filipino': 21.7, 'south_asian': 20.1, 'western': 18.2} items per 100 loops, max/min **1.1968** (no-bias 95th percentile 1.5428, p = 0.647).
- lung_ldct: {'arab': 1.6, 'filipino': 0.9, 'south_asian': 2.2, 'western': 1.2} items per 100 loops, max/min **2.2796** (no-bias 95th percentile inf, p = 0.677).
- lung_nodule: {'arab': 9.7, 'filipino': 9.8, 'south_asian': 9.5, 'western': 7.4} items per 100 loops, max/min **1.317** (no-bias 95th percentile 1.5925, p = 0.329).
CRC loops by age band: {'40-59': 60.9, '60-79': 63.0, '80+': 61.6, '<40': 60.8} review items per 100 loops, max/min **1.0356** (no-bias 95th percentile 1.0885, p = 0.719); recall {'40-59': 0.766, '60-79': 0.776, '80+': 0.767, '<40': 0.777}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 62.2, 'M': 60.0} review items per 100 loops, max/min **1.0373** (no-bias 95th percentile 1.0501, p = 0.138); recall {'F': 0.78, 'M': 0.752}; false closures {'F': 0, 'M': 0}.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off, DR, breast and LDCT readers, PET-low surveillance CT) | Count |
|---|---|
| breast_reader_breast-assess_agree | 68 |
| breast_reader_breast-assess_agree_but_wrong | 0 |
| breast_reader_breast-assess_coded | 0 |
| breast_reader_breast-assess_instances | 68 |
| breast_reader_breast-screen_agree | 352 |
| breast_reader_breast-screen_agree_but_wrong | 0 |
| breast_reader_breast-screen_coded | 237 |
| breast_reader_breast-screen_instances | 354 |
| dr_reader_dr-clinic_agree | 1174 |
| dr_reader_dr-clinic_agree_but_wrong | 0 |
| dr_reader_dr-clinic_coded | 0 |
| dr_reader_dr-clinic_instances | 1206 |
| dr_reader_dr-screen_agree | 5478 |
| dr_reader_dr-screen_agree_but_wrong | 0 |
| dr_reader_dr-screen_coded | 3692 |
| dr_reader_dr-screen_instances | 6219 |
| handoff_acted_state_correct | 339 |
| handoff_closed | 80 |
| handoff_dr_acted_state_correct | 20 |
| handoff_dr_closed | 9 |
| handoff_dr_packets_all_required_fields | 20 |
| handoff_dr_packets_matched | 20 |
| handoff_dr_truth_acted_by_act_by | 9 |
| handoff_dr_truth_closed_after_handoff | 9 |
| handoff_dr_truth_packets | 20 |
| handoff_dr_truth_referrer_acted | 15 |
| handoff_dr_truth_urgent | 4 |
| handoff_lung_acted_state_correct | 23 |
| handoff_lung_closed | 8 |
| handoff_lung_packets_all_required_fields | 23 |
| handoff_lung_packets_matched | 23 |
| handoff_lung_truth_acted_by_act_by | 5 |
| handoff_lung_truth_closed_after_handoff | 8 |
| handoff_lung_truth_packets | 23 |
| handoff_lung_truth_referrer_acted | 13 |
| handoff_lung_truth_urgent | 23 |
| handoff_packets_all_required_fields | 339 |
| handoff_packets_matched | 339 |
| handoff_packets_missed | 1 |
| handoff_truth_acted_by_act_by | 159 |
| handoff_truth_closed_after_handoff | 92 |
| handoff_truth_packets | 340 |
| handoff_truth_referrer_acted | 255 |
| handoff_truth_urgent | 256 |
| ifhas_reader_ifhas-followup_agree | 1782 |
| ifhas_reader_ifhas-followup_agree_but_wrong | 0 |
| ifhas_reader_ifhas-followup_instances | 2203 |
| ifhas_reader_ifhas-screen_agree | 3885 |
| ifhas_reader_ifhas-screen_agree_but_wrong | 0 |
| ifhas_reader_ifhas-screen_instances | 4000 |
| ldct_reader_ldct-screen_agree | 1587 |
| ldct_reader_ldct-screen_agree_but_wrong | 0 |
| ldct_reader_ldct-screen_coded | 952 |
| ldct_reader_ldct-screen_instances | 1598 |
| ldct_reader_ldct-workup_agree | 200 |
| ldct_reader_ldct-workup_agree_but_wrong | 0 |
| ldct_reader_ldct-workup_coded | 0 |
| ldct_reader_ldct-workup_instances | 238 |
| lung_surv_extra_opened | 0 |
| lung_surv_false_closures | 0 |
| lung_surv_missed | 4 |
| lung_surv_obligations | 82 |
| lung_surv_opened | 70 |
| lung_surv_outreach_lung_surveillance_recall:consent_unknown_hold | 3 |
| lung_surv_outreach_lung_surveillance_recall:consent_withheld | 2 |
| lung_surv_outreach_lung_surveillance_recall:released | 16 |
| lung_surv_overdue_flag_correct | 76 |
| lung_surv_pred_closed | 29 |
| lung_surv_pred_overdue | 21 |
| lung_surv_tp | 29 |
| lung_surv_truth_closed | 33 |
| lung_surv_truth_overdue | 27 |
| lung_workup_pred_awaiting_tissue | 8 |
| lung_workup_pred_closed_by_ct | 93 |
| lung_workup_pred_closed_by_pet | 70 |
| lung_workup_pred_closed_by_tissue | 42 |
| lung_workup_truth_awaiting_tissue | 8 |
| lung_workup_truth_path_ct | 137 |
| lung_workup_truth_path_pet | 146 |
| lung_workup_truth_path_tissue | 62 |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity
- 43 MTSamples report texts reused across loops; clinically verified model-assisted answer key; not independent human double-labelling
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
