"""Adversarial generator cases (pipeline/genadv.go): what happened to them in an integrated run.

usage: python3 scripts/adversarial.py <cohort dir> <run dir>   (run dir = the one holding report7.json)
Prints one JSON object: the safety block every run has, then B8 (relatives' Emirates IDs, truth wrong_eid) and B9
(wrong referral numbers, truth wrong_ref) when the cohort has them. Link questions are read from review_queue.json and
answered from reviewer_decisions.ndjson; a "cross-person" pair is two registrations of different persons in truth.
"""
import glob, json, os, sys
from collections import Counter, defaultdict

cohort, run = sys.argv[1], sys.argv[2]
tr = json.load(open(os.path.join(cohort, "truth.json")))
rep = json.load(open(os.path.join(run, "report7.json")))
qid = tr["pk_qid"]


def luhn(s):
    if len(s) != 15 or not s.isdigit():
        return False
    t = 0
    for i, c in enumerate(reversed(s)):
        d = int(c)
        if i % 2:
            d = d * 2 - 9 if d > 4 else d * 2
        t += d
    return t % 10 == 0


# registration -> Emirates ID, intake key -> registration (from the feeds the run consumed)
eid, intake_pk = {}, {}
for f in sorted(glob.glob(os.path.join(cohort, "fhir_*.ndjson"))):
    for line in open(f):
        ev = json.loads(line)
        res = ev["res"]
        for c in res.get("contained", []):
            mrn, e = None, None
            for i in c.get("identifier", []):
                if i["system"].endswith(":mrn"):
                    mrn = i["value"]
                elif "emirates-id" in i["system"]:
                    e = i["value"]
            if mrn:
                pk = ev["prov"] + "|" + mrn
                if e:
                    eid[pk] = e
                if res.get("resourceType") == "ServiceRequest":
                    intake_pk[ev["prov"] + "/ServiceRequest/" + res["id"]] = pk

# link questions and their answers
items = {it["item"]: it for it in json.load(open(os.path.join(run, "review_queue.json")))}
dec = {}
for line in open(os.path.join(run, "reviewer_decisions.ndjson")):
    d = json.loads(line)
    dec[d["item"]] = d


def basis_of(it):
    for c in it.get("chain", []):
        w = c.get("body", {}).get("weakest_edge")
        if w:
            return w.get("basis", "")
    return ""


def cross(pair):
    return len(pair) == 2 and qid.get(pair[0]) and qid.get(pair[1]) and qid[pair[0]] != qid[pair[1]]


def safety(r):
    out = {}
    for pg in ("crc", "cervical"):
        out["false_closures_" + pg] = r["closure"][pg]["wasl_live_with_review"]["false_closures"]
    for pg in ("lung_nodule", "diabetic_retinopathy", "breast"):
        b = r.get("programmes_live", {}).get(pg)
        if b and "closure" in b:
            out["false_closures_" + pg] = b["closure"]["false_closures"]
    bo = r["outreach_live"]["by_origin"].values()
    out["wrong_patient_messages"] = sum(v["wrong_released"] for v in bo)
    out["wrong_intents_pre_gate"] = sum(v["wrong_pre_gate"] for v in bo)
    ct = r["consent_timing"]
    out["released_without_granted_consent"] = ct.get("released_without_granted_consent")
    lk = r["linkage"]
    out["linkage"] = {k: lk.get(k) for k in ("pair_precision", "pair_recall", "false_merge_pairs", "impure_clusters", "cross_person_edges_by_basis")}
    out["linkage"]["eid_links"] = lk["stats"].get("eid_links")
    out["linkage"]["eid_conflicts"] = lk["stats"].get("eid_conflicts")
    ho = Counter()
    for pg, b in r.get("programmes_live", {}).items():
        for k, v in ((b.get("handoff") or {}).get("counts") or {}).items():
            ho[k] += v
    out["handoff_packets"] = dict(ho)
    rv = r["reviewer"]["by_kind"]["all"]
    out["review_items"], out["wrong_final_signoffs"] = rv["items"], rv["wrong_final_signoffs"]
    return out


res = {"safety": safety(rep)}

# all link questions on cross-person pairs, by basis and answer
lq = Counter()
for iid, it in items.items():
    if "link" in it.get("kinds", []) and cross(it.get("pair", [])):
        d = dec.get(iid, {})
        lq[(basis_of(it) or "?") + ":" + (d.get("link") or "unanswered")] += 1
res["cross_person_link_questions"] = dict(lq)

we = tr.get("wrong_eid")
if we:
    by_eid = defaultdict(set)
    for pk, e in eid.items():
        if luhn(e):
            by_eid[e].add(qid.get(pk))
    b8 = Counter()
    for pk, w in we.items():
        b8["registrations"] += 1
        b8["relation_" + w["relation"]] += 1
        if pk not in eid:
            b8["no_event_in_window"] += 1
            continue
        if len(by_eid[eid[pk]]) > 1:
            b8["collides_with_another_person_registration"] += 1
    res["b8_wrong_eid"] = dict(b8)

wr = tr.get("wrong_ref")
if wr:
    b9 = Counter()
    for k, w in wr.items():
        b9["intakes"] += 1
        b9["kind_" + w["kind"]] += 1
        if w["status"] == "revoked":
            b9["revoked"] += 1
            b9["revoked_" + w["kind"] + ("_cites_loop" if w["cited_loop"] else "")] += 1
    # questions raised from a cited reference (basis referral-ref), whether the pair is one person, and the answer
    q = []
    for iid, it in items.items():
        if basis_of(it) != "referral-ref":
            continue
        d = dec.get(iid, {})
        pair = it.get("pair", [])
        wrong_cite = any(intake_pk.get(k) in pair and wr[k]["cited"] == it["loop"] for k in wr)
        q.append({"loop": it["loop"], "pair": pair, "same_person": not cross(pair), "from_wrong_citation": wrong_cite,
                  "answer": d.get("link"), "reviewer_wrong": d.get("wrong")})
    b9["referral_ref_questions"] = len(q)
    b9["referral_ref_questions_cross_person"] = sum(1 for x in q if not x["same_person"])
    b9["referral_ref_questions_from_wrong_citation"] = sum(1 for x in q if x["from_wrong_citation"])
    for answer in ("same", "different"):
        b9["referral_ref_answer_" + answer] = sum(1 for x in q if x["answer"] == answer)
    b9["referral_ref_unanswered"] = sum(1 for x in q if x["answer"] is None)
    b9["wrong_attachments"] = sum(1 for x in q if not x["same_person"] and x["answer"] == "same")
    b9["reviewer_wrong_answers"] = sum(1 for x in q if x["reviewer_wrong"])
    res["b9_wrong_ref"] = dict(b9)
    res["b9_questions"] = [x for x in q if x["from_wrong_citation"] or not x["same_person"] or x["reviewer_wrong"]]

print(json.dumps(res, indent=1, sort_keys=True))
