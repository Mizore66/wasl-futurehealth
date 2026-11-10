#!/usr/bin/env python3
"""File-backup, integrity, migration, and rollback tests; no container daemon required."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from recovery import migrate_files, rollback_files, verify_bundle, write_file_bundle


class RecoveryFileTests(unittest.TestCase):
    def test_nats_payload_is_inside_authenticated_bundle(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "state").write_text("run")
            nats = root / "nats.json"
            nats.write_text('{"streams":[{"digest":"retained"}]}')
            key = root / "backup.key"
            key.write_bytes(b"d" * 32)
            bundle = root / "bundle"
            write_file_bundle(source, bundle, key, nats_json=nats)
            manifest = verify_bundle(bundle, key)
            self.assertIn("nats/nats.json", {r["path"] for r in manifest["files"]})
            (bundle / "files" / "nats" / "nats.json").write_text('{"streams":[]}')
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                verify_bundle(bundle, key)

    def test_tampered_bundle_and_wrong_key_fail_before_destination_changes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "run.ndjson").write_text('{"decision":"held"}\n')
            key = root / "backup.key"
            key.write_bytes(b"a" * 32)
            bundle = root / "bundle"
            write_file_bundle(source, bundle, key)
            destination = root / "destination"
            destination.mkdir()
            (destination / "do-not-touch").write_text("unrelated")

            payload = bundle / "files" / "run.ndjson"
            payload.write_text('{"decision":"forged"}\n')
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                verify_bundle(bundle, key)
            self.assertEqual((destination / "do-not-touch").read_text(), "unrelated")
            with self.assertRaisesRegex(ValueError, "authentication"):
                verify_bundle(bundle, root / "missing.key")
            self.assertEqual((destination / "do-not-touch").read_text(), "unrelated")

    def test_restore_command_verifies_before_contacting_nats_or_creating_run_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "state").write_text("persisted")
            key = root / "backup.key"
            key.write_bytes(b"e" * 32)
            nats = root / "nats.json"
            nats.write_text('{"schema":"wasl-nats-backup-v1","streams":[]}')
            backups = root / "backups"
            backups.mkdir()
            write_file_bundle(source, backups / "bundle", key, nats_json=nats)
            (backups / "bundle" / "files" / "state").write_text("tampered")
            bin_dir = root / "bin"
            bin_dir.mkdir()
            calls = root / "calls"
            wsi = bin_dir / "wsi7"
            wsi.write_text(f'#!/bin/sh\necho "$@" >> "{calls}"\n')
            wsi.chmod(0o755)
            destination = root / "destination"
            env = dict(os.environ, WASL_BIN=str(bin_dir), WASL_OUT=str(destination),
                       WASL_RECOVERY_DIR=str(backups), WASL_RECOVERY_KEY_FILE=str(key),
                       WASL_NATS_URL="nats://invalid:4222", WASL_NATS_EXTERNAL="1")
            result = subprocess.run([str(Path(__file__).resolve().parent / "demo.sh"), "recovery-restore"], env=env,
                                    text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertNotEqual(result.returncode, 0, result.stdout)
            self.assertIn("hash mismatch", result.stdout)
            self.assertFalse(calls.exists(), "tampered bundle reached NATS restore")
            self.assertFalse(destination.exists(), "tampered bundle created destination files")

    def test_https_api_probe_rejects_anonymous_and_records_stable_hash(self):
        secrets = os.environ.get("WASL_TEST_API_SECRETS")
        base_url = os.environ.get("WASL_TEST_API_URL")
        if not secrets or not base_url:
            self.skipTest("set WASL_TEST_API_SECRETS and WASL_TEST_API_URL for the real HTTPS API drill")
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "probe.json"
            result = subprocess.run([str(Path(__file__).resolve().parent / "recovery_api_probe.py"),
                                     "--secrets", secrets, "--base-url", base_url, "--output", str(output)],
                                    text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            self.assertEqual(result.returncode, 0, result.stdout)
            probe = json.loads(output.read_text())
            self.assertEqual(probe["unauthenticated_status"], 401)
            self.assertEqual(len(probe["comparison_sha256"]), 64)
            self.assertIn("meta.loaded_at", probe["nondeterministic_fields"])

    def test_file_migration_can_roll_back_to_the_previous_generation(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            old = root / "old"
            old.mkdir()
            (old / "generation.txt").write_text("old\n")
            key = root / "backup.key"
            key.write_bytes(b"b" * 32)
            old_bundle = root / "old-bundle"
            write_file_bundle(old, old_bundle, key)
            live = root / "live"
            migrate_files(old_bundle, key, live)
            self.assertEqual((live / "generation.txt").read_text(), "old\n")

            new = root / "new"
            new.mkdir()
            (new / "generation.txt").write_text("new\n")
            (new / "added.ndjson").write_text("retained message\n")
            new_bundle = root / "new-bundle"
            write_file_bundle(new, new_bundle, key)
            journal = migrate_files(new_bundle, key, live, replace=True)
            self.assertEqual((live / "generation.txt").read_text(), "new\n")
            self.assertTrue(Path(journal["rollback_path"]).is_dir())

            rollback_files(journal)
            self.assertEqual((live / "generation.txt").read_text(), "old\n")
            self.assertFalse((live / "added.ndjson").exists())

    def test_restore_requires_an_empty_destination_unless_replacement_is_explicit(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "state").write_text("persisted")
            key = root / "backup.key"
            key.write_bytes(b"c" * 32)
            bundle = root / "bundle"
            write_file_bundle(source, bundle, key)
            live = root / "live"
            live.mkdir()
            (live / "state").write_text("current")
            with self.assertRaisesRegex(ValueError, "not empty"):
                migrate_files(bundle, key, live)
            self.assertEqual((live / "state").read_text(), "current")


if __name__ == "__main__":
    unittest.main()
