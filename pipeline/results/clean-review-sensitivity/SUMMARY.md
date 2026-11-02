# Post-hoc clean-review sensitivity evaluation

Code revision `64031803cc96` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · run 2026-11-02T06:13:43.518894Z

**Both simulated reviewer-error rates are zero in this sensitivity run.** This does not replace the frozen stress-test evaluation and is not measured clinical performance. See `README.md` for the matched comparison.

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.859 | 0.835 – 0.883 |
| crc_recall_no_review | 0.3128 | 0.2884 – 0.3372 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9294 | 0.9044 – 0.9544 |
| kpi_truth | 0.329 | 0.3066 – 0.3514 |
| kpi_wasl | 0.302 | 0.2752 – 0.3288 |
| kpi_eid_only | 0.1224 | 0.1141 – 0.1307 |
| kpi_abs_error_wasl | 0.027 | 0.0212 – 0.0328 |
| kpi_abs_error_eid_only | 0.2066 | 0.1889 – 0.2243 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9598 | 0.9486 – 0.971 |
| lung_state_accuracy | 0.9814 | 0.9768 – 0.986 |
| lung_escalation_recall | 0.9324 | 0.9017 – 0.9631 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9956 | 0.9929 – 0.9983 |
| dr_referral_recall | 0.909 | 0.8488 – 0.9692 |
| dr_state_accuracy | 0.996 | 0.9928 – 0.9992 |
| dr_triage_accuracy | 1 | 1.0 – 1.0 |
| dr_overdue_accuracy | 0.9966 | 0.9942 – 0.999 |
| dr_exam_within_deadline_truth | 0.4338 | 0.3443 – 0.5233 |
| dr_exam_within_deadline_wasl | 0.3902 | 0.3097 – 0.4707 |
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
| linkage_pair_f1 | 0.9462 | 0.9428 – 0.9496 |
| reviewer_h_per_1k | 11.3782 | 10.8025 – 11.9539 |
| review_items_per_100_loops | 33.7042 | 32.4048 – 35.0036 |
| throughput_ev_s | 618.7109 | 512.5166 – 724.9052 |
| review_items_per_100_arab | 33.3318 | 31.8524 – 34.8112 |
| review_items_per_100_filipino | 35.522 | 29.6918 – 41.3522 |
| review_items_per_100_south_asian | 35.2518 | 30.6034 – 39.9002 |
| review_items_per_100_western | 31.6108 | 30.7037 – 32.5179 |
| reviewer_h_per_1k_arab | 11.2548 | 10.6588 – 11.8508 |
| reviewer_h_per_1k_filipino | 11.9942 | 9.5993 – 14.3891 |
| reviewer_h_per_1k_south_asian | 11.921 | 10.1505 – 13.6915 |
| reviewer_h_per_1k_western | 10.6412 | 10.2654 – 11.017 |
| review_load_ratio_max_min_origin | 1.211 | 1.0336 – 1.3884 |
| ifhas_abnormal_findings_rate_truth | 0.4136 | 0.3988 – 0.4284 |
| ifhas_abnormal_findings_rate_wasl | 0.4136 | 0.3988 – 0.4284 |
| ifhas_domain_outcome_accuracy | 1 | 1.0 – 1.0 |
| ifhas_followup_loop_recall | 0.92 | 0.9004 – 0.9396 |
| ifhas_followup_within_clock_truth | 0.5468 | 0.517 – 0.5766 |
| ifhas_followup_within_clock_wasl | 0.5038 | 0.4697 – 0.5379 |
| ifhas_overdue_accuracy | 0.9786 | 0.9733 – 0.9839 |
| ifhas_precision | 1 | 1.0 – 1.0 |
| ifhas_recall | 0.9736 | 0.9679 – 0.9793 |
| ifhas_state_accuracy | 0.9772 | 0.9721 – 0.9823 |
| ifhas_triage_accuracy | 1 | 1.0 – 1.0 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3322 | 0.1110% |
| cervical_false_closures | 0 | 1337 | 0.2760% |
| wrong_patient_released | 0 | 16534 | 0.0220% |
| released_without_granted_consent | 0 | 11610 | 0.0320% |
| duplicate_or_lost_outputs | 0 | 216782 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 274706 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 106 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1922 | 0.1920% |
| lung_surveillance_ct_false_closures | 0 | 32 | 10.8880% |
| lung_surveillance_ct_overdue_flag_correct | 89 | 91 | – |
| dr_false_closures | 0 | 5636 | 0.0650% |
| dr_referral_false_closures | 0 | 237 | 1.5440% |
| breast_false_closures | 0 | 338 | 1.0850% |
| breast_recall_false_closures | 0 | 23 | 14.8190% |
| ldct_false_closures | 0 | 1414 | 0.2610% |
| ldct_action_false_closures | 0 | 122 | 2.9780% |
| ifhas_false_closures | 0 | 3409 | 0.1080% |
| ifhas_followup_false_closures | 0 | 1063 | 0.3460% |
| handoff_false_closures | 0 | 92 | 3.9300% |
| handoff_lung_false_closures | 0 | 4 | 60.2360% |
| handoff_dr_false_closures | 0 | 17 | 19.5060% |

