# IFHAS held-out follow-up-note corpus (frozen before scoring)

Synthetic, hand-authored, deterministic (`python3 scripts/ifhas_heldout/build.py`). 52 follow-up notes (`hf01`-`hf52`) with explicit keys (`key.label`, `key.status`, `key.attended`) and design tags (`normal`, `boundary`, `negated`, `distractor`, `multi`, `dna`, `nothing`, `format`, `declined`). Status conventions (`managed`, `not_confirmed`, `pending`, `dna`, `indeterminate`) are in the build script docstring.

Provenance and independence limits: not derived from `scripts/ifhas_corpus.py` or `data/ifhas/followup_notes.json`. The author had seen the reader source and the dev corpus key distribution and first few notes, so this is held-out text but not blind to the reader design. Keys are the author's reading of the status definitions, not a clinician double-label. No model was used to write or label the texts. Coded-screening readers are not covered (they need FHIR observations, not free text); only the free-text follow-up readers are tested.

Frozen in git before any reader was run; nothing in the readers, prompts, caches or thresholds may change in response to its scores.
