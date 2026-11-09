# Durable FHIR notification acknowledgement/replay evidence

Date: 2026-11-10
Scope: approved proposal #1 only
Required integration base: `ab09b3cb8fb6307e639680d03b4e01e6c05f858f`
Tested code revision: `b1fcfae0f34f310d5cb86a72b941fe72a6b54793` (the following evidence-only commit changes no pipeline files)
Evidence type: local, synthetic, single-node NATS; no hosted-green or production claim

## Design and invariants

- `FHIRIN` is a file-backed JetStream work-queue stream with a 4,096-message limit, 256 MiB byte limit, 1 MiB message limit, and `DiscardNew`. A full or unavailable inbox returns 503; decode, id-only vreads/semaphore waits, and durable publish share one two-second acceptance context.
- HTTP 200 is emitted only after JetStream confirms durable notification acceptance. The notification `Nats-Msg-Id` is the SHA-256 of the exact body, so exact sender retries are idempotent inside the stream dedupe window.
- Inbox messages use explicit acknowledgement. `processInboxMessage` persists every included event under `FHIRSUB` key `pending.<subscription-hash>.<zero-padded-event-number>` before its `DoubleAck`; the record carries the verified provider and full canonical subscription reference. One canonical subscription/event key atomically serializes conflicting providers and instance ownership. Processing, payload/provider-identity-conflict, and storage failures are retryable and do not acknowledge the inbox message.
- Reorder, gap, acknowledgement, and checkpoint state is scoped by canonical subscription identity. Event numbers remain unique and contiguous within one subscription; the implementation does not permit two providers to reuse one number within that subscription. A conflicting provider or payload for pending or dispatched state fails closed instead of being deduplicated and acknowledged, and conflicting duplicate entries are rejected before any event in that notification is dispatched.
- New pending records carry an instance ownership token. Creation uses `Create`; recovery and duplicate-delivery ownership transfers use revision-checked `Update`; pending deletion uses `LastRevision`. A stale instance therefore cannot overwrite or delete a newer owner's record. A lost ownership claim retains a scheduled gap retry. Checkpoints are subscription-qualified and written with monotonic CAS retries. Legacy pending records bind only to the subscription named by the legacy checkpoint, and legacy checkpoints cannot rewind newer scoped state. Exact retries of recovered legacy overflow reuse that legacy key rather than creating a second scoped identity. Legacy lookup is restricted to the state bound to that legacy checkpoint (or the no-checkpoint default compatibility state), and a migrated record's stored subscription must also be empty or match the requesting state. Another subscription's legacy record therefore cannot block scoped replay even when no legacy checkpoint binding is available.
- Reorder overflow remains as durable pending state. Pending records are restored after restart and replayed by inbox or `$events` gap-fill processing only when overflow exists and reorder capacity has freed. Ordinary notifications and heartbeats do not rescan the backlog. Recovery of an incomplete contiguous prefix initializes `gapSince`; failed startup `$status` polls and malformed, empty, or invalid HTTP-200 status responses also leave a retry timestamp, including checkpoint-only recovery with no buffered event. `$status` and `$events` responses must match the requested subscription. No overflow is silently discarded.
- The canonical `fhir.<provider>.res` subject, body, `Nats-Msg-Id=<provider>/<Type>/<id>/v<vid>`, and numeric `Wsi-Evn` event number are unchanged. Pending state is removed only after canonical JetStream acknowledgement and a successful ownership/revision check. Canonical IDs and the existing seen ledger suppress duplicate logical output after crash windows and beyond the inbox dedupe window.
- This is exactly-once **logical effect under Wasl-controlled replay**, not external exactly-once delivery. Overflow uses durable pending/replay rather than dead-lettering; `dead_letters` is reported as zero.

Invariant checklist:

1. No HTTP 200 without a durable inbox acknowledgement.
2. No inbox acknowledgement before durable pending storage for every event in the notification.
3. Canonical publish before inbox acknowledgement is safely redelivered with one logical effect.
4. Reorder overflow is durable and replayable, including across consumer restart.
5. Authentication, strict decode, unknown-source rejection, bounded waits, and 503 backpressure remain fail-closed.
6. Same-number events from distinct subscription/provider feeds retain independent pending, reorder, checkpoint, restart, and gap-fill state.
7. Concurrent instances use CAS ownership; stale publishers cannot clear a newer owner's replay state.
8. Recovered gaps remain scheduled after startup status failure without a busy backlog scan.

