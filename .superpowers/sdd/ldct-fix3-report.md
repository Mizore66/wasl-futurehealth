# LDCT safety fix round 3

Branch `feat/ldct-surveillance-fix3` from 9d47e25. Not pushed. Keys, corpus, caches and results untouched.

## Changes
- pipeline/ldctread.go
  - `lwOutstand` now matches need/needs/needed, require/requires/required and "to be considered" (was only "needed"/"required").
  - `lwContrast` gains however/although/though/whereas/yet.
  - New `lwContrastOK` (rather than / instead of / in preference to / as opposed to). In `ldctClauseClean`, every contrast match anywhere in the clause blocks unless it falls after the phrase and is one of those four.
- pipeline/ldctsurv_test.go: `TestLDCTFix3NeedRequireNeverClose` (7 cases), `TestLDCTFix3ContrastAfterPhraseNeverCloses` (8), `TestLDCTFix3ClauseCleanDirect` (8 direct), `TestLDCTFix3GenuinePositivesStillClose` (3).
- Round-2 case renamed "second contrast before the phrase" (see correction).
- Removed the process-only `.superpowers/sdd/ldct-fix-report.md` from the tree (git rm).

## Correction to the prior report
The earlier claim "every contrast match, not just the first" was meaningless: matches were scanned in order, and the old check only looked at matches ending before the phrase, so the earliest match decided and a later one could never matter. The old test "second contrast after an earlier plan" only exercised contrast before the phrase. The real gap was contrast AFTER the phrase, which was never examined. The new case `CT surveillance rather than PET, but biopsy preferred.` has an allowed first contrast ("rather than") and a blocking second one ("but"), so it fails if scanning stops at the first match.

## Evidence
- TDD: tests written first. Before the fix, all 7 need/require subtests, all 8 contrast subtests and the direct test failed. After: pass.
- `go test -run 'LDCT|Ldct|ldct' .` in pipeline: ok, 127 PASS lines in verbose output.
- `go vet .` clean; `gofmt -l` clean on touched files (gofmt realigned a few adjacent var lines in ldctread.go).
- Genuine positives still closing: "CT surveillance rather than immediate biopsy; repeat low-dose chest CT in three months.", "No tissue diagnosis. Surveillance CT in 3 months instead of biopsy.", "Biopsy was benign. Surveillance CT in 3 months."
- Full `go test .` in pipeline (the only Go module): `ok  wsi  128.212s`.

## Residual heuristic limits
Safe over-blocks (stay open, go to review):
- Any contrast word in a surveillance clause blocks, including benign "over 3 months" or "surveillance preferred to biopsy".
- A second surveillance phrase after "instead of" in the same clause blocks (tested, documented).

Unsafe-direction limits (not closed by this round):
- The contrast check is per clause (split on sentence end and semicolon). "Surveillance CT in 3 months. However biopsy preferred." puts the contrast in another sentence and is not caught by it.
- "Patient wants biopsy" with no contrast word and no outstanding word (want, prefer, favour, keen, would like are not in `lwOutstand`) does not block.
- Outstanding/refusal vocabulary is lexical; unlisted phrasings are not recognised.
- The override is reader-B-only; closing still needs two-reader agreement.
