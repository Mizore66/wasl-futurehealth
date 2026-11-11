# Independent Double Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build authenticated, blinded r1/r2 review with disagreement-gated senior r3 resolution across the Go API and Next.js review desk.

**Architecture:** A pure ledger-derived `(item, question)` state machine extends the existing review route without changing canonical decisions. The web app authenticates each reviewer with their assigned API token in an HttpOnly cookie and uses that same credential for detail reads and writes. Four disjoint implementation lanes coordinate through the frozen spec contracts.

**Tech Stack:** Go 1.27.4, standard-library HTTP/JSON, Next.js 16.4.3, React 19, TypeScript, Zod, Vitest, Playwright, Python API smoke.

**Spec:** `docs/superpowers/specs/2026-11-12-independent-double-review-design.md`

## Global Constraints

- Use synthetic fixtures only; no real patient data, outreach or clinical-performance claim.
- Role and actor come only from verified bearer/Basic credentials. Never trust caller identity headers or a browser role/actor selector.
- Legacy requests without `read` remain ledger-only and never advance independent-review state.
- Never alter canonical `out_log.ndjson`, `fhir_log.ndjson`, `reviewer_decisions.ndjson`, case decisions or simulated reviewer behaviour.
- Never rewrite/delete ledger events. Undo/contest of protocol reads returns 409.
- Blinding applies to serialized API responses, detail events, POST responses and the review page's embedded audit history.
- `read` values are exactly `r1`, `r2`, `r3`; states are exactly `awaiting_r1`, `awaiting_r2`, `awaiting_r3`, `resolved_agreed`, `resolved_senior`.
- Protocol marker is exactly `wasl-independent-review/1`; state key is `(item, question)` where question is `link` or `evidence`.
- Auditor/export responses never add actor or note.
- Fixture mode does not persist reviews. The UI has no r1/r2/r3 credential dropdown.
- Do not modify deploy workflows. Existing CI workflow files remain untouched.
- Every commit ends with `Co-Authored-By: Claude Code <noreply@anthropic.com>`.

---

### Task 1: Backend protocol, authentication and projections

**Files:**
- Create: `pipeline/review_protocol.go`, `pipeline/review_protocol_test.go`, `pipeline/api_independent_review_test.go`
- Modify: `pipeline/governance.go`, `pipeline/api.go`, `pipeline/api_undo.go`, `pipeline/api_export.go`, `pipeline/recovery.go`
- Test: `pipeline/api_ledger_contract_test.go`, `pipeline/api_audit_contract_test.go`

**Interfaces:**
- Consumes: existing `apiPrincipal`, `ReviewItem`, `apiServer.reviews`, `appendEventLocked`, `decodeAPIBody`, `Authorize`, `normActor` and redacted export projections.
- Produces: additive request/event fields `read`, `expected_head`, `protocol`, `question`, `resolution_reads`; additive case/POST field `api_review_states` / `review_state`; roles `senior_reviewer` and `senior_clinician`.

- [ ] **Step 1: Write failing reducer and HTTP tests**

Create table-driven tests whose transition expectations are:

```go
func TestReviewProtocolTransitions(t *testing.T) {
    tests := []struct{ name, first, second string; want string }{
        {"agreement", "same", "same", "resolved_agreed"},
        {"disagreement", "same", "different", "awaiting_r3"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Use the real authenticated handler and synthetic r1/r2 principals.
            // POST r1 and r2 with the heads returned by the preceding response.
            // Decode review_state and require tt.want.
        })
    }
}
```

Add separate real-handler tests for same normalized actor across two tokens (403), premature r3 (409), ordinary r3 (403), initial actor r3 (403), valid senior r3 (201), discipline mismatch (403), stale head/duplicate slot (409), terminal writes (409), and legacy request bypass (legacy 201 followed by state still `awaiting_r1`). Before r2, marshal the entire r2 detail response and require it not to contain planted r1 decision/note/hash canaries. Run two concurrent requests at one head and require exactly one 201 and one 409.

Run: `go -C pipeline test . -run 'ReviewProtocol|IndependentReview' -count=1`
Expected: FAIL because protocol fields, transitions and senior roles do not exist.

- [ ] **Step 2: Implement the pure reducer and transition validator**

Create typed internal representations:

```go
type reviewQuestionKey struct { Item, Question string }
type reviewProtocolRead struct {
    Read, Decision, Actor, Role, Hash, Note string
}
type reviewProtocolState struct {
    Item, Question, State, Head string
    Reads []reviewProtocolRead
}

func applyReviewProtocol(states map[reviewQuestionKey]*reviewProtocolState, ev map[string]interface{}) error
func projectReviewStates(states map[reviewQuestionKey]*reviewProtocolState, viewer apiPrincipal) []map[string]interface{}
```

`applyReviewProtocol` ignores events without `protocol`; with the protocol marker it requires known fields, valid hashes/links, correct expected head, distinct normalized actors and the exact state table. r3 requires `senior_reviewer` for link or `senior_clinician` for evidence, differs from both initial actors and references both initial hashes. Terminal states accept no descendant. Sort projections by item then question. Projection omits another actor's initial decision/note/hash while the question awaits r2.

- [ ] **Step 3: Add senior RBAC and request handling**

Add to `governance.go`:

```go
RoleSeniorReviewer  = "senior_reviewer"
RoleSeniorClinician = "senior_clinician"
```

Grant senior reviewers `ActSignLink` and `ActReadQueue`; grant senior clinicians `ActSignEvidence` and `ActReadQueue`. Accept these roles in recovery Basic validation. Extend the review request with `Read string` and `ExpectedHead *string`. With `Read == ""`, execute the existing legacy path unchanged. Otherwise require non-nil `ExpectedHead`, a raised unsigned item, the question implied by item kind plus programme-aware decision, matching ordinary/senior role and the current per-question head. Persist the verified role/actor and protocol metadata through `appendEventLocked`; return `review` plus `review_state`.

- [ ] **Step 4: Add viewer-aware detail, replay and audit/export**

Change detail construction to receive the verified principal and add `api_review_states`. Filter concealed r1 entries from `api_reviews` for other actors while awaiting r2. Preserve unfiltered auditor-only audit history, but redact actor/note as before and allow only `protocol`, `read`, `question`, `expected_head`, `resolution_reads` as new metadata. Call the same semantic reducer from both ledger replay paths. Reject undo/contest targeting protocol events with 409. Do not introduce a new ledger `type` or `kind`.

Run: `go -C pipeline test . -run 'ReviewProtocol|IndependentReview|Ledger|Audit|Undo|Recovery|Security' -count=1`
Expected: PASS.

- [ ] **Step 5: Run the complete backend suite and commit**

Run: `go -C pipeline test ./... -count=1 && go -C pipeline vet ./...`
Expected: PASS with no canonical fixture changes.

Commit only Task 1 files:

