# CRC link-recall design

## Status and scope

Post-hoc clean-review sensitivity measured 543 missed truth-closed CRC loops among 3,865 across five held-out v2 seeds. The largest first-barrier family was 326 loops with no trusted link and no review question, accounting for 8.43 percentage points of recall loss. These observations motivate development but do not authorize tuning on the frozen or B8 seeds.

This design covers candidate-funnel instrumentation, tolerant candidate retrieval and review-routing experiments. It does not change the closure tier, evidence rules, second-identifier requirements, consent behavior or default EID-guard setting. It does not authorize training on evaluation people, paid calls or publication.

## Safety invariant

Candidate retrieval may become more permissive. Merge and closure decisions must not become more permissive by implication.

A retrieved candidate is only a suggestion for scoring or human review. Exact-EID conflicts and durable cannot-link constraints remain enforced. Existing contradiction handling, pair-level confirmation, second-identifier checks, the uncorroborated cap and the 0.99 closure tier remain unchanged. Every experiment must report false closures, wrong-patient releases and linkage contamination alongside recall.

## Current funnel

The canonical path is:

1. Referral and procedure registrations enter `prep_worker.go`.
2. `linker.go` builds or updates registration records and generates candidate pairs through exact-key blocks and the HNSW name index.
3. Candidates receive features, a LightGBM score and calibrated probability. Demographic and EID constraints can reject or constrain them.
4. The current raw merge threshold is 0.7; the review floor is 0.2. The HNSW path currently uses top-K 16, efSearch 100 and efConstruction 64.
5. Merge and second-identifier decisions use the existing review queue. Registration-pair answers can be reused across loops.
6. Closure workers attach evidence only when the relevant cluster and loop rules permit it; closure still uses `v7strict` and its 0.99 link tier.

The existing L5 label means that no trusted report link and no review question were observed. It does not distinguish retrieval failure, score filtering, demographic rejection or report-attachment failure.

## Design

### 1. Opt-in candidate-funnel diagnostics

Add diagnostics at the existing candidate and attachment boundaries. Default behavior and decision output must remain byte-identical when diagnostics are off.

For evaluation only, truth-aware offline analysis may identify the expected referral/procedure registration pair and classify its furthest observed stage:

- `candidate_not_generated`: the expected pair is absent from all retrieval paths;
- `candidate_below_review`: retrieved and scored, but below the review floor;
- `candidate_rejected_demographic`: retrieved but rejected or excluded by a demographic/EID rule, with a reason subcategory;
- `candidate_routed_review`: a review item was raised, with answered/unanswered and same/different outcomes recorded separately;
- `linked_report_not_attached`: registrations share an eligible cluster but the closing report did not attach to the CRC loop;
- `attached_or_closed`: the report reached the loop or the loop closed.

Truth labels are diagnostic inputs only. They must never influence runtime candidates, scores, merges, reviews or closures. Sanitized published summaries contain aggregate counts, not patient identifiers.

Tests pin each funnel category, default-off parity, missing-field handling and the rule that truth diagnostics cannot mutate pipeline state.

### 2. Measure before choosing the retrieval lever

Run the diagnostic baseline on newly generated development cohorts. Use new seeds and development people disjoint from validation; do not use `eval` people, frozen seeds, B8 seeds or calibration people. Vary missing/shifted DOB, EID capture and typos, name noise and report phrasing.

Select the first retrieval experiment from measured dominant barriers:

- If candidate-not-generated dominates in the exact-name index, test larger K/efSearch as one isolated experiment.
- If candidates are absent because of missing or mistyped identifiers, add supplemental tolerant candidate keys.
- If retrieved candidates dominate below-review, change review routing rather than expanding retrieval further.
- If linked-but-unattached dominates, fix report attachment rather than linker thresholds.

This gate prevents a broad linker rewrite when the evidence points to one stage.

### 3. Supplemental tolerant candidate retrieval

Where diagnostics show retrieval failure, supplement rather than replace exact retrieval:

- transliterated/name-skeleton keys already produced by the existing normalizers;
- partial or locally corrupted Emirates ID candidates, subject to the existing conflict rules later in the funnel;
- DOB component keys that tolerate a missing date or a shifted year without treating the date as corroboration;
- existing HNSW candidates with explicitly recorded K/efSearch settings.

Deduplicate pairs across retrieval paths and retain a reason mask showing which paths produced each pair. Keep deterministic ordering and bounded work. Any work cap must be measured and overflow reported; a silent cap could recreate L5 misses.

Supplemental candidates never merge directly. They pass through the same scorer, constraints and review/merge decisions as existing candidates.

### 4. Borderline-candidate review experiment

Expose the review floor as an opt-in development setting whose default preserves the current 0.2 behavior. Test routing additional already-retrieved borderline pairs to human review without changing the 0.7 raw merge threshold or 0.99 closure tier.

Reuse answers at registration-pair level as the current pipeline does. Record additional review items, reviewer hours, unanswered items and review-load distribution by programme and origin. Do not describe lower reviewer error as expected clinician performance; clean-review and stress-review results remain separate.

If a candidate contradicts a hard EID/cannot-link constraint, it remains blocked rather than being routed around the safeguard.

### 5. Report attachment

If diagnostics identify linked-but-unattached cases, trace the closure worker's loop-selection and cluster-eligibility checks. Add focused tests for a correctly linked closing report, repeat procedures, multiple referrals and cross-programme reports. A fix must attach the right patient's evidence without allowing cross-programme or cross-person closure.

## Error handling

Missing DOB, EID, name or sex must produce a defined candidate/constraint outcome, not a panic or an accidental exact match. Duplicate candidate keys must not duplicate merges or review items. Diagnostic write failures must not alter clinical decisions; evaluation should fail loudly rather than publish incomplete funnel evidence.

## Test and evaluation gates

### Unit and regression tests

- Default-off diagnostics and default retrieval/review settings preserve existing behavior.
- Each tolerant retrieval path returns intended candidates and deduplicates overlapping paths.
- Missing and placeholder identifiers do not corroborate identity.
- Exact-EID conflicts, durable cannot-link records, sex contradictions and required confirming reviews remain enforced.
- Additional candidates cannot bypass the merge threshold, second-identifier rule or closure gate.
- Pair-level review reuse remains deterministic; duplicate review items are not created.
- Report attachment tests cover right-patient, wrong-patient and cross-programme cases.

Run the full Go suite and `go vet` after each implementation slice.

### Development evaluation

Use paired baseline/change runs on new development seeds and disjoint development people. Report:

- candidate-funnel counts by barrier;
- right-patient CRC recall and linkage pair recall;
- false closures and wrong-patient releases;
- false-merge pairs, impure clusters and cross-person edges;
- review items, reviewer hours and throughput.

Run both zero-reviewer-error sensitivity and the ordinary stress assumptions where relevant. Do not select settings because they hide a safety failure.

### Untouched validation

Before the first validation run, freeze the code, settings, seeds, person split, report text, noise model and analysis plan in a manifest. Validation uses people and text disjoint from development and seeds not inspected during tuning. Existing frozen/B8 seeds remain regression evidence only, not tuning data.

Success requires improved right-patient recall without new false closures, wrong-patient releases or unexplained linkage contamination. Reviewer cost and residual misses must be disclosed; no production or clinical-effectiveness claim follows from synthetic results.

## Expected code boundaries

- `pipeline/linker.go`: retrieval diagnostics, supplemental candidate generation and constraint-safe review routing;
- `pipeline/ann.go` / `pipeline/model.go`: only if required by the measured retrieval experiment;
- `pipeline/prep_worker.go` and closure attachment code: diagnostic or attachment fixes only if that barrier is observed;
- `pipeline/main.go`: opt-in configuration with defaults preserving current behavior;
- focused `pipeline/*_test.go` regressions;
- external development manifests/raw results, with sanitized aggregate evidence added only after verification.

No frontend, booking, image-reader or unrelated programme redesign is included.
