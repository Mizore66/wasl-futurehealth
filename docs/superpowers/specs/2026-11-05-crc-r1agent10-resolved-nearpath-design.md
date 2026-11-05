# CRC r1agent10: resolved-edge two-hop evidence review

Synthetic data only. No live-NATS deployment, clinician-labelled evidence or clinical-effectiveness claim is included.

## Prior result and fix

r1agent9 selection (941/943/947) failed recall by one closure: candidate and guarded human 629/659, unguarded 630/659. Agent contamination, coverage, citations, human time, replay and safety gates passed. The recall gate was not relaxed.

Read-only diagnosis found that seed-943's first review correctly merged clinic → hospital. The near graph retained the historical near edge, so path selection reused the resolved pair and suppressed the remaining referral → clinic review. Commit `14f07d6` skips resolved near edges and collapses an already-linked final edge to the unresolved first pair. The diagnostic loop now closes; the full Go suite passes. This is not fresh selection evidence.

## Experiment

Use the unchanged pairfeat8 chain, noise v3, cached image reader, sanitized evidence sidecar, `gpt-5.6-luna`, prompt `agent-v4-json` and schema `agent-evidence-tools-v1`.

Arms:

1. `r1photo8ref`: pairfeat8 plus `-r1review`, no demographic guard.
2. `r1photo8`: pairfeat8 plus `-r1review -demoguard`, ordinary human review.
3. `r1agent10`: the guarded stack, bounded two-hop near-path review with resolved-edge handling, and the evidence agent.

The agent remains one cited read only. Agent `same` plus human `same` replaces the second human read; agent/human disagreement uses senior human r3; `different`, `uncertain`, invalid and uncovered output retain ordinary human double review. The agent never signs, writes `fhir.review` or overrides a hard conflict. Human time excludes agent time.

## Fresh seeds

- Selection: **987 991 993**.
- Validation: **997 1027 1037 1043 1049**.

These were fresh at registration. r1agent9's seeds are not reused; its validation seeds remain protected. No prompt, tool, model, threshold or gate tuning is allowed on r1agent10 selection or validation seeds.

## Gates

Carry forward the r1agent8/r1agent9 gates and the disclosed zero-floor amendment:

- Candidate recall ≥ each comparator in selection, stress and validation; no relaxation.
- False merges, cross-person edges and uncapped wrong-pair reviews ≤ guarded; strictly below unguarded when it is > 0, otherwise candidate = 0.
- Human reviewer hours ≤ guarded and ≤ 1.10 × unguarded.
- Zero false closures, wrong releases, replay mismatches, analysis errors and trace errors in every lane.
- Deterministic replay in every arm; eligible coverage ≥ 80%; accepted citation validity 100%.

Stop at the first failed gate. Report impure clusters, wrong final sign-offs, agent confusion, abstentions, uncovered/invalid items, calls, tokens, latency and human seconds saved. Keep agent latency separate from human hours.

## Verification and limits

- Run the full Go and harness suites and an independent results review.
- Preserve the smoke result accurately: 19 actionable recommendations and one schema-valid abstention, no provider error.
- Selection, stress and frozen validation run only in that order; this launch authorizes development only.
- The old failing-seed replay is diagnostic, not selection evidence. Simulated reviewers are truth plus injected error, not clinicians.
- Results use synthetic identity truth and cached clinical verdicts. Live NATS migration requires a separate shadow/assist evaluation.
- No local paths, identifiers, secrets, binaries or models in Git. Never commit the forbidden older plan file. No push or merge without the user's explicit decision. Thin non-record runs after documenting them.
