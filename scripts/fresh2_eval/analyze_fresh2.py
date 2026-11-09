#!/usr/bin/env python3
"""Independent post-score analysis of the fresh2 results (standard library only).

Reads the frozen corpus, key, reader caches and the ORIGINAL results; writes only
data/fresh2/results/ANALYSIS.md and data/fresh2/results/analysis.json. It never edits SCORE.md, score.json, reader_raw.json,
fresh2.py, the guide, the corpus, labels, key or caches, and it verifies that none of them changed since the scoring commit.

  python3 scripts/fresh2_eval/analyze_fresh2.py            # write the two analysis files
  python3 scripts/fresh2_eval/analyze_fresh2.py --check    # recompute and fail if the committed files differ
"""
import argparse, collections, hashlib, json, os, re, subprocess, sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fresh2 as F  # noqa: E402  (read-only use: manifest derivation, scoring recomputation)

REPO = F.REPO
ORIGINAL_SCORE_COMMIT = "8a883fc1a5b8e3758a27bebdb81d4e90a06ff07b"
SCORE_COMMIT = "5b00feb765673a60370e9c07645d0570d0ffe7d7"  # current-history equivalent of the original scoring commit
OUT_MD = "data/fresh2/results/ANALYSIS.md"
OUT_JSON = "data/fresh2/results/analysis.json"
ORIGINAL_RESULTS = ["data/fresh2/results/SCORE.md", "data/fresh2/results/score.json", "data/fresh2/results/reader_raw.json"]
READER_IMPLEMENTATION_FILES = ["pipeline/reader.go", "pipeline/lungread.go", "pipeline/ifhasread.go", "pipeline/ldctread.go"]
STALENESS_DISCLOSURE = "reader implementation changed after scoring; results do not apply to current integration"
INPUT_MANIFEST = "data/fresh2/LABELLER-INPUT-MANIFEST.json"
LABELLER_INPUT = "data/fresh2/labeller-input.json"
CLOSED_STATUS = ("managed", "not_confirmed")
CLOSING_WORKUP = ("wk_treatment", "wk_benign")
DOMS = F.DOMAINS


def load(rel, repo=REPO):
    with open(os.path.join(repo, rel), encoding="utf-8") as f:
        return json.load(f)


def git(repo, *args):
    p = subprocess.run(["git", "-C", repo, *args], capture_output=True, text=True)
    return None if p.returncode else p.stdout


# ------------------------------------------------------------------ helpers mirroring the Go grounding rule
def norm_quote(s):
    """Go normQuote: lowercase, every run of non [a-z0-9] -> one space, trimmed."""
    return re.sub(r"[^a-z0-9]+", " ", s.lower()).strip()


def grounded(quote, text):
    """Go lungGrounded: the quote's alphanumerics appear contiguously (spaces ignored) in the text."""
    words = norm_quote(quote).split()
    return bool(words) and "".join(words) in norm_quote(text).replace(" ", "")


def text_sha(text):
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def parse_composite(label):
    if label in ("dna", "indeterminate"):
        return {}
    return dict(p.split("=") for p in label.split(";"))