## RED evidence before production changes

The initial focused command was:

```sh
go -C pipeline test -run 'Test(HookAcceptedNotificationIsDurableBeforeConsumerRestart|HookDurablePublishFailureThenRetryStoresOneNotification|SubscriptionReorderOverflowRemainsDurablePending)$' -count=1 -v
```

All three tests failed for the intended reasons:

- `HTTP 200 left 0 durable notifications, want 1` (the old hook only queued in memory).
- `publish without an inbox returned 200, want retryable 503`.
- `reorder overflow was discarded without a durable pending record`.

Restart/replay tests were then added before their production methods existed. The focused build was RED because `recoverPending` and `processInboxMessage` were undefined. Canonical setup was separately RED with `stream not found` for `FHIRIN`.

During the first recovery implementation, the reorder and overflow subtests remained RED. Root-cause tracing into the installed JetStream client showed that its key lister stops when its context is cancelled; the implementation had cancelled that context before consuming the keys. Keeping the context alive through consumption made both subtests green without changing their expectations.

Independent review then produced four additional RED regressions before their fixes: gap-fill stopped before durable event 4098 instead of replaying it and advancing to 4099; a blocked id-only hook returned 400 after 5.65s instead of a bounded retryable response; an acknowledged pending key remained forever; and a heartbeat attempted an unnecessary full backlog scan. A fifth regression pinned retention of dispatched-but-not-yet-canonically-acknowledged pending state. Focused re-review after the fixes reported no remaining Critical or Important issues.

A subsequent independent identity/startup review added this RED command before any follow-up production edit:

```sh
go -C pipeline test -run 'Test(SubscriptionIdentitySurvivesRestartAndGapFill|SubscriptionCheckpointsAreScopedByIdentity|ConcurrentSubscriptionInstancesDoNotDeleteNewerPendingOwner|RecoverPendingSchedulesGapFillAfterStartupStatusFailure|CanonicalPublishBeforeInboxAckRedeliveryIsIdempotent|RecoverPendingClearsAlreadyAcknowledgedRecord)$' -count=1 -v
```

The identity restart test recorded one pending record instead of two, zero subscription gap fills, and loss of both providers' events; the checkpoint test found zero scoped checkpoints; the concurrent-instance test observed the stale publisher deleting the replacement owner's only pending record; and startup status failure produced zero gap fills. `TestSubscriptionEventProviderConflictFailsClosed` was separately RED because the conflicting provider was deduplicated without an error. The replacement-consumer and already-acknowledged cleanup tests passed after correcting a test-only JetStream wildcard mistake; they are evidence repairs, not claimed RED production reproductions. The first GREEN attempt also exposed the distinction between `pending.*` and `pending.>` for the new subscription-qualified keys; both production scans and test filters were corrected before the final runs.

A second independent review then produced another RED run before its production fixes:

```sh
go -C pipeline test -run 'Test(RecoverPendingOwnershipConflictRetriesGapFill|RecoveredCheckpointRetriesStatusAfterStartupFailure|LegacyPendingUsesLegacyCheckpointSubscription|ConcurrentSubscriptionProviderConflictFailsClosed|SubscriptionEventPayloadConflictFailsClosed|GapFillRejectsMismatchedSubscription|LegacyCheckpointDoesNotRewindScopedCheckpoint|ConcurrentSubscriptionInstancesDoNotDeleteNewerPendingOwner|CanonicalPublishBeforeInboxAckRedeliveryIsIdempotent)$' -count=1 -v
```

The seven new tests reproduced a stranded lost-CAS claim, checkpoint-only startup status stall, legacy pending assigned to the wrong subscription, cross-instance provider acceptance, same-provider payload-conflict acknowledgement, mismatched gap-response dispatch, and scoped-checkpoint rewind. The strengthened stale-delete and publish-before-checkpoint replacement tests passed against the then-current implementation. Following coordinator confirmation, the final record design uses one canonical subscription/event key with provider in the value rather than redundant provider-qualified keys, so separate instances cannot both persist conflicting providers.

