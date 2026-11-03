# CRC pairfeat6: placeholder dates, name rarity and cross-script hard negatives — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Test whether a `pairfeat5`-recipe pair scorer with four new pair features (placeholder date, real exact date, shared-token rarity, rarest unmatched token) and training-only cross-script hard negatives removes cross-person auto-merges relative to `pairfeat5` without losing CRC recall, with 0 false closures in every arm.

**Architecture:** New modelset `pairfeat6`. Go appends `f30`–`f33` to the 30-feature vector only when a name-rarity table is loaded (`-modelset pairfeat6` at runtime, `featdump -nameidf` when training); every other modelset keeps the 30-feature vector byte for byte. A new `wsi nameidf` command builds the table from the training split's base registrations; `featdump -xsnamesakes 0.10` adds cross-script hard negatives. `encoder/fit_pairfeat1.py --modelset pairfeat6` runs the `pairfeat5` chain (lambda_l2 1.0, leaf check, parity) on 34 features and puts the table into the chain. The cross-script review cap stays on for `pairfeat6`. The harness gains a three-arm candidate `pairfeat6` (baseline, `pairfeat5` reference refit separately at the same HEAD, `pairfeat6`), image off, gated against `pairfeat5` only.

**Tech Stack:** Go 1.22+, Python 3 (numpy, scikit-learn, scipy, lightgbm 4.7.0), Bash.

**Spec:** `docs/superpowers/specs/2026-11-03-crc-pairfeat6-design.md` (binding, user-approved).

## Global Constraints

- With any modelset other than `pairfeat6` (default, `pairfeat1`-`pairfeat5`), the pair-feature vector, its length (30), every `featdump` CSV and sidecar, linker behaviour, traces and every output are byte-identical to HEAD `931a823`. `encoder/fit_scorer.py` without `--nfeat` and `encoder/fit_pairfeat1.py` for `pairfeat1`-`pairfeat5` produce byte-identical argv, outputs and manifests.
- `pairfeat1`-`pairfeat5` and `r1review` driver and harness behaviour, seeds, gates, outputs and tests are unchanged (byte-identity checked by digests). In existing tests only candidate-set equalities and unknown-candidate sentinels may change: the `pairfeat6` sentinels move to `pairfeat7`.
- `pairfeat6` seeds, never used:
  - development 659 661 673;
  - validation 677 683 691 701 709.
- Never touch: development 307–587 as used, validation 401–431, 601–619, 631 641 643 647 653, B8/frozen 11 23 37 41 59 17 29. Spelled out: development 307 311 313 317 331 337 347 353 367 433 449 461 521 541 547 557 563 569 571 577 587; validation 401 409 419 421 431, 601 607 613 617 619, 631 641 643 647 653; B8/frozen 11 23 37 41 59 17 29.
- Raw artifacts under `~/wasl-ml-data/agents2/hardening/pairfeat6/` (the reference refit under `~/wasl-ml-data/agents2/hardening/pairfeat5/`, which `encoder/fit_pairfeat1.py` requires). No `/tmp`. No `/Users` paths, no identifiers, secrets, binaries or models in Git.
- Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`. No push or merge.
- `PATH=/opt/anaconda3/bin:$PATH`. Run Python tests by direct path or `python3 -I -m unittest discover -s scripts -p '<pattern>'`; run Go tests from `pipeline/`.
- Path-only commits: `git add` only the task's new files, then `git commit -m ... -m ... -- <the task's paths>`. Check `git show --stat HEAD` lists exactly those paths.
- Planning left one scratch directory, `~/wasl-ml-data/agents2/hardening/pairfeat6/plan-goldens/` (a BASE binary and two 400-person featdump outputs used to take the Go goldens below). It is removed in Task E's thinning step.

## Spec resolutions (decided here; disclose in the docs)

1. `f30`: a DOB is *usable* if it is a real date (`realDob`) or has day `00` with a real year-month (`YYYY-MM-00` where `YYYY-MM-01` is real; 90 persons in `persons3.json` have such dates). NaN if either DOB is not usable (missing, wrong length, invalid, sentinel `0001-01-01`); else 1 if either is a placeholder (`-01-01`, `-07-01` or day `00`); else 0.
2. `f31`: "both real" means both real and non-placeholder, consistent with `f30`. 1 if equal, 0 if they differ, NaN whenever either is missing, invalid or a placeholder.
3. `f32`/`f33` use *distinct* skeleton tokens (set semantics), summed and compared in sorted order for determinism. `f33` is the largest weight even if negative (a token in every registration has `log(N/(N+1)) < 0`); 0 only when there is no unmatched token. Both are NaN when either skeleton is empty (as `f28`/`f29`).
4. "Training split's registrations": the base registrations (no namesake or cross-script registrations) that `featdump` draws for the five training seeds 1019 1021 1031 1033 1039 at 5,000 persons, noise `v1t`, reproduced by the same code path and RNG sequence. N is their total.
5. Cross-script hard negative: for each chosen base person, two registrations are added: the base person with the placeholder date in a drawn script, and the namesake in the opposite script with the same date, the base person's sex (no DOB or sex noise) and a drawn Emirates ID (`drawEID`). Donors: same split pool, same sex, same first English token, and no other token of either name occurring in the other; origin is not required; single-token names get none. "Such rows" for the floor: rows pairing a namesake with any registration of its base person (the same definition as the existing namesake floors), at least 500 summed over the five training seeds.
6. Featdump is switched to 34 features by `-nameidf PATH` (featdump only), not by `-modelset pairfeat6`: `-modelset` requires `-policy v7strict`, which loads `iso_v7_pairfeat6.json` from the data directory before the chain exists.
7. The cross-script review cap (0.69) is on for `pairfeat6` ("everything about pairfeat5 carries over").
8. The reference arm is a separate `--modelset pairfeat5` fit (the fit only writes under `hardening/<modelset>/`), at `~/wasl-ml-data/agents2/hardening/pairfeat5/pairfeat6-ref-run-1`; `init-experiment` records its chain map and manifest SHA256 in the `pairfeat6` parent's `experiment.json`.
9. Diagnosed classes (reported only) are a deterministic approximation over the final linked different-person pairs of a lane's `link_funnel.ndjson`, in order: `cross_script_lookalike` (one name Arabic-script, the other not), `missing_dob_ambiguous` (a DOB missing on one side), `shared_token` (same script, at least one shared and one differing name token), `other`; `unresolved` when a registration is not in the cohort. Counted on development lanes (dev-clean and dev-stress) for the two non-baseline arms. "Same-person pairs merged" = final linked same-person pairs.
10. "SAFETY zero in every lane of every arm" is checked on arm sums (counts are non-negative, so a zero sum means zero in every lane). `truth_closed` must be equal in all three arms.
11. The last task is named Task E here (the request called it "Task C"; A-D are the build tasks).

---

### Task A: pair features f30–f33 and the rarity table (Go)

**Files:**
- Create `pipeline/pairfeat6.go`, `pipeline/pairfeat6_test.go`.
- Modify:
  - `pipeline/text.go`: `pairFeats`, after line 421 `f = append(f, pairExtraFeats(a, b)...)`;
  - `pipeline/main.go`: flag next to line 177 `gbmFile := ...`; activation at line 268 `XScriptCap = ModelSet == "pairfeat5"`;
  - `pipeline/encdev.go`: `validateModelPolicy`, after line 39–41 (the `pairfeat5` block);
  - `pipeline/xscript.go`: line 9 comment and a new `xscriptCapFor`.

**Interfaces:**
- Produces (`pipeline/pairfeat6.go`):
  - `const NFEAT6 = 34`
  - `var NameIDF *NameIDFTable` (nil by default)
  - `type NameIDFTable struct { Version int; Split string; Persons int; Seeds []int64; Noise string; Registrations int; Counts map[string]int }` with JSON keys `version, split, persons, seeds, noise, registrations, counts`
  - `func readNameIDF(path string) (*NameIDFTable, error)`, `func LoadNameIDF(path string) *NameIDFTable` (panics on error)
  - `func (t *NameIDFTable) Weight(tok string) float64` = `log(Registrations / (1 + Counts[tok]))`
  - `func nameIDFFor(cmd, path, modelSet, wasl string) (*NameIDFTable, error)`
  - `func dobUsable6(d string) bool`, `func dobPlaceholder6(d string) bool`, `func pairfeat6Feats(a, b *Rec, t *NameIDFTable) []float64` (length 4)
- Produces (`pipeline/xscript.go`): `func xscriptCapFor(modelSet string) bool` (true for `pairfeat5`, `pairfeat6`).
- Produces (`pipeline/main.go`): flag `-nameidf PATH` (string, default ""), consumed by Task B.
- Consumes: `Rec.Dob`, `Rec.skel`, `realDob` (`eidguard.go`), `boolFloat` (`text.go`); test helpers `eqNaN` (`pairfeat_test.go`), `runFD2`, `hex256` (`namesake_test.go`).

- [ ] **Step 0: Confirm the BASE goldens** (HEAD must be `931a823`, nothing edited yet):

```bash
G=~/wasl-ml-data/agents2/hardening/pairfeat6/plan-goldens
mkdir -p $G
(cd pipeline && go build -o $G/wsi7.base . && $G/wsi7.base featdump -wasl ../data -encfile ../models/encoder_weights.json \
  -split train -persons 400 -noise v1t -seed 1019 -namesakes 0.1 -familynamesakes 0.1 -out $G/train1019.csv)
shasum -a 256 $G/train1019.csv $G/train1019.csv.namesakes.json
```

Expected: `0b86ffd7f1082bc789b1cdfb2fc5e61af637b23ee9504b94dcd530ad1ec5b99e` (CSV) and `2e292038109d913b8d036f87bfaf8a06dd0a38246340c64914102497a3ca8c16` (sidecar). Stop if they differ.

- [ ] **Step 1: Failing tests.** Create `pipeline/pairfeat6_test.go`:

```go
package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// featdump -persons 400 -seed 1019 -noise v1t -namesakes 0.1 -familynamesakes 0.1 (the pairfeat4/pairfeat5 train argv),
// taken with the CLI (UrduFix on) at 931a823, before pairfeat6.
const (
	goldenTrainNsFam1019   = "0b86ffd7f1082bc789b1cdfb2fc5e61af637b23ee9504b94dcd530ad1ec5b99e"
	goldenTrainNsFam1019Sc = "2e292038109d913b8d036f87bfaf8a06dd0a38246340c64914102497a3ca8c16"
)

func idf6() *NameIDFTable {
	return &NameIDFTable{Version: 1, Registrations: 100,
		Counts: map[string]int{"mhmd": 99, "ahmd": 49, "slh": 9, "zbr": 1, "cmn": 100, "cmm": 100}}
}

func setNameIDF(t *testing.T, tb *NameIDFTable) {
	t.Helper()
	old := NameIDF
	NameIDF = tb
	t.Cleanup(func() { NameIDF = old })
}

func TestNameIDFDefaultsNil(t *testing.T) {
	if NameIDF != nil || NFEAT6 != 34 || NFEAT != 30 {
		t.Fatalf("NameIDF=%v NFEAT6=%d NFEAT=%d", NameIDF, NFEAT6, NFEAT)
	}
}

func TestPairfeat6Weight(t *testing.T) {
	tb := idf6()
	for tok, want := range map[string]float64{"mhmd": 0, "ahmd": math.Log(2), "slh": math.Log(10), "zbr": math.Log(50),
		"cmn": math.Log(100.0 / 101.0), "unseen": math.Log(100)} {
		if got := tb.Weight(tok); math.Abs(got-want) > 1e-15 {
			t.Errorf("Weight(%q) = %v, want %v", tok, got, want)
		}
	}
}

func TestPairfeat6DateFeatures(t *testing.T) {
	nan := math.NaN()
	for _, c := range []struct {
		name, da, db string
		f30, f31     float64
	}{
		{"real equal", "1980-05-17", "1980-05-17", 0, 1},
		{"real differ", "1980-05-17", "1981-05-17", 0, 0},
		{"both 1 January", "1980-01-01", "1980-01-01", 1, nan},
		{"one 1 July", "1980-07-01", "1980-05-17", 1, nan},
		{"day 00", "1980-05-00", "1980-05-17", 1, nan},
		{"both day 00", "1980-05-00", "1980-05-00", 1, nan},
		{"day 00 with invalid month", "1980-13-00", "1980-05-17", nan, nan},
		{"missing", "", "1980-05-17", nan, nan},
		{"invalid", "1980-02-30", "1980-05-17", nan, nan},
		{"sentinel placeholder", "0001-01-01", "0001-01-01", nan, nan},
		{"wrong length", "1980-5-17", "1980-05-17", nan, nan},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, b := &Rec{Dob: c.da, skel: []string{"slh"}}, &Rec{Dob: c.db, skel: []string{"slh"}}
			for _, f := range [][]float64{pairfeat6Feats(a, b, idf6()), pairfeat6Feats(b, a, idf6())} {
				if len(f) != 4 || !eqNaN(f[0], c.f30) || !eqNaN(f[1], c.f31) {
					t.Errorf("f30=%v f31=%v, want %v %v", f[0], f[1], c.f30, c.f31)
				}
			}
		})
	}
}

func TestPairfeat6NameFeatures(t *testing.T) {
	nan := math.NaN()
	for _, c := range []struct {
		name     string
		sa, sb   []string
		f32, f33 float64
	}{
		{"common shared, rare and unseen unmatched", []string{"mhmd", "slh"}, []string{"mhmd", "krm"}, 0, math.Log(100)},
		{"all match, repeats count once", []string{"ahmd", "ahmd", "slh"}, []string{"slh", "ahmd"}, math.Log(2) + math.Log(10), 0},
		{"disjoint", []string{"zbr"}, []string{"ahmd"}, 0, math.Log(50)},
		{"negative weights: the largest, not clamped", []string{"cmn", "mhmd"}, []string{"cmm", "mhmd"}, 0, math.Log(100.0 / 101.0)},
		{"empty skeleton", nil, []string{"slh"}, nan, nan},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, b := &Rec{Dob: "1980-05-17", skel: c.sa}, &Rec{Dob: "1980-05-17", skel: c.sb}
			ab, ba := pairfeat6Feats(a, b, idf6()), pairfeat6Feats(b, a, idf6())
			for i := 2; i < 4; i++ {
				if !eqNaN(ab[i], ba[i]) {
					t.Errorf("f%d not symmetric: %v vs %v", 30+i, ab[i], ba[i])
				}
			}
			if !eqNaN(ab[2], c.f32) || !eqNaN(ab[3], c.f33) {
				t.Errorf("f32=%v f33=%v, want %v %v", ab[2], ab[3], c.f32, c.f33)
			}
		})
	}
}

func TestPairFeatsPairfeat6Appended(t *testing.T) {
	a := &Rec{Eid: "784198020775241", Dob: "1980-01-01", Sex: "F", isAr: true, lat: "mona saleh", tok: []string{"mona", "saleh"}, skel: []string{"mn", "slh"}}
	b := &Rec{Eid: "784198020775231", Dob: "1980-01-01", Sex: "F", lat: "mona salih", tok: []string{"mona", "salih"}, skel: []string{"mn", "slh"}}
	setNameIDF(t, nil)
	base := pairFeats(a, b, 0.5, 0.25)
	if len(base) != NFEAT {
		t.Fatalf("NameIDF nil: len %d, want %d", len(base), NFEAT)
	}
	setNameIDF(t, idf6())
	f := pairFeats(a, b, 0.5, 0.25)
	if len(f) != NFEAT6 {
		t.Fatalf("NameIDF set: len %d, want %d", len(f), NFEAT6)
	}
	for i := range base {
		if !eqNaN(f[i], base[i]) {
			t.Errorf("f%d=%v changed (was %v)", i, f[i], base[i])
		}
	}
	want := []float64{1, math.NaN(), math.Log(100) + math.Log(10), 0}
	for i, w := range want {
		if !eqNaN(f[NFEAT+i], w) {
			t.Errorf("f%d=%v want %v", NFEAT+i, f[NFEAT+i], w)
		}
	}
}

func TestNameIDFLoad(t *testing.T) {
	d := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(d, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good.json", `{"version":1,"split":"train","persons":5000,"seeds":[1019],"noise":"v1t","registrations":10,"counts":{"mhmd":10,"slh":1}}`)
	tb := LoadNameIDF(good)
	if tb.Registrations != 10 || tb.Counts["slh"] != 1 || tb.Weight("x") != math.Log(10) || tb.Split != "train" {
		t.Fatalf("loaded %+v", tb)
	}
	for name, body := range map[string]string{
		"version":       `{"version":2,"registrations":10,"counts":{}}`,
		"zero n":        `{"version":1,"registrations":0,"counts":{}}`,
		"no counts":     `{"version":1,"registrations":10}`,
		"count above n": `{"version":1,"registrations":10,"counts":{"a":11}}`,
		"zero count":    `{"version":1,"registrations":10,"counts":{"a":0}}`,
		"empty token":   `{"version":1,"registrations":10,"counts":{"":1}}`,
		"malformed":     `{"version":1,`,
	} {
		if _, err := readNameIDF(write(strings.ReplaceAll(name, " ", "_")+".json", body)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	missing := filepath.Join(d, "missing.json")
	if _, err := readNameIDF(missing); err == nil {
		t.Error("missing file accepted")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("LoadNameIDF did not panic on a missing file")
			}
		}()
		LoadNameIDF(missing)
	}()
}

func TestNameIDFFor(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "name_idf_pairfeat6.json")
	if err := os.WriteFile(p, []byte(`{"version":1,"registrations":5,"counts":{"a":2}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		cmd, path, ms string
		load, err     bool
	}{
		{"featdump", p, "", true, false},
		{"featdump", "", "", false, false},
		{"featdump", "", "pairfeat6", false, false},
		{"orchestrate", "", "pairfeat6", true, false},
		{"audit", "", "pairfeat6", true, false},
		{"report7", "", "pairfeat6", true, false},
		{"orchestrate", "", "pairfeat5", false, false},
		{"orchestrate", "", "", false, false},
		{"orchestrate", p, "pairfeat6", false, true},
		{"gen", p, "", false, true},
	} {
		tb, err := nameIDFFor(c.cmd, c.path, c.ms, d)
		if (err != nil) != c.err || (tb != nil) != c.load {
			t.Errorf("nameIDFFor(%q, %q, %q) = %v, %v; want load=%v err=%v", c.cmd, c.path, c.ms, tb, err, c.load, c.err)
		}
	}
	if _, err := nameIDFFor("orchestrate", "", "pairfeat6", t.TempDir()); err == nil {
		t.Error("pairfeat6 without a table in -wasl accepted")
	}
}

func TestPairfeat6PolicyAndCap(t *testing.T) {
	for _, p := range []string{"", "v6", "v7ltt"} {
		if err := validateModelPolicy("pairfeat6", p); err == nil || !strings.Contains(err.Error(), "pairfeat6") {
			t.Errorf("validateModelPolicy(pairfeat6, %q) = %v, want an error naming pairfeat6", p, err)
		}
	}
	if err := validateModelPolicy("pairfeat6", "v7strict"); err != nil {
		t.Error(err)
	}
	for ms, want := range map[string]bool{"": false, "pairfeat1": false, "pairfeat2": false, "pairfeat3": false,
		"pairfeat4": false, "pairfeat5": true, "pairfeat6": true} {
		if got := xscriptCapFor(ms); got != want {
			t.Errorf("xscriptCapFor(%q) = %v, want %v", ms, got, want)
		}
	}
}

func TestPairfeat6OffFeatDumpGolden(t *testing.T) {
	setNameIDF(t, nil)
	old := UrduFix
	UrduFix = true // the golden was taken with the CLI, which turns the Urdu fix on
	defer func() { UrduFix = old }()
	c, s := runFD2(t, "train", 1019, 0.1, 0.1)
	if hex256(c) != goldenTrainNsFam1019 || hex256(s) != goldenTrainNsFam1019Sc {
		t.Errorf("train 1019 -namesakes 0.1 -familynamesakes 0.1: csv %s sidecar %s", hex256(c), hex256(s))
	}
}
```

- [ ] **Step 2: RED.** `cd pipeline && go test . -run 'Pairfeat6|NameIDF|PairFeatsPairfeat6' -count=1` fails to compile (undefined `NameIDFTable`, `pairfeat6Feats`, `xscriptCapFor`).
- [ ] **Step 3: Implement.** Create `pipeline/pairfeat6.go`:

```go
package main

// Modelset pairfeat6 pair features f30..f33, appended after the legacy and pairfeat1 features 0..29 only when a
// name-rarity table is loaded (NameIDF: -modelset pairfeat6 at runtime, featdump -nameidf when training pairfeat6).
// Every other modelset keeps the 30-feature vector byte for byte.
//   f30 placeholder date: NaN if either DOB is unusable (missing, invalid, sentinel; a day-00 date with a real year and
//       month is usable); else 1 if either is a 1 January / 1 July placeholder or has day 00; else 0.
//   f31 real exact date: 1 if both DOBs are real, non-placeholder and equal; 0 if both are and differ; NaN otherwise.
//   f32 shared-token rarity: sum of the rarity weights of the distinct skeleton tokens present in both records.
//   f33 rarest unmatched token: the largest rarity weight among distinct skeleton tokens present on one side only
//       (negative weights are not clamped); 0 when every token matches. f32 and f33 are NaN when a skeleton is empty.
// Rarity weight of token t: log(N / (1 + c(t))), N training registrations and c(t) those whose skeleton contains t
// (wsi nameidf, encdev.go); an unseen token has c = 0. Tokens are summed and compared in sorted order.
import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NFEAT6: the pairfeat6 feature count (NFEAT plus f30..f33).
const NFEAT6 = 34

