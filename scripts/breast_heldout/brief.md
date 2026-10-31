Write a deterministic Python 3 generator (standard library only; no network, no LLM calls, no clock, no file reads) of synthetic breast screening texts from Abu Dhabi: 40 screening mammogram reports from breast screening units and 50 documents from a diagnostic breast assessment unit or breast multidisciplinary team (MDT) about women recalled from screening.

Reply with exactly two code blocks:
1. ```python : the complete gen_breast_heldout.py. Random seed {SEED}. Running `python3 gen_breast_heldout.py` writes screen_reports.json and assess_notes.json in the same directory as the script. Running it twice must give byte-identical files.
2. ```markdown : README.md, written as a dataset card: how it works, counts per label and per tag, keying decisions. Name the author only as "independent author ({AUTHOR})". No absolute paths.

JSON format of each file: {"source": "Independent author ({AUTHOR}) - deterministic stdlib generator gen_breast_heldout.py, random seed {SEED}, synthetic breast screening texts, 2026-11-01", "reports": [...]}
Each report: {"id": str, "text": str, "key": {...}, "tags": [str]}. Ids: "hb01".. for screening, "ha01".. for assessment documents. All texts unique.

Screening reports (screen_reports.json). The final assessment uses the ACR BI-RADS 5th edition categories, per breast:
- "0" incomplete, needs additional imaging evaluation (or prior films); "1" negative; "2" benign finding; "3" probably benign; "4" suspicious (4A, 4B, 4C are all "4"); "5" highly suggestive of malignancy; "TR" that breast's images are technically inadequate (positioning, motion, pectoral muscle not shown) and no category is given for it. Do not use category 6.
- key: {"b_right", "b_left", "overall": the category the text states as the overall or final assessment, or null if the text states none, "label", "coded": a CPT II code or null}.
- label (apply exactly): if a breast is "TR": if the other breast is "0", "3", "4" or "5", label = "B" + that category, else label = "TR". Otherwise label = "B" + the more worrying category in the ACR order 1 < 2 < 3 < 0 < 4 < 5 (so "0" outranks "3"). If "overall" is not null it must equal the label's category.
- About 50% of reports come from units that send a coded final assessment: then "coded" = the CPT II code of the label (B0 3340F, B1 3341F, B2 3342F, B3 3343F, B4 3344F, B5 3345F) and the text is a short synoptic summary; otherwise "coded" = null and the text is a radiologist's free-text report (it may or may not state numeric categories). A "TR" label is never coded.
- Aim for roughly: B1 25%, B2 20%, B0 20%, B3 5%, B4 15%, B5 7.5%, TR 7.5%.

Assessment / MDT documents (assess_notes.json):
- key: {"attended": bool, "assessment": "done" | "not_done", "biopsy": "malignant" | "benign" | "pending" | "none", "outcome": "routine" | "early_recall" | "treatment" | "pending", "label"}.
- Field meanings: assessment "done" = any further imaging, ultrasound, clinical examination or biopsy was performed in this episode; "not_done" = she attended but no assessment took place. biopsy = this episode's needle/core biopsy result ("pending" = taken, result awaited; "none" = no biopsy result reported). outcome = the decision: "routine" = return to routine screening; "early_recall" = short-interval follow-up imaging (typically 6 months); "treatment" = referred for cancer treatment (surgery, oncology); "pending" = no decision yet.
- label (apply exactly): not attended -> "dna"; else assessment "not_done", biopsy "pending" or outcome "pending" -> "incomplete"; else outcome "treatment" with biopsy "malignant" -> "to_treatment"; outcome "routine" with biopsy not "malignant" -> "return_routine"; outcome "early_recall" with biopsy not "malignant" -> "early_recall". Do not write any other combination (e.g. treatment without a malignant biopsy, or routine with a malignant biopsy).
- Aim for roughly: return_routine 25%, early_recall 10%, to_treatment 20%, incomplete 30%, dna 15%.

Requirements (minimums): >= 5 distinct layouts per file; UK and US spelling; >= 8 texts with Arabic phrases (e.g. a bilingual header); >= 6 with scan/OCR noise; per-breast wording varied (right/left, RT/LT, R/L, Arabic breast names in a few); >= 6 screening reports that mention a previous screen whose category differs from today's; >= 6 that negate a finding ("no suspicious mass", "no malignant calcification"); >= 4 with 4A/4B/4C; >= 4 assessment documents that mention a biopsy from an earlier year; >= 4 where a biopsy was taken and the result is awaited; >= 3 where the woman declined or could not have the assessment.
Then invent at least 8 further hard-case families of your own: realistic ways a careful breast radiologist or breast clinician reads these texts differently from a naive keyword reader. Use your own knowledge of real breast screening and assessment documentation. Tag each hard report "hard:<family>"; at least 30% of each file must carry a hard:* tag.

Before replying, check in your head that: every label follows the rules from its key fields; minimums are met; output is deterministic. In README, list your hard-case families with one example each.
