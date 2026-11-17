#!/usr/bin/env python3
"""Fresh4 protocol amendment 2: ownership-aware I1 closure gate (see AMENDMENT-2.md).

The original I1 gate (fresh4.py FALSE_CLOSURE_METRICS / PROTOCOL.md section 6) is not edited. This module evaluates the
original gates unchanged and adds the amended I1 gate, which additionally requires zero CRC and cervical
`unsafe_closures` (closure_loss outcome_or_evidence_ownership_v2: a false outcome, or closing evidence owned by another
person or not mappable to one; #193). An aggregate scored with the outcome-only loss cannot satisfy the amended gate:
it is reported `not_evaluable`, never `pass`.
Standard library only.
"""
import argparse
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.join(HERE, ".."))
import fresh4 as X  # noqa: E402
import closure_loss  # noqa: E402

AMENDMENT = {
    "id": "fresh4-amendment-2",
    "version": 2,
    "document": "scripts/fresh4_eval/AMENDMENT-2.md",
    "approved_by": "Anas (protocol owner; repository owner Mizore66)",
    "approved_on": "2026-11-17",
    "amends": "PROTOCOL.md section 6, integrated gate I1",
    "issues": [193, 218],
}
GATE_SCHEMA = "fresh4-integrated-gates/amendment-2"
UNSAFE_CLOSURE_METRICS = ["crc_unsafe_closures", "cervical_unsafe_closures"]
AMENDED_I1_METRICS = X.FALSE_CLOSURE_METRICS + UNSAFE_CLOSURE_METRICS
ORIGINAL_I1 = "I1_%s_zero_listed_false_closures"
AMENDED_I1 = "I1a2_%s_zero_listed_false_or_unsafe_closures"


def amended_i1(noise, block):
    """The amended I1 gate for one noise model's aggregate block (`summary["by_noise"][noise]`)."""
    safety = block.get("safety_pooled", {})
    gate = X._zero_gate(AMENDED_I1 % noise, noise, safety, AMENDED_I1_METRICS)
    gate["protocol_version"] = AMENDMENT["id"]
    gate["closure_loss"] = block.get("closure_loss") or closure_loss.V1
    if gate["closure_loss"] != closure_loss.V2:
        # Outcome-only evidence: unsafe counts were never computed, so this is unknown rather than a measured failure.
        # A measured failure of any outcome-only metric still fails the amended gate.
        if {f["metric"] for f in gate["failures"]} == set(UNSAFE_CLOSURE_METRICS) and \
                all(f.get("error") == "missing" for f in gate["failures"]):
            gate["status"] = "not_evaluable"
            gate["reason"] = ("aggregate closure_loss is %s (outcome-only); ownership-checked unsafe closures were not "
                              "computed, so the amended gate cannot pass on this evidence" % gate["closure_loss"])
    return gate


def amended_integrated_gates(summary):
    """Original fresh4 gates, unchanged and labelled, with the amended I1 gate after each original I1."""
    gates = []
    for g in X.integrated_gates(summary):
        g = dict(g, protocol_version="fresh4-original")
        gates.append(g)
        if summary is not None and g["id"] == ORIGINAL_I1 % g.get("noise"):
            amended = amended_i1(g["noise"], summary.get("by_noise", {}).get(g["noise"], {}))
            gates.append(amended)
    if summary is None:
        gates.append({"id": AMENDED_I1 % "all", "status": "not_run", "protocol_version": AMENDMENT["id"]})
    return gates


def passed(gates):
    return all(g["status"] == "pass" for g in gates)


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("summary", help="final_aggregate summary.json")
    ap.add_argument("--out", required=True, help="new gate JSON (refuses to overwrite)")
    a = ap.parse_args(argv)
    try:
        summary, _ = X.F.read_json(a.summary)
        gates = amended_integrated_gates(summary)
        os.makedirs(os.path.dirname(os.path.abspath(a.out)), exist_ok=True)
        with open(a.out, "x", encoding="utf-8") as f:  # gate evidence is written once, never overwritten
            f.write(X.F.dumps({"schema": GATE_SCHEMA, "amendment": AMENDMENT, "summary": a.summary, "gates": gates}))
    except (X.F.HarnessError, OSError, ValueError) as e:
        print(str(e), file=sys.stderr)
        return 1
    print(X.F.dumps(gates), end="")
    return 0 if passed(gates) else 1


if __name__ == "__main__":
    sys.exit(main())
