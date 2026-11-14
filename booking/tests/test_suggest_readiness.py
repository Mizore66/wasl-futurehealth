"""Clinical readiness: unresolved referral or repeat dispositions never produce a reservable suggestion (issue #151).
Run: python tests/test_suggest_readiness.py"""
import contextlib, io, json, os, sys, tempfile

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import suggest  # noqa: E402

CASES = {
    "open-crc": ("crc", "open", None),
    "inadequate-crc-confirmed": ("crc", "inadequate", None),
    "inadequate-crc-pending": ("crc", "inadequate", ["rv-1"]),
    "rejected-crc": ("crc", "referral_rejected", None),
    "open-crc-pending-link": ("crc", "open", ["rv-2"]),
    "inadequate-cervical-confirmed": ("cervical", "inadequate", None),
    "inadequate-cervical-pending": ("cervical", "inadequate", ["rv-3"]),
    "rejected-cervical": ("cervical", "referral_rejected", None),
}
BLOCKED = {"inadequate-crc-pending", "rejected-crc", "inadequate-cervical-pending", "rejected-cervical"}


def booking(d):
    with open(os.path.join(d, "out_log.ndjson"), "w") as o, open(os.path.join(d, "fhir_log.ndjson"), "w") as f:
        for name, (prog, state, pending) in CASES.items():
            loop = f"ahs-mushrif/ServiceRequest/{name}"
            dec = {"loop": loop, "prog": prog, "state": state, "prov": "ahs-mushrif", "deadline": 56}
            if pending:
                dec["pending"] = pending
            o.write(json.dumps({"data": {"dec": dec}}) + "\n")
    with contextlib.redirect_stdout(io.StringIO()):
        return suggest.main([d, "--slots-per-fit", "10"], write=False, quiet=True)


def test_unresolved_dispositions_are_not_reservable():
    with tempfile.TemporaryDirectory() as d:
        res = booking(d)
    for name in CASES:
        case = res["cases"]["ahs-mushrif/ServiceRequest/" + name]
        if name in BLOCKED:
            assert case["status"] == "not_applicable" and not case.get("reservable") and not case.get("eligible_slots"), (name, case)
            assert case["reason"].startswith("clinical_readiness:"), case
        else:
            assert case["status"] == "suggested" and case["reservable"] is True, (name, case)
    assert res["capacity"]["open_cases_needing_slot"] == 3
    assert res["cervical"]["cases"] == 1


if __name__ == "__main__":
    test_unresolved_dispositions_are_not_reservable()
    print("ok")
