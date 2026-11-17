"""A single-programme sample person journey widened into an actionable cross-programme showcase, for snapshot tests."""
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def showcase_person():
    """The single-programme no-action journey (the evidence_closure person, kept as web test data) widened to three trusted
    programmes with an open obligation and an actionable group, so the fake API can serve its anchor case."""
    person = json.loads((ROOT / "web/tests/fixtures/person-journey-no-action.json").read_text())
    first = person["cases"][0]
    ids = [first["case_id"], "provider-b/ServiceRequest/sr-2", "provider-c/ServiceRequest/sr-3"]
    for case_id, programme in zip(ids[1:], ("cervical", "breast")):
        extra = json.loads(json.dumps(first))
        extra["case_id"] = case_id
        extra["detail"]["case"].update(id=case_id, programme=programme)
        person["cases"].append(extra)
    person["person"].update(case_ids=ids, programmes=["crc", "cervical", "breast"])
    person["obligations"] = [{"case_id": ids[1], "programme": "cervical", "kind": "programme_deadline",
                              "due_day": 90, "status": "open", "source_field": "deadline_day"}]
    outreach = {"kind": "appointment_reminder", "ev": "ev-1", "basis": "p_cal", "p_cal": 0.99, "released": True}
    intents = [{"case_id": case_id, "programme": programme, "first_decision_id": "d-" + str(i),
                "latest_decision_id": "d-" + str(i), "latest_decision_sig": "s" * 64, "lifecycle": "released",
                "outreach": outreach} for i, (case_id, programme) in enumerate(zip(ids, ("crc", "cervical")))]
    flag = "outreach_" + "e" * 64
    person["outreach_flags"] = [{"id": flag, "match": "duplicate", "actionable": True, "intents": intents}]
    person["next_action"] = {"kind": "review_duplicate_outreach", "case_id": ids[0], "outreach_flag_id": flag,
                             "priority": "routine", "overdue": False, "reason_codes": ["duplicate_outreach"],
                             "rule": "wasl-person-next-action/1"}
    return person
