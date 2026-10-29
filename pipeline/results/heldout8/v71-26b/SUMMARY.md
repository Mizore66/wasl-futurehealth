# Final frozen evaluation

Code revision `2cac5690647b` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T09:06:51.970900Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8126 | 0.8028 – 0.8224 |
| crc_recall_no_review | 0.1098 | 0.0938 – 0.1258 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9696 | 0.9543 – 0.9849 |
| kpi_truth | 0.327 | 0.3025 – 0.3515 |
| kpi_wasl | 0.3154 | 0.2913 – 0.3395 |
| kpi_eid_only | 0.1202 | 0.1061 – 0.1343 |
| kpi_abs_error_wasl | 0.0116 | 0.0099 – 0.0133 |
| kpi_abs_error_eid_only | 0.2068 | 0.1926 – 0.221 |
| linkage_pair_f1 | 0.9756 | 0.9722 – 0.979 |
| reviewer_h_per_1k | 29.6762 | 28.4991 – 30.8533 |
| review_items_per_100_loops | 85.7238 | 83.8254 – 87.6222 |
| throughput_ev_s | 505.1583 | 380.7022 – 629.6143 |
| review_items_per_100_arab | 89.4304 | 86.2271 – 92.6337 |
| review_items_per_100_filipino | 79.4292 | 70.9774 – 87.881 |
| review_items_per_100_south_asian | 85.098 | 78.0095 – 92.1865 |
| review_items_per_100_western | 77.9566 | 70.6992 – 85.214 |
| reviewer_h_per_1k_arab | 30.8402 | 29.4176 – 32.2628 |
| reviewer_h_per_1k_filipino | 27.102 | 23.8891 – 30.3149 |
| reviewer_h_per_1k_south_asian | 29.6304 | 26.3782 – 32.8826 |
| reviewer_h_per_1k_western | 27.2776 | 24.8875 – 29.6677 |
| review_load_ratio_max_min_origin | 1.1989 | 1.1043 – 1.2935 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2346 | 0.1570% |
| cervical_false_closures | 0 | 1006 | 0.3660% |
| wrong_patient_released | 0 | 10087 | 0.0370% |
| released_without_granted_consent | 0 | 5530 | 0.0670% |
| duplicate_or_lost_outputs | 0 | 124822 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153052 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 119 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.7456 | 0.7262 – 0.765 |
| crc_recall_no_review | 0.1106 | 0.1074 – 0.1138 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.916 | 0.9054 – 0.9266 |
| kpi_truth | 0.3184 | 0.3082 – 0.3286 |
| kpi_wasl | 0.2874 | 0.2816 – 0.2932 |
| kpi_eid_only | 0.1262 | 0.1193 – 0.1331 |
| kpi_abs_error_wasl | 0.031 | 0.0228 – 0.0392 |
| kpi_abs_error_eid_only | 0.1922 | 0.1831 – 0.2013 |
| linkage_pair_f1 | 0.9428 | 0.9404 – 0.9452 |
| reviewer_h_per_1k | 26.6258 | 26.0118 – 27.2398 |
| review_items_per_100_loops | 75.9618 | 75.4546 – 76.469 |
| throughput_ev_s | 566.5578 | 486.7891 – 646.3264 |
| review_items_per_100_arab | 78.9726 | 75.8394 – 82.1058 |
| review_items_per_100_filipino | 74.16 | 67.786 – 80.534 |
| review_items_per_100_south_asian | 72.7126 | 69.2168 – 76.2084 |
| review_items_per_100_western | 71.9514 | 60.8729 – 83.0299 |
| reviewer_h_per_1k_arab | 27.3244 | 26.1074 – 28.5414 |
| reviewer_h_per_1k_filipino | 27.0066 | 24.4175 – 29.5957 |
| reviewer_h_per_1k_south_asian | 25.4138 | 24.6238 – 26.2038 |
| reviewer_h_per_1k_western | 25.9796 | 22.2151 – 29.7441 |
| review_load_ratio_max_min_origin | 1.1985 | 1.0758 – 1.3211 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2100 | 0.1760% |
| cervical_false_closures | 0 | 971 | 0.3790% |
| wrong_patient_released | 0 | 9444 | 0.0390% |
| released_without_granted_consent | 0 | 5539 | 0.0670% |
| duplicate_or_lost_outputs | 0 | 121026 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153807 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 107 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
