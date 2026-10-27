"""Evaluate one or more encoder JSON files on (a) the held-out REAL-variant test and (b) the synthetic protocol.

Usage: python encoder/eval.py --split eval --variants <wikidata_variants.jsonl> --wsi bin/wsi7 --work <dir> \
           --enc models/encoder_weights.json [--enc new.json ...] [--out results.json]

Retrieval: cosine top-k over the gallery; the gallery is deduplicated on the normalised name, and a query is a hit
at k if its own person's normalised gallery string is among the top-k distinct strings (homonyms count as one).
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


def retrieve(model, q_raw, q_target, gal_raw, ks=(1, 10), dev="cpu"):
    """q_target[i] = raw gallery string of the query's own person. Returns {R@k}."""
    gnorm = [wenc.norm_name(g) for g in gal_raw]
    uniq = sorted(set(gnorm))
    idx = {s: i for i, s in enumerate(uniq)}
    G = torch.tensor(model.embed_raw(uniq, dev))
    Q = torch.tensor(model.embed_raw(q_raw, dev))
    tgt = torch.tensor([idx[wenc.norm_name(t)] for t in q_target])
    hits = {k: 0 for k in ks}
    K = max(ks)
    for i in range(0, len(q_raw), 2048):
        s = Q[i:i + 2048] @ G.T
        top = s.topk(min(K, s.shape[1]), dim=1).indices
        t = tgt[i:i + 2048, None]
        for k in ks:
            hits[k] += int((top[:, :k] == t).any(1).sum())
    n = len(q_raw)
    return {f"R@{k}": hits[k] / max(n, 1) for k in ks} | {"n": n, "gallery": len(uniq)}


def real_eval(model, people, split, dev="cpu"):
    q, g_ar, g_lat = data.real_test(people, split)
    res = {}
    for key, gal in (("lat2ar", g_ar), ("ar2lat", g_lat)):
        qs = q[key]
        res[key] = retrieve(model, [x for x, _ in qs], [gal[qid] for _, qid in qs], list(gal.values()), dev=dev)
    n = res["lat2ar"]["n"] + res["ar2lat"]["n"]
    for k in ("R@1", "R@10"):
        res[k] = (res["lat2ar"][k] * res["lat2ar"]["n"] + res["ar2lat"][k] * res["ar2lat"]["n"]) / max(n, 1)
    res["n"] = n
    return res


def syn_eval(model, pairs, dev="cpu"):
    ar = [p["ar"] for p in pairs]
    lat = [p["lat"] for p in pairs]
    res = {"ar2lat": retrieve(model, ar, lat, lat, dev=dev), "lat2ar": retrieve(model, lat, ar, ar, dev=dev)}
    for k in ("R@1", "R@10"):
        res[k] = (res["ar2lat"][k] + res["lat2ar"][k]) / 2
    return res


def evaluate(model, people, split, wsi, work, dev="cpu"):
    model.eval()
    out = {"real": real_eval(model, people, split, dev)}
    for nz in ("v1", "v2"):
        out["syn_" + nz] = syn_eval(model, data.synthetic_test(wsi, split, nz, work), dev)
    # same-script retrieval (the linker also scores Latin-Latin and Arabic-Arabic registration pairs)
    a, b = data.synthetic_test(wsi, split, "v1", work, 7), data.synthetic_test(wsi, split, "v1", work, 8)
    for key in ("lat", "ar"):
        qa, qb = [p[key] for p in a], [p[key] for p in b]
        out["same_" + key] = retrieve(model, qa, qb, qb, dev=dev)
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--enc", action="append", required=True)
    ap.add_argument("--split", default="eval")
    ap.add_argument("--variants", required=True)
    ap.add_argument("--wsi", required=True)
    ap.add_argument("--work", required=True)
    ap.add_argument("--out")
    a = ap.parse_args()
    people = data.load_people(a.variants)
    allres = {}
    for e in a.enc:
        m = wenc.load_json(e, dropout=0.0)
        r = evaluate(m, people, a.split, a.wsi, a.work)
        allres[os.path.basename(e)] = r
        print(os.path.basename(e), json.dumps({k: {kk: round(vv, 4) for kk, vv in v.items() if kk.startswith("R@")} for k, v in r.items()}))
    if a.out:
        json.dump(allres, open(a.out, "w"), indent=1)


if __name__ == "__main__":
    main()
