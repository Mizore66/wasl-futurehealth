# CRC pairfeat4 Symmetric Namesake Experiment Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Test whether adding training-only family-name namesakes (shared family name, different given name) to the `pairfeat3` recipe removes its remaining false merges while keeping its CRC recall gain, judged with a pre-registered wrong-pair review-load gate.

**Architecture:** Runtime, features, retrieval and safeguards are unchanged. `featdump` gains a second training-only namesake kind, `-familynamesakes P`, beside the existing `-namesakes P` (given-name kind). A new opt-in modelset `pairfeat4` is fitted with the `pairfeat3` seeds, noise and given-name namesakes plus family-name namesakes, and evaluated with the protected paired harness on fresh development seeds. For `pairfeat4` only, the harness's review-load gate counts different-person link-review pairs instead of all link-review pairs.

**Tech Stack:** Go 1.22+, Python 3 (numpy, scikit-learn, scipy, lightgbm 4.7.0), Bash.

**Spec:** `docs/superpowers/specs/2026-11-02-crc-pair-scorer-design.md` (binding for everything not changed here) plus the approved design below.

## Approved design (user-approved 3 Nov 2026; implementation plan also approved)

- Motivation: `pairfeat3` (`pipeline/RESULTS.md`, namesake section) gained +5.87pp development recall but failed on contamination and workload. Its post-hoc diagnosis: of 14 new cross-person edges, 7 (read by eye) shared the family-name token with a different given or middle name, the shape of relatives; the `pairfeat3` namesakes covered only the opposite shape (shared given name, different family name). In `pairfeat2` and `pairfeat3`, 148 of 150 and 165 of 166 extra link-review pairs were same-person, so the total-pair workload gate penalises the recall gain itself.
- Family-name namesakes (training only, `featdump` only, flag `-familynamesakes`, rate **0.10**): for each sampled person, with probability 0.10, add one synthetic namesake whose names (Arabic, English, aliases), sex, country code, origin and Arabic source come from a **real** same-split person who shares the base person's **last** English token (case-insensitive), origin and sex and has a **different first** English token (and a different Qid); if none exists, none is added. DOB: the base DOB with one digit (0.60) or two distinct digits (0.40) of YYYYMMDD changed, built exactly as the given-name kind (digit count drawn once, up to 20 redraws, valid 1900-2025, differs from base). New Luhn-valid Emirates ID. Qid `"nf-" + base.Qid`. Registered like every person with its own seeded stream, independent of the given-name stream. `-familynamesakes 0` (default) leaves `featdump` output (CSV and sidecar) byte-identical. The 0.10 rate is an assumption.
- Given-name namesakes: unchanged from `pairfeat3` (`-namesakes 0.10`). Both kinds are independent; a person may get one, both or neither.
- Not addressed (disclosed): identical-DOB pairs with otherwise different names (3 of 14 in the `pairfeat3` diagnosis) and sex mismatches.
- Seeds: train **1019 1021 1031 1033 1039** (5,000 persons each, both kinds at 0.10), cal **2011**. Calibrators and parity on the plain cal dump (no namesakes). One diagnostic cal dump with both kinds at 0.10, scored with the fitted model, reports a slice per kind.
- Stop check before development: stop if the five train dumps hold fewer than **500** base-namesake rows of **either** kind. The plain-cal count of different-person rows at raw >= 0.7 is reported beside `pairfeat2`'s 31 and `pairfeat3`'s 23, for information only.
- Evaluation: fresh development dev/v2 seeds **433 449 461**; validation eval/v2 **401 409 419 421 431** untouched until development passes. Recall, safety and contamination gates, LightGBM parameters, calibrators, floor 0.01, v7strict only, write-once decisions, same-HEAD binding and stop-on-failure rules unchanged.
- Review-load gate (user choice, pre-registered, `pairfeat4` only): **different-person** distinct link-review pairs (pairs whose two registrations belong to different persons in the offline truth) must be at most **1.10x baseline**, or at most **1** when baseline is 0. Reported but not gating: the old total distinct link-review pair ratio, and extra reviewer hours per extra true closure (`(cand_hours - base_hours) / (cand_tp - base_tp)`, null when the tp difference is not positive). `pairfeat1`, `pairfeat2` and `pairfeat3` keep the total-pair gate; the change is not applied to them retroactively.

