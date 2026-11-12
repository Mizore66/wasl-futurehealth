# Offline container rebuild evidence — 12 Nov 2026

## Scope and revision

Synthetic Wasl pipeline/API image on one macOS/Podman host. Base `main` was `2d595e4c348c6156803094fd8a1cc796d7fa3c50`; tested implementation was `f876d7f`. Podman client 5.8.2, server 5.8.9, Linux/arm64. No second physical machine or Docker Engine result is claimed here.

## Verified

1. Generated `pipeline/vendor` with `go -C pipeline mod vendor`: 215 files, approximately 3.1 MB. `scripts/test_offline_build.py` regenerates it and compares the complete tree recursively.
2. Built once online as `wasl-offline-seed:local` to preseed the base images and Alpine/Python package layer.
3. Ran `scripts/offline_build.sh`. It created a new `/tmp` context and invoked Podman with `--network=none`, `--pull-never` and a fresh `OFFLINE_BUILD_ID`.
4. The Go `RUN ... go vet ... go build ...` layer executed without cache and completed with `GOFLAGS=-mod=vendor GOPROXY=off`. No module proxy or download was used.
5. The preseeded Alpine/Python package layer was reused. The complete image committed successfully as `wasl-offline:local` with image ID `3631e4c202e4d7b2eeb27425087028e9486ff5111774d24728835bfb781f5923`, identical to the online seed build.
6. Full Go tests and vet passed on the vendored tree. Offline/CI contract tests passed 28/28; deployment static checks and the licence-notice bundle check passed.

## Failed attempts retained as limitations

- The first runner used macOS `$TMPDIR` under `/var/folders`; Podman rejected the context with `faccessat ... connection refused`. The VM was healthy and was not restarted. Using `/tmp` fixed context sharing.
- The first network-disabled build completed the forced Go compile but failed at `apk upgrade` because that package layer was not cached. One ordinary online build seeded it; the subsequent complete offline rebuild passed.

## Limits

- This proves a full rebuild with **tracked Go dependencies and preseeded base images/package layers**. A no-cache build on a new or empty-cache host is not an offline build.
- Base-image pull refusal applies to the build command. Preparing those images and the Alpine/Python package layer requires an earlier online build or an equivalent operator-managed cache.
- This is one Podman host, not a second-machine, Docker Engine, production deployment or 24-hour soak claim. The hosted CI offline rebuild is a separate result and is not claimed until its fresh run passes.
