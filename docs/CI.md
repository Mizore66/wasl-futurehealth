# Continuous integration

`.github/workflows/ci.yml` runs on every pull request, including updates to an existing pull request. It has no path filters. Every job has a deadline, third-party actions are pinned to full commit SHAs, `scripts/check_action_pins.py` verifies with GitHub that each SHA is a commit object rather than an annotated tag object, and the workflow token has only `contents: read`.

## Required branch-protection check

Require **`CI required`** in the repository's branch-protection rules. This aggregate job runs with `if: always()` and calls `scripts/ci_required.py` with GitHub's complete `needs` context. It succeeds only when every lane below reports `success`; `failure`, `cancelled`, `skipped`, a missing result, or malformed result data is not green. Do not mark a lane optional or use `continue-on-error` to bypass this contract.

| Job | Required behavior |
| --- | --- |
| Workflow lint | Run pinned `actionlint` and verify that every Action SHA resolves to a commit object. |
| Go tests | Verify modules, run `go vet`, install NATS 2.12.6, run all Go tests without `-short`, and account for every skip. |
| Go race tests | Run all Go tests with `-race` and an explicit 60-minute Go test timeout, and account for every skip. |
| Python tests | Run the scripts, fresh-evaluation, encoder, and all six booking unit-test lanes with LightGBM installed and the Go booking solver built. |
| Integrated demo and materialized Go tests | Run `scripts/demo.sh all`, including its real HTTP smoke, then rerun Go tests against generated outputs and require the named API/RBAC/booking/HL7 tests to pass without hidden subtest skips. |
| Booking pool simulation reproducibility | Run all 30-draw pool simulation lanes and require byte-identical tracked output. |
| Frontend lint typecheck Vitest and build | Check generated-data drift, ESLint, TypeScript, Vitest, and the Next.js production build independently. |
| Playwright all projects | Install Chromium with OS dependencies and run every Chromium project, including serial landing and performance coverage. |
| Go dependency vulnerabilities | Run pinned `govulncheck`. |
| Python dependency vulnerabilities | Audit `requirements-ci.txt` with pinned `pip-audit`. |
| npm dependency vulnerabilities | Audit both production and development dependencies with `pnpm audit`. |
| Filesystem dependency and configuration vulnerabilities | Scan lockfiles and deployment configuration with Trivy; fixable HIGH/CRITICAL findings fail. |
| TruffleHog pull request secrets | Scan the pull-request commit range, including verified, unverified, and unknown results. |
| Clean container build and API smoke | Build both images without cache, scan them, start the isolated Compose deployment, verify pipeline success and materialized health, run the real API smoke, and verify the API and all NATS nodes run as UID 10001 with writable data volumes. |

Dependency Review requires the repository's dependency graph and GitHub's Dependency Review API. This repository does not support that API, so the original brief's optional Dependency Review lane is not in the required workflow. The supported Go, Python, npm, and Trivy scanners above remain fail-closed; enabling Dependency Review later can add an advisory lane without replacing them. Container smoke is functional verification, not production security sign-off.

## Go test skip accounting

All three Go lanes emit `go test -json` JSONL and run `scripts/check_go_test_skips.py` with `if: always()`. The exact-name policy is `scripts/ci_test_policy.json`: ordinary/race lanes may skip materialized-output tests because only integration generates those outputs; integration must instead record passes for `TestAPIFixturesAndContracts`, `TestAPIReviewRBACAndAppendOnly`, `TestAPIBookingSupport`, and `TestHL7RoundTripCohort`. A skip in any required test or subtest fails. Package-level failures/skips, unexpected skips, malformed JSON and missing required passes also fail.

The only other allowed skips are the optional `TestFresh2Reader`, `TestAPIDRFixtureRun`, the two external-target NATS recovery drills, and `TestSeedReplayFixture` (a seeder helper for `scripts/test_recovery_replay.py`, enabled only by `WASL_REPLAY_SEED_URL`). Review `scripts/ci_test_policy.json` on every test rename; do not enable held-out evaluation data or external recovery targets merely to remove a skip. Each lane uploads its raw Go JSON and a `wasl-go-test-skip-manifest/1` manifest containing only test names, counts, required status and errors. `if-no-files-found: error` prevents an evidence upload from passing by omission. A hosted result is claimed only from GitHub, never from this local documentation.

