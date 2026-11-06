#!/usr/bin/env python3
"""Fresh LDCT / IFHAS evaluation and annotation harness (standard library only).

Stages (see README.md):
  validate-corpus  check a text-only 60 + 60 corpus (no keys, no tags)
  template         write an empty label-file skeleton for one labeller (blind: ids only)
  ingest           validate two independent label files, write the agreed key (disagreements stay `unadjudicated`)
                   and the agreement report (observed agreement + Cohen's kappa)
  export-inputs    write key-free corpus files in the layout `wsi7 ldctcache` / `wsi7 ifhascache` read
  manifest         derive the freeze manifest (commit ids) from git history
  check-freeze     verify the commit order and that nothing frozen has changed
  score            run the existing readers ONCE, score agreed and disputed subsets separately, write results

This module never calls a model and never edits readers, thresholds, caches, corpus or results by itself except to
create the results directory exactly once.
"""
import argparse, collections, hashlib, json, os, re, subprocess, sys, tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", ".."))

CORPUS_SCHEMA = "fresh2-corpus/v1"
LABELS_SCHEMA = "fresh2-labels/v1"
KEY_SCHEMA = "fresh2-key/v1"
RAW_SCHEMA = "fresh2-reader-raw/v1"
SCORE_SCHEMA = "fresh2-score/v1"

PER_PROGRAMME = 60
MIN_PER_KIND = 10
KINDS = {
    "ldct-screen": {"programme": "ldct", "fields": ("label",)},
    "ldct-workup": {"programme": "ldct", "fields": ("label",)},
    "ifhas-followup": {"programme": "ifhas", "fields": ("label",)},
    "ifhas-screen": {"programme": "ifhas", "fields": ("label", "coded_label")},
}
KIND_ORDER = ["ldct-screen", "ldct-workup", "ifhas-followup", "ifhas-screen"]
ITEM_FIELDS = {"id", "programme", "kind", "text", "coded"}

ALLOWED = {
    "ldct-screen": ["LR0", "LR1", "LR2", "LR3", "LR4A", "LR4B", "LR4X", "indeterminate"],
    "ldct-workup": ["wk_dna", "wk_incomplete", "wk_treatment", "wk_benign", "wk_surveillance", "indeterminate"],
}
DOMAINS = ["bp", "glyc", "lipid", "weight"]  # fixed order in every composite label
SCREEN_CELLS = {"bp": "NHS", "glyc": "NPDS", "lipid": "NH", "weight": "NO"}
STATUSES = ("managed", "not_confirmed", "pending")
# LOINC code -> (measure, unit); documented in the labelling guide, checked by a test
LOINC = {
    "4548-4": ("HbA1c", "%"), "2345-7": ("random glucose", "mg/dL"), "14749-6": ("random glucose", "mmol/L"),
    "8480-6": ("systolic BP", "mm[Hg]"), "8462-4": ("diastolic BP", "mm[Hg]"),
    "13457-7": ("LDL cholesterol", "mg/dL"), "18262-6": ("LDL cholesterol", "mg/dL"), "2089-1": ("LDL cholesterol", "mg/dL"),
    "22748-8": ("LDL cholesterol", "mmol/L"), "2571-8": ("triglycerides", "mg/dL"), "14927-8": ("triglycerides", "mmol/L"),
    "39156-5": ("BMI", "kg/m2"),
}

DEFAULT_PATHS = {
    "corpus": "data/fresh2/corpus.json",
    "labels": ["data/fresh2/labels/labeller_a.json", "data/fresh2/labels/labeller_b.json"],
    "key": "data/fresh2/key.json",
    "agreement": "data/fresh2/agreement.json",
    "reader_caches": ["data/fresh2/reader-cache/ldct_gemma_cache.json", "data/fresh2/reader-cache/ifhas_gemma_cache.json"],
    "manifest": "data/fresh2/FREEZE.json",
    "results_dir": "data/fresh2/results",
}
# files whose content must be identical at reader_code_commit and at scoring time
DEFAULT_PROTECTED = [
    "pipeline/ldctread.go", "pipeline/ldctrule.go", "pipeline/ifhasread.go", "pipeline/ifhasrule.go",
    "pipeline/evidence.go", "pipeline/lungread.go", "pipeline/reader.go", "pipeline/fresh2_reader_test.go",
    "models/ldct_gemma_cache.json", "models/ifhas_gemma_cache.json",
]
WATCH_PREFIXES = ["data/fresh2/", "scripts/fresh2_eval/"]
MANIFEST_KEYS = ["reader_code_commit", "corpus_commit", "labels_commit", "reader_cache_commit"]
GUIDE_PATH = os.path.join(HERE, "LABELLING-GUIDE.md")
SEMANTICS = ("These keys are model-labelled and not clinician-verified. Two independent labelling passes agreed on the "
             "listed items; every item they disagreed on is `unadjudicated` and carries no key. Agreement between two "
             "models, or between a model and a human reading the same guide, is not clinical truth, and results scored "
             "against this key are not evidence of clinical effectiveness.")


