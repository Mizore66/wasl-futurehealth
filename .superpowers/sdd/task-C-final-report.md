# Task C final report

Branch feat/crc-podman-final (worktree agent-a1506f99db60bf184): detached at 25162af, merged feat/crc-api-smoke-final (876a6a6, includes A/B) as 91706d0. 7bd46ac is an ancestor (verified with merge-base --is-ancestor).

## Hardening (TDD)
- Added 5 tests to scripts/test_deploy.py (check_deploy.py gains a WASL_COMPOSE_FILE override so tests run it on mutated compose copies). Red: 3 failures (short-syntax and long-syntax NATS bind accepted by mutation tests; one test I then re-scoped).
- check_deploy.py: new vol_parts() handles short and long syntax; NATS bind rejection and bind-source existence check use it. No existing check removed.
- Green: `scripts/test_deploy.py` 10 tests OK (PyYAML in temp venv); `check_deploy.py` OK.

## Podman run (project waslfinal1793973723, generated k+64 hex key, loopback port 18817)
- `podman compose up -d --build`: exit 0; wasl:local and wasl-nats:local built (COPY nats.conf step observed).
- nats-0/1/2 healthy; pipeline Exited (0); api healthy; `id -u` = 10001.
- /healthz: status ok, dependencies.nats=up, materialised_run loaded, reader cached, ingest fhir, 2100 cases, simulated.
- Pipeline log: audit, FHIR log tamper lines (fired D4_replay D5_fhir_anchor), forged effect 0/18, provenance 912/912, simulated booking (CRC 814, cervical 314), summary CRC with review P 1.000 R 0.837 0/474 false closures, exactly-once duplicates 0/25188.
- Only publication: 127.0.0.1:18817->8088 (PortBindings); nats ports unpublished.
- `python3 scripts/api_smoke.py --base-url http://127.0.0.1:18817`: "api smoke OK: 2100 cases, 3 fixtures checked", exit 0.
- Cleanup: `down -v` on that project only; remaining container is the unrelated Supabase one (untouched). No global prune.

## Concerns
- Docker Engine and retained-volume restart remain unverified (documented as such).
- Single-seed R 0.837 not claimed as a headline.
- Tamper evidence appears as "FHIR log ... fired=" lines.
- API healthcheck was still "starting" at first inspection, healthy shortly after.
- Podman retains the JS key in container environment on the VM host.
- Static checks need PyYAML (not installed system-wide).
