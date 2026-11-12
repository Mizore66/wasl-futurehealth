#!/usr/bin/env python3
"""Operator-run scheduled encrypted backup and isolated clean-target recovery verification.

No host schedule is installed. `serve` is a foreground loop; `run-due`, `verify`, and `retention` are one-shot commands.
Every automatic slot has at most one attempt. Every verification uses a unique Compose project and new data volumes.
Only Python's standard library and the repository's existing recovery commands are used.
"""
from __future__ import annotations

import argparse
from dataclasses import dataclass
from datetime import datetime, timezone
import fcntl
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import signal
import socket
import subprocess
import sys
import tempfile
import time
from typing import Callable
from urllib.parse import urlsplit

sys.path.insert(0, str(Path(__file__).resolve().parent))
from recovery_catalog import (  # noqa: E402
    EVIDENCE_SCHEMA,
    canonical,
    check_id,
    fsync_dir,
    now_iso,
    parse_time,
    sha256,
    write_exclusive,
)

SCHEMA = "wasl-recovery-scheduler-v1"
ATTEMPT_SCHEMA = "wasl-recovery-attempt-v1"
RESULT_SCHEMA = "wasl-recovery-result-v1"
ID = re.compile(r"[a-z0-9][a-z0-9-]{0,47}")
HEX40 = re.compile(r"[0-9a-f]{40}")
SENSITIVE_KEYS = {"password", "passwd", "authorization", "auth_key", "wrap_key", "encryption_key", "data_key", "dek",
                  "wrapped_dek", "wrap_nonce", "nonce", "private_key", "secret", "secrets", "key_material", "plaintext"}


class ConfigError(ValueError):
    pass


class SafetyError(ValueError):
    pass


class OperationalError(RuntimeError):
    pass


class EvidenceError(RuntimeError):
    pass


class LockBusy(RuntimeError):
    pass


def positive_int(value, label: str) -> int:
    if type(value) is not int or value <= 0:
        raise ConfigError(f"{label} must be a positive integer")
    return value


def absolute_path(value, label: str) -> Path:
    if not isinstance(value, str) or not value or not Path(value).is_absolute():
        raise ConfigError(f"{label} must be an absolute path")
    return Path(value)


def disjoint(paths: list[tuple[str, Path]]) -> None:
    resolved = [(label, path.resolve(strict=False)) for label, path in paths]
    for index, (left_label, left) in enumerate(resolved):
        for right_label, right in resolved[index + 1:]:
            if left == right or left in right.parents or right in left.parents:
                raise ConfigError(f"{left_label} and {right_label} must be disjoint")


@dataclass(frozen=True)
class Config:
    path: Path
    raw: dict

    @property
    def repository(self) -> Path:
        return Path(self.raw["repository"])

    @property
    def revision(self) -> str:
        return self.raw["revision"]

    @property
    def engine(self) -> str:
        return self.raw["engine"]

    @property
    def source(self) -> dict:
        return self.raw["source"]

    @property
    def state_dir(self) -> Path:
        return Path(self.raw["state_dir"])

    @property
    def schedule(self) -> dict:
        return self.raw["schedule"]

    @property
    def retention(self) -> dict:
        return self.raw["retention"]

    @property
    def timeouts(self) -> dict:
        return self.raw["timeouts"]

    @property
    def epoch(self) -> datetime:
        return parse_time(self.schedule["epoch"])

    @classmethod
    def load(cls, path: Path | str) -> "Config":
        path = Path(path)
        try:
            raw = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, ValueError) as exc:
            raise ConfigError(f"scheduler configuration is unreadable or invalid: {exc}") from None
        if not isinstance(raw, dict) or set(raw) != {"schema", "repository", "revision", "engine", "source",
                                                       "state_dir", "schedule", "retention", "timeouts"}:
            raise ConfigError("scheduler configuration has missing or unknown top-level fields")
        if raw["schema"] != SCHEMA or raw["engine"] not in ("podman", "docker"):
            raise ConfigError("unsupported scheduler schema or container engine")
        if raw["revision"] != "HEAD" and (not isinstance(raw["revision"], str) or not HEX40.fullmatch(raw["revision"])):
            raise ConfigError("revision must be HEAD or a full lowercase commit SHA")
        repository = absolute_path(raw["repository"], "repository")
        state_dir = absolute_path(raw["state_dir"], "state_dir")
        source = raw["source"]
        if not isinstance(source, dict) or set(source) != {"project", "secrets_volume", "secrets_dir", "backup_volume",
                                                            "api_url"}:
            raise ConfigError("source configuration has missing or unknown fields")
        for key in ("project", "secrets_volume", "backup_volume"):
            if not isinstance(source[key], str) or not ID.fullmatch(source[key]):
                raise ConfigError(f"unsafe source {key}")
        secrets_dir = absolute_path(source["secrets_dir"], "source.secrets_dir")
        disjoint([("repository", repository), ("state_dir", state_dir), ("source.secrets_dir", secrets_dir)])
        try:
            parsed = urlsplit(source["api_url"])
        except (TypeError, ValueError, AttributeError):
            raise ConfigError("source API URL must be a valid HTTPS loopback URL") from None
        if parsed.scheme != "https" or parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise ConfigError("source API URL must be HTTPS without credentials, query, or fragment")
        try:
            port = parsed.port
            host = parsed.hostname
            loopback = host == "localhost" or (host is not None and ipaddress.ip_address(host).is_loopback)
        except ValueError:
            loopback, port = False, None
        if not loopback or port is None or parsed.path not in ("", "/"):
            raise ConfigError("source API URL must use an explicit loopback HTTPS address and port")
        schedule = raw["schedule"]
        if not isinstance(schedule, dict) or set(schedule) != {"epoch", "backup_interval_seconds",
                                                                 "verify_interval_seconds", "poll_seconds"}:
            raise ConfigError("schedule configuration has missing or unknown fields")
        for key in ("backup_interval_seconds", "verify_interval_seconds", "poll_seconds"):
            positive_int(schedule[key], f"schedule.{key}")
        try:
            epoch = parse_time(schedule["epoch"])
        except (TypeError, ValueError):
            raise ConfigError("schedule.epoch must be an ISO UTC timestamp") from None
        if not isinstance(schedule["epoch"], str) or epoch.utcoffset().total_seconds() != 0:
            raise ConfigError("schedule.epoch must be UTC")
        retention = raw["retention"]
        if not isinstance(retention, dict) or set(retention) != {"enabled", "max_age_days", "keep_newest_verified",
                                                                   "min_verified_good"}:
            raise ConfigError("retention configuration has missing or unknown fields")
        if type(retention["enabled"]) is not bool:
            raise ConfigError("retention.enabled must be Boolean")
        positive_int(retention["max_age_days"], "retention.max_age_days")
        keep = positive_int(retention["keep_newest_verified"], "retention.keep_newest_verified")
        minimum = positive_int(retention["min_verified_good"], "retention.min_verified_good")
        if keep < minimum:
            raise ConfigError("keep_newest_verified must be at least min_verified_good")
        timeouts = raw["timeouts"]
        if not isinstance(timeouts, dict) or set(timeouts) != {"command_seconds", "backup_seconds",
                                                                 "target_start_seconds"}:
            raise ConfigError("timeout configuration has missing or unknown fields")
        for key in timeouts:
            positive_int(timeouts[key], f"timeouts.{key}")
        return cls(path.resolve(), raw)


