# LDCT safety fix round 1

Base 621e331, branch feat/ldct-surveillance-fix1. Files: pipeline/ldctread.go, pipeline/ldctsurv_test.go (new).
No keys, corpus, caches or results changed. fresh2 remains development data, not held-out.

## RED (before the fix, `go test -run 'LDCTReaderB|WrongDocumentContextT|IndeterminateAndIncomplete'`)
- TestLDCTReaderBOverrideRejectsUnsafeSurveillance: 22 of 22 subtests FAIL. Reader B's not_done answer was flipped to
  done/wk_surveillance for every review example (biopsy-contingent, "rather than", PET-CT then surveillance,
  not required / not appropriate / declined, unfit / unable to tolerate / refused / did not consent / no consent /
  chose not to proceed, awaiting / awaited / planned biopsy, malignant tissue, "unless", contradictory plan,
  history-only "Prior CT surveillance in 2023").
- TestLDCTWrongDocumentContextTightened: dental, cardiology, breast-screening, colorectal texts all gave
  wk_incomplete in both readers (should be indeterminate).
- TestLDCTIndeterminateAndIncompleteRouteReview: the dental fixture agreed as wk_incomplete.

## Fix (ldctread.go)
- ldctSurveillancePlan: runs on history-filtered current sentences (ldctCurrentSentences, same filter as reader A's
  workup path), split into `;` clauses. Every clause mentioning surveillance must be unconditional (lwCondition: if /
  unless / then / once / until ...), not the rejected side of a contrast (lwContrast), not negated before (lcNegNear) or
  after (lwFwdNeg) the phrase, and free of refusal words (lwRefusal). At least one such plan must exist.
- Whole-text blockers (stricter than history filtering, so history can only block, never enable): lwNotDone, lwRefusal
  (declined / refused / unfit / unable / cannot tolerate / did not consent / no consent / chose not to proceed, both
  word orders), outstanding tissue or PET (lwTissuePET with lwOutstand: awaiting / awaited / planned / arranged /
  booked / first / to follow ...), and affirmed malignancy.
- ldctWorkupContext replaces the broad lwCtx check: needs a lung/thoracic anchor (lwLungAnchor) plus a work-up term.
  A bare "nodule" is no longer an anchor. Genuine pending work-ups keep wk_incomplete.
- ldctReconcile comment no longer claims the override runs in both readers: it is reader-B-only (reader A sets
  not_done only when lwNotDone matches, which blocks the override). Rule 2 (wrong document) applies to both.
- Item 8: lwSurv broadening is unchanged and still affects reader A outcomes globally (the dev corpus test passes
  unchanged, 95 texts). Title regex stays anchored at text start: a title after a preamble does not match, which is
  safe (no override), and is documented in the comment.

## Tests (pipeline/ldctsurv_test.go, real ldctGemma.fields path)
- 22 unsafe surveillance cases, tissue pending / malignant, negated surveillance, history-only surveillance.
- Genuine MDT surveillance still closes through reader B (two texts, including the f2l-style "CT surveillance rather
  than immediate biopsy" text with "No tissue diagnosis").
- Title-only test: identical body with and without MDT title, no other blocker present; only the title differs.
- Wrong-document negatives (dental, cardiology, breast screening, colorectal) and positives kept (pending biopsy,
  MDT to come, screening referral, EBUS pending, lung biopsy), both readers.
- Downstream: through runLung with LR4B scan, an agreed-nothing indeterminate note stays open with an evidence review
  (hasReview "evidence"); an agreed wk_incomplete note stays open with hold reason workup_incomplete_or_mdt_pending.

## GREEN
- `go test -run 'LDCT' .` ok; `go vet .` clean; `go test -count=1 ./...` in pipeline: `ok wsi 130.767s`.

## Remaining heuristic limits
- All guards are keyword/regex over text; paraphrases not in the lists ("would rather avoid CT follow-up") can slip
  through. Condition words (`then`, `if`) block some genuine plans (safe direction: review).
- Refusal / outstanding checks are whole-text and deliberately over-block (e.g. a history sentence "previously refused
  biopsy" blocks the override). That costs recall, not safety.
- lwLungAnchor can be satisfied by an unrelated lung mention in a multi-problem document; lwCtx remains broad
  (e.g. "oncolog" alone) once an anchor exists. A dental note that also names the lung would pass.
- Contrast detection only handles the rejected phrase appearing after the contrast word; "CT surveillance, not biopsy"
  is not treated as a contrast.
- Reader B never closes alone; the override still requires reader A agreement, and reader A itself cannot reach it.
- Dev corpus numbers are not a fair test; no held-out claim is made.
