# Fresh LDCT/IFHAS eval harness: report

Branch `feat/fresh2-eval-harness`, from `3c99b05c99964fb90c055daf9d7b20e8d2f54997`. Status: **built** (harness only). No corpus, label, cache or score exists; no model was called; readers, thresholds, caches, corpus data and results were not modified.

## What was added

- `scripts/fresh2_eval/fresh2.py`: stdlib CLI/library.
- `scripts/fresh2_eval/LABELLING-GUIDE.md`: blind guide (exact schema, allowed labels, rule definitions, unit/threshold tables, coded-string rules, provenance declarations, model-labelled/not-clinician-verified semantics).
- `scripts/fresh2_eval/README.md`: layout, semantics, freeze order.
- `scripts/fresh2_eval/test_fresh2.py`: 71 tests, written first (initial import failed), fixture placeholder texts only.
- `pipeline/fresh2_reader_test.go`: `TestFresh2Reader`, a skipped-by-default adapter calling existing `LDCTJudge`, `IfhasNoteJudge`, `IfhasScreenJudge` unchanged and replaying cached reader B only.

Reuse: agreement/kappa follows `docs/labelling/agreement.py` (formula pinned by a test) and the corpus layout follows `scripts/ldct_heldout` / `ifhas_heldout` (`{id,text}` reports). `docs/labelling/make_template.py` and `agreement.py` are hard-wired to `reports43.json`/CSV and run on import, so they were not reused; **no generic blind sheet generator was added** (the `template` subcommand writes a JSON skeleton with ids only, which the guide's schema needs).

## Capabilities and tests

| Requirement | Where | Tests |
|---|---|---|
| Validate 60+60 text-only items incl. IFHAS panel + coded input | `validate_corpus` | counts, dup id/text, leaked key/label/tags fields, coded required/forbidden, kind/programme, min per kind, provenance, contamination vs existing corpora |
| Ingest two independent label files | `validate_label_pair` | exact grammar for all four kinds, ids, hash binding, guide hash, isolation declarations, clinician flag, model provenance, distinct ids, identical files, same-family warning |
| Agreement / kappa | `kappa`, `build_key` | values, degenerate case, formula parity |
| Agreed key, disagreements `unadjudicated` | `build_key` | disputed items get `key: null` and both labels, no majority or guess, order-independent, deterministic; screening agrees only if panel and coded labels both agree |
| Freeze order and integrity | `check_freeze`, `build_manifest` | strict ancestry, equal commits, labels at corpus commit, protected reader/cache edits, label edits, fresh cache edits, dirty and untracked files, hand-edited key, pre-existing results |
| Score once | `run_score` | check runs before reader; exclusive `mkdir` results; second run refused; failed reader leaves nothing |
| Agreed and disputed reported separately | `compute_scores`, `render_markdown` | agreed-but-wrong, review split by whether key is decisive, disputed block has no accuracy fields |

Final run: `python3 -m unittest discover -s scripts/fresh2_eval -p 'test_*.py'` -> 71 tests OK (about 90 s, mostly git). Adapter smoke test against the real readers on four placeholder items worked (reader B correctly `unavailable` with no cache; screening panel and coded agreed).

## Next-phase commands (run from repo root)

1. Freeze readers: commit this branch; its commit is `reader_code_commit` (or any later commit that leaves `fresh2.DEFAULT_PROTECTED` untouched).
2. Author corpus to `data/fresh2/corpus.json` in canonical form (write with `fresh2.dumps`; `provenance` required). Then:
   `python3 scripts/fresh2_eval/fresh2.py validate-corpus` and commit corpus alone (= `corpus_commit`).
3. Per labeller (fresh isolated session, given only the guide and corpus):
   `python3 scripts/fresh2_eval/fresh2.py template --labeller-id <id> --kind model --out data/fresh2/labels/labeller_a.json` (and `_b`). Fill, then:
4. `python3 scripts/fresh2_eval/fresh2.py ingest` (writes `key.json`, `agreement.json`; prints kappa). Commit label files + key + agreement (= `labels_commit`). Do not hand-edit.
5. Reader B cache for the fresh texts (local Gemma only; needs `llama-server` on :8791 and `go build -o /tmp/wsi7 ./pipeline`):
   `python3 scripts/fresh2_eval/fresh2.py export-inputs /tmp/fresh2-inputs`
   `/tmp/wsi7 ldctcache -ldctdata /tmp/fresh2-inputs/ldct -out data/fresh2/reader-cache/ldct_gemma_cache.json`
   `/tmp/wsi7 ifhascache -ifhasdata /tmp/fresh2-inputs/ifhas -out data/fresh2/reader-cache/ifhas_gemma_cache.json`
   (run from `pipeline/` with absolute paths; their printed scores are against a nil key and are meaningless). Commit the two caches (= `reader_cache_commit`).
6. `python3 scripts/fresh2_eval/fresh2.py manifest --reader-code-commit <sha> --out data/fresh2/FREEZE.json`, commit it, then `python3 scripts/fresh2_eval/fresh2.py check-freeze`.
7. Score once: `python3 scripts/fresh2_eval/fresh2.py score`, then commit `data/fresh2/results/`; `check-freeze --after-score` afterwards.

## Concerns

- `FREEZE.json` is committed after the freeze commits; it is excluded by design (it is not in the protected or frozen lists) but means `manifest` must be derived from git, not trusted by name alone. `check-freeze` verifies every commit it names.
- Step 5 is a local model call (Gemma 3 4B), the same as the previous held-out run; it is outside this workstream. If it is not run, reader B is `unavailable` for all free-text items and the score reports that.
- `ifhas-screen` has no reader B model (coded results are reader B), so `coded_label` is the one case where the guide asks a labeller to reproduce unit/plausibility checks; labeller disagreement there will be informative, and may be large.
- The guide's rules were written from the published standards and the label definitions in the existing corpus READMEs, not by tracing reader code, but they deliberately mirror the existing label conventions so scores stay comparable. Any wording the first labellers misread is a guide defect; changing the guide changes its hash and invalidates label files, so pilot it on non-corpus text first.
- Python 3.15 was used locally; nothing version-specific is relied upon.
- Test-suite wall time is high (about 90 s) because each freeze test creates a git repo.
