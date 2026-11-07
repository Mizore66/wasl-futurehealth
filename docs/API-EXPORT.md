# Case evidence export (Workflow E)

**Status: built, tested offline on a synthetic run. Not yet routed in `serve`.** The handler exists in
`pipeline/api_export.go`; registering it on the mux is the integrator's step.

## Endpoint

```
GET /api/cases/{id}/export
```

`{id}` is the loop id (URL-encode the slashes, as for the other `/api/cases/` routes).

| Status | When |
|---|---|
| 200 | Known case, GET. |
| 404 | Unknown case, or a path that is not an export. |
| 405 | Any method other than GET on a known case. |

## Document

```json
{
  "schema": "wasl-case-export/1",
  "meta": { "...": "same envelope as the other API responses" },
  "detail": { "...": "exactly the body of GET /api/cases/{id}, without its meta" },
  "audit":  { "...": "exactly the body of GET /api/cases/{id}/audit, without its meta" },
  "content_sha256": "hex sha256"
}
```

- `detail` and `audit` are produced by `caseDetailBody(c)` and `caseAuditBody(c)` in `pipeline/api_export.go`.
  `caseDetail` and `caseAudit` in `api.go` should call these same helpers, so the bodies cannot drift.
- The export adds **no** `exported_at`, no file list and no other field. Nothing is written to the run directory.

## Hash

`content_sha256` = SHA-256 of the canonical JSON (`canon`, sorted keys) of the document with the
`content_sha256` key removed. To verify a served export:

1. Parse the JSON.
2. Delete `content_sha256`.
3. Serialise with sorted keys and no extra whitespace (`canon`).
4. SHA-256 the bytes and compare with the served value.

The hash is computed over the parsed form, so Go struct field order cannot change it.

### What the hash does and does not prove

- It **authenticates the document contents**: any change to a served field, a chain body, or a key breaks it
  (covered by `TestExportHashVerificationAndMutation`).
- It is **deterministic**: while the run and the case/review history are unchanged, repeated exports are
  byte-identical. A new review changes the hash, as it should.
- It is **not a cross-restart semantic ID**. `meta.loaded_at` is set when the server loads the run, so restarting
  the server with the same run changes the document and its hash. Compare exports by content hash only within
  one loaded run, or after removing `meta.loaded_at` if you need a restart-stable comparison.

## Chain verification scope

`audit.chain_verified` covers the **current chain only**: `c.last.Chain`, the hash chain of the case's latest
decision. It does not verify the whole run's history. Whole-run tamper evidence comes from `wsi7 tamper` and the
daily anchors (`audit.anchors`), which are outside this export.

## Known exposure (read before sharing an export)

The export serves what the detail projection already serves. In particular, each evidence block in
`detail.evidence` carries the reader's verbatim `quote`, which is a sentence from the clinical report. A real
materialised run therefore puts raw clinical text in the export. `TestExportKnownEvidenceQuoteExposure` pins this, so
a change in the projection will fail the test and force this section to be updated.

Before sharing an export outside the demo, either redact `quote` fields in the export only or accept the exposure
for synthetic data. Runs in this repository are synthetic (see `meta.note`).

## What the tests check

From `pipeline/`, run `go test -run TestExport -count=1 .`

- `detail` and `audit` are JSON-equal to the live `GET /api/cases/{id}` and `/audit` bodies.
- The hash verifies on the served document, and mutations to a case field, a chain body, the audit verification
  flag, a dropped key, or an added key are each detected.
- Repeated exports are byte-identical, and a new review changes the export.
- Building exports leaves every file in the run directory unchanged.
- 200, 404 (unknown case) and 405 (POST, PUT, DELETE, PATCH) are returned as specified.
- Leak canaries: no salt, cache path, prompt path, reasoning or raw-message token, and no run directory path.
