"""Refit the pair scorer and its calibrators on cosines from a given encoder.

Inputs are `wsi featdump` CSVs (label, corr, f0..f29, near_eid, decade_dob, same_surname,
placeholder_dob, cross_script[, p]; 34 features for modelset pairfeat6, f30..f33 appended; features may be NaN = missing, passed to LightGBM unimputed): train-split cohorts (LightGBM fit; several cohorts of
runtime size so the base rate, and hence the raw-p scale the 0.7 union threshold acts on, matches the cal cohort) and
one cal-split cohort (calibrators). Writes, with suffix X:
  lgb_stream_X.json  LightGBM dump_model JSON (read by pipeline/model.go LoadGBM), 400 trees, binary
  iso_X.json         isotonic p -> P(same person) on cal (sklearn semantics, read by pipeline/iso.go)
  iso_v7_X.json      v7 stratified conservative calibrator: corroborated (corr=1 and p>=0.7) / uncorroborated strata,
                     isotonic plateaus replaced by their one-sided 95% Clopper-Pearson lower bound (adjacent plateaus
                     pooled until monotone); the uncorroborated stratum is capped at 0.989 (pipeline/v7.go).

Usage: python encoder/fit_scorer.py --train c1.csv [--train c2.csv ...] --cal feat_cal.csv --outdir DIR --suffix X [--lambda-l2 L] [--nfeat 34]
"""
import argparse
import json
import os

import numpy as np
from scipy.stats import beta
from sklearn.isotonic import IsotonicRegression
from sklearn.metrics import average_precision_score, roc_auc_score

NFEAT = 30
NFEAT_CHOICES = (30, 34)
SLICES = ("near_eid", "decade_dob", "same_surname", "placeholder_dob", "cross_script")
PARAMS = {"objective": "binary", "num_leaves": 63, "learning_rate": 0.05, "min_child_samples": 50,
          "feature_fraction": 0.9, "bagging_fraction": 0.8, "bagging_freq": 1, "seed": 1, "verbose": -1,
          "deterministic": True, "num_threads": 8}
ROUNDS = 400
UNC_CAP = 0.989


def params_for(lambda_l2=None):
    return PARAMS if lambda_l2 is None else dict(PARAMS, lambda_l2=lambda_l2)


def max_abs_leaf(dump):
    best = 0.0
    for t in dump["tree_info"]:
        stack = [t["tree_structure"]]
        while stack:
            n = stack.pop()
            if "leaf_value" in n:
                best = max(best, abs(float(n["leaf_value"])))
            else:
                stack += [n["left_child"], n["right_child"]]
    return best


def _nonneg_float(s):
    v = float(s)
    if not np.isfinite(v) or v < 0:
        raise argparse.ArgumentTypeError(f"must be a finite float >= 0, got {s}")
    return v


def load(path, nfeat=NFEAT):
    d = np.loadtxt(path, delimiter=",", dtype=np.float64, ndmin=2)
    if d.shape[0] == 0 or d.shape[1] not in (2 + nfeat + len(SLICES), 3 + nfeat + len(SLICES)):
        raise ValueError(f"{path}: expected {2 + nfeat + len(SLICES)} or {3 + nfeat + len(SLICES)} columns, got {d.shape[1]}")
    y, corr = d[:, 0].astype(int), d[:, 1].astype(int)
    X = d[:, 2:2 + nfeat]
    raw = d[:, 2 + nfeat:2 + nfeat + len(SLICES)]
    if not np.isin(d[:, :2], [0, 1]).all() or not np.isin(raw, [0, 1]).all():
        raise ValueError(f"{path}: labels, corr, and slice fields must be binary")
    slices = {name: raw[:, i].astype(bool) for i, name in enumerate(SLICES)}
    return y, corr, X, slices


def slice_report(y, p, slices):
    if not np.isfinite(p).all():
        raise ValueError("nonfinite model prediction")
    out = {}
    for name, mask in slices.items():
        neg = mask & (y == 0)
        out[name] = {"rows": int(mask.sum()), "positives": int((mask & (y == 1)).sum()),
                     "different_person": int(neg.sum()),
                     "different_at_review_0_01": int((neg & (p >= 0.01)).sum()),
                     "different_at_merge_0_7": int((neg & (p >= 0.7)).sum())}
    return out


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


def parse_args(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--train", action="append", required=True, help="repeatable: one featdump CSV per simulated cohort")
    ap.add_argument("--cal", required=True)
    ap.add_argument("--outdir", required=True)
    ap.add_argument("--suffix", required=True)
    ap.add_argument("--lambda-l2", type=_nonneg_float, default=None, help="optional LightGBM lambda_l2 (>= 0)")
    ap.add_argument("--nfeat", type=int, choices=NFEAT_CHOICES, default=NFEAT, help="feature columns per row (34: pairfeat6)")
    return ap.parse_args(argv)


def main():
    a = parse_args()
    import lightgbm as lgb  # deferred so loader/report tests need no LightGBM
    parts = [load(t, a.nfeat) for t in a.train]
    ytr, Xtr = np.concatenate([p[0] for p in parts]), np.concatenate([p[2] for p in parts])
    yca, cca, Xca, sca = load(a.cal, a.nfeat)
    bst = lgb.train(params_for(a.lambda_l2), lgb.Dataset(Xtr, ytr), ROUNDS)
    pca = bst.predict(Xca)
    os.makedirs(a.outdir, exist_ok=True)
    sfx = a.suffix
    dump = bst.dump_model()
    json.dump(dump, open(os.path.join(a.outdir, f"lgb_stream_{sfx}.json"), "w"))
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
    if a.lambda_l2 is not None:
        rep["lambda_l2"] = a.lambda_l2
        rep["max_abs_leaf"] = max_abs_leaf(dump)
    if a.nfeat != NFEAT:
        rep["nfeat"] = a.nfeat
    rep["hard_negative_slices"] = slice_report(yca, pca, sca)
    for t in (0.5, 0.7, 0.9, 0.98, 0.99):
        s = pca >= t
        rep[f"cal_at_{t}"] = {"pred": int(s.sum()), "precision": float(yca[s].mean()) if s.any() else None,
                              "recall": float(yca[s].sum() / yca.sum())}
    json.dump(rep, open(os.path.join(a.outdir, f"fit_report_{sfx}.json"), "w"), indent=1)
    np.save(os.path.join(a.outdir, f"cal_p_{sfx}.npy"), pca)
    print(json.dumps(rep, indent=1))


if __name__ == "__main__":
    main()