```bash
git commit -m "feat: enforce independent double review" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 2: Web reviewer credential and typed one-read action

**Files:**
- Create: `web/lib/api/reviewer-session.ts`, `web/components/review/auth.ts`, `web/tests/reviewer-session.test.ts`
- Modify: `web/lib/api/schema.ts`, `web/lib/api/source.ts`, `web/app/app/review/[id]/actions.ts`
- Test: `web/tests/review-actions.test.ts`, `web/tests/schema.test.ts`, `web/tests/source-auth.test.ts`

**Interfaces:**
- Consumes: Task 1 JSON contracts; no Go source edits.
- Produces: `getReviewCase(caseId: string): Promise<{detail: CaseDetail; authenticated: true} | {detail: null; authenticated: false}>`, `signInReviewer(caseId: string, token: string)`, `signOutReviewer(caseId: string)`, `submitReviewRead(caseId: string, input: ReviewReadInput)`, and schema exports `ReviewRead`, `ReviewState`, `ReviewRequest`, `ReviewResponse`.

- [ ] **Step 1: Write failing schema, credential and action tests**

Pin these TypeScript contracts:

```ts
export const ReviewRead = z.looseObject({
  read: z.enum(["r1", "r2", "r3"]),
  submitted: z.boolean(),
  decision: z.string().optional(),
  hash: z.string().optional(),
});
export const ReviewState = z.looseObject({
  item: z.string(), question: z.enum(["link", "evidence"]),
  state: z.enum(["awaiting_r1", "awaiting_r2", "awaiting_r3", "resolved_agreed", "resolved_senior"]),
  head: z.string(), next_read: z.enum(["r1", "r2", "r3"]).nullable(),
  may_submit: z.boolean(), reads: z.array(ReviewRead), disagreement: z.boolean(),
  final_decision: z.string().optional(), resolution_basis: z.string().optional(), canonical_unchanged: z.literal(true),
});
export type ReviewReadInput = { item: string; question: "link" | "evidence"; decision: string; expectedHead: string };
```

Tests must prove sign-in rejects malformed tokens, a valid token is never rendered/logged, GET and POST use the same raw bearer credential, no `X-Wasl-Role`/`X-Wasl-Actor` header is sent, one action appends exactly one read, and 400/403/404/409 are returned without a success claim. Legacy `postReview` remains for legacy API clients but the desk action never calls it.

Run: `npm test -- --run tests/reviewer-session.test.ts tests/review-actions.test.ts tests/source-auth.test.ts tests/schema.test.ts`
Expected: FAIL because session and read-state contracts are absent.

- [ ] **Step 2: Implement the HttpOnly reviewer session**

`reviewer-session.ts` is server-only. Use cookie name `wasl_review_token`, path `/app/review`, `httpOnly: true`, `secure: true`, `sameSite: "strict"`, and eight-hour max age. Validate tokens with `^\S{16,4096}$`. `signInReviewer` verifies the token by fetching that case with it before setting the cookie; failures return a generic refusal. `signOutReviewer` deletes the cookie and revalidates the case. Never put the token in React props, URLs, local storage or logs.

- [ ] **Step 3: Implement typed API and one-read server action**

Add raw-token fetch helpers beside existing role-token helpers without weakening them. `getReviewCase` uses the session credential; unauthenticated live review returns a sign-in-required state, not the shared r1 credential. `submitReviewRead` derives allowed labels from the server-provided question/item and posts:

```ts
{ item: input.item, decision: input.decision, note: "", read: state.next_read, expected_head: input.expectedHead }
```

The server action reads the latest state with the same credential, requires `may_submit`, and rejects a mismatched `input.question`. On 201 it revalidates review, patient and queue pages. On 409 it revalidates and returns current server truth; it never disables RBAC or retries a stale write automatically.

Run: `npm run typecheck && npm test -- --run tests/reviewer-session.test.ts tests/review-actions.test.ts tests/source-auth.test.ts tests/schema.test.ts`
Expected: PASS.

- [ ] **Step 4: Commit Task 2**

```bash
git commit -m "feat: authenticate independent reviewers" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 3: Blinded review desk and resolved overlays

**Files:**
- Create: `web/components/review/reviewer-login.tsx`, `web/tests/review-protocol.test.ts`, `web/e2e/review-independent.spec.ts`
- Modify: `web/components/review/decision.ts`, `case-view.ts`, `queue.ts`, `review-desk.tsx`, `web/app/app/review/[id]/page.tsx`, `web/app/app/review/page.tsx`, `web/app/app/patients/[id]/page.tsx`, `web/components/case/sections.tsx`, `audit-view.tsx`, `web/lib/people/identity.ts`, `web/lib/domain/stage.ts`
- Test: `web/tests/review.test.ts`, `review-queue.test.ts`, `identity.test.ts`, `ledger-contract.test.ts`, `audit-contract.test.ts`, `domain.test.ts`

**Interfaces:**
- Consumes: Task 2 `ReviewState`, `ReviewReadInput`, sign-in/out and one-read actions.
- Produces: per-question review UI and overlay helpers; no API route or schema changes.

- [ ] **Step 1: Write failing UI/projection tests**

Cover all five states. Assert r1 alone leaves queue/item open and link trust held; while awaiting r2 the other answer is absent from markup; after agreement both answers appear with no senior form; after disagreement both appear and only an authorised senior sees r3; terminal reads render separately by hash. Include combined-kind items as two questions, non-CRC evidence labels, reload persistence and fixture preview. Browser test signs in distinct synthetic r1/r2/senior credentials rather than selecting roles.

Run: `npm test -- --run tests/review-protocol.test.ts tests/review.test.ts tests/review-queue.test.ts tests/identity.test.ts tests/domain.test.ts`
Expected: FAIL because current UI submits one signer pair and treats any review event as final.

- [ ] **Step 2: Implement server-driven question states and sign-in**

Build one card per `(item, question)`. Draft key is `${item}:${question}:${next_read}`. Submit exactly one `ReviewReadInput`; after success clear only that draft and render revalidated server state. Unauthenticated live viewers get the token sign-in form and sign-out control. Do not display credential choices. Hide the page's `HistoryCard` whenever any question conceals an initial read; show neutral status text instead of hidden data.

