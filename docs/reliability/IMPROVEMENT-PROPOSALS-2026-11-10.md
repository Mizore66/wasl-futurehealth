# Ranked reliability improvements for the Wasl submission

**Research date:** 10 Nov 2026

**Research base:** `e73abc9be315cb6f82acea41dac0c166b3929c2c`

**Mode:** research only; no implementation or verification commands were run.

**Citation note (2026-11-12):** `file:line` citations were renumbered to the current files. The cited `ADVERSARIAL.md` lines now record the later follow-ups for the hook, clinical-normalization and reviewer-restart findings; the wording quoted here is from the research base.

## Recommendation

**Do item 1 first: make FHIR Subscription HTTP acknowledgement contingent on durable ingest, with recoverable reorder/overflow handling.** It closes a demonstrated accepted-data-loss window in the built ingest path. The other items are independent follow-ups, not prerequisites for one another.

All five items below are **designed proposals**, not completed work. Existing code and tests are described as **built**; existing measurements are **simulated** unless explicitly stated otherwise. No clinical effectiveness, production readiness, real-feed connectivity, or new evaluation result is claimed. Implementation should wait until the currently red hosted gates are green. Items 1–4 require final hosted CI before merge; item 5 additionally needs a clean Linux Docker host for its release evidence.

Deliberately excluded: Basic/CORS mutation tests, licence-notice review, supply-chain Action/provenance repairs, active soak work, AMD64 work, model tuning, new programmes, and any evaluation-seed or corpus consumption.

## Ranked summary

| Rank | Proposal | Value and measurable outcome | Effort | Release gate |
|---:|---|---|---|---|
| **1** | Durable FHIR notification acceptance and replay | Prevents a crash from losing a notification after Wasl returned HTTP 200; target **0 accepted-but-unpersisted notifications** and explicit replay of overflow | **2–3 days** | Final hosted CI; process-kill tests, not a soak |
| **2** | Fail-closed clinical timestamp, programme and report-status normalization | Prevents malformed/unknown input from silently becoming day-0 CRC evidence; target **0 automatic closures from non-final or semantically invalid reports** in focused fixtures | **1–2 days** | Final hosted CI |
| **3** | Durable, idempotent reviewer restart | Makes the simulated review evidence reproducible across crashes; target **one logical sign-off per review item** after publish/restart/post-dedupe-window replay | **2–3 days** | Final hosted CI; bounded crash matrix, not a soak |
| **4** | CI skip accounting and required-fixture manifest | Turns silent asset-dependent skips into explicit evidence; target **zero unexpected skips**, with every intentional skip named and justified | **0.5–1 day** | Final hosted CI is required to prove the lane |
| **5** | Confidential, versioned backup bundles with rotation and a clean-host restore | Adds confidentiality to an already authenticated recovery bundle; target wrong-key/tamper refusal plus matching logical hashes after key rotation and restore | **2–4 days**, plus host availability | **Blocked for final evidence by a clean Linux Docker host and final hosted CI** |

## 1. Durable FHIR notification acceptance and replay

### Current evidence

- The unresolved-findings register says the hook queues in memory before responding, so a process crash can lose accepted-but-unpublished notifications; reorder overflow also has no durable dead-letter: `docs/reliability/ADVERSARIAL.md:104`.
- The built handler sends the decoded body to `c.inbox` and immediately returns 200; only a full channel receives 503: `pipeline/fhirsub.go:213-237`.
- The channel is drained asynchronously by `processLoop`: `pipeline/fhirsub.go:342-351`. Reordered events beyond `maxSubscriptionReorder` increment a counter and are dropped from the process-local buffer: `pipeline/fhirsub.go:364-380`.
- The machine-readable matrix records the same boundary, not a passing durability result: `docs/reliability/adversarial-matrix.json:40-48` and `:148-156`.

### Concrete deliverable

Introduce a bounded JetStream-backed notification inbox (or an equivalently durable handoff into the existing FHIR stream). A hook success response must occur only after every event in that notification has a durable sequence/acknowledgement. Preserve idempotency keys across sender retries, bound waiting time, and return a retryable failure when durable acceptance cannot be confirmed. Persist reorder-pending records and an explicit dead-letter/replay record rather than silently dropping overflow. Add status counters for durable accepts, retryable accepts, replayed notifications and dead letters.

### Affected files

`pipeline/fhirsub.go`, `pipeline/fhirsub_types.go`, `pipeline/services.go` (stream setup), `pipeline/adversarial_test.go`, `pipeline/nats_adversarial_test.go`, `pipeline/main.go` (retained-log/report wiring if the new stream is dumped), `scripts/demo.sh`, `docs/reliability/ADVERSARIAL.md`, `docs/reliability/adversarial-matrix.json`, and `pipeline/RESULTS.md` only with reproduced results.

