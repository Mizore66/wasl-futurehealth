# Production-shaped backup, restore, and rollback

## Scope and trust boundary

This package adds a **production-shaped recovery path for the synthetic Wasl run**. It is not a claim of production
certification, a physical second-machine test, or a real-feed entrypoint. The existing demo deployment remains available
in `docker-compose.yml`; the recovery path is `deploy/production.compose.yml`.

Built controls:

- JetStream stream configuration, durable consumer configuration, KV checkpoints, and every retained message are backed
  up. Restore is refused unless the target JetStream has zero streams. Each restored stream is read back and compared
  using a logical SHA-256 over retained order, subjects, application headers, and payloads.
- The file bundle contains the materialised run, cohort, anchors, and API review ledger. HMAC-SHA-256 authenticates the
  manifest and every payload hash with a key held outside the bundle. Verification finishes before restore contacts
  NATS or creates run files.
- File replacement stages and fsyncs a complete generation, retains the previous generation, and writes a rollback
  journal. `rollback` swaps the previous generation back without deleting the rejected generation.
- NATS clients and routes use authenticated mutual TLS. The API uses HTTPS and Basic credentials from a file-backed
  user/role map; it overwrites caller-supplied role and actor headers with the authenticated identity. Anonymous API
  requests fail. CORS is disabled unless one explicit origin is configured.
