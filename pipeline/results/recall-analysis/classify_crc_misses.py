#!/usr/bin/env python3
"""CRC truth-closed / Wasl-not-closed loop classification for the B8 frozen evaluation (read-only, offline).

usage: python3 -I classify_crc_misses.py <b8-eval dir> <persons3.json> <out dir>

Inputs (all existing artifacts, never modified), per lane in {wrong-on, wrong-off, clean-on, clean-off-final (guard off, no injected wrong EIDs)} and seed in {17, 29}:
  work/<lane>/v2-s<seed>/cohort/truth.json            truth state per loop, per-report key label, adversarial EID registrations
  work/<lane>/v2-s<seed>/run/main/out_log.ndjson      every decision; the FINAL decision of a loop is the highest oseq
  work/<lane>/v2-s<seed>/run/main/prep_log.ndjson     parsed events with the two-reader evidence verdicts, demographics, EIDs
  work/<lane>/v2-s<seed>/run/main/review_queue.json   every review item raised (kind, reason, pair, EID-contradiction flag)
  work/<lane>/v2-s<seed>/run/main/reviewer_decisions.ndjson  every simulated sign-off with its `wrong` flag (vs truth)

Population: CRC loops with truth.state == "closed" (truth-closed) whose final Wasl state != "closed". This reproduces the
report7 counts (wrong-on: 204/777 and 190/750 missed).

Classification is a FIRST-MATCH decision list over facts read from the final state; every missed loop gets exactly one
category. A loop that matches none of the evidence-backed rules is "UNCLASSIFIED". Nothing is guessed from outcomes
that are not in the logs. Attributes (origin, sex, age, EID-guard activity, report text, prep state) are reported
separately and are NOT used to assign categories.
"""
import collections, csv, json, math, os, sys

B, PERSONS, OUT = sys.argv[1], sys.argv[2], sys.argv[3]
LANES = ["wrong-on", "wrong-off", "clean-on", "clean-off-final"]
SEEDS = [17, 29]
HORIZON = 120.0          # TEnd in gen.go
PREP_DECIDE = 15.0       # PrepDecideDays (prepquery.go)
EVIDENCE_STATES = {"inadequate", "awaiting_histology", "procedure_done"}


def luhn(s):
    if not s or len(s) != 15 or not s.isdigit():
        return False
    t = 0
    for i in range(15):
        d = int(s[14 - i])
        if i % 2 == 1:
            d = d * 2 - 9 if d * 2 > 9 else d * 2
        t += d
    return t % 10 == 0


def dummy_eid(e):
    return (not e) or len(e) != 15 or not e.startswith("784") or len(set(e[3:])) == 1 or e in {"000000000000000", "111111111111111"}


def eid_conflict(a, b):
    """Linker rule (linker.go newRecord / eidContradict): both Luhn-valid and different, or >=3 digits differ."""
    if not a or not b:
        return False
    if luhn(a) and luhn(b) and a != b:
        return True
    if len(a) == 15 and len(b) == 15 and not dummy_eid(a) and not dummy_eid(b):
        return sum(x != y for x, y in zip(a, b)) >= 3
    return False


def wilson(k, n, z=1.96):
    if n == 0:
        return (float("nan"), float("nan"))
    p = k / n
    c = (p + z * z / (2 * n)) / (1 + z * z / n)
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / (1 + z * z / n)
    return (round(c - h, 4), round(c + h, 4))


persons = {p["qid"]: p for p in json.load(open(PERSONS))}


def age_band(dob):
    a = 2026 - int(dob[:4])
    return "<40" if a < 40 else "40-59" if a < 60 else "60-79" if a < 80 else "80+"


def load(lane, seed):
    W = f"{B}/work/{lane}/v2-s{seed}"
    R = W + "/run/main"
    T = json.load(open(W + "/cohort/truth.json"))
    fin, first_pending = {}, {}
    for line in open(R + "/out_log.ndjson"):
        d = json.loads(line)["data"]
        if d["kind"] != "decision":
            continue
        x = d["dec"]
        for pid in x.get("pending") or []:
            key = (x["loop"], pid)
            if key not in first_pending:
                first_pending[key] = x["now"]
        if x["loop"] not in fin or d["oseq"] >= fin[x["loop"]][0]:
            fin[x["loop"]] = (d["oseq"], x)
    fin = {k: v[1] for k, v in fin.items()}
    col = collections.defaultdict(list)       # qid -> colonoscopy events
    path = collections.defaultdict(list)      # colonoscopy report id -> pathology events
    sr = {}                                   # loop -> SR event
    comm = collections.defaultdict(list)      # report id -> prep thread messages
    for line in open(R + "/prep_log.ndjson"):
        d = json.loads(line)["data"]
        if d.get("kind") != "event":
            continue
        e = d["ev"]
        c = e.get("cat")
        if c == "colonoscopy":
            col[T["pk_qid"].get(e["pk"])].append(e)
        elif c == "pathology":
            path[e.get("focus")].append(e)
        elif c in ("prep-query", "prep-escalation", "prep-reply"):
            comm[e["report"]].append((e["day"], c, e.get("reason")))
        elif e.get("rtype") == "ServiceRequest" and e.get("prog") == "crc":
            sr[e["provider"] + "/ServiceRequest/" + e["id"]] = e
    rq = collections.defaultdict(list)
    allitems = json.load(open(R + "/review_queue.json"))
    for i in allitems:
        rq[i["loop"]].append(i)
    rd = {}
    for line in open(R + "/reviewer_decisions.ndjson"):
        o = json.loads(line)
        rd[o["item"]] = o
    # pair-level answers: a link answer is about a registration pair, reused across loops (ReuseLinkAnswer)
    pair_ans = collections.defaultdict(list)
    for i in allitems:
        if "link" in i["kinds"] and i["item"] in rd:
            pair_ans[(tuple(sorted(i["pair"])), i.get("reason") or "")].append(rd[i["item"]])
    return dict(T=T, fin=fin, col=col, path=path, sr=sr, comm=comm, rq=rq, rd=rd, pair_ans=pair_ans, first_pending=first_pending)


