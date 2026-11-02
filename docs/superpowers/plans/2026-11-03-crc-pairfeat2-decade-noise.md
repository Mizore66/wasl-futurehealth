# CRC pairfeat2 Decade-Noise Experiment Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Test whether the existing 30-feature scorer recovers more CRC closures when its training noise includes a generic one-digit DOB keystroke typo, which makes same-person decade-digit-only pairs learnable.

**Architecture:** Runtime, features, retrieval and safeguards are unchanged. A training-only noise variant `-noise v1t` (v1 plus a one-digit YYYYMMDD typo branch) is accepted only by `featdump`. A new opt-in modelset `pairfeat2` is fitted on fresh train/cal seeds and evaluated with the existing protected paired harness, generalized from the single name `pairfeat1` to an allowed set.

**Tech Stack:** Go 1.22+, Python 3 (numpy, scikit-learn, scipy, lightgbm 4.7.0), Bash.

**Spec:** `docs/superpowers/specs/2026-11-02-crc-pair-scorer-design.md` (binding for everything not changed here) plus the approved design below.

## Approved design (user-approved 3 Nov 2026)

- Motivation: the `pairfeat1` experiment (`pipeline/RESULTS.md`, feature-first section) failed its development gate (+0.5874pp < +2.0pp). Its v1 training noise contained zero same-person decade-digit-only DOB pairs, so feature 24 could only learn a different-person signal.
- Training noise `v1t`: identical to v1 except that, when a DOB is corrupted (probability 0.3, unchanged), a new first branch with probability **0.20** replaces exactly one of the eight YYYYMMDD digits, chosen uniformly, with a different digit chosen uniformly from the other nine. The remaining 0.80 of corruptions follow v1's existing branches with v1's existing draw sequence. The typo may yield a calendar-invalid date; existing missing-data handling applies.
- Not copied from v2: v2's ±10-year error is the held-out evaluation corruption model and must not be used for training.
- Seeds (all previously unused): train **1019 1021 1031 1033 1039** (5,000 persons each), cal **2011** (whole cal split), development dev/v2 **317 331 337**. Validation eval/v2 **401 409 419 421 431** (untouched by `pairfeat1`, which stopped at development).
- Gates, LightGBM parameters, calibrators, floor 0.01 in both arms, v7strict only, and stop-on-failure rules are exactly those of the `pairfeat1` plan. Default model, default noise, default floor 0.2 unchanged.

## Global Constraints

- All global constraints of `docs/superpowers/plans/2026-11-02-crc-pair-scorer.md` apply, with seeds replaced by the seeds above for `pairfeat2`.
- `gen` output for v1 and v2 must be byte-identical; `v1t` is refused by every command except `featdump` (configuration error, exit 2).
- `pairfeat1` harness behaviour, seeds and tests must keep working unchanged.
- Raw artifacts under `~/wasl-ml-data/agents2/hardening/pairfeat2/`. Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Python tests run by direct path (`python3 -I encoder/test_x.py`, `python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'`); Go tests from `pipeline/`.

---

### Task A: Training-only `v1t` DOB typo noise and `pairfeat2` policy guard (Go)

**Files:** Modify `pipeline/gen.go` (noisyDob), `pipeline/encdev.go` (runFeatDump DOB draw, validateModelPolicy), `pipeline/main.go` (v1t refusal outside featdump); test in `pipeline/noise_v1t_test.go`, extend `pipeline/encdev_test.go`.

