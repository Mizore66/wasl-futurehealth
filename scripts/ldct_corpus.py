"""Synthetic lung cancer screening (LDCT) text corpus (dev set) for the fifth loop type. Deterministic (seed 31).

Writes data/ldct/screen_reports.json (screening, follow-up and addendum LDCT reports) and data/ldct/workup_notes.json
(thoracic diagnostic unit / lung MDT documents after a Lung-RADS 4B or 4X screen), each with a labelled key. Used to
write reader A (regex) and the reader-B prompt; it is NOT a fair test of either. There is no held-out set for this loop.

Screening label (same rule as pipeline/ldctread.go ldctScreenLabel): a stated final / overall Lung-RADS category
decides ("LR" + category, 0 included); otherwise the most concerning finding in the order 1 < 2 < 3 < 4A < 4B < 4X.
Traps: a prior exam's category (must be ignored), several findings, the S modifier ("Lung-RADS 2S"), 4A vs 4B vs 4X,
word-only categories, negation ("no suspicious nodule"), comparison pending / infectious findings (category 0).

Work-up label: wk_dna | wk_incomplete | wk_treatment | wk_benign | wk_surveillance | indeterminate. wk_treatment needs
malignant tissue AND a documented MDT discussion in the same document (DoH 7.6.1); a treatment referral without MDT
wording is indeterminate (goes to a human). Traps: biopsy awaited, MDT pending, cancer found but MDT still to come,
declined biopsy, an earlier benign biopsy, PET/CT low uptake with surveillance.
usage: python3 scripts/ldct_corpus.py [out_dir]   (default data/ldct)
"""
import json, os, random, sys

OUT = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), '..', 'data', 'ldct')
os.makedirs(OUT, exist_ok=True)
r = random.Random(31)
RANK = {'1': 1, '2': 2, '3': 3, '4A': 4, '4B': 5, '4X': 6}

FIND = {
    '1': ['No pulmonary nodules.', 'No lung nodules are seen.', 'Lungs clear; no nodule identified.'],
    '2': ['4 mm solid nodule in the right upper lobe.', 'Calcified granuloma in the left lower lobe.',
          '6 mm perifissural nodule along the right minor fissure, benign appearance.', '12 mm ground-glass nodule in the left upper lobe.',
          '5 mm solid nodule in the lingula, unchanged for more than 2 years.'],
    '3': ['7 mm solid nodule in the left upper lobe at baseline.', '8 mm part-solid nodule with a solid component under 6 mm in the right lower lobe.',
          '32 mm ground-glass nodule in the right upper lobe.', 'New 5 mm solid nodule in the right middle lobe.'],
    '4A': ['10 mm solid nodule in the right lower lobe.', '9 mm part-solid nodule with a 5 mm solid component in the left upper lobe.',
           'New 7 mm solid nodule in the left lower lobe.', 'Endobronchial nodule in the right lower lobe segmental bronchus.'],
    '4B': ['17 mm solid nodule in the right upper lobe.', 'Part-solid nodule in the left upper lobe with an 8 mm solid component.',
           'Solid nodule in the right lower lobe has grown from 6 mm to 10 mm.'],
    '4X': ['14 mm spiculated solid nodule in the left upper lobe with enlarged left hilar lymph nodes.',
           '9 mm solid nodule with spiculation and a new 12 mm subcarinal lymph node.'],
}
WORD = {'1': 'negative', '2': 'benign appearance or behaviour', '3': 'probably benign', '4A': 'suspicious', '4B': 'very suspicious',
        '4X': 'very suspicious, additional features'}
REC = {'1': 'Continue annual screening: LDCT in 12 months.', '2': 'Continue annual screening: LDCT in 12 months.',
       '3': 'Follow-up LDCT in 6 months.', '4A': 'Follow-up LDCT in 3 months.',
       '4B': 'Refer to the thoracic diagnostic unit: diagnostic chest CT, PET/CT and/or tissue sampling; discuss at the lung MDT.',
       '4X': 'Refer to the thoracic diagnostic unit for PET/CT and tissue sampling; discuss at the lung MDT.'}
INCOMPLETE = [('Prior chest CT from another hospital has been requested for comparison; the final category is deferred.',
               'Comparison with the prior CT is needed before a final category.'),
              ('Patchy consolidation and new clustered nodules in the right lower lobe, most likely infective.',
               'Repeat LDCT in 1 to 3 months to confirm resolution.'),
              ('Severe motion artefact: the lung bases cannot be evaluated.', 'Additional LDCT images are needed.')]
CATFMT = ['Lung-RADS {}', 'Lung-RADS category {}', 'LungRADS {}', 'Lung-RADS v2022 category {}']
HDR = ['Low-dose CT chest, lung cancer screening programme. Double read.', 'LDCT lung screening (baseline round). Two independent readers.',
       'Annual lung cancer screening LDCT. Technique: low-dose, non-contrast, 1 mm reconstructions.',
       'Lung cancer screening CT (designated screening centre). Double reported.']
