# IFHAS pending-rule fix round 1 (branch feat/ifhas-pending-fix1, base 0b40175)

## Changes
- pipeline/ifhasread.go: the global `ifhDeferred` regex is replaced by `ifhPendingNow(j, d)`, judged per domain and per clause
  (split at `;`, `.`, newline). Clauses naming only other domains are ignored. A strong started action (not negated or
  future) in a clause for the domain outranks a monitoring request and a deferral that is not about the treatment decision.
  A negated start or a deferred decision/treatment still holds it pending. Negated start is now `no|not ... commenced|started|initiated|prescribed`
  within one comma-free clause, so "No known drug allergies, amlodipine started today" is managed.
  `ifhPend` (repeat test requested, awaited, declined, ...) is evaluated first and is unchanged, so review/hold behaviour is unchanged.
- pipeline/ifhasfix1_test.go (new): `TestIfhasActionDeferralPrecedence` (21 cases: the review counterexamples plus generic
  positive/negative, other-domain deferral, diagnosis-only, hold cases); `TestIfhasCacheRevisionReask`;
  self-contained fixtures for f2i-004/006/034/037/046 (embedded text and cached reader-B JSON; checked equal to data/fresh2/corpus.json when present, no skips).
- pipeline/ifhasground_test.go: removed the skipping fresh2 helper tests, the dead `_ = text` / `t.Setenv("X","")` code.
- f2i-004 test now asserts: top-level quote and every finding quote ground, B is available (`fields` ok), regex label differs from B label,
  and `IfhasNoteJudge` returns GemmaHit, not Agree, label "disagree".
- f2i-034/006/037/046 test fails (Fatalf) if a fixture is missing; no skip.

## Cache revision
`runIfhasCache` (wsi7 ifhascache) re-asks any entry whose Rev differs from `ifhasRequestRev` (empty or older), errors, or other model.
`IfhasNoteJudge` deliberately replays ANY cached entry whatever its Rev, so frozen reader caches under data/*/reader-cache (pre-Rev) keep
reproducing the evidence they were scored with; stale entries are refreshed only by an explicit cache run. Documented in the test comment;
`TestIfhasCacheRevisionReask` uses an httptest stub: Rev "" and "grounding-0" -> exactly 1 ask and entry stamped current; current Rev -> 0 asks;
judge replays older Rev without a network call.

## Evidence
- TDD red: before the change, `go test -run 'Ifhas(Action|CacheRev|Fresh2)'` failed 12 case assertions in TestIfhasActionDeferralPrecedence
  (e.g. "Amlodipine 5 mg commenced; home BP monitoring requested to titrate" -> pending; "No known drug allergies, amlodipine started today." -> pending).
- After: `go test -count=1 -v -run Ifhas .` 32 PASS, 0 FAIL (includes TestIfhasRegexDevCorpus dev corpus, still all correct).
- `go test -count=1 .` in pipeline: ok 129.95s. `gofmt -l` clean on touched files; `go vet .` clean.
- Keys, corpus and results untouched.

## Caveats
fresh2 f2i-* notes are development data, not held-out, for IFHAS follow-ups. Rules were tuned on authored examples; accuracy on unseen notes is unmeasured.
`ifhDeferScope` is a keyword heuristic for "deferral concerns the treatment decision"; wording like "follow-up deferred" with a start in the same clause stays managed.
