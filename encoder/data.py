"""Dataset construction for the name encoder: real variants per person (persons3 + mined Wikidata labels/aliases),
NETransliteration pairs with leakage removal, held-out real-variant test sets, and the synthetic test protocol.
"""
import json
import os
import random
import subprocess
from collections import defaultdict

import wenc

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
PERSONS = os.path.join(REPO, "data", "persons3.json")

# Leading honorifics stripped from mined variants (registrations do not carry them).
AR_TITLES = {"الدكتور", "دكتور", "د", "الشيخ", "شيخ", "السيد", "الامير", "الأمير", "امير", "الملك", "الامام", "الإمام",
             "الحاج", "الاستاذ", "الأستاذ", "المهندس", "السلطان", "الرئيس", "اللواء", "العميد", "الفريق", "المشير"}
LAT_TITLES = {"dr", "dr.", "sheikh", "shaikh", "shaykh", "sheik", "prince", "king", "imam", "sir", "sayyid", "sayed",
              "haji", "hajji", "hadji", "sultan", "emir", "amir", "president", "general", "gen.", "prof.", "prof", "mr", "mr.", "mrs", "mrs."}


def strip_titles(s, ar):
    toks = s.split()
    titles = AR_TITLES if ar else LAT_TITLES
    while len(toks) > 1 and (toks[0] if ar else toks[0].lower()) in titles:
        toks = toks[1:]
    return " ".join(toks)


def good(normed):
    t = normed.split()
    return len(normed.replace(" ", "")) >= 2 and 1 <= len(t) <= 7


def load_people(variants_path=None):
    """qid -> dict(split, origin, can_ar, can_en, ar=[raw...], lat=[raw...]) with variants deduped on normalised form.
    Canonical names come first in each list."""
    ps = json.load(open(PERSONS))
    mined = {}
    if variants_path and os.path.exists(variants_path):
        for line in open(variants_path):
            j = json.loads(line)
            mined[j["qid"]] = j
    out = {}
    for p in ps:
        m = mined.get(p["qid"], {})
        ar, seen = [], set()
        for v in [p["ar"]] + [strip_titles(x, True) for x in m.get("ar", [])]:
            n = wenc.norm_ar(v)
            if v and good(n) and n not in seen:
                seen.add(n)
                ar.append(v)
        lat, seen = [], set()
        for v in [p["en"]] + list(p["als"] or []) + [strip_titles(x, False) for x in m.get("lat", [])]:
            n = wenc.norm_lat(v)
            if v and not wenc.is_ar(v) and good(n) and n not in seen:
                seen.add(n)
                lat.append(v)
        if not ar or not lat:
            continue
        out[p["qid"]] = {"qid": p["qid"], "split": p["split"], "origin": p["origin"], "ar_src": p.get("ar_src"),
                         "can_ar": p["ar"], "can_en": p["en"], "ar": ar, "lat": lat, "mined": p["qid"] in mined}
    return out


def heldout_strings(people, splits=("dev", "cal", "eval")):
    """Normalised name strings (canonical + every variant) of held-out persons, both scripts."""
    lat, ar = set(), set()
    for p in people.values():
        if p["split"] in splits:
            lat.update(wenc.norm_lat(x) for x in p["lat"])
            ar.update(wenc.norm_ar(x) for x in p["ar"])
    return lat, ar


def load_netrans(path, people, log=None):
    """NETransliteration wd_arabic (English<TAB>Arabic). Drops any pair whose English or Arabic side (normalised) equals
    a name string of a dev/cal/eval person (canonical or any mined variant), to prevent leakage."""
    hl, ha = heldout_strings(people)
    pairs, stats = [], defaultdict(int)
    seen = set()
    for line in open(path, encoding="utf-8"):
        parts = line.rstrip("\n").split("\t")
        if len(parts) != 2:
            stats["malformed"] += 1
            continue
        en, ar = parts
        ne, na = wenc.norm_lat(en), wenc.norm_ar(ar)
        if not wenc.is_ar(ar) or wenc.is_ar(en) or not good(ne) or not good(na):
            stats["unusable"] += 1
            continue
        if ne in hl or na in ha:
            stats["dropped_leakage"] += 1
            continue
        if (ne, na) in seen:
            stats["duplicate"] += 1
            continue
        seen.add((ne, na))
        pairs.append((en, ar))
    stats["kept"] = len(pairs)
    if log is not None:
        log.update(stats)
    return pairs


def real_test(people, split, cap=3, seed=0):
    """Held-out REAL-variant test. Queries are real variants that differ from the canonical name in their script;
    the gallery is every split person's canonical name in the other script.
      lat2ar: real Latin variant (!= canonical en) -> canonical Arabic gallery
      ar2lat: real Arabic label (!= canonical ar)  -> canonical Latin gallery
    Returns dict(direction -> list of (query_raw, qid)), gallery dicts {qid: raw canonical}."""
    r = random.Random(seed)
    q = {"lat2ar": [], "ar2lat": []}
    g_ar, g_lat = {}, {}
    for p in sorted(people.values(), key=lambda x: x["qid"]):
        if p["split"] != split:
            continue
        g_ar[p["qid"]], g_lat[p["qid"]] = p["can_ar"], p["can_en"]
        ce, ca = wenc.norm_lat(p["can_en"]), wenc.norm_ar(p["can_ar"])
        lv = [v for v in p["lat"] if wenc.norm_lat(v) != ce]
        av = [v for v in p["ar"] if wenc.norm_ar(v) != ca]
        for lst, key in ((lv, "lat2ar"), (av, "ar2lat")):
            if len(lst) > cap:
                lst = r.sample(lst, cap)
            q[key] += [(v, p["qid"]) for v in lst]
    return q, g_ar, g_lat


def synthetic_test(wsi, split, noise, work, seed=7):
    """Synthetic protocol: one noisy Arabic + one noisy Latin registration per person (Go gen.go noise), cached."""
    out = os.path.join(work, f"syn_{split}_{noise}_s{seed}.json")
    if not os.path.exists(out):
        subprocess.run([wsi, "encnoise", "-wasl", os.path.join(REPO, "data"), "-split", split, "-seed", str(seed),
                        "-noise", noise, "-out", out], check=True)
    return json.load(open(out))
