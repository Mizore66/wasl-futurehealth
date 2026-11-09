#!/usr/bin/env python3
"""Unit tests for the resumable soak controller and report gates."""
import json
import os
from pathlib import Path
import tempfile
import unittest

import soak


class PersistenceTests(unittest.TestCase):
    def test_atomic_state_replaces_complete_json_without_temp_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "state.json"
            soak.atomic_write_json(path, {"generation": 1, "status": "running"})
            soak.atomic_write_json(path, {"generation": 2, "status": "running"})
            self.assertEqual(json.loads(path.read_text()), {"generation": 2, "status": "running"})
            self.assertEqual([p.name for p in Path(tmp).iterdir()], ["state.json"])

    def test_sample_writer_flushes_each_record_and_rotates_by_utc_hour(self):
        with tempfile.TemporaryDirectory() as tmp:
            now = [0.0]
            writer = soak.SampleWriter(Path(tmp), clock=lambda: now[0])
            writer.append({"seq": 1, "value": "first"})
            first = Path(tmp) / "samples" / "19700101T00.jsonl"
            self.assertEqual(json.loads(first.read_text()), {"seq": 1, "value": "first"})
            now[0] = 3601.0
            writer.append({"seq": 2, "value": "second"})
            second = Path(tmp) / "samples" / "19700101T01.jsonl"
            self.assertEqual(json.loads(second.read_text()), {"seq": 2, "value": "second"})
            writer.close()

    def test_journal_records_are_flushed_and_have_monotonic_sequence_numbers(self):
        with tempfile.TemporaryDirectory() as tmp:
            journal = soak.EventJournal(Path(tmp) / "events.jsonl")
            journal.append("restart_started", action_id="a1")
            journal.append("restart_recovered", action_id="a1", recovery_s=1.25)
            rows = [json.loads(line) for line in (Path(tmp) / "events.jsonl").read_text().splitlines()]
            self.assertEqual([row["seq"] for row in rows], [1, 2])
            self.assertEqual(rows[1]["recovery_s"], 1.25)

    def test_controller_lock_rejects_a_second_live_controller(self):
        with tempfile.TemporaryDirectory() as tmp:
            lock = soak.ControllerLock(Path(tmp) / "controller.lock")
            lock.acquire()
            try:
                with self.assertRaises(soak.ResumeError):
                    soak.ControllerLock(Path(tmp) / "controller.lock").acquire()
            finally:
                lock.close()


