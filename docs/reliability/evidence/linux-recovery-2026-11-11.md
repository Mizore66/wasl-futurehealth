# Linux/Podman recovery evidence — 11 Nov 2026

## Scope and revision

Production-shaped recovery drill using synthetic data and Linux containers on one Podman host. Base `main` was `75e6010a9890c58727c780b87cbec17e011ac948`; the clean target used `b2c01b805130bf30250b87deb80344d310048508` after drill findings were fixed. Compose provider was Docker Compose v5.1.4. No real patient/feed data and no second physical machine were used. `928360b` is a documentation-only follow-up to the tested code.

## Verified

- Explicit isolated seed completed with **60 synthetic cases**.
- Authenticated HTTPS source probe returned one case; anonymous health access returned **401**. Queue reads used reviewer; export used auditor.
- Encrypted v2 backup contained **8 streams / 7,256 messages**. Decrypted NATS payload measured **7,101,707 bytes**, below the recovery service's 64 MB tmpfs cap.
- A clean checkout and new empty volumes restored **8 streams / 7,256 messages**. Independent `recovery-verify` reread all retained messages successfully.
- Source, restored target, ordinary restart, file rollback, and correct-key restart all returned comparison SHA-256 **`9a328bed21f75cc23c5023a2a4360dc088d62b9b61c85f899c3aa05ae6476c24`**. `meta.loaded_at` and full content hashes varied as documented nondeterministic fields.
- File migration followed by rollback restored the original comparison hash. NATS was not modified by migration/rollback.
- Wrong JetStream key on retained volumes was refused before NATS startup: `WASL_JS_KEY does not match the key ... refusing to start.` Restoring the correct key returned the original comparison hash.
- A guaranteed one-byte ciphertext change in a separate bundle copy was refused with `backup payload hash mismatch: gen.json`; independent inspection returned **0 streams and 0 run files**.

## Findings fixed during the drill

1. `scripts/recovery_api_probe.py` used reviewer for the auditor-only export, returning 403. Fixed in `1e4f98c`; a per-route credential regression now covers reviewer queue reads and auditor export.
2. Empty streams have `FirstSeq=LastSeq=0`; `readRecoveryStream` attempted sequence 0 and failed. Fixed in `23ef49e`, with a real-NATS empty-stream regression.
3. Existing KV bundles carried source write preconditions such as `Nats-Expected-Last-Subject-Sequence`. Republishing them failed with `wrong last sequence`. Backup-side filtering was insufficient because the existing bundle retained them. Fixed in `a791160` and `b2c01b8`: original digests are validated, then broker-only preconditions are normalized before restore/readback. A regression loads an existing bundle containing a write precondition and restores it.

Partially restored targets were preserved for diagnosis and not reused. Every successful restore used a new empty project/volumes and the exact committed fix revision. Bundle digests are validated before normalization; broker write preconditions are then excluded from normalized restore/readback digests, so those normalized digests can differ from the original bundle's raw logical digest.

## Not proven / remaining limits

- **Offline build is not proven.** `podman build --network=none`, including with the existing image as layer cache, stopped at `go mod download` because no preseeded Go module cache was available (`proxy.golang.org ... network is unreachable`). No offline-build success is claimed.
- This was one Linux/Podman host, not a physical second machine. No cross-machine recovery claim.
- No 24-hour soak, real Malaffi feed, external professional security review, or independent clinical annotation was performed.
- Live container key-rotation/custody migration was not exercised; rotation behavior has unit coverage only.
- Temporary plaintext deletion is logical unlink, not physical secure erasure.
