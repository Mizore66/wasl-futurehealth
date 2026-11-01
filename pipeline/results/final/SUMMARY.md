# Final frozen evaluation

Code revision `549656327365` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-31T22:49:08.796461Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8426 | 0.8266 – 0.8586 |
| crc_recall_no_review | 0.325 | 0.2991 – 0.3509 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9674 | 0.9482 – 0.9866 |
| kpi_truth | 0.323 | 0.3097 – 0.3363 |
| kpi_wasl | 0.3128 | 0.299 – 0.3266 |
| kpi_eid_only | 0.1284 | 0.1197 – 0.1371 |
| kpi_abs_error_wasl | 0.0102 | 0.0074 – 0.013 |
| kpi_abs_error_eid_only | 0.1946 | 0.1849 – 0.2043 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9862 | 0.9803 – 0.9921 |
| lung_state_accuracy | 0.994 | 0.9922 – 0.9958 |
| lung_escalation_recall | 0.9916 | 0.9771 – 1.0061 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9976 | 0.9959 – 0.9993 |
| dr_referral_recall | 0.978 | 0.9583 – 0.9977 |
| dr_state_accuracy | 0.9978 | 0.9968 – 0.9988 |
| dr_triage_accuracy | 0.9988 | 0.9978 – 0.9998 |
| dr_overdue_accuracy | 0.9984 | 0.997 – 0.9998 |
| dr_exam_within_deadline_truth | 0.3606 | 0.3062 – 0.415 |
| dr_exam_within_deadline_wasl | 0.352 | 0.2937 – 0.4103 |
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
| linkage_pair_f1 | 0.9748 | 0.9734 – 0.9762 |
| reviewer_h_per_1k | 14.2662 | 13.9694 – 14.563 |
| review_items_per_100_loops | 41.3846 | 41.127 – 41.6422 |
| throughput_ev_s | 534.9673 | 519.7606 – 550.1741 |
| review_items_per_100_arab | 41.3162 | 40.0629 – 42.5695 |
| review_items_per_100_filipino | 40.1284 | 37.4589 – 42.7979 |
| review_items_per_100_south_asian | 42.325 | 39.0469 – 45.6031 |
| review_items_per_100_western | 40.8634 | 39.026 – 42.7008 |
| reviewer_h_per_1k_arab | 14.2048 | 13.8301 – 14.5795 |
| reviewer_h_per_1k_filipino | 14.0336 | 12.8043 – 15.2629 |
| reviewer_h_per_1k_south_asian | 14.4956 | 13.0779 – 15.9133 |
| reviewer_h_per_1k_western | 14.2498 | 13.1052 – 15.3944 |
| review_load_ratio_max_min_origin | 1.1133 | 1.0444 – 1.1822 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3156 | 0.1170% |
| cervical_false_closures | 0 | 1404 | 0.2620% |
| wrong_patient_released | 0 | 15247 | 0.0240% |
| released_without_granted_consent | 0 | 9945 | 0.0370% |
| duplicate_or_lost_outputs | 0 | 202696 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 254685 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 117 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 2032 | 0.1810% |
| lung_surveillance_ct_false_closures | 0 | 41 | 8.6040% |
| lung_surveillance_ct_overdue_flag_correct | 98 | 99 | – |
| dr_false_closures | 0 | 5636 | 0.0650% |
| dr_referral_false_closures | 0 | 269 | 1.3620% |
| breast_false_closures | 0 | 345 | 1.0640% |
| breast_recall_false_closures | 0 | 31 | 11.2190% |
| handoff_false_closures | 0 | 88 | 4.1050% |
| handoff_lung_false_closures | 0 | 6 | 45.9260% |
| handoff_dr_false_closures | 0 | 15 | 21.8020% |

