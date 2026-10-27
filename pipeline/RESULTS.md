# v7-merged: one pipeline, one integrated run

> **Stage 1 (27 Oct):** the canonical pipeline now runs the **v7strict** gate by default (`-policy v7strict`). The table below is the v6 legacy baseline. Current numbers: `results/v7strict/summary.txt`. Comparison: the "Stage 1: v6 vs v7strict" section at the end.

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


## Stage 1: v6 vs v7strict (same cohort and seed, full integrated NATS run)

`scripts/demo.sh` (`WASL_POLICY=v7strict` is the default; `v6` is kept for comparison). Every decision carries its `policy` field, and the signature covers it.

| Metric | v6 (legacy) | **v7strict (canonical)** |
|---|---|---|
| CRC with review: P / R / false closures | 1.000 / 0.960 / 0/570 | **1.000 / 0.961 / 0/571** |
| CRC no review: R | 0.242 | 0.232 |
| Cervical with review: false closures | 0/217 | 0/217 |
| Cervical no review: R | 0.785 | 0.744 |
| CRC ≤31-day KPI (Wasl vs truth 0.330) | 0.319 | 0.320 |
| Wrong-patient messages released | 0 | 0 |
| Messages released before a late opt-out was recorded | 16–17 | **5** (the DOB guard holds more; the consent hold removes the rest in stage 3) |
| Reviewer-h per 1k loops | 15.7 | 21.5 (strict prep plus guard items) |
| Exactly-once / tamper / forged | 0 dup · 17/17 · 0/24 | 0 dup · 17/17 · 0/24 |

**v7strict means:**
- **Stratified calibrator** (`models/iso_v7.json`): links not corroborated by DOB, sex and surname are capped below the 0.99 trust tier.
- **Closure link tier** τ = 0.98.
- **Second-identifier guard:** messages on EID-chain or model-only links need an exact non-placeholder DOB match or human confirmation.
- **Per-field two-reader consensus.**
- **Strict prep:** an ungraded prep never closes.

**Tests:** `v7_test.go` covers the strict prep hold, per-field consensus, the DOB guard, the calibrator cap, and policy in the signature.

*Simulated cohort, one seed. The final 5-seed frozen evaluation is stage 7.*


## Stage 2: live local reader (Gemma 3 4B), integrated

`WASL_READER=live scripts/demo.sh` needs a local server: `llama-server -m gemma-3-4b-it-Q4_K_M.gguf --port 8791 -c 8192 -ngl 99 -np 4`.

**How it works:**
- **Reader client:** `pipeline/reader.go`, a Go client for the OpenAI-compatible endpoint. It has a health check, timeout, bounded retries, JSON-schema output, a singleflight per report and a sha256-keyed ledger.
- **Two calls per report (the hybrid):**
  1. a full-context schema call for caecum, prep and polyps;
  2. a dedicated histology question with a verbatim quote, a diagnosis-lexicon check and a visual-impression guard.
- **Grounding:** a positive claim without a grounded quote is downgraded to its conservative value, so an ungrounded answer can only make closure *less* likely.
- **Fail-safe:** a timeout, HTTP error, schema violation or unhealthy server gives "reader unavailable". The two readers then cannot agree, so the case goes to human review. It never closes automatically (`reader_test.go`).
- **Replay determinism:** live answers go to `<run>/reader_ledger.jsonl`. Audit, tamper and report replay the ledger instead of calling the model. Warm-up (`wsi7 readerwarm`) asks each unique report body once before the stream starts.
- **Provenance:** every evidence block records `reader_model`, the per-field values from both readers, the rule and any documentation flags.

**4B reader alone on the 43 MTSamples reports** (answer key LLM-labelled and cross-checked):
- histology field correct on 43/43;
- state agrees with the cached 26B on 35/43;
- 1 report the 4B alone would call "adequate" is actually inadequate. The regex reader disagrees there ("incomplete exam"), so the two-reader gate holds it.
- Median 8.9 s per report on an M-series laptop, 2 calls.

**Integrated runs** (same cohort and seed, v7strict):

| Metric | Cached 26B answers | **Live local 4B** |
|---|---|---|
| CRC with review: P / R / false closures | 1.000 / 0.961 / 0/571 | **1.000 / 0.955 / 0/567** |
| CRC no review: R | 0.232 | 0.123 (the 4B is more conservative, so more cases go to review) |
| Cervical with review | 0/217 | 0/217 |
| Reviewer-h per 1k loops | 21.3 | 25.8 |
| Exactly-once / tamper / forged | 0 dup · 17/17 · 0/24 | 0 dup · 17/17 · 0/24 |

The live ledger for this run is committed (`results/v7strict-live/reader_ledger.jsonl`), so the live result can be replayed without the model: `-reader replay`.
