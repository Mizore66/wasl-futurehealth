"""Paired, person-clustered bootstrap 95% CI for the real-variant R@1/R@10 difference between two encoders.

Usage: python encoder/paired_ci.py --variants V.jsonl --old models/encoder_weights.json --new models/encoder_real_v2.json
       [--split eval] [--boot 2000]
"""
import argparse
import json
import os
import sys

import numpy as np
import torch

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import data  # noqa: E402
import wenc  # noqa: E402


def per_query_hits(model, queries, gal):
    uniq = sorted({wenc.norm_name(g) for g in gal.values()})
    idx = {s: i for i, s in enumerate(uniq)}
    G = torch.tensor(model.embed_raw(uniq))
    Q = torch.tensor(model.embed_raw([x for x, _ in queries]))
    tgt = torch.tensor([idx[wenc.norm_name(gal[qid])] for _, qid in queries])
    top = (Q @ G.T).topk(10, dim=1).indices
    return (top[:, :1] == tgt[:, None]).any(1).numpy(), (top == tgt[:, None]).any(1).numpy()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--variants", required=True)
    ap.add_argument("--old", required=True)
    ap.add_argument("--new", required=True)
    ap.add_argument("--split", default="eval")
    ap.add_argument("--boot", type=int, default=2000)
    a = ap.parse_args()
    people = data.load_people(a.variants)
    q, g_ar, g_lat = data.real_test(people, a.split)
    qids, hits = [], {}
    for name, path in (("old", a.old), ("new", a.new)):
        m = wenc.load_json(path, 0.0).eval()
        h1, h10 = [], []
        for key, gal in (("lat2ar", g_ar), ("ar2lat", g_lat)):
            x1, x10 = per_query_hits(m, q[key], gal)
            h1.append(x1)
            h10.append(x10)
        hits[name] = (np.concatenate(h1), np.concatenate(h10))
    qids = np.array([qid for key in ("lat2ar", "ar2lat") for _, qid in q[key]])
    persons, inv = np.unique(qids, return_inverse=True)
    rng = np.random.default_rng(0)
    out = {"n_queries": len(qids), "n_persons": len(persons)}
    for j, k in ((0, "R@1"), (1, "R@10")):
        d = hits["new"][j].astype(float) - hits["old"][j].astype(float)
        per = np.bincount(inv, weights=d, minlength=len(persons))
        cnt = np.bincount(inv, minlength=len(persons))
        boots = []
        for _ in range(a.boot):
            s = rng.integers(0, len(persons), len(persons))
            boots.append(per[s].sum() / cnt[s].sum())
        out[k] = {"old": float(hits["old"][j].mean()), "new": float(hits["new"][j].mean()), "diff": float(d.mean()),
                  "ci95": [float(np.percentile(boots, 2.5)), float(np.percentile(boots, 97.5))]}
    print(json.dumps(out, indent=1))


if __name__ == "__main__":
    main()
