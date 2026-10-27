"""Synthetic CT report corpus for the incidental lung-nodule loop (Fleischner 2017). Deterministic (seed 17).

Writes data/lung/finding_reports.json (index CT with an incidental nodule) and data/lung/followup_reports.json
(surveillance CT), each with a labelled key. Texts are templated with phrasing, unit (mm/cm) and section-order
variants; a few deliberate traps test the readers:
  - "6 x 5 mm" (mean 5.5 rounds to 6 -> the 6-8 mm band),
  - follow-up worded "stable" whose measurements grew > 1.5 mm (Lung-RADS growth -> growing),
  - follow-up worded "slightly larger" with a <= 1.5 mm change (not growth -> stable),
  - part-solid nodule whose solid component reached >= 6 mm (highly suspicious -> escalate),
  - motion-degraded study (not assessable -> no closure).
The key outcome uses the same rule as the readers (lung.go lungOutcome): not assessable > resolved > solid component
>= 6 mm > mean-diameter growth > 1.5 mm > stable.
usage: python3 scripts/lung_corpus.py [out_dir]   (default data/lung)
"""
import json, os, random, sys

OUT = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), '..', 'data', 'lung')
os.makedirs(OUT, exist_ok=True)
r = random.Random(17)
LOBES = ['right upper lobe', 'right middle lobe', 'right lower lobe', 'left upper lobe', 'lingula', 'left lower lobe']
INDIC = ['Abdominal pain; CT abdomen and pelvis including the lung bases.', 'Chest pain; CT pulmonary angiogram to exclude pulmonary embolism.',
         'Blunt trauma; CT chest to exclude injury.', 'Preoperative assessment before elective surgery.',
         'Persistent cough after a respiratory infection.', 'Renal colic; CT urogram with lung bases.']
DESC = {'solid': ['solid pulmonary nodule', 'solid noncalcified nodule', 'noncalcified solid nodule'],
        'ground_glass': ['pure ground-glass nodule', 'nonsolid (ground-glass) nodule', 'ground-glass opacity nodule'],
        'part_solid': ['part-solid nodule', 'part-solid (mixed) nodule', 'subsolid nodule with a solid component']}


def dims(l, s, unit):
    if unit == 'cm':
        return f'{l / 10:.1f} x {s / 10:.1f} cm'
    if s is None:
        return f'{l} mm'
    return f'{l} x {s} mm'


def mean_mm(l, s):
    s = l if s is None else s
    return int((l + s) / 2 + 0.5)  # half up (all sizes positive) = Fleischner "rounded to the nearest mm"


# ---------- index CT: incidental nodule ----------
FIND = [  # type, long, short (None = one dimension reported), count, solid component mm, unit
    ('solid', 3, 2, 'single', 0, 'mm'), ('solid', 4, 3, 'single', 0, 'mm'), ('solid', 5, 4, 'single', 0, 'mm'),
    ('solid', 4, None, 'single', 0, 'mm'), ('solid', 6, 5, 'single', 0, 'mm'), ('solid', 7, 6, 'single', 0, 'mm'),
    ('solid', 8, 7, 'single', 0, 'mm'), ('solid', 7, 7, 'single', 0, 'cm'), ('solid', 9, 8, 'single', 0, 'mm'),
    ('solid', 12, 10, 'single', 0, 'mm'), ('solid', 16, 14, 'single', 0, 'cm'), ('solid', 11, None, 'single', 0, 'mm'),
    ('solid', 3, 3, 'multiple', 0, 'mm'), ('solid', 5, 4, 'multiple', 0, 'mm'), ('solid', 6, 6, 'multiple', 0, 'mm'),
    ('solid', 8, 6, 'multiple', 0, 'mm'), ('solid', 10, 9, 'multiple', 0, 'mm'), ('solid', 13, 11, 'multiple', 0, 'cm'),
    ('ground_glass', 4, 3, 'single', 0, 'mm'), ('ground_glass', 5, 5, 'single', 0, 'mm'), ('ground_glass', 7, 6, 'single', 0, 'mm'),
    ('ground_glass', 12, 9, 'single', 0, 'mm'), ('ground_glass', 18, 15, 'single', 0, 'cm'), ('ground_glass', 4, 4, 'multiple', 0, 'mm'),
    ('ground_glass', 9, 7, 'multiple', 0, 'mm'), ('part_solid', 5, 4, 'single', 2, 'mm'), ('part_solid', 9, 7, 'single', 3, 'mm'),
    ('part_solid', 14, 11, 'single', 4, 'mm'), ('part_solid', 11, 9, 'single', 5, 'cm'), ('part_solid', 5, 5, 'multiple', 2, 'mm'),
    ('part_solid', 10, 8, 'multiple', 3, 'mm'), ('solid', 5, 5, 'single', 0, 'mm'), ('solid', 6, 4, 'single', 0, 'mm'),
    ('solid', 7, 5, 'multiple', 0, 'mm'), ('ground_glass', 6, 5, 'single', 0, 'mm'), ('part_solid', 7, 6, 'single', 2, 'mm'),
    ('solid', 9, 9, 'multiple', 0, 'mm'), ('ground_glass', 5, 4, 'multiple', 0, 'mm'),
]
COUNT_S = {'single': ['No other pulmonary nodules are identified.', 'This is the only pulmonary nodule.', 'The lungs are otherwise clear.'],
           'multiple': ['Several additional smaller nodules are present in both lungs; the dominant nodule is described.',
                        'Multiple bilateral pulmonary nodules, the largest as described.', 'At least three further nodules are scattered in both lungs, all smaller.']}
