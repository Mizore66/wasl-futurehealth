# Wasl web

The Wasl frontend: the product shell (`/app`), the site (`/`, `/how-it-works`, `/experts`) and the patient phones (`/patient/[step]`). Next.js 16 App Router, TypeScript strict, Tailwind v4, shadcn/ui. All data is synthetic.

The plan, routes and gaps are in [`../docs/FRONTEND-PLAN.md`](../docs/FRONTEND-PLAN.md). The backend contract is [`../docs/API.md`](../docs/API.md).

## Run against the fixtures (no backend)

With `WASL_API_URL` unset, every API read comes from `data/api/`, a copy of `docs/api-samples/`.

```bash
cd web
pnpm install
pnpm gen        # copy the API samples, build data/replay.json, results.json, people.json
pnpm dev        # http://localhost:3000/app
```

Fixtures mode serves the three demo cases and a 25-case page. `postReview` and `postAction` require `WASL_API_URL` and throw in fixtures mode because fixtures cannot append ledger events; Today keeps its call log in the browser instead.

## Run against the live API

```bash
scripts/demo.sh            # once, from the repo root: materialise a run (~5 min)
scripts/demo.sh serve      # API on http://127.0.0.1:8088
cd web
WASL_API_URL=http://127.0.0.1:8088 \
WASL_API_REVIEWER_TOKEN=wasl-demo-reviewer-1-token \
WASL_API_WEB_REVIEWER_TOKEN=wasl-demo-web-reviewer-token \
WASL_API_AUDITOR_TOKEN=wasl-demo-auditor-token \
WASL_API_COORDINATOR_TOKEN=wasl-demo-coordinator-token \
WASL_API_CLINICIAN_TOKEN=wasl-demo-clinician-token \
pnpm dev
```

