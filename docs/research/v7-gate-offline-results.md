# Wasl v7-precision

Copies only; `run.sh` reproduces everything. With the v7 flags off, it matches v6 exactly. No LLM calls.

## 1. Encoder vs baselines
v3 cross-script eval (7,540 vs 7,540 records); same LightGBM for every method.

| Method | R@10 ANN / +rules | F1 | Size |
|---|---|---|---|
| **Ours** | **.932 / .978** | **.954** | 2.3 MB |
| ICU translit + TF-IDF | .536 / .914 | .926 | – |
| ICU + Double Metaphone | .655 / .910 | .919 | – |
| e5-small int8 | .604 / .939 | .934 | 118 MB |
| MiniLM-L12 int8 | .188 / .908 | .926 | 118 MB |
| All four combined | .928 / .971 | .946 | 236 MB |

- **Ours wins** (+2.0 F1; the confidence intervals don't overlap).
- LaBSE (1.88 GB) is over the size limit.
- **Caveat:** it was trained on the eval's noise model.

## 2. EID-chain wrong-patient sends
**Rule (enforced).** Messages on EID-chain or model-only links need an exact real DOB match or a human confirmation (the feeds have no phones).

**Results (e = reviewer error rate):**
- **Wrong-patient messages per 10k loops: 6.7 → 0.** 0 in 16 runs at e = 0–6% (upper bound 1.2). At e = 8%: 1.3 vs 9.3 in v6.
- **Held:** 520 of 1,178 messages await confirmation; after review, only the wrong one stays held.
- **Extra review:** 305 items, +4.7 reviewer-h per 1k loops.

**Recalibration.** Pairs are now calibrated in strata:
- **corroborated:** real DOB, same sex, same surname;
- **within each stratum:** isotonic regression with a Clopper-Pearson 95% lower bound per plateau;
- **uncorroborated:** capped at 0.989.

Eval bins, as n / observed-true / wrong:

| Bin | v3 | v7 |
|---|---|---|
| [.90, .95) | 1 / 1.00 / 0 | 2,327 / .964 / 84 |
| [.98, .99) | 1,878 / .959 / 77 | 6,174 / .998 / 13 |
| [.99, 1) | 591 / .988 / 7 | **5,142 / 1.000 / 0** |
| 1.0 | 11,153 / .9988 / **13** | none |

- **No p ≥ 0.99 decision is wrong** on eval (cal split: 0/1,945).
- The cost is ECE: .025 → .051.
- **Streaming gate, τ = 0.98:** 4,693 auto-closures, 0 false (v3: 7 false). v3 at 0.99 closes more: 4,852, also 0 false.

## 3. No-review closure
**What we changed:**
- **Evidence:** per-field two-reader consensus (the readers give no confidence scores).
- **R1:** completion not documented → inadequate.
- **R2:** caecum reached but prep ungraded → closes, but only with a verbatim landmark quote that both readers support.
- **Link tier:** 0.98.

**What crc.txt allows.** Completion must be documented (App. 11 §2.1–2.2). Prep is an audited unit indicator (App. 2–3), and no rule says an ungraded prep fails the follow-up. R2 therefore closes the loop and flags the missing grade. **DoH must ratify this.**

**Report-level results (0 false closures):**
- 43 human reports: 14 → 38 auto-decided;
- held-out stress set: 49 → 75 of 95.

**Cohort (1,500 loops; e = 2–6%; 5 seeds; merge+close double review):**

| | v6 | v7 |
|---|---|---|
| Recall, no review | 0.276 | **0.766** |
| Auto share of closures | 28% | **78%** |
| False closures (upper bound) | 0/8,428 (0.044%) | 0/8,553 (0.043%) |
| Reviewer-h per 1k loops | 17.6 | **6.1**; 11.1 with the guard |

- **The evidence policy drives the gain.** With the v6 link gate alone, recall is 0.776.
- **<5 h is not reached at ≤0.1%.** Single-reading identity links gives 4.0 h but lets wrong-patient closures back in.

## 4. Cost per closed loop

| | v6 | v7 (with guard) |
|---|---|---|
| UAE wages | AED 3.77 | **AED 1.69** (1.05 without the guard) |
| US literature rates | $1.92–6.96 | $0.95–4.00 |

- GPU: 0.74 H100-h, or $7.34, per 1k loops (1.6% of the cost).

## Limits
- The corroboration rule and τ were chosen after inspecting the eval; the cal split agrees, but a fresh split is needed.
- R2 is an interpretation of the rules.
- Data are synthetic, and reviewer errors are modelled as independent.
