# Final frozen evaluation

Code revision `0f6a5325831e` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-31T18:41:02.615414Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.84 | 0.8238 – 0.8562 |
| crc_recall_no_review | 0.325 | 0.2991 – 0.3509 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9668 | 0.9485 – 0.9851 |
| kpi_truth | 0.323 | 0.3097 – 0.3363 |
| kpi_wasl | 0.3122 | 0.2986 – 0.3258 |
| kpi_eid_only | 0.1284 | 0.1197 – 0.1371 |
| kpi_abs_error_wasl | 0.0108 | 0.0086 – 0.013 |
| kpi_abs_error_eid_only | 0.1946 | 0.1849 – 0.2043 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9858 | 0.9806 – 0.991 |
| lung_state_accuracy | 0.994 | 0.9922 – 0.9958 |
| lung_escalation_recall | 0.9914 | 0.9766 – 1.0062 |
| dr_precision | 1 | 1.0 – 1.0 |
| dr_recall | 0.9976 | 0.9959 – 0.9993 |
| dr_referral_recall | 0.9768 | 0.9561 – 0.9975 |
| dr_state_accuracy | 0.9978 | 0.9968 – 0.9988 |
| dr_triage_accuracy | 0.9988 | 0.9978 – 0.9998 |
| dr_overdue_accuracy | 0.9984 | 0.997 – 0.9998 |
| dr_exam_within_deadline_truth | 0.3514 | 0.2929 – 0.4099 |
| dr_exam_within_deadline_wasl | 0.343 | 0.2811 – 0.4049 |
| linkage_pair_f1 | 0.9746 | 0.9732 – 0.976 |
| reviewer_h_per_1k | 14.711 | 14.3019 – 15.1201 |
| review_items_per_100_loops | 42.8228 | 42.3694 – 43.2762 |
| throughput_ev_s | 498.2881 | 464.9795 – 531.5967 |
| review_items_per_100_arab | 42.4756 | 41.0407 – 43.9105 |
| review_items_per_100_filipino | 41.9226 | 39.3856 – 44.4596 |
| review_items_per_100_south_asian | 44.1138 | 40.9484 – 47.2792 |
| review_items_per_100_western | 42.5324 | 40.3148 – 44.75 |
| reviewer_h_per_1k_arab | 14.564 | 14.0778 – 15.0502 |
| reviewer_h_per_1k_filipino | 14.4172 | 13.4568 – 15.3776 |
| reviewer_h_per_1k_south_asian | 15.0704 | 13.6277 – 16.5131 |
| reviewer_h_per_1k_western | 14.8272 | 13.5984 – 16.056 |
| review_load_ratio_max_min_origin | 1.1128 | 1.0606 – 1.165 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 3147 | 0.1170% |
| cervical_false_closures | 0 | 1403 | 0.2630% |
| wrong_patient_released | 0 | 15094 | 0.0240% |
| released_without_granted_consent | 0 | 10491 | 0.0350% |
| duplicate_or_lost_outputs | 0 | 204655 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 253148 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 117 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 2025 | 0.1820% |
| dr_false_closures | 0 | 5621 | 0.0660% |
| dr_referral_false_closures | 0 | 254 | 1.4420% |
| handoff_false_closures | 0 | 87 | 4.1510% |

