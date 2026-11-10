# Confidential recovery checkpoint — incomplete

Date: 2026-11-10
Branch: `reliability/confidential-recovery-aa26e439`

**This is a work-in-progress checkpoint, not a completion or production-readiness claim.** The confidential encryption/restore implementation is built and locally tested, but the final failure matrix, CI evidence updates and whole-branch independent review are not complete. No Linux Docker or 24-hour run was performed.

## Completed commits

- `f4ad8d81a71020d3b8e0ce7827477e1fe59d8bcc` — keyring and AEAD primitives.
- `4cd7b92ec142281a376b825945078f7349f73772` — exact key IDs and locked rotation.
- `51a696652bbc029062da816a6b9f8d8ec4e70fdd` — encrypted v2 bundle backup/verification with explicit legacy HMAC opt-in.
- `78cc34574d357a3ccecd6f07b9f4e30b13e720f3` — transactional restore, deployment keyring and local replay comparison.

Design spec `5a0c27f` and implementation plan `38ec2b4` are committed.

## Review and test state

- Task 1 passed independent spec and quality review after fixes.
- Task 2 passed independent review with no Critical or Important findings. Deferred minors include ciphertext path/name limits, legacy validation and operational hardening.
- Task 3's 12 local replay tests pass with a real single-node NATS. Source backup, restore summary and target backup logical stream hash maps match. Recovery/crypto tests pass; CI contract and focused Go tests/vet pass.
- Task 3 independent review returned **Needs fixes** after the initial checkpoint. Important findings: `demo.sh` runs `restore-nats` before file installation, so a file-side failure can strand a populated NATS target and the empty-target guard blocks retry. There is no shell-level restore-order/NATS-failure test. The required next fix is to preflight/stage/decrypt before `restore-nats`, install files last, and add a stub-`wsi7` ordering/failure test; document that a failed `restore-nats` needs a manual purge before retry.
- The same review identified nonblocking follow-ups: mask special mode bits, apply directory modes deepest-first, journal before reporting cleanup failure, clean temporary plaintext directories on interruption, document keyring volume upgrade and Dockerfile dependency comments. No Critical findings were reported.
- `test_production_deploy.py::test_nats_starts_fresh_with_both_passwords_digit_leading` failed because Podman returned `connection refused`; this was not shown to be a base-regression and is not claimed as a product test pass.

## Known limitations requiring review

- A Python restore failure after `restore-nats` can leave the target NATS populated. The existing empty-target guard remains, but full file/NATS transactional rollback is not claimed.
- The v2 manifest gained authenticated `dirs` and per-file `mode`; pre-restore Task-2 bundles are not compatible with the new schema. This was pre-integration work, not a released migration.
- `extract-nats` writes the authenticated payload into a mode-0700 temporary directory for the Go restore command. Cleanup is logical, not physical secure erasure.
- Additional files beyond the original task list include `Dockerfile`, `production_volume_load.sh`, `docs/RECOVERY.md`, and an env-gated Go replay fixture test.

## Exact remaining steps

1. Complete Task 3 independent review and fix any Critical/Important findings with RED-first tests.
2. Finish Task 4: wrong/missing/old/new keys, tamper/truncation, unsafe/duplicate paths, rollback, legacy opt-in, nonce uniqueness, log redaction and CI dependency/default assertions.
3. Run full Python recovery/CI suites, full Go suite and vet; update evidence with exact results.
4. Run final whole-branch independent review. Do not integrate until it passes.
5. Keep clean Linux Docker restore and physical secure-erasure claims **not run/unproven**. Confirm hosted results only from GitHub after integration.

No secret files, keyrings, real credentials, raw logs or plaintext recovery payloads are committed. Uncommitted source files at checkpoint: none before this evidence file.
