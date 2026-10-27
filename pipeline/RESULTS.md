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

## Image evidence reader (third reader, hold-only)

Details are in [`image/RESULTS.md`](../image/RESULTS.md). **HyperKvasir competition use is pending written permission from SimulaMet; weights are not distributed.**

**What it is.** Two MobileNetV3-small CNNs, trained on a near-duplicate-grouped split of HyperKvasir:
- **landmark:** caecum vs other lower-GI views, AUC 0.998;
- **prep:** BBPS 2–3 vs 0–1, AUC 0.991, specificity 0.83.

`-image cached|live` wires them in as a third reader. It can only hold or route a report to a human (`prep_grade_one_click`, `image_prep_inadequate`, `image_prep_conflict`) and raise the `missing_caecal_photo` audit flag. It never closes a loop; this is tested exhaustively in `image_test.go`. It is off by default, so all results above are unchanged.

**Integrated run** (seed 2026, simulated photo attachments):
- CRC false closures 0/571 and recall 0.961, the same as without the image reader.
- 0 labels upgraded; 2 held on photo/text prep conflict; 152 `missing_caecal_photo` flags.
- 321 one-click prep-grade items. Evidence reviewer-h per 1k loops fall 11.3 → 6.1. That gain is an assumption: 20 s per one-click grade.

**Limits.** Domain shift: Norwegian video frames vs UAE stills, so local recalibration is mandatory before any pilot.
