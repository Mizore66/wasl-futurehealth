#!/usr/bin/env python3
"""Synthetic generation-catalog, verified-good evidence, and safe-retention tests; no containers required."""
from datetime import datetime, timedelta, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPTS))

from recovery import write_file_bundle  # noqa: E402
from recovery_catalog import (  # noqa: E402
    EVIDENCE_SCHEMA,
    INTENT_SCHEMA,
    Catalog,
    now_iso,
    sha256,
    write_exclusive,
)
from recovery_crypto import new_keyring  # noqa: E402
from test_recovery import flip_last_byte  # noqa: E402


class CatalogTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root, True)
        self.backup = self.root / "backup"
        self.state = self.root / "evidence"
        self.backup.mkdir()
        self.state.mkdir()
        self.keys = self.root / "recovery_keys.json"
        new_keyring(self.keys)
        self.catalog = Catalog(self.backup, self.state, self.keys)
        self.catalog.init()

    def seal(self, run_dir: Path):
        records = []
        for path in sorted(run_dir.rglob("*")):
            if path.is_file() and path.name != "evidence-manifest.json":
                records.append({"path": path.relative_to(run_dir).as_posix(), "size": path.stat().st_size,
                                "sha256": sha256(path)})
        write_exclusive(run_dir / "evidence-manifest.json",
                        {"schema": EVIDENCE_SCHEMA, "created_at": now_iso(), "files": records})

    def attempt(self, attempt_id: str, kind: str, generation: str | None, status: str = "success",
                comparison: bool = False):
        run_dir = self.state / "runs" / attempt_id
        run_dir.mkdir()
        write_exclusive(run_dir / "started.json", {
            "schema": "wasl-recovery-attempt-v1", "attempt_id": attempt_id, "kind": kind,
            "generation": generation, "source_revision": "a" * 40, "started_at": now_iso()})
        if comparison:
            write_exclusive(run_dir / "hash-comparison.json", {"ok": True})
        write_exclusive(run_dir / "result.json", {"attempt_id": attempt_id, "status": status,
                                                   "finished_at": now_iso(), "error": None})
        self.seal(run_dir)
        return run_dir

    def generation(self, number: int, publish: bool = False) -> str:
        generation = f"gen-{number:03d}-fixture"
        self.catalog.prepare(generation)
        source = self.root / f"source-{number}"
        source.mkdir()
        (source / "synthetic.txt").write_text(f"synthetic generation {number}\n")
        (source / "nested").mkdir()
        (source / "nested" / "inner.txt").write_text(f"nested synthetic generation {number}\n")
        nats = self.root / f"nats-{number}.json"
        nats.write_text('{"schema":"wasl-nats-backup-v1","streams":[]}\n')
        write_file_bundle(source, self.catalog.bundle(generation), self.keys, nats_json=nats)
        revision = f"{number:040x}"
        self.attempt(f"backup-{number:03d}", "backup", generation)
        record = self.catalog.register(generation, str(number), revision, f"backup-{number:03d}")
        self.assertEqual(record["generation"], generation)
        if publish:
            self.catalog.publish(generation)
        return generation

    def verify_good(self, generation: str, number: int):
        attempt_id = f"verify-{number:03d}"
        self.attempt(attempt_id, "verify", generation, comparison=True)
        return self.catalog.mark_good(generation, attempt_id)

    def retention_intent(self, generation: str):
        manifest_path = self.catalog.bundle(generation) / "manifest.json"
        manifest = json.loads(manifest_path.read_text())
        digest = hashlib.sha256(manifest_path.read_bytes()).hexdigest()
        directory = self.state / "catalog" / generation
        write_exclusive(directory / "retention-intent.json", {
            "schema": INTENT_SCHEMA, "generation": generation, "manifest_sha256": digest,
            "started_at": now_iso(), "ciphertext_paths": [record["path"] + ".aead" for record in manifest["files"]]})
        return manifest

    def test_register_export_and_atomic_namespace_publication(self):
        generation = self.generation(1)
        manifest_path = self.catalog.bundle(generation) / "manifest.json"
        before = manifest_path.read_bytes()
        output = self.state / "exported.json"
        exported = self.catalog.export_manifest(generation, output)
        self.assertEqual(sha256(output), exported["manifest_sha256"])
        self.catalog.publish(generation)
        self.assertEqual(os.readlink(self.backup / "bundle"),
                         f"scheduler-v1/generations/{generation}/bundle")
        self.assertEqual(self.catalog.resolve("latest")["generation"], generation)
        self.assertEqual(manifest_path.read_bytes(), before, "publication must not modify a completed generation")
        with self.assertRaises(ValueError):
            self.catalog.publish(generation)

    def test_publication_refuses_ordinary_file_directory_and_external_symlink(self):
        generation = self.generation(1)
        for kind in ("file", "directory", "external-link"):
            with self.subTest(kind):
                published = self.backup / "bundle"
                if published.is_symlink() or published.is_file():
                    published.unlink()
                elif published.is_dir():
                    published.rmdir()
                if kind == "file":
                    published.write_text("do not replace")
                elif kind == "directory":
                    published.mkdir()
                else:
                    published.symlink_to("../outside")
                with self.assertRaises(ValueError):
                    self.catalog.publish(generation)
                if kind == "file":
                    self.assertEqual(published.read_text(), "do not replace")
                elif kind == "directory":
                    self.assertTrue(published.is_dir())
                else:
                    self.assertEqual(os.readlink(published), "../outside")

    def test_wrong_key_manifest_and_ciphertext_tamper_fail_closed(self):
        generation = self.generation(1)
        wrong = self.root / "wrong.json"
        new_keyring(wrong)
        with self.assertRaises(ValueError):
            Catalog(self.backup, self.state, wrong).resolve("latest" if self.catalog._published_target() else generation)
        original = (self.catalog.bundle(generation) / "manifest.json").read_bytes()
        (self.catalog.bundle(generation) / "manifest.json").write_bytes(original[:-2] + b"xx")
        with self.assertRaises(ValueError):
            self.catalog.authenticate(generation)
        (self.catalog.bundle(generation) / "manifest.json").write_bytes(original)
        ciphertext = self.catalog.bundle(generation) / "files/synthetic.txt.aead"
        flip_last_byte(ciphertext)
        with self.assertRaises(ValueError):
            self.catalog.authenticate(generation)
        self.assertTrue(ciphertext.exists(), "failed authentication must preserve the failed bundle")

    def test_selection_rejects_modified_backup_evidence(self):
        self.generation(2, publish=True)
        backup_run = self.state / "runs" / "backup-002"
        (backup_run / "started.json").write_text('{"attempt_id":"backup-002","kind":"backup","generation":"other"}\n')
        with self.assertRaises(ValueError):
            self.catalog.resolve("latest")

    def test_only_complete_sealed_successful_verification_can_be_known_good(self):
        generation = self.generation(1)
        attempt_id = "verify-incomplete"
        run_dir = self.state / "runs" / attempt_id
        run_dir.mkdir()
        write_exclusive(run_dir / "started.json", {"attempt_id": attempt_id, "kind": "verify",
                                                    "generation": generation, "source_revision": "a" * 40})
        write_exclusive(run_dir / "result.json", {"attempt_id": attempt_id, "status": "success",
                                                   "finished_at": now_iso()})
        write_exclusive(run_dir / "hash-comparison.json", {"ok": True})
        with self.assertRaises(ValueError):
            self.catalog.mark_good(generation, attempt_id)
        self.assertFalse((self.state / "catalog" / generation / "verified-good.json").exists())
        self.seal(run_dir)
        good = self.catalog.mark_good(generation, attempt_id)
        self.assertEqual(good["generation"], generation)
        repeated = self.catalog.mark_good(generation, attempt_id)
        self.assertEqual(repeated, good)

    def test_automatic_latest_selection_is_good_and_repeat_verification_is_idempotent(self):
        generation = self.generation(2, publish=True)
        first_attempt = "verify-0-aaaaaaaaaaaaaaaa"
        for attempt_id in (first_attempt, "verify-1-bbbbbbbbbbbbbbbb"):
            run_dir = self.attempt(attempt_id, "verify", None, comparison=True)
            started = json.loads((run_dir / "started.json").read_text())
            started.update({"automatic": True, "slot": "0" if attempt_id == first_attempt else "1"})
            (run_dir / "started.json").write_text(json.dumps(started) + "\n")
            write_exclusive(run_dir / "selection.json", {
                "generation": generation,
                "bundle": str(self.catalog.bundle(generation)),
                "record": self.catalog.record(generation),
            })
            # Reseal after adding the automatic-latest selection evidence.
            (run_dir / "evidence-manifest.json").unlink()
            self.seal(run_dir)
            good = self.catalog.mark_good(generation, attempt_id)
            self.assertEqual(good["generation"], generation)
            self.assertEqual(good["manifest_sha256"], self.catalog.record(generation)["manifest_sha256"])
        self.assertEqual(self.catalog.good_record(generation)["attempt_id"], first_attempt)

    def test_retention_dry_run_then_apply_prunes_only_eligible_ciphertext(self):
        generations = [self.generation(number, publish=number == 3) for number in (1, 2, 3)]
        for number, generation in enumerate(generations, 1):
            self.verify_good(generation, number)
        future = datetime.now(timezone.utc) + timedelta(days=40)
        review = self.catalog.retention(30, 2, 2, future, apply=False)
        self.assertEqual(review["status"], "dry-run")
        self.assertEqual([item["generation"] for item in review["candidates"]], [generations[0]])
        self.assertFalse((self.state / "catalog" / generations[0] / "retention-intent.json").exists())
        applied = self.catalog.retention(30, 2, 2, future, apply=True)
        self.assertEqual(applied["pruned"][0]["generation"], generations[0])
        self.assertFalse((self.catalog.bundle(generations[0]) / "files").exists())
        self.assertTrue((self.catalog.bundle(generations[0]) / "manifest.json").is_file())
        self.assertTrue((self.state / "catalog" / generations[0] / "verified-good.json").is_file())
        for generation in generations[1:]:
            self.assertTrue((self.catalog.bundle(generation) / "files").is_dir())
        self.assertEqual(os.readlink(self.backup / "bundle"),
                         f"scheduler-v1/generations/{generations[-1]}/bundle")

    def test_only_known_good_or_too_few_good_blocks_every_prune(self):
        generation = self.generation(1, publish=True)
        self.verify_good(generation, 1)
        future = datetime.now(timezone.utc) + timedelta(days=100)
        review = self.catalog.retention(30, 2, 2, future, apply=True)
        self.assertEqual(review["status"], "blocked")
        self.assertTrue((self.catalog.bundle(generation) / "files").is_dir())
        self.assertFalse((self.state / "catalog" / generation / "retention-intent.json").exists())

    def test_failed_interrupted_unknown_and_corrupted_evidence_block_all_pruning(self):
        generation = self.generation(10, publish=True)
        self.verify_good(generation, 10)
        for label, status in (("failed", "failed"), ("interrupted", "interrupted"), ("unknown", "other")):
            with self.subTest(label):
                bad = self.attempt(f"bad-{label}", "verify", generation, status=status)
                future = datetime.now(timezone.utc) + timedelta(days=100)
                review = self.catalog.retention(30, 1, 1, future, apply=True)
                self.assertEqual(review["status"], "blocked")
                self.assertEqual(review["candidates"], [])
                self.assertTrue((self.catalog.bundle(generation) / "files").is_dir())
                shutil.rmtree(bad)

    def test_corrupted_sealed_evidence_blocks_pruning(self):
        generation = self.generation(20, publish=True)
        self.verify_good(generation, 20)
        run_dir = self.state / "runs" / "verify-020"
        (run_dir / "hash-comparison.json").write_text('{"ok":false}\n')
        review = self.catalog.retention(30, 1, 1, datetime.now(timezone.utc) + timedelta(days=100), apply=True)
        self.assertEqual(review["status"], "blocked")
        self.assertTrue((self.catalog.bundle(generation) / "files").is_dir())

    def test_protected_survivor_revalidation_failure_aborts_before_deletion(self):
        generations = [self.generation(number, publish=number == 31) for number in (30, 31)]
        for number, generation in zip((30, 31), generations):
            self.verify_good(generation, number)
        flip_last_byte(self.catalog.bundle(generations[1]) / "files/synthetic.txt.aead")
        with self.assertRaises(ValueError):
            self.catalog.retention(30, 1, 1, datetime.now(timezone.utc) + timedelta(days=100), apply=True)
        self.assertTrue((self.catalog.bundle(generations[0]) / "files").is_dir())
        self.assertFalse((self.state / "catalog" / generations[0] / "retention-intent.json").exists())

    def test_interrupted_exact_prune_is_reconciled_without_touching_manifest_or_evidence(self):
        generations = [self.generation(number, publish=number == 41) for number in (40, 41)]
        for number, generation in zip((40, 41), generations):
            self.verify_good(generation, number)
        generation = generations[0]
        manifest = self.retention_intent(generation)
        first = self.catalog.bundle(generation) / "files" / (manifest["files"][0]["path"] + ".aead")
        first.unlink()  # simulated crash after one exact payload unlink and before the result record
        manifest_path = self.catalog.bundle(generation) / "manifest.json"
        before_manifest = manifest_path.read_bytes()
        applied = self.catalog.retention(30, 1, 1, datetime.now(timezone.utc) + timedelta(days=100), apply=True)
        self.assertEqual([item["generation"] for item in applied["pruned"]], [generation])
        directory = self.state / "catalog" / generation
        self.assertTrue((directory / "retention-result.json").is_file())
        self.assertFalse((self.catalog.bundle(generation) / "files").exists())
        self.assertEqual(manifest_path.read_bytes(), before_manifest)
        self.assertTrue((directory / "verified-good.json").is_file())
        self.assertTrue((self.catalog.bundle(generations[1]) / "files").is_dir())

    def test_interrupted_prune_resumes_after_empty_payload_directory_removal(self):
        generations = [self.generation(number, publish=number == 46) for number in (45, 46)]
        for number, generation in zip((45, 46), generations):
            self.verify_good(generation, number)
        self.retention_intent(generations[0])
        files = self.catalog.bundle(generations[0]) / "files"
        (files / "synthetic.txt.aead").unlink()
        (files / "nested/inner.txt.aead").unlink()
        (files / "nested").rmdir()  # crash in the empty-directory removal phase
        applied = self.catalog.retention(30, 1, 1, datetime.now(timezone.utc) + timedelta(days=100), apply=True)
        self.assertEqual([item["generation"] for item in applied["pruned"]], [generations[0]])
        self.assertFalse(files.exists())

    def test_interrupted_prune_rechecks_survivor_before_any_unlink(self):
        generations = [self.generation(number, publish=number == 51) for number in (50, 51)]
        for number, generation in zip((50, 51), generations):
            self.verify_good(generation, number)
        self.retention_intent(generations[0])
        flip_last_byte(self.catalog.bundle(generations[1]) / "files/synthetic.txt.aead")
        with self.assertRaises(ValueError):
            self.catalog.retention(30, 1, 1, datetime.now(timezone.utc) + timedelta(days=100), apply=True)
        self.assertTrue((self.catalog.bundle(generations[0]) / "files/synthetic.txt.aead").is_file())
        self.assertTrue((self.catalog.bundle(generations[0]) / "files/nested/inner.txt.aead").is_file())
        self.assertFalse((self.state / "catalog" / generations[0] / "retention-result.json").exists())

    def assert_interrupted_prune_refuses_symlink(self, kind: str):
        generations = [self.generation(number, publish=number == 61) for number in (60, 61)]
        for number, generation in zip((60, 61), generations):
            self.verify_good(generation, number)
        self.retention_intent(generations[0])
        files = self.catalog.bundle(generations[0]) / "files"
        outside = self.root / f"outside-{kind}"
        outside.mkdir()
        if kind == "files-root":
            shutil.copytree(files, outside, dirs_exist_ok=True)
            shutil.rmtree(files)
            files.symlink_to(outside, target_is_directory=True)
            outside_payloads = [outside / "synthetic.txt.aead", outside / "nested/inner.txt.aead"]
        else:
            shutil.copytree(files / "nested", outside, dirs_exist_ok=True)
            shutil.rmtree(files / "nested")
            (files / "nested").symlink_to(outside, target_is_directory=True)
            outside_payloads = [outside / "inner.txt.aead"]
        with self.assertRaises(ValueError):
            self.catalog.retention(30, 1, 1, datetime.now(timezone.utc) + timedelta(days=100), apply=True)
        for payload in outside_payloads:
            self.assertTrue(payload.is_file(), "retention must never unlink through a symbolic link")
        self.assertFalse((self.state / "catalog" / generations[0] / "retention-result.json").exists())

    def test_interrupted_prune_refuses_symlinked_files_root(self):
        self.assert_interrupted_prune_refuses_symlink("files-root")

    def test_interrupted_prune_refuses_symlinked_intermediate_directory(self):
        self.assert_interrupted_prune_refuses_symlink("intermediate")


if __name__ == "__main__":
    unittest.main()
