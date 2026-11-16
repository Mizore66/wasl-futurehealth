# Production-shaped backup, restore, and rollback

## Scope and trust boundary

This package adds a **production-shaped recovery path for the synthetic Wasl run**. It is not a claim of production
certification, a physical second-machine test, or a real-feed entrypoint. The existing demo deployment remains available
in `docker-compose.yml`; the recovery path is `deploy/production.compose.yml`.

Built controls:

- JetStream stream configuration, durable consumer configuration, KV checkpoints, and every retained message are backed
  up. Restore is refused unless the target JetStream has zero streams. Each restored stream is read back and compared
  using a logical SHA-256 over retained order, subjects, headers, and payloads. That digest **excludes broker
  headers**, i.e. the direct-get read metadata (`Nats-Stream`, `Nats-Sequence`, ...) and every key beginning with the
  exact, case-sensitive prefix `Nats-Expected-` or `Nats-Batch-` (the families nats-server 2.12.6 itself strips when
  sourcing or mirroring). `Nats-Expected-*` are optimistic-concurrency write preconditions. `Nats-Batch-*` are
  atomic-batch publish control headers, which the server stores on each batched message; a restored atomic-batch
  message therefore loses its stored `Nats-Batch-*` metadata and is republished on its own, not as part of a batch.
  Wasl publishes no atomic batches. Matching is case-sensitive like the server's, so a look-alike such as
  `nats-expected-stream` is ordinary application data and is kept. `Nats-Msg-Id`, `Nats-Rollup`, and `KV-Operation`
  are kept. Restore first validates each stream's stored digest over the bundle's headers **exactly as backed up**,
  including any preconditions an older bundle carries. It then refuses any message with a header key that has no
  values, a key nats.go would not encode, or a value containing a NUL byte, because the unprefixed digest cannot
  tell such shapes apart from a different header split. Only then does it drop broker headers for publish and readback.
  Built and tested with Go unit tests and local single-node NATS 2.12.6 only.
