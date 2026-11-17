#!/usr/bin/env python3
"""Chunked (wasl-file-backup-v3-chunked) recovery bundle tests: bounded memory and scratch beyond the old 64 MB tmpfs
cap, per-chunk authentication, and fail-closed truncation, reordering, duplication, tampering and wrong keys.
Synthetic payloads only; no container daemon or NATS server required."""
import base64
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile
import tracemalloc
import unittest

SCRIPTS = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPTS))

import recovery  # noqa: E402
from recovery import (  # noqa: E402
    SCHEMA_V2,
    SCHEMA_V3,
    TAG_BYTES,
    _canonical,
    _manifest_mac,
    decrypt_nats_payload,
    migrate_files,
    stream_nats_payload_to_command,
    verify_bundle,
    write_file_bundle,
)
from recovery_crypto import load_keyring, new_keyring, unwrap_dek, wrap_dek  # noqa: E402

OLD_SCRATCH_CAP = 64 * 1000 * 1000  # the production recovery tmpfs is size=64m (64 MiB); this is the stricter bound
# Writes a deterministic synthetic stream to the inherited pipe path given as its last argument.
GENERATOR = r"""
import sys
size, block = int(sys.argv[1]), int(sys.argv[2])
fail_after = int(sys.argv[3])
with open(sys.argv[-1], "wb") as out:
    written, index = 0, 0
    while written < size:
        if 0 <= fail_after <= written:
            sys.exit(7)
        piece = bytes([index % 251]) * min(block, size - written)
        out.write(piece)
        written += len(piece)
        index += 1
"""


def synthetic_digest(size: int, block: int) -> str:
    digest, written, index = hashlib.sha256(), 0, 0
    while written < size:
        piece = bytes([index % 251]) * min(block, size - written)
        digest.update(piece)
        written += len(piece)
        index += 1
    return digest.hexdigest()


def copier(target: Path) -> list[str]:
    """A restore-command stand-in that copies the streamed payload from its pipe argument to `target`."""
    return [sys.executable, "-c", "import sys\nwith open(sys.argv[2], 'rb') as i, open(sys.argv[1], 'wb') as o:\n"
            "    o.write(i.read())", str(target)]


def generator(size: int, block: int = 65536, fail_after: int = -1) -> list[str]:
    return [sys.executable, "-c", GENERATOR, str(size), str(block), str(fail_after)]


class ChunkedBundleTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root, True)
        self.keys = self.root / "recovery_keys.json"
        new_keyring(self.keys)
        self.source = self.root / "source"
        (self.source / "run").mkdir(parents=True)
        (self.source / "run/audit.json").write_text('{"synthetic":true,"padding":"' + "x" * 40 + '"}\n')
        (self.source / "empty.txt").write_bytes(b"")
        self.nats = self.root / "nats.json"
        self.nats.write_bytes(b"synthetic-nats-payload-" * 3)

    def backup(self, name="bundle", chunk_size=5, **extra):
        bundle = self.root / name
        manifest = write_file_bundle(self.source, bundle, self.keys, nats_json=self.nats, chunk_size=chunk_size,
                                     **extra)
        return bundle, manifest

    def resign(self, bundle, mutate):
        manifest = json.loads((bundle / "manifest.json").read_text())
        mutate(manifest)
        manifest.pop("hmac_sha256", None)
        doc, _, _ = load_keyring(self.keys)
        auth = base64.b64decode(doc["keys"][manifest["key_id"]]["auth_key"])
        manifest["hmac_sha256"] = _manifest_mac(manifest, auth)
        (bundle / "manifest.json").write_bytes(_canonical(manifest) + b"\n")

    @staticmethod
    def record(manifest, path):
        return next(r for r in manifest["files"] if r["path"] == path)

    def test_default_backup_is_chunked_and_restores_byte_identically(self):
        bundle, manifest = self.backup()
        self.assertEqual(manifest["schema"], SCHEMA_V3)
        self.assertEqual(manifest["chunk_size"], 5)
        audit = self.record(manifest, "run/audit.json")
        size = (self.source / "run/audit.json").stat().st_size
        self.assertEqual(audit["chunks"], -(-size // 5))
        self.assertEqual((bundle / "files/run/audit.json.aead").stat().st_size, size + audit["chunks"] * TAG_BYTES)
        self.assertEqual(self.record(manifest, "empty.txt")["chunks"], 1)
        raw = b"".join(p.read_bytes() for p in bundle.rglob("*") if p.is_file())
        self.assertNotIn(b"synthetic", raw)
        self.assertEqual(verify_bundle(bundle, self.keys)["schema"], SCHEMA_V3)
        migrate_files(bundle, self.keys, self.root / "live")
        for name in ("run/audit.json", "empty.txt"):
            self.assertEqual((self.root / "live" / name).read_bytes(), (self.source / name).read_bytes())
        self.assertFalse((self.root / "live/nats").exists(), "the NATS payload is never restored as a run file")
        out = self.root / "nats.out"
        decrypt_nats_payload(bundle, self.keys, out)
        self.assertEqual(out.read_bytes(), self.nats.read_bytes())

    def test_every_chunk_corruption_fails_closed(self):
        size = (self.source / "run/audit.json").stat().st_size
        piece = 5 + TAG_BYTES

        def reorder(data):
            return data[:piece] + data[2 * piece:3 * piece] + data[piece:2 * piece] + data[3 * piece:]

        def duplicate(data):
            return data[:piece] + data[:piece] + data[2 * piece:]

        cases = {
            "bit flip": lambda d: d[:7] + bytes([d[7] ^ 1]) + d[8:],
            "reordered chunks": reorder,
            "duplicated chunk": duplicate,
            "truncated mid chunk": lambda d: d[:-3],
            "missing final chunk": lambda d: d[:-(size % 5 + TAG_BYTES if size % 5 else piece)],
            "appended chunk": lambda d: d + d[:piece],
            "empty": lambda d: b"",
        }
        for label, mutate in cases.items():
            for fix_hash in (False, True):
                with self.subTest(label, ciphertext_hash_fixed=fix_hash):
                    bundle, _ = self.backup(f"{label}-{fix_hash}".replace(" ", "-"))
                    cipher = bundle / "files/run/audit.json.aead"
                    tampered = mutate(cipher.read_bytes())
                    cipher.write_bytes(tampered)
                    if fix_hash:  # an attacker with the auth key still cannot make chunk authentication pass
                        self.resign(bundle, lambda m: self.record(m, "run/audit.json").__setitem__(
                            "ciphertext_sha256", hashlib.sha256(tampered).hexdigest()))
                    with self.assertRaises(ValueError):
                        verify_bundle(bundle, self.keys)
                    with self.assertRaises(ValueError):
                        migrate_files(bundle, self.keys, self.root / f"live-{label}-{fix_hash}")
                    self.assertFalse((self.root / f"live-{label}-{fix_hash}").exists())

    def test_truncation_at_a_chunk_boundary_fails_on_the_final_flag(self):
        bundle, manifest = self.backup()
        cipher = bundle / "files/run/audit.json.aead"
        data = cipher.read_bytes()
        record = self.record(manifest, "run/audit.json")
        keep_chunks = record["chunks"] - 1
        plain = (self.source / "run/audit.json").read_bytes()[:keep_chunks * 5]
        shortened = data[:keep_chunks * (5 + TAG_BYTES)]
        cipher.write_bytes(shortened)

        def shrink(m):  # a fully consistent, re-authenticated manifest for the shorter file
            r = self.record(m, "run/audit.json")
            r.update(size=len(plain), chunks=keep_chunks, sha256=hashlib.sha256(plain).hexdigest(),
                     ciphertext_sha256=hashlib.sha256(shortened).hexdigest())
        self.resign(bundle, shrink)
        with self.assertRaisesRegex(ValueError, "chunk .* failed authentication"):
            verify_bundle(bundle, self.keys)

    def test_authenticated_metadata_and_layout_changes_fail_closed(self):
        def nonce_swap(m):
            a, b = self.record(m, "run/audit.json"), self.record(m, "empty.txt")
            a["nonce_prefix"], b["nonce_prefix"] = b["nonce_prefix"], a["nonce_prefix"]

        cases = {
            "chunk size": (lambda m: m.__setitem__("chunk_size", 6), "malformed|failed authentication|wrong size"),
            "chunk size zero": (lambda m: m.__setitem__("chunk_size", 0), "malformed"),
            "chunk size too large": (lambda m: m.__setitem__("chunk_size", recovery.MAX_CHUNK_SIZE + 1), "malformed"),
            "chunk count": (lambda m: self.record(m, "run/audit.json").__setitem__("chunks", 1), "malformed"),
            "size": (lambda m: self.record(m, "run/audit.json").__setitem__(
                "size", self.record(m, "run/audit.json")["size"] - 1), "malformed|wrong size"),
            "plaintext hash": (lambda m: self.record(m, "run/audit.json").__setitem__("sha256", "0" * 64),
                               "plaintext does not match"),
            "nonce prefix swap": (nonce_swap, "failed authentication"),
            "duplicate nonce prefix": (lambda m: self.record(m, "empty.txt").__setitem__(
                "nonce_prefix", self.record(m, "run/audit.json")["nonce_prefix"]), "reuses"),
            "v2 nonce field": (lambda m: self.record(m, "empty.txt").__setitem__("nonce", "A" * 16), "malformed"),
            "missing chunk_size": (lambda m: m.pop("chunk_size"), "malformed"),
            "downgrade to v2 schema": (lambda m: m.__setitem__("schema", SCHEMA_V2), "malformed|unwrap"),
        }
        for label, (mutate, pattern) in cases.items():
            with self.subTest(label):
                bundle, _ = self.backup(label.replace(" ", "-"))
                self.resign(bundle, mutate)
                with self.assertRaisesRegex(ValueError, pattern):
                    verify_bundle(bundle, self.keys)

    def test_unauthenticated_manifest_edit_and_wrong_key_are_refused(self):
        bundle, _ = self.backup()
        path = bundle / "manifest.json"
        original = path.read_text()
        manifest = json.loads(original)
        manifest["chunk_size"] = 7
        path.write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, "authentication"):
            verify_bundle(bundle, self.keys)
        path.write_text(original)
        other = self.root / "other.json"
        new_keyring(other)
        for operation in (lambda: verify_bundle(bundle, other),
                          lambda: decrypt_nats_payload(bundle, other, self.root / "never.json"),
                          lambda: migrate_files(bundle, other, self.root / "never")):
            with self.assertRaises(ValueError):
                operation()
        self.assertFalse((self.root / "never.json").exists())
        self.assertFalse((self.root / "never").exists())

    def test_wrapped_data_key_is_bound_to_the_bundle_format(self):
        _, key_id, entry = load_keyring(self.keys)
        dek = os.urandom(32)
        nonce, wrapped = wrap_dek(entry, key_id, dek, SCHEMA_V3)
        self.assertEqual(unwrap_dek(entry, key_id, nonce, wrapped, SCHEMA_V3), dek)
        with self.assertRaisesRegex(ValueError, "unwrap failed"):
            unwrap_dek(entry, key_id, nonce, wrapped)  # the v2 wrap context cannot open a v3 key
        v2_nonce, v2_wrapped = wrap_dek(entry, key_id, dek)
        with self.assertRaisesRegex(ValueError, "unwrap failed"):
            unwrap_dek(entry, key_id, v2_nonce, v2_wrapped, SCHEMA_V3)

    def test_chunk_size_is_validated_before_anything_is_written(self):
        for bad in (0, -1, recovery.MAX_CHUNK_SIZE + 1, 1.5, True):
            with self.subTest(bad):
                with self.assertRaisesRegex(ValueError, "chunk size"):
                    write_file_bundle(self.source, self.root / "bad", self.keys, chunk_size=bad)
                self.assertFalse((self.root / "bad").exists())

    def test_interrupted_nats_stream_publishes_no_bundle(self):
        before = sorted(p.name for p in self.root.iterdir())
        with self.assertRaisesRegex(ValueError, "NATS backup command failed"):
            write_file_bundle(self.source, self.root / "bundle", self.keys, chunk_size=1024,
                              nats_command=generator(200_000, 4096, fail_after=100_000))
        self.assertEqual(sorted(p.name for p in self.root.iterdir()), before, "no bundle or staging may remain")
        with self.assertRaisesRegex(ValueError, "could not be started"):
            write_file_bundle(self.source, self.root / "bundle", self.keys,
                              nats_command=[str(self.root / "absent-command")])
        self.assertEqual(sorted(p.name for p in self.root.iterdir()), before)
        with self.assertRaisesRegex(ValueError, "either"):
            write_file_bundle(self.source, self.root / "bundle", self.keys, nats_json=self.nats,
                              nats_command=generator(10))
        with self.assertRaisesRegex(ValueError, "chunked"):
            write_file_bundle(self.source, self.root / "bundle", self.keys, nats_command=generator(10),
                              schema=SCHEMA_V2)

    def test_existing_v2_bundle_remains_verifiable_and_restorable(self):
        bundle = self.root / "v2"
        manifest = write_file_bundle(self.source, bundle, self.keys, nats_json=self.nats, schema=SCHEMA_V2)
        self.assertEqual(manifest["schema"], SCHEMA_V2)
        self.assertNotIn("chunk_size", manifest)
        self.assertEqual(verify_bundle(bundle, self.keys)["schema"], SCHEMA_V2)
        migrate_files(bundle, self.keys, self.root / "live")
        self.assertEqual((self.root / "live/run/audit.json").read_bytes(),
                         (self.source / "run/audit.json").read_bytes())
        out = self.root / "nats.out"
        decrypt_nats_payload(bundle, self.keys, out)
        self.assertEqual(out.read_bytes(), self.nats.read_bytes())
        with self.assertRaisesRegex(ValueError, "chunked"):
            stream_nats_payload_to_command(bundle, self.keys, ["cat"])

    def test_streamed_nats_restore_command_sees_verified_payload_and_failures_propagate(self):
        bundle, _ = self.backup(chunk_size=7)
        capture = self.root / "captured"
        stream_nats_payload_to_command(bundle, self.keys, copier(capture))
        self.assertEqual(capture.read_bytes(), self.nats.read_bytes())
        with self.assertRaisesRegex(ValueError, "NATS command failed"):
            stream_nats_payload_to_command(bundle, self.keys, [sys.executable, "-c", "import sys;sys.exit(4)"])
        # A command that stops reading early is a failure even if it exits 0.
        large = self.root / "large-bundle"
        big = self.root / "big-nats.json"
        big.write_bytes(b"n" * 300_000)
        write_file_bundle(self.source, large, self.keys, nats_json=big, chunk_size=4096)
        with self.assertRaisesRegex(ValueError, "stopped reading"):
            stream_nats_payload_to_command(large, self.keys, [sys.executable, "-c", "import sys"])
        # A tampered final chunk is never delivered: the reader sees a truncated stream.
        cipher = bundle / "files/nats/nats.json.aead"
        data = bytearray(cipher.read_bytes())
        data[-1] ^= 1
        cipher.write_bytes(bytes(data))
        seen = self.root / "seen"
        with self.assertRaisesRegex(ValueError, "failed authentication"):
            stream_nats_payload_to_command(bundle, self.keys, copier(seen))
        self.assertLess(seen.stat().st_size, self.nats.stat().st_size)


class BoundedResourceTests(unittest.TestCase):
    """A synthetic NATS stream larger than the old 64 MB scratch cap goes through backup, verify, extraction and a
    streamed restore with small forced chunks; Python heap peaks and scratch use are measured, not assumed."""

    SIZE = 72 * 1024 * 1024  # 75,497,472 bytes > the 64 MiB tmpfs
    CHUNK = 64 * 1024
    HEAP_LIMIT = 4 * 1024 * 1024

    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root, True)
        self.keys = self.root / "recovery_keys.json"
        new_keyring(self.keys)
        self.source = self.root / "source"
        self.source.mkdir()
        (self.source / "run.txt").write_text("synthetic run file\n")
        self.scratch = self.root / "tmp"
        self.scratch.mkdir()

    def measured(self, operation):
        tracemalloc.start()
        try:
            result = operation()
            _, peak = tracemalloc.get_traced_memory()
        finally:
            tracemalloc.stop()
        return result, peak

    def test_streams_beyond_old_scratch_cap_with_bounded_memory_and_no_plaintext_scratch(self):
        self.assertGreater(self.SIZE, OLD_SCRATCH_CAP)
        bundle = self.root / "bundle"
        old_tmp = os.environ.get("TMPDIR")
        os.environ["TMPDIR"] = str(self.scratch)
        tempfile.tempdir = None
        try:
            manifest, backup_peak = self.measured(lambda: write_file_bundle(
                self.source, bundle, self.keys, chunk_size=self.CHUNK, nats_command=generator(self.SIZE, 65536)))
        finally:
            if old_tmp is None:
                os.environ.pop("TMPDIR", None)
            else:
                os.environ["TMPDIR"] = old_tmp
            tempfile.tempdir = None
        self.assertEqual(list(self.scratch.iterdir()), [], "backup staged plaintext in TMPDIR")
        record = next(r for r in manifest["files"] if r["path"] == "nats/nats.json")
        expected = synthetic_digest(self.SIZE, 65536)
        self.assertEqual((record["size"], record["sha256"]), (self.SIZE, expected))
        self.assertEqual(record["chunks"], self.SIZE // self.CHUNK)
        _, verify_peak = self.measured(lambda: verify_bundle(bundle, self.keys))
        out = self.root / "restored-nats.json"
        _, extract_peak = self.measured(lambda: decrypt_nats_payload(bundle, self.keys, out))
        restored_digest = hashlib.sha256()
        with out.open("rb") as handle:
            while chunk := handle.read(1 << 20):
                restored_digest.update(chunk)
        self.assertEqual(restored_digest.hexdigest(), expected, "clean-restore hash differs from the source stream")
        out.unlink()
        hasher = [sys.executable, "-c",
                  "import hashlib,sys\nh=hashlib.sha256()\nwith open(sys.argv[1],'rb') as f:\n"
                  "    for b in iter(lambda: f.read(1<<20), b''): h.update(b)\nprint(h.hexdigest())"]
        captured = self.root / "pipe-hash"
        _, pipe_peak = self.measured(lambda: stream_nats_payload_to_command(
            bundle, self.keys, ["/bin/sh", "-c", f'"$0" "$@" > {captured}', *hasher]))
        self.assertEqual(captured.read_text().strip(), expected, "streamed restore input hash differs")
        peaks = {"backup": backup_peak, "verify": verify_peak, "extract": extract_peak, "pipe": pipe_peak}
        print(f"\nbounded-resource measurement: payload={self.SIZE} chunk={self.CHUNK} "
              f"python_heap_peak_bytes={peaks}", file=sys.stderr)
        for name, peak in peaks.items():
            self.assertLess(peak, self.HEAP_LIMIT, f"{name} heap peak {peak} is not bounded by the chunk size")

    def test_cli_backup_and_verify_process_rss_stays_below_payload_size(self):
        """Whole-process resident memory (Python, OpenSSL and the payload generator), not only the Python heap."""
        bundle = self.root / "cli-bundle"
        command = " ".join(shlex.quote(part) for part in generator(self.SIZE, 65536))
        rss = []
        for argv in (["backup", self.source, bundle, "--key-file", self.keys, "--chunk-size", str(self.CHUNK),
                      "--nats-command", command],
                     ["verify", bundle, "--key-file", self.keys]):
            probe = subprocess.run(
                [sys.executable, "-c", "import resource,subprocess,sys\n"
                 "subprocess.run(sys.argv[1:], check=True, stdout=subprocess.DEVNULL)\n"
                 "print(resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss)",
                 sys.executable, str(SCRIPTS / "recovery.py"), *map(str, argv)],
                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=dict(os.environ, TMPDIR=str(self.scratch)))
            self.assertEqual(probe.returncode, 0, probe.stderr[-2000:])
            rss.append(int(probe.stdout.strip()) * (1 if sys.platform == "darwin" else 1024))  # bytes on macOS
        print(f"\nbounded-resource measurement: payload={self.SIZE} chunk={self.CHUNK} max_rss_bytes backup={rss[0]} "
              f"verify={rss[1]}", file=sys.stderr)
        self.assertEqual(list(self.scratch.iterdir()), [])
        for value in rss:
            self.assertLess(value, self.SIZE, "process memory grew with the payload")


if __name__ == "__main__":
    unittest.main()
