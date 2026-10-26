# Wasl (وصل)

**Evidence-backed follow-up tracking for Abu Dhabi screening programmes, across providers.**

This is our entry to the [Future Health Digital Innovation Challenge](https://hack2skill.com/event/futurehealthchallenge/), challenge #5: *Closing the Screening-to-Care Loop*. **Due 21 Nov 2026.**

## The problem
A woman has a positive stool test (FIT+) at her clinic. Her colonoscopy, if it happens, may be at a different hospital, under a Latin spelling of her name and with no Emirates ID. Her clinic never finds out.

In Abu Dhabi only **30.5%** of FIT+ patients had a colonoscopy, after a median **72 days**, against a 31-day standard (SEHA, BMC Cancer 2021).

In our simulation, Emirates-ID-only tracking reports **12.4%** of referrals reaching colonoscopy within 31 days. The true figure is **33.0%**; Wasl measures **31.9%**. With EID-only tracking, about 1 in 4 patients on the "chase" list have already had their colonoscopy.

## What Wasl does
- **Links** a patient's records across providers and scripts (Arabic ↔ Latin), using a self-trained name encoder, HNSW matching and a calibrated scorer.
- **Closes** a loop only with evidence: two readers (DoH rules plus a local Gemma 3 4B) must agree on the colonoscopy report. Anything uncertain goes to double human review.
- **Owns every step:** named owner, SLA clocks, teach-back, consultation routing, escalation.
- **Proves it:** hash-chained provenance for every closure decision.

## Read first

| | |
|---|---|
| [`docs/BRIEF.md`](docs/BRIEF.md) | **Start here.** Status, evidence, constraints, pilot plan, build plan to 21 Nov |
| [`docs/PITCH.md`](docs/PITCH.md) | Short pitch |
| [`docs/WRITEUP.md`](docs/WRITEUP.md) | Full sourced write-up (to be condensed into the submission) |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | Phase 2–3 design: queue control, multi-loop engine, image reader, Learn-then-Test gate |
| [`docs/CHALLENGE.md`](docs/CHALLENGE.md) | The official challenge statements |
| [`docs/research/`](docs/research/) | Sourced research: costs, deployment, governance, standards, value model, notices |

## Code

| Folder | What | Status |
|---|---|---|
| `pipeline/` | Go + NATS JetStream streaming pipeline: FHIR ingest, linker, closure gate, reviewer, provenance, tamper audit | Built; one merged run (`RESULTS.md`) |
| `pipeline/v7-gate/` | v7 gate: stratified calibration, DOB guard, per-field reader consensus, strict prep rule (`*.go.v7` = files to merge) | Tested offline; **to merge** |
| `booking/` | Constraint-safe booking engine (Python) + simulator, including the network-pooling test | Simulated; **to merge** |
| `reader/` | Local Gemma 3 4B hybrid evidence reader + regex baseline | Verified on 43 reports; **to merge** |
| `patient-parsers/` | Frozen test sets and scores for the teach-back / e-consult parsers | Evaluated |
| `projection/` | Before/after uplift projection from published trial effects | Projection, not a result |
| `models/` | Small model artefacts (isotonic calibrator, cached reader outputs) | |
| `data/evidence/` | 43 MTSamples colonoscopy reports with answer key; pathology templates | Evaluation only |
| `sources/` | DoH CRC/cervical/breast/RAI standards and legal texts (text) | Reference |

## Build and run

**Requirements:** Go ≥1.22, Python 3.10+, and `nats-server` v2.10+ (`brew install nats-server`).

```bash
scripts/demo.sh          # check → build → test → cohort → integrated run → audit → tamper → report  (~5 min)
```

The script checks assets and ports before it starts. If anything is missing, it stops with the exact fix. Steps can also be run one at a time: `scripts/demo.sh check|build|test|cohort|run|audit|tamper|report`.

Outputs go to `out/` (gitignored):
- `out/cohort/`: the deterministic synthetic cohort (seed `WASL_SEED`, default 2026);
- `out/run/main/report7.json`: all metrics;
- `out/run/main/dashboard.json` and `review_queue.json`;
- `out/run/tamper.json`.

**Expected baseline** (one seed, simulated cohort):

| | Result |
|---|---|
| CRC with review | P 1.000, R 0.960, 0/570 false closures |
| Cervical with review | P 1.000, R 0.991, 0/217 false closures |
| ≤31-day KPI | EID-only 0.124, truth 0.330, Wasl 0.319 |
| Exactly-once under process + NATS kills | 0 duplicates, 0 final-state mismatches |
| Tamper | 17/17 attacks detected, 0 false alarms |
| Forged events | 0/24 with any effect |
| Known gap | 16 messages sent before a late opt-out was recorded. The fix is the consent hold, stage 3 of `docs/BACKEND-HANDOFF.md` |

**Local reader (separate until stage 2):** start `llama-server -m gemma-3-4b-it-Q4_K_M.gguf --port 8791`, then run `python reader/hybrid4b.py 8791`.

## Data and honesty
- Names come from Wikidata (CC0).
- Report texts are MTSamples. The site permits educational use with credit; we use them for evaluation only, and they will be replaced by a synthetic DoH App 14 corpus.
- **All patients, cohorts, reviewers and capacity are simulated.** No real patient data is used anywhere.
- Each claim in the docs is marked *built*, *simulated* or *designed*.
