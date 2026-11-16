"""Booking deadlines and windows are absolute clinical days, independent of the synthetic request day.
Collected by the existing CI step `python3 booking/tests/test_suggest_default.py` (it calls run_all() below, which
fails on a missing, renamed or failing case); `python3 booking/tests/test_clinical_dates.py` runs the same set."""
import json, os, sys, tempfile

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import recovery  # noqa: E402
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
        assert c["deadline_day"] == 131 and c["clock_anchor"] == {"kind": "inadequate_procedure", "day": 100}
        assert c.get("booking", {}).get("deadline", 131) == 131   # no feasible post-exam seat is an honest no-slot


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


def decide(decs, now=60):
    return {d["loop"]: {**d, "now": now, "sig": "sig-" + d["loop"]} for d in decs}


def waitlist_via_generate(decs, *argv, arrival=LATE_REQUEST_DAY):
    """Production booking output -> production recovery.generate (no booking.json round trip needed)."""
    res = book(decs, *argv, arrival=arrival)
    with tempfile.TemporaryDirectory() as t:
        with open(os.path.join(t, "out_log.ndjson"), "w") as f:
            for d in decide(decs).values():
                f.write(json.dumps({"data": {"dec": d}}) + "\n")
        out = recovery.generate(t, booking=res)
    return res, out, {w["case_id"]: w for w in out["waitlist"]}


def test_crc_initial_never_offered_before_referral_anchor():
    c = book([dec("crc-sr70", "crc", sr_day=70)], arrival=3)["cases"]["crc-sr70"]
    assert c["requested_day"] == 3 and c["status"] == "suggested"
    assert c["booking"]["day"] >= 70 and c["booking"]["deadline"] == 101 == c["deadline_day"]
    assert c["eligible_slots"] and all(s["day"] >= 70 and s["deadline"] == 101 for s in c["eligible_slots"])
    assert c["booking"]["within_deadline"] is True and c["suggested_slot_id"] in {s["slot_id"] for s in c["eligible_slots"]}


def test_crc_repeat_never_offered_before_exam_and_later_capacity_absent_is_no_slot():
    c = book([dec("crc-rep", "crc", "inadequate", sr_day=0, inadequate_day=100)], arrival=55)["cases"]["crc-rep"]
    assert c["deadline_day"] == 131 and c["clock_anchor"]["day"] == 100
    assert c["status"] == "no_slot_within_horizon" and c["eligible_slots"] == []   # synthetic calendar ends before day 100


def test_crc_zero_negative_and_absent_anchors_keep_request_relative_bound():
    for sr, dl in ((0, 31), (-10, 21)):
        c = book([dec("crc-x", "crc", sr_day=sr)], arrival=20)["cases"]["crc-x"]
        assert c["deadline_day"] == dl and c["booking"]["day"] >= 25
        assert c["eligible_slots"] and min(s["day"] for s in c["eligible_slots"]) >= 25
    c = book([dec("crc-x", "crc", "inadequate", sr_day=4)], arrival=20)["cases"]["crc-x"]   # no exam day: referral fallback
    assert c["clock_anchor"] == {"kind": "referral", "day": 4} and c["deadline_day"] == 35


def test_repeat_negative_and_zero_exam_anchors_remain_absolute():
    # Explicit normalized examination days are signed values; zero is not absence.
    # These are software producer controls, not clinical reader/model evidence.
    for exam in (-10, -1, 0):
        for prog in ("crc", "cervical"):
            case_id = f"{prog}-signed-exam-{exam}"
            c = book([dec(case_id, prog, "inadequate", sr_day=-20, inadequate_day=exam)], arrival=3)["cases"][case_id]
            assert c["clock_anchor"] == {"kind": "inadequate_procedure", "day": exam}
            if prog == "crc":
                assert c["deadline_day"] == exam + 31
            else:
                assert c["window_days"] == [exam + 42, exam + 84]