Review load by name origin: pooled over seeds {'arab': 33.3, 'filipino': 35.6, 'south_asian': 35.3, 'western': 31.6} items per 100 loops, max/min **1.125**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1133 and 95th percentile 1.2396 (mean over seeds); 2 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0502 and 95th percentile 1.0986; the observed pooled 1.125 has p = 0.007.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.021, 'filipino': 0.992, 'south_asian': 0.982, 'western': 0.957}, max/min **1.0664** (no-bias 95th percentile 1.0904, p = 0.182; labels shuffled within seed and programme).
- breast: {'arab': 2.2, 'filipino': 0.0, 'south_asian': 3.9, 'western': 8.5} items per 100 loops, max/min **inf** (no-bias 95th percentile inf, p = 0.433).
- cervical: {'arab': 54.2, 'filipino': 54.4, 'south_asian': 51.4, 'western': 49.3} items per 100 loops, max/min **1.1023** (no-bias 95th percentile 1.199, p = 0.405).
- crc: {'arab': 63.2, 'filipino': 58.6, 'south_asian': 61.4, 'western': 61.2} items per 100 loops, max/min **1.0796** (no-bias 95th percentile 1.116, p = 0.214).
- diabetic_retinopathy: {'arab': 13.2, 'filipino': 17.0, 'south_asian': 13.2, 'western': 14.0} items per 100 loops, max/min **1.2892** (no-bias 95th percentile 1.4151, p = 0.203).
- ifhas: {'arab': 19.5, 'filipino': 18.6, 'south_asian': 17.7, 'western': 16.8} items per 100 loops, max/min **1.1579** (no-bias 95th percentile 1.5561, p = 0.751).
- lung_ldct: {'arab': 2.9, 'filipino': 1.9, 'south_asian': 2.5, 'western': 1.1} items per 100 loops, max/min **2.6132** (no-bias 95th percentile inf, p = 0.576).
- lung_nodule: {'arab': 11.0, 'filipino': 11.6, 'south_asian': 10.0, 'western': 7.5} items per 100 loops, max/min **1.5502** (no-bias 95th percentile 1.5513, p = 0.051).
CRC loops by age band: {'40-59': 59.5, '60-79': 63.9, '80+': 63.4, '<40': 62.0} review items per 100 loops, max/min **1.0746** (no-bias 95th percentile 1.087, p = 0.118); recall {'40-59': 0.849, '60-79': 0.873, '80+': 0.869, '<40': 0.85}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 62.6, 'M': 60.7} review items per 100 loops, max/min **1.0305** (no-bias 95th percentile 1.0479, p = 0.228); recall {'F': 0.858, 'M': 0.863}; false closures {'F': 0, 'M': 0}.

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
| handoff_closed | 92 |
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
| lung_surv_opened | 86 |
| lung_surv_outreach_lung_surveillance_due:released | 4 |
| lung_surv_outreach_lung_surveillance_recall:consent_unknown_hold | 2 |
| lung_surv_outreach_lung_surveillance_recall:consent_withheld | 1 |
| lung_surv_outreach_lung_surveillance_recall:released | 15 |
| lung_surv_overdue_flag_correct | 89 |
| lung_surv_pred_closed | 32 |
| lung_surv_pred_overdue | 18 |
| lung_surv_tp | 32 |
| lung_surv_truth_closed | 33 |
| lung_surv_truth_overdue | 20 |
| lung_workup_pred_awaiting_tissue | 9 |
| lung_workup_pred_closed_by_ct | 93 |
| lung_workup_pred_closed_by_pet | 86 |
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
