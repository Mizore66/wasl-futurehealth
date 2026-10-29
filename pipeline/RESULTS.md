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
- Reviewer: an assumed 4% error rate per simulated read (a stress test, not a measured clinician rate; the offline sweep also ran 2% and 6%), double review on merges and "adequate", dual-reader merges.
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

**Built since (`727c804`):** a stable follow-up now opens the next surveillance loop (`<loop>#surv`) with the Fleischner window (e.g. CT at 18–24 months from the index scan for a multiple solid nodule; 22–26 months from the follow-up for ground-glass ≥ 6 mm). A later agreed CT that says stable or resolved closes it; growth escalates it; it goes overdue at the window end. In the 800-loop lung run, 77 of 152 stable closures opened one; the other 75 are cells where Fleischner stops after a stable scan. All 77 are open and none overdue, because the run ends before any 18-month window does. Test: `TestLungStableFollowUpOpensSurveillanceLoop`.

**Built since (`b53ce89`, see "Lung loop: PET/CT and tissue, patient outreach, HL7 v2" below):** PET/CT and tissue as closing evidence for the > 8 mm cell, consent records and outreach for lung patients, and the HL7 v2 mapping of lung reports.

**What is designed, not built:**
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

**Why nothing was held for consent in this run** (`intents_ever_held_for_consent` = 0; the FHIR runs held about 350 before feed pacing, 7–17 after):
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

**Decision: τ_close raised to 0.99** (`82bceb2`), above the 0.989 cap. A name-only model link can no longer auto-close a loop alone; it needs corroboration (DOB, sex, surname), an Emirates-ID chain or a human. This change was made *after* seeing the error, and we disclose it as post-hoc. `TestV7StrictTauAboveUncorroboratedCap` pins the tier above the cap. The LTT operating rule gets the same floor; it now selects 0.995, which behaves identically to 0.99 on the calibration data.

**Frozen evaluation at τ 0.99** (`82bceb2`, same seeds; superseded by the fairness fix below):

| | v1 noise (dev) | **v2 noise (held out)** |
|---|---|---|
| CRC precision / recall | 1.000 / 0.945 (0.933–0.957) | **1.000 / 0.862 (0.858–0.866)** |
| CRC false closures (pooled) | **0/2,724** (UB 0.14%) | **0/2,468** (UB 0.15%) |
| Cervical false closures | 0/1,012 | 0/911 |
| Wrong-patient messages / messages without consent | 0/4,798 / 0/3,274 | 0/4,453 / 0/3,032 |
| KPI absolute error, Wasl vs EID-only | 0.011 vs 0.201 | 0.039 vs 0.201 |
| Reviewer hours per 1k loops | 25.7 | 22.2 |
| Review-load ratio, max/min origin | 1.16 (1.08–1.24) | 1.18 (1.09–1.27) |
| Duplicates/losses · tamper · forged with effect | 0/101,747 · 85/85 · 0/120 | 0/99,538 · 85/85 · 0/120 |

**What the change cost:**
- Recall fell about 1 pp.
- Review work rose by about 20–35%. Links the new chain would have auto-closed at the cap now go to a human.
- On held-out noise, the review-load ratio's upper interval (1.27) crossed the 1.25 fairness limit, although the mean (1.18) was below it. The fix below brings it back under.
- Default one-seed demo: CRC R 0.948, 0/563.

### Fairness fix: placeholder birth dates (`cab895f`)

**Why Arab patients got more link reviews.** On held-out seed v2-s11 they had 46 link items per 100 loops vs 34 for Western patients.
- **Ruled out:** the share of loops needing a model link (≈63% for every origin), cross-script pairs (≈50% of items everywhere) and registrations per person.
- **The cause:** placeholder birth dates (1 Jan / 1 Jul). They make up 26% of Arab patients, 18% South Asian, 11% Western and 8% Filipino.
  - The gate correctly refuses a placeholder DOB as corroboration, because many people share it.
  - So exact-DOB agreement on true non-EID pairs was 0.38 for Arab patients vs 0.52 for Filipino patients, and fewer Arab links could auto-close.

**The fix.** A placeholder DOB now corroborates only when every name token agrees (same set of at least two skeleton tokens, any order), with the same sex.
- **How the rule was chosen:** on the persons3 `cal` split only (cohorts 101–106), never the eval seeds.
- **Calibration result:** 840 true pairs, 0 false. The existing surname-only rule had 3 false raw pairs on the same data.
- **Test:** `TestCorroboratedPlaceholderDob`.

**Frozen evaluation at `cab895f`** (same seeds; superseded by the cohort fix below):

| | v1 noise (dev) | **v2 noise (held out)** |
|---|---|---|
| CRC precision / recall | 1.000 / 0.947 (0.934–0.959) | **1.000 / 0.864 (0.859–0.870)** |
| CRC recall without review | 0.196 (was 0.184) | 0.184 (was 0.166) |
| CRC false closures (pooled) | **0/2,728** (UB 0.14%) | **0/2,475** (UB 0.15%) |
| Cervical false closures | 0/1,014 | 0/911 |
| Wrong-patient messages / messages without consent | 0/4,904 / 0/3,278 | 0/3,686 / 0/2,650 |
| Reviewer hours per 1k loops | 25.0 (was 25.7) | **20.8 (was 22.2)** |
| Review-load ratio, max/min origin | 1.15 (1.09–1.20) | **1.14 (1.07–1.21)**, was 1.18 (1.09–1.27) |
| Duplicates/losses · tamper · forged with effect | 0/101,413 · 85/85 · 0/120 | 0/95,868 · 85/85 · 0/120 |

**Result.** The review-load ratio's upper interval is now under the 1.25 limit on both noise models. Safety is unchanged: 0 false closures and 0 wrong-patient messages. Reviewer work fell about 6%.

**Caveat on message counts.** Fewer v2 messages were released (3,686 vs 4,453), but the change did not cause this.
- Outreach intents are counted before any gate, and they track each run's ingest order under chaos. These v2 runs were slower: the linker had processed 1.9k vs 4.1k events at the first kill.
- Replaying the same run log with the old binary gives the same intent count (664 for v2-s41).

