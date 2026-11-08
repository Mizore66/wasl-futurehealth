# r1agent13: reverse-near invalidation with parallel seed lanes

Synthetic data only. No live-NATS deployment, clinician-labelled evidence or clinical-effectiveness claim is included.

## Prior result and fix

r1agent12 failed development with recall 659/707 versus unguarded 660/707 and wrong-pair reviews tied 1–1. The missed loop was seed 1063 `ahs-mushrif/ServiceRequest/AHS-sr-13`: a near clinic→lab edge led to an already-linked lab→hospital edge, but reverse-near propagation did not mark the clinic loop as affected. The closure never received the review item.

Commit `c34b336` adds reverse-near invalidation. On the old seed-1063 diagnostic the sidecar investigates the newly exposed pair and the loop closes with zero audit/report7 mismatches. This is diagnostic evidence only, not fresh selection evidence.

## Experiment

Use the unchanged pairfeat9 chain, noise v3, cached image reader, bounded two-hop near-path review with resolved-edge handling, deferred first investigation and the sanitized evidence sidecar. Agent semantics, model, prompt and schema are unchanged.

Arms:
1. `r1photo9ref`: pairfeat9 plus `-r1review`, no demographic guard.
2. `r1photo9`: pairfeat9 plus `-r1review -demoguard`, ordinary human review.
3. `r1agent13`: the guarded stack with reverse-near invalidation and the evidence agent.

The agent never signs or overrides hard conflicts. Agent `same` plus human `same` replaces the second human read; disagreement uses senior human r3; `different`, `uncertain`, invalid and uncovered output retain ordinary human double review. Human time excludes agent time.

## Fresh seeds and parallel execution

- Selection: **1093 1097 1099**.
- Validation: **1103 1109 1117 1121 1123**.

These were fresh at registration. Earlier experiment seeds remain used/protected. No prompt, tool, model, threshold or gate tuning is allowed on r1agent13 seeds.

`pairfeat_parallel.sh` runs one isolated binary and NATS port range per seed and creates the standard composite arm layout. The same git HEAD and binary hash are required. Agent calls use the shared cache with a persistent cross-process cap lock; no seed can exceed the global budget. Development and stress may overlap; validation remains frozen until those gates pass.

## Gates

Carry forward the existing gates and the disclosed zero-floor amendment:

- Candidate recall ≥ each comparator in selection, stress and validation; no relaxation.
- False merges, cross-person edges and uncapped wrong-pair reviews ≤ guarded; strictly below unguarded when it is > 0, otherwise candidate = 0.
- Human reviewer hours ≤ guarded and ≤ 1.10 × unguarded.
- Zero false closures, wrong releases, replay mismatches, analysis errors and trace errors in every lane.
- Deterministic replay in every arm; eligible coverage ≥ 80%; accepted citation validity 100%.

Stop at the first failed gate. Report impure clusters, wrong final sign-offs, agent confusion, abstentions, uncovered/invalid items, calls, tokens, latency and human seconds saved. Keep agent latency separate from human hours.

## Verification and limits

- Run the full Go, agent and harness suites plus parallel-runner and concurrency review.
- Preserve the smoke result accurately: 19 actionable recommendations and one schema-valid abstention, no provider error.
- Old failing-seed replays are diagnostics, not fresh evidence. Simulated reviewers are truth plus injected error, not clinicians.
- Results use synthetic identity truth and cached clinical verdicts. Live NATS migration requires a separate shadow/assist evaluation.
- No local paths, identifiers, secrets, binaries or models in Git. Never commit the forbidden older plan file. No push or merge without the user's explicit decision. Thin non-record runs after documenting them.
