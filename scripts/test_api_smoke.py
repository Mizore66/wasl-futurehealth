#!/usr/bin/env python3
"""Tests for scripts/api_smoke.py and `demo.sh smoke`. A fake HTTP server replays the committed samples; no model calls."""
import copy
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
        self.pages = {}
        for prog in self.dash["by_programme"]:
            p = SAMPLES / f"cases_{prog}_page1.json"
            if p.exists():
                self.pages[prog] = json.loads(p.read_text())
        self.extra_cases = 0
        if mutate:
            mutate(self)

    def route(self, path, query):
        if path == "/healthz":
            return 200, self.health
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
            limit = min(limit, 500)
            row = page["cases"][0]
            page["cases"] = [dict(row, programme=prog)] * min(limit, total) + [dict(row, programme=prog)] * self.extra_cases
            page["total"], page["limit"] = total, limit
            return 200, page
        if path.startswith("/api/cases/"):
            rest = path[len("/api/cases/"):]
            audit = rest.endswith("/audit")
            cid = rest[:-len("/audit")] if audit else rest
            table = self.audit if audit else self.detail
            if cid in table:
                return 200, table[cid]
            return 404, {"error": "unknown case", "id": cid}
        return 404, {"error": "no route"}


def make_handler(fake):
    class H(BaseHTTPRequestHandler):
        def do_GET(self):
            u = urlparse(self.path)
            code, body = fake.route(u.path, parse_qs(u.query))
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
                              capture_output=True, text=True, timeout=60)
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
            f.route = lambda p, q: (200, {"ok": 1}) if p.startswith("/api/cases/") and "unknown" in p else orig(p, q)
        self.assertFails(m, "404")


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
        env = dict(os.environ, WASL_BIN=str(tmp / "bin"), WASL_OUT=str(tmp / "out"), FAKE_LOG=str(log),
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


if __name__ == "__main__":
    unittest.main()