// NameIDF: the pairfeat6 name-rarity table; nil (the default) leaves pairFeats at NFEAT features.
var NameIDF *NameIDFTable

// NameIDFTable: models/name_idf_pairfeat6.json, written by wsi nameidf.
type NameIDFTable struct {
	Version       int            `json:"version"`
	Split         string         `json:"split"`
	Persons       int            `json:"persons"`
	Seeds         []int64        `json:"seeds"`
	Noise         string         `json:"noise"`
	Registrations int            `json:"registrations"`
	Counts        map[string]int `json:"counts"`
}

func readNameIDF(path string) (*NameIDFTable, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t NameIDFTable
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if t.Version != 1 || t.Registrations <= 0 || t.Counts == nil {
		return nil, fmt.Errorf("%s: want version 1, registrations > 0 and a counts map", path)
	}
	for k, c := range t.Counts {
		if k == "" || c < 1 || c > t.Registrations {
			return nil, fmt.Errorf("%s: count %d for token %q outside 1..%d", path, c, k, t.Registrations)
		}
	}
	return &t, nil
}

// LoadNameIDF: readNameIDF, panicking on error (as LoadGBM does).
func LoadNameIDF(path string) *NameIDFTable {
	t, err := readNameIDF(path)
	if err != nil {
		panic(err)
	}
	return t
}

// Weight: log(N / (1 + c(t))).
func (t *NameIDFTable) Weight(tok string) float64 {
	return math.Log(float64(t.Registrations) / float64(1+t.Counts[tok]))
}

// nameIDFFor: the table a process uses. featdump takes it only from -nameidf (pairfeat6 training dumps); every other
// command loads <wasl>/name_idf_pairfeat6.json when -modelset is pairfeat6 and uses none otherwise. -nameidf on any
// command other than featdump is a configuration error.
func nameIDFFor(cmd, path, modelSet, wasl string) (*NameIDFTable, error) {
	if cmd == "featdump" {
		if path == "" {
			return nil, nil
		}
		return readNameIDF(path)
	}
	if path != "" {
		return nil, fmt.Errorf("-nameidf is valid only for featdump (got command %q)", cmd)
	}
	if modelSet == "pairfeat6" {
		return readNameIDF(filepath.Join(wasl, "name_idf_pairfeat6.json"))
	}
	return nil, nil
}

func dobUsable6(d string) bool {
	return realDob(d) || (len(d) == 10 && d[8:] == "00" && realDob(d[:8]+"01"))
}

func dobPlaceholder6(d string) bool {
	return len(d) == 10 && (strings.HasSuffix(d, "-01-01") || strings.HasSuffix(d, "-07-01") || d[8:] == "00")
}

// pairfeat6Feats returns f30..f33 (see the file comment).
func pairfeat6Feats(a, b *Rec, t *NameIDFTable) []float64 {
	nan := math.NaN()
	out := []float64{nan, nan, nan, nan}
	if dobUsable6(a.Dob) && dobUsable6(b.Dob) {
		ph := dobPlaceholder6(a.Dob) || dobPlaceholder6(b.Dob)
		out[0] = boolFloat(ph)
		if !ph { // usable and not a placeholder: both are real dates
			out[1] = boolFloat(a.Dob == b.Dob)
		}
	}
	if len(a.skel) > 0 && len(b.skel) > 0 {
		set := func(s []string) map[string]bool {
			m := map[string]bool{}
			for _, x := range s {
				m[x] = true
			}
			return m
		}
		sa, sb := set(a.skel), set(b.skel)
		all := []string{}
		for x := range sa {
			all = append(all, x)
		}
		for x := range sb {
			if !sa[x] {
				all = append(all, x)
			}
		}
		sort.Strings(all)
		shared, rarest, unmatched := 0.0, 0.0, false
		for _, x := range all {
			w := t.Weight(x)
			if sa[x] && sb[x] {
				shared += w
			} else if !unmatched || w > rarest {
				rarest, unmatched = w, true
			}
		}
		out[2], out[3] = shared, rarest
	}
	return out
}
```

  In `pipeline/text.go` `pairFeats`, after line 421:

```go
	if NameIDF != nil { // modelset pairfeat6 only (pairfeat6.go); every other modelset stays at NFEAT features
		f = append(f, pairfeat6Feats(a, b, NameIDF)...)
	}
```

  In `pipeline/xscript.go`, replace the line-9 comment with `// XScriptCap: set in main.go from xscriptCapFor(ModelSet) after -modelset passes validateModelPolicy.` and add:

```go
// xscriptCapFor: the cap is on for pairfeat5 and pairfeat6 (pairfeat6 keeps every pairfeat5 setting), off otherwise.
func xscriptCapFor(modelSet string) bool { return modelSet == "pairfeat5" || modelSet == "pairfeat6" }
```

  In `pipeline/encdev.go` `validateModelPolicy`, after the `pairfeat5` block (lines 39–41):

```go
	if modelSet == "pairfeat6" && policy != "v7strict" {
		return fmt.Errorf("modelset pairfeat6 supports only -policy v7strict (got %q); it keeps pairfeat5's cross-script review cap, selected under v7strict", policy)
	}
```

  In `pipeline/main.go`, after line 177 add `nameIDFPath := fs.String("nameidf", "", "featdump only: pairfeat6 name-rarity table (wsi nameidf output); adds pair features f30-f33 (pairfeat6.go)")`, and replace line 268 with:

```go
	XScriptCap = xscriptCapFor(ModelSet) // xscript.go: cross-script review cap (pairfeat5, pairfeat6)
	if tb, err := nameIDFFor(cmd, *nameIDFPath, ModelSet, *wasl); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	} else {
		NameIDF = tb // pairfeat6.go: non-nil only for -modelset pairfeat6 or featdump -nameidf
	}
```

- [ ] **Step 4: GREEN.** `cd pipeline && go test . -count=1 && go vet .` (all suites, including `TestPairfeat6OffFeatDumpGolden`, `TestNamesakeFeatDumpRateZeroGolden`, `TestFamilyFeatDumpRateZeroGoldens`, `TestXScript*`).
- [ ] **Step 5: Commit.**

```bash
git add pipeline/pairfeat6.go pipeline/pairfeat6_test.go
git commit -m "pairfeat6: placeholder-date and name-rarity pair features f30-f33 (modelset pairfeat6 only)" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" \
  -- pipeline/pairfeat6.go pipeline/pairfeat6_test.go pipeline/text.go pipeline/main.go pipeline/encdev.go pipeline/xscript.go
```

### Task B: rarity-table build and cross-script hard negatives in featdump (Go)

**Files:**
- Create `pipeline/xsnamesake_test.go`.
- Modify:
  - `pipeline/encdev.go`: header comment (lines 3–7), `validateNoise` (lines 47–52), `runFeatDump` (lines 133–320), new `fdReg`, `readPersons`, `fdSample`, `fdName`, `fdRegister`, `runNameIDF`, `parseIDFSeeds`; import `strconv`;
  - `pipeline/namesake.go`: `XSNamesakeRate`, `xsSeed`, `validateXSNamesakes`, `buildXSNamesakes`, `otherTokensDisjoint`, `xsRegister`;
  - `pipeline/main.go`: flags next to line 149 and 177, validation after line 205, `case "nameidf"` after line 559.

**Interfaces:**
- Consumes (Task A): `NameIDF`, `NameIDFTable`, `LoadNameIDF`, `NFEAT6`, flag `-nameidf` (`*nameIDFPath`).
- Produces:
  - `type fdReg struct { pid int; name, dob, sex, eid string }`
  - `func readPersons(wasl string) []Person`
  - `func fdSample(all []Person, split string, nPersons int, seed int64) (ps, pool []Person, r *rand.Rand)`
  - `func fdName(p *Person, arabic bool, r *rand.Rand) string`
  - `func fdRegister(regs []fdReg, pid int, p *Person, r *rand.Rand) []fdReg`
  - `func runNameIDF(wasl, split, outPath string, nPersons int, seeds []int64)`; command `wsi nameidf -wasl D -split train -persons 5000 -noise v1t -idfseeds 1019,1021,1031,1033,1039 -out PATH`
  - `func parseIDFSeeds(s string) []int64`
  - `var XSNamesakeRate float64` (flag `-xsnamesakes`), `func xsSeed(seed int64) int64` = `seed*2654435761 + 155921`
  - `func validateXSNamesakes(cmd string, rate float64, nameIDF string) error`
  - `func buildXSNamesakes(ps, pool []Person, rate float64, r *rand.Rand) ([]Person, []int)`, `func otherTokensDisjoint(a, b []string) bool`
  - `func xsRegister(regs []fdReg, bpid int, b *Person, xpid int, x *Person, r *rand.Rand) []fdReg`
  - Sidecar keys, only when `-xsnamesakes > 0`: `xs_rate` (float), `xs_namesakes_added` (int), `xs_base_namesake_rows` (int), `xs_rows` ([]int).
  - With `-nameidf`, CSV rows are `2 + 34 + 5 = 41` columns (42 with `-gbmfile`).

- [ ] **Step 1: Failing tests.** Create `pipeline/xsnamesake_test.go`:

```go
package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestXSNamesakeValidate(t *testing.T) {
	for _, c := range []struct {
		cmd  string
		rate float64
		idf  string
		ok   bool
	}{
		{"featdump", 0, "", true}, {"featdump", 0.1, "t.json", true}, {"featdump", 0.1, "", false},
		{"gen", 0.1, "t.json", false}, {"featdump", 1, "t.json", false}, {"featdump", -0.1, "t.json", false},
		{"orchestrate", 0, "", true},
	} {
		if err := validateXSNamesakes(c.cmd, c.rate, c.idf); (err == nil) != c.ok {
			t.Errorf("validateXSNamesakes(%q, %v, %q) = %v, want ok=%v", c.cmd, c.rate, c.idf, err, c.ok)
		}
	}
	if xsSeed(1019) == namesakeSeed(1019) || xsSeed(1019) == familySeed(1019) {
		t.Error("cross-script namesakes share a generator seed with another kind")
	}
}

func TestValidateNoiseNameIDF(t *testing.T) {
	if err := validateNoise("nameidf", "v1t"); err != nil {
		t.Error(err)
	}
	if validateNoise("gen", "v1t") == nil || validateNoise("orchestrate", "v1t") == nil {
		t.Error("v1t accepted outside featdump/nameidf")
	}
}

func TestParseIDFSeeds(t *testing.T) {
	if got := parseIDFSeeds("1019, 1021,,1031"); !reflect.DeepEqual(got, []int64{1019, 1021, 1031}) {
		t.Fatalf("got %v", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("bad seed list accepted")
		}
	}()
	parseIDFSeeds("1019,x")
}

func xsTestPool() []Person {
	return []Person{
		{Qid: "b1", Ar: "محمد سالم", En: "Mohammed Salem", Dob: "1975-06-14", Sex: "M", Spl: "train", Origin: "ae"},
		{Qid: "d1", Ar: "محمد كريم ناصر", En: "Mohammed Karim Nasser", Dob: "1980-02-02", Sex: "M", Spl: "train", Origin: "eg"},
		{Qid: "d2", Ar: "محمد سالم حمد", En: "Mohammed Salem Hamad", Dob: "1981-03-03", Sex: "M", Spl: "train"},
		{Qid: "d3", Ar: "محمد علي", En: "Mohammed Ali", Dob: "1982-04-04", Sex: "F", Spl: "train"},
		{Qid: "b2", Ar: "سارة", En: "Sara", Dob: "1990-05-05", Sex: "F", Spl: "train"},
		{Qid: "b3", Ar: "فاطمة علي", En: "Fatima Ali", Dob: "1991-05-05", Sex: "F", Spl: "train"},
	}
}

func TestOtherTokensDisjoint(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"mohammed salem", "mohammed karim nasser", true},
		{"mohammed salem", "mohammed salem hamad", false},
		{"mohammed salem", "mohammed hamad salem", false},
		{"mohammed salem mohammed", "mohammed karim", false},
		{"mohammed karim", "mohammed salem mohammed", false},
	} {
		if got := otherTokensDisjoint(strings.Fields(c.a), strings.Fields(c.b)); got != c.want {
			t.Errorf("otherTokensDisjoint(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestBuildXSNamesakes(t *testing.T) {
	pool := xsTestPool()
	xs, base := buildXSNamesakes(pool[:1], pool, 0.999999, rand.New(rand.NewSource(1)))
	if len(xs) != 1 || !reflect.DeepEqual(base, []int{0}) {
		t.Fatalf("xs %+v base %v", xs, base)
	}
	x := xs[0]
	if x.Qid != "xs-b1" || x.En != "Mohammed Karim Nasser" || x.Ar != "محمد كريم ناصر" || x.Dob != "1975-01-01" ||
		x.Sex != "M" || x.Origin != "eg" || x.Spl != "train" {
		t.Fatalf("namesake %+v", x)
	}
	if !luhnOK(x.Eid) || !strings.HasPrefix(x.Eid, "7841975") {
		t.Fatalf("namesake Emirates ID %q", x.Eid)
	}
	// single-token name; given name with no other bearer; only donors of the other sex
	for _, ps := range [][]Person{pool[4:5], pool[5:6], pool[3:4]} {
		if got, _ := buildXSNamesakes(ps, pool, 0.999999, rand.New(rand.NewSource(1))); len(got) != 0 {
			t.Errorf("%s got namesake %+v", ps[0].Qid, got)
		}
	}
	r, fresh := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	if got, _ := buildXSNamesakes(pool, pool, 0, r); len(got) != 0 || r.Int63() != fresh.Int63() {
		t.Fatal("rate 0 built namesakes or consumed random draws")
	}
	a, _ := buildXSNamesakes(pool, pool, 0.5, rand.New(rand.NewSource(7)))
	b, _ := buildXSNamesakes(pool, pool, 0.5, rand.New(rand.NewSource(7)))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("buildXSNamesakes is not deterministic")
	}
}

func TestXSRegister(t *testing.T) {
	old := NoiseModel
	NoiseModel = "v1t"
	defer func() { NoiseModel = old }()
	pool := xsTestPool()
	b := pool[0]
	x := Person{Qid: "xs-b1", Ar: pool[1].Ar, En: pool[1].En, Dob: "1975-01-01", Sex: "M", Eid: newEID("1975-01-01", rand.New(rand.NewSource(2)))}
	for seed := int64(1); seed <= 20; seed++ {
		regs := xsRegister(nil, 3, &b, 10, &x, rand.New(rand.NewSource(seed)))
		if len(regs) != 2 || regs[0].pid != 3 || regs[1].pid != 10 {
			t.Fatalf("seed %d: regs %+v", seed, regs)
		}
		if regs[0].dob != "1975-01-01" || regs[1].dob != "1975-01-01" || regs[0].sex != "M" || regs[1].sex != "M" {
			t.Fatalf("seed %d: date or sex noise %+v", seed, regs)
		}
		if isAr(regs[0].name) == isAr(regs[1].name) {
			t.Fatalf("seed %d: same script %q / %q", seed, regs[0].name, regs[1].name)
		}
	}
}

func TestNameIDFBuild(t *testing.T) {
	old := NoiseModel
	NoiseModel = "v1t"
	defer func() { NoiseModel = old }()
	d := t.TempDir()
	p, q := filepath.Join(d, "a.json"), filepath.Join(d, "b.json")
	seeds := []int64{1019, 1021}
	runNameIDF("../data", "train", p, 400, seeds)
	runNameIDF("../data", "train", q, 400, seeds)
	pb, _ := os.ReadFile(p)
	qb, _ := os.ReadFile(q)
	if len(pb) == 0 || !bytes.Equal(pb, qb) {
		t.Fatal("nameidf is empty or not deterministic")
	}
	tb := LoadNameIDF(p)
	all := readPersons("../data")
	n, counts := 0, map[string]int{}
	for _, seed := range seeds {
		ps, _, r := fdSample(all, "train", 400, seed)
		var regs []fdReg
		for pid := range ps {
			regs = fdRegister(regs, pid, &ps[pid], r)
		}
		n += len(regs)
		for _, g := range regs {
			seen := map[string]bool{}
			for _, s := range skeleton(tokensOf(latinForm(g.name))) {
				if !seen[s] {
					seen[s] = true
					counts[s]++
				}
			}
		}
	}
	if tb.Registrations != n || !reflect.DeepEqual(tb.Counts, counts) {
		t.Fatalf("table N=%d (%d tokens), recount N=%d (%d tokens)", tb.Registrations, len(tb.Counts), n, len(counts))
	}
	if tb.Version != 1 || tb.Split != "train" || tb.Persons != 400 || tb.Noise != "v1t" || !reflect.DeepEqual(tb.Seeds, seeds) {
		t.Fatalf("table header %+v", tb)
	}
	if n < 1200 {
		t.Fatalf("only %d base registrations for 2 x 400 persons", n)
	}
}

func intList(t *testing.T, v any) []int {
	t.Helper()
	xs, ok := v.([]any)
	if !ok {
		t.Fatalf("not a list: %v", v)
	}
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = int(x.(float64))
	}
	return out
}

func xsFD(t *testing.T, xs float64, tb *NameIDFTable) (rows [][]string, sc map[string]any, csvB, scB []byte) {
	t.Helper()
	oldN, oldR, oldF, oldX, oldI := NoiseModel, NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF
	NoiseModel, NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF = "v1t", 0.1, 0.1, xs, tb
	defer func() { NoiseModel, NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF = oldN, oldR, oldF, oldX, oldI }()
	out := filepath.Join(t.TempDir(), "o.csv")
	runFeatDump("../data", "../models/encoder_weights.json", "", "train", out, 400, 1019)
	var err error
	if csvB, err = os.ReadFile(out); err != nil {
		t.Fatal(err)
	}
	if scB, err = os.ReadFile(out + ".namesakes.json"); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(scB, &sc); err != nil {
		t.Fatal(err)
	}
	if rows, err = csv.NewReader(bytes.NewReader(csvB)).ReadAll(); err != nil {
		t.Fatal(err)
	}
	return
}

func TestXSFeatDump(t *testing.T) {
	old := NoiseModel
	NoiseModel = "v1t"
	p := filepath.Join(t.TempDir(), "name_idf_pairfeat6.json")
	runNameIDF("../data", "train", p, 400, []int64{1019})
	NoiseModel = old
	tb := LoadNameIDF(p)
	rows, sc, c1, s1 := xsFD(t, 0.1, tb)
	_, _, c2, s2 := xsFD(t, 0.1, tb)
	if !bytes.Equal(c1, c2) || !bytes.Equal(s1, s2) {
		t.Fatal("featdump with -xsnamesakes is not deterministic")
	}
	for i, r := range rows {
		if len(r) != 2+NFEAT6+5 {
			t.Fatalf("row %d has %d columns, want %d", i, len(r), 2+NFEAT6+5)
		}
	}
	added := int(sc["xs_namesakes_added"].(float64))
	xr := intList(t, sc["xs_rows"])
	if sc["xs_rate"].(float64) != 0.1 || added <= 0 || len(xr) == 0 || int(sc["xs_base_namesake_rows"].(float64)) != len(xr) {
		t.Fatalf("sidecar %v", sc)
	}
	other := map[int]bool{}
	for _, i := range append(intList(t, sc["rows"]), intList(t, sc["family_rows"])...) {
		other[i] = true
	}
	pattern := 0
	for _, i := range xr {
		if i < 0 || i >= len(rows) || rows[i][0] != "0" {
			t.Fatalf("xs row %d is not a label-0 row", i)
		}
		if other[i] {
			t.Fatalf("row %d is also a given- or family-name namesake row", i)
		}
		f := rows[i][2:] // f0..f33
		if f[20] == "0" && f[13] == "1" && f[19] == "1" && f[30] == "1" {
			pattern++
		}
	}
	if 2*pattern < added {
		t.Fatalf("only %d of %d cross-script namesakes gave the opposite-script, same-sex, same-placeholder-date row", pattern, added)
	}
	_, sc0, _, _ := xsFD(t, 0, tb)
	for _, k := range []string{"xs_rate", "xs_namesakes_added", "xs_base_namesake_rows", "xs_rows"} {
		if _, ok := sc0[k]; ok {
			t.Errorf("rate-0 sidecar carries %s", k)
		}
	}
}
```

- [ ] **Step 2: RED.** `cd pipeline && go test . -run 'XS|NameIDFBuild|ParseIDFSeeds|ValidateNoiseNameIDF|OtherTokens' -count=1` fails to compile.
- [ ] **Step 3: Implement.**

  `pipeline/namesake.go`, after `namesakeSeed` (line 31):

