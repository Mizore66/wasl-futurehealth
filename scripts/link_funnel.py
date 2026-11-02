#!/usr/bin/env python3
"""Offline six-category CRC link funnel classifier (diagnostics only; truth is offline, never published)."""
import collections, json, os, sys

CATEGORIES = {
    "candidate_not_generated", "candidate_below_review",
    "candidate_rejected_demographic", "candidate_routed_review",
    "linked_report_not_attached", "attached_or_closed",
}
REASONS = {"eid_conflict", "eid_contradiction", "merge_constraint"}
# fixed, identifier-free analysis error codes (the only strings an analysis_error row may carry)
ERROR_CODES = {
    "near_without_review_item", "link_review_without_same_or_different", "unclassified_stage",
    "sr_event_missing", "closing_report_missing", "closing_report_ambiguous",
    "final_decision_missing", "trace_config_missing", "trace_config_conflict", "analysis_failure",
}


def classify_pair(events, items, answers, attached, closed, clustered=False, split_after=None):
    if attached:
        return "attached_or_closed", "report_attached"
    if closed:
        return "attached_or_closed", "loop_closed"
    if clustered:  # latest cluster snapshot holds both registrations (direct or transitive link)
        return "linked_report_not_attached", "eligible_cluster_without_report"
    if split_after is not None:
        # the latest cluster snapshot excludes the report after the last pair-level link: a stale "linked" cannot stand
        events = [e for e in events if e.get("_pos", -1) > split_after and e.get("stage") != "linked"]
        if not events:
            return "candidate_routed_review", "answered_different"
    latest = events[-1] if events else None
    stage = latest.get("stage") if latest else None
    if stage == "linked":
        return "linked_report_not_attached", "eligible_cluster_without_report"
    if stage == "unlinked":  # a reviewer's cannot-link split the pair
        return "candidate_routed_review", "answered_different"
    if items:
        answered = [(a.get("t", 0), n, a) for n, a in
                    ((n, answers[i["item"]]) for n, i in enumerate(items) if i["item"] in answers)]
        if not answered:
            return "candidate_routed_review", "unanswered"
        last = sorted(answered, key=lambda x: (x[0], x[1]))[-1][2]
        if last.get("link") == "same":
            return "candidate_routed_review", "answered_same"
        if last.get("link") == "different":
            return "candidate_routed_review", "answered_different"
        raise ValueError("link_review_without_same_or_different")
    if latest is None:
        return "candidate_not_generated", "absent_from_all_retrieval_paths"
    if stage == "below_review":
        return "candidate_below_review", "below_review_floor"
    if stage == "rejected_demographic":
        r = latest.get("reason")
        return "candidate_rejected_demographic", r if r in REASONS else "unspecified_constraint"
    if stage == "near":
        raise ValueError("near_without_review_item")
    raise ValueError("unclassified_stage")


def public_summary(rows, truth_closed):
    for r in rows:
        if r["category"] != "analysis_error" and r["category"] not in CATEGORIES:
            raise ValueError("unknown_category")
    pub = [r for r in rows if r["category"] != "analysis_error"]
    errors = len(rows) - len(pub)
    categories = collections.Counter(r["category"] for r in pub)
    details = collections.Counter((r["category"], r["detail"]) for r in pub)
    # missed = not attached-or-closed, analysis errors included; this is a link attach rate, not CRC closure recall
    missed = sum(n for c, n in categories.items() if c != "attached_or_closed") + errors
    return {
        "categories": dict(sorted(categories.items())),
        "details": {a + "|" + b: n for (a, b), n in sorted(details.items())},
        "analysis_errors": errors,
        "truth_closed": truth_closed,
        "missed": missed,
        "link_attach_rate": round((truth_closed - missed) / truth_closed, 6) if truth_closed and not errors else None,
    }


def read_ndjson(path):
    out = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            if line.strip():
                out.append(json.loads(line))
    return out


