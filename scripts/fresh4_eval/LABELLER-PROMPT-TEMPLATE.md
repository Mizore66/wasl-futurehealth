You are blind labeller {LABELLER_ID} in a fresh isolated session. Read ONLY these two files:
1. `/Users/anasqumhiyeh/Documents/Codex/wasl-clean/.claude/worktrees/wf_3ae621fd-a2c-1/scripts/fresh2_eval/LABELLING-GUIDE.md`
2. `/Users/anasqumhiyeh/Documents/Codex/wasl-clean/.claude/worktrees/wf_3ae621fd-a2c-1/data/fresh4/labeller-input.json`
Do not read any other file. Do not use web, Bash, Write, Edit or any other tool. Do not inspect canonical corpus/provenance, author material, coverage mapping, reader code, caches, old or other labels, adjudication, results or scores. Label every item independently from its text and the guide. Return ONLY one complete valid JSON object, no Markdown and no commentary. Do not write a file.

Use exactly this top-level schema and metadata:
{
  "schema": "fresh2-labels/v1",
  "corpus_sha256": "{CORPUS_SHA256}",
  "labeller": {
    "id": "{LABELLER_ID}",
    "kind": "model",
    "model": "{MODEL}",
    "model_family": "{MODEL_FAMILY}",
    "prompt_sha256": "{PROMPT_SHA256}",
    "run_date": "2026-11-09",
    "guide_sha256": "{GUIDE_SHA256}",
    "session_isolated": true,
    "saw_reader_implementation": false,
    "saw_other_labels": false,
    "clinician_verified": false
  },
  "labels": [...]
}
Return exactly 100 label rows in input order, one per item ID with no duplicate or omitted IDs. Each row has `id` and `label`; IFHAS-screen rows additionally have `coded_label`. Use exact guide spelling and composite domain order. No rationale, confidence or extra field. Declarations are binding. Do not balance labels or coordinate with anyone.
