import json
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import wasl_agent  # noqa: E402

TOKEN = "secret-auditor-token-xyz"
CASE = "ahs-mushrif/ServiceRequest/AHS-sr-1001"
SAMPLE = Path(__file__).resolve().parents[4] / "docs" / "api-samples" / "case_evidence_closure_audit.json"


class FakeWasl:
    def __init__(self):
        self.routes: dict[str, tuple[int, object]] = {}
        self.requests: list[tuple[str, str, str | None]] = []
        fake = self

        class Handler(BaseHTTPRequestHandler):
            def _serve(self):
                fake.requests.append((self.command, self.path, self.headers.get("Authorization")))
                status, body = fake.routes.get(self.path, (404, {"error": "not found"}))
                payload = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)

            do_GET = do_POST = do_PUT = do_DELETE = _serve

            def log_message(self, *args):
                pass

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self.server.server_port}"
        threading.Thread(target=self.server.serve_forever, daemon=True).start()


@pytest.fixture
def wasl():
    fake = FakeWasl()
    yield fake
    fake.server.shutdown()


class FakeModel:
    def __init__(self):
        self.calls: list[tuple[str, str]] = []

    def __call__(self, system, user):
        self.calls.append((system, user))
        return "The synthetic case closed and the chain verified."


def ask(wasl, model, text):
    return wasl_agent.answer(text, api_url=wasl.url, token=lambda: TOKEN, model=model, deployment="gpt-4.1-mini")


def audit_path(case_id=CASE):
    return f"/api/cases/{case_id}/audit"


def test_no_case_id_makes_no_fetch_and_no_model_call(wasl):
    model = FakeModel()
    tokens_read = []

    reply = wasl_agent.answer(
        "what happened to my case?", api_url=wasl.url, token=lambda: tokens_read.append(1) or TOKEN,
        model=model, deployment="d",
    )

    assert "case id" in reply
    assert wasl.requests == []
    assert model.calls == []
    assert tokens_read == []


@pytest.mark.parametrize("status", [401, 403, 404, 500])
def test_rejected_fetch_returns_status_without_token(wasl, status):
    wasl.routes[audit_path()] = (status, {"error": TOKEN})
    model = FakeModel()

    reply = ask(wasl, model, f"explain {CASE}")

    assert f"HTTP {status}" in reply
    assert TOKEN not in reply
    assert model.calls == []


def test_unreachable_api_returns_short_error(wasl):
    model = FakeModel()
    url = wasl.url
    wasl.server.shutdown()
    wasl.server.server_close()

    reply = wasl_agent.answer(f"explain {CASE}", api_url=url, token=lambda: TOKEN, model=model, deployment="d")

    assert reply.startswith(f"Could not read the audit trail for {CASE}")
    assert TOKEN not in reply
    assert model.calls == []


def test_sample_audit_yields_provenance_and_one_get_with_bearer(wasl):
    wasl.routes[audit_path()] = (200, json.loads(SAMPLE.read_text(encoding="utf-8")))
    model = FakeModel()

    reply = ask(wasl, model, f"Please explain {CASE} for me.")

    first, rest = reply.split("\n", 1)
    assert first == f"[synthetic data | case {CASE} | chain_verified=true | model gpt-4.1-mini]"
    assert rest == "The synthetic case closed and the chain verified."
    assert wasl.requests == [("GET", audit_path(), f"Bearer {TOKEN}")]
    assert len(model.calls) == 1
    system, user = model.calls[0]
    assert "synthetic" in system and "clinical advice" in system
    assert "chain_verified: true" in user
    assert "11 total, 11 match recomputation" in user
    assert TOKEN not in user


def test_unverified_chain_and_mismatched_signature_reach_the_prompt(wasl):
    wasl.routes[audit_path()] = (200, {
        "chain_verified": False,
        "chain": [{"type": "closure", "body": {"state": "closed"}}],
        "decision_signatures": [{"decision_id": "dec-1", "sig": "a", "recomputed": "b"}],
    })
    model = FakeModel()

    reply = ask(wasl, model, CASE)

    assert "chain_verified=false" in reply.splitlines()[0]
    user = model.calls[0][1]
    assert "1 total, 0 match recomputation" in user
    assert "dec-1" in user


def test_prompt_is_capped_for_huge_audits(wasl):
    huge_body = {"note": "x" * 5000}
    wasl.routes[audit_path()] = (200, {
        "chain_verified": True,
        "chain": [{"type": f"event_{i}", "body": huge_body} for i in range(1000)],
        "decision_signatures": [{"decision_id": f"d{i}" * 50, "sig": "a", "recomputed": "b"} for i in range(1000)],
    })
    model = FakeModel()

    ask(wasl, model, CASE)

    user = model.calls[0][1]
    assert len(user) <= wasl_agent.MAX_PROMPT_CHARS
    assert "event_999" in user
    assert "event_0:" not in user


def test_extract_case_id_from_surrounding_text():
    assert wasl_agent.extract_case_id(f"audit for {CASE}, please") == CASE
    assert wasl_agent.extract_case_id("phc-retina/ServiceRequest/PHC-sr-1029?") == "phc-retina/ServiceRequest/PHC-sr-1029"
    assert wasl_agent.extract_case_id("no id here/at all") is None
