# Orchestrated Gemma 3 4B evidence reader (designed before seeing per-report outputs of this script).
# 1) deterministic snippet retrieval per field  2) one enum-constrained question per field with negative examples
# 3) mandatory verbatim quote + code-side validation (landmark / diagnosis lexicon)  4) caecum self-consistency x3 (temps 0, .4, .4)
# Unvalidated -> 'not_stated' (fails safe to review). Same DoH state() rules.
import json, sys, re, time, urllib.request, os, collections
D = os.path.dirname(os.path.abspath(__file__)); port = None
R = json.load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)),'..','data','evidence','reports43.json')))['reports']
E = lambda *v: {"type": "string", "enum": list(v)}
SENT = re.compile(r'(?<=[.;:])\s+|\n+')
KEYS = {
 'caecum': r'cec|caec|ileocec|ileocaec|ileum|appendi|terminal|advanced|passed|reached|extent|splenic|hepatic|descending|transverse|sigmoid|incomplete|unable|limit',
 'prep':   r'prep|cleans|bowel prep|boston|bbps|stool|visuali[sz]ation|residue|lavage',
 'polyps': r'polyp|snare|polypectom|forceps|biops|remov|excis|ablat|cautery|retriev',
 'histology': r'patholog|histolog|adenoma|hyperplastic|carcinoma|dysplas|serrated|tubul|villous|specimen|diagnos|biops|final',
}
LEX = {'caecum': re.compile(r'c(a)?ec(um|al)|ileoc(a)?ec|ileum|appendi', re.I),
       'histology': re.compile(r'adenoma|hyperplastic|carcinoma|dysplas|serrated|tubul|villous|inflammatory polyp|lymphoid|benign colonic|no (evidence of )?malignan|negative for', re.I)}
Q = {
 'caecum': ('Did the colonoscope reach the caecum (cecum, ileocaecal valve, appendiceal orifice or terminal ileum)?\n'
            'yes = text states one of those landmarks was reached. no = text states the scope stopped before the caecum (e.g. only to sigmoid/descending/splenic flexure, incomplete exam). not_stated = landmark not mentioned.', ['yes','no','not_stated']),
 'prep':   ('How was bowel preparation graded? adequate = excellent/good/adequate/fair/satisfactory. inadequate = poor/inadequate/suboptimal limiting the exam. not_stated = no quality statement.', ['adequate','inadequate','not_stated']),
 'polyps': ('Was at least one polyp REMOVED during this procedure (snare, forceps, polypectomy, ablation)? Seeing a polyp without removing it = no.', ['yes','no']),
 'histology': ('Does the text contain a histopathology RESULT, i.e. a tissue diagnosis such as "tubular adenoma" or "hyperplastic polyp"?\n'
               'These are NOT results (answer no): "sent to pathology", "specimens sent for histology", "biopsies taken", "pathology pending", "await pathology".', ['yes','no']),
}
def snippets(text, f):
    s = [x.strip() for x in SENT.split(text) if x.strip()]
    hit = [x for x in s if re.search(KEYS[f], x, re.I)]
    return ' '.join(hit)[:2500] or '(no relevant sentences found)'
def ask(text, f, temp):
    q, opts = Q[f]
    sch = {"type":"object","properties":{"answer":E(*opts),"quote":{"type":"string","maxLength":200}},"required":["answer","quote"]}
    msg = f"Relevant sentences from a colonoscopy report:\n<<<{snippets(text,f)}>>>\n\n{q}\nReturn JSON with 'answer' and 'quote' (an exact copy of the words that justify the answer, or \"\" if not_stated/no)."
    b = {"messages":[{"role":"user","content":msg}],"temperature":temp,"seed":1 if temp==0 else 7,"max_tokens":120,
         "response_format":{"type":"json_schema","json_schema":{"name":"a","schema":sch}}}
    rq = urllib.request.Request(f'http://127.0.0.1:{port}/v1/chat/completions', json.dumps(b).encode(), {'Content-Type':'application/json'})
    r = json.load(urllib.request.urlopen(rq, timeout=600)); j = json.loads(r['choices'][0]['message']['content'])
    return j.get('answer'), j.get('quote','')
norm = lambda s: re.sub(r'\s+',' ',(s or '')).lower().strip()
def validate(text, f, a, quote):
    if f in ('caecum','histology') and a == 'yes':
        if not quote or norm(quote) not in norm(text) or not LEX[f].search(quote): return 'not_stated' if f=='caecum' else 'no'
    if f == 'caecum' and a == 'no' and (not quote or norm(quote) not in norm(text)): return 'not_stated'
    return a
def state(c, p, po, h):
    if c == 'no': return 'not_closed','incomplete_exam'
    if p == 'inadequate': return 'not_closed','inadequate_prep'
    if c == 'not_stated' or p == 'not_stated': return 'needs_human','missing_documentation'
    if po == 'yes' and h == 'no': return 'not_closed','awaiting_histology'
    return 'closed','criteria_met'
