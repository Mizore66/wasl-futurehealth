# Wasl 24-hour soak

## Scope

The soak measures the retained-read API and three-node JetStream cluster after one synthetic pipeline run. It never
restarts the retained lane's one-shot `pipeline`. Active NATS faults at T1 and T13 use separate, disposable Compose
projects and are reported as **fresh reruns**, not resumed ingestion. The package has no continuous production-feed
entrypoint, so this is not a continuous-ingest or production-availability claim.

## Preflight and launch

`scripts/soak.24h.json` freezes the 86,400-second duration, sampling cadence, absolute restart schedule, HTTP latency
budgets, leak windows, recovery budgets, and disk limits before T0. A full run requires a committed, clean source tree.
The controller checks host disk, creates a unique project, loopback API port, image tags and protected key file, then
builds and records image IDs before starting containers.

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

The controller serializes a locked, append-only event journal and atomic state snapshot. Action deadlines are absolute
UTC epochs and waits use a monotonic clock. A controller restart skips completed actions. An action left in `started`
state has an unknown outcome and **blocks resume**; inspect and reconcile the actual project rather than blindly replaying
it. A partial report is always marked incomplete and cannot pass.

Each run contains:

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

API probes report client-observed latency distributions by route; pipeline ingest latency is not substituted. The API
exposes goroutine, heap/GC, FD, thread and current Linux RSS metrics only on container loopback. NATS samples include RSS,
raw stack-derived goroutine count, unique route peers, stream replicas, storage, pending, ack-pending and redelivery counts.
Container samples include state generation, restart count, health, CPU, RSS, PIDs and I/O.

The smoke checks the machinery and restart gates; its single short resource window is not leak evidence. The full soak
requires six hourly post-warmup windows. The smoke uses volume-deleting cleanup only for its own unique disposable project.
A completed retained-read soak uses
`down` without `-v`; a failed run is left for inspection. Fresh active-fault projects are copied out and then removed with
scoped `down -v`. No command prunes global containers, images or volumes.
