# IFHAS quote-grounding and pending-plan fix

Branch feat/ifhas-grounding-pending, from 8a883fc. Changed: pipeline/ifhasread.go; new pipeline/ifhasground_test.go. No keys, corpus, caches or results touched.

## Changes
- Grounding: `ifhasGroundedQuote`. A verbatim quote is kept as is. Otherwise it is cut at a sentence end only if (a) the kept part is made of complete verbatim sentences of the note and (b) the cut tail starts with a quote delimiter, comma or digit and consists only of date/number artefacts or verbatim note fragments. Any natural-language suffix (invented plan or decision) leaves the quote ungrounded. The top-level quote and every finding quote are checked; one invalid required quote still makes reader B unavailable.
- Request side: quote schema `maxLength` 240 and `pattern` forbidding quote delimiters; `max_tokens` 360; `finish_reason=length` is recorded as an error and never replayed; cache entries carry `rev` ("grounding-1") and `runIfhasCache` re-asks entries from another revision. Not exercised against a live llama-server (none was run).
- Pending vs managed (reader A): negated or future actions are discounted before the plan test ("no medication commenced", "to be arranged", "not yet started", "will decide"); "deferred/postponed/on hold" and home/ambulatory monitoring requested force pending; bare "given" no longer counts as a plan (only medication/treatment/prescription/referral given). Review and hold logic untouched.

## Evidence (fresh2 cached reader B; A = regex label; temporary probe, removed)
Before (8a883fc) to after, reader B available:
- f2i-001, 024, 027, 028, 040, 060: false to true
- f2i-031: B true to true. Reader A managed to pending. Before, A and B agreed on managed (wrong); after, B still says managed, so they disagree and the note goes to review.
- f2i-046: false to false. The top-level date tail trims, but its finding quote is a paraphrase, so B stays unavailable.
- f2i-034, 006, 037: false to false (fabricated or ungrounded).
- f2i-004: false to true. Its quotes are verbatim once the date tail is cut, but B says not_confirmed vs A pending, so no agreement (`TestIfhasFresh2F004NeverAgrees`). The brief expected it to stay unavailable; it is available but disagreeing and still goes to a human.

Tests: baseline `go test .` in pipeline: ok 130.7s. After: `go test -run Ifhas`: 30 passing; `go test .`: ok 129.4s. gofmt clean on touched files, go vet clean.
New tests: fresh2 runaway tails, fresh2 fabricated stay unavailable, f2i-046 and f2i-004 specifics, generic trimming accept/reject set (hallucinated plan suffixes, invented insulin, partial sentences, junk only), one invalid finding quote, schema bounds, truncated-response handling, generic pending/managed positive and negative cases, f2i-031.

## Needs a fresh fair test
fresh2 is development data now and must not be reported as held-out for IFHAS follow-ups. The new schema, stop/length handling and revision tag need real Gemma re-asking (`wsi7 ifhascache`) on a newly authored, frozen corpus. The pending/managed regexes and trimming were tuned on f2i examples, so accuracy on unseen notes is unmeasured.
