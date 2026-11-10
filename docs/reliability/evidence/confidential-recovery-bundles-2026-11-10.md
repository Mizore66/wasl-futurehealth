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

## Size and timing (single sample, this laptop)

Reproduce (repo scripts plus stdlib; run from the repository root):

```bash
d=$(mktemp -d) && mkdir -p "$d/src/run" && /tmp/wasl-ci-venv/bin/python -c "open('$d/src/run/a.txt','w').write('a'*100000)" && echo '{}' > "$d/n.json" \
&& /tmp/wasl-ci-venv/bin/python -c "
import sys,time
from pathlib import Path
sys.path.insert(0,'scripts')
from recovery import write_file_bundle, verify_bundle
from recovery_crypto import new_keyring
d=Path('$d'); new_keyring(d/'k.json')
t=time.time(); write_file_bundle(d/'src',d/'b',d/'k.json',nats_json=d/'n.json'); a=time.time()-t
t=time.time(); verify_bundle(d/'b',d/'k.json'); v=time.time()-t
src=sum(p.stat().st_size for p in (d/'src').rglob('*') if p.is_file())+(d/'n.json').stat().st_size
size=sum(p.stat().st_size for p in (d/'b').rglob('*') if p.is_file())
print(f'source={src} B bundle={size} B backup={a*1000:.1f}ms verify={v*1000:.1f}ms')"; rm -rf "$d"
```

Observed once: `source=100003 B bundle=100973 B backup=8.4ms verify=1.2ms` (about 1.0% overhead; timings vary per run). Not a production RPO/RTO. The decrypted NATS payload size of the 9 Nov Podman drill was never recorded in any artifact, so no production-size figure is claimed.

## Secret hygiene

Commands (heuristic scan, not a proof of absence; the 64-hex pattern would flag any real key hash):
`git diff e54fc9b | grep -E '^\+' | grep -E '[A-Za-z0-9+/]{43}=|BEGIN .*PRIVATE|AKIA|[0-9a-f]{64}'` and
`git ls-files | grep -E 'recovery_keys|backup_key|\.aead$|js_key'`. The diff scan's only hit is the line in this document that quotes the scan command itself (a false positive); the file-name scan printed nothing. The CI contract test
`test_no_recovery_key_material_is_tracked` repeats the file-name check and is **skipped, not passed,** when git metadata is
unavailable. Test keyrings are generated in temp dirs at run time.

## Limits

- Any failure after the NATS step begins needs a manual JetStream purge before retry; file/NATS atomicity is not claimed.
- The compose `recovery` container has a read-only root and a **64 MB** `/tmp` tmpfs, the default writable scratch for the plaintext NATS payload in both backup and restore (set `TMPDIR` to a writable mount such as a volume if larger; the root filesystem is read-only); the 9 Nov drill's payload size is unrecorded, so fit is unproven. Destination capacity for a staged copy is also needed.
- Cleanup is logical deletion, not physical secure erasure. Old key generations must be retained for as long as their bundles must be restorable; rotation does not re-encrypt.
- Legacy v1 bundles are unencrypted and explicit opt-in only.
- No external exactly-once guarantee, production RPO/RTO or 24-hour soak is claimed. Hosted CI results are unconfirmed until run on GitHub.
- Review outcomes: Task 1 and Task 2 passed independent review. Task 3 review first returned Needs fixes (restore ordering, shell-level test) and was approved after the fixes. Task 4 was independently reviewed; after fix rounds 1 and 2 (documentation and test-assertion changes only) the re-review approved it. The reviews were run by the coordinator, not by this agent; their reports are not reproduced here. Final whole-branch review and hosted CI remain outstanding.
