# Final frozen evaluation

Code revision `075abc5ad04a` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-11-02T04:33:36.064230Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.844 | 0.8254 – 0.8626 |
| crc_recall_no_review | 0.3188 | 0.2949 – 0.3427 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9692 | 0.955 – 0.9834 |
| kpi_truth | 0.323 | 0.3097 – 0.3363 |
| kpi_wasl | 0.3134 | 0.2989 – 0.3279 |
| kpi_eid_only | 0.1284 | 0.1197 – 0.1371 |
| kpi_abs_error_wasl | 0.0096 | 0.0064 – 0.0128 |
| kpi_abs_error_eid_only | 0.1946 | 0.1849 – 0.2043 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9816 | 0.972 – 0.9912 |
| lung_state_accuracy | 0.992 | 0.9878 – 0.9962 |
| lung_escalation_recall | 0.9792 | 0.963 – 0.9954 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9974 | 0.996 – 0.9988 |
| dr_referral_recall | 0.9696 | 0.9456 – 0.9936 |
| dr_state_accuracy | 0.9978 | 0.9964 – 0.9992 |
| dr_triage_accuracy | 0.9988 | 0.9978 – 0.9998 |
| dr_overdue_accuracy | 0.9982 | 0.9966 – 0.9998 |
| dr_exam_within_deadline_truth | 0.3606 | 0.3062 – 0.415 |
| dr_exam_within_deadline_wasl | 0.3438 | 0.2888 – 0.3988 |
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
| linkage_pair_f1 | 0.9794 | 0.978 – 0.9808 |
| reviewer_h_per_1k | 12.82 | 12.4863 – 13.1537 |
| review_items_per_100_loops | 36.8898 | 36.1299 – 37.6497 |
| throughput_ev_s | 547.7131 | 486.946 – 608.4803 |
| review_items_per_100_arab | 36.6584 | 35.5969 – 37.7199 |
| review_items_per_100_filipino | 36.175 | 34.0747 – 38.2753 |
| review_items_per_100_south_asian | 37.9006 | 34.7492 – 41.052 |
| review_items_per_100_western | 36.5634 | 35.0443 – 38.0825 |
| reviewer_h_per_1k_arab | 12.7384 | 12.3959 – 13.0809 |
| reviewer_h_per_1k_filipino | 12.6108 | 11.5336 – 13.688 |
| reviewer_h_per_1k_south_asian | 13.2798 | 11.8553 – 14.7043 |
| reviewer_h_per_1k_western | 12.524 | 11.8498 – 13.1982 |
| review_load_ratio_max_min_origin | 1.1025 | 1.026 – 1.1791 |
| ifhas_abnormal_findings_rate_truth | 0.4148 | 0.396 – 0.4336 |
| ifhas_abnormal_findings_rate_wasl | 0.4152 | 0.3955 – 0.4349 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.9674 | 0.9563 – 0.9785 |
| ifhas_followup_within_clock_truth | 0.5432 | 0.5286 – 0.5578 |
| ifhas_followup_within_clock_wasl | 0.524 | 0.5097 – 0.5383 |
| ifhas_overdue_accuracy | 0.9908 | 0.9869 – 0.9947 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.9884 | 0.9837 – 0.9931 |
| ifhas_state_accuracy | 0.99 | 0.9859 – 0.9941 |
| ifhas_triage_accuracy | 0.9998 | 0.9992 – 1.0004 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3161 | 0.1170% |
| cervical_false_closures | 0 | 1407 | 0.2620% |
| wrong_patient_released | 0 | 17063 | 0.0220% |
| released_without_granted_consent | 0 | 11828 | 0.0310% |
| duplicate_or_lost_outputs | 0 | 220789 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 273843 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 110 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 2022 | 0.1820% |
| lung_surveillance_ct_false_closures | 0 | 41 | 8.6040% |
| lung_surveillance_ct_overdue_flag_correct | 102 | 102 | – |
| dr_false_closures | 0 | 5634 | 0.0650% |
| dr_referral_false_closures | 0 | 267 | 1.3720% |
| breast_false_closures | 0 | 345 | 1.0640% |
| breast_recall_false_closures | 0 | 31 | 11.2190% |
| ldct_false_closures | 0 | 1398 | 0.2640% |
| ldct_action_false_closures | 0 | 109 | 3.3280% |
| ifhas_false_closures | 0 | 3480 | 0.1060% |
| ifhas_followup_false_closures | 0 | 1140 | 0.3230% |
| handoff_false_closures | 0 | 88 | 4.1050% |
| handoff_lung_false_closures | 0 | 5 | 52.1820% |
| handoff_dr_false_closures | 0 | 14 | 23.1640% |

