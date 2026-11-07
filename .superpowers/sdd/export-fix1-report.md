# Workflow E export fix 1

Branch feat/api-export-fix1, from fefa363.

## Build artifacts
- Removed from the index and disk: `pipeline/wsi` (Mach-O 64-bit arm64 executable, was mode 100755).
- `.gitignore` now ignores `/pipeline/wsi`. `bin/` was already ignored.
- Scan of tracked files for Mach-O/ELF executables found only that file. Scan by name for .test, .out, .prof,
  .pid, .log, nohup, .DS_Store, .pyc, .bak found only `pipeline/results/scale/x10/linker-restarts.log`.
  Kept: it is a sanitized evidence log (its first line says the original is retained in scratch), not a build artifact.
- No source or evidence file was removed.

## Redaction changes (pipeline/api_export.go)
- Allowlisted projection for evidence bodies (detail.evidence and every `evidence` block body in review item
  chains, current.chain, audit.chain, decisions) and for timeline rows. Unlisted keys are dropped whatever the value;
  reader `quote` is not listed.
- Citations are recursed into: tool, arg, field, value kept; other keys dropped; a string value must be a single
  identifier- or date-like token.
- Denylist: `hint` and `reason` removed from the exact-key list; broad `raw_` and `quote` prefix matches removed
  (`raw_id`, `raw_sha`, `quoted_by` survive). `_reason` suffix removed from the denylist.
- hint/reason/why (and `*_hint`, `*_reason`) keep a string only if it is a machine code or a known static server sentence.
- Prose guard unchanged for other fields.

## Tests (pipeline/api_export_test.go)
- TestExportRedactsEveryPath: plants CANARY-RAW-CLINICAL-TEXT separately in detail.evidence, review_items, review_items[0].chain,
  current.chain, audit.chain, signoffs, api_reviews, timeline, citations, booking, outreach, decisions[0].outreach,
  consent_log, escalations, agent_provenance, case and audit.history; first asserts each path contains it, then that
  "CANARY" and the denied keys never appear in the export. Also checks the redacted chain does not verify while
  `chain_verified` stays true, the original chain still verifies, and chain, review queue, live detail and live audit
  bodies are unchanged after export.
- TestExportAllowlistDropsShortPhrases: one- and two-word phrases under unknown keys in evidence, timeline and
  citations are dropped; labels, tool/field/value kept, both citations retained.
- TestExportKeepsSafeMetadata: hint/reason/why codes, raw_id, quoted_by, raw_sha and the static booking sentence kept;
  "poor prep" under reason dropped.
- TestExportLargeNumberCanonicalisation: 9007199254740993, 1e+21, 1.5e-7 served verbatim; hash verifies with
  UseNumber and does not reproduce with a float64 parse.
- Existing TestExport* tests updated where they called redactExport on the detail.

Red first: the new tests failed against the old implementation (canary, short phrases, citations and safe metadata
all failed); they pass after the change.

## Results
- `go test -C pipeline -run TestExport -count=1 -v .`: 11 tests PASS.
- `go test -C pipeline -count=1 ./...`: ok wsi 131.668s (full suite).

## Remaining limits
- Outside the evidence, timeline and citation allowlists, protection is a denylist plus a three-token prose
  heuristic. A one- or two-word clinical phrase under an unlisted key in signoffs, api_reviews, review items, booking,
  outreach, consent_log, escalations, agent provenance or case is not caught. Not all possible prose can be identified.
- Strings that look like machine codes under hint/reason/why are trusted; allowlisted values (labels, flags, rule,
  citation value) are trusted to be structured.
- Identifiers, names in identities and meta.note are served as is.
- Redacted chains cannot be recomputed; third parties must trust the server-side `chain_verified` flag (computed over
  the original chain). content_sha256 only authenticates the served bytes.
- Hash verification needs a number-preserving parser.
- Docs: docs/API-EXPORT.md updated to match.
