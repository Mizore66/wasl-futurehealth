# CRC r1photo8: strict demographic guard under reliable-DOB assumptions — design

Approved 4 Nov 2026. Simulated data only. This experiment uses a new data-generating assumption; its numbers are not directly comparable with earlier experiments.

## Why

The user has chosen to treat a wrong real birth date in a true patient's registration as near-infinitesimal clinic lousiness rather than ordinary noise. Under that assumption, different real birth dates and different known sexes are hard contradictions. Missing demographics are different: they block automatic merging but may be resolved by review.

Earlier generators deliberately corrupt true patients' birth dates, so the old data cannot evaluate this rule. Both the generator and the linker must change together. The guard builds on the full combined stack: the pairfeat7-style scorer plus photo-gated completion review.

## Definitions

For a pair of registrations, under the new `-demoguard` flag:

- **Conflict:** both carry real, non-placeholder birth dates that differ, or both carry known sex values that differ.
- **Automatically mergeable demographics:** both carry the same real, non-placeholder birth date and the same known sex.
- **Needs review:** anything else, including a missing, invalid or placeholder birth date or unknown sex.

Known sex is `M` or `F`; `U` and missing values are unknown. Existing `realDob` and `placeholderDob` semantics apply. A source date that looks like a placeholder is therefore not sufficient for automatic merging.

## Generator: noise profile v3

Add `-noise v3` without changing v1, v1t or v2 behavior.

- Two registrations of the same person never carry different real birth dates.
- Existing same-person birth-date typo, month/day swap, day-of-month and year-shift corruption branches are removed for v3.
- The probability that a registration's birth date is blank is 4.5%, matching v2's existing 0.30 × 0.15 blank rate.
- No new synthetic placeholder birth dates are introduced. Blank and source-placeholder dates remain possible and count as missing for the guard.
- Sex remains unchanged except for the existing 2% unknown-sex rate.
- Name, script and other noise remain those of the corresponding command's existing training or evaluation profile. Only birth-date corruption semantics change.
- The rule applies wherever v3 generates registrations, including training/calibration dumps and evaluation cohorts, so train/serve DOB semantics do not diverge.
- A generator invariant test must find zero same-person pairs with distinct real birth dates across training, calibration and evaluation fixtures.

## Linker guard

`-demoguard` is default-off and forwarded consistently to orchestration, audit/replay and reporting.

1. **Conflict rejection.** A conflicting candidate pair is recorded as `rejected_demographic` with reason `dob_conflict` or `sex_conflict` and becomes a cannot-link. It does not become a wrong-pair review item.
2. **Automatic-merge requirement.** Every automatic merge, including an equal-Emirates-ID forced link, requires automatically mergeable demographics. A high-scoring pair needing review is demoted to the existing near/review path with reason `demographic_missing`; its score is not increased. Pairs already below the review floor do not create new review items.
3. **Human/agent boundary.** A reviewer may answer "same" for a needs-review pair. A reviewer cannot override a known DOB or sex conflict.
4. **Cluster invariant.** Before any merge, including equal-ID and reviewer merges, check all cross-cluster registration pairs. If any would conflict, reject with `demographic_cluster_conflict` and preserve the cannot-link. A record with missing demographics must never bridge two conflicting clusters.
5. Candidate generation, model scoring, calibration and non-demographic review rules are otherwise unchanged. With the flag off, behavior and traces are byte-identical.

Report counts by reason: DOB conflicts, sex conflicts, missing-data demotions, cluster-conflict rejections, equal-ID demotions/rejections and reviewer-approved missing-data merges.

## Fit and experiment arms

New fit modelset `pairfeat8`: the pairfeat7 recipe and 34 features, leave-own-out rarity table, hard-negative protocol and cross-script cap, but trained and calibrated with noise v3. Its artifacts use `_pairfeat8.json` names so earlier chains and manifests remain unchanged.

Three arms, all with `gen -imgphotos` and the cached image reader:

- `baseline`: default chain, no demographic guard.
- `r1photo8ref`: pairfeat8 chain plus `-r1review`, no demographic guard (reference-only arm).
- `r1photo8`: the same pairfeat8 chain plus `-r1review` and `-demoguard` (candidate).

Reference and candidate share one pairfeat8 fit at the experiment HEAD. `experiment.json` pins that fit, its chain hashes and the arm mapping. Earlier candidates and their gates remain byte-identical.

## Seeds

Never used in the harness or fit configurations:

- Development: 827 829 839.
- Validation: 853 857 859 863 877.

All earlier development, validation, training and calibration seeds retain their existing roles. Smoke uses seed 101 and is not evidence.

## Gates (pre-registered; stop at the first failure)

Against `r1photo8ref`:

| | dev-clean (selection) | dev-stress | validation |
|---|---|---|---|
| CRC recall gain | ≥ 0 | ≥ 0 | ≥ 0 |
| `false_merge_pairs` | strictly below | not above | strictly below |
| `cross_person_edges` | strictly below | not above | strictly below |
| uncapped wrong-pair link-review pairs | strictly below | not above | strictly below |
| total reviewer hours | ≤ 1.10 × ref | same | same |

SAFETY zero in every lane of every arm, baseline included: false closures, wrong releases, replay mismatches, analysis errors and trace errors. `impure_clusters`, wrong final sign-offs and the vs-baseline gate are reported, not gating.

**Reported, not gating:** true automatic and reviewer-assisted merges; demographic counts by reason; completion items and answers; edge classes; horizon-excluded recall; paired recall intervals; reviewer workload split into true-pair and wrong-pair items.

## Verification and documentation

- Unit tests cover every pair classification, equal-ID behavior, missing-data demotion, reviewer override rules and multi-record cluster bridging.
- Generator tests prove the no-distinct-real-DOB invariant and preserve v1/v1t/v2 behavior.
- Harness tests prove old-candidate identity and the combined reference/candidate arm mapping and gates.
- Run verification, fit, smoke, development, stress, frozen validation, read-only diagnosis, documentation and independent review. Stop at the first failed gate.
- Document the result plainly in `pipeline/RESULTS.md` and `docs/BACKEND-HANDOFF.md`; commit documentation separately and thin non-record raw artifacts afterward.

## Disclosures and boundaries

- Reliable real DOB is a user-chosen modeling assumption, not a locally measured error rate. Public sources establish that demographic errors occur but do not establish an Abu Dhabi DOB-error rate.
- The revised generator removes a difficulty present in earlier cohorts. Improvements under v3 must not be advertised as improvements on the old data distribution.
- Missing demographics may shift work from automatic merging to reviewers; the recall and workload gates are intended to expose that cost.
- Hard-negative donor bias, synthetic photos and reviewer true-label assumptions from earlier experiments remain applicable.
- Nothing is promoted to default. No real clinical effectiveness is claimed.
- The future evidence-gathering agent may work only on needs-review cases, must cite its evidence, cannot override hard conflicts and retains human sign-off. That agent is a separate milestone, not hidden inside this deterministic-rule experiment.

## Constraints

- No `/tmp`, no `/Users` paths, no patient-level identifiers, secrets, binaries or models in Git.
- Commit trailer `Co-Authored-By: Claude Code <noreply@anthropic.com>`. No push or merge.
- Path-only commits. Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`.
- After results are documented, thin raw lanes and feature CSVs of non-record runs; keep summaries and the run of record.
