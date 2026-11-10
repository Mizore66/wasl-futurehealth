#!/usr/bin/env python3
"""Encrypted, authenticated file bundles and transactional, reversible file-generation migration.

New bundles are always `wasl-file-backup-v2-aesgcm`: every payload is AES-256-GCM encrypted under a per-bundle data
key that is wrapped by the active generation of a versioned keyring stored outside the bundle, and the manifest is
HMAC-authenticated with that generation's independent key. Verification always completes before a migration renames
or writes the destination. No plaintext or key material is included in the bundle. Legacy v1 (HMAC-only) bundles are
read-only and are accepted solely with an explicit `legacy_hmac` opt-in.
"""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import hmac
import json
import os
from pathlib import Path
import re
import shutil
import stat
import sys
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parent))

from recovery_crypto import (  # noqa: E402
    _b64,
    _check_key_id,
    _entry_keys,
    _no_duplicates,
    _unb64,
    decrypt_payload,
    encrypt_payload,
    load_keyring,
    new_dek,
    rotate_keyring,
    unwrap_dek,
    wrap_dek,
)

SCHEMA = "wasl-file-backup-v1"  # legacy, read-only
SCHEMA_V2 = "wasl-file-backup-v2-aesgcm"
FILE_AAD_PREFIX = SCHEMA_V2.encode() + b"|file|"
TAG_BYTES = 16
_HEX64 = re.compile(r"[0-9a-f]{64}")
_MANIFEST_FIELDS = {"schema", "key_id", "created_at", "nondeterministic_fields", "wrapped_dek", "wrap_nonce",
                    "files", "hmac_sha256"}
_RECORD_FIELDS = {"path", "size", "sha256", "nonce", "ciphertext_sha256"}


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


NONDETERMINISTIC = ["created_at", "file_mtime", "nats_sequence", "nats_time"]
_UNSAFE = "backup manifest contains an unsafe or duplicate path"


def _safe_path(value) -> str:
    """A relative POSIX path with no empty, '.' or '..' components, backslashes or control characters."""
    if (not isinstance(value, str) or not value or len(value) > 1024 or "\\" in value
            or any(ord(char) < 32 for char in value)):
        raise ValueError(_UNSAFE)
    if any(part in ("", ".", "..") for part in value.split("/")):
        raise ValueError(_UNSAFE)
    return value


def _check_path_set(paths: list[str]) -> None:
    seen = set()
    for path in paths:
        _safe_path(path)
        if path in seen:
            raise ValueError(_UNSAFE)
        seen.add(path)
    for path in paths:
        parts = path.split("/")
        if any("/".join(parts[:i]) in seen for i in range(1, len(parts))):
            raise ValueError(_UNSAFE)  # a file may not also be a directory


def _write_exclusive(path: Path, data: bytes, mode: int = 0o600) -> None:
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
    descriptor = os.open(path, flags, mode)
    with os.fdopen(descriptor, "wb") as handle:
        handle.write(data)
        handle.flush()
        os.fchmod(handle.fileno(), mode)
        os.fsync(handle.fileno())


def _source_files(source: Path) -> list[tuple[str, Path]]:
    found = []
    for root, dirs, files in os.walk(source):
        dirs.sort()
        files.sort()
        root_path = Path(root)
        for directory in dirs:
            if (root_path / directory).is_symlink():
                raise ValueError(f"symbolic links are not supported in recovery bundles: {root_path / directory}")
        for name in files:
            path = root_path / name
            if path.is_symlink() or not stat.S_ISREG(path.lstat().st_mode):
                raise ValueError(f"only regular files are supported in recovery bundles: {path}")
            found.append((path.relative_to(source).as_posix(), path))
    return found


def _file_aad(key_id: str, path: str, size: int, sha256: str) -> bytes:
    """Unambiguous binding of schema, key id, path, plaintext size and plaintext hash (canonical JSON)."""
    return FILE_AAD_PREFIX + _canonical(
        {"schema": SCHEMA_V2, "key_id": key_id, "path": path, "size": size, "sha256": sha256})


def _write_manifest(bundle: Path, manifest: dict) -> None:
    encoded = _canonical(manifest) + b"\n"
    temporary = bundle / ".manifest.json.tmp"
    _write_exclusive(temporary, encoded)
    os.replace(temporary, bundle / "manifest.json")


