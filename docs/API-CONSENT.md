# Consent event actions API (Workflow C, demo overlay)

Status: **wired in `runServe`**. Demo mode, synthetic data. Nothing here sends a message or changes canonical consent.

## Endpoints

`POST /api/cases/{id}/consent`

Headers: `X-Wasl-Role` (`coordinator` or `clinician`; any other role gets 403 via `Authorize(role, ActRecordConsent)`) and `X-Wasl-Actor` (required, 400 if missing).

```json
{"event": "consent_granted | consent_denied | consent_opted_out", "expected_head": "<case current head signature>", "note": "optional, max 500 characters"}
```

| Status | Meaning |
|---|---|
| 201 | Event appended to the review ledger; response has `review` (the ledger event) and `overlay` |
| 400 | Bad JSON, trailing data, unknown event, missing/overlong actor, missing `expected_head`, or note over 500 characters |
| 403 | Role may not record consent |
| 404 | Unknown case |
| 409 | `expected_head` does not equal the case's current head signature (response carries `current_head`), or a grant after an opt-out |
| 500 | The shared ledger is invalid, its state is indeterminate, or the durable append failed; nothing is recorded in memory |
| 405 | Method other than GET or POST |

`GET /api/cases/{id}/consent` returns `{case, overlay, history}`. `GET /api/cases/{id}` also gains `api_contact: {overlay, history}`.

## Ledger and overlay

- Each event uses the same fail-closed durable append path as `/review` and `/actions`, with `action: record_consent`, `decision: <event>`, and `head_signed` preserved as the case's current head signature. A stale head, invalid ledger or failed append never updates the in-memory overlay.
- The `api_contact` overlay is **derived from that ledger** on each read, so a server restart rebuilds it exactly. The ledger is written first.
- Overlay fields: `status` (`none|granted|denied|opted_out`), `demo_pending_consent` (`""|released|cancelled`; a demo-only view of what the hold would do, distinct from the canonical `case.consent`), `events`, `contact_blocked`, `mode: "demo"`, `canonical_consent`, `canonical_unchanged: true`, `last_event_hash`.
- Grant derives demo hold to `released`. Deny and opt-out derive `cancelled` and set `contact_blocked`. A later grant after a denial is allowed; after an opt-out it is refused (409).
- While `contact_blocked`, `POST /review` with `decision: confirm_contact` returns 409 and writes nothing.

## What does not change

`case.consent`, the decision's `outreach` and `consent_log`, `out_log.ndjson` and every canonical run file stay as the pipeline wrote them. `DispatchC` is never called. In a live deployment these events would be published as signed consent records and replayed through the linker; that path is **designed**, not built.

Detail (`GET /api/cases/{id}`) keeps the upstream JSON bytes and splices `api_contact` in, so large integers, ids and timestamps are not re-encoded. Escaped case ids (`%2F`) are unescaped exactly as the case router does.

## Wiring

`runServe` composes `s.withConsent(s.undoRoutes(s.caseRoutes))` and mounts the export handler on the same `/api/cases/` mux. The consent wrapper handles `/consent`, guards `/review`, adds the detail overlay and delegates all other paths.