Review load by name origin: pooled over seeds {'arab': 41.3, 'filipino': 40.1, 'south_asian': 42.3, 'western': 40.9} items per 100 loops, max/min **1.0556**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1075 and 95th percentile 1.2204 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0466 and 95th percentile 1.0942; the observed pooled 1.0556 has p = 0.357.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.026, 'filipino': 0.934, 'south_asian': 0.969, 'western': 0.999}, max/min **1.0993** (no-bias 95th percentile 1.0823, p = 0.015; labels shuffled within seed and programme).
- breast: {'arab': 6.3, 'filipino': 6.2, 'south_asian': 2.4, 'western': 0.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.338).
- cervical: {'arab': 61.4, 'filipino': 57.0, 'south_asian': 57.1, 'western': 58.7} items per 100 loops, max/min **1.0772** (no-bias 95th percentile 1.1895, p = 0.61).
- crc: {'arab': 68.9, 'filipino': 61.6, 'south_asian': 65.1, 'western': 66.9} items per 100 loops, max/min **1.1186** (no-bias 95th percentile 1.1031, p = 0.015).
- diabetic_retinopathy: {'arab': 14.3, 'filipino': 14.2, 'south_asian': 13.0, 'western': 16.1} items per 100 loops, max/min **1.2347** (no-bias 95th percentile 1.3978, p = 0.309).
- lung_nodule: {'arab': 12.1, 'filipino': 10.9, 'south_asian': 12.8, 'western': 11.0} items per 100 loops, max/min **1.168** (no-bias 95th percentile 1.4726, p = 0.675).
CRC loops by age band: {'40-59': 64.8, '60-79': 68.2, '80+': 68.6, '<40': 67.1} review items per 100 loops, max/min **1.0579** (no-bias 95th percentile 1.0828, p = 0.234); recall {'40-59': 0.819, '60-79': 0.851, '80+': 0.842, '<40': 0.86}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 66.2, 'M': 69.0} review items per 100 loops, max/min **1.0425** (no-bias 95th percentile 1.046, p = 0.076); recall {'F': 0.844, 'M': 0.839}; false closures {'F': 0, 'M': 0}.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off, DR and breast readers, PET-low surveillance CT) | Count |
|---|---|
| breast_reader_breast-assess_agree | 83 |
| breast_reader_breast-assess_agree_but_wrong | 0 |
| breast_reader_breast-assess_coded | 0 |
| breast_reader_breast-assess_instances | 83 |
| breast_reader_breast-screen_agree | 350 |
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
| handoff_acted_state_correct | 331 |
| handoff_closed | 88 |
| handoff_dr_acted_state_correct | 32 |
| handoff_dr_closed | 15 |
| handoff_dr_packets_all_required_fields | 32 |
| handoff_dr_packets_matched | 32 |
| handoff_dr_packets_missed | 1 |
| handoff_dr_truth_acted_by_act_by | 16 |
| handoff_dr_truth_closed_after_handoff | 15 |
| handoff_dr_truth_packets | 33 |
| handoff_dr_truth_referrer_acted | 28 |
| handoff_dr_truth_urgent | 15 |
| handoff_lung_acted_state_correct | 19 |
| handoff_lung_closed | 6 |
| handoff_lung_packets_all_required_fields | 19 |
| handoff_lung_packets_matched | 19 |
| handoff_lung_packets_missed | 1 |
| handoff_lung_truth_acted_by_act_by | 7 |
| handoff_lung_truth_closed_after_handoff | 6 |
| handoff_lung_truth_packets | 20 |
| handoff_lung_truth_referrer_acted | 10 |
| handoff_lung_truth_urgent | 20 |
| handoff_packets_all_required_fields | 331 |
| handoff_packets_matched | 331 |
| handoff_packets_missed | 2 |
| handoff_truth_acted_by_act_by | 152 |
| handoff_truth_closed_after_handoff | 96 |
| handoff_truth_packets | 333 |
| handoff_truth_referrer_acted | 256 |
| handoff_truth_urgent | 248 |
| lung_surv_extra_opened | 3 |
| lung_surv_false_closures | 0 |
| lung_surv_missed | 0 |
| lung_surv_obligations | 99 |
| lung_surv_opened | 96 |
| lung_surv_outreach_lung_surveillance_due:released | 2 |
| lung_surv_outreach_lung_surveillance_recall:consent_unknown_hold | 3 |
| lung_surv_outreach_lung_surveillance_recall:consent_withheld | 1 |
| lung_surv_outreach_lung_surveillance_recall:released | 15 |
| lung_surv_overdue_flag_correct | 98 |
| lung_surv_pred_closed | 41 |
| lung_surv_pred_overdue | 19 |
| lung_surv_tp | 41 |
| lung_surv_truth_closed | 41 |
| lung_surv_truth_overdue | 20 |
| lung_workup_pred_awaiting_tissue | 11 |
| lung_workup_pred_closed_by_ct | 94 |
| lung_workup_pred_closed_by_pet | 99 |
| lung_workup_pred_closed_by_tissue | 51 |
| lung_workup_truth_awaiting_tissue | 11 |
| lung_workup_truth_path_ct | 127 |
| lung_workup_truth_path_pet | 171 |
| lung_workup_truth_path_tissue | 55 |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7822 | 0.769 – 0.7954 |
| crc_recall_no_review | 0.3196 | 0.2957 – 0.3435 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9126 | 0.8899 – 0.9353 |
| kpi_truth | 0.329 | 0.3066 – 0.3514 |
| kpi_wasl | 0.2984 | 0.2727 – 0.3241 |
| kpi_eid_only | 0.1224 | 0.1141 – 0.1307 |
| kpi_abs_error_wasl | 0.0306 | 0.0242 – 0.037 |
| kpi_abs_error_eid_only | 0.2066 | 0.1889 – 0.2243 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9502 | 0.9406 – 0.9598 |
| lung_state_accuracy | 0.9774 | 0.9732 – 0.9816 |
| lung_escalation_recall | 0.9212 | 0.8917 – 0.9507 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9938 | 0.9894 – 0.9982 |
| dr_referral_recall | 0.8866 | 0.7982 – 0.975 |
| dr_state_accuracy | 0.9938 | 0.9892 – 0.9984 |
| dr_triage_accuracy | 0.999 | 0.9978 – 1.0002 |
| dr_overdue_accuracy | 0.995 | 0.9914 – 0.9986 |
| dr_exam_within_deadline_truth | 0.4338 | 0.3443 – 0.5233 |
| dr_exam_within_deadline_wasl | 0.3836 | 0.2916 – 0.4756 |
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
| linkage_pair_f1 | 0.94 | 0.9366 – 0.9434 |
| reviewer_h_per_1k | 12.915 | 12.1153 – 13.7147 |
| review_items_per_100_loops | 37.2612 | 35.6872 – 38.8352 |
| throughput_ev_s | 607.4037 | 463.8568 – 750.9507 |
| review_items_per_100_arab | 36.5678 | 35.3001 – 37.8355 |
| review_items_per_100_filipino | 39.0882 | 33.2307 – 44.9457 |
| review_items_per_100_south_asian | 39.1396 | 33.6531 – 44.6261 |
| review_items_per_100_western | 35.6964 | 34.8222 – 36.5706 |
| reviewer_h_per_1k_arab | 12.6868 | 12.0973 – 13.2763 |
| reviewer_h_per_1k_filipino | 13.5896 | 11.0601 – 16.1191 |
| reviewer_h_per_1k_south_asian | 13.4646 | 11.555 – 15.3742 |
| reviewer_h_per_1k_western | 12.4712 | 11.8611 – 13.0813 |
| review_load_ratio_max_min_origin | 1.1796 | 1.0165 – 1.3427 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3025 | 0.1220% |
| cervical_false_closures | 0 | 1313 | 0.2810% |
| wrong_patient_released | 0 | 14451 | 0.0260% |
| released_without_granted_consent | 0 | 9602 | 0.0380% |
| duplicate_or_lost_outputs | 0 | 198810 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 255578 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 117 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1903 | 0.1940% |
| lung_surveillance_ct_false_closures | 0 | 30 | 11.5700% |
| lung_surveillance_ct_overdue_flag_correct | 85 | 87 | – |
| dr_false_closures | 0 | 5625 | 0.0660% |
| dr_referral_false_closures | 0 | 231 | 1.5840% |
| breast_false_closures | 0 | 338 | 1.0850% |
| breast_recall_false_closures | 0 | 23 | 14.8190% |
| handoff_false_closures | 0 | 86 | 4.1990% |
| handoff_lung_false_closures | 0 | 4 | 60.2360% |
| handoff_dr_false_closures | 0 | 17 | 19.5060% |

