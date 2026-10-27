# Double-labelling the 43 report texts (not done yet)

The answer key in `data/evidence/reports43.json` was produced by an LLM labeller and cross-checked against the regex reader. **No human has labelled these reports yet.** This kit lets both team members label them independently.

1. Each of you runs `python docs/labelling/make_template.py <your_name>`. This writes a blind CSV with the report texts and empty label columns.
2. Each of you labels your own copy separately. Don't compare sheets, and don't look at the key.
3. Run `python docs/labelling/agreement.py docs/labelling/labels_<a>.csv docs/labelling/labels_<b>.csv`. It reports Cohen's κ per field (caecum, prep, polyps, histology, state) between you, and for each of you against the current key.
4. Settle the disagreements it lists together. Report κ, the n, and how many key changes the agreed sheet makes.

It takes about 1.5–2 hours per person: 43 short reports at roughly 2 minutes each.
