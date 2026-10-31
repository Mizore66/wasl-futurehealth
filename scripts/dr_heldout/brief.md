Write a deterministic Python 3 generator (standard library only; no network, no LLM calls, no clock, no file reads) of synthetic diabetic retinopathy texts from Abu Dhabi primary-care diabetic eye screening and hospital eye clinics: 40 screening grading reports and 50 eye-clinic notes written after a referral from screening.

Reply with exactly two code blocks:
1. ```python : the complete gen_dr_heldout.py. Random seed {SEED}. Running `python3 gen_dr_heldout.py` writes screen_reports.json and clinic_notes.json in the same directory as the script. Running it twice must give byte-identical files.
2. ```markdown : README.md, written as a dataset card: how it works, counts per label and per tag, keying decisions. Name the author only as "independent author ({AUTHOR})". No absolute paths.

JSON format of each file: {"source": "Independent author ({AUTHOR}) - deterministic stdlib generator gen_dr_heldout.py, random seed {SEED}, synthetic diabetic retinopathy texts, 2026-10-31", "reports": [...]}
Each report: {"id": str, "text": str, "key": {...}, "tags": [str]}. Ids: "hs01".. for screening, "hc01".. for clinic notes. All texts unique.

Screening reports (screen_reports.json). Grading follows the English NHS diabetic eye screening scheme, per eye:
- R grade: "0" none; "1" background (microaneurysms, dot/blot haemorrhages, a few exudates away from the macula); "2" pre-proliferative (venous beading or loops, IRMA, multiple deep/blot haemorrhages); "3A" active proliferative (new vessels at the disc or elsewhere, pre-retinal or vitreous haemorrhage, tractional detachment); "3S" stable treated proliferative disease (laser scars, regressed or fibrosed vessels, no active new vessels); "U" that eye could not be graded.
- M grade: "1" maculopathy (exudate within 1 disc diameter of the fovea, a group of exudates within the macula, retinal thickening, or any microaneurysm/haemorrhage within 1 disc diameter of the fovea with visual acuity 6/12 or worse); "0" otherwise. For a "U" eye use "".
- key: {"r_right", "r_left", "m_right", "m_left", "urgent": bool (the grader explicitly requests an urgent referral), "label", "coded": label or null}.
- label (apply exactly): if an eye is "U": if the other eye is referable on its own (R "2", "3A", "3S" or M "1") grade from that eye alone, else label = "U". Otherwise R = the worse eye (3A > 3S > 2 > 1 > 0), M = "1" if either eye is M1, label = "R" + R + "M" + M, plus "/urgent" if urgent and R is not "3A".
- About 40% of reports come from sites that send a coded grade: then "coded" = label and the text is a short structured summary that states the per-eye grades; otherwise "coded" = null and the text is a grader's free-text report that describes the findings (it may or may not use codes).
- Aim for roughly: R0M0 30%, R1M0 20%, referable (R2, R3A, R3S or any M1) 35%, U 10%, urgent 5%.

Eye-clinic notes (clinic_notes.json):
- key: {"attended": bool, "fundus": "examined" | "not_examined" | "no_view" | null, "outcome": "treated" | "followup" | "discharged" | "none", "label"}.
- label (apply exactly): not attended -> "dna"; fundus "no_view" (the retina could not be seen, e.g. dense cataract, vitreous haemorrhage) -> "not_assessable"; fundus "not_examined" (the patient was seen but the retina was not examined, e.g. only vision or pressure checked, dilation not done) -> "not_examined"; fundus "examined" -> "examined_" + outcome, where outcome is "treated" (laser, PRP or an anti-VEGF or steroid injection given, started or booked), "discharged" (returned to the screening programme), otherwise "followup" (review, observation, imaging, no treatment needed now).
- Aim for roughly: examined 60% (treated / followup / discharged in similar shares), not_examined 15%, not_assessable 10%, dna 15%.

Requirements (minimums): >= 5 distinct layouts per file; UK and US spelling; >= 8 texts with Arabic phrases (e.g. a bilingual header); >= 6 with scan/OCR noise; per-eye wording varied (right/left, OD/OS, RE/LE, Arabic eye names in a few); >= 6 screening reports that mention a previous screening result that differs from today's; >= 6 that negate a finding ("no new vessels", "no maculopathy"); >= 4 clinic notes whose treatment is negated ("laser not needed"); >= 4 with vision recorded but no retinal exam.
Then invent at least 8 further hard-case families of your own: realistic ways a careful clinician reads these texts differently from a naive keyword reader. Use your own knowledge of real retinal screening and medical-retina documentation. Tag each hard report "hard:<family>"; at least 30% of each file must carry a hard:* tag.

Before replying, check in your head that: every label follows the rules from its key fields; minimums are met; output is deterministic. In README, list your hard-case families with one example each.
