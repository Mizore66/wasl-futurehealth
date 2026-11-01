#!/usr/bin/env python3
"""Fresh held-out IFHAS follow-up-note corpus with explicit answer keys.

Hand-authored; not derived from scripts/ifhas_corpus.py and never scored before the corpus was frozen in git.
Standard library only; deterministic. Writes followup_notes.json into argv[1]
(default data/ifhas/heldout under the repo root).

Key: {"attended": "yes"|"no", "status": {domain: status}, "label": "..."} where domains are
bp (blood pressure), glyc (glycaemia), lipid, weight, and
  managed        the abnormality was confirmed at/for this episode AND a management plan was started or arranged
                 (medicine, lifestyle programme, dietitian/weight clinic, specialist referral)
  not_confirmed  the repeat/confirmatory measurement was normal or the abnormality was explained away; routine screening
  pending        a repeat/confirmatory test is ordered or awaited, or the abnormality is confirmed with no plan yet,
                 or the patient declined/deferred, or the domain is mentioned with no decision
A domain is keyed only if this note addresses it for the current episode. Past values, other people and
negated statements ("no history of diabetes") do not count. label: "dna" if she did not attend; else the keyed
domains sorted alphabetically as dom=status joined by ";"; "indeterminate" if nothing is addressed.
"""
import json, os, sys

R = []


def n(status, tags, text, attended="yes"):
    R.append((status, attended, tags, text))


# ---- single domain ----
n({"bp": "managed"}, ["normal"], "Primary care review following the periodic adult screening. Blood pressure on two clinic readings today was 152/96 and 150/94 mmHg, and her home diary averages 148/92. Essential hypertension is confirmed. Ramipril 2.5 mg started and she will be reviewed in a month.")
n({"bp": "managed"}, ["normal"], "Seen in the cardiometabolic clinic. 24-hour ambulatory monitoring gave a mean of 144/91 consistent with sustained hypertension. Referred to the hypertension clinic; DASH diet leaflet and a salt-reduction plan given.")
n({"bp": "not_confirmed"}, ["normal"], "Follow-up for the raised pressure noted at screening. Today 124/78 on the repeat, and ambulatory average 121/76. White coat effect only. No hypertension; continue routine screening.")
n({"bp": "pending"}, ["normal"], "Her screening blood pressure was high (158/98). Today it is 150/95. We have asked her to record home readings for seven days and return with the diary before deciding on treatment.")
n({"bp": "managed"}, ["boundary"], "Clinic BP 168/102 today, and 170/100 on repeat after resting. Stage 2 hypertension confirmed. Started on amlodipine 5 mg and lifestyle counselling today.")
n({"bp": "not_confirmed"}, ["negated"], "Review of an elevated screening blood pressure. Repeat measurements 118/74 and 120/76. There is no evidence of hypertension and no treatment is needed. Advised routine rescreening.")
n({"bp": "pending"}, ["distractor"], "Her father has hypertension and was started on tablets last year, which worried her. For her own raised BP, a repeat reading today is still 146/92; confirmation requires ambulatory monitoring, which has been requested but not yet done.")
n({"bp": "managed"}, ["distractor"], "Last year her BP was normal (122/80). Now two readings on this visit are 156/98 and 154/96 and hypertension is confirmed. Commenced on losartan 50 mg and referred to the dietitian for salt reduction.")
n({"glyc": "managed"}, ["normal"], "Diabetes clinic intake after the screening HbA1c of 7.4%. A repeat fasting glucose today is 8.1 mmol/L, which confirms type 2 diabetes. Metformin 500 mg twice daily started and enrolled in the structured diabetes education programme.")
n({"glyc": "managed"}, ["normal"], "HbA1c repeated: 6.9% (52 mmol/mol), diagnosis of type 2 diabetes confirmed on two results. Plan: lifestyle programme, dietitian review and metformin.")
n({"glyc": "not_confirmed"}, ["normal"], "Repeat fasting glucose is 5.2 mmol/L and HbA1c 5.4%, so the raised screening glucose was a non-fasting artefact. No diabetes. Routine screening interval.")
n({"glyc": "pending"}, ["normal"], "Screening HbA1c was 6.6%. A confirmatory fasting glucose has been ordered today; the result is not yet back. No treatment started.")
n({"glyc": "pending"}, ["boundary"], "HbA1c 6.1% at screening and today 6.2%: prediabetes range. We discussed risk, but she was asked to attend again in three months with a repeat HbA1c before any plan is made.")
n({"glyc": "managed"}, ["boundary"], "Prediabetes confirmed (HbA1c 6.0% twice). Referred to the diabetes prevention programme and given dietary advice; will be reviewed in six months.")
n({"glyc": "not_confirmed"}, ["negated"], "Repeat HbA1c 5.6%. She does not have diabetes and does not have prediabetes. Return to routine three-yearly screening.")
n({"glyc": "pending"}, ["declined"], "Confirmed random glucose of 13.2 mmol/L but she declined to start any medication or join the education programme today and wishes to think about it. Diabetes is likely; no plan agreed.")
n({"glyc": "managed"}, ["distractor"], "Her sister was diagnosed with diabetes in 2021. For this patient: HbA1c 7.8% repeated, diabetes confirmed. Started metformin, dietitian booked.")
n({"lipid": "managed"}, ["normal"], "Lipid review after screening. Fasting LDL cholesterol repeated at 4.6 mmol/L (178 mg/dL) with a strong family history. Dyslipidaemia confirmed; atorvastatin 20 mg prescribed along with dietary advice.")
n({"lipid": "not_confirmed"}, ["normal"], "Fasting lipid profile repeated: LDL 2.4 mmol/L, triglycerides 1.3 mmol/L. The earlier abnormal result was non-fasting. No lipid disorder; routine screening.")
n({"lipid": "pending"}, ["normal"], "Screening LDL was 4.1 mmol/L. Repeat fasting profile has been requested today; she will phone for the result. No treatment decision made.")
n({"lipid": "managed"}, ["boundary"], "Triglycerides 6.2 mmol/L confirmed on a fasting repeat (very high). Referred to lipid clinic and the dietitian; fibrate considered by the specialist.")
n({"lipid": "pending"}, ["distractor"], "She is already on atorvastatin from another clinic according to the patient, but there are no records. Current fasting LDL is 3.9 mmol/L; we are waiting for the pharmacy record and a repeat profile before changing anything.")
n({"lipid": "not_confirmed"}, ["negated"], "Repeat fasting lipids within normal limits. No hyperlipidaemia. There is no indication for a statin.")
n({"weight": "managed"}, ["normal"], "BMI 34.2 kg/m2 today (class I obesity). Referred to the weight management clinic and dietitian; exercise programme started.")
n({"weight": "managed"}, ["normal"], "Weight 98 kg, BMI 36. Obesity confirmed. Enrolled in the supervised lifestyle programme; orlistat discussed and started.")
n({"weight": "not_confirmed"}, ["normal"], "Re-measured at the clinic. The previously recorded weight was a transcription error: BMI is actually 24.8 kg/m2. Normal weight. No intervention needed.")
n({"weight": "pending"}, ["normal"], "BMI 31.5. She says she has recently gained weight after stopping smoking. Agreed to re-weigh in six weeks before offering a programme.")
n({"weight": "managed"}, ["distractor"], "Her husband is enrolled in the weight programme. The patient's BMI is 33 and she has agreed to join the same weight management group starting next week; dietitian appointment booked.")
n({"weight": "not_confirmed"}, ["negated"], "BMI 22.9. She is not overweight and does not need referral. Waist circumference within the normal range.")
# ---- two domains ----
n({"bp": "managed", "glyc": "managed"}, ["multi"], "Cardiometabolic clinic. Hypertension confirmed (clinic 158/96 and home average 150/94) and amlodipine started. Diabetes also confirmed with a repeat HbA1c of 7.1%; metformin started and referred for diabetes education.")
n({"bp": "not_confirmed", "glyc": "managed"}, ["multi"], "Follow-up of two abnormal screening results. Blood pressure repeat today 119/76, normal; no hypertension. HbA1c repeat 7.2%: diabetes confirmed, lifestyle programme and metformin started.")
n({"bp": "pending", "lipid": "managed"}, ["multi"], "Her pressure was 148/94 today: ambulatory monitoring ordered, result awaited. Fasting LDL confirmed 4.8 mmol/L, started rosuvastatin 10 mg.")
n({"glyc": "pending", "weight": "managed"}, ["multi"], "BMI 35 confirmed: referred to the weight clinic and the dietitian. For the raised glucose, a fasting repeat is to be arranged and has not yet been done.")
n({"lipid": "not_confirmed", "weight": "not_confirmed"}, ["multi", "negated"], "Both abnormal results were not borne out. LDL repeat 2.6 mmol/L (normal). BMI recalculated at 24 after correcting height. No dyslipidaemia, not overweight. Routine screening.")
n({"bp": "managed", "weight": "pending"}, ["multi"], "BP 162/100 and 160/98 on two readings: hypertension confirmed, perindopril started. BMI 32; discussed weight, she would like to try on her own for three months first and will return.")
n({"bp": "managed", "glyc": "pending", "lipid": "managed"}, ["multi"], "Hypertension confirmed, enalapril started. Dyslipidaemia confirmed (LDL 4.5), atorvastatin 20 mg started. HbA1c 6.7% is borderline - fasting glucose test ordered, result not yet received.")
# ---- DNA ----
n({}, ["dna"], "She did not attend her follow-up appointment for the abnormal screening results today. A reminder SMS was sent and the clinic will try to telephone.", attended="no")
n({}, ["dna", "distractor"], "No-show for the cardiometabolic review. For completeness, her blood pressure at screening was 160/100 and her HbA1c 7.0%, but nothing has been assessed or started because she did not come.", attended="no")
n({}, ["dna"], "DNA: patient failed to attend the dietitian and weight clinic booking. Letter sent.", attended="no")
# ---- indeterminate / nothing addressed ----
n({}, ["nothing"], "Patient attended the family medicine clinic for a repeat prescription of her asthma inhaler. Chest clear. Reminded about annual influenza vaccination.")
n({}, ["nothing"], "Referral letter acknowledged. The patient is on the waiting list for a first appointment. No clinical assessment was recorded today.")
n({}, ["nothing", "distractor"], "Telephone call from her daughter asking about the clinic opening hours and parking. The daughter has diabetes herself, but no information about the patient's own results was discussed.")
# ---- format variety ----
n({"bp": "managed"}, ["format"], "FOLLOW-UP | Domain: blood pressure | Repeat: 154/96 | Confirmed hypertension: yes | Plan: ACE inhibitor started, review 4 weeks.")
n({"glyc": "not_confirmed"}, ["format"], "FOLLOW-UP | Domain: glycaemia | Repeat HbA1c: 5.5% | Confirmed: no | Plan: none, routine screening.")
n({"lipid": "pending"}, ["format"], "FOLLOW-UP | Domain: lipids | Repeat: ordered | Result: awaited | Plan: to be decided.")
n({"weight": "managed"}, ["format"], "FOLLOW-UP | Domain: weight | BMI 37 | Plan: bariatric/weight clinic referral sent.")
n({"bp": "pending"}, ["boundary"], "BP in clinic 142/90. Hypertension is probable but she is to complete a week of home monitoring; no tablets yet.")
n({"bp": "managed"}, ["boundary"], "Confirmed hypertension on ambulatory monitoring. She prefers non-drug management so we agreed a supervised exercise and weight-loss programme with the practice nurse; referral sent today.")
n({"glyc": "managed"}, ["boundary"], "Confirmed type 2 diabetes. No tablets for now as HbA1c is only 6.6%; she is enrolled in the diabetes education programme and the dietitian will see her on Monday.")
n({"lipid": "not_confirmed"}, ["boundary"], "Repeat total cholesterol 5.1 mmol/L, LDL 2.9, within target for her risk level. The screening flag is not confirmed as a disorder. No treatment; rescreen in three years.")
n({"bp": "pending", "glyc": "pending"}, ["multi"], "Both blood pressure and glucose are raised again. Neither has been confirmed yet: ambulatory BP monitoring and a fasting glucose are both requested. Results awaited.")
n({"glyc": "managed", "lipid": "not_confirmed"}, ["multi", "negated"], "Diabetes confirmed (HbA1c 7.3%), metformin and education started. Lipids repeated and normal, no dyslipidaemia, no statin.")


def label(attended, status):
    if attended == "no":
        return "dna"
    if not status:
        return "indeterminate"
    return ";".join("%s=%s" % (d, status[d]) for d in sorted(status))


def dump(path, rows):
    out = {"source": "Held-out IFHAS follow-up notes; hand-authored, frozen in git before any reader was run; synthetic", "reports": []}
    for i, (st, att, tags, text) in enumerate(rows, 1):
        out["reports"].append({"id": "hf%02d" % i, "text": text,
                               "key": {"attended": att, "label": label(att, st), "status": st}, "tags": sorted(tags)})
    assert len({r["text"] for r in out["reports"]}) == len(rows)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(out, f, indent=1, ensure_ascii=False)
        f.write("\n")


if __name__ == "__main__":
    d = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "data", "ifhas", "heldout")
    os.makedirs(d, exist_ok=True)
    dump(os.path.join(d, "followup_notes.json"), R)
    print(len(R))
