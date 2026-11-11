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
WASL_API_AUDITOR_TOKEN=wasl-demo-auditor-token \
WASL_API_COORDINATOR_TOKEN=wasl-demo-coordinator-token \
WASL_API_CLINICIAN_TOKEN=wasl-demo-clinician-token \
pnpm dev
```

The tokens are read only by the Next.js server, never exposed as `NEXT_PUBLIC_*` values. The API binds each token to its configured role/actor; the browser's selected actor label is not identity evidence. Use the same development values printed above only for the loopback synthetic demo. Today's coordinator actions (`postAction`) use `WASL_API_COORDINATOR_TOKEN`. For a hosted frontend (Vercel) with fresh per-role tokens, see [`../docs/DEPLOY.md`](../docs/DEPLOY.md#hosted-frontend-vercel). To run the end-to-end suite against the live API, build and test with `WASL_API_URL` and the required role tokens set:

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