- NATS and Wasl containers both run as UID 10001 (each image's USER; the NATS image owns `/data` as that UID), with
  capabilities dropped, no-new-privileges, private tmpfs, and JSON-file log rotation. Go API/recovery logs are JSON
  records. Secret values are never command-line arguments or log fields. The runtime route config is mode 0600 in a
  private tmpfs.
- Destructive cohort generation exists only behind the explicit `seed` profile. Ordinary `up`, `restart`, and recovery
  commands never regenerate the cohort or delete streams/run files.

Remaining boundaries are listed at the end; in particular, bundles are authenticated but **not encrypted** by this tool.
Keep the backup and secret volumes in separate protected storage. `comparison_sha256` excludes `meta.loaded_at`;
`content_sha256`, load times, broker physical metadata, and consumer positions are not expected to be restart-stable.

## Prerequisites and isolated setup

Observed with Podman 5.8.2 client / 5.8.9 server and Docker Compose v5.1.4 as the `podman compose` provider. Docker
Engine is not verified. Run from the repository root. Every name below is an example: use a new unique suffix for each
drill, and never reuse another deployment's project or volumes.

```bash
export ROOT="$PWD"
export COMPOSE_FILE="$ROOT/deploy/production.compose.yml"
export SOURCE=waslrecsrc-unique
export TARGET=waslrecdst-unique
export SECRETS=wasl-recovery-unique-secrets
export BACKUP=wasl-recovery-unique-backup
export DRILL="$ROOT/out/recovery-unique"
mkdir -p "$DRILL"

scripts/production_secrets.sh "$DRILL/secrets"
scripts/production_volume_load.sh "$SECRETS" "$DRILL/secrets"
podman volume create "$BACKUP"
podman run --rm --user 0:0 -v "$BACKUP":/backup \
  docker.io/library/alpine:3.23 chown 10001:10001 /backup

# Every Compose invocation is scoped to one explicit project and deployment-specific external volumes.
export WASL_SECRETS_VOLUME="$SECRETS"
export WASL_BACKUP_VOLUME="$BACKUP"
export WASL_API_PORT=19443
export COMPOSE_PROJECT_NAME="$SOURCE"
podman compose -f "$COMPOSE_FILE" config --quiet
```

`production_secrets.sh` refuses to overwrite an existing path. `production_volume_load.sh` creates a new external secret
volume and copies files with `podman cp`; it does not depend on the Podman VM sharing host paths. Preserve that volume
and a separately protected copy of its recovery keys. Losing the JetStream key makes encrypted retained stores
unreadable; losing the backup key prevents bundle authentication.

## Optional explicit synthetic seed

This command is intentionally destructive and is **not** part of restart or restore. Small loop/person settings below
are for recovery drills only, not evaluation results.

```bash
export COMPOSE_PROJECT_NAME="$SOURCE"
export WASL_LOOPS=40 WASL_DISTRACT=10 WASL_CERV=20 WASL_PERSONS=500 WASL_QUIETTIMEOUT=3m
podman compose -f "$COMPOSE_FILE" --profile seed up -d --build pipeline
podman wait "$SOURCE"-pipeline-1   # must print 0
podman compose -f "$COMPOSE_FILE" up -d api
scripts/recovery_api_probe.py --secrets "$DRILL/secrets" \
  --base-url https://127.0.0.1:19443 --output "$DRILL/source-probe.json"
```

Wait for the API health check to be healthy before the probe. The probe verifies anonymous rejection (401), reads one
synthetic case, and records its comparison/content hashes and `loaded_at`. To include mutable history, an authenticated
reviewer can append a review before backup; the final observed drill did this and returned HTTP 201.

## Backup

Quiesce the seed and workflow writers first. The drill used a completed seed and no in-flight workflow writes. Do not
create a second bundle in the same backup volume; use a new volume for each immutable generation.

```bash
export COMPOSE_PROJECT_NAME="$SOURCE"
podman compose -f "$COMPOSE_FILE" --profile recovery run --rm recovery recovery-backup
```

The output records stream/message counts and per-stream logical hashes. The bundle is at `/backup/bundle` in the
external backup volume and contains `nats/nats.json`, the run files, and `manifest.json`. The HMAC key is not included.

## Restore into empty volumes from a clean checkout

Clone the committed revision into a **new empty directory**. Do not restore from the working tree used to author the
change. The clean checkout may share the already built image cache, but its Compose project and all data volumes must
be new and empty.

```bash
git clone --no-hardlinks /path/to/repository.git "$DRILL/clean-checkout"
cd "$DRILL/clean-checkout"
git rev-parse HEAD   # record the exact restored code revision
export ROOT="$PWD"
export COMPOSE_FILE="$ROOT/deploy/production.compose.yml"
export COMPOSE_PROJECT_NAME="$TARGET"
export WASL_SECRETS_VOLUME="$SECRETS"
export WASL_BACKUP_VOLUME="$BACKUP"
export WASL_API_PORT=19444

podman compose -f "$COMPOSE_FILE" up -d --build nats-0 nats-1 nats-2
podman compose -f "$COMPOSE_FILE" --profile recovery run --rm recovery recovery-restore
podman compose -f "$COMPOSE_FILE" up -d api
podman compose -f "$COMPOSE_FILE" --profile recovery run --rm recovery recovery-verify
scripts/recovery_api_probe.py --secrets "$DRILL/secrets" \
  --base-url https://127.0.0.1:19444 --output "$DRILL/target-probe.json"

# Repeat after an ordinary restart. No seed profile is present in these commands.
podman compose -f "$COMPOSE_FILE" restart nats-0 nats-1 nats-2 api
scripts/recovery_api_probe.py --secrets "$DRILL/secrets" \
  --base-url https://127.0.0.1:19444 --output "$DRILL/restart-probe.json"
python3 - "$DRILL/source-probe.json" "$DRILL/target-probe.json" "$DRILL/restart-probe.json" <<'PY'
import json, sys
source, target, restart = [json.load(open(path)) for path in sys.argv[1:]]
assert source["case_id"] == target["case_id"] == restart["case_id"]
assert source["comparison_sha256"] == target["comparison_sha256"] == restart["comparison_sha256"]
assert source["unauthenticated_status"] == target["unauthenticated_status"] == 401
print("clean restore and restart comparison hashes match")
PY
```

`recovery-restore` performs HMAC and payload verification first, refuses a nonempty JetStream, restores and reads each
message, and only then migrates the run generation. `recovery-verify` independently rereads every retained message after
restart. A clean empty-volume/container restore on one Podman host does **not** prove physical second-machine recovery.

## Reversible file migration and rollback

Stop the API/workflow writers before replacing a generation. NATS is not modified by these two file commands.

```bash
export COMPOSE_PROJECT_NAME="$TARGET"
podman compose -f "$COMPOSE_FILE" stop api
podman compose -f "$COMPOSE_FILE" --profile recovery run --rm --entrypoint python3 recovery \
  /app/scripts/recovery.py migrate /backup/bundle --key-file /run/secrets/backup_key /var/lib/wasl --replace
podman compose -f "$COMPOSE_FILE" --profile recovery run --rm --entrypoint python3 recovery \
  /app/scripts/recovery.py rollback /var/lib/.wasl.migration.json
podman compose -f "$COMPOSE_FILE" start api
```

The journal identifies the rollback and rejected generations. Re-running rollback without a retained previous
generation fails. A fresh restore into an empty destination has no previous generation to roll back to.

## Negative drills

Use separate projects and copies. Never tamper with the original bundle, and never use `down -v` outside the explicitly
named negative-test project.

### Wrong key on retained volumes

```bash
export COMPOSE_PROJECT_NAME="$TARGET"
podman compose -f "$COMPOSE_FILE" down                 # retain this project's volumes
scripts/production_secrets.sh "$DRILL/secrets-wrong"
scripts/production_volume_load.sh wasl-recovery-unique-secrets-wrong "$DRILL/secrets-wrong"
export WASL_SECRETS_VOLUME=wasl-recovery-unique-secrets-wrong
podman compose -f "$COMPOSE_FILE" up -d nats-0
podman logs "$TARGET"-nats-0-1 2>&1 | grep 'does not match the key'
```

Expected: refusal before `nats-server` starts. Restore the original `WASL_SECRETS_VOLUME` afterward.

### Missing key file on retained volumes

```bash
export COMPOSE_PROJECT_NAME="$TARGET"
export WASL_SECRETS_VOLUME="$SECRETS"
podman compose -f "$COMPOSE_FILE" down
scripts/production_volume_load.sh wasl-recovery-unique-secrets-missing "$DRILL/secrets"
podman run --rm --user 0:0 -v wasl-recovery-unique-secrets-missing:/run/secrets \
  docker.io/library/alpine:3.23 rm /run/secrets/js_key
export WASL_SECRETS_VOLUME=wasl-recovery-unique-secrets-missing
podman compose -f "$COMPOSE_FILE" up -d nats-0
podman logs "$TARGET"-nats-0-1 2>&1 | grep 'WASL_JS_KEY_FILE is unreadable'
```

Expected: refusal before `nats-server` starts. The production profile also refuses unmarked retained stores instead of
accepting a legacy unverified key.

### Tampered backup on a separate empty project

```bash
export NEGATIVE=waslrectamper-unique
export TAMPERED=wasl-recovery-unique-backup-tampered
podman volume create "$TAMPERED"
podman run --rm --user 0:0 -v "$BACKUP":/source:ro -v "$TAMPERED":/backup \
  docker.io/library/alpine:3.23 cp -a /source/. /backup/
# Guarantee a first-line payload change; remove the editor artifact so the payload hash branch is exercised.
podman run --rm --user 0:0 -v "$TAMPERED":/backup \
  docker.io/library/alpine:3.23 sed -i.bak 1s/^/X/ /backup/bundle/files/gen.json
podman run --rm --user 0:0 -v "$TAMPERED":/backup \
  docker.io/library/alpine:3.23 rm /backup/bundle/files/gen.json.bak

export COMPOSE_PROJECT_NAME="$NEGATIVE"
export WASL_SECRETS_VOLUME="$SECRETS"
export WASL_BACKUP_VOLUME="$TAMPERED"
podman compose -f "$COMPOSE_FILE" up -d nats-0 nats-1 nats-2
if podman compose -f "$COMPOSE_FILE" --profile recovery run --rm recovery recovery-restore; then
  echo 'tampered restore unexpectedly succeeded' >&2; exit 1
fi
# Assert zero streams and zero restored run files after refusal.
podman compose -f "$COMPOSE_FILE" --profile recovery run --rm --entrypoint python3 recovery -c \
  "import json,os,ssl,urllib.request; assert os.listdir('/var/lib/wasl')==[]; c=ssl.create_default_context(cafile='/run/secrets/ca.crt'); d=json.load(urllib.request.urlopen('https://nats-0:8222/jsz',context=c,timeout=5)); assert d['streams']==0; print(json.dumps({'streams_after_refusal':0,'run_files_after_refusal':0}))"
```

Expected error: `recovery refused: backup payload hash mismatch: gen.json`. An added unauthenticated file or a modified
manifest also fails; a wrong backup key fails authentication before payload access.

## Cleanup

Only after saving nondeterministic-field and hash evidence outside the secret/backup volumes:

```bash
for project in "$SOURCE" "$TARGET" "$NEGATIVE"; do
  export COMPOSE_PROJECT_NAME="$project"
  podman compose -f "$COMPOSE_FILE" --profile seed down -v --remove-orphans
done
# Explicitly remove only the drill's external volumes when their retention period ends.
podman volume rm "$BACKUP" "$TAMPERED" "$SECRETS" \
  wasl-recovery-unique-secrets-wrong wasl-recovery-unique-secrets-missing
rm -rf "$DRILL"   # only this drill's generated secrets, copies, and evidence directory
```

## Observed evidence (9 Nov 2026 UTC)

The drill used synthetic data, isolated Compose projects `waslrecsrccb59036b`, `waslrecdstcb59036b`,
`waslrectampercb59036b`, and final review-inclusive source/target `waslrecsrccb59036b` / `waslrecfinalcb59036b`.
The separate clean-checkout target was `waslrecclonecb59036b`, restored from commit
`c0b18986626663b820b9a6112ad94583eb71a931` with no local checkout changes.

- Explicit seed: 60 cases. Final source included one authenticated review (HTTP 201) and
  `api_reviews.ndjson` SHA-256 `ae403fcc00f9cc9c25e84be840fa6854eaff94d75d95a9eed4a95afdd5cb2f73`.
- Backup/restore/readback: 6 streams, 5,868 retained messages. Logical hashes were identical before backup, immediately
  after restore, and after restart:

| Stream | Logical SHA-256 |
|---|---|
| FACTS | `beb7ac63ca3ed5e92f3e3d83d2c5f9191abeccf33f4e84ac73e36f7c24a861fa` |
| FHIR | `89c29d95211977c51f3fe59b2b68dc708a8d0fedbc7ed01916b1a2eaa60be5ae` |
| KV_FHIRSUB | `bc7d5634b0f84d2c6ddabca13913a2cfeda2d75973fd440e5d28a0c7a6cd3f9e` |
| KV_SEEN | `d80979d194f9e900ba9e9899794949adecf5e26b46d53fa0526805c52a64aad7` |
| OUT | `4a82fa0a146b97bf1f1c1bcdfd6e3d0ef803a10d33053b5e19efafb8b3f1abf6` |
| PREP | `8a99b0ee8c661bf2ef6d025b7493c6b86cdf9344cf89d44fbb0538ac511c5fa6` |

- Final source case `ahs-mushrif/ServiceRequest/AHS-sr-1`: comparison SHA-256
  `7c0a3640c36edda1bd51ae268bebbb68b28605f7b7be9cb3e203fdf6080066ce`, equal after final restore and ordinary
  restart. `meta.loaded_at` was `2026-11-09T08:05:18Z`, `2026-11-09T08:06:52Z`, and `2026-11-09T08:07:10Z`;
  corresponding full content hashes were `74668f653fc68c8afc744747fa3ca196393615eb07db8782602512b1868460d8`,
  `3d0d6e8edcb3a081a77a5c70282f32749c38af3f8aefceb2c4a0b2db7907e7d1`, and
  `b524096434c2836dc578ceb1fbc6a0fed5411f030983dec13430c29870c8bee9`. These are expected nondeterministic fields.
- The clean-checkout target returned that same comparison hash at `2026-11-09T08:24:41Z` and after restart at
  `2026-11-09T08:24:56Z`; its full content hashes were `1bc71eb02e1764c730a18405387b4c01f5b9a29c41adbc282f20ef04c8db7b1f`
  and `df04ddbf8e417d7952e8119c11d80cb765d2571b46ea6115595490aa875be06e`. Independent `recovery-verify` again read
  6 streams / 5,868 messages with the table's hashes.
- Wrong key: `WASL_JS_KEY does not match the key these retained JetStream stores were created with; refusing to start.`
  Missing key: `WASL_JS_KEY_FILE is unreadable; refusing to start.` Correct-key restart subsequently returned the same
  comparison hash.
- Guaranteed tamper: `backup payload hash mismatch: gen.json`; independent inspection returned
  `{"streams_after_refusal": 0, "run_files_after_refusal": 0}`.
- Actual migration retained `/var/lib/.wasl.rollback-59in7b9_`; `rollback` returned status `ok`, and the subsequent API
  probe returned the pre-migration comparison hash. NATS was not changed by migration/rollback.
- Non-root inspection returned `user=1000:1000` for NATS and `user=10001:10001` for API. Anonymous HTTPS API requests
  returned 401. API/recovery log records were JSON; Compose used rotating JSON-file logs.
- An early verification command exposed a zero-valued CLI summary and an advertised-IP reconnect failure. Those were
  fixed and the final command reported 6/5,868 with matching hashes. An initial payload-edit attempt did not change the
  payload and was not counted as tamper evidence; the guaranteed first-line edit above did fail as required.

## Unresolved hardening and verification limits

- Bundle HMAC detects modification but does not encrypt contents or provide a signature/KMS identity. Use protected,
  encrypted, access-controlled backup storage and managed keys for real data. Key rotation is not implemented.
- This is a same-host clean empty-volume/container restore, not a physical second-machine, object-lock/WORM, or off-site
  recovery test. Docker Engine and other operating systems remain unverified.
- Durable consumer configuration is restored, but delivered/ack floors are deliberately not forged; consumers replay
  retained messages. The application must retain idempotent replay handling. Physical sequence/timestamp and consumer
  positions are recorded as nondeterministic, not claimed stable.
- The API still shares a writable run mount so its review ledger is included in the same recovery transaction. A
  read-only canonical projection plus a separately backed-up writable API state volume remains hardening work.
- NATS monitoring is HTTPS on the private Compose network but is not separately password-authenticated or host-published.
  MLLP/FHIR external feed hardening, live production worker entrypoints, provider mappings, key rotation, readiness
  quorum checks, centralized log export, alerts, and resource sizing remain outside this recovery slice.
- The bundle format uses JSON and loads one stream's messages in memory; measured drill size was small. Large-run
  streaming/chunking and restore RPO/RTO objectives still need measurement.
