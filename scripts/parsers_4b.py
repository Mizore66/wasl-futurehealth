"""Score the local Gemma 3 4B (Q4_K_M, llama-server) on the patient-message parsers' frozen test sets.

The cached 26B scores in patient-parsers/*_scores.json came from the same prompts, label definitions, batching and
scoring (copied from the research harness), so the two are directly comparable. Safety numbers: e-consult "unsafe
accept" (a non-accept read as accept would stop chasing); teach-back "missed escalation" (needs_human or misunderstood
read as understood or partial: no human follow-up).

usage: llama-server -m gemma-3-4b-it-Q4_K_M.gguf --port 8791 -c 16384 -ngl 99 -np 4
       python3 scripts/parsers_4b.py [teachback] [econsult]   -> patient-parsers/<task>_scores_4b.json
"""
import json, os, re, sys, math, collections, urllib.request, concurrent.futures as cf
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
URL = os.environ.get("WASL_READER_URL", "http://127.0.0.1:8791") + "/v1/chat/completions"
MODEL = "gemma-3-4b-it-Q4_K_M/local"


def call_llm(prompt, tag):
    body = json.dumps({"messages": [{"role": "user", "content": prompt}], "temperature": 0, "max_tokens": 1200}).encode()
    req = urllib.request.Request(URL, body, {"Content-Type": "application/json"})
    for _ in range(3):
        try:
            return json.load(urllib.request.urlopen(req, timeout=300))["choices"][0]["message"]["content"]
        except Exception as e:
            err = str(e)
    return "ERROR " + err


# ---------------- LLM prompts
EC_DEFS = """- accept: the unit takes the patient (booked or will book) and needs nothing more from the referrer
- conditional: the unit will take the patient ONLY once the referrer does or confirms something (stop an anticoagulant, send a lab value, arrange prep/anaesthetic assessment/interpreter, treat infection, consent)
- needs_info: no decision yet; the unit asks the referrer for missing information or documents before deciding
- redirect: the unit cannot take the patient and points to another unit, hospital, service or named clinician
- reject: the unit declines and hands the case back to the referrer with no alternative unit (not indicated, duplicate, outside programme, patient refused, wrong form, advice only)"""
def ec_prompt(x):
    return f"""You parse replies in a clinician-to-clinician e-consult thread (Abu Dhabi cancer-screening follow-up). The referring primary-care doctor asked a receiving unit (endoscopy or colposcopy) about a rejected, unanswered or inadequate referral. Replies may be English, Arabic (MSA or Gulf dialect), Arabizi, or mixed. Words like "accept" may appear in a refusal; judge the meaning.
Labels:
{EC_DEFS}
Reply: <<<{x['text']}>>>
Return ONLY JSON: {{"label":"accept|conditional|needs_info|redirect|reject","action_for_referrer":"short English","redirect_to":"unit name or empty","evidence":"exact quote from the reply"}}"""

TB_DEFS = """- understood: correctly restates the next step (the right test/visit) and does not contradict who books or the timeframe
- partial: knows something must happen but is vague or wrong on a key detail (wrong test, wrong/relaxed timeframe, thinks they book later or the GP does it, unsure who books); not falsely reassured
- misunderstood: falsely reassured: believes no action is needed, the result was normal, the test is optional, or a medicine/vaccine fixed it
- needs_human: any barrier, fear, distress, refusal, cost, transport, work/travel/family constraint, or a clinical question. This label takes priority over all others."""
NEXT = {"crc": "have a colonoscopy at the endoscopy unit within 31 days; the unit will call to book; take the bowel preparation",
        "cervical": "attend a colposcopy appointment at the women's health clinic; the clinic will send the appointment"}
def tb_prompt(batch):
    items = "\n".join(f'{i+1}. [expected step: {NEXT[x["prog"]]}] <<<{x["text"]}>>>' for i, x in enumerate(batch))
    return f"""Teach-back check for an Abu Dhabi screening programme. After an abnormal screening result, each patient was asked "In your own words, what is your next step?". Replies may be English, Arabic (MSA or Gulf dialect), Arabizi, or mixed. Compare each reply with that patient's expected step.
Labels:
{TB_DEFS}
Replies:
{items}
Return ONLY a JSON array with one object per reply, in order: [{{"n":1,"label":"understood|partial|misunderstood|needs_human","evidence":"exact quote"}}]"""

