#!/usr/bin/env python3
"""Checks behind scripts/verify.sh (stdlib only). Each subcommand prints PASS/FAIL/SKIP lines and exits 1 on a FAIL.

  sums write|check     SHA-256 of every committed file under pipeline/results/final (pipeline/results/final/SHA256SUMS)
  inputs               model/data hashes in manifest.json against the files at the frozen code revision (git)
  recompute            re-run scripts/final_aggregate.py on a copy of the per-seed results; compare with summary.json
  evidence             web/data/evidence.json against summary.json (skipped when the file is absent)
  gotests <log> <json> per-claim result of the named Go tests in a `go test -v` log
  seed <run dir> <seed> <noise> <rev> <crc> <cerv>   score a judge-seed run (report7.json + tamper.json) and print the table
"""
import glob
import hashlib
import json
import math
import os
import shutil
import statistics
import subprocess
import sys
import tempfile

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
FINAL = "pipeline/results/final"
SUMS = FINAL + "/SHA256SUMS"
FAILED = []


def say(status, claim, detail=""):
    print(f"{status:4s}  {claim}" + (f"  ({detail})" if detail else ""), flush=True)
    if status == "FAIL":
        FAILED.append(claim)


def sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for b in iter(lambda: f.read(1 << 20), b""):
            h.update(b)
    return h.hexdigest()


def final_files():
    """Every file under pipeline/results/final except SHA256SUMS itself (tracked files when git is available)."""
    try:
        out = subprocess.run(["git", "-C", ROOT, "ls-files", "-z", FINAL], capture_output=True, check=True).stdout.decode()
        files = [p for p in out.split("\0") if p]
    except (OSError, subprocess.CalledProcessError):
        files = []
    if not files:
        files = [os.path.relpath(os.path.join(d, n), ROOT) for d, _, ns in os.walk(os.path.join(ROOT, FINAL)) for n in ns]
    return sorted(p for p in files if p != SUMS)


def cmd_sums(mode):
    path = os.path.join(ROOT, SUMS)
    if mode == "write":
        with open(path, "w") as f:
            for p in final_files():
                f.write(f"{sha256(os.path.join(ROOT, p))}  {p}\n")
        print(f"wrote {SUMS} ({len(final_files())} files)")
        return
    claim = "Frozen evaluation artifacts are byte-identical to the committed SHA-256 list"
    if not os.path.exists(path):
        return say("FAIL", claim, f"{SUMS} missing")
    with open(path) as f:
        want = {p: h for h, p in (line.rstrip("\n").split("  ", 1) for line in f)}
    bad = [p for p, h in want.items() if not os.path.exists(os.path.join(ROOT, p)) or sha256(os.path.join(ROOT, p)) != h]
    extra = [p for p in final_files() if p not in want]
    if bad or extra:
        return say("FAIL", claim, f"changed or missing: {bad[:5]}; not listed: {extra[:5]}")
    # summary.json and manifest.json are in the list, so the frozen metrics and the frozen thresholds are pinned too
    say("PASS", claim, f"{len(want)} files incl. summary.json, manifest.json and 10 per-seed report7/tamper sets")


def cmd_inputs():
    man = json.load(open(os.path.join(ROOT, FINAL, "manifest.json")))
    rev = man["code_revision"]
    claim = f"Frozen model/data inputs match manifest.json hashes at code revision {rev[:12]}"
    if subprocess.run(["git", "-C", ROOT, "cat-file", "-e", rev + "^{commit}"], capture_output=True).returncode:
        return say("SKIP", claim, "commit not in this clone; run `git fetch --unshallow` for full history")
    bad = []
    for p, h in man["hashes"].items():
        blob = subprocess.run(["git", "-C", ROOT, "show", f"{rev}:{p}"], capture_output=True)
        if blob.returncode or hashlib.sha256(blob.stdout).hexdigest() != h:
            bad.append(p)
    if bad:
        return say("FAIL", claim, f"mismatch: {bad}")
    drift = [p for p, h in man["hashes"].items() if not os.path.exists(os.path.join(ROOT, p)) or sha256(os.path.join(ROOT, p)) != h]
    say("PASS", claim, f"{len(man['hashes'])} files" + (f"; note: changed since the freeze in this checkout: {', '.join(drift)}" if drift else ""))


