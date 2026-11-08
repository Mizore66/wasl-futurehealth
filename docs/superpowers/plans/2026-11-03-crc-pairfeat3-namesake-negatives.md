# CRC pairfeat3 Namesake Hard-Negative Experiment Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Test whether adding training-only namesake hard negatives to the `pairfeat2` recipe keeps its CRC recall gain while removing its extra false merges.

**Architecture:** Runtime, features, retrieval and safeguards are unchanged. `featdump` gains a training-only `-namesakes P` option that adds synthetic namesake persons (same given name, origin and sex as a real person, a different family name, a DOB one or two digits off, their own Emirates ID). A new opt-in modelset `pairfeat3` is fitted with the `pairfeat2` seeds and noise plus namesakes, and evaluated with the existing protected paired harness on fresh development seeds.

**Tech Stack:** Go 1.22+, Python 3 (numpy, scikit-learn, scipy, lightgbm 4.7.0), Bash.

**Spec:** `docs/superpowers/specs/2026-11-02-crc-pair-scorer-design.md` (binding for everything not changed here) plus the approved design below.

## Approved design (user-approved 3 Nov 2026)

- Motivation: `pairfeat2` (`pipeline/RESULTS.md`, decade-noise section) gained +4.38pp development recall but failed on contamination and workload. The read-only diagnosis found its 23 new cross-person edges were pairs with a missing, placeholder or one-to-two-digit-off DOB and an Emirates ID on one side only, which name similarity carried to the merge tier. On the cal split, 30 of `pairfeat2`'s 31 different-person pairs at raw score >= 0.7 had non-identical DOBs. The training data lacks different-person pairs of that shape, so the model never learned that DOB tolerance needs corroboration.
- User choice: retrain with hard negatives (not a runtime merge gate, not a new feature).
- Namesakes (training only, `featdump` only): for each sampled person, with probability **0.10** add one synthetic namesake. Its names (Arabic, English, aliases), sex, country code and origin come from a **real** person in the same split who shares the base person's first English token (case-insensitive), origin and sex and has a different last English token; if none exists, no namesake. Its DOB is the base person's true DOB with **one digit (probability 0.60) or two distinct digits (0.40)** of YYYYMMDD replaced by different digits; the result must be a valid date in 1900-2025 and differ from the base DOB, redrawn up to 20 times, else no namesake. Its Emirates ID is new and Luhn-valid. It is registered exactly like other persons (same registration-count draw, providers, name noise, DOB noise, EID capture) using its own seeded stream; `-namesakes 0` (the default) leaves `featdump` output byte-identical. The 0.10 rate is an assumption; no public namesake rate is used.
- Seeds: train **1019 1021 1031 1033 1039** (5,000 persons each, namesakes 0.10), cal **2011** (whole cal split), all as in `pairfeat2`, so namesakes are the only training change. Calibrators and parity use the plain cal dump (no namesakes), because deployment cohorts have none. A second cal dump with namesakes 0.10 is scored with the fitted model for a namesake slice report only.
- Stop check before development: if the five train dumps hold fewer than **500** base-namesake candidate rows in total, stop and report. The plain-cal count of different-person rows at raw >= 0.7 is reported next to `pairfeat2`'s 31 for information only.
- Evaluation: fresh development dev/v2 seeds **347 353 367**; validation eval/v2 **401 409 419 421 431** (still untouched). Gates, LightGBM parameters, calibrators, floor 0.01 in both arms, v7strict only, write-once decisions, same-HEAD binding and stop-on-failure rules are exactly those of the `pairfeat1` and `pairfeat2` plans, including the distinct link-review pair limit of 1.10x baseline (user kept this gate). Default model, default noise and default floor 0.2 unchanged.

## Global Constraints

- All global constraints of `docs/superpowers/plans/2026-11-02-crc-pair-scorer.md` and `docs/superpowers/plans/2026-11-03-crc-pairfeat2-decade-noise.md` apply, with seeds replaced by the seeds above for `pairfeat3`.
- `-namesakes` must be 0 for every command except `featdump`, and in [0, 1) for `featdump` (configuration error, exit 2). `gen` output and `featdump` output with `-namesakes 0` must be byte-identical to `9bf8017`/`5e59c0b` behaviour.
- `pairfeat1` and `pairfeat2` harness behaviour, seeds and tests must keep working unchanged.
- Do not tune on development seeds 307 311 313 317 331 337, validation 401 409 419 421 431, or B8/frozen seeds 11 23 37 41 59 17 29. Dev seeds 317 331 337 were inspected for the diagnosis and are not reused.
- Raw artifacts under `~/wasl-ml-data/agents2/hardening/pairfeat3/`. No `/tmp` for important artifacts. No `/Users` paths, patient-level identifiers, secrets, binaries or models in Git. Commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`. No push or merge.
- Python tests run by direct path (`python3 -I encoder/test_x.py`, `python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'`); Go tests from `pipeline/`. `PATH=/opt/anaconda3/bin:$PATH`.

