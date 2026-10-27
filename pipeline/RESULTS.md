# v7-merged: one pipeline, one integrated run

> **Stage 1 (27 Oct):** the canonical pipeline now runs the **v7strict** gate by default (`-policy v7strict`). The table below is the v6 legacy baseline. Current numbers: `results/v7strict/summary.txt`. Comparison: the "Stage 1: v6 vs v7strict" section at the end.

**Merge:**
- git 3-way merge in `pipeline/`: v5 base → gov, impact, ann, fhir. Forks untouched.
- Conflicts hand-fixed (main/services/linker). 9/9 `go test` pass.
- Integration bugs fixed:
  - `-ann` was not passed to child processes.
  - No async-publish ack timeout: attempt 1 hung after the NATS kill (`run/main_attempt1_stalled`).
  - The tamper verifier didn't load the parse cache, causing a false alarm.

**The run** (`run/main`, seed 2026):
- 1,500 CRC + 600 cervical loops, 84 opt-outs, 24 forged events.
- Mock Malaffi → FHIR Subscription; HNSW blocking.
- Reviewer: 4% error, double review on merges and "adequate", dual-reader merges.
- RBAC/consent, e-consult/teach-back, anchors.
- Chaos: kills of FHIR consumer, linker, closure-0+prep-0, NATS node n2. Feed: 18 drops, 26 duplicates, 36 reorders.
- **0 new LLM calls** (1,680 replies parsed from Gemma cache).

## Closure (same log for every tracker)

| | CRC P / R / false closures | Cervical P / R / false closures |
|---|---|---|
| **Wasl, live + review** | **1.000 / 0.960 / 0 of 570** (95% UB 0.67%) | **1.000 / 0.991 / 0 of 217** (95% UB 1.7%) |
| Wasl, no review | 1.000 / 0.242 / 0 | 1.000 / 0.785 / 0 |
| EID-only rule | 0.834 / 0.364 / 43 | 0.848 / 0.384 / 15 |
| Any procedure ≤120 d | 0.661 / 0.431 / 131 | 0.604 / 0.466 / 67 |

## Same run, other metrics

- **Reviewer-h/1k loops:** **15.6** (CRC 19.0, cervical 7.2). 814 items, 1,317 reads.
  - 25 wrong final sign-offs: 11 true links answered "different", 14 evidence misreads.
  - None wrongly said "adequate", so errors cost recall, not closures.
- **Review items/100 loops:** Arab 39.8, South Asian 34.9, Filipino 34.6, Western 43.5.
- **Wrong-patient:** 6 intents (28.6/10k) without the gate; **0 released** (UB 18/10k). All 6 held below τ.
- **Forged:** 24/24 denied, 0 effect.
- **Opt-outs:**
  - 0 sent after opt-out was on file; 11 suppressed.
  - 16 sent before the Consent arrived (cohort records it 0–0.5 d after referral).
- **Exactly-once:** 0 duplicate/lost of 15,769 outputs; 28,613/28,613 resources ingested once.
- **Live tamper:** **17/17** detected, 0 false alarms. The insider rewrite is caught only by external anchors.
- **Throughput:** **699 ev/s** (29,432 msgs, 42.1 s).
- **Drop-off:** loop accuracy 96.6% CRC, 98.8% cervical.
  - Top CRC: no acknowledgement 245, not booked 187.
- **E-consult / teach-back state:** 100% / 99.9%. **HNSW vs exact:** byte-identical.
- **Cervical colposcopy ≤3 mo (HSIL+/ASC-H):** 52.6% measured vs 55.3% true (n=38; target 80%).

## KPI honesty: CRC "colonoscopy ≤31 days, >90%"

**Spec (crc.txt):**
- §5.5.4.2: "within 31 days **of referral**".
- App. 2: "between **referral** after positive screening FIT test and conducting a follow-up colonoscopy".
- FIT+→referral is a separate indicator (95%).

**Denominator:** referred FIT+ patients, clock from referral. Not all FIT+, not only accepted. Counting non-attenders is not stated, so both readings shown. Accepted-only changes results ≤1.2 points.

| n=1,318 eligible referrals | All referrals | Colonoscopy seen only |
|---|---|---|
| Truth | 33.0% | 63.7% |
| **EID-only** | **12.4%** | 65.5% |
| **Wasl live** | **31.9%** | 63.5% |

- EID-only misses 434/683 colonoscopies and under-reports by **20.6 points**.
- Wasl: 1.1 points off, 98.9% per-loop agreement.
- This is measurement accuracy only: the real ~33% stays far below 90%.

## Run

```
./run.sh build && ./run.sh test
./run.sh main && ./run.sh tamper && ./run.sh report   # ~5 min
```

- Output: `results/report7.json`, `results/tamper_main.json`.
- Ports: NATS 14822–4/16822–4/18822–4, Malaffi 28880/28890. Kills own PIDs only.

**Limits:** simulated patients/reviewer, independent errors, one seed.


