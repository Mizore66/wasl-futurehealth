#!/usr/bin/env python3
import base64
import copy
import json
import unittest

from redacted_snapshot import audit_snapshot, detail_snapshot


class RedactedSnapshotTest(unittest.TestCase):
    def setUp(self):
        self.export = {
            "schema": "wasl-case-export/1",
            "redacted": True,
            "meta": {"api": "wasl-api/1", "data_mode": "simulated", "note": "safe synthetic note"},
            "detail": {
                "case": {"id": "case-1"},
                "identities": [{"provider": "p", "role": "referral"}],
                "review_items": [{"item": "i1", "chain": []}],
                "signoffs": [{"item": "i1"}],
                "api_reviews": [{"case": "case-1"}],
                "agent_investigations": [{"citations": [{"tool": "get_record", "field": "dob"}]}],
                "agent_provenance": {"cached": True, "experimental": True, "model": "m", "schema_ver": "s"},
            },
            "audit": {"case": "case-1", "history": [{"case": "case-1"}], "chain": []},
        }

    def test_detail_uses_only_redacted_export_and_adds_empty_defaults(self):
        source = copy.deepcopy(self.export)
        detail = detail_snapshot(source)
        self.assertEqual(source, self.export, "input export was mutated")
        self.assertEqual(detail["meta"], self.export["meta"])
        self.assertEqual(detail["identities"][0]["registration"], "")
        self.assertEqual(detail["review_items"][0]["quote"], "")
        self.assertEqual(detail["signoffs"][0]["verdict"], "")
        self.assertEqual(detail["api_reviews"][0]["actor"], "")
        self.assertEqual(detail["api_reviews"][0]["note"], "")
        self.assertEqual(detail["agent_investigations"][0]["citations"][0]["arg"], "")
        self.assertEqual(detail["agent_investigations"][0]["citations"][0]["value"], "")
        self.assertEqual(detail["agent_provenance"]["prompt_ver"], "")

    def test_audit_adds_only_empty_identity_defaults(self):
        audit = audit_snapshot(self.export)
        self.assertEqual(audit["meta"], self.export["meta"])
        self.assertEqual(audit["history"][0]["actor"], "")
        self.assertEqual(audit["history"][0]["note"], "")
        self.assertNotIn("CANARY", json.dumps(audit))

    def test_raw_escaped_nested_and_encoded_canaries_fail_closed(self):
        prose = "CANARY MULTIWORD private clinical text"
        encoded = [
            prose,
            "CANARY-ESCAPED-JSON\\nTEXT",
            [{"nested": prose}],
            base64.b64encode(prose.encode()).decode(),
            base64.urlsafe_b64encode(prose.encode()).decode(),
            base64.urlsafe_b64encode(prose.encode()).decode().rstrip("="),
            prose.encode().hex(),
            "".join(f"\\u{ord(char):04x}" for char in "CANARY") + " private clinical text",
            "".join(f"%{byte:02X}" for byte in prose.encode()),
        ]
        for value in encoded:
            source = copy.deepcopy(self.export)
            source["detail"]["future"] = value
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    detail_snapshot(source)

    def test_urlsafe_base64_canary_fails_closed_for_detail_and_audit(self):
        payload = base64.urlsafe_b64encode(b"CANARY private clinical text: rectal bleeding???").decode().rstrip("=")
        for kind, convert, path in (
            ("detail", detail_snapshot, ("detail", "evidence")),
            ("audit", audit_snapshot, ("audit", "chain")),
        ):
            for encoded in (payload, payload + "=" * (-len(payload) % 4)):
                source = copy.deepcopy(self.export)
                source[path[0]][path[1]] = [{"body": {"label": encoded}}]
                with self.subTest(kind=kind, padded=encoded != payload):
                    with self.assertRaises(ValueError):
                        convert(source)

    def test_non_redacted_or_wrong_schema_fails_closed(self):
        for mutate in (lambda e: e.update(redacted=False), lambda e: e.update(schema="other"), lambda e: e.pop("redacted")):
            source = copy.deepcopy(self.export)
            mutate(source)
            with self.assertRaises(ValueError):
                detail_snapshot(source)


if __name__ == "__main__":
    unittest.main()