class FileLock:
    """A nonblocking flock; acquisition creates no attempt, catalog, target, or deletion activity."""

    def __init__(self, path: Path):
        self.path = path
        self.handle = None

    def __enter__(self):
        flags = os.O_RDWR | os.O_CREAT | getattr(os, "O_NOFOLLOW", 0)
        self.handle = os.fdopen(os.open(self.path, flags, 0o600), "r+")
        try:
            fcntl.flock(self.handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            self.handle.close()
            self.handle = None
            raise LockBusy(f"scheduler lock is held: {self.path.name}") from None
        return self

    def __exit__(self, *args):
        if self.handle is not None:
            fcntl.flock(self.handle.fileno(), fcntl.LOCK_UN)
            self.handle.close()
            self.handle = None


def atomic_json(path: Path, value: dict) -> None:
    temporary = path.parent / f".{path.name}.{os.getpid()}.{next(tempfile._get_candidate_names())}.tmp"
    write_exclusive(temporary, canonical(value) + b"\n")
    os.replace(temporary, path)
    fsync_dir(path.parent)


def load_json(path: Path) -> dict:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"JSON record is not an object: {path}")
    return value


def redact(value, key: str = ""):
    if key.lower() in SENSITIVE_KEYS:
        return "[redacted]"
    if isinstance(value, dict):
        return {k: redact(v, k) for k, v in value.items()}
    if isinstance(value, list):
        return [redact(item) for item in value]
    return value


def sanitize_output(text: str) -> str:
    """Remove key material and credentials from structured command output before it reaches evidence."""
    lines = []
    for line in text.splitlines():
        try:
            value = json.loads(line)
        except ValueError:
            # Commands in this package do not print secrets in free-form output. Avoid storing suspicious assignments.
            line = re.sub(r"(?i)(password|authorization|wrapped_dek|auth_key|private_key)=([^\s]+)",
                          r"\1=[redacted]", line)
            lines.append(line)
        else:
            lines.append(json.dumps(redact(value), sort_keys=True))
    return "\n".join(lines) + ("\n" if text.endswith("\n") else "")


@dataclass
class CommandResult:
    argv: list[str]
    returncode: int
    stdout: str
    stderr: str
    duration_seconds: float