def answer_of(D, item):
    """(answer record or None, via) for a review item: direct sign-off, else a twin answer on the same pair."""
    if item["item"] in D["rd"]:
        return D["rd"][item["item"]], "direct"
    if "link" in item["kinds"]:
        a = D["pair_ans"].get((tuple(sorted(item["pair"])), item.get("reason") or ""))
        if a:
            return a[0], "pair_twin"
    return None, None


def classify(D, k, t):
    """Return (category, subcategory, facts) for one missed loop."""
    fin, T = D["fin"][k], D["T"]
    S = fin["state"]
    f = {"final_state": S, "pathway": t["pathway"], "closed_day": round(t["closed_day"], 2)}
    near = [e for e in D["col"][t["qid"]] if abs(e["day"] - t["closed_day"]) < 0.01]
    if len(near) != 1:
        return "UNCLASSIFIED", "closing report not uniquely identified", f
    R = near[0]
    v = R["evv"]
    tl = T["reports"].get(R["report"], "inadequate")
    f.update(report=R["id"], uid=T["report_uid"].get(R["report"]), key_label=tl, agree=v.get("agree"), machine_label=v.get("label"),
             rule=v.get("rule") or "", prep_ungraded=bool(v.get("prep_ungraded")), prep_regex=(v.get("regex") or {}).get("prep"),
             caecum_regex=(v.get("regex") or {}).get("caecum"), report_day=round(R["day"], 2), prep_q=fin.get("prep_q") or "")
    s = D["sr"].get(k, {})
    se, re_ = s.get("eid"), R.get("eid")
    f["eid_state"] = ("both_missing" if not se and not re_ else "sr_missing" if not se else "proc_missing" if not re_ else
                      "equal" if se == re_ else "differ")
    f["eid_conflict"] = eid_conflict(se, re_)
    f["dob_exact"] = bool(s.get("dob")) and s.get("dob") == R.get("dob")
    f["sex_equal"] = s.get("sex") == R.get("sex")
    f["adversarial_eid_reg"] = (s.get("pk") in T.get("wrong_eid", {})) or (R["pk"] in T.get("wrong_eid", {}))
    items = D["rq"].get(k, [])
    f["n_items"] = len(items)
    f["eid_contra_item"] = any(b["body"].get("eid_contra") for i in items for b in i["chain"])
    ev_items = [i for i in items if "evidence" in i["kinds"] and i.get("report") == R["id"]]
    ans_ev = [(i, answer_of(D, i)[0]) for i in ev_items]
    wrong_ev = [(i, a) for i, a in ans_ev if a and a["wrong"]]
    link_ans = [(i, answer_of(D, i)[0]) for i in items if "link" in i["kinds"]]
    pend = fin.get("pending") or []
    unans = [i for i in items if i["item"] in pend and answer_of(D, i)[0] is None]
    f["unanswered_items"] = len(unans)
    f["any_wrong_signoff_on_loop"] = any((answer_of(D, i)[0] or {}).get("wrong") for i in items)

    if S in EVIDENCE_STATES:
        # ---- evidence branch: the link was trusted, the report is attached, closure was held on evidence grounds
        if S == "procedure_done" and f["prep_ungraded"] and f["prep_q"] in ("awaiting_unit", "escalated"):
            qd = [d for d, c, _ in D["comm"].get(R["report"], []) if c == "prep-query"]
            due = (min(qd) + PREP_DECIDE) if qd else (R["day"] + PREP_DECIDE + 2)
            f["prep_decision_due_day"] = round(due, 1)
            if due > HORIZON:
                return "E1_ungraded_prep_decision_not_due_by_horizon", f["prep_q"], f
            return "UNCLASSIFIED", "ungraded prep due before horizon but unresolved", f
        if wrong_ev:
            i, a = wrong_ev[0]
            sub = "day15_clinician_(assumed_25pct_error)" if i.get("reason") == "prep_ungraded_day15" else "evidence_disagreement_(4pct_error)"
            f["wrong_answer"] = a["evidence"]
            return "E2_reviewer_wrong_evidence_signoff", sub, f
        if [i for i in unans if "evidence" in i["kinds"]]:
            return "E3_evidence_review_unanswered", "", f
        if v.get("agree") and v.get("label") == "inadequate" and tl != "inadequate":
            sub = "R1_caecum_not_documented_both_readers" if "R1_" in f["rule"] else "readers_agree_caecum_not_reached_or_inadequate"
            return "E4_readers_agree_not_adequate_but_key_says_completed", sub, f
        if S == "awaiting_histology" and v.get("agree") and v.get("label") == "awaiting_histology":
            ps = D["path"].get(R["id"], [])
            f["path_events"] = len(ps)
            f["path_labels"] = "|".join(str(p["evv"].get("label")) for p in ps)
            f["path_same_registration"] = all(p["pk"] == R["pk"] for p in ps) if ps else None
            f["path_first_day"] = round(min([p["day"] for p in ps], default=-1), 2)
            if not ps:
                return "E5_pending_histology_no_pathology_event", "", f
            return "E5_histology_present_but_not_accepted", f["path_labels"], f
        hold = [b["body"]["report"].split("/")[-1] for b in fin.get("chain", []) if b["type"] == "hold_decision"]
        f["hold_report"] = hold[0] if hold else ""
        if hold and hold[0] != R["id"] and not (v.get("agree") and v.get("label") == "adequate"):
            # repeat-procedure loop: the hold shown is an EARLIER report; the closing (repeat) report is itself unresolved
            return "E6_repeat_procedure_closing_report_itself_unresolved", f"closing report agree={v.get('agree')} label={v.get('label')} prep_q={f['prep_q'] or '-'}", f
        if hold and hold[0] != R["id"] and v.get("agree") and v.get("label") == "adequate":
            # repeat procedure at another site: its report is machine-adequate but was never attached (an earlier inadequate report still holds the loop)
            return "L6_repeat_procedure_report_not_linked_earlier_inadequate_holds", "", f
        return "UNCLASSIFIED", f"evidence state {S}: no rule matched (agree={v.get('agree')}, machine={v.get('label')}, key={tl}, hold_report_is_closing={bool(hold) and hold[0] == R['id']})", f

    # ---- link branch: loop ended without a trusted procedure report attached
    if S == "referral_rejected":
        return "L0_referral_rejected_state", "late (report day %.0f)" % R["day"] if R["day"] > HORIZON - 5 else "", f
    if unans:
        sub = "second_identifier_only" if all(i.get("reason") == "second_identifier" for i in unans) else "link_question"
        f["raise_day"] = D["first_pending"].get((k, unans[0]["item"]))
        return "L1_link_review_unanswered_at_horizon", sub, f
    wl = [(i, a) for i, a in link_ans if a and a["link"] == "different" and a["wrong"]]
    if wl:
        return "L2_reviewer_wrong_link_different", "", f
    if f["eid_conflict"]:
        return "L3_eid_mismatch_cannot_link", "adversarial_relative_eid" if f["adversarial_eid_reg"] else "noise_only", f
    answered_same = [a for _, a in link_ans if a and a["link"] == "same" and not a["wrong"]]
    if f["eid_contra_item"]:
        return "L4_eid_guard_contradiction_hold", "reviewed_same" if answered_same else "unreviewed", f
    if link_ans and answered_same:
        return "L4b_reviewed_same_still_unlinked", "", f
    if link_ans:
        return "L4c_reviewed_different_correct_still_open", "", f
    return "L5_no_link_established_no_question_raised", f["eid_state"], f


