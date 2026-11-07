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

- Before redaction, `detail` and `audit` are produced by `caseDetailBody(c)` and `caseAuditBody(c)` in `pipeline/api_export.go`.
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

## Redaction

The export is a **redacted copy**; the document carries `"redacted": true`. `detail` and `audit` are deep-copied
(marshal/unmarshal) before redaction, so the live `GET /api/cases/{id}` and `/audit` bodies, the source maps and the
original chain are never modified.

Removed (key and value, at any depth, on every path: `detail.evidence`, `review_items` and their chains,
`current.chain`, `audit.chain`, `signoffs`, `api_reviews`, `timeline`, `agent_investigations` including `citations`):

- keys named `quote`, `text`, `raw`, `raw_text`, `report_text`, `clinical_text`, `note`, `reason`, `message`, `messages`,
  `prompt`, `prompt_text`, `reasoning`, `raw_report`, `raw_report_text`, `document_text`, plus `narrative`, `comment(s)`,
  `excerpt`, `snippet`, `hint`, `free_text`, `citation(s)`;
- any key starting `raw_` or `quote`, or ending `_quote`, `_text`, `_note(s)`, `_reason`, `_message(s)`, `_prompt`,
  `_reasoning`, `_excerpt`, `_snippet`, `_narrative`, `_comment`;
- any string value that reads as prose (three or more whitespace-separated tokens, at least two containing a letter),
  whatever its key, so an unlisted future free-text field is dropped too.

Quote keys are omitted entirely; no hash of a quote is served. Structured metadata, reader verdicts, labels, links,
consent, ids and hashes are kept.

`audit` adds `chain_scope: "current"` and `chain_note`. `chain_verified` and `chain_length` are computed from the
**original** chain before redaction. Block hashes were computed over the original bodies and **cannot be recomputed
from the served bodies**; do not run `verifyChain` over the export and expect true.

### Canonicalisation and number precision

`content_sha256` hashes the generic-JSON form: Go's `encoding/json` with sorted object keys, no whitespace, HTML
characters escaped (`<`, `>`, `&` as `\u003c` etc.). The document is decoded with `UseNumber`, so number literals keep
the text produced by Go's marshaller (integers exact, floats in shortest round-trip form); they are not re-rounded
through float64. A verifier must also parse with number-preserving decoding (or only reserialise via the same rules);
a parser that converts integers above 2^53 to doubles will not reproduce the hash.

## Remaining privacy limits

The prose guard is heuristic: a short clinical phrase (fewer than three tokens) under an unlisted key, or free text
packed into an identifier-like string, is not caught. Structured fields such as `label`, `flags`, `rule`, names in
`identities`, and `meta.note` are served as is. See `.superpowers/sdd/export-redaction-report.md`.

## What the tests check

From `pipeline/`, run `go test -run TestExport -count=1 .`

- `detail` and `audit` equal the live bodies after redaction; a canary sentence planted under many keys on every path
  never appears, `chain_verified` stays true, and live bodies and the original chain are unchanged.
- The hash verifies on the served document, and mutations to a case field, a chain body, the audit verification
  flag, a dropped key, or an added key are each detected.
- Repeated exports are byte-identical, and a new review changes the export.
- Building exports leaves every file in the run directory unchanged.
- 200, 404 (unknown case) and 405 (POST, PUT, DELETE, PATCH) are returned as specified.
- Leak canaries: no salt, cache path, prompt path, reasoning or raw-message token, and no run directory path.
