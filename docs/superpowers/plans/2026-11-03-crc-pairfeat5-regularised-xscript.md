# CRC pairfeat5 Regularised Scorer with Cross-Script Review Cap Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Test whether a leaf-regularised refit of the `pairfeat4` scorer, combined with a runtime cap that sends weak-evidence cross-script pairs to review instead of merging them, keeps the CRC recall gain without increasing linkage contamination.

**Architecture:** Training data, features and retrieval are those of `pairfeat4`. The fit adds LightGBM `lambda_l2 = 1.0`. At runtime, for modelset `pairfeat5` only, a model score on a cross-script pair whose Emirates ID is not valid on both sides and whose DOBs are not exactly equal is capped at 0.69, below the 0.7 merge threshold, so the pair is routed to review; such trace records carry `reason: "xscript_cap"`. Evaluation uses the protected paired harness with the `pairfeat4` wrong-pair review gate.

**Tech Stack:** Go 1.22+, Python 3 (numpy, scikit-learn, scipy, lightgbm 4.7.0), Bash.

**Spec:** `docs/superpowers/specs/2026-11-02-crc-pair-scorer-design.md` (binding for everything not changed here) plus the approved design below.

## Approved design (user-approved 3 Nov 2026, "approved, run it end to end")

- Motivation (post-hoc investigation of `pairfeat4`, development and training-side data): the near-certain scores came from unregularised Newton steps in low-hessian leaves (one leaf of +44 log-odds from 117 training rows with 1 positive scored at p = 4.5e-15; `pairfeat4` max |leaf| 3,337.6; baseline 7.8, `pairfeat2` 12.8, `pairfeat3` 13.2). An exploratory refit of the `pairfeat4` training CSVs with `lambda_l2 = 1` removed every leaf above 5 (max 3.6) and lowered plain-cal different-person rows at raw >= 0.7 from 26 to 21 (`lambda_l2 = 1` plus `min_sum_hessian_in_leaf = 1`: 23), with 0 rows at >= 0.9999. Across 12 baseline development lanes, 0 of 122 false model edges were true twins; 81 were cross-script, 106 had an Emirates ID on one side only and 16 on neither, 65 had a DOB missing on one side. On the regularised cal scores, cross-script pairs with the ID not valid on both sides and DOBs not exactly equal held 10 of 21 different-person rows at >= 0.7 and 535 of 5,876 same-person rows. The `lambda_l2` value and the cap rule were chosen from cal-split (training-side) data; disclosed.
- Model: modelset `pairfeat5`, v7strict only. Training protocol identical to `pairfeat4` (train seeds 1019 1021 1031 1033 1039 at 5,000 persons, cal 2011, noise v1t, given-name and family-name namesakes each 0.10, calibrators and parity on plain cal, diagnostic namesake cal dump) except LightGBM `lambda_l2 = 1.0`. All other LightGBM parameters unchanged.
- Cross-script review cap (runtime, `pairfeat5` only; off for every other modelset, including baseline): applies to a model candidate pair when (a) the two registrations differ in script (pair feature 20, Arabic-script equality, is 0), (b) the Emirates ID is not Luhn-valid on both registrations, and (c) the DOBs are not exactly equal (pair feature 13 is not 1; a missing DOB counts as not equal). Then the model score is capped at **0.69**, so the pair cannot merge on the model alone and, above the review floor, becomes a review candidate. A forced equal-EID link is unaffected. A pair whose score was lowered by the cap is traced with `reason: "xscript_cap"`.
- Stop checks before development: the `pairfeat4` namesake floors (>= 500 base-namesake rows of each kind), and the fitted model must have no leaf with |value| > 5 (else stop and report).
- Evaluation: fresh development dev/v2 seeds **521 541 547**; validation 401 409 419 421 431 untouched until development passes. Gates as for `pairfeat4`: recall >= +2.0pp selection / >= 0 stress / >= +1.0pp validation, safety zero both arms, no increase in false-merge pairs, impure clusters or cross-person edges, different-person distinct link-review pairs <= 1.10x baseline (<= 1 when baseline is 0); total-pair ratio and extra reviewer hours per extra true closure reported, not gating. Stop at the first failed gate. Reported, not gating: per lane, distinct capped pairs split into same-person and different-person.
- Default model, default noise, default floor 0.2 unchanged.

## Global Constraints

