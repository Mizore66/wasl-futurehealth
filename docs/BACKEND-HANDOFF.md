# Wasl backend handoff

## Mission

Turn the existing experiments into one reproducible, demoable backend for the Future Health submission. The product must safely link fragmented screening records, decide whether downstream evidence closes a care loop, protect patient communications, and make every decision auditable.

**Submission deadline:** 21 Nov 2026.

## Read first

1. [`BRIEF.md`](BRIEF.md) — product, evidence, constraints, and submission plan.
2. [`../pipeline/RESULTS.md`](../pipeline/RESULTS.md) — what the canonical integrated run actually proves.
3. [`../pipeline/v7-gate/RESULTS.md`](../pipeline/v7-gate/RESULTS.md) — stricter gate results from a separate implementation.
4. [`../reader/RESULTS-gemma4b.md`](../reader/RESULTS-gemma4b.md) — local-reader result.
5. [`../booking/RESULTS.md`](../booking/RESULTS.md) — booking/capacity simulation result.

## Current architecture

`pipeline/` is the only canonical runtime. It is Go + NATS JetStream:

```text
synthetic cohort
  → mock Malaffi / FHIR Subscription
  → NATS streams
  → prep workers ×3
  → cross-script identity linker
  → closure workers ×4
  → reviewer / RBAC
  → decision, audit, KPI and provenance outputs
```

Current implementation locations:

| Capability | Canonical location | Current state |
|---|---|---|
| NATS, FHIR mock ingest, dedupe, chaos tests | `pipeline/` | integrated |
| Arabic↔Latin encoder, HNSW, scorer, union-find linker | `pipeline/model.go`, `ann.go`, `linker.go` | integrated |
| Regex + cached Gemma evidence verdicts | `pipeline/evidence.go` | integrated, cache replay only |
| Closure state machine and CRC/cervical rules | `pipeline/closure.go`, `rules.go` | integrated |
| Reviewer, RBAC, consent suppression, tamper/audit | `pipeline/governance.go`, `tamper.go`, `audit.go` | integrated |
| Strict v7 gate, calibrated guard, DOB guard | `pipeline/v7-gate/*.go.v7` | separate; merge required |
| Local Gemma 3 4B hybrid reader | `reader/hybrid4b.py` | separate; connect required |
| Booking engine and capacity simulation | `booking/` | separate; simulated |

## Non-negotiable truth

The existing integrated result is strong but limited:

- CRC P=1.000, R=0.960, 0/570 false closures in one simulated seeded run.
- 0 duplicate/lost outputs across 15,769 outputs under recorded chaos tests.
- 17/17 tamper attacks detected and 24/24 forged events denied.
- The merged runtime uses cached Gemma-26B answers. It does **not** make live LLM calls.
- The strict v7 gate, local 4B reader, and booking engine are **not merged**.
- 16 outreach intents were created before a late consent record arrived. A consent hold is required.
- Public Malaffi material points to HL7 v2. Current FHIR ingest is a mock.

Do not merge results from separate modules into an “end-to-end” claim. The final numbers must come from one final integrated run.

---

## Scope and order

Work in this order. Finish and verify each stage before advancing.

### 0. Reproducible baseline

**Goal:** another developer can run the canonical pipeline without tribal knowledge.

Implement:

- Replace hard-coded local paths.
- Add a single top-level command, such as `./scripts/demo.sh` or `make demo`, to:
  1. verify or explain required assets;
  2. build Go code;
  3. start NATS and local dependencies;
  4. generate the deterministic cohort;
  5. execute the run;
  6. run audit/tamper/report;
  7. write outputs to a documented location.
- Document external/local-only assets:
  - `persons3.json`;
  - encoder/scorer weights;
  - NATS server binary;
  - Gemma 3 4B GGUF;
  - Python dependencies for reader and booking.
- Keep secrets out of the repository.

**Done when:** a clean clone produces the current baseline result or fails early with a precise missing-asset instruction.

### 1. Integrate the strict v7 safety gate

