# pairfeat9 Middle-Initial Hard Negatives Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Train and evaluate a pairfeat9 chain with a narrow 0.002 middle-initial hard-negative generator while preserving recall and safety gates.

**Architecture:** Add an isolated training-only generator to the existing pairfeat7/8 negative pipeline. Reuse all 34 features and the pairfeat8 fit protocol with one new `mi` sidecar kind, then register a fresh-seed offline-agent experiment.

**Tech Stack:** Go, Python 3, LightGBM, existing Wasl replay/harness.

**Spec:** `docs/superpowers/specs/2026-11-05-crc-pairfeat9-middle-initial-design.md`

## Global Constraints

- Rate is exactly **0.002**; no broad exact-name negatives, deterministic initial penalty, threshold, `givenOnly`, demoguard or gate change.
- All 34 features and leave-own-out rarity behavior remain unchanged.
- Selection seeds: **1061 1063 1069**. Validation seeds: **1073 1079 1081 1087 1091**. Never train/tune on them or on r1agent11's failing pair.
- Use `PATH=/opt/anaconda3/bin:$PATH` for Python and `python3 -I` for scripts.
- No `/Users` paths, identifiers, secrets, binaries or model files in Git. Experiment artifacts stay under `~/wasl-ml-data`.
- Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`. Use path-only commits with `Co-Authored-By: Claude Code <noreply@anthropic.com>`.
- No push, merge, buyer/clinician/organiser contact or paid fallback. Stop at the first failed gate and disclose all post-hoc changes.
- Fit requires a clean tree and a new output directory. Do not overwrite existing artifacts.
- After documentation, thin non-record raw lanes/CSVs; keep summaries and the run of record.

---

### Task 1: Middle-initial generator and Go registration

**Files:**
- Create: `pipeline/pairfeat9.go`
- Create: `pipeline/pairfeat9_test.go`
- Modify: `pipeline/pairfeat7.go:211-251`
- Modify: `pipeline/pairfeat6.go:118-120`
- Modify: `pipeline/main.go:149-153,217-223`

**Interfaces:**
- Produces: `MINamesakeRate`, `miSeed`, `buildMINamesakes`, `miRegister`; the existing `pairfeat7Negatives` emits a third `hnKind{key:"mi"}` only when `MINamesakeRate > 0`.
- `buildMINamesakes(ps []Person, rate float64, r *rand.Rand) (xs []Person, base []int)` returns one different person per eligible sampled base.
- `miRegister(regs []fdReg, xpid int, x *Person) []fdReg` appends only the namesake registration; the base already has its ordinary registration.

- [ ] **Step 1: Write the failing generator tests**

Create `pipeline/pairfeat9_test.go` with:

```go
package main

import (
    "math/rand"
    "reflect"
    "strings"
    "testing"
)

func TestBuildMINamesakesExactPattern(t *testing.T) {
    ps := []Person{
        {Qid: "a", En: "Kate O'Brien", Dob: "1963-01-01", Sex: "F", Spl: "train", Origin: "ie", Idx: 1},
        {Qid: "b", En: "Kate L. O'Brien", Dob: "1964-02-02", Sex: "F", Spl: "train", Origin: "ie", Idx: 2},
        {Qid: "c", En: "Kate O'Brien", Dob: "1965-03-03", Sex: "U", Spl: "train", Origin: "ie", Idx: 3},
    }
    xs, base := buildMINamesakes(ps[:1], 0.999999, rand.New(rand.NewSource(1)))
    if len(xs) != 1 || !reflect.DeepEqual(base, []int{0}) {
        t.Fatalf("xs=%+v base=%v", xs, base)
    }
    x := xs[0]
    if x.Qid != "mi-a" || x.Dob != "" || x.Eid != "" || x.Sex != "F" || x.Idx != 0 {
        t.Fatalf("namesake %+v", x)
    }
    got := strings.Fields(x.En)
    if len(got) != 3 || got[0] != "Kate" || got[1] != "A" || got[2] != "O'Brien" {
        t.Fatalf("name pattern %q", x.En)
    }
    for _, ps2 := range [][]Person{ps[1:2], ps[2:3]} {
        if got, _ := buildMINamesakes(ps2, 0.999999, rand.New(rand.NewSource(1))); len(got) != 0 {
            t.Errorf("ineligible base produced %+v", got)
        }
    }
}

