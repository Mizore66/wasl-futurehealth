#!/usr/bin/env python3
"""Focused tests for the independent double-review smoke (wasl-independent-review/1).

The stateful fake in test_api_smoke.py enforces the protocol; each test here breaks one rule in the fake and requires
the real-HTTP smoke client to notice. No model calls; synthetic data only."""
import hashlib
import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import test_api_smoke as t  # noqa: E402

QUESTION_ROLE = {"rv-fake-link": "reviewer", "rv-fake-evidence": "clinician"}


class FakeProtocolTests(unittest.TestCase):
    """The stateful fake itself must implement the design's state table."""

    def setUp(self):
        self.fake = t.Fake()
        self.cid = t.RAISED["link"]
        self.item = "rv-fake-link"

    def post(self, token, body, cid=None):
        role, actor = t.PRINCIPALS[token]
        return self.fake.route("POST", f"/api/cases/{cid or self.cid}/review", {}, body, role, actor)

    def body(self, decision, read, head):
        return {"item": self.item, "decision": decision, "note": "", "read": read, "expected_head": head}

    def test_distinct_initial_actors_blinding_and_agreement(self):
        code, r1 = self.post("reviewer-token-1234567890", self.body("same", "r1", ""))
        self.assertEqual(code, 201)
        self.assertEqual(r1["review_state"]["state"], "awaiting_r2")
        head = r1["review_state"]["head"]
        self.assertEqual(self.post("reviewer-token-1234567890", self.body("same", "r2", head))[0], 403)
        self.assertEqual(self.post("reviewer-2-token-1234567890", self.body("same", "r2", "0" * 64))[0], 409)
        self.assertEqual(self.post("reviewer-2-token-1234567890", self.body("same", "r2", "not-a-head"))[0], 400)
        code, view = self.fake.route("GET", f"/api/cases/{self.cid}", {}, None, "reviewer", "smoke-reviewer-2")
        text = json.dumps(view)
        self.assertNotIn(r1["review"]["hash"], text)
        code, r2 = self.post("reviewer-2-token-1234567890", self.body("same", "r2", head))
        self.assertEqual((code, r2["review_state"]["state"]), (201, "resolved_agreed"))
        self.assertEqual(self.post("reviewer-3-token-1234567890", self.body("same", "r3", r2["review_state"]["head"]))[0], 409)

    def test_normalised_same_actor_is_refused_across_tokens(self):
        code, r1 = self.post("reviewer-token-1234567890", self.body("same", "r1", ""))
        t.PRINCIPALS["alias-token-1234567890"] = ("reviewer", "  SMOKE-REVIEWER-1 ")
        self.addCleanup(t.PRINCIPALS.pop, "alias-token-1234567890")
        code, _ = self.post("alias-token-1234567890", self.body("same", "r2", r1["review_state"]["head"]))
        self.assertEqual(code, 403)

    def test_senior_resolution_links_both_reads_and_needs_a_third_authorised_actor(self):
        _, r1 = self.post("reviewer-token-1234567890", self.body("same", "r1", ""))
        _, r2 = self.post("reviewer-2-token-1234567890", self.body("different", "r2", r1["review_state"]["head"]))
        head = r2["review_state"]["head"]
        self.assertEqual(r2["review_state"]["state"], "awaiting_r3")
        self.assertEqual(self.post("reviewer-token-1234567890", self.body("same", "r3", head))[0], 403)
        self.assertEqual(self.post("clinician-3-token-1234567890", self.body("same", "r3", head))[0], 403)
        self.assertEqual(self.post("reviewer-3-token-1234567890", self.body("same", "r3", "0" * 64))[0], 409)
        code, r3 = self.post("reviewer-3-token-1234567890", self.body("same", "r3", head))
        self.assertEqual(code, 201)
        self.assertEqual(r3["review"]["resolution_reads"], [r1["review"]["hash"], r2["review"]["hash"]])
        self.assertEqual(r3["review_state"]["state"], "resolved_senior")
        self.assertEqual(self.post("reviewer-3-token-1234567890", self.body("same", "r3", r3["review_state"]["head"]))[0], 409)

    def test_undo_of_protocol_read_is_409(self):
        _, r1 = self.post("reviewer-token-1234567890", self.body("same", "r1", ""))
        code, _ = self.fake.route("POST", f"/api/cases/{self.cid}/undo", {}, {"target": r1["review"]["hash"]},
                                  "reviewer", "smoke-reviewer-2")
        self.assertEqual(code, 409)

    def test_legacy_request_never_advances_protocol_state(self):
        code, _ = self.post("reviewer-token-1234567890", {"item": self.item, "decision": "same", "note": ""})
        self.assertEqual(code, 201)
        self.assertEqual(self.fake.pstate(self.cid, self.item, "link")[0], "awaiting_r1")


