# Undo / contest report (Workflow D)

Branch `feat/api-undo-contest`, based on 425c091. Files: `pipeline/api_undo.go`, `pipeline/api_undo_test.go`, `docs/API-UNDO.md`.

## Method
TDD: tests written first; first run failed to compile (missing `undoRoutes`, `validateReviewLedger`, `loadAPIVerified`), then implemented until green.

## Evidence
- `go vet` clean; `gofmt` clean on the new files.
- `go test -run 'Undo|Contest|LoadAPI'`: all pass (10 tests, 8 tamper subtests).
- Cases covered: happy-path compensation with preserved events and unchanged canonical state/signature; missing/unknown/cross-case target; missing actor; same actor (incl. case/space variant); unauthorized role by target's original action; duplicate undo; undo of undo; contest, self-contest, second contest, plain undo while contested; resolution rules (different actor from contester and original reviewer, value required, must target a contest, no duplicate); rejected contest restores active; tamper (field edit, reorder, duplicate, removed middle, garbage line, stripped hash, prev rewrite, forged event, truncated tail, deleted file); reload projection; other paths delegated.
- Canonical `out_log` untouched (asserted empty/unchanged), `Decision.State`, `Sig`, `decSig` unchanged.

## Concerns
1. Not wired into `runServe`: I was told to own only the three new files, so `api.go` is unchanged. The handler is `s.undoRoutes(s.caseRoutes)` and loading should use `loadAPIVerified`. Until wired, the endpoint is reachable only in tests.
2. The existing `/review` handler in api.go is not hardened: it computes hashes with the same `canon` function (compatible with my verification) but does not itself verify the file before appending. Tamper is detected on load and on every undo POST, not on `/review`.
3. Hash chain gives tamper evidence, not tamper proof: someone able to rewrite the whole file with recomputed hashes passes, unless the file head is anchored externally. Divergence from the served in-memory ledger is detected while the process runs.
4. Actor identity is a request header (demo mode); "different actor" is only as strong as that.
5. `loadAPIVerified` returns an error rather than panicking; `loadAPI` itself still panics on missing logs (pre-existing).
6. Legacy `/review` events have no `kind` and are treated as `review`; a `/review` event for the same case does not require the prior review to be undone first (no semantic coupling to the sign-off flow).
7. Full `go test -count=1 ./pipeline`: ok (135s).
