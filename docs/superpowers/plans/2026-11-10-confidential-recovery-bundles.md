# Confidential Recovery Bundles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Encrypt and authenticate versioned Wasl recovery bundles with rotation, legacy HMAC opt-in and transactional restore/replay verification.

**Architecture:** A focused Python crypto/keyring module wraps `cryptography` AESGCM; `recovery.py` handles bundle/restore semantics. Fresh DEKs/nonces and authenticated metadata are mandatory; old keyring generations remain read-only.

**Tech Stack:** Python 3.12 stdlib, `cryptography==50.0.2`, existing Go/NATS recovery, real single-node NATS integration tests.

**Spec:** `docs/superpowers/specs/2026-11-10-confidential-recovery-bundles-design.md`

## Global Constraints

- AES-256-GCM; fresh 32-byte DEK per bundle and fresh 96-bit nonces per wrap/file. No custom AES/GCM/KDF or nonce counters.
- Exact file-set/AAD/HMAC validation, path safety, no secrets in code/logs/commits.
- Rotation adds active generations without deleting old keys or rewriting old bundles.
- Legacy HMAC restore only with explicit `--legacy-hmac`; no silent schema fallback or legacy backup.
- Authenticate before destination/NATS/plaintext writes; exclusive transactional restore and plaintext staging cleanup.
- Local logical-hash checks only. No physical-erasure or clean Linux Docker claim, no 24h/model/eval tuning.

---

### Task 1: Keyring, rotation and AESGCM primitives

**Files:**
- Create: `scripts/recovery_crypto.py`
- Create: `scripts/test_recovery_crypto.py`
- Modify: `requirements-ci.txt`

**Interfaces:**
- Produces:
  - `new_keyring(path: Path) -> dict`
  - `load_keyring(path: Path) -> tuple[dict, str, dict]`
  - `rotate_keyring(path: Path) -> str`
  - `new_dek() -> bytes`
  - `encrypt_payload(dek: bytes, aad: bytes, plaintext: bytes) -> tuple[bytes, bytes]`
  - `decrypt_payload(dek: bytes, aad: bytes, nonce: bytes, ciphertext: bytes) -> bytes`
  - `wrap_dek(entry: dict, key_id: str, dek: bytes) -> tuple[str, str]`
  - `unwrap_dek(entry: dict, key_id: str, wrap_nonce: str, wrapped: str) -> bytes`

- [ ] **Step 1: Write failing keyring and crypto tests**

Use generated in-test keys only. Assert raw non-versioned HMAC keys fail, wrong keys fail without containing secret text, rotation preserves old generation/mode, and equal plaintext produces different ciphertext/nonces.

```python
def test_rotation_preserves_old_generation_and_decrypts(self):
    root = Path(self.tmp.name)
    keyring = root / "recovery_keys.json"
    initial = make_keyring(keyring)
    old_id = initial["active"]
    old_plaintext = b"synthetic recovery payload"
    old_dek = new_dek()
    old_entry = initial["keys"][old_id]
    nonce, wrapped = wrap_dek(old_entry, old_id, old_dek)
    new_id = rotate_keyring(keyring)
    rotated, active, _ = load_keyring(keyring)
    self.assertNotEqual(new_id, old_id)
    self.assertIn(old_id, rotated["keys"])
    self.assertEqual(unwrap_dek(old_entry, old_id, nonce, wrapped), old_dek)
    self.assertEqual(unwrap_dek(rotated["keys"][active], active, *wrap_dek(rotated["keys"][active], active, old_dek)), old_dek)
```

`make_keyring` creates schema `wasl-recovery-keyring-v1`, two independent base64 32-byte values and mode 0444. Assert invalid base64/length/active/missing generation errors do not include key bytes.

- [ ] **Step 2: Run RED**

Run: `/tmp/wasl-ci-venv/bin/python scripts/test_recovery_crypto.py`
Expected: import failure for `recovery_crypto` or missing functions.

- [ ] **Step 3: Implement minimal keyring/AESGCM helpers**

Use `os.urandom` for keys/nonces, `AESGCM` for encrypt/decrypt, base64url-safe standard base64, and exclusive temporary keyring writes plus `os.replace`. `wrap_dek` AAD is `b"wasl-file-backup-v2-aesgcm|" + key_id.encode()`. Return `(wrap_nonce_b64, wrapped_dek_b64)` in that order and validate unwrap fields strictly. `rotate_keyring` returns only the nonsecret new ID and preserves the file owner/mode.

Add `cryptography==50.0.2` to `requirements-ci.txt` and install it into the temporary test venv.

- [ ] **Step 4: Run GREEN and negative controls**

Run: `/tmp/wasl-ci-venv/bin/python scripts/test_recovery_crypto.py`
Expected: all keyring, rotation, wrong-key, tamper and nonce-uniqueness tests PASS; no secret in captured stderr.

- [ ] **Step 5: Commit Task 1**

