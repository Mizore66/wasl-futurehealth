# LDCT held-out reader corpus (frozen before scoring)

Synthetic, hand-authored, deterministic (`python3 scripts/ldct_heldout/build.py`). 41 screening reports (`hs01`-`hs41`) and 31 work-up notes (`hw01`-`hw31`), each with an explicit key (`key.label`) and design tags (`normal`, `boundary`, `negated`, `distractor`, `format`, `words`, `modifier`, `contradiction`, ...). Label conventions are in the docstring of the build script.

Provenance and independence limits: the texts were not copied or templated from `scripts/ldct_corpus.py` or `data/ldct/*.json`. The author had seen the reader source (regexes, prompts, schema) and the key distribution and first few texts of the dev corpus, so this is held-out text but not a blind-to-the-reader test; boundary and distractor cases were written deliberately to probe the documented rules. The key is the author's reading of Lung-RADS v2022 and the pipeline's work-up label rules, not a clinician double-label. No model was used to write or label the texts.

Frozen in git before any reader was run on it; nothing in the readers, prompts, caches or thresholds may change in response to its scores.