Final narrow re-review added a mismatched `$status` RED case and an id-only lost-CAS RED case. The combined slash-selection command exercised the status failure; the id-only case required and was run separately as `go -C pipeline test -run 'TestRecoverPendingOwnershipConflictRetriesGapFill/id-only$' -count=1 -v`, failing with event 2 stranded. The fixes allow pending-only id-only events to be vread for a claim retry and reject mismatched status identities before absorption. Final re-review reported no remaining findings for those two defects and passed its own focused race command.

A final correctness re-review then ran this RED command before the last production edits:

```sh
go -C pipeline test -run 'Test(DispatchedEventConflictFailsClosed|SingleNotificationConflictingDuplicateFailsClosed|RecoveredCheckpointRetriesAfterInvalidStatusResponse|LegacyOverflowExactRetryDoesNotDuplicateIdentity)$' -count=1 -v
```

Dispatched provider and payload conflicts were silently deduplicated; conflicting event-1 duplicates in one notification reached `DoubleAck`; malformed, empty, and invalid-status HTTP-200 responses each left only one status request; and legacy overflow retry created separate legacy/scoped records that made restart fail with a duplicate identity. The legacy fixture was corrected after the first implementation attempt to use the already-recovered instance with `pending=true` and no overflow buffer entry, matching the reviewed interleaving. The final implementation reuses the claimed legacy key for its remaining lifecycle, validates retained pending payloads for dispatched duplicates without claiming them, prevalidates each notification's duplicate identities before dispatch, and schedules retry for every invalid HTTP-200 status shape.

A final narrow legacy-isolation review added `TestScopedPendingIgnoresOtherSubscriptionLegacyKey`. Its pre-fix RED run failed with `conflicting legacy durable event for Subscription/sub-a event 2` when scoped A2 pending state coexisted with unrelated legacy B2. Both checkpoint-bound and record-only variants are covered; the latter also exposed and fixed a fallthrough ownership error before the final run. The fix considers the legacy key only for the state bound to that legacy subscription, while the existing duplicate-migration and fail-closed conflict regressions remain green.

## GREEN commands and results

Focused durable set (44 passing JSON test/subtest outcomes, zero skips/failures):

```sh
go -C pipeline test -run 'Test(SetupStreamsCreatesBoundedDurableInbox|HookAcceptedNotificationIsDurableBeforeConsumerRestart|HookDurablePublishFailureThenRetryStoresOneNotification|HookBoundsVreadAcceptanceWait|DurableInboxRestartReplaysAcceptedNotificationOnce|DurablePendingRecoveryRestartsReorderAndOverflow|CanonicalPublishBeforeInboxAckRedeliveryIsIdempotent|SubscriptionReorderOverflowRemainsDurablePending|SubscriptionIdentitySurvivesRestartAndGapFill|SubscriptionCheckpointsAreScopedByIdentity|SubscriptionEventProviderConflictFailsClosed|SubscriptionEventPayloadConflictFailsClosed|DispatchedEventConflictFailsClosed|SingleNotificationConflictingDuplicateFailsClosed|ConcurrentSubscriptionInstancesDoNotOverwritePendingEvent|ConcurrentSubscriptionInstancesDoNotDeleteNewerPendingOwner|ConcurrentSubscriptionProviderConflictFailsClosed|InboxAcknowledgementWaitsForDurablePendingStore|PendingBeforeInboxAckRestartRedeliveryIsIdempotent|GapFillReplaysDurableOverflowWithoutInbox|GapFillRejectsMismatchedSubscription|HeartbeatDoesNotTriggerPendingBacklogRescan|RecoverPendingClearsAlreadyAcknowledgedRecord|RecoverPendingRetainsDispatchedButUnacknowledgedRecord|RecoverPendingSchedulesGapFillAfterStartupStatusFailure|RecoverPendingOwnershipConflictRetriesGapFill|RecoveredCheckpointRetriesStatusAfterStartupFailure|RecoveredCheckpointRetriesAfterInvalidStatusResponse|StatusRejectsMismatchedSubscription|LegacyPendingUsesLegacyCheckpointSubscription|ScopedPendingIgnoresOtherSubscriptionLegacyKey|LegacyOverflowExactRetryDoesNotDuplicateIdentity|LegacyCheckpointDoesNotRewindScopedCheckpoint)$' -count=1 -json
```

Focused race lane:

```sh
go -C pipeline test -race -run 'Test(SetupStreamsCreatesBoundedDurableInbox|Hook|Durable|CanonicalPublish|Subscription|DispatchedEvent|SingleNotification|ConcurrentSubscription|InboxAcknowledgement|PendingBeforeInbox|GapFill|HeartbeatDoesNotTriggerPendingBacklogRescan|RecoverPending|RecoveredCheckpoint|StatusRejects|LegacyPending|ScopedPending|LegacyOverflow|LegacyCheckpoint|UnknownSubscriptionSourceDoesNotAdvanceAcknowledgement)' -count=1 -timeout=5m -json
```

Result: race JSON recorded **54 passed, 0 skipped, 0 failed**, package time 15.362s; the matching non-race JSON accounting run also had 54 expanded durable/FHIR-hook test and subtest passes and zero skips/failures. `go test -list` with the published unanchored regex selects 40 top-level tests. The explicit `ConcurrentSubscription` alternative is required; an earlier broad regex omitted that test and was not accepted as final evidence. The review's separately anchored `$` variant selected only three tests and is not the published command.

The original FHIR adversarial command and the seven-test identity/replacement/cleanup command both passed after the fixes. The five central crash-window tests also passed with `-count=3`; this was a bounded regression repetition, not a soak.

Original FHIR adversarial lane:

```sh
go -C pipeline test -run 'Test(ParseStatusValidatesRequiredFields|HookValidatesBeforeAcknowledgement|HookRejectsDuplicateNotificationEnvelopeKeys|HookRejectsInvalidInlineResourceAndFocusMismatch|HookRejectsOversizeAndBackpressure|SubscriptionRequestRejectsMalformedURL|UnknownSubscriptionSourceDoesNotAdvanceAcknowledgement|SubscriptionReorderAndDuplicateHaveSingleOrderedEffect)$' -count=1
```

Result: PASS. Existing real broker regressions `TestPresentIDsTerminatesWithDuplicateStoredIDs` and `TestJetStreamCrashRestartKeepsAcknowledgedOutputAndDedupes` also passed. `TestSetupStreamsResetsExternalRunState` passed with `FHIRIN` included in its empty-stream assertions.

Full pipeline package suite:

```sh
go -C pipeline test ./... -count=1 -timeout=10m -json
```

Result: **1,279 passed, 8 skipped, 0 failed**, package time 164.107s. Skips were `TestAPIDRFixtureRun`, `TestAPIFixturesAndContracts`, `TestAPIReviewRBACAndAppendOnly`, `TestAPIBookingSupport`, `TestFresh2Reader`, `TestHL7RoundTripCohort`, `TestNATSBackupRestoreReadsRetainedMessagesOnEmptyCluster`, and `TestNATSBackupRestorePreservesRepeatedMessageIDs`.

`go -C pipeline vet ./...` passed. `git diff --check` passed. `pipeline/RESULTS.md` was not changed because no final integrated run was performed.

## Remaining crash-matrix and evidence gaps

- Consumer replacement uses a new `SubConsumer`, new publisher/seen-ledger state, the production checkpoint loader at HWM 0, and a deliberately unacknowledged inbox delivery after canonical publish; it does not reuse the first consumer's in-memory dedupe map. The replacement must invoke its publisher once while the retained canonical stream count remains one, proving crash-window replay reaches canonical duplicate suppression rather than short-circuiting on a restored HWM. An actual FHIR consumer OS-process kill immediately after HTTP 200 was not run.
- Missing-stream and pending-store outages were injected. A broker-accepted publish with a lost/timed-out acknowledgement was not separately injected.
- Tests use one real local NATS node. Concurrent ownership fixtures use independent consumer objects in one OS process, not separate OS processes or hosts. R3 elections/partitions, multi-region replication, physical power/media faults, and external NATS ownership remain untested here.
- No post-dedupe-window time travel, multi-notification partial-batch failure matrix, or id-only/vread crash matrix was run.
- No final integrated demo/evaluation run was performed, so no new `comparison_sha256`, clinical, zero-loss, Malaffi, or production result is claimed. Backup/restore of a live pending inbox is not established by the two skipped recovery tests.
- No normalization, reviewer durability, model/eval, soak, AMD64, paid-service, or hosted-CI work is included.
