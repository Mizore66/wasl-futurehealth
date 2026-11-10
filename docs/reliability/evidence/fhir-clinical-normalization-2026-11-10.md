# Fail-closed clinical normalization evidence

Date: 2026-11-10
Scope: approved proposal #2 only
Required integration base: `68d5974ad08144ab642812f494449bc4f3dc188c`
Tested code revision: `59c05784003ed8e2db65ad47030c32c1ae78fb8b` (each follow-up production fix has a preceding RED test commit; subsequent evidence-only changes do not modify pipeline files)
Evidence type: local, newly constructed synthetic fixtures; direct FHIR durable-hook checks use one real local NATS node. No hosted-green, clinical-effectiveness, or production claim.

## Explicit Wasl ingest profile

Base FHIR optionality is not treated as permission to invent clinical meaning. The following are **Wasl profile requirements**, not claims that every base FHIR R4 field is mandatory. Existing transport checks for resource type/id, `meta.source`, positive `meta.versionId`, and valid `meta.lastUpdated` remain in force.

| Resource | Required clinical normalization |
|---|---|
| ServiceRequest | `authoredOn`; explicit `code.text` programme mapping. The structured `ifhas-screening` category retains its documented legacy CRC route only when the IFHAS engine is disabled, and routes to IFHAS when enabled. |
| Communication | Either `sent` or `received`; a present `received` takes precedence and must be valid (including non-empty string form). |
| Task | `lastModified`. |
| Appointment | `start`; explicit `serviceType.text` programme mapping. |
| DiagnosticReport | `effectiveDateTime`; a supported report code/category; a known `status`; revision rules below. Unknown or text-only report codes do not become colonoscopy/CRC. |
| Encounter | `period.start`. |
| Consent | `dateTime`. |

Accepted timestamp forms are RFC3339/RFC3339Nano dateTime with an explicit zone and the FHIR partial forms `YYYY`, `YYYY-MM`, and `YYYY-MM-DD`. Partial values anchor at midnight UTC. A genuine timestamp on cohort day 0 remains valid; a missing/malformed value returns an error instead of becoming day 0. Wasl-specific specimen-date extensions are validated when present.

Rejections are structured `NormalizationError` values persisted on skipped PREP records as `reject`, with resource identity and field where available. Codes are `invalid_timestamp`, `unknown_programme`, `invalid_report_status`, `invalid_report_version`, and `invalid_resource`. Direct poll/subscription validation and HL7 `ToResource` return the same errors; the prep status counter is `normalization_rejected`. The canonical FHIR subject, event path, reader contract, and valid evidence verdicts are unchanged.

## DiagnosticReport status and version policy

| Status | Normalization | Can supply closing evidence? |
|---|---|---|
| `final` | Accepted; legacy missing version is internal version 0, while transport metadata requirements still apply | Yes, if it is the current report version and ordinary evidence/link gates pass |
| `amended`, `corrected`, `appended` | Requires integer `meta.versionId >= 2` | Yes, only as the current version after explicit re-evaluation |
| `registered`, `partial`, `preliminary`, `unknown` | Accepted for state tracking | No |
| `cancelled`, `entered-in-error` | Requires integer `meta.versionId >= 2`; current facts are recomputed without the report | No; withdraws earlier evidence |
| Missing or any other spelling/value | Structured `invalid_report_status` rejection | No |

For one provider/registration/report ID, only a strictly higher version replaces the current report. A lower version is counted as `stale_report_version`; an equal-version semantic conflict is counted as `conflicting_report_version` and retires the active ID/version in either arrival order. Neither can restore an earlier final report. A newer non-final or withdrawal version reopens/re-holds the loop as dictated by ordinary rules. This is deterministic re-evaluation, not an assertion that a corrected report is clinically true.

The same eligibility helper gates CRC/cervical downstream facts and the lung, DR, breast, LDCT, and IFHAS same-registration fact builders. Human sign-offs do not make a filtered non-final report into a downstream fact.

Active evidence overrides and evidence review items are keyed by report ID and `meta.versionId`; the version is propagated through `DownFact`, `ReviewItem`, and `Signoff`. Versions zero and one intentionally share the historical replay identity; v2 and above are distinct. A v1 adequate decision remains in the append-only sign-off history but cannot close v2, suppress a distinct v2 review, or attach itself to automatic-v2 closure provenance. For v2+, the linker requires the sign-off's deterministic evidence item ID to match its loop/report/version/purpose. Prep query/reply Communications carry `urn:wsi:report-version`, so a v1 unit grade cannot close corrected v2.

