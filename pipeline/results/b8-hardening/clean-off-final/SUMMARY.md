# Final frozen evaluation

Code revision `075abc5ad04a` · policy `v7strict` · seeds [17, 29] · source `fhir` · held-out persons only · frozen 2026-11-02T04:18:45.738142Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (2 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7715 | 0.7143 – 0.8287 |
| crc_recall_no_review | 0.32 | 0.2692 – 0.3708 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.909 | 0.7819 – 1.0361 |
| kpi_truth | 0.328 | 0.2772 – 0.3788 |
| kpi_wasl | 0.293 | 0.2676 – 0.3184 |
| kpi_eid_only | 0.1295 | 0.1231 – 0.1359 |
| kpi_abs_error_wasl | 0.035 | 0.0096 – 0.0604 |
| kpi_abs_error_eid_only | 0.1985 | 0.1413 – 0.2557 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.945 | 0.8942 – 0.9958 |
| lung_state_accuracy | 0.9735 | 0.9671 – 0.9799 |
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
| linkage_pair_f1 | 0.942 | 0.9166 – 0.9674 |
| reviewer_h_per_1k | 10.999 | 10.3891 – 11.6089 |
| review_items_per_100_loops | 31.651 | 27.5724 – 35.7296 |
| throughput_ev_s | 686.6852 | 452.4641 – 920.9062 |
| review_items_per_100_arab | 32.099 | 29.6849 – 34.5131 |
| review_items_per_100_filipino | 30.3525 | 16.8143 – 43.8907 |
| review_items_per_100_south_asian | 32.799 | 22.5071 – 43.0909 |
| review_items_per_100_western | 28.9615 | 16.3127 – 41.6103 |
| reviewer_h_per_1k_arab | 11.139 | 9.6143 – 12.6637 |
| reviewer_h_per_1k_filipino | 10.1995 | 7.6519 – 12.7471 |
| reviewer_h_per_1k_south_asian | 11.536 | 10.5449 – 12.5271 |
| reviewer_h_per_1k_western | 10.109 | 3.5146 – 16.7034 |
| review_load_ratio_max_min_origin | 1.1382 | 0.9307 – 1.3458 |
| ifhas_abnormal_findings_rate_truth | 0.3915 | 0.3089 – 0.4741 |
| ifhas_abnormal_findings_rate_wasl | 0.392 | 0.3031 – 0.4809 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.8965 | 0.4073 – 1.3857 |
| ifhas_followup_within_clock_truth | 0.539 | 0.1197 – 0.9583 |
| ifhas_followup_within_clock_wasl | 0.49 | -0.0945 – 1.0745 |
| ifhas_overdue_accuracy | 0.975 | 0.8988 – 1.0512 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.9675 | 0.8341 – 1.1009 |
| ifhas_state_accuracy | 0.971 | 0.8694 – 1.0726 |
| ifhas_triage_accuracy | 0.9995 | 0.9931 – 1.0059 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1178 | 0.3130% |
| cervical_false_closures | 0 | 523 | 0.7030% |
| wrong_patient_released | 0 | 6557 | 0.0560% |
| released_without_granted_consent | 0 | 4734 | 0.0780% |
| duplicate_or_lost_outputs | 0 | 85747 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 109482 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 42 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 0 | – | – |
| tamper_detected | 34 | 34 | – |
| tamper_false_alarms | 0 | 2 | – |
| lung_false_closures | 0 | 802 | 0.4590% |
| lung_surveillance_ct_false_closures | 0 | 18 | 18.5300% |
| lung_surveillance_ct_overdue_flag_correct | 39 | 41 | – |
| dr_false_closures | 0 | 2254 | 0.1640% |
| dr_referral_false_closures | 0 | 107 | 3.3890% |
| breast_false_closures | 0 | 139 | 2.6190% |
| breast_recall_false_closures | 0 | 6 | 45.9260% |
| ldct_false_closures | 0 | 566 | 0.6500% |
| ldct_action_false_closures | 0 | 51 | 6.9780% |
| ifhas_false_closures | 0 | 1371 | 0.2690% |
| ifhas_followup_false_closures | 0 | 398 | 0.9230% |
| handoff_false_closures | 0 | 29 | 11.9440% |
| handoff_lung_false_closures | 0 | 4 | 60.2360% |
| handoff_dr_false_closures | 0 | 5 | 52.1820% |

Review load by name origin: pooled over seeds {'arab': 32.1, 'filipino': 30.4, 'south_asian': 32.8, 'western': 29.0} items per 100 loops, max/min **1.1332**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1134 and 95th percentile 1.2416 (mean over seeds); 0 of 2 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0794 and 95th percentile 1.1678; the observed pooled 1.1332 has p = 0.12.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.044, 'filipino': 0.921, 'south_asian': 0.969, 'western': 0.941}, max/min **1.1334** (no-bias 95th percentile 1.1445, p = 0.079; labels shuffled within seed and programme).
- breast: {'arab': 2.9, 'filipino': 0.0, 'south_asian': 0.0, 'western': 0.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 1.0).
- cervical: {'arab': 58.3, 'filipino': 47.1, 'south_asian': 50.9, 'western': 51.3} items per 100 loops, max/min **1.237** (no-bias 95th percentile 1.3122, p = 0.18).
- crc: {'arab': 59.4, 'filipino': 57.6, 'south_asian': 57.5, 'western': 52.5} items per 100 loops, max/min **1.133** (no-bias 95th percentile 1.19, p = 0.228).
- diabetic_retinopathy: {'arab': 11.2, 'filipino': 10.4, 'south_asian': 13.1, 'western': 13.5} items per 100 loops, max/min **1.2959** (no-bias 95th percentile 1.7943, p = 0.597).
- ifhas: {'arab': 16.9, 'filipino': 13.9, 'south_asian': 12.0, 'western': 17.8} items per 100 loops, max/min **1.4838** (no-bias 95th percentile 2.0671, p = 0.435).
- lung_ldct: {'arab': 1.8, 'filipino': 0.0, 'south_asian': 3.1, 'western': 2.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.522).
- lung_nodule: {'arab': 11.3, 'filipino': 4.3, 'south_asian': 7.8, 'western': 7.9} items per 100 loops, max/min **2.6269** (no-bias 95th percentile 2.0497, p = 0.007).
CRC loops by age band: {'40-59': 58.1, '60-79': 55.1, '80+': 59.4, '<40': 58.9} review items per 100 loops, max/min **1.0784** (no-bias 95th percentile 1.1459, p = 0.461); recall {'40-59': 0.761, '60-79': 0.753, '80+': 0.771, '<40': 0.798}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 58.3, 'M': 56.8} review items per 100 loops, max/min **1.0261** (no-bias 95th percentile 1.0812, p = 0.532); recall {'F': 0.78, 'M': 0.751}; false closures {'F': 0, 'M': 0}.

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
| handoff_closed | 29 |
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
