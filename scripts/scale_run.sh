#!/usr/bin/env bash
# Scale run (pipeline/results/scale/): the normal-size five-programme cohort or a ~10x one, chaos on, cached reader,
# FHIR source, with peak RSS per process and JetStream store size sampled by scripts/scale_monitor.py.
# Usage: WASL_PORT_OFFSET=<n> scripts/scale_run.sh normal|x10 <scratch dir> [results dir]
#   normal: demo defaults (6,500 eval persons, 1,500 CRC, 600 cervical) + 800 lung, 1,200 DR, 150 breast (~41k messages)
#   x10:    every person split (64,394 persons, training persons included: NOT a held-out accuracy estimate),
#           15,000 CRC, 6,000 cervical, 8,000 lung, 12,000 DR, 400 breast (breast is capped by the women pool), 7,000 distractors
# WASL_SCALE_TAMPER=0|1: run the tamper check (default 1 for normal, 0 for x10: it replays the whole run 17 times).
# Writes <results dir>/<label>/: summary.txt, report7.json, orchestrate.json, rss.json, timing.json, tamper.txt;
# unexpected linker exits also retain a sanitized linker-restarts.log for the separate safety investigation.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LABEL="${1:?normal|x10}"; SCR="${2:?scratch dir}"; RES="${3:-$ROOT/pipeline/results/scale}"
export WASL_OUT="$SCR/$LABEL" WASL_LUNG=800 WASL_DR=1200 WASL_BREAST=150 WASL_LUNGSURV=1 WASL_DRNEXT=1 WASL_XREFLINK=1 WASL_REFREJECT=true
if [ "$LABEL" = x10 ]; then
  export WASL_SPLIT=all WASL_PERSONS=0 WASL_LOOPS=15000 WASL_DISTRACT=7000 WASL_CERV=6000 WASL_LUNG=8000 WASL_DR=12000 WASL_BREAST=400
  export WASL_CHAOSSPAN="${WASL_CHAOSSPAN:-150}" WASL_QUIETTIMEOUT="${WASL_QUIETTIMEOUT:-60m}"
fi
RUN="$WASL_OUT/run/v7strict-cached-fhir-lung-dr-breast-drnext-lungsurv-xreflink"
D="$RES/$LABEL"; mkdir -p "$D" "$WASL_OUT"
now() { python3 -c 'import time;print(time.time())'; }
load() { sysctl -n vm.loadavg 2>/dev/null || cat /proc/loadavg; }
L0="$(load)"
"$ROOT/scripts/demo.sh" check
t0=$(now); "$ROOT/scripts/demo.sh" cohort > "$WASL_OUT/cohort.log" 2>&1; t1=$(now)
mkdir -p "$RUN"
python3 "$ROOT/scripts/scale_monitor.py" "$RUN" "$WASL_OUT/rss.json" 1 & MON=$!
trap 'kill $MON 2>/dev/null || true' EXIT
"$ROOT/scripts/demo.sh" run > "$WASL_OUT/run.log" 2>&1; t2=$(now)
"$ROOT/scripts/demo.sh" audit > /dev/null 2>&1; t3=$(now)
if [ "${WASL_SCALE_TAMPER:-$([ "$LABEL" = x10 ] && echo 0 || echo 1)}" = 1 ]; then   # x10: off by default (17 full replays)
  "$ROOT/scripts/demo.sh" tamper > "$WASL_OUT/tamper.txt" 2>&1
else echo "tamper: not run (WASL_SCALE_TAMPER=0)" > "$WASL_OUT/tamper.txt"; fi; t4=$(now)
"$ROOT/scripts/demo.sh" report > "$WASL_OUT/summary.txt" 2>&1; t5=$(now)
kill -TERM $MON; wait $MON || true; trap - EXIT
L1="$(load)"
cp "$WASL_OUT/rss.json" "$D/rss.json"; cp "$WASL_OUT/summary.txt" "$WASL_OUT/tamper.txt" "$D/"
python3 - "$RUN/report7.json" "$D/report7.json" <<'EOF'
import json, os, sys
r = json.load(open(sys.argv[1]))
r["run"] = "./run/" + os.path.basename(r.get("run", "").rstrip("/"))   # no local paths in committed results
json.dump(r, open(sys.argv[2], "w"), indent=1)
EOF
python3 - "$RUN/orchestrate.json" "$D/orchestrate.json" <<'EOF'
import json, sys
o = json.load(open(sys.argv[1]))
for k in ("linker", "reviewer", "fhirsub", "malaffi", "closures"):   # live status snapshots: large, and may hold paths
    o.pop(k, None)
for ev in o.get("chaos", []):
    ev.pop("status_before_kill", None)
json.dump(o, open(sys.argv[2], "w"), indent=1, default=str)
EOF
python3 - "$RUN/logs/linker.log" "$D/linker-restarts.log" "$D/orchestrate.json" <<'EOF'
import json, os, re, sys
src, dst, orchestrate = sys.argv[1:]
o = json.load(open(orchestrate))
n = o.get("supervisor", {}).get("linker", {}).get("unexpected_exits_restarted", 0)
if not n:
    if os.path.exists(dst):
        os.remove(dst)
    raise SystemExit(0)
text = open(src).read()
text = re.sub(r"/Users/\S+?/pipeline/", "<worktree>/pipeline/", text)
text = re.sub(r"/opt/homebrew/Cellar/go/\S+/libexec/", "<go>/", text)
text = re.sub(r"/(?:Users|home)/[^\s:]+", "<local>", text)
with open(dst, "w") as f:
    f.write("# Sanitized linker log: %d unexpected supervisor restarts; original retained in scratch.\n" % n)
    f.write(text)
EOF
python3 - "$D/timing.json" "$LABEL" "$t0" "$t1" "$t2" "$t3" "$t4" "$t5" "$L0" "$L1" "$(sysctl -n hw.ncpu 2>/dev/null || nproc)" "$(sysctl -n hw.memsize 2>/dev/null || echo 0)" <<'EOF'
import json, sys
a = sys.argv
t = [float(x) for x in a[3:9]]
json.dump({"label": a[2], "cohort_s": round(t[1] - t[0], 1), "orchestrate_s": round(t[2] - t[1], 1), "audit_s": round(t[3] - t[2], 1),
           "tamper_s": round(t[4] - t[3], 1), "report_s": round(t[5] - t[4], 1), "total_s": round(t[5] - t[0], 1),
           "loadavg_start": a[9], "loadavg_end": a[10], "cpus": int(a[11]), "mem_gb": round(int(a[12]) / 2**30, 1)},
          open(a[1], "w"), indent=1)
EOF
grep -rl "/[U]sers/\|/[h]ome/" "$D" && { echo "local paths leaked into $D"; exit 1; } || true
cat "$D/summary.txt"; cat "$D/timing.json"