def write_file_bundle(source: Path | str, bundle: Path | str, key_file: Path | str,
                      nats_json: Path | str | None = None) -> dict:
    """Write a new encrypted v2 bundle. Reads the keyring's active generation only."""
    source, bundle = Path(source), Path(bundle)
    _, key_id, entry = load_keyring(Path(key_file))
    _, auth_key = _entry_keys(entry)
    if not source.is_dir():
        raise ValueError(f"backup source is not a directory: {source}")
    if bundle.exists() or bundle.is_symlink():
        raise ValueError(f"backup destination already exists: {bundle}")
    items = _source_files(source)
    if nats_json is not None:
        nats_source = Path(nats_json)
        if nats_source.is_symlink() or not nats_source.is_file():
            raise ValueError(f"NATS backup payload is not a regular file: {nats_source}")
        items.append(("nats/nats.json", nats_source))
    items.sort(key=lambda item: item[0])
    _check_path_set([relative for relative, _ in items])
    bundle.parent.mkdir(parents=True, exist_ok=True)
    dek = new_dek()
    wrap_nonce, wrapped = wrap_dek(entry, key_id, dek)
    staging = Path(tempfile.mkdtemp(prefix=f".{bundle.name}.staging-", dir=bundle.parent))
    try:
        (staging / "files").mkdir(mode=0o700)
        records = []
        for relative, path in items:
            plaintext = path.read_bytes()
            sha256 = hashlib.sha256(plaintext).hexdigest()
            nonce, ciphertext = encrypt_payload(dek, _file_aad(key_id, relative, len(plaintext), sha256), plaintext)
            target = staging / "files" / (relative + ".aead")
            target.parent.mkdir(parents=True, exist_ok=True)
            _write_exclusive(target, ciphertext)
            records.append({"path": relative, "size": len(plaintext), "sha256": sha256, "nonce": _b64(nonce),
                            "ciphertext_sha256": hashlib.sha256(ciphertext).hexdigest()})
        manifest = {
            "schema": SCHEMA_V2,
            "key_id": key_id,
            "created_at": datetime.now(timezone.utc).isoformat(),
            "nondeterministic_fields": NONDETERMINISTIC,
            "wrapped_dek": wrapped,
            "wrap_nonce": wrap_nonce,
            "files": records,
        }
        manifest["hmac_sha256"] = _manifest_mac(manifest, auth_key)
        _write_manifest(staging, manifest)
        _fsync_directory(staging)
        os.replace(staging, bundle)
        _fsync_directory(bundle.parent)
        return manifest
    except BaseException:
        shutil.rmtree(staging, ignore_errors=True)
        raise


def _read_manifest(bundle: Path) -> dict:
    path = bundle / "manifest.json"
    try:
        if path.is_symlink() or not path.is_file():
            raise OSError("not a regular file")
        doc = json.loads(path.read_bytes().decode("utf-8"), object_pairs_hook=_no_duplicates)
    except (OSError, ValueError):  # includes JSON, UTF-8 and duplicate-key errors
        raise ValueError("backup manifest unavailable or invalid") from None
    if not isinstance(doc, dict):
        raise ValueError("backup manifest unavailable or invalid")
    return doc


def _v2_check_manifest(keyring, manifest) -> tuple[bytes, list[dict]]:
    """Authenticate a v2 manifest against the keyring and return (data key, validated file records)."""
    if not isinstance(manifest, dict) or set(manifest) != _MANIFEST_FIELDS:
        raise ValueError("backup manifest is malformed")
    if manifest["schema"] != SCHEMA_V2:
        raise ValueError("unsupported backup manifest schema")
    key_id = _check_key_id(manifest["key_id"])
    generations = keyring.get("keys") if isinstance(keyring, dict) else None
    entry = generations.get(key_id) if isinstance(generations, dict) else None
    if entry is None:
        raise ValueError("recovery key generation for this bundle is not in the keyring")
    _, auth_key = _entry_keys(entry)
    mac = manifest["hmac_sha256"]
    if not isinstance(mac, str) or not _HEX64.fullmatch(mac) or not hmac.compare_digest(
            mac, _manifest_mac(manifest, auth_key)):
        raise ValueError("backup authentication failed: wrong key or modified manifest")
    if not isinstance(manifest["created_at"], str) or manifest["nondeterministic_fields"] != NONDETERMINISTIC:
        raise ValueError("backup manifest is malformed")
    records = manifest["files"]
    if not isinstance(records, list):
        raise ValueError("backup manifest is malformed")
    nonces = set()
    for record in records:
        if not isinstance(record, dict) or set(record) != _RECORD_FIELDS:
            raise ValueError("backup manifest file record is malformed")
        size = record["size"]
        if type(size) is not int or size < 0:
            raise ValueError("backup manifest file record is malformed")
        for name in ("sha256", "ciphertext_sha256"):
            if not isinstance(record[name], str) or not _HEX64.fullmatch(record[name]):
                raise ValueError("backup manifest file record is malformed")
        nonce = _unb64(record["nonce"], 12, "file nonce")
        if nonce in nonces:
            raise ValueError("backup manifest reuses a file nonce")
        nonces.add(nonce)
    _check_path_set([record["path"] for record in records])
    dek = unwrap_dek(entry, key_id, manifest["wrap_nonce"], manifest["wrapped_dek"])
    return dek, records