**Interfaces produced:** `corruptDob1(dob string, r *rand.Rand) string` (v1's corruption body, extracted verbatim; `noisyDob` = p check then `corruptDob1`); `noisyDobV1T(dob string, r *rand.Rand, p float64) string`; `validateModelPolicy` rejects `pairfeat1` and `pairfeat2` with any policy other than `v7strict`.

- [ ] **Step 1: Failing tests.** (a) For a fixed seed and 20,000 draws on several DOBs, `noisyDob` output equals a test-local copy of the pre-change function body and leaves the RNG in the same state. (b) `noisyDobV1T` over 200,000 draws: uncorrupted share ≈ 0.70; among corrupted, typo share ≈ 0.20 (±0.01); every typo output differs from the input in exactly one YYYYMMDD digit position with a different digit, dashes in place; position distribution roughly uniform; decade-only (position 2) ≈ 1/8 of typos. (c) `validateModelPolicy("pairfeat2", p)` errors for `""`, `v6`, `v7ltt` and passes `v7strict`. (d) A helper `validateNoise(cmd, noise string) error` refuses `v1t` for every cmd except `featdump` and accepts `v1`/`v2` everywhere. (e) `runFeatDump` with NoiseModel `v1t` (40 train persons, seed 1019) is deterministic and produces at least one row with feature 24 == 1 and label 1 (if 40 persons is too few, use the smallest size that reliably does, and state it).
- [ ] **Step 2: Verify RED** (`cd pipeline && go test . -run 'V1T|Noise|PairFeat|FeatDump' -count=1`).
- [ ] **Step 3: Implement.** Extract `corruptDob1` with no change to draw order. `noisyDobV1T`: `if r.Float64() > p { return dob }`; `if r.Float64() < 0.20 { i := []int{0,1,2,3,5,6,8,9}[r.Intn(8)]; b := []byte(dob); b[i] = byte('0' + (int(b[i]-'0')+1+r.Intn(9))%10); return string(b) }`; `return corruptDob1(dob, r)`. In runFeatDump use `noisyDobV1T(p.Dob, r, .3)` when `NoiseModel == "v1t"`, keeping v1 and v2 branches. Name noise for v1t is v1's (existing `== "v2"` checks already route v1t to v1). Call `validateNoise(cmd, *noise)` right after flag parsing, before any command runs; exit 2 with `configuration error:`. Extend the modelset guard to `pairfeat2`.
- [ ] **Step 4: Verify GREEN**, full `go test . -count=1`, `go vet .`, and a gen byte-identity check for v1 and v2 on a small fixed cohort before/after (temporary or committed test; state which).
- [ ] **Step 5: Commit** only the listed Go files.

### Task B: Parameterize the training driver by modelset (Python)

**Files:** Modify `encoder/fit_pairfeat1.py`, `encoder/test_fit_pairfeat1.py`.

**Interface:** `python3 -I encoder/fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|pairfeat2]` (default `pairfeat1`, byte-identical behaviour and manifests). `PROTOCOLS = {"pairfeat1": {train seeds 1001 1003 1007 1009 1013, cal 2003, noise "v1"}, "pairfeat2": {train seeds 1019 1021 1031 1033 1039, cal 2011, noise "v1t"}}`; persons 5000, whole cal split, LightGBM settings unchanged for both. Output parent must be `~/wasl-ml-data/agents2/hardening/<modelset>/`. Chain files and fit suffix use the modelset name (`encoder_<ms>.json`, `lgb_stream_<ms>.json`, `iso_<ms>.json`, `iso_v7_<ms>.json`, `cal_p_<ms>.npy`). artifact-manifest keys: `git_head`, `training_manifest_sha256`, `csv_sha256`, `baseline_chain_sha256`, `<ms>_chain_sha256` (filenames as keys), `parity`; training manifest records `modelset` and `noise`. Refuse to overwrite any existing `models/*_<ms>.json`. Add `models/*_pairfeat2.json` to `.gitignore`.

- [ ] Steps: failing tests for both protocols (constants, paths, keys, CSV names `feat_train_<seed>.csv`, `feat_cal_<seed>.csv`, `feat_cal_<seed>_gbm.csv`), RED, implement, GREEN (`python3 -I encoder/test_fit_pairfeat1.py`, `python3 -I encoder/test_fit_scorer.py`), commit only these files plus `.gitignore`.

### Task C: Generalize the protected harness to an allowed candidate set (Python/Bash)

**Files:** Modify `scripts/pairfeat_experiment.sh`, `scripts/pairfeat_compare.py`, `scripts/test_pairfeat_compare.py`.

**Interface:** `ARM` ∈ `baseline | pairfeat1 | pairfeat2`. `CANDIDATES = {"pairfeat1": {"dev_seeds": [307, 311, 313]}, "pairfeat2": {"dev_seeds": [317, 331, 337]}}`; validation seeds `[401, 409, 419, 421, 431]` for both. An experiment parent serves exactly one candidate: `artifact-manifest.json` must carry `<candidate>_chain_sha256`, and the candidate is inferred from that single key (refuse if zero or more than one candidate key). Baseline lanes in that parent use the candidate's seed set. Funnel `config.modelset` must equal `""` or the candidate name. Analysis CLI unchanged: `MODE OUT_JSON baseline=ROOT <candidate>=ROOT`; selection writes `selected_modelset` = candidate or null. All gate numbers, write-once rules, frozen-validation checks and same-HEAD binding unchanged.

- [ ] Steps: failing tests that every existing pairfeat1 test still passes unmodified (or with only fixture renames), plus pairfeat2 tests for seeds, inferred candidate, refusal of mixed/absent candidate keys, guard argv (`-modelset pairfeat2` only on orchestrate/audit/report7), funnel modelset check; RED, implement, GREEN (`python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'`, `bash -n`), commit only these files.

### Task D: Run once and document

- [ ] After Tasks A–C pass review and a final cross-task review: full verification; `python3 -I encoder/fit_pairfeat1.py ~/wasl-ml-data/agents2/hardening/pairfeat2/run-1 --modelset pairfeat2`; inspect fit report and slices (decade_dob must now contain same-person rows; report if not and stop). Run paired dev-clean (baseline, pairfeat2) and gate; on pass, stress then freeze-validation then validation; stop at the first failed gate. Document aggregate results and every ruling in `pipeline/RESULTS.md` (new section) and a pointer in `docs/BACKEND-HANDOFF.md`; independent doc review before commit.
