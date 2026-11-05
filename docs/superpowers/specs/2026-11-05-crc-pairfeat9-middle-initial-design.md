# pairfeat9: narrow middle-initial hard negatives

Approved 5 Nov 2026 at a 0.002 training-only rate. Synthetic data only. No gate, threshold, demographic-policy or live-deployment change is included.

## Evidence and limitation

r1agent11 failed selection with recall tied at 610/648 but one wrong-pair review versus zero in the unguarded arm. The pair was Kate O'Brien versus Kate L. O'Brien, same sex, one missing DOB and no downstream EID, model p=0.8482. Senior human review correctly rejected it. The evidence agent cannot reduce that review count because the review already exists.

Read-only analysis of the five pairfeat8 training CSVs (2,540,886 rows, 61,154 positives) found:

- exact token sets with missing DOB/no comparable EID: 543 true-same versus 69 different-person pairs;
- tight 2-vs-3-token, same-script target-like shapes: 15 true versus 11 different;
- near-name missing-demographic pairs with high shared rarity: many true links;
- existing `givenOnly` and missing-date hard negatives do not cover the exact middle-initial shape.

This class is ambiguous and has no zero-loss solution. pairfeat9 is a recall-gated experiment, not a deterministic initial penalty and not a claim that near-exact names are different people.

## Change

Keep pairfeat8's 34 features, leave-own-out rarity table, `givenOnly`, model hyperparameters, demographic guard and review floors unchanged. Add a separate training-only hard-negative generator at **rate 0.002**, with its own deterministic random stream.

Each generated different-person pair must have:

1. identical first and last tokens in the same script;
2. exactly one single-letter middle token present on only one side;
3. the same known sex;
4. one unchanged DOB and the other DOB empty, explicitly including placeholder DOBs;
5. an empty EID on the empty-DOB side, so there is no comparable EID;
6. one registration per person, with the registration selected so the pair is generated as a candidate.

Do not broaden the generator to arbitrary exact-name/missing-DOB negatives, relax `givenOnly`, downweight same-person positives or alter feature weights. `f30–f33` remain unchanged.

## Implementation boundary

- `pipeline/pairfeat9.go`: independent generator, registration logic and exact-pattern validation.
- `pipeline/encdev.go`: register the hard-negative kind and emit a dedicated `mi_rows` sidecar through the existing mechanism.
- `pipeline/main.go`: training-only flag and validation; no effect on evaluation cohorts.
- `pipeline/pairfeat6.go`: include pairfeat9 in `usesNameIDF`; retain 34 features.
- `encoder/fit_pairfeat1.py`: clone pairfeat8's protocol, add only the new kind, and extend argument, manifest, slice and floor reporting.
- `scripts/pairfeat_compare.py` and `scripts/pairfeat_experiment.sh`: register the pairfeat9 chain and fresh seeds when evaluation is configured.

The sidecar must prove the intended pattern, not merely count generated persons. Tests cover exact shape, determinism, no effect when the rate is zero, no use of evaluation-only generation, fit argument/manifest handling and unchanged feature count.

## Fresh seeds

- Selection: **1061 1063 1069**.
- Validation: **1073 1079 1081 1087 1091**.

These seeds were fresh at registration. r1agent11's 1011/1017/1023 and all earlier experiment seeds remain used/protected. The failing pair is diagnostic only and must not be used for training or tuning.

## Gates

The experiment keeps the r1agent8/r1agent11 comparator structure and all recall and safety gates:

- Candidate recall ≥ each comparator in selection, stress and validation; no relaxation.
- False merges, cross-person edges and uncapped wrong-pair reviews ≤ guarded; strictly below unguarded when it is > 0, otherwise candidate = 0.
- Human reviewer hours ≤ guarded and ≤ 1.10 × unguarded.
- Zero false closures, wrong releases, replay mismatches, analysis errors and trace errors in every lane.
- Deterministic replay in every arm; eligible agent coverage ≥ 80%; accepted citation validity 100%.

Stop at the first failed gate. If wrong-pair reviews do not improve or true-link recall falls, pairfeat9 is not selected and the rate is not broadened post hoc. Report the dedicated middle-initial slice separately from other hard negatives.

## Verification and disclosures

- Write tests before implementation and run the full Go, fit and harness suites.
- Preserve the existing evidence-agent boundary and all previous infrastructure fixes.
- Selection, stress and frozen validation run only in that order after a clean fit.
- Report training-side slice counts, exact-pattern validation, score changes and all failures, including true-link losses.
- Results are synthetic and use existing cached clinical verdicts; simulated reviewers are truth plus injected error, not clinicians.
- No local paths, identifiers, secrets, binaries or models in Git. Never commit the forbidden older plan file. No push or merge without the user's explicit decision. Thin non-record runs after documenting them.
