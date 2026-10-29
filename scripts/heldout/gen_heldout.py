#!/usr/bin/env python3
"""Held-out synthetic colonoscopy report corpus (independent second author).

Deterministic generator: Python 3 standard library only, random seed 2027.
Writes reports_heldout.json and README.md next to this script.

Every report has a hidden reality (caecum truly reached, prep truly adequate,
tissue taken, histology result present).  The text documents some or all of
it; key_fields record what the TEXT documents, truth records what happened.
"""
import hashlib
import json
import os
import random
import re
import sys
from collections import Counter

SEED = 2027
N_REPORTS = 300
OUT_DIR = os.path.dirname(os.path.abspath(__file__))
rng = random.Random(SEED)


def ch(seq):
    return rng.choice(seq)


def p(x):
    return rng.random() < x


def wch(pairs):
    tot = sum(w for _, w in pairs)
    r = rng.random() * tot
    for v, w in pairs:
        r -= w
        if r < 0:
            return v
    return pairs[-1][0]


# ---------------------------------------------------------------- vocabulary
SEGS = ["rectum", "sigmoid colon", "descending colon", "splenic flexure",
        "transverse colon", "hepatic flexure", "ascending colon", "caecum"]
SEG_SHORT = {0: "rectum", 1: "sigmoid", 2: "desc colon", 3: "SF", 4: "TC",
             5: "HF", 6: "asc colon", 7: "caecum"}
SEG_AR = {0: "المستقيم", 1: "القولون السيني", 2: "القولون النازل",
          3: "الثنية الطحالية", 4: "القولون المستعرض", 5: "الثنية الكبدية",
          6: "القولون الصاعد", 7: "الأعور"}
SEG_CM = {0: (4, 14), 1: (18, 38), 2: (40, 52), 3: (55, 60), 4: (62, 85),
          5: (88, 98), 6: (100, 115), 7: (118, 135)}

UNITS = ["Corniche Digestive Health Centre - Endoscopy Unit, Abu Dhabi",
         "Al Mushrif Specialist Clinic - Endoscopy Suite",
         "Khalifa City Medical Centre - GI & Endoscopy",
         "Saadiyat Day Surgery Hospital - Endoscopy Department",
         "Reem Island Gastroenterology Centre",
         "Mussafah Community Hospital - Endoscopy Unit",
         "Al Bateen Digestive Diseases Clinic",
         "Shakhbout City Medical Centre - Endoscopy"]
ENDOS = ["Dr R. Haddad", "Dr S. Menon", "Dr L. Al Hashimi", "Dr K. Farouk",
         "Dr M. O'Brien", "Dr P. Iyer", "Dr N. Al Mazrouei", "Dr T. Okafor",
         "Dr H. Saleh", "Dr J. Varghese"]
PATHS = ["Dr A. Karam", "Dr F. Rahman", "Dr E. Novak", "Dr Y. Al Suwaidi"]
NURSES = ["RN A. Cruz", "RN M. Thomas", "RN S. Ali", "RN B. Reyes"]
SCOPES = ["Olympus CF-HQ190L", "Olympus PCF-H190DL", "Pentax EC38-i10L",
          "Fujifilm EC-760R-V/L", "Olympus CF-EZ1500DL"]
PRODUCTS_UK = ["Moviprep 2 L split dose", "Plenvu 1 L split dose",
               "Picolax, two sachets", "Klean-Prep 4 L", "Citrafleet, two sachets",
               "Plenvu, same-day regimen"]
PRODUCTS_US = ["GoLytely 4 L split dose", "SUPREP split dose",
               "Plenvu 1 L split dose", "MiraLAX/Gatorade regimen",
               "Sutab tablets, split dose"]
GRADE_AR = {"excellent": "ممتازة", "good": "جيدة", "adequate": "كافية",
            "fair": "مقبولة", "poor": "ضعيفة", "inadequate": "غير كافية"}

HEAD = {
    "indication": ["INDICATION", "Indication(s)", "Reason for procedure",
                   "Clinical indication", "Referral reason"],
    "sedation": ["SEDATION", "Sedation / analgesia", "Medications", "Sedation & analgesia"],
    "prep": ["BOWEL PREPARATION", "Bowel prep", "Preparation", "Prep"],
    "extent": ["EXTENT OF EXAMINATION", "Extent", "Procedure", "Extent / landmarks",
               "Depth of insertion"],
    "withdrawal": ["WITHDRAWAL TIME", "Withdrawal"],
    "findings": ["FINDINGS", "Findings", "Endoscopic findings"],
    "specimens": ["SPECIMENS", "Interventions / specimens", "Therapeutic procedures",
                  "Specimens & interventions"],
    "complications": ["COMPLICATIONS", "Complications", "Adverse events"],
    "impression": ["IMPRESSION", "Conclusion", "Diagnosis", "Summary",
                   "Endoscopic diagnosis"],
    "recs": ["RECOMMENDATIONS", "Plan", "Recommendations / follow-up", "Follow-up"],
    "histology": ["HISTOLOGY", "Histopathology", "Pathology"],
}
HEAD_AR = {"indication": "دواعي الإجراء", "sedation": "التخدير",
           "prep": "تحضير الأمعاء", "extent": "مدى الفحص", "withdrawal": "وقت السحب",
           "findings": "النتائج", "specimens": "الإجراءات والعينات",
           "complications": "المضاعفات", "impression": "الانطباع",
           "recs": "التوصيات", "histology": "الأنسجة المرضية"}
TERSE_LABEL = {"indication": "Ind", "sedation": "Sed", "prep": "Prep", "extent": "Ext",
               "withdrawal": "WT", "findings": "Fx", "specimens": "Rx/Spec",
               "complications": "Cx", "impression": "Imp", "recs": "Plan",
               "histology": "Histo"}
ORDER = ["indication", "sedation", "prep", "extent", "withdrawal", "findings",
         "specimens", "complications", "impression", "recs", "histology"]

US_REPS = [("Caec", "Cec"), ("caec", "cec"), ("Haemorrh", "Hemorrh"),
           ("haemorrh", "hemorrh"), ("Anaesth", "Anesth"), ("anaesth", "anesth"),
           ("haemost", "hemost"), ("Diarrhoea", "Diarrhea"), ("diarrhoea", "diarrhea"),
           ("Anaemia", "Anemia"), ("anaemia", "anemia"), ("faecal", "fecal"),
           ("Faecal", "Fecal"), ("faeces", "feces"), ("programme", "program"),
           ("visualis", "visualiz"), ("colour", "color"), ("paediatric", "pediatric"),
           ("haemoglobin", "hemoglobin")]
ABBR_REPS = [("terminal ileum", "TI"), ("Terminal ileum", "TI"),
             ("ileocaecal valve", "ICV"), ("appendiceal orifice", "AO"),
             ("Appendiceal orifice", "AO"), ("Boston Bowel Preparation Scale", "BBPS"),
             ("cold snare polypectomy", "CSP"), ("tubular adenomas", "TAs"),
             ("Tubular adenoma", "TA"), ("tubular adenoma", "TA"),
             ("Sessile serrated lesion", "SSL"), ("sessile serrated lesion", "SSL"),
             ("low-grade dysplasia", "LGD"), ("hyperplastic-appearing", "HP-looking")]


# ---------------------------------------------------------------- planning
def make_plan(target):
    P = {"target": target}
    if target in ("adequate", "awaiting_histology"):
        P["mode"] = "ok"
        if p(0.13):
            P["cae_doc"], P["cae_real"] = "not_stated", "yes"
        else:
            P["cae_doc"], P["cae_real"] = "yes", "yes"
        if p(0.17):
            P["prep_doc"], P["prep_real"] = "not_stated", "adequate"
        else:
            P["prep_doc"], P["prep_real"] = "adequate", "adequate"
        if target == "awaiting_histology":
            P["tissue"], P["hist"] = True, False
        else:
            P["tissue"] = p(0.45)
            P["hist"] = P["tissue"]
    else:
        mode = wch([("cae", 32), ("prep", 40), ("both", 8), ("hidden", 20)])
        P["mode"] = mode
        if mode == "cae":
            P["cae_doc"], P["cae_real"] = "no", "no"
            if p(0.25):
                P["prep_doc"] = "not_stated"
                P["prep_real"] = "adequate" if p(0.85) else "inadequate"
            else:
                P["prep_doc"], P["prep_real"] = "adequate", "adequate"
        elif mode == "prep":
            P["prep_doc"], P["prep_real"] = "inadequate", "inadequate"
            if p(0.2):
                P["cae_doc"] = "not_stated"
                P["cae_real"] = "yes" if p(0.85) else "no"
            else:
                P["cae_doc"], P["cae_real"] = "yes", "yes"
        elif mode == "both":
            P["cae_doc"], P["cae_real"] = "no", "no"
            P["prep_doc"], P["prep_real"] = "inadequate", "inadequate"
        else:  # documentation silent, reality bad
            if p(0.4):
                P["cae_doc"], P["cae_real"] = "not_stated", "no"
                if p(0.7):
                    P["prep_doc"], P["prep_real"] = "adequate", "adequate"
                else:
                    P["prep_doc"], P["prep_real"] = "not_stated", "adequate"
            else:
                P["prep_doc"], P["prep_real"] = "not_stated", "inadequate"
                if p(0.8):
                    P["cae_doc"], P["cae_real"] = "yes", "yes"
                else:
                    P["cae_doc"], P["cae_real"] = "not_stated", "yes"
        P["tissue"] = p(0.45)
        P["hist"] = P["tissue"] and p(0.25)
    return P


