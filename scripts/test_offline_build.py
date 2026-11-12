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

    def offline_branch(self, df):
        branch = df.split('"$OFFLINE_PACKAGES" = "1"')[1]
        return branch.split("else", 1)

    def assert_offline_guards(self, offline, release):
        self.assertIn("amd64|arm64) ;;", offline)
        self.assertIn("OFFLINE_PACKAGES=1 requires TARGETARCH", offline)
        self.assertIn("exit 1", offline)
        self.assertIn(release + ".*) ;;", offline)
        self.assertIn("/etc/alpine-release", offline)
        self.assertIn("sha256sum -c SHA256SUMS", offline)
        self.assertLess(offline.index("sha256sum -c"), offline.index("apk add"))
        self.assertLess(offline.index("/etc/alpine-release"), offline.index("sha256sum -c"))
        self.assertNotIn("apk upgrade", offline)
        self.assertNotIn("--repository", offline)
        self.assertIn("--no-network", offline)

    def test_wasl_runtime_has_offline_package_branch(self):
        df = (ROOT / "Dockerfile").read_text()
        for needle in ("ARG TARGETARCH", "ARG OFFLINE_PACKAGES=0",
                       "FROM scratch AS offline-bundle", "COPY deploy/offline/ /",
                       "RUN --mount=type=bind,from=offline-bundle,source=/,target=/vendor,readonly",
                       "/vendor/apk/${TARGETARCH}/*.apk", "USER 10001:10001"):
            self.assertIn(needle, df)
        self.assertNotRegex(df.split("FROM ${PYTHON_IMAGE}")[1], r"(?m)^COPY .*offline")
        offline, online = self.offline_branch(df)
        self.assert_offline_guards(offline, "3.24")
        self.assertIn("--no-index --find-links /vendor/wheels/${TARGETARCH} --require-hashes "
                      "-r /vendor/locks/wheels-${TARGETARCH}.txt", offline)
        self.assertNotIn("numpy==", offline)
        online = online.split("pip uninstall")[0]
        self.assertIn("apk upgrade --no-cache", online)
        self.assertIn("apk add --no-cache bash util-linux", online)
        self.assertNotIn("TARGETARCH", online)
        self.assertNotIn("/vendor", online)

    def test_nats_runtime_has_offline_package_branch(self):
        df = (ROOT / "deploy/nats/Dockerfile").read_text()
        for needle in ("ARG TARGETARCH", "ARG OFFLINE_PACKAGES=0",
                       "FROM scratch AS offline-bundle", "COPY offline/ /",
                       "RUN --mount=type=bind,from=offline-bundle,source=/,target=/vendor,readonly",
                       "/vendor/apk/${TARGETARCH}/*.apk", "USER 10001:10001", "chown -R 10001:10001 /data"):
            self.assertIn(needle, df)
        self.assertNotRegex(df.split("nats:${NATS_VERSION}-alpine")[1], r"(?m)^COPY .*offline")
        offline, online = self.offline_branch(df)
        self.assert_offline_guards(offline, "3.22")
        online = online.split("fi \\")[0]
        self.assertIn("apk upgrade --no-cache && apk add --no-cache ca-certificates", online)
        self.assertNotIn("TARGETARCH", online)

    def test_sha256sums_cover_every_bundle_file(self):
        for base in (ROOT / "deploy/offline", ROOT / "deploy/nats/offline"):
            listed = {l.split(None, 1)[1].strip() for l in (base / "SHA256SUMS").read_text().splitlines() if l.strip()}
            for sub in ("apk", "wheels"):
                if (base / sub).is_dir():
                    for f in (base / sub).rglob("*"):
                        if f.is_file():
                            self.assertIn(str(f.relative_to(base)), listed)

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


