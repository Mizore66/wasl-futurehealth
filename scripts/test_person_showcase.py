"""The published person journey must demonstrate an actionable cross-programme workflow (issue #161)."""
import copy
import json
import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from person_showcase import showcase_errors  # noqa: E402

PERSON = "person_" + "a" * 64
FLAG = "outreach_" + "b" * 64


def journey():
    cases = ["p/ServiceRequest/1", "p/ServiceRequest/2", "p/ServiceRequest/3"]
    return {
        "schema": "wasl-person-journey/1",
        "person": {
            "id": PERSON, "anchor_case_id": cases[0], "case_ids": cases, "programmes": ["crc", "cervical", "breast"],
            "identity_provenance": {"source": "canonical_linker_clusters", "artifact_schema": "wasl-identity-clusters/1"},
        },
        "cases": [{"case_id": c} for c in cases],
        "obligations": [{"case_id": cases[1], "status": "open"}],
        "outreach_flags": [{"id": FLAG, "actionable": True, "intents": [{"case_id": cases[0]}, {"case_id": cases[2]}]}],
        "next_action": {"kind": "review_duplicate_outreach", "case_id": cases[0], "outreach_flag_id": FLAG, "rule": "wasl-person-next-action/1"},
    }


class ShowcaseErrors(unittest.TestCase):
    def test_actionable_cross_programme_journey_passes(self):
        self.assertEqual(showcase_errors(journey()), [])

    def test_rejects_each_missing_state(self):
        mutations = {
            "programmes": lambda j: j["person"].update(programmes=["crc", "cervical"]),
            "open obligation": lambda j: j["obligations"][0].update(status="met"),
            "actionable outreach": lambda j: j["outreach_flags"][0].update(actionable=False),
            "next action": lambda j: j.update(next_action={"kind": "none", "rule": "wasl-person-next-action/1"}),
            "provenance": lambda j: j["person"]["identity_provenance"].update(source="guess"),
            "case": lambda j: j["next_action"].update(case_id="p/ServiceRequest/9"),
            "outreach flag": lambda j: j["next_action"].update(outreach_flag_id="outreach_" + "c" * 64),
            "membership": lambda j: j["cases"].pop(),
            "rule": lambda j: j["next_action"].update(rule="other"),
        }
        for word, mutate in mutations.items():
            with self.subTest(word):
                broken = copy.deepcopy(journey())
                mutate(broken)
                errors = showcase_errors(broken)
                self.assertTrue(any(word in e for e in errors), errors)

    def test_published_copies_must_describe_the_same_person(self):
        other = copy.deepcopy(journey())
        other["person"]["id"] = "person_" + "d" * 64
        self.assertTrue(any("same person" in e for e in showcase_errors(journey(), other)))


class PublishedJourney(unittest.TestCase):
    def test_committed_copies_are_one_actionable_cross_programme_person(self):
        root = pathlib.Path(__file__).resolve().parent.parent
        docs, web = (json.loads((root / p).read_text()) for p in ("docs/api-samples/person_journey.json", "web/data/api/person_journey.json"))
        self.assertEqual(showcase_errors(docs, web), [])
        self.assertEqual(docs, web)


if __name__ == "__main__":
    unittest.main()
