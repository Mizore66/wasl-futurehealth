#!/usr/bin/env python3
"""Real-HTTP smoke test of the read/write API (`wsi7 serve`). Stdlib only; synthetic data only.

Usage: python3 scripts/api_smoke.py --base-url http://127.0.0.1:8088 [--timeout 10]
Exits 0 when every check passes, 1 on the first failed assertion or transport error.
Fixtures are iterated dynamically: the three original tags must be present, any others are validated too.
"""
import argparse
import contextlib
import copy
import decimal
import hashlib
import json
import math
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

    def request(self, method, path, body=None, role=None, actor=None):
        """Return (status, parsed JSON). Transport errors and non-JSON bodies fail the smoke."""
        data = None if body is None else json.dumps(body, separators=(",", ":")).encode()
        headers = {}
        if data is not None:
            headers["Content-Type"] = "application/json"
        if role is not None:
            headers["X-Wasl-Role"] = role
        if actor is not None:
            headers["X-Wasl-Actor"] = actor
        req = urllib.request.Request(self.base + path, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as r:
                code, raw = r.status, r.read()
        except urllib.error.HTTPError as e:
            with contextlib.closing(e):
                code, raw = e.code, e.read()
        except (urllib.error.URLError, OSError) as e:
            raise SmokeFailure(f"{method} {path}: {e}")
        try:
            return code, json.loads(raw)
        except ValueError:
            raise SmokeFailure(f"{method} {path}: status {code}, body is not JSON")

    def get(self, path):
        return self.request("GET", path)

    def post(self, path, body, role, actor, expected=201):
        code, payload = self.request("POST", path, body, role, actor)
        check(code == expected, f"POST {path}: expected {expected}, got {code}")
        return payload

    def ok(self, path):
        code, body = self.get(path)
        check(code == 200, f"GET {path}: expected 200, got {code}")
        return body


def case_path(cid, suffix=""):
    return "/api/cases/" + urllib.parse.quote(cid, safe="/") + suffix


def go_float(value):
    """Match encoding/json's shortest float and fixed/exponential thresholds."""
    if not math.isfinite(value):
        raise SmokeFailure("cannot canonicalise a non-finite JSON number")
    rep = repr(value).lower()
    absolute = abs(value)
    if absolute == 0 or 1e-6 <= absolute < 1e21:
        fixed = format(decimal.Decimal(rep), "f")
        if "." in fixed:
            fixed = fixed.rstrip("0").rstrip(".")
        return fixed
    mantissa, exponent = rep.split("e")
    if "." in mantissa:
        mantissa = mantissa.rstrip("0").rstrip(".")
    return f"{mantissa}e{int(exponent):+d}"


def canonical_text(value):
    if value is None:
        return "null"
    if value is True:
        return "true"
    if value is False:
        return "false"
    if isinstance(value, int):
        return str(value)
    if isinstance(value, float):
        return go_float(value)
    if isinstance(value, str):
        # Go's encoder also HTML-escapes these runes; export meta may contain them.
        encoded = json.dumps(value, ensure_ascii=False)
        return (encoded.replace("<", "\\u003c").replace(">", "\\u003e").replace("&", "\\u0026")
                .replace("\u2028", "\\u2028").replace("\u2029", "\\u2029"))
    if isinstance(value, list):
        return "[" + ",".join(canonical_text(v) for v in value) + "]"
    if isinstance(value, dict):
        return "{" + ",".join(canonical_text(str(k)) + ":" + canonical_text(value[k]) for k in sorted(value)) + "}"
    raise SmokeFailure(f"cannot canonicalise JSON value of type {type(value).__name__}")


def canonical_json(value):
    return canonical_text(value).encode()


def check_event_hash(event, label, prev=None):
    check(isinstance(event, dict), f"{label}: event is not an object")
    digest = event.get("hash")
    check(isinstance(digest, str) and len(digest) == 64, f"{label}: missing ledger hash")
    unsigned = {k: v for k, v in event.items() if k != "hash"}
    check(hashlib.sha256(canonical_json(unsigned)).hexdigest() == digest, f"{label}: ledger hash does not match event contents")
    if prev is not None:
        check(event.get("prev") == prev, f"{label}: prev does not link to the prior event")
    return digest


def check_export_redaction(value, forbidden_keys, path="export"):
    if isinstance(value, dict):
        for key, child in value.items():
            check(key not in forbidden_keys, f"{path}: sensitive field {key!r} was not redacted")
            check_export_redaction(child, forbidden_keys, f"{path}.{key}")
    elif isinstance(value, list):
        for i, child in enumerate(value):
            check_export_redaction(child, forbidden_keys, f"{path}[{i}]")


def workflow_smoke(c, cid):
    """Exercise demo overlays on one synthetic fixture; canonical decisions must not move."""
    path = case_path(cid)
    before = c.ok(path)
    before_audit = c.ok(path + "/audit")
    canonical_case = copy.deepcopy(before.get("case", {}))
    canonical_case.pop("api_overlay", None)
    canonical_before = (canonical_case, before.get("current"), before.get("decisions"))
    signatures_before = before_audit.get("decision_signatures")
    chain_before = before_audit.get("chain")
    original_history = before_audit.get("history", [])
    check(isinstance(original_history, list), "workflow: initial audit history is not a list")
    new_events = []
    prev = None

    def record(response, key, label):
        nonlocal prev
        event = response.get(key)
        prev = check_event_hash(event, label, prev)
        new_events.append(event)
        return event

    # RBAC and optimistic concurrency are part of the real HTTP workflow contract.
    c.post(path + "/actions", {"expected_head": before["api_overlay"]["head"], "action": "note", "text": "denied"},
           "auditor", "smoke-auditor", expected=403)
    action = c.post(path + "/actions", {
        "expected_head": before["api_overlay"]["head"], "action": "note",
        "text": "SMOKE-PRIVATE-COORDINATOR-TEXT",
    }, "coordinator", "smoke-coordinator")
    action_event = record(action, "event", "coordinator action")
    check(action_event.get("type") == "coordinator_action" and action_event.get("kind") == "note", "coordinator action: wrong event type/kind")
    check(action["api_overlay"].get("head") == action_event["hash"], "coordinator action: overlay head did not advance")
    c.post(path + "/actions", {"expected_head": "stale-smoke-head", "action": "note", "text": "must not persist"},
           "coordinator", "smoke-coordinator", expected=409)

    consent = c.post(path + "/consent", {
        "event": "consent_denied", "expected_head": before["current"]["sig"],
        "note": "SMOKE-PRIVATE-CONSENT-NOTE",
    }, "coordinator", "smoke-consent")
    consent_event = record(consent, "review", "consent")
    overlay = consent.get("overlay", {})
    check(overlay.get("status") == "denied" and overlay.get("contact_blocked") is True, "consent: denial did not block demo contact")
    check(overlay.get("canonical_consent") == before["case"].get("consent") and overlay.get("canonical_unchanged") is True,
          "consent: canonical consent changed or was not disclosed as unchanged")
    consent_view = c.ok(path + "/consent")
    check(consent_view.get("overlay", {}).get("last_event_hash") == consent_event["hash"], "consent: durable history head differs")

    review = c.post(path + "/review", {
        "decision": "same", "note": "SMOKE-PRIVATE-REVIEW-NOTE",
    }, "reviewer", "smoke-reviewer-1")
    review_event = record(review, "review", "review seed")
    undo = c.post(path + "/undo", {
        "target": review_event["hash"], "note": "SMOKE-PRIVATE-UNDO-NOTE",
    }, "reviewer", "smoke-reviewer-2")
    undo_event = record(undo, "event", "undo")
    check(undo_event.get("kind") == "undo" and undo.get("canonical_reopened") is False and undo.get("auto_closure") is False,
          "undo: wrong compensating event or canonical state changed")

    contest_target = c.post(path + "/review", {
        "decision": "same", "note": "SMOKE-PRIVATE-REVIEW-NOTE",
    }, "reviewer", "smoke-reviewer-1")
    contest_target_event = record(contest_target, "review", "contest target review")
    contest = c.post(path + "/undo", {
        "target": contest_target_event["hash"], "contest": True, "note": "SMOKE-PRIVATE-CONTEST-NOTE",
    }, "reviewer", "smoke-reviewer-2")
    contest_event = record(contest, "event", "contest")
    check(contest_event.get("kind") == "contest" and contest.get("requires_resolution") is True, "contest: wrong event or missing resolution requirement")
    resolution = c.post(path + "/undo", {
        "target": contest_event["hash"], "resolution": "upheld",
    }, "reviewer", "smoke-reviewer-3")
    record(resolution, "event", "contest resolution")
    check(resolution.get("canonical_reopened") is False and resolution.get("auto_closure") is False,
          "contest resolution: canonical case was reopened or automatically closed")
    projection = c.ok(path + "/undo")
    statuses = {e.get("hash"): e.get("status") for e in projection.get("events", [])}
    check(statuses.get(review_event["hash"]) == "compensated" and statuses.get(undo_event["hash"]) == "applied",
          "undo: original review is not compensated or compensating event is not applied")
    check(statuses.get(contest_target_event["hash"]) == "compensated", "contest resolution: original review is not compensated")
    check(statuses.get(contest_event["hash"]) == "upheld", "contest resolution: contest is not upheld")
    check(projection.get("canonical_state") == before["case"].get("state"), "contest resolution: canonical state changed")

    after = c.ok(path)
    after_audit = c.ok(path + "/audit")
    after_case = copy.deepcopy(after.get("case", {}))
    after_case.pop("api_overlay", None)
    check((after_case, after.get("current"), after.get("decisions")) == canonical_before,
          "workflow: canonical case or decisions changed")
    check(after_audit.get("decision_signatures") == signatures_before and after_audit.get("chain") == chain_before,
          "workflow: canonical audit chain or decision signatures changed")
    check(after_audit.get("chain_verified") is True and after_audit.get("chain_length") == before_audit.get("chain_length"),
          "workflow: canonical audit verification changed")
    history = after_audit.get("history", [])
    check(history[:len(original_history)] == original_history, "workflow: prior durable audit history changed")
    check(history[len(original_history):] == new_events, "workflow: durable audit history does not contain exactly the accepted events")

    export = c.ok(path + "/export")
    check(export.get("schema") == "wasl-case-export/1" and export.get("redacted") is True, "export: schema/redaction marker missing")
    served_hash = export.get("content_sha256")
    unsigned = {k: v for k, v in export.items() if k != "content_sha256"}
    check(hashlib.sha256(canonical_json(unsigned)).hexdigest() == served_hash, "export: content_sha256 does not authenticate the document")
    redacted = {"detail": export.get("detail"), "audit": export.get("audit")}
    check_export_redaction(redacted, {"actor", "target_actor", "owner", "canonical_owner", "note", "text", "quote", "reasoning"})
    for canary in ("SMOKE-PRIVATE-COORDINATOR-TEXT", "SMOKE-PRIVATE-CONSENT-NOTE", "SMOKE-PRIVATE-REVIEW-NOTE",
                   "SMOKE-PRIVATE-UNDO-NOTE",
                   "SMOKE-PRIVATE-CONTEST-NOTE", "smoke-coordinator", "smoke-consent", "smoke-reviewer-1",
                   "smoke-reviewer-2", "smoke-reviewer-3"):
        check(canary not in canonical_json(redacted).decode(), f"export: private canary {canary!r} leaked")
    check(export.get("audit", {}).get("chain_verified") is True and export.get("audit", {}).get("chain_length") == before_audit.get("chain_length"),
          "export: redaction changed canonical audit verification")
    repeat = c.ok(path + "/export")
    check(repeat == export, "export: unchanged case did not produce a deterministic document")
    code, _ = c.request("POST", path + "/export", {})
    check(code == 405, f"export POST: expected 405, got {code}")
    code, body = c.get(case_path("smoke-unknown/ServiceRequest/none", "/export"))
    check(code == 404 and "error" in body, f"unknown export: expected JSON 404, got {code}")


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
    workflow_smoke(c, fx["review_hold"]["id"])
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
    print(f"api smoke OK: {total} cases, {nfx} fixtures and workflow routes checked ({args.base_url})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
