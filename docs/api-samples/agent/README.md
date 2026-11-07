# Agent API samples (separate, CRC-only lane)

Status: **built** (responses from a real materialised run, served read-only); the data are **simulated** and synthetic. The agent investigations are **experimental**, cached, and were produced offline. Nothing here called a model or investigator.

These samples are separate from the seven-programme samples in `docs/api-samples/` because that run has no agent sidecar artifacts (`agent_recs.ndjson`, `agent_replay.json`), so it has no `agent_review` fixture. This set comes from one run that does: the r1agent10 seed-987 lane (`.../v2-s987/run/agent/main`, cohort `.../v2-s987/cohort`). It is CRC-only (600 cases), so the other six programme pages do not exist here.

## Files

- `healthz.json`, `dashboard.json`, `fixtures.json`, `cases_crc_page1.json`
- `case_<fixture>.json` and `case_<fixture>_audit.json` for `agent_review`, `evidence_closure` and `false_closure_trap`. This lane has no `review_hold` fixture.

`case_agent_review.json` carries `agent_investigations` (cited label `same`, human link `same` with an `agent` read, citations from `get_record`) and `agent_provenance` (`cached`, `experimental`, model, prompt and schema versions). Fixture ids are stable across repeated requests.

## Regenerating

```bash
WASL_SERVE_RUN=<lane>/run/agent/main WASL_SERVE_DATA=<lane>/cohort WASL_OUT=<scratch dir> scripts/demo.sh serve <host:port> &
WASL_SNAPSHOT_PROGRAMMES=crc WASL_API_ADDR=<host:port> WASL_OUT=<scratch dir> \
  WASL_SNAPSHOT_DIR=docs/api-samples/agent scripts/demo.sh snapshot
```

`scripts/api_smoke.py` is not expected to pass on this lane: its generic subset requires the `review_hold` fixture, which an agent-only lane lacks. The check was not weakened.

## Deviation

The generator cannot run `WASL_IFHAS=800` (background adult pool exhausted: 882 adults for 800 loops plus 100 distractors), so IFHAS capacity is 700 for the generated runs, as in `pipeline/RESULTS.md`. This lane was generated at that capacity; it contains no IFHAS cases.
