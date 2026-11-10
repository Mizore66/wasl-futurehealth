# Confidential recovery bundles: local evidence

Date: 2026-11-10. Branch `reliability/confidential-recovery-aa26e439`, base `e54fc9b`. Status: **built** and locally tested (macOS, Python 3.12.16 in `/tmp/wasl-ci-venv`, Go 1.27.3, real single-node `nats-server` for replay). **Simulated/NOT RUN:** clean Linux Docker restore, physical second machine, 24-hour soak, model/eval runs.

## Commands and results

| Command | Result |
|---|---|
| `/tmp/wasl-ci-venv/bin/python -m unittest discover -v -s scripts -p 'test_recovery*.py'` | 62 tests, OK, 1 skipped (real HTTPS API drill, needs `WASL_TEST_API_*`); 7.0 s |
| `... -p 'test_ci_contract.py'` | 25 tests, OK |
| `... discover -s scripts -p 'test_*.py'` (all scripts) | 930 tests, 1 failure, 1 skipped; 107 s. The failure is `test_production_deploy.test_nats_starts_fresh_with_both_passwords_digit_leading`: Podman `connection refused` (no container daemon here); environmental, same as the checkpoint, not a product pass |
| `go -C pipeline test . -count=1` | ok, 225 s |
| `go -C pipeline vet .` | clean |

## Failure matrix covered (all assert refusal before any destination write)

- Keys: wrong keyring, missing file, raw legacy key used as v2 key, old generation pruned from keyring (old bundle refused, new bundle still readable), keyring holding only the old generation (new bundle refused), old-key retention after rotation, old generations unmodified by verification.
- Manifest: any field edit without re-signing, extra/missing/duplicate keys, truncation (empty, half, short), non-object, invalid UTF-8, unknown key id, bad wrapped DEK / nonce.
- Authenticated-but-invalid content: traversal, absolute, `.`/empty components, backslash, duplicate and file/dir clash for file and directory records, orphan parent directory, setuid modes, extra fields, wrong size/hash disagreeing with plaintext.
- Ciphertext: bit flip, truncation (zero, tag dropped, short), append, prepend, nonce swap, file swap, extra/missing/symlink/stray files, plaintext leak file at bundle root.
- Restore: rollback is single-use and refused for fresh-restore journals; failed NATS hook leaves no destination, staging or temp payload and names the manual purge; failed install restores the previous generation.
- Legacy: v1 refused by default for verify, migrate and CLI; accepted only with `--legacy-hmac`; v2 bundles refuse the legacy path.
- Nonces: 8 bundles x 12 identical files give all-unique file and wrap nonces, distinct wrapped DEKs and ciphertexts.
- Redaction: CLI failure output for six failure modes contains no key bytes (base64 or hex), raw key text, plaintext marker, wrapped DEK or wrap nonce, and no traceback.
- CI contract: `cryptography==50.0.2` in `requirements-ci.txt` and Dockerfile; python lane installs `requirements-ci.txt` and runs all `test_*.py`; `demo.sh` and compose default to `recovery_keys.json`, legacy never a default; no tracked key, `.aead`, `.pem` or `.key` files.

No new production behavior was needed; the added tests passed on first run against the Task 1-3 implementation (they are regression coverage, not RED-first fixes).

## Size and timing (single sample, 100,003-byte source, this laptop)

Bundle 100,973 bytes (about 1.0% overhead: 16-byte tags, 12-byte nonces, manifest). Backup 8.4 ms, verify 1.2 ms. Not a production RPO/RTO.

## Secret hygiene

Diff against `e54fc9b` scanned for base64 keys, private-key headers, 64-hex values: none. `git ls-files` has no `recovery_keys.json`, `backup_key`, `js_key` or `.aead`. Test keyrings are generated in temp dirs at run time.

## Limits

- Any failure after the NATS step begins needs a manual JetStream purge before retry; file/NATS atomicity is not claimed.
- Restore needs `/tmp` (or `TMPDIR`) capacity for the decrypted NATS payload and destination capacity for a staged copy.
- Cleanup is logical deletion, not physical secure erasure. Old key generations must be retained for as long as their bundles must be restorable; rotation does not re-encrypt.
- Legacy v1 bundles are unencrypted and explicit opt-in only.
- No external exactly-once guarantee, production RPO/RTO or 24-hour soak is claimed. Hosted CI results are unconfirmed until run on GitHub.
- Independent review (Task 4 step 4) was not performed in this session by instruction.
