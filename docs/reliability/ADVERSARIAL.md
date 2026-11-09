# Adversarial robustness evidence

## Scope

This document records **built controls and executed tests**, not production readiness. The tests use synthetic fixtures, hermetic filesystem faults, real handler/parser entrypoints, and a real single-node NATS server. Physical disk failure, power loss, multi-node partitions, and authenticated transport boundaries are separate unresolved work.

The machine-readable source of truth is [`adversarial-matrix.json`](adversarial-matrix.json). Baseline: `91cff71626e38a509395bef3c031d1d1f82a2773`.

The [verification manifest](evidence/verification-2026-11-09.manifest.json) binds the final raw logs, artifact hashes, commands, and matrix SHA-256 to `tested_source.code_commit`, the final code-only commit `3e1c3733ec44525baf95cc169b22aad189a51fde`. `tested_source.code_tree` is that commit's full Git tree. Every listed test command ran with no working-tree changes under `pipeline/` or `scripts/`; later evidence-only commits change only `docs/reliability/`.

`tested_source.tree` has explicit source-only semantics: it is the deterministic Git tree of the final evidence revision with the complete `docs/reliability/evidence/` directory removed. It covers the final tracked source and reliability documentation, including this document and the matrix, without attempting a self-referential hash of evidence or of the containing commit. Reproduce it from the final evidence revision with a temporary index:

```sh
tmp_index=$(mktemp)
GIT_INDEX_FILE="$tmp_index" git read-tree HEAD
GIT_INDEX_FILE="$tmp_index" git rm -r --cached docs/reliability/evidence
GIT_INDEX_FILE="$tmp_index" git write-tree
rm -f "$tmp_index"
```

The output must equal `tested_source.tree`. Independently, `git diff --exit-code 3e1c3733ec44525baf95cc169b22aad189a51fde..HEAD -- pipeline scripts` must be empty. That command is the executable check that the evidence revision did not alter the canonical runtime or test/fuzz scripts after the recorded test run.

## Durable FHIR acknowledgement follow-up

Proposal #1 is implemented and tested separately from the historical manifest above. See [the durable acknowledgement/replay evidence report](evidence/fhir-durable-ack-replay-2026-11-10.md) for the updated base, tested code revision, RED/GREEN commands, invariants, and remaining crash-matrix limits. The follow-up uses a bounded JetStream work-queue inbox and durable canonical subscription/event pending records with verified provider identity in each value. Separate instances use CAS ownership and revision-checked deletion; startup recovery schedules gaps even when the initial status poll fails. It does not establish external exactly-once delivery, Malaffi behavior, multi-region safety, or production readiness.

## Built controls

- FHIR and embedded HL7 resources reject duplicate JSON keys and malformed nested shapes without escaping a panic as an accepted event.
- HL7 rejects duplicate singleton segments, out-of-order MSH, missing sender/control IDs, and malformed embedded Patients. MLLP has a 64 KiB preamble limit and a 1 MiB frame limit.
- SubscriptionStatus validates required fields, numeric values, timestamps, and focus grammar. Notification envelopes reject duplicate JSON keys before acknowledgement. Hooks validate before acknowledgement and return 503 instead of blocking on a full inbox. Unknown sources do not advance the acknowledgement checkpoint. The follow-up makes HTTP 200 contingent on durable FHIRIN acceptance and retains reorder overflow as canonical subscription/event FHIRSUB pending records with verified provider identity and CAS ownership.
- Poll/export decoding checks bundle and manifest errors, required resource metadata, vreads, and NDJSON scanner errors. Invalid data blocks successful completion and EOF.
- Review, consent, and undo share strict bounded JSON decoding: exact and case-variant duplicate keys, unknown fields, trailing data, and valid prefixes beyond the byte limit are rejected.
- Independent API writers serialize through a Unix file lock and revalidate the on-disk chain under that lock before append.
- Reader replay rejects malformed, truncated, stale-model, schema-invalid, and conflicting ledger records. Review and reader ledger records are limited to 16 MiB during fragmented reads, before an oversized record can be accumulated. A live answer enters the cache only after its durable append succeeds.
- Recovery scanning counts fetched stream records rather than unique message IDs, so duplicate or empty IDs cannot make the termination condition impossible.
- Five native Go fuzz targets cover FHIR normalization, HL7/resource conversion, MLLP framing, SubscriptionStatus, and strict API body decoding.

## Executed environment

| Item | Recorded value |
|---|---|
| OS | macOS 27.2, Darwin 27.2.0, arm64 |
| Go | `go1.27.3 darwin/arm64` |
| NATS | `nats-server v2.15.2` at `/opt/homebrew/bin/nats-server` |
| NATS wiring | Temporary worktree-local `bin/nats-server` symlink to the system binary |
| Fuzz smoke | Four workers, three seconds per target |
| Filesystem faults | In-process write, short-write, sync, close, rollback, and missing-parent injection |

To reproduce the real-NATS command when `nats-server` is already installed:

