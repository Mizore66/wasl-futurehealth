#!/usr/bin/env python3
"""Post-score analysis of the fresh3 results (standard library only).

Reads the frozen fresh3 corpus, key, labels, caches and the ORIGINAL results; writes only
data/fresh3/results/ANALYSIS.md and data/fresh3/results/analysis.json. It never edits the corpus, labels, key, caches,
SCORE.md, score.json, reader_raw.json, the readers or fresh2.py, and it verifies that none of them changed since scoring.
It reuses the read-only helpers of analyze_fresh2.py (which is not modified).

  python3 scripts/fresh2_eval/analyze_fresh3.py            # write the two analysis files
  python3 scripts/fresh2_eval/analyze_fresh3.py --check    # recompute and fail if the committed files differ
"""
import argparse, collections, hashlib, json, os, re, sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import fresh2 as F  # noqa: E402
import analyze_fresh2 as A  # noqa: E402

REPO = F.REPO
ROOT = "data/fresh3"
P3 = F.make_paths(ROOT)
SCORE_COMMIT = "d3222328fc1cbfeff40faf4731fa09f4e3cc19b8"  # current-history equivalent of the original scoring commit
OUT_MD = ROOT + "/results/ANALYSIS.md"
OUT_JSON = ROOT + "/results/analysis.json"
ORIGINAL_RESULTS = [ROOT + "/results/SCORE.md", ROOT + "/results/score.json", ROOT + "/results/reader_raw.json"]
HASHED = ([P3["corpus"], ROOT + "/labeller-input.json", ROOT + "/LABELLER-INPUT-MANIFEST.json", "scripts/fresh2_eval/LABELLING-GUIDE.md"] +
          list(P3["labels"]) + [P3["key"], P3["agreement"]] + list(P3["reader_caches"]) + [P3["manifest"]] + ORIGINAL_RESULTS)
FRESH2_BASELINE = {  # from data/fresh2/results/ANALYSIS.md and score.json (read-only; recomputed below as a cross-check)
    "ifhas_followup_b_unavailable_agreed": 10, "ifhas_followup_agreed_but_wrong": 1, "ldct_workup_agreed_but_wrong": 3}

NARRATIVE = {
    "f3l-008": {
        "diagnosis": "reader_defect",
        "text": "The note states an MDT consensus plan: an interval thin-section chest CT at three months, due 04 August 2026, "
                "with a CT request entered. The guide (rule 5) says a stated repeat-CT plan is a decision, not pending, so the "
                "model-labelled key `wk_surveillance` (both labellers) follows the guide and the text is not ambiguous. Reader A "
                "(regex) has no match for the phrase because `interval thin-section chest CT` does not fit its `interval CT` "
                "pattern, so the outcome defaulted to `pending`. Reader B (Gemma) extracted `workup=not_done, outcome=pending` "
                "although it quoted the plan sentence itself. The new MDT-title/surveillance override could not fire, for three separate "
                "reasons: its title pattern is anchored at the start of the text (the first line is the hospital name), it needs "
                "`outcome` or `decision` after `MDT`/`multidisciplinary` (the title is `Thoracic Multidisciplinary Meeting Record`), "
                "and its surveillance-plan test needs a phrase such as `interval CT` or `CT in 3 months` (confirmed: neither "
                "`ldctSurveillancePlan` nor the title pattern matches this note).",
        "direction_note": "Holding: the loop stays open (`wk_incomplete`) where the key says a surveillance plan exists. The wrong "
                          "follow-up path would be started, but nothing is closed."},
    "f3l-019": {
        "diagnosis": "reader_defect_with_key_strict_per_guide",
        "text": "A thoracic clinic note: the opacity has completely resolved on interval CT, no biopsy, discharge, resume annual "
                "LDCT through the referring service. The key `indeterminate` (both labellers) is the strict reading of guide rule 4 "
                "(a bare return to annual screening needs benign tissue or a recorded MDT decision, and neither is present). A "
                "clinician could reasonably read a resolved nodule as a benign outcome, so the key is defensible but conservative "
                "rather than the only reading; it is not a key error under the frozen guide. Both readers are still wrong for "
                "either reading, because nothing in the note is pending. Reader A misses `resume annual LDCT` (its pattern wants "
                "`annual screening`; the only sentence mentioning `interval CT` is dropped as history because it contains "
                "`previously`), so the outcome defaulted to `pending`. Reader B (Gemma) invented `tissue=benign` (there is no "
                "biopsy or tissue result) and `workup=not_done`. The wrong-document rule did not fire because the note contains "
                "LDCT and thoracic wording.",
        "direction_note": "Holding. Counterfactual: Gemma's invented `tissue=benign` plus `annual` would have produced the closing "
                          "label `wk_benign` had it also said `workup=done`; closing still needs reader A to agree, and reader A "
                          "said `wk_incomplete`, so no closure was possible. The safe outcome here rested on two independent "
                          "extraction errors, not on a safeguard."},
    "f3l-048": {
        "diagnosis": "reader_defect",
        "text": "A diagnostic CT chest radiology report (17 x 13 mm irregular nodule, comparison with the screening LDCT, signed by "
                "the radiologist). It has no attendance event, no MDT, no tissue plan and no outcome, so it is a different kind of "
                "document; the key `indeterminate` (both labellers) sends it to a human and is consistent with the guide. Both "
                "readers defaulted to `wk_incomplete`: reader A because `outcome` defaults to `pending` when no outcome phrase is "
                "found, and reader B (Gemma) quoted the signature line `Reported by Dr ...` and set `mdt=discussed`, which the text "
                "never states. The wrong-document rule did not fire because it only tests for a lung/thoracic anchor and a work-up "
                "term, and the text satisfies both through `screening LDCT` and `LDCT`, used here only as the comparison study.",
        "direction_note": "Holding: the loop stays open, but the document is not routed to a human as the key expects. Holding an "
                          "open nodule is not itself unsafe, but a wrong-kind document confidently labelled as an incomplete work-up "
                          "is the failure the new wrong-document safeguard was meant to catch."},
}


