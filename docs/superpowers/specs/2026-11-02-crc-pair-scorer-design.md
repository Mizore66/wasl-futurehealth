# CRC feature-first pair-scorer redesign

**Status:** proposed, not built. No runtime change, model fit, clinical lane, or validation run is authorized by this document alone.

## Goal

Improve CRC closure recall by helping the existing identity scorer recognize retrieved same-person pairs that it currently scores far below review, without changing candidate retrieval or weakening any merge, closure, identity, evidence, consent, or outreach safeguard.

The experiment must isolate the contribution of new pair features and a refit LightGBM. The existing encoder stays fixed. Encoder retraining, candidate-generation changes, attachment fixes, and scorer hyperparameter searches are separate work.

## Evidence and research

### Current implementation

The canonical runtime is `pipeline/`:

- `pipeline/text.go` constructs 23 pair features. Existing indices 6, 13 and 19 are consumed positionally by corroboration logic.
- `pipeline/linker.go` retrieves candidates, computes features, and obtains raw pair probabilities from the LightGBM scorer. Raw scores influence merge/review routing and cluster guards.
- `pipeline/model.go` loads the deterministic Go LightGBM dump reader.
- `pipeline/encdev.go` groups encoder, scorer, and calibrator files under `-modelset`; policy identity records the selected modelset.
- `encoder/fit_scorer.py` and `encoder/run_all.sh` provide the existing scorer/calibrator fitting pattern.
- Calibrated probabilities and their conservative strata remain downstream of raw-score routing.

### Development-only miss evidence

Read-only analysis used only existing dev/v2 seeds 101, 103 and 107 from the completed review-floor experiment. It did not inspect eval, validation, frozen, B8, cal, or train pair traces and did not run a clinical lane.

At review floor 0.01, 48 truth-closed CRC misses remained in `candidate_below_review`. All 48 were retrieved same-person pairs, not absent candidates:

- 43/48 scored below 0.001; further floor lowering is therefore not a credible remedy.
- 9 had both EIDs present with a one- or two-digit difference.
- 27 had a decade-digit-only DOB difference; 23 remained after assigning the EID group first.
- 7 had a day/month-only DOB difference or missing DOB.
- 9 had multi-field DOB changes, no EID rescue, and mostly cross-script names; these are not assumed scorer-fixable.

A same-development, rule-shaped hypothesis combining EID near-match and decade-DOB/sex/current-score signals covered 21/48 with no different-person pair in that particular routed slice. This is a feature hypothesis, not a validated rule or expected clinical gain. It was derived and measured on the same development pairs and carries overfitting risk. Those seeds are diagnostic only for this experiment and are excluded from model selection.

The scorer cannot repair the separate residual categories: 3 candidates not generated, 2 demographic-constraint rejections, and 32 linked-report-not-attached misses in these development lanes. No claim is made that all 48 are recoverable.

Aggregate research artifacts are under `~/wasl-ml-data/agents2/hardening/scorer-research/`. Registration-level pickles contain synthetic identifiers and must never enter Git.

### Public methods

The design uses established entity-resolution principles rather than a new large model:

- Splink's Fellegi–Sunter documentation describes field-level comparison evidence and explicitly warns that estimated/placeholder dates distort agreement evidence: https://moj-analytical-services.github.io/splink/topic_guides/theory/fellegi_sunter.html and https://moj-analytical-services.github.io/splink/topic_guides/comparisons/comparisons_and_comparison_levels.html
- Ditto shows the value of difficult-example augmentation for learned entity matching, but its transformer approach is intentionally out of scope here: Li et al., https://arxiv.org/abs/2004.00584
- WDC Products emphasizes unseen entities and corner-case evaluation; entity/person-disjoint evaluation is therefore mandatory: Peeters, Der and Bizer, https://arxiv.org/abs/2301.09521
- Calibration must be measured rather than assumed for a changed model: Guo et al., https://arxiv.org/abs/1706.04599

These sources motivate comparison features, hard-negative measurement, person-disjoint evaluation, and recalibration. They do not establish performance on Wasl's synthetic cohorts.

## Architecture

