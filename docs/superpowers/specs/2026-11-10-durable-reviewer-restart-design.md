# Durable reviewer restart design

Date: 2026-11-10
Base: `8f9eb76034f402a65e7e09501f4b35a0e0ce0ae6`
Scope: proposal #3 only. Simulated reviewer infrastructure; no reviewer/model, threshold, clinical, normalization, AMD64 or soak change.

## Invariants

1. One logical sign-off per review item, including replay after the one-hour JetStream dedupe window.
2. A prepared sign-off, its content hash, exact local ledger line, worklist effect and OUT position are durable before any publish.
3. Recovery completes before consuming new OUT messages. An already-published sign-off is reconciled by identity and content, not regenerated.
4. Local ledger writes are checked, fsynced, closed and idempotent. A conflicting stored sign-off or ledger line fails closed.
5. Double review, senior resolution, worklist reuse and canonical append-only history are unchanged.
6. Reviewer status is derived from recovered durable state. A message is ACKed only after its required durable state is complete.

## Architecture

Create an R3 file-backed JetStream KV bucket `REVIEWER` for each isolated run. Keep the implementation in a focused `reviewer_durable.go`; `services.go` retains process orchestration and the existing review/hold policy. Existing worklist helpers in `governance.go` remain the source of skip/record semantics.

### Records

A review-item key is a bounded hash of the item ID; the record retains the literal item ID and validates it on decode. Records contain:

- schema version and item ID;
- state, moving forward only in this order: `prepared`, then `ledger_written`, then `published` (plus a durable skipped-item marker);
- the review item and final `Signoff` as prepared by the existing reviewer policy;
- SHA-256 of the canonical sign-off resource body;
- exact bytes of the existing `reviewer_decisions.ndjson` line, including its original timing fields;
- wrong-answer flag and worklist effect needed by `idWorklist.record` and `linkWorklist.record`;
- originating OUT stream sequence and publication timestamp;
- optional observed FHIR stream sequence after publication.

A separate status record stores the last OUT sequence and counters derived from recovered item records. KV updates validate item identity and use revision-checked transitions; stale state regression or payload conflict is an error.

### Write and recovery flow

For a new review item:

1. Evaluate the existing reviewer once and compute its worklist effects, wrong flag, canonical sign-off body/hash and exact local ledger line.
2. Persist `prepared` with fsync durability supplied by the R3 file-backed KV write, before calling publish.
3. If the item is already present in FHIR, require identical sign-off content. Otherwise publish the prepared body with the existing `signoff/<item-id>` message ID. Publication errors leave the record prepared and the message unacknowledged.
4. Persist `published`, including the observed FHIR sequence when available.
5. Append the exact ledger line through the checked write/sync/close helper. An existing identical line is accepted; absence is appended; conflict is fatal.
6. Persist `ledger_written`, derive status from the record set, then ACK the OUT message.

For a worklist skip, persist the skipped marker and status before ACK. For non-review OUT messages, persist only status before ACK. Worklist skip evaluation uses restored maps, so a queued twin makes the same skip decision after restart.

At startup, before creating the message iterator:

1. Read and validate all reviewer records and the local reviewer ledger. Sort item records by OUT sequence and item ID for deterministic recovery.
2. Restore seen IDs, worklist maps, counters and last OUT position from records. Re-apply only their recorded worklist effects; never invoke reviewer draws during recovery.
3. Reconcile each prepared/published record against FHIR by `signoff/<item-id>`, scanning retained stream history when necessary. Require the stored content hash; a conflicting same-ID body is fatal. A missing body is published only from the prepared record, including after dedupe expiry.
4. Finish checked/idempotent local ledger reconciliation and state transitions before consuming new messages.

The OUT consumer becomes a named explicit-ACK durable consumer. ACK failure leaves delivery eligible for replay; replay uses the same recovered records and cannot create a second logical answer.

## Failure semantics

- KV/read/write or state-transition failure: no publish and no ACK where the failure precedes those effects; processing stops rather than skipping an item.
- Existing same-ID FHIR body with a different hash: stop with no publish or local-ledger update.
- Local ledger write short-write/sync/close failure: preserve the durable prepared/published record, do not ACK, and reconcile on restart. Reuse `appendToLedgerFile` semantics rather than adding unchecked writes.
- Crash after FHIR acknowledgement but before `published`: startup finds the body by ID/hash, marks it published and completes the ledger without republishing.
- Crash after local append but before `ledger_written`: startup accepts the identical existing line and completes the checkpoint without appending again.
- Presenter-hold crash before `prepared`: no sign-off effect exists; the existing hold path may be recomputed. A prepared presenter answer is reused unchanged.

## Bounded restart matrix

Use real single-node NATS with fresh process-local reviewer state for each restart:

1. Before publish.
2. After FHIR acknowledgement but before the `published` KV update.
3. After the checked local append but before `ledger_written`.
4. Second-identifier worklist reuse.
5. Link-answer worklist reuse.
6. Replay after the dedupe window with the sign-off absent from the in-memory/seen cache.
7. Local short-write, sync and close failures.

For every case assert one logical sign-off per item, no lost review item, no conflicting ledger line, identical worklist maps, recovered status position/counters, exact ledger bytes and a matching final comparison hash. Include double-review/senior-resolution fixtures to prove their existing sign-off content is preserved. Tests restart retained NATS/KV state, not a 24-hour soak or held-out cohort.

## Files

- New focused `pipeline/reviewer_durable.go` and `pipeline/reviewer_durable_test.go`.
- `pipeline/services.go`: replace process-local reviewer checkpoint handling with the durable service and explicit ACK consumer.
- `pipeline/governance.go`: only small serialization/effect helpers if needed; reviewer policy remains unchanged.
- `docs/reliability/ADVERSARIAL.md`, `docs/reliability/adversarial-matrix.json`, and a focused evidence report.

No new local-WAL artifact, recovery-bundle format, demo command or external dependency is planned. NATS backup already covers stream/KV state; any omission found during implementation must be reported before scope changes.

## Limits

Exactly-once external side effects, cross-host filesystem semantics, physical power loss, human UI behavior and live-agent equivalence remain unproven. Evidence is simulated-reviewer evidence under controlled Wasl replay, not clinician performance or production exactly-once delivery.