Cross-report body dedupe includes status and version in its exact content key and retains normalized-body plus durable logical-report-lineage newest-version barriers, including for withdrawals and IDs not seen previously. Once IDs associate on active normalized content, a newer changed-body revision still retires the older associated IDs. Newer corrections/withdrawals retire older other-ID evidence; late older versions and equal-version semantic conflicts cannot restore closure. Replaced body-owner entries are removed even when the replacement body is empty, and an independent later report may reuse a released body. Exact same-version/status cross-transport duplicates remain deduplicated, and a newer own revision can still resolve the reviewed equal-generation duplicate case.

## HL7 conversion profile

`ToResource` applies OBR before specialised conversion or ZWS fallback:

- OBR-4 supplies report code or generic order programme text; unknown programmes fail with the same structured code as direct FHIR.
- OBR-6 supplies order `authoredOn`; OBR-7 supplies report `effectiveDateTime`. HL7 `YYYY[MM[DD]]` and second-precision/fractional `YYYYMMDDHHMMSS[.S...]` values are converted and validated. Other precisions are rejected.
- OBR-25 accepts exact FHIR status tokens and the Wasl mappings `F=final`, `C=corrected`, `P=preliminary`, `S=partial`, `O=registered`, `X=cancelled`, `E=entered-in-error`, and `U=unknown`. Missing/unknown values are rejected. Encoding collapses amended/appended to `C`; their evidence eligibility is identical, while exact `amended`/`appended` tokens are accepted on input.
- Generic report encoding now writes status at indexed OBR-25, matching specialised converters. Direct time parsing avoids float-day rounding in round trips.
- Before returning a publishable resource/message ID, `ToResource` requires ZWS-1 `ResourceType/id/vversion`, the ZWS-2 embedded body, and ZWS-3 to agree on type, id, and version. The embedded transport metadata must have a positive `versionId` and valid `lastUpdated`. DiagnosticReport OBR-3 must also equal the body id. A mismatch returns no publishable message ID/resource; the listener's existing error path NAKs instead of publishing and acknowledging it.
- Generic, DR, lung, and breast report rebuilds clear segment-owned `presentedForm`, `conclusion`, and `conclusionCode` before processing OBX. Omitting TX or CE/CWE therefore removes the corresponding embedded value/fallback rather than resurrecting stale content; present OBX values are still rebuilt. Generic CE and CWE conclusions retain the first `^` component, matching DR/lung coded-value behavior.
- Communication, Task, Appointment, Encounter, and Consent missing/malformed clinical times are tested through HL7 `ToResource` with the same structured rejection as direct FHIR. Valid embedded-body round trips have identical normalized events after canonical-JSON materialization. Their SCH/PV1/TXA/OBR time fields are not independently reconstructed; this is embedded-ZWS-body equivalence only.

## Behavior demonstrated

| Input | Result |
|---|---|
| Missing/malformed timestamp on any profiled resource | Structured rejection; no canonical event, closure, or outreach release |
| Valid full or documented partial timestamp | Accepted at the expected UTC cohort day |
| Unknown ServiceRequest/Appointment programme or DiagnosticReport code | `unknown_programme`; never implicit CRC |
| Empty programme reaching the linker directly | No loop; `unknown_programme` accounting |
| `registered`/`partial`/`preliminary`/`unknown` report | State may be tracked; no closing fact, closure, or outreach release |
| `cancelled`/`entered-in-error` revision | Earlier closing fact is withdrawn and the loop re-evaluates open |
| Newer corrected/amended/appended revision | Explicit re-evaluation; adequate current evidence may close, inadequate current evidence holds |
| v1 adequate human sign-off followed by corrected v2 | v1 decision remains historical; v2 inadequate/disagreeing/unavailable readers hold, with a distinct fresh review where required |
| Same-body/different-ID correction or withdrawal | Normalized-body and durable lineage barriers retire superseded evidence in either arrival order; exact duplicates retain lineage aliases, changed-body revisions keep group membership, and stale IDs/conflicts/former owners cannot restore closure |
| Legacy unversioned sign-off on v1 | Historical v0/v1 identity remains valid; exact version separation begins at v2 |
| v1 unit prep reply followed by corrected ungraded v2 | v2 awaits its own versioned reply instead of inheriting v1's grade |
| Automatic adequate v2 with historical v1 sign-off | Closure remains automatic; no historical human-signoff block or HumanAssisted flag |
| Mismatched ZWS/OBR/body identity | Conversion error with empty message ID/resource before the listener's durable publish/ACK |
| Generic/DR/lung/breast omitted TX or coded OBX | Corresponding embedded presented text/conclusion is absent instead of stale |
| Communication/Task/Appointment/Encounter/Consent missing or malformed time | Same `invalid_timestamp` code/field as direct FHIR; valid embedded-body replay events match |
| Stale or equal-version conflicting final report | Ignored; cannot reverse a withdrawal/correction |
| Valid current generated fixtures and direct durable-hook resources | Existing evidence and round-trip behavior retained |
| Live PREP versus JSON-materialized PREP replay | Identical final decision signatures in the focused status/version sequences |

