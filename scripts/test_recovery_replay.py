#!/usr/bin/env python3
"""Transactional v2 restore, keyring rotation, deployment keyring and local single-node NATS replay comparison.

Logical hashes come from the Go `wsi7` summaries (never reimplemented here). This is a local single-node check; it is
not a Linux Docker, multi-node or soak result.
"""
import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import stat
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

SCRIPTS = Path(__file__).resolve().parent
ROOT = SCRIPTS.parent
sys.path.insert(0, str(SCRIPTS))

import recovery  # noqa: E402
import soak  # noqa: E402
from recovery import migrate_files, rollback_files, write_file_bundle  # noqa: E402
from recovery_crypto import load_keyring, new_keyring, rotate_keyring  # noqa: E402
from test_recovery import write_v1_bundle  # noqa: E402

NATS_SERVER = shutil.which("nats-server")


def make_tree(root: Path) -> Path:
    source = root / "source"
    (source / "run/empty").mkdir(parents=True)
    (source / "run/empty").chmod(0o750)
    (source / "run/audit.json").write_text('{"synthetic":true}\n')
    (source / "run/audit.json").chmod(0o640)
    (source / "run/tool.sh").write_text("#!/bin/sh\n")
    (source / "run/tool.sh").chmod(0o700)
    (source / "top.txt").write_text("top")
    return source


def tree_state(path: Path) -> dict:
    out = {}
    for item in sorted(path.rglob("*")):
        rel = item.relative_to(path).as_posix()
        out[rel] = (stat.S_IMODE(item.stat().st_mode), item.read_bytes() if item.is_file() else None)
    return out


def flip_last_byte(path: Path) -> None:
    data = path.read_bytes()
    path.write_bytes(data[:-1] + bytes([data[-1] ^ 1]))


def cli(*args):
    return subprocess.run([sys.executable, str(SCRIPTS / "recovery.py"), *map(str, args)], stdout=subprocess.PIPE,
                          stderr=subprocess.STDOUT, text=True)


class TransactionalRestoreTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root, True)
        self.keys = self.root / "recovery_keys.json"
        new_keyring(self.keys)
        self.source = make_tree(self.root)
        self.nats = self.root / "nats.json"
        self.nats.write_text('{"schema":"wasl-nats-backup-v1","streams":[]}')
        self.work = self.root / "work"
        self.work.mkdir()

    def backup(self, name="bundle"):
        write_file_bundle(self.source, self.root / name, self.keys, nats_json=self.nats)
        return self.root / name

    def leftovers(self):
        return sorted(p.name for p in self.work.iterdir() if ".restore-" in p.name)

    def test_restore_reproduces_bytes_modes_and_empty_directories_without_plaintext_nats(self):
        bundle = self.backup()
        destination = self.work / "live"
        journal = migrate_files(bundle, self.keys, destination)
        self.assertEqual(tree_state(destination), tree_state(self.source))
        self.assertTrue((destination / "run/empty").is_dir())
        self.assertFalse((destination / "nats").exists(), "NATS payload must not be left as plaintext in the run dir")
        self.assertIsNone(journal["rollback_path"])
        self.assertEqual(self.leftovers(), [])

    def test_replace_rolls_back_a_populated_destination(self):
        bundle = self.backup()
        destination = self.work / "live"
        destination.mkdir()
        (destination / "old").write_text("old generation")
        journal = migrate_files(bundle, self.keys, destination, replace=True)
        self.assertFalse((destination / "old").exists())
        self.assertEqual((destination / "top.txt").read_text(), "top")
        rollback_files(journal)
        self.assertEqual((destination / "old").read_text(), "old generation")
        populated = self.work / "other"
        populated.mkdir()
        (populated / "keep").write_text("x")
        with self.assertRaisesRegex(ValueError, "not empty"):
            migrate_files(bundle, self.keys, populated)
        self.assertEqual((populated / "keep").read_text(), "x")

    def test_failed_install_restores_previous_generation_and_cleans_staging(self):
        bundle = self.backup()
        destination = self.work / "live"
        destination.mkdir()
        (destination / "old").write_text("old generation")
        resolved = destination.resolve()
        real = os.replace

        def flaky(src, dst, *args, **kwargs):
            if Path(dst) == resolved and ".restore-" in str(src):
                raise OSError("simulated install failure")
            return real(src, dst, *args, **kwargs)
        with mock.patch.object(recovery.os, "replace", flaky), self.assertRaises(OSError):
            migrate_files(bundle, self.keys, destination, replace=True)
        self.assertEqual((destination / "old").read_text(), "old generation")
        self.assertEqual(self.leftovers(), [])

    def test_rotation_restores_prior_and_new_bundles(self):
        first = self.backup("first")
        before, old_id, _ = load_keyring(self.keys)
        new_id = rotate_keyring(self.keys)
        second = self.backup("second")
        after, active, _ = load_keyring(self.keys)
        self.assertEqual((active, set(after["keys"])), (new_id, set(before["keys"]) | {new_id}))
        self.assertEqual(json.loads((first / "manifest.json").read_text())["key_id"], old_id)
        self.assertEqual(json.loads((second / "manifest.json").read_text())["key_id"], new_id)
        for name, bundle in (("a", first), ("b", second)):
            migrate_files(bundle, self.keys, self.work / name)
            self.assertEqual(tree_state(self.work / name), tree_state(self.source))

    def test_tampered_or_wrong_key_bundles_leave_destination_untouched(self):
        bad = self.backup("bad")
        flip_last_byte(bad / "files/top.txt.aead")
        good = self.backup("good")
        wrong = self.root / "other_keys.json"
        new_keyring(wrong)
        destination = self.work / "live"
        destination.mkdir()
        (destination / "keep").write_text("keep")
        for bundle, keys in ((bad, self.keys), (good, wrong)):
            with self.assertRaises(ValueError):
                migrate_files(bundle, keys, destination, replace=True)
        self.assertEqual(os.listdir(destination), ["keep"])
        self.assertEqual(sorted(p.name for p in self.work.iterdir()), ["live"])

    def test_legacy_bundle_requires_explicit_opt_in_for_restore(self):
        key = self.root / "backup_key"
        key.write_text("k" * 64 + "\n")
        legacy = self.root / "legacy"
        write_v1_bundle(self.source, legacy, key)
        with self.assertRaisesRegex(ValueError, "legacy"):
            migrate_files(legacy, key, self.work / "l1")
        self.assertFalse((self.work / "l1").exists())
        migrate_files(legacy, key, self.work / "l1", legacy_hmac=True)
        self.assertEqual((self.work / "l1/top.txt").read_text(), "top")

    def test_cleanup_failure_is_reported_not_hidden(self):
        bundle = self.backup()
        original = recovery.install_staging_with_rollback

        def install_leaving_residue(staging, destination, replace):
            result = original(staging, destination, replace)
            Path(staging).mkdir()
            (Path(staging) / "residue").write_text("x")
            return result

        def failing_rmtree(path, *args, **kwargs):
            kwargs["onerror"](os.rmdir, str(path), (OSError, OSError("denied"), None))
        with mock.patch.object(recovery, "install_staging_with_rollback", install_leaving_residue), \
                mock.patch.object(recovery.shutil, "rmtree", failing_rmtree):
            with self.assertRaisesRegex(ValueError, "could not be fully removed"):
                migrate_files(bundle, self.keys, self.work / "live")
        journal = json.loads((self.work / ".live.migration.json").read_text())
        self.assertEqual(journal["destination"], str((self.work / "live").resolve()))
        self.assertEqual((self.work / "live/top.txt").read_text(), "top")

    def resign(self, bundle, mutate):
        manifest = json.loads((bundle / "manifest.json").read_text())
        mutate(manifest)
        manifest.pop("hmac_sha256", None)
        doc, _, _ = load_keyring(self.keys)
        auth = base64.b64decode(doc["keys"][manifest["key_id"]]["auth_key"])
        manifest["hmac_sha256"] = recovery._manifest_mac(manifest, auth)
        (bundle / "manifest.json").write_bytes(recovery._canonical(manifest) + b"\n")

    def test_special_mode_bits_are_masked_at_backup_and_rejected_in_manifests(self):
        os.chmod(self.source / "run/empty", 0o1777)
        os.chmod(self.source / "top.txt", 0o4755)
        bundle = self.backup()
        manifest = json.loads((bundle / "manifest.json").read_text())
        self.assertTrue(all(0 <= d["mode"] <= 0o777 for d in manifest["dirs"]))
        self.assertTrue(all(0 <= f["mode"] <= 0o777 for f in manifest["files"]))
        migrate_files(bundle, self.keys, self.work / "live")
        self.assertEqual(stat.S_IMODE((self.work / "live/top.txt").stat().st_mode), 0o755)
        for label, mutate in (("dir", lambda m: m["dirs"][0].__setitem__("mode", 0o1777)),
                              ("file", lambda m: m["files"][0].__setitem__("mode", 0o4755))):
            bad = self.backup("bad-" + label)
            self.resign(bad, mutate)
            with self.assertRaisesRegex(ValueError, "malformed"):
                migrate_files(bad, self.keys, self.work / ("x-" + label))
            self.assertFalse((self.work / ("x-" + label)).exists())

    def test_directory_modes_apply_deepest_first(self):
        bundle = self.backup()
        self.resign(bundle, lambda m: [d.__setitem__("mode", 0o600) for d in m["dirs"] if d["path"] == "run"])
        destination = self.work / "live"
        migrate_files(bundle, self.keys, destination)
        self.addCleanup(os.chmod, destination / "run", 0o700)
        self.assertEqual(stat.S_IMODE((destination / "run").stat().st_mode), 0o600)
        self.assertEqual(self.leftovers(), [])
        os.chmod(destination / "run", 0o700)
        self.assertEqual(stat.S_IMODE((destination / "run/empty").stat().st_mode), 0o750)

    def test_ciphertext_path_collision_is_rejected_cleanly(self):
        (self.source / "a").write_text("1")
        (self.source / "a.aead").mkdir()
        (self.source / "a.aead/x").write_text("2")
        with self.assertRaisesRegex(ValueError, "unsafe or duplicate"):
            write_file_bundle(self.source, self.root / "collide", self.keys)
        self.assertFalse((self.root / "collide").exists())
        self.assertEqual([p.name for p in self.root.iterdir() if "staging" in p.name], [])

    def test_extract_nats_authenticates_first_and_writes_private_file(self):
        bundle = self.backup()
        out = self.root / "extracted.json"
        self.assertEqual(cli("extract-nats", bundle, out, "--key-file", self.keys).returncode, 0)
        self.assertEqual(out.read_bytes(), self.nats.read_bytes())
        self.assertEqual(stat.S_IMODE(out.stat().st_mode), 0o600)
        flip_last_byte(bundle / "files/nats/nats.json.aead")
        refused = self.root / "refused.json"
        self.assertNotEqual(cli("extract-nats", bundle, refused, "--key-file", self.keys).returncode, 0)
        self.assertFalse(refused.exists())


