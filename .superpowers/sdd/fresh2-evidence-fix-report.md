# fresh2 evidence fix report

Branch feat/fresh2-evidence-fix1 (from 8a883fc). No push.

## Outputs
- `python3 -m unittest discover -s scripts/fresh2_eval -p 'test_*.py'`: Ran 83 tests, OK (71 harness + 12 new in test_analyze_fresh2.py).
- `python3 scripts/fresh2_eval/fresh2.py check-freeze --after-score`: `freeze OK`
- `python3 scripts/fresh2_eval/analyze_fresh2.py --check`: `analysis files up to date`
- `git diff --stat 8a883fc HEAD` over corpus, labels, key, reader caches, SCORE.md, score.json, reader_raw.json, fresh2.py, guide, pipeline/*.go, models: empty (no protected file changed).

## Findings
1. data/fresh2/labeller-input.json = byte copy of the sibling workspace out/fresh2-blind/corpus.json (sha256 0b98766d...a84, items identical to canonical corpus, canonical sha 16536712...f7). Manifest data/fresh2/LABELLER-INPUT-MANIFEST.json holds both prompt texts (guide + suffix; prefix + guide + suffix), their hashes (reproduced exactly: 166f80b7... gemini-3.5-flash, 7b4395fe... kimi-k3.1-pro), model families and the declaration. Prompt texts were reconstructed from the run scripts in /tmp (f2g/run.py, kimi_run/run.py), and the hashes match the label files.
2. IFHAS screening reported separately: A 6/28, coded B 28/28 (2 are keys whose coded_label is itself unavailable), 27/30 panels say fasting vs guide's random glucose.
   - Review's "24 glyc X" was not reproduced. Reader A returns glyc=X on 26/28 agreed panels; 21 of those have a keyed glyc value (the rest are keyed X). Both unadjudicated panels are also X under A. Disclosed in ANALYSIS.md.
3. ANALYSIS.md / analysis.json: 4 wrong = 1 closing (f2i-031), 3 holding (f2l-002, f2l-021 wrong-kind; f2l-029 surveillance->incomplete).
4. Unavailable = 13: 11 ifhas-followup (all cache hits, all fail main quote grounding; 9 only that, 2 also finding quotes: f2i-037 unadjudicated, f2i-046) + 2 ifhas-screen (implausible coded: f2i-017 HbA1c 53 %, f2i-022 glucose 12.4 mg/dL); LDCT 0. Grounding re-implemented from Go normQuote/lungGrounded. Manifest re-derived via fresh2.build_manifest: identical.
5. RESULTS.md, SUBMISSION-DRAFT.md, BRIEF.md updated; older 0/124 left intact; no 0-unsafe claim.

## Remaining limitations
- Labelling ran in sibling workspaces; the input copy and prompt reconstruction are not a recording of the API calls (raw transcripts not archived). Timestamp 04:07 is file mtime.
- The key's own X cells for glyc (4 coded-X) and the fasting/random guide mismatch are unchanged by design.
- analysis.json records check_freeze errors excluding "uncommitted change" lines so it is reproducible.
- Plausibility of coded values in the analysis is a heuristic cross-check; Go rule is the authority.