class HarnessError(Exception):
    pass


# ---------------------------------------------------------------- basics
def dumps(obj):
    return json.dumps(obj, indent=1, ensure_ascii=False, sort_keys=True) + "\n"


def canon_sha(obj):
    return hashlib.sha256(dumps(obj).encode("utf-8")).hexdigest()


def sha_bytes(b):
    return hashlib.sha256(b).hexdigest()


def guide_sha256():
    with open(GUIDE_PATH, "rb") as f:
        return sha_bytes(f.read())


def norm_text(s):
    return re.sub(r"\s+", " ", s).strip().lower()


def read_json(path):
    with open(path, "rb") as f:
        raw = f.read()
    return json.loads(raw.decode("utf-8")), raw


def write_new(path, text):
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)


# ---------------------------------------------------------------- label grammar
def label_error(kind, value, field="label"):
    """None if `value` is an allowed label for this kind/field, else a short reason."""
    if not isinstance(value, str):
        return "not a string"
    if kind in ALLOWED:
        return None if value in ALLOWED[kind] and field == "label" else f"{value!r} is not one of {ALLOWED[kind]}"
    if kind == "ifhas-followup":
        if value in ("dna", "indeterminate"):
            return None
        return _composite_error(value, STATUSES, partial=True)
    if kind == "ifhas-screen":
        if field == "coded_label" and value == "unavailable":
            return None
        return _composite_error(value, None, partial=False)
    return f"unknown kind {kind!r}"


def _composite_error(value, statuses, partial):
    if not value:
        return "empty label"
    parts = value.split(";")
    keys = []
    for p in parts:
        if p.count("=") != 1:
            return f"{value!r}: each part must be domain=value"
        d, v = p.split("=")
        if d not in DOMAINS:
            return f"{value!r}: unknown domain {d!r}"
        if statuses is not None and v not in statuses:
            return f"{value!r}: status {v!r} not in {list(statuses)}"
        if statuses is None and v not in SCREEN_CELLS[d] + "X":
            return f"{value!r}: cell {v!r} not allowed for {d}"
        keys.append(d)
    if len(set(keys)) != len(keys):
        return f"{value!r}: domain repeated"
    if keys != [d for d in DOMAINS if d in keys]:
        return f"{value!r}: domains must be in the order {DOMAINS}"
    if not partial and keys != DOMAINS:
        return f"{value!r}: all four domains required"
    return None


# ---------------------------------------------------------------- corpus
def validate_corpus(corpus, known_texts=None):
    errs = []
    if not isinstance(corpus, dict) or corpus.get("schema") != CORPUS_SCHEMA:
        return [f"schema must be {CORPUS_SCHEMA!r}"]
    prov = corpus.get("provenance")
    if (not isinstance(prov, dict) or prov.get("authoring") not in ("model", "human") or not prov.get("author_id")
            or not isinstance(prov.get("author_saw_reader_code"), bool) or not prov.get("created")):
        errs.append("provenance needs authoring (model|human), author_id, author_saw_reader_code (bool) and created")
    items = corpus.get("items")
    if not isinstance(items, list):
        return errs + ["items must be a list"]
    ids, texts = set(), {}
    by_kind, by_prog = collections.Counter(), collections.Counter()
    for n, it in enumerate(items):
        where = it.get("id", f"#{n}") if isinstance(it, dict) else f"#{n}"
        if not isinstance(it, dict):
            errs.append(f"item {where}: not an object")
            continue
        for extra in sorted(set(it) - ITEM_FIELDS):
            errs.append(f"item {where}: unexpected field {extra!r} (corpus must be text-only; no keys, labels or tags)")
        iid = it.get("id")
        if not isinstance(iid, str) or not re.fullmatch(r"[A-Za-z0-9._-]+", iid or ""):
            errs.append(f"item {where}: id must match [A-Za-z0-9._-]+")
        elif iid in ids:
            errs.append(f"item {where}: duplicate id")
        else:
            ids.add(iid)
        kind = it.get("kind")
        if kind not in KINDS:
            errs.append(f"item {where}: unknown kind {kind!r}")
        else:
            by_kind[kind] += 1
            if it.get("programme") != KINDS[kind]["programme"]:
                errs.append(f"item {where}: programme must be {KINDS[kind]['programme']!r} for kind {kind}")
            else:
                by_prog[it["programme"]] += 1
            if kind == "ifhas-screen":
                errs.extend(_coded_errors(where, it.get("coded")))
            elif "coded" in it:
                errs.append(f"item {where}: coded is allowed only ifhas-screen items")
        t = it.get("text")
        if not isinstance(t, str) or not t.strip():
            errs.append(f"item {where}: text must be a non-empty string")
        else:
            nt = norm_text(t)
            if nt in texts:
                errs.append(f"item {where}: duplicate text (same as {texts[nt]})")
            texts.setdefault(nt, where)
            if known_texts and nt in known_texts:
                errs.append(f"item {where}: text is already in an existing corpus")
    for prog in ("ldct", "ifhas"):
        if by_prog[prog] != PER_PROGRAMME:
            errs.append(f"programme {prog} has {by_prog[prog]} items, need exactly {PER_PROGRAMME}")
    for kind in KIND_ORDER:
        if by_kind[kind] < MIN_PER_KIND:
            errs.append(f"kind {kind} has {by_kind[kind]} items, need at least {MIN_PER_KIND}")
    return errs


