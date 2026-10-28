"""Second-reader cache from an OpenAI-compatible endpoint (the 26B teacher path), in the gemma_cache.json layout.

Same prompt as the local reader (reader/prompt.py ONE); one call per unique text; temperature 0. Writes <out> keyed by
SHA-256 of the text, plus <out without .json>.model with the model id (the pipeline records it as reader provenance).
Endpoint and key come only from the environment: WASL_LLM_URL (base, e.g. https://host), WASL_LLM_KEY, WASL_LLM_MODEL.
  python3 scripts/reader_cache_api.py data/evidence/reports_app14.json data/evidence/pathology_templates.json out.json
"""
import concurrent.futures as cf, hashlib, json, os, sys, time, urllib.request

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "reader"))
from prompt import ONE, pj  # noqa: E402

reports, paths, out = sys.argv[1], sys.argv[2], sys.argv[3]
URL, KEY, MODEL = os.environ["WASL_LLM_URL"].rstrip("/"), os.environ["WASL_LLM_KEY"], os.environ["WASL_LLM_MODEL"]
texts = {}
for t in [r["text"] for r in json.load(open(reports))["reports"]] + [p["text"] for p in json.load(open(paths))]:
    texts[hashlib.sha256(t.encode()).hexdigest()] = t


def ask(sha):
    body = {"model": MODEL, "messages": [{"role": "user", "content": ONE.format(t=texts[sha])}], "temperature": 0, "max_tokens": 600}
    for attempt in range(5):
        try:
            rq = urllib.request.Request(URL + "/v1/chat/completions", json.dumps(body).encode(),
                                        {"Content-Type": "application/json", "Authorization": "Bearer " + KEY})
            j = pj(json.load(urllib.request.urlopen(rq, timeout=300))["choices"][0]["message"]["content"])
            if j.get("caecum_reached") in ("yes", "no", "not_stated") and j.get("prep") in ("adequate", "inadequate", "not_stated"):
                q = {"caecum": j.get("caecum_quote", ""), "prep": j.get("prep_quote", ""), "polyps": j.get("polyps_quote", ""),
                     "histology": j.get("histology_quote", "")}
                return sha, {"caecum": j["caecum_reached"], "prep": j["prep"], "polyps": j.get("polyps_removed", "no"),
                             "histology": j.get("histology_present", "no"), "quote": q["caecum"], "quotes": q}
        except Exception as e:  # noqa: BLE001
            print(sha[:8], "retry", attempt, str(e)[:80], file=sys.stderr)
        time.sleep(2 ** attempt)
    return sha, None  # left out: a cache miss routes the report to review


cache = json.load(open(out)) if os.path.exists(out) else {}  # resume: only texts not yet answered are asked
with cf.ThreadPoolExecutor(int(os.environ.get("WASL_LLM_WORKERS", "4"))) as ex:
    for sha, entry in ex.map(ask, [s for s in texts if s not in cache]):
        if entry:
            cache[sha] = entry
json.dump(cache, open(out, "w"), indent=1)
open(out[:-5] + ".model", "w").write(MODEL + "\n")
print(f"{len(cache)}/{len(texts)} texts read by {MODEL} -> {out}")
