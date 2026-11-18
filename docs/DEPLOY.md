# Deploying Wasl on a provider network's own servers

This is the package for the "second door" in [`WRITEUP.md`](WRITEUP.md): a provider network runs Wasl on its own
servers to track its own patients across its own facilities. It is a demo-grade package: it brings up the same
system that the integrated runs in [`pipeline/RESULTS.md`](../pipeline/RESULTS.md) use, on synthetic data. It is not
production-ready; the gaps are listed at the end. For the separate production-shaped authenticated TLS, file-backed
secret, persistent-message backup/restore, and reversible file-migration path, use [`RECOVERY.md`](RECOVERY.md). Its
runbook includes actual isolated clean-restore evidence and the remaining verification limits.

## One command

```bash
scripts/deploy_up.sh            # = random environment key + docker compose up --build
```

This builds the image, starts a 3-node NATS JetStream cluster, runs the demo cohort end to end (with kill/restart
chaos) and then serves the read API on `http://127.0.0.1:8088` ([`API.md`](API.md)). Check it with
`curl 127.0.0.1:8088/healthz` and `curl -H 'Authorization: Bearer wasl-demo-reviewer-1-token' 127.0.0.1:8088/api/fixtures`. Every `/api` route requires a role-bound bearer token; the published value here is only the loopback synthetic-demo default, not a production secret. The run summary is the last output of the
`pipeline` container (`docker compose logs pipeline`).

`scripts/check_deploy.py` checks the package statically without Docker (compose references, non-root image, key
handling, NATS config baked into `wasl-nats:local` with no host bind mount, every `COPY` source present and not excluded by `.dockerignore`). It and the static tests in `scripts/test_deploy.py` need PyYAML; without it they exit or error with a clear message and never report a pass.

## What runs

