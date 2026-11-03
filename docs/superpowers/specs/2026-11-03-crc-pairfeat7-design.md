# CRC pair scorer pairfeat7: hard negatives for missing and identical real birth dates — design

Approved in direction 3 Nov 2026 (user chose "document pairfeat6, then design pairfeat7"). Simulated data only.

## Why

pairfeat6 (experiment `pairfeat6/run-1`, HEAD `d042ba9`) failed its development gate against pairfeat5 on seeds 659 661 673: false-merge pairs 27 vs 25 and cross-person edges 19 vs 17, recall equal (626/678), safety 0. A read-only diagnosis of those development lanes (now used) compared the different-person linked pairs of the two arms:

- **Removed by pairfeat6: 7.** Mostly common-name pairs whose real birth years differ. The real exact-date and placeholder features did what they were built for.
- **Added by pairfeat6: 9.**
  - **4 with one birth date missing.** The names share only a common given name, or a given name plus a truncated or initial form. pairfeat5 scored them 0.12–0.47 (review band); pairfeat6 scored them 0.71–0.92 (merge). With the date missing, f30 and f31 are NaN and the rarity features decide; training has no negatives of this shape.
  - **4 with an identical real birth date and different names** (no shared skeleton token in two of them), including one 3-edge cluster in one seed. The real exact-date feature (f31) became too strong: in training, two records with the same real date are almost always the same person, because chance date collisions among 5,000 people are rare.
  - **1 with the same 1 January placeholder** and one shared token.
- Seed 673 accounts for +6 of the net +2; n = 3.

Missing dates are mildly label-correlated in training (positive rate 4.8% when f30 is NaN vs 2.25% otherwise on seed 1019; 6.0% vs 4.1% on calibration seed 2011), present in both, so this is not a training-only leak; the gap is the missing negatives.

## Change

New modelset `pairfeat7`. Everything about `pairfeat6` carries over (features f30–f33, leave-own-out rarity table, cross-script hard negatives at 0.10, the pairfeat5 settings and the cross-script review cap), plus two hard-negative types in training, each added for 10% of training persons and only for `pairfeat7`:

1. **Missing-date namesakes (`featdump -mdnamesakes 0.10`).** A different person who has the same sex, shares the base person's given name and no other skeleton token (and no family token that is a prefix or truncation of the other's), and whose registration has no birth date. The base person keeps their date. Same script as the base person in half the cases, opposite script in the other half.
2. **Same-real-date strangers (`featdump -rdnamesakes 0.10`).** A different person with the same sex and the same real, non-placeholder birth date as the base person, and no skeleton token in common. Same script as the base person in half the cases, opposite script in the other half.

Both are different persons in truth, both enter the candidate pairs like any other negative, neither enters the rarity table, and each has its own sidecar row counts and the existing 500-row floor. Truncated or initial-form names with a missing date are deliberately not generated as negatives: they are the truly ambiguous class, where real duplicates look the same, and should stay in review.

## Experiment

- Arms on the same cohorts, image reader off, `-r1review` off: `baseline`; `pairfeat5` (reference, refit at the experiment's HEAD, chain equal to `pairfeat5/run-2`'s); `pairfeat7` (candidate).
- Seeds, never used: development 719 727 733; validation 739 743 751 757 761. All earlier seeds stay untouched, including pairfeat6's development 659 661 673. pairfeat6's validation seeds 677 683 691 701 709 were never run and stay unused.
- Harness: the pairfeat6 three-arm machinery with candidate `pairfeat7`.

## Gates (pre-registered; stop at the first failure)

Unchanged from pairfeat6, against the pairfeat5 arm:

| | dev-clean (selection) | dev-stress | validation |
|---|---|---|---|
| `false_merge_pairs` | strictly below | not above | strictly below |
| `cross_person_edges` | strictly below | not above | strictly below |
| CRC recall gain | ≥ 0 | ≥ 0 | ≥ 0 |
| uncapped wrong-pair link-review pairs | ≤ max(1.10 × ref, ref + 1) | same | same |
| total reviewer hours | ≤ 1.10 × ref | same | same |

SAFETY zero in every lane of every arm, baseline included. Impure clusters, wrong final sign-offs and the vs-baseline gate are reported, not gating.

**Reported, not gating:** the pairfeat6 edge classes plus two new ones on development (`missing_dob_given_only`, `same_real_dob_different_names`); true merges for both arms; capped pairs; the unseen-token shares; the plain-calibration different-at-merge count beside pairfeat5's; slice reports for all three hard-negative types.

## Constraints

- No `/tmp`, no `/Users` paths, no identifiers, secrets, binaries or models in Git.
- Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. No push or merge.
- After the result is documented, thin the raw lanes of non-record runs.
- Disclose: the design comes from a post-hoc diagnosis of used development lanes (571 577 587 for pairfeat6, 659 661 673 for pairfeat7); all hard negatives are built to patterns of our own synthetic generator, so a gain may partly reflect learning the generator; this is the third scorer candidate in a row tuned on diagnosed failures, so the fresh validation seeds are the only unbiased evidence.
