#!/usr/bin/env python3
"""Generation catalog and conservative ciphertext retention for the Wasl recovery scheduler.

This module operates on an already-mounted dedicated backup root and evidence directory. It uses only the Python
standard library plus the existing recovery.py implementation. Completed generations are immutable; mutable catalog
and verification records live outside the backup generation. No command accepts or prints secret key material.
"""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parent))
from recovery import SCHEMA_V2, verify_bundle  # noqa: E402

SCHEMA = "wasl-recovery-generation-v1"
GOOD_SCHEMA = "wasl-recovery-verified-good-v1"
EVIDENCE_SCHEMA = "wasl-recovery-evidence-v1"
INTENT_SCHEMA = "wasl-recovery-retention-intent-v1"
RESULT_SCHEMA = "wasl-recovery-retention-result-v1"
ID = re.compile(r"[a-z0-9][a-z0-9-]{0,79}")
HEX40 = re.compile(r"[0-9a-f]{40}")
HEX64 = re.compile(r"[0-9a-f]{64}")
NAMESPACE = Path("scheduler-v1/generations")


def canonical(value) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")


def now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        while chunk := handle.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def fsync_dir(path: Path) -> None:
    descriptor = os.open(path, os.O_RDONLY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def write_exclusive(path: Path, value, mode: int = 0o600) -> None:
    data = value if isinstance(value, bytes) else canonical(value) + b"\n"
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
    descriptor = os.open(path, flags, mode)
    with os.fdopen(descriptor, "wb") as handle:
        handle.write(data)
        handle.flush()
        os.fchmod(handle.fileno(), mode)
        os.fsync(handle.fileno())
    fsync_dir(path.parent)


def read_json(path: Path) -> dict:
    if path.is_symlink() or not path.is_file():
        raise ValueError(f"required regular JSON file is unavailable: {path.name}")
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"JSON record is not an object: {path.name}")
    return value


def check_id(value: str, label: str = "generation") -> str:
    if not isinstance(value, str) or not ID.fullmatch(value):
        raise ValueError(f"unsafe {label} identifier")
    return value


def parse_time(value: str) -> datetime:
    if not isinstance(value, str):
        raise ValueError("timestamp must be a string")
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ValueError("timestamp lacks a timezone")
    return parsed.astimezone(timezone.utc)


def evidence_manifest(run_dir: Path) -> dict:
    """Validate the final evidence manifest and every file it authenticates."""
    manifest = read_json(run_dir / "evidence-manifest.json")
    if manifest.get("schema") != EVIDENCE_SCHEMA or not isinstance(manifest.get("files"), list):
        raise ValueError("invalid evidence manifest")
    seen = set()
    for record in manifest["files"]:
        if not isinstance(record, dict) or set(record) != {"path", "size", "sha256"}:
            raise ValueError("invalid evidence manifest file record")
        relative = record["path"]
        if (not isinstance(relative, str) or not relative or relative.startswith("/")
                or any(part in ("", ".", "..") for part in relative.split("/")) or relative in seen):
            raise ValueError("unsafe evidence path")
        seen.add(relative)
        path = run_dir / relative
        if path.is_symlink() or not path.is_file() or path.stat().st_size != record["size"]:
            raise ValueError("evidence file size or type mismatch")
        if not isinstance(record["sha256"], str) or not HEX64.fullmatch(record["sha256"]):
            raise ValueError("invalid evidence digest")
        if sha256(path) != record["sha256"]:
            raise ValueError("evidence file digest mismatch")
    actual = {p.relative_to(run_dir).as_posix() for p in run_dir.rglob("*")
              if p.is_file() and p.name != "evidence-manifest.json"}
    if actual != seen:
        raise ValueError("evidence file set differs from its manifest")
    return manifest


