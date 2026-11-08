# CRC Feature-First Pair Scorer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and evaluate an opt-in 30-feature pair scorer that recovers retrieved same-person links without changing retrieval or weakening identity, closure, evidence, consent, or attachment safeguards.

**Architecture:** Append seven deterministic EID/DOB/name comparisons to the existing 23-feature vector, reuse the current encoder and candidate generator, and refit LightGBM plus both existing calibrators on fresh train/cal splits. A protected paired harness compares the current modelset and `pairfeat1`, both at review floor 0.01, through development, synthetic reviewer stress, and only then untouched validation.

**Tech Stack:** Go 1.22+, Python 3, NumPy, scikit-learn, SciPy, LightGBM, Bash, existing Wasl FHIR/NATS/report7/link-funnel harness.

**Spec:** `docs/superpowers/specs/2026-11-02-crc-pair-scorer-design.md`

## Global Constraints

- Work only in this worktree on `feat/crc-link-recall-diagnostics`; no merge, push, publication, frontend work, paid calls, live credentials, buyer/clinician/organiser contact, or external patient data.
- Read `AGENTS.md` and `docs/BACKEND-HANDOFF.md`; `pipeline/` is canonical. Every result must be labelled built, simulated, or designed.
- Default modelset and default review floor 0.2 remain unchanged. Experimental arms both use `-linkfloor 0.01`; merge 0.7, closure 0.99, uncorroborated cap 0.989, and equal-EID contradiction clamp 0.2 remain unchanged.
- Existing feature indices 0–22, especially corroboration indices 6, 13, and 19, keep their semantics. Candidate retrieval and the encoder are fixed. No encoder fallback, hyperparameter sweep, threshold adjustment, or attachment change is permitted after a failed gate.
- Training seeds are exactly train `1001 1003 1007 1009 1013`, cal `2003`; development is dev/v2 `307 311 313`; untouched validation is eval/v2 `401 409 419 421 431`. Never use frozen/B8 seeds `11 23 37 41 59 17 29` or prior experiment dev/eval seeds for this experiment.
- LightGBM is exactly 400 rounds, 63 leaves, learning rate 0.05, seed 1, deterministic mode, and the existing remaining `PARAMS`. Fit on train people only; calibrate on cal people only. No dev/eval/cal person may enter fitting.
- `pairfeat1` supports `v7strict` only; `pairfeat1` + `v7ltt` fails closed. Generated models, CSVs, pickles, binaries, and raw traces stay outside Git. Committed files contain no absolute home-directory paths, registration-level identifiers, or secrets.
- Raw artifacts live under `~/wasl-ml-data/agents2/hardening/pairfeat1/`; use `PATH=/opt/anaconda3/bin:$PATH`; never use `/tmp` for experiment artifacts; preserve every failed root. Do not run `gofmt -w` on pre-existing unformatted files.
- Clean and stress evidence stay separate. The 4% ordinary and 25% day-15 ungraded-prep reviewer errors are synthetic stress assumptions, not measured clinician rates. Small-n t intervals are descriptive, not clinical-effectiveness proof.
- Arms run sequentially because `bin/wsi7` is shared. No clinical lane may start until runtime and harness reviews pass and the relevant manifest is frozen. Any failed gate preserves artifacts and stops without retuning or another seed set.
- End every commit with `Co-Authored-By: Claude Code <noreply@anthropic.com>`.

## File Structure

- `pipeline/text.go`: legacy feature compatibility plus the seven appended comparisons.
- `pipeline/pairfeat_test.go`: exact feature, missing-data, and default-model parity tests.
- `pipeline/encdev.go`, `pipeline/main.go`, `pipeline/encdev_test.go`: model policy guard and existing four-file modelset naming.
- `pipeline/link_funnel.go`, `pipeline/services.go`, `pipeline/link_funnel_test.go`: modelset identity in diagnostics.
- `pipeline/gen.go`, `pipeline/encdev.go`, `pipeline/encdev_test.go`: one shared EID-noise draw and deterministic 30-feature/slice dumping.
- `encoder/fit_scorer.py`, `encoder/test_fit_scorer.py`: strict 30-feature fitting schema, calibrators, and hard-negative slice report.
- `encoder/fit_pairfeat1.py`, `encoder/test_fit_pairfeat1.py`: frozen training manifest, artifact hashes, encoder copy, and Go/Python score parity.
- `scripts/pairfeat_experiment.sh`, `scripts/pairfeat_compare.py`, `scripts/test_pairfeat_compare.py`: protected paired lanes, gates, validation freezing, workload, residual, and interval analysis.
- `pipeline/RESULTS.md`, `docs/BACKEND-HANDOFF.md`: aggregate simulated outcome and limitations only.

---

### Task 1: Append and pin the 30-feature contract

**Files:**
- Modify: `pipeline/text.go:260-336`
- Create: `pipeline/pairfeat_test.go`

**Interfaces:**
- Produces: `const NFEAT = 30` and `pairFeats(a, b *Rec, cos, gap float64) []float64` with legacy indices 0–22 and appended indices 23–29.
- Produces: `pairExtraFeats(a, b *Rec) []float64`, always length seven, using `math.NaN()` for unavailable EID/DOB/name evidence.
- Consumes: `Rec.Eid`, `Rec.tok`, `Rec.skel`, `dummyEID`, `realDob`, and `placeholderDob`.

