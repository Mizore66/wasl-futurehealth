# Final frozen evaluation

Code revision `a66b6a254c3c` · policy `v7strict` · seeds [11, 23, 37, 41, 59] · source `fhir` · held-out persons only · frozen 2026-10-28T03:35:34.711234Z

Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.


## Noise v1: training-time corruptions (gen.go) (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.948 | 0.9396 – 0.9564 |
| crc_recall_no_review | 0.19 | 0.1704 – 0.2096 |
| cervical_precision | 0.999 | 0.9962 – 1.0018 |
| cervical_recall | 0.9706 | 0.9531 – 0.9881 |
| kpi_truth | 0.3312 | 0.3022 – 0.3602 |
| kpi_wasl | 0.3216 | 0.2952 – 0.348 |
| kpi_eid_only | 0.1238 | 0.1093 – 0.1383 |
| kpi_abs_error_wasl | 0.0096 | 0.0058 – 0.0134 |
| kpi_abs_error_eid_only | 0.2074 | 0.1857 – 0.2291 |
| linkage_pair_f1 | 0.9788 | 0.9737 – 0.9839 |
| reviewer_h_per_1k | 25.846 | 24.8163 – 26.8757 |
| review_items_per_100_loops | 74.495 | 71.5944 – 77.3956 |
| throughput_ev_s | 678.1907 | 612.986 – 743.3955 |
| review_items_per_100_arab | 76.416 | 71.5296 – 81.3024 |
| review_items_per_100_filipino | 66.2554 | 63.6499 – 68.8609 |
| review_items_per_100_south_asian | 74.6774 | 72.4497 – 76.9051 |
| review_items_per_100_western | 72.6122 | 68.7446 – 76.4798 |
| reviewer_h_per_1k_arab | 26.6114 | 24.937 – 28.2858 |
| reviewer_h_per_1k_filipino | 24.1558 | 21.9817 – 26.3299 |
| reviewer_h_per_1k_south_asian | 25.5874 | 24.8905 – 26.2843 |
| reviewer_h_per_1k_western | 24.6946 | 22.9336 – 26.4556 |
| review_load_ratio_max_min_origin | 1.1693 | 1.0885 – 1.2502 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2703 | 0.1360% |
| cervical_false_closures | 1 | 1008 | 0.5510% |
| wrong_patient_released | 0 | 5160 | 0.0710% |
| released_without_granted_consent | 0 | 3638 | 0.1010% |
| duplicate_or_lost_outputs | 0 | 99302 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 153646 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 114 | – |
| forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery) | 0 | – | – |
| forged_consent_target_genuinely_opted_out (RBAC denied the forgery) | 2 | – | – |
| tamper_detected | 85 | 85 | – |
| tamper_false_alarms | 0 | 5 | – |

## Noise v2: independent held-out corruptions (noise2.go), never used for training/calibration/rule design (5 seeds)

| Metric | Mean | 95% interval |
|---|---|---|
| crc_precision | 1 | 1.0 – 1.0 |
| crc_recall | 0.8782 | 0.8627 – 0.8937 |
| crc_recall_no_review | 0.1946 | 0.1789 – 0.2103 |
| cervical_precision | 1 | 1.0 – 1.0 |
| cervical_recall | 0.9114 | 0.9002 – 0.9226 |
| kpi_truth | 0.3266 | 0.3071 – 0.3461 |
| kpi_wasl | 0.2914 | 0.2745 – 0.3083 |
| kpi_eid_only | 0.1212 | 0.1107 – 0.1317 |
| kpi_abs_error_wasl | 0.0352 | 0.0268 – 0.0436 |
| kpi_abs_error_eid_only | 0.2054 | 0.1954 – 0.2154 |
| linkage_pair_f1 | 0.9422 | 0.9374 – 0.947 |
| reviewer_h_per_1k | 23.0036 | 22.1091 – 23.8981 |
| review_items_per_100_loops | 65.9048 | 63.4865 – 68.3231 |
| throughput_ev_s | 680.5455 | 631.042 – 730.049 |
| review_items_per_100_arab | 67.5824 | 64.8284 – 70.3364 |
| review_items_per_100_filipino | 62.866 | 56.9949 – 68.7371 |
| review_items_per_100_south_asian | 66.2978 | 62.2402 – 70.3554 |
| review_items_per_100_western | 61.2056 | 56.2482 – 66.163 |
| reviewer_h_per_1k_arab | 23.6188 | 22.7493 – 24.4883 |
| reviewer_h_per_1k_filipino | 21.5158 | 20.3057 – 22.7259 |
| reviewer_h_per_1k_south_asian | 23.0094 | 20.8524 – 25.1664 |
| reviewer_h_per_1k_western | 21.747 | 19.3729 – 24.1211 |
| review_load_ratio_max_min_origin | 1.156 | 1.0977 – 1.2144 |

| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |
|---|---|---|---|
| crc_false_closures | 0 | 2532 | 0.1460% |
| cervical_false_closures | 0 | 966 | 0.3810% |
| wrong_patient_released | 0 | 5136 | 0.0720% |
| released_without_granted_consent | 0 | 3541 | 0.1040% |
| duplicate_or_lost_outputs | 0 | 98126 | 0.0040% |
| ingest_lost_or_duplicated | 0 | 153692 | – |
| final_state_mismatch_loops | 0 | – | – |
| forged_with_effect | 0 | 111 | – |
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
