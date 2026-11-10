You are blind labeller fresh4-labeller-b in a fresh isolated session. Read ONLY these two files:
1. `/Users/anasqumhiyeh/Documents/Codex/wasl-clean/.claude/worktrees/wf_3ae621fd-a2c-1/scripts/fresh2_eval/LABELLING-GUIDE.md`
2. `/Users/anasqumhiyeh/Documents/Codex/wasl-clean/.claude/worktrees/wf_3ae621fd-a2c-1/data/fresh4/labeller-input.json`
Do not read any other file. Do not use web, Bash, Write, Edit or any other tool. Do not inspect canonical corpus/provenance, author material, coverage mapping, reader code, caches, old or other labels, adjudication, results or scores. Label every item independently from its text and the guide. Return ONLY one complete valid JSON object, no Markdown and no commentary. Do not write a file.

Use exactly this top-level schema and metadata:
{
  "schema": "fresh2-labels/v1",
  "corpus_sha256": "5aa826689564db25cb8c9ade7bbf727b6d7240848241a2eaaa3b9f60c87852ce",
  "labeller": {
    "id": "fresh4-labeller-b",
    "kind": "model",
    "model": "opus",
    "model_family": "anthropic-claude",
    "prompt_sha256": "e5e28fbc0251574b9980ada97547fee946ac98d443b4d208e888e76ec1b1d97a",
    "run_date": "2026-11-09",
    "guide_sha256": "628a945665297dc91bb523a24ea07da7646554992a13bd1c5d0de7c5dc9e0197",
    "session_isolated": true,
    "saw_reader_implementation": false,
    "saw_other_labels": false,
    "clinician_verified": false
  },
  "labels": [...]
}
Return exactly 100 label rows in input order, one per item ID with no duplicate or omitted IDs. Each row has `id` and `label`; IFHAS-screen rows additionally have `coded_label`. Use exact guide spelling and composite domain order. No rationale, confidence or extra field. Declarations are binding. Do not balance labels or coordinate with anyone.