**Source:** `pipeline/v7-gate/`.

Merge the v7 policy into `pipeline/`; do not retain a second runnable pipeline as the product path.

Required behaviour:

- Link confidence uses calibrated strata and conservative treatment of uncorroborated matches.
- EID-chain/model-only communications require exact DOB match or human confirmation.
- Evidence uses per-field two-reader consensus.
- CRC closure requires documented completion; choose and document one policy for ungraded prep:
  - **strict policy:** hold as quality-unverified; or
  - **R2 policy:** allow closure only when both readers provide an explicit caecal-landmark quote, while flagging missing prep grade.
- Merge and high-consequence evidence decisions receive double review; disagreement goes to a senior/third resolution path.

**Done when:**

- the canonical `pipeline/` run uses the v7 policy;
- unit tests cover the new guard and evidence policy;
- the run writes policy/version metadata to each decision;
- the old v6 and new v7 metrics are clearly separate in output.

### 2. Connect the local Gemma 3 4B reader

**Source:** `reader/hybrid4b.py`.

Keep Go as the orchestrator. Run the local reader as a local HTTP dependency, initially through the existing `llama-server` OpenAI-compatible endpoint.

Implement:

- A Go reader client with:
  - health check;
  - timeout;
  - bounded retries;
  - schema validation;
  - report SHA/id propagated in request and response;
  - clear “reader unavailable” result that routes to review.
- A stable JSON contract:

```json
{
  "report_sha": "string",
  "caecum_reached": "yes | no | not_stated",
  "caecum_quote": "string",
  "prep": "adequate | inadequate | not_stated",
  "prep_quote": "string",
  "polyps_removed": "yes | no",
  "polyps_quote": "string",
  "histology_present": "yes | no",
  "histology_quote": "string",
  "state": "string",
  "reason": "string",
  "model_version": "string",
  "latency_ms": 0
}
```

- Preserve regex/rule-reader output as the independent second reader.
- Persist both reader outputs, quotes, schema/version and agreement result for provenance.
- Make live local inference configurable. Cached answers may remain available only as an explicit demo/test mode.

**Done when:** a normal canonical run can use the local reader; reader failure never causes automatic closure; case output contains both reader verdicts and quotes.

### 3. Implement consent hold and outreach lifecycle

The existing system suppresses known opt-outs but can create outreach before late consent arrives. Fix that before any patient-facing demo claim.

Implement:

- A per-patient contact-permission state: `unknown`, `granted`, `denied`, `opted_out`.
- Pending-outreach storage keyed by loop and patient cluster.
- An invariant: dispatch requires `granted` at the time of dispatch.
- Late consent:
  - `granted` reevaluates pending eligible outreach;
  - `denied`/`opted_out` cancels queued outreach;
  - duplicated/reordered consent messages remain idempotent.
- Audit entries for hold, release, cancellation and attempted violation.

**Done when:** tests cover referral-before-consent, consent-before-referral, opt-out-after-queueing, duplicate consent and restart/replay; no dispatch appears before granted consent.

### 4. Add production-shaped HL7 v2 ingest

The FHIR Subscription consumer stays useful for the mock. Add an HL7 v2 MLLP adapter as the production-shaped integration tier.

Implement:

- MLLP listener or a deterministic mock MLLP source.
- Parse the minimum message shapes required for demo:
  - patient identity/demographics;
  - FIT-positive/referral result;
  - colonoscopy/procedure result;
  - pathology/histology result;
  - consent update if represented.
- Map to the existing internal event schema rather than creating a parallel closure path.
- Include source system, source message ID, timestamps and raw-message hash in provenance.
- Deduplicate/replay safely across connection restart, duplicate message and reordered delivery.

**Done when:** a representative HL7 sequence drives the same link → evidence → closure flow as FHIR; duplicates and restarts do not create duplicate state changes.

### 5. Serve materialised state for the frontend

The frontend should consume real run data first, then a thin Go API over the same materialised state.

Implement read endpoints:

| Endpoint | Returns |
|---|---|
| `GET /api/dashboard` | KPI, SLA, state counts, queue/review counts, capacity summary, simulation metadata |
| `GET /api/cases` | Filterable case summaries: programme, state, owner, deadline, consent, review status |
| `GET /api/cases/{id}` | Timeline, linked identities, evidence, reader outputs, decisions, provenance |
| `GET /api/cases/{id}/audit` | Hash chain / anchor verification and event history |
| `POST /api/cases/{id}/review` | Authenticated review/sign-off action in demo mode |
| `GET /healthz` | Dependency health: NATS, reader and ingestion adapter |

Rules:

- Use only synthetic/pseudonymous fixture data.
- Include `data_mode: simulated | demo` and run/policy/model version fields in API output.
- Enforce RBAC on mutating endpoints.
- Make review actions append audit events rather than overwrite history.

**Done when:** the frontend can render the demo cases (`docs/api-samples/fixtures.json`) exclusively from pipeline/API data, without invented client-side records.

### 6. Integrate booking as simulated decision support

**Source:** `booking/`.

Do this after safety, consent, ingest and API work.

Implement:

- A clear boundary: booking is simulated decision support, not live scheduling.
- Export bookable slots, constraints, proposed booking/rebooking and capacity shortfall in a stable JSON schema.
- Attach eligible open CRC cases to simulated booking suggestions.
- Show whether network pooling is relevant only when capacity is geographically misallocated.

**Do not claim:** the optimiser beats greedy. The current result shows a tie within noise.

**Done when:** API/dashboard can show a real pipeline case, its simulated booking suggestion, and the capacity condition that governs it.

### 7. Final end-to-end evaluation

This is a backend responsibility because the final scorecard must come from the actual integrated runtime.

Implement/run:

- Freeze code revision, thresholds, policy choice, model version, cohort generator, noise model and seed list before final evaluation.
- Use five independent seeds.
- Keep development/calibration and held-out evaluation cohorts separate.
- Double-label the 43 reports between the two team members; record agreement and disagreements. No clinician-label claim.
- Report:
  - precision, recall and false closures;
  - wrong-patient dispatches;
  - review items/hours;
  - message-before-consent count;
  - duplicates/losses;
  - tamper and forged-event detection;
  - subgroup review burden;
  - confidence intervals and explicit denominator.

**Done when:** `pipeline/results/final/` contains one manifest with inputs, versions, seeds, raw outputs, summary metrics and limitations.

---

## Frontend contract: required demo cases

The backend must consistently expose these fixtures:

1. **Evidence-supported closure**
   - FIT+ referral;
   - Arabic-script identity;
   - downstream Latin-script record without usable EID;
   - trusted link;
   - reader agreement with quotes;
   - consent granted;
   - closure and verified provenance.
2. **False-closure trap**
   - procedure exists;
   - incomplete exam, poor/ungraded prep, or missing histology;
   - Wasl holds/reopens instead of closing.
3. **Review/consent hold**
   - uncertain link or reader disagreement;
   - no outreach while consent is unknown;
   - review and eventual signed resolution.

The frontend owner is responsible for presentation. The backend owner is responsible for stable JSON, deterministic IDs, and a command that regenerates these cases.

## Quality gates before calling the project showcase-ready

- One command starts the whole demo with deterministic data.
- One integrated run uses strict safety policy, live/local reader, consent hold and the materialised API.
- HL7 v2 messages reach the canonical event path.
- Node/process kill, duplicate and reorder tests pass on the final path.
- A reader timeout, weak match, disagreement, incomplete evidence or missing consent always produces a hold/review state—not an automatic closure or dispatch.
- UI data is generated from the final run.
- Submission language labels every result as built, simulated or designed.

## Formerly out of scope: now built (Oct 2026)

These were listed as out of scope until the gates above passed. After stages 0–7, all six extensions were built. Each is optional and off by default, so the baseline is unchanged.