## RED evidence before production changes

Initial table command:

```sh
go -C pipeline test -run '^(TestWaslTimestampProfileRejectsMissingAndMalformedValues|TestWaslTimestampProfileRejectsMalformedOptionalClinicalTimes|TestWaslTimestampProfileAcceptsDocumentedPartialDates|TestProgrammeMappingNeverDefaultsUnknownTextToCRC|TestLinkerDoesNotOpenCRCForEmptyProgramme|TestEveryDiagnosticReportStatusHasFailClosedEvidenceSemantics|TestSpecialisedDiagnosticFactBuildersRejectNonFinalEvidence|TestDiagnosticReportStatusCannotCloseOrReleaseAndReplayMatches|TestInvalidClinicalInputCannotReachCanonicalClosure|TestReportCorrectionsWithdrawalsAndStaleVersionsReevaluateSafely|TestRevisionAndWithdrawalStatusesRequireExplicitNewVersion|TestHL7ToResourceUsesClinicalSegmentStatusAndTimestamp|TestHL7ServiceRequestUsesSegmentProgramme|TestPartialTimestampExpectedEpochArithmetic)$' -count=1 -v
```

Result: **exit 1 (RED)**. Missing/malformed timestamps normalised at day 0; partial dates lost their real day; empty/unknown text became CRC; every non-final/withdrawn status produced facts; revision/withdrawal cases failed; and HL7 OBR values were hidden by the embedded resource.

Two follow-up gaps were also captured before their fixes:

```sh
go -C pipeline test -run '^TestDiagnosticReportProgrammeMappingIsExplicit$' -count=1 -v
go -C pipeline test -run '^TestHL7SpecialisedReportsAcceptWithdrawalMapping$' -count=1 -v
```

The first was RED for missing, unknown-coded, and text-only reports all becoming colonoscopy. The second was RED for breast `X=cancelled` (`invalid breast segments`), while DR and lung controls passed. Existing valid-fixture failures then exposed and fixed the generic OBR status position, float-day timestamp rounding, and missing explicit fixture fields without changing test expectations to permit unsafe behavior.

The independently reviewed follow-up regressions were also run before any follow-up production edit:

```sh
go -C pipeline test -run '^(TestVersionedHumanOverrideDoesNotCloseCorrectedReport|TestSameBodyDifferentIDCorrectionReevaluatesAndCleansOldBodyOwner|TestSameBodyDifferentIDWithdrawalRemovesClosingEvidence|TestHL7IdentityMismatchRejectedBeforeDurablePublish|TestHL7OmittedOBXDoesNotResurrectEmbeddedClinicalContent|TestHL7NonReportClinicalTimeValidationAndReplayEquivalence)$' -count=1 -json
```

Result: **exit 1 (RED)**. There were **24 failing leaf cases**: 3 v1-signoff→v2 cases, 1 same-body correction/old-owner case, 2 withdrawals, 9 HL7 identity mismatches, and 9 omitted-OBX cases. The v1 adequate control closed before each v2 failure. The new non-report table's **15 missing/malformed/valid replay leaf cases passed against the pre-fix code**, documenting existing normalization behavior rather than claiming a RED reproduction for that coverage item. JSON accounting, including parent tests and the package event, recorded 29 fail and 21 pass events.