class CleanRunnerContractTests(unittest.TestCase):
    def setUp(self):
        self.runner = (ROOT / "scripts/offline_build_clean.sh").read_text()
        self.ci = (ROOT / ".github/workflows/ci.yml").read_text()

    def test_runner_flags_and_both_builds(self):
        for needle in ("--no-cache", "--network=none", "--pull-never", "--pull=false",
                       "OFFLINE_PACKAGES=1", "OFFLINE_RUNTIME=0", 'OFFLINE_BUILD_ID=$BUILD_ID',
                       'BUILD_ID="$(date +%s)"', "wasl-offline-clean:local", "wasl-nats-offline-clean:local"):
            self.assertIn(needle, self.runner)
        self.assertEqual(len(re.findall(r'^"\$ENGINE" build "\$\{COMMON\[@\]\}"', self.runner, re.M)), 2)
        self.assertIn('"$CONTEXT/deploy/nats"', self.runner)

    def test_runner_checks_binaries_uid_and_context_is_tracked_only(self):
        for needle in ("10001:10001", "/app/bin/wsi7", " -h", "/usr/local/bin/nats-server", '"nats-server: v${NATS_VERSION}"',
                       "image inspect"):
            self.assertIn(needle, self.runner)
        self.assertEqual(self.runner.count("--entrypoint id"), 2)
        self.assertIn('git -C "$ROOT" ls-files -z |', self.runner)
        self.assertNotIn("-coz", self.runner)

    def test_preflight_versions_match_build_args(self):
        for needle in ('"GO_VERSION=$GO_VERSION"', '"PYTHON_VERSION=$PYTHON_VERSION"', '"PYTHON_IMAGE=$PY_IMAGE"',
                       '"NATS_VERSION=$NATS_VERSION"', 'GO_IMAGE="golang:${GO_VERSION}-bookworm"',
                       'PY_IMAGE="python:${PYTHON_VERSION}-alpine"', 'NATS_IMAGE="nats:${NATS_VERSION}-alpine"'):
            self.assertIn(needle, self.runner)
        wasl_line = next(l for l in self.runner.splitlines() if '--tag "$WASL_TAG"' in l)
        nats_line = next(l for l in self.runner.splitlines() if '--tag "$NATS_TAG"' in l)
        self.assertIn("GO_VERSION=$GO_VERSION", wasl_line)
        self.assertIn("PYTHON_VERSION=$PYTHON_VERSION", wasl_line)
        self.assertIn("GO_VERSION=$GO_VERSION", nats_line)
        self.assertIn("NATS_VERSION=$NATS_VERSION", nats_line)

    def test_ci_runs_clean_runner_after_online_builds_with_buildkit(self):
        online = self.ci.index("docker build --no-cache --tag wasl-nats:local deploy/nats")
        clean = self.ci.index("DOCKER_BUILDKIT=1 WASL_CONTAINER_ENGINE=docker scripts/offline_build_clean.sh")
        self.assertLess(online, clean)
        self.assertLess(clean, self.ci.index("Scan the pipeline image"))

    def test_startup_stages_are_reported_and_ci_preloads_base_images(self):
        self.assertIn("failed (exit $rc) during stage: $STAGE", self.runner)
        for stage in ("base-image preflight", "fresh context", "build wasl", "build nats", "verify images"):
            self.assertIn(stage, self.runner)
        pull = self.ci.index("Preload base images into the Docker image store")
        run = self.ci.index("DOCKER_BUILDKIT=1 WASL_CONTAINER_ENGINE=docker scripts/offline_build_clean.sh")
        self.assertLess(pull, run)
        for image in ("golang:1.27.4-bookworm", "python:3.12-alpine", "nats:2.12.6-alpine"):
            self.assertIn("docker pull " + image, self.ci[pull:run])

    def test_dockerfiles_do_not_resolve_a_syntax_frontend(self):
        for path in ("Dockerfile", "deploy/nats/Dockerfile"):
            self.assertNotIn("# syntax=", (ROOT / path).read_text())


if __name__ == "__main__":
    unittest.main()
