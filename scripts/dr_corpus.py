"""Synthetic diabetic retinopathy text corpus (dev set) for the third loop type. Deterministic (seed 19).

Writes data/dr/screen_reports.json (retinal screening grading reports) and data/dr/clinic_notes.json (eye-clinic
notes after a referral), each with a labelled key. Used to write reader A (regex) and the reader-B prompt; it is NOT
a fair test of either. The held-out set (data/dr/heldout/) is written by another model family and scored once.

Screening label (same rule as pipeline/drread.go drScreenLabel): worse eye R grade (3A > 3S > 2 > 1 > 0), M1 if
either eye has maculopathy, "/urgent" if the grader asks for an urgent referral. An ungradable eye makes the whole
screen U unless the other eye is referable on its own (R2+, R3S or M1), in which case that eye's grade stands.
Traps: per-eye wording (right/left, OD/OS, RE/LE), negation ("no new vessels", "no maculopathy"), a previous
screening result in the text (must be ignored), treated stable proliferative disease (R3S, not R3A), cataract or
small pupil (ungradable), grader-requested urgent referral on an R2 screen.

Clinic label: dna | not_assessable | not_examined | examined_treated | examined_followup | examined_discharged.
Traps: vision checked but no fundus exam (not_examined), negated treatment ("laser not required") = follow-up,
treatment booked rather than given = treated, discharge back to screening.
usage: python3 scripts/dr_corpus.py [out_dir]   (default data/dr)
"""
import json, os, random, sys

OUT = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), '..', 'data', 'dr')
os.makedirs(OUT, exist_ok=True)
r = random.Random(19)
RANK = {'0': 0, '1': 1, '2': 2, '3S': 3, '3A': 4}

R_TEXT = {
    '0': ['no diabetic retinopathy', 'no retinopathy detected', 'retina normal, no diabetic changes'],
    '1': ['background diabetic retinopathy (scattered microaneurysms)', 'mild non-proliferative diabetic retinopathy',
          'a few dot haemorrhages and microaneurysms (background retinopathy)'],
    '2': ['pre-proliferative retinopathy with venous beading', 'moderate non-proliferative diabetic retinopathy',
          'multiple blot haemorrhages and IRMA (pre-proliferative)', 'severe non-proliferative diabetic retinopathy'],
    '3A': ['active proliferative diabetic retinopathy with new vessels at the disc',
           'new vessels elsewhere with pre-retinal haemorrhage (active proliferative disease)',
           'proliferative retinopathy: NVE superotemporally, actively bleeding'],
    '3S': ['stable treated proliferative retinopathy: laser scars, regressed new vessels',
           'previous panretinal photocoagulation, no active new vessels, stable (R3S)',
           'treated proliferative disease, stable since laser'],
    'U': ['ungradable: dense cataract obscures the view', 'image quality inadequate (small pupil), ungradable',
          'unable to grade: media opacity'],
}
M_TEXT = {'0': ['no maculopathy', 'macula clear', 'no exudates near the fovea'],
          '1': ['maculopathy present (exudate within one disc diameter of the fovea)', 'macular oedema suspected (M1)',
                'group of exudates within the macula']}
EYE = [('Right eye', 'Left eye'), ('OD', 'OS'), ('RE', 'LE')]
NEG = ['No new vessels seen.', 'No pre-retinal haemorrhage.', '']


def referable(rg, mg):
    return rg in ('2', '3A', '3S') or mg == '1'


def screen_label(rr, rl, mr, ml, urgent):
    eyes = [(rr, mr), (rl, ml)]
    if any(g == 'U' for g, _ in eyes):
        ok = [(g, m) for g, m in eyes if g != 'U' and referable(g, m)]
        if not ok:
            return 'U'
        eyes = ok
    w = max((g for g, _ in eyes), key=lambda g: RANK[g])
    m = '1' if any(m == '1' for _, m in eyes) else '0'
    return 'R' + w + 'M' + m + ('/urgent' if urgent and w != '3A' else '')  # R3A is urgent anyway


def eye_text(g, m):
    t = r.choice(R_TEXT[g])
    if g == 'U':
        return t
    return t + '; ' + r.choice(M_TEXT[m])


screens = []
PLAN = ([('0', '0', '0', '0')] * 8 + [('1', '0', '0', '0'), ('0', '1', '0', '0'), ('1', '1', '0', '0')] * 2 +
        [('2', '1', '0', '0'), ('1', '2', '0', '0'), ('2', '2', '0', '1')] * 2 + [('0', '0', '1', '0'), ('1', '0', '0', '1')] +
        [('3A', '1', '0', '0'), ('2', '3A', '1', '0'), ('3A', '3A', '0', '0')] + [('3S', '1', '0', '0'), ('3S', '3S', '0', '0')] +
        [('U', 'U', '0', '0'), ('U', '0', '0', '0'), ('U', '2', '0', '0'), ('1', 'U', '0', '0'), ('U', '1', '0', '1')] +
        [('2', '0', '0', '0', 'urgent'), ('1', '2', '0', '1', 'urgent')])