| Item | Where | Status |
|---|---|---|
| Image reader / HyperKvasir | `image/`, `pipeline/image.go`, `-image cached\|live` | Built, hold-only (it can never close a loop); weights published with the dataset authors' permission |
| Learn-then-Test statistical gate | `pipeline/ltt.go`, `scripts/ltt_*`, `-policy v7ltt` | Built; operating tier 0.995 (design floor 0.99, above the 0.989 uncorroborated cap), 0 errors on 12,600 cal-split loops |
| Lung-nodule/Fleischner loop | `pipeline/lung*.go`, `fleischner.go`, `-lung N` | Built on a synthetic CT report corpus |
| Direct-to-procedure workflow | `booking/booking/levers.py`, `booking/sim/levers_sim.py` | Simulated decision support |
| Prep-risk model | same (Dik 2015 score) | Simulated decision support; suggestions need a clinician order |
| Encoder retraining on external Arabic↔Latin data | `encoder/`, `-modelset v3` | Built and merged: Wikidata + NETransliteration (CC0), real-variant R@1 0.548 → 0.660; JRC-Names and ANETAC rejected on licence |

## Seven loop types: built, with simulated evaluation

The canonical pipeline now has seven loop types: CRC and cervical (default cohort), lung nodule (`-lung N` / `WASL_LUNG=N`), diabetic retinopathy (`-dr N` / `WASL_DR=N`), breast (`-breast N` / `WASL_BREAST=N`), lung cancer screening LDCT (`-ldct N` / `WASL_LDCT=N`) and IFHAS abnormal cardiometabolic-result follow-up (`-ifhas N` / `WASL_IFHAS=N`). LDCT and IFHAS are **built**, off by default, and evaluated only on synthetic patients, reports/results, attendance, notes and reviewers; no real-world clinical effectiveness claim is established. LDCT is distinct from the incidental-nodule programme. Fresh held-out text now covers LDCT screening/work-up and IFHAS free-text follow-up readers, but it is non-blind, single-author synthetic evidence without clinician labels; the integrated generators still reuse development texts.

IFHAS reader B is cached local Gemma 3 4B. Its prompt was tuned on the same 227-note synthetic development corpus that the integrated generator reuses. Reader B was unavailable on 19/227 notes; in the initial matched multi-programme run 96/437 follow-up report instances and 35/800 screens went to review. Integrated agreement, follow-up recall and false-closure results therefore are not independent reader evidence. On 52 fresh held-out follow-up notes, reader A scored 36/52, reader B 34/52 with 8 invalid/ungrounded answers, 0 agreed-but-wrong and 29/52 routed to review; unsafe single-reader pending/not-confirmed errors remain and are disclosed in `pipeline/RESULTS.md`. IFHAS coded/panel screening readers still need fresh held-out evidence. The 59-day routine clock contains a 28-day receiving-clinic Wasl assumption; the 10-day severe-result management clock is also a modelling choice.

**Post-hoc shared-baseline change (`9bfe0d2`).** With `ReuseLinkAnswer=true` (the default), simulated reused-link review draws are keyed by registration pair rather than twin item ID. This removes arrival-order dependence, but changes reviewer draws and can change results for every existing programme relative to `5e9140f`, including frozen-eval configurations. IFHAS-off parity at the new head is not parity with the old baseline. `ReuseLinkAnswer=false` retains the legacy item-keyed path and has a regression pinning the original answers. On the same seed-2026 cohort and settings, measured with-review recall changed from `5e9140f` to the new head as follows: CRC 0.851 → 0.860, cervical 0.962 → 0.957, lung 0.983 → 0.993, DR 0.997 → 0.996; breast was unchanged. All no-review programme blocks were identical and all normal safety counters stayed zero. This is a measured non-parity disclosure, not an improvement claim; details are in `pipeline/RESULTS.md` and local run paths are in the coordinator report.