Review load by name origin: pooled over seeds {'arab': 36.7, 'filipino': 36.2, 'south_asian': 37.9, 'western': 36.6} items per 100 loops, max/min **1.0481**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.107 and 95th percentile 1.2223 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0452 and 95th percentile 1.0907; the observed pooled 1.0481 has p = 0.451.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.026, 'filipino': 0.933, 'south_asian': 0.97, 'western': 0.999}, max/min **1.0993** (no-bias 95th percentile 1.0788, p = 0.01; labels shuffled within seed and programme).
- breast: {'arab': 4.2, 'filipino': 3.1, 'south_asian': 1.2, 'western': 0.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.658).
- cervical: {'arab': 62.3, 'filipino': 56.5, 'south_asian': 57.6, 'western': 58.7} items per 100 loops, max/min **1.1028** (no-bias 95th percentile 1.1767, p = 0.394).
- crc: {'arab': 69.1, 'filipino': 61.6, 'south_asian': 64.8, 'western': 66.1} items per 100 loops, max/min **1.1217** (no-bias 95th percentile 1.1024, p = 0.01).
- diabetic_retinopathy: {'arab': 14.4, 'filipino': 14.2, 'south_asian': 13.0, 'western': 16.2} items per 100 loops, max/min **1.2431** (no-bias 95th percentile 1.3904, p = 0.261).
- ifhas: {'arab': 19.9, 'filipino': 20.1, 'south_asian': 20.2, 'western': 24.0} items per 100 loops, max/min **1.2036** (no-bias 95th percentile 1.5215, p = 0.547).
- lung_ldct: {'arab': 2.1, 'filipino': 1.7, 'south_asian': 3.4, 'western': 1.2} items per 100 loops, max/min **2.9456** (no-bias 95th percentile inf, p = 0.426).
- lung_nodule: {'arab': 12.2, 'filipino': 11.2, 'south_asian': 12.8, 'western': 10.6} items per 100 loops, max/min **1.201** (no-bias 95th percentile 1.4427, p = 0.559).
CRC loops by age band: {'40-59': 65.1, '60-79': 67.6, '80+': 68.6, '<40': 67.0} review items per 100 loops, max/min **1.0541** (no-bias 95th percentile 1.0825, p = 0.27); recall {'40-59': 0.815, '60-79': 0.86, '80+': 0.833, '<40': 0.867}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 66.2, 'M': 68.7} review items per 100 loops, max/min **1.0374** (no-bias 95th percentile 1.0448, p = 0.114); recall {'F': 0.848, 'M': 0.834}; false closures {'F': 0, 'M': 0}.

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
| crc_recall | 0.785 | 0.7721 – 0.7979 |
| crc_recall_no_review | 0.3128 | 0.2884 – 0.3372 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9062 | 0.876 – 0.9364 |
| kpi_truth | 0.329 | 0.3066 – 0.3514 |
| kpi_wasl | 0.298 | 0.272 – 0.324 |
| kpi_eid_only | 0.1224 | 0.1141 – 0.1307 |
| kpi_abs_error_wasl | 0.031 | 0.0254 – 0.0366 |
| kpi_abs_error_eid_only | 0.2066 | 0.1889 – 0.2243 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.952 | 0.9405 – 0.9635 |
| lung_state_accuracy | 0.978 | 0.9734 – 0.9826 |
| lung_escalation_recall | 0.9186 | 0.8775 – 0.9597 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9942 | 0.9903 – 0.9981 |
| dr_referral_recall | 0.8944 | 0.8201 – 0.9687 |
| dr_state_accuracy | 0.9942 | 0.99 – 0.9984 |
| dr_triage_accuracy | 0.999 | 0.9978 – 1.0002 |
| dr_overdue_accuracy | 0.9954 | 0.9927 – 0.9981 |
| dr_exam_within_deadline_truth | 0.4338 | 0.3443 – 0.5233 |
| dr_exam_within_deadline_wasl | 0.3858 | 0.2979 – 0.4737 |
| breast_precision | 1 | 1.0 – 1.0 |
| breast_recall | 0.9942 | 0.9843 – 1.0041 |
| breast_recall_loop_recall | 0.9314 | 0.8121 – 1.0507 |
| breast_state_accuracy | 0.9944 | 0.9849 – 1.0039 |
| breast_triage_accuracy | 1 | 1.0 – 1.0 |
| breast_overdue_accuracy | 0.9944 | 0.9849 – 1.0039 |
| breast_recall_outcome_accuracy | 1 | 1.0 – 1.0 |
| breast_assessment_within_deadline_truth | 0.5516 | 0.2912 – 0.812 |
| breast_assessment_within_deadline_wasl | 0.4894 | 0.2426 – 0.7362 |
| breast_abnormal_recall_rate_truth | 0.0886 | 0.0462 – 0.131 |
| breast_abnormal_recall_rate_wasl | 0.0886 | 0.0462 – 0.131 |
| breast_early_recall_rate_truth | 0.0084 | -0.0011 – 0.0179 |
| breast_early_recall_rate_wasl | 0.0084 | -0.0011 – 0.0179 |
| ldct_precision | 1 | 1.0 – 1.0 |
| ldct_recall | 0.998 | 0.9942 – 1.0018 |
| ldct_action_loop_recall | 0.9808 | 0.9459 – 1.0157 |
| ldct_state_accuracy | 0.9982 | 0.9949 – 1.0015 |
| ldct_triage_accuracy | 1 | 1.0 – 1.0 |
| ldct_overdue_accuracy | 0.9982 | 0.9949 – 1.0015 |
| ldct_action_outcome_accuracy | 1 | 1.0 – 1.0 |
| ldct_next_step_within_clock_truth | 0.5552 | 0.4388 – 0.6716 |
| ldct_next_step_within_clock_wasl | 0.5388 | 0.4429 – 0.6347 |
| ldct_abnormal_recall_rate_truth | 0.1354 | 0.1305 – 0.1403 |
| ldct_abnormal_recall_rate_wasl | 0.1354 | 0.1305 – 0.1403 |
| ldct_cancer_detection_rate_truth | 0.014 | 0.0103 – 0.0177 |
| ldct_cancer_detection_rate_wasl | 0.0132 | 0.0101 – 0.0163 |
| linkage_pair_f1 | 0.9422 | 0.9381 – 0.9463 |
| reviewer_h_per_1k | 11.6316 | 10.9639 – 12.2993 |
| review_items_per_100_loops | 33.5584 | 32.3128 – 34.804 |
| throughput_ev_s | 554.5136 | 360.8136 – 748.2136 |
| review_items_per_100_arab | 33.187 | 31.7281 – 34.6459 |
| review_items_per_100_filipino | 35.3714 | 29.6772 – 41.0656 |
| review_items_per_100_south_asian | 35.0842 | 30.5953 – 39.5731 |
| review_items_per_100_western | 31.5014 | 30.4276 – 32.5752 |
| reviewer_h_per_1k_arab | 11.4672 | 10.7573 – 12.1771 |
| reviewer_h_per_1k_filipino | 12.2294 | 9.6109 – 14.8479 |
| reviewer_h_per_1k_south_asian | 12.225 | 10.5036 – 13.9464 |
| reviewer_h_per_1k_western | 10.978 | 10.2919 – 11.6641 |
| review_load_ratio_max_min_origin | 1.2095 | 1.0326 – 1.3865 |
| ifhas_abnormal_findings_rate_truth | 0.4136 | 0.3988 – 0.4284 |
| ifhas_abnormal_findings_rate_wasl | 0.4138 | 0.3992 – 0.4284 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.906 | 0.8885 – 0.9235 |
| ifhas_followup_within_clock_truth | 0.5468 | 0.517 – 0.5766 |
| ifhas_followup_within_clock_wasl | 0.4978 | 0.4623 – 0.5333 |
| ifhas_overdue_accuracy | 0.9762 | 0.9719 – 0.9805 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.9688 | 0.9633 – 0.9743 |
| ifhas_state_accuracy | 0.9728 | 0.9679 – 0.9777 |
| ifhas_triage_accuracy | 0.9998 | 0.9992 – 1.0004 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3035 | 0.1210% |
| cervical_false_closures | 0 | 1303 | 0.2830% |
| wrong_patient_released | 0 | 16486 | 0.0220% |
| released_without_granted_consent | 0 | 11650 | 0.0320% |
| duplicate_or_lost_outputs | 0 | 217009 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 274706 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 106 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1907 | 0.1930% |
| lung_surveillance_ct_false_closures | 0 | 32 | 10.8880% |
| lung_surveillance_ct_overdue_flag_correct | 88 | 91 | – |
| dr_false_closures | 0 | 5627 | 0.0660% |
| dr_referral_false_closures | 0 | 233 | 1.5710% |
| breast_false_closures | 0 | 338 | 1.0850% |
| breast_recall_false_closures | 0 | 23 | 14.8190% |
| ldct_false_closures | 0 | 1413 | 0.2610% |
| ldct_action_false_closures | 0 | 121 | 3.0030% |
| ifhas_false_closures | 0 | 3392 | 0.1090% |
| ifhas_followup_false_closures | 0 | 1047 | 0.3520% |
| handoff_false_closures | 0 | 86 | 4.1990% |
| handoff_lung_false_closures | 0 | 4 | 60.2360% |
| handoff_dr_false_closures | 0 | 17 | 19.5060% |