## Stage 1: v6 vs v7strict (same cohort and seed, full integrated NATS run)

`scripts/demo.sh` (`WASL_POLICY=v7strict` is the default; `v6` is kept for comparison). Every decision carries its `policy` field, and the signature covers it.

| Metric | v6 (legacy) | **v7strict (canonical)** |
|---|---|---|
| CRC with review: P / R / false closures | 1.000 / 0.960 / 0/570 | **1.000 / 0.961 / 0/571** |
| CRC no review: R | 0.242 | 0.232 |
| Cervical with review: false closures | 0/217 | 0/217 |
| Cervical no review: R | 0.785 | 0.744 |
| CRC ≤31-day KPI (Wasl vs truth 0.330) | 0.319 | 0.320 |
| Wrong-patient messages released | 0 | 0 |
| Messages released before a late opt-out was recorded | 16–17 | **5** (the DOB guard holds more; the consent hold removes the rest in stage 3) |
| Reviewer-h per 1k loops | 15.7 | 21.5 (strict prep plus guard items) |
| Exactly-once / tamper / forged | 0 dup · 17/17 · 0/24 | 0 dup · 17/17 · 0/24 |

**v7strict means:**
- **Stratified calibrator** (`models/iso_v7.json`): links not corroborated by DOB, sex and surname are capped below the 0.99 trust tier.
- **Closure link tier** τ = 0.98.
- **Second-identifier guard:** messages on EID-chain or model-only links need an exact non-placeholder DOB match or human confirmation.
- **Per-field two-reader consensus.**
- **Strict prep:** an ungraded prep never closes.

**Tests:** `v7_test.go` covers the strict prep hold, per-field consensus, the DOB guard, the calibrator cap, and policy in the signature.

*Simulated cohort, one seed. The final 5-seed frozen evaluation is stage 7.*


## Stage 2: live local reader (Gemma 3 4B), integrated

`WASL_READER=live scripts/demo.sh` needs a local server: `llama-server -m gemma-3-4b-it-Q4_K_M.gguf --port 8791 -c 8192 -ngl 99 -np 4`.

**How it works:**
- **Reader client:** `pipeline/reader.go`, a Go client for the OpenAI-compatible endpoint. It has a health check, timeout, bounded retries, JSON-schema output, a singleflight per report and a sha256-keyed ledger.
- **Two calls per report (the hybrid):**
  1. a full-context schema call for caecum, prep and polyps;
  2. a dedicated histology question with a verbatim quote, a diagnosis-lexicon check and a visual-impression guard.
- **Grounding:** a positive claim without a grounded quote is downgraded to its conservative value, so an ungrounded answer can only make closure *less* likely.
- **Fail-safe:** a timeout, HTTP error, schema violation or unhealthy server gives "reader unavailable". The two readers then cannot agree, so the case goes to human review. It never closes automatically (`reader_test.go`).
- **Replay determinism:** live answers go to `<run>/reader_ledger.jsonl`. Audit, tamper and report replay the ledger instead of calling the model. Warm-up (`wsi7 readerwarm`) asks each unique report body once before the stream starts.
- **Provenance:** every evidence block records `reader_model`, the per-field values from both readers, the rule and any documentation flags.

**4B reader alone on the 43 MTSamples reports** (answer key LLM-labelled and cross-checked):
- histology field correct on 43/43;
- state agrees with the cached 26B on 35/43;
- 1 report the 4B alone would call "adequate" is actually inadequate. The regex reader disagrees there ("incomplete exam"), so the two-reader gate holds it.
- Median 8.9 s per report on an M-series laptop, 2 calls.

**Integrated runs** (same cohort and seed, v7strict):

| Metric | Cached 26B answers | **Live local 4B** |
|---|---|---|
| CRC with review: P / R / false closures | 1.000 / 0.961 / 0/571 | **1.000 / 0.955 / 0/567** |
| CRC no review: R | 0.232 | 0.123 (the 4B is more conservative, so more cases go to review) |
| Cervical with review | 0/217 | 0/217 |
| Reviewer-h per 1k loops | 21.3 | 25.8 |
| Exactly-once / tamper / forged | 0 dup · 17/17 · 0/24 | 0 dup · 17/17 · 0/24 |

The live ledger for this run is committed (`results/v7strict-live/reader_ledger.jsonl`), so the live result can be replayed without the model: `-reader replay`.


## Stage 3: consent hold and outreach lifecycle

**What changed:**
- **Contact permission** per patient: `unknown | granted | denied | opted_out`, built by the linker from authorised Consent records (`screening-outreach-messaging`).
  - Idempotent: a duplicate record is a no-op.
  - A stale `permit` never overrides a later withdrawal.
- **Outreach gate** (`BuildOutreach`) and **send-time check** (`DispatchC`):
  - a message is released only if permission is `granted` at that point;
  - `unknown` is **held** (`consent_unknown_hold`) and re-evaluated when consent arrives;
  - `denied` / `opted_out` is **cancelled**.
