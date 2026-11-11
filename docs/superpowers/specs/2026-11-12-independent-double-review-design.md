# Independent double review and senior resolution design

GitHub issue: #41. Status: approved for autonomous implementation by the user on 2026-11-12.

## Goal

Add an interactive, authenticated r1/r2/r3 workflow without changing canonical pipeline decisions or rewriting review history. Two initial reviewers must have distinct token-bound actors. Their initial answers remain blinded until both are submitted. A disagreement alone opens resolution by a distinct, authorised senior actor.

This is a synthetic demo workflow. Tokens establish configured actor identities, not proof that separate real people or clinicians participated.

## Existing behaviour to preserve

- The simulated `ReviewerCfg.Review` pipeline remains unchanged.
- Legacy `POST /review` requests without `read` remain append-only ledger events. They never advance independent-review state and remain labelled as not independently double-reviewed.
- Existing consent, coordinator and undo/contest events retain their meaning and hash-chain order.
- Canonical case state, `out_log.ndjson`, `fhir_log.ndjson` and `reviewer_decisions.ndjson` are not modified by interactive review.
- Fixture mode remains a non-persisting preview.
- Auditor history remains actor- and note-redacted.

## Protocol

Extend `POST /api/cases/{id}/review` with optional fields:

```json
{
  "item": "rv-123",
  "decision": "same",
  "note": "",
  "read": "r1",
  "expected_head": ""
}
```

`read` is a workflow slot, not an RBAC role. A request containing it uses protocol `wasl-independent-review/1`; the server derives `question` (`link` or `evidence`) from the raised item and decision. Combined-kind items have independent `(item, question)` state machines so reviewer and clinician authority never collapse into one answer.

`expected_head` must be present for protocol requests. It is the last protocol event hash for the question, or empty for the initial read. Transition validation and append occur under the existing server mutex. A failed append leaves memory unchanged.

Persist additive event fields:

```text
protocol, read, question, expected_head, resolution_reads
```

`resolution_reads` on r3 contains the r1 and r2 event hashes. Existing event `role`, `actor`, `decision`, `note`, `prev` and `hash` fields remain. No new ledger `type` or `kind` is introduced.

### States

| State | Accepted write | Result |
|---|---|---|
| `awaiting_r1` | Appropriate ordinary reviewer/clinician submits r1 at the initial head | `awaiting_r2` |
| `awaiting_r2` | A different actor submits r2 at the current head | `resolved_agreed` if answers match, otherwise `awaiting_r3` |
| `awaiting_r3` | A distinct authorised senior submits r3 linked to both initial reads | `resolved_senior` |
| Any terminal state | No further reads | 409 |

Items already having a canonical final sign-off are ineligible for new protocol writes. Legacy interactive events do not advance the machine.

Use 400 for malformed input, 403 for RBAC/same-actor/non-senior refusal, 404 for unknown case/item and 409 for wrong state, duplicate slot, premature r3 or stale head. Actor comparison reuses trimmed, case-insensitive normalisation; distinct tokens bound to one actor are not independent.

### Senior authority

Add `senior_reviewer` and `senior_clinician` roles with the read/sign capabilities of their disciplines. Only ordinary reviewer/clinician roles may submit r1/r2; only matching senior roles may submit r3. The third demo reviewer/clinician credential becomes senior. Basic-auth recovery validation and deployment bootstrap recognise the same roles.

No caller-supplied identity header, browser-selected role or actor label grants authority.

## Projection and blinding

Case detail and protocol POST responses add `api_review_states`. Each `(item, question)` entry contains:

```text
item, question, state, head, next_read, may_submit, reads,
disagreement, final_decision, resolution_basis, canonical_unchanged
```

Each read entry identifies its slot and submission status. Visible resolved entries include decision, event hash and read slot; actor and note are not needed in this projection.

Before r2 submits:

- r1's decision and note are omitted for every other actor.
- The other actor's r1 event is omitted from viewer-facing `api_reviews`; its hidden hash is also omitted from `review_states`.
- A viewer sees their own submitted answer, or only “first read submitted” when viewing as the next actor.
- POST responses follow the same rule.

After r2, both initial answers become visible. Agreement exposes no r3 action. Disagreement exposes both answers and gates r3 on both state and current senior authority.

