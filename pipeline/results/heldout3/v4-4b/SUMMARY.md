# Final frozen evaluation

Code revision `7f72a6e6bd0c` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-29T04:40:54.832376Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 0.9962 | 0.9937 – 0.9987 |
| crc_recall | 0.8222 | 0.8063 – 0.8381 |
| crc_recall_no_review | 0.0854 | 0.0672 – 0.1036 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9706 | 0.9462 – 0.995 |
| kpi_truth | 0.3172 | 0.2942 – 0.3402 |
| kpi_wasl | 0.3084 | 0.2845 – 0.3323 |
| kpi_eid_only | 0.1246 | 0.1163 – 0.1329 |
| kpi_abs_error_wasl | 0.0088 | 0.0063 – 0.0113 |
| kpi_abs_error_eid_only | 0.1926 | 0.1759 – 0.2093 |
| linkage_pair_f1 | 0.979 | 0.977 – 0.981 |
| reviewer_h_per_1k | 31.0624 | 30.0765 – 32.0483 |
| review_items_per_100_loops | 88.1428 | 86.1609 – 90.1247 |
| throughput_ev_s | 302.8051 | 286.3094 – 319.3008 |
| review_items_per_100_arab | 90.9286 | 87.5383 – 94.3189 |
| review_items_per_100_filipino | 86.304 | 75.3986 – 97.2094 |
| review_items_per_100_south_asian | 86.6438 | 80.7588 – 92.5288 |
| review_items_per_100_western | 82.3522 | 76.7455 – 87.9589 |
| reviewer_h_per_1k_arab | 31.4266 | 30.4779 – 32.3753 |
| reviewer_h_per_1k_filipino | 31.2564 | 26.0469 – 36.4659 |
| reviewer_h_per_1k_south_asian | 30.6782 | 27.9618 – 33.3946 |
| reviewer_h_per_1k_western | 30.395 | 29.381 – 31.409 |
| review_load_ratio_max_min_origin | 1.1832 | 1.1626 – 1.2037 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 9 | 2324 | 0.7340% |
| cervical_false_closures | 0 | 1007 | 0.3660% |
| wrong_patient_released | 0 | 10030 | 0.0370% |
| released_without_granted_consent | 0 | 6162 | 0.0600% |
| duplicate_or_lost_outputs | 0 | 126991 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153102 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 110 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 0.996 | 0.9934 – 0.9986 |
| crc_recall | 0.7554 | 0.7277 – 0.7831 |
| crc_recall_no_review | 0.0926 | 0.0758 – 0.1094 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9066 | 0.8994 – 0.9138 |
| kpi_truth | 0.3222 | 0.3077 – 0.3367 |
| kpi_wasl | 0.2884 | 0.2716 – 0.3052 |
| kpi_eid_only | 0.1304 | 0.1224 – 0.1384 |
| kpi_abs_error_wasl | 0.0338 | 0.0273 – 0.0403 |
| kpi_abs_error_eid_only | 0.1918 | 0.1775 – 0.2061 |
| linkage_pair_f1 | 0.9406 | 0.9364 – 0.9448 |
| reviewer_h_per_1k | 28.3668 | 27.5333 – 29.2003 |
| review_items_per_100_loops | 78.5046 | 76.3486 – 80.6606 |
| throughput_ev_s | 359.4914 | 158.1206 – 560.8621 |
| review_items_per_100_arab | 80.951 | 78.0583 – 83.8437 |
| review_items_per_100_filipino | 76.3346 | 66.6101 – 86.0591 |
| review_items_per_100_south_asian | 77.0104 | 72.7764 – 81.2444 |
| review_items_per_100_western | 73.8846 | 64.1956 – 83.5736 |
| reviewer_h_per_1k_arab | 29.4664 | 27.9845 – 30.9483 |
| reviewer_h_per_1k_filipino | 26.9216 | 22.9398 – 30.9034 |
| reviewer_h_per_1k_south_asian | 27.7088 | 25.2626 – 30.155 |
| reviewer_h_per_1k_western | 26.5586 | 23.6765 – 29.4407 |
| review_load_ratio_max_min_origin | 1.1996 | 1.1437 – 1.2555 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 9 | 2183 | 0.7810% |
| cervical_false_closures | 0 | 961 | 0.3830% |
| wrong_patient_released | 0 | 9408 | 0.0390% |
| released_without_granted_consent | 0 | 5747 | 0.0640% |
| duplicate_or_lost_outputs | 0 | 122574 | 0.0030% |
| ingest_lost_or_duplicated | 0 | 153977 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 104 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
