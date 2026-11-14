"""Female-list protection must hold in the reservable-slot catalogue, not only in engine assignment, and must follow the
CURRENT canonical clock (decision `now`) rather than staying frozen at the original request day.
Run: python booking/tests/test_catalogue_protection.py   (also executed by python booking/tests/test_suggest_default.py)"""
import copy
import os
import sys
import tempfile
import unittest
from types import SimpleNamespace

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
sys.path.insert(0, HERE)
from booking.model import Prefs, NoShowModel
from booking.calendar import Calendar
from booking.engine import BookingEngine
import suggest
from suggest import _eligible_slots
from test_suggest_default import synthetic_run


def _setup():
    cal = Calendar([5, 20, 20], ["U0", "U1", "U2"], ["AD"] * 3, [4, 4, 4], [True, True, False], [True] * 3, [False] * 3, [False] * 3)
    eng = BookingEngine(cal, NoShowModel(), eps=0.0, prep_lead=3, horizon=60, protect=4.0)
    for _ in range(10):
        eng._hard_arr.append((0, "AD"))
    return eng, cal


def _ids(eng, cal, prefs, day=0, current_day=None):
    return {s["slot_id"] for s in _eligible_slots(eng, cal, "colonoscopy", prefs, day, day + 31, day + 5, 3, current_day)}


PROTECTED = {f"sim:colonoscopy:1:{seat}" for seat in range(1, 5)}


def _catalogue(last, sr, d):
    return suggest.main([d], preloaded=(last, sr), write=False, quiet=True)["cases"]


def _slot_ids(case):
    return {s["slot_id"] for s in case["eligible_slots"]}


class CatalogueProtection(unittest.TestCase):
    def test_protected_seats_are_not_offered_to_ordinary_or_soft_patients(self):
        eng, cal = _setup()
        self.assertEqual(eng._held(0, [0, 1, 2]), {1: 4})
        for pref in ("none", "soft"):
            self.assertFalse(PROTECTED & _ids(eng, cal, Prefs(region="AD", female_endoscopist=pref)))

    def test_protected_seats_are_offered_to_hard_preference_patients(self):
        eng, cal = _setup()
        self.assertLessEqual(PROTECTED, _ids(eng, cal, Prefs(region="AD", female_endoscopist="hard")))

    def test_protected_seats_release_at_the_seven_day_boundary(self):
        eng, cal = _setup()
        ordinary = Prefs(region="AD")
        self.assertFalse(PROTECTED & _ids(eng, cal, ordinary, day=12))
        self.assertLessEqual(PROTECTED, _ids(eng, cal, ordinary, day=13))

    def test_catalogue_generation_does_not_consume_engine_demand_state(self):
        eng, cal = _setup()
        before = list(eng._hard_arr)
        _ids(eng, cal, Prefs(region="AD"), day=500)
        self.assertEqual(eng._hard_arr, before)
        _ids(eng, cal, Prefs(region="AD"), day=0, current_day=500)
        self.assertEqual(eng._hard_arr, before)

    def test_same_original_request_releases_on_the_current_clock(self):
        # One original request on day 0; only the CURRENT clock moves. Held day-20 seats release when 20 <= now + 7.
        eng, cal = _setup()
        ordinary = Prefs(region="AD")
        self.assertFalse(PROTECTED & _ids(eng, cal, ordinary, day=0))                    # no current clock: request-as-of
        self.assertFalse(PROTECTED & _ids(eng, cal, ordinary, day=0, current_day=12))    # one day before the boundary
        self.assertLessEqual(PROTECTED, _ids(eng, cal, ordinary, day=0, current_day=13))  # boundary: released
        self.assertFalse(PROTECTED & _ids(eng, cal, ordinary, day=0, current_day=-5))    # clock never precedes the request
        hard = Prefs(region="AD", female_endoscopist="hard")
        self.assertEqual(_ids(eng, cal, hard, day=0), _ids(eng, cal, hard, day=0, current_day=13))   # hard unaffected

    def test_producer_uses_the_canonical_decision_clock(self):
        with tempfile.TemporaryDirectory() as t:
            d = os.path.join(t, "golden-run")
            os.makedirs(d)
            synthetic_run(d)
            last, sr = suggest.load(d)
            base = _catalogue(last, sr, d)
            ordinary = [k for k, c in base.items() if c["procedure"] == "colonoscopy" and c["constraints"]["female_endoscopist"] != "hard"]
            hard = [k for k, c in base.items() if c["procedure"] == "colonoscopy" and c["constraints"]["female_endoscopist"] == "hard"]
            self.assertTrue(ordinary and hard)

            def at(offset):
                moved = copy.deepcopy(last)
                for k, dec in moved.items():
                    dec["now"] = suggest.arrival_day(k, SimpleNamespace(arrivals="uniform")) + offset
                return _catalogue(moved, sr, d)

            same_day = at(0)          # current clock == request day: identical to request-as-of
            later = at(13)            # same original requests, clock advanced past the release boundary
            for k in base:
                self.assertEqual(_slot_ids(same_day[k]), _slot_ids(base[k]), k)
            for k in hard:            # hard preferences keep every seat regardless of clock
                self.assertEqual(_slot_ids(later[k]), _slot_ids(base[k]), k)
            released = [k for k in ordinary if _slot_ids(later[k]) - _slot_ids(base[k])]
            self.assertTrue(released, "no ordinary case gained protected seats on the current clock")
            for k in ordinary:        # release only adds seats; nothing already offered is removed
                self.assertLessEqual(_slot_ids(base[k]), _slot_ids(later[k]), k)
            for k in base:            # the original request, constraints and suggestion are untouched
                self.assertEqual(later[k]["requested_day"], base[k]["requested_day"])
                self.assertEqual(later[k].get("booking"), base[k].get("booking"))


EXPECTED_TESTS = {
    "test_protected_seats_are_not_offered_to_ordinary_or_soft_patients",
    "test_protected_seats_are_offered_to_hard_preference_patients",
    "test_protected_seats_release_at_the_seven_day_boundary",
    "test_catalogue_generation_does_not_consume_engine_demand_state",
    "test_same_original_request_releases_on_the_current_clock",
    "test_producer_uses_the_canonical_decision_clock",
}


def run_named():
    """Discover and execute every protection test by name; used by test_suggest_default.py (the CI entry point)."""
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(CatalogueProtection)
    found = {t.id().rsplit(".", 1)[1] for t in suite}
    assert found == EXPECTED_TESTS, found ^ EXPECTED_TESTS
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    assert result.wasSuccessful() and result.testsRun == len(EXPECTED_TESTS), result
    return f"{result.testsRun} catalogue protection tests executed by name"


if __name__ == "__main__":
    unittest.main()