| Service | Image | What it does | Lifetime |
|---|---|---|---|
| `nats-0`, `nats-1`, `nats-2` | `wasl-nats:local` (`deploy/nats/Dockerfile`: NATS 2.12.6 rebuilt with Go 1.27.4 and patched modules on `nats:2.12.6-alpine`, with Alpine upgrades and `nats.conf` copied in) | JetStream cluster from [`deploy/nats/nats.conf`](../deploy/nats/nats.conf): file store, **ChaCha20-Poly1305 encryption at rest**, streams with 3 replicas (Raft quorum writes); runs as UID 10001 | long-running |
| `pipeline` | `wasl:local` (this repo's `Dockerfile`) | `scripts/demo.sh container`: generate the synthetic cohort, run every pipeline service against the cluster (mock HIE, FHIR subscription consumer or HL7 MLLP listener, 3 prep workers, linker, 4 closure partitions, simulated reviewer), then audit, tamper check, booking and the summary | one-shot, exits 0 |
| `api` | `wasl:local` | `scripts/demo.sh serve`: the read API over the finished run | long-running, starts after `pipeline` succeeds |

The pipeline services run as supervised processes inside the `pipeline` container. The orchestrator restarts a
crashed service and performs the demo's chaos kills (linker, closure, prep, the FHIR consumer). It does not start or
kill NATS nodes when the cluster is external (`WASL_NATS_EXTERNAL=1`), so the demo's NATS-node kill is skipped in
this package; use `docker compose kill nats-2 && docker compose start nats-2` during a run to test it.

The image is multi-stage: a static Go build (`CGO_ENABLED=0`), then `python:3.12-alpine` with numpy and scipy for the
cohort augmentation, booking and summary scripts. Alpine updates, Bash and util-linux are installed; the unused pip/ensurepip toolchain is removed after wheel installation. It runs as uid 10001. It contains the committed synthetic inputs and
JSON model weights only: no keys, no patient data, no run outputs.

## Offline rebuild with preseeded dependencies

Go modules are tracked in `pipeline/vendor`; the build uses `GOFLAGS=-mod=vendor` and `GOPROXY=off`, never `go mod download`. After changing `pipeline/go.mod` or `go.sum`, regenerate and review that directory with `go -C pipeline mod vendor`. `scripts/test_offline_build.py` checks it for drift.

```bash
podman build --tag wasl:local .                # one online build to seed the runtime image
scripts/offline_build.sh                       # full rebuild with networking disabled
```

The second command creates a fresh `/tmp` build context (including on macOS, where the Podman VM cannot read the default `$TMPDIR`), includes only tracked files plus the vendor directory, requires the online-seeded `wasl:local` runtime image, refuses base-image pulls and passes `--network=none`. It starts from that image with `OFFLINE_RUNTIME=1`, verifies the required Python packages, Bash, `flock` and UID 10001, deletes inherited `/app` and run-output contents, and only then copies the checkout. No Alpine or Python package command runs. It changes `OFFLINE_BUILD_ID` so the Go vet/compile layer actually reruns rather than succeeding only from cache; that layer also fails if it can open a TCP connection to the Go module proxy. A no-cache build on a new or empty-cache host is **not** an offline build. Docker Engine uses the same runner after its ordinary online CI build: `WASL_CONTAINER_ENGINE=docker scripts/offline_build.sh`. Set `WASL_RUNTIME_IMAGE` to use a different preseeded runtime tag. This seeded path covers only the Wasl pipeline/API image; use the clean build below for both images.

## Clean offline build (no layer cache, no network)

```bash
WASL_CONTAINER_ENGINE=podman scripts/offline_build_clean.sh                      # Podman
DOCKER_BUILDKIT=1 WASL_CONTAINER_ENGINE=docker scripts/offline_build_clean.sh    # Docker Engine
```

This builds both the Wasl image (`wasl-offline-clean:local`) and the NATS image (`wasl-nats-offline-clean:local`) from a fresh `/tmp` context with `--no-cache --network=none`, `OFFLINE_PACKAGES=1`, `OFFLINE_RUNTIME=0` and a new `OFFLINE_BUILD_ID`, using `--pull-never` (Podman) or `--pull=false` (Docker). No seeded runtime image is used. Dependencies come from tracked bundles: `pipeline/vendor`, `deploy/nats/build/vendor`, `deploy/offline/{apk,wheels}/<arch>` and `deploy/nats/offline/apk/<arch>`, mounted at build time from scratch stages and hash-checked before install. The runner then runs `wsi7 -h` and `nats-server -v` with overridden entrypoints and checks that both final images run as UID 10001.

Requirements and limits:

- **BuildKit is required on Docker** (`--mount=type=bind,from=...` in both Dockerfiles). Podman/Buildah supports it natively.
- **Base images must already be loaded**: `golang:1.27.4-bookworm`, `python:3.12-alpine` and `nats:2.12.6-alpine`. The runner refuses to pull and exits if one is missing. Their Alpine releases must match the bundles (Wasl 3.24.*, NATS 3.22.*) or the build fails closed.
- **Clean vs seeded**: the clean build has no layer cache and no runtime seed. The older `scripts/offline_build.sh` path above reuses an online-built runtime image (`OFFLINE_RUNTIME=1`) and is faster but weaker; it is not the clean-build proof.
- **Not an empty-host claim**: base-image archives are out of scope, so a host with no images cannot do this.
- **Bundle maintenance**: bundles are pinned to their generation date and to linux/amd64 and linux/arm64. Regenerate them online with the generation scripts, review the SHA-256 manifests, and regenerate for security updates or a new architecture; stale bundles are not updated by the offline build.

## Ports and volumes

| | Where | Notes |
|---|---|---|
| API | `127.0.0.1:8088` on the host (`WASL_API_PORT` to change) | loopback only; put a TLS reverse proxy with authentication in front for anything else |
| NATS client 4222, cluster 6222, monitor 8222 | compose network only | not published to the host |
| Mock HIE, FHIR rest-hook, HL7 MLLP | `127.0.0.1` inside the `pipeline` container | demo traffic only |
| `nats-0`, `nats-1`, `nats-2` volumes | `/data/jetstream` | encrypted JetStream stores; unreadable without `WASL_JS_KEY`; new named volumes are writable by NATS UID 10001 |
| `wasl-run` volume | `/var/lib/wasl` | cohort and run outputs (`run/<policy>-cached-<source>/`): logs, decisions, `report7.json`, review queue. The API reads it |

## Configuration

- `WASL_JS_KEY` (required): the JetStream encryption key, passed to the three NATS containers only. It must start with
  a letter. `scripts/deploy_up.sh` makes a random one if it is unset; the script does not write it to the repository or
  image. Docker does store container environment configuration on the Docker host, so protect that host and use suitable
  secret/KMS controls in production. Losing the key makes retained stores unreadable.
- **Retained volumes need the original key.** If this Compose project's `nats-*` volumes exist and `WASL_JS_KEY` is
  unset, `deploy_up.sh` refuses to generate an incompatible key. Supply the original key to keep the data. To discard
  this deployment's stores and run data intentionally, run `docker compose down -v`, then rerun `deploy_up.sh`.
  When upgrading volumes created by the former root-running NATS image, first make each retained store writable with
  `docker compose run --rm --no-deps --user root --entrypoint /bin/chown <nats-node> -R 10001:10001 /data`; keep the
  normal runtime non-root.
- `WASL_SEED`, `WASL_POLICY` (`v7strict`, `v7ltt`, `v6`), `WASL_SOURCE` (`fhir`, `hl7`), `WASL_LUNG`, `WASL_DR`,
  `WASL_BREAST`: as in `scripts/demo.sh`. The reader is the cached one (no model server in the package).
- `docker compose down` keeps the volumes; `docker compose down -v` deletes the stores and the run. Each new pipeline
  run recreates `FHIR`/`PREP`/`FACTS`/`OUT` and deletes the `SEEN`, `FHIRSUB` and `FHIRPOLL` KV buckets before services
  start. Thus a new cohort cannot silently inherit replay suppression, subscription/polling checkpoints or EOF state;
  service restarts within that run still resume from their checkpoints. Setup fails if reset fails.
- `WASL_NATS_URL` and `WASL_NATS_EXTERNAL=1` must be set together. Either setting alone is rejected by the Go commands
  and `scripts/demo.sh`; with neither set, the demo uses its local three-node cluster.
- `WASL_API_TOKENS` (required by direct `wsi7 serve`) contains comma-separated `token:role:actor` entries;
  `WASL_SYNTHETIC_DATA_ONLY=1` is a mandatory acknowledgment. `scripts/demo.sh` supplies documented loopback-demo
  defaults and separate `WASL_API_*_TOKEN` client variables. Override both together. CORS is off unless exact origins
  are listed in `WASL_API_ORIGINS`. See [`SECURITY.md`](SECURITY.md) before any non-demo use; this static-token API is
  not approved for real clinical data.
- `WASL_RECEIVING_UNIT_CLINICIANS` and `WASL_REFERRING_CLINICIANS` name which token actors may answer a CRC referral.
  Each is a comma-separated list of `scope:actor` entries, where `actor` is the actor a `WASL_API_TOKENS` entry binds.
  `WASL_RECEIVING_UNIT_CLINICIANS` maps a receiving unit to a clinician (for example `skmc-endo:hosted-unit-clinician`),
  who may accept or return referrals with `POST /referral-response`. `WASL_REFERRING_CLINICIANS` maps a referring
  provider to its GP (for example `ahs-mushrif:hosted-referrer`), who may answer a returned referral with
  `POST /referrer-response`. A referrer answers every returned packet of its provider, including the canonical lung
  work-up packets of `rad-imaging` and DR eye-clinic packets of `phc-retina` (for example
  `ahs-mushrif:hosted-referrer,rad-imaging:hosted-lung-referrer,phc-retina:hosted-eye-referrer`); one actor maps to one
  provider. The two lists must not share an actor. The same referring-clinician actor may acknowledge its provider's
  outcome hand-off (`referrer_acknowledged`, docs/API-OUTCOME-HANDOFF.md), so a demo needs no separate `referrer`-role
  token for it; a `token:referrer:referrer/<provider>` entry in `WASL_API_TOKENS` still works. A referring clinician may
  write only to its own provider's cases (other providers' case writes return 403); reads are not provider-scoped. Left empty, both endpoints refuse every answer with
  403. `scripts/demo.sh` assigns its clinician token's actor (`smoke-clinician`) to `skmc-endo` and assigns no referrer.

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

