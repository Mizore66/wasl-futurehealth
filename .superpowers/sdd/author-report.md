# Fresh2 LDCT/IFHAS corpus authoring report

Disclosure: model-authored synthetic text; no author key; not clinician-verified.

## Calls
- Author model: gpt-6.1-sol (served model reported as gpt-6.1-sol) via the existing LiteLLM key; text generation only.
- Uncached calls: 24 total (LDCT 12, IFHAS 12), 0 retries, cap 120 (enforced by an on-disk ledger that counts a call before it is sent). 5 items per call, all HTTP 200.
- Prompt version fresh2-author-prompt-v1, schema version fresh2-corpus-schema-v1. Shape briefs (routine, urgent, incomplete, follow-up, DNA, addendum, wrong-kind, trusted-link-needed, safety-trap, boundary values, unit errors, missing domains) lived only in prompts; item order was shuffled with a fixed seed; no label or key is stored. Prompt text, briefs and raw responses are kept outside the repo (not committed); the system-prompt hash is in each manifest.

## Files
- data/ldct/heldout2/corpus.json and AUTHOR-MANIFEST.json (60 items, ids f2l-001..060: 30 screening, 30 work-up)
- data/ifhas/heldout2/corpus.json and AUTHOR-MANIFEST.json (60 items, ids f2i-001..060: 30 screening panels, 30 follow-up notes)

## Schema contract for blind labellers
corpus.json: {source, disclosure, schema_version, reports: [ {id, text, metadata} ]}.
metadata (all items): programme (lung_ldct | ifhas), document_role (screening | downstream; the slot the document was filed under, not a statement that it is the right kind), document_title, author_role, facility_name, facility_type, index_date, document_date, registered_patient {name,dob,sex} (who the document is expected to attach to), patient_on_document {name,dob|null,sex|null,emirates_id|null} (how the document identifies the patient; differs from registered_patient on some items), details.
details by kind:
- LDCT screening: exam_type, technique, dose_dlp_mgy_cm, double_read, comparison_available, prior_exam_date.
- LDCT work-up: setting, encounter_type, referenced_screening_date.
- IFHAS screening: visit_type, fasting, printed_panel_in_text=true (the text is the printed panel) and coded_results [{loinc, display, value, unit (UCUM)}] (the coded screening input; may be empty, partial or disagree with the print).
- IFHAS follow-up: setting, encounter_type, referenced_screening_date.
Labellers should label from text (and coded_results for IFHAS screening) per the design docs, with an explicit option for wrong kind / unreadable / needs human. Metadata contains no category, status or expected label.

## Validation (script outside repo; all passed)
- 60 + 60 items, ids unique, texts unique within and across programmes (120 distinct), non-empty.
- All required metadata fields present, dates YYYY-MM-DD, facility_type in enum, IFHAS coded_results numeric with loinc and unit.
- No label-leaking words (wrong kind, safety trap, shape, expected label, ground truth), no local paths, URLs, emails, key-like strings in text or metadata.
- JSON parses; manifest hashes: corpus sha256, per-item text sha256, system-prompt and spec hashes.

## Concerns
- Not independently checked clinically; a model-written item may not realise its intended shape or may be ambiguous; that is for the blind labellers to expose.
- document_role reveals the filing slot (e.g. a mammogram filed as screening); this is intended input, not a label.
- Model drift toward stylised names/facilities; the author model saw the programme descriptions in the prompt (not reader code).
- No scoring or labelling was done; readers, thresholds, caches untouched.
