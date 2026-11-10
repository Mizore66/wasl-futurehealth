# Security and privacy assurance remediation

Date: 2026-11-10. Scope: integrated branch base `8b046a841bc1bbf9e76141414c4d7c1fb1a32926` plus the uncommitted remediation reviewed here. Status is **built and locally tested**; this is not an external professional security assessment or proof of security.

## Independent findings and remediation

A fresh read-only agent reviewed API authorization, export/disclosure surfaces, registration pseudonymization, snapshots, metadata, logs and encoded canaries. The coordinator reproduced the findings before changing production code. Every production fix has a tracked regression that failed for the intended reason before the fix.

- Basic authentication could listen over plaintext HTTP. Startup now requires both TLS files whenever `WASL_API_USERS_FILE` is configured.
- Successful health and audit responses used operator/log-derived metadata. They now use the fixed redacted export envelope; method errors on case routes use the same envelope.
- Public read snapshots copied unknown fields and narrative notes. Health, dashboard, fixture and case-list snapshots now use per-response schemas, required top-level fields, a fixed nested known-key inventory, fixed operational text, and fixed metadata. Unknown JSON filenames are rejected before publication.
- Nested Base64 and folded Base64 bypassed synthetic canary detection. Go and Python now perform bounded recursive decoding with candidate limits, handle malformed/interior encoding junk, check non-UTF-8 bytes for ASCII canaries, and preserve the benign four-layer boundary.
- Coordinator snooze reasons and unknown nested fields are removed from public snapshots. Arbitrary prose is not accepted as an operational string.

Independent follow-up review found seven additional cases, including nested token-shaped clinical fields and audit method-error metadata. These were reproduced and fixed. Its final scoped review then found one Go-only interior-junk Base64 case; a failing-first regression was added and the decoder now handles it.

## Fresh verification

Commands were run from the repository root unless noted.

| Verification | Result |
| --- | --- |
| `go -C pipeline vet .` | Passed |
| `go -C pipeline test -count=1 -timeout=20m .` | Passed, 236.488 s on the final run |
| Focused Go security regressions with `-race` | Passed, 3.142 s |
| `redacted_snapshot_test.py` | 14 tests passed |
| `test_api_smoke.py` | 40 tests passed |
| `test_deploy.py` | 32 tests passed |
| All scripts except the disclosed Podman-only case | 871 tests passed, 1 unrelated external-target skip, 0 failures/errors |
| Tracked public health/dashboard/fixture/seven-programme case-list inputs | 10 inputs projected without schema errors |
| `git diff --check` | Clean |

The Python exclusion is exactly `test_nats_starts_fresh_with_both_passwords_digit_leading`: the local Podman daemon refused the connection. It was not counted as passing. Hosted/container checks must establish that lane independently.

The final independent scoped re-review reported **no remaining findings**. It confirmed that the exact interior-junk payload is rejected by both `exportCanaryString` and `exportScalarOK`, while the benign four-layer Base64 boundary remains accepted. This was a read-only agent review, not a professional security assessment.

## Limits

- Tests use synthetic data and local handlers only. No live deployment, real patient data or full seven-programme production run was used.
- The canary is a synthetic regression tripwire, not a general clinical-text classifier. Default-deny schemas are the primary public-snapshot boundary.
- Agent review is independent of the implementation author but is not an external professional security/privacy assessment.
- No clean Linux Docker restore, second physical machine restore or 24-hour soak is claimed.
