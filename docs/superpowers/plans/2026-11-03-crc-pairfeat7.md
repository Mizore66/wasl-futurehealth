# CRC pairfeat7: missing-date and same-real-date hard negatives — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Test whether `pairfeat6` plus two training-only hard-negative types (missing-date namesakes sharing only a given name, and same-real-date strangers with no shared skeleton token) removes cross-person auto-merges relative to a separately refit `pairfeat5` without losing CRC recall, with SAFETY zero in every arm.

**Architecture:** New modelset `pairfeat7`. It reuses `pairfeat6`'s features `f30`–`f33`, its leave-own-out rarity table v2 and its cross-script review cap through one predicate, `usesNameIDF(modelSet)` (`pairfeat6` or `pairfeat7`), with its own table file `name_idf_pairfeat7.json`; no feature or table code is copied. A new `pipeline/pairfeat7.go` adds `featdump -mdnamesakes RATE` and `-rdnamesakes RATE` (with `-nameidf` only), appended after the cross-script negatives with their own generators, so every dump without them is byte-identical. `encoder/fit_pairfeat1.py --modelset pairfeat7 --reference REF` runs the `pairfeat6` protocol plus the two rates, enforces a 500-row floor per kind, writes per-kind slices, and prints the plain-calibration different-at-merge-0.7 count beside the reference fit's. The harness gains candidate `pairfeat7` on `pairfeat6`'s three-arm machinery and gate, with two more reported development edge classes from `edge_class7`, a refinement of `edge_class`.

**Tech Stack:** Go 1.22+, Python 3 (numpy, scikit-learn, scipy, lightgbm 4.7.0), Bash.

**Spec:** `docs/superpowers/specs/2026-11-03-crc-pairfeat7-design.md` (binding, user-approved).

## Global Constraints

