"""Breast held-out author loop: an external model writes the generator from the brief alone; we only run it and return
errors/validation output to the same model. We never edit its code. Usage: python3 author.py <model> <seed> [out_name]
Needs WASL_LLM_URL and WASL_LLM_KEY in the environment (an OpenAI-compatible gateway)."""
import json, os, re, subprocess, sys, time, urllib.request, collections
MODEL, SEED = sys.argv[1], sys.argv[2]
OUTNAME = sys.argv[3] if len(sys.argv) > 3 else "heldout"  # data/breast/<OUTNAME>
URL, KEY = os.environ["WASL_LLM_URL"].rstrip("/"), os.environ["WASL_LLM_KEY"]
D = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(D, "..", "..", "data", "breast", OUTNAME)
AUTHOR = {"gemini-4-argon": "Gemini 4 Argon, Google", "kimi-k3.1-pro": "Kimi K3.1 Pro, Moonshot AI", "gpt-6.1-sol": "GPT-6.1 Sol, OpenAI"}.get(MODEL, MODEL)
BRIEF = open(os.path.join(D, "brief.md")).read().replace("{SEED}", SEED).replace("{AUTHOR}", AUTHOR)
log = open(os.path.join(D, "author.log"), "a")
RANK = {"1": 0, "2": 1, "3": 2, "0": 3, "4": 4, "5": 5}
CPT = {"B0": "3340F", "B1": "3341F", "B2": "3342F", "B3": "3343F", "B4": "3344F", "B5": "3345F"}


def ask(msgs):
    body = {"model": MODEL, "messages": msgs, "max_tokens": 64000}
    for a in range(4):
        try:
            rq = urllib.request.Request(URL + "/v1/chat/completions", json.dumps(body).encode(),
                                        {"Content-Type": "application/json", "Authorization": "Bearer " + KEY})
            return json.load(urllib.request.urlopen(rq, timeout=1800))["choices"][0]["message"]["content"]
        except Exception as e:  # noqa: BLE001
            print("retry", a, type(e).__name__, file=log, flush=True); time.sleep(10 * (a + 1))
    sys.exit("model unreachable")


def blocks(txt):
    py = re.findall(r"```python\n(.*?)```", txt, re.S); md = re.findall(r"```markdown\n(.*?)```", txt, re.S)
    return (max(py, key=len) if py else None), (max(md, key=len) if md else None)


def screen_label(k):
    cats = [k["b_right"], k["b_left"]]
    if "TR" in cats:
        ok = [c for c in cats if c in ("0", "3", "4", "5")]
        if not ok:
            return "TR"
        cats = ok
    return "B" + max(cats, key=lambda c: RANK[c])


def assess_label(k):
    if not k["attended"]:
        return "dna"
    if k["assessment"] == "not_done" or k["biopsy"] == "pending" or k["outcome"] == "pending":
        return "incomplete"
    if k["outcome"] == "treatment" and k["biopsy"] == "malignant":
        return "to_treatment"
    if k["outcome"] == "routine" and k["biopsy"] != "malignant":
        return "return_routine"
    if k["outcome"] == "early_recall" and k["biopsy"] != "malignant":
        return "early_recall"
    return "invalid combination"


def validate():
    out = subprocess.run([sys.executable, "-I", "gen_breast_heldout.py"], cwd=D, capture_output=True, text=True, timeout=600)
    if out.returncode: return "generator failed:\n" + (out.stdout + out.stderr)[-4000:]
    files = {"screen_reports.json": (40, screen_label), "assess_notes.json": (50, assess_label)}
    first = {f: open(os.path.join(D, f), "rb").read() for f in files}
    subprocess.run([sys.executable, "-I", "gen_breast_heldout.py"], cwd=D, capture_output=True, timeout=600)
    errs = [f + " is not byte-identical across two runs" for f in files if open(os.path.join(D, f), "rb").read() != first[f]]
    for f, (n, lab) in files.items():
        R = json.load(open(os.path.join(D, f))).get("reports", [])
        if len(R) != n: errs.append(f"{f}: {len(R)} reports, need {n}")
        if len({r.get("text") for r in R}) != len(R): errs.append(f + ": texts not unique")
        for r in R:
            try:
                k = r["key"]
                if lab(k) != k["label"]: errs.append(f"{r['id']}: label {k['label']} does not follow the rules (expected {lab(k)})")
                if f.startswith("screen"):
                    if any(c not in RANK and c != "TR" for c in (k["b_right"], k["b_left"])): errs.append(r["id"] + ": categories must be 0-5 or TR")
                    if k["overall"] is not None and "B" + k["overall"] != k["label"]: errs.append(r["id"] + ": overall must be null or the label's category")
                    if k["coded"] not in (None, CPT.get(k["label"])): errs.append(r["id"] + ": coded must be null or the CPT II code of the label")
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
    if py: open(os.path.join(D, "gen_breast_heldout.py"), "w").write(py)
    if md: open(os.path.join(D, "README.md"), "w").write(md)
    problem = "Your reply had no ```python block with the full generator." if not py else validate()
    print(f"round {rnd}: {'OK' if not problem else problem[:300]}", file=log, flush=True)
    if not problem and os.path.exists(os.path.join(D, "README.md")):
        os.makedirs(OUT, exist_ok=True)
        for f in ("screen_reports.json", "assess_notes.json"):
            os.replace(os.path.join(D, f), os.path.join(OUT, f))
        print("DONE", rnd); break
    if not problem: problem = "Validation passed, but the ```markdown README.md block is missing."
    msgs.append({"role": "user", "content": "I ran your generator exactly as written. Problems:\n" + problem +
                 "\n\nFix them. Reply with the COMPLETE corrected gen_breast_heldout.py in one ```python block and the COMPLETE README.md in one ```markdown block."})
else:
    print("NOT DONE after 6 rounds")
