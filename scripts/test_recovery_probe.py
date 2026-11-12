import base64
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock
import urllib.error

SCRIPTS = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPTS))
import recovery_api_probe  # noqa: E402


class Response:
    status = 200

    def __init__(self, payload):
        self.payload = payload

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False

    def read(self):
        return json.dumps(self.payload).encode()


class RecoveryProbeTests(unittest.TestCase):
    def test_export_uses_auditor_while_queue_reads_use_reviewer(self):
        with tempfile.TemporaryDirectory() as tmp:
            secrets = Path(tmp) / "secrets"
            secrets.mkdir()
            (secrets / "api_reviewer_password").write_text("reviewer-secret\n")
            (secrets / "api_auditor_password").write_text("auditor-secret\n")
            (secrets / "ca.crt").write_text("test-only\n")
            output = Path(tmp) / "probe.json"
            identities = {}

            def open_url(request, context, timeout):
                path = request.full_url.split("127.0.0.1:19443", 1)[1]
                if "Authorization" not in request.headers:
                    raise urllib.error.HTTPError(request.full_url, 401, "denied", {}, None)
                basic = request.headers["Authorization"].split(" ", 1)[1]
                identities[path] = base64.b64decode(basic).decode()
                if path == "/healthz":
                    return Response({"cases": 1})
                if path == "/api/cases?limit=1":
                    return Response({"cases": [{"id": "case-1"}]})
                if path == "/api/cases/case-1/export":
                    return Response({"comparison_sha256": "a" * 64, "content_sha256": "b" * 64,
                                     "meta": {"loaded_at": "2026-11-11T00:00:00Z", "data_mode": "simulated"}})
                raise AssertionError(path)

            argv = ["recovery_api_probe.py", "--secrets", str(secrets),
                    "--base-url", "https://127.0.0.1:19443", "--output", str(output)]
            with mock.patch.object(sys, "argv", argv), \
                    mock.patch("ssl.create_default_context", return_value=object()), \
                    mock.patch("urllib.request.urlopen", side_effect=open_url):
                recovery_api_probe.main()

            self.assertEqual(identities["/healthz"], "reviewer:reviewer-secret")
            self.assertEqual(identities["/api/cases?limit=1"], "reviewer:reviewer-secret")
            self.assertEqual(identities["/api/cases/case-1/export"], "auditor:auditor-secret")
            probe = json.loads(output.read_text())
            self.assertEqual(probe["unauthenticated_status"], 401)
            self.assertEqual(probe["data_mode"], "simulated")


if __name__ == "__main__":
    unittest.main()
