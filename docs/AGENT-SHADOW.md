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
- Bounds (flags may lower, never raise): 20 items, 40 uncached provider requests, 2 responses and 6 tool calls per item,
  45 s per provider request, 15 minutes overall by default. The counter is persisted in `usage.json` and in the cache's
  `.uncached_calls` file, so a restart or deleting one file cannot reset it.
- Signals: the first SIGINT/SIGTERM disables new uncached requests and forwards SIGTERM to the running worker. Cached
  replay of what was already consumed can finish. A second signal exits. Nothing restarts the process: it is not an
  orchestrator child and has no supervisor.
- The worker's environment is `PATH`, `HOME`, locale and TLS variables only; the provider variables are added in live
  mode only. The orchestrator and its children never receive them.

## What the model sees

Only the sanitized tool outputs of `ToolAPI`: HMAC aliases, names, scripts, provider names, DOB and its status, sex,
identifier presence and agreement. The staged lane per item (`run/main/review_queue.json`, `link_funnel.ndjson`,
`prep_log.ndjson`, `agent_need.ndjson`, empty `cohort/`) is built from the two registrations in the shadow's own linker
state at raise time. It never contains live review quotes, chain blocks, heads, report text, raw report ids, truth,
reviewer answers, donor maps or secrets, and is deleted as soon as the item finishes.

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

- Go: `pipeline/agentshadow_test.go`, `agentshadow_nats_test.go` (real `nats-server`, skipped if `bin/nats-server` is
  absent). Python: `scripts/test_agent_shadow_worker.py`. No test makes an external call.
- Off-vs-on comparison: `scripts/agent_shadow_parity.sh`, `scripts/agent_shadow_parity.py`. Strict means exact record
  order with only the documented timing fields removed (stream sequence numbers and timestamps, `Wsi-Ots`, `pub_ts`,
  `proc`, `lat_ms`, prep `ots`). Sorted comparisons are reported separately as secondary and never counted.
  The result is in the Task D report, with the limits stated there.
- Live smoke: `scripts/agent_shadow_smoke.sh`, 3 items and 6 uncached requests at most, synthetic cohort only.

## Limits

A spike on a synthetic cohort with simulated records. The report gives coverage, agreement with the simulated human
answer, fallback reasons, latency and token counts. Agreement on a few items says nothing about accuracy, and nothing
here shows reviewers would save time or that recall would improve.
