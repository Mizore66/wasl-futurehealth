# CRC combined candidate r1photo7: pairfeat7 scorer plus photo-gated completion review — design

Approved 4 Nov 2026 (user: full r1review recall margin). Simulated data only.

## Why

pairfeat7 and r1review each passed their own experiment against pairfeat5, but never together:
- pairfeat7 (`pairfeat7/run-1`, image off): false merges 77 → 41, cross-person edges 70 → 38, recall +0.44pp on validation.
- r1review (`pairfeat5/r1review-run-2`, image on): recall +1.64pp, false closures 0, linking identical to pairfeat5.

They touch different stages (the pair scorer vs evidence routing), so the gains should be roughly additive, but that has not been measured. This experiment measures the combination.

## Change

New experiment candidate `r1photo7`. No pipeline code changes: `-modelset pairfeat7` (features f30–f33, leave-own-out rarity table, cross-script cap) and `-r1review` (photo-gated completion review) are independent flags that already compose.

- **Arms**, all with synthetic photo attachment at `gen` (`-imgphotos`) and the cached image reader (`-image cached`, `-imgcache models/image_verdicts.json`), so the comparison inside the experiment is fair:
  - `baseline` (default model);
  - `pairfeat5` (reference: pairfeat5 chain refit at the experiment's HEAD, which must equal `pairfeat5/run-2`'s);
  - `r1photo7` (candidate: pairfeat7 chain + `-r1review`).
- The pairfeat7 chain is fitted at the same HEAD (`--modelset pairfeat7 --reference <pairfeat5 fit>`), reusing pairfeat7/run-1's protocol; its rarity table and model files are the pairfeat7-named ones.
- Numbers are not comparable with the image-off pairfeat5/pairfeat7 runs or with each other across experiments.

## Seeds

Never used: development 769 773 787; validation 797 809 811 821 823. All earlier seeds stay untouched (development 307–733 as used, validation 401–431, 601–619, 631–653, 677–709, 739 743 751 757 761, B8/frozen 11 23 37 41 59 17 29).

## Gates (pre-registered; stop at the first failure)

Against the pairfeat5 arm. Both contributions must hold:

| | dev-clean (selection) | dev-stress | validation |
|---|---|---|---|
| CRC recall gain (r1review's gate) | ≥ +1.0pp | ≥ 0 | ≥ +0.5pp |
| `false_merge_pairs` (pairfeat7's gate) | strictly below | not above | strictly below |
| `cross_person_edges` (pairfeat7's gate) | strictly below | not above | strictly below |
| uncapped wrong-pair link-review pairs | not above reference | same | same |
| total reviewer hours | ≤ 1.10 × ref | same | same |

SAFETY zero in every lane of every arm, baseline included: false closures, wrong releases, replay mismatches, analysis errors, trace errors. `impure_clusters` and wrong final sign-offs reported, not gated. The vs-baseline gate is reported, not gating.

**Reported, not gating:** `completion_undocumented` items and answers (with the closing-unsafe vs safe-direction split in stress); edge classes; true merges; horizon-excluded recall; the image-reader summary per arm; the plain-calibration different-at-merge count.

## Constraints

- No `/tmp`, no `/Users` paths, no identifiers, secrets, binaries or models in Git.
- Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. No push or merge.
- After the result is documented, thin the raw lanes of non-record runs.
- Disclose: the combination is expected to be additive but is measured here for the first time; simulated photos with assumed rates (85% / 10%) and a HyperKvasir-trained CNN needing local recalibration; reviewer answers come from the true label with `ErrUngraded` 0.25 in stress; pairfeat7's hard negatives carry the nearest-embedding-donor bias already disclosed; synthetic data; n = 3 dev / 5 validation seeds.
