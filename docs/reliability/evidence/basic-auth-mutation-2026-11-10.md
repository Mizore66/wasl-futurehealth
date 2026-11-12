# Basic-auth/CORS focused mutation review

## Context and scope

- Base / integration evidence commit: `e73abc9be315cb6f82acea41dac0c166b3929c2c`.
- Implementation under review: `46f3cff79867f037c93425c4c890119a73f32418`.
- Final context: one test/report commit on local branch `security/basic-auth-mutation-review`, with the base above as its parent. No merge or push was performed.
- Production behavior was not changed. The intended diff is limited to `pipeline/recovery_test.go` and this report.
- Every HTTP case exercises the production-shaped in-process chain `auth(basicAPIHandler(s))`, where `auth` comes from `apiAuthFromEnvironment` with a real temporary users file. Runs use synthetic materialized cases only.

## Test coverage added or strengthened

`TestBasicAuthEnforcesReadRBACAndPreflight` pins auditor case-detail denial, reviewer/coordinator export denial, and auditor audit/export success. Its unauthenticated export preflight now also sends `Access-Control-Request-Method: GET`, uses an explicitly empty request body, and requires an empty 204 response body.

`TestBasicAuthCORSRejectsUntrustedOriginAndSkipsCaseHandlersOnPreflight` adds:

- unauthenticated trusted preflights to both case detail and case export, each with `Access-Control-Request-Method: GET`, an explicitly empty request body, exact trusted `Access-Control-Allow-Origin`, 204 status, and an empty response body (a downstream case handler would return 405 with a JSON body);
- authenticated GET and unauthenticated OPTIONS requests from `https://evil.example`, requiring 403, no `Access-Control-Allow-Origin`, and no case ID disclosure.

No existing assertion was removed or relaxed.

## Commands and clean-tree results

All commands were run in this worktree. `-count=1` disabled Go test caching. The worktree's absolute local path was replaced with the repo-relative `pipeline` on 2026-11-12; the commands are otherwise as run.

```sh
go -C pipeline test . -run '^(TestBasicAuthEnforcesReadRBACAndPreflight|TestAPIAuthenticationDerivesRoleAndActor|TestAPIAuthenticationRejectsMissingAndWrongCredentials)$' -count=1 -v
```

Baseline before the added CORS cases: `PASS`, `ok wsi 1.122s`; all three selected tests and all seven RBAC subtests passed.

```sh
go -C pipeline test . -run '^TestBasicAuth' -count=1 -v
```

Strengthened baseline: `PASS`, `ok wsi 3.879s`; both Basic-auth tests and every subtest passed.

```sh
go -C pipeline test . -run '^TestBasicAuth' -count=1
```

Final focused run after restoring every production mutant: `PASS`, `ok wsi 0.453s`.

```sh
go -C pipeline test . -count=1
```

Broader package runs: `PASS`, `ok wsi 140.552s`; fresh pre-commit rerun `PASS`, `ok wsi 150.212s`.

## Real temporary production mutations

Each mutant was applied alone, tested with the exact command below, and restored before the next mutant. Expected result for every row was a security-test failure (`FAIL`, exit 1); that is what occurred. `KILLED` below means the listed failure was directly observed, not inferred.

```sh
go -C pipeline test . -run '^TestBasicAuth' -count=1 -v
```

| Mutant | Exact production mutation | Raw summarized actual result | Outcome |
|---|---|---|---|
| Route-action mapping collapsed | In `pipeline/api_security.go`, replaced the complete `apiReadAction` body with `return ActReadQueue`. | `auditor_can_read_audit` and `auditor_can_export`: 403, want 200. `reviewer_cannot_export` and `coordinator_cannot_export`: 200, want 403. Package `FAIL`, exit 1. | **KILLED** |
| `authorizeVerified` OPTIONS bypass removed | Changed its pass-through condition from `!strings.HasPrefix(r.URL.Path, "/api/") || r.Method == http.MethodOptions` to only `!strings.HasPrefix(r.URL.Path, "/api/")`. | Existing export preflight: 403 `read permission denied`, want 204. New detail/export preflights: 403, want 204. Package `FAIL`, exit 1. | **KILLED** |
| Untrusted origin accepted | Changed the CORS deny guard to `if !origins[origin] && origin != "https://evil.example" {`, specially admitting the attack origin. | `untrusted_origin_GET`: 200 with case detail, want 403. `untrusted_origin_OPTIONS`: 204, want 403. Package `FAIL`, exit 1. | **KILLED** |
| Preflight routed downstream | Deleted the complete `if r.Method == http.MethodOptions { ... }` block in `corsWithOrigins`, so valid preflight fell through to `h.ServeHTTP`. | Existing export preflight and both new detail/export preflights reached case handlers: 405 `method not allowed` with JSON bodies, want empty 204. Package `FAIL`, exit 1. | **KILLED** |
| Basic handler lost authorization selection | In `pipeline/api.go`, changed `basicAPIHandler` from `return s.authorizeVerified(apiHandler(s))` to `return apiHandler(s)`. | `auditor_cannot_read_case_details`: 200, want 403. Reviewer/coordinator export: 200, want 403. Auditor audit/export remained 200. Package `FAIL`, exit 1. | **KILLED** |
| Recovery OPTIONS bypass removed | In `pipeline/recovery.go`, deleted the `r.Method == http.MethodOptions` pass-through block at the start of Basic authentication. | Existing export preflight and new detail/export preflights: 401 `authentication required`, want 204. Untrusted OPTIONS also became 401 instead of the required CORS 403. Package `FAIL`, exit 1. | **KILLED** |

## Survivors and restoration

- Survivors: **none** (6/6 executed mutants killed).
- After the required four mutants and the two integration-specific mutants, `git diff --exit-code -- pipeline/api_security.go pipeline/api.go pipeline/recovery.go` exited 0.
- `git diff --check` exited 0 and `git status --short` showed only `M pipeline/recovery_test.go` before this report was added.

## Not-run limits

- No container, Docker/Podman, external broker, live TLS listener, deployment, or external asset verification was run.
- No AMD64/cross-architecture verification and no soak was started or touched.
- No race-detector, fuzz, Python, frontend, or real-HTTP smoke run was performed in this review; evidence is from native macOS Go tests using synthetic in-process fixtures.
- These targeted mutants do not establish complete mutation coverage or production certification.