## Global Constraints

- All global constraints of `docs/superpowers/plans/2026-11-02-crc-pair-scorer.md`, `docs/superpowers/plans/2026-11-03-crc-pairfeat2-decade-noise.md` and `docs/superpowers/plans/2026-11-03-crc-pairfeat3-namesake-negatives.md` apply, with seeds replaced by the seeds above for `pairfeat4`.
- `-familynamesakes` (like `-namesakes`) must be 0 for every command except `featdump` and in [0, 1) for `featdump`, NaN rejected (configuration error, exit 2). `gen` output, and `featdump` output with `-familynamesakes 0`, must be byte-identical to HEAD `749d9f8` behaviour (including `-namesakes 0.1` runs).
- `pairfeat1`-`pairfeat3` harness behaviour, seeds, gates, outputs and tests must keep working unchanged.
- Do not tune on development seeds 307 311 313 317 331 337 347 353 367, validation 401 409 419 421 431, or B8/frozen seeds 11 23 37 41 59 17 29.
- Raw artifacts under `~/wasl-ml-data/agents2/hardening/pairfeat4/`. No `/tmp` for important artifacts. No `/Users` paths, patient-level identifiers, secrets, binaries or models in Git. Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`. No push or merge.
- Python tests by direct path (`python3 -I encoder/test_x.py`, `python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'`); Go tests from `pipeline/`. `PATH=/opt/anaconda3/bin:$PATH`.

---

### Task A: Family-name namesakes in `featdump` and `pairfeat4` policy guard (Go)

**Files:** Modify `pipeline/namesake.go`, `pipeline/namesake_test.go`, `pipeline/encdev.go` (runFeatDump, validateModelPolicy), `pipeline/main.go` (flag, validation).

**Interfaces produced:**
- `var FamilyNamesakeRate float64` (flag `-familynamesakes`, default 0, help `featdump only: share of persons given a synthetic family-name namesake hard negative (training only)`).
- `func validateNamesakeRate(flag, cmd string, rate float64) error` used for both flags: error if `!(rate >= 0 && rate < 1)` (rejects NaN) or `rate > 0 && cmd != "featdump"`; message names the flag. `validateNamesakes(cmd, rate)` stays as a wrapper for `-namesakes` so existing tests pass.
- `func familySeed(seed int64) int64 { return seed*2654435761 + 130363 }`.
- `func buildFamilyNamesakes(ps, pool []Person, rate float64, r *rand.Rand) ([]Person, []int)`: same contract and draw sequence shape as `buildNamesakes`, but indexed by last English token; eligible donor: `d.Qid != b.Qid && d.Origin == b.Origin && d.Sex == b.Sex && first(d) != first(b)`; Qid `"nf-" + b.Qid`. Implement by factoring `buildNamesakes` into `buildNamesakesBy(ps, pool, rate, r, byLast bool, prefix string)` with `buildNamesakes` and `buildFamilyNamesakes` as thin wrappers; the given-name path must keep its exact draw order (golden-checked).
- Sidecar `<out>.namesakes.json`: unchanged keys and bytes when `FamilyNamesakeRate == 0`. When `FamilyNamesakeRate > 0`, four more keys: `family_rate`, `family_namesakes_added`, `family_base_namesake_rows`, `family_rows` (0-based CSV row indices whose pair is a family namesake and its own base person, either orientation). The existing `rows` / `base_namesake_rows` keep meaning the given-name kind only.
- `validateModelPolicy` rejects `pairfeat4` with any policy other than `v7strict`.

