# v7-merged: one pipeline, one integrated run

**Merge:**
- git 3-way merge in `pipeline/`: v5 base → gov, impact, ann, fhir. Forks untouched.
- Conflicts hand-fixed (main/services/linker). 9/9 `go test` pass.
- Integration bugs fixed:
  - `-ann` was not passed to child processes.
  - No async-publish ack timeout: attempt 1 hung after the NATS kill (`run/main_attempt1_stalled`).
  - The tamper verifier didn't load the parse cache, causing a false alarm.

**The run** (`run/main`, seed 2026):
- 1,500 CRC + 600 cervical loops, 84 opt-outs, 24 forged events.
- Mock Malaffi → FHIR Subscription; HNSW blocking.
- Reviewer: 4% error, double review on merges and "adequate", dual-reader merges.
- RBAC/consent, e-consult/teach-back, anchors.
- Chaos: kills of FHIR consumer, linker, closure-0+prep-0, NATS node n2. Feed: 18 drops, 26 duplicates, 36 reorders.
- **0 new LLM calls** (1,680 replies parsed from Gemma cache).

## Closure (same log for every tracker)

| | CRC P / R / false closures | Cervical P / R / false closures |
|---|---|---|
| **Wasl, live + review** | **1.000 / 0.960 / 0 of 570** (95% UB 0.67%) | **1.000 / 0.991 / 0 of 217** (95% UB 1.7%) |
| Wasl, no review | 1.000 / 0.242 / 0 | 1.000 / 0.785 / 0 |
| EID-only rule | 0.834 / 0.364 / 43 | 0.848 / 0.384 / 15 |
| Any procedure ≤120 d | 0.661 / 0.431 / 131 | 0.604 / 0.466 / 67 |

## Same run, other metrics

- **Reviewer-h/1k loops:** **15.6** (CRC 19.0, cervical 7.2). 814 items, 1,317 reads.
  - 25 wrong final sign-offs: 11 true links answered "different", 14 evidence misreads.
  - None wrongly said "adequate", so errors cost recall, not closures.
- **Review items/100 loops:** Arab 39.8, South Asian 34.9, Filipino 34.6, Western 43.5.
- **Wrong-patient:** 6 intents (28.6/10k) without the gate; **0 released** (UB 18/10k). All 6 held below τ.
- **Forged:** 24/24 denied, 0 effect.
- **Opt-outs:**
  - 0 sent after opt-out was on file; 11 suppressed.
  - 16 sent before the Consent arrived (cohort records it 0–0.5 d after referral).
- **Exactly-once:** 0 duplicate/lost of 15,769 outputs; 28,613/28,613 resources ingested once.
- **Live tamper:** **17/17** detected, 0 false alarms. The insider rewrite is caught only by external anchors.
- **Throughput:** **699 ev/s** (29,432 msgs, 42.1 s).
- **Drop-off:** loop accuracy 96.6% CRC, 98.8% cervical.
  - Top CRC: no acknowledgement 245, not booked 187.
- **E-consult / teach-back state:** 100% / 99.9%. **HNSW vs exact:** byte-identical.
- **Cervical colposcopy ≤3 mo (HSIL+/ASC-H):** 52.6% measured vs 55.3% true (n=38; target 80%).

## KPI honesty: CRC "colonoscopy ≤31 days, >90%"

**Spec (crc.txt):**
- §5.5.4.2: "within 31 days **of referral**".
- App. 2: "between **referral** after positive screening FIT test and conducting a follow-up colonoscopy".
- FIT+→referral is a separate indicator (95%).

**Denominator:** referred FIT+ patients, clock from referral. Not all FIT+, not only accepted. Counting non-attenders is not stated, so both readings shown. Accepted-only changes results ≤1.2 points.

| n=1,318 eligible referrals | All referrals | Colonoscopy seen only |
|---|---|---|
| Truth | 33.0% | 63.7% |
| **EID-only** | **12.4%** | 65.5% |
| **Wasl live** | **31.9%** | 63.5% |

- EID-only misses 434/683 colonoscopies and under-reports by **20.6 points**.
- Wasl: 1.1 points off, 98.9% per-loop agreement.
- This is measurement accuracy only: the real ~33% stays far below 90%.

## Run

```
./run.sh build && ./run.sh test
./run.sh main && ./run.sh tamper && ./run.sh report   # ~5 min
```

- Output: `results/report7.json`, `results/tamper_main.json`.
- Ports: NATS 14822–4/16822–4/18822–4, Malaffi 28880/28890. Kills own PIDs only.

**Limits:** simulated patients/reviewer, independent errors, one seed.
