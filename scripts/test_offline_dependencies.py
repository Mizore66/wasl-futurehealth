"""Tests for the offline dependency manifest tool (run: python3 -m unittest scripts/test_offline_dependencies.py)."""
import gzip
import hashlib
import io
import os
import tarfile
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

TOOL = Path(__file__).resolve().parent / "offline_dependency_manifest.py"
HEX = "0123456789abcdef"


def run(cmd, root, *extra):
    return subprocess.run([sys.executable, "-I", str(TOOL), cmd, str(root), *extra], capture_output=True, text=True)


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


ARCH_APK = {"amd64": "x86_64", "arm64": "aarch64"}
ARCH_WHL = {"amd64": "x86_64", "arm64": "aarch64"}


def make_apk(path, name, ver, arch):
    info = ("pkgname = %s\npkgver = %s\narch = %s\n" % (name, ver, arch)).encode()
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w") as t:
        ti = tarfile.TarInfo(".PKGINFO")
        ti.size = len(info)
        t.addfile(ti, io.BytesIO(info))
    Path(path).write_bytes(gzip.compress(buf.getvalue()))


class ProfileTests(unittest.TestCase):
    """Layout/lock validation: check <root> --profile nats|wasl."""

    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.root = Path(self._tmp.name)
        self.build("nats")

    def tearDown(self):
        self._tmp.cleanup()

    def build(self, profile):
        rel = {"nats": "3.22", "wasl": "3.24"}[profile]
        for arch in ("amd64", "arm64"):
            d = self.root / "apk" / arch
            d.mkdir(parents=True, exist_ok=True)
            pk = [("ca-certificates", "2026-r0")] + ([("bash", "5.3-r0"), ("util-linux", "2.42-r1")] if profile == "wasl" else [])
            lines = ["# alpine-release: %s.1" % rel]
            for n, v in pk:
                make_apk(d / ("%s-%s.apk" % (n, v)), n, v, ARCH_APK[arch])
                lines.append("%s=%s" % (n, v))
            (self.root / "locks").mkdir(exist_ok=True)
            (self.root / "locks" / ("apk-%s.lock" % arch)).write_text("\n".join(lines) + "\n")
            if profile == "wasl":
                w = self.root / "wheels" / arch
                w.mkdir(parents=True, exist_ok=True)
                wl = ["# alpine-release: %s.1" % rel]
                for n, v, tag in [("pip", "26.3", "py3-none-any"), ("numpy", "2.3.5", "cp312-cp312-musllinux_1_2_" + ARCH_WHL[arch]),
                                  ("scipy", "1.16.3", "cp312-cp312-musllinux_1_2_" + ARCH_WHL[arch]),
                                  ("cryptography", "50.0.2", "cp311-abi3-musllinux_1_2_" + ARCH_WHL[arch])]:
                    fn = "%s-%s-%s.whl" % (n, v, tag)
                    (w / fn).write_bytes(fn.encode())
                    wl.append("%s==%s --hash=sha256:%s  # %s" % (n, v, hashlib.sha256(fn.encode()).hexdigest(), fn))
                (self.root / "locks" / ("wheels-%s.txt" % arch)).write_text("\n".join(wl) + "\n")
        self.profile = profile
        self.regen()

    def regen(self):
        sums = self.root / "SHA256SUMS"
        if sums.exists():
            sums.unlink()
        self.assertEqual(run("generate", self.root).returncode, 0)

    def ok(self):
        return run("check", self.root, "--profile", self.profile).returncode == 0

    def test_valid_nats_and_wasl(self):
        self.assertTrue(self.ok())
        self.tearDown(); self.setUp(); self.build("wasl")
        self.assertTrue(self.ok())

    def test_rejects_empty_tree(self):
        for f in sorted(self.root.rglob("*"), reverse=True):
            f.unlink() if f.is_file() else f.rmdir()
        (self.root / "SHA256SUMS").write_text("")
        self.assertFalse(self.ok())

    def test_rejects_missing_arch(self):
        for f in (self.root / "apk" / "arm64").iterdir():
            f.unlink()
        (self.root / "locks" / "apk-arm64.lock").unlink()
        self.regen()
        self.assertFalse(self.ok())

    def test_rejects_nested_file(self):
        (self.root / "apk" / "amd64" / "sub").mkdir()
        (self.root / "apk" / "amd64" / "sub" / "x-1-r0.apk").write_bytes(b"x")
        self.regen()
        self.assertFalse(self.ok())

    def test_rejects_wrong_suffix(self):
        (self.root / "apk" / "amd64" / "evil.sh").write_text("x")
        self.regen()
        self.assertFalse(self.ok())

    def test_rejects_apk_arch_mismatch(self):
        make_apk(self.root / "apk" / "amd64" / "ca-certificates-2026-r0.apk", "ca-certificates", "2026-r0", "aarch64")
        self.regen()
        self.assertFalse(self.ok())

    def test_rejects_apk_metadata_name_mismatch(self):
        make_apk(self.root / "apk" / "amd64" / "ca-certificates-2026-r0.apk", "other", "2026-r0", "x86_64")
        self.regen()
        self.assertFalse(self.ok())

    def test_rejects_file_not_in_lock(self):
        make_apk(self.root / "apk" / "amd64" / "extra-1-r0.apk", "extra", "1-r0", "x86_64")
        self.regen()
        self.assertFalse(self.ok())

    def test_rejects_lock_entry_without_file(self):
        with open(self.root / "locks" / "apk-amd64.lock", "a") as f:
            f.write("ghost=1-r0\n")
        self.regen()
        self.assertFalse(self.ok())

    def test_rejects_wrong_release(self):
        p = self.root / "locks" / "apk-amd64.lock"
        p.write_text(p.read_text().replace("3.22.1", "3.24.1"))
        self.regen()
        self.assertFalse(self.ok())

    def test_wheel_rules(self):
        self.tearDown(); self.setUp(); self.build("wasl")
        d = self.root / "wheels" / "amd64"
        lock = self.root / "locks" / "wheels-amd64.txt"
        for bad in ("numpy-2.3.5-cp313-cp313-musllinux_1_2_x86_64.whl",
                    "numpy-2.3.5-cp312-cp312-manylinux_2_28_x86_64.whl",
                    "numpy-2.3.5-cp312-cp312-musllinux_1_2_aarch64.whl",
                    "numpy-2.3.5-cp312-cp312-win_amd64.whl"):
            orig = lock.read_text()
            old = d / "numpy-2.3.5-cp312-cp312-musllinux_1_2_x86_64.whl"
            new = d / bad
            old.rename(new)
            lock.write_text(orig.replace(old.name, bad))
            self.regen()
            self.assertFalse(self.ok(), bad)
            new.rename(old)
            lock.write_text(orig)
        self.regen()
        self.assertTrue(self.ok())

    def test_rejects_wheel_hash_not_matching_lock(self):
        self.tearDown(); self.setUp(); self.build("wasl")
        (self.root / "wheels" / "arm64" / "pip-26.3-py3-none-any.whl").write_bytes(b"tampered")
        self.regen()
        self.assertFalse(self.ok())

    def test_rejects_symlink_in_profile(self):
        os.symlink("apk-amd64.lock", self.root / "locks" / "l")
        self.assertFalse(self.ok())


if __name__ == "__main__":
    unittest.main()
