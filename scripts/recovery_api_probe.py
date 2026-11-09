#!/usr/bin/env python3
"""Read one synthetic case through authenticated HTTPS and record restart-comparison evidence; never print passwords."""
import argparse
import base64
import json
from pathlib import Path
import ssl
import urllib.error
import urllib.parse
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--secrets", type=Path, required=True)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if not args.base_url.startswith("https://"):
        raise SystemExit("recovery probe requires an HTTPS base URL")
    password = (args.secrets / "api_reviewer_password").read_text().strip()
    authorization = "Basic " + base64.b64encode(("reviewer:" + password).encode()).decode()
    context = ssl.create_default_context(cafile=str(args.secrets / "ca.crt"))

    def get(path, authenticated=True):
        request = urllib.request.Request(args.base_url + path,
                                         headers={"Authorization": authorization} if authenticated else {})
        with urllib.request.urlopen(request, context=context, timeout=10) as response:
            return response.status, json.load(response)

    try:
        get("/healthz", False)
        raise SystemExit("unauthenticated health request unexpectedly succeeded")
    except urllib.error.HTTPError as exc:
        unauthenticated_status = exc.code
    status, health = get("/healthz")
    _, listing = get("/api/cases?limit=1")
    case_id = listing["cases"][0]["id"]
    _, export = get("/api/cases/" + urllib.parse.quote(case_id, safe="") + "/export")
    result = {
        "health_status": status,
        "unauthenticated_status": unauthenticated_status,
        "cases": health["cases"],
        "case_id": case_id,
        "comparison_sha256": export["comparison_sha256"],
        "content_sha256": export["content_sha256"],
        "loaded_at": export["meta"]["loaded_at"],
        "nondeterministic_fields": ["meta.loaded_at", "content_sha256"],
    }
    args.output.write_text(json.dumps(result, sort_keys=True, indent=2) + "\n")
    print(json.dumps({"status": "ok", "probe": result}, sort_keys=True))


if __name__ == "__main__":
    main()
