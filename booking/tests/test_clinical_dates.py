"""Booking deadlines and windows are absolute clinical days, independent of the synthetic request day.
Run: python -m pytest booking/tests/test_clinical_dates.py"""
import os, sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import suggest  # noqa: E402

LATE_REQUEST_DAY = 42


def dec(loop, prog, state="open", sr_day=0, **extra):
    return {"loop": loop, "prog": prog, "state": state, "prov": "ahs-mushrif", "sr_day": sr_day, **extra}


def book(decs, *argv, arrival=LATE_REQUEST_DAY):
    last = {d["loop"]: d for d in decs}
    orig = suggest.arrival_day
    suggest.arrival_day = (lambda k, a: arrival) if arrival is not None else orig
    try:
        return suggest.main(["run", "--slots-per-fit", "60", *argv], preloaded=(last, {}), write=False, quiet=True)
    finally:
        suggest.arrival_day = orig


def test_crc_deadline_is_referral_plus_clock_not_request_day():
    res = book([dec("crc-late", "crc", sr_day=0)])
    c = res["cases"]["crc-late"]
    assert c["requested_day"] == LATE_REQUEST_DAY
    assert c["status"] == "suggested" and c["booking"]["deadline"] == 31
    assert c["booking"]["day"] > 31 and c["booking"]["within_deadline"] is False
    assert res["summary"]["within_31d"] == 0
    assert all(not s["within_deadline"] and s["deadline"] == 31 for s in c["eligible_slots"])


def test_crc_deadline_ignores_arrival_and_shifts_with_sr_day():
    base = book([dec("crc-a", "crc", sr_day=0)], arrival=10)["cases"]["crc-a"]["booking"]["deadline"]
    other = book([dec("crc-a", "crc", sr_day=0)], arrival=50)["cases"]["crc-a"]["booking"]["deadline"]
    shifted = book([dec("crc-a", "crc", sr_day=17)], arrival=10)["cases"]["crc-a"]["booking"]["deadline"]
    assert base == other == 31 and shifted == 31 + 17


def test_crc_repeat_deadline_is_inadequate_exam_plus_31():
    for arrival in (3, 55):
        c = book([dec("crc-rep", "crc", "inadequate", sr_day=0, inadequate_day=100)], arrival=arrival)["cases"]["crc-rep"]
        assert c["booking"]["deadline"] == 131 and c["clock_anchor"] == {"kind": "inadequate_procedure", "day": 100}


def test_urgent_cervical_initial_window_is_referral_relative():
    c = book([dec("cx-urgent", "cervical", sr_day=0, deadline=14)])["cases"]["cx-urgent"]
    assert c["window_days"] == [0, 14]
    assert c["status"] == "suggested" and c["booking"]["day"] > 14 and c["within_window"] is False


def test_cervical_repeat_window_is_unsatisfactory_exam_plus_42_to_84():
    for arrival in (3, 55):
        c = book([dec("cx-rep", "cervical", "inadequate", sr_day=0, inadequate_day=100)], arrival=arrival)["cases"]["cx-rep"]
        assert c["window_days"] == [142, 184] and c["repeat_after_unsatisfactory"] is True


def test_aggregates_match_per_case_absolute_comparisons():
    decs = [dec(f"crc-{i}", "crc", sr_day=i * 4) for i in range(30)] + \
           [dec(f"cx-{i}", "cervical", sr_day=i * 3, deadline=[14, 28, 56][i % 3]) for i in range(12)]
    for argv in ([], ["--slots-per-fit", "0.3"]):
        res = book(decs, *argv, arrival=None)
        crc = [c for c in res["cases"].values() if c["procedure"] == "colonoscopy"]
        cerv = [c for c in res["cases"].values() if c["procedure"] == "colposcopy"]
        assert res["summary"]["within_31d"] == sum(1 for c in crc if c["status"] == "suggested" and c["booking"]["day"] <= c["deadline_day"])
        assert res["cervical"]["within_window"] == sum(1 for c in cerv if c["status"] == "suggested" and c["window_days"][0] <= c["booking"]["day"] <= c["window_days"][1])
        assert all(c["deadline_day"] == c["clock_anchor"]["day"] + 31 for c in crc)
    assert any(c["status"] == "no_slot_within_horizon" for c in res["cases"].values())
