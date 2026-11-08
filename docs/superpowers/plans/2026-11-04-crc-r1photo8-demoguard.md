# r1photo8 Strict Demographic Guard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a reliable-real-DOB noise profile and a strict DOB/sex guard to the combined CRC stack, then measure it against the same stack without the guard on fresh seeds.

**Architecture:** Noise v3 removes same-person real-DOB corruption while preserving missing-data rates and each command's name-noise profile. A default-off `-demoguard` enforces pair and cluster constraints in the linker. A shared `pairfeat8` fit serves a reference-only arm and the guarded candidate; the harness requires lower contamination and wrong-pair review counts without recall loss.

**Tech Stack:** Go `pipeline/`, Python `encoder/fit_pairfeat1.py` and `scripts/pairfeat_compare.py`, Bash `scripts/pairfeat_experiment.sh`.

**Spec:** `docs/superpowers/specs/2026-11-04-crc-r1photo8-demoguard-design.md` (binding).

## Global Constraints

- v1, v1t and v2 behavior and every earlier candidate's argv, gates, outputs and digests remain byte-identical.
- `-demoguard` defaults off. With it off, linker behavior and traces are unchanged.
- Development seeds: 827 829 839. Validation seeds: 853 857 859 863 877. Smoke seed 101 is not evidence.
- Reference `r1photo8ref` and candidate `r1photo8` share one pairfeat8 fit; there is no cross-noise pairfeat5 calibration comparison.
- SAFETY zero in every lane of every arm, baseline included.
- No `/tmp`, no `/Users` paths, no patient-level identifiers, secrets, binaries or models in Git.
- `PATH=/opt/anaconda3/bin:$PATH` for Python. Run Go tests from `pipeline/`; run Python harness tests with `python3 -I -m unittest discover -s scripts -p 'test_*.py'`.
- Commit trailer `Co-Authored-By: Claude Code <noreply@anthropic.com>`. Path-only commits. Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`. No push or merge.
- After results are documented, thin raw lanes and feature CSVs of non-record runs; keep summaries and the run of record.

## File Structure

- `pipeline/noise3.go`: v3 DOB corruption only.
- `pipeline/gen.go`, `pipeline/encdev.go`, `pipeline/genlung.go`, `pipeline/bench.go`: select v3 DOB noise while retaining command-appropriate name noise.
- `pipeline/demoguard.go`: pair classification and cluster-conflict checks.
- `pipeline/linker.go`, `pipeline/main.go`: enforcement and child-flag forwarding.
- `pipeline/pairfeat6.go`: pairfeat8's shared rarity-table mapping.
- `encoder/fit_pairfeat1.py`: pairfeat8 fit protocol and v3 source pinning.
- `scripts/pairfeat_compare.py`, `scripts/pairfeat_experiment.sh`: shared-chain three-arm experiment and gates.
- Tests live beside each component; experiment artifacts stay outside Git under `~/wasl-ml-data/agents2/hardening/`.

---

### Task 1: Noise profile v3

**Files:**
- Create: `pipeline/noise3.go`
- Modify: `pipeline/gen.go`, `pipeline/encdev.go`, `pipeline/genlung.go`, `pipeline/bench.go`, `pipeline/main.go`
- Test: `pipeline/noise3_test.go`, `pipeline/noise_v1t_test.go`

**Interfaces:**
- Produces: `noisyDob3(dob string, r *rand.Rand, p float64) string`.
- Preserves: existing `noisyDob`, `noisyDobV1T`, `noisyDob2`, name-noise functions and `NoiseModel` values.
- Consumed by: every registration DOB call site when `NoiseModel == "v3"`.

- [ ] **Step 1: Write failing v3 unit and invariant tests**

Create `pipeline/noise3_test.go` with these tests:

```go
func TestNoisyDob3OnlyBlankOrOriginal(t *testing.T) {
	const dob = "1980-02-22"
	for seed := int64(0); seed < 2000; seed++ {
		got := noisyDob3(dob, rand.New(rand.NewSource(seed)), .3)
		if got != dob && got != "" {
			t.Fatalf("seed %d: got %q", seed, got)
		}
	}
}