finds = []
for i, (typ, l, s, cnt, solid, unit) in enumerate(FIND):
    lobe, d = r.choice(LOBES), r.choice(DESC[typ])
    nod = f'{d} in the {lobe} measuring {dims(l, s, unit)}'
    if typ == 'part_solid':
        nod += f', with a solid component of {solid} mm'
    order = r.random()
    find = f'There is a {nod}. {r.choice(COUNT_S[cnt])}'
    other = r.choice(['No mediastinal, hilar or axillary lymphadenopathy.', 'Heart size normal. No pleural or pericardial effusion.',
                      'No consolidation. Mild dependent atelectasis at both bases.'])
    findings = f'{find} {other}' if order < .6 else f'{other} {find}'
    imp = r.choice([f'Incidental {d} in the {lobe}.', f'Incidental pulmonary nodule ({dims(l, s, unit)}), {lobe}.',
                    'Incidental pulmonary nodule as described; follow-up per Fleischner Society 2017 guidance.'])
    if cnt == 'multiple':
        imp = imp.replace('Incidental pulmonary nodule', 'Incidental pulmonary nodules').replace('Incidental ' + d, 'Multiple incidental nodules, dominant ' + d)
    text = f"CT CHEST\nCLINICAL INDICATION: {r.choice(INDIC)}\nTECHNIQUE: Axial CT, 1.25 mm reconstructions.\nFINDINGS: {findings}\nIMPRESSION: {imp}"
    finds.append({'id': f'lf{i + 1:02d}', 'text': text,
                  'key': {'type': typ, 'long_mm': l, 'short_mm': s if s is not None else l, 'count': cnt, 'solid_mm': solid, 'mean_mm': mean_mm(l, s)}})

# ---------- surveillance CT: outcome ----------
def outcome(k):
    if k['not_assessable']:
        return 'not_assessable'
    if k['resolved']:
        return 'resolved'
    if k['solid_mm'] >= 6:
        return 'suspicious_solid'
    pm = (k['prior_long'] + k['prior_short']) / 2
    cm = (k['cur_long'] + k['cur_short']) / 2
    return 'growing' if cm - pm > 1.5 else 'stable'


