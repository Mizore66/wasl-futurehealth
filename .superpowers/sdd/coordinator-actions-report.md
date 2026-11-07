# Coordinator actions (Workflow A) report

Branch feat/api-coordinator-actions, base 425c091. Owned files: pipeline/api.go, pipeline/governance.go, pipeline/api_actions_test.go, docs/API.md.

## Evidence
- `go vet .` clean. `gofmt -l` clean on the changed Go files.
- Focused: `go test -run TestActions -count=1 -race .` -> ok. 11 tests: happy path all kinds, validation (23 bad bodies plus actor checks), RBAC and routing (403/404/405), stale head 409, 16-way same-head race (1x201 + 15x409), independent cases both 201, contact outcome consent 409 and no dispatch, append failure 500 with state untouched and retry, reload replay, malformed-ledger fail-closed (13 mutations + truncated + denied-consent), legacy review events sharing the chain, canonical immutability.
- Full: `go test -count=1 ./...` -> ok wsi 134.8s.
- TDD note: tests and implementation were written in one pass, not red-first per test. The first runs did fail (null-scan helper, a nil snooze in JSON, name clash) and were fixed.

## Design
- New permission ActCoordinateCase, granted to coordinator only (sign, consent and dispatch permissions unchanged).
- Events go to the shared api_reviews.ndjson chain (type coordinator_action, kind, data, head, per-case seq). Overlay lives in apiCase.ov and is exposed as api_overlay (summary and detail). Canonical fields, out_log and signatures are untouched; `current` carries no overlay.
- Case head = last decision Sig until the first coordinator event, then the latest event hash. Head check, append+fsync and memory update all happen under s.mu, so same-head races have one winner.
- Append failure: 500, any partial line truncated, memory unchanged.
- loadReviews strictly replays: chain, hash, case, role, head, seq, canonical data, consent, trailing newline. First failure sets ledgerErr: /review and /actions return 500, healthz shows review_ledger=invalid, reads still work.

## Concerns
1. Legacy /review events are now chain-verified on replay. A hand-edited or differently hashed api_reviews.ndjson puts the server in ledger-invalid mode. Events written by the current code verify (tested).
2. /review now also returns 500 when the ledger is invalid; it still has no rollback on write failure (pre-existing).
3. gofmt realigned a few pre-existing lines in api.go and governance.go, so the diff has some unrelated whitespace churn.
4. Event hash is over Go's sorted-key JSON; other-language verifiers must canonicalise the same way.
5. Four existing API tests skip here (no materialised run at ../out/run), so the api_overlay field in case summaries was verified only on synthetic runs. List responses take the lock once per case.
6. Snooze is a record only; it does not hide cases or alter filters. Overlay owner does not feed the `owner` filter.
7. Consent is checked against the last decision at request time; a later consent change does not invalidate recorded outcomes. One global lock serialises writes across cases (no false conflicts, but they queue).