Review load by name origin: pooled over seeds {'arab': 33.2, 'filipino': 35.4, 'south_asian': 35.1, 'western': 31.5} items per 100 loops, max/min **1.124**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1139 and 95th percentile 1.2395 (mean over seeds); 2 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0497 and 95th percentile 1.0985; the observed pooled 1.124 has p = 0.007.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.021, 'filipino': 0.992, 'south_asian': 0.982, 'western': 0.958}, max/min **1.0653** (no-bias 95th percentile 1.0883, p = 0.185; labels shuffled within seed and programme).
- breast: {'arab': 2.2, 'filipino': 0.0, 'south_asian': 3.9, 'western': 8.5} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.433).
- cervical: {'arab': 53.9, 'filipino': 54.1, 'south_asian': 51.5, 'western': 48.8} items per 100 loops, max/min **1.1086** (no-bias 95th percentile 1.1961, p = 0.351).
- crc: {'arab': 62.8, 'filipino': 58.3, 'south_asian': 60.9, 'western': 61.1} items per 100 loops, max/min **1.0785** (no-bias 95th percentile 1.1157, p = 0.22).
- diabetic_retinopathy: {'arab': 13.2, 'filipino': 17.0, 'south_asian': 13.2, 'western': 14.0} items per 100 loops, max/min **1.2892** (no-bias 95th percentile 1.4246, p = 0.201).
- ifhas: {'arab': 19.5, 'filipino': 18.6, 'south_asian': 17.7, 'western': 16.8} items per 100 loops, max/min **1.1628** (no-bias 95th percentile 1.5581, p = 0.726).
- lung_ldct: {'arab': 3.0, 'filipino': 1.9, 'south_asian': 2.5, 'western': 1.1} items per 100 loops, max/min **2.7268** (no-bias 95th percentile inf, p = 0.541).
- lung_nodule: {'arab': 11.0, 'filipino': 11.6, 'south_asian': 9.9, 'western': 7.4} items per 100 loops, max/min **1.5741** (no-bias 95th percentile 1.5562, p = 0.041).
CRC loops by age band: {'40-59': 59.4, '60-79': 63.3, '80+': 63.1, '<40': 61.4} review items per 100 loops, max/min **1.0657** (no-bias 95th percentile 1.0866, p = 0.199); recall {'40-59': 0.785, '60-79': 0.797, '80+': 0.798, '<40': 0.766}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 62.2, 'M': 60.4} review items per 100 loops, max/min **1.0295** (no-bias 95th percentile 1.0478, p = 0.235); recall {'F': 0.785, 'M': 0.786}; false closures {'F': 0, 'M': 0}.

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
