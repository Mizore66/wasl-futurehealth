# Fresh LDCT / IFHAS evaluation and annotation harness

Standard-library Python plus one read-only Go test adapter. **Complete for two rounds.** fresh2 (60 items per programme) and fresh3 (50 per programme) have each been frozen, labelled blind and scored once; their corpora, labels, keys, reader caches and results are committed under `data/fresh2/` and `data/fresh3/`. No model is called by this code.

Design: a text-only corpus (60 LDCT + 60 IFHAS items) is frozen first; two independent labellers then label it blind from `LABELLING-GUIDE.md`; items they agree on get a key and every disagreement stays `unadjudicated` (never guessed); the reader caches for the fresh texts are built; then the existing, unmodified readers are scored once, with agreed and disputed items reported separately.

## Files

- `fresh2.py`: CLI and library (`validate-corpus`, `template`, `ingest`, `export-inputs`, `manifest`, `check-freeze`, `score`).
- `LABELLING-GUIDE.md`: blind guide with exact schema, allowed labels and rules. Its sha256 is recorded in every label file and checked.
- `test_fresh2.py`: 71 tests (102 across the directory: 71 fresh2 + 12 config + 12 `test_analyze_fresh2.py` + 7 `test_analyze_fresh3.py`), fixture texts only (`python3 -m unittest discover -s scripts/fresh2_eval -p 'test_*.py'`).
- `../../pipeline/fresh2_reader_test.go`: `TestFresh2Reader`, skipped unless `FRESH2_CORPUS` is set. Calls the existing `LDCTJudge`, `IfhasNoteJudge` and `IfhasScreenJudge` unchanged; reads no key; replays reader-B caches only (no model call).

## Data layout (under `data/fresh2/`; `data/fresh3/` has the same layout)

`corpus.json`, `labels/labeller_a.json`, `labels/labeller_b.json`, `key.json`, `agreement.json`, `reader-cache/ldct_gemma_cache.json`, `reader-cache/ifhas_gemma_cache.json`, `FREEZE.json`, `results/` (created exactly once by `score`). Both rounds also keep `labeller-input.json`, `LABELLER-INPUT-MANIFEST.json` and `author/`; fresh2 `results/` additionally holds `analysis.json` and `ANALYSIS.md` from `analyze_fresh2.py`.

Corpus items: `{id, programme, kind, text}` plus `coded` for `ifhas-screen`. Kinds: `ldct-screen`, `ldct-workup`, `ifhas-followup`, `ifhas-screen`; 60 items per programme, at least 10 per kind. Any `key`, `label`, `tags` or other extra field is rejected, so the corpus cannot leak answers. Items whose normalised text already appears in `data/ldct/**` or `data/ifhas/**` are rejected. The corpus must be saved in canonical form (`fresh2.dumps`): label files bind to its sha256.

## Semantics and provenance

Keys are **model-labelled and not clinician-verified** (the key file says so in `semantics.statement`). `clinician_verified` must be `false` in every label file; a human labeller does not change that. Every label file records labeller id, kind, model, model family, prompt sha256, run date, guide sha256, and the declarations `session_isolated`, `saw_reader_implementation`, `saw_other_labels`; ingest rejects a file that does not declare isolation. Two labellers from the same model family are accepted with a warning in the key and agreement report, because agreement then overstates independence. The corpus `provenance` block records who or what authored the text and whether the author saw reader code.

Agreement is reported as observed agreement and Cohen's kappa per kind and field (same formula as `docs/labelling/agreement.py`; kappa is reported as `null` with a note when both labellers use one identical label throughout). `ifhas-screen` items have two fields (`label` from the panel, `coded_label` from the coded string) and agree only if both match.

## Scoring rules

Agreed subset: reader A correct, reader B correct, B unavailable, readers agree, agreed-but-wrong, and routing to review split by whether the key was decisive. An agreed reader answer is wrong if it differs from the key, or if the key expects review (`indeterminate`, any `X` screening domain, panel/coded mismatch). Disputed subset: reader behaviour only (agreed or routed to review, and whether the agreed label matches either labeller); no accuracy is computed. Scores describe software on synthetic text, not clinical effectiveness.

## Freeze order

`reader_code_commit` < `corpus_commit` < `labels_commit` < `reader_cache_commit` < HEAD, each a strict ancestor of the next. `check-freeze` additionally requires: protected reader code and production caches unchanged since `reader_code_commit` (list in `fresh2.DEFAULT_PROTECTED`); corpus absent at the reader commit; label files and key absent at the corpus commit; fresh caches absent at the labels commit; no file frozen at its commit has changed since; the key equals what the committed label files regenerate; no uncommitted or untracked file under `data/fresh2/`, `scripts/fresh2_eval/` or a protected path; and no `results/` directory exists before scoring. `score` runs the check first and refuses to run the reader if it fails, creates `results/` with an exclusive mkdir, and so cannot be run twice.

## Configuring a later round (fresh3)

The evaluation root and item count are read from the environment; the defaults reproduce fresh2 exactly.

- `FRESH_EVAL_DIR` (default `data/fresh2`): repo-relative root for `corpus.json`, `labels/labeller_a.json`, `labels/labeller_b.json`, `key.json`, `agreement.json`, `reader-cache/*_gemma_cache.json`, `FREEZE.json` and `results/`. Absolute paths and `..` are rejected. The root is also the watched directory in `check-freeze`.
- `FRESH_EVAL_PER_PROGRAMME` (default `60`; fresh3 uses `50`; minimum 20): exact items per programme in `validate-corpus` and `ingest`.
- Items whose text already appears in any other `data/fresh*/corpus.json` (for example fresh2 when building fresh3) are rejected as reuse.

Running fresh3: `FRESH_EVAL_DIR=data/fresh3 FRESH_EVAL_PER_PROGRAMME=50 python3 scripts/fresh2_eval/fresh2.py check-freeze --after-score`. Its `reader_code_commit` is `8c675c1`, the commit after LDCT/IFHAS reader hardening, so the hardened readers are the protected code for fresh3.

Expected fresh2 behaviour after hardening: `check-freeze --after-score` with the default root (fresh2) now fails with `pipeline/ldctread.go changed since reader_code_commit` and `pipeline/ifhasread.go changed since reader_code_commit`, because it compares protected files to HEAD and the readers were deliberately hardened after fresh2 was scored. This is expected and is not a fresh2 freeze breach. fresh2's protocol is that protected reader files and production caches are unchanged from its `reader_code_commit` to its scoring commit `8a883fc`; `test_analyze_fresh2.py` checks exactly that (an empty `git diff` over the protected list between those two commits) and tolerates only the known post-scoring edits to those two readers and to `fresh2.py`.

Not generalised: the Go adapter `pipeline/fresh2_reader_test.go` (it takes the corpus and cache directory from the environment, which `score` passes from the configured root, but it is protected reader code and was not touched), `analyze_fresh2.py` (fresh2-only, pinned to fresh2 paths and score commit 8a883fc) and `LABELLING-GUIDE.md`.
