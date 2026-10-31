"""Synthetic breast screening text corpus (dev set) for the fourth loop type. Deterministic (seed 29).

Writes data/breast/screen_reports.json (screening mammogram reports) and data/breast/assess_notes.json (diagnostic
breast assessment unit / MDT documents after a recall), each with a labelled key. Used to write reader A (regex) and
the reader-B prompt; it is NOT a fair test of either. The held-out set (data/breast/heldout/) is written by another
model family and scored once.

Screening label (same rule as pipeline/breastread.go breastScreenLabel): the overall BI-RADS final assessment, i.e. the
more worrying breast in the ACR 5th edition order 1 < 2 < 3 < 0 < 4 < 5 ("B" + category); "TR" when the screen is
technically inadequate, unless the other breast carries a recall category (0, 3, 4, 5) on its own.
Traps: per-breast wording (right/left, RT/LT, Arabic), a previous screen's category (must be ignored), negation
("no suspicious mass"), 4A/4B/4C, word-only assessments ("benign", "negative"), technical inadequacy in one breast.

Assessment label: dna | incomplete | return_routine | early_recall | to_treatment (to_treatment needs a malignant
needle biopsy in the same document; an outcome while the biopsy is pending is incomplete).
Traps: biopsy taken but result awaited, "discussed at MDT" without a decision, core-biopsy B-codes (B2 benign, B5
malignant) that look like BI-RADS, negated malignancy, patient declined biopsy, a previous benign biopsy.
usage: python3 scripts/breast_corpus.py [out_dir]   (default data/breast)
"""
import json, os, random, sys

OUT = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), '..', 'data', 'breast')
os.makedirs(OUT, exist_ok=True)
r = random.Random(29)
RANK = {'1': 0, '2': 1, '3': 2, '0': 3, '4': 4, '5': 5, '6': 6}
CPT = {'0': '3340F', '1': '3341F', '2': '3342F', '3': '3343F', '4': '3344F', '5': '3345F'}

FIND = {
    '1': ['no suspicious mass, calcification or architectural distortion', 'no mammographic abnormality',
          'normal fibroglandular tissue, no focal lesion'],
    '2': ['benign vascular calcifications', 'a stable circumscribed mass, unchanged, in keeping with a fibroadenoma',
          'an intramammary lymph node in the upper outer quadrant', 'an oil cyst with rim calcification'],
    '3': ['a circumscribed oval mass, probably benign', 'a small group of round punctate calcifications, probably benign'],
    '0': ['a focal asymmetry in the upper outer quadrant seen on the CC view only', 'a possible mass on the MLO view needing further views',
          'a developing asymmetry compared with the previous screen'],
    '4': ['grouped amorphous calcifications, suspicious', 'an irregular mass with indistinct margins, 11 mm',
          'architectural distortion without a central mass, suspicious'],
    '5': ['a spiculated mass with pleomorphic calcifications, highly suggestive of malignancy',
          'an irregular spiculated mass 18 mm with skin tethering, highly suggestive of malignancy'],
    'TR': ['technically inadequate: the pectoral muscle is not shown on the MLO view', 'motion blur on the CC view; images inadequate for reading'],
}
WORD = {'1': 'negative', '2': 'benign', '3': 'probably benign', '0': 'incomplete, needs additional imaging evaluation',
        '4': 'suspicious abnormality', '5': 'highly suggestive of malignancy'}
SIDE = [('Right breast', 'Left breast'), ('RT', 'LT'), ('Right', 'Left'), ('الثدي الأيمن (right)', 'الثدي الأيسر (left)')]
CATFMT = ['BI-RADS {}', 'BIRADS {}', 'BI-RADS category {}', 'category {}']


def screen_label(br, bl):
    cats = [br, bl]
    if 'TR' in cats:
        ok = [c for c in cats if c in ('0', '3', '4', '5')]
        if not ok:
            return 'TR'
        cats = ok
    w = max(cats, key=lambda c: RANK[c])
    return 'B' + w


