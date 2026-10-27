# Encoder retrained on real name variants: results

**Status: built.** Retrieval tests are offline. The integrated runs are **simulated** cohorts with one seed. No real patient data is involved.

**Decision: the defaults are swapped.**
- `models/encoder_weights.json`, `lgb_stream.json`, `iso.json` and `iso_v7.json` are now the real-pair chain.
- The previous chain stays selectable as `-modelset v3` (`WASL_MODELSET=v3`): `models/{encoder_v3,lgb_stream_v3,iso_v3,iso_v7_v3}.json`.

How to reproduce: [`run_all.sh`](run_all.sh). Data and licences: [`README.md`](README.md).

## What changed

| | Previous encoder (v3) | New encoder (real_v2) |
|---|---|---|
| Architecture | 2-layer pre-LN transformer per script, DM 128, 4 heads, FF 256, OUT 64, L 40 | identical, so Go inference is unchanged |
| Size | 566,657 parameters (2.27 MB fp32) | the same |
| Training pairs | persons3 canonical pairs + the `gen.go` synthetic noise (the noise model the evaluation cohorts use) | **real** variants (below) + the same synthetic noise as augmentation only (p = 0.5 per side) |

**Training data for the new encoder** (166,002 items per epoch):
- 46,593 train-split persons, each with every real Arabic label (persons3 + Wikidata ar/arz) × every real Latin label or alias (persons3 en/als + Wikidata en, fr, de, es, it, nl, tr, id, ms). The mining covered all 64,394 QIDs and found 147k Latin and 99k Arabic labels; the train split contributes 103k Latin and 73k Arabic.
- 119,409 NETransliteration pairs. Before that, 6,154 pairs were removed because they matched a dev/cal/eval person, and 19,614 duplicates were removed.

**Objective.**
- Symmetric cross-script InfoNCE with a learnable temperature.
- Same-script InfoNCE, weight 0.5.
- False negatives masked: identical normalised strings across items.
- Hard negatives from epoch 3: half of the batches are built from neighbourhoods in the model's own cross-script kNN index, with same-surname neighbours placed first.
- 32 epochs, AdamW, lr 5e-4 with cosine decay, batch 512, trained from scratch.

**Checkpoint selection.**
- Selection score on dev: real R@1 + synthetic-v1 R@1 + ½ (same-script Latin R@1 + same-script Arabic R@1).
- Two seeds were trained. Seed 2, epoch 31 has the higher dev score (2.530 vs 2.518). Seed 1 is within 1 pp on every test metric (shown below).

**Export check.** Go `Embed` and PyTorch agree on the exported weights (min cos 0.99999988 on 55 names; normalisers identical on 5,055 names; `parity.py`).

## 1. Retrieval: before / after (held-out **eval** split, never used for training or selection)

Gallery: the canonical name of every eval person in the other script (≈8.8k distinct). Hits are counted on normalised strings, so homonyms count once.

| Test | Queries | R@1 old → new | R@10 old → new |
|---|---|---|---|
| **Real variants, all** | 11,209 | **0.548 → 0.660** | **0.727 → 0.809** |
| – Latin variant → Arabic canonical | 8,452 | 0.523 → 0.635 | 0.709 → 0.790 |
| – Arabic variant → Latin canonical | 2,757 | 0.624 → 0.737 | 0.781 → 0.868 |
| – same-surname subset (gallery holds a same-surname distractor) | 4,628 | 0.525 → 0.643 | 0.712 → 0.800 |
| Synthetic protocol, v1 noise (training-time model) | 8,871 × 2 directions | 0.773 → 0.860 | 0.900 → 0.938 |
| Synthetic protocol, v2 noise (independent held-out model) | 8,871 × 2 | 0.674 → 0.759 | 0.806 → 0.865 |
| Same-script, Latin ↔ Latin (two v1 registrations) | 8,871 | 0.882 → 0.918 | 0.941 → 0.959 |
| Same-script, Arabic ↔ Arabic | 8,871 | 0.974 → 0.984 | 0.998 → 0.999 |

- **Paired, person-clustered bootstrap** (5,335 persons, 2,000 resamples; `paired_ci.py`), real variants:
  - R@1 +11.2 pp, 95% CI [+10.4, +12.0];
  - R@10 +8.2 pp, 95% CI [+7.5, +8.9].
- **Seed 1** (not selected): real R@1/R@10 0.652/0.798; synthetic v1 0.856/0.936; synthetic v2 0.746/0.852.
- **Dev split** (used for selection), old → new: real 0.586 → 0.697 / 0.761 → 0.833; synthetic v1 0.805 → 0.877 (R@1).

