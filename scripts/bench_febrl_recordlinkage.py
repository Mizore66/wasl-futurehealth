"""FEBRL4 baselines with the `recordlinkage` package (third-party; NOT a repo dependency).

Runs only inside the throwaway virtualenv that scripts/bench_febrl.sh creates under /tmp. Two baselines:

1. "published": the exact recordlinkage recipe from preclink's benchmark script
   (https://github.com/finite-sample/preclink/blob/main/examples/benchmark_febrl.py, function benchmark_recordlinkage):
   block on postcode; Jaro-Winkler on given_name, surname, address_1, suburb; exact postcode; link if the 5 scores
   sum to >= 3.5. preclink's README reports 94.3% precision, 80.8% recall, 87.0% F1 on FEBRL4.
2. "same_fields": the same recipe restricted to the fields Wasl sees (given_name, surname, date_of_birth), with the
   same rule (sum >= 0.7 x number of comparisons = 2.1 of 3). Blocking cannot use postcode (not a Wasl field), so it
   is the union of exact blocks on given_name, surname and date_of_birth.

Usage: python -I scripts/bench_febrl_recordlinkage.py OUT.json
"""

import json
import sys

import pandas as pd  # noqa: F401  (pulled in by recordlinkage)
import recordlinkage
from recordlinkage.datasets import load_febrl4


def wilson(k, n, z=1.959963984540054):
    if n == 0:
        return {"k": k, "n": n, "value": None, "low": None, "high": None}
    p = k / n
    c = (p + z * z / (2 * n)) / (1 + z * z / n)
    h = z * ((p * (1 - p) / n + z * z / (4 * n * n)) ** 0.5) / (1 + z * z / n)
    return {"k": k, "n": n, "value": round(p, 6), "low": round(max(0, c - h), 6), "high": round(min(1, c + h), 6)}


def score(pred, truth, n_b):
    tp = len(pred & truth)
    fp = len(pred - truth)
    prec, rec = wilson(tp, len(pred)), wilson(tp, len(truth))
    f1 = 2 * tp / (len(pred) + len(truth)) if tp else 0.0
    wrong_b = {b for (a, b) in pred - truth}
    hit_b = {b for (a, b) in pred & truth}
    return {"predicted_pairs": len(pred), "true_positives": tp, "false_positives": fp, "false_negatives": len(truth) - tp,
            "precision": prec, "recall": rec, "f1": round(f1, 6),
            "false_match_rate": wilson(len(wrong_b), n_b), "missed_match_rate": wilson(len(truth) - len(hit_b), len(truth))}


def main(out):
    df_a, df_b = load_febrl4()
    truth = set()
    for idx_b in df_b.index:
        idx_a = str(idx_b).split("-dup")[0] + "-org"
        if idx_a in df_a.index:
            truth.add((idx_a, str(idx_b)))

    # 1. published recipe (verbatim logic)
    ix = recordlinkage.Index()
    ix.block("postcode")
    cands = ix.index(df_a, df_b)
    c = recordlinkage.Compare()
    c.string("given_name", "given_name", method="jarowinkler", label="given_name")
    c.string("surname", "surname", method="jarowinkler", label="surname")
    c.string("address_1", "address_1", method="jarowinkler", label="address_1")
    c.string("suburb", "suburb", method="jarowinkler", label="suburb")
    c.exact("postcode", "postcode", label="postcode")
    f = c.compute(cands, df_a, df_b)
    pred1 = {(str(a), str(b)) for a, b in f[f.sum(axis=1) >= 3.5].index}

    # 2. same fields as Wasl
    ix2 = recordlinkage.Index()
    ix2.block("given_name")
    ix2.block("surname")
    ix2.block("date_of_birth")
    cands2 = ix2.index(df_a, df_b)
    c2 = recordlinkage.Compare()
    c2.string("given_name", "given_name", method="jarowinkler", label="given_name")
    c2.string("surname", "surname", method="jarowinkler", label="surname")
    c2.exact("date_of_birth", "date_of_birth", label="date_of_birth")
    f2 = c2.compute(cands2, df_a, df_b)
    pred2 = {(str(a), str(b)) for a, b in f2[f2.sum(axis=1) >= 2.1].index}
    blk2 = {(str(a), str(b)) for a, b in cands2}

    res = {
        "recordlinkage_version": recordlinkage.__version__,
        "true_pairs": len(truth),
        "published": {"recipe": "block postcode; JW given_name, surname, address_1, suburb; exact postcode; sum >= 3.5",
                      "candidate_pairs": len(cands), **score(pred1, truth, len(df_b))},
        "same_fields": {"recipe": "blocks given_name | surname | date_of_birth; JW given_name, surname; exact date_of_birth; sum >= 2.1",
                        "candidate_pairs": len(cands2), "blocking_recall": wilson(len(blk2 & truth), len(truth)),
                        **score(pred2, truth, len(df_b))},
    }
    with open(out, "w") as fh:
        json.dump(res, fh, indent=1)
        fh.write("\n")
    for k in ("published", "same_fields"):
        r = res[k]
        print(k, "P", r["precision"]["value"], "R", r["recall"]["value"], "F1", r["f1"])


if __name__ == "__main__":
    main(sys.argv[1])