---

### Task A: Training-only namesakes in `featdump` and `pairfeat3` policy guard (Go)

**Files:** Create `pipeline/namesake.go`, `pipeline/namesake_test.go`; modify `pipeline/encdev.go` (runFeatDump, validateModelPolicy), `pipeline/main.go` (`-namesakes` flag, validation, featdump call).

**Interfaces produced:**
- `var NamesakeRate float64` (flag `-namesakes`, default 0).
- `func validateNamesakes(cmd string, rate float64) error` — error if `rate < 0 || rate >= 1`, or `rate > 0 && cmd != "featdump"`.
- `func namesakeSeed(seed int64) int64 { return seed*2654435761 + 104729 }`.
- `func buildNamesakes(ps, pool []Person, rate float64, r *rand.Rand) (ns []Person, base []int)` — `ns[i]` is the namesake of `ps[base[i]]`; `pool` is every person of the same split with non-empty `Ar` and `En`, in file order.
- `func namesakeDob(dob string, r *rand.Rand) (string, bool)` and `func newEID(dob string, r *rand.Rand) string`.
- `runFeatDump` writes a sidecar `<out>.namesakes.json`: `{"rate": float, "seed": int, "namesakes_added": int, "base_namesake_rows": int, "rows": [int, ...]}` (0-based CSV row indices whose pair is a base person and its own namesake). Written for every `featdump` run, including rate 0 (empty `rows`).
- `validateModelPolicy` rejects `pairfeat3` with any policy other than `v7strict`.

- [ ] **Step 1: Record the byte-identity golden at BASE.** Before any change, build `bin/wsi7` and run `featdump -split train -persons 400 -noise v1t -seed 1019` and `-split cal -noise v1t -seed 2011 -persons 400` into `~/wasl-ml-data/agents2/hardening/pairfeat3/golden/`; record both SHA-256 values in the report and as constants in `namesake_test.go`.
- [ ] **Step 2: Failing tests** in `pipeline/namesake_test.go`:
  (a) `validateNamesakes`: `("featdump", 0)`, `("featdump", 0.1)`, `("gen", 0)` pass; `("gen", 0.1)`, `("orchestrate", 0.1)`, `("featdump", -0.1)`, `("featdump", 1)` fail.
  (b) `namesakeDob` over 20,000 draws on `"1976-03-08"` and `"1970-01-01"`: every output is a valid date in 1900-2025, differs from the input in 1 or 2 YYYYMMDD positions with dashes in place; share with 1 differing digit is 0.60 ± 0.02.
  (c) `newEID`: 15 digits, starts with `"784"` + the DOB year, `luhnOK` true, 1,000 draws give at least 990 distinct values.
  (d) `buildNamesakes` on a hand-built pool (`Mohamed Ali` M arab, `Mohamed Hassan` M arab, `Mohamed Saleh` F arab, `Mohamed Ali` M arab duplicate surname, `Anand Kumar` M south_asian) with rate 0.999999: the namesake of the first person always takes names from `Mohamed Hassan` (same first token, origin, sex; different last token), never from the same-surname, other-sex or other-origin entries; a person with no eligible donor gets none; `Qid` is `"ns-" + base.Qid`, `Spl` is the base split.
  (e) `buildNamesakes` with rate 0 returns no namesakes and makes no draws from `r` (compare `r.Int63()` after the call against a fresh stream).
  (f) `featdump` with `-namesakes 0` reproduces both Step 1 goldens byte for byte, and its sidecar has `namesakes_added` 0 and empty `rows`.
  (g) `featdump -split train -persons 400 -noise v1t -seed 1019 -namesakes 0.1` run twice is byte-identical (CSV and sidecar); the sidecar has `namesakes_added` > 0 and `base_namesake_rows` >= 1; every listed row has label 0. (If 400 persons is too few for (g), use the smallest size that reliably works and state it.)
  (h) `validateModelPolicy("pairfeat3", p)` errors for `""`, `v6`, `v7ltt` and passes `v7strict`; `pairfeat1` and `pairfeat2` behaviour unchanged.
- [ ] **Step 3: Verify RED** (`cd pipeline && go test . -run 'Namesake|PairFeat|FeatDump|ModelPolicy' -count=1`).
- [ ] **Step 4: Implement.**

