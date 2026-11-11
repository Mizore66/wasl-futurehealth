# Case evidence export (Workflow E)

**Status: wired in `runServe` and tested on a synthetic run.** The handler is in `pipeline/api_export.go`.

## Endpoint

```
GET /api/cases/{id}/export
```

`{id}` is the loop id (URL-encode the slashes, as for the other `/api/cases/` routes). The route requires an authenticated principal with `ActReadAudit`; it is intended for internal audit, not external disclosure. Missing/invalid credentials receive 401 and non-auditor roles receive 403.

| Status | When |
|---|---|
| 200 | Known case, GET with `ActReadAudit`. |
| 401 | Missing or invalid bearer token. |
| 403 | Authenticated role without `ActReadAudit`. |
| 404 | Unknown case, or a path that is not an export. |
| 405 | Any method other than GET on a known case. |

## Document

```json
{
  "schema": "wasl-case-export/1",
  "meta": { "...": "fixed redacted envelope, not log-derived metadata" },
  "detail": { "...": "default-deny redaction of GET /api/cases/{id}, without its meta" },
  "audit":  { "...": "the redacted body of GET /api/cases/{id}/audit, without its meta" },
  "comparison_sha256": "hex sha256",
  "content_sha256": "hex sha256"
}
```

- Before redaction, `detail` and `audit` are produced by `caseDetailBody(c)` and `caseAuditBody(c)` in `pipeline/api_export.go`.
  `caseDetail` serves a deep copy of the shared detail helper with registration keys pseudonymized; tests compare both redacted projections against the real wrapped live routes, including consent and ledger overlays.
- The export adds **no** `exported_at`, no file list and no other field. Nothing is written to the run directory.

## Hashes

The export carries two SHA-256 hashes over the same number-preserving canonical JSON (`canon`, sorted keys):

- `content_sha256` covers the **full export**, including `comparison_sha256` and `meta.loaded_at`. Only the
  `content_sha256` key itself is removed before hashing, preserving the existing content-hash contract.
- `comparison_sha256` covers the export with the `content_sha256` and `comparison_sha256` hash fields removed and
  only `meta.loaded_at` removed from the payload. The rest of `meta`, plus `schema`, `redacted`, `detail` and `audit`,
  are all included. Excluding the hash fields avoids self-reference; no other payload field is excluded.

To verify `content_sha256`, parse with number preservation, delete only `content_sha256`, serialise with sorted keys
and no extra whitespace, then compare the SHA-256 of those bytes with the served value. To verify
`comparison_sha256`, delete both hash fields and `meta.loaded_at`, then apply the same canonicalisation and comparison.
Both hashes are computed over the parsed form, so Go struct field order cannot change them.

### What the hashes do and do not prove

- `content_sha256` **authenticates the served document contents**: any change to a served field, a chain body, or a
  key breaks it (covered by `TestExportHashVerificationAndMutation`). It remains deterministic within one loaded run,
  but changes when `meta.loaded_at` changes after a restart.
- `comparison_sha256` is a **restart-stable comparison convenience**: equivalent redacted exports have the same value
  when only `meta.loaded_at` differs. A meaningful content change, including a new review, changes it. Mutating any
  covered field or adding, dropping or changing a key makes verification fail; deliberately changing `meta.loaded_at`
  alone does not.
- A matching `comparison_sha256` does **not** prove that two exports came from the same source run, that their load
  times match, that omitted or redacted source data was identical, or that the redaction was faithful. It compares
  only the covered served representation, subject to SHA-256 collision resistance.
- Neither hash is a signature or proof of server honesty. Anyone who can alter the document and recompute the hashes
  can produce internally consistent values. Neither hash verifies the original unredacted chain; chain scope is
  described separately below.

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
- **Registration-bearing fields use one deterministic pseudonym.** Link `referral_patient`, `weakest_edge.a/b`, identity `registration` on the secured detail route, and registration `pair` arrays in review/signoff/outreach/agent records and chain bodies become `reg_` plus SHA-256 of a domain-separated registration key. Equal source registrations therefore retain operational equality joins across detail, current/audit chains and snapshots without serving the literal key. The transform is idempotent and operates only on deep-copied served projections, never canonical chains or evidence. These unkeyed pseudonyms are deterministic and guessable for known registrations; they are not semantic anonymity or de-identification.
- **Citations are recursed into, not dropped.** Each `agent_investigations[].citations` entry keeps only `tool` and
  `field`; arguments, values (including exact DOBs), quote, excerpt, result and future keys are dropped. Identity entries
  likewise omit registration keys and retain only provider/role.
