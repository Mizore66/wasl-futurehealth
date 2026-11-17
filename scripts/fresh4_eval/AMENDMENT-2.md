# Fresh4 protocol amendment 2: ownership-aware I1 closure gate

| | |
|---|---|
| Amendment id | `fresh4-amendment-2` (protocol version 2 of gate I1) |
| Amends | `PROTOCOL.md` section 6, integrated gate **I1** |
| Approved by | Anas, fresh4 protocol owner (repository owner `Mizore66`) |
| Approved on | 2026-11-17 |
| Issues | #193 (closure safety scoring ignored wrong-patient evidence), #218 (migrate the I1 gate) |
| Implementation | `scripts/fresh4_eval/amendment2.py`, tests in `scripts/fresh4_eval/test_amendment2.py` |

`PROTOCOL.md`, `fresh4.py` and the frozen results are **not edited** by this amendment. The original I1 gate keeps
its pre-registered definition and its recorded result. The amended gate is evaluated next to it and is labelled with
this amendment's id.

## Rationale

Issue #193 found a defect in the scorer that feeds I1, not in the pipeline under test. `report7` counted a predicted
closure as a false closure only when the loop's truth state was not `closed`. A loop that really was closed, but was
closed on **another patient's report**, therefore counted as a correct closure. I1 reads those outcome-only counts
(`crc_false_closures`, `cervical_false_closures`), so a wrong-patient closure could pass "I1: zero listed false
closures". That contradicts what I1 exists to protect: no loop is closed without evidence that belongs to the patient.

PR #219 added the ownership-aware closure loss `outcome_or_evidence_ownership_v2`. A predicted closure is **unsafe**
when its outcome is false, or when its closing report belongs to another person (foreign evidence), or cannot be
mapped to a person in the truth (unverified evidence). `final_aggregate.py` publishes the pooled counts as
`crc_unsafe_closures` and `cervical_unsafe_closures` with Clopper-Pearson bounds, only for v2 artifacts.

## Amended gate

**I1 (amendment 2), per noise model:** zero listed false closures for every metric of the original I1 list, **and**
zero `crc_unsafe_closures` and zero `cervical_unsafe_closures`. Numerator/denominator are pooled counts/predicted
closures per metric, as in the original. Gate id: `I1a2_<noise>_zero_listed_false_or_unsafe_closures`.

- Missing evidence still fails, as in the original protocol ("partial evidence cannot pass").
- An aggregate scored with the outcome-only loss (`closure_loss` absent or `outcome_only_v1`) never computed unsafe
  closures. If those are the only missing metrics, the amended gate is `not_evaluable`, with the reason recorded. It is
  never `pass`. A measured nonzero outcome-only false closure still makes it `fail`.
- Thresholds stay zero. No model, rule, threshold, reviewer setting, corpus item, label or seed changes.
- Only CRC and cervical carry ownership-checked counts today. The other programmes in the I1 list are still
  outcome-only, exactly as in the original gate. Extending ownership checks to them would need a further amendment.

## Why this is not an outcome-dependent amendment

Section 1 of the protocol prohibits outcome-dependent amendments. This amendment is not one:

- It was motivated by an independent audit finding (#193) about the scorer, with a synthetic reproduction. It does not
  respond to any fresh4 result.
- The frozen fresh4 integrated run (`data/fresh4/results/integrated/`) predates the v2 loss. Its aggregate has no
  unsafe counts, so no fresh4 outcome on this metric existed or could have been seen.
- The change only adds a requirement. It cannot turn a recorded failure into a pass.

## Effect on the frozen fresh4 run

Evaluated without rerunning anything:

```bash
python3 scripts/fresh4_eval/amendment2.py data/fresh4/results/integrated/summary.json \
  --out data/fresh4/amendments/amendment-2-gates-frozen-integrated-run.json
```

Result (`data/fresh4/amendments/amendment-2-gates-frozen-integrated-run.json`): every original gate keeps its recorded
status (I0 to I4 pass for v1 and v2). The amended I1 is `not_evaluable` for both noise models, because the run's
`report7.json` files are outcome-only (v1). The recorded original I1 pass (0/18,984 for v1, 0/18,354 for v2) is
unchanged and remains a statement about outcome-only false closures. It is **not** evidence that no closure used
another patient's report.

Evaluating the amended gate needs a new integrated run with the current `report7` (v2 loss). That has not been run.
Section 6 keeps the frozen result as recorded, so a v2 run does not replace it. It should use newly registered seeds,
recorded before it starts. Everything here is simulated; no result is clinical evidence.