CATS_ORDER = ["E1_ungraded_prep_decision_not_due_by_horizon", "E2_reviewer_wrong_evidence_signoff", "E3_evidence_review_unanswered",
              "E4_readers_agree_not_adequate_but_key_says_completed", "E5_pending_histology_no_pathology_event",
              "E5_histology_present_but_not_accepted", "E6_repeat_procedure_closing_report_itself_unresolved", "L0_referral_rejected_state", "L1_link_review_unanswered_at_horizon",
              "L2_reviewer_wrong_link_different", "L3_eid_mismatch_cannot_link", "L4_eid_guard_contradiction_hold",
              "L4b_reviewed_same_still_unlinked", "L4c_reviewed_different_correct_still_open", "L5_no_link_established_no_question_raised",
              "L6_repeat_procedure_report_not_linked_earlier_inadequate_holds", "UNCLASSIFIED"]

rows, summ, denoms = [], {}, {}
data = {}
for lane in LANES:
    for seed in SEEDS:
        data[(lane, seed)] = load(lane, seed)

for (lane, seed), D in data.items():
    T, fin = D["T"], D["fin"]
    crc = {k: t for k, t in T["loops"].items() if t["prog"] == "crc"}
    tc = [k for k, t in crc.items() if t["state"] == "closed"]
    missed = [k for k in tc if fin[k]["state"] != "closed"]
    fc = sum(1 for k, t in crc.items() if t["state"] != "closed" and fin[k]["state"] == "closed")
    # wrong-person TPs: truth-closed loops "closed" by Wasl using another person's report
    repPK = {}
    for e in (x for lst in D["col"].values() for x in lst):
        repPK[e["id"]] = e["pk"]
    wp = [k for k in tc if fin[k]["state"] == "closed" and T["pk_qid"].get(repPK.get(fin[k].get("closing_report"))) != crc[k]["qid"]]
    denoms[(lane, seed)] = dict(crc_loops=len(crc), truth_closed=len(tc), wasl_closed_tp=len(tc) - len(missed), missed=len(missed),
                                false_closures=fc, recall=round((len(tc) - len(missed)) / len(tc), 4), tp_closed_with_other_persons_report=len(wp),
                                recall_right_patient_only=round((len(tc) - len(missed) - len(wp)) / len(tc), 4))
    for k in missed:
        t = crc[k]
        cat, sub, f = classify(D, k, t)
        p = persons.get(t["qid"], {})
        rows.append(dict(lane=lane, seed=seed, loop=k, category=cat, sub=sub, origin=t["origin"], sex=p.get("sex", ""),
                         age_band=age_band(p["dob"]) if p.get("dob") else "", **f))

