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

Three layers apply, in this order:

1. **Allowlisted projection** for evidence and timeline bodies: `detail.evidence`, every `evidence` block body in
   `review_items[].chain`, `current.chain`, `audit.chain` and `decisions`, and `detail.timeline`. Only listed keys
   survive (report id, labels, verdicts, agree flag, reader model, rule, flags, reader state strings, structured
   reader fields such as DR grades, BI-RADS, Lung-RADS and nodule sizes, image-reader scores and hashes; timeline
   seq/type/id/provider/status/day/category/raw_sha/source). Every other key is dropped whatever its value, so a one- or
   two-word clinical phrase under an unknown key cannot leak from these paths. Reader `quote` is not on the list.
2. **Citations** (`agent_investigations[].citations`) are recursed into, not dropped: each citation keeps `tool`,
   `arg`, `field` and `value`; any other key (quote, excerpt, result) is dropped, and a string `value` is kept only if it is a single
   identifier- or date-like token.
3. **Denylist and prose guard** for everything else (signoffs, api_reviews, review items, booking, outreach,
   consent_log, escalations, agent provenance, case, audit history):
   - exact keys `actor`, `target_actor`, `owner`, `canonical_owner`, `quote(s)`, `text`, `raw`, `raw_text`, `report_text`, `clinical_text`, `note`, `message(s)`, `prompt`,
     `prompt_text`, `reasoning`, `raw_report(_text)`, `document_text`, `narrative`, `comment(s)`, `summary_text`, `excerpt`,
     `snippet`, `free_text`, and keys ending `_quote(s)`, `_text`, `_note(s)`, `_message(s)`, `_prompt`, `_reasoning`,
     `_excerpt`, `_snippet`, `_narrative`, `_comment`. There is no `raw_`/`quote` prefix match, so `raw_id`, `raw_sha`
     and `quoted_by` survive. User-supplied actor and owner identity text is omitted from review history, audit history,
     coordinator overlays and case projections; roles, actions, decisions, timestamps and hashes remain;
   - operational `hint`, `reason`, `why` (and `*_hint`, `*_reason`) keep machine codes (lower-case tokens joined by
     underscores, e.g. `second_identifier`, `prep_grade_one_click`) or one of the server's own static sentences on
     server-generated paths. Coordinator `data` and `snooze` projections are user-controlled, so their operational
     strings are always dropped regardless of underscore formatting;
   - any other string that reads as prose (three or more tokens, at least two containing a letter) is dropped.

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

- Redaction is not a guarantee that no clinical content remains. Outside the evidence, timeline and citation
  allowlists, protection is a denylist plus a prose heuristic: a one- or two-word clinical phrase under an unlisted
  key in signoffs, api_reviews, review items, booking, outreach, consent_log, escalations, agent provenance or case
  fields is **not** caught, and neither is free text packed into one identifier-like token. Machine-code-shaped
  strings are trusted only outside coordinator/snooze user input. Not all possible prose can be identified.
- Listed values in the allowlists (labels, `flags`, `rule`, reader state strings, citation `value`) are trusted
  to be structured; they are not inspected beyond length, newline and prose checks.
- Identifiers (patient registration keys, report ids, names in `identities`, `meta.note`) are served as is.
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
- One- and two-word phrases under unknown keys in evidence, timeline and citations are dropped; hint/reason/why
  machine codes and `raw_id`/`quoted_by`/`raw_sha` are preserved; integers above 2^53 and extreme floats keep their text.
- The hash verifies on the served document, and mutations to a case field, a chain body, the audit verification
  flag, a dropped key, or an added key are each detected.
- Repeated exports are byte-identical, and a new review changes the export.
- Building exports leaves every file in the run directory unchanged.
- 200, 404 (unknown case) and 405 (POST, PUT, DELETE, PATCH) are returned as specified.
- Leak canaries: no salt, cache path, prompt path, reasoning or raw-message token, and no run directory path.
