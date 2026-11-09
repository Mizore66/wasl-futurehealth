#!/usr/bin/env python3
"""Resumable, stdlib-only Wasl availability and resource soak.

The retained-read lane never restarts the one-shot pipeline. Fresh active-fault
lanes use separate Compose projects and are explicitly labelled fresh reruns.
"""
from __future__ import annotations

import argparse
import contextlib
import copy
import decimal
import fcntl
import hashlib
import json
import math
import os
from pathlib import Path
import re
import secrets
import signal
import socket
import statistics
import subprocess
import shutil
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request
from datetime import datetime, timezone


SCHEMA = "wasl-soak/1"
PROJECT_RE = re.compile(r"^waslsoak-[a-z0-9][a-z0-9-]{0,40}$")


class ResumeError(RuntimeError):
    pass


def utc_text(epoch=None):
    value = time.time() if epoch is None else epoch
    return datetime.fromtimestamp(value, timezone.utc).isoformat().replace("+00:00", "Z")


def fsync_directory(path):
    fd = os.open(str(path), os.O_RDONLY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def atomic_write_json(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix="." + path.name + ".", dir=str(path.parent))
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(value, handle, sort_keys=True, separators=(",", ":"), allow_nan=False)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
        fsync_directory(path.parent)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


class SampleWriter:
    """One fsynced JSON object per line, rotated into UTC-hour segments."""

    def __init__(self, run_dir, clock=time.time):
        self.root = Path(run_dir) / "samples"
        self.root.mkdir(parents=True, exist_ok=True)
        self.clock = clock
        self.handle = None
        self.segment = None
        self.lock = threading.Lock()

    def _open(self, epoch):
        segment = datetime.fromtimestamp(epoch, timezone.utc).strftime("%Y%m%dT%H")
        if segment == self.segment:
            return
        if self.handle is not None:
            self.handle.close()
        self.segment = segment
        self.handle = (self.root / (segment + ".jsonl")).open("a", encoding="utf-8", buffering=1)

    def append(self, record):
        encoded = json.dumps(record, sort_keys=True, separators=(",", ":"), allow_nan=False) + "\n"
        with self.lock:
            self._open(self.clock())
            self.handle.write(encoded)
            self.handle.flush()
            os.fsync(self.handle.fileno())

    def close(self):
        with self.lock:
            if self.handle is not None:
                self.handle.close()
                self.handle = None


class EventJournal:
    def __init__(self, path, clock=time.time):
        self.path = Path(path)
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.clock = clock
        self.lock = threading.Lock()
        self.seq = 0
        if self.path.exists():
            with self.path.open(encoding="utf-8") as handle:
                for line in handle:
                    if line.strip():
                        self.seq = max(self.seq, int(json.loads(line)["seq"]))

    def append(self, event, **fields):
        with self.lock:
            self.seq += 1
            row = {"schema": SCHEMA + "/event", "seq": self.seq, "event": event,
                   "epoch": self.clock(), "ts": utc_text(self.clock()), **fields}
            encoded = json.dumps(row, sort_keys=True, separators=(",", ":"), allow_nan=False) + "\n"
            fd = os.open(str(self.path), os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
            try:
                view = memoryview(encoded.encode("utf-8"))
                while view:
                    written = os.write(fd, view)
                    view = view[written:]
                os.fsync(fd)
            finally:
                os.close(fd)
            return row


class ControllerLock:
    def __init__(self, path):
        self.path = Path(path)
        self.handle = None

    def acquire(self):
        self.path.parent.mkdir(parents=True, exist_ok=True)
        handle = self.path.open("a+", encoding="utf-8")
        try:
            fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            handle.close()
            raise ResumeError("another live controller holds " + str(self.path)) from exc
        handle.seek(0)
        handle.truncate()
        handle.write(json.dumps({"pid": os.getpid(), "ts": utc_text()}) + "\n")
        handle.flush()
        os.fsync(handle.fileno())
        self.handle = handle

    def close(self):
        if self.handle is not None:
            fcntl.flock(self.handle.fileno(), fcntl.LOCK_UN)
            self.handle.close()
            self.handle = None


def resume_state(path, manifest):
    with Path(path).open(encoding="utf-8") as handle:
        state = json.load(handle)
    if state.get("manifest_sha256") != manifest.get("manifest_sha256"):
        raise ResumeError("state does not match the frozen manifest")
    for action_id, action in state.get("actions", {}).items():
        if action.get("status") == "started":
            raise ResumeError("action %s has unknown outcome; inspect and reconcile it before resume" % action_id)
    return state


def next_pending_action(state):
    pending = [(action["due_epoch"], action_id, action)
               for action_id, action in state.get("actions", {}).items()
               if action.get("status") == "pending"]
    if not pending:
        return None
    _, action_id, action = min(pending)
    return {action_id: action}


def cleanup_args(project, disposable):
    if not PROJECT_RE.fullmatch(project):
        raise ValueError("refusing cleanup for an unscoped Compose project")
    return ["down", "-v"] if disposable else ["down"]


def percentile(values, fraction):
    ordered = sorted(values)
    if not ordered:
        return None
    rank = max(1, int(math.ceil(fraction * len(ordered))))
    return ordered[rank - 1]


def window_medians(samples, window_s, extractor):
    groups = {}
    for sample in samples:
        value = extractor(sample)
        if value is None:
            continue
        key = int(sample["elapsed_s"] // window_s)
        groups.setdefault(key, []).append(float(value))
    return [{"window": key, "elapsed_s": key * window_s, "median": statistics.median(values)}
            for key, values in sorted(groups.items())]


def growth_gate(windows, minimum, limit, relative=False):
    result = {"evaluated": len(windows) >= minimum, "windows": windows, "limit": limit}
    if len(windows) < minimum:
        result.update(passed=False, reason="insufficient post-warmup windows")
        return result
    first, last = windows[0]["median"], windows[-1]["median"]
    observed = (last - first) / first if relative and first else last - first
    result.update(passed=observed <= limit, observed=observed,
                  first=first, last=last)
    return result


def restart_summary(events, budgets, required_kinds=()):
    starts = {}
    recovered = {}
    failures = {}
    for event in events:
        action_id = event.get("action_id")
        if not action_id:
            continue
        if event.get("event") == "restart_started":
            starts.setdefault(action_id, []).append(event)
        elif event.get("event") == "restart_recovered":
            recovered.setdefault(action_id, []).append(event)
        elif event.get("event") == "restart_failed":
            failures.setdefault(action_id, []).append(event)
    rows = []
    valid = set(starts) == set(recovered) and not failures
    valid = valid and all(len(starts[key]) == 1 and len(recovered.get(key, [])) == 1 for key in starts)
    started_events = [event for entries in starts.values() for event in entries]
    for kind in required_kinds:
        if not any(event.get("kind") == kind for event in started_events):
            valid = False
    for action_id, entries in sorted(starts.items()):
        start = entries[0]
        recovery = recovered.get(action_id, [None])[0]
        duration = recovery.get("recovery_s") if recovery else None
        budget = budgets.get(start.get("kind"), budgets.get("default", 0))
        within = duration is not None and duration <= budget
        valid = valid and within
        rows.append({"action_id": action_id, "kind": start.get("kind"), "recovery_s": duration,
                     "budget_s": budget, "within_budget": within})
    return rows, valid


def build_report(manifest, config, samples, events, final_status):
    gates_config = config["gates"]
    warmup = config.get("warmup_s", 0)
    post_warmup = [row for row in samples if row.get("elapsed_s", 0) >= warmup and row.get("phase") != "bootstrap"]
    steady = [row for row in post_warmup if not row.get("outage_id")]
    planned = [row for row in post_warmup if row.get("outage_id")]
    latency_values = {}
    api_errors = []
    steady_requests = 0
    planned_requests = 0
    for sample in steady:
        for route, probe in sample.get("api", {}).get("routes", {}).items():
            steady_requests += 1
            latency_values.setdefault(route, []).append(probe["latency_ms"])
            if probe.get("timeout") or int(probe.get("status") or 0) == 0 or int(probe.get("status") or 0) >= 500:
                api_errors.append({"seq": sample.get("seq"), "route": route, "probe": probe})
    for sample in planned:
        planned_requests += len(sample.get("api", {}).get("routes", {}))
    latency = {}
    for route, values in sorted(latency_values.items()):
        latency[route] = {"count": len(values), "p50": percentile(values, .50),
                          "p90": percentile(values, .90), "p95": percentile(values, .95),
                          "p99": percentile(values, .99), "max": max(values)}
    route_budgets = gates_config.get("route_p95_ms", {})
    latency_pass = bool(latency) and all(
        summary["p95"] <= route_budgets.get(route, gates_config["max_api_p95_ms"]) and
        summary["p99"] <= gates_config["max_api_p99_ms"]
        for route, summary in latency.items())
    goroutines = window_medians(post_warmup, gates_config["leak_window_s"],
                                lambda row: row.get("api", {}).get("runtime", {}).get("goroutines"))
    rss = window_medians(post_warmup, gates_config["leak_window_s"],
                         lambda row: row.get("containers", {}).get("api", {}).get("rss_bytes"))
    stores = window_medians(post_warmup, gates_config["leak_window_s"],
                            lambda row: max([n.get("store_bytes", 0) for n in row.get("nats", {}).values()] or [0]))
    restarts, recovery_pass = restart_summary(events, gates_config["max_recovery_s"],
                                               gates_config.get("required_restart_kinds", ()))
    gaps = []
    for before, after in zip(samples, samples[1:]):
        gap = after["epoch"] - before["epoch"]
        if gap > gates_config["max_sample_gap_s"]:
            gaps.append({"before_seq": before.get("seq"), "after_seq": after.get("seq"), "gap_s": gap})
    unexpected_restarts = [{"seq": row.get("seq"), "services": row["unexpected_restarts"]}
                           for row in samples if row.get("unexpected_restarts")]
    comparisons = [event for event in events if event.get("event") == "comparison"]
    comparisons_pass = all(event.get("matched") is True for event in comparisons)
    nats_capacity = gates_config["nats_store_capacity_bytes"]
    capacity_violations = [{"seq": row.get("seq"), "node": node, "store_bytes": stats.get("store_bytes")}
                           for row in samples for node, stats in row.get("nats", {}).items()
                           if stats.get("store_bytes", 0) > nats_capacity * gates_config["max_nats_store_fraction"]]
    ack_violations = [{"seq": row.get("seq"), "node": node, "ack_pending": stats.get("ack_pending")}
                      for row in steady for node, stats in row.get("nats", {}).items()
                      if stats.get("ack_pending", 0) != 0]
    nats_health_violations = [{"seq": row.get("seq"), "node": node,
                                "healthy": stats.get("healthy"), "routes": stats.get("routes")}
                               for row in steady for node, stats in row.get("nats", {}).items()
                               if stats.get("healthy") is not True or stats.get("routes") != 2]
    budget_violations = [{"seq": row.get("seq"), "violations": row["budget_violations"]}
                         for row in samples if row.get("budget_violations")]
    observed_duration = max([row.get("elapsed_s", 0) for row in samples] or [0])
    gates = {
        "completed_duration": {"passed": final_status == "completed" and observed_duration >= config["duration_s"],
                                "observed_s": observed_duration, "required_s": config["duration_s"]},
        "api_latency_p95": {"passed": latency_pass, "routes": latency,
                             "default_p95_ms": gates_config["max_api_p95_ms"],
                             "p99_ms": gates_config["max_api_p99_ms"]},
        "api_errors": {"passed": not api_errors, "unexpected": api_errors},
        "api_goroutine_growth": growth_gate(goroutines, gates_config["min_leak_windows"],
                                              gates_config["max_goroutine_growth"]),
        "api_rss_growth": growth_gate(rss, gates_config["min_leak_windows"],
                                       gates_config["max_rss_growth_fraction"], relative=True),
        "nats_store_growth": growth_gate(stores, gates_config["min_leak_windows"],
                                          gates_config["max_nats_store_growth_bytes"]),
        "restart_recovery": {"passed": recovery_pass, "restarts": restarts},
        "collector_heartbeat": {"passed": not gaps, "gaps": gaps},
        "unexpected_restarts": {"passed": not unexpected_restarts, "observed": unexpected_restarts},
        "export_comparison": {"passed": comparisons_pass, "checks": len(comparisons)},
        "nats_store_capacity": {"passed": not capacity_violations, "violations": capacity_violations},
        "nats_ack_pending": {"passed": not ack_violations, "violations": ack_violations},
        "nats_health": {"passed": not nats_health_violations, "violations": nats_health_violations},
        "disk_budget": {"passed": not budget_violations, "violations": budget_violations},
    }
    return {
        "schema": SCHEMA + "/report", "run_id": manifest.get("run_id"), "project": manifest.get("project"),
        "status": final_status, "evaluation_scope": config.get("evaluation_scope", "24-hour retained-read"),
        "generated_at": utc_text(), "overall_pass": final_status == "completed" and all(g["passed"] for g in gates.values()),
        "summary": {"samples": len(samples), "steady_state_samples": len(steady),
                    "steady_state_api_requests": steady_requests, "planned_outage_api_requests": planned_requests,
                    "api_latency_ms": latency, "restarts": restarts},
        "gates": gates,
    }


class ActionBook:
    def __init__(self, run_dir, journal):
        self.path = Path(run_dir) / "state.json"
        with self.path.open(encoding="utf-8") as handle:
            self.state = json.load(handle)
        self.journal = journal

    def save(self):
        atomic_write_json(self.path, self.state)

    def execute(self, action_id, function):
        action = self.state["actions"][action_id]
        if action.get("status") != "pending":
            raise ResumeError("action %s is not pending" % action_id)
        action["status"] = "started"
        action["started_epoch"] = time.time()
        self.save()
        self.journal.append("action_started", action_id=action_id, kind=action.get("kind"))
        try:
            function()
        except Exception as exc:
            self.journal.append("action_error", action_id=action_id, kind=action.get("kind"),
                                  error=type(exc).__name__ + ": " + str(exc))
            raise
        action["status"] = "completed"
        action["finished_epoch"] = time.time()
        self.save()
        self.journal.append("action_completed", action_id=action_id, kind=action.get("kind"))


def probe_json(url, timeout):
    started = time.monotonic()
    status = 0
    raw = b""
    error = None
    timed_out = False
    try:
        request = urllib.request.Request(url, headers={"Accept": "application/json"}, method="GET")
        with urllib.request.urlopen(request, timeout=timeout) as response:
            status = response.status
            raw = response.read()
    except urllib.error.HTTPError as exc:
        with contextlib.closing(exc):
            status = exc.code
            raw = exc.read()
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        reason = getattr(exc, "reason", exc)
        timed_out = isinstance(reason, TimeoutError) or "timed out" in str(reason).lower()
        error = type(reason).__name__ + ": " + str(reason)
    body = None
    if raw:
        try:
            body = json.loads(raw)
        except (ValueError, UnicodeDecodeError) as exc:
            error = "invalid JSON: " + str(exc)
    return {"status": status, "latency_ms": round((time.monotonic() - started) * 1000, 3),
            "bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest(), "timeout": timed_out,
            "error": error, "body": body}


def parse_nats_snapshot(varz, jsz, routez, stacks):
    streams = {}
    pending = ack_pending = redelivered = 0
    replicas_current = True
    for account in jsz.get("account_details", []):
        for stream in account.get("stream_detail", []):
            state = stream.get("state", {})
            consumers = stream.get("consumer_detail", [])
            cluster = stream.get("cluster", {})
            replicas = cluster.get("replicas", [])
            current = all(replica.get("current") is True for replica in replicas)
            replicas_current = replicas_current and current
            pending += sum(int(consumer.get("num_pending", 0)) for consumer in consumers)
            ack_pending += sum(int(consumer.get("num_ack_pending", 0)) for consumer in consumers)
            redelivered += sum(int(consumer.get("num_redelivered", 0)) for consumer in consumers)
            streams[stream.get("name", "unknown")] = {
                "messages": state.get("messages", 0), "bytes": state.get("bytes", 0),
                "first_seq": state.get("first_seq"), "last_seq": state.get("last_seq"),
                "leader": cluster.get("leader"), "replicas_current": current,
                "replicas": [{"name": replica.get("name"), "current": replica.get("current"),
                              "lag": replica.get("lag")} for replica in replicas],
                "consumers": [{"name": consumer.get("name"), "pending": consumer.get("num_pending", 0),
                               "ack_pending": consumer.get("num_ack_pending", 0),
                               "redelivered": consumer.get("num_redelivered", 0)} for consumer in consumers],
            }
    match = re.search(r"goroutine profile: total\s+(\d+)", stacks)
    goroutines = int(match.group(1)) if match else len(re.findall(r"(?m)^goroutine\s+\d+\s+\[", stacks))
    names = sorted({route.get("remote_name") or route.get("name") for route in routez.get("routes", [])} - {None})
    return {
        "rss_bytes": varz.get("mem"), "connections": varz.get("connections"),
        "slow_consumers": varz.get("slow_consumers"), "in_msgs": varz.get("in_msgs"),
        "out_msgs": varz.get("out_msgs"), "in_bytes": varz.get("in_bytes"),
        "out_bytes": varz.get("out_bytes"), "uptime": varz.get("uptime"),
        "jetstream_memory_bytes": jsz.get("memory", 0), "store_bytes": jsz.get("storage", jsz.get("store", 0)),
        "queue_pending": pending, "ack_pending": ack_pending, "redelivered": redelivered,
        "routes": len(names), "raw_route_connections": routez.get("num_routes", len(routez.get("routes", []))),
        "route_names": names,
        "goroutines": goroutines if match or stacks else None,
        "replicas_current": replicas_current and bool(streams), "streams": streams,
    }


def read_jsonl_segments(root):
    rows = []
    for path in sorted((Path(root) / "samples").glob("*.jsonl")):
        with path.open(encoding="utf-8") as handle:
            for line_number, line in enumerate(handle, 1):
                if not line.strip():
                    continue
                try:
                    rows.append(json.loads(line))
                except ValueError as exc:
                    raise ValueError("invalid sample %s:%d: %s" % (path, line_number, exc)) from exc
    return rows


def read_events(path):
    if not Path(path).exists():
        return []
    with Path(path).open(encoding="utf-8") as handle:
        return [json.loads(line) for line in handle if line.strip()]


def atomic_write_text(path, text):
    path = Path(path)
    fd, temporary = tempfile.mkstemp(prefix="." + path.name + ".", dir=str(path.parent))
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            handle.write(text)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
        fsync_directory(path.parent)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def write_report(run_dir, final_status):
    root = Path(run_dir)
    with (root / "manifest.json").open(encoding="utf-8") as handle:
        manifest = json.load(handle)
    report = build_report(manifest, manifest["config"], read_jsonl_segments(root),
                          read_events(root / "events.jsonl"), final_status)
    atomic_write_json(root / "report.json", report)
    atomic_write_json(root / "summary.json", report["summary"])
    completion = "COMPLETE" if final_status == "completed" else "INCOMPLETE"
    lines = ["Wasl soak report: " + completion,
             "Run: %s (%s)" % (report["run_id"], report["evaluation_scope"]),
             "Overall gate result: " + ("PASS" if report["overall_pass"] else "FAIL"),
             "Samples: %d; steady-state API requests: %d; planned-outage API requests: %d" % (
                 report["summary"]["samples"], report["summary"]["steady_state_api_requests"],
                 report["summary"]["planned_outage_api_requests"]), ""]
    for name, gate in report["gates"].items():
        lines.append("%-28s %s" % (name, "PASS" if gate["passed"] else "FAIL"))
    atomic_write_text(root / "report.txt", "\n".join(lines) + "\n")
    return report


def disk_budget_violations(disk, nats, config):
    budget = config.get("disk", {})
    violations = []
    if disk.get("host_free_bytes", math.inf) < budget.get("min_host_free_bytes", 0):
        violations.append("host free bytes below pre-registered budget")
    if disk.get("artifact_bytes", 0) > budget.get("max_artifact_bytes", math.inf):
        violations.append("artifact bytes above pre-registered budget")
    capacity = config["gates"]["nats_store_capacity_bytes"]
    limit = capacity * config["gates"]["max_nats_store_fraction"]
    for node, stats in nats.items():
        if stats.get("store_bytes", 0) > limit:
            violations.append("%s JetStream store above pre-registered budget" % node)
    return violations


def measured_restart(journal, collector, action_id, kind, operation, recovery):
    started = time.monotonic()
    journal.append("restart_started", action_id=action_id, kind=kind)
    collector.set_outage(action_id)
    try:
        operation()
        recovery()
    except Exception as exc:
        journal.append("restart_failed", action_id=action_id, kind=kind,
                       recovery_s=round(time.monotonic() - started, 3),
                       error=type(exc).__name__ + ": " + str(exc))
        raise
    finally:
        collector.set_outage(None)
    journal.append("restart_recovered", action_id=action_id, kind=kind,
                   recovery_s=round(time.monotonic() - started, 3))


def export_canonical(value):
    import api_smoke
    def normalise(item):
        if isinstance(item, decimal.Decimal):
            return float(item)
        if isinstance(item, list):
            return [normalise(child) for child in item]
        if isinstance(item, dict):
            return {key: normalise(child) for key, child in item.items()}
        return item
    return api_smoke.canonical_json(normalise(value))


def verify_export(raw):
    document = json.loads(raw, parse_float=decimal.Decimal)
    digest = document.get("content_sha256")
    unsigned = {key: value for key, value in document.items() if key != "content_sha256"}
    actual = hashlib.sha256(export_canonical(unsigned)).hexdigest()
    return {"comparison_sha256": document.get("comparison_sha256"), "content_sha256": digest,
            "content_hash_valid": isinstance(digest, str) and digest == actual,
            "loaded_at": document.get("meta", {}).get("loaded_at")}


def compare_exports(baseline, current):
    mismatches = {}
    for fixture, raw in baseline.items():
        if fixture not in current:
            mismatches[fixture] = "missing"
            continue
        try:
            before = verify_export(raw)
            after = verify_export(current[fixture])
            if not after["content_hash_valid"]:
                mismatches[fixture] = "invalid content hash"
            elif before["comparison_sha256"] != after["comparison_sha256"]:
                mismatches[fixture] = "comparison_sha256 changed"
        except (ValueError, TypeError, AttributeError) as exc:
            mismatches[fixture] = "invalid export: " + str(exc)
    for fixture in current.keys() - baseline.keys():
        mismatches[fixture] = "unexpected fixture"
    return {"matched": not mismatches, "mismatches": mismatches, "fixtures": sorted(baseline)}


def pipeline_invariants(report, logs):
    violations = []
    zero_paths = [
        ("consent_timing", "released_without_granted_consent"),
        ("consent_timing", "released_after_opt_out_on_file"),
        ("outreach_no_review", "wrong_patient_released"),
    ] + [("crash_recovery", name) for name in (
        "duplicate_outputs", "lost_outputs", "unexpected_outputs", "final_state_mismatch_loops",
        "ingest_duplicates", "ingest_lost", "prep_duplicates", "prep_lost", "prep_content_mismatch",
        "logical_duplicate_decisions")]
    live_outreach = "outreach_live" if "outreach_live" in report else "outreach"
    zero_paths.insert(2, (live_outreach, "wrong_patient_released"))
    for section, name in zero_paths:
        value = report.get(section, {}).get(name)
        if value != 0:
            violations.append("%s.%s=%r (required zero)" % (section, name, value))
    for programme, results in report.get("closure", {}).items():
        for lane, metrics in results.items():
            if lane.startswith("wasl_") and isinstance(metrics, dict) and metrics.get("false_closures") != 0:
                violations.append("closure.%s.%s.false_closures=%r (required zero)" % (
                    programme, lane, metrics.get("false_closures")))
    if "timeout waiting for quiescence" in logs.lower():
        violations.append("quiescence timeout recorded")
    return violations


def tamper_invariants(evidence):
    violations = []
    attacks = evidence.get("attacks", [])
    if len(attacks) < 2:
        return ["tamper evidence has no attack cases"]
    for attack in attacks:
        if attack.get("attack") != "none (baseline)" and attack.get("detected") is not True:
            violations.append("undetected tamper attack: %s" % attack.get("attack", "unknown"))
    return violations


def validate_config(config):
    required_intervals = ("health_interval_s", "api_interval_s", "nats_interval_s",
                          "container_interval_s", "disk_interval_s", "request_timeout_s")
    for name in required_intervals:
        if not isinstance(config.get(name), (int, float)) or config[name] <= 0:
            raise ValueError(name + " must be positive")
    if config.get("evaluation_scope", "").startswith("24-hour") and config.get("duration_s") != 86400:
        raise ValueError("a 24-hour soak must pre-register duration_s=86400")
    if config.get("duration_s", 0) <= 0 or config.get("warmup_s", 0) < 0:
        raise ValueError("invalid duration or warmup")
    required_gates = ("max_api_p95_ms", "max_api_p99_ms", "max_rss_growth_fraction",
                      "max_goroutine_growth", "max_nats_store_growth_bytes", "max_recovery_s",
                      "max_sample_gap_s", "max_nats_store_fraction", "nats_store_capacity_bytes",
                      "leak_window_s", "min_leak_windows")
    for name in required_gates:
        if name not in config.get("gates", {}):
            raise ValueError("missing pre-registered gate " + name)
    schedule = config.get("schedule", [])
    ids = [action.get("id") for action in schedule]
    if len(ids) != len(set(ids)) or not all(ids):
        raise ValueError("schedule action IDs must be unique and non-empty")
    times = [action.get("at_s") for action in schedule]
    if any(not isinstance(value, (int, float)) or value < 0 or value > config["duration_s"] for value in times):
        raise ValueError("schedule action times are outside the soak")
    if times != sorted(times):
        raise ValueError("schedule must be ordered by absolute offset")
    if config.get("disk", {}).get("min_host_free_bytes") is None or config.get("disk", {}).get("max_artifact_bytes") is None:
        raise ValueError("disk budgets must be pre-registered")


def initial_state(manifest_sha256, config, start_epoch):
    return {"schema": SCHEMA + "/state", "manifest_sha256": manifest_sha256,
            "status": "running", "start_epoch": start_epoch,
            "actions": {action["id"]: {"status": "pending", "kind": action["kind"],
                                       "due_epoch": start_epoch + action["at_s"]}
                        for action in config.get("schedule", [])}}


def detect_unexpected_restarts(previous, current):
    unexpected = []
    for service, before in previous.items():
        after = current.get(service)
        if not after or service == "pipeline":
            continue
        if (before.get("started_at") and after.get("started_at") != before.get("started_at")) or \
                after.get("restart_count", 0) > before.get("restart_count", 0) or \
                (before.get("status") == "running" and after.get("status") in ("exited", "dead")):
            unexpected.append(service)
    return sorted(unexpected)


def parse_byte_size(value):
    if isinstance(value, (int, float)):
        return int(value)
    if not isinstance(value, str):
        return None
    match = re.match(r"^\s*([0-9.]+)\s*([kmgt]?i?b)?", value, re.I)
    if not match:
        return None
    units = {"b": 1, "kb": 1000, "mb": 1000**2, "gb": 1000**3, "tb": 1000**4,
             "kib": 1024, "mib": 1024**2, "gib": 1024**3, "tib": 1024**4}
    return int(float(match.group(1)) * units.get((match.group(2) or "b").lower(), 1))


def normalize_container_stat(stat):
    cpu = stat.get("CPU", stat.get("CPUPerc", stat.get("cpu_percent")))
    if isinstance(cpu, str):
        cpu = cpu.rstrip("%")
    usage = stat.get("MemUsage", stat.get("mem_usage", ""))
    used, _, limit = usage.partition("/") if isinstance(usage, str) else ("", "", "")
    net_in, _, net_out = str(stat.get("net_io", "")).partition("/")
    block_in, _, block_out = str(stat.get("block_io", "")).partition("/")
    pids = stat.get("PIDs", stat.get("Pids", stat.get("pids")))
    return {
        "cpu_percent": float(cpu) if cpu is not None else None,
        "rss_bytes": parse_byte_size(stat.get("Mem", stat.get("mem", used))) if not used else parse_byte_size(used),
        "memory_limit_bytes": parse_byte_size(stat.get("MemLimit", stat.get("mem_limit", limit))),
        "pids": int(pids) if isinstance(pids, str) and pids.isdigit() else pids,
        "network_input_bytes": parse_byte_size(stat.get("NetInput", stat.get("net_input", net_in))),
        "network_output_bytes": parse_byte_size(stat.get("NetOutput", stat.get("net_output", net_out))),
        "block_input_bytes": parse_byte_size(stat.get("BlockInput", stat.get("block_input", block_in))),
        "block_output_bytes": parse_byte_size(stat.get("BlockOutput", stat.get("block_output", block_out))),
    }


def directory_bytes(path):
    total = 0
    for root, _, files in os.walk(path):
        for name in files:
            try:
                total += os.lstat(os.path.join(root, name)).st_size
            except OSError:
                pass
    return total


class CommandError(RuntimeError):
    def __init__(self, result):
        self.result = result
        diagnostics = ((result.stderr or "")[-4000:] + "\n" + (result.stdout or "")[-4000:]).strip()
        super().__init__("command exited %d: %s%s" % (result.returncode, result.args[0],
                         "; output: " + diagnostics if diagnostics else ""))
        self.stdout = result.stdout
        self.stderr = result.stderr


class Compose:
    def __init__(self, engine, repo, project, env_file):
        if not PROJECT_RE.fullmatch(project):
            raise ValueError("invalid isolated project")
        self.engine = engine
        self.repo = str(Path(repo).resolve())
        self.project = project
        self.env_file = str(env_file)

    def run(self, args, timeout=120, check=True):
        result = subprocess.run(args, cwd=self.repo, text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=timeout)
        if check and result.returncode:
            raise CommandError(result)
        return result

    def compose(self, *args, timeout=120, check=True):
        return self.run([self.engine, "compose", "--env-file", self.env_file, "-p", self.project,
                         *args], timeout=timeout, check=check)

    def exec_result(self, service, args, timeout=20):
        return self.compose("exec", "-T", service, *args, timeout=timeout, check=False)

    def exec(self, service, args, timeout=20):
        result = self.exec_result(service, args, timeout)
        if result.returncode:
            raise CommandError(result)
        return result.stdout

    def service_ids(self):
        result = self.compose("ps", "-aq", timeout=20)
        ids = [line.strip() for line in result.stdout.splitlines() if line.strip()]
        services = {}
        for service in ("nats-0", "nats-1", "nats-2", "pipeline", "api"):
            one = self.compose("ps", "-aq", service, timeout=20).stdout.strip().splitlines()
            if one:
                services[service] = one[0].strip()
        return services

    def inspect_state(self, container_id):
        result = self.run([self.engine, "inspect", "--format", "{{json .State}}|{{.RestartCount}}",
                           container_id], timeout=60)
        state_text, restart_count = result.stdout.strip().rsplit("|", 1)
        state = json.loads(state_text)
        health = state.get("Health") or state.get("Healthcheck") or {}
        return {"container_id": container_id, "status": str(state.get("Status", "unknown")).lower(),
                "health": health.get("Status"), "health_failing_streak": health.get("FailingStreak"),
                "exit_code": state.get("ExitCode"), "oom": state.get("OOMKilled"),
                "pid": state.get("Pid"), "started_at": state.get("StartedAt"),
                "finished_at": state.get("FinishedAt"), "restart_count": int(restart_count or 0)}

    def stats(self, container_ids):
        if not container_ids:
            return []
        result = self.run([self.engine, "stats", "--no-stream", "--format", "json", *container_ids], timeout=30)
        text = result.stdout.strip()
        if not text:
            return []
        try:
            parsed = json.loads(text)
            return parsed if isinstance(parsed, list) else [parsed]
        except ValueError:
            return [json.loads(line) for line in text.splitlines() if line.strip()]

    def stop(self, service):
        self.compose("stop", service, timeout=90)

    def start(self, service):
        self.compose("start", service, timeout=90)

    def down(self):
        self.compose("down", timeout=120)

    def up_read_stack(self):
        self.compose("up", "-d", "--no-deps", "--no-build",
                     "nats-0", "nats-1", "nats-2", "api", timeout=120)


class Collector:
    def __init__(self, run_dir, manifest, compose, writer, journal):
        self.root = Path(run_dir)
        self.manifest = manifest
        self.config = manifest["config"]
        self.compose = compose
        self.writer = writer
        self.journal = journal
        self.seq = 0
        self.started_monotonic = time.monotonic()
        self.previous_containers = {}
        self.fixture_id = None
        self.phase = "bootstrap"
        self.outage_id = None
        self.outage_lock = threading.Lock()
        self.last_budget_violations = []
        self.budget_violations = []
        self.stop_event = threading.Event()

    def set_phase(self, phase):
        self.phase = phase

    def set_outage(self, outage_id):
        with self.outage_lock:
            self.outage_id = outage_id

    def _due(self, name):
        if self.config[name] <= 0:
            return True
        return self.seq == 0 or self.seq % max(1, int(self.config[name] / self.config["health_interval_s"])) == 0

    def _api(self, record):
        base = self.manifest["api_url"]
        routes = record["api"]["routes"]
        if self._due("health_interval_s"):
            routes["health"] = probe_json(base + "/healthz", self.config["request_timeout_s"])
        if not self._due("api_interval_s"):
            return
        paths = {"dashboard": "/api/dashboard", "cases": "/api/cases?limit=25", "fixtures": "/api/fixtures"}
        for route, path in paths.items():
            routes[route] = probe_json(base + path, self.config["request_timeout_s"])
        fixture_body = routes["fixtures"].get("body") or {}
        fixtures = fixture_body.get("fixtures", {}) if isinstance(fixture_body, dict) else {}
        if fixtures:
            first = fixtures.get("review_hold") or next(iter(fixtures.values()))
            self.fixture_id = first.get("id")
        if self.fixture_id:
            import urllib.parse
            case = "/api/cases/" + urllib.parse.quote(self.fixture_id, safe="/")
            for route, suffix in (("case_detail", ""), ("case_audit", "/audit"), ("case_export", "/export")):
                routes[route] = probe_json(base + case + suffix, self.config["request_timeout_s"])
        result = self.compose.exec_result("api", ["python3", "-c",
            "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8090/debug/runtime', timeout=3).read().decode())"],
            timeout=10)
        if result.returncode:
            raise CommandError(result)
        record["api"]["runtime"] = json.loads(result.stdout)

    def _nats(self, record):
        if not self._due("nats_interval_s"):
            return
        for node in ("nats-0", "nats-1", "nats-2"):
            stats = {"healthy": False}
            record["nats"][node] = stats
            try:
                health = self.compose.exec_result(node, ["wget", "-qO-", "http://127.0.0.1:8222/healthz?js-enabled-only=true"])
                stats["healthy"] = health.returncode == 0
                stats["health_body"] = health.stdout.strip()
                varz = json.loads(self.compose.exec(node, ["wget", "-qO-", "http://127.0.0.1:8222/varz"]))
                jsz = json.loads(self.compose.exec(node, ["wget", "-qO-",
                    "http://127.0.0.1:8222/jsz?streams=true&consumers=true"]))
                routez = json.loads(self.compose.exec(node, ["wget", "-qO-", "http://127.0.0.1:8222/routez"]))
                stacks = self.compose.exec(node, ["wget", "-qO-", "http://127.0.0.1:8222/stacksz"])
                stats.update(parse_nats_snapshot(varz, jsz, routez, stacks))
            except (CommandError, ValueError, subprocess.TimeoutExpired) as exc:
                record["errors"].append(node + ": " + type(exc).__name__)

    def _containers(self, record):
        if not self._due("container_interval_s"):
            return
        ids = self.compose.service_ids()
        running = []
        id_services = {}
        for service, container_id in ids.items():
            state = self.compose.inspect_state(container_id)
            record["containers"][service] = state
            id_services[container_id] = service
            if state["status"] == "running":
                running.append(container_id)
        for stat in self.compose.stats(running):
            container_id = stat.get("ContainerID") or stat.get("ID") or stat.get("Id") or stat.get("id")
            service = id_services.get(container_id) or next((name for cid, name in id_services.items()
                                                             if container_id and cid.startswith(container_id)), None)
            if not service:
                continue
            record["containers"][service].update(normalize_container_stat(stat))
        if not record.get("outage_id"):
            record["unexpected_restarts"] = detect_unexpected_restarts(self.previous_containers, record["containers"])
        self.previous_containers = {name: dict(state) for name, state in record["containers"].items()}

    def sample_once(self):
        epoch = time.time()
        record = {"schema": SCHEMA + "/sample", "run_id": self.manifest["run_id"],
                  "project": self.manifest["project"], "seq": self.seq + 1, "epoch": epoch, "ts": utc_text(epoch),
                  "monotonic_s": time.monotonic(), "elapsed_s": max(0, epoch - self.manifest["start_epoch"]),
                  "phase": self.phase,
                  "sample_duration_s": 0, "errors": [], "api": {"routes": {}}, "nats": {}, "containers": {}}
        with self.outage_lock:
            if self.outage_id:
                record["outage_id"] = self.outage_id
        started = time.monotonic()
        for name, function in (("api", self._api), ("nats", self._nats), ("containers", self._containers)):
            try:
                function(record)
            except Exception as exc:
                record["errors"].append(name + ": " + type(exc).__name__ + ": " + str(exc))
        if self._due("disk_interval_s"):
            usage = shutil.disk_usage(self.root)
            stats = os.statvfs(self.root)
            record["disk"] = {"host_free_bytes": usage.free, "host_total_bytes": usage.total,
                              "host_free_inodes": stats.f_ffree, "artifact_bytes": directory_bytes(self.root)}
        record["sample_duration_s"] = round(time.monotonic() - started, 3)
        violations = disk_budget_violations(record.get("disk", {}), record["nats"], self.config)
        record["budget_violations"] = violations
        if violations:
            self.budget_violations = violations
        if violations != self.last_budget_violations:
            self.journal.append("disk_budget", violations=violations)
            self.last_budget_violations = violations
        self.seq += 1
        self.writer.append(record)
        return record

    def run(self):
        interval = self.config["health_interval_s"]
        deadline = time.monotonic()
        while not self.stop_event.is_set():
            self.sample_once()
            deadline += interval
            delay = max(0, deadline - time.monotonic())
            self.stop_event.wait(delay)

    def stop(self):
        self.stop_event.set()


ARTIFACT_FILES = ("out_log.ndjson", "anchors.json", "audit.json", "report7.json", "tamper.json",
                  "orchestrate.json", "api_reviews.ndjson")


def file_sha256(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def manifest_digest(manifest):
    value = {key: item for key, item in manifest.items() if key != "manifest_sha256"}
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def free_loopback_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def fetch_raw(url, timeout):
    request = urllib.request.Request(url, headers={"Accept": "application/json"}, method="GET")
    with urllib.request.urlopen(request, timeout=timeout) as response:
        if response.status != 200:
            raise RuntimeError("GET %s returned %d" % (url, response.status))
        return response.read()


def create_run(config, output_root, engine, repo):
    validate_config(config)
    repo = Path(repo).resolve()
    if shutil.which(engine) is None:
        raise RuntimeError("container engine not found: " + engine)
    status = subprocess.run(["git", "status", "--porcelain"], cwd=repo, text=True,
                            stdout=subprocess.PIPE, check=True).stdout
    if config.get("require_clean_source") and status:
        raise RuntimeError("24-hour source must be committed and clean before T0")
    Path(output_root).mkdir(parents=True, exist_ok=True)
    if shutil.disk_usage(output_root).free < config["disk"]["min_host_free_bytes"]:
        raise RuntimeError("host free disk is below the pre-registered T0 budget")
    run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ-") + secrets.token_hex(3)
    project = "waslsoak-" + run_id.lower()
    root = Path(output_root).resolve() / run_id
    root.mkdir(parents=True, exist_ok=False)
    (root / "logs").mkdir()
    env_file = root / ".env"
    port = free_loopback_port()
    image_suffix = run_id.lower().replace("z-", "z")
    env_text = ("WASL_JS_KEY=k%s\nWASL_API_PORT=%d\nWASL_IMAGE=wasl-soak-%s:local\n"
                "WASL_NATS_IMAGE=wasl-nats-soak-%s:local\n") % (
                    secrets.token_hex(32), port, image_suffix, image_suffix)
    fd = os.open(str(env_file), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as handle:
        handle.write(env_text)
        handle.flush()
        os.fsync(handle.fileno())
    compose = Compose(engine, repo, project, env_file)
    existing = compose.run([engine, "ps", "-aq", "--filter",
                            "label=com.docker.compose.project=" + project], timeout=20).stdout.strip()
    if existing:
        raise RuntimeError("refusing to reuse a non-empty Compose project")
    start_epoch = time.time()
    head = subprocess.run(["git", "rev-parse", "HEAD"], cwd=repo, text=True,
                          stdout=subprocess.PIPE, check=True).stdout.strip()
    engine_version = compose.run([engine, "version", "--format", "{{json .Server.Version}}"], timeout=20).stdout.strip()
    compose_version = compose.compose("version", timeout=20).stdout.strip()
    try:
        total_memory = os.sysconf("SC_PAGE_SIZE") * os.sysconf("SC_PHYS_PAGES")
    except (ValueError, OSError, AttributeError):
        total_memory = None
    manifest = {
        "schema": SCHEMA + "/manifest", "run_id": run_id, "project": project,
        "created_at": utc_text(start_epoch), "start_epoch": start_epoch,
        "api_url": "http://127.0.0.1:%d" % port, "engine": engine,
        "engine_version": engine_version, "compose_version": compose_version,
        "source": {"git_sha": head, "dirty_at_smoke_creation": bool(status)},
        "config": copy.deepcopy(config), "config_sha256": hashlib.sha256(
            json.dumps(config, sort_keys=True, separators=(",", ":")).encode()).hexdigest(),
        "source_hashes": {name: file_sha256(repo / name) for name in
                          ("docker-compose.yml", "Dockerfile", "deploy/nats/nats.conf", "deploy/nats/entrypoint.sh")},
        "host": {"platform": sys.platform, "cpu_count": os.cpu_count(), "memory_bytes": total_memory,
                 "disk_free_bytes": shutil.disk_usage(root).free},
        "seed": 2026, "policy": "v7strict", "source_mode": "fhir", "reader": "cached",
        "stream_replicas": 3, "lane_semantics": "retained-read; pipeline is never rerun; active faults are fresh projects",
    }
    manifest["manifest_sha256"] = manifest_digest(manifest)
    atomic_write_json(root / "manifest.json", manifest)
    atomic_write_json(root / "state.json", initial_state(manifest["manifest_sha256"], config, start_epoch))
    return root


class SoakRunner:
    def __init__(self, run_dir, resume=False):
        self.root = Path(run_dir).resolve()
        with (self.root / "manifest.json").open(encoding="utf-8") as handle:
            self.manifest = json.load(handle)
        self.config = self.manifest["config"]
        self.repo = Path(__file__).resolve().parent.parent
        self.compose = Compose(self.manifest["engine"], self.repo, self.manifest["project"], self.root / ".env")
        self.journal = EventJournal(self.root / "events.jsonl")
        self.writer = SampleWriter(self.root)
        self.collector = Collector(self.root, self.manifest, self.compose, self.writer, self.journal)
        self.lock = ControllerLock(self.root / "controller.lock")
        if resume:
            self.state = resume_state(self.root / "state.json", self.manifest)
        else:
            with (self.root / "state.json").open(encoding="utf-8") as handle:
                self.state = json.load(handle)
        self.book = ActionBook(self.root, self.journal)
        if self.state["actions"].get("bootstrap", {}).get("status") == "completed":
            self.collector.set_phase("retained-read")
        self.current_action_id = None
        self.stop_signal = False

    def log(self, message):
        print("%s %s" % (utc_text(), message), flush=True)

    def compose_logs(self, compose, service):
        return compose.compose("logs", "--no-color", "--timestamps", service, timeout=120).stdout

    def record_image_ids(self):
        values = dict(line.split("=", 1) for line in (self.root / ".env").read_text().splitlines()
                      if line and not line.startswith("#"))
        images = []
        for variable in ("WASL_IMAGE", "WASL_NATS_IMAGE"):
            tag = values[variable]
            result = self.compose.run([self.compose.engine, "image", "inspect", "--format", "{{.Id}}", tag], timeout=30)
            images.append({"image": tag, "id": result.stdout.strip()})
        atomic_write_json(self.root / "images.json", {"images": images})

    def wait_bootstrap(self, compose=None, api_url=None, timeout=None):
        compose = compose or self.compose
        api_url = api_url or self.manifest["api_url"]
        deadline = time.monotonic() + (timeout or self.config["bootstrap_timeout_s"])
        last = "not started"
        while time.monotonic() < deadline:
            try:
                ids = compose.service_ids()
                pipeline = compose.inspect_state(ids["pipeline"])
                api = compose.inspect_state(ids["api"])
                last = "pipeline=%s/%s api=%s/%s" % (pipeline["status"], pipeline["exit_code"],
                                                        api["status"], api["health"])
                if pipeline["status"] == "exited" and pipeline["exit_code"] != 0:
                    raise RuntimeError("pipeline exited " + str(pipeline["exit_code"]))
                if pipeline["status"] == "exited" and pipeline["exit_code"] == 0 and api["status"] == "running":
                    probe = probe_json(api_url + "/healthz", self.config["request_timeout_s"])
                    body = probe.get("body") or {}
                    dependencies = body.get("dependencies", {})
                    if (probe["status"] == 200 and body.get("status") == "ok" and
                            dependencies.get("materialised_run") == "loaded" and
                            dependencies.get("review_ledger") == "ok"):
                        return
            except (KeyError, CommandError, ValueError, subprocess.TimeoutExpired) as exc:
                last = type(exc).__name__ + ": " + str(exc)
            time.sleep(2)
        raise RuntimeError("bootstrap readiness timeout: " + last)

    def run_api_smoke(self, api_url, destination):
        result = subprocess.run([sys.executable, str(self.repo / "scripts/api_smoke.py"),
                                 "--base-url", api_url, "--timeout", str(self.config["request_timeout_s"])],
                                cwd=self.repo, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                timeout=300)
        atomic_write_text(destination, result.stdout)
        if result.returncode:
            raise RuntimeError("mutating API smoke failed; see " + str(destination))

    def freeze_exports(self, destination, api_url=None):
        api_url = api_url or self.manifest["api_url"]
        destination = Path(destination)
        destination.mkdir(parents=True, exist_ok=True)
        import urllib.parse
        fixtures = json.loads(fetch_raw(api_url + "/api/fixtures", self.config["request_timeout_s"]))["fixtures"]
        exports = {}
        for fixture, detail in sorted(fixtures.items()):
            path = "/api/cases/" + urllib.parse.quote(detail["id"], safe="/") + "/export"
            raw = fetch_raw(api_url + path, self.config["request_timeout_s"])
            verified = verify_export(raw)
            if not verified["content_hash_valid"] or not verified["comparison_sha256"]:
                raise RuntimeError("invalid export hash for " + fixture)
            safe_name = re.sub(r"[^a-zA-Z0-9_.-]", "_", fixture) + ".json"
            (destination / safe_name).write_bytes(raw)
            exports[fixture] = raw
        return exports

    def current_artifact_hashes(self, compose=None):
        compose = compose or self.compose
        root = self.config["run_container_path"]
        code = ("import hashlib,json,pathlib; p=pathlib.Path(%r); "
                "print(json.dumps({n:hashlib.sha256((p/n).read_bytes()).hexdigest() for n in %r}))") % (
                    root, ARTIFACT_FILES)
        return json.loads(compose.exec("api", ["python3", "-c", code], timeout=60))

    def save_artifacts(self, label, compose=None, api_url=None):
        compose = compose or self.compose
        destination = self.root / label
        destination.mkdir(parents=True, exist_ok=True)
        ids = compose.service_ids()
        for name in ARTIFACT_FILES:
            source = "%s:%s/%s" % (ids["api"], self.config["run_container_path"], name)
            compose.run([compose.engine, "cp", source, str(destination / name)], timeout=120)
        try:
            logs = self.compose_logs(compose, "pipeline")
            atomic_write_text(destination / "pipeline.log", logs)
        except CommandError:
            baseline_log = self.root / "baseline" / "pipeline.log"
            if compose is not self.compose or not baseline_log.exists():
                raise
            logs = baseline_log.read_text()
            atomic_write_json(destination / "pipeline-log-source.json", {
                "source": "baseline/pipeline.log", "reason": "pipeline container removed by retained-read recreation"})
        with (destination / "report7.json").open(encoding="utf-8") as handle:
            report = json.load(handle)
        violations = pipeline_invariants(report, logs)
        with (destination / "tamper.json").open(encoding="utf-8") as handle:
            violations.extend(tamper_invariants(json.load(handle)))
        with (destination / "orchestrate.json").open(encoding="utf-8") as handle:
            orchestrate = json.load(handle)
        for process, counts in orchestrate.get("supervisor", {}).items():
            if counts.get("unexpected_exits_restarted", 0) != 0:
                violations.append("unplanned supervisor restart: " + process)
        atomic_write_json(destination / "artifact-hashes.json", {
            name: file_sha256(destination / name) for name in ARTIFACT_FILES})
        atomic_write_json(destination / "pipeline-invariants.json", {"passed": not violations, "violations": violations})
        if violations:
            raise RuntimeError("pipeline invariant failure: " + "; ".join(violations))
        return report

    def compare_baseline(self, action_id):
        current = self.freeze_exports(self.root / "comparisons" / action_id)
        baseline = {}
        for fixture in current:
            safe_name = re.sub(r"[^a-zA-Z0-9_.-]", "_", fixture) + ".json"
            baseline[fixture] = (self.root / "baseline" / "exports" / safe_name).read_bytes()
        comparison = compare_exports(baseline, current)
        hashes = self.current_artifact_hashes()
        with (self.root / "baseline" / "artifact-hashes.json").open(encoding="utf-8") as handle:
            expected = json.load(handle)
        artifact_mismatches = {name: {"before": expected.get(name), "after": hashes.get(name)}
                               for name in ARTIFACT_FILES if expected.get(name) != hashes.get(name)}
        health = probe_json(self.manifest["api_url"] + "/healthz", self.config["request_timeout_s"])
        ledger = (health.get("body") or {}).get("dependencies", {}).get("review_ledger")
        matched = comparison["matched"] and not artifact_mismatches and ledger == "ok"
        self.journal.append("comparison", action_id=action_id, matched=matched,
                            export_mismatches=comparison["mismatches"], artifact_mismatches=artifact_mismatches,
                            review_ledger=ledger)
        if not matched:
            raise RuntimeError("restart-stable comparison failed for " + action_id)

    def bootstrap(self):
        self.log("building isolated images for " + self.manifest["project"])
        self.compose.compose("build", timeout=1200)
        self.record_image_ids()
        self.journal.append("images_frozen", images_file="images.json")
        self.compose.compose("up", "-d", "--no-build", timeout=self.config["bootstrap_timeout_s"])
        self.wait_bootstrap()
        self.wait_nats()
        self.log("pipeline completed; running the single mutating baseline smoke")
        self.run_api_smoke(self.manifest["api_url"], self.root / "logs" / "baseline-api-smoke.log")
        self.save_artifacts("baseline")
        self.freeze_exports(self.root / "baseline" / "exports")
        self.collector.set_phase("retained-read")
        self.journal.append("baseline_frozen", pipeline_rerun=False)

    def wait_api(self):
        deadline = time.monotonic() + self.config["gates"]["max_recovery_s"]["api"]
        while time.monotonic() < deadline:
            probe = probe_json(self.manifest["api_url"] + "/healthz", self.config["request_timeout_s"])
            dependencies = (probe.get("body") or {}).get("dependencies", {})
            if (probe["status"] == 200 and dependencies.get("materialised_run") == "loaded" and
                    dependencies.get("nats") == "up" and dependencies.get("review_ledger") == "ok"):
                return
            time.sleep(1)
        raise RuntimeError("API did not recover within its budget")

    def nats_ready(self, compose=None):
        compose = compose or self.compose
        details = {}
        for node in ("nats-0", "nats-1", "nats-2"):
            health = compose.exec_result(node, ["wget", "-qO-", "http://127.0.0.1:8222/healthz?js-enabled-only=true"], timeout=10)
            if health.returncode:
                return False, {node: "unhealthy"}
            jsz = json.loads(compose.exec(node, ["wget", "-qO-",
                "http://127.0.0.1:8222/jsz?streams=true&consumers=true"], timeout=15))
            routez = json.loads(compose.exec(node, ["wget", "-qO-", "http://127.0.0.1:8222/routez"], timeout=15))
            snapshot = parse_nats_snapshot({}, jsz, routez, "")
            details[node] = {"routes": snapshot["routes"], "replicas_current": snapshot["replicas_current"],
                             "streams": sorted(snapshot["streams"])}
            if snapshot["routes"] != 2 or not snapshot["replicas_current"] or not {"FHIR", "PREP", "FACTS", "OUT"} <= set(snapshot["streams"]):
                return False, details
        return True, details

    def wait_nats(self, kind="nats_node", compose=None):
        budget = self.config["gates"]["max_recovery_s"].get(kind, 180)
        deadline = time.monotonic() + budget
        details = {}
        while time.monotonic() < deadline:
            try:
                ready, details = self.nats_ready(compose)
                if ready:
                    return
            except (CommandError, ValueError, subprocess.TimeoutExpired):
                pass
            time.sleep(2)
        raise RuntimeError("NATS did not recover routes/quorum/current replicas: " + json.dumps(details))

    def recover_read(self):
        self.wait_nats()
        self.wait_api()
        self.compare_baseline(self.current_action_id or "recovery")

    def api_restart(self, action_id, kill=False):
        def operation():
            if kill:
                self.compose.compose("kill", "-s", "KILL", "api", timeout=60)
                self.compose.compose("start", "api", timeout=60)
            else:
                self.compose.compose("restart", "api", timeout=120)
        measured_restart(self.journal, self.collector, action_id, "api", operation, self.recover_read)

    def nats_restart(self, action_id, node):
        if node not in ("nats-0", "nats-1", "nats-2"):
            raise ValueError("invalid NATS restart target")
        def operation():
            self.compose.stop(node)
            time.sleep(self.config["outage_s"])
            self.compose.start(node)
        measured_restart(self.journal, self.collector, action_id, "nats_node", operation, self.recover_read)

    def recreate_read_stack(self, action_id):
        def recovery():
            self.compose.up_read_stack()
            self.recover_read()
        measured_restart(self.journal, self.collector, action_id, "read_stack",
                         self.compose.down, recovery)

    def assert_quorum_loss(self):
        deadline = time.monotonic() + 30
        details = {}
        while True:
            ready, details = self.nats_ready()
            if not ready:
                break
            if time.monotonic() >= deadline:
                raise RuntimeError("quorum-loss negative test did not produce a JetStream dependency failure")
            time.sleep(1)
        health = probe_json(self.manifest["api_url"] + "/healthz", self.config["request_timeout_s"])
        if health["status"] != 200:
            raise RuntimeError("read API did not remain live during declared quorum-loss negative test")
        self.journal.append("expected_negative_outcome", action_id=self.current_action_id,
                            outcome="JetStream unavailable while materialized read API remains live", details=details)

    def quorum_loss(self, action_id):
        def operation():
            self.compose.stop("nats-1")
            self.compose.stop("nats-2")
            self.assert_quorum_loss()
            self.compose.start("nats-1")
            self.compose.start("nats-2")
        measured_restart(self.journal, self.collector, action_id, "quorum", operation, self.recover_read)

    def wait_nats_processes(self, compose):
        deadline = time.monotonic() + 300
        while time.monotonic() < deadline:
            healthy = True
            for node in ("nats-0", "nats-1", "nats-2"):
                result = compose.exec_result(node, ["wget", "-qO-",
                    "http://127.0.0.1:8222/healthz?js-enabled-only=true"], timeout=10)
                healthy = healthy and result.returncode == 0
            if healthy:
                return
            time.sleep(1)
        raise RuntimeError("fresh NATS processes did not become healthy")

    def start_active_lane(self, compose):
        compose.compose("up", "-d", "--no-build", "--no-deps", "nats-0", "nats-1", "nats-2", timeout=120)
        self.wait_nats_processes(compose)
        compose.compose("up", "-d", "--no-build", "--no-deps", "pipeline", timeout=60)

    def wait_active_ingest(self, compose):
        deadline = time.monotonic() + 300
        while time.monotonic() < deadline:
            try:
                ids = compose.service_ids()
                if compose.inspect_state(ids["pipeline"])["status"] == "running":
                    jsz = json.loads(compose.exec("nats-0", ["wget", "-qO-",
                        "http://127.0.0.1:8222/jsz?streams=true&consumers=true"], timeout=15))
                    parsed = parse_nats_snapshot({}, jsz, {}, "")
                    if parsed["streams"].get("FHIR", {}).get("messages", 0) >= 100:
                        return
            except (CommandError, KeyError, ValueError, subprocess.TimeoutExpired):
                pass
            time.sleep(2)
        raise RuntimeError("fresh lane never reached active FHIR ingestion")

    def active_fault(self, action_id):
        lane_root = self.root / ("active-" + action_id)
        lane_root.mkdir(parents=True, exist_ok=True)
        project = "waslsoak-a-" + secrets.token_hex(5)
        env_file = lane_root / ".env"
        source_env = (self.root / ".env").read_text().splitlines()
        values = dict(line.split("=", 1) for line in source_env if line and not line.startswith("#"))
        values["WASL_API_PORT"] = str(free_loopback_port())
        values["WASL_JS_KEY"] = "k" + secrets.token_hex(32)
        fd = os.open(str(env_file), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            handle.write("".join(key + "=" + value + "\n" for key, value in values.items()))
            handle.flush()
            os.fsync(handle.fileno())
        compose = Compose(self.manifest["engine"], self.repo, project, env_file)
        api_url = "http://127.0.0.1:" + values["WASL_API_PORT"]
        self.journal.append("fresh_lane_started", action_id=action_id, project=project,
                            semantics="fresh rerun, not resumed ingestion")
        try:
            def operation():
                self.start_active_lane(compose)
                self.wait_active_ingest(compose)
                compose.compose("kill", "-s", "KILL", "nats-2", timeout=60)
                time.sleep(self.config["outage_s"])
                compose.compose("start", "nats-2", timeout=60)
            def active_recovery():
                self.wait_nats("active_fresh_nats", compose)
                compose.compose("up", "-d", "--no-build", "--no-deps", "api", timeout=60)
                self.wait_bootstrap(compose, api_url, self.config["bootstrap_timeout_s"])
            measured_restart(self.journal, self.collector, action_id, "active_fresh_nats", operation,
                             active_recovery)
            self.run_api_smoke(api_url, lane_root / "api-smoke.log")
            self.save_artifacts(str(lane_root.relative_to(self.root)), compose, api_url)
            self.journal.append("fresh_lane_verified", action_id=action_id, project=project)
        finally:
            try:
                atomic_write_text(lane_root / "pipeline-cleanup.log", self.compose_logs(compose, "pipeline"))
            except Exception as exc:
                self.journal.append("fresh_lane_log_error", action_id=action_id, error=str(exc))
            compose.compose(*cleanup_args(project, disposable=True), timeout=120, check=False)

    def final_compare(self, action_id):
        self.compare_baseline(action_id)
        self.save_artifacts("final")
        for service in ("api", "nats-0", "nats-1", "nats-2"):
            atomic_write_text(self.root / "logs" / ("final-" + service + ".log"), self.compose_logs(self.compose, service))

    def dispatch(self, action_id, action):
        self.current_action_id = action_id
        kind = action["kind"]
        self.log("action %s (%s) started" % (action_id, kind))
        if kind == "bootstrap":
            self.bootstrap()
        elif kind == "api_restart":
            self.api_restart(action_id)
        elif kind == "api_kill":
            self.api_restart(action_id, kill=True)
        elif kind == "nats_restart":
            self.nats_restart(action_id, action["node"])
        elif kind == "recreate_read_stack":
            self.recreate_read_stack(action_id)
        elif kind == "quorum_loss":
            self.quorum_loss(action_id)
        elif kind == "active_fault":
            self.active_fault(action_id)
        elif kind == "final_compare":
            self.final_compare(action_id)
        else:
            raise ValueError("unknown action kind " + kind)
        self.log("action %s completed" % action_id)

    def wait_until(self, due_epoch):
        while True:
            if self.collector.budget_violations:
                raise RuntimeError("disk budget violation: " + "; ".join(self.collector.budget_violations))
            if self.stop_signal:
                raise KeyboardInterrupt()
            remaining = due_epoch - time.time()
            if remaining <= 0:
                return
            # Absolute wall deadlines survive resume; monotonic waits avoid chained-sleep drift.
            target = time.monotonic() + remaining
            time.sleep(max(0, min(1, target - time.monotonic())))

    def run(self):
        self.lock.acquire()
        final_status = "failed"
        thread = threading.Thread(target=self.collector.run, name="soak-sampler", daemon=True)
        thread.start()
        self.journal.append("controller_started", pid=os.getpid(), resume=any(
            action["status"] == "completed" for action in self.state["actions"].values()))
        try:
            while True:
                pending = [(action["due_epoch"], action_id) for action_id, action in self.state["actions"].items()
                           if action["status"] == "pending"]
                if not pending:
                    break
                due, action_id = min(pending)
                self.wait_until(due)
                schedule_action = next(action for action in self.config["schedule"] if action["id"] == action_id)
                self.book.execute(action_id, lambda: self.dispatch(action_id, schedule_action))
            self.wait_until(self.manifest["start_epoch"] + self.config["duration_s"])
            final_status = "completed"
            self.state["status"] = "completed"
            self.state["finished_epoch"] = time.time()
            self.book.state = self.state
            self.book.save()
            self.journal.append("controller_completed", duration_s=time.time() - self.manifest["start_epoch"])
        except KeyboardInterrupt:
            final_status = "interrupted"
            self.journal.append("controller_interrupted")
        except Exception as exc:
            final_status = "failed"
            self.journal.append("controller_failed", error=type(exc).__name__ + ": " + str(exc))
            self.log("controller failed: " + str(exc))
        finally:
            self.collector.stop()
            thread.join()
            try:
                self.collector.sample_once()
            except Exception as exc:
                self.journal.append("final_sample_error", error=str(exc))
            self.writer.close()
            if final_status == "completed":
                try:
                    self.journal.append("cleanup_started", disposable=self.config["disposable"])
                    self.compose.compose(*cleanup_args(self.manifest["project"], self.config["disposable"]), timeout=180)
                    self.journal.append("cleanup_completed")
                except Exception as exc:
                    final_status = "failed"
                    self.journal.append("cleanup_failed", error=str(exc))
            report = write_report(self.root, final_status)
            self.lock.close()
            self.log("report %s overall_pass=%s path=%s" % (final_status, report["overall_pass"], self.root / "report.json"))
        return 0 if final_status == "completed" and report["overall_pass"] else 2


def smoke_config(config):
    config = copy.deepcopy(config)
    config.update({"evaluation_scope": "short synchronous smoke; not 24-hour evidence", "duration_s": 180,
                   "warmup_s": 120, "health_interval_s": 2, "api_interval_s": 5, "nats_interval_s": 10,
                   "container_interval_s": 10, "disk_interval_s": 30, "request_timeout_s": 5,
                   "outage_s": 1, "cleanup": "down-v", "disposable": True, "require_clean_source": False})
    config["gates"].update({"leak_window_s": 30, "min_leak_windows": 1, "max_sample_gap_s": 180,
                            "required_restart_kinds": ["api", "nats_node"]})
    config["schedule"] = [
        {"id": "bootstrap", "at_s": 0, "kind": "bootstrap"},
        {"id": "api-graceful-smoke", "at_s": 130, "kind": "api_restart"},
        {"id": "nats2-stop-smoke", "at_s": 150, "kind": "nats_restart", "node": "nats-2"},
        {"id": "api-kill-smoke", "at_s": 165, "kind": "api_kill"},
        {"id": "final-compare-smoke", "at_s": 175, "kind": "final_compare"},
    ]
    validate_config(config)
    return config


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("run", "smoke"):
        command = sub.add_parser(name)
        command.add_argument("--output-root", default="pipeline/results/soak/runs")
        command.add_argument("--engine", default="podman" if shutil.which("podman") else "docker")
        if name == "run":
            command.add_argument("--config", default="scripts/soak.24h.json")
            command.add_argument("--resume", metavar="RUN_DIR")
    report_parser = sub.add_parser("report")
    report_parser.add_argument("--run-dir", required=True)
    report_parser.add_argument("--status", choices=("running", "failed", "interrupted"), default="running")
    args = parser.parse_args(argv)
    repo = Path(__file__).resolve().parent.parent
    if args.command == "report":
        report = write_report(Path(args.run_dir).resolve(), args.status)
        print(json.dumps({"overall_pass": report["overall_pass"], "report": str(Path(args.run_dir).resolve() / "report.json")}))
        return 0
    if args.command == "run" and args.resume:
        root = Path(args.resume).resolve()
    else:
        config_path = repo / (args.config if args.command == "run" else "scripts/soak.24h.json")
        with config_path.open(encoding="utf-8") as handle:
            config = json.load(handle)
        if args.command == "smoke":
            config = smoke_config(config)
        root = create_run(config, (repo / args.output_root).resolve(), args.engine, repo)
        print("SOAK_RUN_DIR=" + str(root), flush=True)
    runner = SoakRunner(root, resume=bool(args.command == "run" and args.resume))
    def stop(_signum, _frame):
        runner.stop_signal = True
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    return runner.run()


if __name__ == "__main__":
    raise SystemExit(main())