### Verification

Use deterministic, bounded tests with synthetic fixtures only:

1. Kill the consumer immediately after HTTP 200 and before asynchronous processing; after restart, every acknowledged event must appear exactly once logically in FHIR output.
2. Inject a JetStream publish failure/timeout; the hook must not return success, and sender retry must converge without a duplicate logical effect.
3. Deliver duplicate and reordered notifications across restart, including reorder capacity exhaustion; every event must be durably pending, replayed, or explicitly dead-lettered—never only counted and discarded.
4. Compare the materialized `comparison_sha256`, accepted/durable/dead-letter counters and chain verification before and after replay. Run the ordinary Go/race/integration lanes in hosted CI, not a soak.

### Remaining limits

This proves a synthetic single-system failure contract, not Malaffi behaviour, multi-region durability, R3 partition safety, or end-to-end clinical correctness. A dead letter still requires an operator replay procedure. Do not convert the existing simulated zero-loss figures into a new claim until the final integrated command is rerun.

### Primary source

HL7's Subscriptions Backport IG defines rest-hook delivery as HTTP POST and gives a success HTTP code such as 200 as the endpoint's acceptance response; it does **not** define a sender retry policy. Wasl must therefore test its own retry/dedupe contract rather than assume one: <https://hl7.org/fhir/uv/subscriptions-backport/STU1.1/channels.html#rest-hook>.

## 2. Fail-closed clinical timestamp, programme and report-status normalization

### Current evidence

- Clinical semantics remain an explicit safety concern: day-0 timestamp defaults, unrecognized-program defaults and non-final report-status gating are unresolved: `docs/reliability/ADVERSARIAL.md:103`.
- `tsToDay` returns `0` for every parse failure: `pipeline/types.go:17-22`.
- `progOfText` defaults any unrecognized text to `crc`: `pipeline/rules.go:96-114`; the linker has another empty-programme-to-CRC fallback: `pipeline/linker.go:735-749`.
- The DiagnosticReport normalizer copies `status` but extracts report timing/text without a final-status validity gate: `pipeline/fhir.go:59-60` and `:160-203`.

### Concrete deliverable

Return structured normalization errors instead of implicit defaults. Require valid, resource-appropriate clinical timestamps under the Wasl ingest profile; route missing/malformed values to rejection or review, never day 0. Replace the programme fallback with an explicit mapping and `unknown_programme` quarantine/review outcome. Define a versioned DiagnosticReport status policy: preliminary/partial/registered reports cannot close; cancelled/entered-in-error reports cannot supply closing evidence; corrected/amended/appended versions retain version chaining and re-evaluation semantics. Propagate the same decision through FHIR, HL7 conversion and offline replay.

### Affected files

`pipeline/types.go`, `pipeline/rules.go`, `pipeline/fhir.go`, `pipeline/hl7.go`, `pipeline/linker.go`, `pipeline/services.go` (invalid-event accounting), focused new Go regression files, `docs/reliability/ADVERSARIAL.md`, `docs/reliability/adversarial-matrix.json`, and `docs/SECURITY.md` if the rejection boundary changes.

### Verification

Table-drive each supported resource through direct FHIR, HL7 `ToResource`, and materialized-log replay. Include malformed/missing dates, valid partial dates, unknown programme text, every DiagnosticReport status, corrected versions, cancelled reports and entered-in-error withdrawals. Assert no non-final/invalid evidence closes or releases outreach, no unknown programme becomes CRC, valid current fixtures retain their decisions, and live/replay hashes agree. These fixtures must be newly minimal and synthetic; do not consume evaluation seeds or corpora.

### Remaining limits

Base FHIR R4 makes some fields optional, so the required-field profile is a Wasl safety choice and must be documented as such. Status validation does not establish report truth, clinician agreement, reader accuracy or clinical effectiveness. Real-feed profile mappings remain unverified.

### Primary sources

- FHIR R4 defines `final` as “complete and verified by an authorized person,” while `partial` may be incomplete/unverified and `preliminary` still has non-final results; cancelled and entered-in-error are separate withdrawal states: <https://hl7.org/fhir/R4/valueset-diagnostic-report-status.html>.
- FHIR R4 requires valid dates and defines partial `dateTime` forms; it provides no rule coercing malformed values to a real instant: <https://hl7.org/fhir/R4/datatypes.html#dateTime>.
- Base `ServiceRequest.authoredOn` and `code` are optional (`0..1`), which is why Wasl's stricter ingest profile must be explicit rather than presented as a base-FHIR requirement: <https://hl7.org/fhir/R4/servicerequest-definitions.html#ServiceRequest.authoredOn> and <https://hl7.org/fhir/R4/servicerequest-definitions.html#ServiceRequest.code>.

