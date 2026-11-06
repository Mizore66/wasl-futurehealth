# Fresh3 LDCT/IFHAS corpus authoring report

Disclosure: model-authored synthetic text; no author key; not clinician-verified.

## Calls
- Author model: gpt-6.1-sol (served model reported as gpt-6.1-sol) via the existing LiteLLM key (OpenAI-compatible chat completions); text generation only. No other model was used or substituted.
- Uncached calls: 20 total (LDCT 10, IFHAS 10), 0 retries, cap 100 (an on-disk ledger counted each call before it was sent). 5 items per call; every call succeeded first time. Tokens: 16,539 prompt, 56,758 completion.
- Prompt version fresh3-author-prompt-v1, schema version fresh3-corpus-schema-v1. Scenario briefs lived only in prompts; they are not stored in the corpus or manifests; item order was shuffled per programme with a fixed seed. No label or key was authored. Prompts, briefs and raw responses are kept outside the repo; the system-prompt hash and a hash of the author script plus briefs are in each manifest.

## Files
- data/fresh3/author/ldct/corpus.json and AUTHOR-MANIFEST.json: 50 items, ids f3l-001..050 (25 screening, 25 downstream/work-up)
- data/fresh3/author/ifhas/corpus.json and AUTHOR-MANIFEST.json: 50 items, ids f3i-001..050 (25 screening panels, 25 follow-up notes)

## Coverage (by brief, not stored in data)
Routine, urgent, incomplete, follow-up, DNA, addendum, wrong-kind, trusted-link-needed and safety-trap shapes in both programmes, plus quote-grounding edge families (broken or checkbox category text, split lines, off-domain mentions), pending families and work-up edge families (inadequate biopsy, PET-only, MDT-only, pathology-only). IFHAS screening carries printed panels plus coded results, including empty, partial, disagreeing and unit-mismatched coded sets and previous-year columns. LDCT covers baseline/annual/follow-up/addendum exams and thoracic clinic, MDT, pathology, imaging, telephone and letter work-up documents.

## Schema contract for blind labellers
corpus.json: {source, disclosure, schema_version, reports: [ {id, text, metadata} ]}.
metadata (all items): programme (lung_ldct | ifhas), document_role (screening | downstream; the slot the document was filed under, not a statement that it is the right kind), document_title, author_role, facility_name, facility_type (clinic|hospital|laboratory|screening_centre|other), index_date, document_date (YYYY-MM-DD), registered_patient {name,dob,sex} (who the document is expected to attach to), patient_on_document {name,dob|null,sex|null,emirates_id|null} (how the document identifies the patient; differs from registered_patient on some items), details.
details by kind:
- LDCT screening: exam_type, technique, dose_dlp_mgy_cm, double_read, comparison_available, prior_exam_date.
- LDCT work-up: setting, encounter_type, referenced_screening_date.
- IFHAS screening: visit_type, fasting, printed_panel_in_text=true (the text is the printed panel), coded_results [{loinc, display, value, unit (UCUM)}] (coded screening input; may be empty, partial or differ from the print).
- IFHAS follow-up: setting, encounter_type, referenced_screening_date.
Label from text (and coded_results for IFHAS screening) per docs/research/ldct-loop-design.md and ifhas-loop-design.md, with explicit options for wrong kind / unreadable / needs human. Metadata contains no category, status or expected label.

## Validation (script outside repo; all passed, 0 problems)
- 50 + 50 items; ids unique; texts unique within and across programmes (100 distinct); non-empty.
- Required metadata and per-kind details present; dates YYYY-MM-DD and document_date >= index_date (latest 2026-10-15); facility_type in enum; registered_patient complete; IFHAS coded_results numeric with loinc and unit.
- No label-leaking words (wrong kind, safety trap, shape, scenario, synthetic, filed as, trusted link, quote grounding, expected label, ground truth), no local paths, URLs, emails or key-like strings in text or metadata. No title matched a category/status word regex.
- JSON parses; manifests hold corpus sha256 and per-item text sha256.

## Concerns
- Not clinically checked; a model-written item may not realise its intended shape or may be ambiguous; blind labellers will expose this. The author model saw programme descriptions in the prompt, not reader code or thresholds.
- Identity-variant items are few (4 in LDCT, 2 in IFHAS differ in name or dob from the registered patient) and only 3 IFHAS screenings have empty coded_results; per-family counts are small.
- document_role reveals the filing slot; intended input, not a label.
- Model drift toward stylised UAE names and facility names.
- No scoring or labelling done; readers, thresholds, caches, fresh2 labels and evaluation code untouched.