func TestNoisyDob3BlankRate(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	blank := 0
	for i := 0; i < 20000; i++ {
		if noisyDob3("1980-02-22", r, .3) == "" { blank++ }
	}
	if got := float64(blank) / 20000; got < .04 || got > .05 {
		t.Fatalf("blank rate %.4f outside [0.04, 0.05]", got)
	}
}
```

Add `TestNoise3FDRegisterInvariant`: set `NoiseModel = "v3"` with cleanup, construct a `Person` with a real DOB and valid name/EID fields, call `fdRegister` for seeds 0–199, and assert every returned registration's DOB is blank or exactly the source DOB. Repeat with `NoiseModel = "v2"` and assert the old generator produces at least one distinct real DOB, proving the test can detect the old behavior.

Add `TestNoise3GeneratedCohortInvariant`: generate a small evaluation cohort with `runGen`, read `truth.json` with the existing `readTruth` helper, walk each Patient resource in `fhir_*.ndjson`, map its provider/MRN identifier through `Truth.PKQid`, and assert no two nonblank real DOBs differ within one QID. Include at least one generated blank DOB in the fixture assertion so the test covers both branches.

Extend `TestValidateNoise` so v3 is accepted by `gen`, `featdump`, `nameidf`, `orchestrate` and `bench`, while v1t remains training-only. Add a v1/v1t/v2 golden test that records their outputs for fixed seeds before changing call sites and requires them to remain unchanged after implementation.

- [ ] **Step 2: Watch the new tests fail**

Run:

```bash
cd pipeline && go test . -run 'Noise3|ValidateNoise' -count=1
```

Expected: compile failure because `noisyDob3` is undefined; the v2 control and old-profile golden tests pass.

- [ ] **Step 3: Implement v3 DOB noise**

Create `pipeline/noise3.go`:

```go
package main

import "math/rand"