def diff(rec, com, path=""):
    """Paths where a committed value differs from (or is missing in) the recomputed one. Extra recomputed keys are returned apart."""
    bad, extra = [], []
    if isinstance(com, dict) and isinstance(rec, dict):
        for k in com:
            if k not in rec:
                bad.append(f"{path}/{k} missing")
            else:
                b, e = diff(rec[k], com[k], f"{path}/{k}")
                bad += b; extra += e
        extra += [f"{path}/{k}" for k in rec if k not in com]
    elif isinstance(com, list) and isinstance(rec, list) and len(com) == len(rec):
        for i, (r, c) in enumerate(zip(rec, com)):
            b, e = diff(r, c, f"{path}[{i}]")
            bad += b; extra += e
    elif rec != com:
        bad.append(f"{path}: recomputed {rec!r} vs committed {com!r}")
    return bad, extra


def cmd_recompute():
    claim = "summary.json (means, 95% intervals, pooled safety counts, Clopper-Pearson bounds) recomputes exactly from the per-seed results"
    tmp = tempfile.mkdtemp(prefix="wasl-verify-")
    try:
        shutil.copytree(os.path.join(ROOT, FINAL), os.path.join(tmp, "final"))
        os.remove(os.path.join(tmp, "final", "summary.json"))
        os.remove(os.path.join(tmp, "final", "SUMMARY.md"))
        p = subprocess.run([sys.executable, os.path.join(ROOT, "scripts", "final_aggregate.py"), os.path.join(tmp, "final")],
                           cwd=os.path.join(ROOT, "scripts"), capture_output=True, text=True)
        if p.returncode:
            return say("FAIL", claim, "final_aggregate.py failed: " + p.stderr.strip().splitlines()[-1])
        rec = json.load(open(os.path.join(tmp, "final", "summary.json")))
    finally:
        shutil.rmtree(tmp, ignore_errors=True)
    com = json.load(open(os.path.join(ROOT, FINAL, "summary.json")))
    bad, extra = diff(rec, com)
    if bad:
        return say("FAIL", claim, f"{len(bad)} differences, first: {bad[:3]}")
    n = sum(len(v["metrics"]) + len(v["safety_pooled"]) for v in com["by_noise"].values())
    note = f"scripts/final_aggregate.py, {n} metric and safety rows over v1+v2"
    if extra:  # the aggregator has since gained a label naming the closure-safety loss; it is not in the frozen file
        note += f"; recomputation also adds {', '.join(sorted(set(e.rsplit('/', 1)[-1] for e in extra)))} (a label, not a number)"
    say("PASS", claim, note)


def t_interval(xs):
    """Mean and 95% t-interval over seeds, exactly as scripts/final_aggregate.py ci()."""
    t95 = {1: 12.706, 2: 4.303, 3: 3.182, 4: 2.776, 5: 2.571, 6: 2.447, 7: 2.365, 8: 2.306, 9: 2.262}
    m, sd = statistics.mean(xs), statistics.stdev(xs)
    h = t95.get(len(xs) - 1, 2.0) * sd / math.sqrt(len(xs))
    return [round(m, 4), round(m - h, 4), round(m + h, 4)]


