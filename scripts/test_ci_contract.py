#!/usr/bin/env python3
"""Executable contract checks for the required-check gate and GitHub Actions CI."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / ".github" / "workflows" / "ci.yml"
REQUIRED_CHECK = ROOT / "scripts" / "ci_required.py"


class RequiredCheckTests(unittest.TestCase):
    def run_gate(self, results, *required):
        env = os.environ.copy()
        env["NEEDS_JSON"] = json.dumps(results)
        return subprocess.run(
            [sys.executable, str(REQUIRED_CHECK), *(required or ("go-test",))],
            cwd=ROOT,
            env=env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )

    def test_all_required_checks_successful_is_green(self):
        proc = self.run_gate({"go-test": {"result": "success"}})
        self.assertEqual(proc.returncode, 0, proc.stdout)

    def test_failed_check_is_not_green(self):
        proc = self.run_gate({"go-test": {"result": "failure"}})
        self.assertEqual(proc.returncode, 1, proc.stdout)
        self.assertIn("go-test=failure", proc.stdout)

    def test_skipped_check_is_not_green(self):
        proc = self.run_gate({"go-test": {"result": "skipped"}})
        self.assertEqual(proc.returncode, 1, proc.stdout)
        self.assertIn("go-test=skipped", proc.stdout)

    def test_cancelled_check_is_not_green(self):
        proc = self.run_gate({"go-test": {"result": "cancelled"}})
        self.assertEqual(proc.returncode, 1, proc.stdout)

    def test_missing_check_is_not_green(self):
        proc = self.run_gate({}, "go-test", "python-test")
        self.assertEqual(proc.returncode, 1, proc.stdout)
        self.assertIn("python-test=missing", proc.stdout)

    def test_malformed_results_are_not_green(self):
        env = os.environ.copy()
        env["NEEDS_JSON"] = "not-json"
        proc = subprocess.run(
            [sys.executable, str(REQUIRED_CHECK), "go-test"],
            cwd=ROOT,
            env=env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        self.assertEqual(proc.returncode, 2, proc.stdout)


class WorkflowTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.doc = yaml.safe_load(WORKFLOW.read_text())
        cls.jobs = cls.doc["jobs"]

    def runs(self, job):
        return "\n".join(step.get("run", "") for step in self.jobs[job]["steps"])

    def steps_using(self, prefix):
        found = []
        for job in self.jobs.values():
            for step in job["steps"]:
                if str(step.get("uses", "")).startswith(prefix + "@"):
                    found.append(step)
        return found

    def test_every_pull_request_runs_all_required_lanes(self):
        trigger = self.doc.get("on", self.doc.get(True))
        self.assertIn("pull_request", trigger)
        self.assertNotIn("pull_request_target", trigger)
        expected = {
            "actionlint",
            "go-test",
            "go-race",
            "python-test",
            "integration",
            "booking-simulation",
            "frontend",
            "playwright",
            "go-vulnerabilities",
            "python-vulnerabilities",
            "npm-vulnerabilities",
            "filesystem-vulnerabilities",
            "trufflehog",
            "dependency-review",
            "container",
        }
        self.assertEqual(set(self.jobs["required"]["needs"]), expected)
        for name in expected:
            self.assertIn("timeout-minutes", self.jobs[name], name)

    def test_required_gate_runs_after_failures_and_rejects_skips(self):
        job = self.jobs["required"]
        self.assertEqual(job["if"], "always()")
        self.assertEqual(job["name"], "CI required")
        self.assertEqual(job["steps"][-1]["env"]["NEEDS_JSON"], "${{ toJson(needs) }}")
        self.assertIn("scripts/ci_required.py", job["steps"][-1]["run"])

    def test_workflow_is_least_privilege_and_fail_closed(self):
        self.assertEqual(self.doc["permissions"], {"contents": "read"})
        forbidden = []

        def walk(value):
            if isinstance(value, dict):
                if value.get("continue-on-error") is True:
                    forbidden.append(value)
                for child in value.values():
                    walk(child)
            elif isinstance(value, list):
                for child in value:
                    walk(child)

        walk(self.doc)
        self.assertEqual(forbidden, [])

    def test_every_action_is_pinned_to_a_full_commit(self):
        uses = []

        def walk(value):
            if isinstance(value, dict):
                if "uses" in value:
                    uses.append(value["uses"])
                for child in value.values():
                    walk(child)
            elif isinstance(value, list):
                for child in value:
                    walk(child)

        walk(self.doc)
        self.assertTrue(uses)
        for ref in uses:
            self.assertRegex(ref, r"^[^/@]+/[^/@]+@[0-9a-f]{40}$", ref)

    def test_go_tests_race_and_real_nats_are_required(self):
        self.assertIn("go mod verify", self.runs("go-test"))
        self.assertIn("go test -count=1 -timeout=20m .", self.runs("go-test"))
        self.assertIn("go test -race -count=1 -timeout=60m .", self.runs("go-race"))
        self.assertEqual(self.jobs["go-race"]["timeout-minutes"], 80)
        for lane in ("go-test", "go-race"):
            self.assertIn("github.com/nats-io/nats-server/v2@v2.12.1", self.runs(lane))
            setup = self.steps_using("actions/setup-go")
            self.assertTrue(setup)
            for step in setup:
                self.assertEqual(step["with"]["go-version-file"], "pipeline/go.mod")
                self.assertEqual(step["with"]["cache-dependency-path"], "pipeline/go.sum")

    def test_all_python_lanes_and_booking_solver_are_required(self):
        commands = self.runs("python-test")
        for directory in ("scripts", "scripts/fresh2_eval", "encoder"):
            self.assertIn(f"python3 -m unittest discover -v -s {directory} -p 'test_*.py'", commands)
        for name in ("engine", "levers", "lookahead", "lookahead_eval", "suggest_default"):
            self.assertIn(f"python3 booking/tests/test_{name}.py", commands)
        self.assertIn("go build -o ../bin/wsi7 .", commands)
        self.assertIn("python3 scripts/check_deploy.py", commands)
        for step in self.steps_using("actions/setup-python"):
            self.assertEqual(step["with"]["python-version"], "3.12.16")
            self.assertEqual(step["with"]["cache"], "pip")
            self.assertEqual(step["with"]["cache-dependency-path"], "requirements-ci.txt")

    def test_integration_and_pool_simulation_are_explicit_required_lanes(self):
        integration = self.runs("integration")
        self.assertIn("scripts/demo.sh all", integration)
        self.assertIn("go test -count=1 -timeout=20m .", integration)
        self.assertIn("github.com/nats-io/nats-server/v2@v2.12.1", integration)
        simulation = self.runs("booking-simulation")
        self.assertIn("python3 booking/sim/pool_test.py", simulation)
        self.assertIn("git diff --exit-code", simulation)

    def test_frontend_has_separate_static_unit_and_build_gates(self):
        commands = self.runs("frontend")
        for command in ("pnpm gen", "pnpm lint", "pnpm typecheck", "pnpm test", "pnpm build"):
            self.assertIn(command, commands)
        self.assertIn("git diff --exit-code", commands)
        for step in self.steps_using("actions/setup-node"):
            self.assertEqual(step["with"]["node-version-file"], "web/.nvmrc")
            self.assertEqual(step["with"]["cache"], "pnpm")
            self.assertEqual(step["with"]["cache-dependency-path"], "web/pnpm-lock.yaml")

    def test_playwright_installs_browser_and_preserves_diagnostics(self):
        commands = self.runs("playwright")
        self.assertIn("playwright install --with-deps chromium", commands)
        self.assertIn("pnpm exec playwright test\n", commands)
        self.assertNotIn("--project=", commands)
        uploads = self.steps_using("actions/upload-artifact")
        self.assertTrue(any(step.get("if") == "failure()" for step in uploads))

    def test_dependency_and_vulnerability_scanners_fail_on_findings(self):
        self.assertIn("golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...", self.runs("go-vulnerabilities"))
        self.assertIn("python3 -m pip_audit -r requirements-ci.txt", self.runs("python-vulnerabilities"))
        self.assertIn("pnpm audit --audit-level=high", self.runs("npm-vulnerabilities"))
        trivy = self.steps_using("aquasecurity/trivy-action")
        self.assertTrue(trivy)
        for step in trivy:
            self.assertEqual(step["with"]["exit-code"], "1")
            self.assertEqual(step["with"]["severity"], "HIGH,CRITICAL")
        review = self.steps_using("actions/dependency-review-action")
        self.assertEqual(len(review), 1)
        self.assertEqual(review[0]["with"]["fail-on-severity"], "high")

    def test_trufflehog_scans_the_pull_request_range(self):
        step = self.steps_using("trufflesecurity/trufflehog")[0]
        self.assertEqual(step["with"]["base"], "${{ github.event.pull_request.base.sha }}")
        self.assertEqual(step["with"]["head"], "${{ github.event.pull_request.head.sha }}")
        self.assertIn("unverified", step["with"]["extra_args"])

    def test_container_is_built_from_checkout_and_smoke_tested(self):
        commands = self.runs("container")
        self.assertIn("docker build --no-cache", commands)
        self.assertIn("docker compose up --detach --no-build", commands)
        self.assertIn("docker compose ps --all --quiet pipeline", commands)
        self.assertIn("scripts/api_smoke.py --base-url", commands)
        self.assertIn("docker compose down --volumes --remove-orphans", commands)
        self.assertIn("materialised_run", commands)
        self.assertIn("10001", commands)
        logs = next(step for step in self.jobs["container"]["steps"] if step.get("name") == "Print service logs after a failure")
        self.assertIn('export WASL_JS_KEY="${WASL_JS_KEY:-', logs["run"])

    def test_container_key_is_not_exposed_to_later_third_party_actions(self):
        steps = self.jobs["container"]["steps"]
        generated = next(i for i, step in enumerate(steps) if step.get("name") == "Generate an ephemeral JetStream key")
        self.assertEqual([step["name"] for step in steps[generated + 1:] if "uses" in step], [])

    def test_workflow_linter_and_tool_versions_are_pinned(self):
        self.assertIn("github.com/rhysd/actionlint/cmd/actionlint@v1.7.10", self.runs("actionlint"))
        pins = (ROOT / "requirements-ci.txt").read_text().splitlines()
        expected = {
            "numpy==2.3.5",
            "scipy==1.16.3",
            "PyYAML==6.0.3",
            "scikit-learn==1.9.1",
            "lightgbm==4.7.0",
            "pip-audit==2.10.1",
        }
        self.assertTrue(expected.issubset(set(pins)))
        package = json.loads((ROOT / "web" / "package.json").read_text())
        pnpm = self.steps_using("pnpm/action-setup")
        self.assertTrue(pnpm)
        for step in pnpm:
            self.assertEqual("pnpm@" + step["with"]["version"], package["packageManager"])
        self.assertRegex((ROOT / "web" / ".nvmrc").read_text().strip(), r"^\d+\.\d+\.\d+$")


if __name__ == "__main__":
    unittest.main()
