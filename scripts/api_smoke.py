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
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

from redacted_snapshot import _canary_string, _registration_alias

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

    @staticmethod
    def token(role, actor=None):
        role = role or "reviewer"
        suffix = ""
        if role == "reviewer" and actor in {"smoke-reviewer-2", "smoke-reviewer-3"}:
            suffix = "_" + actor.rsplit("-", 1)[1]
        name = "WASL_API_" + role.upper() + suffix + "_TOKEN"
        token = os.environ.get(name, "")
        if not token:
            raise SmokeFailure(f"missing {name} for authenticated smoke request")
        return token

    @staticmethod
    def env_token(name):
        token = os.environ.get(name, "")
        if not token:
            raise SmokeFailure(f"missing {name} for authenticated smoke request")
        return token

    def request(self, method, path, body=None, role=None, actor=None, authenticate=True, token_env=None):
        """Return (status, parsed JSON). Transport errors and non-JSON bodies fail the smoke.
        `token_env` names the environment variable holding the exact bearer token to send (independent-review actors)."""
        data = None if body is None else json.dumps(body, separators=(",", ":")).encode()
        headers = {}
        if data is not None:
            headers["Content-Type"] = "application/json"
        if authenticate:
            headers["Authorization"] = "Bearer " + (self.env_token(token_env) if token_env else self.token(role, actor))
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

    def get(self, path, role=None):
        return self.request("GET", path, role=role)

    def post(self, path, body, role, actor, expected=201):
        code, payload = self.request("POST", path, body, role, actor)
        check(code == expected, f"POST {path}: expected {expected}, got {code}")
        return payload

    def ok(self, path, role=None):
        code, body = self.get(path, role)
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
            fixed_meta_note = path == "export.meta" and key == "note"
            check(fixed_meta_note or key not in forbidden_keys, f"{path}: sensitive field {key!r} was not redacted")
            registration_scalar = (key in {"referral_patient", "registration"} or
                                   path.endswith(".weakest_edge") and key in {"a", "b"})
            if registration_scalar and child not in (None, ""):
                check(isinstance(child, str) and child == _registration_alias(child),
                      f"{path}.{key}: registration key was not pseudonymized")
            if key == "pair" and isinstance(child, list):
                for i, item in enumerate(child):
                    check(isinstance(item, str) and (not item or item == _registration_alias(item)),
                          f"{path}.pair[{i}]: registration key was not pseudonymized")
            check_export_redaction(child, forbidden_keys, f"{path}.{key}")
    elif isinstance(value, list):
        for i, child in enumerate(value):
            check_export_redaction(child, forbidden_keys, f"{path}[{i}]")
    elif isinstance(value, str):
        check(not _canary_string(value), f"{path}: test canary was not redacted")


def workflow_smoke(c, cid):
    """Exercise demo overlays on one synthetic fixture; canonical decisions must not move."""
    path = case_path(cid)
    before = c.ok(path)
    before_audit = c.ok(path + "/audit", "auditor")
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
    after_audit = c.ok(path + "/audit", "auditor")
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
    check([event.get("hash") for event in history[len(original_history):]] == [event["hash"] for event in new_events],
          "workflow: durable audit history does not contain exactly the accepted event hashes in order")

    export = c.ok(path + "/export", "auditor")
    check(export.get("schema") == "wasl-case-export/1" and export.get("redacted") is True, "export: schema/redaction marker missing")
    served_hash = export.get("content_sha256")
    unsigned = {k: v for k, v in export.items() if k != "content_sha256"}
    check(hashlib.sha256(canonical_json(unsigned)).hexdigest() == served_hash, "export: content_sha256 does not authenticate the document")
    redacted = export
    check_export_redaction(redacted, {"actor", "target_actor", "owner", "canonical_owner", "note", "text", "quote", "reasoning"})
    for canary in ("SMOKE-PRIVATE-COORDINATOR-TEXT", "SMOKE-PRIVATE-CONSENT-NOTE", "SMOKE-PRIVATE-REVIEW-NOTE",
                   "SMOKE-PRIVATE-UNDO-NOTE",
                   "SMOKE-PRIVATE-CONTEST-NOTE", "smoke-coordinator", "smoke-consent", "smoke-reviewer-1",
                   "smoke-reviewer-2", "smoke-reviewer-3"):
        check(canary not in canonical_json(redacted).decode(), f"export: private canary {canary!r} leaked")
    check(export.get("audit", {}).get("chain_verified") is True and export.get("audit", {}).get("chain_length") == before_audit.get("chain_length"),
          "export: redaction changed canonical audit verification")
    repeat = c.ok(path + "/export", "auditor")
    check(repeat == export, "export: unchanged case did not produce a deterministic document")
    code, _ = c.request("POST", path + "/export", {}, role="auditor")
    check(code == 405, f"export POST: expected 405, got {code}")
    code, body = c.get(case_path("smoke-unknown/ServiceRequest/none", "/export"), "auditor")
    check(code == 404 and "error" in body, f"unknown export: expected JSON 404, got {code}")


