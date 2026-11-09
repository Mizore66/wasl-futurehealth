#!/usr/bin/env python3
"""Tests for scripts/api_smoke.py and `demo.sh smoke`. A stateful fake HTTP server replays the committed samples and workflow routes; no model calls."""
import base64
import copy
import hashlib
import importlib.util
import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import threading
import unittest
from urllib.parse import urlparse, parse_qs

ROOT = Path(__file__).resolve().parent.parent
SAMPLES = ROOT / "docs" / "api-samples"
SMOKE = ROOT / "scripts" / "api_smoke.py"
DEMO = ROOT / "scripts" / "demo.sh"
TOKENS = {
    "WASL_API_REVIEWER_TOKEN": "reviewer-token-1234567890",
    "WASL_API_REVIEWER_2_TOKEN": "reviewer-2-token-1234567890",
    "WASL_API_REVIEWER_3_TOKEN": "reviewer-3-token-1234567890",
    "WASL_API_AUDITOR_TOKEN": "auditor-token-1234567890",
    "WASL_API_COORDINATOR_TOKEN": "coordinator-token-1234567890",
    "WASL_API_CLINICIAN_TOKEN": "clinician-token-1234567890",
}
PRINCIPALS = {
    "reviewer-token-1234567890": ("reviewer", "smoke-reviewer-1"),
    "reviewer-2-token-1234567890": ("reviewer", "smoke-reviewer-2"),
    "reviewer-3-token-1234567890": ("reviewer", "smoke-reviewer-3"),
    "auditor-token-1234567890": ("auditor", "smoke-auditor"),
    "coordinator-token-1234567890": ("coordinator", "smoke-coordinator"),
    "clinician-token-1234567890": ("clinician", "smoke-clinician"),
}


def load(name):
    return json.loads((SAMPLES / name).read_text())


