"""Refit the pair scorer and its calibrators on cosines from a given encoder.

Inputs are `wsi featdump` CSVs (label, corr, f0..f22[, p]): train-split cohorts (LightGBM fit; several cohorts of
runtime size so the base rate, and hence the raw-p scale the 0.7 union threshold acts on, matches the cal cohort) and
one cal-split cohort (calibrators). Writes, with suffix X:
  lgb_stream_X.json  LightGBM dump_model JSON (read by pipeline/model.go LoadGBM), 400 trees, binary
  iso_X.json         isotonic p -> P(same person) on cal (sklearn semantics, read by pipeline/iso.go)
  iso_v7_X.json      v7 stratified conservative calibrator: corroborated (corr=1 and p>=0.7) / uncorroborated strata,
                     isotonic plateaus replaced by their one-sided 95% Clopper-Pearson lower bound (adjacent plateaus
                     pooled until monotone); the uncorroborated stratum is capped at 0.989 (pipeline/v7.go).

Usage: python encoder/fit_scorer.py --train c1.csv [--train c2.csv ...] --cal feat_cal.csv --outdir DIR --suffix X
"""
import argparse
import json
import os

import lightgbm as lgb
import numpy as np
from scipy.stats import beta
from sklearn.isotonic import IsotonicRegression
from sklearn.metrics import average_precision_score, roc_auc_score

NFEAT = 23
PARAMS = {"objective": "binary", "num_leaves": 63, "learning_rate": 0.05, "min_child_samples": 50,
          "feature_fraction": 0.9, "bagging_fraction": 0.8, "bagging_freq": 1, "seed": 1, "verbose": -1,
          "deterministic": True, "num_threads": 8}
ROUNDS = 400
UNC_CAP = 0.989


def load(path):
    d = np.loadtxt(path, delimiter=",", dtype=np.float64)
    return d[:, 0].astype(int), d[:, 1].astype(int), d[:, 2:2 + NFEAT]


def cp_lower(k, n, alpha=0.05):
    return 0.0 if k == 0 else float(beta.ppf(alpha, k, n - k + 1))


def conservative_iso(p, y, cap=None):
    """Isotonic fit -> plateaus -> one-sided 95% CP lower bound per plateau, pooled until non-decreasing."""
    if len(p) == 0:
        return {"X": [0.0, 1.0], "Y": [0.0, 0.0]}
    o = np.argsort(p, kind="stable")
    p, y = p[o], y[o]
    fit = IsotonicRegression(out_of_bounds="clip", y_min=0, y_max=1).fit(p, y).predict(p)
    blocks = []  # [lo_idx, hi_idx) runs of equal fitted value
    s = 0
    for i in range(1, len(p) + 1):
        if i == len(p) or fit[i] != fit[s]:
            blocks.append([s, i])
            s = i

    def lb(b):
        k = int(y[b[0]:b[1]].sum())
        return cp_lower(k, b[1] - b[0])

    vals = [lb(b) for b in blocks]
    changed = True
    while changed:
        changed = False
        for i in range(len(blocks) - 1):
            if vals[i] > vals[i + 1]:
                blocks[i] = [blocks[i][0], blocks[i + 1][1]]
                del blocks[i + 1]
                vals[i] = lb(blocks[i])
                del vals[i + 1]
                changed = True
                break
    X, Y = [], []
    for b, v in zip(blocks, vals):
        v = min(v, cap) if cap is not None else v
        X += [float(p[b[0]]), float(p[b[1] - 1])]
        Y += [v, v]
    return {"X": X, "Y": Y, "plateaus": [{"lo": float(p[b[0]]), "hi": float(p[b[1] - 1]), "n": b[1] - b[0],
                                          "k": int(y[b[0]:b[1]].sum()), "lb": v} for b, v in zip(blocks, vals)]}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--train", action="append", required=True, help="repeatable: one featdump CSV per simulated cohort")
    ap.add_argument("--cal", required=True)
    ap.add_argument("--outdir", required=True)
    ap.add_argument("--suffix", required=True)
    a = ap.parse_args()
    parts = [load(t) for t in a.train]
    ytr, Xtr = np.concatenate([p[0] for p in parts]), np.concatenate([p[2] for p in parts])
    yca, cca, Xca = load(a.cal)
    bst = lgb.train(PARAMS, lgb.Dataset(Xtr, ytr), ROUNDS)
    pca = bst.predict(Xca)
    os.makedirs(a.outdir, exist_ok=True)
    sfx = a.suffix
    json.dump(bst.dump_model(), open(os.path.join(a.outdir, f"lgb_stream_{sfx}.json"), "w"))
    iso = IsotonicRegression(out_of_bounds="clip", y_min=0, y_max=1).fit(pca, yca)
    json.dump({"X": iso.X_thresholds_.tolist(), "Y": iso.y_thresholds_.tolist()}, open(os.path.join(a.outdir, f"iso_{sfx}.json"), "w"))
    corr = (cca == 1) & (pca >= 0.7)
    c = conservative_iso(pca[corr], yca[corr])
    u = conservative_iso(pca[~corr], yca[~corr], UNC_CAP)
    json.dump({"corroborated": {"X": c["X"], "Y": c["Y"]}, "uncorroborated": {"X": u["X"], "Y": u["Y"]}},
              open(os.path.join(a.outdir, f"iso_v7_{sfx}.json"), "w"))
    rep = {"train_rows": len(ytr), "train_pos": int(ytr.sum()), "cal_rows": len(yca), "cal_pos": int(yca.sum()),
           "cal_auc": roc_auc_score(yca, pca), "cal_ap": average_precision_score(yca, pca),
           "corroborated_plateaus": c["plateaus"][-3:], "uncorroborated_top_plateaus": u["plateaus"][-3:]}
    for t in (0.5, 0.7, 0.9, 0.98, 0.99):
        s = pca >= t
        rep[f"cal_at_{t}"] = {"pred": int(s.sum()), "precision": float(yca[s].mean()) if s.any() else None,
                              "recall": float(yca[s].sum() / yca.sum())}
    json.dump(rep, open(os.path.join(a.outdir, f"fit_report_{sfx}.json"), "w"), indent=1)
    np.save(os.path.join(a.outdir, f"cal_p_{sfx}.npy"), pca)
    print(json.dumps(rep, indent=1))


if __name__ == "__main__":
    main()