class CommandRunner:
    """Execute and durably record nonsecret argv and sanitized stdout/stderr; never record the process environment."""

    def __init__(self, run_dir: Path):
        self.run_dir = run_dir
        self.commands = run_dir / "commands"
        self.commands.mkdir(parents=True, exist_ok=True)
        self.sequence = len(list(self.commands.glob("*-command.json")))

    def run(self, name: str, argv: list[str], timeout: int, env: dict | None = None,
            cwd: Path | None = None, check: bool = True) -> CommandResult:
        self.sequence += 1
        safe_name = re.sub(r"[^a-z0-9-]", "-", name.lower())[:60]
        prefix = f"{self.sequence:04d}-{safe_name}"
        started = now_iso()
        started_monotonic = time.monotonic()
        process = None
        try:
            process = subprocess.Popen(argv, cwd=cwd, env=env, text=True, stdout=subprocess.PIPE,
                                       stderr=subprocess.PIPE, start_new_session=True)
            stdout, stderr = process.communicate(timeout=timeout)
            returncode = process.returncode
        except subprocess.TimeoutExpired:
            returncode = 124
            if process is not None:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                stdout, stderr = process.communicate()
            else:
                stdout, stderr = "", ""
            stderr += f"\ncommand exceeded {timeout} seconds and its process group was terminated\n"
        except OSError as exc:
            returncode, stdout, stderr = 127, "", f"command could not be executed: {type(exc).__name__}\n"
        duration = time.monotonic() - started_monotonic
        stdout, stderr = sanitize_output(stdout), sanitize_output(stderr)
        stdout_path, stderr_path = self.commands / f"{prefix}.stdout", self.commands / f"{prefix}.stderr"
        write_exclusive(stdout_path, stdout.encode("utf-8", "replace"))
        write_exclusive(stderr_path, stderr.encode("utf-8", "replace"))
        record = {"schema": "wasl-recovery-command-v1", "name": name, "argv": argv, "started_at": started,
                  "finished_at": now_iso(), "duration_seconds": duration, "exit_status": returncode,
                  "stdout": stdout_path.name, "stderr": stderr_path.name}
        if cwd is not None:
            record["cwd"] = str(cwd)
        write_exclusive(self.commands / f"{prefix}-command.json", record)
        result = CommandResult(list(argv), returncode, stdout, stderr, duration)
        if check and returncode != 0:
            if returncode == 124:
                raise OperationalError(f"{name} timed out; target and failure evidence were preserved")
            raise OperationalError(f"{name} failed with exit status {returncode}; see immutable command evidence")
        return result


