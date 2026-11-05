# CRC r1agent11: investigate before a human rejection becomes cannot-link

Synthetic data only. No live-NATS deployment, clinician-labelled evidence or clinical-effectiveness claim is included.

## Prior result and fix

r1agent10 passed selection at 658/688 in all arms, then failed stress: candidate and guarded human 595/688, unguarded 600/688. The recall gate was not relaxed and validation did not run.

Read-only tracing found six stress-only missed loops. Every one had a true-same link rejected by the simulated human's first read, with no agent read. The rejection became cannot-link during the first replay, so the pair was ineligible before the sidecar investigated it. The agent reduced review time but could not correct those errors.

Commit `00a5e3c` changes the controller: an eligible pair with no recommendation remains unsigned and is emitted to `agent_need.ndjson`. The sidecar investigates it, then the next replay performs the human+agent read. A terminal `uncertain` still falls back to ordinary human review. Human sign-off remains mandatory and hard conflicts remain excluded. Stats reconstruct already-closed items from their sign-offs to preserve human-time accounting.

On stress seed-991 diagnostics, all three previously missed loops now close, with zero audit/report7 mismatches. This is diagnostic evidence only, not selection evidence.

## Experiment

Use the unchanged pairfeat8 chain, noise v3, cached image reader, bounded two-hop near-path review with resolved-edge handling, sanitized evidence sidecar, `gpt-5.6-luna`, prompt `agent-v4-json` and schema `agent-evidence-tools-v1`.

Arms:

1. `r1photo8ref`: pairfeat8 plus `-r1review`, no demographic guard.
2. `r1photo8`: pairfeat8 plus `-r1review -demoguard`, ordinary human review.
3. `r1agent11`: the guarded stack with two-hop review and deferred first investigation.

Agent `same` plus human `same` replaces the second human read; agent/human disagreement uses senior human r3; `different`, `uncertain`, invalid and uncovered output retain ordinary human double review. The agent never signs, writes `fhir.review` or overrides a hard conflict. Human time excludes agent time.

## Fresh seeds

- Selection: **1011 1017 1023**.
- Validation: **1035 1041 1047 1051 1057**.

These were fresh at registration. Earlier selection/stress seeds are not reused. No prompt, tool, model, threshold or gate tuning is allowed on r1agent11 selection or validation seeds.

## Gates

Carry forward the existing gates and the disclosed zero-floor amendment:

- Candidate recall ≥ each comparator in selection, stress and validation; no relaxation.
- False merges, cross-person edges and uncapped wrong-pair reviews ≤ guarded; strictly below unguarded when it is > 0, otherwise candidate = 0.
- Human reviewer hours ≤ guarded and ≤ 1.10 × unguarded.
- Zero false closures, wrong releases, replay mismatches, analysis errors and trace errors in every lane.
- Deterministic replay in every arm; eligible coverage ≥ 80%; accepted citation validity 100%.

Stop at the first failed gate. Report impure clusters, wrong final sign-offs, agent confusion, abstentions, uncovered/invalid items, calls, tokens, latency and human seconds saved. Keep agent latency separate from human hours.

## Verification and limits

- Run the full Go and harness suites and an independent results review.
- Preserve the smoke result accurately: 19 actionable recommendations and one schema-valid abstention, no provider error.
- Selection, stress and frozen validation run only in that order.
- Old failing-seed replays are diagnostics, not fresh evidence. Simulated reviewers are truth plus injected error, not clinicians.
- Results use synthetic identity truth and cached clinical verdicts. Live NATS migration requires a separate shadow/assist evaluation.
- No local paths, identifiers, secrets, binaries or models in Git. Never commit the forbidden older plan file. No push or merge without the user's explicit decision. Thin non-record runs after documenting them.
