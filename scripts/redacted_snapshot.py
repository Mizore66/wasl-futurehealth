#!/usr/bin/env python3
"""Convert an auditor-redacted case export into frontend fixture detail/audit JSON.

No raw detail or audit response is accepted. Fields required by the frontend but deliberately removed by redaction are
empty strings; source values are never copied back into a snapshot.
"""
import argparse
import base64
import binascii
import copy
import json
import sys
import urllib.parse

SCHEMA = "wasl-case-export/1"
SENSITIVE_KEYS = {"quote", "actor", "target_actor", "registration", "text", "reasoning", "prompt", "raw_text", "excerpt"}
MAX_CANARY_INPUT = 16384


def _canary_string(value):
    if len(value) > MAX_CANARY_INPUT:
        return True  # fail closed rather than allocating an unbounded decoding buffer
    candidates = [value]
    if "%" in value:
        candidates.append(urllib.parse.unquote(value))
    if "\\u" in value:
        try:
            candidates.append(json.loads('"' + value.replace('"', '\\"') + '"'))
        except ValueError:
            pass
    compact = value.strip()
    if len(compact) >= 8 and len(compact) % 4 == 0:
        try:
            candidates.append(base64.b64decode(compact, validate=True).decode("utf-8"))
        except (ValueError, UnicodeDecodeError, binascii.Error):
            pass
    if len(compact) >= 8 and len(compact) % 2 == 0:
        try:
            candidates.append(bytes.fromhex(compact).decode("utf-8"))
        except ValueError:
            pass
    return any("CANARY" in candidate.upper() for candidate in candidates)


def _checked_export(export):
    if not isinstance(export, dict) or export.get("schema") != SCHEMA or export.get("redacted") is not True:
        raise ValueError("snapshot input must be a wasl-case-export/1 document marked redacted")
    for key in ("meta", "detail", "audit"):
        if not isinstance(export.get(key), dict):
            raise ValueError(f"redacted export is missing object {key}")

    def check(value, path="export"):
        if isinstance(value, dict):
            for key, child in value.items():
                # meta.note is fixed server text; every other note is a user narrative field.
                if key == "note" and path != "export.meta" and child not in (None, ""):
                    raise ValueError(f"unredacted narrative field at {path}.{key}")
                if key in SENSITIVE_KEYS and child not in (None, ""):
                    raise ValueError(f"unredacted sensitive field at {path}.{key}")
                if key in {"arg", "value"} and ".citations" in path and child not in (None, ""):
                    raise ValueError(f"unredacted citation identifier at {path}.{key}")
                if isinstance(child, str) and _canary_string(child):
                    raise ValueError(f"test canary at {path}.{key}")
                check(child, f"{path}.{key}")
        elif isinstance(value, list):
            for index, child in enumerate(value):
                check(child, f"{path}[{index}]")

    check(export)


def _review_defaults(events):
    for event in events if isinstance(events, list) else []:
        if isinstance(event, dict):
            event["actor"] = ""
            event["note"] = ""


def detail_snapshot(export):
    _checked_export(export)
    detail = copy.deepcopy(export["detail"])
    detail["meta"] = copy.deepcopy(export["meta"])
    for identity in detail.get("identities", []):
        identity["registration"] = ""
    for item in detail.get("review_items", []):
        item["quote"] = ""
    for signoff in detail.get("signoffs", []):
        signoff.setdefault("verdict", "")
    _review_defaults(detail.get("api_reviews"))
    for investigation in detail.get("agent_investigations", []):
        for citation in investigation.get("citations", []):
            citation["arg"] = ""
            citation["value"] = ""
    if isinstance(detail.get("agent_provenance"), dict):
        detail["agent_provenance"]["prompt_ver"] = ""
    return detail


def audit_snapshot(export):
    _checked_export(export)
    audit = copy.deepcopy(export["audit"])
    audit["meta"] = copy.deepcopy(export["meta"])
    _review_defaults(audit.get("history"))
    return audit


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("kind", choices=("detail", "audit"))
    args = parser.parse_args(argv)
    try:
        export = json.load(sys.stdin)
        output = detail_snapshot(export) if args.kind == "detail" else audit_snapshot(export)
    except (ValueError, TypeError, KeyError) as exc:
        print(f"redacted snapshot refused: {exc}", file=sys.stderr)
        return 1
    json.dump(output, sys.stdout, ensure_ascii=False, indent=1)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