**Observed, 6 Nov 2026 (Podman 5.8.2 machine, rootful service, Docker Compose v5.1.4 as the `podman compose`
provider; not Docker Engine, which remains unverified).** An isolated project with its own named volumes and a generated
valid `WASL_JS_KEY` was built and brought up with `podman compose -p <unique> --env-file <file> up -d`, API on a
non-default loopback port:

- `wasl:local` and `wasl-nats:local` built. NATS config is copied into the image (`deploy/nats/Dockerfile`) because a
  bind mount of `nats.conf` failed to read under the Podman VM, which did not share the checkout directory.
- `nats-0/1/2` reached healthy; `pipeline` exited 0; `api` reached healthy and ran as UID 10001.
- `/healthz` returned `status: ok` with `dependencies.nats: up`, `materialised_run: loaded`, 2100 cases,
  `data_mode: simulated`. The API was the only published port, bound to `127.0.0.1`.
- The pipeline log showed the audit, tamper/forgery checks (forged events with effect 0/18), simulated booking and the
  summary (CRC with review P 1.000, R 0.837, 0/474 false closures; one seeded run, simulated, not a new headline).
- Final run on the combined source (API, smoke and deployment work merged), fresh project, port 18817: both images built, the
  three `nats` services healthy, `pipeline` exit 0, `api` healthy as UID 10001, `/healthz` with `dependencies.nats: up`,
  and `python3 scripts/api_smoke.py --base-url http://127.0.0.1:18817` passed against the container over real HTTP
  (2100 cases, 3 fixtures). The pipeline log again showed audit, FHIR tamper lines (forged events with effect 0/18,
  provenance 912/912), simulated booking, and summary (CRC with review R 0.837, 0/474 false closures; exactly-once
  duplicates 0 / 25188). The isolated project was removed with `down -v`; only that project was touched.
