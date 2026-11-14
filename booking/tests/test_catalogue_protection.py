"""Female-list protection must hold in the reservable-slot catalogue, not only in engine assignment."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from booking.model import Prefs, NoShowModel
from booking.calendar import Calendar
from booking.engine import BookingEngine
from suggest import _eligible_slots


def _setup():
    cal = Calendar([5, 20, 20], ["U0", "U1", "U2"], ["AD"] * 3, [4, 4, 4], [True, True, False], [True] * 3, [False] * 3, [False] * 3)
    eng = BookingEngine(cal, NoShowModel(), eps=0.0, prep_lead=3, horizon=60, protect=4.0)
    for _ in range(10):
        eng._hard_arr.append((0, "AD"))
    return eng, cal


def _ids(eng, cal, prefs, day=0):
    return {s["slot_id"] for s in _eligible_slots(eng, cal, "colonoscopy", prefs, day, day + 31, day + 5, 3)}


PROTECTED = {f"sim:colonoscopy:1:{seat}" for seat in range(1, 5)}


def test_protected_seats_are_not_offered_to_ordinary_or_soft_patients():
    eng, cal = _setup()
    assert eng._held(0, [0, 1, 2]) == {1: 4}
    for pref in ("none", "soft"):
        assert not PROTECTED & _ids(eng, cal, Prefs(region="AD", female_endoscopist=pref))


def test_protected_seats_are_offered_to_hard_preference_patients():
    eng, cal = _setup()
    assert PROTECTED <= _ids(eng, cal, Prefs(region="AD", female_endoscopist="hard"))


def test_protected_seats_release_at_the_seven_day_boundary():
    eng, cal = _setup()
    ordinary = Prefs(region="AD")
    assert not PROTECTED & _ids(eng, cal, ordinary, day=12)
    assert PROTECTED <= _ids(eng, cal, ordinary, day=13)


def test_catalogue_generation_does_not_consume_engine_demand_state():
    eng, cal = _setup()
    before = list(eng._hard_arr)
    _ids(eng, cal, Prefs(region="AD"), day=500)
    assert eng._hard_arr == before