Review load by name origin: pooled over seeds {'arab': 42.5, 'filipino': 41.9, 'south_asian': 44.1, 'western': 42.5} items per 100 loops, max/min **1.0528**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1088 and 95th percentile 1.2195 (mean over seeds); 0 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0454 and 95th percentile 1.0964; the observed pooled 1.0528 has p = 0.377.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.02, 'filipino': 0.942, 'south_asian': 0.975, 'western': 1.007}, max/min **1.083** (no-bias 95th percentile 1.0849, p = 0.057; labels shuffled within seed and programme).
- cervical: {'arab': 62.9, 'filipino': 59.4, 'south_asian': 58.6, 'western': 60.4} items per 100 loops, max/min **1.0733** (no-bias 95th percentile 1.1836, p = 0.676).
- crc: {'arab': 70.1, 'filipino': 63.7, 'south_asian': 67.1, 'western': 69.2} items per 100 loops, max/min **1.1004** (no-bias 95th percentile 1.1028, p = 0.058).
- diabetic_retinopathy: {'arab': 14.2, 'filipino': 14.2, 'south_asian': 12.8, 'western': 15.8} items per 100 loops, max/min **1.2334** (no-bias 95th percentile 1.4027, p = 0.345).
- lung_nodule: {'arab': 12.4, 'filipino': 11.2, 'south_asian': 13.6, 'western': 11.5} items per 100 loops, max/min **1.2157** (no-bias 95th percentile 1.4973, p = 0.535).
CRC loops by age band: {'40-59': 66.5, '60-79': 69.9, '80+': 69.9, '<40': 68.8} review items per 100 loops, max/min **1.0521** (no-bias 95th percentile 1.0843, p = 0.324); recall {'40-59': 0.813, '60-79': 0.851, '80+': 0.84, '<40': 0.858}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 67.7, 'M': 70.8} review items per 100 loops, max/min **1.0463** (no-bias 95th percentile 1.0449, p = 0.046); recall {'F': 0.84, 'M': 0.839}; false closures {'F': 0, 'M': 0}.

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
| handoff_closed | 87 |
| handoff_packets_all_required_fields | 330 |
| handoff_packets_matched | 330 |
| handoff_packets_missed | 3 |
| handoff_truth_acted_by_act_by | 152 |
| handoff_truth_closed_after_handoff | 96 |
| handoff_truth_packets | 333 |
| handoff_truth_referrer_acted | 256 |
| handoff_truth_urgent | 248 |
| lung_workup_pred_awaiting_tissue | 10 |
| lung_workup_pred_closed_by_ct | 93 |
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
| crc_recall | 0.7746 | 0.761 – 0.7882 |
| crc_recall_no_review | 0.3196 | 0.2957 – 0.3435 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9058 | 0.8798 – 0.9318 |
| kpi_truth | 0.329 | 0.3066 – 0.3514 |
| kpi_wasl | 0.2968 | 0.271 – 0.3226 |
| kpi_eid_only | 0.1224 | 0.1141 – 0.1307 |
| kpi_abs_error_wasl | 0.0322 | 0.0254 – 0.039 |
| kpi_abs_error_eid_only | 0.2066 | 0.1889 – 0.2243 |
| lung_precision | 1 | 1.0 – 1.0 |
| lung_recall | 0.9494 | 0.9404 – 0.9584 |
| lung_state_accuracy | 0.9772 | 0.9735 – 0.9809 |
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
| reviewer_h_per_1k | 13.1874 | 12.331 – 14.0438 |
| review_items_per_100_loops | 38.173 | 36.4735 – 39.8725 |
| throughput_ev_s | 666.2281 | 439.332 – 893.1242 |
| review_items_per_100_arab | 37.428 | 36.2827 – 38.5733 |
| review_items_per_100_filipino | 40.2316 | 34.6273 – 45.8359 |
| review_items_per_100_south_asian | 40.1906 | 34.2089 – 46.1723 |
| review_items_per_100_western | 36.4124 | 35.4777 – 37.3471 |
| reviewer_h_per_1k_arab | 12.9752 | 12.3862 – 13.5642 |
| reviewer_h_per_1k_filipino | 13.9062 | 11.5072 – 16.3052 |
| reviewer_h_per_1k_south_asian | 13.7366 | 11.6483 – 15.8249 |
| reviewer_h_per_1k_western | 12.652 | 12.1451 – 13.1589 |
| review_load_ratio_max_min_origin | 1.1814 | 1.0171 – 1.3457 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2995 | 0.1230% |
| cervical_false_closures | 0 | 1303 | 0.2830% |
| wrong_patient_released | 0 | 14343 | 0.0260% |
| released_without_granted_consent | 0 | 9473 | 0.0390% |
| duplicate_or_lost_outputs | 0 | 198734 | 0.0020% |
| ingest_lost_or_duplicated | 0 | 254055 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 117 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |
| lung_false_closures | 0 | 1898 | 0.1940% |
| dr_false_closures | 0 | 5608 | 0.0660% |
| dr_referral_false_closures | 0 | 214 | 1.7090% |
| handoff_false_closures | 0 | 78 | 4.6190% |

Review load by name origin: pooled over seeds {'arab': 37.4, 'filipino': 40.3, 'south_asian': 40.3, 'western': 36.4} items per 100 loops, max/min **1.1064**.
With origin labels shuffled (no origin effect), the per-seed max/min has median 1.1172 and 95th percentile 1.242 (mean over seeds); 2 of 5 seeds are above it at p < 0.05.
Pooled over seeds, the no-bias max/min has median 1.0501 and 95th percentile 1.101; the observed pooled 1.1064 has p = 0.032.
Standardised by programme (observed / expected items from each loop's programme mean): {'arab': 1.01, 'filipino': 1.006, 'south_asian': 0.993, 'western': 0.974}, max/min **1.0368** (no-bias 95th percentile 1.0914, p = 0.654; labels shuffled within seed and programme).
- cervical: {'arab': 53.3, 'filipino': 53.8, 'south_asian': 52.2, 'western': 49.5} items per 100 loops, max/min **1.0878** (no-bias 95th percentile 1.2058, p = 0.573).
- crc: {'arab': 62.7, 'filipino': 59.2, 'south_asian': 61.6, 'western': 61.6} items per 100 loops, max/min **1.0586** (no-bias 95th percentile 1.1143, p = 0.444).
- diabetic_retinopathy: {'arab': 12.9, 'filipino': 16.7, 'south_asian': 13.3, 'western': 14.1} items per 100 loops, max/min **1.2991** (no-bias 95th percentile 1.412, p = 0.175).
- lung_nodule: {'arab': 10.6, 'filipino': 12.5, 'south_asian': 9.9, 'western': 7.8} items per 100 loops, max/min **1.6014** (no-bias 95th percentile 1.5704, p = 0.039).
CRC loops by age band: {'40-59': 60.2, '60-79': 63.5, '80+': 62.3, '<40': 62.0} review items per 100 loops, max/min **1.0536** (no-bias 95th percentile 1.0885, p = 0.372); recall {'40-59': 0.77, '60-79': 0.782, '80+': 0.791, '<40': 0.762}; false closures {'40-59': 0, '60-79': 0, '80+': 0, '<40': 0}.
CRC loops by sex: {'F': 62.1, 'M': 61.5} review items per 100 loops, max/min **1.0105** (no-bias 95th percentile 1.0488, p = 0.704); recall {'F': 0.772, 'M': 0.78}; false closures {'F': 0, 'M': 0}.

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