**Learn-then-Test re-run at `cab895f`** (same 12,600 cal-split loops; superseded by the cohort fix below): the operating tier is still **0.995**, with 0 false closures and 0 wrong-patient messages.
- With review: recall 0.874 (was 0.873); review items 68.4 per 100 loops (was 70.2).
- Without review: recall 0.275 (was 0.256).

### Cohort fix: women in CRC loops (`a66b6a2`)

**The problem:** CRC loops had no women. Cervical loops took the women first from a Wikidata pool that is 22% female, so every CRC patient was a man. The real Abu Dhabi FIT+ population is 69.6% women (SEHA, PMC8343888).

**The fix:**
- Sampling keeps every woman in the split.
- 69.6% of CRC loops are women, drawn from all women in the sample, so some also hold a cervical loop. Those women have the same registrations under both programmes.
- **Eval cohorts:** 69.6% women in CRC; 315 of them also have a cervical loop (seed 11).
- **Calibration cohorts:** the cal split has only 779 women, so they reach 51.9%, with 600 also having a cervical loop. The Learn-then-Test cohorts are therefore less female than the eval cohorts. This is disclosed as a limit of the guarantee's scope.

**Frozen evaluation at `a66b6a2`** (same seeds; superseded by the override block below):

| | v1 noise (dev) | **v2 noise (held out)** |
|---|---|---|
| CRC precision / recall | 1.000 / 0.948 (0.940–0.956) | **1.000 / 0.878 (0.863–0.894)** |
| CRC false closures (pooled) | **0/2,703** (UB 0.14%) | **0/2,532** (UB 0.15%) |
| Cervical false closures | **1/1,008** | 0/966 |
| Wrong-patient messages / messages without consent | 0/5,160 / 0/3,638 | 0/5,136 / 0/3,541 |
| KPI absolute error, Wasl vs EID-only | 0.010 vs 0.207 | 0.035 vs 0.205 |
| Linkage pair F1 | 0.979 | 0.942 |
| Reviewer hours per 1k loops | 25.8 | 23.0 |
| Review-load ratio, max/min origin | 1.17 (1.09–1.25) | **1.16 (1.10–1.21)** |
| Duplicates/losses · tamper · forged with effect | 0/99,302 · 85/85 · 0/114 | 0/98,126 · 85/85 · 0/111 |

**The cervical false closure** (dev noise, seed 23, loop `ahs-mushrif/ServiceRequest/AHS-sr-160`) was simulated human error. Linkage was not involved.
- The patient link was correct (an Emirates-ID / human-confirmed path).
- Both machine readers said "not closed / unsatisfactory".
- Both simulated clinicians in the double review wrongly signed it "adequate". The simulation gives each read an assumed 4% error rate, a stress test rather than a measured rate for real clinicians. With independent errors, a double error happens about 0.16% of the time per item. Real error rates are unknown until the pilot measures them.
- The patient's other loop (CRC) played no part.
- Held-out noise has 0/966. The result shows that the double review has no backstop when both reviews override the report. It does not show that real clinicians make this mistake.

**Forged-event counter fix** (`f7e5cf1`; the pipeline runs were unchanged, only the report was re-scored):
- Four forged consent denials happened to target patients who genuinely withdrew consent later.
- The report had counted those patients' suppressed messages as the forgeries' effect.
- RBAC rejected every forged consent record, so these are now counted separately (`forged_consent_target_genuinely_opted_out`).
- Forged events with effect: 0.

**Learn-then-Test on the new calibration cohorts** (12,600 loops, 51.9% of CRC loops women):
- The operating tier is still **0.995**, with 0 false closures and 0 wrong-patient messages.
- With review, recall is 0.883; without review, 0.295.

**Female-list booking in the pipeline:** across the five v2 runs there are 133–170 open CRC cases per run that require a female endoscopist. All were suggested a slot within 31 days, vs 97–100% for everyone else (`booking.json` → `summary.female_list`).

### Override block and cervical repeat booking (`9c3f969`)

