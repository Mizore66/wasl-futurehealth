# CRC r1review: completion-not-documented goes to a clinician — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Test whether routing colonoscopy reports in which neither reader finds a caecum statement to a clinician review item, instead of an automatic "inadequate", raises CRC recall over `pairfeat5` with 0 false closures, including under stress.

**Architecture:** One opt-in closure-policy flag, `-r1review`. In the two-reader combination, the `R1_completion_not_documented` branch yields `needs_human`. Closure raises an evidence item with purpose `completion_undocumented`, which the synthetic reviewer answers at `ErrUngraded`. The harness gains a three-arm candidate `r1review` (baseline, pairfeat5 reference, candidate) that reuses the pairfeat5 chain.

**Tech Stack:** Go 1.22+, Python 3, Bash.

**Spec:** `docs/superpowers/specs/2026-11-03-crc-r1-review-design.md` (binding).

## Global Constraints

- With `-r1review` off, every output (decisions, traces, review queue, reports, funnel, harness JSON for `pairfeat1`-`pairfeat5`) is byte-identical to HEAD `db5ce5e`.
- `pairfeat1`-`pairfeat5` driver and harness behaviour, seeds, gates, outputs and tests are unchanged.
- `r1review` seeds:
  - development 557 563 569;
  - validation 601 607 613 617 619.