The real-variant queries are hard. They include given/family reordering ("فطيم، لطفي"), extra middle names, Turkish and French spellings ("Hüssam Muhammed Luka"), initials ("DN Spergel") and kunyas. The old encoder had seen none of these sources.

## 2. Pair scorer and calibrators refit on the new cosines

LightGBM, `iso` and the stratified `iso_v7` were refit with `fit_scorer.py`:
- **Training rows:** `wsi featdump` rows, generated exactly like the runtime linker's candidates (exact top-10 cosine + rule-key blocking + `pairFeats`).
- **LightGBM:** five train-split cohorts of 5,000 persons each, about runtime size, so the raw-p scale that the 0.7 union threshold acts on matches the runtime.
- **Calibrators:** one cal-split cohort (3,588 persons).
- **iso_v7:** isotonic plateaus are replaced by their one-sided 95% Clopper-Pearson lower bound, and the uncorroborated stratum is capped at 0.989.
- **Go check:** Go `GBM.Predict` matches LightGBM (max |Δp| 5e-10 over 139,295 cal pairs).

Cal split, candidate pairs:

| Scorer chain | AP | P / R at p ≥ 0.7 | P / R at p ≥ 0.98 |
|---|---|---|---|
| v3 encoder + v3 scorer (previous defaults) | 0.9960 | 0.984 / 0.955 | 0.995 / 0.909 |
| v3 encoder + scorer refit with this pipeline (control) | 0.9969 | 0.991 / 0.952 | 0.999 / 0.864 |
| **new encoder + refit scorer** | **0.9978** | **0.991 / 0.964** | **0.998 / 0.901** |

## 3. Integrated runs (`scripts/demo.sh`, policy v7strict, seed 2026, simulated cohort)

| Run | CRC with review: P / R / false closures | CRC no review: R / false | Cervical with review: R / false | Wrong-patient msgs | Reviewer-h / 1k loops |
|---|---|---|---|---|---|
| Previous defaults, `all` | 1.000 / **0.961** / 0 of 571 | 0.232 / 0 of 138 | 0.991 / 0 of 217 | 0 | 20.6 |
| New chain, same cohort (`WASL_MODELSET=real_v2`, run → report) | 1.000 / **0.970** / 0 of 576 | 0.258 / 0 of 153 | 1.000 / 0 of 219 | 0 | 19.6 |
| New chain, `WASL_MODELSET=real_v2 scripts/demo.sh all` | 1.000 / **0.968** / 0 of 575 | 0.258 / 0 of 153 | 1.000 / 0 of 219 | 0 | 20.3 |
| **Swapped defaults, `scripts/demo.sh all` (no env)** | 1.000 / **0.970** / 0 of 576 | 0.258 / 0 of 153 | 1.000 / 0 of 219 | 0 | 19.8 |

All runs show:
- exactly-once with 0 duplicates and 0 final-state mismatches;
- all provenance chains verified;
- 0 of 24 forged events with effect;
- 0 messages released without granted consent.

`go test` passes with the swapped defaults.

**Swap rule:** CRC false closures stay 0 and CRC recall drops by no more than 1 pp vs 0.961. **Met.** Recall rose by 0.7–0.9 pp and false closures stayed 0.

A control run of the v3 encoder with the refit scorer, on the earlier pre-rebase cohort, gave CRC 0.966, 0 of 574. Part of the integrated gain therefore comes from the refit scorer. The encoder's own gain is the retrieval table above.

## Caveats

- **One seed.** The integrated runs use one cohort seed, and differences of under 1 pp are within run-to-run noise. The five-seed frozen evaluation (`scripts/final_eval.sh`, `pipeline/results/final/`) has **not** been re-run with the new chain. `pipeline/RESULTS.md` and the stage-7 numbers still describe the v3 chain.
- **Integrated cohorts are synthetic.** The noise comes from `gen.go`, so the integrated check shows no regression; it is not a measure of the real-variant gain. The real-variant gain is the retrieval result above.
- **Wikidata variants are not registrations.** They are a proxy for real-world spelling variation. They over-represent public figures and contain some non-registration forms (honorifics are stripped; kunyas and initials remain).
- **Residual NETransliteration overlap.** A pair is dropped only when its English or Arabic side exactly matches (after normalisation) a held-out person's name string. A held-out person can still appear under a third spelling on both sides.
- **Scorer refit differs from the original.** The original scorer and calibrator training code is not in this repo. Rows come from `featdump`, a re-implementation of the runtime candidate generation. The control run shows it reproduces the operating point (CRC 0.966, 0 false closures, with the v3 encoder).
- **Learn-then-Test selection is stale.** `pipeline/results/ltt/` (`WASL_POLICY=v7ltt`) was computed on the v3 chain and should be re-run before that policy is used with the new defaults.
