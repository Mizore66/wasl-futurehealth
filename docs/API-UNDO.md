# Undo and contest API (Workflow D)

Status: **built** (demo mode, synthetic data). Code: `pipeline/api_undo.go`, tests: `pipeline/api_undo_test.go`.

Undo and contest append compensating events to the existing review ledger `<run>/api_reviews.ndjson`. Nothing is edited or deleted. The canonical `out_log`, `Decision.State` and decision signatures are never touched. Undo status is a separate projection over the ledger.

## Endpoints

| Call | Purpose |
|---|---|
| `POST /api/cases/{id}/undo` | Undo or contest a review event, or resolve a contest |
| `GET /api/cases/{id}/undo` | Per-event status and case status, plus `canonical_state` for comparison |

Headers as for `/review`: `X-Wasl-Role`, `X-Wasl-Actor` (both required).

### Undo / contest body

```json
{"target": "<hash of a prior review event in this case>", "contest": false, "note": "..."}
```

`contest: true` records a contest instead of a compensation.

### Resolve a contest

```json
{"target": "<hash of the contest event>", "resolution": "upheld|rejected", "note": "..."}
```

`upheld` marks the original review `compensated`; `rejected` returns it to `active`.

## Rules

- `target` must be a review event of this case. Unknown and cross-case hashes both return 404.
- Authorization uses the target event's original action (`signoff_link`, `signoff_evidence`, `record_consent`) through the same RBAC matrix as `/review`.
- The actor must differ from the target's actor (compared trimmed and case-insensitive). For a resolution the actor must differ from both the contester and the original reviewer.
- A target can be undone or contested only while `active`. Undoing an undo, contest or resolution is refused (409). A second undo or contest of the same target is refused (409). A contest must be resolved before anything else happens to its target.
- A contest never closes or reopens the case. Responses carry `canonical_reopened: false`, `auto_closure: false`; contests also carry `requires_resolution: true`.

## Event

Compensating events carry: `kind` (`undo|contest|contest_resolution`), `case`, `target`, `target_actor`, `target_action`, `action`, `role`, `actor`, `note`, `head_signed` (the case's current decision signature), `action_head` (the ledger head for the case at append time), `at`, `prev`, `hash`.

## Status model

- Review event: `active`, `contested`, `compensated`.
- Contest event: `open`, `upheld`, `rejected`.
- Case status: `contested` if any review is contested, else `compensated` if any is compensated, else `active`, else `none`.

## Ledger integrity

`readVerifiedLedger` checks, line by line: single JSON object, `prev` links to the preceding hash (`genesis` first), hash equals the recomputed SHA-256 of the event, no duplicate hashes. `loadAPIVerified(run, data)` refuses a tampered ledger. Every POST re-verifies the file and requires it to agree with the in-memory ledger (length and head), otherwise it returns 503 and appends nothing.

Wiring: the handler is `s.undoRoutes(s.caseRoutes)`, which serves `/undo` and delegates every other path. `pipeline/api.go` is not modified; mounting it in `runServe` and switching `runServe` to `loadAPIVerified` is a two-line follow-up for the api.go owner.

## Limits

- Demo-mode actor identity is a header, not authentication.
- The ledger is a single global chain with one writer process; the verification catches edits, reorder, duplication and truncation, but not a full recomputed rewrite by someone who can write the file (no external anchor for it yet).
- Legacy events written by `/review` have no `kind`; they are treated as `review`.