func TestMINamesakesDeterministicAndZeroRate(t *testing.T) {
    ps := []Person{{Qid: "a", En: "Kate O'Brien", Dob: "1963-01-01", Sex: "F"}}
    r, fresh := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
    if got, _ := buildMINamesakes(ps, 0, r); len(got) != 0 || r.Int63() != fresh.Int63() {
        t.Fatal("rate zero generated a person or consumed randomness")
    }
    a, _ := buildMINamesakes(ps, 0.5, rand.New(rand.NewSource(7)))
    b, _ := buildMINamesakes(ps, 0.5, rand.New(rand.NewSource(7)))
    if !reflect.DeepEqual(a, b) {
        t.Fatal("generator is not deterministic")
    }
    regs := miRegister(nil, 9, &a[0])
    if len(regs) != 1 || regs[0].pid != 9 || regs[0].name != a[0].En || regs[0].dob != "" || regs[0].eid != "" {
        t.Fatalf("registration %+v", regs)
    }
}

func TestPairfeat9Registration(t *testing.T) {
    if !usesNameIDF("pairfeat9") || !xscriptCapFor("pairfeat9") {
        t.Fatal("pairfeat9 must inherit rarity features and cross-script cap")
    }
    if err := validateModelPolicy("pairfeat9", "v7strict"); err != nil {
        t.Fatal(err)
    }
    if err := validateIDFNamesakes("-minamesakes", "featdump", 0.002, "idf.json"); err != nil {
        t.Fatal(err)
    }
    if err := validateIDFNamesakes("-minamesakes", "gen", 0.002, "idf.json"); err == nil {
        t.Fatal("training-only flag accepted on gen")
    }
}
```

- [ ] **Step 2: Run the tests and confirm the missing-function failure**

Run: `go test -run 'TestBuildMINamesakesExactPattern|TestMINamesakesDeterministicAndZeroRate|TestPairfeat9Registration' -v`
Expected: compile failure because `buildMINamesakes`, `miRegister` and pairfeat9 registration do not exist.

- [ ] **Step 3: Implement the isolated generator**

Create `pipeline/pairfeat9.go`:

```go
package main

import (
    "math/rand"
    "strings"
)

var MINamesakeRate float64 // featdump only, with -nameidf: middle-initial hard negatives (pairfeat9)

func miSeed(seed int64) int64 { return seed*2654435761 + 307093 }

func miName(en string, bi int) (string, bool) {
    t := strings.Fields(latinForm(en))
    if len(t) < 2 {
        return "", false
    }
    for _, x := range t[1:len(t)-1] {
        if len(x) == 1 {
            return "", false
        }
    }
    initial := string(rune('A' + bi%26))
    out := []string{t[0], initial}
    out = append(out, t[1:]...)
    return strings.Join(out, " "), true
}

func buildMINamesakes(ps []Person, rate float64, r *rand.Rand) ([]Person, []int) {
    if rate <= 0 {
        return nil, nil
    }
    var xs []Person
    var base []int
    for bi := range ps {
        b := &ps[bi]
        if r.Float64() >= rate || (b.Sex != "M" && b.Sex != "F") {
            continue
        }
        name, ok := miName(b.En, bi)
        if !ok {
            continue
        }
        xs = append(xs, Person{Qid: "mi-" + b.Qid, En: name, Dob: "", Sex: b.Sex, Cc: b.Cc,
            Spl: b.Spl, Origin: b.Origin, ArSrc: b.ArSrc})
        base = append(base, bi)
    }
    return xs, base
}

