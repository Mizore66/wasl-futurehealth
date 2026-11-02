# CRC Review Floor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Test whether lowering only the existing review-routing floor improves CRC closure recall without weakening linkage, closure, evidence, consent, or attachment safeguards.

**Architecture:** One process-wide `LinkReview` value configures both `LinkerCore.Lo` and every closure-side near-fact gate. The default remains 0.2; non-default values are explicitly forwarded to child/replay processes and stamped into policy identity. Fresh paired development runs select at most one arm, stress review must pass, and only then may a frozen untouched validation run begin.

**Tech Stack:** Go 1.22+, Bash, Python 3 standard library, existing Wasl pipeline/report7/link-funnel harness.

**Spec:** `docs/superpowers/specs/2026-11-02-crc-review-floor-design.md`

## Global Constraints

- Work only in this worktree on `feat/crc-link-recall-diagnostics`; no merge, push, publication, frontend work, paid calls, scorer redesign, model training, retrieval expansion, or attachment changes.
- Read `AGENTS.md` and `docs/BACKEND-HANDOFF.md`; `pipeline/` is canonical.
- Default is exactly 0.2. Accept only `0 < floor <= 0.2`; default invocations keep existing child arguments and behavior.
- Merge 0.7, closure 0.99, uncorroborated cap 0.989, EID/cannot-link/contradiction/second-identifier rules, evidence, consent, and attachment remain unchanged.
- Equal-EID contradiction clamp remains `max(L.Lo, 0.2)`, never the lowered floor.
- Raw GBM score remains the routing input. Near facts remain untrusted; only the existing human path can merge them.
- Development seeds are exactly 101, 103, 107 on dev/v2. Never inspect validation seeds 211, 223, 227, 229, 233 before code, selected floor, analysis plan, manifest, and seeds are frozen.
- Never use cal/train people or previous frozen/B8 seeds 11, 23, 37, 41, 59, 17, 29.
- Clean and stress results stay separate. Stress reviewer errors 0.04 and 0.25 are synthetic assumptions, not clinician measurements.
- Raw evidence stays under `~/wasl-ml-data/agents2/hardening/`; committed files contain aggregate results only and no `/Users` paths or secrets. Preserve failed artifacts.
- Use `PATH=/opt/anaconda3/bin:$PATH`; do not run `gofmt -w` on pre-existing unformatted files; never commit binaries.
- Stop at 18:00 local rather than skipping safety checks. An incomplete or negative experiment is a valid reported outcome.

---

### Task 1: Configure the linker floor without changing its safeguards

**Files:**
- Modify: `pipeline/linker.go:101-117,492-540`
- Create: `pipeline/linkfloor_test.go`
- Modify: `pipeline/link_funnel_test.go` only if a shared fixture is needed

**Interfaces:**
- Produces: package variable `LinkReview float64`, initialized to 0.2.
- Produces: `NewLinkerCore` sets `Lo: LinkReview`; `NewLinkerCoreANN` inherits it.
- Consumes: existing `scoredGBM`, `eidEvent`, `sameCluster`, and trace fixtures in `pipeline/link_funnel_test.go`.

- [ ] **Step 1: Write failing default/routing/clamp tests**

Create `pipeline/linkfloor_test.go`. Use subtests that save and restore `LinkReview` with `t.Cleanup` so tests cannot leak configuration:

```go
package main

import "testing"

func withLinkFloor(t *testing.T, floor float64) {
	t.Helper()
	old := LinkReview
	LinkReview = floor
	t.Cleanup(func() { LinkReview = old })
}

func TestLinkFloorCandidateRouting(t *testing.T) {
	second := func() *Prep {
		return eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", "")
	}
	for _, tc := range []struct {
		name, want string
		floor float64
	}{{"default", "below_review", 0.2}, {"enabled", "near", 0.01}} {
		t.Run(tc.name, func(t *testing.T) {
			withLinkFloor(t, tc.floor)
			_, traces, _ := traceFixture(scoredGBM(-4), second())
			pairs := pairsOnly(traces)
			if len(pairs) != 1 || pairs[0].Stage != tc.want {
				t.Fatalf("want %s, got %+v", tc.want, pairs)
			}
		})
	}
}

func TestLinkFloorDoesNotMergeOrCloseCandidate(t *testing.T) {
	withLinkFloor(t, 0.01)
	L, traces, _ := traceFixture(scoredGBM(-4), eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", ""))
	if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N2") {
		t.Fatal("review-floor candidate merged below LinkThr")
	}
	if got := pairsOnly(traces); len(got) != 1 || got[0].Stage != "near" || got[0].P >= LinkThr {
		t.Fatalf("unexpected routing: %+v", got)
	}
}

func TestEqualEIDContradictionClampStaysAtPointTwo(t *testing.T) {
	withLinkFloor(t, 0.01)
	oldGuard := EIDContraGuard
	EIDContraGuard = true
	t.Cleanup(func() { EIDContraGuard = oldGuard })
	eid := luhnComplete("78419802077524")
	L := NewLinkerCore(scoredGBM(-20), 1)
	var traces []LinkTrace
	L.trace = func(e LinkTrace) { traces = append(traces, e) }
	L.Apply(eidEvent("sr-1", "ahs-mushrif", "A1", "Lamia Eddinari", "1999-07-19", eid))
	other := eidEvent("r-2", "noor-endo", "N2", "Lamia Eddinari", "1999-07-19", eid)
	other.Ev.Sex = "male"
	L.Apply(other)
	pairs := pairsOnly(traces)
	if len(pairs) != 1 || pairs[0].Stage != "near" || pairs[0].P != 0.2 {
		t.Fatalf("contradiction clamp moved with review floor: %+v", pairs)
	}
	if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N2") {
		t.Fatal("contradicting equal-EID records merged")
	}
}
```

The score `-4` produces a raw probability between 0.01 and 0.2 and below the 0.7 merge threshold; the identical name/DOB fixture produces a retrieved candidate.

Add a default-parity test that constructs two fresh cores with `LinkReview = 0.2`, applies the same two `eidEvent` inputs, and compares `canon(outputs)`, `Stats`, and cluster membership exactly as `TestLinkFunnelDoesNotChangeDecisions` does. It must fail before implementation because `LinkReview` is currently a constant in another file and `withLinkFloor` cannot assign it.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run 'TestLinkFloor|TestEqualEIDContradictionClamp' -count=1
```

Expected: compile failure because `LinkReview` is a constant and cannot be assigned. Record the actual failure in the task report.

- [ ] **Step 3: Implement the minimal core change**

In `pipeline/closure.go`, change only the constant declaration into a package variable with the same default and comment:

```go
var LinkReview = 0.20 // below: ignored; -linkfloor may lower review routing only
```

Keep the other constants in the existing `const` block. In `NewLinkerCore`, replace `Lo: 0.2` with `Lo: LinkReview`. Replace only the equal-EID contradiction clamp:

```go
if c.p < L.Lo {
	c.p = L.Lo
}
```

with:

```go
if min := 0.2; c.p < min {
	c.p = min
}
```

Do not change the later `c.p >= L.Thr` / `c.p >= L.Lo` routing branches.

- [ ] **Step 4: Verify core behavior and existing safeguard suites**

Run:

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run 'TestLinkFloor|TestEqualEIDContradictionClamp|TestLinkFunnel|TestEID|TestSecond|TestGovernance' -count=1
```

Expected: PASS. If the clamp fixture does not reach the intended contradiction branch, fix the test fixture using existing `eidguard` tests; do not relax the assertion.

- [ ] **Step 5: Commit Task 1**

```bash
git add pipeline/linker.go pipeline/closure.go pipeline/linkfloor_test.go
git commit -m "Configure linker review floor"
```

---

### Task 2: Make every closure-side near gate use the configured floor

**Files:**
- Modify: `pipeline/closure.go:313`, `pipeline/dr.go:103`, `pipeline/breast.go:106`, `pipeline/ifhas.go:122`, `pipeline/ldct.go:118`, `pipeline/lung.go:146`, `pipeline/handoffext.go:51`, `pipeline/handoffref.go:65`
- Create: `pipeline/linkfloor_closure_test.go`

**Interfaces:**
- Consumes: mutable package variable `LinkReview` from Task 1.
- Produces: all near-fact consumers accept facts at the configured floor; no new review or merge API.

- [ ] **Step 1: Write a failing closure-gate regression**

Use the existing `df` helper from `governance_test.go` to construct a CRC loop with one downstream untrusted near fact at strength 0.05. Set `LinkReview = 0.01`, apply it through `NewClosureCore`, and assert a `link` review item is emitted. Add the default 0.2 control asserting no such item and assert no closed decision results:

```go
func TestLinkFloorNearFactRaisesReviewWithoutClosing(t *testing.T) {
	for _, tc := range []struct {
		floor float64
		wantReviews int
	}{{0.2, 0}, {0.01, 1}} {
		withLinkFloor(t, tc.floor)
		facts := &LoopFacts{LoopID: "L", Prog: "crc", SRDay: 1,
			Down: []DownFact{df("DiagnosticReport", "final", 0.05, "wasl", true)}}
		out := NewClosureCore(0, nil).Apply(&FactsMsg{ID: "L", InSeq: 1, Facts: facts})
		reviews := 0
		for _, msg := range out {
			if msg.Item != nil {
				for _, kind := range msg.Item.Kinds {
					if kind == "link" { reviews++ }
				}
			}
			if msg.Dec != nil && msg.Dec.State == "closed" {
				t.Fatalf("near fact closed loop at floor %v", tc.floor)
			}
		}
		if reviews != tc.wantReviews {
			t.Fatalf("floor %v: want %d link reviews, got %d: %+v", tc.floor, tc.wantReviews, reviews, out)
		}
	}
}
```

- [ ] **Step 2: Run the closure regression and verify RED**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run TestLinkFloorNearFactRaisesReviewWithoutClosing -count=1
```

Expected: enabled case emits zero link reviews because `closure.go` still compares against the old constant or the test fails to compile until Task 1 is present. With Task 1's variable in place, confirm the failure is specifically the enabled case.

- [ ] **Step 3: Convert every hard-coded consumer**

Because Task 1 changed `LinkReview` from a constant to a variable, all existing comparisons already read the configured value. Verify every occurrence and remove no check:

```bash
rg -n 'LinkReview|0\.20|0\.2' pipeline/{closure,dr,breast,ifhas,ldct,lung,handoffext,handoffref}.go
```

Expected gates: closure 1, DR 1, breast 1, IFHAS 1, LDCT 1, lung 1, handoffext 1, handoffref 1. Do not replace numeric thresholds belonging to evidence, timing, merge, or closure.

- [ ] **Step 4: Add safeguard and answer-reuse coverage**

Extend existing tests rather than weakening them. Run the named suites covering EID conflict, cannot-link, sex/DOB contradiction, second identifier, uncorroborated cap, pair answer reuse, and twin suppression. Add one low-floor table run to the existing governance test proving two loops for the same pair produce one reviewer answer and no duplicate queued twin:

```go
withLinkFloor(t, 0.01)
// Reuse the existing ReuseLinkAnswer two-loop fixture and assertions unchanged.
```

- [ ] **Step 5: Verify full Go behavior**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -count=1
PATH=/opt/anaconda3/bin:$PATH go vet ./pipeline
```

Expected: PASS with no new warnings.

- [ ] **Step 6: Commit Task 2**

```bash
git add pipeline/linkfloor_closure_test.go pipeline/*_test.go
git commit -m "Test configured near-fact review gates"
```

Stage only tests actually changed for this task; never use a broad add if it includes unrelated files.

---

### Task 3: Add `-linkfloor`, forwarding, identity, and replay parity

**Files:**
- Modify: `pipeline/main.go:144,188-193,276-282,334-378`
- Modify: `pipeline/link_funnel_test.go` for actual-floor config coverage
- Create: `pipeline/linkfloor_cli_test.go`

**Interfaces:**
- Produces: CLI flag `-linkfloor`, default 0.2, valid only in `(0,0.2]`.
- Produces: global `LinkReview`; optional child argument `-linkfloor <value>` only when below 0.2.
- Produces: policy suffix `+linkfloor=<decimal>` only when below 0.2.
- Consumes: existing `v7Args`, `LinkFunnelArgs`, `PolicyName`, `NewLinkerCoreANN` paths.

- [ ] **Step 1: Write failing CLI/identity tests**

Test validation through a small pure helper rather than spawning the whole binary:

```go
func TestValidateLinkFloor(t *testing.T) {
	for _, floor := range []float64{0.2, 0.05, 0.01} {
		if err := validateLinkFloor(floor); err != nil { t.Fatalf("%v: %v", floor, err) }
	}
	for _, floor := range []float64{0, -0.01, 0.200001, 1} {
		if err := validateLinkFloor(floor); err == nil { t.Fatalf("%v accepted", floor) }
	}
}

func TestLinkFloorArgsAndPolicy(t *testing.T) {
	old := LinkReview
	t.Cleanup(func() { LinkReview = old })
	LinkReview = 0.2
	if got := linkFloorArgs(); len(got) != 0 { t.Fatalf("default argv changed: %v", got) }
	LinkReview = 0.01
	got := linkFloorArgs()
	if len(got) != 2 || got[0] != "-linkfloor" || got[1] != "0.01" { t.Fatalf("%v", got) }
	if got := linkFloorPolicySuffix(); got != "+linkfloor=0.01" { t.Fatalf("%q", got) }
}
```

- [ ] **Step 2: Run tests and verify RED**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run 'TestValidateLinkFloor|TestLinkFloorArgsAndPolicy' -count=1
```

Expected: undefined helper failures.

- [ ] **Step 3: Implement flag, validation, and default-off forwarding**

Add near `-linkfunnel`:

```go
linkFloor := fs.Float64("linkfloor", 0.2, "linker+closure: candidate review floor (0,0.2]; merge/closure thresholds unchanged")
```

Immediately after `fs.Parse`:

```go
if err := validateLinkFloor(*linkFloor); err != nil {
	fmt.Fprintln(os.Stderr, "configuration error:", err)
	os.Exit(2)
}
LinkReview = *linkFloor
```

Add helpers in `main.go` or a focused `linkfloor.go`:

```go
func validateLinkFloor(v float64) error {
	if !(v > 0 && v <= 0.2) { return fmt.Errorf("-linkfloor must be >0 and <=0.2, got %v", v) }
	return nil
}
func linkFloorArgs() []string {
	if LinkReview == 0.2 { return nil }
	return []string{"-linkfloor", strconv.FormatFloat(LinkReview, 'g', -1, 64)}
}
func linkFloorPolicySuffix() string {
	if LinkReview == 0.2 { return "" }
	return "+linkfloor=" + strconv.FormatFloat(LinkReview, 'g', -1, 64)
}
```

Append `PolicyName += linkFloorPolicySuffix()` after the base policy name is built. Append `v7Args = append(v7Args, linkFloorArgs()...)` before child construction. This reaches orchestrate children and any audit/report path already receiving `v7Args`; inspect direct audit/report invocations and add the same arguments wherever they do not use `v7Args`. The linker and audit replay must construct cores after parsing the flag.

The funnel already records `ReviewFloor: L.Lo`; extend `TestLinkFunnelWriterWritesConfigAndPair` to assert `"review_floor":0.01` when configured with 0.01. Keep sticky writer errors unchanged.

- [ ] **Step 4: Verify CLI tests, default parity, and forwarding**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -run 'TestLinkFloor|TestValidateLinkFloor|TestEqualEID|TestLinkFunnel' -count=1
PATH=/opt/anaconda3/bin:$PATH go test ./pipeline -count=1
PATH=/opt/anaconda3/bin:$PATH go vet ./pipeline
bash -n scripts/link_funnel_dev.sh
```

Build once with `scripts/demo.sh build`, then verify invalid values exit 2 and default output contains no new child flag. Remove `bin/wsi7` afterward; do not commit it.

- [ ] **Step 5: Commit Task 3**

```bash
git add pipeline/main.go pipeline/linkfloor.go pipeline/linkfloor_cli_test.go pipeline/link_funnel_test.go
git commit -m "Forward opt-in link review floor"
```

---

### Task 4: Generalize the protected paired evaluation harness

**Files:**
- Create: `scripts/link_floor_experiment.sh`
- Create: `scripts/link_floor_compare.py`
- Create: `scripts/test_link_floor_compare.py`
- Modify: no existing runtime files

**Interfaces:**
- Consumes: `-linkfloor` from Task 3 and existing `scripts/link_funnel_dev.sh` command structure.
- Produces: per-lane `funnel.json`, `report7.json`, `status-linker.json`, aggregate `summary.json`, and machine-readable `selection.json`.

- [ ] **Step 1: Write analyzer tests before the harness**

Use synthetic report/funnel dictionaries covering: pooled recall arithmetic, exactly +1.0pp pass, below-gate fail, any safety/analysis failure rejects, contamination increase rejects, and deterministic tie preference. Selection rule fixed before seeing results: among passing arms prefer 0.05 over 0.01 to minimize reviewer exposure; select no arm if neither passes.

