# CRC r1agent9: two-hop demographic evidence review

Approved for implementation under the user's standing instruction to diagnose and improve the model after a failed experiment. Synthetic data only. No live-NATS deployment, clinician-labelled evidence or clinical-effectiveness claim is included.

## Why r1agent8 failed

The r1agent8 selection probe (881/883/887) found:

- candidate 653 closures versus guarded-human 653 and unguarded 654;
- false merges/cross-person edges 1/1 versus guarded 1/1 and unguarded 7/7;
- human review 21.727 h versus guarded 24.795 h;
- 207/207 eligible items covered, 204 accepted recommendations with 204 valid citations;
- one missed closure on seed 881 caused by a clinic → laboratory → endoscopy path whose two demographic-missing links were both demoted.

The current near-miss logic exposed only a direct hospital neighbour. Because the intermediate laboratory was not in the referral cluster, neither weak link reached review. This is a routing defect, not a prompt problem. No recall-gate relaxation is permitted.

Separate infrastructure fixes were required and verified: source-log preservation (`0a0f8c9`), provider environment restoration and the pre-candidate zero-floor gate amendment (`2789662`), OUT-only divergence analysis (`e58f551`), and report7 agent-policy restoration (`6d7618b`). The two-hop routing fix is `e0c28e3`.

## Change

With `DemoGuard` enabled, near-path discovery considers at most two near edges from a referral cluster to a hospital registration. Path strength is the weakest pair score. That pair is surfaced for review first; after a human-signed merge, the next fixed-point replay exposes the remaining link. Direct one-hop behaviour and all unguarded behaviour remain unchanged.

Hard demographic conflicts, valid-ID conflicts, cannot-link constraints and cluster-conflict checks still exclude paths. The agent never sees or overrides those conflicts. There is no unbounded graph search and no automatic merge from a near path.

The failing seed-881 case is diagnostic only: it now closes, with zero audit and report7 mismatches. It is not a selection result and will not be rerun for selection.

## Unchanged agent boundary

Use the r1agent8 sanitized sidecar, cached `gpt-5.6-luna` responses, prompt `agent-v4-json` and schema `agent-evidence-tools-v1`. The agent returns cited `same`, `different` or `uncertain` recommendations through allow-listed tools. It never signs a review, writes `fhir.review` or overrides a hard conflict.

- Agent `same` plus human `same`: the agent replaces the second human read.
- Agent `same` plus human `different`: senior human r3 decides.
- Agent `different` or `uncertain`: ordinary human double review continues.
- Uncovered or invalid output falls back to ordinary human review.

Human time excludes agent time. Replay consumes only cached recommendations and must be deterministic.

## Arms and artifacts

1. `r1photo8ref`: pairfeat8 plus `-r1review`, no demographic guard.
2. `r1photo8`: pairfeat8 plus `-r1review -demoguard`, ordinary human review.
3. `r1agent9`: the same guarded stack plus two-hop near-path review and the evidence agent.

All arms use noise v3, the cached image reader and the unchanged pairfeat8 chain. The chain is reused by hash; there is no model refit or threshold change. Each arm runs the new routing code where applicable, so the guarded-human comparator isolates the routing fix from the agent's incremental effect.

## Fresh seeds

- Selection: **941 943 947**.
- Validation: **953 967 977 979 983**.

These seeds were absent from candidate and fit configurations before registration. r1agent8's 881/883/887 are now used development seeds and are not reused. Its 907/911/919/929/937 validation seeds remain protected and unused. No prompt, tool, threshold or gate tuning is allowed on r1agent9 selection or validation seeds.

## Gates

The r1agent8 gates are carried forward with its disclosed zero-floor amendment:

| Metric | Requirement |
|---|---|
| CRC recall | Candidate ≥ each comparator in selection, stress and validation; no relaxation |
| false merges and cross-person edges | ≤ guarded in every mode; strictly below unguarded when it is > 0, otherwise candidate = 0 |
| uncapped wrong-pair link reviews | Same comparison rule as contamination |
| human reviewer hours | ≤ guarded and ≤ 1.10 × unguarded |
| safety | Zero false closures, wrong releases, replay mismatches, analysis errors and trace errors in every lane |
| deterministic replay | Required for every arm |
| eligible-item coverage | ≥ 80% in each mode |
| citation validity | 100% of accepted non-abstaining recommendations |

The zero-floor amendment was registered before any r1agent8 candidate verdict: strict improvement below zero is impossible, so zero must be matched. It is not a response to the recall failure and cannot be used to weaken recall.

Stop at the first failed gate. Report impure clusters, wrong final sign-offs, agent confusion, abstentions, uncovered/invalid items, calls, tokens, latency and human seconds saved. Keep agent latency separate from human hours.

## Verification and disclosures

- Unit test two-hop path selection and replay determinism; run the full Go and harness suites.
- Preserve the earlier 20-item smoke result accurately: 19 actionable recommendations and one schema-valid abstention, with no provider error.
- Run selection, stress and frozen validation in that order, followed by read-only diagnosis and independent results review.
- The simulated human is truth plus injected error, not an independent clinician. The two-hop diagnostic is not fresh evidence.
- Results use synthetic identity truth and existing cached clinical verdicts; no clinical performance or deployment claim.
- Live NATS migration remains a separate shadow/assist experiment with deterministic fallback and a kill switch.
- No `/Users` paths, identifiers, secrets, binaries or models in Git. Never commit the forbidden older plan file. No push or merge without the user's explicit decision. Thin non-record runs only after documenting them.