os.makedirs(OUT, exist_ok=True)
keys = sorted({k for r in rows for k in r})
lead = ["lane", "seed", "loop", "category", "sub", "origin", "sex", "age_band"]
with open(OUT + "/missed_loops.csv", "w", newline="") as fh:
    w = csv.DictWriter(fh, fieldnames=lead + [k for k in keys if k not in lead], lineterminator="\n")
    w.writeheader()
    for r in rows:
        w.writerow(r)

# ---- summary tables
def cat_table(lane, seeds):
    c = collections.Counter(r["category"] for r in rows if r["lane"] == lane and r["seed"] in seeds)
    n = sum(denoms[(lane, s)]["truth_closed"] for s in seeds)
    m = sum(c.values())
    return {cat: dict(n=c[cat], pct_of_missed=round(100 * c[cat] / m, 1) if m else None,
                      pct_of_truth_closed=round(100 * c[cat] / n, 2), wilson95_of_truth_closed=wilson(c[cat], n)) for cat in CATS_ORDER if c[cat]}, m, n


summary = {"denominators": {f"{l}/s{s}": v for (l, s), v in denoms.items()}, "categories": {}, "sub_categories": {}}
for lane in LANES:
    for label, seeds in (("pooled", SEEDS), ("s17", [17]), ("s29", [29])):
        tab, m, n = cat_table(lane, seeds)
        summary["categories"][f"{lane}/{label}"] = dict(missed=m, truth_closed=n, table=tab)
    sc = collections.Counter((r["category"], r["sub"]) for r in rows if r["lane"] == lane)
    summary["sub_categories"][lane] = {f"{a} :: {b}": v for (a, b), v in sorted(sc.items())}

# splits for wrong-on: recall and category by attribute (denominator = truth-closed loops with that attribute)
splits = {}
for lane in LANES:
    for attr in ("origin", "sex", "age_band"):
        tot, miss = collections.Counter(), collections.Counter()
        for seed in SEEDS:
            D = data[(lane, seed)]
            for k, t in D["T"]["loops"].items():
                if t["prog"] != "crc" or t["state"] != "closed":
                    continue
                p = persons.get(t["qid"], {})
                g = t["origin"] if attr == "origin" else p.get("sex", "?") if attr == "sex" else age_band(p["dob"]) if p.get("dob") else "?"
                tot[g] += 1
                miss[g] += D["fin"][k]["state"] != "closed"
        splits[f"{lane}/{attr}"] = {g: dict(truth_closed=tot[g], missed=miss[g], recall=round(1 - miss[g] / tot[g], 4),
                                           wilson95_recall=tuple(round(1 - x, 4) for x in wilson(miss[g], tot[g])[::-1])) for g in sorted(tot)}
summary["splits_recall"] = splits
# category x attribute for wrong-on
cx = {}
for attr in ("origin", "sex", "age_band", "seed"):
    c = collections.defaultdict(collections.Counter)
    for r in rows:
        if r["lane"] == "wrong-on":
            c[r[attr]][r["category"][:2]] += 1   # E1..E5, L0..L5, UN
    cx[attr] = {g: dict(v) for g, v in sorted(c.items(), key=lambda kv: str(kv[0]))}
summary["wrong_on_group_family_counts"] = cx

