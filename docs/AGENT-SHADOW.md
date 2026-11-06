# Agent shadow (read-only spike)

Status: **built as a spike, simulated cohort only.** It is not integrated into the product path, is not started by the
orchestrator, and makes no claim about recall, reviewer time, clinical effectiveness or assist readiness.

`wsi7 agentshadow` watches a running pipeline and, for a small number of eligible missing-demographic link questions,
asks the evidence investigator (`scripts/agent_evidence.py`, `scripts/agent_investigator.py`) what it would have
recommended. The recommendation is written to a private directory. Nothing reads it back: the human review path
continues unchanged and no decision, sign-off, link, outreach message, audit entry or API review state is touched.

## What it reads, what it writes

| | |
|---|---|
| Reads | the `PREP` and `OUT` JetStream streams, through ordered ephemeral consumers (no durable consumer, no acks) |
| Publishes | nothing. No `Publish`, no stream, consumer or KV creation. A test asserts this on the wire and in the source |
| Writes | `<run>/agent_shadow/` and `<run>/status-agentshadow.json`, nothing else |
| Does not use | `-shadow` / `WASL_SHADOW` (clinician pilot mode), `-agentrecs` (agentreplay input) |

Files in `<run>/agent_shadow/`: `observations.ndjson` (one line per item outcome, last line wins), `usage.json`
(persisted call and item counters), `report.json`, `health.json`, `agent_shadow_recs.ndjson` (the model's own terminal
records, `write_recs` format), `cache/` (provider response cache and alias salt), `STOP` (optional stop file).

## How it decides to investigate

1. PREP messages are re-sequenced and applied to a fresh `LinkerCore` and `ClosureCore` with the same apply path as the
   `audit.go` replay. The outputs are discarded, but each output's ledger hash is kept.
2. Each live `OUT` message is compared with the shadow's own output at the same partition and ledger position. A hash
   mismatch, a live output the shadow never derived, or a mismatching item (id, pair or head) marks the shadow
   **degraded**.
3. A review item is investigated only if all of these hold: the live and derived items match on id, pair and head; it is
   a plain link question (`isLinkQuestion`); and at the moment it was raised the pair was a missing-demographic
   question (`agentEligiblePair`: exact `demographic_missing` trace, no hard conflict, no contradicting EID, no
   cannot-link) with no human answer yet for that pair. Eligibility is judged at raise time, not at the end of the stream.
4. One item per unordered pair: a twin question on another loop reuses the first outcome.

**Degraded means no new provider call.** Items still queued are recorded as skipped (`degraded`). The status and
`health.json` say so.

## Controls

- `-agentmode cached` is the default. Live needs `-agentmode live`, `WASL_AGENTSHADOW_LIVE=1` and
  `WASL_LLM_URL`, `WASL_LLM_KEY`, `WASL_LLM_MODEL` in the environment. In cached mode the worker has no transport object,
  so a cache miss cannot become a network call.
- Cache first, always: a request whose cache key exists is replayed with no gate and no counter change.
- Before every **uncached** request the worker checks, in order: live mode, SIGTERM/SIGINT, the stop file
  (`<run>/agent_shadow/STOP`), the overall deadline, and the persisted call cap. Any failure is an operational
  fallback.
- Bounds (flags may lower, never raise; zero, negative or too-large values are rejected, not clamped): 20 items, 40
  uncached provider requests, 2 responses and 6 tool calls per item, 45 s per provider request, 15 minutes overall by
  default (the overall window starts at the first process start and is persisted, so a restart does not reset it).
  The lifetime call count lives in `usage.json` and is **seeded into every worker** (`--calls-used`), so a different or
  empty response cache cannot reset it; the worker's own cache counter can only raise it. Before a live item starts, its
  worst case (two requests) is reserved and fsynced; if `usage.json` cannot be written, no uncached request is made and
  the shadow is marked degraded.
- Cached replay of a live-populated cache: the non-secret model name is part of the cache key, so cached and live runs
  name the same model (`WASL_LLM_MODEL`, else the `model.txt` a live worker leaves beside the cache). Cached mode still
  has no transport object. A killed, stopped or capped shadow therefore replays what live run recorded, identically.
- Signals: the first SIGINT/SIGTERM disables new uncached requests and forwards SIGTERM to the running worker. A signal
  that lands between the decision to start a worker and the process existing is held and delivered right after start
  (and the worker installs its handlers before its heavy imports). Degrading (divergence) does the same, and both also
  create a kill file the worker checks before every uncached request. Cached replay of what was already consumed can
  finish. A second signal exits. Nothing restarts the process: it is not an orchestrator child and has no supervisor.
- Consumption: the stream consumers only read; a separate goroutine runs the workers, so a long investigation never
  stops the reading. The process exits only when everything stored was consumed, nothing is queued and no worker runs.
  If anything is still waiting at exit (live outputs without their PREP, a PREP gap, queued items) the shadow is marked
  `degraded` with reason `incomplete:...` and `incomplete: true` in the status and report. Consumer errors back off and
  re-create the ordered consumer. A PREP/OUT stream that was recreated under the same run directory is detected and
  degrades the shadow. `NATS` must be local (127.0.0.1, localhost, ::1).
