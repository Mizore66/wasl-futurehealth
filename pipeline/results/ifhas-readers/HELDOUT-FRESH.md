# Fresh held-out IFHAS follow-up reader run

The 52-note synthetic corpus and key in `data/ifhas/heldout/` were frozen at `2555a27` before unchanged readers were scored at `35958dc`. No reader, prompt, production cache, generator or threshold was tuned after scoring.

- Reader A: 36/52 correct.
- Reader B: 34/52 correct; 8 answers were invalid or ungrounded and routed to review.
- Agreed but wrong: 0/52; to review: 29/52 (56%).
- Unsafe-direction single-reader errors remain, including pending domains read as managed or `not_confirmed`; disagreement, indeterminate output or grounding caught every error in this reader-level check.

`v2_heldout_fresh_test.json` is the verbatim score output and `v2_heldout_fresh_gemma_cache.json` contains only these fresh-text reader-B answers. The scoped review replayed the cache and reproduced every count.

This is non-blind, single-author synthetic evidence without clinician labels, not an end-to-end false-closure result. It covers only free-text follow-up readers, not IFHAS coded/panel screening readers. The integrated generator still reuses development texts. See `../ldct-readers/HELDOUT-FRESH.md` for the combined original note and `../../RESULTS.md` for the full disclosure.
