# Wasl (وصل)

**Evidence-backed follow-up tracking for Abu Dhabi screening programmes, across providers.**

This is our entry to the [Future Health Digital Innovation Challenge](https://hack2skill.com/event/futurehealthchallenge/), challenge #5: *Closing the Screening-to-Care Loop*. **Due 21 Nov 2026.**

## The problem
A woman has a positive stool test (FIT+) at her clinic. Her colonoscopy, if it happens, may be at a different hospital, under a Latin spelling of her name and with no Emirates ID. Her clinic never finds out.

In Abu Dhabi only **30.5%** of FIT+ patients had a colonoscopy, after a median **72 days**, against a 31-day standard (SEHA, BMC Cancer 2021).

In our simulation, Emirates-ID-only tracking reports **12.4%** of referrals reaching colonoscopy within 31 days. The true figure is **33.0%**; Wasl measures **32.0%**. With EID-only tracking, about 1 in 4 patients on the "chase" list have already had their colonoscopy.

## What Wasl does
- **Links** a patient's records across providers and scripts (Arabic ↔ Latin), using a self-trained name encoder, HNSW matching and a calibrated scorer.
- **Closes** a loop only with evidence: two readers (DoH rules plus a local Gemma 3 4B) must agree on the colonoscopy report. Anything uncertain goes to double human review.
- **Owns every step:** named owner, SLA clocks, teach-back, consultation routing, escalation.
- **Proves it:** hash-chained provenance for every closure decision.
- **Certifies its own threshold:** the closure link tier is chosen by Learn-then-Test on a separate calibration cohort, with a finite-sample bound on false closures and wrong-patient messages.
- **Reads photos too (hold-only):** a third reader checks caecal-landmark and bowel-prep photos. It can hold a loop or route it to a one-click human grade; it can never close one.
- **Runs more than one loop type:** incidental lung nodules (Fleischner 2017) run on the same engine as CRC and cervical screening.
- **Suggests the next step:** simulated booking decision support, direct-to-procedure pathways (DoH App 9 checklist) and bowel-prep risk (Dik 2015 score), always as suggestions a clinician confirms.

## Read first

| | |
|---|---|
| [`docs/BRIEF.md`](docs/BRIEF.md) | **Start here.** Status, evidence, constraints, pilot plan, build plan to 21 Nov |
| [`docs/PITCH.md`](docs/PITCH.md) | Short pitch |
| [`docs/WRITEUP.md`](docs/WRITEUP.md) | Full sourced write-up (to be condensed into the submission) |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | Phase 2–3 design: queue control, multi-loop engine, image reader, Learn-then-Test gate |
| [`docs/CHALLENGE.md`](docs/CHALLENGE.md) | The official challenge statements |
| [`docs/research/`](docs/research/) | Sourced research: costs, deployment, governance, standards, value model, notices |
| [`docs/API.md`](docs/API.md) | **Frontend contract**: endpoints, fields, the three demo cases. Static samples are in [`docs/api-samples/`](docs/api-samples/) |

## Code

| Folder | What | Status |
|---|---|---|
| `pipeline/` | Go + NATS JetStream streaming pipeline: **HL7 v2 MLLP** + FHIR ingest, linker, **v7strict closure gate**, live/cached evidence reader, consent hold, reviewer, provenance, tamper audit | Built; integrated runs in `pipeline/results/` |
| `booking/` | Constraint-safe booking engine (Python), simulator and network-pooling test; `suggest.py` attaches simulated booking suggestions and the capacity condition to pipeline cases | Simulated decision support (not live scheduling) |
| `image/` | Hold-only image reader: HyperKvasir training, evaluation, live sidecar. Weights in `models/image/` (published with the dataset authors' permission) | Built; hold-only |
| `reader/` | Python reference for the hybrid reader prompt (the Go client in `pipeline/reader.go` is canonical) | Reference |
| `patient-parsers/` | Frozen test sets and scores for the teach-back / e-consult parsers | Evaluated |
| `projection/` | Before/after uplift projection from published trial effects | Projection, not a result |
| `models/` | Small model artefacts (isotonic calibrator, cached reader outputs) | |
| `data/evidence/` | 43 MTSamples colonoscopy reports with answer key; pathology templates | Evaluation only |
| `data/lung/` | Synthetic CT index-finding and follow-up report corpus with answer key (lung-nodule loop) | Synthetic |
| `sources/` | DoH CRC/cervical/breast/RAI standards and legal texts (text) | Reference |

## Build and run

**Requirements:** Go ≥1.22, Python 3.10+, and `nats-server` v2.10+ (`brew install nats-server`).

```bash
scripts/demo.sh          # check → build → test → cohort → integrated run → audit → tamper → report  (~5 min)
```

The script checks assets and ports before it starts. If anything is missing, it stops with the exact fix. Steps can also be run one at a time: `scripts/demo.sh check|build|test|cohort|run|audit|tamper|booking|report|ltt|final`. Then `scripts/demo.sh serve` starts the API for the frontend (see `docs/API.md`).

Outputs go to `out/` (gitignored):
- `out/cohort/`: the deterministic synthetic cohort (seed `WASL_SEED`, default 2026);
- `out/run/<policy>/report7.json`: all metrics;
- `out/run/<policy>/dashboard.json` and `review_queue.json`;
- `out/run/<policy>/tamper.json`.

**Expected baseline** (`v7strict` policy, one seed, simulated cohort; `WASL_POLICY=v6` gives the legacy run):

| | Result |
|---|---|
| CRC with review | P 1.000, R 0.948, 0/563 false closures |
| Cervical with review | P 1.000, R 0.991, 0/217 false closures |
| ≤31-day KPI | EID-only 0.124, truth 0.330, Wasl 0.316 |
| Exactly-once under process + NATS kills | 0 duplicates, 0 final-state mismatches |
| Tamper | 17/17 attacks detected, 0 false alarms |
| Forged events | 0/24 with any effect |
| Consent hold | 0 messages released without granted contact permission (619 released, 373 held until consent, 32 cancelled) |

**Final frozen evaluation** (`scripts/demo.sh final`, ~50 min; 5 seeds × 2 noise models, code frozen at `82bceb2`; [`pipeline/results/final/`](pipeline/results/final/SUMMARY.md)). On the **held-out v2 noise model**:
- CRC P 1.000, R 0.862 (95% CI 0.858–0.866).
- **0/2,468 false closures** (95% upper bound 0.15%).
- 0/4,453 wrong-patient messages; 0/3,032 messages sent without granted consent.
- KPI error 0.038 vs 0.201 for EID-only tracking.

On dev noise, recall is 0.945 with 0/2,724 false closures. The closure tier is 0.99: an earlier run at 0.98 found one wrong-patient closure, so we raised it (a disclosed post-hoc change; see the "Final code" section of [`pipeline/RESULTS.md`](pipeline/RESULTS.md)). The human double-labelling of the 43 reports is still pending ([`docs/labelling/`](docs/labelling/)).

**Options** (environment variables for `scripts/demo.sh`):

| Variable | Values | What it does |
|---|---|---|
| `WASL_POLICY` | `v7strict` (default), `v7ltt`, `v6` | `v7ltt` uses the closure tier certified by Learn-then-Test (`scripts/demo.sh ltt` re-runs the calibration) |
| `WASL_READER` | `cached` (default), `live` | `live` calls the local Gemma 3 4B reader |
| `WASL_SOURCE` | `fhir` (default), `hl7` | `hl7` ingests HL7 v2.5.1 over MLLP |
| `WASL_IMAGE` | `off` (default), `cached`, `live` | Hold-only photo reader |
| `WASL_LUNG` | `0` (default) or a loop count, e.g. `800` | Adds the incidental lung-nodule programme |

**Beyond the core loop** (each is optional and off by default, so the baseline above is unchanged):

| Feature | Result | Status |
|---|---|---|
| Learn-then-Test gate | Tier 0.99 certified on 12,600 calibration loops (0 false closures, 0 wrong-patient messages): P(both rates ≤ 0.1%) ≥ 95% for loops like the calibration set | Built; guarantee covers simulated data only |
| Image reader | Caecum AUC 0.998, prep AUC 0.991 on held-out photos; in an integrated run 0/571 false closures and 0 upgrades | Built, hold-only |
| Lung-nodule loop | 800 loops: P 1.000, R 0.993, 0/297 false closures, Fleischner recommendation accuracy 1.000 | Built on synthetic CT reports |
| Queue levers | At 1.2 slots per FIT+, direct-to-procedure plus prep targeting adds about +13 pp ≤31-day completion; no lever helps below 1.0 slot | Simulated |

Details: [`pipeline/RESULTS.md`](pipeline/RESULTS.md), [`image/RESULTS.md`](image/RESULTS.md), [`booking/RESULTS.md`](booking/RESULTS.md), [`pipeline/results/ltt/LTT.md`](pipeline/results/ltt/LTT.md).

**Live local reader:** run `llama-server -m gemma-3-4b-it-Q4_K_M.gguf --port 8791 -c 8192 -ngl 99 -np 4`, then `WASL_READER=live scripts/demo.sh`. The default (`cached`) replays recorded Gemma 26B answers and needs no model. Results for both are in `pipeline/RESULTS.md`.

## Data and honesty
- Names come from Wikidata (CC0).
- Endoscopy photos come from HyperKvasir (Borgli et al., *Sci Data* 2020; CC BY 4.0). Competition use is with the authors' written permission.
- Report texts are MTSamples. The site permits educational use with credit; we use them for evaluation only, and they will be replaced by a synthetic DoH App 14 corpus.
- **All patients, cohorts, reviewers and capacity are simulated.** No real patient data is used anywhere.
- Each claim in the docs is marked *built*, *simulated* or *designed*.
