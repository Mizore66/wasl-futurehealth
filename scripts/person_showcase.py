"""Check that a published person journey demonstrates an actionable cross-programme workflow (issue #161).

Usage: python3 person_showcase.py <person_journey.json> [<second copy that must describe the same person>]
"""
import json
import sys

NEXT_RULE = "wasl-person-next-action/1"


def showcase_errors(journey, other=None):
    person = journey.get("person", {})
    case_ids = person.get("case_ids", [])
    members = set(case_ids)
    errors = []
    if len(set(person.get("programmes", []))) < 3:
        errors.append("fewer than three programmes")
    if person.get("identity_provenance", {}).get("source") != "canonical_linker_clusters":
        errors.append("provenance is not the canonical trusted clusters")
    current = [c.get("case_id") for c in journey.get("cases", [])]
    retained = {r.get("case_id") for r in journey.get("retained_references", [])}
    if len(current) < 2 or current != case_ids[: len(current)] or members != set(current) | retained or \
            person.get("anchor_case_id") != case_ids[0]:
        errors.append("case membership is inconsistent")
    obligations = journey.get("obligations", [])
    if not any(o.get("status") in ("open", "overdue") and o.get("case_id") in members for o in obligations):
        errors.append("no open obligation on a member case")
    flags = {f.get("id"): f for f in journey.get("outreach_flags", [])}
    if not any(f.get("actionable") is True and all(i.get("case_id") in members for i in f.get("intents", [])) for f in flags.values()):
        errors.append("no actionable outreach group")
    action = journey.get("next_action", {})
    if action.get("rule") != NEXT_RULE:
        errors.append("next action rule is not " + NEXT_RULE)
    if action.get("kind") in (None, "none"):
        errors.append("no populated next action")
    elif action.get("case_id") not in members:
        errors.append("next action case is not a member case")
    if "outreach_flag_id" in action and action["outreach_flag_id"] not in flags:
        errors.append("next action outreach flag is not in the journey")
    if other is not None and other.get("person", {}).get("id") != person.get("id"):
        errors.append("copies do not describe the same person")
    return errors


def main(paths):
    docs = [json.load(open(p, encoding="utf-8")) for p in paths]
    errors = showcase_errors(docs[0], docs[1] if len(docs) > 1 else None)
    for error in errors:
        print(f"person showcase: {error}", file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