- `scripts/check_deploy.py` now also rejects long-syntax (`type: bind`) host mounts, covered by `scripts/test_deploy.py`.
- Podman retains container environment configuration on the VM host, so the JetStream key is not kept out of the host.

**Observed, 7 Nov 2026: retained-volume restart (same Podman 5.8.2 / Compose v5.1.4 setup, isolated projects `waslrestart7`
and `waslrestart8`, loopback port 18931, generated valid key).**

- First run, unmodified source (`waslrestart7`): `up -d --build` gave healthy NATS, `pipeline` exit 0, healthy API,
  `api_smoke.py` passed (2100 cases, 3 fixtures); `report7.json` had 0 duplicate outputs, 0 lost, 0 false closures in
  every wasl lane (CRC with review R 0.837, 0/474; cervical R 0.967, 0/204). `podman compose down` (no `-v`) kept all four
  volumes. **Defect observed:** a following `up` with a different valid-looking key started all three NATS nodes with
  `Error decrypting our stream metafile: unable to recover keys`, then `Resetting stream cluster state` and
  `detected duplicate assignment` for FHIR/PREP/FACTS/OUT/KV_*: nats-server does not refuse, so a wrong key lets the
  cluster discard the retained streams. (A missing key was already refused by Compose interpolation.)
- Fix: `deploy/nats/entrypoint.sh` (the `wasl-nats:local` entrypoint) refuses to start (exit 1, key never printed) on an
  empty key, an unhashable key, or a key whose domain-prefixed SHA-256 differs from `/data/.wasl-key-check`. On a fresh
  volume the marker is written (umask 077, temp file then `mv`) only after JetStream reports healthy, so a failed first
  start, for example an all-digit key that nats-server parses as an out-of-range number, leaves no marker and the corrected
  key starts normally. The script waits on the same nats-server process, forwards TERM/INT and keeps its exit code and
  logs. Volumes created before this check have no marker; they start with a message and no marker is written, since the key
  cannot be verified. Covered by tests in `scripts/test_deploy.py` (failed first start, empty hash, marker mode and
  atomicity, signal forwarding) and static checks in `scripts/check_deploy.py`. No key safeguard was weakened; the key is
  still not in the repository or image.
- Legacy caveat: the marker is written by a poller that checks JetStream health about once per second. A crash between
  JetStream becoming healthy and the marker write (including the gap before the next poll) can leave a volume with retained
  data and no marker. The next start treats that volume as legacy/unverified: it starts with the "no key marker" message and
  does not get wrong-key refusal. The key check is therefore not guaranteed for such a volume; it is not shown to be
  exercised on a real crash.
- Fix-round verification: a malformed all-digit first start left no marker; the corrected key then
  became healthy and wrote a 0600 marker; a wrong key after a valid marker was refused with no decryption or reset warnings;
  and a same-key down/up passed API smoke (2100 cases) with 0 false closures.
- Scope of the restart: what is shown is that a restart with the correct key recovers the retained stores without
  decryption warnings, and that a wrong key is refused. It does not show that retained messages were read back. The
  `pipeline` service is one-shot and, by design, resets and re-runs the cohort on every `up` (see Configuration), so
  the second run's 2100 cases are a fresh cohort rerun on the retained stores, not a resumed one. Run outputs in
  `wasl-run` are overwritten.

Direct Podman path: `podman compose --env-file <file with WASL_JS_KEY=k<64 hex>> -p <project> up -d --build`.
`scripts/deploy_up.sh` remains Docker-only. Cleanup of the isolated project is `podman compose -p <project> down -v`.

Before that, Docker had **not** been run on the build machine. Static and emulated checks:

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

## Hosted frontend (Vercel)

The Next.js app in `web/` reads the API from its **server** (React Server Components and server actions), never from
the browser, so the API needs a public HTTPS URL but no CORS changes. Vercel cannot run the Go API or NATS: run this
package on any host with Docker, put an HTTPS endpoint in front of the loopback port, and point Vercel at it.