func miRegister(regs []fdReg, xpid int, x *Person) []fdReg {
    return append(regs, fdReg{xpid, x.En, "", x.Sex, "", 0})
}
```

In `pairfeat7Negatives`, add `{"mi", MINamesakeRate, miSeed(seed)}` after the rd kind. Select `buildMINamesakes(ps, k.rate, r)` for mi and call `miRegister(regs, first+i, &xs[i])`; retain md/rd's existing `hnRegister` calls and increment `first` exactly once per generated person. Do not alter md/rd behavior.

In `pairfeat6.go`, use:

```go
func usesNameIDF(modelSet string) bool {
    return modelSet == "pairfeat6" || modelSet == "pairfeat7" || modelSet == "pairfeat8" || modelSet == "pairfeat9"
}
```

In `main.go`, add:

```go
fs.Float64Var(&MINamesakeRate, "minamesakes", 0, "featdump only, with -nameidf: share of persons given a synthetic middle-initial namesake hard negative (pairfeat9 training only)")
```

Add `{"-minamesakes", MINamesakeRate}` to the existing `validateIDFNamesakes` loop.

- [ ] **Step 4: Run generator and existing negative tests**

Run: `go test -run 'TestBuildMINamesakes|TestMINamesakes|TestPairfeat9Registration|TestPairfeat7Validate|TestBuildMDNamesakes|TestBuildRDNamesakes|TestHNRegister' -v`
Expected: PASS, with no change to md/rd behavior.

- [ ] **Step 5: Commit**

```bash
git add pipeline/pairfeat9.go pipeline/pairfeat9_test.go pipeline/pairfeat7.go pipeline/pairfeat6.go pipeline/main.go
git commit -m "Add pairfeat9 middle-initial hard negatives" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 2: Fit protocol, sidecar validation and fit tests

**Files:**
- Modify: `encoder/fit_pairfeat1.py:37-57,149-162,251-270,409-414,430-470,526-560,669-679`
- Modify: `encoder/test_fit_pairfeat1.py` (pairfeat8 protocol and slice tests)

**Interfaces:**
- `PROTOCOLS["pairfeat9"]` is pairfeat8 plus `mi_namesakes: 0.002`.
- `HARD_NEGATIVES7` becomes `(("md", "missing-date"), ("rd", "same-date"), ("mi", "middle-initial"))`.
- Existing generic loops emit `mi_rows`, `mi_namesake_slice`, manifest totals and code hashes. Pairfeat8 emits no mi fields.

- [ ] **Step 1: Write failing fit protocol tests**

Add a `Pairfeat9Protocol` test class with:

```python
def test_protocol_and_arguments(self):
    p = fp.PROTOCOLS["pairfeat9"]
    self.assertEqual(p["mi_namesakes"], 0.002)
    self.assertEqual(p["noise"], "v3")
    self.assertEqual(fp.nfeat("pairfeat9"), 34)
    self.assertIn("-minamesakes", fp._namesake_args("pairfeat9"))
    self.assertIn("0.002", fp._namesake_args("pairfeat9"))
    self.assertNotIn("-minamesakes", fp._namesake_args("pairfeat8"))
    self.assertIn("pipeline/pairfeat9.go", fp.base_manifest.__code__.co_consts)

def test_mi_slice_and_manifest(self):
    with tempfile.TemporaryDirectory() as d:
        c = os.path.join(d, "x.csv")
        sc = os.path.join(d, "x.namesakes.json")
        with open(c, "w", newline="") as h:
            csv.writer(h).writerows([[0] + ["0"] * 41, [0] + ["0"] * 41])
        fp.write_json(sc, {"mi_rows": [0, 1], "rows": [], "family_rows": []})
        got = fp.hard_negative_slice(c, sc, "mi", width=42)
        self.assertEqual(got["rows"], 2)
```

Also assert `HARD_NEGATIVES7` contains `("mi", "middle-initial")` and pairfeat8's manifest does not acquire `mi_namesakes`.

- [ ] **Step 2: Run tests and confirm failure**

Run: `PATH=/opt/anaconda3/bin:$PATH python3 -I encoder/test_fit_pairfeat1.py Pairfeat9Protocol -v`
Expected: FAIL because pairfeat9 is absent.

- [ ] **Step 3: Implement the fit protocol**

Add to `PROTOCOLS`:

```python
"pairfeat9": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v3",
              "namesakes": 0.10, "family_namesakes": 0.10, "lambda_l2": 1.0, "xs_namesakes": 0.10, "nfeat": 34,
              "md_namesakes": 0.10, "rd_namesakes": 0.10, "mi_namesakes": 0.002},
```