```go
// namesake.go
var NamesakeRate float64 // flag -namesakes; training-only (featdump)

func validateNamesakes(cmd string, rate float64) error {
	if rate < 0 || rate >= 1 {
		return fmt.Errorf("-namesakes must be in [0, 1) (got %v)", rate)
	}
	if rate > 0 && cmd != "featdump" {
		return fmt.Errorf("-namesakes is training-only and valid only for featdump (got command %q)", cmd)
	}
	return nil
}

func namesakeSeed(seed int64) int64 { return seed*2654435761 + 104729 }

var dobPos = []int{0, 1, 2, 3, 5, 6, 8, 9}

func namesakeDob(dob string, r *rand.Rand) (string, bool) {
	for try := 0; try < 20; try++ {
		n := 1
		if r.Float64() >= 0.60 {
			n = 2
		}
		b := []byte(dob)
		for _, k := range r.Perm(8)[:n] {
			i := dobPos[k]
			b[i] = byte('0' + (int(b[i]-'0')+1+r.Intn(9))%10)
		}
		s := string(b)
		if s != dob && realDob(s) && s[:4] >= "1900" && s[:4] <= "2025" {
			return s, true
		}
	}
	return "", false
}

func newEID(dob string, r *rand.Rand) string {
	p := "784" + dob[:4]
	for len(p) < 14 {
		p += string(byte('0' + r.Intn(10)))
	}
	return p + string(luhnDigit(p))
}

func firstLastEn(en string) (string, string) {
	t := strings.Fields(strings.ToLower(en))
	if len(t) == 0 {
		return "", ""
	}
	return t[0], t[len(t)-1]
}

func buildNamesakes(ps, pool []Person, rate float64, r *rand.Rand) ([]Person, []int) {
	if rate <= 0 {
		return nil, nil
	}
	byFirst := map[string][]int{}
	for i := range pool {
		f, _ := firstLastEn(pool[i].En)
		byFirst[f] = append(byFirst[f], i)
	}
	var ns []Person
	var base []int
	for bi := range ps {
		b := &ps[bi]
		if r.Float64() >= rate {
			continue
		}
		f, l := firstLastEn(b.En)
		var elig []int
		for _, j := range byFirst[f] {
			d := &pool[j]
			_, dl := firstLastEn(d.En)
			if d.Qid != b.Qid && d.Origin == b.Origin && d.Sex == b.Sex && dl != l {
				elig = append(elig, j)
			}
		}
		if len(elig) == 0 {
			continue
		}
		d := pool[elig[r.Intn(len(elig))]]
		dob, ok := namesakeDob(b.Dob, r)
		if !ok {
			continue
		}
		ns = append(ns, Person{Qid: "ns-" + b.Qid, Ar: d.Ar, En: d.En, Als: d.Als, Dob: dob, Sex: d.Sex,
			Cc: d.Cc, Eid: newEID(dob, r), Spl: b.Spl, Origin: d.Origin, ArSrc: d.ArSrc})
		base = append(base, bi)
	}
	return ns, base
}
```

  In `runFeatDump`: extract the existing per-person registration loop body into a closure `register(pid int, p *Person, r *rand.Rand)` with no change to draw order, and call it with the main stream `r` for `ps` exactly as today. Then, only if `NamesakeRate > 0`: `nr := rand.New(rand.NewSource(namesakeSeed(seed)))`; `pool` = all persons of `split` with non-empty `Ar` and `En` in file order (before the shuffle); `ns, base := buildNamesakes(ps, pool, NamesakeRate, nr)`; register `ns[i]` with pid `len(ps)+i` using `nr`, recording `nsBase[len(ps)+i] = base[i]`. The existing `r.Shuffle(len(regs), ...)` follows unchanged. When writing each candidate row, if `{pids[c.j], g.pid}` is `{p, nsBase[p]}` for some namesake pid `p`, append the row index to `rows`. After the loop write the sidecar JSON next to `-out` (`<out>.namesakes.json`, sorted keys, trailing newline). In `main.go`: register `fs.Float64Var(&NamesakeRate, "namesakes", 0, "featdump only: share of persons given a synthetic namesake hard negative (training only)")`, call `validateNamesakes(cmd, NamesakeRate)` right after `validateNoise`, exit 2 with `configuration error:`. Extend the `validateModelPolicy` condition to `pairfeat3`.
- [ ] **Step 5: Verify GREEN**: `go test . -count=1`, `go vet .`, and confirm Step 1 goldens still match (test f). Confirm `gen` v1 and v2 output on a small fixed cohort is byte-identical before and after (state how).
- [ ] **Step 6: Commit** only the listed Go files.

### Task B: `pairfeat3` protocol in the training driver (Python)

**Files:** Modify `encoder/fit_pairfeat1.py`, `encoder/test_fit_pairfeat1.py`, `.gitignore`.

