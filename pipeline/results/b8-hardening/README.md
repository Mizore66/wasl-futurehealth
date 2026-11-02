# Fresh B8 wrong-EID evaluation

This is a **post-hoc, two-seed safety evaluation**, separate from the headline frozen evaluation. All four lanes ran at `075abc5ad04ae14dff01dc7b9ddd1cedd5671d51` on fresh seeds **17 and 29**, with independent v2 noise, held-out persons, cached readers, FHIR ingest, `v7strict`, chaos, and all seven programmes. LDCT and IFHAS used 300 and 800 loops per run. The optional EID-contradiction guard remains **off by default**.

The clean and wrong-EID cohorts are matched apart from `WASL_WRONGEID`: 0 versus 0.02. Each guard-off/guard-on pair uses the same seeds and evaluation settings. The 4% reviewer error is a synthetic stress-test assumption, not a measured clinician error rate.

## Closure safety and cost

| Cohort | Guard | CRC false closures | Cervical false closures | Lung false closures | CRC recall | Cervical recall | Review items / 100 loops | Reviewer h / 1k loops |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| Clean | Off | 0/1,178 | 0/523 | 0/802 | 0.7715 | 0.9090 | 31.651 | 10.999 |
| Clean | On | 0/1,176 | 0/524 | 0/801 | 0.7700 | 0.9110 | 32.735 | 11.333 |
| Wrong EID 2% | Off | **16/1,166** | **2/520** | **1/796** | 0.7535 | 0.9005 | 31.9205 | 10.9645 |
| Wrong EID 2% | On | 0/1,133 | 0/515 | 0/794 | 0.7420 | 0.8955 | 33.012 | 11.308 |

Under wrong-EID stress, enabling the guard removed all **19 observed false closures**. CRC recall fell by **1.15 percentage points**, cervical recall by **0.50 points**, and review load rose by **1.0915 items per 100 loops**. In the clean cohort, CRC recall fell by 0.15 points, cervical recall rose by 0.20 points, and review load rose by 1.084 items per 100 loops. Two seeds are insufficient to establish a small clean-cohort recall effect in either direction.

All four lanes had zero wrong-patient releases, releases without granted consent, duplicate/lost outputs, ingest loss/duplication, final-state mismatches, and forged events with effect. DR, breast, LDCT, IFHAS, hand-off, and lung-surveillance false-closure counters were zero in every lane. Each two-seed lane detected **34/34 tamper attacks** with zero false alarms on its two clean baselines.

## Residual linkage contamination

Pair precision/recall below are unweighted means over the two seeds; pair and cluster counts are pooled.

| Cohort | Guard | Pair precision | Pair recall | False-merge pairs | Impure clusters | Cross-person edges by basis |
|---|---|---:|---:|---:|---:|---|
| Clean | Off | 0.9915 | 0.8970 | 156 | 68 | `wasl`: 99 |
| Clean | On | 0.9915 | 0.8810 | 156 | 68 | `wasl`: 99 |
| Wrong EID 2% | Off | 0.9190 | 0.8690 | 1,623 | 568 | `eid`: 905; `wasl`: 99 |
| Wrong EID 2% | On | 0.9835 | 0.8535 | 298 | 122 | `eid`: 78; `human`: 1; `wasl`: 99 |

The guard does **not** make the linker perfectly accurate. Compatible or missing-demographic wrong EIDs, same-surname relatives, and model- or human-only contamination can remain. Residual cross-person edges and impure clusters must not be omitted when describing guard-on results. Guard-off false closures are preserved here as failures; they were not retuned away.

## Evidence and reproduction

Each lane contains its frozen manifest, aggregate `summary.json`/`SUMMARY.md`, and both per-seed reports, tamper results, subgroup analyses, and text summaries. [`comparison.json`](comparison.json) combines the headline values and all pooled safety counters. The committed files contain no local absolute paths.

From a clean checkout at the recorded revision, each lane can be reproduced with:

```bash
WASL_NOISES=v2 WASL_LDCT=300 WASL_IFHAS=800 \
WASL_WRONGEID=<0-or-0.02> WASL_EIDGUARD=<0-or-1> \
WASL_FINAL_DIR=<empty-results-dir> WASL_FINAL_WORK=<scratch-dir> \
scripts/final_eval.sh 17 29
```

The historical headline evaluation and scale runs are separate evidence sets. These two post-hoc seeds do not replace their five-seed aggregates, and no model, threshold, or calibrator was refit for this evaluation.