- [ ] **Step 1: Write failing exact-value and missing-data tests**

Create table-driven tests with two normalized records. Assert all seven appended values, including `math.IsNaN`, for: one-digit EID typo; unequal/missing/dummy EIDs; decade-only DOB change; day-only and month-only changes; invalid and placeholder DOBs; reordered equal token sets; equal skeletons; and empty names. Include this exact decade case:

```go
func TestPairExtraFeatDecadeAndNearEID(t *testing.T) {
	a := &Rec{Eid: "784198020775241", Dob: "1980-05-17", tok: []string{"mona", "saleh"}, skel: []string{"mn", "slh"}}
	b := &Rec{Eid: "784198020775231", Dob: "1990-05-17", tok: []string{"saleh", "mona"}, skel: []string{"mn", "slh"}}
	got := pairExtraFeats(a, b)
	want := []float64{1, 1, 1, 0, 0, 1, 1}
	if len(got) != len(want) { t.Fatalf("len=%d", len(got)) }
	for i := range want {
		if got[i] != want[i] { t.Errorf("f%d=%v want %v", i+23, got[i], want[i]) }
	}
}

func TestPairExtraFeatMissingIsNotAgreement(t *testing.T) {
	for _, tc := range []struct{ name, da, db string }{
		{"placeholder", "1980-01-01", "1990-01-01"},
		{"invalid", "1980-02-30", "1990-02-30"},
		{"missing", "", "1990-05-17"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := pairExtraFeats(&Rec{Dob: tc.da, tok: []string{"a"}}, &Rec{Dob: tc.db, tok: []string{"a"}})
			for i := 1; i <= 4; i++ {
				if !math.IsNaN(got[i]) { t.Errorf("f%d=%v want NaN", i+23, got[i]) }
			}
		})
	}
}
```

Add a legacy-position test asserting length 30 and exact values at indices 6, 13, 19, 20, 21, and 22 for a fixed fixture. Add `TestDefaultGBMIgnoresAppendedFeatures`: load `../models/lgb_stream.json`, compute `p23 := g.Predict(f[:23])` and `p30 := g.Predict(f)`, require `len(f) == 30` and exact `p23 == p30`; repeat `g.Predict(f)` 100 times and require bit-identical results. The length assertion must fail before implementation.

- [ ] **Step 2: Run tests and verify RED**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run 'TestPairExtraFeat|TestDefaultGBM|TestPairFeatLegacy' -count=1
```

Expected: compile failure because `pairExtraFeats` is undefined; after a temporary stub, the length/default-parity test fails against the 23-feature vector. Remove any temporary stub before the real implementation.

- [ ] **Step 3: Implement the seven comparisons**

Add `math.NaN()` values only for unavailable evidence. Use the existing normalized token and skeleton slices; do not add a second normalization path.

```go
const NFEAT = 30 // legacy 0..22 + pairfeat1 comparisons 23..29

func pairExtraFeats(a, b *Rec) []float64 {
	nan := math.NaN()
	out := []float64{nan, nan, nan, nan, nan, nan, nan}
	if a.Eid != "" && b.Eid != "" && len(a.Eid) == len(b.Eid) && !dummyEID(a.Eid) && !dummyEID(b.Eid) {
		n := 0
		for i := range a.Eid { if a.Eid[i] != b.Eid[i] { n++ } }
		out[0] = float64(n)
	}
	if realDob(a.Dob) && realDob(b.Dob) && !placeholderDob(a.Dob) && !placeholderDob(b.Dob) {
		ndiff, diffPos, pos := 0, 0, 0
		for i := 0; i < 10; i++ {
			if i == 4 || i == 7 { continue }
			if a.Dob[i] != b.Dob[i] { ndiff++; diffPos = pos }
			pos++
		}
		out[1], out[2] = 0, float64(ndiff)
		if ndiff == 1 && diffPos == 3 { out[1] = 1 }
		out[3] = boolFloat(a.Dob[:7] == b.Dob[:7] && a.Dob != b.Dob)
		out[4] = boolFloat(a.Dob[:4] == b.Dob[:4] && a.Dob[8:] == b.Dob[8:] && a.Dob != b.Dob)
	}
	if len(a.tok) > 0 && len(b.tok) > 0 { out[5] = boolFloat(stringSetEq(a.tok, b.tok)) }
	if len(a.skel) > 0 && len(b.skel) > 0 { out[6] = boolFloat(stringSeqEq(a.skel, b.skel)) }
	return out
}
```

Add equality helpers with distinct set and ordered-sequence semantics; do not reuse `skelSetEq`, which rejects singleton sets:

```go
func stringSetEq(a, b []string) bool {
	sa, sb := map[string]bool{}, map[string]bool{}
	for _, x := range a { sa[x] = true }
	for _, x := range b { sb[x] = true }
	if len(sa) == 0 || len(sa) != len(sb) { return false }
	for x := range sa { if !sb[x] { return false } }
	return true
}