For a local NATS version check, set `WASL_TEST_NATS_SERVER=/absolute/path/to/nats-server`. An explicitly configured missing or non-starting binary fails the test rather than silently skipping it.

## Version and cache policy

- Go comes from `pipeline/go.mod`; its module cache is keyed by `pipeline/go.sum`. Go 1.27.4 is the minimum because GO-2026-6859 affects the Go 1.27.3 standard library; `govulncheck` 1.9.0 carries a Go-1.27-compatible SSA implementation.
- Python is 3.12.16. Direct test and scanner dependencies are pinned in `requirements-ci.txt`; the pip cache is keyed by that file.
- Node is pinned in `web/.nvmrc`; pnpm is pinned by `web/package.json` and repeated explicitly in the workflow. The pnpm cache is keyed by `web/pnpm-lock.yaml`, and installs always use `--frozen-lockfile`.
- NATS, actionlint, govulncheck, Trivy, TruffleHog, and pip are pinned to explicit versions in the workflow.
- Trivy uses its date-keyed vulnerability-database cache. Browser installation does not use an unversioned system Chromium.
- Update an action's full SHA and its version comment together. Do not replace SHA pins with tags, annotated tag object SHAs, or `main`; run `python3 scripts/check_action_pins.py` to verify object types.

## Local equivalents

Run commands from the repository root unless a block says otherwise. Local success does not mark the GitHub check successful; branch protection still uses the GitHub result.

### Workflow gate and Go

```bash
python3 -m pip install -r requirements-ci.txt
python3 -m unittest discover -v -s scripts -p 'test_ci_contract.py'
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.10 -color .github/workflows/ci.yml
python3 scripts/check_action_pins.py

mkdir -p bin
GOBIN="$PWD/bin" go install github.com/nats-io/nats-server/v2@v2.12.6
(
  cd pipeline
  go mod download
  go mod verify
  go vet .
  go test -json -count=1 -timeout=20m . | tee /tmp/wasl-go-test.jsonl
  go test -json -race -count=1 -timeout=60m . | tee /tmp/wasl-go-race.jsonl
)
python3 scripts/check_go_test_skips.py --input /tmp/wasl-go-test.jsonl --lane go-test --output /tmp/wasl-go-test-manifest.json
python3 scripts/check_go_test_skips.py --input /tmp/wasl-go-race.jsonl --lane go-race --output /tmp/wasl-go-race-manifest.json
```

Do not run the ordinary and race suites concurrently in the same network namespace: some integration tests bind fixed ports.

### Python

Use Python 3.12.16 and an isolated environment:

```bash
python3.12 -m venv .venv-ci
source .venv-ci/bin/activate
python3 -m pip install --upgrade 'pip==26.2.2'
python3 -m pip install -r requirements-ci.txt
python3 -m pip check
python3 -c 'import lightgbm, numpy, scipy, sklearn, yaml'
(
  cd pipeline
  mkdir -p ../bin
  go build -o ../bin/wsi7 .
)
python3 -m unittest discover -v -s scripts -p 'test_*.py'
python3 -m unittest discover -v -s scripts/fresh2_eval -p 'test_*.py'
python3 -m unittest discover -v -s encoder -p 'test_*.py'
python3 booking/tests/test_engine.py
python3 booking/tests/test_levers.py
python3 booking/tests/test_lookahead.py
python3 booking/tests/test_lookahead_eval.py
python3 booking/tests/test_pool_golden.py
python3 booking/tests/test_suggest_default.py
python3 scripts/check_deploy.py
python3 booking/sim/pool_test.py
git diff --exit-code
```

