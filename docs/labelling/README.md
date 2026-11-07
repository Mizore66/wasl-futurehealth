# Clinically verified labels for the 43 report texts

**Status: complete.** Two independent model-assisted labelling passes were reconciled and clinically verified by the team's clinician. After adjudication of u9 and u41, Cohen's κ is **1.000 for caecum, prep, polyps, histology and state** across all 43 reports. This is **clinically verified model-assisted labelling, not independent human double-labelling**.

- `labels_Anas_completed.csv` and `labels_Kai_completed.csv` — frozen completed sheets.
- `agreement.json` — observed agreement and κ for all five scored clinical fields, plus comparison semantics for the free-text reason column.
- `adjudications.csv` — clinician's final caecum labels and reasons for u9 and u41.
- `../../data/evidence/reports43.json` — updated key with method metadata and u38's outcome changed from `adequate` to `inadequate`; other outcomes unchanged.

The verified reader evaluation has **0 unsafe automatic adequacy labels** after the `fair` preparation rule was corrected to `inadequate_prep`.

1. Each of you runs `python docs/labelling/make_template.py <your_name>`. This writes a blind CSV with the report texts and empty label columns.
2. Each of you labels your own copy separately. Don't compare sheets, and don't look at the key.
3. Run `python docs/labelling/agreement.py docs/labelling/labels_<a>.csv docs/labelling/labels_<b>.csv`. It reports Cohen's κ per field (caecum, prep, polyps, histology, state) between you, and for each of you against the current key.
4. Settle the disagreements it lists together. Report κ, the n, and how many key changes the agreed sheet makes.

It takes about 1.5–2 hours per person: 43 short reports at roughly 2 minutes each.
