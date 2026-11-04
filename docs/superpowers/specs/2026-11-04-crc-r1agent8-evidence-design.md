# CRC r1agent8: evidence-investigator agent with human double-read — design

Approved 4 Nov 2026. Synthetic data only; no human double-labelling is required for this experiment. No live-NATS deployment or clinical-effectiveness claim is included.

## Why

r1photo8's strict demographic guard reduced contamination but lost two closures under simulated reviewer error. Missing-demographic true pairs require human merge decisions, creating more opportunities for mistakes. This experiment tests whether a tool-using evidence investigator can supply one reliable read while the simulated human retains final sign-off.

The agent is evaluated on synthetic identity truth and existing cached clinical verdicts. The 43-report human-labelling task is independent and remains pending.

## Architecture: sidecar plus deterministic replay

A Python sidecar, `scripts/agent_investigator.py`, investigates eligible missing-demographic link-review items through allow-listed tools. It never receives raw filesystem access or live NATS access.

1. Run the three pipeline arms to produce paired initial lanes.
2. The sidecar builds sanitized evidence bundles and writes `agent_recs.ndjson` containing item ID, pair, label, citations, model metadata and tool-call trace.
3. Offline replay treats a valid recommendation as one read, never as a sign-off. The existing simulated human and senior-read rules remain authoritative.
4. If replay raises new eligible items, the sidecar investigates those too. Iterate to the existing fixed point, subject to the call caps. Uncovered items fall back to the ordinary human double-read and count against coverage.
5. Control arms use the identical offline replay without agent recommendations.

Agent responses are temperature 0, structured JSON and cached by SHA256 of model, prompt, tool schema and evidence bundle. The cache is the replay and audit record.

## Eligible population and safety boundary

Eligible items are link questions joined by unordered registration pair to `near/demographic_missing` trace records. The controller performs this join outside the agent's evidence sandbox.

The agent never sees DOB/sex conflicts, conflicting valid Emirates IDs, merge-constraint failures or any hard-conflict case. Existing cannot-link and cluster checks still apply to every resulting merge. A recommendation cannot create a sign-off by itself.

For each item the agent returns `same`, `different` or `uncertain`, with citations. Malformed output, unsupported claims, invalid citations, timeout or exhausted per-item budget becomes `uncertain`. Every citation must resolve through the tool API and support the stated field.

## Read semantics

| Agent read | Simulated human read | Replay behavior |
|---|---|---|
| `same` | `same` | Agreement; the agent replaces the second human read |
| `same` | `different` | Existing senior human tie-break decides |
| `different` or `uncertain` | `same` | Existing human double-read continues unchanged |
| `different` or `uncertain` | `different` | Human rejection stands |

The agent never writes `fhir.review`. Human sign-off remains mandatory. Human reviewer hours count only human reads; agent calls, tokens and latency are reported separately.

## Tools and leakage controls

The sidecar exposes only:

- `get_record(pk)`: registration demographics, name/script and non-truth metadata.
- `get_identifiers(pk)`: the registration's identifiers and validity flags, not truth or donor identity.
- `get_report_fields(report_id)`: structured cached reader fields required for evidence review, not raw report text or answer keys.
- `get_photo_verdict(photo_ref)`: cached image-reader verdict, never the true photo class.
- `get_pair_context(item_id)`: the sanitized review item, pair and model score.

The evidence sandbox contains only generated, allow-listed bundles. It excludes `truth.json`, person source files, `reviewer_decisions.ndjson`, `funnel.json`, `link_funnel.ndjson`, answer keys, donor maps and any field encoding person identity or outcome. The controller reads the trace solely to select eligible pairs; the agent does not receive it.

No real MTSamples text, raw clinical document or real patient data is sent to Gemini. Photo pixels are not required; only cached structured verdicts cross the tool boundary. Tests attempt path traversal, forbidden-file access, truth-shaped identifiers and fabricated citations and must reject them.

## Gemini runtime and limits

Use the existing no-cost Gemini access through the OpenAI-compatible gateway pattern in `scripts/reader_cache_api.py`. Model name/version, prompt version and tool-schema version are pinned in the run manifest. Secrets remain environment-only.

- At most 2 Gemini calls and 6 tool calls per item.
- At most 1,500 uncached Gemini calls in selection and 2,500 in validation.
- A 20-item smoke validates schema, citations, sandboxing and provider availability before a full run.
- Reaching a call cap or encountering a provider error aborts the experiment; no paid fallback and no silent partial-coverage run.
- Cached responses are required for identical replay and do not count toward the call cap.

