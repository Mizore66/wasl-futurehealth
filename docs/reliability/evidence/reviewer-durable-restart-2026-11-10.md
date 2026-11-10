# Durable reviewer restart evidence (2026-11-10)

**Status:** built and tested for a bounded, synthetic, single-node matrix. Base `18262425febc36a50cfbfa07ac9935ff77c94a38`.

## What was run

| Command | Result |
|---|---|
| `go -C pipeline test . -run 'TestReviewer' -count=3` | PASS, 305.6s (three consecutive passes of every reviewer test) |
| `go -C pipeline test . -count=1` | PASS, 271.2s (full package) |
| `go -C pipeline vet .` | PASS, no output |

Task 4 test additions (characterization of the Task 1-3 implementation; no production code changed):

- `TestReviewerCrashMatrixMatchesUninterruptedControl`: 10 subtests = 5 crash windows (prepared; ledger appended but unrecorded; ledger_written; published but unrecorded; published but unrecorded after the FHIR dedupe window lapsed) x reuse flags off/on. After restart the whole OUT stream is redelivered and recovery is run a second time. Compared with an uninterrupted control: one retained FHIR body per `signoff/<item>`, total FHIR message count, `wl`/`lw` worklists (`reflect.DeepEqual`), full reviewer status including last OUT, SHA-256 of the ledger (wall-clock fields removed), stored sign-off, body hash and double/senior reads.
- `TestReviewerRestartMatrixLedgerFaultsThenRecovers`: short write, sync failure and close failure during the recovery ledger append fail recovery with nothing published; a later restart completes with one sign-off and control-identical state.
- `TestReviewerConflictsStopWithoutAckOrPublish`: a conflicting retained FHIR sign-off, and a conflicting local ledger line, fail recovery; the OUT message is not acknowledged (ack floor 0), nothing further is published, the record does not advance and the conflicting ledger is not modified.

Earlier tests (Tasks 1-3) cover store transitions, ledger reconcile, retained-stream scan, in-process retry, presenter hold and the senior third read.

## Not claimed

- Exactly-once external effects: a sign-off already accepted by an external consumer is outside this test. Only the single-node retained FHIR stream is checked.
- Physical power loss or media faults: crashes are simulated by discarding the service object and faults by hermetic fakes.
- Cross-host locking or multi-node NATS.
- Clinician performance: the reviewer is synthetic.
- The failed 24-hour soak limitation is preserved unchanged; no soak, normalization or AMD64 run was made.
- No new dependency and no local WAL were added.
