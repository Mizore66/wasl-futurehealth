# Submission evidence consistency audit

Branch `feat/submission-evidence-audit`, based on `3c99b05`.

## Files changed
- Added `scripts/check_evidence_consistency.py` (stdlib checker; exit 1 on findings).
- Added `scripts/test_evidence_consistency.py` (14 tests: real-repo pass + mutation tests per rule).
- Wording-only edits: `README.md`, `docs/PITCH.md`, `docs/WRITEUP.md`, `docs/BACKEND-HANDOFF.md`, `docs/SUBMISSION-DRAFT.md`.
  - PITCH frozen-evaluation bullet updated to final 075abc5 figures (0.785, 0/3,035, 0/16,486, 0/11,650, 11.6 h); older run named as superseded. PITCH "LLM-produced key until human double-labelling" replaced with the verified model-assisted wording.
  - WRITEUP frozen-evaluation row marked as the earlier 5496563 run, superseded; stale "no clinician labels" limit clarified.
  - README line 91: 0.782 marked as intermediate, superseded by 0.785.
  - HANDOFF: stale "Double-label the 43 reports" instruction replaced with the verified wording.
  - SUBMISSION-DRAFT: kappa sentence now carries "not independent human double-labelling".
- No results, models, corpus data or metrics changed.

## Rules checked
current-figures, stale-figure, kappa-data, kappa-wording, accuracy-claim (20 decisions / 19 correct / 0 unsafe, fair -> inadequate_prep), stale-labelling, ldct-ifhas-caveat (0/124 agreed-but-wrong, non-blind, generators reuse development texts), agent-wording (r1agent10 five fewer closures than unguarded, validation did not run), status-labels, overclaim.

## Commands
- `python3 scripts/check_evidence_consistency.py` -> 0 finding(s) (13 before fixes)
- `python3 -m unittest scripts/test_evidence_consistency.py` -> Ran 14 tests, OK
- `python3 -m unittest scripts/test_deploy.py scripts/test_api_smoke.py` -> 38 tests, 5 errors, all `ModuleNotFoundError: yaml` (environment, unrelated to this change)

## Remaining limitations
- Regex/line-based: the checker catches phrasing drift it knows about, not arbitrary new claims. Negation detection is a per-line heuristic.
- Stale-figure rule only covers the specific superseded 5496563 figures; other historical RESULTS.md sections are not scanned.
- Authoritative final figures are parsed from the 075abc5 section of RESULTS.md; renaming that heading needs a checker update.
- Go suites not run (docs-only change).