# report text concentration (wrong-on): per key-uid truth-closed vs missed
uidtab = collections.defaultdict(lambda: [0, 0, ""])
for seed in SEEDS:
    D = data[("wrong-on", seed)]
    for k, t in D["T"]["loops"].items():
        if t["prog"] != "crc" or t["state"] != "closed":
            continue
        near = [e for e in D["col"][t["qid"]] if abs(e["day"] - t["closed_day"]) < 0.01]
        R = near[0]
        u = D["T"]["report_uid"].get(R["report"])
        v = R["evv"]
        uidtab[u][0] += 1
        uidtab[u][1] += D["fin"][k]["state"] != "closed"
        uidtab[u][2] = "key=%s|agree=%s|rule=%s|ungraded=%s" % (D["T"]["reports"].get(R["report"]), v.get("agree"), v.get("rule") or "-", bool(v.get("prep_ungraded")))
summary["report_text_concentration_wrong_on"] = {u: dict(truth_closed=a, missed=b, evidence_profile=c) for u, (a, b, c) in sorted(uidtab.items(), key=lambda kv: -kv[1][1])}

# paired-lane transitions (same loops, same persons; wrong-on vs wrong-off vs clean-on)
pair = {}
for seed in SEEDS:
    Don, Doff, Dcl = (data[(l, seed)] for l in LANES[:3])
    crc = {k: t for k, t in Don["T"]["loops"].items() if t["prog"] == "crc" and t["state"] == "closed"}
    c = collections.Counter()
    ex = collections.defaultdict(list)
    for k in crc:
        on, off, cl = (D["fin"][k]["state"] == "closed" for D in (Don, Doff, Dcl))
        c[f"on={int(on)} off={int(off)} clean={int(cl)}"] += 1
        ex[f"on={int(on)} off={int(off)} clean={int(cl)}"].append(k)
    wp_off = set()
    repPK = {e["id"]: e["pk"] for lst in Doff["col"].values() for e in lst}
    for k in crc:
        if Doff["fin"][k]["state"] == "closed" and Doff["T"]["pk_qid"].get(repPK.get(Doff["fin"][k].get("closing_report"))) != crc[k]["qid"]:
            wp_off.add(k)
    guard_loss = [k for k in crc if Doff["fin"][k]["state"] == "closed" and Don["fin"][k]["state"] != "closed"]
    pair[f"s{seed}"] = dict(
        patterns=dict(sorted(c.items())),
        guard_loss_total=len(guard_loss),
        guard_loss_where_wrong_off_closed_with_other_persons_report=sum(k in wp_off for k in guard_loss),
        guard_loss_right_patient=sum(k not in wp_off for k in guard_loss),
        wrong_off_tp_with_other_persons_report=len(wp_off),
        guard_loss_wrong_on_category=dict(collections.Counter(next(r["category"] for r in rows if r["lane"] == "wrong-on" and r["loop"] == k) for k in guard_loss)),
        adversarial_eid_effect_paired_closed_in_clean_not_in_wrong_on=sum(1 for k in crc if Dcl["fin"][k]["state"] == "closed" and Don["fin"][k]["state"] != "closed"),
        closed_in_wrong_on_not_in_clean=sum(1 for k in crc if Dcl["fin"][k]["state"] != "closed" and Don["fin"][k]["state"] == "closed"))
summary["paired_lanes"] = pair

# loops missed in wrong-on: what happens in clean-on (baseline vs EID-attributable)
wo = {}
for seed in SEEDS:
    Don, Dcl, Doff = data[("wrong-on", seed)], data[("clean-on", seed)], data[("wrong-off", seed)]
    c = collections.Counter()
    for r in rows:
        if r["lane"] == "wrong-on" and r["seed"] == seed:
            cl = Dcl["fin"][r["loop"]]["state"] == "closed"
            off = Doff["fin"][r["loop"]]["state"] == "closed"
            c[(r["category"], "clean_closed" if cl else "clean_also_missed", "off_closed" if off else "off_also_missed")] += 1
    wo[f"s{seed}"] = {f"{a} | {b} | {d}": v for (a, b, d), v in sorted(c.items())}
summary["wrong_on_missed_vs_other_lanes"] = wo
l5 = collections.defaultdict(collections.Counter)
for r in rows:
    if r["lane"] == "wrong-on" and r["category"].startswith("L5"):
        l5["eid_state"][r["eid_state"]] += 1
        l5["dob_exact_match"][r["dob_exact"]] += 1
        l5["script_sr_vs_proc"][f"{r.get('pathway','')[:0]}{r['report_day'] and ''}"] += 0
        l5["report_day_bucket"]["<90" if float(r["report_day"]) < 90 else "90-110" if float(r["report_day"]) < 110 else ">=110"] += 1
        l5["origin"][r["origin"]] += 1
summary["wrong_on_L5_descriptors_not_causes"] = {a: {str(k): v for k, v in c.items() if v} for a, c in l5.items() if a != "script_sr_vs_proc"}
tm = collections.defaultdict(collections.Counter)
for r in rows:
    if r["lane"] == "wrong-on":
        tm[r["category"]]["<90" if float(r["report_day"]) < 90 else "90-110" if float(r["report_day"]) < 110 else ">=110"] += 1