- The worker's environment is `PATH`, `HOME`, locale and TLS variables only; the provider variables are added in live
  mode only. The orchestrator and its children never receive them.

## What the model sees

Only the sanitized tool outputs of `ToolAPI`: HMAC aliases, names, scripts, provider names, DOB and its status, sex,
identifier presence and agreement. The staged lane per item (`run/main/review_queue.json`, `link_funnel.ndjson`,
`prep_log.ndjson`, `agent_need.ndjson`, empty `cohort/`) is built from the two registrations in the shadow's own linker
state at raise time. It never contains live review quotes, chain blocks, heads, report text, raw report ids, truth,
reviewer answers, donor maps or secrets. The worker needs the two registrations' names, DOBs and identifier, so the lane
is the one place those exist: it is staged in a private (0700) directory outside the run directory
(`$TMPDIR/wasl-agentshadow-<uid>-<hash>`), deleted when the item finishes, swept at start-up and at exit. Persistent
artifacts (`observations.ndjson`, `agent_shadow_recs.ndjson`, `usage.json`, status) carry only salted aliases of the
pair (`pair_...`, `reg_...`), a hash of the cache path, and no names, DOBs, identifiers, keys or paths. The model's
cited field values (a name or DOB the tool returned) do appear in `agent_shadow_recs.ndjson` by design: that file is
the investigator's own record format (`write_recs`).

## Fallback is not an answer

A provider failure, timeout, cache miss, stop, deadline, kill or cap is recorded as `outcome: fallback` with a stable
reason and **no label**. Only the model's own terminal record (`same`, `different`, or its own `uncertain` with a
stable model-side reason) is a recommendation. A fallback never becomes a fabricated `uncertain`. Retryable fallbacks
(`cache_miss`, `killed`, `stop_file`, `deadline`, `cap_calls`, `item_cap`, `degraded`, worker errors) are tried again by a
later process within the same caps and do not use a second item slot; final outcomes are not repeated after a restart.

## Running it

```bash
# beside a running orchestrate (same -data, -wasl, -run, -P and policy flags; -demoguard is required)
./wsi7 agentshadow -run RUN -data COHORT -wasl models -P 2 -modelset v3 -policy v7strict -linkfloor 0.01 -r1review -demoguard \
  -agentmode cached
# stop: touch RUN/agent_shadow/STOP  (blocks new uncached requests)   or send SIGTERM
```

It exits when every provider has sent EOF and no event has arrived for `-agentidle`, or after a signal once the streams
are caught up, and prints the report. Scope check: `-data` must hold a generated cohort (`cohort_summary.json`).

## Evidence in this repository

- Go: `pipeline/agentshadow_test.go`, `agentshadow_fix1_test.go` (the real `python3` worker against a loopback fake
  provider: cache replay after live, restart with a different cache, kill/degrade races, raw-artifact scan, long drain),
  `agentshadow_nats_test.go` (real `nats-server`, skipped if `bin/nats-server` is absent). Python:
  `scripts/test_agent_shadow_worker.py`. No test makes an external call (the "provider" is 127.0.0.1).
- Off-vs-on comparison: `scripts/agent_shadow_parity.sh`, `scripts/agent_shadow_parity.py`. Strict means exact record
  order with only the documented timing fields removed (stream sequence numbers and timestamps, `Wsi-Ots`, `pub_ts`,
  `proc`, `lat_ms`, prep `ots`). Sorted comparisons are reported separately as secondary and never counted.
  Result, one synthetic cohort (seed 9101): the base, `off`, `off2` and `on` runs are strictly equal, with 5,226 FHIR,
  5,226 PREP and 2,798 OUT messages and 0 reviewer decisions. This is a parity result only: no recall, reviewer-time,
  clinical or assist-readiness claim. Its scope is in the Limits section below.
- Live smoke: `scripts/agent_shadow_smoke.sh`, 3 items and 6 uncached requests at most, synthetic cohort only; free
  loopback port, waits for the broker instead of a fixed sleep.

## Limits

The parity claim covers only the deterministic no-fault, single-partition HL7, `-noreview` configuration (no reviewer
decisions exist in it). `-source file`, default fault injection and the simulated reviewer are not byte-comparable even
without a shadow, so nothing is claimed for them. Workers run one process per item; a restart after a hard crash loses
at most the in-flight item's outcome (usage was reserved first, so the cap still holds); there is no mid-run checkpoint of
the consumer position (a restart replays both streams from sequence 1).

A spike on a synthetic cohort with simulated records. The report gives coverage, agreement with the simulated human
answer, fallback reasons, latency and token counts. Agreement on a few items says nothing about accuracy, and nothing
here shows reviewers would save time or that recall would improve.