# ---------------------------------------------------------------------------------------------------------------------
# Independent double review (protocol wasl-independent-review/1). Separate from the legacy workflow above: it uses only
# raised, still-unsigned synthetic items discovered from the API and never the legacy blank-item undo/contest fixtures.
# ---------------------------------------------------------------------------------------------------------------------
PROTOCOL = "wasl-independent-review/1"
STALE_HEAD = "0" * 64   # well-formed but never a real head: must reach stale handling (409)
MALFORMED_HEAD = "not-a-head"  # not empty and not 64 hex: must be rejected as malformed (400)
PROTOCOL_FIELDS = ("protocol", "read", "question", "expected_head", "resolution_reads")
STATES = {"awaiting_r1", "awaiting_r2", "awaiting_r3", "resolved_agreed", "resolved_senior"}
# question -> credentials (r1, r2, senior), decisions (first, second), and a credential of the other discipline.
DISCIPLINES = {
    "link": {"r1": "WASL_API_REVIEWER_TOKEN", "r2": "WASL_API_REVIEWER_2_TOKEN", "senior": "WASL_API_REVIEWER_3_TOKEN",
             "other": "WASL_API_CLINICIAN_TOKEN", "decisions": ("same", "different")},
    "evidence": {"r1": "WASL_API_CLINICIAN_TOKEN", "r2": "WASL_API_CLINICIAN_2_TOKEN", "senior": "WASL_API_CLINICIAN_3_TOKEN",
                 "other": "WASL_API_REVIEWER_TOKEN", "decisions": ("adequate", "inadequate")},
}
INDEPENDENT_ACTORS = ("smoke-reviewer-1", "smoke-reviewer-2", "smoke-reviewer-3",
                      "smoke-clinician", "smoke-clinician-2", "smoke-clinician-3")


def review_state(detail, item, question, label):
    matches = [s for s in detail.get("api_review_states", []) if s.get("item") == item and s.get("question") == question]
    check(len(matches) == 1, f"{label}: expected exactly one api_review_states entry for {item}/{question}, got {len(matches)}")
    state = matches[0]
    check(state.get("state") in STATES, f"{label}: unknown protocol state {state.get('state')!r}")
    check(state.get("canonical_unchanged") is True, f"{label}: state does not disclose canonical_unchanged")
    return state


def read_entry(state, slot):
    found = [r for r in state.get("reads", []) if r.get("read") == slot]
    return found[0] if found else None


