# r1agent12: evidence-agent experiment on the pairfeat9 chain

Approved 5 Nov 2026. Synthetic data only. Registers the harness experiment for the pairfeat9 model chain (see `2026-11-05-crc-pairfeat9-middle-initial-design.md`). No gate, threshold, demographic-policy or live-deployment change.

## Evidence

r1agent11 failed selection with recall tied at 610/648 but one wrong-pair review versus zero in the unguarded arm (Kate O'Brien versus Kate L. O'Brien, same sex, one missing DOB, no downstream EID, p=0.8482). The review already existed, so the evidence agent could not remove it. Read-only analysis of the pairfeat8 training CSVs (2,540,886 rows, 61,154 positives) showed the exact middle-initial shape is ambiguous (15 true versus 11 different-person tight shapes) and is not covered by `givenOnly` or the missing-date hard negatives. pairfeat9 adds a narrow training-only hard-negative generator at rate 0.002 for that shape. The failing pair is diagnostic only and is not used for training or tuning.

## What changes

- The deferred first investigation before a human rejection can become cannot-link, and the resolved-edge fix, carry over from r1agent11 unchanged.
- The chain changes from pairfeat8 to **pairfeat9**: `CHAIN_FILES["pairfeat9"]` = encoder, lgb_stream, iso, iso_v7 and name_idf files (`*_pairfeat9.json`); `-modelset pairfeat9`; manifest key `pairfeat9_chain_sha256`.
- Comparator arms move to the pairfeat9 chain: reference-only `r1photo9ref` (`-r1review`, unguarded) and `r1photo9` (`-r1review -demoguard`, guarded), both chain pairfeat9, noise v3, image on, never experiment candidates (r1photo8/r1photo8ref require the pairfeat8 fit, so `init-experiment` could not pin one shared pairfeat9 fit with them). r1agent12 has `reference=r1photo9ref`, `base_arm=r1photo9`; one shared pairfeat9 fit serves all three arms and `init-experiment EXPERIMENT_DIR r1agent12` pins it. The runner takes the arm mapping from its REF/BASEARM table rows.
- Everything else is the r1agent11 configuration: `reference_gate="r1agent8"`, `agent_replay=True`, noise v3, flags `-r1review -demoguard`, edge classes pairfeat7, image cached.

## Unchanged agent semantics

Evidence-agent boundary, replay determinism, coverage and citation checks are as in r1agent8 to r1agent11. Earlier candidates (including their seeds, chains and gates) are untouched.

## Fresh seeds

- Selection (dev): **1061 1063 1069**.
- Validation: **1073 1079 1081 1087 1091**.

These were fresh at registration. r1agent11's 1011/1017/1023 and 1035/1041/1047/1051/1057, and all earlier seeds, stay used.

## Gates (all existing, none relaxed)

- Candidate recall >= each comparator in selection, stress and validation.
- False merges, cross-person edges and uncapped wrong-pair reviews <= guarded; strictly below unguarded when that is > 0, otherwise 0.
- Reviewer hours <= guarded and <= 1.10 x unguarded.
- Zero false closures, wrong releases, replay mismatches, analysis errors and trace errors in every lane.
- Deterministic replay in every arm; eligible agent coverage >= 80%; accepted citation validity 100%.

Stop at the first failed gate. Order: selection, stress, frozen validation, only after a clean fit. If wrong-pair reviews do not improve or true-link recall falls, pairfeat9 is not selected and the rate is not broadened post hoc. Report the middle-initial slice separately. Results are synthetic, use cached clinical verdicts, and simulated reviewers are truth plus injected error, not clinicians.