Independent re-review then produced additional RED rounds before each corresponding production edit:

```sh
go -C pipeline test -run '^(TestCorrectedReportRejectsSignoffWithMismatchedReviewIdentity|TestLegacyUnversionedSignoffStillAppliesToVersionOne|TestVersionedPrepReplyDoesNotCloseCorrectedReport|TestHL7OmittedOBXDoesNotResurrectEmbeddedClinicalContent|TestHL7GenericCWECodedConclusionIsPreserved)$' -count=1 -json
```

Result: **10 failing leaf cases** for forged v2 sign-off identity, legacy v1 replay, v1 prep-reply reuse, six `conclusion` resurrections, and generic CWE loss. A follow-up component-bearing CE/CWE test was separately RED for both types (`current-coded-result^^L` retained whole).

```sh
go -C pipeline test -run '^(TestSameBodyDifferentIDCorrectionReevaluatesAndCleansOldBodyOwner|TestSameBodyDifferentIDWithdrawalRemovesClosingEvidence|TestSameBodyDifferentIDRevisionBypassesDuplicateOnlyForNewerOwnVersion|TestEmptyBodyRevisionCleansOldBodyOwner|TestCorrectedAutomaticClosureDoesNotReuseHistoricalSignoffProvenance)$' -count=1 -json
```

Result: **8 failing leaf cases**: other-ID adequate evidence surviving correction, 2 withdrawals, 2 equal-key early-break cases, 2 empty-body owner cases, and historical-signoff provenance on automatic v2. After linking the empty-body fixture, a temporary old-cleanup mutation reproduced both empty-body failures again before the fixed condition was restored.

```sh
go -C pipeline test -run '^(TestLateUnseenOlderIDCannotRestoreSupersededBody|TestBodyVersionBarrierHandlesHigherAndConflictingVersions)$' -count=1 -v
```

Result: **5 failing leaf cases**: reverse-arrival corrected/cancelled/entered-in-error, a higher other-ID version with older own history, and an equal-version/different-status conflict. Final cumulative independent read-only review of `59c0578` reported **no remaining Critical or Important findings**.

## GREEN commands and results

Focused clinical suite:

```sh
go -C pipeline test -run '^(TestWaslTimestampProfileRejectsMissingAndMalformedValues|TestWaslTimestampProfileRejectsMalformedOptionalClinicalTimes|TestWaslTimestampProfileAcceptsDocumentedPartialDates|TestProgrammeMappingNeverDefaultsUnknownTextToCRC|TestDiagnosticReportProgrammeMappingIsExplicit|TestLinkerDoesNotOpenCRCForEmptyProgramme|TestEveryDiagnosticReportStatusHasFailClosedEvidenceSemantics|TestSpecialisedDiagnosticFactBuildersRejectNonFinalEvidence|TestDiagnosticReportStatusCannotCloseOrReleaseAndReplayMatches|TestInvalidClinicalInputCannotReachCanonicalClosure|TestReportCorrectionsWithdrawalsAndStaleVersionsReevaluateSafely|TestRevisionAndWithdrawalStatusesRequireExplicitNewVersion|TestHL7ToResourceUsesClinicalSegmentStatusAndTimestamp|TestHL7SpecialisedReportsAcceptWithdrawalMapping|TestHL7ServiceRequestUsesSegmentProgramme|TestPartialTimestampExpectedEpochArithmetic|TestVersionedHumanOverrideDoesNotCloseCorrectedReport|TestCorrectedReportRejectsSignoffWithMismatchedReviewIdentity|TestLegacyUnversionedSignoffStillAppliesToVersionOne|TestVersionedPrepReplyDoesNotCloseCorrectedReport|TestSameBodyDifferentIDCorrectionReevaluatesAndCleansOldBodyOwner|TestSameBodyDifferentIDWithdrawalRemovesClosingEvidence|TestSameBodyDifferentIDRevisionBypassesDuplicateOnlyForNewerOwnVersion|TestLateUnseenOlderIDCannotRestoreSupersededBody|TestBodyVersionBarrierHandlesHigherAndConflictingVersions|TestEmptyBodyRevisionCleansOldBodyOwner|TestCorrectedAutomaticClosureDoesNotReuseHistoricalSignoffProvenance|TestHL7IdentityMismatchRejectedBeforeDurablePublish|TestHL7OmittedOBXDoesNotResurrectEmbeddedClinicalContent|TestHL7GenericCWECodedConclusionIsPreserved|TestHL7NonReportClinicalTimeValidationAndReplayEquivalence)$' -count=1 -json
```

