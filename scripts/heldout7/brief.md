Write a deterministic Python 3 generator (standard library only; no network, no LLM calls, no clock, no file reads) of 300 synthetic colonoscopy reports from Abu Dhabi day-surgery and hospital endoscopy units, with an answer key. This is a held-out test corpus for a report-reading system. You are an independent author: write it from this brief and your own knowledge of real endoscopy documentation.

Reply with exactly two code blocks:
1. ```python : the complete gen_heldout7.py. Random seed {SEED}. Running `python3 gen_heldout7.py` writes reports_heldout7.json in the same directory as the script. Running it twice must give byte-identical output.
2. ```markdown : README.md, written as a dataset card: how it works, counts per tag, keying decisions. Name the author only as "independent author ({AUTHOR})". No absolute paths.

JSON format:
{"source": "Seventh independent author ({AUTHOR}) - deterministic stdlib generator gen_heldout7.py, random seed {SEED}, 300 synthetic Abu Dhabi colonoscopy reports, 2026-10-29", "reports": [...]}
Each report: {"uid": "u001".., "sha": <sha256 hex of text>, "text": str, "truth": str, "key_fields": {...}, "key_state": str, "key_reason": str, "tags": [str]}. All 300 texts unique.

Semantics:
- Each report has a hidden reality: caecum reached (bool), prep adequate (bool), tissue taken (bool: any polypectomy OR any biopsy), histology result back (bool, only meaningful if tissue taken).
- truth (from reality): "inadequate" if caecum not reached or prep inadequate; else "awaiting_histology" if tissue taken and no histology result; else "adequate". Aim for about 40% adequate, 30% awaiting_histology, 30% inadequate.
- key_fields (from what the TEXT supports, read as a careful clinician would): caecum yes/no/not_stated; prep adequate/inadequate/not_stated; polyps yes/no (= tissue taken this procedure); tissue_taken yes/no (same value as polyps); histology yes/no (a result for THIS procedure's tissue is present in the text).
- key_state / key_reason, applied in order to key_fields: caecum == "no" -> not_closed / incomplete_exam; prep == "inadequate" -> not_closed / inadequate_prep; caecum or prep "not_stated" -> needs_human / missing_documentation; tissue yes and histology no -> not_closed / awaiting_histology; else closed / criteria_met.
- Unstated items usually hide a fine procedure, but sometimes hide a failure (tag hidden:undocumented_failure).
- Boston Bowel Preparation Scale: adequate only if every segment >= 2 (a total >= 6 alone is not enough). Verbal "excellent/good/fair/adequate" = adequate; "poor/inadequate" = inadequate.

Requirements (minimums): >= 6 distinct layouts; UK and US spelling; >= 30 reports with Arabic text; >= 25 with scan/OCR noise; >= 40 ungraded prep; >= 25 unstated extent; >= 30 Boston scores in varied formats; >= 25 non-polyp biopsies; >= 20 negated tissue statements; >= 10 reports where a later addendum or correction changes a key field (either direction), keyed per the corrected statement; >= 10 references to PRIOR procedures (history, prior histology, prior incomplete exams) that must not be read as this procedure.
Then invent at least 10 further hard-case families of your own: realistic ways a careful clinician reads a report differently from a naive keyword reader. Use your own knowledge of real endoscopy documentation. Vary wording widely: do not reuse one fixed sentence per family. Aim for >= 35% of reports with at least one hard:* tag. Tag every report with fmt:*, spelling:*, prep:*, tissue:*, and a hard:* tag for each hard case it contains.

Before replying, check in your head that: key_state follows the rules from key_fields; truth follows the hidden reality; minimums are met; sha matches text; output is deterministic. In README, list your invented hard-case families with one line each, and any keying decisions a reviewer should check.