class Catalog:
    def __init__(self, backup_root: Path | str, state_dir: Path | str, key_file: Path | str | None = None,
                 host_uid: int | None = None, host_gid: int | None = None,
                 service_uid: int | None = None, service_gid: int | None = None):
        self.root = Path(backup_root)
        self.state = Path(state_dir)
        self.key_file = Path(key_file) if key_file is not None else None
        self.host_uid = host_uid
        self.host_gid = host_gid
        self.service_uid = service_uid
        self.service_gid = service_gid
        self.generations = self.root / NAMESPACE

    def _chown(self, path: Path, uid: int, gid: int) -> None:
        if (path.stat().st_uid, path.stat().st_gid) != (uid, gid):
            os.chown(path, uid, gid)

    def require_init(self) -> None:
        if not (self.state / "initialized.json").is_file() or not self.generations.is_dir():
            raise ValueError("recovery scheduler catalog is not initialized")

    def init(self) -> dict:
        for path, label in ((self.root, "backup root"), (self.state, "evidence directory")):
            if path.is_symlink() or not path.is_dir():
                raise ValueError(f"{label} must be a real directory")
            entries = set(os.listdir(path))
            allowed = {"run.lock", "serve.lock", "init"} if path == self.state else set()
            if entries - allowed:
                raise ValueError(f"{label} must be new and empty")
        self.generations.mkdir(parents=True, mode=0o700)
        if self.service_uid is not None:
            self._chown(self.root / "scheduler-v1", self.service_uid, self.service_gid)
            self._chown(self.generations, self.service_uid, self.service_gid)
        for name in ("runs", "catalog", "work", "slots/backup", "slots/verify"):
            (self.state / name).mkdir(parents=True, mode=0o700)
        if self.host_uid is not None:
            for path in self.state.rglob("*"):
                if path.is_dir() and path.name != "init":
                    self._chown(path, self.host_uid, self.host_gid)
        record = {"schema": "wasl-recovery-scheduler-state-v1", "created_at": now_iso()}
        write_exclusive(self.state / "initialized.json", record)
        fsync_dir(self.generations)
        fsync_dir(self.root / "scheduler-v1")
        fsync_dir(self.root)
        return record

    def generation_dir(self, generation: str) -> Path:
        check_id(generation)
        return self.generations / generation

    def bundle(self, generation: str) -> Path:
        return self.generation_dir(generation) / "bundle"

    def prepare(self, generation: str) -> dict:
        self.require_init()
        path = self.generation_dir(generation)
        path.mkdir(mode=0o700)  # exclusive; deterministic slots can never reuse a partial generation
        if self.service_uid is not None:
            self._chown(path, self.service_uid, self.service_gid)  # the existing recovery service identity
        fsync_dir(path.parent)
        return {"generation": generation, "path": str(path)}

    def authenticate(self, generation: str) -> tuple[dict, str]:
        if self.key_file is None:
            raise ValueError("a recovery keyring is required")
        bundle = self.bundle(generation)
        manifest = verify_bundle(bundle, self.key_file)
        if manifest.get("schema") != SCHEMA_V2:
            raise ValueError("only encrypted v2 generations are supported")
        manifest_path = bundle / "manifest.json"
        digest = sha256(manifest_path)
        record_path = self.generation_dir(generation) / "generation.json"
        if record_path.exists():
            record = read_json(record_path)
            if record.get("manifest_sha256") != digest:
                raise ValueError("generation manifest differs from its immutable catalog record")
        return manifest, digest

    def register(self, generation: str, slot: str, revision: str, attempt_id: str) -> dict:
        self.require_init()
        check_id(attempt_id, "attempt")
        if not HEX40.fullmatch(revision):
            raise ValueError("source revision must be a full commit")
        manifest, digest = self.authenticate(generation)
        directory = self.generation_dir(generation)
        if set(os.listdir(directory)) != {"bundle"}:
            raise ValueError("generation directory contains unexpected or already completed records")
        record = {
            "schema": SCHEMA,
            "generation": generation,
            "slot": slot,
            "source_revision": revision,
            "backup_attempt_id": attempt_id,
            "created_at": manifest["created_at"],
            "manifest_sha256": digest,
            "bundle_schema": SCHEMA_V2,
        }
        write_exclusive(directory / "generation.json", record)
        fsync_dir(directory)
        return record

    def record(self, generation: str) -> dict:
        self.require_init()
        record = read_json(self.generation_dir(generation) / "generation.json")
        required = {"schema", "generation", "slot", "source_revision", "backup_attempt_id", "created_at",
                    "manifest_sha256", "bundle_schema"}
        if (set(record) != required or record["schema"] != SCHEMA or record["generation"] != generation
                or record["bundle_schema"] != SCHEMA_V2 or not HEX64.fullmatch(record["manifest_sha256"])
                or not HEX40.fullmatch(record["source_revision"])):
            raise ValueError("invalid generation catalog record")
        check_id(record["backup_attempt_id"], "attempt")
        parse_time(record["created_at"])
        return record

    def _published_target(self) -> str | None:
        link = self.root / "bundle"
        if not link.is_symlink():
            if link.exists() or link.is_symlink():
                raise ValueError("published bundle path is not a symlink")
            return None
        target = os.readlink(link)
        parts = Path(target).parts
        if (Path(target).is_absolute() or len(parts) != 4 or parts[:2] != ("scheduler-v1", "generations")
                or parts[3] != "bundle"):
            raise ValueError("published bundle symlink leaves the scheduler namespace")
        check_id(parts[2])
        return parts[2]

    def publish(self, generation: str) -> dict:
        self.require_init()
        self.record(generation)
        self.authenticate(generation)
        current = self._published_target()
        if current == generation:
            raise ValueError("generation is already published")
        relative = (NAMESPACE / generation / "bundle").as_posix()
        temporary = self.root / f".bundle-{os.getpid()}-{next(tempfile._get_candidate_names())}.tmp"
        os.symlink(relative, temporary)
        try:
            os.replace(temporary, self.root / "bundle")
            fsync_dir(self.root)
        finally:
            if temporary.is_symlink():
                temporary.unlink()
        return {"published": generation, "target": relative}

    def resolve(self, selected: str, authenticate: bool = True) -> dict:
        self.require_init()
        generation = self._published_target() if selected == "latest" else check_id(selected)
        if generation is None:
            raise ValueError("no published backup generation is available")
        record = self.record(generation)
        if authenticate:
            self.authenticate(generation)
            backup_run = self.state / "runs" / record["backup_attempt_id"]
            started = read_json(backup_run / "started.json")
            result = read_json(backup_run / "result.json")
            if (started.get("generation") != generation or started.get("kind") != "backup"
                    or result.get("status") != "success"):
                raise ValueError("selected generation lacks successful immutable backup evidence")
            evidence_manifest(backup_run)
        return {"generation": generation, "bundle": str(self.bundle(generation)), "record": record}

    def export_manifest(self, generation: str, output: Path) -> dict:
        record = self.record(generation)
        self.authenticate(generation)
        output = output if output.is_absolute() else self.state / output
        output.relative_to(self.state)  # evidence exports cannot leave state_dir
        if ".." in output.parts:
            raise ValueError("unsafe evidence export path")
        if not output.parent.is_dir():
            raise ValueError("evidence export parent is absent")
        source = self.bundle(generation) / "manifest.json"
        data = source.read_bytes()
        if hashlib.sha256(data).hexdigest() != record["manifest_sha256"]:
            raise ValueError("exported manifest digest mismatch")
        write_exclusive(output, data)
        return {"generation": generation, "manifest_sha256": record["manifest_sha256"], "output": str(output)}

    def mark_good(self, generation: str, attempt_id: str) -> dict:
        self.require_init()
        check_id(attempt_id, "attempt")
        record = self.record(generation)
        self.authenticate(generation)
        run_dir = self.state / "runs" / attempt_id
        started = read_json(run_dir / "started.json")
        result = read_json(run_dir / "result.json")
        comparison = read_json(run_dir / "hash-comparison.json")
        if (started.get("attempt_id") != attempt_id or started.get("kind") != "verify"
                or started.get("generation") not in (generation, None) or result.get("status") != "success"
                or result.get("attempt_id") != attempt_id or comparison.get("ok") is not True):
            raise ValueError("verification attempt evidence is incomplete or unsuccessful")
        if started.get("generation") is None:
            selection = read_json(run_dir / "selection.json")
            if selection.get("generation") != generation:
                raise ValueError("verification selection does not match the generation")
            raise ValueError("verification attempt evidence is incomplete or unsuccessful")
        evidence_manifest(run_dir)
        source_revision = started.get("source_revision")
        if source_revision == "HEAD":
            source_revision = read_json(run_dir / "source-revision.json").get("resolved")
        if not isinstance(source_revision, str) or not HEX40.fullmatch(source_revision):
            raise ValueError("verification attempt lacks a resolved full source revision")
        catalog_dir = self.state / "catalog" / generation
        catalog_dir.mkdir(parents=True, exist_ok=True)
        good = {
            "schema": GOOD_SCHEMA,
            "generation": generation,
            "attempt_id": attempt_id,
            "verified_at": result.get("finished_at"),
            "manifest_sha256": record["manifest_sha256"],
            "evidence_manifest_sha256": sha256(run_dir / "evidence-manifest.json"),
            "source_revision": source_revision,
        }
        if not isinstance(good["verified_at"], str):
            raise ValueError("verification result lacks a completion timestamp")
        parse_time(good["verified_at"])
        write_exclusive(self.state / "catalog" / generation / "verified-good.json", good)
        return good

    def good_record(self, generation: str) -> dict | None:
        path = self.state / "catalog" / generation / "verified-good.json"
        if not path.exists():
            return None
        good = read_json(path)
        required = {"schema", "generation", "attempt_id", "verified_at", "manifest_sha256",
                    "evidence_manifest_sha256", "source_revision"}
        if (set(good) != required or good["schema"] != GOOD_SCHEMA or good["generation"] != generation
                or good["manifest_sha256"] != self.record(generation)["manifest_sha256"]):
            raise ValueError("invalid verified-good record")
        check_id(good["attempt_id"], "attempt")
        parse_time(good["verified_at"])
        run_dir = self.state / "runs" / good["attempt_id"]
        evidence_manifest(run_dir)
        if sha256(run_dir / "evidence-manifest.json") != good["evidence_manifest_sha256"]:
            raise ValueError("verified-good evidence manifest identity mismatch")
        result = read_json(run_dir / "result.json")
        comparison = read_json(run_dir / "hash-comparison.json")
        if result.get("status") != "success" or comparison.get("ok") is not True:
            raise ValueError("verified-good attempt no longer has successful comparison evidence")
        return good

    def attempt_states(self) -> dict[str, list[dict]]:
        states: dict[str, list[dict]] = {}
        runs = self.state / "runs"
        for run_dir in sorted(runs.iterdir()) if runs.is_dir() else []:
            if not run_dir.is_dir() or run_dir.is_symlink():
                states.setdefault("unknown", []).append({"attempt_id": run_dir.name, "state": "unknown"})
                continue
            try:
                started = read_json(run_dir / "started.json")
                generation = started.get("generation")
                check_id(started.get("attempt_id", ""), "attempt")
                if started["attempt_id"] != run_dir.name:
                    raise ValueError("attempt directory mismatch")
                result_path = run_dir / "result.json"
                if not result_path.exists():
                    state = "interrupted"
                else:
                    result = read_json(result_path)
                    if result.get("attempt_id") != run_dir.name:
                        raise ValueError("result attempt mismatch")
                    evidence_manifest(run_dir)
                    result_state = result.get("status")
                    state = result_state if result_state in ("success", "failed", "interrupted") else "unknown"
                if generation is None and (run_dir / "selection.json").is_file():
                    generation = read_json(run_dir / "selection.json").get("generation")
                if generation is not None:
                    check_id(generation)
                    states.setdefault(generation, []).append({"attempt_id": run_dir.name, "state": state})
                else:
                    states.setdefault("unknown", []).append({"attempt_id": run_dir.name, "state": state})
            except (OSError, ValueError, KeyError, json.JSONDecodeError):
                states.setdefault("unknown", []).append({"attempt_id": run_dir.name, "state": "corrupted"})
        return states

    def _pruned(self, generation: str) -> bool:
        return (self.state / "catalog" / generation / "retention-result.json").is_file()

    def _reconcile_pruning(self) -> None:
        catalog_root = self.state / "catalog"
        for directory in sorted(catalog_root.iterdir()) if catalog_root.is_dir() else []:
            intent = directory / "retention-intent.json"
            result = directory / "retention-result.json"
            if intent.is_file() and not result.exists():
                record = read_json(intent)
                if record.get("schema") != INTENT_SCHEMA:
                    raise ValueError("invalid retention intent")
                self.prune(check_id(record.get("generation")), record.get("manifest_sha256", ""))

    def retention(self, max_age_days: int, keep_newest: int, minimum_good: int, now: datetime | None = None,
                  apply: bool = False) -> dict:
        self.require_init()
        if type(max_age_days) is not int or max_age_days <= 0 or not keep_newest >= minimum_good >= 1:
            raise ValueError("invalid retention policy")
        self._reconcile_pruning()
        now = now or datetime.now(timezone.utc)
        states = self.attempt_states()
        bad_global = [dict(item, generation=generation) for generation, items in states.items()
                      for item in items if item["state"] != "success"]
        if bad_global:
            return {"status": "blocked", "reason": "failed, interrupted, pending, unknown, or corrupted evidence",
                    "candidates": [], "blocked_attempts": bad_global}
        records = []
        for directory in sorted(self.generations.iterdir()):
            if not directory.is_dir() or directory.is_symlink():
                raise ValueError("unsafe entry in scheduler generation namespace")
            generation = check_id(directory.name)
            record = self.record(generation)
            good = self.good_record(generation)
            if good is not None and not self._pruned(generation):
                records.append((record, good))
        good_records = sorted(records, key=lambda item: parse_time(item[1]["verified_at"]))
        if len(good_records) < minimum_good:
            return {"status": "blocked", "reason": "fewer than the configured minimum verified-good generations remain",
                    "candidates": [], "verified_good": len(good_records)}
        protected_newest = {record["generation"] for record, _ in good_records[-keep_newest:]}
        published = self._published_target()
        candidates = []
        for record, good in good_records:
            generation = record["generation"]
            age = now - parse_time(record["created_at"])
            reasons = []
            if age.total_seconds() <= max_age_days * 86400:
                reasons.append("not older than max_age_days")
            if generation in protected_newest:
                reasons.append("within newest verified-good retention set")
            if generation == published:
                reasons.append("published generation")
            remaining = len(good_records) - len(candidates)
            if remaining <= minimum_good:
                reasons.append("minimum verified-good survivor count")
            if reasons:
                continue
            states_for_generation = states.get(generation, [])
            if any(item["state"] != "success" for item in states_for_generation):
                continue
            candidates.append({"generation": generation, "manifest_sha256": record["manifest_sha256"]})
        survivors = [record["generation"] for record, _ in good_records
                     if record["generation"] not in {c["generation"] for c in candidates}]
        if len(survivors) < minimum_good or (good_records and good_records[-1][0]["generation"] not in survivors):
            return {"status": "blocked", "reason": "newest known-good restore would not survive", "candidates": []}
        # Reauthenticate every protected survivor before planning or performing any deletion.
        for generation in survivors:
            self.authenticate(generation)
        for candidate in candidates:
            self.authenticate(candidate["generation"])
        if not apply:
            return {"status": "dry-run", "candidates": candidates, "protected_survivors": survivors}
        pruned = []
        for candidate in candidates:
            pruned.append(self.prune(candidate["generation"], candidate["manifest_sha256"]))
        return {"status": "applied", "candidates": candidates, "pruned": pruned, "protected_survivors": survivors}

    def prune(self, generation: str, expected_manifest_sha256: str) -> dict:
        """Delete only authenticated ciphertext paths for one exact generation; retain manifest and host evidence."""
        check_id(generation)
        record = self.record(generation)
        if record["manifest_sha256"] != expected_manifest_sha256:
            raise ValueError("retention expected manifest digest differs from catalog")
        directory = self.state / "catalog" / generation
        directory.mkdir(parents=True, exist_ok=True)
        intent_path = directory / "retention-intent.json"
        result_path = directory / "retention-result.json"
        if result_path.exists():
            result = read_json(result_path)
            if result.get("generation") != generation or result.get("manifest_sha256") != expected_manifest_sha256:
                raise ValueError("retention result identity mismatch")
            return result
        manifest = read_json(self.bundle(generation) / "manifest.json")
        if sha256(self.bundle(generation) / "manifest.json") != expected_manifest_sha256:
            raise ValueError("retention manifest digest mismatch")
        if intent_path.exists():
            intent = read_json(intent_path)
            if (intent.get("generation") != generation
                    or intent.get("manifest_sha256") != expected_manifest_sha256):
                raise ValueError("retention intent identity mismatch")
        else:
            intent = {"schema": INTENT_SCHEMA, "generation": generation,
                      "manifest_sha256": expected_manifest_sha256, "started_at": now_iso(),
                      "ciphertext_paths": [r["path"] + ".aead" for r in manifest["files"]]}
            write_exclusive(intent_path, intent)
        files_root = self.bundle(generation) / "files"
        for file_record in manifest["files"]:
            relative = file_record["path"] + ".aead"
            target = files_root / relative
            if not target.exists() and not target.is_symlink():
                continue  # crash reconciliation: this exact payload was already unlinked
            if target.is_symlink() or not target.is_file():
                raise ValueError("retention payload is not a regular file")
            if sha256(target) != file_record["ciphertext_sha256"]:
                raise ValueError("retention ciphertext changed after authentication; refusing deletion")
            target.unlink()
            fsync_dir(target.parent)
        # Remove only now-empty directories below files/, deepest first. The manifest remains beside files/.
        if files_root.exists():
            directories = []
            for root, dirs, files in os.walk(files_root):
                if files:
                    raise ValueError("unexpected file remains after exact-generation pruning")
                for name in dirs:
                    path = Path(root) / name
                    if path.is_symlink():
                        raise ValueError("unexpected symlink after pruning")
                    directories.append(path)
            for path in sorted(directories, key=lambda item: len(item.parts), reverse=True):
                path.rmdir()
            files_root.rmdir()
            fsync_dir(self.bundle(generation))
        result = {"schema": RESULT_SCHEMA, "generation": generation,
                  "manifest_sha256": expected_manifest_sha256, "pruned_at": now_iso(),
                  "manifest_retained": True, "evidence_retained": True}
        write_exclusive(result_path, result)
        return result


