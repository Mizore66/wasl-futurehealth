# Security and privacy boundaries

**Status: hardening for the synthetic demonstration, not production certification.** The API still must never be used
with real clinical data. `wsi7 serve` requires the explicit `WASL_SYNTHETIC_DATA_ONLY=1` acknowledgment; that variable
is an operator acknowledgment, not a data classifier or proof that a run is synthetic.

## API authentication and read authorization

Every `/api` route requires `Authorization: Bearer <token>`. `WASL_API_TOKENS` is a comma-separated list of
`token:role:actor`; each token must have at least 16 characters and binds a role to one immutable actor. Missing or
invalid configuration prevents startup. Tokens are compared as SHA-256 digests and are not written to responses or logs.
The role/actor headers accepted by the old demo (`X-Wasl-Role`, `X-Wasl-Actor`) are overwritten from the authenticated
principal and have no identity value at the HTTP boundary.

- `ActReadQueue`: coordinator, clinician and reviewer tokens can read dashboard, fixture/list and raw case detail.
  Raw evidence quotes, review notes and linked identities are **clinical-by-design** and available only on this path.
- `ActReadAudit`: auditor tokens can read audit and export. Audit responses use the same default-deny redacted chain as
  the export; they no longer return the original clinical chain. Auditors cannot read raw detail or enumerate the raw
  queue. Export errors use the fixed redacted envelope too.
- Mutations retain action-specific RBAC. Review, consent, coordinator, undo and contest identities come from the token,
  so actor renaming and role-header spoofing cannot bypass reviewer separation.
- `/healthz` is the only unauthenticated route and exposes operational health, not case detail.

CORS is disabled by default. `WASL_API_ORIGINS` accepts an exact comma-separated origin allowlist; other origins receive
403 and wildcard credentials/origins are never emitted. Only `Authorization` and `Content-Type` are allowed in preflight.

The published `scripts/demo.sh` tokens are development-only constants for a loopback synthetic demo, not secrets or a
production identity system. A real deployment needs OIDC/JWT or mTLS principals, TLS, secret management, token rotation,
central authorization and a production threat review; none of those is claimed here.

## Export and snapshot disclosure policy

`GET /api/cases/{id}/export` is an **internal-audit export for an authenticated auditor**, not an external disclosure or
a de-identification claim. Its metadata is rebuilt from fixed server values. Log-derived policy, model, run and unknown
metadata are not copied. Identity registration keys and citation arguments/values (including exact DOBs) are omitted.
Case/report IDs, programme, labels, measurements, dates, linkage information and chain hashes intentionally remain for
internal audit and can be linkable; recipients must be access-controlled and the document must not be represented as
HIPAA/GDPR-deidentified.

The detail/audit projectors are default-deny. Canary regressions cover raw multiword text, escaped and nested values,
Base64, hex, percent/JSON-Unicode encodings, allowlisted fields, metadata and error responses. Both hashes are rechecked
after redaction. They authenticate document contents, not server honesty or source-chain fidelity.

New `scripts/demo.sh snapshot` output is derived only from the auditor-redacted export through
`scripts/redacted_snapshot.py`; raw detail/audit responses are not accepted. The converter fails closed on sensitive
fields and raw/escaped/nested/encoded canaries, adding empty strings only where the frontend schema requires a redacted
field.

**Intentional legacy exception:** the already committed `docs/api-samples/` and `web/data/api*` files retain synthetic
MTSamples/Wikidata demonstration quotes and registrations for the UI's clinical-by-design fixture mode. They are public
synthetic examples, not access-controlled clinical records, and must never be replaced with real data. This exception
does not apply to newly generated snapshots, which are redacted-only.

## Outbound clinical-text destinations

Live text/image readers and lung, DR, breast, LDCT and IFHAS cache builders accept only explicit loopback HTTP(S) URLs
(`127.0.0.1`, `::1` or `localhost`), without credentials, query/fragment or an unexpected base path. Unsafe live reader
configuration fails startup; calls fail closed with no request. Remote inference, TLS authentication and remote-host
allowlisting are not built.