class Fake:
    """Serves the committed samples the way the Go API does (paging, 404)."""

    def __init__(self, mutate=None):
        self.health = load("healthz.json")
        self.dash = load("dashboard.json")
        self.fix = load("fixtures.json")
        self.detail, self.audit = {}, {}
        for tag, f in self.fix["fixtures"].items():
            self.detail[f["id"]] = load(f"case_{tag}.json")
            self.audit[f["id"]] = load(f"case_{tag}_audit.json")
            self.detail[f["id"]].setdefault("agent_investigations", [])  # samples taken before the agent fields existed
            self.detail[f["id"]].setdefault("agent_provenance", {"cached": False, "experimental": False})
        self.pages = {}
        for prog in self.dash["by_programme"]:
            p = SAMPLES / f"cases_{prog}_page1.json"
            if p.exists():
                self.pages[prog] = json.loads(p.read_text())
        self.extra_cases = 0
        self.events = []
        if mutate:
            mutate(self)

    def event(self, values):
        ev = dict(values, at=f"2026-11-09T00:00:{len(self.events):02d}Z", mode="demo",
                  prev=self.events[-1]["hash"] if self.events else "genesis")
        unsigned = json.dumps(ev, sort_keys=True, separators=(",", ":")).encode()
        ev["hash"] = hashlib.sha256(unsigned).hexdigest()
        self.events.append(ev)
        return ev

    def case_events(self, cid):
        return [e for e in self.events if e["case"] == cid]

    def coordinator_overlay(self, cid):
        events = [e for e in self.case_events(cid) if e.get("type") == "coordinator_action"]
        current = self.detail[cid]["current"]
        return {"head": events[-1]["hash"] if events else current["sig"], "actions": len(events),
                "owner": "", "last_action_at": events[-1]["at"] if events else "",
                "ledger_status": "ok", "notes": [], "contact_outcomes": [],
                "canonical_owner": self.detail[cid]["case"].get("owner", "")}

    def consent_overlay(self, cid):
        events = [e for e in self.case_events(cid) if e.get("action") == "record_consent" and
                  e.get("decision", "").startswith("consent_")]
        status = {"consent_denied": "denied"}.get(events[-1]["decision"], "none") if events else "none"
        return {"status": status, "demo_pending_consent": "cancelled" if status == "denied" else "",
                "events": len(events), "contact_blocked": status == "denied", "mode": "demo",
                "canonical_consent": self.detail[cid]["case"]["consent"], "canonical_unchanged": True,
                "last_event_hash": events[-1]["hash"] if events else ""}

    def undo_projection(self, cid):
        statuses = {}
        rows = []
        for e in self.case_events(cid):
            kind = e.get("kind", "review")
            status = "active" if kind == "review" else "applied"
            if kind == "undo":
                statuses[e["target"]] = "compensated"
            elif kind == "contest":
                statuses[e["target"]] = "contested"
                status = "open"
            elif kind == "contest_resolution":
                statuses[e["target"]] = e["resolution"]
                if e["resolution"] == "upheld":
                    contest = next(x for x in self.case_events(cid) if x["hash"] == e["target"])
                    statuses[contest["target"]] = "compensated"
            statuses[e["hash"]] = status
            rows.append({"hash": e["hash"], "kind": kind, "status": status})
        for row in rows:
            row["status"] = statuses[row["hash"]]
        return {"case": cid, "case_status": "compensated", "canonical_state": self.detail[cid]["case"]["state"], "events": rows}

    def live_detail(self, cid):
        detail = copy.deepcopy(self.detail[cid])
        overlay = self.coordinator_overlay(cid)
        detail["api_overlay"] = overlay
        detail["case"]["api_overlay"] = {k: v for k, v in overlay.items() if k not in ("notes", "contact_outcomes", "canonical_owner")}
        detail["api_reviews"] = self.case_events(cid)
        detail["review_ledger"] = {"status": "ok", "events_applied": len(self.events)}
        detail["api_contact"] = {"overlay": self.consent_overlay(cid), "history": self.case_events(cid)}
        return detail

    def live_audit(self, cid):
        audit = copy.deepcopy(self.audit[cid])
        audit["history"] = self.case_events(cid)
        return audit

    def export_doc(self, cid):
        detail = self.live_detail(cid)
        audit = self.live_audit(cid)
        safe_keys = {"case", "item", "decision", "role", "action", "at", "head_signed", "mode", "prev", "hash",
                     "type", "kind", "head", "seq", "target", "target_action", "resolution", "action_head"}
        safe_events = [{k: v for k, v in e.items() if k in safe_keys} for e in self.case_events(cid)]
        overlay = self.consent_overlay(cid)
        overlay.pop("canonical_consent", None)  # keep the fake projection small; structural fields remain
        overlay["canonical_consent"] = detail["case"]["consent"]
        redacted_detail = {"case": {"id": cid, "state": detail["case"]["state"]},
                           "current": {"sig": detail["current"]["sig"]}, "decisions": [],
                           "api_reviews": safe_events,
                           "api_overlay": {k: self.coordinator_overlay(cid)[k] for k in ("head", "actions", "ledger_status")},
                           "api_contact": {"overlay": overlay, "history": safe_events}}
        redacted_audit = {"case": cid, "chain_verified": audit["chain_verified"],
                          "chain_length": audit["chain_length"], "decision_signatures": audit["decision_signatures"],
                          "history": safe_events}
        doc = {"schema": "wasl-case-export/1", "redacted": True,
               "meta": {"api": "wasl-api/1", "data_mode": "simulated", "run": "redacted-internal-audit",
                        "policy": "redacted-internal-audit", "reader_model": "not-disclosed",
                        "loaded_at": "2026-11-09T00:00:00Z", "note": "safe synthetic note"},
               "detail": redacted_detail, "audit": redacted_audit}
        doc["content_sha256"] = hashlib.sha256(json.dumps(doc, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        return doc

    def route(self, method, path, query, body=None, role="", actor=""):
        if path == "/healthz":
            return 200, self.health
        if path.startswith("/api/"):
            if not role:
                return 401, {"error": "authentication required"}
            if method == "GET":
                audit_route = path.endswith("/audit") or path.endswith("/export")
                if audit_route and role != "auditor":
                    return 403, {"error": "read permission denied"}
                if not audit_route and role == "auditor":
                    return 403, {"error": "read permission denied"}
        if path == "/api/dashboard":
            return 200, self.dash
        if path == "/api/fixtures":
            return 200, self.fix
        if path == "/api/cases":
            prog = query.get("programme", [""])[0]
            page = copy.deepcopy(next(iter(self.pages.values())))
            total = self.dash["by_programme"][prog]["cases"]
            limit = int(query.get("limit", ["100"])[0] or 100)
            if limit <= 0:
                limit = 100
            if limit > 500:
                limit = 100  # real API: an over-max limit falls back to the default
            row = page["cases"][0]
            page["cases"] = [dict(row, programme=prog)] * min(limit, total) + [dict(row, programme=prog)] * self.extra_cases
            page["total"], page["limit"] = total, limit
            return 200, page
        if path.startswith("/api/cases/"):
            rest = path[len("/api/cases/"):]
            for suffix in ("/actions", "/consent", "/review", "/undo", "/export"):
                if rest.endswith(suffix):
                    cid = rest[:-len(suffix)]
                    if cid not in self.detail:
                        return 404, {"error": "unknown case", "id": cid}
                    current = self.detail[cid]["current"]
                    if suffix == "/export":
                        if method != "GET":
                            return 405, {"error": "method not allowed"}
                        return 200, self.export_doc(cid)
                    if suffix == "/actions":
                        if role != "coordinator":
                            return 403, {"error": "rbac"}
                        if body["expected_head"] != self.coordinator_overlay(cid)["head"]:
                            return 409, {"error": "stale", "current_head": self.coordinator_overlay(cid)["head"]}
                        ev = self.event({"type": "coordinator_action", "kind": body["action"], "case": cid,
                                         "data": {"text": body["text"]}, "role": role, "actor": actor,
                                         "action": "coordinate_case", "head": body["expected_head"],
                                         "seq": self.coordinator_overlay(cid)["actions"] + 1})
                        return 201, {"event": ev, "api_overlay": self.coordinator_overlay(cid)}
                    if suffix == "/consent":
                        if method == "GET":
                            return 200, {"case": cid, "overlay": self.consent_overlay(cid), "history": self.case_events(cid)}
                        ev = self.event({"case": cid, "item": "", "decision": body["event"], "note": body["note"],
                                         "role": role, "actor": actor, "action": "record_consent", "head_signed": current["sig"]})
                        return 201, {"review": ev, "overlay": self.consent_overlay(cid)}
                    if suffix == "/review":
                        ev = self.event({"case": cid, "item": body.get("item", ""), "decision": body["decision"],
                                         "note": body.get("note", ""), "role": role, "actor": actor,
                                         "action": "signoff_link", "head_signed": current["sig"]})
                        return 201, {"review": ev}
                    if suffix == "/undo":
                        if method == "GET":
                            return 200, self.undo_projection(cid)
                        target = next(e for e in self.events if e["hash"] == body["target"])
                        if body.get("contest"):
                            ev = self.event({"kind": "contest", "case": cid, "target": target["hash"],
                                             "target_actor": target["actor"], "target_action": target["action"],
                                             "action": target["action"], "role": role, "actor": actor,
                                             "note": body.get("note", ""), "head_signed": current["sig"],
                                             "action_head": self.case_events(cid)[-1]["hash"]})
                            return 201, {"event": ev, "requires_resolution": True,
                                         "canonical_reopened": False, "auto_closure": False}
                        if not body.get("resolution"):
                            ev = self.event({"kind": "undo", "case": cid, "target": target["hash"],
                                             "target_actor": target["actor"], "target_action": target["action"],
                                             "action": target["action"], "role": role, "actor": actor,
                                             "note": body.get("note", ""), "head_signed": current["sig"],
                                             "action_head": self.case_events(cid)[-1]["hash"]})
                            return 201, {"event": ev, "canonical_reopened": False, "auto_closure": False}
                        ev = self.event({"kind": "contest_resolution", "case": cid, "target": target["hash"],
                                         "target_actor": target["target_actor"], "target_action": target["action"],
                                         "action": target["action"], "role": role, "actor": actor, "note": "",
                                         "resolution": body["resolution"], "head_signed": current["sig"],
                                         "action_head": self.case_events(cid)[-1]["hash"]})
                        return 201, {"event": ev, "canonical_reopened": False, "auto_closure": False}
            audit = rest.endswith("/audit")
            cid = rest[:-len("/audit")] if audit else rest
            if cid in self.detail:
                return 200, self.live_audit(cid) if audit else self.live_detail(cid)
            return 404, {"error": "unknown case", "id": cid}
        return 404, {"error": "no route"}


def principal(headers):
    scheme, _, token = headers.get("Authorization", "").partition(" ")
    return PRINCIPALS.get(token, ("", "")) if scheme.lower() == "bearer" else ("", "")


def make_handler(fake):
    class H(BaseHTTPRequestHandler):
        def do_GET(self):
            u = urlparse(self.path)
            role, actor = principal(self.headers)
            code, body = fake.route("GET", u.path, parse_qs(u.query), None, role, actor)
            b = json.dumps(body).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(b)))
            self.end_headers()
            self.wfile.write(b)

        def do_POST(self):
            u = urlparse(self.path)
            size = int(self.headers.get("Content-Length", "0"))
            body = json.loads(self.rfile.read(size) or b"{}")
            role, actor = principal(self.headers)
            code, body = fake.route("POST", u.path, parse_qs(u.query), body, role, actor)
            b = json.dumps(body).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(b)))
            self.end_headers()
            self.wfile.write(b)

        def log_message(self, *a):
            pass
    return H


