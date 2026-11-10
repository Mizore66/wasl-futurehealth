#!/usr/bin/env python3
"""Authenticated file bundles and transactional, reversible file-generation migration.

The manifest authenticates metadata and every payload digest with a key stored outside the bundle. Verification is
always completed before a migration renames or writes the destination. No secret is included in the bundle.
"""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import hmac
import json
import os
from pathlib import Path
import shutil
import stat
import tempfile

SCHEMA = "wasl-file-backup-v1"


def _canonical(value) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")


def _read_key(path: Path) -> bytes:
    try:
        key = path.read_bytes()
    except OSError as exc:
        raise ValueError(f"backup authentication key unavailable: {exc}") from exc
    if len(key) < 32:
        raise ValueError("backup authentication key must contain at least 32 bytes")
    return key


def _manifest_mac(manifest: dict, key: bytes) -> str:
    body = {k: v for k, v in manifest.items() if k != "hmac_sha256"}
    return hmac.new(key, _canonical(body), hashlib.sha256).hexdigest()


def _copy_tree_contents(source: Path, destination: Path) -> list[dict]:
    records = []
    for root, dirs, files in os.walk(source):
        dirs.sort()
        files.sort()
        root_path = Path(root)
        for directory in dirs:
            path = root_path / directory
            if path.is_symlink():
                raise ValueError(f"symbolic links are not supported in recovery bundles: {path}")
            (destination / path.relative_to(source)).mkdir(parents=True, exist_ok=True)
        for name in files:
            path = root_path / name
            if path.is_symlink() or not stat.S_ISREG(path.stat().st_mode):
                raise ValueError(f"only regular files are supported in recovery bundles: {path}")
            relative = path.relative_to(source)
            target = destination / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            digest = hashlib.sha256()
            with path.open("rb") as src, target.open("xb") as dst:
                while chunk := src.read(1024 * 1024):
                    digest.update(chunk)
                    dst.write(chunk)
                dst.flush()
                os.fsync(dst.fileno())
            os.chmod(target, stat.S_IMODE(path.stat().st_mode) & 0o777)
            records.append({"path": relative.as_posix(), "sha256": digest.hexdigest(), "size": path.stat().st_size})
    return records


def _write_manifest(bundle: Path, records: list[dict], key: bytes) -> dict:
    manifest = {
        "schema": SCHEMA,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "nondeterministic_fields": ["created_at", "file_mtime", "nats_sequence", "nats_time"],
        "files": records,
    }
    manifest["hmac_sha256"] = _manifest_mac(manifest, key)
    encoded = _canonical(manifest) + b"\n"
    temporary = bundle / ".manifest.json.tmp"
    with temporary.open("xb") as handle:
        handle.write(encoded)
        handle.flush()
        os.fsync(handle.fileno())
    os.chmod(temporary, 0o600)
    os.replace(temporary, bundle / "manifest.json")
    return manifest


def write_file_bundle(source: Path | str, bundle: Path | str, key_file: Path | str, nats_json: Path | str | None = None) -> dict:
    source, bundle = Path(source), Path(bundle)
    key = _read_key(Path(key_file))
    if not source.is_dir():
        raise ValueError(f"backup source is not a directory: {source}")
    if bundle.exists():
        raise ValueError(f"backup destination already exists: {bundle}")
    bundle.parent.mkdir(parents=True, exist_ok=True)
    staging = Path(tempfile.mkdtemp(prefix=f".{bundle.name}.staging-", dir=bundle.parent))
    try:
        records = _copy_tree_contents(source, staging / "files")
        if nats_json is not None:
            nats_source = Path(nats_json)
            if not nats_source.is_file():
                raise ValueError(f"NATS backup payload is not a regular file: {nats_source}")
            nats_target = staging / "files" / "nats" / "nats.json"
            nats_target.parent.mkdir(parents=True, exist_ok=True)
            digest = hashlib.sha256()
            with nats_source.open("rb") as src, nats_target.open("xb") as dst:
                while chunk := src.read(1024 * 1024):
                    digest.update(chunk)
                    dst.write(chunk)
                dst.flush()
                os.fsync(dst.fileno())
            os.chmod(nats_target, 0o600)
            records.append({"path": "nats/nats.json", "sha256": digest.hexdigest(), "size": nats_source.stat().st_size})
            records.sort(key=lambda record: record["path"])
        manifest = _write_manifest(staging, records, key)
        os.replace(staging, bundle)
        _fsync_directory(bundle.parent)
        return manifest
    except Exception:
        shutil.rmtree(staging, ignore_errors=True)
        raise


