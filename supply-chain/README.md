# Supply-chain evidence

This directory contains a **source dependency inventory and scan evidence**, not a claim that source scanning proves a container or deployment is vulnerability-free. The CI workflow separately builds and scans the application and NATS images so installed OS and runtime packages are covered.

## Policy

- Trivy is pinned to `0.76.2`; CI verifies the release archive's SHA-256 before extracting it.
- `trivy.yaml` enables development dependencies and all severities: `UNKNOWN`, `LOW`, `MEDIUM`, `HIGH`, and `CRITICAL`.
- Any reported vulnerability fails the source gate and each image gate. A HIGH/CRITICAL-only result is not considered clean.
- There are no ignore entries, ignored vulnerability statuses, unfixed-vulnerability exclusions, or accepted-risk suppressions. Do not add them to make a run green.
- License identifiers are reported when discovered. Missing license metadata remains visible as an empty `licenses` array and in `summary.without_discovered_license`; it is not interpreted as permission or public domain. Scanner output is evidence for review, **not legal clearance**.
- Module-level advisories are retained even when the vulnerable package path is not compiled. Reachability needs a separate code-level analysis and must not be inferred from an SBOM alone.

## Prerequisites

Install Trivy `0.76.2`, Python 3, Go, Node 22, and pnpm `10.32.1`. For the most complete source license discovery, install the locked dependencies first:

```sh
pnpm --dir web install --frozen-lockfile
go -C pipeline mod download
python3 -I scripts/test_supply_chain.py
```

Without `web/node_modules`, Trivy can still inventory pnpm packages but cannot discover most of their license identifiers. This is a completeness limitation, not a clean license result.

The tracked macOS arm64 report inventories 760 language packages: 632 have at least one discovered license identifier and **128 have none**. The missing entries are predominantly platform-specific optional npm packages that are not installed on this host; 32 are non-development packages and 96 are development packages. They remain explicitly listed with empty license arrays and require review before any license-clearance claim. A Linux scan discovers a different optional-package subset, so its fresh CI artifact can have a different count.

## Generate tracked source evidence

The source tree must be clean. Run:

```sh
./scripts/supply-chain.sh generate
```

This writes:

| File | Contents |
| --- | --- |
| `repository.cdx.json` | CycloneDX source SBOM; pnpm and Go packages, npm integrity hashes, Go module checksums, declared Python runtime packages, and declared base images |
| `vulnerabilities.json` | Deterministic, normalized findings with every severity retained |
| `licenses.json` | Per-package discovered license identifiers and explicit missing-license counts |
| `manifest.json` | Source revision, Trivy vulnerability-DB provenance, offline/online mode, and evidence SHA-256 hashes |

The generator exits 1 after writing evidence if vulnerabilities remain. Operational failures exit 2 or the scanner's failure code; neither is treated as a clean scan.

A Git commit cannot contain its own hash. Evidence is therefore generated at a clean implementation commit and then committed separately. Its `source.revision` identifies that implementation commit. Verification accepts a descendant evidence-only commit only when all dependency and container declarations listed in `SOURCE_PATHS` are unchanged since the recorded revision. Later dependency changes require regeneration.

## Verify tracked evidence

```sh
./scripts/supply-chain.sh verify
```

Verification requires a clean tree, regenerates all three core reports with Trivy and the current lockfiles, compares their JSON content with the tracked copies, and applies the vulnerability gate. Set `WASL_SUPPLY_CHAIN_VERIFY_OUTPUT=out/supply-chain/source` to retain fresh reports as CI does.

On the same host, comparison is exact. Across different operating systems or architectures, optional-package license discovery varies: comparison still requires identical vulnerability reports, package identities, non-license SBOM content, and non-conflicting license findings for packages discovered on both hosts. `scan_environment` and the SBOM's `wasl:license-discovery-*` properties record this context. This canonicalization never ignores a package addition/removal or a vulnerability change.

### Offline rerun

After Trivy's vulnerability DB has already been downloaded:

```sh
WASL_SUPPLY_CHAIN_OFFLINE=1 ./scripts/supply-chain.sh verify
```

