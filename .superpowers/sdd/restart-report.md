# Retained-volume restart verification (7 Nov 2026)

Branch feat/podman-retained-restart from 3c99b05. Podman 5.8.2, Docker Compose v5.1.4 provider. Env files in /tmp (mode 600), generated keys, port 18931.

## Commands
- `podman compose -p waslrestart7 --env-file env1 up -d --build` -> NATS healthy x3, pipeline exit 0, API healthy; `scripts/api_smoke.py --base-url http://127.0.0.1:18931` OK (2100 cases, 3 fixtures). report7: duplicate_outputs 0, ingest_lost 0, false closures 0 (CRC with review 0/474, cervical 0/204).
- `down` (no -v): 4 volumes kept.
- `up -d` with another valid key (envbad): DEFECT. nats-server started, "unable to recover keys", "Resetting stream cluster state", "duplicate assignment" for all streams. Retained data lost to the cluster.
- Fix test-first: `scripts/test_deploy.py` new tests (failed first), then `deploy/nats/entrypoint.sh` + `deploy/nats/Dockerfile` ENTRYPOINT. Tests: 12 OK (with pyyaml venv); `check_deploy.py` OK.
- waslrestart8 (fixed): up --build OK, smoke OK; `down`; wrong key -> all nats exit 1 with refusal, no decrypt warnings, volumes kept; missing key -> Compose interpolation error, nothing created; original key `up -d` -> healthy, `streams ready, replicas 3`, pipeline exit 0, API healthy, smoke OK, 2100 cases, CRC R 0.837 0/474, cervical R 0.967 0/204, duplicates 0 / 25169, lost 0, mismatches 0, 0 decryption warnings.
- Cleanup: `down -v` on waslrestart7 and 8 only; 0 containers/volumes remain. No prune, no Supabase.

## Concerns
- The one-shot pipeline resets streams and re-runs the cohort on every `up`; so "2100 cases after restart" is a fresh run, not proof that old JetStream message data was read back. Key survival/protection is what is proven.
- Marker only guards volumes created after the fix; legacy volumes start unverified (logged).
- Marker is a domain-prefixed SHA-256 of the key on the volume (offline guessing possible only for low-entropy keys; generated keys are 256-bit).
- Docker Engine still unverified; deploy_up.sh is Docker-only. Host python lacks pyyaml, so check_deploy/tests need it installed (used a /tmp venv).
- First build took about 7 minutes (cold cache).