The tokens are read only by the Next.js server, never exposed as `NEXT_PUBLIC_*` values. The API binds each token to its configured role/actor. Use the same development values printed above only for the loopback synthetic demo. Generic shared reads (dashboard, lists, case pages) use `WASL_API_WEB_REVIEWER_TOKEN`, which must be bound to an actor that is not one of the assigned readers, so a concealed first read is never rendered under an initial reader's identity. Today's coordinator actions (`postAction`) use `WASL_API_COORDINATOR_TOKEN`. Live undo, contest and contest resolution (audit and review pages) act only as the signed-in credential: there is no credential picker, the server action refuses a client-supplied credential, and without a session the page shows a sign-in prompt instead of controls. A resolution must be chosen; none is preselected. The API refuses a resolver who is the contester or the original reviewer, so a contest and its resolution need separate sign-ins with distinct configured actors. For a hosted frontend (Vercel) with fresh per-role tokens, see [`../docs/DEPLOY.md`](../docs/DEPLOY.md#hosted-frontend-vercel). In the local demo, credentials 1 and 2 are ordinary r1/r2 readers and credential 3 is the senior (`senior_reviewer` / `senior_clinician`) who alone can resolve a disagreement as r3. The protocol is documented in [`../docs/API.md`](../docs/API.md#post-review-independent-double-review-wasl-independent-review1).

### Reviewer sign-in modes

`WASL_REVIEW_AUTH_MODE=demo` is the default and needs no OIDC configuration. The reviewer enters an assigned token; a server action verifies it and keeps it in an HttpOnly, Secure, SameSite=Strict cookie scoped to `/app`. This remains the reproducible synthetic demo flow. A token identifies a configured demo actor, not a separate person.

`WASL_REVIEW_AUTH_MODE=oidc` disables token-entry actions and shows an OIDC sign-in link instead. Configure the confidential client and an exact, server-only subject map:

```bash
WASL_REVIEW_AUTH_MODE=oidc \
WASL_OIDC_ISSUER=https://id.example.test \
WASL_OIDC_CLIENT_ID=wasl-review \
WASL_OIDC_CLIENT_SECRET=server-only-secret \
WASL_OIDC_REDIRECT_URI=https://wasl.example.test/app/auth/oidc/callback \
WASL_OIDC_SUBJECT_MAP_JSON='{"provider-subject-r1":"reviewer","provider-subject-r2":"reviewer_2","provider-subject-senior":"reviewer_3"}' \
WASL_OIDC_SESSION_DB=/private/var/lib/wasl/review-sessions.sqlite \
WASL_API_REVIEWER_TOKEN=server-only-r1-token \
WASL_API_REVIEWER_2_TOKEN=server-only-r2-token \
WASL_API_REVIEWER_3_TOKEN=server-only-senior-token \
pnpm dev
```

The issuer and callback must use HTTPS. Discovery must report the exact issuer, authorization/token endpoints, a JWKS URI and RS256 support. The callback must be registered exactly with the provider. Allowed mapping values are `reviewer`, `reviewer_2`, `reviewer_3`, `clinician`, `clinician_2` and `clinician_3`; each may be assigned once. Shared `web_reviewer`, `auditor` and `coordinator` credentials cannot be selected. Only verified `(iss, sub)` claims select a mapping: browser parameters and ID-token role/group/actor claims never do.

The OIDC cookie contains only a random session ID, not an API credential or provider token. Login state is consumed once after ten minutes at most; sessions have a fixed eight-hour expiry and can be revoked by logout. Logout is local and does not claim to end the provider's global IdP session. OIDC verifies a provider subject and maps it to a configured actor; it does not itself prove separate real-world people.

The SQLite store is for local verification and **single-instance production-shaped operation only**. Keep its file server-only with restricted permissions. Multi-instance deployment requires a shared `SessionStore` implementation and separate verification; no production-readiness or live-identity-provider claim is made here.

For loopback synthetic testing only, set `WASL_OIDC_INSECURE_LOOPBACK=1` and `WASL_SYNTHETIC_DATA_ONLY=1`, and use loopback issuer/callback URLs. `tests/support/synthetic-oidc-provider.ts` generates RS256 keys, discovery, JWKS, single-use codes and PKCE checks on an ephemeral port. The focused Vitest files and `e2e/review-oidc.spec.ts` start it automatically; no external identity provider is contacted.

Fixture mode needs no credential and persists nothing. To run the end-to-end suite against the live API, build and test with `WASL_API_URL` and the required role tokens set:

```bash
WASL_API_URL=http://127.0.0.1:8088 pnpm verify
```

## Verify

```bash
pnpm verify     # tsc → eslint → vitest → next build → playwright
```

- `tests/` (Vitest): every file in `docs/api-samples` parses, also rewritten as a τ 0.99 run; domain functions; the replay loop; the numbers pinned in `data/results.json`.
- `e2e/` (Playwright): every route returns 200, renders one `h1` and a visible honesty label, and has no serious axe violations. Screenshots go to `test-results/screens/`.
- To test one spec against your own dev server, start `pnpm exec next dev -p <port>` and run `PLAYWRIGHT_BASE_URL=http://localhost:<port> pnpm exec playwright test e2e/<spec>.ts`.

## Layout

| Path | What | Who edits |
|---|---|---|
| `lib/api/schema.ts` | zod schemas for every endpoint; objects are loose, enums are closed | shared, read-only in phase 2 |
| `lib/api/source.ts` | `getDashboard`, `getCases`, `getCase`, `getAudit`, `getFixtures`, `postReview`, `postAction` | shared |
| `lib/domain/` | `dayToDate`, `formatDay`, `caseStage` (the 8-step vocabulary), case id helpers | shared |
| `lib/replay/` | `step(state, event)`, `next(replay, state)`, `initialState` | shared |
| `lib/results/`, `lib/people/` | typed access to `data/results.json` and `data/people.json` | shared |
| `components/wasl/` | `StatusMark`, `SealedChain`, `HonestyLabel`, `Stat` | shared |
| `components/shell/` | the app sidebar and the phase 1 stub page | shared |
| `components/ui/` | shadcn/ui: button, tabs, dialog, tooltip, toggle-group, separator | shared |
| `app/<route>/` | one folder per route; each owner replaces its stub | the route's owner |

## Generated data

`pnpm gen` rebuilds everything under `data/` from files in the repo. Do not edit the outputs by hand.

| Output | Built from | Label |
|---|---|---|
| `data/api/*.json` | `docs/api-samples/` | simulated |
| `data/replay.json` | `pipeline/results/showcase/{summary.txt,reader_ledger.jsonl,report7.json,tamper.json}` | replay |
| `data/results.json` | `pipeline/results/final/SUMMARY.md`, the `docs/BRIEF.md` evidence table, `docs/PITCH.md`, the dashboard sample | simulated |
| `data/people.json` | the design's canonical cast; `data/persons3.json` has no registration mapping | simulated |

`results.json` records each number with its source file. Conflicting sources go in `conflicts`, not in `entries`. Read numbers with `result(id)`, which throws on an unknown id.