def test_no_slot_crc_recovery_targets_are_absolute_deadlines():
    decs = [dec("crc-i", "crc", sr_day=200, deadline=31), dec("crc-r", "crc", "inadequate", sr_day=0, inadequate_day=100, deadline=31)]
    res, out, w = waitlist_via_generate(decs, arrival=42)
    assert res["cases"]["crc-i"]["status"] == res["cases"]["crc-r"]["status"] == "no_slot_within_horizon"
    assert w["crc-i"]["deadline_day"] == 231 and w["crc-r"]["deadline_day"] == 131   # not request+31 (73 / 62)
    assert w["crc-r"]["requested_day"] == 42 and w["crc-i"]["source_decision_sig"] == "sig-crc-i"
    assert out["as_of_day"] == 60 and out["slots"] == []
    w55 = waitlist_via_generate(decs[1:], arrival=55)[2]["crc-r"]
    assert w55["deadline_day"] == 131   # not request 55 + 31 = 86


def test_no_slot_cervical_recovery_targets_use_window():
    decs = [dec("cx-i", "cervical", sr_day=300, deadline=14), dec("cx-r", "cervical", "inadequate", sr_day=0, inadequate_day=300, deadline=14)]
    res, _, w = waitlist_via_generate(decs, arrival=42)
    assert res["cases"]["cx-i"]["status"] == res["cases"]["cx-r"]["status"] == "no_slot_within_horizon"
    assert (w["cx-i"]["repeat_window_lower_bound"], w["cx-i"]["deadline_day"]) == (0, 314)   # not request+14 = 56
    assert (w["cx-r"]["repeat_window_lower_bound"], w["cx-r"]["deadline_day"]) == (342, 384)


def test_recovery_legacy_fallback_only_when_absolute_metadata_absent():
    d = dec("c", "crc", deadline=31)
    assert recovery.waitlist("c", d, {"requested_day": 42})["deadline_day"] == 73
    assert recovery.waitlist("c", dec("c", "cervical", deadline=14), {"requested_day": 42})["deadline_day"] == 56
    for absolute in (0, -5, 131):
        assert recovery.waitlist("c", d, {"requested_day": 42, "deadline_day": absolute})["deadline_day"] == absolute
    assert recovery.waitlist("c", dec("c", "cervical", deadline=14), {"requested_day": 42, "window_days": [-9, 0]})["deadline_day"] == 0


def run_all():
    """Execute every test_* case in this module; fail on a missing/renamed case or any failure. Returns the names run."""
    names = sorted(n for n, f in globals().items() if n.startswith("test_") and callable(f))
    missing = sorted(set(EXPECTED) - set(names))
    failed = []
    for n in names:
        try:
            globals()[n]()
        except Exception as e:  # noqa: BLE001 - report every failing case
            failed.append(f"{n}: {type(e).__name__}: {e}")
    if missing or failed or len(names) != len(EXPECTED):
        raise AssertionError(f"clinical-date cases: missing={missing} failed={failed} ran={len(names)}/{len(EXPECTED)}")
    return names


EXPECTED = [
    "test_crc_deadline_is_referral_plus_clock_not_request_day", "test_crc_deadline_ignores_arrival_and_shifts_with_sr_day",
    "test_crc_repeat_deadline_is_inadequate_exam_plus_31", "test_urgent_cervical_initial_window_is_referral_relative",
    "test_cervical_repeat_window_is_unsatisfactory_exam_plus_42_to_84", "test_aggregates_match_per_case_absolute_comparisons",
    "test_crc_initial_never_offered_before_referral_anchor", "test_crc_repeat_never_offered_before_exam_and_later_capacity_absent_is_no_slot",
    "test_crc_zero_negative_and_absent_anchors_keep_request_relative_bound", "test_repeat_negative_and_zero_exam_anchors_remain_absolute",
    "test_no_slot_crc_recovery_targets_are_absolute_deadlines",
    "test_no_slot_cervical_recovery_targets_use_window", "test_recovery_legacy_fallback_only_when_absolute_metadata_absent",
]


if __name__ == "__main__":
    print("clinical-date cases passed:", len(run_all()))