- **Audit trail:** every hold, release or cancel transition is appended to the decision as `consent_log` (signed with the decision; history is never overwritten).
- **Metric** (`report7.json`: `consent_timing.released_without_granted_consent`): it replays the same state machine from the FHIR log and counts any release that lacked a prior granted permission.
- **Cohort:** every referral patient now has a contact-permission record arriving 0–0.5 days after referral. 4% deny, 5% of permits are delivered twice, and 2% withdraw later.
- **Tests** (`consent_test.go`):
  - referral before consent (hold, then release on grant);
  - consent before referral;
  - opt-out after queueing (cancel);
  - withdrawal after grant, and a stale permit afterwards;
  - duplicate consent;
  - replay determinism;
  - no dispatch without a grant.

**Integrated run** (v7strict, cached reader, regenerated cohort):

| Metric | Before (stage 2) | **Stage 3** |
|---|---|---|
| Messages released before a late opt-out was recorded | 5 | **0** |
| Releases without granted consent | not measured | **0 / 619** |
| Intents held for consent, then released / cancelled | n/a | 373 held, 216 released after grant, 12 cancelled |
| CRC with review: P / R / false closures | 1.000 / 0.961 / 0/571 | 1.000 / 0.961 / 0/571 |
| Tamper / forged / duplicates | 17/17 · 0/24 · 0 | 17/17 · 0/24 · 0 |


## Stage 4: HL7 v2 ingest over MLLP (production-shaped)

`WASL_SOURCE=hl7 scripts/demo.sh`. Malaffi's public interface is HL7 v2, so this is the production tier; FHIR remains the mock tier.

**How it works (`pipeline/hl7.go`):**
- **Mock HIE sender (`hl7send`):** encodes each cohort resource as HL7 v2.5.1 and sends it over MLLP: ADT^A04 (encounter), ORM^O01 (referral and task), ORU^R01 (colonoscopy, pathology and colposcopy result), SIU^S12 (appointment) and MDM^T02 (communication, consent, sign-off).
  - Faults: duplicate re-sends, adjacent reorders, mid-stream TCP drops (resumed from the last ACK with an at-least-once re-send).
- **Listener (`hl7mllp`):** parses MSH, PID, ORC, OBR, OBX, SCH, PV1 and TXA, and maps them to the **same internal event schema** as the FHIR path. There is no parallel closure path.
  - Patient identity (MRN, Emirates ID, name, DOB, sex) comes from PID; report text and coded conclusions come from OBX.
  - Wasl-specific fields with no standard v2 home travel in a Z-segment (ZWS).
  - It publishes with `Nats-Msg-Id = <prov>/<Type>/<id>/v<vid>`, so JetStream dedupe absorbs duplicates and replays. It returns `MSA|AA` only after JetStream acknowledges the publish.
  - Provenance headers: raw-message sha256, source system (MSH-4), control id (MSH-10).
