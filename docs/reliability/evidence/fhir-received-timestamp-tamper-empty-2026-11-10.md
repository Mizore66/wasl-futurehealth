# FHIR received-only timestamps and empty tamper evaluations

**Status:** root cause reproduced; both fixes built and tested. This was a bounded diagnostic and unit/integration-test investigation, not a new soak or a green end-to-end smoke claim. The failed runtime, report and service logs remain preserved under `pipeline/results/soak/runs/20261110T024209Z-989dc4/`. No 24-hour run was started or scheduled.

## Evidence and root cause

The short smoke stopped after 550 canonical FHIR messages and then failed during tamper evaluation. Its preserved state showed:

- before the planned subscription kill: `published=550`, `buffered=100`, `durable_pending=100`, `next=551`;
- after replacement: `replayed_pending_events=100`, but the same 100 later events still waited for missing event 551;
- the mock served 935 `$events` requests and returned 1,000 historical events per request, while the consumer recorded zero `gap_fills`;
- the first gap batch therefore failed during decode, before absorption or canonical publish. This was not loss of the 100 accepted durable events.

A deterministic regeneration of the same synthetic smoke cohort isolated the first decode rejection at event **581**, `Communication/AHS-sr-48-comm-2`: `invalid_timestamp`, field `sent`, detail `timestamp is required`. That event is inside the first 551–1550 gap response, so one received-only Communication rejected the whole batch on every retry. Subsequent diagnostic rejections had the same received-only shape.

Commit `9bb5491` made timestamp normalization fail closed but unconditionally parsed `sent` before the existing `received` override. The generator's inbound Communications carry `received` without `sent`, matching the previously accepted precedence. The durable FHIR changes exposed this generator/normalizer mismatch; they did not cause the ingest stall. `ingest_lost=31699` was the resulting generated-versus-ingested accounting signal, not a separate accounting-code root cause. Zero closed predictions followed from the stalled evidence ingest.

## Fixes

Communication normalization now selects `received` when its key is present and otherwise requires `sent`. Existing and new tests reject malformed, empty, null and non-string `received` values, even with valid `sent`, and Communications with neither timestamp. No generator, durable replay, checkpoint or accounting code changed.

Tamper evaluation now returns an error when no closed human-signed prediction exists instead of constructing attacks with target index -1. The CLI prints the error and exits 2. Against the preserved failed runtime it now reports:

```text
tamper: no closed human-signed prediction; refusing tamper evaluation
```

with exit code 2 and no panic.

## Regression and verification evidence

- `TestCommunicationReceivedOnlyUsesReceivedTimestamp` failed before the normalization fix and passes after it, including day precedence and PREP acceptance.
- `TestCommunicationRejectsMalformedReceivedWithValidSent` reproduced empty/null/numeric bypasses before the key-presence fix; all reject with `invalid_timestamp` after it. Existing missing-both and malformed-string tests also pass.
- `TestSubscriptionBatchAcceptsReceivedOnlyCommunication` failed on the original code with the same `sent`-required error and passes with an ordered sent-only/received-only batch.
- `TestReplacementConsumerReplaysFullPendingSet` proves a replacement publishes all three durable pending events, reaches HWM 3 and leaves zero pending records.
- `TestTamperRejectsRunWithoutClosedHumanSignedPrediction` failed before the error-return contract existed and passes with the guard; its globals are restored after the test.
- `TestTamperCLIRejectsEmptyPredictionsWithoutPanic` executes the real `main()` in a helper process and asserts exit 2, the diagnostic and no panic.
- Focused durable recovery/gap-fill tests passed.
- A diagnostic decode of all **32,327** regenerated smoke events completed with zero normalization failures.
- Full `go -C pipeline test . -count=1` passed after the review fixes in **161.171s**; `go -C pipeline vet .` also passed.
- Independent final review found no remaining Critical or Important issues and returned **Ready to merge: Yes**.

No new container smoke was run after these fixes. The earlier short smoke remains failed and is not represented as green.
