#!/usr/bin/env python3
"""Offline-build contracts; the container build itself is scripts/offline_build.sh."""
import filecmp
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


class OfflineBuildTests(unittest.TestCase):
    def test_dockerfile_and_runner_disable_module_and_container_networks(self):
        dockerfile = (ROOT / "Dockerfile").read_text()
        self.assertNotIn("go mod download", dockerfile)
        self.assertIn("COPY pipeline/vendor ./vendor", dockerfile)
        self.assertIn("GOFLAGS=-mod=vendor GOPROXY=off", dockerfile)
        self.assertIn("ARG OFFLINE_BUILD_ID=0", dockerfile)
        runner = (ROOT / "scripts/offline_build.sh").read_text()
        self.assertIn("--network=none", runner)
        self.assertIn("--pull-never", runner)
        self.assertIn("--pull=false", runner)
        self.assertIn('OFFLINE_BUILD_ID=$(date +%s)', runner)
        self.assertIn("ls-files -coz --exclude-standard -- pipeline/vendor", runner)

    def assert_vendor_tree_equal(self, left, right):
        comparison = filecmp.dircmp(left, right, ignore=[])
        self.assertEqual([], comparison.left_only)
        self.assertEqual([], comparison.right_only)
        self.assertEqual([], comparison.funny_files)
        for name in comparison.common_files:
            self.assertTrue(filecmp.cmp(Path(left) / name, Path(right) / name, shallow=False), name)
        for directory in comparison.common_dirs:
            self.assert_vendor_tree_equal(Path(left) / directory, Path(right) / directory)

    def test_tracked_vendor_matches_go_mod_vendor(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "vendor"
            subprocess.run(["go", "-C", str(ROOT / "pipeline"), "mod", "vendor", "-o", str(out)], check=True)
            self.assert_vendor_tree_equal(ROOT / "pipeline/vendor", out)


if __name__ == "__main__":
    unittest.main()
