# Offline container rebuild evidence — 12 Nov 2026

## Scope and revision

Synthetic Wasl pipeline/API image on one macOS/Podman host. Base `main` was `2d595e4c348c6156803094fd8a1cc796d7fa3c50`; tested implementation was `9fb8586`. Podman client 5.8.2, server 5.8.9, Linux/arm64. No second physical machine or Docker Engine result is claimed here.

## Verified

1. Generated `pipeline/vendor` with `go -C pipeline mod vendor`: 215 files, approximately 3.1 MB. `scripts/test_offline_build.py` regenerates it and compares the complete tree recursively.
2. Built once online as `wasl:local` to preseed the complete runtime image, including Alpine/Python packages.
3. Ran `scripts/offline_build.sh`. It created a new `/tmp` context, required `wasl:local`, and invoked Podman with `--network=none`, `--pull-never`, `PYTHON_IMAGE=wasl:local`, `OFFLINE_RUNTIME=1` and a fresh `OFFLINE_BUILD_ID`. No APK or pip command ran in the offline build. The runtime was checked for `numpy`, `scipy`, `cryptography`, Bash, `flock` and UID 10001; inherited `/app` and run-output contents were removed before the current checkout was copied.
4. The Go `RUN ... go vet ... go build ...` layer executed without cache and completed with `GOFLAGS=-mod=vendor GOPROXY=off`. No module proxy or download was used. Its outbound-TCP probe failed closed when networking was deliberately enabled, and passed only when the connection was unavailable as expected offline.
5. The complete image committed successfully as `wasl-offline:local` with image ID `4de38d1363d082eb4e09e6247a5aa02ff3d3c8322bf8d83a02450d0bdae56b06`; `wsi7 -h` ran successfully from that image.
6. A separate seed with a planted `/app/stale-artifact` rebuilt successfully and the artifact was absent from the offline image. A plain `python:3.12-alpine` seed was rejected at `import numpy` as required.
7. Full Go tests and vet passed on the vendored tree. Offline/CI contract tests passed 28/28; deployment static checks and the licence-notice bundle check passed.

## Failed attempts retained as limitations

- The first runner used macOS `$TMPDIR` under `/var/folders`; Podman rejected the context with `faccessat ... connection refused`. The VM was healthy and was not restarted. Using `/tmp` fixed context sharing.
- The first network-disabled build completed the forced Go compile but failed at `apk upgrade` because that package layer was not cached. One ordinary online build seeded it; the subsequent complete offline rebuild passed. Hosted Docker later showed that its RUN cache is network-mode-specific, so relying on a cached package RUN was insufficient. Starting offline from the complete online-seeded `wasl:local` image with package installation disabled removed that dependency. An intermediate placement of `PYTHON_IMAGE` between stages was also rejected by Podman; declaring it globally fixed the parser error.

## Limits

- This proves a full rebuild with **tracked Go dependencies and preseeded base images/package layers**. A no-cache build on a new or empty-cache host is not an offline build.
- Base-image pull refusal applies to the build command. Preparing those images and the Alpine/Python package layer requires an earlier online build or an equivalent operator-managed cache.
- This is one Podman host, not a second-machine, Docker Engine, production deployment or 24-hour soak claim. The hosted CI offline rebuild is a separate result and is not claimed until its fresh run passes.
