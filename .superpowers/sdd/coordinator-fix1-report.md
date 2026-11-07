# Coordinator actions, review fix 1

Branch `feat/api-coordinator-fix1`, based on 2cfdd0d. Not pushed.

## Findings and changes

1. Legacy `/review` and `/actions` now share `appendEventLocked` (pipeline/api.go): hash, marshal fully, call `appendLine`, and only then update `s.reviews`. `appendToLedgerFile` checks Write (including short writes), Sync, Truncate and Close. Test: `TestLegacyReviewWriteFailureThenCoordinatorAppend` (failure gives 500, no memory change, empty file, next coordinator append returns 201, reload is clean).
2. Both handlers build `(code, body)` inside a closure that holds `s.mu`, then call `writeJSONResp` after unlocking. Test: `TestResponsesAreWrittenWithoutHoldingTheLock` uses a ResponseWriter that probes `s.mu.TryLock()` on WriteHeader/Write for 201, 409 and 500 paths on both endpoints. Mutation check: re-adding a lock around the legacy `writeJSONResp` made it fail ("s.mu held while writing the HTTP response"); reverted.
3. Error classes (`TestAppendToLedgerFileErrorClasses`): write or short-write failure with successful truncate and close is a clean error (state untouched, writes continue). Truncate failure, close failure, or sync failure wraps `errLedgerIndeterminate`; `appendEventLocked` then sets `ledgerErr`, so both write paths return 500 until restart/replay. Test `TestIndeterminateAppendMarksLedgerFailedAndRefusesWrites` covers both endpoints, including that writes stay refused after the disk "recovers" and that case detail shows `review_ledger.status=invalid`.
4. Docs (docs/API.md, "Fail-closed replay and re-materialised runs"). Case detail gains `review_ledger` {status, detail, events_applied}. Test `TestReplayFailsClosedWhenRematerialisedRunChangesHead` (changed head gives ledgerErr mentioning head, no overlay past the failing line, ledger view invalid).
5. gofmt churn reverted in api.go and governance.go (struct alignment, `sort.Slice` wrap, pickFixtures comment, closure reflow, const alignment). `git diff 2cfdd0d` on the two files contains only intended changes. `gofmt -l` still lists both files because they were not gofmt-clean before this work; that is deliberate.
6. `rejectDuplicateKeys` (nested objects and arrays) applied to action bodies and ledger replay lines; `cleanText` rejects Unicode Cf (zero-width, bidi override, BOM, soft hyphen, word joiner) for owner/text/reason/note/actor; replay `seq` must equal exactly `n+1` as a float (1.5 no longer truncates). Tests: `TestActionInputRejectsDuplicateKeysAndFormatCharacters`, `TestReplayRejectsDuplicateKeysFractionalSeqAndFormatChars`.
7. Existing coordinator tests unchanged and passing.

## Evidence

- `go vet ./pipeline` clean.
- `go test .` in pipeline: `ok wsi 131.667s` (full suite, including all earlier coordinator tests).
- Test code is in pipeline/api_actions_fix1_test.go (new file; api_actions_test.go untouched).

## Remaining concerns

- Indeterminate-failure state is cleared only by restart, which replays the file; a half-written line that survived would then fail replay as "truncated", which is the intended fail-closed outcome but needs operator action.
- `appendEventLocked` falls back to a default file appender when `appendLine` is nil, because older tests construct `apiServer` without `loadAPI`.
- Re-materialisation test simulates the changed head by changing the canonical signature in memory, not by regenerating a run; the consent-change path is covered by the existing malformed-ledger test.
- A failed replay still serves a partial overlay on reads (flagged by `review_ledger` on detail and `api_overlay` is not flagged in the list summary; healthz shows `review_ledger: invalid`).
- Legacy ledger lines are still only chain-checked, not schema-checked.
- Full-file gofmt of api.go/governance.go remains outstanding by request.