- [ ] **Step 3: Implement queue/case/audit overlays**

Queue completion requires every eligible question to be `resolved_agreed` or `resolved_senior`. Only those states produce a final overlay decision. `caseStage` and `linkTrust` ignore non-terminal protocol reads and preserve legacy behaviour. Case/audit rows label `First read`, `Second read`, `Senior resolution`; r1/r2 are not final sign-offs. Preserve simulated sign-off summaries and actor/note redaction. Keep undo/contest controls only on eligible legacy events.

- [ ] **Step 4: Verify UI and accessibility**

Run: `npm run typecheck && npm run lint && npm test && npm run build`
Expected: PASS. Run the focused Playwright scenario against its stateful synthetic mock with serious/critical axe checks and reload after every read; no concealed answer may appear in page content or embedded history.

- [ ] **Step 5: Commit Task 3**

```bash
git commit -m "feat: add blinded double-review desk" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 4: Live smoke, bootstrap and documentation

**Files:**
- Modify: `scripts/demo.sh`, `scripts/production_secrets.sh`, `scripts/api_smoke.py`, `scripts/test_api_smoke.py`, `scripts/test_production_deploy.py`
- Modify: `docs/API.md`, `docs/SECURITY.md`, `docs/DEPLOY.md`, `docs/API-UNDO.md`, `docs/API-EXPORT.md`, `web/README.md`
- Create: `scripts/test_independent_review_smoke.py` for focused stateful-fake protocol tests

**Interfaces:**
- Consumes: Task 1 roles/protocol and Task 2 assigned-token sign-in. No CI workflow or deploy workflow changes.
- Produces: repeatable real-HTTP proof and truthful operator instructions.

- [ ] **Step 1: Write failing smoke/bootstrap tests**

The stateful fake must require two normalized-distinct initial actors, blind another actor before r2, require senior role and a third actor for r3, preserve resolution links and reject stale heads. Bootstrap tests require demo reviewer/clinician credentials 1 and 2 to remain ordinary and credential 3 to become `senior_reviewer`/`senior_clinician`. Production secrets must generate distinct r1/r2/senior credentials without printing them in documentation.

Run: `python3 -m unittest scripts.test_api_smoke scripts.test_production_deploy`
Expected: FAIL on senior bindings and unimplemented independent protocol.

- [ ] **Step 2: Implement real-HTTP independent review smoke**

Add `independent_review_smoke` beside the legacy workflow. Discover a raised synthetic item, submit r1, assert same-actor refusal and blinded r2 detail, submit disagreement, reject ordinary/premature r3, submit authorised senior r3, reload, verify ordered slots/hashes/redaction and unchanged canonical hashes. Also run agreement on a second item and reject r3. Keep this separate from legacy blank-item undo/contest fixtures.

- [ ] **Step 3: Update bootstrap and truthful docs**

Bind third demo credentials to senior roles. Document the exact state table, status codes, token sign-in, cookie scope, blinding, mixed-ledger behaviour, 409 undo/contest boundary, and synthetic-demo limits. Explicitly state that tokens do not prove separate humans, the ledger is not WORM, canonical state remains unchanged and no production OIDC is built.

Run: `python3 -m unittest scripts.test_api_smoke scripts.test_production_deploy && python3 scripts/notices.py check`
Expected: PASS.

- [ ] **Step 4: Commit Task 4**

```bash
git commit -m "test: verify independent review end to end" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 5: Integration review and merge gate

**Files:** coordinator integration fixes only; no new feature scope.

- [ ] **Step 1: Run all local gates on the integrated tree**

```bash
go -C pipeline test ./... -count=1
go -C pipeline vet ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
npm run typecheck && npm run lint && npm test && npm run build
```

Expected: all PASS. Run focused real-handler and browser protocol scenarios and record exact counts.

- [ ] **Step 2: Dispatch independent whole-branch review**

Review the spec and full diff for actor bypass, token leakage, blinding leaks, invalid replay, canonical mutation, legacy regressions and accessibility. Fix every Critical/Important finding and rerun the affected complete gate.

- [ ] **Step 3: Push, create PR and wait for fresh CI**

PR must close #41 and end with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`. Merge only when the PR is mergeable and every relevant fresh check is green. If main advances, resolve conflicts preserving all three prior review/governance/consent features and wait for a new green run.