```sh
mkdir -p bin
ln -sf "$(command -v nats-server)" bin/nats-server
go -C pipeline test -run 'Test(PresentIDsTerminatesWithDuplicateStoredIDs|JetStreamCrashRestartKeepsAcknowledgedOutputAndDedupes|SeenLedgerDropsOldReplays|SetupStreamsResetsExternalRunState|ShadowOverNATSDuplicatesAndRestart|ShadowOverNATSKilledMidRunDoesNotRestartItself)$' -count=1 -v
```

The tests bind local ephemeral ports except the pre-existing seen-ledger test, which uses port 14899. A missing binary causes some pre-existing tests to skip; use verbose output and do not count `SKIP` as a pass.

The final full run explicitly skipped six asset-dependent tests: `TestAPIBookingSupport`, `TestAPIDRFixtureRun`, `TestAPIFixturesAndContracts`, `TestAPIReviewRBACAndAppendOnly`, `TestFresh2Reader`, and `TestHL7RoundTripCohort`. The cohort round trip therefore was not executed in this worktree; the deterministic HL7 adversarial and framing tests did run.

## Reliability matrix

| Area | Result | Reproduction and executed evidence |
|---|---|---|
| Malformed/duplicate FHIR | **Pass** | `go -C pipeline test -run '^(TestNormaliseRejectsShapeConfusionWithoutPanics|FuzzNormaliseNeverPanics)$' -count=1 -v`. Both the deterministic test and fuzz seed corpus execute explicit duplicate outer `resourceType` and nested sign-off fixtures. Null/scalar members, missing arrays, malformed sign-offs, and duplicate outer/nested keys are rejected without panics; generated fuzzing is recorded separately below. |
| Malformed/duplicate/out-of-order HL7 | **Pass** | `go -C pipeline test -run 'Test(HL7RejectsMalformedAndOutOfOrderSegments|HL7ToResourceRejectsShapeConfusionWithoutPanics|HL7ToResourceRejectsResourceNormaliseDrops|MLLPBoundsPreambleAndFrame)$' -count=1`. Covers duplicate MSH/ZWS, MSH after PID, null Patients with or without PID, bad trailers, oversized preamble/frame, and unterminated input. `ToResource` refuses embedded resources that `normalise` would drop. |
| Subscription validation and reorder | **Pass** | `go -C pipeline test -run 'Test(ParseStatusValidatesRequiredFields|HookValidatesBeforeAcknowledgement|HookRejectsDuplicateNotificationEnvelopeKeys|HookRejectsInvalidInlineResourceAndFocusMismatch|HookRejectsOversizeAndBackpressure|SubscriptionRequestRejectsMalformedURL|UnknownSubscriptionSourceDoesNotAdvanceAcknowledgement|SubscriptionReorderAndDuplicateHaveSingleOrderedEffect)$' -count=1`. Duplicate envelope keys receive 400 without queueing. Events 2, 1, 1 produce ordered effects 1, 2 with one duplicate dropped. |
| Durable FHIR acknowledgement and replay | **Pass, synthetic single node** | Forty-four focused durable JSON test/subtest outcomes and 54 expanded durable/FHIR-hook outcomes pass with real NATS; the corrected focused race lane includes the concurrent-owner regression. HTTP 200 requires durable inbox acceptance; pending storage precedes inbox acknowledgement; provider/subscription/event identity survives restart and gap-fill; stale owners cannot delete claimed replay state; startup status failure does not stall recovered gaps. [Commands and limits](evidence/fhir-durable-ack-replay-2026-11-10.md). |
| Poll and bulk export | **Pass** | `go -C pipeline test -run 'Test(PollRejectsMalformedResponsesWithoutCompletion|ExportRejectsMalformedManifestAndTruncatedNDJSON|ExportRejectsNullAndNonObjectManifestsWithoutNATSCompletion)$' -count=1 -v`. Malformed bundles/manifests/resources and a 16 MiB + 1 record do not complete the feed. Real-NATS cases reject JSON null, array, string, number, boolean, empty-object, and null-output manifests with no export success, checkpoint, resource, EOF state, or FHIR stream message. |
| Strict API bodies | **Pass** | `go -C pipeline test -run 'TestAPIMutationsRejectDuplicateUnknownAndOversizedBodies' -count=1`. Review, consent, and undo reject exact/case-variant duplicates, unknown fields, and valid-prefix bodies beyond their limits. |
| Native fuzz smoke | **Pass** | `scripts/adversarial_fuzz.sh`. All five targets generated beyond baseline and passed. The runner rejects baseline-only/zero-generation passes. [Raw logs](evidence/adversarial-fuzz-2026-11-09/) and exact counts are bound to the tested source in the verification manifest. Counts are host/cache dependent. |
| Longer fuzz | **Not run** | Reproducible command: `FUZZTIME=60s scripts/adversarial_fuzz.sh`. Only the bounded three-second-per-target variant was executed. |
| Concurrent writers | **Pass** | `go -C pipeline test -race -run 'Test(IndependentServersCannotForkReviewLedger|ActionsSameHeadConcurrencyExactlyOneWins|ActionsIndependentCasesConcurrent)$' -count=1`. Two independent servers cannot both fork the same head; 16 same-head requests have one winner. |
| Corrupt/truncated ledgers | **Pass** | `go -C pipeline test -run 'Test(ReadLedgerRecordStopsAtLimitWithoutReadingWholeInput|LedgerReadersRejectOversizedRecordsDuringBoundedReads|ReviewLedgerRejectsDuplicateKeysAndInvalidBaseEvents|ReaderLedgerFailsClosedOnCorruptionAndStaleModel|ActionsMalformedLedgerFailsClosed|UndoRejectsTruncatedOrDeletedLedger|UndoAppendFailureLeavesNoTraceAndReloadRejectsTornLine)$' -count=1`. Unsafe replay and subsequent writes fail closed. Terminated and unterminated records over 16 MiB are rejected during bounded reads. |
| Disk-full/partial-write injection | **Pass, hermetic only** | `go -C pipeline test -race -run 'Test(AppendToLedgerFileErrorClasses|ActionsAppendFailureLeavesStateUntouched|IndeterminateAppendMarksLedgerFailedAndRefusesWrites|UndoIndeterminateAppendBlocksEveryWriter|ReaderDoesNotCacheAnswerBeforeDurableAppend)$' -count=1`. Failed appends do not produce cached/acknowledged success; indeterminate state blocks writers. |
| NATS duplicates and restart | **Pass, single node** | Command above. Duplicate-ID recovery scan terminates; an acknowledged message survives abrupt broker restart; replay does not create another stored effect; existing duplicate/restart tests pass. |
| Full regression and vet | **Pass with six asset-dependent skips** | Fresh `go -C pipeline test ./... -count=1 -timeout=10m -json`, focused race run, and `go -C pipeline vet ./...` passed with real NATS enabled. [Raw JSON events](evidence/full-regression-2026-11-09.jsonl); exact counts, skips, timing, commands, log hashes, matrix hash, and tested commit/tree are in the verification manifest. |
| Reviewer crash/restart | **Not run** | No crash-after-publish or post-dedupe-window reviewer restart matrix was added. Reviewer progress is not durable under all such restarts. |
| Physical OS/media faults | **Not run** | Requires an isolated block device or fault-injection filesystem and a power-failure harness. No physical ENOSPC, torn-media, machine-power, R3 quorum, or network-partition claim is made. |