# ------------------------------------------------------------------ 1. IFHAS screening, reported separately
def screen_section(raw_rows, key_items, corpus_items):
    rows = {r["id"]: r for r in raw_rows if r["kind"] == "ifhas-screen"}
    agreed = [k for k in key_items if k["kind"] == "ifhas-screen" and k["status"] == "agreed"]
    a_correct = sum(rows[k["id"]]["a"] == k["key"]["label"] for k in agreed)
    b_correct = sum(rows[k["id"]]["b"] == k["key"]["coded_label"] for k in agreed)
    b_key_unavailable = sum(k["key"]["coded_label"] == "unavailable" for k in agreed)
    b_unavailable = sum(not rows[k["id"]]["b_available"] for k in agreed)
    a_glyc_x = [k["id"] for k in agreed if "glyc=X" in rows[k["id"]]["a"] and "glyc=X" not in k["key"]["label"]]
    keyed_glyc = collections.Counter(parse_composite(k["key"]["label"]).get("glyc") for k in agreed)
    a_glyc_all_x = sum("glyc=X" in rows[k["id"]]["a"] for k in agreed)
    texts = {c["id"]: c["text"] for c in corpus_items if c["kind"] == "ifhas-screen"}
    fasting = sum(bool(re.search(r"fasting", t, re.I)) for t in texts.values())
    random_mention = sum(bool(re.search(r"random", t, re.I)) for t in texts.values())
    fasting_yes = sum(bool(re.search(r"fasting\s*:\s*yes|fasting (?:panel|duration)", t, re.I)) for t in texts.values())
    return {
        "reader_b_is": "deterministic coded LOINC/UCUM results (kind ifhas-screen, model field 'coded_lab'); NOT a Gemma reader",
        "panels_total": len(texts), "agreed_n": len(agreed),
        "reader_a_correct": a_correct, "reader_b_coded_matches_key": b_correct,
        "reader_b_unavailable": b_unavailable,
        "key_coded_label_unavailable": b_key_unavailable,
        "note_b": "28/28 counts the 2 items whose key coded_label is itself 'unavailable' as matching; it measures coded-result "
                  "parsing against the key, not a language-model reader.",
        "reader_a_glyc_x_where_key_has_value": len(a_glyc_x),
        "reader_a_glyc_x_ids": a_glyc_x,
        "reader_a_glyc_x_total": a_glyc_all_x,
        "keyed_glyc_distribution": dict(sorted((str(k), v) for k, v in keyed_glyc.items())),
        "guide_glucose_requirement": "random glucose (LABELLING-GUIDE.md section on glyc; LOINC 2345-7 and 14749-6 are labelled 'random glucose')",
        "panels_mentioning_fasting": fasting, "panels_mentioning_random": random_mention,
        "panels_stating_fasting_status_or_duration": fasting_yes,
        "fasting_vs_random_disclosure": f"The guide requires a random glucose, but {fasting}/{len(texts)} panels say fasting. The reader-A "
                                        "panel regex reads only random-glucose wording, so it returns glyc=X on those panels; the key "
                                        "(model-labelled from the same guide) holds values for many of them. The guide and key were "
                                        "frozen and are not changed here.",
    }


# ------------------------------------------------------------------ 2/3. agreed-but-wrong direction
def direction(kind, key_label, reader_label):
    """closing: the reader asserts resolution the key does not support; holding: the reader keeps the loop open
    (possibly the wrong kind of hold) where the key says something else."""
    if kind == "ldct-workup":
        if reader_label in CLOSING_WORKUP and key_label != reader_label:
            return "closing"
        return "holding"
    if kind == "ifhas-followup":
        kd, rd = parse_composite(key_label), parse_composite(reader_label)
        if key_label == "dna" or key_label == "indeterminate":
            return "closing" if any(v in CLOSED_STATUS for v in rd.values()) else "holding"
        for d, v in rd.items():
            if v in CLOSED_STATUS and kd.get(d) in ("pending", None):
                return "closing"
        return "holding"
    return "holding"


NARRATIVE = {
    "f2i-031": "CLOSING direction. Key bp=pending (home BP monitoring requested, 'No medication commenced today; decision deferred pending home "
               "readings'); both readers said bp=managed, so the loop would be closed while the key says it is still pending. This is the "
               "one agreed-but-wrong answer that could close a loop that should stay open.",
    "f2l-002": "HOLDING direction, wrong kind. A pneumonia discharge summary with no lung-cancer work-up content; key indeterminate (a human "
               "should see it). Both readers confidently said wk_incomplete, which keeps the loop open but does not route it to review and "
               "mislabels an unrelated document as an incomplete work-up.",
    "f2l-021": "HOLDING direction, wrong kind. A smoking-cessation review that only mentions the nodule in passing; key indeterminate. Both "
               "readers said wk_incomplete: the loop stays open, but the document was not recognised as the wrong kind of note.",
    "f2l-029": "HOLDING direction. MDT outcome with a stated CT surveillance plan (a decision, not 'pending'); key wk_surveillance. Both readers "
               "said wk_incomplete, so a surveillance plan is held as an incomplete work-up. The loop stays open but the wrong follow-up path "
               "would be started.",
}


def wrong_section(score_by_kind, key_items, raw_rows):
    keyed = {k["id"]: k for k in key_items}
    rows = {r["id"]: r for r in raw_rows}
    out = []
    for kind, s in sorted(score_by_kind.items()):
        for i in s["agreed_but_wrong_ids"]:
            kl = keyed[i]["key"]["label"]
            rl = rows[i]["label"]
            out.append({"id": i, "kind": kind, "key": kl, "reader_a": rows[i]["a"], "reader_b": rows[i]["b"], "agreed_label": rl,
                        "direction": direction(kind, kl, rl), "explanation": NARRATIVE.get(i, "")})
    c = collections.Counter(o["direction"] for o in out)
    return {"items": out, "closing": c.get("closing", 0), "holding": c.get("holding", 0), "total": len(out)}


