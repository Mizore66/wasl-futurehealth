# Fresh held-out reader run (LDCT and IFHAS)

Corpora: `data/ldct/heldout/` and `data/ifhas/heldout/`, frozen in the git commit that precedes the commit adding these files.
Run once with the unchanged production readers (reader A regex; reader B local Gemma 3 4B Q4_K_M with the shipped prompt v2), via `wsi7 ldctcache` / `wsi7 ifhascache`. No tuning, no paid or external model calls.

Files:
- `v2_heldout_fresh_test.json` (here and in `../ifhas-readers/`): verbatim tool output.
- `v2_heldout_fresh_gemma_cache.json`: the reader-B answers for these texts, kept separate from the production caches in `models/`.

The `note` string in the LDCT raw output ("dev corpus only ...") is a stale label printed by the tool; this run is the held-out corpus. The corpus authors had seen the reader design (see the corpus READMEs), so this is held-out text, not a blind test. The IFHAS run covers only the free-text follow-up readers.
