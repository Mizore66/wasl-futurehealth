"""Mine real name variants for persons3 QIDs from Wikidata (CC0).

For every person in data/persons3.json, fetch labels + aliases via wbgetentities and keep the variants below.
Politeness: 50 ids per request, strictly serial, 1 s pause between requests, identifying User-Agent, back-off on
errors. Requests are read-only, so maxlag is not sent (it tracks query-service lag and can block reads for hours).
Kept:
  - Latin-script labels/aliases in en, fr, de, es, it, nl, tr, id, ms
  - Arabic-script labels/aliases in ar, arz
Output (JSONL, one line per QID, resumable): {"qid", "split", "lat": [...], "ar": [...]}
The person split is carried through so train/dev/cal/eval never mix downstream.

Usage: python encoder/mine_wikidata.py data/persons3.json OUT.jsonl
"""
import json
import os
import re
import sys
import time
import urllib.parse
import urllib.request

LAT_LANGS = ["en", "fr", "de", "es", "it", "nl", "tr", "id", "ms"]
AR_LANGS = ["ar", "arz"]
UA = "WaslResearchBot/0.1 (Arabic-Latin person-name variant mining for a research prototype; https://github.com/Mizore66)"
API = "https://www.wikidata.org/w/api.php"
AR_RE = re.compile(r"[؀-ۿ]")
LAT_RE = re.compile(r"[A-Za-z]")


def fetch(ids, tries=8):
    q = urllib.parse.urlencode({"action": "wbgetentities", "ids": "|".join(ids), "props": "labels|aliases",
                                "languages": "|".join(LAT_LANGS + AR_LANGS), "format": "json"})
    for t in range(tries):
        try:
            req = urllib.request.Request(API + "?" + q, headers={"User-Agent": UA})
            with urllib.request.urlopen(req, timeout=60) as r:
                j = json.load(r)
            if "error" in j:
                if j["error"].get("code") == "maxlag":
                    time.sleep(5 * (t + 1))
                    continue
                raise RuntimeError(j["error"])
            return j.get("entities", {})
        except Exception as e:  # network hiccup / 429: back off
            print("retry", t, e, file=sys.stderr, flush=True)
            time.sleep(10 * (t + 1))
    raise RuntimeError("giving up on batch " + ids[0])


def variants(ent):
    lat, ar = [], []
    labs, als = ent.get("labels", {}), ent.get("aliases", {})
    for lang in LAT_LANGS + AR_LANGS:
        vals = [labs[lang]["value"]] if lang in labs else []
        vals += [a["value"] for a in als.get(lang, [])]
        for v in vals:
            v = v.strip()
            if not v:
                continue
            if lang in AR_LANGS:
                if AR_RE.search(v) and not LAT_RE.search(v) and v not in ar:
                    ar.append(v)
            elif LAT_RE.search(v) and not AR_RE.search(v) and v not in lat:
                lat.append(v)
    return lat, ar


def main(persons_path, out_path, passes=5):
    """Batches that keep failing are skipped and retried in a later pass (the output is resumable)."""
    for _ in range(passes):
        if one_pass(persons_path, out_path) == 0:
            return


def one_pass(persons_path, out_path):
    persons = json.load(open(persons_path))
    split = {p["qid"]: p["split"] for p in persons}
    done = set()
    if os.path.exists(out_path):
        for line in open(out_path):
            done.add(json.loads(line)["qid"])
    order = {"eval": 0, "dev": 1, "cal": 2, "train": 3}  # held-out splits first: they gate test sets and leakage filters
    todo = sorted((q for q in split if q not in done and q.startswith("Q")), key=lambda q: order.get(split[q], 4))
    print(f"{len(done)} done, {len(todo)} to fetch", file=sys.stderr, flush=True)
    with open(out_path, "a") as f:
        for i in range(0, len(todo), 50):
            ids = todo[i:i + 50]
            try:
                ents = fetch(ids)
            except RuntimeError as e:
                print("skipped:", e, file=sys.stderr, flush=True)
                time.sleep(60)
                continue
            for q in ids:
                e = ents.get(q, {})
                lat, ar = variants(e) if "missing" not in e else ([], [])
                f.write(json.dumps({"qid": q, "split": split[q], "lat": lat, "ar": ar}, ensure_ascii=False) + "\n")
            f.flush()
            if (i // 50) % 50 == 0:
                print(f"{i + len(ids)}/{len(todo)}", file=sys.stderr, flush=True)
            time.sleep(1.0)
    return len(todo)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