class ShellRestoreOrderTests(unittest.TestCase):
    """demo.sh recovery-restore with a stub wsi7: files are staged/decrypted before NATS, installed after."""

    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root, True)
        self.keys = self.root / "recovery_keys.json"
        new_keyring(self.keys)
        self.source = make_tree(self.root)
        self.nats = self.root / "nats.json"
        self.nats.write_text('{"schema":"wasl-nats-backup-v1","streams":[]}')
        self.backups = self.root / "backups"
        self.backups.mkdir()
        write_file_bundle(self.source, self.backups / "bundle", self.keys, nats_json=self.nats)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.calls = self.root / "calls"
        self.out = self.root / "work" / "out"
        self.out.parent.mkdir()
        shim = self.bin / "python3"
        shim.write_text(f'#!/bin/sh\nexec "{sys.executable}" "$@"\n')
        shim.chmod(0o755)
        self.stub(0)

    def stub(self, code):
        wsi = self.bin / "wsi7"
        # records the call, whether the destination holds files yet, and whether a staging dir exists right now
        wsi.write_text(f'#!/bin/sh\necho "$1 out=$(ls "{self.out}" 2>/dev/null | wc -l | tr -d " ") '
                       f'staged=$(ls -a "{self.out.parent}" | grep -c restore-)" >> "{self.calls}"\nexit {code}\n')
        wsi.chmod(0o755)

    def run_restore(self, key=None, **extra):
        env = dict(os.environ, PATH=f"{self.bin}{os.pathsep}{os.environ['PATH']}", WASL_BIN=str(self.bin),
                   WASL_OUT=str(self.out), WASL_RECOVERY_DIR=str(self.backups),
                   WASL_RECOVERY_KEY_FILE=str(key or self.keys), WASL_NATS_URL="nats://invalid:4222",
                   WASL_NATS_EXTERNAL="1", **extra)
        return subprocess.run([str(SCRIPTS / "demo.sh"), "recovery-restore"], env=env, text=True,
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT)

    def recorded(self):
        return self.calls.read_text().splitlines() if self.calls.exists() else []

    def test_success_order_is_stage_then_nats_then_install(self):
        result = self.run_restore()
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertEqual(self.recorded(), ["restore-nats out=0 staged=1"])
        self.assertEqual((self.out / "top.txt").read_text(), "top")
        self.assertEqual(sorted(p.name for p in self.out.parent.iterdir()), [".out.migration.json", "out"])

    def test_populated_destination_without_replace_never_reaches_nats(self):
        self.out.mkdir()
        (self.out / "keep").write_text("keep")
        result = self.run_restore()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.recorded(), [])
        self.assertEqual(os.listdir(self.out), ["keep"])

    def test_wrong_key_and_invalid_destination_leave_nats_untouched(self):
        wrong = self.root / "wrong.json"
        new_keyring(wrong)
        self.assertNotEqual(self.run_restore(key=wrong).returncode, 0)
        self.assertEqual(self.recorded(), [])
        self.assertFalse(self.out.exists())
        self.out.write_text("a file, not a directory")
        self.assertNotEqual(self.run_restore().returncode, 0)
        self.assertEqual(self.recorded(), [])

    def test_nats_failure_installs_nothing_and_names_the_manual_purge(self):
        self.out.mkdir()
        (self.out / "old").write_text("old")
        self.stub(3)
        result = self.run_restore(WASL_RECOVERY_REPLACE="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(self.recorded()), 1)
        self.assertIn("manual", result.stdout)
        self.assertEqual(os.listdir(self.out), ["old"])
        self.assertEqual([p.name for p in self.out.parent.iterdir() if "restore-" in p.name], [])

    def test_nats_payload_temp_is_gone_after_restore(self):
        pattern = ".wasl-nats-payload-*"
        roots = (self.out.parent, Path(tempfile.gettempdir()))
        before = {str(p) for r in roots for p in r.glob(pattern)}
        self.run_restore()
        self.assertEqual({str(p) for r in roots for p in r.glob(pattern)}, before)


class DeploymentKeyringTests(unittest.TestCase):
    def test_secret_bootstrap_adds_keyring_without_printing_and_keeps_legacy_key(self):
        with tempfile.TemporaryDirectory() as tmp:
            secrets = Path(tmp) / "secrets"
            result = subprocess.run([str(SCRIPTS / "production_secrets.sh"), str(secrets)], text=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(result.returncode, 0, result.stdout)
            self.assertTrue((secrets / "backup_key").is_file())
            doc, active, entry = load_keyring(secrets / "recovery_keys.json")
            self.assertEqual(list(doc["keys"]), [active])
            for value in entry.values():
                self.assertNotIn(value, result.stdout)
            self.assertEqual(stat.S_IMODE((secrets / "recovery_keys.json").stat().st_mode), 0o444)

    def test_demo_and_compose_default_to_the_keyring(self):
        demo = (SCRIPTS / "demo.sh").read_text()
        self.assertIn("/run/secrets/recovery_keys.json", demo)
        self.assertNotIn("/run/secrets/backup_key", demo)
        self.assertIn("recovery_keys.json", (ROOT / "deploy/production.compose.yml").read_text())
        self.assertIn("recovery_keys.json", (SCRIPTS / "production_volume_load.sh").read_text())


def free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


@unittest.skipUnless(NATS_SERVER and shutil.which("go"), "nats-server and go are required for the replay drill")
class NatsReplayTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root = Path(tempfile.mkdtemp())
        cls.binary = cls.root / "wsi7"
        subprocess.run(["go", "build", "-o", str(cls.binary), "."], cwd=ROOT / "pipeline", check=True)
        cls.procs = []

    @classmethod
    def tearDownClass(cls):
        for proc in cls.procs:
            proc.terminate()
            proc.wait(10)
        shutil.rmtree(cls.root, ignore_errors=True)

    def start_nats(self, name):
        port = free_port()
        proc = subprocess.Popen([NATS_SERVER, "-a", "127.0.0.1", "-p", str(port), "-js", "-sd",
                                 str(self.root / f"store-{name}")], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.procs.append(proc)
        for _ in range(100):
            try:
                socket.create_connection(("127.0.0.1", port), 0.2).close()
                return f"nats://127.0.0.1:{port}"
            except OSError:
                time.sleep(0.1)
        raise RuntimeError("nats-server did not start")

    def wsi(self, url, *args):
        env = dict(os.environ, WASL_NATS_URL=url, WASL_NATS_EXTERNAL="1")
        result = subprocess.run([str(self.binary), *args], env=env, text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, cwd=self.root)
        self.assertEqual(result.returncode, 0, result.stderr[-2000:])
        return json.loads(result.stdout.strip().splitlines()[-1])

    def seed(self, url):
        subprocess.run(["go", "test", "-run", "^TestSeedReplayFixture$", "-count=1", "."], cwd=ROOT / "pipeline",
                       env=dict(os.environ, WASL_REPLAY_SEED_URL=url), check=True, stdout=subprocess.DEVNULL)

    @staticmethod
    def export(comparison):
        document = {"meta": {"loaded_at": "t"}, "comparison_sha256": comparison}
        document["content_sha256"] = hashlib.sha256(soak.export_canonical(
            {k: v for k, v in document.items() if k != "content_sha256"})).hexdigest()
        return json.dumps(document)

    def test_encrypted_bundle_replay_preserves_per_stream_logical_hashes(self):
        source_url, target_url = self.start_nats("source"), self.start_nats("target")
        self.seed(source_url)
        work = self.root / "replay"
        work.mkdir()
        plain = work / "nats.json"
        source_summary = self.wsi(source_url, "backup-nats", "-bundle", str(plain))["summary"]
        self.assertGreaterEqual(source_summary["streams"], 2)
        self.assertGreaterEqual(source_summary["messages"], 4)
        keys = work / "recovery_keys.json"
        new_keyring(keys)
        files = make_tree(work)
        bundle = work / "bundle"
        write_file_bundle(files, bundle, keys, nats_json=plain)
        plain.unlink()
        wrong = work / "wrong.json"
        new_keyring(wrong)
        extracted = work / "extracted.json"
        self.assertNotEqual(cli("extract-nats", bundle, extracted, "--key-file", wrong).returncode, 0)
        self.assertFalse(extracted.exists())
        self.assertEqual(cli("extract-nats", bundle, extracted, "--key-file", keys).returncode, 0)
        restored = self.wsi(target_url, "restore-nats", "-bundle", str(extracted))["summary"]
        extracted.unlink()
        self.assertEqual(restored["logical_sha256_by_stream"], source_summary["logical_sha256_by_stream"])
        target_summary = self.wsi(target_url, "backup-nats", "-bundle", str(work / "target-nats.json"))["summary"]
        self.assertEqual(target_summary["logical_sha256_by_stream"], source_summary["logical_sha256_by_stream"])
        self.assertEqual(target_summary["messages"], source_summary["messages"])
        migrate_files(bundle, keys, work / "live")
        self.assertEqual(tree_state(work / "live"), tree_state(files))
        # The existing API export helper compares redacted source/restored documents.
        self.assertTrue(soak.compare_exports({"f": self.export("c" * 64)}, {"f": self.export("c" * 64)})["matched"])
        self.assertFalse(soak.compare_exports({"f": self.export("c" * 64)}, {"f": self.export("d" * 64)})["matched"])


if __name__ == "__main__":
    unittest.main()