- BASE for code is `acefac5` (HEAD at planning; its `pipeline/*.go`, `encoder`, `scripts` and `.gitignore` equal `d042ba9`'s, the `pairfeat6` experiment HEAD). With any modelset other than `pairfeat7` (default, `pairfeat1`-`pairfeat6`), every `featdump` CSV and sidecar, the pair-feature vector, linker behaviour, traces and every output are byte-identical to `acefac5`; `featdump` without `-mdnamesakes`/`-rdnamesakes` is byte-identical to `acefac5` whatever its other flags. `encoder/fit_pairfeat1.py` for `pairfeat1`-`pairfeat6` produces byte-identical argv, outputs and manifests. `encoder/fit_scorer.py` is not edited (`--nfeat 34` already exists).
- `pairfeat1`-`pairfeat6` and `r1review` driver and harness behaviour, seeds, gates, outputs and tests are unchanged (byte-identity pinned by digests: Go goldens in Task A, `FIT_GOLDEN` in Task C, `P6_GOLDEN` and the existing r1review digests in Task D). In existing tests only these lines may change: candidate-set equalities (`CANDIDATES`, `ALLOWED_ARMS`, `MODELSET`, `sorted(fp.PROTOCOLS)`), the unknown-candidate sentinels (`pairfeat7` moves to `pairfeat8`), and the `Pairfeat6Candidate` line that excludes candidates other than `P6` from `reference_chain_in_experiment` (it now excludes `(P6, P7)`).
- `pairfeat7` seeds, never used:
  - development 719 727 733;
  - validation 739 743 751 757 761.
- Never touch: development 307–587 as used and 659 661 673; validation 401–431, 601–619, 631 641 643 647 653 and 677 683 691 701 709 (pairfeat6's, never run, kept unused); B8/frozen 11 23 37 41 59 17 29. Spelled out: development 307 311 313 317 331 337 347 353 367 433 449 461 521 541 547 557 563 569 571 577 587 659 661 673; validation 401 409 419 421 431, 601 607 613 617 619, 631 641 643 647 653, 677 683 691 701 709; B8/frozen 11 23 37 41 59 17 29.
- Raw artifacts under `~/wasl-ml-data/agents2/hardening/pairfeat7/` (the reference refit under `~/wasl-ml-data/agents2/hardening/pairfeat5/`, which `encoder/fit_pairfeat1.py` requires). No `/tmp`. No `/Users` paths, no identifiers, secrets, binaries or models in Git.
- Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`. No push or merge.
- `PATH=/opt/anaconda3/bin:$PATH`. Run Python tests by direct path or `python3 -I -m unittest discover -s scripts -p '<pattern>'`; run Go tests from `pipeline/`. Build every binary from a `git archive` tree with `go build -buildvcs=false` (an archive has no `.git`).
- Path-only commits: `git add` only the task's new files, then `git commit -m ... -m ... -- <the task's paths>`. Check `git show --stat HEAD` lists exactly those paths.
- Do not edit `pipeline/RESULTS.md` or `docs/BACKEND-HANDOFF.md` before Task E9: another session is writing the `pairfeat6` result there. E9 starts only after that work is committed (`git status --porcelain -- pipeline/RESULTS.md docs/BACKEND-HANDOFF.md` prints nothing).
- Go formatting: `gofmt -l` must print nothing for the files this plan creates and for `pairfeat6.go`, `xscript.go`, `encdev.go`. Do not run `gofmt -w` on `pipeline/main.go` (it is not gofmt-clean at BASE; reformatting it would add unrelated changes).
- Planning left two scratch directories outside Git under `~/wasl-ml-data/agents2/hardening/pairfeat7/`: `plan-goldens/` (a BASE binary and the 400-person `pairfeat6` dumps behind the Go goldens) and `plan-scratch/` (a local clone at `acefac5` with this plan's code applied, used to verify every test below). Neither is read by any task; both are removed in E10.

## Spec resolutions (decided here; disclose in the docs)

1. **Modelset mapping.** `usesNameIDF(ms)` is true for `pairfeat6` and `pairfeat7`; `nameIDFFor` loads `<wasl>/name_idf_<ms>.json` for both, `xscriptCapFor` and the `v7strict`-only policy follow it. Features `f30`–`f33`, the v2 leave-own-out table (`Person.Idx`, `looScope`) and `wsi nameidf` are unchanged; the fit's `uses_name_idf` is already "nfeat == 34", so `pairfeat7` gets the table under its own names (`chain_files`, `nameidf_argv`). `featdump` stays switched to 34 features by `-nameidf`, and the new flags need `-nameidf` (as `-xsnamesakes`).
2. **Given name** = the first runtime token: `tokensOf(latinForm(En))`, which drops articles and the `al-` prefix ("Mohammed Al Salem" is `mohammed salem`). Its skeleton `g` must be non-empty.
3. **"No other skeleton token"**: the two names' skeleton-token sets intersect in exactly `{g}`.
4. **"No family token that is a prefix or truncation of the other's"** (`givenOnly`): both names have at least two tokens, and with family tokens = every token after the first, (a) no family token of one name is a prefix of a family token of the other (equal tokens count as prefixes), and (b) the same for their skeletons. Excluded examples: "Mohammed S" (initial), "Abdullah Rahmani" vs "Rahman" (truncation), "Mohammed Abdullah" vs "Abd Karim" (`abd` prefix of `abdullah`), "Mohammed Salman" vs "Salem" (skeleton `slm` prefix of `slmn`), "Selim"/"Salem" (same skeleton). Single-token names get none.
5. **Registrations** (`hnRegister`, as `xsRegister`): the base person with their true birth date and sex in a drawn script (Arabic with p = 1/2), the namesake in the same script with p = 1/2, else the opposite; no DOB or sex noise; Emirates IDs drawn by `drawEID`; the namesake has no aliases and no persons-file index (`Idx` 0), so it never enters, and is never left out of, the rarity table.
6. **Missing-date donor**: the split pool (file order), a different QID, the same sex, `givenOnly`, a 10-character DOB; drawn uniformly. The namesake takes the donor's names, country, origin and Arabic source, the base person's sex and split, DOB "" and an Emirates ID drawn for the donor's birth year.
7. **Same-real-date stranger** (flag): the base date must be real and not a placeholder (`realDob` and not `dobPlaceholder6`: not 1 January, 1 July or day 00). Donors: a different QID, the same sex, both skeletons non-empty and disjoint. The spec does not say how to pick the donor; a uniform draw gives 4 and 11 candidate rows per 5,000-person seed (1019, 1021), so the 500-row floor over five seeds is unreachable: with no shared skeleton token, a pair reaches `featdump`'s candidates only through the top-10 cosine list (every rule key needs a shared skeleton token). The donor is therefore the eligible person whose English-name embedding (`-encfile`) has the largest dot product with the base person's (first in pool order on a tie): 602 rows at seed 1019. These are the look-alike strangers that merged in the diagnosis. Cost: the donor search embeds the split pool (46,651 train persons), about +185 s per training dump, about +15 minutes per fit.
8. **Generator order and seeds**: `md` then `rd`, after the cross-script kind, each with its own `rand.New(rand.NewSource(...))` (`mdSeed`, `rdSeed`); their registrations are appended before the shuffle. A rate of 0 draws nothing and loads no encoder, so every dump without the flags is byte-identical.
9. **Floors and rows**: "rows" of a kind are rows pairing a namesake with any registration of its base person (`namesakePairRow`, as the other kinds); each kind needs at least 500 rows summed over the five training seeds (messages `namesake floor not met (missing-date)` / `(same-date)`). The sidecar gains `md_rate`, `md_namesakes_added`, `md_base_namesake_rows`, `md_rows` and the `rd_` equivalents, only for a kind whose rate is > 0.
10. **Reported fit numbers**: per-kind slices of the namesake-calibration dump (rows, different-person, different at review 0.01, different at merge 0.7) for `xs`, `md` and `rd`; the plain-calibration different-at-merge-0.7 count beside the reference's. The reference count needs the reference fit, so `pairfeat7` requires `--reference REF_DIR`: its `artifact-manifest.json` must carry exactly the `pairfeat5` chain, at the same git HEAD, with an integer `plain_cal_diff_at_merge_0_7`; the artifact manifest records it as `plain_cal_diff_at_merge_0_7_reference`. Nothing here gates.
11. **New edge classes** (`edge_class7`, harness, reported only): a refinement of `edge_class`, so merging the new classes back gives `pairfeat6`'s class. `missing_dob_given_only` = what `edge_class` calls `missing_dob_ambiguous` where the registration names (ordered lowercase alphabetic tokens) have two or more tokens each, equal first tokens, no other shared token, and no later token of one a prefix of a later token of the other. `same_real_dob_different_names` = what `edge_class` calls `other` where both dates are the same real, non-placeholder date (valid, 1900–2100, not 01-01 or 07-01) and the names share no token. A same-date pair with a shared token stays `shared_token`; a cross-script one stays `cross_script_lookalike`. The harness rule works on raw tokens, not skeletons (the generator rule is stricter), and is applied only to `pairfeat7` experiments; `pairfeat6` and `r1review` outputs are unchanged.
12. **Gate**: `pairfeat6`'s gate is reused unchanged (`reference_gate: "pairfeat6"`); its arm check is parameterised by the candidate ("arms must be pairfeat7 vs its reference arm pairfeat5"). `pairfeat6`'s output is unchanged (digest).
13. **SAFETY zero in every lane of every arm** is checked on arm sums (counts are non-negative), baseline included, as in `pairfeat6`. `truth_closed` must be equal in all three arms.
14. **Fit paths**: reference refit to `hardening/pairfeat5/pairfeat7-ref-run-1` (its chain must equal `pairfeat5/run-2`'s), candidate to `hardening/pairfeat7/run-1`.
15. **vs-baseline gate**: computed and reported, not gating; E8 also reports `gate(baseline, pairfeat5)` for context.
16. **Protected seeds**: `pairfeat6`'s seeds are protected by the guard (a lane may run only its own candidate's seeds) and pinned by tests; they are not added to `RETIRED_SEEDS`, which would also refuse them inside `pairfeat6`'s own experiment and so change `pairfeat6`'s behaviour.
17. The tasks are named A–E (A–D build, E runs).

---

### Task A: the modelset mapping (Go)

**Files:**
- Create `pipeline/pairfeat7_test.go`.
- Modify:
  - `pipeline/pairfeat6.go`: the `nameIDFFor` comment and body (lines 118–137 at BASE), a new `usesNameIDF` above it;
  - `pipeline/xscript.go`: `xscriptCapFor` and its comment (lines 12–13);
  - `pipeline/encdev.go`: `validateModelPolicy`, the `pairfeat6` block (lines 44–46).

**Interfaces:**
- Produces (`pipeline/pairfeat6.go`): `func usesNameIDF(modelSet string) bool` (true for `pairfeat6`, `pairfeat7`); `nameIDFFor(cmd, path, modelSet, wasl)` loads `<wasl>/name_idf_<modelSet>.json` when `usesNameIDF(modelSet)` (signature unchanged).
- Produces (`pipeline/xscript.go`): `xscriptCapFor` true for `pairfeat5`, `pairfeat6`, `pairfeat7`.
- Produces (`pipeline/encdev.go`): `validateModelPolicy("pairfeat7", p)` refuses every `p` but `v7strict` with `modelset pairfeat7 supports only -policy v7strict (got "p"); ...`; `pairfeat6`'s message is byte-identical.
- Produces (test helpers in `pipeline/pairfeat7_test.go`, used by Task B): `cliState(t)`, `pf7Table(t) (*NameIDFTable, []byte)`, `pf6Dump(t, split, seed, xs, tb) (csvB, scB []byte)`.
- Consumes: `runNameIDF`, `runFeatDump`, `LoadNameIDF`, `NamesakeRate`, `FamilyNamesakeRate`, `XSNamesakeRate`, `NameIDF`, `UrduFix`, `NoiseModel`, `Providers` and the programme provider lists; test helper `hex256` (`namesake_test.go`).

- [ ] **Step 0: Confirm BASE and re-derive the goldens** (nothing edited yet):

```bash
git diff --quiet acefac5 HEAD -- 'pipeline/*.go' encoder scripts .gitignore && echo "code at BASE"    # must print it
G=~/wasl-ml-data/agents2/hardening/pairfeat7/plan-goldens
mkdir -p $G/src
git archive acefac5 pipeline | tar -x -C $G/src
(cd $G/src/pipeline && go build -buildvcs=false -o $G/wsi7.base .)
FD=(featdump -wasl "$PWD/data" -encfile "$PWD/models/encoder_weights.json" -persons 400 -noise v1t)
$G/wsi7.base nameidf -wasl "$PWD/data" -split train -persons 400 -noise v1t -idfseeds 1019 -out $G/idf1019.json
$G/wsi7.base "${FD[@]}" -split train -seed 1019 -namesakes 0.1 -familynamesakes 0.1 -xsnamesakes 0.1 \
  -nameidf $G/idf1019.json -out $G/train1019.pf6.csv 2> $G/train.log
$G/wsi7.base "${FD[@]}" -split cal -seed 2011 -nameidf $G/idf1019.json -out $G/cal2011.pf6.csv 2> $G/cal.log
shasum -a 256 $G/idf1019.json $G/train1019.pf6.csv $G/train1019.pf6.csv.namesakes.json $G/cal2011.pf6.csv
rm -rf $G/src
```

  Expected, in order: `c6491f0ae65573e417cf9539684afcdfe74ad497643b32637ec2484fca987512`, `b151d686140719a1d92b164de18a53159977804a0f81724d757f64bb6ed63d01`, `d08510f57fd43b124a8d84b0bd089a5600817665b81188a26455edd0398bd79b`, `7f5cd895b195bbbbf2a8ac8c953f7cea8f7ee637bbed8c10c1fc4a9c1d6d7a39`. Stop if any differs. (The in-process test below takes the same values: `cliState` sets the CLI's Urdu fix and `v1t` noise and drops the programme providers that earlier tests register.)

- [ ] **Step 1: Failing tests.** Create `pipeline/pairfeat7_test.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pairfeat6 training dumps at 400 persons, taken with the CLI (UrduFix on) at acefac5, before pairfeat7:
// wsi nameidf -split train -persons 400 -noise v1t -idfseeds 1019; featdump -split train -persons 400 -noise v1t
// -seed 1019 -namesakes 0.1 -familynamesakes 0.1 -xsnamesakes 0.1 -nameidf <that table>; featdump -split cal
// -persons 400 -noise v1t -seed 2011 -nameidf <that table>.
const (
	goldenPF6IDF1019     = "c6491f0ae65573e417cf9539684afcdfe74ad497643b32637ec2484fca987512"
	goldenPF6Train1019   = "b151d686140719a1d92b164de18a53159977804a0f81724d757f64bb6ed63d01"
	goldenPF6Train1019Sc = "d08510f57fd43b124a8d84b0bd089a5600817665b81188a26455edd0398bd79b"
	goldenPF6Cal2011     = "7f5cd895b195bbbbf2a8ac8c953f7cea8f7ee637bbed8c10c1fc4a9c1d6d7a39"
)

// cliState: the CLI's UrduFix and noise v1t, with only the base providers (earlier tests may leave the lung/DR/
// breast/LDCT/IFHAS programme providers registered; the CLI goldens, with no programme data in ../data, have none).
func cliState(t *testing.T) {
	t.Helper()
	oldU, oldP, oldN := UrduFix, Providers, NoiseModel
	t.Cleanup(func() { UrduFix, Providers, NoiseModel = oldU, oldP, oldN })
	UrduFix, NoiseModel = true, "v1t"
	prog := map[string]bool{}
	for _, l := range [][]ProvCfg{lungProviders, drProviders, breastProviders, ldctProviders, ifhasProviders} {
		for _, p := range l {
			prog[p.ID] = true
		}
	}
	Providers = nil
	for _, p := range oldP {
		if !prog[p.ID] {
			Providers = append(Providers, p)
		}
	}
}

// pf7Table: wsi nameidf -split train -persons 400 -idfseeds 1019, in process.
func pf7Table(t *testing.T) (*NameIDFTable, []byte) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "name_idf_pairfeat7.json")
	runNameIDF("../data", "train", p, 400, []int64{1019})
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return LoadNameIDF(p), b
}

// pf6Dump: an in-process featdump of 400 persons with name-rarity table tb: on the train split with given-name and
// family namesakes 0.1, on any other split without them, and cross-script namesakes at rate xs.
func pf6Dump(t *testing.T, split string, seed int64, xs float64, tb *NameIDFTable) (csvB, scB []byte) {
	t.Helper()
	oldR, oldF, oldX, oldI := NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF
	defer func() { NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF = oldR, oldF, oldX, oldI }()
	ns := 0.0
	if split == "train" {
		ns = 0.1
	}
	NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF = ns, ns, xs, tb
	out := filepath.Join(t.TempDir(), "o.csv")
	runFeatDump("../data", "../models/encoder_weights.json", "", split, out, 400, seed)
	var err error
	if csvB, err = os.ReadFile(out); err != nil {
		t.Fatal(err)
	}
	if scB, err = os.ReadFile(out + ".namesakes.json"); err != nil {
		t.Fatal(err)
	}
	return
}

func TestPairfeat6DumpsUnchangedGolden(t *testing.T) {
	cliState(t)
	tb, raw := pf7Table(t)
	if hex256(raw) != goldenPF6IDF1019 {
		t.Errorf("name-rarity table %s", hex256(raw))
	}
	c, s := pf6Dump(t, "train", 1019, 0.1, tb)
	if hex256(c) != goldenPF6Train1019 || hex256(s) != goldenPF6Train1019Sc {
		t.Errorf("pairfeat6 train dump: csv %s sidecar %s", hex256(c), hex256(s))
	}
	c, _ = pf6Dump(t, "cal", 2011, 0, tb)
	if hex256(c) != goldenPF6Cal2011 {
		t.Errorf("pairfeat6 cal dump: csv %s", hex256(c))
	}
}

func TestPairfeat7ModelsetTables(t *testing.T) {
	for ms, want := range map[string]bool{"": false, "pairfeat1": false, "pairfeat5": false, "pairfeat6": true, "pairfeat7": true, "pairfeat8": false} {
		if usesNameIDF(ms) != want {
			t.Errorf("usesNameIDF(%q) = %v", ms, !want)
		}
	}
	for ms, want := range map[string]bool{"": false, "pairfeat4": false, "pairfeat5": true, "pairfeat6": true, "pairfeat7": true, "pairfeat8": false} {
		if xscriptCapFor(ms) != want {
			t.Errorf("xscriptCapFor(%q) = %v", ms, !want)
		}
	}
	for _, p := range []string{"", "v6", "v7ltt"} {
		err := validateModelPolicy("pairfeat7", p)
		if err == nil || !strings.Contains(err.Error(), "modelset pairfeat7 supports only -policy v7strict") {
			t.Errorf("validateModelPolicy(pairfeat7, %q) = %v", p, err)
		}
		if err := validateModelPolicy("pairfeat6", p); err == nil ||
			err.Error() != `modelset pairfeat6 supports only -policy v7strict (got "`+p+`"); it keeps pairfeat5's cross-script review cap, selected under v7strict` {
			t.Errorf("pairfeat6 policy message changed: %v", err)
		}
	}
	if err := validateModelPolicy("pairfeat7", "v7strict"); err != nil {
		t.Error(err)
	}
	d := t.TempDir()
	for ms, n := range map[string]int{"pairfeat6": 6, "pairfeat7": 7} {
		body := fmt.Sprintf(`{"version":1,"registrations":%d,"counts":{"a":1}}`, n)
		if err := os.WriteFile(filepath.Join(d, "name_idf_"+ms+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for ms, n := range map[string]int{"pairfeat6": 6, "pairfeat7": 7} {
		for _, cmd := range []string{"orchestrate", "audit", "report7"} {
			tb, err := nameIDFFor(cmd, "", ms, d)
			if err != nil || tb == nil || tb.Registrations != n {
				t.Errorf("nameIDFFor(%s, %s) = %+v, %v; want its own table (N=%d)", cmd, ms, tb, err, n)
			}
		}
	}
	if tb, err := nameIDFFor("featdump", "", "pairfeat7", d); tb != nil || err != nil {
		t.Errorf("featdump without -nameidf loaded %+v, %v", tb, err)
	}
	if _, err := nameIDFFor("orchestrate", "", "pairfeat7", t.TempDir()); err == nil {
		t.Error("pairfeat7 without a table in -wasl accepted")
	}
}
```

- [ ] **Step 2: RED.** `cd pipeline && go test . -run 'Pairfeat6DumpsUnchangedGolden|Pairfeat7ModelsetTables' -count=1` fails to compile (undefined `usesNameIDF`).
- [ ] **Step 3: Implement** (shown as a diff against BASE):

```diff
diff --git a/pipeline/pairfeat6.go b/pipeline/pairfeat6.go
index 73e7147..edc842e 100644
--- a/pipeline/pairfeat6.go
+++ b/pipeline/pairfeat6.go
@@ -115,9 +115,13 @@ func (s looScope) weight(tok string) float64 {
 	return math.Log(float64(s.n) / float64(1+c))
 }
 
-// nameIDFFor: the table a process uses. featdump takes it only from -nameidf (pairfeat6 training dumps); every other
-// command loads <wasl>/name_idf_pairfeat6.json when -modelset is pairfeat6 and uses none otherwise. -nameidf on any
-// command other than featdump is a configuration error.
+// usesNameIDF: the modelsets whose chain carries a name-rarity table name_idf_<modelset>.json and pair features
+// f30..f33 (pairfeat7 reuses pairfeat6's features and table code under its own file names).
+func usesNameIDF(modelSet string) bool { return modelSet == "pairfeat6" || modelSet == "pairfeat7" }
+
+// nameIDFFor: the table a process uses. featdump takes it only from -nameidf (pairfeat6 and pairfeat7 training dumps);
+// every other command loads <wasl>/name_idf_<modelset>.json when usesNameIDF(-modelset) and uses none otherwise.
+// -nameidf on any command other than featdump is a configuration error.
 func nameIDFFor(cmd, path, modelSet, wasl string) (*NameIDFTable, error) {
 	if cmd == "featdump" {
 		if path == "" {
@@ -128,8 +132,8 @@ func nameIDFFor(cmd, path, modelSet, wasl string) (*NameIDFTable, error) {
 	if path != "" {
 		return nil, fmt.Errorf("-nameidf is valid only for featdump (got command %q)", cmd)
 	}
-	if modelSet == "pairfeat6" {
-		return readNameIDF(filepath.Join(wasl, "name_idf_pairfeat6.json"))
+	if usesNameIDF(modelSet) {
+		return readNameIDF(filepath.Join(wasl, "name_idf_"+modelSet+".json"))
 	}
 	return nil, nil
 }
diff --git a/pipeline/xscript.go b/pipeline/xscript.go
index b2ff666..369c72e 100644
--- a/pipeline/xscript.go
+++ b/pipeline/xscript.go
@@ -9,8 +9,9 @@ package main
 // XScriptCap: set in main.go from xscriptCapFor(ModelSet) after -modelset passes validateModelPolicy.
 var XScriptCap bool
 
-// xscriptCapFor: the cap is on for pairfeat5 and pairfeat6 (pairfeat6 keeps every pairfeat5 setting), off otherwise.
-func xscriptCapFor(modelSet string) bool { return modelSet == "pairfeat5" || modelSet == "pairfeat6" }
+// xscriptCapFor: the cap is on for pairfeat5 and for pairfeat6 and pairfeat7 (usesNameIDF; both keep every pairfeat5
+// setting), off otherwise.
+func xscriptCapFor(modelSet string) bool { return modelSet == "pairfeat5" || usesNameIDF(modelSet) }
 
 // XScriptCapP: the capped model score.
 const XScriptCapP = 0.69
```

  In `pipeline/encdev.go`, `validateModelPolicy`, replace the `pairfeat6` block (lines 44–46):

```go
	if usesNameIDF(modelSet) && policy != "v7strict" { // pairfeat6, pairfeat7
		return fmt.Errorf("modelset %s supports only -policy v7strict (got %q); it keeps pairfeat5's cross-script review cap, selected under v7strict", modelSet, policy)
	}
```

- [ ] **Step 4: GREEN.** `cd pipeline && go test . -count=1 && go vet . && gofmt -l pairfeat7_test.go pairfeat6.go xscript.go encdev.go` (all suites pass, including `TestPairfeat6DumpsUnchangedGolden`, `TestPairfeat6PolicyAndCap`, `TestNameIDFFor`, `TestPairfeat6OffFeatDumpGolden`; `gofmt -l` prints nothing). The full suite takes about 2 minutes.
- [ ] **Step 5: Commit.**

```bash
git add pipeline/pairfeat7_test.go
git commit -m "pairfeat7: modelset mapping (usesNameIDF) reusing pairfeat6's features, table and review cap" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" \
  -- pipeline/pairfeat7_test.go pipeline/pairfeat6.go pipeline/xscript.go pipeline/encdev.go
git show --stat HEAD    # exactly the four files
```

### Task B: missing-date and same-real-date hard negatives in featdump (Go)

**Files:**
- Create `pipeline/pairfeat7.go`, `pipeline/pairfeat7_negatives_test.go`.
- Modify:
  - `pipeline/encdev.go`: the `runFeatDump` header comment (line 289), the call after the cross-script block (after line 334 `}` closing `if XSNamesakeRate > 0`), the row loop (after the `xsRows` append, lines 369–371), the sidecar (before line 419 `sc, err := json.Marshal(scm)`);
  - `pipeline/main.go`: two flags before line 148 (`-xsnamesakes`), validation after the `validateXSNamesakes` block (lines 209–212), two comments (lines 275, 280).

**Interfaces:**
- Consumes (Task A): `usesNameIDF`; test helpers `cliState`, `pf7Table`, `pf6Dump`.
- Consumes (existing): `Person`, `fdReg{pid, name, dob, sex, eid, tid}`, `fdName`, `drawEID`, `newEID`, `tokensOf`, `latinForm`, `skeleton`, `skelTok`, `realDob`, `dobPlaceholder6`, `dot`, `LoadEncoder(...).Embed`, `validateNamesakeRate`, `namesakePairRow`; test helpers `intList`, `luhnOK`, `isAr`, `namesakeSeed`, `familySeed`, `xsSeed`.
- Produces (`pipeline/pairfeat7.go`):
  - `var MDNamesakeRate, RDNamesakeRate float64` (flags `-mdnamesakes`, `-rdnamesakes`, default 0)
  - `func mdSeed(seed int64) int64` = `seed*2654435761 + 181081`; `func rdSeed(seed int64) int64` = `seed*2654435761 + 206233`
  - `func validateIDFNamesakes(flag, cmd string, rate float64, nameIDF string) error`
  - `type hnName struct{ tok, skel []string }`, `func newHNName(en string) hnName`, `func strSet(s []string) map[string]bool`, `func prefixRelated(a, b []string) bool`
  - `func givenOnly(a, b hnName) bool`, `func noSharedSkeleton(a hnName, aset map[string]bool, b hnName) bool`
  - `func buildMDNamesakes(ps, pool []Person, rate float64, r *rand.Rand) ([]Person, []int)`
  - `func buildRDNamesakes(ps, pool []Person, rate float64, r *rand.Rand, emb func(string) []float32) ([]Person, []int)`
  - `func hnRegister(regs []fdReg, bpid int, b *Person, xpid int, x *Person, r *rand.Rand) []fdReg`
  - `type hnKind struct { key string; rate float64; base map[int]int; added int; rows []int }`
  - `func pairfeat7Negatives(regs []fdReg, ps, pool []Person, seed int64, first int, encPath string) ([]fdReg, []*hnKind)`
- Produces (sidecar, only for a kind with rate > 0): `md_rate` (float), `md_namesakes_added` (int), `md_base_namesake_rows` (int), `md_rows` ([]int), and the same with `rd_`. CSV width is unchanged (41 with `-nameidf`, 42 with `-gbmfile`).

- [ ] **Step 1: Failing tests.** Create `pipeline/pairfeat7_negatives_test.go`:

```go
package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// pf7Dump: pf6Dump with missing-date and same-date hard negatives at rates md and rd, parsed.
func pf7Dump(t *testing.T, split string, seed int64, xs, md, rd float64, tb *NameIDFTable) (rows [][]string, sc map[string]any, csvB, scB []byte) {
	t.Helper()
	oldM, oldD := MDNamesakeRate, RDNamesakeRate
	defer func() { MDNamesakeRate, RDNamesakeRate = oldM, oldD }()
	MDNamesakeRate, RDNamesakeRate = md, rd
	csvB, scB = pf6Dump(t, split, seed, xs, tb)
	if err := json.Unmarshal(scB, &sc); err != nil {
		t.Fatal(err)
	}
	var err error
	if rows, err = csv.NewReader(bytes.NewReader(csvB)).ReadAll(); err != nil {
		t.Fatal(err)
	}
	return
}

func TestPairfeat7Validate(t *testing.T) {
	for _, flag := range []string{"-mdnamesakes", "-rdnamesakes"} {
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
			err := validateIDFNamesakes(flag, c.cmd, c.rate, c.idf)
			if (err == nil) != c.ok || (err != nil && !strings.Contains(err.Error(), flag)) {
				t.Errorf("validateIDFNamesakes(%s, %q, %v, %q) = %v, want ok=%v", flag, c.cmd, c.rate, c.idf, err, c.ok)
			}
		}
	}
	seeds := map[int64]bool{}
	for _, s := range []int64{namesakeSeed(1019), familySeed(1019), xsSeed(1019), mdSeed(1019), rdSeed(1019)} {
		seeds[s] = true
	}
	if len(seeds) != 5 {
		t.Error("hard-negative kinds share a generator seed")
	}
}

func TestGivenOnly(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Mohammed Salem", "Mohammed Karim Nasser", true},
		{"Fatima Ali", "Fatima Hassan", true},
		{"Mohammed Al Salem", "Mohammed Nasser", true},    // the article is not a token
		{"Mohammed Salem", "Mohammed Salem Hamad", false}, // shares Salem
		{"Mohammed Salem", "Mohammed Hamad Salem", false}, // in any position
		{"Mohammed Selim", "Mohammed Salem", false},       // same skeleton slm
		{"Mohammed Salem", "Ahmed Karim", false},          // different given name
		{"Mohammed Salem", "Mohammed", false},             // single token
		{"Mohammed Salem", "Mohammed S", false},           // initial
		{"Abdullah Rahman", "Abdullah Rahmani", false},    // truncation (and the same skeleton)
		{"Mohammed Salman", "Mohammed Salem", false},      // skeleton slm is a prefix of slmn
		{"Mohammed Abd Karim", "Mohammed Abdullah", false},
	} {
		a, b := newHNName(c.a), newHNName(c.b)
		if givenOnly(a, b) != c.want || givenOnly(b, a) != c.want {
			t.Errorf("givenOnly(%q, %q) = %v / %v, want %v", c.a, c.b, givenOnly(a, b), givenOnly(b, a), c.want)
		}
	}
}

func TestNoSharedSkeleton(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Mohammed Salem", "Karim Nasser", true},
		{"Mohammed Salem", "Ahmed Karim", false}, // mohammed and ahmed share the skeleton md
		{"Mohammed Salem", "Karim Salim", false}, // salem and salim: slm
		{"Fatima Ali", "Noor Hassan", true},
		{"Mohammed Salem", "", false},
	} {
		a, b := newHNName(c.a), newHNName(c.b)
		if noSharedSkeleton(a, strSet(a.skel), b) != c.want || noSharedSkeleton(b, strSet(b.skel), a) != c.want {
			t.Errorf("noSharedSkeleton(%q, %q), want %v", c.a, c.b, c.want)
		}
	}
}

func pf7Pool() []Person {
	return []Person{
		{Qid: "b1", Ar: "محمد سالم", En: "Mohammed Salem", Dob: "1975-06-14", Sex: "M", Spl: "train", Origin: "ae", Idx: 1},
		{Qid: "d1", Ar: "محمد كريم ناصر", En: "Mohammed Karim Nasser", Als: []string{"M Karim"}, Dob: "1980-02-02", Sex: "M", Spl: "train", Origin: "eg", Idx: 2},
		{Qid: "d2", Ar: "محمد سالم حمد", En: "Mohammed Salem Hamad", Dob: "1981-03-03", Sex: "M", Spl: "train", Idx: 3},
		{Qid: "d3", Ar: "محمد كريم", En: "Mohammed Karim", Dob: "1982-04-04", Sex: "F", Spl: "train", Idx: 4},
		{Qid: "d4", Ar: "محمد س", En: "Mohammed S", Dob: "1983-05-05", Sex: "M", Spl: "train", Idx: 5},
		{Qid: "d5", Ar: "محمد سلمان", En: "Mohammed Salman", Dob: "1984-06-06", Sex: "M", Spl: "train", Idx: 6},
		{Qid: "s1", Ar: "عمر فارس", En: "Omar Faris", Dob: "1990-01-02", Sex: "M", Spl: "train", Origin: "jo", Idx: 7},
		{Qid: "s2", Ar: "كريم ناصر", En: "Karim Nasser", Dob: "1991-02-03", Sex: "M", Spl: "train", Origin: "eg", Idx: 8},
		{Qid: "s3", Ar: "نور حسن", En: "Noor Hassan", Dob: "1992-03-04", Sex: "F", Spl: "train", Idx: 9},
	}
}

func TestBuildMDNamesakes(t *testing.T) {
	pool := pf7Pool()
	md, base := buildMDNamesakes(pool[:1], pool, 0.999999, rand.New(rand.NewSource(1)))
	if len(md) != 1 || !reflect.DeepEqual(base, []int{0}) {
		t.Fatalf("md %+v base %v", md, base)
	}
	x := md[0] // d1 is the only donor: d2 shares Salem, d3 is female, d4 an initial, d5 a skeleton prefix
	if x.Qid != "md-b1" || x.En != "Mohammed Karim Nasser" || x.Ar != "محمد كريم ناصر" || x.Dob != "" || x.Sex != "M" ||
		x.Als != nil || x.Origin != "eg" || x.Spl != "train" || x.Idx != 0 {
		t.Fatalf("namesake %+v", x)
	}
	if !luhnOK(x.Eid) || !strings.HasPrefix(x.Eid, "7841980") {
		t.Fatalf("namesake Emirates ID %q", x.Eid)
	}
	for _, ps := range [][]Person{pool[6:7], pool[3:4]} { // no other Omar; no other female Mohammed
		if got, _ := buildMDNamesakes(ps, pool, 0.999999, rand.New(rand.NewSource(1))); len(got) != 0 {
			t.Errorf("%s got namesake %+v", ps[0].Qid, got)
		}
	}
	r, fresh := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	if got, _ := buildMDNamesakes(pool, pool, 0, r); len(got) != 0 || r.Int63() != fresh.Int63() {
		t.Fatal("rate 0 built namesakes or consumed random draws")
	}
	a, _ := buildMDNamesakes(pool, pool, 0.5, rand.New(rand.NewSource(7)))
	b, _ := buildMDNamesakes(pool, pool, 0.5, rand.New(rand.NewSource(7)))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("buildMDNamesakes is not deterministic")
	}
}

func TestBuildRDNamesakes(t *testing.T) {
	pool := pf7Pool()
	vec := map[string][]float32{"Mohammed Salem": {1, 0}, "Omar Faris": {0.9, 0.1}, "Karim Nasser": {0.2, 0.9}}
	emb := func(s string) []float32 {
		if v, ok := vec[s]; ok {
			return v
		}
		return []float32{0, 0}
	}
	rd, base := buildRDNamesakes(pool[:1], pool, 0.999999, rand.New(rand.NewSource(1)), emb)
	if len(rd) != 1 || !reflect.DeepEqual(base, []int{0}) {
		t.Fatalf("rd %+v base %v", rd, base)
	}
	x := rd[0] // eligible: s1, s2 (same sex, no shared skeleton); s1 is nearest
	if x.Qid != "rd-b1" || x.En != "Omar Faris" || x.Dob != "1975-06-14" || x.Sex != "M" || x.Als != nil ||
		x.Origin != "jo" || x.Idx != 0 || !luhnOK(x.Eid) || !strings.HasPrefix(x.Eid, "7841975") {
		t.Fatalf("stranger %+v", x)
	}
	vec["Karim Nasser"] = []float32{0.9, 0.7} // a tie with s1 keeps the first in pool order (s1); above it, s2 wins
	if rd, _ := buildRDNamesakes(pool[:1], pool, 0.999999, rand.New(rand.NewSource(1)), emb); rd[0].En != "Omar Faris" {
		t.Errorf("tie went to %s", rd[0].En)
	}
	vec["Karim Nasser"] = []float32{1, 0.5}
	if rd, _ := buildRDNamesakes(pool[:1], pool, 0.999999, rand.New(rand.NewSource(1)), emb); rd[0].En != "Karim Nasser" {
		t.Errorf("nearest is %s", rd[0].En)
	}
	for _, dob := range []string{"1975-01-01", "1975-07-01", "1975-06-00", "", "1975-02-30"} { // placeholder, missing, invalid
		b := pool[0]
		b.Dob = dob
		if got, _ := buildRDNamesakes([]Person{b}, pool, 0.999999, rand.New(rand.NewSource(1)), emb); len(got) != 0 {
			t.Errorf("DOB %q got stranger %+v", dob, got)
		}
	}
	r, fresh := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	if got, _ := buildRDNamesakes(pool, pool, 0, r, emb); len(got) != 0 || r.Int63() != fresh.Int63() {
		t.Fatal("rate 0 built strangers or consumed random draws")
	}
}

func TestHNRegister(t *testing.T) {
	cliState(t)
	pool := pf7Pool()
	b := pool[0]
	b.Eid = newEID(b.Dob, rand.New(rand.NewSource(5)))
	x := Person{Qid: "md-b1", Ar: pool[1].Ar, En: pool[1].En, Dob: "", Sex: "M", Eid: newEID("1980-02-02", rand.New(rand.NewSource(2)))}
	same := 0
	for seed := int64(1); seed <= 40; seed++ {
		regs := hnRegister(nil, 3, &b, 10, &x, rand.New(rand.NewSource(seed)))
		if len(regs) != 2 || regs[0].pid != 3 || regs[1].pid != 10 || regs[0].tid != 1 || regs[1].tid != 0 {
			t.Fatalf("seed %d: regs %+v", seed, regs)
		}
		if regs[0].dob != "1975-06-14" || regs[1].dob != "" || regs[0].sex != "M" || regs[1].sex != "M" {
			t.Fatalf("seed %d: date or sex noise %+v", seed, regs)
		}
		if strings.Contains(regs[0].name, "Karim") || strings.Contains(regs[1].name, "Salem") {
			t.Fatalf("seed %d: a registration carries the other person's name: %q / %q", seed, regs[0].name, regs[1].name)
		}
		if isAr(regs[0].name) == isAr(regs[1].name) {
			same++
		}
	}
	if same < 10 || same > 30 {
		t.Fatalf("same script in %d of 40 draws, want about half", same)
	}
}

// checkKind: the rows of hard-negative kind key (md or rd) are label-0 rows of no other kind (seen, updated), the sidecar
// counts match, and at least half of them have the kind's feature pattern.
func checkKind(t *testing.T, rows [][]string, sc map[string]any, key string, seen map[int]string, want func(f []string) bool) {
	t.Helper()
	added := int(sc[key+"_namesakes_added"].(float64))
	xr := intList(t, sc[key+"_rows"])
	if sc[key+"_rate"].(float64) != 0.1 || added <= 0 || len(xr) == 0 || int(sc[key+"_base_namesake_rows"].(float64)) != len(xr) {
		t.Fatalf("%s sidecar %v", key, sc)
	}
	hit := 0
	for _, i := range xr {
		if i < 0 || i >= len(rows) || rows[i][0] != "0" {
			t.Fatalf("%s row %d is not a label-0 row", key, i)
		}
		if k, ok := seen[i]; ok {
			t.Fatalf("%s row %d is also a %s row", key, i, k)
		}
		seen[i] = key
		if want(rows[i][2:]) {
			hit++
		}
	}
	t.Logf("%s: %d added, %d rows, %d with the pattern", key, added, len(xr), hit)
	if 2*hit < len(xr) {
		t.Errorf("%s: only %d of %d rows have the hard-negative pattern", key, hit, len(xr))
	}
}

// mdPattern: one DOB missing (f12 0, f30 NaN) and the same sex (f19). rdPattern: equal real dates (f13, f31), the same
// sex and no shared skeleton token (f7, skeleton Jaccard, 0).
func mdPattern(f []string) bool { return f[12] == "0" && f[30] == "NaN" && f[19] == "1" }
func rdPattern(f []string) bool { return f[13] == "1" && f[31] == "1" && f[19] == "1" && f[7] == "0" }

func TestPF7FeatDump(t *testing.T) {
	cliState(t)
	tb, _ := pf7Table(t)
	// train: the missing-date kind next to the given-name, family and cross-script kinds; the same-date kind's donor
	// search embeds the split pool, so it runs on the smaller cal split
	rows, sc, _, _ := pf7Dump(t, "train", 1019, 0.1, 0.1, 0, tb)
	seen := map[int]string{}
	for _, k := range []string{"rows", "family_rows", "xs_rows"} {
		for _, i := range intList(t, sc[k]) {
			seen[i] = k
		}
	}
	checkKind(t, rows, sc, "md", seen, mdPattern)
	for _, k := range []string{"rd_rate", "rd_namesakes_added", "rd_base_namesake_rows", "rd_rows"} {
		if _, ok := sc[k]; ok {
			t.Errorf("rd rate 0, sidecar carries %s", k)
		}
	}
	rows, sc, c1, s1 := pf7Dump(t, "cal", 2011, 0, 0.1, 0.1, tb)
	_, _, c2, s2 := pf7Dump(t, "cal", 2011, 0, 0.1, 0.1, tb)
	if !bytes.Equal(c1, c2) || !bytes.Equal(s1, s2) {
		t.Fatal("featdump with -mdnamesakes and -rdnamesakes is not deterministic")
	}
	for i, r := range rows {
		if len(r) != 2+NFEAT6+5 {
			t.Fatalf("row %d has %d columns, want %d", i, len(r), 2+NFEAT6+5)
		}
	}
	seen = map[int]string{}
	checkKind(t, rows, sc, "md", seen, mdPattern)
	checkKind(t, rows, sc, "rd", seen, rdPattern)
	_, sc, _, _ = pf7Dump(t, "cal", 2011, 0, 0, 0, tb)
	for k := range sc {
		if strings.HasPrefix(k, "md_") || strings.HasPrefix(k, "rd_") {
			t.Errorf("rates 0, sidecar carries %s", k)
		}
	}
}
```

- [ ] **Step 2: RED.** `cd pipeline && go test . -run 'Pairfeat7Validate|GivenOnly|NoSharedSkeleton|BuildMDNamesakes|BuildRDNamesakes|HNRegister|PF7FeatDump' -count=1` fails to compile (undefined `MDNamesakeRate`, `givenOnly`, `buildMDNamesakes`, ...).
- [ ] **Step 3: Implement.** Create `pipeline/pairfeat7.go`:

```go
package main

// Modelset pairfeat7 training-only hard negatives for featdump (with -nameidf: pairfeat7 training dumps; spec
// 2026-11-03-crc-pairfeat7-design.md). pairfeat7 reuses pairfeat6's features f30..f33 and leave-own-out rarity table
// (pairfeat6.go; its own file name_idf_pairfeat7.json) and adds two kinds of synthetic different persons. Each kind
// adds, for a share of the sampled persons, one namesake (no persons-file index: never in the rarity table, never left
// out of it) and two registrations without DOB or sex noise: the base person with their true birth date in a drawn
// script, and the namesake in the same script with probability 1/2, else the opposite (hnRegister).
//   -mdnamesakes  missing-date namesake: same sex as the base person, their given name and no other skeleton token, no
//                 family token or family-token skeleton a prefix of the other name's (givenOnly), and no birth date.
//   -rdnamesakes  same-real-date stranger: same sex, the base person's real non-placeholder birth date, no skeleton token
//                 in common (noSharedSkeleton); the donor is the eligible person whose name embedding is nearest.
import (
	"fmt"
	"math/rand"
	"strings"
)

var MDNamesakeRate float64 // flag -mdnamesakes; training-only (featdump with -nameidf: pairfeat7)

var RDNamesakeRate float64 // flag -rdnamesakes; training-only (featdump with -nameidf: pairfeat7)

func mdSeed(seed int64) int64 { return seed*2654435761 + 181081 }

func rdSeed(seed int64) int64 { return seed*2654435761 + 206233 }

// validateIDFNamesakes: a pairfeat7 hard-negative rate is in [0, 1), featdump only, and only with -nameidf.
func validateIDFNamesakes(flag, cmd string, rate float64, nameIDF string) error {
	if err := validateNamesakeRate(flag, cmd, rate); err != nil {
		return err
	}
	if rate > 0 && nameIDF == "" {
		return fmt.Errorf("%s needs -nameidf (missing-date and same-date hard negatives are pairfeat7 training data)", flag)
	}
	return nil
}

// hnName: the runtime tokens (latinForm, tokensOf) and skeleton of a persons-file English name.
type hnName struct{ tok, skel []string }

func newHNName(en string) hnName {
	t := tokensOf(latinForm(en))
	return hnName{t, skeleton(t)}
}

func strSet(s []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range s {
		m[x] = true
	}
	return m
}

// prefixRelated: some x of a and y of b with x a prefix of y or y a prefix of x (equal strings count).
func prefixRelated(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if strings.HasPrefix(x, y) || strings.HasPrefix(y, x) {
				return true
			}
		}
	}
	return false
}

// givenOnly: the missing-date namesake name rule. Both names have two or more tokens; the first tokens (the given
// names) are equal and their skeleton g is non-empty; the two skeleton-token sets share g and nothing else; no family
// token (a token after the first) of one name is a prefix of a family token of the other; and no family-token skeleton
// of one is a prefix of a family-token skeleton of the other. Initial and truncated family names ("S", "Abd",
// "Rahman" / "Rahmani", "Salman" / "Salem" by skeleton) are therefore never generated.
func givenOnly(a, b hnName) bool {
	if len(a.tok) < 2 || len(b.tok) < 2 || a.tok[0] != b.tok[0] {
		return false
	}
	g := skelTok(a.tok[0])
	if g == "" {
		return false
	}
	sb := strSet(b.skel)
	for _, x := range a.skel {
		if sb[x] && x != g {
			return false
		}
	}
	return !prefixRelated(a.tok[1:], b.tok[1:]) && !prefixRelated(skeleton(a.tok[1:]), skeleton(b.tok[1:]))
}

// noSharedSkeleton: the same-date stranger name rule: both skeletons non-empty and disjoint.
func noSharedSkeleton(a hnName, aset map[string]bool, b hnName) bool {
	if len(a.skel) == 0 || len(b.skel) == 0 {
		return false
	}
	for _, x := range b.skel {
		if aset[x] {
			return false
		}
	}
	return true
}

// buildMDNamesakes: md[i] is the missing-date namesake of ps[base[i]]: prefix "md-", DOB "", the base person's sex, the
// donor's names without aliases, a drawn Emirates ID with the donor's birth year. Donors: pool (the split, file order),
// same sex, givenOnly with the base name, a 10-character DOB; one is drawn uniformly. A base with no donor gets none.
func buildMDNamesakes(ps, pool []Person, rate float64, r *rand.Rand) ([]Person, []int) {
	if rate <= 0 {
		return nil, nil
	}
	names := make([]hnName, len(pool))
	idx := map[string][]int{}
	for i := range pool {
		names[i] = newHNName(pool[i].En)
		if len(names[i].tok) > 1 {
			idx[names[i].tok[0]] = append(idx[names[i].tok[0]], i)
		}
	}
	var md []Person
	var base []int
	for bi := range ps {
		b := &ps[bi]
		if r.Float64() >= rate {
			continue
		}
		bn := newHNName(b.En)
		if len(bn.tok) < 2 {
			continue
		}
		var elig []int
		for _, j := range idx[bn.tok[0]] {
			d := &pool[j]
			if d.Qid != b.Qid && d.Sex == b.Sex && len(d.Dob) == 10 && givenOnly(bn, names[j]) {
				elig = append(elig, j)
			}
		}
		if len(elig) == 0 {
			continue
		}
		d := pool[elig[r.Intn(len(elig))]]
		md = append(md, Person{Qid: "md-" + b.Qid, Ar: d.Ar, En: d.En, Dob: "", Sex: b.Sex, Cc: d.Cc,
			Eid: newEID(d.Dob, r), Spl: b.Spl, Origin: d.Origin, ArSrc: d.ArSrc})
		base = append(base, bi)
	}
	return md, base
}

// buildRDNamesakes: rd[i] is the same-real-date stranger of ps[base[i]]: prefix "rd-", the base person's DOB and sex,
// the donor's names without aliases, a drawn Emirates ID. Bases need a real, non-placeholder DOB (realDob and not
// dobPlaceholder6). Donors: pool, same sex, noSharedSkeleton with the base name; the donor is the eligible person whose
// English-name embedding emb(En) has the largest dot product with the base person's (the first in pool order on a tie),
// so that the pair can reach the linker's candidate list. A base with no donor gets none.
func buildRDNamesakes(ps, pool []Person, rate float64, r *rand.Rand, emb func(string) []float32) ([]Person, []int) {
	if rate <= 0 {
		return nil, nil
	}
	names := make([]hnName, len(pool))
	for i := range pool {
		names[i] = newHNName(pool[i].En)
	}
	vecs := make([][]float32, len(pool)) // embedded on first use
	var rd []Person
	var base []int
	for bi := range ps {
		b := &ps[bi]
		if r.Float64() >= rate {
			continue
		}
		if !realDob(b.Dob) || dobPlaceholder6(b.Dob) {
			continue
		}
		bn := newHNName(b.En)
		bset, bv := strSet(bn.skel), emb(b.En)
		best, bestS := -1, 0.0
		for j := range pool {
			d := &pool[j]
			if d.Qid == b.Qid || d.Sex != b.Sex || !noSharedSkeleton(bn, bset, names[j]) {
				continue
			}
			if vecs[j] == nil {
				vecs[j] = emb(d.En)
			}
			if s := dot(bv, vecs[j]); best < 0 || s > bestS {
				best, bestS = j, s
			}
		}
		if best < 0 {
			continue
		}
		d := pool[best]
		rd = append(rd, Person{Qid: "rd-" + b.Qid, Ar: d.Ar, En: d.En, Dob: b.Dob, Sex: b.Sex, Cc: d.Cc,
			Eid: newEID(b.Dob, r), Spl: b.Spl, Origin: d.Origin, ArSrc: d.ArSrc})
		base = append(base, bi)
	}
	return rd, base
}

// hnRegister: the two registrations of a pairfeat7 hard negative: base person b with their true DOB in a drawn script,
// namesake x with its own DOB in the same script with probability 1/2, else the opposite; true sex, no noise.
func hnRegister(regs []fdReg, bpid int, b *Person, xpid int, x *Person, r *rand.Rand) []fdReg {
	ar := r.Float64() < .5
	xar := ar
	if r.Float64() >= .5 {
		xar = !ar
	}
	regs = append(regs, fdReg{bpid, fdName(b, ar, r), b.Dob, b.Sex, drawEID(b, r), b.Idx})
	return append(regs, fdReg{xpid, fdName(x, xar, r), x.Dob, x.Sex, drawEID(x, r), x.Idx})
}

// hnKind: one pairfeat7 hard-negative kind in a featdump: sidecar key prefix, rate, namesake pid -> base pid, the
// number added, and the rows pairing a namesake with a registration of its base person.
type hnKind struct {
	key   string
	rate  float64
	base  map[int]int
	added int
	rows  []int
}

// pairfeat7Negatives: the md then rd hard negatives, each only when its rate is > 0 and with its own generator
// (mdSeed, rdSeed); their registrations are appended to regs with person indices from first on. With both rates 0 it
// returns regs unchanged and no kind (no draw, no encoder load).
func pairfeat7Negatives(regs []fdReg, ps, pool []Person, seed int64, first int, encPath string) ([]fdReg, []*hnKind) {
	var kinds []*hnKind
	for _, k := range []struct {
		key  string
		rate float64
		seed int64
	}{{"md", MDNamesakeRate, mdSeed(seed)}, {"rd", RDNamesakeRate, rdSeed(seed)}} {
		if k.rate <= 0 {
			continue
		}
		r := rand.New(rand.NewSource(k.seed))
		var xs []Person
		var base []int
		if k.key == "md" {
			xs, base = buildMDNamesakes(ps, pool, k.rate, r)
		} else {
			xs, base = buildRDNamesakes(ps, pool, k.rate, r, LoadEncoder(encPath).Embed)
		}
		hk := &hnKind{key: k.key, rate: k.rate, base: map[int]int{}, added: len(xs), rows: []int{}}
		for i := range xs {
			regs = hnRegister(regs, base[i], &ps[base[i]], first+i, &xs[i], r)
			hk.base[first+i] = base[i]
		}
		first += len(xs)
		kinds = append(kinds, hk)
	}
	return regs, kinds
}
```

  Wire it into `pipeline/encdev.go` and `pipeline/main.go` (diff against BASE):

```diff
diff --git a/pipeline/encdev.go b/pipeline/encdev.go
index 8e288cc..5ee9aa5 100644
--- a/pipeline/encdev.go
+++ b/pipeline/encdev.go
@@ -286,7 +286,7 @@ func parseIDFSeeds(s string) []int64 {
 // distinct providers (gen.go registration model: provider script mix, name/DOB/sex noise); registrations stream in
 // random order and every candidate pair (new, earlier) is written once.
 // CSV columns: label,corr,f0..f29,near_eid,decade_dob,same_surname,placeholder_dob,cross_script[,p from -gbmfile]
-// (37 columns, 38 with p); with -nameidf (pairfeat6) f0..f33: 41 columns, 42 with p. Features and p are %.17g (exact float64 round-trip, NaN as NaN); slice columns are 0/1.
+// (37 columns, 38 with p); with -nameidf (pairfeat6, pairfeat7) f0..f33: 41 columns, 42 with p. Features and p are %.17g (exact float64 round-trip, NaN as NaN); slice columns are 0/1.
 func runFeatDump(wasl, encPath, gbmPath, split, outPath string, nPersons int, seed int64) {
 	var gbm *GBM
 	if gbmPath != "" {
@@ -332,6 +332,7 @@ func runFeatDump(wasl, encPath, gbmPath, split, outPath string, nPersons int, se
 		}
 		xsAdded = len(xs)
 	}
+	regs, hn := pairfeat7Negatives(regs, ps, pool, seed, len(ps)+nsAdded+nfAdded+xsAdded, encPath) // pairfeat7.go
 	r.Shuffle(len(regs), func(i, j int) { regs[i], regs[j] = regs[j], regs[i] })
 	enc := LoadEncoder(encPath)
 	L := NewLinkerCore(nil, 1)
@@ -369,6 +370,11 @@ func runFeatDump(wasl, encPath, gbmPath, split, outPath string, nPersons int, se
 				if namesakePairRow(xsBase, g.pid, pids[c.j]) {
 					xsRows = append(xsRows, rowIdx)
 				}
+				for _, k := range hn {
+					if namesakePairRow(k.base, g.pid, pids[c.j]) {
+						k.rows = append(k.rows, rowIdx)
+					}
+				}
 			}
 			corr := 0
 			if corroborated(rec, L.recs[c.j], c.feat) {
@@ -416,6 +422,10 @@ func runFeatDump(wasl, encPath, gbmPath, split, outPath string, nPersons int, se
 		scm["xs_rate"], scm["xs_namesakes_added"] = XSNamesakeRate, xsAdded
 		scm["xs_base_namesake_rows"], scm["xs_rows"] = len(xsRows), xsRows
 	}
+	for _, k := range hn { // pairfeat7: md_* and rd_* keys, only for a kind whose rate is > 0
+		scm[k.key+"_rate"], scm[k.key+"_namesakes_added"] = k.rate, k.added
+		scm[k.key+"_base_namesake_rows"], scm[k.key+"_rows"] = len(k.rows), k.rows
+	}
 	sc, err := json.Marshal(scm)
 	if err != nil {
 		panic(err)
diff --git a/pipeline/main.go b/pipeline/main.go
index 9ce1800..4ec0dd3 100644
--- a/pipeline/main.go
+++ b/pipeline/main.go
@@ -145,6 +145,8 @@ func main() {
 	linkFloor := fs.Float64("linkfloor", 0.2, "linker+closure: candidate review floor (0,0.2]; merge/closure thresholds unchanged")
 	linkFunnel := fs.String("linkfunnel", "", "linker: write opt-in candidate-funnel NDJSON to this path (diagnostics only)")
 	split := fs.String("split", "eval", "gen: person split eval (held-out test) | dev (development only, not held-out validation) | cal (Learn-then-Test calibration) | all (every split, incl. model training persons: scale/throughput tests only, accuracy not held-out)")
+	fs.Float64Var(&MDNamesakeRate, "mdnamesakes", 0, "featdump only, with -nameidf: share of persons given a synthetic missing-date namesake hard negative (pairfeat7 training only)")
+	fs.Float64Var(&RDNamesakeRate, "rdnamesakes", 0, "featdump only, with -nameidf: share of persons given a synthetic same-real-date stranger hard negative (pairfeat7 training only)")
 	fs.Float64Var(&XSNamesakeRate, "xsnamesakes", 0, "featdump only, with -nameidf: share of persons given a synthetic cross-script namesake hard negative (pairfeat6 training only)")
 	fs.Float64Var(&FamilyNamesakeRate, "familynamesakes", 0, "featdump only: share of persons given a synthetic family-name namesake hard negative (training only)")
 	fs.Float64Var(&NamesakeRate, "namesakes", 0, "featdump only: share of persons given a synthetic namesake hard negative (training only)")
@@ -210,6 +212,15 @@ func main() {
 		fmt.Fprintln(os.Stderr, "configuration error:", err)
 		os.Exit(2)
 	}
+	for _, v := range []struct {
+		flag string
+		rate float64
+	}{{"-mdnamesakes", MDNamesakeRate}, {"-rdnamesakes", RDNamesakeRate}} {
+		if err := validateIDFNamesakes(v.flag, cmd, v.rate, *nameIDFPath); err != nil {
+			fmt.Fprintln(os.Stderr, "configuration error:", err)
+			os.Exit(2)
+		}
+	}
 	if err := validateLinkFloor(*linkFloor); err != nil {
 		fmt.Fprintln(os.Stderr, "configuration error:", err)
 		os.Exit(2)
@@ -272,12 +283,12 @@ func main() {
 		fmt.Fprintln(os.Stderr, "configuration error:", err)
 		os.Exit(2)
 	}
-	XScriptCap = xscriptCapFor(ModelSet) // xscript.go: cross-script review cap (pairfeat5, pairfeat6)
+	XScriptCap = xscriptCapFor(ModelSet) // xscript.go: cross-script review cap (pairfeat5, pairfeat6, pairfeat7)
 	if tb, err := nameIDFFor(cmd, *nameIDFPath, ModelSet, *wasl); err != nil {
 		fmt.Fprintln(os.Stderr, "configuration error:", err)
 		os.Exit(2)
 	} else {
-		NameIDF = tb // pairfeat6.go: non-nil only for -modelset pairfeat6 or featdump -nameidf
+		NameIDF = tb // pairfeat6.go: non-nil only for -modelset pairfeat6/pairfeat7 or featdump -nameidf
 	}
 	DualMerge = *dual
 	switch *policy {
```

- [ ] **Step 4: GREEN.** `cd pipeline && go test . -count=1 && go vet . && gofmt -l pairfeat7.go pairfeat7_negatives_test.go encdev.go` (`TestPF7FeatDump` takes about 45 s and logs about 46 `md` rows on train and 44 `md` / 90 `rd` rows on cal; every earlier golden passes). Then the CLI checks at full size. Each 5,000-person train dump without `-rdnamesakes` takes about 65 s, with it about 250 s; the block takes about 12 minutes.

```bash
ID=~/wasl-ml-data/agents2/hardening/pairfeat7/taskB-identity
mkdir -p $ID/base-src
git archive acefac5 pipeline | tar -x -C $ID/base-src
(cd $ID/base-src/pipeline && go build -buildvcs=false -o $ID/wsi7.base .)
(cd pipeline && go build -buildvcs=false -o $ID/wsi7.head .)
FD=(featdump -wasl "$PWD/data" -encfile "$PWD/models/encoder_weights.json" -noise v1t)
T6=$PWD/models/name_idf_pairfeat6.json
for v in base head; do
  $ID/wsi7.$v "${FD[@]}" -split train -persons 5000 -seed 1019 -namesakes 0.1 -familynamesakes 0.1 \
    -out $ID/p5train.$v.csv 2> $ID/p5train.$v.log
  $ID/wsi7.$v "${FD[@]}" -split train -persons 5000 -seed 1019 -namesakes 0.1 -familynamesakes 0.1 -xsnamesakes 0.1 \
    -nameidf $T6 -out $ID/p6train.$v.csv 2> $ID/p6train.$v.log
  $ID/wsi7.$v "${FD[@]}" -split cal -seed 2011 -gbmfile "$PWD/models/lgb_stream_pairfeat5.json" \
    -out $ID/p5cal.$v.csv 2> $ID/p5cal.$v.log
  $ID/wsi7.$v "${FD[@]}" -split cal -seed 2011 -gbmfile "$PWD/models/lgb_stream_pairfeat6.json" -nameidf $T6 \
    -out $ID/p6cal.$v.csv 2> $ID/p6cal.$v.log
done
for f in p5train.csv p5train.csv.namesakes.json p6train.csv p6train.csv.namesakes.json p5cal.csv p6cal.csv; do
  b=$(shasum -a 256 < $ID/${f%%.*}.base.${f#*.} | cut -c1-64); h=$(shasum -a 256 < $ID/${f%%.*}.head.${f#*.} | cut -c1-64)
  [ "$b" = "$h" ] && echo "same $f" || echo "DIFFERENT $f"
done
$ID/wsi7.head "${FD[@]}" -split train -persons 5000 -seed 1019 -namesakes 0.1 -familynamesakes 0.1 -xsnamesakes 0.1 \
  -mdnamesakes 0.1 -rdnamesakes 0.1 -nameidf $T6 -out $ID/p7train.csv 2> $ID/p7train.log
python3 -I - $ID/p7train.csv <<'PY'
import csv, json, sys
p = sys.argv[1]
widths = {len(r) for r in csv.reader(open(p, newline=""))}
sc = json.load(open(p + ".namesakes.json"))
print(json.dumps({"widths": sorted(widths), **{k: sc[k] for k in ("xs_base_namesake_rows", "md_namesakes_added",
      "md_base_namesake_rows", "rd_namesakes_added", "rd_base_namesake_rows")}}))
PY
$ID/wsi7.head "${FD[@]}" -split train -persons 50 -seed 1 -mdnamesakes 0.1 -out $ID/x.csv; echo "exit $?"   # needs -nameidf
$ID/wsi7.head "${FD[@]}" -split train -persons 50 -seed 1 -rdnamesakes 1 -nameidf $T6 -out $ID/x.csv; echo "exit $?"
```

  Expected: six `same` lines; widths `[41]`; `md_base_namesake_rows` and `rd_base_namesake_rows` each above 100 (planning measured about 239 and 602 at seed 1019; five seeds must reach the 500 floors; report the values); both refusals `exit 2`. Stop and report if any line says `DIFFERENT`.
- [ ] **Step 5: Commit.**

```bash
git add pipeline/pairfeat7.go pipeline/pairfeat7_negatives_test.go
git commit -m "pairfeat7: missing-date and same-real-date hard negatives (featdump -mdnamesakes, -rdnamesakes)" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" \
  -- pipeline/pairfeat7.go pipeline/pairfeat7_negatives_test.go pipeline/encdev.go pipeline/main.go
git show --stat HEAD    # exactly the four files
```

### Task C: the pairfeat7 fit protocol (Python)

**Files:** Modify `encoder/fit_pairfeat1.py`, `encoder/test_fit_pairfeat1.py`, `.gitignore`.

**Interfaces:**
- Consumes (Task B): `featdump -mdnamesakes RATE -rdnamesakes RATE` (with `-nameidf`); sidecar keys `md_namesakes_added`, `md_base_namesake_rows`, `md_rows` and the `rd_` equivalents.
- Produces (`encoder/fit_pairfeat1.py`):
  - `PROTOCOLS["pairfeat7"] = dict(PROTOCOLS["pairfeat6"], md_namesakes=0.10, rd_namesakes=0.10, reference="pairfeat5")` (written out literally)
  - `HARD_NEGATIVES7 = (("md", "missing-date"), ("rd", "same-date"))`, `PAIRFEAT7_CODE_FILES = ["pipeline/pairfeat7.go"]`, `USAGE`
  - `_namesake_args` appends `-mdnamesakes 0.1 -rdnamesakes 0.1` for `pairfeat7` (train and namesake-cal dumps); `chain_files`, `nameidf_argv`, `cal_argv`, `parity_argv`, `fit_argv` work for `pairfeat7` unchanged (34 features, its own file names)
  - `check_namesake_floor(sidecars, "pairfeat7")` returns `(given, family, cross_script, missing_date, same_date)`
  - `hard_negative_slice(csv_path, sidecar_path, key, width=38) -> dict`
  - `split_reference(argv) -> (argv, ref_dir or None)`; `read_reference(ref_dir, modelset, head) -> dict or None`
  - `base_manifest` adds `pipeline/pairfeat7.go` to the code hashes for `pairfeat7`; `training_manifest` adds `md_namesakes`, `rd_namesakes`, per-seed `md_/rd_namesakes_added` and `md_/rd_base_namesake_rows`, and `md_/rd_namesake_base_rows_total`
  - `artifact_manifest(..., md_namesake_slice=None, rd_namesake_slice=None, plain_cal_diff_at_merge_0_7_reference=None)`
  - CLI: `fit_pairfeat1.py NEW_OUTPUT_DIR --modelset pairfeat7 --reference REFERENCE_FIT_DIR` (`--reference` required for `pairfeat7`, refused for every other modelset; exit message `--reference: ...`)

- [ ] **Step 0: Re-derive `FIT_GOLDEN` at BASE** (before editing; HEAD has Tasks A and B only, so the fit driver is still `acefac5`'s):

```bash
git diff --quiet acefac5 HEAD -- encoder && echo "encoder at BASE"    # must print it
cd encoder && python3 -I - <<'PY'
import hashlib, json, os, sys, tempfile
sys.path.insert(0, os.getcwd())
import test_fit_pairfeat1 as T
fp, make_repo, FILES, PF6_FILES = T.fp, T.make_repo, T.FILES, T.PF6_FILES


def fit_tables(d):
    """pairfeat1-pairfeat6 fit-driver tables: protocols, chain names, widths, every dump argv, the fit argv (interpreter
    path replaced), the floor result, and the base and training manifests over fixed fake files (git_head dropped).
    d is an empty scratch directory; make_repo(d, FILES + PF6_FILES) is called here."""
    make_repo(d, FILES + PF6_FILES)
    W, D, E, I = "/w/bin/wsi7", "/w/data", "/o/enc.json", "/o/chain/name_idf.json"
    enc, idf = os.path.join(d, "enc.json"), os.path.join(d, "idf.json")
    for p, b in ((enc, b"enc"), (idf, b"idf")):
        with open(p, "wb") as f:
            f.write(b)
    sc = {"namesakes_added": 9, "base_namesake_rows": 300, "family_namesakes_added": 8, "family_base_namesake_rows": 300,
          "xs_namesakes_added": 7, "xs_base_namesake_rows": 300}
    out = {}
    for k in ("pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4", "pairfeat5", "pairfeat6"):
        u = fp.uses_name_idf(k)
        i = I if u else None
        seeds = fp.PROTOCOLS[k]["train_seeds"]
        base = fp.base_manifest(d, enc, k, name_idf=idf if u else None)
        base.pop("git_head")
        tm = fp.training_manifest(d, enc, k, {s: (enc, sc) for s in seeds}, dict(base, git_head="h"))
        floor = fp.check_namesake_floor([sc] * len(seeds), k)
        out[k] = {"protocol": fp.PROTOCOLS[k], "chain": fp.chain_files(k), "nfeat": fp.nfeat(k),
                  "widths": [fp.csv_width(k, False), fp.csv_width(k, True)], "args": fp.parse_args(["x", "o", "--modelset", k]),
                  "csvs": [fp.train_csv_names(k), fp.cal_csv_name(k), fp.parity_csv_name(k), fp.namesake_cal_csv_name(k)],
                  "train": [fp.train_argv(W, D, E, k, s, "/o/t%d.csv" % s, idf=i) for s in seeds],
                  "cal": fp.cal_argv(W, D, E, k, "/o/c.csv", idf=i),
                  "parity": fp.parity_argv(W, D, E, k, "/c/g.json", "/o/p.csv", idf=i),
                  "ns_cal": fp.namesake_cal_argv(W, D, E, k, "/c/g.json", "/o/n.csv", idf=i),
                  "fit": ["PYTHON"] + fp.fit_argv("/r", "/o", "/o/chain", k)[1:],
                  "nameidf": fp.nameidf_argv(W, D, k, I) if u else None,
                  "floor": list(floor) if isinstance(floor, tuple) else floor, "base": base, "training": tm}
    return out


with tempfile.TemporaryDirectory() as d:
    print(hashlib.sha256(json.dumps(fit_tables(d), sort_keys=True).encode()).hexdigest())
PY
```

  Expected: `41a96a2aac94139d02a9f7ec28751c3e382507080264bdb989abbcd88c726964`. Stop if it differs.

- [ ] **Step 1: Failing tests.** In `encoder/test_fit_pairfeat1.py`:
  - `Protocols.test_constants`: the expected `sorted(fp.PROTOCOLS)` list gains `"pairfeat7"` (the only edit to an existing test):

```python
        self.assertEqual(sorted(fp.PROTOCOLS), ["pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4", "pairfeat5", "pairfeat6",
                                                "pairfeat7"])
```

  - Insert before the final `if __name__ == "__main__":` block:

```python
# ================================================================ pairfeat7: pairfeat6 plus missing-date and same-date hard negatives

PF7_FILES = ["pipeline/pairfeat7.go"]
# fit_tables() digest computed with the fit driver at acefac5 (before pairfeat7)
FIT_GOLDEN = "41a96a2aac94139d02a9f7ec28751c3e382507080264bdb989abbcd88c726964"


def fit_tables(d):
    """pairfeat1-pairfeat6 fit-driver tables: protocols, chain names, widths, every dump argv, the fit argv (interpreter
    path replaced), the floor result, and the base and training manifests over fixed fake files (git_head dropped).
    d is an empty scratch directory; make_repo(d, FILES + PF6_FILES) is called here."""
    make_repo(d, FILES + PF6_FILES)
    W, D, E, I = "/w/bin/wsi7", "/w/data", "/o/enc.json", "/o/chain/name_idf.json"
    enc, idf = os.path.join(d, "enc.json"), os.path.join(d, "idf.json")
    for p, b in ((enc, b"enc"), (idf, b"idf")):
        with open(p, "wb") as f:
            f.write(b)
    sc = {"namesakes_added": 9, "base_namesake_rows": 300, "family_namesakes_added": 8, "family_base_namesake_rows": 300,
          "xs_namesakes_added": 7, "xs_base_namesake_rows": 300}
    out = {}
    for k in ("pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4", "pairfeat5", "pairfeat6"):
        u = fp.uses_name_idf(k)
        i = I if u else None
        seeds = fp.PROTOCOLS[k]["train_seeds"]
        base = fp.base_manifest(d, enc, k, name_idf=idf if u else None)
        base.pop("git_head")
        tm = fp.training_manifest(d, enc, k, {s: (enc, sc) for s in seeds}, dict(base, git_head="h"))
        floor = fp.check_namesake_floor([sc] * len(seeds), k)
        out[k] = {"protocol": fp.PROTOCOLS[k], "chain": fp.chain_files(k), "nfeat": fp.nfeat(k),
                  "widths": [fp.csv_width(k, False), fp.csv_width(k, True)], "args": fp.parse_args(["x", "o", "--modelset", k]),
                  "csvs": [fp.train_csv_names(k), fp.cal_csv_name(k), fp.parity_csv_name(k), fp.namesake_cal_csv_name(k)],
                  "train": [fp.train_argv(W, D, E, k, s, "/o/t%d.csv" % s, idf=i) for s in seeds],
                  "cal": fp.cal_argv(W, D, E, k, "/o/c.csv", idf=i),
                  "parity": fp.parity_argv(W, D, E, k, "/c/g.json", "/o/p.csv", idf=i),
                  "ns_cal": fp.namesake_cal_argv(W, D, E, k, "/c/g.json", "/o/n.csv", idf=i),
                  "fit": ["PYTHON"] + fp.fit_argv("/r", "/o", "/o/chain", k)[1:],
                  "nameidf": fp.nameidf_argv(W, D, k, I) if u else None,
                  "floor": list(floor) if isinstance(floor, tuple) else floor, "base": base, "training": tm}
    return out


class FitByteIdentity(unittest.TestCase):
    def test_pairfeat1_to_pairfeat6_unchanged(self):
        with tempfile.TemporaryDirectory() as d:
            got = fit_tables(d)
        self.assertEqual(hashlib.sha256(json.dumps(got, sort_keys=True).encode()).hexdigest(), FIT_GOLDEN)


class Pairfeat7(unittest.TestCase):
    W, D, E, I = "/w/bin/wsi7", "/w/data", "/o/enc.json", "/o/chain/name_idf_pairfeat7.json"

    def test_protocol_and_widths(self):
        self.assertEqual(fp.PROTOCOLS["pairfeat7"], dict(fp.PROTOCOLS["pairfeat6"], md_namesakes=0.1, rd_namesakes=0.1,
                                                         reference="pairfeat5"))
        for k in EARLIER + ("pairfeat6",):
            for key in ("md_namesakes", "rd_namesakes", "reference"):
                self.assertNotIn(key, fp.PROTOCOLS[k])
        self.assertEqual((fp.nfeat("pairfeat7"), fp.csv_width("pairfeat7", False), fp.csv_width("pairfeat7", True)), (34, 41, 42))
        self.assertTrue(fp.uses_name_idf("pairfeat7"))
        self.assertEqual(fp.HARD_NEGATIVES7, (("md", "missing-date"), ("rd", "same-date")))

    def test_cli_chain_output_gitignore(self):
        self.assertEqual(fp.parse_args(["x", "o", "--modelset", "pairfeat7"]), ("o", "pairfeat7"))
        self.assertEqual(fp.chain_files("pairfeat7"), ["encoder_pairfeat7.json", "lgb_stream_pairfeat7.json",
                                                       "iso_pairfeat7.json", "iso_v7_pairfeat7.json", "name_idf_pairfeat7.json"])
        with tempfile.TemporaryDirectory() as home:
            for ms in ("pairfeat7", "pairfeat5"):
                os.makedirs(os.path.join(home, "wasl-ml-data", "agents2", "hardening", ms))
            p = os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat7", "run-1")
            self.assertEqual(fp.validate_output(p, home, "pairfeat7"), p)
            r = os.path.join(home, "wasl-ml-data", "agents2", "hardening", "pairfeat5", "pairfeat7-ref-run-1")
            self.assertEqual(fp.validate_output(r, home, "pairfeat5"), r)
            with self.assertRaises(ValueError):
                fp.validate_output(p, home, "pairfeat6")
        root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        with open(os.path.join(root, ".gitignore")) as f:
            self.assertIn("models/*_pairfeat7.json", f.read().splitlines())

    def test_dump_argv(self):
        W, D, E, I = self.W, self.D, self.E, self.I
        hn = ["-mdnamesakes", "0.1", "-rdnamesakes", "0.1"]
        self.assertEqual(fp.nameidf_argv(W, D, "pairfeat7", I), fp.nameidf_argv(W, D, "pairfeat6", I))
        t6 = fp.train_argv(W, D, E, "pairfeat6", 1019, "/o/t.csv", idf=I)
        self.assertEqual(fp.train_argv(W, D, E, "pairfeat7", 1019, "/o/t.csv", idf=I), t6[:-4] + hn + t6[-4:])
        self.assertEqual(fp.cal_argv(W, D, E, "pairfeat7", "/o/c.csv", idf=I), fp.cal_argv(W, D, E, "pairfeat6", "/o/c.csv", idf=I))
        self.assertEqual(fp.parity_argv(W, D, E, "pairfeat7", "/c/g.json", "/o/p.csv", idf=I),
                         fp.parity_argv(W, D, E, "pairfeat6", "/c/g.json", "/o/p.csv", idf=I))
        n6 = fp.namesake_cal_argv(W, D, E, "pairfeat6", "/c/g.json", "/o/n.csv", idf=I)
        self.assertEqual(fp.namesake_cal_argv(W, D, E, "pairfeat7", "/c/g.json", "/o/n.csv", idf=I), n6[:-6] + hn + n6[-6:])
        with self.assertRaises(ValueError):
            fp.train_argv(W, D, E, "pairfeat7", 1019, "/o/t.csv")             # pairfeat7 needs the table
        a7, a6 = fp.fit_argv("/r", "/o", "/o/chain", "pairfeat7"), fp.fit_argv("/r", "/o", "/o/chain", "pairfeat6")
        self.assertEqual(a7, [x.replace("pairfeat6", "pairfeat7") for x in a6])

    def test_floor_five_kinds(self):
        def sc(g=250, f=250, x=250, m=250, r=250):
            return {"base_namesake_rows": g, "family_base_namesake_rows": f, "xs_base_namesake_rows": x,
                    "md_base_namesake_rows": m, "rd_base_namesake_rows": r}
        self.assertEqual(fp.check_namesake_floor([sc(), sc(r=260)], "pairfeat7"), (500, 500, 500, 500, 510))
        for kw, label in ((dict(m=249), "missing-date"), (dict(r=249), "same-date"), (dict(x=249), "cross-script")):
            with self.assertRaises(fp.NamesakeFloorError) as cm:
                fp.check_namesake_floor([sc(), sc(**kw)], "pairfeat7")
            self.assertEqual(str(cm.exception), f"namesake floor not met ({label}): 499 < 500")
        self.assertEqual(fp.check_namesake_floor([sc(r=0), sc(m=0)], "pairfeat6"), (500, 500, 500))

    def test_slices(self):
        with tempfile.TemporaryDirectory() as d:
            c = os.path.join(d, "ns.csv")
            write_rows(c, [go_row6(0.9), go_row6(0.005), go_row6(0.01), go_row6(0.75)])
            sc = os.path.join(d, "ns.csv.namesakes.json")
            fp.write_json(sc, {"md_rows": [0, 1], "rd_rows": [2, 3], "xs_rows": [], "rows": [], "family_rows": []})
            self.assertEqual(fp.hard_negative_slice(c, sc, "md", width=42),
                             {"rows": 2, "different_person": 2, "different_at_review_0_01": 1, "different_at_merge_0_7": 1})
            self.assertEqual(fp.hard_negative_slice(c, sc, "rd", width=42),
                             {"rows": 2, "different_person": 2, "different_at_review_0_01": 2, "different_at_merge_0_7": 1})
            with self.assertRaises(ValueError):
                fp.hard_negative_slice(c, sc, "md")                             # default width 38

    def test_base_and_training_manifest(self):
        with tempfile.TemporaryDirectory() as d:
            make_repo(d, FILES + PF6_FILES + PF7_FILES)
            enc, idf = os.path.join(d, "enc.json"), os.path.join(d, "name_idf_pairfeat7.json")
            for p, b in ((enc, b"enc"), (idf, b"idf")):
                with open(p, "wb") as f:
                    f.write(b)
            base = fp.base_manifest(d, enc, "pairfeat7", name_idf=idf)
            self.assertEqual(base["code_sha256"], {n: sh(n.encode()) for n in FILES + PF6_FILES + PF7_FILES})
            self.assertEqual((base["nfeat"], base["modelset"], base["name_idf_sha256"]), (34, "pairfeat7", sh(b"idf")))
            self.assertEqual(set(fp.base_manifest(d, enc, "pairfeat6", name_idf=idf)["code_sha256"]), set(FILES + PF6_FILES))
            sides = {}
            for s in fp.PROTOCOLS["pairfeat7"]["train_seeds"]:
                p = os.path.join(d, f"s{s}.json")
                with open(p, "w") as f:
                    f.write(str(s))
                sides[s] = (p, {"namesakes_added": 90, "base_namesake_rows": 110, "family_namesakes_added": 40,
                                "family_base_namesake_rows": 60, "xs_namesakes_added": 70, "xs_base_namesake_rows": 150,
                                "md_namesakes_added": 80, "md_base_namesake_rows": 200, "rd_namesakes_added": 75,
                                "rd_base_namesake_rows": 120})
            m = fp.training_manifest(d, enc, "pairfeat7", sides, base)
            self.assertEqual((m["md_namesakes"], m["md_namesake_base_rows_total"], m["rd_namesakes"], m["rd_namesake_base_rows_total"]),
                             (0.1, 1000, 0.1, 600))
            e = m["namesake_seeds"]["1021"]
            self.assertEqual((e["md_namesakes_added"], e["md_base_namesake_rows"], e["rd_namesakes_added"], e["rd_base_namesake_rows"]),
                             (80, 200, 75, 120))
            m6 = fp.training_manifest(d, enc, "pairfeat6", sides, fp.base_manifest(d, enc, "pairfeat6", name_idf=idf))
            self.assertFalse([k for k in m6 if k.startswith(("md_", "rd_"))])
            self.assertNotIn("md_namesakes_added", m6["namesake_seeds"]["1021"])

    def test_reference(self):
        head = "a" * 40
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "artifact-manifest.json")
            good = {"git_head": head, "baseline_chain_sha256": {"x": "1"}, "pairfeat5_chain_sha256": {"y": "2"},
                    "plain_cal_diff_at_merge_0_7": 21}
            fp.write_json(p, good)
            self.assertEqual(fp.read_reference(d, "pairfeat7", head),
                             {"modelset": "pairfeat5", "plain_cal_diff_at_merge_0_7": 21, "artifact_manifest_sha256": fp.sha256(p)})
            for bad in (dict(good, git_head="b" * 40), dict(good, plain_cal_diff_at_merge_0_7=None),
                        dict(good, plain_cal_diff_at_merge_0_7=True), {k: v for k, v in good.items() if k != "pairfeat5_chain_sha256"},
                        dict(good, pairfeat6_chain_sha256={"z": "3"})):
                fp.write_json(p, bad)
                with self.assertRaises(ValueError, msg=str(bad)):
                    fp.read_reference(d, "pairfeat7", head)
            with self.assertRaises(ValueError):
                fp.read_reference(None, "pairfeat7", head)                        # required for pairfeat7
            with self.assertRaises(OSError):
                fp.read_reference(os.path.join(d, "missing"), "pairfeat7", head)
            for k in EARLIER + ("pairfeat6",):
                self.assertIsNone(fp.read_reference(None, k, head))
                with self.assertRaises(ValueError):
                    fp.read_reference(d, k, head)                                 # no other modelset takes one
        self.assertEqual(fp.split_reference(["x", "o", "--modelset", "pairfeat7", "--reference", "/r"]),
                         (["x", "o", "--modelset", "pairfeat7"], "/r"))
        self.assertEqual(fp.split_reference(["x", "o"]), (["x", "o"], None))
        for bad in (["x", "o", "--reference"], ["x", "o", "--reference", "/a", "--reference", "/b"]):
            with self.assertRaises(SystemExit):
                fp.split_reference(bad)

    def test_artifact_manifest_and_models(self):
        with tempfile.TemporaryDirectory() as d:
            models, chain, out = (os.path.join(d, n) for n in ("models", "chain", "out"))
            for p in (models, chain, out):
                os.makedirs(p)
            for n in fp.BASELINE_FILES:
                with open(os.path.join(models, n), "wb") as f:
                    f.write(b"b")
            for n in fp.chain_files("pairfeat7"):
                with open(os.path.join(chain, n), "wb") as f:
                    f.write(b"c" + n.encode())
            with open(os.path.join(out, "training-manifest.json"), "wb") as f:
                f.write(b"{}")
            sl = {"rows": 3, "different_person": 3, "different_at_review_0_01": 2, "different_at_merge_0_7": 1}
            ref = {"modelset": "pairfeat5", "plain_cal_diff_at_merge_0_7": 21, "artifact_manifest_sha256": "cd" * 32}
            m = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat7", max_abs_leaf=3.1, lambda_l2=1.0,
                                     xs_namesake_slice=sl, md_namesake_slice=sl, rd_namesake_slice=sl, name_idf_sha256="ab" * 32,
                                     plain_cal_diff_at_merge_0_7_reference=ref)
            self.assertEqual(m["pairfeat7_chain_sha256"], {n: sh(b"c" + n.encode()) for n in fp.chain_files("pairfeat7")})
            self.assertEqual((m["md_namesake_slice"], m["rd_namesake_slice"], m["plain_cal_diff_at_merge_0_7_reference"]), (sl, sl, ref))
            m6 = fp.artifact_manifest("a" * 40, out, [], models, chain, 3, 0.0, "pairfeat7", max_abs_leaf=3.1, lambda_l2=1.0)
            for k in ("md_namesake_slice", "rd_namesake_slice", "plain_cal_diff_at_merge_0_7_reference"):
                self.assertNotIn(k, m6)
            fp.check_models_free(models, "pairfeat7")
            fp.copy_chain_to_models(chain, models, "pairfeat7")
            with self.assertRaises(ValueError):
                fp.check_models_free(models, "pairfeat7")
```

- [ ] **Step 2: RED.** `python3 -I encoder/test_fit_pairfeat1.py` fails (`FitByteIdentity` passes; the `Pairfeat7` tests fail with `KeyError: 'pairfeat7'` and missing attributes).
- [ ] **Step 3: Implement** (diff against BASE):

```diff
diff --git a/encoder/fit_pairfeat1.py b/encoder/fit_pairfeat1.py
index 5584944..021033d 100644
--- a/encoder/fit_pairfeat1.py
+++ b/encoder/fit_pairfeat1.py
@@ -1,7 +1,9 @@
 """Frozen driver: dump features, fit the pair scorer chain, hash everything.
 
-Usage: python3 -I encoder/fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5|pairfeat6]
-NEW_OUTPUT_DIR must be a new directory under ~/wasl-ml-data/agents2/hardening/<modelset>/.
+Usage: python3 -I encoder/fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|...|pairfeat7] [--reference REFERENCE_FIT_DIR]
+NEW_OUTPUT_DIR must be a new directory under ~/wasl-ml-data/agents2/hardening/<modelset>/. --reference (pairfeat7 only, and
+required there) is the separately fitted reference (pairfeat5) at the same git HEAD; its plain-cal different-at-merge-0.7
+count is reported beside the candidate's (not gated).
 Orchestration is stdlib only (numpy is imported solely to read cal_p_<modelset>.npy).
 """
 import csv
@@ -28,7 +30,13 @@ PROTOCOLS = {
                   "namesakes": 0.10, "family_namesakes": 0.10, "lambda_l2": 1.0},
     "pairfeat6": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
                   "namesakes": 0.10, "family_namesakes": 0.10, "lambda_l2": 1.0, "xs_namesakes": 0.10, "nfeat": 34},
+    # pairfeat7 (spec 2026-11-03-crc-pairfeat7-design.md): pairfeat6 plus missing-date and same-real-date hard negatives
+    "pairfeat7": {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t",
+                  "namesakes": 0.10, "family_namesakes": 0.10, "lambda_l2": 1.0, "xs_namesakes": 0.10, "nfeat": 34,
+                  "md_namesakes": 0.10, "rd_namesakes": 0.10, "reference": "pairfeat5"},
 }
+# pairfeat7 hard-negative kinds: featdump flag and sidecar prefix, and the floor message label
+HARD_NEGATIVES7 = (("md", "missing-date"), ("rd", "same-date"))
 LEAF_LIMIT = 5.0
 NAMESAKE_FLOOR = 500
 DEFAULT_MODELSET = "pairfeat1"
@@ -40,6 +48,9 @@ TOLERANCE = 1e-9
 CODE_FILES = ["pipeline/text.go", "pipeline/encdev.go", "pipeline/gen.go",
               "encoder/fit_scorer.py", "encoder/fit_pairfeat1.py"]
 PAIRFEAT6_CODE_FILES = ["pipeline/pairfeat6.go", "pipeline/namesake.go"]
+PAIRFEAT7_CODE_FILES = ["pipeline/pairfeat7.go"]
+USAGE = ("usage: fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5|pairfeat6|pairfeat7]"
+         " [--reference REFERENCE_FIT_DIR]")
 
 
 def nfeat(modelset):
@@ -47,7 +58,7 @@ def nfeat(modelset):
 
 
 def uses_name_idf(modelset):
-    """pairfeat6: 34 features, the name-rarity table built first and part of the chain."""
+    """pairfeat6, pairfeat7: 34 features, the name-rarity table built first and part of the chain."""
     return nfeat(modelset) == 34
 
 
@@ -139,6 +150,10 @@ def _namesake_args(modelset):
     xs = PROTOCOLS[modelset].get("xs_namesakes")
     if xs:
         args += ["-xsnamesakes", str(xs)]
+    for key, _ in HARD_NEGATIVES7:
+        rate = PROTOCOLS[modelset].get(key + "_namesakes")
+        if rate:
+            args += [f"-{key}namesakes", str(rate)]
     return args
 
 
@@ -241,7 +256,14 @@ def check_namesake_floor(sidecars, modelset=None):
             xtotal = _sidecar_rows_total(sidecars, "xs_base_namesake_rows")
             if xtotal < NAMESAKE_FLOOR:
                 raise NamesakeFloorError(f"namesake floor not met (cross-script): {xtotal} < {NAMESAKE_FLOOR}")
-            return total, ftotal, xtotal
+            more = []
+            for key, label in HARD_NEGATIVES7:
+                if PROTOCOLS[modelset].get(key + "_namesakes"):
+                    t = _sidecar_rows_total(sidecars, key + "_base_namesake_rows")
+                    if t < NAMESAKE_FLOOR:
+                        raise NamesakeFloorError(f"namesake floor not met ({label}): {t} < {NAMESAKE_FLOOR}")
+                    more.append(t)
+            return (total, ftotal, xtotal) + tuple(more)
         return total, ftotal
     if total < NAMESAKE_FLOOR:
         raise NamesakeFloorError(f"namesake floor not met: {total} < {NAMESAKE_FLOOR}")
@@ -290,12 +312,56 @@ def xs_namesake_slice(csv_path, sidecar_path, width=38):
     return _slice(csv_path, sidecar_path, "xs_rows", width)
 
 
+def hard_negative_slice(csv_path, sidecar_path, key, width=38):
+    """pairfeat7: the md (missing-date) or rd (same-real-date) rows of a namesake-cal dump."""
+    return _slice(csv_path, sidecar_path, key + "_rows", width)
+
+
 def plain_cal_diff_at_merge(parity_csv, width=38):
     return sum(1 for label, p in _read_gbm_rows(parity_csv, width) if label == 0 and p >= 0.7)
 
 
+def split_reference(argv):
+    """argv without "--reference DIR", and DIR (None when absent)."""
+    args = list(argv)
+    if "--reference" not in args:
+        return args, None
+    i = args.index("--reference")
+    if args.count("--reference") != 1 or i + 1 >= len(args):
+        sys.exit(USAGE)
+    ref = args[i + 1]
+    del args[i:i + 2]
+    return args, ref
+
+
+def read_reference(ref_dir, modelset, head):
+    """A modelset whose protocol names a "reference" (pairfeat7: pairfeat5) needs ref_dir, that reference's fit
+    directory: its artifact-manifest.json must carry exactly the reference chain, at git HEAD head, with a
+    plain_cal_diff_at_merge_0_7 count. Returns {"modelset", "plain_cal_diff_at_merge_0_7", "artifact_manifest_sha256"}
+    (reported beside the candidate's count, not gated). Every other modelset refuses a ref_dir and returns None."""
+    ref = PROTOCOLS[modelset].get("reference")
+    if ref is None:
+        if ref_dir is not None:
+            raise ValueError(f"{modelset} takes no --reference")
+        return None
+    if ref_dir is None:
+        raise ValueError(f"{modelset} needs --reference (the {ref} reference fit directory)")
+    path = os.path.join(ref_dir, "artifact-manifest.json")
+    with open(path) as f:
+        m = json.load(f)
+    chains = [k for k in m if k.endswith("_chain_sha256") and k != "baseline_chain_sha256"]
+    if chains != [f"{ref}_chain_sha256"]:
+        raise ValueError(f"the reference fit must carry exactly the {ref} chain (got {chains})")
+    if m.get("git_head") != head:
+        raise ValueError(f"the reference fit is at git HEAD {m.get('git_head')}, not {head}")
+    v = m.get("plain_cal_diff_at_merge_0_7")
+    if isinstance(v, bool) or not isinstance(v, int) or v < 0:
+        raise ValueError("the reference fit has no plain_cal_diff_at_merge_0_7 count")
+    return {"modelset": ref, "plain_cal_diff_at_merge_0_7": v, "artifact_manifest_sha256": sha256(path)}
+
+
 def parse_args(argv):
-    usage = "usage: fit_pairfeat1.py NEW_OUTPUT_DIR [--modelset pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5|pairfeat6]"
+    usage = USAGE
     args = list(argv[1:])
     modelset = DEFAULT_MODELSET
     if "--modelset" in args:
@@ -337,7 +403,8 @@ def check_go_parity(go_scores, python_scores, tolerance=1e-9):
 
 def base_manifest(root, encoder, modelset=DEFAULT_MODELSET, name_idf=None):
     proto = PROTOCOLS[modelset]
-    files = CODE_FILES + (PAIRFEAT6_CODE_FILES if uses_name_idf(modelset) else [])
+    files = CODE_FILES + (PAIRFEAT6_CODE_FILES if uses_name_idf(modelset) else []) + \
+        (PAIRFEAT7_CODE_FILES if proto.get("md_namesakes") else [])
     m = {"git_head": subprocess.check_output(["git", "-C", root, "rev-parse", "HEAD"], text=True).strip(),
          "nfeat": nfeat(modelset), "train": {"split": "train", "seeds": list(proto["train_seeds"]), "persons": PERSONS},
          "cal": {"split": "cal", "seeds": [proto["cal_seed"]], "persons": 0},
@@ -384,6 +451,16 @@ def training_manifest(root, encoder, modelset, sidecars, base=None):
                 e["xs_namesakes_added"] = int(sc["xs_namesakes_added"])
                 e["xs_base_namesake_rows"] = int(sc["xs_base_namesake_rows"])
             m["xs_namesake_base_rows_total"] = sum(int(sc["xs_base_namesake_rows"]) for _, sc in sidecars.values())
+        for key, _ in HARD_NEGATIVES7:
+            rate7 = PROTOCOLS[modelset].get(key + "_namesakes")
+            if not rate7:
+                continue
+            m[key + "_namesakes"] = rate7
+            for seed, (_, sc) in sorted(sidecars.items()):
+                e = m["namesake_seeds"][str(seed)]
+                e[key + "_namesakes_added"] = int(sc[key + "_namesakes_added"])
+                e[key + "_base_namesake_rows"] = int(sc[key + "_base_namesake_rows"])
+            m[key + "_namesake_base_rows_total"] = sum(int(sc[key + "_base_namesake_rows"]) for _, sc in sidecars.values())
     return m
 
 
@@ -443,7 +520,8 @@ def write_json(path, obj):
 def artifact_manifest(git_head, out, csv_names, models_dir, chain_dir, rows, max_delta,
                       modelset=DEFAULT_MODELSET, namesake_slice=None, plain_cal_diff_at_merge_0_7=None,
                       family_namesake_slice=None, ns_cal_sidecar_sha256=None, max_abs_leaf=None, lambda_l2=None,
-                      xs_namesake_slice=None, name_idf_sha256=None, unseen_token_share=None):
+                      xs_namesake_slice=None, name_idf_sha256=None, unseen_token_share=None,
+                      md_namesake_slice=None, rd_namesake_slice=None, plain_cal_diff_at_merge_0_7_reference=None):
     m = {
         "git_head": git_head,
         "training_manifest_sha256": sha256(os.path.join(out, "training-manifest.json")),
@@ -468,6 +546,12 @@ def artifact_manifest(git_head, out, csv_names, models_dir, chain_dir, rows, max
         m["name_idf_sha256"] = name_idf_sha256
     if unseen_token_share is not None:
         m["unseen_token_share"] = unseen_token_share
+    if md_namesake_slice is not None:
+        m["md_namesake_slice"] = md_namesake_slice
+    if rd_namesake_slice is not None:
+        m["rd_namesake_slice"] = rd_namesake_slice
+    if plain_cal_diff_at_merge_0_7_reference is not None:
+        m["plain_cal_diff_at_merge_0_7_reference"] = plain_cal_diff_at_merge_0_7_reference
     return m
 
 
@@ -492,6 +576,7 @@ def run(cmd, cwd):
 
 
 def main(argv):
+    argv, ref_dir = split_reference(argv)
     arg, modelset = parse_args(argv)
     proto = PROTOCOLS[modelset]
     root = os.path.dirname(os.path.dirname(os.path.realpath(__file__)))
@@ -499,6 +584,11 @@ def main(argv):
     models = os.path.join(root, "models")
     check_models_free(models, modelset)
     check_clean_tree(root)
+    head0 = subprocess.check_output(["git", "-C", root, "rev-parse", "HEAD"], text=True).strip()
+    try:
+        reference = read_reference(os.path.abspath(ref_dir) if ref_dir else None, modelset, head0)
+    except (OSError, ValueError) as e:
+        sys.exit(f"--reference: {e}")
     run(["go", "build", "-o", os.path.join(root, "bin", "wsi7"), "."], os.path.join(root, "pipeline"))
     os.mkdir(out)
     chain = os.path.join(out, "chain")
@@ -570,6 +660,16 @@ def main(argv):
             extra["ns_cal_sidecar_sha256"] = sha256(ns_path + ".namesakes.json")
         if proto.get("xs_namesakes"):
             extra["xs_namesake_slice"] = xs_namesake_slice(ns_path, ns_path + ".namesakes.json", width=width + 1)
+        for key, _ in HARD_NEGATIVES7:
+            if proto.get(key + "_namesakes"):
+                extra[key + "_namesake_slice"] = hard_negative_slice(ns_path, ns_path + ".namesakes.json", key,
+                                                                     width=width + 1)
+    if reference is not None:  # pairfeat7: reported beside the candidate's count, not gated
+        extra["plain_cal_diff_at_merge_0_7_reference"] = reference
+        print(f"plain cal different-at-merge-0.7 (reported, not gated): {modelset} "
+              f"{extra['plain_cal_diff_at_merge_0_7']} vs {reference['modelset']} {reference['plain_cal_diff_at_merge_0_7']}")
+        for key in ("xs", "md", "rd"):
+            print(f"{key} hard-negative slice (namesake cal dump): {json.dumps(extra[key + '_namesake_slice'], sort_keys=True)}")
     if idf:
         with open(idf) as fh:
             n_reg = json.load(fh)["registrations"]
```

  `.gitignore`: add `models/*_pairfeat7.json` after `models/*_pairfeat6.json` (it also covers `models/name_idf_pairfeat7.json`).
- [ ] **Step 4: GREEN.** `python3 -I encoder/test_fit_pairfeat1.py && python3 -I encoder/test_fit_scorer.py` (all 75 fit-driver tests pass, including `FitByteIdentity`, the byte-identity of `pairfeat1`-`pairfeat6`; the fit-scorer tests are unchanged and pass).
- [ ] **Step 5: Commit.**

```bash
git commit -m "pairfeat7 fit: missing-date and same-date hard negatives, per-kind floors and slices, reference count" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" \
  -- encoder/fit_pairfeat1.py encoder/test_fit_pairfeat1.py .gitignore
git show --stat HEAD    # exactly the three files
```

### Task D: three-arm pairfeat7 candidate in the harness (Python/Bash)

**Files:** `scripts/pairfeat_compare.py`, `scripts/pairfeat_experiment.sh`, `scripts/test_pairfeat_compare.py`.

**Interfaces:**
- Consumes (Task C): the `pairfeat7` artifact manifest (`pairfeat7_chain_sha256` over five files) and the separate `pairfeat5` reference fit's artifact manifest.
- Produces (`scripts/pairfeat_compare.py`):
  - `CANDIDATES["pairfeat7"]` = `pairfeat6`'s entry with `"dev_seeds": [719, 727, 733]`, `"validation_seeds": [739, 743, 751, 757, 761]` and `"edge_classes": "pairfeat7"`
  - `MODELSET["pairfeat7"] = "pairfeat7"`; `CHAIN_FILES["pairfeat7"]` = the four `*_pairfeat7.json` chain files plus `name_idf_pairfeat7.json`
  - `EDGE_CLASSES7 = ("cross_script_lookalike", "missing_dob_given_only", "missing_dob_ambiguous", "same_real_dob_different_names", "shared_token", "other", "unresolved")`
  - `_ordered_tokens(s) -> list`, `_real_dob(d) -> bool`, `_given_only(na, nb) -> bool`, `edge_class7(ra, rb) -> str`
  - `EDGE_CLASSIFIERS = {"pairfeat7": (EDGE_CLASSES7, edge_class7)}`, `edge_classifier(candidate) -> (names, classify)` (`(EDGE_CLASSES, edge_class)` for every other candidate)
  - `linked_pair_report(lines, pk_qid, regs=None, candidate=None)`; `aggregate_arm` sums the lanes' own edge-class keys (equal key sets required)
  - `_pairfeat6_gate` checks the arms of any candidate whose `reference_gate` is `"pairfeat6"`
  - `init-experiment EXPERIMENT_DIR pairfeat7 REFERENCE_FIT_DIR`, `freeze-validation`, the guard and preflight work for `pairfeat7` unchanged (through `reference_chain: "experiment"`)
- Produces (`scripts/pairfeat_experiment.sh`): arm `pairfeat7`; seed table line `pairfeat7) DEV_SEEDS="719 727 733"; VAL_SEEDS="739 743 751 757 761"; REF=pairfeat5; REFCHAIN=experiment;;`.

- [ ] **Step 0: Confirm the pairfeat6 and r1review goldens at BASE** (before editing; HEAD has Tasks A-C only, the harness is still `acefac5`'s):

```bash
git diff --quiet acefac5 HEAD -- scripts && echo "scripts at BASE"    # must print it
cd scripts && python3 -I - <<'PY'
import contextlib, hashlib, io, os, sys
sys.path.insert(0, os.getcwd())
import test_pairfeat_compare as T
pfc = T.pfc


def tables(c):
    root, bin_, o = "/repo", "/repo/bin/wsi7", "/data/exp/arm/v2-s0"
    return {"arms": list(pfc.arms(c)), "modes": pfc.modes(c), "review_gate": pfc.review_gate(c),
            "settings": {m: pfc.expected_settings(m, c) for m in pfc.MODES},
            "chain": {a: [pfc.CHAIN_KEY[a], list(pfc.CHAIN_FILES[a]), pfc.MODELSET[a]] for a in pfc.arms(c)},
            "lanes": {m: {str(s): {a: pfc.lane_commands(root, bin_, o, s, m, a, candidate=c) for a in pfc.arms(c)}
                          for s in pfc.modes(c)[m]["seeds"]} for m in pfc.MODES}}


for name, c, cls in (("pairfeat6", T.P6, T.CLIPairfeat6), ("r1review", T.R1, T.CLIR1review)):
    t = cls(); t.setUp()
    try:
        with contextlib.redirect_stdout(io.StringIO()):
            assert t.run3("dev-clean", t.three()) == 0
        got = {n: hashlib.sha256(T._rb(os.path.join(t.exp, n))).hexdigest() for n in ("dev-clean-summary.json", "selection.json")}
    finally:
        t.tearDown()
    print(name, "driver", T._digest(tables(c)), "dev-clean", T._digest(got))
PY
```

  Expected:
  - `pairfeat6 driver e8dab315ae162db1d8af6d7bacbbe960c532dbdd35b76105908421939bc1b5d8 dev-clean 9ff3c02bbd1f1041f1e87d5e83ce2523113618e70ad8f41dbe4c9e684848c7f5`
  - `r1review driver 67706e7258577a131cb022260d8e1222ec103a974c86601abfa0b8924c0a4609 dev-clean f17691f0fa469dc7c4efa1b164d4ade29fd75a03fbcea4f7bf8510f543d9d53c` (the values `R1reviewByteIdentity` already pins)

  Stop if any differs.

- [ ] **Step 1: Failing tests.** In `scripts/test_pairfeat_compare.py`:
  - First move every unknown-candidate sentinel off `pairfeat7`: `sed -i '' 's/pairfeat7/pairfeat8/g' scripts/test_pairfeat_compare.py`, then confirm with `grep -c pairfeat8 scripts/test_pairfeat_compare.py` that exactly the 13 sentinel lines changed (today's lines 1024 1058 1128 1137 1148 1195 1310 1389 1393 1567 1588 2470 2505) and `grep -c pairfeat7 scripts/test_pairfeat_compare.py` prints 0.
  - `Candidates.test_candidate_set_and_seeds`: the expected `CANDIDATES` dict's last entry becomes `"pairfeat6": P6_ENTRY, "pairfeat7": P7_ENTRY})`; `ALLOWED_ARMS` becomes `("baseline", "pairfeat1", "pairfeat2", "pairfeat3", "pairfeat4", "pairfeat5", "r1review", "pairfeat6", "pairfeat7")`; the expected `MODELSET` ends `"pairfeat6": "pairfeat6", "pairfeat7": "pairfeat7"})`.
  - `Pairfeat6Candidate.test_entry_and_tables` (line 3481): `if c != P6))` becomes `if c not in (P6, P7)))`.
  - Insert before the final `if __name__ == "__main__":` block:

```python
# ================================================================ candidate pairfeat7 (pairfeat6's three arms and gates, two more edge classes)

P7 = "pairfeat7"
P7_DEV = [719, 727, 733]
P7_VAL = [739, 743, 751, 757, 761]
CAND7_FILES = ("encoder_pairfeat7.json", "lgb_stream_pairfeat7.json", "iso_pairfeat7.json", "iso_v7_pairfeat7.json",
               "name_idf_pairfeat7.json")
FILES[P7] = CAND7_FILES
DEV_SEEDS[P7] = P7_DEV
P7_ENTRY = {"dev_seeds": [719, 727, 733], "validation_seeds": [739, 743, 751, 757, 761], "review_gate": "wrong_pair_cap",
            "report_xscript_cap": True, "reference": "pairfeat5", "reference_chain": "experiment",
            "reference_gate": "pairfeat6", "baseline_gating": False, "report_linked_pairs": True, "edge_classes": "pairfeat7"}
# pairfeat6's used development seeds and its never-run validation seeds join the never-touch list
NEVER_TOUCH7 = USED_DEV + [659, 661, 673] + USED_VAL + [677, 683, 691, 701, 709] + B8_SEEDS
# pairfeat6 driver tables and dev-clean analysis outputs computed with the harness at acefac5 (before pairfeat7)
P6_GOLDEN = {"driver": "e8dab315ae162db1d8af6d7bacbbe960c532dbdd35b76105908421939bc1b5d8",
             "dev-clean": "9ff3c02bbd1f1041f1e87d5e83ce2523113618e70ad8f41dbe4c9e684848c7f5"}
# one pair per new class and its pairfeat6 neighbour: given name only / initial (both one DOB missing); the same real date
# with no shared token / the same 1 January placeholder
PF7_PATIENTS = [("g|a", "Mohammed Salem", ""), ("g|b", "Mohammed Karim", "1980-02-02"),
                ("t|a", "Mohammed Salem", ""), ("t|b", "Mohammed S", "1980-02-02"),
                ("r|a", "Omar Faris", "1975-06-14"), ("r|b", "Karim Nasser", "1975-06-14"),
                ("q|a", "Omar Faris", "1975-01-01"), ("q|b", "Karim Nasser", "1975-01-01")]
PF7_PK = {"g|a": "G1", "g|b": "G2", "t|a": "T1", "t|b": "T2", "r|a": "R1", "r|b": "R2", "q|a": "Q1", "q|b": "Q2"}
PF7_LINKED = [link("g|a", "g|b"), link("t|a", "t|b"), link("r|a", "r|b"), link("q|b", "q|a")]


def p6_tables():
    c, root, bin_, o = P6, "/repo", "/repo/bin/wsi7", "/data/exp/arm/v2-s0"
    return {"arms": list(pfc.arms(c)), "modes": pfc.modes(c), "review_gate": pfc.review_gate(c),
            "settings": {m: pfc.expected_settings(m, c) for m in pfc.MODES},
            "chain": {a: [pfc.CHAIN_KEY[a], list(pfc.CHAIN_FILES[a]), pfc.MODELSET[a]] for a in pfc.arms(c)},
            "lanes": {m: {str(s): {a: pfc.lane_commands(root, bin_, o, s, m, a, candidate=c) for a in pfc.arms(c)}
                          for s in pfc.modes(c)[m]["seeds"]} for m in pfc.MODES}}


class Pairfeat6ByteIdentity(unittest.TestCase):
    """pairfeat6 driver tables and dev-clean analysis outputs equal the harness's at acefac5."""

    def test_driver_tables_unchanged(self):
        self.assertEqual(_digest(p6_tables()), P6_GOLDEN["driver"])

    def test_dev_clean_outputs_unchanged(self):
        t = CLIPairfeat6()
        t.setUp()
        try:
            self.assertEqual(t.run3("dev-clean", t.three()), 0)
            got = {n: hashlib.sha256(_rb(os.path.join(t.exp, n))).hexdigest() for n in ("dev-clean-summary.json", "selection.json")}
        finally:
            t.tearDown()
        self.assertEqual(_digest(got), P6_GOLDEN["dev-clean"])


class Pairfeat7Candidate(unittest.TestCase):
    def test_entry_and_tables(self):
        self.assertEqual(pfc.CANDIDATES[P7], P7_ENTRY)
        self.assertEqual({k: v for k, v in P7_ENTRY.items() if k not in ("dev_seeds", "validation_seeds", "edge_classes")},
                         {k: v for k, v in P6_ENTRY.items() if k not in ("dev_seeds", "validation_seeds")})
        self.assertEqual(pfc.arms(P7), ("baseline", P5, P7))
        self.assertEqual(pfc.modes(P7), {
            "dev-clean": {"split": "dev", "seeds": P7_DEV, "rerr": "0", "errungraded": "0"},
            "dev-stress": {"split": "dev", "seeds": P7_DEV, "rerr": "0.04", "errungraded": "0.25"},
            "validation-clean": {"split": "eval", "seeds": P7_VAL, "rerr": "0", "errungraded": "0"}})
        self.assertEqual(pfc.CHAIN_FILES[P7], CAND7_FILES)
        self.assertEqual((pfc.MODELSET[P7], pfc.CHAIN_KEY[P7], pfc.chain_of(P7), pfc.reference(P7), pfc.review_gate(P7)),
                         (P7, "pairfeat7_chain_sha256", P7, P5, "wrong_pair_cap"))
        self.assertEqual(pfc.extra_flags(P7), [])
        self.assertFalse(pfc.lane_image(P7) or pfc.baseline_gating(P7))
        self.assertTrue(pfc.reference_chain_in_experiment(P7))
        self.assertIn(P7, pfc.CHAIN_CANDIDATES)
        s = pfc.expected_settings("validation-clean", P7)
        self.assertEqual((s["image"], s["reference"], s["candidate_extra_flags"], s["arms"], s["candidate_modelset"], s["seeds"]),
                         ("off", P5, [], ["baseline", P5, P7], P7, P7_VAL))
        self.assertEqual(pfc.edge_classifier(P7), (pfc.EDGE_CLASSES7, pfc.edge_class7))
        for c in pfc.CANDIDATES:
            if c != P7:
                self.assertEqual(pfc.edge_classifier(c), (pfc.EDGE_CLASSES, pfc.edge_class))

    def test_seeds_never_touched_before(self):
        used = set(NEVER_TOUCH7 + list(pfc.RETIRED_SEEDS) + pfc.VALIDATION_SEEDS)
        for c, spec in pfc.CANDIDATES.items():
            if c != P7:
                used |= set(spec["dev_seeds"] + spec.get("validation_seeds", []))
        self.assertFalse(set(P7_DEV + P7_VAL) & used)
        self.assertTrue(all(s > 709 for s in P7_DEV + P7_VAL))


class Pairfeat7Guard(unittest.TestCase):
    BIN = "/repo/bin/wsi7"

    def cmds(self, mode, a, seed):
        return pfc.lane_commands("/repo", self.BIN, "/data/exp/arm/v2-s%d" % seed, seed, mode, a, candidate=P7)

    def test_lane_argv_per_arm(self):
        for a, ms in (("baseline", None), (P5, "pairfeat5"), (P7, "pairfeat7")):
            for name, argv, _ in self.cmds("dev-clean", a, 719):
                if name in pfc.MODEL_CMDS:
                    if ms:
                        self.assertEqual(argv[argv.index("-modelset") + 1], ms)
                    else:
                        self.assertNotIn("-modelset", argv)
                    self.assertEqual(argv[argv.index("-image") + 1], "off")
                for f in ("-r1review", "-imgphotos", "-imgcache"):
                    self.assertNotIn(f, argv)
        self.assertEqual(self.cmds("dev-clean", P5, 719),
                         pfc.lane_commands("/repo", self.BIN, "/data/exp/arm/v2-s719", 719, "dev-clean", P5))

    def test_every_lane_passes_the_guard(self):
        for mode in pfc.MODES:
            for s in pfc.mode_settings(mode, P7)["seeds"]:
                for a in pfc.arms(P7):
                    for _, argv, _ in self.cmds(mode, a, s):
                        pfc.guard(argv, mode, a, self.BIN, P7)

    def test_refusals(self):
        orch = {n: v for n, v, _ in self.cmds("dev-clean", P5, 719)}["orchestrate"]
        bad = list(orch)
        bad[bad.index("-modelset") + 1] = "pairfeat7"
        with self.assertRaises(pfc.GuardError):
            pfc.guard(bad, "dev-clean", P5, self.BIN, P7)
        for a in (R1, P4, P6):
            with self.assertRaises(pfc.GuardError):
                pfc.guard(orch, "dev-clean", a, self.BIN, P7)
        # pairfeat6's development and never-run validation seeds, another experiment's, old validation, validation in dev, retired
        for seed in (659, 661, 673, 677, 709, 571, 401, 739, 557):
            for name, argv, _ in self.cmds("dev-clean", P7, seed):
                if name in ("gen", "orchestrate"):
                    with self.assertRaises(pfc.GuardError, msg=(name, seed)):
                        pfc.guard(argv, "dev-clean", P7, self.BIN, P7)
        for seed in (677, 683, 691, 701, 709):  # never run in a pairfeat7 validation either
            for name, argv, _ in self.cmds("validation-clean", P7, seed):
                if name in ("gen", "orchestrate"):
                    with self.assertRaises(pfc.GuardError, msg=(name, seed)):
                        pfc.guard(argv, "validation-clean", P7, self.BIN, P7)


class Pairfeat7Preflight(unittest.TestCase):
    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.r = Repo(self.td.name, cand=P7)
        for f in CAND5_FILES + CAND7_FILES:
            with open(os.path.join(self.r.root, "models", f), "w") as h:
                h.write("model " + f)
        self.ref = os.path.join(self.td.name, "ref")
        os.makedirs(self.ref)
        _wj(os.path.join(self.ref, "artifact-manifest.json"), {"git_head": HEAD, "baseline_chain_sha256": self.r.hashes(BASE_FILES),
                                                               "pairfeat5_chain_sha256": self.r.hashes(CAND5_FILES)})
        self.ep = os.path.join(self.r.exp, "experiment.json")

    def tearDown(self):
        self.td.cleanup()

    def test_init_preflight_freeze(self):
        self.r.artifact_manifest()
        with self.assertRaises(pfc.PreflightError):
            pfc.init_experiment(self.r.exp, P6, self.ref, repo=self.r.root)    # the manifest carries the pairfeat7 chain
        want = {"candidate": P7, "reference": P5, "reference_model_chain": self.r.hashes(CAND5_FILES),
                "reference_artifact_manifest_sha256": _sha(os.path.join(self.ref, "artifact-manifest.json"))}
        self.assertEqual(pfc.init_experiment(self.r.exp, P7, self.ref, repo=self.r.root), want)
        for a, files in (("baseline", BASE_FILES), (P5, CAND5_FILES), (P7, CAND7_FILES)):
            self.assertEqual(pfc.preflight_identity("dev-clean", a, self.r.exp, self.r.root, HEAD), (P7, self.r.hashes(files)))
        for a in (R1, P6):
            with self.assertRaises(pfc.PreflightError):
                pfc.preflight_identity("dev-clean", a, self.r.exp, self.r.root, HEAD)
        ident = {"git_head": HEAD, "baseline_chain_sha256": self.r.hashes(BASE_FILES), "pairfeat7_chain_sha256": self.r.hashes(CAND7_FILES),
                 "reference_model_chain": self.r.hashes(CAND5_FILES), "candidate": P7}
        sp = os.path.join(self.r.exp, "selection.json")
        _wj(sp, dict(ident, selected_modelset=P7, mode="dev-clean", seeds=P7_DEV, **{"pass": True}))
        _wj(os.path.join(self.r.exp, "stress.json"), dict(ident, mode="dev-stress", seeds=P7_DEV, selection_sha256=_sha(sp), **{"pass": True}))
        vm = pfc.freeze_validation(self.r.exp, self.r.root, HEAD)
        self.assertEqual((vm["candidate"], vm["seeds"], vm["reference_model_chain"]), (P7, P7_VAL, self.r.hashes(CAND5_FILES)))
        for a, files in (("baseline", BASE_FILES), (P5, CAND5_FILES), (P7, CAND7_FILES)):
            self.assertEqual(pfc.preflight_identity("validation-clean", a, self.r.exp, self.r.root, HEAD), (P7, self.r.hashes(files)))


class Pairfeat7Gate(unittest.TestCase):
    def g(self, cand, kind="selection", ref=None):
        return pfc.reference_gate(ref or pf6_arm(P5, 530), cand, kind, base=pf6_arm("baseline", 500))

    def test_same_gate_as_pairfeat6(self):
        for kind in pfc.GATE_KINDS:
            for kw in (dict(fm=9, cpe=9, uncapped=6, hours=11.0), dict(fm=10, cpe=9), dict(fm=11, cpe=11), dict(uncapped=7),
                       dict(hours=11.01), dict(fm=9, cpe=9, false_closures=1)):
                g6, g7 = self.g(pf6_arm(P6, 530, **kw), kind), self.g(pf6_arm(P7, 530, **kw), kind)
                self.assertEqual(json.dumps(g7, sort_keys=True), json.dumps(g6, sort_keys=True).replace(P6, P7), (kind, kw))

    def test_arm_names(self):
        self.assertEqual(self.g(pf6_arm(P7, 530, fm=9, cpe=9), ref=pf6_arm(P4, 530))["reasons"],
                         ["arms must be pairfeat7 vs its reference arm pairfeat5"])
        self.assertEqual(self.g(pf6_arm(P6, 530, fm=9, cpe=9), ref=pf6_arm(P4, 530))["reasons"],
                         ["arms must be pairfeat6 vs its reference arm pairfeat5"])


class EdgeClass7(unittest.TestCase):
    def test_classes(self):
        r = lambda n, d="1980-05-17": {"name": n, "dob": d}
        for ra, rb, want in (
                (r("محمد سالم"), r("Mohammed Karim"), "cross_script_lookalike"),
                (r("Mohammed Salem", ""), r("Mohammed Karim Nasser"), "missing_dob_given_only"),
                (r("Fatima Ali", ""), r("Fatima Hassan", ""), "missing_dob_given_only"),
                (r("Mohammed Salem", ""), r("Mohammed S"), "missing_dob_ambiguous"),          # initial
                (r("Abdul Rahman", ""), r("Abdul Rahmani"), "missing_dob_ambiguous"),        # truncation
                (r("Ahmed Ali", ""), r("Ahmed Ali"), "missing_dob_ambiguous"),               # the same name
                (r("Mohammed Salem", ""), r("Mohammed Salem Nasser"), "missing_dob_ambiguous"),
                (r("Mohammed Salem", ""), r("Ahmed Karim"), "missing_dob_ambiguous"),        # no shared given name
                (r("Mohammed", ""), r("Mohammed Karim"), "missing_dob_ambiguous"),           # single token
                (r("Omar Faris"), r("Karim Nasser"), "same_real_dob_different_names"),
                (r("Omar Faris", "1980-01-01"), r("Karim Nasser", "1980-01-01"), "other"),   # placeholder
                (r("Omar Faris", "1980-07-01"), r("Karim Nasser", "1980-07-01"), "other"),
                (r("Omar Faris", "1980-05-00"), r("Karim Nasser", "1980-05-00"), "other"),   # day 00
                (r("Omar Faris", "1980-02-30"), r("Karim Nasser", "1980-02-30"), "other"),   # invalid
                (r("Omar Faris"), r("Karim Nasser", "1980-05-18"), "other"),                 # different dates
                (r("Ahmed Ali"), r("ahmed ali"), "other"),                                   # identical token sets
                (r("Mohammed Salem"), r("Mohammed Nasser"), "shared_token"),
                (None, r("Ahmed Ali"), "unresolved")):
            self.assertEqual(pfc.edge_class7(ra, rb), want, (ra, rb))
            self.assertEqual(pfc.edge_class7(rb, ra), want, (rb, ra))
            # a refinement of edge_class: merging the two new classes back gives pairfeat6's class
            back = {"missing_dob_given_only": "missing_dob_ambiguous", "same_real_dob_different_names": "other"}
            self.assertEqual(back.get(want, want), pfc.edge_class(ra, rb))
        self.assertEqual(set(pfc.EDGE_CLASSES7) - set(pfc.EDGE_CLASSES), {"missing_dob_given_only", "same_real_dob_different_names"})

    def test_linked_pair_report_and_aggregate(self):
        pk = dict(PF6_PK, **PF7_PK)
        regs = {k: {"name": n, "dob": d} for k, n, d in PF6_PATIENTS + PF7_PATIENTS}
        lines = trace_lines(PF6_LINKED + PF7_LINKED).splitlines(True)
        want7 = {"cross_script_lookalike": 1, "missing_dob_given_only": 1, "missing_dob_ambiguous": 2,
                 "same_real_dob_different_names": 1, "shared_token": 1, "other": 1, "unresolved": 0}
        self.assertEqual(pfc.linked_pair_report(lines, pk, regs, P7),
                         {"same_person": 1, "different_person": 7, "edge_classes": want7})
        self.assertEqual(pfc.linked_pair_report(lines, pk, regs, P6)["edge_classes"],
                         {"cross_script_lookalike": 1, "missing_dob_ambiguous": 3, "shared_token": 1, "other": 2, "unresolved": 0})
        self.assertEqual(pfc.linked_pair_report(lines, pk, regs), pfc.linked_pair_report(lines, pk, regs, P6))
        lp7 = pfc.linked_pair_report(lines, pk, regs, P7)
        lanes = [pfc.lane_metrics(s, report(), funnel(), {}, LOAD, linked=lp7) for s in (1, 2, 3)]
        self.assertEqual(pfc.aggregate_arm(P7, lanes)["linked_pairs"]["edge_classes"], {k: 3 * v for k, v in want7.items()})
        mixed = lanes[:2] + [pfc.lane_metrics(3, report(), funnel(), {}, LOAD, linked=pfc.linked_pair_report(lines, pk, regs, P6))]
        with self.assertRaises(ValueError):
            pfc.aggregate_arm(P7, mixed)


class CLIPairfeat7(CLIPairfeat6):
    """pairfeat7 lanes: pairfeat6's synthetic lanes plus one pair per new edge class (CLIPairfeat6.make_arm extended).
    CLIPairfeat6's own tests are not re-run here (they are nulled below)."""
    CAND = P7

    def setUp(self):
        self.td = tempfile.TemporaryDirectory()
        self.exp = os.path.join(self.td.name, "exp")
        os.makedirs(self.exp)
        self.maps = {"baseline": chain(BASE_FILES, "b"), P5: chain(CAND5_FILES, "r"), P7: chain(CAND7_FILES, "c")}
        _wj(os.path.join(self.exp, "artifact-manifest.json"),
            {"git_head": HEAD, "baseline_chain_sha256": self.maps["baseline"], "pairfeat7_chain_sha256": self.maps[P7]})
        _wj(os.path.join(self.exp, "experiment.json"), {"candidate": P7, "reference": P5, "reference_model_chain": self.maps[P5],
                                                       "reference_artifact_manifest_sha256": "d" * 64})

    def make_arm(self, a, tp, *args, **kw):
        root = super().make_arm(a, tp, *args, **kw)
        for d in sorted(os.listdir(root)):
            if not d.startswith("v2-s"):
                continue
            lane = os.path.join(root, d)
            t = _rj(os.path.join(lane, "cohort/truth.json"))
            t["pk_qid"].update(PF7_PK)
            _wj(os.path.join(lane, "cohort/truth.json"), t)
            with open(os.path.join(lane, "cohort/fhir_p.ndjson"), "a") as h:
                h.write(trace_lines([patient_line(k, n, b) for k, n, b in PF7_PATIENTS]))
            with open(os.path.join(lane, "run/main/link_funnel.ndjson"), "a") as h:
                h.write(trace_lines(PF7_LINKED))
        return root

    def three(self, mode="dev-clean", b=5, f=6, c=7, **ck):
        cand = dict(fm=1, cpe=2)
        cand.update(ck)
        return (self.make_arm("baseline", b, mode=mode, fm=3), self.make_arm(P5, f, mode=mode, fm=3),
                self.make_arm(P7, c, mode=mode, **cand))

    def run3(self, mode, roots, out=None):
        b, f, c = roots
        return pfc.main([mode, out or os.path.join(self.exp, mode + "-summary.json"), "baseline=" + b, "pairfeat5=" + f,
                         "pairfeat7=" + c])

    def test_p7_dev_clean(self):
        self.assertEqual(self.run3("dev-clean", self.three()), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual((sel["candidate"], sel["reference"], sel["selected_modelset"], sel["pass"], sel["seeds"]),
                         (P7, P5, P7, True, P7_DEV))
        self.assertEqual((sel["pairfeat7_chain_sha256"], sel["reference_model_chain"]), (self.maps[P7], self.maps[P5]))
        self.assertEqual(sel["reference_gate"]["contamination"], {"false_merge_pairs": {P5: 9, P7: 3, "rule": "strictly below"},
                                                                  "cross_person_edges": {P5: 9, P7: 6, "rule": "strictly below"}})
        s = _rj(os.path.join(self.exp, "dev-clean-summary.json"))
        self.assertEqual(s["rarity_table_sha256"], self.maps[P7]["name_idf_pairfeat7.json"])
        for a in (P5, P7):
            self.assertEqual(s["arms"][a]["linked_pairs"], {"same_person": 3, "different_person": 21, "edge_classes": {
                "cross_script_lookalike": 3, "missing_dob_given_only": 3, "missing_dob_ambiguous": 6,
                "same_real_dob_different_names": 3, "shared_token": 3, "other": 3, "unresolved": 0}})
        self.assertNotIn("linked_pairs", s["arms"]["baseline"])

    def test_p7_contamination_must_fall(self):
        self.assertEqual(self.run3("dev-clean", self.three(fm=3)), 0)
        sel = _rj(os.path.join(self.exp, "selection.json"))
        self.assertEqual(sel["reference_gate"]["reasons"], ["false_merge_pairs not strictly below reference (9 >= 9)"])
        self.assertEqual((sel["selected_modelset"], sel["pass"]), (None, False))

    def test_p7_validation_has_no_classes(self):
        r = Repo(os.path.join(self.td.name, "v"), cand=P7)
        for fn in CAND5_FILES + CAND7_FILES:
            with open(os.path.join(r.root, "models", fn), "w") as h:
                h.write("model " + fn)
        ref = os.path.join(self.td.name, "v", "ref")
        os.makedirs(ref)
        _wj(os.path.join(ref, "artifact-manifest.json"), {"git_head": HEAD, "baseline_chain_sha256": r.hashes(BASE_FILES),
                                                          "pairfeat5_chain_sha256": r.hashes(CAND5_FILES)})
        self.exp = r.exp
        self.maps = {"baseline": r.hashes(BASE_FILES), P5: r.hashes(CAND5_FILES), P7: r.hashes(CAND7_FILES)}
        r.artifact_manifest()
        pfc.init_experiment(r.exp, P7, ref, repo=r.root)
        self.assertEqual(self.run3("dev-clean", self.three()), 0)
        self.assertEqual(self.run3("dev-stress", self.three(mode="dev-stress", fm=3, cpe=3)), 0)
        pfc.freeze_validation(r.exp, r.root, HEAD)
        v3 = self.three(mode="validation-clean")
        res = pfc.analyse("validation-clean", os.path.join(r.exp, "validation-clean-summary.json"),
                          {"baseline": v3[0], P5: v3[1], P7: v3[2]}, repo=r.root, head=HEAD)
        self.assertTrue(res["pass"])
        self.assertEqual(res["arms"][P7]["linked_pairs"], {"same_person": 5, "different_person": 35})


for _n in [n for n in vars(CLIPairfeat6) if n.startswith("test_")]:
    setattr(CLIPairfeat7, _n, None)  # pairfeat6's assertions (P6 constants) are not re-run with pairfeat7 lanes


class RunnerPairfeat7(RunnerFixture, unittest.TestCase):
    def write_experiment(self, cand=P7):
        _wj(os.path.join(self.exp, "experiment.json"), {"candidate": cand, "reference": P5,
                                                       "reference_model_chain": chain(CAND5_FILES, "r"),
                                                       "reference_artifact_manifest_sha256": "d" * 64})

    def test_parent_needs_experiment_json(self):
        self.real_artifact_manifest(cand=P7)
        self.assert_refused(self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True))   # no experiment.json
        self.write_experiment(P6)
        self.assert_refused(self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True))   # names pairfeat6
        os.remove(os.path.join(self.exp, "experiment.json"))
        self.write_experiment()
        r = self.run_sh(self.root, "dev-clean", "baseline", preflight_only=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("candidate pairfeat7 seeds 719 727 733", r.stdout)
        for a in (P5, P7):
            r = self.run_sh(self.root, "dev-clean", a, preflight_only=True)
            self.assert_refused(r)
            self.assertNotIn("invalid arm", r.stderr)
        for a in (R1, P6):
            self.assert_refused(self.run_sh(self.root, "dev-clean", a, preflight_only=True))

    def test_runner_tables(self):
        with open(SCRIPT) as h:
            text = h.read()
        self.assertIn('pairfeat7) DEV_SEEDS="719 727 733"; VAL_SEEDS="739 743 751 757 761"; REF=pairfeat5; REFCHAIN=experiment;;', text)
        self.assertIn("baseline|pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5|pairfeat6) ;; pairfeat7) ;; r1review) ;;", text)

    def test_independent_recheck(self):
        self.real_artifact_manifest(cand=P7)
        self.assertNotEqual(runner_recheck(self.exp, self.head, P7, P7, P5, "experiment"), 0)   # no experiment.json
        self.write_experiment()
        self.assertEqual(runner_recheck(self.exp, self.head, P7, P7, P5, "experiment"), 0)
        self.assertNotEqual(runner_recheck(self.exp, self.head, P7, P7), 0)
        self.assertNotEqual(runner_recheck(self.exp, self.head, P6, P6, P5, "experiment"), 0)  # the manifest carries pairfeat7
```

- [ ] **Step 2: RED.** `python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'` fails (no `pairfeat7` candidate, no `edge_class7`, the runner does not know arm `pairfeat7`); `Pairfeat6ByteIdentity` passes.
- [ ] **Step 3: Implement** (diff against BASE):

```diff
diff --git a/scripts/pairfeat_compare.py b/scripts/pairfeat_compare.py
index ab5afe3..f7fba76 100644
--- a/scripts/pairfeat_compare.py
+++ b/scripts/pairfeat_compare.py
@@ -27,6 +27,11 @@ below in selection and validation, not above in stress; uncapped wrong-pair revi
 hours <= 1.10 x ref; SAFETY zero in all three arms. vs-baseline computed, not gating. Reported: linked pairs (same/different
 person; diagnosed classes on development), capped pairs, rarity-table SHA256.
 
+Three-arm candidate pairfeat7 (development 719 727 733, validation 739 743 751 757 761): exactly pairfeat6's machinery and
+gates (its own five-file chain with name_idf_pairfeat7.json, a separately fitted pairfeat5 reference recorded by
+`init-experiment EXPERIMENT_DIR pairfeat7 REFERENCE_FIT_DIR`), with development edge classes EDGE_CLASSES7: pairfeat6's
+classes plus missing_dob_given_only and same_real_dob_different_names (edge_class7, a refinement of edge_class).
+
 Usage
   pairfeat_compare.py MODE OUT_JSON baseline=ROOT [<reference>=ROOT] <candidate>=ROOT
       MODE dev-clean | dev-stress | validation-clean. OUT_JSON must sit in the experiment parent directory (the directory
@@ -34,7 +39,7 @@ Usage
       (both write-once, next to OUT_JSON). <candidate> must be the parent's candidate; a three-arm candidate also needs
       its reference root (r1review: baseline=, pairfeat5= and r1review=).
   pairfeat_compare.py init-experiment EXPERIMENT_DIR r1review
-  | init-experiment EXPERIMENT_DIR pairfeat6 REFERENCE_FIT_DIR
+  | init-experiment EXPERIMENT_DIR pairfeat6|pairfeat7 REFERENCE_FIT_DIR
       Writes EXPERIMENT_DIR/experiment.json (write-once) when artifact-manifest.json carries exactly the pairfeat5 chain.
   pairfeat_compare.py freeze-validation EXPERIMENT_DIR
       Writes EXPERIMENT_DIR/validation-manifest.json (write-once) only when selection.json selected the parent's candidate
@@ -80,6 +85,7 @@ CRC tp = programmes_live.crc.pred_closed - false_closures (funnel attached_or_cl
 Residual categories and reviewer wrong_final_signoffs are reported outcomes, not gate substitutions.
 Small-n caveat: t intervals over 3 (dev) or 5 (validation) seeds are wide and descriptive only.
 """
+import datetime
 import glob
 import hashlib
 import json
@@ -116,7 +122,13 @@ CANDIDATES = {"pairfeat1": {"dev_seeds": [307, 311, 313], "review_gate": "total"
               "pairfeat6": {"dev_seeds": [659, 661, 673], "validation_seeds": [677, 683, 691, 701, 709],
                             "review_gate": "wrong_pair_cap", "report_xscript_cap": True,
                             "reference": "pairfeat5", "reference_chain": "experiment", "reference_gate": "pairfeat6",
-                            "baseline_gating": False, "report_linked_pairs": True}}
+                            "baseline_gating": False, "report_linked_pairs": True},
+              # pairfeat7 (spec 2026-11-03-crc-pairfeat7-design.md): pairfeat6 plus missing-date and same-real-date hard
+              # negatives; pairfeat6's three arms, reference refit and gates; two more development edge classes
+              "pairfeat7": {"dev_seeds": [719, 727, 733], "validation_seeds": [739, 743, 751, 757, 761],
+                            "review_gate": "wrong_pair_cap", "report_xscript_cap": True,
+                            "reference": "pairfeat5", "reference_chain": "experiment", "reference_gate": "pairfeat6",
+                            "baseline_gating": False, "report_linked_pairs": True, "edge_classes": "pairfeat7"}}
 # Seeds that no lane may ever run again (guard refuses them in every experiment): r1review's first seeds, retired by
 # amendment 3 (development 557 563 569 used once; validation 601 607 613 617 619 retired unused).
 RETIRED_SEEDS = (557, 563, 569, 601, 607, 613, 617, 619)
@@ -145,6 +157,10 @@ REFERENCE_CHAIN_KEY = "reference_model_chain"  # experiment.json / decision file
 EXPERIMENT_KEYS = frozenset({"candidate", "reference"})
 EXPERIMENT_REF_KEYS = EXPERIMENT_KEYS | {REFERENCE_CHAIN_KEY, "reference_artifact_manifest_sha256"}
 EDGE_CLASSES = ("cross_script_lookalike", "missing_dob_ambiguous", "shared_token", "other", "unresolved")
+# pairfeat7 (CANDIDATES[c]["edge_classes"] == "pairfeat7"): edge_class7 splits missing_dob_given_only out of
+# missing_dob_ambiguous and same_real_dob_different_names out of other
+EDGE_CLASSES7 = ("cross_script_lookalike", "missing_dob_given_only", "missing_dob_ambiguous",
+                 "same_real_dob_different_names", "shared_token", "other", "unresolved")
 LINKED_KEYS = ("same_person", "different_person")
 ALLOWED_ARMS = ("baseline",) + tuple(CANDIDATES)
 
@@ -205,7 +221,7 @@ CHAIN_CANDIDATES = tuple(c for c in CANDIDATES if chain_of(c) == c)  # candidate
 EXTRA_FLAGS = tuple(sorted({f for c in CANDIDATES for f in extra_flags(c)}))
 BARE_FLAGS = EXTRA_FLAGS + ("-imgphotos",)  # bare boolean flags the guard counts (and whose misspellings it refuses)
 MODELSET = {"baseline": "", "pairfeat1": "pairfeat1", "pairfeat2": "pairfeat2", "pairfeat3": "pairfeat3", "pairfeat4": "pairfeat4",
-            "pairfeat5": "pairfeat5", "pairfeat6": "pairfeat6"}
+            "pairfeat5": "pairfeat5", "pairfeat6": "pairfeat6", "pairfeat7": "pairfeat7"}
 MODELSET.update({c: MODELSET[chain_of(c)] for c in CANDIDATES if chain_of(c) != c})
 CHAIN_FILES = {
     "baseline": ("encoder_weights.json", "lgb_stream.json", "iso.json", "iso_v7.json"),
@@ -216,6 +232,8 @@ CHAIN_FILES = {
     "pairfeat5": ("encoder_pairfeat5.json", "lgb_stream_pairfeat5.json", "iso_pairfeat5.json", "iso_v7_pairfeat5.json"),
     "pairfeat6": ("encoder_pairfeat6.json", "lgb_stream_pairfeat6.json", "iso_pairfeat6.json", "iso_v7_pairfeat6.json",
                   "name_idf_pairfeat6.json"),
+    "pairfeat7": ("encoder_pairfeat7.json", "lgb_stream_pairfeat7.json", "iso_pairfeat7.json", "iso_v7_pairfeat7.json",
+                  "name_idf_pairfeat7.json"),
 }
 CHAIN_FILES.update({c: CHAIN_FILES[chain_of(c)] for c in CANDIDATES if chain_of(c) != c})
 CHAIN_SUFFIX = "_chain_sha256"
@@ -1065,12 +1083,63 @@ def edge_class(ra, rb):
     return "other"
 
 
-def linked_pair_report(lines, pk_qid, regs=None):
-    """Reported only (pairfeat6 experiments, non-baseline arms): final linked pairs split by truth (same_person = true
-    merges); with regs (development lanes) the different-person pairs are also counted by edge_class. An unmapped
-    registration is an analysis error."""
+def _ordered_tokens(s):
+    return re.findall(r"[^\W\d_]+", s.lower())
+
+
+def _real_dob(d):
+    """A real, non-placeholder birth date: YYYY-MM-DD, a valid calendar date in 1900-2100, not 1 January or 1 July
+    (a day-00 date is not a calendar date)."""
+    if not isinstance(d, str) or not re.fullmatch(r"\d{4}-\d{2}-\d{2}", d) or d[5:] in ("01-01", "07-01"):
+        return False
+    try:
+        y = datetime.date.fromisoformat(d).year
+    except ValueError:
+        return False
+    return 1900 <= y <= 2100
+
+
+def _given_only(na, nb):
+    """Names that share their first token and nothing else: two or more tokens each, equal first tokens, no other token
+    of either name in the other, and no later token of one a prefix of a later token of the other (initials and
+    truncated forms stay missing_dob_ambiguous)."""
+    ta, tb = _ordered_tokens(na), _ordered_tokens(nb)
+    if len(ta) < 2 or len(tb) < 2 or ta[0] != tb[0] or set(ta) & set(tb) != {ta[0]}:
+        return False
+    return not any(x.startswith(y) or y.startswith(x) for x in ta[1:] for y in tb[1:])
+
+
+def edge_class7(ra, rb):
+    """pairfeat7's reported-only class of a remaining different-person linked pair: edge_class, with two of its classes
+    split. A pair edge_class calls missing_dob_ambiguous is missing_dob_given_only when the names share their first
+    token and nothing else (_given_only); a pair it calls other is same_real_dob_different_names when both birth dates
+    are the same real, non-placeholder date (_real_dob) and the names share no token. Every other pair keeps its
+    edge_class."""
+    c = edge_class(ra, rb)
+    if c == "missing_dob_ambiguous" and _given_only(ra["name"], rb["name"]):
+        return "missing_dob_given_only"
+    if c == "other" and _real_dob(ra["dob"]) and ra["dob"] == rb["dob"] and \
+            not _name_tokens(ra["name"]) & _name_tokens(rb["name"]):
+        return "same_real_dob_different_names"
+    return c
+
+
+EDGE_CLASSIFIERS = {"pairfeat7": (EDGE_CLASSES7, edge_class7)}
+
+
+def edge_classifier(candidate):
+    """(class names, classifier) of a candidate's experiment: pairfeat6's unless CANDIDATES[c]["edge_classes"] names
+    another set."""
+    return EDGE_CLASSIFIERS.get(CANDIDATES.get(candidate, {}).get("edge_classes"), (EDGE_CLASSES, edge_class))
+
+
+def linked_pair_report(lines, pk_qid, regs=None, candidate=None):
+    """Reported only (pairfeat6 and pairfeat7 experiments, non-baseline arms): final linked pairs split by truth
+    (same_person = true merges); with regs (development lanes) the different-person pairs are also counted by the
+    experiment's edge classes (edge_classifier). An unmapped registration is an analysis error."""
     same = diff = 0
-    classes = dict.fromkeys(EDGE_CLASSES, 0)
+    names, classify = edge_classifier(candidate)
+    classes = dict.fromkeys(names, 0)
     for a, b in sorted(final_linked_pairs(lines)):
         pa, pb = pk_qid.get(a), pk_qid.get(b)
         if not (isinstance(pa, str) and pa and isinstance(pb, str) and pb):
@@ -1080,7 +1149,7 @@ def linked_pair_report(lines, pk_qid, regs=None):
             continue
         diff += 1
         if regs is not None:
-            classes[edge_class(regs.get(a), regs.get(b))] += 1
+            classes[classify(regs.get(a), regs.get(b))] += 1
     out = {"same_person": same, "different_person": diff}
     if regs is not None:
         out["edge_classes"] = classes
@@ -1195,7 +1264,10 @@ def aggregate_arm(arm, lanes):
         if any(ec):
             if not all(ec):
                 raise ValueError("edge_classes present in only some lanes of arm %s" % arm)
-            a["linked_pairs"]["edge_classes"] = {k: sum(l["linked_pairs"]["edge_classes"][k] for l in lanes) for k in EDGE_CLASSES}
+            keys = set(lanes[0]["linked_pairs"]["edge_classes"])
+            if any(set(l["linked_pairs"]["edge_classes"]) != keys for l in lanes):
+                raise ValueError("lanes of arm %s report different edge classes" % arm)
+            a["linked_pairs"]["edge_classes"] = {k: sum(l["linked_pairs"]["edge_classes"][k] for l in lanes) for k in sorted(keys)}
     return a
 
 
@@ -1338,8 +1410,9 @@ def _pairfeat6_gate(ref, cand, kind, base):
         raise ValueError("the pairfeat6 gate needs the baseline arm (SAFETY is gated in every arm)")
     rk, ck = ref.get("arm"), cand.get("arm")
     reasons = []
-    if ck != "pairfeat6" or rk != reference("pairfeat6"):
-        reasons.append("arms must be pairfeat6 vs its reference arm pairfeat5")
+    own = ck if CANDIDATES.get(ck, {}).get("reference_gate") == "pairfeat6" else "pairfeat6"  # pairfeat6, pairfeat7
+    if ck != own or rk != reference(own):
+        reasons.append("arms must be %s vs its reference arm %s" % (own, reference(own)))
     if not base["truth_closed"] == ref["truth_closed"] == cand["truth_closed"]:
         reasons.append("truth_closed differs between arms")
     gain = None
@@ -1542,7 +1615,7 @@ def _load_lane(path, seed, arm, mode, candidate):
     if CANDIDATES[candidate].get("report_linked_pairs") and arm != "baseline":  # pairfeat6: reported, never gated
         regs = cohort_registrations(os.path.join(path, "cohort")) if MODES[mode]["split"] == "dev" else None
         with open(os.path.join(path, "run/main/link_funnel.ndjson")) as h:
-            linked = linked_pair_report(h, pk, regs)
+            linked = linked_pair_report(h, pk, regs, candidate)
     if lane_image(candidate):
         check_lane_image(r, os.path.join(path, "run/main/out_log.ndjson"), arm, candidate)
     return lane_metrics(seed, r, f, j("run/main/status-linker.json"), review_load(q, dec, pk, capped), xs, candidate, cu, linked)
diff --git a/scripts/pairfeat_experiment.sh b/scripts/pairfeat_experiment.sh
index d40e9c6..a745ce8 100755
--- a/scripts/pairfeat_experiment.sh
+++ b/scripts/pairfeat_experiment.sh
@@ -3,12 +3,13 @@
 # Usage: scripts/pairfeat_experiment.sh NEW_ROOT MODE ARM
 #   MODE  dev-clean (dev; candidate dev seeds; rerr 0/0) | dev-stress (dev; candidate dev seeds; rerr 0.04, ungraded 0.25)
 #         | validation-clean (eval; 401 409 419 421 431, r1review 631 641 643 647 653; rerr 0/0)
-#   ARM   baseline (default modelset) | pairfeat1 | pairfeat2 | pairfeat3 | pairfeat4 | pairfeat5 | pairfeat6 (-modelset <candidate> on orchestrate/audit/report7 only)
+#   ARM   baseline (default modelset) | pairfeat1 | pairfeat2 | pairfeat3 | pairfeat4 | pairfeat5 | pairfeat6 | pairfeat7 (-modelset <candidate> on orchestrate/audit/report7 only)
 #         | r1review (-modelset pairfeat5 plus -r1review on orchestrate/audit/report7 only)
 # Candidate dev seeds: pairfeat1 307 311 313; pairfeat2 317 331 337; pairfeat3 347 353 367; pairfeat4 433 449 461; pairfeat5 521 541 547;
 # pairfeat6 659 661 673 (validation 677 683 691 701 709); a pairfeat6 parent (its own chain) needs experiment.json from
 # `init-experiment EXPERIMENT_DIR pairfeat6 REFERENCE_FIT_DIR`, which records the separately fitted pairfeat5 reference; it
-# serves baseline, pairfeat5 and pairfeat6, image off;
+# serves baseline, pairfeat5 and pairfeat6, image off; pairfeat7 719 727 733 (validation 739 743 751 757 761), the same
+# shape (`init-experiment EXPERIMENT_DIR pairfeat7 REFERENCE_FIT_DIR`; baseline, pairfeat5 and pairfeat7, image off);
 # r1review 571 577 587 (amendment 3; 557 563 569 / 601-619 retired, refused by the guard). An r1review experiment (named by its write-once experiment.json, `pairfeat_compare.py init-experiment`,
 # over a pairfeat5 artifact-manifest.json) serves three arms: baseline, pairfeat5 (reference; exactly a pairfeat5 lane) and
 # r1review (the pairfeat5 chain plus -r1review), all on r1review's seeds. Every arm of an r1review experiment runs gen with
@@ -48,7 +49,7 @@ case "$MODE" in
   validation-clean) SPLIT=eval; RERR=0;    ERRU=0;;
   *) echo "invalid mode: $MODE" >&2; exit 2;;
 esac
-case "$ARM" in baseline|pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5|pairfeat6) ;; r1review) ;; *) echo "invalid arm: $ARM (allowed baseline pairfeat1 pairfeat2 pairfeat3 pairfeat4 pairfeat5 pairfeat6 r1review)" >&2; exit 2;; esac
+case "$ARM" in baseline|pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5|pairfeat6) ;; pairfeat7) ;; r1review) ;; *) echo "invalid arm: $ARM (allowed baseline pairfeat1 pairfeat2 pairfeat3 pairfeat4 pairfeat5 pairfeat6 pairfeat7 r1review)" >&2; exit 2;; esac
 [ -n "${HOME:-}" ] || { echo "HOME is not set" >&2; exit 2; }
 DATA_BASE="$HOME/wasl-ml-data"
 [ -d "$DATA_BASE" ] || { echo "missing $DATA_BASE" >&2; exit 2; }
@@ -79,6 +80,7 @@ case "$CAND" in
   pairfeat5) DEV_SEEDS="521 541 547";;
   r1review) DEV_SEEDS="571 577 587"; VAL_SEEDS="631 641 643 647 653"; CHAIN=pairfeat5; REF=pairfeat5;;
   pairfeat6) DEV_SEEDS="659 661 673"; VAL_SEEDS="677 683 691 701 709"; REF=pairfeat5; REFCHAIN=experiment;;
+  pairfeat7) DEV_SEEDS="719 727 733"; VAL_SEEDS="739 743 751 757 761"; REF=pairfeat5; REFCHAIN=experiment;;
   *) echo "preflight refused: unknown experiment candidate: $CAND" >&2; exit 2;;
 esac
 case "$ARM" in baseline|"$CAND") ;; *) [ -n "$REF" ] && [ "$ARM" = "$REF" ] || { echo "arm $ARM is not served by the $CAND experiment in $PARENT" >&2; exit 2; };; esac
```

- [ ] **Step 4: GREEN.** `python3 -I -m unittest discover -s scripts -p 'test_*.py'` (357 tests, including `ByteIdentity`, `R1reviewByteIdentity`, `Pairfeat6ByteIdentity` and every `pairfeat6` and r1review class) and `bash -n scripts/pairfeat_experiment.sh`.
- [ ] **Step 5: Commit.**

```bash
git commit -m "Harness: three-arm pairfeat7 candidate with two more development edge classes" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" \
  -- scripts/pairfeat_compare.py scripts/pairfeat_experiment.sh scripts/test_pairfeat_compare.py
git show --stat HEAD    # exactly the three files
```

### Task E: run once, diagnose, document

Run only after Tasks A-D pass their reviews and a final cross-task review. Shell setup for every step:

```bash
export PATH=/opt/anaconda3/bin:$PATH
H=~/wasl-ml-data/agents2/hardening
EXP=$H/pairfeat7/run-1
REF=$H/pairfeat5/pairfeat7-ref-run-1
```

- [ ] **E1: Full verification** at the final HEAD:

```bash
(cd pipeline && go vet . && go test . -count=1)
python3 -I encoder/test_fit_scorer.py
python3 -I encoder/test_fit_pairfeat1.py
python3 -I -m unittest discover -s scripts -p 'test_*.py'
bash -n scripts/pairfeat_experiment.sh
git status --porcelain --untracked-files=all -- pipeline encoder scripts .gitignore    # must print nothing
git diff --stat acefac5 HEAD -- pipeline/RESULTS.md docs/BACKEND-HANDOFF.md            # only the other session's pairfeat6 write-up, if any
df -h ~                                                                               # at least 25 GB free, else stop
```

- [ ] **E2: Move existing model files aside** (the fit refuses to overwrite them; the `pairfeat6` files stay, nothing reads them):

```bash
mkdir -p $H/pairfeat7/models-aside/pairfeat5 $H/pairfeat7/models-aside/pairfeat7
find models -maxdepth 1 -name '*_pairfeat7.json' -exec mv {} $H/pairfeat7/models-aside/pairfeat7/ \;
find models -maxdepth 1 -name '*_pairfeat5.json' -exec mv {} $H/pairfeat7/models-aside/pairfeat5/ \;
find models -maxdepth 1 \( -name '*_pairfeat5.json' -o -name '*_pairfeat7.json' \)    # must print nothing
```

- [ ] **E3: Refit the pairfeat5 reference** at this HEAD (output parent must be `hardening/pairfeat5/`):

```bash
python3 -I encoder/fit_pairfeat1.py $REF --modelset pairfeat5 > $H/pairfeat5/pairfeat7-ref-run-1.fit.log 2>&1; echo "exit $?"
python3 -I - "$REF/artifact-manifest.json" "$H/pairfeat5/run-2/artifact-manifest.json" <<'PY'
import json, sys
a, b = (json.load(open(p)) for p in sys.argv[1:3])
k = "pairfeat5_chain_sha256"
print("equal" if a[k] == b[k] else "DIFFERENT")
print(json.dumps({"refit": a[k], "run-2": b[k], "plain_cal_diff_at_merge_0_7": a["plain_cal_diff_at_merge_0_7"]}, indent=1, sort_keys=True))
sys.exit(0 if a[k] == b[k] else 1)
PY
```

  Expected `exit 0` and `equal`. If the chain differs from `pairfeat5/run-2`'s, stop and report (the spec requires equality).
- [ ] **E4: Fit pairfeat7** at the same HEAD into the experiment parent (about 15 minutes longer than a `pairfeat6` fit: the same-date donor search embeds the train pool in each of the five training dumps):

```bash
python3 -I encoder/fit_pairfeat1.py $EXP --modelset pairfeat7 --reference $REF > $H/pairfeat7/run-1.fit.log 2>&1; echo "exit $?"
grep -e 'plain cal different-at-merge' -e 'hard-negative slice' $H/pairfeat7/run-1.fit.log
python3 -I - "$EXP" "$REF" <<'PY'
import json, os, sys
exp, ref = sys.argv[1:3]
am = json.load(open(os.path.join(exp, "artifact-manifest.json")))
tm = json.load(open(os.path.join(exp, "training-manifest.json")))
fr = json.load(open(os.path.join(exp, "chain", "fit_report_pairfeat7.json")))
idf = json.load(open(os.path.join(exp, "chain", "name_idf_pairfeat7.json")))
r5 = json.load(open(os.path.join(ref, "artifact-manifest.json")))
print(json.dumps({"git_head": am["git_head"], "reference_git_head": r5["git_head"], "max_abs_leaf": am["max_abs_leaf"],
                  "parity": am["parity"], "name_idf_sha256": am["name_idf_sha256"],
                  "idf_registrations": idf["registrations"], "idf_tokens": len(idf["counts"]),
                  "floors": {k: tm[k + "namesake_base_rows_total"] for k in ("", "family_", "xs_", "md_", "rd_")},
                  "slices": {k: am[k + "namesake_slice"] for k in ("", "family_", "xs_", "md_", "rd_")},
                  "plain_cal_diff_at_merge_0_7": [am["plain_cal_diff_at_merge_0_7"], am["plain_cal_diff_at_merge_0_7_reference"]],
                  "unseen_token_share": am["unseen_token_share"],
                  "cal_auc": fr["cal_auc"], "cal_ap": fr["cal_ap"], "nfeat": fr["nfeat"],
                  "hard_negative_slices": fr["hard_negative_slices"]}, indent=1, sort_keys=True))
PY
```

  Stop on exit 3 (a namesake floor, including missing-date or same-date) or exit 4 (leaf check) and report. The two `git_head` values must be equal (the fit refuses otherwise). Report the printed values; the plain-calibration different-at-merge-0.7 count is shown beside the reference's (`pairfeat6` had 27 against `pairfeat5`'s 21) for information only.
- [ ] **E5: Name the experiment:**

```bash
python3 -I scripts/pairfeat_compare.py init-experiment $EXP pairfeat7 $REF
```

- [ ] **E6: One-lane smoke** (seed 101, dev split; not evidence, not a protected seed) with the exact harness argv:

```bash
S=$H/pairfeat7/smoke
mkdir -p $S/bin $S/src
git archive HEAD pipeline | tar -x -C $S/src
(cd $S/src/pipeline && go build -buildvcs=false -o $S/bin/wsi7 .)
ln -s "$(readlink bin/nats-server)" $S/bin/nats-server
cat > $S/smoke.py <<'PY'
import json, os, subprocess, sys
root, S = sys.argv[1], sys.argv[2]
sys.path.insert(0, os.path.join(root, "scripts"))
import pairfeat_compare as pfc
bin_ = os.path.join(S, "bin", "wsi7")
o = os.path.join(S, "v2-s101"); os.mkdir(o); os.mkdir(os.path.join(o, "run"))
for name, argv, log in pfc.lane_commands(root, bin_, o, 101, "dev-clean", "pairfeat7", candidate="pairfeat7"):
    with open(os.path.join(o, log), "wb") as h:
        p = subprocess.run(argv, stdout=h, stderr=subprocess.STDOUT)
    print(name, p.returncode, flush=True)
    if p.returncode:
        sys.exit(1)
lm = pfc._load_lane(o, 101, "pairfeat7", "dev-clean", "pairfeat7")
print(json.dumps({k: lm.get(k) for k in ("analysis_errors", "trace_error", "replay_mismatch", "false_closures", "recall",
                                         "false_merge_pairs", "cross_person_edges", "xscript_cap", "linked_pairs")}, indent=1))
PY
python3 -I $S/smoke.py "$PWD" $S > $S/smoke.log 2>&1; echo "exit $?"; cat $S/smoke.log
```

  Expected: every step exits 0; `analysis_errors`, `trace_error`, `replay_mismatch`, `false_closures` are 0; `linked_pairs.edge_classes` has the seven `EDGE_CLASSES7` keys. Stop and diagnose otherwise.
- [ ] **E7: Development, stress, validation**, stopping at the first failed gate. Check `df -h ~` (at least 25 GB free) before each mode. Run each mode as one background script (Bash `run_in_background`), arms sequentially:

```bash
cat > $EXP/dev.sh <<'SH'
#!/bin/bash
set -e
cd "$1"; export PATH=/opt/anaconda3/bin:$PATH; P="$2"
for a in baseline pairfeat5 pairfeat7; do
  scripts/pairfeat_experiment.sh $P/dev-$a dev-clean $a > $P/dev-$a.log 2>&1
  echo "arm $a done $(date)"
done
python3 -I scripts/pairfeat_compare.py dev-clean $P/dev-clean.json baseline=$P/dev-baseline pairfeat5=$P/dev-pairfeat5 pairfeat7=$P/dev-pairfeat7
SH
bash $EXP/dev.sh "$PWD" $EXP > $EXP/dev.log 2>&1
python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d["pass"], d["reference_gate"]["reasons"], d["gate"]["reasons"])' $EXP/dev-clean.json
```

  On pass, the same script shape for stress (roots `$EXP/stress-<arm>`, mode `dev-stress`, analysis `dev-stress $EXP/dev-stress.json`), then `python3 -I scripts/pairfeat_compare.py freeze-validation $EXP`, then validation (roots `$EXP/val-<arm>`, mode `validation-clean`, analysis `validation-clean $EXP/validation-clean.json`). SAFETY must be zero in every arm, baseline included (the gate checks the arm sums; report the counts of all three arms). A lane crash is set aside (`mv $EXP/<root> $EXP/<root>-crashed-1`), its cause recorded, and the arm re-run once; a second crash stops the run. Validation seeds are not inspected before `validation-manifest.json` exists.
- [ ] **E8: Read-only diagnosis** (scripts and outputs under `$EXP/diag/`, outside Git; no lane file is written). Write `$EXP/diag/diag7.py`:

```python
"""Read-only diagnosis of a pairfeat7 experiment (Task E8). Usage: diag7.py REPO_ROOT EXPERIMENT_DIR [CANDIDATE]
Prints JSON: per development lane (dev-clean and, if run, dev-stress) of the reference and candidate arms, the final
linked different-person pairs removed, remaining and new, the true merges lost and gained, and the CRC loops whose final
decision is closed, lost and gained; then, per analysis JSON present, the vs-baseline gates of both arms."""
import json
import os
import sys

root, exp = sys.argv[1], sys.argv[2]
cand = sys.argv[3] if len(sys.argv) > 3 else "pairfeat7"
sys.path.insert(0, os.path.join(root, "scripts"))
import pairfeat_compare as pfc  # noqa: E402

REF = pfc.reference(cand)
ANALYSES = (("dev-clean.json", "selection"), ("dev-stress.json", "stress"), ("validation-clean.json", "validation"))


def arabic(s):
    return any("\u0600" <= ch <= "\u06ff" for ch in s or "")


def max_p(lane):
    best = {}
    with open(os.path.join(lane, "run/main/link_funnel.ndjson")) as h:
        for ln in h:
            r = json.loads(ln)
            if r.get("kind") == "pair":
                k = tuple(sorted((r["a"], r["b"])))
                best[k] = max(best.get(k, 0.0), r.get("p") or 0.0)
    return best


def linked(lane):
    with open(os.path.join(lane, "cohort/truth.json")) as h:
        pk = json.load(h)["pk_qid"]
    with open(os.path.join(lane, "run/main/link_funnel.ndjson")) as h:
        pairs = {tuple(sorted(k)) for k in pfc.final_linked_pairs(h)}
    same = {k for k in pairs if pk.get(k[0]) is not None and pk.get(k[0]) == pk.get(k[1])}
    return same, pairs - same


def crc_closed(lane):
    last = {}
    with open(os.path.join(lane, "run/main/out_log.ndjson")) as h:
        for ln in h:
            d = json.loads(ln).get("data") or {}
            if d.get("kind") == "decision" and d["dec"].get("prog") == "crc":
                last[d["dec"]["loop"]] = d["dec"]["state"]
    return {k for k, v in last.items() if v == "closed"}


def describe(regs, k, pr, pc):
    ra, rb = regs.get(k[0]), regs.get(k[1])
    a, b = ra or {}, rb or {}
    return {"pair": list(k), "class": pfc.edge_class7(ra, rb), "p_" + REF: round(pr.get(k, 0.0), 3),
            "p_" + cand: round(pc.get(k, 0.0), 3), "dobs": [a.get("dob"), b.get("dob")],
            "script_mismatch": arabic(a.get("name")) != arabic(b.get("name")),
            "given_only": pfc._given_only(a.get("name") or "", b.get("name") or ""),
            "same_real_dob": bool(pfc._real_dob(a.get("dob")) and a.get("dob") == b.get("dob"))}


out = {"lanes": {}, "gates": {}}
for prefix in ("dev", "stress"):
    if not os.path.isdir(os.path.join(exp, "%s-%s" % (prefix, cand))):
        continue
    for s in pfc.CANDIDATES[cand]["dev_seeds"]:
        lr, lc = (os.path.join(exp, "%s-%s" % (prefix, a), "v2-s%d" % s) for a in (REF, cand))
        regs = dict(pfc.cohort_registrations(os.path.join(lr, "cohort")))
        regs.update(pfc.cohort_registrations(os.path.join(lc, "cohort")))
        pr, pc = max_p(lr), max_p(lc)
        (sr, dr), (sc, dc) = linked(lr), linked(lc)
        cr, cc = crc_closed(lr), crc_closed(lc)
        d = lambda ks: [describe(regs, k, pr, pc) for k in sorted(ks)]
        out["lanes"]["%s-s%d" % (prefix, s)] = {
            "different_person": {REF: len(dr), cand: len(dc), "removed": d(dr - dc), "remaining": d(dr & dc), "new": d(dc - dr)},
            "true_merges": {REF: len(sr), cand: len(sc), "lost": d(sr - sc), "gained": d(sc - sr)},
            "crc_closed": {REF: len(cr), cand: len(cc), "lost": sorted(cr - cc), "gained": sorted(cc - cr)}}
for name, kind in ANALYSES:
    p = os.path.join(exp, name)
    if os.path.exists(p):
        with open(p) as h:
            a = json.load(h)
        out["gates"][name] = {"baseline_vs_" + cand: a["gate"],
                              "baseline_vs_" + REF: pfc.gate(a["arms"]["baseline"], a["arms"][REF], kind)}
json.dump(out, sys.stdout, indent=1, sort_keys=True)
print()
```

  Run `python3 -I $EXP/diag/diag7.py "$PWD" $EXP > $EXP/diag/diag7.json` (planning ran it unchanged on `pairfeat6/run-1` with candidate `pairfeat6`: it reproduced the diagnosed 7 removed and 9 new pairs). Report from it and from the analysis JSONs:
  - per arm and seed, `linked_pairs` (same-person = true merges; edge classes on development, including the two new ones) and `xscript_cap` (pairs, same-person, different-person);
  - per development lane, the different-person pairs `pairfeat7` removed, kept and added vs `pairfeat5`, with class, highest `p` of each arm, both DOBs, script mismatch, given-only and same-real-date flags: whether the `missing_dob_given_only` and `same_real_dob_different_names` pairs fell, and what replaced them;
  - true merges lost and gained by `pairfeat7` vs `pairfeat5` (same attributes): whether the new negatives taught "one date missing, given name only" or "same real date, different names" as different-person on real duplicates;
  - CRC closures lost and gained vs `pairfeat5` (final decision per loop from `run/main/out_log.ndjson`);
  - the vs-baseline gate of `pairfeat7` and, for context, of `pairfeat5` (reported, not gating), per analysis present.
- [ ] **E9: Document** in `pipeline/RESULTS.md` (new section after the `pairfeat6` section, titled with the experiment HEAD and outcome) and `docs/BACKEND-HANDOFF.md` (a paragraph after the `pairfeat6` paragraph, same style). First confirm the other session's `pairfeat6` write-up is committed: `git status --porcelain -- pipeline/RESULTS.md docs/BACKEND-HANDOFF.md` prints nothing; otherwise wait. State built/simulated. Include: the change; seeds; the gates table verbatim; per-mode results (recall per arm, gain vs `pairfeat5` with the paired t interval and n, false-merge pairs, cross-person edges, impure clusters, uncapped wrong-pair reviews, reviewer hours, SAFETY counts in all arms); the vs-baseline gate (reported); the reported-only metrics (edge classes on development including the two new ones, true merges, capped pairs, unseen-token shares, the plain-calibration different-at-merge count beside `pairfeat5`'s, the three hard-negative slices, rarity SHA256 and table N); fit facts (max |leaf|, the five floors, parity); the reference refit's equality with `pairfeat5/run-2`; the smoke; every rerun and the stop point. Disclose:
  - the design came from a post-hoc read-only diagnosis of used development lanes (571 577 587 for `pairfeat6`, 659 661 673 for `pairfeat7`);
  - all hard negatives are built to patterns of our own synthetic generator, so a gain may partly reflect learning the generator rather than real naming;
  - this is the third scorer candidate in a row tuned on diagnosed failures, so the fresh validation seeds are the only unbiased evidence;
  - the same-date donor is the nearest-embedding stranger (resolution 7), a choice the spec did not make, forced by the row floor;
  - the spec resolutions that shape results (2, 3, 4, 5, 7, 11);
  - default model, noise and floor 0.2 unchanged; nothing promoted to default; no clinical effectiveness proven.

  Then get an independent doc review (a reviewer given the two diffs, the analysis JSONs, `diag7.json` and the spec, checking every number, the disclosures and the constraints: no `/Users` or `/tmp` path, no identifier). Fix, then commit those two files only:

```bash
grep -n -e '/Users' -e '/tmp' pipeline/RESULTS.md docs/BACKEND-HANDOFF.md      # no new hit
git add pipeline/RESULTS.md docs/BACKEND-HANDOFF.md
git commit -m "Document pairfeat7: missing-date and same-real-date hard negatives vs pairfeat5" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>" -- pipeline/RESULTS.md docs/BACKEND-HANDOFF.md
git show --stat HEAD                                                            # exactly the two files
```

- [ ] **E10: Thin non-record runs.** Keep the record run `$EXP` whole (lanes, CSVs, chain, manifests, analyses, diag). Remove raw data of everything else, keeping summaries, manifests, sidecars and logs:

```bash
rm -rf $H/pairfeat7/smoke/v2-s101 $H/pairfeat7/smoke/bin $H/pairfeat7/smoke/src
rm -rf $H/pairfeat7/taskB-identity/base-src
find $H/pairfeat7/taskB-identity -maxdepth 1 \( -name '*.csv' -o -name 'wsi7.*' \) -delete
find $REF -maxdepth 1 -name 'feat_*.csv' -delete
find $EXP -maxdepth 1 -type d -name '*-crashed-*' -exec rm -rf {} +
rm -rf $H/pairfeat7/plan-goldens $H/pairfeat7/plan-scratch
du -sh $H/pairfeat7 $REF
```

---

## Self-review (writing-plans)

**Spec coverage.**
- Everything about `pairfeat6` carries over (f30–f33, leave-own-out table, cross-script negatives at 0.10, `pairfeat5` settings, review cap): Task A (`usesNameIDF` drives `nameIDFFor`, `xscriptCapFor`, the policy), Task C (`PROTOCOLS["pairfeat7"]` = `pairfeat6`'s values plus two rates; `test_dump_argv` checks the argv equals `pairfeat6`'s plus the two flags).
- Missing-date namesakes at 0.10 (same sex, given name and no other skeleton token, no prefix or truncation, no date, base keeps its date, half same script): Task B (`givenOnly`, `buildMDNamesakes`, `hnRegister`; `TestGivenOnly` covers initials, truncations, skeleton prefixes, articles; `TestHNRegister` checks the script split, dates and sex; `mdPattern` in `TestPF7FeatDump`).
- Same-real-date strangers at 0.10 (same sex, same real non-placeholder date, no shared skeleton token, half same script): Task B (`noSharedSkeleton`, `buildRDNamesakes` refuses 1 January, 1 July, day 00, missing and invalid dates; `rdPattern`), resolution 7 for the donor.
- Different persons in truth, ordinary negatives, never in the rarity table, own sidecar counts and the 500-row floor: Task B (`Idx` 0, label-0 rows disjoint from other kinds, `md_*`/`rd_*` keys), Task C (`check_namesake_floor` with five kinds).
- Truncated or initial forms with a missing date not generated: `givenOnly` rule (b) and its tests; the harness keeps them in `missing_dob_ambiguous`.
- Arms baseline / `pairfeat5` refit at the same HEAD with the `run-2` chain / `pairfeat7`, image off, no `-r1review`: Task D (`Pairfeat7Guard`, `Pairfeat7Preflight`), Task E (E3 equality, E4 `--reference` HEAD check).
- Seeds and never-touch list including 659 661 673 and 677 683 691 701 709: Global Constraints, `Pairfeat7Candidate.test_seeds_never_touched_before`, `Pairfeat7Guard.test_refusals`.
- Gates unchanged from `pairfeat6`, SAFETY in every arm, vs-baseline reported: Task D (`Pairfeat7Gate.test_same_gate_as_pairfeat6`, `CLIPairfeat7`).
- Reported: `pairfeat6` edge classes plus `missing_dob_given_only` and `same_real_dob_different_names` on development, true merges, capped pairs, unseen-token shares, plain-cal count beside `pairfeat5`'s, three slice reports: Task D (`edge_class7`, `EdgeClass7`), Task C (`read_reference`, `hard_negative_slice`, printed in `main`), E4, E8.
- Constraints, disclosures, thinning: Global Constraints, E9, E10.

**Byte identity.** Go: `TestPairfeat6DumpsUnchangedGolden` (table, train and cal dumps at `acefac5`), the existing `pairfeat5`/namesake goldens, and Task B's six CLI hashes at 5,000 persons. Fit: `FitByteIdentity` (`pairfeat1`-`pairfeat6` tables). Harness: `Pairfeat6ByteIdentity` and the existing `R1reviewByteIdentity`, `ByteIdentity`. Every digest has a Step 0 command that re-derives it at BASE.

**Placeholder scan.** No "TBD"/"similar to Task N"; every new function has its full code; every golden and digest is a real value taken at `acefac5` during planning. All test and implementation code in Tasks A-D was applied to a clone at `acefac5` and passed there: `go test` (full suite), `go vet`, `gofmt -l`, `encoder/test_fit_pairfeat1.py` (75 tests), `encoder/test_fit_scorer.py`, `scripts` discover `test_*.py` (357 tests), `bash -n`; the CLI refusals were checked by hand. E8's script ran on `pairfeat6/run-1`. E9's figures are run results, not code.

**Type consistency.** `MDNamesakeRate`/`RDNamesakeRate` are the flag targets in `main.go`, read by `pairfeat7Negatives` and set by `pf7Dump`; `hnKind.key` (`md`, `rd`) prefixes the sidecar keys `<key>_rate`, `<key>_namesakes_added`, `<key>_base_namesake_rows`, `<key>_rows`, which `HARD_NEGATIVES7` reads in `check_namesake_floor`, `training_manifest` and `hard_negative_slice`, and `checkKind` reads in the Go test; `fdReg`'s six fields (`pid, name, dob, sex, eid, tid`) match `hnRegister` and `TestHNRegister`; `name_idf_pairfeat7.json` is the same name in `nameIDFFor` (`"name_idf_"+modelSet+".json"`), `chain_files`, `CHAIN_FILES`, `CAND7_FILES` and `.gitignore`'s glob; `plain_cal_diff_at_merge_0_7_reference` is written by `main` and checked by `test_artifact_manifest_and_models`; `edge_classifier(candidate)` returns `EDGE_CLASSES7`'s names, which `linked_pair_report`, `aggregate_arm` (lane keys) and `CLIPairfeat7` use; `linked_pair_report(lines, pk_qid, regs=None, candidate=None)` matches `_load_lane`'s call; `_pairfeat6_gate(ref, cand, kind, base)` keeps its signature.