// noisyDob3 keeps a true patient's real DOB identical across registrations.
// Its blank rate matches v2: p=0.30 gives 0.30*0.15 = 4.5% missing.
func noisyDob3(dob string, r *rand.Rand, p float64) string {
	if r.Float64() < p*.15 {
		return ""
	}
	return dob
}
```

At every DOB call site listed below, select `noisyDob3` first when `NoiseModel == "v3"`, otherwise retain the existing branch order:

- `pipeline/gen.go` registration creation.
- `pipeline/encdev.go` `fdRegister` and evaluation pair generation.
- `pipeline/genlung.go` registration creation.
- `pipeline/bench.go` synthetic registration creation.

For evaluation name generation in `gen.go`, `genlung.go` and `encnoise`, treat v3 like v2 (`NoiseModel == "v2" || NoiseModel == "v3"`). For training/featdump name generation, leave the existing v2-only conditions unchanged so v3 uses the same v1/v1t name path as before. Do not otherwise alter random-draw order or name noise.

Update the `-noise` help text in `pipeline/main.go` to document `v3 (reliable real DOB; 4.5% blank)`.

- [ ] **Step 4: Verify v3 and all old noise tests**

Run:

```bash
cd pipeline && go test . -run 'Noise|Noisy|ValidateNoise' -count=1
cd pipeline && go test . -count=1
```

Expected: all pass; old-profile golden values are unchanged.

- [ ] **Step 5: Commit Task 1**

```bash
git add pipeline/noise3.go pipeline/noise3_test.go pipeline/noise_v1t_test.go pipeline/gen.go pipeline/encdev.go pipeline/genlung.go pipeline/bench.go pipeline/main.go
git commit -m "Add noise v3 with reliable real DOBs"
```

---

### Task 2: Strict linker demographic guard

**Files:**
- Create: `pipeline/demoguard.go`
- Modify: `pipeline/linker.go`, `pipeline/main.go`, `pipeline/encdev.go` comments as needed
- Test: `pipeline/demoguard_test.go`

**Interfaces:**
- Produces: `DemoGuard bool`, `demoClassify(a, b *Rec) demoDecision`, `(*LinkerCore).demoClash(ra, rb int) bool`.
- Decision values: `demoNeedsReview`, `demoAutoOK`, `demoDOBConflict`, `demoSexConflict`.
- Consumes: `realDob`, `placeholderDob`, existing `LinkerCore.Link`, candidate routing and reviewer `basis == "human"` path.

- [ ] **Step 1: Write failing classification tests**

Create a table test with exact records:

```go
func TestDemoClassify(t *testing.T) {
	cases := []struct {
		name, dobA, sexA, dobB, sexB string
		want demoDecision
	}{
		{"exact", "1980-02-22", "F", "1980-02-22", "F", demoAutoOK},
		{"dob conflict", "1980-02-22", "F", "1981-02-22", "F", demoDOBConflict},
		{"sex conflict", "1980-02-22", "F", "1980-02-22", "M", demoSexConflict},
		{"missing dob", "", "F", "1980-02-22", "F", demoNeedsReview},
		{"placeholder dob", "1980-01-01", "F", "1980-01-01", "F", demoNeedsReview},
		{"unknown sex", "1980-02-22", "U", "1980-02-22", "F", demoNeedsReview},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := &Rec{Dob: tc.dobA, Sex: tc.sexA}, &Rec{Dob: tc.dobB, Sex: tc.sexB}
			if got := demoClassify(a, b); got != tc.want { t.Fatalf("got %v want %v", got, tc.want) }
		})
	}
}
```

Add a both-conflict case and pin DOB-first precedence.

- [ ] **Step 2: Write failing routing, EID, reviewer and cluster tests**

Use `eidEvent`, `applySeq`, `sameCluster`, `pairsOnly`, `constGBM` and `scoredGBM` from the existing tests. Each test enables `DemoGuard` with cleanup and disables unrelated EID-contradiction guarding unless it is specifically under test.

Cover exactly:

1. Exact DOB/sex with a high model score merges.
2. Different real DOB emits `rejected_demographic/dob_conflict`, records a cannot-link, and does not merge or create a near item.
3. Different known sex behaves the same with `sex_conflict`.
4. Missing DOB with a high model score emits `near/demographic_missing`, does not merge, and retains the original score.
5. A below-floor missing-data pair remains below review unless it is an equal-EID pair that would otherwise force a link.
6. Equal valid EID + exact demographics force-links.
7. Equal valid EID + conflicting demographics cannot-links despite the ID.
8. Equal valid EID + missing demographics becomes a near item rather than force-linking.
9. `L.Link(a, b, 1, true, "human")` succeeds for a needs-review pair and increments `demoguard_human_missing_merge`.
10. Human `"same"` cannot override a direct known conflict.
11. Human-link A (real DOB) to B (missing DOB), then attempt to human-link B to C (different real DOB): reject with `demographic_cluster_conflict`; A, B and C must not form one cluster.
12. With `DemoGuard = false`, repeat conflict and missing-DOB cases and require the pre-change trace and cluster outcome.

- [ ] **Step 3: Watch Task 2 tests fail**

Run:

```bash
cd pipeline && go test . -run 'Demo' -count=1
```

Expected: compile failure because the guard types and functions do not exist.

- [ ] **Step 4: Implement pair classification and cluster checks**

Create `pipeline/demoguard.go`:

```go
package main

type demoDecision uint8

const (
	demoNeedsReview demoDecision = iota
	demoAutoOK
	demoDOBConflict
	demoSexConflict
)

var DemoGuard bool

func usableDemoDOB(d string) bool { return realDob(d) && !placeholderDob(d) }
func knownDemoSex(s string) bool { return s == "M" || s == "F" }

func demoClassify(a, b *Rec) demoDecision {
	if usableDemoDOB(a.Dob) && usableDemoDOB(b.Dob) && a.Dob != b.Dob {
		return demoDOBConflict
	}
	if knownDemoSex(a.Sex) && knownDemoSex(b.Sex) && a.Sex != b.Sex {
		return demoSexConflict
	}
	if usableDemoDOB(a.Dob) && usableDemoDOB(b.Dob) && a.Dob == b.Dob &&
		knownDemoSex(a.Sex) && knownDemoSex(b.Sex) && a.Sex == b.Sex {
		return demoAutoOK
	}
	return demoNeedsReview
}

