# Final evidence/test cleanup report

Branch: feat/final-evidence-test-cleanup-wt2 (the requested name was already checked out in another worktree), base cd7fc9e866af58ebb6cf6e148490e383cc7cd837.
Commit: 6c912b4a318234be7a0be478edb030300e6ec3f0 (not pushed).

## Changes
- README.md: 0/124 sentence now says it was against a single-author key, not zero error, and points to 4 (fresh2) and 3 (fresh3) agreed-but-wrong in pipeline/RESULTS.md.
- docs/BACKEND-HANDOFF.md: original non-blind single-author caveat kept as historical; added reference to later independent two-labeller fresh2/fresh3 rounds (4 and 3 agreed-but-wrong).
- scripts/fresh2_eval/README.md: 95 -> 102 (71 + 12 + 12 + 7).
- with-open fixes: test_fresh2.py (key load, FAKE_READER script x3, score.json, reader-input docs), test_analyze_fresh2.py (guide, ANALYSIS json/md), analyze_fresh3.py (--check), plus test_analyze_fresh3.py line 66 (found by the ResourceWarning run). Assertions unchanged.
- __pycache__: `git ls-files | grep -c pycache` = 0; no pycache file is tracked. Nothing deleted.

## Results
- First run, `python3 -W error::ResourceWarning -m unittest discover -s scripts/fresh2_eval -p 'test_*.py'`: Ran 102 tests, OK (142.5s), but showed one remaining ResourceWarning in test_analyze_fresh3.py:66 (fixed after).
- After that fix: test_analyze_fresh3.py with -W error::ResourceWarning: 7 tests OK. A final full 102-test rerun was started under heavy machine load; see the return message for its outcome.
- `FRESH_EVAL_DIR=data/fresh3 FRESH_EVAL_PER_PROGRAMME=50 python3 scripts/fresh2_eval/fresh2.py check-freeze --after-score` on the committed tree: "freeze OK", rc=0.