- [ ] **Step 1: Record goldens at BASE** before editing: build `bin/wsi7`, run `featdump -split train -persons 400 -noise v1t -seed 1019 -namesakes 0.1` and `-split cal -noise v1t -seed 2011 -persons 400` into `~/wasl-ml-data/agents2/hardening/pairfeat4/golden/`; record SHA-256 of both CSVs and both sidecars as test constants. Save `bin/wsi7` as `bin/wsi7.base` for the gen check.
- [ ] **Step 2: Failing tests** (extend `pipeline/namesake_test.go`):
  (a) `validateNamesakeRate`: both flags accept `("featdump", 0)` and `("featdump", 0.1)`, `("gen", 0)`; reject `("gen", 0.1)`, `("featdump", -0.1)`, `("featdump", 1)`, `("featdump", math.NaN())`; message contains the flag name.
  (b) `buildFamilyNamesakes` on a hand-built pool (`Mohamed Ali` M arab, `Hassan Ali` M arab, `Mohamed Ali` M arab second Qid, `Sara Ali` F arab, `Hassan Ali` M south_asian, `Omar Khan` M arab) with rate 0.999999: the first person's namesake always takes names from the arab male `Hassan Ali`; never the same-first-token entry, the female or the other-origin one; a person with no eligible donor gets none; Qid prefix `nf-`.
  (c) rate 0 makes no draws from `r`.
  (d) `featdump` with `-familynamesakes 0` reproduces all four Step 1 goldens byte for byte.
  (e) `featdump -split train -persons 400 -noise v1t -seed 1019 -namesakes 0.1 -familynamesakes 0.1` is deterministic across two runs; sidecar has `family_namesakes_added > 0`, `family_base_namesake_rows >= 1`; a unit-tested helper `namesakePairRow(nsBase map[int]int, a, b int) bool` (hand-built map, both orientations, non-pairs) is what both row lists use; every listed row has label 0; `rows` and `family_rows` are disjoint.
  (f) `validateModelPolicy("pairfeat4", p)` errors for `""`, `v6`, `v7ltt`, passes `v7strict`; other modelsets unchanged.
- [ ] **Step 3: RED** (`cd pipeline && go test . -run 'Namesake|Family|PairFeat|FeatDump|ModelPolicy' -count=1`).
- [ ] **Step 4: Implement** per the interfaces. In `runFeatDump`, after the given-name block: if `FamilyNamesakeRate > 0`, `fr := rand.New(rand.NewSource(familySeed(seed)))`, `nf, fbase := buildFamilyNamesakes(ps, pool, FamilyNamesakeRate, fr)`, register `nf[i]` with pid `len(ps)+len(ns)+i` using `fr`, recording `nfBase`; the existing shuffle follows. Row bookkeeping uses `namesakePairRow` for both maps. In `main.go` register the flag and call `validateNamesakeRate("-familynamesakes", cmd, FamilyNamesakeRate)` beside the existing check; extend the modelset guard to `pairfeat4`.
- [ ] **Step 5: GREEN**: `go test . -count=1`, `go vet .`; `gen` v1 and v2 output on a small fixed cohort identical between `bin/wsi7.base` and the new binary (state commands and hashes).
- [ ] **Step 6: Commit** only the listed Go files.

### Task B: `pairfeat4` protocol in the training driver (Python)

**Files:** Modify `encoder/fit_pairfeat1.py`, `encoder/test_fit_pairfeat1.py`, `.gitignore`.

**Interfaces:**
- Consumes: `featdump -familynamesakes P` and the sidecar keys from Task A.
- Produces: `PROTOCOLS["pairfeat4"] = {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t", "namesakes": 0.10, "family_namesakes": 0.10}`; other protocols gain `"family_namesakes": 0.0`. CLI `--modelset pairfeat1|pairfeat2|pairfeat3|pairfeat4`. Output parent `~/wasl-ml-data/agents2/hardening/pairfeat4/`. Chain files `*_pairfeat4.json`, `cal_p_pairfeat4.npy`.

