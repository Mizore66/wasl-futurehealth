# Empty-Cache Offline Build Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build both Wasl and NATS images with empty Docker/Podman layer caches and no container-network access.

**Architecture:** Track architecture-specific APK and Python wheel bundles beside the existing Go vendor trees. Offline Docker stages install only those files; clean runners use `--no-cache`, disable base-image pulls and run binary smoke checks. Base images remain preloaded and are the only external inputs.

**Tech Stack:** Go 1.27.4, Docker/Podman, Alpine 3.24, Python 3.12 wheels, Bash, Python unittest.

**Spec:** `docs/superpowers/specs/2026-11-12-empty-cache-offline-build-design.md`

## Global Constraints

- Empty layer cache means `--no-cache`; preloaded base images are required. No empty-host claim.
- No `go get`, module download, APK index fetch or networked pip install in an offline path.
- Support `amd64` and `arm64`; use `TARGETARCH` selection.
- Go: `GOFLAGS=-mod=vendor`, `GOPROXY=off`; Python: `--no-index --find-links`; APK: direct local `.apk` paths.
- Preserve non-root runtime UID 10001, NATS config/entrypoint, existing online defaults and all security controls.
- Bundle generation is online-only; builds use tracked files and SHA-256 manifests.
- Never weaken vulnerability scans, evidence generation or deployment tests.
- Commit messages end with `Co-Authored-By: Claude Code <noreply@anthropic.com>`.

---

### Task 1: Pin and vendor the NATS build

**Files:**
- Create: `deploy/nats/build/go.mod`, `go.sum`, `main.go`, `vendor/**`
- Modify: `deploy/nats/Dockerfile`
- Test: `scripts/test_offline_build.py`

**Interfaces:**
- Consumes: existing NATS 2.12.6 and patched dependency versions.
- Produces: a self-contained build module and `OFFLINE_BUILD_ID`/`OFFLINE_PACKAGES` build arguments.

- [ ] **Step 1: Add failing static tests**

Require `deploy/nats/build/go.mod`, `go.sum`, `main.go`, `vendor/modules.txt`; require the Dockerfile not to contain `go get` or `go mod init`; require `GOPROXY=off`, `GOFLAGS=-mod=vendor`, `OFFLINE_BUILD_ID` and the outbound probe. Run `python3 -m unittest scripts.test_offline_build` and confirm failure on the missing module.

- [ ] **Step 2: Generate the pinned module**

`main.go` is:

```go
package main

import _ "github.com/nats-io/nats-server/v2"

func main() {}
```

Initialise `go.mod` as `module wasl-nats-build` with `go 1.27.4`, then run:

```bash
go -C deploy/nats/build get github.com/nats-io/nats-server/v2@v2.12.6 golang.org/x/crypto@v0.58.1-0.20261009181718-2f1f834330b2 golang.org/x/sys@v0.49.0 github.com/klauspost/compress@v1.20.0
go -C deploy/nats/build mod vendor
```

Keep generated go.sum and all vendor files. Extend the existing recursive byte-exact vendor test to regenerate this tree too.

- [ ] **Step 3: Make the NATS Dockerfile self-contained**

Copy `build/` into `/src`, set vendor/proxy environment, add the unique offline compile argument and use:

```dockerfile
RUN go vet . && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nats-server github.com/nats-io/nats-server/v2
```

Add the same conditional `timeout 3 bash -c 'exec 3<>/dev/tcp/proxy.golang.org/443'` refusal used by Wasl. Do not alter final UID, config or entrypoint.

- [ ] **Step 4: Verify and commit**

Run `go -C deploy/nats/build vet . && go -C deploy/nats/build test .` and the static tests. Commit `feat: vendor the NATS build dependencies`.

---

### Task 2: Generate and verify architecture-specific APK/wheel bundles

**Files:**
- Create: `scripts/offline_dependencies.sh`, `scripts/offline_dependency_manifest.py`, `scripts/test_offline_dependencies.py`
- Create: `deploy/offline/apk/{amd64,arm64}/**`, `deploy/offline/wheels/{amd64,arm64}/**`, `deploy/offline/SHA256SUMS`
- Create: `deploy/nats/offline/apk/{amd64,arm64}/**`, `deploy/nats/offline/SHA256SUMS`

**Interfaces:**
- Consumes: online container runtime for generation.
- Produces: `python3 scripts/offline_dependency_manifest.py generate|check <root>` and repeatable tracked dependency bundles.

- [ ] **Step 1: Write failing manifest tests**

Test that generate writes `<relative-path>  <64-hex-sha256>` sorted by path; check accepts that tree; a one-byte file change, deletion, extra unlisted artifact and path traversal line are rejected. Run the tests and confirm `offline_dependency_manifest.py` is missing.

- [ ] **Step 2: Implement manifest generation/checking**

Use `hashlib.sha256(path.read_bytes())`; reject symlinks and absolute/parent-relative paths. `generate` writes only regular files excluding `SHA256SUMS` itself. `check` requires exact path sets and hashes. No files are executed by the manifest tool.

- [ ] **Step 3: Implement online generation**

For each `amd64 arm64`, run `python:3.12-alpine` using `--platform linux/$arch` and a `/tmp` output mount:

```bash
pip download --only-binary=:all: --dest /out \
  pip==26.3 numpy==2.3.5 scipy==1.16.3 cryptography==50.0.2
apk fetch --recursive --output /out \
  bash util-linux ca-certificates krb5-libs zlib
```