def _v2_check_files(bundle: Path, records: list[dict]) -> None:
    """The bundle must contain exactly manifest.json and the authenticated ciphertext files, no links."""
    try:
        root_entries = set(os.listdir(bundle))
    except OSError:
        raise ValueError("backup bundle is unreadable") from None
    files_dir = bundle / "files"
    if root_entries != {"manifest.json", "files"} or files_dir.is_symlink() or not files_dir.is_dir():
        raise ValueError("backup bundle contains unexpected entries")
    expected_files = {record["path"] + ".aead" for record in records}
    expected_dirs = {"/".join(path.split("/")[:i]) for path in expected_files for i in range(1, len(path.split("/")))}
    actual_files, actual_dirs = set(), set()
    for root, dirs, files in os.walk(files_dir):
        root_path = Path(root)
        for name in dirs:
            if (root_path / name).is_symlink():
                raise ValueError("backup bundle contains a symbolic link")
            actual_dirs.add((root_path / name).relative_to(files_dir).as_posix())
        for name in files:
            path = root_path / name
            if path.is_symlink() or not stat.S_ISREG(path.lstat().st_mode):
                raise ValueError("backup bundle contains a non-regular file")
            actual_files.add(path.relative_to(files_dir).as_posix())
    if actual_files != expected_files or actual_dirs != expected_dirs:
        raise ValueError("backup payload file set differs from authenticated manifest")


def _v2_read_payload(bundle: Path, dek: bytes, key_id: str, record: dict) -> bytes:
    relative = record["path"]
    try:
        ciphertext = (bundle / "files" / (relative + ".aead")).read_bytes()
    except OSError:
        raise ValueError(f"backup payload missing: {relative}") from None
    if len(ciphertext) != record["size"] + TAG_BYTES:
        raise ValueError(f"backup payload truncated or wrong size: {relative}")
    if not hmac.compare_digest(hashlib.sha256(ciphertext).hexdigest(), record["ciphertext_sha256"]):
        raise ValueError(f"backup payload hash mismatch: {relative}")
    aad = _file_aad(key_id, relative, record["size"], record["sha256"])
    try:
        plaintext = decrypt_payload(dek, aad, _unb64(record["nonce"], 12, "file nonce"), ciphertext)
    except ValueError:
        raise ValueError(f"backup payload decryption failed authentication: {relative}") from None
    if len(plaintext) != record["size"] or not hmac.compare_digest(
            hashlib.sha256(plaintext).hexdigest(), record["sha256"]):
        raise ValueError(f"backup payload plaintext does not match its authenticated size or hash: {relative}")
    return plaintext


def _verify_v1(bundle: Path, key_file: Path, manifest: dict) -> dict:
    key = _read_key(key_file)
    if not isinstance(manifest.get("files"), list):
        raise ValueError("unsupported or incomplete backup manifest")
    if not hmac.compare_digest(str(manifest.get("hmac_sha256", "")), _manifest_mac(manifest, key)):
        raise ValueError("backup authentication failed: wrong key or modified manifest")
    paths = set()
    for record in manifest["files"]:
        if not isinstance(record, dict):
            raise ValueError("backup manifest contains an unsafe or duplicate path")
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
        if not hmac.compare_digest(digest.hexdigest(), str(record.get("sha256", ""))):
            raise ValueError(f"backup payload hash mismatch: {relative}")
        if path.stat().st_size != record.get("size"):
            raise ValueError(f"backup payload size mismatch: {relative}")
    actual = {p.relative_to(bundle / "files").as_posix() for p in (bundle / "files").rglob("*") if p.is_file()}
    if actual != paths:
        raise ValueError("backup payload file set differs from authenticated manifest")
    return manifest