Mock FHIR subscription callbacks must exactly equal `WASL_MOCK_CALLBACK_URL`, the configured loopback consumer hook
(the local `/hook` path is allowed), including when `full-resource` is selected; a different loopback port is rejected.
The orchestrator derives this URL from `SrcCfg.Hook`. The mock server and consumer use `WASL_MOCK_FHIR_TOKEN` and `WASL_MOCK_HOOK_TOKEN`; the
orchestrator generates independent random values per run instead of compiled-in bearer constants. Standalone mock and
consumer commands require those variables explicitly. Mock REST, subscription and bulk-export routes still share one
mock credential; separate production scopes are not built and non-synthetic use is not approved.

## Logs and materialized artifacts

Normal demo `link`/`hold` output no longer prints names, registrations or clinical quotes. Clinical run directories and
logs use 0700/0600; this covers raw FHIR/OUT/PREP dumps, review queues, reviewer decisions, `HOLD_ITEM.json`, API review
ledgers, reader/image ledgers, agent artifacts and model/cache outputs. Existing API/reader/image ledgers and NATS config
are chmodded when opened. Other pre-existing files are not recursively migrated; operators must tighten old run
volumes separately. Pipeline failures must not be treated as permission to log raw reports, prompts or bearer tokens.

## Independent-agent review: scope, findings and limits

Two independent review agents reviewed the pre-hardening tree at base `91cff71`. "Independent" means separate agents,
not an external professional security assessment. Their source review and targeted test evidence covered case list,
full/redacted detail, audit, export and metadata; snapshots/fixtures and serialized artifacts; review/consent/
coordinator/undo RBAC; mock FHIR subscriptions/full-resource delivery/status/events/bulk export; live reader and cache
builders; demo/stdout logs; path/identifier handling, error bodies, canonical hashes and permission modes. No SQL layer
exists in the reviewed API; case IDs are map keys rather than filesystem paths, so the reviewed export ID decoding does
not itself provide traversal.

Confirmed findings and disposition:

1. **High: unauthenticated wildcard-CORS raw clinical reads.** Fixed with mandatory bearer authentication, read RBAC,
   redacted auditor chains and origin deny-by-default.
2. **Medium: self-asserted mutation role/actor.** Fixed with token-bound roles and immutable actors.
3. **Medium: log-derived export metadata bypass.** Fixed with a fixed redacted envelope, bounded raw provenance,
   redacted errors and whole-document canary/hash regressions.
4. **Medium: linkable redacted record.** Registration and citation/DOB values were removed. The residual linkability is
   intentionally limited to authenticated internal audit and is not called de-identification.
5. **Medium: arbitrary reader and mock callback egress.** Fixed with loopback-only destinations across all reviewed
   clients and per-run environment credentials for the synthetic mock.
6. **Low: demo output and permissive artifacts.** Names/registrations/quotes were removed from normal demo output;
   clinical writers were changed to 0700/0600 and key legacy ledgers are tightened on open.

Limits of this remediation and review:

- Static bearer tokens are a demo control, not enterprise authentication. No external penetration test, professional
  privacy assessment, formal re-identification analysis or real-clinical-data approval was performed.
- Redaction remains path/shape based and cannot establish semantic de-identification. Chain hashes are not signatures.
- No remote inference or non-synthetic mock/FHIR deployment is approved. Production credential scoping, TLS, OIDC/mTLS,
  reverse-proxy hardening and secret rotation remain required.
- Tests use synthetic runs and intercepted transports; no external canary service receives a request in the tests. A
  full fresh seven-programme deployment and browser suite were not rerun in this isolated worktree. Web dependencies
  were absent, so the added Vitest authentication test is provided but was not executed here.
- Legacy public fixture text is synthetic-only by explicit exception. Existing filesystem modes outside the ledgers and
  NATS config tightened on open require a separate migration if old runs are retained.

## Primary regressions

- `pipeline/security_test.go`: authentication/RBAC/CORS, token identity binding, auditor redaction, metadata/errors,
  raw/encoded canaries, direct identifiers, loopback egress, mock credentials/callbacks, demo output and file modes.
- `pipeline/api_export_test.go`: path-sensitive redaction, deterministic hashes, live-source immutability and export
  status/hash contracts.
- `scripts/api_smoke.py`: real-HTTP unauthenticated/RBAC checks and whole-export canary scanning.
- `scripts/redacted_snapshot_test.py`: fail-closed snapshot conversion and raw/escaped/nested/encoded canaries.
- `web/tests/source-auth.test.ts`: reviewer/auditor tokens and removal of self-asserted identity headers (not executed
  in this worktree because web dependencies were unavailable).
