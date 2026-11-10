#!/usr/bin/env python3
"""Account for every Go test skip and require integration fixture tests to pass.

Only test names and counts are emitted; raw test output and clinical payloads are
not included in the manifest. Stdlib only.
"""
import argparse
import json
from pathlib import Path
import sys


POLICY_PATH = Path(__file__).resolve().with_name("ci_test_policy.json")


def policy():
    with POLICY_PATH.open(encoding="utf-8") as handle:
        return json.load(handle)


def top_level(test):
    return test.split("/", 1)[0]


def evaluate(events, lane):
    rules = policy()
    required = rules["required_by_lane"].get(lane, [])
    allowed = dict(rules["allowed_skips"])
    allowed.update(rules.get("allowed_skips_by_lane", {}).get(lane, {}))
    required_status = {name: "missing" for name in required}
    errors = []
    allowed_events = []
    unexpected = 0
    failed = 0
    terminal_pass = False
    test_events = 0
    for row in events:
        action = row.get("Action")
        test = row.get("Test", "")
        if not isinstance(test, str):
            raise ValueError("Go test event Test field must be a string")
        top = top_level(test) if test else ""
        if action == "run" and test:
            test_events += 1
        elif action == "pass" and not test:
            terminal_pass = True
        elif action == "pass" and test in required_status:
            if required_status[test] == "missing":
                required_status[test] = "passed"
        elif action == "skip":
            if top in required_status:
                if required_status[top] != "failed":
                    required_status[top] = "skipped"
                errors.append("required test skipped: " + top)
            elif top in allowed:
                allowed_events.append(test)
            else:
                unexpected += 1
                errors.append("unexpected skip: " + (test or "<package>"))
        elif action == "fail":
            failed += 1
            if top in required_status:
                required_status[top] = "failed"
            errors.append("test failure: " + (test or "<package>"))
    if test_events == 0:
        errors.append("no test events in Go JSON log")
    if not terminal_pass:
        errors.append("missing terminal package pass event")
    for name in required:
        if required_status[name] == "missing":
            errors.append("missing required pass: " + name)
    report = {
        "schema": "wasl-go-test-skip-manifest/1",
        "lane": lane,
        "required": required_status,
        "allowed_skips": sorted(allowed_events),
        "summary": {
            "required_passed": sum(status == "passed" for status in required_status.values()),
            "failed": failed,
            "allowed_skips": len(allowed_events),
            "unexpected_skips": unexpected,
        },
        "errors": errors,
    }
    return report, errors


def load_events(path):
    events = []
    with Path(path).open(encoding="utf-8") as handle:
        for number, line in enumerate(handle, 1):
            if not line.strip():
                continue
            try:
                row = json.loads(line)
            except ValueError as exc:
                raise ValueError("invalid Go test JSON at line %d: %s" % (number, exc)) from exc
            if not isinstance(row, dict) or not isinstance(row.get("Action"), str):
                raise ValueError("invalid Go test JSON event at line %d" % number)
            events.append(row)
    return events


def write_manifest(path, report):
    Path(path).write_text(json.dumps(report, sort_keys=True, indent=2) + "\n", encoding="utf-8")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--input", required=True, help="go test -json JSONL file")
    parser.add_argument("--lane", required=True, choices=("go-test", "go-race", "integration"))
    parser.add_argument("--output", help="write a nonclinical JSON manifest to this path")
    args = parser.parse_args(argv)
    try:
        report, errors = evaluate(load_events(args.input), args.lane)
    except (OSError, ValueError) as exc:
        print("ci skip accounting FAILED: " + str(exc), file=sys.stderr)
        return 1
    if args.output:
        write_manifest(args.output, report)
    for error in errors:
        print("ci skip accounting FAILED: " + error, file=sys.stderr)
    if errors:
        return 1
    print("ci skip accounting OK: " + json.dumps(report["summary"], sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