def cmd_evidence():
    claim = "web/data/evidence.json numbers equal summary.json"
    path = os.path.join(ROOT, "web", "data", "evidence.json")
    if not os.path.exists(path):
        return say("SKIP", claim, "web/data/evidence.json not present in this checkout")
    ev, s = json.load(open(path)), json.load(open(os.path.join(ROOT, FINAL, "summary.json")))
    man, bad, n = s["manifest"], [], 0
    g = ev.get("generated", {})
    if g.get("revision") != man["code_revision"] or g.get("frozen_at") != man["frozen_at"]:
        bad.append("generated.revision/frozen_at differ from the manifest")
    r4 = lambda x: None if x is None else round(x, 4)
    cands = {nz: list(v["metrics"].values()) for nz, v in s["by_noise"].items()}
    for nz in cands:  # pair precision/recall are per-seed report7.json values (hash-pinned) that summary.json does not pool
        reps = [json.load(open(os.path.join(d, "report7.json"))) for d in sorted(glob.glob(os.path.join(ROOT, FINAL, f"{nz}-s*")))]
        cands[nz] += [t_interval([x["linkage"][k] for x in reps]) for k in ("pair_precision", "pair_recall")]
    for row in ev.get("accuracy", {}).get("linkage", []):  # a summary metric for that noise, as is or as 1 - x
        n += 1
        got = (r4(row["value"]), r4(row["low"]), r4(row["high"]))
        ok = any(got in ((r4(m), r4(lo), r4(hi)), (r4(1 - m), r4(1 - hi), r4(1 - lo)))
                 for m, lo, hi in cands[row["noise"]] if None not in (m, lo, hi))
        ok or bad.append(f"linkage {row['noise']} '{row['metric']}' {got}")
    pooled = {tuple(v) for nz in s["by_noise"].values() for v in nz["safety_pooled"].values()}
    pooled |= {(v[1] - v[0], v[1], None) for nz in s["by_noise"].values() for v in nz["safety_pooled"].values() if v[1]}  # "not detected" = n - k
    for nz in s["by_noise"]:  # linker false-merge pairs, summed over the per-seed report7.json files
        reps = glob.glob(os.path.join(ROOT, FINAL, f"{nz}-s*", "report7.json"))
        pooled.add((sum(json.load(open(x))["linkage"]["false_merge_pairs"] for x in reps), None, None))
    for row in ev.get("accuracy", {}).get("safety", []):  # events, denominator and upper bound as one pooled row
        n += 1
        if (row["events"], row["denominator"], row["upper_95"]) not in pooled:
            bad.append(f"safety '{row['metric']}' {(row['events'], row['denominator'], row['upper_95'])}")
    for row in ev.get("impact", {}).get("per_1000", []):  # the 31-day KPI rows of each frozen noise, x 1000
        for nz in ("v1", "v2"):
            if row["row"].startswith("Completed colonoscopy within 31 days") and f"frozen {nz}" in row["row"]:
                n += 1
                m = s["by_noise"][nz]["metrics"]
                want = tuple(round(1000 * m[k][0], 1) for k in ("kpi_eid_only", "kpi_wasl", "kpi_truth"))
                got = tuple(round(row[k], 1) for k in ("eid_only", "wasl", "truth"))
                want == got or bad.append(f"per_1000 {nz} {got} vs {want}")
    gen = os.path.join(ROOT, "scripts", "evidence.py")
    if os.path.exists(gen):  # the generator's own drift check: the committed file is what summary.json produces now
        p = subprocess.run([sys.executable, gen, "--check"], cwd=ROOT, capture_output=True, text=True)
        n += 1
        p.returncode == 0 or bad.append("scripts/evidence.py --check reports drift: " + (p.stdout + p.stderr).strip()[-200:])
    if bad:
        return say("FAIL", claim, f"{len(bad)} mismatches, first: {bad[:3]}")
    say("PASS", claim, f"{n} checks: linkage rows, pooled safety rows, 31-day KPI per 1,000" + (", evidence.py --check" if os.path.exists(gen) else ""))


