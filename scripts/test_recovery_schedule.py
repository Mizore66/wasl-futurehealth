#!/usr/bin/env python3
"""Scheduler state-machine and orchestration tests using synthetic fixtures and command fakes only."""
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import shutil
import sys
import tempfile
import unittest
from unittest import mock

SCRIPTS = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPTS))

from recovery import write_file_bundle  # noqa: E402
from recovery_catalog import Catalog, write_exclusive  # noqa: E402
from recovery_crypto import new_keyring  # noqa: E402
import recovery_schedule  # noqa: E402
from recovery_schedule import (  # noqa: E402
    CommandResult,
    CommandRunner,
    Config,
    ConfigError,
    EvidenceError,
    FileLock,
    LockBusy,
    OperationalError,
    SafetyError,
    Scheduler,
    atomic_json,
    load_json,
)

REVISION = "a" * 40
HASH_A = "a" * 64
HASH_B = "b" * 64


class Crash(BaseException):
    pass


class FakeRunner:
    def __init__(self, run_dir: Path, owner: "FakeScheduler"):
        self.run_dir = run_dir
        self.owner = owner

    def run(self, name: str, argv: list[str], timeout: int, env=None, cwd=None, check=True) -> CommandResult:
        self.owner.calls.append((name, argv, env, self.run_dir))
        stdout, stderr, code = "", "", 0
        if name == "inspect-source-writers":
            stdout = self.owner.source_services
        elif name == "encrypted-generation-backup":
            stdout = json.dumps({"status": "ok", "operation": "backup-nats",
                                 "summary": {"streams": 1, "messages": 2,
                                             "logical_sha256_by_stream": {"FACTS": HASH_A}}}) + "\n"
        elif name == "restore-into-empty-target":
            stdout = json.dumps({"status": "ok", "operation": "restore-nats",
                                 "summary": {"streams": 1, "messages": 2,
                                             "logical_sha256_by_stream": {"FACTS": HASH_A}}}) + "\n"
            code = self.owner.restore_code
        elif name == "independent-target-stream-verification":
            digest = HASH_B if self.owner.hash_mismatch else HASH_A
            stdout = json.dumps({"status": "ok", "operation": "verify-nats",
                                 "summary": {"streams": 1, "messages": 2,
                                             "logical_sha256_by_stream": {"FACTS": digest}}}) + "\n"
        elif name == "require-absent-target-volume":
            code = 0 if any(arg.endswith("_" + self.owner.preexisting_volume) for arg in argv) else 1
            stderr = "no such volume\n" if code else ""
        elif name == "clone-attempt-private-revision":
            Path(argv[-1]).mkdir(parents=True)
        if name == self.owner.timeout_command:
            code = 124
        if check and code != 0:
            raise OperationalError(f"{name} failed with exit status {code}")
        return CommandResult(argv, code, stdout, stderr, 0.01)


class FakeScheduler(Scheduler):
    def __init__(self, *args, **kwargs):
        self.calls = []
        self.catalog_calls = []
        self.preexisting_volume = ""
        self.hash_mismatch = False
        self.restore_code = 0
        self.timeout_command = ""
        self.fail_resolve = False
        self.fail_revision = False
        self.source_services = "api\n"
        self.mark_good_error = False
        super().__init__(*args, runner_factory=lambda run_dir: FakeRunner(run_dir, self), **kwargs)

    def resolve_revision(self, runner):
        if self.fail_revision:
            raise OperationalError("source revision could not be resolved")
        return REVISION

    def catalog(self, runner, action, generation=None, extra=None):
        self.catalog_calls.append((action, generation, list(extra or [])))
        if action == "resolve":
            if self.fail_resolve:
                raise OperationalError("backup authentication failed: wrong key or modified manifest")
            generation = "gen-fixture"
            return {"generation": generation, "bundle": f"/backup/scheduler-v1/generations/{generation}/bundle",
                    "record": {"generation": generation, "source_revision": REVISION,
                               "backup_attempt_id": "backup-fixture", "manifest_sha256": "c" * 64}}
        if action == "export":
            write_exclusive(runner.run_dir / "bundle-manifest.json", b'{"schema":"wasl-file-backup-v2-aesgcm"}\n')
            return {"generation": generation, "manifest_sha256": "c" * 64}
        if action == "mark-good":
            if self.mark_good_error:
                raise OperationalError("could not persist verified-good record")
            return {"generation": generation, "verified": True}
        if action == "retention":
            return {"status": "applied" if "--apply" in (extra or []) else "dry-run", "candidates": []}
        return {"generation": generation, "status": "ok"}

    def probe(self, runner, checkout, base_url, output):
        document = {"health_status": 200, "unauthenticated_status": 401, "case_id": "synthetic/case-1",
                    "comparison_sha256": "d" * 64, "content_sha256": "e" * 64,
                    "loaded_at": "2026-01-01T00:00:00Z", "data_mode": "simulated"}
        write_exclusive(output, document)
        return document


class SchedulerTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root, True)
        self.repository = self.root / "repo"
        self.secrets = self.root / "secrets"
        self.state = self.root / "evidence"
        for path in (self.repository, self.secrets, self.state):
            path.mkdir()
        for name in ("runs", "work", "slots/backup", "slots/verify"):
            (self.state / name).mkdir(parents=True)
        write_exclusive(self.state / "initialized.json", {"schema": "wasl-recovery-scheduler-state-v1"})
        self.raw = {
            "schema": "wasl-recovery-scheduler-v1", "repository": str(self.repository), "revision": REVISION,
            "engine": "podman", "source": {"project": "waslrecsrc-test", "secrets_volume": "wasl-test-secrets",
                                             "secrets_dir": str(self.secrets), "backup_volume": "wasl-test-backups",
                                             "api_url": "https://127.0.0.1:19443"},
            "state_dir": str(self.state), "schedule": {"epoch": "2026-01-01T00:00:00Z",
                                                         "backup_interval_seconds": 86400,
                                                         "verify_interval_seconds": 604800, "poll_seconds": 1},
            "retention": {"enabled": True, "max_age_days": 30, "keep_newest_verified": 2,
                          "min_verified_good": 2},
            "timeouts": {"command_seconds": 10, "backup_seconds": 20, "target_start_seconds": 30}}
        self.config_path = self.root / "scheduler.json"
        self.config_path.write_text(json.dumps(self.raw))
        self.config = Config.load(self.config_path)
        self.clock = datetime(2026, 1, 2, tzinfo=timezone.utc)
        self.scheduler = FakeScheduler(self.config, now=lambda: self.clock)
        self.seed_backup_evidence()

    def seed_backup_evidence(self):
        run = self.state / "runs" / "backup-fixture"
        run.mkdir()
        write_exclusive(run / "started.json", {"attempt_id": "backup-fixture", "kind": "backup",
                                                "generation": "gen-fixture"})
        write_exclusive(run / "source-probe.json", {
            "unauthenticated_status": 401, "case_id": "synthetic/case-1", "comparison_sha256": "d" * 64,
            "content_sha256": "e" * 64, "loaded_at": "2026-01-01T00:00:00Z", "data_mode": "simulated"})
        write_exclusive(run / "backup-summary.json", {
            "summary": {"logical_sha256_by_stream": {"FACTS": HASH_A}}})

    def names(self):
        return [call[0] for call in self.scheduler.calls]

    def preclaim_verify_zero(self):
        write_exclusive(self.state / "slots/verify/0.json", {"attempt_id": "preexisting"})

    def test_repeated_run_due_in_one_slot_creates_no_duplicate_backup_or_target(self):
        self.preclaim_verify_zero()
        first = self.scheduler.run_due()
        calls_after_first = len(self.scheduler.calls)
        second = self.scheduler.run_due()
        self.assertEqual(len(first), 1)
        self.assertEqual(second, [])
        self.assertEqual(self.names().count("encrypted-generation-backup"), 1)
        self.assertEqual(len(self.scheduler.calls), calls_after_first)
        self.assertTrue((self.state / "runs" / "backup-1" / "result.json").is_file())
        self.assertIn("restore-source-api", self.names())
        self.assertEqual(self.scheduler.catalog_calls[-1][0], "publish")

    def test_initially_stopped_source_api_is_probed_then_returned_to_stopped(self):
        self.scheduler.source_services = ""
        self.scheduler.backup("1")
        self.assertIn("start-source-api-for-evidence-probe", self.names())
        self.assertIn("restore-initially-stopped-source-api", self.names())
        self.assertNotIn("restore-source-api", self.names())
        self.assertLess(self.names().index("start-source-api-for-evidence-probe"),
                        self.names().index("restore-initially-stopped-source-api"))

    def test_revision_resolution_failure_consumes_slot_and_is_not_retried(self):
        self.preclaim_verify_zero()
        self.scheduler.fail_revision = True
        with self.assertRaises(OperationalError):
            self.scheduler.run_due()
        run = self.state / "runs" / "backup-1"
        self.assertEqual(load_json(run / "result.json")["status"], "failed")
        self.scheduler.fail_revision = False
        self.assertEqual(self.scheduler.run_due(), [])
        self.assertNotIn("encrypted-generation-backup", self.names())

    def test_missed_slots_are_recorded_and_never_caught_up(self):
        self.preclaim_verify_zero()
        self.scheduler.run_due()
        self.clock += timedelta(days=3)  # current slot 4; slots 2 and 3 were missed
        self.scheduler.run_due()
        missed = load_json(self.state / "slots/backup/missed-2-3.json")
        self.assertEqual((missed["first_missed"], missed["last_missed"], missed["catch_up"]), (2, 3, False))
        self.assertFalse((self.state / "slots/backup/2.json").exists())
        self.assertFalse((self.state / "slots/backup/3.json").exists())
        self.assertTrue((self.state / "slots/backup/4.json").exists())
        self.assertEqual(self.names().count("encrypted-generation-backup"), 2)

    def test_interrupted_slot_is_sealed_and_requires_explicit_retry(self):
        run = self.state / "runs" / "backup-1-aaaaaaaaaaaa"
        run.mkdir()
        write_exclusive(run / "started.json", {
            "attempt_id": run.name, "kind": "backup", "automatic": True, "slot": "1",
            "generation": "gen-1-aaaaaaaaaaaa", "target_project": None})
        write_exclusive(run / "source-api-intent.json", {"should_be_running": True})
        self.preclaim_verify_zero()
        self.scheduler.reconcile()
        self.assertEqual(load_json(run / "result.json")["status"], "interrupted")
        self.assertIn("reconcile-restore-source-api", self.names())
        self.assertEqual(self.scheduler.run_due(), [])
        self.assertNotIn("encrypted-generation-backup", self.names())

    def test_crash_after_temporary_start_restores_initially_stopped_source_api(self):
        run = self.state / "runs" / "backup-2-bbbbbbbbbbbb"
        run.mkdir()
        write_exclusive(run / "started.json", {
            "attempt_id": run.name, "kind": "backup", "automatic": True, "slot": "2",
            "generation": "gen-2-bbbbbbbbbbbb", "target_project": None})
        write_exclusive(run / "source-api-intent.json", {"should_be_running": False})
        self.scheduler.reconcile()
        stop = [call for call in self.scheduler.calls if call[0] == "reconcile-restore-initially-stopped-source-api"]
        self.assertEqual(len(stop), 1)
        self.assertEqual(stop[0][1][-2:], ["stop", "api"])
        self.assertNotIn("reconcile-restore-source-api", self.names())
        self.assertEqual(load_json(run / "result.json")["status"], "interrupted")
        self.assertFalse((self.state / "halted.json").exists())

    def test_crash_stopped_source_api_restore_failure_halts_scheduler(self):
        run = self.state / "runs" / "backup-3-cccccccccccc"
        run.mkdir()
        write_exclusive(run / "started.json", {
            "attempt_id": run.name, "kind": "backup", "automatic": True, "slot": "3",
            "generation": "gen-3-cccccccccccc", "target_project": None})
        write_exclusive(run / "source-api-intent.json", {"should_be_running": False})
        self.scheduler.timeout_command = "reconcile-restore-initially-stopped-source-api"
        self.scheduler.reconcile()
        self.assertTrue((self.state / "halted.json").is_file())
        self.assertIn("source API restoration failed", load_json(run / "result.json")["error"])

    def test_daemon_and_operation_lock_contention_have_no_attempt_side_effects(self):
        before = set((self.state / "runs").iterdir())
        for name in ("run.lock", "serve.lock"):
            with self.subTest(name), FileLock(self.state / name):
                with self.assertRaises(LockBusy):
                    with FileLock(self.state / name):
                        pass
                self.assertEqual(set((self.state / "runs").iterdir()), before)

    def test_wrong_key_or_tamper_fails_before_clone_or_target_creation(self):
        self.scheduler.fail_resolve = True
        with self.assertRaises(OperationalError):
            self.scheduler.verify("latest")
        self.assertNotIn("clone-attempt-private-revision", self.names())
        self.assertNotIn("start-empty-target-nats", self.names())
        failed = [path for path in (self.state / "runs").glob("verify-manual-*")]
        self.assertEqual(len(failed), 1)
        self.assertEqual(load_json(failed[0] / "result.json")["status"], "failed")
        self.assertTrue((failed[0] / "evidence-manifest.json").is_file())

    def test_preexisting_target_volume_is_refused_without_up_seed_replace_or_cleanup(self):
        self.scheduler.preexisting_volume = "nats-1"
        with self.assertRaises(SafetyError):
            self.scheduler.verify("gen-fixture")
        joined = "\n".join(" ".join(call[1]) for call in self.scheduler.calls)
        self.assertNotIn(" up ", f" {joined} ")
        self.assertNotIn("seed", joined)
        self.assertNotIn("down", joined)
        self.assertNotIn("WASL_RECOVERY_REPLACE", joined)
        for _, _, env, _ in self.scheduler.calls:
            if env:
                self.assertNotIn("WASL_RECOVERY_REPLACE", env)

    def test_hash_mismatch_timeout_and_partial_restore_preserve_target_and_issue_no_down(self):
        for label in ("hash", "timeout", "partial"):
            with self.subTest(label):
                self.scheduler.hash_mismatch = label == "hash"
                self.scheduler.timeout_command = "restore-into-empty-target" if label == "timeout" else ""
                self.scheduler.restore_code = 3 if label == "partial" else 0
                with self.assertRaises(OperationalError):
                    self.scheduler.verify("gen-fixture")
                self.assertNotIn("remove-successful-target-only", self.names())
                self.assertNotIn("mark-good", [call[0] for call in self.scheduler.catalog_calls])
                self.scheduler.hash_mismatch = False
                self.scheduler.timeout_command = ""
                self.scheduler.restore_code = 0

    def test_successful_verification_compares_nats_api_and_restart_then_marks_good_before_cleanup(self):
        attempt = self.scheduler.verify("gen-fixture")
        comparison = load_json(self.state / "runs" / attempt / "hash-comparison.json")
        self.assertTrue(comparison["ok"])
        self.assertEqual(comparison["backup_logical_sha256_by_stream"],
                         comparison["restore_logical_sha256_by_stream"])
        self.assertEqual(comparison["restore_logical_sha256_by_stream"],
                         comparison["verify_logical_sha256_by_stream"])
        self.assertTrue(comparison["checks"]["anonymous_probes_are_401"])
        self.assertTrue(comparison["checks"]["data_mode_is_simulated"])
        mark_index = [call[0] for call in self.scheduler.catalog_calls].index("mark-good")
        self.assertEqual(mark_index, len(self.scheduler.catalog_calls) - 1)
        self.assertIn("ordinary-target-restart", self.names())
        self.assertIn("remove-successful-target-only", self.names())
        self.assertLess(self.names().index("ordinary-target-restart"), self.names().index("remove-successful-target-only"))
        cleanup = load_json(self.state / "cleanup" / attempt / "result.json")
        self.assertFalse(cleanup["external_volumes_removed"])

    def test_full_automatic_latest_verification_selects_marks_good_and_cleans_target(self):
        backup_root = self.root / "backup"
        source = self.root / "source"
        backup_root.mkdir()
        source.mkdir()
        (source / "synthetic.txt").write_text("synthetic latest generation\n")
        (backup_root / "scheduler-v1/generations").mkdir(parents=True)
        keys = self.root / "keys.json"
        new_keyring(keys)
        catalog = Catalog(backup_root, self.state, keys)
        catalog.prepare("gen-fixture")
        nats = self.root / "nats.json"
        nats.write_text('{"schema":"wasl-nats-backup-v1","streams":[]}\n')
        write_file_bundle(source, catalog.bundle("gen-fixture"), keys, nats_json=nats)
        backup_run = self.state / "runs/backup-fixture"
        write_exclusive(backup_run / "result.json", {
            "attempt_id": "backup-fixture", "status": "success", "finished_at": "2026-01-01T00:00:00+00:00"})
        Scheduler.finalize_evidence(backup_run)
        catalog.register("gen-fixture", "fixture", REVISION, "backup-fixture")
        catalog.publish("gen-fixture")
        fake_catalog = self.scheduler.catalog

        def real_selected_catalog(runner, action, generation=None, extra=None):
            self.scheduler.catalog_calls.append((action, generation, list(extra or [])))
            if action == "resolve":
                return catalog.resolve(generation)
            if action == "export":
                return catalog.export_manifest(generation, runner.run_dir / "bundle-manifest.json")
            if action == "mark-good":
                return catalog.mark_good(generation, extra[1])
            return fake_catalog(runner, action, generation, extra)

        self.scheduler.catalog = real_selected_catalog
        attempt = self.scheduler.verify("latest", slot="0", automatic=True)
        run = self.state / "runs" / attempt
        self.assertIsNone(load_json(run / "started.json")["generation"])
        self.assertEqual(load_json(run / "selection.json")["generation"], "gen-fixture")
        self.assertEqual(load_json(run / "result.json")["status"], "success")
        mark_good = [call for call in self.scheduler.catalog_calls if call[0] == "mark-good"]
        self.assertEqual(mark_good, [("mark-good", "gen-fixture", ["--attempt-id", attempt])])
        self.assertEqual(catalog.good_record("gen-fixture")["attempt_id"], attempt)
        self.assertIn("remove-successful-target-only", self.names())

    def test_empty_or_malformed_api_comparison_fields_do_not_pass(self):
        backup = {"summary": {"logical_sha256_by_stream": {"FACTS": HASH_A}}}
        probe = {"case_id": None, "comparison_sha256": None, "unauthenticated_status": 401,
                 "data_mode": "simulated"}
        comparison = Scheduler.compare_evidence(backup, backup, backup, probe, probe, probe)
        self.assertFalse(comparison["ok"])
        self.assertFalse(comparison["checks"]["case_ids_match"])
        self.assertFalse(comparison["checks"]["comparison_hashes_match"])

    def test_each_manual_verification_uses_a_unique_target(self):
        first = self.scheduler.verify("gen-fixture")
        second = self.scheduler.verify("gen-fixture")
        first_started = load_json(self.state / "runs" / first / "started.json")
        second_started = load_json(self.state / "runs" / second / "started.json")
        self.assertNotEqual(first_started["target_project"], second_started["target_project"])
        targets = [call[1][-1] for call in self.scheduler.calls if call[0] == "clone-attempt-private-revision"]
        self.assertNotEqual(targets[0], targets[1])

    def test_evidence_finalization_failure_skips_mark_good_and_cleanup(self):
        with mock.patch.object(Scheduler, "finalize_evidence", side_effect=EvidenceError("simulated fsync failure")):
            with self.assertRaises(EvidenceError):
                self.scheduler.verify("gen-fixture")
        self.assertNotIn("mark-good", [call[0] for call in self.scheduler.catalog_calls])
        self.assertNotIn("remove-successful-target-only", self.names())

    def test_duplicate_attempt_id_cannot_overwrite_started_evidence(self):
        run = self.scheduler._new_attempt("verify", "duplicate-id", REVISION, None, "gen-fixture", None, False)
        before = (run / "started.json").read_bytes()
        with self.assertRaises(FileExistsError):
            self.scheduler._new_attempt("verify", "duplicate-id", REVISION, None, "gen-other", None, False)
        self.assertEqual((run / "started.json").read_bytes(), before)

    def test_source_api_restore_failure_halts_future_scheduled_operations(self):
        self.scheduler.timeout_command = "restore-source-api"
        with self.assertRaises(OperationalError):
            self.scheduler.backup("1")
        self.assertTrue((self.state / "halted.json").is_file())
        self.assertIn("restore-source-api", self.names())
        run = self.state / "runs" / "backup-1"
        self.assertIn("halted", load_json(run / "result.json")["error"])

    def test_retention_is_dry_run_unless_apply_is_explicit(self):
        self.scheduler.retention()
        self.scheduler.retention(apply=True)
        retention = [call for call in self.scheduler.catalog_calls if call[0] == "retention"]
        self.assertNotIn("--apply", retention[0][2])
        self.assertIn("--apply", retention[1][2])

    def test_configuration_rejects_non_loopback_http_overlap_and_bad_retention(self):
        for label, mutate in (
            ("http", lambda raw: raw["source"].__setitem__("api_url", "http://127.0.0.1:19443")),
            ("external", lambda raw: raw["source"].__setitem__("api_url", "https://192.0.2.1:19443")),
            ("overlap", lambda raw: raw.__setitem__("state_dir", str(self.repository / "evidence"))),
            ("retention", lambda raw: raw["retention"].update(keep_newest_verified=1, min_verified_good=2)),
            ("interval", lambda raw: raw["schedule"].__setitem__("poll_seconds", 0)),
        ):
            with self.subTest(label):
                raw = json.loads(json.dumps(self.raw))
                mutate(raw)
                path = self.root / f"bad-{label}.json"
                path.write_text(json.dumps(raw))
                with self.assertRaises(ConfigError):
                    Config.load(path)

    def set_cursor(self, backup, verify, days):
        self.clock = datetime(2026, 1, 1, tzinfo=timezone.utc) + timedelta(days=days)
        atomic_json(self.state / "schedule.json", {"last_backup_slot": backup, "last_verify_slot": verify})

    def tree(self):
        return {str(p.relative_to(self.state)): p.read_bytes() for p in sorted(self.state.rglob("*"))
                if p.is_file() and not p.name.startswith("schedule") and p.name != "events.jsonl"}

    def missed_events(self):
        path = self.state / "events.jsonl"
        lines = path.read_text().splitlines() if path.exists() else []
        return [e for e in map(json.loads, lines) if e["event"] == "slots_missed"]

    def crash_once(self, target, name, when=lambda *a, **k: True):
        original = getattr(target, name)
        fired = []

        def crashing(*args, **kwargs):
            if not fired and when(*args, **kwargs):
                fired.append(True)
                raise Crash()
            return original(*args, **kwargs)
        return mock.patch.object(target, name, crashing)

    def test_missed_slot_marker_replay_converges_after_each_crash_window(self):
        self.preclaim_verify_zero()
        windows = {
            "after marker": lambda: self.crash_once(self.scheduler, "_event"),
            "after event": lambda: self.crash_once(recovery_schedule, "atomic_json", lambda path, *_: Path(path).name == "schedule.json"),
        }
        for name, inject in windows.items():
            with self.subTest(name):
                self.setUp()
                self.preclaim_verify_zero()
                self.set_cursor(1, 0, 3)
                with inject(), self.assertRaises(Crash):
                    self.scheduler.run_due()
                self.assertTrue((self.state / "slots/backup/missed-2-2.json").is_file())
                first = self.scheduler.run_due()
                self.assertEqual(len(first), 1)
                self.assertEqual(load_json(self.state / "schedule.json")["last_backup_slot"], 3)
                self.assertEqual(len(self.missed_events()), 1)
                settled = self.tree()
                self.assertEqual(self.scheduler.run_due(), [])
                self.assertEqual(self.tree(), settled)
                self.assertEqual(self.names().count("encrypted-generation-backup"), 1)

    def test_missed_verification_slot_replay_converges(self):
        self.set_cursor(20, 1, 21)  # backup slot 21 (claimed below), verify slot 3 with slot 2 missed
        write_exclusive(self.state / "slots/backup/21.json", {"attempt_id": "preexisting"})
        with self.crash_once(self.scheduler, "_event", lambda event: event["kind"] == "verify"), self.assertRaises(Crash):
            self.scheduler.run_due()
        self.assertTrue((self.state / "slots/verify/missed-2-2.json").is_file())
        self.scheduler.run_due()
        self.assertEqual(load_json(self.state / "schedule.json")["last_verify_slot"], 3)
        self.assertEqual([e["kind"] for e in self.missed_events()], ["verify"])
        self.assertTrue(list((self.state / "runs").glob("verify-3-*")))

    def test_conflicting_missed_slot_marker_fails_closed_without_modification(self):
        self.preclaim_verify_zero()
        self.set_cursor(1, 0, 3)
        marker = self.state / "slots/backup/missed-2-2.json"
        write_exclusive(marker, {"schema": "wasl-recovery-missed-slots-v1", "kind": "backup", "first_missed": 2,
                                 "last_missed": 2, "recorded_at": "2026-01-01T00:00:00Z", "catch_up": True})
        before = marker.read_bytes()
        with self.assertRaises(SafetyError):
            self.scheduler.run_due()
        self.assertEqual(marker.read_bytes(), before)
        self.assertEqual(load_json(self.state / "schedule.json")["last_backup_slot"], 1)
        self.assertEqual(self.names().count("encrypted-generation-backup"), 0)

    def test_crash_before_attempt_evidence_never_loses_the_due_slot(self):
        def midway_run_dir(*args, **kwargs):
            (self.state / "runs" / "backup-3").mkdir()
            raise Crash()
        windows = {
            "before run directory": lambda: self.crash_once(self.scheduler, "_new_attempt"),
            "after run directory": lambda: mock.patch.object(self.scheduler, "_new_attempt", midway_run_dir),
            "after started before claim": lambda: self.crash_once(self.scheduler, "_claim_slot"),
        }
        for name, inject in windows.items():
            with self.subTest(name):
                self.setUp()
                self.preclaim_verify_zero()
                self.set_cursor(2, 0, 3)
                with inject(), self.assertRaises(Crash):
                    self.scheduler.run_due()
                self.assertEqual(load_json(self.state / "schedule.json")["last_backup_slot"], 3)
                self.scheduler.reconcile()
                self.scheduler.run_due()
                self.assertTrue((self.state / "slots/backup/3.json").is_file())
                runs = [p for p in (self.state / "runs").iterdir() if p.name.startswith("backup-3")]
                self.assertEqual(len(runs), 1)
                self.assertIn(load_json(runs[0] / "result.json")["status"], ("interrupted", "success"))
                self.assertLessEqual(self.names().count("encrypted-generation-backup"), 1)
                settled = self.tree()
                self.scheduler.reconcile()
                self.assertEqual(self.scheduler.run_due(), [])
                self.assertEqual(self.tree(), settled)


class CommandEvidenceTests(unittest.TestCase):
    def test_structured_secret_material_is_redacted_before_evidence_write(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            run_dir = root / "evidence"
            run_dir.mkdir()
            runner = CommandRunner(run_dir)
            secret = "do-not-persist-this-value"
            script = root / "synthetic-output.sh"
            script.write_text("#!/bin/sh\nprintf '{\"password\":\"" + secret + "\",\"wrapped_dek\":\"" + secret + "\"}\\n'\n")
            script.chmod(0o700)
            result = runner.run("synthetic-output", [str(script)], 5)
            self.assertEqual(result.returncode, 0)
            self.assertNotIn(secret, " ".join(result.argv), "secret values must not be command arguments")
            for path in run_dir.rglob("*"):
                if path.is_file():
                    self.assertNotIn(secret, path.read_text())
            record = json.loads(result.stdout)
            self.assertEqual(record["password"], "[redacted]")
            self.assertEqual(record["wrapped_dek"], "[redacted]")


if __name__ == "__main__":
    unittest.main()