class IndependentSmokeDetectsViolations(unittest.TestCase):
    def assertFails(self, mutate, needle):
        r = t.run_smoke(t.Fake(mutate))
        self.assertNotEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn(needle, r.stdout + r.stderr)

    def test_consistent_fake_passes_independent_smoke(self):
        r = t.run_smoke(t.Fake())
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("independent double review", r.stdout)

    def test_missing_raised_items_fail(self):
        def m(f):
            f.raised = {}
        self.assertFails(m, "raised unsigned item")

    def test_r1_leak_to_second_reader_is_caught(self):
        def m(f):
            f.visible_events = lambda cid, actor: f.case_events(cid)
        self.assertFails(m, "leaked")

    def test_same_actor_second_read_is_caught(self):
        def m(f):
            orig = f.protocol_review
            def lax(cid, body, role, actor):
                if body.get("read") == "r2":
                    actor = actor + "-other"
                return orig(cid, body, role, actor)
            f.protocol_review = lax
        self.assertFails(m, "expected 403")

    def test_r3_after_agreement_is_caught(self):
        def m(f):
            orig = f.pstate
            f.pstate = lambda cid, item, q: ("awaiting_r3", orig(cid, item, q)[1]) if orig(cid, item, q)[0] == "resolved_agreed" else orig(cid, item, q)
        self.assertFails(m, "agreement did not resolve")

    def test_ordinary_role_r3_is_caught(self):
        def m(f):
            orig = f.protocol_review
            def lax(cid, body, role, actor):
                if body.get("read") == "r3":
                    role = "senior_" + QUESTION_ROLE[body["item"]]  # drop the role check for r3
                return orig(cid, body, role, actor)
            f.protocol_review = lax
        self.assertFails(m, "expected 403")

    def test_stale_head_acceptance_is_caught(self):
        def m(f):
            orig = f.protocol_review
            def lax(cid, body, role, actor):  # accepts a well-formed but stale head by substituting the real one
                if body.get("expected_head") == "0" * 64:
                    item = body["item"]
                    body = dict(body, expected_head=f.shown_head(cid, item, f.raised[cid][item]))
                return orig(cid, body, role, actor)
            f.protocol_review = lax
        self.assertFails(m, "expected 409")

    def test_audit_actor_or_note_leak_is_caught(self):
        def m(f):
            orig = f.live_audit
            f.live_audit = lambda cid: dict(orig(cid), history=f.case_events(cid))
        self.assertFails(m, "exposes actor or note")

    def test_undo_of_protocol_read_must_be_409(self):
        def m(f):
            orig = f.route
            def lax(method, path, query, body=None, role="", actor=""):
                if path.endswith("/undo") and method == "POST" and body and body.get("target") and \
                        any(e.get("protocol") and e["hash"] == body["target"] for e in f.events):
                    return 201, {"event": {}}
                return orig(method, path, query, body, role, actor)
            f.route = lax
        self.assertFails(m, "undo of a concealed r1")

    def test_canonical_change_is_caught(self):
        def m(f):
            orig = f.live_detail
            def changed(cid):
                d = orig(cid)
                if cid in f.raised and any(k[0] == cid for k in f.protocol):
                    d["current"]["state"] = "mutation"
                return d
            f.live_detail = changed
        self.assertFails(m, "canonical case or decisions changed")

    def test_lost_resolution_links_in_audit_are_caught(self):
        def m(f):
            orig = f.live_audit
            def stripped(cid):
                doc = orig(cid)
                doc["history"] = [{k: v for k, v in e.items() if k != "resolution_reads"} for e in doc["history"]]
                return doc
            f.live_audit = stripped
        self.assertFails(m, "resolution links")


if __name__ == "__main__":
    unittest.main()