- **Listed strings still have a shape check.** Ordinary string leaves must be one structural token of at most 80
  characters from the identifier/date/hash character set; whitespace (including newlines), prose and longer strings
  are dropped. A bounded canary tripwire also rejects the synthetic canary in raw, JSON-escaped, Base64, hex and
  percent-encoded forms even when its outer shape matches an allowlisted structural field. Numbers, booleans and nulls
  remain. Server-generated operational fields such as review-item `reason`,
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

Both hashes use the generic-JSON form: Go's `encoding/json` with sorted object keys, no whitespace, HTML
characters escaped (`<`, `>`, `&` as `\u003c` etc.). The document is decoded with `UseNumber`, so number literals keep
the text produced by Go's marshaller (integers exact, floats in shortest round-trip form); they are not re-rounded
through float64. A verifier must also parse with number-preserving decoding (or only reserialise via the same rules);
a parser that converts integers above 2^53 to doubles will not reproduce either hash.

### Independent-review events

Protocol events keep `protocol`, `read`, `question`, `expected_head`, `resolution_reads`, `prev` and `hash` so slot order and the `r3` links to `r1`/`r2` stay verifiable; `actor`, `target_actor` and `note` are removed as for every other event. Decisions of unresolved questions are not served to the second reader; the export is auditor-only. `scripts/api_smoke.py` checks this with planted note and actor canaries.

## Remaining privacy limits

- Redaction is **not mathematical proof of de-identification** and not a guarantee that no clinical content remains.
  Default-deny schemas and string-shape checks limit accidental structural and prose leaks, but they do not establish
  HIPAA/GDPR de-identification, measure re-identification risk, or replace a disclosure review.
- A listed field is trusted to have its documented structural meaning. Labels, flags, rules, reader states,
  operational codes and identifiers can themselves contain sensitive or misleading single-token values; the
  projector checks their path and shape, not their clinical semantics. Unknown keys and strings outside the accepted
  shapes are dropped, but no prose classifier can recognise every possible representation.
- The recipient is an authenticated internal auditor, not an external or lower-privilege recipient. Case/report ids,
  provider ids, enum-like clinical labels, measurements, dates and hashes are deliberately preserved where listed and
  remain linkable. Registration keys are replaced by deterministic, linkable pseudonyms at the documented join fields; identity entries and citation arguments/values (including exact DOBs) omit the raw values. The export `meta`
  is rebuilt from fixed redacted constants and server-generated load time, not copied from logs or the normal envelope.
- The redacted chain **cannot be recomputed**. `audit.chain_verified` and `chain_length` were computed by the server over the
  original unredacted chain, and a third party cannot reproduce them from the export. They must trust the server-side
  verification flag. `content_sha256` authenticates the served bytes only, not that the redaction was faithful, and
  anyone who can alter the server can alter both.
- Number preservation requires a number-preserving parser (see above); a float64 parse changes integers above 2^53
  and the hash will not reproduce.

## Web download

**Built** (issue #46). The case audit page (`/app/patients/{id}/audit`) shows the export's `content_sha256` and
`comparison_sha256` below the chain and links to `/app/patients/{id}/audit/export`. That route handler fetches this
endpoint on the server with `WASL_API_AUDITOR_TOKEN` (`web/lib/api/export.ts`, marked `server-only`) and streams the
API's bytes unchanged as `wasl-synthetic-redacted-export_<sanitised id>.json`. A 401, 403 or 404 from the API becomes
a fixed message; the API's error body and metadata are never passed on. Without `WASL_API_URL` (fixtures mode) there
is no auditor API, so the action is disabled with that reason and no file is generated. `web/tests/fixtures/export_redacted.json`
is real handler output pinned by `TestExportContractFixture` (regenerate with `WASL_WRITE_EXPORT_FIXTURE=1`), and
`web/tests/audit-export.test.ts` checks byte-for-byte passthrough and recomputes both hashes from the downloaded bytes.

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
- Both hashes verify on the served document, and mutations to a case field, a chain body, the audit verification
  flag, a dropped key, or an added key are each detected. Number-preserving verification is checked for both hashes.
- Repeated exports are byte-identical. Changing only `meta.loaded_at` preserves `comparison_sha256` but changes
  `content_sha256`; a new review changes both the export and `comparison_sha256`.
- Building exports leaves every file in the run directory unchanged.
- 200, 401/403 through the secured route wrapper, 404 (unknown case) and 405 (POST, PUT, DELETE, PATCH) are returned
  as specified. Export error envelopes cannot leak log-derived metadata.
- Leak canaries: no salt, cache path, prompt path, reasoning or raw-message token, and no run directory path. Metadata,
  allowlisted structural fields and error bodies reject raw, escaped, nested, Base64, hex and percent-encoded canaries.
