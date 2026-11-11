#!/usr/bin/env python3
"""Offline validation and deterministic rendering of the third-party licence notice bundle."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path, PurePosixPath

sys.path.insert(0, str(Path(__file__).resolve().parent))

from supply_chain import _npm_integrity  # noqa: E402

EVIDENCE_CLASSES = ("direct-gitHead", "tag-candidate", "unresolved-native")
PRODUCTION_COUNTS = {"direct-gitHead": 47, "tag-candidate": 21, "unresolved-native": 13}
BUNDLE_NAME = "THIRD-PARTY-NOTICES.txt"
MANIFEST_REL = Path("supply-chain") / "notice-evidence.json"
COMMIT_RE = re.compile(r"^[0-9a-f]{40}$")
SHA_RE = re.compile(r"^[0-9a-f]{64}$")


def _no_duplicate_keys(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def load_manifest(path: Path) -> dict:
    try:
        text = path.read_bytes().decode("utf-8")
        return json.loads(text, object_pairs_hook=_no_duplicate_keys)
    except UnicodeDecodeError as error:
        raise ValueError(f"{path}: not valid UTF-8: {error}") from error
    except json.JSONDecodeError as error:
        raise ValueError(f"{path}: invalid JSON: {error}") from error


def _nonempty_str(value, label: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{label} must be a non-empty string")
    return value


def _notice_file(root: Path, rel: object, label: str) -> Path:
    if not isinstance(rel, str) or not rel:
        raise ValueError(f"{label}: invalid notice path {rel!r}")
    pure = PurePosixPath(rel)
    if pure.is_absolute() or ".." in pure.parts or "\\" in rel or not pure.parts:
        raise ValueError(f"{label}: invalid notice path {rel!r}")
    base = (root / "supply-chain").resolve()
    resolved = (base / Path(*pure.parts)).resolve()
    if not resolved.is_relative_to(base):
        raise ValueError(f"{label}: notice path {rel!r} escapes supply-chain")
    return resolved


def _read_notice(root: Path, entry: object, label: str) -> str:
    if not isinstance(entry, dict):
        raise ValueError(f"{label}: notice must be an object")
    path = _notice_file(root, entry.get("path"), label)
    digest = entry.get("sha256")
    if not isinstance(digest, str) or not SHA_RE.match(digest):
        raise ValueError(f"{label}: notice sha256 must be 64 lowercase hex characters")
    if not path.is_file():
        raise ValueError(f"{label}: missing notice file {entry['path']}")
    data = path.read_bytes()
    if hashlib.sha256(data).hexdigest() != digest:
        raise ValueError(f"{label}: SHA-256 mismatch for {entry['path']}")
    try:
        return data.decode("utf-8")
    except UnicodeDecodeError as error:
        raise ValueError(f"{label}: {entry['path']} is not valid UTF-8") from error


def _validate_package(package: object, index: int) -> tuple[str, str]:
    if not isinstance(package, dict):
        raise ValueError(f"packages[{index}] must be an object")
    name = _nonempty_str(package.get("name"), f"packages[{index}] name")
    version = _nonempty_str(package.get("version"), f"packages[{index}] version")
    label = f"{name}@{version}"
    _nonempty_str(package.get("declaration"), f"{label} declaration")
    cls = package.get("evidence_class")
    if cls not in EVIDENCE_CLASSES:
        raise ValueError(f"{label}: unknown evidence_class {cls!r}")
    source = package.get("source")
    if not isinstance(source, dict):
        raise ValueError(f"{label}: source must be an object")
    _nonempty_str(source.get("url"), f"{label} source url")
    notices = package.get("notices", [])
    if not isinstance(notices, list):
        raise ValueError(f"{label}: notices must be a list")
    if cls == "direct-gitHead":
        commit = source.get("commit")
        if not isinstance(commit, str) or not COMMIT_RE.match(commit):
            raise ValueError(f"{label}: direct row requires a 40-character source commit")
    elif cls == "tag-candidate":
        if not isinstance(source.get("tag"), str) or not source["tag"].strip():
            raise ValueError(f"{label}: tag row requires source tag")
        commit = source.get("resolved_commit")
        if not isinstance(commit, str) or not COMMIT_RE.match(commit):
            raise ValueError(f"{label}: tag row requires a 40-character resolved_commit")
        if not isinstance(package.get("qualification"), str) or not package["qualification"].strip():
            raise ValueError(f"{label}: tag row requires qualification")
    if cls != "unresolved-native" and not notices:
        raise ValueError(f"{label}: {cls} row requires notices")
    if cls == "unresolved-native":
        if notices:
            raise ValueError(f"{label}: unresolved row must not have notices")
        if not isinstance(package.get("limitation"), str) or not package["limitation"].strip():
            raise ValueError(f"{label}: unresolved row requires limitation")
    return name, version


def validate_notice_manifest(
    manifest: dict, root: Path, expected_counts: dict[str, int] | None = None
) -> None:
    """Raise ValueError on any invalid manifest, notice file or lockfile mismatch."""
    if not isinstance(manifest, dict):
        raise ValueError("manifest must be an object")
    if manifest.get("schema_version") != 1 or isinstance(manifest.get("schema_version"), bool):
        raise ValueError("schema_version must be 1")
    packages = manifest.get("packages")
    if not isinstance(packages, list):
        raise ValueError("packages must be a list")
    root = Path(root)
    lock = root / "web" / "pnpm-lock.yaml"
    try:
        locked = _npm_integrity(lock)
    except OSError as error:
        raise ValueError(f"cannot read web/pnpm-lock.yaml: {error}") from error
    seen: set[tuple[str, str]] = set()
    counts = {cls: 0 for cls in EVIDENCE_CLASSES}
    for index, package in enumerate(packages):
        key = _validate_package(package, index)
        label = f"{key[0]}@{key[1]}"
        if key in seen:
            raise ValueError(f"duplicate package {label}")
        seen.add(key)
        if key not in locked:
            raise ValueError(f"{label}: not in web/pnpm-lock.yaml")
        for entry in package.get("notices", []):
            _read_notice(root, entry, label)
        counts[package["evidence_class"]] += 1
    if expected_counts is not None and counts != dict(expected_counts):
        raise ValueError(f"evidence_class counts {counts} != expected {dict(expected_counts)}")


def render_notice_bundle(manifest: dict, root: Path) -> str:
    """Render a deterministic text bundle in manifest order. Offline; no timestamps or paths."""
    root = Path(root)
    lines = [
        "THIRD-PARTY NOTICES",
        "===================",
        "",
        "Generated from supply-chain/notice-evidence.json. Do not edit by hand.",
        "This is packaged evidence, not a legal clearance or an exhaustive audit.",
        "",
        "Evidence classes:",
        "  direct-gitHead    Notice text copied from the exact upstream commit named by the",
        "                    npm package gitHead.",
        "  tag-candidate     Notice text from an upstream tag and its resolved commit. This is",
        "                    qualified context, not direct npm-artifact-to-commit proof.",
        "  unresolved-native Declared or bundled notices were not located. Unresolved; no",
        "                    notice text is supplied.",
        "",
        "Upstream copies do not prove the texts shipped in the npm tarballs.",
        "",
    ]
    for package in manifest["packages"]:
        source = package["source"]
        lines += [
            "-" * 72,
            f"{package['name']}@{package['version']}",
            f"Declaration: {package['declaration']}",
            f"Evidence class: {package['evidence_class']}",
            f"Source: {source['url']}",
        ]
        for field, title in (("commit", "Commit"), ("tag", "Tag"), ("resolved_commit", "Resolved commit")):
            if field in source:
                lines.append(f"{title}: {source[field]}")
        if package.get("qualification"):
            lines.append(f"Qualification: {package['qualification']}")
        if package.get("limitation"):
            lines.append(f"Limitation: {package['limitation']}")
        for entry in package.get("notices", []):
            text = _read_notice(root, entry, f"{package['name']}@{package['version']}")
            lines += ["", f"Notice {entry['path']} (SHA-256 {entry['sha256']}):", "", text.rstrip("\n")]
        lines.append("")
    return "\n".join(lines).rstrip("\n") + "\n"


def check_bundle(manifest: dict, root: Path) -> None:
    """Fail unless the tracked bundle is byte-identical to the rendered manifest."""
    bundle = Path(root) / "supply-chain" / BUNDLE_NAME
    try:
        tracked = bundle.read_bytes()
    except OSError as error:
        raise ValueError(f"cannot read supply-chain/{BUNDLE_NAME}: {error}") from error
    if tracked != render_notice_bundle(manifest, root).encode("utf-8"):
        raise ValueError(f"supply-chain/{BUNDLE_NAME} differs from the rendered manifest; regenerate it")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("check", "render"):
        p = sub.add_parser(name)
        p.add_argument("--root", type=Path, required=True)
        p.add_argument("--no-production-counts", action="store_true", help="skip the 47/21/13 count check")
        if name == "render":
            p.add_argument("--output", type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        manifest = load_manifest(args.root / MANIFEST_REL)
        counts = None if args.no_production_counts else PRODUCTION_COUNTS
        validate_notice_manifest(manifest, args.root, counts)
        if args.command == "render":
            args.output.write_text(render_notice_bundle(manifest, args.root), encoding="utf-8")
        else:
            check_bundle(manifest, args.root)
    except (ValueError, OSError) as error:
        print(f"notices: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