def _coded_errors(where, coded):
    if not isinstance(coded, str) or not coded.strip():
        return [f"item {where}: ifhas-screen requires a coded string 'code|value|unit;...'"]
    for part in coded.split(";"):
        f = part.split("|")
        if len(f) != 3:
            return [f"item {where}: coded part {part!r} is not code|value|unit"]
        try:
            float(f[1])
        except ValueError:
            return [f"item {where}: coded value {f[1]!r} is not a number"]
    return []


def load_known_texts(repo):
    """Normalised texts of every existing corpus (dev and earlier held-out) so a fresh item cannot be a reuse."""
    out = set()
    for rel in ("data/ldct", "data/ifhas"):
        for root, _, files in os.walk(os.path.join(repo, rel)):
            for fn in files:
                if fn.endswith(".json") and "cache" not in fn:
                    try:
                        doc, _ = read_json(os.path.join(root, fn))
                    except (ValueError, OSError):
                        continue
                    for r in doc.get("reports", []) if isinstance(doc, dict) else []:
                        if isinstance(r, dict) and isinstance(r.get("text"), str):
                            out.add(norm_text(r["text"]))
    return out


# ---------------------------------------------------------------- label files
def validate_label_pair(corpus, la, lb, bytes_a, bytes_b):
    errs = []
    errs += _label_file_errors(corpus, la, "labeller file A")
    errs += _label_file_errors(corpus, lb, "labeller file B")
    ia = (la.get("labeller") or {}).get("id") if isinstance(la, dict) else None
    ib = (lb.get("labeller") or {}).get("id") if isinstance(lb, dict) else None
    if ia and ia == ib:
        errs.append("labellers must be distinct (same labeller id in both files)")
    if bytes_a == bytes_b:
        errs.append("the two label files are byte-identical; they must be independent")
    return errs


def _label_file_errors(corpus, lf, name):
    errs = []
    if not isinstance(lf, dict) or lf.get("schema") != LABELS_SCHEMA:
        return [f"{name}: schema must be {LABELS_SCHEMA!r}"]
    if lf.get("corpus_sha256") != canon_sha(corpus):
        errs.append(f"{name}: corpus_sha256 does not match the corpus")
    lab = lf.get("labeller")
    if not isinstance(lab, dict):
        return errs + [f"{name}: labeller block missing"]
    if not lab.get("id"):
        errs.append(f"{name}: labeller.id required")
    if lab.get("kind") not in ("model", "human"):
        errs.append(f"{name}: labeller.kind must be model or human")
    if lab.get("kind") == "model":
        for k in ("model", "model_family", "prompt_sha256"):
            if not lab.get(k):
                errs.append(f"{name}: model labeller requires {k}")
    elif lab.get("kind") == "human" and not lab.get("name"):
        errs.append(f"{name}: human labeller requires name")
    if not lab.get("run_date"):
        errs.append(f"{name}: labeller.run_date required")
    if lab.get("guide_sha256") != guide_sha256():
        errs.append(f"{name}: guide_sha256 does not match LABELLING-GUIDE.md")
    if lab.get("session_isolated") is not True:
        errs.append(f"{name}: session_isolated must be true")
    if lab.get("saw_reader_implementation") is not False:
        errs.append(f"{name}: saw_reader_implementation must be false")
    if lab.get("saw_other_labels") is not False:
        errs.append(f"{name}: saw_other_labels must be false")
    if lab.get("clinician_verified") is not False:
        errs.append(f"{name}: clinician_verified must be false (this workstream makes no clinician claim)")
    kinds = {it["id"]: it["kind"] for it in corpus.get("items", []) if isinstance(it, dict) and "id" in it}
    seen = set()
    for row in lf.get("labels", []) if isinstance(lf.get("labels"), list) else []:
        rid = row.get("id") if isinstance(row, dict) else None
        if rid not in kinds:
            errs.append(f"{name}: unknown id {rid!r}")
            continue
        if rid in seen:
            errs.append(f"{name}: duplicate label row for {rid}")
            continue
        seen.add(rid)
        kind = kinds[rid]
        allowed_fields = {"id"} | set(KINDS[kind]["fields"])
        for extra in sorted(set(row) - allowed_fields):
            errs.append(f"{name}: {rid}: coded_label only belongs to ifhas-screen" if extra == "coded_label"
                        else f"{name}: {rid}: unexpected field {extra!r}")
        for field in KINDS[kind]["fields"]:
            if field not in row:
                errs.append(f"{name}: {rid}: {field} required")
                continue
            e = label_error(kind, row[field], field)
            if e:
                errs.append(f"{name}: {rid}: {field} {e}")
    for iid in sorted(set(kinds) - seen):
        errs.append(f"{name}: missing label for {iid}")
    return errs