FU_HDR = ['Follow-up LDCT ({} months), lung cancer screening programme.', 'Interval low-dose CT ({}-month follow-up) for a screen-detected nodule.']


def cat_text(c, s=False):
    t = r.choice(CATFMT).format(c)
    return t + ('S' if s else '')


screens, seen = [], set()
# (label, style); styles: prose (findings + final category), multi (per-finding categories, no stated final),
# words (word category only), zero (category 0), followup, addendum
PLAN = ([('1', 'prose')] * 3 + [('1', 'words')] * 2 + [('2', 'prose')] * 4 + [('2', 'multi')] * 2 + [('2', 'words')] * 2 +
        [('3', 'prose')] * 3 + [('3', 'multi')] * 2 + [('3', 'words')] + [('4A', 'prose')] * 3 + [('4A', 'multi')] * 2 + [('4A', 'words')] +
        [('4B', 'prose')] * 3 + [('4B', 'multi')] * 2 + [('4X', 'prose')] * 2 + [('4X', 'multi')] +
        [('0', 'zero')] * 6 + [('2', 'followup')] * 3 + [('3', 'followup')] * 2 + [('4B', 'followup')] +
        [('2', 'addendum')] * 2 + [('1', 'addendum')] + [('3', 'addendum')] + [('4A', 'addendum')])
for i, (c, style) in enumerate(PLAN):
    lab = 'LR' + c
    for _ in range(60):
        parts = [r.choice(HDR)]
        if style == 'followup':
            parts = [r.choice(FU_HDR).format(r.choice(['3', '6']))]
        if style == 'addendum':
            parts = ['Addendum to the screening LDCT report.', r.choice(['The prior chest CT (2024) has now been received and compared.',
                                                                         'Comparison with the outside CT from 2023 is now available.'])]
        if r.random() < .3 and style not in ('addendum',):
            parts.append(f"Prior LDCT ({r.choice(['2024', '2025'])}): {cat_text(r.choice(['2', '3', '4A']))}, for comparison only.")
        s_mod = c in ('1', '2', '3') and r.random() < .2
        if style == 'zero':
            find, rec = INCOMPLETE[i % 3]
            parts += [find, f'Final assessment: {cat_text("0")}.', rec]
        elif style == 'multi':
            lower = [k for k in RANK if RANK[k] < RANK[c] and k != '1']
            other = r.choice(lower) if lower else '2'
            fs = [(c, r.choice(FIND[c])), (other, r.choice(FIND[other]))]
            if RANK[other] > RANK[c]:
                fs = [(c, r.choice(FIND[c]))]
            r.shuffle(fs)
            for k, (cc, f) in enumerate(fs):
                parts.append(f'Nodule {k + 1}: {f[0].lower() + f[1:-1]}, {cat_text(cc)}.')
            parts.append(REC[c])
        elif style == 'words':
            parts.append(r.choice(FIND[c]))
            if c in ('1', '2'):
                parts.append('No suspicious nodule.')
            parts.append(f'Lung-RADS assessment: {WORD[c]}.')
            parts.append(REC[c])
        else:  # prose, followup, addendum
            f = r.choice(FIND[c])
            if style == 'followup' and c == '2':
                f = r.choice(['The 7 mm left upper lobe nodule is unchanged in size (stepped down per Lung-RADS v2022).',
                              'The previously seen 9 mm part-solid nodule is smaller.', 'The 6 mm right lower lobe nodule has resolved.'])
            if style == 'followup' and c == '4B':
                f = 'The 9 mm right lower lobe nodule has grown to 13 mm.'
            parts.append(f)
            if c in ('1', '2') and r.random() < .5:
                parts.append(r.choice(['No suspicious nodule or mass.', 'No new nodules.']))
            if s_mod:
                parts.append('Moderate coronary artery calcification (S finding), for primary care review.')
            lead = r.choice(['Final assessment', 'Overall', 'Impression', 'Conclusion'])
            parts.append(f'{lead}: {cat_text(c, s_mod)} ({WORD[c]}).')
            parts.append(REC[c])
        text = ' '.join(parts)
        if text not in seen:
            break
    seen.add(text)
    screens.append({'id': f'ls{i + 1:02d}', 'text': text, 'key': {'label': lab, 'style': style}})