### Opt-in model chain

Add an experimental modelset named `pairfeat1` with this chain:

- `encoder_pairfeat1.json`: an exact local copy of the current default encoder weights; no encoder retraining.
- `lgb_stream_pairfeat1.json`: the refit 30-feature LightGBM.
- `iso_pairfeat1.json`: refit isotonic calibrator.
- `iso_v7_pairfeat1.json`: refit stratified conservative calibrator.

Generated weights and calibrators remain local evaluation artifacts and are not committed. `PolicyName` must stamp `+models=pairfeat1`; live, audit, replay, tamper, report, and analysis paths must load the same chain through the existing modelset interface.

The default modelset and `-linkfloor 0.2` remain unchanged. The experiment compares the current scorer and `pairfeat1` with both arms explicitly using `-linkfloor 0.01`, so any measured gain is additional to the completed review-floor result.

`pairfeat1` initially supports `v7strict` only. Selecting `v7ltt` with `pairfeat1` must fail closed with a configuration error until a separate Learn–then–Test selection is performed. No v7ltt result may be claimed from the stale selection.

### Feature contract

Keep existing indices 0–22 byte-compatible and append seven features at indices 23–29. Do not reorder or reinterpret existing features.

| Index | Feature | Definition |
|---:|---|---|
| 23 | `eid_hamming` | If both non-placeholder EIDs are present with equal length, the number of mismatching character positions; otherwise NaN. Exact-EID and conflict/cannot-link handling still occurs in the existing rule paths. |
| 24 | `dob_decade_digit_only` | For two valid, informative YYYYMMDD values: 1 when century and units year digits, month and day match and only the tens-of-year digit differs; otherwise 0. NaN when either DOB is missing, invalid, or a known placeholder. |
| 25 | `dob_digit_difference_count` | For two valid, informative fixed-width YYYYMMDD values: count of differing digit positions from 0 through 8; otherwise NaN. |
| 26 | `dob_day_only` | For two valid, informative dates: 1 when year and month match and day differs; otherwise 0. NaN for missing, invalid, or placeholder dates. |
| 27 | `dob_month_only` | For two valid, informative dates: 1 when year and day match and month differs; otherwise 0. NaN for missing, invalid, or placeholder dates. |
| 28 | `name_token_set_equal` | After existing script-appropriate normalization, 1 when both nonempty name token sets are equal independent of order; otherwise 0. NaN when either normalized name has no tokens. |
| 29 | `name_skeleton_equal` | 1 when the existing nonempty normalized name skeletons are equal; otherwise 0. NaN when either skeleton is empty. |

Missing or placeholder DOBs must not become agreement signals. Existing exact-DOB, surname-skeleton, and sex features keep their positions and semantics. Feature generation must be deterministic and allocation-safe enough for the existing linker workload.

The hard safeguards remain outside the model: Luhn-valid equal-EID behavior, conflicting-EID and typo-contradiction cannot-link rules, merge constraints, cluster guards, human-review semantics, second-identifier checks, consent, evidence gates, and attachment behavior are unchanged. Merge remains 0.7, closure tier 0.99, uncorroborated cap 0.989, and the equal-EID contradiction clamp 0.2. Near facts cannot directly close loops or trigger outreach.

## Data and training protocol

Freeze a development/training manifest before fitting. It must record Git HEAD, feature-extractor and fitting-code hashes, all seeds/splits, cohort/generator settings, encoder identity, and output artifact hashes after fitting.

1. Generate scorer rows from five fresh train-split cohorts with seeds **1001, 1003, 1007, 1009, 1013**.
2. Use the existing candidate retrieval and fixed encoder. Labels come only from synthetic train truth. Never use cal/dev/eval people for fitting.
3. Fit one LightGBM with the existing recorded configuration: 400 rounds, 63 leaves, learning rate 0.05, seed 1, deterministic mode. No hyperparameter sweep.
4. Include all retrieved pairs under the existing fitting weighting. Do not manufacture a rule from the 21-pair development hypothesis. Measure hard-negative slices for near-EIDs, decade DOB changes, same-surname pairs, placeholder DOBs, and cross-script names.
5. Refit both calibrators using only cal-split seed **2003** and the existing calibrator construction. Do not move raw merge/review thresholds to compensate for a scale change.
6. Report pair-level discrimination and routing exposure overall and by hard-negative slice, including different-person pairs that would cross merge/review thresholds. Pair AUC is diagnostic, not the clinical selection gate.