Offline mode sets `--offline-scan --skip-db-update` and records the cached DB metadata in the fresh manifest. **A completely cold offline scan is not supported**: Trivy requires a pre-seeded DB. License discovery also depends on the language package caches/installations described above.

## Built release images

Build the exact candidate images, then scan them:

```sh
docker build -t wasl:candidate .
docker build -t wasl-nats:candidate ./deploy/nats
./scripts/supply-chain.sh image wasl:candidate wasl-nats:candidate
```

Results are written under `out/supply-chain/images/<image>/`. Unlike the source SBOM, these image SBOMs include packages actually installed in each image, including OS packages and the Python runtime. Each image has an independent vulnerability gate; findings from one image do not prevent reports for the other image from being generated. For a Podman-backed local scan, build with Podman and set `TRIVY_IMAGE_SOURCE=podman`; if Trivy cannot discover the machine socket, also set `TRIVY_PODMAN_HOST` to the raw path reported by `podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}'` (without a `unix://` prefix). CI uses the Docker default.

Base-image tags are mutable. Release review must retain the generated image SBOM and Trivy image metadata for the actual image digest; rebuilding the same tag later is not proof of the same artifact.

## Remediation recorded by this change

The initial independent scan reported:

- `CVE-2026-118764` in development-only `minimatch@10.2.6`, fixed in `11.0.1`. The pnpm override targets affected `>=10 <11.0.1` resolutions without suppressing other minimatch versions.
- `GO-2026-5932` on the module `golang.org/x/crypto@v0.57.0`, severity `UNKNOWN`, fixed at `v0.58.1-0.20261009181718-2f1f834330b2`. The module is upgraded; this does not assert that the old unmaintained `openpgp` path was reachable.

The checked-in `vulnerabilities.json` is the authoritative result of the subsequent **source** scan, not this historical description.

### Image-driven remediations

Exercising the new image gate found additional release blockers that the source scan could not see:

- `nats:2.12.1-alpine` produced 188 findings (3 CRITICAL, 64 HIGH, 66 MEDIUM, 53 LOW, 2 UNKNOWN). Moving to `nats:2.12.6-alpine` removed the NATS server-module findings but left 133 in its bundled Go runtime/modules and Alpine packages. The Dockerfile now rebuilds NATS 2.12.6 with Go 1.27.3 and patched modules, replaces the upstream binary, and applies Alpine updates. The patched NATS binary passed the standalone rootfs analyzer, and the composed candidate image reported zero findings across all severities.
- `python:3.12-slim-bookworm` produced 273 findings, 267 without fixed versions in the scanner report. The application runtime now uses Alpine with OS updates, Bash, util-linux, and the same NumPy/SciPy pins. Pip 26.3 is used only during construction; pip and ensurepip are then removed because they are unnecessary at runtime and their vendored SBOM included advisories. The complete application candidate reported zero findings and ran as UID 10001 with NumPy 2.3.5 and SciPy 1.16.3.

Local Buildah could not access either the worktree or its own temporary build directory (`faccessat ... connection refused`), so an actual `docker build`/`podman build` was not claimed locally. The candidate layers were instead exercised in disposable Podman containers using the Dockerfile's build/install commands and copied sources. The shared Podman VM was not restarted because unrelated containers were running. CI still performs the authoritative clean Docker builds and image scans.

The candidate image license scans discovered identifiers for 73 of 81 application packages and 18 of 28 NATS packages. **8 application and 10 NATS packages still have no discovered identifier** in these reports. Those gaps remain review items just like the source-report gaps; zero vulnerabilities is not license clearance.

## Known boundaries

- The source SBOM lists Python declarations and base-image references, but it is deliberately labeled `source; not a built container SBOM`. It does not pretend to contain installed base-image OS packages.
- Npm integrity values hash registry artifacts. Go `h1:` values are module content hashes, not interchangeable zip-file SHA-256 values, so they are retained as explicitly named properties rather than mislabeled.
- The source SBOM does not claim license coverage for Python declarations or base images; use the corresponding built-image report.
- The frontend production build currently fetches Geist, Geist Mono, Hanken Grotesk, IBM Plex Sans Arabic, and Noto Nastaliq Urdu from Google Fonts. A network-disabled `next build` therefore remains blocked independently of these scanners. Supply-chain verification does not claim to fix that build limitation.
