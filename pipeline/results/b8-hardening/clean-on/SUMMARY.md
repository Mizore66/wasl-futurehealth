# Final frozen evaluation

Code revision `075abc5ad04a` · policy `v7strict` · seeds [17, 29] · source `fhir` · held-out persons only · frozen 2026-11-02T03:34:18.954134Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (2 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.77 | 0.6938 – 0.8462 |
| crc_recall_no_review | 0.2985 | 0.254 – 0.343 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.911 | 0.8094 – 1.0126 |
| kpi_truth | 0.328 | 0.2772 – 0.3788 |
| kpi_wasl | 0.291 | 0.2656 – 0.3164 |
| kpi_eid_only | 0.1295 | 0.1231 – 0.1359 |
| kpi_abs_error_wasl | 0.037 | 0.0116 – 0.0624 |
| kpi_abs_error_eid_only | 0.1985 | 0.1413 – 0.2557 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.944 | 0.8805 – 1.0075 |
| lung_state_accuracy | 0.973 | 0.9603 – 0.9857 |
| lung_escalation_recall | 0.8925 | 0.3525 – 1.4325 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9925 | 0.9607 – 1.0243 |
| dr_referral_recall | 0.8635 | 0.3743 – 1.3527 |
| dr_state_accuracy | 0.9925 | 0.9607 – 1.0243 |
| dr_triage_accuracy | 1 | 1.0 – 1.0 |
| dr_overdue_accuracy | 0.993 | 0.9676 – 1.0184 |
| dr_exam_within_deadline_truth | 0.46 | -0.4167 – 1.3367 |
| dr_exam_within_deadline_wasl | 0.3805 | -0.1722 – 0.9332 |
| breast_precision | 1 | 1.0 – 1.0 |
| breast_recall | 1 | 1.0 – 1.0 |
| breast_recall_loop_recall | 1 | 1.0 – 1.0 |
| breast_state_accuracy | 1 | 1.0 – 1.0 |
| breast_triage_accuracy | 1 | 1.0 – 1.0 |
| breast_overdue_accuracy | 1 | 1.0 – 1.0 |
| breast_recall_outcome_accuracy | 1 | 1.0 – 1.0 |
| breast_assessment_within_deadline_truth | 0.2 | -2.3412 – 2.7412 |
| breast_assessment_within_deadline_wasl | 0.2 | -2.3412 – 2.7412 |
| breast_abnormal_recall_rate_truth | 0.05 | -0.2168 – 0.3168 |
| breast_abnormal_recall_rate_wasl | 0.05 | -0.2168 – 0.3168 |
| breast_early_recall_rate_truth | 0.014 | 0.014 – 0.014 |
| breast_early_recall_rate_wasl | 0.014 | 0.014 – 0.014 |
| ldct_precision | 1 | 1.0 – 1.0 |
| ldct_recall | 0.9965 | 0.952 – 1.041 |
| ldct_action_loop_recall | 0.9695 | 0.582 – 1.357 |
| ldct_state_accuracy | 0.997 | 0.9589 – 1.0351 |
| ldct_triage_accuracy | 1 | 1.0 – 1.0 |
| ldct_overdue_accuracy | 0.997 | 0.9589 – 1.0351 |
| ldct_action_outcome_accuracy | 1 | 1.0 – 1.0 |
| ldct_next_step_within_clock_truth | 0.5955 | -0.1478 – 1.3388 |
| ldct_next_step_within_clock_wasl | 0.571 | -0.4836 – 1.6256 |
| ldct_abnormal_recall_rate_truth | 0.1315 | 0.0235 – 0.2395 |
| ldct_abnormal_recall_rate_wasl | 0.1315 | 0.0235 – 0.2395 |
| ldct_cancer_detection_rate_truth | 0.0135 | -0.0691 – 0.0961 |
| ldct_cancer_detection_rate_wasl | 0.0115 | -0.0965 – 0.1195 |
| linkage_pair_f1 | 0.9335 | 0.9017 – 0.9653 |
| reviewer_h_per_1k | 11.3325 | 10.7417 – 11.9233 |
| review_items_per_100_loops | 32.735 | 28.3514 – 37.1186 |
| throughput_ev_s | 762.8679 | 535.2665 – 990.4693 |
| review_items_per_100_arab | 33.0325 | 31.3235 – 34.7415 |
| review_items_per_100_filipino | 31.698 | 20.6565 – 42.7395 |
| review_items_per_100_south_asian | 34.4435 | 28.4272 – 40.4598 |
| review_items_per_100_western | 29.5985 | 22.3116 – 36.8854 |
| reviewer_h_per_1k_arab | 11.406 | 11.1392 – 11.6728 |
| reviewer_h_per_1k_filipino | 10.514 | 9.4721 – 11.5559 |
| reviewer_h_per_1k_south_asian | 12.12 | 11.3449 – 12.8951 |
| reviewer_h_per_1k_western | 10.3185 | 5.0265 – 15.6105 |
| review_load_ratio_max_min_origin | 1.1638 | 1.0806 – 1.2471 |
| ifhas_abnormal_findings_rate_truth | 0.3915 | 0.3089 – 0.4741 |
| ifhas_abnormal_findings_rate_wasl | 0.392 | 0.3031 – 0.4809 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.89 | 0.4834 – 1.2966 |
| ifhas_followup_within_clock_truth | 0.539 | 0.1197 – 0.9583 |
| ifhas_followup_within_clock_wasl | 0.4845 | -0.0301 – 0.9991 |
| ifhas_overdue_accuracy | 0.9735 | 0.9163 – 1.0307 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.965 | 0.8634 – 1.0666 |
| ifhas_state_accuracy | 0.969 | 0.8928 – 1.0452 |
| ifhas_triage_accuracy | 0.9995 | 0.9931 – 1.0059 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1176 | 0.3130% |
| cervical_false_closures | 0 | 524 | 0.7020% |
| wrong_patient_released | 0 | 6582 | 0.0560% |
| released_without_granted_consent | 0 | 4713 | 0.0780% |
| duplicate_or_lost_outputs | 0 | 85757 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 109482 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 42 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 0 | – | – |
| tamper_detected | 34 | 34 | – |
| tamper_false_alarms | 0 | 2 | – |
| lung_false_closures | 0 | 801 | 0.4590% |
| lung_surveillance_ct_false_closures | 0 | 18 | 18.5300% |
| lung_surveillance_ct_overdue_flag_correct | 39 | 41 | – |
| dr_false_closures | 0 | 2254 | 0.1640% |
| dr_referral_false_closures | 0 | 107 | 3.3890% |
| breast_false_closures | 0 | 139 | 2.6190% |
| breast_recall_false_closures | 0 | 6 | 45.9260% |
| ldct_false_closures | 0 | 566 | 0.6500% |
| ldct_action_false_closures | 0 | 51 | 6.9780% |
| ifhas_false_closures | 0 | 1368 | 0.2690% |
| ifhas_followup_false_closures | 0 | 395 | 0.9300% |
| handoff_false_closures | 0 | 28 | 12.3440% |
| handoff_lung_false_closures | 0 | 4 | 60.2360% |
| handoff_dr_false_closures | 0 | 5 | 52.1820% |

Review load by name origin: pooled over seeds {'arab': 33.0, 'filipino': 31.7, 'south_asian': 34.5, 'western': 29.6} items per 100 loops, max/min **1.1641**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1128 and 95th percentile 1.2347 (mean over seeds); 0 of 2 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.079 and 95th percentile 1.1647; the observed pooled 1.1641 has p = 0.05.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.037, 'filipino': 0.931, 'south_asian': 0.985, 'western': 0.929}, max/min **1.1163** (no-bias 95th percentile 1.1437, p = 0.133; labels shuffled within seed and programme).
- breast: {'arab': 4.4, 'filipino': 0.0, 'south_asian': 0.0, 'western': 0.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 1.0).
- cervical: {'arab': 58.2, 'filipino': 51.0, 'south_asian': 53.5, 'western': 53.1} items per 100 loops, max/min **1.1418** (no-bias 95th percentile 1.3143, p = 0.537).
- crc: {'arab': 60.8, 'filipino': 58.5, 'south_asian': 59.0, 'western': 52.6} items per 100 loops, max/min **1.1552** (no-bias 95th percentile 1.1839, p = 0.119).
- diabetic_retinopathy: {'arab': 11.7, 'filipino': 12.1, 'south_asian': 13.3, 'western': 13.8} items per 100 loops, max/min **1.1806** (no-bias 95th percentile 1.8157, p = 0.857).
- ifhas: {'arab': 18.5, 'filipino': 15.7, 'south_asian': 15.5, 'western': 18.8} items per 100 loops, max/min **1.2099** (no-bias 95th percentile 2.0141, p = 0.855).
- lung_ldct: {'arab': 1.8, 'filipino': 0.0, 'south_asian': 3.9, 'western': 2.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.465).
- lung_nodule: {'arab': 12.5, 'filipino': 4.3, 'south_asian': 9.4, 'western': 9.0} items per 100 loops, max/min **2.9013** (no-bias 95th percentile 1.9677, p = 0.003).
CRC loops by age band: {'40-59': 59.7, '60-79': 55.9, '80+': 60.4, '<40': 60.1} review items per 100 loops, max/min **1.0791** (no-bias 95th percentile 1.1427, p = 0.448); recall {'40-59': 0.759, '60-79': 0.751, '80+': 0.774, '<40': 0.796}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 59.7, 'M': 57.4} review items per 100 loops, max/min **1.041** (no-bias 95th percentile 1.0826, p = 0.32); recall {'F': 0.779, 'M': 0.751}; false closures {'F': 0, 'M': 0}.

| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off, DR, breast and LDCT readers, PET-low surveillance CT) | Count |
|---|---|
| breast_reader_breast-assess_agree | 25 |
| breast_reader_breast-assess_agree_but_wrong | 0 |
| breast_reader_breast-assess_coded | 0 |
| breast_reader_breast-assess_instances | 25 |
| breast_reader_breast-screen_agree | 142 |
| breast_reader_breast-screen_agree_but_wrong | 0 |
| breast_reader_breast-screen_coded | 101 |
| breast_reader_breast-screen_instances | 142 |
| dr_reader_dr-clinic_agree | 483 |
| dr_reader_dr-clinic_agree_but_wrong | 0 |
| dr_reader_dr-clinic_coded | 0 |
| dr_reader_dr-clinic_instances | 498 |
| dr_reader_dr-screen_agree | 2236 |
| dr_reader_dr-screen_agree_but_wrong | 0 |
| dr_reader_dr-screen_coded | 1514 |
| dr_reader_dr-screen_instances | 2477 |
| handoff_acted_state_correct | 131 |
| handoff_closed | 28 |
| handoff_dr_acted_state_correct | 10 |
| handoff_dr_closed | 5 |
| handoff_dr_packets_all_required_fields | 10 |
| handoff_dr_packets_matched | 10 |
| handoff_dr_truth_acted_by_act_by | 5 |
| handoff_dr_truth_closed_after_handoff | 5 |
| handoff_dr_truth_packets | 10 |
| handoff_dr_truth_referrer_acted | 8 |
| handoff_dr_truth_urgent | 9 |
| handoff_lung_acted_state_correct | 8 |
| handoff_lung_closed | 4 |
| handoff_lung_packets_all_required_fields | 8 |
| handoff_lung_packets_matched | 8 |
| handoff_lung_truth_acted_by_act_by | 5 |
| handoff_lung_truth_closed_after_handoff | 4 |
| handoff_lung_truth_packets | 8 |
| handoff_lung_truth_referrer_acted | 7 |
| handoff_lung_truth_urgent | 8 |
| handoff_packets_all_required_fields | 131 |
| handoff_packets_matched | 131 |
| handoff_truth_acted_by_act_by | 62 |
| handoff_truth_closed_after_handoff | 31 |
| handoff_truth_packets | 131 |
| handoff_truth_referrer_acted | 99 |
| handoff_truth_urgent | 96 |
| ifhas_reader_ifhas-followup_agree | 675 |
| ifhas_reader_ifhas-followup_agree_but_wrong | 0 |
| ifhas_reader_ifhas-followup_instances | 829 |
| ifhas_reader_ifhas-screen_agree | 1566 |
| ifhas_reader_ifhas-screen_agree_but_wrong | 0 |
| ifhas_reader_ifhas-screen_instances | 1600 |
| ldct_reader_ldct-screen_agree | 644 |
| ldct_reader_ldct-screen_agree_but_wrong | 0 |
| ldct_reader_ldct-screen_coded | 384 |
| ldct_reader_ldct-screen_instances | 649 |
| ldct_reader_ldct-workup_agree | 88 |
| ldct_reader_ldct-workup_agree_but_wrong | 0 |
| ldct_reader_ldct-workup_coded | 0 |
| ldct_reader_ldct-workup_instances | 96 |
| lung_surv_extra_opened | 0 |
| lung_surv_false_closures | 0 |
| lung_surv_missed | 1 |
| lung_surv_obligations | 41 |
| lung_surv_opened | 34 |
| lung_surv_outreach_lung_surveillance_due:consent_unknown_hold | 2 |
| lung_surv_outreach_lung_surveillance_recall:released | 8 |
| lung_surv_overdue_flag_correct | 39 |
| lung_surv_pred_closed | 18 |
| lung_surv_pred_overdue | 8 |
| lung_surv_tp | 18 |
| lung_surv_truth_closed | 19 |
| lung_surv_truth_overdue | 10 |
| lung_workup_pred_awaiting_tissue | 3 |
| lung_workup_pred_closed_by_ct | 37 |
| lung_workup_pred_closed_by_pet | 34 |
| lung_workup_pred_closed_by_tissue | 25 |
| lung_workup_truth_awaiting_tissue | 4 |
| lung_workup_truth_path_ct | 57 |
| lung_workup_truth_path_pet | 76 |
| lung_workup_truth_path_tissue | 24 |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