## 3. Durable, idempotent reviewer restart

### Current evidence

- The executed matrix marks reviewer crash/restart **not run** and says reviewer progress is not durable under all such restarts: `docs/reliability/ADVERSARIAL.md:90` and `:105`.
- The built reviewer uses an ordered consumer, then initializes `seen` and both worklists as process-local maps: `pipeline/services.go:697-710`.
- It marks an item seen before review/publish: `pipeline/services.go:727-751`. Sign-off publish occurs before an unchecked local NDJSON write: `pipeline/services.go:797-805`.
- The local full regression passed with six asset-dependent skips; it did not supply reviewer crash evidence: `docs/reliability/evidence/verification-2026-11-09.manifest.json:179-201`.

### Concrete deliverable

Add a durable reviewer checkpoint/outbox keyed by review item ID, containing the sign-off hash, worklist reuse state and OUT stream position. Restore it before consuming, make publish and local ledger writes checked/idempotent, and reconcile an already-published sign-off after a crash instead of generating a second logical answer. Keep reviewer status derived from the recovered checkpoint. Preserve double-review and senior-resolution rules; this is infrastructure, not a reviewer/model change.

### Affected files

`pipeline/services.go`, reviewer/worklist helpers in `pipeline/governance.go`, `pipeline/recovery.go` only if checkpoint backup semantics change, new focused reviewer restart tests, `scripts/demo.sh`, `scripts/recovery.py` only for any new checkpoint artifact, `docs/reliability/ADVERSARIAL.md`, and `docs/reliability/adversarial-matrix.json`.

### Verification

Run a bounded synthetic matrix with kills: before publish, after JetStream acknowledgement but before local append, after local append, during second-identifier/link-answer reuse, and after the one-hour JetStream dedupe window. Restart from retained state and assert one logical sign-off per item, identical worklist reuse, no lost review item, no conflicting ledger line, correct status position, and matching final comparison hash. Check injected short-write/sync/close errors. Do not run a soak or use held-out evaluation inputs.

### Remaining limits

Exactly-once external side effects are not established; the target is idempotent logical effect under Wasl's controlled replay. Cross-host lock/filesystem behaviour, human UI state, physical power loss and live-agent equivalence remain separate. The evidence remains simulated-reviewer evidence, not clinician performance.

### Primary sources

NATS documents ordered consumers as ephemeral, without individual acknowledgements, and warns that a crashed long-running job “starts over”; it recommends named consumers with explicit acknowledgements for completion-tracked work: <https://docs.nats.io/learn/jetstream/ordered-consumer>. Explicit acknowledgement stops redelivery but does not make processing exactly once: <https://docs.nats.io/learn/jetstream/acknowledgment>.

## 4. CI skip accounting and required-fixture manifest

### Current evidence

- The recorded full run explicitly skipped six asset-dependent tests, including API contract/RBAC, booking and HL7 cohort coverage: `docs/reliability/ADVERSARIAL.md:70`; exact names are in `docs/reliability/evidence/verification-2026-11-09.manifest.json:193-200`.
- The hosted Go lane currently treats `go test` exit status as sufficient: `.github/workflows/ci.yml:45-60`. The integrated lane generates a demo and reruns tests, but has no skip assertion: `.github/workflows/ci.yml:145-157`.
- The aggregate `required` job requires job success without skipped jobs, not individual Go tests: `.github/workflows/ci.yml:476-506`.

### Concrete deliverable

Emit `go test -json` and a small skip manifest for both Go lanes. Fail on any unexpected test skip; use a reviewed exact-name allowlist only for intentionally opt-in external/destructive or evaluation-corpus tests, with reason and owning documentation. Require the integration lane's generated fixtures to cover the named API/RBAC/booking/HL7 smoke tests or fail explicitly. Upload the manifest as CI evidence without clinical payloads.

### Affected files

`.github/workflows/ci.yml`, a focused script such as `scripts/check_go_test_skips.py` plus `scripts/test_check_go_test_skips.py`, `docs/reliability/README.md` or a short section in `docs/reliability/ADVERSARIAL.md`, and the verification manifest schema/documentation. No workflow Action pins or provenance tooling should change under this proposal.

### Verification