## Seeds and reproducibility

Deterministic regressions use checked-in table fixtures and fixed message IDs. The reader request uses model seed `1`. Native fuzz targets use checked-in seed corpora in `pipeline/adversarial_fuzz_test.go` plus the Go fuzz engine's persistent coverage cache.

The Go tool does not expose a numeric fuzz-engine seed flag. Therefore the duration and safety invariants are reproducible, but the exact generated sequence and execution counts are not promised. The fuzz runner retains each target's raw log and fails unless Go reports entry into generation and executions beyond the completed baseline. Any generated failure artifact must be promoted to a deterministic test before closing its finding.

## Unresolved findings and environment limits

1. **Transport and actor authentication are not built here.** MLLP remains unauthenticated, EOF is not separately authorized, and sign-off roles/read attestations remain payload assertions. Keep MLLP on an isolated integration subnet; this is not an authentication substitute.
2. **MLLP slow-client resistance is incomplete.** Frame/preamble memory is bounded, but connection deadlines, connection caps, and a network slowloris matrix remain outstanding.
3. **Clinical semantics are incomplete.** Day-0 timestamp defaults, unrecognized-program defaults, and non-final report-status gating remain safety concerns even when JSON shape validation passes.
4. **Historical hook acknowledgement finding addressed by the follow-up.** The original implementation queued in memory before responding and discarded reorder overflow. The durable acknowledgement/replay follow-up adds a bounded JetStream inbox and durable pending/replay state; its synthetic single-node limits and untested crash windows are listed in the follow-up report.
5. **Reviewer restart behavior remains unresolved.** Local reviewer progress, post-window sign-off replay, reviewer ledger I/O checking, and reviewer status synchronization were not fixed by this change.
6. **Other filesystem paths need propagation work.** `writeStatus`, materialization, and stream dumps still have unchecked I/O errors. Ledger append tests do not establish their durability.
7. **Malformed NATS payloads lack a durable dead-letter contract.** Buffered FACTS sequence provenance also needs correction during reorder drains.
8. **External NATS ownership is global.** Fresh setup destructively resets streams; active-run namespacing or single-active-run ownership is not implemented.
9. **Lock portability is limited.** The ledger lock uses Unix `flock`. Independent server values in one process were tested; Windows, cross-host, and network-filesystem behavior were not.
10. **Reader invalid-ledger policy reduces availability deliberately.** A malformed or conflicting ledger disables replay/live caching until an operator remedies it rather than silently accepting a subset.

No unresolved item above should be read as covered by the passing rows.
