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

`WASL_POLICY=v7ltt WASL_READER=live WASL_SOURCE=hl7 WASL_IMAGE=cached WASL_LUNG=800 scripts/demo.sh all` (seed 2026; chaos on: process kills plus a NATS node kill; HL7 connection drops and duplicate re-sends). Outputs: [`results/showcase/`](results/showcase/) (since replaced by the `cfcf50f` showcase run below).

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

The missed packets were traced later; see "Hand-off packet trace" below.

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

**Still open at the time:** PET/CT closures did not yet open a surveillance-CT child loop, and lung loops had no hand-off packet; both were built later (see "Fourth loop type, follow-up tracking and hand-off extensions"). The reader-B coded result is synthetic, not from a real PACS or LIS.

### Frozen evaluation with every new piece on (`a3ad83a`)

The headline evaluation now runs with the ungraded-prep query, the rejected-referral hand-off and **800 lung-nodule loops per run**, on MTSamples with reader v7.1 (`results/final/`).

**First run (`b53ce89`): 4 CRC false closures** (3/2,285 dev noise, 1/2,164 held-out), all one class. The report had ungraded prep and tissue taken, with histology still pending. The day-15 clinician answered "adequate" (25% assumed error) and the engine closed the loop without histology. This was a bug against the documented design, where the clinician decides the prep only. **Fix (`a3ad83a`, post-hoc, disclosed):** the day-15 answer sets the prep grade, and closure still follows the report's tissue and histology fields (test `TestDay15AdequateKeepsHistologyWait`). Raw results of the failed run are kept outside the repo.

**Re-run at `a3ad83a`:**

| | Dev noise v1 | Held-out noise v2 |
|---|---|---|
| CRC precision / recall | 1.000 / 0.761 (0.734–0.788) | 1.000 / 0.703 (0.680–0.726) |
| CRC false closures | **0 / 2,185** | **0 / 2,061** (UB 0.18%) |
| Cervical false closures | 0 / 1,023 | 0 / 987 |
| Lung false closures / recall | **0 / 1,506** / 0.984 | **0 / 1,463** / 0.949 |
| Hand-off: packets matched / closed after re-referral | 206 of 223 / 66, 0 false | 176 of 210 / 44, 0 false |
| Wrong-patient messages | 0 / 12,421 | 0 / 11,685 |
| Sent without granted consent | 0 / 7,278 | 0 / 7,138 |
| KPI error, Wasl vs EID-only | 0.010 vs 0.208 | 0.029 vs 0.191 |
| Review-load ratio across origins, per-seed mean (95% CI) | 1.19 (1.11–1.28) | 1.18 (1.05–1.31) |
| Review-load ratio across origins, pooled over seeds (no-bias 95th pct) | 1.15 (1.10), p = 0.002 | 1.13 (1.11), p = 0.02 |
| Reviewer hours per 1k loops | 19.1 | 17.2 |
| Duplicate or lost outputs; tamper detected | 0 / 141,181; 85 / 85 | 0 / 137,134; 85 / 85 |

**Recall fell** from 0.918 / 0.851 at `54ff4ba` because 20 of the 43 MTSamples reports never grade the bowel prep. Those loops used to be closed by a simulated reviewer who graded prep from text that contains no grade. Now they wait for the unit's grade or the day-15 clinician, as the DoH standard requires, and the unit's replies are simulated with assumed rates. Under those assumptions the lower recall is the honest number; with real units it depends on how often reports carry a grade (1–65% missing in published audits).

**Review-load ratio (investigated):** the per-seed interval crossing 1.25 is mostly how the metric behaves, and a small real gap sits under it.

- **The metric is noisy and above 1 by construction.** It is max/min over four origin groups of unequal size (about 250 Filipino to 1,500 Arab loops per run). With the origin labels shuffled across loops, so that origin has no effect at all, the per-seed ratio still has a median of 1.11–1.13 and a 95th percentile of 1.24–1.28. Per seed, 1 of 5 runs on each noise model is above that range at p < 0.05. The old headline (`0665072`) already had one seed at 1.254.
- **Pooled over the five seeds, the gap is real but under the limit:** 1.15 on dev noise and 1.13 on held-out noise, against a no-bias pooled median of about 1.05 (p = 0.002 and 0.02).
- **Where it comes from:** patient-link review items. Pooled link items per 100 loops (Arab / Filipino / South Asian / Western) are 49.1 / 49.3 / 47.9 / 42.8 on dev noise and 43.5 / 39.9 / 40.7 / 38.2 on held-out noise; evidence items differ by at most 1.7. Western names get the fewest link reviews on both noise models.
- **Traced (two causes, one per group):**
  - *Arab and South Asian: placeholder birth dates.* In the person data, 24.8% of Arab and 16.0% of South Asian persons have a 1 Jan or 1 Jul birth date, against 6.2% Western and 6.5% Filipino. A placeholder date cannot serve as the second identifier for a message, so the outreach guard asks a human: 9.9 (Arab) vs 5.6 (Western) guard items per 100 loops on dev noise, 10.1 vs 4.1 on held-out. A placeholder date also corroborates a link only on a full name match: 2.5–3.1 vs 1.3–1.5 items. Without the placeholder-date items, Arab and South Asian load drops to the Western level.
  - *Filipino: surnames that do not match token for token.* Corroboration required the last surname skeleton to agree. Exact-DOB, same-sex links that stayed at the 0.989 cap: 7.0 vs 3.4–4.9 items per 100 on dev noise, 7.2 vs 5.0–6.0 on held-out. Most of them are cross-script pairs whose transliterated surname differs by one consonant class (Bacalso / باسالسو), which hits non-Arabic names more; the rest differ by a maternal middle surname, a hyphenated married name or the token order.
  - *Not a lever for the gap:* about 65% of the placeholder-date guard items are on links where both registrations carry the same Emirates ID. Counting a matching EID as the second identifier would remove about 12 review items per 100 loops for every group (about 20% of all review) but leaves the ratio unchanged (1.16 / 1.13). It is also untestable here: the generator never produces a wrong EID, so the cohort cannot show the rule is safe.
  - Both causes come from deliberate safety rules (a shared birth date or a partial surname cannot identify a person). The next section relaxes the surname rule and adds the Emirates ID where the evidence allows.
- **It is reviewer time, not accuracy:** 0 wrong-patient messages and 0 false closures in every origin.

The aggregate now reports the pooled ratio and both no-bias references (`scripts/origin_null.py`, `results/final/*/origin_null.json`). The pipeline is unchanged.

### Identity rules: surname variants and the Emirates ID as second identifier (`03e430d`, new headline)

Post-hoc change after the `a3ad83a` freeze, made after the review-load trace above. Disclosed.

- **Surname variants (`-corrshared`, `-corrfuzzy`).** An exact-DOB, same-sex link also corroborates when the names share two distinct skeleton tokens of 3+ letters in any order (maternal middle surname, hyphenated or reordered name), or when the last surname skeletons (4+ letters) are within one edit (transliteration). Checked before the run on the cal split (persons held out from the eval cohorts; cohorts 101-107, both noise models): **0 additional false corroborations** among 1.7M candidate pairs of different persons (3 and 15 with the old rule, unchanged); +356 / +912 true pairs corroborated.
- **Emirates ID as the message second identifier (`-eidsecond`).** The Joint Commission and JCI accept an assigned ID number as one of two identifiers, and DoH pairs the Emirates ID with the name; no public Emirates ID error rate exists, and the Luhn check cannot catch a valid number that belongs to someone else ([`docs/research/eid-second-identifier.md`](../docs/research/eid-second-identifier.md)). So the number counts only when it is the same 784-prefixed, Luhn-valid, non-default ID on both registrations, sex and names agree, the birth dates do not contradict (equal, or the same year when one is a placeholder), and no other registration carries it with a different sex or name. Unit tests cover a relative's card, a reused number, a default ID and contradicting dates. **Limit:** the headline evaluation used here does not give a person someone else's Emirates ID, so it cannot measure the rule's safety under that stress; a relative with the same sex and surname still passes (the residual risk). The later post-hoc B8 evaluation below adds wrong-EID cohorts and separately measures the optional closure guard.

| | Dev noise v1: `a3ad83a` → `03e430d` | Held-out v2: `a3ad83a` → `03e430d` |
|---|---|---|
| Review items per 100 loops | 58.7 → **52.6** | 52.0 → **47.1** |
| Reviewer hours per 1k loops | 19.1 → **17.3** | 17.2 → **15.7** |
| Messages released (all with granted consent) | 7,278 → 8,085 | 7,138 → 7,656 |
| CRC false closures | 0 / 2,185 → 0 / 2,185 | 0 / 2,061 → 0 / 2,063 |
| Wrong-patient messages | 0 / 12,421 → 0 / 12,409 | 0 / 11,685 → 0 / 11,683 |
| CRC recall | 0.761 → 0.761 | 0.703 → 0.704 |
| Cervical / lung recall | unchanged | unchanged |
| Review-load ratio, per seed (95% CI) | 1.19 (1.11–1.28) → 1.19 (1.11–1.27) | 1.18 (1.05–1.31) → **1.16 (1.08–1.24)** |
| Review-load ratio, pooled over seeds | 1.15 → 1.14 (p = 0.005) | 1.13 → 1.12 (p = 0.03) |

**Result.** Reviewer time falls about 10% and more patients get their message without waiting for a human; every safety count stays at 0. The origin gap barely moves, as expected: the rules help every origin, and the remaining gap is the placeholder-date effect for Arab patients, which neither rule targets. Raw runs are kept outside the repo.

**Side issue found:** `noisyLatin2` (noise v2) panics on one cal-split name at seed 101 (slice bounds in `noise2.go`). The eval cohorts are not affected; it is left unfixed because changing the held-out noise model would change the held-out cohorts. Seed 107 was used instead for the cal-split check.

### Hand-off packet trace and fixes (`87ae618`, `e0b86c9`, `eea6f26`, new headline)

At `03e430d` the frozen run missed 51 packets that truth has (17 dev, 34 held-out) and built 38 that truth lacks (16, 22). Every mismatch was traced. Post-hoc changes, disclosed.

| Class | Loops | Cause | Fix |
|---|---|---|---|
| Missed: the re-referral was rejected again | 28 | The packet took the latest rejection, the re-referral's, so the original packet vanished | The packet uses the rejection the e-consult was about; a later rejection by another unit counts as the referrer acting |
| Missed: the unit's rejection never linked | ~23 | The unit's rejected intake was not tied to the patient, so no rejection was visible | The e-consult question is keyed by the referral; it now carries its trigger and the unit asked, and the packet is built from the thread (reason "other") |
| Extra: e-consult about a repeat after inadequate prep | 20 | That e-consult was paired with an older rejection that a later acceptance had already superseded | A packet needs an e-consult about a returned referral |
| Extra: redirect, then rejected, after an unanswered referral | 18 | **Truth was wrong**: the generator wrote packets only after a rejected referral, while `handoff.go` treats both as final | Generator: a redirect the second unit rejects is final in every branch |

The first re-run missed 71 packets per noise model: a multi-turn e-consult's "followup" question overwrote the opening "rejected" trigger. Fixed in `e0b86c9` (a follow-up turn keeps the thread's opening trigger), with a test through the closure core that fails without the fix.

The second re-run left 5 mismatches:
- **4 missed (linkage limit, not fixed):** a redirect where the second unit's rejection was never linked to the patient. The e-consult thread ends at the redirect, so nothing on the referrer's side shows the rejection.
- **1 extra (truth corrected, `eea6f26`):** the unit's reply came after the run ended, so at the end the e-consult had gone 5 business days unanswered, a `reject_no_reply` packet by the rule. The generator had decided the late reply, so truth had no packet. Truth now carries the packet when the deadline passes before a reply that falls after the run. No random draws: on seed 41 the FHIR files are byte-identical and truth differs only in that loop.

| | Dev noise v1: `03e430d` → `eea6f26` | Held-out v2: `03e430d` → `eea6f26` |
|---|---|---|
| Truth packets | 223 → 230 | 210 → 223 |
| Packets built and matched | 206 → **229** | 176 → **220** |
| Missed / extra | 17 / 16 → **1 / 0** | 34 / 22 → **3 / 0** |
| Referrer's action state correct | 206 → 229 | 174 → 220 |
| Closed after re-referral (false) | 66 (0) → 67 (0) | 44 (0) → 45 (0) |
| CRC / cervical / lung false closures | 0 / 0 / 0 | 0 / 0 / 0 |
| Wrong-patient messages; sent without consent | 0; 0 | 0; 0 |
| CRC recall | 0.761 → 0.762 | 0.704 → 0.704 |
| Reviewer hours per 1k loops | 17.3 → 17.3 | 15.7 → 15.7 |
| Review-load ratio, per seed; pooled | 1.19 (1.11–1.27); 1.14 → 1.19 (1.11–1.26); 1.14 | 1.16 (1.08–1.24); 1.12 → 1.17 (1.10–1.23); 1.13 |

Truth gains 7 and 13 packets from the two generator corrections. Small differences in the other rows are run-to-run variation (the chaos and timing schedule differs between runs).

### Noise fix, MTSamples key correction, short surnames (`6245613`, `bb3728e`, `f1848c8`, `e4c7553`, new headline)

Four post-hoc changes, disclosed.

1. **Held-out noise model fix (`6245613`).** `noisyLatin2` searched a lower-cased copy of the name and used the index on the original. Lower-casing "İ" changes its byte length, so the index was off and the generator could panic; initials were cut at one byte, which garbled non-ASCII first letters. It now matches case-insensitively in place and cuts initials by rune. Dev noise (v1) is unchanged.
2. **MTSamples key correction (`bb3728e`, `f1848c8`).** Three reports (u2, u10, u11) describe colon biopsies with no polyps. The key said they close without histology; they must wait for histology, which is what Wasl did. Each of these waits was counted as a miss, and a reviewer simulated from the key could copy the wrong answer into a closure. The key now marks them `awaiting_histology`. Four reports (u1, u6, u9, u40) gained `tissue_taken: yes` with no change of outcome. u4 is `tissue_taken: no`: the tissue was gastric, and the readers misread it as colonic (a known reader weakness). Every changed entry carries a `key_note`. These are our corrections; the human double-labelling of the 43 reports is still pending.
3. **Short surnames (`e4c7553`).** The first run on the corrected key released 1 wrong-patient message: two different people with exact-DOB agreement and the same 2-letter surname skeleton, which the surname rule counted as corroboration. A surname of 1–2 letters (after skeleton reduction) now corroborates only when the given name's skeleton also agrees. On the calibration split, false corroborations fell 3 → 0 (v1) and 17 → 11 (v2); true corroborations fell 5.0% and 3.4%. A stricter rule (all name tokens equal) was rejected: it cost about 15% of true corroborations.
4. **Review-load ratio.** The key correction changes the cohort's random draws, so the persons in each seed are reshuffled. After the reshuffle the held-out per-seed ratio is 1.28 (1.11–1.45), above the 1.25 limit; pooled over seeds it is 1.19. The small Filipino group moved most: 44.4 → 51.7 items per 100 loops on held-out noise, but 54.3 → 52.1 on dev noise. Traced in the next section.

| | Dev noise v1: `eea6f26` → `e4c7553` | Held-out v2: `eea6f26` → `e4c7553` |
|---|---|---|
| CRC recall | 0.762 → **0.809** | 0.704 → **0.737** (0.724–0.750) |
| CRC / cervical / lung false closures | 0/2,187, 0/1,023, 0/1,506 → **0/2,287, 0/1,020, 0/1,506** | 0/2,063, 0/987, 0/1,463 → **0/2,090, 0/991, 0/1,463** |
| Wrong-patient messages; sent without consent | 0; 0 | 0/11,682; 0/7,539 → **0/11,846; 0/7,544** |
| KPI error, Wasl vs EID-only | – | 0.029 vs 0.191 → 0.034 vs 0.197 |
| Reviewer hours per 1k loops | 17.3 → 17.4 | 15.7 → 15.9 |
| Hand-off packets matched / truth (extra) | 229/230 (0) → 229/230 (0) | 220/223 (0) → 233/236 (0) |
| Closed after re-referral (false) | 67 (0) → 76 (0) | 45 (0) → 71 (0) |
| Review-load ratio, per seed; pooled | 1.19 (1.11–1.27); 1.14 → **1.24 (1.15–1.32); 1.18** | 1.17 (1.10–1.23); 1.13 → **1.28 (1.11–1.45); 1.19** |

The recall gain comes from the key correction. Where recall still goes on held-out noise (869 of 2,932 truly closed loops missed at `eea6f26`): about 230 were the key error (now fixed), about 140 the simulated day-15 clinician error, 252 linkage misses and about 240 other legitimate waits.

### Review-load trace and fixes (`6907f6e`, `3caa8a1`, new headline)

At `e4c7553` the held-out per-seed ratio was 1.28 (1.11–1.45). Traced on the frozen runs; two post-hoc changes, disclosed.

**What raised it: chance pathway mix in the smallest group.**
- The short-surname rule is not the cause. Re-running the five held-out cohorts with it off lowered every origin by about 1 item per 100 loops; Filipino stayed high.
- The key correction changes the generator's random draws. Each origin keeps its loop count, but loops get new pathways. Filipino CRC loops in "awaiting histology" (the heaviest pathway, about 85 items per 100 loops) went from 16% to 26%; other origins moved 2–4 points. The generator draws the pathway independently of the name.
- Diagnostic (not a criterion): divide each origin's items by the items its programme-and-pathway mix predicts. This adjusted ratio was 1.18 at `eea6f26` and 1.21 at `e4c7553`. Arab is above prediction in every seed (1.01–1.08): the placeholder-date effect. Filipino swings both ways (0.86–1.12): small-group noise.

**What Wasl could fix.**
1. *Repeated second-identifier questions (`6907f6e`, `3caa8a1`).* A human's "same" on the outreach guard is double-read and is about two registrations, but it counted only on the loop where it was asked. Other loops of the same patient asked again: 1.4 (dev) and 2.1 (held-out) items per 100 loops, in every origin. The linker now applies the answer to every loop of the pair, and a later "different" withdraws it (`-reuseconfirm`). The guard items of a patient's loops are raised together, so the review worklist also drops a queued twin once its pair is answered "same" (about 75 per run). Tests: `TestReuseIDConfirmAcrossLoops`, `TestIDWorklistDedupe`.
2. *Bigger smallest group (`6907f6e`).* The evaluation now uses every held-out person (8,871, was 6,500) with 1.36× the loops: 2,040 CRC, 816 cervical and 1,088 lung loops per run. Filipino has about 360 loops per run instead of about 250. More is not possible without moving dev or calibration persons into the held-out set, which we did not do.

**What it could not fix.** The remaining gap is the outreach guard on placeholder birth dates (+2.2 to +3.3 items per 100 loops for Arab vs Western). A placeholder date and a common full name can be two people, so the guard stays.

| | Dev noise v1: `e4c7553` → `3caa8a1` | Held-out v2: `e4c7553` → `6907f6e` → `3caa8a1` |
|---|---|---|
| Loops per run | 2,900 → 3,944 | 2,900 → 3,944 → 3,944 |
| Review-load ratio, per seed | 1.24 (1.15–1.32) → **1.13 (1.06–1.20)** | 1.28 (1.11–1.45) → 1.21 (1.07–1.35) → **1.21 (1.09–1.34)** |
| Review-load ratio, pooled (p vs shuffled labels) | 1.18 (< 0.001) → **1.08 (0.12)** | 1.19 (< 0.001) → 1.10 (0.05) → **1.08 (0.11)** |
| Seeds above 1.25 | – → 0 of 5 | – → 2 of 5 → 2 of 5 |
| Adjusted ratio (diagnostic), per seed | – | 1.21 → 1.14 → – |
| Review items per 100 loops | 53.2 → 52.9 | 48.8 → 49.7 → **47.3** |
| Reviewer hours per 1k loops | 17.4 → 17.2 | 15.9 → 16.2 → **15.5** |
| CRC recall | 0.809 → 0.810 | 0.737 → 0.747 → 0.747 |
| CRC / cervical / lung false closures | 0/3,034, 0/1,403, 0/2,026 | 0/2,886, 0/1,300, 0/1,899 |
| Wrong-patient messages; sent without consent | 0/17,809; 0/11,197 | 0/16,743; 0/10,620 |
| Hand-off packets matched / truth (extra) | 330/333 (0) | 296/300 (0) |

Counts in the last four rows are at `3caa8a1`. The `6907f6e` run applied the reuse in the linker only, before the worklist dedupe; its review items rose because the cohorts are larger and the twins were still asked. The `e4c7553` → `3caa8a1` change mixes the code fix with the larger cohorts; per-loop rates are comparable, totals are not.

### Reader v7.2: upper-GI tissue in combined reports (`dc7fee2`, new headline)

Post-hoc reader change, disclosed. Every MTSamples u4 loop (an EGD and a colonoscopy in one report) waited for histology and never closed: 128 per noise model at `3caa8a1`.

- **Cause.** The regex reader's tissue check matched "Several biopsies were taken" (antral mass), "polyps, which were removed with biopsy forceps" (fundus) and "Biopsy was also taken for CLO". All three are gastric; the colon was normal. Gemma read no colon tissue, the readers disagreed, and a disagreement on tissue holds the loop for histology.
- **Fix 1.** Only in a report that names an upper-GI procedure (EGD, OGD, gastroscopy, upper endoscopy), each sentence gets a site: upper-GI when it names only stomach, duodenum or oesophagus sites, colon when it names a colon site, otherwise the site of the sentence before it. A tissue match in an upper-GI sentence no longer counts. A sentence naming both sites, or one before any site is named, still counts, so a mistake costs a wait, not a false closure.
- **Fix 2.** Fix 1 exposed two colon phrasings that v7.1 only caught because a gastric biopsy in the same report also matched: "sigmoid colon … biopsied (x4)" (corpus 4 q275) and "cold-snare excision of a … colon polyp; specimen recovered" (corpus 8 v054). The tissue check now also matches "biopsied" (unless negated earlier in its sentence: "no tissue was resected or biopsied"), "excision" or "snare" before "polyp", and "polyp … recovered".
- **Checked before the run** on all eight corpora and MTSamples, both cached second readers (`results/v7.2-reader-scores/`): 72 regex tissue answers change (66 no → yes, 6 yes → no), all 72 toward the key, 0 against. Unsafe auto-closures stay 0 in all 17 corpus × reader runs; correct auto-decisions rise on corpora 2, 3, 4 and 8 (for example corpus 8 with the 26B: 96 → 100). The fair-test counts reported for earlier readers are unchanged; corpora 4 and 8 have now been seen by this change, so they are no longer fair tests of v7.2. Tests: `TestTissueIgnoresUpperGI`.

| | Dev noise v1: `3caa8a1` → `dc7fee2` | Held-out v2: `3caa8a1` → `dc7fee2` |
|---|---|---|
| u4 loops closed / truly closed | 0 / 132 → **121 / 132** | 0 / 124 → **111 / 124** |
| CRC recall | 0.810 → **0.842** (0.825–0.858) | 0.747 → **0.775** (0.762–0.789) |
| CRC false closures | 0 / 3,034 → 0 / 3,153 | 0 / 2,886 → 0 / 2,997 |
| Cervical / lung false closures | 0 / 1,403; 0 / 2,026 | 0 / 1,300; 0 / 1,899 |
| Wrong-patient messages; sent without consent | 0 / 17,802; 0 / 11,093 | 0 / 16,767; 0 / 10,611 |
| Reviewer hours per 1k loops | 17.2 → 17.4 | 15.5 → 15.6 |
| Review-load ratio, per seed; pooled (p) | 1.13 (1.06–1.20); 1.08 (0.12) → 1.13 (1.06–1.20); 1.08 (0.10) | 1.21 (1.09–1.34); 1.08 (0.11) → 1.22 (1.10–1.35); 1.09 (0.10) |
| Hand-off packets matched / truth (extra) | 330 / 333 (0) | 296 / 300 (0) |

The reader now agrees on u4 (both say no colon tissue). The 24 u4 loops that still do not close end open (13), inadequate (8) or awaiting histology (3); they come from the same other causes as elsewhere (linkage misses, the simulated reviewer and day-15 clinician error rates) and were not traced one by one.

### Backlog gaps: own-programme outreach, subgroup check, ingest tiers (`194f355`…`c2a74e2`, new headline)

Post-hoc changes, disclosed. These close items earlier left out of scope. The frozen evaluation was re-run at `c2a74e2` (same cohort: every held-out person, 3,944 loops per run).

- **AI-use disclosure (`194f355`).** DoH AI Policy 3.1.6.1. Every patient message template now has a v2 that says the reminder comes from an automated follow-up service and that a care team member reads the replies (English and Arabic, teach-back variants too). v1 stays in the approved set so already-sent messages still verify. Tests: `TestTemplatesDiscloseAutomation`.
- **Outreach only for the loop's own programme (`f17f7a1`), a bug found by the new subgroup check.** A woman in both a CRC and a cervical loop got each appointment, no-show and rejected-referral message twice: once from each loop. In the final loop states, outreach entries fell by 1,055 on dev noise (CRC loops 576, cervical loops 479) and 946 held-out (524, 422); lung loops are unchanged. Messages checked for wrong patient fell 17,802 → 14,247 (dev) and 16,767 → 13,479 (held-out). Tests: `TestOutreachOnlyForOwnProgramme`.
- **Review load and recall by age band and sex, CRC loops (`f17f7a1`).** `scripts/subgroups.py`, pooled over seeds with the same within-seed shuffled-label test as the origin ratio. Age bands <40, 40–59, 60–79, 80+.

| CRC loops | Dev v1: `dc7fee2` → `c2a74e2` | Held-out v2: `dc7fee2` → `c2a74e2` |
|---|---|---|
| Age band, max/min review load (p) | 1.057 (0.23) → 1.058 (0.23) | 1.062 (0.24) → 1.056 (0.33) |
| Sex, max/min review load (p) | 1.034 (0.15) → 1.009 (0.67) | **1.094 (< 0.001)** → 1.050 (0.042) |
| Sex, items per 100 loops F / M | 71.2 / 68.9 → 68.3 / 69.0 | 65.5 / 59.9 → 62.9 / 59.9 |
| Recall by group | 0.82–0.86, unchanged | 0.76–0.79, unchanged |
| False closures | 0 in every group | 0 in every group |

