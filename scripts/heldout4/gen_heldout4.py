#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Held-out corpus 4: synthetic Abu Dhabi colonoscopy reports with an answer key.

Deterministic, stdlib only, no network. Running

    python3 gen_heldout4.py

writes reports_heldout4.json next to this file. Re-running produces a
byte-identical file (random seed 4049, fixed corpus date, no wall clock).

    python3 gen_heldout4.py --stats    also prints tag counts and checks

Design: each report is first given a hidden *reality* (caecum reached, prep
adequate, tissue taken, histology back). A documentation plan then decides
what the text says about each item (stated, unstated, hedged, corrected in an
addendum, written in Arabic, ...). The answer key (key_fields) is derived from
that plan, i.e. from what the text supports, never from keyword matching.
All names, units and identifiers are fictional.
"""
import hashlib
import json
import os
import random
import sys
from datetime import date, timedelta

SEED = 4049
N_REPORTS = 300
CORPUS_DATE = "2026-10-29"
OUT_NAME = "reports_heldout4.json"
SOURCE = ("Fourth independent author (Claude agent, Anthropic) - deterministic stdlib "
          "generator gen_heldout4.py, random seed 4049, 300 synthetic Abu Dhabi "
          "colonoscopy reports, " + CORPUS_DATE)

R = random.Random(SEED)


def ch(seq):
    return R.choice(seq)


def wch(options):
    """Weighted choice over [(item, weight), ...]; zero weights are skipped."""
    opts = [(o, w) for o, w in options if w > 0]
    total = sum(w for _, w in opts)
    x = R.random() * total
    acc = 0.0
    for item, w in opts:
        acc += w
        if x < acc:
            return item
    return opts[-1][0]


# ---------------------------------------------------------------------------
# Static vocab
# ---------------------------------------------------------------------------
SEGS = ["rectum", "sigmoid colon", "descending colon", "splenic flexure",
        "transverse colon", "hepatic flexure", "ascending colon", "caecum"]
SEG_AR = {"rectum": "المستقيم", "sigmoid colon": "القولون السيني",
          "descending colon": "القولون النازل", "splenic flexure": "الثنية الطحالية",
          "transverse colon": "القولون المستعرض", "hepatic flexure": "الثنية الكبدية",
          "ascending colon": "القولون الصاعد", "caecum": "الأعور"}
SEG_AB = {"rectum": "R", "sigmoid colon": "SC", "descending colon": "DC",
          "splenic flexure": "SF", "transverse colon": "TC", "hepatic flexure": "HF",
          "ascending colon": "AC", "caecum": "Caec"}

FACILITIES = [
    "Al Maqta Day Surgery Centre", "Yas Gate Endoscopy Centre",
    "Khalifa Park Day Hospital - Endoscopy Unit", "Mussafah Digestive Day Care",
    "Saadiyat Gastro Day Unit", "Al Rawdah Hospital Endoscopy Suite",
    "Shakhbout City Day Surgery Centre", "Al Reef Medical Centre - Day Case Endoscopy",
    "Corniche Road Specialty Hospital - GI Unit", "Al Ain Road Day Surgery",
]
DOCTORS = ["Dr L. Haddad", "Dr S. Menon", "Dr R. Al Mansoori", "Dr K. Osei",
           "Dr M. Farouk", "Dr J. Pereira", "Dr N. Al Zaabi", "Dr T. Iqbal",
           "Dr H. Brennan", "Dr A. Rahman"]
GPS = ["Dr P. Varghese", "Dr F. Al Hosani", "Dr E. Clarke", "Dr Y. Saleh", "Dr D. Kapoor"]

EM_MALE = [("Mohammed", "محمد"), ("Ahmed", "أحمد"), ("Khalid", "خالد"), ("Saeed", "سعيد"),
           ("Omar", "عمر"), ("Rashid", "راشد"), ("Yousef", "يوسف"), ("Hamdan", "حمدان")]
EM_FEMALE = [("Fatima", "فاطمة"), ("Aisha", "عائشة"), ("Mariam", "مريم"), ("Noura", "نورة"),
             ("Hessa", "حصة"), ("Latifa", "لطيفة"), ("Shamma", "شما"), ("Salama", "سلامة")]
EM_FAMILY = [("Al Hammadi", "الحمادي"), ("Al Ketbi", "الكتبي"), ("Al Mazrouei", "المزروعي"),
             ("Al Dhaheri", "الظاهري"), ("Al Shamsi", "الشامسي"), ("Al Nuaimi", "النعيمي"),
             ("Al Suwaidi", "السويدي"), ("Al Ameri", "العامري")]
EXPAT = [("Priya Nair", "F"), ("John Mathews", "M"), ("Maria Santos", "F"),
         ("Ravi Kumar", "M"), ("Sara Haddad", "F"), ("Imran Qureshi", "M"),
         ("Elena Popescu", "F"), ("Grace Okafor", "F"), ("Tomasz Nowak", "M"),
         ("Ahmad Rahimi", "M")]

MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

IND_GENERAL = ["Positive faecal immunochemical test (FIT)", "Iron deficiency anaemia",
               "Altered bowel habit for 3 months", "Intermittent rectal bleeding",
               "Family history of colorectal cancer (first-degree relative diagnosed at 52)",
               "Surveillance after previous adenomas", "Average-risk screening",
               "Lower abdominal pain and weight loss"]

POLYP_RES = ["tubular adenoma with low-grade dysplasia, completely excised",
             "hyperplastic polyp", "sessile serrated lesion without dysplasia",
             "tubulovillous adenoma with low-grade dysplasia; margins clear",
             "tubular adenoma, low-grade dysplasia"]
BX_RES = {
    "random": ["no evidence of microscopic colitis",
               "collagenous colitis (thickened subepithelial collagen band)",
               "lymphocytic colitis"],
    "ibd": ["chronic quiescent colitis; negative for dysplasia",
            "mild chronic active colitis; no dysplasia"],
    "erythema": ["mild active proctitis", "non-specific inflammation; no granulomas"],
    "ulcer": ["non-specific ulceration; no CMV inclusions or granulomas"],
    "stricture": ["fibrosis with chronic inflammation; no malignancy"],
    "mass": ["invasive adenocarcinoma, moderately differentiated"],
}

PENDING = ["Histology awaited.", "Histology: pending.",
           "Specimens sent to histopathology - result to follow.",
           "Await histopathology; results will be reviewed in clinic.",
           "Path pending.", "Histology to follow; will be communicated to the referrer."]

NEGATED = ["No biopsies were taken.", "No polyps seen; no tissue sampled.",
           "Polypectomy: none performed.", "Specimens sent: none.", "Biopsies: nil.",
           "Polyps: 0. Biopsies: 0.", "No biopsy was indicated and none was taken.",
           "No lesions requiring biopsy or removal; nothing sent to the laboratory."]

FILLER = [("Moderate sigmoid diverticulosis without inflammation.", "Diverticular disease", "sigmoid colon"),
          ("Small internal haemorrhoids on retroflexion.", "Haemorrhoids", "rectum"),
          ("Normal rectal mucosa on retroflexion.", None, "rectum"),
          ("Scattered left-sided diverticula.", "Diverticular disease", "descending colon"),
          ("Melanosis coli in the examined segments.", "Melanosis coli", "rectum"),
          ("Normal vascular pattern in the examined segments.", None, "rectum")]

# Base text is British English; US reports are converted with these pairs.
US_PAIRS = [("CAEC", "CEC"), ("Caec", "Cec"), ("caec", "cec"),
            ("Anaem", "Anem"), ("anaem", "anem"), ("Haem", "Hem"), ("haem", "hem"),
            ("Oesoph", "Esoph"), ("oesoph", "esoph"), ("Faec", "Fec"), ("faec", "fec"),
            ("Anaesth", "Anesth"), ("anaesth", "anesth"), ("Diarrhoea", "Diarrhea"),
            ("diarrhoea", "diarrhea"), ("Coeliac", "Celiac"), ("coeliac", "celiac"),
            ("colour", "color"), ("Tumour", "Tumor"), ("tumour", "tumor"),
            ("visualised", "visualized"), ("localised", "localized"),
            ("organised", "organized"), ("recognised", "recognized"),
            ("Centre", "Center"), ("centre", "center"), ("litre", "liter"),
            ("paediatric", "pediatric"), ("oedema", "edema"), ("behaviour", "behavior")]

LBL = {
    "indication": ("Indication", "INDICATION", "دواعي الفحص"),
    "history": ("Relevant history", "HISTORY", "التاريخ المرضي"),
    "procedure": ("Procedure", "PROCEDURE", "الإجراء"),
    "ogd": ("Upper GI endoscopy (same session)", "OGD_SAME_SESSION", "تنظير المعدة"),
    "template": ("Structured fields", "STRUCTURED_FIELDS", "حقول النموذج"),
    "extent": ("Extent of examination", "EXTENT", "مدى الفحص"),
    "prep": ("Bowel preparation", "BOWEL_PREP", "تحضير الأمعاء"),
    "metrics": ("Procedure data", "PROC_DATA", "بيانات الإجراء"),
    "findings": ("Findings", "FINDINGS", "النتائج"),
    "histology": ("Histopathology", "HISTOLOGY", "نتيجة الأنسجة"),
    "impression": ("Impression", "IMPRESSION", "الانطباع"),
    "recs": ("Recommendations", "PLAN", "التوصيات"),
    "note": ("Notes", "NOTES", "ملاحظات"),
}
HL7_CODE = {"indication": "IND", "history": "HX", "procedure": "PROC", "ogd": "OGD",
            "template": "TPL", "extent": "EXT", "prep": "PREP", "metrics": "DATA",
            "findings": "FIND", "histology": "PATH", "impression": "IMP", "recs": "PLAN",
            "note": "NOTE"}

LAYOUT_W = [("free_text", 15), ("headed", 17), ("key_value", 11), ("table", 10),
            ("form", 11), ("dictation", 11), ("hl7", 7), ("letter", 9), ("bilingual", 9)]
OCR_OK = {"free_text", "headed", "table", "form", "letter", "bilingual"}


def fmt_date(d, us, style):
    if us:
        return (f"{d.month:02d}/{d.day:02d}/{d.year}" if style == 0
                else f"{MONTHS[d.month - 1]} {d.day}, {d.year}")
    return (f"{d.day:02d}/{d.month:02d}/{d.year}" if style == 0
            else f"{d.day} {MONTHS[d.month - 1]} {d.year}")


def to_us(text):
    for a, b in US_PAIRS:
        text = text.replace(a, b)
    return text


def lc_first(s):
    if len(s) > 1 and s[0].isupper() and s[1].islower():
        return s[0].lower() + s[1:]
    return s


# ---------------------------------------------------------------------------
# Component builders
# ---------------------------------------------------------------------------
REASONS = {
    "looping": ("looping despite abdominal pressure and position change", "looping"),
    "angulation": ("a fixed, angulated sigmoid colon with severe diverticular disease",
                   "fixed SC angulation"),
    "discomfort": ("patient discomfort despite additional sedation", "pt discomfort"),
    "stricture": ("a tight, non-traversable stricture", "stricture"),
    "mass": ("an obstructing circumferential tumour", "obstructing lesion"),
    "prep": ("solid faecal residue precluding safe advancement", "poor prep"),
}

EXT_YES = [
    "The colonoscope was advanced to the caecum; the appendiceal orifice and ileocaecal valve were identified and photographed.",
    "Caecum reached. Appendiceal orifice and ileocaecal valve seen.",
    "Complete colonoscopy to the caecum with photo-documentation of caecal landmarks.",
    "Extent: caecum (appendiceal orifice photographed).",
    "Caecal intubation was achieved and confirmed by identification of the appendiceal orifice.",
    "The scope was passed to the caecal pole; landmarks confirmed and photographed.",
]
EXT_NO = [
    "The scope could not be advanced beyond the {seg} because of {reason}. The caecum was not reached.",
    "Incomplete examination: the furthest point reached was the {seg} ({reason}).",
    "Procedure terminated at the {seg} owing to {reason}; caecal intubation was not achieved.",
    "Extent of examination: {seg} only. Incomplete colonoscopy - {reason}.",
    "Furthest point reached: {seg}. Caecum not reached ({reason}).",
]
EXT_TI = ["The terminal ileum was intubated for approximately 5 cm and appeared normal.",
          "Terminal ileum intubated - normal villous pattern, no ulceration.",
          "The scope was passed into the terminal ileum, which was inspected and normal."]
EXT_ABBR_YES = ["CI achieved (AO + ICV photographed). TI not entered.",
                "Ix to caecum - AO/ICV seen, CI confirmed.",
                "CI: Y (AO, ICV ph.)."]
EXT_ABBR_NO = ["CI not achieved - scope to {ab} only ({rs}).",
               "Ix to {ab}; CI: N ({rs})."]
EXT_ANAST = ["Post right hemicolectomy: the ileocolic anastomosis was reached and the neo-terminal ileum briefly intubated; the anastomosis appeared healthy.",
             "Ileocolonic anastomosis reached (previous right hemicolectomy) - healthy, no recurrence seen."]
EXT_RECOV = ["Initial passage was limited to the splenic flexure because of looping; after abdominal pressure and a change to the supine position the caecum was reached and the appendiceal orifice identified.",
             "The first attempt stalled in the sigmoid colon; on re-insertion with a paediatric colonoscope the caecum was successfully reached (ileocaecal valve and appendiceal orifice seen)."]
EXT_BENIGN = ["No difficulty was encountered in reaching the caecum; caecal landmarks identified.",
              "Caecal intubation was not difficult; the appendiceal orifice was clearly seen.",
              "The caecum was reached with no looping and no need for abdominal pressure; landmarks photographed."]
EXT_HEDGE = ["The scope was advanced to what appeared to be the caecum; however, the appendiceal orifice and ileocaecal valve could not be confidently identified.",
             "Probably reached the caecum - landmarks not clearly seen because of pooled fluid.",
             "Extent: ?caecum (landmarks not confirmed).",
             "Caecal intubation could not be confirmed; the most proximal point reached may have been the caecum or the hepatic flexure."]
TEMPLATE_DEFAULT = ["Extent of examination: Caecum   [template default - not edited]",
                    "Caecal intubation: YES (auto-populated)",
                    "Completion: Complete to caecum <default>"]


def make_extent(mode, c, seg, rkey):
    """Return dict: text, form, body (caecum value as written in body), add, tags."""
    reason, rs = REASONS[rkey] if rkey else ("", "")
    out = {"text": "", "form": None, "body": None, "add": None, "tags": [], "template": None}
    if mode == "stated":
        if c:
            out["text"] = ch(EXT_YES)
            out["form"] = ("caecum", "yes", None)
            out["body"] = "yes"
        else:
            out["text"] = ch(EXT_NO).format(seg=seg, reason=reason)
            out["form"] = ("caecum", "no", seg)
            out["body"] = "no"
    elif mode == "unstated":
        out["form"] = ("caecum", None, None)
    elif mode == "ti":
        out["text"] = ch(EXT_TI)
        out["body"] = "yes"
        out["tags"].append("hard:ti_implies_caecum")
    elif mode == "abbrev":
        if c:
            out["text"] = ch(EXT_ABBR_YES)
            out["body"] = "yes"
        else:
            out["text"] = ch(EXT_ABBR_NO).format(ab=SEG_AB[seg], rs=rs)
            out["body"] = "no"
        out["tags"].append("hard:abbreviation_heavy")
    elif mode == "anastomosis":
        out["text"] = ch(EXT_ANAST)
        out["body"] = "yes"
        out["tags"].append("hard:post_resection_extent")
    elif mode == "recovered":
        out["text"] = ch(EXT_RECOV)
        out["body"] = "yes"
        out["tags"].append("hard:recovered_extent")
    elif mode == "benign_neg":
        out["text"] = ch(EXT_BENIGN)
        out["body"] = "yes"
        out["tags"].append("hard:benign_negation")
    elif mode == "hedged":
        out["text"] = ch(EXT_HEDGE)
        out["tags"].append("hard:hedged_extent")
    elif mode == "template_contra":
        out["template"] = ch(TEMPLATE_DEFAULT)
        out["text"] = (f"The scope could not be passed beyond the {seg} because of {reason}, "
                       "and the procedure was terminated at that point.")
        out["body"] = "no"
        out["tags"].append("hard:template_default_contradicted")
    elif mode == "arabic":
        if c:
            out["text"] = "تم الوصول إلى الأعور وتم التعرف على فتحة الزائدة الدودية والصمام اللفائفي الأعوري."
            out["body"] = "yes"
        else:
            out["text"] = f"لم يتم الوصول إلى الأعور؛ توقف الفحص عند {SEG_AR[seg]}."
            out["body"] = "no"
        out["tags"].append("hard:arabic_key_statement")
    elif mode == "correction":
        if c:
            out["text"] = ("Incomplete colonoscopy - the caecum was not reached; the furthest point "
                           "was thought to be the hepatic flexure.")
            out["body"] = "no"
            out["add"] = ("Correction to extent: on review of the stored images the appendiceal orifice "
                          "and ileocaecal valve are clearly visible. Caecal intubation WAS achieved; this "
                          "was a complete colonoscopy and the earlier statement is withdrawn.")
        else:
            out["text"] = ch(EXT_YES)
            out["body"] = "yes"
            out["add"] = (f"Correction to extent: review of the photo-documentation shows that the landmark "
                          f"recorded as the caecum was the {seg}. The caecum was NOT reached and the "
                          "examination was incomplete. CT colonography has been requested.")
        out["tags"].append("hard:addendum_correction")
    elif mode == "self_correction":
        if c:
            out["text"] = ("the scope was advanced to the hepatic flexure, scratch that, correction, the scope "
                           "was advanced to the caecum and the appendiceal orifice and ileocaecal valve were seen.")
            out["body"] = "yes"
        else:
            out["text"] = (f"the caecum was reached, scratch that, correction, the scope only reached the {seg} "
                           f"because of {reason} and the caecum was not reached.")
            out["body"] = "no"
        out["tags"].append("hard:dictation_self_correction")
    return out


GRADES_OK = ["excellent", "good", "fair", "adequate"]
PREP_OK_T = ["Bowel preparation: {g}.", "{G} bowel preparation.",
             "The quality of bowel preparation was {g}.", "Prep: {g}."]
PREP_BAD_T = ["Bowel preparation: poor.", "Inadequate bowel preparation.",
              "The bowel preparation was poor, with solid stool that could not be cleared.",
              "Prep: inadequate.", "Aronchick scale: poor."]
AGENT_ONLY = ["Bowel preparation: 4 L PEG, split dose.",
              "Prep: sodium picosulfate/magnesium citrate (2 sachets) plus bisacodyl.",
              "Bowel prep regimen: 2 L PEG with ascorbate, second dose 5 h before the procedure.",
              "Preparation: low-residue diet for 2 days, then 4 L PEG."]
COMPLIANCE = ["Pre-procedure checklist: bowel preparation taken as instructed - YES.",
              "Patient reports completing the full bowel preparation with clear watery output.",
              "Nursing admission note: prep completed, last stool clear yellow fluid."]


def bbps_segs(ok, trap, aborted=False):
    if aborted:
        # BBPS convention: segments not reached because of poor prep score 0
        return (0, 0, 1)
    if ok:
        return tuple(R.choice([2, 2, 3, 3, 3]) for _ in range(3))
    if trap:
        vals = [3, 3, 3]
        bad = R.randrange(3)
        vals[bad] = R.choice([0, 1])
        if vals[bad] == 1 and R.random() < 0.5:
            other = (bad + 1 + R.randrange(2)) % 3
            vals[other] = 2
        return tuple(vals)
    while True:
        vals = tuple(R.choice([0, 1, 1, 2, 2]) for _ in range(3))
        if sum(vals) <= 5 and min(vals) < 2:
            return vals


def bbps_text(segs):
    r, t, l = segs
    s = r + t + l
    fmt = R.randrange(9)
    tags = []
    if fmt == 0:
        txt = f"BBPS {r}-{t}-{l} (total {s})."
    elif fmt == 1:
        txt = f"Boston Bowel Preparation Scale: right {r}, transverse {t}, left {l}; total {s}/9."
    elif fmt == 2:
        txt = f"BBPS: R={r} T={t} L={l} ({s})."
    elif fmt == 3:
        txt = f"BBPS {s} ({r}/{t}/{l})."
    elif fmt == 4:
        txt = f"Boston score: L{l} T{t} R{r} = {s}."
        tags.append("hard:bbps_nonstandard_order")
    elif fmt == 5:
        txt = f"BBPS (R/T/L) {r}/{t}/{l}."
    elif fmt == 6:
        txt = f"Boston bowel prep scale {r}+{t}+{l}={s}."
    elif fmt == 7:
        txt = f"BBPS total {s}/9 [right colon {r}, transverse colon {t}, left colon {l}]."
    else:
        txt = (f"Right colon: {r}\nTransverse colon: {t}\nLeft colon (incl. rectum): {l}\n"
               f"BBPS total: {s}")
    return txt, tags


def make_prep(mode, pr, c, fail, force_trap=False):
    out = {"text": "", "form": None, "body": None, "add": None, "tags": [], "key": None,
           "metrics": None, "boston": False}
    if mode == "verbal":
        if pr:
            g = ch(GRADES_OK)
            if g == "fair" and R.random() < 0.5:
                out["text"] = "Bowel preparation: fair (adequate for detection of lesions >5 mm)."
            elif g != "adequate" and R.random() < 0.2:
                out["text"] = f"Aronchick scale: {g}."
            else:
                out["text"] = ch(PREP_OK_T).format(g=g, G=g.capitalize())
            out["form"] = ("prep", g)
        else:
            out["text"] = ch(PREP_BAD_T)
            out["form"] = ("prep", "poor")
        out["key"] = "adequate" if pr else "inadequate"
    elif mode == "bbps_full":
        trap = (not pr) and c and (R.random() < 0.85 or force_trap)
        segs = bbps_segs(pr, trap, aborted=(not pr and not c))
        if (not pr) and sum(segs) >= 6:
            out["tags"].append("hard:bbps_segment_fail")
        txt, tg = bbps_text(segs)
        out["tags"] += tg
        out["text"] = txt
        out["form"] = ("bbps", segs)
        out["key"] = "adequate" if pr else "inadequate"
        out["boston"] = True
    elif mode == "bbps_total":
        if pr:
            s = ch([8, 9]) if R.random() < 0.6 else ch([6, 7])
        else:
            s = ch([2, 3, 4, 5]) if (R.random() < 0.6 and c) else (ch([6, 7]) if c else ch([0, 1]))
        out["text"] = ch(["BBPS total {s}.", "Boston score {s}/9.", "BBPS = {s}.",
                          "Bowel prep (Boston): {s}."]).format(s=s)
        out["key"] = "adequate" if s >= 8 else ("inadequate" if s <= 5 else "not_stated")
        out["tags"].append("hard:bbps_total_only")
        out["boston"] = True
    elif mode in ("ungraded", "score_lookalike"):
        out["form"] = ("prep", None)
        out["key"] = "not_stated"
        if mode == "score_lookalike":
            out["metrics"] = (f"Gloucester comfort score {ch([2, 3])}/5. Withdrawal time "
                              f"{R.randint(6, 14)} min. ASA grade {ch([1, 2, 3])}. "
                              f"Nurse-recorded pain score {R.randint(1, 3)}/10.")
            out["tags"].append("hard:score_lookalike")
    elif mode == "agent_only":
        out["text"] = ch(AGENT_ONLY)
        out["key"] = "not_stated"
        out["tags"].append("hard:prep_agent_only")
    elif mode == "compliance":
        out["text"] = ch(COMPLIANCE)
        out["key"] = "not_stated"
        out["tags"].append("hard:prep_compliance_not_quality")
    elif mode == "descriptive":
        if pr:
            out["text"] = (ch(["Only small volumes of clear fluid, easily suctioned; the entire colonic mucosa was well visualised.",
                               "Minimal clear residual fluid; all segments were seen in full after washing."])
                           if c else
                           "Small volume of clear fluid only; the mucosa of the examined segments was well visualised.")
        else:
            out["text"] = ("Large amounts of solid and semi-solid stool in the right and transverse colon that could not be cleared by washing; mucosal views were severely limited."
                           if c else
                           "Solid faecal residue obscured much of the lumen in the examined segments and the mucosa could not be adequately assessed.")
        out["key"] = "adequate" if pr else "inadequate"
        out["tags"].append("hard:descriptive_prep")
    elif mode == "segmental":
        out["text"] = ch(["Preparation good in the left colon but poor in the right colon, with adherent stool despite washing.",
                          "Prep: excellent in the rectosigmoid, inadequate in the ascending colon and hepatic flexure.",
                          "Left colon clean; right colon prep poor (solid stool) - small lesions may have been missed proximally."])
        out["key"] = "inadequate"
        out["tags"].append("hard:segmental_prep")
    elif mode == "arabic":
        out["text"] = (ch(["تحضير الأمعاء جيد.", "تحضير الأمعاء ممتاز."]) if pr
                       else "تحضير الأمعاء ضعيف وغير كافٍ.")
        out["key"] = "adequate" if pr else "inadequate"
        out["tags"].append("hard:arabic_key_statement")
    elif mode == "correction":
        if pr:
            out["text"] = "Bowel preparation: poor."
            out["body"] = "inadequate"
            if c:
                out["add"] = ("Amendment to bowel preparation: 'poor' was entered in error. The preparation "
                              "was good throughout (BBPS 3/3/2, total 8).")
                out["boston"] = True
            else:
                out["add"] = ("Amendment to bowel preparation: 'poor' was entered in error; the preparation "
                              "of the examined segments was good.")
        else:
            out["text"] = "Bowel preparation: adequate."
            out["body"] = "adequate"
            out["add"] = ("Correction to bowel preparation: on review, the right colon contained solid stool "
                          "that could not be cleared (BBPS right 1, transverse 2, left 3). Preparation was "
                          "inadequate; repeat colonoscopy with extended preparation advised.")
            out["boston"] = True
        out["key"] = "adequate" if pr else "inadequate"
        out["tags"].append("hard:addendum_correction")
    if out["body"] is None and out["key"] in ("adequate", "inadequate"):
        out["body"] = out["key"]
    return out


def polyp_sentences(locs, abbrev):
    n = ch([1, 1, 1, 2, 2, 3])
    sents, specs = [], []
    for _ in range(n):
        loc = ch(locs)
        size = ch([3, 4, 5, 6, 7, 8, 10, 12, 15, 20])
        morph = ch(["sessile", "pedunculated", "flat (Paris 0-IIa)", "sessile (Paris 0-Is)",
                    "semi-pedunculated"])
        if size <= 5:
            tech, ab = ch([("cold forceps polypectomy", "CFP"), ("cold snare polypectomy", "CSP")])
        elif size <= 9:
            tech, ab = ("cold snare polypectomy", "CSP")
        else:
            tech, ab = ch([("hot snare polypectomy", "HSP"),
                           ("endoscopic mucosal resection after saline lift", "EMR")])
        if abbrev:
            sents.append(f"{ab} {SEG_AB[loc]} {size}mm polyp, retrieved.")
        else:
            sents.append(f"A {size} mm {morph} polyp in the {loc} was removed by {tech} and retrieved for histology.")
        specs.append((f"{loc} polyp", POLYP_RES))
    return sents, specs, n


def biopsy_sentences(kind, locs, abbrev):
    if kind == "random":
        where = ("the right and left colon (separate pots)" if "ascending colon" in locs
                 else "the examined colon and rectum (separate pots)")
        s = f"Macroscopically normal mucosa. Random biopsies taken from {where} to exclude microscopic colitis."
        if abbrev:
            s = "Mucosa NAD. Random Bx x4 for microscopic colitis."
        return [s], [("random colonic biopsies", BX_RES["random"])]
    if kind == "ibd":
        s = "Quiescent colitis appearance. Segmental surveillance biopsies taken (two per segment) as per protocol."
        if abbrev:
            s = "Quiescent UC. Segmental Bx x2 per segment per protocol."
        return [s], [("segmental surveillance biopsies", BX_RES["ibd"])]
    if kind == "erythema":
        s = "Patchy erythema with loss of vascular pattern in the rectum; biopsies taken (x2)."
        if abbrev:
            s = "Patchy erythema R - Bx x2."
        return [s], [("rectal biopsies", BX_RES["erythema"])]
    if kind == "ulcer":
        loc = ch([l for l in locs if l in ("sigmoid colon", "descending colon", "rectum")] or ["rectum"])
        s = f"A 6 mm shallow ulcer in the {loc} was biopsied (x3)."
        if abbrev:
            s = f"6mm ulcer {SEG_AB[loc]} - Bx x3."
        return [s], [(f"{loc} ulcer biopsies", BX_RES["ulcer"])]
    if kind == "stricture":
        s = "Short benign-appearing stricture in the sigmoid colon, passable with the scope; biopsied (x4)."
        return [s], [("sigmoid stricture biopsies", BX_RES["stricture"])]
    raise ValueError(kind)


def pick_locs(stop_idx, exclude_caecum, anastomosis):
    locs = SEGS[:stop_idx + 1]
    if anastomosis:
        locs = SEGS[:5]
    if exclude_caecum and "caecum" in locs:
        locs = [l for l in locs if l != "caecum"]
    return locs


# ---------------------------------------------------------------------------
# One report
# ---------------------------------------------------------------------------
def gen_report(idx, truth, fail, mrn, force=None):
    force = force or {}
    tags = []
    # ---- hidden reality
    if truth == "adequate":
        c, pr = True, True
        t = R.random() < 0.45
        h = t
    elif truth == "awaiting_histology":
        c, pr, t, h = True, True, True, False
    else:
        c = fail == "prep"
        pr = fail == "caecum"
        t = R.random() < 0.4
        h = t and R.random() < 0.4

    layout = wch(LAYOUT_W)
    us = R.random() < (0.15 if layout == "bilingual" else 0.42)
    arabic = layout == "bilingual" or R.random() < 0.12
    ocr = layout in OCR_OK and R.random() < 0.22

    # ---- failure geometry
    rkey, stop_idx = None, 7
    if not c:
        if fail == "both":
            rkey = "prep"
            stop_idx = R.randint(1, 5)
        else:
            rkey = wch([("looping", 35), ("angulation", 20), ("discomfort", 20),
                        ("stricture", 8), ("mass", 17 if t else 0)])
            stop_idx = R.randint(1, 2) if rkey in ("mass", "stricture", "angulation") else R.randint(1, 6)
    stop_seg = SEGS[stop_idx] if not c else None

    # ---- extent documentation
    if c:
        ext_mode = wch([("stated", 44), ("unstated", 12), ("ti", 8), ("abbrev", 6),
                        ("anastomosis", 4), ("recovered", 5), ("benign_neg", 5), ("hedged", 5),
                        ("arabic", 9 if arabic else 0), ("correction", 5),
                        ("self_correction", 45 if layout == "dictation" else 0)])
    else:
        ext_mode = wch([("stated", 50), ("unstated", 12), ("hedged", 8), ("template_contra", 20),
                        ("abbrev", 5), ("arabic", 9 if arabic else 0),
                        ("correction", 10 if fail != "both" else 0),
                        ("self_correction", 45 if layout == "dictation" else 0)])
    ext_mode = force.get("ext_mode", ext_mode)
    ext = make_extent(ext_mode, c, stop_seg, rkey)
    caecum_key = "not_stated" if ext_mode in ("unstated", "hedged") else ("yes" if c else "no")
    tags += ext["tags"]
    abbrev = ext_mode == "abbrev"
    anast = ext_mode == "anastomosis"

    # ---- prep documentation
    if pr:
        prep_mode = wch([("verbal", 36), ("bbps_full", 16 if c else 0), ("bbps_total", 7 if c else 0),
                         ("ungraded", 7), ("agent_only", 5), ("compliance", 4), ("score_lookalike", 4),
                         ("descriptive", 5), ("arabic", 8 if arabic else 0), ("correction", 6)])
    elif c:
        prep_mode = wch([("verbal", 24), ("bbps_full", 32), ("bbps_total", 10),
                         ("segmental", 30), ("ungraded", 7), ("agent_only", 3),
                         ("compliance", 3), ("descriptive", 7), ("arabic", 8 if arabic else 0),
                         ("correction", 7)])
    else:
        # aborted because of the preparation itself: the body always grades it as inadequate
        prep_mode = wch([("verbal", 30), ("bbps_full", 20), ("bbps_total", 8),
                         ("descriptive", 10), ("arabic", 10 if arabic else 0)])
    prep_mode = force.get("prep_mode", prep_mode)
    prep = make_prep(prep_mode, pr, c, fail, force.get("trap", False))
    prep_key = prep["key"]
    tags += prep["tags"]

    # ---- tissue documentation
    exclude_caecum = caecum_key == "not_stated" or ext_mode in ("ti", "anastomosis", "correction")
    locs = pick_locs(stop_idx, exclude_caecum, anast)
    sents, specs, imp_items = [], [], []
    tissue_body = t
    tissue_add = None
    negated = False
    has_polyp = has_bx = False
    ind_force = None
    if t:
        if rkey == "mass":
            kind = "mass"
        else:
            kind = wch([("polyp", 55), ("biopsy", 35), ("both", 10)])
        tmode = wch([("plain", 80), ("specimen_label", 30 if kind == "biopsy" else 0),
                     ("correction", 6), ("arabic", 9 if arabic else 0)])
        tmode = force.get("tmode", tmode)
        if kind == "mass":
            tmode = "plain"
        if tmode == "plain":
            if kind in ("polyp", "both"):
                s, sp, n = polyp_sentences(locs, abbrev)
                sents += s
                specs += sp
                has_polyp = True
                imp_items.append(f"{n} polyp{'s' if n > 1 else ''} removed")
            if kind in ("biopsy", "both"):
                bk = wch([("random", 30), ("ibd", 22), ("erythema", 18), ("ulcer", 14),
                          ("stricture", 6 if c else 0)])
                s, sp = biopsy_sentences(bk, locs, abbrev)
                sents += s
                specs += sp
                has_bx = True
                if bk == "random":
                    ind_force = "diarrhoea"
                elif bk == "ibd":
                    ind_force = "ibd"
                imp_items.append({"random": "Normal-appearing mucosa - random biopsies taken",
                                  "ibd": "Quiescent colitis - surveillance biopsies taken",
                                  "erythema": "Distal proctitis - biopsied",
                                  "ulcer": "Solitary ulcer - biopsied",
                                  "stricture": "Sigmoid stricture - biopsied"}[bk])
            if kind == "mass":
                cm = {1: R.randint(18, 30), 2: R.randint(32, 40)}[stop_idx]
                if caecum_key == "not_stated":
                    sents.append(f"Friable, partly circumferential mass at {cm} cm ({SEGS[stop_idx]}); biopsies "
                                 "taken (x6) and the site tattooed distally.")
                else:
                    sents.append(f"Circumferential friable mass at {cm} cm ({SEGS[stop_idx]}) causing near-complete "
                                 "obstruction; biopsies taken (x6) and the site tattooed distally.")
                specs.append(("mass biopsies", BX_RES["mass"]))
                has_bx = True
                imp_items.append(f"Obstructing {SEGS[stop_idx]} tumour - biopsied")
        elif tmode == "specimen_label":
            sites = [l for l in locs if l != "caecum"]
            a, b = ("rectum", "sigmoid colon") if len(sites) < 2 else tuple(sorted(R.sample(sites, 2), key=SEGS.index))
            sents.append(ch([f"Patchy erythema in the {a} and {b}.",
                             "Macroscopically unremarkable mucosa.",
                             f"Mild granularity in the {a}."]))
            sents.append(f"Specimens: Pot A - {a}; Pot B - {b} (to histopathology).")
            specs.append((f"pots A-B ({a}, {b})", BX_RES["random"] + BX_RES["erythema"]))
            has_bx = True
            tags.append("hard:specimen_label_only")
            imp_items.append("Specimens sent")
        elif tmode == "arabic":
            loc = ch([l for l in locs if l != "caecum"])
            if kind == "polyp":
                sents.append(f"تم استئصال سليلة صغيرة من {SEG_AR[loc]} وإرسالها للفحص النسيجي.")
                specs.append((f"{loc} polyp", POLYP_RES))
                has_polyp = True
            else:
                sents.append(f"تم أخذ خزعات من {SEG_AR[loc]}.")
                specs.append((f"{loc} biopsies", BX_RES["erythema"] + BX_RES["random"]))
                has_bx = True
            tags.append("hard:arabic_key_statement")
        elif tmode == "correction":
            loc = ch([l for l in locs if l in ("rectum", "sigmoid colon", "descending colon")] or ["rectum"])
            sents.append("No biopsies taken.")
            tissue_body = False
            tissue_add = (f"Correction to findings: two biopsies WERE taken from the {loc} (pot A) and sent for "
                          "histology; the earlier entry 'No biopsies taken' is incorrect.")
            specs.append((f"{loc} biopsies", BX_RES["erythema"] + BX_RES["random"]))
            has_bx = True
            tags.append("hard:addendum_correction")
        tags.append("tissue:polypectomy" if has_polyp else None)
        tags.append("tissue:biopsy" if has_bx else None)
    else:
        tmode = wch([("negated", 55), ("silent", 20), ("not_resected", 12), ("planned_not_done", 12),
                     ("correction", 6), ("arabic", 8 if arabic else 0)])
        tmode = force.get("tmode", tmode)
        if tmode == "negated":
            sents.append(ch(NEGATED))
            negated = True
        elif tmode == "not_resected":
            loc = ch([l for l in locs if l != "caecum"])
            s = R.randint(3, 25)
            sents.append(ch([
                f"A {min(s, 5)} mm hyperplastic-appearing polyp in the rectum was left in situ (NICE type 1); no polypectomy performed.",
                f"A {max(s, 15)} mm laterally spreading lesion in the {loc} was tattooed distally and NOT resected; referred for EMR at a later date.",
                f"Small {min(s, 8)} mm sessile polyp in the {loc} noted - not removed (patient on dual antiplatelet therapy); for elective polypectomy.",
            ]))
            negated = "no polypectomy" in sents[-1] or "NOT resected" in sents[-1] or "not removed" in sents[-1]
            tags.append("hard:polyp_not_resected")
            imp_items.append("Polyp/lesion seen - not resected")
        elif tmode == "planned_not_done":
            sents.append(ch([
                "Biopsies were planned to assess for microscopic colitis but were not taken because the patient had not stopped clopidogrel.",
                "Biopsy of the rectal erythema was intended but deferred - INR 3.1 on the day.",
            ]))
            if "microscopic colitis" in sents[-1]:
                ind_force = "diarrhoea"
            negated = True
            tags.append("hard:planned_not_done")
        elif tmode == "correction":
            loc = ch([l for l in locs if l in ("rectum", "sigmoid colon")] or ["rectum"])
            sents.append(f"Biopsies taken from the {loc} (x2).")
            tissue_body = True
            tissue_add = ("Correction: no biopsies were taken at this procedure. The biopsy entry above was "
                          "recorded in error (template carry-over) and has been voided; no specimens were sent.")
            negated = True
            tags.append("hard:addendum_correction")
        elif tmode == "arabic":
            sents.append("لم يتم أخذ أي خزعات ولم يتم استئصال أي سلائل.")
            negated = True
            tags.append("hard:arabic_key_statement")
        tags.append("tissue:none")
    if negated:
        tags.append("tissue:negated")
        tags.append("hard:negated_tissue")
    tags = [x for x in tags if x]

    # ---- filler findings (only in reachable segments)
    for txt, imp, seg in R.sample(FILLER, R.choice([0, 1, 1, 2])):
        if seg in locs or seg == "rectum":
            sents.append(txt)
            if imp:
                imp_items.append(imp)
    if not sents:
        sents.append("No mucosal abnormality in the segments examined.")
    if abbrev and R.random() < 0.6:
        sents.append("No other abnormality (NAD).")

    # ---- histology documentation
    hist_text = None
    hist_add = None
    prior = None
    hist_mode = "none"
    if t and h:
        hist_mode = wch([("inline", 65), ("addendum", 35)])
    elif t:
        hist_mode = wch([("pending", 45), ("request", 20), ("none", 20), ("pending_prior", 15)])
    else:
        hist_mode = wch([("none", 80), ("na", 20)])
    if tmode == "correction" and not t:
        hist_mode = "none"
    if tmode == "correction" and t and hist_mode == "none":
        hist_mode = "pending"

    # ---- dates
    pdate = date(2025, 1, 6) + timedelta(days=R.randrange(0, 630))
    dstyle = R.randrange(2)
    hdate = pdate + timedelta(days=R.randint(5, 14))
    adate = pdate + timedelta(days=R.randint(1, 20))
    D = lambda d: fmt_date(d, us, dstyle)

    def result_text():
        parts = []
        for i, (label, pool) in enumerate(specs):
            parts.append(f"{'ABCDEFG'[i]}. {label}: {ch(pool)}")
        return "; ".join(parts) + "."

    if hist_mode == "inline":
        hist_text = f"Histopathology (reported {D(hdate)}): " + result_text()
    elif hist_mode == "addendum":
        hist_text = ch(PENDING)
        hist_add = f"Histology now available ({D(hdate)}): " + result_text()
        tags.append("hard:addendum_histology")
    elif hist_mode in ("pending", "pending_prior"):
        hist_text = ch(PENDING)
        if hist_mode == "pending_prior":
            prior = "prior_histology"
    elif hist_mode == "request":
        hist_text = (f"Histopathology request form completed: lab no. H{pdate.year % 100}-{R.randint(10000, 99999)}, "
                     f"{len(specs)} specimen pot(s), clinical details '{('?adenoma' if has_polyp else ch(['?microscopic colitis', '?inflammation', '?IBD']))}'. "
                     "Specimen status: RECEIVED IN LABORATORY.")
        tags.append("hard:request_not_result")
    elif hist_mode == "na":
        hist_text = ch(["Histology: not applicable.", "Histopathology: N/A - no specimens."])
    if tissue_add and t and h and hist_mode == "inline":
        # result belongs after the correction that introduces the specimen
        hist_add = f"Histology ({D(hdate)}): " + result_text()
        hist_text = None
        hist_mode = "addendum"

    # ---- prior procedures
    if prior is None and R.random() < 0.13:
        prior = wch([("prior_incomplete", 30), ("prior_poor_prep", 20), ("prior_histology", 25),
                     ("prior_biopsy", 15), ("copy_forward", 10)])
    history = None
    py = R.randint(2018, 2024)
    if prior == "prior_incomplete":
        history = (f"Previous colonoscopy ({py}) was incomplete - the scope could not be passed beyond the "
                   f"{ch(['sigmoid colon', 'splenic flexure', 'hepatic flexure'])} and the caecum was not reached.")
    elif prior == "prior_poor_prep":
        history = (f"Last colonoscopy ({py}) was abandoned because of poor bowel preparation; an extended "
                   "regimen was used on this occasion.")
    elif prior == "prior_histology":
        history = f"Prior polypectomy ({py}): histology showed {ch(POLYP_RES)}."
    elif prior == "prior_biopsy":
        history = f"Biopsies taken at colonoscopy in {py} showed {ch(BX_RES['random'][1:] + BX_RES['ibd'])}."
    elif prior == "copy_forward":
        history = (f"[Text carried forward from report dated {D(date(py, R.randint(1, 12), R.randint(1, 28)))}: "
                   "'Biopsies taken from the rectum and sigmoid for histology. Caecum not reached.']")
    if prior:
        tags.append("hard:prior_procedure")

    # ---- bidirectional session
    bidir = R.random() < (0.12 if not t else 0.04)
    ogd_text = None
    ogd_hist = None
    if bidir:
        ogd_text = (f"OGD: {ch(['Mild antral gastritis', 'Small hiatus hernia and mild oesophagitis (LA grade A)', 'Normal oesophagus, stomach and duodenum', 'Erythematous duodenal mucosa'])}. "
                    + ch(["CLO test and gastric biopsies (antrum and body) taken.",
                          "Duodenal (D2) biopsies taken to exclude coeliac disease.",
                          "Gastric biopsies taken (Sydney protocol)."]))
        if R.random() < 0.5:
            ogd_hist = "OGD specimens: " + ch([
                "gastric biopsies - chronic active gastritis, H. pylori organisms identified.",
                "D2 biopsies - normal villous architecture; no increase in intraepithelial lymphocytes.",
                "gastric biopsies - mild chronic gastritis, H. pylori not seen."])
        tags.append("hard:bidirectional_session")

    # ---- indication
    if anast:
        indication = f"Surveillance after right hemicolectomy ({R.randint(2019, 2023)}) for caecal adenocarcinoma"
    elif ind_force == "diarrhoea":
        indication = "Chronic watery diarrhoea for 4 months"
    elif ind_force == "ibd":
        indication = f"Ulcerative colitis (diagnosed {R.randint(2008, 2016)}) - dysplasia surveillance"
    else:
        indication = ch(IND_GENERAL)
    if bidir:
        indication += " and dyspepsia"

    # ---- procedure / metrics
    sed = ch(["conscious sedation (midazolam {m} mg, fentanyl {f} micrograms)",
              "propofol sedation administered by an anaesthetist",
              "nitrous oxide/oxygen (50:50) only, at patient request",
              "conscious sedation (midazolam {m} mg)"]).format(m=ch([2, 3, 4, 5]), f=ch([25, 50, 75]))
    pname = ("OGD and colonoscopy in the same session" if bidir
             else ch(["Colonoscopy", "Diagnostic colonoscopy", "Colonoscopy (day case)"]))
    procedure = f"{pname} under {sed}."
    metrics = prep["metrics"]
    if metrics is None and R.random() < 0.25:
        metrics = f"Withdrawal time {R.randint(6, 15)} min."

    # ---- impression / recommendations (from what the body says)
    imp = []
    if ext["body"] == "no" and ext_mode != "template_contra" and R.random() < 0.7:
        imp.append(f"Incomplete colonoscopy{(' to the ' + stop_seg) if ext_mode != 'correction' else ''}")
    elif ext["body"] == "yes" and ext_mode in ("stated", "correction", "benign_neg", "recovered") and R.random() < 0.5:
        imp.append("Complete colonoscopy")
    if prep["body"] == "inadequate" and R.random() < 0.7:
        imp.append("Inadequate bowel preparation")
    imp += imp_items
    if not imp:
        imp.append("No significant pathology in the segments examined")
    impression = "; ".join(imp) + "."
    recs = []
    if ext["body"] == "no":
        recs.append(ch(["CT colonography to complete assessment of the proximal colon.",
                        "Repeat colonoscopy under propofol sedation to complete the examination."]))
    if prep["body"] == "inadequate":
        recs.append("Repeat colonoscopy with extended bowel preparation within 3 months.")
    if rkey == "mass":
        recs.append("Urgent CT staging and colorectal MDT referral.")
    if tissue_body and hist_mode in ("pending", "pending_prior", "request", "none", "addendum"):
        recs.append("Surveillance interval to be set once histology is available.")
    if not recs and t and h:
        recs.append(ch(["Surveillance interval per guideline in light of the histology above.",
                        "Clinic review to discuss histology; surveillance as per guideline."]))
    if not recs:
        recs.append(ch(["Return to GP care.", "Repeat colonoscopy in 5 years.",
                        "Repeat colonoscopy in 10 years if asymptomatic.", "Follow up in clinic in 6 weeks."]))
    recs_text = " ".join(recs)

    # ---- assemble pieces
    pieces = [("indication", indication + ".", None)]
    if history:
        pieces.append(("history", history, None))
    pieces.append(("procedure", procedure, None))
    if ogd_text:
        pieces.append(("ogd", ogd_text, None))
    if ext["template"]:
        pieces.append(("template", ext["template"], None))
    pieces.append(("extent", ext["text"], ext["form"]))
    pieces.append(("prep", prep["text"], prep["form"]))
    if metrics:
        pieces.append(("metrics", metrics, None))
    pieces.append(("findings", " ".join(sents), None))
    htxt = " ".join(x for x in [hist_text, ogd_hist] if x)
    if htxt:
        pieces.append(("histology", htxt, None))
    pieces.append(("impression", impression, None))
    pieces.append(("recs", recs_text, None))
    if arabic and layout != "bilingual":
        pieces.append(("note", ch(["المريض بحالة مستقرة بعد الإجراء وخرج بصحبة مرافق.",
                                   "تم شرح النتائج للمريض وأسرته.",
                                   "يرجى مراجعة العيادة بعد أسبوعين."]), None))

    addenda = []
    if ext["add"]:
        addenda.append((adate, ext["add"]))
    if prep["add"]:
        addenda.append((adate + timedelta(days=1), prep["add"]))
    if tissue_add:
        addenda.append((pdate + timedelta(days=1), tissue_add))
    if hist_add:
        addenda.append((hdate, hist_add))
    addenda.sort(key=lambda x: x[0])
    addenda = [(D(d), txt) for d, txt in addenda]

    # ---- header
    if arabic or R.random() < 0.45:
        male = R.random() < 0.5
        g, gar = ch(EM_MALE if male else EM_FEMALE)
        f, far = ch(EM_FAMILY)
        name, name_ar, sx = f"{g} {f}", f"{gar} {far}", ("M" if male else "F")
    else:
        name, sx = ch(EXPAT)
        name_ar = None
    if R.random() < 0.25:
        name = "".join(p[0] for p in name.replace("Al ", "Al").split()) + " (initials)"
        name_ar = None
    hdr = {"facility": ch(FACILITIES), "name": name, "name_ar": name_ar,
           "age": R.randint(40, 82), "sx": sx,
           "mrn": ch(["AD", "MRN-", "", "UH"]) + str(mrn), "date": D(pdate),
           "doc": ch(DOCTORS), "gp": ch(GPS), "arabic": arabic, "us": us,
           "time": f"{R.randint(7, 16):02d}:{ch(['00', '15', '20', '35', '40', '50'])}"}

    text, extra_tags = render(layout, hdr, pieces, addenda)
    tags += extra_tags
    if us:
        text = to_us(text)
    if ocr:
        text = ocr_noise(text, hdr)
        tags.append("noise:ocr")
        tags.append("hard:ocr_noise")

    # ---- key + truth
    tissue_key = "yes" if t else "no"
    hist_key = "yes" if (t and h) else "no"
    key_fields = {"caecum": caecum_key, "prep": prep_key, "polyps": tissue_key,
                  "tissue_taken": tissue_key, "histology": hist_key}
    key_state, key_reason = apply_key_rules(key_fields)
    if (caecum_key == "not_stated" and not c) or (prep_key == "not_stated" and not pr):
        tags.append("hidden:undocumented_failure")

    prep_tag = {"verbal": "prep:verbal", "bbps_full": "prep:bbps_segments", "bbps_total": "prep:bbps_total",
                "ungraded": "prep:ungraded", "agent_only": "prep:ungraded", "compliance": "prep:ungraded",
                "score_lookalike": "prep:ungraded", "descriptive": "prep:descriptive",
                "segmental": "prep:segmental_verbal", "arabic": "prep:arabic_verbal",
                "correction": "prep:corrected"}[prep_mode]
    tags = ([f"fmt:{layout}", "spelling:us" if us else "spelling:uk", prep_tag]
            + (["prep:boston"] if prep["boston"] else [])
            + [f"extent:{ext_mode}", f"hist:{hist_mode}"]
            + (["lang:arabic"] if arabic or "arabic" in (ext_mode, prep_mode, tmode) else [])
            + tags)
    seen = []
    for tg in tags:
        if tg not in seen:
            seen.append(tg)
    tags = seen

    reality = {"caecum": c, "prep": pr, "tissue": t, "histology": h}
    return {
        "uid": f"q{idx:03d}",
        "sha": hashlib.sha256(text.encode("utf-8")).hexdigest(),
        "text": text,
        "truth": truth,
        "key_fields": key_fields,
        "key_state": key_state,
        "key_reason": key_reason,
        "tags": tags,
    }, reality


def apply_key_rules(k):
    if k["caecum"] == "no":
        return "not_closed", "incomplete_exam"
    if k["prep"] == "inadequate":
        return "not_closed", "inadequate_prep"
    if k["caecum"] == "not_stated" or k["prep"] == "not_stated":
        return "needs_human", "missing_documentation"
    if k["tissue_taken"] == "yes" and k["histology"] == "no":
        return "not_closed", "awaiting_histology"
    return "closed", "criteria_met"


def truth_from_reality(r):
    if not r["caecum"] or not r["prep"]:
        return "inadequate"
    if r["tissue"] and not r["histology"]:
        return "awaiting_histology"
    return "adequate"


# ---------------------------------------------------------------------------
# Layout renderers
# ---------------------------------------------------------------------------
def nonempty(pieces):
    return [(f, t, fm) for f, t, fm in pieces if t]


def form_line(form):
    kind = form[0]
    if kind == "caecum":
        v = form[1]
        y = "X" if v == "yes" else " "
        n = "X" if v == "no" else " "
        line = f"Caecum reached:        [{y}] Yes   [{n}] No"
        if v == "no":
            line += f"\nFurthest point reached: {form[2]}"
        elif v is None:
            line += "\nFurthest point reached: ______________"
        return line
    if kind == "prep":
        g = form[1]
        boxes = []
        for opt in ["Excellent", "Good", "Fair", "Poor"]:
            mark = "X" if g and g.lower() == opt.lower() else " "
            boxes.append(f"[{mark}] {opt}")
        return "Bowel preparation:     " + "  ".join(boxes)
    if kind == "bbps":
        r, t, l = form[1]
        return f"BBPS   Right: {r}   Transverse: {t}   Left: {l}   Total: {r + t + l}"
    return ""


def render(layout, hdr, pieces, addenda):
    fn = {"free_text": r_free, "headed": r_headed, "key_value": r_kv, "table": r_table,
          "form": r_form, "dictation": r_dictation, "hl7": r_hl7, "letter": r_letter,
          "bilingual": r_bilingual}[layout]
    return fn(hdr, pieces, addenda)


def signoff(hdr):
    return ch([f"Endoscopist: {hdr['doc']}", f"Electronically signed by {hdr['doc']} at {hdr['time']}",
               f"Signed: {hdr['doc']} (Consultant Gastroenterologist)"])


def r_free(hdr, pieces, addenda):
    L = []
    if R.random() < 0.7:
        L.append(hdr["facility"])
    L.append(ch(["COLONOSCOPY REPORT", "Colonoscopy report", "Endoscopy report - lower GI"]))
    if hdr["arabic"]:
        L.append("تقرير تنظير القولون" + (f" - {hdr['name_ar']}" if hdr["name_ar"] else ""))
    L.append(f"{hdr['name']}, {hdr['age']}{hdr['sx']}, MRN {hdr['mrn']}. Date of procedure: {hdr['date']}.")
    L.append("")
    groups = [["indication", "history"], ["procedure", "ogd", "template", "extent", "prep", "metrics"],
              ["findings", "histology"], ["impression", "recs", "note"]]
    pref = {"indication": "Indication: ", "impression": "Impression: ", "recs": "Plan: "}
    for grp in groups:
        para = []
        for f, t, _ in nonempty(pieces):
            if f in grp:
                if f == "template":
                    para.append("\n" + t + "\n")
                else:
                    para.append(pref.get(f, "") + t.replace("\n", "; "))
        if para:
            L.append(" ".join(para).replace(" \n", "\n").replace("\n ", "\n").strip())
            L.append("")
    for d, txt in addenda:
        L.append(f"Addendum ({d}, {hdr['doc']}): {txt}")
        L.append("")
    L.append(signoff(hdr))
    return "\n".join(L), []


def r_headed(hdr, pieces, addenda):
    style = R.randrange(3)
    L = [hdr["facility"].upper(), "LOWER GI ENDOSCOPY REPORT"]
    if hdr["arabic"]:
        L.append("تقرير تنظير القولون")
    L.append(f"Patient: {hdr['name']}    Age/Sex: {hdr['age']}/{hdr['sx']}    MRN: {hdr['mrn']}")
    if hdr["arabic"] and hdr["name_ar"]:
        L.append(f"الاسم: {hdr['name_ar']}")
    L.append(f"Date: {hdr['date']}    Endoscopist: {hdr['doc']}    Referrer: {hdr['gp']}")
    L.append("")
    for f, t, _ in nonempty(pieces):
        lab = LBL[f][0]
        if style == 0:
            L.append(f"{lab.upper()}:")
        elif style == 1:
            L.append(lab)
            L.append("-" * len(lab))
        else:
            L.append(f"## {lab}")
        L.append(t)
        L.append("")
    for d, txt in addenda:
        L.append(f"ADDENDUM - {d} - {hdr['doc']}")
        L.append(txt)
        L.append("")
    L.append(signoff(hdr))
    return "\n".join(L), []


def r_kv(hdr, pieces, addenda):
    sep = ch([": ", "=", " | "])
    L = [f"REPORT_TYPE{sep}COLONOSCOPY", f"FACILITY{sep}{hdr['facility']}",
         f"PATIENT_NAME{sep}{hdr['name']}"]
    if hdr["arabic"] and hdr["name_ar"]:
        L.append(f"PATIENT_NAME_AR{sep}{hdr['name_ar']}")
    L += [f"PATIENT_MRN{sep}{hdr['mrn']}", f"PATIENT_AGE{sep}{hdr['age']}",
          f"PATIENT_SEX{sep}{hdr['sx']}", f"PROC_DATE{sep}{hdr['date']}",
          f"PROC_TIME{sep}{hdr['time']}", f"ENDOSCOPIST{sep}{hdr['doc']}"]
    tags = []
    used = {}
    for f, t, form in pieces:
        key = LBL[f][1]
        if not t:
            if form and form[0] == "caecum" and R.random() < 0.6:
                L.append(f"CAECUM_REACHED{sep}")
                tags.append("hard:blank_form_field")
            elif form and form[0] == "prep" and R.random() < 0.6:
                L.append(f"PREP_QUALITY{sep}")
                tags.append("hard:blank_form_field")
            continue
        used[key] = used.get(key, 0) + 1
        if used[key] > 1:
            key = f"{key}_{used[key]}"
        L.append(f"{key}{sep}{t.replace(chr(10), ' / ')}")
    for i, (d, txt) in enumerate(addenda, 1):
        L.append(f"ADDENDUM_{i}_DATE{sep}{d}")
        L.append(f"ADDENDUM_{i}_TEXT{sep}{txt}")
    L.append(f"SIGNED_BY{sep}{hdr['doc']}")
    L.append(f"EXPORT_STATUS{sep}FINAL")
    return "\n".join(L), tags


def r_table(hdr, pieces, addenda):
    md = R.random() < 0.5
    L = [hdr["facility"], "Colonoscopy - procedure summary"]
    if hdr["arabic"]:
        L.append("تقرير تنظير القولون")
    L.append(f"{hdr['name']} | {hdr['age']} {hdr['sx']} | MRN {hdr['mrn']} | {hdr['date']} | {hdr['doc']}")
    L.append("")
    rows = []
    for f, t, form in pieces:
        if not t:
            if form and form[0] in ("caecum", "prep") and R.random() < 0.5:
                rows.append(("Caecum reached" if form[0] == "caecum" else "Prep quality", ""))
            continue
        rows.append((LBL[f][0], t.replace("\n", "; ")))
    for d, txt in addenda:
        rows.append((f"Addendum {d}", txt))
    if md:
        L.append("| Section | Details |")
        L.append("|---|---|")
        for a, b in rows:
            L.append(f"| {a} | {b} |")
    else:
        w = max(len(a) for a, _ in rows) + 2
        L.append("+" + "-" * w + "+" + "-" * 60)
        for a, b in rows:
            L.append("| " + a.ljust(w - 1) + "| " + b)
        L.append("+" + "-" * w + "+" + "-" * 60)
    L.append("")
    L.append(signoff(hdr))
    tags = ["hard:blank_form_field"] if any(b == "" for _, b in rows) else []
    return "\n".join(L), tags


def r_form(hdr, pieces, addenda):
    L = ["=== COLONOSCOPY PROCEDURE RECORD ===", f"Unit: {hdr['facility']}",
         f"Patient: {hdr['name']}      MRN: {hdr['mrn']}      Age/Sex: {hdr['age']}{hdr['sx']}",
         f"Date: {hdr['date']}      Time: {hdr['time']}      Endoscopist: {hdr['doc']}"]
    if hdr["arabic"]:
        L.insert(1, "سجل إجراء تنظير القولون")
    L.append("")
    tags = []
    for f, t, form in pieces:
        if form and (form[0] in ("caecum", "bbps") or (form[0] == "prep" and (form[1] in (None, "excellent", "good", "fair", "poor")))):
            L.append(form_line(form))
            if form[1] is None:
                tags.append("hard:blank_form_field")
            if t and (f == "prep" and form[0] == "bbps"):
                continue
            if t and R.random() < 0.5 and f == "extent":
                L.append(f"Extent comments: {t}")
            continue
        if not t:
            continue
        lab = {"extent": "Extent / comments", "prep": "Bowel preparation (free text)"}.get(f, LBL[f][0])
        L.append(f"{lab}: {t.replace(chr(10), '; ')}")
    for d, txt in addenda:
        L.append(f"Addendum ({d}): {txt}")
    L.append("")
    L.append(f"Endoscopist signature: {hdr['doc']}        Nurse: ____________")
    return "\n".join(L), tags


def r_dictation(hdr, pieces, addenda):
    spoken = R.random() < 0.5
    stop = " period " if hdr["us"] else " full stop "
    L = [f"[Voice transcription - {hdr['facility']}]",
         f"This is {hdr['doc']} dictating a colonoscopy report for {hdr['name']}, MRN {hdr['mrn']}, procedure date {hdr['date']}."]
    body = []
    lead = {"indication": "The indication was ", "history": "Of note, ", "procedure": "",
            "ogd": "", "template": "The structured template reads ", "extent": "",
            "prep": "", "metrics": "", "findings": "Findings: ", "histology": "",
            "impression": "My impression is ", "recs": "Plan: ", "note": ""}
    for f, t, _ in nonempty(pieces):
        t2 = t.replace("\n", ", ")
        ld = lead[f]
        if ld and ld.endswith(" "):
            t2 = lc_first(t2)
        body.append((ld + t2).strip())
        if f in ("history", "metrics", "histology") and R.random() < 0.3:
            body.append("Um, [inaudible] okay.")
    text = " ".join(body)
    if spoken:
        text = text.replace(". ", stop)
        if text.endswith("."):
            text = text[:-1] + stop.rstrip()
    L.append(text)
    for d, txt in addenda:
        L.append(f"Addendum dictated {d}: {txt}")
    L.append("End of dictation.")
    L.append(f"Transcribed: VR engine / checked by {hdr['doc']}")
    return "\n".join(L), []


def r_hl7(hdr, pieces, addenda):
    def esc(s):
        return s.replace("|", "\\F\\").replace("\n", "\\.br\\")
    nm = hdr["name"].split()
    if "(initials)" in hdr["name"]:
        nm = [nm[0], "INITIALS"]
    L = [f"MSH|^~\\&|ENDOREP|{hdr['facility'][:12].upper().replace(' ', '')}|EMR|ADH|{hdr['date']}||ORU^R01|{R.randint(100000, 999999)}|P|2.5",
         f"PID|1||{hdr['mrn']}^^^AD||{nm[-1]}^{nm[0]}||{hdr['age']}Y|{hdr['sx']}",
         f"OBR|1||{R.randint(1000000, 9999999)}|COLO^Colonoscopy|||{hdr['date']}|||||||||{hdr['doc']}"]
    if hdr["arabic"] and hdr["name_ar"]:
        L.append(f"NTE|1||Patient name (Arabic): {hdr['name_ar']}")
    n = 0
    for f, t, _ in nonempty(pieces):
        n += 1
        L.append(f"OBX|{n}|TX|{HL7_CODE[f]}^{LBL[f][0]}||{esc(t)}||||||F")
    for d, txt in addenda:
        n += 1
        L.append(f"OBX|{n}|TX|ADD^Addendum {d}||{esc(txt)}||||||C")
    return "\n".join(L), []


def r_letter(hdr, pieces, addenda):
    L = [hdr["facility"], hdr["date"], "", f"Dear {hdr['gp']},", "",
         f"Re: {hdr['name']}, {hdr['age']}{hdr['sx']}, MRN {hdr['mrn']}"]
    if hdr["arabic"] and hdr["name_ar"]:
        L.append(f"({hdr['name_ar']})")
    L.append("")
    d = {f: t for f, t, _ in nonempty(pieces)}
    p1 = f"Thank you for referring this patient. The indication was {lc_first(d['indication'])}"
    if "history" in d:
        p1 += " " + d["history"]
    L.append(p1)
    L.append("")
    mid = [d[f].replace("\n", "; ") for f in ("procedure", "ogd", "template", "extent", "prep", "metrics") if f in d]
    L.append("I performed the procedure on " + hdr["date"] + ". " + " ".join(mid))
    L.append("")
    L.append(" ".join(d[f] for f in ("findings", "histology") if f in d))
    L.append("")
    L.append(f"In summary: {d['impression']} {d['recs']}")
    if "note" in d:
        L.append(d["note"])
    L.append("")
    L.append("Yours sincerely,")
    L.append(hdr["doc"])
    L.append("Consultant Gastroenterologist")
    for dd, txt in addenda:
        L.append("")
        L.append(f"Addendum ({dd}): {txt}")
    return "\n".join(L), []


def r_bilingual(hdr, pieces, addenda):
    L = [f"{hdr['facility']}", "COLONOSCOPY REPORT / تقرير تنظير القولون",
         f"Name / الاسم: {hdr['name']}" + (f" / {hdr['name_ar']}" if hdr["name_ar"] else ""),
         f"MRN / الرقم الطبي: {hdr['mrn']}    Age / العمر: {hdr['age']}    Sex / الجنس: {hdr['sx']}",
         f"Date / التاريخ: {hdr['date']}    Endoscopist / الطبيب: {hdr['doc']}", ""]
    for f, t, _ in nonempty(pieces):
        L.append(f"{LBL[f][0]} / {LBL[f][2]}:")
        L.append(t)
        L.append("")
    for d, txt in addenda:
        L.append(f"Addendum / ملحق ({d}):")
        L.append(txt)
        L.append("")
    L.append(signoff(hdr))
    return "\n".join(L), []


# ---------------------------------------------------------------------------
# OCR noise (never drops words; never touches digits, Arabic or negations)
# ---------------------------------------------------------------------------
PROTECT = {"not", "none", "no", "nil", "without", "never", "was", "were", "yes", "nor", "n"}
SUBS = [("rn", "m"), ("m", "rn"), ("l", "1"), ("o", "0"), ("e", "c"), ("i", "l"), ("cl", "d"), ("u", "v")]


def mangle(w):
    opts = [(a, b) for a, b in SUBS if a in w]
    if not opts:
        return w
    a, b = ch(opts)
    return w.replace(a, b, 1)


def ocr_noise(text, hdr):
    out = []
    if R.random() < 0.6:
        out.append(ch(["*** SCANNED DOCUMENT - QUALITY MAY BE REDUCED ***",
                       f"Received by fax {hdr['date']} {R.randint(10, 16)}:{R.randint(10, 59)}  Page 1/1",
                       "[scan] p.1", "|||||  |  ||| ||"]))
    for line in text.split("\n"):
        words = line.split(" ")
        nw = []
        for w in words:
            core = w.strip(".,;:()[]'\"").lower()
            if (w and w.isascii() and len(w) >= 4 and not any(x.isdigit() for x in w)
                    and core not in PROTECT and w.upper() != w and R.random() < 0.07):
                w = mangle(w)
            nw.append(w)
            if R.random() < 0.012:
                nw.append(ch(["~", "'", ".", ","]))
        line = " ".join(nw)
        if len(line) > 50 and R.random() < 0.12:
            cut = line.find(" ", len(line) // 2)
            if cut > 0:
                line = line[:cut] + "\n" + line[cut + 1:]
        out.append(line)
    if R.random() < 0.4:
        out.append("- 1 -")
    return "\n".join(out)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
def build():
    truths = ["adequate"] * 120 + ["awaiting_histology"] * 90 + ["inadequate"] * 90
    R.shuffle(truths)
    fails = ["caecum"] * 38 + ["prep"] * 37 + ["both"] * 15
    R.shuffle(fails)
    mrns = R.sample(range(1000000, 9999999), N_REPORTS)
    reports, realities = [], []
    # small fixed quotas so that thin prep-failure families are not left to chance
    prep_fail_slots = [i for i in range(N_REPORTS) if truths[i] == "inadequate"
                       and fails[sum(1 for x in truths[:i] if x == "inadequate")] == "prep"]
    forces = {}
    for n, slot in enumerate(prep_fail_slots[:19]):
        if n < 8:
            forces[slot] = {"prep_mode": "segmental"}
        elif n < 14:
            forces[slot] = {"prep_mode": "bbps_full", "trap": True}
        else:
            forces[slot] = {"prep_mode": "correction"}      # corrected towards inadequate
    # corrections towards failure / no tissue are rarer by chance, so a few are fixed
    caecum_fail_slots = [i for i in range(N_REPORTS) if truths[i] == "inadequate"
                         and fails[sum(1 for x in truths[:i] if x == "inadequate")] == "caecum"]
    for slot in caecum_fail_slots[:5]:
        forces[slot] = {"ext_mode": "correction"}           # corrected to "caecum NOT reached"
    adequate_slots = [i for i in range(N_REPORTS) if truths[i] == "adequate"]
    for slot in adequate_slots[:10]:
        forces[slot] = {"tmode": "correction"}              # tissue corrected in either direction
    for slot in adequate_slots[10:15]:
        forces[slot] = {"ext_mode": "anastomosis"}          # post right hemicolectomy end point
    fi = 0
    for i, tr in enumerate(truths, 1):
        fail = None
        if tr == "inadequate":
            fail = fails[fi]
            fi += 1
        rep, real = gen_report(i, tr, fail, mrns[i - 1], forces.get(i - 1))
        reports.append(rep)
        realities.append(real)
    return reports, realities


MINIMUMS = {
    "layouts": 6, "arabic": 30, "ocr": 25, "ungraded_prep": 40, "unstated_extent": 25,
    "boston": 30, "nonpolyp_biopsy": 25, "negated_tissue": 20, "corrections": 10,
    "prior_refs": 10, "invented_families": 10,
}
REQUIRED_HARD = {"hard:addendum_correction", "hard:addendum_histology", "hard:prior_procedure",
                 "hard:negated_tissue", "hard:ocr_noise"}


def check(reports, realities):
    errors = []
    texts = set()
    for rep, real in zip(reports, realities):
        if hashlib.sha256(rep["text"].encode("utf-8")).hexdigest() != rep["sha"]:
            errors.append(f"{rep['uid']}: sha mismatch")
        if rep["text"] in texts:
            errors.append(f"{rep['uid']}: duplicate text")
        texts.add(rep["text"])
        if apply_key_rules(rep["key_fields"]) != (rep["key_state"], rep["key_reason"]):
            errors.append(f"{rep['uid']}: key_state does not follow rules")
        if truth_from_reality(real) != rep["truth"]:
            errors.append(f"{rep['uid']}: truth does not follow reality")
        k = rep["key_fields"]
        if k["polyps"] != k["tissue_taken"]:
            errors.append(f"{rep['uid']}: polyps != tissue_taken")
        if k["histology"] == "yes" and k["tissue_taken"] != "yes":
            errors.append(f"{rep['uid']}: histology without tissue")
        for fld, real_v in (("caecum", real["caecum"]), ("prep", real["prep"])):
            v = k[fld]
            if v in ("yes", "adequate") and not real_v or v in ("no", "inadequate") and real_v:
                errors.append(f"{rep['uid']}: stated {fld} contradicts reality")
        if (k["tissue_taken"] == "yes") != real["tissue"]:
            errors.append(f"{rep['uid']}: tissue key contradicts reality")
        for need in ("fmt:", "spelling:", "prep:", "tissue:"):
            if not any(tg.startswith(need) for tg in rep["tags"]):
                errors.append(f"{rep['uid']}: missing {need} tag")
    return errors


def stats(reports):
    from collections import Counter
    tc = Counter(tg for r in reports for tg in r["tags"])
    c = {
        "layouts": len([t for t in tc if t.startswith("fmt:")]),
        "arabic": tc["lang:arabic"],
        "ocr": tc["noise:ocr"],
        "ungraded_prep": tc["prep:ungraded"],
        "unstated_extent": tc["extent:unstated"],
        "boston": tc["prep:boston"],
        "nonpolyp_biopsy": tc["tissue:biopsy"],
        "negated_tissue": tc["tissue:negated"],
        "corrections": tc["hard:addendum_correction"],
        "prior_refs": tc["hard:prior_procedure"],
        "invented_families": len([t for t in tc if t.startswith("hard:") and t not in REQUIRED_HARD]),
    }
    hard_share = sum(1 for r in reports if any(t.startswith("hard:") for t in r["tags"])) / len(reports)
    return tc, c, hard_share


def main():
    reports, realities = build()
    errors = check(reports, realities)
    tc, counts, hard_share = stats(reports)
    for k, v in MINIMUMS.items():
        if counts[k] < v:
            errors.append(f"minimum not met: {k} = {counts[k]} < {v}")
    if hard_share < 0.35:
        errors.append(f"hard share {hard_share:.2f} < 0.35")
    if errors:
        sys.stderr.write("\n".join(errors) + "\n")
        sys.exit(1)
    out = {"source": SOURCE, "reports": reports}
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), OUT_NAME)
    with open(path, "w", encoding="utf-8", newline="\n") as fh:
        json.dump(out, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    if "--stats" in sys.argv:
        from collections import Counter
        print("truth:", dict(Counter(r["truth"] for r in reports)))
        print("key_state:", dict(Counter(r["key_state"] for r in reports)))
        print("key_reason:", dict(Counter(r["key_reason"] for r in reports)))
        print("minimum counts:", counts)
        print(f"reports with >=1 hard:* tag: {hard_share:.1%}")
        for tg, n in sorted(tc.items()):
            print(f"  {tg}: {n}")
    else:
        print(f"wrote {OUT_NAME}: {len(reports)} reports")


if __name__ == "__main__":
    main()
