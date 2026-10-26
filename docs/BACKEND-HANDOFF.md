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

**Done when:** the frontend can render the three demo cases exclusively from pipeline/API data, without invented client-side records.

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

## Explicitly out of scope until the gates above pass

- Image reader / HyperKvasir.
- Learn-then-Test statistical gate.
- Lung-nodule/Fleischner loop.
- Direct-to-procedure workflow.
- Prep-risk model.
- Encoder retraining on external Arabic↔Latin datasets.

## Working conventions

- Prefer a narrow vertical slice over parallel architecture rewrites.
- Keep `pipeline/` canonical; delete or clearly deprecate copied logic once callers migrate.
- Add a focused test with every safety or state-machine change.
- Never overwrite an audit decision; append a new signed event.
- Keep model calls local/demo-only and never introduce real patient data.
- Update `pipeline/RESULTS.md` only with results reproduced from the final integrated command.
