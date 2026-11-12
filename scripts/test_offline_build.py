#!/usr/bin/env python3
"""Offline-build contracts; the container build itself is scripts/offline_build.sh."""
import filecmp
import re
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
        self.assertIn('python -c "import numpy, scipy, cryptography"', dockerfile)
        self.assertIn("find /app /var/lib/wasl", dockerfile)
        runner = (ROOT / "scripts/offline_build.sh").read_text()
        self.assertIn("--network=none", runner)
        self.assertIn("--pull-never", runner)
        self.assertIn("--pull=false", runner)
        self.assertIn('OFFLINE_BUILD_ID=$(date +%s)', runner)
        self.assertIn('PYTHON_IMAGE=$RUNTIME_IMAGE', runner)
        self.assertIn("OFFLINE_RUNTIME=1", runner)
        self.assertIn('image inspect "$RUNTIME_IMAGE"', runner)
        self.assertIn("ls-files -coz --exclude-standard -- pipeline/vendor", runner)

    def test_nats_build_is_pinned_vendored_and_offline(self):
        build = ROOT / "deploy/nats/build"
        for name in ("go.mod", "go.sum", "main.go", "vendor/modules.txt"):
            self.assertTrue((build / name).is_file(), name)
        dockerfile = (ROOT / "deploy/nats/Dockerfile").read_text()
        self.assertNotIn("go get", dockerfile)
        self.assertNotIn("go mod init", dockerfile)
        self.assertIn("COPY build/ ./", dockerfile)
        self.assertIn("GOFLAGS=-mod=vendor GOPROXY=off", dockerfile)
        self.assertIn("ARG OFFLINE_BUILD_ID=0", dockerfile)
        self.assertIn("proxy.golang.org/443", dockerfile)
        self.assertIn("go vet . && CGO_ENABLED=0 go build -trimpath", dockerfile)
        self.assertIn("USER 10001:10001", dockerfile)

    def test_wasl_runtime_has_offline_package_branch(self):
        df = (ROOT / "Dockerfile").read_text()
        for needle in ("ARG TARGETARCH", "ARG OFFLINE_PACKAGES=0",
                       "COPY deploy/offline/wheels/${TARGETARCH} /vendor/wheels",
                       "COPY deploy/offline/apk/${TARGETARCH} /vendor/apk",
                       "--no-index --find-links /vendor/wheels", "/vendor/apk/*.apk",
                       "USER 10001:10001"):
            self.assertIn(needle, df)
        offline = df.split('"$OFFLINE_PACKAGES" = "1"')[1].split("else")[0]
        self.assertNotIn("apk upgrade", offline)
        self.assertNotIn("--repository", offline)
        self.assertIn("--no-network", offline)
        self.assertNotRegex(offline, r"pip install(?![^;]*--no-index)")
        online = df.split("else", 1)[1].split("pip uninstall")[0]
        self.assertIn("apk upgrade --no-cache", online)
        self.assertIn("apk add --no-cache bash util-linux", online)

    def test_nats_runtime_has_offline_package_branch(self):
        df = (ROOT / "deploy/nats/Dockerfile").read_text()
        for needle in ("ARG TARGETARCH", "ARG OFFLINE_PACKAGES=0",
                       "COPY offline/apk/${TARGETARCH} /vendor/apk", "USER 10001:10001",
                       "chown -R 10001:10001 /data"):
            self.assertIn(needle, df)
        branch = df.split('"$OFFLINE_PACKAGES" = "1"')[1]
        offline, online = branch.split("else", 1)
        self.assertNotIn("apk upgrade", offline)
        self.assertIn("--no-network /vendor/apk/*.apk", offline)
        self.assertIn("apk upgrade --no-cache && apk add --no-cache ca-certificates", online)

    def test_offline_bundles_exist_per_arch(self):
        for arch in ("amd64", "arm64"):
            self.assertEqual(7, len(list((ROOT / "deploy/nats/offline/apk" / arch).glob("*.apk"))))
            self.assertTrue(list((ROOT / "deploy/offline/apk" / arch).glob("*.apk")))
            self.assertTrue(list((ROOT / "deploy/offline/wheels" / arch).glob("*.whl")))

    def test_nats_version_matches_go_mod_and_tools_module_is_vendored(self):
        df = (ROOT / "deploy/nats/Dockerfile").read_text()
        arg = re.search(r"^ARG NATS_VERSION=(\S+)$", df, re.M).group(1)
        build = ROOT / "deploy/nats/build"
        mod = re.search(r"github\.com/nats-io/nats-server/v2 v(\S+)", (build / "go.mod").read_text()).group(1)
        self.assertEqual(mod, arg)
        self.assertIn("github.com/nats-io/nats-server/v2", (build / "tools.go").read_text())
        modules = (build / "vendor/modules.txt").read_text()
        self.assertIn("# github.com/nats-io/nats-server/v2 v%s" % mod, modules)
        self.assertTrue((build / "vendor/github.com/nats-io/nats-server/v2").is_dir())

    def test_tracked_nats_vendor_matches_go_mod_vendor(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "vendor"
            subprocess.run(["go", "-C", str(ROOT / "deploy/nats/build"), "mod", "vendor", "-o", str(out)], check=True)
            self.assert_vendor_tree_equal(ROOT / "deploy/nats/build/vendor", out)

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
