#!/usr/bin/env python3
"""Check a deployed Wasl API: health, auth, a read, and that a coordinator write survives a restart.

  python3 deploy/azure/verify_api.py smoke  <base-url>               # health and 401 only; no token, no write
  python3 deploy/azure/verify_api.py write  <base-url> <token-dir>   # appends one note, records its marker
  python3 deploy/azure/verify_api.py reread <base-url> <token-dir>   # after a restart: the marker is still there

<token-dir> holds wasl-reviewer.token and wasl-coordinator.token (see docs/DEPLOY.md, "Hosted frontend").
Exits non-zero on the first failed check. Synthetic data only.
"""
import json, pathlib, sys, urllib.error, urllib.parse, urllib.request, uuid

MARKER_FILE = "verify_marker.json"


def call(base, path, token=None, body=None):
    headers = {"Accept": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    data = None
    if body is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(body).encode()
    req = urllib.request.Request(base.rstrip("/") + path, data=data, headers=headers, method="POST" if data else "GET")
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return r.status, json.loads(r.read() or b"null")
    except urllib.error.HTTPError as e:
        return e.code, None


def check(ok, msg):
    print(("PASS " if ok else "FAIL ") + msg)
    if not ok:
        sys.exit(1)


def case_path(case_id):
    return "/api/cases/" + urllib.parse.quote(case_id, safe="/")


def notes(base, coord, case_id):
    code, case = call(base, case_path(case_id), coord)
    check(code == 200, f"GET case {case_id} -> {code}")
    overlay = case.get("api_overlay") or case["case"].get("api_overlay")
    check(isinstance(overlay, dict) and "head" in overlay, "case detail carries api_overlay.head")
    return overlay, json.dumps(overlay)


def main():
    mode, base = sys.argv[1], sys.argv[2]

    code, health = call(base, "/healthz")
    check(code == 200 and health.get("status") == "ok", f"/healthz -> {code} {health and health.get('dependencies')}")
    check(health.get("cases", 0) > 0, f"materialised run loaded with {health.get('cases')} cases")
    code, _ = call(base, "/api/fixtures")
    check(code == 401, f"/api/fixtures without token -> {code}")
    if mode == "smoke":
        return

    tokdir = pathlib.Path(sys.argv[3])
    reviewer = (tokdir / "wasl-reviewer.token").read_text().strip()
    coord = (tokdir / "wasl-coordinator.token").read_text().strip()
    code, fixtures = call(base, "/api/fixtures", reviewer)
    check(code == 200, f"/api/fixtures with reviewer token -> {code}")

    marker_path = tokdir / MARKER_FILE
    if mode == "write":
        case_id = fixtures["fixtures"]["evidence_closure"]["id"]
        overlay, _ = notes(base, coord, case_id)
        marker = f"azure-verify-{uuid.uuid4()}"
        code, _ = call(base, case_path(case_id) + "/actions", coord,
                       {"expected_head": overlay["head"], "action": "note", "text": marker})
        check(code == 201, f"coordinator note append -> {code}")
        _, overlay = notes(base, coord, case_id)
        check(marker in overlay, "note visible on immediate re-read")
        marker_path.write_text(json.dumps({"case_id": case_id, "marker": marker}))
        print(f"marker recorded in {marker_path}; restart the API, then run 'reread'")
    elif mode == "reread":
        m = json.loads(marker_path.read_text())
        _, overlay = notes(base, coord, m["case_id"])
        check(m["marker"] in overlay, f"note {m['marker']} survived the restart (ledger replayed from the share)")
    else:
        sys.exit(f"unknown mode {mode}")


if __name__ == "__main__":
    main()
