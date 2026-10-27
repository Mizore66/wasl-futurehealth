# Final frozen evaluation

Code revision `ebf85a955ab1` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-27T18:55:31.515600Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 0.9996 | 0.9985 – 1.0007 |
| crc_recall | 0.9564 | 0.945 – 0.9678 |
| crc_recall_no_review | 0.2774 | 0.2568 – 0.298 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9776 | 0.9666 – 0.9886 |
| kpi_truth | 0.3282 | 0.3114 – 0.345 |
| kpi_wasl | 0.32 | 0.2997 – 0.3403 |
| kpi_eid_only | 0.1274 | 0.1193 – 0.1355 |
| kpi_abs_error_wasl | 0.0082 | 0.0043 – 0.0121 |
| kpi_abs_error_eid_only | 0.2008 | 0.1917 – 0.2099 |
| linkage_pair_f1 | 0.9788 | 0.976 – 0.9816 |
| reviewer_h_per_1k | 18.7934 | 17.9928 – 19.594 |
| review_items_per_100_loops | 50.6668 | 47.9795 – 53.3541 |
| throughput_ev_s | 398.5864 | 120.6658 – 676.507 |
| review_items_per_100_arab | 52.5226 | 49.8072 – 55.238 |
| review_items_per_100_filipino | 49.1272 | 45.65 – 52.6044 |
| review_items_per_100_south_asian | 47.1212 | 42.0601 – 52.1823 |
| review_items_per_100_western | 49.9492 | 45.9064 – 53.992 |
| reviewer_h_per_1k_arab | 19.1898 | 18.2167 – 20.1629 |
| reviewer_h_per_1k_filipino | 18.7226 | 17.1483 – 20.2969 |
| reviewer_h_per_1k_south_asian | 17.4268 | 16.1251 – 18.7285 |
| reviewer_h_per_1k_western | 19.4084 | 17.757 – 21.0598 |
| review_load_ratio_max_min_origin | 1.1335 | 1.0292 – 1.2378 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 1 | 2757 | 0.2020% |
| cervical_false_closures | 0 | 1031 | 0.3570% |
| wrong_patient_released | 0 | 3991 | 0.0920% |
| released_without_granted_consent | 0 | 2791 | 0.1320% |
| duplicate_or_lost_outputs | 0 | 92329 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 153118 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 120 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8726 | 0.8618 – 0.8834 |
| crc_recall_no_review | 0.233 | 0.2011 – 0.2649 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.8934 | 0.8575 – 0.9293 |
| kpi_truth | 0.3218 | 0.3091 – 0.3345 |
| kpi_wasl | 0.2864 | 0.2763 – 0.2965 |
| kpi_eid_only | 0.121 | 0.1089 – 0.1331 |
| kpi_abs_error_wasl | 0.0354 | 0.0317 – 0.0391 |
| kpi_abs_error_eid_only | 0.2008 | 0.1877 – 0.2139 |
| linkage_pair_f1 | 0.9352 | 0.9295 – 0.9409 |
| reviewer_h_per_1k | 18.0804 | 17.0988 – 19.062 |
| review_items_per_100_loops | 49.9716 | 47.0914 – 52.8518 |
| throughput_ev_s | 569.8939 | 510.5148 – 629.2729 |
| review_items_per_100_arab | 51.199 | 47.8012 – 54.5968 |
| review_items_per_100_filipino | 50.2966 | 44.2478 – 56.3454 |
| review_items_per_100_south_asian | 49.47 | 45.3671 – 53.5729 |
| review_items_per_100_western | 46.267 | 42.2275 – 50.3065 |
| reviewer_h_per_1k_arab | 18.3426 | 17.2319 – 19.4533 |
| reviewer_h_per_1k_filipino | 17.8786 | 14.8142 – 20.943 |
| reviewer_h_per_1k_south_asian | 17.969 | 16.5122 – 19.4258 |
| reviewer_h_per_1k_western | 17.348 | 16.5786 – 18.1174 |
| review_load_ratio_max_min_origin | 1.1597 | 1.0406 – 1.2789 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2498 | 0.1480% |
| cervical_false_closures | 0 | 917 | 0.4010% |
| wrong_patient_released | 0 | 4063 | 0.0910% |
| released_without_granted_consent | 0 | 2778 | 0.1330% |
| duplicate_or_lost_outputs | 0 | 94015 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 152539 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 120 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 3 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