def make_polyp(max_idx):
    weights = [(0, 18), (1, 30), (2, 10), (3, 5), (4, 12), (5, 6), (6, 10), (7, 9)]
    idx = wch([(i, w) for i, w in weights if i <= max_idx])
    size = wch([(s, 10) for s in range(2, 6)] + [(s, 8) for s in range(6, 10)]
               + [(s, 3) for s in range(10, 19)])
    if size <= 5:
        paris = ch(["0-Is", "0-IIa", "0-IIa", "0-Is"])
    elif size < 10:
        paris = ch(["0-Is", "0-IIa", "0-Isp", "0-Ip"])
    else:
        paris = ch(["0-Is", "0-Ip", "0-Isp", "0-IIa"])
    if size <= 4:
        method = "cfp" if p(0.4) else "csp"
    elif size < 10:
        method = "csp"
    else:
        method = ch(["hsp", "emr"]) if paris != "0-Ip" else "hsp"
    optical = None
    if p(0.05):
        if idx >= 5 and size >= 5 and p(0.5):
            optical = "serrated"
        elif idx <= 1 and size <= 5 and p(0.4):
            optical = "nice1"
        else:
            optical = "nice2"
    # histology result (used only if the addendum is present)
    if optical == "nice1" or (idx <= 1 and size <= 5 and p(0.45)):
        res = "hp"
    elif optical == "serrated" or (idx >= 5 and p(0.25)):
        res = "ssl"
    elif size >= 10 and p(0.4):
        res = "tva"
    else:
        res = "ta"
    lo, hi = SEG_CM[idx]
    return {"idx": idx, "size": size, "paris": paris, "method": method,
            "optical": optical, "res": res, "cm": rng.randint(lo, hi)}


def plan_tissue(P):
    cd = P["cae_doc"]
    if cd == "yes":
        max_idx = 7
    elif cd == "no":
        max_idx = P["ext_idx"]
    else:
        max_idx = 5  # never imply the right colon / caecum when extent is silent
    P["max_idx"] = max_idx
    P["removed"], P["left"], P["biopsies"] = [], [], []
    if P["tissue"]:
        kind = wch([("polyp", 60), ("biopsy", 28), ("both", 12)])
        if kind in ("polyp", "both"):
            for _ in range(wch([(1, 55), (2, 30), (3, 15)])):
                P["removed"].append(make_polyp(max_idx))
        if kind in ("biopsy", "both"):
            opts = [("random", 34), ("erythema", 18), ("rectal_ulcer", 12)]
            if cd == "yes":
                opts.append(("ti", 20))
            if max_idx >= 2:
                opts.append(("stricture", 10))
            P["biopsies"].append(wch(opts))
    if p(0.035):
        P["left"] = [{"idx": ch([0, 0, 1]), "size": ch([2, 3, 3, 4])}
                     for _ in range(wch([(1, 60), (2, 40)]))]


# ---------------------------------------------------------------- report text
class Doc:
    def __init__(self, P):
        self.P = P
        self.tags = set()


def sp_us(s):
    for a, b in US_REPS:
        s = s.replace(a, b)
    return s


def abbreviate(s):
    for a, b in ABBR_REPS:
        s = s.replace(a, b)
    return s


def polyp_word(n):
    return {1: "One", 2: "Two", 3: "Three"}[n]


METHOD_FULL = {"cfp": "cold forceps polypectomy", "csp": "cold snare polypectomy",
               "hsp": "hot snare polypectomy", "emr":
               "endoscopic mucosal resection (EMR) after submucosal lift"}
METHOD_SHORT = {"cfp": "CFP", "csp": "CSP", "hsp": "HSP", "emr": "EMR"}
MORPH = {"0-Is": "sessile", "0-IIa": "flat elevated", "0-Ip": "pedunculated",
         "0-Isp": "semi-pedunculated"}
OPT_FULL = {"nice2": ", adenomatous in appearance (NICE type 2)",
            "nice1": ", hyperplastic-appearing (NICE type 1)",
            "serrated": ", with a cloud-like surface and mucus cap suggestive of a "
                        "sessile serrated lesion"}
OPT_SHORT = {"nice2": " NICE 2", "nice1": " NICE 1", "serrated": " ?SSL (mucus cap)"}
RES_TEXT = {"ta": ["Tubular adenoma with low-grade dysplasia; completely excised.",
                   "Tubular adenoma, low-grade dysplasia. Margins clear."],
            "hp": ["Hyperplastic polyp. No dysplasia.",
                   "Hyperplastic polyp; no dysplasia identified."],
            "ssl": ["Sessile serrated lesion without dysplasia.",
                    "Sessile serrated lesion, no dysplasia."],
            "tva": ["Tubulovillous adenoma with low-grade dysplasia; margins clear.",
                    "Tubulovillous adenoma, low-grade dysplasia, completely excised."]}
BX_RES = {"random": "Colonic mucosa within normal limits; no features of microscopic "
                    "(lymphocytic or collagenous) colitis.",
          "ti": "Ileal mucosa with mild focal active inflammation; no granulomas, "
                "no dysplasia.",
          "erythema": "Mild nonspecific chronic inflammation; no dysplasia.",
          "stricture": "Colonic mucosa with mild chronic inflammation in keeping with "
                       "diverticular-associated change; no dysplasia.",
          "rectal_ulcer": "Appearances consistent with solitary rectal ulcer "
                          "syndrome; no dysplasia."}
BX_DESC = {"random": "random colonic biopsies", "ti": "terminal ileal biopsies",
           "erythema": "sigmoid biopsies (erythema)",
           "stricture": "sigmoid stricture biopsies",
           "rectal_ulcer": "rectal ulcer biopsies"}


