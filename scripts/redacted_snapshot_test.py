#!/usr/bin/env python3
import base64
import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from redacted_snapshot import audit_snapshot, detail_snapshot


def valid_public_payload(kind):
    payloads = {
        "health": {
            "status": "ok", "cases": 1,
            "dependencies": {"materialised_run": "loaded", "nats": "not_required_for_read_api", "reader": "cached", "ingest_adapter": "fhir", "review_ledger": "ok"},
        },
        "dashboard": {
            "cases": 0, "closed": 0, "overdue": 0, "states": {}, "dropoff": {}, "review": {}, "by_programme": {}, "fixtures": {},
            "small_cell_rule": "counts 1-4 shown as <5",
            "sla": {"ack_days": 1, "colonoscopy_days": 31, "notification_days": 2, "referral_response_days": 3},
            "providers": [], "origins": [],
            "metrics": {key: {} for key in ("closure", "consent_timing", "crc_kpi_31d", "forged", "provenance", "throughput_ev_s")},
            "capacity": {
                "condition": {}, "data_mode": "simulated", "inputs": {}, "levers": {},
                "note": "Synthetic endoscopy calendar and constraint/no-show assumptions; not live scheduling. Optimiser ~= greedy (no optimisation claim).",
                "status": "ok", "summary": {},
            },
        },
        "fixtures": {"fixtures": {}},
        "cases": {"total": 0, "offset": 0, "limit": 100, "cases": []},
        "operations": {
            "schema": "wasl-operations/1", "observed_at": "2026-11-13T12:00:00Z",
            "api": {"status": "ok", "source": "/healthz", "dependencies": {"materialised_run": "loaded", "nats": "up", "reader": "cached", "ingest_adapter": "fhir", "review_ledger": "ok"}},
            "nats": {"status": "reachable", "source": "nats_jetstream_consumer_info"},
            "queue": {"status": "available", "source": "nats_jetstream_consumer_info", "coverage": "known_durable_consumers", "consumers": [
                {"stream": "FHIRIN", "consumer": "notifications", "status": "available", "num_pending": 2, "num_ack_pending": 1},
                {"stream": "FHIR", "consumer": "prep", "status": "available", "num_pending": 3, "num_ack_pending": 0},
                {"stream": "OUT", "consumer": "reviewer", "status": "available", "num_pending": 4, "num_ack_pending": 1},
            ]},
            "run": {"status": "available", "scope": "completed_synthetic_run", "source": "report7.json", "latency_ms": {"n": 2, "p50": 1.5, "p90": 2.5, "p99": 3.5, "max": 4}, "recovery": {"generated_resources": 10, "fhir_msgs": 11, "expected_outputs": 8, "live_outputs": 8, "ingest_lost": 0, "ingest_duplicates": 0, "lost_outputs": 0, "duplicate_outputs": 0}},
            "api_measurements": {"latency": {"status": "unmeasured", "reason": "api_request_latency_not_recorded"}, "error_rate": {"status": "unmeasured", "reason": "api_request_outcomes_not_recorded"}},
            "backup": {"status": "unmeasured", "source": "recovery_bundle_manifest", "reason": "backup_manifest_not_configured"},
        },
    }
    payload = copy.deepcopy(payloads[kind])
    payload["meta"] = {}
    return payload


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
        nested = prose
        for _ in range(6):
            nested = base64.urlsafe_b64encode(nested.encode()).decode().rstrip("=")
            encoded.append(nested)
        folded = base64.urlsafe_b64encode(prose.encode()).decode()
        encoded.extend([folded[:12] + "\n" + folded[12:], folded[:12] + " \t" + folded[12:]])
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

    def run_snapshot(self, arguments, payload=None):
        return subprocess.run(
            [sys.executable, str(Path(__file__).with_name("redacted_snapshot.py")), *arguments],
            input=None if payload is None else json.dumps(payload), text=True,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
        )

    def test_public_reads_drop_unknown_fields_and_rebuild_metadata(self):
        meta = {"api": "wasl-api/1", "data_mode": "simulated", "note": "rectal bleeding", "run": "MRN-1234"}
        payloads = {}
        for kind in ("health", "dashboard", "fixtures", "cases"):
            payloads[kind] = valid_public_payload(kind)
            payloads[kind]["meta"] = copy.deepcopy(meta)
            payloads[kind]["future_clinical"] = "rectal bleeding"
        payloads["fixtures"]["fixtures"] = {"review_hold": {"id": "case-1", "owner": "operator", "future_clinical": "rectal bleeding"}}
        payloads["cases"]["cases"] = [{"id": "case-1", "owner": "operator", "future_clinical": "rectal bleeding"}]
        for kind, payload in payloads.items():
            with self.subTest(kind=kind):
                proc = self.run_snapshot([kind], payload)
                self.assertEqual(proc.returncode, 0, proc.stderr)
                self.assertNotIn("future_clinical", proc.stdout)
                self.assertNotIn("rectal bleeding", proc.stdout)
                self.assertNotIn("MRN-1234", proc.stdout)
                self.assertNotIn("operator", proc.stdout)

    def test_public_dashboard_rejects_nonconstant_clinical_note(self):
        payload = valid_public_payload("dashboard")
        payload["capacity"]["note"] = "sessile adenoma"
        proc = self.run_snapshot(["dashboard"], payload)
        self.assertNotEqual(proc.returncode, 0)
        self.assertNotIn("sessile adenoma", proc.stdout)

    def test_staged_public_snapshot_rejects_unknown_clinical_fields(self):
        with tempfile.TemporaryDirectory() as directory:
            payload = valid_public_payload("dashboard")
            payload["capacity"].update(note="sessile adenoma", future_clinical="rectal bleeding")
            Path(directory, "dashboard.json").write_text(json.dumps(payload))
            proc = self.run_snapshot(["scan-dir", directory])
            self.assertNotEqual(proc.returncode, 0)
            self.assertNotIn("sessile adenoma", proc.stdout)

    def test_nested_public_objects_drop_unknown_keys_and_overlay_narratives(self):
        dashboard = valid_public_payload("dashboard")
        dashboard["metrics"]["closure"] = {"future_clinical": "rectal_bleeding", "patient": "MRN-738291"}
        proc = self.run_snapshot(["dashboard"], dashboard)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(json.loads(proc.stdout)["metrics"]["closure"], {})

        cases = valid_public_payload("cases")
        cases["cases"] = [{"id": "case-1", "api_overlay": {"snooze": {
            "until": "2026-11-11T00:00:00Z", "reason": "patient asked to wait", "future_clinical": "rectal_bleeding",
        }}}]
        proc = self.run_snapshot(["cases"], cases)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        snooze = json.loads(proc.stdout)["cases"][0]["api_overlay"]["snooze"]
        self.assertEqual(snooze, {"until": "2026-11-11T00:00:00Z"})
        self.assertNotIn("rectal_bleeding", proc.stdout)
        self.assertNotIn("patient asked to wait", proc.stdout)

    def test_non_utf8_and_malformed_encoded_canaries_fail_closed(self):
        raw = b"CANARY private clinical text\xff"
        encoded = [
            base64.b64encode(raw).decode(), raw.hex(),
            base64.b64encode(b"CANARY private clinical text").decode() + "%",
            "%43%41%4E%41%52%59%ZZ",
        ]
        for value in encoded:
            source = copy.deepcopy(self.export)
            source["detail"]["future"] = value
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    detail_snapshot(source)

    def test_exactly_four_base64_layers_around_benign_text_is_accepted(self):
        benign = "adequate"
        for _ in range(4):
            benign = base64.urlsafe_b64encode(benign.encode()).decode().rstrip("=")
        source = copy.deepcopy(self.export)
        source["detail"]["future"] = benign
        converted = detail_snapshot(source)
        self.assertEqual(converted["future"], benign)

    def test_staged_scan_rejects_unrecognized_json_filename(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "cases_crc_page2.json").write_text('{"clinical":"rectal bleeding"}')
            proc = self.run_snapshot(["scan-dir", directory])
            self.assertNotEqual(proc.returncode, 0)

    def test_health_requires_documented_fields(self):
        proc = self.run_snapshot(["health"], {"meta": {}})
        self.assertNotEqual(proc.returncode, 0)

    def test_operations_projects_only_the_public_contract(self):
        payload = valid_public_payload("operations")
        payload["meta"] = {"loaded_at": "2026-11-13T11:00:00Z", "run": "/private/run", "policy": "secret-policy"}
        payload["future_internal"] = "CANARY internal error"
        payload["nats"]["internal_error"] = "dial secret-host with password"
        payload["backup"].update({"path": "/secret/manifest", "key_id": "secret-key", "wrapped_dek": "secret-key-material"})
        proc = self.run_snapshot(["operations"], payload)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        projected = json.loads(proc.stdout)
        self.assertEqual(projected["schema"], "wasl-operations/1")
        self.assertEqual(projected["queue"]["consumers"][0]["num_pending"], 2)
        self.assertEqual(projected["queue"]["consumers"][2], {"stream": "OUT", "consumer": "reviewer", "status": "available", "num_pending": 4, "num_ack_pending": 1})
        self.assertEqual(projected["backup"], {"status": "unmeasured", "source": "recovery_bundle_manifest", "reason": "backup_manifest_not_configured"})
        self.assertEqual(projected["meta"]["run"], "redacted-internal-audit")
        for secret in ("future_internal", "internal_error", "secret-host", "password", "/secret/manifest", "secret-key", "/private/run", "secret-policy"):
            self.assertNotIn(secret, proc.stdout)

    def test_operations_requires_each_contract_section(self):
        payload = valid_public_payload("operations")
        for key in ("observed_at", "api", "nats", "queue", "run", "api_measurements", "backup"):
            with self.subTest(missing=key):
                incomplete = copy.deepcopy(payload)
                incomplete.pop(key)
                proc = self.run_snapshot(["operations"], incomplete)
                self.assertNotEqual(proc.returncode, 0)

    def test_staged_operations_uses_the_same_exact_projection(self):
        with tempfile.TemporaryDirectory() as directory:
            payload = valid_public_payload("operations")
            payload["meta"] = {"api": "wasl-api/1", "data_mode": "simulated", "run": "redacted-internal-audit", "policy": "redacted-internal-audit", "reader_model": "not-disclosed", "loaded_at": "", "note": "All patients, providers and outcomes are synthetic (Wikidata names, MTSamples report texts). Not real patient data."}
            Path(directory, "operations.json").write_text(json.dumps(payload))
            proc = self.run_snapshot(["scan-dir", directory])
            self.assertEqual(proc.returncode, 0, proc.stderr)
            payload["backup"]["wrapped_dek"] = "secret-key-material"
            Path(directory, "operations.json").write_text(json.dumps(payload))
            proc = self.run_snapshot(["scan-dir", directory])
            self.assertNotEqual(proc.returncode, 0)
            self.assertNotIn("secret-key-material", proc.stdout)


if __name__ == "__main__":
    unittest.main()