- All global constraints of the `pairfeat2`, `pairfeat3` and `pairfeat4` plans (`docs/superpowers/plans/2026-11-03-crc-pairfeat{2-decade-noise,3-namesake-negatives,4-symmetric-namesakes}.md`) apply, with seeds replaced by the seeds above.
- With any modelset other than `pairfeat5`, linker behaviour, traces and outputs must be byte-identical to HEAD `ae94e78` (the cap code path must not run, and no trace field changes). `fit_scorer.py` without `--lambda-l2` must produce byte-identical outputs.
- `pairfeat1`-`pairfeat4` driver and harness behaviour, seeds, gates, outputs and tests unchanged.
- Do not tune on development seeds 307 311 313 317 331 337 347 353 367 433 449 461, validation 401 409 419 421 431, or B8/frozen seeds 11 23 37 41 59 17 29.
- Raw artifacts under `~/wasl-ml-data/agents2/hardening/pairfeat5/`. No `/tmp`. No `/Users` paths, patient-level identifiers, secrets, binaries or models in Git. Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`. No push or merge.
- Python tests by direct path; Go tests from `pipeline/`. `PATH=/opt/anaconda3/bin:$PATH`.

---

### Task A: Cross-script review cap in the linker and `pairfeat5` guard (Go)

**Files:** Create `pipeline/xscript.go`, `pipeline/xscript_test.go`; modify `pipeline/linker.go` (merge decision), `pipeline/main.go` (activation), `pipeline/encdev.go` (validateModelPolicy).

**Interfaces produced:**
- `var XScriptCap bool` — set in `main.go` immediately after `ModelSet = *modelSet` passes `validateModelPolicy`: `XScriptCap = ModelSet == "pairfeat5"`.
- `const XScriptCapP = 0.69`.
- `func xscriptWeak(a, b *Rec, feat []float64) bool` — true when `len(feat) > 20 && feat[20] == 0 && !(a.eidOK && b.eidOK) && feat[13] != 1`.
- In the linker's per-candidate decision, after the existing Emirates ID handling (forced equal-EID link, contradiction clamp, `eidContradict` rejection) and before `if c.p >= L.Thr`: `capped := false; if XScriptCap && c.p > XScriptCapP && xscriptWeak(r, o, c.feat) { c.p = XScriptCapP; capped = true }`. A capped pair that lands in the near band emits its `near` trace with `Reason: "xscript_cap"`; one below the floor emits `below_review` with the same reason. `L.Stats["xscript_capped"]++` for each capped pair. No other change to the decision.
- `validateModelPolicy` rejects `pairfeat5` with any policy other than `v7strict`.

- [ ] **Step 1: Failing tests** (`pipeline/xscript_test.go`): (a) `xscriptWeak` truth table: cross-script with one valid EID and different DOBs → true; same-script → false; both EIDs valid → false; exact DOB (feat[13]==1) → false; missing DOB (feat[13]==0) → true; short feat → false. (b) A linker-level test using the existing test helpers for building records/candidates: a cross-script pair with model score 0.95, EID on one side only, DOBs differing — with `XScriptCap=false` it merges; with `XScriptCap=true` it does not merge, is a near candidate at p 0.69, and its trace record has stage `near` and reason `xscript_cap`; a same-script pair at 0.95 still merges with the cap on; an equal valid-EID pair still force-links. Restore `XScriptCap` with `t.Cleanup`. (c) `validateModelPolicy("pairfeat5", p)` errors for `""`, `v6`, `v7ltt`, passes `v7strict`. (d) `XScriptCap` defaults to false.
- [ ] **Step 2: RED** (`cd pipeline && go test . -run 'XScript|ModelPolicy' -count=1`).
- [ ] **Step 3: Implement.**
- [ ] **Step 4: GREEN**: `go test . -count=1`, `go vet .`. Byte-identity check: build `bin/wsi7` at BASE (save as `bin/wsi7.base`) and at HEAD, run the same small deterministic linker workload with the default modelset on both (state which command and cohort; e.g. `featdump` plus a short single-lane run used elsewhere in the tests), and show identical output hashes.
- [ ] **Step 5: Commit** only the listed Go files.

### Task B: `lambda_l2` in the fit and the `pairfeat5` protocol (Python)

**Files:** Modify `encoder/fit_scorer.py`, `encoder/test_fit_scorer.py`, `encoder/fit_pairfeat1.py`, `encoder/test_fit_pairfeat1.py`, `.gitignore`.

**Interfaces:**
- `fit_scorer.py` gains optional `--lambda-l2 X` (float >= 0). When given, the LightGBM params are `dict(PARAMS, lambda_l2=X)` and the fit report records `"lambda_l2": X`; when absent, params, outputs and report are byte-identical to today. Add `max_abs_leaf(dump) -> float` (max |leaf_value| over all trees) and record `"max_abs_leaf"` in the report only when `--lambda-l2` is given.
- `PROTOCOLS["pairfeat5"]` = the `pairfeat4` protocol plus `"lambda_l2": 1.0`; other protocols unchanged (no `lambda_l2` key needed; treat absent as none). The fit argv for `pairfeat5` carries `--lambda-l2 1.0`; others unchanged. `LEAF_LIMIT = 5.0`: after the fit, for `pairfeat5` only, read `lgb_stream_pairfeat5.json` and stop (exit 4, `leaf value check failed: M > 5.0`) before calibrator parity and manifests if `max_abs_leaf > 5.0`; record `max_abs_leaf` in the artifact manifest. CLI `--modelset ...|pairfeat5`; output parent `~/wasl-ml-data/agents2/hardening/pairfeat5/`; chain `*_pairfeat5.json`, `cal_p_pairfeat5.npy`; `.gitignore` gains `models/*_pairfeat5.json`.

- [ ] **Step 1: Failing tests:** fit_scorer arg parsing and param construction with and without `--lambda-l2` (factor `params_for(lambda_l2)`), `max_abs_leaf` on a small hand-built dump; driver constants, `pairfeat1`-`pairfeat4` argv unchanged, `pairfeat5` fit argv carries `--lambda-l2 1.0`, train/cal/ns-cal argv equal to `pairfeat4`'s, leaf check raises at 5.0001 and passes at 5.0, manifest keys, overwrite refusal.
- [ ] **Step 2: RED**, **Step 3: implement**, **Step 4: GREEN** (`python3 -I encoder/test_fit_scorer.py`, `python3 -I encoder/test_fit_pairfeat1.py`).
- [ ] **Step 5: Commit** only these files.

### Task C: `pairfeat5` candidate and capped-pair report in the harness (Python/Bash)

**Files:** Modify `scripts/pairfeat_experiment.sh`, `scripts/pairfeat_compare.py`, `scripts/test_pairfeat_compare.py`.

**Interface:** `CANDIDATES["pairfeat5"] = {"dev_seeds": [521, 541, 547], "review_gate": "wrong_pair", "report_xscript_cap": True}`; ARM gains `pairfeat5`. For a candidate with `report_xscript_cap`, each candidate-arm lane reports `xscript_cap = {"pairs": N, "same_person": S, "different_person": D}` from distinct pair keys of `link_funnel.ndjson` records with `reason == "xscript_cap"` (truth from that lane's `cohort/truth.json` `pk_qid`; an unmapped registration aborts analysis before any write-once file), aggregated per arm; not gating, not emitted for other candidates or the baseline arm.

- [ ] **Step 1: Failing tests:** existing tests pass (only candidate-set equalities and unknown-candidate sentinels may change, `pairfeat5` sentinels move to `pairfeat6`); `pairfeat5` seeds, inference, guard argv, wrong-pair gate applied; capped-pair counting from a fixture trace (duplicates, same/different person, unmapped → abort before writes); field absent for `pairfeat4` outputs.
- [ ] **Step 2: RED**, **Step 3: implement**, **Step 4: GREEN** (`python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'`, `bash -n scripts/pairfeat_experiment.sh`).
- [ ] **Step 5: Commit** only these files.

### Task D: Run once and document

- [ ] After A-C pass review and a final cross-task review: full verification; build; `python3 -I encoder/fit_pairfeat1.py ~/wasl-ml-data/agents2/hardening/pairfeat5/run-1 --modelset pairfeat5`.
- [ ] Stop if a floor or the leaf check fails. Report fit metrics, `max_abs_leaf`, slices, `plain_cal_diff_at_merge_0_7` beside 31/23/26 (information only).
- [ ] Paired dev-clean on 521 541 547 and gate; on pass, dev-stress, freeze-validation, validation-clean; stop at the first failed gate.
- [ ] Document in `pipeline/RESULTS.md` (new section after `pairfeat4`) and `docs/BACKEND-HANDOFF.md`, including the capped-pair report and, if the dev gate fails, the same read-only false-merge diagnosis; also add a short post-hoc paragraph to the `pairfeat4` section pointing to the leaf-regularisation finding. Independent doc review before commit; commit only those two files.