def build(i, target):
    P = make_plan(target)
    D = Doc(P)
    T = D.tags
    P["us"] = p(0.3)
    layout = wch([("template", 30), ("dictation", 18), ("terse", 15),
                  ("numbered", 15), ("form", 12)])
    arabic = None
    if p(0.17):
        arabic = wch([("bilingual", 35), ("dup", 30), ("footer", 20), ("only_key", 25)])
        if arabic == "bilingual":
            layout = "bilingual"
    P["layout"] = layout
    reg = "terse" if layout == "terse" else "full"
    abbrev = layout == "terse" or p(0.18)
    if P["cae_doc"] == "no":
        P["ext_idx"] = wch([(1, 18), (2, 10), (3, 24), (4, 16), (5, 16), (6, 16)])
    plan_tissue(P)

    # --- styles for extent and prep
    cd, pd = P["cae_doc"], P["prep_doc"]
    if cd == "yes":
        opts = [("explicit", 30), ("landmarks", 22), ("time", 8), ("difficult", 4),
                ("hedged", 3)]
        opts.append(("ti", 14))
        if layout == "form":
            opts.append(("checkbox", 45))
        cst = wch(opts)
        if P["removed"] and "ti" not in P["biopsies"] and p(0.18):
            P["removed"][0]["idx"] = 7
            lo, hi = SEG_CM[7]
            P["removed"][0]["cm"] = rng.randint(lo, hi)
            cst = "implied_polyp"
    elif cd == "no":
        if P["ext_idx"] == 6:
            cst = "near"
        else:
            opts = [("cannot_advance", 30), ("extent_field", 25), ("abandoned", 15)]
            if layout in ("template", "numbered", "form", "bilingual"):
                opts.append(("planned_vs_actual", 18))
            if layout == "form":
                opts.append(("checkbox", 40))
            cst = wch(opts)
    else:
        opts = [("omitted", 55), ("insertion_only", 25)]
        if layout in ("template", "numbered", "bilingual", "form"):
            opts.append(("blank", 20))
        if layout == "form":
            opts.append(("unchecked", 40))
        cst = wch(opts)
    if pd == "adequate":
        opts = [("verbal", 52), ("bbps", 22), ("other", 4)]
        if layout == "form":
            opts.append(("checkbox", 35))
    elif pd == "inadequate":
        opts = [("verbal", 18), ("bbps_total", 32), ("bbps_segment", 40), ("mixed", 18),
                ("other", 4)]
        if layout == "form":
            opts.append(("checkbox", 25))
    else:
        opts = [("product", 35), ("compliance", 12), ("omitted", 25), ("protocol", 10)]
        if layout in ("template", "numbered", "bilingual", "form"):
            opts.append(("blank", 14))
        if layout == "form":
            opts.append(("unchecked", 30))
    if cd != "yes":
        # a Boston right-colon score or a right-colon prep comment would imply the
        # right colon was examined; keep silent/incomplete extents uncontaminated
        opts = [o for o in opts if not o[0].startswith("bbps") and o[0] != "mixed"]
    pst = wch(opts)

    # Arabic-only key statement
    ar_only = None
    if arabic == "only_key":
        cands = []
        if cd in ("yes", "no") and cst not in ("implied_polyp", "checkbox",
                                               "planned_vs_actual", "near"):
            cands.append("cae")
        if pd in ("adequate", "inadequate") and pst in ("verbal", "checkbox"):
            cands.append("prep")
        if cands:
            ar_only = ch(cands)
        else:
            arabic = "dup"
    P["cst"], P["pst"], P["ar_only"], P["arabic"] = cst, pst, ar_only, arabic

    # --- grades
    if pst in ("verbal", "checkbox"):
        if pd == "adequate":
            P["grade"] = wch([("excellent", 18), ("good", 45), ("adequate", 26), ("fair", 6)])
        else:
            P["grade"] = wch([("poor", 70), ("inadequate", 30)])
    if pst == "bbps":
        P["bbps"] = [rng.randint(2, 3) for _ in range(3)]
    elif pst == "bbps_total":
        while True:
            b = [rng.randint(0, 3) for _ in range(3)]
            if sum(b) < 6:
                break
        P["bbps"] = b
    elif pst == "bbps_segment":
        while True:
            b = [rng.randint(0, 3) for _ in range(3)]
            if sum(b) >= 6 and min(b) < 2:
                break
        P["bbps"] = b

    # --- indication
    if "random" in P["biopsies"]:
        ind = "fit_diarrhoea"
    else:
        ind = wch([("fit", 50), ("fit_ida", 10), ("fit_famhx", 8), ("surv", 20),
                   ("fit_bowel", 12)])
    P["ind"] = ind
    P["prior_hist"] = ind == "surv" and p(0.25)
    P["prior_incomplete"] = cd == "yes" and p(0.035)
    P["prior_poor"] = pd == "adequate" and p(0.035)

    products = PRODUCTS_US if P["us"] else PRODUCTS_UK
    P["product"] = ch(products)
    age = rng.randint(45, 74)
    sex = ch(["M", "F"])
    day, mon = rng.randint(1, 28), rng.randint(1, 10)
    P["date"] = f"{day:02d}/{mon:02d}/2026"
    hday = day + rng.randint(5, 12)
    hmon = mon
    if hday > 28:
        hday -= 28
        hmon += 1
    P["hdate"] = f"{hday:02d}/{hmon:02d}/2026"
    endo = ch(ENDOS)

    S = {}  # section -> list of (text, is_key)

    # ---------------- indication
    yr = rng.randint(2019, 2024)
    L = []
    if reg == "full":
        if ind == "fit":
            L.append(ch(["Positive faecal immunochemical test (FIT) from the Abu Dhabi "
                         "colorectal cancer screening programme.",
                         f"FIT positive ({rng.randint(25, 300)} µg Hb/g faeces); "
                         "asymptomatic screening participant.",
                         "Referred following a positive FIT on routine screening."]))
        elif ind == "fit_ida":
            L.append(f"Positive FIT with iron deficiency anaemia "
                     f"(haemoglobin {rng.randint(90, 115) / 10:.1f} g/dL).")
        elif ind == "fit_famhx":
            L.append(f"Positive FIT; father diagnosed with colorectal cancer aged "
                     f"{rng.randint(52, 68)}.")
        elif ind == "fit_bowel":
            L.append("Positive FIT and a recent change in bowel habit (looser stools).")
        elif ind == "fit_diarrhoea":
            L.append("Positive FIT; also three months of non-bloody watery diarrhoea.")
        else:
            if P["prior_hist"]:
                n = rng.randint(2, 3)
                L.append(ch([f"Surveillance: colonoscopy in {yr} removed {n} tubular "
                             f"adenomas (largest {rng.randint(6, 14)} mm).",
                             f"Post-polypectomy surveillance; previous histology ({yr}): "
                             f"{n} tubular adenomas."]))
            else:
                L.append(f"Surveillance colonoscopy; previous polypectomy in {yr}.")
        if P["prior_incomplete"]:
            L.append(f"Previous colonoscopy ({yr}) was incomplete - the scope could not "
                     "pass the splenic flexure.")
        if P["prior_poor"]:
            L.append(f"A previous attempt in {yr} was abandoned because of poor bowel "
                     "preparation.")
    else:
        L.append({"fit": "FIT+", "fit_ida": "FIT+ / IDA", "fit_famhx": "FIT+, FHx CRC (father)",
                  "fit_bowel": "FIT+ & looser stools", "fit_diarrhoea": "FIT+ & watery diarrhoea 3/12",
                  "surv": (f"Surv - prev TA x{rng.randint(2, 3)} ({yr})" if P["prior_hist"]
                           else f"Surv - prev polypectomy {yr}")}[ind])
        if P["prior_incomplete"]:
            L.append(f"prev colonoscopy {yr} incomplete (stopped at SF)")
        if P["prior_poor"]:
            L.append(f"prev attempt {yr} abandoned - poor prep")
    S["indication"] = [(x, True) for x in L]
    if P["prior_hist"]:
        T.add("prior_histology_mentioned")
    if P["prior_incomplete"]:
        T.add("prior_incomplete_mentioned")
    if P["prior_poor"]:
        T.add("prior_poor_prep_mentioned")

    # ---------------- sedation
    sed = ch(["Midazolam 2 mg IV and fentanyl 50 mcg IV.",
              "Midazolam 3 mg IV + pethidine 25 mg IV.",
              "Propofol sedation administered by the anaesthetist.",
              "Entonox only.", "Unsedated at patient request.",
              "Conscious sedation: midazolam 2.5 mg IV."])
    if reg == "terse":
        sed = ch(["midaz 2mg + fent 50mcg", "propofol (anaesthetist)", "Entonox",
                  "nil (pt choice)", "midaz 3mg/peth 25mg"])
    S["sedation"] = [(sed, False)]
    if pd == "not_stated" and p(0.18):
        S["sedation"].append(("Sedation adequate; patient comfortable (comfort score 2/5)."
                              if reg == "full" else "sedation adequate, comfortable", True))
        T.add("adequate_word_elsewhere")

    # ---------------- prep
    L = []
    prod = P["product"]
    g = P.get("grade")
    if pst == "verbal":
        if reg == "full":
            if pd == "adequate":
                if g == "fair":
                    L.append(ch([f"Bowel preparation: {prod}. Quality: fair - some liquid "
                                 "stool, suctioned; mucosa adequately seen.",
                                 "Preparation was fair, with residual liquid stool that was "
                                 "cleared by suction."]))
                else:
                    L.append(ch([f"Bowel preparation: {prod}. Quality: {g.capitalize()}.",
                                 f"The bowel preparation was {g} throughout.",
                                 f"Quality of bowel preparation: {g.capitalize()}.",
                                 f"{prod} taken. Preparation {g}."]))
            else:
                L.append(ch([f"Bowel preparation was {g}, with solid stool that could not "
                             "be cleared; small lesions may have been missed.",
                             f"Quality of preparation: {g.capitalize()} (thick residue "
                             "throughout).",
                             f"Prep: {prod}. Quality: {g.capitalize()}."]))
        else:
            if pd == "adequate":
                L.append(ch([f"{g}", f"{g} ({prod.split(',')[0].split(' ')[0]})",
                             f"{g.upper()}"]))
            else:
                L.append(ch([f"{g.upper()} - solid stool ++", f"{g}, ++ residue"]))
        if g == "fair":
            T.add("prep_fair")
        if P["ar_only"] == "prep":
            L = [f"جودة تحضير الأمعاء: {GRADE_AR[g]}"]
            if p(0.6):
                L.insert(0, f"{prod}." if reg == "full" else prod)
    elif pst in ("bbps", "bbps_total", "bbps_segment"):
        r_, t_, l_ = P["bbps"]
        s_ = r_ + t_ + l_
        fmt = wch([("named", 25), ("dash", 20), ("total_first", 20), ("no_total", 15),
                   ("left_first", 20)])
        if fmt == "named":
            txt = (f"Boston Bowel Preparation Scale: right {r_}, transverse {t_}, "
                   f"left {l_} (total {s_}/9).")
        elif fmt == "dash":
            txt = f"BBPS {r_}-{t_}-{l_} = {s_} (R-T-L)"
        elif fmt == "total_first":
            txt = f"BBPS total {s_} (R{r_} T{t_} L{l_})"
            T.add("bbps_total_first")
        elif fmt == "no_total":
            txt = f"Boston score: R {r_} / T {t_} / L {l_}"
            T.add("bbps_no_total")
        else:
            txt = f"BBPS L{l_} T{t_} R{r_}, total {s_}"
        if reg == "full" and p(0.6):
            L.append(f"Preparation: {prod}.")
        L.append(txt)
        T.add("bbps")
        T.add({"bbps": "bbps_pass", "bbps_total": "bbps_fail_total",
               "bbps_segment": "bbps_fail_segment"}[pst])
    elif pst == "mixed":
        L.append("Preparation was good in the rectum and left colon but poor in the right "
                 "colon, where adherent solid stool obscured the mucosa."
                 if reg == "full" else "L colon good / R colon POOR (solid stool)")
        T.add("prep_mixed_segments")
    elif pst == "other":
        if pd == "adequate":
            L.append(ch(["Aronchick scale: good.", "Ottawa bowel preparation score 3 - good.",
                         "Aronchick: excellent."]))
        else:
            L.append(ch(["Aronchick scale: poor.", "Ottawa score 10 - poor."]))
        T.add("other_scale")
    elif pst == "checkbox":
        boxes = ["Excellent", "Good", "Fair", "Poor", "Inadequate"]
        gg = g.capitalize() if g != "adequate" else "Fair"
        if g == "adequate":
            boxes = ["Excellent", "Good", "Adequate", "Poor", "Inadequate"]
            gg = "Adequate"
        L.append("Prep quality:  " + "  ".join(
            f"[{'X' if b == gg else ' '}] {b}" for b in boxes))
        if g == "fair":
            T.add("prep_fair")
        if P["ar_only"] == "prep":
            L = [f"جودة تحضير الأمعاء: {GRADE_AR[g]}"]
        T.add("prep_checkbox")
    elif pst == "product":
        L.append(ch([f"Bowel preparation: {prod}.", f"{prod}, low-residue diet for 2 days.",
                     f"Prep regimen: {prod}."]) if reg == "full" else prod)
        T.update(["prep_ungraded", "prep_product_only"])
    elif pst == "compliance":
        L.append(ch([f"Patient reports good compliance with {prod}; last dose 4 h before "
                     "the procedure.",
                     f"{prod} taken as instructed, good compliance reported by patient."])
                 if reg == "full" else f"{prod} - pt reports good compliance")
        T.update(["prep_ungraded", "prep_compliance_only"])
    elif pst == "omitted":
        T.update(["prep_ungraded", "prep_omitted"])
    elif pst == "protocol":
        L.append("Preparation: as per unit protocol." if reg == "full" else "per protocol")
        T.update(["prep_ungraded"])
    elif pst == "blank":
        L.append(ch(["Prep quality: ____________", "BBPS:  R __  T __  L __  Total __"]))
        T.update(["prep_ungraded", "prep_blank_field"])
    elif pst == "unchecked":
        L.append("Prep quality:  [ ] Excellent  [ ] Good  [ ] Fair  [ ] Poor")
        T.update(["prep_ungraded", "prep_unchecked_boxes"])
    S["prep"] = [(x, True) for x in L]

    # ---------------- extent
    L = []
    seg = SEGS[P["ext_idx"]] if cd == "no" else None
    if cd == "no":
        rs = ["recurrent looping", "patient discomfort despite sedation",
              "a long redundant colon"]
        if P["ext_idx"] <= 2:
            rs += ["fixed sigmoid angulation", "severe diverticular disease"]
        if pd == "inadequate":
            rs += ["poor bowel preparation"]
        reason = ch(rs)
    if cst == "explicit":
        if reg == "full":
            L.append(ch(["The colonoscope was advanced to the caecum, confirmed by "
                         "identification of the appendiceal orifice and ileocaecal valve; "
                         "photo-documentation obtained.",
                         "Complete colonoscopy to the caecum. Caecal landmarks photographed.",
                         f"Caecum reached at {rng.randint(4, 15)} minutes. Appendiceal "
                         "orifice and ileocaecal valve identified.",
                         "Extent of examination: caecum (photo-documented)."]))
        else:
            L.append(ch(["caecum, photo-doc'd", "caecum (AO + ICV seen)",
                         "to caecum, pics taken"]))
    elif cst == "landmarks":
        if reg == "full":
            L.append(ch(["Appendiceal orifice and ileocaecal valve clearly seen and "
                         "photographed.",
                         f"Landmarks identified: appendiceal orifice, ileocaecal valve "
                         f"(images {rng.randint(5, 9)}, {rng.randint(10, 14)}).",
                         "The ileocaecal valve and appendiceal orifice were visualised; "
                         "photographs taken."]))
        else:
            L.append("AO + ICV identified, photo-doc'd")
        T.add("caecum_by_landmarks")
    elif cst == "ti":
        n = rng.randint(3, 10)
        if reg == "full":
            L.append(ch([f"The terminal ileum was intubated for {n} cm and appeared normal.",
                         "Ileal intubation achieved; terminal ileum normal.",
                         "Extent: terminal ileum."]))
        else:
            L.append(ch([f"TI intubated {n}cm - NAD", "TI"]))
        T.add("caecum_via_TI")
    elif cst == "time":
        t = rng.randint(4, 16)
        L.append(f"Caecal intubation time: {t} min." if reg == "full"
                 else f"time to caecum {t} min")
    elif cst == "difficult":
        L.append("Insertion was difficult because of a long, redundant sigmoid with "
                 "looping; the caecum was eventually reached using abdominal pressure and "
                 "a change to the supine position." if reg == "full" else
                 "difficult insertion (looping) - caecum reached w/ abdo pressure + supine")
        T.add("difficult_but_complete")
    elif cst == "hedged":
        L.append("Caecum reached. The appendiceal orifice was partly obscured by fluid, "
                 "but the ileocaecal valve was clearly identified and photographed."
                 if reg == "full" else "caecum - AO partly obscured by fluid, ICV clearly seen")
        T.add("hedged_landmark")
    elif cst == "checkbox" and cd == "yes":
        L.append("Caecal intubation:  [X] Yes  [ ] No      Landmarks:  [X] AO  [X] ICV  "
                 f"[{'X' if p(0.3) else ' '}] TI")
        T.add("caecum_checkbox")
    elif cst == "implied_polyp":
        T.add("caecum_implied_by_polyp")
    elif cst == "near":
        L.append("The scope reached the proximal ascending colon but the caecal pole could "
                 "not be visualised despite position change and abdominal pressure; the "
                 "appendiceal orifice and ileocaecal valve were not identified. "
                 "Examination incomplete." if reg == "full" else
                 "reached prox asc colon; caecal pole/AO/ICV NOT seen - incomplete")
        T.add("near_complete")
    elif cst == "cannot_advance":
        L.append(f"The scope could not be advanced beyond the {seg} because of {reason}; "
                 "the caecum was not reached." if reg == "full" else
                 f"{SEG_SHORT[P['ext_idx']]} only - unable to advance ({reason}). Caecum NOT reached")
    elif cst == "extent_field":
        L.append(f"Extent of examination: {seg}. Caecum not reached ({reason})."
                 if reg == "full" else
                 f"{SEG_SHORT[P['ext_idx']]} (incomplete - {reason})")
    elif cst == "abandoned":
        L.append(f"Procedure abandoned at the {seg} due to {reason}. Incomplete examination."
                 if reg == "full" else f"abandoned at {SEG_SHORT[P['ext_idx']]} ({reason})")
    elif cst == "planned_vs_actual":
        L.append("Intended extent: caecum")
        L.append(f"Actual extent reached: {seg} ({reason})")
        T.add("planned_vs_actual")
    elif cst == "insertion_only":
        L.append(ch(["Insertion was uneventful.",
                     "The colonoscope was inserted under direct vision with minimal looping.",
                     "Scope passed without difficulty; careful withdrawal examination "
                     "performed."]) if reg == "full" else "insertion uneventful")
        T.update(["extent_unstated"])
    elif cst == "omitted":
        T.update(["extent_unstated"])
    elif cst == "blank":
        L.append(ch(["Extent of examination: ____________", "Furthest point reached: [        ]"]))
        T.update(["extent_unstated", "extent_blank_field"])
    elif cst == "unchecked":
        L.append("Caecal intubation:  [ ] Yes  [ ] No      Landmarks:  [ ] AO  [ ] ICV  [ ] TI")
        T.update(["extent_unstated", "extent_unchecked"])
    if cd == "no" and cst == "checkbox":
        L.append(f"Caecal intubation:  [ ] Yes  [X] No      Furthest point reached: {seg}")
        L.append(f"Reason incomplete: {reason}")
        T.add("caecum_checkbox")
    if P["ar_only"] == "cae":
        if cd == "yes":
            L = ["تم الوصول إلى الأعور، وتم التعرف على فتحة الزائدة الدودية والصمام "
                 "اللفائفي الأعوري وتوثيقهما بالصور."]
        else:
            L = [f"لم يتم الوصول إلى الأعور؛ توقف الفحص عند {SEG_AR[P['ext_idx']]}."]
    S["extent"] = [(x, True) for x in L]

    # ---------------- withdrawal time
    if cd == "yes" and p(0.7):
        S["withdrawal"] = [(f"{rng.randint(6, 15)} min" if reg == "terse"
                            else f"Withdrawal time: {rng.randint(6, 15)} minutes.", False)]
    elif cd == "no" and P["ar_only"] != "cae" and p(0.4):
        S["withdrawal"] = [("N/A" if reg == "terse" else
                            "Withdrawal time: not applicable (incomplete examination).", True)]
    elif cd == "not_stated" and p(0.4):
        S["withdrawal"] = [("not recorded" if reg == "terse" else
                            "Withdrawal time: not recorded.", False)]
    else:
        S["withdrawal"] = []

    # ---------------- findings
    L = []
    pot = 0
    pots = []  # (pot no, description, result text)
    for pp in P["removed"]:
        pot += 1
        site = SEGS[pp["idx"]]
        opt = pp["optical"]
        if opt:
            T.add("optical_dx_not_histology")
        if reg == "full":
            at = f" at {pp['cm']} cm" if p(0.35) else ""
            if pp["idx"] == 7 and p(0.5):
                txt = (f"A {pp['size']} mm {MORPH[pp['paris']]} caecal polyp (Paris "
                       f"{pp['paris']}){OPT_FULL.get(opt, '')} was removed by "
                       f"{METHOD_FULL[pp['method']]} and retrieved (pot {pot}).")
            else:
                txt = ch([f"A {pp['size']} mm {MORPH[pp['paris']]} polyp (Paris {pp['paris']}) "
                          f"in the {site}{at}{OPT_FULL.get(opt, '')}, removed by "
                          f"{METHOD_FULL[pp['method']]} and retrieved (pot {pot}).",
                          f"In the {site}{at} there was a {pp['size']} mm "
                          f"{MORPH[pp['paris']]} polyp{OPT_FULL.get(opt, '')}. "
                          f"{METHOD_FULL[pp['method']][0].upper()}"
                          f"{METHOD_FULL[pp['method']][1:]} performed; polyp retrieved "
                          f"for histology (pot {pot})."])
        else:
            txt = (f"{SEG_SHORT[pp['idx']]} {pp['size']}mm {pp['paris']}"
                   f"{OPT_SHORT.get(opt, '')} -> {METHOD_SHORT[pp['method']]}, retr (pot {pot})")
        L.append(txt)
        adj = {"rectum": "rectal", "caecum": "caecal"}.get(site, site)
        pots.append((pot, f"{adj} polyp, {pp['size']} mm", ch(RES_TEXT[pp["res"]])))
    for bx in P["biopsies"]:
        pot += 1
        if bx == "random":
            where = ("right and left colon" if cd == "yes" else
                     ch(["sigmoid and descending colon", "left colon"]) if P["max_idx"] >= 2
                     else "sigmoid colon")
            txt = (f"Colonic mucosa appeared normal. In view of the diarrhoea, random "
                   f"biopsies were taken from the {where} for microscopic colitis (pot {pot})."
                   if reg == "full" else f"random bx {where} (?MC) - pot {pot}")
            T.add("random_biopsies")
        elif bx == "ti":
            txt = (f"A small aphthous ulcer was seen in the terminal ileum; biopsies taken "
                   f"(pot {pot})." if reg == "full" else f"TI: small aphthous ulcer, bx (pot {pot})")
            T.add("ti_biopsy")
        elif bx == "erythema":
            txt = (f"Patchy erythema in the sigmoid colon, biopsied x2 (pot {pot})."
                   if reg == "full" else f"sigmoid patchy erythema - bx x2 (pot {pot})")
        elif bx == "stricture":
            txt = (f"Short, smooth stricture in the sigmoid colon within a segment of "
                   f"diverticular disease, traversed with the standard colonoscope; biopsies "
                   f"taken (pot {pot})." if reg == "full" else
                   f"sigmoid: short smooth diverticular stricture, traversed, bx (pot {pot})")
        else:
            txt = (f"Small shallow ulcer on the anterior rectal wall (?solitary rectal ulcer), "
                   f"biopsied (pot {pot})." if reg == "full" else
                   f"rectum: small ant wall ulcer ?SRUS - bx (pot {pot})")
        L.append(txt)
        pots.append((pot, BX_DESC[bx], BX_RES[bx]))
        T.add("biopsy_nonpolyp")
    if P["left"]:
        n = len(P["left"])
        sz = max(x["size"] for x in P["left"])
        site = SEGS[P["left"][0]["idx"]]
        if reg == "full":
            L.append(ch([f"{polyp_word(n)} diminutive ({sz} mm) hyperplastic-appearing "
                         f"polyp{'s' if n > 1 else ''} in the {site} (NICE type 1); left in situ.",
                         f"{polyp_word(n)} {sz} mm pale polyp{'s' if n > 1 else ''} in the "
                         f"{site}, hyperplastic in appearance; not removed."]))
        else:
            L.append(f"{n}x {sz}mm HP-looking polyp{'s' if n > 1 else ''} {SEG_SHORT[P['left'][0]['idx']]} - left in situ")
        T.add("polyp_left_in_situ")
    others = []
    if P["max_idx"] >= 1 and p(0.3):
        others.append("Scattered sigmoid diverticulosis." if reg == "full" else "sigmoid diverticulosis")
    if p(0.3):
        others.append("Small internal haemorrhoids on retroflexion." if reg == "full"
                      else "small int haemorrhoids (retroflex)")
    if P["max_idx"] >= 4 and p(0.03):
        others.append("A 10 mm yellow submucosal lesion in the transverse colon with a "
                      "positive pillow sign, consistent with a lipoma; not sampled."
                      if reg == "full" else "TC lipoma (pillow sign +) - not sampled")
        T.add("lipoma_not_sampled")
    L += others
    if not P["removed"] and not P["biopsies"] and not P["left"]:
        L.insert(0, ch(["No polyps or mass lesions seen.", "No polyps identified."])
                 if reg == "full" else "no polyps / no mass")
    tissue = bool(P["removed"] or P["biopsies"])
    neg = None
    if not tissue and p(0.6):
        if layout == "form":
            neg = "Specimens taken:  [ ] Yes  [X] No"
        elif reg == "full":
            neg = ch(["No biopsies taken.", "No polypectomy performed and no biopsies taken.",
                      "No tissue was sampled.", "No specimens sent to pathology.",
                      "No biopsies were required."])
        else:
            neg = ch(["bx: nil", "no bx", "specimens: nil", "polypectomy N/A; bx none"])
        T.add("negated_tissue")
    if P["removed"] and not P["biopsies"] and p(0.06):
        L.append("No biopsies were taken from the background mucosa." if reg == "full"
                 else "no bx of background mucosa")
        T.add("negation_but_tissue")
    S["findings"] = [(x, True) for x in L]

    # ---------------- specimens
    L = []
    pending_line = None
    if tissue:
        if layout == "form":
            L.append("Specimens taken:  [X] Yes  [ ] No")
        for n_, desc, _ in pots:
            L.append(f"Pot {n_}: {desc}." if reg == "full" else f"pot {n_}: {desc}")
        if (not P["hist"] and p(0.65)) or (P["hist"] and p(0.15)):
            pending_line = ch(["Sent to histopathology.",
                               "All specimens sent for histology; results awaited.",
                               "Histology pending.", "Specimens dispatched to the laboratory."])
            if reg == "terse":
                pending_line = ch(["-> histo", "histo pending", "sent to lab"])
            L.append(pending_line)
            T.add("histology_pending_stated")
            if P["hist"]:
                T.add("pending_then_result")
    else:
        if neg:
            L.append(neg)
    S["specimens"] = [(x, True) for x in L]

    # ---------------- complications
    comp = ch(["None.", "No immediate complications.", "Nil."])
    if P["removed"] and p(0.15):
        comp = "Minor oozing after polypectomy, controlled with one haemostatic clip."
    if reg == "terse":
        comp = "nil" if comp != "Minor oozing after polypectomy, controlled with one haemostatic clip." else "minor ooze post-polypectomy - 1 clip"
    S["complications"] = [(comp, False)]

    # ---------------- impression
    L = []
    caecum_word_ok = cst not in ("implied_polyp",) and P["ar_only"] != "cae"
    if cd == "no" and P["ar_only"] != "cae":
        L.append(f"Incomplete colonoscopy - examination to the {seg} only."
                 if reg == "full" else f"incomplete (to {SEG_SHORT[P['ext_idx']]})")
    if pd == "inadequate" and P["ar_only"] != "prep" and (
            not pst.startswith("bbps") or p(0.5)) and p(0.75):
        L.append(ch(["Inadequate bowel preparation.", "Poor bowel preparation limits the "
                     "examination."]) if reg == "full" else "inadequate prep")
    if P["removed"]:
        n = len(P["removed"])
        sizes = ", ".join(str(x["size"]) for x in P["removed"])
        if any(x["idx"] == 7 for x in P["removed"]) and cst == "implied_polyp":
            L.append((f"{polyp_word(n)} polyps removed, including a caecal polyp ({sizes} mm)."
                      if n > 1 else f"Caecal polyp removed ({sizes} mm).")
                     if reg == "full" else f"{n} polyp(s) removed ({sizes}mm)")
        else:
            L.append(f"{polyp_word(n)} polyp{'s' if n > 1 else ''} removed ({sizes} mm)."
                     if reg == "full" else f"{n} polyp(s) removed ({sizes}mm)")
    for bx in P["biopsies"]:
        L.append({"random": "Normal-appearing mucosa; random biopsies taken.",
                  "ti": "Terminal ileal aphthous ulcer - biopsied.",
                  "erythema": "Patchy sigmoid erythema - biopsied.",
                  "stricture": "Benign-appearing diverticular stricture - biopsied.",
                  "rectal_ulcer": "Small rectal ulcer, ?SRUS - biopsied."}[bx]
                 if reg == "full" else BX_DESC[bx])
    if P["left"]:
        L.append("Diminutive hyperplastic-appearing polyp(s) left in situ."
                 if reg == "full" else "diminutive HP-looking polyp(s) left")
    if not L:
        if cd == "yes" and caecum_word_ok and p(0.5):
            L.append("Normal colonoscopy to the caecum." if reg == "full" else "NAD to caecum")
        else:
            L.append(ch(["No polyp or mass seen.", "No significant abnormality."])
                     if reg == "full" else "NAD")
    S["impression"] = [(x, True) for x in L]

    # ---------------- recommendations
    L = []
    if cd == "no":
        if P["ar_only"] == "cae":
            L.append("CT colonography." if reg == "full" else "CTC")
        else:
            r = wch([("ctc_caecum", 22), ("ctc", 40), ("repeat", 38)])
            if r == "ctc_caecum":
                L.append("CT colonography to complete assessment of the proximal colon "
                         "and caecum." if reg == "full" else "CTC to complete prox colon/caecum")
                T.add("caecum_mentioned_not_reached")
            elif r == "ctc":
                L.append("CT colonography to image the unexamined proximal colon."
                         if reg == "full" else "CTC")
            else:
                L.append("Repeat colonoscopy under propofol with a paediatric colonoscope."
                         if reg == "full" else "repeat under propofol, paeds scope")
    if pd == "inadequate":
        L.append("Repeat colonoscopy within 3 months with a 2-day low-residue diet and "
                 "enhanced split-dose preparation." if reg == "full"
                 else "repeat <3/12 w/ 2-day diet + enhanced prep")
    if tissue:
        L.append(ch(["Await histology; surveillance interval to be confirmed on results.",
                     f"Clinic review with histology in {rng.randint(2, 4)} weeks.",
                     "Surveillance interval to be decided once histology is available."])
                 if reg == "full" else "await histo")
    if not L:
        if cd == "yes" and pd == "adequate":
            L.append(ch(["Return to routine screening; repeat colonoscopy in 10 years.",
                         "No further colonoscopy needed for 10 years; return to FIT "
                         "screening programme."]) if reg == "full" else "routine screening 10y")
        else:
            L.append(ch(["Follow up with the referring physician.",
                         "Results discussed with the patient; GP letter sent."])
                     if reg == "full" else "f/u referring dr")
    S["recs"] = [(x, True) for x in L]

    # ---------------- histology heading (pending status) and addendum
    S["histology"] = []
    if tissue and not P["hist"] and layout in ("template", "numbered", "form", "bilingual") \
            and p(0.35):
        S["histology"] = [(ch(["Awaited.", "Pending.", "To follow."]), True)]
        T.add("histology_pending_stated")
    elif not tissue and layout in ("template", "numbered", "form") and p(0.25):
        S["histology"] = [("N/A - no specimens.", True)]
    addendum = []
    if P["hist"]:
        hd = ch([f"ADDENDUM - HISTOPATHOLOGY ({P['hdate']})",
                 f"Histopathology addendum - reported {P['hdate']}",
                 f"PATHOLOGY REPORT (received {P['hdate']})",
                 f"Histology result ({P['hdate']}):"])
        if arabic in ("bilingual", "dup") and p(0.5):
            hd = f"إضافة: نتيجة الفحص النسيجي / Histology addendum ({P['hdate']})"
        addendum.append(hd)
        for n_, desc, res in pots:
            addendum.append(f"Pot {n_} ({desc}): {res}")
        if any(x[2] == BX_RES["random"] for x in pots):
            T.add("histology_normal_result")
        ress = [pp["res"] for pp in P["removed"]]
        if any(r in ("tva",) for r in ress) or any(
                pp["res"] in ("ta", "ssl") and pp["size"] >= 10 for pp in P["removed"]):
            cm_ = "Surveillance colonoscopy in 3 years."
        elif any(r in ("ta", "ssl") for r in ress):
            cm_ = "Low-risk findings; return to the screening programme per guideline."
        else:
            cm_ = "Benign findings; no colonoscopic surveillance required."
        addendum.append(f"Comment: {cm_}")
        addendum.append(f"Reported by {ch(PATHS)}, Consultant Histopathologist.")
        T.add("histology_addendum")

    # ---------------- Arabic duplicate statements
    ar_dup = []
    if arabic in ("dup", "bilingual"):
        if cd == "yes" and P["ar_only"] != "cae" and cst != "implied_polyp" and p(0.7):
            ar_dup.append(("extent", "تم الوصول إلى الأعور."))
        if cd == "no" and P["ar_only"] != "cae" and p(0.7):
            ar_dup.append(("extent", f"لم يتم الوصول إلى الأعور؛ توقف الفحص عند {SEG_AR[P['ext_idx']]}."))
        if pst in ("verbal", "checkbox") and P["ar_only"] != "prep" and p(0.7):
            ar_dup.append(("prep", f"جودة تحضير الأمعاء: {GRADE_AR[g]}"))
        if tissue:
            ar_dup.append(("specimens", "تم أخذ عينات وإرسالها إلى المختبر."))
        else:
            ar_dup.append(("specimens", "لم تؤخذ أي خزعات."))
            T.add("negated_tissue")
        ar_dup.append(("complications", "لا توجد مضاعفات." if comp in ("None.", "Nil.",
                       "No immediate complications.", "nil") else "نزيف بسيط تمت السيطرة عليه."))
        for sec, txt in ar_dup:
            S[sec].append((txt, sec != "complications"))
    footer = []
    if arabic == "footer" or (arabic == "bilingual" and p(0.4)):
        bits = ["ملخص للمريض: تم إجراء تنظير القولون."]
        if P["removed"]:
            bits.append("تم استئصال سليلة وإرسالها للفحص النسيجي.")
        elif P["biopsies"]:
            bits.append("تم أخذ خزعات وإرسالها إلى المختبر.")
        else:
            bits.append("لم تؤخذ أي خزعات.")
            T.add("negated_tissue")
        if tissue and not P["hist"]:
            bits.append("بانتظار نتيجة الفحص النسيجي.")
        bits.append("يرجى مراجعة العيادة حسب الموعد.")
        footer = [" ".join(bits)]
        T.add("arabic_footer")
    if arabic:
        T.add("arabic")
    if arabic == "bilingual":
        T.add("arabic_headings")
    if P["ar_only"]:
        T.add("arabic_only_key")
        T.add(f"arabic_only_{P['ar_only']}")

    # ---------------- header / signature
    mrn = f"{rng.randint(100, 999)}-{rng.randint(1000, 9999)}-{rng.randint(10, 99)}"
    unit = ch(UNITS)
    scope = ch(SCOPES)
    nurse = ch(NURSES)
    t_h, t_m = rng.randint(7, 16), rng.randint(0, 59)
    header = []
    if layout == "dictation":
        header.append(f"{unit}")
        header.append(f"Colonoscopy - dictated report. MRN {mrn}. {age}{sex}. "
                      f"Procedure date {P['date']}.")
    elif layout == "terse":
        header.append(f"COLONOSCOPY  {P['date']}  MRN {mrn}  {age}/{sex}  {endo}")
    else:
        title = ch(["COLONOSCOPY REPORT", "Colonoscopy Report", "LOWER GI ENDOSCOPY REPORT",
                    "Procedure Report - Colonoscopy"])
        if layout == "bilingual":
            title = "تقرير تنظير القولون / COLONOSCOPY REPORT"
        header.append(unit)
        header.append(title)
        header.append(f"Patient MRN: {mrn}    Age/Sex: {age}/{sex}    Date: {P['date']}")
        header.append(f"Endoscopist: {endo}    Assistant: {nurse}    Instrument: {scope}")
        if p(0.4):
            header.append("Consent: written informed consent obtained.")
    sig = [ch([f"Electronically signed by {endo} on {P['date']} at {t_h:02d}:{t_m:02d}.",
               f"{endo}, Consultant Gastroenterologist",
               f"Signed: {endo} / {nurse}"])]

    # ---------------- layout assembly
    order = list(ORDER)
    if layout != "dictation" and p(0.3):
        tw = ch(["imp_top", "ext_before_prep", "sed_after_prep", "recs_before_comp"])
        T.add("section_order_varied")
        if tw == "imp_top":
            order.remove("impression")
            order.insert(0, "impression")
        elif tw == "ext_before_prep":
            order.remove("extent")
            order.insert(order.index("prep"), "extent")
        elif tw == "sed_after_prep":
            order.remove("sedation")
            order.insert(order.index("prep") + 1, "sedation")
        else:
            order.remove("recs")
            order.insert(order.index("complications"), "recs")
    lines = [(h, False) for h in header]
    if layout == "dictation":
        paras = [["indication", "sedation"], ["prep", "extent", "withdrawal"],
                 ["findings", "specimens"], ["complications"], ["impression"], ["recs"],
                 ["histology"]]
        lines.append(("", False))
        for grp in paras:
            buf, key = [], False
            for sec in grp:
                items = S.get(sec, [])
                if not items:
                    continue
                txts = [t for t, _ in items]
                if sec == "complications":
                    txts = ["Complications: " + txts[0][0].lower() + txts[0][1:]] + txts[1:]
                if sec == "impression":
                    txts = ["In summary: " + " ".join(txts)]
                if sec == "recs":
                    txts = ["Plan: " + " ".join(txts)]
                if sec == "histology":
                    txts = ["Histology: " + " ".join(txts)]
                if sec == "sedation":
                    txts = ["Sedation: " + txts[0][0].lower() + txts[0][1:]] + txts[1:] \
                        if not txts[0].startswith(("Sedation", "Conscious", "Entonox",
                                                    "Unsedated", "Propofol")) else txts
                txts = [x if x.rstrip().endswith((".", ":", ")")) or re.search(AR, x)
                        else x.rstrip() + "." for x in txts]
                buf += txts
                key = key or any(k for _, k in items)
            if buf:
                lines.append((" ".join(buf), key))
                lines.append(("", False))
        T.add("layout_dictation")
    else:
        T.add(f"layout_{layout}")
        num = 0
        for sec in order:
            items = S.get(sec, [])
            if not items:
                if sec in ("prep", "extent") and layout in ("template", "numbered", "bilingual") \
                        and p(0.15):
                    pass  # heading silently dropped
                continue
            if layout == "terse":
                first = True
                for t, k in items:
                    lines.append((f"{TERSE_LABEL[sec]}: {t}" if first else f"   {t}", k))
                    first = False
                continue
            h = ch(HEAD[sec])
            if layout == "numbered":
                num += 1
                h = f"{num}. {h}"
            elif layout == "bilingual":
                h = f"{h} / {HEAD_AR[sec]}"
            if layout in ("template", "bilingual") and h.isupper():
                h = h + ":"
            if layout == "numbered" or (layout == "template" and p(0.5) and len(items) == 1):
                lines.append((f"{h} - {items[0][0]}" if layout == "numbered"
                              else f"{h} {items[0][0]}" if h.endswith(":")
                              else f"{h}: {items[0][0]}", items[0][1]))
                for t, k in items[1:]:
                    lines.append(("   " + t, k))
            else:
                lines.append((h if h.endswith(":") else h + ":", False))
                for t, k in items:
                    lines.append(("  " + t if layout == "form" else t, k))
            if layout in ("template", "bilingual"):
                lines.append(("", False))
    for f in footer:
        lines.append(("", False))
        lines.append((f, True))
    lines.append(("", False))
    lines += [(s_, False) for s_ in sig]
    if addendum:
        lines.append(("", False))
        lines.append(("-" * 40, False))
        lines += [(a, True) for a in addendum]

    # ---------------- text transforms (abbrev, spelling, typos, noise)
    if abbrev:
        lines = [(abbreviate(t), k) for t, k in lines]
        T.add("abbreviations")
    if P["us"]:
        lines = [(sp_us(t), k) for t, k in lines]
        T.add("us_spelling")
    else:
        T.add("uk_spelling")
    if p(0.10):
        lines = add_typos(lines, P["us"], T)
    if p(0.14):
        lines = add_scan_noise(lines)
        T.add("scan_noise")
    text = "\n".join(t for t, _ in lines).strip("\n") + "\n"
    text = re.sub(r"\bA (8|11|18) mm", r"An \1 mm", text)
    text = re.sub(r"\n{3,}", "\n\n", text)

    # ---------------- keys
    key_fields = {"caecum": cd, "prep": pd,
                  "polyps": "yes" if tissue else "no",
                  "tissue_taken": "yes" if tissue else "no",
                  "histology": "yes" if P["hist"] else "no"}
    ks, kr = key_state(key_fields)
    P["tissue_real"] = tissue
    tr = truth_of(P)
    assert tr == target, (tr, target, P)
    if tr == "inadequate" and ks == "needs_human":
        T.add("hidden_reality_bad")
    if ks == "needs_human" and tr != "inadequate":
        T.add("hidden_reality_ok")
    return {"uid": f"h{i}", "text": text, "truth": tr, "key_fields": key_fields,
            "key_state": ks, "key_reason": kr, "tags": sorted(T)}, P