- Never touch development seeds 307 311 313 317 331 337 347 353 367 433 449 461 521 541 547, validation 401 409 419 421 431, or B8/frozen seeds 11 23 37 41 59 17 29.
- Raw artifacts under `~/wasl-ml-data/agents2/hardening/r1review/`. No `/tmp`. No `/Users` paths, patient-level identifiers, secrets, binaries or models in Git.
- Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`. No push or merge.
- Run Python tests by direct path or `python3 -I -m unittest discover -s scripts -p '<pattern>'`; run Go tests from `pipeline/`. `PATH=/opt/anaconda3/bin:$PATH`.

---

### Task A: `-r1review` closure policy (Go)

**Files:**
- Create `pipeline/r1review.go`, `pipeline/r1review_test.go`.
- Modify:
  - `pipeline/main.go`: flag and `v7Args` pass-through next to `-prepquery`;
  - `pipeline/evidence.go`: the R1 branch in the two-reader combination, around the `rules["R1_completion_not_documented"] = true` line;
  - `pipeline/closure.go`: the evidence loop, around `if label == "" { raise(r, "evidence"); continue }`, plus the hold-decision block;
  - `pipeline/prepquery.go`: `errFor`.

**Interfaces produced:**
- `var R1ReviewOn bool`, set from flag `-r1review` (default false). The flag is appended to `v7Args` as `fmt.Sprintf("-r1review=%v", R1ReviewOn)`, so every subprocess (linker, closure, audit, report7 replay) sees the same value.
- `const CompletionPurpose = "completion_undocumented"`.
- Evidence (`evidence.go`): in the branch where `c == "not_stated" && R["caecum"] == "not_stated" && G["caecum"] == "not_stated"`:
  - with `R1ReviewOn` on, keep `lab = "needs_human"` and set `rules["R1_completion_review"] = true`, and set a new `v.R1Review = true` on the verdict (new bool field, JSON `r1_review,omitempty`), so `len(outs)==1` yields `Agree=false, Label="disagree"` as other needs-human cases do;
  - with it off, the code path is unchanged.
- Closure (`closure.go`): where `label == ""` today calls `raise(r, "evidence")`, call `raiseFor(r, "evidence", CompletionPurpose)` instead when `R1ReviewOn && r.d.Ev != nil && r.d.Ev.R1Review`, then `continue`.
  - The signed answer arrives in `f.EvOver[r.d.ReportID]`, because `pilotKey(CompletionPurpose)` is `""`. It goes through the existing human-override path, including the `explicitInadequate` confirm.
  - Track `completionPending = true` and a `completionRow` (same shape as `holdRow`).
  - In the not-closed branch, when no other hold applies (`!(awaiting || inadequate)`) and `completionPending`, write the same `link_decision` / `evidence` / `hold_decision` chain with `"state": "completion_review"`. The loop `state` string itself is computed as today.
  - The review item's `Reason` is `CompletionPurpose`, as `raiseFor` sets it for `PrepDay15Purpose`. Check and mirror exactly how `PrepDay15Purpose` gets its `Reason`.
- Reviewer (`prepquery.go` `errFor`): `it.Reason == PrepDay15Purpose || it.Reason == CompletionPurpose` returns `ErrUngraded`.

- [ ] **Step 1: Failing tests** (`pipeline/r1review_test.go`):
  - (a) Evidence: a report whose only completion text is "advanced to the ileocecal valve", with prep ungraded and no tissue, gives `Label "inadequate"` and rule `R1_completion_not_documented` with the flag off. With it on, it gives `Agree false`, `R1Review true`, and rule text containing `R1_completion_review`.
  - (b) A report saying "could not be advanced past the ileocecal valve" stays `inadequate` with the flag on.
  - (c) A report with an explicit caecum statement is unchanged with the flag on.
  - (d) Closure with the flag on: the R1 report raises exactly one evidence item with `Reason == CompletionPurpose`, the decision is not `inadequate` and has no `rejected_inadequate` escalation, and the chain has a `hold_decision` with state `completion_review`. After a signed `adequate` (`f.EvOver[reportID] = "adequate"`) the loop closes, or waits for histology when tissue was taken. After a signed `inadequate` it is `inadequate`.
  - (e) `errFor` returns `ErrUngraded` for `CompletionPurpose`.
  - (f) With the flag off, closure decisions for the same inputs are identical to current behaviour.

  Use the existing evidence/closure test helpers; grep `prepquery_test.go` and `closure*_test.go` for how items and `EvOver` are built.
- [ ] **Step 2: RED** `cd pipeline && go test . -run 'R1Review' -count=1`.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: GREEN.** Run `go test . -count=1` and `go vet .`. Byte-identity with the flag off:
  - build `bin/wsi7` at BASE and HEAD;
  - run `gen` + `sim` for one small deterministic dev cohort (e.g. `-seed 101 -persons 400`), plus `readerscore -policy v7strict` on one corpus with its cache;
  - show identical output hashes.

  Artifacts go under `~/wasl-ml-data/agents2/hardening/r1review/taskA-identity/`.
- [ ] **Step 5: Commit** the listed Go files only.

### Task B: three-arm `r1review` candidate in the harness

**Files:** `scripts/pairfeat_compare.py`, `scripts/pairfeat_experiment.sh`, `scripts/test_pairfeat_compare.py`.

**Interfaces:**
- `CANDIDATES["r1review"]` keys:
  - `"dev_seeds": [557, 563, 569]` and `"validation_seeds": [601, 607, 613, 617, 619]`;
  - `"review_gate": "wrong_pair_cap"` and `"report_xscript_cap": True`;
  - `"chain": "pairfeat5"`: the candidate arm loads the `pairfeat5` chain files, runs lanes with `-modelset pairfeat5 -policy v7strict`, and adds `-r1review`;
  - `"reference": "pairfeat5"`;
  - `"extra_flags": ["-r1review"]`.

  `mode_settings` uses `CANDIDATES[c].get("validation_seeds", VALIDATION_SEEDS)` for validation-clean, so `pairfeat1`-`pairfeat5` keep 401-431. `arms("r1review")` returns `("baseline", "pairfeat5", "r1review")`; the other candidates still return two arms.
- Arm runner (`pairfeat_experiment.sh` / `lane_commands` / `guard`):
  - The `r1review` arm passes `-modelset pairfeat5` plus `-r1review` to orchestrate, audit and report7.
  - The `pairfeat5` arm inside an `r1review` experiment runs exactly as a pairfeat5 lane, with the same cap and the same flags.
  - `guard` allows `-r1review` only on the `r1review` arm and refuses it everywhere else.
  - The funnel modelset check for the `r1review` arm expects `pairfeat5`.
- Candidate identity: the experiment directory holds a write-once `experiment.json`, `{"candidate": "r1review", "reference": "pairfeat5"}`, created by a new subcommand `pairfeat_compare.py init-experiment <exp> r1review`.
  - It requires an existing `artifact-manifest.json` whose single chain key is `pairfeat5_chain_sha256`, and it refuses to overwrite.
  - Candidate inference returns `r1review` when `experiment.json` names it and its reference matches the manifest's chain. It returns today's inference otherwise. A missing or contradicting `experiment.json` with an `r1review` arm requested is an error.
- Analysis (`analyse`): for `r1review` all three roots (`baseline=`, `pairfeat5=`, `r1review=`) are required.
  - The baseline gate is the existing `gate(base, cand, kind)` with `wrong_pair_cap`. The `xscript_cap` loading applies to both non-baseline arms; the baseline-capped abort is unchanged.
  - New `reference_gate(ref, cand, kind)` reasons:
    - recall gain vs reference at least `REF_MIN_GAIN_PP = {"selection": 1.0, "stress": 0.0, "validation": 0.5}`;
    - safety (CRC false closures, wrong releases, wrong final sign-offs) zero in the reference arm too;
    - `false_merge_pairs`, `impure_clusters`, `cross_person_edges` and `reviewer.wrong_distinct_pairs` not above the reference's;
    - `reviewer.hours` at most `1.10 ×` the reference's.
  - Output JSON gains `"reference_gate"`; `pass` requires both gates. The write-once selection/stress/freeze/validation chain is unchanged, and `selected_modelset` is `"r1review"` on a pass.
  - Reported only: per arm, the counts of review items with reason `completion_undocumented` and their answers (`adequate` / `inadequate` / other) from `review_queue.json` + `reviewer_decisions.ndjson`.
- `pairfeat1`-`pairfeat5` outputs are byte-identical; existing tests keep passing. Only candidate-set equalities and sentinels may change: `r1review` is a real candidate now, and sentinels use `pairfeat6`.

- [ ] **Step 1: Failing tests** cover:
  - seeds and arms for `r1review`;
  - the guard (flag allowed only on the r1review arm);
  - `lane_commands` argv per arm;
  - `init-experiment` (requires a pairfeat5 manifest, write-once, refuses a mismatch);
  - inference;
  - three-root analysis (missing reference root → error);
  - `reference_gate`: pass, gain below minimum, false merges above reference, hours above 1.10×, safety nonzero in reference;
  - the combined pass;
  - completion item counting;
  - `pairfeat5` analysis JSON unchanged.
- [ ] **Step 2: RED**, **Step 3: implement**, **Step 4: GREEN** (`python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'`, `bash -n scripts/pairfeat_experiment.sh`).
- [ ] **Step 5: Commit** these files only.

