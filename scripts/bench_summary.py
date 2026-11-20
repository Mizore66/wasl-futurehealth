"""Build pipeline/results/benchmarks/summary.json from the benchmark result files (stdlib only).

Item shape = the `benchmarks[]` items of web/data/evidence.json:
  {"name", "dataset", "url", "metric", "wasl", "baseline", "baseline_source", "note"}
Values are fractions (0..1). Inputs (any missing file just drops its items):
  febrl/wasl_names_dob.json, febrl/wasl_names_dob_ssid.json  (bin/wsi7 febrl; scripts/bench_febrl.sh)
  febrl/recordlinkage.json                                    (scripts/bench_febrl_recordlinkage.py)
  names/encoder_namepairs.json                                (bin/wsi7 namepairs; scripts/bench_names.sh)
Usage: python3 -I scripts/bench_summary.py pipeline/results/benchmarks
"""

import json
import os
import sys

FEBRL_URL = "https://recordlinkage.readthedocs.io/en/latest/ref-datasets.html"
PRECLINK = "preclink 1.2.0 README benchmark table (recordlinkage recipe in examples/benchmark_febrl.py): https://pypi.org/project/preclink/1.2.0/"
RERUN = "recordlinkage 0.16 rerun by scripts/bench_febrl_recordlinkage.py"
NAMES_URL = "https://huggingface.co/datasets/Sayedyousef/arabic-name-pairs"


def load(d, rel):
    p = os.path.join(d, rel)
    if not os.path.exists(p):
        return None
    with open(p) as fh:
        return json.load(fh)


def ci(r):
    return f"{r['value']:.3f} [{r['low']:.3f}, {r['high']:.3f}] ({r['k']}/{r['n']})"


