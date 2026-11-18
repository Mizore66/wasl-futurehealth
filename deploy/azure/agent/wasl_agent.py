"""Explain a Wasl case's redacted audit trail for an auditor. Read-only: one GET, one model call."""

from __future__ import annotations

import json
import logging
import re
import urllib.error
import urllib.parse
import urllib.request
from typing import Callable

log = logging.getLogger("wasl_agent")

ModelFn = Callable[[str, str], str]
TokenSource = Callable[[], str]

# Loop ids look like `ahs-mushrif/ServiceRequest/AHS-sr-1001` (docs/API.md, docs/api-samples).
CASE_ID_RE = re.compile(r"\b[a-z0-9][a-z0-9-]*/[A-Z][A-Za-z]+/[A-Za-z0-9][A-Za-z0-9._-]*")

FETCH_TIMEOUT_S = 10
MAX_EVENTS = 12
MAX_EVENT_CHARS = 400
MAX_PROMPT_CHARS = 6000
MAX_OUTPUT_TOKENS = 500

SYSTEM_PROMPT = (
    "You explain the audit trail of one Wasl screening-loop case to an auditor in plain language. "
    "All data is synthetic; say so. Describe what happened to the case in order, and state plainly "
    "whether the hash chain verified and whether the decision signatures matched their recomputation. "
    "Do not give clinical advice or recommendations about any patient. Do not invent events that "
    "are not in the data. Keep it under 200 words."
)

NO_CASE_ID_REPLY = (
    "Please give a Wasl case id, for example ahs-mushrif/ServiceRequest/AHS-sr-1001. "
    "I explain the redacted audit trail of one synthetic case."
)


class AuditFetchError(Exception):
    def __init__(self, reason: str, status: int | None = None):
        super().__init__(reason)
        self.reason = reason
        self.status = status


def extract_case_id(text: str) -> str | None:
    match = CASE_ID_RE.search(text)
    return match.group(0) if match else None


def fetch_audit(api_url: str, case_id: str, token: str, timeout: float = FETCH_TIMEOUT_S) -> dict:
    url = f"{api_url.rstrip('/')}/api/cases/{urllib.parse.quote(case_id, safe='/')}/audit"
    request = urllib.request.Request(
        url, method="GET", headers={"Authorization": f"Bearer {token}", "Accept": "application/json"}
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return json.loads(response.read())
    except urllib.error.HTTPError as err:
        raise AuditFetchError("request rejected", status=err.code) from None
    except (urllib.error.URLError, TimeoutError) as err:
        reason = "timed out" if isinstance(getattr(err, "reason", err), TimeoutError) else "unreachable"
        raise AuditFetchError(reason) from None
    except json.JSONDecodeError:
        raise AuditFetchError("response was not JSON") from None


def _trim(value: object, limit: int) -> str:
    text = json.dumps(value, separators=(",", ":"), sort_keys=True)
    return text if len(text) <= limit else text[: limit - 3] + "..."


def build_prompt(case_id: str, audit: dict) -> str:
    chain = audit.get("chain") or []
    signatures = audit.get("decision_signatures") or []
    mismatched = [s.get("decision_id") for s in signatures if s.get("sig") != s.get("recomputed")]
    shown = chain[-MAX_EVENTS:]
    lines = [
        f"case: {case_id}",
        f"chain_verified: {json.dumps(audit.get('chain_verified'))}",
        f"chain_length: {len(chain)}",
        f"decision_signatures: {len(signatures)} total, {len(signatures) - len(mismatched)} match recomputation",
    ]
    if mismatched:
        lines.append(f"mismatched decision ids: {_trim(mismatched, MAX_EVENT_CHARS)}")
    lines.append(f"review history entries: {len(audit.get('history') or [])}")
    lines.append(f"chain events (last {len(shown)} of {len(chain)}, bodies trimmed):")
    for i, event in enumerate(shown, start=len(chain) - len(shown) + 1):
        lines.append(f"{i}. {event.get('type', '?')}: {_trim(event.get('body'), MAX_EVENT_CHARS)}")
    return "\n".join(lines)[:MAX_PROMPT_CHARS]


def provenance_line(case_id: str, audit: dict, deployment: str) -> str:
    verified = json.dumps(audit.get("chain_verified"))
    return f"[synthetic data | case {case_id} | chain_verified={verified} | model {deployment}]"


def answer(text: str, *, api_url: str, token: TokenSource, model: ModelFn, deployment: str) -> str:
    case_id = extract_case_id(text)
    if case_id is None:
        return NO_CASE_ID_REPLY
    try:
        audit = fetch_audit(api_url, case_id, token())
    except AuditFetchError as err:
        status = f" (HTTP {err.status})" if err.status is not None else ""
        return f"Could not read the audit trail for {case_id}: {err.reason}{status}."
    try:
        explanation = model(SYSTEM_PROMPT, build_prompt(case_id, audit))
    except Exception as err:  # the model client raises many SDK-specific types; the reply must stay short
        log.exception("model call failed for %s", case_id)
        return f"{provenance_line(case_id, audit, deployment)}\nThe model call failed ({type(err).__name__})."
    return f"{provenance_line(case_id, audit, deployment)}\n{explanation}"
