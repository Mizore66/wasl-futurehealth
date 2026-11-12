# Empty-cache offline container build design

Approved scope: empty Docker/Podman build cache for both Wasl and NATS images, with preloaded base images. No base-image archives or empty-host claim.

## Goal

Run complete Wasl pipeline/API and NATS image builds with `--no-cache` and container networking disabled. Every RUN step must execute without downloading Go modules, Python packages or Alpine packages.

## Dependency bundles

Track these source-controlled bundles for linux/amd64 and linux/arm64:

- `pipeline/vendor`: existing Wasl Go modules.
- `deploy/nats/build`: a minimal Go module importing `github.com/nats-io/nats-server/v2`, with pinned `go.mod`, `go.sum`, `vendor` and a generation source file. Include patched x/crypto, x/sys and klauspost/compress versions already used by the NATS build.
- `deploy/offline/wheels/<arch>`: binary Python wheels for pip 26.3, numpy 2.3.5, scipy 1.16.3, cryptography 50.0.2 and their dependencies.
- `deploy/offline/apk/<arch>`: Bash, util-linux, ca-certificates and required/upgraded Alpine packages for the Wasl runtime.
- `deploy/nats/offline/apk/<arch>`: ca-certificates and required/upgraded Alpine packages for NATS.

Generation scripts run online only when updating bundles. They use containers for architecture-correct artifacts and record SHA-256 manifests. Tracked wheels/APKs are inputs reviewed with hashes, not executable scripts run from their directories.

## Dockerfiles

Wasl:

- Keep normal online behavior as the default.
- Add `OFFLINE_PACKAGES=1` to install APKs directly from the selected architecture directory and use `pip --no-index --find-links` for wheels. Offline mode must not run `apk upgrade`, fetch an APK index or resolve Python dependencies from a network.
- Use `TARGETARCH` to select `amd64` or `arm64`.
- Retain `GOFLAGS=-mod=vendor`, `GOPROXY=off`, the forced compile ID and the outbound-network probe. A clean build uses `OFFLINE_RUNTIME=0`, so no seeded application/runtime image is needed.

NATS:

- Replace `go mod init`/`go get` with the pinned build module and vendor tree; set `GOFLAGS=-mod=vendor` and `GOPROXY=off`.
- Add the same forced compile ID and outbound probe.
- Install runtime APKs directly from the architecture bundle without an index or network.
- Keep the final non-root UID, embedded configs, entrypoint and platform behavior unchanged.

## Runner and CI

Add `scripts/offline_build_clean.sh` that:

1. Creates a fresh `/tmp` context containing tracked files plus generated dependency bundles only.
2. Refuses base-image pulls.
3. Builds Wasl and NATS with `--no-cache` and `--network=none`.
4. Uses Docker-compatible and Podman-compatible flags.
5. Runs the Wasl executable and NATS binary version check from the resulting images.

The existing seeded-runtime runner remains available but is not the clean-build proof. CI runs the clean runner after its ordinary online builds, and documentation distinguishes:

- **Clean offline build:** empty layer cache, preloaded base images, all dependencies tracked.
- **Seeded-runtime rebuild:** the existing faster but weaker path.
- **Empty host:** not supported; base-image archives are out of scope.

## Verification

- Vendor-drift and dependency-manifest tests, including byte-exact comparisons.
- Both architectures present and every wheel/APK hash matches.
- No network package command remains in offline paths.
- Actual `--no-cache --network=none` builds of both images on Podman/arm64 and hosted Docker/amd64.
- Network-enabled probes fail; offline probes pass.
- Wasl executable and NATS binary run from the new images.
- Full Go/vet, Python deployment/supply-chain tests and licence checks pass.
- Independent review confirms that no base-image, cached-layer or hidden-download claim is overstated.

## Limits

- Base images must already exist locally; no second-machine or empty-host claim.
- Dependency bundles increase repository size and require architecture-specific security updates.
- NATS/CA/Python package freshness is pinned to the generation date; later security updates require regenerating and reviewing bundles.
- No production-readiness, second-machine recovery or 24-hour soak claim.
