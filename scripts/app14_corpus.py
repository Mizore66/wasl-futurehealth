"""Synthetic colonoscopy report corpus in the DoH App 14 field layout (replaces the 43 MTSamples texts for evaluation).

Each report is rendered from a spec, and the spec is the answer key:
  key_fields  what the TEXT documents, per the reader field definitions (reader/prompt.py): caecum, prep, polyps
              (a colonic polyp removed), histology (a histopathology result in the document);
  truth       what happened, per the DoH closure rule: inadequate if the caecum was not reached or prep was poor;
              awaiting_histology if any tissue was taken (polyp OR biopsy) and no result is in the document;
              otherwise adequate. Undocumented items keep a hidden true value, as a real unit would know it.
The spec, phrasing and noise were written before any reader saw the corpus. Run:
  python3 scripts/app14_corpus.py [out.json] [n] [seed]
"""
import hashlib, json, random, sys

OUT = sys.argv[1] if len(sys.argv) > 1 else "data/evidence/reports_app14.json"
N = int(sys.argv[2]) if len(sys.argv) > 2 else 300
SEED = int(sys.argv[3]) if len(sys.argv) > 3 else 14
r = random.Random(SEED)
pick = r.choice


def chance(p):
    return r.random() < p


# ---------- spec ----------
def make_spec():
    s = {}
    s["indication"] = pick(["Positive faecal immunochemical test (FIT) on population screening", "FIT positive, screening programme referral",
                            "Positive FIT, asymptomatic", "Surveillance after previous adenoma", "FIT+ referral from primary care",
                            "Iron deficiency anaemia and positive FIT"])
    x = r.random()
    s["extent"] = "reached" if x < .80 else "incomplete" if x < .92 else "not_stated"
    s["extent_true"] = s["extent"] if s["extent"] != "not_stated" else ("reached" if chance(.7) else "incomplete")
    x = r.random()
    s["prep"] = "adequate" if x < .65 else "inadequate" if x < .77 else "not_stated"
    s["prep_true"] = s["prep"] if s["prep"] != "not_stated" else ("adequate" if chance(.85) else "inadequate")
    if s["extent"] == "incomplete" and s["prep"] == "inadequate":
        s["incomplete_reason"] = "poor_prep"
    elif s["extent"] == "incomplete":
        s["incomplete_reason"] = pick(["looping", "discomfort", "stricture", "angulation"])
    x = r.random()
    s["tissue"] = "none" if x < .45 else "polyps" if x < .88 else "mass_biopsy" if x < .93 else "random_biopsy"
    if s["extent_true"] == "incomplete" and s["tissue"] == "polyps" and chance(.5):
        s["tissue"] = "none"
    s["n_polyps"] = r.randint(1, 4) if s["tissue"] == "polyps" else 0
    s["histology_in_doc"] = s["tissue"] == "polyps" and chance(.35)
    s["layout"] = "structured" if chance(.6) else "narrative"
    s["bilingual"] = chance(.15)
    s["arabic_only_field"] = s["bilingual"] and s["prep"] != "not_stated" and chance(.35)  # prep grade stated only in Arabic
    s["ocr"] = chance(.12)
    s["spelling"] = pick(["us", "uk"])
    return s


def answer_key(s):
    k = {"caecum": {"reached": "yes", "incomplete": "no", "not_stated": "not_stated"}[s["extent"]],
         "prep": s["prep"], "polyps": "yes" if s["tissue"] == "polyps" else "no",
         "histology": "yes" if s["histology_in_doc"] else "no",
         "tissue_taken": "yes" if s["tissue"] != "none" else "no"}
    if s["extent_true"] != "reached" or s["prep_true"] != "adequate":
        truth = "inadequate"
    elif s["tissue"] != "none" and not s["histology_in_doc"]:
        truth = "awaiting_histology"
    else:
        truth = "adequate"
    c, p, po, h = k["caecum"], k["prep"], k["polyps"], k["histology"]
    if c == "no":
        st = ("not_closed", "incomplete_exam")
    elif p == "inadequate":
        st = ("not_closed", "inadequate_prep")
    elif c == "not_stated" or p == "not_stated":
        st = ("needs_human", "missing_documentation")
    elif po == "yes" and h == "no":
        st = ("not_closed", "awaiting_histology")
    else:
        st = ("closed", "criteria_met")
    return k, truth, st