def add_typos(lines, us, T):
    out = []
    did_key = False
    for t, k in lines:
        if not k and len(t) > 25 and p(0.35) and not re.search(r"[؀-ۿ]", t):
            words = t.split(" ")
            j = rng.randrange(len(words))
            w = words[j]
            if len(w) > 4 and w.isalpha():
                q = rng.randrange(1, len(w) - 2)
                words[j] = w[:q] + w[q + 1] + w[q] + w[q + 2:]
                t = " ".join(words)
        if k and not did_key and p(0.25):
            for a, b in [("caecum", "ceacum"), ("biopsies", "biospies"), ("polyp ", "polpy ")]:
                if a in t and not (us and a == "caecum"):
                    t = t.replace(a, b, 1)
                    did_key = True
                    T.add("typo_key_term")
                    break
        out.append((t, k))
    T.add("typo")
    return out


def garble_word(w):
    kind = ch(["rn", "1", "0", "c", "drop", "space", "dup"])
    if kind == "rn" and "m" in w:
        return w.replace("m", "rn", 1)
    if kind == "1" and "l" in w:
        return w.replace("l", "1", 1)
    if kind == "0" and "o" in w:
        return w.replace("o", "0", 1)
    if kind == "c" and "e" in w:
        return w.replace("e", "c", 1)
    if kind == "drop" and len(w) > 5:
        q = rng.randrange(1, len(w) - 1)
        return w[:q] + w[q + 1:]
    if kind == "space" and len(w) > 5:
        q = rng.randrange(2, len(w) - 2)
        return w[:q] + " " + w[q:]
    if kind == "dup" and len(w) > 3:
        q = rng.randrange(len(w))
        return w[:q] + w[q] + w[q:]
    return w + ch(["~", ".", ","])


