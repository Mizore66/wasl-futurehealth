# Durable reviewer restart evidence (2026-11-10)

**Status:** built and tested for a bounded, synthetic, single-node matrix. Branch base `8f9eb76`; code head `85697b90be4ea1aa8036dacab3cb6ee89d5001bc` (fresh-record hot path: no retained-stream scan and no ledger parse; recovery and retry-after-failure still do both, and recovery parses the ledger once).

## What was run

| Command | Result |
|---|---|
| `go -C pipeline test . -run 'TestReviewer' -count=1` | PASS, 55.959s (at 85697b9) |
| `go -C pipeline test . -count=1` | PASS, 229.055s (full package, at 85697b9) |
| `go -C pipeline vet .` | PASS, no output |

Task 4 test additions (characterization of the Task 1-3 implementation; no production code changed):

- `TestReviewerCrashMatrixMatchesUninterruptedControl`: 10 subtests = 5 crash windows (prepared; ledger appended but unrecorded; ledger_written; published but unrecorded; published but unrecorded after the FHIR dedupe window lapsed) x reuse flags off/on. After restart the whole OUT stream is redelivered and recovery is run a second time. Compared with an uninterrupted control: one retained FHIR body per `signoff/<item>`, total FHIR message count, `wl`/`lw` worklists (`reflect.DeepEqual`), full reviewer status including last OUT, SHA-256 of the ledger (wall-clock fields removed), stored sign-off, body hash and double/senior reads.
- Reuse is now actually exercised: an earlier second-identifier "same" answer (`rid-first`) feeds the id worklist, so with reuse on `rid-twin` and `rv-twin` are stored `skipped` with no sign-off, and with reuse off both are `published`. `TestReviewerRestartAdoptsSkippedTwin` crashes after a skipped twin and checks adoption on redelivery. `TestReviewerPostWindowRawRepublishIsAccepted` is the control proving a raw same-ID republish is stored again once the dedupe window lapses.
- Double reads and the senior third read are checked by the separate Task 3 test `TestReviewerRestartKeepsSeniorThirdRead`; the matrix compares whole stored sign-offs (at least two reads).
- `TestReviewerRestartMatrixLedgerFaultsThenRecovers` (prepared record, reuse off; it swaps the package-global `reviewerAppendLine`, so it and the other fault tests must not run in parallel): short write, sync failure and close failure during the recovery ledger append fail recovery with nothing published; a later restart completes with one sign-off and control-identical state.
- `TestReviewerConflictsStopWithoutAckOrPublish`: a conflicting retained FHIR sign-off, and a conflicting local ledger line, fail recovery; the OUT message is delivered (Delivered>=1, NumAckPending>0) but not acknowledged (ack floor 0), recovery error text is pinned, the ledger-conflict record stays `prepared`, nothing further is published, the record does not advance and the conflicting ledger is not modified.

Hot-path tests: `TestReviewerFreshRecordSkipsStreamScanAndLedgerParse` (0 scans, 0 ledger parses for fresh records), `TestReviewerRetryAfterFailureStillScansAndValidatesLedger`, `TestReviewerRecoveryParsesLedgerOnceAndScansPerRecord`. State order is `prepared` -> `ledger_written` -> `published`. An earlier run of this document recorded `-count=3` at 316.0s and a full package run of 275.3s on an older head; those figures are superseded by the table above and were not re-measured.

Earlier tests (Tasks 1-3) cover store transitions, ledger reconcile, retained-stream scan, in-process retry, presenter hold and the senior third read.

## Not claimed

- Exactly-once external effects: a sign-off already accepted by an external consumer is outside this test. Only the single-node retained FHIR stream is checked.
- Physical power loss or media faults: crashes are simulated by discarding the service object and faults by hermetic fakes.
- Cross-host locking or multi-node NATS.
- Clinician performance: the reviewer is synthetic.
- The failed 24-hour soak limitation is preserved unchanged; no soak, normalization or AMD64 run was made.
- No new dependency and no local WAL were added.
