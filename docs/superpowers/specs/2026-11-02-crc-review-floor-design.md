# CRC borderline review-floor experiment design

## Status and scope

Approved for a timeboxed development experiment on 2 Nov 2026. This design covers only review routing for candidates that retrieval already produced. It does not include scorer redesign, model training, retrieval expansion, report-attachment changes, frontend work, paid calls, publication, or a production/clinical-effectiveness claim.

The existing development funnel contains 60 `candidate_below_review` misses among 665 truth-closed CRC loops. Their median raw score is 0.000015; 43 score below 0.001. A floor of 0.01 routes 11 of the 60 and has a maximum possible attach-recall gain of 1.65 percentage points, before closure rules and reviewer decisions. The expected workload is about 49 additional referral-to-hospital review pairs, approximately 10% above the 507 baseline link items. This is a small, bounded experiment, not a fix for the 43 very-low-score misses.

## Safety invariant

Only review routing may become more permissive. The raw merge threshold remains 0.7, the closure tier remains 0.99, and the uncorroborated cap remains 0.989. Exact-EID conflicts, durable cannot-link constraints, contradiction confirmation, second-identifier rules, evidence rules, consent behavior, and all report-attachment rules remain unchanged.

A below-threshold candidate is only an untrusted review suggestion. It must never merge automatically, become trusted evidence, or trigger outreach without the existing human and closure paths. Truth remains restricted to offline evaluation.

## Runtime design

Add an opt-in global linker setting, exposed as `-linkfloor`, with default 0.2. Accept values greater than zero and at most 0.2; reject values outside that range. The setting applies to the shared linker, while evaluation cohorts remain CRC-focused. A CRC-only runtime switch would require programme knowledge before the linker receives it and is intentionally excluded.

Use the same configured floor at both boundaries:

1. `LinkerCore.Lo`, which puts an already-retrieved candidate into the existing near/review path after hard constraints have been applied.
2. The closure-side `LinkReview` checks, which currently discard near facts below 0.2. All programme implementations must use the shared configured value so a new near fact is not silently dropped.

Keep the equal-EID contradiction clamp at 0.2 even when the routing floor is lower. Do not weaken that existing contradiction path. Raw GBM score remains the routing input; calibrated probabilities and closure thresholds do not change.

Forward `-linkfloor` to orchestrate children, audit, replay, and report7. Stamp a floor-specific policy/configuration name so lanes cannot be mixed. Default 0.2 must preserve existing command-line arguments when the flag is absent and produce identical clinical facts, clusters, review decisions, and final states. The funnel config must record the actual floor.

Invalid flags, missing replay configuration, trace-write failures, and analysis errors must fail the evaluation loudly. Diagnostic failures must not alter clinical decisions.

## Tests

Use test-first changes for:

- default 0.2 parity with the pre-change path;
- a candidate between 0.01 and 0.2 remaining below review by default and entering the existing near path when enabled;
- the corresponding near fact producing a review item rather than being discarded by closure;
- no automatic merge below 0.7 and no closure below the existing tiers;
- EID conflict, cannot-link, sex/demographic contradiction, second-identifier, and uncorroborated-cap safeguards;
- the equal-EID contradiction clamp remaining 0.2;
- deterministic pair-level answer reuse without duplicate review items;
- flag validation, child/replay forwarding, policy naming, and live-versus-replay parity;
- funnel configuration and trace-write failure handling.

Run the complete Go suite, `go vet`, Python tests, shell syntax checks, diff checks, and secret scans after implementation.

## Paired development evaluation

Use the existing CRC-focused dev/v2 cohorts and seeds 101, 103, 107, with cached reader, image off, `-chaos 0`, retained diagnostic `-subkill 1`, and no NATS kill. Generate fresh paired lanes at the same code revision; do not reuse a binary or mix configurations.

Clean-review lanes:

| Arm | Floor | Seeds | Reviewer errors |
|---|---:|---|---|
| Baseline | 0.2 | 101, 103, 107 | ordinary 0; day-15 ungraded-prep 0 |
| Conservative | 0.05 | 101, 103, 107 | ordinary 0; day-15 ungraded-prep 0 |
| Primary | 0.01 | 101, 103, 107 | ordinary 0; day-15 ungraded-prep 0 |

Select at most one candidate after these runs. Selection requires a pooled CRC closure-recall gain of at least 1.0 percentage point over baseline, no false closures, no wrong-patient releases, no replay mismatches, no analysis/trace-write errors, and no increase in false-merge pairs, impure clusters, or cross-person edges. Report link-attach rate separately from CRC closure recall. Report reviewer items, distinct pairs, reviewer hours, unanswered items, and burden by programme/origin. If neither arm meets the gate, retain 0.2 and stop.

Stress-review lanes then compare baseline and the selected arm on the same three development seeds using ordinary reviewer error 0.04 and day-15 ungraded-prep reviewer error 0.25. These are synthetic stress assumptions, not measured clinician rates. Any false closure, wrong-patient release, replay mismatch, or increase in linkage contamination rejects the setting. Keep clean and stress results separate.

## Untouched validation

Freeze the selected code, floor, manifests, analysis plan, and the following seeds before the first validation run. Use eval people with v2 noise and CRC-focused cohorts. Seeds 211, 223, 227, 229, and 233 are reserved for this experiment; do not inspect them during development. Do not use cal/train people or the previous frozen/B8 seeds.

Run paired baseline and selected-arm clean-review lanes on all five seeds. Validation requires improved CRC closure recall, zero false closures, zero wrong-patient releases, zero replay mismatches, zero analysis/trace-write errors, and no increase in linkage contamination. Disclose reviewer cost, residual misses, confidence intervals, and all post-hoc changes. If the gate fails, retain the existing default and report the failed validation without retuning on these seeds.

## Evidence and handoff

Raw runs and manifests stay external under `~/wasl-ml-data/agents2/hardening/`. Public or committed summaries contain aggregate counts/settings only. Preserve baseline and failed-lane artifacts. Update results documentation with development versus validation, clean versus stress review, and simulated-data limitations. No merge, push, publication, or further scorer work is implied by this design.