func stringSeqEq(a, b []string) bool {
	if len(a) == 0 || len(a) != len(b) { return false }
	for i := range a { if a[i] != b[i] { return false } }
	return true
}
```

`pairExtraFeats` cannot use the local `b2f` closure inside `pairFeats`; add `func boolFloat(x bool) float64 { if x { return 1 }; return 0 }` and make both functions use it, removing the old closure. Append with `f = append(f, pairExtraFeats(a, b)...)` after existing feature 22. Before the existing fixed-width DOB slicing, add this guard so malformed or noninformative dates cannot panic; preserve valid-date outputs exactly:

```go
if len(da) != 10 || len(db) != 10 || !realDob(da) || !realDob(db) {
	return []float64{0, 0, 0, 0, 0, -1, 0}
}
```

- [ ] **Step 4: Verify feature and safeguard suites**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run 'TestPair|TestDefaultGBM|TestEID|TestSecond|TestLinkFloor|TestLinkFunnel' -count=1
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -count=1
```

Expected: PASS. Review the diff to confirm no threshold, retrieval, closure, consent, or attachment file changed.

- [ ] **Step 5: Commit Task 1**

```bash
git add pipeline/text.go pipeline/pairfeat_test.go
git commit -m "Append targeted CRC pair features" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 2: Guard `pairfeat1` policy and stamp model identity

**Files:**
- Modify: `pipeline/encdev.go:1-30`, `pipeline/main.go:247-272`, `pipeline/link_funnel.go:14-23`, `pipeline/services.go:490`
- Modify: `pipeline/encdev_test.go`, `pipeline/link_funnel_test.go`

**Interfaces:**
- Produces: `validateModelPolicy(modelSet, policy string) error`; only the combination `pairfeat1`/`v7ltt` is rejected.
- Produces: `LinkFunnelConfig.ModelSet string ` + "`json:\"modelset\"`" + ``, populated from package `ModelSet` for live and replay traces.
- Consumes: existing `mf` modelset naming and `PolicyName += "+models=" + ModelSet`.

- [ ] **Step 1: Write failing configuration and identity tests**

Extend `TestModelSetFiles` with the exact `encoder_pairfeat1.json`, `lgb_stream_pairfeat1.json`, `iso_pairfeat1.json`, and `iso_v7_pairfeat1.json` names. Add:

```go
func TestPairFeatRejectsStaleLTTSelection(t *testing.T) {
	if err := validateModelPolicy("pairfeat1", "v7ltt"); err == nil {
		t.Fatal("pairfeat1 accepted stale v7ltt selection")
	}
	for _, policy := range []string{"v7ltt", "v6", ""} {
		if err := validateModelPolicy("pairfeat1", policy); err == nil { t.Errorf("pairfeat1 accepted policy %q", policy) }
	}
	for _, tc := range [][2]string{{"pairfeat1", "v7strict"}, {"", "v7ltt"}, {"real_v2", "v7ltt"}} {
		if err := validateModelPolicy(tc[0], tc[1]); err != nil { t.Errorf("%v: %v", tc, err) }
	}
}
```

Extend the config-writer test to set `ModelSet = "pairfeat1"`, restore it with `t.Cleanup`, and require `"modelset":"pairfeat1"` in the first NDJSON row. Add a default assertion for `"modelset":""` so baseline identity cannot disappear.

- [ ] **Step 2: Run tests and verify RED**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run 'TestModelSetFiles|TestPairFeatRejects|TestLinkFunnelWriter' -count=1
```

Expected: compile failure for the missing guard and `ModelSet` config field.

- [ ] **Step 3: Implement the minimal guard and trace field**

In `pipeline/encdev.go`:

```go
func validateModelPolicy(modelSet, policy string) error {
	if modelSet == "pairfeat1" && policy != "v7strict" {
		return fmt.Errorf("modelset pairfeat1 supports only -policy v7strict; v7ltt needs a separate selection")
	}
	return nil
}
```

Immediately after `ModelSet = *modelSet`, print `configuration error:` and exit 2 if the guard returns an error, before policy calibrator loading. Add the `ModelSet` field to `LinkFunnelConfig` and set `ModelSet: ModelSet` in the existing literal in `services.go`. Do not alter `mf`, policy suffixing, child `v7Args`, thresholds, or calibrator selection.

- [ ] **Step 4: Verify modelset, replay, and all runtime tests**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run 'TestModelSet|TestPairFeat|TestLinkFunnel|Test.*Replay|TestEID|TestSecond' -count=1
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -count=1
```

Expected: PASS. Manually inspect that `PolicyName` still adds `+models=pairfeat1` and existing `v7Args` forwards `-modelset` to audit/replay children.

- [ ] **Step 5: Commit Task 2**

