# Local Gemma 3 4B (Q4_K_M, 2.5 GB, llama.cpp on Metal), run 2026-10-21

Same harness and 43 MTSamples colonoscopy reports as before. The reference is the cached Gemma-4-26B verdicts. The model file was already on disk from the Oct 19 navigator run.

| Reader | Joint state + reason vs 26B | κ | Two-reader gate (regex + this reader) | p50 latency |
|---|---|---|---|---|
| regex | 41/43 | – | – | – |
| **Gemma 3 4B, JSON-schema constrained** | **35/43** | 0.68 | 9 auto-resolved, **0 wrong**, 34 to review | 6.9 s |
| Gemma 3 4B, free-form | 5/43 (34 parse fails) | – | – | 3.0 s |
| Gemma-4-26B (reference) | 43/43 | 1.0 | 14 auto-resolved, 0 wrong, 29 to review | ~2.1 s (proxy) |
| gemma-3-270m / SmolLM-135M | ≤10/43 | – | – | <1.5 s |

- **Errors are one-sided.** All 8 disagreements are histology over-calls: in 19 reports the 4B said a histology result was present when the 26B said no, and 5 of those flipped "awaiting histology" to "criteria met".
- **The two-reader gate catches them.** Regex disagrees with the 4B on those reports, so they go to review and none closes wrongly.
- **Cost of the 4B:** fewer auto-closures (9 vs 14 of 43) and more review (34 vs 29).
- **Easy next step:** a stricter histology prompt, or a deterministic regex for histology alone.

## Orchestration attempt (orch4b.py, run 2026-10-21)
The design was fixed before running:
- snippets per field;
- one enum question per field, with negative examples;
- a verbatim quote, checked by a landmark/diagnosis lexicon;
- caecum asked 3 times, majority vote.

| Variant | Joint vs 26B | vs human key | Gate auto-resolved (26B: 14) | Gate wrong |
|---|---|---|---|---|
| Single schema call | 35/43 | – | 9 | 0 |
| Fully orchestrated | **29/43** | 29/43 | 8 | 0 |
| **Hybrid: schema call (caecum/prep/polyps) + orchestrated histology question**, recombined offline from the two measured runs | **40/43** | 40/43 | **13** | **0** |

**Why full orchestration was worse:** snippet retrieval dropped context. Prep agreement fell from 41 to 28 and caecum from 40 to 34. The histology question with a lexicon check, however, fixed histology: 24 → **43/43**.

**What to build:** one full-context schema call plus a separate histology check. This is 2 calls per report and about 12 s. The hybrid is computed from the two measured runs, not run as a single pipeline, so it still needs one confirmation run.

## CONFIRMED: hybrid as a single pipeline (hybrid4b.py, 2026-10-21)
Agreement with the 26B model is **40/43**, and **40/43** with the human key.

| Field | Matches 26B |
|---|---|
| Caecum | 40/43 |
| Prep | 41/43 |
| Polyps | 40/43 |
| Histology | 43/43 |

- **Two-reader gate with regex:** **13 auto-resolved (26B: 14), 0 wrong.**
- **Speed:** median 7.5 s per report (2 calls) on the M2. Total wall time 339 s for 43 reports.
- **Reproduction:** this matches the offline recombination exactly.
- **Runs fully local:** 2.5 GB Q4 model, llama.cpp, no proxy.