def cat_text(c):
    s = r.choice(CATFMT).format(c)
    if c == '4' and r.random() < .5:
        s = s.replace(' 4', ' 4' + r.choice('ABC'))
    return s


screens, seen = [], set()
PLAN = ([('1', '1')] * 7 + [('2', '1'), ('1', '2'), ('2', '2')] * 3 + [('3', '1'), ('2', '3')] +
        [('0', '1'), ('1', '0'), ('0', '2'), ('3', '0')] * 2 + [('4', '1'), ('2', '4'), ('4', '0')] * 2 + [('5', '1'), ('1', '5'), ('5', '4')] +
        [('TR', 'TR'), ('TR', '1'), ('2', 'TR'), ('TR', '4')])
for i, (br, bl) in enumerate(PLAN):
    lab = screen_label(br, bl)
    style = ['prose', 'prose', 'synoptic', 'words'][i % 4]
    if lab == 'TR' and style == 'synoptic':
        style = 'prose'
    for _ in range(50):
        sr, sl = r.choice(SIDE)
        hdr = r.choice(['Screening mammogram, bilateral CC and MLO views. Double read.', 'Bilateral screening mammography (2D). Two independent readers.',
                        'Mammography screening report. Breast composition: ACR ' + r.choice('ABCD') + '.'])
        parts = [hdr]
        if r.random() < .3:
            parts.append(f"Previous screen ({r.choice(['2022', '2023', '2024'])}): BI-RADS {r.choice(['2', '3', '0'])}, for comparison only.")
        if style == 'synoptic':
            text = (f'Synoptic screening mammography report. {sr}: BI-RADS {br}. {sl}: BI-RADS {bl}. '
                    f'Final assessment: BI-RADS {lab[1:]} ({WORD[lab[1:]]}).')
            if lab in ('B0', 'B3', 'B4', 'B5'):
                text += ' Recall to the breast assessment unit.'
        else:
            for side, c in ((sr, br), (sl, bl)):
                f = r.choice(FIND[c])
                if style == 'words' or c == 'TR':
                    parts.append(f'{side}: {f}.' + ('' if c == 'TR' else f' Assessment: {WORD[c]}.'))
                else:
                    parts.append(f'{side}: {f}. {cat_text(c)}.')
            if br in ('1', '2') and bl in ('1', '2') and r.random() < .6:
                parts.append(r.choice(['No suspicious mass or malignant-type calcification.', 'No architectural distortion.']))
            if lab == 'TR':
                parts.append('Technical repeat required before a final assessment can be given.')
            elif style == 'prose' and r.random() < .6:
                parts.append(f'Overall: {cat_text(lab[1:])}.')
            if lab in ('B0', 'B3', 'B4', 'B5'):
                parts.append(r.choice(['Recall for further assessment.', 'Refer to the diagnostic breast assessment unit.']))
            elif lab != 'TR':
                parts.append('Routine recall in 2 years.')
            text = ' '.join(parts)
        if text not in seen:
            break
    seen.add(text)
    screens.append({'id': f'bs{i + 1:02d}', 'text': text, 'key': {'b_right': br, 'b_left': bl, 'label': lab, 'style': style,
                    'coded': CPT[lab[1:]] if lab != 'TR' else None}})