```bash
git add pipeline/encdev.go pipeline/main.go pipeline/link_funnel.go pipeline/services.go pipeline/encdev_test.go pipeline/link_funnel_test.go
git commit -m "Guard experimental pair-scorer modelset" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 3: Make feature dumping EID-aware and deterministic

**Files:**
- Modify: `pipeline/gen.go:237-239,370-420`, `pipeline/encdev.go:97-223`
- Modify: `pipeline/encdev_test.go`

**Interfaces:**
- Produces: `drawEID(p *Person, r *rand.Rand) string`, the single implementation of 62% valid, 26% missing, 12% one-digit-mistyped EIDs used by both `runGen` and `runFeatDump`.
- Produces: featdump CSV columns `label,corr,f0..f29,near_eid,decade_dob,same_surname,placeholder_dob,cross_script[,p]` (37 columns without `p`, 38 with `p`).
- Produces: slice values as 0/1 in that order. `near_eid` is feature 23 equal to 1 or 2; `decade_dob` is feature 24; `same_surname` is feature 6; `placeholder_dob` means either input DOB is a placeholder; `cross_script` is feature 20 equal to 0.

- [ ] **Step 1: Write failing EID-draw and dump-schema tests**

Add a 10,000-draw test using a valid 15-digit synthetic EID. Require all three outcomes to occur, every mistyped value to differ at exactly one digit, and no mutation of `Person.Eid`. Add a dump test that runs two 40-person train-only dumps to separate files under `t.TempDir()` with seed 1001, requires byte-identical CSVs, exactly 37 fields per row, and at least one finite feature 23. Assert the five slice columns are 0/1 and `near_eid`, `decade_dob`, `same_surname`, and `cross_script` agree with their source features.

```go
func TestFeatDumpPairFeatSchemaDeterministic(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a.csv")
	b := filepath.Join(t.TempDir(), "b.csv")
	for _, out := range []string{a, b} {
		runFeatDump("../data", "../models/encoder_weights.json", "", "train", out, 40, 1001)
	}
	ba, err := os.ReadFile(a); if err != nil { t.Fatal(err) }
	bb, err := os.ReadFile(b); if err != nil { t.Fatal(err) }
	if !bytes.Equal(ba, bb) { t.Fatal("featdump is not deterministic") }
	rows, err := csv.NewReader(bytes.NewReader(ba)).ReadAll()
	if err != nil { t.Fatal(err) }
	if len(rows) == 0 { t.Fatal("no candidate rows") }
	finiteEID := false
	for n, row := range rows {
		if len(row) != 37 { t.Fatalf("row %d has %d fields", n, len(row)) }
		v, err := strconv.ParseFloat(row[2+23], 64); if err != nil { t.Fatal(err) }
		finiteEID = finiteEID || !math.IsNaN(v)
	}
	if !finiteEID { t.Fatal("all training EID features are NaN") }
}
```

- [ ] **Step 2: Run tests and verify RED**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run 'TestDrawEID|TestFeatDumpPairFeat' -count=1
```

Expected: compile failure for `drawEID`; after the shared draw exists, schema failure because featdump still emits 32 columns and no EIDs.

- [ ] **Step 3: Share the generator draw and extend featdump**

Move the existing `x := r.Float64()` EID switch from `runGen` into `drawEID` unchanged, and replace the original switch with `g.eid = drawEID(p, r)`. Do not reorder any other random draw. In `runFeatDump`, add `eid` to `regT`, call `eid := drawEID(&p, r)` immediately after the existing sex draw, and append it positionally in the registration literal. Set `Rec.Eid` and `Rec.eidOK = luhnOK(rec.Eid)` before candidate generation. After appending each record, update `L.eidIdx[rec.Eid]` for nonempty EIDs exactly as runtime registration does, so featdump retains the existing exact-EID retrieval path as well as cosine/rule-key candidates.

After the 30 features, write these five values without identifiers:

```go
slices := []int{
	boolInt(!math.IsNaN(c.feat[23]) && (c.feat[23] == 1 || c.feat[23] == 2)),
	boolInt(c.feat[24] == 1),
	boolInt(c.feat[6] == 1),
	boolInt(placeholderDob(rec.Dob) || placeholderDob(L.recs[c.j].Dob)),
	boolInt(c.feat[20] == 0),
}
for _, x := range slices { fmt.Fprintf(w, ",%d", x) }
```

Define `func boolInt(x bool) int { if x { return 1 }; return 0 }` beside `boolFloat` and use it for the five integer slice fields. Update the featdump comment to the exact 37/38-column schema.

- [ ] **Step 4: Verify dump, generator, parity, and full tests**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run 'TestDrawEID|TestFeatDump|TestGen|TestModelSet|TestPair' -count=1
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -count=1
```

Expected: PASS. Confirm no generated CSV or binary is tracked with `git status --short`.

- [ ] **Step 5: Commit Task 3**

```bash
git add pipeline/gen.go pipeline/encdev.go pipeline/encdev_test.go
git commit -m "Add EID-aware pair feature dumping" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 4: Fit strict 30-feature models and report hard negatives

**Files:**
- Modify: `encoder/fit_scorer.py:1-115`
- Create: `encoder/test_fit_scorer.py`

**Interfaces:**
- Consumes: Task 3's 37/38-column numeric CSV.
- Produces: `load(path) -> (y, corr, X, slices)` with `X.shape[1] == 30` and `slices` as a dictionary of five NumPy boolean vectors.
- Produces: `slice_report(y, p, slices) -> dict`; each slice reports `rows`, `positives`, `different_person`, `different_at_review_0_01`, and `different_at_merge_0_7`.
- Produces: the existing `lgb_stream_<suffix>.json`, `iso_<suffix>.json`, `iso_v7_<suffix>.json`, `fit_report_<suffix>.json`, and `cal_p_<suffix>.npy` interfaces unchanged.

- [ ] **Step 1: Write failing loader and slice tests**

Construct small CSVs in `tempfile.TemporaryDirectory()` with exactly 30 features and five slice fields. Test 37- and 38-column acceptance; reject 36 columns, extra columns, empty input, nonbinary labels/corr/slices, and nonfinite model predictions. Verify the slice counts independently:

```python
def test_slice_report_counts_different_person_exposure(self):
    y = np.array([0, 0, 1, 0])
    p = np.array([0.02, 0.8, 0.9, 0.001])
    slices = {"near_eid": np.array([True, True, True, False])}
    got = fs.slice_report(y, p, slices)["near_eid"]
    self.assertEqual(got, {"rows": 3, "positives": 1, "different_person": 2,
                           "different_at_review_0_01": 2, "different_at_merge_0_7": 1})
```

