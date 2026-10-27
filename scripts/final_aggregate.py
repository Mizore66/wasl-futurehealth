"""Aggregate pipeline/results/final/<noise>-s<seed>/ into summary.json + SUMMARY.md (means, 95% t-intervals, Clopper-Pearson UBs)."""
import json, os, sys, glob, math, collections
from statistics import mean, stdev
try:
    from scipy.stats import beta, t as tdist
except ImportError:
    beta = tdist = None
F = sys.argv[1]
man = json.load(open(os.path.join(F, 'manifest.json')))
T95 = {1: 12.706, 2: 4.303, 3: 3.182, 4: 2.776, 5: 2.571, 6: 2.447, 7: 2.365, 8: 2.306, 9: 2.262}


def ci(xs):
    if len(xs) < 2:
        return [round(xs[0], 4), None, None] if xs else [None, None, None]
    m, s = mean(xs), stdev(xs)
    h = T95.get(len(xs) - 1, 2.0) * s / math.sqrt(len(xs))
    return [round(m, 4), round(m - h, 4), round(m + h, 4)]


def cp_ub(k, n):
    if n == 0:
        return None
    if beta is not None:
        return round(float(beta.ppf(0.975, k + 1, n - k)) if k < n else 1.0, 5)
    return round(1 - 0.025 ** (1 / n), 5) if k == 0 else None


out = {"manifest": man, "by_noise": {}}
md = [f"# Final frozen evaluation\n", f"Code revision `{man['code_revision'][:12]}` · policy `{man['policy']}` · seeds {man['seeds']} · source `{man['source']}` · "
      f"held-out persons only · frozen {man['frozen_at']}\n" + (f"\n{man['report_note']} (`{man['report_revision'][:12]}`).\n" if man.get('report_revision') else ""),
      "Means with 95% intervals over seeds (t). Zero-event safety metrics are pooled, with a Clopper–Pearson 95% upper bound.\n"]
