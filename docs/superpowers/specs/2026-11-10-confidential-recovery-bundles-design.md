# Confidential, versioned recovery bundles

Date: 2026-11-10
Base: `8b046a841bc1bbf9e76141414c4d7c1fb1a32926`
Scope: proposal #5 only. No reviewer/model/evaluation, normalization, AMD64 or soak work.

## Requirements

- Encrypt every run-file and NATS payload with AES-256-GCM using the reviewed Python `cryptography` AESGCM implementation, pinned for CI.
- Generate a fresh 32-byte data-encryption key (DEK) per bundle and a fresh 96-bit nonce per wrapped DEK and per file. Never derive a nonce from a counter or reuse one under the same DEK.
- Authenticate all metadata. AAD binds schema, key ID, relative path, plaintext size and plaintext SHA-256; the complete manifest is HMAC-authenticated with a separate authentication key.
- Keep key material outside bundles. Bundle logs and errors may contain key IDs, paths, sizes and hashes, never secret values.
- Rotate by adding a generation and making it active. Old generations remain available for verification/restore until an operator separately retires them; rotation never deletes or rewrites old bundles.
- Reject wrong/missing keys, modified metadata/ciphertext, truncation, extra files and unsafe paths before destination writes or NATS contact.
- Preserve legacy HMAC verification/restore only through an explicit `--legacy-hmac` opt-in. New backups are always encrypted v2.
- Restore into a new empty destination and require the existing logical stream/API comparison hashes to match. Clean Linux Docker is unavailable and remains **not run**.

## Bundle format

Directory layout:

```text
manifest.json
files/<relative-path>.aead
```

`manifest.json` contains:

```json
{
  "schema": "wasl-file-backup-v2-aesgcm",
  "key_id": "nonsecret-generation-id",
  "created_at": "UTC timestamp",
  "nondeterministic_fields": ["created_at", "file_mtime", "nats_sequence", "nats_time"],
  "wrapped_dek": "base64 KEK-wrapped DEK including GCM tag",
  "wrap_nonce": "base64 96-bit nonce",
  "files": [
    {
      "path": "nats/nats.json",
      "size": 1234,
      "sha256": "plaintext SHA-256",
      "nonce": "base64 fresh 96-bit nonce",
      "ciphertext_sha256": "ciphertext plus GCM tag SHA-256"
    }
  ],
  "hmac_sha256": "HMAC over canonical manifest without this field"
}
```

The wrapping AAD is `wasl-file-backup-v2-aesgcm|key_id`. Each file AAD additionally binds path, size and plaintext hash. Ciphertext files include the GCM tag and use mode 0600. Manifest path validation, duplicate rejection and exact file-set comparison retain v1's rules. No plaintext payload file or DEK appears in the bundle.

Whole-file AESGCM is accepted for the existing demo-size bundles; large-run streaming objectives remain outside this proposal and are not claimed.

## Keyring and rotation

`recovery_keys.json` is generated only by `production_secrets.sh` into the protected external secret directory. It is never committed or printed.

```json
{
  "schema": "wasl-recovery-keyring-v1",
  "active": "generation-id",
  "keys": {
    "generation-id": {
      "wrap_key": "base64 32 random bytes",
      "auth_key": "base64 32 independent random bytes"
    }
  }
}
```

`recovery.py rotate-key --key-file recovery_keys.json` creates a fresh random generation ID and independent wrapping/auth keys, preserves every prior generation, updates `active`, fsyncs and atomically replaces the keyring while preserving its existing owner/mode. The protected secret directory is mode 0700; the keyring may be 0444 so non-root container UIDs can read it without making the parent traversable. Backup reads `active`; verify/migrate select the manifest's `key_id`. A keyring with duplicate/invalid entries, unknown active ID, missing manifest key ID or malformed key bytes is rejected. Operators control old-key retention and secure deletion separately.

`production_secrets.sh` continues to create `backup_key` for existing legacy bundles and adds `recovery_keys.json`. It never overwrites an existing secret directory. Deployment passes `recovery_keys.json` for new v2 operations; legacy commands must name the HMAC key and opt in explicitly.

## Backup and verification

1. Validate source files and keyring before creating the bundle. The destination must not exist.
2. Generate DEK, wrap it under the active wrapping key, then read each regular source file into a new ciphertext file with a fresh nonce and AAD. NATS `nats.json` uses the same path.
3. Hash plaintext and ciphertext, fsync payloads, write/fsync the HMAC-authenticated manifest, and atomically install the bundle. Failure removes only the new staging directory.
4. Verification recomputes manifest HMAC, validates key ID/path/file set, decrypts each payload with AAD, checks GCM authentication, plaintext hash and exact size, and rejects extra/truncated/corrupt files. It does not create plaintext artifacts.

## Restore and NATS replay

1. Verification completes before destination creation, NATS calls or plaintext writes.
2. Decrypt into a new mode-0700 restore staging directory with exclusive file creation and fsync. A failure removes staging and does not publish or rename destination state.
3. Install through the existing generation/rollback journal. Existing rollback history and append-only run logs remain unchanged.
4. Restore NATS only from authenticated decrypted `nats.json`; the existing empty-stream guard, stream/consumer configuration restore and logical SHA-256 read-back apply. Compare API `comparison_sha256` and stream logical hashes after replay.
5. Delete plaintext staging on success or failure. Physical secure-erasure guarantees depend on the storage medium and are not claimed.

The legacy HMAC path is read-only for old bundles and uses the same transactional restore only when `--legacy-hmac` is present. There is no silent schema fallback or legacy backup mode.

## CLI and deployment

Existing `backup`, `verify`, `migrate`, `rollback`, `recovery-backup`, `recovery-restore`, and `recovery-verify` entrypoints remain, with v2 keyring semantics. Add `rotate-key`. Legacy verification/migration requires both the old HMAC key and `--legacy-hmac`. Update demo/deployment defaults and docs to `recovery_keys.json`; no key value is accepted on a command line or emitted in logs.

Pin `cryptography` in `requirements-ci.txt` after an installation compatibility check. Do not implement AES/GCM, a KDF, key wrapping or nonce management in project code beyond calls to the reviewed library and OS randomness.

## TDD and verification matrix

Write failing tests before each behavior:

- Backup contains no plaintext marker or key material; ciphertext differs for equal inputs due to fresh DEK/nonces.
- Wrong keyring, missing active/manifest key ID, malformed keys and non-versioned raw keys fail without output.
- Manifest/path/nonce/ciphertext tamper, truncation and extra files fail before destination/NATS changes.
- Rotation backs up with the new generation and restores both new and prior bundles; wrong old/new keyrings fail.
- Legacy HMAC works only with explicit opt-in and refuses default use.
- Restore rollback, exact plaintext bytes/modes, NATS logical stream hashes and API comparison hash match the source.
- Errors never contain secret material and no key file is committed.
- Full Python recovery tests, Go tests/vet and CI contract tests pass.

Local clean restore/replay uses new temporary directories and a fresh single-node NATS instance. Clean Linux Docker checkout → seed → backup → rotate → empty-volume restore is **not run** and remains a separate deployment gate. Exactly-once external side effects, physical power loss, cross-host filesystem semantics and production RPO/RTO remain unproven.
