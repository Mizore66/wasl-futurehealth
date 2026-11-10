#!/usr/bin/env python3
"""File-backup, integrity, migration, and rollback tests; no container daemon required."""
import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))

from recovery import (  # noqa: E402
    _canonical,
    _decrypt_v2_payloads,
    _manifest_mac,
    migrate_files,
    rollback_files,
    verify_bundle,
    write_file_bundle,
)
from recovery_crypto import load_keyring, new_keyring  # noqa: E402

SCRIPTS = Path(__file__).resolve().parent


def make_keyring(path: Path) -> dict:
    keyring = new_keyring(path)
    os.chmod(path, 0o444)
    return keyring


def flip_last_byte(path: Path) -> None:
    data = path.read_bytes()
    path.write_bytes(data[:-1] + bytes([data[-1] ^ 1]))


def make_source_with_nats(root: Path):
    source = root / "source"
    (source / "run").mkdir(parents=True)
    (source / "run/audit.json").write_text('{"synthetic":true}\n')
    nats = root / "nats.json"
    nats.write_text('{"streams":[],"marker":"synthetic clinical marker"}\n')
    return source, nats


def write_v1_bundle(source: Path, bundle: Path, key_file: Path) -> dict:
    """Test-only legacy fixture; production has no legacy backup mode."""
    key = Path(key_file).read_bytes()
    bundle.mkdir(parents=True, exist_ok=True)
    records = []
    for path in sorted(p for p in Path(source).rglob("*") if p.is_file()):
        relative = path.relative_to(source)
        target = bundle / "files" / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        data = path.read_bytes()
        target.write_bytes(data)
        records.append({"path": relative.as_posix(), "sha256": hashlib.sha256(data).hexdigest(), "size": len(data)})
    manifest = {"schema": "wasl-file-backup-v1", "created_at": "2026-01-01T00:00:00+00:00",
                "nondeterministic_fields": [], "files": records}
    manifest["hmac_sha256"] = _manifest_mac(manifest, key)
    (bundle / "manifest.json").write_bytes(_canonical(manifest) + b"\n")
    return manifest