def add_scan_noise(lines):
    out = []
    for t, k in lines:
        if not k and t.strip() and not re.search(r"[؀-ۿ]", t):
            words = t.split(" ")
            for j, w in enumerate(words):
                if len(w) > 3 and p(0.22):
                    words[j] = garble_word(w)
            t = " ".join(words)
            if p(0.15):
                t = t + "  |"
        out.append((t, k))
    if p(0.7):
        out.insert(0, (ch(["FAX RECEIVED  " + f"{rng.randint(1, 28):02d}/10/2026 "
                           f"{rng.randint(8, 17)}:{rng.randint(10, 59)}  P.00{rng.randint(1, 3)}",
                           "*** SCANNED DOCUMENT - Medical Records ***",
                           "[scan: page 1/2]  ....  ..  .", "Scanned copy - quality: low"]),
                       False))
    pos = rng.randrange(len(out) // 2, len(out))
    while pos < len(out) and out[pos][0].strip() == "":
        pos += 1
    # page break only between lines, never inside one
    out.insert(pos, (ch(["--- Page 1 of 2 ---", "-- p.2 --", "~ ~ ~ ~ ~ ~ ~ ~", "[page break]"]),
                     False))
    return out


# ---------------------------------------------------------------- keys / truth
def key_state(k):
    if k["caecum"] == "no":
        return "not_closed", "incomplete_exam"
    if k["prep"] == "inadequate":
        return "not_closed", "inadequate_prep"
    if k["caecum"] == "not_stated" or k["prep"] == "not_stated":
        return "needs_human", "missing_documentation"
    if k["tissue_taken"] == "yes" and k["histology"] == "no":
        return "not_closed", "awaiting_histology"
    return "closed", "criteria_met"


def truth_of(P):
    if P["cae_real"] == "no" or P["prep_real"] == "inadequate":
        return "inadequate"
    if P["tissue_real"] and not P["hist"]:
        return "awaiting_histology"
    return "adequate"


# ---------------------------------------------------------------- self-audit
AR = r"[؀-ۿ]"
CAE_EVIDENCE = re.compile(r"caec|ceacum|cecum|cecal|ileoc|appendiceal|\bAO\b|\bICV\b|"
                          r"terminal ileum|\bTI\b|ileal intubation|الأعور", re.I)
CAE_NO = re.compile(r"not reached|could not be advanced|abandoned|[Ii]ncomplete|"
                    r"NOT reached|NOT seen|Actual extent|\[X\] No|لم يتم الوصول")
PREP_GRADE = re.compile(r"\b(excellent|good|fair|poor|adequate|adequately|inadequate|"
                        r"Boston|BBPS: R \d|Aronchick|Ottawa)\b|"
                        r"ممتازة|جيدة|كافية|مقبولة|ضعيفة", re.I)
HIST_RESULT = re.compile(r"dysplasia|\bLGD\b|within normal limits|nonspecific chronic|"
                         r"active inflammation|rectal ulcer syndrome|diverticular-associated")
ADDENDUM = re.compile(r"ADDENDUM|[Aa]ddendum|PATHOLOGY REPORT|Histology result \(")
TISSUE_WORDS = re.compile(r"pot \d|retrieved|retr \(|biopsied|bx \(pot|"
                          r"Specimens taken:  \[X\] Yes", re.I)


def audit(rep, P):
    errs = []
    t = rep["text"]
    k = rep["key_fields"]
    if key_state(k) != (rep["key_state"], rep["key_reason"]):
        errs.append("key_state mismatch")
    if truth_of(P) != rep["truth"]:
        errs.append("truth mismatch")
    if k["polyps"] != k["tissue_taken"]:
        errs.append("polyps != tissue")
    if k["histology"] == "yes" and k["tissue_taken"] != "yes":
        errs.append("histology without tissue")
    # documented-vs-reality consistency
    if k["caecum"] != "not_stated" and k["caecum"] != P["cae_real"]:
        errs.append("caecum reality differs from text")
    if k["prep"] != "not_stated" and k["prep"] != P["prep_real"]:
        errs.append("prep reality differs from text")
    tl = t.lower()
    t_nobox = re.sub(r"[^\n]*\[ \] Yes  \[ \] No[^\n]*", "", t)
    if k["caecum"] == "not_stated" and CAE_EVIDENCE.search(t_nobox):
        errs.append("caecum mentioned in not_stated report: "
                    + CAE_EVIDENCE.search(t_nobox).group(0))
    if k["caecum"] == "yes" and not CAE_EVIDENCE.search(t):
        errs.append("no caecum evidence")
    if k["caecum"] == "no" and not CAE_NO.search(t):
        errs.append("no incomplete evidence")
    if k["prep"] == "not_stated":
        stripped = re.sub(r"good compliance|[^\n]*(?:2/5|adequate, comfortable)[^\n]*|\[ \] \w+", "", t)
        m = PREP_GRADE.search(stripped)
        if m:
            errs.append("prep graded word in not_stated report: " + m.group(0))
    if k["histology"] == "no":
        if HIST_RESULT.search(t) or ADDENDUM.search(t):
            errs.append("histology result text in no-histology report")
    else:
        if not ADDENDUM.search(t) or not HIST_RESULT.search(t):
            errs.append("histology yes without addendum")
    if k["tissue_taken"] == "no" and TISSUE_WORDS.search(t):
        errs.append("tissue words in no-tissue report: " + TISSUE_WORDS.search(t).group(0))
    if k["tissue_taken"] == "yes" and not re.search(r"[Pp]ot \d|pot \d", t):
        errs.append("tissue yes but no pot")
    tl = re.sub(r"\[ \] \w+", "", tl)
    if k["prep"] == "adequate" and re.search(r"\bpoor\b|inadequate", tl) and \
            "prior_poor_prep_mentioned" not in rep["tags"]:
        errs.append("adequate prep but poor word")
    # BBPS rule check
    if "bbps" in rep["tags"]:
        seg = P["bbps"]
        exp = "adequate" if sum(seg) >= 6 and min(seg) >= 2 else "inadequate"
        if exp != k["prep"]:
            errs.append("BBPS rule mismatch")
    return errs


# ---------------------------------------------------------------- main
def main():
    targets = (["adequate"] * 120 + ["awaiting_histology"] * 90 + ["inadequate"] * 90)
    rng.shuffle(targets)
    reports, plans, seen = [], [], set()
    i = 0
    for tgt in targets:
        while True:
            rep, P = build(i, tgt)
            if rep["text"] not in seen:
                break
        seen.add(rep["text"])
        rep["sha"] = hashlib.sha256(rep["text"].encode("utf-8")).hexdigest()
        rep = {"uid": rep["uid"], "sha": rep["sha"], "text": rep["text"],
               "truth": rep["truth"], "key_fields": rep["key_fields"],
               "key_state": rep["key_state"], "key_reason": rep["key_reason"],
               "tags": rep["tags"]}
        reports.append(rep)
        plans.append(P)
        i += 1

    all_errs = []
    for rep, P in zip(reports, plans):
        for e in audit(rep, P):
            all_errs.append(f"{rep['uid']}: {e}")
    if all_errs:
        print("\n".join(all_errs))
        sys.exit("AUDIT FAILED")

    out = {"source": f"Independent held-out corpus, second author; gen_heldout.py "
                     f"(stdlib only, deterministic, random seed {SEED}); {N_REPORTS} reports",
           "reports": reports}
    with open(os.path.join(OUT_DIR, "reports_heldout.json"), "w", encoding="utf-8") as f:
        json.dump(out, f, ensure_ascii=False, indent=1)

    stats = summarise(reports, plans)
    write_readme(stats)
    print(json.dumps(stats, ensure_ascii=False, indent=1))
    if "--spot" in sys.argv:
        srng = random.Random(SEED + 1)
        for rep in srng.sample(reports, 20):
            print("=" * 70)
            print(rep["uid"], rep["truth"], rep["key_fields"], rep["key_state"])
            print(rep["text"])


HARD = {"bbps_fail_segment", "prep_compliance_only", "prep_unchecked_boxes",
        "prep_blank_field", "adequate_word_elsewhere", "prep_fair", "prep_mixed_segments",
        "caecum_implied_by_polyp", "planned_vs_actual", "near_complete",
        "caecum_mentioned_not_reached", "polyp_left_in_situ", "negation_but_tissue",
        "optical_dx_not_histology", "prior_histology_mentioned", "prior_incomplete_mentioned",
        "prior_poor_prep_mentioned", "pending_then_result", "histology_normal_result",
        "arabic_only_key", "extent_unchecked", "extent_blank_field"}


def summarise(reports, plans):
    tags = Counter(t for r in reports for t in r["tags"])
    hard = sum(1 for r in reports if HARD & set(r["tags"]))
    return {
        "n": len(reports),
        "unique_texts": len({r["text"] for r in reports}),
        "truth": dict(Counter(r["truth"] for r in reports)),
        "key_state": dict(Counter(r["key_state"] for r in reports)),
        "key_reason": dict(Counter(r["key_reason"] for r in reports)),
        "caecum_key": dict(Counter(r["key_fields"]["caecum"] for r in reports)),
        "prep_key": dict(Counter(r["key_fields"]["prep"] for r in reports)),
        "tissue_key": dict(Counter(r["key_fields"]["tissue_taken"] for r in reports)),
        "histology_key": dict(Counter(r["key_fields"]["histology"] for r in reports)),
        "reports_with_hard_case": hard,
        "tags": dict(sorted(tags.items(), key=lambda x: (-x[1], x[0]))),
    }


def write_readme(s):
    tg = s["tags"]

    def c(name):
        return tg.get(name, 0)
    lines = [
        "# Held-out colonoscopy report corpus (independent second author)",
        "",
        "Generated by `gen_heldout.py` (Python 3 standard library only, deterministic, "
        "random seed 2027). Re-running the script reproduces `reports_heldout.json` exactly "
        "and re-runs the self-audit (key_state recomputed from key_fields, truth recomputed "
        "from each report's hidden reality, regex checks that silent items really are silent).",
        "",
        f"## Mix ({s['n']} reports, {s['unique_texts']} unique texts)",
        "",
        f"- truth: {s['truth']}",
        f"- key_state: {s['key_state']}",
        f"- key_reason: {s['key_reason']}",
        f"- caecum key: {s['caecum_key']}; prep key: {s['prep_key']}",
        f"- tissue_taken key: {s['tissue_key']}; histology key: {s['histology_key']}",
        f"- reports with at least one careless-reader trap (tags in HARD set): "
        f"{s['reports_with_hard_case']} ({100 * s['reports_with_hard_case'] / s['n']:.0f}%)",
        "",
        "## Variation",
        "",
        f"- Layouts: templated sections ({c('layout_template')}), free-text dictation "
        f"({c('layout_dictation')}), terse shorthand ({c('layout_terse')}), numbered verbose "
        f"({c('layout_numbered')}), checkbox form ({c('layout_form')}), bilingual "
        f"English/Arabic headings ({c('layout_bilingual')}); section order varied in "
        f"{c('section_order_varied')}; heading names vary per report.",
        f"- Spelling: UK {c('uk_spelling')}, US {c('us_spelling')}; abbreviations (TI, ICV, AO, "
        f"BBPS, CSP, TA, SSL, LGD...) in {c('abbreviations')}.",
        f"- Arabic text in {c('arabic')} (headings {c('arabic_headings')}, patient summary "
        f"footer {c('arabic_footer')}, key statement only in Arabic {c('arabic_only_key')}).",
        f"- Scanning noise (OCR letter swaps, fax/scan banners, page breaks) in "
        f"{c('scan_noise')}; applied only to non-key lines. Human typos in {c('typo')} "
        f"(misspelled key term, e.g. 'ceacum', in {c('typo_key_term')}).",
        "",
        "## Documentation cases",
        "",
        f"- Prep ungraded: {c('prep_ungraded')} (product named only {c('prep_product_only')}, "
        f"patient 'good compliance' only {c('prep_compliance_only')}, omitted "
        f"{c('prep_omitted')}, blank template field {c('prep_blank_field')}, all boxes "
        f"unchecked {c('prep_unchecked_boxes')}, 'Sedation adequate' elsewhere "
        f"{c('adequate_word_elsewhere')}).",
        f"- Boston scores: {c('bbps')} (pass {c('bbps_pass')}, total < 6 {c('bbps_fail_total')}, "
        f"total >= 6 but a segment < 2 {c('bbps_fail_segment')}; total written first "
        f"{c('bbps_total_first')}, no total written {c('bbps_no_total')}).",
        f"- Other prep wording: 'fair' (keyed adequate) {c('prep_fair')}, segment-mixed "
        f"good-left/poor-right (keyed inadequate) {c('prep_mixed_segments')}, Aronchick/Ottawa "
        f"with verbal grade {c('other_scale')}, checkbox grade {c('prep_checkbox')}.",
        f"- Extent unstated: {c('extent_unstated')} (blank field {c('extent_blank_field')}, "
        f"unchecked boxes {c('extent_unchecked')}).",
        f"- Caecum documented indirectly: landmarks only {c('caecum_by_landmarks')}, TI "
        f"intubation {c('caecum_via_TI')}, implied only by a caecal polypectomy "
        f"{c('caecum_implied_by_polyp')}, checkbox {c('caecum_checkbox')}, difficult insertion "
        f"{c('difficult_but_complete')}, hedged landmark {c('hedged_landmark')}.",
        f"- Incomplete-exam traps: 'Intended extent: caecum' vs actual {c('planned_vs_actual')}, "
        f"reached ascending colon but not caecal pole {c('near_complete')}, caecum named in "
        f"the CTC recommendation {c('caecum_mentioned_not_reached')}; previous incomplete "
        f"colonoscopy in the history of a complete one {c('prior_incomplete_mentioned')}; "
        f"previous poor prep in the history of a well-prepped one {c('prior_poor_prep_mentioned')}.",
        f"- Tissue: non-polypectomy biopsies {c('biopsy_nonpolyp')} (random/microscopic colitis "
        f"{c('random_biopsies')}, TI {c('ti_biopsy')}, plus erythema, stricture, rectal ulcer); "
        f"explicit negated tissue statements {c('negated_tissue')}; 'no biopsies of background "
        f"mucosa' alongside a polypectomy {c('negation_but_tissue')}; diminutive polyps seen and "
        f"left in situ {c('polyp_left_in_situ')}; lipoma 'not sampled' {c('lipoma_not_sampled')}.",
        f"- Histology: result addendum {c('histology_addendum')} (normal-mucosa result "
        f"{c('histology_normal_result')}, 'sent to histopathology' line still present above the "
        f"addendum {c('pending_then_result')}); pending wording {c('histology_pending_stated')}; "
        f"optical diagnosis (NICE/serrated features) that is not histology "
        f"{c('optical_dx_not_histology')}; prior histology in the indication "
        f"{c('prior_histology_mentioned')}.",
        f"- Hidden reality: key needs_human but truth inadequate {c('hidden_reality_bad')}, "
        f"needs_human with truth adequate/awaiting {c('hidden_reality_ok')}.",
        "",
        "## Keying decisions",
        "",
        "- 'Fair' prep is adequate (per brief). Good-left/poor-right prep is keyed inadequate "
        "(a poorly prepared segment, analogous to a Boston segment < 2).",
        "- Aronchick/Ottawa are only used with a verbal grade; keyed by that grade.",
        "- Prior histology quoted in the indication (previous tubular adenomas) is not a result "
        "for this procedure: histology = no.",
        "- An 'Intended extent: caecum' field is a plan, not documentation: caecum = no when the "
        "actual extent says otherwise. A CTC recommendation naming the caecum does not document it.",
        "- Boston scores and right-colon prep comments only appear when the caecum is documented "
        "as reached (a right-segment score would otherwise imply right-colon inspection).",
        "- Withdrawal time alone is never used as evidence of caecal intubation; silent-extent "
        "reports never mention the ascending colon, caecum, landmarks or TI.",
        "- Polyps left in situ (diminutive, hyperplastic-appearing, rectosigmoid) are tissue = no.",
        "- Arabic-only key statements (caecum reached/not reached, prep grade) are keyed as a "
        "bilingual clinician would read them.",
        "",
        "## Tag counts",
        "",
    ] + [f"- {k}: {v}" for k, v in s["tags"].items()]
    with open(os.path.join(OUT_DIR, "README.md"), "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")


if __name__ == "__main__":
    main()
