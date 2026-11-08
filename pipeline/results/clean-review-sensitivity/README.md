# Clean-review sensitivity evaluation

**Post-hoc reviewer-error sensitivity, not a replacement for the frozen stress-test evaluation.** These are simulated results, not measured clinical performance.

Five held-out v2 seeds (`11 23 37 41 59`) were rerun at `6403180` with both injected reviewer-error rates set to zero:

- ordinary simulated reviewer error: `-rerr 0` instead of `0.04`;
- day-15 ungraded-prep simulated reviewer error: `-errungraded 0` instead of `0.25`.

All other evaluation settings were unchanged: held-out people, cached readers, FHIR, `v7strict`, chaos, EID guard off, LDCT 300 and IFHAS 800 loops per run, and adversarial generation off. No model, threshold, calibrator, reader or generator changed. No paid calls were made. The decision code is the same as the frozen `075abc5` evaluation; `6403180` added only evaluation evidence and documentation.

## CRC result

| Seed | Stress-review recall | Clean-review recall | Difference |
|---|---:|---:|---:|
| 11 | 0.791 | 0.863 | +7.2 pp |
| 23 | 0.790 | 0.866 | +7.6 pp |
| 37 | 0.785 | 0.872 | +8.7 pp |
| 41 | 0.767 | 0.825 | +5.8 pp |
| 59 | 0.792 | 0.869 | +7.7 pp |
| **Mean over seeds** | **0.785** | **0.859** | **+7.4 pp** |

Clean-review mean CRC precision was **1.000** and mean recall was **0.859**, with a 95% t interval over the five seed means of **0.835–0.883**. There were **0/3,322 CRC false closures** (Clopper–Pearson 95% upper bound 0.111%).

The matched frozen stress-review result remains **0.785 recall (0.772–0.798), 0/3,035 CRC false closures**. Its assumed 4% ordinary and 25% day-15 ungraded-prep reviewer-error rates are stress inputs, not measured clinician error rates. The clean-review lanes instead make simulated reviewers agree with simulated truth; they do not establish that real reviewers are error-free.

## Other programmes and safety

| Clean-review metric | Mean over five seeds |
|---|---:|
| Cervical recall | 0.9294 |
| Incidental lung-nodule recall | 0.9598 |
| Diabetic-retinopathy recall / referral recall | 0.9956 / 0.9090 |
| Breast recall / recall-loop recall | 0.9942 / 0.9314 |
| LDCT recall / action-loop recall | 0.9988 / 0.9870 |
| IFHAS recall / follow-up-loop recall | 0.9736 / 0.9200 |
| Reviewer time, hours per 1,000 loops | 11.3782 |

Pooled safety results:

- false closures: CRC 0/3,322; cervical 0/1,337; incidental lung 0/1,922; lung surveillance CT 0/32; DR 0/5,636; DR referral 0/237; breast 0/338; breast recall 0/23; LDCT 0/1,414; LDCT action 0/122; IFHAS 0/3,409; IFHAS follow-up 0/1,063;
- wrong-patient releases 0/16,534 and releases without granted consent 0/11,610;
- duplicate/lost outputs 0/216,782; ingest loss/duplication 0/274,706; final-state mismatches and forged effects zero;
- tamper detection 85/85, false alarms 0/5.

One lung-surveillance obligation was missed and 89/91 surveillance overdue flags were correct; surveillance false closures remained zero. These limitations are not hidden by the zero-reviewer-error setting.

## Interpretation and files

This sensitivity isolates the effect of the two reviewer-error assumptions in this synthetic harness. It does not remove identity noise, missing evidence, ungraded prep, reader/rule limitations, timing limits or chaos. The remaining clean-review recall loss is therefore not evidence that all non-reviewer barriers have been solved.

- `manifest.json`: seeds, settings, input hashes and both zero reviewer-error rates.
- `summary.json` and `SUMMARY.md`: aggregate metrics and safety counts.
- `v2-s*/`: each lane's `report7.json`, tamper, summary, origin-null and subgroup outputs.

For reproduction, use an external copy of `scripts/final_eval.sh` with the two flag changes above and `WASL_NOISES=v2 WASL_LDCT=300 WASL_IFHAS=800`; keep the original frozen script unchanged. Full per-programme intervals, subgroup results and limitations are in `SUMMARY.md` and `summary.json`.