```python
def test_selects_conservative_when_both_pass(self):
    baseline = arm(floor=0.2, tp=500, truth=600)
    conservative = arm(floor=0.05, tp=507, truth=600)  # +1.1667pp
    primary = arm(floor=0.01, tp=510, truth=600)
    result = select_arm(baseline, [conservative, primary])
    self.assertEqual(result["selected_floor"], 0.05)

def test_contamination_increase_rejects(self):
    baseline = arm(floor=0.2, tp=500, truth=600, cross_person_edges=2)
    candidate = arm(floor=0.01, tp=510, truth=600, cross_person_edges=3)
    self.assertIsNone(select_arm(baseline, [candidate])["selected_floor"])
```

Define test `arm()` with all required fields: false closures, wrong-patient releases total/by-origin, replay mismatch, analysis errors, trace-write error, false-merge pairs, impure clusters, cross-person edges. Safety fields default to zero in the helper; tests must explicitly set each rejection field once.

- [ ] **Step 2: Run analyzer tests and verify RED**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest scripts/test_link_floor_compare.py -v
```

Expected: import failure for missing `link_floor_compare.py`.

- [ ] **Step 3: Implement aggregate analyzer**

Compute pooled CRC recall as `sum(tp)/sum(truth_closed)` and gain in percentage points. Extract TP from `programmes_live.crc.tp`; do not confuse it with funnel `attached_or_closed`. Sum the three linkage contamination fields from `report7.json["linkage"]`; preserve per-seed values. Record reviewer items, distinct pairs, hours, unanswered, and burden by programme/origin using the exact existing `report7.json["reviewer"]` fields; fail loudly if a required field is absent rather than silently substituting zero.

`select_arm` returns a dictionary with `selected_floor` (`0.05`, `0.01`, or `None`), per-arm gate results, gains, and reasons. It must not read validation paths. Include a `t_interval(values)` helper using only stdlib; for five validation seeds use two-sided 95% t critical 2.776 and for three development seeds 4.303, returning mean ± `critical*sample_stdev/sqrt(n)` and documenting small-n uncertainty.

- [ ] **Step 4: Implement the protected lane runner**

Copy the existing harness's path/symlink/new-directory protections and subprocess guard. Interface:

```bash
scripts/link_floor_experiment.sh NEW_ROOT dev-clean 0.2
scripts/link_floor_experiment.sh NEW_ROOT dev-clean 0.05
scripts/link_floor_experiment.sh NEW_ROOT dev-clean 0.01
scripts/link_floor_experiment.sh NEW_ROOT dev-stress 0.2
scripts/link_floor_experiment.sh NEW_ROOT dev-stress 0.05  # use 0.01 instead if that arm won
scripts/link_floor_experiment.sh NEW_ROOT validation-clean 0.2
scripts/link_floor_experiment.sh NEW_ROOT validation-clean 0.05  # use 0.01 instead if that arm won
```

Modes fix these exact settings:

| Mode | Split | Seeds | rerr | errungraded |
|---|---|---|---:|---:|
| dev-clean | dev | 101 103 107 | 0 | 0 |
| dev-stress | dev | 101 103 107 | 0.04 | 0.25 |
| validation-clean | eval | 211 223 227 229 233 | 0 | 0 |

The runner must reject every other mode/floor/seed combination, require a new root per arm, use cached reader/image off/chaos 0/subkill 1/no NATS kill, pass `-linkfloor` to orchestrate/audit/report7, require linker status with no trace error, and run classifier/report checks. It must refuse to create validation lanes unless `validation-manifest.json` already exists in the parent experiment directory and its Git HEAD, selected floor, analyzer SHA256, and seeds exactly match the current frozen inputs. Do not generate that manifest inside the lane runner.

- [ ] **Step 5: Verify harness and all script tests**

```bash
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest scripts/test_link_floor_compare.py scripts/test_link_funnel.py -v
bash -n scripts/link_floor_experiment.sh
```

Expected: all PASS. Statically test invalid split/seeds/floors and missing validation manifest with no clinical process launched.

- [ ] **Step 6: Commit Task 4**

```bash
git add scripts/link_floor_experiment.sh scripts/link_floor_compare.py scripts/test_link_floor_compare.py
git commit -m "Add paired link-floor experiment harness"
```

---

### Task 5: Run development, stress, and frozen validation; report the verdict

**Files:**
- Create externally: `~/wasl-ml-data/agents2/hardening/link-floor-experiment/`
- Modify: `pipeline/RESULTS.md` with aggregate experiment results only
- Modify: `docs/BACKEND-HANDOFF.md` only to point to the experiment result and its simulated-data limitations

**Interfaces:**
- Consumes: protected runner and analyzer from Task 4.
- Produces: selected floor or explicit no-selection; development/stress/validation verdicts; no default change beyond 0.2.

- [ ] **Step 1: Freeze development inputs and run three clean arms**

Record Git HEAD and SHA256 of the runner/analyzer in an external development manifest before any run. Use three separate fresh roots and the exact commands in Task 4. Do not run arms against different code revisions or reuse generated cohorts across revisions. Keep the existing binary out of Git.

- [ ] **Step 2: Apply selection gates once**

Run the analyzer on all three arms. Select at most one arm only when pooled CRC closure-recall gain is at least +1.0pp and all safety/replay/analysis/contamination gates pass. If both pass, choose 0.05 under the predeclared conservative tie rule. If neither passes, retain 0.2, skip stress/validation, and proceed directly to documentation with `selected_floor: null`.

- [ ] **Step 3: Stress only baseline and selected arm**

If selected, run fresh dev-stress lanes at reviewer errors 0.04/0.25 on the same three seeds. Any false closure, wrong-patient release, replay mismatch, analysis/trace failure, or increase in any linkage contamination field rejects the candidate. Keep these results separate from clean-review results and label both error rates synthetic assumptions.

- [ ] **Step 4: Freeze untouched validation before its first run**

Only after development and stress selection succeeds, write external `validation-manifest.json` with actual values. From the experiment directory, set `FLOOR` to the selected `0.05` or `0.01`, then run:

```bash
FLOOR=0.05 python3 -I - <<'PY'
import hashlib, json, os, subprocess
sha = lambda p: hashlib.sha256(open(p, "rb").read()).hexdigest()
manifest = {
    "git_head": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
    "selected_floor": float(os.environ["FLOOR"]),
    "seeds": [211, 223, 227, 229, 233],
    "split": "eval", "noise": "v2",
    "reviewer_error": 0, "day15_ungraded_prep_reviewer_error": 0,
    "analysis": "paired pooled CRC recall; improved means strictly greater than baseline; all safety and contamination gates unchanged",
    "runner_sha256": sha("scripts/link_floor_experiment.sh"),
    "analyzer_sha256": sha("scripts/link_floor_compare.py"),
}
with open(os.path.expanduser("~/wasl-ml-data/agents2/hardening/link-floor-experiment/validation-manifest.json"), "x") as h:
    json.dump(manifest, h, indent=2, sort_keys=True)
