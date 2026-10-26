# Wasl agent entry point

## Backend, integrations, pipeline, safety, evaluation, or deployment work

**Read [`docs/BACKEND-HANDOFF.md`](docs/BACKEND-HANDOFF.md) before inspecting or changing code.** It is the authoritative execution brief for the backend owner: current architecture, truthful implementation status, task order, module boundaries, acceptance tests, and API contract for the frontend.

## Product, evidence, pitch, or roadmap work

Read [`docs/BRIEF.md`](docs/BRIEF.md) first, then consult the relevant document under `docs/`.

## Repository truth

- `pipeline/` is the canonical runtime.
- `pipeline/v7-gate/`, `reader/`, and `booking/` contain useful but currently separate work. Do not describe them as integrated until an end-to-end pipeline run proves it.
- State whether any claim or result is **built**, **simulated**, or **designed**.
