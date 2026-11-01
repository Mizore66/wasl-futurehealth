# Final frozen evaluation

Code revision `73ec4e4c1314` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-11-01T12:59:41.204084Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8446 | 0.8243 – 0.8649 |
| crc_recall_no_review | 0.325 | 0.2991 – 0.3509 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9716 | 0.9551 – 0.9881 |
| kpi_truth | 0.323 | 0.3097 – 0.3363 |
| kpi_wasl | 0.3138 | 0.2988 – 0.3288 |
| kpi_eid_only | 0.1284 | 0.1197 – 0.1371 |
| kpi_abs_error_wasl | 0.0092 | 0.0055 – 0.0129 |
| kpi_abs_error_eid_only | 0.1946 | 0.1849 – 0.2043 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9826 | 0.9724 – 0.9928 |
| lung_state_accuracy | 0.9924 | 0.9884 – 0.9964 |
| lung_escalation_recall | 0.9828 | 0.9704 – 0.9952 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9974 | 0.996 – 0.9988 |
| dr_referral_recall | 0.9696 | 0.9456 – 0.9936 |
| dr_state_accuracy | 0.9978 | 0.9964 – 0.9992 |
| dr_triage_accuracy | 0.9988 | 0.9978 – 0.9998 |
| dr_overdue_accuracy | 0.9982 | 0.9966 – 0.9998 |
| dr_exam_within_deadline_truth | 0.3606 | 0.3062 – 0.415 |
| dr_exam_within_deadline_wasl | 0.3458 | 0.2917 – 0.3999 |
| breast_precision | 1 | 1.0 – 1.0 |
| breast_recall | 0.9972 | 0.9894 – 1.005 |
| breast_recall_loop_recall | 0.9818 | 0.9313 – 1.0323 |
| breast_state_accuracy | 0.9972 | 0.9894 – 1.005 |
| breast_triage_accuracy | 1 | 1.0 – 1.0 |
| breast_overdue_accuracy | 0.9972 | 0.9894 – 1.005 |
| breast_recall_outcome_accuracy | 1 | 1.0 – 1.0 |
| breast_assessment_within_deadline_truth | 0.5422 | 0.1482 – 0.9362 |
| breast_assessment_within_deadline_wasl | 0.524 | 0.1435 – 0.9045 |
| breast_abnormal_recall_rate_truth | 0.103 | 0.0493 – 0.1567 |
| breast_abnormal_recall_rate_wasl | 0.103 | 0.0493 – 0.1567 |
| breast_early_recall_rate_truth | 0.0056 | -0.0039 – 0.0151 |
| breast_early_recall_rate_wasl | 0.0056 | -0.0039 – 0.0151 |
| ldct_precision | 1 | 1.0 – 1.0 |
| ldct_recall | 0.9978 | 0.9917 – 1.0039 |
| ldct_action_loop_recall | 0.976 | 0.9094 – 1.0426 |
| ldct_state_accuracy | 0.998 | 0.9924 – 1.0036 |
| ldct_triage_accuracy | 1 | 1.0 – 1.0 |
| ldct_overdue_accuracy | 0.998 | 0.9924 – 1.0036 |
| ldct_action_outcome_accuracy | 1 | 1.0 – 1.0 |
| ldct_next_step_within_clock_truth | 0.5534 | 0.3947 – 0.7121 |
| ldct_next_step_within_clock_wasl | 0.5396 | 0.4003 – 0.6789 |
| ldct_abnormal_recall_rate_truth | 0.1372 | 0.1239 – 0.1505 |
| ldct_abnormal_recall_rate_wasl | 0.1372 | 0.1239 – 0.1505 |
| ldct_cancer_detection_rate_truth | 0.0106 | -0.0002 – 0.0214 |
| ldct_cancer_detection_rate_wasl | 0.0092 | 0.0001 – 0.0183 |
| linkage_pair_f1 | 0.9752 | 0.9742 – 0.9762 |
| reviewer_h_per_1k | 12.7178 | 12.3065 – 13.1291 |
| review_items_per_100_loops | 36.5636 | 35.6089 – 37.5183 |
| throughput_ev_s | 360.2768 | 350.2697 – 370.2839 |
| review_items_per_100_arab | 36.2 | 34.9947 – 37.4053 |
| review_items_per_100_filipino | 35.8002 | 33.6126 – 37.9878 |
| review_items_per_100_south_asian | 37.6924 | 34.3696 – 41.0152 |
| review_items_per_100_western | 36.5622 | 35.4188 – 37.7056 |
| reviewer_h_per_1k_arab | 12.5984 | 12.2481 – 12.9487 |
| reviewer_h_per_1k_filipino | 12.4944 | 11.393 – 13.5958 |
| reviewer_h_per_1k_south_asian | 13.1946 | 11.6492 – 14.74 |
| reviewer_h_per_1k_western | 12.54 | 11.957 – 13.123 |
| review_load_ratio_max_min_origin | 1.1065 | 1.0243 – 1.1888 |
| ifhas_abnormal_findings_rate_truth | 0.4148 | 0.396 – 0.4336 |
| ifhas_abnormal_findings_rate_wasl | 0.4152 | 0.3955 – 0.4349 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.9682 | 0.9577 – 0.9787 |
| ifhas_followup_within_clock_truth | 0.5432 | 0.5286 – 0.5578 |
| ifhas_followup_within_clock_wasl | 0.524 | 0.5085 – 0.5395 |
| ifhas_overdue_accuracy | 0.991 | 0.9872 – 0.9948 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.9886 | 0.984 – 0.9932 |
| ifhas_state_accuracy | 0.9902 | 0.9861 – 0.9943 |
| ifhas_triage_accuracy | 0.9998 | 0.9992 – 1.0004 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3163 | 0.1170% |
| cervical_false_closures | 0 | 1410 | 0.2610% |
| wrong_patient_released | 0 | 17041 | 0.0220% |
| released_without_granted_consent | 0 | 12380 | 0.0300% |
| duplicate_or_lost_outputs | 0 | 220693 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 273849 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 116 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 2024 | 0.1820% |
| lung_surveillance_ct_false_closures | 0 | 41 | 8.6040% |
| lung_surveillance_ct_overdue_flag_correct | 102 | 102 | – |
| dr_false_closures | 0 | 5634 | 0.0650% |
| dr_referral_false_closures | 0 | 267 | 1.3720% |
| breast_false_closures | 0 | 345 | 1.0640% |
| breast_recall_false_closures | 0 | 31 | 11.2190% |
| ldct_false_closures | 0 | 1398 | 0.2640% |
| ldct_action_false_closures | 0 | 109 | 3.3280% |
| ifhas_false_closures | 0 | 3481 | 0.1060% |
| ifhas_followup_false_closures | 0 | 1141 | 0.3230% |
| handoff_false_closures | 0 | 88 | 4.1050% |
| handoff_lung_false_closures | 0 | 5 | 52.1820% |
| handoff_dr_false_closures | 0 | 14 | 23.1640% |

