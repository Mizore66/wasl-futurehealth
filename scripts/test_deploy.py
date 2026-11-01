#!/usr/bin/env python3
"""Focused deployment shell tests; a fake docker CLI avoids requiring Docker."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


class DeploymentShellTests(unittest.TestCase):
    def run_deploy(self, tmp, key=None, volume=None):
        tmp = Path(tmp)
        log = tmp / "compose.log"
        docker = tmp / "docker"
        docker.write_text("""#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-} ${2:-}" = "volume ls" ]; then
  volume=""
  for arg in "$@"; do
    case "$arg" in label=com.docker.compose.volume=nats-*) volume="${arg##*=}";; esac
  done
  [ "${FAKE_VOLUME:-}" != "$volume" ] || printf '%s\\n' "project_$volume"
  exit 0
fi
if [ "${1:-}" = compose ]; then
  case "${WASL_JS_KEY:-}" in [A-Za-z]*) ;; *) exit 9;; esac
  printf '%s\\n' "$*" >> "$FAKE_LOG"
  exit 0
fi
exit 2
""")
        docker.chmod(0o755)
        env = os.environ.copy()
        env.update(PATH=str(tmp) + os.pathsep + env["PATH"], FAKE_LOG=str(log))
        env.pop("WASL_JS_KEY", None)
        env.pop("COMPOSE_PROJECT_NAME", None)
        if key is not None:
            env["WASL_JS_KEY"] = key
        if volume is not None:
            env["FAKE_VOLUME"] = volume
        else:
            env.pop("FAKE_VOLUME", None)
        result = subprocess.run([str(ROOT / "scripts/deploy_up.sh"), "-d"], cwd=ROOT, env=env,
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        return result, log

    def test_fresh_deployment_generates_key(self):
        with tempfile.TemporaryDirectory() as tmp:
            result, log = self.run_deploy(tmp)
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertIn("not written to repo or image", result.stdout)
            self.assertIn("compose up --build -d", log.read_text())

    def test_retained_volume_refuses_new_key(self):
        with tempfile.TemporaryDirectory() as tmp:
            result, log = self.run_deploy(tmp, volume="nats-1")
            self.assertEqual(result.returncode, 1, result.stdout)
            self.assertIn("Refusing to generate a different key", result.stdout)
            self.assertIn("original WASL_JS_KEY", result.stdout)
            self.assertIn("docker compose down -v", result.stdout)
            self.assertFalse(log.exists(), "compose must not start with an incompatible generated key")

    def test_retained_volume_accepts_supplied_key(self):
        with tempfile.TemporaryDirectory() as tmp:
            result, log = self.run_deploy(tmp, key="k-test-original", volume="nats-2")
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertTrue(log.exists())

    def test_demo_rejects_half_configured_external_nats(self):
        cases = [
            ({"WASL_NATS_URL": "nats://nats-0:4222"}, "WASL_NATS_URL requires WASL_NATS_EXTERNAL=1"),
            ({"WASL_NATS_EXTERNAL": "1"}, "WASL_NATS_EXTERNAL=1 requires WASL_NATS_URL"),
        ]
        for extra, message in cases:
            with self.subTest(extra=extra):
                env = os.environ.copy()
                env.pop("WASL_NATS_URL", None)
                env.pop("WASL_NATS_EXTERNAL", None)
                env.update(extra)
                result = subprocess.run([str(ROOT / "scripts/demo.sh"), "check"], cwd=ROOT, env=env,
                                        text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
                self.assertEqual(result.returncode, 2, result.stdout)
                self.assertIn(message, result.stdout)

    def test_demo_refuses_binary_older_than_sources(self):
        # ROOT is derived from the script location, so run a copy inside a minimal tree
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "scripts").mkdir()
            (root / "pipeline").mkdir()
            (root / "bin").mkdir()
            script = root / "scripts/demo.sh"
            script.write_text((ROOT / "scripts/demo.sh").read_text())
            script.chmod(0o755)
            (root / "pipeline/main.go").write_text("package main\n")
            exe = root / "bin/wsi7"
            exe.write_text("#!/bin/sh\nexit 0\n")
            exe.chmod(0o755)
            old = (root / "pipeline/main.go").stat().st_mtime - 1000
            os.utime(exe, (old, old))
            env = {k: v for k, v in os.environ.items() if k not in ("WASL_BIN", "WASL_NATS_URL", "WASL_NATS_EXTERNAL")}
            r = subprocess.run([str(script), "report"], cwd=root, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(r.returncode, 2, r.stdout)
            self.assertIn("stale binary", r.stdout)
            # a prebuilt WASL_BIN is trusted as given: the guard must not fire (the command then fails later, for other reasons)
            env["WASL_BIN"] = str(root / "bin")
            r = subprocess.run([str(script), "report"], cwd=root, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertNotIn("stale binary", r.stdout)


if __name__ == "__main__":
    unittest.main()