def cmd_gotests(log, claims_json):
    status = {}
    for line in open(log, errors="replace"):
        for st in ("PASS", "FAIL", "SKIP"):
            if line.startswith(f"--- {st}: "):
                status[line.split()[2]] = st
    for claim, tests in json.load(open(claims_json)).items():
        miss = [t for t in tests if status.get(t) != "PASS"]
        say("FAIL" if miss else "PASS", claim, (f"not passing: {', '.join(f'{t}={status.get(t, 'not run')}' for t in miss)}" if miss
                                               else f"{len(tests)} Go tests: {', '.join(tests)}"))


def cp_upper(k, n, conf=0.975):
    """Clopper-Pearson one-sided 97.5% upper bound (two-sided 95%), as scripts/final_aggregate.py (scipy beta.ppf)."""
    if n == 0:
        return None
    if k >= n:
        return 1.0
    if k == 0:
        return round(1 - (1 - conf) ** (1 / n), 5)

    def cdf(p):  # P(X <= k), X ~ Binomial(n, p)
        return sum(math.exp(math.lgamma(n + 1) - math.lgamma(i + 1) - math.lgamma(n - i + 1) + i * math.log(p) + (n - i) * math.log1p(-p))
                   for i in range(k + 1))
    lo, hi = k / n, 1.0
    for _ in range(100):
        mid = (lo + hi) / 2
        lo, hi = (mid, hi) if cdf(mid) > 1 - conf else (lo, mid)
    return round(hi, 5)