**Status: designed, not verified.** This section has not been run end to end against a hosted deployment.

Every `/api` route needs a bearer token that the API binds to one role and one actor (`WASL_API_TOKENS`, see
[`SECURITY.md`](SECURITY.md)). The published `scripts/demo.sh` defaults are loopback-only constants: a publicly
reachable API must not use them, so generate fresh tokens for each role and independent reviewer credential.

1. On the API host, generate one token per credential the frontend uses, bind each to a role and the actor the ledger should
   record, and start the package detached, then wait for the API:

   ```bash
   umask 077
   for r in reviewer reviewer-2 reviewer-3 clinician clinician-2 clinician-3 auditor coordinator unit-clinician referrer; do openssl rand -hex 32 > "wasl-$r.token"; done
   export WASL_API_TOKENS="$(cat wasl-reviewer.token):reviewer:hosted-reviewer-1,$(cat wasl-reviewer-2.token):reviewer:hosted-reviewer-2,$(cat wasl-reviewer-3.token):senior_reviewer:hosted-reviewer-3,$(cat wasl-clinician.token):clinician:hosted-clinician,$(cat wasl-clinician-2.token):clinician:hosted-clinician-2,$(cat wasl-clinician-3.token):senior_clinician:hosted-clinician-3,$(cat wasl-auditor.token):auditor:hosted-auditor,$(cat wasl-coordinator.token):coordinator:hosted-coordinator,$(cat wasl-unit-clinician.token):clinician:hosted-unit-clinician,$(cat wasl-referrer.token):clinician:hosted-referrer"
   export WASL_RECEIVING_UNIT_CLINICIANS="skmc-endo:hosted-unit-clinician" WASL_REFERRING_CLINICIANS="ahs-mushrif:hosted-referrer"
   scripts/deploy_up.sh -d                            # detached: does not attach to the logs
   until curl -sf 127.0.0.1:8088/healthz >/dev/null; do sleep 10; done   # the pipeline runs first (~5 min)
   curl -s -o /dev/null -w '%{http_code}\n' 127.0.0.1:8088/api/dashboard   # 401: no token
   curl -s -H "Authorization: Bearer $(cat wasl-reviewer.token)" 127.0.0.1:8088/api/dashboard | head -c 200
   ```

   `docker-compose.yml` passes `WASL_API_TOKENS`, `WASL_RECEIVING_UNIT_CLINICIANS` and `WASL_REFERRING_CLINICIANS` to the `api` service. Left unset or empty, `scripts/demo.sh serve`
   falls back to the published demo tokens, which is only acceptable on loopback. Each token needs at least 16
   characters; actors are the names the ledger records, so choose ones that identify who operates each credential.
   `/healthz` is the only unauthenticated route. Keep the token files out of the repository.
   The command above already binds the third reviewer and clinician tokens to `senior_reviewer` / `senior_clinician`
   (credentials 1 and 2 stay `reviewer` / `clinician`), each with a different actor. Two tokens bound to the same actor are not independent and the API refuses
   the second read. Reviewers sign in on the website with their own assigned bearer token (HttpOnly cookie, HTTPS required). This bearer-token form is the synthetic-demo path.
   `scripts/production_secrets.sh` generates, for the Basic-auth production package, `api_users.json` entries `reviewer`, `reviewer-2`,
   `senior-reviewer`, `clinician`, `clinician-2`, `senior-clinician`, `coordinator` and `auditor`, each with its own
   random password, without printing them. These credentials identify configured actors, not separate humans; there is
   no OIDC. **Limitation:** the Basic-auth production package exposes independent review through its API only; the web sign-in form accepts bearer tokens (or, with `WASL_REVIEW_AUTH_MODE=oidc`, only OIDC sign-in, which disables the token form), so those Basic passwords do not work in it and there is no production web path for independent review yet. The smoke (`scripts/demo.sh smoke`) exercises the protocol only on synthetic data.
2. Expose `127.0.0.1:8088` over HTTPS, for example `caddy reverse-proxy --from api.example.org --to 127.0.0.1:8088`
   on a host with a DNS name, or a tunnel (`cloudflared tunnel --url http://127.0.0.1:8088`) for a short demo.
