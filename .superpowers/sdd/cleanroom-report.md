# Clean-room reproducibility report

Commit under test: 3c99b05c99964fb90c055daf9d7b20e8d2f54997 (branch feat/clean-room-demo-check).
Fresh clone: `/tmp/wasl-cleanroom-run/fresh.apzboY/wasl`, made with `git clone --no-hardlinks <worktree>` then `git checkout 3c99b05`. It had no `bin/`, `out/` or untracked files (`git status` clean at start and after both runs). Outputs went to the clone's gitignored `out/` and to `/tmp/wasl-cleanroom-run/fresh.apzboY/{out7,samples}`. Logs: `demo-all.log`, `demo7.log`, `serve.log` in `/tmp/wasl-cleanroom-run/fresh.apzboY/`.

Environment: macOS, go from /opt/homebrew/bin, `PATH=/opt/anaconda3/bin:/tmp/wasl-cleanroom-tools:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin`. Only external asset supplied: `nats-server` v2.12.1 (symlink in /tmp/wasl-cleanroom-tools to an existing local binary). No GGUF, no model or network calls (WASL_READER=cached default).

## Commands and results

1. `scripts/demo.sh all` (defaults: v7strict, cached, fhir, seed 2026): exit 0, 8m40s.
   check OK, build OK, Go tests `ok wsi 149.9s`, cohort, run (chaos incl. NATS node kill), audit, tamper (all three tampers detected), booking, report, smoke.
   Baseline: CRC with review P 1.000 R 0.837, false closures 0/474; cervical P 1.000 R 0.967, 0/204. Exactly-once duplicates 0/25176, provenance 912/912, forged-with-effect 0/18, wrong-patient releases 0, consent-less releases 0. `api smoke OK: 2100 cases, 3 fixtures checked`.
2. Seven-programme recipe from docs/api-samples/README.md with `WASL_OUT=.../out7 WASL_PORT_OFFSET=100 WASL_LUNG=800 WASL_DR=1200 WASL_BREAST=70 WASL_LDCT=300 WASL_IFHAS=700 WASL_DRNEXT=1 WASL_LUNGSURV=1 WASL_XREFLINK=1 WASL_REFREJECT=true`, steps check, cohort, run, audit, tamper, booking, report, smoke (Go tests skipped; already run in step 1): exit 0. Zero false closures in all seven programmes (CRC 0/473, cervical 0/205, lung 0/298, DR 0/1120, breast 0/68, LDCT 0/280, IFHAS 0/602). `api smoke OK: 5280 cases, 4 fixtures`.
3. `demo.sh serve` + `demo.sh snapshot` with `WASL_SNAPSHOT_DIR=.../samples`: succeeded, 18 files. Compared with committed `docs/api-samples`: same file set (apart from README.md and `agent/`), same fixture ids and tags. Differences are run-dependent only: `loaded_at`, `in_seq` and ids built from it, `throughput_ev_s`, `released_total` 1902 vs 1891, `intents_ever_held_for_consent` 440 vs 442. Not byte-identical.
4. `python3 scripts/test_api_smoke.py`: 28 tests OK. `go test -run 'API|Api|Serve'` with `WASL_TEST_RUN=<out7 lane>`: ok.
5. Missing-asset check: with no nats-server on PATH and no bin/, `demo.sh check` prints `MISSING: nats-server v2.10+` plus install instructions (precise, fails early).

## Defect found and fixed (test-first)

The regeneration recipe in docs/api-samples/README.md exported `WASL_IFHAS=800`, which the same README says panics the generator. It also did not name the nats-server requirement or say that regenerated files are not byte-identical. New test `scripts/test_api_samples_readme.py` failed (3 failures), then the README was fixed (IFHAS=700, nats-server note, non-identical note) and the test passes. No code, checks or metrics changed.

## Limitations

- Single machine and one seed (2026); nats-server was an existing local binary, not downloaded.
- Committed samples were not reproduced byte for byte (timing-dependent fields above).
- Live reader (Gemma GGUF), Docker/Podman deployment, ltt and final evaluation were not run.
- The full Go suite ran once, inside `demo.sh all`; it was not repeated separately.
