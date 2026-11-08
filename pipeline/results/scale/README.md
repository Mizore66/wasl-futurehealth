# Scale runs: normal versus approximately 10×

## Post-hardening validation

Fresh normal and x10 runs completed at decision code `e27eefc`; their sanitized artifacts were committed at `075abc5`.
Both passed the measured safety and replay checks:

| Metric | Normal | x10 |
|---|---:|---:|
| Unexpected linker restarts | 0 | 0 |
| CRC false closures, live / no-review | 0/484 / 0/175 | 0/4,741 / 0/1,868 |
| Cervical false closures, live / no-review | 0/203 / 0/117 | 0/1,983 / 0/1,054 |
| Lung / DR / breast false closures | 0/299 / 0/1,121 / 0/144 | 0/3,021 / 0/11,359 / 0/386 |
| Wrong-patient releases | 0 | 0 |
| Releases without granted consent | 0 | 0 |
| Duplicate / lost outputs | 0/31,087 | 0/310,662 |
| Live-versus-replay final-state mismatches | 0 | 0 |
| Provenance chains verified | 3,162/3,162 | 30,567/30,567 |
| Forged events with effect | 0/19 | 0/24 |
| Throughput | 708 events/s | 858 events/s |
| Reviewer time | 13.3 h/1k loops | 13.7 h/1k loops |
| Total measured wall time | 519.8 s | 5,436.7 s |

Artifacts are in [`normal-hardened/`](normal-hardened/) and [`x10-hardened/`](x10-hardened/). These are single-seed
simulated runs on a shared host, not controlled capacity benchmarks. The x10 cohort includes training persons, so its
accuracy is not held-out evidence. Its tamper replay remained off; the forged-event counter above is the integrated
RBAC check, not 24 successful tamper replays.

The fixes were post-hoc and followed observed failures: linker status snapshotting, EID-conflict and cluster-bridge
handling, placeholder-DOB corroboration, and durable cannot-link constraints after EID-clash refusals. The
placeholder-DOB rule did not trigger a calibrator refit. The final x10 run recorded 2,374 `eid_cluster_clash` events
and 95 `cannot_rejects`. No threshold or guard default was changed.

Earlier evidence is retained below and in `normal/` and `x10/`: the original x10 had 80 unexpected linker restarts,
two false closures and four wrong-patient releases; a stale-binary rerun had 43 unexpected restarts, three live false
closures and four wrong-patient releases and must not be attributed to the fixed sources; the first correctly rebuilt
rerun had zero crashes and false closures but still one wrong-patient release. Root-cause analysis showed that an EID-clash refusal could be lost after a wrong simulated
reviewer split. The durable cannot-link fix addressed that mechanism, and the fresh rerun above then measured zero
wrong-patient releases. These later zero counts do not erase any historical failure.

## Historical baseline and failure analysis

**The historical x10 run is a failed safety result, not a release candidate.** With chaos on it produced one CRC false
closure, one cervical false closure and four released wrong-patient messages. It also had **80 unexpected linker process
restarts**, separate from its one scheduled linker chaos kill; the normal run had zero unexpected linker exits. No
threshold or safety rule was changed after seeing these results. The normal-size comparison had zero for all three
primary safety measures.

Both historical runs used seed 2026, `v7strict`, cached evidence/image readers, mock FHIR Subscription ingest, three prep workers,
four closure partitions, three NATS nodes and the standard kill/restart chaos cycles. Full sanitized reports are in
[`normal/`](normal/) and [`x10/`](x10/); `report7.json` is the source for the metrics below.

## Cohorts and execution

| | Normal | x10 |
|---|---:|---:|
| Person selection | 6,500 held-out `eval` persons | all 64,394 persons (`split=all`, including training persons) |
| CRC / cervical / lung / DR / breast loops | 1,500 / 600 / 800 / 1,200 / 150 | 15,000 / 6,000 / 8,000 / 12,000 / 400 |
| Distractors | 700 | 7,000 |
| FHIR messages | 40,786 | 406,143 (9.96×) |
| Output messages | 31,012 | 311,018 (10.03×) |
| Chaos schedule | default 15 s span | 600 s span (`-chaosspan`) |
| Linker scheduled chaos kills / unexpected restarts | 1 / 0 | 1 / **80** |
| Quiescence timeout | default 5 min | 60 min |
| Tamper check | run (17/17 attacks detected) | **not run** (17 full replays were impractical) |

