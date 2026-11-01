#!/usr/bin/env python3
"""Sample peak RSS per Wasl process and JetStream store size during a run (scale runs, pipeline/results/scale/).

Usage: scale_monitor.py <run dir> <out.json> [interval_s]
Runs until SIGTERM/SIGINT, then writes <out.json>. Processes are matched by the run dir in their command line
(wsi7 children carry `-run <dir>`, nats-server carries `-c <dir>/nats-N/nats.conf`) and named by role only, so the
output contains no local paths. NATS storage is the size of <run dir>/nats-*/ (the orchestrator's local cluster).
"""
import json, os, re, signal, subprocess, sys, time

run, out = os.path.abspath(sys.argv[1]), sys.argv[2]
interval = float(sys.argv[3]) if len(sys.argv) > 3 else 1.0
peak, samples, store_peak, store_last = {}, 0, 0, 0
total_peak = 0
loads, series = [], []   # series: [unix time, host 1-min load] every 10 samples
t0 = time.time()


def role(cmd):
    if "nats-server" in cmd:
        m = re.search(r"nats-(\d+)/nats\.conf", cmd)
        return "nats-%s" % (m.group(1) if m else "?")
    parts = cmd.split()
    sub = parts[1] if len(parts) > 1 else "?"
    for flag in ("-part", "-name", "-prov"):
        if flag in parts:
            i = parts.index(flag)
            if i + 1 < len(parts):
                return "%s-%s" % (sub, parts[i + 1])
    return sub


def du(path):
    n = 0
    for root, _, files in os.walk(path):
        for f in files:
            try:
                n += os.lstat(os.path.join(root, f)).st_size
            except OSError:
                pass
    return n


def finish(*_):
    res = {"interval_s": interval, "samples": samples, "duration_s": round(time.time() - t0, 1),
           "peak_rss_mb": {k: round(v / 1024, 1) for k, v in sorted(peak.items())},
           "peak_rss_total_mb": round(total_peak / 1024, 1),
           "nats_store_peak_mb": round(store_peak / 1e6, 1), "nats_store_last_mb": round(store_last / 1e6, 1),
           "host_load1_mean": round(sum(loads) / len(loads), 1) if loads else None,
           "host_load1_max": round(max(loads), 1) if loads else None, "host_load1_series": series}
    with open(out, "w") as f:
        json.dump(res, f, indent=1)
    sys.exit(0)


signal.signal(signal.SIGTERM, finish)
signal.signal(signal.SIGINT, finish)
k = 0
while True:
    try:
        ps = subprocess.run(["ps", "-axo", "pid=,rss=,command="], capture_output=True, text=True).stdout
    except Exception:
        ps = ""
    tot = 0
    for line in ps.splitlines():
        f = line.split(None, 2)
        if len(f) < 3 or run not in f[2]:
            continue
        if "scale_monitor" in f[2] or not ("wsi7" in f[2] or "nats-server" in f[2]):
            continue
        r, rss = role(f[2]), int(f[1])
        peak[r] = max(peak.get(r, 0), rss)
        tot += rss
    total_peak = max(total_peak, tot)
    loads.append(os.getloadavg()[0])   # whole host (other jobs included): context for the throughput figures
    if k % 10 == 0:
        series.append([round(time.time(), 1), round(loads[-1], 1)])
    if k % 5 == 0:
        try:
            stores = [os.path.join(run, d) for d in os.listdir(run) if d.startswith("nats-")]
        except OSError:
            stores = []
        if stores:
            store_last = sum(du(s) for s in stores)
            store_peak = max(store_peak, store_last)
    samples += 1
    k += 1
    time.sleep(interval)