- The file bundle contains the materialised run, cohort, anchors, and API review ledger. New bundles are
  `wasl-file-backup-v3-chunked`: each payload (including the NATS payload) is split into fixed-size chunks (1 MiB by
  default), and every chunk is AES-256-GCM encrypted and authenticated on its own under a fresh per-bundle data key,
  which is wrapped by the active generation of a versioned keyring (`recovery_keys.json`) held outside the bundle; the
  manifest is HMAC-SHA-256 authenticated with that generation's independent key. Backup, verification, extraction and
  restore stream one chunk at a time (see [Chunked bundles and streamed NATS payloads](#chunked-bundles-and-streamed-nats-payloads)).
  Existing `wasl-file-backup-v2-aesgcm` bundles remain verifiable and restorable unchanged. Verification finishes
  before restore contacts NATS or creates run files.
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

Remaining boundaries are listed at the end. v2 and v3 bundles are encrypted and authenticated, but the keyring is the single
secret that unlocks them: keep the backup and secret volumes in separate protected storage. `comparison_sha256` excludes `meta.loaded_at`;
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
unreadable; losing the recovery keyring (`recovery_keys.json`, v2 and v3 bundles) or the legacy `backup_key` (v1 bundles) prevents bundle authentication and decryption.

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
synthetic case as reviewer, and exports it as auditor (the export role is auditor-only). It records comparison/content hashes and `loaded_at`. To include mutable history, an authenticated reviewer can append a review before backup; the final observed drill did this and returned HTTP 201.

## Backup

Quiesce the seed and workflow writers first. The drill used a completed seed and no in-flight workflow writes. Do not
create a second bundle in the same backup volume; use a new volume for each immutable generation.

```bash
export COMPOSE_PROJECT_NAME="$SOURCE"
podman compose -f "$COMPOSE_FILE" --profile recovery run --rm recovery recovery-backup
```

The output records stream/message counts and per-stream logical hashes. The bundle is at `/backup/bundle` in the
external backup volume and contains `manifest.json` and one `files/<path>.aead` ciphertext per run file plus the NATS
payload. Neither the keyring nor any data key is included. `recovery-backup` streams the NATS payload from
`wsi7 backup-nats -stream` through an inherited pipe straight into chunk encryption; no plaintext NATS file is written.
`WASL_RECOVERY_CHUNK_BYTES` overrides the 1 MiB chunk size (1 byte to 64 MiB).

### Key custody, rotation and legacy bundles

- `rotate-key` (`python3 scripts/recovery.py rotate-key --key-file <keyring>`) adds a new active generation under a
  lock and prints only its id. Older generations are **read-only and must be retained for as long as any bundle written
  under them must stay restorable**; pruning a generation makes its bundles permanently unreadable (the tool refuses
  them with a "key generation ... not in the keyring" error). Rotation does not re-encrypt existing bundles.
- Back up the keyring separately from the bundles. Losing it, or the generation a bundle names, loses that bundle.
- Legacy v1 (HMAC-only, unencrypted) bundles are read-only and are accepted only with an explicit opt-in:
  `--legacy-hmac` on `verify`/`migrate`, or `WASL_RECOVERY_LEGACY_HMAC=1` with `WASL_RECOVERY_KEY_FILE` pointing at the
  old `backup_key`. It is never a default; `backup` always writes chunked v3.
- Adding or rotating the keyring on an existing deployment: **in-place change of an existing secrets volume is not
  supported.** `production_secrets.sh` refuses an existing directory, `production_volume_load.sh` refuses an existing
  volume, and the `recovery` service mounts the secrets volume read-only. The working path is to create a **new**
  secret directory and **new** volume: run `production_secrets.sh <new-dir>`, then copy the existing `backup_key`
  (for legacy reads) and, to keep earlier v2 bundles restorable, the existing `recovery_keys.json` from your protected
  copy over the new files, run `recovery.py rotate-key --key-file <new-dir>/recovery_keys.json` on that host copy if
  you want a fresh active generation, load it with `production_volume_load.sh <new-volume> <new-dir>`, and point
  `WASL_SECRETS_VOLUME` at it. Keep the same `js_key` and TLS/password files if the retained JetStream stores and
  clients must keep working; replacing only the recovery files means copying the old values for those too. This
  procedure is documented, not exercised in a container drill.
- **Scratch space.** For chunked v3 bundles neither backup nor restore nor `recovery-verify` writes a plaintext NATS
  file: the payload moves through pipes, so the 64 MB `/tmp` tmpfs of the `recovery` service
  (`tmpfs: ["/tmp:size=64m,mode=1777"]` in `deploy/production.compose.yml`) no longer limits the NATS payload size.
  Restore still decrypts the **run files** into a private mode-0700 staging directory beside the destination (the
  writable `wasl-run:/var/lib` volume), so that filesystem needs room for one full decrypted copy of the run files; the
  NATS payload is never part of that copy. Restoring an **existing v2 bundle** still decrypts its NATS payload, whole and
  in memory, into a private file under `${TMPDIR:-/tmp}`, so for v2 bundles the 64 MB tmpfs remains the cap: raise its
  `size=` or mount a larger writable volume and set `TMPDIR` if a v2 payload exceeds it (`python3
  scripts/recovery.py extract-nats <bundle> - --key-file <keyring> | wc -c` measures it without writing plaintext).
  Temporary plaintext is removed logically (unlink), which is **not physical secure erasure**.

## Chunked bundles and streamed NATS payloads

**Built and tested locally (Go unit tests, Python unit tests, and single-node NATS 2.12.6 drills); not yet run in a
container drill or on a second machine.**

### Formats

- **File bundle `wasl-file-backup-v3-chunked`.** The manifest adds `chunk_size` (1 byte to 64 MiB, 1 MiB by default)
  and records, per file, the explicit plaintext `size`, `chunks` (which must equal `max(1, ceil(size / chunk_size))`),
  the whole-plaintext `sha256`, the whole-ciphertext `ciphertext_sha256`, and a 4-byte `nonce_prefix` unique within the
  bundle. `files/<path>.aead` is the concatenation of the chunks; each is `ciphertext || 16-byte tag`, so the file is
  exactly `size + 16 * chunks` bytes. Chunk `i` uses nonce `nonce_prefix || i` (64-bit big-endian) under the fresh
  per-bundle data key, and its AES-GCM associated data binds the schema, key id, path, chunk size, chunk index and a
  final-chunk flag. The data key is wrapped with associated data `wasl-file-backup-v3-chunked|<key id>`.
- **NATS payload `wasl-nats-backup-v2-stream`.** Newline-delimited JSON: a header with the stream count; per stream a
  `stream` record (configuration and durable consumers), one `message` record per retained message, and a
  `stream_end` record with its explicit message count, data byte total and v1 logical digest of the headers exactly as
  stored; and a final `end` trailer with the stream and message totals. Streams appear in strictly ascending name
  order, messages in strictly increasing sequence order. One record is limited to 96 MiB (a 64 MiB NATS
  `max_payload` message encoded); backup refuses to write a larger record.

### Fail-closed behaviour (tested)

- Each chunk is authenticated before any of its plaintext is used. A bit flip, a reordered, duplicated or appended
  chunk, a truncation inside a chunk, and a truncation exactly at a chunk boundary (caught by the final-chunk flag even
  when an attacker with the authentication key rewrites the manifest consistently), a wrong or missing key generation,
  a swapped nonce prefix, an inconsistent chunk size or count, and an unauthenticated manifest edit all fail closed.
- When plaintext is delivered to a consumer (a restored file, `extract-nats`, or a pipe into `check-nats` or
  `restore-nats`), the final chunk is held back until the whole-ciphertext and whole-plaintext hashes have matched, so
  a consumer never receives a complete stream that failed final verification.
- The NATS stream reader refuses a missing trailer, data after the trailer, truncation anywhere, reordered or
  duplicated messages or streams, a misnumbered stream, count/size/total mismatches, unknown fields, malformed headers
  and oversized records. The **original digest of each stream is validated before its normalised content is used**,
  exactly as for v1 payloads; broker headers are dropped only for publish and readback.
- A pipe cannot be reread, so restore validates the complete authenticated payload offline with
  `wsi7 check-nats -bundle <pipe>` first, then streams it a second time into `wsi7 restore-nats -bundle <pipe>
  -prechecked`. Without `-prechecked` a non-rereadable stream is refused before JetStream is touched; a regular file is
  scanned completely before the first write. The restore pass re-validates every stream's digest and reads every
  restored stream back. `recovery.py migrate` refuses a chunked bundle unless `--nats-check-command` is given.
- Restore creates each stream with the shortest accepted 100 ms deduplication window, waits only when a
  `Nats-Msg-Id` was republished within the last 200 ms (tracked for that long only, so memory stays bounded), fails if
  any message is suppressed as a duplicate, and then restores the stream's own configuration.

### Interrupted transfers and partial restores

- A NATS backup command that exits non-zero, or a stream that cannot be encrypted, leaves no bundle: the staging
  directory beside the destination is removed and nothing is published; existing bundles and the source are untouched.
- A restore never deletes or overwrites the bundle, refuses a non-empty destination unless `WASL_RECOVERY_REPLACE=1`
  (which retains the previous generation for rollback), and refuses a JetStream that already has streams.
- A validation failure (`check-nats`) happens before any JetStream write and installs no files. A failure while
  `restore-nats` runs (including an interrupted pipe, which it reports as truncation) leaves whatever it already
  created in JetStream **in place as evidence**; the run files are not installed, and a retry is refused until an
  operator purges JetStream manually. Automatic purge is deliberately not implemented.

### Measured resource use (synthetic, local)

Measured on macOS/arm64 with Python 3.13 and Go 1.27.4 by `scripts/test_recovery_chunked.py` and
`pipeline/recovery_stream_test.go`, using a synthetic 75,497,472-byte (72 MiB) NATS payload, larger than the old
64 MB scratch cap, with chunks forced to 64 KiB:

| Path | Measured |
| --- | --- |
| Python heap peak (`tracemalloc`): streamed backup / verify / extraction / pipe into a reader | 0.29 / 0.21 / 0.21 / 0.22 MB |
| Whole-process max RSS, `recovery.py backup` with the streamed generator / `recovery.py verify` | about 32 / 32 MB (interpreter and OpenSSL baseline) |
| Plaintext written to `TMPDIR` during backup, extraction and streamed restore input | none |
| Go retained (post-GC) heap growth while validating a 100,784,621-byte `wasl-nats-backup-v2-stream` payload (`scanNATSStream`, 64 KiB messages) | about 1.3 MB |
| Clean extraction and streamed-pipe SHA-256 vs the source stream | equal |

These are single-run observations on one developer machine, not a sizing guarantee. Peak memory is bounded by the
chunk size (Python) and the largest single NATS message record (Go); the Go bound therefore grows with the configured
NATS `max_payload`, up to the 96 MiB record limit.

### Remaining unknowns

- **RPO and RTO are unmeasured.** No production-sized stream has been backed up or restored; restore throughput is
  bounded by synchronous republishing of one message at a time and was not timed against a target. No RPO/RTO claim is
  made.
- Restore decrypts and parses the NATS payload three times (bundle verification, `check-nats`, `restore-nats`), which
  trades CPU time for bounded memory; its cost on a large payload is unmeasured.
- The new path has not been exercised in the Compose/Podman container drill, on Linux, or on a second machine; the
  11 Nov Linux evidence below is for v2 bundles.
- Restoring an existing v2 bundle keeps the previous whole-payload memory and `TMPDIR` requirements; re-creating a new
  v3 backup from the restored deployment is the migration path to bounded resources.

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

`recovery-restore` authenticates the bundle, stages and decrypts the run files, validates the streamed NATS payload
offline with `wsi7 check-nats` (grammar, ordering, counts, sizes and original digests, before any JetStream write),
refuses a nonempty JetStream, restores and reads each message, and only then installs the run generation. A file-side failure (wrong key, tamper, populated
destination without `WASL_RECOVERY_REPLACE=1`) therefore leaves NATS untouched. If `restore-nats` itself fails, JetStream
may be partly populated. **Any failure after the NATS step begins (a `restore-nats` error, a readback error, or a file
install error after NATS was restored) requires a manual JetStream purge before the restore is retried**, because the
empty-target guard is deliberately not relaxed and full file/NATS atomicity is not claimed. `recovery-verify` independently rereads every retained message after
restart. A clean empty-volume/container restore on one Podman host does **not** prove physical second-machine recovery.

## Reversible file migration and rollback

Stop the API/workflow writers before replacing a generation. NATS is not modified by these two file commands.

```bash
export COMPOSE_PROJECT_NAME="$TARGET"
podman compose -f "$COMPOSE_FILE" stop api
podman compose -f "$COMPOSE_FILE" --profile recovery run --rm --entrypoint python3 recovery \
  /app/scripts/recovery.py migrate /backup/bundle --key-file /run/secrets/recovery_keys.json /var/lib/wasl --replace
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
  docker.io/library/alpine:3.23 sed -i.bak 1s/^/X/ /backup/bundle/files/gen.json.aead
podman run --rm --user 0:0 -v "$TAMPERED":/backup \
  docker.io/library/alpine:3.23 rm /backup/bundle/files/gen.json.aead.bak

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

> **This block is v1 / HMAC-only evidence**: those bundles were authenticated but not encrypted and used the old
> `backup_key`. It is not evidence of v2 encryption, the keyring, or the staged-restore order.

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

## Observed v2 Linux evidence (11 Nov 2026)

See [`docs/reliability/evidence/linux-recovery-2026-11-11.md`](reliability/evidence/linux-recovery-2026-11-11.md): an encrypted v2 bundle restored 8 streams / 7,256 messages into a clean checkout and new empty volumes on Podman; HTTPS hashes matched after restore, restart, file rollback, and correct-key restart. Wrong-key and guaranteed-ciphertext-tamper drills failed closed. The 7,101,707-byte decrypted NATS payload fits the 64 MB tmpfs cap. Original bundle digests are validated before broker write preconditions are normalized for restore/readback; normalized digests can therefore differ from an older bundle's raw logical digest. That drill could not complete an offline build. A [12 Nov follow-up](reliability/evidence/offline-container-build-2026-11-12.md) completed the full rebuild with tracked Go modules and preseeded base images/package layers; a later [clean-build record](reliability/evidence/offline-clean-build-2026-11-12.md) covers `--no-cache --network=none` builds of both images on Podman/arm64 with preloaded base images; that is still not an empty-host or hosted Docker claim.

## API backup-age observation

When `WASL_BACKUP_MANIFEST` explicitly names a bundle `manifest.json`, `GET /api/operations` reports the manifest schema and `created_at`, plus its age at the API's `observed_at`. The production-shaped Compose API configures `/backup/bundle/manifest.json` and mounts the recovery service's external `wasl-backup` volume read-only at `/backup`, so the documented backup path is observable by the API. The read is bounded; incomplete manifest structures, directory/payload-file records, and future or invalid timestamps are rejected. With no configured path, backup metadata is explicitly unmeasured; read, validation, and future-time failures are unavailable without an age. Observation and age calculation retain subsecond precision.

This observation is **not restore verification**. The API does not authenticate the manifest or use key material, decrypt payloads, run `recovery-verify`, or perform a restore. `verification_status: "not_verified_by_api"` is deliberate. A structurally valid manifest and creation timestamp are not proof of a successful backup, usable restore, current recovery point, or RPO. The recovery commands above remain the verification path; the operations endpoint does not alter bundles or recovery files.

## Operator-run scheduled backup and clean-target verification

**Built:** `scripts/recovery_schedule.py` and `scripts/recovery_catalog.py` provide UTC-slot scheduling, immutable generation
publication, isolated clean-target verification, crash classification, and explicitly applied ciphertext retention. The
implementation uses the existing encrypted (chunked v3 since this change; v2 generations remain verifiable) `recovery-backup`, `recovery-restore`, and `recovery-verify` paths and the
authenticated reviewer/auditor HTTPS probe. The scheduler never invokes the `seed` profile, never sets
`WASL_RECOVERY_REPLACE`, does not install cron/systemd timers, and does not change deployment workflows.

The example configuration is [`recovery-scheduler.example.json`](recovery-scheduler.example.json). Use a dedicated
synthetic source deployment, dedicated secret/backup volumes, and a new evidence directory. The source repository and
`wasl:local` image must contain the scheduler scripts; the catalog helper bind-mounts the configured repository read-only
and never receives secret values as arguments. `HEAD` is resolved and recorded for every attempt, but a pinned full commit
SHA is recommended for steady operation. The target checkout is cloned with `git clone --no-hardlinks` at that resolved
revision.

Initialize only a **new empty backup volume and new empty evidence directory**:

```bash
export ROOT=/srv/wasl/repo
python3 "$ROOT/scripts/recovery_schedule.py" --config /srv/wasl/scheduler.json init
python3 "$ROOT/scripts/recovery_schedule.py" --config /srv/wasl/scheduler.json run-due
python3 "$ROOT/scripts/recovery_schedule.py" --config /srv/wasl/scheduler.json serve
python3 "$ROOT/scripts/recovery_schedule.py" --config /srv/wasl/scheduler.json verify --generation latest
python3 "$ROOT/scripts/recovery_schedule.py" --config /srv/wasl/scheduler.json retention
python3 "$ROOT/scripts/recovery_schedule.py" --config /srv/wasl/scheduler.json retention --apply
```

`serve` is an operator-run foreground loop, not an installed service. `run-due` is the one-shot equivalent for an external
scheduler chosen by the operator. Configuration validation requires absolute disjoint paths, an HTTPS loopback source URL,
safe project/volume identifiers, positive intervals/timeouts, and `keep_newest_verified >= min_verified_good >= 1`.

### Backup generations and source availability

Each scheduled backup first requires the pipeline/workflow writer to be stopped. It records whether the source API was
running and stops a running API to exclude review-ledger writes. After encrypted backup and complete bundle authentication,
the API is returned to its prior state. An initially running API is restarted; an initially stopped API is started only
for the authenticated source evidence probe and then stopped again. **The source API therefore has a deliberate temporary
outage during backup.** If its prior state cannot be restored, `halted.json` prevents later scheduled operations until an
operator intervenes.

The dedicated volume layout is:

```text
/backup/scheduler-v1/generations/<generation-id>/bundle/
/backup/bundle -> scheduler-v1/generations/<current-id>/bundle
```

The existing backup command receives a generation-specific `WASL_RECOVERY_DIR`. The catalog authenticates the complete
bundle before atomically moving only the `bundle` symlink, preserving `/backup/bundle/manifest.json` as the existing API
observation path. Publication refuses an ordinary file/directory or a symlink outside the scheduler namespace. A failed
or partial generation is never published or reused.

UTC slots are calculated from `epoch`; only the current slot is eligible. Missed-slot ranges are recorded and never
caught up. Each automatic backup/verification slot gets at most one attempt, whether it succeeds or fails. Repeated
`run-due` calls in one slot do not create another backup or target. Manual `verify` is always a new attempt and target.
Configured intervals are scheduling controls, **not measured RPO or RTO**.

### Clean-target verification

Verification resolves and authenticates the selected encrypted v2 or v3 generation before creating target resources. It then:

1. clones the resolved revision into an attempt-private checkout;
2. generates a unique target project and requires its `nats-0`, `nats-1`, `nats-2`, and `wasl-run` volumes and Compose
   containers not to exist;
3. starts new target NATS volumes, restores without replacement, starts the API, independently rereads stream hashes, and
   runs authenticated/anonymous HTTPS probes;
4. ordinarily restarts target NATS/API and probes again; and
5. requires backup/restore/independent-verification stream-hash maps to match and source/target/restart `case_id` and
   `comparison_sha256` to match. Every anonymous probe must return 401 and every export must report
   `meta.data_mode: "simulated"`. `content_sha256` and `meta.loaded_at` are recorded but are not expected to match.

`deploy/recovery-verification.compose.yml` mounts the selected backup generation read-only in the recovery service. Secret
and backup external volumes are inputs only. Only after all comparisons and sealed evidence succeed is the generation
marked `verified_good`; only then does `down -v` run, scoped to the generated target project. External secret/backup
volumes are not removed. Post-verification cleanup evidence is sealed separately under `cleanup/<attempt-id>/` so cleanup
logs cannot alter the sealed verification attempt.

A failed target is left intact. Compose status/logs and resource identities are captured before the attempt is sealed;
the scheduler never automatically issues `down`, volume deletion, or purge for it. A partial NATS restore remains subject
to the **manual JetStream purge rule** above. Diagnose or remove that explicitly named target only through a separate,
operator-reviewed command. Wrong keys, modified manifests/ciphertexts, preexisting targets, hash mismatches, timeouts, and
partial restores all fail closed without target cleanup.

### Crash handling, locks, and evidence

`serve.lock` prevents duplicate foreground daemons. A separate `run.lock` serializes `run-due`, manual verification,
backup, catalog, retention, and deletion activity. Both use nonblocking `flock`; contention exits 75 before attempt or
resource creation, and process death releases the locks.

Every attempt has an exclusively created `runs/<attempt-id>/started.json` and at most one `result.json`. On restart, a
missing result is classified `interrupted`, its automatic slot is not retried, and its target/generation is never reused.
If a published cursor has no durable claim (`slots/<kind>/<slot>.json`) and no started attempt, an unstarted slot that is still the current UTC slot may make its one safe attempt. Once that cursor's slot has expired, the next scheduling pass instead writes a durable `slots/<kind>/abandoned-<slot>.json` marker and a single `slot_abandoned` event (`catch_up: false`; the current UTC slot is unaffected); the expired slot is not retried or silently lost. Empty run directories are removed on restart. A `started.json` without a claim is reconciled as `interrupted`, not retried. Reconcile accepts a slot claim that exactly matches the interrupted attempt (replay after a crash) and rejects any other claim without overwriting it; a replayed identical missed-slot marker is accepted, and a conflicting one fails closed. Persisted source-API intent is reconciled before other work. Evidence includes the resolved nonsecret configuration and
revision, generation/target identities, source/target/restart probes, backup/restore/verify summaries, explicit hash
comparisons, exported manifest and SHA-256, sanitized per-command stdout/stderr, timestamps, durations, exit statuses,
and failure diagnostics. `evidence-manifest.json` authenticates the final file set with SHA-256; files and directories
are fsynced. Scheduler transitions are appended to `events.jsonl` under `run.lock`. Environment dumps, rendered Compose
configuration, passwords, keys, wrapped key material, decrypted payloads, and secret-file contents are not evidence
fields.

Exit states are 0 for success/no due slot, 1 for operational failure, 2 for invalid configuration/safety preconditions,
3 for evidence-finalization failure (cleanup and retention are skipped), and 75 for lock contention.

### Retention review and explicit apply

`retention` without `--apply` is a dry run. Automatic deletion is limited to scheduler-created ciphertext whose complete
immutable evidence previously passed clean-target verification and which is older than `max_age_days`, outside the newest
`keep_newest_verified`, not published, and has no failed/interrupted/pending/unknown/corrupted attempt state. Failed or
unknown evidence blocks the entire retention pass. Unverified generations, failed-run artifacts/logs, exported manifests,
and host evidence are never automatically deleted.

At least `min_verified_good` generations must survive, including the newest known-good restore. The only known-good
restore can never be pruned. Before any deletion, every protected survivor is authenticated again; any failure aborts the
whole pass. Each candidate is deleted individually by exact generation ID and expected manifest SHA-256, never by wildcard
or volume-wide cleanup. A retention intent is fsynced before deletion and a result afterward; the next pass reconciles a
crash by inspecting only that exact generation path. Pruning removes ciphertext payloads but retains the bundle manifest,
hashes, verification evidence, and pruning history on the host.

The scheduler does not rotate or prune recovery key generations. Keep every key generation needed by every retained bundle
in separately protected custody; pruning a key generation can make even retained ciphertext permanently unreadable.

**Verification status:** focused synthetic unit/state-machine tests exercise scheduling, locking, wrong-key/tamper refusal,
clean-target preconditions, restart hash comparison, failure preservation, and retention. A manually approved Linux/Podman
end-to-end scheduler drill has **not been run or recorded** in this change. Same-host clean restores do not prove
second-machine recovery or production readiness, and neither configured intervals nor a successful restore establishes a
production RPO/RTO.

## Unresolved hardening and verification limits

- v2 and v3 bundles are encrypted (AES-256-GCM) and HMAC-authenticated, but there is no signature/KMS identity and the
  keyring is a local file. Use protected, access-controlled backup storage and managed keys for real data. Rotation adds
  generations but does not re-encrypt old bundles. Temporary plaintext cleanup is logical, not physical erasure.
- **Not run:** a physical second-machine restore, an offline build on a host without preloaded base images (the Podman/arm64 clean build assumes them), or a 24-hour soak.
  The 11 Nov v2 Podman evidence is a same-host clean empty-volume/container restore (see the evidence link above).
  No external exactly-once or production RPO/RTO claim is made.
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
- Other server-interpreted headers are still kept and replayed as stored: `Nats-TTL`, `Nats-Incr`, `Nats-Schedule-*`
  and `Nats-Marker-Reason`. Wasl uses none of them: it sets no AllowMsgTTL, counter, schedule or marker options, so no
  stored Wasl message carries them. A stream that did use them could be re-interpreted on restore (for example a
  fresh TTL clock or a counter increment); this path does not handle that.
- The v1 logical digest is not length-prefixed. The header-shape refusal above closes the empty-value split, but
  valid keys with non-empty values can still be regrouped without changing the digest (for example
  `{"A":["a","Nats-Expected-X","x"]}` and `{"A":["a"],"Nats-Expected-X":["x"]}`). It is a consistency check, not
  tamper protection; v2 bundle authentication (HMAC plus AES-GCM) is what rejects deliberate edits. A length-prefixed
  digest would need a new bundle format.
- The bundle format uses JSON and loads one stream's messages in memory; measured drill size was small. Large-run
  streaming/chunking and restore RPO/RTO objectives still need measurement.
