# Final frozen evaluation

Code revision `45ed3a0ee0d4` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-27T04:49:27.187797Z

runs executed at code_revision; report7 re-scored from the unchanged run logs at report_revision (forged-effect attribution fix only) (`5bb022a97094`).

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.9542 | 0.9436 – 0.9648 |
| crc_recall_no_review | 0.2604 | 0.2429 – 0.2779 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9642 | 0.9397 – 0.9887 |
| kpi_truth | 0.3282 | 0.3114 – 0.345 |
| kpi_wasl | 0.319 | 0.3 – 0.338 |
| kpi_eid_only | 0.1274 | 0.1193 – 0.1355 |
| kpi_abs_error_wasl | 0.0092 | 0.006 – 0.0124 |
| kpi_abs_error_eid_only | 0.2008 | 0.1917 – 0.2099 |
| linkage_pair_f1 | 0.9706 | 0.9677 – 0.9735 |
| reviewer_h_per_1k | 20.0708 | 18.9874 – 21.1542 |
| review_items_per_100_loops | 55.2286 | 52.3296 – 58.1276 |
| throughput_ev_s | 713.7351 | 654.83 – 772.6401 |
| review_items_per_100_arab | 56.9034 | 53.2267 – 60.5801 |
| review_items_per_100_filipino | 53.209 | 46.8466 – 59.5714 |
| review_items_per_100_south_asian | 51.8858 | 48.7424 – 55.0292 |
| review_items_per_100_western | 55.223 | 52.7242 – 57.7218 |
| reviewer_h_per_1k_arab | 20.3464 | 18.8412 – 21.8516 |
| reviewer_h_per_1k_filipino | 19.8818 | 17.257 – 22.5066 |
| reviewer_h_per_1k_south_asian | 18.7852 | 17.751 – 19.8194 |
| reviewer_h_per_1k_western | 21.0756 | 19.9758 – 22.1754 |
| review_load_ratio_max_min_origin | 1.1239 | 1.0217 – 1.2261 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2750 | 0.1340% |
| cervical_false_closures | 0 | 1017 | 0.3620% |
| wrong_patient_released | 0 | 4260 | 0.0870% |
| released_without_granted_consent | 0 | 2910 | 0.1270% |
| duplicate_or_lost_outputs | 0 | 94523 | 0.0040% |
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
| crc_recall | 0.8638 | 0.8553 – 0.8723 |
| crc_recall_no_review | 0.2146 | 0.177 – 0.2522 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.8886 | 0.8524 – 0.9248 |
| kpi_truth | 0.3218 | 0.3091 – 0.3345 |
| kpi_wasl | 0.2836 | 0.2734 – 0.2938 |
| kpi_eid_only | 0.121 | 0.1089 – 0.1331 |
| kpi_abs_error_wasl | 0.0382 | 0.0344 – 0.042 |
| kpi_abs_error_eid_only | 0.2008 | 0.1877 – 0.2139 |
| linkage_pair_f1 | 0.9278 | 0.9241 – 0.9315 |
| reviewer_h_per_1k | 18.4738 | 17.2013 – 19.7463 |
| review_items_per_100_loops | 51.6382 | 48.0387 – 55.2377 |
| throughput_ev_s | 665.5325 | 567.0341 – 764.0308 |
| review_items_per_100_arab | 52.5608 | 48.2884 – 56.8332 |
| review_items_per_100_filipino | 52.4374 | 45.9282 – 58.9466 |
| review_items_per_100_south_asian | 51.0792 | 47.0141 – 55.1443 |
| review_items_per_100_western | 48.8532 | 44.4281 – 53.2783 |
| reviewer_h_per_1k_arab | 18.5848 | 17.2064 – 19.9632 |
| reviewer_h_per_1k_filipino | 18.4534 | 15.6833 – 21.2235 |
| reviewer_h_per_1k_south_asian | 18.427 | 16.9599 – 19.8941 |
| reviewer_h_per_1k_western | 18.0764 | 16.8952 – 19.2576 |
| review_load_ratio_max_min_origin | 1.1393 | 1.0305 – 1.2482 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2473 | 0.1490% |
| cervical_false_closures | 0 | 912 | 0.4040% |
| wrong_patient_released | 0 | 3720 | 0.0990% |
| released_without_granted_consent | 0 | 2652 | 0.1390% |
| duplicate_or_lost_outputs | 0 | 93648 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 152539 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 120 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Limitations
- the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test
- synthetic cohorts, reviewers, capacity; no clinician labels
- 43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader
- cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately
- one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed
