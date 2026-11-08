"""Synthetic IFHAS follow-up note corpus (dev set) for the sixth loop type. Deterministic (seed 61).

Writes data/ifhas/followup_notes.json: notes from the family / internal medicine clinic that receives IFHAS referrals
for abnormal cardiometabolic results (DoH IFHAS standard V10/2025, Appendix 8), each with a labelled key. Used to write
reader A (regex) and the reader-B prompt; it is NOT a fair test of either (same author as the readers).

Label (same rule as pipeline/ifhasrule.go ifhasNoteLabel): "dna" if the person did not attend; otherwise
"<domain>=<status>;..." over the domains the note addresses, keys sorted, domains bp | glyc | lipid | weight, status
  managed       the abnormality is confirmed (or re-measured abnormal) AND a management plan is started
                (medicine, lifestyle / weight programme, dietitian, specialist referral)
  not_confirmed the repeat test or readings are normal or below the threshold: back to routine screening
  pending       repeat test ordered or awaited, readings still being collected, confirmed but no plan yet, or declined
Traps: last year's values, prediabetes on a repeat HbA1c ("diabetes not confirmed" + lifestyle advice = not_confirmed),
white-coat readings on home monitoring, negation ("no hypertension"), a confirmed diagnosis with the plan deferred,
a declined repeat, a non-fasting triglyceride repeated fasting, HbA1c in mmol/mol, a referral header naming a domain
the note never addresses (that domain is not mentioned).
usage: python3 scripts/ifhas_corpus.py [out_dir]   (default data/ifhas)
"""
import itertools, json, os, random, sys

OUT = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), '..', 'data', 'ifhas')
os.makedirs(OUT, exist_ok=True)
r = random.Random(61)
DOMS = ['bp', 'glyc', 'lipid', 'weight']
STATUS = ['managed', 'not_confirmed', 'pending']


def a1c(lo, hi):
    return f"{r.uniform(lo, hi):.1f}"


def mmolmol(pct):
    return round((float(pct) - 2.152) / 0.09148)


def glyc(st):
    a1 = a1c(6.5, 9.4)
    if st == 'managed':
        a2 = a1c(6.6, 9.8)
        return r.choice([
            f"Repeat HbA1c today {a2}% (IFHAS screening {a1}%), confirming type 2 diabetes. Started metformin 500 mg twice daily and referred to diabetes education.",
            f"Fasting plasma glucose {r.uniform(7.2, 11.5):.1f} mmol/L on repeat, consistent with diabetes alongside the screening HbA1c of {a1}%. Type 2 diabetes confirmed; enrolled in the diabetes clinic and a structured lifestyle programme.",
            f"HbA1c {a1c(10.2, 13.5)}% with a random glucose of {r.randint(305, 420)} mg/dL and thirst and polyuria for two months. Diabetes confirmed clinically; basal insulin commenced and endocrinology referral sent.",
            f"Second HbA1c {mmolmol(a2)} mmol/mol confirms diabetes. Management plan: metformin, dietitian appointment and review in 3 months.",
            f"Glycated haemoglobin repeated: {a2}%. Diagnosis of type 2 diabetes made; started on sitagliptin-metformin and given diet and exercise advice.",
            f"Last year's HbA1c was {a1c(5.4, 5.9)}%. Repeat HbA1c now {a2}%, so diabetes is confirmed; metformin started and foot and eye screening arranged.",
        ])
    if st == 'not_confirmed':
        a1, a2 = a1c(6.5, 7.0), a1c(5.7, 6.3)
        return r.choice([
            f"Repeat HbA1c {a2}% (screening {a1}%); diabetes not confirmed. Prediabetes range: lifestyle advice given and HbA1c again in 12 months.",
            f"Fasting glucose {r.uniform(4.6, 5.5):.1f} mmol/L on repeat, within normal limits. The raised random glucose at screening is not confirmed; back to routine IFHAS screening.",
            f"Second HbA1c {a1c(5.0, 5.6)}%, below the diagnostic threshold. No diabetes. Routine screening in 3 years.",
            f"HbA1c rechecked at {mmolmol(a2)} mmol/mol: does not meet the criteria for diabetes. Advised on diet and activity; repeat yearly.",
            f"Oral glucose tolerance test normal (2-hour glucose {r.uniform(5.5, 7.4):.1f} mmol/L). Diabetes ruled out.",
        ])
    return r.choice([
        f"Repeat HbA1c requested to confirm the screening result of {a1}%; results awaited.",
        f"Fasting plasma glucose ordered; we will review when the result is back.",
        f"HbA1c {a1}% at screening discussed. The patient declined a repeat blood test today and will think about it.",
        f"Repeat HbA1c {a1c(6.6, 8.5)}% confirms type 2 diabetes. Treatment options discussed; the plan will be agreed at the next visit.",
        f"Blood taken today for a second HbA1c; result not yet available.",
    ])


def bp(st):
    s, d = r.randint(142, 172), r.randint(88, 104)
    if st == 'managed':
        return r.choice([
            f"Clinic BP repeated: {s}/{d} and {s - r.randint(0, 6)}/{d - r.randint(0, 4)} mmHg. Home BP average {s - 8}/{d - 4}. Hypertension confirmed; started amlodipine 5 mg once daily with lifestyle advice, BP review in 4 weeks.",
            f"BP {r.randint(182, 200)}/{r.randint(108, 118)} mmHg again today, no headache or visual symptoms, fundi normal. Severe hypertension confirmed; amlodipine and perindopril started, recheck in one week.",
            f"Ambulatory blood pressure monitoring average {s - 10}/{d - 6} mmHg confirms stage 1 hypertension. Lifestyle programme started (salt reduction, exercise) and referred to the cardiology clinic.",
            f"Home BP diary averages {s - 5}/{d - 3}. Diagnosis: essential hypertension. Losartan 50 mg commenced.",
            f"Blood pressure remains raised at {s}/{d} on three readings. Hypertension diagnosed and treatment started with indapamide.",
        ])
    if st == 'not_confirmed':
        return r.choice([
            f"Home BP diary over 7 days averages {r.randint(118, 132)}/{r.randint(72, 82)} mmHg. White-coat effect; hypertension not confirmed. Recheck at the next routine screening.",
            f"Repeat clinic readings {r.randint(118, 134)}/{r.randint(70, 84)} and {r.randint(118, 134)}/{r.randint(70, 84)} mmHg, normal. No hypertension. Routine follow-up.",
            f"Ambulatory monitoring daytime average {r.randint(120, 133)}/{r.randint(74, 83)}: normal. The screening blood pressure was not confirmed.",
            f"BP today {r.randint(116, 132)}/{r.randint(70, 84)} after rest, and normal on the home monitor. Not hypertensive; no treatment needed.",
        ])
    return r.choice([
        f"Home BP monitor issued; the patient will return with a 7-day diary.",
        f"Ambulatory blood pressure monitoring booked for next week before deciding on treatment.",
        f"BP {s}/{d} today. Recheck in 2 weeks before any diagnosis is made.",
        f"Blood pressure discussed; readings to be repeated at the next visit.",
    ])


def lipid(st):
    if st == 'managed':
        return r.choice([
            f"Fasting lipid profile repeated: LDL {r.uniform(5.0, 6.6):.1f} mmol/L, triglycerides {r.uniform(1.4, 2.6):.1f} mmol/L. Severe hypercholesterolaemia confirmed; atorvastatin 40 mg started and familial hypercholesterolaemia assessment requested.",
            f"Fasting triglycerides {r.uniform(6.0, 9.5):.1f} mmol/L on repeat, confirming severe hypertriglyceridaemia. Fenofibrate and dietary advice started.",
            f"LDL cholesterol {r.randint(195, 250)} mg/dL confirmed on a fasting sample; rosuvastatin 20 mg commenced and referral to the lipid clinic.",
            f"Repeat LDL-C {r.uniform(4.9, 6.0):.1f} mmol/L. High-intensity statin therapy started.",
        ])
    if st == 'not_confirmed':
        return r.choice([
            f"Fasting lipid profile repeated: triglycerides {r.uniform(1.2, 2.4):.1f} mmol/L and LDL {r.uniform(2.6, 4.2):.1f} mmol/L. The raised non-fasting triglyceride at screening is not confirmed; routine screening.",
            f"Repeat LDL {r.uniform(3.1, 4.6):.1f} mmol/L, below the threshold for severe hypercholesterolaemia; no statin needed now.",
            f"Fasting lipids are normal on repeat (LDL {r.randint(100, 160)} mg/dL, TG {r.randint(90, 190)} mg/dL). Lipid abnormality not confirmed.",
        ])
    return r.choice([
        f"Fasting lipid profile ordered; result to follow.",
        f"The patient had eaten before the screening test; fasting lipids requested and a review booked.",
        f"Lipid results discussed; repeat fasting sample next week, then decide about a statin.",
        f"Statin discussed for the raised LDL but the patient wants to try diet first and decide later; repeat lipids in 3 months.",
    ])


def weight(st):
    bmi = f"{r.uniform(30.2, 41.0):.1f}"
    if st == 'managed':
        return r.choice([
            f"BMI {bmi} kg/m2 confirmed (weight {r.randint(85, 125)} kg). Obesity: referred to the dietitian and enrolled in the weight-management programme.",
            f"Obesity class II (BMI {bmi}). Weight-loss plan agreed; GLP-1 therapy started.",
            f"BMI {bmi}. Weight management: dietitian appointment booked and an exercise prescription given.",
            f"Weight {r.randint(90, 130)} kg, BMI {bmi}; obesity confirmed. Referred to the bariatric and weight-management clinic.",
        ])
    if st == 'not_confirmed':
        return r.choice([
            f"Re-measured weight {r.randint(70, 85)} kg and height {r.randint(170, 185)} cm: BMI {r.uniform(24.0, 29.4):.1f} kg/m2. The screening BMI was a measurement error; obesity not confirmed.",
            f"Height was entered wrongly at screening; corrected BMI {r.uniform(25.0, 29.5):.1f}, so not obese. No weight programme needed.",
        ])
    return r.choice([
        f"BMI {bmi} kg/m2 discussed; the patient is not ready to start a weight programme and will reconsider at the next visit.",
        f"Weight discussed; a plan will be decided after the blood results.",
        f"Referral to the dietitian offered for BMI {bmi}; the patient declined for now.",
    ])


GEN = {'bp': bp, 'glyc': glyc, 'lipid': lipid, 'weight': weight}
TOPIC = {'bp': 'raised blood pressure', 'glyc': 'raised HbA1c', 'lipid': 'raised cholesterol', 'weight': 'raised BMI'}
HEAD = ["IFHAS follow-up visit, family medicine clinic.", "Internal medicine clinic. Referral from IFHAS screening.",
        "Follow-up after the periodic adult screening (IFHAS).", "Family physician review of abnormal IFHAS results."]


def label(st):
    return ';'.join(f"{d}={st[d]}" for d in sorted(st))


docs = []


def add(text, st, attended='yes'):
    lab = 'dna' if attended == 'no' else label(st)
    docs.append({'id': f'if{len(docs) + 1:03d}', 'text': text,
                 'key': {'label': lab, 'attended': attended, 'status': st if attended == 'yes' else {}}})


def note(st, header_extra=None):
    parts = [r.choice(HEAD)]
    if header_extra:
        parts.append(f"Referred for {' and '.join(TOPIC[d] for d in header_extra)}.")
    ds = list(st)
    r.shuffle(ds)
    for d in ds:
        parts.append(GEN[d](st[d]))
    return ' '.join(parts)


# single-domain notes: every domain x status, several variants
for d in DOMS:
    for s in STATUS:
        for _ in range(7):
            add(note({d: s}), {d: s})
# referral header names a second domain the note never addresses (that domain stays not mentioned)
for d in DOMS:
    other = r.choice([x for x in DOMS if x != d])
    s = r.choice(STATUS)
    add(note({d: s}, header_extra=[d, other]), {d: s})
# two-domain notes: every pair x every status combination
for a, b in itertools.combinations(DOMS, 2):
    for sa, sb in itertools.product(STATUS, STATUS):
        for _ in range(2):
            add(note({a: sa, b: sb}), {a: sa, b: sb})
# three- and four-domain notes (common combinations)
for trio in itertools.combinations(DOMS, 3):
    for combo in [('managed',) * 3, ('pending',) * 3, ('managed', 'pending', 'managed'), ('not_confirmed', 'managed', 'managed')]:
        st = dict(zip(trio, combo))
        add(note(st), st)
for combo in [('managed',) * 4, ('pending', 'managed', 'managed', 'managed'), ('managed', 'not_confirmed', 'managed', 'pending')]:
    st = dict(zip(DOMS, combo))
    add(note(st), st)
# did not attend
DNA = ["Patient did not attend the IFHAS follow-up appointment for {t}. A second appointment has been offered.",
       "DNA. Booked for review of {t} after IFHAS screening; no-show, letter sent.",
       "Failed to attend the follow-up clinic for {t}. The coordinator will call to rebook.",
       "Did not attend today's appointment ({t}). Rebook requested."]
for _ in range(12):
    d = r.choice(DOMS)
    add(r.choice(HEAD) + ' ' + r.choice(DNA).format(t=TOPIC[d]), {}, attended='no')

json.dump({'corpus': 'ifhas follow-up notes (dev, synthetic, seed 61)', 'reports': docs},
          open(os.path.join(OUT, 'followup_notes.json'), 'w'), indent=1, ensure_ascii=False)
labs = {}
for x in docs:
    labs[x['key']['label']] = labs.get(x['key']['label'], 0) + 1
print(len(docs), 'notes,', len(labs), 'distinct labels')