def verify_bundle(bundle: Path | str, key_file: Path | str) -> dict:
    bundle = Path(bundle)
    key = _read_key(Path(key_file))
    try:
        manifest = json.loads((bundle / "manifest.json").read_text())
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError(f"backup manifest unavailable or invalid: {exc}") from exc
    if manifest.get("schema") != SCHEMA or not isinstance(manifest.get("files"), list):
        raise ValueError("unsupported or incomplete backup manifest")
    if not hmac.compare_digest(manifest.get("hmac_sha256", ""), _manifest_mac(manifest, key)):
        raise ValueError("backup authentication failed: wrong key or modified manifest")
    paths = set()
    for record in manifest["files"]:
        relative = Path(record.get("path", ""))
        if relative.is_absolute() or ".." in relative.parts or not relative.parts or relative.as_posix() in paths:
            raise ValueError("backup manifest contains an unsafe or duplicate path")
        paths.add(relative.as_posix())
        path = bundle / "files" / relative
        if not path.is_file() or path.is_symlink():
            raise ValueError(f"backup payload missing: {relative}")
        digest = hashlib.sha256()
        with path.open("rb") as handle:
            while chunk := handle.read(1024 * 1024):
                digest.update(chunk)
        if not hmac.compare_digest(digest.hexdigest(), record.get("sha256", "")):
            raise ValueError(f"backup payload hash mismatch: {relative}")
        if path.stat().st_size != record.get("size"):
            raise ValueError(f"backup payload size mismatch: {relative}")
    actual = {p.relative_to(bundle / "files").as_posix() for p in (bundle / "files").rglob("*") if p.is_file()}
    if actual != paths:
        raise ValueError("backup payload file set differs from authenticated manifest")
    return manifest


def _fsync_directory(path: Path) -> None:
    descriptor = os.open(path, os.O_RDONLY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def migrate_files(bundle: Path | str, key_file: Path | str, destination: Path | str, replace: bool = False) -> dict:
    bundle, destination = Path(bundle).resolve(), Path(destination).resolve()
    manifest = verify_bundle(bundle, key_file)
    if destination.exists() and not destination.is_dir():
        raise ValueError(f"migration destination is not a directory: {destination}")
    has_contents = destination.exists() and any(destination.iterdir())
    if has_contents and not replace:
        raise ValueError(f"migration destination is not empty (use replace to retain a rollback): {destination}")
    destination.parent.mkdir(parents=True, exist_ok=True)
    staging = Path(tempfile.mkdtemp(prefix=f".{destination.name}.restore-", dir=destination.parent))
    rollback = None
    try:
        _copy_tree_contents(bundle / "files", staging)
        if has_contents:
            rollback = Path(tempfile.mkdtemp(prefix=f".{destination.name}.rollback-", dir=destination.parent))
            rollback.rmdir()
            os.replace(destination, rollback)
        elif destination.exists():
            destination.rmdir()
        os.replace(staging, destination)
        _fsync_directory(destination.parent)
    except Exception:
        if rollback is not None and rollback.exists() and not destination.exists():
            os.replace(rollback, destination)
        shutil.rmtree(staging, ignore_errors=True)
        raise
    journal = {
        "schema": "wasl-file-migration-v1",
        "destination": str(destination),
        "rollback_path": str(rollback) if rollback is not None else None,
        "installed_path": None,
        "bundle_hmac_sha256": manifest["hmac_sha256"],
        "completed_at": datetime.now(timezone.utc).isoformat(),
    }
    journal_path = destination.parent / f".{destination.name}.migration.json"
    temporary = destination.parent / f".{destination.name}.migration.json.tmp"
    temporary.write_bytes(_canonical(journal) + b"\n")
    os.chmod(temporary, 0o600)
    os.replace(temporary, journal_path)
    _fsync_directory(destination.parent)
    journal["journal_path"] = str(journal_path)
    return journal


def rollback_files(journal: dict) -> None:
    if journal.get("schema") != "wasl-file-migration-v1" or journal.get("installed_path"):
        raise ValueError("invalid or already reversed migration journal")
    destination = Path(journal["destination"])
    rollback = Path(journal["rollback_path"]) if journal.get("rollback_path") else None
    if rollback is None or not rollback.is_dir():
        raise ValueError("migration has no retained rollback generation")
    rejected = destination.parent / f".{destination.name}.rejected"
    if rejected.exists():
        raise ValueError(f"rollback rejected-generation path already exists: {rejected}")
    os.replace(destination, rejected)
    try:
        os.replace(rollback, destination)
    except Exception:
        os.replace(rejected, destination)
        raise
    journal["installed_path"] = str(rejected)
    journal_path = Path(journal.get("journal_path", destination.parent / f".{destination.name}.migration.json"))
    temporary = journal_path.with_suffix(".tmp")
    temporary.write_bytes(_canonical(journal) + b"\n")
    os.replace(temporary, journal_path)
    _fsync_directory(destination.parent)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("backup", "verify", "migrate"):
        command = sub.add_parser(name)
        command.add_argument("path", type=Path, help="source for backup; bundle for verify/migrate")
        command.add_argument("--key-file", type=Path, required=True)
        if name == "backup":
            command.add_argument("bundle", type=Path)
            command.add_argument("--nats-json", type=Path)
        if name == "migrate":
            command.add_argument("destination", type=Path)
            command.add_argument("--replace", action="store_true")
    rollback = sub.add_parser("rollback")
    rollback.add_argument("journal", type=Path)
    args = parser.parse_args()
    if args.command == "backup":
        result = write_file_bundle(args.path, args.bundle, args.key_file, args.nats_json)
    elif args.command == "verify":
        result = verify_bundle(args.path, args.key_file)
    elif args.command == "migrate":
        result = migrate_files(args.path, args.key_file, args.destination, args.replace)
    else:
        result = rollback_files(json.loads(args.journal.read_text()))
    print(json.dumps({"status": "ok", "command": args.command, "result": result}, sort_keys=True, default=str))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as exc:
        raise SystemExit(f"recovery refused: {exc}")