def cmd_seed(run, seed, noise, rev, crc, cerv):
    r = json.load(open(os.path.join(run, "report7.json")))
    t = json.load(open(os.path.join(run, "tamper.json")))
    ref = json.load(open(os.path.join(ROOT, FINAL, "summary.json")))["by_noise"].get(noise, {})
    rm, rs = ref.get("metrics", {}), ref.get("safety_pooled", {})
    pct = lambda x: "–" if x is None else f"{100 * x:.3g}%"
    reps = [json.load(open(x)) for x in sorted(glob.glob(os.path.join(ROOT, FINAL, f"{noise}-s*", "report7.json")))]
    for k in ("pair_precision", "pair_recall"):  # per-seed report7.json values, same t-interval as the aggregator
        m, lo, hi = t_interval([x["linkage"][k] for x in reps]) if reps else (None, None, None)
        if m is not None:
            rm["1-" + k] = [round(1 - m, 4), round(1 - hi, 4), round(1 - lo, 4)]
    frz = lambda k: (f"{rm[k][0]:.4g} [{rm[k][1]:.4g}, {rm[k][2]:.4g}]" if k in rm else "–")
    print(f"\nJudge-seed run: seed {seed}, noise {noise}, code revision {rev[:12]}, {crc} CRC + {cerv} cervical loops "
          f"({r['loops']} loops scored), all held-out persons, policy v7strict, chaos on.\n"
          f"Scored against the generator's truth file. Frozen column: the committed five-seed {noise} result "
          f"(2,040 CRC + 816 cervical loops per seed plus lung, DR, breast, LDCT and IFHAS loops), mean [95% interval].\n")
    print(f"{'Accuracy':48s} {'this seed':>22s}   frozen {noise}")
    rows = []
    for prog in ("crc", "cervical"):
        c = r["closure"][prog]["wasl_live_with_review"]
        rows += [(f"{prog} closure precision", f"{c['precision']:.3f}", frz(f"{prog}_precision")),
                 (f"{prog} closure recall (missed = 1 - recall)", f"{c['recall']:.3f}", frz(f"{prog}_recall"))]
    lk = r["linkage"]
    rows += [("identity pairs: false-match rate (1 - pair precision)", f"{1 - lk['pair_precision']:.4f}", frz("1-pair_precision")),
             ("identity pairs: missed-match rate (1 - pair recall)", f"{1 - lk['pair_recall']:.4f}", frz("1-pair_recall")),
             ("identity pairs: F1", f"{lk['pair_f1']:.3f}", frz("linkage_pair_f1"))]
    k = r["crc_kpi_31d"]
    for name, key, fk in (("CRC 31-day KPI, EID-only matching", "eid_only", "kpi_eid_only"), ("CRC 31-day KPI, Wasl", "wasl_live", "kpi_wasl"),
                          ("CRC 31-day KPI, simulated truth", "truth", "kpi_truth")):
        rows.append((name, f"{k[key]['kpi_all_referrals']:.3f}", frz(fk)))
    for a, b, c in rows:
        print(f"{a:48s} {b:>22s}   {c}")
    print(f"\n{'Safety (events / denominator, 95% upper bound)':48s} {'this seed':>22s}   frozen {noise} pooled")
    ol, ct, f, cr = r["outreach_live"], r["consent_timing"], r["forged"], r["crash_recovery"]
    att = [a for a in t["attacks"] if a["attacker"] != "-"]
    det, fa, nf = sum(a["detected"] for a in att), sum(a["detected"] for a in t["attacks"] if a["attacker"] == "-"), sum(a["attacker"] == "-" for a in t["attacks"])
    gates = []
    for prog in ("crc", "cervical"):
        c = r["closure"][prog]["wasl_live_with_review"]
        gates.append((f"{prog} false closures", c["false_closures"], c["closed_pred"], f"{prog}_false_closures", c["false_closures"] == 0))
    wrong = sum(v["wrong_released"] for v in ol["by_origin"].values())
    gates += [("wrong-patient messages released", wrong, sum(v["intents"] for v in ol["by_origin"].values()), "wrong_patient_released", wrong == 0),
              ("released without granted consent", ct.get("released_without_granted_consent", 0), ct.get("released_total", 0),
               "released_without_granted_consent", ct.get("released_without_granted_consent", 0) == 0),
              ("forged events with any effect", f["forged_with_any_effect"], f["forged_events"], "forged_with_effect", f["forged_with_any_effect"] == 0),
              ("duplicate or lost outputs", cr["duplicate_outputs"] + cr.get("lost_outputs", 0), cr["expected_outputs"], "duplicate_or_lost_outputs",
               cr["duplicate_outputs"] + cr.get("lost_outputs", 0) == 0),
              ("tamper attacks detected", det, len(att), "tamper_detected", det == len(att) and len(att) > 0),
              ("tamper false alarms on the untouched run", fa, nf, "tamper_false_alarms", fa == 0)]
    for name, ev, den, key, ok in gates:
        ub = cp_upper(ev, den) if ev == 0 and "tamper" not in name else None
        fr = rs.get(key)
        frs = f"{fr[0]}/{fr[1]}" + (f", <= {pct(fr[2])}" if fr and fr[2] is not None and "tamper" not in key else "") if fr else "–"
        print(f"{name:48s} {f'{ev}/{den}' + (f', <= {pct(ub)}' if ub is not None else ''):>22s}   {frs}")
    print("\nUpper bounds are exact Clopper-Pearson (one-sided 97.5%, about 3.7/n when there are zero events). "
          "A few hundred loops give wide bounds: zero false closures in ~100 closures only bounds the rate below ~3.6%; "
          "the frozen run pools ~3,000 closures per noise model.\n")
    for name, ev, den, _, ok in gates:
        say("PASS" if ok else "FAIL", f"seed {seed}: {name} = {ev}/{den}")


def main(argv):
    cmd = argv[0] if argv else ""
    if cmd == "sums":
        cmd_sums(argv[1])
    elif cmd == "inputs":
        cmd_inputs()
    elif cmd == "recompute":
        cmd_recompute()
    elif cmd == "evidence":
        cmd_evidence()
    elif cmd == "gotests":
        cmd_gotests(argv[1], argv[2])
    elif cmd == "seed":
        cmd_seed(*argv[1:7])
    else:
        print(__doc__)
        return 2
    return 1 if FAILED else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