def discover_independent_items(c, programmes, need=2):
    """Raised synthetic items awaiting r1 for which the discipline's r1 credential may submit; discovered, never hard-coded."""
    found = []
    for programme in programmes:
        page = c.ok(f"/api/cases?programme={urllib.parse.quote(programme)}&review=open&limit={MAX_LIMIT}")
        for row in page.get("cases", []):
            cid = row.get("id")
            viewers = [("link", "WASL_API_REVIEWER_TOKEN")]
            if programme == "crc":  # evidence decisions are programme-specific labels; adequate/inadequate is the CRC set
                viewers.append(("evidence", "WASL_API_CLINICIAN_TOKEN"))
            for question, env in viewers:
                code, detail = c.request("GET", case_path(cid), token_env=env)
                check(code == 200, f"independent discovery: GET {cid} returned {code}")
                for st in detail.get("api_review_states", []):
                    if st.get("question") == question and st.get("state") == "awaiting_r1" and st.get("may_submit") is True:
                        found.append((cid, st["item"], question))
            if len(found) >= need:
                return found[:need]
    raise SmokeFailure(f"independent review: found {len(found)} raised unsigned item(s) awaiting r1, need {need}")


class Reader:
    """One configured actor: every request carries that actor's exact bearer token and no identity header."""

    def __init__(self, c, env):
        self.c, self.env = c, env

    def detail(self, cid):
        code, body = self.c.request("GET", case_path(cid), token_env=self.env)
        check(code == 200, f"GET {cid} as {self.env}: expected 200, got {code}")
        return body

    def read(self, cid, item, decision, slot, head, note="", expected=201, raw_body=None):
        body = raw_body if raw_body is not None else {"item": item, "decision": decision, "note": note,
                                                     "read": slot, "expected_head": head}
        code, payload = self.c.request("POST", case_path(cid, "/review"), body, token_env=self.env)
        check(code == expected, f"POST {slot} {item} as {self.env}: expected {expected}, got {code} ({payload.get('error', '')})")
        return payload


def protocol_event(response, slot, question, label):
    event = response.get("review")
    check(isinstance(event, dict), f"{label}: response has no review event")
    check(event.get("protocol") == PROTOCOL and event.get("read") == slot and event.get("question") == question,
          f"{label}: event does not carry protocol/read/question {PROTOCOL}/{slot}/{question}")
    return event


def canonical_view(detail):
    case = copy.deepcopy(detail.get("case", {}))
    case.pop("api_overlay", None)
    return (case, detail.get("current"), detail.get("decisions"))