- [ ] **Step 2: Run Python tests and verify RED**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest encoder.test_fit_scorer -v
```

Expected: import/attribute failure for the new loader contract and `slice_report`.

- [ ] **Step 3: Implement strict parsing and slice reporting**

Set `NFEAT = 30` and `SLICES = ("near_eid", "decade_dob", "same_surname", "placeholder_dob", "cross_script")`. Keep `PARAMS`, `ROUNDS`, and `UNC_CAP` unchanged. Implement:

```python
def load(path):
    d = np.loadtxt(path, delimiter=",", dtype=np.float64, ndmin=2)
    if d.shape[0] == 0 or d.shape[1] not in (2 + NFEAT + len(SLICES), 3 + NFEAT + len(SLICES)):
        raise ValueError(f"{path}: expected 37 or 38 columns, got {d.shape[1]}")
    y, corr = d[:, 0].astype(int), d[:, 1].astype(int)
    X = d[:, 2:2 + NFEAT]
    raw = d[:, 2 + NFEAT:2 + NFEAT + len(SLICES)]
    if not np.isin(d[:, :2], [0, 1]).all() or not np.isin(raw, [0, 1]).all():
        raise ValueError(f"{path}: labels, corr, and slice fields must be binary")
    slices = {name: raw[:, i].astype(bool) for i, name in enumerate(SLICES)}
    return y, corr, X, slices

def slice_report(y, p, slices):
    if not np.isfinite(p).all():
        raise ValueError("nonfinite model prediction")
    out = {}
    for name, mask in slices.items():
        neg = mask & (y == 0)
        out[name] = {"rows": int(mask.sum()), "positives": int((mask & (y == 1)).sum()),
                     "different_person": int(neg.sum()),
                     "different_at_review_0_01": int((neg & (p >= 0.01)).sum()),
                     "different_at_merge_0_7": int((neg & (p >= 0.7)).sum())}
    return out
```

Update tuple unpacking for train/cal, concatenate only `y` and `X` for training, and add `rep["hard_negative_slices"] = slice_report(yca, pca, sca)`. Do not tune thresholds or add model parameters. Preserve the existing isotonic and conservative-calibrator construction verbatim.

- [ ] **Step 4: Run focused and existing Python checks**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest encoder.test_fit_scorer -v
PATH=/opt/anaconda3/bin:$PATH python3 -I -m py_compile encoder/fit_scorer.py
```

Expected: PASS. No fitting or clinical lane is part of this task.

- [ ] **Step 5: Commit Task 4**

```bash
git add encoder/fit_scorer.py encoder/test_fit_scorer.py
git commit -m "Fit and measure thirty-feature pair scorer" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 5: Freeze training inputs and produce hashed local artifacts

**Files:**
- Create: `encoder/fit_pairfeat1.py`, `encoder/test_fit_pairfeat1.py`

**Interfaces:**
- Produces: `python3 -I encoder/fit_pairfeat1.py NEW_OUTPUT_DIR`; output must be a new directory under `~/wasl-ml-data/agents2/hardening/pairfeat1/`.
- Produces: `training-manifest.json` before fitting and `artifact-manifest.json` after fitting, with full Git HEAD, code/encoder/input SHA256, seeds/splits, `persons=5000` for each train dump, whole cal split, noise `v1`, LightGBM parameters, baseline and candidate four-file chain SHA256 values, and Go/Python maximum absolute score delta.
- Produces locally: `encoder_pairfeat1.json`, `lgb_stream_pairfeat1.json`, `iso_pairfeat1.json`, `iso_v7_pairfeat1.json` in the output chain and copies into ignored `models/` for the experimental runtime.

- [ ] **Step 1: Write failing manifest and protection tests**

Test that `validate_output` rejects an existing path, a path outside the required parent, and `..` segments. Test `base_manifest` against a temporary fake repository: exact seeds/splits, 30 features, 400 rounds, encoder SHA256, and SHA256 for `pipeline/text.go`, `pipeline/encdev.go`, `pipeline/gen.go`, `encoder/fit_scorer.py`, and `encoder/fit_pairfeat1.py`. Test `check_go_parity` rejects a row-count mismatch, NaN, or delta above `1e-9`, and returns the maximum delta for valid vectors.

- [ ] **Step 2: Run tests and verify RED**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest encoder.test_fit_pairfeat1 -v
```

Expected: import failure because the orchestration module does not exist.

- [ ] **Step 3: Implement the frozen fitting driver**

Use only Python standard library for orchestration. Resolve the repository from `__file__`; require a clean tracked tree and no untracked files under `pipeline`, `encoder`, and `scripts` except `__pycache__`. Build `bin/wsi7` with `go build`. Copy `models/encoder_weights.json` byte-for-byte to `encoder_pairfeat1.json` and require equal SHA256. Write the pre-fit manifest before any dump. Implement the tested helpers with these exact semantics:

```python
def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()

def validate_output(path, home):
    parent = os.path.realpath(os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat1"))
    if ".." in pathlib.PurePath(path).parts or os.path.lexists(path):
        raise ValueError("output must be a new path without .. segments")
    if os.path.commonpath([parent, os.path.realpath(os.path.dirname(path))]) != parent:
        raise ValueError("output parent is outside the protected experiment directory")
    return path

def check_go_parity(go_scores, python_scores, tolerance=1e-9):
    if len(go_scores) != len(python_scores) or not go_scores:
        raise ValueError("Go/Python parity row-count mismatch")
    delta = max(abs(float(a) - float(b)) for a, b in zip(go_scores, python_scores))
    if not math.isfinite(delta) or delta > tolerance:
        raise ValueError(f"Go/Python score delta {delta} exceeds {tolerance}")
    return delta

def base_manifest(root, encoder):
    files = ["pipeline/text.go", "pipeline/encdev.go", "pipeline/gen.go",
             "encoder/fit_scorer.py", "encoder/fit_pairfeat1.py"]
    return {"git_head": subprocess.check_output(["git", "-C", root, "rev-parse", "HEAD"], text=True).strip(),
            "nfeat": 30, "train": {"split": "train", "seeds": [1001, 1003, 1007, 1009, 1013], "persons": 5000},
            "cal": {"split": "cal", "seeds": [2003], "persons": 0}, "noise": "v1",
            "rounds": 400, "num_leaves": 63, "learning_rate": 0.05, "seed": 1,
            "encoder_sha256": sha256(encoder),
            "code_sha256": {name: sha256(os.path.join(root, name)) for name in files}}
```

Run these exact operations, stopping on the first failure and preserving the new root:

```bash
OUT="$1"
mkdir "$OUT" "$OUT/chain"
for seed in 1001 1003 1007 1009 1013; do
  bin/wsi7 featdump -wasl data -encfile "$OUT/chain/encoder_pairfeat1.json" \
    -split train -persons 5000 -noise v1 -seed "$seed" -out "$OUT/feat_train_${seed}.csv"
done
bin/wsi7 featdump -wasl data -encfile "$OUT/chain/encoder_pairfeat1.json" \
  -split cal -noise v1 -seed 2003 -out "$OUT/feat_cal_2003.csv"
python3 -I encoder/fit_scorer.py \
  --train "$OUT/feat_train_1001.csv" --train "$OUT/feat_train_1003.csv" \
  --train "$OUT/feat_train_1007.csv" --train "$OUT/feat_train_1009.csv" \
  --train "$OUT/feat_train_1013.csv" --cal "$OUT/feat_cal_2003.csv" \
  --outdir "$OUT/chain" --suffix pairfeat1
```

Use the fixed encoder copy for every dump. Hash each CSV, the default `encoder_weights.json`, `lgb_stream.json`, `iso.json`, and `iso_v7.json`, and all four candidate chain artifacts into `artifact-manifest.json`. Rerun the cal dump with `-gbmfile "$OUT/chain/lgb_stream_pairfeat1.json"`; parse its final `p` column and compare with `"$OUT/chain/cal_p_pairfeat1.npy"` using `check_go_parity(..., tolerance=1e-9)`. Record row count and maximum delta; fail rather than deleting or replacing artifacts on mismatch. Copy the four chain files to `models/` only after parity passes, refusing to overwrite any existing `*_pairfeat1.json` target. Never add them to Git.

- [ ] **Step 4: Verify driver tests and static safety checks**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest encoder.test_fit_scorer encoder.test_fit_pairfeat1 -v
PATH=/opt/anaconda3/bin:$PATH python3 -I -m py_compile encoder/fit_pairfeat1.py
grep -nE '1001|1003|1007|1009|1013|2003|400|63|0\.05|1e-9' encoder/fit_pairfeat1.py
```

Expected: PASS and exact protocol constants present. Do not run the driver in this task; Task 7 runs it once after review.

- [ ] **Step 5: Commit Task 5**

```bash
git add encoder/fit_pairfeat1.py encoder/test_fit_pairfeat1.py
git commit -m "Freeze pair-scorer training artifacts" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 6: Add the protected paired clinical harness and analyzer

**Files:**
- Create: `scripts/pairfeat_experiment.sh`, `scripts/pairfeat_compare.py`, `scripts/test_pairfeat_compare.py`

**Interfaces:**
- Runner: `scripts/pairfeat_experiment.sh NEW_ROOT MODE ARM`, where `MODE` is `dev-clean`, `dev-stress`, or `validation-clean`; `ARM` is `baseline` or `pairfeat1`.
- Analyzer: `scripts/pairfeat_compare.py MODE OUT_JSON baseline=ROOT pairfeat1=ROOT` writes aggregate results and, for dev-clean, `selection.json`.
- Freeze: `scripts/pairfeat_compare.py freeze-validation EXPERIMENT_DIR` reads a passing selection and writes `validation-manifest.json` containing current HEAD, selected model/artifact hashes, runner/analyzer SHA256, exact settings, and validation seeds.
- Every arm writes `build.json` with HEAD, binary SHA256, mode, arm, modelset (`""` or `pairfeat1`), floor 0.01, and all four selected model-chain SHA256 values.

- [ ] **Step 1: Write failing gate and identity tests**

Copy the useful fixture style from `scripts/test_link_floor_compare.py`, but test the new module directly. Cover:

```python
def test_frozen_gates_and_workload(self):
    base = arm("baseline", 500, 600, distinct_pairs=100)
    self.assertTrue(pfc.gate(base, arm("pairfeat1", 512, 600, distinct_pairs=110), "selection")["pass"])
    self.assertFalse(pfc.gate(base, arm("pairfeat1", 511, 600, distinct_pairs=110), "selection")["pass"])
    self.assertFalse(pfc.gate(base, arm("pairfeat1", 512, 600, distinct_pairs=111), "selection")["pass"])
    self.assertTrue(pfc.gate(base, arm("pairfeat1", 506, 600, distinct_pairs=105), "validation")["pass"])
    self.assertFalse(pfc.gate(base, arm("pairfeat1", 505, 600, distinct_pairs=105), "validation")["pass"])
    self.assertTrue(pfc.gate(base, arm("pairfeat1", 500, 600, distinct_pairs=105), "stress")["pass"])
    self.assertFalse(pfc.gate(base, arm("pairfeat1", 499, 600, distinct_pairs=105), "stress")["pass"])
```

Test every safety field in both arms, every contamination increase, truth-denominator mismatch, and zero-baseline workload handling. Require exactly 2.0pp development and 1.0pp validation thresholds and at most 10% distinct-link-pair growth.

Add runner rejection tests for invalid mode/arm/floor injection, existing/output-outside roots, stress without a passing selection, validation without a manifest, wrong HEAD/seeds/model/artifact/runner/analyzer hashes, and candidate `v7ltt`. Test analyzer rejection of mislabeled funnel modelset/floor/scope, differing HEAD or binary SHA between arms, and either arm's artifact hashes disagreeing with the frozen manifest (baseline and candidate hashes are intentionally different). Test freeze writes exactly `401 409 419 421 431` only after development selection and stress pass.

- [ ] **Step 2: Run harness tests and verify RED**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest scripts.test_pairfeat_compare -v
```

Expected: import failure because the new analyzer does not exist.

- [ ] **Step 3: Implement runner and analyzer from the proven floor harness**

Copy `scripts/link_floor_experiment.sh` to `scripts/pairfeat_experiment.sh` and `scripts/link_floor_compare.py` to `scripts/pairfeat_compare.py`, then make these explicit changes rather than editing the old harness:

```python
MODES = {
    "dev-clean": {"split": "dev", "seeds": [307, 311, 313], "rerr": "0", "errungraded": "0"},
    "dev-stress": {"split": "dev", "seeds": [307, 311, 313], "rerr": "0.04", "errungraded": "0.25"},
    "validation-clean": {"split": "eval", "seeds": [401, 409, 419, 421, 431], "rerr": "0", "errungraded": "0"},
}
FLOOR = "0.01"
MIN_GAIN_PP = {"selection": 2.0, "validation": 1.0}
MAX_REVIEW_GROWTH = 0.10
ARMS = ("baseline", "pairfeat1")
```

The runner must add `-modelset pairfeat1` only to candidate sim/orchestrate/audit/report7 commands, explicitly add `-linkfloor 0.01` to both arms, and require `-policy v7strict`. `dev-clean` requires the parent `artifact-manifest.json` and checks current HEAD plus the selected arm's four hashes; `dev-stress` additionally requires passing selection; validation uses only the frozen validation manifest. Guard every subprocess argument as the old runner does. Preserve cached reader, image off, FHIR, chaos 0, subkill 1, no NATS kill, HNSW, CRC-only generation, sequential arms, clean-tree checks, new roots, logs, and failure preservation.

Extend `check_funnel_floor` with `check_funnel_modelset(funnel, arm)`, requiring `config.modelset == ("" if arm == "baseline" else "pairfeat1")`. Extend `lane_metrics` to include every funnel category, plus:

```python
attached = _req(funnel, "categories").get("attached_or_closed", 0)
if attached < tp:
    raise ValueError("attached_or_closed cannot be less than exact CRC tp")
residual = {name: _req(funnel, "categories").get(name, 0) for name in
            ("candidate_not_generated", "candidate_below_review", "candidate_rejected_demographic",
             "candidate_routed_review", "linked_report_not_attached")}
residual["attached_not_closed"] = attached - tp
if sum(residual.values()) != truth - tp:
    raise ValueError("residual categories do not partition CRC closure misses")
```

Aggregate residuals and reviewer `wrong_final_signoffs` from `report7.reviewer.by_kind.all`; keep these as reported outcomes, not hidden safety substitutions. Add per-seed recall, pooled recall, `t_interval` for per-seed recall, and a paired t interval over candidate-minus-baseline percentage-point gains. Apply safety and contamination checks to both arms and workload gating to all three mode kinds. Development writes `selected_modelset: "pairfeat1"` only on pass, otherwise null.

- [ ] **Step 4: Verify all harness tests and syntax**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest scripts.test_pairfeat_compare scripts.test_link_floor_compare -v
bash -n scripts/pairfeat_experiment.sh
PATH=/opt/anaconda3/bin:$PATH python3 -I -m py_compile scripts/pairfeat_compare.py
```

Expected: PASS. Independently review that old floor scripts are unchanged and no eval lane has run.

- [ ] **Step 5: Commit Task 6**