def pairkey(a, b):
    return tuple(sorted([a, b]))


def ordered(records):
    """Dedupe exact records, then sort by (seq default 0, file order); each kept record gets its stream position _pos."""
    seen, out = set(), []
    for n, r in enumerate(records):
        k = json.dumps(r, sort_keys=True)
        if k not in seen:
            seen.add(k)
            out.append((r.get("seq", 0), n, r))
    out.sort(key=lambda x: (x[0], x[1]))
    return [dict(r, _pos=i) for i, (_, _, r) in enumerate(out)]


def load(run, cohort):
    with open(cohort + "/truth.json", encoding="utf-8") as f:
        truth = json.load(f)
    fin = {}
    for rec in read_ndjson(run + "/out_log.ndjson"):
        d = rec.get("data") or {}
        if d.get("kind") != "decision":
            continue
        dec = d.get("dec") or {}
        lp = dec.get("loop")
        if lp is not None and (lp not in fin or d.get("oseq", 0) >= fin[lp][0]):
            fin[lp] = (d.get("oseq", 0), dec)
    sr, col = {}, collections.defaultdict(list)
    for rec in read_ndjson(run + "/prep_log.ndjson"):
        d = rec.get("data") or {}
        if d.get("kind") != "event":
            continue
        e = d.get("ev") or {}
        if e.get("cat") == "colonoscopy":
            col[(truth.get("pk_qid") or {}).get(e.get("pk"))].append(e)
        elif e.get("rtype") == "ServiceRequest" and e.get("prog") == "crc":
            sr[str(e.get("provider")) + "/ServiceRequest/" + str(e.get("id"))] = e
    with open(run + "/review_queue.json", encoding="utf-8") as f:
        queue = json.load(f)
    rd = {}
    for o in read_ndjson(run + "/reviewer_decisions.ndjson"):  # repeated records for one item: latest t wins, then file order
        if o["item"] not in rd or o.get("t", 0) >= rd[o["item"]].get("t", 0):
            rd[o["item"]] = o
    trace = read_ndjson(run + "/link_funnel.ndjson")
    configs = []
    for c in trace:
        if c.get("kind") == "config" and c.get("config") not in configs:
            configs.append(c.get("config"))
    pairs = collections.defaultdict(list)
    members = {}
    for r in ordered([t for t in trace if t.get("kind") in ("pair", "cluster")]):  # one stream so snapshot/pair order is comparable
        if r["kind"] == "pair":
            pairs[pairkey(r.get("a"), r.get("b"))].append(r)
        else:
            s = frozenset(r.get("members") or [])
            for m in s:
                members[m] = (r["_pos"], s)
    return dict(truth=truth, fin=fin, sr=sr, col=col, queue=queue, rd=rd, configs=configs, pairs=pairs, members=members)


def link_items(queue):
    """Actual identity-link questions only: second-identifier messaging items never count."""
    return [i for i in queue if "link" in (i.get("kinds") or []) and not i.get("reason")]