Result: **226 test/subtest runs, 226 passes, 0 skips/failures across 31 explicitly selected top-level tests**, plus the package pass event (227 JSON `pass` events total).

Direct FHIR durable hooks with the repository-documented worktree-local `bin/nats-server` symlink:

```sh
go -C pipeline test -run '^(TestSetupStreamsCreatesBoundedDurableInbox|TestStatusRejectsMismatchedSubscription|TestUnknownSubscriptionSourceDoesNotAdvanceAcknowledgement|TestHookValidatesBeforeAcknowledgement|TestHookRejectsDuplicateNotificationEnvelopeKeys|TestHookRejectsInvalidInlineResourceAndFocusMismatch|TestHookRejectsOversizeAndBackpressure|TestSubscriptionRequestRejectsMalformedURL|TestSubscriptionReorderAndDuplicateHaveSingleOrderedEffect|TestHookAcceptedNotificationIsDurableBeforeConsumerRestart|TestHookDurablePublishFailureThenRetryStoresOneNotification|TestDurableInboxRestartReplaysAcceptedNotificationOnce|TestDurablePendingRecoveryRestartsReorderAndOverflow|TestCanonicalPublishBeforeInboxAckRedeliveryIsIdempotent|TestSubscriptionReorderOverflowRemainsDurablePending|TestInboxAcknowledgementWaitsForDurablePendingStore|TestGapFillReplaysDurableOverflowWithoutInbox|TestHookBoundsVreadAcceptanceWait|TestHeartbeatDoesNotTriggerPendingBacklogRescan|TestRecoverPendingRetainsDispatchedButUnacknowledgedRecord|TestPendingBeforeInboxAckRestartRedeliveryIsIdempotent|TestSubscriptionIdentitySurvivesRestartAndGapFill|TestSubscriptionCheckpointsAreScopedByIdentity|TestConcurrentSubscriptionInstancesDoNotDeleteNewerPendingOwner|TestConcurrentSubscriptionInstancesDoNotOverwritePendingEvent|TestSubscriptionEventProviderConflictFailsClosed|TestRecoverPendingSchedulesGapFillAfterStartupStatusFailure|TestRecoverPendingOwnershipConflictRetriesGapFill|TestRecoveredCheckpointRetriesStatusAfterStartupFailure|TestRecoveredCheckpointRetriesAfterInvalidStatusResponse|TestLegacyPendingUsesLegacyCheckpointSubscription|TestScopedPendingIgnoresOtherSubscriptionLegacyKey|TestLegacyOverflowExactRetryDoesNotDuplicateIdentity|TestConcurrentSubscriptionProviderConflictFailsClosed|TestDispatchedEventConflictFailsClosed|TestSingleNotificationConflictingDuplicateFailsClosed|TestSubscriptionEventPayloadConflictFailsClosed|TestGapFillRejectsMismatchedSubscription|TestLegacyCheckpointDoesNotRewindScopedCheckpoint|TestRecoverPendingClearsAlreadyAcknowledgedRecord)$' -count=1 -timeout=5m -json
```

Result: **54 test/subtest runs, 54 passes, 0 skips/failures across 40 explicitly selected top-level tests**, plus the package pass event. This is a new measured count, not a reuse of the earlier 54. `TestSetupStreamsCreatesBoundedDurableInbox`, `TestStatusRejectsMismatchedSubscription`, and `TestUnknownSubscriptionSourceDoesNotAdvanceAcknowledgement` each have an explicit JSON pass event. The direct hook focus/body mismatch control is also explicit; broad-prefix-only unrelated legacy review, pending-result, and recovery-digest tests are not selected.

Focused race lane:

```sh
go -C pipeline test -race -run '^(TestWaslTimestampProfileRejectsMissingAndMalformedValues|TestWaslTimestampProfileRejectsMalformedOptionalClinicalTimes|TestWaslTimestampProfileAcceptsDocumentedPartialDates|TestProgrammeMappingNeverDefaultsUnknownTextToCRC|TestDiagnosticReportProgrammeMappingIsExplicit|TestLinkerDoesNotOpenCRCForEmptyProgramme|TestEveryDiagnosticReportStatusHasFailClosedEvidenceSemantics|TestSpecialisedDiagnosticFactBuildersRejectNonFinalEvidence|TestDiagnosticReportStatusCannotCloseOrReleaseAndReplayMatches|TestInvalidClinicalInputCannotReachCanonicalClosure|TestReportCorrectionsWithdrawalsAndStaleVersionsReevaluateSafely|TestRevisionAndWithdrawalStatusesRequireExplicitNewVersion|TestHL7ToResourceUsesClinicalSegmentStatusAndTimestamp|TestHL7SpecialisedReportsAcceptWithdrawalMapping|TestHL7ServiceRequestUsesSegmentProgramme|TestPartialTimestampExpectedEpochArithmetic|TestVersionedHumanOverrideDoesNotCloseCorrectedReport|TestCorrectedReportRejectsSignoffWithMismatchedReviewIdentity|TestLegacyUnversionedSignoffStillAppliesToVersionOne|TestVersionedPrepReplyDoesNotCloseCorrectedReport|TestSameBodyDifferentIDCorrectionReevaluatesAndCleansOldBodyOwner|TestSameBodyDifferentIDWithdrawalRemovesClosingEvidence|TestSameBodyDifferentIDRevisionBypassesDuplicateOnlyForNewerOwnVersion|TestLateUnseenOlderIDCannotRestoreSupersededBody|TestBodyVersionBarrierHandlesHigherAndConflictingVersions|TestEmptyBodyRevisionCleansOldBodyOwner|TestCorrectedAutomaticClosureDoesNotReuseHistoricalSignoffProvenance|TestHL7IdentityMismatchRejectedBeforeDurablePublish|TestHL7OmittedOBXDoesNotResurrectEmbeddedClinicalContent|TestHL7GenericCWECodedConclusionIsPreserved|TestHL7NonReportClinicalTimeValidationAndReplayEquivalence|TestSetupStreamsCreatesBoundedDurableInbox|TestStatusRejectsMismatchedSubscription|TestUnknownSubscriptionSourceDoesNotAdvanceAcknowledgement|TestHookValidatesBeforeAcknowledgement|TestHookRejectsDuplicateNotificationEnvelopeKeys|TestHookRejectsInvalidInlineResourceAndFocusMismatch|TestHookRejectsOversizeAndBackpressure|TestSubscriptionRequestRejectsMalformedURL|TestSubscriptionReorderAndDuplicateHaveSingleOrderedEffect|TestHookAcceptedNotificationIsDurableBeforeConsumerRestart|TestHookDurablePublishFailureThenRetryStoresOneNotification|TestDurableInboxRestartReplaysAcceptedNotificationOnce|TestDurablePendingRecoveryRestartsReorderAndOverflow|TestCanonicalPublishBeforeInboxAckRedeliveryIsIdempotent|TestSubscriptionReorderOverflowRemainsDurablePending|TestInboxAcknowledgementWaitsForDurablePendingStore|TestGapFillReplaysDurableOverflowWithoutInbox|TestHookBoundsVreadAcceptanceWait|TestHeartbeatDoesNotTriggerPendingBacklogRescan|TestRecoverPendingRetainsDispatchedButUnacknowledgedRecord|TestPendingBeforeInboxAckRestartRedeliveryIsIdempotent|TestSubscriptionIdentitySurvivesRestartAndGapFill|TestSubscriptionCheckpointsAreScopedByIdentity|TestConcurrentSubscriptionInstancesDoNotDeleteNewerPendingOwner|TestConcurrentSubscriptionInstancesDoNotOverwritePendingEvent|TestSubscriptionEventProviderConflictFailsClosed|TestRecoverPendingSchedulesGapFillAfterStartupStatusFailure|TestRecoverPendingOwnershipConflictRetriesGapFill|TestRecoveredCheckpointRetriesStatusAfterStartupFailure|TestRecoveredCheckpointRetriesAfterInvalidStatusResponse|TestLegacyPendingUsesLegacyCheckpointSubscription|TestScopedPendingIgnoresOtherSubscriptionLegacyKey|TestLegacyOverflowExactRetryDoesNotDuplicateIdentity|TestConcurrentSubscriptionProviderConflictFailsClosed|TestDispatchedEventConflictFailsClosed|TestSingleNotificationConflictingDuplicateFailsClosed|TestSubscriptionEventPayloadConflictFailsClosed|TestGapFillRejectsMismatchedSubscription|TestLegacyCheckpointDoesNotRewindScopedCheckpoint|TestRecoverPendingClearsAlreadyAcknowledgedRecord)$' -count=1 -timeout=5m -json
```