FU = [  # type, prior (l,s), current (l,s), solid component now, variant
    ('solid', (6, 5), (6, 5), 0, 'unchanged'), ('solid', (7, 6), (7, 6), 0, 'stable'), ('solid', (8, 7), (8, 7), 0, 'unchanged'),
    ('solid', (5, 4), (5, 4), 0, 'stable'), ('solid', (7, 6), (8, 6), 0, 'slightly_larger'), ('solid', (6, 6), (7, 6), 0, 'slightly_larger'),
    ('solid', (12, 10), (12, 10), 0, 'unchanged'), ('solid', (9, 8), (9, 8), 0, 'stable'), ('solid', (6, 5), (8, 7), 0, 'stable_trap'),
    ('solid', (7, 6), (9, 8), 0, 'grown'), ('solid', (8, 7), (11, 9), 0, 'grown'), ('solid', (5, 4), (8, 6), 0, 'grown'),
    ('solid', (12, 10), (15, 13), 0, 'grown'), ('solid', (7, 5), None, 0, 'resolved'), ('solid', (6, 5), None, 0, 'resolved'),
    ('solid', (9, 8), None, 0, 'resolved'), ('solid', (7, 6), (7, 6), 0, 'motion'), ('solid', (10, 9), (10, 9), 0, 'unchanged'),
    ('ground_glass', (7, 6), (7, 6), 0, 'unchanged'), ('ground_glass', (12, 9), (12, 9), 0, 'stable'), ('ground_glass', (6, 5), (6, 5), 0, 'stable'),
    ('ground_glass', (9, 7), (12, 10), 0, 'grown'), ('ground_glass', (7, 6), None, 0, 'resolved'), ('ground_glass', (5, 4), None, 0, 'resolved'),
    ('ground_glass', (12, 9), (14, 11), 0, 'stable_trap'), ('ground_glass', (4, 4), (5, 4), 0, 'slightly_larger'), ('ground_glass', (9, 7), (9, 7), 0, 'motion'),
    ('part_solid', (9, 7), (9, 7), 3, 'unchanged'), ('part_solid', (14, 11), (14, 11), 4, 'stable'), ('part_solid', (11, 9), (11, 9), 5, 'unchanged'),
    ('part_solid', (9, 7), (10, 8), 7, 'solid_grew'), ('part_solid', (14, 11), (14, 12), 8, 'solid_grew'), ('part_solid', (10, 8), (13, 11), 4, 'grown'),
    ('part_solid', (7, 6), None, 0, 'resolved'), ('part_solid', (5, 5), (5, 5), 2, 'stable'), ('part_solid', (10, 8), (10, 8), 3, 'motion'),
    ('solid', (8, 6), (8, 6), 0, 'stable'), ('solid', (13, 11), (13, 11), 0, 'unchanged'), ('solid', (10, 9), (12, 11), 0, 'grown'),
    ('ground_glass', (18, 15), (18, 15), 0, 'unchanged'), ('ground_glass', (5, 5), (5, 5), 0, 'stable'), ('part_solid', (7, 6), (7, 6), 2, 'stable'),
    ('solid', (4, 3), (4, 3), 0, 'stable'), ('solid', (5, 5), (6, 5), 0, 'slightly_larger'), ('solid', (11, 11), (11, 11), 0, 'unchanged'),
    ('ground_glass', (4, 3), (4, 3), 0, 'unchanged'), ('solid', (6, 4), None, 0, 'resolved'), ('part_solid', (14, 11), (17, 14), 5, 'grown'),
]
fus = []
for i, (typ, pr, cu, solid, var) in enumerate(FU):
    lobe, d = r.choice(LOBES), r.choice(DESC[typ])
    unit = 'cm' if r.random() < .15 else 'mm'
    mo = r.choice([3, 4, 6, 8, 12])
    comp = f'COMPARISON: CT chest {mo} months prior.'
    sol = f' The solid component measures {solid} mm.' if typ == 'part_solid' and solid and var != 'solid_grew' else ''
    if var in ('unchanged', 'stable', 'stable_trap'):
        word = {'unchanged': 'is unchanged', 'stable': 'is stable', 'stable_trap': 'is stable in appearance'}[var]
        body = f'The previously noted {d} in the {lobe} {word}, now measuring {dims(*cu, unit)} (previously {dims(*pr, unit)}).{sol}'
        imp = r.choice([f'Stable {d}.', f'No change in the {lobe} nodule.', 'Stable pulmonary nodule.'])
    elif var == 'slightly_larger':
        body = f'The {d} in the {lobe} is perhaps slightly larger, measuring {dims(*cu, unit)} versus {dims(*pr, unit)} previously, within measurement variability.{sol}'
        imp = 'Pulmonary nodule without significant interval change.'
    elif var == 'grown':
        body = f'The {d} in the {lobe} has increased in size, now {dims(*cu, unit)}, previously {dims(*pr, unit)}.{sol}'
        imp = r.choice([f'Interval growth of the {lobe} nodule.', 'Enlarging pulmonary nodule; recommend specialist review.'])
    elif var == 'solid_grew':
        body = (f'The {d} in the {lobe} measures {dims(*cu, unit)} (previously {dims(*pr, unit)}). '
                f'The solid component has increased and now measures {solid} mm.')
        imp = 'Part-solid nodule with enlarging solid component, suspicious for malignancy.'
    elif var == 'resolved':
        body = r.choice([f'The previously described {d} in the {lobe} ({dims(*pr, unit)}) has resolved.',
                         f'The {lobe} nodule seen on the prior study is no longer visualised.'])
        imp = r.choice(['Resolution of the previously seen pulmonary nodule, consistent with an inflammatory cause.', 'Previously seen nodule has resolved.'])
    else:  # motion
        body = f'Marked respiratory motion artefact degrades the lung windows; the {lobe} nodule cannot be reliably measured or compared.'
        imp = 'Non-diagnostic for nodule follow-up due to motion artefact; repeat CT recommended.'
    other = r.choice(['No new pulmonary nodules.', 'No new nodules. No lymphadenopathy.', 'No pleural effusion. No new nodules.'])
    text = f"CT CHEST (NODULE FOLLOW-UP)\n{comp}\nFINDINGS: {body} {other}\nIMPRESSION: {imp}"
    k = {'type': typ, 'variant': var, 'prior_long': pr[0], 'prior_short': pr[1], 'cur_long': cu[0] if cu else 0, 'cur_short': cu[1] if cu else 0,
         'solid_mm': solid if typ == 'part_solid' else 0, 'resolved': var == 'resolved', 'not_assessable': var == 'motion'}
    k['outcome'] = outcome(k)
    fus.append({'id': f'lu{i + 1:02d}', 'text': text, 'key': k})

json.dump({'source': 'synthetic templated corpus (scripts/lung_corpus.py, seed 17)', 'reports': finds},
          open(os.path.join(OUT, 'finding_reports.json'), 'w'), indent=1, ensure_ascii=False)
json.dump({'source': 'synthetic templated corpus (scripts/lung_corpus.py, seed 17)', 'reports': fus},
          open(os.path.join(OUT, 'followup_reports.json'), 'w'), indent=1, ensure_ascii=False)
from collections import Counter
print('finding', len(finds), Counter(f['key']['type'] for f in finds))
print('followup', len(fus), Counter(f['key']['outcome'] for f in fus))
