# CI skip accounting and required fixtures

**Status:** built and locally verified. No hosted pass is claimed. No model/evaluation tuning, soak or AMD64 lane was run.

## Implementation

`check_go_test_skips.py` parses `go test -json` events and writes `wasl-go-test-skip-manifest/1` with test names, counts, required status and errors, excluding raw test output and clinical payloads. `ci_test_policy.json` is the exact-name policy. Package/test failures, malformed events, unexpected skips, required-test/subtest skips and missing required passes fail closed.

The `go-test`, `go-race` and `integration` jobs capture JSON, run the checker with `if: always()`, and upload raw JSON plus the manifest with `if-no-files-found: error`. Ordinary/race lanes may skip materialized-output tests because only integration generates those outputs. Integration requires passes for:

- `TestAPIFixturesAndContracts`
- `TestAPIReviewRBACAndAppendOnly`
- `TestAPIBookingSupport`
- `TestHL7RoundTripCohort`

The only additional allowed skips are `TestFresh2Reader`, `TestAPIDRFixtureRun`, and the two external-target NATS recovery drills. No evaluation corpus or external recovery target is enabled to make a skip disappear.

## Local evidence

- Parser/policy tests: **13/13 PASS**, including required-pass deletion, package events, subtests, empty/truncated logs, sticky failures, malformed JSON and non-string fields.
- Workflow contract tests: **23/23 PASS** with pinned `PyYAML==6.0.3` in a temporary Python 3.12 environment.
- Actual ordinary Go baseline: **8 allowed skips, 0 unexpected skips**.
- Integrated `scripts/demo.sh all`: **exit 0** with pinned NumPy/SciPy dependencies.
- Against the generated materialized outputs, all four required tests passed. `TestHL7RoundTripCohort` checked **4,618 of 32,327** resources. The integration checker reported `required_passed=4`, `failed=0`, `unexpected_skips=0`.

The local full Go suite after the separate bounded-dedupe and identical-copy recovery changes passed in **218.855s**. Those changes are commits `79869d7`, `e221867`, and `09c8b36`, outside this CI patch. Independent CI re-review found no remaining Critical or Important findings and returned **Ready to merge: Yes**. Hosted artifact upload and pipefail behavior must still be confirmed on the first GitHub run; local evidence does not cover them.