class RecoveryFileTests(unittest.TestCase):
    def test_nats_payload_is_inside_authenticated_bundle(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "state").write_text("run")
            nats = root / "nats.json"
            nats.write_text('{"streams":[{"digest":"retained"}]}')
            key = root / "recovery_keys.json"
            make_keyring(key)
            bundle = root / "bundle"
            write_file_bundle(source, bundle, key, nats_json=nats)
            manifest = verify_bundle(bundle, key)
            self.assertIn("nats/nats.json", {r["path"] for r in manifest["files"]})
            flip_last_byte(bundle / "files" / "nats" / "nats.json.aead")
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                verify_bundle(bundle, key)

    def test_tampered_bundle_and_wrong_key_fail_before_destination_changes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "run.ndjson").write_text('{"decision":"held"}\n')
            key = root / "recovery_keys.json"
            make_keyring(key)
            bundle = root / "bundle"
            write_file_bundle(source, bundle, key)
            destination = root / "destination"
            destination.mkdir()
            (destination / "do-not-touch").write_text("unrelated")

            flip_last_byte(bundle / "files" / "run.ndjson.aead")
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                verify_bundle(bundle, key)
            self.assertEqual((destination / "do-not-touch").read_text(), "unrelated")
            with self.assertRaises(ValueError):
                verify_bundle(bundle, root / "missing.key")
            self.assertEqual((destination / "do-not-touch").read_text(), "unrelated")

    def test_restore_command_verifies_before_contacting_nats_or_creating_run_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "source"
            source.mkdir()
            (source / "state").write_text("persisted")
            key = root / "recovery_keys.json"
            make_keyring(key)
            nats = root / "nats.json"
            nats.write_text('{"schema":"wasl-nats-backup-v1","streams":[]}')
            backups = root / "backups"
            backups.mkdir()
            write_file_bundle(source, backups / "bundle", key, nats_json=nats)
            flip_last_byte(backups / "bundle" / "files" / "state.aead")
            bin_dir = root / "bin"
            bin_dir.mkdir()
            calls = root / "calls"
            wsi = bin_dir / "wsi7"
            wsi.write_text(f'#!/bin/sh\necho "$@" >> "{calls}"\n')
            wsi.chmod(0o755)
            destination = root / "destination"
            shim = bin_dir / "python3"  # the interpreter that has `cryptography`
            shim.write_text(f'#!/bin/sh\nexec "{sys.executable}" "$@"\n')
            shim.chmod(0o755)
            env = dict(os.environ, PATH=f"{bin_dir}{os.pathsep}{os.environ['PATH']}",
                       WASL_BIN=str(bin_dir), WASL_OUT=str(destination),
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
            write_v1_bundle(old, old_bundle, key)
            live = root / "live"
            migrate_files(old_bundle, key, live, legacy_hmac=True)
            self.assertEqual((live / "generation.txt").read_text(), "old\n")

            new = root / "new"
            new.mkdir()
            (new / "generation.txt").write_text("new\n")
            (new / "added.ndjson").write_text("retained message\n")
            new_bundle = root / "new-bundle"
            write_v1_bundle(new, new_bundle, key)
            journal = migrate_files(new_bundle, key, live, replace=True, legacy_hmac=True)
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
            write_v1_bundle(source, bundle, key)
            live = root / "live"
            live.mkdir()
            (live / "state").write_text("current")
            with self.assertRaisesRegex(ValueError, "not empty"):
                migrate_files(bundle, key, live, legacy_hmac=True)
            self.assertEqual((live / "state").read_text(), "current")


class RecoveryV2Tests(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)
        self.keyring = self.root / "recovery_keys.json"
        make_keyring(self.keyring)

    def backup(self, name="bundle"):
        source, nats = self.root / "source", self.root / "nats.json"
        if not source.exists():
            source, nats = make_source_with_nats(self.root)
        bundle = self.root / name
        return bundle, write_file_bundle(source, bundle, self.keyring, nats_json=nats)

    def resign(self, bundle, mutate):
        """Mutate the manifest and re-authenticate it, to reach the checks behind the HMAC."""
        manifest = json.loads((bundle / "manifest.json").read_text())
        mutate(manifest)
        manifest.pop("hmac_sha256", None)
        doc, _, _ = load_keyring(self.keyring)
        auth = base64.b64decode(doc["keys"][manifest["key_id"]]["auth_key"])
        manifest["hmac_sha256"] = _manifest_mac(manifest, auth)
        (bundle / "manifest.json").write_bytes(_canonical(manifest) + b"\n")

    @staticmethod
    def record(manifest, path):
        return next(r for r in manifest["files"] if r["path"] == path)

    def staging(self, name):
        path = self.root / name
        path.mkdir(mode=0o700)
        os.chmod(path, 0o700)
        return path

    def test_v2_payloads_are_encrypted_and_authenticated(self):
        source, nats = make_source_with_nats(self.root)
        bundle = self.root / "bundle"
        manifest = write_file_bundle(source, bundle, self.keyring, nats_json=nats)
        self.assertEqual(manifest["schema"], "wasl-file-backup-v2-aesgcm")
        raw = b"".join(path.read_bytes() for path in bundle.rglob("*") if path.is_file())
        self.assertNotIn(b"synthetic", raw)
        _, active, entry = load_keyring(self.keyring)
        for value in entry.values():
            self.assertNotIn(value.encode(), raw)
            self.assertNotIn(base64.b64decode(value), raw)
        self.assertEqual(manifest["key_id"], active)
        self.assertEqual({r["path"] for r in manifest["files"]}, {"run/audit.json", "nats/nats.json"})
        self.assertEqual(verify_bundle(bundle, self.keyring)["key_id"], manifest["key_id"])
        for path in (bundle / "files").rglob("*.aead"):
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
        ciphertext = bundle / "files/nats/nats.json.aead"
        with ciphertext.open("r+b") as handle:
            handle.truncate(3)
        with self.assertRaisesRegex(ValueError, "decrypt|authentication|truncated"):
            verify_bundle(bundle, self.keyring)

    def test_equal_sources_get_fresh_dek_and_nonces(self):
        one, m1 = self.backup("one")
        two, m2 = self.backup("two")
        self.assertNotEqual(m1["wrapped_dek"], m2["wrapped_dek"])
        self.assertNotEqual(m1["wrap_nonce"], m2["wrap_nonce"])
        nonces = [r["nonce"] for r in m1["files"] + m2["files"]]
        self.assertEqual(len(nonces), len(set(nonces)))
        self.assertNotEqual((one / "files/run/audit.json.aead").read_bytes(),
                            (two / "files/run/audit.json.aead").read_bytes())

    def test_wrong_missing_or_raw_keys_are_refused(self):
        bundle, _ = self.backup()
        other = self.root / "other.json"
        make_keyring(other)
        raw = self.root / "backup_key"
        raw.write_bytes(b"k" * 32)
        for key in (other, self.root / "absent.json", raw):
            with self.assertRaises(ValueError):
                verify_bundle(bundle, key)
            if key is not other:  # a different valid keyring may legitimately create its own bundles
                with self.assertRaises(ValueError):
                    write_file_bundle(self.root / "source", self.root / "again", key)
                self.assertFalse((self.root / "again").exists())

    def test_unauthenticated_manifest_tamper_is_refused(self):
        bundle, _ = self.backup()
        original = (bundle / "manifest.json").read_text()
        for field in ("key_id", "wrapped_dek", "wrap_nonce", "created_at"):
            manifest = json.loads(original)
            manifest[field] = manifest[field][:-2] + ("AA" if not manifest[field].endswith("AA") else "BB")
            (bundle / "manifest.json").write_text(json.dumps(manifest))
            with self.assertRaises(ValueError):
                verify_bundle(bundle, self.keyring)
        for mutate in (lambda m: m["files"][0].__setitem__("path", "run/other.json"),
                       lambda m: m["files"][0].__setitem__("size", m["files"][0]["size"] + 1),
                       lambda m: m["files"][0].__setitem__("sha256", "0" * 64),
                       lambda m: m["files"][0].__setitem__("nonce", m["files"][1]["nonce"]),
                       lambda m: m["files"][0].__setitem__("ciphertext_sha256", "0" * 64),
                       lambda m: m.__setitem__("extra", 1),
                       lambda m: m["files"].pop(),
                       lambda m: m.pop("hmac_sha256")):
            manifest = json.loads(original)
            mutate(manifest)
            (bundle / "manifest.json").write_text(json.dumps(manifest))
            with self.assertRaisesRegex(ValueError, "authentication|malformed"):
                verify_bundle(bundle, self.keyring)
        (bundle / "manifest.json").write_text(original.replace('"schema"', '"schema":"x","schema"', 1))
        with self.assertRaises(ValueError):
            verify_bundle(bundle, self.keyring)

    def test_unknown_key_id_is_refused(self):
        bundle, _ = self.backup()
        manifest = json.loads((bundle / "manifest.json").read_text())
        manifest["key_id"] = "g-unknown"
        (bundle / "manifest.json").write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, "key"):
            verify_bundle(bundle, self.keyring)

    def test_authenticated_manifest_with_bad_content_is_refused(self):
        def field(name, value):
            return lambda m: m["files"][0].__setitem__(name, value)
        cases = (
            ("traversal", field("path", "../escape"), "unsafe|duplicate"),
            ("absolute", field("path", "/etc/passwd"), "unsafe|duplicate"),
            ("dot", field("path", "run/./audit.json"), "unsafe|duplicate"),
            ("empty-part", field("path", "run//audit.json"), "unsafe|duplicate"),
            ("backslash", field("path", "run\\audit.json"), "unsafe|duplicate"),
            ("duplicate", lambda m: m["files"][0].__setitem__("path", m["files"][1]["path"]), "unsafe|duplicate"),
            ("file-dir-clash", lambda m: m["files"][0].__setitem__("path", m["files"][1]["path"] + "/x"),
             "unsafe|duplicate"),
            ("extra-field", field("owner", 1), "malformed"),
            ("bool-size", field("size", True), "malformed"),
            ("negative-size", field("size", -1), "malformed"),
            ("short-nonce", field("nonce", "AAAA"), "malformed"),
            ("upper-hash", field("sha256", "A" * 64), "malformed"),
            ("schema", lambda m: m.__setitem__("schema", "wasl-file-backup-v3"), "unsupported|schema"),
            ("files-not-list", lambda m: m.__setitem__("files", {}), "malformed"),
            ("bad-wrapped-dek", lambda m: m.__setitem__("wrapped_dek", "AAAA"), "malformed"),
        )
        for label, mutate, pattern in cases:
            with self.subTest(label):
                bundle, _ = self.backup(f"b-{label}")
                self.resign(bundle, mutate)
                with self.assertRaisesRegex(ValueError, pattern):
                    verify_bundle(bundle, self.keyring)

    def test_authenticated_metadata_that_disagrees_with_plaintext_is_refused(self):
        for label, mutate in (("size", lambda r: r.__setitem__("size", r["size"] + 1)),
                              ("sha256", lambda r: r.__setitem__("sha256", "0" * 64))):
            with self.subTest(label):
                bundle, _ = self.backup(f"d-{label}")
                self.resign(bundle, lambda m: mutate(self.record(m, "run/audit.json")))
                with self.assertRaisesRegex(ValueError, "decrypt|authentication|truncated|size|hash"):
                    verify_bundle(bundle, self.keyring)

    def test_ciphertext_tamper_truncation_swap_and_file_set(self):
        bundle, _ = self.backup()
        cipher = bundle / "files/run/audit.json.aead"
        good = cipher.read_bytes()
        flipped = bytes([good[0] ^ 1]) + good[1:]
        cipher.write_bytes(flipped)
        with self.assertRaisesRegex(ValueError, "hash mismatch"):
            verify_bundle(bundle, self.keyring)
        # with an authenticated ciphertext hash, GCM itself must reject the flipped bit
        self.resign(bundle, lambda m: self.record(m, "run/audit.json").__setitem__(
            "ciphertext_sha256", hashlib.sha256(flipped).hexdigest()))
        with self.assertRaisesRegex(ValueError, "decrypt|authentication"):
            verify_bundle(bundle, self.keyring)
        cipher.write_bytes(good[:-4])
        self.resign(bundle, lambda m: self.record(m, "run/audit.json").__setitem__(
            "ciphertext_sha256", hashlib.sha256(good[:-4]).hexdigest()))
        with self.assertRaisesRegex(ValueError, "truncated|size"):
            verify_bundle(bundle, self.keyring)
        # authenticated nonce swap between files breaks decryption
        fresh, _ = self.backup("fresh")

        def swap(m):
            a, b = m["files"][0], m["files"][1]
            a["nonce"], b["nonce"] = b["nonce"], a["nonce"]
        self.resign(fresh, swap)
        with self.assertRaisesRegex(ValueError, "decrypt|authentication"):
            verify_bundle(fresh, self.keyring)
        # swapping two ciphertext files (hashes fixed up) fails because AAD binds path/size/hash
        swapped, _ = self.backup("swapped")
        a, b = swapped / "files/run/audit.json.aead", swapped / "files/nats/nats.json.aead"
        da, db = a.read_bytes(), b.read_bytes()
        a.write_bytes(db)
        b.write_bytes(da)

        def fix(m):
            ra, rb = self.record(m, "run/audit.json"), self.record(m, "nats/nats.json")
            ra["nonce"], rb["nonce"] = rb["nonce"], ra["nonce"]
            ra["ciphertext_sha256"] = hashlib.sha256(db).hexdigest()
            rb["ciphertext_sha256"] = hashlib.sha256(da).hexdigest()
        self.resign(swapped, fix)
        with self.assertRaises(ValueError):
            verify_bundle(swapped, self.keyring)
        for label, action in (
            ("extra", lambda b: (b / "files/run/extra.aead").write_bytes(b"x")),
            ("extra-dir-file", lambda b: (b / "files/stray").write_bytes(b"x")),
            ("symlink", lambda b: (b / "files/run/link.aead").symlink_to(b / "manifest.json")),
            ("symlink-dir", lambda b: (b / "files/linkdir").symlink_to(b / "files/run")),
            ("missing", lambda b: (b / "files/run/audit.json.aead").unlink()),
            ("root-extra", lambda b: (b / "plaintext.txt").write_text("leak")),
        ):
            with self.subTest(label):
                broken, _ = self.backup(f"c-{label}")
                action(broken)
                with self.assertRaises(ValueError):
                    verify_bundle(broken, self.keyring)

    def test_legacy_hmac_requires_explicit_opt_in(self):
        source, _ = make_source_with_nats(self.root)
        key = self.root / "backup_key"
        key.write_bytes(b"l" * 32)
        legacy = self.root / "legacy"
        write_v1_bundle(source, legacy, key)
        for candidate in (key, self.keyring):
            with self.assertRaisesRegex(ValueError, "legacy"):
                verify_bundle(legacy, candidate)
        with self.assertRaisesRegex(ValueError, "legacy"):
            migrate_files(legacy, key, self.root / "live")
        self.assertFalse((self.root / "live").exists())
        self.assertEqual(verify_bundle(legacy, key, legacy_hmac=True)["schema"], "wasl-file-backup-v1")
        with self.assertRaises(ValueError):
            verify_bundle(legacy, self.keyring, legacy_hmac=True)
        v2, _ = self.backup()
        with self.assertRaisesRegex(ValueError, "legacy|schema"):
            verify_bundle(v2, key, legacy_hmac=True)

    def test_legacy_flag_on_cli_verify(self):
        source, _ = make_source_with_nats(self.root)
        key = self.root / "backup_key"
        key.write_bytes(b"l" * 32)
        legacy = self.root / "legacy"
        write_v1_bundle(source, legacy, key)
        script = str(SCRIPTS / "recovery.py")
        plain = subprocess.run([sys.executable, script, "verify", str(legacy), "--key-file", str(key)],
                               text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        self.assertNotEqual(plain.returncode, 0)
        opted = subprocess.run([sys.executable, script, "verify", str(legacy), "--key-file", str(key), "--legacy-hmac"],
                               text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        self.assertEqual(opted.returncode, 0, opted.stdout)

    def test_decrypt_into_empty_private_staging(self):
        bundle, _ = self.backup()
        doc, _, _ = load_keyring(self.keyring)
        manifest = verify_bundle(bundle, self.keyring)
        staging = self.staging("staging")
        _decrypt_v2_payloads(bundle, doc, manifest, staging)
        self.assertEqual((staging / "run/audit.json").read_text(), '{"synthetic":true}\n')
        self.assertIn(b"synthetic clinical marker", (staging / "nats/nats.json").read_bytes())
        recorded = (self.root / "source/run/audit.json").stat().st_mode & 0o777
        self.assertEqual(stat.S_IMODE((staging / "run/audit.json").stat().st_mode), recorded)
        self.assertEqual(stat.S_IMODE((staging / "run").stat().st_mode), (self.root / "source/run").stat().st_mode & 0o777)
        with self.assertRaisesRegex(ValueError, "empty"):
            _decrypt_v2_payloads(bundle, doc, manifest, staging)
        loose = self.root / "loose"
        loose.mkdir()
        os.chmod(loose, 0o755)
        with self.assertRaisesRegex(ValueError, "0700"):
            _decrypt_v2_payloads(bundle, doc, manifest, loose)
        self.assertEqual(list(loose.iterdir()), [])
        with self.assertRaises(ValueError):
            _decrypt_v2_payloads(bundle, doc, manifest, self.root / "absent")
        link = self.root / "link"
        link.symlink_to(self.staging("target"))
        with self.assertRaises(ValueError):
            _decrypt_v2_payloads(bundle, doc, manifest, link)

    def test_decrypt_failure_and_forged_manifest_leave_staging_empty(self):
        bundle, _ = self.backup()
        doc, _, _ = load_keyring(self.keyring)
        manifest = verify_bundle(bundle, self.keyring)
        forged = json.loads(json.dumps(manifest))
        forged["files"][0]["path"] = "evil"
        clean = self.staging("clean")
        with self.assertRaises(ValueError):
            _decrypt_v2_payloads(bundle, doc, forged, clean)
        self.assertEqual(list(clean.iterdir()), [])
        # the last file is corrupted, so plaintext already written for earlier files must be removed again
        names = sorted(r["path"] for r in manifest["files"])
        cipher = bundle / "files" / (names[-1] + ".aead")
        flip_last_byte(cipher)
        again = self.staging("again")
        with self.assertRaises(ValueError):
            _decrypt_v2_payloads(bundle, doc, manifest, again)
        self.assertEqual(list(again.iterdir()), [])

    def test_rotate_key_cli_prints_only_new_id_and_keeps_old_bundles_readable(self):
        old_bundle, old_manifest = self.backup("old")
        result = subprocess.run([sys.executable, str(SCRIPTS / "recovery.py"), "rotate-key", "--key-file",
                                 str(self.keyring)], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(result.returncode, 0, result.stderr)
        doc, active, _ = load_keyring(self.keyring)
        self.assertEqual(result.stdout, active + "\n")
        self.assertEqual(result.stderr, "")
        self.assertNotEqual(active, old_manifest["key_id"])
        self.assertIn(old_manifest["key_id"], doc["keys"])
        new_bundle, new_manifest = self.backup("new")
        self.assertEqual(new_manifest["key_id"], active)
        self.assertEqual(verify_bundle(old_bundle, self.keyring)["key_id"], old_manifest["key_id"])
        self.assertEqual(verify_bundle(new_bundle, self.keyring)["key_id"], active)

    def test_cli_backup_and_verify_roundtrip_logs_no_secrets(self):
        source, nats = make_source_with_nats(self.root)
        bundle = self.root / "cli-bundle"
        script = str(SCRIPTS / "recovery.py")
        outputs = []
        for args in (["backup", str(source), str(bundle), "--key-file", str(self.keyring), "--nats-json", str(nats)],
                     ["verify", str(bundle), "--key-file", str(self.keyring)],
                     ["verify", str(bundle), "--key-file", str(self.root / "absent.json")]):
            result = subprocess.run([sys.executable, script, *args], text=True, stdout=subprocess.PIPE,
                                    stderr=subprocess.STDOUT)
            outputs.append((result.returncode, result.stdout))
        self.assertEqual([code == 0 for code, _ in outputs], [True, True, False])
        _, _, entry = load_keyring(self.keyring)
        for _, text in outputs:
            for value in entry.values():
                self.assertNotIn(value, text)
            self.assertNotIn("synthetic", text)


if __name__ == "__main__":
    unittest.main()
