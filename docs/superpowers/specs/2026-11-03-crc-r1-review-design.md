# CRC "completion not documented" goes to a clinician (r1review) — design

Approved 3 Nov 2026. Simulated data only.

## Why

On `pairfeat5` development lanes (seeds 521 541 547, already used, read-only), 66 of 719 true CRC closures were missed. Only 17 of them are on the pair scorer's side: 14 below the review floor and 3 never retrieved. 49 happened after the link succeeded. The baseline arm shows the same post-link groups, so none of them is a scorer effect.

- **A, 20 loops.** Both readers found no caecum statement. The common quote is "to the ileocecal valve" (20 loops); others are "passed up the ileum" (2) and "to the cecum" (1). Rule `R1_completion_not_documented` labelled the report `inadequate` with `Agree = true`. That label is final: no evidence review item was raised for any of the 20 loops, and the loop stays `inadequate` with escalation `rejected_inadequate`. The truth says the colonoscopy was complete. So the system orders a repeat automatically, which is a wrong verdict as well as a missed closure.
- **B and C, 29 loops.** The prep grade is ungraded and the loop is waiting on the endoscopy unit (`awaiting_unit` or `escalated`), or a repeat report is waiting while an older inadequate report holds the state. For 25 of the 29, the run ended less than 15 days after the true closure. The pipeline was correctly waiting for the unit's grade or its built-in day-15 clinician decision. This is mostly right-censoring at the run horizon, not a defect.

Reader v7 (`a3f9ccd`) deliberately stopped counting the ileocaecal valve alone as caecal intubation, after it produced false closures on corpus 6 (scope at the valve, caecum not reached). This design keeps that rule: the valve alone never closes a loop automatically.

## Change

- New flag `-r1review` (bool, default false), passed through the same `v7Args` plumbing as `-prepquery`.
- In `pipeline/evidence.go`, the two-reader combination: when `R1ReviewOn` is set, the branch that currently sets `lab = "inadequate"` with rule `R1_completion_not_documented` sets `lab = "needs_human"` instead. It keeps the rule name for tracing as `R1_completion_review`. Everything else is unchanged:
  - caecum `no` (stated failure to reach it, including the corpus 6 patterns) stays automatically `inadequate`;
  - no new pattern ever produces `adequate`.
- In `pipeline/closure.go`, a report whose evidence carries `R1_completion_review` raises one evidence review item with a new purpose `completion_undocumented`, mirroring `raiseFor(r, "evidence", PrepDay15Purpose)`. A signed answer goes through the existing human-override path (`f.EvOver`):
  - "adequate" closes, subject to the report's own tissue and histology fields, exactly as the day-15 prep decision does;
  - "inadequate" holds as today.

  Until the item is answered, the loop is held with `hold_decision` state `completion_review` (not `inadequate`), and no `rejected_inadequate` escalation is sent.
- Synthetic reviewer (`pipeline/governance.go`, `errFor`): `completion_undocumented` items use `ErrUngraded`, the assumed error of a decision made without documentation, as the day-15 prep decision does. This is 0 in clean lanes and 0.25 in stress lanes. The simulated reviewer answers from the report's true label, which assumes the clinician can resolve completion, for example from the endoscopist or the images. That assumption is disclosed.
- With `-r1review` off, every output is byte-identical to HEAD.

## Experiment

- Arms on the same cohorts:
  - `baseline` (default model);
  - `pairfeat5` (reference: pairfeat5 chain, cap on);
  - `r1review` (pairfeat5 chain, cap on, plus `-r1review`).

  No refit: `r1review` reuses the pairfeat5 chain files.
- Seeds:
  - fresh development seeds 557 563 569;
  - fresh validation seeds 601 607 613 617 619.

  Never used before. Development 307–547, validation 401–431 and B8/frozen 11 23 37 41 59 17 29 are not touched.
- Harness: a three-arm analysis with candidate `r1review` and reference arm `pairfeat5`. The write-once selection → stress → freeze → validation chain and same-HEAD binding stay as they are.

## Gates (pre-registered; stop at the first failure)

1. **`r1review` vs baseline.** Exactly the `pairfeat5` gates, using `wrong_pair_cap`, with recall at least +2.0pp for selection, at least 0 for stress and at least +1.0pp for validation.
2. **`r1review` vs `pairfeat5`.**
   - Recall gain at least +1.0pp for selection, at least 0 for stress and at least +0.5pp for validation.
   - CRC false closures 0 in every lane of every arm, stress included. Wrong releases and wrong final sign-offs 0.
   - `false_merge_pairs`, `impure_clusters`, `cross_person_edges` and different-person link-review pairs are not above the pairfeat5 arm's.
   - Total reviewer hours at most 1.10 × the pairfeat5 arm's.
3. **Reported, not gating.**
   - `completion_undocumented` items: count and answers (same and different person).
   - The 7 report corpora plus MTSamples re-scored with `-r1review`, which must still show 0 unsafe labels, because `needs_human` never closes.
   - Recall excluding loops whose true closure falls within 15 days of the lane's last decision.

## Constraints

- No `/tmp`, no `/Users` paths, no identifiers, secrets, binaries or models in Git.
- Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- No push or merge.
- After the result is documented, thin the raw lanes of non-record runs.
