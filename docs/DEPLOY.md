# Deploying Wasl on a provider network's own servers

This is the package for the "second door" in [`WRITEUP.md`](WRITEUP.md): a provider network runs Wasl on its own
servers to track its own patients across its own facilities. It is a demo-grade package: it brings up the same
system that the integrated runs in [`pipeline/RESULTS.md`](../pipeline/RESULTS.md) use, on synthetic data. It is not
production-ready; the gaps are listed at the end.

## One command

```bash
scripts/deploy_up.sh            # = random environment key + docker compose up --build
```

This builds the image, starts a 3-node NATS JetStream cluster, runs the demo cohort end to end (with kill/restart
chaos) and then serves the read API on `http://127.0.0.1:8088` ([`API.md`](API.md)). Check it with
`curl 127.0.0.1:8088/healthz` and `curl 127.0.0.1:8088/api/fixtures`. The run summary is the last output of the
`pipeline` container (`docker compose logs pipeline`).

`scripts/check_deploy.py` checks the package statically without Docker (compose references, non-root image, key
handling, every `COPY` source present and not excluded by `.dockerignore`).

## What runs

| Service | Image | What it does | Lifetime |
|---|---|---|---|
| `nats-0`, `nats-1`, `nats-2` | `nats:2.12.1-alpine` | JetStream cluster from [`deploy/nats/nats.conf`](../deploy/nats/nats.conf): file store, **ChaCha20-Poly1305 encryption at rest**, streams with 3 replicas (Raft quorum writes) | long-running |
| `pipeline` | `wasl:local` (this repo's `Dockerfile`) | `scripts/demo.sh container`: generate the synthetic cohort, run every pipeline service against the cluster (mock HIE, FHIR subscription consumer or HL7 MLLP listener, 3 prep workers, linker, 4 closure partitions, simulated reviewer), then audit, tamper check, booking and the summary | one-shot, exits 0 |
| `api` | `wasl:local` | `scripts/demo.sh serve`: the read API over the finished run | long-running, starts after `pipeline` succeeds |

The pipeline services run as supervised processes inside the `pipeline` container. The orchestrator restarts a
crashed service and performs the demo's chaos kills (linker, closure, prep, the FHIR consumer). It does not start or
kill NATS nodes when the cluster is external (`WASL_NATS_EXTERNAL=1`), so the demo's NATS-node kill is skipped in
this package; use `docker compose kill nats-2 && docker compose start nats-2` during a run to test it.

The image is multi-stage: a static Go build (`CGO_ENABLED=0`), then `python:3.12-slim` with numpy and scipy for the
cohort augmentation, booking and summary scripts. It runs as uid 10001. It contains the committed synthetic inputs and
JSON model weights only: no keys, no patient data, no run outputs.

## Ports and volumes

| | Where | Notes |
|---|---|---|
| API | `127.0.0.1:8088` on the host (`WASL_API_PORT` to change) | loopback only; put a TLS reverse proxy with authentication in front for anything else |
| NATS client 4222, cluster 6222, monitor 8222 | compose network only | not published to the host |
| Mock HIE, FHIR rest-hook, HL7 MLLP | `127.0.0.1` inside the `pipeline` container | demo traffic only |
| `nats-0`, `nats-1`, `nats-2` volumes | `/data/jetstream` | encrypted JetStream stores; unreadable without `WASL_JS_KEY` |
| `wasl-run` volume | `/var/lib/wasl` | cohort and run outputs (`run/<policy>-cached-<source>/`): logs, decisions, `report7.json`, review queue. The API reads it |

## Configuration

- `WASL_JS_KEY` (required): the JetStream encryption key, passed to the three NATS containers only. It must start with
  a letter. `scripts/deploy_up.sh` makes a random one if it is unset; the script does not write it to the repository or
  image. Docker does store container environment configuration on the Docker host, so protect that host and use suitable
  secret/KMS controls in production. Losing the key makes retained stores unreadable.
- **Retained volumes need the original key.** If this Compose project's `nats-*` volumes exist and `WASL_JS_KEY` is
  unset, `deploy_up.sh` refuses to generate an incompatible key. Supply the original key to keep the data. To discard
  this deployment's stores and run data intentionally, run `docker compose down -v`, then rerun `deploy_up.sh`.
- `WASL_SEED`, `WASL_POLICY` (`v7strict`, `v7ltt`, `v6`), `WASL_SOURCE` (`fhir`, `hl7`), `WASL_LUNG`, `WASL_DR`,
  `WASL_BREAST`: as in `scripts/demo.sh`. The reader is the cached one (no model server in the package).
- `docker compose down` keeps the volumes; `docker compose down -v` deletes the stores and the run. Each new pipeline
  run recreates `FHIR`/`PREP`/`FACTS`/`OUT` and deletes the `SEEN`, `FHIRSUB` and `FHIRPOLL` KV buckets before services
  start. Thus a new cohort cannot silently inherit replay suppression, subscription/polling checkpoints or EOF state;
  service restarts within that run still resume from their checkpoints. Setup fails if reset fails.
- `WASL_NATS_URL` and `WASL_NATS_EXTERNAL=1` must be set together. Either setting alone is rejected by the Go commands
  and `scripts/demo.sh`; with neither set, the demo uses its local three-node cluster.

## Pointing it at real HL7 feeds

The production-shaped ingest tier is HL7 v2 over MLLP (`WRITEUP.md`, tier a). For a network's own feeds:

1. Run the MLLP listener (`wsi7 hl7mllp -hl7 0.0.0.0:2575 -run <run dir>`) as its own long-running service with
   `WASL_NATS_URL` set to the cluster, and publish its port on the network's integration subnet only. It ACKs a
   message only after JetStream has persisted it, so the sending engine re-sends anything unacknowledged.
2. Point the network's integration engine at it: ADT, ORM^O01, ORU^R01, SIU and MDM for the facilities in scope.
   Dedupe is by `source|type|id|version`, so re-sends and replays are safe (JetStream window 1 h, then the KV seen-ledger).
3. Do not run the generator, the mock HIE or `hl7send`. Run the linker, prep, closure and reviewer services the same
   way (`wsi7 linker|prep|closure|reviewer` with `-run` and `-wasl models`).
4. Replace the demo provider table (`Providers` in `pipeline/gen.go` and the programme files) with the network's
   facilities and their MRN systems, and map the network's local result codes to the programme readers.

Steps 3 and 4 are code and configuration work that has not been done: today every service still takes the demo
cohort directory, and there is no production entrypoint. They are listed under "not production-ready".

## Validation done for this package

Docker was **not installed** on the build machine, so neither `docker build` nor `docker compose up` has been run.
What was checked instead:

- The Go build stage compiles statically for `linux/amd64` and `linux/arm64` (`CGO_ENABLED=0 go build -trimpath`),
  and `GOOS=linux go vet` passes.
- `nats-server -t` accepts `deploy/nats/nats.conf`.
- `scripts/check_deploy.py` passes. `scripts/test_deploy.py` exercises retained-volume key refusal and external-NATS
  validation with a fake Docker CLI; no Docker daemon is needed.
- The container path was emulated natively: a tree with exactly the files the `Dockerfile` copies, a static binary,
  `scripts/demo.sh container` run against three `nats-server` processes started from `deploy/nats/nats.conf` (ports
  changed to local ones) with `WASL_NATS_URL` and `WASL_NATS_EXTERNAL=1`, then `scripts/demo.sh serve`. Results are
  under "Emulated run" below.

## Emulated run

The same seed was run through the native default path and the emulated container path on the build machine. These are
simulated cohort results; the emulated path used the external-NATS mode and therefore skipped the orchestrator's
NATS-node kill, while the native path included it.

| Metric | Native | Emulated container layout |
|---|---:|---:|
| FHIR messages / outputs | 33,389 / 24,805 | 33,386 / 24,808 |
| Pipeline wall time / throughput | 87.355 s / 382.22 events/s | 88.596 s / 376.84 events/s |
| Duplicate / lost outputs | 0 / 0 | 0 / 0 |
| Live-versus-replay final-state mismatches | 0 | 0 |
| CRC P / R / false closures | 1.000 / 0.851 / 0 of 479 | 1.000 / 0.851 / 0 of 479 |
| Cervical P / R / false closures | 1.000 / 0.967 / 0 of 204 | 1.000 / 0.967 / 0 of 204 |
| Wrong-patient / without-consent releases | 0 / 0 | 0 / 0 |
| Provenance chains / forged events with effect | 905/905 / 0 of 24 | 905/905 / 0 of 24 |

The summaries are **not byte-identical**: 99 scalar fields differed, chiefly chaos/replay timing, subscription resume
positions, review/outreach counts and latency. The three-message/output difference is nondeterministic chaos/replay
behaviour, so this comparison does not claim exact equality.

The emulated API returned `status: ok`, 2,100 materialised cases and `nats: up` from `/healthz`, plus all three expected
fixtures from `/api/fixtures`. Each NATS store was about 149 MB. A plaintext marker occurred 33,381 times in the FHIR
log and zero times across all three JetStream stores; NATS also logged `Encryption: ChaCha20-Poly1305`. This supports,
but does not replace a security audit of, encryption at rest. All emulation processes were stopped afterward.

The separate normal and approximately 10× scale measurements, including the x10 safety failures and annual-volume
storage extrapolation, are in [`pipeline/results/scale/`](../pipeline/results/scale/).

## Not production-ready

- **Not executed under Docker.** Image build, compose networking, healthchecks and volume permissions are untested.
- **Demo entrypoint only.** There is no mode that ingests real feeds without the generator (see above), and the
  provider table and code mappings are the demo's.
- **One container for all pipeline services.** They talk only through NATS and their run directory, but the
  orchestrator supervises them as processes; one container per service (or a scheduler such as Kubernetes) needs a
  shared run directory or status in NATS KV instead of files.
- **No TLS or authentication** on NATS, MLLP or the API. NATS needs TLS and accounts/nkeys between nodes and clients;
  MLLP needs a VPN or TLS wrapper; the API needs authentication and role-based access (the API's review action is a demo).
- **Key management** is an environment variable recorded in Docker host container configuration. A deployment needs
  host-secret controls, KMS/HSM injection and a key-rotation procedure.
- **NATS containers run as the image's default user** and NATS limits (4 GB file store per node) are the demo's;
  size them from the scale results in [`pipeline/results/scale/`](../pipeline/results/scale/).
- **No backups, monitoring or alerting** (the SLOs in `WRITEUP.md` are a design, not implemented here).
- **The reader is the cached one.** A live local reader (Gemma 3 4B on llama-server) is not part of the package.
