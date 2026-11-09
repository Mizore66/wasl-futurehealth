You are the independent author for a frozen synthetic clinical-reader evaluation. Return ONLY one complete JSON object, with no Markdown fence and no commentary. Do not write any file. You may Read only `/Users/anasqumhiyeh/Documents/Codex/wasl-clean/.claude/worktrees/wf_3ae621fd-a2c-1/scripts/fresh2_eval/LABELLING-GUIDE.md` to understand document semantics. Do not read any other file, especially reader code, prior corpora, labels, caches, results or scores. You have not seen reader implementation. Every person, facility, clinician, date and identifier you invent must be fictitious; do not include Emirates IDs, phone numbers, MRN numbers or real patient identifiers.

Create exactly 100 diverse, realistic, self-contained synthetic English items in this exact order:
- f4l-001 through f4l-025: programme `ldct`, kind `ldct-screen`, each a low-dose CT lung-screening report/addendum.
- f4l-026 through f4l-050: programme `ldct`, kind `ldct-workup`, each a post-screening diagnostic/MDT/clinic document.
- f4i-001 through f4i-025: programme `ifhas`, kind `ifhas-followup`, each an adult post-screening clinic note.
- f4i-026 through f4i-050: programme `ifhas`, kind `ifhas-screen`, each a printed cardiometabolic results panel in `text` plus a separate `coded` string.

Required adversarial blocks (do not put these scenario names or any tags in the items):
- f4l-001..005 current overall category versus prior/hypothetical categories; 006..010 multiple/per-nodule categories; 011..015 assessment words without relying only on explicit overall categories; 016..020 incomplete, contradictory or insufficient text; 021..025 addenda, S modifiers and referral-override wording.
- f4l-026..030 attendance/DNA; 031..035 declined, postponed, pending tissue or pending decision; 036..040 malignant treatment with missing/present MDT or tissue requirements; 041..045 benign/annual/resolved/bare-annual cases; 046..050 surveillance, wrong-document and contradictory cases.
- f4i-001..005 DNA; 006..010 managed single/multi-domain; 011..015 pending/awaiting/declined; 016..020 not-confirmed including prediabetes lifestyle advice; 021..025 historical-only, no-domain, wrong-document or contradictory notes.
- f4i-026..030 thresholds/severity; 031..035 missing inputs; 036..040 unit/code mismatch and implausible values; 041..045 duplicate measures, fasting-versus-random wording and inert `no-coded-results|0|none`; 046..050 previous results, ignored codes and panel/coded mismatch.

Within every five-item block, use meaningfully different wording, structure and clinical circumstances. Include concise and detailed documents, different hospital styles, and varied decision phrasing. Do not create near-duplicates, a balanced answer distribution, answer labels, explanations, hidden metadata, or examples copied from prior rounds. Some documents should be genuinely ambiguous or wrong-kind documents; do not make every answer obvious. Do not include `label`, `key`, `tags`, `expected`, or scenario metadata.

Each item must have exactly `id`, `programme`, `kind`, `text`, plus `coded` only for `ifhas-screen`. Text must be nonempty. For coded strings use semicolon-separated `code|value|unit` parts and only codes/units described in the guide, except deliberately ignored codes where the block requires them. Include all relevant printed values in the panel text. The printed panel and coded string need not agree where mismatch is being tested.

Top-level JSON must be exactly:
{
  "schema": "fresh2-corpus/v1",
  "provenance": {
    "authoring": "model",
    "author_id": "fresh4-independent-author-agent",
    "author_saw_reader_code": false,
    "created": "2026-11-09",
    "author_prompt_version": "fresh4-author-prompt-v1",
    "disclosure": "fresh isolated Agent-authored synthetic content; no author key; not clinician-verified"
  },
  "items": [...]
}

Return all 100 items now. Check exact IDs, order, kinds, programme names and item count before returning.