Review load by name origin: pooled over seeds {'arab': 36.2, 'filipino': 35.8, 'south_asian': 37.7, 'western': 36.6} items per 100 loops, max/min **1.0533**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1079 and 95th percentile 1.2253 (mean over seeds); 1 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0457 and 95th percentile 1.0907; the observed pooled 1.0533 has p = 0.38.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.022, 'filipino': 0.932, 'south_asian': 0.973, 'western': 1.007}, max/min **1.0971** (no-bias 95th percentile 1.0799, p = 0.013; labels shuffled within seed and programme).
- breast: {'arab': 4.2, 'filipino': 3.1, 'south_asian': 1.2, 'western': 0.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.658).
- cervical: {'arab': 61.3, 'filipino': 55.9, 'south_asian': 55.9, 'western': 58.3} items per 100 loops, max/min **1.0958** (no-bias 95th percentile 1.1816, p = 0.459).
- crc: {'arab': 68.5, 'filipino': 61.0, 'south_asian': 65.2, 'western': 66.2} items per 100 loops, max/min **1.1231** (no-bias 95th percentile 1.1035, p = 0.01).
- diabetic_retinopathy: {'arab': 14.3, 'filipino': 14.2, 'south_asian': 12.9, 'western': 16.0} items per 100 loops, max/min **1.2341** (no-bias 95th percentile 1.385, p = 0.273).
- ifhas: {'arab': 19.5, 'filipino': 20.1, 'south_asian': 20.0, 'western': 24.2} items per 100 loops, max/min **1.2397** (no-bias 95th percentile 1.5358, p = 0.452).
- lung_ldct: {'arab': 2.1, 'filipino': 1.7, 'south_asian': 3.4, 'western': 1.2} items per 100 loops, max/min **2.9456** (no-bias 95th percentile inf, p = 0.426).
- lung_nodule: {'arab': 11.9, 'filipino': 10.7, 'south_asian': 12.6, 'western': 10.8} items per 100 loops, max/min **1.1751** (no-bias 95th percentile 1.445, p = 0.647).
CRC loops by age band: {'40-59': 64.4, '60-79': 67.7, '80+': 68.1, '<40': 66.9} review items per 100 loops, max/min **1.0577** (no-bias 95th percentile 1.0826, p = 0.226); recall {'40-59': 0.816, '60-79': 0.861, '80+': 0.835, '<40': 0.866}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 65.8, 'M': 68.6} review items per 100 loops, max/min **1.0434** (no-bias 95th percentile 1.0449, p = 0.057); recall {'F': 0.847, 'M': 0.838}; false closures {'F': 0, 'M': 0}.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off, DR, breast and LDCT readers, PET-low surveillance CT) | Count |
|---|---|
| breast_reader_breast-assess_agree | 83 |
| breast_reader_breast-assess_agree_but_wrong | 0 |
| breast_reader_breast-assess_coded | 0 |
| breast_reader_breast-assess_instances | 83 |
| breast_reader_breast-screen_agree | 356 |
| breast_reader_breast-screen_agree_but_wrong | 0 |
| breast_reader_breast-screen_coded | 248 |
| breast_reader_breast-screen_instances | 357 |
| dr_reader_dr-clinic_agree | 1190 |
| dr_reader_dr-clinic_agree_but_wrong | 0 |
| dr_reader_dr-clinic_coded | 0 |
| dr_reader_dr-clinic_instances | 1207 |
| dr_reader_dr-screen_agree | 5513 |
| dr_reader_dr-screen_agree_but_wrong | 0 |
| dr_reader_dr-screen_coded | 3724 |
| dr_reader_dr-screen_instances | 6210 |
| handoff_acted_state_correct | 333 |
| handoff_closed | 88 |
| handoff_dr_acted_state_correct | 32 |
| handoff_dr_closed | 14 |
| handoff_dr_packets_all_required_fields | 32 |
| handoff_dr_packets_matched | 32 |
| handoff_dr_packets_missed | 1 |
| handoff_dr_truth_acted_by_act_by | 16 |
| handoff_dr_truth_closed_after_handoff | 15 |
| handoff_dr_truth_packets | 33 |
| handoff_dr_truth_referrer_acted | 28 |
| handoff_dr_truth_urgent | 15 |
| handoff_lung_acted_state_correct | 18 |
| handoff_lung_closed | 5 |
| handoff_lung_packets_all_required_fields | 18 |
| handoff_lung_packets_matched | 18 |
| handoff_lung_packets_missed | 2 |
| handoff_lung_truth_acted_by_act_by | 7 |
| handoff_lung_truth_closed_after_handoff | 6 |
| handoff_lung_truth_packets | 20 |
| handoff_lung_truth_referrer_acted | 10 |
| handoff_lung_truth_urgent | 20 |
| handoff_packets_all_required_fields | 333 |
| handoff_packets_matched | 333 |
| handoff_truth_acted_by_act_by | 152 |
| handoff_truth_closed_after_handoff | 96 |
| handoff_truth_packets | 333 |
| handoff_truth_referrer_acted | 256 |
| handoff_truth_urgent | 248 |
| ifhas_reader_ifhas-followup_agree | 1832 |
| ifhas_reader_ifhas-followup_agree_but_wrong | 0 |
| ifhas_reader_ifhas-followup_instances | 2257 |
| ifhas_reader_ifhas-screen_agree | 3860 |
| ifhas_reader_ifhas-screen_agree_but_wrong | 0 |
| ifhas_reader_ifhas-screen_instances | 4000 |
| ldct_reader_ldct-screen_agree | 1577 |
| ldct_reader_ldct-screen_agree_but_wrong | 0 |
| ldct_reader_ldct-screen_coded | 932 |
| ldct_reader_ldct-screen_instances | 1594 |
| ldct_reader_ldct-workup_agree | 224 |
| ldct_reader_ldct-workup_agree_but_wrong | 0 |
| ldct_reader_ldct-workup_coded | 0 |
| ldct_reader_ldct-workup_instances | 259 |
| lung_surv_extra_opened | 0 |
| lung_surv_false_closures | 0 |
| lung_surv_missed | 1 |
| lung_surv_obligations | 102 |
| lung_surv_opened | 97 |
| lung_surv_outreach_lung_surveillance_due:released | 2 |
| lung_surv_outreach_lung_surveillance_recall:consent_unknown_hold | 3 |
| lung_surv_outreach_lung_surveillance_recall:consent_withheld | 1 |
| lung_surv_outreach_lung_surveillance_recall:released | 16 |
| lung_surv_overdue_flag_correct | 102 |
| lung_surv_pred_closed | 41 |
| lung_surv_pred_overdue | 20 |
| lung_surv_tp | 41 |
| lung_surv_truth_closed | 42 |
| lung_surv_truth_overdue | 20 |
| lung_workup_pred_awaiting_tissue | 11 |
| lung_workup_pred_closed_by_ct | 93 |
| lung_workup_pred_closed_by_pet | 97 |
| lung_workup_pred_closed_by_tissue | 50 |
| lung_workup_truth_awaiting_tissue | 11 |
| lung_workup_truth_path_ct | 127 |
| lung_workup_truth_path_pet | 171 |
| lung_workup_truth_path_tissue | 55 |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7862 | 0.7729 – 0.7995 |
| crc_recall_no_review | 0.3196 | 0.2957 – 0.3435 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9064 | 0.8764 – 0.9364 |
| kpi_truth | 0.329 | 0.3066 – 0.3514 |
| kpi_wasl | 0.2992 | 0.273 – 0.3254 |
| kpi_eid_only | 0.1224 | 0.1141 – 0.1307 |
| kpi_abs_error_wasl | 0.0298 | 0.0247 – 0.0349 |
| kpi_abs_error_eid_only | 0.2066 | 0.1889 – 0.2243 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9526 | 0.9414 – 0.9638 |
| lung_state_accuracy | 0.9782 | 0.9738 – 0.9826 |
| lung_escalation_recall | 0.9186 | 0.8775 – 0.9597 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9944 | 0.9902 – 0.9986 |
| dr_referral_recall | 0.8988 | 0.819 – 0.9786 |
| dr_state_accuracy | 0.9942 | 0.99 – 0.9984 |
| dr_triage_accuracy | 0.999 | 0.9978 – 1.0002 |
| dr_overdue_accuracy | 0.9954 | 0.9927 – 0.9981 |
| dr_exam_within_deadline_truth | 0.4338 | 0.3443 – 0.5233 |
| dr_exam_within_deadline_wasl | 0.3886 | 0.3037 – 0.4735 |
| breast_precision | 1 | 1.0 – 1.0 |
| breast_recall | 0.997 | 0.9887 – 1.0053 |
| breast_recall_loop_recall | 0.9714 | 0.892 – 1.0508 |
| breast_state_accuracy | 0.9972 | 0.9894 – 1.005 |
| breast_triage_accuracy | 1 | 1.0 – 1.0 |
| breast_overdue_accuracy | 0.9972 | 0.9894 – 1.005 |
| breast_recall_outcome_accuracy | 1 | 1.0 – 1.0 |
| breast_assessment_within_deadline_truth | 0.5516 | 0.2912 – 0.812 |
| breast_assessment_within_deadline_wasl | 0.5294 | 0.2289 – 0.8299 |
| breast_abnormal_recall_rate_truth | 0.0886 | 0.0462 – 0.131 |
| breast_abnormal_recall_rate_wasl | 0.0886 | 0.0462 – 0.131 |
| breast_early_recall_rate_truth | 0.0084 | -0.0011 – 0.0179 |
| breast_early_recall_rate_wasl | 0.0084 | -0.0011 – 0.0179 |
| ldct_precision | 1 | 1.0 – 1.0 |
| ldct_recall | 0.9988 | 0.9968 – 1.0008 |
| ldct_action_loop_recall | 0.987 | 0.9649 – 1.0091 |
| ldct_state_accuracy | 0.9988 | 0.9968 – 1.0008 |
| ldct_triage_accuracy | 1 | 1.0 – 1.0 |
| ldct_overdue_accuracy | 0.9988 | 0.9968 – 1.0008 |
| ldct_action_outcome_accuracy | 1 | 1.0 – 1.0 |
| ldct_next_step_within_clock_truth | 0.5552 | 0.4388 – 0.6716 |
| ldct_next_step_within_clock_wasl | 0.5444 | 0.4395 – 0.6493 |
| ldct_abnormal_recall_rate_truth | 0.1354 | 0.1305 – 0.1403 |
| ldct_abnormal_recall_rate_wasl | 0.1354 | 0.1305 – 0.1403 |
| ldct_cancer_detection_rate_truth | 0.014 | 0.0103 – 0.0177 |
| ldct_cancer_detection_rate_wasl | 0.0132 | 0.0101 – 0.0163 |
| linkage_pair_f1 | 0.9394 | 0.9358 – 0.943 |
| reviewer_h_per_1k | 11.5052 | 10.788 – 12.2224 |
| review_items_per_100_loops | 33.1442 | 31.7611 – 34.5273 |
| throughput_ev_s | 531.5939 | 226.8638 – 836.3239 |
| review_items_per_100_arab | 32.4742 | 31.3022 – 33.6462 |
| review_items_per_100_filipino | 35.4822 | 29.6807 – 41.2837 |
| review_items_per_100_south_asian | 34.8746 | 30.1935 – 39.5557 |
| review_items_per_100_western | 31.5912 | 30.9155 – 32.2669 |
| reviewer_h_per_1k_arab | 11.2434 | 10.546 – 11.9408 |
| reviewer_h_per_1k_filipino | 12.25 | 9.6136 – 14.8864 |
| reviewer_h_per_1k_south_asian | 12.1704 | 10.4188 – 13.922 |
| reviewer_h_per_1k_western | 11.0214 | 10.3972 – 11.6456 |
| review_load_ratio_max_min_origin | 1.1992 | 1.0194 – 1.3791 |
| ifhas_abnormal_findings_rate_truth | 0.4136 | 0.3988 – 0.4284 |
| ifhas_abnormal_findings_rate_wasl | 0.4138 | 0.3992 – 0.4284 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.9062 | 0.8855 – 0.9269 |
| ifhas_followup_within_clock_truth | 0.5468 | 0.517 – 0.5766 |
| ifhas_followup_within_clock_wasl | 0.4978 | 0.4619 – 0.5337 |
| ifhas_overdue_accuracy | 0.976 | 0.9707 – 0.9813 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.9688 | 0.9626 – 0.975 |
| ifhas_state_accuracy | 0.973 | 0.9672 – 0.9788 |
| ifhas_triage_accuracy | 0.9998 | 0.9992 – 1.0004 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3040 | 0.1210% |
| cervical_false_closures | 0 | 1303 | 0.2830% |
| wrong_patient_released | 0 | 16472 | 0.0220% |
| released_without_granted_consent | 0 | 11565 | 0.0320% |
| duplicate_or_lost_outputs | 0 | 216504 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 274715 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 115 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1908 | 0.1930% |
| lung_surveillance_ct_false_closures | 0 | 32 | 10.8880% |
| lung_surveillance_ct_overdue_flag_correct | 88 | 91 | – |
| dr_false_closures | 0 | 5628 | 0.0660% |
| dr_referral_false_closures | 0 | 234 | 1.5640% |
| breast_false_closures | 0 | 339 | 1.0820% |
| breast_recall_false_closures | 0 | 24 | 14.2470% |
| ldct_false_closures | 0 | 1414 | 0.2610% |
| ldct_action_false_closures | 0 | 122 | 2.9780% |
| ifhas_false_closures | 0 | 3392 | 0.1090% |
| ifhas_followup_false_closures | 0 | 1047 | 0.3520% |
| handoff_false_closures | 0 | 86 | 4.1990% |
| handoff_lung_false_closures | 0 | 4 | 60.2360% |
| handoff_dr_false_closures | 0 | 17 | 19.5060% |