func (L *LinkerCore) demoClash(ra, rb int) bool {
	for _, x := range L.members[ra] {
		for _, y := range L.members[rb] {
			d := demoClassify(L.recs[x], L.recs[y])
			if d == demoDOBConflict || d == demoSexConflict { return true }
		}
	}
	return false
}
```

- [ ] **Step 5: Enforce the guard in candidate routing and `Link`**

In the candidate loop, after existing different-EID conflict handling and before equal-EID force-linking:

- Classify only when `DemoGuard` is on.
- On conflict, call `cannot2`, increment `demoguard_dob_conflict` or `demoguard_sex_conflict`, emit `rejected_demographic` with the matching reason, and continue.
- Track `equalEID` and `demoNeedsReview`. An equal-EID pair that would have forced a link but needs review must enter the near path even when its model score is below `Lo`; do not change its score.
- In the normal decision, automatic `Link` is allowed only for `demoAutoOK`. A high-scoring needs-review pair emits/stores `near` with reason `demographic_missing`; increment `demoguard_missing` and `demoguard_eid_missing` when applicable. Below-floor ordinary pairs remain below review.

In `LinkerCore.Link`, enforce the invariant independently of candidate routing:

```go
if DemoGuard {
	if basis != "human" && demoClassify(L.recs[a], L.recs[b]) != demoAutoOK {
		return false
	}
	if L.demoClash(ra, rb) {
		L.cannot2(a, b)
		L.Stats["demoguard_cluster_conflict"]++
		return false
	}
}
```

Preserve existing EID and `mergeAllowed` checks. Human merges bypass only the `demoAutoOK` requirement, never `demoClash` or an existing cannot-link. On a successful human merge of a needs-review pair, increment `demoguard_human_missing_merge`. Extend the deferred rejection trace so a guard rejection reports `demographic_cluster_conflict`; leave its old `merge_constraint` fallback unchanged for other failures.

Register `-demoguard` in `pipeline/main.go`, assign `DemoGuard`, and append `-demoguard` to child `v7Args` only when on, following the `-eidguard` pattern. Do not add anything to default child argv.

- [ ] **Step 6: Verify guard and full Go suite**

Run:

```bash
cd pipeline && go test . -run 'Demo|EID|LinkFloor|XScript' -count=1
cd pipeline && go vet .
cd pipeline && go test . -count=1
```

Expected: all pass, including every guard-off parity test.

- [ ] **Step 7: Commit Task 2**

```bash
git add pipeline/demoguard.go pipeline/demoguard_test.go pipeline/linker.go pipeline/main.go pipeline/encdev.go
git commit -m "Add strict DOB and sex demographic guard"
```

---

### Task 3: pairfeat8 runtime and fit protocol

**Files:**
- Modify: `pipeline/pairfeat6.go`, `encoder/fit_pairfeat1.py`, `.gitignore`
- Test: `pipeline/pairfeat8_test.go`, `encoder/test_fit_pairfeat1.py`

**Interfaces:**
- Consumes: v3 from Task 1 and pairfeat7's 34-feature/rarity/hard-negative recipe.
- Produces: model files `encoder_pairfeat8.json`, `lgb_stream_pairfeat8.json`, `iso_pairfeat8.json`, `iso_v7_pairfeat8.json`, `name_idf_pairfeat8.json`; manifest key `pairfeat8_chain_sha256`.
- `read_reference("pairfeat8", None)` returns `None`; no cross-noise reference is allowed.

- [ ] **Step 1: Write failing runtime and fit tests**

In `pipeline/pairfeat8_test.go`, assert:

```go
func TestPairfeat8RuntimeMapping(t *testing.T) {
	if !usesNameIDF("pairfeat8") || !xscriptCapFor("pairfeat8") {
		t.Fatal("pairfeat8 must inherit rarity features and the cross-script cap")
	}
	if err := validateModelPolicy("pairfeat8", "v7strict"); err != nil { t.Fatal(err) }
	if err := validateModelPolicy("pairfeat8", "v7ltt"); err == nil { t.Fatal("accepted non-strict policy") }
}
```

In `encoder/test_fit_pairfeat1.py`, add a `Pairfeat8` class asserting:

- `PROTOCOLS["pairfeat8"]` equals pairfeat7's protocol except `noise == "v3"` and no `reference` key.
- Training/calibration seeds, 34 features, all namesake rates and L2 are unchanged.
- Every generated featdump/nameidf argv contains `-noise v3`; artifact names end in `_pairfeat8.json`.
- `read_reference("pairfeat8", None) is None`.
- `chain_files("pairfeat8")` contains exactly the five files above.
- The v3 manifest source list includes `pipeline/noise3.go`; pairfeat1–7 source lists do not.
- `.gitignore` contains `models/*_pairfeat8.json`.
- Existing fit golden tables remain unchanged.

- [ ] **Step 2: Watch the tests fail**

Run:

```bash
cd pipeline && go test . -run Pairfeat8 -count=1
PATH=/opt/anaconda3/bin:$PATH python3 -I encoder/test_fit_pairfeat1.py
```

Expected: Go mapping assertion fails; Python reports missing pairfeat8 protocol/artifact behavior.

- [ ] **Step 3: Implement pairfeat8**

Add pairfeat8 to `usesNameIDF` in `pipeline/pairfeat6.go`; `xscriptCapFor` then inherits the cap automatically. Update comments that enumerate pairfeat6/7.

In `encoder/fit_pairfeat1.py`:

```python
"pairfeat8": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v3",
              "namesakes": 0.10, "family_namesakes": 0.10, "lambda_l2": 1.0,
              "xs_namesakes": 0.10, "nfeat": 34, "md_namesakes": 0.10, "rd_namesakes": 0.10},
```

Only `noise` and omission of pairfeat7's `reference` key are intentional. Add pairfeat8 to usage text and modelset choices. Add a v3-only code-file list containing pairfeat7's code files plus `pipeline/noise3.go`; do not change earlier lists. Add `models/*_pairfeat8.json` to `.gitignore`.

- [ ] **Step 4: Verify fit and runtime suites**

Run:

```bash
cd pipeline && go test . -run 'Pairfeat|NameIDF|XScript' -count=1
PATH=/opt/anaconda3/bin:$PATH python3 -I encoder/test_fit_pairfeat1.py
PATH=/opt/anaconda3/bin:$PATH python3 -I encoder/test_fit_scorer.py
```

Expected: all pass and old fit goldens are unchanged.

- [ ] **Step 5: Commit Task 3**

```bash
git add pipeline/pairfeat6.go pipeline/pairfeat8_test.go encoder/fit_pairfeat1.py encoder/test_fit_pairfeat1.py .gitignore
git commit -m "Add pairfeat8 fit protocol under noise v3"
```

---

### Task 4: Shared-chain three-arm harness and gates

**Files:**
- Modify: `scripts/pairfeat_compare.py`, `scripts/pairfeat_experiment.sh`, `scripts/test_pairfeat_compare.py`

**Interfaces:**
- Reference-only arm: `r1photo8ref`, chain pairfeat8, flags `["-r1review"]`.
- Candidate: `r1photo8`, chain pairfeat8, flags `["-r1review", "-demoguard"]`.
- Shared experiment identity: `reference_chain == "shared"`; one fit manifest and one chain map serve both non-baseline arms.
- `lane_noise(candidate)` defaults to `LANE["noise"]` and returns `"v3"` only for these two new arms.
- `_r1photo8_gate(ref, cand, kind, base)` implements the spec table.

- [ ] **Step 1: Pin old behavior and write failing new tests**

Before editing the harness, generate characterization digests for pairfeat7 and r1photo7 using the existing table-building helpers and add them as golden tests. They must pass before the new implementation and remain unchanged afterward.

Add tests for:

```python
R8REF = {"chain": "pairfeat8", "reference_only": True, "extra_flags": ["-r1review"], "lane_image": True, "noise": "v3"}
R8 = {"chain": "pairfeat8", "reference": "r1photo8ref", "reference_chain": "shared",
      "extra_flags": ["-r1review", "-demoguard"], "lane_image": True, "noise": "v3"}
```

Cover:

1. `chain_of` and `extra_flags` for both arms; reference-only is refused by `init-experiment` as the candidate.
2. `init-experiment EXP r1photo8` writes candidate, reference, chain, chain hashes, arm mapping and fit-manifest SHA without a second fit directory.
3. Reference and candidate lanes use identical pairfeat8 chain hashes and `-noise v3` at gen.
4. Candidate orchestrate/audit/report7 argv ends with `-r1review -demoguard`; reference ends with `-r1review`; baseline has neither.
5. The guard requires exactly one candidate `-demoguard`, zero reference/baseline occurrences, and rejects a v2 gen lane.
6. `build.json` records each arm's own flags and v3 only for the new experiment.
7. Completion review aggregation keys off `extra_flags(arm)`, so both new arms report completion items.
8. Shared-chain `resolve_candidate`, `governing_view`, validation freeze and same-HEAD checks work; a changed chain hash is refused.
9. All old golden digests and seed-collision checks pass; reference-only seed duplication does not count as a second experiment use.

- [ ] **Step 2: Write failing gate tests**

Construct three-arm summaries and assert:

- Selection and validation pass only when recall gain ≥0, false merges, cross-person edges and uncapped wrong-pair reviews are strictly below reference, wrong-pair count is lower, hours ≤1.10×, and every SAFETY count is zero.
- Stress uses not-above rather than strictly-below for those three contamination/workload counts.
- Each metric fails independently when its rule is violated, including recall −0.01pp, wrong pairs equal to reference in selection/validation, and any baseline SAFETY count.
- `impure_clusters`, wrong final sign-offs and the baseline comparison do not decide pass.
- Demographic reason counts are reported but not gated.

- [ ] **Step 3: Watch the harness tests fail**

Run:

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest discover -s scripts -p 'test_*.py'
```

Expected: new r1photo8 tests fail; all old characterization tests pass.

- [ ] **Step 4: Implement candidate configuration and shared identity**

Add to `CANDIDATES`:

```python
"r1photo8ref": {"dev_seeds": [827, 829, 839], "validation_seeds": [853, 857, 859, 863, 877],
                "review_gate": "wrong_pair_cap", "report_xscript_cap": True,
                "chain": "pairfeat8", "reference_only": True, "extra_flags": ["-r1review"],
                "lane_image": True, "noise": "v3", "report_linked_pairs": True, "edge_classes": "pairfeat7"},
"r1photo8": {"dev_seeds": [827, 829, 839], "validation_seeds": [853, 857, 859, 863, 877],
             "review_gate": "wrong_pair_cap", "report_xscript_cap": True,
             "chain": "pairfeat8", "reference": "r1photo8ref", "reference_chain": "shared",
             "reference_gate": "r1photo8", "extra_flags": ["-r1review", "-demoguard"],
             "lane_image": True, "noise": "v3", "baseline_gating": False,
             "report_linked_pairs": True, "edge_classes": "pairfeat7"},
```

Add the five-file pairfeat8 `CHAIN_FILES` entry. Derive `CHAIN_CANDIDATES` from all `chain_of` values so manifest inference recognizes pairfeat8 without creating a standalone experiment candidate. Add `reference_only(arm)` and filter reference-only entries out of candidate-selection/enumeration checks and cross-experiment seed-collision checks.

Implement the shared path in `init_experiment`, `read_experiment` and `resolve_candidate`: pin `artifact_manifest_sha256`, `pairfeat8_chain_sha256`, `chain = "pairfeat8"` and the arm mapping; validate the map against the fit manifest. Keep old experiment key sets and branches unchanged.

- [ ] **Step 5: Implement lane noise, metrics and runner support**

Add `lane_noise(candidate)` and use it in `lane_commands`, `guard`, expected settings and build checks. Add v3 to build records only for the new arms. Change completion aggregation to `"-r1review" in extra_flags(arm)` and allow completion items for any arm carrying that flag.

Add per-lane and aggregate counters for `demoguard_dob_conflict`, `demoguard_sex_conflict`, `demoguard_missing`, `demoguard_eid_missing`, `demoguard_cluster_conflict` and `demoguard_human_missing_merge`, plus link-trace reasons `dob_conflict`, `sex_conflict`, `demographic_missing` and `demographic_cluster_conflict`.

Update `scripts/pairfeat_experiment.sh` arm validation, seed table, comments and manifest recheck:

```bash
r1photo8) DEV_SEEDS="827 829 839"; VAL_SEEDS="853 857 859 863 877"; CHAIN=pairfeat8; REF=r1photo8ref; REFCHAIN=shared;;
```

Accept both arm names and require the shared experiment identity/chain map. Run `bash -n` afterward.

- [ ] **Step 6: Implement `_r1photo8_gate`**

Route `reference_gate == "r1photo8"` from `reference_gate`. Reuse existing safety, recall, hours and reviewer-count helpers, but implement these exact rules:

```python
need = 0.0
strict = kind in ("selection", "validation")
for key in ("false_merge_pairs", "cross_person_edges", "uncapped_wrong_distinct_pairs"):
    # candidate < reference when strict; candidate <= reference in stress
```

Require equal nonzero truth totals, SAFETY and wrong-release-by-origin zero in baseline/reference/candidate, and hours ≤1.10×. Return the same structured gate shape as earlier gates, with reasons naming the failed metric and both counts. Only this reference gate decides pass; the baseline gate remains reported.

- [ ] **Step 7: Verify all harness tests and runner syntax**

Run:

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest discover -s scripts -p 'test_*.py'
bash -n scripts/pairfeat_experiment.sh
```

Expected: all pass, including every old digest.

- [ ] **Step 8: Commit Task 4**

```bash
git add scripts/pairfeat_compare.py scripts/pairfeat_experiment.sh scripts/test_pairfeat_compare.py
git commit -m "Add shared-chain r1photo8 experiment and gates"
```

---

### Task 5: Fit, smoke, experiment, diagnosis and documentation

**Files:**
- Modify only at the documentation step: `pipeline/RESULTS.md`, `docs/BACKEND-HANDOFF.md`
- Artifacts outside Git: `~/wasl-ml-data/agents2/hardening/pairfeat8/run-1/`

**Interfaces:**
- Consumes: all previous tasks at one clean HEAD.
- Produces: one pairfeat8 fit, frozen experiment manifests, three-arm analyses and a documented selected/not-selected outcome.

- [ ] **Step 1: Verify the exact run HEAD**

Record `git rev-parse HEAD`. Require a clean tracked tree for `pipeline/`, `encoder/` and `scripts/`, at least 20 GB free, then run:

```bash
cd pipeline && go vet . && go test . -count=1
PATH=/opt/anaconda3/bin:$PATH python3 -I encoder/test_fit_pairfeat1.py
PATH=/opt/anaconda3/bin:$PATH python3 -I encoder/test_fit_scorer.py
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest discover -s scripts -p 'test_*.py'
bash -n scripts/pairfeat_experiment.sh
```

All must pass before fitting. Move any pre-existing `models/*_pairfeat8.json` files into the experiment's `models-aside/` directory so the fit must recreate them.

- [ ] **Step 2: Fit pairfeat8 and validate artifacts**

Run:

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I encoder/fit_pairfeat1.py "$HOME/wasl-ml-data/agents2/hardening/pairfeat8/run-1" --modelset pairfeat8
```

Require exit 0, recorded noise v3 and run HEAD, five restored model files, 34 features, parity within the driver's tolerance, all hard-negative floors satisfied, and a present `pairfeat8_chain_sha256`. Do not supply a pairfeat5 reference.

Initialize without a second fit:

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I scripts/pairfeat_compare.py init-experiment "$HOME/wasl-ml-data/agents2/hardening/pairfeat8/run-1" r1photo8
```

Read back `experiment.json` and confirm candidate/reference/arm mapping and chain hashes.

- [ ] **Step 3: Smoke the exact candidate lane**

Build a smoke `wsi7` at the run HEAD, add the smoke bin directory's `nats-server` symlink, and drive seed 101 through `lane_commands(..., candidate="r1photo8")` using the harness's own argv. Require every subprocess exit 0, `-noise v3` at gen, exactly one `-demoguard` on model commands, zero SAFETY counts, and valid demographic trace counts. Inspect at least one generated same-person registration set and confirm no distinct real DOBs.

- [ ] **Step 4: Run development and stop at the first failed gate**

For arms `baseline`, `r1photo8ref`, `r1photo8`, run `scripts/pairfeat_experiment.sh` in dev-clean on seeds 827 829 839. Analyse with:

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I scripts/pairfeat_compare.py dev-clean "$EXP/dev-clean.json" baseline="$EXP/dev-baseline" r1photo8ref="$EXP/dev-r1photo8ref" r1photo8="$EXP/dev-r1photo8"
```

Require same-HEAD/chain binding and zero SAFETY counts. If the reference gate fails, stop and document; do not alter seeds, thresholds or gates.

- [ ] **Step 5: Run stress and frozen validation only after a dev pass**

Run dev-stress for the same three arms, analyse, then:

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I scripts/pairfeat_compare.py freeze-validation "$EXP"
```

Only after stress passes, run validation-clean on 853 857 859 863 877 and analyse. Stop at the first failure.

- [ ] **Step 6: Diagnose without changing the run**

Write a read-only analysis under the experiment's `diag/` directory covering: recall by seed and paired intervals; false merges; cross-person edges; impure clusters; uncapped wrong-pair reviews; true automatic merges; reviewer-assisted missing-data merges; hours; each demographic rejection/demotion reason; completion items and answer direction; edge classes; and horizon-excluded recall. Check specifically whether missing demographics merely converted automatic true merges into reviewer workload.

- [ ] **Step 7: Document and independently review the outcome**

Add a new r1photo8 section after r1photo7 in `pipeline/RESULTS.md` and one paragraph in `docs/BACKEND-HANDOFF.md`. State the changed DOB assumption before any result, keep old-distribution numbers separate, report all three modes and every gate, disclose failures and reruns, and state that nothing was promoted to default. Include the agentic boundary: hard conflicts cannot be overridden; a future evidence agent may only address needs-review cases with cited evidence and human sign-off.

Dispatch an independent documentation reviewer to check every number against analysis JSON and every claim against the spec. Apply corrections, then commit only:

```bash
git add pipeline/RESULTS.md docs/BACKEND-HANDOFF.md
git commit -m "Document r1photo8 demographic-guard experiment"
```

- [ ] **Step 8: Thin non-record artifacts and record completion**

Keep summaries, manifests, diagnosis, reviews and the run of record. Delete raw smoke lanes, scratch binaries and feature CSVs of non-record refits. Record the final HEAD, outcome, record-run location and freed disk space in the SDD ledger. Do not push or merge.

---

## Self-Review

- **Spec coverage:** Task 1 covers v3 and generator invariants; Task 2 covers pair, EID, reviewer and cluster rules; Task 3 covers pairfeat8 runtime/fit; Task 4 covers shared arms, v3 lanes, metrics and every gate; Task 5 covers verification, fresh-seed execution, diagnosis, documentation and thinning.
- **No placeholders:** each implementation task names its files, interfaces, test cases, commands and commit boundary. Experiment outcomes are deliberately not pre-written.
- **Type/name consistency:** `noisyDob3`, `DemoGuard`, `demoClassify`, `demoDecision`, `demoClash`, `lane_noise`, `_r1photo8_gate`, `r1photo8ref`, `r1photo8` and `pairfeat8_chain_sha256` are used consistently across tasks.
- **Intentional design rulings:** pairfeat8 has no pairfeat5 reference because v1t/v3 calibration counts are not comparable; reference-only seed sharing is not a second experiment use; evaluation names use v2-style noise while featdump names retain their training profile.
