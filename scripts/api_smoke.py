#!/usr/bin/env python3
"""Real-HTTP smoke test of the read API (`wsi7 serve`). Stdlib only; synthetic data only.

Usage: python3 scripts/api_smoke.py --base-url http://127.0.0.1:8088 [--timeout 10]
Exits 0 when every check passes, 1 on the first failed assertion or transport error.
Fixtures are iterated dynamically: the three original tags must be present, any others are validated too.
"""
import argparse
import json
import sys
import urllib.error
import urllib.parse
import urllib.request

REQUIRED_FIXTURES = ("evidence_closure", "false_closure_trap", "review_hold")
MAX_LIMIT = 500
DEFAULT_LIMIT = 100


class SmokeFailure(Exception):
    pass


def check(cond, msg):
    if not cond:
        raise SmokeFailure(msg)


class Client:
    def __init__(self, base, timeout):
        self.base, self.timeout = base.rstrip("/"), timeout

    def get(self, path):
        """Return (status, parsed JSON). Transport errors and non-JSON bodies fail the smoke."""
        try:
            with urllib.request.urlopen(self.base + path, timeout=self.timeout) as r:
                code, raw = r.status, r.read()
        except urllib.error.HTTPError as e:
            code, raw = e.code, e.read()
        except (urllib.error.URLError, OSError) as e:
            raise SmokeFailure(f"GET {path}: {e}")
        try:
            return code, json.loads(raw)
        except ValueError:
            raise SmokeFailure(f"GET {path}: status {code}, body is not JSON")

    def ok(self, path):
        code, body = self.get(path)
        check(code == 200, f"GET {path}: expected 200, got {code}")
        return body


def case_path(cid, suffix=""):
    return "/api/cases/" + urllib.parse.quote(cid, safe="/") + suffix


def smoke(c):
    health = c.ok("/healthz")
    check(health.get("status") == "ok", f"healthz status is {health.get('status')!r}")
    check(health.get("meta", {}).get("data_mode") == "simulated", "healthz meta.data_mode is not simulated")
    dash = c.ok("/api/dashboard")
    total = dash.get("cases")
    check(isinstance(total, int) and total > 0, "dashboard cases is not a positive integer")
    check(health.get("cases") == total, f"healthz cases {health.get('cases')} != dashboard cases {total}")
    progs = dash.get("by_programme", {})
    check(sum(p["cases"] for p in progs.values()) == total, "dashboard by_programme cases do not sum to cases")
    check(sum(dash.get("states", {}).values()) == total, "dashboard states do not sum to cases")
    check(0 <= dash.get("closed", -1) <= total and 0 <= dash.get("overdue", -1) <= total, "dashboard closed/overdue out of range")
    for name, p in progs.items():
        check(0 <= p["closed"] <= p["cases"] and 0 <= p["overdue"] <= p["cases"], f"dashboard by_programme {name} counts out of range")

    for name, p in progs.items():
        for asked, bound in ((5, 5), (MAX_LIMIT * 2, None)):
            page = c.ok(f"/api/cases?programme={urllib.parse.quote(name)}&limit={asked}")
            applied = page.get("limit")
            if bound is None:  # over-max limit: the API falls back to its default page size (documented 100), never above the max
                check(applied == DEFAULT_LIMIT, f"cases programme={name}: over-max limit applied {applied!r}, expected default {DEFAULT_LIMIT}")
                bound = applied
            check(page.get("total") == p["cases"], f"cases programme={name}: total {page.get('total')} != dashboard {p['cases']}")
            check(len(page["cases"]) <= bound, f"cases programme={name}: {len(page['cases'])} rows exceed limit {bound}")
            check(len(page["cases"]) == min(bound, p["cases"]), f"cases programme={name}: unexpected page size {len(page['cases'])}")
            check(all(r.get("programme") == name for r in page["cases"]), f"cases programme={name}: foreign programme in page")

    fx = c.ok("/api/fixtures").get("fixtures", {})
    for tag in REQUIRED_FIXTURES:
        check(tag in fx, f"fixtures missing required key {tag}")
    check(fx["evidence_closure"].get("closed") is True, "fixture evidence_closure is not closed")
    check(fx["false_closure_trap"].get("closed") is False, "fixture false_closure_trap is closed")
    check(fx["review_hold"].get("review") not in (None, "", "none"), "fixture review_hold carries no review")
    for tag in sorted(fx):
        f = fx[tag]
        cid = f.get("id")
        check(bool(cid), f"fixture {tag}: no id")
        code, d = c.get(case_path(cid))
        check(code == 200, f"fixture {tag}: detail returned {code}")
        summary = d.get("case")
        check(isinstance(summary, dict) and summary.get("id") == cid, f"fixture {tag}: detail case id differs from fixture id")
        cur = d.get("current", {})
        check(cur.get("state") == f.get("state"), f"fixture {tag}: detail state {cur.get('state')!r} != fixture {f.get('state')!r}")
        inv = d.get("agent_investigations")
        check(isinstance(inv, list), f"fixture {tag}: agent_investigations is not a list")
        prov = d.get("agent_provenance")
        check(isinstance(prov, dict) and isinstance(prov.get("cached"), bool), f"fixture {tag}: agent_provenance missing or malformed")
        if tag == "agent_review":
            check(len(inv) >= 1, "fixture agent_review: no agent_investigations")
            for r in inv:
                check(r.get("label") == "same", "fixture agent_review: investigation label is not same")
                check(bool(r.get("citations")), "fixture agent_review: investigation has no citations")
                check(isinstance(r.get("human"), dict) and r["human"].get("link") == "same" and "agent" in r["human"].get("reads", []),
                      "fixture agent_review: human sign-off context lacks agent read / same link")
            check(prov.get("cached") is True and prov.get("experimental") is True, "fixture agent_review: agent_provenance is not cached/experimental")
            check(set(prov) <= {"cached", "experimental", "model", "prompt_ver", "schema_ver"}, f"fixture agent_review: agent_provenance exposes extra keys {sorted(prov)}")
            check(f.get("closed") is True and d.get("case", {}).get("closed") is True, "fixture agent_review: case is not closed")
        else:
            check(len(inv) == 0 or prov.get("cached") is True, f"fixture {tag}: investigations without cached agent_provenance")
        check(isinstance(d.get("decisions"), list) and len(d["decisions"]) == f.get("decisions"), f"fixture {tag}: decision count differs from fixture")
        code, a = c.get(case_path(cid, "/audit"))
        check(code == 200, f"fixture {tag}: audit returned {code}")
        check(a.get("case") == cid, f"fixture {tag}: audit case differs")
        check(a.get("chain_verified") is True, f"fixture {tag}: audit chain_verified is not true")
        check(isinstance(a.get("chain_length"), int) and a["chain_length"] >= 1, f"fixture {tag}: audit chain is empty")

    code, body = c.get(case_path("smoke-unknown/ServiceRequest/none"))
    check(code == 404, f"unknown case: expected 404, got {code}")
    check("error" in body, "unknown case: 404 body has no error")
    return len(fx), total


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--base-url", required=True)
    ap.add_argument("--timeout", type=float, default=10.0, help="per-request timeout in seconds")
    args = ap.parse_args(argv)
    try:
        nfx, total = smoke(Client(args.base_url, args.timeout))
    except SmokeFailure as e:
        print(f"api smoke FAILED: {e}", file=sys.stderr)
        return 1
    print(f"api smoke OK: {total} cases, {nfx} fixtures checked ({args.base_url})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
