#!/usr/bin/env python3
"""Compare run directories for the agentshadow off-vs-on check (scripts/agent_shadow_parity.sh, docs/AGENT-SHADOW.md).

    agent_shadow_parity.py RUN_A RUN_B [RUN_C ...]    first run is the reference; prints one JSON report; exit 1 on any diff

Artifacts compared: out_log.ndjson, fhir_log.ndjson, prep_log.ndjson, reviewer_decisions.ndjson, and the outreach
artifacts (every Outreach intent carried by a decision, plus the per-loop final decision). Excluded because they are
timing or log metadata, never content: stream sequence numbers, Ts, Wsi-Ots and other publish-time headers, pub_ts,
proc, lat_ms, the prep `ots` field, and the reviewer's wall-clock `t`/`ms`.

Two levels are reported:
  strict       sorted, normalised bytes of every record (order within a stream is also compared per subject when the
               stream's order is deterministic: out.p<k> is, fhir/prep interleaving across concurrent producers is not)
  final_state  the last decision per loop (highest oseq) and the outreach intents, which are what a patient or the
               API can observe
"""
import hashlib, json, sys

TIMING_HDR = {"Wsi-Ots"}
TIMING_OUT = {"pub_ts", "proc", "lat_ms"}


def load(path):
    try:
        with open(path, encoding="utf-8") as f:
            return [json.loads(l) for l in f if l.strip()]
    except FileNotFoundError:
        return None


def canon(v):
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def sha(items):
    return hashlib.sha256("\n".join(items).encode()).hexdigest()[:16]


def norm_log(rows, kind):
    out = []
    for m in rows:
        hdr = {k: v for k, v in (m.get("hdr") or {}).items() if k not in TIMING_HDR}
        d = m["data"]
        if kind == "out":
            d = {k: v for k, v in d.items() if k not in TIMING_OUT}
        elif kind == "prep":
            d = {k: v for k, v in d.items() if k != "ots"}
        out.append((m.get("subj", ""), canon(hdr), canon(d)))
    return out


def summarize(run):
    s = {}
    fh, pr, ou = (load(f"{run}/{n}_log.ndjson") for n in ("fhir", "prep", "out"))
    rv = load(f"{run}/reviewer_decisions.ndjson")
    s["counts"] = {"fhir": len(fh or []), "prep": len(pr or []), "out": len(ou or []), "reviewer_decisions": len(rv or [])}
    s["fhir_ordered"] = sha([canon(x) for x in norm_log(fh or [], "fhir")])
    s["prep_by_seq"] = sha([canon(norm_log([m], "prep")[0]) for m in sorted(pr or [], key=lambda m: m["data"]["seq"])])
    s["prep_arrival_order"] = sha([canon(x) for x in norm_log(pr or [], "prep")])  # informational: prep workers race
    s["fhir_sorted"] = sha(sorted(canon(x) for x in norm_log(fh or [], "fhir")))
    s["prep_sorted"] = sha(sorted(canon(x) for x in norm_log(pr or [], "prep")))
    s["out_sorted"] = sha(sorted(canon(x) for x in norm_log(ou or [], "out")))
    by_subj = {}
    for x in norm_log(ou or [], "out"):
        by_subj.setdefault(x[0], []).append(canon(x))
    s["out_ordered_per_subject"] = {k: sha(v) for k, v in sorted(by_subj.items())}
    s["reviewer_sorted"] = sha(sorted(canon({k: v for k, v in r.items() if k not in ("t", "ms")}) for r in (rv or [])))
    seq, final, outreach = {}, {}, []
    for m in ou or []:
        d = m["data"]
        dec = d.get("dec")
        if not dec:
            continue
        seq.setdefault(dec["loop"], []).append(canon({k: v for k, v in dec.items()}))
        if d.get("oseq", 0) >= final.get(dec["loop"], (0, None))[0]:
            final[dec["loop"]] = (d.get("oseq", 0), canon({k: v for k, v in dec.items()}))
        for o in dec.get("outreach") or []:
            outreach.append(canon(o))
    s["per_loop_decision_sequence"] = sha(sorted(f"{k}|{sha(v)}" for k, v in seq.items()))
    s["final_decisions"] = sha(sorted(f"{k}|{v[1]}" for k, v in final.items()))
    s["final_decision_count"] = len(final)
    s["outreach_intents_sorted"] = sha(sorted(outreach))
    s["outreach_intents_count"] = len(outreach)
    return s


# STRICT is the byte-equivalence claim: exact record order of every stream (out per subject), per-loop decision order.
# The *_sorted keys are reported as a weaker, clearly labelled secondary and never count towards strict_equal.
STRICT = ("counts", "fhir_ordered", "prep_by_seq", "out_ordered_per_subject", "per_loop_decision_sequence", "reviewer_sorted")
SECONDARY = ("prep_arrival_order", "fhir_sorted", "prep_sorted", "out_sorted")
FINAL = ("final_decisions", "final_decision_count", "outreach_intents_sorted", "outreach_intents_count", "reviewer_sorted")


def main(argv):
    runs = argv[1:]
    if len(runs) < 2:
        print(__doc__)
        return 2
    sums = [summarize(r) for r in runs]
    ref = sums[0]
    report = {"reference": runs[0], "comparisons": []}
    bad = 0
    for r, s in zip(runs[1:], sums[1:]):
        strict = {k: ref[k] == s[k] for k in STRICT}
        secondary = {k: ref[k] == s[k] for k in SECONDARY}
        final = {k: ref[k] == s[k] for k in FINAL}
        report["comparisons"].append({"run": r, "strict_equal": all(strict.values()), "final_state_equal": all(final.values()),
                                      "strict": strict, "secondary_sorted_only": secondary, "final_state": final, "counts_ref": ref["counts"], "counts_run": s["counts"]})
        bad += (not all(final.values()))
    print(json.dumps(report, indent=1, sort_keys=True))
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