Make these exact changes:
- Append `("mi", "middle-initial")` to `HARD_NEGATIVES7`.
- Add `PAIRFEAT9_CODE_FILES = ["pipeline/pairfeat9.go"]` and include it in `base_manifest` only when `proto.get("mi_namesakes")` is set.
- Include pairfeat9 in `USAGE`.
- Keep the generic `_namesake_args`, floor, training-manifest and slice loops unchanged; they already consume `HARD_NEGATIVES7`.
- Extend the hard-negative slice print loop to include `"mi"` and update comments to pairfeat7/9.

- [ ] **Step 4: Run fit tests**

Run: `PATH=/opt/anaconda3/bin:$PATH python3 -I encoder/test_fit_pairfeat1.py -v`
Expected: all tests PASS, including pairfeat8 with no mi fields.

- [ ] **Step 5: Commit**

```bash
git add encoder/fit_pairfeat1.py encoder/test_fit_pairfeat1.py
git commit -m "Register pairfeat9 fit protocol" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 3: Fit and verify pairfeat9 artifacts

**Files:**
- Create outside Git: `~/wasl-ml-data/agents2/hardening/pairfeat9/run-1/`
- Modify outside Git only: `models/` pairfeat9 files written by the fit

**Interfaces:**
- Produces `chain/{encoder,lgb_stream,iso,iso_v7,name_idf}_pairfeat9.json`, `training-manifest.json` and `artifact-manifest.json`.
- The artifact manifest records `mi_namesake_slice`, `mi_namesakes=0.002` in the training manifest, exact CSV/model hashes and code HEAD.

- [ ] **Step 1: Verify clean source and full suites**

Run: `git status --short -- pipeline encoder scripts && go test ./... && PATH=/opt/anaconda3/bin:$PATH python3 -I encoder/test_fit_pairfeat1.py`
Expected: no runtime-code changes/untracked files and all tests PASS.

- [ ] **Step 2: Run the fit**

```bash
mkdir -p "$HOME/wasl-ml-data/agents2/hardening/pairfeat9"
PATH=/opt/anaconda3/bin:$PATH python3 -I encoder/fit_pairfeat1.py \
  "$HOME/wasl-ml-data/agents2/hardening/pairfeat9/run-1" --modelset pairfeat9