def classify_loop(D, loop, t):
    sre = D["sr"].get(loop)
    if sre is None:
        raise ValueError("sr_event_missing")
    near = [e for e in D["col"].get(t.get("qid"), []) if abs(e.get("day", 1e9) - t["closed_day"]) < 0.01]
    if not near:
        raise ValueError("closing_report_missing")
    if len(near) != 1:
        raise ValueError("closing_report_ambiguous")
    rep = near[0]
    expected = pairkey(sre["pk"], rep["pk"])
    fin = D["fin"].get(loop)
    if fin is None:
        raise ValueError("final_decision_missing")
    fin = fin[1]
    closed = fin.get("state") == "closed"
    want = str(rep.get("provider")) + "/DiagnosticReport/" + str(rep.get("id"))
    attached = any((b.get("body") or {}).get("report") == want for b in fin.get("chain") or [])
    all_items = link_items(D["queue"])
    pair_ans = {}  # latest answer per pair by reviewer t, ties to the later queue item
    for n, i in enumerate(all_items):
        a = D["rd"].get(i["item"])
        if a is not None:
            k = pairkey(*i["pair"])
            if k not in pair_ans or (a.get("t", 0), n) >= pair_ans[k][0]:
                pair_ans[k] = ((a.get("t", 0), n), a)
    items = [i for i in all_items if i.get("loop") == loop or pairkey(*i["pair"]) == expected]
    answers = {}
    for i in items:
        a = D["rd"].get(i["item"]) or (pair_ans.get(pairkey(*i["pair"])) or (None, None))[1]
        if a is not None:
            answers[i["item"]] = a
    events = D["pairs"].get(expected, [])
    if events and events[-1].get("stage") in ("below_review", "rejected_demographic") \
            and not any(pairkey(*i["pair"]) == expected for i in items):
        items = []  # an explicit terminal trace for the expected pair is not relabelled by another pair's item on the loop
        answers = {}
    snap = D["members"].get(sre["pk"])
    ms = snap[1] if snap else None
    clustered = bool(ms) and rep["pk"] in ms
    split_after = None
    if ms and not clustered:
        linked = [e["_pos"] for e in events if e.get("stage") == "linked"]
        if linked and max(linked) < snap[0]:
            split_after = snap[0]
    return classify_pair(events, items, answers, attached, closed, clustered, split_after)


def analyse(D):
    truth_loops = (D["truth"].get("loops") or {})
    loops = sorted(k for k, v in truth_loops.items() if v.get("prog") == "crc" and v.get("state") == "closed")
    cfg_err = None
    if not D["configs"]:
        cfg_err = "trace_config_missing"
    elif len(D["configs"]) > 1:
        cfg_err = "trace_config_conflict"
    rows = []
    for lp in loops:
        if cfg_err:
            rows.append({"category": "analysis_error", "detail": cfg_err})
            continue
        try:
            c, d = classify_loop(D, lp, truth_loops[lp])
        except ValueError as e:
            m = str(e)
            c, d = "analysis_error", m if m in ERROR_CODES else "analysis_failure"
        except (KeyError, TypeError, AttributeError, IndexError):
            c, d = "analysis_error", "analysis_failure"
        rows.append({"category": c, "detail": d})
    return rows, len(loops), (None if cfg_err else D["configs"][0])


def main(argv):
    if len(argv) != 3:
        print("usage: link_funnel.py RUN_DIR COHORT_DIR OUT_JSON", file=sys.stderr)
        return 2
    run, cohort, out = argv
    try:  # never leave output from an earlier run behind a failed analysis
        os.remove(out)
    except FileNotFoundError:
        pass
    except OSError:
        print("input_unreadable", file=sys.stderr)
        return 2
    try:  # a linker trace-write failure means the trace is incomplete: never publish funnel evidence from it
        sp = os.path.join(run, "status-linker.json")
        if os.path.exists(sp):
            with open(sp, encoding="utf-8") as f:
                st = json.load(f)
            if not isinstance(st, dict):
                raise ValueError("status")
            if st.get("link_funnel_error"):
                print("link_funnel_write_error", file=sys.stderr)
                return 2
    except (OSError, ValueError):
        print("input_unreadable", file=sys.stderr)
        return 2
    try:
        D = load(run, cohort)
    except (OSError, ValueError, KeyError, TypeError, AttributeError):
        print("input_unreadable", file=sys.stderr)
        return 2
    rows, n, cfg = analyse(D)
    summary = public_summary(rows, n)
    summary["config"] = cfg
    summary["scope"] = {"person_split": "dev", "noise": "v2", "reviewer_error": 0, "day15_ungraded_prep_reviewer_error": 0}
    with open(out, "w", encoding="utf-8") as f:
        json.dump(summary, f, indent=1, sort_keys=True)
        f.write("\n")
    return 2 if summary["analysis_errors"] else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