No persons were repeated. Breast was capped at 400 loops: the generator could not satisfy the requested 1,500 from
background women aged 40–90 with no other loop. The x10 cohort therefore expands CRC, cervical, lung and DR by 10×,
but breast by only 2.67×. Because `split=all` includes model-training persons, x10 accuracy is **not held out**.

The x10 run retained zero duplicate/lost outputs and zero replay final-state mismatches, but its safety failures mean
it must not be presented as a successful scale validation. Its four wrong-patient releases were all `p_cal`-basis
appointment reminders with exact DOB matches; the detailed synthetic event IDs remain in `x10/report7.json`.

## Throughput, latency and wall time

| Metric | Normal | x10 |
|---|---:|---:|
| Pipeline wall time | 70.721 s | 3,397.403 s (48.04×) |
| Throughput | 576.72 events/s | 119.55 events/s |
| End-to-end latency p50 | 30.828 s | 363.787 s |
| End-to-end latency p90 | 34.768 s | 3,154.499 s |
| End-to-end latency p99 | 35.381 s | 3,162.541 s |
| End-to-end latency max | 35.676 s | 3,176.129 s |
| Latency sample count | 17,460 | 173,917 |
| Cohort generation | 109.6 s | 3,579.0 s |
| Audit | 206.8 s | 1,128.3 s |
| Tamper | 1,441.4 s | not run |
| Report | 333.2 s | 858.9 s |
| Total measured wall time | 2,185.3 s | 9,003.8 s |

Latency is converted from `report7.json.latency_ms`. Total wall time is cohort + pipeline + audit + optional tamper +
report; it is not directly comparable because x10 omitted tamper. These were single runs on a shared 12-CPU, 32-GB
machine, not controlled capacity benchmarks. The normal run started at load average 25.67 and ended at 298.30; x10
started at 295.32 and ended at 5.65. The x10 monitor saw mean/max 1-minute load 9.3/68.6 after cohort generation.
Other jobs account for much of this load, so the throughput drop must not be attributed to cohort size alone.

## Peak RSS and NATS storage

Peaks are sampled once per second by `scripts/scale_monitor.py`; MB are MiB for RSS and decimal MB for store sizes.
The concurrent total is the maximum sum of live matched processes, not the sum of independent per-role peaks.

| Process / store | Normal peak | x10 peak |
|---|---:|---:|
| `malaffi` | 400.9 MB | 2,867.3 MB |
| `fhirsub` | 125.6 MB | 772.3 MB |
| `linker` | 196.5 MB | 2,296.1 MB |
| `prep-0` / `prep-1` / `prep-2` | 57.5 / 55.5 / 51.4 MB | 82.3 / 82.3 / 83.2 MB |
| `closure-0` / `closure-1` / `closure-2` / `closure-3` | 49.0 / 47.2 / 48.7 / 46.2 MB | 207.7 / 206.4 / 202.6 / 205.4 MB |
| `reviewer` | 39.3 MB | 221.9 MB |
| `nats-0` / `nats-1` / `nats-2` | 175.0 / 183.0 / 136.7 MB | 724.8 / 744.3 / 691.3 MB |
| `orchestrate` | 26.3 MB | 464.8 MB |
| `setup` | 15.8 MB | 15.6 MB |
| `audit` | 777.6 MB | 7,323.5 MB |
| `tamper` | 2,025.9 MB | not run |
| `report7` | 751.1 MB | 7,536.1 MB |
| Concurrent process total | 2,025.9 MB | 7,536.1 MB |
| NATS store, all 3 replicas | 660.5 MB peak / 609.0 MB final | 6,336.3 MB peak / 6,293.5 MB final |

