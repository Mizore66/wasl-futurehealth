#!/usr/bin/env python3
"""Focused deployment shell tests; a fake docker CLI avoids requiring Docker."""
import os
from pathlib import Path
import subprocess
import sys
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

    def nats_env(self, tmp, key, data, ready=True, exit_code=0, extra_tools=None):
        """Env for deploy/nats/entrypoint.sh with fake docker-entrypoint.sh (nats-server) and wget (health probe)."""
        tmp = Path(tmp)
        bindir = tmp / "bin"
        bindir.mkdir(exist_ok=True)
        fake = bindir / "docker-entrypoint.sh"
        fake.write_text('#!/bin/sh\necho started >> "$FAKE_STARTED"\nsleep "${FAKE_SLEEP:-0}"\nexit "${FAKE_EXIT:-0}"\n')
        fake.chmod(0o755)
        wget = bindir / "wget"
        wget.write_text('#!/bin/sh\n[ "${FAKE_READY:-0}" = 1 ]\n')
        wget.chmod(0o755)
        for name, body in (extra_tools or {}).items():
            (bindir / name).write_text(body)
            (bindir / name).chmod(0o755)
        env = dict(os.environ, PATH=str(bindir) + os.pathsep + os.environ["PATH"], WASL_NATS_DATA=str(data),
                   FAKE_STARTED=str(tmp / "started"), FAKE_SLEEP="1", FAKE_READY="1" if ready else "0", FAKE_EXIT=str(exit_code))
        env.pop("WASL_JS_KEY", None)
        if key is not None:
            env["WASL_JS_KEY"] = key
        return env

    def run_nats_entrypoint(self, tmp, key, data, **kw):
        env = self.nats_env(tmp, key, data, **kw)
        r = subprocess.run(["sh", str(ROOT / "deploy/nats/entrypoint.sh"), "nats-server"], env=env, text=True,
                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        return r, Path(tmp) / "started"

    def test_nats_entrypoint_failed_first_start_leaves_no_marker(self):
        with tempfile.TemporaryDirectory() as tmp:
            data = Path(tmp) / "data"
            data.mkdir()
            # a leading-digit key makes nats-server fail before JetStream is up: its exit code must pass through
            r, started = self.run_nats_entrypoint(tmp, "1badkey", data, ready=False, exit_code=3)
            self.assertEqual(r.returncode, 3, r.stdout)
            self.assertFalse((data / ".wasl-key-check").exists(), "a failed first start must not record the key")
            self.assertNotIn("1badkey", r.stdout)
            started.unlink()
            r, started = self.run_nats_entrypoint(tmp, "kgood1111", data)
            self.assertEqual(r.returncode, 0, r.stdout)
            self.assertTrue(started.exists())
            self.assertTrue((data / ".wasl-key-check").exists(), "a healthy first start records the key")
            r, _ = self.run_nats_entrypoint(tmp, "kother2222", data)
            self.assertEqual(r.returncode, 1, r.stdout)

    def test_nats_entrypoint_marker_is_private_atomic_and_secret_free(self):
        with tempfile.TemporaryDirectory() as tmp:
            data = Path(tmp) / "data"
            r, _ = self.run_nats_entrypoint(tmp, "kgood1111", data)
            self.assertEqual(r.returncode, 0, r.stdout)
            marker = data / ".wasl-key-check"
            self.assertEqual(marker.stat().st_mode & 0o777, 0o600)
            self.assertEqual(len(marker.read_text().strip()), 64)
            self.assertNotIn("kgood1111", marker.read_text())
            self.assertEqual(sorted(p.name for p in data.iterdir()), [".wasl-key-check"], "no temp file may remain")
        src = (ROOT / "deploy/nats/entrypoint.sh").read_text()
        self.assertIn("umask 077", src)
        self.assertRegex(src, r'mv "\$tmp" "\$marker"')

    def test_nats_entrypoint_refuses_when_hash_is_empty(self):
        with tempfile.TemporaryDirectory() as tmp:
            data = Path(tmp) / "data"
            data.mkdir()
            tools = {"sha256sum": "#!/bin/sh\ncat >/dev/null\n"}
            r, started = self.run_nats_entrypoint(tmp, "kgood1111", data, extra_tools=tools)
            self.assertEqual(r.returncode, 1, r.stdout)
            self.assertFalse(started.exists())
            self.assertFalse((data / ".wasl-key-check").exists())
            (data / ".wasl-key-check").write_text("")
            r, started = self.run_nats_entrypoint(tmp, "kgood1111", data, extra_tools=tools)
            self.assertEqual(r.returncode, 1, r.stdout)
            self.assertFalse(started.exists())

    def test_nats_entrypoint_forwards_sigterm_and_keeps_exit_code(self):
        import signal
        import time
        with tempfile.TemporaryDirectory() as tmp:
            data = Path(tmp) / "data"
            tools = {"docker-entrypoint.sh": '#!/bin/sh\ntrap \'echo term >> "$FAKE_STARTED"; exit 7\' TERM\n'
                     'echo started >> "$FAKE_STARTED"\nsleep 30 &\nwait\n'}
            env = self.nats_env(tmp, "kgood1111", data, extra_tools=tools)
            p = subprocess.Popen(["sh", str(ROOT / "deploy/nats/entrypoint.sh"), "nats-server"], env=env,
                                 stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            started = Path(tmp) / "started"
            for _ in range(50):
                if started.exists() and (data / ".wasl-key-check").exists():
                    break
                time.sleep(0.1)
            p.send_signal(signal.SIGTERM)
            p.wait(timeout=10)
            p.stdout.close()
            self.assertEqual(p.returncode, 7)
            self.assertIn("term", started.read_text())

    def test_nats_image_wires_entrypoint(self):
        df = (ROOT / "deploy/nats/Dockerfile").read_text()
        self.assertRegex(df, r"COPY --chmod=0755 entrypoint\.sh /usr/local/bin/wasl-nats-entrypoint\.sh")
        self.assertIn('ENTRYPOINT ["/usr/local/bin/wasl-nats-entrypoint.sh"]', df)

    def test_nats_entrypoint_refuses_wrong_or_missing_key_on_retained_data(self):
        with tempfile.TemporaryDirectory() as tmp:
            data = Path(tmp) / "data"
            data.mkdir()
            r, started = self.run_nats_entrypoint(tmp, "kaaaa1111", data)
            self.assertEqual(r.returncode, 0, r.stdout)
            self.assertTrue(started.exists())
            (data / "jetstream").mkdir()
            (data / "jetstream" / "store").write_text("retained")
            started.unlink()
            for key in ("kbbbb2222", None, ""):
                with self.subTest(key=key):
                    r, started = self.run_nats_entrypoint(tmp, key, data)
                    self.assertEqual(r.returncode, 1, r.stdout)
                    self.assertFalse(started.exists(), "nats-server must not start with a wrong key")
                    self.assertNotIn("kbbbb2222", r.stdout)
                    self.assertEqual((data / "jetstream" / "store").read_text(), "retained")
            r, started = self.run_nats_entrypoint(tmp, "kaaaa1111", data)
            self.assertEqual(r.returncode, 0, r.stdout)
            self.assertTrue(started.exists())
            self.assertNotIn("kaaaa1111", (data / ".wasl-key-check").read_text())

    def test_nats_entrypoint_leaves_unmarked_retained_data_alone(self):
        with tempfile.TemporaryDirectory() as tmp:
            data = Path(tmp) / "data"
            (data / "jetstream").mkdir(parents=True)
            (data / "jetstream" / "store").write_text("retained")
            r, started = self.run_nats_entrypoint(tmp, "kaaaa1111", data)
            self.assertEqual(r.returncode, 0, r.stdout)
            self.assertIn("no key marker", r.stdout)
            self.assertFalse((data / ".wasl-key-check").exists(), "an unverified key must not be recorded")

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

    def check_compose(self, mutate):
        """Run check_deploy.py against a mutated copy of docker-compose.yml; returns (returncode, output)."""
        import yaml
        compose = yaml.safe_load((ROOT / "docker-compose.yml").read_text())
        mutate(compose)
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "docker-compose.yml"
            path.write_text(yaml.safe_dump(compose))
            env = dict(os.environ, WASL_COMPOSE_FILE=str(path))
            r = subprocess.run([sys.executable, str(ROOT / "scripts/check_deploy.py")], cwd=ROOT, env=env,
                               text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        return r.returncode, r.stdout

    def test_static_check_passes_on_committed_compose(self):
        code, out = self.check_compose(lambda c: None)
        self.assertEqual(code, 0, out)

    def test_static_check_rejects_short_syntax_nats_bind(self):
        code, out = self.check_compose(lambda c: c["services"]["nats-0"]["volumes"].append("./deploy/nats/nats.conf:/etc/nats/wasl.conf:ro"))
        self.assertEqual(code, 1, out)
        self.assertIn("bind-mounts host path", out)

    def test_static_check_rejects_long_syntax_nats_bind(self):
        bind = {"type": "bind", "source": "./deploy/nats/nats.conf", "target": "/etc/nats/wasl.conf", "read_only": True}
        code, out = self.check_compose(lambda c: c["services"]["nats-1"]["volumes"].append(bind))
        self.assertEqual(code, 1, out)
        self.assertIn("bind-mounts host path", out)

    def test_static_check_long_syntax_bind_source_checked_like_short(self):
        bind = {"type": "bind", "source": "./does/not/exist", "target": "/x"}
        code, out = self.check_compose(lambda c: c["services"]["api"].setdefault("volumes", []).append(bind))
        self.assertEqual(code, 1, out)
        self.assertIn("bind source ./does/not/exist missing", out)

    def test_static_check_accepts_long_syntax_named_volume(self):
        vol = {"type": "volume", "source": "nats-2", "target": "/data"}
        def mutate(c):
            c["services"]["nats-2"]["volumes"] = [vol]
        code, out = self.check_compose(mutate)
        self.assertEqual(code, 0, out)


if __name__ == "__main__":
    unittest.main()