**Post-hoc hardening status (2 Nov).** X10 status-race and identity-linkage fixes, a durable cannot-link constraint after EID-clash refusal, LDCT/IFHAS outreach consent/provider fixes, and the default-off `-eidguard` B8 closure guard are integrated. Fresh normal and x10 runs at `e27eefc` passed the measured safety checks after earlier failed runs were preserved and root-caused; see `pipeline/results/scale/README.md`. Fresh B8 seeds 17/29 reproduced 19 guard-off false closures under 2% wrong-EID stress and measured zero with the guard on, at a 1.15-point CRC recall cost and higher review load. Residual cross-person edges and impure clusters remain, so guard-on is not a perfect-linkage claim. Full evidence is in `pipeline/results/b8-hardening/` and `pipeline/RESULTS.md`. The final ten-lane frozen evaluation at `075abc5` is complete: held-out v2 CRC precision/recall 1.000/0.785, 0/3,035 CRC false closures, zero wrong-patient or unconsented releases, and 85/85 tamper detections per noise model; all programme closure-safety counters are in `pipeline/results/final/SUMMARY.md`. A read-only B8 recall-loss analysis is packaged in `pipeline/results/recall-analysis/`; its largest first-barrier family is identity/linking, and its generalized new-seed split proposal makes no model or threshold changes. Docker execution remains blocked because the CLI is unavailable; only static deployment checks passed. A post-hoc five-seed held-out v2 sensitivity with both simulated reviewer-error rates set to zero measured CRC recall 0.859 (0.835–0.883), 0/3,322 false closures and zero wrong-patient or unconsented releases, versus 0.785 under reviewer-error stress; see `pipeline/results/clean-review-sensitivity/`. This supplements rather than replaces the frozen stress result. All scale, guard and reviewer results are simulated; the 4% ordinary and 25% day-15 reviewer errors are stress assumptions, not measured clinician rates.

**CRC review-floor experiment (`8832e82`).** The built, opt-in `-linkfloor` changes review routing only; merge/closure safeguards and the default 0.2 remain unchanged. In paired simulated CRC cohorts, 0.05 failed the development gate, while 0.01 improved pooled clean development recall from 0.8331 to 0.8496 and passed separate synthetic reviewer-error stress. Untouched five-seed eval/v2 validation then improved recall from 0.8455 to 0.8625 (+1.7071pp), with zero listed safety/replay/analysis failures and no increased linkage contamination. Review burden increased, residual misses and nonzero linkage contamination remain, and the result establishes no clinical effectiveness or non-CRC safety claim. The post-hoc analyzer-only fix, exact per-seed results, workload and residual funnel categories are documented in `pipeline/RESULTS.md`; 0.01 is a validated non-default candidate, not a changed production default.

**CRC feature-first pair-scorer experiment (`dc21cec`, not selected).** The built, opt-in `pairfeat1` modelset appends seven pair features (indices 23-29) with a refit LightGBM and calibrators on the fixed encoder. It failed the pre-registered development gate: in paired simulated CRC cohorts (seeds 307/311/313, floor 0.01 both arms) pooled recall rose from 0.8811 to 0.8869 (+0.5874pp, below the +2.0pp gate), so stress and validation were not run and eval seeds remain untouched. Safety counts were zero in both arms; contamination decreased but was not zero, and this n=3 development figure is not a validated benefit. Frozen v1 training noise contained no same-person decade-digit-only DOB pairs, so the earlier decade-DOB hypothesis was not tested. The default model and default floor 0.2 are unchanged and `pairfeat1` is not a candidate. Post-hoc changes, artifact identities and per-seed results are in `pipeline/RESULTS.md`; no non-CRC safety or recall claim is made and no clinical effectiveness is proven.