NOTES = {
    'wk_treatment': [('yes', 'done', 'malignant', 'treatment', 'CT-guided biopsy of the right upper lobe nodule: adenocarcinoma. Discussed at the lung MDT: stage IA, referred to thoracic surgery for lobectomy.'),
                     ('yes', 'done', 'malignant', 'treatment', 'EBUS-TBNA: squamous cell carcinoma. Lung MDT decision: referred to clinical oncology for stereotactic radiotherapy.'),
                     ('yes', 'done', 'malignant', 'treatment', 'Histology: non-small cell lung cancer. Discussed at the multidisciplinary team meeting; referral to the thoracic surgical team sent.')],
    'wk_benign': [('yes', 'done', 'benign', 'annual', 'PET/CT: no FDG uptake. CT-guided biopsy: organising pneumonia, no malignancy. MDT: benign, return to annual screening.'),
                  ('', 'done', 'none', 'annual', 'Diagnostic chest CT: the nodule has resolved, in keeping with infection. Return to annual LDCT screening.'),
                  ('yes', 'done', 'benign', 'annual', 'Bronchoscopic biopsy: granuloma, no malignant cells. Discussed at MDT: benign; back to the annual screening programme.')],
    'wk_surveillance': [('yes', 'done', 'none', 'surveillance', 'PET/CT shows low uptake (SUVmax 1.4). MDT: not suitable for biopsy; surveillance CT in 3 months.'),
                        ('yes', 'done', 'none', 'surveillance', 'Diagnostic CT with contrast: the lesion is smaller. MDT plan: interval CT in 3 months.'),
                        ('', 'done', 'benign', 'surveillance', 'CT-guided biopsy: non-diagnostic benign lung tissue. Plan: repeat CT in 3 months to confirm stability.')],
    'wk_incomplete': [('', 'done', 'pending', 'pending', 'Seen in the thoracic clinic. CT-guided biopsy performed today; histology awaited, for discussion at the next lung MDT.'),
                      ('', 'done', 'none', 'pending', 'Thoracic clinic review. PET/CT booked; biopsy to follow. For MDT discussion.'),
                      ('', 'not_done', 'none', 'pending', 'Attended the thoracic clinic but declined biopsy today; to return after counselling.'),
                      ('', 'done', 'pending', 'pending', 'Biopsy suspicious for malignancy; immunohistochemistry pending. MDT pending.'),
                      ('', 'done', 'malignant', 'pending', 'CT-guided biopsy: adenocarcinoma. For discussion at the lung MDT next week before referral.')],
    'wk_dna': [('', '', 'none', 'pending', 'Did not attend the thoracic clinic appointment. Second appointment sent.'),
               ('', '', 'none', 'pending', 'DNA for CT-guided biopsy. Patient contacted, rebooking.'),
               ('', '', 'none', 'pending', 'Failed to attend the diagnostic CT appointment today. Screening centre informed.')],
    'indeterminate': [('', 'done', 'malignant', 'treatment', 'CT-guided biopsy: adenocarcinoma. Referred to thoracic surgery.'),
                      ('', 'done', 'malignant', 'treatment', 'Histology confirms small cell lung cancer; referral to medical oncology sent.')],
}
PLANW = ['wk_treatment'] * 9 + ['wk_benign'] * 8 + ['wk_surveillance'] * 7 + ['wk_incomplete'] * 12 + ['wk_dna'] * 6 + ['indeterminate'] * 3
WHDR = ['Thoracic diagnostic unit, regional hospital.', 'Lung MDT outcome record.', 'Respiratory clinic note (screen-detected nodule).',
        'Thoracic oncology rapid-access clinic.']
notes, used = [], set()
for i, lab in enumerate(PLANW):
    for _ in range(60):
        mdt, a, t, o, base = r.choice(NOTES[lab])
        hdrs = [h for h in WHDR if 'MDT' not in h] if lab == 'indeterminate' else WHDR  # no MDT wording on the no-MDT trap
        text = r.choice(hdrs) + ' ' + base
        if r.random() < .25:
            text = r.choice(hdrs[:3]) + ' Referred from lung cancer screening (Lung-RADS ' + r.choice(['4B', '4X']) + '). ' + base
        if r.random() < .2 and lab != 'wk_dna':
            text += ' Previous benign biopsy in 2019.'
        if text not in used:
            break
    used.add(text)
    notes.append({'id': f'lw{i + 1:02d}', 'text': text, 'key': {'attended': lab != 'wk_dna', 'workup': a or 'not_done', 'mdt': mdt or 'no',
                  'tissue': t, 'outcome': o, 'label': lab}})

src = 'synthetic templated corpus (scripts/ldct_corpus.py, seed 31); dev set, used to write the readers; no held-out set'
json.dump({'source': src, 'reports': screens}, open(os.path.join(OUT, 'screen_reports.json'), 'w'), indent=1, ensure_ascii=False)
json.dump({'source': src, 'reports': notes}, open(os.path.join(OUT, 'workup_notes.json'), 'w'), indent=1, ensure_ascii=False)
print(len(screens), 'screening reports,', len(notes), 'work-up documents ->', OUT)
