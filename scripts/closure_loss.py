"""Closure-safety loss carried by a scored artifact (pipeline/analyze.go closureLossVersion).

v2 counts a predicted closure as unsafe when its outcome is false or its closing evidence is not proven to belong to
the loop's person (foreign or unverified ownership). v1 artifacts predate that check and carry outcome-only
false closures, so they are labelled rather than silently read as ownership-checked.
"""
V2 = "outcome_or_evidence_ownership_v2"
V1 = "outcome_only_v1"


def one(counts):
    if "unsafe_closures" in counts:
        return counts["unsafe_closures"], V2
    return counts["false_closures"], V1


def pooled(rows):
    scored = [one(r) for r in rows]
    versions = {v for _, v in scored}
    if len(versions) > 1:
        raise ValueError("closure-loss definitions differ across artifacts: %s" % sorted(versions))
    return sum(k for k, _ in scored), versions.pop() if versions else V2
