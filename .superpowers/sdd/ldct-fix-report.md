# LDCT wrong-document and MDT-surveillance fix

Branch `feat/ldct-context-surveillance` (from 8a883fc). Status: built, tested locally, not pushed.

## Change (pipeline/ldctread.go)
New `ldctReconcile(fields, text)` runs inside both reader A (`regexLDCTWorkup`) and reader B (`ldctGemma.fields`), after field extraction and before labelling. It does not touch `ldctWorkupLabel`, keys, corpora, caches or stored results.

1. Surveillance consistency: if the title line is an MDT outcome/decision, the extracted fields say workup=not_done with outcome surveillance/pending, tissue is not malignant/pending, the text records no real non-performance (declined, cancelled, postponed, machine unavailable), and an affirmed (non-negated) surveillance plan is stated, then workup is set to done and outcome to surveillance. Otherwise nothing changes.
2. Wrong document: a would-be `wk_incomplete` becomes `indeterminate` unless the text has genuine work-up context (MDT, biopsy/tissue terms, PET/CT, surgery/oncology/radiotherapy, Lung-RADS or direct screening referral, diagnostic CT) or states a work-up outcome (annual/surveillance). A passing "nodule identified at screening" is not context.
3. `lwSurv` extended to "CT surveillance" and "repeat (low-dose) (chest) CT in".

Review/hold safeguards are unchanged: the only new downgrade is wk_incomplete to indeterminate (both route to human review); the only new closure needs an MDT-titled document with an affirmed surveillance plan, no tissue and no non-performance. Both readers must still agree.

## Tests
New `pipeline/ldctcontext_test.go`: generic positive/negative cases for both rules (pneumonia discharge, smoking-cessation passing mention, pending biopsy, MDT-booked, screening referral stay wk_incomplete; surveillance override positive, plus no-title, declined, no-plan, negated, malignant-tissue negatives).
- Before: dev regex test (95 texts) passes; new tests absent. fresh2 replay (existing caches, unchanged readers): f2l-002, f2l-021, f2l-029 all agreed-but-wrong as wk_incomplete.
- After: `go test -run LDCT` passes (including TestLDCTRegexDevCorpus, 95 texts, unchanged). fresh2 replay diff on LDCT items: f2l-002 and f2l-021 wk_incomplete to disagree (both readers indeterminate; goes to review, key indeterminate); f2l-029 wk_incomplete to wk_surveillance (matches key). No other LDCT item changed. Full `go test .` (package wsi): ok, 133 s.
- Note: an interim context regex wrongly demoted f2l-058 (a genuine respiratory clinic, patient undecided, key wk_incomplete); fixed by adding "following/after/referred from screening" as context. This shows the context list is a heuristic.

## Fresh fair-test requirements
fresh2 is now development data: these rules were written against f2l-002/021/029 (and f2l-058 was used to tune the context list). Do not call it held-out or re-score it as such. A fair test needs a newly authored LDCT corpus with unseen documents (wrong-document types beyond pneumonia/smoking cessation, e.g. cardiology, dental, other-organ oncology that mentions a nodule; MDT outcomes with wording variants; vague work-up notes), authored and keyed independently of this change, frozen before reader run, readers/caches run once, and scored once.

## Concerns
- Context regex is keyword based; terse genuine work-up notes without these terms would be demoted to indeterminate (safe direction, extra review) and wrong documents containing e.g. "oncology" could stay wk_incomplete.
- Reader-B caches are keyed by text and replayed unchanged; rules apply at replay, so no cache regeneration was needed.