def label_warnings(la, lb):
    w = []
    a, b = la["labeller"], lb["labeller"]
    if a.get("kind") == b.get("kind") == "model" and a.get("model_family") == b.get("model_family"):
        w.append("both labellers are the same model family; agreement overstates independence")
    return w


# ---------------------------------------------------------------- agreement and key
def kappa(a, b):
    """(Cohen's kappa rounded to 3 places or None, note)."""
    n = len(a)
    if n == 0:
        return None, "no items"
    po = sum(x == y for x, y in zip(a, b)) / n
    ca, cb = collections.Counter(a), collections.Counter(b)
    pe = sum(ca[k] * cb[k] for k in set(a) | set(b)) / n / n
    if pe == 1:
        return None, "undefined: both labellers used a single identical label for every item"
    return round((po - pe) / (1 - pe), 3), ""


def build_key(corpus, la, lb, bytes_a, bytes_b):
    errs = validate_corpus(corpus) + validate_label_pair(corpus, la, lb, bytes_a, bytes_b)
    if errs:
        raise HarnessError("cannot build key:\n  " + "\n  ".join(errs))
    pairs = sorted([(la, bytes_a), (lb, bytes_b)], key=lambda p: p[0]["labeller"]["id"])
    (A, ba), (B, bb) = pairs
    ida, idb = A["labeller"]["id"], B["labeller"]["id"]
    rows = [{r["id"]: r for r in L["labels"]} for L in (A, B)]
    items, by_kind = [], {}
    n_agreed = 0
    for it in corpus["items"]:
        kind = it["kind"]
        fields = KINDS[kind]["fields"]
        ra, rb = rows[0][it["id"]], rows[1][it["id"]]
        agreed = all(ra[f] == rb[f] for f in fields)
        entry = {"id": it["id"], "kind": kind, "programme": it["programme"],
                 "status": "agreed" if agreed else "unadjudicated",
                 "key": {f: ra[f] for f in fields} if agreed else None,
                 "labels": {ida: {f: ra[f] for f in fields}, idb: {f: rb[f] for f in fields}}}
        items.append(entry)
        n_agreed += agreed
        k = by_kind.setdefault(kind, {"n": 0, "agreed": 0, "disagreements": [], "_f": {f: ([], []) for f in fields}})
        k["n"] += 1
        k["agreed"] += agreed
        if not agreed:
            k["disagreements"].append(it["id"])
        for f in fields:
            k["_f"][f][0].append(ra[f])
            k["_f"][f][1].append(rb[f])
    for kind, k in by_kind.items():
        fs = k.pop("_f")
        k["item_agreement"] = round(k["agreed"] / k["n"], 3)
        k["fields"] = {}
        for f, (x, y) in fs.items():
            kv, note = kappa(x, y)
            k["fields"][f] = {"n": len(x), "observed_agreement": round(sum(p == q for p, q in zip(x, y)) / len(x), 3),
                              "kappa": kv, "kappa_note": note}
    total = len(items)
    meta = lambda L, raw: {"labeller_id": L["labeller"]["id"], "sha256": sha_bytes(raw), "labeller": L["labeller"]}
    key = {"schema": KEY_SCHEMA, "corpus_sha256": canon_sha(corpus),
           "counts": {"items": total, "agreed": n_agreed, "unadjudicated": total - n_agreed},
           "semantics": {"model_labelled": any(L["labeller"]["kind"] == "model" for L in (A, B)),
                         "clinician_verified": False, "statement": SEMANTICS},
           "label_files": [meta(A, ba), meta(B, bb)],
           "warnings": label_warnings(A, B), "items": items}
    agreement = {"schema": "fresh2-agreement/v1", "labellers": [ida, idb],
                 "overall": {"items": total, "agreed": n_agreed, "item_agreement": round(n_agreed / total, 3)},
                 "by_kind": by_kind, "warnings": key["warnings"], "semantics": key["semantics"]}
    return key, agreement


# ---------------------------------------------------------------- freeze verification
def _git(repo, *args):
    p = subprocess.run(["git", "-C", repo, *args], capture_output=True, text=True)
    return None if p.returncode else p.stdout


