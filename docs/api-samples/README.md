# API samples

Static copies of read API responses (`wsi7 serve`) so the frontend can be built without a backend. All data is synthetic. Status: **built** (generated from a real materialised run); the data are **simulated**. These committed files are a legacy clinical-by-design fixture exception: they retain synthetic demonstration quotes/registrations and are not access-controlled clinical records. New snapshots are redacted-only; see [`../SECURITY.md`](../SECURITY.md).

## Files

- `healthz.json`, `dashboard.json`, `fixtures.json`
- `cases_<programme>_page1.json`: first 25 cases per programme present in the run (only nonempty pages are kept)
- `case_<fixture>.json` and `case_<fixture>_audit.json`: detail and audit chain for every fixture the API returns
- `person_journey.json`: the published person journey (below), whose anchor case is `case_person_journey.json`

## Live check

`scripts/demo.sh smoke` starts `serve` on loopback, polls `/healthz`, runs `python3 scripts/api_smoke.py --base-url ...` over real HTTP, then stops the server. It runs after `report` in `scripts/demo.sh all` (not in `container`). It checks health and dashboard consistency, bounded case lists per programme, the fixture tags `evidence_closure`, `false_closure_trap` and `review_hold` as a subset, detail and audit for every returned fixture, and a 404 for an unknown case. On the synthetic `review_hold` fixture it also exercises coordinator, consent, review, undo/contest and sanitized-export routes, checking status codes, canonical-decision immutability, ledger hashes/history, redaction and the export content hash. Its tests (`python3 scripts/test_api_smoke.py`) use a local stateful fake server and make no model calls.

To smoke or serve an existing materialised lane without regenerating anything or calling a model:

```bash
WASL_SERVE_RUN=<run dir> WASL_SERVE_DATA=<cohort dir> WASL_SMOKE_PORT=<free port> scripts/demo.sh smoke
```

## Regenerating the samples

Run the seven-programme demo in an isolated output directory and port offset, then serve it and snapshot into a clean staging directory:

```bash
export PATH=/opt/anaconda3/bin:$PATH      # NumPy/SciPy for the booking step; nats-server v2.10+ must also be on PATH (or bin/nats-server)
export WASL_OUT=<isolated out dir> WASL_PORT_OFFSET=<offset>
export WASL_LUNG=800 WASL_DR=1200 WASL_BREAST=70 WASL_LDCT=300 WASL_IFHAS=700 \
       WASL_DRNEXT=1 WASL_LUNGSURV=1 WASL_XREFLINK=1 WASL_REFREJECT=true WASL_SHOWCASE=4
scripts/demo.sh all                       # includes the smoke step
WASL_API_ADDR=127.0.0.1:<free port> scripts/demo.sh serve &   # then, with the server up:
WASL_API_ADDR=127.0.0.1:<free port> scripts/demo.sh snapshot  # writes docs/api-samples/
```