### Task C: run once, corpus check, document

- [ ] After A and B pass their reviews and a final cross-task review, run full verification (all suites).
- [ ] Move `models/*_pairfeat5.json` to `~/wasl-ml-data/agents2/hardening/pairfeat5/run-2/models-copy/`. Then refit pairfeat5 at the current HEAD with `python3 -I encoder/fit_pairfeat1.py ~/wasl-ml-data/agents2/hardening/r1review/run-1 --modelset pairfeat5`. Expect chain hashes equal to `pairfeat5` run-2's; report any difference and stop if they differ. Then run `init-experiment` for r1review.
- [ ] Dev-clean with three arms on 557 563 569 → analyse.
  - On pass: dev-stress (three arms) → analyse → freeze-validation → validation-clean (three arms on 601-619) → analyse.
  - Stop at the first failed gate. A lane crash is set aside with the cause recorded and re-run once.
  - Check `df -h ~` before starting; at least 20 GB must be free.
- [ ] Reader corpus check, reported only: run `wsi readerscore -policy v7strict` with and without `-r1review` on every keyed corpus and both cached readers, as listed in `pipeline/RESULTS.md` (corpora 1-8 caches under `models/heldout*/`, and MTSamples). Report `unsafe`, `auto_unsafe_adequate_when_truth_not`, `auto_decided` and the count moved from automatic `inadequate` to review. `unsafe` must stay 0.
- [ ] Read-only diagnosis on the r1review dev lanes:
  - new completion items and their answers;
  - closures gained or lost vs pairfeat5;
  - the horizon-excluded recall (loops whose true closure is within 15 days of the lane's last decision removed).
- [ ] Document in `pipeline/RESULTS.md` (new section after pairfeat5) and `docs/BACKEND-HANDOFF.md`. Disclose:
  - the post-hoc origin (pairfeat5 dev diagnosis);
  - the reviewer-answer assumption;
  - `ErrUngraded`;
  - every rerun.

  Get an independent doc review, then commit those two files only.
- [ ] Thin the raw lanes of non-record runs; keep the record run whole.

---

## Amendment 2: photo-gated routing (spec Amendment 2)

### Task A2: photo gate (Go)

**Files:** `pipeline/r1review.go`, `pipeline/r1review_test.go`, and `pipeline/services.go` (the prep stage where `applyImage` is called).

- Add `func r1photoGate(v *TwoVerdict, iv *ImageVerdict)`. It is a no-op unless `R1ReviewOn && v != nil && v.R1Review`.
  - If `iv != nil && iv.Caecal == "verified"`: keep the completion review and add review hint `caecal_photo_verified`, so the item carries the photo context. Reuse the existing `ReviewHint` / `Flags` fields.
  - Otherwise: revert to exactly the flag-off verdict: `Agree = true`, `Label = "inadequate"`, `Rule = "R1_completion_not_documented"`, `R1Review = false`. Every other field must be as the flag-off judge would have produced.
- In `services.go`, call it for colonoscopy reports right after the optional `applyImage`. Pass `iv = ImageR.Read(ev.Images)` when the image reader is on (reuse the same verdict object, do not read twice) and `nil` when it is off. It is the only verdict path that feeds closure. Confirm replay/audit/report7 consume the stored verdicts and do not re-judge.
- Tests:
  - verified → review;
  - unverified, missing, and image off (nil) → exact flag-off verdict;
  - flag off → `r1photoGate` never changes anything;
  - prep-stage integration: a report with a verified caecal photo raises the `completion_undocumented` item; without the photo it stays auto-inadequate.

  Flag-off byte-identity as in Task A.

### Task B2: photo + image-reader lanes for r1review experiments (harness)

**Files:** `scripts/pairfeat_compare.py`, `scripts/pairfeat_experiment.sh`, `scripts/test_pairfeat_compare.py`.

- `CANDIDATES["r1review"]["lane_image"] = True`.
- In an r1review experiment, every arm (baseline, pairfeat5, r1review) runs:
  - `gen` with `-imgphotos`;
  - orchestrate, audit and report7 with `-image cached`, if they accept it; check how `-image` is consumed.
- `guard` requires `-image cached` and `-imgphotos` exactly in r1review experiments, and `-image off` with no `-imgphotos` in every other experiment. `build.json` records the image setting.
- `pairfeat1`-`pairfeat5` argv, outputs and pins are unchanged.
- Tests: argv per arm, guard refusals both ways, and the pins unchanged.

### Task C (amended)

- The run is as in Task C, with photos and the image reader on in all three arms.
- The reported-only diagnosis adds:
  - completion items with verified photos and their answers;
  - converted reports reverted for lack of a verified photo;
  - the image reader summary (`caecal_photo` counts).

## Amendment 3 (user-approved 3 Nov 2026): gate vs pairfeat5 only, fresh seeds

See spec Amendment 3. Task B3 (harness): for candidate `r1review`, the vs-baseline gate is computed and reported but does not affect `pass`; `pass` = reference gate (vs pairfeat5) pass. `CANDIDATES["r1review"]` and the runner's seed table use development 571 577 587 and validation 631 641 643 647 653; 557 563 569 and 601 607 613 617 619 become protected (refused). pairfeat1-pairfeat5 unchanged. Task C then reruns with a new experiment directory `pairfeat5/r1review-run-2`.