# ---------- phrasing ----------
def sp(s, us, uk):
    return us if s["spelling"] == "us" else uk


AR = {"Indication": "الاستطباب", "Bowel preparation": "تحضير الأمعاء", "Extent": "مدى الفحص", "Findings": "النتائج",
      "Interventions": "الإجراءات", "Impression": "الانطباع", "Recommendations": "التوصيات", "Sedation": "التخدير"}
AR_PREP = {"adequate": ["التحضير جيد", "التحضير ممتاز", "تنظيف الأمعاء جيد"], "inadequate": ["التحضير ضعيف", "تنظيف الأمعاء غير كاف"]}


def head(s, h):
    return f"{AR[h]} / {h}:" if s["bilingual"] and h in AR else f"{h}:"


def prep_text(s):
    protocol = pick(["split-dose PEG 4 L", "sodium picosulfate / magnesium citrate", "low-volume PEG-ascorbate 2 L", "split-dose PEG 2 L + bisacodyl"])
    if s["prep"] == "not_stated":
        return pick([f"Bowel preparation with {protocol} as instructed.", f"Patient took {protocol}.", f"Prep: {protocol}.", ""])
    if s["arabic_only_field"]:
        return f"Protocol {protocol}. {pick(AR_PREP[s['prep']])}."
    if chance(.45):  # Boston Bowel Preparation Scale; adequate = total >= 6 with every segment >= 2
        if s["prep"] == "adequate":
            seg = [r.randint(2, 3) for _ in range(3)]
        else:
            seg = [r.randint(0, 1), r.randint(0, 2), r.randint(0, 2)]
            r.shuffle(seg)
        seg_txt = f"(right {seg[0]}, transverse {seg[1]}, left {seg[2]})"
        return pick([f"{protocol}. Boston Bowel Preparation Scale {sum(seg)}/9 {seg_txt}.",
                     f"Preparation {protocol}; BBPS total {sum(seg)} {seg_txt}."])
    if s["prep"] == "adequate":
        w = pick(["excellent", "good", "adequate", "fair, adequate for detection of polyps >5 mm", "good, with minimal residual fluid"])
    else:
        w = pick(["poor", "inadequate", "poor, with solid stool throughout the right colon", "inadequate; views obscured by residual stool"])
    return pick([f"{protocol}. Quality of preparation: {w}.", f"Prep ({protocol}) was {w}.", f"Bowel cleansing {w} ({protocol})."])


def extent_text(s):
    caec, ic, ti = sp(s, "cecum", "caecum"), sp(s, "ileocecal valve", "ileocaecal valve"), "terminal ileum"
    if s["extent"] == "reached":
        lm = pick([f"the appendiceal orifice and {ic}", f"the {ic} and appendiceal orifice, photo-documented", f"intubation of the {ti}",
                   f"the appendiceal orifice (photo taken)", f"the {ic}; {ti} intubated for 5 cm"])
        return pick([f"The colonoscope was advanced to the {caec}, identified by {lm}.", f"Complete colonoscopy to the {caec}. {caec.capitalize()} identified by {lm}.",
                     f"Extent: {caec} reached. Landmarks: {lm}.", f"Scope passed to the {caec}; {caec} identified by {lm}."])
    if s["extent"] == "incomplete":
        where = pick(["the splenic flexure", "the hepatic flexure", "the mid-transverse colon", "the sigmoid-descending junction", "the descending colon"])
        why = {"poor_prep": "because of poor preparation", "looping": "because of persistent looping despite abdominal pressure and position change",
               "discomfort": "because of patient discomfort", "stricture": "because of a fixed angulated stricture", "angulation": "because of a fixed sigmoid angulation"}[s["incomplete_reason"]]
        return pick([f"The scope could not be advanced beyond {where} {why}. Incomplete examination.",
                     f"Extent: {where} only; {caec} not reached {why}.", f"Examination was incomplete to {where} {why}."])
    return pick(["The colonoscope was inserted and advanced under direct vision.", "Scope introduced per rectum and advanced.", "Standard colonoscope used.", ""])