for noise in ["v1", "v2"]:
    runs = sorted(glob.glob(os.path.join(F, f"{noise}-s*", "report7.json")))
    if not runs:
        continue
    R = [json.load(open(p)) for p in runs]
    T = [json.load(open(p.replace('report7.json', 'tamper.json'))) for p in runs]
    m = collections.defaultdict(list)
    pool = collections.Counter()
    for r, t in zip(R, T):
        for prog in ("crc", "cervical"):
            c = r['closure'][prog]['wasl_live_with_review']
            m[f"{prog}_precision"].append(c['precision']); m[f"{prog}_recall"].append(c['recall'])
            m[f"{prog}_recall_no_review"].append(r['closure'][prog]['wasl_no_review']['recall'])
            pool[f"{prog}_false_closures"] += c['false_closures']; pool[f"{prog}_closed"] += c['closed_pred']
        k = r['crc_kpi_31d']
        m["kpi_wasl"].append(k['wasl_live']['kpi_all_referrals']); m["kpi_truth"].append(k['truth']['kpi_all_referrals']); m["kpi_eid_only"].append(k['eid_only']['kpi_all_referrals'])
        m["kpi_abs_error_wasl"].append(abs(k['wasl_live']['kpi_all_referrals'] - k['truth']['kpi_all_referrals']))
        m["kpi_abs_error_eid_only"].append(abs(k['eid_only']['kpi_all_referrals'] - k['truth']['kpi_all_referrals']))
        m["linkage_pair_f1"].append(r['linkage']['pair_f1'])
        m["reviewer_h_per_1k"].append(r['reviewer']['by_kind']['all']['reviewer_hours_per_1k_loops'])
        m["review_items_per_100_loops"].append(r['reviewer']['by_kind']['all']['items_per_100_loops'])
        for o, v in r['reviewer']['by_origin'].items():  # subgroup review burden (name origin)
            m[f"review_items_per_100_{o}"].append(v['items_per_100_loops'])
            m[f"reviewer_h_per_1k_{o}"].append(v['reviewer_hours_per_1k_loops'])
        bo = r['reviewer']['by_origin']
        rates = [v['items_per_100_loops'] for v in bo.values()]
        m["review_load_ratio_max_min_origin"].append(max(rates) / min(rates))
        cr = r['crash_recovery']
        pool["duplicate_outputs"] += cr['duplicate_outputs']; pool["lost_outputs"] += cr.get('lost_outputs', 0); pool["outputs"] += cr['expected_outputs']
        pool["ingest_lost"] += cr['ingest_lost']; pool["ingest_duplicates"] += cr['ingest_duplicates']; pool["resources"] += cr['generated_resources']
        pool["final_state_mismatch"] += cr['final_state_mismatch_loops']
        ol = r['outreach_live']
        pool["wrong_patient_released"] += sum(v['wrong_released'] for v in ol['by_origin'].values())
        pool["outreach_intents"] += sum(v['intents'] for v in ol['by_origin'].values())
        ct = r['consent_timing']
        pool["released"] += ct.get('released_total', 0); pool["released_without_granted_consent"] += ct.get('released_without_granted_consent', 0)
        pool["forged"] += r['forged']['forged_events']; pool["forged_effect"] += r['forged']['forged_with_any_effect']
        pool["forged_target_falsely_merged_by_linker"] += r['forged'].get('forged_target_pair_falsely_merged_by_linker', 0)
        att = [a for a in t['attacks'] if a['attacker'] != '-']
        pool["tamper_attacks"] += len(att); pool["tamper_detected"] += sum(a['detected'] for a in att)
        pool["tamper_false_alarm"] += sum(a['detected'] for a in t['attacks'] if a['attacker'] == '-')
        m["throughput_ev_s"].append(r['throughput_ev_s'])
    agg = {k: ci(v) for k, v in m.items()}
    safety = {
        "crc_false_closures": [pool["crc_false_closures"], pool["crc_closed"], cp_ub(pool["crc_false_closures"], pool["crc_closed"])],
        "cervical_false_closures": [pool["cervical_false_closures"], pool["cervical_closed"], cp_ub(pool["cervical_false_closures"], pool["cervical_closed"])],
        "wrong_patient_released": [pool["wrong_patient_released"], pool["outreach_intents"], cp_ub(pool["wrong_patient_released"], pool["outreach_intents"])],
        "released_without_granted_consent": [pool["released_without_granted_consent"], pool["released"], cp_ub(pool["released_without_granted_consent"], pool["released"])],
        "duplicate_or_lost_outputs": [pool["duplicate_outputs"] + pool["lost_outputs"], pool["outputs"], cp_ub(pool["duplicate_outputs"] + pool["lost_outputs"], pool["outputs"])],
        "ingest_lost_or_duplicated": [pool["ingest_lost"] + pool["ingest_duplicates"], pool["resources"], None],
        "final_state_mismatch_loops": [pool["final_state_mismatch"], None, None],
        "forged_with_effect": [pool["forged_effect"], pool["forged"], None],
        "forged_target_pair_falsely_merged_by_linker (RBAC denied the forgery)": [pool["forged_target_falsely_merged_by_linker"], None, None],
        "tamper_detected": [pool["tamper_detected"], pool["tamper_attacks"], None],
        "tamper_false_alarms": [pool["tamper_false_alarm"], len(T), None],
    }
    out["by_noise"][noise] = {"seeds": len(R), "metrics": agg, "safety_pooled": safety}
    md.append(f"\n## Noise {noise}: {man['noise_models'][noise]} ({len(R)} seeds)\n")
    md.append("| Metric | Mean | 95% interval |\n|---|---|---|")
    for k in ["crc_precision", "crc_recall", "crc_recall_no_review", "cervical_precision", "cervical_recall", "kpi_truth", "kpi_wasl", "kpi_eid_only",
              "kpi_abs_error_wasl", "kpi_abs_error_eid_only", "linkage_pair_f1", "reviewer_h_per_1k", "review_items_per_100_loops", "throughput_ev_s"] + \
             sorted(x for x in agg if (x.startswith("review_items_per_100_") and x != "review_items_per_100_loops") or (x.startswith("reviewer_h_per_1k_"))) + ["review_load_ratio_max_min_origin"]:
        if k in agg:
            a = agg[k]
            md.append(f"| {k} | {a[0]} | {'–' if a[1] is None else f'{a[1]} – {a[2]}'} |")
    md.append("\n| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |\n|---|---|---|---|")
    for k, (e, n, ub) in safety.items():
        md.append(f"| {k} | {e} | {'–' if n is None else n} | {'–' if ub is None else f'{ub:.4%}'} |")
md.append("\n## Limitations\n" + "\n".join(f"- {x}" for x in man["limitations"]))
json.dump(out, open(os.path.join(F, 'summary.json'), 'w'), indent=1)
open(os.path.join(F, 'SUMMARY.md'), 'w').write("\n".join(md) + "\n")
print("\n".join(md))
