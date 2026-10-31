"""DR held-out author loop: an external model writes the generator from the brief alone; we only run it and return
errors/validation output to the same model. We never edit its code. Usage: python3 author.py <model> <seed>
Needs WASL_LLM_URL and WASL_LLM_KEY in the environment (an OpenAI-compatible gateway)."""
import json, os, re, subprocess, sys, time, urllib.request, collections
MODEL, SEED = sys.argv[1], sys.argv[2]
URL, KEY = os.environ["WASL_LLM_URL"].rstrip("/"), os.environ["WASL_LLM_KEY"]
D = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(D, "..", "..", "data", "dr", "heldout")
AUTHOR = {"gemini-4-argon": "Gemini 4 Argon, Google", "kimi-k3.1-pro": "Kimi K3.1 Pro, Moonshot AI"}.get(MODEL, MODEL)
BRIEF = open(os.path.join(D, "brief.md")).read().replace("{SEED}", SEED).replace("{AUTHOR}", AUTHOR)
log = open(os.path.join(D, "author.log"), "a")
RANK = {"0": 0, "1": 1, "2": 2, "3S": 3, "3A": 4}


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


def screen_label(k):
    ref = lambda r, m: r in ("2", "3A", "3S") or m == "1"
    eyes = [(k["r_right"], k["m_right"]), (k["r_left"], k["m_left"])]
    if any(r == "U" for r, _ in eyes):
        eyes = [(r, m) for r, m in eyes if r != "U" and ref(r, m)]
        if not eyes:
            return "U"
    w = max((r for r, _ in eyes), key=lambda r: RANK[r])
    m = "1" if any(m == "1" for _, m in eyes) else "0"
    return "R" + w + "M" + m + ("/urgent" if k["urgent"] and w != "3A" else "")


def clinic_label(k):
    if not k["attended"]:
        return "dna"
    if k["fundus"] == "no_view":
        return "not_assessable"
    if k["fundus"] == "not_examined":
        return "not_examined"
    return "examined_" + k["outcome"]


def validate():
    out = subprocess.run([sys.executable, "-I", "gen_dr_heldout.py"], cwd=D, capture_output=True, text=True, timeout=600)
    if out.returncode: return "generator failed:\n" + (out.stdout + out.stderr)[-4000:]
    files = {"screen_reports.json": (40, screen_label), "clinic_notes.json": (50, clinic_label)}
    first = {f: open(os.path.join(D, f), "rb").read() for f in files}
    subprocess.run([sys.executable, "-I", "gen_dr_heldout.py"], cwd=D, capture_output=True, timeout=600)
    errs = [f + " is not byte-identical across two runs" for f in files if open(os.path.join(D, f), "rb").read() != first[f]]
    for f, (n, lab) in files.items():
        R = json.load(open(os.path.join(D, f))).get("reports", [])
        if len(R) != n: errs.append(f"{f}: {len(R)} reports, need {n}")
        if len({r.get("text") for r in R}) != len(R): errs.append(f + ": texts not unique")
        for r in R:
            try:
                if lab(r["key"]) != r["key"]["label"]: errs.append(f"{r['id']}: label {r['key']['label']} does not follow the rules (expected {lab(r['key'])})")
                if f.startswith("screen") and r["key"]["coded"] not in (None, r["key"]["label"]): errs.append(r["id"] + ": coded must be null or the label")
            except Exception as e:  # noqa: BLE001
                errs.append(f"{r.get('id')}: malformed ({e})")
        hard = sum(any(t.startswith("hard:") for t in r.get("tags", [])) for r in R)
        if hard < 0.3 * n: errs.append(f"{f}: only {hard} reports have a hard:* tag, need >= {int(0.3 * n + .99)}")
        c = collections.Counter(r.get("key", {}).get("label") for r in R)
        print(f, dict(c), file=log, flush=True)
    return "\n".join(errs[:40])


msgs = [{"role": "user", "content": BRIEF}]
for rnd in range(6):
    reply = ask(msgs); msgs.append({"role": "assistant", "content": reply})
    open(os.path.join(D, f"reply{rnd}.txt"), "w").write(reply)
    py, md = blocks(reply)
    if py: open(os.path.join(D, "gen_dr_heldout.py"), "w").write(py)
    if md: open(os.path.join(D, "README.md"), "w").write(md)
    problem = "Your reply had no ```python block with the full generator." if not py else validate()
    print(f"round {rnd}: {'OK' if not problem else problem[:300]}", file=log, flush=True)
    if not problem and os.path.exists(os.path.join(D, "README.md")):
        os.makedirs(OUT, exist_ok=True)
        for f in ("screen_reports.json", "clinic_notes.json"):
            os.replace(os.path.join(D, f), os.path.join(OUT, f))
        print("DONE", rnd); break
    if not problem: problem = "Validation passed, but the ```markdown README.md block is missing."
    msgs.append({"role": "user", "content": "I ran your generator exactly as written. Problems:\n" + problem +
                 "\n\nFix them. Reply with the COMPLETE corrected gen_dr_heldout.py in one ```python block and the COMPLETE README.md in one ```markdown block."})
else:
    print("NOT DONE after 6 rounds")
