# Final frozen evaluation

Code revision `075abc5ad04a` · policy `v7strict` · seeds [17, 29] · source `fhir` · held-out persons only · frozen 2026-11-02T04:04:02.726586Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (2 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.742 | 0.6785 – 0.8055 |
| crc_recall_no_review | 0.2815 | 0.1989 – 0.3641 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.8955 | 0.8002 – 0.9908 |
| kpi_truth | 0.328 | 0.2772 – 0.3788 |
| kpi_wasl | 0.2815 | 0.2497 – 0.3133 |
| kpi_eid_only | 0.1295 | 0.1231 – 0.1359 |
| kpi_abs_error_wasl | 0.0465 | 0.0274 – 0.0656 |
| kpi_abs_error_eid_only | 0.1985 | 0.1413 – 0.2557 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9355 | 0.891 – 0.98 |
| lung_state_accuracy | 0.97 | 0.9573 – 0.9827 |
| lung_escalation_recall | 0.8815 | 0.4813 – 1.2817 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.991 | 0.9656 – 1.0164 |
| dr_referral_recall | 0.8395 | 0.4647 – 1.2143 |
| dr_state_accuracy | 0.991 | 0.9656 – 1.0164 |
| dr_triage_accuracy | 1 | 1.0 – 1.0 |
| dr_overdue_accuracy | 0.9915 | 0.9724 – 1.0106 |
| dr_exam_within_deadline_truth | 0.46 | -0.4167 – 1.3367 |
| dr_exam_within_deadline_wasl | 0.369 | -0.1774 – 0.9154 |
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
| ldct_recall | 0.995 | 0.9315 – 1.0585 |
| ldct_action_loop_recall | 0.9545 | 0.3764 – 1.5326 |
| ldct_state_accuracy | 0.995 | 0.9315 – 1.0585 |
| ldct_triage_accuracy | 1 | 1.0 – 1.0 |
| ldct_overdue_accuracy | 0.995 | 0.9315 – 1.0585 |
| ldct_action_outcome_accuracy | 1 | 1.0 – 1.0 |
| ldct_next_step_within_clock_truth | 0.5955 | -0.1478 – 1.3388 |
| ldct_next_step_within_clock_wasl | 0.5585 | -0.6549 – 1.7719 |
| ldct_abnormal_recall_rate_truth | 0.1315 | 0.0235 – 0.2395 |
| ldct_abnormal_recall_rate_wasl | 0.1315 | 0.0235 – 0.2395 |
| ldct_cancer_detection_rate_truth | 0.0135 | -0.0691 – 0.0961 |
| ldct_cancer_detection_rate_wasl | 0.01 | -0.1171 – 0.1371 |
| linkage_pair_f1 | 0.9135 | 0.8944 – 0.9326 |
| reviewer_h_per_1k | 11.308 | 10.9649 – 11.6511 |
| review_items_per_100_loops | 33.012 | 29.3273 – 36.6967 |
| throughput_ev_s | 721.6623 | 487.8429 – 955.4817 |
| review_items_per_100_arab | 33.251 | 32.4505 – 34.0515 |
| review_items_per_100_filipino | 31.891 | 25.7413 – 38.0407 |
| review_items_per_100_south_asian | 34.857 | 23.5868 – 46.1272 |
| review_items_per_100_western | 29.915 | 29.3813 – 30.4487 |
| reviewer_h_per_1k_arab | 11.3695 | 10.7914 – 11.9476 |
| reviewer_h_per_1k_filipino | 10.528 | 10.3882 – 10.6678 |
| reviewer_h_per_1k_south_asian | 12.13 | 11.5963 – 12.6637 |
| reviewer_h_per_1k_western | 10.261 | 6.8685 – 13.6535 |
| review_load_ratio_max_min_origin | 1.1652 | 0.8092 – 1.5211 |
| ifhas_abnormal_findings_rate_truth | 0.3915 | 0.3089 – 0.4741 |
| ifhas_abnormal_findings_rate_wasl | 0.392 | 0.3031 – 0.4809 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.8515 | 0.2734 – 1.4296 |
| ifhas_followup_within_clock_truth | 0.539 | 0.1197 – 0.9583 |
| ifhas_followup_within_clock_wasl | 0.464 | -0.1713 – 1.0993 |
| ifhas_overdue_accuracy | 0.963 | 0.8614 – 1.0646 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.9535 | 0.8074 – 1.0996 |
| ifhas_state_accuracy | 0.9585 | 0.8378 – 1.0792 |
| ifhas_triage_accuracy | 0.9995 | 0.9931 – 1.0059 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 1133 | 0.3250% |
| cervical_false_closures | 0 | 515 | 0.7140% |
| wrong_patient_released | 0 | 6481 | 0.0570% |
| released_without_granted_consent | 0 | 4647 | 0.0790% |
| duplicate_or_lost_outputs | 0 | 85719 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 109482 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 42 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 0 | – | – |
| tamper_detected | 34 | 34 | – |
| tamper_false_alarms | 0 | 2 | – |
| lung_false_closures | 0 | 794 | 0.4640% |
| lung_surveillance_ct_false_closures | 0 | 18 | 18.5300% |
| lung_surveillance_ct_overdue_flag_correct | 39 | 41 | – |
| dr_false_closures | 0 | 2251 | 0.1640% |
| dr_referral_false_closures | 0 | 104 | 3.4850% |
| breast_false_closures | 0 | 139 | 2.6190% |
| breast_recall_false_closures | 0 | 6 | 45.9260% |
| ldct_false_closures | 0 | 565 | 0.6510% |
| ldct_action_false_closures | 0 | 50 | 7.1120% |
| ifhas_false_closures | 0 | 1351 | 0.2730% |
| ifhas_followup_false_closures | 0 | 378 | 0.9710% |
| handoff_false_closures | 0 | 27 | 12.7700% |
| handoff_lung_false_closures | 0 | 4 | 60.2360% |
| handoff_dr_false_closures | 0 | 5 | 52.1820% |

Review load by name origin: pooled over seeds {'arab': 33.3, 'filipino': 31.9, 'south_asian': 34.9, 'western': 29.9} items per 100 loops, max/min **1.1656**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.113 and 95th percentile 1.2349 (mean over seeds); 0 of 2 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0786 and 95th percentile 1.1631; the observed pooled 1.1656 has p = 0.047.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.036, 'filipino': 0.927, 'south_asian': 0.988, 'western': 0.931}, max/min **1.1172** (no-bias 95th percentile 1.1414, p = 0.126; labels shuffled within seed and programme).
- breast: {'arab': 4.4, 'filipino': 0.0, 'south_asian': 0.0, 'western': 0.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 1.0).
- cervical: {'arab': 59.6, 'filipino': 52.9, 'south_asian': 54.0, 'western': 54.9} items per 100 loops, max/min **1.1281** (no-bias 95th percentile 1.3103, p = 0.589).
- crc: {'arab': 61.4, 'filipino': 57.6, 'south_asian': 60.3, 'western': 53.8} items per 100 loops, max/min **1.1411** (no-bias 95th percentile 1.188, p = 0.175).
- diabetic_retinopathy: {'arab': 11.5, 'filipino': 12.1, 'south_asian': 13.1, 'western': 13.8} items per 100 loops, max/min **1.1955** (no-bias 95th percentile 1.8084, p = 0.823).
- ifhas: {'arab': 17.8, 'filipino': 15.7, 'south_asian': 14.9, 'western': 17.3} items per 100 loops, max/min **1.1906** (no-bias 95th percentile 2.0608, p = 0.896).
- lung_ldct: {'arab': 1.8, 'filipino': 0.0, 'south_asian': 3.9, 'western': 1.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.514).
- lung_nodule: {'arab': 12.4, 'filipino': 5.4, 'south_asian': 9.4, 'western': 8.7} items per 100 loops, max/min **2.3054** (no-bias 95th percentile 1.9677, p = 0.013).
CRC loops by age band: {'40-59': 59.5, '60-79': 57.4, '80+': 61.3, '<40': 60.9} review items per 100 loops, max/min **1.0688** (no-bias 95th percentile 1.1458, p = 0.571); recall {'40-59': 0.727, '60-79': 0.727, '80+': 0.749, '<40': 0.765}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 60.6, 'M': 57.9} review items per 100 loops, max/min **1.0469** (no-bias 95th percentile 1.0815, p = 0.247); recall {'F': 0.745, 'M': 0.736}; false closures {'F': 0, 'M': 0}.

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
| handoff_closed | 27 |
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
| lung_workup_pred_closed_by_ct | 36 |
| lung_workup_pred_closed_by_pet | 34 |
| lung_workup_pred_closed_by_tissue | 24 |
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