summary["wrong_on_report_day_by_category"] = {a: dict(c) for a, c in tm.items()}

# ---- exposure by closing-report evidence profile (corpus / prep state), all lanes
def profile(R, key):
    v = R["evv"]; rx = v.get("regex") or {}
    if v.get("prep_ungraded"):
        return "ungraded_prep_readers_disagree(caecum_yes,prep_not_stated)"
    if v.get("agree"):
        return f"readers_agree:{v.get('label')}" + ("(R1_caecum_not_documented)" if "R1_" in (v.get("rule") or "") else "")
    return f"readers_disagree_other(key={key})"
expo = {}
for lane in LANES:
    c = collections.defaultdict(lambda: [0, 0])
    for seed in SEEDS:
        D = data[(lane, seed)]
        for k, t in D["T"]["loops"].items():
            if t["prog"] != "crc" or t["state"] != "closed":
                continue
            R = [e for e in D["col"][t["qid"]] if abs(e["day"] - t["closed_day"]) < 0.01][0]
            pr = profile(R, D["T"]["reports"].get(R["report"]))
            c[pr][0] += 1
            c[pr][1] += D["fin"][k]["state"] == "closed"
    expo[lane] = {a: dict(truth_closed=n, wasl_closed=m, recall=round(m / n, 4)) for a, (n, m) in sorted(c.items())}
summary["exposure_by_closing_report_profile"] = expo

# ---- DOB relation between referral registration and procedure registration (wrong-on, all truth-closed): where link failures sit
def dobrel(a, b):
    if not a or not b: return "missing"
    if a == b: return "exact"
    if a[:4] == b[:4]: return "same_year"
    if abs(int(a[:4]) - int(b[:4])) <= 10: return "year_shift_le10"
    return "far"
dr = {}
for lane in LANES:
    c = collections.defaultdict(collections.Counter)
    for seed in SEEDS:
        D = data[(lane, seed)]
        L5 = {r["loop"] for r in rows if r["lane"] == lane and r["seed"] == seed and r["category"].startswith("L5")}
        for k, t in D["T"]["loops"].items():
            if t["prog"] != "crc" or t["state"] != "closed":
                continue
            R = [e for e in D["col"][t["qid"]] if abs(e["day"] - t["closed_day"]) < 0.01][0]
            rel = dobrel(D["sr"][k].get("dob"), R.get("dob"))
            st = "linked_or_other_cause" if D["fin"][k]["state"] == "closed" or k not in L5 else "L5"
            c[rel]["truth_closed"] += 1
            c[rel]["closed"] += D["fin"][k]["state"] == "closed"
            c[rel]["L5_no_link"] += k in L5
    dr[lane] = {a: dict(v, L5_share_of_truth_closed=round(v["L5_no_link"] / v["truth_closed"], 4)) for a, v in sorted(c.items())}
summary["L5_by_dob_relation_referral_vs_procedure_registration"] = dr

# ---- reviewer answers by purpose (wrong-on, all items answered; `wrong` = disagrees with simulated truth key)
rv = collections.defaultdict(lambda: [0, 0])
for seed in SEEDS:
    D = data[("wrong-on", seed)]
    purp = {i["item"]: (i.get("reason") or "") for l in D["rq"].values() for i in l}
    for item, o in D["rd"].items():
        key = ("evidence:" if "evidence" in o["kinds"] else "link:") + (purp.get(item) or "plain")
        rv[key][0] += 1
        rv[key][1] += bool(o["wrong"])
summary["wrong_on_signoff_wrong_rate_by_purpose_SIMULATED"] = {a: dict(answered=n, wrong=w, wrong_rate=round(w / n, 4)) for a, (n, w) in sorted(rv.items())}

# ---- right-patient accounting: a truth-closed loop that Wasl "closed" using ANOTHER person's report counts as a TP in
# report7 recall (state == closed) but is a wrong-evidence closure. Separate it before attributing the guard's recall cost.
rp = {}
for seed in SEEDS:
    Don, Doff = data[("wrong-on", seed)], data[("wrong-off", seed)]
    crc = {k: t for k, t in Don["T"]["loops"].items() if t["prog"] == "crc" and t["state"] == "closed"}
    repPK = {e["id"]: e["pk"] for lst in Doff["col"].values() for e in lst}
    def right(D, k):
        d = D["fin"][k]
        return d["state"] == "closed" and D["T"]["pk_qid"].get(repPK.get(d.get("closing_report"))) == crc[k]["qid"]
    on_closed = {k for k in crc if Don["fin"][k]["state"] == "closed"}
    off_closed = {k for k in crc if Doff["fin"][k]["state"] == "closed"}
    off_right = {k for k in crc if right(Doff, k)}
    on_right = {k for k in crc if right(Don, k)}
    rp[f"s{seed}"] = dict(
        truth_closed=len(crc), on_closed=len(on_closed), off_closed=len(off_closed), headline_gap_loops=len(off_closed) - len(on_closed),
        off_closed_with_other_persons_report=len(off_closed - off_right), on_closed_with_other_persons_report=len(on_closed - on_right),
        right_patient_closed_on=len(on_right), right_patient_closed_off=len(off_right),
        net_right_patient_gap_loops=len(off_right) - len(on_right),
        gross_loss_off_right_but_on_not_closed=len(off_right - on_closed), gross_gain_on_closed_but_off_not_right=len(on_closed - off_right))