Use separate output directories for Wasl wheels/APKs. Generate the NATS APK subset with `ca-certificates krb5-libs zlib`. Reject any non-wheel/non-APK artifact, generate both SHA256SUMS files and never run an interpreter from an artifact directory.

- [ ] **Step 4: Generate both architectures and verify**

Run the generator for `amd64 arm64`, run both manifest checks, inspect artifact sizes and confirm no source file was overwritten outside `deploy/offline` and `deploy/nats/offline`. Commit `feat: add offline runtime dependency bundles`.

---

### Task 3: Install offline runtime packages in both Dockerfiles

**Files:**
- Modify: `Dockerfile`, `deploy/nats/Dockerfile`
- Test: `scripts/test_offline_build.py`, `scripts/check_deploy.py`

**Interfaces:**
- Consumes: Task 2 artifact paths and `TARGETARCH`.
- Produces: `OFFLINE_PACKAGES=1` runtime path independent of any seeded Wasl image.

- [ ] **Step 1: Add failing offline-path tests**

Require `ARG TARGETARCH`, `ARG OFFLINE_PACKAGES=0`, architecture-selected COPY paths, `--no-index --find-links /vendor/wheels` and direct `/vendor/apk/*.apk` installation. Assert the offline branch does not run `apk upgrade` or use an APK repository index.

- [ ] **Step 2: Implement the Wasl runtime branch**

Keep `OFFLINE_RUNTIME=0` for clean builds. Add:

```dockerfile
COPY deploy/offline/wheels/${TARGETARCH} /vendor/wheels
COPY deploy/offline/apk/${TARGETARCH} /vendor/apk
```

In the existing package RUN, when `OFFLINE_PACKAGES=1`, use `apk add --no-cache /vendor/apk/*.apk`, then `pip install --no-index --find-links /vendor/wheels` with the existing four pinned requirements. Otherwise preserve the current online commands. Continue user creation, cleanup and final UID 10001.

- [ ] **Step 3: Implement the NATS runtime branch**

Copy `deploy/nats/offline/apk/${TARGETARCH}` relative to the existing NATS context as `offline/apk/${TARGETARCH}`. With `OFFLINE_PACKAGES=1`, directly install those APKs; preserve ca-certificates, user/group creation, `/data` ownership and entrypoint. Otherwise preserve `apk upgrade` plus online installation.

- [ ] **Step 4: Verify static contracts and commit**

Run `python3 -m unittest scripts.test_offline_build` and `python3 scripts/check_deploy.py`. Commit `feat: install runtime packages from offline bundles`.

---

### Task 4: Clean runner, CI and documentation

**Files:**
- Create: `scripts/offline_build_clean.sh`, `docs/reliability/evidence/offline-clean-build-2026-11-12.md`
- Modify: `.github/workflows/ci.yml`, `docs/DEPLOY.md`, `docs/CI.md`, `docs/RECOVERY.md`

**Interfaces:**
- Consumes: all prior bundles and Docker build arguments.
- Produces: `WASL_CONTAINER_ENGINE=docker|podman scripts/offline_build_clean.sh`.

- [ ] **Step 1: Add a clean-build runner**

Reuse the tracked-plus-vendor fresh-context pattern from `offline_build.sh`. Refuse base-image pulls, then run both builds with:

```bash
--no-cache --network=none \
--build-arg OFFLINE_PACKAGES=1 \
--build-arg OFFLINE_BUILD_ID=$(date +%s)
```

Use `--pull-never` for Podman and `--pull=false` for Docker. Tag `wasl-offline-clean:local` and `wasl-nats-offline-clean:local`. Run `wsi7 -h` and `nats-server -v` with overridden entrypoints; check final UID 10001.

- [ ] **Step 2: Add CI after the ordinary online image builds**

Run `DOCKER_BUILDKIT=1 WASL_CONTAINER_ENGINE=docker scripts/offline_build_clean.sh`. Keep scans and existing Compose smoke unchanged.

- [ ] **Step 3: Document exact scope**

State that clean builds have no layer cache and no runtime seed, but still require preloaded base images. Distinguish Wasl/NATS clean builds from the older seeded-runtime path and from empty-host support. Document online bundle regeneration and architecture/security-update responsibilities.

- [ ] **Step 4: Record only reproduced evidence**

Run the clean runner on Podman/arm64, preserve exact commands, image IDs and failed attempts. Do not claim hosted Docker/amd64 until its fresh CI run passes. Regenerate supply-chain evidence with `scripts/supply-chain.sh generate` and verify it. Commit `feat: verify empty-cache offline image builds`.

---

### Task 5: Full verification, independent review and merge

- [ ] **Step 1: Run full gates**

```bash
go -C pipeline test ./... -count=1
go -C pipeline vet ./...
PYTHONPATH=. python3 -m unittest scripts.test_offline_build scripts.test_offline_dependencies scripts.test_supply_chain scripts.test_ci_contract
python3 scripts/check_deploy.py
python3 scripts/notices.py check --root .
```

Run both clean image builds and binary checks; verify SHA256SUMS and all vendor trees.

- [ ] **Step 2: Independent review**

Review actual build logs and diff for hidden downloads, architecture mismatch, stale seed artifacts, false cache evidence, bundle integrity, non-root runtime and unsupported empty-host claims. Fix every Critical/Important finding and re-review.

- [ ] **Step 3: Push, PR and merge only on fresh green CI**

PR description ends with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`. Wait for every relevant check on the final head, especially the clean Docker offline step, then merge. If main advances, resolve and rerun CI.