```

Expected: new `run-1`; no overwrite or paid dependency.

- [ ] **Step 3: Verify artifact identity and mi evidence**

Run a Python check that:
- every chain file exists and matches `artifact-manifest.json["pairfeat9_chain_sha256"]`;
- `training-manifest.json` has `mi_namesakes == 0.002` and positive `mi_namesake_base_rows_total`;
- every `mi_rows` sidecar row is label 0 with `f12 == "0"`, `f19 == "1"`, `f20 == "1"` and `f23 == "NaN"`;
- feature width is 41 columns without p and 42 with p;
- the artifact's `mi_namesake_slice` has nonzero rows.

If any check fails, preserve the failed run for diagnosis and use a new output path after fixing the cause; do not delete evidence or broaden the rate.

- [ ] **Step 4: Commit only source changes needed by the fit**

If no source changes were needed, make no commit. If a defect required a fix, add its regression test, rerun both suites, and commit only those source files with the required trailer.

---

### Task 4: Register the r1agent12 experiment and harness tests

**Files:**
- Modify: `scripts/pairfeat_compare.py` chain registry and `CANDIDATES`
- Modify: `scripts/pairfeat_experiment.sh` allowed arms and seed table
- Modify: `scripts/test_pairfeat_compare.py` candidate expectations and a new `R1agent12Config` test
- Create: `docs/superpowers/specs/2026-11-05-crc-r1agent12-pairfeat9-design.md`

**Interfaces:**
- `CHAIN_FILES["pairfeat9"]` contains encoder, lgb, iso, iso_v7 and name_idf files.
- `r1agent12` uses chain pairfeat9, reference r1photo8ref, base r1photo8, `reference_gate="r1agent8"`, `agent_replay=True`, noise v3 and flags `-r1review -demoguard`.
- Seeds are exactly the spec's 1061/1063/1069 and 1073/1079/1081/1087/1091.

- [ ] **Step 1: Write failing harness tests**

Add `R1agent12Config` asserting the exact chain, arms, flags, seed uniqueness and a passing `_r1agent8_gate` fixture. Update expected `CANDIDATES`, `ALLOWED_ARMS`, `MODELSET`, noise, edge-class and runner tables to include pairfeat9/r1agent12 while preserving earlier candidates.

- [ ] **Step 2: Run tests and confirm failure**

Run: `PATH=/opt/anaconda3/bin:$PATH python3 -I scripts/test_pairfeat_compare.py R1agent12Config -v`
Expected: KeyError for r1agent12.

- [ ] **Step 3: Implement registration**

Add the pairfeat9 `CHAIN_FILES` entry and r1agent12 config using the exact interface above. Add `r1agent12` to the runner's allowed arms, `CHAIN=pairfeat9`, seed table and shared-arm mapping. Do not change other candidates' seeds, chains or gates.

Create the spec documenting pairfeat9 evidence, the deferred-investigation and resolved-edge fixes, unchanged agent semantics, fresh seeds and all existing gates.

- [ ] **Step 4: Run all harness tests**

Run: `PATH=/opt/anaconda3/bin:$PATH python3 -I scripts/test_pairfeat_compare.py`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add scripts/pairfeat_compare.py scripts/pairfeat_experiment.sh scripts/test_pairfeat_compare.py \
  docs/superpowers/specs/2026-11-05-crc-r1agent12-pairfeat9-design.md
git commit -m "Register r1agent12 pairfeat9 experiment" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 5: Initialize and run gated evaluation

**Files:**
- Create outside Git: `~/wasl-ml-data/agents2/hardening/pairfeat9/r1agent12-run-1/`

**Interfaces:**
- Three arms: r1photo8ref, r1photo8, r1agent12. The pairfeat9 chain is used by all three; no model refit occurs during lane execution.

- [ ] **Step 1: Create the experiment parent and initialize**

Copy/hard-link the fit CSVs and chain from `pairfeat9/run-1`, copy its two manifests, then update `artifact-manifest.json` `git_head` to the r1agent12 registration commit. Run:

```bash
python3 -I scripts/pairfeat_compare.py init-experiment \
  "$HOME/wasl-ml-data/agents2/hardening/pairfeat9/r1agent12-run-1" r1agent12
for arm in r1photo8 r1photo8ref r1agent12; do
  PAIRFEAT_PREFLIGHT_ONLY=1 scripts/pairfeat_experiment.sh \
  "$HOME/wasl-ml-data/agents2/hardening/pairfeat9/r1agent12-run-1/dev-$arm" dev-clean "$arm" || exit
 done
```

Expected: all three preflights pass.

- [ ] **Step 2: Run development only**

Run all three arms sequentially at the same HEAD/binary and analyze with:

```bash
python3 -I scripts/pairfeat_compare.py dev-clean EXPERIMENT/dev-analysis.json \
  r1photo8=EXPERIMENT/dev-r1photo8 \
  r1photo8ref=EXPERIMENT/dev-r1photo8ref \
  r1agent12=EXPERIMENT/dev-r1agent12
```

Use the existing no-cost environment capture and `WASL_LLM_MODEL=gpt-5.6-luna`; never print secrets. Stop on the first arm or analysis error.

- [ ] **Step 3: Inspect the selection verdict**

Read `dev-analysis.json` and `selection.json`. Record recall, false merges, cross-person edges, wrong-pair reviews, human hours, replay/safety counts, coverage, citations and the dedicated mi slice. If any gate fails, stop and diagnose read-only; do not start stress or broaden the rate.

- [ ] **Step 4: Run stress and validation only after passes**

After a passing selection, run `dev-stress` on 1061/1063/1069 and analyze. After stress passes, run `freeze-validation`, then validation on 1073/1079/1081/1087/1091. Do not change code, prompts, models, thresholds or gates between stages.

- [ ] **Step 5: Document and review before any integration decision**

Write and independently review `pipeline/RESULTS.md` and `docs/BACKEND-HANDOFF.md`, labeling results simulated and disclosing every infrastructure and experiment amendment. Commit only the final result documents at that stage. Present merge/PR/keep options; do not push or merge without the user's choice. Thin non-record raw runs only after documentation.