**CRC decade-noise pair-scorer experiment (`9bf8017`, not selected).** The built, opt-in `pairfeat2` modelset reuses the `pairfeat1` features and trains on new seeds with `v1t` noise, v1 plus a training-only one-digit DOB typo (probability 0.20 within the 0.3 DOB corruption), so same-person decade-digit-only pairs are learnable; v2's evaluation corruption was deliberately not used. In paired simulated CRC development cohorts (seeds 317/331/337, floor 0.01 both arms) pooled recall rose from 0.8776 to 0.9215 (+4.3807pp, paired n=3 t interval +3.23 to +5.53pp, descriptive only), but the pre-registered selection gate failed on contamination and workload: false-merge pairs 31 to 43, impure clusters 28 to 39, cross-person edges 29 to 40 (up in every seed) and distinct link-review pairs +24.7% against a 10% cap. Safety counts were zero in both arms. Stress and validation were not run, eval seeds remain untouched, and no threshold or gate was changed after seeing results. The default model and default floor 0.2 are unchanged and `pairfeat2` is not a candidate; the contamination increase is an untested next thing to investigate, not a validated direction. Details are in `pipeline/RESULTS.md`; no non-CRC safety or recall claim is made and no clinical effectiveness is proven.

**CRC namesake hard-negative pair-scorer experiment (`f3a2f3c`, not selected).** The built, opt-in `pairfeat3` modelset is the successor to `pairfeat2`. It keeps `pairfeat2`'s features, `v1t` noise, seeds and fit, and adds training-only synthetic namesakes (rate 0.10, an assumption): people sharing a first name, origin and sex with a different surname, a DOB one or two digits off and a new Emirates ID. The idea was that `pairfeat2`'s contamination came from training lacking such different-person pairs. In paired simulated CRC development cohorts (fresh seeds 347/353/367, floor 0.01 both arms) pooled recall rose from 0.8612 to 0.9199 (+5.8655pp, n=3, descriptive only), but the pre-registered gate failed again: false-merge pairs 24 to 32, impure clusters 23 to 27, cross-person edges 23 to 28 and distinct link-review pairs +23.8% against a 10% cap. Safety counts were zero in both arms. The plain-cal count of different-person pairs at raw >= 0.7 fell from 31 to 23 (same cal seed 2011, like for like), which did not carry through to development contamination. Stress and validation were not run, eval seeds remain untouched, and no threshold or gate was changed after seeing results. The default model and default floor 0.2 are unchanged and `pairfeat3` is not a candidate. Details are in `pipeline/RESULTS.md`; no non-CRC safety or recall claim is made and no clinical effectiveness is proven.

**CRC symmetric-namesake pair-scorer experiment (`a643daf`, not selected).** The built, opt-in `pairfeat4` modelset is the successor to `pairfeat3`. It keeps `pairfeat3`'s recipe and adds training-only family-name namesakes (shared last name, origin and sex, different first name, DOB one or two digits off, new Emirates ID; rate 0.10, an assumption), the shape `pairfeat3`'s diagnosis found by eye in 7 of its 14 new false edges. For this experiment only, the review-load gate counted different-person link-review pairs (limit 1.10x baseline, or 1 if baseline is 0) instead of all pairs; this was pre-registered and user-approved before any run, and is not applied to earlier experiments. In paired simulated CRC development cohorts (fresh seeds 433/449/461, floor 0.01 both arms) pooled recall rose from 0.8569 to 0.9153 (+5.8394pp, n=3, descriptive only) and the wrong-pair gate passed (7 to 4), but false-merge pairs rose 38 to 41 and cross-person edges 36 to 37, so the selection gate failed; impure clusters stayed at 35 and safety counts were zero in both arms. Total link-review pairs rose 522 to 638 (not gating; mostly same-person). At these small counts the wrong-pair limit allows almost no net new wrong pair, which is disclosed in `pipeline/RESULTS.md`. Stress and validation were not run, eval seeds remain untouched, and no threshold or gate was changed after seeing results. The default model and default floor 0.2 are unchanged and `pairfeat4` is not a candidate. No non-CRC safety or recall claim is made and no clinical effectiveness is proven.

## Working conventions

- Prefer a narrow vertical slice over parallel architecture rewrites.
- Keep `pipeline/` canonical; delete or clearly deprecate copied logic once callers migrate.
- Add a focused test with every safety or state-machine change.
- Never overwrite an audit decision; append a new signed event.
- Keep model calls local/demo-only and never introduce real patient data.
- Update `pipeline/RESULTS.md` only with results reproduced from the final integrated command.