for i, p in enumerate(PLAN):
    rr, rl, mr, ml = p[:4]
    urgent = len(p) > 4
    lab = screen_label(rr, rl, mr, ml, urgent)
    style = 'coded_summary' if i % 4 == 3 else 'prose'
    er, el = r.choice(EYE)
    if style == 'prose':
        parts = ['Diabetic eye screening. Two-field mydriatic photography, graded by primary and secondary graders.']
        if r.random() < .3:
            parts.append(f"Last screening: R{r.choice(['1', '2'])}M0 (for comparison only).")
        parts.append(f'{er}: {eye_text(rr, mr)}.')
        parts.append(f'{el}: {eye_text(rl, ml)}.')
        if rr in ('2', '1') and rl in ('2', '1', '0'):
            parts.append(r.choice(NEG))
        if urgent:
            parts.append('Grader requests urgent referral: rapid progression since the last screen.')
        text = ' '.join(x for x in parts if x)
    else:
        g = lambda x, m: 'U' if x == 'U' else f'R{x}M{m}'
        text = (f'Automated grading result (approved grading system, secondary grader confirmed). {er}: {g(rr, mr)}. '
                f'{el}: {g(rl, ml)}.' + (' Urgent referral requested.' if urgent else ''))
    screens.append({'id': f'drs{i + 1:02d}', 'text': text, 'key': {'r_right': rr, 'r_left': rl, 'm_right': mr, 'm_left': ml,
                    'urgent': urgent, 'label': lab, 'style': style, 'coded': lab}})

CLINIC = {
    'examined_treated': ['Dilated fundus examination: active proliferative diabetic retinopathy right eye. Panretinal photocoagulation given today.',
                         'DFE: centre-involving diabetic macular oedema left eye. First anti-VEGF injection given today; loading doses booked.',
                         'Fundoscopy after dilation shows new vessels at the disc. PRP laser booked for next week.',
                         'Dilated examination confirms clinically significant macular oedema. Intravitreal anti-VEGF commenced.'],
    'examined_followup': ['Dilated fundoscopy: moderate non-proliferative retinopathy, no macular oedema. No laser required at this stage. Review in eye clinic in 4 months.',
                          'DFE: severe NPDR both eyes, no new vessels. Treatment not indicated today; review in 3 months.',
                          'Fundus examined after dilation: maculopathy without centre involvement. Observe; OCT and review in 6 months.',
                          'Dilated fundus exam: stable treated PDR. Anti-VEGF not indicated. Follow-up in eye clinic in 6 months.'],
    'examined_discharged': ['Dilated fundus examination: mild background changes only, no maculopathy. Discharged back to diabetic eye screening.',
                            'DFE shows no sight-threatening disease. Return to annual retinal screening; discharged from eye clinic.',
                            'Fundoscopy with dilation: exudates outside the macula, no oedema on OCT. Discharged to the screening programme.'],
    'not_examined': ['Visual acuity 6/9 right, 6/12 left. Patient declined dilating drops (driving). Fundus not examined. Rebook for dilated exam.',
                     'Seen in eye clinic: visual acuity and intraocular pressure checked only. No fundus examination today; rebooked.',
                     'Refraction performed. Dilation deferred, retina not examined. New appointment to follow.'],
    'not_assessable': ['Dense cataract precludes any fundal view in either eye. Listed for cataract surgery; retina to be assessed after.',
                       'No view of the posterior pole through vitreous haemorrhage. Fundus not assessable; B-scan arranged.',
                       'Media opacity: fundus cannot be assessed. Refer for cataract assessment.'],
    'dna': ['Patient did not attend the eye clinic appointment. DNA letter sent to patient and GP.',
            'DNA. Second appointment to be offered.',
            'Did not attend today. Rebook as per DNA policy.'],
}
PLANC = (['examined_treated'] * 10 + ['examined_followup'] * 12 + ['examined_discharged'] * 8 + ['not_examined'] * 7 +
         ['not_assessable'] * 6 + ['dna'] * 7)
HDR = ['Ophthalmology clinic (diabetic retinopathy referral).', 'Medical retina clinic note.', 'Eye clinic, regional hospital.']
notes = []
for i, lab in enumerate(PLANC):
    base = r.choice(CLINIC[lab])
    text = r.choice(HDR) + ' ' + base
    if lab.startswith('examined') and r.random() < .4:
        text += ' Visual acuity 6/' + r.choice(['6', '9', '12', '18']) + ' both eyes.'
    out = lab.split('_', 1)[1] if lab.startswith('examined') else 'none'
    notes.append({'id': f'drc{i + 1:02d}', 'text': text, 'key': {'examined': lab.startswith('examined'), 'outcome': out, 'label': lab}})

src = 'synthetic templated corpus (scripts/dr_corpus.py, seed 19); dev set, used to write the readers'
json.dump({'source': src, 'reports': screens}, open(os.path.join(OUT, 'screen_reports.json'), 'w'), indent=1, ensure_ascii=False)
json.dump({'source': src, 'reports': notes}, open(os.path.join(OUT, 'clinic_notes.json'), 'w'), indent=1, ensure_ascii=False)
print(len(screens), 'screening reports,', len(notes), 'clinic notes ->', OUT)
