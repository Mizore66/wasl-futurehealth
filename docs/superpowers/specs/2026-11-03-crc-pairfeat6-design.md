# CRC pair scorer pairfeat6: placeholder dates, name rarity and cross-script hard negatives — design

Approved 3 Nov 2026. Simulated data only.

## Why

After pairfeat5 (validation `pairfeat5/r1review-run-2`), every remaining cross-person edge comes from the match model's own auto-merges (basis `wasl`; 73 on validation). A read-only diagnosis on the used development lanes 571 577 587 (pairfeat5 arm) found 18 different-person linked pairs, all scored between the 0.7 merge threshold and 0.98, none capped:

- **8 cross-script lookalikes.** An Arabic-script name against a clearly different Latin-script name, the same sex and the same birth date, often a 1 January placeholder. The cross-script review cap does not apply because it requires birth dates that are not exactly equal.
- **7 shared common token.** The two names share one very common token ("Mohammed", "Ahmed") and differ otherwise, with the same or a near birth date.
- **3 truly ambiguous.** A near-identical common name with the birth date missing on one side. No field separates these from the same person; review is the right outcome for them.

Two concrete gaps in the current features explain the first two groups:
- `dobFeats` flags only 1 July as a placeholder (feature 17). A shared 1 January date looks like an exact, real birth-date match. (`placeholderDob` in `v7.go` knows both, but only the v7 corroboration rule uses it.)
- No feature weighs how common a name token is. Sharing "Mohammed" counts as much as sharing a rare family name.

## Change

New modelset `pairfeat6`. Everything about `pairfeat5` carries over (training and calibration seeds 1019 1021 1031 1033 1039 / 2011, noise `v1t`, namesakes 0.10, family namesakes 0.10, `lambda_l2` 1.0, cross-script review cap 0.69), plus:

1. **Four pair features, appended after the existing 30 and computed only when the modelset is `pairfeat6`.** For every other modelset the feature vector, its length and every output are byte-identical to HEAD.
   - `f30` placeholder date: 1 if either birth date ends in `-01-01` or `-07-01` or has day `00`; 0 if both are real non-placeholder dates; NaN if either is missing or invalid.
   - `f31` real exact date: 1 if both dates are real, non-placeholder and equal; 0 if both are real and differ; NaN otherwise.
   - `f32` shared-token rarity: sum of the rarity weights of the name skeleton tokens present in both records.
   - `f33` rarest unmatched token: the largest rarity weight among skeleton tokens present on only one side; 0 if every token matches.
   - Rarity weight of token t: `log(N / (1 + c(t)))`, where c(t) is the number of training registrations whose skeleton contains t and N the total number of training registrations. An unseen token gets the weight of c = 0. The table is built once from the training split's registrations, written to `models/name_idf_pairfeat6.json`, and included in the pairfeat6 model-chain SHA256.
2. **Cross-script hard negatives in training (`featdump -xsnamesakes 0.10`, pairfeat6 only).** For 10% of training persons, add a different person who:
   - is in the opposite script;
   - has the same sex and the same placeholder birth date (1 January of the same year);
   - shares the given name, with every other name token different.
   Both registrations enter the candidate pairs like any other negative. The fit refuses to run if fewer than the existing namesake floor of such rows exist.
3. Fitting, calibration, Go/Python parity and the leaf check follow the pairfeat5 chain exactly (`encoder/fit_pairfeat1.py --modelset pairfeat6`).

## Experiment

- Arms on the same cohorts, image reader off, `-r1review` off:
  - `baseline` (default model);
  - `pairfeat5` (reference: its chain refit at the experiment's HEAD, which must equal `pairfeat5/run-2`'s);
  - `pairfeat6` (candidate).
- Seeds, never used:
  - development 659 661 673;
  - validation 677 683 691 701 709.

  The never-touch list (development 307–587 as used, validation 401–431, 601–619, 631 641 643 647 653, B8/frozen 11 23 37 41 59 17 29) is not touched.
- Harness: the three-arm analysis with reference arm `pairfeat5` (as for r1review), write-once selection → stress → freeze → validation, same-HEAD binding, guard on argv.

## Gates (pre-registered; stop at the first failure)

Against the pairfeat5 arm:

| | dev-clean (selection) | dev-stress | validation |
|---|---|---|---|
| `false_merge_pairs` | strictly below | not above | strictly below |
| `cross_person_edges` | strictly below | not above | strictly below |
| CRC recall gain | ≥ 0 | ≥ 0 | ≥ 0 |
| uncapped wrong-pair link-review pairs | ≤ max(1.10 × ref, ref + 1) | same | same |
| total reviewer hours | ≤ 1.10 × ref | same | same |

In every lane of every arm, including baseline: false closures, wrong releases, replay mismatches, analysis errors and trace errors are 0 (the `SAFETY` set). `impure_clusters` and wrong final sign-offs are reported, not gated. The pairfeat5 vs-baseline gate is reported, not gating.

**Reported, not gating:**
- On development, counts of the three diagnosed classes among remaining cross-person edges, for both arms.
- Same-person pairs merged (true pairs) for both arms, so any loss of true merges is visible.
- Capped pairs, capped same-person pairs and the rarity-table SHA256.

## Constraints

- No `/tmp`, no `/Users` paths, no identifiers, secrets, binaries or models in Git.
- Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- No push or merge.
- After the result is documented, thin the raw lanes of non-record runs.
- Disclose that the design comes from a post-hoc diagnosis of used development lanes, and that the hard negatives are built to the diagnosed pattern of our own synthetic generator, so a gain may partly reflect learning the generator rather than real naming.

## Amendment 1 (user-approved 3 Nov 2026, before any pairfeat6 lane ran): leave-own-out rarity counts

**Why.** The final branch review found a train/serve skew. The rarity table is counted on the same training registrations that become the training rows, so in training almost every token has a count of at least 1: only 0.7% of same-person and 0.8% of different-person training pairs (seed 1019) hit the unseen-token weight, against 7.1% and 13.4% on calibration seed 2011, which behaves like runtime. The model would learn rarity on easier data than it meets live.

**Change.**
- The rarity table also stores, for each person that contributed registrations, that person's own token counts and registration count (keyed by the person's index in the persons file; the table is a model file, never committed).
- When featdump builds a row for a pair of persons a and b (training and calibration rows alike), it uses leave-own-out counts: c′(t) = c(t) − own_a(t) − own_b(t) and N′ = N − regs_a − regs_b. A person absent from the table contributes 0.
- At runtime no cohort person is in the table, so the counts are used as they are; runtime behaviour is unchanged.
- The fit step reports the share of same-person and different-person pairs at the unseen-token weight for one training seed and for the calibration seed, so the remaining skew is visible.

Everything else in the spec is unchanged.