PY
```

Change `FLOOR=0.05` to `FLOOR=0.01` when the primary arm won. Hash and preserve this manifest before launching either validation arm. No result-driven changes are allowed afterward.

- [ ] **Step 5: Run paired untouched validation and report**

Run baseline and selected arm on all five eval/v2 seeds. Validation passes only with strictly improved pooled CRC closure recall and zero listed safety/replay/analysis failures with no increase in any contamination field. Report per-seed and pooled recall, absolute gain, small-n 95% t interval across seed recalls, reviewer burden, residual funnel misses, and all failures. If validation fails, preserve artifacts and retain default 0.2 without retuning.

- [ ] **Step 6: Run final verification and secret scans**

```bash
PATH=/opt/anaconda3/bin:$PATH go test ./... -count=1
PATH=/opt/anaconda3/bin:$PATH go vet ./...
PATH=/opt/anaconda3/bin:$PATH python3 -I -m unittest discover -s scripts -p 'test_*.py' -v
bash -n scripts/link_funnel_dev.sh scripts/link_floor_experiment.sh
git diff --check
git status --short
```

Run the repository's established TruffleHog scan without bypassing it. Confirm no `/Users` paths, secrets, generated binaries, raw patient/pair traces, or external evidence are staged.

- [ ] **Step 7: Commit honest aggregate documentation**

Document built versus simulated versus designed status, exact code revision, all arms including failures, clean versus stress review, development versus untouched validation, post-hoc changes, residual misses, and that no clinical effectiveness is proven. If no arm passed, say recall did not improve under this experiment.

```bash
git add pipeline/RESULTS.md docs/BACKEND-HANDOFF.md docs/superpowers/plans/2026-11-02-crc-review-floor.md
git commit -m "Record CRC review-floor experiment results"
```