```bash
git add scripts/recovery_crypto.py scripts/test_recovery_crypto.py requirements-ci.txt
git commit -m "feat: add versioned recovery keyring and AEAD"
```

---

### Task 2: Encrypted v2 backup/verification and explicit legacy opt-in

**Files:**
- Modify: `scripts/recovery.py`
- Modify: `scripts/test_recovery.py`

**Interfaces:**
- Consumes: Task 1 keyring/AESGCM helpers.
- Produces:
  - `write_file_bundle(source, bundle, key_file, nats_json=None) -> dict` (v2)
  - `verify_bundle(bundle, key_file, legacy_hmac=False) -> dict`
  - `_decrypt_v2_payloads(bundle, keyring, manifest, destination) -> None`
  - CLI `rotate-key --key-file PATH`

- [ ] **Step 1: Write failing encrypted bundle tests**

Create synthetic run/NATS files and assert the installed bundle contains no plaintext markers, DEK or keyring keys. Verify fresh nonces/ciphertext for equal sources. Test wrong keyring, manifest/path/nonce/ciphertext tamper, truncation, extra files, unsafe paths and missing key IDs. Verify old HMAC fixture works only when `legacy_hmac=True` and refuses default use. Define the fixtures in `test_recovery.py`:

```python
class RecoveryV2Tests(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)
        self.keyring = self.root / "recovery_keys.json"
        make_keyring(self.keyring)  # production helper from recovery_crypto

def make_source_with_nats(root):
    source = root / "source"
    (source / "run").mkdir(parents=True)
    (source / "run/audit.json").write_text('{"synthetic":true}\n')
    nats = root / "nats.json"
    nats.write_text('{"streams":[],"marker":"synthetic clinical marker"}\n')
    return source, nats

def test_v2_payloads_are_encrypted_and_authenticated(self):
    source, nats = make_source_with_nats(self.root)
    bundle = self.root / "bundle"
    manifest = write_file_bundle(source, bundle, self.keyring, nats_json=nats)
    self.assertEqual(manifest["schema"], "wasl-file-backup-v2-aesgcm")
    raw = b"".join(path.read_bytes() for path in bundle.rglob("*") if path.is_file())
    self.assertNotIn(b"synthetic clinical marker", raw)
    self.assertEqual(verify_bundle(bundle, self.keyring)["key_id"], manifest["key_id"])
    ciphertext = bundle / "files/nats/nats.json.aead"
    with ciphertext.open("r+b") as handle:
        handle.truncate(3)
    with self.assertRaisesRegex(ValueError, "decrypt|authentication|truncated"):
        verify_bundle(bundle, self.keyring)
```

Keep v1 fixture creation local to tests; do not expose a production legacy-backup command.

- [ ] **Step 2: Run RED**

Run: `/tmp/wasl-ci-venv/bin/python scripts/test_recovery.py`
Expected: missing v2 schema/AEAD helpers or plaintext assertions fail.

- [ ] **Step 3: Implement v2 manifest, per-file encryption and strict verification**

Use schema `wasl-file-backup-v2-aesgcm`, fresh DEK/wrap nonce and per-file nonce. Store `.aead` paths, plaintext size/hash and ciphertext hash. Wrap AAD exactly as Task 1; file AAD binds schema, key ID, path, size and plaintext hash with an unambiguous canonical encoding. HMAC uses the active generation's independent `auth_key` over canonical manifest without `hmac_sha256`.

Verify exact authenticated file set before decrypting, enforce GCM/plaintext size/hash and reject extra/missing/unsafe files. `_decrypt_v2_payloads` writes exclusively into a caller-provided empty staging directory with mode 0700 and cleans on error. CLI `rotate-key` prints only the new key ID.

- [ ] **Step 4: Run GREEN**

Run: `/tmp/wasl-ci-venv/bin/python scripts/test_recovery.py`
Expected: all v1-opt-in and v2 negative/positive tests PASS; no plaintext or key material in bundle/logs.

- [ ] **Step 5: Commit Task 2**

```bash
git add scripts/recovery.py scripts/test_recovery.py
git commit -m "feat: encrypt recovery bundle payloads"
```

---

### Task 3: Transactional restore, deployment keyring and local replay comparison

**Files:**
- Modify: `scripts/recovery.py`
- Modify: `scripts/production_secrets.sh`
- Modify: `scripts/demo.sh`
- Modify: `deploy/production.compose.yml`
- Modify: `pipeline/recovery_test.go` only if an existing NATS restore seam needs instrumentation
- Create: `scripts/test_recovery_replay.py`

**Interfaces:**
- Consumes: Task 2 verification/decryption, existing `migrate_files`, Go `backup-nats`/`restore-nats` and logical digest functions.
- Produces:
  - `install_staging_with_rollback(staging: Path, destination: Path, replace: bool) -> dict` (the existing generation/rollback install factored from `migrate_files` without changing its journal)
  - v2 transactional file restore and authenticated NATS restore with unchanged logical hashes; v2 defaults for `recovery-backup`, `recovery-restore`, `recovery-verify`.