```go
var XSNamesakeRate float64 // flag -xsnamesakes; training-only (featdump with -nameidf: pairfeat6)

func xsSeed(seed int64) int64 { return seed*2654435761 + 155921 }

// validateXSNamesakes: -xsnamesakes in [0, 1), featdump only, and only with -nameidf (pairfeat6 training dumps).
func validateXSNamesakes(cmd string, rate float64, nameIDF string) error {
	if err := validateNamesakeRate("-xsnamesakes", cmd, rate); err != nil {
		return err
	}
	if rate > 0 && nameIDF == "" {
		return fmt.Errorf("-xsnamesakes needs -nameidf (cross-script hard negatives are pairfeat6 training data)")
	}
	return nil
}

// otherTokensDisjoint: no token after the first of either name occurs anywhere in the other name.
func otherTokensDisjoint(a, b []string) bool {
	set := func(s []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range s {
			m[x] = true
		}
		return m
	}
	sa, sb := set(a), set(b)
	for _, x := range a[1:] {
		if sb[x] {
			return false
		}
	}
	for _, x := range b[1:] {
		if sa[x] {
			return false
		}
	}
	return true
}

// buildXSNamesakes: the pairfeat6 cross-script hard negatives. For a share rate of ps, xs[i] is a different person
// whose first English token equals ps[base[i]]'s, whose other tokens share nothing with the base name (and the reverse),
// with the base person's sex and the 1 January placeholder of the base person's birth year. Donors come from pool (the
// split, file order). A base with a single-token name, no usable year or no donor gets none.
func buildXSNamesakes(ps, pool []Person, rate float64, r *rand.Rand) ([]Person, []int) {
	if rate <= 0 {
		return nil, nil
	}
	idx := map[string][]int{}
	for i := range pool {
		if t := strings.Fields(strings.ToLower(pool[i].En)); len(t) > 1 {
			idx[t[0]] = append(idx[t[0]], i)
		}
	}
	var xs []Person
	var base []int
	for bi := range ps {
		b := &ps[bi]
		if r.Float64() >= rate {
			continue
		}
		bt := strings.Fields(strings.ToLower(b.En))
		if len(bt) < 2 || len(b.Dob) != 10 {
			continue
		}
		dob := b.Dob[:4] + "-01-01"
		if !realDob(dob) {
			continue
		}
		var elig []int
		for _, j := range idx[bt[0]] {
			d := &pool[j]
			if d.Qid != b.Qid && d.Sex == b.Sex && otherTokensDisjoint(bt, strings.Fields(strings.ToLower(d.En))) {
				elig = append(elig, j)
			}
		}
		if len(elig) == 0 {
			continue
		}
		d := pool[elig[r.Intn(len(elig))]]
		xs = append(xs, Person{Qid: "xs-" + b.Qid, Ar: d.Ar, En: d.En, Als: d.Als, Dob: dob, Sex: b.Sex, Cc: d.Cc,
			Eid: newEID(dob, r), Spl: b.Spl, Origin: d.Origin, ArSrc: d.ArSrc})
		base = append(base, bi)
	}
	return xs, base
}

// xsRegister: the two registrations of a cross-script namesake: base person b in a drawn script and the namesake x in
// the other, both with x's placeholder DOB and their true sex (no DOB or sex noise) and a drawn Emirates ID.
func xsRegister(regs []fdReg, bpid int, b *Person, xpid int, x *Person, r *rand.Rand) []fdReg {
	ar := r.Float64() < .5
	regs = append(regs, fdReg{bpid, fdName(b, ar, r), x.Dob, b.Sex, drawEID(b, r)})
	return append(regs, fdReg{xpid, fdName(x, !ar, r), x.Dob, x.Sex, drawEID(x, r)})
}
```

  `pipeline/encdev.go`: add the line `//   wsi nameidf -wasl <data dir> -split train -persons 5000 -noise v1t -idfseeds 1019,... -out name_idf_pairfeat6.json   pairfeat6 name-rarity table` to the header comment; add `"strconv"` to the imports; change `validateNoise` to accept `nameidf`:

```go
func validateNoise(cmd, noise string) error {
	if noise == "v1t" && cmd != "featdump" && cmd != "nameidf" {
		return fmt.Errorf("-noise v1t is training-only and valid only for featdump and nameidf (got command %q)", cmd)
	}
	return nil
}
```

  Before `runFeatDump`, add:

```go
// fdReg: one featdump registration (person index, name, DOB, sex, Emirates ID).
type fdReg struct {
	pid                 int
	name, dob, sex, eid string
}

func readPersons(wasl string) []Person {
	b, err := os.ReadFile(filepath.Join(wasl, "persons3.json"))
	if err != nil {
		panic(err)
	}
	var all []Person
	if err := json.Unmarshal(b, &all); err != nil {
		panic(err)
	}
	return all
}

// fdSample: featdump's person sample: the split's persons with both names, shuffled with the seed and cut to nPersons,
// the split pool in file order, and the generator after the shuffle.
func fdSample(all []Person, split string, nPersons int, seed int64) ([]Person, []Person, *rand.Rand) {
	ps := []Person{}
	for _, p := range all {
		if p.Spl == split && p.Ar != "" && p.En != "" {
			ps = append(ps, p)
		}
	}
	pool := append([]Person(nil), ps...) // every person of the split, file order, before the shuffle
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(ps), func(i, j int) { ps[i], ps[j] = ps[j], ps[i] })
	if nPersons > 0 && nPersons < len(ps) {
		ps = ps[:nPersons]
	}
	return ps, pool, r
}

// fdName: one noisy registration name of p in the given script (gen.go noise: v2, else v1/v1t).
func fdName(p *Person, arabic bool, r *rand.Rand) string {
	if arabic {
		if NoiseModel == "v2" {
			return noisyArabic2(p.Ar, r)
		}
		return noisyArabic(p.Ar, r)
	}
	opts := append([]string{p.En}, p.Als...)
	src := p.En
	if r.Float64() < .5 {
		src = opts[r.Intn(len(opts))]
	}
	if NoiseModel == "v2" {
		return noisyLatin2(src, r)
	}
	return noisyLatin(src, r)
}

// fdRegister: p registers at 1-4 distinct providers (featdump's registration model); appended to regs.
func fdRegister(regs []fdReg, pid int, p *Person, r *rand.Rand) []fdReg {
	k := 1
	if x := r.Float64(); x < .1 {
		k = 4
	} else if x < .35 {
		k = 3
	} else if x < .7 {
		k = 2
	}
	for _, pi := range r.Perm(len(Providers))[:k] {
		prov := Providers[pi]
		name := fdName(p, r.Float64() < prov.PAr, r)
		var dob string
		if NoiseModel == "v2" {
			dob = noisyDob2(p.Dob, r, .3)
		} else if NoiseModel == "v1t" {
			dob = noisyDobV1T(p.Dob, r, .3)
		} else {
			dob = noisyDob(p.Dob, r, .3)
		}
		sex := p.Sex
		if r.Float64() < .02 {
			sex = "U"
		}
		regs = append(regs, fdReg{pid, name, dob, sex, drawEID(p, r)})
	}
	return regs
}

// runNameIDF: the pairfeat6 name-rarity table (pairfeat6.go). For each seed it draws featdump's person sample and base
// registrations exactly as featdump does (same code and RNG sequence; no namesake or cross-script registrations) and
// counts, per skeleton token, the registrations whose skeleton contains it; N is the registration total.
func runNameIDF(wasl, split, outPath string, nPersons int, seeds []int64) {
	if len(seeds) == 0 {
		panic("nameidf: -idfseeds is required")
	}
	all := readPersons(wasl)
	t := NameIDFTable{Version: 1, Split: split, Persons: nPersons, Seeds: seeds, Noise: NoiseModel, Counts: map[string]int{}}
	for _, seed := range seeds {
		ps, _, r := fdSample(all, split, nPersons, seed)
		var regs []fdReg
		for pid := range ps {
			regs = fdRegister(regs, pid, &ps[pid], r)
		}
		for _, g := range regs {
			seen := map[string]bool{}
			for _, s := range skeleton(tokensOf(latinForm(g.name))) {
				if !seen[s] {
					seen[s] = true
					t.Counts[s]++
				}
			}
			t.Registrations++
		}
	}
	b, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(outPath, append(b, '\n'), 0o644); err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "nameidf %s: %d registrations, %d tokens\n", split, t.Registrations, len(t.Counts))
}

// parseIDFSeeds: -idfseeds "1019,1021,..." (empty items skipped).
func parseIDFSeeds(s string) []int64 {
	var out []int64
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x == "" {
			continue
		}
		v, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			panic("bad -idfseeds " + s)
		}
		out = append(out, v)
	}
	return out
}
```

  In `runFeatDump`:
  - Update the CSV comment (lines 136–137) to add: "with -nameidf (pairfeat6) f0..f33: 41 columns, 42 with p".
  - Replace lines 143–216 (persons read, filter, pool, shuffle, `regT`, the `register` closure, the base loop) with:

```go
	ps, pool, r := fdSample(readPersons(wasl), split, nPersons, seed)
	regs := []fdReg{}
	register := func(pid int, p *Person, r *rand.Rand) { regs = fdRegister(regs, pid, p, r) }
	for pid := range ps {
		register(pid, &ps[pid], r)
	}
```

  - After the family block (after line 238 `}`), before `r.Shuffle(len(regs), ...)`:

```go
	xsBase := map[int]int{}
	xsAdded := 0
	if XSNamesakeRate > 0 { // pairfeat6: base and cross-script namesake registrations (namesake.go)
		xr := rand.New(rand.NewSource(xsSeed(seed)))
		xs, xbase := buildXSNamesakes(ps, pool, XSNamesakeRate, xr)
		first := len(ps) + nsAdded + nfAdded
		for i := range xs {
			regs = xsRegister(regs, xbase[i], &ps[xbase[i]], first+i, &xs[i], xr)
			xsBase[first+i] = xbase[i]
		}
		xsAdded = len(xs)
	}
```

  - Line 249: `nsRows, nfRows, xsRows := []int{}, []int{}, []int{}`; after the `nfRows` append block (line 270–272) add:

```go
				if namesakePairRow(xsBase, g.pid, pids[c.j]) {
					xsRows = append(xsRows, rowIdx)
				}
```

  - After the family sidecar block (line 312–315), before `sc, err := json.Marshal(scm)`:

```go
	if XSNamesakeRate > 0 {
		scm["xs_rate"], scm["xs_namesakes_added"] = XSNamesakeRate, xsAdded
		scm["xs_base_namesake_rows"], scm["xs_rows"] = len(xsRows), xsRows
	}
```

  `pipeline/main.go`:
  - after line 149: `fs.Float64Var(&XSNamesakeRate, "xsnamesakes", 0, "featdump only, with -nameidf: share of persons given a synthetic cross-script namesake hard negative (pairfeat6 training only)")`;
  - after the `nameIDFPath` line (Task A): `idfSeeds := fs.String("idfseeds", "", "nameidf: comma-separated featdump seeds whose base registrations the rarity table counts")`;
  - after the `-familynamesakes` validation block (line 202–205):

```go
	if err := validateXSNamesakes(cmd, XSNamesakeRate, *nameIDFPath); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
```

  - after `case "featdump":` (lines 558–559): `case "nameidf":` / `runNameIDF(*wasl, *split, *out, *persons, parseIDFSeeds(*idfSeeds))`.
- [ ] **Step 4: GREEN.** `cd pipeline && go test . -count=1 && go vet .` (the Task A golden, the existing namesake goldens and the new tests). Then the CLI byte-identity and pairfeat6 sanity check at full size:

```bash
ID=~/wasl-ml-data/agents2/hardening/pairfeat6/taskB-identity
mkdir -p $ID/base-src
git archive 931a823 pipeline | tar -x -C $ID/base-src
(cd $ID/base-src/pipeline && go build -o $ID/wsi7.base .)
(cd pipeline && go build -o $ID/wsi7.head .)
for v in base head; do
  $ID/wsi7.$v featdump -wasl "$PWD/data" -encfile "$PWD/models/encoder_weights.json" -split train -persons 5000 \
    -noise v1t -seed 1019 -namesakes 0.1 -familynamesakes 0.1 -out $ID/train1019.$v.csv 2> $ID/train1019.$v.log
  $ID/wsi7.$v featdump -wasl "$PWD/data" -encfile "$PWD/models/encoder_weights.json" -split cal -noise v1t -seed 2011 \
    -gbmfile "$PWD/models/lgb_stream_pairfeat5.json" -out $ID/cal2011.$v.csv 2> $ID/cal2011.$v.log
done
shasum -a 256 $ID/train1019.*.csv $ID/train1019.*.csv.namesakes.json $ID/cal2011.*.csv
$ID/wsi7.head nameidf -wasl "$PWD/data" -split train -persons 5000 -noise v1t -idfseeds 1019 -out $ID/idf1019.json
$ID/wsi7.head featdump -wasl "$PWD/data" -encfile "$PWD/models/encoder_weights.json" -split train -persons 5000 -noise v1t \
  -seed 1019 -namesakes 0.1 -familynamesakes 0.1 -xsnamesakes 0.1 -nameidf $ID/idf1019.json -out $ID/train1019.pf6.csv
python3 -I - $ID/train1019.pf6.csv <<'PY'
import csv, json, sys
p = sys.argv[1]
widths = {len(r) for r in csv.reader(open(p, newline=""))}
sc = json.load(open(p + ".namesakes.json"))
print(json.dumps({"widths": sorted(widths), "xs_namesakes_added": sc["xs_namesakes_added"],
                  "xs_base_namesake_rows": sc["xs_base_namesake_rows"]}))
PY
```

  Expected: base and head hashes pairwise equal (train CSV, train sidecar, cal CSV); widths `[41]`; `xs_base_namesake_rows` > 100 (five seeds must reach the 500 floor; report the value). Stop and report if a hash differs.
- [ ] **Step 5: Commit.**

```bash
git add pipeline/xsnamesake_test.go
git commit -m "pairfeat6: name-rarity table build (wsi nameidf) and cross-script hard negatives (featdump -xsnamesakes)" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" \
  -- pipeline/xsnamesake_test.go pipeline/encdev.go pipeline/namesake.go pipeline/main.go
```

### Task C: the pairfeat6 fit protocol (Python)

**Files:** Modify `encoder/fit_scorer.py`, `encoder/test_fit_scorer.py`, `encoder/fit_pairfeat1.py`, `encoder/test_fit_pairfeat1.py`, `.gitignore`.

**Interfaces:**
- Consumes (Tasks A, B): `wsi nameidf` argv and output; `featdump -nameidf PATH -xsnamesakes RATE`; 41/42-column CSVs; sidecar keys `xs_namesakes_added`, `xs_base_namesake_rows`, `xs_rows`.
- Produces (`encoder/fit_scorer.py`): `NFEAT_CHOICES = (30, 34)`; `load(path, nfeat=NFEAT)`; flag `--nfeat {30,34}` (default 30); report key `"nfeat"` only when 34.
- Produces (`encoder/fit_pairfeat1.py`):
  - `PROTOCOLS["pairfeat6"] = dict(PROTOCOLS["pairfeat5"], xs_namesakes=0.10, nfeat=34)`
  - `nfeat(modelset) -> int`, `uses_name_idf(modelset) -> bool`, `csv_width(modelset, with_p) -> int`, `name_idf_name(modelset) -> str`
  - `chain_files("pairfeat6")` = the four files plus `name_idf_pairfeat6.json`
  - `nameidf_argv(wsi, data, modelset, out) -> list`
  - `train_argv(..., out, idf=None)`, `cal_argv(..., out, idf=None)`, `parity_argv(..., out, idf=None)`, `namesake_cal_argv(..., out, idf=None)`: `pairfeat6` requires `idf`, earlier modelsets refuse it
  - `fit_argv` adds `--nfeat 34` for `pairfeat6`
  - `check_namesake_floor` returns `(given, family, cross_script)` for `pairfeat6` (message `namesake floor not met (cross-script): N < 500`)
  - width keyword `width` on `_read_gbm_rows` (38), `_slice` / `namesake_slice` / `family_namesake_slice` / `xs_namesake_slice` / `plain_cal_diff_at_merge` / `read_p_column` (38) and `check_prefix_identity` (37)
  - `base_manifest(root, encoder, modelset, name_idf=None)`: `"nfeat"` from the protocol, `"name_idf_sha256"` and the code files `pipeline/pairfeat6.go`, `pipeline/namesake.go` for `pairfeat6` only
  - `training_manifest` adds `xs_namesakes`, per-seed `xs_namesakes_added` / `xs_base_namesake_rows`, `xs_namesake_base_rows_total`
  - `artifact_manifest(..., xs_namesake_slice=None, name_idf_sha256=None)`

- [ ] **Step 1: Failing tests.**

  `encoder/test_fit_scorer.py`: add after `LambdaL2Test`:

```python
class NFeatTest(unittest.TestCase):
    def _load(self, lines, nfeat):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "f.csv")
            with open(path, "w") as h:
                h.write("\n".join(lines) + "\n")
            return fs.load(path, nfeat)

    def test_34_features(self):
        y, corr, X, sl = self._load([_row(1, 1, feats=["0.5"] * 34, sl=(1, 0, 0, 1, 0))], 34)
        self.assertEqual(X.shape, (1, 34))
        self.assertTrue(sl["near_eid"][0] and sl["placeholder_dob"][0])
        y, corr, X, sl = self._load([_row(0, 0, feats=["0.5"] * 34, p="0.25")], 34)
        self.assertEqual(X.shape, (1, 34))

    def test_width_must_match_nfeat(self):
        with self.assertRaises(ValueError):
            self._load([_row()], 34)                         # a 37-column pairfeat5 row
        with self.assertRaises(ValueError):
            self._load([_row(feats=["0.5"] * 34)], 30)

    def test_default_unchanged(self):
        self.assertEqual(fs.NFEAT, 30)
        self.assertEqual(fs.NFEAT_CHOICES, (30, 34))
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "f.csv")
            with open(path, "w") as h:
                h.write(_row() + "\n")
            self.assertEqual(fs.load(path)[2].shape, (1, 30))

    def test_parse_args(self):
        base = ["--train", "a", "--cal", "c", "--outdir", "o", "--suffix", "s"]
        self.assertEqual(fs.parse_args(base).nfeat, 30)
        self.assertEqual(fs.parse_args(base + ["--nfeat", "34"]).nfeat, 34)
        for bad in ("31", "x", "0"):
            with self.assertRaises(SystemExit):
                fs.parse_args(base + ["--nfeat", bad])
```

  In `TinyFitTest`, give `_csvs(self, d, nfeat=30)` and `_run(self, d, sfx, extra, nfeat=30)` an `nfeat` parameter (`range(nfeat)` in the feature list; `_run` passes it to `_csvs`), and add:

```python
    def test_nfeat_34_report(self):
        try:
            import lightgbm  # noqa: F401
        except ImportError:
            self.skipTest("no lightgbm")
        with tempfile.TemporaryDirectory() as d:
            plain = self._run(d, "a", [])
            wide = self._run(d, "b", ["--nfeat", "34"], nfeat=34)
        self.assertNotIn("nfeat", plain)
        self.assertEqual(wide["nfeat"], 34)
        self.assertEqual(set(wide) - {"nfeat"}, set(plain))
```

  `encoder/test_fit_pairfeat1.py`:
  - `make_repo(d)` becomes `make_repo(d, names=FILES)` (writes and commits `names`);
  - `Protocols.test_constants`: the sorted list becomes `["pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4", "pairfeat5", "pairfeat6"]`;
  - add after `Pairfeat5`:

```python
PF6_FILES = ["pipeline/pairfeat6.go", "pipeline/namesake.go"]
EARLIER = ("pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4", "pairfeat5")


def go_row6(p=None):
    feats = ["%.17g" % (0.1 * i) for i in range(34)]
    r = ["0", "0"] + feats + ["0", "1", "0", "1", "1"]
    return r + (["%.17g" % p] if p is not None else [])


class Pairfeat6(unittest.TestCase):
    W, D, E, I = "/w/bin/wsi7", "/w/data", "/o/enc.json", "/o/chain/name_idf_pairfeat6.json"

    def test_protocol_and_widths(self):
        self.assertEqual(fp.PROTOCOLS["pairfeat6"], dict(fp.PROTOCOLS["pairfeat5"], xs_namesakes=0.1, nfeat=34))
        for k in EARLIER:
            self.assertNotIn("xs_namesakes", fp.PROTOCOLS[k])
            self.assertNotIn("nfeat", fp.PROTOCOLS[k])
            self.assertEqual((fp.nfeat(k), fp.csv_width(k, False), fp.csv_width(k, True)), (30, 37, 38))
            self.assertFalse(fp.uses_name_idf(k))
        self.assertEqual((fp.nfeat("pairfeat6"), fp.csv_width("pairfeat6", False), fp.csv_width("pairfeat6", True)), (34, 41, 42))
        self.assertTrue(fp.uses_name_idf("pairfeat6"))

    def test_cli_chain_output_gitignore(self):
        self.assertEqual(fp.parse_args(["x", "o", "--modelset", "pairfeat6"]), ("o", "pairfeat6"))
        self.assertEqual(fp.chain_files("pairfeat6"), ["encoder_pairfeat6.json", "lgb_stream_pairfeat6.json",
                                                       "iso_pairfeat6.json", "iso_v7_pairfeat6.json", "name_idf_pairfeat6.json"])
        self.assertEqual(fp.name_idf_name("pairfeat6"), "name_idf_pairfeat6.json")
        for k in EARLIER:
            self.assertEqual(len(fp.chain_files(k)), 4)
        with tempfile.TemporaryDirectory() as home:
            base = os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat6")
            os.makedirs(base)
            p = os.path.join(base, "r")
            self.assertEqual(fp.validate_output(p, home, "pairfeat6"), p)
            with self.assertRaises(ValueError):
                fp.validate_output(p, home, "pairfeat5")
        root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        with open(os.path.join(root, ".gitignore")) as f:
            self.assertIn("models/*_pairfeat6.json", f.read().splitlines())

    def test_nameidf_argv(self):
        self.assertEqual(fp.nameidf_argv(self.W, self.D, "pairfeat6", self.I),
                         [self.W, "nameidf", "-wasl", self.D, "-split", "train", "-persons", "5000", "-noise", "v1t",
                          "-idfseeds", "1019,1021,1031,1033,1039", "-out", self.I])

    def test_dump_argv(self):
        W, D, E, I = self.W, self.D, self.E, self.I
        t5 = fp.train_argv(W, D, E, "pairfeat5", 1019, "/o/t.csv")
        self.assertEqual(fp.train_argv(W, D, E, "pairfeat6", 1019, "/o/t.csv", idf=I),
                         t5[:-2] + ["-xsnamesakes", "0.1", "-nameidf", I, "-out", "/o/t.csv"])
        c5 = fp.cal_argv(W, D, E, "pairfeat5", "/o/c.csv")
        self.assertEqual(fp.cal_argv(W, D, E, "pairfeat6", "/o/c.csv", idf=I), c5[:-2] + ["-nameidf", I, "-out", "/o/c.csv"])
        p5 = fp.parity_argv(W, D, E, "pairfeat5", "/c/g.json", "/o/p.csv")
        self.assertEqual(fp.parity_argv(W, D, E, "pairfeat6", "/c/g.json", "/o/p.csv", idf=I),
                         p5[:-4] + ["-nameidf", I, "-gbmfile", "/c/g.json", "-out", "/o/p.csv"])
        n5 = fp.namesake_cal_argv(W, D, E, "pairfeat5", "/c/g.json", "/o/n.csv")
        self.assertEqual(fp.namesake_cal_argv(W, D, E, "pairfeat6", "/c/g.json", "/o/n.csv", idf=I),
                         n5[:-4] + ["-xsnamesakes", "0.1", "-nameidf", I, "-gbmfile", "/c/g.json", "-out", "/o/n.csv"])
        with self.assertRaises(ValueError):
            fp.train_argv(W, D, E, "pairfeat6", 1019, "/o/t.csv")            # pairfeat6 needs the table
        with self.assertRaises(ValueError):
            fp.cal_argv(W, D, E, "pairfeat6", "/o/c.csv")
        with self.assertRaises(ValueError):
            fp.train_argv(W, D, E, "pairfeat5", 1019, "/o/t.csv", idf=I)     # earlier modelsets take none

    def test_fit_argv(self):
        a6 = fp.fit_argv("/r", "/o", "/o/chain", "pairfeat6")
        self.assertEqual(a6[-4:], ["--lambda-l2", "1.0", "--nfeat", "34"])
        self.assertEqual(a6[-5], "pairfeat6")
        for k in EARLIER:
            self.assertNotIn("--nfeat", fp.fit_argv("/r", "/o", "/o/chain", k))

    def test_floor_three_kinds(self):
        def sc(g, f, x):
            return {"base_namesake_rows": g, "family_base_namesake_rows": f, "xs_base_namesake_rows": x}
        self.assertEqual(fp.check_namesake_floor([sc(250, 250, 300), sc(250, 250, 200)], "pairfeat6"), (500, 500, 500))
        with self.assertRaises(fp.NamesakeFloorError) as cm:
            fp.check_namesake_floor([sc(250, 250, 250), sc(250, 250, 249)], "pairfeat6")
        self.assertEqual(str(cm.exception), "namesake floor not met (cross-script): 499 < 500")
        self.assertEqual(fp.check_namesake_floor([sc(250, 250, 0), sc(250, 250, 0)], "pairfeat5"), (500, 500))

    def test_widths(self):
        with tempfile.TemporaryDirectory() as d:
            g = os.path.join(d, "g.csv")
            write_rows(g, [go_row6(0.25), go_row6(0.5)])
            self.assertEqual(fp.read_p_column(g, width=42), [0.25, 0.5])
            with self.assertRaises(ValueError):
                fp.read_p_column(g)                                            # default width 38
            plain = os.path.join(d, "a.csv")
            write_rows(plain, [go_row6(), go_row6()])
            fp.check_prefix_identity(plain, g, width=41)
            with self.assertRaises(ValueError):
                fp.check_prefix_identity(plain, g)
            self.assertEqual(fp.plain_cal_diff_at_merge(g, width=42), 0)

    def test_xs_slice(self):
        with tempfile.TemporaryDirectory() as d:
            c = os.path.join(d, "ns.csv")
            write_rows(c, [go_row6(0.9), go_row6(0.005), go_row6(0.01), go_row6(0.75)])
            sc = os.path.join(d, "ns.csv.namesakes.json")
            fp.write_json(sc, {"xs_rows": [1, 2, 3], "rows": [], "family_rows": []})
            self.assertEqual(fp.xs_namesake_slice(c, sc, width=42),
                             {"rows": 3, "different_person": 3, "different_at_review_0_01": 2, "different_at_merge_0_7": 1})
            with self.assertRaises(ValueError):
                fp.xs_namesake_slice(c, sc)

    def test_base_and_training_manifest(self):
        with tempfile.TemporaryDirectory() as d:
            make_repo(d, FILES + PF6_FILES)
            enc, idf = os.path.join(d, "enc.json"), os.path.join(d, "name_idf_pairfeat6.json")
            for p, b in ((enc, b"enc"), (idf, b"idf")):
                with open(p, "wb") as f:
                    f.write(b)
            base = fp.base_manifest(d, enc, "pairfeat6", name_idf=idf)
            self.assertEqual((base["nfeat"], base["name_idf_sha256"]), (34, sh(b"idf")))
            self.assertEqual(base["code_sha256"], {n: sh(n.encode()) for n in FILES + PF6_FILES})
            with self.assertRaises(ValueError):
                fp.base_manifest(d, enc, "pairfeat6")
            b5 = fp.base_manifest(d, enc, "pairfeat5")
            self.assertEqual(b5["nfeat"], 30)
            self.assertNotIn("name_idf_sha256", b5)
            self.assertEqual(set(b5["code_sha256"]), set(FILES))
            with self.assertRaises(ValueError):
                fp.base_manifest(d, enc, "pairfeat5", name_idf=idf)
            sides = {}
            for s in fp.PROTOCOLS["pairfeat6"]["train_seeds"]:
                p = os.path.join(d, f"s{s}.json")
                with open(p, "w") as f:
                    f.write(str(s))
                sides[s] = (p, {"namesakes_added": 90, "base_namesake_rows": 110, "family_namesakes_added": 40,
                                "family_base_namesake_rows": 60, "xs_namesakes_added": 70, "xs_base_namesake_rows": 150})
            m = fp.training_manifest(d, enc, "pairfeat6", sides, base)
            self.assertEqual((m["xs_namesakes"], m["xs_namesake_base_rows_total"], m["name_idf_sha256"]), (0.1, 750, sh(b"idf")))
            self.assertEqual((m["namesake_seeds"]["1021"]["xs_namesakes_added"],
                              m["namesake_seeds"]["1021"]["xs_base_namesake_rows"]), (70, 150))
            m5 = fp.training_manifest(d, enc, "pairfeat5", sides)
            self.assertNotIn("xs_namesakes", m5)
            self.assertNotIn("xs_namesakes_added", m5["namesake_seeds"]["1021"])

    def test_artifact_manifest_and_models(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain, out = (os.path.join(d, n) for n in ("models", "chain", "out"))
            for p in (models, chain, out):
                os.makedirs(p)
            for n in fp.BASELINE_FILES:
                with open(os.path.join(models, n), "wb") as f:
                    f.write(b"b")
            for n in fp.chain_files("pairfeat6") + fp.chain_files("pairfeat5"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"c" + n.encode())
            with open(os.path.join(out, "training-manifest.json"), "wb") as f:
                f.write(b"{}")
            xl = {"rows": 3, "different_person": 3, "different_at_review_0_01": 2, "different_at_merge_0_7": 1}
            m = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat6", max_abs_leaf=3.1,
                                     lambda_l2=1.0, xs_namesake_slice=xl, name_idf_sha256="ab" * 32)
            self.assertEqual(m["pairfeat6_chain_sha256"], {n: sh(b"c" + n.encode()) for n in fp.chain_files("pairfeat6")})
            self.assertEqual((m["xs_namesake_slice"], m["name_idf_sha256"]), (xl, "ab" * 32))
            m5 = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat5", max_abs_leaf=3.1, lambda_l2=1.0)
            self.assertNotIn("xs_namesake_slice", m5)
            self.assertNotIn("name_idf_sha256", m5)
            fp.check_models_free(models, "pairfeat6")
            with open(os.path.join(models, "name_idf_pairfeat6.json"), "wb") as f:
                f.write(b"old")
            with self.assertRaises(ValueError):
                fp.check_models_free(models, "pairfeat6")
            fp.check_models_free(models, "pairfeat5")
            os.remove(os.path.join(models, "name_idf_pairfeat6.json"))
            fp.copy_chain_to_models(chain, models, "pairfeat6")
            with open(os.path.join(models, "name_idf_pairfeat6.json"), "rb") as f:
                self.assertEqual(f.read(), b"cname_idf_pairfeat6.json")
```

- [ ] **Step 2: RED.** `python3 -I encoder/test_fit_scorer.py` and `python3 -I encoder/test_fit_pairfeat1.py` fail (no `NFEAT_CHOICES`, no `pairfeat6` protocol).
- [ ] **Step 3: Implement.**

  `encoder/fit_scorer.py`:
  - docstring: add `[--nfeat 34]` to Usage and "(34 features: modelset pairfeat6, f30..f33 appended)" to the input description;
  - `NFEAT_CHOICES = (30, 34)` under `NFEAT = 30`;
  - `load`:

```python
def load(path, nfeat=NFEAT):
    d = np.loadtxt(path, delimiter=",", dtype=np.float64, ndmin=2)
    if d.shape[0] == 0 or d.shape[1] not in (2 + nfeat + len(SLICES), 3 + nfeat + len(SLICES)):
        raise ValueError(f"{path}: expected {2 + nfeat + len(SLICES)} or {3 + nfeat + len(SLICES)} columns, got {d.shape[1]}")
    y, corr = d[:, 0].astype(int), d[:, 1].astype(int)
    X = d[:, 2:2 + nfeat]
    raw = d[:, 2 + nfeat:2 + nfeat + len(SLICES)]
    if not np.isin(d[:, :2], [0, 1]).all() or not np.isin(raw, [0, 1]).all():
        raise ValueError(f"{path}: labels, corr, and slice fields must be binary")
    slices = {name: raw[:, i].astype(bool) for i, name in enumerate(SLICES)}
    return y, corr, X, slices
```

  - `parse_args`: `ap.add_argument("--nfeat", type=int, choices=NFEAT_CHOICES, default=NFEAT, help="feature columns per row (34: pairfeat6)")`;
  - `main`: `parts = [load(t, a.nfeat) for t in a.train]`, `yca, cca, Xca, sca = load(a.cal, a.nfeat)`, and after the `lambda_l2` block `if a.nfeat != NFEAT:` / `rep["nfeat"] = a.nfeat`.

  `encoder/fit_pairfeat1.py`:
  - docstring usage: `[--modelset pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5|pairfeat6]` (and in `parse_args`' `usage`);
  - `PROTOCOLS` gains:

```python
    "pairfeat6": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
                  "namesakes": 0.10, "family_namesakes": 0.10, "lambda_l2": 1.0, "xs_namesakes": 0.10, "nfeat": 34},
```

  - replace `chain_files` and add helpers:

```python
CODE_FILES = ["pipeline/text.go", "pipeline/encdev.go", "pipeline/gen.go",
              "encoder/fit_scorer.py", "encoder/fit_pairfeat1.py"]
PAIRFEAT6_CODE_FILES = ["pipeline/pairfeat6.go", "pipeline/namesake.go"]


def nfeat(modelset):
    return PROTOCOLS[modelset].get("nfeat", 30)


def uses_name_idf(modelset):
    """pairfeat6: 34 features, the name-rarity table built first and part of the chain."""
    return nfeat(modelset) == 34


def csv_width(modelset, with_p):
    return 2 + nfeat(modelset) + 5 + (1 if with_p else 0)


def name_idf_name(modelset):
    return f"name_idf_{modelset}.json"


def chain_files(modelset):
    files = [f"encoder_{modelset}.json", f"lgb_stream_{modelset}.json",
             f"iso_{modelset}.json", f"iso_v7_{modelset}.json"]
    if uses_name_idf(modelset):
        files.append(name_idf_name(modelset))
    return files
```

  - `fit_argv`: after the `lambda_l2` lines, `if uses_name_idf(modelset):` / `argv += ["--nfeat", str(nfeat(modelset))]`;
  - argv builders:

```python
def _namesake_args(modelset):
    rate = PROTOCOLS[modelset]["namesakes"]
    args = ["-namesakes", str(rate)] if rate else []
    family = PROTOCOLS[modelset]["family_namesakes"]
    if family:
        args += ["-familynamesakes", str(family)]
    xs = PROTOCOLS[modelset].get("xs_namesakes")
    if xs:
        args += ["-xsnamesakes", str(xs)]
    return args


def _idf_args(modelset, idf):
    if uses_name_idf(modelset):
        if not idf:
            raise ValueError(f"{modelset} dumps need the name-rarity table (-nameidf)")
        return ["-nameidf", idf]
    if idf is not None:
        raise ValueError(f"{modelset} dumps take no name-rarity table")
    return []


def nameidf_argv(wsi, data, modelset, out):
    proto = PROTOCOLS[modelset]
    return [wsi, "nameidf", "-wasl", data, "-split", "train", "-persons", str(PERSONS), "-noise", proto["noise"],
            "-idfseeds", ",".join(str(s) for s in proto["train_seeds"]), "-out", out]


def train_argv(wsi, data, enc, modelset, seed, out, idf=None):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "train", "-persons", str(PERSONS),
            "-noise", proto["noise"], "-seed", str(seed)] + _namesake_args(modelset) + _idf_args(modelset, idf) + ["-out", out]


def cal_argv(wsi, data, enc, modelset, out, idf=None):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal",
            "-noise", proto["noise"], "-seed", str(proto["cal_seed"])] + _idf_args(modelset, idf) + ["-out", out]


def parity_argv(wsi, data, enc, modelset, gbmfile, out, idf=None):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal",
            "-noise", proto["noise"], "-seed", str(proto["cal_seed"])] + _idf_args(modelset, idf) + \
        ["-gbmfile", gbmfile, "-out", out]


def namesake_cal_argv(wsi, data, enc, modelset, gbmfile, out, idf=None):
    proto = PROTOCOLS[modelset]
    return [wsi, "featdump", "-wasl", data, "-encfile", enc, "-split", "cal",
            "-noise", proto["noise"], "-seed", str(proto["cal_seed"])] + _namesake_args(modelset) + \
        _idf_args(modelset, idf) + ["-gbmfile", gbmfile, "-out", out]
```

  - `check_namesake_floor`: inside the family branch, before `return total, ftotal`:

```python
        if PROTOCOLS[modelset].get("xs_namesakes"):
            xtotal = _sidecar_rows_total(sidecars, "xs_base_namesake_rows")
            if xtotal < NAMESAKE_FLOOR:
                raise NamesakeFloorError(f"namesake floor not met (cross-script): {xtotal} < {NAMESAKE_FLOOR}")
            return total, ftotal, xtotal
```

  - widths (messages keep today's text at the default widths):

```python
def _read_gbm_rows(path, width=38):
    out = []
    with open(path, newline="") as f:
        for row in csv.reader(f):
            if len(row) != width:
                raise ValueError(f"expected {width} fields per row, got {len(row)}")
            out.append((float(row[0]), float(row[-1])))
    return out
```

  `_slice(csv_path, sidecar_path, key, width=38)` calls `_read_gbm_rows(csv_path, width)`; `namesake_slice(csv_path, sidecar_path, width=38)` and `family_namesake_slice(csv_path, sidecar_path, width=38)` pass `width`; add `def xs_namesake_slice(csv_path, sidecar_path, width=38): return _slice(csv_path, sidecar_path, "xs_rows", width)`; `plain_cal_diff_at_merge(parity_csv, width=38)` passes it; `read_p_column(path, width=38)` checks `len(row) != width` with message `f"expected {width} fields per row, got {len(row)}"`; `check_prefix_identity(plain_csv, gbm_csv, width=37)` checks `len(x) != width or len(y) != width + 1 or x != y[:width]`.
  - `base_manifest(root, encoder, modelset=DEFAULT_MODELSET, name_idf=None)`: `files = CODE_FILES + (PAIRFEAT6_CODE_FILES if uses_name_idf(modelset) else [])`, `"nfeat": nfeat(modelset)`, and at the end:

```python
    if uses_name_idf(modelset):
        if name_idf is None:
            raise ValueError(f"{modelset} manifest needs the name-rarity table")
        m["name_idf_sha256"] = sha256(name_idf)
    elif name_idf is not None:
        raise ValueError(f"{modelset} has no name-rarity table")
    return m
```

  - `training_manifest`: inside `if rate:`, after the family block:

```python
        xs = PROTOCOLS[modelset].get("xs_namesakes")
        if xs:
            m["xs_namesakes"] = xs
            for seed, (_, sc) in sorted(sidecars.items()):
                e = m["namesake_seeds"][str(seed)]
                e["xs_namesakes_added"] = int(sc["xs_namesakes_added"])
                e["xs_base_namesake_rows"] = int(sc["xs_base_namesake_rows"])
            m["xs_namesake_base_rows_total"] = sum(int(sc["xs_base_namesake_rows"]) for _, sc in sidecars.values())
```

  - `artifact_manifest(..., max_abs_leaf=None, lambda_l2=None, xs_namesake_slice=None, name_idf_sha256=None)`: at the end `if xs_namesake_slice is not None: m["xs_namesake_slice"] = xs_namesake_slice` and `if name_idf_sha256 is not None: m["name_idf_sha256"] = name_idf_sha256`;
  - `main`: after `data = os.path.join(root, "data")`:

```python
    idf = None
    if uses_name_idf(modelset):  # pairfeat6: the name-rarity table first; every dump below reads it
        idf = os.path.join(chain, name_idf_name(modelset))
        run(nameidf_argv(wsi, data, modelset, idf), root)
```

  then `base = base_manifest(root, enc, modelset, name_idf=idf)`; pass `idf=idf` to `train_argv`, `cal_argv`, `parity_argv`, `namesake_cal_argv`; set `width = csv_width(modelset, False)` before the parity checks and use `check_prefix_identity(..., width=width)`, `read_p_column(..., width=width + 1)`, `namesake_slice(..., width=width + 1)`, `plain_cal_diff_at_merge(..., width=width + 1)`, `family_namesake_slice(..., width=width + 1)`; inside `if ns_rate:` add `if proto.get("xs_namesakes"): extra["xs_namesake_slice"] = xs_namesake_slice(ns_path, ns_path + ".namesakes.json", width=width + 1)`; pass `name_idf_sha256=sha256(idf) if idf else None` to `artifact_manifest`. The floor call is unchanged (it now also checks cross-script rows for `pairfeat6`).

  `.gitignore`: add `models/*_pairfeat6.json` after `models/*_pairfeat5.json` (it also covers `models/name_idf_pairfeat6.json`).
- [ ] **Step 4: GREEN.** `python3 -I encoder/test_fit_scorer.py && python3 -I encoder/test_fit_pairfeat1.py`. Byte-identity of the other modelsets: for `pairfeat1`-`pairfeat5`, the existing argv, manifest and floor tests pass unchanged.
- [ ] **Step 5: Commit.**

```bash
git commit -m "pairfeat6 fit: 34-feature protocol, name-rarity table in the chain, cross-script namesake floor" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" \
  -- encoder/fit_scorer.py encoder/test_fit_scorer.py encoder/fit_pairfeat1.py encoder/test_fit_pairfeat1.py .gitignore
```

### Task D: three-arm pairfeat6 candidate in the harness (Python/Bash)

**Files:** `scripts/pairfeat_compare.py`, `scripts/pairfeat_experiment.sh`, `scripts/test_pairfeat_compare.py`.

**Interfaces:**
- Consumes (Task C): the `pairfeat6` artifact manifest (`pairfeat6_chain_sha256` over five files) and a separate `pairfeat5` fit's artifact manifest.
- Produces (`scripts/pairfeat_compare.py`):
  - `CANDIDATES["pairfeat6"] = {"dev_seeds": [659, 661, 673], "validation_seeds": [677, 683, 691, 701, 709], "review_gate": "wrong_pair_cap", "report_xscript_cap": True, "reference": "pairfeat5", "reference_chain": "experiment", "reference_gate": "pairfeat6", "baseline_gating": False, "report_linked_pairs": True}`
  - `MODELSET["pairfeat6"] = "pairfeat6"`; `CHAIN_FILES["pairfeat6"]` = five files (with `name_idf_pairfeat6.json`)
  - `REFERENCE_CHAIN_KEY = "reference_model_chain"`; `EXPERIMENT_KEYS`, `EXPERIMENT_REF_KEYS`
  - `reference_chain_in_experiment(candidate) -> bool`, `arm_chain(doc, arm, candidate) -> dict|None`, `chain_label(arm, candidate) -> str`, `governing_view(doc, exp_dir, candidate) -> dict`
  - `init_experiment(exp_dir, candidate, reference_dir=None, repo=REPO)`; CLI `init-experiment EXPERIMENT_DIR CANDIDATE [REFERENCE_FIT_DIR]`. `pairfeat6` `experiment.json` = `{"candidate": "pairfeat6", "reference": "pairfeat5", "reference_model_chain": {<4 files>: sha}, "reference_artifact_manifest_sha256": sha}`
  - `EDGE_CLASSES = ("cross_script_lookalike", "missing_dob_ambiguous", "shared_token", "other", "unresolved")`, `LINKED_KEYS = ("same_person", "different_person")`
  - `final_linked_pairs(lines) -> set[tuple[str, str]]`, `cohort_registrations(cohort_dir) -> dict[str, dict]`, `edge_class(ra, rb) -> str`, `linked_pair_report(lines, pk_qid, regs=None) -> dict`
  - `lane_metrics(..., completion=None, linked=None)` adds `"linked_pairs"`; `aggregate_arm` sums it
  - `PF6_STRICT_KINDS = ("selection", "validation")`, `PF6_GROWTH = Fraction(11, 10)`, `PF6_WRONG_SLACK = 1`, `PF6_MIN_GAIN_PP = 0.0`; `_pairfeat6_gate(ref, cand, kind, base) -> dict`, reached through `reference_gate`
  - analysis output `rarity_table_sha256` for `pairfeat6`; selection/stress/validation-manifest carry `reference_model_chain`
- Produces (`scripts/pairfeat_experiment.sh`): arm `pairfeat6`; seed table line `pairfeat6) DEV_SEEDS="659 661 673"; VAL_SEEDS="677 683 691 701 709"; REF=pairfeat5; REFCHAIN=experiment;;`; the re-check gets `"$REF" "$REFCHAIN"`.

- [ ] **Step 0: Confirm the r1review goldens at BASE** (before editing; HEAD has Tasks A-C only, the harness is still `931a823`'s):

```bash
cd scripts && python3 -I - <<'PY'
import contextlib, hashlib, io, os, sys
sys.path.insert(0, os.getcwd())
import test_pairfeat_compare as T
pfc = T.pfc
ROOT, BIN, O, c = "/repo", "/repo/bin/wsi7", "/data/exp/arm/v2-s0", T.R1
tables = {"arms": list(pfc.arms(c)), "modes": pfc.modes(c), "review_gate": pfc.review_gate(c),
          "settings": {m: pfc.expected_settings(m, c) for m in pfc.MODES},
          "chain": {a: [pfc.CHAIN_KEY[a], list(pfc.CHAIN_FILES[a]), pfc.MODELSET[a]] for a in pfc.arms(c)},
          "lanes": {m: {str(s): {a: pfc.lane_commands(ROOT, BIN, O, s, m, a, candidate=c) for a in pfc.arms(c)}
                        for s in pfc.modes(c)[m]["seeds"]} for m in pfc.MODES}}
t = T.CLIR1review(); t.setUp()
try:
    with contextlib.redirect_stdout(io.StringIO()):
        assert t.run3("dev-clean", t.three()) == 0
    got = {n: hashlib.sha256(T._rb(os.path.join(t.exp, n))).hexdigest() for n in ("dev-clean-summary.json", "selection.json")}
finally:
    t.tearDown()
print("driver", T._digest(tables)); print("dev-clean", T._digest(got))
PY
```

  Expected: `driver 67706e7258577a131cb022260d8e1222ec103a974c86601abfa0b8924c0a4609`, `dev-clean f17691f0fa469dc7c4efa1b164d4ade29fd75a03fbcea4f7bf8510f543d9d53c`. Stop if they differ.

- [ ] **Step 1: Failing tests.** In `scripts/test_pairfeat_compare.py`:
  - First move every unknown-candidate sentinel off `pairfeat6`: `sed -i '' 's/pairfeat6/pairfeat7/g' scripts/test_pairfeat_compare.py`, then confirm with `grep -c pairfeat7 scripts/test_pairfeat_compare.py` that exactly the 13 sentinel lines changed (today's lines 1022 1056 1126 1135 1146 1193 1308 1387 1391 1565 1586 2468 2503).
  - `Candidates.test_candidate_set_and_seeds`: the expected `CANDIDATES` dict gains `"pairfeat6": P6_ENTRY` (literal, as below); `ALLOWED_ARMS` becomes `("baseline", "pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4", "pairfeat5", "r1review", "pairfeat6")`; the expected `MODELSET` gains `"pairfeat6": "pairfeat6"`.
  - Append at the end of the file:

```python
# ================================================================ candidate pairfeat6 (three arms: baseline, pairfeat5 reference fitted separately, pairfeat6)

P6 = "pairfeat6"
P6_DEV = [659, 661, 673]
P6_VAL = [677, 683, 691, 701, 709]
CAND6_FILES = ("encoder_pairfeat6.json", "lgb_stream_pairfeat6.json", "iso_pairfeat6.json", "iso_v7_pairfeat6.json",
               "name_idf_pairfeat6.json")
FILES[P6] = CAND6_FILES
DEV_SEEDS[P6] = P6_DEV
P6_ENTRY = {"dev_seeds": [659, 661, 673], "validation_seeds": [677, 683, 691, 701, 709], "review_gate": "wrong_pair_cap",
            "report_xscript_cap": True, "reference": "pairfeat5", "reference_chain": "experiment",
            "reference_gate": "pairfeat6", "baseline_gating": False, "report_linked_pairs": True}
USED_DEV = [307, 311, 313, 317, 331, 337, 347, 353, 367, 433, 449, 461, 521, 541, 547, 557, 563, 569, 571, 577, 587]
USED_VAL = [401, 409, 419, 421, 431, 601, 607, 613, 617, 619, 631, 641, 643, 647, 653]
B8_SEEDS = [11, 23, 37, 41, 59, 17, 29]
# r1review driver tables and dev-clean analysis outputs computed with the harness at 931a823 (before pairfeat6)
R1_GOLDEN = {"driver": "67706e7258577a131cb022260d8e1222ec103a974c86601abfa0b8924c0a4609",
             "dev-clean": "f17691f0fa469dc7c4efa1b164d4ade29fd75a03fbcea4f7bf8510f543d9d53c"}
PF6_PATIENTS = [("m|1", "Mohammed Salem", "1980-05-17"), ("m|2", "Mohamed Salem", "1980-05-17"),
                ("x|ar", "محمد سالم", "1975-01-01"), ("x|la", "Mohammed Karim", "1975-01-01"),
                ("y|a", "Ahmed Ali", ""), ("y|b", "Ahmed Ali", "1990-02-02"),
                ("z|a", "Mohammed Salem", "1985-03-03"), ("z|b", "Mohammed Nasser", "1985-03-04")]
PF6_PK = {"m|1": "M1", "m|2": "M1", "x|ar": "X1", "x|la": "X2", "y|a": "Y1", "y|b": "Y2", "z|a": "Z1", "z|b": "Z2"}


def link(a, b, **kw):
    return dict({"kind": "pair", "a": a, "b": b, "p": 0.98, "stage": "linked"}, **kw)


# one true merge (m), one pair per diagnosed class (x, y, z), a duplicate record and a pair unlinked later
PF6_LINKED = [link("m|1", "m|2"), link("m|2", "m|1"), link("x|ar", "x|la"), link("y|a", "y|b"), link("z|a", "z|b"),
              link("z|b", "m|1"), {"kind": "pair", "a": "m|1", "b": "z|b", "stage": "unlinked"}]


def patient_line(key, name, dob):
    prov, mrn = key.split("|")
    pt = {"resourceType": "Patient", "name": [{"text": name}],
          "identifier": [{"system": "urn:ae:%s:mrn" % prov, "value": mrn},
                         {"system": "http://doh.gov.ae/fhir/sid/emirates-id", "value": "784"}]}
    if dob:
        pt["birthDate"] = dob
    return {"arrival": 0.01, "prov": prov, "res": {"resourceType": "Encounter", "contained": [pt]}}


def pf6_arm(name, tp, truth=600, fm=10, cpe=10, uncapped=5, hours=10.0, **kw):
    a = cap_arm(name, tp, truth, uncapped, 0, fm, cross_person_edges=cpe, **kw)
    a["reviewer"]["hours"] = hours
    return a


def runner_recheck(exp, head, *args, mode="dev-clean"):
    """Run the runner's independent clean-HEAD manifest re-check (its inline Python) with these trailing arguments."""
    with open(SCRIPT) as h:
        lines = h.read().split("\n")
    start = next(i for i, l in enumerate(lines) if l.startswith('python3 -I - "$PARENT" "$MODE" "$HEAD"'))
    end = next(i for i in range(start + 1, len(lines)) if lines[i] == "PY")
    code = "\n".join(lines[start + 2:end])
    return subprocess.run(["python3", "-I", "-", exp, mode, head, SCRIPT, os.path.join(HERE, "pairfeat_compare.py")] + list(args),
                          input=code, capture_output=True, text=True, timeout=60).returncode


class Pairfeat6Candidate(unittest.TestCase):
    def test_entry_and_tables(self):
        self.assertEqual(pfc.CANDIDATES[P6], P6_ENTRY)
        self.assertEqual(pfc.arms(P6), ("baseline", P5, P6))
        self.assertEqual(pfc.modes(P6), {
            "dev-clean": {"split": "dev", "seeds": P6_DEV, "rerr": "0", "errungraded": "0"},
            "dev-stress": {"split": "dev", "seeds": P6_DEV, "rerr": "0.04", "errungraded": "0.25"},
            "validation-clean": {"split": "eval", "seeds": P6_VAL, "rerr": "0", "errungraded": "0"}})
        self.assertEqual(pfc.CHAIN_FILES[P6], CAND6_FILES)
        self.assertEqual((pfc.MODELSET[P6], pfc.CHAIN_KEY[P6], pfc.chain_of(P6), pfc.reference(P6), pfc.review_gate(P6)),
                         (P6, "pairfeat6_chain_sha256", P6, P5, "wrong_pair_cap"))
        self.assertEqual(pfc.extra_flags(P6), [])
        self.assertFalse(pfc.lane_image(P6))
        self.assertFalse(pfc.baseline_gating(P6))
        self.assertTrue(pfc.reference_chain_in_experiment(P6))
        self.assertFalse(any(pfc.reference_chain_in_experiment(c) for c in pfc.CANDIDATES if c != P6))
        self.assertIn(P6, pfc.CHAIN_CANDIDATES)
        s = pfc.expected_settings("validation-clean", P6)
        self.assertEqual((s["image"], s["reference"], s["candidate_extra_flags"], s["arms"], s["candidate_modelset"], s["seeds"]),
                         ("off", P5, [], ["baseline", P5, P6], P6, P6_VAL))
        self.assertNotIn("imgphotos", s)

    def test_seeds_never_touched_before(self):
        used = set(USED_DEV + USED_VAL + B8_SEEDS + list(pfc.RETIRED_SEEDS) + pfc.VALIDATION_SEEDS)
        for c, spec in pfc.CANDIDATES.items():
            if c != P6:
                used |= set(spec["dev_seeds"] + spec.get("validation_seeds", []))
        self.assertFalse(set(P6_DEV + P6_VAL) & used)
        self.assertTrue(all(s > 587 for s in P6_DEV + P6_VAL))


class Pairfeat6Guard(unittest.TestCase):
    BIN = "/repo/bin/wsi7"

    def cmds(self, mode, a, seed):
        return pfc.lane_commands("/repo", self.BIN, "/data/exp/arm/v2-s%d" % seed, seed, mode, a, candidate=P6)

    def test_lane_argv_per_arm(self):
        for a, ms in (("baseline", None), (P5, "pairfeat5"), (P6, "pairfeat6")):
            for name, argv, _ in self.cmds("dev-clean", a, 659):
                if name in pfc.MODEL_CMDS:
                    if ms:
                        self.assertEqual(argv[argv.index("-modelset") + 1], ms)
                    else:
                        self.assertNotIn("-modelset", argv)
                    self.assertEqual(argv[argv.index("-image") + 1], "off")
                for f in ("-r1review", "-imgphotos", "-imgcache"):
                    self.assertNotIn(f, argv)
        # the reference arm's lanes are exactly pairfeat5 lanes
        self.assertEqual(self.cmds("dev-clean", P5, 659),
                         pfc.lane_commands("/repo", self.BIN, "/data/exp/arm/v2-s659", 659, "dev-clean", P5))

    def test_every_lane_passes_the_guard(self):
        for mode in pfc.MODES:
            for s in pfc.mode_settings(mode, P6)["seeds"]:
                for a in pfc.arms(P6):
                    for _, argv, _ in self.cmds(mode, a, s):
                        pfc.guard(argv, mode, a, self.BIN, P6)

    def test_refusals(self):
        orch = {n: v for n, v, _ in self.cmds("dev-clean", P5, 659)}["orchestrate"]
        bad = list(orch)
        bad[bad.index("-modelset") + 1] = "pairfeat6"
        with self.assertRaises(pfc.GuardError):
            pfc.guard(bad, "dev-clean", P5, self.BIN, P6)
        for a in (R1, P4):
            with self.assertRaises(pfc.GuardError):
                pfc.guard(orch, "dev-clean", a, self.BIN, P6)
        for seed in (571, 401, 677, 557):          # another experiment's dev seed, old validation, validation in dev, retired
            for name, argv, _ in self.cmds("dev-clean", P6, seed):
                if name in ("gen", "orchestrate"):
                    with self.assertRaises(pfc.GuardError, msg=(name, seed)):
                        pfc.guard(argv, "dev-clean", P6, self.BIN, P6)
        gen = {n: v for n, v, _ in self.cmds("dev-clean", P6, 659)}["gen"]
        with self.assertRaises(pfc.GuardError):
            pfc.guard(gen + ["-imgphotos"], "dev-clean", P6, self.BIN, P6)


class Pairfeat6Preflight(unittest.TestCase):
    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.r = Repo(self.td.name, cand=P6)
        for f in CAND5_FILES + CAND6_FILES:
            with open(os.path.join(self.r.root, "models", f), "w") as h:
                h.write("model " + f)
        self.ref = os.path.join(self.td.name, "ref")   # the separately fitted pairfeat5 reference
        os.makedirs(self.ref)
        self.ep = os.path.join(self.r.exp, "experiment.json")

    def tearDown(self):
        self.td.cleanup()

    def ref_manifest(self, **kw):
        m = {"git_head": HEAD, "baseline_chain_sha256": self.r.hashes(BASE_FILES), "pairfeat5_chain_sha256": self.r.hashes(CAND5_FILES)}
        m.update(kw)
        _wj(os.path.join(self.ref, "artifact-manifest.json"), m)
        return m

    def init(self):
        return pfc.init_experiment(self.r.exp, P6, self.ref, repo=self.r.root)

    def ident(self, **kw):
        d = {"git_head": HEAD, "baseline_chain_sha256": self.r.hashes(BASE_FILES), "pairfeat6_chain_sha256": self.r.hashes(CAND6_FILES),
             "reference_model_chain": self.r.hashes(CAND5_FILES), "candidate": P6}
        d.update(kw)
        return d

    def pi(self, mode, a):
        return pfc.preflight_identity(mode, a, self.r.exp, self.r.root, HEAD)

    def test_init_experiment(self):
        with self.assertRaises(pfc.PreflightError):
            self.init()                                                   # no artifact manifest
        self.r.artifact_manifest()
        with self.assertRaises(pfc.PreflightError):
            self.init()                                                   # no reference manifest
        good = self.ref_manifest()
        with self.assertRaises(pfc.PreflightError):
            pfc.init_experiment(self.r.exp, P6, repo=self.r.root)         # the reference fit directory is required
        with self.assertRaises(pfc.PreflightError):
            pfc.init_experiment(self.r.exp, R1, self.ref, repo=self.r.root)
        with self.assertRaises(pfc.PreflightError):
            pfc.init_experiment(self.r.exp, P6, self.r.exp, repo=self.r.root)   # a parent is not its own reference
        for bad in (dict(good, git_head="c" * 40), dict(good, baseline_chain_sha256=chain(BASE_FILES, "x")),
                    dict(good, pairfeat5_chain_sha256=chain(CAND5_FILES, "x")),       # not the models/ files
                    {k: v for k, v in good.items() if k != "pairfeat5_chain_sha256"},
                    dict(good, pairfeat4_chain_sha256=chain(CAND4_FILES, "c")),
                    dict(good, pairfeat5_chain_sha256={"x": "1"})):
            _wj(os.path.join(self.ref, "artifact-manifest.json"), bad)
            with self.assertRaises(pfc.PreflightError, msg=str(bad)):
                self.init()
            self.assertFalse(os.path.exists(self.ep))
        self.ref_manifest()
        want = {"candidate": P6, "reference": P5, "reference_model_chain": self.r.hashes(CAND5_FILES),
                "reference_artifact_manifest_sha256": _sha(os.path.join(self.ref, "artifact-manifest.json"))}
        self.assertEqual(self.init(), want)
        self.assertEqual(_rj(self.ep), want)
        before = _rb(self.ep)
        with self.assertRaises(pfc.PreflightError):
            self.init()                                                   # write-once
        self.assertEqual(_rb(self.ep), before)

    def test_init_experiment_cli_arguments(self):
        self.r.artifact_manifest()
        self.ref_manifest()
        self.assertEqual(pfc.main(["init-experiment", self.r.exp, P6]), 2)               # no reference fit directory
        self.assertEqual(pfc.main(["init-experiment", self.r.exp, P6, self.ref, "x"]), 2)
        self.assertEqual(pfc.main(["init-experiment", self.r.exp, R1, self.ref]), 2)
        self.assertFalse(os.path.exists(self.ep))

    def test_preflight_per_arm(self):
        self.r.artifact_manifest()
        for a in pfc.arms(P6):
            with self.assertRaises(pfc.PreflightError):
                self.pi("dev-clean", a)                                   # no experiment.json
        self.ref_manifest()
        self.init()
        for a, files in (("baseline", BASE_FILES), (P5, CAND5_FILES), (P6, CAND6_FILES)):
            self.assertEqual(self.pi("dev-clean", a), (P6, self.r.hashes(files)))
        for a in (R1, P4):
            with self.assertRaises(pfc.PreflightError):
                self.pi("dev-clean", a)
        good = _rb(self.ep)
        e = _rj(self.ep)
        for bad in ({k: v for k, v in e.items() if k != "reference_model_chain"}, dict(e, reference=P4),
                    dict(e, reference_model_chain={"x": "1"}), dict(e, reference_artifact_manifest_sha256="z"),
                    dict(e, candidate=R1), dict(e, extra=1)):
            _wj(self.ep, bad)
            for a in pfc.arms(P6):
                with self.assertRaises(pfc.PreflightError, msg=str(bad)):
                    self.pi("dev-clean", a)
        with open(self.ep, "wb") as h:
            h.write(good)
        with open(os.path.join(self.r.root, "models", "iso_pairfeat5.json"), "a") as h:
            h.write("x")                                                  # a changed reference model file
        with self.assertRaises(pfc.PreflightError):
            self.pi("dev-clean", P5)
        self.pi("dev-clean", P6)

    def test_selection_stress_freeze_and_validation(self):
        self.r.artifact_manifest()
        self.ref_manifest()
        self.init()
        sp, tp = (os.path.join(self.r.exp, n) for n in ("selection.json", "stress.json"))
        sel = self.ident(selected_modelset=P6, mode="dev-clean", seeds=P6_DEV, **{"pass": True})
        for bad in ({k: v for k, v in sel.items() if k != "reference_model_chain"},
                    dict(sel, reference_model_chain=chain(CAND5_FILES, "x")),
                    dict(sel, pairfeat5_chain_sha256=self.r.hashes(CAND5_FILES))):
            _wj(sp, bad)
            with self.assertRaises(pfc.PreflightError, msg=str(bad)):
                self.pi("dev-stress", "baseline")
        _wj(sp, sel)
        self.assertEqual(self.pi("dev-stress", P5)[0], P6)
        _wj(tp, self.ident(mode="dev-stress", seeds=P6_DEV, selection_sha256=_sha(sp), **{"pass": True}))
        vm = pfc.freeze_validation(self.r.exp, self.r.root, HEAD)
        self.assertEqual((vm["candidate"], vm["seeds"]), (P6, P6_VAL))
        self.assertEqual(vm["reference_model_chain"], self.r.hashes(CAND5_FILES))
        self.assertEqual(vm["experiment_sha256"], _sha(self.ep))
        self.assertNotIn("pairfeat5_chain_sha256", vm)
        for a, files in (("baseline", BASE_FILES), (P5, CAND5_FILES), (P6, CAND6_FILES)):
            self.assertEqual(self.pi("validation-clean", a), (P6, self.r.hashes(files)))
        vp = os.path.join(self.r.exp, "validation-manifest.json")
        frozen = _rb(vp)
        _wj(vp, dict(vm, reference_model_chain=chain(CAND5_FILES, "x")))
        with self.assertRaises(pfc.PreflightError):
            self.pi("validation-clean", "baseline")
        with open(vp, "wb") as h:
            h.write(frozen)
        self.pi("validation-clean", "baseline")


class Pairfeat6Gate(unittest.TestCase):
    def g(self, cand, kind="selection", ref=None, base=None):
        return pfc.reference_gate(ref or pf6_arm(P5, 530), cand, kind, base=base or pf6_arm("baseline", 500))

    def test_pass_at_every_limit(self):
        g = self.g(pf6_arm(P6, 530, fm=9, cpe=9, uncapped=6, hours=11.0))
        self.assertEqual(json.dumps(g, sort_keys=True), json.dumps({
            "pass": True, "kind": "selection", "reference": P5, "gain_pp": 0.0, "min_gain_pp": 0.0,
            "contamination": {"false_merge_pairs": {P5: 10, P6: 9, "rule": "strictly below"},
                              "cross_person_edges": {P5: 10, P6: 9, "rule": "strictly below"}},
            "uncapped_wrong_distinct_pairs": {P5: 5, P6: 6, "max_allowed": 6.0},
            "hours": {P5: 10.0, P6: 11.0, "max_allowed": 11.0},
            "reported": {"impure_clusters": {P5: 0, P6: 0}, "wrong_final_signoffs": {P5: 0, P6: 0},
                         "xscript_cap": {P5: None, P6: None}, "linked_pairs": {P5: None, P6: None}, "gating": False},
            "reasons": []}, sort_keys=True))

    def test_contamination_strictly_below_except_stress(self):
        for kind in ("selection", "validation"):
            self.assertEqual(self.g(pf6_arm(P6, 530, fm=10, cpe=9), kind)["reasons"],
                             ["false_merge_pairs not strictly below reference (10 >= 10)"])
            self.assertEqual(self.g(pf6_arm(P6, 530, fm=9, cpe=10), kind)["reasons"],
                             ["cross_person_edges not strictly below reference (10 >= 10)"])
        self.assertTrue(self.g(pf6_arm(P6, 530), "stress")["pass"])        # equal: not above
        g = self.g(pf6_arm(P6, 530, fm=11, cpe=11), "stress")
        self.assertEqual(g["reasons"], ["false_merge_pairs above reference (11 > 10)", "cross_person_edges above reference (11 > 10)"])
        self.assertEqual(g["contamination"]["false_merge_pairs"]["rule"], "not above")

    def test_recall_gain_at_least_zero(self):
        for kind in pfc.GATE_KINDS:
            self.assertTrue(self.g(pf6_arm(P6, 530, fm=9, cpe=9), kind)["pass"], kind)
            self.assertEqual(self.g(pf6_arm(P6, 529, fm=9, cpe=9), kind)["reasons"], ["gain vs reference -0.1667pp below 0.0pp"])

    def test_uncapped_wrong_pair_limit(self):
        for ru, ok, bad, shown in ((5, 6, 7, "6.00"), (20, 22, 23, "22.00"), (0, 1, 2, "1.00")):
            ref = pf6_arm(P5, 530, uncapped=ru)
            self.assertTrue(self.g(pf6_arm(P6, 530, fm=9, cpe=9, uncapped=ok), ref=ref)["pass"], ru)
            self.assertEqual(self.g(pf6_arm(P6, 530, fm=9, cpe=9, uncapped=bad), ref=ref)["reasons"],
                             ["uncapped wrong-pair link-review pairs %d exceed %s (max(1.10 x %d, %d + 1))" % (bad, shown, ru, ru)])

    def test_hours(self):
        self.assertEqual(self.g(pf6_arm(P6, 530, fm=9, cpe=9, hours=11.01))["reasons"],
                         ["reviewer hours 11.01 exceed 11.00 (reference 10.00 +10%)"])

    def test_safety_in_every_arm(self):
        for k in pfc.SAFETY:
            for label, which in (("baseline ", "base"), ("reference ", "ref"), ("", "cand")):
                a = {"base": pf6_arm("baseline", 500), "ref": pf6_arm(P5, 530), "cand": pf6_arm(P6, 530, fm=9, cpe=9)}
                a[which][k] = 1
                g = pfc.reference_gate(a["ref"], a["cand"], "stress", base=a["base"])
                self.assertEqual(g["reasons"], [label + k + " nonzero"])
        b = pf6_arm("baseline", 500, wrong_released_by_origin={"a": 1})
        self.assertEqual(self.g(pf6_arm(P6, 530, fm=9, cpe=9), base=b)["reasons"], ["baseline wrong_released_by_origin nonzero"])

    def test_reported_and_inputs(self):
        g = self.g(pf6_arm(P6, 530, fm=9, cpe=9, impure_clusters=50))
        self.assertTrue(g["pass"])
        self.assertEqual(g["reported"]["impure_clusters"], {P5: 0, P6: 50})
        with self.assertRaises(ValueError):
            pfc.reference_gate(pf6_arm(P5, 530), pf6_arm(P6, 530), "selection")     # the baseline arm is required
        with self.assertRaises(ValueError):
            self.g(pf6_arm(P6, 530), "bogus")
        with self.assertRaises(ValueError):
            self.g(arm(P6, 530, 600))                                               # uncapped wrong pairs are required
        self.assertFalse(self.g(pf6_arm(P6, 530, fm=9, cpe=9), ref=pf6_arm(P4, 530))["pass"])
        self.assertIn("truth_closed differs between arms", self.g(pf6_arm(P6, 530, fm=9, cpe=9, truth=601))["reasons"])

    def test_selection_ignores_the_baseline_gate(self):
        base = pf6_arm("baseline", 500, fm=5)                                       # pairfeat6 prevents no false merge vs baseline
        s = pfc.select(base, pf6_arm(P6, 530, fm=9, cpe=9), pf6_arm(P5, 530))
        self.assertEqual((s["candidate"], s["reference"], s["selected_modelset"], s["pass"]), (P6, P5, P6, True))
        self.assertFalse(s["gate"]["pass"])
        self.assertIs(s["gate"]["gating"], False)
        s = pfc.select(base, pf6_arm(P6, 530), pf6_arm(P5, 530))                     # false merges equal: reference gate fails
        self.assertEqual((s["selected_modelset"], s["pass"]), (None, False))


class LinkedPairs(unittest.TestCase):
    def test_final_linked_pairs(self):
        lines = trace_lines([link("a", "b"), link("b", "a"), link("c", "d"), cap_rec("e", "f"),
                             {"kind": "pair", "a": "d", "b": "c", "stage": "unlinked"}, {"kind": "config"}]).splitlines(True)
        self.assertEqual(pfc.final_linked_pairs(lines), {("a", "b")})
        with self.assertRaises(ValueError):
            pfc.final_linked_pairs([json.dumps({"kind": "pair", "a": "a", "stage": "linked"})])

    def test_cohort_registrations(self):
        with tempfile.TemporaryDirectory() as d:
            with open(os.path.join(d, "fhir_p.ndjson"), "w") as h:
                h.write(trace_lines([patient_line("p|1", "Ahmed Ali", "1990-02-02"), patient_line("p|1", "Changed", "2000-01-01"),
                                     patient_line("q|2", "Sara", "")]))
            self.assertEqual(pfc.cohort_registrations(d), {"p|1": {"name": "Ahmed Ali", "dob": "1990-02-02"},
                                                           "q|2": {"name": "Sara", "dob": ""}})

    def test_edge_class(self):
        r = lambda n, d="1980-01-01": {"name": n, "dob": d}
        self.assertEqual(pfc.edge_class(r("محمد سالم"), r("Mohammed Karim")), "cross_script_lookalike")
        self.assertEqual(pfc.edge_class(r("Mohammed Karim", ""), r("محمد سالم")), "cross_script_lookalike")   # script first
        self.assertEqual(pfc.edge_class(r("Ahmed Ali", ""), r("Ahmed Ali")), "missing_dob_ambiguous")
        self.assertEqual(pfc.edge_class(r("Mohammed Salem"), r("Mohammed Nasser")), "shared_token")
        self.assertEqual(pfc.edge_class(r("Ahmed Ali"), r("ahmed ali")), "other")         # identical token sets
        self.assertEqual(pfc.edge_class(r("Ahmed Ali"), r("Sara Nasser")), "other")       # nothing shared
        self.assertEqual(pfc.edge_class(None, r("Ahmed Ali")), "unresolved")

    def test_linked_pair_report(self):
        pk = {"a": "P1", "b": "P1", "c": "P2", "d": "P3"}
        regs = {"a": {"name": "Ahmed Ali", "dob": "1990-02-02"}, "c": {"name": "محمد", "dob": "1975-01-01"},
                "d": {"name": "Mohammed", "dob": "1975-01-01"}}
        lines = trace_lines([link("a", "b"), link("c", "d")]).splitlines(True)
        self.assertEqual(pfc.linked_pair_report(lines, pk), {"same_person": 1, "different_person": 1})
        self.assertEqual(pfc.linked_pair_report(lines, pk, regs)["edge_classes"],
                         {"cross_script_lookalike": 1, "missing_dob_ambiguous": 0, "shared_token": 0, "other": 0, "unresolved": 0})
        with self.assertRaises(ValueError):
            pfc.linked_pair_report(trace_lines([link("a", "z")]).splitlines(True), pk)

    def test_lane_metrics_and_aggregate(self):
        lp = {"same_person": 2, "different_person": 1, "edge_classes": dict.fromkeys(pfc.EDGE_CLASSES, 0)}
        lp["edge_classes"]["shared_token"] = 1
        lanes = [pfc.lane_metrics(s, report(), funnel(), {}, LOAD, linked=lp) for s in (1, 2, 3)]
        self.assertEqual(lanes[0]["linked_pairs"], lp)
        a = pfc.aggregate_arm(P6, lanes)
        self.assertEqual(a["linked_pairs"], {"same_person": 6, "different_person": 3,
                                             "edge_classes": dict(dict.fromkeys(pfc.EDGE_CLASSES, 0), shared_token=3)})
        with self.assertRaises(ValueError):
            pfc.aggregate_arm(P6, lanes[:2] + [pfc.lane_metrics(3, report(), funnel(), {}, LOAD)])
        self.assertNotIn("linked_pairs", pfc.aggregate_arm(P5, [pfc.lane_metrics(1, report(), funnel(), {}, LOAD)]))


class CLIPairfeat6(CLIFixture, unittest.TestCase):
    CAND = P6

    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.exp = os.path.join(self.td.name, "exp")
        os.makedirs(self.exp)
        self.maps = {"baseline": chain(BASE_FILES, "b"), P5: chain(CAND5_FILES, "r"), P6: chain(CAND6_FILES, "c")}
        _wj(os.path.join(self.exp, "artifact-manifest.json"),
            {"git_head": HEAD, "baseline_chain_sha256": self.maps["baseline"], "pairfeat6_chain_sha256": self.maps[P6]})
        _wj(os.path.join(self.exp, "experiment.json"), {"candidate": P6, "reference": P5, "reference_model_chain": self.maps[P5],
                                                       "reference_artifact_manifest_sha256": "d" * 64})

    def make_arm(self, a, tp, *args, recs=None, fm=None, cpe=None, wrong=2, hours=None, **kw):
        root = super().make_arm(a, tp, *args, **kw)
        for d in sorted(os.listdir(root)):
            if not d.startswith("v2-s"):
                continue
            lane = os.path.join(root, d)
            q, pk = queue(same=1, wrong=wrong)
            pk.update({"c1": "C1", "c2": "C1", "c3": "C3", "c4": "C4"})
            pk.update(PF6_PK)
            _wj(os.path.join(lane, "run/main/review_queue.json"), q)
            os.makedirs(os.path.join(lane, "cohort"))
            _wj(os.path.join(lane, "cohort/truth.json"), {"pk_qid": pk, "loops": {}})
            with open(os.path.join(lane, "cohort/fhir_p.ndjson"), "w") as h:
                h.write(trace_lines([patient_line(k, n, b) for k, n, b in PF6_PATIENTS]))
            r = recs if recs is not None else ([] if a == "baseline" else [cap_rec("c1", "c2"), cap_rec("c3", "c4")])
            with open(os.path.join(lane, "run/main/link_funnel.ndjson"), "w") as h:
                h.write(trace_lines(r + PF6_LINKED))
            rp = os.path.join(lane, "run/main/report7.json")
            rj = _rj(rp)
            if fm is not None:
                rj["linkage"]["false_merge_pairs"] = fm
            if cpe is not None:
                rj["linkage"]["cross_person_edges_by_basis"] = {"wasl": cpe}
            if hours is not None:
                rj["reviewer"]["by_kind"]["all"]["sim_hours"] = hours
            _wj(rp, rj)
        return root

    def three(self, mode="dev-clean", b=5, f=6, c=7, **ck):
        cand = dict(fm=1, cpe=2)
        cand.update(ck)
        return (self.make_arm("baseline", b, mode=mode, fm=3), self.make_arm(P5, f, mode=mode, fm=3),
                self.make_arm(P6, c, mode=mode, **cand))

    def run3(self, mode, roots, out=None):
        b, f, c = roots
        return pfc.main([mode, out or os.path.join(self.exp, mode + "-summary.json"), "baseline=" + b, "pairfeat5=" + f,
                         "pairfeat6=" + c])

    def test_dev_clean_pass(self):
        self.assertEqual(self.run3("dev-clean", self.three()), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual((sel["candidate"], sel["reference"], sel["selected_modelset"], sel["pass"], sel["seeds"]),
                         (P6, P5, P6, True, P6_DEV))
        self.assertEqual((sel["pairfeat6_chain_sha256"], sel["reference_model_chain"]), (self.maps[P6], self.maps[P5]))
        self.assertNotIn("pairfeat5_chain_sha256", sel)
        self.assertIs(sel["gate"]["gating"], False)
        rg = sel["reference_gate"]
        self.assertEqual(rg["contamination"], {"false_merge_pairs": {P5: 9, P6: 3, "rule": "strictly below"},
                                               "cross_person_edges": {P5: 9, P6: 6, "rule": "strictly below"}})
        self.assertEqual(rg["uncapped_wrong_distinct_pairs"], {P5: 6, P6: 6, "max_allowed": 7.0})
        self.assertEqual(rg["hours"], {P5: 4.5, P6: 4.5, "max_allowed": 4.95})
        self.assertEqual(rg["gain_pp"], 10.0)
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        self.assertEqual(s["rarity_table_sha256"], self.maps[P6]["name_idf_pairfeat6.json"])
        for a in (P5, P6):
            self.assertEqual(s["arms"][a]["linked_pairs"], {"same_person": 3, "different_person": 9, "edge_classes": {
                "cross_script_lookalike": 3, "missing_dob_ambiguous": 3, "shared_token": 3, "other": 0, "unresolved": 0}})
            self.assertEqual(s["arms"][a]["xscript_cap"], {"pairs": 6, "same_person": 3, "different_person": 3})
            self.assertNotIn("completion_undocumented", s["arms"][a])
        self.assertNotIn("linked_pairs", s["arms"]["baseline"])

    def test_contamination_must_fall(self):
        self.assertEqual(self.run3("dev-clean", self.three(fm=3)), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual(sel["reference_gate"]["reasons"], ["false_merge_pairs not strictly below reference (9 >= 9)"])
        self.assertEqual((sel["selected_modelset"], sel["pass"]), (None, False))

    def test_three_roots_and_experiment_json_required(self):
        b, f, c = self.three()
        out = os.path.join(self.exp, "s.json")
        for roots in ({"baseline": b, P6: c}, {"baseline": b, P5: f}, {P5: f, P6: c}):
            with self.assertRaises(ValueError, msg=str(roots)):
                pfc.analyse("dev-clean", out, roots)
        ep = os.path.join(self.exp, "experiment.json")
        saved = _rb(ep)
        os.remove(ep)
        with self.assertRaises(ValueError):
            self.run3("dev-clean", (b, f, c))
        with open(ep, "wb") as h:
            h.write(saved)
        self.assertFalse(os.path.exists(os.path.join(self.exp, "selection.json")))

    def test_arm_identity_rejected(self):
        b, f, c = self.three()
        for roots in ((b, self.make_arm(P5, 6, fm=3, chain_map=self.maps[P6], name="f-map"), c),
                      (b, self.make_arm(P5, 6, fm=3, cfg_modelset=P6, name="f-ms"), c),
                      (b, f, self.make_arm(P6, 7, fm=1, cpe=2, chain_map=self.maps[P5], name="c-map"))):
            with self.assertRaises((ValueError, KeyError, OSError)):
                self.run3("dev-clean", roots)
            self.assertFalse(os.path.exists(os.path.join(self.exp, "selection.json")))
        self.assertEqual(self.run3("dev-clean", (b, f, c)), 0)

    def test_stress_freeze_and_validation_end_to_end(self):
        r = Repo(os.path.join(self.td.name, "v"), cand=P6)
        for fn in CAND5_FILES + CAND6_FILES:
            with open(os.path.join(r.root, "models", fn), "w") as h:
                h.write("model " + fn)
        ref = os.path.join(self.td.name, "v", "ref")
        os.makedirs(ref)
        _wj(os.path.join(ref, "artifact-manifest.json"), {"git_head": HEAD, "baseline_chain_sha256": r.hashes(BASE_FILES),
                                                          "pairfeat5_chain_sha256": r.hashes(CAND5_FILES)})
        self.exp = r.exp
        self.maps = {"baseline": r.hashes(BASE_FILES), P5: r.hashes(CAND5_FILES), P6: r.hashes(CAND6_FILES)}
        r.artifact_manifest()
        pfc.init_experiment(r.exp, P6, ref, repo=r.root)
        st3 = self.three(mode="dev-stress", fm=3, cpe=3)                # equal contamination passes in stress
        with self.assertRaises(ValueError):
            self.run3("dev-stress", st3)                                # no selection yet
        self.assertEqual(self.run3("dev-clean", self.three()), 0)
        self.assertEqual(self.run3("dev-stress", st3), 0)
        st = _rj(os.path.join(r.exp, "stress.json"))
        self.assertEqual((st["pass"], st["candidate"], st["reference"], st["seeds"]), (True, P6, P5, P6_DEV))
        self.assertEqual(st["reference_gate"]["contamination"]["false_merge_pairs"]["rule"], "not above")
        self.assertEqual(st["reference_model_chain"], self.maps[P5])
        vm = pfc.freeze_validation(r.exp, r.root, HEAD)
        self.assertEqual((vm["candidate"], vm["seeds"], vm["reference_model_chain"]), (P6, P6_VAL, self.maps[P5]))
        out = os.path.join(r.exp, "validation-clean-summary.json")
        v3 = self.three(mode="validation-clean")
        run = lambda: pfc.analyse("validation-clean", out, {"baseline": v3[0], P5: v3[1], P6: v3[2]}, repo=r.root, head=HEAD)
        res = run()
        self.assertTrue(res["pass"])
        self.assertEqual((res["reference_gate"]["kind"], res["candidate"], res["seeds"]), ("validation", P6, P6_VAL))
        self.assertEqual(res["arms"][P6]["linked_pairs"], {"same_person": 5, "different_person": 15})   # no classes outside development
        with self.assertRaises(FileExistsError):
            run()


class RunnerPairfeat6(RunnerFixture, unittest.TestCase):
    def write_experiment(self):
        _wj(os.path.join(self.exp, "experiment.json"), {"candidate": P6, "reference": P5,
                                                       "reference_model_chain": chain(CAND5_FILES, "r"),
                                                       "reference_artifact_manifest_sha256": "d" * 64})

    def test_parent_needs_experiment_json(self):
        self.real_artifact_manifest(cand=P6)
        self.assert_refused(self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True))   # no experiment.json
        self.write_experiment()
        r = self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("candidate pairfeat6 seeds 659 661 673", r.stdout)
        for a in (P5, P6):                      # fake model maps: refused by preflight, not by arm parsing
            r = self.run_sh(self.root, "dev-clean", a, preflight_only=True)
            self.assert_refused(r)
            self.assertNotIn("invalid arm", r.stderr)
        self.assert_refused(self.run_sh(self.root, "dev-clean", R1, preflight_only=True))

    def test_runner_tables(self):
        with open(SCRIPT) as h:
            text = h.read()
        self.assertIn('pairfeat6) DEV_SEEDS="659 661 673"; VAL_SEEDS="677 683 691 701 709"; REF=pairfeat5; REFCHAIN=experiment;;', text)

    def test_independent_recheck(self):
        self.real_artifact_manifest(cand=P6)
        self.assertNotEqual(runner_recheck(self.exp, self.head, P6, P6, P5, "experiment"), 0)   # no experiment.json
        self.write_experiment()
        self.assertEqual(runner_recheck(self.exp, self.head, P6, P6, P5, "experiment"), 0)
        self.assertNotEqual(runner_recheck(self.exp, self.head, P6, P6), 0)                    # without REFCHAIN the file is refused
        self.assertNotEqual(runner_recheck(self.exp, self.head, P6, P6, R1, "experiment"), 0)  # wrong reference
        self.assertNotEqual(runner_recheck(self.exp, self.head, P5, P5), 0)                    # the manifest carries pairfeat6


class R1reviewByteIdentity(unittest.TestCase):
    """r1review driver tables and dev-clean analysis outputs equal the harness's at 931a823."""

    def test_driver_tables_unchanged(self):
        c, root, bin_, o = R1, "/repo", "/repo/bin/wsi7", "/data/exp/arm/v2-s0"
        t = {"arms": list(pfc.arms(c)), "modes": pfc.modes(c), "review_gate": pfc.review_gate(c),
             "settings": {m: pfc.expected_settings(m, c) for m in pfc.MODES},
             "chain": {a: [pfc.CHAIN_KEY[a], list(pfc.CHAIN_FILES[a]), pfc.MODELSET[a]] for a in pfc.arms(c)},
             "lanes": {m: {str(s): {a: pfc.lane_commands(root, bin_, o, s, m, a, candidate=c) for a in pfc.arms(c)}
                           for s in pfc.modes(c)[m]["seeds"]} for m in pfc.MODES}}
        self.assertEqual(_digest(t), R1_GOLDEN["driver"])

    def test_dev_clean_outputs_unchanged(self):
        t = CLIR1review()
        t.setUp()
        try:
            self.assertEqual(t.run3("dev-clean", t.three()), 0)
            got = {n: hashlib.sha256(_rb(os.path.join(t.exp, n))).hexdigest() for n in ("dev-clean-summary.json", "selection.json")}
        finally:
            t.tearDown()
        self.assertEqual(_digest(got), R1_GOLDEN["dev-clean"])
```

- [ ] **Step 2: RED.** `python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'` fails (no `pairfeat6` candidate, no `final_linked_pairs`, ...).
- [ ] **Step 3: Implement** in `scripts/pairfeat_compare.py`:
  - imports: add `import glob` and `import re`.
  - module docstring: after the r1review paragraph add: "Three-arm candidate pairfeat6 (development 659 661 673, validation 677 683 691 701 709): its own chain (five files with the name-rarity table), image off, no extra flag; the reference arm pairfeat5 is a separate `fit_pairfeat1.py --modelset pairfeat5` fit at the same HEAD, recorded by `init-experiment EXPERIMENT_DIR pairfeat6 REFERENCE_FIT_DIR` in experiment.json (`reference_model_chain`, `reference_artifact_manifest_sha256`); decision files carry that map as `reference_model_chain`. Gate (pre-registered, spec gates table): vs pairfeat5, recall gain >= 0; false_merge_pairs and cross_person_edges strictly below in selection and validation, not above in stress; uncapped wrong-pair review pairs <= max(1.10 x ref, ref + 1); reviewer hours <= 1.10 x ref; SAFETY zero in all three arms. vs-baseline computed, not gating. Reported: linked pairs (same/different person; diagnosed classes on development), capped pairs, rarity-table SHA256." Add `| init-experiment EXPERIMENT_DIR pairfeat6 REFERENCE_FIT_DIR` to the usage text.
  - `CANDIDATES` (lines 85–100): add the `pairfeat6` entry after `r1review`:

```python
              # pairfeat6 (spec 2026-11-03-crc-pairfeat6-design.md): its own chain (pairfeat5's recipe, pair features
              # f30-f33, a name-rarity table, cross-script hard negatives); three arms against a pairfeat5 reference refit
              # separately at the same HEAD (its map lives in experiment.json); image off; gated vs pairfeat5 only
              "pairfeat6": {"dev_seeds": [659, 661, 673], "validation_seeds": [677, 683, 691, 701, 709],
                            "review_gate": "wrong_pair_cap", "report_xscript_cap": True,
                            "reference": "pairfeat5", "reference_chain": "experiment", "reference_gate": "pairfeat6",
                            "baseline_gating": False, "report_linked_pairs": True}}
```

  - after `REF_MAX_HOURS_GROWTH` (line 119):

```python
# pairfeat6 gate vs its reference (spec gates table)
PF6_STRICT_KINDS = ("selection", "validation")  # false_merge_pairs and cross_person_edges strictly below the reference
PF6_GROWTH = Fraction(11, 10)  # uncapped wrong-pair review pairs and reviewer hours at most 1.10 x the reference
PF6_WRONG_SLACK = 1  # ... or the reference + 1 uncapped wrong-pair review pairs, whichever is larger
PF6_MIN_GAIN_PP = 0.0  # CRC recall gain over the reference >= 0 in every kind
REFERENCE_CHAIN_KEY = "reference_model_chain"  # experiment.json / decision files: a separately fitted reference's map
EXPERIMENT_KEYS = frozenset({"candidate", "reference"})
EXPERIMENT_REF_KEYS = EXPERIMENT_KEYS | {REFERENCE_CHAIN_KEY, "reference_artifact_manifest_sha256"}
EDGE_CLASSES = ("cross_script_lookalike", "missing_dob_ambiguous", "shared_token", "other", "unresolved")
LINKED_KEYS = ("same_person", "different_person")
```

  - after `baseline_gating` (line 143):

```python
def reference_chain_in_experiment(candidate):
    """True for a three-arm candidate whose reference arm was fitted separately (pairfeat6): the reference's model map is
    recorded in experiment.json and decision files under REFERENCE_CHAIN_KEY, not in artifact-manifest.json."""
    return CANDIDATES.get(candidate, {}).get("reference_chain") == "experiment"


def arm_chain(doc, arm, candidate):
    """The model-chain map doc records for an arm of this candidate's experiment."""
    if reference_chain_in_experiment(candidate) and arm == reference(candidate):
        return doc.get(REFERENCE_CHAIN_KEY)
    return doc.get(CHAIN_KEY[arm])


def chain_label(arm, candidate):
    return REFERENCE_CHAIN_KEY if reference_chain_in_experiment(candidate) and arm == reference(candidate) else CHAIN_KEY[arm]
```

  - `MODELSET` (line 161–162): add `"pairfeat6": "pairfeat6"` to the literal; `CHAIN_FILES` (line 164–171): add

```python
    "pairfeat6": ("encoder_pairfeat6.json", "lgb_stream_pairfeat6.json", "iso_pairfeat6.json", "iso_v7_pairfeat6.json",
                  "name_idf_pairfeat6.json"),
```

  - `read_experiment` (lines 327–338):

```python
def read_experiment(exp_dir):
    """The parsed experiment.json of a parent, or None when it has none. A present file must be exactly
    {"candidate": <three-arm candidate>, "reference": <its reference arm>}, plus, for a candidate whose reference was fitted
    separately (pairfeat6), REFERENCE_CHAIN_KEY (a well-formed map of the reference's model files) and
    "reference_artifact_manifest_sha256"."""
    ep = os.path.join(exp_dir, EXPERIMENT)
    if not os.path.lexists(ep):
        return None
    e = _read_json(ep)
    c = e.get("candidate") if isinstance(e, dict) else None
    ok = isinstance(c, str) and c in CANDIDATES and bool(reference(c))
    if ok:
        want = EXPERIMENT_REF_KEYS if reference_chain_in_experiment(c) else EXPERIMENT_KEYS
        ok = set(e) == want and e["reference"] == reference(c)
    if not ok:
        raise PreflightError("experiment.json must be exactly {\"candidate\": <three-arm candidate>, \"reference\": <its reference>}"
                             " (allowed %s)" % sorted(x for x in CANDIDATES if reference(x)))
    if reference_chain_in_experiment(c):
        _check_chain_map(e[REFERENCE_CHAIN_KEY], reference(c), EXPERIMENT)
        if not _is_hex(e["reference_artifact_manifest_sha256"], 64):
            raise PreflightError("experiment.json reference_artifact_manifest_sha256 must be a SHA256")
    return e
```

  - `resolve_candidate` (lines 341–352):

```python
def resolve_candidate(doc, what, exp_dir=None):
    found = infer_candidate(doc, what)
    e = read_experiment(exp_dir) if exp_dir is not None else None
    if reference_chain_in_experiment(found):  # pairfeat6: experiment.json is required and names it
        if e is None or e["candidate"] != found:
            raise PreflightError("a %s parent needs experiment.json naming %s and its separately fitted reference "
                                 "(init-experiment EXPERIMENT_DIR %s REFERENCE_FIT_DIR)" % (found, found, found))
        return found
    if e is None:
        return found
    if chain_of(e["candidate"]) != found or e["reference"] != found:
        raise PreflightError("experiment.json candidate %s (reference %s) contradicts the %s chain in %s"
                             % (e["candidate"], e["reference"], found, what))
    return e["candidate"]


def governing_view(doc, exp_dir, candidate):
    """doc (artifact-manifest.json) with, for a candidate whose reference was fitted separately (pairfeat6), the reference
    map from experiment.json under REFERENCE_CHAIN_KEY, so arm_chain() reads every arm from one document; else doc."""
    if not reference_chain_in_experiment(candidate):
        return doc
    e = read_experiment(exp_dir)
    if e is None or e["candidate"] != candidate:
        raise PreflightError("the %s experiment needs experiment.json naming it" % candidate)
    return dict(doc, **{REFERENCE_CHAIN_KEY: e[REFERENCE_CHAIN_KEY]})
```

  - `init_experiment` (lines 355–374) becomes:

```python
def init_experiment(exp_dir, candidate, reference_dir=None, repo=REPO):
    """Write EXPERIMENT_DIR/experiment.json (write-once) naming a three-arm candidate and its reference.
    r1review: the parent's artifact-manifest.json carries exactly the reference (pairfeat5) chain; no reference_dir.
    pairfeat6: the parent's artifact-manifest.json carries the pairfeat6 chain; reference_dir is the separate
    fit_pairfeat1.py --modelset pairfeat5 output, whose artifact-manifest.json must carry exactly the pairfeat5 chain at the
    same git_head with the same baseline chain, equal to the models/ files now; its map and SHA256 are recorded."""
    if not isinstance(candidate, str) or candidate not in CANDIDATES or not reference(candidate):
        raise PreflightError("init-experiment needs a three-arm candidate (%s), got %r"
                             % ("|".join(sorted(c for c in CANDIDATES if reference(c))), candidate))
    ep = os.path.join(exp_dir, EXPERIMENT)
    if os.path.lexists(ep):
        raise PreflightError("experiment.json already exists; it is written once and never rewritten")
    for n in (SELECTION, STRESS, VALIDATION_MANIFEST):
        if os.path.lexists(os.path.join(exp_dir, n)):
            raise PreflightError("%s already exists: experiment.json must name the candidate before any decision" % n)
    am = _read_json(os.path.join(exp_dir, ARTIFACT_MANIFEST))
    found = infer_candidate(am, ARTIFACT_MANIFEST)
    ref = reference(candidate)
    if not reference_chain_in_experiment(candidate):
        if reference_dir is not None:
            raise PreflightError("init-experiment %s takes no reference fit directory" % candidate)
        if found != chain_of(candidate) or found != ref:
            raise PreflightError("artifact-manifest.json carries the %s chain, %s needs %s" % (found, candidate, chain_of(candidate)))
        _check_maps(am, candidate, ARTIFACT_MANIFEST)
        e = {"candidate": candidate, "reference": ref}
        _write_once(ep, e)
        return e
    if reference_dir is None:
        raise PreflightError("init-experiment %s needs the reference fit directory (fit_pairfeat1.py --modelset %s output)"
                             % (candidate, ref))
    if found != candidate:
        raise PreflightError("artifact-manifest.json carries the %s chain, %s needs its own" % (found, candidate))
    if not _is_hex(am.get("git_head"), 40):
        raise PreflightError("artifact-manifest.json needs a full git_head")
    for a in ("baseline", candidate):
        _check_chain_map(am.get(CHAIN_KEY[a]), a, ARTIFACT_MANIFEST)
    if os.path.realpath(reference_dir) == os.path.realpath(exp_dir):
        raise PreflightError("the reference fit directory must be another fit's directory")
    rp = os.path.join(reference_dir, ARTIFACT_MANIFEST)
    what = "reference " + ARTIFACT_MANIFEST
    ram = _read_json(rp, what)
    if infer_candidate(ram, what) != ref:
        raise PreflightError("the reference fit must carry exactly the %s chain" % ref)
    if ram.get("git_head") != am["git_head"]:
        raise PreflightError("the reference fit was made at another git HEAD than the %s fit" % candidate)
    if ram.get(CHAIN_KEY["baseline"]) != am[CHAIN_KEY["baseline"]]:
        raise PreflightError("the reference fit records another baseline chain")
    _check_chain_map(ram.get(CHAIN_KEY[ref]), ref, what)
    _check_files_match(repo, ref, ram[CHAIN_KEY[ref]], "the " + what)
    e = {"candidate": candidate, "reference": ref, REFERENCE_CHAIN_KEY: dict(ram[CHAIN_KEY[ref]]),
         "reference_artifact_manifest_sha256": sha256_file(rp)}
    _write_once(ep, e)
    return e
```

  - `_check_maps` (lines 377–379): `_check_chain_map(arm_chain(doc, a, candidate), a, what)`.
  - `check_artifact_manifest` (lines 403–411): `_check_maps(governing_view(am, exp_dir, candidate), candidate, "artifact-manifest.json")`.
  - `check_selection` (lines 424–426) and `check_stress` (lines 439–441): the arm loop compares `arm_chain(sel, a, candidate) != arm_chain(am, a, candidate)` (resp. `st`), message `"... %s differs from artifact-manifest.json" % chain_label(a, candidate)`. Callers pass the governing view.
  - `check_frozen_validation` (lines 468–491): after the experiment SHA check, `view = governing_view(am, exp_dir, candidate)`; the arm loop compares `arm_chain(vm, a, candidate) != arm_chain(view, a, candidate)` (message with `chain_label`); `check_selection(_read_json(sp), view, head, candidate)` and `check_stress(_read_json(tp), view, head, sha256_file(sp), candidate)`.
  - `preflight_identity` (lines 494–510): validation `_check_files_match(repo, arm, arm_chain(vm, arm, candidate), "validation-manifest.json")`; dev: `view = governing_view(am, exp_dir, candidate)` after `_check_arm`, `check_selection(..., view, head, candidate)`, and `_check_files_match(repo, arm, arm_chain(view, arm, candidate), "artifact-manifest.json")`.
  - `freeze_validation` (lines 518–544): `view = governing_view(am, exp_dir, candidate)` after `check_artifact_manifest`; `check_selection`/`check_stress` get `view`; `_check_files_match(repo, a, arm_chain(view, a, candidate), ...)`; after the `experiment_sha256` lines: `if reference_chain_in_experiment(candidate):` / `vm[REFERENCE_CHAIN_KEY] = dict(view[REFERENCE_CHAIN_KEY])`.
  - after `completion_review` (line 884), add:

```python
def final_linked_pairs(lines):
    """Distinct sorted (a, b) registration pairs linked at the end of a link_funnel.ndjson trace: a "linked" pair record
    adds the pair, a later "unlinked" record removes it."""
    keys = set()
    for ln in lines:
        if not ln.strip():
            continue
        r = json.loads(ln)
        if not isinstance(r, dict) or r.get("kind") != "pair" or r.get("stage") not in ("linked", "unlinked"):
            continue
        a, b = r.get("a"), r.get("b")
        if not isinstance(a, str) or not isinstance(b, str):
            raise ValueError("linked pair record must name two registrations")
        k = tuple(sorted((a, b)))
        if r["stage"] == "linked":
            keys.add(k)
        else:
            keys.discard(k)
    return keys


def cohort_registrations(cohort_dir):
    """Registration key (provider|MRN) -> {"name", "dob"} from the lane's cohort/fhir_*.ndjson Patient resources (top-level
    or contained; the first occurrence of a key is kept)."""
    out = {}
    for p in sorted(glob.glob(os.path.join(glob.escape(cohort_dir), "fhir_*.ndjson"))):
        with open(p) as h:
            for ln in h:
                if not ln.strip():
                    continue
                r = json.loads(ln)
                res = r.get("res") or {}
                for c in [res] + list(res.get("contained") or []):
                    if not isinstance(c, dict) or c.get("resourceType") != "Patient":
                        continue
                    names = c.get("name") or [{}]
                    n0 = names[0] if isinstance(names[0], dict) else {}
                    name = n0.get("text") or " ".join([n0.get("family") or ""] + list(n0.get("given") or [])).strip()
                    for idf in c.get("identifier") or []:
                        if isinstance(idf, dict) and str(idf.get("system", "")).endswith(":mrn") and idf.get("value"):
                            out.setdefault("%s|%s" % (r.get("prov"), idf["value"]), {"name": name, "dob": c.get("birthDate") or ""})
    return out


def _arabic(s):
    return any("؀" <= ch <= "ۿ" for ch in s)


def _name_tokens(s):
    return set(re.findall(r"[^\W\d_]+", s.lower()))


def edge_class(ra, rb):
    """Reported-only class of a remaining different-person linked pair (the pairfeat6 design's diagnosis), in order:
    cross_script_lookalike (one Arabic-script name, one not); missing_dob_ambiguous (a birth date missing on one side);
    shared_token (same script, at least one shared and one differing name token); other. unresolved: a registration that
    the cohort does not describe."""
    if ra is None or rb is None:
        return "unresolved"
    if _arabic(ra["name"]) != _arabic(rb["name"]):
        return "cross_script_lookalike"
    if not ra["dob"] or not rb["dob"]:
        return "missing_dob_ambiguous"
    ta, tb = _name_tokens(ra["name"]), _name_tokens(rb["name"])
    if ta & tb and ta ^ tb:
        return "shared_token"
    return "other"


def linked_pair_report(lines, pk_qid, regs=None):
    """Reported only (pairfeat6 experiments, non-baseline arms): final linked pairs split by truth (same_person = true
    merges); with regs (development lanes) the different-person pairs are also counted by edge_class. An unmapped
    registration is an analysis error."""
    same = diff = 0
    classes = dict.fromkeys(EDGE_CLASSES, 0)
    for a, b in sorted(final_linked_pairs(lines)):
        pa, pb = pk_qid.get(a), pk_qid.get(b)
        if not (isinstance(pa, str) and pa and isinstance(pb, str) and pb):
            raise ValueError("linked pair registration is not mapped by cohort/truth.json pk_qid")
        if pa == pb:
            same += 1
            continue
        diff += 1
        if regs is not None:
            classes[edge_class(regs.get(a), regs.get(b))] += 1
    out = {"same_person": same, "different_person": diff}
    if regs is not None:
        out["edge_classes"] = classes
    return out
```

  - `lane_metrics` (line 887): signature `def lane_metrics(seed, report7, funnel, status, load, xscript_cap=None, candidate=None, completion=None, linked=None):`; before `return m`: `if linked is not None:` / `m["linked_pairs"] = json.loads(json.dumps(linked))  # pairfeat6 non-baseline arms; reported, never gated`.
  - `aggregate_arm` (before `return a`, line 983):

```python
    lp = ["linked_pairs" in l for l in lanes]
    if any(lp):
        if not all(lp):
            raise ValueError("linked_pairs present in only some lanes of arm %s" % arm)
        a["linked_pairs"] = {k: sum(l["linked_pairs"][k] for l in lanes) for k in LINKED_KEYS}
        ec = ["edge_classes" in l["linked_pairs"] for l in lanes]
        if any(ec):
            if not all(ec):
                raise ValueError("edge_classes present in only some lanes of arm %s" % arm)
            a["linked_pairs"]["edge_classes"] = {k: sum(l["linked_pairs"]["edge_classes"][k] for l in lanes) for k in EDGE_CLASSES}
```

  - `reference_gate` (line 1113): first statements:

```python
    ca0 = cand.get("arm")
    if isinstance(ca0, str) and CANDIDATES.get(ca0, {}).get("reference_gate") == "pairfeat6":
        return _pairfeat6_gate(ref, cand, kind, base)
```

  and add after it:

```python
def _pairfeat6_gate(ref, cand, kind, base):
    """Gate of pairfeat6 against its reference arm pairfeat5 (spec 2026-11-03-crc-pairfeat6-design.md, gates table):
    recall gain >= 0; false_merge_pairs and cross_person_edges strictly below the reference in selection and validation,
    not above in stress; uncapped wrong-pair link-review pairs <= max(1.10 x ref, ref + 1); reviewer hours <= 1.10 x ref;
    the SAFETY set and wrong releases by origin zero in the baseline, reference and candidate arms. impure_clusters,
    wrong final sign-offs, capped pairs and linked pairs are reported, not gated."""
    if kind not in GATE_KINDS:
        raise ValueError("gate kind must be one of %s" % (GATE_KINDS,))
    if base is None or base.get("arm") != "baseline":
        raise ValueError("the pairfeat6 gate needs the baseline arm (SAFETY is gated in every arm)")
    rk, ck = ref.get("arm"), cand.get("arm")
    reasons = []
    if ck != "pairfeat6" or rk != reference("pairfeat6"):
        reasons.append("arms must be pairfeat6 vs its reference arm pairfeat5")
    if not base["truth_closed"] == ref["truth_closed"] == cand["truth_closed"]:
        reasons.append("truth_closed differs between arms")
    gain = None
    if not ref["truth_closed"] or not cand["truth_closed"]:
        reasons.append("truth_closed is zero")
    else:
        g = 100 * (Fraction(cand["tp"], cand["truth_closed"]) - Fraction(ref["tp"], ref["truth_closed"]))
        gain = float(g)
        if g < 0:
            reasons.append("gain vs reference %.4fpp below 0.0pp" % gain)
    for label, a in (("baseline ", base), ("reference ", ref), ("", cand)):
        for k in SAFETY:
            if a[k] != 0:
                reasons.append(label + k + " nonzero")
        if any(v != 0 for v in a["wrong_released_by_origin"].values()):
            reasons.append(label + "wrong_released_by_origin nonzero")
    strict = kind in PF6_STRICT_KINDS
    contam = {}
    for k in ("false_merge_pairs", "cross_person_edges"):
        rv, cv = ref[k], cand[k]
        contam[k] = {rk: rv, ck: cv, "rule": "strictly below" if strict else "not above"}
        if strict and cv >= rv:
            reasons.append("%s not strictly below reference (%d >= %d)" % (k, cv, rv))
        elif not strict and cv > rv:
            reasons.append("%s above reference (%d > %d)" % (k, cv, rv))
    vals = []
    for label, a in (("reference", ref), ("candidate", cand)):
        v = a["reviewer"].get("uncapped_wrong_distinct_pairs")
        if isinstance(v, bool) or not isinstance(v, int) or v < 0:
            raise ValueError("pairfeat6 gate needs reviewer.uncapped_wrong_distinct_pairs in the %s arm" % label)
        vals.append(v)
    ru, cu = vals
    ulimit = max(ru * PF6_GROWTH, Fraction(ru + PF6_WRONG_SLACK))
    if cu > ulimit:
        reasons.append("uncapped wrong-pair link-review pairs %d exceed %.2f (max(1.10 x %d, %d + 1))" % (cu, float(ulimit), ru, ru))
    rh, chh = ref["reviewer"]["hours"], cand["reviewer"]["hours"]
    hlimit = Fraction(rh) * PF6_GROWTH
    if Fraction(chh) > hlimit:
        reasons.append("reviewer hours %.2f exceed %.2f (reference %.2f +10%%)" % (chh, float(hlimit), rh))
    reported = {"impure_clusters": {rk: ref["impure_clusters"], ck: cand["impure_clusters"]},
                "wrong_final_signoffs": {rk: ref["reviewer"]["wrong_final_signoffs"], ck: cand["reviewer"]["wrong_final_signoffs"]},
                "xscript_cap": {rk: ref.get("xscript_cap"), ck: cand.get("xscript_cap")},
                "linked_pairs": {rk: ref.get("linked_pairs"), ck: cand.get("linked_pairs")}, "gating": False}
    return {"pass": not reasons, "kind": kind, "reference": rk, "gain_pp": None if gain is None else round(gain, 4),
            "min_gain_pp": PF6_MIN_GAIN_PP, "contamination": contam,
            "uncapped_wrong_distinct_pairs": {rk: ru, ck: cu, "max_allowed": float(ulimit)},
            "hours": {rk: rh, ck: chh, "max_allowed": float(hlimit)}, "reported": reported, "reasons": reasons}
```

  - `_load_lane` (lines 1225–1263): the completion line becomes `cu = completion_review(q, dec) if "-r1review" in extra_flags(candidate) else None  # r1review experiments, every arm`; before the `lane_image` check:

```python
    linked = None
    if CANDIDATES[candidate].get("report_linked_pairs") and arm != "baseline":  # pairfeat6: reported, never gated
        regs = cohort_registrations(os.path.join(path, "cohort")) if MODES[mode]["split"] == "dev" else None
        with open(os.path.join(path, "run/main/link_funnel.ndjson")) as h:
            linked = linked_pair_report(h, pk, regs)
```

  and the return passes `cu, linked`.
  - `analyse` (lines 1266–1352): in the dev branch, after `candidate = resolve_candidate(gov, gov_name, exp)` add `gov = governing_view(gov, exp, candidate)`; in the arm loop compare `b["model_chain_sha256"] != arm_chain(gov, a, candidate)`; after the `ident` dict: `if reference_chain_in_experiment(candidate):` / `ident[REFERENCE_CHAIN_KEY] = gov[REFERENCE_CHAIN_KEY]`; and

```python
    rarity = [f for f in CHAIN_FILES[candidate] if f.startswith("name_idf_")]
    if rarity:  # pairfeat6: the name-rarity table's SHA256 (reported; it is part of the candidate's chain map)
        res["rarity_table_sha256"] = gov[CHAIN_KEY[candidate]][rarity[0]]
```

  - `main` init-experiment (lines 1369–1379):

```python
    if argv[:1] == ["init-experiment"]:
        if len(argv) not in (3, 4):
            print("usage: pairfeat_compare.py init-experiment EXPERIMENT_DIR CANDIDATE [REFERENCE_FIT_DIR]", file=sys.stderr)
            return 2
        try:
            e = init_experiment(os.path.abspath(argv[1]), argv[2], os.path.abspath(argv[3]) if len(argv) == 4 else None)
        except PreflightError as e:
            print("init-experiment refused: " + str(e), file=sys.stderr)
            return 2
        print(json.dumps(dict(e, written=EXPERIMENT), sort_keys=True))
        return 0
```

  In `scripts/pairfeat_experiment.sh`:
  - header comment: add `pairfeat6` to the ARM list, "pairfeat6 659 661 673 (validation 677 683 691 701 709); a pairfeat6 parent (its own chain) needs experiment.json from `init-experiment EXPERIMENT_DIR pairfeat6 REFERENCE_FIT_DIR`, which records the separately fitted pairfeat5 reference; it serves baseline, pairfeat5 and pairfeat6, image off";
  - arm case: `case "$ARM" in baseline|pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5|pairfeat6) ;; r1review) ;; *) echo "invalid arm: $ARM (allowed baseline pairfeat1 pairfeat2 pairfeat3 pairfeat4 pairfeat5 pairfeat6 r1review)" >&2; exit 2;; esac`;
  - seed table: `CHAIN="$CAND"; REF=""; REFCHAIN=""` and, after the `r1review)` line, `  pairfeat6) DEV_SEEDS="659 661 673"; VAL_SEEDS="677 683 691 701 709"; REF=pairfeat5; REFCHAIN=experiment;;`;
  - the re-check invocation line gains `"$REF" "$REFCHAIN"` after `"$CHAIN"`, and its code becomes:

```python
import hashlib, json, os, sys
parent, mode, head, runner, analyzer, cand, chain = sys.argv[1:8]
ref, refchain = (sys.argv[8:10] + ["", ""])[:2]
sha = lambda p: hashlib.sha256(open(p, "rb").read()).hexdigest()
name = "validation-manifest.json" if mode == "validation-clean" else "artifact-manifest.json"
with open(os.path.join(parent, name)) as h:
    m = json.load(h)
if m.get("git_head") != head:
    sys.exit(1)
if [k for k in m if k.endswith("_chain_sha256") and k != "baseline_chain_sha256"] != [chain + "_chain_sha256"]:
    sys.exit(1)  # exactly one candidate chain, and it is the chain this run's candidate runs
ep = os.path.join(parent, "experiment.json")
if chain != cand:  # a candidate borrowing a chain is named only by experiment.json
    if not os.path.isfile(ep) or os.path.islink(ep):
        sys.exit(1)
    with open(ep) as h:
        e = json.load(h)
    if not isinstance(e, dict) or e.get("candidate") != cand or e.get("reference") != chain:
        sys.exit(1)
elif refchain == "experiment":  # its own chain plus a separately fitted reference (pairfeat6): experiment.json names both
    if not os.path.isfile(ep) or os.path.islink(ep):
        sys.exit(1)
    with open(ep) as h:
        e = json.load(h)
    if not isinstance(e, dict) or e.get("candidate") != cand or e.get("reference") != ref \
            or not isinstance(e.get("reference_model_chain"), dict):
        sys.exit(1)
elif os.path.lexists(ep):
    sys.exit(1)  # a parent with experiment.json serves only the candidate it names
if mode == "validation-clean" and (m.get("runner_sha256") != sha(runner) or m.get("analyzer_sha256") != sha(analyzer)
                                   or m.get("candidate") != cand):
    sys.exit(1)
if mode == "dev-stress":
    with open(os.path.join(parent, "selection.json")) as h:
        s = json.load(h)
    if s.get("git_head") != head or s.get("selected_modelset") != cand or s.get("candidate") != cand or s.get("pass") is not True:
        sys.exit(1)
```

  The validation manifest of a `pairfeat6` parent carries no `pairfeat5_chain_sha256` key (the map is `reference_model_chain`), so the single-chain-key check holds in every mode.
- [ ] **Step 4: GREEN.** `python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'` (including `ByteIdentity`, `R1reviewByteIdentity`, every r1review class) and `bash -n scripts/pairfeat_experiment.sh`.
- [ ] **Step 5: Commit.**

```bash
git commit -m "Harness: three-arm pairfeat6 candidate against a separately refit pairfeat5 reference" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" \
  -- scripts/pairfeat_compare.py scripts/pairfeat_experiment.sh scripts/test_pairfeat_compare.py
```

### Task E: run once, diagnose, document

Run only after Tasks A-D pass their reviews and a final cross-task review. Shell setup for every step:

```bash
export PATH=/opt/anaconda3/bin:$PATH
H=~/wasl-ml-data/agents2/hardening
EXP=$H/pairfeat6/run-1
REF=$H/pairfeat5/pairfeat6-ref-run-1
```

- [ ] **E1: Full verification** at the final HEAD:

```bash
(cd pipeline && go vet . && go test . -count=1)
python3 -I encoder/test_fit_scorer.py
python3 -I encoder/test_fit_pairfeat1.py
python3 -I -m unittest discover -s scripts -p 'test_*.py'
bash -n scripts/pairfeat_experiment.sh
git status --porcelain --untracked-files=all -- pipeline encoder scripts    # must print nothing
df -h ~                                                                    # at least 20 GB free, else stop
```

- [ ] **E2: Move existing model files aside** (the fit refuses to overwrite them):

```bash
mkdir -p $H/pairfeat6/models-aside/pairfeat5 $H/pairfeat6/models-aside/pairfeat6
find models -maxdepth 1 -name '*_pairfeat6.json' -exec mv {} $H/pairfeat6/models-aside/pairfeat6/ \;
find models -maxdepth 1 -name '*_pairfeat5.json' -exec mv {} $H/pairfeat6/models-aside/pairfeat5/ \;
find models -maxdepth 1 \( -name '*_pairfeat5.json' -o -name '*_pairfeat6.json' \)    # must print nothing
```

- [ ] **E3: Refit the pairfeat5 reference** at this HEAD (output parent must be `hardening/pairfeat5/`):

```bash
python3 -I encoder/fit_pairfeat1.py $REF --modelset pairfeat5 > $H/pairfeat5/pairfeat6-ref-run-1.fit.log 2>&1; echo "exit $?"
python3 -I - "$REF/artifact-manifest.json" "$H/pairfeat5/run-2/artifact-manifest.json" <<'PY'
import json, sys
a, b = (json.load(open(p)) for p in sys.argv[1:3])
k = "pairfeat5_chain_sha256"
print("equal" if a[k] == b[k] else "DIFFERENT")
print(json.dumps({"refit": a[k], "run-2": b[k]}, indent=1, sort_keys=True))
sys.exit(0 if a[k] == b[k] else 1)
PY
```

  Expected `exit 0` and `equal`. If the chain differs from `pairfeat5/run-2`'s, stop and report (the spec requires equality).
- [ ] **E4: Fit pairfeat6** at the same HEAD into the experiment parent:

```bash
python3 -I encoder/fit_pairfeat1.py $EXP --modelset pairfeat6 > $H/pairfeat6/run-1.fit.log 2>&1; echo "exit $?"
python3 -I - "$EXP" "$REF" <<'PY'
import json, os, sys
exp, ref = sys.argv[1:3]
am = json.load(open(os.path.join(exp, "artifact-manifest.json")))
tm = json.load(open(os.path.join(exp, "training-manifest.json")))
fr = json.load(open(os.path.join(exp, "chain", "fit_report_pairfeat6.json")))
idf = json.load(open(os.path.join(exp, "chain", "name_idf_pairfeat6.json")))
r5 = json.load(open(os.path.join(ref, "artifact-manifest.json")))
print(json.dumps({"git_head": am["git_head"], "reference_git_head": r5["git_head"], "max_abs_leaf": am["max_abs_leaf"],
                  "parity": am["parity"], "name_idf_sha256": am["name_idf_sha256"],
                  "idf_registrations": idf["registrations"], "idf_tokens": len(idf["counts"]),
                  "floors": [tm["namesake_base_rows_total"], tm["family_namesake_base_rows_total"], tm["xs_namesake_base_rows_total"]],
                  "namesake_slice": am["namesake_slice"], "family_namesake_slice": am["family_namesake_slice"],
                  "xs_namesake_slice": am["xs_namesake_slice"],
                  "plain_cal_diff_at_merge_0_7": [am["plain_cal_diff_at_merge_0_7"], r5["plain_cal_diff_at_merge_0_7"]],
                  "cal_auc": fr["cal_auc"], "cal_ap": fr["cal_ap"], "nfeat": fr["nfeat"],
                  "hard_negative_slices": fr["hard_negative_slices"]}, indent=1, sort_keys=True))
PY
```

  Stop on exit 3 (a namesake floor, including cross-script) or exit 4 (leaf check) and report. The two `git_head` values must be equal. Report the printed values; `plain_cal_diff_at_merge_0_7` is shown beside pairfeat5's for information only.
- [ ] **E5: Name the experiment:**

```bash
python3 -I scripts/pairfeat_compare.py init-experiment $EXP pairfeat6 $REF
```

- [ ] **E6: One-lane smoke** (seed 101, dev split; not evidence, not a protected seed) with the exact harness argv:

```bash
S=$H/pairfeat6/smoke
mkdir -p $S/bin
(cd pipeline && go build -o $S/bin/wsi7 .)
ln -s "$(readlink bin/nats-server)" $S/bin/nats-server
cat > $S/smoke.py <<'PY'
import json, os, subprocess, sys
root, S = sys.argv[1], sys.argv[2]
sys.path.insert(0, os.path.join(root, "scripts"))
import pairfeat_compare as pfc
bin_ = os.path.join(S, "bin", "wsi7")
o = os.path.join(S, "v2-s101"); os.mkdir(o); os.mkdir(os.path.join(o, "run"))
for name, argv, log in pfc.lane_commands(root, bin_, o, 101, "dev-clean", "pairfeat6", candidate="pairfeat6"):
    with open(os.path.join(o, log), "wb") as h:
        p = subprocess.run(argv, stdout=h, stderr=subprocess.STDOUT)
    print(name, p.returncode, flush=True)
    if p.returncode:
        sys.exit(1)
lm = pfc._load_lane(o, 101, "pairfeat6", "dev-clean", "pairfeat6")
print(json.dumps({k: lm.get(k) for k in ("analysis_errors", "trace_error", "replay_mismatch", "false_closures", "recall",
                                         "false_merge_pairs", "cross_person_edges", "xscript_cap", "linked_pairs")}, indent=1))
PY
python3 -I $S/smoke.py "$PWD" $S > $S/smoke.log 2>&1; echo "exit $?"; cat $S/smoke.log
```

  Expected: every step exits 0; `analysis_errors`, `trace_error`, `replay_mismatch`, `false_closures` are 0; `linked_pairs.edge_classes` present. Stop and diagnose otherwise.
- [ ] **E7: Development, stress, validation**, stopping at the first failed gate. Check `df -h ~` (>= 20 GB free) before each mode. Run each mode as one background script (Bash `run_in_background`), arms sequentially:

```bash
cat > $EXP/dev.sh <<'SH'
#!/bin/bash
set -e
cd "$1"; export PATH=/opt/anaconda3/bin:$PATH; P="$2"
for a in baseline pairfeat5 pairfeat6; do
  scripts/pairfeat_experiment.sh $P/dev-$a dev-clean $a > $P/dev-$a.log 2>&1
  echo "arm $a done $(date)"
done
python3 -I scripts/pairfeat_compare.py dev-clean $P/dev-clean.json baseline=$P/dev-baseline pairfeat5=$P/dev-pairfeat5 pairfeat6=$P/dev-pairfeat6
SH
bash $EXP/dev.sh "$PWD" $EXP > $EXP/dev.log 2>&1
python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d["pass"], d["reference_gate"]["reasons"], d["gate"]["reasons"])' $EXP/dev-clean.json
```

  On pass, the same script shape for stress (roots `$EXP/stress-<arm>`, mode `dev-stress`, analysis `dev-stress $EXP/dev-stress.json`), then `python3 -I scripts/pairfeat_compare.py freeze-validation $EXP`, then validation (roots `$EXP/val-<arm>`, mode `validation-clean`, analysis `validation-clean $EXP/validation-clean.json`). A lane crash is set aside (`mv $EXP/<root> $EXP/<root>-crashed-1`), its cause recorded, and the arm re-run once; a second crash stops the run. Validation seeds are not inspected before `validation-manifest.json` exists.
- [ ] **E8: Read-only diagnosis** (scripts and outputs under `$EXP/diag/`, outside Git; no lane file is written):
  - from the analysis JSONs: per arm and seed, `linked_pairs` (same-person = true merges; edge classes on development), `xscript_cap` (pairs, same-person, different-person), `reference_gate.reported`, vs-baseline gate (reported);
  - per development lane of `pairfeat5` and `pairfeat6`: the final linked different-person pairs (`pfc.final_linked_pairs`, truth `pk_qid`) with `pfc.edge_class` over `pfc.cohort_registrations`, the highest `p` in the trace, both-DOB-placeholder flag, script mismatch; which were removed by `pairfeat6`, which remain, which are new;
  - true merges lost and gained by `pairfeat6` vs `pairfeat5` (same attributes), to show whether the hard negatives taught "opposite script and placeholder date means different person" on real pairs;
  - CRC closures gained and lost vs `pairfeat5` (final decision per loop from `run/main/out_log.ndjson`, as in the r1review diagnosis).
- [ ] **E9: Document** in `pipeline/RESULTS.md` (new section after the r1review section, titled with the experiment HEAD and outcome) and `docs/BACKEND-HANDOFF.md` (a paragraph after the r1review paragraph, same style). State built/simulated. Include: the change; seeds; the gates table verbatim; per-mode results (recall per arm, gain vs `pairfeat5` with the paired t interval and n, false-merge pairs, cross-person edges, impure clusters, uncapped wrong-pair reviews, reviewer hours, SAFETY counts in all arms); the vs-baseline gate (reported); the reported-only metrics (diagnosed classes on development, true merges, capped pairs, rarity SHA256, table N); fit facts (max |leaf|, floors, slices, parity); the reference refit's equality with `pairfeat5/run-2`; the smoke; every rerun and the stop point. Disclose:
  - the design came from a post-hoc read-only diagnosis of used development lanes 571 577 587;
  - the hard negatives are built to the diagnosed pattern of our own synthetic generator, so a gain may partly reflect learning the generator rather than real naming;
  - the spec resolutions above that shape results (1, 2, 4, 5, 9);
  - default model, noise and floor 0.2 unchanged; nothing promoted to default; no clinical effectiveness proven.

  Then get an independent doc review (a reviewer given the two diffs, the analysis JSONs and the spec, checking every number, the disclosures and the constraints: no `/Users` or `/tmp` path, no identifier). Fix, then commit those two files only:

```bash
grep -n -e '/Users' -e '/tmp' pipeline/RESULTS.md docs/BACKEND-HANDOFF.md      # no new hit
git add pipeline/RESULTS.md docs/BACKEND-HANDOFF.md
git commit -m "Document pairfeat6: placeholder-date and name-rarity features with cross-script hard negatives vs pairfeat5" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" -- pipeline/RESULTS.md docs/BACKEND-HANDOFF.md
git show --stat HEAD                                                            # exactly the two files
```

- [ ] **E10: Thin non-record runs.** Keep the record run `$EXP` whole (lanes, CSVs, chain, manifests, analyses). Remove raw data of everything else, keeping summaries, manifests, sidecars and logs:

```bash
rm -rf $H/pairfeat6/smoke/v2-s101 $H/pairfeat6/smoke/bin
rm -rf $H/pairfeat6/taskB-identity/base-src
find $H/pairfeat6/taskB-identity -maxdepth 1 \( -name '*.csv' -o -name 'wsi7.*' \) -delete
find $REF -maxdepth 1 -name 'feat_*.csv' -delete
find $EXP -maxdepth 1 -type d -name '*-crashed-*' -exec rm -rf {} +
rm -rf $H/pairfeat6/plan-goldens
du -sh $H/pairfeat6 $REF
```

---

## Self-review (writing-plans)

**Spec coverage.**
- Four features `f30`–`f33`, appended after the 30, only for `pairfeat6`, byte-identical otherwise: Task A (`pairfeat6Feats`, `NameIDF` gate in `pairFeats`, golden test, `TestPairFeatsPairfeat6Appended`); Task B identity hashes at 5,000 persons.
- Rarity weight `log(N/(1+c))`, unseen `c = 0`, built once from the training split's registrations, `models/name_idf_pairfeat6.json`, in the chain SHA256: Task A (`Weight`, loader), Task B (`wsi nameidf`), Task C (`chain_files`, artifact manifest), Task D (five-file `CHAIN_FILES`, `rarity_table_sha256`).
- `featdump -xsnamesakes 0.10`, opposite script, same sex, same 1 January placeholder of the same year, shared given name, other tokens different, sidecar counts, floor: Task B (`buildXSNamesakes`, `xsRegister`, sidecar keys, `TestXSFeatDump`), Task C (floor `(cross-script)`).
- Fit, calibration, parity, leaf check as `pairfeat5`, `lambda_l2` 1.0, cap 0.69 kept: Task C (`PROTOCOLS["pairfeat6"]` from `pairfeat5`, `--nfeat 34`, widths), Task A (`xscriptCapFor`).
- Experiment arms baseline / `pairfeat5` refit at the same HEAD equal to `run-2` / `pairfeat6`, image off, no `-r1review`: Task D (`arms`, guard, `init_experiment` HEAD binding), Task E (E3 equality check).
- Seeds and never-touch list: Global Constraints, Task D `test_seeds_never_touched_before` and guard refusals.
- Gates table and SAFETY in every arm, vs-baseline reported, reported-only metrics: Task D (`_pairfeat6_gate`, `baseline_gating: False`, `linked_pair_report`, `edge_class`, `xscript_cap`, `rarity_table_sha256`).
- Write-once selection → stress → freeze → validation, same-HEAD binding, guard: unchanged machinery extended through `governing_view` / `arm_chain`; Task D Preflight and CLI tests.
- Constraints and disclosures, thinning: Global Constraints, E9, E10.

**Placeholder scan.** No "TBD"/"similar to Task N"; every new function has code; the two Go goldens and two Python digests are real values taken at `931a823` during planning, each with a Step 0 command that re-derives them. E9's figures are run results, not code.

**Type consistency.** `NameIDFTable.Registrations` (int) / `Counts` (`map[string]int`) are written by `runNameIDF` and read by `readNameIDF` and `Weight`; `fdReg` fields `pid, name, dob, sex, eid` match their uses in `runFeatDump`, `xsRegister` and the tests; `xs_base_namesake_rows` / `xs_namesakes_added` / `xs_rows` are written by Go and read by `check_namesake_floor`, `training_manifest` and `xs_namesake_slice`; `name_idf_pairfeat6.json` is the same name in `chain_files`, `CHAIN_FILES`, `nameIDFFor` and `.gitignore`'s glob; `reference_model_chain` and `reference_artifact_manifest_sha256` are the same keys in `init_experiment`, `read_experiment`, `governing_view`, `freeze_validation`, the runner re-check and the tests; `reference_gate(ref, cand, kind, base)` keeps its signature and dispatches to `_pairfeat6_gate`; `lane_metrics(..., completion=None, linked=None)` matches `_load_lane`'s call.