# ------------------------------------------------------------------ 4. reader-B unavailability, exact reasons
def plausible_coded(token):
    """Analysis-only heuristic naming the implausible coded value (the Go rule is the authority; the key's own
    coded_label=='unavailable' is the cross-check)."""
    code, val, unit = token.split("|")
    v = float(val)
    if code == "4548-4" and unit == "%" and not 3.0 <= v <= 20.0:
        return False
    if code == "2345-7" and unit == "mg/dL" and not 20 <= v <= 1500:
        return False
    return True


def unavailable_section(raw_rows, key_items, corpus_items, ifhas_cache):
    keyed = {k["id"]: k for k in key_items}
    texts = {c["id"]: c for c in corpus_items}
    rows = []
    for r in raw_rows:
        if r["b_available"]:
            continue
        k, status = r["kind"], keyed[r["id"]]["status"]
        if k == "ifhas-followup":
            t = texts[r["id"]]["text"]
            g = ifhas_cache.get(text_sha(t))
            reasons = []
            if g is None:
                reasons.append("cache_miss")
            else:
                if g.get("error"):
                    reasons.append("cache_error")
                if not grounded(g.get("quote", ""), t):
                    reasons.append("main_quote_not_grounded")
                bad = [x["topic"] for x in g.get("findings", []) if not grounded(x.get("quote", ""), t)]
                if bad:
                    reasons.append("finding_quote_not_grounded:" + ",".join(bad))
            rows.append({"id": r["id"], "kind": k, "key_status": status, "reasons": reasons})
        else:
            bad = [tk for tk in texts[r["id"]]["coded"].split(";") if not plausible_coded(tk)]
            rows.append({"id": r["id"], "kind": k, "key_status": status,
                         "reasons": ["coded_results_missing_or_implausible"], "implausible_coded_values": bad,
                         "key_coded_label": keyed[r["id"]]["key"].get("coded_label") if status == "agreed" else None})
    fu = [x for x in rows if x["kind"] == "ifhas-followup"]
    rc = collections.Counter(tuple(x["reasons"]) for x in fu)
    top = sum("main_quote_not_grounded" in x["reasons"] for x in fu)
    fnd = sum(any(r.startswith("finding_quote_not_grounded") for r in x["reasons"]) for x in fu)
    return {
        "rows": rows,
        "by_kind_total": dict(collections.Counter(x["kind"] for x in rows)),
        "by_kind_agreed": dict(collections.Counter(x["kind"] for x in rows if x["key_status"] == "agreed")),
        "by_kind_unadjudicated": dict(collections.Counter(x["kind"] for x in rows if x["key_status"] != "agreed")),
        "ifhas_followup": {
            "unavailable_total": len(fu), "with_cache_entry": sum("cache_miss" not in x["reasons"] for x in fu),
            "main_quote_not_grounded": top, "main_quote_not_grounded_only": sum(x["reasons"] == ["main_quote_not_grounded"] for x in fu),
            "main_and_finding_quote_not_grounded": fnd, "cache_miss": sum("cache_miss" in x["reasons"] for x in fu),
            "cache_error": sum("cache_error" in x["reasons"] for x in fu),
            "reason_combinations": {" + ".join(k): v for k, v in rc.items()},
        },
        "ldct": 0,
        "unavailable_is_not_correct": "Unavailable is not a correct answer. SCORE.md 'B correct' counts only answers equal to the key; "
                                      "'B unavailable' is counted separately and routes the item to a human. The one exception is "
                                      "ifhas-screen, where the key's own coded_label is 'unavailable' for 2 items (see screen section).",
    }


# ------------------------------------------------------------------ verification
def verify_manifest(repo):
    m = load(F.make_paths(F.DEFAULT_EVAL_DIR)["manifest"], repo)
    derived = F.build_manifest(repo, m["reader_code_commit"])
    return {"manifest_file": m, "rederived": derived, "equal": derived == m,
            "check_freeze_after_score_errors": [e for e in F.check_freeze(repo, m, pre_score=False)
                                                if not e.startswith("uncommitted change")],
            "note": "uncommitted-file warnings are excluded from this record so the committed analysis is reproducible; "
                    "run `fresh2.py check-freeze --after-score` for the live cleanliness check"}


