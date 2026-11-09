#!/usr/bin/env python3
"""Production-shaped deployment contracts, using generated throwaway secrets only."""
import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


class ProductionDeploymentTests(unittest.TestCase):
    def test_secret_bootstrap_is_file_backed_nonprinting_and_non_overwriting(self):
        with tempfile.TemporaryDirectory() as tmp:
            secrets = Path(tmp) / "secrets"
            result = subprocess.run([str(ROOT / "scripts/production_secrets.sh"), str(secrets)],
                                    text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(result.returncode, 0, result.stdout)
            required = {"js_key", "nats_password", "route_password", "api_users.json", "backup_key",
                        "ca.key", "ca.crt", "server.key", "server.crt"}
            self.assertTrue(required.issubset({p.name for p in secrets.iterdir()}))
            self.assertEqual(stat.S_IMODE(secrets.stat().st_mode), 0o700)
            certificate = subprocess.run(["openssl", "x509", "-in", str(secrets / "server.crt"), "-noout", "-text"],
                                         check=True, text=True, stdout=subprocess.PIPE).stdout
            self.assertIn("TLS Web Client Authentication", certificate, "NATS routes mutually authenticate as TLS clients")
            self.assertIn("Authority Key Identifier", certificate)
            ca_certificate = subprocess.run(["openssl", "x509", "-in", str(secrets / "ca.crt"), "-noout", "-text"],
                                            check=True, text=True, stdout=subprocess.PIPE).stdout
            self.assertIn("CA:TRUE", ca_certificate)
            users = json.loads((secrets / "api_users.json").read_text())
            self.assertEqual({u["role"] for u in users.values()}, {"coordinator", "clinician", "reviewer", "auditor"})
            for value in [(secrets / name).read_text().strip() for name in ("js_key", "nats_password", "route_password", "backup_key")]:
                self.assertNotIn(value, result.stdout, "secret material appeared in bootstrap output")
            before = (secrets / "js_key").read_bytes()
            again = subprocess.run([str(ROOT / "scripts/production_secrets.sh"), str(secrets)],
                                   text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertNotEqual(again.returncode, 0)
            self.assertEqual((secrets / "js_key").read_bytes(), before)

    def test_secret_volume_loader_copies_files_without_bind_mounts_or_logging_values(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            secrets = root / "secrets"
            subprocess.run([str(ROOT / "scripts/production_secrets.sh"), str(secrets)], check=True,
                           stdout=subprocess.PIPE, text=True)
            log = root / "engine.log"
            fake = root / "podman"
            fake.write_text('''#!/bin/sh
printf "%s\\n" "$*" >> "$FAKE_ENGINE_LOG"
if [ "${1:-}" = volume ] && [ "${2:-}" = inspect ] && [ "${FAKE_VOLUME_EXISTS:-0}" != 1 ]; then exit 1; fi
if [ "${1:-}" = cp ]; then
  for arg in "$@"; do
    if [ -d "$arg" ] && [ -e "$arg/ca.key" ]; then echo "CA private key copied" >&2; exit 9; fi
  done
fi
''')
            fake.chmod(0o755)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"], FAKE_ENGINE_LOG=str(log))
            result = subprocess.run([str(ROOT / "scripts/production_volume_load.sh"), "wasl-test-secrets", str(secrets)],
                                    env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(result.returncode, 0, result.stdout)
            commands = log.read_text()
            self.assertIn("volume create wasl-test-secrets", commands)
            self.assertIn("cp ", commands)
            self.assertNotIn(":/source", commands, "host bind mounts do not work in the supported Podman VM")
            for name in ("js_key", "nats_password", "route_password", "backup_key"):
                self.assertNotIn((secrets / name).read_text().strip(), result.stdout)
            env["FAKE_VOLUME_EXISTS"] = "1"
            existing = subprocess.run([str(ROOT / "scripts/production_volume_load.sh"), "wasl-test-secrets", str(secrets)],
                                      env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertNotEqual(existing.returncode, 0, "an existing secret volume must never be overwritten")

    def test_nats_entrypoint_reads_key_from_file_and_refuses_missing_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            bindir = root / "bin"
            bindir.mkdir()
            started = root / "started"
            fake = bindir / "docker-entrypoint.sh"
            fake.write_text('#!/bin/sh\necho started >> "$FAKE_STARTED"\nprintf "%s\\n" "$*" > "$FAKE_ARGS"\n')
            fake.chmod(0o755)
            wget = bindir / "wget"
            wget.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$FAKE_WGET_LOG"\nexit 0\n')
            wget.chmod(0o755)
            data = root / "data"
            data.mkdir()
            key = root / "key"
            key.write_text("k-file-backed-secret\n")
            env = dict(os.environ, PATH=str(bindir) + os.pathsep + os.environ["PATH"],
                       WASL_NATS_DATA=str(data), FAKE_STARTED=str(started), FAKE_ARGS=str(root / "args"),
                       FAKE_WGET_LOG=str(root / "wget.log"), WASL_JS_KEY_FILE=str(key), WASL_NATS_TLS="1",
                       WASL_ROUTE_USER="route-user", WASL_ROUTE_PASSWORD="route-password",
                       WASL_NATS_RUNTIME_CONFIG=str(root / "rendered.conf"),
                       WASL_NATS_CONFIG_TEMPLATE=str(ROOT / "deploy/nats/nats-production.conf"))
            env.pop("WASL_JS_KEY", None)
            result = subprocess.run(["sh", str(ROOT / "deploy/nats/entrypoint.sh"), "nats-server"], env=env,
                                    text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertTrue(started.exists())
            self.assertNotIn("k-file-backed-secret", result.stdout)
            self.assertIn("--no-check-certificate", (root / "wget.log").read_text())
            rendered = (root / "rendered.conf").read_text()
            self.assertIn("nats-route://route-user:route-password@nats-0:6222", rendered)
            self.assertIn(str(root / "rendered.conf"), (root / "args").read_text())
            started.unlink()
            env["WASL_JS_KEY_FILE"] = str(root / "missing")
            result = subprocess.run(["sh", str(ROOT / "deploy/nats/entrypoint.sh"), "nats-server"], env=env,
                                    text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(started.exists())


if __name__ == "__main__":
    unittest.main()