def sha_file(rel, repo=REPO):
    with open(os.path.join(repo, rel), "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


def protocol(repo, key, corpus, manifest):
    labels = [A.load(p, repo) for p in P3["labels"]]
    return {
        "file_sha256": {p: sha_file(p, repo) for p in HASHED},
        "key_canonical_sha256": F.canon_sha(key),
        "freeze_manifest": manifest,
        "scoring_commit": SCORE_COMMIT,
        "labellers": [{k: l["labeller"][k] for k in ("id", "model", "model_family", "prompt_sha256", "guide_sha256", "run_date",
                                                      "session_isolated", "saw_reader_implementation", "saw_other_labels",
                                                      "clinician_verified")} for l in labels],
        "author": {k: corpus["provenance"][k] for k in ("author_id", "author_prompt_version", "author_system_prompt_sha256",
                                                          "author_saw_reader_code", "authoring", "disclosure")},
        "readers": {"reader_code_commit": manifest["reader_code_commit"],
                    "reader_b": "local Gemma 3 4B (cached) for ldct-screen, ldct-workup, ifhas-followup; none for ifhas-screen"},
    }


def ldct_workup_detail(key_items, raw_rows):
    keyed = {k["id"]: k for k in key_items if k["kind"] == "ldct-workup" and k["status"] == "agreed"}
    rows = {r["id"]: r for r in raw_rows}
    expects_review = [i for i, k in keyed.items() if k["key"]["label"] == "indeterminate"]
    routed = [i for i in expects_review if not rows[i]["agree"]]
    wrong = [i for i in expects_review if rows[i]["agree"]]
    return {"agreed_n": len(keyed), "key_indeterminate_ids": expects_review, "key_indeterminate_routed_to_review": routed,
            "key_indeterminate_agreed_wrong": wrong}


def wrong_section(by_kind, key_items, raw_rows):
    w = A.wrong_section(by_kind, key_items, raw_rows)
    for x in w["items"]:
        n = NARRATIVE[x["id"]]
        x["diagnosis"], x["explanation"], x["direction_note"] = n["diagnosis"], n["text"], n["direction_note"]
    return w


def unavailable_section(raw_rows, key_items, corpus_items, ifhas_cache):
    keyed = {k["id"]: k for k in key_items}
    texts = {c["id"]: c for c in corpus_items}
    rows = []
    for r in raw_rows:
        if r["b_available"] or r["kind"] not in ("ifhas-followup", "ifhas-screen"):
            continue
        status = keyed[r["id"]]["status"]
        if r["kind"] == "ifhas-followup":
            t = texts[r["id"]]["text"]
            g = ifhas_cache.get(A.text_sha(t))
            if g is None:
                reasons = ["cache_miss"]
            else:
                reasons = []
                if g.get("error"):
                    reasons.append("cache_error")
                if not A.grounded(g.get("quote", ""), t):
                    reasons.append("main_quote_not_grounded")
                bad = [x["topic"] for x in g.get("findings", []) if not A.grounded(x.get("quote", ""), t)]
                if bad:
                    reasons.append("finding_quote_not_grounded:" + ",".join(bad))
            rows.append({"id": r["id"], "kind": r["kind"], "key_status": status, "reasons": reasons,
                         "cache_rev": (g or {}).get("rev")})
        else:
            bad = [tk for tk in texts[r["id"]]["coded"].split(";") if not A.plausible_coded(tk)]
            rows.append({"id": r["id"], "kind": r["kind"], "key_status": status, "reasons": ["coded_results_implausible"],
                         "implausible_coded_values": bad,
                         "key_coded_label": keyed[r["id"]]["key"].get("coded_label") if status == "agreed" else None})
    return {"rows": rows,
            "by_kind_total": dict(collections.Counter(x["kind"] for x in rows)),
            "by_kind_agreed": dict(collections.Counter(x["kind"] for x in rows if x["key_status"] == "agreed")),
            "by_kind_unadjudicated": dict(collections.Counter(x["kind"] for x in rows if x["key_status"] != "agreed")),
            "ldct": sum(1 for r in raw_rows if r["kind"].startswith("ldct") and not r["b_available"])}


def coded_screen_detail(raw_rows, key_items, corpus_items):
    """Separate IFHAS coded screening: flagged unavailable vs placeholder (no coded results) vs key coded_label."""
    rows = {r["id"]: r for r in raw_rows if r["kind"] == "ifhas-screen"}
    texts = {c["id"]: c for c in corpus_items}
    out = {"flag_unavailable": [], "placeholder_no_coded_results": [], "key_coded_unavailable": [], "b_wrong_vs_key": []}
    for k in key_items:
        if k["kind"] != "ifhas-screen":
            continue
        i = k["id"]
        r, agreed = rows[i], k["status"] == "agreed"
        if not r["b_available"]:
            out["flag_unavailable"].append(i)
        if texts[i]["coded"].startswith("no-coded-results"):
            out["placeholder_no_coded_results"].append(i)
        if agreed:
            if k["key"]["coded_label"] == "unavailable":
                out["key_coded_unavailable"].append(i)
            if r["b"] != k["key"]["coded_label"]:
                out["b_wrong_vs_key"].append(i)
    out["note"] = ("`b_available` is false only when the coded string is missing or has an implausible value. Items whose coded string "
                   "is the inert placeholder `no-coded-results|0|none` still return b_available=true with an all-X label. When the key's "
                   "coded_label is `unavailable`, SCORE.md counts reader B correct only if it literally returned `unavailable`.")
    return out


def build(repo=REPO):
    old = os.environ.get("FRESH_EVAL_PER_PROGRAMME")
    os.environ["FRESH_EVAL_PER_PROGRAMME"] = "50"  # fresh3 corpus size; check_freeze validates the corpus against it
    try:
        return _build(repo)
    finally:
        if old is None:
            os.environ.pop("FRESH_EVAL_PER_PROGRAMME", None)
        else:
            os.environ["FRESH_EVAL_PER_PROGRAMME"] = old


def _build(repo):
    key, raw = A.load(P3["key"], repo), A.load(ROOT + "/results/reader_raw.json", repo)
    corpus, manifest = A.load(P3["corpus"], repo), A.load(P3["manifest"], repo)
    stored = A.load(ROOT + "/results/score.json", repo)
    res = F.compute_scores(key, raw)
    by_kind = res["agreed"]["by_kind"]
    cache = A.load(P3["reader_caches"][1], repo)
    gemma_kinds = ["ifhas-followup", "ldct-screen", "ldct-workup"]
    gem = {k: by_kind[k] for k in gemma_kinds}
    sums = lambda f: sum(v[f] for v in gem.values())
    unav = unavailable_section(raw["items"], key["items"], corpus["items"], cache)
    wrong = wrong_section(by_kind, key["items"], raw["items"])
    screen = A.screen_section(raw["items"], key["items"], corpus["items"])
    # The shared fresh2 helper's note still says "28/28" (its fresh2 denominator); override it with the fresh3 figure.
    screen["note_b"] = (f"{screen['reader_b_coded_matches_key']}/{screen['agreed_n']}; of the {screen['key_coded_label_unavailable']} items whose key "
                        "coded_label is unavailable, only f3i-013 counts as a match (B returned `unavailable`); f3i-032 and f3i-046 are misses. "
                        "It measures coded-result parsing against the key, not a language-model reader.")
    f2 = A.load("data/fresh2/results/score.json", repo)["agreed"]["by_kind"]
    f2_base = {"ifhas_followup_b_unavailable_agreed": f2["ifhas-followup"]["reader_b_unavailable"],
               "ifhas_followup_agreed_but_wrong": f2["ifhas-followup"]["agreed_but_wrong"],
               "ldct_workup_agreed_but_wrong": f2["ldct-workup"]["agreed_but_wrong"]}
    f3 = {"ifhas_followup_b_unavailable_agreed": by_kind["ifhas-followup"]["reader_b_unavailable"],
          "ifhas_followup_agreed_but_wrong": by_kind["ifhas-followup"]["agreed_but_wrong"],
          "ldct_workup_agreed_but_wrong": by_kind["ldct-workup"]["agreed_but_wrong"]}
    unscored = [i for i in key["items"] if i["status"] != "agreed"]
    return {
        "schema": "fresh3-analysis/v1",
        "semantics": key["semantics"]["statement"],
        "status": "built; run on simulated (model-authored synthetic) text; model-labelled keys; not clinical effectiveness",
        "protocol": protocol(repo, key, corpus, manifest),
        "counts": key["counts"],
        "agreement": {"item_agreement": A.load(P3["agreement"], repo)["overall"],
                      "unadjudicated_ids": [i["id"] for i in unscored]},
        "scores_by_kind_agreed": by_kind,
        "scores_totals_agreed": res["agreed"]["totals"],
        "gemma_reader_kinds_agreed_subset": {
            "kinds": gemma_kinds,
            "totals": {"n": sums("n"), "reader_a_correct": sums("reader_a_correct"), "reader_b_gemma_correct": sums("reader_b_correct"),
                       "reader_b_unavailable": sums("reader_b_unavailable"), "readers_agree": sums("agree"),
                       "agreed_but_wrong": sums("agreed_but_wrong")}},
        "ifhas_screening_coded_not_gemma": screen,
        "ifhas_coded_screening_unavailability": coded_screen_detail(raw["items"], key["items"], corpus["items"]),
        "disputed_subset_unscored": res["disputed"],
        "agreed_but_wrong": wrong,
        "ldct_workup_detail": ldct_workup_detail(key["items"], raw["items"]),
        "reader_b_unavailable": unav,
        "comparison_fresh2_to_fresh3": {
            "scope": "Only definitions that match are compared. Different corpora, different author model, different size; "
                     "descriptive, not a statistical test, and not an overall accuracy comparison.",
            "fresh2": f2_base, "fresh3": f3,
            "expected_headline": {"ifhas_followup_b_unavailable_agreed": [10, 1], "ifhas_followup_agreed_but_wrong": [1, 0],
                                  "ldct_workup_agreed_but_wrong": [3, 3]},
            "headline_matches": f2_base == FRESH2_BASELINE and (f3["ifhas_followup_b_unavailable_agreed"], f3["ifhas_followup_agreed_but_wrong"],
                                                                f3["ldct_workup_agreed_but_wrong"]) == (1, 0, 3),
            "not_compared": ["overall accuracy", "overall agreed-but-wrong (4 of 113 vs 3 of 93: different mix of kinds)",
                             "'0 unsafe' (not claimed for either round)"]},
        "safeguard_replay": REPLAY,
        "verification": verification(repo, key, raw, stored, res),
    }


# Replay of the CURRENT readers over the FRESH2 items, via the committed Go regression test
# pipeline/fresh3_safeguard_test.go (TestFresh3SafeguardReplayFresh2). Post-hoc and tuned: the safeguards were written
# after seeing these items, so this is a regression check, not evidence of generalisation.
REPLAY = {
    "method": "committed Go test pipeline/fresh3_safeguard_test.go (TestFresh3SafeguardReplayFresh2) calling LDCTJudge over "
              "data/fresh2/corpus.json with the fresh2 reader cache (no model call); fresh2 original results are unchanged",
    "test": "pipeline/fresh3_safeguard_test.go",
    "fresh2_f2l-002": {"original": "both wk_incomplete", "current_code": "A indeterminate, B indeterminate (routed to review)"},
    "fresh2_f2l-021": {"original": "both wk_incomplete", "current_code": "A indeterminate, B indeterminate (routed to review)"},
    "fresh2_f2l-029": {"original": "both wk_incomplete", "current_code": "A wk_surveillance (regex), B wk_surveillance (the MDT-title surveillance override turned Gemma's workup=not_done into done)"},
    "fresh3_rule_firings_on_ldct_workup": "in the same replay over all 25 fresh3 work-up items (agreed and disputed), neither the wrong-document rule "
                                          "(for reader A or B) nor the MDT-title surveillance override changed any label, so neither safeguard "
                                          "fired once on fresh3",
    "caveat": "post-hoc regression check on the items the rules were written for; it is not a fair test and not generalisation evidence",
}


def verification(repo, key, raw, stored, res):
    m = A.load(P3["manifest"], repo)
    derived = F.build_manifest(repo, m["reader_code_commit"], paths=P3)
    errs = [e for e in F.check_freeze(repo, m, paths=P3, pre_score=False) if not e.startswith("uncommitted change")]
    core = {k: v for k, v in stored.items() if k != "frozen"}
    prot = list(F.DEFAULT_PROTECTED) + [P3["corpus"], P3["key"], P3["agreement"], P3["manifest"]] + list(P3["labels"]) + \
        list(P3["reader_caches"]) + ORIGINAL_RESULTS
    changed = [p for p in (A.git(repo, "diff", "--name-only", SCORE_COMMIT, "HEAD", "--", *prot) or "").split() if p]
    since_reader = [p for p in (A.git(repo, "diff", "--name-only", m["reader_code_commit"], SCORE_COMMIT, "--", *F.DEFAULT_PROTECTED) or "").split() if p]
    return {"score_json_matches_recomputation": res == core, "manifest_rederived_equal": derived == m,
            "check_freeze_after_score_errors": errs, "files_compared": len(prot),
            "changed_since_scoring_commit": changed, "protected_changed_since_reader_code_commit": since_reader}


# ------------------------------------------------------------------ rendering
def render(a):
    s, w, u, v, p = (a["ifhas_screening_coded_not_gemma"], a["agreed_but_wrong"], a["reader_b_unavailable"], a["verification"], a["protocol"])
    cs, cmp_, g = a["ifhas_coded_screening_unavailability"], a["comparison_fresh2_to_fresh3"], a["gemma_reader_kinds_agreed_subset"]
    ag, wd = a["agreement"], a["ldct_workup_detail"]
    bk, T = a["scores_by_kind_agreed"], a["scores_totals_agreed"]
    L = ["# Fresh3 post-score analysis", "", "**" + a["semantics"] + "**", "",
         "Generated by `scripts/fresh2_eval/analyze_fresh3.py` from the frozen fresh3 corpus, key, labels, caches and the original results. "
         "It does not edit any frozen file or `SCORE.md`, `score.json`, `reader_raw.json`. Status: **built**, run on **simulated** "
         "(model-authored synthetic) text, with **model-labelled** keys. Fresh3 is the fair test after reader hardening: the hardening "
         "commits precede the corpus, and neither the author nor the labellers saw reader code (declared; chronology rests on commit order "
         "and declared hashes, with no recording of the model calls).", "",
         f"Items {a['counts']['items']}; two labellers agreed on **{ag['item_agreement']['agreed']}/{ag['item_agreement']['items']}**; "
         f"**{a['counts']['unadjudicated']} unadjudicated** ({', '.join(ag['unadjudicated_ids'])}) carry no key and are not scored for accuracy.", "",
         "## 1. Protocol and hashes", "",
         f"- Reader code commit `{p['freeze_manifest']['reader_code_commit']}`; corpus `{p['freeze_manifest']['corpus_commit']}`; labels "
         f"`{p['freeze_manifest']['labels_commit']}`; reader caches `{p['freeze_manifest']['reader_cache_commit']}`; scored once at `{p['scoring_commit']}`.",
         f"- Author: `{p['author']['author_id']}` (prompt `{p['author']['author_prompt_version']}`, system prompt sha256 `{p['author']['author_system_prompt_sha256']}`, saw reader code: {p['author']['author_saw_reader_code']}).",
         "- Labellers: " + "; ".join(f"`{l['id']}` = {l['model']} ({l['model_family']}), prompt sha256 `{l['prompt_sha256']}`, isolated {l['session_isolated']}, saw reader {l['saw_reader_implementation']}, saw other labels {l['saw_other_labels']}" for l in p["labellers"]) + ".",
         "- Reader B (Gemma 3 4B, cached) is a Google-family model and one labeller is Gemini (Google): correlation between them is possible and was not tested.",
         f"- Key canonical sha256 `{p['key_canonical_sha256']}`.", "", "| file | sha256 |", "|---|---|"]
    for f, h in p["file_sha256"].items():
        L.append(f"| `{f}` | `{h}` |")
    L += ["", "## 2. Exact scores by kind (agreed subset, n=%d)" % T["n"], "",
          "| kind | n | A correct | B correct | B unavailable | readers agree | agreed but wrong | review (key decisive) | review (key expects) |", "|---|---|---|---|---|---|---|---|---|"]
    for k, x in sorted(bk.items()):
        L.append(f"| {k} | {x['n']} | {x['reader_a_correct']} | {x['reader_b_correct']} | {x['reader_b_unavailable']} | {x['agree']} | {x['agreed_but_wrong']} | {x['routed_to_review_key_decisive']} | {x['routed_to_review_key_expects_review']} |")
    L.append(f"| **total** | {T['n']} | {T['reader_a_correct']} | {T['reader_b_correct']} | {T['reader_b_unavailable']} | {T['agree']} | {T['agreed_but_wrong']} | {T['routed_to_review_key_decisive']} | {T['routed_to_review_key_expects_review']} |")
    gt = g["totals"]
    L += ["", f"Gemma-read kinds only (LDCT screen, LDCT work-up, IFHAS follow-up; n={gt['n']}): reader A correct {gt['reader_a_correct']}, Gemma reader B correct {gt['reader_b_gemma_correct']}, B unavailable {gt['reader_b_unavailable']}, readers agree {gt['readers_agree']}, agreed-but-wrong {gt['agreed_but_wrong']}. "
          "The `total` row mixes Gemma and IFHAS coded screening and should not be read as a Gemma figure.", "",
          "Disputed subset (no key, unscored for accuracy): " + "; ".join(f"{k} n={x['n']}, readers agreed {x['reader_agreed']}, routed to review {x['reader_routed_to_review']}" for k, x in sorted(a["disputed_subset_unscored"]["by_kind"].items())) +
          f". The one agreed disputed item ({', '.join(a['disputed_subset_unscored']['by_kind']['ldct-screen']['ids_reader_agreed'])}) matches one labeller; this is behaviour, not accuracy.", "",
          "## 3. IFHAS coded screening is not a Gemma reader", "",
          f"Reader B for `ifhas-screen` is the deterministic coded LOINC/UCUM string (`coded_lab`); reader A is a regex over the printed panel. No language model reads these items. Agreed panels {s['agreed_n']} of {s['panels_total']}: reader A matched the key on **{s['reader_a_correct']}/{s['agreed_n']}**; coded reader B matched the key's `coded_label` on **{s['reader_b_coded_matches_key']}/{s['agreed_n']}**; of the {s['key_coded_label_unavailable']} items whose key coded_label is unavailable, only f3i-013 counts as a match (B returned `unavailable`); f3i-032 and f3i-046 are misses.",
          f"- Guide/panel mismatch persists: the guide wants a random glucose but **{s['panels_mentioning_fasting']}/{s['panels_total']}** panels say fasting. Reader A returned `glyc=X` on **{s['reader_a_glyc_x_total']}/{s['agreed_n']}** agreed panels, on **{s['reader_a_glyc_x_where_key_has_value']}** of them where the key holds a real value. The guide and key are frozen; this is a limitation of the test and of reader A, not a reader fix. Reader A is the weaker reader here: only 1 agreed panel had both readers agree.",
          f"- Coded unavailability, exact: flagged `b_available=false`: {cs['flag_unavailable']}; placeholder `no-coded-results|0|none` (B returns an all-X label with `b_available=true`): {cs['placeholder_no_coded_results']}; key coded_label `unavailable` (agreed): {cs['key_coded_unavailable']}; agreed items where B differs from the key coded_label: {cs['b_wrong_vs_key']}.",
          f"- {cs['note']}", "",
          "## 4. The agreed-but-wrong answers: diagnosis and direction", "",
          f"{w['total']} agreed-but-wrong, all in `ldct-workup`: **{w['closing']} closing-direction**, **{w['holding']} holding-direction**. Closing = both readers resolve a loop the key says is not resolved; holding = the loop stays open but is the wrong state or a wrong-kind document is not routed to a human.", "",
          "| id | key | reader A | reader B | agreed label | direction | diagnosis |", "|---|---|---|---|---|---|---|"]
    for x in w["items"]:
        L.append(f"| {x['id']} | {x['key']} | {x['reader_a']} | {x['reader_b']} | {x['agreed_label']} | {x['direction']} | {x['diagnosis']} |")
    L.append("")
    for x in w["items"]:
        L += [f"- **{x['id']}**: {x['explanation']} {x['direction_note']}"]
    L += ["", "No key error was found: all three keys agree with the frozen guide under a literal reading, and f3l-019 is the only one where a reasonable "
          "clinician could prefer another label (still not `wk_incomplete`). No text was found to be so ambiguous that a reader error is excusable. "
          "Because the keys are model-labelled, even this classification is a reading of the guide, not a clinical judgement.", "",
          f"Of the {len(wd['key_indeterminate_ids'])} agreed work-up items keyed `indeterminate` ({', '.join(wd['key_indeterminate_ids'])}), "
          f"{len(wd['key_indeterminate_routed_to_review'])} were routed to review ({', '.join(wd['key_indeterminate_routed_to_review'])}) and "
          f"{len(wd['key_indeterminate_agreed_wrong'])} were agreed wrong ({', '.join(wd['key_indeterminate_agreed_wrong'])}).", "",
          "### What the new safeguards did and did not fix", "",
          "- **Did (post-hoc, on the items they were written for):** a replay of the current code over the fresh2 cache gives `indeterminate` (routed to review) for f2l-002 and f2l-021 and `wk_surveillance` for f2l-029, the three fresh2 holding-direction errors. Original fresh2 results are unchanged and this replay is a regression check, not a fair test.",
          "- **Did not generalise on LDCT work-up:** on fresh3 the same error classes recurred on new items (a surveillance plan held as incomplete; wrong-kind or outcome-free documents held as incomplete): 3 agreed-but-wrong before and 3 after, on different items. The wrong-document rule tests only for a lung or LDCT mention plus a work-up term, so a radiology report or clinic note that mentions the screening LDCT passes it; the surveillance override needs both an MDT-outcome title and the exact phrases `interval CT`/`CT in N months`.",
          "- **Did not fix reader-B extraction:** Gemma still invented fields (`mdt=discussed` from a signature line, `tissue=benign` with no biopsy) and defaulted to `pending`; reader A still defaults `outcome=pending` when no phrase matches, which is why absence of evidence becomes `wk_incomplete` on both sides.",
          "- **Improved on IFHAS follow-up (descriptive):** B unavailable on the agreed follow-up items fell from 10 to 1 and the one fresh2 closing-direction error class (f2i-031, pending read as managed) did not recur (0 agreed-but-wrong on 23 agreed follow-up items). With 23 items this is a descriptive observation, not proof of the fix.", "",
          "## 5. Why reader B was unavailable (exact, recomputed)", "",
          f"Total `b_available=false`: {sum(u['by_kind_total'].values())} ({u['by_kind_total']}); agreed {u['by_kind_agreed']}, unadjudicated {u['by_kind_unadjudicated']}. LDCT: {u['ldct']}. Nothing is assumed from fresh2.", "",
          "| id | kind | key status | reasons |", "|---|---|---|---|"]
    for x in u["rows"]:
        extra = f" {x['implausible_coded_values']}" if x.get("implausible_coded_values") else ""
        L.append(f"| {x['id']} | {x['kind']} | {x['key_status']} | {'; '.join(x['reasons'])}{extra} |")
    L += ["", "IFHAS follow-up: the single unavailable item (f3i-003, a dental progress note, key `indeterminate`) has a cache entry with no error; its main quote is grounded but one per-finding quote (`weight`) is a paraphrase that is not contiguous in the note, so the grounding rule rejects the whole answer. The model also produced three findings for a dental note with no screening content, so routing it to a human matches what the key expects for it. IFHAS screening: f3i-013 coded `2345-7|5.4|mg/dL` (a mmol/L-scale value under a mg/dL unit) is implausible. "
          "Unavailable is not a correct answer; it routes to a human and is counted separately from B correct (with the one exception above for a key coded_label of `unavailable`).", "",
          "## 6. Fresh2 to fresh3 comparison (matching definitions only)", "", cmp_["scope"], "",
          "| measure (agreed subset) | fresh2 | fresh3 |", "|---|---|---|",
          f"| IFHAS follow-up reader B unavailable | {cmp_['fresh2']['ifhas_followup_b_unavailable_agreed']} | {cmp_['fresh3']['ifhas_followup_b_unavailable_agreed']} |",
          f"| IFHAS follow-up agreed-but-wrong | {cmp_['fresh2']['ifhas_followup_agreed_but_wrong']} | {cmp_['fresh3']['ifhas_followup_agreed_but_wrong']} |",
          f"| LDCT work-up agreed-but-wrong | {cmp_['fresh2']['ldct_workup_agreed_but_wrong']} | {cmp_['fresh3']['ldct_workup_agreed_but_wrong']} |", "",
          "Denominators differ (fresh2 29 agreed follow-up and 26 agreed work-up; fresh3 23 and 22), as do the author model and the item mix. No overall accuracy improvement is claimed, and no `0 unsafe` claim is made for either round. Reader B unavailable counts only the follow-up kind because that is where the definition matches.", "",
          "## 7. Limits", "",
          "- Model-labelled keys, not clinician-verified; agreement between two models (and the guide's literal rules) is not clinical truth. Not evidence of clinical effectiveness.",
          "- Small (100 items, 93 agreed), one run, one authoring model; item counts per kind of 22 to 24 give wide intervals.",
          "- The guide is unchanged from fresh2, including the random-glucose requirement that most fasting panels contradict.",
          "- Fresh3 is the fair test only in the sense that the readers were frozen before the corpus existed; reader hardening was informed by fresh2 errors, so fresh2 is a development set for the safeguards.",
          "- None of the 3 agreed-but-wrong answers is closing-direction, but this is a count on synthetic text, not a rate; no zero-unsafe claim is made.", "",
          "## 8. Verification", "",
          f"- `score.json` equals a fresh `compute_scores(key, reader_raw)`: **{v['score_json_matches_recomputation']}**.",
          f"- Freeze manifest re-derived from git history equals `FREEZE.json`: **{v['manifest_rederived_equal']}**.",
          f"- `check_freeze(after-score)` errors: {v['check_freeze_after_score_errors'] or 'none'}.",
          f"- Frozen/original/protected files changed since scoring commit {p['scoring_commit']}: {v['changed_since_scoring_commit'] or 'none'} ({v['files_compared']} files compared); protected reader files changed between reader_code_commit and scoring: {v['protected_changed_since_reader_code_commit'] or 'none'}.", ""]
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
        same = False
        if os.path.exists(jp) and os.path.exists(mp):
            with open(jp) as f:
                jsame = f.read() == js
            with open(mp) as f:
                same = jsame and f.read() == md
        print("analysis files up to date" if same else "analysis files differ")
        return 0 if same else 1
    for path, text in ((jp, js), (mp, md)):
        with open(path, "w", encoding="utf-8") as f:
            f.write(text)
    print("wrote", OUT_JSON, "and", OUT_MD)
    return 0


if __name__ == "__main__":
    sys.exit(main())