summary["guard_gap_decomposition"] = rp
summary["guard_gap_decomposition"]["pooled"] = {k: sum(rp[f"s{s}"][k] for s in SEEDS) for k in rp["s17"]}

# ---- recall by truth closing day (horizon / timing)
rt = {}
for lane in LANES:
    c = collections.defaultdict(lambda: [0, 0])
    for seed in SEEDS:
        D = data[(lane, seed)]
        for k, t in D["T"]["loops"].items():
            if t["prog"] != "crc" or t["state"] != "closed":
                continue
            b = "<60" if t["closed_day"] < 60 else "60-89" if t["closed_day"] < 90 else "90-104" if t["closed_day"] < 105 else "105-120"
            c[b][0] += 1
            c[b][1] += D["fin"][k]["state"] == "closed"
    rt[lane] = {a: dict(truth_closed=n, wasl_closed=m, recall=round(m / n, 4)) for a, (n, m) in sorted(c.items())}
summary["recall_by_truth_closing_day"] = rt

# ---- ungraded-prep cross-cut: share of each wrong-on category whose closing report has ungraded prep
ug = collections.defaultdict(lambda: [0, 0])
for r in rows:
    if r["lane"] == "wrong-on":
        ug[r["category"]][0] += 1
        ug[r["category"]][1] += str(r["prep_ungraded"]) == "True"
summary["wrong_on_ungraded_prep_share_by_category"] = {a: dict(missed=n, closing_report_prep_ungraded=m) for a, (n, m) in sorted(ug.items())}
tot_un = sum(m for n, m in ug.values())
summary["wrong_on_ungraded_prep_total"] = dict(missed_with_ungraded_prep_closing_report=tot_un, truth_closed_with_ungraded_prep=expo["wrong-on"].get("ungraded_prep_readers_disagree(caecum_yes,prep_not_stated)", {}).get("truth_closed"))

# ---- text concentration for E4 / R1
e4 = collections.Counter(r["uid"] for r in rows if r["lane"] == "wrong-on" and r["category"].startswith("E4"))
summary["wrong_on_E4_by_report_text"] = dict(e4)

# ---- day-15 ungraded-prep clinician decision on truth-closed CRC loops (wrong-on). The 25% per-read error (ErrUngraded) is an
# ASSUMED stress rate in pipeline/prepquery.go, not a clinician error rate and not the 4% general reviewer error.
d15 = collections.Counter()
for seed in SEEDS:
    D = data[("wrong-on", seed)]
    for k, t in D["T"]["loops"].items():
        if t["prog"] != "crc" or t["state"] != "closed":
            continue
        its = [i for i in D["rq"].get(k, []) if i.get("reason") == "prep_ungraded_day15"]
        if not its:
            continue
        ans = [D["rd"].get(i["item"]) for i in its]
        for a in ans:
            if a is None:
                d15["raised_unanswered"] += 1
                continue
            d15["answered"] += 1
            d15["wrong"] += bool(a["wrong"])
            d15["final_closed_after_" + ("wrong" if a["wrong"] else "right")] += D["fin"][k]["state"] == "closed"
            d15["final_not_closed_after_" + ("wrong" if a["wrong"] else "right")] += D["fin"][k]["state"] != "closed"
summary["wrong_on_day15_decisions_on_truth_closed_loops"] = dict(d15)
# ---- L5 detail
l5d = collections.Counter()
for r in rows:
    if r["lane"] == "wrong-on" and r["category"].startswith("L5"):
        l5d[(r["final_state"], r["n_items"])] += 1
summary["wrong_on_L5_final_state_and_n_review_items"] = {f"{a} | items={b}": v for (a, b), v in sorted(l5d.items())}
l5ident = {}
for lane in LANES:
    l5ident[lane] = sorted(r["loop"] + "#" + str(r["seed"]) for r in rows if r["lane"] == lane and r["category"].startswith("L5"))
summary["L5_identical_loop_set_across_lanes"] = dict(wrong_on_eq_wrong_off=l5ident["wrong-on"] == l5ident["wrong-off"], wrong_on_eq_clean_on=l5ident["wrong-on"] == l5ident["clean-on"], n=len(l5ident["wrong-on"]))

# ---- L5 descriptors (what the unlinked pair looks like; NOT causes): EID typo distance, which side is Luhn-valid, name similarity
import difflib
def ham(a, b):
    return sum(x != y for x, y in zip(a, b)) if a and b and len(a) == len(b) else None