def verify_bundle(bundle: Path | str, key_file: Path | str, legacy_hmac: bool = False) -> dict:
    """Verify a bundle completely. v2 needs a keyring; legacy v1 needs the HMAC key AND legacy_hmac=True."""
    bundle = Path(bundle)
    manifest = _read_manifest(bundle)
    schema = manifest.get("schema")
    if schema == SCHEMA:
        if not legacy_hmac:
            raise ValueError("legacy HMAC bundle refused: pass --legacy-hmac to use it explicitly")
        return _verify_v1(bundle, Path(key_file), manifest)
    if legacy_hmac:
        raise ValueError("--legacy-hmac only applies to legacy v1 bundles")
    if schema != SCHEMA_V2:
        raise ValueError("unsupported backup manifest schema")
    keyring, _, _ = load_keyring(Path(key_file))
    dek, records = _v2_check_manifest(keyring, manifest)
    _v2_check_files(bundle, records)
    for record in records:
        _v2_read_payload(bundle, dek, manifest["key_id"], record)
    return manifest


def _empty_directory(directory: Path) -> None:
    for entry in os.scandir(directory):
        try:
            if entry.is_dir(follow_symlinks=False):
                shutil.rmtree(entry.path, ignore_errors=True)
            else:
                os.unlink(entry.path)
        except OSError:
            pass


def _decrypt_v2_payloads(bundle: Path | str, keyring: dict, manifest: dict, destination: Path | str) -> None:
    """Decrypt an authenticated v2 bundle into a caller-provided empty mode-0700 directory.

    The manifest is re-authenticated and the exact file set re-checked here, so a forged manifest cannot reach the
    filesystem. On any error the staging directory is emptied again (this is not physical erasure).
    """
    bundle, destination = Path(bundle), Path(destination)
    try:
        info = destination.lstat()
    except OSError:
        raise ValueError("decryption staging directory is unavailable") from None
    if not stat.S_ISDIR(info.st_mode):
        raise ValueError("decryption staging path is not a directory")
    if stat.S_IMODE(info.st_mode) != 0o700 or info.st_uid != os.getuid():
        raise ValueError("decryption staging directory must be owned by this user with mode 0700")
    if any(destination.iterdir()):
        raise ValueError("decryption staging directory must be empty")
    dek, records = _v2_check_manifest(keyring, manifest)
    _v2_check_files(bundle, records)
    try:
        made = [destination]
        for record in records:
            plaintext = _v2_read_payload(bundle, dek, manifest["key_id"], record)
            parts = record["path"].split("/")
            current = destination
            for part in parts[:-1]:
                current = current / part
                if not current.exists():
                    os.mkdir(current, 0o700)
                    os.chmod(current, 0o700)
                    made.append(current)
            _write_exclusive(current / parts[-1], plaintext)
        for directory in reversed(made):
            _fsync_directory(directory)
    except BaseException:
        _empty_directory(destination)
        raise


def _fsync_directory(path: Path) -> None:
    descriptor = os.open(path, os.O_RDONLY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def migrate_files(bundle: Path | str, key_file: Path | str, destination: Path | str, replace: bool = False,
                  legacy_hmac: bool = False) -> dict:
    bundle, destination = Path(bundle).resolve(), Path(destination).resolve()
    if _read_manifest(bundle).get("schema") == SCHEMA_V2:
        # Encrypted restore is added with the transactional installer; never copy ciphertext as if it were plaintext.
        raise ValueError("restoring encrypted v2 bundles is not supported by this command yet")
    manifest = verify_bundle(bundle, key_file, legacy_hmac=legacy_hmac)
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
        if name in ("verify", "migrate"):
            command.add_argument("--legacy-hmac", action="store_true",
                                 help="explicitly accept a legacy HMAC-only v1 bundle (key file is the old HMAC key)")
        if name == "migrate":
            command.add_argument("destination", type=Path)
            command.add_argument("--replace", action="store_true")
    rotate = sub.add_parser("rotate-key")
    rotate.add_argument("--key-file", type=Path, required=True)
    rollback = sub.add_parser("rollback")
    rollback.add_argument("journal", type=Path)
    args = parser.parse_args()
    if args.command == "backup":
        result = write_file_bundle(args.path, args.bundle, args.key_file, args.nats_json)
    elif args.command == "verify":
        result = verify_bundle(args.path, args.key_file, args.legacy_hmac)
    elif args.command == "rotate-key":
        print(rotate_keyring(args.key_file))  # only the non-secret new generation id
        return
    elif args.command == "migrate":
        result = migrate_files(args.path, args.key_file, args.destination, args.replace, args.legacy_hmac)
    else:
        result = rollback_files(json.loads(args.journal.read_text()))
    print(json.dumps({"status": "ok", "command": args.command, "result": result}, sort_keys=True, default=str))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as exc:
        raise SystemExit(f"recovery refused: {exc}")