The fix removed most of the held-out sex gap. The rest (about 3 items per 100 loops, p = 0.04) was traced: women who also have a cervical loop get about 4 more link reviews (skmc-endo ServiceRequest) per 100 CRC loops; women without a cervical loop match men. More registrations give more identity questions. It costs reviewer time, not recall, and was not changed.
- **Same report under two ids (`a1cafce`).** A result can arrive once over ORU and once over FHIR with different ids. The linker keys DiagnosticReports on content (category, focus, day, coded result, text hash) and keeps the first. Counter `duplicate_report_other_id`: 0 in every evaluation run (no false merges on the cohort). Tests: `TestCrossTransportDuplicateReport`.
- **Local Gemma 3 4B on the patient-reply sets (`adfb3a7`).** Teach-back: 62/100 correct (95% CI 0.52–0.71), **19/50 missed escalations**; e-consult: 75/100, 0/80 unsafe accepts. The 26B scores 98/100 with 0/50 missed. The 4B must not parse teach-back replies; the pipeline reads only the 26B cache and sends a miss to a human, so no code change was needed.
- **UAE public holidays (`5fe2d5d`).** Hand-off business days skip the 2026 federal holidays. Tests: Eid case in `handoff_test.go`.
- **FHIR polling with nightly Bulk Data catch-up (`c2a74e2`, `-source fhirpoll`).** For a sending system with no subscriptions: `_history?_since` polling with a watermark, plus a nightly `$export` from the watermark minus 7 days. The mock HIE makes 662 versions visible late. Crash test with the poller killed: 0 lost, 0 duplicated, 130 versions recovered by `$export`. Negative control (`WASL_EXPORT_OFF=1`): 133 lost. Results: `results/fhirpoll/`.
- **Sustainability estimate (`3f7f87f`).** About 0.73 kWh and 0.34 kg CO₂ per 1,000 loops, mostly reader GPU time. `docs/research/sustainability.md`.

| | Dev noise v1: `dc7fee2` → `c2a74e2` | Held-out v2: `dc7fee2` → `c2a74e2` |
|---|---|---|
| CRC recall | 0.842 → 0.842 (0.825–0.858) | 0.775 → 0.775 (0.762–0.789) |
| CRC false closures | 0 / 3,153 | 0 / 2,997 |
| Cervical / lung false closures | 0 / 1,402; 0 / 2,026 | 0 / 1,300; 0 / 1,899 |
| Wrong-patient messages; sent without consent | 0 / 14,247; 0 / 9,001 | 0 / 13,479; 0 / 8,678 |
| Reviewer hours per 1k loops | 17.4 → 17.0 | 15.6 → 15.2 |
| Review-load ratio, per seed; pooled (p) | 1.13 (1.05–1.22); 1.08 (0.11) | 1.21 (1.10–1.33); 1.08 (0.13) |
| Hand-off packets matched / truth (extra) | 330 / 333 (0) | 296 / 300 (0) |


## Third loop type: diabetic retinopathy screening to eye-clinic exam (`ee99e76`…`4cafa26`, new headline)

**Claim tested** (as for lung): one engine closes every follow-up loop; a new loop type is a rule table, two readers and a generator. Design: `docs/research/dr-loop-design.md`.

**Rule (built, `drrule.go`).** `doh2024dr`: who is referable comes from the DoH Abu Dhabi *Standard for Diagnosis and Management of Diabetes Mellitus Type 1 and 2* (DOH/ST/SDMDMT/V1/2024, cl. 3.13.85: any macular oedema, moderate or worse non-proliferative, any proliferative disease; 3.13.81: R-grade wording, annual repeat). DoH sets no deadline in weeks, so the deadlines are borrowed from the NHS Diabetic Eye Screening Programme Pathway Standards (2018): R3A (or an urgent request) seen within 42 days, R2/M1/R3S within 91. Our own choices, no source: ungradable images get a repeat photo within 28 days; treated stable proliferative disease (R3S) is routine; R0/R1 without maculopathy rescreen in 12 months (the DoH 1–2 year option needs HbA1c, which the data lack). `drrule_test.go` covers every cell.

**States (built, `dr.go`)**, same gates as the other programmes: R0/R1 → `no_referral_needed`; U → open with a 28-day repeat-photo clock, a later graded screen decides; referable → closes only on a trusted-link eye-clinic note that both readers (or a clinician) read as a retinal exam with an outcome; did not attend → `no_show` (rebook outreach); vision checked only → stays open; no fundal view → `not_assessable` (clinician). Overdue clock = grading day + deadline. Tests: `dr_test.go` (one per state, wrong-patient links, unread notes blocking later ones, an exam before the deciding grade not counting).

**Readers.** Reader A regex, reader B local Gemma 3 4B (cached, quote-grounded). A coded grade from grading software is a second reader that must equal reader A's reading of the summary text, never a single source. One shared rule turns fields into a label; disagreement or an ungrounded quote → review.

| Corpus (synthetic) | Author | Role | Screening: auto-decided / agreed-but-wrong | Clinic notes: auto-decided / agreed-but-wrong |
|---|---|---|---|---|
| Dev, 34 + 50 (`scripts/dr_corpus.py`) | this work | written against | v1: 23/34 / 0 | v1: 44/50 / 0 |
| Held-out 1, 40 + 50 (`scripts/dr_heldout/`) | Gemini 4 Argon | fair test of v1, then tuning | v1: **0/40** / 0 | v1: 22/50 / 0 |
| Held-out 2, 40 + 50 (`scripts/dr_heldout2/`) | GPT-6.1 Sol | **fair test of v2** | v2: **20/40 / 1** | v2: **20/50 / 1** |

- v1 failed held-out 1 safely: reader A was overfitted to the dev layouts (0/40 screens readable), so every free-text screen would have gone to a human. Held-out 1 became tuning data for v2 (regex rewrite; Gemma prompt v3 → v5: attendance wording, urgency only from a quoted sentence that really says "urgent", because the 4B over-called a yes/no urgency field). The Gemma prompt was revised twice on dev (v1 → v3) and twice on held-out 1 (v3 → v5).
- The fair test of v2 (held-out 2, a third model family, committed before scoring) has 2 agreed-but-wrong labels, and neither changes a decision: hs39 both readers R3AM1, key R1M1/urgent (same action: urgent referral, 42 days); hc29 both "examined, follow-up", key "examined, discharged" (both close the loop: the retina was examined). Reader A's main remaining error is negated lists ("No microaneurysms, haemorrhages … or new vessels" read as new vessels → R3A): over-grading, the safe direction, to review.
- What this means in practice: on unseen text about half the free-text screens and 40% of clinic notes would auto-decide; the rest go to a reviewer. The generator draws its free text from the dev corpus, so the integrated runs below overstate agreement; the held-out-2 row is the honest estimate. Results: `results/dr-readers/`.

**Generator (built, `gendr.go`; cohort simulated).** 60% coded-grade sites; grade mix ungradable 5%, gradable R0 81% / R1 10% / routine referable 6.5% / R3A 2.5% (any retinopathy ≈ 19%, Al-Ain survey: Al-Maskari & El-Sadig, *BMC Ophthalmology* 2007, 7:11); referable patients examined within the deadline 50%, late 25%, never 25%; DNA, vision-only and no-view visits; distractor notes for other diabetics. Own RNG stream, background persons with no other loop, emitted last: `-dr 0` leaves every other feed byte-identical (`TestGenDRLeavesOtherProgrammesUnchanged`). Consent: the screening clinic records it for every DR patient (4% deny), own RNG.

**Integrated run** (seed 2026, 1,200 DR + 800 lung + the screening cohort, chaos on): DR P 1.000, R 0.997, **0/1,121 false closures**; referral closures P 1.000, R 0.968; triage (rule action and deadline) 0.999; overdue flags 0.998; exam within the deadline truth 0.419 vs Wasl 0.398. No-change check against the same run with DR off: no-review metrics identical for CRC, cervical and lung; lung identical with review too; 2 loops differ, both one woman's CRC and cervical loops. With DR on the simulated reviewer answers later, so both loops raised a link question on the same registration pair; one answer was "same", the other "different" (the assumed 4% error), and the later "different" split the pair. Both ended open, not closed: safe direction. This is an existing gap, not a DR bug: link questions from two loops on one pair are not deduplicated (second-identifier questions are, since `3caa8a1`). Fixed later at `d3a1e64` (one link question per registration pair, below). `results/dr/`.

**Frozen evaluation at `4cafa26`** (same seeds and held-out persons as `c2a74e2`; 1,200 DR screens per run, so 5,144 loops per run instead of 3,944; files in `results/final/`):

| | Dev noise v1: `c2a74e2` → `4cafa26` | Held-out v2: `c2a74e2` → `4cafa26` |
|---|---|---|
| CRC recall | 0.842 → 0.842 (0.825–0.858) | 0.775 → 0.775 (0.761–0.789) |
| CRC / cervical / lung false closures | 0 / 3,153; 0 / 1,402; 0 / 2,026 | 0 / 2,995; 0 / 1,300; 0 / 1,899 |
| **DR false closures** (referral loops) | **0 / 5,621** (0 / 254) | **0 / 5,608** (0 / 214) |
| DR recall; referral recall | 0.998; 0.977 (0.956–0.998) | 0.994; **0.880 (0.792–0.968)** |
| DR triage (action and deadline); overdue flags | 0.999; 0.998 | 0.999; 0.995 |
| DR exam within deadline, truth vs Wasl | 0.351 vs 0.343 | 0.418 vs 0.368 |
| Wrong-patient messages; sent without consent | 0 / 15,179; 0 / 9,954 | 0 / 14,343; 0 / 9,438 |
| Reviewer hours per 1k loops | 17.0 → 14.7 | 15.2 → 13.3 |
| Review-load ratio, per seed; pooled (p) | 1.13 → 1.12 (1.07–1.18); 1.08 → 1.06 (0.28) | 1.21 → 1.19 (1.04–1.34); **1.08 → 1.11 (0.023)** |
| CRC review load by sex, max/min (p) | 1.009 → 1.011 (0.63) | 1.050 → 1.054 (0.028) |

- **Other programmes.** CRC, cervical and lung recall are unchanged. Held-out CRC closures moved 2,997 → 2,995 because the shared simulated reviewer answers in a different order (the gap described under the integrated run). Reviewer hours per 1,000 loops fall only because DR loops need few reviews (13 items per 100 loops vs 44–47 for the others).
- **Readers in the runs.** Screens: 5,513/6,210 (dev) and 5,523/6,207 (held-out) auto-decided, 60% with a coded grade; clinic notes 1,087/1,102 and 1,068/1,090; 0 agreed-but-wrong. The generator reuses dev-corpus text, so these overstate agreement (see the readers table).
- **Held-out referral recall 0.88, wide interval.** Fewer than 50 referral loops per run; the misses are loops left open for review, not closed wrongly.
- **Pooled origin ratio now significant on held-out noise (1.11, p = 0.023).** Traced (`reviewer_decisions` against truth, 5 held-out seeds). It is a mix effect, not a new per-programme gap. Without DR loops the ratio is 1.085, as before. Arab patients carry more of the DR loops (25.5% of their loops vs 20–23% for other origins), and DR loops need few reviews, so their pooled review rate drops below the others. Within DR loops the ratio is 1.26 (Filipino 16.3 items per 100 loops, n = 454; Arab 12.9, n = 3,531), on small groups. The per-seed mean (1.19) stays under the 1.25 limit, but its upper interval (1.34) crosses it. A per-programme comparison is the fairer measure once loop mixes differ by origin; not changed.

### Origin ratio by programme; one link question per registration pair (`d3a1e64`, new headline)

Post-hoc changes after the `4cafa26` evaluation, disclosed. The frozen evaluation was re-run at `d3a1e64` (same seeds and cohorts; files in `results/final/`).