Review load by name origin: pooled over seeds {'arab': 32.5, 'filipino': 35.5, 'south_asian': 34.9, 'western': 31.6} items per 100 loops, max/min **1.1246**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1139 and 95th percentile 1.2367 (mean over seeds); 2 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0503 and 95th percentile 1.0979; the observed pooled 1.1246 has p = 0.012.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.012, 'filipino': 1.008, 'south_asian': 0.988, 'western': 0.973}, max/min **1.0397** (no-bias 95th percentile 1.0903, p = 0.566; labels shuffled within seed and programme).
- breast: {'arab': 2.8, 'filipino': 0.0, 'south_asian': 3.9, 'western': 8.5} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.5).
- cervical: {'arab': 52.2, 'filipino': 53.0, 'south_asian': 51.2, 'western': 49.5} items per 100 loops, max/min **1.0718** (no-bias 95th percentile 1.1908, p = 0.68).
- crc: {'arab': 61.9, 'filipino': 58.9, 'south_asian': 60.5, 'western': 61.0} items per 100 loops, max/min **1.0509** (no-bias 95th percentile 1.1159, p = 0.562).
- diabetic_retinopathy: {'arab': 13.1, 'filipino': 17.0, 'south_asian': 13.2, 'western': 14.0} items per 100 loops, max/min **1.2935** (no-bias 95th percentile 1.412, p = 0.196).
- ifhas: {'arab': 18.9, 'filipino': 18.2, 'south_asian': 17.4, 'western': 16.6} items per 100 loops, max/min **1.1375** (no-bias 95th percentile 1.5686, p = 0.808).
- lung_ldct: {'arab': 2.8, 'filipino': 1.9, 'south_asian': 2.8, 'western': 1.1} items per 100 loops, max/min **2.5185** (no-bias 95th percentile inf, p = 0.619).
- lung_nodule: {'arab': 10.3, 'filipino': 12.0, 'south_asian': 9.9, 'western': 7.6} items per 100 loops, max/min **1.5882** (no-bias 95th percentile 1.5616, p = 0.039).
CRC loops by age band: {'40-59': 59.0, '60-79': 62.5, '80+': 62.6, '<40': 61.1} review items per 100 loops, max/min **1.0611** (no-bias 95th percentile 1.0882, p = 0.253); recall {'40-59': 0.786, '60-79': 0.798, '80+': 0.799, '<40': 0.767}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 61.5, 'M': 60.4} review items per 100 loops, max/min **1.0179** (no-bias 95th percentile 1.0473, p = 0.487); recall {'F': 0.786, 'M': 0.786}; false closures {'F': 0, 'M': 0}.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off, DR, breast and LDCT readers, PET-low surveillance CT) | Count |
|---|---|
| breast_reader_breast-assess_agree | 75 |
| breast_reader_breast-assess_agree_but_wrong | 0 |
| breast_reader_breast-assess_coded | 0 |
| breast_reader_breast-assess_instances | 75 |
| breast_reader_breast-screen_agree | 351 |
| breast_reader_breast-screen_agree_but_wrong | 0 |
| breast_reader_breast-screen_coded | 235 |
| breast_reader_breast-screen_instances | 356 |
| dr_reader_dr-clinic_agree | 1147 |
| dr_reader_dr-clinic_agree_but_wrong | 0 |
| dr_reader_dr-clinic_coded | 0 |
| dr_reader_dr-clinic_instances | 1173 |
| dr_reader_dr-screen_agree | 5523 |
| dr_reader_dr-screen_agree_but_wrong | 0 |
| dr_reader_dr-screen_coded | 3746 |
| dr_reader_dr-screen_instances | 6207 |
| handoff_acted_state_correct | 300 |
| handoff_closed | 86 |
| handoff_dr_acted_state_correct | 35 |
| handoff_dr_closed | 17 |
| handoff_dr_packets_all_required_fields | 35 |
| handoff_dr_packets_matched | 35 |
| handoff_dr_truth_acted_by_act_by | 14 |
| handoff_dr_truth_closed_after_handoff | 17 |
| handoff_dr_truth_packets | 35 |
| handoff_dr_truth_referrer_acted | 24 |
| handoff_dr_truth_urgent | 14 |
| handoff_lung_acted_state_correct | 21 |
| handoff_lung_closed | 4 |
| handoff_lung_packets_all_required_fields | 21 |
| handoff_lung_packets_matched | 21 |
| handoff_lung_truth_acted_by_act_by | 7 |
| handoff_lung_truth_closed_after_handoff | 4 |
| handoff_lung_truth_packets | 21 |
| handoff_lung_truth_referrer_acted | 16 |
| handoff_lung_truth_urgent | 21 |
| handoff_packets_all_required_fields | 300 |
| handoff_packets_matched | 300 |
| handoff_truth_acted_by_act_by | 142 |
| handoff_truth_closed_after_handoff | 102 |
| handoff_truth_packets | 300 |
| handoff_truth_referrer_acted | 228 |
| handoff_truth_urgent | 232 |
| ifhas_reader_ifhas-followup_agree | 1762 |
| ifhas_reader_ifhas-followup_agree_but_wrong | 0 |
| ifhas_reader_ifhas-followup_instances | 2200 |
| ifhas_reader_ifhas-screen_agree | 3897 |
| ifhas_reader_ifhas-screen_agree_but_wrong | 0 |
| ifhas_reader_ifhas-screen_instances | 4000 |
| ldct_reader_ldct-screen_agree | 1586 |
| ldct_reader_ldct-screen_agree_but_wrong | 0 |
| ldct_reader_ldct-screen_coded | 958 |
| ldct_reader_ldct-screen_instances | 1599 |
| ldct_reader_ldct-workup_agree | 235 |
| ldct_reader_ldct-workup_agree_but_wrong | 0 |
| ldct_reader_ldct-workup_coded | 0 |
| ldct_reader_ldct-workup_instances | 265 |
| lung_surv_extra_opened | 0 |
| lung_surv_false_closures | 0 |
| lung_surv_missed | 1 |
| lung_surv_obligations | 91 |
| lung_surv_opened | 85 |
| lung_surv_outreach_lung_surveillance_due:released | 4 |
| lung_surv_outreach_lung_surveillance_recall:consent_unknown_hold | 2 |
| lung_surv_outreach_lung_surveillance_recall:consent_withheld | 1 |
| lung_surv_outreach_lung_surveillance_recall:released | 14 |
| lung_surv_overdue_flag_correct | 88 |
| lung_surv_pred_closed | 32 |
| lung_surv_pred_overdue | 17 |
| lung_surv_tp | 32 |
| lung_surv_truth_closed | 33 |
| lung_surv_truth_overdue | 20 |
| lung_workup_pred_awaiting_tissue | 9 |
| lung_workup_pred_closed_by_ct | 91 |
| lung_workup_pred_closed_by_pet | 85 |
| lung_workup_pred_closed_by_tissue | 49 |
| lung_workup_truth_awaiting_tissue | 9 |
| lung_workup_truth_path_ct | 142 |
| lung_workup_truth_path_pet | 166 |
| lung_workup_truth_path_tissue | 54 |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
