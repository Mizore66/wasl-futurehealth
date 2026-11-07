# Shadow status fix1 report

Branch feat/api-shadow-status-fix1, base bb2d66b.

Findings fixed (pipeline/api_shadow_status.go):
1. done/degraded/killed/incomplete must all be present bools. Missing or wrong type: status unknown (done missing/non-bool, or done:false with other flags missing) or finished_degraded; clean_success only when all four present and done=true, others false. `running` requires all four flags. Test: TestShadowStatusMissingOrWrongTypeFlagsNeverClean (10 cases).
2. Status file empty/truncated/garbage/array/null/string/`{}`: 503 {available:false,status:"invalid"}, no clean_success. Missing file still 200 not_run. Test: TestShadowStatusInvalidFilesMakeNoLiveClaim (7 cases).
3. counts filtered to closed set from agentshadow.go (cache_changed, eligible, items_matched, link_items_matched, not_eligible, out_*, twins_skipped, outcome_{fallback,ineligible,skipped,recommendation}); fallback_reasons to the reason codes of agentshadow.go and scripts/agent_shadow_worker.py, plus provider_http_NNN and staging_error:<class> collapsed to staging_error. Values must be non-negative integers; non-map containers dropped. mode restricted to cached|live; scalar fields must be numbers. Tests: TestShadowStatusCountsAndReasonsClosedSet (AHS ids, paths, alias, names, script tags), TestShadowStatusScalarFieldsValidated.
4. health omitted entirely when file missing; ts must parse as RFC 3339, else omitted. Tests: TestShadowStatusMissingHealthStillAvailable (asserts no "health" substring in body), TestShadowStatusInvalidHealthTimestampOmitted.
5. No writes, no cache, GET only unchanged. docs/AGENT-SHADOW.md Status API section updated.

Evidence: `go test -run ShadowStatus -v .` in pipeline: 13 tests PASS (ok wsi). `go test .` in pipeline: ok wsi. gofmt clean for the changed files, go vet clean.
Red phase: new tests failed against the old handler before the rewrite (health ts "/tmp/leak/AHS-1" emitted, null status file returned 200).

Concerns: the reason set is hand-maintained; a new reason in Go or the worker needs adding here or it is silently dropped. Handler still unregistered in api.go (as before).