## Exactly-once, replay and every safety count

| Metric | Normal | x10 |
|---|---:|---:|
| Duplicate / lost outputs | 0 / 0 | 0 / 0 |
| Ingest duplicates / lost | 0 / 0 | 0 / 0 |
| Prep duplicates / lost / content mismatches | 0 / 0 / 0 | 0 / 0 / 0 |
| Live-versus-replay final-state mismatches | 0 | 0 |
| Provenance chains verified | 3,160 / 3,160 | 30,598 / 30,598 |
| Forged events with any effect | 0 / 24 | 0 / 24 |
| CRC false closures, live / no-review | 0 / 0 | **1 / 1** |
| Cervical false closures, live / no-review | 0 / 0 | **1 / 1** |
| Lung closure false closures | 0 / 297 closed | 0 / 3,036 closed |
| DR closure false closures | 0 / 1,122 closed | 0 / 11,359 closed |
| DR referral-closure false closures | 0 / 61 closed | 0 / 573 closed |
| Breast closure false closures | 0 / 144 closed | 0 / 386 closed |
| Breast recall-closure false closures | 0 / 8 closed | 0 / 24 closed |
| CRC / cervical prep-query false closures | 0 / 0 | 0 / 0 |
| Wrong-patient messages released | 0 | **4** |
| Messages released without granted consent | 0 | 0 |
| Messages after opt-out / before late opt-out recorded | 0 / 0 | 0 / 0 |
| Consent released / held / cancelled | 1,532 / 324 / 85 | 15,209 / 3,169 / 910 |

## Unexpected linker restarts

The x10 supervisor recorded 80 unexpected linker exits and restarted the process each time, in addition to the one
scheduled chaos kill. The normal run recorded zero. All 80 x10 crashes are `fatal error: concurrent map` failures in
linker status JSON serialization: 42 concurrent map iteration/write and 38 concurrent map read/write. Their stacks pass
through `main.writeStatus` and `main.runLinker.func2`; this is a concurrent map-access defect, not a chaos event. The
full sanitized log is preserved in [`x10/linker-restarts.log`](x10/linker-restarts.log), with the supervisor counts in
[`x10/orchestrate.json`](x10/orchestrate.json); the original log remains in the scale scratch directory for the separate
safety investigation. No race fix was made in this review. These crashes remain a stop-ship finding alongside the false
closures and wrong-patient releases; zero duplicate/lost outputs and zero replay mismatches do not excuse either result.

## Abu Dhabi volume context

`docs/WRITEUP.md` labels these rough emirate volumes as assumptions: **50–110k loops/year** including the weakest
IFHAS scenario, or **20–50k/year without it**. That is approximately 137–301 or 55–137 loops/day respectively. The
component assumptions are incidental imaging 10–30k, DR 6–12k, IFHAS abnormal results 30–60k (a scenario, not a
published estimate), breast recall 2–10k, lung LDCT under 0.5k, and CRC + cervical 2–6k per year.

For an explicitly labelled storage extrapolation only, x10 used 9.81 synthetic FHIR messages/loop and 0.153 MB of
three-replica NATS storage/loop. Carrying those ratios to the assumed 50–110k loops/year gives 0.49–1.08M messages/year
and 7.7–16.8 GB of replicated storage per retained year (about 2.6–5.6 GB/node). Real case mix, payload sizes and
retention will differ; this is not production sizing or a capacity guarantee. The demo's 4 GB/node limit would need
review at the upper end of this synthetic extrapolation.

## Reproduce

```bash
WASL_PORT_OFFSET=<assigned-offset> scripts/scale_run.sh normal <scratch-dir> pipeline/results/scale
WASL_PORT_OFFSET=<assigned-offset> WASL_CHAOSSPAN=600 scripts/scale_run.sh x10 <scratch-dir> pipeline/results/scale
```

Each invocation overwrites its label's result directory. Use separate scratch directories for concurrent runs. Set
`WASL_SCALE_TAMPER=1` to opt into the x10 tamper replay. The committed JSON contains no local absolute paths.
