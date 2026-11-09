#!/usr/bin/env python3
"""Project redacted case exports and non-clinical API reads into frontend snapshot JSON.

No raw detail or audit response is accepted. Fields required by the frontend but deliberately removed by redaction are
empty strings; source values are never copied back into a case snapshot. Health, dashboard, fixture and list responses
use a separate projection that removes owner identities and rebuilds log-derived metadata.
"""
import argparse
import base64
import binascii
import copy
import hashlib
import json
from pathlib import Path
import re
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
    if len(compact) >= 8:
        padded = compact + "=" * (-len(compact) % 4)
        for encoded in (padded, padded.replace("-", "+").replace("_", "/")):
            try:
                candidates.append(base64.b64decode(encoded, validate=True).decode("utf-8"))
            except (ValueError, UnicodeDecodeError, binascii.Error):
                pass
    if len(compact) >= 8 and len(compact) % 2 == 0:
        try:
            candidates.append(bytes.fromhex(compact).decode("utf-8"))
        except ValueError:
            pass
    return any("CANARY" in candidate.upper() for candidate in candidates)


_REGISTRATION_ALIAS = re.compile(r"^reg_[0-9a-f]{64}$")


def _registration_alias(value):
    """Return the Go projector's deterministic join alias; this is not semantic anonymity."""
    if not value or _REGISTRATION_ALIAS.fullmatch(value):
        return value
    digest = hashlib.sha256(b"wasl-registration-pseudonym-v1\0" + value.encode("utf-8")).hexdigest()
    return "reg_" + digest


def _pseudonymize_registration_fields(value, parent=""):
    if isinstance(value, dict):
        for key, child in value.items():
            registration_scalar = (key in {"referral_patient", "registration"} or
                                   parent == "weakest_edge" and key in {"a", "b"})
            if registration_scalar and isinstance(child, str):
                value[key] = _registration_alias(child)
            else:
                value[key] = _pseudonymize_registration_fields(child, key)
    elif isinstance(value, list):
        for index, child in enumerate(value):
            value[index] = _pseudonymize_registration_fields(child, parent)
    elif isinstance(value, str) and parent == "pair":
        return _registration_alias(value)
    return value


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
    _pseudonymize_registration_fields(detail)
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
    _pseudonymize_registration_fields(audit)
    audit["meta"] = copy.deepcopy(export["meta"])
    _review_defaults(audit.get("history"))
    return audit


def read_snapshot(payload):
    """Project non-clinical health/dashboard/fixture/list responses for public snapshots.

    These reviewer reads never include raw evidence, but their summaries can contain canonical or overlay owner IDs.
    Drop identity/narrative fields recursively and rebuild log-derived metadata just as the auditor export does.
    """
    if not isinstance(payload, dict):
        raise ValueError("snapshot read projection requires an object")
    source_meta = payload.get("meta", {})
    if not isinstance(source_meta, dict):
        raise ValueError("snapshot read projection meta must be an object")
    dropped = SENSITIVE_KEYS | {"owner", "canonical_owner"}

    def project(value, path="snapshot"):
        if isinstance(value, dict):
            out = {}
            for key, child in value.items():
                if isinstance(key, str) and _canary_string(key):
                    raise ValueError(f"snapshot canary at {path}.{key}")
                if key not in dropped:
                    out[key] = project(child, f"{path}.{key}")
            return out
        if isinstance(value, list):
            return [project(child, f"{path}[{index}]") for index, child in enumerate(value)]
        if isinstance(value, str) and _canary_string(value):
            raise ValueError(f"snapshot canary at {path}")
        return copy.deepcopy(value)

    output = project(payload)
    output["meta"] = {
        "api": source_meta.get("api", "wasl-api/1"),
        "data_mode": source_meta.get("data_mode", "simulated"),
        "run": "redacted-internal-audit",
        "policy": "redacted-internal-audit",
        "reader_model": "not-disclosed",
        "loaded_at": source_meta.get("loaded_at", ""),
        "note": source_meta.get("note", ""),
    }
    return output


def _check_snapshot_canaries(value, path):
    if isinstance(value, dict):
        for key, child in value.items():
            if isinstance(key, str) and _canary_string(key):
                raise ValueError(f"snapshot canary at {path}.{key}")
            _check_snapshot_canaries(child, f"{path}.{key}")
    elif isinstance(value, list):
        for index, child in enumerate(value):
            _check_snapshot_canaries(child, f"{path}[{index}]")
    elif isinstance(value, str) and _canary_string(value):
        raise ValueError(f"snapshot canary at {path}")


def scan_snapshot_directory(directory):
    """Fail before publication if any staged JSON string contains a raw or encoded canary."""
    root = Path(directory)
    if not root.is_dir():
        raise ValueError("snapshot staging directory is missing")
    for path in sorted(root.rglob("*")):
        if path.is_file():
            try:
                value = json.loads(path.read_text(encoding="utf-8"))
            except (OSError, UnicodeError, ValueError) as exc:
                raise ValueError(f"invalid staged JSON {path.name}: {exc}") from exc
            _check_snapshot_canaries(value, str(path))


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("kind", choices=("detail", "audit", "health", "dashboard", "fixtures", "cases", "scan-dir"))
    parser.add_argument("path", nargs="?")
    args = parser.parse_args(argv)
    try:
        if args.kind == "scan-dir":
            if not args.path:
                raise ValueError("scan-dir requires a staging directory")
            scan_snapshot_directory(args.path)
            return 0
        if args.path:
            raise ValueError("unexpected path argument")
        payload = json.load(sys.stdin)
        if args.kind == "detail":
            output = detail_snapshot(payload)
        elif args.kind == "audit":
            output = audit_snapshot(payload)
        else:
            output = read_snapshot(payload)
    except (OSError, UnicodeError, ValueError, TypeError, KeyError) as exc:
        print(f"redacted snapshot refused: {exc}", file=sys.stderr)
        return 1
    json.dump(output, sys.stdout, ensure_ascii=False, indent=1)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