def main(argv=None) -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--backup-root", type=Path, required=True)
    parser.add_argument("--state-dir", type=Path, required=True)
    parser.add_argument("--key-file", type=Path)
    parser.add_argument("--host-uid", type=int)
    parser.add_argument("--host-gid", type=int)
    parser.add_argument("--service-uid", type=int)
    parser.add_argument("--service-gid", type=int)
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("init")
    for name in ("prepare", "register", "publish", "resolve", "export", "mark-good"):
        command = sub.add_parser(name)
        command.add_argument("--generation", required=True)
        if name == "register":
            command.add_argument("--slot", required=True)
            command.add_argument("--revision", required=True)
            command.add_argument("--attempt-id", required=True)
        if name == "resolve":
            command.add_argument("--no-authenticate", action="store_true")
        if name == "export":
            command.add_argument("--output", type=Path, required=True)
        if name == "mark-good":
            command.add_argument("--attempt-id", required=True)
    retention = sub.add_parser("retention")
    retention.add_argument("--max-age-days", type=int, required=True)
    retention.add_argument("--keep-newest", type=int, required=True)
    retention.add_argument("--minimum-good", type=int, required=True)
    retention.add_argument("--apply", action="store_true")
    retention.add_argument("--now", help="UTC ISO timestamp, for deterministic review")
    args = parser.parse_args(argv)
    if (args.host_uid is None) != (args.host_gid is None):
        parser.error("--host-uid and --host-gid must be supplied together")
    if (args.service_uid is None) != (args.service_gid is None):
        parser.error("--service-uid and --service-gid must be supplied together")
    catalog = Catalog(args.backup_root, args.state_dir, args.key_file, args.host_uid, args.host_gid,
                      args.service_uid, args.service_gid)
    if args.command == "init":
        result = catalog.init()
    elif args.command == "prepare":
        result = catalog.prepare(args.generation)
    elif args.command == "register":
        result = catalog.register(args.generation, args.slot, args.revision, args.attempt_id)
    elif args.command == "publish":
        result = catalog.publish(args.generation)
    elif args.command == "resolve":
        result = catalog.resolve(args.generation, not args.no_authenticate)
    elif args.command == "export":
        result = catalog.export_manifest(args.generation, args.output)
    elif args.command == "mark-good":
        result = catalog.mark_good(args.generation, args.attempt_id)
    else:
        parsed_now = parse_time(args.now) if args.now else None
        result = catalog.retention(args.max_age_days, args.keep_newest, args.minimum_good, parsed_now, args.apply)
    print(json.dumps({"status": "ok", "result": result}, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        print(f"recovery catalog refused: {exc}", file=sys.stderr)
        raise SystemExit(1)
