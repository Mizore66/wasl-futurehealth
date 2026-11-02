# Final frozen evaluation

Code revision `075abc5ad04a` · policy `v7strict` · seeds [17, 29] · source `fhir` · held-out persons only · frozen 2026-11-02T03:49:02.775899Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (2 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 0.986 | 0.9606 – 1.0114 |
| crc_recall | 0.7535 | 0.6836 – 0.8234 |
| crc_recall_no_review | 0.317 | 0.2535 – 0.3805 |
| cervical_precision | 0.996 | 0.996 – 0.996 |
| cervical_recall | 0.9005 | 0.7925 – 1.0085 |
| kpi_truth | 0.328 | 0.2772 – 0.3788 |
| kpi_wasl | 0.2875 | 0.2684 – 0.3066 |
| kpi_eid_only | 0.1295 | 0.1231 – 0.1359 |
| kpi_abs_error_wasl | 0.0405 | 0.0087 – 0.0723 |
| kpi_abs_error_eid_only | 0.1985 | 0.1413 – 0.2557 |
| lung_precision | 0.9985 | 0.9794 – 1.0176 |
| lung_recall | 0.9365 | 0.9047 – 0.9683 |
| lung_state_accuracy | 0.97 | 0.97 – 0.97 |
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
| linkage_pair_f1 | 0.893 | 0.893 – 0.893 |
| reviewer_h_per_1k | 10.9645 | 10.5007 – 11.4283 |
| review_items_per_100_loops | 31.9205 | 28.2421 – 35.5989 |
| throughput_ev_s | 685.0682 | 568.4198 – 801.7166 |
| review_items_per_100_arab | 32.2435 | 27.6122 – 36.8748 |
| review_items_per_100_filipino | 30.545 | 19.4527 – 41.6373 |
| review_items_per_100_south_asian | 33.387 | 18.3177 – 48.4563 |
| review_items_per_100_western | 29.228 | 17.2335 – 41.2225 |
| reviewer_h_per_1k_arab | 11.097 | 8.5304 – 13.6636 |
| reviewer_h_per_1k_filipino | 10.2235 | 6.8246 – 13.6224 |
| reviewer_h_per_1k_south_asian | 11.5325 | 8.8833 – 14.1817 |
| reviewer_h_per_1k_western | 10.018 | 3.6142 – 16.4218 |
| review_load_ratio_max_min_origin | 1.1494 | 1.1048 – 1.1939 |
| ifhas_abnormal_findings_rate_truth | 0.3915 | 0.3089 – 0.4741 |
| ifhas_abnormal_findings_rate_wasl | 0.392 | 0.3031 – 0.4809 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.8555 | 0.2266 – 1.4844 |
| ifhas_followup_within_clock_truth | 0.539 | 0.1197 – 0.9583 |
| ifhas_followup_within_clock_wasl | 0.468 | -0.2181 – 1.1541 |
| ifhas_overdue_accuracy | 0.9645 | 0.8438 – 1.0852 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.955 | 0.7898 – 1.1202 |
| ifhas_state_accuracy | 0.9595 | 0.8261 – 1.0929 |
| ifhas_triage_accuracy | 0.9995 | 0.9931 – 1.0059 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 16 | 1166 | 2.2190% |
| cervical_false_closures | 2 | 520 | 1.3820% |
| wrong_patient_released | 0 | 6624 | 0.0560% |
| released_without_granted_consent | 0 | 4650 | 0.0790% |
| duplicate_or_lost_outputs | 0 | 85727 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 109482 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 42 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 0 | – | – |
| tamper_detected | 34 | 34 | – |
| tamper_false_alarms | 0 | 2 | – |
| lung_false_closures | 1 | 796 | 0.6980% |
| lung_surveillance_ct_false_closures | 0 | 18 | 18.5300% |
| lung_surveillance_ct_overdue_flag_correct | 39 | 41 | – |
| dr_false_closures | 0 | 2251 | 0.1640% |
| dr_referral_false_closures | 0 | 104 | 3.4850% |
| breast_false_closures | 0 | 139 | 2.6190% |
| breast_recall_false_closures | 0 | 6 | 45.9260% |
| ldct_false_closures | 0 | 565 | 0.6510% |
| ldct_action_false_closures | 0 | 50 | 7.1120% |
| ifhas_false_closures | 0 | 1353 | 0.2720% |
| ifhas_followup_false_closures | 0 | 380 | 0.9660% |
| handoff_false_closures | 0 | 28 | 12.3440% |
| handoff_lung_false_closures | 0 | 4 | 60.2360% |
| handoff_dr_false_closures | 0 | 5 | 52.1820% |

Review load by name origin: pooled over seeds {'arab': 32.2, 'filipino': 30.5, 'south_asian': 33.4, 'western': 29.2} items per 100 loops, max/min **1.1432**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1143 and 95th percentile 1.2441 (mean over seeds); 0 of 2 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0802 and 95th percentile 1.1685; the observed pooled 1.1432 has p = 0.101.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.04, 'filipino': 0.918, 'south_asian': 0.977, 'western': 0.941}, max/min **1.132** (no-bias 95th percentile 1.1446, p = 0.082; labels shuffled within seed and programme).
- breast: {'arab': 2.9, 'filipino': 0.0, 'south_asian': 0.0, 'western': 0.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 1.0).
- cervical: {'arab': 57.7, 'filipino': 47.1, 'south_asian': 52.1, 'western': 52.2} items per 100 loops, max/min **1.2241** (no-bias 95th percentile 1.3086, p = 0.221).
- crc: {'arab': 60.3, 'filipino': 57.9, 'south_asian': 58.9, 'western': 53.3} items per 100 loops, max/min **1.1308** (no-bias 95th percentile 1.1885, p = 0.239).
- diabetic_retinopathy: {'arab': 11.3, 'filipino': 10.4, 'south_asian': 13.1, 'western': 13.5} items per 100 loops, max/min **1.2959** (no-bias 95th percentile 1.805, p = 0.595).
- ifhas: {'arab': 16.4, 'filipino': 14.8, 'south_asian': 11.4, 'western': 17.3} items per 100 loops, max/min **1.5178** (no-bias 95th percentile 2.101, p = 0.414).
- lung_ldct: {'arab': 1.8, 'filipino': 0.0, 'south_asian': 3.1, 'western': 1.0} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.57).
- lung_nodule: {'arab': 11.4, 'filipino': 4.3, 'south_asian': 7.8, 'western': 7.9} items per 100 loops, max/min **2.6465** (no-bias 95th percentile 2.0712, p = 0.008).
CRC loops by age band: {'40-59': 58.0, '60-79': 56.2, '80+': 61.1, '<40': 60.1} review items per 100 loops, max/min **1.0865** (no-bias 95th percentile 1.1474, p = 0.399); recall {'40-59': 0.739, '60-79': 0.738, '80+': 0.752, '<40': 0.781}; false closures {'40-59': 4, '60-79': 2, '80+': 3, '<40': 7}.
CRC loops by sex: {'F': 59.1, 'M': 57.9} review items per 100 loops, max/min **1.0205** (no-bias 95th percentile 1.0844, p = 0.647); recall {'F': 0.758, 'M': 0.742}; false closures {'F': 13, 'M': 3}.

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
| lung_workup_pred_closed_by_ct_false | 1 |
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