def main(d):
    out = []
    nd = load(d, "febrl/wasl_names_dob.json")
    ns = load(d, "febrl/wasl_names_dob_ssid.json")
    rl = load(d, "febrl/recordlinkage.json")
    pub = rl["published"] if rl else None
    same = rl["same_fields"] if rl else None
    if nd:
        a = nd["auto_link"]
        common = ("FEBRL4 (5,000 + 5,000 synthetic Australian records, known 1:1 mapping; not made by the team). "
                  "Wasl config names_dob: name = given + surname, DOB; no sex, no ID (FEBRL's 7-digit soc_sec_id cannot "
                  "fill the Emirates-ID slot); encoder, features, LightGBM scorer, 0.7 union threshold and merge rules "
                  "unchanged; final clusters scored. ")
        for metric, key, pv in (("precision", "precision", 0.943), ("recall", "recall", 0.808)):
            out.append({"name": f"FEBRL4 linkage {metric}, names + DOB", "dataset": "FEBRL4", "url": FEBRL_URL,
                        "metric": metric, "wasl": a[key]["value"], "baseline": pv, "baseline_source": PRECLINK,
                        "note": common + f"Wasl {ci(a[key])}, Wilson 95%. Baseline uses address, suburb and postcode "
                                         "(fields Wasl does not have) and blocks on postcode; reproduced exactly by " + RERUN + "."})
        out.append({"name": "FEBRL4 linkage F1, names + DOB", "dataset": "FEBRL4", "url": FEBRL_URL, "metric": "f1",
                    "wasl": a["f1"], "baseline": 0.870, "baseline_source": PRECLINK,
                    "note": common + f"{a['predicted_pairs']} predicted pairs, {a['true_positives']} correct."})
        if same:
            out.append({"name": "FEBRL4 linkage F1, same fields as Wasl", "dataset": "FEBRL4", "url": FEBRL_URL, "metric": "f1",
                        "wasl": a["f1"], "baseline": same["f1"], "baseline_source": RERUN + " (published recipe restricted to given name, surname, DOB)",
                        "note": f"Equal-information comparison. Baseline precision {ci(same['precision'])}, recall {ci(same['recall'])}; "
                                f"Wasl precision {ci(a['precision'])}, recall {ci(a['recall'])}."})
        out.append({"name": "FEBRL4 false-match rate, names + DOB", "dataset": "FEBRL4", "url": FEBRL_URL,
                    "metric": "false_match_rate", "wasl": a["false_match_rate"]["value"],
                    "baseline": pub["false_match_rate"]["value"] if pub else None,
                    "baseline_source": RERUN + " (published recipe)" if pub else None,
                    "note": "Share of the 5,000 B records auto-linked to at least one wrong A record. "
                            f"Wasl {ci(a['false_match_rate'])}; missed-match rate {ci(a['missed_match_rate'])}. "
                            f"With the human-review band (score 0.2 to 0.7) added, recall {nd['auto_or_review']['recall']['value']:.3f} "
                            f"at precision {nd['auto_or_review']['precision']['value']:.3f} (direct pairs)."})
    if ns:
        a = ns["auto_link"]
        out.append({"name": "FEBRL4 linkage recall, names + DOB + ID (adapted)", "dataset": "FEBRL4", "url": FEBRL_URL,
                    "metric": "recall", "wasl": a["recall"]["value"], "baseline": 0.808, "baseline_source": PRECLINK,
                    "note": "Adaptation: soc_sec_id wrapped into a synthetic 784-prefixed Luhn-valid number so Wasl's "
                            "Emirates-ID rules apply (equal -> forced link; both valid and different -> never linked). "
                            f"Precision {ci(a['precision'])}, recall {ci(a['recall'])}. Every miss is a record whose ID "
                            "FEBRL corrupted: a valid-looking but wrong ID vetoes the link, which is Wasl's designed behaviour."})
    np_ = load(d, "names/encoder_namepairs.json")
    if np_:
        tiers = {t["tier"]: t for t in np_["tiers"]}
        t = tiers["pair_dedup"]
        base = (f"Sayedyousef/arabic-name-pairs (CC-BY-4.0; single-token names, merged from JRC-Names, machine "
                f"translation and an LLM by its author). Name encoder alone, no scorer. {t['pairs']:,} pairs after removing "
                f"{np_['dropped_exact_pair_overlap']} exact overlaps with the encoder's training pairs. ")
        out.append({"name": "Arabic-Latin name encoder ROC-AUC, hard negatives", "dataset": "arabic-name-pairs", "url": NAMES_URL,
                    "metric": "roc_auc", "wasl": t["roc_auc_hard_negatives"][0], "baseline": None, "baseline_source": None,
                    "note": base + "Hard negative: another Arabic name with the same first letter and length within one letter. "
                            f"95% interval [{t['roc_auc_hard_negatives'][1]:.4f}, {t['roc_auc_hard_negatives'][2]:.4f}] (Hanley-McNeil)."})
        r = {x["direction"]: x for x in t["retrieval"]}
        out.append({"name": "Arabic-Latin name encoder top-1 retrieval, Latin to Arabic", "dataset": "arabic-name-pairs",
                    "url": NAMES_URL, "metric": "top1_accuracy", "wasl": r["latin_to_arabic"]["top1"]["value"],
                    "baseline": None, "baseline_source": None,
                    "note": base + f"Gallery {r['latin_to_arabic']['gallery']:,} Arabic names. Top-1 {ci(r['latin_to_arabic']['top1'])}; "
                            f"top-10 (the linker's candidate depth) {ci(r['latin_to_arabic']['top10'])}; Arabic to Latin top-1 "
                            f"{ci(r['arabic_to_latin']['top1'])}."})
        th = t["threshold"]
        out.append({"name": "Arabic-Latin name encoder precision at a cosine threshold", "dataset": "arabic-name-pairs",
                    "url": NAMES_URL, "metric": "precision", "wasl": th["precision"]["value"], "baseline": None, "baseline_source": None,
                    "note": base + f"The runtime has no standalone encoder threshold (cosine feeds top-10 blocking and the scorer), so "
                            f"cosine {th['cosine_threshold']} was picked for best F1 on a random 20% of pairs and reported on the other "
                            f"80% with one hard negative per positive: precision {ci(th['precision'])}, recall {ci(th['recall'])}."})
        u = tiers.get("unseen_tokens")
        if u:
            ru = {x["direction"]: x for x in u["retrieval"]}
            out.append({"name": "Arabic-Latin name encoder top-1 retrieval, names never seen in training", "dataset": "arabic-name-pairs",
                        "url": NAMES_URL, "metric": "top1_accuracy", "wasl": ru["latin_to_arabic"]["top1"]["value"],
                        "baseline": None, "baseline_source": None,
                        "note": f"Strict subset: {u['pairs']:,} pairs whose English string and every Arabic token never occur as a "
                                f"word in any training name. Smaller gallery ({ru['latin_to_arabic']['gallery']} Arabic names), so "
                                f"not comparable with the full-set number. Top-1 {ci(ru['latin_to_arabic']['top1'])}; ROC-AUC hard "
                                f"negatives {u['roc_auc_hard_negatives'][0]:.4f}."})
    path = os.path.join(d, "summary.json")
    with open(path, "w") as fh:
        json.dump(out, fh, indent=1, ensure_ascii=False)
        fh.write("\n")
    print(f"{len(out)} benchmark items -> {path}")


if __name__ == "__main__":
    main(sys.argv[1])