def _commit_ok(repo, c):
    return isinstance(c, str) and re.fullmatch(r"[0-9a-f]{40}", c) is not None and _git(repo, "cat-file", "-e", c + "^{commit}") is not None


def _exists_at(repo, c, path):
    return _git(repo, "cat-file", "-e", f"{c}:{path}") is not None


def _changed(repo, c, paths):
    out = _git(repo, "diff", "--name-only", c, "HEAD", "--", *paths)
    return None if out is None else [x for x in out.splitlines() if x]


def check_freeze(repo, manifest, paths=None, protected=None, pre_score=True):
    """Return a list of problems; empty means the freeze is intact (and, if pre_score, nothing has been scored yet)."""
    P = paths or DEFAULT_PATHS
    prot = protected or DEFAULT_PROTECTED
    errs = []
    for k in MANIFEST_KEYS:
        if k not in manifest:
            errs.append(f"manifest missing {k}")
        elif not _commit_ok(repo, manifest[k]):
            errs.append(f"{k} {manifest[k]!r} is not a commit in this repository")
    if errs:
        return errs
    chain = [("reader_code_commit", "corpus_commit"), ("corpus_commit", "labels_commit"), ("labels_commit", "reader_cache_commit")]
    for early, late in chain:
        if manifest[early] == manifest[late]:
            errs.append(f"{early} and {late} must be strictly ordered, but are the same commit")
        elif _git(repo, "merge-base", "--is-ancestor", manifest[early], manifest[late]) is None:
            errs.append(f"{early} is not an ancestor of {late}")
    if _git(repo, "merge-base", "--is-ancestor", manifest["reader_cache_commit"], "HEAD") is None:
        errs.append("reader_cache_commit is not an ancestor of HEAD")
    if errs:
        return errs
    rc, cc, lc, kc = (manifest[k] for k in MANIFEST_KEYS)
    labelfiles = list(P["labels"]) + [P["key"]]
    for p in prot:
        if not _exists_at(repo, rc, p):
            errs.append(f"protected file {p} does not exist at reader_code_commit")
    if _exists_at(repo, rc, P["corpus"]):
        errs.append("corpus must be absent at reader_code_commit")
    for p in [P["corpus"]]:
        if not _exists_at(repo, cc, p):
            errs.append(f"{p} missing at corpus_commit")
    for p in labelfiles:
        if _exists_at(repo, cc, p):
            errs.append(f"{p} is present at corpus_commit; labels must be committed after the corpus")
        if not _exists_at(repo, lc, p):
            errs.append(f"{p} missing at labels_commit")
    for p in P["reader_caches"]:
        if _exists_at(repo, lc, p):
            errs.append(f"{p} is present at labels_commit; fresh reader caches must be committed after the labels")
        if not _exists_at(repo, kc, p):
            errs.append(f"{p} missing at reader_cache_commit")
    # nothing frozen may differ between its freeze commit and HEAD
    for title, commit, files in (("reader_code_commit", rc, prot), ("corpus_commit", cc, [P["corpus"]]),
                                 ("labels_commit", lc, labelfiles), ("reader_cache_commit", kc, list(P["reader_caches"]))):
        ch = _changed(repo, commit, files)
        for p in ch or []:
            role = ""
            if p in P["labels"]:
                role = "labeller_" + "ab"[P["labels"].index(p)] + " "
            errs.append(f"{role}{p} changed since {title}")
    # key must be exactly what the committed label files regenerate
    errs += _key_regen_errors(repo, P)
    # cleanliness: no uncommitted change or untracked file under watched paths
    st = _git(repo, "status", "--porcelain", "--untracked-files=all")
    watched = [x for x in WATCH_PREFIXES] + list(prot)
    for line in (st or "").splitlines():
        path = line[3:].strip().strip('"')
        if not pre_score and (path + "/").startswith(P["results_dir"] + "/"):
            continue
        if any(path.startswith(w) for w in watched):
            errs.append(f"uncommitted change under watched path: {path}")
    if pre_score and os.path.exists(os.path.join(repo, P["results_dir"])):
        errs.append(f"results directory {P['results_dir']} already exists; the readers are scored once")
    return errs


def _key_regen_errors(repo, P):
    try:
        corpus, _ = read_json(os.path.join(repo, P["corpus"]))
        (la, ba), (lb, bb) = (read_json(os.path.join(repo, p)) for p in P["labels"])
        have, _ = read_json(os.path.join(repo, P["key"]))
        pre = validate_corpus(corpus) + validate_label_pair(corpus, la, lb, ba, bb)
        if pre:
            return ["corpus/labels invalid: " + "; ".join(pre[:5])]
        want, _ = build_key(corpus, la, lb, ba, bb)
    except (OSError, ValueError, HarnessError) as e:
        return [f"cannot verify key: {e}"]
    if want != have:
        return ["key file differs from the key regenerated from the label files; it must be regenerated by `ingest`, never edited"]
    return []