The fresh-evaluation suites inspect Git ancestry. Use a full clone, not a shallow checkout. `booking/sim/pool_test.py` is deliberately a separate reproducibility simulation, not part of unittest discovery; its tracked `pool_test.json` must remain unchanged. Probabilities are rounded to 12 decimal places and assignment costs are solved as integer nanodays so low-bit libm/SIMD differences cannot change scheduling. The same golden covers verified Darwin/ARM64, Linux/x86-64, and Linux/ARM64 environments; unsupported platforms fail explicitly.

### Integrated demo

With the Python environment and `bin/nats-server` above available:

```bash
scripts/demo.sh all
(
  cd pipeline
  go test -json -count=1 -timeout=20m . | tee /tmp/wasl-integration.jsonl
)
python3 scripts/check_go_test_skips.py --input /tmp/wasl-integration.jsonl --lane integration --output /tmp/wasl-integration-manifest.json
git diff --exit-code
```

This uses the cached reader, not live Gemma. Live model calls, frozen multi-seed evaluation, and the 24-hour soak are separate workflows/processes, not pull-request unit tests.

### Frontend and Playwright

```bash
cd web
nvm use
corepack enable
corepack pnpm install --frozen-lockfile
corepack pnpm gen
git -C .. diff --exit-code
corepack pnpm exec next typegen
corepack pnpm lint
corepack pnpm typecheck
corepack pnpm test
corepack pnpm build
corepack pnpm exec playwright install --with-deps chromium
CI=1 WASL_API_URL= REVIEW_LIVE_MOCK= corepack pnpm exec playwright test
```

`CI=1` forbids focused Playwright tests and enables GitHub, list, and never-opening HTML reporters. Failure traces, screenshots, test artifacts, and the HTML report are retained for seven days in GitHub. The performance project remains required and runs serially after the functional projects; do not hide a performance failure with a project filter.

### Security scanners

```bash
(
  cd pipeline
  go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
)
python3 -m pip_audit -r requirements-ci.txt
(
  cd web
  corepack pnpm audit --audit-level=high
)
trivy fs --scanners vuln,misconfig --severity HIGH,CRITICAL \
  --ignore-unfixed --skip-dirs .git,web/node_modules --exit-code 1 .
```

Use Trivy v0.69.3. For the TruffleHog equivalent, set `BASE_SHA` and `HEAD_SHA` to the pull-request endpoints and use the pinned image:

```bash
docker run --rm -v "$PWD:/tmp" -w /tmp \
  ghcr.io/trufflesecurity/trufflehog:3.92.5 \
  git file:///tmp --since-commit "$BASE_SHA" --branch "$HEAD_SHA" \
  --fail --no-update --results=verified,unverified,unknown
```

GitHub Dependency Review has no exact offline equivalent and is optional only where the repository supports its API. The three language audits and Trivy are the required local vulnerability checks. TruffleHog release tags use a `v` prefix, but its published container tags deliberately do not; `3.92.5` is the verified multi-architecture image for release `v3.92.5`.

### Containers

Use a unique Compose project and never commit or upload the key or rendered Compose configuration:

```bash
export COMPOSE_PROJECT_NAME="wasl-ci-local-$$"
export WASL_API_PORT=18088
export WASL_JS_KEY="k$(openssl rand -hex 32)"
trap 'docker compose down --volumes --remove-orphans' EXIT
python3 scripts/check_deploy.py
docker compose config --quiet
docker build --no-cache --tag wasl:local .
docker build --no-cache --tag wasl-nats:local deploy/nats
trivy image --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1 wasl:local
trivy image --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1 wasl-nats:local
docker compose up --detach --no-build
```

Wait for the `pipeline` container to exit 0 and for `GET /healthz` to report `status=ok`, `dependencies.nats=up`, `dependencies.materialised_run=loaded`, and at least one case. The workflow implements a bounded poll and fails immediately on a nonzero pipeline exit. Then run:

```bash
python3 scripts/api_smoke.py --base-url "http://127.0.0.1:${WASL_API_PORT}"
test "$(docker compose exec -T api id -u)" = 10001
for node in nats-0 nats-1 nats-2; do
  docker compose exec -T "$node" sh -c 'test "$(id -u)" = 10001 && test -w /data'
done
git diff --exit-code
docker compose down --volumes --remove-orphans
trap - EXIT
```