class ResumeAndCleanupTests(unittest.TestCase):
    def manifest(self, tmp):
        manifest = {"project": "waslsoak-test", "run_id": "test"}
        manifest["manifest_sha256"] = soak.manifest_digest(manifest)
        path = Path(tmp) / "manifest.json"
        soak.atomic_write_json(path, manifest)
        return manifest

    def test_resume_refuses_an_action_with_unknown_outcome(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest = self.manifest(tmp)
            path = Path(tmp) / "state.json"
            soak.atomic_write_json(path, {
                "manifest_sha256": manifest["manifest_sha256"], "actions": {"a1": {"status": "started", "due_epoch": 10}},
            })
            with self.assertRaisesRegex(soak.ResumeError, "unknown outcome"):
                soak.resume_state(path, manifest)

    def test_resume_preserves_absolute_deadlines_and_completed_actions(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest = self.manifest(tmp)
            path = Path(tmp) / "state.json"
            soak.atomic_write_json(path, {
                "manifest_sha256": manifest["manifest_sha256"],
                "actions": {
                    "a1": {"status": "completed", "due_epoch": 10},
                    "a2": {"status": "pending", "due_epoch": 20},
                },
            })
            state = soak.resume_state(path, manifest)
            self.assertEqual(state["actions"]["a2"]["due_epoch"], 20)
            self.assertEqual(soak.next_pending_action(state)["a2"]["due_epoch"], 20)

    def test_resume_rejects_a_different_manifest(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest = self.manifest(tmp)
            path = Path(tmp) / "state.json"
            soak.atomic_write_json(path, {"manifest_sha256": "different", "actions": {}})
            with self.assertRaisesRegex(soak.ResumeError, "manifest"):
                soak.resume_state(path, manifest)

    def test_cleanup_is_scoped_and_never_deletes_retained_evidence_volumes(self):
        self.assertEqual(soak.cleanup_args("waslsoak-a1", disposable=True), ["down", "-v"])
        self.assertEqual(soak.cleanup_args("waslsoak-a1", disposable=False), ["down"])
        with self.assertRaises(ValueError):
            soak.cleanup_args("other-project", disposable=True)
        with self.assertRaises(ValueError):
            soak.cleanup_args("waslsoak-a1;podman", disposable=True)


class GateTests(unittest.TestCase):
    def config(self):
        return {
            "duration_s": 7200,
            "warmup_s": 0,
            "gates": {
                "latency_window_s": 1800,
                "leak_window_s": 1800,
                "min_leak_windows": 2,
                "max_api_p95_ms": 500,
                "max_api_p99_ms": 1000,
                "max_rss_growth_fraction": 0.25,
                "max_goroutine_growth": 5,
                "max_nats_store_growth_bytes": 1000,
                "max_recovery_s": {"api": 30},
                "max_sample_gap_s": 20,
                "max_nats_store_fraction": 0.85,
                "nats_store_capacity_bytes": 4000,
            },
        }

    def sample(self, elapsed, latency=100, goroutines=10, rss=1000, store=100):
        return {
            "seq": int(elapsed / 10) + 1,
            "epoch": 1_000_000 + elapsed,
            "elapsed_s": elapsed,
            "api": {"routes": {"health": {"status": 200, "latency_ms": latency, "timeout": False}},
                    "runtime": {"goroutines": goroutines}},
            "containers": {"api": {"rss_bytes": rss}},
            "nats": {"nats-0": {"store_bytes": store, "ack_pending": 0, "healthy": True, "routes": 2}},
        }

    def test_latency_distribution_uses_nearest_rank_per_route(self):
        self.assertEqual(soak.percentile([1, 2, 3, 4], 0.50), 2)
        self.assertEqual(soak.percentile([1, 2, 3, 4], 0.95), 4)
        self.assertIsNone(soak.percentile([], 0.95))
        samples = [self.sample(i * 10, latency=v) for i, v in enumerate([10, 20, 30, 40])]
        report = soak.build_report({"run_id": "r"}, self.config(), samples, [], "completed")
        self.assertEqual(report["summary"]["api_latency_ms"]["health"]["p95"], 40)
        self.assertEqual(report["summary"]["api_latency_ms"]["health"]["count"], 4)

    def test_pre_registered_leak_and_latency_gates_fail(self):
        samples = [self.sample(0), self.sample(1800, latency=900, goroutines=20, rss=2000, store=2000),
                   self.sample(3600, latency=900, goroutines=30, rss=3000, store=3000)]
        report = soak.build_report({"run_id": "r"}, self.config(), samples, [], "completed")
        self.assertFalse(report["gates"]["api_latency_p95"]["passed"])
        self.assertFalse(report["gates"]["api_goroutine_growth"]["passed"])
        self.assertFalse(report["gates"]["api_rss_growth"]["passed"])
        self.assertFalse(report["gates"]["nats_store_growth"]["passed"])
        self.assertFalse(report["overall_pass"])

    def test_restart_gate_requires_exactly_one_measured_recovery(self):
        events = [{"seq": 1, "event": "restart_started", "action_id": "a1", "kind": "api"},
                  {"seq": 2, "event": "restart_recovered", "action_id": "a1", "kind": "api", "recovery_s": 4.5}]
        samples = [self.sample(0), self.sample(10)]
        report = soak.build_report({"run_id": "r"}, self.config(), samples, events, "completed")
        self.assertTrue(report["gates"]["restart_recovery"]["passed"])
        self.assertEqual(report["summary"]["restarts"][0]["recovery_s"], 4.5)
        report = soak.build_report({"run_id": "r"}, self.config(), samples, events[:1], "completed")
        self.assertFalse(report["gates"]["restart_recovery"]["passed"])
        self.assertFalse(report["overall_pass"])

    def test_restart_gate_rejects_recovery_over_budget(self):
        events = [{"event": "restart_started", "action_id": "a1", "kind": "api"},
                  {"event": "restart_recovered", "action_id": "a1", "kind": "api", "recovery_s": 31}]
        report = soak.build_report({"run_id": "r"}, self.config(), [self.sample(0)], events, "completed")
        self.assertFalse(report["gates"]["restart_recovery"]["passed"])

    def test_incomplete_run_cannot_pass(self):
        report = soak.build_report({"run_id": "r"}, self.config(), [self.sample(0)], [], "running")
        self.assertFalse(report["overall_pass"])
        self.assertEqual(report["status"], "running")
        self.assertFalse(report["gates"]["completed_duration"]["passed"])

    def test_disk_budget_reports_host_artifact_and_nats_violations(self):
        config = self.config()
        config["disk"] = {"min_host_free_bytes": 1000, "max_artifact_bytes": 2000}
        violations = soak.disk_budget_violations(
            {"host_free_bytes": 999, "artifact_bytes": 2001},
            {"nats-0": {"store_bytes": 3401}}, config)
        self.assertEqual(len(violations), 3)
        self.assertTrue(any("host free" in v for v in violations))
        self.assertTrue(any("artifact" in v for v in violations))
        self.assertTrue(any("nats-0" in v for v in violations))

    def test_planned_outage_samples_are_not_counted_as_steady_state_failures(self):
        samples = [self.sample(0), self.sample(10), self.sample(20)]
        samples[1]["outage_id"] = "planned-a1"
        samples[1]["api"]["routes"]["health"].update(status=503, timeout=True)
        events = [{"event": "restart_started", "action_id": "a1", "kind": "api"},
                  {"event": "restart_recovered", "action_id": "a1", "kind": "api", "recovery_s": 2}]
        report = soak.build_report({"run_id": "r"}, self.config(), samples, events, "completed")
        self.assertEqual(report["summary"]["steady_state_api_requests"], 2)
        self.assertEqual(report["summary"]["planned_outage_api_requests"], 1)
        self.assertTrue(report["gates"]["api_errors"]["passed"])


class ActionBookTests(unittest.TestCase):
    def test_started_state_is_durable_before_action_execution(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            journal = soak.EventJournal(root / "events.jsonl")
            state = {"actions": {"a1": {"status": "pending", "due_epoch": 10}}}
            soak.atomic_write_json(root / "state.json", state)
            book = soak.ActionBook(root, journal)
            observed = []
            def action():
                observed.append(json.loads((root / "state.json").read_text())["actions"]["a1"]["status"])
            book.execute("a1", action)
            self.assertEqual(observed, ["started"])
            self.assertEqual(book.state["actions"]["a1"]["status"], "completed")
            self.assertTrue(book.state["actions"]["a1"]["finished_epoch"] >= book.state["actions"]["a1"]["started_epoch"])

    def test_failed_action_remains_unknown_and_cannot_be_blindly_replayed(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            journal = soak.EventJournal(root / "events.jsonl")
            soak.atomic_write_json(root / "state.json", {"actions": {"a1": {"status": "pending", "due_epoch": 10}}})
            book = soak.ActionBook(root, journal)
            with self.assertRaisesRegex(RuntimeError, "injected"):
                book.execute("a1", lambda: (_ for _ in ()).throw(RuntimeError("injected")))
            self.assertEqual(book.state["actions"]["a1"]["status"], "started")


class ProbeTests(unittest.TestCase):
    def test_nats_snapshot_extracts_queue_health_replicas_and_goroutines(self):
        snapshot = soak.parse_nats_snapshot(
            {"mem": 123, "connections": 4, "slow_consumers": 2, "in_msgs": 10, "out_msgs": 20,
             "in_bytes": 100, "out_bytes": 200, "uptime": "1m0s"},
            {"memory": 50, "store": 200, "account_details": [{"stream_detail": [{
                "name": "FHIR", "state": {"messages": 10, "bytes": 100},
                "cluster": {"leader": "n0", "replicas": [{"name": "n1", "current": True},
                                                             {"name": "n2", "current": False, "lag": 3}]},
                "consumer_detail": [{"name": "consumer", "num_pending": 4, "num_ack_pending": 2,
                                      "num_redelivered": 1}],
            }]}]},
            {"num_routes": 2, "routes": [{"name": "n1"}, {"name": "n2"}]},
            "goroutine profile: total 42\n",
        )
        self.assertEqual(snapshot["rss_bytes"], 123)
        self.assertEqual(snapshot["queue_pending"], 4)
        self.assertEqual(snapshot["ack_pending"], 2)
        self.assertEqual(snapshot["redelivered"], 1)
        self.assertEqual(snapshot["goroutines"], 42)
        self.assertFalse(snapshot["replicas_current"])
        self.assertEqual(snapshot["streams"]["FHIR"]["messages"], 10)

    def test_http_probe_records_latency_status_bytes_and_json_without_throwing_on_500(self):
        import threading
        from http.server import BaseHTTPRequestHandler, HTTPServer
        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                body = b'{"status":"degraded"}'
                self.send_response(503)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
            def log_message(self, *_):
                pass
        server = HTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            probe = soak.probe_json("http://127.0.0.1:%d/healthz" % server.server_port, 2)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()
        self.assertEqual(probe["status"], 503)
        self.assertEqual(probe["body"], {"status": "degraded"})
        self.assertEqual(probe["bytes"], len(b'{"status":"degraded"}'))
        self.assertGreaterEqual(probe["latency_ms"], 0)
        self.assertFalse(probe["timeout"])

    def test_http_probe_rejects_empty_success_response(self):
        import threading
        from http.server import BaseHTTPRequestHandler, HTTPServer
        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                self.send_response(204)
                self.end_headers()
            def log_message(self, *_):
                pass
        server = HTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            probe = soak.probe_json("http://127.0.0.1:%d/healthz" % server.server_port, 2)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()
        self.assertEqual(probe["status"], 204)
        self.assertEqual(probe["bytes"], 0)
        self.assertEqual(probe["error"], "empty JSON response")


class ReportFileTests(unittest.TestCase):
    def test_report_generator_writes_machine_readable_partial_report(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            config = GateTests().config()
            manifest = {"run_id": "short", "project": "waslsoak-short", "config": config}
            soak.atomic_write_json(root / "manifest.json", manifest)
            writer = soak.SampleWriter(root, clock=lambda: 1_000_000)
            writer.append(GateTests().sample(0))
            writer.close()
            soak.EventJournal(root / "events.jsonl").append("controller_started")
            report = soak.write_report(root, "running")
            self.assertFalse(report["overall_pass"])
            self.assertEqual(report["status"], "running")
            self.assertEqual(json.loads((root / "report.json").read_text())["run_id"], "short")
            self.assertIn("INCOMPLETE", (root / "report.txt").read_text())


class CollectorTests(unittest.TestCase):
    def test_failed_probes_still_create_a_sample_and_budget_signal(self):
        class BrokenCompose:
            def service_ids(self):
                raise RuntimeError("engine unavailable")
            def exec(self, *args):
                raise RuntimeError("engine unavailable")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            config = GateTests().config()
            config.update({"health_interval_s": 0, "api_interval_s": 0, "nats_interval_s": 0,
                           "container_interval_s": 0, "disk_interval_s": 0, "request_timeout_s": .05,
                           "disk": {"min_host_free_bytes": 0, "max_artifact_bytes": 1000000}})
            manifest = {"run_id": "r", "project": "waslsoak-r", "start_epoch": 1_000_000,
                        "api_url": "http://127.0.0.1:9", "config": config}
            writer = soak.SampleWriter(root)
            journal = soak.EventJournal(root / "events.jsonl")
            collector = soak.Collector(root, manifest, BrokenCompose(), writer, journal)
            row = collector.sample_once()
            writer.close()
            self.assertEqual(row["seq"], 1)
            self.assertTrue(row["errors"])
            self.assertEqual(row["api"]["routes"]["health"]["status"], 0)
            self.assertTrue((root / "samples").exists())

    def test_unexpected_restart_detection_ignores_expected_pipeline_exit(self):
        previous = {"api": {"started_at": "a", "restart_count": 0, "container_id": "old"},
                    "pipeline": {"started_at": "p", "restart_count": 0, "container_id": "p"}}
        current = {"api": {"started_at": "b", "restart_count": 0, "container_id": "old"},
                   "pipeline": {"started_at": "p", "restart_count": 0, "container_id": "p",
                                "status": "exited", "exit_code": 0}}
        self.assertEqual(soak.detect_unexpected_restarts(previous, current), ["api"])
        current["api"]["started_at"] = "a"
        self.assertEqual(soak.detect_unexpected_restarts(previous, current), [])


class ConfigTests(unittest.TestCase):
    def config(self):
        config = GateTests().config()
        config.update({"evaluation_scope": "24-hour retained-read", "duration_s": 86400,
                       "health_interval_s": 10, "api_interval_s": 30, "nats_interval_s": 30,
                       "container_interval_s": 60, "disk_interval_s": 60, "request_timeout_s": 5,
                       "disk": {"min_host_free_bytes": 1, "max_artifact_bytes": 1000000},
                       "schedule": [{"id": "a1", "at_s": 60, "kind": "api_restart"}]})
        return config

    def test_full_soak_must_pre_register_24_hours_and_unique_ordered_schedule(self):
        config = self.config()
        soak.validate_config(config)
        state = soak.initial_state("abc", config, 1000)
        self.assertEqual(state["actions"]["a1"]["due_epoch"], 1060)
        config["duration_s"] = 60
        with self.assertRaisesRegex(ValueError, "86400"):
            soak.validate_config(config)

    def test_duplicate_action_ids_are_rejected(self):
        config = self.config()
        config["schedule"].append(dict(config["schedule"][0]))
        with self.assertRaisesRegex(ValueError, "unique"):
            soak.validate_config(config)


class EvidenceTests(unittest.TestCase):
    def test_export_hash_verification_and_restart_comparison(self):
        import hashlib
        unsigned = {"schema": "wasl-case-export/1", "comparison_sha256": "stable", "value": 1.25}
        digest = hashlib.sha256(soak.export_canonical(unsigned)).hexdigest()
        raw = json.dumps(dict(unsigned, content_sha256=digest)).encode()
        verified = soak.verify_export(raw)
        self.assertTrue(verified["content_hash_valid"])
        self.assertEqual(verified["comparison_sha256"], "stable")
        changed = json.dumps(dict(unsigned, comparison_sha256="drift", content_sha256=digest)).encode()
        comparison = soak.compare_exports({"fixture": raw}, {"fixture": changed})
        self.assertFalse(comparison["matched"])
        self.assertIn("fixture", comparison["mismatches"])

    def test_pipeline_invariants_cover_wasl_lanes_not_intentional_baselines(self):
        report = {
            "closure": {"crc": {"baseline": {"false_closures": 3}, "wasl_live_with_review": {"false_closures": 0}}},
            "consent_timing": {"released_without_granted_consent": 0, "released_after_opt_out_on_file": 0},
            "crash_recovery": {name: 0 for name in ("duplicate_outputs", "lost_outputs", "unexpected_outputs",
                "final_state_mismatch_loops", "ingest_duplicates", "ingest_lost", "prep_duplicates", "prep_lost",
                "prep_content_mismatch", "logical_duplicate_decisions")},
            "outreach_live": {"wrong_patient_released": 0},
            "outreach_no_review": {"wrong_patient_released": 0},
        }
        self.assertEqual(soak.pipeline_invariants(report, "quiescent: complete"), [])
        report["closure"]["crc"]["wasl_live_with_review"]["false_closures"] = 1
        report["crash_recovery"]["lost_outputs"] = 2
        violations = soak.pipeline_invariants(report, "timeout waiting for quiescence")
        self.assertEqual(len(violations), 3)
        self.assertTrue(any("quiescence" in item for item in violations))


class RestartMeasurementTests(unittest.TestCase):
    def test_measured_restart_has_one_terminal_event_and_clears_outage(self):
        with tempfile.TemporaryDirectory() as tmp:
            journal = soak.EventJournal(Path(tmp) / "events.jsonl")
            class Collector:
                outage = "unchanged"
                def set_outage(self, value):
                    self.outage = value
            collector = Collector()
            soak.measured_restart(journal, collector, "a1", "api", lambda: None, lambda: None)
            self.assertIsNone(collector.outage)
            events = soak.read_events(Path(tmp) / "events.jsonl")
            self.assertEqual([row["event"] for row in events], ["restart_started", "restart_recovered"])
            self.assertGreaterEqual(events[1]["recovery_s"], 0)

    def test_failed_restart_is_not_reported_as_recovered(self):
        with tempfile.TemporaryDirectory() as tmp:
            journal = soak.EventJournal(Path(tmp) / "events.jsonl")
            class Collector:
                def set_outage(self, value):
                    self.outage = value
            collector = Collector()
            with self.assertRaisesRegex(RuntimeError, "still down"):
                soak.measured_restart(journal, collector, "a1", "api", lambda: None,
                                      lambda: (_ for _ in ()).throw(RuntimeError("still down")))
            self.assertIsNone(collector.outage)
            events = soak.read_events(Path(tmp) / "events.jsonl")
            self.assertEqual([row["event"] for row in events], ["restart_started", "restart_failed"])


class LifecycleTests(unittest.TestCase):
    def runner(self, tmp, compose):
        runner = soak.SoakRunner.__new__(soak.SoakRunner)
        runner.compose = compose
        runner.journal = soak.EventJournal(Path(tmp) / "events.jsonl")
        runner.config = {"outage_s": 0}
        class Collector:
            def set_outage(self, value):
                self.outage = value
        runner.collector = Collector()
        runner.recover_read = lambda: None
        return runner

    def test_recreation_never_starts_the_one_shot_pipeline(self):
        class FakeCompose:
            def __init__(self):
                self.calls = []
            def down(self):
                self.calls.append(("down",))
            def up_read_stack(self):
                self.calls.append(("up", "nats-0", "nats-1", "nats-2", "api"))
        compose = FakeCompose()
        with tempfile.TemporaryDirectory() as tmp:
            runner = self.runner(tmp, compose)
            runner.recreate_read_stack("recreate")
        self.assertEqual(compose.calls, [("down",), ("up", "nats-0", "nats-1", "nats-2", "api")])
        self.assertNotIn("pipeline", compose.calls[-1])

    def test_nats_restart_stops_and_starts_only_the_declared_node(self):
        class FakeCompose:
            def __init__(self):
                self.calls = []
            def stop(self, node):
                self.calls.append(("stop", node))
            def start(self, node):
                self.calls.append(("start", node))
        compose = FakeCompose()
        with tempfile.TemporaryDirectory() as tmp:
            runner = self.runner(tmp, compose)
            runner.nats_restart("nats-action", "nats-2")
        self.assertEqual(compose.calls, [("stop", "nats-2"), ("start", "nats-2")])


class CreateRunTests(unittest.TestCase):
    def test_create_run_initializes_a_missing_output_parent(self):
        import subprocess
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            repo = root / "repo"
            (repo / "deploy/nats").mkdir(parents=True)
            (repo / "scripts").mkdir()
            for name in ("docker-compose.yml", "Dockerfile", "deploy/nats/nats.conf", "deploy/nats/entrypoint.sh"):
                (repo / name).write_text("test\n")
            subprocess.run(["git", "init", "-q", str(repo)], check=True)
            subprocess.run(["git", "-C", str(repo), "-c", "user.email=test@example.invalid", "-c", "user.name=test",
                            "commit", "--allow-empty", "-qm", "base"], check=True)
            engine = root / "engine"
            engine.write_text("#!/bin/sh\nexit 0\n")
            engine.chmod(0o755)
            config = ConfigTests().config()
            config["require_clean_source"] = False
            output = root / "missing/nested/runs"
            run_dir = soak.create_run(config, output, str(engine), repo)
            self.assertTrue((run_dir / "manifest.json").exists())
            self.assertEqual((run_dir / ".env").stat().st_mode & 0o777, 0o600)


class SafetyGateTests(unittest.TestCase):
    def test_transport_failure_and_disk_budget_violation_fail_final_gates(self):
        config = GateTests().config()
        healthy = GateTests().sample(0)
        failed = GateTests().sample(10)
        failed["api"]["routes"]["health"].update(status=0, error="connection refused")
        failed["budget_violations"] = ["host free bytes below pre-registered budget"]
        report = soak.build_report({"run_id": "r"}, config, [healthy, failed], [], "completed")
        self.assertFalse(report["gates"]["api_errors"]["passed"])
        self.assertFalse(report["gates"]["disk_budget"]["passed"])

    def test_unhealthy_nats_outside_declared_window_fails(self):
        config = GateTests().config()
        sample = GateTests().sample(0)
        sample["nats"]["nats-0"]["healthy"] = False
        report = soak.build_report({"run_id": "r"}, config, [sample], [], "completed")
        self.assertFalse(report["gates"]["nats_health"]["passed"])


class CommandDiagnosticsTests(unittest.TestCase):
    def test_command_error_preserves_bounded_build_diagnostics(self):
        import subprocess
        result = subprocess.CompletedProcess(["podman", "compose", "build"], 1,
                                             stdout="build stdout", stderr="exact compiler failure")
        error = soak.CommandError(result)
        self.assertIn("exact compiler failure", str(error))
        self.assertIn("build stdout", str(error))


class ImageEvidenceTests(unittest.TestCase):
    def test_image_ids_use_engine_inspection_and_exclude_secret_env(self):
        class Result:
            def __init__(self, value):
                self.stdout = value + "\n"
        class FakeCompose:
            engine = "podman"
            def __init__(self):
                self.calls = []
            def run(self, args, timeout):
                self.calls.append(args)
                return Result("sha256:" + ("a" * 64))
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / ".env").write_text("WASL_JS_KEY=kdo-not-archive\nWASL_IMAGE=wasl-soak-test:local\n"
                                       "WASL_NATS_IMAGE=wasl-nats-soak-test:local\n")
            runner = soak.SoakRunner.__new__(soak.SoakRunner)
            runner.root = root
            runner.compose = FakeCompose()
            runner.record_image_ids()
            evidence = (root / "images.json").read_text()
            self.assertIn("wasl-soak-test:local", evidence)
            self.assertIn("wasl-nats-soak-test:local", evidence)
            self.assertNotIn("do-not-archive", evidence)
            self.assertTrue(all("image" in call for call in runner.compose.calls))


class BootstrapTests(unittest.TestCase):
    def test_bootstrap_up_uses_the_pre_registered_pipeline_budget(self):
        class FakeCompose:
            def __init__(self):
                self.calls = []
            def compose(self, *args, timeout):
                self.calls.append((args, timeout))
        with tempfile.TemporaryDirectory() as tmp:
            runner = soak.SoakRunner.__new__(soak.SoakRunner)
            runner.root = Path(tmp)
            runner.compose = FakeCompose()
            runner.config = {"bootstrap_timeout_s": 1200, "run_container_path": "/run", "schedule": []}
            runner.manifest = {"project": "waslsoak-test", "api_url": "http://127.0.0.1:1"}
            runner.journal = soak.EventJournal(Path(tmp) / "events.jsonl")
            import types
            runner.state = {"actions": {}}
            runner.book = types.SimpleNamespace(state=runner.state, save=lambda: None)
            runner.collector = types.SimpleNamespace(set_phase=lambda phase: None)
            runner.record_image_ids = lambda: None
            runner.wait_bootstrap = lambda: None
            runner.wait_nats = lambda: None
            runner.run_api_smoke = lambda *args: None
            runner.save_artifacts = lambda *args: None
            runner.freeze_exports = lambda *args: None
            runner.bootstrap()
            up = next(item for item in runner.compose.calls if item[0][0] == "up")
            self.assertEqual(up[1], 1200)


class CurrentNATSSchemaTests(unittest.TestCase):
    def test_storage_unique_route_members_and_raw_stacks_are_supported(self):
        snapshot = soak.parse_nats_snapshot(
            {}, {"storage": 156815942, "account_details": []},
            {"num_routes": 4, "routes": [{"remote_name": "n1"}, {"remote_name": "n1"},
                                          {"remote_name": "n2"}, {"remote_name": "n2"}]},
            "goroutine 1 [running]:\nframe\ngoroutine 2 [wait]:\nframe\n")
        self.assertEqual(snapshot["store_bytes"], 156815942)
        self.assertEqual(snapshot["routes"], 2)
        self.assertEqual(snapshot["raw_route_connections"], 4)
        self.assertEqual(snapshot["route_names"], ["n1", "n2"])
        self.assertEqual(snapshot["goroutines"], 2)

    def test_bootstrap_samples_do_not_count_as_steady_state_transport_errors(self):
        row = GateTests().sample(0)
        row["phase"] = "bootstrap"
        row["api"]["routes"]["health"].update(status=0, error="connection refused")
        report = soak.build_report({"run_id": "r"}, GateTests().config(), [row, GateTests().sample(10)], [], "completed")
        self.assertEqual(report["summary"]["steady_state_api_requests"], 1)
        self.assertTrue(report["gates"]["api_errors"]["passed"])


class ContainerStatsSchemaTests(unittest.TestCase):
    def test_podman_lowercase_stats_are_normalized(self):
        stat = {"id": "12dd1fbb7526", "cpu_percent": "120.12%", "mem_usage": "2.005GB / 8.287GB",
                "net_io": "300.3MB / 182.9MB", "block_io": "6.631MB / 168.1MB", "pids": "11"}
        normalized = soak.normalize_container_stat(stat)
        self.assertEqual(normalized["cpu_percent"], 120.12)
        self.assertEqual(normalized["rss_bytes"], 2005000000)
        self.assertEqual(normalized["memory_limit_bytes"], 8287000000)
        self.assertEqual(normalized["network_input_bytes"], 300300000)
        self.assertEqual(normalized["network_output_bytes"], 182900000)
        self.assertEqual(normalized["block_input_bytes"], 6631000)
        self.assertEqual(normalized["block_output_bytes"], 168100000)
        self.assertEqual(normalized["pids"], 11)


class LateScheduleTests(unittest.TestCase):
    def test_quorum_loss_is_awaited_until_dependency_failure_is_observed(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            runner = soak.SoakRunner.__new__(soak.SoakRunner)
            runner.journal = soak.EventJournal(Path(tmp) / "events.jsonl")
            runner.current_action_id = "quorum"
            runner.manifest = {"api_url": "http://127.0.0.1:9"}
            runner.config = {"request_timeout_s": 1}
            outcomes = iter([True, False])
            runner.nats_ready = lambda: (next(outcomes), {})
            with patch("soak.probe_json", return_value={"status": 200}), patch("soak.time.sleep", return_value=None):
                runner.assert_quorum_loss()
            self.assertEqual(runner.current_action_id, "quorum")

    def test_artifact_copy_uses_api_container_after_pipeline_removal(self):
        import types
        class FakeCompose:
            engine = "podman"
            def service_ids(self):
                return {"api": "api-container"}
            def run(self, args, timeout):
                destination = Path(args[-1])
                if destination.name == "report7.json":
                    report = {
                        "closure": {"crc": {"wasl_live_with_review": {"false_closures": 0}}},
                        "consent_timing": {"released_without_granted_consent": 0, "released_after_opt_out_on_file": 0},
                        "crash_recovery": {name: 0 for name in ("duplicate_outputs", "lost_outputs", "unexpected_outputs",
                            "final_state_mismatch_loops", "ingest_duplicates", "ingest_lost", "prep_duplicates", "prep_lost",
                            "prep_content_mismatch", "logical_duplicate_decisions")},
                        "outreach": {"wrong_patient_released": 0}, "outreach_no_review": {"wrong_patient_released": 0}}
                    destination.write_text(json.dumps(report))
                elif destination.name == "orchestrate.json":
                    destination.write_text("{}")
                elif destination.name == "tamper.json":
                    destination.write_text(json.dumps({"attacks": [{"attack": "none (baseline)", "detected": False},
                                                                  {"attack": "test", "detected": True}]}))
                else:
                    destination.write_text("")
                return types.SimpleNamespace(stdout="")
            def compose(self, *args, timeout):
                return types.SimpleNamespace(stdout="quiescent")
        with tempfile.TemporaryDirectory() as tmp:
            runner = soak.SoakRunner.__new__(soak.SoakRunner)
            runner.root = Path(tmp)
            runner.compose = FakeCompose()
            runner.config = {"run_container_path": "/run"}
            runner.save_artifacts("final")
            evidence = json.loads((Path(tmp) / "final" / "pipeline-invariants.json").read_text())
            self.assertTrue(evidence["passed"])


class ActiveLaneTests(unittest.TestCase):
    def test_active_lane_starts_pipeline_without_dependency_wait(self):
        class FakeCompose:
            def __init__(self):
                self.calls = []
            def compose(self, *args, timeout):
                self.calls.append(args)
        runner = soak.SoakRunner.__new__(soak.SoakRunner)
        runner.compose = FakeCompose()
        runner.wait_nats_processes = lambda compose: None
        runner.start_active_lane(runner.compose)
        self.assertEqual(runner.compose.calls, [
            ("up", "-d", "--no-build", "--no-deps", "nats-0", "nats-1", "nats-2"),
            ("up", "-d", "--no-build", "--no-deps", "pipeline"),
        ])

    def test_bootstrap_waits_for_current_replicas_before_freezing(self):
        class FakeCompose:
            def compose(self, *args, timeout):
                return None
        with tempfile.TemporaryDirectory() as tmp:
            runner = soak.SoakRunner.__new__(soak.SoakRunner)
            runner.root = Path(tmp)
            runner.compose = FakeCompose()
            runner.config = {"bootstrap_timeout_s": 3600, "run_container_path": "/run", "schedule": []}
            runner.manifest = {"project": "waslsoak-test", "api_url": "http://127.0.0.1:1"}
            runner.journal = soak.EventJournal(Path(tmp) / "events.jsonl")
            import types
            runner.state = {"actions": {}}
            runner.book = types.SimpleNamespace(state=runner.state, save=lambda: None)
            calls = []
            runner.collector = types.SimpleNamespace(set_phase=lambda phase: None)
            runner.record_image_ids = lambda: None
            runner.wait_bootstrap = lambda: None
            runner.wait_nats = lambda: calls.append("nats-ready")
            runner.run_api_smoke = lambda *args: calls.append("api-smoke")
            runner.save_artifacts = lambda *args: None
            runner.freeze_exports = lambda *args: None
            runner.bootstrap()
            self.assertEqual(calls, ["nats-ready", "api-smoke"])


class TamperEvidenceTests(unittest.TestCase):
    def test_every_non_baseline_attack_must_be_detected(self):
        evidence = {"attacks": [{"attack": "none (baseline)", "detected": False},
                                {"attack": "FHIR log: reorder", "detected": True}]}
        self.assertEqual(soak.tamper_invariants(evidence), [])
        evidence["attacks"][1]["detected"] = False
        self.assertEqual(len(soak.tamper_invariants(evidence)), 1)


class SteadyStateTimingTests(unittest.TestCase):
    def test_bootstrap_reanchors_pending_actions_to_retained_read_start(self):
        import types
        from unittest.mock import patch
        class FakeCompose:
            def compose(self, *args, timeout):
                return None
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            config = ConfigTests().config()
            config["bootstrap_timeout_s"] = 1200
            config["run_container_path"] = "/run"
            config["schedule"].insert(0, {"id": "bootstrap", "at_s": 0, "kind": "bootstrap"})
            state = soak.initial_state("abc", config, 1000)
            runner = soak.SoakRunner.__new__(soak.SoakRunner)
            runner.root = root
            runner.compose = FakeCompose()
            runner.config = config
            runner.manifest = {"project": "waslsoak-test", "api_url": "http://127.0.0.1:1"}
            runner.journal = soak.EventJournal(root / "events.jsonl")
            runner.state = state
            runner.book = types.SimpleNamespace(state=state, save=lambda: None)
            runner.collector = types.SimpleNamespace(set_phase=lambda phase: None)
            runner.record_image_ids = lambda: None
            runner.wait_bootstrap = lambda: None
            runner.wait_nats = lambda: None
            runner.run_api_smoke = lambda *args: None
            runner.save_artifacts = lambda *args: None
            runner.freeze_exports = lambda *args: None
            with patch("soak.time.time", return_value=5000):
                runner.bootstrap()
            self.assertEqual(state.get("steady_start_epoch"), 5000)
            self.assertEqual(state["actions"]["a1"]["due_epoch"], 5060)

    def test_duration_gate_excludes_bootstrap_elapsed_time(self):
        config = GateTests().config()
        bootstrap = GateTests().sample(10000)
        bootstrap["phase"] = "bootstrap"
        steady = GateTests().sample(10)
        report = soak.build_report({"run_id": "r"}, config, [bootstrap, steady], [], "completed")
        self.assertFalse(report["gates"]["completed_duration"]["passed"])
        self.assertEqual(report["gates"]["completed_duration"]["observed_s"], 10)

    def test_completion_deadline_uses_persisted_steady_start(self):
        import types
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            runner = soak.SoakRunner.__new__(soak.SoakRunner)
            runner.root = root
            runner.manifest = {"start_epoch": 100, "project": "waslsoak-test"}
            runner.config = {"duration_s": 10, "disposable": False}
            runner.state = {"status": "running", "steady_start_epoch": 1000, "actions": {}}
            runner.book = types.SimpleNamespace(state=runner.state, save=lambda: None)
            runner.journal = soak.EventJournal(root / "events.jsonl")
            runner.collector = types.SimpleNamespace(run=lambda: None, stop=lambda: None,
                                                       sample_once=lambda: None, budget_violations=[])
            runner.writer = types.SimpleNamespace(close=lambda: None)
            runner.lock = types.SimpleNamespace(acquire=lambda: None, close=lambda: None)
            runner.compose = types.SimpleNamespace(compose=lambda *args, **kwargs: None)
            deadlines = []
            runner.wait_until = deadlines.append
            with patch("soak.write_report", return_value={"overall_pass": True}):
                self.assertEqual(runner.run(), 0)
            self.assertEqual(deadlines, [1010])


class SchedulerStateTests(unittest.TestCase):
    def test_runner_and_action_book_share_one_state_object(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            manifest = {"engine": "podman", "project": "waslsoak-test", "run_id": "r",
                        "api_url": "http://127.0.0.1:1", "start_epoch": 0, "config": {}}
            soak.atomic_write_json(root / "manifest.json", manifest)
            soak.atomic_write_json(root / "state.json", {"actions": {}})
            runner = soak.SoakRunner(root)
            self.assertIs(runner.book.state, runner.state)


class ManifestAuthenticationTests(unittest.TestCase):
    def test_resume_recomputes_the_frozen_manifest_digest(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            manifest = {"run_id": "r", "project": "waslsoak-test",
                        "config": {"duration_s": 86400, "gates": {"max_api_p95_ms": 500}}}
            manifest["manifest_sha256"] = soak.manifest_digest(manifest)
            soak.atomic_write_json(root / "state.json", {
                "manifest_sha256": manifest["manifest_sha256"], "actions": {}})
            manifest["config"]["duration_s"] = 1
            with self.assertRaisesRegex(soak.ResumeError, "digest"):
                soak.resume_state(root / "state.json", manifest)


class EvidenceCompletenessTests(unittest.TestCase):
    def test_health_only_observations_do_not_satisfy_route_coverage_or_latency(self):
        report = soak.build_report({"run_id": "r"}, GateTests().config(),
                                   [GateTests().sample(0), GateTests().sample(10)], [], "completed")
        self.assertIn("api_route_coverage", report["gates"])
        self.assertFalse(report["gates"]["api_route_coverage"]["passed"])
        self.assertFalse(report["gates"]["api_latency_p95"]["passed"])

    def test_client_side_http_errors_fail_api_gate(self):
        sample = GateTests().sample(0)
        sample["api"]["routes"]["health"]["status"] = 404
        report = soak.build_report({"run_id": "r"}, GateTests().config(), [sample], [], "completed")
        self.assertFalse(report["gates"]["api_errors"]["passed"])

    def test_zero_steady_api_requests_cannot_pass(self):
        sample = GateTests().sample(0)
        sample["phase"] = "bootstrap"
        report = soak.build_report({"run_id": "r"}, GateTests().config(), [sample], [], "completed")
        self.assertFalse(report["gates"]["api_errors"]["passed"])

    def test_zero_export_comparisons_cannot_pass(self):
        report = soak.build_report({"run_id": "r"}, GateTests().config(),
                                   [GateTests().sample(0)], [], "completed")
        self.assertFalse(report["gates"]["export_comparison"]["passed"])

    def test_zero_steady_nats_observations_cannot_pass(self):
        sample = GateTests().sample(0)
        sample["phase"] = "bootstrap"
        report = soak.build_report({"run_id": "r"}, GateTests().config(), [sample], [], "completed")
        self.assertFalse(report["gates"]["nats_health"]["passed"])
        self.assertFalse(report["gates"]["nats_ack_pending"]["passed"])

    def test_nats_health_requires_current_replicas(self):
        sample = GateTests().sample(0)
        sample["nats"]["nats-0"]["replicas_current"] = False
        report = soak.build_report({"run_id": "r"}, GateTests().config(), [sample], [], "completed")
        self.assertFalse(report["gates"]["nats_health"]["passed"])

    def test_nats_growth_ignores_cadence_slots_without_nats_payloads(self):
        samples = [GateTests().sample(0, store=100), GateTests().sample(10), GateTests().sample(20),
                   GateTests().sample(1800, store=5000), GateTests().sample(1810), GateTests().sample(1820)]
        for sample in samples:
            if sample["elapsed_s"] not in (0, 1800):
                sample["nats"] = {}
        report = soak.build_report({"run_id": "r"}, GateTests().config(), samples, [], "completed")
        self.assertFalse(report["gates"]["nats_store_growth"]["passed"])
        self.assertEqual(report["gates"]["nats_store_growth"]["observed"], 4900)


class NATSReplicaTests(unittest.TestCase):
    def snapshot_with_replicas(self, replicas):
        return soak.parse_nats_snapshot({}, {"account_details": [{"stream_detail": [{
            "name": "FHIR", "state": {}, "cluster": {"leader": "n0", "replicas": replicas},
        }]}]}, {"routes": [{"name": "n1"}, {"name": "n2"}]}, "")

    def test_empty_replica_list_is_not_current(self):
        self.assertFalse(self.snapshot_with_replicas([])["replicas_current"])

    def test_one_current_peer_is_not_a_three_node_replicated_stream(self):
        self.assertFalse(self.snapshot_with_replicas([{"name": "n1", "current": True}])["replicas_current"])

    def test_two_current_peers_are_required(self):
        snapshot = self.snapshot_with_replicas([{"name": "n1", "current": True},
                                                {"name": "n2", "current": True}])
        self.assertTrue(snapshot["replicas_current"])


class LeaderAuthoritativeNATSTests(unittest.TestCase):
    def raw_views(self):
        views = {}
        stream_names = ("FHIR", "PREP", "FACTS", "OUT")
        for observer in range(3):
            details = []
            for index, name in enumerate(stream_names):
                leader = index % 3
                replicas = [{"name": "n%d" % peer,
                             "current": observer == leader or peer == leader,
                             "lag": 1000 if observer != leader and peer != leader else None}
                            for peer in range(3) if peer != observer]
                details.append({"name": name, "state": {"messages": 10, "bytes": 100},
                                "cluster": {"leader": "n%d" % leader, "replicas": replicas}})
            views["nats-%d" % observer] = {"account_details": [{"stream_detail": details}]}
        return views

    def parsed_views(self, views):
        parsed = {}
        for node, jsz in views.items():
            snapshot = soak.parse_nats_snapshot({}, jsz, {"routes": [{"name": "n1"}, {"name": "n2"}]}, "")
            snapshot.update(healthy=True, routes=2, ack_pending=0, store_bytes=100)
            parsed[node] = snapshot
        return parsed

    def readiness(self, views):
        import types
        class FakeCompose:
            def exec_result(self, node, args, timeout=10):
                return types.SimpleNamespace(returncode=0)
            def exec(self, node, args, timeout=15):
                if "jsz?" in args[-1]:
                    return json.dumps(views[node])
                return json.dumps({"routes": [{"name": "n1"}, {"name": "n2"}]})
        runner = soak.SoakRunner.__new__(soak.SoakRunner)
        return runner.nats_ready(FakeCompose())

    def report_health(self, views):
        sample = GateTests().sample(0)
        sample["nats"] = self.parsed_views(views)
        report = soak.build_report({"run_id": "r"}, GateTests().config(), [sample], [], "completed")
        return report["gates"]["nats_health"]

    def test_readiness_ignores_follower_local_false_when_leaders_are_current(self):
        ready, details = self.readiness(self.raw_views())
        self.assertTrue(ready, details)

    def test_readiness_rejects_a_leader_reporting_non_current_peers(self):
        views = self.raw_views()
        views["nats-0"]["account_details"][0]["stream_detail"][0]["cluster"]["replicas"][0]["current"] = False
        ready, _ = self.readiness(views)
        self.assertFalse(ready)

    def test_readiness_rejects_a_missing_leader_authoritative_view(self):
        views = self.raw_views()
        views["nats-0"]["account_details"][0]["stream_detail"].pop(0)
        ready, _ = self.readiness(views)
        self.assertFalse(ready)

    def test_steady_health_uses_the_same_leader_authoritative_views(self):
        self.assertTrue(self.report_health(self.raw_views())["passed"])

    def test_steady_health_rejects_non_current_or_missing_leader_views(self):
        import copy
        for mutation in ("non-current", "missing"):
            with self.subTest(mutation=mutation):
                views = self.parsed_views(self.raw_views())
                leader_stream = views["nats-0"]["streams"]["FHIR"]
                if mutation == "non-current":
                    leader_stream["replicas_current"] = False
                    leader_stream["current_replica_peers"] = 1
                else:
                    del views["nats-0"]["streams"]["FHIR"]
                sample = GateTests().sample(0)
                sample["nats"] = views
                report = soak.build_report({"run_id": "r"}, GateTests().config(), [sample], [], "completed")
                self.assertFalse(report["gates"]["nats_health"]["passed"])

    def test_steady_health_preserves_health_routes_and_missing_node_checks(self):
        import copy
        baseline = self.parsed_views(self.raw_views())
        for mutation in ("health", "routes", "missing"):
            with self.subTest(mutation=mutation):
                views = copy.deepcopy(baseline)
                if mutation == "health":
                    views["nats-1"]["healthy"] = False
                elif mutation == "routes":
                    views["nats-2"]["routes"] = 1
                else:
                    del views["nats-1"]
                sample = GateTests().sample(0)
                sample["nats"] = views
                report = soak.build_report({"run_id": "r"}, GateTests().config(), [sample], [], "completed")
                self.assertFalse(report["gates"]["nats_health"]["passed"])


class RestartBaselineTests(unittest.TestCase):
    def test_zero_container_start_timestamp_is_not_an_unexpected_restart(self):
        previous = {"api": {"started_at": "0001-01-01T00:00:00Z", "restart_count": 0,
                            "container_id": "api", "status": "created"}}
        current = {"api": {"started_at": "2026-11-09T08:00:00Z", "restart_count": 0,
                           "container_id": "api", "status": "running"}}
        self.assertEqual(soak.detect_unexpected_restarts(previous, current), [])

    def test_collector_restores_sequence_and_container_generation_from_samples(self):
        import types
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            config = GateTests().config()
            manifest = {"run_id": "r", "project": "waslsoak-r", "start_epoch": 0, "config": config}
            writer = soak.SampleWriter(root, clock=lambda: 1000)
            prior = {"seq": 17, "containers": {"api": {"started_at": "2026-11-09T08:00:00Z",
                                                        "restart_count": 3, "container_id": "api"}}}
            writer.append(prior)
            writer.close()
            collector = soak.Collector(root, manifest, types.SimpleNamespace(), soak.SampleWriter(root),
                                       soak.EventJournal(root / "events.jsonl"))
            self.assertEqual(collector.seq, 17)
            self.assertEqual(collector.previous_containers, prior["containers"])


class TornJournalTests(unittest.TestCase):
    def test_event_journal_recovers_only_the_unterminated_trailing_record(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "events.jsonl"
            path.write_text('{"seq":1,"event":"first"}\n{"seq":2,"event":"trunc')
            try:
                journal = soak.EventJournal(path)
                journal.append("after_recovery")
                events = soak.read_events(path)
            except ValueError as exc:
                self.fail("torn final journal record blocked recovery: %s" % exc)
            self.assertEqual([row["event"] for row in events], ["first", "after_recovery"])
            self.assertEqual([row["seq"] for row in events], [1, 2])

    def test_sample_reader_and_writer_recover_a_torn_final_segment(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            segment = root / "samples" / "19700101T00.jsonl"
            segment.parent.mkdir()
            segment.write_text('{"seq":1}\n{"seq":2,"partial":')
            try:
                rows = soak.read_jsonl_segments(root)
                writer = soak.SampleWriter(root, clock=lambda: 0)
                writer.append({"seq": 2})
                writer.close()
                rows = soak.read_jsonl_segments(root)
            except ValueError as exc:
                self.fail("torn final sample record blocked recovery: %s" % exc)
            self.assertEqual(rows, [{"seq": 1}, {"seq": 2}])


class ExportCoverageTests(unittest.TestCase):
    def test_restart_comparison_rejects_a_fixture_missing_from_current_exports(self):
        import hashlib
        import types
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            exports = root / "baseline" / "exports"
            exports.mkdir(parents=True)
            unsigned = {"schema": "wasl-case-export/1", "comparison_sha256": "stable"}
            digest = hashlib.sha256(soak.export_canonical(unsigned)).hexdigest()
            raw = json.dumps(dict(unsigned, content_sha256=digest)).encode()
            for name in ("first.json", "second.json"):
                (exports / name).write_bytes(raw)
            soak.atomic_write_json(root / "baseline" / "artifact-hashes.json", {})
            runner = soak.SoakRunner.__new__(soak.SoakRunner)
            runner.root = root
            runner.journal = soak.EventJournal(root / "events.jsonl")
            runner.manifest = {"api_url": "http://127.0.0.1:1"}
            runner.config = {"request_timeout_s": 1}
            runner.freeze_exports = lambda destination: {"first": raw}
            runner.current_artifact_hashes = lambda: {}
            health = {"body": {"dependencies": {"review_ledger": "ok"}}}
            with patch("soak.probe_json", return_value=health):
                with self.assertRaisesRegex(RuntimeError, "comparison failed"):
                    runner.compare_baseline("restart")


class PartialContainerBaselineTests(unittest.TestCase):
    def test_resume_merges_the_last_observed_generation_for_each_service(self):
        import types
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            manifest = {"run_id": "r", "project": "waslsoak-r", "start_epoch": 0,
                        "config": GateTests().config()}
            writer = soak.SampleWriter(root, clock=lambda: 1000)
            api = {"started_at": "2026-11-09T08:00:00Z", "restart_count": 0, "container_id": "api"}
            writer.append({"seq": 1, "containers": {"api": api, "nats-0": {"container_id": "n0"}}})
            writer.append({"seq": 2, "containers": {"nats-0": {"container_id": "n0"}}})
            writer.close()
            collector = soak.Collector(root, manifest, types.SimpleNamespace(), soak.SampleWriter(root),
                                       soak.EventJournal(root / "events.jsonl"))
            self.assertEqual(collector.previous_containers.get("api"), api)
            changed = {"api": dict(api, started_at="2026-11-09T09:00:00Z"),
                       "nats-0": {"container_id": "n0"}}
            self.assertEqual(soak.detect_unexpected_restarts(collector.previous_containers, changed), ["api"])


class HourlyTornSegmentTests(unittest.TestCase):
    def test_new_hour_repairs_the_previous_segments_crash_tail(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            segment = root / "samples" / "19700101T00.jsonl"
            segment.parent.mkdir()
            segment.write_text('{"seq":1}\n{"seq":2,"partial":')
            writer = soak.SampleWriter(root, clock=lambda: 3600)
            writer.append({"seq": 2})
            writer.close()
            try:
                rows = soak.read_jsonl_segments(root)
            except ValueError as exc:
                self.fail("hourly rollover stranded a torn sample: %s" % exc)
            self.assertEqual(rows, [{"seq": 1}, {"seq": 2}])


class ActiveFaultIsolationTests(unittest.TestCase):
    def make_runner(self, root, cleanup_rc):
        import types
        runner = soak.SoakRunner.__new__(soak.SoakRunner)
        runner.root = root
        runner.repo = root
        runner.manifest = {"engine": "podman"}
        runner.config = {"outage_s": 0, "bootstrap_timeout_s": 1200}
        runner.journal = soak.EventJournal(root / "events.jsonl")
        runner.collector = types.SimpleNamespace(calls=[], set_outage=lambda value: runner.collector.calls.append(value))
        runner.start_active_lane = lambda compose: None
        runner.wait_active_ingest = lambda compose: None
        runner.wait_nats = lambda *args: None
        runner.wait_bootstrap = lambda *args: None
        runner.run_api_smoke = lambda *args: None
        runner.save_artifacts = lambda *args: None
        runner.compose_logs = lambda *args: "pipeline logs"
        result = types.SimpleNamespace(returncode=cleanup_rc, args=["podman", "compose", "down", "-v"],
                                       stdout="", stderr="injected cleanup failure")
        fake_compose = types.SimpleNamespace(compose=lambda *args, **kwargs: result)
        return runner, fake_compose

    def test_fresh_lane_fault_does_not_mark_retained_samples_as_outages(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / ".env").write_text("WASL_API_PORT=1\nWASL_JS_KEY=ktest\n")
            runner, compose = self.make_runner(root, 0)
            with patch("soak.Compose", return_value=compose):
                runner.active_fault("active-test")
            self.assertEqual(runner.collector.calls, [])

    def test_fresh_lane_cleanup_failure_fails_without_verified_event(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / ".env").write_text("WASL_API_PORT=1\nWASL_JS_KEY=ktest\n")
            runner, compose = self.make_runner(root, 1)
            with patch("soak.Compose", return_value=compose):
                with self.assertRaises(soak.CommandError):
                    runner.active_fault("active-test")
            events = soak.read_events(root / "events.jsonl")
            self.assertNotIn("fresh_lane_verified", [row["event"] for row in events])


if __name__ == "__main__":
    unittest.main()