No paid calls, live credentials, external patient data, buyer/clinician/organiser contact, or publication are permitted.

## Clinical experiment protocol

All arms use paired CRC-only v2 cohorts, cached readers, image off, FHIR, chaos 0, subkill 1, and no NATS kill. Preserve every output root and failure. Arms run sequentially because the built executable is shared.

The baseline is the current modelset with `-linkfloor 0.01`; the candidate is `pairfeat1` with `-linkfloor 0.01`. Truth-closed denominators must match. Exact CRC TP is `pred_closed - false_closures`; link attach rate is not closure recall.

### Development selection

Use fresh dev-split seeds **307, 311, 313**. These replace 101/103/107 for selection because the earlier seeds informed the feature hypotheses.

Select `pairfeat1` only when all requirements pass:

- pooled CRC closure-recall gain is at least **+2.0 percentage points** over baseline;
- false closures, wrong-patient releases, replay mismatches, analysis errors, and trace-write errors are zero in both arms;
- false-merge pairs, impure clusters, and summed cross-person edges do not increase;
- distinct link-review pairs increase by no more than **10%**;
- model identity, artifact hashes, split, floor, and scope match the frozen manifest.

Report all outcomes, reviewer items/hours/unanswered, residual funnel categories, and small-n intervals. A failure stops the experiment with no retuning and no automatic encoder fallback.

### Synthetic reviewer stress

Only baseline and a selected candidate run on the same three dev seeds with 4% ordinary reviewer error and 25% day-15 ungraded-prep reviewer error. These are synthetic stress assumptions, not measured clinician rates. Require no recall regression and every safety, replay, analysis, contamination, and workload gate above. Keep stress and clean evidence separate.

### Untouched validation

Only after development and stress pass, freeze a validation manifest containing current Git HEAD, selected model identity and artifact hashes, runner/analyzer hashes, exact settings, and fresh eval-split seeds **401, 409, 419, 421, 431**. Do not inspect these seeds before freezing.

Validation requires:

- pooled CRC closure-recall gain of at least **+1.0 percentage point**;
- every safety/replay/analysis/contamination gate above;
- no more than 10% growth in distinct link-review pairs.

If validation fails, preserve artifacts, retain the current model, and stop without another seed set or threshold adjustment. If it passes, `pairfeat1` remains an opt-in candidate; changing the default requires separate user authorization.

## Verification and disclosures

The implementation plan must include tests for:

- exact appended-feature values, missing/placeholder behavior, and stable legacy indices;
- default-model output parity and `pairfeat1` policy identity;
- invalid `pairfeat1`/`v7ltt` configuration;
- Go/model feature-dump parity and deterministic repeated scoring;
- unchanged EID/cannot-link, merge, closure, second-identifier, consent, and near-fact safeguards;
- live/audit/replay consistency and no trace-write failures;
- protected split/seed/floor/model/artifact identity in the experiment harness.

Final documentation must disclose all post-hoc changes and failed runs, both arms, clean versus stress evidence, per-seed and pooled results, small-n intervals, reviewer burden, residual misses, and nonzero contamination. The model applies globally but CRC-focused cohorts establish no non-CRC safety or recall claim. Simulated results do not prove clinical effectiveness.

## Implementation phases

1. Add and test the 30-feature contract behind the existing model interface.
2. Add the opt-in modelset chain, configuration guard, policy identity, and replay tests.
3. Extend deterministic feature dumping/fitting and produce hashed train/cal artifacts under the frozen protocol.
4. Add the protected paired development/stress/validation harness and offline analyzer.
5. Run the gated experiment once and record honest aggregate results.

Each phase has its own tests and review. No clinical lane may run until runtime and harness reviews pass and the relevant manifest is frozen.