`scripts/demo.sh snapshot` does the gating itself: it writes into a fresh staging directory beside `docs/api-samples/`, requires all seven programme pages (`crc`, `cervical`, `lung_nodule`, `diabetic_retinopathy`, `breast`, `lung_ldct`, `ifhas`) to be nonempty, fetches only the auditor-redacted export for every fixture, converts it with `scripts/redacted_snapshot.py`, then deletes every tracked file except `README.md` and moves the staged files in, so no stale fixture files remain. Raw detail/audit responses and raw/escaped/nested/encoded canaries are refused. If any step fails the tracked samples are left untouched. `WASL_SNAPSHOT_DIR` overrides the target (used by the tests). The published `person_journey.json` is the person anchored at the API's `person_journey` fixture (**built**: `pickPersonFixture` in `pipeline/api_people.go`). That fixture is the first trusted canonical cluster whose journey spans at least three programmes and has an open obligation, an actionable outreach group and a `wasl-person-next-action/1` next action. The snapshot fails, leaving the samples untouched, when a run has no such person or `scripts/person_showcase.py` rejects the staged journey. It falls back to the `evidence_closure` person only when `WASL_SNAPSHOT_PLAIN_PERSON=1` is set, with a warning and without the showcase check, so other samples can still be regenerated from a run that has no qualifying person. A regenerated seven-programme lane without `WASL_SHOWCASE` (seed 2026, 17 Nov 2026) contains **no** qualifying person: of 6,650 trusted clusters only two span three or more programmes, and neither has an outreach intent on a second open case, so `scripts/demo.sh snapshot` refuses it. `WASL_SHOWCASE=4` (gen `-breastshowcase 4`, **built**, `pipeline/genbreast.go`) is the deterministic source of the published person. After every other breast draw, on its own RNG stream, it adds one breast loop for each of the first four women (by Qid, aged 40-90) whose CRC and cervical loops both end the window open at an outreach-raising step (booked, no-show, rejected referral; for CRC also an inadequate colonoscopy). Other feeds and every loop drawn without it are unchanged (`TestGenBreastShowcaseAddsLoopsOnlyForOutreachReadyWomen`). Linker thresholds, outreach rules and next-action rules are unchanged: the run decides whether she is linked, whether her intents are actionable and whether her consent lets them through. With the recipe above (seed 2026, 17 Nov 2026, `pipeline` at this commit) the API picks the anchor `ahs-mushrif/ServiceRequest/AHS-sr-930` (CRC, booked): three trusted cases (CRC booked, cervical no-show, breast closed with no recall needed), two open or overdue programme deadlines, an actionable cross-programme overlap group (the CRC appointment reminder and the cervical rebook-after-no-show, both released) and the next action `review_duplicate_outreach` on the anchor, citing that group. `scripts/demo.sh snapshot` accepted the lane, `scripts/person_showcase.py` passed, and `scripts/demo.sh smoke` passed. **Only `person_journey.json`, `case_person_journey.json` and `case_person_journey_audit.json` were taken from that snapshot.** The other files still come from the earlier lane, because the current pipeline sends the earlier `review_hold` case (`AHS-sr-1079`) to `suspected_malignancy`, so a full replacement would move `review_hold` to another case and report, which the review fixtures, `web/data/review-sources.json` and the review browser specs are bound to. Replacing every sample in one pass is left to an integration change. The earlier single-programme CRC journey is kept as separate test data for the no-action state (`web/tests/fixtures/person-journey-no-action.json`). `scripts/redacted_snapshot.py` fills the fields the auditor-redacted export leaves out and the web schema requires with neutral placeholders (empty strings, `redacted-decision-<n>` decision labels in order, a `redacted` consent-overlay revision), so a regenerated case detail opens in fixtures mode. Checking these lanes also required three runtime fixes (LDCT clock-anchor block hashing, the Fleischner surveillance parent head, and the person projection's policy-name check); before them the recipe aborted at `run` or `report` and `/api/people` returned 503 on real v7 runs. Under CPU contention `run` may also need `WASL_QUIETTIMEOUT=20m`. Finally rerun the Go API tests with `WASL_TEST_RUN=<run dir>` set so the run-dependent tests do not skip.

Note: with `WASL_IFHAS=800` the generator panics after the other six programmes (background pool exhausted, 882 adults for 800 loops plus 100 distractors); the recipe above therefore uses `WASL_IFHAS=700`, as in `pipeline/RESULTS.md`.

A clean-clone regeneration is not byte-identical to the committed files. Case and programme ids match and the safety invariants checked by the smoke test hold. Fixture ids match only while the pipeline's case outcomes do: on 17 Nov `review_hold` moved from `AHS-sr-1079` to `AHS-sr-1229` because the current pipeline no longer closes the former. Run-dependent fields differ: `loaded_at`, event sequence numbers (`in_seq` and the ids built from them), `throughput_ev_s`, and two consent counters (`released_total`, `intents_ever_held_for_consent`). Their differences are likely timing-dependent; this is not proven to be caused by NATS timing. Compare structure and safety invariants, not bytes.

## Agent samples (separate lane)

The seven-programme samples above come from a run with no agent sidecar artifacts, so they contain no `agent_review` fixture and no real agent investigations. A truthful, separate CRC-only set built from the r1agent10 seed-987 lane (real cached agent output) is in [`agent/`](agent/README.md). `WASL_SNAPSHOT_PROGRAMMES=crc` scopes the required programme pages (comma list; default is all seven). IFHAS generator capacity is 700, not 800, for that lane too. The two sets are not merged and do not share fixture ids.