**Programme-aware origin check (`scripts/origin_null.py`, `scripts/final_aggregate.py`).** Programmes carry very different review loads (CRC about 60 items per 100 loops, DR about 13), and the loop mix differs by origin, so the overall ratio partly measures the mix. Two checks were added after the significant `4cafa26` result was seen (a post-hoc change of the measure; the pre-declared 1.25 limit on the per-seed ratio is kept and still reported), pooled over seeds with origin labels shuffled within each seed and programme: an indirectly standardised ratio (per origin, observed items over the items expected from each of its loops' programme mean) and the ratio inside each programme.

| Pooled over 5 seeds, at `d3a1e64` | Dev noise v1 | Held-out v2 |
|---|---|---|
| Overall max/min (p) | 1.058 (0.29) | 1.103 (0.041) |
| **Standardised by programme** (p) | **1.100 (0.014)** | **1.037 (0.64)** |
| CRC (p) | **1.123 (0.017)** | 1.063 (0.38) |
| Cervical; lung; DR (p) | 1.09 (0.53); 1.18 (0.66); 1.25 (0.29) | 1.11 (0.37); 1.55 (0.06); 1.27 (0.25) |

- **Multiple comparisons.** These are about 16 subgroup tests per evaluation (overall, standardised and four programmes on two noise models, plus sex and age band). No p-value here passes a Bonferroni cut of about 0.003, so the significant values below are weak evidence, to be read as signals to watch, not established gaps.
- **Held-out noise:** no origin gap once the mix is accounted for (1.04, p = 0.64). The significant overall ratio is the DR mix effect traced in the previous section.
- **Dev noise: a possible gap the overall ratio hid (p = 0.014, weak; see above).** On CRC loops Arab patients get more review items than Filipino patients (pooled per 100 loops: identity questions 50.6 vs 44.9, evidence 18.0 vs 16.2). The CRC pathway mix is the same across origins (for example 23.6% vs 22.7% awaiting histology), so the extra items are within pathways, mostly identity questions. This matches the earlier trace (placeholder birth dates, which cannot corroborate a link, are commonest in Arab registrations), but this run does not prove that cause. It costs reviewer time, not accuracy: CRC recall and false closures (0 in every group) do not differ. The overall dev ratio (1.06) hid it because Arab patients carry more of the low-review DR loops. Not changed.

**One link question per registration pair (`-reuselink`, default on).** The linker applies a human link answer to the registration pair itself (merge, or cannot-link), so it already holds for every loop of the patient. But two loops of one patient could each queue a link question on the same pair before the first answer landed; both were asked, and when the simulated reviewer's assumed 4% error made the answers disagree, the later one won (the no-change check in the DR section found one such case). The review worklist now drops a queued link question once its pair has a human answer, either "same" or "different"; a later answer still replaces it. Second-identifier and evidence questions are unaffected. Test: `TestLinkWorklistDedupe`. Trade-off: every link answer is already a double read (`merge+close`), but a twin question used to be an accidental third look that could split a wrongly merged pair; it is now dropped. Wrong-patient messages and false closures stay 0 in all 10 runs, but the simulation's 4% error rate is an assumption, so this cost is not measured for real reviewers. In the 10 runs it dropped 47–77 link questions per run (about 3% of answered items).

| | Dev noise v1: `4cafa26` → `d3a1e64` | Held-out v2: `4cafa26` → `d3a1e64` |
|---|---|---|
| CRC recall | 0.842 → 0.842 (0.827–0.858) | 0.775 → 0.776 (0.762–0.790) |
| Cervical recall | 0.966 → 0.968 | 0.903 → 0.907 |
| CRC / cervical / lung / DR false closures | 0 / 3,155; 0 / 1,405; 0 / 2,026; 0 / 5,621 | 0 / 2,999; 0 / 1,305; 0 / 1,899; 0 / 5,608 |
| Wrong-patient messages; sent without consent | 0 / 15,158; 0 / 9,939 | 0 / 14,334; 0 / 9,482 |
| Review items per 100 loops; reviewer hours per 1k loops | 42.9 → 41.6; 14.7 → 14.3 | 38.6 → 37.3; 13.3 → 12.9 |
| Origin ratio per seed; pooled (p); standardised (p) | 1.12 (1.06–1.18); 1.06 (0.29); 1.10 (0.014) | 1.18 (1.02–1.34); 1.10 (0.041); 1.04 (0.64) |
| CRC review load by sex, max/min (p) | 1.011 (0.63) → 1.041 (0.09) | **1.054 (0.028) → 1.016 (0.55)** |
| CRC review load by age band, max/min (p) | 1.055 (0.26) → 1.058 (0.23) | 1.055 (0.34) → 1.063 (0.23) |

- **What changed and what is noise.** The solid effect is about 3% fewer review items, matching the 47–77 dropped questions per run. The recall changes (CRC +0.001, cervical +0.002 to +0.004) are inside their intervals: changing the order of the reviewer's answers alone moves results by that much. DR results are identical.
- **The held-out sex gap is gone, and traced.** Held-out CRC identity questions per 100 loops, `4cafa26` → `d3a1e64`: women who also have a cervical loop 49.5 → 43.9; other women 43.6 → 43.5; men 42.4 → 42.3. Only the women with two loops changed, so their two loops had been asking about the same registration pair.
- **Dev sex ratio rose** 1.011 (p = 0.63) → 1.041 (p = 0.09), not significant; most likely run-to-run noise. Not traced.

### Learn-then-Test re-run, and a confirming review for contradicting links (`0f6a532`, new headline)

**Why re-run.** Learn-then-Test had last run at `a5248c5` (28 Oct). Since then the identity rules changed (surname variants, Emirates ID as second identifier, short surnames, answer reuse). The calibration cohorts were regenerated with the current generator (same seeds 101–106, cal split, v2 noise).

**First re-run (`4b58c9b`) found one failure.** At the operating tier 0.995 with review: 1 false closure and 1 loop with a wrong-patient message in 12,600 (the tier still certified: α = 0.001). Both were one loop (cohort 103, `ahs-mushrif/ServiceRequest/AHS-sr-880`):
- A link question asked whether a woman registered as "ابتسام عبد الله", born 1 Jan 1972, was the same person as a man registered as "عبد الله", born 1 Jan 1973.
- Both simulated reads answered "same" (the assumed 4% error twice; about 0.16% per item). The records merged, the man's colonoscopy report joined her loop, a day-15 prep answer "adequate" closed it, and a message went to the wrong person.
- The gap: a human "same" was accepted even when the two records contradict each other. The simulated reviewer errs at a fixed rate whatever the evidence, so this is a stress test, not a measured clinician error; nothing enforced a second look.

**Fix (`0f6a532`, post-hoc, disclosed; flag `-linkconfirm`, default on).** A human "same" on a pair whose recorded sex differs, or whose real (non-placeholder) birth dates are more than 365 days apart, no longer merges. The loop raises a confirming link review (`link_contradiction_confirm`) that names the contradiction, and only that answer merges. Placeholder birth dates and unknown sex are missing data, not contradictions. It mirrors the override block (`OverrideConfirm`) and covers all four programmes. Tests: `TestLinkContradiction`, `TestContradictingLinkNeedsConfirmation`.

**Learn-then-Test at `0f6a532`** (files in [`results/ltt/`](results/ltt/LTT.md)):

| With review, τ 0.995 | `a5248c5` (old cohorts) | `4b58c9b` (no guard) | **`0f6a532`** |
|---|---|---|---|
| False closures / wrong-patient message loops (of 12,600) | 0 / 0 | 1 / 1 | **0 / 0** |
| Recall | 0.883 | 0.814 | 0.813 |
| Review items per 100 loops | 82.4 | 61.9 | 63.5 |
| Loosest certified tier | 0.5 | 0.8 | 0.8 |

The operating tier is still 0.995. Recall with review is lower than at `a5248c5` because ungraded-prep reports now wait for the unit's grade (as in the frozen evaluation); without review it rose from 0.29 to 0.40 (the identity rules). The guarantee still covers loops like the synthetic calibration set only.

**Frozen evaluation at `0f6a532`** (same seeds and cohorts; files in `results/final/`). The guard held 48–97 "same" answers per run for a confirming review. Most are probably true matches whose records disagree from data-entry noise, so they cost reviewer time; this evaluation did not count how many were wrong merges prevented.

| | Dev noise v1: `d3a1e64` → `0f6a532` | Held-out v2: `d3a1e64` → `0f6a532` |
|---|---|---|
| CRC recall | 0.842 → 0.840 (0.824–0.856) | 0.776 → 0.775 (0.761–0.788) |
| Cervical; lung; DR recall | 0.967; 0.986; 0.998 | 0.906; 0.949; 0.994 |
| CRC / cervical / lung / DR false closures | 0 / 3,147; 0 / 1,403; 0 / 2,025; 0 / 5,621 | 0 / 2,995; 0 / 1,303; 0 / 1,898; 0 / 5,608 |
| Wrong-patient messages; sent without consent | 0 / 15,094; 0 / 10,491 | 0 / 14,343; 0 / 9,473 |
| Review items per 100 loops; reviewer hours per 1k loops | 41.6 → 42.8; 14.3 → 14.7 | 37.3 → 38.2; 12.9 → 13.2 |
| Origin ratio per seed; pooled (p); standardised (p) | 1.11 (1.06–1.17); 1.05 (0.38); 1.08 (0.057) | 1.18 (1.02–1.35); 1.11 (0.032); 1.04 (0.65) |
| CRC by sex; by age band, max/min (p) | 1.046 (0.046); 1.052 (0.32) | 1.011 (0.70); 1.054 (0.37) |

- **Cost:** about 3% more review items; recall changes are inside run-to-run noise. Every safety count is 0, as before.
- **Subgroup signals** (weak evidence after about 16 tests; see the previous section): the dev CRC origin gap is now 1.10 (p = 0.058), the dev sex ratio 1.046 (p = 0.046), and held-out lung loops by origin 1.60 (p = 0.039; Western 7.5 vs Filipino 11.3 items per 100 loops at `d3a1e64`, small groups). None passes a Bonferroni cut; all cost reviewer time, not recall or safety.

## Fourth loop type, follow-up tracking and hand-off extensions (`77e82aa`…`cfcf50f`, new headline)

**What changed.** One merge of seven work branches. Each new behaviour has a flag; the "default" column says how the frozen evaluation below ran it. Post-hoc changes are marked.

| Item | Flag (default) | Post-hoc? |
|---|---|---|
| Breast screening recall loop (fourth loop type) | `-breast N` (gen; 70 per run in the frozen eval) | no |
| DR feeds over HL7 v2 | none (always on for `WASL_SOURCE=hl7`) | no |
| DR next due date and overdue recall | `-drnext` (off; on in the frozen eval) | no |
| Lung surveillance CT after a PET/CT low-uptake closure | `-lungsurv` (off; on in the frozen eval) | no |
| Lung and DR hand-off packets for rejected referrals | `-refreject` (gen, off; on in the frozen eval) | no |
| Rejected intake that cites the referral raises a link question | `-xreflink` (off; on in the frozen eval) | **yes** |
| Second-identifier "same" on contradicting records needs a confirming review | `-linkconfirm` (on) | **yes** |
| Contradiction check uses sex only; birth-date arm behind a flag | `-linkconfirmdob` (off) | **yes** |
| Offline review loop answers in the live reviewer's order | `-replayworklist` (on) | no |
| Lung and DR message templates | `-progtpl` (on) | no |
| API: DR fixture, closed flag for `no_referral_needed`, review_hold fallback, per-decision outreach | none | fixture fallback **yes** |

With every flag off, each generator feed is byte-identical to the previous generator (tests: `TestGenBreastLeavesOtherProgrammesUnchanged`, `genlungsurv_test.go`, `TestGenDRNextAddsOnlyFollowUpNotes`, `handoffext_test.go`).

### Breast screening recall to assessment outcome (`dafd5bc`…`9734c63`)

**Claim tested** (as for lung and DR): one engine closes every follow-up loop; a new loop type is a rule table, two readers and a generator. Design: [`docs/research/breast-loop-design.md`](../docs/research/breast-loop-design.md).

**Rule (`breastrule.go`, `doh2025breast`).** From the DoH *Standard for Breast Cancer Screening & Diagnosis* (DoH/ST/BCSD/ADPHC/V2/2025):
- BI-RADS 1/2 return to routine screening (3.8.2; every 2 years, Appendix 2).
- 0/3/4/5 are recalled to the diagnostic breast assessment unit (3.8.3).
- Clocks from Appendix 3 KPI 11 (working days × 7/5): outcome within 14 days for 0/3, 21 days for 4/5.
- When the breasts differ, the more worrying one decides, in the ACR BI-RADS 5th edition order 1 < 2 < 3 < 0 < 4 < 5.
- Coded values are the CPT II codes 3340F–3345F (Appendix 6).
- Our own choices, no source: BI-RADS 3 on a screen is a recall; a technically inadequate screen gets a 21-day repeat clock; 5 is urgent and 4 suspicious. BI-RADS 6 or an unknown label holds for a human.

**States (`breast.go`).** Same gates as the other programmes. A recall closes only on a trusted-link assessment document that both readers (or a clinician) read as an outcome: return to routine, early recall (flagged for audit, 3.9.10), or referred to treatment. Referral to treatment needs a malignant needle biopsy in the same document (3.9.11); the 10-working-day treatment-referral due day is recorded, not tracked. Did not attend → `no_show`; biopsy awaited or MDT pending → stays open. Radiology–pathology concordance (3.9.8) is not checked; it stays with the MDT.

**Readers.** Reader A is regex; reader B is the local Gemma 3 4B (cached, quote-grounded). A coded final assessment is a second reader that must equal reader A's reading of the summary text; it is never used alone.

| Corpus (synthetic) | Author | Role | Screens auto-decided / agreed-but-wrong | Assessment documents auto-decided / agreed-but-wrong |
|---|---|---|---|---|
| Dev, 39 + 50 (`scripts/breast_corpus.py`) | this work | written against | prompt v1 35/39 → v2 36/39 / 0 | v1 46/50 → v2 50/50 / 0 |
| Held-out, 40 + 50 (`scripts/breast_heldout/`) | GPT-6.1 Sol, committed before any reader ran | **fair test of v2** | **34/40 / 0** (coded path 20/20) | **38/50 / 0** |

- Reader A on held-out text: 38/40 screens, 38/50 assessment documents. It read all 10 cancer referrals worded "refer to the breast cancer surgical service" as incomplete (the safe direction).
- Gemma: 35/40 screens (2 quotes not found in the text), 50/50 assessment documents. It reads a technically inadequate image as BI-RADS 0, so those screens go to review. Prompts v3 and v4 scored worse on dev and were dropped.
- The readers were not tuned on the held-out set. The generator reuses dev text, so agreement in the integrated runs is optimistic; the held-out row is the honest estimate. Results: [`results/breast-readers/`](results/breast-readers/).

**Generator (`genbreast.go`, simulated).** Women 40–90 (Appendix 2 sets no upper age); 70% coded final assessment; recall 10%; outcome documented within the clock 60%, late 25%, never 15%; first-visit DNA 8%. All percentages are assumptions. **Capacity:** the held-out person pool (22% women, drawn after CRC, cervical and DR) allows about 85 breast loops per frozen-evaluation cohort, so the evaluation uses 70 (80 failed on seed 37, v2).

**Integrated runs** (seed 2026, chaos on; [`results/breast/`](results/breast/)):
- 160 breast + 800 lung + 1,200 DR: P 1.000, R 0.994, **0/154 false closures**; recall loops R 0.909 (10/11).
- Same run with breast off: every CRC, cervical, lung and DR metric identical; 0 of 4,189 other loops changed final state.
- 350 breast, lung and DR off: P 1.000, R 0.997, **0/333 false closures**; recall loops R 0.964 (27/28).
- Both misses were BI-RADS 0 recalls whose assessment document never linked; they stayed open (the safe direction).

### Diabetic retinopathy: HL7 v2 feeds and next due date (`8f694f4`, `433d72b`)

**HL7.** DR screening reports and eye-clinic notes travel as ORU^R01 results with local codes `dr-screen` / `dr-clinic`; the tracking order travels as ORM^O01. The listener rebuilds every DR resource from the segments, not from the backup FHIR copy inside the message. All 261 DR resources of a generated cohort give the same canonical events over HL7 as over FHIR, also with the backup copy's clinical fields deleted (`TestHL7DRRoundTripEquivalence`). An integrated run (600 DR loops, seed 2026, chaos on) gave a byte-identical DR block in `report7.json` over HL7 and FHIR: P 1.000, R 0.998, 0/558 false closures. No LOINC code is used: no verified code covers the R/M grade (LOINC LL2117-1 has no R/M or U answers), so the codes are local.

**Next due date (`-drnext`).** A resolved DR loop records when the patient is next due:
- R0/R1: rescreen at the grade day + 365 (doh2024dr).
- Discharged from the eye clinic: annual screening from the clinic day.
- Follow-up or treated: the interval the clinic note states, else 91 or 28 days. Both defaults are our assumptions, with no outside source.

Past the due day with no later read screen or linked, attended clinic visit, the decision is flagged `recall_overdue` (escalation `dr_recall_overdue`); the loop stays resolved. No patient message is sent for an overdue recall yet. In the 600-loop run the next due date matched truth for 558 of 559 resolved loops, and 4 of 4 overdue recalls were flagged with no false flags. The interval is read from the note by the rule reader only, and the generator uses the same function, so extraction accuracy is not independently tested. The follow-up attendance rates in the generator are assumptions.

### Lung surveillance CT after a PET/CT low-uptake closure (`c67fc45`)

With `-lungsurv`, a PET/CT low-uptake closure of a > 8 mm nodule opens a surveillance-CT child loop (`<loop>#surv`). BTS 2015 (Callister et al., *Thorax* 2015;70 Suppl 2) puts a Herder < 10% nodule under CT surveillance; the 60–120 day window after the PET is our policy.
- The parent stays `closed` (closer `pet`, `closed_into: surveillance_ct_due`) and carries the obligation as `lung.surveillance`.
- A later chest CT on a trusted link, read the same by both readers (or signed by a clinician), closes the child if stable or resolved, or escalates it to the lung MDT if growing. An untrusted link or reader disagreement raises a review item.
- Past the due day it is overdue (`overdue_lung_surveillance_ct`), with a reminder and a recall sent only with granted consent.
- Generator rates are assumptions: adherence within 45%, late 25%, never 30%; stable 82%, resolved 6%, growing 12%.

In the frozen evaluation (below), 0/41 dev and 0/30 held-out surveillance closures were false, and the overdue flag was correct for 98/99 and 85/87 obligations. **Open issue:** the pipeline opened 3 (dev) and 4 (held-out) surveillance loops with no matching obligation in truth. These sent no wrong-patient message (0 overall), but the cause is not yet traced.

### Hand-off: missed packets and lung/DR rejections (`d38ee30`, `b5375b8`)

**Missed packets (post-hoc, disclosed; `-xreflink`).** The frozen run at `0f6a532` missed 7 packets (3 dev, 4 held-out), all redirects that the second unit rejected:
- 4 were a linkage limit: the second unit registered the patient with a corrupted identity (a one-digit Emirates ID typo, a birth year off by 10), so its rejected intake never reached the loop.
- 3 were simulated reviewer errors: a wrong "different" on the second unit's record, or on the record it was linked through.

Every intake already names the referral it answers. With `-xreflink`, such a rejected intake raises an ordinary link question on the cited loop. Only a human "same" attaches it (with the confirming review when the records contradict); a "different" removes it for good; no question is asked when identity already points to another referral's patient. Re-running the five frozen cohorts with misses recovered all 4 linkage misses; the 3 reviewer-error misses remain, on purpose (a human "different" is not overridden). Cost: 28–39 extra link questions per held-out run (3–5 dev), up to 1.8% more reviewer time. CRC recall rose by 0.007–0.010 on held-out noise, because a confirmed record brings in the patient's later events at that unit. The generator never writes a wrong referral number, so these runs cannot test a wrong reference.

**Lung and DR hand-off (`-refreject`).** Lung > 8 mm work-up referrals and DR eye-clinic referrals can now be rejected. These programmes have no e-consult, so a rejection on a trusted link is final; the packet, act-by clock, escalation and referrer action reuse the CRC code. There is one receiving unit per programme, so no alternative is suggested. Assumptions: half of the never-examined referable loops had the referral rejected; every > 8 mm work-up is urgent; DR is urgent for R3A or an urgent request. "Patient informed" is always false for these programmes: their outreach has no referral-rejected message.

### Safety and review-loop changes (`d703fb4`, `5861b8a`, `0ab46d1`)

**Second-identifier confirm (post-hoc extension of `0f6a532`; `-linkconfirm`).** A second-identifier answer "same" on contradicting records used to release the held message and confirm the pair for the patient's other loops. It now follows the link rule: the answer is held and a confirming review (`second_identifier_confirm`) names the contradiction; only its "same" counts. No observed failure on this path prompted it; it was written by analogy with the link rule. On calibration cohort 103 at τ 0.995: 48 confirming reviews, all true matches (none prevented a wrong message); 3 wrong "different" answers kept true-patient messages held (the safe direction).

**What the birth-date arm of the contradiction check bought (trace, post-hoc).** Rebuilt from the feeds of the `0f6a532` frozen evaluation (the rebuild matches the linker counts exactly in all 10 runs):

| | Dev v1 | Held-out v2 |
|---|---|---|
| "Same" answers held for a contradiction (birth date / sex) | 402 (402 / 0) | 273 (273 / 0) |
| Held pairs that were truly different people | **0** | **0** |
| Confirming reviews answered | 354 | 237 |
| Confirmer said "different" on a truly same pair (simulated error) | 16 | 10 |

Every hold was a true match whose real birth dates differ by more than a year from data-entry noise. No reviewer answered "same" on a truly different pair in any run, so the guard had nothing to catch; 15 truly closed loops stayed open because of the 26 wrong confirmations. Of the 100 truly different pairs that reached link review, the rule would have flagged 10 (7 on sex, 3 on birth date). **Change (`0ab46d1`, post-hoc, disclosed):** the contradiction check now uses recorded sex only; the birth-date arm is kept behind `-linkconfirmdob` (default off). The guard still targets the rare double reviewer error found at `4b58c9b` (a woman and a man merged), which was a sex contradiction. Effect at the new code: review items per 100 loops 42.8 → 41.4 (dev) and 38.2 → 37.3 (held-out), with every safety count still 0 (table below).

**Offline review order (`-replayworklist`).** The offline review loop used by Learn-then-Test and the sweeps answered items in id order. It now answers in emission order and drops twin questions the way the live reviewer does. It is deterministic; cohort 103 at τ 0.995: 1,473 → 1,405 answered items (10 dropped), recall unchanged.

**Message templates (`-progtpl`).** Lung and DR outreach intents had no approved template. New families `lnfu` (remind, recall) and `drsfu` (repeat, remind, recall, rebook) in English and Arabic, v1 plus the v2 AI-use disclosure. They name no test or finding. The content guard also blocks lung and retina finding words. The 24 earlier template ids and their text are unchanged and pinned by a test. The retinopathy sender's display name was "Diabetes clinic, primary care", which reveals a diagnosis in every message; it is now "Primary care eye screening".

### API (`b30a8da`, `cfcf50f`)

- `programme=diabetic_retinopathy` and `programme=breast` are documented in [`docs/API.md`](../docs/API.md), with the `dr` object (including `next_due`), `lung.surveillance` and the `breast` object.
- **Fix:** DR loops in `no_referral_needed` (and breast loops in `no_recall_needed`) are served as `closed: true`, matching the metrics. Before, the API reported 1,061 of 1,200 DR loops in the older demo run as not closed.
- New demo fixture `dr_exam_closure`: the first DR loop by id where a referable screen was closed by an eye-clinic exam within the deadline, over a trusted cross-provider link, with both readers agreeing and no review anywhere in its history.
- **review_hold fallback (post-hoc):** in the new runs, 1 of about 2,100 CRC/cervical loops had consent held then granted, so the fixture often had no candidate. When none exists, it falls back to the first closed loop with signed link and evidence reviews whose outreach was stopped by the consent hold or the second-identifier guard. Each decision in the history now carries its own `outreach`, so this is visible in the response.
- [`docs/api-samples/`](../docs/api-samples/) is regenerated from a five-programme demo run: four fixtures and one case page per programme.

### Subgroup traces (read-only; scripts not committed)

**Origin and placeholder birth dates (dev, CRC, `0f6a532`).** Arab patients had 52.1 identity review items per 100 CRC loops vs 47.6 for Filipino patients. Placeholder birth dates (1 Jan / 1 Jul) are about twice as common among Arab patients: 28.9% of referral registrations vs 13.7% (25.0% vs 8.0% in the source person data). Loops with a placeholder referral birth date get about 20 more identity items per 100 in every origin. Within placeholder strata the gap falls to about 1–1.5 items per 100, and a permutation test with origin shuffled within seed and stratum no longer separates it from chance (one-sided p 0.24–0.36, vs 0.017 unstratified). Placeholder composition accounts for most of the gap. Caveats: only 126 Filipino loops in the placeholder stratum; one low Filipino seed; Arab vs Filipino is the post-hoc extreme pair.

**Held-out lung by origin (1.60, p 0.04 at `0f6a532`).** Review items arise only on loops that reach a follow-up event, and the generated cohort put fewer Western loops in those states (23.0% vs 27–32%; adherence is drawn independently of origin). Among those loops the load is 34–39 items per 100 (max/min 1.14, p 0.77). Attributed to cohort mix and small counts.

**Dev CRC by sex (1.046, p 0.046 at `0f6a532`).** Men carry about 3 more items per 100, higher in 4 of 5 seeds and not removed by adjusting for origin, placeholder birth date, pathway or registration count. The mechanism is not identified. With about 16 subgroup tests per evaluation (Bonferroni cut about 0.003), this is flagged for re-check, not claimed as an effect. At the new code it is 1.043 (p 0.076) dev and 1.017 (p 0.52) held-out.

### Showcase run at `cfcf50f`: every component in one integrated run

`WASL_POLICY=v7ltt WASL_READER=live WASL_SOURCE=hl7 WASL_IMAGE=cached WASL_LUNG=800 WASL_DR=1200 WASL_BREAST=150 WASL_LUNGSURV=1 WASL_DRNEXT=1 WASL_XREFLINK=1 WASL_REFREJECT=true scripts/demo.sh all` (seed 2026; chaos on: linker and closure+prep kills, an HL7 listener kill, a NATS node kill; HL7 connection drops and duplicate re-sends). Live local Gemma 3 4B reader: 245 unique report bodies, all read. Outputs replace [`results/showcase/`](results/showcase/).

| | Result |
|---|---|
| CRC with review | P 1.000, R 0.854, **0/481 false closures** |
| Cervical with review | P 1.000, R 0.967, 0/204 |
| Lung nodules (800 loops) | P 1.000, R 0.983, 0/297; recommendation accuracy 1.000; surveillance CT 6/6 closed, 0 false, 2/2 escalated |
| Diabetic retinopathy (1,200) | P 1.000, R 0.997, 0/1,122; referral closures R 0.968; next due date agrees with truth 0.997 |
| Breast (150) | P 1.000, R 0.993, 0/144; recall loops R 0.889 |
| Lung and DR hand-off | 3/3 and 3/3 packets, 0 false closures after re-referral |
| ≤31-day KPI | EID-only 0.126, truth 0.329, Wasl 0.319 |
| HL7 ingest and exactly-once | 40,983 messages, 0 lost, 0 duplicated; 0 duplicate outputs in 31,677; 0 final-state mismatches |
| Provenance / forged / tamper | 3,163/3,163 chains verified; 0/24 forged events with effect; 17/17 tamper attacks detected, 0 false alarms |
| Wrong-patient messages / without granted consent | 0 / 0 (1,811 released, 317 held for consent, 93 cancelled) |

One seed with the live 4B reader, so recall is not comparable with the frozen evaluation (5 seeds, cached 26B answers). Unlike the previous showcase, consent holds fired here (317 held, 0 released without consent); why the order changed with the new programmes has not been traced.

### Learn-then-Test at `5496563` ([`results/ltt/`](results/ltt/LTT.md))

| With review, τ 0.995 | `0f6a532` | **`5496563`** |
|---|---|---|
| False closures / wrong-patient message loops (of 12,600) | 0 / 0 | **0 / 0** |
| Recall | 0.813 | 0.816 |
| Review items per 100 loops | 63.5 | 58.8 |
| Loosest certified tier | 0.8 | 0.8 |

The operating tier stays 0.995. The calibration cohorts contain CRC and cervical loops only; the new programmes are covered by the frozen evaluation, not by this guarantee.

### Frozen evaluation at `5496563` ([`results/final/`](results/final/SUMMARY.md))

Same seeds and person split as before. New in the cohorts: 70 breast loops per run, rejected lung/DR referrals, lung surveillance CTs and DR next-due follow-ups; `-xreflink` and `-drnext` on.

| | Dev noise v1: `0f6a532` → `5496563` | Held-out v2: `0f6a532` → `5496563` |
|---|---|---|
| CRC recall | 0.840 → 0.843 (0.827–0.859) | 0.775 → **0.782** (0.769–0.795) |
| Cervical; lung; DR recall | 0.967; 0.986; 0.998 | 0.913; 0.950; 0.994 |
| DR referral recall | 0.978 | 0.887 (0.798–0.975) |
| Breast recall; recall-loop recall | 0.997; 0.982 | 0.994; 0.931 |
| CRC / cervical / lung / DR / breast false closures | 0/3,156; 0/1,404; 0/2,032; 0/5,636; 0/345 | 0/3,025; 0/1,313; 0/1,903; 0/5,625; 0/338 |
| Lung surveillance; DR referral; breast recall false closures | 0/41; 0/269; 0/31 | 0/30; 0/231; 0/23 |
| Hand-off packets matched (all; lung; DR) | 331/333; 19/20; 32/33 | 299/300; 21/21; 35/35 |
| Hand-off false closures (all; lung; DR) | 0/88; 0/6; 0/15 | 0/86; 0/4; 0/17 |
| Wrong-patient messages; sent without consent | 0/15,247; 0/9,945 | 0/14,451; 0/9,602 |
| Review items per 100 loops; reviewer hours per 1k loops | 42.8 → 41.4; 14.7 → 14.3 | 38.2 → 37.3; 13.2 → 12.9 |
| KPI absolute error (Wasl; Emirates-ID-only) | 0.010; 0.195 | 0.031; 0.207 |
| Origin pooled (p); standardised by programme (p) | 1.056 (0.36); 1.099 (0.015) | 1.098 (0.057); 1.031 (0.77) |

- Every safety count is 0. Tamper: 85/85 detected on both, 0 false alarms.
- Small denominators give wide bounds: breast recall 0/31 (95% upper bound 11.2%), lung hand-off 0/6 (45.9%), lung surveillance 0/41 (8.6%).
- **Subgroups** (weak evidence after about 16 tests): dev CRC by origin 1.119 (p 0.015) and dev standardised 1.099 (p 0.015) are the lowest p-values; neither passes a Bonferroni cut, and the placeholder trace above explains most of the CRC origin gap. Held-out lung by origin is 1.56 (p 0.053), the cohort-mix pattern traced above. Breast by origin (inf dev, 4.27 held-out) rests on a few items per group and is not interpretable.

## Fifth loop type: lung cancer screening LDCT

**Status: built; every result below is simulated.** Rule `acr2022lungrads` adds a separate `lung_ldct` programme through the canonical mock Malaffi → NATS → linker → closure → review path, with regex plus cached Gemma 3 4B readers, an optional coded Lung-RADS path, deterministic default-off `-ldct N` generation, metrics and API review labels. It is not the incidental-nodule/Fleischner `lung_nodule` programme.

**Post-hoc safety review.** Addenda and same-day rereads cannot close LR3/LR4A follow-up; wrong-kind labels hold explicitly; benign annual work-up closure requires benign tissue or an MDT decision. Sign-off, denied/opted-out/`MsgOptOut`, closed-loop and outcome-negation regressions pass. In the matched 300-index/303-loop rerun, with-review resolved and action closure P/R were both 1.000/1.000 with 0/278 and 0/21 false closures, and state/triage/overdue accuracy was 1.000. Without review, resolved/action recall was 0.986/0.810 because the two new benign-evidence holds await review. Wrong-patient releases, releases without granted consent, opt-out-timing violations, duplicate/lost outputs and forgery effects were all 0. Non-LDCT feeds were byte-identical to LDCT-off; CRC, incidental lung, DR and breast metrics were unchanged, while one additional simulated cervical wrong-link sign-off reduced live recall from 0.967 to 0.962 with false closures still 0, so exact live cross-programme parity is not claimed.

**Integrated reader evidence is development-only.** After the safety change, reader A scored 50/50 reports and 43/45 work-ups, Gemma v2 48/50 and 39/45, with 0 agreed-but-wrong; no labels or cache answers were rewritten. The generator reuses dev texts and all cohorts/reviewers are simulated, so integrated runs are not fair reader tests. Coded/text agreement only shows that the code matches reader A's summary reading; it is not independent full-report validation and agreeing paths can still be wrong. A separate fresh, non-blind held-out text check is reported below.

## Sixth loop type: IFHAS abnormal cardiometabolic-result follow-up

**Status: built; every result below is simulated.** Rule `doh2025ifhas` adds follow-up of abnormal IFHAS blood pressure, HbA1c/glucose, lipids and BMI, behind generator `-ifhas N` and run wiring `WASL_IFHAS=N` (both default 0). It uses printed-panel plus coded-FHIR screening readers, regex plus cached local Gemma 3 4B follow-up-note readers, trusted cross-provider links and independent per-domain closure clocks. The 59-day routine clock includes a 28-day receiving-clinic Wasl assumption; the severe-result 10-day management clock is also a modelling choice. No paid model calls were made.

**Integrated reader evidence is development-only.** The 227-note synthetic corpus was written by the same author as reader A, Gemma's v2 prompt was tuned on it, and the integrated generator reuses those same notes. Reader B scored 143/227 and was unavailable on 19/227 dev notes. In the matched multi-programme run, reader B was unavailable on 38/437 follow-up report instances; 96/437 follow-up instances and 35/800 screens went to review. Therefore integrated agreement, follow-up recall and zero false closures are **not independent reader evidence or a fair reader test**. Screening panel/coded agreement also checks internal consistency of values from the same generator, not independent clinical verification. A separate fresh, non-blind held-out follow-up-note check is reported below; coded/panel screening readers remain without fresh held-out evidence.

**Integrated synthetic runs, seed 2026 with chaos.** The IFHAS-alone run had 1,000 loops: closure P/R 1.000/0.997, 0/872 false closures, follow-up-only P/R 1.000/0.990, domain outcome accuracy 1.000 and state accuracy 0.997. The review-fix matched multi-programme run had 800 IFHAS loops: closure P/R 1.000/0.987, 0/685 false closures, follow-up-only P/R 1.000/0.961, domain outcome accuracy 1.000 and state accuracy 0.989. Every CRC, cervical, lung, DR and breast evaluator block was identical in `programmes_live` and `programmes_no_review` with IFHAS on versus off; `closure` and `crc_kpi_31d` were identical, and all normal safety counters stayed zero. Across 4,350 non-IFHAS loops, final clinical state, overdue flag, reported closure and procedure day matched. One live-run administrative CRC drop-off label differed (`MED-sr-1`: `not_booked` versus `referral_unanswered`); this is disclosed rather than tuned around and does not change a safety counter or programme evaluator block.

**Review fixes.** Reader A now gives suspected/requested/to-be-confirmed and still-pending monitoring cues precedence over white-coat/not-confirmed phrases, without turning completed monitoring or documented management into pending; the reviewer's exact ambulatory-monitoring trap and the whole dev corpus are regression-tested. IFHAS ServiceRequests route only from structured `category=ifhas-screening` with IFHAS enabled, so CRC/cervical text merely mentioning IFHAS cannot be misrouted. The API 400 message now names lung, DR, breast and IFHAS evidence labels. `ReuseLinkAnswer=false` has a regression pinning the legacy item-keyed reviewer answers.

### Fresh held-out LDCT and IFHAS reader check (`2555a27` frozen corpus, `35958dc` scores)

Fresh synthetic text and answer keys were committed before either unchanged reader ran. Regex reader A and local Gemma 3 4B reader B were then scored once; no reader, prompt, production cache, generator or threshold changed after seeing scores. The cache replay and all counts below were independently reproduced in the scoped evidence review.

| Fresh text set | N | Reader A correct | Reader B correct | B invalid/ungrounded | Agreed but wrong | To review |
|---|---:|---:|---:|---:|---:|---:|
| LDCT screening | 41 | 31 | 41 | 0 | 0 | 10 (24%) |
| LDCT work-up | 31 | 26 | 30 | 1 | 0 | 7 (23%) |
| IFHAS follow-up | 52 | 36 | 34 | 8 | 0 | 29 (56%) |

There were **0/124 agreed-but-wrong answers**, but material single-reader errors remain. LDCT reader A read two incomplete exams (key LR0) as negative LR1 and one 4X as 4A. IFHAS reader A called some pending domains managed; reader B called seven pending domains `not_confirmed` and dropped pending/`not_confirmed` domains on hf36 and hf52. Disagreement, an indeterminate result or an invalid/ungrounded answer caught every error in this check; this was not an end-to-end false-closure evaluation. The IFHAS coded/panel screening readers and LDCT coded second reader were not covered.

Limits: small single-author synthetic corpora, one run, no clinician labels; the author had seen the reader design and some development items, so these are held-out texts, **not blind or independent clinical evidence**. The frozen integrated generators still reuse development texts. Raw outputs, separate reader-B caches and disclosures are in `results/ldct-readers/` and `results/ifhas-readers/`; corpus keys/builders are in `data/ldct/heldout/`, `data/ifhas/heldout/` and `scripts/*_heldout/`. No paid or external model calls were used.

**Post-hoc shared-baseline impact (`9bfe0d2`).** Pair-deterministic reused-link review was added after an initial cervical on/off mismatch. It is correct for a single registration-pair question, but with default `ReuseLinkAnswer=true` it changes shared simulated reviewer draws for existing programmes relative to `5e9140f`. New-head IFHAS-off results must not be called equal to the old baseline. An exported `5e9140f` binary and the new head were run on the identical seed-2026 IFHAS-off cohort and settings:

| With-review recall | `5e9140f` | New head |
|---|---:|---:|
| CRC | 0.851 | 0.860 |
| Cervical | 0.962 | 0.957 |
| Lung | 0.983 | 0.993 |
| DR | 0.997 | 0.996 |
| Breast | 0.994 | 0.994 |

Every no-review programme block was identical; with-review false closures, wrong-patient releases and consent violations remained zero in both. These differences are a measured non-parity disclosure, not an improvement claim or a rerun of the frozen evaluation.

## Integrated seven-programme run at `73ec4e4`

`WASL_LUNG=800 WASL_DR=1200 WASL_BREAST=160 WASL_LDCT=300 WASL_IFHAS=700 WASL_LUNGSURV=1 WASL_DRNEXT=1 WASL_XREFLINK=1 WASL_REFREJECT=true scripts/demo.sh all` (seed 2026, cached readers, FHIR, `v7strict`, chaos on). IFHAS is 700 here because 800 loops plus its 100 distractors exceeded the remaining synthetic background-person pool after the other six programmes. The frozen evaluation's larger held-out pool passed a generation-capacity check at 800 on all five seeds.

| Programme | With-review result |
|---|---|
| CRC | P 1.000, R 0.860, 0/484 false closures |
| Cervical | P 1.000, R 0.962, 0/203 false closures |
| Incidental lung nodule | P 1.000, R 0.993, 0/300 false closures |
| Diabetic retinopathy | P 1.000, R 0.996, 0/1,121 false closures; referral recall 0.952 |
| Breast | P 1.000, R 0.994, 0/154 false closures; recall-loop recall 0.909 |
| Lung cancer screening LDCT | P 1.000, R 1.000, 0/278 false closures; action-loop P/R 1.000/1.000 |
| IFHAS | P 1.000, R 0.990, 0/616 false closures; follow-up-loop P/R 1.000/0.975 |

Wrong-patient messages, releases without granted consent and releases before a late opt-out was recorded were all 0; duplicate outputs and final-state mismatches were 0; 4,176/4,176 provenance chains verified, 0/24 forged events had effect, and the tamper checks fired. This is one synthetic seed with simulated reviewers and reused development reader corpora for LDCT/IFHAS, not a held-out reader or clinical-effectiveness evaluation. C14 surveillance follow-ups and the adversarial generators remained off. API samples were regenerated from this run.

### Frozen seven-programme evaluation at `73ec4e4` ([`results/final/`](results/final/SUMMARY.md))

`WASL_LDCT=300 WASL_IFHAS=800 scripts/final_eval.sh`, with the existing frozen cohort sizes and flags. Five seeds × v1/v2 noise, held-out persons, cached readers, FHIR, `v7strict`, chaos on. C14 surveillance follow-ups and adversarial generation stayed off. This was the frozen evaluation at `73ec4e4`; it is retained as historical evidence. The final-code rerun at `075abc5` is below.

| Mean with-review recall | Dev noise v1 | Held-out noise v2 |
|---|---:|---:|
| CRC | 0.845 | 0.786 |
| Cervical | 0.972 | 0.906 |
| Incidental lung nodule | 0.983 | 0.953 |
| Diabetic retinopathy; referral loop | 0.997; 0.970 | 0.994; 0.899 |
| Breast; recall loop | 0.997; 0.982 | 0.997; 0.971 |
| Lung cancer screening LDCT; action loop | 0.998; 0.976 | 0.999; 0.987 |
| IFHAS; follow-up loop | 0.989; 0.968 | 0.969; 0.906 |

| Pooled safety | v1 | v2 |
|---|---:|---:|
| CRC / cervical / lung / DR / breast false closures | 0/3,163; 0/1,410; 0/2,024; 0/5,634; 0/345 | 0/3,040; 0/1,303; 0/1,908; 0/5,628; 0/339 |
| DR referral / breast recall false closures | 0/267; 0/31 | 0/234; 0/24 |
| LDCT / LDCT action false closures | 0/1,398; 0/109 | 0/1,414; 0/122 |
| IFHAS / IFHAS follow-up false closures | 0/3,481; 0/1,141 | 0/3,392; 0/1,047 |
| Lung surveillance false closures; extra loops without truth obligation | 0/41; 0 | 0/32; 0 |
| Wrong-patient messages / without granted consent | 0/17,041; 0/12,380 | 0/16,472; 0/11,565 |

Duplicate/lost outputs, ingest loss/duplication, final-state mismatches and forged effects were 0; tamper detection was 85/85 with 0 false alarms in each noise model. Review load was 36.6 (v1) and 33.1 (v2) items per 100 loops across the changed seven-programme mix; these are not like-for-like workload comparisons with `5496563`. The post-hoc pair-keyed reviewer change also prevents treating small recall differences from that baseline as isolated effects of any one feature.

The A1 surveillance truth fix removed the previously disclosed extra-loop mismatch: `lung_surv_extra_opened=0` in both pooled evaluations; one surveillance obligation was missed in each. Subgroup evidence remains weak under roughly 16 tests (Bonferroni cut about 0.003): the lowest programme-standardised origin result was v1 p=0.013, and v2 lung origin p=0.039; neither passes that cut. Small denominators retain wide zero-event bounds.

**Reader caveat:** held-out persons and v2 identity noise do not make LDCT/IFHAS reader results independent. Their generators still reuse development texts, including the tuned IFHAS corpus. No paid calls or new held-out reader corpora were used.

## Integrated seven-programme run at `075abc5`

`WASL_PERSONS=0 WASL_LUNG=800 WASL_DR=1200 WASL_BREAST=70 WASL_LDCT=300 WASL_IFHAS=800 WASL_LUNGSURV=1 WASL_DRNEXT=1 WASL_XREFLINK=1 WASL_REFREJECT=true scripts/demo.sh all` (seed 2026, cached readers, FHIR, `v7strict`, chaos on). The full held-out person pool supports 800 IFHAS loops plus distractors; the default 6,500-person pool had only 882 eligible IFHAS adults and could not support the required 900. This run used 70 breast loops, versus 160 in the earlier integrated run.

| Programme | With-review result |
|---|---|
| CRC | P 1.000, R 0.837, 0/471 false closures |
| Cervical | P 1.000, R 0.977, 0/217 false closures |
| Incidental lung nodule | P 1.000, R 0.977, 0/298 false closures |
| Diabetic retinopathy | P 1.000, R 0.996, 0/1,129 false closures; referral recall 0.923 |
| Breast | P 1.000, R 1.000, 0/69 false closures; recall-loop recall 1.000 |
| Lung cancer screening LDCT | P 1.000, R 0.996, 0/281 false closures; action-loop recall 0.962 |
| IFHAS | P 1.000, R 0.984, 0/679 false closures; follow-up-loop recall 0.957 |

Wrong-patient messages, releases without granted consent and releases before a late opt-out was recorded were all 0; duplicate outputs and final-state mismatches were 0; 4,187/4,187 provenance chains verified; 0/19 forged events had effect; tamper detection was 17/17 with no baseline false alarm. Throughput was 628 events/s and reviewer time 12.3 h/1k loops under concurrent evaluation load; this is not a controlled capacity benchmark. This is one synthetic seed with simulated reviewers and reused development reader corpora for LDCT/IFHAS, not independent reader or clinical-effectiveness evidence. C14 surveillance follow-ups and adversarial generation remained off. Sanitized artifacts are in [`results/integrated-075/`](results/integrated-075/). API samples were regenerated from this run and verified to contain all seven programme case lists and all four fixtures.

### Frozen seven-programme evaluation at `075abc5`

`WASL_LDCT=300 WASL_IFHAS=800 scripts/final_eval.sh`, with the existing frozen cohort sizes and flags. Five seeds × v1/v2 noise, held-out persons, cached readers, FHIR, `v7strict`, chaos on. C14 surveillance follow-ups and adversarial generation stayed off; the EID guard retained its default-off setting. This is the final-code rerun and supersedes the `73ec4e4` artifacts, which remain above as historical evidence.

| Mean with-review recall | Dev noise v1 | Held-out noise v2 |
|---|---:|---:|
| CRC | 0.844 | 0.785 |
| Cervical | 0.969 | 0.906 |
| Incidental lung nodule | 0.982 | 0.952 |
| Diabetic retinopathy; referral loop | 0.997; 0.970 | 0.994; 0.894 |
| Breast; recall loop | 0.997; 0.982 | 0.994; 0.931 |
| Lung cancer screening LDCT; action loop | 0.998; 0.976 | 0.998; 0.981 |
| IFHAS; follow-up loop | 0.988; 0.967 | 0.969; 0.906 |

| Pooled safety | v1 | v2 |
|---|---:|---:|
| CRC / cervical / lung / DR / breast false closures | 0/3,161; 0/1,407; 0/2,022; 0/5,634; 0/345 | 0/3,035; 0/1,303; 0/1,907; 0/5,627; 0/338 |
| DR referral / breast recall false closures | 0/267; 0/31 | 0/233; 0/23 |
| LDCT / LDCT action false closures | 0/1,398; 0/109 | 0/1,413; 0/121 |
| IFHAS / IFHAS follow-up false closures | 0/3,480; 0/1,140 | 0/3,392; 0/1,047 |
| Lung surveillance false closures; extra loops without truth obligation | 0/41; 0 | 0/32; 0 |
| Wrong-patient messages / without granted consent | 0/17,063; 0/11,828 | 0/16,486; 0/11,650 |

Duplicate/lost outputs, ingest loss/duplication, final-state mismatches and forged effects were 0; tamper detection was 85/85 with 0 false alarms in each noise model. Review load was 36.9 (v1) and 33.6 (v2) items per 100 loops; reviewer time was 12.8 and 11.6 h/1k loops. One surveillance obligation was missed in each noise model; surveillance overdue flags were correct for 102/102 obligations in v1 and 88/91 in v2, with zero surveillance false closures.

Subgroup evidence remains weak under roughly 16 tests (Bonferroni cut about 0.003). The programme-standardised origin ratio was 1.0993 (p=0.010) in v1 and 1.0653 (p=0.185) in v2; v2 CRC age-band and sex ratios were 1.0657 (p=0.199) and 1.0295 (p=0.235). The lowest programme-specific origin result was v2 lung p=0.041. None passes the Bonferroni cut. Small denominators retain wide zero-event bounds.

**Reader caveat:** held-out persons and v2 identity noise do not make LDCT/IFHAS reader results independent. Their generators still reuse development texts, including the tuned IFHAS corpus. No paid calls or new held-out reader corpora were used.

### Post-hoc clean-review sensitivity at `6403180`

The five held-out v2 frozen lanes were rerun with ordinary simulated reviewer error set from 4% to **0%** and day-15 ungraded-prep simulated reviewer error from 25% to **0%**. All other settings stayed unchanged, including held-out people, cached readers, FHIR, `v7strict`, chaos and default-off EID guard. No model, threshold, calibrator, reader or generator changed; no paid calls were made. The decision code is unchanged from `075abc5`; `6403180` added evidence and documentation only.

| CRC result over five seeds | Reviewer-error stress | Clean-review sensitivity |
|---|---:|---:|
| Mean recall | 0.785 | **0.859** |
| 95% t interval over seed means | 0.772–0.798 | **0.835–0.883** |
| Mean precision | 1.000 | **1.000** |
| Pooled false closures | 0/3,035 | **0/3,322** |

The paired seed recalls were 0.791→0.863, 0.790→0.866, 0.785→0.872, 0.767→0.825 and 0.792→0.869, a mean increase of **7.4 percentage points**. Clean-review wrong-patient releases were 0/16,534 and releases without granted consent 0/11,610; all programme false-closure, duplicate/loss, final-state and forgery-effect counters remained zero. One lung-surveillance obligation was missed and 89/91 surveillance overdue flags were correct.

This is a **post-hoc reviewer-error sensitivity**, not a replacement frozen headline or measured clinical performance. Zero injected error makes simulated reviewers agree with simulated truth; it does not establish error-free clinicians. Identity noise, missing evidence, ungraded prep, reader/rule limits, timing and chaos remain. Full per-seed, programme, safety and subgroup evidence is in [`results/clean-review-sensitivity/`](results/clean-review-sensitivity/README.md).

## Post-hoc identity, scale and deployment hardening

All changes in this section followed review or observed failures and are disclosed as post-hoc. No paid calls were used. The B8 closure guard remains default-off and is evaluated separately from the headline frozen evaluation.

### Minor outreach fixes

At `5a7094c` (merged at `3d2b6e6`), LDCT and IFHAS outreach now requires `Contact == granted`, matching the other programmes, and cross-programme provider attribution counts LDCT and IFHAS providers. Focused regressions and the full Go suite passed. The unused legacy `DispatchC` path retains its documented consent behavior.

### X10 scale and identity hardening

The original x10 run was a failed safety result: 80 unexpected linker restarts, one CRC false closure, one cervical false closure and four wrong-patient releases. Fixes at `b67371d` (merged at `879505d`) covered mutex-protected linker status snapshots, EID-conflict detection including non-Luhn IDs, cluster-bridge prevention, and placeholder-DOB corroboration requiring informative name skeletons. The calibrator was **not refit** for the placeholder-DOB rule. No closure threshold or default guard setting changed.

A subsequent rerun used a stale binary and reproduced old crashes; it is not evidence against the fixed sources. A stale-binary guard was added at `bb8b1d7` (merged at `8e0d821`). The first correctly rebuilt rerun then had zero unexpected restarts and zero false closures, but still one wrong-patient release. Root-cause tracing showed that an EID-clash refusal was not durable: after a wrong simulated reviewer-different split removed the clashing EID, a bridge record could merge the previously refused pair. At `5045e15` (merged at `e27eefc`), an EID-clash refusal now also records a cannot-link constraint. Two regressions failed on the unfixed code and passed after the fix; the full Go and race suites and `go vet` passed.

A fresh, explicitly rebuilt x10 run at `e27eefc` then measured zero unexpected linker restarts, wrong-patient releases, CRC/cervical false closures in either lane, consent violations, duplicate/lost outputs, final-state mismatches and forged effects. Lung, DR and breast false closures were also zero; provenance verified 30,567/30,567. It recorded 2,374 `eid_cluster_clash` events and 95 `cannot_rejects`. A fresh normal-scale run passed the same measured safety checks. Full tables and the preserved historical failures are in [`results/scale/README.md`](results/scale/README.md), with sanitized artifacts in `normal-hardened/` and `x10-hardened/`.

These are single-seed simulated runs on a shared host, not controlled capacity benchmarks. X10 includes training persons and is not held-out accuracy evidence. The final x10 run did not execute the optional full tamper replay. Historical failed artifacts were preserved rather than rewritten.

### Fresh B8 wrong-EID evaluation at `075abc5`

The post-hoc `-eidguard` change (`27d03b4`, hardened at `97aaab1` and merged at `4b8aceb`) adds equal-EID demographic contradiction checks, direct/multi-hop/hub and arrival-order handling, below-threshold near-miss review routing, human same/different behavior and cross-programme closure checks. It remains **off by default**. It does not refit the linker or calibrator.

Fresh seeds **17 and 29** used v2 held-out noise, seven programmes, cached readers, FHIR, `v7strict`, chaos, LDCT 300 and IFHAS 800. Four matched lanes compared clean versus 2% wrong-EID generation and guard off versus on. The 4% reviewer error is an assumed stress value, not a measured clinician error rate.

| Cohort | Guard | CRC / cervical / lung false closures | CRC recall | Cervical recall | Review items / 100 loops |
|---|---|---|---:|---:|---:|
| Clean | Off | 0/1,178; 0/523; 0/802 | 0.7715 | 0.9090 | 31.651 |
| Clean | On | 0/1,176; 0/524; 0/801 | 0.7700 | 0.9110 | 32.735 |
| Wrong EID 2% | Off | **16/1,166; 2/520; 1/796** | 0.7535 | 0.9005 | 31.9205 |
| Wrong EID 2% | On | 0/1,133; 0/515; 0/794 | 0.7420 | 0.8955 | 33.012 |

Under wrong-EID stress, the guard removed all **19 observed false closures**. CRC recall cost 1.15 percentage points, cervical recall cost 0.50 points, and review load increased by 1.0915 items per 100 loops (reviewer time 10.9645 → 11.308 h/1k loops). In the clean cohort, CRC recall changed by −0.15 points, cervical by +0.20 points, and review load increased by 1.084 items per 100 loops. Two seeds cannot establish the small clean-cohort recall differences.

Every lane had zero wrong-patient releases, releases without granted consent, duplicate/lost outputs, ingest loss/duplication, final-state mismatches and forged effects. All other programme false-closure counters were zero, and each lane detected 34/34 tamper attacks with no baseline false alarms.

Guard-on wrong-EID linkage still contained **298 false-merge pairs, 122 impure clusters, 78 cross-person EID edges, one cross-person human edge and 99 cross-person `wasl` edges** across the two seeds. Guard-off had 1,623 false-merge pairs, 568 impure clusters, 905 cross-person EID edges and 99 `wasl` edges. Clean lanes retained 156 false-merge pairs, 68 impure clusters and 99 `wasl` edges in both guard settings. Mean pair recall fell from 0.8690 to 0.8535 under wrong-EID stress and from 0.8970 to 0.8810 in clean cohorts when the guard was enabled. Compatible/missing-demographic wrong EIDs, same-surname relatives and model/human-only contamination remain possible.

The complete manifests, per-seed reports, subgroup analyses, tamper evidence and pooled safety counters are in [`results/b8-hardening/`](results/b8-hardening/README.md). These two post-hoc seeds supplement rather than replace the five-seed headline evaluation.

### CRC recall-loss analysis on B8 seeds 17/29

A read-only offline classifier analysed truth-closed CRC loops left open in the four fresh B8 lanes. It did not rerun the pipeline, train a model, change a threshold/calibrator or tune on evaluation seeds. In wrong-EID guard-on, 394 of 1,527 truth-closed loops remained open. First-match barriers were identity/linking 195 (49.5%), simulated reviewer error 105 (26.6%), timing/ungraded prep 45 (11.4%), and model/rule/other 49 (12.4%). The largest single category was 146 loops with no established link and no review item. Ungraded prep appeared in 241 misses across categories; pending histology and unanswered review explained none in this replay.

Most loss was baseline: 351 wrong-on misses were also missed in clean-on. Guard-off also closed 13 truth-closed loops using another person's report. Excluding those wrong-patient closures, right-patient recall was 0.7446 guard-off versus 0.7420 guard-on: a net four-loop or 0.26-point cost, rather than the headline 17-loop or 1.15-point difference. This accounting does not excuse guard-off false closures.

L5 was strongly associated with DOB distortion descriptively: its share of truth-closed loops was 2.2% with exact DOB and 27.1% with a year shift of up to ten years. Ungraded-prep recall was 0.700 versus 0.861 with agreed adequate prep; days 105–120 had recall 0.591. All 40 E4 misses came from one report phrasing (u18), so they are not a general reader-error estimate. Two-seed subgroup intervals overlapped, but that is not evidence of no bias.

The proposed development design uses new seeds from non-`eval` people, varied noise/evidence/horizon and independently authored report text, with person-disjoint pre-registered validation. Frozen and B8 seeds remain locked; no training implementation was requested or performed. Full categories, denominators, paired-lane accounting, caveats, split design, row-level evidence and the deterministic classifier are in [`results/recall-analysis/`](results/recall-analysis/README.md). The coordinator reproduced its JSON, CSV and console output byte-identically.

### CRC candidate review-floor experiment at `8832e82`

**Built:** the opt-in `-linkfloor` flag accepts `(0,0.2]` and defaults to **0.2**, leaving default behavior unchanged. It lowers only review routing for candidates that retrieval already produced. Merge remains 0.7, the closure tier 0.99, the uncorroborated cap 0.989, and the equal-EID contradiction clamp 0.2; EID/cannot-link, second-identifier, evidence, consent and attachment rules are unchanged. The configured floor reaches orchestrate children, audit/replay, report7 and policy identity. The default-parity test establishes repeatability at 0.2, not byte parity with a pre-change binary.

**Simulated evaluation:** paired CRC-only v2 cohorts used cached readers, image off, FHIR, chaos 0, subkill 1 and no paid calls. Development seeds were 101/103/107. The frozen selection rule required at least +1.0 percentage point pooled CRC closure-recall gain, no listed safety/replay/analysis failure and no increased linkage contamination, preferring 0.05 if both candidates passed. Validation seeds 211/223/227/229/233 were untouched until development and stress selection succeeded and the configuration was frozen. CRC closure recall is exact pooled `(pred_closed − false_closures) / truth_closed`, not link attach rate.

| Clean development | 0.2 baseline | 0.05 | 0.01 |
|---|---:|---:|---:|
| Seed 101 | 0.8348 | 0.8478 | 0.8478 |
| Seed 103 | 0.8174 | 0.8217 | 0.8435 |
| Seed 107 | 0.8488 | 0.8537 | 0.8585 |
| Pooled TP / truth | 554/665 | 559/665 | 565/665 |
| Pooled recall | 0.8331 | 0.8406 | **0.8496** |
| Pooled gain vs baseline | — | +0.7519pp | **+1.6541pp** |
| Mean seed recall, 95% t interval (n=3) | 0.7946–0.8727 | 0.7988–0.8833 | 0.8307–0.8692 |
| Selection | retained baseline | **failed +1.0pp gate** | **selected** |

The 0.01 arm's mean paired per-seed gain was +1.6296pp, with a wide small-n 95% t interval of −0.5163 to +3.7754pp. This interval is descriptive; selection used the predeclared pooled gate, not statistical significance.

The selected arm then passed a separate synthetic stress run with 4% ordinary reviewer error and 25% day-15 ungraded-prep reviewer error. These are assumed stress values, **not measured clinician rates**.

| Development, reviewer-error stress | 0.2 baseline | 0.01 |
|---|---:|---:|
| Seed 101 | 0.7783 | 0.7913 |
| Seed 103 | 0.7435 | 0.7696 |
| Seed 107 | 0.7756 | 0.7854 |
| Pooled TP / truth | 509/665 | 520/665 |
| Pooled recall | 0.7654 | **0.7820** |
| Pooled gain | — | **+1.6541pp; gate passed** |
| Mean seed recall, 95% t interval (n=3) | 0.7177–0.8139 | 0.7542–0.8100 |

| Untouched clean validation | 0.2 baseline | 0.01 |
|---|---:|---:|
| Seed 211 | 0.8489 | 0.8667 |
| Seed 223 | 0.8494 | 0.8703 |
| Seed 227 | 0.8230 | 0.8451 |
| Seed 229 | 0.8333 | 0.8378 |
| Seed 233 | 0.8756 | 0.8955 |
| Pooled TP / truth | 941/1,113 | 960/1,113 |
| Pooled recall | 0.8455 | **0.8625** |
| Pooled gain | — | **+1.7071pp; gate passed** |
| Mean seed recall, 95% t interval (n=5) | 0.8213–0.8708 | 0.8348–0.8914 |

Validation improved on every seed. Its mean paired per-seed gain was +1.7045pp (95% t interval +0.8120 to +2.5971pp). The small-n intervals do not establish clinical effectiveness or generalization beyond these synthetic cohorts. The validated non-default candidate is **0.01**; the shipped default remains **0.2**.

Reviewer burden increased modestly. `Link pairs` are distinct link-review pairs; `Items` and `Hours` cover all reviewer work, pooled across the arm's seeds. No link-review item was unanswered.

| Mode | Floor | Link pairs | Items | Hours |
|---|---:|---:|---:|---:|
| Development clean | 0.2 / 0.05 / 0.01 | 507 / 517 / 528 | 1,071 / 1,085 / 1,096 | 34.943 / 35.265 / 35.503 |
| Development stress | 0.2 / 0.01 | 506 / 528 | 1,063 / 1,090 | 35.967 / 36.718 |
| Validation clean | 0.2 / 0.01 | 908 / 936 | 1,830 / 1,870 | 61.003 / 62.381 |

All arms had **zero false closures, wrong-patient releases, replay mismatches, analysis errors and funnel trace-write errors**. The injected stress errors produced 87 wrong final sign-offs in the baseline arm and 89 in the 0.01 arm; these cost recall but did not become wrong-patient releases. Linkage contamination did not increase, but was not zero: development and stress each retained 36 false-merge pairs, 30 impure clusters and 30 cross-person edges in both arms; validation retained 144, 125 and 131 respectively in both arms. These are not perfect-linkage or non-CRC safety claims.

Residual truth-closed CRC loops remained open. The funnel categories below partition those misses: `attached_or_closed` includes both closed loops and attached-but-not-closed loops, so the latter are separated below. Attach count is not closure recall.

| Mode | Floor | Residual misses | Attached, not closed | Below review | No candidate | Demographic rejection | Routed review | Linked report not attached |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Development clean | 0.2 / 0.05 / 0.01 | 111 / 106 / 100 | 15 / 15 / 15 | 60 / 54 / 48 | 3 / 3 / 3 | 2 / 2 / 2 | 0 / 0 / 0 | 31 / 32 / 32 |
| Development stress | 0.2 / 0.01 | 156 / 145 | 52 / 52 | 60 / 48 | 3 / 3 | 2 / 2 | 6 / 6 | 33 / 34 |
| Validation clean | 0.2 / 0.01 | 172 / 153 | 26 / 26 | 104 / 84 | 8 / 8 | 1 / 1 | 0 / 0 | 33 / 34 |

**Post-hoc disclosures:** development lanes completed at runtime revision `039d449`. Aggregation then failed because report7 exposes `pred_closed` and `false_closures`, not `tp`. The analyzer-only fix at `8832e82` derived exact TP and added consistency tests; preserved development lanes were reclassified offline without rerunning them or inspecting candidate results before the fix. Stress and validation ran at `8832e82`, with no runtime change from `039d449`. An earlier pre-generation dependency failure (missing NATS) and all failed artifacts were preserved. Lowering `LinkReview` also changes the strength assigned to untrusted `xrefFacts` referral-reference rows; this second behavior is included in the measured runs. The floor applies globally, but these CRC-focused cohorts do not establish non-CRC recall or safety. All results are simulated; no clinical effectiveness is proven.

### CRC feature-first pair-scorer experiment at `dc21cec` (not selected)

**Outcome:** the experiment failed its pre-registered development selection gate and stopped as specified. `pairfeat1` is **not a candidate**. The default model and the default review floor of 0.2 are unchanged. All results below are simulated; they prove no clinical effectiveness.

**Built:** an opt-in `pairfeat1` modelset (code at `dc21cec`) keeps the 23 existing pair features byte-compatible and appends seven at indices 23-29: `eid_hamming`, `dob_decade_digit_only`, `dob_digit_difference_count`, `dob_day_only`, `dob_month_only`, `name_token_set_equal` and `name_skeleton_equal`. Missing or placeholder DOBs yield NaN rather than agreement. The chain is a local copy of the default encoder (not retrained), a refit 30-feature LightGBM and two refit calibrators. `pairfeat1` is accepted only with `v7strict`; `v7ltt` fails closed. Merge 0.7, closure tier 0.99, the uncorroborated cap, the equal-EID clamp, EID/cannot-link, second-identifier, evidence, consent and attachment rules are unchanged. Generated model files are git-ignored.

**Training protocol (simulated):** one fit with the existing scorer configuration on fresh train-split seeds 1001, 1003, 1007, 1009 and 1013 (5,000 persons each) under training noise v1, with the fixed encoder: 400 rounds, 63 leaves, learning rate 0.05, seed 1, no hyperparameter search. Calibrators were refit on cal-split seed 2003 only. Train had 1,508,325 retrieved pairs (42,270 same-person); cal (the whole cal split, 3,588 persons) had 143,933 (6,273 same-person). Cal pair AUC was 0.99997 and AP 0.99944. At raw LightGBM score >= 0.99 (a threshold on the uncalibrated score, not the calibrated closure tier) cal pair precision was 0.9990 and recall 0.9434. Overall different-person routing exposure on cal: of 137,660 different-person pairs, 195 scored raw >= 0.01 (review) and 23 scored raw >= 0.7 (merge); the slice table below breaks this down. Pair AUC is diagnostic, not the selection gate.

**Artifact identity and parity:** the artifact manifest hashes all four `pairfeat1` chain files and the baseline chain (the training manifest `e368b3bc` hashes the encoder and code; the governing manifest `287cc80b` bound the development lanes). Chain files: `lgb_stream_pairfeat1` `f1d3ac57`, `iso_pairfeat1` `b12b9908`, `iso_v7_pairfeat1` `202de5a8`; `encoder_pairfeat1` `5e6284f8` is identical to the baseline encoder. The baseline chain is `lgb_stream` `13d3e272`, `iso` `17e434ad`, `iso_v7` `63ef7bd0`. Both development arms used the same binary (`967bada4`) at `dc21cec`. Go/Python scoring parity over all 143,933 cal rows had max |delta| 2.2e-16 (limit 1e-9).

**Hard-negative slices (cal split, seed 2003):**

| Slice | Rows | Same-person | Different-person | Different-person at raw >= 0.01 | At raw >= 0.7 |
|---|---:|---:|---:|---:|---:|
| near EID | 1,037 | 1,037 | 0 | 0 | 0 |
| decade-digit DOB | 94 | 0 | 94 | 0 | 0 |
| same surname | 19,515 | 5,037 | 14,478 | 103 | 14 |
| placeholder DOB | 83,774 | 1,905 | 81,869 | 81 | 10 |
| cross-script names | 47,698 | 3,166 | 44,532 | 114 | 15 |

**Decade-noise finding:** the frozen v1 training noise produced **zero same-person decade-digit-only DOB pairs** (all 94 decade rows in cal are different-person; the near-EID slice likewise contains no different-person pairs). Feature 24 could therefore only be learned as a different-person signal. The earlier development diagnostic hypothesis, that decade-digit DOB errors seen under v2 evaluation noise are recoverable by this feature, **was not tested** by this protocol. Training on v2 noise would have leaked the evaluation corruption model, so it was not done; changing training noise would be a new experiment.

**Paired development clean (seeds 307, 311, 313; both arms `-linkfloor 0.01`, `v7strict`, CRC-only v2, cached readers, image off, FHIR, chaos 0, subkill 1):** exact TP is `pred_closed - false_closures`, not attach rate; truth-closed denominators match.

| Seed | Baseline TP / truth | Baseline recall | `pairfeat1` TP / truth | `pairfeat1` recall | Gain |
|---|---:|---:|---:|---:|---:|
| 307 | 185/212 | 0.8726 | 185/212 | 0.8726 | +0.00pp |
| 311 | 207/233 | 0.8884 | 210/233 | 0.9013 | +1.29pp |
| 313 | 208/236 | 0.8814 | 209/236 | 0.8856 | +0.42pp |
| Pooled | 600/681 | 0.8811 | 604/681 | 0.8869 | **+0.5874pp** |

The mean paired per-seed gain was +0.5704pp with a 95% t interval of -1.0598 to +2.2006pp (n=3; small-n and descriptive only). **Gate verdict: +0.5874pp is below the required +2.0pp, so `pairfeat1` was not selected.** The 4-loop pooled difference is small relative to this noise.

**Safety, contamination, workload:** false closures, wrong-patient releases, replay mismatches, analysis errors and trace-write errors were zero in both arms, with zero wrong final sign-offs and zero unanswered review items. Linkage contamination decreased but was not zero: false-merge pairs 35 to 26, impure clusters 27 to 25, cross-person edges 31 to 25. This is n=3 development evidence from a not-selected arm and is not a validated contamination benefit. Distinct link-review pairs rose 556 to 577 (+3.8%, within the 10% limit), items 1,128 to 1,153 and reviewer hours 37.721 to 38.354.

| Residual truth-closed misses | Baseline | `pairfeat1` |
|---|---:|---:|
| Total open | 81 | 77 |
| Attached, not closed | 11 | 11 |
| Below review | 44 | 40 |
| No candidate generated | 4 | 4 |
| Demographic rejection | 0 | 0 |
| Routed to review | 0 | 0 |
| Linked report not attached | 22 | 22 |

The scorer-addressable category (below review) shrank by four. The categories a scorer change cannot reach, "linked report not attached" (22) and "no candidate generated" (4), were unchanged; "attached, not closed" (11) was also unchanged but is not claimed to be scorer-unreachable.

**Not run:** per the pre-registered stop rule, synthetic reviewer stress and untouched validation were **not run**. No validation manifest was frozen and eval seeds 401, 409, 419, 421 and 431 remain untouched. All output roots, including both arms' logs, are preserved locally.

**Result-affecting post-hoc changes and rulings:**
- The plan's decade feature test (`diffPos == 3`) contradicted the spec: feature 24 is set only when YYYYMMDD position 2 (the tens-of-year digit) alone differs.
- The `dobFeats` guard is length-only (`len != 10` returns the existing missing sentinel); a real-date check would have changed legacy features 0-22 for invalid 10-character dates.
- The Go LightGBM reader now maps NaN to 0 when a node's missing type is not NaN, matching LightGBM; legacy features are never NaN, so default scores are unchanged (pinned by a parity test).
- `featdump` writes features and scores with `%.17g` so Go/Python parity is meaningful.
- Review of Task 2 found orchestrate/audit children received `-modelset` but not `-policy`, so the strict guard would have rejected every child; `-policy` is now forwarded to children rather than loosening the guard.
- The candidate arm passes `-modelset pairfeat1` to orchestrate, audit and report7 but not to the simulator, so both arms share identical cohort and truth inputs.
- Final whole-branch review found `read_p_column` expected a header row that the Go dump does not write. This was fixed before the fit (`dc21cec`), together with a row-by-row feature identity check against the dump, an end-of-run Git HEAD assertion and a git-ignore rule for the generated chain.
- The artifact manifest shape (`git_head`, per-arm hash maps keyed by chain filename) was specified by ruling because the plan only said "four hashes".

**Process rulings and deviations:**
- Python tests were run by direct path under `-I` because `encoder/` is not a package.
- `lightgbm` 4.7.0 was not installed in the environment and was installed (free, PyPI) before the fit.
- The Task 5 driver imports numpy only to read the cal score `.npy` (the plan said standard library only).
- The Task 6 fix round was widened to two minor findings: validation preflight re-verifies upstream hashes and the passing selection/stress decisions, and the validation output is write-once.
- Strict same-HEAD binding from training through validation was accepted as intended; a harness fix mid-experiment would have required a refit and new manifest.
- Go tasks ran sequentially (1, 2, 3) and Python tasks 4-6 in parallel; Task 2 ran concurrently with Task 1's review.
- These results docs were first committed (`742f863`) before the independent document review; this correction follows that review.

**Scope:** the model applies globally but these cohorts are CRC-only. No non-CRC safety or recall claim is made, and simulated results establish no clinical effectiveness. No further seeds, thresholds, features, hyperparameters or encoder fallback are authorized for this arm.

### CRC decade-noise pair-scorer experiment at `9bf8017` (not selected)

**Outcome:** the experiment failed its pre-registered development selection gate and stopped as specified. `pairfeat2` is **not a candidate**. This is the successor to the feature-first `pairfeat1` experiment above, which could not test its decade-digit DOB hypothesis because its training noise contained no same-person decade-digit-only pairs. The default model and the default review floor of 0.2 are unchanged. All results below are simulated; they prove no clinical effectiveness.

**Built:** an opt-in `pairfeat2` modelset (code at `9bf8017`) uses the same 30-feature layout, fixed encoder, LightGBM configuration and calibrator procedure as `pairfeat1` (400 rounds, 63 leaves, learning rate 0.05, seed 1, no hyperparameter search). The only intended change is the training noise. Runtime, features, retrieval, merge 0.7, closure tier 0.99, the uncorroborated cap and the identity, consent and attachment rules are unchanged. `pairfeat2` is accepted only with `v7strict`. Generated model files are git-ignored.

**Training noise `v1t` (training only):** identical to v1 (DOB corrupted with probability 0.3) except that a corrupted DOB first takes, with probability 0.20, a typo branch that replaces exactly one of the eight YYYYMMDD digits, chosen uniformly, with a different digit chosen uniformly from the other nine. The remaining 0.80 of corruptions follow v1's existing branches with v1's existing draw sequence. The typo may produce a calendar-invalid date; existing missing-data handling applies. This is a generic keystroke typo and makes same-person decade-digit-only pairs (position 2 alone differs) appear in training. It deliberately does **not** copy v2's plus or minus ten-year error, because that is the held-out evaluation corruption model and training on it would leak the evaluation. `v1t` is refused by every command except `featdump`, so simulated cohorts, the runtime and evaluation noise are unaffected; v1 and v2 `gen` output was verified byte-identical before and after the change.

**Seeds (all previously unused):** train 1019, 1021, 1031, 1033 and 1039 (5,000 persons each), calibrators on cal seed 2011 (the whole cal split), paired development (dev/v2) seeds 317, 331 and 337. Validation seeds 401, 409, 419, 421 and 431 were not touched.

**Training and calibration metrics:** train had 1,511,069 retrieved pairs (41,992 same-person); cal had 139,084 (6,034 same-person). Cal pair AUC was 0.99990 and AP 0.99877. At raw LightGBM score >= 0.99 (an uncalibrated threshold, not the calibrated closure tier) cal pair precision was 0.9998 and recall 0.9375. Pair AUC is diagnostic, not the selection gate.

**Artifact identity and parity:** the training manifest is `07595ba5`. Chain files: `lgb_stream_pairfeat2` `b0afcdcf`, `iso_pairfeat2` `3293079c`, `iso_v7_pairfeat2` `aeca71e8`; `encoder_pairfeat2` `5e6284f8` is identical to the baseline encoder. The baseline chain is `lgb_stream` `13d3e272`, `iso` `17e434ad`, `iso_v7` `63ef7bd0`. The governing manifest `912c27de` bound the development lanes, and both arms used the same binary (`1b12c36c`) at `9bf8017`. Go/Python scoring parity over all 139,084 cal rows had max |delta| 1.1e-16 (limit 1e-9).

**Hard-negative slices (cal split, seed 2011):**

| Slice | Rows | Same-person | Different-person | Different-person at raw >= 0.01 | At raw >= 0.7 |
|---|---:|---:|---:|---:|---:|
| near EID | 1,020 | 1,018 | 2 | 2 | 0 |
| decade-digit DOB | 144 | 39 | 105 | 1 | 0 |
| same surname | 18,781 | 4,986 | 13,795 | 120 | 14 |
| placeholder DOB | 78,253 | 1,657 | 76,596 | 153 | 20 |
| cross-script names | 45,349 | 3,021 | 42,328 | 172 | 17 |

The decade-digit DOB slice now contains 39 same-person rows (it had none under v1 in `pairfeat1`), so the precondition that experiment lacked is met and feature 24 can be learned as a same-person signal. Overall different-person routing exposure on cal: of 133,050 different-person pairs, 280 scored raw >= 0.01 (review) and 31 scored raw >= 0.7 (merge). The `pairfeat1` figures were 195 and 23 of 137,660 on a different cal seed and cohort, so they are not a like-for-like comparison.

**Paired development clean (seeds 317, 331, 337; both arms `-linkfloor 0.01`, `v7strict`, CRC-only v2, cached readers, image off, FHIR, chaos 0, subkill 1):** exact TP is `pred_closed - false_closures`, not attach rate; truth-closed denominators match across arms.

| Seed | Baseline TP / truth | Baseline recall | `pairfeat2` TP / truth | `pairfeat2` recall | Gain |
|---|---:|---:|---:|---:|---:|
| 317 | 196/224 | 0.8750 | 207/224 | 0.9241 | +4.91pp |
| 331 | 196/219 | 0.8950 | 205/219 | 0.9361 | +4.11pp |
| 337 | 189/219 | 0.8630 | 198/219 | 0.9041 | +4.11pp |
| Pooled | 581/662 | 0.8776 | 610/662 | 0.9215 | **+4.3807pp** |

The mean paired per-seed gain was +4.3766pp with a 95% t interval of +3.2276 to +5.5257pp (n=3; small-n and descriptive only). The gain clears the +2.0pp recall criterion in every seed.

**Gate verdict: not selected.** The selection gate requires the recall gain and also no increase in linkage contamination and a distinct link-review pair count no more than 10% above baseline. Four reasons failed:
- False-merge pairs increased, 31 to 43 (per seed 12 to 15, 7 to 13, 12 to 15).
- Impure clusters increased, 28 to 39 (per seed 10 to 14, 7 to 10, 11 to 15).
- Cross-person edges increased, 29 to 40 (per seed 11 to 14, 7 to 11, 11 to 15). Contamination rose in every seed.
- Distinct link-review pairs rose 523 to 652 (+24.7%) against a cap of 575.3 (baseline plus 10%).

**Safety and workload:** false closures, wrong-patient releases, replay mismatches, analysis errors and trace-write errors were zero in both arms, with zero wrong final sign-offs and zero unanswered review items. Reviewer workload rose: items 1,067 to 1,211 and simulated reviewer hours 35.04 to 39.31 (per seed 10.93 to 12.32, 11.70 to 13.24, 12.41 to 13.75). The zero-safety result is n=3 development evidence and is not a validated safety property.

| Residual truth-closed misses | Baseline | `pairfeat2` |
|---|---:|---:|
| Total open | 81 | 52 |
| Attached, not closed | 17 | 18 |
| Below review | 35 | 4 |
| No candidate generated | 4 | 4 |
| Demographic rejection | 1 | 0 |
| Routed to review | 0 | 0 |
| Linked report not attached | 24 | 26 |

The scorer-addressable category (below review) fell from 35 to 4, which accounts for the recall gain; the categories a scorer change cannot reach ("no candidate generated" 4, unchanged) did not move, and "linked report not attached" and "attached, not closed" were not reduced (24 to 26 and 17 to 18).

**Interpretation:** in development, the retrained scorer recovered most of the misses that fell below the review floor, and the recall gain is observed in these simulated cohorts (n=3, descriptive). It came with more cross-person linkage and more link-review load. By the pre-registered rule the arm is not selected. A read-only diagnosis of the contamination increase was run after the gate and is reported below. This is a development result from three seeds, not validation evidence, and no claim is made that it would generalize.

**Post-hoc diagnosis (read-only, after the gate; development lanes only; nothing was changed or re-run):** direct cross-person model edges (score >= 0.7) were compared between arms across the three development seeds. `pairfeat2` added 23 cross-person edges that baseline did not have and removed 11 that baseline had (recomputed totals match `report7` on two seeds and are one edge above it on the third). In baseline, 15 of the 23 added pairs were review-band candidates and 8 were below the 0.01 floor, so some pairs moved from near-zero to the merge tier. Of the 23 added cross-person edges, 21 had an Emirates ID on only one side (so the ID check could not veto them) and 2 had none; 14 were cross-script. Their DOBs: missing on one side 7, equal 2, one digit apart 5, two digits apart 5 and three or more digits apart 4 (6 of the last 9 involve a YYYY-01-01 placeholder). Several pairs share a given name with a different family name; this was read by eye on a subset and not counted. In the same lanes `pairfeat2` added 216 same-person edges, which are dominated by one-digit (135) and two-digit (44) DOB differences. Reading: the typo noise made DOB disagreement cheaper as evidence against a match in general, not only for decade-digit-only pairs, so where the DOB is missing, a placeholder or one or two digits off, and the ID is on one side only, name similarity alone could carry a pair to the merge tier. The same relaxation plausibly accounts for the recall gain; this was not isolated by an ablation. The extra link-review load is also mostly the recall gain: of 150 link-review pairs present only in `pairfeat2`, 148 are same-person; across all 150, 109 were below the floor in baseline, 22 were in the review band and 19 were already linked; 21 baseline review pairs were no longer asked. This is a diagnosis of three development seeds, not a tested fix, and no change is authorized from it.

**Not run:** per the pre-registered stop rule, synthetic reviewer stress and untouched validation were **not run**. No validation manifest was frozen and eval seeds 401, 409, 419, 421 and 431 remain untouched. No threshold, gate criterion, feature, hyperparameter, seed or analysis rule was changed after seeing results.

**Result-affecting rulings:**
- The `pairfeat1` training manifest gained a `modelset` field for future runs; the brief required both recording the modelset and byte-identical `pairfeat1` behaviour, and recording won. The completed `pairfeat1` run's stored manifests are untouched.
- The pre-generalization `pairfeat1` arm roots are archival only: the harness now requires a `candidate` field in `build.json` and `selection.json`, which they lack, so they cannot be re-analysed. `pairfeat1` is concluded and its aggregate outputs are write-once.
- `v1t` is training-only and refused outside `featdump` (configuration error, exit 2), so it cannot reach generation, the runtime or evaluation.

**Process rulings and deviations:**
- Go task A and Python tasks B and C ran in parallel, since they touched disjoint files; the cost was only commit interleaving.
- Deferred minor review findings (none affect results): the `--modelset=X` form is unsupported and fails closed; the driver's main flow is covered end to end by the run, not by a unit test; the `featdump` `v1t` test fails rather than skips when data or models are absent; the typo-shape test bounds are loose; the `-noise` help text omits `v1t`; a non-string validation-manifest candidate raises a TypeError (fails closed); lane seed identity rests on the directory name; the gate's distinct-pairs fallback key.
- Strict same-HEAD binding from training through selection was kept, so HEAD stayed at `9bf8017` for the whole run.

**Scope:** the model applies globally but these cohorts are CRC-only. No non-CRC safety or recall claim is made, and simulated results establish no clinical effectiveness. The default model and default floor 0.2 are unchanged. No further seeds, thresholds, features, hyperparameters or noise variants are authorized for this arm.

### Deployment verification limit

Static deployment validation passed: `scripts/check_deploy.py` found the expected five services and four volumes, all five tests in `scripts/test_deploy.py` passed, and shell syntax checks passed. The Docker CLI is unavailable in the evaluation environment, so the image build, Compose startup, container healthchecks and containerized demo were **not executed**. Static checks are not presented as Docker runtime evidence.

### CRC namesake hard-negative pair-scorer experiment at `f3a2f3c` (not selected)

**Outcome:** the experiment failed its pre-registered development selection gate and stopped as specified. `pairfeat3` is **not a candidate**. This is the successor to the `pairfeat2` experiment above, motivated by its post-hoc diagnosis: the new cross-person edges there were mostly pairs with a missing, placeholder or one-to-two-digit-off DOB, an Emirates ID on one side only and a shared given name, which name similarity alone carried to the merge tier. The hypothesis was that training lacked different-person pairs of that shape, so adding them would keep the recall gain without the contamination. The recall gain was kept (+5.87pp here against +4.38pp for `pairfeat2`, on different development seeds, so not like-for-like), but contamination and link-review workload still rose and the gate failed on the same four reasons. The default model and the default review floor of 0.2 are unchanged. All results below are simulated; they prove no clinical effectiveness.

**Built:** an opt-in `pairfeat3` modelset (code at `f3a2f3c`) uses the same 30-feature layout, fixed encoder, `v1t` training noise, LightGBM configuration and calibrator procedure as `pairfeat2` (400 rounds, 63 leaves, learning rate 0.05, seed 1, no hyperparameter search). The only intended change is the training data. Runtime, features, retrieval, merge 0.7, closure tier 0.99, the uncorroborated cap and the identity, consent and attachment rules are unchanged. `pairfeat3` is accepted only with `v7strict`. Generated model files are git-ignored.

**Training data, synthetic namesakes (training only):** `featdump` gained a `-namesakes` rate, valid only for `featdump` (0 or a value in [0, 1); anything else is a configuration error, exit 2). The rate was 0.10 for training, which is an assumption; no public namesake rate was used. For each sampled person, with probability 0.10 one synthetic namesake is added:
- Its names (Arabic, English, aliases), sex, country code and origin come from a real person in the same split who shares the base person's first English token (case-insensitive), origin and sex and has a different last English token. If no such person exists, no namesake is added.
- Its DOB is the base person's true DOB with one digit of YYYYMMDD replaced (probability 0.60) or two distinct digits replaced (0.40), each by a different digit. The result must be a valid date in 1900-2025 and differ from the base DOB; up to 20 redraws are made, after which no namesake is added (about 0.2% of DOB draws in unit tests; the rate in the training run was not recorded).
- Its Emirates ID is new and Luhn-valid.
- It is registered like any other person (same registration-count draw, providers, name noise, DOB noise and ID capture) using its own seeded stream, so the main stream is untouched.
- `-namesakes 0` (the default) leaves `featdump` output byte-identical, verified against SHA-256 goldens recorded before the change and pinned in a test; `gen` output for v1 and v2 was also verified byte-identical. Namesakes cannot reach generation, the runtime or evaluation.

The change adds two kinds of different-person training rows: base-namesake pairs (shared first name, different surname, DOB one or two digits off) and namesake-to-donor pairs (near-identical names, unrelated DOB, different ID). The stop floor and the namesake slice below count only the first kind.

**Seeds:** train 1019, 1021, 1031, 1033 and 1039 (5,000 persons each, namesakes 0.10) and cal 2011 (the whole cal split) were reused from `pairfeat2`, so namesakes are the only training change. Calibrators and parity use the plain cal dump (no namesakes), because deployment cohorts have none. A second cal dump with namesakes 0.10 was scored with the fitted model for a namesake slice report only. Paired development (dev/v2) seeds are 347, 353 and 367, fresh because 317, 331 and 337 were inspected in the `pairfeat2` diagnosis. Validation seeds 401, 409, 419, 421 and 431 were not touched.

**Namesake floor and counts:** the pre-registered stop check required at least 500 base-namesake candidate rows across the five train dumps; there were 5,199 (1,054, 1,068, 959, 1,058 and 1,060 per seed). The five dumps added 2,054 namesakes (412, 428, 396, 413 and 405). Fewer than the nominal 500 per seed were added; people without an eligible donor get none (the exact split of causes was not tabulated).

**Training and calibration metrics:** train had 1,660,380 retrieved pairs (45,492 same-person); plain cal had 139,084 (6,034 same-person). Cal pair AUC was 0.99993 and AP 0.99890. At raw LightGBM score >= 0.99 (an uncalibrated threshold, not the calibrated closure tier) cal pair precision was 0.9998 and recall 0.9108. Pair AUC is diagnostic, not the selection gate.

**Artifact identity and parity:** the training manifest is `6f66cd03`. Chain files: `lgb_stream_pairfeat3` `87c947b2`, `iso_pairfeat3` `6f94f565`, `iso_v7_pairfeat3` `378a7317`; `encoder_pairfeat3` `5e6284f8` is identical to the baseline encoder. The baseline chain is `lgb_stream` `13d3e272`, `iso` `17e434ad`, `iso_v7` `63ef7bd0`. The governing manifest `0dd0e3cc` bound the development lanes, and both arms used the same binary (`9d50c3a2`) at `f3a2f3c`. Go/Python scoring parity over all 139,084 cal rows had max |delta| 1.1e-16 (limit 1e-9).

**Hard-negative slices (plain cal split, seed 2011):**

| Slice | Rows | Same-person | Different-person | Different-person at raw >= 0.01 | At raw >= 0.7 |
|---|---:|---:|---:|---:|---:|
| near EID | 1,020 | 1,018 | 2 | 2 | 0 |
| decade-digit DOB | 144 | 39 | 105 | 0 | 0 |
| same surname | 18,781 | 4,986 | 13,795 | 115 | 11 |
| placeholder DOB | 78,253 | 1,657 | 76,596 | 145 | 16 |
| cross-script names | 45,349 | 3,021 | 42,328 | 161 | 11 |

Different-person pairs on plain cal that scored raw >= 0.7 numbered 23 for `pairfeat3` against 31 for `pairfeat2`. This is like for like (same cal seed, same plain cal dump; the helper was checked to reproduce `pairfeat2`'s 31). It is information only, not a gate, and it did not carry through to the development lanes (see the gate verdict).

**Namesake slice (diagnostic cal dump with namesakes 0.10, scored with the fitted model, information only):** 465 base-namesake rows, all different-person; 14 scored raw >= 0.01 and 1 scored >= 0.7. Read this slice with care. It mostly measures easy pairs: in a scratch sample of 216 base-namesake rows `pairfeat2` itself put only 2 at raw >= 0.7, so a small `pairfeat3` count is weak evidence of the mechanism. The harder new negatives are the namesake-to-donor pairs (near-identical names, unrelated DOB), which neither the floor nor this slice counts.

**Paired development clean (seeds 347, 353, 367; both arms `-linkfloor 0.01`, `v7strict`, CRC-only v2, cached readers, image off, FHIR, chaos 0, subkill 1):** exact TP is `pred_closed - false_closures`, not attach rate; truth-closed denominators match across arms.

| Seed | Baseline TP / truth | Baseline recall | `pairfeat3` TP / truth | `pairfeat3` recall | Gain |
|---|---:|---:|---:|---:|---:|
| 347 | 211/235 | 0.8979 | 223/235 | 0.9489 | +5.11pp |
| 353 | 187/222 | 0.8423 | 200/222 | 0.9009 | +5.86pp |
| 367 | 204/242 | 0.8430 | 220/242 | 0.9091 | +6.61pp |
| Pooled | 602/699 | 0.8612 | 643/699 | 0.9199 | **+5.8655pp** |

The mean paired per-seed gain was +5.8579pp with a 95% t interval of +3.9882 to +7.7276pp (n=3; small-n and descriptive only). The gain clears the +2.0pp recall criterion in every seed.

**Gate verdict: not selected.** The selection gate is unchanged and pre-registered: the recall gain, no increase in linkage contamination, and a distinct link-review pair count no more than 10% above baseline (the user kept the 1.10x limit for this experiment). Four reasons failed:
- False-merge pairs increased, 24 to 32 (per seed 10 to 12, 10 to 11, 4 to 9).
- Impure clusters increased, 23 to 27 (per seed 9 to 8, 10 to 10, 4 to 9).
- Cross-person edges increased, 23 to 28 (per seed 9 to 8, 10 to 11, 4 to 9). The rise is not in every seed: seed 347 fell by one on impure clusters and cross-person edges, but false-merge pairs rose in all three.
- Distinct link-review pairs rose 568 to 703 (+23.8%) against a cap of 624.8 (baseline plus 10%).

**Safety and workload:** false closures, wrong-patient releases, replay mismatches, analysis errors and trace-write errors were zero in both arms, with zero wrong final sign-offs and zero unanswered review items. Reviewer workload rose: items 1,116 to 1,270 and simulated reviewer hours 36.99 to 42.00 (per seed 12.60 to 13.72, 11.53 to 13.14, 12.86 to 15.14). The zero-safety result is n=3 development evidence and is not a validated safety property.

| Residual truth-closed misses | Baseline | `pairfeat3` |
|---|---:|---:|
| Total open | 97 | 56 |
| Attached, not closed | 11 | 12 |
| Below review | 56 | 12 |
| No candidate generated | 5 | 4 |
| Demographic rejection | 0 | 0 |
| Routed to review | 0 | 1 |
| Linked report not attached | 25 | 27 |

The scorer-addressable category (below review) fell from 56 to 12, which accounts for most of the recall gain (below review fell by 44; net open misses fell by 41); the categories a scorer change cannot reach did not improve materially ("no candidate generated" 5 to 4, "attached, not closed" 11 to 12, "linked report not attached" 25 to 27).

**Interpretation:** in development, adding namesake hard negatives did not remove the cost seen in `pairfeat2`. The like-for-like cal count of different-person pairs at raw >= 0.7 fell from 31 to 23, but in the development lanes contamination still rose (false-merge pairs 24 to 32) and link-review load rose by 23.8%. Cal-split pair counts are therefore not a reliable predictor of development contamination here. The development seeds differ from `pairfeat2`'s (347, 353, 367 against 317, 331, 337), so the `pairfeat2` and `pairfeat3` development contamination increases are not a like-for-like comparison, and no claim is made that one arm is better than the other. A read-only diagnosis of the contamination increase was run after the gate and is reported below. This is a development result from three seeds, not validation evidence, and no claim is made that it would generalize.

**Post-hoc diagnosis (read-only, after the gate; development lanes only; nothing was changed or re-run):** with the same method as the `pairfeat2` diagnosis, direct cross-person model edges (score >= 0.7) were compared between arms. `pairfeat3` added 14 cross-person edges that baseline did not have and removed 11 that baseline had, a net change of +3 (recomputed totals match `report7` on two seeds and are two edges above it for baseline on seed 353). Of the added 14, 11 had an Emirates ID on one side only and 3 had none; 9 were cross-script; in baseline 7 had been below the 0.01 floor, 6 in the review band and 1 rejected by the demographic check. Their DOBs: one digit apart 6, two digits apart 3, equal 3 and missing on one side 2; 5 of them pair YYYY-01-01 placeholder dates with a different year. Read by eye on all 14 (not a counted feature): 7 share the family-name token but differ in the given or middle name, only 1 shares just the given name (the shape the namesakes were built from), and 3 have identical DOBs with otherwise different names. The 11 removed edges are mostly equal DOB (5) or DOB missing on one side (4). The rise in false-merge pairs (+8) splits into 4 more impure clusters and 4 extra pairs from larger impure clusters: this is consistent with higher recall letting a wrong edge join clusters that already hold two or more registrations of one person (an untested reading). The impure-cluster change is concentrated in one seed (347: 9 to 8; 353: 10 to 10; 367: 4 to 9). Link-review load is again mostly the recall gain: of 166 link-review pairs present only in `pairfeat3`, 165 are same-person; across all 166, 126 were below the floor in baseline, 12 were in the review band and 28 were already linked; 31 baseline review pairs were no longer asked. Reading: some removed edges are of the targeted shape (shared given name, different family name), though this was not counted; the namesake training set was built from that one shape only, whereas about half of the remaining false edges are the complementary shape (shared family name, different given name, as with relatives) or placeholder DOBs with year differences. This is a diagnosis of three development seeds with small counts, not a tested fix, and no change is authorized from it.

**Not run:** per the pre-registered stop rule, synthetic reviewer stress and untouched validation were **not run**. No validation manifest was frozen and eval seeds 401, 409, 419, 421 and 431 remain untouched. No threshold, gate criterion, feature, hyperparameter, seed or analysis rule was changed after seeing results.

**Result-affecting rulings:**
- The namesake DOB draws the one-or-two-digit count once, before the redraw loop, and redraws only positions and digits up to 20 times. The plan's code redrew the count on each retry, which gave a one-digit share of about 0.77 against the approved 0.60; the approved design text was followed.
- The 0.10 namesake rate is an assumption, fixed before the run and not tuned.
- Namesakes are training-only and refused outside `featdump`, so simulated cohorts, the runtime and evaluation noise are unaffected.
- The plain cal dump (no namesakes) feeds the calibrators and parity; the namesake cal dump feeds only the information-only slice.

**Process rulings and deviations:**
- Go task A and Python tasks B and C ran in parallel, since they touched disjoint files; the cost was only commit interleaving.
- Task C edited seven existing harness assertions, because making `pairfeat3` a real candidate necessarily changes the exact candidate-set checks; the reviewer confirmed intent was preserved.
- A final cross-task review found no must-fix items before the run. It also pointed out the namesake-to-donor rows and the slice limits described above, and these are disclosed here.
- Deferred minor review findings (none affect results): `validateNamesakes` accepts NaN (the driver only sends 0.1); the stderr person count excludes namesakes; the training manifest is written after the train dumps for `pairfeat3` (HEAD did not move during the run); `code_sha256` omits the namesake and main Go files (the recorded git head pins them); the namesake slice reader has no finiteness or range assertions and the namesake-cal sidecar hash is not recorded; a missing train sidecar gives a raw traceback; the driver's main flow is covered end to end by the run rather than by a unit test; some docstrings still list only `pairfeat1` and `pairfeat2`; the Task A rows test checks only label 0 (the sidecar row set was independently verified against person identifiers in the final review); there is no end-to-end `pairfeat3` fixture test (the equivalent check was run by hand in the final review).
- Task B edited two existing driver tests, as its brief required.
- Strict same-HEAD binding was kept, so HEAD stayed at `f3a2f3c` for the whole run.

**Scope:** the model applies globally but these cohorts are CRC-only. No non-CRC safety or recall claim is made, and simulated results establish no clinical effectiveness. The default model and default floor 0.2 are unchanged. No further seeds, thresholds, features, hyperparameters or noise variants are authorized for this arm.

### CRC symmetric-namesake pair-scorer experiment at `a643daf` (not selected)

**Outcome:** the experiment failed its pre-registered development selection gate on contamination and stopped as specified. `pairfeat4` is **not a candidate**. It is the successor to the `pairfeat3` experiment above, motivated by that diagnosis: about half of the new false edges there shared a family name with a different given name (the shape of relatives), the complementary shape to the given-name namesakes `pairfeat3` trained on. The hypothesis was that adding family-name namesakes to the `pairfeat3` recipe would keep the recall gain and remove the remaining false merges. The recall gain was kept (+5.8394pp pooled), and the new wrong-pair review gate passed (7 to 4), but false-merge pairs rose 38 to 41 and cross-person edges 36 to 37, so the gate failed on those two reasons. Impure clusters were unchanged (35 to 35). The default model and the default review floor of 0.2 are unchanged. All results below are simulated, development-only and descriptive (n=3 seeds); they prove no clinical effectiveness.

**Built:** an opt-in `pairfeat4` modelset (code at `a643daf`) uses the same 30-feature layout, fixed encoder, `v1t` training noise, LightGBM configuration and calibrator procedure as `pairfeat3` (400 rounds, 63 leaves, learning rate 0.05, seed 1, no hyperparameter search). The only intended change is the training data. Runtime, features, retrieval, merge 0.7, closure tier 0.99, the uncorroborated cap and the identity, consent and attachment rules are unchanged. `pairfeat4` is accepted only with `v7strict`. Generated model files are git-ignored.

**Training data, family-name namesakes (training only):** `featdump` gained a `-familynamesakes` rate beside `-namesakes`, with the same validation (0 or a value in [0, 1) and only for `featdump`; anything else is a configuration error, exit 2). The rate was 0.10 for training, which is an assumption; no public rate was used. For each sampled person, with probability 0.10 one synthetic family-name namesake is added:
- Its names (Arabic, English, aliases), sex, country code and origin come from a real person in the same split who shares the base person's last English token (case-insensitive), origin and sex and has a different first English token. If no such person exists, none is added.
- Its DOB is the base person's true DOB with one digit of YYYYMMDD replaced (probability 0.60) or two distinct digits replaced (0.40), built exactly as in the given-name kind (digit count drawn once, up to 20 redraws, valid date in 1900-2025, differs from the base DOB).
- Its Emirates ID is new and Luhn-valid.
- It is registered like any other person using its own seeded stream, independent of the given-name stream and registered after it, so the given-name draws and the main stream are untouched.
- Given-name namesakes are unchanged at 0.10; the two kinds are independent, so a person can get one, both or neither.
- `-familynamesakes 0` (the default) leaves `featdump` output byte-identical, verified against SHA-256 goldens recorded before the change (including a `-namesakes 0.1` run) and pinned in a test; `gen` output for v1 and v2 was also verified byte-identical. Namesakes cannot reach generation, the runtime or evaluation.
- Not addressed by either kind: identical-DOB pairs with otherwise different names, and sex mismatches.

**Seeds:** train 1019, 1021, 1031, 1033 and 1039 (5,000 persons each, both kinds at 0.10) and cal 2011 (the whole cal split) are the `pairfeat3` seeds, so the family kind is the only training change. Calibrators and parity use the plain cal dump (no namesakes), because deployment cohorts have none. A second cal dump with both kinds at 0.10 was scored with the fitted model for slice reports only. Paired development (dev/v2) seeds are 433, 449 and 461, fresh. They differ from the earlier experiments' development seeds (`pairfeat2` 317, 331, 337; `pairfeat3` 347, 353, 367), so cross-experiment development comparisons are not like for like. Validation seeds 401, 409, 419, 421 and 431 were not touched.

**Namesake floor and counts:** the pre-registered stop check required at least 500 base-namesake candidate rows of each kind across the five train dumps. There were 5,169 given-name rows (1,042, 1,059, 960, 1,060 and 1,048 per seed) and 3,297 family-name rows (690, 784, 664, 547 and 612). The dumps added 2,054 given-name namesakes (412, 428, 396, 413 and 405) and 1,284 family-name namesakes (249, 292, 261, 229 and 253). Given-name namesake counts match `pairfeat3`'s; row counts differ slightly because adding family namesakes changes the registration set and the shuffle.

**Training and calibration metrics:** train had 1,762,366 retrieved pairs (47,645 same-person); plain cal had 139,084 (6,034 same-person). Cal pair AUC was 0.99991 and AP 0.99642 (`pairfeat3`: 0.99993 and 0.99890). At raw LightGBM score >= 0.99 (an uncalibrated threshold, not the calibrated closure tier) cal pair precision was 0.9996 and recall 0.8941 (`pairfeat3`: 0.9998 and 0.9108). Pair AUC is diagnostic, not the selection gate.

**Artifact identity and parity:** the training manifest is `c371897e`. Chain files: `lgb_stream_pairfeat4` `b9433674`, `iso_pairfeat4` `92b1af69`, `iso_v7_pairfeat4` `fe76d2c8`; `encoder_pairfeat4` `5e6284f8` is identical to the baseline encoder. The baseline chain is `lgb_stream` `13d3e272`, `iso` `17e434ad`, `iso_v7` `63ef7bd0`. The governing manifest `0f0903d3` bound the development lanes, and both arms used the same binary (`81553cc0`) at `a643daf`. Go/Python scoring parity over all 139,084 cal rows had max |delta| 2.2e-16 (limit 1e-9).

**Hard-negative slices (plain cal split, seed 2011):**

| Slice | Rows | Same-person | Different-person | Different-person at raw >= 0.01 | At raw >= 0.7 |
|---|---:|---:|---:|---:|---:|
| near EID | 1,020 | 1,018 | 2 | 1 | 0 |
| decade-digit DOB | 144 | 39 | 105 | 0 | 0 |
| same surname | 18,781 | 4,986 | 13,795 | 120 | 12 |
| placeholder DOB | 78,253 | 1,657 | 76,596 | 165 | 17 |
| cross-script names | 45,349 | 3,021 | 42,328 | 168 | 12 |

Different-person pairs on plain cal that scored raw >= 0.7 numbered 31 for `pairfeat2`, 23 for `pairfeat3` and 26 for `pairfeat4`. This is like for like (same plain cal dump, seed 2011) and information only, not a gate. The count did not fall further here.

**Namesake slices (diagnostic cal dump with both kinds at 0.10, scored with the fitted model, information only):** 457 given-name base rows, all different-person; 17 scored raw >= 0.01 and 1 scored >= 0.7. 214 family-name base rows, all different-person; 6 scored >= 0.01 and none >= 0.7. For a same-dump comparison, the `pairfeat3` model on the same `pairfeat4` diagnostic dump scored 14 at 0.01 and 1 at 0.7 on the given-name rows, and 24 at 0.01 and 0 at 0.7 on the family-name rows. So the family-name rows were scored lower by `pairfeat4` than by `pairfeat3` at the review floor (6 against 24), while the given-name rows moved slightly the other way (17 against 14). These dumps differ from `pairfeat3`'s own diagnostic dump (465 given-name rows there), so `pairfeat3`'s recorded slice is not the comparator. As before, these slices mostly measure easy pairs and are weak evidence of mechanism.

**Paired development clean (seeds 433, 449, 461; both arms `-linkfloor 0.01`, `v7strict`, CRC-only v2, cached readers, image off, FHIR, chaos 0, subkill 1):** exact TP is `pred_closed - false_closures`, not attach rate; truth-closed denominators match across arms.

| Seed | Baseline TP / truth | Baseline recall | `pairfeat4` TP / truth | `pairfeat4` recall | Gain |
|---|---:|---:|---:|---:|---:|
| 433 | 210/249 | 0.8434 | 227/249 | 0.9116 | +6.83pp |
| 449 | 189/219 | 0.8630 | 203/219 | 0.9269 | +6.39pp |
| 461 | 188/217 | 0.8664 | 197/217 | 0.9078 | +4.15pp |
| Pooled | 587/685 | 0.8569 | 627/685 | 0.9153 | **+5.8394pp** |

The mean paired per-seed gain was +5.7892pp with a 95% t interval of +2.2163 to +9.3620pp (n=3; small-n and descriptive only). The gain clears the +2.0pp recall criterion in every seed.

**Pre-registered review-load gate change (this experiment only):** before any run, and approved by the user together with the design, the review-load criterion for `pairfeat4` changed from the total distinct link-review pair count to the count of different-person distinct link-review pairs (pairs whose two registrations belong to different persons in the offline truth). The limit is at most 1.10x baseline, or at most 1 when the baseline is 0. The total-pair ratio and extra reviewer hours per extra true closure are reported but do not gate. The reason was that in the `pairfeat2` and `pairfeat3` lanes almost all extra link-review pairs were same-person (148 of 150 and 165 of 166), so the total-pair gate mostly penalised the recall gain itself. The change is for `pairfeat4` only; `pairfeat1` to `pairfeat3` keep the total-pair gate and are not re-judged. The other gates (recall gain, contamination) are unchanged.

**Gate verdict: not selected.** Two reasons failed:
- False-merge pairs increased, 38 to 41 (per seed 10 to 10, 11 to 16, 17 to 15).
- Cross-person edges increased, 36 to 37 (per seed 9 to 9, 10 to 13, 17 to 15). The rise is concentrated in one seed (449); seeds 433 and 461 did not rise.

Passed: recall gain (+5.8394pp against 2.0pp); impure clusters 35 to 35 (per seed 8 to 9, 10 to 12, 17 to 14); and the wrong-pair review gate, with different-person distinct link-review pairs falling from 7 to 4 against a limit of 7.7 (per seed 3 to 2, 3 to 2, 1 to 0). Reported, not gating: distinct link-review pairs rose 522 to 638 (ratio 1.22; the old total-pair limit would have been 574.2, so the old gate would also have failed), and extra reviewer hours per extra true closure were 0.116 (39.788 minus 35.144 hours over 40 extra closures).

**Disclosure about the new gate:** the final cross-task review observed that at these small counts the wrong-pair limit allows almost no net new wrong-pair review. With a baseline of about 2 such pairs the limit is 2.2, so one extra wrong pair fails; the rule is also not monotone (baseline 0 allows 1, baseline 1 allows 1). Re-run on `pairfeat3`'s lanes the gate would have failed (3 wrong pairs against a limit of 2.2). The controller kept the approved gate exactly as approved, since changing a pre-registered gate overnight without the user would have been an unapproved change. Here the gate passed with margin (4 against 7.7), so it did not decide the outcome; the failure came from the two contamination criteria.

**Safety and workload:** false closures, wrong-patient releases, replay mismatches, analysis errors and trace-write errors were zero in both arms, with zero wrong final sign-offs and zero unanswered review items. Reviewer workload rose: items 1,097 to 1,244 and simulated reviewer hours 35.14 to 39.79 (per seed 12.09 to 13.57, 11.75 to 13.49, 11.30 to 12.72). The zero-safety result is n=3 development evidence and is not a validated safety property.

| Residual truth-closed misses | Baseline | `pairfeat4` |
|---|---:|---:|
| Total open | 98 | 58 |
| Attached, not closed | 12 | 12 |
| Below review | 48 | 8 |
| No candidate generated | 4 | 5 |
| Demographic rejection | 1 | 0 |
| Routed to review | 1 | 0 |
| Linked report not attached | 32 | 33 |

The scorer-addressable category (below review) fell from 48 to 8, which accounts for the recall gain (net open misses fell by 40, matching the 40 extra exact closures); the categories a scorer change cannot reach did not improve ("no candidate generated" 4 to 5, "attached, not closed" 12 to 12, "linked report not attached" 32 to 33).

**Interpretation:** in development, adding family-name namesakes did not remove the contamination cost: false-merge pairs rose by 3 and cross-person edges by 1, small counts from three seeds. Under the new gate the review-load result passed: of the 150 pairs asked only in `pairfeat4`, 147 are same-person, and different-person review pairs fell (7 to 4). The old total-pair gate would still have failed; the workload rise tracks the recall gain. The like-for-like plain-cal count at raw >= 0.7 rose from 23 to 26 and development false-merge pairs rose by 3; with three seeds and different development seeds from `pairfeat3`, no relationship between the cal count and development contamination is claimed. The development seeds differ from the earlier experiments', so no claim is made that `pairfeat4` is better or worse than `pairfeat3` on contamination. This is a development result from three seeds, not validation evidence, and no claim is made that it would generalize.

**Post-hoc diagnosis (read-only, after the gate; development lanes only; nothing was changed or re-run):** with the same method as the `pairfeat3` diagnosis, direct cross-person model edges (score >= 0.7) were compared between arms. `pairfeat4` added 18 cross-person edges that baseline did not have and removed 18 that baseline had, a recomputed net change of 0 (37 against 37). The harness reports 36 against 37 (+1): the recomputed `pairfeat4` totals match the report on all three seeds, but the recomputed baseline is one edge above the report on seed 433 (10 against 9), which accounts for the difference. Of the added 18, 10 had an Emirates ID on one side only and 8 had none; 13 were cross-script; in baseline 8 had been below the 0.01 floor, 7 in the review band, 2 rejected by the demographic check and 1 already linked transitively. Their DOBs: 8 of the 18 pair a YYYY-01-01 placeholder date with a different year, 5 are missing on one side, 2 are equal, 2 are one digit apart and 1 is two digits apart. Six of the 18 scored >= 0.9999, five of them with weak name similarity (read by eye, not a counted feature); none of `pairfeat3`'s added edges scored that high (its maximum was about 0.98).
- On the plain cal dump, `pairfeat4` has 2 different-person rows at raw >= 0.9999, where `pairfeat2` and `pairfeat3` have 0. Both rows have both DOBs present, the same month and day, a year gap capped at 10, an equal surname skeleton, a full-name Jaro-Winkler similarity of 0.665 and no EID or DOB extras; `pairfeat3` scores them about 1e-7.
- Training data does not explain it by label balance. In training seed 1019, rows with that feature pattern held 268 positive and 10,315 negative pairs for `pairfeat4` (263 and 9,696 for `pairfeat3`); rows of the pattern with low name similarity held 10 positive and about 9,000 negative in both. The cause is unknown; tree extrapolation outside the trained feature region is suspected but untested.
- Clusters: the rise in false-merge pairs (38 to 41) is 3 extra pairs from larger impure clusters; the impure-cluster count did not change (35 to 35).
- Review: of 150 link-review pairs present only in `pairfeat4`, 147 are same-person and 3 are different-person; 123 were below the floor in baseline, 14 were in the review band and 13 were already linked; 34 baseline review pairs were no longer asked.
- Reading: `pairfeat4` scored fewer of the family-name rows at the review floor than `pairfeat3` did on the same dump (6 against 24); whether the family-name training data caused this was not tested, and 'relative-shaped' is a descriptive label, not a verified property but 8 of the 18 new false edges are placeholder-DOB pairs with a year gap, a shape neither namesake kind covers; the rest are missing, equal or one-to-two-digit-off DOBs, and some scored very high; the training labels checked (seed 1019) show no imbalance that explains it, and the cause is unknown. This is a diagnosis of three development seeds with small counts, not a tested fix, and no change is authorized from it.

**Not run:** per the pre-registered stop rule, synthetic reviewer stress, the validation freeze and untouched validation were **not run**. No validation manifest was frozen and eval seeds 401, 409, 419, 421 and 431 remain untouched. No threshold, gate criterion, feature, hyperparameter, seed or analysis rule was changed after seeing results.

**Result-affecting rulings:**
- The 0.10 family-name namesake rate is an assumption, fixed before the run and not tuned. The given-name rate stays at 0.10.
- The family kind uses the same DOB construction as the given-name kind (digit count drawn once, before the redraw loop), as approved.
- The review-load gate change above is pre-registered and approved before any run, and applies to `pairfeat4` only. The controller kept it unchanged after the final review pointed out its small-count behaviour (ledger ruling).
- An unmapped registration in the wrong-pair count aborts the analysis before any write-once file is written, rather than counting as an analysis error, so the decision stays re-runnable; this did not occur.
- Namesakes are training-only and refused outside `featdump`, so simulated cohorts, the runtime and evaluation noise are unaffected.
- The plain cal dump (no namesakes) feeds the calibrators and parity; the diagnostic cal dump feeds only the information-only slices.

**Process rulings and deviations:**
- Go task A (`featdump` flag), Python task B (training driver) and Python task C (harness gate) ran in parallel, since they touched disjoint files; the cost was only commit interleaving.
- Task C edited existing harness assertions only where making `pairfeat4` a real candidate changes exact candidate-set checks; the reviewer confirmed intent was preserved.
- A final cross-task review found no must-fix items before the run. It also supplied the same-dump `pairfeat3` slice comparator and the note on the gate's small-count behaviour, both disclosed here.
- Deferred minor review findings (none affect results): some test and doc comments are stale or slightly inaccurate; a missing family sidecar key would raise a bare `KeyError` (the real sidecar carries every key); `check_namesake_floor` returns a tuple for `pairfeat4` and an int otherwise; the zero-baseline failure reason text prints "+10%" although the limit is the absolute 1; no test covers the wrong-pair gate with a total baseline of 0; the arms' `truth.json` files are not cross-checked in the analyser (their hashes were identical in the review on `pairfeat3` lanes); the base manifest's `code_sha256` omits `namesake.go` and `main.go` (the recorded git head pins them); there is no permanent end-to-end test of the driver (a reduced-scale smoke run covered it in the final review).
- Strict same-HEAD binding was kept, so HEAD stayed at `a643daf` for the whole run.

**Scope:** the model applies globally but these cohorts are CRC-only. No non-CRC safety or recall claim is made, and simulated results establish no clinical effectiveness. The default model and default floor 0.2 are unchanged. No further seeds, thresholds, features, hyperparameters or noise variants are authorized for this arm.

**Post-hoc follow-up (after this section was written; training-side data, nothing above re-judged):** the very high `pairfeat4` scores (six added cross-person edges at >= 0.9999, two plain-cal different-person rows at that level) were traced to the fit, not the training labels. For both plain-cal rows, one feature (name-skeleton similarity) contributed +62.8 log-odds, +44.0 of it (about 70%) from a single leaf that held 117 training rows with 1 positive whose pre-tree score was p = 4.5e-15 (leaf hessian sum 0.0013). `pairfeat4` was fitted with LightGBM's default `lambda_l2 = 0`, so such low-hessian leaves took unregularised Newton steps: the largest |leaf value| in `pairfeat4` is 3,337.6 (70 leaves above 5), against 7.8 for the baseline scorer, 12.8 for `pairfeat2` and 13.2 for `pairfeat3`. The `pairfeat5` refit of the same training recipe with `lambda_l2 = 1` has a largest |leaf value| of 3.63. That experiment is reported in the next section.

### CRC regularised pair-scorer with cross-script review cap at `56d31fd` (selected under the amended gate; not promoted to default)

**Outcome:** `pairfeat5` passed the development selection gate, synthetic reviewer stress and the frozen five-seed validation, all under a cap-aware review gate that was amended and pre-registered before any `pairfeat5` development lane ran (Amendment 1 below). In paired simulated CRC validation cohorts pooled recall rose from 0.8683 to 0.9301 (+6.18pp pooled; mean paired per-seed gain +6.19pp, 95% t interval +4.72 to +7.67pp, n=5) and every contamination count fell (false-merge pairs 173 to 87). The run of record (run-2, HEAD `56d31fd`) is a disclosed second pass: a first run (run-1) stopped during validation on a classifier gap, and the whole chain was re-run after a diagnostic-only fix. The cap has a cost the CRC recall gate does not measure: capped same-person pairs left unmerged and unreviewed outnumbered capped different-person pairs (validation 269 against 80), which splits identities away from CRC loops. That cost is reported below as an open product question, not a gate failure. The default model, the default noise and the default review floor of 0.2 are unchanged; nothing was promoted to default. All results are simulated; they prove no clinical effectiveness.

**Design motivation (post-hoc `pairfeat4` investigation, disclosed):** the investigation in the follow-up paragraph above found the near-certain `pairfeat4` scores came from unregularised leaves. An exploratory refit of the `pairfeat4` training CSVs with `lambda_l2 = 1` removed every leaf above 5 (largest 3.6) and lowered the plain-cal count of different-person rows at raw >= 0.7 from 26 to 21 (with `min_sum_hessian_in_leaf = 1` added as well: 23), with 0 rows at >= 0.9999. Across 12 baseline development lanes from the earlier experiments (already used; read only), none of the 122 false model edges were exact twins (same source name and DOB); 81 were cross-script, 106 had an Emirates ID on one side only and 16 on neither, and 65 had a DOB missing on one side. On the regularised cal scores, cross-script pairs with the ID not valid on both sides and DOBs not exactly equal held 10 of the 21 different-person rows at >= 0.7 and 535 of 5,876 same-person rows at >= 0.7. The cap rule's shape was taken from the false edges on those already-used development lanes of earlier experiments (not `pairfeat5`'s seeds); the rule and the `lambda_l2` value were then checked on these cal-split (training-side) figures. `pairfeat5`'s own development seeds were fresh.

**Built:** an opt-in `pairfeat5` modelset with two changes.
- Fit (code at `c3bd6c7`, `7390a99`): the `pairfeat4` training protocol (same 30 features, fixed encoder, `v1t` noise, both namesake kinds at 0.10, calibrators and parity on the plain cal dump, 400 rounds, 63 leaves, learning rate 0.05, seed 1) with LightGBM `lambda_l2 = 1.0`; all other parameters unchanged. `fit_scorer.py` without `--lambda-l2` is byte-identical to before. A stop check required no leaf with |value| > 5 (read from the fit report).
- Cross-script review cap (runtime, code at `d07e8a7`, `pairfeat5` only; off for every other modelset including baseline): for a model candidate pair whose registrations differ in script (feature 20 is 0), whose Emirates ID is not Luhn-valid on both sides and whose DOBs are not exactly equal (feature 13 is not 1; a missing DOB counts as not equal), a model score above 0.69 is capped at 0.69, below the 0.7 merge threshold. The pair cannot merge on the model alone; above the review floor it becomes a near (review-band) candidate. The cap sits after the Emirates ID handling, so a forced equal-EID link is unaffected. Capped pairs are traced with `reason: "xscript_cap"`.
- `pairfeat5` is accepted only with `v7strict`. Retrieval, features, merge 0.7, closure tier 0.99, the uncorroborated cap and the identity, consent and attachment rules are unchanged. Generated model files are git-ignored.

**Seeds:** train 1019, 1021, 1031, 1033 and 1039 (5,000 persons each) and cal 2011 are `pairfeat4`'s, so the fit change is `lambda_l2` only. Paired development (dev/v2) seeds 521, 541 and 547 were fresh. Validation used eval/v2 seeds 401, 409, 419, 421 and 431 after development and stress passed (see the run history for their first, partial use in run-1).

**Amendment 1, cap-aware review gate (pre-registered before any `pairfeat5` development lane):** the approved design kept `pairfeat4`'s wrong-pair review gate (different-person distinct link-review pairs at most 1.10x baseline, or 1 when the baseline is 0). Before any `pairfeat5` development lane, a read-only estimate on the already-used `pairfeat4` development lanes (seeds 433, 449, 461; nothing tuned) found that 20 of the 37 different-person model edges at >= 0.7 in the `pairfeat4` arm met the cap rule (approximated from registration fields). If each became a different-person review pair, the expected `pairfeat5` count was about 24 against a limit of 7.7 (baseline 7), so the original gate would have counted every prevented false merge as review harm and failed by construction. The estimate also found 222 of 2,656 same-person edges met the rule (about 74 per lane). The user chose to keep the cap, and with the user's approval the gate `wrong_pair_cap` was pre-registered in the plan (`09524fe`) and the harness (`3af6821`) before seeds 521, 541 and 547 were used. For `pairfeat5` only (`pairfeat4` keeps `wrong_pair`):
- Recall, safety, false-merge pairs, impure clusters and cross-person edges gates unchanged.
- Uncapped wrong-pair reviews (different-person link-review pairs without an `xscript_cap` record, candidate) at most 1.10x the baseline's wrong-pair reviews (at most 1 when that is 0).
- Prevented false merges (baseline minus candidate false-merge pairs) at least 1.
- Capped wrong-pair reviews (candidate) at most 2 per prevented false merge.
- The baseline trace must contain no `xscript_cap` record (else analysis aborts before any write).
- Reported, not gating: capped same-person pairs, total distinct review pairs and ratio, extra reviewer hours per extra true closure, full capped-pair counts.

The estimate turned out wrong. Capped wrong-pair reviews were 0 in every lane, because the review queue asks only the link questions an open loop depends on and no capped different-person pair was asked: none of the 24 (development) or 81 (validation) different-person capped pairs became a link-review pair, and in the development diagnosis 279 of 374 capped pairs were never queued (details below). Under the original `wrong_pair` gate `pairfeat5` would also have passed (1 wrong-pair review against a limit of 3.3 in development and in stress, 4 against 6.6 in validation), so the amendment did not decide the outcome.

**Run history, in order (all artifacts preserved):**
- **run-0-preamend (HEAD `d07e8a7`, information only).** The fit started before the amendment. When the user asked to check the review-gate risk, the driver was stopped during the fit, before any development lane; the fit itself (training-side only) completed. Because HEAD then moved to `09524fe`, its artifact manifest could not bind later lanes. It is kept for information: namesake floors passed, largest |leaf| 3.63, plain-cal 21. No lane used it.
- **Refused run-1 attempt (HEAD `3af6821`).** The driver refused before writing anything because the run-0 model files were still in `models/` (overwrite refusal). They were moved aside and the driver relaunched.
- **run-1 (HEAD `3af6821`).** Fit identical to run-0 (same four chain-file hashes). Development clean passed and stress passed (figures equal to run-2's below except distinct link-review pairs, which differed by 1 or 2 per arm, review items and reviewer hours). The validation manifest was frozen at `3af6821`. The validation baseline arm completed (956/1101) and the `pairfeat5` arm completed seed 401; seed 409 ran through `orchestrate`, `audit` and `report7`, then the funnel classifier stopped on it with an analysis error: for one CRC loop the expected true pair had a single trace record, stage near at p 0.69 with reason `xscript_cap`, no link-review item and no shared cluster. The cap had left a loop-relevant true pair neither merged nor queued, a state the classifier treated as impossible. Candidate seeds 419, 421 and 431 were not run. This was not a gate failure; the protocol could not complete at `3af6821`.
- **Classifier fix (`56d31fd`, user-approved).** `scripts/link_funnel.py` gained the funnel category `candidate_capped_not_queued` (detail `capped_near_without_review_item`), returned only when the latest record is near with reason `xscript_cap` and there is no review item; any other near-without-item case still raises. The category is a residual (open-loop) category for diagnosis; no gate reads it. `pairfeat1`-`pairfeat4` outputs are byte-identical (pinned by a test). On the preserved run-1 seed-409 lane it gave 0 analysis errors and 1 capped-not-queued loop.
- **run-2 (HEAD `56d31fd`, run of record, user-approved).** The protocol binds fit, development, stress, freeze and validation to one HEAD, so the whole chain was re-run. The fit was identical again. Development and stress seeds were therefore run a second time with the same model and the same linker code (only the classifier changed). Validation seeds were partly seen once before: in run-1 the baseline arm was complete and its 956/1101 was seen; `pairfeat5` seed 401 had completed and seed 409 had run through `orchestrate`, `audit` and `report7` before the classifier stopped. No run-1 `pairfeat5` validation recall or gate figure was read, but one capped loop's trace on seed 409 was read to diagnose the stop. The run-2 baseline validation arm reproduced run-1's 956/1101. This is a disclosed second pass over those seeds, not untouched validation in the strict sense.
- **run-2 stress lane crash (set aside, re-run).** The baseline stress lane for seed 541 failed in `orchestrate` with a NATS "no servers available" panic, and the resume then failed at `go build` with "no space left on device" (203 MiB free). The panic was attributed to the same full-disk condition (attributed, not proven). With user approval, disk space was freed (the Go build cache and unreferenced earlier scale-run outputs), the partial baseline stress lanes for seeds 521 and 541 were kept aside, the baseline stress arm was run again from seed 521, and the `pairfeat5` stress arm, which had never started, was then run, before the freeze and validation.

| run-1 against run-2 (same seeds, same model) | Recall run-1 / run-2 | Distinct link-review pairs run-1 / run-2 | Items run-1 / run-2 | Reviewer hours run-1 / run-2 |
|---|---:|---:|---:|---:|
| Development clean, baseline | 605/719 / 605/719 | 544 / 546 | 1,090 / 1,088 | 35.477 / 35.418 |
| Development clean, `pairfeat5` | 653/719 / 653/719 | 550 / 549 | 1,119 / 1,117 | 36.511 / 36.466 |
| Development stress, baseline | 547/719 / 547/719 | 543 / 545 | 1,083 / 1,084 | 35.449 / 35.460 |
| Development stress, `pairfeat5` | 592/719 / 592/719 | 548 / 549 | 1,114 / 1,113 | 36.504 / 36.484 |

Recall, contamination counts, capped-pair counts and gate decisions were identical between runs; only distinct link-review pairs (by 1 or 2 per arm), review items (by 1 or 2 per arm) and reviewer hours moved. The binary hash also differs (`35a290da` at `3af6821`, `f489224d` at `56d31fd`), although the linker source is unchanged between them: the `3af6821..56d31fd` diff touches only `scripts/`, so the hash difference is presumably Go's embedded VCS stamp. Task A had already found that the traced `orchestrate` lane is not byte-reproducible between two runs of the same binary, which is the likely source; this was not isolated.

**Training and calibration metrics:** train had 1,762,366 retrieved pairs (47,645 same-person); plain cal had 139,084 (6,034 same-person). Namesake floors passed with the same counts as `pairfeat4`: 5,169 given-name and 3,297 family-name base rows (floor 500 each); 2,054 given-name and 1,284 family-name namesakes added. Largest |leaf value| 3.634 (limit 5). Cal pair AUC was 0.99991 and AP 0.99895 (`pairfeat4`: 0.99991 and 0.99642). At raw LightGBM score >= 0.99 (an uncalibrated threshold, not the calibrated closure tier) cal pair precision was 1.0000 and recall 0.8918 (`pairfeat4`: 0.9996 and 0.8941). Pair AUC is diagnostic, not the selection gate.

**Artifact identity and parity:** the training manifest is `13d7226d`. Chain files: `lgb_stream_pairfeat5` `41cbb8f3`, `iso_pairfeat5` `782ef505`, `iso_v7_pairfeat5` `e454b022`; `encoder_pairfeat5` `5e6284f8` is identical to the baseline encoder. The same four hashes were produced in run-0, run-1 and run-2. The baseline chain is `lgb_stream` `13d3e272`, `iso` `17e434ad`, `iso_v7` `63ef7bd0`. In run-2 the governing manifest `bab8ea26` bound every lane, both arms used the same binary (`f489224d`) at `56d31fd`, the selection decision is `9be7428f`, the stress decision `baf5f9ba` and the validation manifest `74a131b5`. Go/Python scoring parity over all 139,084 cal rows had max |delta| 1.1e-16 (limit 1e-9).

**Hard-negative slices (plain cal split, seed 2011):**

| Slice | Rows | Same-person | Different-person | Different-person at raw >= 0.01 | At raw >= 0.7 |
|---|---:|---:|---:|---:|---:|
| near EID | 1,020 | 1,018 | 2 | 2 | 0 |
| decade-digit DOB | 144 | 39 | 105 | 1 | 0 |
| same surname | 18,781 | 4,986 | 13,795 | 134 | 9 |
| placeholder DOB | 78,253 | 1,657 | 76,596 | 184 | 15 |
| cross-script names | 45,349 | 3,021 | 42,328 | 199 | 11 |

Different-person pairs on plain cal that scored raw >= 0.7 numbered 31 for `pairfeat2`, 23 for `pairfeat3`, 26 for `pairfeat4` and 21 for `pairfeat5` (same plain cal dump; information only, not a gate; these are raw scores before the runtime cap). Against `pairfeat4`, every slice's count at >= 0.7 fell or stayed level and every slice's count at >= 0.01 rose (for example cross-script 168 to 199), consistent with regularisation pulling extreme scores toward the middle. Namesake slices (diagnostic cal dump, information only): 457 given-name base rows, 18 at >= 0.01 and 1 at >= 0.7; 214 family-name base rows, 8 at >= 0.01 and 0 at >= 0.7 (`pairfeat4` on a dump with the same namesake sidecar: 17 and 1, 6 and 0).

**Paired development clean (seeds 521, 541, 547; both arms `-linkfloor 0.01`, `v7strict`, CRC-only v2, cached readers, image off, FHIR, chaos 0, subkill 1):** exact TP is `pred_closed - false_closures`, not attach rate; truth-closed denominators match across arms.

| Seed | Baseline TP / truth | Baseline recall | `pairfeat5` TP / truth | `pairfeat5` recall | Gain |
|---|---:|---:|---:|---:|---:|
| 521 | 196/234 | 0.8376 | 213/234 | 0.9103 | +7.26pp |
| 541 | 211/248 | 0.8508 | 228/248 | 0.9194 | +6.85pp |
| 547 | 198/237 | 0.8354 | 212/237 | 0.8945 | +5.91pp |
| Pooled | 605/719 | 0.8414 | 653/719 | 0.9082 | **+6.6759pp** |

The mean paired per-seed gain was +6.6757pp with a 95% t interval of +4.9456 to +8.4058pp (n=3; small-n and descriptive only).

**Selection gate verdict: passed** (`wrong_pair_cap`):
- Recall gain +6.6759pp against +2.0pp.
- False-merge pairs fell, 52 to 19 (per seed 11 to 5, 8 to 2, 33 to 12).
- Impure clusters fell, 37 to 16 (per seed 11 to 5, 7 to 2, 19 to 9).
- Cross-person edges fell, 42 to 17 (per seed 11 to 5, 8 to 2, 23 to 10). Contamination fell in every seed.
- Uncapped wrong-pair reviews 3 to 1 against a limit of 3.3 (per seed 1 to 1, 2 to 0, 0 to 0).
- Prevented false merges 33 (at least 1 required); capped wrong-pair reviews 0 against a limit of 66.
- Reported, not gating: capped same-person pairs 350; distinct link-review pairs 546 to 549 (ratio 1.005; the old total-pair limit would have been 600.6); extra reviewer hours per extra true closure 0.022 (36.466 minus 35.418 hours over 48 extra closures).

**Development stress (same seeds; 4% ordinary and 25% day-15 ungraded-prep synthetic reviewer error, assumed stress values, not measured clinician rates):**

| Seed | Baseline TP / truth | Baseline recall | `pairfeat5` TP / truth | `pairfeat5` recall | Gain |
|---|---:|---:|---:|---:|---:|
| 521 | 179/234 | 0.7650 | 194/234 | 0.8291 | +6.41pp |
| 541 | 190/248 | 0.7661 | 206/248 | 0.8306 | +6.45pp |
| 547 | 178/237 | 0.7511 | 192/237 | 0.8101 | +5.91pp |
| Pooled | 547/719 | 0.7608 | 592/719 | 0.8234 | **+6.2587pp** |

The mean paired per-seed gain was +6.2563pp (95% t interval +5.5033 to +7.0094pp, n=3). **Stress gate: passed** (minimum gain 0). Contamination matched development clean (false-merge pairs 52 to 19, impure clusters 37 to 16, cross-person edges 42 to 17); uncapped wrong-pair reviews 3 to 1 (limit 3.3); prevented false merges 33; capped wrong-pair reviews 0 (limit 66); distinct link-review pairs 545 to 549. The injected errors produced 79 wrong final sign-offs in the baseline arm and 82 in the `pairfeat5` arm; they cost recall and did not become wrong-patient releases.

**Validation clean (seeds 401, 409, 419, 421, 431; same settings; configuration frozen in the validation manifest before the run-2 validation lanes; see the run history for the run-1 first pass):**

| Seed | Baseline TP / truth | Baseline recall | `pairfeat5` TP / truth | `pairfeat5` recall | Gain |
|---|---:|---:|---:|---:|---:|
| 401 | 188/218 | 0.8624 | 203/218 | 0.9312 | +6.88pp |
| 409 | 190/221 | 0.8597 | 205/221 | 0.9276 | +6.79pp |
| 419 | 193/215 | 0.8977 | 204/215 | 0.9488 | +5.12pp |
| 421 | 183/215 | 0.8512 | 199/215 | 0.9256 | +7.44pp |
| 431 | 202/232 | 0.8707 | 213/232 | 0.9181 | +4.74pp |
| Pooled | 956/1,101 | 0.8683 | 1,024/1,101 | 0.9301 | **+6.1762pp** |

The mean paired per-seed gain was +6.1935pp with a 95% t interval of +4.7177 to +7.6693pp (n=5; small-n and descriptive only). Recall rose on every seed.

**Validation gate verdict: passed** (`wrong_pair_cap`, minimum gain +1.0pp):
- Recall gain +6.1762pp.
- False-merge pairs fell, 173 to 87 (per seed 30 to 11, 40 to 21, 41 to 19, 30 to 15, 32 to 21).
- Impure clusters fell, 131 to 75 (per seed 25 to 10, 30 to 19, 26 to 13, 25 to 14, 25 to 19).
- Cross-person edges fell, 144 to 79 (per seed 27 to 10, 34 to 21, 29 to 15, 26 to 14, 28 to 19). Contamination fell in every seed but is not zero.
- Uncapped wrong-pair reviews 6 to 4 against a limit of 6.6 (per seed 2 to 1, 3 to 2, 1 to 1, 0 to 0, 0 to 0).
- Prevented false merges 86; capped wrong-pair reviews 0 against a limit of 172.
- Reported, not gating: capped same-person pairs 564; distinct link-review pairs 892 to 892 (ratio 1.000; old total-pair limit 981.2); extra reviewer hours per extra true closure 0.0255 (61.717 minus 59.983 hours over 68 extra closures).

**Safety and workload:** false closures, wrong-patient releases, replay mismatches, analysis errors and trace-write errors were zero in both arms in every run-2 mode, with zero unanswered review items and zero wrong final sign-offs outside the injected stress errors. `Link pairs` are distinct link-review pairs; `Items` and `Hours` cover all reviewer work, pooled across the arm's seeds.

| Mode | Arm | Link pairs | Wrong-pair reviews (all uncapped) | Items | Hours |
|---|---|---:|---:|---:|---:|
| Development clean | baseline / `pairfeat5` | 546 / 549 | 3 / 1 | 1,088 / 1,117 | 35.418 / 36.466 |
| Development stress | baseline / `pairfeat5` | 545 / 549 | 3 / 1 | 1,084 / 1,113 | 35.460 / 36.484 |
| Validation clean | baseline / `pairfeat5` | 892 / 892 | 6 / 4 | 1,805 / 1,846 | 59.983 / 61.717 |

The zero-safety result is simulated evidence from these cohorts and is not a validated safety property.

| Residual truth-closed misses | Development clean baseline | Development clean `pairfeat5` | Validation baseline | Validation `pairfeat5` |
|---|---:|---:|---:|---:|
| Total open | 114 | 66 | 145 | 77 |
| Attached, not closed | 19 | 20 | 23 | 26 |
| Below review | 64 | 14 | 80 | 6 |
| Capped, not queued | 0 | 0 | 0 | 2 |
| No candidate generated | 3 | 3 | 3 | 3 |
| Demographic rejection | 0 | 0 | 1 | 0 |
| Routed to review | 0 | 0 | 0 | 0 |
| Linked report not attached | 28 | 29 | 38 | 40 |

The scorer-addressable category (below review) fell from 64 to 14 in development and from 80 to 6 in validation, which accounts for the recall gain (validation net open misses fell by 68, matching the 68 extra exact closures). Two validation loops (seeds 409 and 431) stayed open because a capped true pair was left neither merged nor queued; development had none. The categories a scorer change cannot reach did not improve ("linked report not attached" 38 to 40 and "attached, not closed" 23 to 26 in validation).

**Capped-pair report (not gating):** distinct pairs with an `xscript_cap` record in the `pairfeat5` arm, split by offline truth; the baseline arms had none.

| Lane | Capped pairs | Same-person | Different-person | Capped, not queued (loops) |
|---|---:|---:|---:|---:|
| Development 521 | 121 | 115 | 6 | 0 |
| Development 541 | 121 | 115 | 6 | 0 |
| Development 547 | 132 | 120 | 12 | 0 |
| Development total | 374 | 350 | 24 | 0 |
| Validation 401 | 132 | 118 | 14 | 0 |
| Validation 409 | 135 | 117 | 18 | 1 |
| Validation 419 | 121 | 104 | 17 | 0 |
| Validation 421 | 117 | 101 | 16 | 0 |
| Validation 431 | 140 | 124 | 16 | 1 |
| Validation total | 645 | 564 | 81 | 2 |

Stress lanes had the same capped counts as development clean (374, 350, 24). Most capped pairs are same-person: 350 of 374 in development and 564 of 645 in validation.

**Cost not captured by CRC recall: split identities (read-only diagnosis; development on the run-1 lanes, whose capped counts and recall equal run-2's; validation on run-2):** the link-review queue asks only the link questions a loop depends on, so a capped pair that no open CRC loop needs is neither merged nor asked. In the development diagnosis the 374 capped pairs ended as: 95 queued (all same-person, all linked by the reviewer), 49 same-person linked through another path without being queued, 206 same-person and 24 different-person left in the review band without being queued. No capped pair reached the review queue without being linked. At the end of the lanes, capped pairs left unmerged and unreviewed in separate clusters were:

| Capped pairs left apart at the end | Same-person | Of these, in one cluster in the paired baseline lane | Different-person | Of these, in one cluster in the paired baseline lane |
|---|---:|---:|---:|---:|
| Development (seeds 521, 541, 547) | 183 | 128 | 24 | 15 |
| Validation (seeds 401-431) | 269 | 187 | 80 | 43 |

So on validation the cap left 187 same-person pairs apart that the baseline arm had joined, against 43 different-person pairs it kept apart that the baseline had joined (development 128 against 15). These are registrations of one person held as separate identities away from CRC loops; CRC recall does not price them, and the clean lanes have no simulated reviewer error, so a queued same-person pair would have been linked. Two cautions apply. The comparator is the baseline arm (the default model, with no cap), not `pairfeat5` without the cap; no cap-off `pairfeat5` arm was run, so how much of the false-merge reduction comes from the cap and how much from regularisation is not isolated. And the prevented false-merge count (33 in development, 86 in validation) counts different-person registration pairs inside clusters, so it is not expected to equal the different-person capped pairs (15 and 43) the baseline joined. One of the 81 different-person capped validation pairs ended in one cluster anyway, by a path not traced further. Whether capped pairs should always be queued for review, and what that would cost, is an open product question: in the development diagnosis, queueing every capped pair would have added up to 279 link-review pairs to run-1's 550 (not run).

**Interpretation:** in these simulated CRC cohorts the regularised refit with the cross-script cap kept the recall gain the earlier pair-scorer experiments found (+6.18pp pooled on validation) and, unlike `pairfeat2`-`pairfeat4`, reduced every contamination count in every seed, with no rise in distinct link-review pairs on validation (892 to 892). The cost moved out of the CRC metrics into unmerged same-person registrations, as reported above. The development seeds differ from the earlier experiments' and validation is a disclosed second pass, so no claim is made that it would generalize beyond these synthetic cohorts. This makes `pairfeat5` a candidate that passed its pre-registered gates, not a validated default.

**Implementation notes (from parked review findings; none changed a result):**
- `L.pscore` keeps the raw, uncapped model score for a capped pair (it is written before the cap), so the cluster-average merge guard (`GuardThr` 0.25) sees the raw value. Both 0.69 and any raw score above it sit far above 0.25, so this can change a guard decision only when a cluster's average pair score is near 0.25. A capped pair can still end in one cluster through a third record; the cap never claimed to stop that.
- The transitive `linked` trace record also carries the raw score for a capped pair, so trace readers must use the `near` or `below_review` record with `reason: "xscript_cap"` for the capped value.
- The linker stat `xscript_capped` counts cap firings, not distinct pairs; it equalled the distinct capped-pair count in every lane checked (development and validation).
- The cap relies on the merge threshold being above 0.69 (default 0.7, used in every lane here); a threshold at or below 0.69 would let capped pairs merge.

**Result-affecting rulings:**
- Amendment 1 (cap-aware gate, ratio 2 per prevented false merge) was user-approved and committed at `09524fe`/`3af6821` before any `pairfeat5` development lane; the fitted model and every other gate were unchanged.
- The classifier category `candidate_capped_not_queued` was added after run-1 stopped in validation, with user approval; it is diagnostic and no gate reads it, and the whole chain was re-run at the new HEAD rather than resuming validation.
- run-0-preamend is information only; run-2 is the run of record; the run-1 development and stress figures are kept as the first record and their differences are reported above.
- The leaf stop check reads `max_abs_leaf` from the fit report (missing or non-finite stops the run, exit 4) instead of re-walking the trees in the driver, so the check is not independent of `fit_scorer`'s tree walk.
- An unmapped registration in the capped-pair or wrong-pair counts aborts the analysis before any write-once file; this did not occur.

**Process rulings and deviations:**
- Go task A (linker cap) and Python tasks B (fit) and C (harness) ran in parallel on disjoint files; the cost was only commit interleaving. C2 (cap-aware gate) and C3 (classifier category) followed.
- Task A could not show trace byte-identity for other modelsets on the traced `orchestrate` lane, because that lane is not reproducible run to run even with the base binary; identity was shown on `featdump`, generation and simulation output (byte-identical) and an identical trace schema. The ruling accepted this because the harness compares paired lanes, not trace bytes.
- Task B's fix round (`7390a99`) was not run strictly test-first.
- Task C2 was not written in strict test-first order (only the pre-existing `pairfeat5` tests were red first); its tests were added and pass.
- Deferred minor review findings (none affect results): a test subclass re-runs inherited tests; truth is loaded only through the wrong-pair path; no end-to-end exit-4 test in the driver's main flow; a stale `encdev` doc comment; no missing-DOB feature-13 assertion in the linker test; a test fixture uses stage "model"; the gate trusts the loader for the baseline's zero capped count; the baseline scan checks pair records only; the zero-baseline reason text; a weak abort-message assertion and a no-op slice in a test; `aggregate_arm` silently drops the new category on mixed lanes (unreachable through the lane loader).
- Strict same-HEAD binding was kept, so run-2 stayed at `56d31fd` from fit through validation.

**Scope:** the model and cap apply globally but these cohorts are CRC-only. No non-CRC safety or recall claim is made, and simulated results establish no clinical effectiveness. The default model, default noise and default floor 0.2 are unchanged and `pairfeat5` was not promoted. No further seeds, thresholds, features, hyperparameters or noise variants are authorized for this arm.

### CRC "completion not documented" clinician review with an image-verified caecal photo (`r1review`) at `694a417` (selected under the amended gate; not promoted to default)

**Outcome:** with the opt-in flag `-r1review`, a colonoscopy report that the two readers would otherwise mark `inadequate` only because they found no caecum statement goes to a clinician instead, but only when the hold-only image reader has verified a caecal photo on that report. Run on top of the `pairfeat5` linker, it passed development, stress and five-seed validation against the `pairfeat5` arm, under a gate that was changed after the first development run (Amendment 3 below). In paired simulated CRC validation cohorts pooled recall rose from 0.9406 (`pairfeat5`) to 0.9569 (+1.64pp pooled; mean paired per-seed gain +1.64pp, 95% t interval +1.17 to +2.11pp, n=5), with zero false closures in every arm and contamination counts identical to `pairfeat5`'s. Against the default baseline (0.8777) the gain was +7.92pp, but the vs-baseline check, reported and not gating under Amendment 3, **failed on validation**: wrong-pair reviews rose from 4 to 7 against a limit of 4.4. That rise comes from the `pairfeat5` linker, not from `-r1review` (the `pairfeat5` arm also had 7), so on these image-on seeds `pairfeat5` would fail its own wrong-pair check. Under the gates as pre-registered before Amendment 3, `r1review` would have stopped at run-1 development and would also have failed validation. Four assumptions carry the result: the photos are simulated at assumed rates, the image reader was trained on non-local images, the simulated clinician answers from the true label, and its error rate is assumed. The result therefore means "if units photograph caecal completion and a clinician can resolve it from the photo", not a measured effect. The default model, the default reader policy and the default review floor of 0.2 are unchanged; nothing was promoted to default. All results are simulated; they prove no clinical effectiveness.

**Origin (post-hoc, disclosed):** the change came from a read-only diagnosis of the already-used `pairfeat5` development lanes (seeds 521, 541, 547), not from a prior plan. Of 66 missed true CRC closures, 17 were on the pair scorer's side and 49 happened after the link succeeded; the baseline arm showed the same post-link groups, so none was a scorer effect. In one group of 20 loops, both readers found no caecum statement (the common quote was "to the ileocecal valve"), rule `R1_completion_not_documented` labelled the report `inadequate` with both readers agreeing, no review item was raised, and the loop ended `inadequate` with a repeat requested, while the truth says the colonoscopy was complete: a wrong verdict as well as a missed closure. The other 29 post-link misses were mostly loops waiting on an ungraded bowel-prep grade when the run ended (25 of 29 within 15 days of the end), which is right-censoring at the run horizon rather than a defect. Reader v7 deliberately stopped counting the ileocaecal valve alone as caecal intubation after the corpus 6 false closures; this change keeps that rule: the valve alone never closes a loop automatically.

**Built:** an opt-in flag with three parts.
- Conversion (code at `573dd3e`, `e8c52d2`): with `-r1review` on, a report whose flag-off verdict is `inadequate`, agreed by both readers, with `R1_completion_not_documented` as its only rule becomes `needs_human` (rule `R1_completion_review`). A stated failure to reach the caecum, including the corpus 6 patterns, stays automatically `inadequate`, and no new pattern ever produces `adequate`.
- Photo gate (Amendment 2, code at `b05a302`, `c8c9d14`): the converted report goes to a clinician only when the image reader returns `caecal_photo = "verified"`, a photo filed at the caecum station that the landmark CNN scores at or above its threshold. Every other converted report reverts to the flag-off verdict, an automatic `inadequate` with `R1_completion_not_documented`. With the image reader off, every conversion reverts, so `-r1review` changes nothing without `-image cached` (or `live`) and photos. `readerscore -r1review` runs without images, so all its conversions revert.
- Review item (code at `573dd3e`, `d0a7f2f`): one evidence item with purpose `completion_undocumented`, showing the verified photo. Until it is answered, the loop is held as `completion_review` and no `rejected_inadequate` escalation is sent. A signed "adequate" closes through the existing override path, subject to the report's tissue and histology fields (the histology wait is keyed on the signed item's purpose); "inadequate" holds as before. The image reader still never closes a loop.
- Synthetic reviewer: `completion_undocumented` items use `ErrUngraded`, the assumed error of a decision made without documentation (0 in clean lanes, 0.25 in stress lanes, assumed values, not measured clinician rates). The simulated reviewer answers from the report's true label, which assumes a clinician can resolve completion from the photo or the endoscopist.
- With `-r1review` off, every output is byte-identical to before (pinned by tests).

**Arms, seeds and photos:** three paired arms on the same cohorts: `baseline` (default model), `pairfeat5` (reference: `pairfeat5` chain, cap on) and `r1review` (`pairfeat5` chain, cap on, plus `-r1review`); all at `-linkfloor 0.01`, `v7strict`, CRC-only v2, cached readers, chaos 0. The scorer was not changed: the `pairfeat5` chain was refit at each run HEAD and had to equal `pairfeat5` run-2's, which it did both times (`lgb_stream_pairfeat5` `41cbb8f3`, `iso_pairfeat5` `782ef505`, `iso_v7_pairfeat5` `e454b022`, encoder `5e6284f8`; model files byte-identical; only the HEAD and the training-manifest hash differ). All three arms ran with synthetic photo attachment at generation (`-imgphotos`) and the image reader at `-image cached` (the simulation step itself runs image-off in every arm; see the rulings below), so the comparison inside this experiment is fair, but **none of these numbers is comparable with image-off runs**, including the `pairfeat5` section above (which also used other seeds). Photo attachment is simulated at assumed rates tied to the text answer key: 85% of reports that document the caecum get a true caecum photo, and 10% of the others get a mis-filed non-caecal photo. The landmark CNN was trained on HyperKvasir (Norwegian video frames) and was not recalibrated on local images. Seeds: run-1 development 557, 563 and 569 (used once, then retired); run-2 development 571, 577 and 587 and validation 631, 641, 643, 647 and 653, all never used before. The originally planned validation seeds 601, 607, 613, 617 and 619 were retired unused.

**Gates (pre-registered; stop at the first failure):**
- Gate 1, `r1review` against baseline: exactly the `pairfeat5` gates (`wrong_pair_cap`), recall gain at least +2.0pp for selection, 0 for stress and +1.0pp for validation. Under Amendment 3 this is reported, not gating.
- Gate 2, `r1review` against `pairfeat5`: recall gain at least +1.0pp for selection, 0 for stress and +0.5pp for validation; the safety set (false closures, wrong releases, replay mismatches, analysis errors, trace errors) zero in every lane of every arm, stress included; false-merge pairs, impure clusters, cross-person edges and wrong-pair link-review pairs not above the `pairfeat5` arm's; total reviewer hours at most 1.10 times the `pairfeat5` arm's.
- Reported, not gating: completion items and their answers, the corpus re-score, recall excluding loops whose true closure falls within 15 days of the lane's last decision, and wrong final sign-offs.

**Amendment 1 (`7ba9275`, before any lane):** gate 2 first listed "wrong final sign-offs 0", which cannot hold in stress, where the 4% ordinary synthetic reviewer error makes about 80 per arm by design. It was replaced by the harness's existing safety set; wrong final sign-offs are reported.

**Amendment 2 (`3ef7464`, user-approved, before any lane): the text-only design was estimated to be unsafe.** A read-only pre-run check of the lanes' 43-report corpus found that the text-only conversion moved 7 reports: 6 truly inadequate and 1 complete (the valve report). In the already-used `pairfeat5` run-2 stress lanes, 96 loops carried one of the truly inadequate converted reports and 26 the complete one. At the assumed 0.25 error, about 24 wrong "adequate" sign-offs were expected, so the stress gate of zero false closures would have failed by construction (about 4 at 0.04). On the held-out corpora about 80% of converted reports are truly complete, so the balance depends on the report mix. The run was stopped before any `r1review` lane and the user chose the photo gate above. The clinician item now shows a verified photo, the reviewer error stays `ErrUngraded`, and the image reader still never closes a loop.

**Amendment 3 (`b349191`, harness `d45ec44`, `694a417`; user-approved, post-hoc, after the first development run):** run-1 development clean failed gate 1 on one count only (details below), a property of the `pairfeat5` linker that `-r1review` cannot change. Gate 1 became reported, not gating; gate 2 gates, unchanged; the baseline arm's safety set is still enforced; and development and validation moved to fresh seeds with a new experiment directory and a refit at the new HEAD. Nothing about the mechanism or its thresholds changed. This gate change was made after seeing development results, which are reported below alongside the rerun.

**Run history, in order (all artifacts preserved):**
- **Pre-run safety check (read-only):** stopped the text-only design before any lane (Amendment 2).
- **Smoke lane (seed 101, 400 persons, exact harness command, HEAD `4377e83`):** the first attempt hung in `orchestrate` because the smoke test's own binary folder lacked the `nats-server` link that the harness's folder has; this was a test-setup gap, not a code defect. The rerun was clean: all seven steps exited 0, the safety counts were 0, recall 0.922, and all 4 completion items had a verified photo and were answered "awaiting histology" (the truth label), 0 wrong.
- **Refit path refused once:** the fit script refused the planned output path (fit outputs must sit under the modelset's own directory), so both refits were written under the `pairfeat5` directory; this changes no result.
- **run-1 (HEAD `4377e83`, development seeds 557, 563, 569): stopped at gate 1.** Development clean passed gate 2 but failed gate 1: uncapped wrong-pair link-review pairs 6 against a limit of 5.5 (baseline 5 plus 10%). The `pairfeat5` arm also had 6 and the `r1review` arm's link output was identical to `pairfeat5`'s. The selection file was written with nothing selected and the run stopped as pre-registered.
- **Wrong-pair diagnosis (read-only):** all 11 wrong-pair reviews (5 baseline, 6 `pairfeat5`) were cross-script lookalikes scored 0.01 to 0.09, at or a few times above the 0.01 review floor: an Arabic-script referral name against a Latin-script endoscopy name, a shared given name, the same sex, and a birth year equal or one apart, often a 1 January placeholder, with the true person registered elsewhere. `pairfeat5` moved 2 of baseline's below the floor, moved 2 others above it, and asked 1 further pair that was already above the floor in baseline but not asked there. All 11 were answered "different" correctly and none caused a merge or a closure. With a baseline count of 5, a 10% limit fails on any net +1.
- **run-2 (HEAD `694a417`, run of record, user-approved):** development, stress and validation all passed gate 2; no rerun or crash.

| run-1 development clean (seeds 557, 563, 569) | Baseline | `pairfeat5` | `r1review` |
|---|---:|---:|---:|
| TP / truth | 566/678 | 610/678 | 632/678 |
| Recall | 0.8348 | 0.8997 | 0.9322 |
| False closures | 0 | 0 | 0 |
| False-merge pairs | 44 | 28 | 28 |
| Cross-person edges | 38 | 24 | 24 |
| Impure clusters | 34 | 23 | 23 |
| Wrong-pair reviews (all uncapped) | 5 | 6 | 6 |
| Reviewer hours | 33.773 | 34.621 | 35.419 |
| Completion items (wrong) | 0 | 0 | 23 (0) |

On run-1, `r1review` gained +3.2448pp pooled over `pairfeat5` (mean paired per-seed +3.25pp, 95% t interval -1.23 to +7.73pp, n=3) and +9.7345pp over baseline; hours were 1.023 times `pairfeat5`'s. All 23 completion items had a verified photo and were answered "awaiting histology", matching the truth.

**Artifact identity (run-2):** governing manifest `5138f172`, binary `fac06ad0` (all arms), image-verdict cache `5f90208c`, selection decision `a6b094e2`, stress decision `4fbc99d1`, validation manifest `59ee3085` (frozen before any validation lane), training manifest `86a1785d`; refit parity max |delta| 1.1e-16.

**Paired development clean (run-2, seeds 571, 577, 587):**

| Seed | Baseline TP / truth | `pairfeat5` TP / truth | `r1review` TP / truth | `r1review` recall | Gain vs `pairfeat5` |
|---|---:|---:|---:|---:|---:|
| 571 | 194/230 | 205/230 | 212/230 | 0.9217 | +3.04pp |
| 577 | 181/204 | 187/204 | 193/204 | 0.9461 | +2.94pp |
| 587 | 197/241 | 217/241 | 224/241 | 0.9295 | +2.90pp |
| Pooled | 572/675 (0.8474) | 609/675 (0.9022) | 629/675 | 0.9319 | **+2.9630pp** |

The mean paired per-seed gain over `pairfeat5` was +2.9631pp (95% t interval +2.7842 to +3.1419pp, n=3, descriptive only). **Selection gate 2: passed.** Safety counts were 0 in every lane of every arm; false-merge pairs (18), impure clusters (15), cross-person edges (15) and wrong-pair link-review pairs (3: 1 uncapped, 2 capped) were identical to `pairfeat5`'s; hours 34.972 to 35.821 (1.024 times, limit 1.10). Gate 1 (reported): +8.4444pp over baseline (mean paired +8.30pp, 95% t interval +1.61 to +14.99pp), uncapped wrong-pair reviews 2 to 1 (limit 2.2), false-merge pairs 36 to 18; it would have passed. All 20 extra closures over `pairfeat5` came through a completion item on the closing report, and no `pairfeat5` closure was lost.

**Development stress (same seeds; 4% ordinary and 25% `ErrUngraded` synthetic reviewer error, assumed values):** pooled recall baseline 528/675 (0.7822), `pairfeat5` 562/675 (0.8326), `r1review` 578/675 (0.8563); +2.3704pp over `pairfeat5` (per seed +2.17, +2.45, +2.49pp; mean +2.37pp, 95% t interval +1.94 to +2.80pp, n=3) and +7.4074pp over baseline. **Stress gate 2: passed** (minimum gain 0). Safety counts were 0 in every arm; contamination equalled `pairfeat5`'s; wrong-pair link-review pairs 4 (`pairfeat5`) and 3 (`r1review`); hours 35.889 to 36.826 (1.026 times). Wrong final sign-offs were 77, 79 and 85 (reported). Gate 1 (reported) would have passed (uncapped wrong-pair reviews 2 to 1).

**Validation clean (run-2, seeds 631, 641, 643, 647, 653; configuration frozen in the validation manifest before any validation lane):**

| Seed | Baseline TP / truth | `pairfeat5` TP / truth | `r1review` TP / truth | `r1review` recall | Gain vs `pairfeat5` | Gain vs baseline |
|---|---:|---:|---:|---:|---:|---:|
| 631 | 197/231 | 217/231 | 220/231 | 0.9524 | +1.30pp | +9.96pp |
| 641 | 210/237 | 223/237 | 228/237 | 0.9620 | +2.11pp | +7.59pp |
| 643 | 211/242 | 228/242 | 231/242 | 0.9545 | +1.24pp | +8.26pp |
| 647 | 185/209 | 199/209 | 203/209 | 0.9713 | +1.91pp | +8.61pp |
| 653 | 216/242 | 225/242 | 229/242 | 0.9463 | +1.65pp | +5.37pp |
| Pooled | 1,019/1,161 | 1,092/1,161 | 1,111/1,161 | 0.9569 | **+1.6365pp** | +7.9242pp |

| Validation, pooled | Baseline | `pairfeat5` | `r1review` |
|---|---:|---:|---:|
| Recall | 0.8777 | 0.9406 | 0.9569 |
| TP / truth | 1,019/1,161 | 1,092/1,161 | 1,111/1,161 |
| False closures | 0 | 0 | 0 |
| False-merge pairs | 177 | 79 | 79 |
| Cross-person edges | 154 | 73 | 73 |
| Impure clusters | 140 | 68 | 68 |
| Wrong-pair reviews (all uncapped) | 4 | 7 | 7 |
| Reviewer hours | 59.670 | 61.719 | 62.311 |
| Completion items | 0 | 0 | 24 |

Paired per-seed gain against `pairfeat5`: mean +1.6430pp, 95% t interval +1.1733 to +2.1126pp; against baseline: mean +7.9601pp, 95% t interval +5.8701 to +10.0501pp (n=5; small-n and descriptive only). Recall rose on every seed against both arms.

**Validation gate 2 verdict: passed** (minimum gain +0.5pp): recall gain +1.6365pp; safety counts 0 in every lane of every arm; false-merge pairs, impure clusters, cross-person edges and wrong-pair link-review pairs identical to `pairfeat5`'s; hours 1.010 times (limit 1.10).

**Validation gate 1 (reported, not gating): failed.** Contamination fell against baseline (false-merge pairs 177 to 79, prevented false merges 98; impure clusters 140 to 68; cross-person edges 154 to 73), capped wrong-pair reviews were 0 (limit 196), and distinct link-review pairs rose 927 to 943 (ratio 1.017). But uncapped wrong-pair reviews rose from 4 to 7 against a limit of 4.4. The `pairfeat5` arm has the same 7, so this is the `pairfeat5` linker: on these image-on seeds `pairfeat5` would fail its own wrong-pair check under its pre-registered validation gate, although it passed on its own validation seeds (6 to 4 on seeds 401-431). The extra wrong-pair reviews on these validation seeds were not traced pair by pair; the run-1 diagnosis above is the only one.

**Completion items** (raised only when the image reader verified a caecal photo; "awaiting histology" means the reviewer judged the exam complete and the loop waits on histology):

| Mode | Items | Answered "awaiting histology" | "adequate" | "inadequate" | Wrong | Closing-unsafe wrong | Safe-direction wrong |
|---|---:|---:|---:|---:|---:|---:|---:|
| Development clean | 24 | 23 | 0 | 1 | 0 | 0 | 0 |
| Development stress | 23 | 19 | 1 | 3 | 4 | 1 | 3 |
| Validation clean | 24 | 23 | 0 | 1 | 0 | 0 | 0 |

In development, every item had a verified photo; of the 24 clean items, 20 gained a closure, 3 were on loops whose truth was still waiting on histology at the end of the run (staying open is correct), and 1 was a truly inadequate report that nevertheless carried a verified photo (answered "inadequate", correctly). Validation items were not traced one by one. Clean lanes have zero reviewer error and the answer comes from the true label, so a wrong answer there is impossible by construction. In stress, 3 of the 4 wrong answers were safe-direction (complete exams answered "inadequate", costing recall), and **1 was closing-unsafe**: the same truly inadequate report with a verified photo was answered "adequate". It did not cause a false closure only because another report on the same loop closed it correctly; with no such report it could have become a false closure (a signed "adequate" closes subject to the report's tissue and histology fields). The zero false closures in stress therefore rest on the assumed reviewer error rate and on this cohort's report mix, not on the mechanism.

| Residual truth-closed misses | Dev clean baseline | Dev clean `pairfeat5` | Dev clean `r1review` | Validation baseline | Validation `pairfeat5` | Validation `r1review` |
|---|---:|---:|---:|---:|---:|---:|
| Total open | 103 | 66 | 46 | 142 | 69 | 50 |
| Attached, not closed | 19 | 22 | 3 | 16 | 20 | 3 |
| Below review | 46 | 6 | 6 | 84 | 8 | 8 |
| Capped, not queued | 0 | 0 | 0 | 0 | 0 | 0 |
| No candidate generated | 3 | 3 | 3 | 6 | 6 | 6 |
| Demographic rejection | 0 | 0 | 0 | 1 | 0 | 0 |
| Routed to review | 0 | 0 | 0 | 0 | 0 | 0 |
| Linked report not attached | 35 | 35 | 34 | 35 | 35 | 33 |

The category this change addresses, "attached, not closed", fell from 22 to 3 in development and from 20 to 3 in validation; scorer-side categories were unchanged against `pairfeat5`.

**Remaining `r1review` misses by cause (development clean, read-only diagnosis, 46 loops):**
- 34 linked, with the closing report not attached while the loop waits on an ungraded bowel-prep query (19 still awaiting the unit's grade, 15 past the day-15 escalation); 29 of the 34 had a true closure within 15 days of the run end. The cause is inferred from the final prep state and funnel stage; the outputs do not say directly that the prep stage held the report.
- 6 below the review floor.
- 3 never retrieved by any candidate path.
- 3 converted reports without a verified photo (2 unverified, 1 no photo), reverted to automatic `inadequate` by design although all 3 exams were truly complete.

**Horizon-excluded recall (development clean only):** excluding the 95 of 675 truth closures that fall after day 105 of the 120-day run (the same loops in all arms), recall was baseline 511/580 (0.8810), `pairfeat5` 546/580 (0.9414) and `r1review` 565/580 (0.9741). Stress and validation were not computed this way.

**Reviewer workload:** against `pairfeat5`, total reviewer hours rose by 2.4% (development clean), 2.6% (stress) and 1.0% (validation), about 1.9 to 3.5 minutes of simulated reviewer time per extra closure; review items rose by the completion items in development and validation (validation 1,930 to 1,954) and by 27 for 23 items in stress. Distinct link-review pairs moved by 2 or fewer (validation 945 to 943); the cause was not isolated. Hours come from the simulated reviewer-time model, not measured clinician time.

**Corpus check (reported, not gating):** the report corpora carry no photos, so the photo path cannot be tested on them. At HEAD, `readerscore -r1review` output was byte-identical to flag off on all 17 corpus and cache runs (MTSamples with 26B, and corpora 1 to 8 with 26B and 4B), with 0 unsafe labels. For information, a binary built at `e8c52d2` (text-only conversion, **before Amendment 2**) also gave 0 unsafe and 0 automatic unsafe labels everywhere, because `needs_human` never closes; it moved these reports to review:

| Text-only conversion, pre-Amendment 2 | Moved to review | Truly complete | Truly inadequate |
|---|---:|---:|---:|
| Corpora 1-8, 26B cache | 278 | 220 | 58 |
| Corpora 1-8, 4B cache | 263 | 223 | 40 |
| MTSamples (43 reports), 26B | 7 | 1 | 6 |

**Practical meaning:** in these simulated cohorts, sending "completion not documented" reports to a clinician when a caecal photo is verified recovered 20 (development, of 675 true closures) and 19 (validation, of 1,161) closures over `pairfeat5`, at about 1 to 2.6% more reviewer hours, without any false closure. A DoH pilot would need three things this experiment assumes rather than shows: units filing a caecal photo for each colonoscopy as the CRC specification requires (Appendix 11, section 2.2), with the real documentation rate measured; the landmark CNN recalibrated and validated on local images; and clinician time for these items, with their real error rate on them measured, since one wrong "adequate" in stress came close to a false closure. The text-only form must not be used: on this cohort's report mix it routes enough truly inadequate reports to a clinician that, at the assumed 25% stress error, about 24 wrong "adequate" sign-offs were expected (Amendment 2).

**Result-affecting rulings:**
- Amendments 1 and 2 were made before any `r1review` lane; Amendment 2 was user-approved and changed the mechanism.
- Amendment 3 was user-approved and post-hoc: it changed which gate decides after a development failure. Run-1 figures are reported above.
- All three arms run image-on (`-imgphotos`, `-image cached`); the simulation step itself runs image-off in every arm, which keeps the comparison fair.
- Conversion was narrowed (`e8c52d2`) to verdicts that `R1_completion_not_documented` alone made `inadequate`; partial-R1 reports keep the flag-off verdict.
- The corpus check was set to (a) HEAD identity and (b) text-only counts from a separate binary, labelled pre-Amendment 2.
- The three-arm analysis refuses if a baseline or `pairfeat5` lane has any completion item, checks each lane's image stamp, and still enforces the baseline arm's safety set under Amendment 3 (a review finding fixed at `694a417` before any run-2 lane).

**Process rulings and deviations:**
- The Go and Python tasks ran in parallel on disjoint files, as for `pairfeat5`.
- The traced `orchestrate` lane is not hash-reproducible run to run (as already found for `pairfeat5`); CRC figures matched.
- Deferred minor review findings (none affect results): a completion item reuses the ordinary evidence item id by design; "wrong" counts last-wins per item; no completion-pending escalation; the kind guard applies the gate only to colonoscopy reports (correct today); some test gaps for mixed holds.

**Scope:** synthetic data only, with synthetic photos, a synthetic reviewer and n=5 validation seeds. No non-CRC claim is made, and simulated results establish no clinical effectiveness. The default model, default reader policy and default floor 0.2 are unchanged and `-r1review` was not promoted.
