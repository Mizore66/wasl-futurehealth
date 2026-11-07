# Final API/deploy cleanup report

Base cd7fc9e. Branch `feat/final-api-deploy-cleanup-wt2` (the requested name `feat/final-api-deploy-cleanup` was already
checked out in another worktree, agent-af331562e559b5b73, so it could not be reused).

## Red then green
- Go: `go test -run 'TestAgentIncomplete|TestAgentDuplicate' .` before the fix failed all 7 incomplete-citation subtests
  ("partial citation must yield no agent projection") and the duplicate test ("want 1 investigation, got 2"). After: ok.
- demo.sh: `test_snapshot_sigint_removes_staging_and_keeps_samples` failed with the fix reverted (staging dir left), passes with it.
- check_deploy: absolute and `../` missing-source tests failed (exit 0) before, pass after.

## Changes
1. `pipeline/api.go` cleanCitations: every citation needs tool, arg, field, value; strings must be nonempty; otherwise the
   whole record set fails closed (no projection, no agent_review). Tests: TestAgentIncompleteCitationsFailClosed
   (`[{}]`, each missing key, empty value, good+partial).
2. `loadAgent`: per loop, repeated consumed sign-offs for the same item keep the first; exact item+loop+pair and fail-closed
   checks unchanged. Test: TestAgentDuplicateSignoffsDeduped.
3. `scripts/demo.sh` snapshot: trap removes the staging dir on EXIT/INT/TERM, cleared after the successful move.
   Contents of snapshots unchanged. Tests: sigint, failure and success leave no staging dir and tracked samples intact.
4. `scripts/check_deploy.py`: every bind source (absolute, `../`, `~`, `./`, bare relative path containing `/`) is checked
   for existence; named volumes (no `/`) still accepted. A missing PyYAML now exits with a clear message instead of a traceback.
   Tests: missing absolute, missing `../`, existing absolute accepted. `docs/DEPLOY.md`: one sentence on the PyYAML requirement.

## Final runs
- `cd pipeline && go test ./...`: `ok  wsi  149.284s`.
- `/opt/anaconda3/bin/python3 -m unittest scripts.test_deploy` (has PyYAML): Ran 22 tests, OK.
- `/opt/anaconda3/bin/python3 scripts/check_deploy.py`: static checks OK (5 services, 4 volumes).
- `/opt/homebrew/bin/python3 scripts/check_deploy.py` (no PyYAML): exit 1 with "needs PyYAML ... did NOT run".
