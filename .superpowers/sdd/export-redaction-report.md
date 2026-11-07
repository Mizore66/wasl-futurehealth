# Export redaction report (Workflow E)

Branch feat/api-export-redaction, based on 6246507.

## Evidence
- `go test -run TestExport -count=1 .` in pipeline/: ok.
- `go vet .` clean; `go test -count=1 ./...` in pipeline/: `ok wsi 130.182s`.
- TestExportRedactsEveryPath plants CANARY-RAW-CLINICAL-TEXT under quote, gemma_quote, reasoning, an unlisted
  future_field, nested raw_text and list strings across: evidence, review_items (+ chain), current.chain, audit.chain,
  signoffs, api_reviews, timeline, agent_investigations (citations). Asserts none appear in the export bytes;
  chain_verified true, chain_length 4, chain_scope "current", redacted true; the hash-recompute note present; the served
  redacted chain does NOT verify; live detail/audit bodies and the original chain are byte-identical after export.
- TestExportNumberCanonicalisation: deterministic bytes; hash verifies when parsed with UseNumber.
- Existing hash/mutation, determinism, no-file-write, status and leak-canary tests still pass.

## Design
Deep copy by marshal/unmarshal (UseNumber) -> denylist keys (exact, raw_/quote prefix, _text/_note/_reason... suffix)
-> prose-value guard on all remaining strings. chain_verified/chain_length computed from the original chain first.
No exported_at, persistence or whole-run claim added. Quote keys omitted, no hashes of quotes.

## Remaining privacy limits
- Prose guard needs >=3 tokens with 2+ alphabetic: a 1-2 word clinical phrase under an unlisted key is served.
- Structured strings (label, flags, rule, reader_model, identity names, meta.note) are served as-is.
- The prose guard may drop legitimate multi-word strings (false positives); the denylist drops `hint` and `reason`
  fields that carry operational meaning (e.g. second_identifier reason).
- Redacted block hashes no longer verify; third parties cannot check chain integrity from the export, only trust the
  server-side chain_verified (current chain only).
- Number precision: verifiers must parse numbers without float64 conversion for integers above 2^53.
- Not covered: route is still unregistered in serve; access control on the export is out of scope.