def build_manifest(repo, reader_code_commit, paths=None):
    P = paths or DEFAULT_PATHS

    def added(path):
        out = _git(repo, "log", "--diff-filter=A", "--format=%H", "--", path)
        lines = (out or "").split()
        return lines[-1] if lines else None

    def latest(commits):
        commits = [c for c in commits if c]
        best = None
        for c in commits:
            if best is None or _git(repo, "merge-base", "--is-ancestor", best, c) is not None:
                best = c
        return best
    return {"reader_code_commit": reader_code_commit, "corpus_commit": added(P["corpus"]),
            "labels_commit": latest([added(p) for p in list(P["labels"]) + [P["key"]]]),
            "reader_cache_commit": latest([added(p) for p in P["reader_caches"]])}


# ---------------------------------------------------------------- scoring
def expected_agree(k):
    """Whether the two readers SHOULD agree on an item with this agreed key (the system's safe behaviour)."""
    label = k["label"]
    if "coded_label" in k:
        return "X" not in label and k["coded_label"] == label
    return label != "indeterminate"


def _validate_raw(key, raw):
    if not isinstance(raw, dict) or raw.get("schema") != RAW_SCHEMA or not isinstance(raw.get("items"), list):
        raise HarnessError(f"reader output must have schema {RAW_SCHEMA!r} and an items list")
    want = {i["id"]: i["kind"] for i in key["items"]}
    seen = {}
    for r in raw["items"]:
        for f in ("id", "kind", "a", "b", "b_available", "agree", "label"):
            if f not in r:
                raise HarnessError(f"reader output row {r.get('id')!r} lacks {f}")
        if r["id"] in seen:
            raise HarnessError(f"reader output has {r['id']} twice")
        if want.get(r["id"]) != r["kind"]:
            raise HarnessError(f"reader output row {r['id']} is not a corpus item of that kind")
        seen[r["id"]] = r
    missing = sorted(set(want) - set(seen))
    if missing:
        raise HarnessError(f"reader output is missing {len(missing)} items, e.g. {missing[:3]}")
    return seen


def compute_scores(key, raw):
    rows = _validate_raw(key, raw)
    ag, dis = {}, {}
    for it in key["items"]:
        r = rows[it["id"]]
        kind = it["kind"]
        if it["status"] == "agreed":
            k = it["key"]
            s = ag.setdefault(kind, {"n": 0, "reader_a_correct": 0, "reader_b_correct": 0, "reader_b_unavailable": 0, "agree": 0,
                                     "agreed_and_right": 0, "agreed_but_wrong": 0, "agreed_but_wrong_ids": [],
                                     "routed_to_review_key_decisive": 0, "routed_to_review_key_expects_review": 0})
            s["n"] += 1
            s["reader_a_correct"] += r["a"] == k["label"]
            s["reader_b_correct"] += r["b"] == k.get("coded_label", k["label"])
            s["reader_b_unavailable"] += not r["b_available"]
            if r["agree"]:
                s["agree"] += 1
                if expected_agree(k) and r["label"] == k["label"]:
                    s["agreed_and_right"] += 1
                else:
                    s["agreed_but_wrong"] += 1
                    s["agreed_but_wrong_ids"].append(it["id"])
            elif expected_agree(k):
                s["routed_to_review_key_decisive"] += 1
            else:
                s["routed_to_review_key_expects_review"] += 1
        else:
            s = dis.setdefault(kind, {"n": 0, "reader_agreed": 0, "reader_routed_to_review": 0, "reader_b_unavailable": 0,
                                      "agreed_label_matches_one_labeller": 0, "agreed_label_matches_neither_labeller": 0,
                                      "ids_reader_agreed": []})
            s["n"] += 1
            s["reader_b_unavailable"] += not r["b_available"]
            if r["agree"]:
                s["reader_agreed"] += 1
                s["ids_reader_agreed"].append(it["id"])
                if any(v["label"] == r["label"] for v in it["labels"].values()):
                    s["agreed_label_matches_one_labeller"] += 1
                else:
                    s["agreed_label_matches_neither_labeller"] += 1
            else:
                s["reader_routed_to_review"] += 1
    sums = lambda d, fields: {f: sum(v[f] for v in d.values()) for f in fields}
    return {
        "schema": SCORE_SCHEMA,
        "agreed": {"by_kind": ag, "totals": sums(ag, ["n", "reader_a_correct", "reader_b_correct", "reader_b_unavailable", "agree",
                                                      "agreed_and_right", "agreed_but_wrong", "routed_to_review_key_decisive",
                                                      "routed_to_review_key_expects_review"]),
                   "note": "Scored against the agreed key. An agreed reader answer is wrong if it differs from the key or if the "
                           "key expects review (indeterminate, a missing screening domain, or a coded/panel mismatch)."},
        "disputed": {"by_kind": dis, "totals": sums(dis, ["n", "reader_agreed", "reader_routed_to_review"]),
                     "note": "Items the two labellers disagreed on have no key and are unscored for accuracy; only reader behaviour "
                             "is counted here. Nothing in this block is evidence that the reader is right or wrong."},
        "models": raw.get("models", {}),
    }


