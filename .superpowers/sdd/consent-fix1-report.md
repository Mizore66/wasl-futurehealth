# Consent fix1 report

Branch: feat/api-consent-fix1 (from a50b495). api.go not edited. Status: NOT WIRED (route unregistered until integration).

## Fixes
1. Detail wrapper no longer decodes into map[string]interface{}. `spliceAPIContact` validates the upstream bytes with json.Valid and splices `"api_contact":{...}` before the closing brace, so large integers, ids, escapes, timestamps and key order are untouched. Content-Length from upstream is dropped. Test: TestConsentDetailPreservesLargeIntegers (in_seq 9007199254740993, a 30-digit number, `<` escape, nanosecond timestamp, key order). It failed before (in_seq became ...992, big became 1.23e+29).
2. withConsent now applies url.PathUnescape to the id from splitCasePath, matching caseRoutes. Test: TestConsentEscapedCaseIDMatchesCaseRoutes (`%252F` path: consent POST 201, detail overlay "denied", confirm_contact 409 with no ledger write). It failed before (detail had no overlay).
3. Overlay field renamed `pending_consent` -> `demo_pending_consent`; docs say it is a demo-only view distinct from canonical case.consent. Test: TestConsentOverlayFieldNotAmbiguous.
4. TestConsentDetailOverlayStatus (detail overlay status/blocked/pending/canonical for none, granted, denied, opted_out), TestConsentRepeatedAndDenyThenOptOut (deny->opt-out 201, repeated opt-out 201 with events=3, deny after opt-out 409, 3 ledger events). Comment in history test corrected.
5. docs/API-CONSENT.md: "Status: not wired"; wiring section says not yet registered.

## Evidence
- `go test -run Consent -v .`: 20 PASS (10 in api_consent_test.go), ok.
- `go test .` (full pipeline package): ok wsi 132.012s.
- go vet: clean. gofmt: api_consent*.go not listed (many pre-existing files are).
- Before the fix, 3 of the new tests failed as expected (detail status/field name, large int, escaped id).

## Remaining concerns
- Repeated opt-out appends a ledger event each time (by design: auditable); no dedupe.
- Splice requires upstream detail to be a JSON object; otherwise the response passes through without api_contact. If upstream already had an `api_contact` key, the output would hold a duplicate key (not the case today).
- Indented upstream output is re-indented with json.Indent (whitespace only).
- Route still needs registration in api.go by integration.
