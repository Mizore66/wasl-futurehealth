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
| `data/evidence/` | 300 synthetic App 14 colonoscopy reports (spec = answer key), 43 MTSamples reports, pathology templates | Evaluation only |
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

**Final frozen evaluation** (`scripts/demo.sh final`, ~50 min; 5 seeds × 2 noise models, code frozen at `c2a74e2`, reader v7.2, every held-out person with 2,040 CRC, 816 cervical and 1,088 lung-nodule loops per run, with the ungraded-prep query, the rejected-referral hand-off, the surname-variant and Emirates ID identity rules, the corrected MTSamples biopsy key, and one second-identifier question per registration pair; [`pipeline/results/final/`](pipeline/results/final/SUMMARY.md)). On the **held-out v2 noise model**:
- CRC P 1.000, R 0.775 (95% CI 0.762–0.789).
- **0/2,997 CRC false closures** (95% upper bound 0.12%); cervical 0/1,300; lung 0/1,899 (lung recall 0.950).
- 0/13,479 wrong-patient messages; 0/8,678 messages sent without granted consent.
- KPI error 0.032 vs 0.207 for EID-only tracking.
- Reviewer time 15.2 h per 1,000 loops (17.2 before the identity rules).
- Rejected-referral hand-off: 296 of 300 packets built (0 extra), 0 false closures after re-referral; the 4 missed are rejections the linker never tied to the patient.
- Review load across name origins: max/min ratio 1.21 per seed (95% CI 1.10–1.33) and 1.08 pooled over seeds, under the 1.25 limit. The pooled gap is no longer statistically significant (p = 0.13 against shuffled origin labels; it was p < 0.001 at `e4c7553`). Two of five seeds are still above 1.25: max/min over four unequal groups is noisy (no-bias 95th percentile about 1.24 per seed). Placeholder birth dates (1 Jan / 1 Jul; 25% of Arab vs 6% of Western persons) still send more messages to a human on purpose: they cannot identify a person. It costs reviewer time, not accuracy.
- CRC review load and recall by age band and by sex: max/min 1.06 by age band (p = 0.33) and 1.05 by sex (p = 0.04; women who also have a cervical loop get about 4 more link reviews per 100 CRC loops); recall 0.76–0.79 in every group, 0 false closures.

Recall is lower than in earlier runs (0.851 held-out at `54ff4ba`) for one main reason: 20 of the 43 MTSamples reports never grade the bowel prep, and those loops now wait for the endoscopy unit's grade or a day-15 clinician decision instead of a reviewer grading prep from text that has no grade. The unit's replies are simulated with assumed rates (see "Ungraded bowel prep" in [`pipeline/RESULTS.md`](pipeline/RESULTS.md)). The first run of this evaluation (at `b53ce89`) found 4 CRC false closures, all one bug: a day-15 "adequate" answer on an ungraded report closed the loop while histology was still pending. It is fixed (the day-15 answer decides the prep only) and disclosed. On dev noise, recall is 0.842 with 0/3,153 CRC, 0/1,402 cervical and 0/2,026 lung false closures. Recall rose from 0.704 (held-out) when the MTSamples key was corrected: three reports with colon biopsies but no polyps had been keyed as closing without histology, so Wasl's correct wait counted as a miss. A human "adequate" against a report that explicitly says it is inadequate now needs a confirming review. Two identity rules were added after the freeze at `a3ad83a`, also disclosed: surname variants (shared name tokens, one-edit transliteration) corroborate an exact-DOB link, and a matching Emirates ID with agreeing name, sex and DOB counts as the second identifier for a message. They cut reviewer time by about 10% with every safety count still 0. The hand-off packet logic was then fixed after a trace of its misses, and the generator's hand-off truth was corrected in two edge cases. The held-out noise model was fixed (it crashed or garbled names with "İ" or non-ASCII initials), the MTSamples key was corrected on biopsies, and a 2-letter surname no longer corroborates a link alone after it caused one wrong-patient message in a run. Last, a human's second-identifier answer now counts for every loop of the same registration pair, and the evaluation uses every held-out person so the smallest name-origin group is larger. Reader v7.2 ignores gastric and duodenal tissue in combined EGD + colonoscopy reports (MTSamples u4 had always waited for histology) and adds two colon tissue phrasings; recall rose to 0.775 held-out. A CRC loop no longer messages a woman about her colposcopy appointment (a duplicate of the cervical loop's message, found by the new age-and-sex check) (all disclosed in `pipeline/RESULTS.md`); the cohort has no wrong-person Emirates IDs, so only unit tests cover a relative's card. The closure tier is 0.99: an earlier run at 0.98 found one wrong-patient closure, so we raised it (a disclosed post-hoc change; see the "Final code" section of [`pipeline/RESULTS.md`](pipeline/RESULTS.md)). The human double-labelling of the 43 reports is still pending ([`docs/labelling/`](docs/labelling/)).

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
- Report texts: a 300-report synthetic DoH App 14 corpus (main reader test) plus 43 MTSamples texts (real dictation; the site permits educational use with credit). The synthetic corpus found two reader blind spots, now fixed and disclosed (biopsies as tissue taken; Boston scores graded by number): see "Synthetic App 14 report corpus" in [`pipeline/RESULTS.md`](pipeline/RESULTS.md). Seven more 300-report corpora (three by Claude agents, then GPT-6.1 Sol, Gemini 4 Argon, Kimi K3.1 Pro and GPT-6.1 Sol again), each frozen before any reader ran, were each the fair test of the previous reader fix. Three found an unsafe phrasing class (correction addenda; an earlier procedure's histology next to a pending result; a caecum seen but not reached). Across the seven fair tests, 6 of 2,100 unseen reports auto-closed unsafely with the 26B, which is **6 of 211 automatic closures (2.8%)**; with the local 4B, 10 of 137 (7.3%). Each class was fixed by a generic rule; the current reader v7.2 has 0 unsafe auto-closures on all seven corpora and MTSamples, and the last two fair tests (corpus 7 for v7, corpus 8 for the shipped v7.1) passed with both readers. So a pilot starts in shadow mode: every automatic closure is confirmed by a clinician until 300 in a row are correct, then a random 10% audit continues. See the last two corpus sections of [`pipeline/RESULTS.md`](pipeline/RESULTS.md).
- **All patients, cohorts, reviewers and capacity are simulated.** No real patient data is used anywhere.
- Each claim in the docs is marked *built*, *simulated* or *designed*.
