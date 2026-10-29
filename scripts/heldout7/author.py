"""Corpus 6 author loop: an external model writes the generator from the brief alone; we only run it and return
errors/validation output to the same model. We never edit its code. Usage: python3 author.py <model> <seed>"""
import hashlib, json, os, re, subprocess, sys, time, urllib.request, collections
MODEL, SEED = sys.argv[1], sys.argv[2]
URL, KEY = os.environ["WASL_LLM_URL"].rstrip("/"), os.environ["WASL_LLM_KEY"]
D = os.path.dirname(os.path.abspath(__file__))
AUTHOR = {"gemini-4-argon": "Gemini 4 Argon, Google", "kimi-k3.1-pro": "Kimi K3.1 Pro, Moonshot AI"}.get(MODEL, MODEL)
BRIEF = open(os.path.join(D, "brief.md")).read().replace("{SEED}", SEED).replace("{AUTHOR}", AUTHOR)
log = open(os.path.join(D, "author.log"), "a")

def ask(msgs):
    body = {"model": MODEL, "messages": msgs, "max_tokens": 64000}
    for a in range(4):
        try:
            rq = urllib.request.Request(URL + "/v1/chat/completions", json.dumps(body).encode(),
                                        {"Content-Type": "application/json", "Authorization": "Bearer " + KEY})
            return json.load(urllib.request.urlopen(rq, timeout=1800))["choices"][0]["message"]["content"]
        except Exception as e:  # noqa: BLE001
            print("retry", a, str(e)[:120], file=log, flush=True); time.sleep(10 * (a + 1))
    sys.exit("model unreachable")

def blocks(txt):
    py = re.findall(r"```python\n(.*?)```", txt, re.S); md = re.findall(r"```markdown\n(.*?)```", txt, re.S)
    return (max(py, key=len) if py else None), (max(md, key=len) if md else None)

def validate():
    out = subprocess.run([sys.executable, "-I", "gen_heldout7.py"], cwd=D, capture_output=True, text=True, timeout=600)
    if out.returncode: return "generator failed:\n" + (out.stdout + out.stderr)[-4000:]
    p = os.path.join(D, "reports_heldout7.json"); first = open(p, "rb").read()
    subprocess.run([sys.executable, "-I", "gen_heldout7.py"], cwd=D, capture_output=True, timeout=600)
    errs = [] if open(p, "rb").read() == first else ["output is not byte-identical across two runs"]
    d = json.load(open(p)); R = d.get("reports", [])
    if len(R) != 300: errs.append(f"{len(R)} reports, need 300")
    if len({r.get('text') for r in R}) != len(R): errs.append("texts not unique")
    def st(k):
        c, pr, t, h = k["caecum"], k["prep"], k["tissue_taken"], k["histology"]
        if c == "no": return "not_closed", "incomplete_exam"
        if pr == "inadequate": return "not_closed", "inadequate_prep"
        if "not_stated" in (c, pr): return "needs_human", "missing_documentation"
        if t == "yes" and h == "no": return "not_closed", "awaiting_histology"
        return "closed", "criteria_met"
    for r in R:
        try:
            k = r["key_fields"]
            if hashlib.sha256(r["text"].encode()).hexdigest() != r["sha"]: errs.append(r["uid"] + ": sha mismatch")
            if st(k) != (r["key_state"], r["key_reason"]): errs.append(r["uid"] + ": key_state does not follow rules")
            if k["polyps"] != k["tissue_taken"]: errs.append(r["uid"] + ": polyps != tissue_taken")
            if r["truth"] not in ("adequate", "inadequate", "awaiting_histology"): errs.append(r["uid"] + ": bad truth")
        except Exception as e:  # noqa: BLE001
            errs.append(f"{r.get('uid')}: malformed ({e})")
    c = collections.Counter(r.get("truth") for r in R); hard = sum(any(t.startswith("hard:") for t in r.get("tags", [])) for r in R)
    if hard < 105: errs.append(f"only {hard} reports have a hard:* tag, need >= 105")
    return "\n".join(errs[:40]) + (f"\n(truth counts {dict(c)})" if errs else "")

msgs = [{"role": "user", "content": BRIEF}]
for rnd in range(6):
    reply = ask(msgs); msgs.append({"role": "assistant", "content": reply})
    open(os.path.join(D, f"reply{rnd}.txt"), "w").write(reply)
    py, md = blocks(reply)
    if py: open(os.path.join(D, "gen_heldout7.py"), "w").write(py)
    if md: open(os.path.join(D, "README.md"), "w").write(md)
    problem = "Your reply had no ```python block with the full generator." if not py else validate()
    print(f"round {rnd}: {'OK' if not problem else problem[:300]}", file=log, flush=True)
    if not problem and os.path.exists(os.path.join(D, "README.md")):
        print("DONE", rnd); break
    if not problem: problem = "Validation passed, but the ```markdown README.md block is missing."
    msgs.append({"role": "user", "content": "I ran your generator exactly as written. Problems:\n" + problem +
                 "\n\nFix them. Reply with the COMPLETE corrected gen_heldout7.py in one ```python block and the COMPLETE README.md in one ```markdown block."})
else:
    print("NOT DONE after 6 rounds")
