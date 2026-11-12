# Undo and contest API (Workflow D)

Status: **wired in `runServe`** (demo mode, synthetic data). Code: `pipeline/api_undo.go`, tests: `pipeline/api_undo_test.go` and `pipeline/api_undo_fix1_test.go`.

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
- The actor must differ from the target's actor (compared trimmed and case-insensitive). For a resolution the actor must differ from both the contester and the original reviewer. Replay also verifies that `target_actor` is bound to the actual targeted review, not a value asserted by a contest.
- A target can be undone or contested only while `active`. Undoing an undo, contest or resolution is refused (409). A second undo or contest of the same target is refused (409), including a new contest of a target whose earlier contest was rejected. A contest must be resolved before anything else happens to its target.
- A contest never closes or reopens the case. Responses carry `canonical_reopened: false`, `auto_closure: false`; contests also carry `requires_resolution: true`.

## Independent-review reads

Events with `protocol: "wasl-independent-review/1"` (reads `r1`, `r2`, `r3`) cannot be undone or contested in this version: a target that is a protocol read returns 409. `type:"review_claim"` claim, renew, release and takeover events likewise return 409: lease history is append-only, and expiry does not delete or compensate it. Undoing a read would need dependent invalidation of the later reads, which is not designed. Legacy `/review` events keep the behaviour in this document.

## Event

Compensating events carry: `kind` (`undo|contest|contest_resolution`), `case`, `target`, `target_actor`, `target_action`, `action`, `role`, `actor`, `note`, `head_signed` (the case's current decision signature), `action_head` (the ledger head for the case at append time), `at`, `prev`, `hash`.

## Status model

- Review event: `active`, `contested`, `compensated`.
- Contest event: `open`, `upheld`, `rejected`.
- Case status: `contested` if any review is contested, else `compensated` if any is compensated, else `active`, else `none`.

## Ledger integrity

`readVerifiedLedger` checks, line by line: newline termination, a 16 MiB record cap, one JSON object with no trailing value, `prev` links to the preceding hash (`genesis` first), hash equals the recomputed SHA-256 of the event, and no duplicate hashes. `validateLedgerSemantics` then replays the ledger and rejects chain-valid but impossible histories: unknown kind, empty/unknown/cross-case target, forged target actor, undo or contest of a non-review, self-undo, unauthorised or empty role, resolution without an open contest, double resolution, resolver equal to contester or original reviewer, and a contest of a target whose earlier contest was rejected. Coordinator actions share the chain and are schema-validated by `loadReviews`. `loadAPIVerified(run, data)` refuses any invalid ledger.

Every POST re-verifies the file (chain and semantics) and requires it to equal the in-memory ledger hash for hash, in order; otherwise it returns 503 and appends nothing. The role is checked before the target is looked up, so an unauthorised or empty role gets the same 403 for existing, cross-case and unknown hashes.

Append is durable and atomic: the full line is written, fsynced and closed, and each error is checked, before memory is updated. On a clean rolled-back failure the response is 500 and memory is untouched. A sync, close or rollback-truncate failure marks the shared ledger indeterminate and blocks review, coordinator, consent and undo writers. Request bodies must contain one JSON object; actor and note lengths are bounded so API-created records remain replayable.

`runServe` mounts `s.undoRoutes(s.caseRoutes)` inside the consent wrapper and loads through `loadAPIVerified`.

## Web controls

**Built.** The audit and review pages offer undo, contest and resolve only to a signed-in person, and only as that person's own assigned token: the reviewer-session cookie from independent double review (PR #69), entered in a sign-in form on the page. There is no credential, role or actor picker, and the server action (`web/lib/api/governance-actions.ts`) reads the token from the HttpOnly cookie and refuses any client-supplied `credential`, `token`, `role` or `actor` field. Without a session the page shows a sign-in prompt instead of controls. The resolution has an empty placeholder and is required; nothing is upheld by default. Coordinators follow the same model: a coordinator signs in with their own token, and the API's RBAC decides what it may do. The Next.js server no longer uses server-held role credentials for these writes.

Independence still depends on separate sign-ins with distinct tokens that an operator issued to distinct people. A token proves a configured actor, not a person; one person holding several tokens can still sign out and in again as a second actor.

## Limits

- Demo-mode actor identity is a header, not authentication.
- The ledger is a single global chain with one writer process; the verification catches edits, reorder, duplication and truncation, but not a full recomputed rewrite by someone who can write the file (no external anchor for it yet).
- Legacy events written by `/review` have no `kind`; they are treated as `review`.
