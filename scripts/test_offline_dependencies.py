"""Tests for the offline dependency manifest tool (run: python3 -m unittest scripts/test_offline_dependencies.py)."""
import hashlib
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

TOOL = Path(__file__).resolve().parent / "offline_dependency_manifest.py"
HEX = "0123456789abcdef"


def run(cmd, root):
    return subprocess.run([sys.executable, "-I", str(TOOL), cmd, str(root)], capture_output=True, text=True)


class ManifestTests(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.root = Path(self._tmp.name)
        (self.root / "apk" / "amd64").mkdir(parents=True)
        (self.root / "apk" / "amd64" / "b.apk").write_bytes(b"bbb")
        (self.root / "apk" / "amd64" / "a.apk").write_bytes(b"aaa")
        (self.root / "z.whl").write_bytes(b"zzz")

    def tearDown(self):
        self._tmp.cleanup()

    def generated(self):
        self.assertEqual(run("generate", self.root).returncode, 0)

    def test_generate_sorted_and_formatted(self):
        self.generated()
        lines = (self.root / "SHA256SUMS").read_text().splitlines()
        self.assertEqual([l.split("  ", 1)[1] for l in lines], ["apk/amd64/a.apk", "apk/amd64/b.apk", "z.whl"])
        for l in lines:
            digest, _ = l.split("  ", 1)
            self.assertEqual(len(digest), 64)
            self.assertTrue(set(digest) <= set(HEX))
        self.assertEqual(lines[0].split("  ")[0], hashlib.sha256(b"aaa").hexdigest())

    def test_check_accepts_generated_tree(self):
        self.generated()
        self.assertEqual(run("check", self.root).returncode, 0)

    def test_check_rejects_modified_byte(self):
        self.generated()
        (self.root / "z.whl").write_bytes(b"zzy")
        self.assertNotEqual(run("check", self.root).returncode, 0)

    def test_check_rejects_deleted_file(self):
        self.generated()
        (self.root / "z.whl").unlink()
        self.assertNotEqual(run("check", self.root).returncode, 0)

    def test_check_rejects_extra_file(self):
        self.generated()
        (self.root / "extra.whl").write_bytes(b"x")
        self.assertNotEqual(run("check", self.root).returncode, 0)

    def test_check_rejects_traversal_and_absolute(self):
        for bad in ("../outside", "/etc/passwd", "a/../../x"):
            self.generated()
            with open(self.root / "SHA256SUMS", "a") as f:
                f.write("%s  %s\n" % (hashlib.sha256(b"").hexdigest(), bad))
            self.assertNotEqual(run("check", self.root).returncode, 0, bad)

    def test_rejects_symlink(self):
        os.symlink("z.whl", self.root / "link.whl")
        self.assertNotEqual(run("generate", self.root).returncode, 0)
        (self.root / "link.whl").unlink()
        self.generated()
        os.symlink("z.whl", self.root / "link.whl")
        self.assertNotEqual(run("check", self.root).returncode, 0)

    def test_rejects_malformed_line_and_bad_digest(self):
        self.generated()
        with open(self.root / "SHA256SUMS", "a") as f:
            f.write("nothex  q.whl\n")
        self.assertNotEqual(run("check", self.root).returncode, 0)

    def test_rejects_duplicate_entry(self):
        self.generated()
        p = self.root / "SHA256SUMS"
        first = p.read_text().splitlines()[0]
        p.write_text(p.read_text() + first + "\n")
        self.assertNotEqual(run("check", self.root).returncode, 0)


if __name__ == "__main__":
    unittest.main()