LOC = ["caecum", "ascending colon", "hepatic flexure", "transverse colon", "descending colon", "sigmoid colon", "rectum"]


def tissue_text(s):
    caec = sp(s, "cecum", "caecum")
    # a polyp "in the caecum" would itself document caecal intubation, so locations stay distal when extent is unknown/short
    near = LOC if s["extent"] == "reached" else LOC[3:] if s["extent"] == "not_stated" else LOC[4:]
    loc = lambda: pick(near).replace("caecum", caec)
    if s["tissue"] == "none":
        f = pick(["No polyps or mass lesions seen.", "Normal mucosa throughout. No polyps.", "Scattered sigmoid diverticula. No polyps.",
                  "Small internal haemorrhoids on retroflexion. Otherwise normal.", "No mucosal abnormality."])
        return f, "None.", pick(["Normal colonoscopy.", "No neoplasia seen.", "Diverticulosis; no polyps."])
    if s["tissue"] == "polyps":
        items, ivs = [], []
        for i in range(s["n_polyps"]):
            size = pick([3, 4, 5, 6, 8, 10, 12, 15])
            morph = pick(["Paris 0-Is", "Paris 0-Ip", "Paris 0-IIa", "sessile", "pedunculated"])
            meth = pick(["cold snare", "cold forceps", "hot snare", "EMR"] if size >= 10 else ["cold snare", "cold forceps"])
            where = loc()
            items.append(f"{size} mm {morph} polyp in the {where}")
            ivs.append(pick([f"{where}: polypectomy by {meth}, retrieved", f"{meth} resection of the {where} polyp, specimen retrieved",
                             f"{size} mm polyp removed with {meth} and sent to histology"]))
        f = "; ".join(items) + "."
        return f, "; ".join(ivs) + ".", pick([f"{s['n_polyps']} polyp(s) removed.", "Colonic polyps, resected.", "Polypectomy performed."])
    if s["tissue"] == "mass_biopsy":
        where = pick(["sigmoid colon", "rectum", "ascending colon", "descending colon"])
        f = f"Ulcerated, friable circumferential mass in the {where}, suspicious for malignancy."
        return f, f"Multiple biopsies taken from the mass. Tattoo placed distal to the lesion.", "Suspicious mass; biopsies sent. Urgent MDT referral."
    f = pick(["Mucosa appeared normal throughout.", "Mild patchy erythema in the sigmoid colon."])
    return f, "Random biopsies taken from the right and left colon for microscopic colitis.", "Biopsies taken; await histology."


HISTO = ["Tubular adenoma with low-grade dysplasia.", "Hyperplastic polyp; no dysplasia.", "Sessile serrated lesion without dysplasia.",
         "Tubulovillous adenoma, low-grade dysplasia, completely excised."]


def recs(s, k):
    if k["caecum"] == "no" or s["prep"] == "inadequate":
        return pick(["Repeat colonoscopy with extended preparation within 3 months.", "CT colonography to complete the examination.",
                     "Repeat procedure under propofol with a paediatric colonoscope."])
    if s["tissue"] == "mass_biopsy":
        return "Await histology; staging CT; MDT discussion."
    if s["tissue"] != "none" and not s["histology_in_doc"]:
        return pick(["Await histology; surveillance interval to follow.", "Follow-up per pathology.", "Surveillance interval based on histology."])
    return pick(["Return to the screening programme.", "Repeat colonoscopy in 10 years.", "Surveillance colonoscopy in 3 years.", "No further follow-up needed."])