def run_smoke(fake):
    srv = HTTPServer(("127.0.0.1", 0), make_handler(fake))
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    try:
        return subprocess.run([sys.executable, str(SMOKE), "--base-url", f"http://127.0.0.1:{srv.server_port}", "--timeout", "5"],
                              capture_output=True, text=True, timeout=60, env=dict(os.environ, **TOKENS))
    finally:
        srv.shutdown()
        srv.server_close()


class SmokeValidatorTests(unittest.TestCase):
    def assertFails(self, mutate, needle):
        r = run_smoke(Fake(mutate))
        self.assertNotEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn(needle, r.stdout + r.stderr)

    def test_passes_on_consistent_api(self):
        r = run_smoke(Fake())
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_export_canonical_json_matches_go_number_and_string_format(self):
        spec = importlib.util.spec_from_file_location("api_smoke_under_test", SMOKE)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        self.assertEqual(module.canonical_json([0.0, -0.0, 1e-7, 1e-6, 1e-5, 1e20, 1e21, 1.25]),
                         b'[0,-0,1e-7,0.000001,0.00001,100000000000000000000,1e+21,1.25]')
        self.assertEqual(module.canonical_json("<&> "), b'"\\u003c\\u0026\\u003e\\u2028"')

    def test_workflow_coordinator_route_must_be_exercised(self):
        def m(f):
            orig = f.route
            f.route = lambda method, p, q, body, role, actor: (404, {"error": "broken actions route"}) if p.endswith("/actions") else orig(method, p, q, body, role, actor)
        self.assertFails(m, "actions")

    def test_workflow_event_hash_is_verified(self):
        def m(f):
            orig = f.event
            def bad(values):
                ev = orig(values)
                ev["hash"] = "0" * 64
                return ev
            f.event = bad
        self.assertFails(m, "ledger hash")

    def test_workflow_canonical_decision_must_not_change(self):
        def m(f):
            orig = f.live_detail
            def changed(cid):
                detail = orig(cid)
                if f.events:
                    detail["current"]["state"] = "mutation"
                return detail
            f.live_detail = changed
        self.assertFails(m, "canonical case or decisions changed")

    def test_workflow_audit_must_contain_durable_new_events(self):
        def m(f):
            orig = f.live_audit
            f.live_audit = lambda cid: dict(orig(cid), history=[])
        self.assertFails(m, "durable audit history")

    def test_export_private_fields_must_be_redacted(self):
        def m(f):
            orig = f.export_doc
            def leaked(cid):
                doc = orig(cid)
                doc["detail"]["note"] = "SMOKE-PRIVATE-COORDINATOR-TEXT"
                unsigned = {k: v for k, v in doc.items() if k != "content_sha256"}
                doc["content_sha256"] = hashlib.sha256(json.dumps(unsigned, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
                return doc
            f.export_doc = leaked
        self.assertFails(m, "sensitive field")

    def test_export_urlsafe_base64_canary_is_rejected_despite_valid_hash(self):
        def m(f):
            orig = f.export_doc
            def leaked(cid):
                doc = orig(cid)
                doc["detail"]["case"]["state"] = base64.urlsafe_b64encode(
                    b"CANARY private clinical text: rectal bleeding???").decode().rstrip("=")
                unsigned = {k: v for k, v in doc.items() if k != "content_sha256"}
                doc["content_sha256"] = hashlib.sha256(json.dumps(unsigned, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
                return doc
            f.export_doc = leaked
        self.assertFails(m, "test canary")

    def test_export_non_canary_registration_keys_are_rejected_despite_valid_hash(self):
        def m(f):
            orig = f.export_doc
            def leaked(cid):
                doc = orig(cid)
                pair = ["clinic-a|MRN-738291", "hospital-b|MRN-104857"]
                doc["detail"]["links"] = [{"referral_patient": pair[0],
                                           "weakest_edge": {"a": pair[0], "b": pair[1]}}]
                doc["detail"]["review_items"] = [{"pair": pair}]
                unsigned = {k: v for k, v in doc.items() if k != "content_sha256"}
                doc["content_sha256"] = hashlib.sha256(json.dumps(unsigned, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
                return doc
            f.export_doc = leaked
        self.assertFails(m, "registration key")

    def test_export_content_hash_is_verified(self):
        def m(f):
            orig = f.export_doc
            def changed(cid):
                doc = orig(cid)
                doc["detail"]["case"]["state"] = "mutation"
                return doc
            f.export_doc = changed
        self.assertFails(m, "content_sha256")

    def test_unreachable_server_fails(self):
        r = subprocess.run([sys.executable, str(SMOKE), "--base-url", "http://127.0.0.1:1", "--timeout", "1"],
                           capture_output=True, text=True, timeout=60)
        self.assertNotEqual(r.returncode, 0)

    def test_health_case_count_must_match_dashboard(self):
        self.assertFails(lambda f: f.health.__setitem__("cases", f.health["cases"] + 1), "dashboard")

    def test_dashboard_programmes_must_sum_to_cases(self):
        def m(f):
            next(iter(f.dash["by_programme"].values()))["cases"] += 1
        self.assertFails(m, "by_programme")

    def test_case_list_must_respect_limit(self):
        self.assertFails(lambda f: setattr(f, "extra_cases", 5), "limit")

    def test_required_fixture_keys_are_checked_as_subset(self):
        def m(f):
            del f.fix["fixtures"]["review_hold"]
        self.assertFails(m, "review_hold")

    def test_extra_fixture_is_validated_dynamically(self):
        def m(f):
            src = f.fix["fixtures"]["evidence_closure"]
            f.fix["fixtures"]["agent_review"] = dict(src, id="x/ServiceRequest/none", fixture="agent_review")
        self.assertFails(m, "agent_review")

    def test_extra_fixture_with_good_detail_passes(self):
        def m(f):
            src = f.fix["fixtures"]["evidence_closure"]
            f.fix["fixtures"]["extra_ok"] = dict(src, fixture="extra_ok")
        r = run_smoke(Fake(m))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_audit_chain_must_verify(self):
        def m(f):
            next(iter(f.audit.values()))["chain_verified"] = False
        self.assertFails(m, "chain_verified")

    def test_evidence_closure_must_be_closed(self):
        def m(f):
            f.fix["fixtures"]["evidence_closure"]["closed"] = False
        self.assertFails(m, "evidence_closure")

    def test_false_closure_trap_must_stay_open(self):
        def m(f):
            f.fix["fixtures"]["false_closure_trap"]["closed"] = True
        self.assertFails(m, "false_closure_trap")

    def test_unknown_case_must_be_404(self):
        def m(f):
            orig = f.route
            f.route = lambda method, p, q, body, role, actor: (200, {"ok": 1}) if p.startswith("/api/cases/") and "unknown" in p else orig(method, p, q, body, role, actor)
        self.assertFails(m, "404")

    def with_agent(self, f):
        base = next(iter(f.fix["fixtures"]))
        src = f.fix["fixtures"]["evidence_closure"]
        cid = "ahs-mushrif/ServiceRequest/AGENT-sr-1"
        f.fix["fixtures"]["agent_review"] = dict(src, id=cid)
        d = copy.deepcopy(f.detail[src["id"]])
        d["case"]["id"] = cid
        d["agent_investigations"] = [{"item": "r1", "pair": ["a", "b"], "label": "same",
                                      "citations": [{"tool": "t", "arg": "a", "field": "f", "value": "v"}],
                                      "human": {"link": "same", "reads": ["r1", "agent"], "role": "reviewer"}}]
        d["agent_provenance"] = {"cached": True, "experimental": True, "model": "m", "prompt_ver": "1", "schema_ver": "1"}
        f.detail[cid] = d
        a = copy.deepcopy(f.audit[src["id"]])
        a["case"] = cid
        f.audit[cid] = a
        f.agent = d

    def test_agent_review_fixture_passes_when_consistent(self):
        r = run_smoke(Fake(self.with_agent))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_agent_review_needs_a_cited_same_investigation(self):
        def m(f):
            self.with_agent(f)
            f.agent["agent_investigations"] = []
        self.assertFails(m, "agent_review")

    def test_agent_review_needs_cached_provenance(self):
        def m(f):
            self.with_agent(f)
            f.agent["agent_provenance"]["cached"] = False
        self.assertFails(m, "agent_provenance")

    def test_agent_review_must_not_expose_paths_or_reasoning(self):
        def m(f):
            self.with_agent(f)
            f.agent["agent_provenance"]["cache_path"] = "/x"
        self.assertFails(m, "agent_provenance")

    def test_ordinary_fixture_agent_investigations_must_be_a_list(self):
        def m(f):
            self.with_agent(f)
            f.detail[f.fix["fixtures"]["evidence_closure"]["id"]]["agent_investigations"] = None
        self.assertFails(m, "agent_investigations")


FAKE_BIN = """#!/usr/bin/env python3
import json, os, sys
from http.server import HTTPServer
sys.path.insert(0, os.environ["TEST_SCRIPTS"])
import test_api_smoke as t
open(os.environ["FAKE_LOG"], "a").write(" ".join(sys.argv[1:]) + "\\n")
if os.environ.get("FAKE_DIE"):
    sys.exit(3)
host, port = sys.argv[sys.argv.index("-addr") + 1].rsplit(":", 1)
HTTPServer((host, int(port)), t.make_handler(t.Fake())).serve_forever()
"""


def free_port():
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    p = s.getsockname()[1]
    s.close()
    return p


class DemoShellTests(unittest.TestCase):
    def run_demo(self, tmp, extra=None, die=False):
        tmp = Path(tmp)
        (tmp / "bin").mkdir()
        b = tmp / "bin" / "wsi7"
        b.write_text(FAKE_BIN)
        b.chmod(0o755)
        log = tmp / "serve.log"
        env = dict(os.environ, **TOKENS, WASL_BIN=str(tmp / "bin"), WASL_OUT=str(tmp / "out"), FAKE_LOG=str(log),
                   TEST_SCRIPTS=str(ROOT / "scripts"), WASL_SMOKE_PORT=str(free_port()), WASL_SMOKE_WAIT="10")
        if die:
            env["FAKE_DIE"] = "1"
        env.update(extra or {})
        r = subprocess.run(["bash", str(DEMO), "smoke"], capture_output=True, text=True, env=env, timeout=90)
        return r, log

    def test_smoke_honours_serve_run_and_data_overrides(self):
        with tempfile.TemporaryDirectory() as tmp:
            r, log = self.run_demo(tmp, {"WASL_SERVE_RUN": "/tmp/lane-x", "WASL_SERVE_DATA": "/tmp/cohort-x"})
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            args = log.read_text()
            self.assertIn("serve", args)
            self.assertIn("-run /tmp/lane-x", args)
            self.assertIn("-data /tmp/cohort-x", args)
            self.assertIn("-addr 127.0.0.1:", args)

    def test_serve_defaults_to_the_run_and_cohort(self):
        with tempfile.TemporaryDirectory() as tmp:
            r, log = self.run_demo(tmp)
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            args = log.read_text()
            self.assertIn("/out/run/v7strict-cached-fhir", args)
            self.assertIn("-data " + str(Path(tmp) / "out" / "cohort"), args)

    def test_server_that_never_becomes_healthy_fails_boundedly(self):
        with tempfile.TemporaryDirectory() as tmp:
            r, _ = self.run_demo(tmp, {"WASL_SMOKE_WAIT": "2"}, die=True)
            self.assertNotEqual(r.returncode, 0)

    def test_server_is_terminated_after_smoke(self):
        with tempfile.TemporaryDirectory() as tmp:
            port = free_port()
            r, _ = self.run_demo(tmp, {"WASL_SMOKE_PORT": str(port)})
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            s = socket.socket()
            s.settimeout(1)
            self.assertNotEqual(s.connect_ex(("127.0.0.1", port)), 0)
            s.close()

    def test_all_runs_smoke_after_report_and_container_does_not(self):
        lines = DEMO.read_text().splitlines()
        alls = [l for l in lines if l.strip().startswith("all)")]
        cont = [l for l in lines if l.strip().startswith("container)")]
        self.assertIn("report; smoke;;", alls[0])
        self.assertNotIn("smoke", cont[0])


class SnapshotTests(unittest.TestCase):
    PROGS = ["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"]

    def snap(self, fake, target, programmes=None):
        srv = HTTPServer(("127.0.0.1", 0), make_handler(fake))
        threading.Thread(target=srv.serve_forever, daemon=True).start()
        try:
            env = dict(os.environ, **TOKENS, WASL_OUT=str(Path(target).parent / "out"), WASL_SNAPSHOT_DIR=str(target),
                       WASL_API_ADDR=f"127.0.0.1:{srv.server_port}", WASL_BIN=str(Path(target).parent / "bin"))
            if programmes:
                env["WASL_SNAPSHOT_PROGRAMMES"] = programmes
            return subprocess.run(["bash", str(DEMO), "snapshot"], capture_output=True, text=True, env=env, timeout=90)
        finally:
            srv.shutdown()
            srv.server_close()

    def test_snapshot_replaces_target_without_stale_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "samples"
            target.mkdir()
            (target / "case_stale_fixture.json").write_text("{}")
            (target / "README.md").write_text("keep")
            r = self.snap(Fake(), target)
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            self.assertFalse((target / "case_stale_fixture.json").exists())
            self.assertEqual((target / "README.md").read_text(), "keep")
            for prog in self.PROGS:
                self.assertTrue(json.loads((target / f"cases_{prog}_page1.json").read_text())["cases"], prog)
            for tag in Fake().fix["fixtures"]:
                self.assertTrue((target / f"case_{tag}.json").exists())
                self.assertTrue((target / f"case_{tag}_audit.json").exists())

    def test_snapshot_fails_and_keeps_target_when_a_programme_page_is_empty(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "samples"
            target.mkdir()
            (target / "case_old.json").write_text("{}")
            def m(f):
                f.dash["by_programme"]["ifhas"]["cases"] = 0
            r = self.snap(Fake(m), target)
            self.assertNotEqual(r.returncode, 0)
            self.assertIn("ifhas", r.stdout + r.stderr)
            self.assertTrue((target / "case_old.json").exists())

    def test_snapshot_removes_owner_identifiers_from_reviewer_reads(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "samples"
            def m(f):
                for fixture in f.fix["fixtures"].values():
                    fixture["owner"] = "PRIVATE-OWNER-CANARY"
                    fixture["canonical_owner"] = "PRIVATE-OWNER-CANARY"
                for page in f.pages.values():
                    for case in page["cases"]:
                        case["owner"] = "PRIVATE-OWNER-CANARY"
                        case["canonical_owner"] = "PRIVATE-OWNER-CANARY"
            r = self.snap(Fake(m), target, programmes="crc")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            for name in ("fixtures.json", "cases_crc_page1.json"):
                published = (target / name).read_text()
                self.assertNotIn("PRIVATE-OWNER-CANARY", published, name)
                self.assertNotIn('"owner"', published, name)
                self.assertNotIn('"canonical_owner"', published, name)

    def test_snapshot_scans_staging_and_preserves_previous_samples_on_canary(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "samples"
            target.mkdir()
            (target / "case_old.json").write_text('{"preserved":true}')
            def m(f):
                f.dash["future_server_field"] = "PRIVATE-OWNER-CANARY"
            r = self.snap(Fake(m), target, programmes="crc")
            self.assertNotEqual(r.returncode, 0, r.stdout + r.stderr)
            self.assertIn("canary", (r.stdout + r.stderr).lower())
            self.assertTrue((target / "case_old.json").exists())
            self.assertFalse((target / "dashboard.json").exists())

    def test_default_stages_all_seven_programmes(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "samples"
            r = self.snap(Fake(), target)
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            got = sorted(p.name for p in target.glob("cases_*_page1.json"))
            self.assertEqual(got, sorted(f"cases_{p}_page1.json" for p in self.PROGS))

    def test_scoped_programmes_stage_only_those_pages(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "samples"
            def m(f):
                f.dash["by_programme"]["ifhas"]["cases"] = 0  # other programmes may be absent in a scoped lane
            r = self.snap(Fake(m), target, programmes="crc")
            self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
            self.assertEqual([p.name for p in target.glob("cases_*_page1.json")], ["cases_crc_page1.json"])
            for tag in Fake().fix["fixtures"]:
                self.assertTrue((target / f"case_{tag}_audit.json").exists())

    def test_scoped_programme_must_be_nonempty(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "samples"
            target.mkdir()
            (target / "case_old.json").write_text("{}")
            def m(f):
                f.dash["by_programme"]["crc"]["cases"] = 0
            r = self.snap(Fake(m), target, programmes="crc")
            self.assertNotEqual(r.returncode, 0)
            self.assertTrue((target / "case_old.json").exists())

    def test_unknown_programme_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            r = self.snap(Fake(), Path(tmp) / "samples", programmes="crc,bogus")
            self.assertNotEqual(r.returncode, 0)
            self.assertIn("bogus", r.stdout + r.stderr)


if __name__ == "__main__":
    unittest.main()