- [ ] **Step 1: Write failing restore/rotation/replay tests**

Test exact restored bytes/modes and rollback on a populated destination. After rotation, restore both prior and new bundles. Wrong/tampered bundles leave destination and NATS state untouched. Generate `recovery_keys.json` in a new temp secret directory and assert existing `backup_key` remains for legacy.

For replay, use a real single-node NATS: create messages/consumers/KV, run the Go `wsi7 backup-nats` command, include `nats.json` in encrypted v2, restore into a fresh empty NATS with `restore-nats`, then run `backup-nats` on the target. Compare the per-stream `logical_sha256_by_stream` maps emitted by the source and target Go summaries; do not reimplement the digest in Python. Assert restored API export `comparison_sha256` values by passing source/restored redacted fixture documents to the existing `soak.compare_exports` helper, rather than inventing a new comparison implementation.

- [ ] **Step 2: Run RED**

Run: `/tmp/wasl-ci-venv/bin/python scripts/test_recovery_replay.py`
Expected: v2 migrate/deploy keyring failures before implementation.

- [ ] **Step 3: Implement exclusive transactional restore and deployment wiring**

Verify before destination creation or NATS calls. Decrypt into empty restore staging, fsync, then use the existing generation/rollback install. Cleanup staging on success/failure without claiming physical erasure. Preserve append-only history. The restore core follows this explicit sequence:

```python
manifest = verify_bundle(bundle, keyring_path)
keyring, _, _ = load_keyring(keyring_path)
if destination.exists() and any(destination.iterdir()) and not replace:
    raise ValueError("migration destination is not empty")
staging = Path(tempfile.mkdtemp(prefix=".wasl-v2-restore-", dir=destination.parent))
try:
    _decrypt_v2_payloads(bundle, keyring, manifest, staging)
    install_staging_with_rollback(staging, destination, replace)
finally:
    shutil.rmtree(staging, ignore_errors=True)
```

Generate `recovery_keys.json` in `production_secrets.sh` without printing it. Update demo/deployment defaults to that file and keep legacy `backup_key` only for explicit old bundles. Do not change NATS empty-target guards or logical digest semantics.

- [ ] **Step 4: Run GREEN**

Run: `/tmp/wasl-ci-venv/bin/python scripts/test_recovery_replay.py`
Expected: restore/rotation/replay comparisons PASS with fresh single-node NATS; no plaintext staging remains.

- [ ] **Step 5: Commit Task 3**

```bash
git add scripts/recovery.py scripts/production_secrets.sh scripts/demo.sh deploy/production.compose.yml scripts/test_recovery_replay.py pipeline/recovery_test.go
git commit -m "feat: restore encrypted recovery bundles transactionally"
```

---

### Task 4: Full failure matrix, CI contracts and evidence

**Files:**
- Modify: `scripts/test_recovery.py`, `scripts/test_recovery_replay.py`, `scripts/test_ci_contract.py`
- Modify: `docs/RECOVERY.md`
- Create: `docs/reliability/evidence/confidential-recovery-bundles-2026-11-10.md`

**Interfaces:**
- Consumes: complete v2 implementation from Tasks 1–3.
- Produces: documented reproduction commands, limits and exact evidence.

- [ ] **Step 1: Complete negative and CI tests**

Cover wrong/missing/old/new keys, every tamper/truncation class, duplicate/unsafe paths, rollback, legacy opt-in, nonce uniqueness and log redaction. Add a workflow/CI contract assertion that `cryptography==50.0.2` is pinned and v2 keyring defaults are used. Verify no tracked key files or secret values exist in the diff.

- [ ] **Step 2: Run full verification**

Run: `/tmp/wasl-ci-venv/bin/python -m unittest discover -v -s scripts -p 'test_recovery*.py'`
Run: `/tmp/wasl-ci-venv/bin/python -m unittest discover -v -s scripts -p 'test_ci_contract.py'`
Run: `go -C pipeline test . -count=1`
Run: `go -C pipeline vet .`
Expected: all PASS with no secret output.

- [ ] **Step 3: Document accurate limits**

Update backup/restore/rotation runbook and evidence with exact counts/timings, bundle size, legacy opt-in and old-key retention responsibilities. Mark clean Linux Docker restore **not run** and do not claim physical secure erasure, external exactly-once, production RPO/RTO or a 24-hour soak.

- [ ] **Step 4: Request independent review**

Provide the complete diff, spec, crypto invariants and negative/replay evidence. Fix Critical/Important findings through RED-first regressions and repeat full verification.

- [ ] **Step 5: Commit final implementation and evidence**

```bash
git add scripts docs requirements-ci.txt deploy/production.compose.yml
git commit -m "docs: record confidential recovery bundle evidence"
```