def render(s, k):
    sed = pick(["Propofol (anaesthetist-led).", "Midazolam 2 mg and fentanyl 50 mcg.", "Conscious sedation.", "No sedation."])
    prep, ext = prep_text(s), extent_text(s)
    find, iv, imp = tissue_text(s)
    if s["extent"] == "incomplete":
        imp = "Incomplete colonoscopy. " + imp
    rec = recs(s, k)
    wt = f"Withdrawal time {r.randint(6, 14)} minutes." if s["extent"] == "reached" and chance(.6) else ""
    if s["layout"] == "structured":
        parts = ["COLONOSCOPY REPORT", f"{head(s, 'Indication')} {s['indication']}.", f"{head(s, 'Sedation')} {sed}",
                 f"{head(s, 'Bowel preparation')} {prep}" if prep else "", f"{head(s, 'Extent')} {ext} {wt}".strip(),
                 f"{head(s, 'Findings')} {find}", f"{head(s, 'Interventions')} {iv}", "Complications: none.",
                 f"{head(s, 'Impression')} {imp}", f"{head(s, 'Recommendations')} {rec}"]
        text = "\n".join(p for p in parts if p)
    else:
        text = " ".join(p for p in [f"Colonoscopy for {s['indication'] if s['indication'][:3].isupper() else s['indication'][0].lower() + s['indication'][1:]}.", f"Sedation: {sed}", prep, ext, wt,
                                     f"Findings: {find}", iv if s["tissue"] != "none" else "", f"Impression: {imp}", rec] if p)
    if s["histology_in_doc"]:
        dx = pick(HISTO) if s["n_polyps"] == 1 else f"{s['n_polyps']} specimens: " + "; ".join(pick(HISTO) for _ in range(s["n_polyps"]))
        text += "\n\nPATHOLOGY ADDENDUM (reported " + str(r.randint(4, 12)) + " days later): " + dx
    return text


PROTECT = ["cecum", "caecum", "ileocecal", "ileocaecal", "appendiceal", "terminal ileum", "flexure", "boston", "bbps", "prep", "preparation",
           "cleansing", "polyp", "snare", "forceps", "emr", "biops", "adenoma", "hyperplastic", "serrated", "dysplasia", "histolog", "pathology",
           "incomplete", "excellent", "good", "adequate", "fair", "poor", "inadequate", "not reached", "could not", "only"]


def ocr(text):
    """OCR-like noise on words that carry no key evidence (the answer key stays what a careful human reads)."""
    out = []
    for w in text.split(" "):
        lw = w.lower()
        if any(p in lw for p in PROTECT) or chance(.85):
            out.append(w)
            continue
        w = pick([w.replace("m", "rn"), w.replace("l", "1"), w.replace("o", "0"), w.replace("e", "c"), w[:-1] if len(w) > 3 else w, w + w[-1:]])
        out.append(w)
    t = " ".join(out)
    return t.replace("\n", pick(["\n", " ", "\n\n"]))


reports, seen = [], set()
while len(reports) < N:
    s = make_spec()
    k, truth, st = answer_key(s)
    text = render(s, k)
    if s["ocr"]:
        text = ocr(text)
    sha = hashlib.sha256(text.encode()).hexdigest()
    if sha in seen:
        continue
    seen.add(sha)
    tags = [t for t, on in [("ocr", s["ocr"]), ("bilingual", s["bilingual"]), ("prep_arabic_only", s["arabic_only_field"]),
                             ("narrative", s["layout"] == "narrative"), ("random_biopsy", s["tissue"] == "random_biopsy"),
                             ("mass_biopsy", s["tissue"] == "mass_biopsy"), ("prep_ungraded", s["prep"] == "not_stated"),
                             ("extent_not_stated", s["extent"] == "not_stated")] if on]
    reports.append({"uid": f"a{len(reports)}", "sha": sha, "text": text, "truth": truth, "key_fields": k, "key_state": st[0],
                    "key_reason": st[1], "tags": tags, "spec": s})

json.dump({"source": f"synthetic DoH App 14-layout colonoscopy reports, scripts/app14_corpus.py seed {SEED}; key = generation spec, "
                     "frozen before any reader ran", "reports": reports}, open(OUT, "w"), indent=1, ensure_ascii=False)
from collections import Counter
print(OUT, len(reports), Counter(x["truth"] for x in reports), Counter(x["key_state"] for x in reports))
print("tags", Counter(t for x in reports for t in x["tags"]))