def pj(t, arr=False):
    m = re.search(r"\[.*\]" if arr else r"\{.*\}", t or "", re.S)
    try: return json.loads(m.group(0))
    except Exception: return [] if arr else {}

def wilson(k, n):
    if n == 0: return (float("nan"),) * 2
    z = 1.96; p = k / n
    c = (p + z*z/(2*n)); r = z*math.sqrt(p*(1-p)/n + z*z/(4*n*n)); d = 1 + z*z/n
    return round((c - r)/d, 3), round((c + r)/d, 3)
def macro_f1(gold, pred, labs):
    f = []
    for l in labs:
        tp = sum(g == l and p == l for g, p in zip(gold, pred)); fp = sum(g != l and p == l for g, p in zip(gold, pred)); fn = sum(g == l and p != l for g, p in zip(gold, pred))
        f.append(0 if tp == 0 else 2*tp/(2*tp+fp+fn))
    return round(sum(f)/len(f), 3)

def score(task, items, preds):
    labs = sorted({x["label"] for x in items})
    out = {}
    for sysn, P in preds.items():
        g = [x["label"] for x in items]; p = [P.get(x["id"], "unparsed") for x in items]
        k = sum(a == b for a, b in zip(g, p))
        r = {"n": len(g), "correct": k, "accuracy": round(k/len(g), 3), "acc_ci95": wilson(k, len(g)), "macro_f1": macro_f1(g, p, labs),
             "confusion": collections.Counter(f"{a}->{b}" for a, b in zip(g, p) if a != b).most_common(12)}
        bys = collections.defaultdict(lambda: [0, 0])
        for x, pp in zip(items, p):
            bys[x["style"]][1] += 1; bys[x["style"]][0] += x["label"] == pp
        r["by_style"] = {s: f"{v[0]}/{v[1]}" for s, v in sorted(bys.items())}
        if task == "econsult":  # safety: a non-accept read as accept would wrongly hand ownership to the unit and stop chasing
            na = [(a, b) for a, b in zip(g, p) if a != "accept"]
            bad = sum(b == "accept" for a, b in na)
            r["unsafe_accept"] = f"{bad}/{len(na)}"; r["unsafe_accept_ci95"] = wilson(bad, len(na))
        else:   # safety: needs_human/misunderstood read as understood/partial -> no human follow-up
            nh = [(a, b) for a, b in zip(g, p) if a in ("needs_human", "misunderstood")]
            bad = sum(b in ("understood", "partial") for a, b in nh)
            r["missed_escalation"] = f"{bad}/{len(nh)}"; r["missed_escalation_ci95"] = wilson(bad, len(nh))
            fr = sum(a == "misunderstood" and b == "understood" for a, b in zip(g, p))
            r["false_reassurance_accepted"] = fr
        out[sysn] = r
    return out


if __name__ == "__main__":
    for task in sys.argv[1:] or ["teachback", "econsult"]:
        items = json.load(open(os.path.join(ROOT, "patient-parsers", task + "_testset.json")))
        raw = {}
        if task == "econsult":  # one reply per call, as for the 26B
            with cf.ThreadPoolExecutor(4) as ex:
                fs = {ex.submit(call_llm, ec_prompt(x), x["id"]): x for x in items}
                for f in cf.as_completed(fs):
                    raw[fs[f]["id"]] = pj(f.result())
        else:  # 5 replies per call, as for the 26B
            B = [items[i:i + 5] for i in range(0, len(items), 5)]
            with cf.ThreadPoolExecutor(4) as ex:
                fs = {ex.submit(call_llm, tb_prompt(b), b[0]["id"]): b for b in B}
                for f in cf.as_completed(fs):
                    arr = pj(f.result(), True)
                    for i, x in enumerate(fs[f]):
                        raw[x["id"]] = next((o for o in arr if isinstance(o, dict) and o.get("n") == i + 1), {})
        preds = {"gemma3_4b_local": {k: (v.get("label") or "unparsed").strip().lower() for k, v in raw.items()}}
        res = score(task, items, preds)
        res["model"] = MODEL
        json.dump(res, open(os.path.join(ROOT, "patient-parsers", task + "_scores_4b.json"), "w"), ensure_ascii=False, indent=1)
        r = res["gemma3_4b_local"]
        print(task, r["correct"], "/", r["n"], r["acc_ci95"], "F1", r["macro_f1"], r.get("unsafe_accept", r.get("missed_escalation")), r["by_style"])
        print("   ", r["confusion"][:8])
