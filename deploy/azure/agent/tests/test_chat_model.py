import json
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

import main


def test_chat_model_posts_one_bearer_authenticated_completion(monkeypatch):
    seen = []

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            seen.append((self.path, self.headers.get("Authorization"), body))
            payload = json.dumps({"choices": [{"message": {"content": "synthetic summary"}}]}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

        def log_message(self, *_):
            pass

    server = HTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    monkeypatch.delenv("WASL_KEYVAULT_URL", raising=False)
    monkeypatch.setenv("WASL_LLM_KEY", "k" * 32)
    main.secret.cache_clear()
    try:
        call = main.chat_model(f"http://127.0.0.1:{server.server_port}/", "gemma-3-4b-it-Q4_K_M")
        assert call("system text", "user text") == "synthetic summary"
    finally:
        server.shutdown()
        main.secret.cache_clear()

    assert len(seen) == 1
    path, auth, body = seen[0]
    assert path == "/v1/chat/completions"
    assert auth == "Bearer " + "k" * 32
    assert body["model"] == "gemma-3-4b-it-Q4_K_M"
    assert body["max_tokens"] == main.wasl_agent.MAX_OUTPUT_TOKENS
    assert [m["role"] for m in body["messages"]] == ["system", "user"]