3. In the Vercel project (root directory `web`), set these environment variables for Production (and Preview if
   wanted), then redeploy. All are server-only: never prefix any of them with `NEXT_PUBLIC_`.

   | Variable | Value | Used for |
   |---|---|---|
   | `WASL_API_URL` | the public HTTPS URL from step 2, no trailing path | every request |
   | `WASL_API_WEB_REVIEWER_TOKEN` | a token bound to a reviewer-role actor that is **not** an assigned reader | dashboard, lists and case detail on shared pages; never an r1/r2 credential |
   | `WASL_API_REVIEWER_TOKEN` | contents of `wasl-reviewer.token` | only the `postReview` helper, which no page calls; for review or governance, issue it to a person, who signs in with it |
   | `WASL_API_REVIEWER_2_TOKEN` | contents of `wasl-reviewer-2.token` | not read by the web server; issue it to a person, who signs in with it |
   | `WASL_API_REVIEWER_3_TOKEN` | contents of `wasl-reviewer-3.token` | not read by the web server; the senior_reviewer signs in with it for r3 and contest resolution |
   | `WASL_API_CLINICIAN_TOKEN` | contents of `wasl-unit-clinician.token`, whose actor the API lists in `WASL_RECEIVING_UNIT_CLINICIANS` | the receiving unit's Accept and Return answers on `/app/referrals` (`POST /referral-response`), and the `postReview` helper, which no page calls. Every visitor who can open the site answers as this actor, so bind it to a dedicated unit-clinician actor, not to a person's review credential; built, synthetic ledger only |
   | `WASL_API_CLINICIAN_2_TOKEN` | contents of `wasl-clinician-2.token` | not read by the web server; issue it to a person, who signs in with it |
   | `WASL_API_CLINICIAN_3_TOKEN` | contents of `wasl-clinician-3.token` | not read by the web server; the senior_clinician signs in with it for r3, contest resolution and patient-contest resolution on the case page (#167) |
   | `WASL_API_AUDITOR_TOKEN` | contents of `wasl-auditor.token` | the redacted audit view |
   | `WASL_API_COORDINATOR_TOKEN` | contents of `wasl-coordinator.token` | Today's coordinator actions (`POST /actions`) |
   | `WASL_API_REFERRER_TOKEN` | contents of `wasl-referrer.token`, whose actor the API lists in `WASL_REFERRING_CLINICIANS` and not in `WASL_RECEIVING_UNIT_CLINICIANS` | the referring GP's answers on `/app/referrals/returned` (`POST /referrer-response`); built, synthetic ledger only |
   | `WASL_API_REFERRER_LUNG_TOKEN` | optional; a token whose actor `WASL_REFERRING_CLINICIANS` assigns to `rad-imaging` | the lung work-up referrer's answers on `/app/referrals/returned?programme=lung_nodule&provider=rad-imaging`; without it those answers are refused with a configuration message; built, synthetic ledger only |
   | `WASL_API_REFERRER_DR_TOKEN` | optional; a token whose actor `WASL_REFERRING_CLINICIANS` assigns to `phc-retina` | the eye-screening referrer's answers on `/app/referrals/returned?programme=diabetic_retinopathy&provider=phc-retina`; built, synthetic ledger only |

   Without `WASL_API_URL` the site serves the committed snapshots (`web/data/api`) and review/Today writes stay in
   the browser.

   To sign reviewers and clinicians in through your identity provider instead of typed tokens, also set these
   (details and the full mapping rules in [`web/README.md`](../web/README.md#reviewer-sign-in-modes)). OIDC mode turns
   the token form off: the sign-in card shows only **Sign in with OIDC**. The table above still applies:
   `WASL_API_WEB_REVIEWER_TOKEN` and `WASL_API_AUDITOR_TOKEN` are required, and each mapped credential's
   `WASL_API_*_TOKEN` must be set on the web server, which uses it on behalf of the signed-in subject.

   | Variable | Value | Used for |
   |---|---|---|
   | `WASL_REVIEW_AUTH_MODE` | `oidc` (default `demo`, the token form) | turns OIDC sign-in on and token entry off |
   | `WASL_OIDC_ISSUER` | the provider's exact HTTPS issuer URL | discovery and ID-token `iss` checks |
   | `WASL_OIDC_CLIENT_ID` | the confidential client registered for Wasl | authorization and token requests |
   | `WASL_OIDC_CLIENT_SECRET` | that client's secret | token request (client_secret_basic) |
   | `WASL_OIDC_REDIRECT_URI` | `https://<site>/app/auth/oidc/callback`, registered exactly with the provider | the callback |
   | `WASL_OIDC_SUBJECT_MAP_JSON` | JSON object from provider subject to one of `reviewer`, `reviewer_2`, `reviewer_3`, `clinician`, `clinician_2`, `clinician_3` | which server-held credential a verified subject acts with |
   | `WASL_OIDC_SESSION_STORE` | `sqlite` (default) or `nats` | where login transactions and sessions live (below) |
   | `WASL_OIDC_SESSION_DB` | path to a server-only SQLite file | the `sqlite` store |
   | `WASL_OIDC_INSECURE_LOOPBACK`, `WASL_SYNTHETIC_DATA_ONLY` | leave unset in production; `1` and `1` allow plain HTTP to a loopback provider for synthetic tests only | local testing |

   A failed sign-in shows a generic notice in the browser and logs one `[wasl-oidc] … refused: category=…` warning
   on the web server (for example `discovery`, `config`, `token_exchange`, `unmapped_subject`, `session_store`), so a
   misconfigured deployment can be diagnosed without exposing tokens or subjects.

   With `WASL_REVIEW_AUTH_MODE=oidc`, login transactions and review sessions are stored server-side. The default SQLite
   store (`WASL_OIDC_SESSION_DB`) serves one web instance only; it is not shared between Vercel function instances.
   Run more than one web replica only with `WASL_OIDC_SESSION_STORE=nats`, which stores them in JetStream key-value
   buckets on this package's NATS cluster over TLS with a NATS user (variables and limits in
   [`web/README.md`](../web/README.md#session-storage)). The NATS client port must then be reachable from the web
   servers, which `docker-compose.yml` does not publish. **Status:** built and verified with two web replicas against a
   single loopback JetStream server on synthetic data; not verified against the three-node TLS cluster, behind a load
   balancer, on Vercel or with a live identity provider.

The tokens authenticate the frontend **server**, not the person using the site. The API ignores client-supplied
`X-Wasl-Role`/`X-Wasl-Actor` headers and records the actor bound to the token that the frontend server selected for the
action, so every Today action is recorded as the coordinator token's actor. Anyone who can use the hosted site can
therefore act with that identity on the synthetic run: protect the Vercel deployment (Vercel Authentication or a
password). Independent review reads and undo, contest and contest resolution are different: they act only as the token
a person typed into the page's sign-in form (one HttpOnly cookie, one identity per browser), never a server-held one,
and the page offers no credential choice. Their independence still rests on separate sign-ins with distinct tokens that
the operator issued to distinct people. Review and coordinator events are appended to
`api_reviews.ndjson` on the `wasl-run` volume and survive an API restart; a new pipeline run that changes case heads
makes the old ledger invalid (see `API.md`). To rotate a credential, change both `WASL_API_TOKENS` on the API host
(then rerun `scripts/deploy_up.sh -d` with the original `WASL_JS_KEY` exported) and the matching Vercel variable.

## Azure

Observed on 17 Nov 2026 in one Azure for Students subscription, region `japaneast`. The image was built locally from
a checkout of commit `4bfd661` with LF line endings, and tagged `4bfd661c-lf`. The live API returned 401 for every
published `wasl-demo-*` token. The runbook
is [`deploy/azure/README.md`](../deploy/azure/README.md). Synthetic data only.

**The model stays small and local by design.** Wasl's language model is Gemma 3 4B, the quantized file
`gemma-3-4b-it-Q4_K_M.gguf` that the reader already runs through `llama-server`. A hospital runs that file on its own
machine, so patient text never reaches a cloud-hosted model. The hosted demo runs the identical file on a rented Modal
GPU that stands in for that machine and receives only synthetic data. No frontier model is in any request path.

| Part | Label | What was verified |
|---|---|---|
| `wsi7 serve` API on Azure Container Apps | **built** | `deploy/azure/verify_api.py` passed against the Container Apps URL. `/healthz` reported 2,148 cases with `nats: not_required_for_read_api`. Calls without a token got 401, and `/api/fixtures` returned 200 with a fresh reviewer token. A coordinator note returned 201. After a revision restart, a replica created after the write still served the note, and the note's line was read back from the Azure Files share. |
| Pipeline run that produced the API's data | **built, local** | It ran on local Docker with the 3-node NATS cluster and exited 0. The run directory was then copied to Azure Files. **The pipeline did not run on Azure.** |
| Gemma 3 4B on Modal (`deploy/modal/gemma_server.py`) | **built** | 18 Nov 2026: `llama-server` (llama.cpp `server-cuda12-b12030`) on one T4 serves `ggml-org/gemma-3-4b-it-GGUF` at a pinned revision, and the download checks the file's SHA-256. Model routes returned 401 without the key or with a wrong one, and 200 with it. One sample each: a cold request after a fresh deploy took 135 s including the image pull, a warm one 1.3 s, and generation ran at 50.6 tokens per second. The cached reader answers and the published results are unchanged; no metric uses this server. |
| `gpt-4.1-mini` on Foundry | **built, unused** | Deployed on 17 Nov for the agent's first version. Since 18 Nov nothing calls it; kept on request. |
| `wasl-audit-explainer` Foundry hosted agent | **built, demo** | One end-to-end call was observed. The agent read the auditor token from Key Vault, sent `GET /api/cases/{id}/audit` to the Azure API, and called the model. The API log shows only GETs from that actor. On 17 Nov, with `gpt-4.1-mini`, the answer was fluent but contained a false statement about the 31-day window. On 18 Nov the agent was switched to Gemma 3 4B on Modal, reading the server key from Key Vault; one end-to-end call returned in 12 s with the chain length, verification and 11 matching signatures stated correctly. Agent output is unvalidated and is not a clinical or audit finding. |
| Vercel web app on the Azure API | **built** | 18 Nov 2026: production redeployed with `WASL_API_URL` and the server tokens. Pages showed the Azure run's 2,148 cases, and the API log recorded the dashboard, case-list and case-detail reads as `hosted-web-reviewer`. The production alias is public by decision, so judges can open it (see the runbook). |
| Continuous deployment (`.github/workflows/deploy.yml`) | **built, not yet run on GitHub** | actionlint 1.7.10 and `scripts/check_action_pins.py` pass. The agent job's deploy script and the API job's roll-and-smoke script were run locally with the same commands. They produced agent version 3 and API revision 4. The workflow's first GitHub run needs the one-time variables and the Vercel token (runbook, "Continuous deployment"). The web job's `vercel build` on a Linux runner was not rehearsed. On 18 Nov a Model job was added for `deploy/modal/`; its deploy and its health and 401 checks were run by hand, not by the workflow. It also needs a Modal API token. |
| The Go live readers against the Modal server, NATS on Azure, pipeline as a Container Apps job, Entra ID sign-in for the web app | **designed** | Not attempted. The Go readers send no auth header, which the Modal server requires. Student subscriptions have no Azure GPU quota, which is why the GPU is on Modal. |

Problems found on the way, each fixed in the deploy rather than in Wasl code:

- **CRLF checkout.** With `core.autocrlf=true` on Windows, every `*.sh` file was checked out with CRLF endings. Container
  entrypoints then failed with `no such file or directory`. `.gitattributes` now forces LF for `*.sh`, `*.py`, `*.conf`
  and `Dockerfile`.
- **Express environments.** Container Apps environments created without a VNet came out as "express" environments, which
  refuse Azure Files volumes. The environment now runs on its own VNet.
- **Ledger chmod on SMB.** The ledger append chmods `api_reviews.ndjson` to 0600. On a default Azure Files mount every
  file belongs to root, so the chmod by UID 10001 failed with EPERM and every write returned 500. Taking the lock itself
  worked. The share is now mounted with `uid=10001,gid=10001,file_mode=0600,dir_mode=0700`.
- **Restart overlap.** A revision restart starts the new replica before the old one stops, so for a short time two
  processes share the ledger. The `flock` and the ledger re-check under the lock (`appendEventLocked`) are meant for
  this case. No concurrent write was attempted during the overlap, so that case is **not tested** on Azure Files.
- **Unknowns.**
  - The agent's identity still holds Foundry User and Cognitive Services OpenAI User on the Foundry account from its
    `gpt-4.1-mini` version. The Gemma version needs neither; they were not removed.
  - The first agent version failed model authentication even after the roles were granted. A redeploy fixed it, and a
    token cached from before the roles is the likely reason. This is **inferred**, not tested.

## Not production-ready

- **Verified under Podman + Docker Compose provider only.** Docker Engine and other hosts are untested. A retained-volume restart is checked under Podman only (7 Nov, above): a correct-key restart recovers the stores without decryption warnings and a wrong key is refused; retained messages are not shown to be read back, and the pipeline reruns a fresh cohort. The key marker script is not exercised on Docker Engine.
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
- **No centralized monitoring or alerting** (the SLOs in `WRITEUP.md` are a design, not implemented here). The separate recovery profile adds authenticated backup/restore; its storage encryption, RPO/RTO, and second-machine limits are documented in `RECOVERY.md`.
- **The reader is the cached one.** A live local reader (Gemma 3 4B on llama-server) is not part of the package.