Unit-test the parser with pass/fail/skip JSON, subtests, package-level skip events, allowlisted and unexpected names. In the final hosted CI run, prove that generated demo assets exist, required fixture tests report `pass` rather than `skip`, every remaining skip is explicitly allowlisted, and deleting one required fixture makes the lane fail. Do not enable optional evaluation corpora to make a skip disappear.

### Remaining limits

A pass/skip manifest proves execution accounting, not test quality or coverage. It cannot certify tests needing unavailable licensed/local-only assets, and an allowlist can become stale; review it on every test rename. No hosted result exists in this research report.

### Primary source

No external source is needed: this proposal follows directly from the repository's recorded skips and current workflow commands.

## 5. Confidential, versioned backup bundles with rotation and clean-host restore

### Current evidence

- The built recovery bundle authenticates its manifest and payloads but is explicitly **not encrypted**: `docs/RECOVERY.md:14-16` and `:28-30`.
- The unresolved limits state that bundle HMAC does not encrypt contents, key rotation is not implemented, and only a same-host Podman clean-volume restore was observed: `docs/RECOVERY.md:282-297`.
- The implementation copies plaintext file/NATS payloads and authenticates hashes: `scripts/recovery.py:42-88` and `:91-121`; verification subsequently hashes those plaintext files: `scripts/recovery.py:127-158`.
- The observed drill is valuable but simulated and same-host, not physical second-machine or Docker Engine evidence: `docs/RECOVERY.md:238-280`.

### Concrete deliverable

Define a versioned bundle format using a reviewed AEAD implementation, a fresh data-encryption key and nonce per bundle, authentication of nonsecret format/key metadata, and wrapping under a key held outside the bundle. Add key IDs and a rotation procedure that can decrypt older bundles without reusing nonces or destroying the old key prematurely. Refuse missing/wrong keys and modified ciphertext before creating run files or contacting the restore target. Preserve the existing logical stream and comparison-hash checks. Update the runbook with secure deletion/retention responsibilities rather than claiming HMAC provides confidentiality.

### Affected files

`scripts/recovery.py`, `scripts/test_recovery.py`, `pipeline/recovery.go` and `pipeline/recovery_test.go` for the NATS payload boundary, `scripts/production_secrets.sh`, `scripts/production_volume_load.sh`, `deploy/production.compose.yml`, `docs/RECOVERY.md`, and dependency lock/manifest files only if a reviewed crypto dependency is selected. Do not change supply-chain Action/provenance configuration in this item.

### Verification

Unit/integration checks must cover plaintext-marker absence, wrong/missing/old/new key behaviour, nonce uniqueness across bundles, ciphertext/manifest truncation and tamper refusal with zero restored files/streams, rollback, and legacy-v1 migration or explicit rejection. Then, on a **clean Linux Docker host**, perform the documented seed → backup → rotate → empty-volume restore → ordinary restart drill from a clean checkout; require equal logical stream hashes and API `comparison_sha256`, anonymous 401, non-root users, and no secrets in process arguments/logs. Record measured bundle size and restore duration without claiming an RPO/RTO until objectives are set.

### Remaining limits

Encryption does not provide off-site isolation, object lock/WORM, KMS/HSM identity, availability, secure host memory, or proof against a malicious host. A same-host Linux restore still is not physical second-machine DR. Streaming/chunking and large-run restore objectives remain unresolved. Final evidence is **blocked** until both a clean Linux Docker host and green final hosted CI are available; no container drill was run here.

### Primary source

RFC 5116 defines AEAD as providing confidentiality plus authentication and requires nonce uniqueness for each key; this supports using a specified AEAD interface rather than adding a custom encrypt-and-MAC construction. It does not solve key management: <https://www.rfc-editor.org/rfc/rfc5116#section-2.1> and <https://www.rfc-editor.org/rfc/rfc5116#section-3.1>.

## Not run and evidence boundaries

- No tests, fuzzing, containers, Docker/Podman, Linux/AMD64 checks, soak, model tuning, evaluation, seed/corpus consumption, deployment, external service, or buyer/organiser contact was performed for this research.
- No repository code, workflow, recovery tool or submission claim was changed. Existing passing results were read as historical evidence, not rerun or extended.
- The cited local adversarial environment is macOS arm64 with a real single-node NATS server, not clean Linux Docker: `docs/reliability/ADVERSARIAL.md:49-70`.
- Existing submission evidence already distinguishes cached frozen evaluation, separate live-reader/HL7 runs and simulated patients/reviewers: `docs/SUBMISSION-DRAFT.md:49-83` and `:111-120`. These proposals do not alter those denominators or limitations.
- Hosted gate status was not independently queried. Per the task constraint, all implementation remains paused while hosted gates are red; the report itself is the only committed change.