def verify_protected(repo):
    """Every frozen/original file must be byte-identical to the scoring commit and to its freeze commit."""
    if git(repo, "cat-file", "-e", SCORE_COMMIT + "^{commit}") is None:
        return {"skipped": f"scoring commit {SCORE_COMMIT} not in this repository", "changed": [], "checked": []}
    m = load(F.make_paths(F.DEFAULT_EVAL_DIR)["manifest"], repo)
    P = F.make_paths(F.DEFAULT_EVAL_DIR)
    paths = (list(F.DEFAULT_PROTECTED) + [P["corpus"], P["key"], P["agreement"], P["manifest"]] + list(P["labels"]) +
             list(P["reader_caches"]) + ORIGINAL_RESULTS + ["scripts/fresh2_eval/fresh2.py", "scripts/fresh2_eval/LABELLING-GUIDE.md"])
    paths = [p for p in paths if git(repo, "cat-file", "-e", f"{SCORE_COMMIT}:{p}") is not None]
    changed = [p for p in git(repo, "diff", "--name-only", SCORE_COMMIT, "--", *paths).split() if p]
    since_reader = [p for p in git(repo, "diff", "--name-only", m["reader_code_commit"], SCORE_COMMIT, "--", *F.DEFAULT_PROTECTED).split() if p]
    dirty = []  # working-tree cleanliness is checked live by fresh2.py check-freeze, not recorded here
    return {"scoring_commit": SCORE_COMMIT, "checked": paths, "changed_since_scoring_commit": changed,
            "protected_changed_since_reader_code_commit": since_reader, "uncommitted": dirty, "changed": changed + since_reader + dirty}


def integration_staleness(repo, original_commit, score_commit, root):
    """Describe current-code drift without changing or reassigning the historical scoring revisions."""
    results = [root + "/results/SCORE.md", root + "/results/score.json", root + "/results/reader_raw.json"]
    stored = [root + "/results/score.json", root + "/results/reader_raw.json"]
    changed = (git(repo, "diff", "--name-only", score_commit, "HEAD", "--", *READER_IMPLEMENTATION_FILES) or "").splitlines()
    original_diff = (git(repo, "diff", "--name-only", original_commit, score_commit, "--", *results) or "").splitlines()
    stored_diff = (git(repo, "diff", "--name-only", score_commit, "--", *stored) or "").splitlines()
    return {
        "disclosure": STALENESS_DISCLOSURE if changed else "reader implementation matches the historical scoring blobs",
        "original_scoring_commit": original_commit,
        "current_history_scoring_equivalent": score_commit,
        "changed_reader_files": changed,
        "historical_results_match_original_scoring_commit": not original_diff,
        "stored_score_and_raw_results_unchanged": not stored_diff,
    }


def render_staleness(s):
    lines = ["**Historical/staleness disclosure: " + s["disclosure"] + ".**", "",
             "Substantively changed reader files:"]
    lines += ["- `" + p + "`" for p in s["changed_reader_files"]]
    lines += ["", "The stored `score.json` and `reader_raw.json` remain historical inputs; no reader was rerun and no score was changed."]
    return lines


def recompute_original_scores(repo):
    key, raw = load(F.make_paths(F.DEFAULT_EVAL_DIR)["key"], repo), load("data/fresh2/results/reader_raw.json", repo)
    res = F.compute_scores(key, raw)
    stored = load("data/fresh2/results/score.json", repo)
    stored_core = {k: v for k, v in stored.items() if k != "frozen"}  # "frozen" is run metadata (head, key hash), not a score
    return res, {"score_json_matches_recomputation": res == stored_core}