- **Tests (`hl7_test.go`):**
  - 4,395 cohort resources of every type round-trip FHIR → HL7 → event identically to the FHIR path;
  - escaping (`| ^ & ~ \`) and Arabic names survive;
  - MLLP framing.

**Integrated run:** HL7 source, v7strict, cached reader, same cohort and seed. Chaos: kill the linker, kill closure plus prep, kill the MLLP listener, kill a NATS node, 2 TCP drops, 3% duplicates, 3% reorders.

| Metric | FHIR mock source | **HL7 v2 MLLP source** |
|---|---|---|
| Resources ingested exactly once | 30,668 / 30,668 | **30,668 / 30,668** (0 lost, 0 duplicates; 888 duplicate re-sends absorbed) |
| CRC with review: P / R / false closures | 1.000 / 0.961 / 0/571 | **1.000 / 0.966 / 0/574** |
| Cervical with review | 0/217 | 0/217 |
| Duplicate / lost outputs; final-state mismatches | 0 / 0; 0 | **0 / 0; 0** |
| Releases without granted consent | 0 / 619 | **0 / 1,082** (independently re-verified) |
| Tamper / forged | 17/17 · 0/24 | 17/17 · 0/24 |

The HL7 feed is delivered in arrival order (the FHIR mock adds heavy reordering), so consent records arrive before the facts that need them. That is why no intents were held for consent in this run, and why more referral-side facts reach the gate.


## Stage 5: API for the frontend

`scripts/demo.sh serve` starts the API (`wsi7 serve`) over the materialised run. **The contract is in `docs/API.md`; static responses are in `docs/api-samples/`.**

**Endpoints:**
- `/healthz`, `/api/dashboard`, `/api/fixtures`
- `/api/cases` (filters and paging), `/api/cases/{id}`, `/api/cases/{id}/audit`
- `POST /api/cases/{id}/review` (RBAC, append-only, hash-chained review ledger)

**Data:** everything comes from the run's logs. `meta.data_mode = simulated`, plus the policy and reader model actually used.

**Pipeline additions for the frontend:**
- **`hold_decision` provenance block:** records why an inadequate or awaiting-histology loop was not closed (used for the false-closure trap).
- **Decisions carry `contact`:** the consent status at that point.

**The three demo fixtures,** picked deterministically from the run:
- `evidence_closure`: a cross-provider link plus agreeing readers plus granted consent, closed, chain verified.
- `false_closure_trap`: inadequate evidence, held.
- `review_hold`: signed double review of the link and evidence, outreach held for consent, then closed.

**Tests (`api_test.go`):**
- fixture contracts;
- filters and paging;
- 404 for an unknown case;
- RBAC: an auditor or coordinator cannot sign, the actor is required, unknown items are rejected;
- review actions append a hash-chained ledger and never mutate the decision log;
- no `null` lists.

## Stage 7: frozen final evaluation (5 seeds × 2 noise models)

**Frozen setup.** `scripts/demo.sh final` won't run on uncommitted code. Before any run, it writes [`results/final/manifest.json`](results/final/manifest.json), which records:
- the code revision (`45ed3a0`);
- the policy, `v7strict`;
- every threshold;
- sha256 of each model and data file;
- the seed list (11, 23, 37, 41, 59).

**The runs.** Every run is a full integrated NATS run with chaos testing. All of them use the held-out Wikidata eval split of persons, under two noise models:
- **v1:** the corruption model the encoder, calibrator and v7 rules were developed on.
- **v2** ([`noise2.go`](noise2.go)): an independent model never used for training, calibration or rule design. It adds:
  - phonetic respellings, dropped particles, swapped name order and initials;
  - OCR confusions;
  - Arabic hamza, ya and ta-marbuta variants;
  - 15% missing DOB, plus DOB transpositions and shifts.

**Statistics.** Each mean comes with a 95% t-interval over the 5 seeds. Zero-event safety counts are pooled over seeds, with a Clopper–Pearson 95% upper bound. Full tables: [`results/final/SUMMARY.md`](results/final/SUMMARY.md).

| | v1 noise (dev) | **v2 noise (held out)** |
|---|---|---|
| CRC precision / recall (with review) | 1.000 / 0.954 (0.944–0.965) | **1.000 / 0.864 (0.855–0.872)** |
| CRC recall, no review | 0.260 | 0.215 |
| Cervical precision / recall | 1.000 / 0.964 | 1.000 / 0.889 |
| **CRC false closures** (pooled) | 0/2,750 (UB 0.13%) | **0/2,473 (UB 0.15%)** |
| Cervical false closures | 0/1,017 (UB 0.36%) | 0/912 (UB 0.40%) |
| **Wrong-patient messages released** | 0/4,260 (UB 0.09%) | **0/3,720 (UB 0.10%)** |
| Messages released without granted consent | 0/2,910 | 0/2,652 |
| ≤31-day KPI: truth / Wasl / EID-only | 0.328 / 0.319 / 0.127 | 0.322 / 0.284 / 0.121 |
| KPI absolute error: Wasl / EID-only | 0.009 / 0.201 | 0.038 / 0.201 |
| Linkage pair F1 | 0.971 | 0.928 |
| Reviewer hours per 1k loops | 20.1 (19.0–21.2) | 18.5 (17.2–19.7) |
| Review items per 100 loops, by name origin (arab / filipino / south_asian / western) | 56.9 / 53.2 / 51.9 / 55.2 | 52.6 / 52.4 / 51.1 / 48.9 |
| Review-load ratio, max/min origin | 1.12 (1.02–1.23) | 1.14 (1.03–1.25) |
| Duplicates or losses (outputs; ingest) | 0/94,523; 0/153,118 | 0/93,648; 0/152,539 |
| Tamper detected / false alarms | 85/85 / 0 | 85/85 / 0 |
| Forged events with effect | 0/120 | 0/120 |

**What the held-out test shows:**
- **Safety holds.** No false closures, wrong-patient messages or consent leaks on noise the system never saw.
- **Recall is lower.** It falls by about 9 points, because the gate *holds* the links it can no longer trust; it doesn't close them.
- **The KPI cost.** Wasl under-reports the 31-day KPI by about 4 points instead of about 1. That is still 5× closer to truth than EID-only tracking.

This is the trade-off the gate is designed to make: lost recall becomes review work or open cases, never wrong closures.

**Forged-event accounting fix** (re-scored from unchanged logs at `5bb022a`):
- In 2 v2 seeds, RBAC denied a forged "same" sign-off, but the linker had independently merged the targeted pair, a true false merge.
- The first scoring counted this as a forged-event effect. It is now reported separately, as `forged_target_pair_falsely_merged_by_linker`. It is also already counted in `linkage.false_merge_pairs`.
- Neither merge led to a false closure or a wrong-patient message.

**Not done: needs both team members.** The 43 report texts still carry an LLM-produced answer key. [`docs/labelling/`](../docs/labelling/) holds blind labelling sheets and a Cohen's κ script. Each of you labels separately, about 1.5–2 h. Until that is done, every evidence number is measured against the LLM key, not human labels.

**Other limits:**
- Reviewers, capacity and cohorts are synthetic.
- Cached 26B reader answers were used. Results for the live 4B reader are in the stage 2 section.
- Only the FHIR source was run. HL7 is covered in stage 4.
- The v7 rules were designed on earlier v1-noise seeds, which is why v2 is the honest held-out number.

## Learn-then-Test closure gate (replaces the hand-set tier)

**Problem it fixes.** v7strict's closure link tier τ_close = 0.98 was picked by hand after looking at eval errors.

**What we built.** Learn-then-Test (Angelopoulos, Bates, Candès, Jordan, Lei; *Ann. Appl. Stat.* 2025) picks the tier from a separate calibration set and comes with a finite-sample guarantee:

> P( false-closure rate ≤ α_fc **and** wrong-patient-message rate ≤ α_wp ) ≥ 1 − δ, with α = 0.001 and δ = 0.05.

**How it works:**
1. **Calibration cohorts:** `scripts/ltt_cohorts.sh` builds 6 cohorts, 12,600 loops in total.
   - People come from the persons3 **`cal` split**. It is disjoint from the `eval` split behind every reported result, and from the training split.
   - Corruptions use the held-out v2 noise model.
   - Review is the same as the main run: 4% reviewer error, merge+close double review.
2. **Loss counts:** `wsi7 ltt` replays each cohort through the same deterministic linker and closure cores at each candidate tier, with and without review. It counts per-loop losses.
3. **Threshold choice:** `scripts/ltt_select.py` computes exact binomial-tail p-values (Hoeffding–Bentkus values are shown alongside). It tests tiers from strict to loose in a fixed sequence, splitting δ across the 2 risks (Bonferroni).
4. **Using it:** `-policy v7ltt` reads the selected tier, and decisions are stamped `…+ltt`. `scripts/demo.sh ltt` reruns the calibration. Full table: [`results/ltt/LTT.md`](results/ltt/LTT.md).

**Results:**

| | With review | No review |
|---|---|---|
| Tiers certified (both risks) | 0.999 … 0.70 | 0.999 … 0.50 |
| False closures at τ = 0.98 | 0/12,600 | 0/12,600 |
| False closures at the loosest certified tier | 3/12,600 (τ 0.70) | 2/12,600 (τ 0.50) |
| Wrong-patient message loops, every tier | 0/12,600 | 0/12,600 |
| **Operating tier** (pre-declared rule: strictest certified tier with recall within 0.5 pp of the best) | **0.98** (recall 0.874) | 0.50 (recall 0.387) |

**What this shows:**
- **The hand-set tier is now justified.** The with-review LTT operating tier is the same 0.98 v7strict used, but it now comes from data the rules were never designed on, with a stated guarantee.
- **Looser tiers aren't worth it.** They certify (3 false closures in 12,600 is still below α at δ/2), but gain only 0.3 pp recall.
- **They held on unseen data.** On held-out eval cohort v2-s23, τ = 0.90 and τ = 0.70 also gave 0 false closures, with recall 0.890 vs 0.870.

**Limits:**
- The guarantee covers loops that are exchangeable with the calibration cohorts: the same synthetic generator, cal-split persons, v2 noise and reviewer model. **It is not a guarantee on real Abu Dhabi data.**
- The pilot must rerun the calibration on locally adjudicated loops. With 0 errors, certifying α = 0.001 at δ/2 needs about 3.7k loops per stratum.
- Origin-stratified guarantees would need about 3.7k loops per origin. We don't have those yet.
## Image evidence reader (third reader, hold-only)

Details are in [`image/RESULTS.md`](../image/RESULTS.md). **HyperKvasir competition use: written permission received from SimulaMet (October 2026) for this hackathon submission.**

**What it is.** Two MobileNetV3-small CNNs, trained on a near-duplicate-grouped split of HyperKvasir:
- **landmark:** caecum vs other lower-GI views, AUC 0.998;
- **prep:** BBPS 2–3 vs 0–1, AUC 0.991, specificity 0.83.

`-image cached|live` wires them in as a third reader. It can only hold or route a report to a human (`prep_grade_one_click`, `image_prep_inadequate`, `image_prep_conflict`) and raise the `missing_caecal_photo` audit flag. It never closes a loop; this is tested exhaustively in `image_test.go`. It is off by default, so all results above are unchanged.

**Integrated run** (seed 2026, simulated photo attachments):
- CRC false closures 0/571 and recall 0.961, the same as without the image reader.
- 0 labels upgraded; 2 held on photo/text prep conflict; 152 `missing_caecal_photo` flags.
- 321 one-click prep-grade items. Evidence reviewer-h per 1k loops fall 11.3 → 6.1. That gain is an assumption: 20 s per one-click grade.

**Limits.** Domain shift: Norwegian video frames vs UAE stills, so local recalibration is mandatory before any pilot.

## Second loop type: incidental lung nodules (Fleischner 2017)

**Claim tested:** one engine closes every follow-up loop; a new loop type is a rule row, two report readers and a generator, not a new pipeline. The lung loop runs through the same mock Malaffi → NATS → prep → identity linker → closure → reviewer/RBAC → audit/tamper path as CRC and cervical, in the same integrated run.

**Rule (built).** `fleischner2017` = `fleischnerRec(type, size, count, risk)` in `fleischner.go`, a pure function over the Fleischner Society 2017 table (MacMahon et al., *Radiology* 2017;284(1):228–243, PMID 28240562). `fleischner_test.go` covers every cell (solid single/multiple × low/high risk × <6 / 6–8 / >8 mm; subsolid ground-glass / part-solid / multiple × <6 / ≥6 mm), mean-diameter rounding (6 × 5 mm → 6 mm band) and scope. Window policy is ours, not Fleischner's: "6–12 mo" → [6, 12] months, "3–6 mo" → [3, 6], a single point "12 mo" → [10, 13], "CT at 3 mo, PET/CT or tissue sampling" → [0, 3]. "Optional CT at 12 months" is tracked like a recommended CT (flagged `optional`); unknown risk is treated as high.

**State machine (built, `lung.go`).** Same gates as the screening programmes:

| Condition | State | Provenance block |
|---|---|---|
| age < 35, known primary cancer or immunosuppressed | `out_of_scope` (clinician decides; never closed by the rule) | `hold_decision` |
| index CT: readers disagree / unavailable | open, evidence review item | `hold_decision` |
| index CT agreed, Fleischner cell = no routine follow-up | `no_followup_needed` | `evidence` → `recommendation` → `closure_decision` |
| follow-up CT on an untrusted link (p_cal < τ, near miss) | open, link review item, dropoff `unlinked` | `hold_decision` |
| follow-up CT trusted, readers disagree | open, evidence review item; a later CT cannot close past an unread earlier one | `hold_decision` |
| trusted + agreed `stable` / `resolved` | `closed` (next interval or discharge, `within_window` recorded) | … → `closure_decision` |
| trusted + agreed `growing` (mean diameter +>1.5 mm) / solid component ≥ 6 mm | `escalated` to specialist / MDT (end state) | … → `escalation_decision` |
| trusted + agreed `not_assessable` | `not_assessable` (repeat CT) | `hold_decision` |

Overdue clock = window end. A clinician sign-off on an evidence item substitutes for reader agreement, exactly as for CRC (double read on closing answers under `merge+close`).

**Readers (built; corpus simulated).** `data/lung/` holds a synthetic templated corpus written for this work by `scripts/lung_corpus.py` (seed 17): 38 index-CT texts and 48 follow-up texts with a labelled key, phrasing / unit (mm, cm) / section-order variants and traps (6 × 5 mm rounding, "stable" wording over > 1.5 mm growth, "slightly larger" within 1.5 mm, part-solid solid component reaching 7–8 mm, motion-degraded study, slice thickness "1.25 mm"). Reader A is regex/rules; reader B is the local **Gemma 3 4B** (Q4_K_M via `llama-server`, JSON-schema constrained, verbatim-quote grounding), asked once per unique text by `wsi7 lungcache` and replayed from `models/lung_gemma_cache.json` (no model calls in the run; a miss or ungrounded quote = unavailable → review). Both readers only extract fields; one shared rule turns fields into the label.

| Corpus (labelled key) | Index CT, n=38 | Follow-up CT, n=48 |
|---|---|---|
| Reader A (regex) correct | 38 | 48 |
| Reader B (Gemma 3 4B), prompt v1 → v2 → **v3 (frozen)** | 29 → 38 → **38** | 42 → 38 → **47** |
| Both agree (v3) / agreed but wrong vs key | 38 / **0** | 47 / **0** |

Read with care: reader A was written against the same templates it is scored on, and the Gemma prompt was revised twice on this corpus (v1 missed "multiple" counts and cm conversion and called resolved nodules non-assessable; v2 over-called "resolved"). There is no held-out lung text. Every reader-B error in every round was a disagreement routed to review, never an agreed wrong label; the one remaining v3 error (lu10, a 7×6 → 9×8 mm nodule read as stable) is exactly the case the gate exists for.

**Generator (built; cohort simulated).** `wsi7 gen -lung N` (`genlung.go`): an outpatient imaging centre reports the incidental nodule (DiagnosticReport) and opens a tracking ServiceRequest (age, exclusion, clinical risk from the EHR); the follow-up CT happens at a regional hospital radiology department under a name variant, with the CRC linkage noise model (script, typos, DOB noise, 62/26/12% EID capture). Other people's chest CTs at that hospital are distractors. Index CTs fall on days −420…90 (a look-back cohort, because Fleischner windows are 3–13 months and the observation window is the same 120 days as CRC). Simulated parameters: 4% under 35, 5% exclusions, 8% risk not recorded; follow-up within window 40%, late 22%, never 38% (tuned near the ~38% appropriate-follow-up rate in `docs/ROADMAP.md`); outcomes stable / resolved / growing / solid-component / not-assessable. Separate RNG stream and background persons only.

**CRC/cervical unchanged (verified).** With `-lung 0` the generator output is **byte-identical** to the pre-lung generator (sha256 of all 7 cohort files, `-noise v1` and `v2`, seed 2026, demo sizes). With `-lung 800` the five screening provider feeds are still byte-identical (only `truth.json`, `cohort_summary.json` and the two lung feeds differ). On the `-lung 0` cohort, `wsi7 sim` from the old and new binaries gives identical outputs, final states, clusters and metrics JSON. `genlung_test.go` pins the feed invariance.

**Integrated run (built, simulated data).** `WASL_LUNG=800 scripts/demo.sh all`, seed 2026, v7strict, cached reader, FHIR source, chaos on (FHIR consumer, linker, closure-0 + prep-0, NATS node kills), reviewer 4% error with double read on merges and closing answers. 1,500 CRC + 600 cervical + **800 lung** loops in one run, 33,801 FHIR messages, 19,650 outputs, 713 ev/s.

| Lung nodule (n=800) | Wasl live + review | Wasl, no review |
|---|---|---|
| Closure (closed + no_followup_needed) P / R | **1.000 / 0.993** | 1.000 / 0.957 |
| **False closures** | **0 of 297** (95% UB 1.3%) | 0 of 286 |
| Escalation (growing / solid ≥ 6 mm) recall / precision | 0.973 / 1.000 (36 of 37) | 0.757 / 1.000 |
| Overdue-flag accuracy (clock = window end) | 0.996 | 0.974 |
| Recommendation accuracy (action + window vs truth) | **1.000** (719 of 719 in-scope loops, coverage 1.000) | 1.000 |
| State accuracy | 0.996 | 0.973 |
| Appropriate follow-up within window (n=349 elapsed windows) | truth 0.418, **Wasl-measured 0.413**, per-loop agreement 0.994 | |

- Out of scope: 81/81 held as `out_of_scope`, none closed. No-follow-up-needed: 107/107.
- Two readers in the run: index CT 800/800 agree (38 unique texts), follow-up 361/362 agree (48 unique texts); agreed-but-wrong vs truth 0.
- Review: 29 lung items (3.6 per 100 loops), 55 reads, 0.98 reviewer-h per 1k loops, 0 wrong final sign-offs.
- The 3 misses (2 closed, 1 escalated, all predicted `open`): the follow-up CT registration was never linked, not even as a near miss. A linker recall loss, safe direction.
- Same run, screening programmes: CRC P/R/false closures 1.000 / 0.961 / 0 of 571, cervical 1.000 / 0.991 / 0 of 217 — the same closure numbers as the committed screening-only v7strict summary (`results/v7strict/summary.txt`). Exactly-once 0 duplicate / 0 lost of 19,650 outputs; provenance chains 1,739/1,739 verified; tamper 17/17 attacks detected, untampered baseline fires nothing; live-vs-replay final mismatch 0. Outreach counts (e.g. 546 released, 0 without granted consent) shift slightly against the screening-only run because reviewer answers interleave differently with the extra events; the consent invariant holds.

**What is designed, not built:**
- The next surveillance interval (e.g. CT at 18–24 months, ground-glass every 2 years) is recorded as `next_step` on the closure; it does not yet open a new loop.
- PET/CT or tissue sampling as closing evidence for the > 8 mm solid cell (only follow-up CT reports are read).
- Lung outreach: no consent records are generated for lung patients, so contact stays `unknown` and nothing is sent.
- HL7 v2 mapping of the lung reports (the run used the FHIR source only).
- Real radiology text: the corpus is synthetic and small; local reports need a new labelled set before any claim about reader accuracy.

**Reproduce:** `python3 scripts/lung_corpus.py` (corpus), `wsi7 lungcache -lungdata data/lung -out models/lung_gemma_cache.json` with `llama-server -m gemma-3-4b-it-Q4_K_M.gguf --port 8791 -c 8192 -ngl 99 -np 4` (cache, ~2.5 min), `WASL_LUNG=800 scripts/demo.sh all` (run, ~10 min). Metrics: `report7.json` → `programmes_live.lung_nodule`.

## Showcase run: every component in one integrated run

`WASL_POLICY=v7ltt WASL_READER=live WASL_SOURCE=hl7 WASL_IMAGE=cached WASL_LUNG=800 scripts/demo.sh all` (seed 2026; chaos on: process kills plus a NATS node kill; HL7 connection drops and duplicate re-sends). Outputs: [`results/showcase/`](results/showcase/).

**Configuration:**
- live local Gemma 3 4B reader;
- HL7 v2.5.1 over MLLP;
- Learn-then-Test closure tier;
- hold-only image reader;
- consent hold;
- lung-nodule loop;
- materialised API inputs.

| | Result |
|---|---|
| CRC with review | P 1.000, R 0.958, **0/569 false closures** |
| Cervical with review | P 1.000, R 0.991, 0/217 |
| Lung nodules (800 loops) | P 1.000, R 0.993, 0/297; recommendation accuracy 1.000 |
| ≤31-day KPI | EID-only 0.124, truth 0.330, Wasl 0.322 |
| HL7 ingest | 32,630 resources, 0 lost, 0 duplicated |
| Exactly-once | 0 duplicates in 25,620 outputs; 0 final-state mismatches |
| Provenance / forged / tamper | 1,741/1,741 chains verified; 0/24 forged events with effect; every tamper attack detected |
| Wrong-patient messages / messages without granted consent | 0 / 0 |
| Policy stamp | `v7strict+cal=true+guard=true+tau=0.98+ltt+image=cached` on every decision |

**Why nothing was held for consent in this run** (`intents_ever_held_for_consent` = 0; the FHIR runs hold about 350):
- **Not a consent bug.** In true arrival order, every patient's consent is on file before any outreach trigger. Checked: 581/581 triggers arrive at least 0.5 days after the first consent.
- **HL7 keeps that order.** The mock HIE sends one arrival-ordered stream, so the gate never has to wait.
- **The FHIR mock doesn't.** It publishes each provider's feed separately and they drift apart, so a downstream trigger can be processed before the referral and its consent. That is exactly when the hold fires.
- **Where the hold is tested:** the stage 3 tests and the FHIR runs. The invariant (0 released without granted consent) holds in both paths.

## Final code: frozen evaluation and Learn-then-Test re-run (new encoder chain)

**What was re-run.** Both evaluations were run again on the final code, `ebf85a9`: v7strict, the retrained encoder, the refit scorer and calibrators, and every merged feature (each optional and off by default). They replace the files in [`results/final/`](results/final/SUMMARY.md) and [`results/ltt/`](results/ltt/LTT.md). The earlier stage 7 and LTT sections above describe the previous chain (`45ed3a0`).

| | v1 noise (dev) | **v2 noise (held out)** |
|---|---|---|
| CRC precision / recall | 1.000 / 0.956 (0.945–0.968) | **1.000 / 0.873 (0.862–0.883)** |
| CRC false closures (pooled) | **1/2,757** (UB 0.20%) | **0/2,498** (UB 0.15%) |
| Cervical false closures | 0/1,031 | 0/917 |
| Wrong-patient messages / messages without consent | 0/3,991 / 0/2,791 | 0/4,063 / 0/2,778 |
| KPI absolute error, Wasl vs EID-only | 0.008 vs 0.201 | 0.035 vs 0.201 |
| Linkage pair F1 | 0.979 (was 0.971) | 0.935 (was 0.928) |
| Duplicates/losses · tamper · forged with effect | 0/92,329 · 85/85 · 0/120 | 0/94,015 · 85/85 · 0/120 |

**The new chain links better.** Held-out recall is up 0.9 pp, linkage F1 is up, and KPI error is down.

**One false closure** (dev noise, seed 37, loop `medlab/ServiceRequest/MED-sr-225`). Its colonoscopy report belonged to a different person.
- **The link:** a 2-hop path whose weakest edge was a model-only name match (`skmc-endo|SKM-MRN000390` ↔ `noor-endo|NOO-MRN000837`), with no corroborating DOB, sex or surname. Its calibrated p was **0.989**, exactly the cap for uncorroborated pairs.
- **The evidence:** the readers disagreed, double review signed the report as adequate, and nobody was asked about the *link*.
- **Why it closed:** v7strict closes at τ_close = 0.98, below the 0.989 cap. So a capped, uncorroborated model link can close a loop, provided the evidence passes.
- **Why it appeared now:** the new encoder and scorer put more pairs at that cap.
- **The old chain** never showed this: 0/2,750 and 0/12,600.

**Learn-then-Test on the new chain** (12,600 cal-split loops):

| τ_close | False closures | Recall | Certified |
|---|---|---|---|
| 0.99 | **0** | 0.873 | yes |
| 0.98 | 1 | 0.883 | yes (operating tier under the pre-declared rule) |
| 0.90 | 3 | 0.886 | yes |
| 0.70 | 13 | 0.887 | no |

The certificate still holds at 0.98 (1/12,600 is below α = 0.001 at δ/2). But 0.99, the tier that keeps uncorroborated links out of auto-closure entirely, costs about 1 pp of recall and showed 0 false closures in both the calibration and evaluation data. **Open decision:** keep 0.98 (certified, slightly higher recall) or move v7strict to 0.99. Choosing after seeing this error would be a post-hoc change and must be disclosed as one.