- [ ] **Step 1: Failing tests:** constants; `pairfeat1`-`pairfeat3` argv unchanged (no `-familynamesakes`); `pairfeat4` train argv carries `-namesakes 0.1 -familynamesakes 0.1`, plain cal and parity argv carry neither; diagnostic cal `feat_cal_2011_ns.csv` carries both plus `-gbmfile <chain>/lgb_stream_pairfeat4.json`; `check_namesake_floor` for `pairfeat4` raises `NamesakeFloorError` when either kind's total is below 500 (message names the kind: `namesake floor not met (family): N < 500`) and passes at 500/500; `family_namesake_slice(csv, sidecar)` mirrors `namesake_slice` on `family_rows`; both slice helpers raise `ValueError` on an out-of-range row, a listed row with label 1, or a non-finite p; training manifest records `family_namesakes`, per-seed `family_namesakes_added`, `family_base_namesake_rows`; artifact-manifest gains `family_namesake_slice` and the ns-cal sidecar sha256 (`ns_cal_sidecar_sha256`); refuse to overwrite `models/*_pairfeat4.json`.
- [ ] **Step 2: RED**, **Step 3: implement** (`pairfeat1`-`pairfeat3` code paths and outputs byte-identical; capture the base training manifest before the dump loop for `pairfeat4`), **Step 4: GREEN** (`python3 -I encoder/test_fit_pairfeat1.py`, `python3 -I encoder/test_fit_scorer.py`). Add `models/*_pairfeat4.json` to `.gitignore`.
- [ ] **Step 5: Commit** only these files.

### Task C: `pairfeat4` candidate and wrong-pair review gate in the harness (Python/Bash)

**Files:** Modify `scripts/pairfeat_experiment.sh`, `scripts/pairfeat_compare.py`, `scripts/test_pairfeat_compare.py`.

**Interface:** `ARM` gains `pairfeat4`; `CANDIDATES["pairfeat4"] = {"dev_seeds": [433, 449, 461], "review_gate": "wrong_pair"}`; existing candidates get `"review_gate": "total"`. Each lane's reviewer load gains `wrong_distinct_pairs` (distinct link-review pairs whose two registrations map to different persons via the lane's `cohort/truth.json` `pk_qid`; an unmapped registration is an analysis error) — computed and emitted **only** when the experiment candidate's gate is `wrong_pair`, so `pairfeat1`-`pairfeat3` outputs are unchanged. `gate()` for a `wrong_pair` candidate: fail if `cw > 1.10 * bw` or (`bw == 0` and `cw > 1`), reason `wrong-pair link-review pairs %d exceed %.2f (baseline %d +10%%)`; the gate dict also reports `distinct_pairs` (total, with the old ratio, not gating) and `extra_hours_per_extra_closure`. Selection, stress and validation all use the candidate's gate. Same-HEAD binding, write-once and frozen-validation rules unchanged.

- [ ] **Step 1: Failing tests:** all existing tests pass (only exact candidate-set equalities and unknown-candidate sentinels may change, keeping intent); `pairfeat4` seeds, inference, guard argv, funnel modelset; `wrong_distinct_pairs` from a fixture queue with a known truth map (same-person, different-person, duplicate pair, unmapped registration → error); gate pass at exactly 1.10x, fail just above, baseline 0 with candidate 1 passes and 2 fails; total-pair overage alone does not fail a `wrong_pair` candidate; `pairfeat3` gate behaviour unchanged on the same fixture; `extra_hours_per_extra_closure` null when tp does not rise.
- [ ] **Step 2: RED**, **Step 3: implement**, **Step 4: GREEN** (`python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'`, `bash -n scripts/pairfeat_experiment.sh`).
- [ ] **Step 5: Commit** only these files.

### Task D: Run once and document

- [ ] After Tasks A-C pass review and a final cross-task review: full verification; build `bin/wsi7`; `python3 -I encoder/fit_pairfeat1.py ~/wasl-ml-data/agents2/hardening/pairfeat4/run-1 --modelset pairfeat4`.
- [ ] If either namesake floor fails, stop and report. Otherwise report fit metrics, both namesake slices and `plain_cal_diff_at_merge_0_7` beside 31 and 23 (information only).
- [ ] Paired dev-clean (baseline, pairfeat4) on 433 449 461 and gate; on pass, dev-stress, then freeze-validation, then validation-clean; stop at the first failed gate.
- [ ] Document aggregate results and every ruling in `pipeline/RESULTS.md` (new section after `pairfeat3`) and a pointer in `docs/BACKEND-HANDOFF.md`, including the pre-registered gate change; if the dev gate fails, run the same read-only false-merge diagnosis as for `pairfeat3` and add it as a labelled post-hoc paragraph. Independent doc review before commit; commit only those two files.
