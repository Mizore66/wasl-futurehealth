# Case evidence export (Workflow E)

**Status: wired in `runServe` and tested on a synthetic run.** The handler is in `pipeline/api_export.go`.

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
  `caseDetail` calls the shared detail helper; tests compare both projections against the real wrapped live routes, including consent and ledger overlays.
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

The projection is **default-deny and path-sensitive**. There is no generic fallback for an unrecognised section, key,
array element or chain body: a future field must be added deliberately to the relevant schema in
`pipeline/api_export.go` before it can be exported.

- **Every detail and audit section has an allowlist.** This covers case/current/decisions, linked identities,
  links/holds, review items/signoffs, API review and audit history, coordinator and consent overlays,
  outreach/consent transitions/escalations, booking, agent investigations/provenance, review-ledger status, audit
  signatures and anchors, as well as evidence and timeline. Nested arrays are projected element by element. Unknown
  keys are dropped regardless of whether their value is one word, two words, newline-free text or an identifier-shaped
  token.
- **Chain bodies are selected by block type.** Evidence, link, hold, closure, recommendation, human-signoff,
  auto-audit, escalation, surveillance-opened and outreach-guard bodies have separate operational schemas. An unknown
  block type keeps only its structural `type`/`prev`/`hash` shell and an empty body. Evidence keeps report ids, labels,
  verdicts, agree flags, reader model/rule/flags/state strings, structured DR/BI-RADS/Lung-RADS/nodule measurements and
  image-reader scores/hashes; reader `quote` is not listed. Timeline keeps
  seq/type/id/provider/status/day/category/raw_sha/source.
- **Citations are recursed into, not dropped.** Each `agent_investigations[].citations` entry keeps only `tool`,
  `arg`, `field` and `value`; quote, excerpt, result and future keys are dropped.
- **Listed strings still have a shape check.** Ordinary string leaves must be one structural token of at most 80
  characters from the identifier/date/hash character set; whitespace (including newlines), prose and longer strings
  are dropped. Numbers, booleans and nulls remain. Server-generated operational fields such as review-item `reason`,
  hold `why`, escalation codes and image/review `hint` accept lower-case machine tokens, including single-token codes
  such as `capacity` and `unowned`. The fixed server sentence used for a not-applicable booking is explicitly allowed.
- **Dynamic maps are bounded.** IFHAS domain-status/deadline maps accept only `bp`, `glyc`, `lipid` and `weight`.
  Audit anchors accept only the `fhir` key or partition-subject keys of the form `out.p<N>`; their hash values still
  pass the scalar shape check.
- **User-controlled identity and narrative fields are not listed.** `actor`, `target_actor`, `owner`,
  `canonical_owner`, notes/messages/prompts/reasoning, raw clinical text and coordinator/snooze reasons are omitted.
  Coordinator history data keeps only `until`, `outcome` and `channel`; a snooze overlay keeps only `at`, `event` and
  `until`. Roles, actions, decisions, structural timestamps and hashes remain. Agent provenance keeps only `cached`,
  `experimental`, `model` and `schema_ver`.

Quote keys are omitted entirely; no hash of a quote is served. Structured metadata, reader verdicts, labels, links,
consent states, ids, measurements and hashes are kept only at their listed paths.

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

- Redaction is **not mathematical proof of de-identification** and not a guarantee that no clinical content remains.
  Default-deny schemas and string-shape checks limit accidental structural and prose leaks, but they do not establish
  HIPAA/GDPR de-identification, measure re-identification risk, or replace a disclosure review.
- A listed field is trusted to have its documented structural meaning. Labels, flags, rules, reader states, citation
  values, operational codes and identifiers can themselves contain sensitive or misleading single-token values; the
  projector checks their path and shape, not their clinical semantics. Unknown keys and strings outside the accepted
  shapes are dropped, but no prose classifier can recognise every possible representation.
- Structural identifiers (patient registration keys, provider ids and report ids), enum-like clinical labels,
  measurements and hashes are deliberately preserved where listed. The normal response `meta` envelope, including
  `meta.note`, is not part of the detail/audit redactor and is served as is.
- The redacted chain **cannot be recomputed**. `audit.chain_verified` and `chain_length` were computed by the server over the
  original unredacted chain, and a third party cannot reproduce them from the export. They must trust the server-side
  verification flag. `content_sha256` authenticates the served bytes only, not that the redaction was faithful, and
  anyone who can alter the server can alter both.
- Number preservation requires a number-preserving parser (see above); a float64 parse changes integers above 2^53
  and the hash will not reproduce.
## What the tests check

From `pipeline/`, run `go test -run TestExport -count=1 .`

- `detail` and `audit` equal the live bodies after redaction; `CANARY-RAW-CLINICAL-TEXT` is planted separately in every
  path (evidence, review items and chains, current and audit chain, signoffs, api_reviews, timeline, citations, booking,
  outreach, consent_log, escalations, agent provenance, case). The test first asserts each path carries it, then that it
  never appears in the export; `chain_verified` stays true, the redacted chain does not verify, and live bodies and the
  original chain are unchanged.
- One-word, two-word, newline-containing and identifier-shaped clinical canaries under unknown keys are dropped in
  evidence, timeline, citations and every remaining detail/audit section, including nested arrays, chain bodies and
  dynamic maps. Unknown string shapes under listed keys are also dropped.
- Intended structural ids, enums, measurements, hashes and server operational fields are pinned, including single-token
  escalation/booking codes, IFHAS dynamic-map values, coordinator/consent metadata and audit anchors. User-controlled
  actor/owner fields and coordinator/snooze reasons stay out. Integers above 2^53 and extreme floats keep their text.
- The hash verifies on the served document, and mutations to a case field, a chain body, the audit verification
  flag, a dropped key, or an added key are each detected.
- Repeated exports are byte-identical, and a new review changes the export.
- Building exports leaves every file in the run directory unchanged.
- 200, 404 (unknown case) and 405 (POST, PUT, DELETE, PATCH) are returned as specified.
- Leak canaries: no salt, cache path, prompt path, reasoning or raw-message token, and no run directory path.