Review load by name origin: pooled over seeds {'arab': 36.6, 'filipino': 39.1, 'south_asian': 39.2, 'western': 35.7} items per 100 loops, max/min **1.098**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1165 and 95th percentile 1.2421 (mean over seeds); 2 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0492 and 95th percentile 1.1007; the observed pooled 1.098 has p = 0.057.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.01, 'filipino': 1.004, 'south_asian': 0.99, 'western': 0.98}, max/min **1.0305** (no-bias 95th percentile 1.0903, p = 0.773; labels shuffled within seed and programme).
- breast: {'arab': 3.9, 'filipino': 2.8, 'south_asian': 3.9, 'western': 11.9} items per 100 loops, max/min **4.2712** (no-bias 95th percentile inf, p = 0.404).
- cervical: {'arab': 52.6, 'filipino': 53.3, 'south_asian': 51.1, 'western': 49.3} items per 100 loops, max/min **1.0809** (no-bias 95th percentile 1.1979, p = 0.624).
- crc: {'arab': 61.9, 'filipino': 58.5, 'south_asian': 60.8, 'western': 61.0} items per 100 loops, max/min **1.058** (no-bias 95th percentile 1.1092, p = 0.432).
- diabetic_retinopathy: {'arab': 13.1, 'filipino': 16.7, 'south_asian': 13.2, 'western': 14.0} items per 100 loops, max/min **1.2739** (no-bias 95th percentile 1.4001, p = 0.211).
- lung_nodule: {'arab': 10.1, 'filipino': 11.8, 'south_asian': 9.8, 'western': 7.6} items per 100 loops, max/min **1.5576** (no-bias 95th percentile 1.5616, p = 0.053).
CRC loops by age band: {'40-59': 59.0, '60-79': 62.6, '80+': 62.5, '<40': 61.1} review items per 100 loops, max/min **1.0603** (no-bias 95th percentile 1.0882, p = 0.267); recall {'40-59': 0.779, '60-79': 0.787, '80+': 0.8, '<40': 0.77}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 61.5, 'M': 60.4} review items per 100 loops, max/min **1.0172** (no-bias 95th percentile 1.0481, p = 0.515); recall {'F': 0.781, 'M': 0.786}; false closures {'F': 0, 'M': 0}.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off, DR and breast readers, PET-low surveillance CT) | Count |
|---|---|
| breast_reader_breast-assess_agree | 75 |
| breast_reader_breast-assess_agree_but_wrong | 0 |
| breast_reader_breast-assess_coded | 0 |
| breast_reader_breast-assess_instances | 75 |
| breast_reader_breast-screen_agree | 346 |
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
| handoff_acted_state_correct | 299 |
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
| handoff_packets_all_required_fields | 299 |
| handoff_packets_matched | 299 |
| handoff_packets_missed | 1 |
| handoff_truth_acted_by_act_by | 142 |
| handoff_truth_closed_after_handoff | 102 |
| handoff_truth_packets | 300 |
| handoff_truth_referrer_acted | 228 |
| handoff_truth_urgent | 232 |
| lung_surv_extra_opened | 4 |
| lung_surv_false_closures | 0 |
| lung_surv_missed | 2 |
| lung_surv_obligations | 87 |
| lung_surv_opened | 81 |
| lung_surv_outreach_lung_surveillance_due:released | 3 |
| lung_surv_outreach_lung_surveillance_recall:consent_unknown_hold | 2 |
| lung_surv_outreach_lung_surveillance_recall:consent_withheld | 1 |
| lung_surv_outreach_lung_surveillance_recall:released | 15 |
| lung_surv_overdue_flag_correct | 85 |
| lung_surv_pred_closed | 30 |
| lung_surv_pred_overdue | 18 |
| lung_surv_tp | 30 |
| lung_surv_truth_closed | 32 |
| lung_surv_truth_overdue | 20 |
| lung_workup_pred_awaiting_tissue | 9 |
| lung_workup_pred_closed_by_ct | 90 |
| lung_workup_pred_closed_by_pet | 85 |
| lung_workup_pred_closed_by_tissue | 51 |
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