# ------------------------------------------------------------------ build
def build(repo=REPO):
    key, raw = load(F.make_paths(F.DEFAULT_EVAL_DIR)["key"], repo), load("data/fresh2/results/reader_raw.json", repo)
    corpus = load(F.make_paths(F.DEFAULT_EVAL_DIR)["corpus"], repo)
    cache = load("data/fresh2/reader-cache/ifhas_gemma_cache.json", repo)
    res, rec = recompute_original_scores(repo)
    by_kind = res["agreed"]["by_kind"]
    screen = screen_section(raw["items"], key["items"], corpus["items"])
    gemma_kinds = ["ifhas-followup", "ldct-screen", "ldct-workup"]
    gem = {k: by_kind[k] for k in gemma_kinds}
    sums = lambda f: sum(v[f] for v in gem.values())
    gemma_totals = {"n": sums("n"), "reader_a_correct": sums("reader_a_correct"), "reader_b_gemma_correct": sums("reader_b_correct"),
                    "reader_b_unavailable": sums("reader_b_unavailable"), "readers_agree": sums("agree"),
                    "agreed_but_wrong": sums("agreed_but_wrong")}
    return {
        "schema": "fresh2-analysis/v1",
        "semantics": key["semantics"]["statement"],
        "counts": key["counts"],
        "ifhas_screening_coded_not_gemma": screen,
        "gemma_reader_kinds_agreed_subset": {"kinds": gemma_kinds, "by_kind": gem, "totals": gemma_totals,
                                             "note": "Reader B is local Gemma 3 4B (cached) for these three kinds only. ifhas-screen is excluded."},
        "agreed_but_wrong": wrong_section(by_kind, key["items"], raw["items"]),
        "reader_b_unavailable": unavailable_section(raw["items"], key["items"], corpus["items"], cache),
        "verification": {"original_scores": rec, "manifest": verify_manifest(repo), "protected_files": verify_protected(repo),
                         "integration_staleness": integration_staleness(repo, ORIGINAL_SCORE_COMMIT, SCORE_COMMIT, "data/fresh2")},
    }


