# Empty-cache offline container build evidence — 12 Nov 2026

## Scope and revision

Synthetic Wasl pipeline/API image and rebuilt NATS image, one macOS host with Podman client 5.8.2 / server 5.8.9, Linux/arm64. Dependency bundles and Dockerfiles were those approved at `c0fee33`; the runner `scripts/offline_build_clean.sh` was added on top (the commit that contains this file). **No Docker Engine, hosted CI/amd64, second-machine or empty-host result is claimed.** The hosted CI step exists in `.github/workflows/ci.yml` but is not claimed until a fresh run passes.

## Command

```bash
WASL_CONTAINER_ENGINE=podman scripts/offline_build_clean.sh
```

The runner:

1. Requires `golang:1.27.4-bookworm`, `python:3.12-alpine` and `nats:2.12.6-alpine` to exist locally (preloaded base images; it never pulls). Local IDs: golang `4cfad6c86f52...`, python `99019a77a487...`, nats `4564348acc3c...`. Alpine releases were 3.24.4 (Wasl) and 3.22.3 (NATS), matching the bundles.
2. Extracts tracked files only into a new `/tmp/wasl-offline-clean.*` context.
3. Builds both images with `--no-cache --network=none --pull-never --build-arg OFFLINE_PACKAGES=1 --build-arg OFFLINE_BUILD_ID=$(date +%s)`; Wasl also gets `OFFLINE_RUNTIME=0`. Tags: `wasl-offline-clean:local`, `wasl-nats-offline-clean:local`.
4. Checks `.Config.User == 10001:10001` on both, runs `wsi7 -h` and `id -u` (Wasl) and `nats-server -v` (must equal `v2.12.6`) and `id -u` (NATS) with overridden entrypoints and `--network=none`.

## Results

| Run | Result | Wasl image ID | NATS image ID |
|---|---|---|---|
| 1 (first runner revision) | Both builds and all checks passed; wall time 94 s | `1b38de74f2d552d6fb502a11aaecd6f2c94e22245959c12e0401235fe3d8580b` | `022ba6dfcb236e30bd20164f86e5d5256d0baa1664ef0a9a91aa832c69dee5bf` |
| 2 (final runner) | Exit 0; both builds, UID, `wsi7 -h`, `nats-server: v2.12.6` passed | `80d2c3c3b1a423babc2345a73aed77de6404b54b4307740d1a6c14fa54ee7cc9` | `71dd9dec265bce7f03111fd0aec11d77c370c8dc273015f2acafc489684a2280` |

Both builds ran every RUN step (no cache), including the Go vet/compile layers with `-mod=vendor GOPROXY=off` and the outbound-TCP probe (which passed, meaning the network was unavailable), the `SHA256SUMS` check of each bundle, `apk add --no-network` and, for Wasl, `pip --no-index --require-hashes`.

Negative checks of the runner: `GO_VERSION=9.9.9` exits 1 with "base image ... is not loaded locally; clean builds never pull"; `WASL_CONTAINER_ENGINE=bogus` exits 2.

## Failures and observations

- Run 1's `wsi7 -h` check printed nothing and relied on a fallback `|| ... | grep usage` branch that never proves anything. It was replaced with a plain exit-status check, and the NATS version is now compared exactly. Run 2 is the run of record.
- The NATS stage logged `WARNING: fetching https://dl-cdn.alpinelinux.org/alpine/v3.22/...: temporary error` and the Wasl stage logged DNS transient errors during `apk add --no-cache --no-network`. These are the expected failed index lookups with no network (the install itself used only the bundled `.apk` files and succeeded); no package was downloaded. Both stages upgraded packages from the tracked files only (for example `musl`, `libcrypto3`, `ca-certificates`).
- `scripts/test_deploy.py::test_nats_image_wires_entrypoint` and `scripts/test_supply_chain.py::test_nats_binary_is_rebuilt_with_patched_toolchain_and_modules` initially failed because they asserted the old `go get` text; both now assert the vendored build (`COPY build/`, `-mod=vendor`, versions in `deploy/nats/build/go.mod`). `supply-chain.sh generate` initially failed with `Cannot stabilize scanner component reference` because the second Go module produced duplicate UUID-ref libraries; `scripts/supply_chain.py` now names them `wasl:library:<purl>@<manifest>` (regression test added first) and ignores `FROM scratch` bundle stages. Evidence was regenerated at 808799a and `verify` passes.
- `scripts/test_production_deploy.py::test_nats_starts_fresh_with_both_passwords_digit_leading` failed with `faccessat .../deploy/nats: connection refused`: the Podman VM cannot read this worktree path (the same cause as the `$TMPDIR` problem in the earlier record). It is an environment limitation, not a build result.

## Fix-round run (run of record)

After the review fixes (matching `GO_VERSION`/`PYTHON_VERSION`/`NATS_VERSION`/`PYTHON_IMAGE` build args, tracked-files-only context, removal of the `# syntax=docker/dockerfile:1` line from both Dockerfiles) the runner was rerun with `WASL_CONTAINER_ENGINE=podman scripts/offline_build_clean.sh` at commit `755032792642735b14bde0f5813e0d585293ae61` with a clean tree (`git status --short` empty). Exit 0; no frontend image was resolved. Wasl `42aaf9cf484972292fa66cce5936de51b866cc7f921804586f416cfcf101cac4`, NATS `db5a11b5e15bb04d4cac75fbb408b787935d938355476bd90a76e3ed38cecfd4` (both arm64). The earlier runs above were at `bc569f2` plus an uncommitted runner edit (run 1) and `bc569f2` pre-commit (run 2), so this run supersedes them. Supply-chain evidence was regenerated afterwards.

## Limits

- Base images must be preloaded locally. Preparing them (and the bundles) required online access and is out of scope. This is not an empty-host build.
- BuildKit is required on Docker for the `--mount` bind of the scratch-stage bundle; hosted Docker was not run.
- Only linux/arm64 was built. The amd64 bundle hashes are verified inside the build (the whole `SHA256SUMS` is checked) but amd64 packages were not installed or run.
- Bundles are pinned to their generation date; security updates require regeneration and review.
- No production-readiness, second-machine recovery or 24-hour soak claim.
