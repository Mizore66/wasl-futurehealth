#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
gen_heldout3.py -- third, independent held-out corpus of synthetic colonoscopy
reports (Abu Dhabi units, FIT-positive screening / surveillance) with answer keys.

Deterministic, standard library only, random seed 3031.
Usage:  python3 gen_heldout3.py
Writes reports_heldout3.json and README.md next to this file, then runs a self-audit.

Design: every report starts from a hidden reality (caecum truly reached? prep truly
adequate? tissue taken?) plus a documentation plan (what the text states about each
item).  The text is then rendered in one of six layouts with optional overlays
(US spelling, Arabic, typos, scanning noise).  The answer key is derived from the
documentation plan, the truth from the hidden reality.  Hard cases are tagged "hard:*".
"""
import collections
import datetime
import hashlib
import json
import os
import random
import re

SEED = 3031
N = 300
HERE = os.path.dirname(os.path.abspath(__file__))
OUT_JSON = os.path.join(HERE, 'reports_heldout3.json')
OUT_README = os.path.join(HERE, 'README.md')
R = random.Random(SEED)


def ch(seq):
    return R.choice(seq)


def p(x):
    return R.random() < x


def wch(pairs):
    items, weights = zip(*pairs)
    return R.choices(items, weights)[0]


def cap(s):
    return s[:1].upper() + s[1:]


# --------------------------------------------------------------------------- data
SEG = ['rectum', 'sigmoid colon', 'descending colon', 'splenic flexure',
       'transverse colon', 'hepatic flexure', 'ascending colon', 'caecum']
SEG_SHORT = {'rectum': 'rect', 'sigmoid colon': 'sig', 'descending colon': 'DC',
             'splenic flexure': 'SF', 'transverse colon': 'TC', 'hepatic flexure': 'HF',
             'ascending colon': 'AC', 'caecum': 'caecum'}
SEG_W = {'rectum': 2, 'sigmoid colon': 4, 'descending colon': 2, 'splenic flexure': 0.7,
         'transverse colon': 2, 'hepatic flexure': 0.8, 'ascending colon': 2, 'caecum': 1.5}
STOPS = SEG[1:7]

UNITS = ['Al Reem Digestive Health Centre, Abu Dhabi',
         'Saadiyat Endoscopy Day Unit',
         'Khalifa City Gastroenterology Clinic',
         'Corniche Specialist Hospital - Endoscopy Suite',
         'Mussafah Medical Centre - GI Unit',
         'Al Bateen Day Surgery Centre - Endoscopy',
         'Yas Gastro & Liver Clinic']
DOCTORS = ['Dr. Layla Mansour', 'Dr. R. Kapoor', 'Dr. Omar Al Shamsi', 'Dr. S. Fernandes',
           'Dr. Hind Yousef', 'Dr. M. Okafor', 'Dr. Fatima Al Hammadi', 'Dr. J. Whitaker',
           'Dr. Karim Nassar', 'Dr. A. Rahman']
NURSES = ['RN Joy Santos', 'RN M. Pillai', 'RN Aisha Khalid', 'RN T. Mendoza']
PATHOLOGISTS = ['Dr. N. Haddad', 'Dr. P. Iyer', 'Dr. G. Moreau', 'Dr. Salma Darwish']
SCOPES = ['Olympus CF-HQ190L', 'Olympus PCF-H190DL', 'Pentax EC-3890Li', 'Fujifilm EC-760R-V/L']
SOFTWARE = ['EndoRecord v5.1', 'GIView Reporting 3.4', 'ScopeNote Pro 2.0', 'MedEndo Suite 11']
CLINICS = ['Family Medicine Clinic, Al Mushrif', 'Primary Care Centre, Baniyas', 'Screening Clinic, Khalifa City']
PRODUCTS = ['split-dose PEG 4 L', 'Moviprep (split dose)', 'Plenvu 1 L (split dose)',
            'sodium picosulfate / magnesium citrate (Picoprep)', 'Klean-Prep 4 L',
            'PEG 2 L + bisacodyl', 'same-day Plenvu',
            'low-residue diet x2 days + split-dose Moviprep']
SEDATION = [
    ('Midazolam 2 mg IV, fentanyl 50 mcg IV',
     'Conscious sedation was given with midazolam 2 mg and fentanyl 50 micrograms intravenously.',
     'midaz 2/fent 50'),
    ('Midazolam 3 mg + pethidine 25 mg IV',
     'Sedation: midazolam 3 mg and pethidine 25 mg IV.',
     'midaz 3/peth 25'),
    ('Propofol (anaesthetist-administered, MAC)',
     'The procedure was performed under propofol sedation administered by the anaesthetist.',
     'propofol (anaes)'),
    ('None - patient declined; Entonox used',
     'No intravenous sedation was given at the patient\'s request; Entonox was used.',
     'unsedated, Entonox'),
    ('Midazolam 2.5 mg IV',
     'Midazolam 2.5 mg was given intravenously.',
     'midaz 2.5'),
]
DATE_FMTS = ['%d/%m/%Y', '%Y-%m-%d', '%d %b %Y', '%d-%b-%y', '%d.%m.%Y']

METHODS = {
    'cold_snare': ('cold snare', 'Cold snare polypectomy', 'CSP'),
    'cold_forceps': ('cold biopsy forceps', 'Cold forceps polypectomy', 'CFP'),
    'hot_snare': ('hot snare (prophylactic clip applied)', 'Hot snare polypectomy + clip', 'HSP'),
    'emr': ('EMR (submucosal lift, en bloc snare)', 'Endoscopic mucosal resection, en bloc', 'EMR'),
}

LABELS = {
    'indication': (['Indication', 'Indication(s)', 'Reason for procedure', 'Clinical indication'], 'دواعي الإجراء'),
    'prep': (['Bowel preparation', 'Bowel prep', 'Preparation', 'Prep'], 'تحضير الأمعاء'),
    'sedation': (['Sedation', 'Sedation/analgesia', 'Medications'], 'التخدير'),
    'extent': (['Extent of examination', 'Extent', 'Extent reached', 'Completeness'], 'مدى الفحص'),
    'withdrawal': (['Withdrawal time', 'Withdrawal'], 'زمن السحب'),
    'findings': (['Findings', 'Endoscopic findings', 'Observations'], 'النتائج'),
    'polyps': (['Polyps', 'Polyp details', 'Lesions'], 'السلائل'),
    'interventions': (['Interventions/specimens', 'Therapy & specimens', 'Procedures performed', 'Specimens'], 'الإجراءات والعينات'),
    'complications': (['Complications', 'Adverse events'], 'المضاعفات'),
    'impression': (['Impression', 'Conclusion', 'Summary', 'Endoscopic diagnosis'], 'الانطباع'),
    'recommendations': (['Recommendations', 'Plan', 'Follow-up'], 'التوصيات'),
}

AR_GRADE = {'excellent': 'ممتاز', 'good': 'جيد', 'adequate': 'مقبول', 'fair': 'مقبول'}

US_MAP = [('Caecal', 'Cecal'), ('caecal', 'cecal'), ('CAECAL', 'CECAL'),
          ('Caecum', 'Cecum'), ('caecum', 'cecum'), ('CAECUM', 'CECUM'),
          ('haemorrhoids', 'hemorrhoids'), ('Haemorrhoids', 'Hemorrhoids'),
          ('haemostasis', 'hemostasis'), ('diarrhoea', 'diarrhea'), ('faecal', 'fecal'),
          ('Faecal', 'Fecal'), ('faeces', 'feces'), ('oedematous', 'edematous'),
          ('oedema', 'edema'), ('paediatric', 'pediatric'), ('anaesthetist', 'anesthesiologist'),
          ('programme', 'program'), ('visualisation', 'visualization'),
          ('haemorrhage', 'hemorrhage'), ('anaes', 'anes')]

OCR = {'normal': 'norrnal', 'mucosa': 'rnucosa', 'examination': 'exarnination',
       'colonoscope': 'co1onoscope', 'patient': 'patlent', 'performed': 'perforrned',
       'identified': 'identifled', 'procedure': 'proccdure', 'advanced': 'advauced',
       'minutes': 'rninutes', 'Impression': 'lmpression', 'Findings': 'Fiudings',
       'sedation': 'sedatiou', 'Endoscopist': 'Endoscop1st', 'Indication': 'lndication'}
PROTECT = ('caec', 'cecum', 'cecal', 'ileum', 'ileoc', 'biops', 'polyp', 'adenom', 'prep',
           'poor', 'inadequ', 'histo', 'patho', 'reached', 'incomplete', 'flexure', 'append', 'snare')
TYPOS = [('caecum', ['ceacum', 'caecuum', 'caceum']), ('cecum', ['cecun', 'ceccum']),
         ('biopsies', ['biospies', 'biopises']), ('polyp', ['polpy', 'ployp']),
         ('preparation', ['preperation', 'preparaton']), ('withdrawal', ['withdrawl']),
         ('Withdrawal', ['Withdrawl']), ('histology', ['histolgy', 'hisotlogy'])]

BOSTON_ADEQ_W = [(2, 4), (3, 6)]
BOSTON_LOW_TOTAL = [(1, 2, 2), (1, 1, 2), (0, 1, 2), (1, 1, 1), (2, 1, 2), (0, 2, 2), (1, 2, 1), (2, 2, 1)]
BOSTON_TOTAL_OK_SEG_LOW = [(3, 3, 1), (1, 2, 3), (1, 3, 3), (3, 2, 1), (2, 3, 1), (0, 3, 3), (1, 3, 2), (3, 1, 3)]


# --------------------------------------------------------------- scenario plan
def build_scenarios():
    truths = ['adequate'] * 120 + ['awaiting_histology'] * 90 + ['inadequate'] * 90
    R.shuffle(truths)
    inad = (['doc_incomplete'] * 30 + ['doc_poor_prep'] * 33 + ['hidden_caecum'] * 10 +
            ['hidden_prep'] * 11 + ['corr_caecum'] * 3 + ['corr_prep'] * 3)
    R.shuffle(inad)
    out = []
    for t in truths:
        s = dict(truth=t, correction=None, inad_kind=None)
        if t == 'inadequate':
            k = inad.pop()
            s['inad_kind'] = k
            if k == 'doc_incomplete':
                s['caecum_doc'], s['caecum_real'] = 'no', False
                r = R.random()
                if r < 0.55:
                    s['prep_doc'], s['prep_real'] = 'adequate', True
                elif r < 0.78:
                    s['prep_doc'], s['prep_real'] = 'inadequate', False
                else:
                    s['prep_doc'], s['prep_real'] = 'not_stated', p(0.8)
            elif k == 'doc_poor_prep':
                s['prep_doc'], s['prep_real'] = 'inadequate', False
                r = R.random()
                if r < 0.78:
                    s['caecum_doc'], s['caecum_real'] = 'yes', True
                elif r < 0.93:
                    s['caecum_doc'], s['caecum_real'] = 'not_stated', p(0.7)
                else:
                    s['caecum_doc'], s['caecum_real'] = 'no', False
            elif k == 'hidden_caecum':
                s['caecum_doc'], s['caecum_real'] = 'not_stated', False
                s['prep_doc'] = 'adequate' if p(0.7) else 'not_stated'
                s['prep_real'] = True
            elif k == 'hidden_prep':
                s['prep_doc'], s['prep_real'] = 'not_stated', False
                s['caecum_doc'] = 'yes' if p(0.8) else 'not_stated'
                s['caecum_real'] = True
            elif k == 'corr_caecum':
                s.update(caecum_doc='no', caecum_real=False, prep_doc='adequate', prep_real=True,
                         correction='caecum_to_no')
            elif k == 'corr_prep':
                s.update(prep_doc='inadequate', prep_real=False, caecum_doc='yes', caecum_real=True,
                         correction='prep_to_poor')
            s['tissue'] = p(0.4)
            s['histo'] = s['tissue'] and p(0.3)
        else:
            s['caecum_doc'] = 'yes' if p(0.83) else 'not_stated'
            s['caecum_real'] = True
            s['prep_doc'] = 'adequate' if p(0.74) else 'not_stated'
            s['prep_real'] = True
            if t == 'awaiting_histology':
                s['tissue'], s['histo'] = True, False
            else:
                s['tissue'] = p(0.32)
                s['histo'] = s['tissue']
        if s['tissue']:
            r = R.random()
            s['tissue_kind'] = 'polyp' if r < 0.62 else ('biopsy' if r < 0.85 else 'both')
        else:
            s['tissue_kind'] = None
        out.append(s)
    cands = [s for s in out if s['truth'] != 'inadequate' and s['caecum_doc'] == 'yes'
             and s['correction'] is None]
    for s in R.sample(cands, 3):
        s['correction'] = 'caecum_to_yes'
    return out


# -------------------------------------------------------------- polyp helpers
def make_polyp(site, p_opt=0.0):
    size = R.choices([2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 15], [4, 8, 8, 9, 8, 6, 5, 3, 4, 3, 2])[0]
    if size <= 3:
        morph = ch(['sessile (Paris 0-Is)', 'flat (Paris 0-IIa)', 'sessile'])
        method = ch(['cold_forceps', 'cold_snare'])
    elif size <= 9:
        morph = ch(['sessile (Paris 0-Is)', 'flat (Paris 0-IIa)', 'sessile', 'semi-pedunculated (Paris 0-Isp)'])
        method = 'cold_snare'
    elif p(0.5):
        morph, method = 'pedunculated (Paris 0-Ip)', 'hot_snare'
    else:
        morph, method = ch(['sessile (Paris 0-Is)', 'flat (Paris 0-IIa)']), 'emr'
    distal = site in ('rectum', 'sigmoid colon')
    if size <= 5 and distal and p(0.5):
        dx = 'Hyperplastic polyp.'
    elif not distal and p(0.25):
        dx = 'Sessile serrated lesion without dysplasia.'
    elif size >= 10 and p(0.4):
        dx = 'Tubulovillous adenoma with low-grade dysplasia; completely excised.'
    else:
        dx = 'Tubular adenoma with low-grade dysplasia.'
    optical = None
    if p(p_opt):
        optical = 'NICE 1, likely hyperplastic' if (dx.startswith('Hyper') and p(0.7)) else 'NICE 2, likely adenoma'
    m = re.search(r'0-I\w*', morph)
    return dict(site=site, size=size, morph=morph, morph_short=m.group(0) if m else 'sessile',
                method=method, dx=dx, optical=optical)


def biopsy_types(maxidx):
    if maxidx >= 6:
        rs, rss = 'right and left colon', 'R+L colon'
    elif maxidx >= 2:
        rs, rss = 'descending and sigmoid colon', 'DC+sig'
    else:
        rs, rss = 'sigmoid colon and rectum', 'sig+rect'
    return [
        dict(key='random', minidx=1, diarrhoea=True,
             prose=f'The mucosa appeared macroscopically normal; random biopsies were taken from the {rs} to exclude microscopic colitis',
             find='Macroscopically normal mucosa', act=f'Random biopsies, {rs}',
             terse=f'random bx {rss}', spec=f'random colonic biopsies ({rs})',
             dx='Colonic mucosa within normal limits; no features of microscopic (lymphocytic or collagenous) colitis.'),
        dict(key='ulcer', minidx=0, diarrhoea=False,
             prose='A shallow 8 mm ulcer was seen on the anterior rectal wall; four biopsies were taken from its edge',
             find='Rectum: 8 mm shallow ulcer, anterior wall', act='Biopsies x4, rectal ulcer edge',
             terse='8mm rectal ulcer, bx x4', spec='rectal ulcer biopsies',
             dx='Rectal mucosa with mucosal prolapse-type changes; no dysplasia.'),
        dict(key='erythema', minidx=1, diarrhoea=False,
             prose='There was patchy erythema with loss of the vascular pattern in the sigmoid colon, which was biopsied',
             find='Sigmoid: patchy erythema, loss of vascular pattern', act='Sigmoid mucosal biopsies x3',
             terse='patchy erythema sig, bx x3', spec='sigmoid mucosal biopsies',
             dx='Mild non-specific chronic inflammation; no granulomas, no dysplasia.'),
        dict(key='nodule', minidx=2, diarrhoea=False,
             prose='A 4 mm smooth, submucosal-appearing nodule in the descending colon was sampled with biopsy forceps (two bites)',
             find='Descending colon: 4 mm smooth submucosal-appearing nodule', act='Forceps biopsies x2 of descending colon nodule',
             terse='4mm SM nodule DC, bx x2', spec='descending colon nodule biopsies',
             dx='Prominent benign lymphoid aggregate; no dysplasia.'),
        dict(key='transverse', minidx=4, diarrhoea=False,
             prose='An area of erythematous, oedematous mucosa in the transverse colon was biopsied',
             find='Transverse colon: focal erythema and oedema', act='Transverse colon biopsies x2',
             terse='TC erythema, bx x2', spec='transverse colon biopsies',
             dx='Mild active colitis, non-specific; no dysplasia.'),
        dict(key='ti', minidx=7, diarrhoea=False,
             prose='Several small aphthous ulcers were seen in the terminal ileum and biopsies were taken',
             find='Terminal ileum: scattered aphthous ulcers', act='Terminal ileal biopsies x4',
             terse='aphthous ulcers TI, bx x4', spec='terminal ileal biopsies',
             dx='Small-bowel mucosa with mild active ileitis; no granulomas, no dysplasia.'),
    ]


OTHER_FINDINGS = [
    ('There was sigmoid diverticulosis.', 'Sigmoid diverticulosis', 'sig diverticulosis', 1, None, 'Sigmoid diverticulosis.'),
    ('Scattered diverticula were present in the descending and sigmoid colon.',
     'Scattered diverticula, descending and sigmoid colon', 'diverticula DC/sig', 2, None, 'Left-sided diverticular disease.'),
    ('Small non-bleeding internal haemorrhoids were seen on retroflexion.',
     'Grade I internal haemorrhoids (retroflexion)', 'int haemorrhoids', 0, None, 'Small internal haemorrhoids.'),
    ('A 10 mm yellowish lipoma with a positive cushion sign was seen in the transverse colon; it was not sampled.',
     'Transverse colon: 10 mm lipoma (cushion sign +), not sampled', 'TC lipoma, not sampled', 4,
     'hard:lesion_not_sampled', 'Transverse colon lipoma (not sampled).'),
    ('There was mild melanosis coli.', 'Melanosis coli', 'melanosis', 1, None, 'Melanosis coli.'),
]


def boston_text(r, t, l, fmt):
    s = r + t + l
    if fmt == 0:
        x = f'BBPS {r}-{t}-{l} (total {s})'
    elif fmt == 1:
        x = f'Boston Bowel Preparation Scale: right colon {r}, transverse colon {t}, left colon {l}; total {s}/9'
    elif fmt == 2:
        x = f'BBPS: R{r} T{t} L{l} = {s}'
    elif fmt == 3:
        x = f'BBPS {s} ({r}/{t}/{l}, R/T/L)'
    elif fmt == 4:
        x = f'Boston score {s}/9 [Rt {r} | Tr {t} | Lt {l}]'
    elif fmt == 5:
        x = f'BBPS (L/T/R): {l}/{t}/{r}, total {s}'
    elif fmt == 6:
        x = f'BBPS {s}/9'
    else:
        x = None
    if fmt == 7:
        lines = ['Boston Bowel Prep Scale', f'Right: {r}', f'Transverse: {t}', f'Left: {l}', f'Total: {s}']
        return lines, 'Boston Bowel Preparation Scale: right ' + str(r) + ', transverse ' + str(t) + ', left ' + str(l) + ', total ' + str(s) + '.', f'BBPS {r}/{t}/{l}={s}'
    return [x], x + '.', x


# ----------------------------------------------------------------- one report
def build_report(idx, s):
    tags = []

    def tag(t):
        if t not in tags:
            tags.append(t)

    style = wch([('kv', 25), ('headed', 22), ('dictation', 18), ('terse', 12), ('table', 11), ('form', 10)])
    if style == 'form' and s['correction']:
        style = ch(['kv', 'headed'])
    us = p(0.35)
    arabic = p(0.16)
    noise = p(0.13)
    typo = p(0.08)
    tag('fmt:' + style)
    H = p(0.22)   # this report may carry optional hard cases
    tag('spelling:us' if us else 'spelling:uk')

    age = R.randint(45, 76)
    sex = ch(['M', 'F'])
    sexword = 'man' if sex == 'M' else 'woman'
    proc_date = datetime.date(2024, 1, 2) + datetime.timedelta(days=R.randint(0, 640))
    dfmt = ch(DATE_FMTS)
    ds = proc_date.strftime(dfmt)
    tm = f'{R.randint(7, 16):02d}:{ch(["00", "10", "15", "25", "30", "40", "45", "55"])}'

    def later(a, b):
        return (proc_date + datetime.timedelta(days=R.randint(a, b))).strftime(dfmt)

    doc = ch(DOCTORS)
    unit = ch(UNITS)
    scope = ch(SCOPES)
    mrn = f'{ch(["AD", "MRN", "UID"])}-{R.randint(100000, 999999)}'

    cdoc, pdoc, corr = s['caecum_doc'], s['prep_doc'], s['correction']
    disp_c = {'caecum_to_no': 'yes', 'caecum_to_yes': 'no'}.get(corr, cdoc)
    disp_p = 'adequate' if corr == 'prep_to_poor' else pdoc

    # ---- extent planning
    reasons = [('fixed angulation in the sigmoid colon', 'fixed angulation'),
               ('recurrent looping', 'looping'),
               ('patient discomfort despite sedation', 'discomfort'),
               ('a narrow, diverticular sigmoid colon', 'narrow divertic. sig')]
    stop = reason = None
    if cdoc == 'no':
        if pdoc == 'inadequate' and corr is None and p(0.45):
            reason = ('solid faecal residue obscuring the lumen', 'solid stool')
            stop = ch(STOPS)
        else:
            reason = ch(reasons)
            if 'diverticular' in reason[0]:
                stop = 'sigmoid colon'
            elif 'sigmoid' in reason[0]:
                stop = ch(['sigmoid colon', 'descending colon'])
            else:
                stop = ch(STOPS)
        maxidx = SEG.index(stop)
    elif cdoc == 'yes':
        maxidx = 7
    else:
        maxidx = 6
    ext_var = None
    implied = False
    if cdoc == 'yes' and corr is None:
        if style != 'form' and H and p(0.18):
            implied = True
        else:
            pool = [('plain', 30), ('ti', 16), ('landmarks', 16), ('transillum', 10)]
            if H:
                pool += [('difficult', 12), ('ti_not', 12), ('reinsert', 8)]
            ext_var = wch(pool)
    elif corr == 'caecum_to_no':
        ext_var = 'plain'
    elif corr == 'caecum_to_yes':
        ext_var = 'shown_sigmoid'
    ns_var = None
    if cdoc == 'not_stated':
        if style == 'form':
            ns_var = ch(['blank', 'omit'])
        else:
            ns_var = wch([('omit', 25), ('vague', 30), ('audit', 20), ('scope_only', 25)]) if H else 'omit'
        tag('extent:unstated')

    # ---- tissue planning
    kind = s['tissue_kind']
    allowed_sites = SEG[:maxidx + 1]

    def pick_site():
        return R.choices(allowed_sites, [SEG_W[x] for x in allowed_sites])[0]

    polyps, biopsies, left_polyp = [], [], None
    if kind in ('polyp', 'both'):
        for _ in range(wch([(1, 55), (2, 30), (3, 15)])):
            polyps.append(make_polyp(pick_site(), 0.35 if H else 0.0))
    if kind in ('biopsy', 'both'):
        opts = [b for b in biopsy_types(maxidx) if b['minidx'] <= maxidx]
        if not (cdoc == 'yes' and corr is None and not implied):
            opts = [b for b in opts if b['key'] != 'ti']
        b = ch(opts)
        biopsies.append(b)
        if b['key'] == 'ti':
            ext_var = 'ti'
    if H and ((kind in (None, 'biopsy') and p(0.4)) or (kind == 'polyp' and p(0.12))):
        left_polyp = dict(site=ch(['rectum', 'sigmoid colon']), size=ch([2, 3, 4]))
    if implied:
        tag('hard:caecum_implied_by_finding')
        if polyps:
            polyps[0] = make_polyp('caecum', 0.0)
    if kind:
        tag('tissue:' + {'polyp': 'polypectomy_only', 'biopsy': 'biopsy_only', 'both': 'polypectomy_and_biopsy'}[kind])
    else:
        tag('tissue:none')

    others = []
    for f in OTHER_FINDINGS:
        if f[3] <= maxidx and p((0.15 if H else 0.0) if f[4] else 0.11):
            others.append(f)
            if f[4]:
                tag(f[4])
    if implied and not polyps:
        others.insert(0, ('The appendiceal orifice appeared normal.', 'Appendiceal orifice: normal',
                          'AO nl', 7, None, None))

    # ---- indication
    ind_kind = wch([('fit', 66), ('surv', 24), ('fh', 10)])
    val = R.randint(20, 400)
    if ind_kind == 'fit':
        ind_kv = ch(['Positive FIT (national bowel cancer screening programme)', 'FIT positive - screening referral',
                     f'Positive faecal immunochemical test ({val} µg Hb/g faeces)', 'Abnormal FIT, asymptomatic',
                     'FIT+ screening'])
        ind_clause = ch([f'was referred after a positive faecal immunochemical test ({val} µg/g) on screening',
                         'was referred from primary care with a positive FIT'])
        ind_terse = ch(['FIT+ screen', f'FIT+ ({val})', '+ve FIT'])
        ind_ar = 'نتيجة إيجابية لفحص الدم الخفي في البراز'
    elif ind_kind == 'surv':
        yr = R.randint(2018, 2022)
        ind_ar = 'متابعة دورية بعد استئصال سلائل سابقة'
        if H and p(0.5):
            n = R.randint(1, 4)
            pl = n > 1
            ind_kv = (f'Surveillance. Colonoscopy {yr}: {n} polyp{"s" if pl else ""} removed; histology '
                      f'tubular adenoma{"s" if pl else ""} with low-grade dysplasia.')
            ind_clause = (f'attends for surveillance; at colonoscopy in {yr} {n} polyp{"s were" if pl else " was"} '
                          f'removed and histology showed tubular adenoma{"s" if pl else ""} with low-grade dysplasia')
            ind_terse = f'Surv (TA x{n}, {yr})'
            tag('hard:prior_histology_in_indication')
        else:
            ind_kv = f'Surveillance - previous adenomas ({yr})'
            ind_clause = f'attends for polyp surveillance (adenomas removed in {yr})'
            ind_terse = f'Surv, adenomas {yr}'
    else:
        ind_kv = 'Family history of colorectal cancer (first-degree relative diagnosed <60 y)'
        ind_clause = 'was referred for screening because of a family history of colorectal cancer in a first-degree relative'
        ind_terse = 'FHx CRC'
        ind_ar = 'تاريخ عائلي لسرطان القولون'
    r = R.random() if H else 1.0
    if r < 0.12:
        y2 = R.randint(2022, 2024)
        ind_kv += f'; previous colonoscopy ({y2}) incomplete to the splenic flexure because of looping'
        ind_clause += f'; a previous colonoscopy in {y2} was incomplete to the splenic flexure because of looping'
        ind_terse += f'; prev scope {y2} incomplete (SF)'
        tag('hard:prior_incomplete_in_indication')
    elif r < 0.24:
        ind_kv += '; repeat of an earlier attempt that was abandoned because of poor bowel preparation'
        ind_clause += '; an earlier attempt was abandoned because of poor bowel preparation'
        ind_terse += '; prev attempt abandoned (poor prep)'
        tag('hard:prior_poor_prep_in_indication')
    if any(b['diarrhoea'] for b in biopsies):
        ind_kv += '; also 6 months of watery diarrhoea'
        ind_clause += ' and also reports six months of watery diarrhoea'
        ind_terse += '; chronic diarrhoea'

    # ---- prep
    product = ch(PRODUCTS)
    prep_kv = prep_prose = prep_terse = None
    prep_form = None      # ('tick', word) / ('boston', r,t,l) / ('other', text) / ('blank',) / None
    prep_ar = None
    prep_disp_short = None
    if disp_p == 'not_stated':
        tag('prep:ungraded')
        if style == 'form':
            ung = ch(['blank', 'product']) if H else 'product'
        else:
            pool = [('product', 40), ('omit', 18)]
            if H:
                pool += [('compliance', 30), ('deferred', 18)]
                if style in ('kv', 'table'):
                    pool.append(('blank', 18))
            ung = wch(pool)
        if ung == 'product':
            prep_kv, prep_prose, prep_terse = [product], f'Bowel preparation was with {product}.', product.split(' (')[0]
        elif ung == 'compliance':
            prep_kv = [f'{product} - taken as instructed; patient reports clear effluent']
            prep_prose = f'The patient took {product} as instructed and reported clear effluent.'
            prep_terse = f'{product.split(" (")[0]}, pt reports clear effluent'
            tag('hard:prep_compliance_not_grade')
        elif ung == 'deferred':
            prep_kv = [f'{product}; quality - see nursing pre-procedure checklist']
            prep_prose = f'Bowel preparation: {product}; quality documented in the nursing record.'
            prep_terse = f'{product.split(" (")[0]} (see nursing notes)'
            tag('hard:prep_deferred_to_other_record')
        elif ung == 'blank':
            prep_kv = [f'Agent: {product}', 'Quality: ________']
            prep_prose = f'Bowel preparation: {product}. Quality: ________'
            prep_terse = product
            tag('hard:blank_field')
        elif ung == 'omit':
            pass
        if style == 'form':
            prep_form = ('blank',) if ung == 'blank' else None
            if ung == 'blank':
                tag('hard:checkbox_blank')
    else:
        shown_inadequate = (disp_p == 'inadequate')
        if style == 'form':
            fmt_kind = wch([('verbal', 70), ('boston', 30)])
        elif shown_inadequate:
            fmt_kind = wch([('verbal', 42), ('boston', 50), ('other', 8)])
        else:
            fmt_kind = wch([('verbal', 62), ('boston', 30), ('other', 8)])
        if cdoc == 'no' and not shown_inadequate and fmt_kind == 'boston':
            fmt_kind = 'verbal'
        if fmt_kind == 'verbal':
            tag('prep:verbal')
            if not shown_inadequate:
                w = ch(['excellent', 'good'] + (['fair'] if H else [])) if style == 'form' else ch(['excellent', 'good', 'good', 'good', 'adequate', 'adequate', 'excellent'] + (['fair', 'fair'] if H else []))
                if corr == 'prep_to_poor':
                    w = 'good'
                if w == 'fair':
                    tag('hard:prep_fair_is_adequate')
                extra = {'excellent': ' - clean mucosa, minimal clear fluid',
                         'good': ' (small volume of clear fluid, easily aspirated)',
                         'adequate': ' with some washing required',
                         'fair': ' - some semi-solid residue cleared with washing; mucosa adequately seen'}[w]
                prep_kv = [f'{product}. Quality: {cap(w)}']
                prep_prose = ch([f'Bowel preparation ({product}) was {w}.',
                                 f'The quality of the bowel preparation ({product}) was {w}{extra}.'])
                prep_terse = f'{product.split(" (")[0]}, {w}'
                prep_form = ('tick', cap(w))
                prep_ar = f'تحضير الأمعاء {AR_GRADE[w]}'
                prep_disp_short = w
            else:
                if cdoc == 'yes' and style != 'form' and H and p(0.4):
                    prep_kv = [f'{product}. Left colon good, right colon poor (solid stool) - overall inadequate']
                    prep_prose = (f'Bowel preparation with {product} was good in the left colon but poor in the right '
                                  f'colon with solid stool; overall inadequate.')
                    prep_terse = f'{product.split(" (")[0]}: L good / R poor -> inadequate overall'
                    tag('hard:segmental_mixed_prep')
                else:
                    w = ch(['poor', 'inadequate'])
                    extra = ch([' - solid stool that could not be cleared', ' with semi-solid stool coating the mucosa',
                                ' - unable to exclude lesions larger than 5 mm'])
                    prep_kv = [f'{product}. Quality: {cap(w)}{extra}']
                    prep_prose = f'The bowel preparation ({product}) was {w}{extra}.'
                    prep_terse = f'{product.split(" (")[0]}, {w}'
                prep_form = ('tick', 'Poor')
                prep_ar = 'تحضير الأمعاء ضعيف وغير كافٍ'
        elif fmt_kind == 'boston':
            tag('prep:boston')
            if not shown_inadequate:
                segs = tuple(R.choices([2, 3], [4, 6])[0] for _ in range(3))
            else:
                if cdoc == 'no':
                    segs = ch([x for x in BOSTON_LOW_TOTAL if x[0] <= 1])
                elif p(0.35):
                    segs = ch(BOSTON_LOW_TOTAL)
                else:
                    segs = ch(BOSTON_TOTAL_OK_SEG_LOW)
            if corr == 'prep_to_poor':
                segs = (2, 2, 3)
            rr, tt, ll = segs
            tot = rr + tt + ll
            fmts = [0, 1, 2, 3, 4, 7] + (([5] + ([6] if (tot >= 8 or tot < 6) else [])) if H else [])
            fmt = ch(fmts)
            if fmt == 5:
                tag('hard:bbps_reversed_segment_order')
            if fmt == 6:
                tag('hard:bbps_total_only')
            if shown_inadequate and tot >= 6:
                tag('hard:bbps_total_ok_segment_low')
            elif shown_inadequate:
                tag('prep:boston_total_below_6')
            kvl, pr, te = boston_text(rr, tt, ll, fmt)
            prep_kv = [product] + kvl
            prep_prose = f'Bowel preparation was with {product}. {pr}'
            prep_terse = te
            prep_form = ('boston', rr, tt, ll)
            prep_ar = ('تحضير الأمعاء غير كافٍ' if shown_inadequate else 'تحضير الأمعاء مقبول') + f' (BBPS {tot})'
            prep_disp_short = f'BBPS {rr}-{tt}-{ll}'
        else:
            tag('prep:other_scale_with_verbal')
            if not shown_inadequate:
                n, w = ch([(1, 'excellent'), (2, 'good'), (3, 'fair')])
                prep_ar = f'تحضير الأمعاء {AR_GRADE[w]}'
            else:
                n, w = ch([(4, 'poor'), (5, 'inadequate')])
                prep_ar = 'تحضير الأمعاء ضعيف وغير كافٍ'
            prep_kv = [f'{product}. Aronchick scale {n} ({w})']
            prep_prose = f'Bowel preparation with {product} was {w} (Aronchick scale {n}).'
            prep_terse = f'Aronchick {n} ({w})'
            prep_form = ('other', f'Aronchick {n} ({w})')

    # ---- sedation, withdrawal, complications
    sed_kv, sed_prose, sed_terse = ch(SEDATION)
    wt = None
    if p(0.5 if cdoc == 'not_stated' else 0.78) or (style == 'form' and cdoc != 'not_stated'):
        m = R.randint(6, 14)
        sec_ = R.choice([0, 15, 30, 45])
        if p(0.2):
            wt_kv = f'{m}:{sec_:02d} min'
        else:
            wt_kv = f'{m} min'
        wt = dict(kv=wt_kv, prose=f'The withdrawal time was {m} minutes.', terse=f'WT {m}m', m=m)
        if cdoc == 'not_stated' and ns_var != 'blank':
            tag('hard:withdrawal_time_without_extent')
    comp_kind = 'none'
    if polyps and p(0.07):
        comp_kind = 'bleed'
    elif p(0.04):
        comp_kind = 'desat'
    comp = {'none': (ch(['None', 'Nil', 'No immediate complications']),
                     ch(['There were no immediate complications.', 'No complications.']), 'nil'),
            'bleed': ('Minor immediate bleeding at polypectomy site; haemostasis with one clip',
                      'There was minor immediate bleeding at a polypectomy site, controlled with one clip.',
                      'minor bleed, clip x1'),
            'desat': ('Transient desaturation to 89%, resolved with supplemental oxygen',
                      'There was a transient desaturation to 89%, which resolved with supplemental oxygen.',
                      'transient desat, O2')}[comp_kind]

    # ---- extent texts
    n_ti = R.randint(3, 10)
    EXT_YES = {
        'plain': ('Caecum (appendiceal orifice and ICV identified, photo-documented)',
                  'The colonoscope was advanced to the caecum, identified by the appendiceal orifice and ileocaecal valve; photo-documentation was obtained.',
                  'To caecum, AO/ICV photo\'d'),
        'ti': (f'Terminal ileum (intubated {n_ti} cm)',
               f'The colonoscope was advanced to the caecum and the terminal ileum was intubated for about {n_ti} cm.',
               f'TI intubated {n_ti}cm'),
        'landmarks': ('Complete to caecum - landmarks: appendiceal orifice, ileocaecal valve, tri-radiate fold; photos: yes',
                      'Caecal intubation was confirmed by visualisation of the appendiceal orifice, ileocaecal valve and tri-radiate fold, with photographs taken.',
                      'Caecum: AO+ICV+TRF, pics'),
        'transillum': ('Caecum (RIF transillumination, appendiceal orifice photographed)',
                       'The caecum was reached (transillumination in the right iliac fossa; appendiceal orifice photographed).',
                       'Caecum (transillum, AO pic)'),
        'difficult': ('Caecum - reached after abdominal pressure and position change (difficult, recurrent looping)',
                      'Insertion was difficult because of recurrent sigmoid looping; with abdominal pressure and a change to the supine position the caecum was eventually reached and the appendiceal orifice photographed.',
                      'Looping++, caecum reached w/ pressure'),
        'ti_not': ('Caecum (AO + ICV seen). TI not intubated.',
                   'The caecum was reached and the ileocaecal valve and appendiceal orifice were identified. The terminal ileum was not intubated.',
                   'Caecum reached (ICV/AO), TI not intubated'),
        'reinsert': ('Caecum - not reached with adult scope; reached with paediatric colonoscope, AO photographed',
                     'The caecum could not be reached initially; after changing to a paediatric colonoscope the caecal pole was reached and the appendiceal orifice photographed.',
                     'Failed w/ adult scope -> paed scope to caecum, AO pic'),
        'shown_sigmoid': ('Sigmoid colon', 'Extent of examination: sigmoid colon.', 'Ext: sig'),
    }
    if style == 'form' and ext_var in ('difficult', 'ti_not', 'reinsert'):
        ext_var = 'plain'
    if ext_var in ('difficult',):
        tag('hard:difficult_but_complete')
    if ext_var in ('ti_not', 'reinsert'):
        tag('hard:negation_near_caecum')
    ext = None
    if ext_var:
        e = EXT_YES[ext_var]
        ext = dict(kv=e[0], prose=e[1], terse=e[2])
    elif cdoc == 'no':
        st, rs = stop, reason
        k = R.randint(0, 2)
        ext = dict(
            kv=[f'{cap(st)} - incomplete ({rs[0]})', f'Incomplete; furthest point reached: {st} ({rs[0]})',
                f'Not complete - {st}. Caecum not reached ({rs[0]}).'][k],
            prose=[f'The colonoscope could only be advanced to the {st} because of {rs[0]}; the caecum was not reached.',
                   f'Incomplete examination: the procedure was terminated at the {st} ({rs[0]}).',
                   f'Despite position changes and abdominal pressure it was not possible to advance beyond the {st} ({rs[0]}). Caecal intubation was not achieved.'][k],
            terse=ch([f'Incomplete, to {SEG_SHORT[st]} only ({rs[1]})', f'Caecum NOT reached; max {SEG_SHORT[st]} ({rs[1]})']))
    elif ns_var == 'vague':
        ext = dict(kv='Colonoscope inserted; colon examined on withdrawal',
                   prose='The colonoscope was inserted and the colon was examined on withdrawal.',
                   terse='Scope in, colon examined on w/d', label='vague')
        tag('hard:vague_extent')
    elif ns_var == 'scope_only':
        ext = dict(kv=f'{scope}; insertion uneventful',
                   prose=f'A {scope} was used; insertion was uneventful.',
                   terse=f'{scope.split()[-1]}, insertion uneventful', label='vague')
        tag('hard:vague_extent')
    elif ns_var == 'audit':
        tag('hard:audit_stat_not_this_procedure')

    # ---- findings / polyps / specimens
    pot_style = ch(['letter', 'number'])
    specimens = []

    def potname():
        i = len(specimens)
        return 'ABCDEFG'[i] if pot_style == 'letter' else str(i + 1)

    f_prose, f_kv, f_terse = [], [], []     # non-polyp findings (incl. biopsy lesions)
    pol_prose, pol_kv, act_kv, pol_terse = [], [], [], []
    optical_any = False
    for pl in polyps:
        pot = potname()
        mp, mk, ma = METHODS[pl['method']]
        if pl['optical']:
            optical_any = True
        opt = f' ({pl["optical"]})' if pl['optical'] else ''
        site = pl['site']
        pol_prose.append(ch([
            f'In the {site} there was a {pl["size"]} mm {pl["morph"]} polyp{opt}, removed by {mp} and retrieved (pot {pot}).',
            f'A {pl["size"]} mm {pl["morph"]} polyp{opt} in the {site} was resected with {mp} and retrieved for histology (pot {pot}).']))
        pol_kv.append(f'{cap(site)}: {pl["size"]} mm, {pl["morph"]}{opt}')
        act_kv.append(f'{mk} - {site} polyp, retrieved (pot {pot})')
        pol_terse.append(f'{pl["size"]}mm {pl["morph_short"]} {SEG_SHORT[site]} polyp{" ("+pl["optical"]+")" if pl["optical"] else ""} -> {ma}, retr')
        specimens.append((pot, f'{site} polyp, {pl["size"]} mm', pl['dx']))
    for b in biopsies:
        pot = potname()
        f_prose.append(f'{b["prose"]} (pot {pot}).')
        f_kv.append(b['find'])
        act_kv.append(f'{b["act"]} (pot {pot})')
        f_terse.append(b['terse'])
        specimens.append((pot, b['spec'], b['dx']))
    if left_polyp:
        ls, lz = left_polyp['site'], left_polyp['size']
        pol_prose.append(f'A {lz} mm diminutive, hyperplastic-appearing (NICE 1) polyp in the {ls} was left in situ.')
        pol_kv.append(f'{cap(ls)}: {lz} mm, flat (Paris 0-IIa), NICE 1 - not removed, left in situ')
        pol_terse.append(f'{lz}mm HP-looking {SEG_SHORT[ls]} polyp, left in situ')
        if not s['tissue']:
            tag('hard:polyp_seen_not_removed')
    for f in others:
        f_prose.append(f[0])
        f_kv.append(f[1])
        f_terse.append(f[2])
    if biopsies and not polyps and not left_polyp and ((H and p(0.6)) or style in ('kv', 'table', 'form')):
        tag('hard:no_polyps_but_biopsy')
        if style not in ('kv', 'table', 'form'):
            f_prose.insert(0, 'No polyps were seen.')
            f_kv.insert(0, 'No polyps')
            f_terse.insert(0, 'no polyps')
    if optical_any and not s['histo']:
        tag('hard:optical_diagnosis_not_histology')

    # negations
    neg = None
    if not s['tissue']:
        if p(0.5):
            neg = ch([('None', 'No biopsies were taken and no polypectomy was performed.', 'no bx/polypectomy'),
                      ('Nil - no tissue sampled', 'No tissue was sampled.', 'nil sampled'),
                      ('No biopsies taken', 'Biopsies were not taken as the mucosa was normal.', 'no bx'),
                      ('Not applicable - no polyps removed, no biopsies', 'No polyps were removed and no biopsies were taken.', 'no specimens')])
            tag('negated_tissue_statement')
    elif kind == 'polyp' and all(pl['method'] != 'cold_forceps' for pl in polyps) and H and p(0.5):
        neg = ('No biopsies taken', 'No other biopsies were taken.', 'no bx')
        tag('hard:negation_with_tissue')

    pending = None
    if s['tissue'] and p((0.5 if H else 0.0) if s['histo'] else 0.55):
        pending = ch([('Histology pending', 'Histology is pending.', 'histo pending'),
                      ('Specimens sent to histopathology - results to follow', 'Specimens were sent to histopathology; results to follow.', 'sent to path'),
                      ('Path: awaited', 'Pathology is awaited.', 'path awaited')])
        tag('histology_pending_stated')
        if s['histo']:
            tag('hard:stale_pending_with_result_appended')

    # ---- impression / recommendations
    imp = []
    if cdoc == 'no' and corr is None and p(0.85):
        imp.append(f'Incomplete colonoscopy (to the {stop}).')
    if disp_p == 'inadequate' and p(0.5):
        imp.append('Inadequate bowel preparation - small lesions may have been missed.')
    if polyps:
        imp.append(f'{len(polyps)} polyp{"s" if len(polyps) > 1 else ""} removed '
                   f'({", ".join(sorted(set(pl["site"] for pl in polyps), key=SEG.index))}).')
        if optical_any and p(0.5):
            imp.append('Probable adenoma(s) on optical assessment.')
    for b in biopsies:
        imp.append({'random': 'Normal-appearing mucosa; random biopsies taken.',
                    'ulcer': 'Solitary rectal ulcer - biopsied.',
                    'erythema': 'Patchy sigmoid erythema - biopsied.',
                    'nodule': 'Small descending colon nodule - biopsied.',
                    'transverse': 'Focal transverse colitis - biopsied.',
                    'ti': 'Terminal ileal aphthous ulceration - biopsied.'}[b['key']])
    if left_polyp:
        imp.append('Diminutive hyperplastic-appearing polyp left in situ.')
    for f in others:
        if f[5]:
            imp.append(f[5])
    if not imp:
        if disp_c == 'yes':
            imp.append('Normal colonoscopy to the caecum.')
        elif cdoc == 'not_stated':
            imp.append(ch(['Normal colonoscopy.', 'No significant abnormality detected.']))
            tag('hard:normal_exam_no_extent')
        else:
            imp.append('No abnormality in the segments examined.')
    rec = []
    if cdoc == 'no' and corr is None:
        rec.append(ch(['CT colonography to complete assessment of the proximal colon.',
                       'Repeat colonoscopy under propofol sedation with an experienced endoscopist.']))
    if disp_p == 'inadequate':
        rec.append('Repeat colonoscopy within 3 months with a 2-day low-residue diet and split-dose preparation.')
    if s['tissue']:
        rec.append(ch(['Await histology.', 'Surveillance interval to be decided once histology is available.',
                       'Review histology in clinic in 4 weeks.']))
    if not rec:
        rec.append('Next surveillance colonoscopy in 5 years.' if ind_kind == 'surv'
                   else 'No further colonoscopy required for 10 years; return to the screening programme.')

    # ---- appended blocks (histology, addenda)
    tail_blocks = []
    if s['histo']:
        tag('histology_result_appended')
        hd = later(4, 12)
        labno = f'H{proc_date.year % 100}-{R.randint(1000, 9999)}'
        path = ch(PATHOLOGISTS)
        fmt = ch([0, 1, 2] + ([3] if arabic else []))
        if fmt == 0:
            lines = ['HISTOPATHOLOGY REPORT (appended ' + hd + ')', f'Lab no: {labno}']
            for pot, lab, dx in specimens:
                lines += [f'Specimen {pot}: {lab}', f'  Diagnosis: {dx}']
            lines.append(f'Reported by: {path}, Consultant Histopathologist')
        elif fmt == 1:
            lines = [f'Histology (reported {hd}, {labno}): ' + ' '.join(f'{pot}) {lab}: {dx}' for pot, lab, dx in specimens)]
        elif fmt == 2:
            lines = [f'ADDENDUM {hd} - pathology received. ' + '; '.join(f'pot {pot} ({lab}) - {dx[:-1]}' for pot, lab, dx in specimens) + '.']
        else:
            lines = [f'نتيجة الأنسجة / PATHOLOGY RESULT ({hd})'] + [f'{pot}. {lab}: {dx}' for pot, lab, dx in specimens]
        tail_blocks.append('\n'.join(lines))
    if corr == 'caecum_to_no':
        tail_blocks.append(f'ADDENDUM ({later(0, 3)}) - {doc}: CORRECTION. The extent above was recorded as caecum '
                           f'because of a template default. The examination in fact reached the {stop} only '
                           f'({reason[0]}); the caecum was NOT reached. CT colonography has been requested.')
        tag('hard:correction_addendum_caecum')
    elif corr == 'caecum_to_yes':
        tail_blocks.append(f'ADDENDUM ({later(0, 3)}) - {doc}: correction to the extent field, which was '
                           f'auto-populated in error. The examination was complete to the caecum; the appendiceal '
                           f'orifice and ileocaecal valve were identified and photographed.')
        tag('hard:correction_addendum_caecum')
    elif corr == 'prep_to_poor':
        orig = prep_disp_short or 'good'
        tail_blocks.append(f'ADDENDUM {later(0, 2)} - CORRECTION: bowel preparation was entered as "{orig}" in error. '
                           f'The preparation was poor, with solid stool in the ascending colon and caecum '
                           f'(BBPS 1-2-2, total 5). Repeat colonoscopy with extended preparation is advised.')
        tag('hard:correction_addendum_prep')
    elif H and p(0.12):
        tail_blocks.append(f'Addendum ({later(0, 5)}): referring clinic corrected to {ch(CLINICS)}. '
                           f'No other changes to this report.')
        tag('hard:addendum_nonkey')
    R.shuffle(tail_blocks)

    # ---- Arabic
    ar_mode = None
    ar_lines = []
    if arabic:
        tag('lang:arabic')
        if style in ('kv', 'headed', 'table'):
            ar_mode = wch([('bilingual', 50), ('summary', 30), ('header', 20)])
        else:
            ar_mode = wch([('summary', 65), ('header', 35)])
        if ar_mode == 'summary':
            ar_lines.append('الملخص باللغة العربية:')
            ar_lines.append(f'دواعي الإجراء: {ind_ar}.')
            if corr is None:
                if cdoc == 'yes':
                    ar_lines.append('تم الوصول إلى الأعور مع توثيق العلامات التشريحية.')
                elif cdoc == 'no':
                    ar_lines.append(f'لم يتم الوصول إلى الأعور (فحص غير مكتمل).')
                if prep_ar:
                    ar_lines.append(prep_ar + '.')
            if not s['tissue']:
                ar_lines.append('لم تؤخذ أي خزعات ولم يتم استئصال أي سلائل.')
                tag('negated_tissue_statement')
            else:
                if polyps:
                    ar_lines.append('تم استئصال سلائل وإرسالها للفحص النسيجي.')
                if biopsies:
                    ar_lines.append('تم أخذ خزعات وإرسالها للفحص النسيجي.')
                if not s['histo']:
                    ar_lines.append('بانتظار نتيجة التشريح المرضي.')
            if comp_kind == 'none':
                ar_lines.append('المضاعفات: لا يوجد.')

    # ---- labels
    case_mode = ch(['as', 'as', 'upper'])
    ar_first = p(0.4)
    labsel = {k: ch(v[0]) for k, v in LABELS.items()}
    if ext and ext.get('label') == 'vague':
        labsel['extent'] = ch(['Procedure', 'Technique', 'Insertion'])
    elif disp_c == 'yes' and p(0.25):
        labsel['extent'] = 'Caecal intubation'

    def label(k):
        lab = labsel[k]
        if case_mode == 'upper':
            lab = lab.upper()
        if ar_mode == 'bilingual':
            lab = f'{LABELS[k][1]} | {lab}' if ar_first else f'{lab} / {LABELS[k][1]}'
        return lab

    # ---- section bodies (kv lists / prose strings / terse strings)
    sec = collections.OrderedDict()
    sec['indication'] = dict(kv=[ind_kv], prose=cap(ind_kv) + ('' if ind_kv.endswith('.') else '.'), terse=ind_terse)
    if prep_kv:
        sec['prep'] = dict(kv=prep_kv, prose=prep_prose, terse=prep_terse)
    sec['sedation'] = dict(kv=[sed_kv], prose=sed_prose, terse=sed_terse)
    if ext:
        sec['extent'] = dict(kv=[ext['kv']], prose=ext['prose'], terse=ext['terse'])
    if wt:
        sec['withdrawal'] = dict(kv=[wt['kv']], prose=wt['prose'], terse=wt['terse'])
    find_kv = f_kv[:] if f_kv else [ch(['Normal mucosa', 'No mucosal abnormality'])]
    all_prose = f_prose + pol_prose
    find_prose = ' '.join(all_prose) if all_prose else ch(['The colonic mucosa appeared normal.', 'No mucosal abnormality was seen.'])
    sec['findings'] = dict(kv=find_kv, prose=find_prose, terse='; '.join(pol_terse + f_terse) or 'NAD')
    sec['polyps'] = dict(kv=pol_kv if pol_kv else [ch(['None', 'Nil', 'No polyps'])], prose=None, terse=None)
    iv_kv = act_kv[:]
    iv_prose = []
    iv_terse = []
    if specimens:
        iv_prose.append('Specimens: ' + '; '.join(f'pot {pot} - {lab}' for pot, lab, _ in specimens) + '.')
    if neg:
        iv_kv.append(neg[0])
        iv_prose.append(neg[1])
        iv_terse.append(neg[2])
    if pending:
        iv_kv.append(pending[0])
        iv_prose.append(pending[1])
        iv_terse.append(pending[2])
    if iv_kv:
        sec['interventions'] = dict(kv=iv_kv, prose=' '.join(iv_prose) if iv_prose else '', terse='; '.join(iv_terse))
    sec['complications'] = dict(kv=[comp[0]], prose=comp[1], terse=comp[2])
    sec['impression'] = dict(kv=imp, prose=' '.join(imp), terse='; '.join(x.rstrip('.') for x in imp))
    sec['recommendations'] = dict(kv=rec, prose=' '.join(rec), terse='; '.join(x.rstrip('.') for x in rec))

    order = ['indication', 'prep', 'sedation', 'extent', 'withdrawal', 'findings', 'polyps',
             'interventions', 'complications', 'impression', 'recommendations']
    if p(0.3):
        order.remove('sedation')
        order.insert(1, 'sedation')
    if p(0.2):
        order.remove('prep')
        order.insert(order.index('withdrawal') + 1, 'prep')
    if p(0.2):
        order.remove('impression')
        order.insert(1, 'impression')
    if p(0.15):
        order.remove('withdrawal')
        order.insert(order.index('findings') + 1, 'withdrawal')

    footer = []
    if ns_var == 'audit' or (cdoc != 'not_stated' and p(0.04)):
        footer.append(f'Endoscopist quality indicators (rolling 12 months): caecal intubation rate {R.randint(92, 98)}%, '
                      f'adenoma detection rate {R.randint(24, 41)}%.')
    footer.append(ch([f'Electronically signed: {doc}, {ds} {tm}', f'Signed: {doc}', f'Report verified by {doc} ({ds})',
                      f'Images captured: {R.randint(4, 16)}. Signed {doc}.']))

    # ---- render
    hdr_ar = 'تقرير تنظير القولون' if ar_mode in ('header', 'bilingual') else None
    if style == 'kv':
        sep = ch([': ', ':\t', ' - '])
        soft = ch(SOFTWARE)
        L = [f'{soft} - Procedure Report', f'Facility: {unit}',
             f'Patient ID: {mrn}    Age/Sex: {age}/{sex}',
             f'Procedure: Colonoscopy    Date: {ds} {tm}',
             f'Endoscopist: {doc}    Nurse: {ch(NURSES)}', f'Instrument: {scope}', '-' * 48]
        if hdr_ar:
            L.insert(0, hdr_ar)
        bullet = ch(['- ', '* ', 'num'])
        for k in order:
            if k not in sec:
                continue
            body = sec[k]['kv']
            if len(body) == 1:
                L.append(f'{label(k)}{sep}{body[0]}')
            else:
                L.append(f'{label(k)}{sep.rstrip()}')
                for j, b in enumerate(body):
                    L.append('  ' + (f'{j + 1}. ' if bullet == 'num' else bullet) + b)
        text = '\n'.join(L)
    elif style == 'headed':
        hstyle = ch(['colon', 'upper', 'md', 'underline'])
        L = [unit, 'COLONOSCOPY REPORT', f'MRN: {mrn}   Age: {age} y   Sex: {sex}',
             f'Date of procedure: {ds}', f'Endoscopist: {doc}', '']
        if hdr_ar:
            L.insert(1, hdr_ar)
        for k in order:
            if k not in sec or k == 'polyps':
                continue
            body = sec[k]['prose']
            if not body:
                continue
            lab = label(k)
            if hstyle == 'colon':
                L.append(f'{lab}:')
            elif hstyle == 'upper':
                L.append(lab.upper())
            elif hstyle == 'md':
                L.append(f'## {lab}')
            else:
                L += [lab, '-' * min(len(lab), 30)]
            L += [body, '']
        text = '\n'.join(L).rstrip()
    elif style == 'dictation':
        L = [unit if 'Endoscopy' in unit else f'{unit} - Endoscopy', f'Colonoscopy, {ds}. Dictated by {doc}; transcribed {later(0, 2)}.']
        if hdr_ar:
            L.insert(0, hdr_ar)
        p1 = [f'This {age}-year-old {sexword} {ind_clause}.', sed_prose]
        if 'prep' in sec:
            p1.append(sec['prep']['prose'])
        if 'extent' in sec:
            p1.append(sec['extent']['prose'])
        if wt:
            p1.append(wt['prose'])
        if order.index('prep') > order.index('extent') and 'prep' in sec and 'extent' in sec:
            p1[2], p1[3] = p1[3], p1[2]
        L += ['', ' '.join(p1), '', find_prose]
        p3 = ' '.join(x for x in [sec.get('interventions', {}).get('prose', ''), comp[1]] if x)
        L += ['', p3, '', 'IMPRESSION:', sec['impression']['prose'], '', 'PLAN:', sec['recommendations']['prose']]
        text = '\n'.join(L)
    elif style == 'terse':
        TL = {'indication': 'Ind', 'prep': 'Prep', 'sedation': 'Sed', 'extent': 'Ext', 'withdrawal': 'WT',
              'findings': 'Fx', 'interventions': 'Tx', 'complications': 'Comp', 'impression': 'Imp',
              'recommendations': 'Plan'}
        parts = []
        for k in order:
            if k not in sec or k == 'polyps':
                continue
            tv = sec[k]['terse']
            if not tv:
                continue
            if k == 'withdrawal':
                parts.append(tv)
            else:
                parts.append(f'{TL[k]}: {tv}')
        L = [f'COLONOSCOPY {ds} | {doc} | {mrn} | {age}{sex}']
        if hdr_ar:
            L.insert(0, hdr_ar)
        line = []
        for part in parts:
            line.append(part)
            if len(line) >= R.randint(2, 4):
                L.append('. '.join(line) + '.')
                line = []
        if line:
            L.append('. '.join(line) + '.')
        text = '\n'.join(L)
    elif style == 'table':
        tform = ch(['md', 'pipe'])
        L = [unit, f'Colonoscopy report - {ds} - {doc} - {mrn}', '']
        if hdr_ar:
            L.insert(0, hdr_ar)
        if tform == 'md':
            L += ['| Item | Detail |', '|---|---|']
        for k in order:
            if k not in sec:
                continue
            val_ = '; '.join(x.rstrip('.') for x in sec[k]['kv'])
            if tform == 'md':
                L.append(f'| {label(k)} | {val_} |')
            else:
                L.append(f'{label(k):<26}| {val_}')
        text = '\n'.join(L)
    else:  # form
        X, O = ch([('☒', '☐'), ('[X]', '[ ]'), ('(x)', '( )')])

        def box(opts, sel):
            return '  '.join(f'{X if o == sel else O} {o}' for o in opts)
        L = [unit, 'STRUCTURED COLONOSCOPY REPORT', f'MRN {mrn}   Date {ds}   Endoscopist {doc}', '']
        if hdr_ar:
            L.insert(0, hdr_ar)
        L.append(f'Indication: {ind_kv}')
        L.append(f'Bowel prep agent: {product}')
        if prep_form and prep_form[0] == 'tick':
            L.append('Prep quality: ' + box(['Excellent', 'Good', 'Fair', 'Poor'], prep_form[1]))
        elif prep_form and prep_form[0] == 'boston':
            _, rr, tt, ll = prep_form
            L.append(f'BBPS: Right [{rr}]  Transverse [{tt}]  Left [{ll}]  Total [{rr + tt + ll}]')
        elif prep_form and prep_form[0] == 'blank':
            L.append('Prep quality: ' + box(['Excellent', 'Good', 'Fair', 'Poor'], None))
        L.append(f'Sedation: {sed_kv}')
        if cdoc == 'yes':
            L.append('Caecum reached: ' + box(['Yes', 'No'], 'Yes'))
            L.append(f'Landmarks: {X} AO  {X} ICV  {X if ext_var == "ti" else O} TI intubated   Photo-documented: {X}')
        elif cdoc == 'no':
            L.append('Caecum reached: ' + box(['Yes', 'No'], 'No') + f'   Furthest point: {stop}   Reason: {reason[0]}')
        elif ns_var == 'blank':
            L.append('Caecum reached: ' + box(['Yes', 'No'], None))
            tag('hard:checkbox_blank')
        if wt:
            L.append(f'Withdrawal time: {wt["kv"]}')
        L.append('Polyps found: ' + box(['Yes', 'No'], 'Yes' if (polyps or left_polyp) else 'No')
                 + (f'   Number: {len(polyps) + (1 if left_polyp else 0)}' if (polyps or left_polyp) else ''))
        L.append('Polypectomy: ' + box(['Yes', 'No'], 'Yes' if polyps else 'No') + '     Biopsy: '
                 + box(['Yes', 'No'], 'Yes' if biopsies else 'No'))
        if not s['tissue']:
            tag('negated_tissue_statement')
        elif not (polyps and biopsies):
            tag('hard:negation_with_tissue')
        fx = pol_kv + f_kv
        L.append('Findings: ' + ('; '.join(fx) if fx else 'Normal mucosa'))
        if act_kv:
            L.append('Specimens: ' + '; '.join(act_kv))
        L.append('Complications: ' + (box(['None', 'Bleeding', 'Perforation', 'Other'], 'None') if comp_kind == 'none'
                                      else box(['None', 'Bleeding', 'Perforation', 'Other'], 'Bleeding' if comp_kind == 'bleed' else 'Other') + f' ({comp[0]})'))
        L.append('Histology: ' + box(['Sent - pending', 'N/A'], 'Sent - pending' if s['tissue'] else 'N/A'))
        if s['tissue']:
            tag('histology_pending_stated')
            if s['histo']:
                tag('hard:stale_pending_with_result_appended')
        L.append('Impression: ' + ' '.join(imp))
        L.append('Recommendations: ' + ' '.join(rec))
        text = '\n'.join(L)

    if ar_lines:
        text += '\n\n' + '\n'.join(ar_lines)
    for blk in tail_blocks:
        text += '\n\n' + blk
    text += '\n\n' + '\n'.join(footer)
    if ar_mode == 'header':
        text += '\n' + f'توقيع الطبيب: {doc}'

    # ---- overlays
    if us:
        for a, b in US_MAP:
            text = text.replace(a, b)
    if typo:
        cands = [(w, alts) for w, alts in TYPOS if w in text]
        if cands:
            tag('typo')
            for w, alts in R.sample(cands, min(len(cands), R.randint(1, 2))):
                occ = [m.start() for m in re.finditer(re.escape(w), text)]
                pos = ch(occ)
                text = text[:pos] + ch(alts) + text[pos + len(w):]
    if noise:
        tag('noise:scan')
        text = add_noise(text)

    hidden = dict(caecum_real=s['caecum_real'], prep_real=s['prep_real'], tissue=s['tissue'], histo=s['histo'])
    key_fields = dict(caecum=cdoc, prep=pdoc, polyps='yes' if s['tissue'] else 'no',
                      tissue_taken='yes' if s['tissue'] else 'no', histology='yes' if s['histo'] else 'no')
    if (key_fields['caecum'] == 'not_stated' or key_fields['prep'] == 'not_stated') and s['truth'] == 'inadequate' \
            and cdoc != 'no' and pdoc != 'inadequate':
        tag('hidden:undocumented_failure')
    return text, tags, key_fields, hidden


def add_noise(text):
    out = []
    for ln in text.split('\n'):
        if not re.search('[؀-ۿ]', ln):
            for w, nw in OCR.items():
                if w in ln and p(0.45):
                    ln = ln.replace(w, nw, 1)
            if len(ln) > 50 and p(0.3):
                ws = [m for m in re.finditer(r'[A-Za-z]{9,}', ln)
                      if not any(k in m.group(0).lower() for k in PROTECT)]
                if ws:
                    m = ch(ws)
                    cut = R.randint(3, len(m.group(0)) - 3)
                    ln = ln[:m.start() + cut] + '-\n' + ln[m.start() + cut:]
            if p(0.08):
                ln += '   '
        out.append(ln)
        if p(0.04):
            out.append(ch(['|', '. .', '~', ' ', "'"]))
    top = ch(['[Scanned document - page 1/1]', 'SCANNED COPY  -  NOT VALID WITHOUT STAMP', '~ ~ ~', '|||  |  |',
              'Fax received ' + str(R.randint(1, 28)).zfill(2) + '/' + str(R.randint(1, 12)).zfill(2) + ' 0' + str(R.randint(7, 9)) + ':' + str(R.randint(10, 59))])
    bot = ch(['~~ page 1 of 1 ~~', '[end of scan]', '- 1 -', 'Page 1/1   (copy)'])
    return top + '\n' + '\n'.join(out) + '\n' + bot


# ----------------------------------------------------------------- key / truth
def key_state(kf):
    if kf['caecum'] == 'no':
        return 'not_closed', 'incomplete_exam'
    if kf['prep'] == 'inadequate':
        return 'not_closed', 'inadequate_prep'
    if kf['caecum'] == 'not_stated' or kf['prep'] == 'not_stated':
        return 'needs_human', 'missing_documentation'
    if kf['tissue_taken'] == 'yes' and kf['histology'] == 'no':
        return 'not_closed', 'awaiting_histology'
    return 'closed', 'criteria_met'


def truth_of(h):
    if not h['caecum_real'] or not h['prep_real']:
        return 'inadequate'
    if h['tissue'] and not h['histo']:
        return 'awaiting_histology'
    return 'adequate'


# ----------------------------------------------------------------- audit checks
TICK = ('☒', '[X]', '(x)')


def lint(text, kf, tags):
    """Coarse consistency checks between rendered text and key; returns list of problems."""
    probs = []
    low = text.lower()
    lines = low.split('\n')
    if kf['caecum'] == 'not_stated':
        pat = re.compile(r'caec|cecum|cecal|ileoc|terminal ileum|appendi|\bti\b|ceacum|caceum|cecun|ceccum')
        for ln in lines:
            if 'intubation rate' in ln:
                continue
            if 'caecum reached:' in ln.replace('cecum', 'caecum') and not any(t.lower() in ln for t in TICK):
                continue
            if pat.search(ln):
                probs.append('caecum-word in not_stated: ' + ln[:80])
    if kf['prep'] == 'not_stated':
        pat = re.compile(r'bbps|boston|aronchick|excellent|\bfair\b|\bpoor\b|inadequate|\bgood\b|\badequate')
        for ln in lines:
            if 'abandoned' in ln or 'abandoned' in ln.replace('-\n', ''):
                continue
            if 'prep quality:' in ln and not any(t.lower() in ln for t in TICK):
                continue
            if pat.search(ln):
                probs.append('prep-grade word in not_stated: ' + ln[:80])
    if kf['tissue_taken'] == 'no':
        if re.search(r'snare|retrieved|\bemr\b|pot [a-g1-9]\b|biopsies x|biopsied|were taken from', low):
            probs.append('tissue words in tissue=no')
    else:
        if not re.search(r'pot [a-g1-9]\b|polypectomy|\(pot|specimens:|-> (csp|cfp|hsp|emr)|\bbx\b', low):
            probs.append('no specimen marker in tissue=yes')
    has_result = bool(re.search(r'histopathology report|histology \(reported|pathology received|pathology result', low))
    if has_result != (kf['histology'] == 'yes'):
        probs.append('histology block mismatch')
    return probs


# ----------------------------------------------------------------- main
def main():
    scen = build_scenarios()
    reports = []
    seen = set()
    for i, s in enumerate(scen):
        text, tags, kf, hidden = build_report(i, s)
        tries = 0
        while text in seen:
            tries += 1
            text += ' '  # never expected; guarantees uniqueness without changing content
        seen.add(text)
        st, rs = key_state(kf)
        reports.append(dict(uid=f't{i}', sha=hashlib.sha256(text.encode('utf-8')).hexdigest(), text=text,
                            truth=s['truth'], key_fields=kf, key_state=st, key_reason=rs, tags=tags,
                            _hidden=hidden))

    # ---------------- self-audit
    problems = []
    for r in reports:
        if truth_of(r['_hidden']) != r['truth']:
            problems.append((r['uid'], 'truth mismatch'))
        if key_state(r['key_fields']) != (r['key_state'], r['key_reason']):
            problems.append((r['uid'], 'key_state mismatch'))
        kf, h = r['key_fields'], r['_hidden']
        if kf['caecum'] != 'not_stated' and (kf['caecum'] == 'yes') != h['caecum_real']:
            problems.append((r['uid'], 'documented caecum != reality'))
        if kf['prep'] != 'not_stated' and (kf['prep'] == 'adequate') != h['prep_real']:
            problems.append((r['uid'], 'documented prep != reality'))
        if kf['polyps'] != kf['tissue_taken']:
            problems.append((r['uid'], 'polyps != tissue_taken'))
        if kf['histology'] == 'yes' and kf['tissue_taken'] == 'no':
            problems.append((r['uid'], 'histology without tissue'))
        for pr in lint(r['text'], kf, r['tags']):
            problems.append((r['uid'], pr))
    assert len({r['text'] for r in reports}) == N
    assert len({r['sha'] for r in reports}) == N

    truth_c = collections.Counter(r['truth'] for r in reports)
    state_c = collections.Counter(f"{r['key_state']}/{r['key_reason']}" for r in reports)
    tag_c = collections.Counter(t for r in reports for t in r['tags'])
    hard_n = sum(1 for r in reports if any(t.startswith('hard:') for t in r['tags']))
    quotas = {
        'ungraded prep (key prep=not_stated)': (sum(r['key_fields']['prep'] == 'not_stated' for r in reports), 40),
        'unstated extent (key caecum=not_stated)': (sum(r['key_fields']['caecum'] == 'not_stated' for r in reports), 25),
        'Boston scores': (tag_c['prep:boston'], 30),
        'Boston total>=6 with a segment <2': (tag_c['hard:bbps_total_ok_segment_low'], 1),
        'biopsies that are not polypectomies': (tag_c['tissue:biopsy_only'] + tag_c['tissue:polypectomy_and_biopsy'], 25),
        'negated tissue statements': (tag_c['negated_tissue_statement'] + tag_c['hard:negation_with_tissue'], 20),
        'Arabic text': (tag_c['lang:arabic'], 30),
        'scanning noise': (tag_c['noise:scan'], 25),
    }
    boston_sides = collections.Counter(r['key_fields']['prep'] for r in reports if 'prep:boston' in r['tags'])
    for name, (have, need) in quotas.items():
        if have < need:
            problems.append(('corpus', f'quota {name}: {have} < {need}'))
    if not (boston_sides['adequate'] and boston_sides['inadequate']):
        problems.append(('corpus', 'Boston not on both sides of threshold'))

    print('truth:', dict(truth_c))
    print('key_state:', dict(state_c))
    print('hard-case reports:', hard_n, f'({100 * hard_n / N:.1f}%)')
    print('boston sides:', dict(boston_sides))
    for name, (have, need) in quotas.items():
        print(f'  {name}: {have} (min {need})')
    print('tags:')
    for t, c in sorted(tag_c.items()):
        print(f'  {t}: {c}')
    if problems:
        print('PROBLEMS:')
        for pr in problems:
            print('  ', pr)
        raise SystemExit(1)

    for r in reports:
        del r['_hidden']
    doc = dict(source='Third independent author (Claude agent, Anthropic) - deterministic stdlib generator '
                      'gen_heldout3.py, random seed 3031, 300 synthetic Abu Dhabi colonoscopy reports, 2026-10-29',
               reports=reports)
    with open(OUT_JSON, 'w', encoding='utf-8') as f:
        json.dump(doc, f, ensure_ascii=False, indent=1)
    write_readme(truth_c, state_c, tag_c, hard_n, quotas, boston_sides)
    print('wrote', OUT_JSON)


TAG_DESC = {
    'hard:prior_histology_in_indication': 'indication quotes histology from a PRIOR colonoscopy (not this procedure)',
    'hard:prior_incomplete_in_indication': 'indication says a PREVIOUS colonoscopy was incomplete',
    'hard:prior_poor_prep_in_indication': 'indication says a PREVIOUS attempt had poor prep',
    'hard:difficult_but_complete': 'difficult/looping insertion, caecum eventually reached',
    'hard:negation_near_caecum': '"TI not intubated" or "not reached with adult scope" but caecum reached',
    'hard:caecum_implied_by_finding': 'no extent statement; caecal polyp or appendiceal orifice described',
    'hard:withdrawal_time_without_extent': 'withdrawal time given but extent never stated',
    'hard:audit_stat_not_this_procedure': 'footer quotes endoscopist caecal intubation RATE, extent not stated',
    'hard:vague_extent': '"colon examined on withdrawal" / "insertion uneventful" - no extent',
    'hard:normal_exam_no_extent': '"Normal colonoscopy" impression without any extent statement',
    'hard:bbps_total_ok_segment_low': 'Boston total >= 6 but one segment < 2 (inadequate)',
    'hard:bbps_reversed_segment_order': 'Boston written L/T/R instead of R/T/L',
    'hard:bbps_total_only': 'Boston total only (9, 8, or < 6 - all unambiguous)',
    'hard:prep_fair_is_adequate': 'verbal grade "fair" (keyed adequate)',
    'hard:segmental_mixed_prep': 'good left / poor right, overall inadequate',
    'hard:prep_compliance_not_grade': 'patient took prep / clear effluent - not a quality grade',
    'hard:prep_deferred_to_other_record': '"see nursing checklist" - not a grade',
    'hard:blank_field': 'prep quality field left blank',
    'hard:checkbox_blank': 'form checkbox row with nothing ticked',
    'hard:correction_addendum_caecum': 'addendum corrects extent (either direction)',
    'hard:correction_addendum_prep': 'addendum corrects prep from good to poor',
    'hard:addendum_nonkey': 'addendum that changes nothing relevant',
    'hard:negation_with_tissue': '"No biopsies taken" / Biopsy:No while polyps were snared (tissue=yes)',
    'hard:no_polyps_but_biopsy': '"No polyps" / Polyps: None while non-polyp biopsies were taken',
    'hard:polyp_seen_not_removed': 'diminutive polyp left in situ, nothing else sampled (tissue=no)',
    'hard:lesion_not_sampled': 'lipoma described, "not sampled"',
    'hard:optical_diagnosis_not_histology': 'NICE/"likely adenoma" optical call, no histology result',
    'hard:stale_pending_with_result_appended': 'body says histology pending, result appended later (histology=yes)',
}


def write_readme(truth_c, state_c, tag_c, hard_n, quotas, boston_sides):
    L = ['# heldout3 - third independent synthetic colonoscopy corpus', '',
         'Generated by `gen_heldout3.py` (Python 3 stdlib only, seed 3031, no LLM, no network). '
         'Written from the task brief alone, without reading the other corpora or the Wasl code.', '',
         f'300 reports, unique texts. Truth: ' + ', '.join(f'{k} {v}' for k, v in sorted(truth_c.items())) + '.',
         'Key state: ' + ', '.join(f'{k} {v}' for k, v in sorted(state_c.items())) + '.', '',
         f'Reports with at least one `hard:*` tag: {hard_n} ({100 * hard_n / N:.0f}%).', '',
         '## How it works', '',
         'Each report gets a hidden reality (caecum reached, prep adequate, tissue taken, histology back) and a '
         'documentation plan (what the text states). Truth comes from reality, the key from the text. '
         'Items left unstated mostly hide a fine procedure, but about one in five hides a failure '
         '(tag `hidden:undocumented_failure`).', '',
         '## Layouts and overlays', '',
         '- `fmt:kv` software export with key: value lines; `fmt:headed` headed prose sections; '
         '`fmt:dictation` narrative dictation; `fmt:terse` abbreviated one-liners; `fmt:table` markdown/pipe table; '
         '`fmt:form` structured checkbox form (three glyph styles).',
         '- Section order shuffled (sedation/prep/impression/withdrawal moves); label wording and case vary.',
         '- `spelling:us` vs `spelling:uk`; `lang:arabic` (bilingual headings, Arabic summary block, or Arabic title + signature); '
         '`typo` (misspelt key words such as "ceacum", "polpy"); `noise:scan` (OCR letter swaps on non-key words, '
         'hyphenated line breaks, stray marks, scan/fax banners).',
         '- Prep: verbal grades, Boston in 8 formats (incl. L/T/R order, multi-line, total-only), Aronchick with verbal grade, '
         'or ungraded (product only, compliance, "see nursing notes", blank field, omitted).',
         '- Tissue: cold/hot snare, cold forceps, EMR; non-polyp biopsies (random for diarrhoea, rectal ulcer, sigmoid erythema, '
         'descending nodule, transverse colitis, terminal ileal aphthae); pathology appended in 4 formats.', '',
         '## Counts', '']
    for name, (have, need) in quotas.items():
        L.append(f'- {name}: {have} (brief minimum {need})')
    L.append(f'- Boston by key: {dict(boston_sides)}')
    L += ['', '## Hard cases (tag: count - meaning)', '']
    for t, d in TAG_DESC.items():
        L.append(f'- `{t}`: {tag_c.get(t, 0)} - {d}')
    L += ['', '## Other tags', '']
    for t, c in sorted(tag_c.items()):
        if not t.startswith('hard:'):
            L.append(f'- `{t}`: {c}')
    L += ['', '## Keying decisions', '',
          '- A caecal polyp or "appendiceal orifice normal" with no extent statement is keyed caecum=yes (necessarily implies it).',
          '- Withdrawal time, "insertion uneventful", "normal colonoscopy" or an endoscopist audit rate alone are caecum=not_stated.',
          '- Boston total-only is used only for totals 9, 8 or < 6, where segment rule is decidable.',
          '- Corrections in addenda override the body; the key follows the corrected statement.',
          '- Diminutive hyperplastic-appearing polyps left in situ are the only unremoved polyps; nothing large is deferred, no malignancy results.',
          '- Every appended pathology result covers all specimens of the procedure.', '']
    with open(OUT_README, 'w', encoding='utf-8') as f:
        f.write('\n'.join(L))


if __name__ == '__main__':
    main()