class Scheduler:
    def __init__(self, config: Config, now: Callable[[], datetime] | None = None,
                 runner_factory: Callable[[Path], CommandRunner] = CommandRunner):
        self.config = config
        self.state = config.state_dir
        self.now = now or (lambda: datetime.now(timezone.utc))
        self.runner_factory = runner_factory

    def require_init(self) -> None:
        if not (self.state / "initialized.json").is_file():
            raise SafetyError("scheduler state is not initialized; use a new empty evidence directory and init first")

    def runner(self, attempt: Path) -> CommandRunner:
        return self.runner_factory(attempt)

    def compose(self, project: str, checkout: Path | None = None, override: bool = False) -> list[str]:
        checkout = checkout or self.config.repository
        argv = [self.config.engine, "compose", "-f", str(checkout / "deploy/production.compose.yml")]
        if override:
            argv += ["-f", str(checkout / "deploy/recovery-verification.compose.yml")]
        return argv

    def compose_env(self, project: str, api_port: int, generation: str | None = None) -> dict:
        # Only nonsecret deployment identifiers are supplied here. Secret values stay inside the external secret volume.
        env = dict(os.environ)
        env.update({"COMPOSE_PROJECT_NAME": project,
                    "WASL_SECRETS_VOLUME": self.config.source["secrets_volume"],
                    "WASL_BACKUP_VOLUME": self.config.source["backup_volume"],
                    "WASL_API_PORT": str(api_port)})
        env.pop("WASL_RECOVERY_REPLACE", None)
        if generation is not None:
            env["WASL_RECOVERY_GENERATION"] = generation
        return env

    def catalog(self, runner: CommandRunner, action: str, generation: str | None = None,
                extra: list[str] | None = None) -> dict:
        argv = [self.config.engine, "run", "--rm", "--user", "0:0", "--network", "none",
                "--entrypoint", "python3", "-v", f"{self.config.source['backup_volume']}:/backup",
                "-v", f"{self.config.source['secrets_volume']}:/run/secrets:ro",
                "-v", f"{self.state}:/evidence", "-v", f"{self.config.repository}:/app:ro",
                "wasl:local", "/app/scripts/recovery_catalog.py", "--backup-root", "/backup",
                "--state-dir", "/evidence", "--key-file", "/run/secrets/recovery_keys.json",
                "--service-uid", "10001", "--service-gid", "10001"]
        if self.config.engine == "docker":
            # Docker writes host-root files; map evidence ownership back to the operator. Rootless Podman instead maps
            # container root to the host caller, so chowning to the numeric host UID inside Podman would be wrong.
            argv += ["--host-uid", str(os.getuid()), "--host-gid", str(os.getgid())]
        argv += [action]
        if generation is not None:
            argv += ["--generation", generation]
        argv += extra or []
        result = runner.run(f"catalog-{action}", argv, self.config.timeouts["command_seconds"])
        try:
            document = json.loads(result.stdout.strip().splitlines()[-1])
            return document["result"]
        except (ValueError, IndexError, KeyError):
            raise OperationalError(f"catalog {action} returned no valid nonsecret result") from None

    def resolve_revision(self, runner: CommandRunner) -> str:
        revision = "HEAD^{commit}" if self.config.revision == "HEAD" else self.config.revision + "^{commit}"
        result = runner.run("resolve-source-revision",
                            ["git", "-C", str(self.config.repository), "rev-parse", revision],
                            self.config.timeouts["command_seconds"])
        resolved = result.stdout.strip()
        if not HEX40.fullmatch(resolved):
            raise SafetyError("source revision did not resolve to one full commit")
        return resolved

    def _new_attempt(self, kind: str, attempt_id: str, revision: str, slot: str | None,
                     generation: str | None, target_project: str | None, automatic: bool) -> Path:
        check_id(attempt_id, "attempt")
        run_dir = self.state / "runs" / attempt_id
        run_dir.mkdir(mode=0o700)  # duplicate attempt IDs can never overwrite evidence
        started = {
            "schema": ATTEMPT_SCHEMA,
            "attempt_id": attempt_id,
            "kind": kind,
            "automatic": automatic,
            "slot": slot,
            "generation": generation,
            "target_project": target_project,
            "source_project": self.config.source["project"],
            "source_revision": revision,
            "started_at": now_iso(),
            "configuration": self.config.raw,
        }
        write_exclusive(run_dir / "started.json", started)
        write_exclusive(run_dir / "resolved-config.json", self.config.raw)
        fsync_dir(run_dir)
        return run_dir

    def _finish(self, run_dir: Path, attempt_id: str, status: str, error: str | None = None) -> None:
        result = {"schema": RESULT_SCHEMA, "attempt_id": attempt_id, "status": status,
                  "finished_at": now_iso(), "error": error}
        write_exclusive(run_dir / "result.json", result)
        self.finalize_evidence(run_dir)

    @staticmethod
    def finalize_evidence(run_dir: Path) -> None:
        records = []
        for path in sorted(run_dir.rglob("*")):
            if path.is_symlink():
                raise EvidenceError("attempt evidence contains a symbolic link")
            if not path.is_file() or path.name == "evidence-manifest.json":
                continue
            relative = path.relative_to(run_dir).as_posix()
            with path.open("rb") as handle:
                os.fsync(handle.fileno())
            records.append({"path": relative, "size": path.stat().st_size, "sha256": sha256(path)})
        manifest = {"schema": EVIDENCE_SCHEMA, "created_at": now_iso(), "files": records}
        try:
            write_exclusive(run_dir / "evidence-manifest.json", manifest)
        except OSError as exc:
            raise EvidenceError(f"evidence finalization failed: {type(exc).__name__}") from None
        fsync_dir(run_dir)

    def _event(self, event: dict) -> None:
        record = dict(event, observed_at=now_iso())
        path = self.state / "events.jsonl"
        flags = os.O_WRONLY | os.O_CREAT | os.O_APPEND | getattr(os, "O_NOFOLLOW", 0)
        with os.fdopen(os.open(path, flags, 0o600), "ab") as handle:
            handle.write(canonical(record) + b"\n")
            handle.flush()
            os.fsync(handle.fileno())
        fsync_dir(self.state)

    def _slot_path(self, kind: str, slot: str) -> Path:
        if not slot.isdigit():
            raise SafetyError("invalid schedule slot")
        return self.state / "slots" / kind / f"{slot}.json"

    def _claim_slot(self, kind: str, slot: str, attempt_id: str) -> None:
        write_exclusive(self._slot_path(kind, slot),
                        {"schema": "wasl-recovery-slot-v1", "kind": kind, "slot": slot, "attempt_id": attempt_id})

    def reconcile(self) -> None:
        """Classify crash-interrupted attempts and restore persisted source-API intent before new work."""
        for run_dir in sorted((self.state / "runs").iterdir()):
            if not run_dir.is_dir() or not (run_dir / "started.json").is_file():
                continue
            started = load_json(run_dir / "started.json")
            result_path = run_dir / "result.json"
            if result_path.exists():
                if started.get("automatic") and started.get("slot") is not None:
                    slot_path = self._slot_path(started["kind"], started["slot"])
                    if not slot_path.exists():
                        self._claim_slot(started["kind"], started["slot"], started["attempt_id"])
                if not (run_dir / "evidence-manifest.json").exists():
                    self.finalize_evidence(run_dir)
                continue
            runner = self.runner(run_dir)
            restoration_error = None
            intent_path = run_dir / "source-api-intent.json"
            if intent_path.is_file():
                intent = load_json(intent_path)
                if intent.get("should_be_running") is True:
                    try:
                        runner.run("reconcile-restore-source-api",
                                   self.compose(self.config.source["project"]) + ["up", "-d", "--wait", "api"],
                                   self.config.timeouts["target_start_seconds"],
                                   env=self.compose_env(self.config.source["project"],
                                                        urlsplit(self.config.source["api_url"]).port))
                    except OperationalError as exc:
                        restoration_error = str(exc)
                        atomic_json(self.state / "halted.json",
                                    {"schema": "wasl-recovery-scheduler-halt-v1", "reason": restoration_error,
                                     "attempt_id": started["attempt_id"], "created_at": now_iso()})
            target = started.get("target_project")
            if target:
                self.capture_target(run_dir, target, self.state / "work" / started["attempt_id"] / "checkout")
            if started.get("automatic") and started.get("slot") is not None:
                self._claim_slot(started["kind"], started["slot"], started["attempt_id"])
            error = "process restarted before a result was committed; explicit manual retry is required"
            if restoration_error:
                error += "; source API restoration failed and scheduled runs are halted"
            self._finish(run_dir, started["attempt_id"], "interrupted", error)
            self._event({"event": "attempt_interrupted", "attempt_id": started["attempt_id"]})

    def _schedule_slot(self, kind: str) -> tuple[str | None, int]:
        interval = self.config.schedule[f"{kind}_interval_seconds"]
        elapsed = int((self.now().astimezone(timezone.utc) - self.config.epoch).total_seconds())
        if elapsed < 0:
            return None, interval
        current = elapsed // interval
        path = self.state / "schedule.json"
        state = load_json(path) if path.is_file() else {}
        key = f"last_{kind}_slot"
        last = state.get(key)
        if last is None:
            last = current - 1  # slots before the first observed run are outside this scheduler's lifetime
        if type(last) is not int or current < last:
            raise SafetyError("schedule clock moved behind its persisted slot")
        if current > last + 1:
            missed = {"schema": "wasl-recovery-missed-slots-v1", "kind": kind,
                      "first_missed": last + 1, "last_missed": current - 1, "recorded_at": now_iso(),
                      "catch_up": False}
            write_exclusive(self.state / "slots" / kind / f"missed-{last + 1}-{current - 1}.json", missed)
            self._event({"event": "slots_missed", "kind": kind, "first": last + 1, "last": current - 1})
        state[key] = current
        atomic_json(path, state)
        return (str(current) if current > last else None), interval

    def run_due(self) -> list[str]:
        completed = []
        for kind in ("backup", "verify"):
            slot, _ = self._schedule_slot(kind)
            if slot is None:
                continue
            if self._slot_path(kind, slot).exists():
                continue  # a crash after slot claim is reconciled, never automatically retried
            if kind == "backup":
                attempt_id = self.backup(slot, automatic=True)
            else:
                attempt_id = self.verify("latest", slot=slot, automatic=True)
            completed.append(attempt_id)
        return completed

    def backup(self, slot: str, automatic: bool = True) -> str:
        generation = f"gen-{slot}"
        attempt_id = f"backup-{slot}"
        run_dir = self._new_attempt("backup", attempt_id, self.config.revision, slot, generation, None, automatic)
        self._claim_slot("backup", slot, attempt_id)
        runner = self.runner(run_dir)
        source_project = self.config.source["project"]
        source_port = urlsplit(self.config.source["api_url"]).port
        should_restore = False
        error = None
        revision = self.config.revision
        try:
            revision = self.resolve_revision(runner)
            write_exclusive(run_dir / "source-revision.json",
                            {"schema": "wasl-recovery-source-revision-v1", "requested": self.config.revision,
                             "resolved": revision})
            services_result = runner.run("inspect-source-writers",
                                         self.compose(source_project) + ["ps", "--status", "running", "--services"],
                                         self.config.timeouts["command_seconds"],
                                         env=self.compose_env(source_project, source_port))
            services = set(services_result.stdout.split())
            if "pipeline" in services:
                raise SafetyError("source pipeline/workflow writer is running; stop it before scheduled backup")
            should_restore = "api" in services
            write_exclusive(run_dir / "source-api-intent.json",
                            {"schema": "wasl-recovery-source-api-intent-v1", "should_be_running": should_restore,
                             "recorded_at": now_iso()})
            if should_restore:
                runner.run("quiesce-source-api", self.compose(source_project) + ["stop", "api"],
                           self.config.timeouts["command_seconds"], env=self.compose_env(source_project, source_port))
            self.catalog(runner, "prepare", generation)
            backup_env = self.compose_env(source_project, source_port)
            backup_env["WASL_RECOVERY_DIR"] = f"/backup/scheduler-v1/generations/{generation}"
            backup = runner.run("encrypted-generation-backup",
                                self.compose(source_project) + ["--profile", "recovery", "run", "--rm",
                                                                "-e", f"WASL_RECOVERY_DIR=/backup/scheduler-v1/generations/{generation}",
                                                                "recovery", "recovery-backup"],
                                self.config.timeouts["backup_seconds"], env=backup_env)
            summary = self.operation_summary(backup.stdout, "backup-nats")
            write_exclusive(run_dir / "backup-summary.json", summary)
            runner.run("authenticate-complete-generation",
                       self.compose(source_project) + ["--profile", "recovery", "run", "--rm", "--no-deps",
                                                       "--entrypoint", "python3", "recovery", "/app/scripts/recovery.py", "verify",
                                                       f"/backup/scheduler-v1/generations/{generation}/bundle",
                                                       "--key-file", "/run/secrets/recovery_keys.json"],
                       self.config.timeouts["backup_seconds"], env=self.compose_env(source_project, source_port))
        except (OperationalError, SafetyError, OSError, ValueError) as exc:
            error = str(exc)
        finally:
            if should_restore:
                try:
                    runner.run("restore-source-api", self.compose(source_project) + ["up", "-d", "--wait", "api"],
                               self.config.timeouts["target_start_seconds"],
                               env=self.compose_env(source_project, source_port))
                except OperationalError:
                    restoration = "source API restoration failed; scheduled runs halted for operator intervention"
                    atomic_json(self.state / "halted.json",
                                {"schema": "wasl-recovery-scheduler-halt-v1", "reason": restoration,
                                 "attempt_id": attempt_id, "created_at": now_iso()})
                    error = f"{error}; {restoration}" if error else restoration
        if error is None:
            temporary_api = False
            try:
                if not should_restore:
                    runner.run("start-source-api-for-evidence-probe",
                               self.compose(source_project) + ["up", "-d", "--wait", "api"],
                               self.config.timeouts["target_start_seconds"],
                               env=self.compose_env(source_project, source_port))
                    temporary_api = True
                self.probe(runner, self.config.repository, self.config.source["api_url"], run_dir / "source-probe.json")
                if temporary_api:
                    runner.run("restore-initially-stopped-source-api",
                               self.compose(source_project) + ["stop", "api"],
                               self.config.timeouts["command_seconds"],
                               env=self.compose_env(source_project, source_port))
                    temporary_api = False
                self.catalog(runner, "register", generation,
                             ["--slot", slot, "--revision", revision, "--attempt-id", attempt_id])
                exported = self.catalog(runner, "export", generation,
                                        ["--output", f"/evidence/runs/{attempt_id}/bundle-manifest.json"])
                write_exclusive(run_dir / "bundle-manifest.sha256", exported["manifest_sha256"] + "\n")
                self.catalog(runner, "publish", generation)
            except (OperationalError, OSError, ValueError) as exc:
                error = str(exc)
            finally:
                if temporary_api:
                    try:
                        runner.run("restore-initially-stopped-source-api",
                                   self.compose(source_project) + ["stop", "api"],
                                   self.config.timeouts["command_seconds"],
                                   env=self.compose_env(source_project, source_port))
                    except OperationalError:
                        restoration = "initially stopped source API could not be restored; scheduled runs halted"
                        atomic_json(self.state / "halted.json",
                                    {"schema": "wasl-recovery-scheduler-halt-v1", "reason": restoration,
                                     "attempt_id": attempt_id, "created_at": now_iso()})
                        error = f"{error}; {restoration}" if error else restoration
        self._finish(run_dir, attempt_id, "failed" if error else "success", error)
        self._event({"event": "backup_finished", "attempt_id": attempt_id, "generation": generation,
                     "status": "failed" if error else "success"})
        if error:
            if "writer is running" in error:
                raise SafetyError(error)
            raise OperationalError(error)
        return attempt_id

    @staticmethod
    def operation_summary(stdout: str, operation: str) -> dict:
        for line in reversed(stdout.splitlines()):
            try:
                value = json.loads(line)
            except ValueError:
                continue
            if value.get("status") == "ok" and value.get("operation") == operation and isinstance(value.get("summary"), dict):
                return value
        raise OperationalError(f"{operation} returned no valid stream-hash summary")

    def probe(self, runner: CommandRunner, checkout: Path, base_url: str, output: Path) -> dict:
        runner.run("authenticated-https-probe",
                   [sys.executable, str(checkout / "scripts/recovery_api_probe.py"), "--secrets",
                    self.config.source["secrets_dir"], "--base-url", base_url, "--output", str(output)],
                   self.config.timeouts["command_seconds"])
        return load_json(output)

    @staticmethod
    def free_port() -> int:
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            return sock.getsockname()[1]

    def _require_absent_target(self, runner: CommandRunner, project: str) -> None:
        for volume in ("nats-0", "nats-1", "nats-2", "wasl-run"):
            name = f"{project}_{volume}"
            result = runner.run("require-absent-target-volume", [self.config.engine, "volume", "inspect", name],
                                self.config.timeouts["command_seconds"], check=False)
            if result.returncode == 0:
                raise SafetyError(f"target volume already exists: {name}")
            if not re.search(r"(?i)no such|not found|does not exist", result.stderr):
                raise OperationalError("container engine could not establish that target volumes are absent")
        containers = runner.run("require-absent-target-containers",
                                [self.config.engine, "ps", "-a", "--filter", f"label=com.docker.compose.project={project}",
                                 "--format", "{{.Names}}"], self.config.timeouts["command_seconds"])
        if containers.stdout.strip():
            raise SafetyError("target Compose project already has containers")

    def capture_target(self, run_dir: Path, project: str, checkout: Path) -> None:
        if not checkout.is_dir():
            return
        runner = self.runner(run_dir)
        port = self.free_port()
        generation = None
        if (run_dir / "selection.json").is_file():
            generation = load_json(run_dir / "selection.json").get("generation")
        if generation is None:
            started = load_json(run_dir / "started.json")
            generation = started.get("generation")
        env = self.compose_env(project, port, generation)
        compose = self.compose(project, checkout, override=True)
        runner.run("preserve-target-status", compose + ["ps", "-a"], self.config.timeouts["command_seconds"],
                   env=env, check=False)
        runner.run("preserve-target-logs", compose + ["logs", "--no-color"], self.config.timeouts["command_seconds"],
                   env=env, check=False)

    @staticmethod
    def compare_evidence(backup: dict, restore: dict, verify: dict,
                         source: dict, target: dict, restart: dict) -> dict:
        backup_map = backup["summary"]["logical_sha256_by_stream"]
        restore_map = restore["summary"]["logical_sha256_by_stream"]
        verify_map = verify["summary"]["logical_sha256_by_stream"]
        nats_match = backup_map == restore_map == verify_map
        case_values = [source.get("case_id"), target.get("case_id"), restart.get("case_id")]
        hash_values = [source.get("comparison_sha256"), target.get("comparison_sha256"),
                       restart.get("comparison_sha256")]
        case_match = (all(isinstance(value, str) and value for value in case_values)
                      and case_values[0] == case_values[1] == case_values[2])
        comparison_match = (all(isinstance(value, str) and re.fullmatch(r"[0-9a-f]{64}", value)
                                for value in hash_values) and hash_values[0] == hash_values[1] == hash_values[2])
        anonymous = [source.get("unauthenticated_status"), target.get("unauthenticated_status"),
                     restart.get("unauthenticated_status")]
        modes = [source.get("data_mode"), target.get("data_mode"), restart.get("data_mode")]
        checks = {"nats_stream_hashes_match": nats_match, "case_ids_match": case_match,
                  "comparison_hashes_match": comparison_match, "anonymous_probes_are_401": anonymous == [401, 401, 401],
                  "data_mode_is_simulated": modes == ["simulated", "simulated", "simulated"]}
        return {"schema": "wasl-recovery-hash-comparison-v1", "ok": all(checks.values()), "checks": checks,
                "backup_logical_sha256_by_stream": backup_map, "restore_logical_sha256_by_stream": restore_map,
                "verify_logical_sha256_by_stream": verify_map,
                "api": {"source": source, "target": target, "restart": restart},
                "nondeterministic_observed_not_compared": ["content_sha256", "meta.loaded_at"]}

    def verify(self, selected: str, slot: str | None = None, automatic: bool = False) -> str:
        nonce = secrets.token_hex(8)
        attempt_id = f"verify-{slot}-{nonce}" if automatic else f"verify-manual-{nonce}"
        target_project = f"waslrecver-{nonce}"
        run_dir = self.state / "runs" / attempt_id
        run_dir.mkdir(mode=0o700)
        generation_for_start = None if selected == "latest" else check_id(selected)
        # started.json is exclusive and precedes revision resolution and every mutable resource operation.
        write_exclusive(run_dir / "started.json", {
            "schema": ATTEMPT_SCHEMA, "attempt_id": attempt_id, "kind": "verify", "automatic": automatic,
            "slot": slot, "generation": generation_for_start, "target_project": target_project,
            "source_project": self.config.source["project"], "source_revision": self.config.revision,
            "started_at": now_iso(), "configuration": self.config.raw})
        write_exclusive(run_dir / "resolved-config.json", self.config.raw)
        if automatic:
            self._claim_slot("verify", slot, attempt_id)
        runner = self.runner(run_dir)
        error = None
        checkout = self.state / "work" / attempt_id / "checkout"
        generation = generation_for_start
        revision = self.config.revision
        try:
            revision = self.resolve_revision(runner)
            write_exclusive(run_dir / "source-revision.json",
                            {"schema": "wasl-recovery-source-revision-v1", "requested": self.config.revision,
                             "resolved": revision})
            resolved = self.catalog(runner, "resolve", selected)
            generation = resolved["generation"]
            record = resolved["record"]
            write_exclusive(run_dir / "selection.json", resolved)
            exported = self.catalog(runner, "export", generation,
                                    ["--output", f"/evidence/runs/{attempt_id}/bundle-manifest.json"])
            write_exclusive(run_dir / "bundle-manifest.sha256", exported["manifest_sha256"] + "\n")
            checkout.parent.mkdir(parents=True, exist_ok=True)
            runner.run("clone-attempt-private-revision",
                       ["git", "clone", "--no-hardlinks", str(self.config.repository), str(checkout)],
                       self.config.timeouts["command_seconds"])
            runner.run("checkout-source-revision", ["git", "-C", str(checkout), "checkout", "--detach", revision],
                       self.config.timeouts["command_seconds"])
            self._require_absent_target(runner, target_project)
            port = self.free_port()
            write_exclusive(run_dir / "target-resources.json", {
                "schema": "wasl-recovery-target-resources-v1", "target_project": target_project,
                "api_url": f"https://127.0.0.1:{port}",
                "required_absent_volumes": [f"{target_project}_{name}" for name in
                                            ("nats-0", "nats-1", "nats-2", "wasl-run")],
                "external_inputs_read_only": [self.config.source["secrets_volume"],
                                               self.config.source["backup_volume"]]})
            env = self.compose_env(target_project, port, generation)
            compose = self.compose(target_project, checkout, override=True)
            runner.run("start-empty-target-nats", compose + ["up", "-d", "--build", "--wait",
                                                              "nats-0", "nats-1", "nats-2"],
                       self.config.timeouts["target_start_seconds"], env=env)
            restore = runner.run("restore-into-empty-target",
                                 compose + ["--profile", "recovery", "run", "--rm", "--no-deps",
                                            "recovery", "recovery-restore"],
                                 self.config.timeouts["backup_seconds"], env=env)
            restore_summary = self.operation_summary(restore.stdout, "restore-nats")
            write_exclusive(run_dir / "restore-summary.json", restore_summary)
            runner.run("start-target-api", compose + ["up", "-d", "--wait", "api"],
                       self.config.timeouts["target_start_seconds"], env=env)
            verification = runner.run("independent-target-stream-verification",
                                      compose + ["--profile", "recovery", "run", "--rm", "--no-deps",
                                                 "recovery", "recovery-verify"],
                                      self.config.timeouts["backup_seconds"], env=env)
            verify_summary = self.operation_summary(verification.stdout, "verify-nats")
            write_exclusive(run_dir / "verify-summary.json", verify_summary)
            target_url = f"https://127.0.0.1:{port}"
            target = self.probe(runner, checkout, target_url, run_dir / "target-probe.json")
            runner.run("ordinary-target-restart", compose + ["restart", "nats-0", "nats-1", "nats-2", "api"],
                       self.config.timeouts["target_start_seconds"], env=env)
            runner.run("await-restarted-target", compose + ["up", "-d", "--wait", "nats-0", "nats-1", "nats-2", "api"],
                       self.config.timeouts["target_start_seconds"], env=env)
            restart = self.probe(runner, checkout, target_url, run_dir / "restart-probe.json")
            backup_run = self.state / "runs" / record["backup_attempt_id"]
            source = load_json(backup_run / "source-probe.json")
            backup_summary = load_json(backup_run / "backup-summary.json")
            comparison = self.compare_evidence(backup_summary, restore_summary, verify_summary, source, target, restart)
            write_exclusive(run_dir / "hash-comparison.json", comparison)
            if not comparison["ok"]:
                raise OperationalError("clean-target NATS or authenticated API comparison failed")
        except (OperationalError, SafetyError, OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
            error = str(exc)
        if error:
            try:
                self.capture_target(run_dir, target_project, checkout)
            except Exception as capture_error:  # preservation failure is itself evidence; never trigger cleanup
                error += f"; target diagnostics capture failed: {type(capture_error).__name__}"
            self._finish(run_dir, attempt_id, "failed", error)
            self._event({"event": "verification_finished", "attempt_id": attempt_id,
                         "generation": generation, "status": "failed"})
            if isinstance(error, str) and ("already exists" in error or "already has containers" in error
                                           or "revision differs" in error):
                raise SafetyError(error)
            raise OperationalError(error)
        self._finish(run_dir, attempt_id, "success")
        catalog_action_dir = self.state / "catalog-actions" / attempt_id
        catalog_action_dir.mkdir(parents=True, exist_ok=True)
        self.catalog(self.runner(catalog_action_dir), "mark-good", generation, ["--attempt-id", attempt_id])
        # Cleanup is deliberately after durable success and verified-good publication. Its logs live outside the sealed
        # verification attempt so later command records cannot invalidate the final evidence manifest.
        cleanup_dir = self.state / "cleanup" / attempt_id
        cleanup_dir.mkdir(parents=True, exist_ok=True)
        cleanup_runner = self.runner(cleanup_dir)
        env = self.compose_env(target_project, self.free_port(), generation)
        cleanup_runner.run("remove-successful-target-only",
                           self.compose(target_project, checkout, override=True) + ["down", "-v", "--remove-orphans"],
                           self.config.timeouts["command_seconds"], env=env)
        write_exclusive(cleanup_dir / "result.json",
                        {"schema": "wasl-recovery-target-cleanup-v1", "attempt_id": attempt_id,
                         "target_project": target_project, "external_volumes_removed": False,
                         "finished_at": now_iso()})
        self.finalize_evidence(cleanup_dir)
        self._event({"event": "verification_finished", "attempt_id": attempt_id,
                     "generation": generation, "status": "success", "target_cleaned": True})
        return attempt_id

    def retention(self, apply: bool = False) -> dict:
        if not self.config.retention["enabled"]:
            raise SafetyError("retention is disabled by configuration")
        # Reconciliation is catalog-local and resumes only an exact generation intent after crash.
        revision_dir = self.state / "work" / f"retention-{secrets.token_hex(6)}"
        revision_dir.mkdir(parents=True, exist_ok=False)
        runner = self.runner(revision_dir)
        result = self.catalog(runner, "retention", extra=[
            "--max-age-days", str(self.config.retention["max_age_days"]),
            "--keep-newest", str(self.config.retention["keep_newest_verified"]),
            "--minimum-good", str(self.config.retention["min_verified_good"]),
        ] + (["--apply"] if apply else []))
        self._event({"event": "retention_review", "apply": apply, "status": result.get("status"),
                     "candidates": [item["generation"] for item in result.get("candidates", [])]})
        return result


def locked(config: Config, name: str):
    return FileLock(config.state_dir / name)


def cli(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", type=Path, required=True)
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("init")
    sub.add_parser("run-due")
    sub.add_parser("serve")
    verify = sub.add_parser("verify")
    verify.add_argument("--generation", required=True)
    retention = sub.add_parser("retention")
    retention.add_argument("--apply", action="store_true")
    args = parser.parse_args(argv)
    config = Config.load(args.config)
    scheduler = Scheduler(config)
    if args.command == "init":
        if config.state_dir.exists() and any(config.state_dir.iterdir()):
            raise SafetyError("init requires a new empty evidence directory")
        config.state_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
        with locked(config, "run.lock"):
            runner = scheduler.runner(config.state_dir / "init")
            # init has no attempt ID; retain its nonsecret command log separately from immutable attempts.
            result = scheduler.catalog(runner, "init")
        print(json.dumps({"status": "ok", "result": result}, sort_keys=True))
        return 0
    scheduler.require_init()
    if args.command == "serve":
        with locked(config, "serve.lock"):
            while True:
                if (config.state_dir / "halted.json").exists():
                    raise OperationalError("scheduler is halted pending source API operator intervention")
                try:
                    with locked(config, "run.lock"):
                        scheduler.reconcile()
                        scheduler.run_due()
                except OperationalError as exc:
                    print(f"scheduled operation failed: {exc}", file=sys.stderr)
                time.sleep(config.schedule["poll_seconds"])
    with locked(config, "run.lock"):
        scheduler.reconcile()
        if (config.state_dir / "halted.json").exists() and args.command in ("run-due", "retention"):
            raise OperationalError("scheduler is halted pending source API operator intervention")
        if args.command == "run-due":
            result = {"attempts": scheduler.run_due()}
        elif args.command == "verify":
            result = {"attempt": scheduler.verify(args.generation)}
        else:
            result = scheduler.retention(args.apply)
    print(json.dumps({"status": "ok", "result": result}, sort_keys=True))
    return 0


def main() -> None:
    try:
        raise SystemExit(cli())
    except LockBusy as exc:
        print(f"scheduler busy: {exc}", file=sys.stderr)
        raise SystemExit(75)
    except (ConfigError, SafetyError) as exc:
        print(f"scheduler configuration/safety refusal: {exc}", file=sys.stderr)
        raise SystemExit(2)
    except EvidenceError as exc:
        print(f"scheduler evidence finalization failure: {exc}; cleanup and retention skipped", file=sys.stderr)
        raise SystemExit(3)
    except (OperationalError, OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
        print(f"scheduler operation failed: {exc}", file=sys.stderr)
        raise SystemExit(1)


if __name__ == "__main__":
    main()
