# Soak source-version drift: root cause and regression

**Status:** root cause established from preserved evidence; harness fix built, process-tested and independently approved. The original failed project was not rerun or deleted; shutdown logs were added before its containers/network were removed, and its evidence volumes remain. A separate short disposable smoke reached the pipeline but failed before API startup for the separate ingestion/tamper reasons below. Its runtime evidence is preserved and its disposable resources were cleaned. No 24-hour soak was started or scheduled.

## Preserved failure

Run `20261109T120307Z-2e75af` remains incomplete with `overall_pass=false`. Its immediate active-lane failure was:

```text
api smoke FAILED: unauthenticated case list: expected 401, got 200
```

The other reported failures (collector heartbeat, NATS health/ack-pending, missing final comparison and incomplete duration) are not reclassified or fixed by this investigation.

## Root cause

The controller mixed two source revisions:

| Time (UTC) | Evidence |
|---|---|
| 9 Nov 12:03 | The manifest froze Git revision `fd1ad5a870815737394ad9da7b8d0ea1e8c9f789`; the API image was built from that checkout. |
| 9 Nov 13:20 | `active-fhir-01` passed the then-current smoke, which had no unauthenticated-401 check. |
| 9 Nov 17:10 | The coordinator checkout advanced to `46f3cff79867f037c93425c4c890119a73f32418`, whose tree contained bearer authentication and the newer smoke contract. The already running controller and recorded image did not change. |
| 10 Nov 02:15 | `active-fhir-13` reused the recorded old image but invoked the newer `scripts/api_smoke.py` from the live coordinator checkout; it failed on the new 401 assertion. |

The preserved image ID is `fdc44e9632046d29cfcbf0db253f57a33d927fa1e7f1c826c745a7f3df8ed588`, matching `images.json`. Read-only image inspection shows the same ID and no API token or Basic-auth variables in the image environment. The lane `.env` files likewise contain no API token entries; the old launcher did not require them.

At `fd1ad5`, `runServe` served `cors(mux)` directly and trusted demo role headers; it had no bearer middleware. Thus the retained old API's unauthenticated 200 is real **for that old revision**, but it was not introduced by the later authentication integration. Current `runServe` selects the bearer-backed handler when no Basic-auth recovery file is configured and fails closed when bearer configuration is missing. This failed soak is not evidence that the integrated handler bypasses authentication, and it is not evidence that current deployment authentication passed a soak.

The harness defect was in `SoakRunner`: it derived `self.repo` from the running script's live checkout. `run_api_smoke` later executed `self.repo/scripts/api_smoke.py`, and baseline/fresh-lane Compose lifecycle commands also used that live directory. `require_clean_source=true` checked cleanliness only at creation; it could not prevent later checkout changes.

## Minimal fix

Run creation now copies the Git-listed working source (tracked plus non-ignored untracked files, preserving dirty-tree smoke behavior) into `run/source`. Modified and non-ignored untracked files are included, tracked working-tree deletions are omitted, and symlinks are rejected rather than copied as links to mutable targets. The build, manifest source hashes, API smoke, retained-lane Compose actions and fresh-lane Compose actions all use this directory. Resume re-executes `run/source/scripts/soak.py`; missing or symlinked frozen controllers are refused rather than silently using a live checkout.

## Regression evidence

The initial three regressions were watched fail before implementation:

1. Editing `scripts/api_smoke.py` after run creation does not change the smoke that executes.
2. A later Compose restart runs from `run/source`, not the edited checkout.
3. `run --resume` executes the frozen controller.

Independent review then identified symlink escapes and cached-but-deleted working-tree files. New absolute/relative symlink, deleted-file and symlinked-resume regressions failed before those fixes. A second review pass identified symlinked `source/` and `source/scripts/` parents; process regressions proved external execution before the shared path guard and pass after it, including direct runner construction. Positive coverage also proves dirty modified/untracked files enter the snapshot and a missing frozen controller is refused.

After the review fixes, `python3 scripts/test_soak.py CreateRunTests` passed 11/11 tests and the full `python3 scripts/test_soak.py` suite passed 91/91. `python3 scripts/test_api_smoke.py` passed 40/40, and `go -C pipeline test . -run 'Security|Authentication' -count=1` passed. The source-isolation regressions also passed three consecutive runs. Independent final review reported no remaining Critical or Important findings and **Ready to merge: Yes**.

## Short smoke outcome and cleanup

Run `20261110T024209Z-989dc4` used the dirty-tree snapshot path with the documented local demo tokens. Its images built from `run/source`, and recursive comparisons of the frozen `pipeline/` (excluding generated results) and `scripts/` against the checkout found no differences. This directly exercised inclusion of the modified/untracked harness files rather than only `HEAD`.

The smoke did **not** become green. The pipeline timed out waiting for quiescence after 550 FHIR messages, its audit recorded `generated_resources=32245`, `ingest_lost=31699` and zero closed predictions, and the subsequent tamper command panicked with `index out of range [-1]` at `tamper.go:430`. Compose therefore stopped before API startup with pipeline exit 2. This is a separate ingestion/tamper failure, not evidence of an omitted snapshot file or an API-auth result; the immediate ingestion cause was not investigated in this task.

The failed short run's report, source snapshot, service logs, audit/tamper diagnostics and complete materialized runtime were preserved under `pipeline/results/soak/runs/20261110T024209Z-989dc4/` before its disposable `down -v`. No second smoke or 24-hour run was started. Final checks found no soak controllers or containers. The original failed run's logs/state were likewise preserved before `down` without `-v`; all four of its evidence volumes remain.
