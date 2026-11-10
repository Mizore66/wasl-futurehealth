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
            # an all-digit key that nats-server parses as an out-of-range number makes it fail before JetStream is up:
            # its exit code must pass through
            bad_key = "9" * 30
            r, started = self.run_nats_entrypoint(tmp, bad_key, data, ready=False, exit_code=3)
            self.assertEqual(r.returncode, 3, r.stdout)
            self.assertFalse((data / ".wasl-key-check").exists(), "a failed first start must not record the key")
            self.assertNotIn(bad_key, r.stdout)
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
        self.assertIn("github.com/nats-io/nats-server/v2@v${NATS_VERSION}", df)
        self.assertIn("github.com/klauspost/compress@v1.20.0", df)
        self.assertIn("golang.org/x/crypto@v0.58.1-0.20261009181718-2f1f834330b2", df)
        self.assertIn("FROM golang:${GO_VERSION}-bookworm AS build", df)
        self.assertIn("FROM --platform=linux/${TARGETARCH} nats:${NATS_VERSION}-alpine", df)
        self.assertIn("USER 10001:10001", df)
        self.assertIn("chown -R 10001:10001 /data", df)
        self.assertIn('VOLUME ["/data"]', df)

    def test_nats_base_is_resolved_for_target_architecture(self):
        # A cached single-architecture tag must not override the requested build target.
        df = (ROOT / "deploy/nats/Dockerfile").read_text()
        self.assertIn("FROM --platform=linux/${TARGETARCH} nats:${NATS_VERSION}-alpine", df)

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

    def test_demo_check_ignores_empty_lsof_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            bindir = Path(tmp) / "bin"
            bindir.mkdir()
            lsof = bindir / "lsof"
            lsof.write_text("#!/bin/sh\nexit 0\n")
            lsof.chmod(0o755)
            env = os.environ.copy()
            env.update(PATH=str(bindir) + os.pathsep + "/opt/anaconda3/bin" + os.pathsep + env["PATH"],
                       WASL_NATS_EXTERNAL="1", WASL_NATS_URL="nats://nats-0:4222")
            result = subprocess.run([str(ROOT / "scripts/demo.sh"), "check"], cwd=ROOT, env=env,
                                    text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertIn("assets OK", result.stdout)

    def test_demo_check_uses_listener_table_not_all_open_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            bindir = Path(tmp) / "bin"
            bindir.mkdir()
            lsof = bindir / "lsof"
            lsof.write_text("#!/bin/sh\nprintf '1 /bin/bash 0 /dev/null\\n'\n")
            lsof.chmod(0o755)
            netstat = bindir / "netstat"
            netstat.write_text("#!/bin/sh\nprintf 'Active Internet connections (only servers)\\n'\n")
            netstat.chmod(0o755)
            env = os.environ.copy()
            env.update(PATH=str(bindir) + os.pathsep + "/opt/anaconda3/bin" + os.pathsep + env["PATH"],
                       WASL_NATS_EXTERNAL="1", WASL_NATS_URL="nats://nats-0:4222")
            result = subprocess.run([str(ROOT / "scripts/demo.sh"), "check"], cwd=ROOT, env=env,
                                    text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertIn("assets OK", result.stdout)

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

    def snapshot_env(self, tmp, curl_body):
        bindir = Path(tmp) / "bin"
        bindir.mkdir()
        curl = bindir / "curl"
        curl.write_text("#!/usr/bin/env bash\n" + curl_body)
        curl.chmod(0o755)
        samples = Path(tmp) / "api-samples"
        samples.mkdir()
        (samples / "README.md").write_text("keep\n")
        (samples / "tracked.json").write_text("{}\n")
        env = {k: v for k, v in os.environ.items() if not k.startswith("WASL_")}
        env.update(PATH=str(bindir) + os.pathsep + env["PATH"], WASL_BIN=str(bindir), WASL_SNAPSHOT_DIR=str(samples),
                   WASL_SNAPSHOT_PROGRAMMES="crc")
        return env, samples

    def staging_dirs(self, tmp):
        return [p for p in Path(tmp).iterdir() if ".staging." in p.name]

    def test_snapshot_sigint_removes_staging_and_keeps_samples(self):
        import signal, time
        with tempfile.TemporaryDirectory() as tmp:
            env, samples = self.snapshot_env(tmp, 'case "$*" in *dashboard*) sleep 30;; esac\necho "{}"\n')
            p = subprocess.Popen([str(ROOT / "scripts/demo.sh"), "snapshot"], cwd=ROOT, env=env, start_new_session=True,
                                 stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            try:
                for _ in range(100):
                    if self.staging_dirs(tmp):
                        break
                    time.sleep(0.1)
                self.assertTrue(self.staging_dirs(tmp), "staging directory never appeared")
                time.sleep(0.5)
                os.killpg(p.pid, signal.SIGINT)  # like Ctrl-C: the whole foreground group
                p.communicate(timeout=20)
            finally:
                if p.poll() is None:
                    os.killpg(p.pid, signal.SIGKILL)
            self.assertEqual(self.staging_dirs(tmp), [])
            self.assertEqual((samples / "tracked.json").read_text(), "{}\n")

    def test_snapshot_sigterm_exits_143_and_cleans_everything(self):
        import signal, time
        with tempfile.TemporaryDirectory() as tmp:
            env, samples = self.snapshot_env(tmp, 'case "$*" in *dashboard*) sleep 30;; esac\necho "{}"\n')
            p = subprocess.Popen([str(ROOT / "scripts/demo.sh"), "snapshot"], cwd=ROOT, env=env, start_new_session=True,
                                 stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            try:
                for _ in range(100):
                    if self.staging_dirs(tmp):
                        break
                    time.sleep(0.1)
                self.assertTrue(self.staging_dirs(tmp), "staging directory never appeared")
                time.sleep(0.5)
                os.killpg(p.pid, signal.SIGTERM)
                p.communicate(timeout=20)
            finally:
                if p.poll() is None:
                    os.killpg(p.pid, signal.SIGKILL)
            self.assertEqual(p.returncode, 143)
            self.assertEqual([x.name for x in Path(tmp).iterdir() if ".staging." in x.name or ".old." in x.name], [])
            self.assertEqual((samples / "tracked.json").read_text(), "{}\n")
            self.assertEqual((samples / "README.md").read_text(), "keep\n")

    def test_snapshot_curls_are_time_bounded(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = Path(tmp) / "curl.log"
            body = 'echo "$*" >> %s\ncase "$*" in *fixtures*) echo \'{"fixtures":{}}\';; *programme*) echo \'{"cases":[1]}\';; *) echo "{}";; esac\n' % log
            env, samples = self.snapshot_env(tmp, body)
            r = subprocess.run([str(ROOT / "scripts/demo.sh"), "snapshot"], cwd=ROOT, env=env, text=True,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(r.returncode, 0, r.stdout)
            lines = log.read_text().splitlines()
            self.assertTrue(lines)
            for l in lines:
                self.assertIn("--max-time", l)

    def test_snapshot_success_removes_stale_files_keeps_readme(self):
        ok = 'case "$*" in *fixtures*) echo \'{"fixtures":{}}\';; *programme*) echo \'{"cases":[1]}\';; *) echo "{}";; esac\n'
        with tempfile.TemporaryDirectory() as tmp:
            env, samples = self.snapshot_env(tmp, ok)
            (samples / "stale_case.json").write_text("old\n")
            r = subprocess.run([str(ROOT / "scripts/demo.sh"), "snapshot"], cwd=ROOT, env=env, text=True,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(r.returncode, 0, r.stdout)
            self.assertFalse((samples / "stale_case.json").exists())
            self.assertFalse((samples / "tracked.json").exists(), "old samples not in the new set are removed")
            self.assertEqual((samples / "README.md").read_text(), "keep\n")
            self.assertTrue((samples / "dashboard.json").exists())
            self.assertEqual([x.name for x in Path(tmp).iterdir() if ".staging." in x.name or ".old." in x.name], [])

    def test_snapshot_failure_and_success_leave_no_staging(self):
        ok = 'case "$*" in *fixtures*) echo \'{"fixtures":{}}\';; *programme*) echo \'{"cases":[1]}\';; *) echo "{}";; esac\n'
        with tempfile.TemporaryDirectory() as tmp:
            env, samples = self.snapshot_env(tmp, "exit 22\n")
            r = subprocess.run([str(ROOT / "scripts/demo.sh"), "snapshot"], cwd=ROOT, env=env, text=True,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertNotEqual(r.returncode, 0)
            self.assertEqual(self.staging_dirs(tmp), [])
            self.assertEqual((samples / "tracked.json").read_text(), "{}\n")
        with tempfile.TemporaryDirectory() as tmp:
            env, samples = self.snapshot_env(tmp, ok)
            r = subprocess.run([str(ROOT / "scripts/demo.sh"), "snapshot"], cwd=ROOT, env=env, text=True,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(r.returncode, 0, r.stdout)
            self.assertEqual(self.staging_dirs(tmp), [])
            self.assertTrue((samples / "healthz.json").exists())
            self.assertTrue((samples / "README.md").exists())

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

    def test_static_check_rejects_missing_parent_relative_bind_source(self):
        code, out = self.check_compose(lambda c: c["services"]["api"].setdefault("volumes", []).append("../nonexistent-wasl-dir:/x"))
        self.assertEqual(code, 1, out)
        self.assertIn("bind source ../nonexistent-wasl-dir missing", out)

    def test_static_check_accepts_env_expression_and_anonymous_volume(self):
        def mutate(c):
            vols = c["services"]["api"].setdefault("volumes", [])
            vols.append("${DATA:-./data}:/x")
            vols.append("/var/lib/x")
            vols.append({"type": "volume", "target": "/anon"})
        code, out = self.check_compose(mutate)
        self.assertEqual(code, 0, out)

    def test_static_check_anonymous_absolute_volume_with_missing_path_is_skipped(self):
        code, out = self.check_compose(lambda c: c["services"]["api"].setdefault("volumes", []).append("/nonexistent-wasl-dir/x:/x"))
        self.assertEqual(code, 0, out)

    def test_static_check_rejects_missing_tilde_bind_source(self):
        code, out = self.check_compose(lambda c: c["services"]["api"].setdefault("volumes", []).append("~/nonexistent-wasl-dir-xyz:/x"))
        self.assertEqual(code, 1, out)
        self.assertIn("bind source ~/nonexistent-wasl-dir-xyz missing", out)

    def test_static_check_rejects_missing_bare_relative_bind_source(self):
        code, out = self.check_compose(lambda c: c["services"]["api"].setdefault("volumes", []).append("nonexistent-wasl/dir:/x"))
        self.assertEqual(code, 1, out)
        self.assertIn("bind source nonexistent-wasl/dir missing", out)

    def test_static_check_rejects_env_expression_nats_bind(self):
        code, out = self.check_compose(lambda c: c["services"]["nats-0"]["volumes"].append("${DATA:-./data}:/x"))
        self.assertEqual(code, 1, out)
        self.assertIn("bind-mounts host path", out)

    def test_static_check_accepts_existing_absolute_bind_source(self):
        code, out = self.check_compose(lambda c: c["services"]["api"].setdefault("volumes", []).append(str(ROOT / "scripts") + ":/x"))
        self.assertEqual(code, 0, out)

    def check_production(self, mutate):
        """Run check_deploy.py against a mutated copy of deploy/production.compose.yml; returns (returncode, output)."""
        import yaml
        compose = yaml.safe_load((ROOT / "deploy/production.compose.yml").read_text())
        mutate(compose)
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "production.compose.yml"
            path.write_text(yaml.safe_dump(compose))
            env = dict(os.environ, WASL_PRODUCTION_COMPOSE_FILE=str(path))
            r = subprocess.run([sys.executable, str(ROOT / "scripts/check_deploy.py")], cwd=ROOT, env=env,
                               text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        return r.returncode, r.stdout

    def test_production_nats_user_matches_image_owner(self):
        import yaml
        dockerfile = (ROOT / "deploy/nats/Dockerfile").read_text().splitlines()
        image_user = [l.split()[1] for l in dockerfile if l.startswith("USER ")][-1]
        services = yaml.safe_load((ROOT / "deploy/production.compose.yml").read_text())["services"]
        for name in ("nats-0", "nats-1", "nats-2"):
            self.assertEqual(services[name].get("user"), image_user, name)
        self.assertIn("chown -R %s" % image_user, "\n".join(dockerfile))

    def test_static_check_rejects_production_nats_user_mismatch(self):
        def mutate(c):
            c["services"]["nats-1"]["user"] = "1000:1000"
        code, out = self.check_production(mutate)
        self.assertEqual(code, 1, out)
        self.assertIn("production compose: nats-1 runs as user '1000:1000'", out)

    def test_static_check_rejects_production_nats_without_user(self):
        code, out = self.check_production(lambda c: c["services"]["nats-2"].pop("user"))
        self.assertEqual(code, 1, out)
        self.assertIn("production compose: nats-2 runs as user None", out)

    def test_static_check_rejects_dev_nats_user_mismatch(self):
        code, out = self.check_compose(lambda c: c["services"]["nats-0"].__setitem__("user", "1000:1000"))
        self.assertEqual(code, 1, out)
        self.assertIn("compose: nats-0 runs as user '1000:1000'", out)


if __name__ == "__main__":
    unittest.main()
