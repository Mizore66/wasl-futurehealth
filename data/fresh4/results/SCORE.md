# Fresh4 frozen reader scores

**Historical/staleness disclosure: reader implementation changed after scoring; results do not apply to current integration.**

Fresh4's own scoring revision `9d653b4b1f6a2871651dbbcc64ef62e8b8c92fd0` and publication revision `bc25c6968ebf914d5ef53a2c1b069a15d90c4184` contain identical reader blobs for the four files below; current integration blobs differ. The historical scores are preserved without rerunning readers or changing scores.

- `pipeline/ifhasread.go`
- `pipeline/ldctread.go`
- `pipeline/lungread.go`
- `pipeline/reader.go`

**Fresh4 keys are model-labelled and model-adjudicated from a frozen guide, not clinician-verified. Agreed and adjudicated strata are reported separately; neither is clinical truth or evidence of clinical effectiveness.**

Status: built; simulated synthetic text; cached local Gemma reader; no clinical-effectiveness claim.

Key origins: 100 direct agreements + 0 adjudicated = 100 scoreable; 0 unresolved.

| stratum | scoreable n | A correct | B correct | B unavailable | readers agree | agreed but wrong |
|---|---:|---:|---:|---:|---:|---:|
| agreed | 100 | 64 | 77 | 10 | 39 | 3 |
| adjudicated | 0 | 0 | 0 | 0 | 0 | 0 |
| pooled | 100 | 64 | 77 | 10 | 39 | 3 |

Pooled per-kind denominators and Wilson 95% intervals are in score.json/analysis_wilson95.

Wrong agreed answers: 0 closing-direction and 3 holding-direction.

| gate | status | numerator | denominator |
|---|---|---:|---:|
| R1_complete_raw_output | pass | None | None |
| R2_zero_closing_direction | pass | 0 | 100 |

Unresolved items are not scored for accuracy. No gate or reader was changed after seeing output.
