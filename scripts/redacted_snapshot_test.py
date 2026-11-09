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

    def test_non_canary_registration_keys_are_pseudonymized_consistently(self):
        referral = "clinic-a|MRN-738291"
        downstream = "hospital-b|MRN-104857"
        pair = [referral, downstream]
        link = {"type": "link_decision", "body": {"referral_patient": referral,
                "weakest_edge": {"a": referral, "b": downstream, "p": 0.99, "basis": "model"}}}
        signoff = {"type": "human_signoff", "body": {"item": "link-item", "pair": pair}}
        guard = {"type": "outreach_guard", "body": {"pair": pair}}
        chain = [link, signoff, guard]
        source = copy.deepcopy(self.export)
        source["detail"].update({
            "links": [link["body"]],
            "current": {"chain": chain, "outreach": [{"pair": pair}]},
            "outreach": [{"pair": pair}],
            "decisions": [{"outreach": [{"pair": pair}]}],
        })
        source["detail"]["review_items"][0].update({"pair": pair, "chain": chain})
        source["detail"]["signoffs"][0]["pair"] = pair
        source["detail"]["agent_investigations"][0]["pair"] = pair
        source["audit"]["chain"] = chain

        detail = detail_snapshot(source)
        audit = audit_snapshot(source)
        for output in (detail, audit):
            encoded = json.dumps(output)
            self.assertNotIn(referral, encoded)
            self.assertNotIn(downstream, encoded)
            self.assertNotIn("MRN-738291", encoded)
            self.assertNotIn("MRN-104857", encoded)
        self.assertEqual(source["detail"]["links"][0]["referral_patient"], referral, "input export was mutated")

        referral_alias = detail["links"][0]["referral_patient"]
        downstream_alias = detail["links"][0]["weakest_edge"]["b"]
        self.assertEqual(referral_alias, "reg_1df2acb61bcae715657386bf8c868361026674da9a05dbe962280e76a3ab2367")
        self.assertEqual(downstream_alias, "reg_e3cee6f89fbb3c4faa10a54fca448976804fa980d6902b45ac447dc2370cb207")
        self.assertRegex(referral_alias, r"^reg_[0-9a-f]{64}$")
        self.assertRegex(downstream_alias, r"^reg_[0-9a-f]{64}$")
        self.assertEqual(detail["links"][0]["weakest_edge"]["a"], referral_alias)
        pair_outputs = [
            detail["review_items"][0]["pair"], detail["signoffs"][0]["pair"],
            detail["outreach"][0]["pair"], detail["current"]["outreach"][0]["pair"],
            detail["decisions"][0]["outreach"][0]["pair"], detail["agent_investigations"][0]["pair"],
        ]
        pair_outputs.extend(block["body"]["pair"] for block in detail["current"]["chain"] if "pair" in block["body"])
        pair_outputs.extend(block["body"]["pair"] for block in detail["review_items"][0]["chain"] if "pair" in block["body"])
        pair_outputs.extend(block["body"]["pair"] for block in audit["chain"] if "pair" in block["body"])
        for output in pair_outputs:
            self.assertEqual(output, [referral_alias, downstream_alias])

    def test_non_redacted_or_wrong_schema_fails_closed(self):
        for mutate in (lambda e: e.update(redacted=False), lambda e: e.update(schema="other"), lambda e: e.pop("redacted")):
            source = copy.deepcopy(self.export)
            mutate(source)
            with self.assertRaises(ValueError):
                detail_snapshot(source)


if __name__ == "__main__":
    unittest.main()
