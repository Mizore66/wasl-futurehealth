You are the independent disagreements-only adjudicator in a fresh isolated session. Read ONLY:
1. `/Users/anasqumhiyeh/Documents/Codex/wasl-clean/.claude/worktrees/wf_3ae621fd-a2c-1/scripts/fresh2_eval/LABELLING-GUIDE.md`
2. `/Users/anasqumhiyeh/Documents/Codex/wasl-clean/.claude/worktrees/wf_3ae621fd-a2c-1/data/fresh4/adjudication-input.json`
Do not read any other file. Do not use web, Bash, Write, Edit or other tools. The input contains every disagreement and only disagreements, with text/coded values and both blind labels. You must not inspect agreed items, canonical corpus/provenance, reader implementation, caches, reader outputs, scores, gates or integrated results.

For each item, choose `adjudicated` only when the frozen guide and text support a defensible key; otherwise choose `unresolved`. You may select a guide-valid label different from both labellers when the text requires it. Record a concise item-specific rationale quoting or identifying the decisive text and an honest `low`, `moderate`, or `high` uncertainty. Never guess to eliminate disagreement.

Return ONLY one complete valid JSON object, no Markdown or commentary, and do not write a file:
{
  "schema": "fresh4-adjudication/v1",
  "corpus_sha256": "5aa826689564db25cb8c9ade7bbf727b6d7240848241a2eaaa3b9f60c87852ce",
  "adjudicator": {
    "id": "fresh4-adjudicator-agent",
    "kind": "model",
    "model": "sonnet",
    "model_family": "anthropic-claude",
    "prompt_sha256": "598d93c6ed655a97b77df8900b8fabb0a111b1f19bf541f88a4223d879fe651d",
    "run_date": "2026-11-09",
    "session_isolated": true,
    "saw_reader_outputs": false,
    "saw_scores": false,
    "clinician_verified": false
  },
  "items": [
    {"id": "...", "resolution": "adjudicated", "key": {"label": "...", "coded_label": "only for ifhas-screen"}, "rationale": "...", "uncertainty": "low"},
    {"id": "...", "resolution": "unresolved", "key": null, "rationale": "...", "uncertainty": "high"}
  ]
}
Return exactly one row for every input disagreement, in input order, with no agreed IDs and no extras. For adjudicated non-screen items the key contains only `label`; for adjudicated ifhas-screen items it contains `label` and `coded_label`. Declarations are binding.