Result: **280 test/subtest runs, 280 passes, 0 skips/failures across 71 explicitly selected top-level tests**, plus the package pass event (281 JSON `pass` events total), package time 15.613s. The same three formerly omitted durable tests have explicit race JSON pass events. This replaces the earlier broad-prefix 211 count with the measured final selection.

Full package and vet:

```sh
go -C pipeline test ./... -count=1 -timeout=10m -json
go -C pipeline vet ./...
```

Final full-run accounting: **1,513 test/subtest runs, 1,505 passes, 8 skips, 0 failures**, plus the package pass event (1,506 JSON `pass` events total), package time 163.251s. Vet passed. The eight intentional asset/environment skips were `TestAPIDRFixtureRun`, `TestAPIFixturesAndContracts`, `TestAPIReviewRBACAndAppendOnly`, `TestAPIBookingSupport`, `TestFresh2Reader`, `TestHL7RoundTripCohort`, `TestNATSBackupRestoreReadsRetainedMessagesOnEmptyCluster`, and `TestNATSBackupRestorePreservesRepeatedMessageIDs`.

`git diff --check` passed. `pipeline/RESULTS.md` was not changed because no final integrated demo/evaluation run was performed.

## Files changed

Production: `pipeline/types.go`, `rules.go`, `fhir.go`, `fhirpoll.go`, `fhirsub.go`, `services.go`, `linker.go`, `lung.go`, `dr.go`, `breast.go`, `ldct.go`, `ifhas.go`, `hl7.go`, `hl7lung.go`, and `hl7breast.go`.

Focused tests and explicit valid-fixture updates: `pipeline/clinical_normalization_test.go` (new), `agentreplay_test.go`, `agentshadow_test.go`, `confirm_test.go`, `consent_test.go`, `eidcluster_test.go`, `eidguard_fix_test.go`, `eidguard_test.go`, `fhirsub_durable_test.go`, `governance_test.go`, `handoffref_test.go`, `histclock_test.go`, `hl7_test.go`, `ldct_test.go`, `r1review_test.go`, and `v7_test.go`.

Documentation: `docs/reliability/ADVERSARIAL.md`, `adversarial-matrix.json`, and this report.

Follow-up production: `pipeline/types.go`, `linker.go`, `closure.go`, `governance.go`, `r1review.go`, `prepquery.go`, `gen.go`, `fhir.go`, `lung.go`, `dr.go`, `breast.go`, `ldct.go`, `ifhas.go`, `hl7.go`, `hl7dr.go`, and `hl7lung.go`. Follow-up regressions: `pipeline/clinical_revision_hl7_test.go`.

## Remaining limits

- Fixtures are newly minimal and synthetic. No evaluation corpus, held-out seed, model tuning, paid service, container, soak, AMD64 lane, or final integrated run was used.
- Real Malaffi/FHIR/HL7 profile mappings and optional-field behavior remain unverified. Status validation does not establish report truth, clinician agreement, reader accuracy, or clinical effectiveness.
- Direct FHIR durability tests inherit proposal #1's limits: no actual consumer OS-process kill, broker-accepted publish with lost acknowledgement, R3/multi-host partition, or live external inbox proof. The focused live/materialized comparisons are deterministic in-process checks, not a live Malaffi equivalence claim.
- HL7 identity tests exercise `ToResource`, the boundary immediately before the existing listener's JetStream publish/ACK. No socket-level MLLP ACK/durable-stream test or external feed was added for this follow-up. Non-report clinical-time equivalence uses the embedded ZWS-2 body; segment-only/wiped-ZWS reconstruction of Communication, Task, Appointment, Encounter, and Consent times is not supported or claimed.
- No reviewer-durability work, transport authentication, MLLP slow-client matrix, malformed-NATS dead-letter contract, or other proposal was implemented.
- No clinical denominators, evaluation metrics, `comparison_sha256`, hosted CI status, production readiness, or zero-loss result is newly claimed.
