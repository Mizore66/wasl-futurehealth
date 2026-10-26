# Data

| File | What | Provenance |
|---|---|---|
| `persons3.json` | 64,394 person records used to build the synthetic cohorts. Each record has an Arabic name, Latin name, DOB, sex and origin | Names, DOB and sex come from **Wikidata (CC0)**, plus some LLM-generated Arabic transliterations (`ar_src: "llm"`). Missing DOBs are randomly generated. **The `eid` field is synthetic**: a random number in the Emirates ID format (784 + birth year + 7 random digits + Luhn check). It is *not* anyone's real Emirates ID. |
| `evidence/reports43.json` | 43 colonoscopy report texts with readers' outputs and an answer key | MTSamples (educational use, credited). The key is LLM-labelled and cross-checked with a regex reader. |
| `evidence/pathology_templates.json` | Pathology report templates for synthetic histology | Written by the team |

**No real patient data is in this repo.** Every patient, loop, referral and report linkage is simulated from these files.