**Interfaces:**
- Consumes: `featdump -namesakes P` and its sidecar `<out>.namesakes.json` (Task A).
- Produces: `PROTOCOLS["pairfeat3"] = {"train_seeds": [1019, 1021, 1031, 1033, 1039], "cal_seed": 2011, "noise": "v1t", "namesakes": 0.10}`; existing protocols gain `"namesakes": 0.0`. `NAMESAKE_FLOOR = 500`. CLI `--modelset pairfeat1|pairfeat2|pairfeat3`. Chain files `encoder_pairfeat3.json`, `lgb_stream_pairfeat3.json`, `iso_pairfeat3.json`, `iso_v7_pairfeat3.json`, `cal_p_pairfeat3.npy`. Output parent must be `~/wasl-ml-data/agents2/hardening/pairfeat3/`.

- [ ] **Step 1: Failing tests:** protocol constants for all three modelsets; `pairfeat1` and `pairfeat2` featdump argv are unchanged (no `-namesakes` argument at all); `pairfeat3` train argv carries `-namesakes 0.1` and its plain cal and parity argv carry none; the diagnostic cal argv `feat_cal_2011_ns.csv` carries `-namesakes 0.1` and `-gbmfile <chain>/lgb_stream_pairfeat3.json`; `check_namesake_floor(sidecars)` raises `NamesakeFloorError` below 500 total `base_namesake_rows` and passes at 500; `namesake_slice(csv, sidecar)` returns `{"rows", "different_person", "different_at_review_0_01", "different_at_merge_0_7"}` from the listed rows using the Go `p` column; training manifest for `pairfeat3` records `modelset`, `noise`, `namesakes`, and per-train-seed `{namesakes_added, base_namesake_rows, sidecar_sha256}`; artifact-manifest keys are those of `pairfeat2` plus `namesake_slice` and `plain_cal_diff_at_merge_0_7`; refusal to overwrite existing `models/*_pairfeat3.json`.
- [ ] **Step 2: Verify RED** (`python3 -I encoder/test_fit_pairfeat1.py`).
- [ ] **Step 3: Implement.** Order for `pairfeat3`: train dumps with namesakes → `check_namesake_floor` over the five train sidecars (exit 3 with `namesake floor not met: N < 500`, before any fit; record N in the training manifest) → plain cal dump → fit (`fit_scorer.py` unchanged) → parity dump and checks (unchanged) → diagnostic namesake cal dump scored with the fitted model → `namesake_slice` → `plain_cal_diff_at_merge_0_7` from the parity CSV (label 0 and Go p >= 0.7) → manifests → copy chain. `pairfeat1` and `pairfeat2` code paths stay byte-identical. Add `models/*_pairfeat3.json` to `.gitignore`.
- [ ] **Step 4: Verify GREEN** (`python3 -I encoder/test_fit_pairfeat1.py`, `python3 -I encoder/test_fit_scorer.py`).
- [ ] **Step 5: Commit** only these files.

### Task C: Add `pairfeat3` to the protected harness (Python/Bash)

**Files:** Modify `scripts/pairfeat_experiment.sh`, `scripts/pairfeat_compare.py`, `scripts/test_pairfeat_compare.py`.

**Interface:** `ARM` ∈ `baseline | pairfeat1 | pairfeat2 | pairfeat3`. `CANDIDATES["pairfeat3"] = {"dev_seeds": [347, 353, 367]}`; validation seeds `[401, 409, 419, 421, 431]`. Candidate inference, funnel `config.modelset` check, gates, write-once rules, frozen-validation checks and same-HEAD binding unchanged.

- [ ] **Step 1: Failing tests:** every existing test passes unmodified; new tests for `pairfeat3` seeds, inferred candidate from a `pairfeat3_chain_sha256` key, refusal of mixed `pairfeat2`+`pairfeat3` keys, guard argv (`-modelset pairfeat3` only on orchestrate/audit/report7), funnel modelset check, `bash -n`.
- [ ] **Step 2: Verify RED**, **Step 3: implement**, **Step 4: GREEN** (`python3 -I -m unittest discover -s scripts -p 'test_*_compare.py'`, `bash -n scripts/pairfeat_experiment.sh`).
- [ ] **Step 5: Commit** only these files.

### Task D: Run once and document

- [ ] After Tasks A-C pass review and a final cross-task review: full verification (`go test . -count=1`, `go vet .`, all Python tests); build `bin/wsi7` at HEAD; `python3 -I encoder/fit_pairfeat1.py ~/wasl-ml-data/agents2/hardening/pairfeat3/run-1 --modelset pairfeat3`.
- [ ] If the namesake floor fails, stop and report. Otherwise inspect the fit report, slices, namesake slice and `plain_cal_diff_at_merge_0_7` (report beside `pairfeat2`'s 31; information only).
- [ ] Run paired dev-clean (baseline, pairfeat3) on seeds 347 353 367 and gate; on pass, stress, then freeze validation, then validation; stop at the first failed gate.
- [ ] Document aggregate results and every ruling in `pipeline/RESULTS.md` (new section after `pairfeat2`) and a pointer in `docs/BACKEND-HANDOFF.md`; independent doc review before commit; commit only those two files.
