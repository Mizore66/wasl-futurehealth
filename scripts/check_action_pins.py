#!/usr/bin/env python3
"""Verify that every third-party GitHub Action pin is a full commit object."""
import argparse
import json
import os
from pathlib import Path
import re
import sys
import urllib.error
import urllib.request

USES = re.compile(r"^\s*(?:-\s*)?uses:\s*([^\s#]+)", re.MULTILINE)
PIN = re.compile(r"^([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)@([0-9a-f]{40})$")


def parse_workflow(text):
    """Return unique repository/SHA pairs and malformed third-party uses entries."""
    refs, errors = [], []
    for value in USES.findall(text):
        if value.startswith("./"):
            continue
        match = PIN.fullmatch(value)
        if match is None:
            errors.append(f"{value} is not pinned to a full lowercase commit SHA")
            continue
        ref = (match.group(1), match.group(2))
        if ref not in refs:
            refs.append(ref)
    return refs, errors


def fetch_commit(repo, sha):
    """Get a commit object; GitHub returns 404 here for annotated tag objects."""
    url = f"https://api.github.com/repos/{repo}/git/commits/{sha}"
    headers = {"Accept": "application/vnd.github+json", "User-Agent": "wasl-ci-pin-check"}
    token = os.environ.get("GITHUB_TOKEN")
    if token:
        headers["Authorization"] = f"Bearer {token}"
    request = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        if error.code == 404:
            return None
        raise


def validate_refs(refs, fetch):
    errors = []
    for repo, sha in refs:
        name = f"{repo}@{sha}"
        try:
            commit = fetch(repo, sha)
        except (OSError, ValueError) as error:
            errors.append(f"{name} could not be verified: {error}")
            continue
        if commit is None:
            errors.append(f"{name} is not a commit object (an annotated tag object is not a commit pin)")
        elif commit.get("sha") != sha:
            errors.append(f"{name} returned a different commit")
    return errors


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("workflow", nargs="?", type=Path,
                        default=Path(__file__).resolve().parents[1] / ".github" / "workflows" / "ci.yml")
    args = parser.parse_args(argv)
    refs, errors = parse_workflow(args.workflow.read_text(encoding="utf-8"))
    if not refs and not errors:
        errors.append("no third-party actions found")
    errors.extend(validate_refs(refs, fetch_commit))
    for error in errors:
        print(error, file=sys.stderr)
    if not errors:
        print(f"verified {len(refs)} unique GitHub Action commit pins")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
