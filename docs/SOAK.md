# Wasl 24-hour soak

## Scope

The soak measures the retained-read API and three-node JetStream cluster after one synthetic pipeline run. It never
restarts the retained lane's one-shot `pipeline`. Active NATS faults at T1 and T13 use separate, disposable Compose
projects and are reported as **fresh reruns**, not resumed ingestion. The package has no continuous production-feed
entrypoint, so this is not a continuous-ingest or production-availability claim.

## Preflight and launch

`scripts/soak.24h.json` freezes the 86,400-second duration, sampling cadence, absolute restart schedule, HTTP latency
budgets, leak windows, recovery budgets, and disk limits before T0. A full run requires a committed, clean source tree.
Run creation copies the Git-listed working source into `run/source`; modified and non-ignored untracked files are included,
tracked working-tree deletions are omitted, and symlinks are rejected so the snapshot cannot point back to mutable source.
Image builds, API smoke scripts and every later
Compose action use that snapshot, so commits or edits in the original checkout cannot change a run in progress. Resume
re-executes `run/source/scripts/soak.py`; missing snapshots and symlinks in the source, scripts or controller paths are
refused rather than resumed from a live checkout. The controller checks host disk, creates a unique project, loopback API port, image tags and protected key file, then
builds and records image IDs before starting containers. **T0 is the persisted `steady_start_epoch` after bootstrap,
NATS readiness, the mutating baseline smoke, artifact capture and export freezing complete.** Image build and bootstrap
time are not included in the 86,400-second retained-read measurement. Pending action deadlines and the completion
deadline are re-anchored to that T0; bootstrap samples cannot satisfy the observed-duration gate.

```bash
python3 scripts/soak.py smoke --engine podman
python3 scripts/soak.py run --config scripts/soak.24h.json --engine podman \
  > pipeline/results/soak/runs/24h-launch.log 2>&1
```

Use `--engine docker` on a verified Docker Engine host. Only the synchronous smoke is short. The `run` command is the
real 24-hour measurement and must be launched as a supervised background task. Do not describe it as complete until its
final `report.json` has `status: completed` and the observed-duration gate passes.

## Resume and reports

```bash
python3 scripts/soak.py run --resume /absolute/path/to/run --engine podman
python3 scripts/soak.py report --run-dir /absolute/path/to/run --status running
```

The controller serializes a locked, append-only event journal and atomic state snapshot. Resume recomputes the frozen
manifest digest rather than trusting its stored hash, restores the last sample sequence and each service's latest
container generation, and shares one state object between the scheduler and action book. Action deadlines are absolute UTC epochs and waits use a
monotonic clock. A controller restart skips completed actions. An action left in `started` state has an unknown outcome
and **blocks resume**; inspect and reconcile the actual project rather than blindly replaying it. A malformed,
unterminated final JSONL fragment is treated as a crash tail and removed before the next append, including the prior
hour's segment before an hourly rollover; malformed complete or interior records remain fatal. A partial report is always marked incomplete and cannot pass.

Each run contains:

- `source/`: frozen Git-listed working source used by the build, controller resume, smoke and Compose actions;
- `manifest.json`: source/configuration hashes, engine versions, isolated project, host context and pre-registered gates;
- `images.json`: immutable image tags and IDs captured after build and before pipeline start;
- `samples/*.jsonl`: fsynced, UTC-hour-rotated API, runtime, process, NATS, queue, health, container and disk samples;
- `events.jsonl`: action, restart, recovery, comparison, negative-test and cleanup events;
- `state.json`: atomic action/controller state;
- `baseline/`, `comparisons/`, `final/`, `active-*`: synthetic run evidence and restart-stable export comparisons;
- `summary.json`, `report.json`, `report.txt`: generated aggregates and gate outcomes.

`.env` is mode 0600 and contains the original JetStream key. It is excluded from Git and Docker builds. Never publish it,
full container inspect output, environment dumps, or resolved Compose configuration. The stored manifest and image evidence
contain no key material.

## Measurements and cleanup

API probes report client-observed latency distributions by route; pipeline ingest latency is not substituted. Every
full API cadence must cover health, dashboard, cases, fixtures, case detail, audit and export. Transport failures,
invalid JSON and HTTP 4xx/5xx responses fail the API error gate; missing routes and empty latency evidence fail closed.
Restart-stable export comparison requires actual comparison events, including every scheduled final comparison, and
compares the complete frozen baseline fixture set so missing or unexpected current fixtures fail. The API
exposes goroutine, heap/GC, FD, thread and current Linux RSS metrics only on container loopback. NATS samples include RSS,
raw stack-derived goroutine count, unique route peers, stream replicas, storage, pending, ack-pending and redelivery counts.
NATS health requires two routes and two current replica peers for every replicated stream, and health/ack-pending gates
require non-empty steady observations. Storage-growth windows ignore cadence slots with no NATS payload rather than
inserting zero-byte samples. Container samples include state generation, restart count, health, CPU, RSS, PIDs and I/O.

The smoke checks the machinery and restart gates; its single short resource window is not leak evidence. The full soak
requires six hourly post-warmup windows. The smoke uses volume-deleting cleanup only for its own unique disposable project.
A completed retained-read soak uses
`down` without `-v`; a failed run is left for inspection. Fresh active-fault projects are copied out and then removed with
scoped `down -v`. No command prunes global containers, images or volumes.

## Invalidated evidence: smoke 3b0362

Run `20261109T111946Z-3b0362` remains failed. Its `compose start api` restarted the completed one-shot pipeline, so
samples after the pipeline's `2026-11-09T19:30:18.487296723+08:00` start are not retained-read evidence. In particular,
the negative NATS storage-growth value reflects that unauthorized second pipeline run replacing stream contents, not
storage reclamation or a leak result. The second pipeline's differing FHIR/OUT counts and all later lifecycle observations
are invalid for availability or resource claims; they must not be used to resume or pass this run.