**Override block** (post-hoc: added after the `a66b6a2` evaluation's cervical false closure, and disclosed as such).
- **The rule:** a reader may extract an explicit inadequacy from a report (coded "unsatisfactory", caecum not reached, inadequate prep). If a human then signs "adequate" on that report, the loop stays open, and a confirming review is raised that shows the quote. Only the confirmation decides.
- **Why:** the simulated reviewers' 4% error rate is an assumption, not evidence that clinicians err. The block closes a path the double review couldn't catch: two humans overriding the report itself.
- **Test:** `TestOverrideAgainstReportNeedsConfirmation`.
- **In the frozen evaluation** it fired once in 10 runs, on the original case (`v1-s23`, `AHS-sr-160`).
  - The confirming review said "inadequate".
  - The loop stayed open with the `rejected_inadequate`, `econsult_re_refer` and overdue escalations.
  - Reviewer hours rose by about 0.1–0.2 per 1,000 loops.

**Frozen evaluation at `cdeac0a`** (same seeds; these are the files in `results/final/` now):

| | v1 noise (dev) | **v2 noise (held out)** |
|---|---|---|
| CRC precision / recall | 1.000 / 0.948 (0.940–0.956) | **1.000 / 0.878 (0.863–0.894)** |
| CRC false closures (pooled) | **0/2,703** (UB 0.14%) | **0/2,532** (UB 0.15%) |
| Cervical false closures | **0/1,008** (was 1) | 0/966 |
| Wrong-patient messages / messages without consent | 0/5,523 / 0/3,846 | 0/5,493 / 0/3,709 |
| KPI absolute error, Wasl vs EID-only | 0.010 vs 0.207 | 0.035 vs 0.205 |
| Reviewer hours per 1k loops | 26.0 | 23.2 |
| Review-load ratio, max/min origin | 1.17 (1.10–1.24) | **1.16 (1.09–1.23)** |
| Duplicates/losses · tamper · forged with effect | 0/100,576 · 85/85 · 0/114 | 0/99,254 · 85/85 · 0/111 |

**Learn-then-Test** (same cohorts; files in [`results/ltt/`](results/ltt/LTT.md)) is unchanged: τ 0.995, 0 false closures, 0 wrong-patient messages, recall 0.883.

**Cervical repeat booking** (`booking.json` → `cervical`). Suggestions now cover cervical loops that still need a colposcopy, at the same slot density per day as the CRC calendar.
- **First colposcopy:** inside the DoH App 5 window.
- **Repeat after an unsatisfactory exam:** 6–12 weeks later (DoH 10.6.4). It is booked when the result arrives, using `Event.earliest`.
- **On the five held-out runs:** 127/127 repeats landed inside the 6–12 week window. First colposcopies landed inside their window 83–88% of the time; most misses are the 14-day urgent windows.
- **Assumption:** half of colposcopy clinics are run by a female clinician [A].

### H7: correlated reviewer error (`a38d35e`)

**Question:** the double review assumes the two readers make independent mistakes. What happens if they don't?
- `ReviewerCfg.Rho` (ρ) is the chance that the second (and tie-break) reader copies the first reader's random draw for an item. At ρ=1 both readers make the same mistake on the same item, so a double read catches nothing.
- **Both the error rate and ρ are assumed stress-test values.** Nothing here is evidence that clinicians err, or err together.

**Run:** one calibration cohort (`ltt5 cal-s101`), v7strict, 5 reviewer seeds per cell, error rates 2/4/6%, ρ 0/0.25/0.5/1. Full table: [`results/h7/H7.md`](results/h7/H7.md).

| Error rate | Single read | Double, ρ=0 | ρ=0.25 | ρ=0.5 | ρ=1 |
|---|---|---|---|---|---|
| 2% | 1 / 2,442 | 0 / 3,395 | 0 / 3,396 | 1 / 3,397 | 2 / 3,399 |
| 4% | 3 / 2,418 | 0 / 3,328 | 1 / 3,331 | 3 / 3,334 | 4 / 3,336 |
| 6% | 4 / 2,401 | 0 / 3,270 | 1 / 3,277 | 4 / 3,282 | 5 / 3,286 |

(False closures / closures, summed over 5 seeds.)

**What it shows:**
- Double review removes every false closure only when the two reads are independent.
- The protection fades fast: at ρ=0.5 the false-closure rate per closure is already close to single review. At ρ=1 it is the same (4% error: 0.12% vs 0.12%).
- Reviewer hours barely change with ρ (27.5–28.1 per 1k loops), so cost doesn't reveal the problem. Only an audit does.

**So the correlated-error mitigations matter more than the double read itself:** different roles for the two reads, a blinded second read, reviewer-pair rotation, and the 2% weekly senior audit sample, which is how a pilot would measure ρ. The override block (above) covers the worst case, two humans overriding a report that states an inadequacy. It does not cover correlated errors on reports that are silent.

### Ingest pacing, replay ledger, encryption at rest: final evaluation at `d61c29a`

**What changed in the pipeline:**
- **Replay ledger** (`seen.go`): every ingested message id is kept in a replicated KV bucket, so an old version that a source re-sends after the 1 h dedupe window is dropped, not stored twice. The check is one KV read; the id is written after the publish ack; at start-up the adapter re-records the stream tail a crash may have left unrecorded. Per run it dropped 90–225 re-deliveries; ingest lost 0 and duplicated 0. Test: `TestSeenLedgerDropsOldReplays`.
- **Encryption at rest:** every JetStream store (streams and KV) uses ChaCha20-Poly1305. The key comes only from the environment (`WASL_JS_KEY`; an HSM in deployment); a run without it gets a random key. The store files contain no plaintext names.
- **Feed pacing** (`fhirsub.go`): a provider's publisher waits until every event more than 128 numbers older is acked. Event numbers follow HIE arrival order (about 250 a simulated day).

**Why pacing changed the outreach numbers.** The replay harness pushes the whole feed as fast as it can, and each provider publishes in parallel. Before pacing, the big feeds fell up to ~60 simulated days behind the small ones. So a referral could reach the stream after its own appointment, or a message intent before the patient's consent record. Wasl handled both correctly (no reminder for a past appointment; hold until consent), but the counts reflected the harness, not the system. With pacing, per v1-s23:

| | Before pacing (`cdeac0a`) | Paced (`d61c29a`) |
|---|---|---|
| Outreach intents | 1,188 | 2,029 |
| Released / without granted consent | 852 / 0 | 1,150 / 0 |
| Intents held for consent | 373 | 7 |
| Outputs | 20,528 | 25,011 |

- The consent hold is still needed when consent genuinely arrives late; in paced runs that is about 7–17 intents a run, not ~350.
- Reviewer hours rose (v2 23.2 → 25.9 per 1k loops) because more outreach reaches the second-identifier guard.
- Closure is unchanged within noise.

**Frozen evaluation** (files in `results/final/`; 3 runs in parallel, so throughput is per run under contention):

| | v1 noise (dev) | **v2 noise (held out)** |
|---|---|---|
| CRC precision / recall | 1.000 / 0.946 (0.939–0.953) | **1.000 / 0.879 (0.862–0.896)** |
| CRC false closures (pooled) | **0/2,698** (UB 0.14%) | **0/2,533** (UB 0.15%) |
| Cervical false closures | 0/1,004 | 0/968 |
| Wrong-patient messages / messages without consent | 0/10,090 / 0/5,540 | **0/9,398 / 0/5,467** |
| KPI absolute error, Wasl vs EID-only | 0.010 vs 0.207 | 0.035 vs 0.205 |
| Reviewer hours per 1k loops | 28.7 | 25.9 |
| Review-load ratio, max/min origin | 1.15 (1.07–1.22) | **1.17 (1.12–1.23)** |
| Duplicates/losses · tamper · forged with effect | 0/124,601 · 85/85 · 0/114 | 0/121,060 · 85/85 · 0/111 |
| Throughput (3 runs sharing the machine) | 560 ev/s | 627 ev/s |

**Patient-side measures** (new in `report7.json` → `patient_side`, from the event log, not the decision state):
- Stalled loops flagged overdue: 5,236/5,236 (v1) and 5,233/5,233 (v2), a median 0.5 days after the deadline (one decision tick).
- First escalation: a median 23.7–23.9 days before the deadline, so most stalls get a warning well before they breach.
- Results communicated within 21 days (15 working days): Wasl's measure equals the truth in every run (0.855–0.887).

**Learn-then-Test** is not re-run: the closure gate and calibrator are unchanged since `results/ltt/` (τ 0.995, 0 false closures).

### Synthetic App 14 report corpus, and the v3 reader fix

**Corpus** ([`data/evidence/reports_app14.json`](../data/evidence/reports_app14.json), `scripts/app14_corpus.py`, committed in `7ee951d` before any reader saw it). 300 colonoscopy reports in the DoH App 14 field layout (indication, prep protocol and quality, extent and landmarks, findings, interventions, impression, recommendations, optional pathology addendum).
- **Answer key = the generation spec.** `key_fields` is what the text documents, per the reader field definitions. `truth` is what happened, per the DoH rule: not adequate if the caecum was not reached or prep was poor; awaiting histology if any tissue was taken and no result is in the document. Undocumented items keep a hidden true value.
- **Variation:** 52 ungraded preps, 27 with no stated extent, 42 with Arabic/English headings (14 grade prep only in Arabic), 32 with OCR noise, 128 narrative, 8 mass biopsies, 20 random biopsies, Boston scores in about half of the graded preps.
- It replaces the 43 MTSamples texts as the main reader test. MTSamples stays as a second, real-text set.

**What it found** (`results/app14/before-fix-*`): both second readers, and the regex reader, had two blind spots that MTSamples never exercised.

| Reader check vs the key, 300 reports | Local 4B | 26B (third-party proxy) |
|---|---|---|
| Fields right: caecum / prep / polyps / histology | 260 / 196 / 300 / 287 | 300 / 299 / 300 / 300 |
| Auto-decided (both readers agree, no human) | 97 | 156 |
| **Unsafe auto-closures** | **8** | **9** |
| Frozen eval, CRC false closures v1 / v2 | **24/2,664 · 26/2,512** | **23/2,553 · 29/2,413** |

- **Biopsies:** the field asked for "a colonic polyp removed". A mass or random biopsy with no result yet therefore read as "nothing awaited", and the loop closed. This is a definition gap, so the more accurate 26B fell into it just as often.
- **Boston scores:** the regex counted any written Boston score as a grade, and the 4B read "Boston 2/9" as adequate.
- Every one of the 50 + 52 false closures traces to these 8–9 report texts. Cervical, wrong-patient and consent results were unaffected.

**Fix (`be441e7`, post-hoc, disclosed):**
- The field is now **`tissue_taken`**: a polyp removed or any biopsy taken (prompt, both readers, regex).
- A **Boston score is graded by its numbers**: adequate only with total ≥ 6 and every segment ≥ 2 (regex, and a code-side check of the second reader's own prep quote).
- A second-reader "adequate" whose quote has no grade word (only a product name) counts as not stated.
- Both readers re-read both corpora; the main cache (`models/gemma_cache.json`) is the 26B with the v3 prompt.

**After the fix** (`results/app14/v3-*`):

| | Local 4B | 26B |
|---|---|---|
| Fields right: caecum / prep / tissue / histology | 264 / 284 / 289 / 287 | **300 / 300 / 300 / 300** |
| Auto-decided / unsafe auto-closures | 163 / **0** | 202 / **0** |
| Frozen eval, CRC false closures v1 / v2 | **0/2,633 · 0/2,487** | **0/2,544 · 0/2,398** |
| CRC recall v1 / v2 | 0.911 / 0.856 | 0.880 / 0.825 |
| Reviewer hours per 1k loops v1 / v2 | 28.4 / 25.9 | 25.1 / 22.8 |
| Wrong-patient / without consent, v2 | 0/9,483 · 0/5,664 | 0/9,487 · 0/5,311 |

- The 4B's remaining tissue errors over-call "taken", so they only keep loops open.
- The 26B decides more reports without review (fewer hours). Its lower recall comes from reports with no documented extent. When both readers agree it is "not stated", the DoH completion rule (R1) keeps the loop open even when the hidden truth is complete. That is the safe direction.
- **MTSamples, re-run with the v3 readers** (`results/final/`, code `315a914`): unchanged. 0/2,698 and 0/2,533 CRC false closures, recall 0.946 / 0.879. Throughput in that run is lower (315 ev/s) because six evaluation runs shared the machine.

**Limits:**
- The corpus is synthetic and template-written, so it is cleaner than real dictation. The 43 MTSamples texts remain the real-text check.
- The fix was designed after seeing the failures on this corpus, so a fresh held-out corpus (new seed, phrasing written by someone else) is needed before claiming the 0.
- Caches: `models/app14/gemma_cache_{4b,26b}.json`. Reproduce: `WASL_REPORTS=data/evidence/reports_app14.json WASL_MODELS=<dir with one of them as gemma_cache.json> scripts/final_eval.sh`.

### Three independent report corpora, and readers v4 and v5

*Continued in the next section: corpora 5 and 6, written by other model families, broke v5 and v6. The current reader is v7.1 and the headline numbers are from it.*

The App 14 fix above was designed on the corpus that found the problem, so the 0 needed a fresh test. Each new corpus is 300 synthetic colonoscopy reports with an answer key, written by an independent author (a separate Claude agent) from a brief alone, without the Wasl code or the other corpora. Each was committed before any reader ran on it. Every corpus that broke a reader version then became tuning data, and the next corpus is the fair test of the fix.

| Corpus | Seed | Commit | Hard cases | Fair test of |
|---|---|---|---|---|
| 2: [`reports_heldout.json`](../data/evidence/reports_heldout.json) | 2027 | `ab6daa2` | 51% | v3 |
| 3: [`reports_heldout3.json`](../data/evidence/reports_heldout3.json) | 3031 | `7f72a6e` | 39% | v4 |
| 4: [`reports_heldout4.json`](../data/evidence/reports_heldout4.json) | 4049 | `8916058` | 91% (23 invented families, 75 correction addenda) | v5 |

Generators and dataset cards: `scripts/heldout/`, `scripts/heldout3/`, `scripts/heldout4/`.

**Corpus 2, fair test of v3: safe, but low recall** (`results/heldout/v3-*`). 0 false closures with either reader. CRC recall on v2 noise fell to 0.65 (26B) and 0.55 (4B). Two causes:
- the Boston parser read the first digit of "Boston 2+3+2 = 7" as the total, so good preps went to review;
- a tissue diagnosis from an earlier procedure ("2019: two tubular adenomas removed") was read as this procedure's histology, which only became visible once prep was fixed.

**v4 (`7962054`, tuned on corpus 2, disclosed):** a Boston score is graded only when all three segments are read and agree with any stated total (or the total is below 6); otherwise prep is undecided. A histology mention inside a sentence about an earlier procedure is ignored, in the regex and in the second reader's quote. Corpus 2 recall on v2 noise: 26B 0.76, 4B 0.64; 0 false closures.

**Corpus 3, fair test of v4: the 4B failed** (`results/heldout3/v4-*`).

| | 26B | Local 4B |
|---|---|---|
| Unsafe auto-closures (reader score) | 0 | **2** (t258, t287) |
| Frozen eval, CRC false closures v1 / v2 | 0/2,275 · 1/2,105 | **9/2,324 · 9/2,183** |

- Both 4B cases were correction addenda: the body says one thing, a signed addendum corrects it ("BBPS 2-2-3 entered in error … BBPS 1-2-2, total 5"; "recorded as caecum because of a template default … the caecum was NOT reached"). The readers took the first statement. In t258 the 4B quoted the correction, but the v4 quote check took the first of the two Boston scores in the quote and overrode it. All 18 4B false closures trace to these two texts.
- The single 26B false closure is not a reader error. Both readers said "prep not stated", the report went to review, and the simulated reviewer marked it adequate. The 4% reviewer error rate is an assumed stress-test rate.

**v5 (`533360b`, tuned on corpus 3, disclosed):**
- A report that says part of it was wrong (correction, "entered in error", amended, "should read", template default, تصحيح) is never auto-closed; it goes to review. An ordinary addendum (for example a histology result) does not trigger this. A first version that also matched "addendum" cost about 5% of auto-decisions on corpus 1 and was narrowed.
- Two Boston scores in one text that grade differently decide nothing, in the regex and in the second reader's quote.
- Re-scored on corpora 1–3 and MTSamples: 0 unsafe with either reader; the only change elsewhere is one auto-decision fewer for the 4B on corpus 3.

**Corpus 4, fair test of v5: passed with both readers** (`results/heldout4/v5-*`).

| | 26B | Local 4B |
|---|---|---|
| Fields right vs key: caecum / prep / tissue / histology | 291 / 282 / 294 / 296 | 198 / 227 / 269 / 245 |
| Auto-decided / right / wrong in the safe direction | 135 / 118 / 17 | 122 / 93 / 29 |
| **Unsafe auto-closures** (v4 code on the same caches) | **0** (0) | **0** (1, q005) |
| Frozen eval, CRC false closures v1 / v2 | **0/2,507 · 0/2,270** | **0/2,338 · 0/2,140** |
| CRC recall v1 / v2 | 0.877 / 0.805 | 0.817 / 0.759 |
| Reviewer hours per 1k loops v1 / v2 | 29.3 / 26.3 | 32.1 / 29.3 |
| Wrong-patient / without consent, v2 | 0/9,363 · 0/6,076 | 0/9,359 · 0/6,834 |

**All frozen evaluations, CRC false closures on held-out v2 noise:**

| Corpus | v3 26B | v3 4B | v4 26B | v4 4B | v5 26B | v5 4B |
|---|---|---|---|---|---|---|
| MTSamples (43) | 0/2,533 | – | 0/2,534 | – | **0/2,534** | – |
| 1: App 14 | 0/2,398 | 0/2,487 | 0/2,397 | 0/2,485 | = v4 (reader score unchanged) | = v4 |
| 2 | 0/1,867 | 0/1,584 | 0/2,182 | 0/1,837 | = v4 | = v4 |
| 3 | – | – | 1/2,105 (reviewer) | **9/2,183** | reader score 0 unsafe | reader score 0 unsafe |
| 4 | – | – | – | – | **0/2,270** | **0/2,140** |

- MTSamples headline at v5 (`results/final/`, code `8916058`): CRC P 1.000, recall 0.947 / 0.879, 0/2,700 · 0/2,534 false closures, 0/9,386 wrong-patient, 0/6,285 without consent, review-load ratio across origins 1.18 (1.11–1.25).
- Where v5 did not change the reader score (corpora 1, 2, MTSamples), the v4 frozen runs stand in for v5; v5 frozen runs were made where it matters (corpus 4, MTSamples).

**What this does and does not show:**
- Each reader version that a fresh corpus broke was fixed on that corpus and passed the next one. Two of three fresh corpora found a real gap; the 26B never produced an unsafe auto-closure on a fresh corpus, the 4B did once (corpus 3).
- Corpus 4 is the only fair test of v5. One clean corpus is evidence, not proof; the next unseen phrasing family may break it again, which is why every auto-closure is still double-checked by a second reader and every disagreement goes to a human.
- The 26B decides more reports and is more accurate on every field. The 4B stays safe by sending more to review (+3 reviewer hours per 1k loops on corpus 4).
- All four corpora are synthetic and written by Claude agents; they share a model family even with separate authors. Real Abu Dhabi reports have not been tested.
- Caches: `models/heldout{,3,4}/gemma_cache_{26b,4b}.json`. Reproduce: `WASL_REPORTS=data/evidence/reports_heldout4.json WASL_MODELS=<dir with one cache as gemma_cache.json> scripts/final_eval.sh`; reader score: `wsi readerscore -policy v7strict -wasl <dir> -reports <corpus>`.

### Corpora 5–8 by other model families, and readers v6, v7, v7.1

Corpora 2–4 were all written by Claude agents, which share one model family and may share blind spots. Corpora 5–7 were each written by a different model family from the same brief ([`scripts/heldout6/brief.md`](../scripts/heldout6/brief.md); corpus 5 used the same text by hand). The author scripts only run the generator and return validation errors to the model; we never edit the generator. Each corpus was committed before any reader ran on it.

| Corpus | Author | Seed | Commit | Hard cases | Fair test of |
|---|---|---|---|---|---|
| 5: [`reports_heldout5.json`](../data/evidence/reports_heldout5.json) | GPT-6.1 Sol (OpenAI), via Codex | 5051 | `1c6dcbc` | 98% | v5 |
| 6: [`reports_heldout6.json`](../data/evidence/reports_heldout6.json) | Gemini 4 Argon (Google) | 6067 | `b667b86` | 49% | v6 |
| 7: [`reports_heldout7.json`](../data/evidence/reports_heldout7.json) | Kimi K3.1 Pro (Moonshot AI) | 7079 | `3eb4d03` | 59% | v7 |
| 8: [`reports_heldout8.json`](../data/evidence/reports_heldout8.json) | GPT-6.1 Sol (OpenAI), via the gateway | 8081 | `2cac569` | 100% (21 families) | v7.1 (shipped) |

**Corpus 5, fair test of v5: failed with both readers** (3 unsafe auto-closures each).
- Every case: this procedure's result was still pending ("Current accession …: histology awaited"; "received; gross description only"), while a diagnosis from an earlier procedure or the stomach sat in wording the v4 filter did not know ("Historical surveillance record (2024 colonoscopy): tubular adenoma"; "that accession is separate from today's procedure"). Both readers took it as this procedure's histology.
- Frozen evaluation, v5, 26B (`results/heldout5/v5-26b`): **5/2,129 · 7/2,109** CRC false closures (95% upper bounds 0.55%, 0.68%).
- **v6 (`9bb60ee`, disclosed):** a report that says this procedure's result is pending, awaited or only received, and also holds a tissue diagnosis, is never auto-closed. More wordings mark a sentence as an earlier procedure.

**Corpus 6, fair test of v6: failed** (26B 3 unsafe, 4B 5 unsafe; `results/heldout6/`).
- Every case: the scope reached the ileocaecal valve, or saw the caecum at a distance, but did not reach it ("reached the distal lip of the ileocecal valve, but … could not be advanced past the valve"; "caecal pole visualised at a distance, though severe looping prevented advancing the tip into the caecum"). The reader prompt itself counted the valve as the caecum, so both readers followed our own definition.
- **v7 (`a3f9ccd`, `03862fe`, disclosed):** checked in code, with no new model calls. The caecum counts as reached only with an explicit caecum statement, the appendiceal orifice or the terminal ileum, as in the DoH caecal-landmark standard. Any clause saying the caecum, its landmarks or the scope tip could not be reached, or was seen only at a distance, means "not reached".

**Corpus 7, fair test of v7: passed with both readers.**

| | 26B | Local 4B |
|---|---|---|
| Unsafe auto-closures | **0** | **0** |
| Auto-decided / right / wrong in the safe direction | 216 / 177 / 39 | 223 / 159 / 64 |
| Frozen eval, CRC false closures v1 / v2 | **0/2,245 · 0/2,021** | **0/1,889 · 0/1,720** |
| CRC recall v1 / v2 | 0.779 / 0.719 | 0.656 / 0.612 (v7; most of the gap is the "appendiceal opening" synonym fixed in v7.1) |

**v7.1 (`54ff4ba`, recall only, disclosed).** v7 cost recall: 17 complete exams in corpus 7 said "appendiceal *opening*", and on MTSamples "retroflexion … failed to reveal lesions" and "the terminal ileum could not be intubated" read as caecum not reached. MTSamples recall fell from 0.947 / 0.879 to 0.917 / 0.850, still with 0 false closures (`results/mts-v7`). v7.1 counts inability only with movement verbs (reach, advance, intubate, pass, manoeuvre…), skips clauses about the terminal ileum alone, and adds "appendiceal opening". Re-scored on all seven corpora and MTSamples, with 0 unsafe for either reader (`results/v7.1-reader-scores/`). One MTSamples report ("to the ileocecal valve") now goes to review, where the team key counted it as complete.

**Corpus 8, fair test of the shipped v7.1: passed with both readers** (`results/heldout8/v71-*`). Written after v7.1 was committed, so v7.1 (not only v7) has a fair test.

| | 26B | Local 4B |
|---|---|---|
| Fields right vs key: caecum / prep / tissue / histology | 298 / 299 / 300 / 300 | 278 / 226 / 300 / 229 |
| Auto-decided / right / wrong in the safe direction | 125 / 96 / 29 | 100 / 66 / 34 |
| **Unsafe auto-closures** | **0** | **0** |
| Frozen eval, CRC false closures v1 / v2 | **0/2,346 · 0/2,100** | **0/2,272 · 0/2,029** |
| CRC recall v1 / v2 | 0.813 / 0.746 | 0.787 / 0.720 |

**MTSamples headline at v7.1** (`results/final/`, code `54ff4ba`): CRC P 1.000, recall 0.918 / 0.851, **0/2,617 · 0/2,452** CRC false closures, cervical 0/1,004 · 0/969, 0/9,410 wrong-patient and 0/5,315 without consent on held-out noise, review-load ratio across origins 1.18 (1.11–1.24). Recall is 3 points below reader v5 (0.947 / 0.879) because of one report reused across about 40 loops: "advanced to the ileocecal valve" no longer counts as caecal completion without the appendiceal orifice (the DoH landmark standard). The team key counted it complete. This is a deliberate trade: counting the valve alone is what failed on corpus 6.

**Every fair test** (unsafe auto-closures, reader score, 300 unseen reports each):

| Corpus | Author | Reader tested | 26B | Local 4B |
|---|---|---|---|---|
| 2 | Claude | v3 | 0 | 0 |
| 3 | Claude | v4 | 0 | 2 |
| 4 | Claude | v5 | 0 | 0 |
| 5 | GPT-6.1 Sol | v5 | 3 | 3 |
| 6 | Gemini 4 Argon | v6 | 3 | 5 |
| 7 | Kimi K3.1 Pro | v7 | 0 | 0 |
| 8 | GPT-6.1 Sol | v7.1 | 0 | 0 |
| **Total, per report** | | | **6 / 2,100 (0.29%)** | **10 / 2,100 (0.48%)** |
| **Total, per automatic closure** | | | **6 / 211 (2.8%, 95% UB 6.1%)** | **10 / 137 (7.3%, UB 13.0%)** |
| Shipped v7.1 and v7 only (corpora 7, 8) | | | 0 / 71 auto-closures (UB 5.1%) | 0 / 47 (UB 7.6%) |

**What this shows:**
- **On unseen phrasing, the readers are not perfect.** Three of seven fresh corpora found a phrasing family that both readers agreed on wrongly. Per report that is 6 of 2,100 with the 26B (0.29%) and 10 of 2,100 with the local 4B (0.48%). But most reports go to a human anyway, so the rate that matters for an automatic closure is **6 of 211 auto-closures with the 26B (2.8%, 95% upper bound 6.1%) and 10 of 137 with the 4B (7.3%)**: about 1 in 35 automatic 26B closures on these corpora was wrong. All of them came from corpora 3, 5 and 6; the shipped v7.1 and v7 had 0 of 71 on corpora 7 and 8, which still only bounds the rate below 5.1%. These corpora were built to be hard (49–100% hard cases), so real reports should do better, but that is unmeasured.
- **Each failure was a whole class, not a random error.** Each was fixed by a generic rule (corrections, pending results, landmarks) and stayed fixed on every other corpus. v7.1 has 0 unsafe on all seven corpora and MTSamples with either reader.
- **Other model families found what Claude-written corpora did not.** The two classes found by GPT and Gemini were invisible in three Claude corpora.
- **A pilot cannot start with unchecked automatic closure.** To catch a new error class that affects 3% of auto-closures with 95% probability, a clinician must re-read about 100 of them; a 10% sample alone would need about 1,000 auto-closures to get there. **Pilot rule (proposed):** start in shadow mode, where every automatic closure is only a suggestion and a clinician confirms it, until 300 in a row are confirmed correct (0/300 gives a 95% upper bound of 1.2%). Then auto-close with a random 10% clinician audit. Any confirmed error returns that phrasing class to shadow mode, becomes a rule plus a regression test, and restarts the count of 300.
- The local 4B is less accurate than the 26B on every field. It stays safe mainly by sending more to review, and it had more unsafe cases on the hardest corpus (6).
- All corpora are synthetic. Real reports have not been tested.

Caches: `models/heldout{5,6,7,8}/gemma_cache_{26b,4b}.json`. Reader score: `wsi readerscore -policy v7strict -wasl <dir> -reports <corpus>`.

### Ungraded bowel prep: query the unit, escalate, clinician at day 15 (`e344a1a`)

**What changed.** A colonoscopy report that records the caecum but no bowel-prep grade no longer goes to a reviewer, because a reviewer cannot grade prep from a report that does not state it. Wasl now queries the endoscopy-unit lead, escalates to the programme coordinator after 5 working days, and at day 15 a clinician decides. A usable grade from the unit closes the loop as usual; "cannot determine", or no answer by day 15, goes to the clinician (`pipeline/prepquery.go`).

**Simulated unit.** No published study measures how units answer this query, so every rate is an assumption built from proxies ([`docs/research/prep-query-rates.md`](../docs/research/prep-query-rates.md)): 55% answer within 5 working days, 20% only after escalation, 25% never; 50% of answers are a usable grade. The day-15 clinician decision is given a 25% per-read error because the clinician decides without a grade. **This is a stress-test rate, not evidence that clinicians make errors.**

**Demo cohort** (seed 2026, `results/prep-query/`):

| | Prep flow off (before) | Prep flow on |
|---|---|---|
| CRC recall (with review) | 0.925 | 0.781 |
| CRC false closures | 0 / 540 | 0 / 443 |
| Reviewer hours per 1k loops | 28.7 | 24.9 |
| Ungraded-prep loops: unit graded / escalated / awaiting unit | – | 127 / 9 / 26 |
| Closed after the query | – | 99, 0 false |

Recall falls because the old simulation was too generous: its reviewer graded prep from text that contains no grade. The new number is the honest one under these assumptions.

### Pilot switches: shadow mode and the audit sample (`4796f3b`, `b699ab9`)

**What changed.** Two switches, off by default:
- **Shadow mode** (`-shadow`, `WASL_SHADOW=1`): every automatic closure becomes a suggestion that a clinician confirms.
- **Audit sample** (`-autoaudit p`, `WASL_AUTOAUDIT=p`): after automatic closure starts, a random share p of automatic closures is re-read by a clinician, and a disagreement re-opens the loop.

**Streak rule fix.** The pilot rule needs 300 confirmations in a row. With a 4% assumed reviewer error, a clinician wrongly rejects a correct closure about once every 25 checks, so the count would almost never reach 300 (longest run 81). A rejection now goes to a senior adjudicator (same assumed error rate), and **only an adjudicated reader error resets the count.**

**Demo cohort** (seed 2026, `results/pilot/`):

| | Normal | Shadow mode | 10% audit sample |
|---|---|---|---|
| CRC recall / false closures | 0.781 / 0 | 0.765 / 0 | 0.776 / 0 |
| Reviewer hours per 1k loops | 24.9 | 33.2 | 25.8 |
| Clinician checks | – | 339: 326 confirmed, 13 rejected | 33 audits |
| Rejections traced to the reader / to the reviewer | – | 0 / 13 | 0 / 4 |
| 300 in a row reached | – | day 120 (longest 326) | – |

The 4 audit rejections re-opened 4 correct closures: a cost in reviewer time, not a safety failure. Neither switch found a reader error on this cohort.

### H7 mitigations: blinded second read, pair rotation, senior audit (`d9e243e`)

H7 showed that double review loses its protection when the two reviewers' errors are correlated. Three standard mitigations are now switches (`-reviewblind`, `-reviewrotate`, `-senioraudit p`). Effect sizes come from published screening-mammography double reading, so they transfer only partly to document review:
- **Blinding** removes 50% of the copying (range 15–85%). Discordance rose when the second reader was blinded (Klompenhouwer, *Eur J Cancer* 2015; Cooper, *Eur Radiol* 2022); 45–75% of readers followed a wrong prior suggestion (Dratsch, *Radiology* 2023).
- **Rotation** removes 10% (range 0–30%). This is an assumption: no evidence of pair-specific correlated error was found (Gommers, *Radiology* 2023).
- **Senior audit:** 2% of signed closing answers, with the senior at half the reviewer's error rate (assumption; RADPEER disagreement 2.9%, Borgstede, *JACR* 2004).

**False closures, 4% reviewer error, merge+close double read, 5 seeds** (`results/h7-mitigations/`; about 3,330 closures per cell):

| Design | ρ = 0 | ρ = 0.25 | ρ = 0.5 | ρ = 1 |
|---|---|---|---|---|
| Double read | 0 | 3 | 4 | 4 |
| + blinded second read | 0 | 1 | 3 | 4 |
| + pair rotation | 0 | 3 | 4 | 4 |
| + 2% senior audit | 0 | 3 | 4 | 4 |
| All three | 0 | 1 | 3 | 4 |

ρ is the assumed correlation between the two reviewers; every rate here is a stress-test assumption. **Blinding is the only mitigation that helps**, and only while correlation is partial; at a blind factor of 0.85 every ρ gives 1. A 2% audit is too small to catch 4 bad closures in 3,330; it measures the error rate, it does not fix it. All false closures come from about 4 items that are hard for any reader. Reviewer time is unchanged (23.0–23.4 h per 1k loops).

### Rejected-referral hand-off packet (`b53ce89`)

**What changed.** When a referral is finally rejected, responsibility goes back to the referring doctor (DoH Standard for Primary Healthcare Services 2022, 4.9.5). Wasl now builds a structured hand-off packet for them (`pipeline/handoff.go`). "Finally rejected" means one of: the e-consult outcome is reject; a redirect that the second unit also rejects; or a rejected referral whose e-consult is still unanswered after 5 business days.

The packet holds: the referral, a reason code from a closed list, advice to the referrer, a suggested unit, any information requested, the responsible clinician, timestamps, whether the patient was told, the outreach consent, and an **act-by date** (2 business days for urgent, 5 for routine). If the referrer has not acted by then, the care coordinator is chased; after the same number of business days again, the medical director; and an urgent loop still not re-referred 14 days after the original referral is flagged.

Fields and clocks come from NHS e-RS, Ontario eReferral, CPSO and FHIR Task, with the DoH roles ([`docs/research/referral-handoff-lung.md`](../docs/research/referral-handoff-lung.md), part A). No public DoH rule sets a deadline for the referrer, so the act-by dates are assumptions. So are the referrer's simulated actions: 45% act by act-by, 20% after the coordinator, 10% after the medical director, 25% never.

**Same cohort with and without it** (seed 2026, `results/handoff/`). The referrer is simulated after every other loop on its own random stream. With the switch off, the cohort is byte-identical to the previous generator; with it on, only the rejected loops differ.

| | Off | On |
|---|---|---|
| CRC recall / false closures | 0.783 / 0 | 0.784 / 0 (9 more closures, all correct) |
| Cervical recall / false closures | 0.966 / 0 | 0.962 / 0 |
| Finally rejected loops (CRC + cervical) | 37, all stuck | 37 packets in truth, 31 built by Wasl (+5 the truth does not have) |
| Packets with every required field | – | 36 / 36 |
| Referrer acted (truth) / acted by act-by | – | 28 / 17 |
| Loops closed after re-referral (truth / Wasl) | 0 / 0 | 15 / 12, 0 false |

The 6 missed packets have not been analysed yet. Their loops still raise the existing e-consult escalations.

### Lung loop: PET/CT and tissue, patient outreach, HL7 v2 (`b53ce89`)

This closes the three lung gaps listed above.

**PET/CT and tissue as closing evidence** (`pipeline/lungworkup.go`). Fleischner 2017 allows "CT at 3 months, PET/CT, or tissue sampling" for a solid nodule over 8 mm, but only a follow-up CT could close the loop.
- PET/CT with low uptake closes the loop (next step: CT surveillance, BTS 2015).
- An FDG-avid PET refers to the lung MDT and waits for tissue within 28 days (NHS optimal lung pathway). After 28 days it is overdue.
- Benign tissue closes the loop; malignant tissue escalates to the MDT; non-diagnostic tissue holds for repeat sampling.
- PET/CT applies only to the over-8 mm cell; it never closes a loop whose Fleischner cell asks for CT.

PET and pathology reports carry a coded result. Reader A is a regex on the text and reader B is the coded result; the loop moves only when they agree, or a clinician signs. In the simulation, 3% of coded results contradict the text.

Simulated parameters: recommendation to PET median 11 days and PET to procedure median 13 days (sourced). The rest are assumptions: work-up choice (CT 40%, PET/CT 45%, tissue 15%), 35% FDG-avid, biopsy after an avid PET 85%, and the result mixes. Texts are synthetic templates, so the regex reader was written against them.

**Patient outreach and consent.** Lung patients now get consent records from the imaging centre: 4% deny, and an assumed 10% have no record yet. Two messages are added: a follow-up reminder in the last 14 days of the Fleischner window, and an overdue recall. Both are sent in the imaging centre's name, phone first then letter, and **only with granted consent**: held while consent is unknown, cancelled on deny (UAE Federal Law 2/2019 Art. 16; DoH PHC 4.10.2).

**HL7 v2** (`pipeline/hl7lung.go`). Lung reports travel as ORU^R01:
- OBR-4: LOINC with the local code as the alternate identifier (CT chest 24627-2, PET/CT 81555-5, pathology 11526-1).
- OBR-25: result status.
- OBX: one per report section (findings 18782-3, impression 19005-8, recommendation 18783-1), plus the coded result in a CWE OBX.

The listener rebuilds code, date, status, text and coded result from the segments, not from the Z-segment copy (test `TestHL7LungReportFromSegments`). The full-cohort HL7 round-trip test passes on a cohort with lung loops.

**Integrated run over HL7 v2** (seed 2026, 800 lung loops, chaos on, `results/lung-workup/`):

| | Value |
|---|---|
| Lung state accuracy | 0.994 |
| Lung closures | 296 of 301 true, **0 false** (95% UB 1.3%) |
| Escalations | 30 of 30, 0 extra |
| Over-8 mm loops closed by CT / PET/CT / tissue | 10 / 15 / 4 |
| Lung messages released / held for consent / cancelled | 148 / 18 / 7 |
| Messages released without granted consent (whole run) | **0 / 1,450** |
| CRC / cervical false closures in the same run | 0 / 0 |

With `-handoff=false -lungworkup=false` the generator output is byte-identical to the previous one, with and without `-lung 800`.

**Still open:** PET/CT closures do not yet open a surveillance-CT child loop. Lung loops have no e-consult or hand-off packet. The reader-B coded result is synthetic, not from a real PACS or LIS.