def independent_review_smoke(c, programmes):
    """Real-HTTP proof of blinded r1/r2, disagreement-gated senior r3 and agreement without r3. Synthetic data only."""
    (cid_a, item_a, q_a), (cid_b, item_b, q_b) = discover_independent_items(c, programmes)
    canon = {}
    for cid in {cid_a, cid_b}:
        d = c.ok(case_path(cid))
        a = c.ok(case_path(cid, "/audit"), "auditor")
        canon[cid] = (canonical_view(d), a.get("decision_signatures"), a.get("chain"), a.get("chain_length"), len(a.get("history", [])))
    accepted = []   # (case, item, question, slot, event) in acceptance order
    chain_prev = [None]

    def accept(response, cid, item, question, slot, label):
        event = protocol_event(response, slot, question, label)
        check_event_hash(event, label)
        if chain_prev[0] is not None:
            check(event.get("prev") == chain_prev[0], f"{label}: prev does not link to the prior accepted event")
        chain_prev[0] = event["hash"]
        accepted.append((cid, item, question, slot, event))
        state = response.get("review_state")
        check(isinstance(state, dict) and state.get("item") == item and state.get("question") == question,
              f"{label}: response has no matching review_state")
        check(state.get("canonical_unchanged") is True, f"{label}: review_state does not disclose canonical_unchanged")
        return event, state

    # A: disagreement -> senior r3.        B: agreement -> no r3.
    for cid, item, question, outcome in ((cid_a, item_a, q_a, "disagree"), (cid_b, item_b, q_b, "agree")):
        d = DISCIPLINES[question]
        first, second = d["decisions"]
        r1, r2, senior = Reader(c, d["r1"]), Reader(c, d["r2"]), Reader(c, d["senior"])
        other = Reader(c, d["other"])
        tag = f"{outcome} {item}/{question}"
        note1 = f"SMOKE-INDEP-R1-NOTE-{outcome}"
        start = review_state(r1.detail(cid), item, question, tag + " initial")
        check(start.get("state") == "awaiting_r1" and start.get("next_read") == "r1" and not start.get("head"),
              f"{tag}: initial state is not awaiting_r1 with an empty head")

        # Requests that must be refused without consuming the slot.
        r1.read(cid, item, first, "r1", "", raw_body={"item": item, "decision": first, "note": "", "read": "r1"}, expected=400)
        other.read(cid, item, first, "r1", "", expected=403)
        check(c.request("POST", case_path(cid, "/review"), {"item": item, "decision": first, "read": "r1", "expected_head": ""},
                        authenticate=False)[0] == 401, f"{tag}: unauthenticated protocol read was not 401")
        senior.read(cid, item, first, "r1", "", expected=403)       # senior is not an initial reader
        check(review_state(r1.detail(cid), item, question, tag).get("state") == "awaiting_r1", f"{tag}: refused writes moved the state")

        e1, s1 = accept(r1.read(cid, item, first, "r1", "", note=note1), cid, item, question, "r1", tag + " r1")
        check(s1.get("state") == "awaiting_r2" and s1.get("next_read") == "r2", f"{tag}: r1 did not open awaiting_r2")
        r1_own_state = review_state(r1.detail(cid), item, question, tag + " own view")
        r1_own = read_entry(r1_own_state, "r1")
        check(r1_own and r1_own.get("decision") == first, f"{tag}: the first reader cannot see their own answer")

        # Same actor and duplicate slot are refused; the second reader is blinded to r1.
        r1.read(cid, item, first, "r2", r1_own_state["head"], expected=403)   # same actor, current head as that actor sees it
        r2.read(cid, item, second if outcome == "disagree" else first, "r1", "", expected=409)
        blind = r2.detail(cid)
        blind_state = review_state(blind, item, question, tag + " blinded view")
        check(blind_state.get("state") == "awaiting_r2" and blind_state.get("may_submit") is True,
              f"{tag}: second reader is not offered r2")
        entry = read_entry(blind_state, "r1")
        check(entry is None or (entry.get("submitted") is True and "decision" not in entry and "hash" not in entry),
              f"{tag}: blinded r1 entry exposes decision or hash")
        full = json.dumps(blind, sort_keys=True)
        check(note1 not in full, f"{tag}: r1 note leaked to the second reader")
        check(e1["hash"] not in full, f"{tag}: r1 event hash leaked to the second reader")
        check(not any(e.get("hash") == e1["hash"] or (e.get("item") == item and e.get("read") == "r1")
                      for e in blind.get("api_reviews", [])), f"{tag}: r1 event present in the second reader's api_reviews")
        senior.read(cid, item, first, "r3", blind_state["head"], expected=409)      # premature r3
        # Mid-state: a hidden r1 cannot be undone or contested by another ordinary actor (the target hash is not disclosed
        # to them, so a guessed/known hash must not be actionable), and the second reader's view carries no r1 event.
        code, _ = c.request("POST", case_path(cid, "/undo"), {"target": e1["hash"], "note": ""}, token_env=d["r2"])
        check(code == 409, f"{tag}: mid-state undo of a concealed r1 expected 409, got {code}")
        code, _ = c.request("POST", case_path(cid, "/undo"), {"target": e1["hash"], "contest": True, "note": ""}, token_env=d["r2"])
        check(code == 409, f"{tag}: mid-state contest of a concealed r1 expected 409, got {code}")
        # The review desk reads only the viewer-authorised detail; audit/export are auditor-authorised and are not
        # fetched with a reviewer credential.
        for suffix in ("/audit", "/export"):
            code, _ = c.request("GET", case_path(cid, suffix), token_env=d["r2"])
            check(code == 403, f"{tag}: reviewer credential read {suffix} mid-state (expected 403, got {code})")
        r2.read(cid, item, first, "r2", MALFORMED_HEAD, expected=400)
        stale = r2.read(cid, item, second if outcome == "disagree" else first, "r2", STALE_HEAD, expected=409)
        check(note1 not in json.dumps(stale) and e1["hash"] not in json.dumps(stale), f"{tag}: stale-head refusal leaked r1")

        decision2 = second if outcome == "disagree" else first
        e2, s2 = accept(r2.read(cid, item, decision2, "r2", blind_state["head"], note=f"SMOKE-INDEP-R2-NOTE-{outcome}"),
                        cid, item, question, "r2", tag + " r2")
        check(e2.get("prev") == e1["hash"], f"{tag}: r2 is not chained after r1")
        revealed = review_state(r1.detail(cid), item, question, tag + " after r2")
        slots = {r["read"]: r for r in revealed.get("reads", [])}
        check(slots.get("r1", {}).get("decision") == first and slots.get("r2", {}).get("decision") == decision2
              and slots["r1"].get("hash") == e1["hash"] and slots["r2"].get("hash") == e2["hash"],
              f"{tag}: both initial answers and hashes are not visible after r2")

        if outcome == "agree":
            check(s2.get("state") == "resolved_agreed" and s2.get("next_read") is None and s2.get("may_submit") is False
                  and s2.get("disagreement") is False and s2.get("final_decision") == first,
                  f"{tag}: agreement did not resolve without r3")
            senior.read(cid, item, first, "r3", s2["head"], expected=409)
            continue

        check(s2.get("state") == "awaiting_r3" and s2.get("disagreement") is True and s2.get("next_read") == "r3",
              f"{tag}: disagreement did not open awaiting_r3")
        head3 = s2["head"]
        check(review_state(r1.detail(cid), item, question, tag).get("may_submit") is False
              and review_state(r2.detail(cid), item, question, tag).get("may_submit") is False,
              f"{tag}: an initial reader is offered r3")
        check(review_state(senior.detail(cid), item, question, tag).get("may_submit") is True,
              f"{tag}: the senior is not offered r3")
        r1.read(cid, item, first, "r3", head3, expected=403)         # ordinary role, initial actor
        r2.read(cid, item, first, "r3", head3, expected=403)
        other.read(cid, item, first, "r3", head3, expected=403)      # wrong discipline
        senior.read(cid, item, first, "r3", STALE_HEAD, expected=409)
        e3, s3 = accept(senior.read(cid, item, first, "r3", head3, note=f"SMOKE-INDEP-R3-NOTE-{outcome}"),
                        cid, item, question, "r3", tag + " r3")
        check(e3.get("resolution_reads") == [e1["hash"], e2["hash"]] and e3.get("prev") == e2["hash"],
              f"{tag}: r3 does not link both initial reads")
        check(s3.get("state") == "resolved_senior" and s3.get("next_read") is None and s3.get("final_decision") == first
              and bool(s3.get("resolution_basis")), f"{tag}: r3 did not produce resolved_senior with a basis")
        senior.read(cid, item, first, "r3", s3["head"], expected=409)   # terminal
        r1.read(cid, item, first, "r2", s3["head"], expected=409)
        code, body = c.request("POST", case_path(cid, "/undo"), {"target": e1["hash"], "note": ""}, token_env=d["r2"])
        check(code == 409, f"{tag}: undo of a protocol read expected 409, got {code}")

    # Reload from the server and verify ordered slots, hashes, redaction and unchanged canonical data.
    for cid in sorted({a[0] for a in accepted}):
        path = case_path(cid)
        mine = [a for a in accepted if a[0] == cid]
        for item, question in sorted({(a[1], a[2]) for a in mine}):
            d = DISCIPLINES[question]
            state = review_state(Reader(c, d["senior"]).detail(cid), item, question, "reload")
            want = [a for a in mine if a[1] == item and a[2] == question]
            check([r.get("read") for r in state["reads"] if r.get("submitted")] == [a[3] for a in want],
                  f"reload {item}/{question}: submitted slots are not ordered as accepted")
            check([r.get("hash") for r in state["reads"] if r.get("submitted")] == [a[4]["hash"] for a in want],
                  f"reload {item}/{question}: slot hashes differ from the accepted events")
            check(state["state"] == ("resolved_senior" if len(want) == 3 else "resolved_agreed"),
                  f"reload {item}/{question}: terminal state is {state['state']!r}")
        audit = c.ok(path + "/audit", "auditor")
        history = audit.get("history", [])
        proto = [e for e in history if e.get("protocol")]
        check([e.get("hash") for e in proto if e.get("hash") in {a[4]["hash"] for a in mine}] == [a[4]["hash"] for a in mine],
              f"{cid}: audit history lacks the accepted protocol events in order")
        for e in proto:
            if e.get("hash") not in {a[4]["hash"] for a in mine}:
                continue
            check(e.get("protocol") == PROTOCOL and e.get("read") in {"r1", "r2", "r3"} and e.get("question") in DISCIPLINES,
                  f"{cid}: audit event lost protocol/read/question")
            check(not ({"actor", "note", "target_actor"} & set(e)), f"{cid}: audit event exposes actor or note")
            if e.get("read") == "r3":
                by_hash = {a[4]["hash"]: a for a in mine}
                check(e.get("resolution_reads") == by_hash[e["hash"]][4].get("resolution_reads"),
                      f"{cid}: audit r3 lost its resolution links")
        blob = canonical_json(audit).decode()
        for canary in ["SMOKE-INDEP-R1-NOTE", "SMOKE-INDEP-R2-NOTE", "SMOKE-INDEP-R3-NOTE", *INDEPENDENT_ACTORS]:
            check(canary not in blob, f"{cid}: audit leaked private canary {canary!r}")
        export = c.ok(path + "/export", "auditor")
        unsigned = {k: v for k, v in export.items() if k != "content_sha256"}
        check(hashlib.sha256(canonical_json(unsigned)).hexdigest() == export.get("content_sha256"),
              f"{cid}: export content_sha256 does not authenticate the document")
        check_export_redaction(export, {"actor", "target_actor", "owner", "canonical_owner", "note", "text", "quote", "reasoning"})
        blob = canonical_json(export).decode()
        for canary in ["SMOKE-INDEP-R1-NOTE", "SMOKE-INDEP-R2-NOTE", "SMOKE-INDEP-R3-NOTE", *INDEPENDENT_ACTORS]:
            check(canary not in blob, f"{cid}: export leaked private canary {canary!r}")
        before = canon[cid]
        after_detail = c.ok(path)
        check(canonical_view(after_detail) == before[0], f"{cid}: canonical case or decisions changed under independent review")
        check(audit.get("decision_signatures") == before[1] and audit.get("chain") == before[2]
              and audit.get("chain_length") == before[3], f"{cid}: canonical audit chain or decision signatures changed")
        check(audit.get("chain_verified") is True, f"{cid}: canonical audit chain no longer verifies")
        check(len(history) >= before[4] + len(mine), f"{cid}: audit history did not grow by the accepted reads")
    return len(accepted)


def smoke(c):
    code, _ = c.request("GET", "/api/cases", authenticate=False)
    check(code == 401, f"unauthenticated case list: expected 401, got {code}")
    code, _ = c.get("/api/cases", "auditor")
    check(code == 403, f"auditor raw case list: expected 403, got {code}")
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
        code, a = c.get(case_path(cid, "/audit"), "auditor")
        check(code == 200, f"fixture {tag}: audit returned {code}")
        check(a.get("case") == cid, f"fixture {tag}: audit case differs")
        check(a.get("chain_verified") is True, f"fixture {tag}: audit chain_verified is not true")
        check(isinstance(a.get("chain_length"), int) and a["chain_length"] >= 1, f"fixture {tag}: audit chain is empty")

    code, body = c.get(case_path("smoke-unknown/ServiceRequest/none"))
    check(code == 404, f"unknown case: expected 404, got {code}")
    check("error" in body, "unknown case: 404 body has no error")
    workflow_smoke(c, fx["review_hold"]["id"])
    independent_review_smoke(c, list(progs))
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
    print(f"api smoke OK: {total} cases, {nfx} fixtures, workflow routes and independent double review checked ({args.base_url})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