The reducer is pure and rebuilt from the ledger. Startup semantic replay and live writes use the same transition validator so chain-valid but impossible histories fail closed. Audit/export allowlists preserve protocol, slot, question, head and resolution links while omitting actor and note.

Undo/contest targeting protocol reads returns 409 in this version. Dependent-invalidation semantics are explicitly out of scope; legacy compensation remains supported.

## Authenticated web reviewer

The current shared server-side role tokens cannot establish the person using the site. Live review therefore uses a reviewer-supplied assigned API token as the web credential:

1. The reviewer enters their token in a sign-in form.
2. A Next.js server action verifies it by making an authenticated API request.
3. On success, the token is stored in an HttpOnly, Secure, SameSite=Strict, review-path-scoped cookie.
4. Review GET and POST requests use that exact credential. The browser never supplies actor or role headers and the server does not map a selected slot to a shared token.
5. Sign-out clears the cookie. Fixture mode does not request credentials or persist writes.

This is credential authentication for the synthetic demo, not OIDC or human identity assurance. Deployment must use HTTPS. Existing non-review pages retain their current server-to-API credentials.

The review page uses the viewer credential for detail reads. It suppresses the embedded auditor history while any initial answer is concealed, preventing an auditor-token side channel. General case/audit pages retain existing access controls.

## Frontend workflow

- Replace the case-wide answer pair with drafts keyed by `(item, question, read)` and submit one read at a time.
- Show per-question status: first read due, second read due, disagreement/senior due, agreed or senior-resolved.
- After r1, retain the item in the queue and do not mark the case settled or the identity link trusted.
- After r2, reveal both initial answers. Only a disagreement with an authorised senior viewer shows the resolution form.
- Persist and render r1, r2 and r3 as separate entries keyed by event hash. Do not call an initial read a final sign-off.
- Keep automated report-reader evidence separate from interactive human reads.
- Keep legacy simulated sign-offs as historical summaries; never invent individual answers from their `reads` ID lists.
- Preserve programme-specific labels, non-CRC/evidence-only coverage, keyboard focus and accessibility.
- Do not reuse the governance credential selector for r3.

Queue and case-stage projections treat only `resolved_agreed` or `resolved_senior` as completed protocol overlays. Canonical `case.review` remains unchanged, so the UI must apply this overlay rather than rewriting canonical data.

## Parallel ownership

1. **Backend protocol:** new review protocol reducer/projection files; API, auth, ledger replay, audit/export and undo integration; focused Go tests.
2. **Frontend credential/contracts:** schemas, API source, reviewer cookie sign-in/out and one-read server actions; focused unit tests.
3. **Frontend workflow:** review view/desk/queue/case/audit/identity projections and accessibility-preserving UI; focused unit/browser tests.
4. **Verification/operations:** real-HTTP smoke, deployment/bootstrap credentials, API/security/deployment documentation and CI-facing checks.

Contracts in this document are the coordination boundary. The coordinator resolves integration mismatches and runs whole-branch review.

## Verification

Required automated evidence:

- Distinct-actor agreement and disagreement flows.
- Same-actor r2 and r3 refusal, including two tokens bound to one normalised actor.
- Premature/non-senior r3 refusal and no r3 after agreement.
- Discipline-specific RBAC for link and evidence questions.
- Full-response canary tests proving r1 decision/note are absent for r2 before submission.
- Concurrent duplicate/stale reads yield one append and one 409.
- Restart replay reproduces state; impossible/tampered histories fail closed.
- Mixed legacy/protocol ledgers remain valid; legacy events never advance protocol state.
- Append failure leaves state unchanged.
- Auditor/export history preserves all slots and resolution links without actor/note leakage.
- Canonical output hashes remain unchanged.
- Browser reload persistence, blinding, disagreement, senior resolution, keyboard focus and axe checks.
- Real-HTTP smoke through the authenticated handler in integrated and container CI.

## Non-goals and limits

- No model training, threshold changes, new programmes or canonical-state migration.
- No live Malaffi, clinical pilot, professional identity proof or real clinician participation claim.
- No hard deletion, WORM/storage-signature claim or external proof of human independence.
- No undo/contest of protocol reads until dependent invalidation is designed.
- No production OIDC provider or new paid service.