l5x = collections.Counter()
for seed in SEEDS:
    D = data[("wrong-on", seed)]
    miss = {r["loop"] for r in rows if r["lane"] == "wrong-on" and r["seed"] == seed and r["category"].startswith("L5")}
    for k in miss:
        t = D["T"]["loops"][k]
        sre = D["sr"][k]
        R = [e for e in D["col"][t["qid"]] if abs(e["day"] - t["closed_day"]) < 0.01][0]
        a, b = sre.get("eid"), R.get("eid")
        if a and b and a != b:
            h = ham(a, b)
            l5x[f"eid_differ hamming={h if h is not None and h < 3 else '>=3'} luhn_valid(sr,proc)=({int(luhn(a))},{int(luhn(b))})"] += 1
        na, nb = (sre.get("name") or ""), (R.get("name") or "")
        arabic = lambda x: any("\u0600" <= c <= "\u06ff" for c in x)
        l5x["name scripts: sr=%s proc=%s" % ("ar" if arabic(na) else "lat", "ar" if arabic(nb) else "lat")] += 1
summary["wrong_on_L5_pair_descriptors_not_causes"] = dict(sorted(l5x.items()))

# ---- EID-guard activity on truth-closed CRC loops (an eid_contra review item exists on the loop), per lane
ga = {}
for lane in LANES:
    c = collections.Counter()
    for seed in SEEDS:
        D = data[(lane, seed)]
        for k, t in D["T"]["loops"].items():
            if t["prog"] != "crc" or t["state"] != "closed":
                continue
            act = any(b["body"].get("eid_contra") for i in D["rq"].get(k, []) for b in i["chain"])
            adv = any(pk in D["T"].get("wrong_eid", {}) for pk in {D["sr"][k].get("pk")})
            key = ("guard_item" if act else "no_guard_item")
            c[key + "/truth_closed"] += 1
            c[key + "/wasl_closed"] += D["fin"][k]["state"] == "closed"
    ga[lane] = dict(c)
summary["eid_guard_activity_on_truth_closed_crc_loops"] = ga

# ---- secondary flags (NOT used for assignment) so overlaps are visible: category x flag, wrong-on
sec = collections.defaultdict(collections.Counter)
for r in rows:
    if r["lane"] == "wrong-on":
        c = r["category"]
        sec[c]["n"] += 1
        sec[c]["ungraded_prep"] += str(r["prep_ungraded"]) == "True"
        sec[c]["report_day>=105"] += float(r["report_day"]) >= 105
        sec[c]["any_wrong_signoff_on_loop"] += str(r["any_wrong_signoff_on_loop"]) == "True"
        sec[c]["eid_conflict_between_registrations"] += str(r["eid_conflict"]) == "True"
summary["wrong_on_secondary_flags_by_category"] = {a: dict(v) for a, v in sorted(sec.items())}

# ---- L5 mutually exclusive sub-split (descriptive; precedence order shown)
def l5sub(D, k, t):
    s = D["sr"][k]
    R = [e for e in D["col"][t["qid"]] if abs(e["day"] - t["closed_day"]) < 0.01][0]
    a, b = s.get("eid"), R.get("eid")
    if a and b and a != b and sum(x != y for x, y in zip(a, b)) <= 2 and len(a) == len(b):
        return "1_EID_differs_by_<=2_digits (typo-like)"
    da, db = s.get("dob"), R.get("dob")
    if da and da == db:
        return "2_DOB_exact_match (name/translit only)"
    if not da or not db:
        return "3_DOB_missing_on_a_side"
    if da[:4] == db[:4]:
        return "4_DOB_same_year_day/month_differs"
    if abs(int(da[:4]) - int(db[:4])) <= 10:
        return "5_DOB_year_off_<=10"
    return "6_DOB_year_far_(>10)"
l5s = collections.defaultdict(collections.Counter)
for seed in SEEDS:
    D = data[("wrong-on", seed)]
    for r in rows:
        if r["lane"] == "wrong-on" and r["seed"] == seed and r["category"].startswith("L5"):
            l5s["pooled"][l5sub(D, r["loop"], D["T"]["loops"][r["loop"]])] += 1
            l5s[f"s{seed}"][l5sub(D, r["loop"], D["T"]["loops"][r["loop"]])] += 1
summary["wrong_on_L5_exclusive_subsplit"] = {a: dict(sorted(v.items())) for a, v in l5s.items()}
json.dump(summary, open(OUT + "/summary.json", "w"), indent=1, default=str)
print(json.dumps(summary["denominators"], indent=1))
for lane in LANES:
    t = summary["categories"][f"{lane}/pooled"]
    print("\n==", lane, "pooled missed", t["missed"], "of truth-closed", t["truth_closed"])
    for c, v in t["table"].items():
        print(f"  {c:62s} {v['n']:4d}  {v['pct_of_missed']:5.1f}% of missed  {v['pct_of_truth_closed']:5.2f}% of truth-closed")