```bash
git add scripts/pairfeat_experiment.sh scripts/pairfeat_compare.py scripts/test_pairfeat_compare.py
git commit -m "Protect paired pair-scorer evaluation" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 7: Run the gated experiment once and document the outcome

**Files:**
- Modify: `pipeline/RESULTS.md`, `docs/BACKEND-HANDOFF.md`
- External only: `~/wasl-ml-data/agents2/hardening/pairfeat1/`

**Interfaces:**
- Consumes: reviewed Tasks 1–6 and frozen train/validation manifests.
- Produces: one fit, one paired development comparison, conditional paired stress, conditional untouched validation, and aggregate documentation. No production default change.

- [ ] **Step 1: Run pre-lane verification and independent reviews**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -count=1
PATH=/opt/anaconda3/bin:$PATH go vet ./pipeline
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest encoder.test_fit_scorer encoder.test_fit_pairfeat1 scripts.test_pairfeat_compare -v
bash -n scripts/pairfeat_experiment.sh
git diff --check HEAD~6..HEAD
```

Require fresh runtime and harness reviews to approve spec compliance and code quality. Verify all generated model files are ignored and no clinical process is running. Record exact HEAD; all lanes for a phase must use that same reviewed runtime.

- [ ] **Step 2: Fit once under the frozen training protocol**

```bash
export PATH=/opt/anaconda3/bin:$PATH
export EXP="$HOME/wasl-ml-data/agents2/hardening/pairfeat1"
python3 -I encoder/fit_pairfeat1.py "$EXP/run-1"
```

Inspect only train/cal fit reports and artifact/parity manifests. Require finite scores, Go/Python delta at most `1e-9`, matching encoder hash, exact train/cal identities, and all four artifact hashes. Report hard-negative slice exposure. A schema, parity, or artifact failure stops for root-cause correction before any clinical lane; disclose any code change and refreeze rather than quietly reusing the manifest.

- [ ] **Step 3: Run paired development clean and apply the stop gate**

```bash
scripts/pairfeat_experiment.sh "$EXP/run-1/dev-baseline" dev-clean baseline
scripts/pairfeat_experiment.sh "$EXP/run-1/dev-pairfeat1" dev-clean pairfeat1
python3 -I scripts/pairfeat_compare.py dev-clean "$EXP/run-1/dev-clean.json" \
  baseline="$EXP/run-1/dev-baseline" pairfeat1="$EXP/run-1/dev-pairfeat1"
```

Proceed only if pooled gain is at least +2.0pp, all listed safety/replay/analysis counts are zero in both arms, no contamination measure increases, distinct link-review pairs grow by at most 10%, and identity checks pass. On failure, skip stress/validation and document the negative result with both arms and preserved roots.

- [ ] **Step 4: Run paired synthetic stress only after development selection**

```bash
scripts/pairfeat_experiment.sh "$EXP/run-1/stress-baseline" dev-stress baseline
scripts/pairfeat_experiment.sh "$EXP/run-1/stress-pairfeat1" dev-stress pairfeat1
python3 -I scripts/pairfeat_compare.py dev-stress "$EXP/run-1/dev-stress.json" \
  baseline="$EXP/run-1/stress-baseline" pairfeat1="$EXP/run-1/stress-pairfeat1"
```

Require no recall regression and every safety/replay/analysis/contamination/workload gate. Label 4%/25% as synthetic assumptions and report wrong final sign-offs in both arms. Failure stops before validation.

- [ ] **Step 5: Freeze and run untouched validation only after stress passes**

```bash
python3 -I scripts/pairfeat_compare.py freeze-validation "$EXP/run-1"
scripts/pairfeat_experiment.sh "$EXP/run-1/val-baseline" validation-clean baseline
scripts/pairfeat_experiment.sh "$EXP/run-1/val-pairfeat1" validation-clean pairfeat1
python3 -I scripts/pairfeat_compare.py validation-clean "$EXP/run-1/validation-clean.json" \
  baseline="$EXP/run-1/val-baseline" pairfeat1="$EXP/run-1/val-pairfeat1"
```

Do not inspect eval outputs before the manifest exists. Require at least +1.0pp and every other gate. A failure preserves all artifacts, retains the current model, and permits no new seeds, thresholds, features, hyperparameters, or encoder fallback.

- [ ] **Step 6: Document verified aggregate outcomes**

Add one `pipeline/RESULTS.md` section containing Git HEAD, manifests/artifact identities, train/cal pair metrics and hard-negative slices, per-seed and pooled clean/stress/validation results, descriptive t intervals, reviewer items/hours/unanswered/wrong final sign-offs, distinct link pairs, residual partition including attached-not-closed, nonzero contamination, and every post-hoc change or failed run. State exact CRC TP as `pred_closed - false_closures`; never substitute attach rate.

Update `docs/BACKEND-HANDOFF.md` with a short pointer. Regardless of outcome, state that the default model remains unchanged. If validation passes, describe `pairfeat1` only as a validated opt-in candidate; simulated CRC evidence establishes neither clinical effectiveness nor non-CRC safety/recall.

- [ ] **Step 7: Verify documentation and commit only aggregate Markdown**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -count=1
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest encoder.test_fit_scorer encoder.test_fit_pairfeat1 scripts.test_pairfeat_compare -v
git diff --check
! grep -R -n "$HOME\|BEGIN .*PRIVATE KEY" pipeline/RESULTS.md docs/BACKEND-HANDOFF.md
git status --short
```

Request an independent results-document review against the preserved aggregate JSON. Commit only the two Markdown files after approval:

```bash
git add pipeline/RESULTS.md docs/BACKEND-HANDOFF.md
git commit -m "Record feature-first pair-scorer experiment" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

No merge, push, default-model change, or publication is authorized by this plan.