def render_markdown(res, key):
    L = ["# Fresh LDCT / IFHAS reader scores", "", "**" + key["semantics"]["statement"] + "**", "",
         f"Items: {key['counts']['items']}; agreed {key['counts']['agreed']}; unadjudicated {key['counts']['unadjudicated']}.", "",
         "## Agreed subset (scored against the agreed key)", "",
         "| kind | n | A correct | B correct | B unavailable | readers agree | agreed but wrong | review (key decisive) | review (key expects) |",
         "|---|---|---|---|---|---|---|---|---|"]
    for kind, s in sorted(res["agreed"]["by_kind"].items()):
        L.append(f"| {kind} | {s['n']} | {s['reader_a_correct']} | {s['reader_b_correct']} | {s['reader_b_unavailable']} | {s['agree']} | "
                 f"{s['agreed_but_wrong']} | {s['routed_to_review_key_decisive']} | {s['routed_to_review_key_expects_review']} |")
        if s["agreed_but_wrong_ids"]:
            L.append(f"|  | agreed-but-wrong ids: {', '.join(s['agreed_but_wrong_ids'])} | | | | | | | |")
    t = res["agreed"]["totals"]
    L += [f"| **total** | {t.get('n', 0)} | {t.get('reader_a_correct', 0)} | {t.get('reader_b_correct', 0)} | {t.get('reader_b_unavailable', 0)} | "
          f"{t.get('agree', 0)} | {t.get('agreed_but_wrong', 0)} | {t.get('routed_to_review_key_decisive', 0)} | "
          f"{t.get('routed_to_review_key_expects_review', 0)} |", "", res["agreed"]["note"], "",
          "## Disputed subset (no key; unscored for accuracy)", "",
          "| kind | n | readers agree | routed to review | agreed label matches one labeller | matches neither |", "|---|---|---|---|---|---|"]
    for kind, s in sorted(res["disputed"]["by_kind"].items()):
        L.append(f"| {kind} | {s['n']} | {s['reader_agreed']} | {s['reader_routed_to_review']} | "
                 f"{s['agreed_label_matches_one_labeller']} | {s['agreed_label_matches_neither_labeller']} |")
    L += ["", res["disputed"]["note"], ""]
    return "\n".join(L)


# ---------------------------------------------------------------- reader inputs and the single scoring run
def export_reader_inputs(corpus, outdir):
    """Key-free files in the layout the cache builders read: <out>/ldct/*, <out>/ifhas/followup_notes.json."""
    layout = {"ldct-screen": ("ldct", "screen_reports.json"), "ldct-workup": ("ldct", "workup_notes.json"),
              "ifhas-followup": ("ifhas", "followup_notes.json")}
    for kind, (sub, fn) in layout.items():
        rep = [{"id": it["id"], "text": it["text"]} for it in corpus["items"] if it["kind"] == kind]
        write_new(os.path.join(outdir, sub, fn), dumps({"source": "fresh2 corpus, text only", "reports": rep}))


def run_reader(cmd, cwd, env):
    """Run the reader command once; its output goes to a temp file outside cwd so a failure leaves nothing behind."""
    with tempfile.TemporaryDirectory() as td:
        out = os.path.join(td, "raw.json")
        full = dict(os.environ)
        full.update(env)
        full["FRESH2_OUT"] = out
        p = subprocess.run(cmd, cwd=cwd, env=full, capture_output=True, text=True)
        if p.returncode or not os.path.exists(out):
            raise HarnessError(f"reader command failed (exit {p.returncode}):\n{(p.stdout + p.stderr)[-2000:]}")
        raw, _ = read_json(out)
        return raw


def run_score(repo, manifest, paths=None, reader_cmd=None, reader_cwd=None):
    P = paths or DEFAULT_PATHS
    errs = check_freeze(repo, manifest, paths=P)
    if errs:
        raise HarnessError("freeze check failed; reader not run:\n  " + "\n  ".join(errs))
    key, _ = read_json(os.path.join(repo, P["key"]))
    cmd = reader_cmd or ["go", "test", "-count=1", "-run", "^TestFresh2Reader$", "."]
    cwd = reader_cwd or os.path.join(repo, "pipeline")
    env = {"FRESH2_CORPUS": os.path.join(repo, P["corpus"]),
           "FRESH2_CACHE_DIR": os.path.dirname(os.path.join(repo, P["reader_caches"][0]))}
    raw = run_reader(cmd, cwd, env)
    res = compute_scores(key, raw)
    head = (_git(repo, "rev-parse", "HEAD") or "").strip()
    res["frozen"] = dict(manifest, head=head, key_sha256=canon_sha(key))
    rd = os.path.join(repo, P["results_dir"])
    os.makedirs(os.path.dirname(rd), exist_ok=True)
    os.mkdir(rd)  # exclusive: raises if a results directory appeared meanwhile
    write_new(os.path.join(rd, "reader_raw.json"), dumps(raw))
    write_new(os.path.join(rd, "score.json"), dumps(res))
    write_new(os.path.join(rd, "SCORE.md"), render_markdown(res, key))
    return res