NOTES = {
    'return_routine': [('done', 'none', 'routine', 'Further views and spot compression: the asymmetry disperses, no true lesion. Ultrasound normal. Return to routine 2-yearly screening.'),
                       ('done', 'none', 'routine', 'Triple assessment: clinical examination normal; ultrasound shows a simple cyst at 2 o\'clock. No biopsy required. Discharged to routine screening.'),
                       ('done', 'benign', 'routine', 'US-guided core biopsy left breast: fibroadenoma (B2), concordant with imaging. MDT outcome: benign, return to routine screening.'),
                       ('done', 'benign', 'routine', 'Stereotactic vacuum biopsy of the calcifications: benign fibrocystic change, no atypia, no evidence of malignancy. MDT: concordant benign; back to the screening programme.')],
    'early_recall': [('done', 'none', 'early_recall', 'Ultrasound: circumscribed oval mass 7 mm, probably benign. MDT outcome: early recall for repeat imaging in 6 months.'),
                     ('done', 'benign', 'early_recall', 'Core biopsy: benign breast tissue (B2), but imaging-pathology concordance uncertain. MDT: short-interval follow-up mammogram in 6 months.'),
                     ('done', 'none', 'early_recall', 'Additional views: probably benign punctate calcifications. Six-month early recall mammogram arranged.')],
    'to_treatment': [('done', 'malignant', 'treatment', 'US-guided core biopsy right breast: invasive ductal carcinoma, grade 2 (B5b). MDT: referred to breast surgery for wide local excision and sentinel node biopsy.'),
                     ('done', 'malignant', 'treatment', 'Stereotactic biopsy: high-grade DCIS (B5a). MDT recommends surgery; referral to the breast surgeon sent today.'),
                     ('done', 'malignant', 'treatment', 'Histology from the core biopsy: invasive lobular carcinoma. Discussed at MDT: neoadjuvant chemotherapy, referred to medical oncology.')],
    'incomplete': [('done', 'pending', 'pending', 'Seen in the assessment clinic. Ultrasound-guided core biopsy of the left breast mass taken. Results awaited; for discussion at the next MDT.'),
                   ('done', 'pending', 'treatment', 'Imaging highly suspicious (U5). Core biopsy taken today, histology awaited; provisional referral to the breast surgeon pending results.'),
                   ('not_done', 'none', 'pending', 'Attended the assessment clinic but declined biopsy today; counselled, to return next week.'),
                   ('done', 'none', 'pending', 'Discussed at MDT: further imaging with breast MRI requested before a decision.'),
                   ('not_done', 'none', 'pending', 'Patient arrived; the ultrasound machine was unavailable. Assessment rebooked for Thursday.')],
    'dna': [('', 'none', 'pending', 'Patient did not attend her breast assessment appointment. Second appointment sent.'),
            ('', 'none', 'pending', 'DNA. Recall letter re-sent and GP informed.'),
            ('', 'none', 'pending', 'Failed to attend the diagnostic breast clinic today. Rebook as per policy.')],
}
PLANC = ['return_routine'] * 13 + ['early_recall'] * 7 + ['to_treatment'] * 10 + ['incomplete'] * 12 + ['dna'] * 8
HDR = ['Diagnostic breast assessment unit.', 'Breast assessment clinic, regional hospital.', 'Breast MDT outcome record.', 'One-stop breast clinic note.']
notes, used = [], set()
for i, lab in enumerate(PLANC):
    for _ in range(50):
        a, b, o, base = r.choice(NOTES[lab])
        text = r.choice(HDR) + ' ' + base
        if r.random() < .25:
            text = r.choice(HDR[:2]) + ' Recalled from screening (BI-RADS ' + r.choice(['0', '4', '5']) + '). ' + base
        if r.random() < .2 and lab != 'dna':
            text += ' Previous benign biopsy in 2019.'
        if text not in used:
            break
    used.add(text)
    notes.append({'id': f'ba{i + 1:02d}', 'text': text, 'key': {'attended': lab != 'dna', 'assessment': a or 'not_done', 'biopsy': b,
                  'outcome': o, 'label': lab}})

src = 'synthetic templated corpus (scripts/breast_corpus.py, seed 29); dev set, used to write the readers'
json.dump({'source': src, 'reports': screens}, open(os.path.join(OUT, 'screen_reports.json'), 'w'), indent=1, ensure_ascii=False)
json.dump({'source': src, 'reports': notes}, open(os.path.join(OUT, 'assess_notes.json'), 'w'), indent=1, ensure_ascii=False)
print(len(screens), 'screening reports,', len(notes), 'assessment documents ->', OUT)