def render(a):
    s, g, w, u, v = (a["ifhas_screening_coded_not_gemma"], a["gemma_reader_kinds_agreed_subset"], a["agreed_but_wrong"],
                     a["reader_b_unavailable"], a["verification"])
    t = g["totals"]
    L = ["# Fresh2 post-score analysis", "", "**" + a["semantics"] + "**", ""]
    L += render_staleness(v["integration_staleness"])
    L += ["", "Generated by `scripts/fresh2_eval/analyze_fresh2.py` from the frozen corpus, key, caches and the original results. It does not edit "
         "`SCORE.md`, `score.json` or `reader_raw.json`. Status: **built** and run on **simulated** (model-authored synthetic) text.", "",
         f"Items {a['counts']['items']}; agreed {a['counts']['agreed']}; unadjudicated {a['counts']['unadjudicated']}.", "",
         "## 1. IFHAS screening is coded results, not a Gemma reader", "",
         "Reader B for `ifhas-screen` is the deterministic coded LOINC/UCUM string (`coded_lab`), and reader A is a regex over the printed "
         "panel. No language model reads these items, so they are reported separately and excluded from the Gemma figures below.", "",
         f"- Agreed screening panels: {s['agreed_n']} (of {s['panels_total']}). Reader A (panel regex) matched the key on **{s['reader_a_correct']}/{s['agreed_n']}**; "
         f"coded reader B matched the key's `coded_label` on **{s['reader_b_coded_matches_key']}/{s['agreed_n']}**, including the "
         f"{s['key_coded_label_unavailable']} items whose key coded_label is itself `unavailable` (coded parsing is deterministic code, not a model).",
         f"- Guide/panel mismatch: the labelling guide requires a **random** glucose, but **{s['panels_mentioning_fasting']}/{s['panels_total']}** panels say fasting. "
         f"Reader A's regex reads only random-glucose wording, so it returned `glyc=X` on **{s['reader_a_glyc_x_total']}/{s['agreed_n']}** agreed panels; on "
         f"**{s['reader_a_glyc_x_where_key_has_value']}** of them the key holds a real glyc value, not X (keyed glyc distribution: {s['keyed_glyc_distribution']}). "
         "The guide and key were frozen and are not changed; this is a disclosed limitation of the test, not a reader fix. "
         "(Reader A also returned glyc=X on both unadjudicated panels, which have no key.)",
         f"- Reader-B unavailable on screening: {u['by_kind_total'].get('ifhas-screen', 0)} (implausible coded value under its unit; see section 4).", "",
         "## 2. Gemma-reader results (LDCT screen, LDCT work-up, IFHAS follow-up; agreed subset)", "",
         "| kind | n | A correct | B (Gemma) correct | B unavailable | readers agree | agreed but wrong |", "|---|---|---|---|---|---|---|"]
    for k, x in g["by_kind"].items():
        L.append(f"| {k} | {x['n']} | {x['reader_a_correct']} | {x['reader_b_correct']} | {x['reader_b_unavailable']} | {x['agree']} | {x['agreed_but_wrong']} |")
    L += [f"| **total** | {t['n']} | {t['reader_a_correct']} | {t['reader_b_gemma_correct']} | {t['reader_b_unavailable']} | {t['readers_agree']} | {t['agreed_but_wrong']} |",
          "", "Scored against a model-labelled key. Items the labellers disagreed on (7) carry no key and are not scored for accuracy.", "",
          "## 3. The agreed-but-wrong answers, by direction", "",
          f"{w['total']} agreed-but-wrong answers: **{w['closing']} closing-direction**, **{w['holding']} holding-direction**. A closing-direction error "
          "is one where both readers resolve a loop the key says is not resolved; a holding-direction error keeps the loop open but names the wrong state "
          "or fails to route a wrong-kind document to a human.", "",
          "| id | kind | key | agreed reader label | direction |", "|---|---|---|---|---|"]
    for x in w["items"]:
        L.append(f"| {x['id']} | {x['kind']} | {x['key']} | {x['agreed_label']} | {x['direction']} |")
    L.append("")
    for x in w["items"]:
        L.append(f"- **{x['id']}**: {x['explanation']}")
    L += ["", "This is a count of four on 113 agreed items for synthetic text. It is not a rate of unsafe closure on real patients, and a "
          "count of one closing-direction error means the zero-error claim cannot be made.", "",
          "## 4. Why reader B was unavailable", "",
          "Recomputed from `reader_raw.json` (`b_available=false`) and the frozen Gemma cache, using the Go grounding rule (`normQuote`/`lungGrounded`).", "",
          f"- Total unavailable: {sum(u['by_kind_total'].values())} ({u['by_kind_total']}); agreed {u['by_kind_agreed']}, unadjudicated {u['by_kind_unadjudicated']}. LDCT: {u['ldct']}.",
          f"- IFHAS follow-up: **{u['ifhas_followup']['unavailable_total']}** unavailable, all with a cache entry (cache misses {u['ifhas_followup']['cache_miss']}, cache errors {u['ifhas_followup']['cache_error']}). "
          f"In all **{u['ifhas_followup']['main_quote_not_grounded']}** the model's main decision quote is not contiguous in the note (quote-grounding failure); "
          f"**{u['ifhas_followup']['main_quote_not_grounded_only']}** fail on that alone and **{u['ifhas_followup']['main_and_finding_quote_not_grounded']}** also have ungrounded per-finding quotes.",
          f"- Combinations: {u['ifhas_followup']['reason_combinations']}.",
          "- IFHAS screening: 2 (`coded_results_missing_or_implausible`): " + "; ".join(
              f"{x['id']} {x['implausible_coded_values']}" for x in u["rows"] if x["kind"] == "ifhas-screen") + ".", "",
          u["unavailable_is_not_correct"], "",
          "| id | kind | key status | reasons |", "|---|---|---|---|"]
    for x in u["rows"]:
        L.append(f"| {x['id']} | {x['kind']} | {x['key_status']} | {'; '.join(x['reasons'])} |")
    m, p, o = v["manifest"], v["protected_files"], v["original_scores"]
    L += ["", "## 5. Verification", "",
          f"- `score.json` equals a fresh `compute_scores(key, reader_raw)`: **{o['score_json_matches_recomputation']}**.",
          f"- Freeze manifest re-derived from git history with `fresh2.build_manifest`: **{'identical' if m['equal'] else 'DIFFERS'}** to `FREEZE.json`.",
          f"- `check_freeze(after-score)` errors: {m['check_freeze_after_score_errors'] or 'none'}.",
          f"- Protected/original files changed since scoring commit {p.get('scoring_commit', SCORE_COMMIT)}: "
          f"{p.get('changed') or 'none'} ({len(p.get('checked', []))} files compared).", ""]
    return "\n".join(L)


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", default=REPO)
    ap.add_argument("--check", action="store_true")
    a = ap.parse_args(argv)
    res = build(a.repo)
    js, md = F.dumps(res), render(res) + "\n"
    jp, mp = os.path.join(a.repo, OUT_JSON), os.path.join(a.repo, OUT_MD)
    if a.check:
        same = os.path.exists(jp) and open(jp).read() == js and os.path.exists(mp) and open(mp).read() == md
        print("analysis files up to date" if same else "analysis files differ")
        return 0 if same else 1
    with open(jp, "w", encoding="utf-8") as f:
        f.write(js)
    with open(mp, "w", encoding="utf-8") as f:
        f.write(md)
    print("wrote", OUT_JSON, "and", OUT_MD)
    return 0


if __name__ == "__main__":
    sys.exit(main())