# ---------------------------------------------------------------- CLI
def _template(corpus, args):
    lab = {"id": args.labeller_id, "kind": args.kind, "run_date": "", "guide_sha256": guide_sha256(), "session_isolated": None,
           "saw_reader_implementation": None, "saw_other_labels": None, "clinician_verified": False}
    lab.update({"model": "", "model_family": "", "prompt_sha256": ""} if args.kind == "model" else {"name": ""})
    rows = []
    for it in corpus["items"]:
        r = {"id": it["id"], "label": ""}
        if it["kind"] == "ifhas-screen":
            r["coded_label"] = ""
        rows.append(r)
    return {"schema": LABELS_SCHEMA, "corpus_sha256": canon_sha(corpus), "labeller": lab, "labels": rows}


def _load_corpus(path):
    corpus, raw = read_json(path)
    if raw.decode("utf-8") != dumps(corpus):
        raise HarnessError(f"{path} is not in canonical form (re-save it with fresh2.dumps); label files bind to its exact bytes")
    return corpus


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", default=REPO)
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("validate-corpus"); p.add_argument("corpus", nargs="?")
    p = sub.add_parser("template"); p.add_argument("--labeller-id", required=True); p.add_argument("--kind", choices=["model", "human"], required=True); p.add_argument("--out", required=True)
    p = sub.add_parser("ingest"); p.add_argument("--labels", nargs=2)
    p = sub.add_parser("export-inputs"); p.add_argument("outdir")
    p = sub.add_parser("manifest"); p.add_argument("--reader-code-commit", required=True); p.add_argument("--out")
    p = sub.add_parser("check-freeze"); p.add_argument("--manifest"); p.add_argument("--after-score", action="store_true")
    p = sub.add_parser("score"); p.add_argument("--manifest")
    a = ap.parse_args(argv)
    repo, P = a.repo, DEFAULT_PATHS
    path = lambda rel: os.path.join(repo, rel)
    try:
        if a.cmd == "validate-corpus":
            c = _load_corpus(a.corpus or path(P["corpus"]))
            errs = validate_corpus(c, load_known_texts(repo))
            print("\n".join(errs) if errs else f"corpus OK: {len(c['items'])} items, sha256 {canon_sha(c)}")
            return 1 if errs else 0
        if a.cmd == "template":
            doc = _template(_load_corpus(path(P["corpus"])), a)
            write_new(a.out, dumps(doc))
            print("wrote", a.out)
            return 0
        if a.cmd == "ingest":
            c = _load_corpus(path(P["corpus"]))
            errs = validate_corpus(c, load_known_texts(repo))
            if errs:
                print("\n".join(errs))
                return 1
            lp = a.labels or [path(x) for x in P["labels"]]
            (la, ba), (lb, bb) = read_json(lp[0]), read_json(lp[1])
            key, agr = build_key(c, la, lb, ba, bb)
            write_new(path(P["key"]), dumps(key))
            write_new(path(P["agreement"]), dumps(agr))
            print(json.dumps({"counts": key["counts"], "overall": agr["overall"], "warnings": key["warnings"]}, indent=1))
            for kind, k in agr["by_kind"].items():
                print(kind, {f: (v["observed_agreement"], v["kappa"]) for f, v in k["fields"].items()})
            return 0
        if a.cmd == "export-inputs":
            export_reader_inputs(_load_corpus(path(P["corpus"])), a.outdir)
            print("wrote reader inputs under", a.outdir)
            return 0
        if a.cmd == "manifest":
            m = build_manifest(repo, a.reader_code_commit)
            text = dumps(m)
            if a.out:
                write_new(a.out, text)
            print(text, end="")
            return 0
        if a.cmd in ("check-freeze", "score"):
            m, _ = read_json(a.manifest or path(P["manifest"]))
            if a.cmd == "check-freeze":
                errs = check_freeze(repo, m, pre_score=not a.after_score)
                print("\n".join(errs) if errs else "freeze OK")
                return 1 if errs else 0
            res = run_score(repo, m)
            print(json.dumps({"agreed": res["agreed"]["totals"], "disputed": res["disputed"]["totals"]}, indent=1))
            print("wrote", path(P["results_dir"]))
            return 0
    except HarnessError as e:
        print(e, file=sys.stderr)
        return 1
    return 2


if __name__ == "__main__":
    sys.exit(main())
