# API samples

Static copies of the read API responses (`wsi7 serve`) so the frontend can be built without a backend. All data is synthetic. Status: **built** (generated from a real materialised run); the data are **simulated**.

## Files

- `healthz.json`, `dashboard.json`, `fixtures.json`
- `cases_<programme>_page1.json`: first 25 cases per programme present in the run (only nonempty pages are kept)
- `case_<fixture>.json` and `case_<fixture>_audit.json`: detail and audit chain for every fixture the API returns

## Live check

`scripts/demo.sh smoke` starts `serve` on loopback, polls `/healthz`, runs `python3 scripts/api_smoke.py --base-url ...` over real HTTP, then stops the server. It runs after `report` in `scripts/demo.sh all` (not in `container`). It checks health and dashboard consistency, bounded case lists per programme, the fixture tags `evidence_closure`, `false_closure_trap` and `review_hold` as a subset, detail and audit for every returned fixture, and a 404 for an unknown case. Its tests (`python3 scripts/test_api_smoke.py`) use a local fake server and make no model calls.

To smoke or serve an existing materialised lane without regenerating anything or calling a model:

```bash
WASL_SERVE_RUN=<run dir> WASL_SERVE_DATA=<cohort dir> WASL_SMOKE_PORT=<free port> scripts/demo.sh smoke
```

## Regenerating the samples

Run the seven-programme demo in an isolated output directory and port offset, then serve it and snapshot into a clean staging directory:

```bash
export PATH=/opt/anaconda3/bin:$PATH
export WASL_OUT=<isolated out dir> WASL_PORT_OFFSET=<offset>
export WASL_LUNG=800 WASL_DR=1200 WASL_BREAST=70 WASL_LDCT=300 WASL_IFHAS=800 \
       WASL_DRNEXT=1 WASL_LUNGSURV=1 WASL_XREFLINK=1 WASL_REFREJECT=true
scripts/demo.sh all                       # includes the smoke step
WASL_API_ADDR=127.0.0.1:<free port> scripts/demo.sh serve &   # then, with the server up:
WASL_API_ADDR=127.0.0.1:<free port> scripts/demo.sh snapshot  # writes docs/api-samples/
```

`scripts/demo.sh snapshot` does the gating itself: it writes into a fresh staging directory beside `docs/api-samples/`, requires all seven programme pages (`crc`, `cervical`, `lung_nodule`, `diabetic_retinopathy`, `breast`, `lung_ldct`, `ifhas`) to be nonempty, fetches detail and audit for every fixture the API returns, then deletes every tracked file except `README.md` and moves the staged files in, so no stale fixture files remain. If any step fails the tracked samples are left untouched. `WASL_SNAPSHOT_DIR` overrides the target (used by the tests). Finally rerun the Go API tests with `WASL_TEST_RUN=<run dir>` set so the run-dependent tests do not skip.

Note: with `WASL_IFHAS=800` the generator panics after the other six programmes (background pool exhausted, 882 adults for 800 loops plus 100 distractors); the samples were regenerated with `WASL_IFHAS=700`, as in `pipeline/RESULTS.md`.