## Arms and fit

Three arms, all with noise v3, the cached image reader and the same pairfeat8 chain refit at the experiment HEAD:

1. `r1photo8ref`: pairfeat8 plus `-r1review`, no demographic guard (unguarded comparator).
2. `r1photo8`: pairfeat8 plus `-r1review -demoguard`, ordinary simulated human review (guarded-human comparator).
3. `r1agent8`: pairfeat8 plus `-r1review -demoguard`, with `-agentrecs` during offline replay (candidate).

There is no default-model arm in this experiment. The harness must support an explicit comparator-baseline arm rather than silently inserting the default model. All three arms share one fit and identical cohorts. Earlier candidates and seed protections remain unchanged.

## Seeds

- Prompt and tool debugging: already-used r1photo8 development lanes 827 829 839 and smoke 101.
- Fresh selection: 881 883 887.
- Fresh validation: 907 911 919 929 937.

Selection and validation seeds were checked absent from the harness and fit configurations. No prompt, tool or threshold tuning on selection or validation seeds.

## Gates (pre-registered; stop at the first failure)

| Metric | Requirement |
|---|---|
| CRC recall | Candidate ≥ each comparator in selection, stress and validation |
| `false_merge_pairs` and `cross_person_edges` | ≤ guarded-human in every mode; strictly below unguarded in selection/validation and not above in stress |
| uncapped wrong-pair link-review pairs | ≤ guarded-human in every mode; strictly below unguarded in selection/validation and not above in stress |
| human reviewer hours | ≤ guarded-human; ≤ 1.10 × unguarded |
| SAFETY | Zero false closures, wrong releases, replay mismatches, analysis errors and trace errors in every lane of every arm |
| eligible-item agent coverage | ≥ 80% in each mode |
| citation validity | 100% of accepted recommendations have valid supporting citations |

**Post-hoc zero-floor amendment (5 Nov 2026, before any candidate verdict):** the completed selection comparators had zero uncapped wrong-pair link-review pairs in the unguarded arm, making the original “strictly below” rule impossible for every candidate. For false merges, cross-person edges and uncapped wrong-pair reviews, a strict comparison now means candidate < unguarded when unguarded > 0, and candidate = 0 when unguarded = 0. The guarded-arm ceilings, stress rules, other gates, models, prompts and seeds are unchanged. This amendment was made before inspecting a candidate metric or verdict and will be disclosed in the results.

Impure clusters, wrong final sign-offs and default-model comparisons are reported, not gated. Report the agent's confusion matrix, abstentions, uncovered items, invalid-output count, calls, tokens, latency and human seconds saved. Agent time must not be hidden in human reviewer hours.

## Verification and error handling

- Unit tests cover every read-semantics cell, fixed-point replay, new-item coverage, invalid output/citation fallback and hard-conflict refusal.
- Sandbox tests prove no truth, answer-key, reviewer-decision or real-text access.
- Harness tests prove identical cohorts/chain across arms, correct comparator mapping, per-arm flags and every gate independently.
- Smoke must produce 20 valid cached recommendation traces with no forbidden access and no provider error.
- Run selection, stress, frozen validation and read-only diagnosis, stopping at the first failed gate.
- Document agent accuracy separately from end-to-end recall and disclose that the simulated human is truth plus injected error, not an independent clinical reader.

## Disclosures and next step

- Results are synthetic and use existing cached clinical verdicts whose limitations are unchanged; no clinician-labelled evidence or clinical effectiveness is claimed.
- Gemini's output is another model's judgment, not ground truth. Prompts and tools are tuned only on already-used development lanes.
- The unguarded and guarded comparators share the user-chosen reliable-DOB assumption from r1photo8; results are not comparable with v1/v2 experiments.
- A live NATS migration is a separate later experiment: first live shadow mode, then assist mode with human sign-off, deterministic fallback and a kill switch. Selection here does not authorize deployment.

## Constraints

- No `/tmp`, no `/Users` paths, no patient-level identifiers, secrets, binaries or models in Git.
- Commit trailer `Co-Authored-By: Claude Code <noreply@anthropic.com>`. No push or merge.
- Path-only commits. Never commit `docs/superpowers/plans/2026-11-02-crc-link-recall.md`.
- After results are documented, thin raw lanes and feature CSVs of non-record runs; keep summaries and the run of record.
