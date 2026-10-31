"""Aggregate pipeline/results/final/<noise>-s<seed>/ into summary.json + SUMMARY.md (means, 95% t-intervals, Clopper-Pearson UBs)."""
import json, os, sys, glob, math, collections, random
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
        for o, v in bo.items():
            pool[f"origin_items_{o}"] += v['items']; pool[f"origin_loops_{o}"] += v['loops']
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
        pool["forged_consent_target_opted_out"] += r['forged'].get('forged_consent_target_genuinely_opted_out', 0)
        att = [a for a in t['attacks'] if a['attacker'] != '-']
        pool["tamper_attacks"] += len(att); pool["tamper_detected"] += sum(a['detected'] for a in att)
        pool["tamper_false_alarm"] += sum(a['detected'] for a in t['attacks'] if a['attacker'] == '-')
        m["throughput_ev_s"].append(r['throughput_ev_s'])
        lu = r.get('programmes_live', {}).get('lung_nodule')
        if lu:  # incidental lung-nodule loops (second loop type), when the run has them
            lc = lu['closure']
            m["lung_precision"].append(lc['precision']); m["lung_recall"].append(lc['recall'])
            m["lung_state_accuracy"].append(lu['state_accuracy']); m["lung_escalation_recall"].append(lu['escalation']['recall'])
            pool["lung_false_closures"] += lc['false_closures']; pool["lung_closed"] += lc['pred_closed']
            for k2, v2 in (lu.get('workup_gt8mm') or {}).items():
                pool["lung_workup_" + k2] += v2
        for prog in ("crc", "cervical"):  # rejected-referral hand-off packet (handoff.go)
            ho = ((r.get('programmes_live', {}).get(prog) or {}).get('handoff') or {}).get('counts') or {}
            for k2, v2 in ho.items():
                pool[f"handoff_{k2}"] += v2
        ps = r.get('patient_side')
        if ps:
            m["stalled_days_after_deadline_flagged_median"].append(ps['days_after_deadline_flagged_median'])
            m["stalled_days_before_deadline_first_escalation_median"].append(ps['days_before_deadline_first_escalation_median'])
            m["stalled_share_flagged"].append(ps['stalled_flagged_overdue'] / max(1, ps['stalled_loops_truth']))
            m["results_within_21d_agreement"].append(ps['results_communicated_within_21d_agreement'])
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
        "forged_consent_target_genuinely_opted_out (RBAC denied the forgery)": [pool["forged_consent_target_opted_out"], None, None],
        "tamper_detected": [pool["tamper_detected"], pool["tamper_attacks"], None],
        "tamper_false_alarms": [pool["tamper_false_alarm"], len(T), None],
    }
    if pool["lung_closed"]:
        safety["lung_false_closures"] = [pool["lung_false_closures"], pool["lung_closed"], cp_ub(pool["lung_false_closures"], pool["lung_closed"])]
    if pool["handoff_closed"]:
        safety["handoff_false_closures"] = [pool["handoff_false_closures"], pool["handoff_closed"], cp_ub(pool["handoff_false_closures"], pool["handoff_closed"])]
    extra = {k: v for k, v in pool.items() if k.startswith("lung_workup_") or k.startswith("handoff_")}
    # review-load ratio: the per-seed max/min is noisy with four unequal origin groups, so also report it pooled over
    # seeds and next to a no-bias reference (origin labels shuffled across loops; scripts/origin_null.py)
    pooled_rates = {k[len("origin_items_"):]: 100 * v / pool["origin_loops_" + k[len("origin_items_"):]] for k, v in pool.items() if k.startswith("origin_items_")}
    origin_ratio = {"per_seed_mean_ci": agg.get("review_load_ratio_max_min_origin"),
                    "pooled_items_per_100": {k: round(v, 1) for k, v in sorted(pooled_rates.items())},
                    "pooled_ratio": round(max(pooled_rates.values()) / min(pooled_rates.values()), 4) if pooled_rates else None}
    nulls = [json.load(open(x)) for x in sorted(glob.glob(os.path.join(F, f"{noise}-s*", "origin_null.json")))]
    if nulls:
        origin_ratio.update({"no_bias_median_mean": round(mean(x["null_median"] for x in nulls), 4),
                             "no_bias_p95_mean": round(mean(x["null_p95"] for x in nulls), 4),
                             "seeds_above_no_bias_p05": sum(x["p_value"] < 0.05 for x in nulls), "seeds_with_reference": len(nulls)})
        if all("items_per_loop_hist" in x for x in nulls):  # pooled test: shuffle origin labels within each seed, pool, max/min
            rng, groups, null = random.Random(20261030), sorted(nulls[0]["loops"]), []
            seeds = [([o for o in groups for _ in range(x["loops"][o])], [int(c) for c, n in x["items_per_loop_hist"].items() for _ in range(n)]) for x in nulls]
            for _ in range(1000):
                L, I = collections.Counter(), collections.Counter()
                for labels, counts in seeds:
                    rng.shuffle(labels)
                    for o, c in zip(labels, counts):
                        L[o] += 1; I[o] += c
                rates = [I[o] / L[o] for o in groups]
                null.append(max(rates) / min(rates))
            null.sort()
            origin_ratio.update({"pooled_no_bias_median": round(null[500], 4), "pooled_no_bias_p95": round(null[950], 4),
                                 "pooled_p_value": round(sum(x >= origin_ratio["pooled_ratio"] for x in null) / 1000, 4)})
    # age band and sex (CRC loops only; scripts/subgroups.py): pooled review load, recall and a within-seed shuffled-label test
    subs = [json.load(open(x)) for x in sorted(glob.glob(os.path.join(F, f"{noise}-s*", "subgroups.json")))]
    subgroups = {}
    for name in (subs[0] if subs else {}):
        G = sorted(subs[0][name]["groups"])
        tot = {g: collections.Counter() for g in G}
        for x in subs:
            for g in G:
                for k in ("loops", "items", "crc_truly_closed", "crc_closed_by_wasl", "crc_false_closures"):
                    tot[g][k] += x[name]["groups"][g][k]
        rate = {g: 100 * tot[g]["items"] / tot[g]["loops"] for g in G}
        obs = max(rate.values()) / min(rate.values())
        rng, null = random.Random(20261031), []
        seeds = [([g for g in G for _ in range(x[name]["groups"][g]["loops"])],
                  [int(c) for g in G for c, n in x[name]["groups"][g]["items_per_loop_hist"].items() for _ in range(n)]) for x in subs]
        for _ in range(1000):
            L, I = collections.Counter(), collections.Counter()
            for labels, counts in seeds:
                rng.shuffle(labels)
                for g, c in zip(labels, counts):
                    L[g] += 1; I[g] += c
            r = [I[g] / L[g] for g in G]
            null.append(max(r) / min(r))
        null.sort()
        subgroups[name] = {"pooled_items_per_100": {g: round(rate[g], 1) for g in G}, "pooled_ratio": round(obs, 4),
                           "pooled_no_bias_p95": round(null[950], 4), "pooled_p_value": round(sum(v >= obs for v in null) / 1000, 4),
                           "crc_recall": {g: round(tot[g]["crc_closed_by_wasl"] / max(1, tot[g]["crc_truly_closed"]), 3) for g in G},
                           "crc_false_closures": {g: tot[g]["crc_false_closures"] for g in G}}
    out["by_noise"][noise] = {"seeds": len(R), "metrics": agg, "safety_pooled": safety, "pooled_counts": extra, "review_load_by_origin": origin_ratio,
                              "crc_by_age_band_and_sex": subgroups}
    md.append(f"\n## Noise {noise}: {man['noise_models'][noise]} ({len(R)} seeds)\n")
    md.append("| Metric | Mean | 95% interval |\n|---|---|---|")
    for k in ["crc_precision", "crc_recall", "crc_recall_no_review", "cervical_precision", "cervical_recall", "kpi_truth", "kpi_wasl", "kpi_eid_only",
              "kpi_abs_error_wasl", "kpi_abs_error_eid_only", "lung_precision", "lung_recall", "lung_state_accuracy", "lung_escalation_recall", "linkage_pair_f1", "reviewer_h_per_1k", "review_items_per_100_loops", "throughput_ev_s"] + \
             sorted(x for x in agg if (x.startswith("review_items_per_100_") and x != "review_items_per_100_loops") or (x.startswith("reviewer_h_per_1k_"))) + ["review_load_ratio_max_min_origin"]:
        if k in agg:
            a = agg[k]
            md.append(f"| {k} | {a[0]} | {'–' if a[1] is None else f'{a[1]} – {a[2]}'} |")
    md.append("\n| Safety (pooled over seeds) | Events | Denominator | 95% upper bound |\n|---|---|---|---|")
    for k, (e, n, ub) in safety.items():
        md.append(f"| {k} | {e} | {'–' if n is None else n} | {'–' if ub is None else f'{ub:.4%}'} |")
    o = origin_ratio
    md.append(f"\nReview load by name origin: pooled over seeds {o['pooled_items_per_100']} items per 100 loops, max/min **{o['pooled_ratio']}**.")
    if "no_bias_median_mean" in o:
        md.append(f"With origin labels shuffled (no origin effect), the per-seed max/min has median {o['no_bias_median_mean']} and 95th percentile "
                  f"{o['no_bias_p95_mean']} (mean over seeds); {o['seeds_above_no_bias_p05']} of {o['seeds_with_reference']} seeds are above it at p < 0.05.")
    if "pooled_p_value" in o:
        md.append(f"Pooled over seeds, the no-bias max/min has median {o['pooled_no_bias_median']} and 95th percentile {o['pooled_no_bias_p95']}; "
                  f"the observed pooled {o['pooled_ratio']} has p = {o['pooled_p_value']}.")
    for name, sg in subgroups.items():
        md.append(f"CRC loops by {name.replace('_', ' ')}: {sg['pooled_items_per_100']} review items per 100 loops, max/min **{sg['pooled_ratio']}** "
                  f"(no-bias 95th percentile {sg['pooled_no_bias_p95']}, p = {sg['pooled_p_value']}); recall {sg['crc_recall']}; false closures {sg['crc_false_closures']}.")
    if extra:
        md.append("\n| Pooled counts (lung > 8 mm work-up, rejected-referral hand-off) | Count |\n|---|---|")
        md.extend(f"| {k} | {v} |" for k, v in sorted(extra.items()))
md.append("\n## Limitations\n" + "\n".join(f"- {x}" for x in man["limitations"]))
json.dump(out, open(os.path.join(F, 'summary.json'), 'w'), indent=1)
open(os.path.join(F, 'SUMMARY.md'), 'w').write("\n".join(md) + "\n")
print("\n".join(md))
