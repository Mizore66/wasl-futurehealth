#!/usr/bin/env python3
"""Fail unless every named entry in a GitHub Actions needs context succeeded."""
import json
import os
import sys


def main(argv=None):
    required = list(sys.argv[1:] if argv is None else argv)
    if not required:
        print("required-check: no job names supplied", file=sys.stderr)
        return 2
    try:
        needs = json.loads(os.environ["NEEDS_JSON"])
        if not isinstance(needs, dict):
            raise TypeError("needs context is not an object")
    except (KeyError, TypeError, ValueError) as exc:
        print(f"required-check: invalid needs context: {exc}", file=sys.stderr)
        return 2

    failures = []
    for name in required:
        entry = needs.get(name)
        result = entry.get("result") if isinstance(entry, dict) else None
        if result != "success":
            failures.append(f"{name}={result or 'missing'}")
    if failures:
        print("required-check: unsuccessful or missing lanes: " + ", ".join(failures), file=sys.stderr)
        return 1
    print(f"required-check: all {len(required)} lanes succeeded")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
