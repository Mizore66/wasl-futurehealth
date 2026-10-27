"""Train the two hold-only image readers on HyperKvasir (CC BY 4.0; see DATA.md for the competition-use caveat).

  landmark: caecum photo (positive) vs other lower-GI views (retroflex-rectum, polyp, ulcerative-colitis-grade-2)
  prep:     BBPS 2-3 "adequate" (positive) vs BBPS 0-1 "inadequate"

Split: the official HyperKvasir 2-fold split (fold 0 -> train/val, fold 1 -> held-out test). HyperKvasir publishes no
patient ids, so near-duplicate leakage across folds is measured with a 64-bit dHash and reported (metrics are also given
on the test set with near-duplicates removed).

Usage: python image/train.py <data_dir> <out_dir>
  data_dir: img/<class>/**.jpg + 2_fold_split.csv (from image/download.sh)
  out_dir:  landmark.pt, prep.pt, *.onnx, metrics.json, verdicts.json (test-image verdicts keyed by sha256; no pixels)
"""
import csv, hashlib, json, os, random, sys, time
import numpy as np
import torch, torch.nn as nn
from PIL import Image
from torchvision import models, transforms

DATA, OUT = sys.argv[1], sys.argv[2]
SPLIT_MODE = sys.argv[3] if len(sys.argv) > 3 else "grouped"  # grouped (primary) | official
os.makedirs(OUT, exist_ok=True)
SEED = 2026
random.seed(SEED); np.random.seed(SEED); torch.manual_seed(SEED)
DEV = "mps" if torch.backends.mps.is_available() else "cpu"
MODEL_VERSION = "hk-mnv3s-v1"
TASKS = {
    "landmark": {"pos": ["cecum"], "neg": ["retroflex-rectum", "polyps", "ulcerative-colitis-grade-2"]},
    "prep": {"pos": ["bbps-2-3"], "neg": ["bbps-0-1"]},
}

split = {r["file-name"]: int(r["split-index"]) for r in csv.DictReader(open(os.path.join(DATA, "2_fold_split.csv")), delimiter=";")}


def files(cls):
    out = []
    for root, _, fs in os.walk(os.path.join(DATA, "img", cls)):
        out += [os.path.join(root, f) for f in fs if f.endswith(".jpg")]
    return sorted(out)


def sha(p):
    return hashlib.sha256(open(p, "rb").read()).hexdigest()


def dhash(p):
    im = Image.open(p).convert("L").resize((9, 8), Image.BILINEAR)
    a = np.asarray(im, dtype=np.int16)
    bits = (a[:, 1:] > a[:, :-1]).flatten()
    return int("".join("1" if b else "0" for b in bits), 2)


def dhash256(p):
    a = np.asarray(Image.open(p).convert("L").resize((17, 16), Image.BILINEAR), dtype=np.int16)
    return (a[:, 1:] > a[:, :-1]).flatten()


def grouped_split(classes, thr=20):
    """Near-duplicate groups (union-find over 256-bit dHash, Hamming <= thr) within each class; whole groups go to one
    fold (sha256 of the group's first file name, mod 2). HyperKvasir has no patient/video ids, so this approximates a
    video-level split: consecutive frames of one video land in the same fold."""
    out = {}
    for c in classes:
        fs = files(c)
        H = np.array([dhash256(p) for p in fs])
        par = list(range(len(fs)))

        def find(i):
            while par[i] != i:
                par[i] = par[par[i]]; i = par[i]
            return i
        for i in range(len(fs)):
            d = (H[i + 1:] != H[i]).sum(1)
            for j in np.nonzero(d <= thr)[0]:
                a, b = find(i), find(i + 1 + j)
                if a != b:
                    par[b] = a
        groups = {}
        for i in range(len(fs)):
            groups.setdefault(find(i), []).append(i)
        for g in groups.values():
            key = min(os.path.basename(fs[i]) for i in g)
            fold = int(hashlib.sha256(key.encode()).hexdigest(), 16) % 2
            for i in g:
                out[os.path.basename(fs[i])] = fold
        print(f"grouped split {c}: {len(fs)} images in {len(groups)} near-duplicate groups", flush=True)
    return out


NORM = transforms.Normalize([0.485, 0.456, 0.406], [0.229, 0.224, 0.225])
TF_TRAIN = transforms.Compose([transforms.RandomResizedCrop(224, scale=(0.6, 1.0)), transforms.RandomHorizontalFlip(),
                               transforms.RandomVerticalFlip(), transforms.ColorJitter(0.2, 0.2, 0.1), transforms.ToTensor(), NORM])
TF_EVAL = transforms.Compose([transforms.Resize(256), transforms.CenterCrop(224), transforms.ToTensor(), NORM])


_IMG = {}


def load(p):  # decode once, keep a 256-px copy in memory (single process; macOS spawn workers re-import the script)
    if p not in _IMG:
        im = Image.open(p).convert("RGB")
        im.thumbnail((320, 320))
        _IMG[p] = im
    return _IMG[p]


class DS(torch.utils.data.Dataset):
    def __init__(self, items, tf):
        self.items, self.tf = items, tf

    def __len__(self):
        return len(self.items)

    def __getitem__(self, i):
        p, y = self.items[i]
        return self.tf(load(p)), torch.tensor(y, dtype=torch.float32)


def make_model():
    m = models.mobilenet_v3_small(weights=models.MobileNet_V3_Small_Weights.IMAGENET1K_V1)
    m.classifier[3] = nn.Linear(m.classifier[3].in_features, 1)
    return m


@torch.no_grad()
def logits(m, items):
    m.eval()
    dl = torch.utils.data.DataLoader(DS(items, TF_EVAL), batch_size=64, num_workers=0)
    return np.concatenate([m(x.to(DEV)).squeeze(1).float().cpu().numpy() for x, _ in dl])


def auc(y, s):
    y, s = np.asarray(y), np.asarray(s)
    order = np.argsort(s)
    r = np.empty(len(s)); r[order] = np.arange(1, len(s) + 1)
    # average ranks for ties
    for v in np.unique(s):
        idx = s == v
        r[idx] = r[idx].mean()
    npos = y.sum(); nneg = len(y) - npos
    return float((r[y == 1].sum() - npos * (npos + 1) / 2) / (npos * nneg))


def ece(y, p, bins=10):
    y, p = np.asarray(y), np.asarray(p)
    e = 0.0
    for i in range(bins):
        lo, hi = i / bins, (i + 1) / bins
        idx = (p >= lo) & (p < hi) if i < bins - 1 else (p >= lo) & (p <= hi)
        if idx.any():
            e += idx.mean() * abs(y[idx].mean() - p[idx].mean())
    return float(e)


def fit_temperature(z, y):
    best, bt = 1e9, 1.0
    for t in np.exp(np.linspace(np.log(0.2), np.log(5), 200)):
        p = 1 / (1 + np.exp(-z / t))
        nll = -np.mean(y * np.log(p + 1e-9) + (1 - y) * np.log(1 - p + 1e-9))
        if nll < best:
            best, bt = nll, t
    return float(bt)


def boot(fn, y, p, n=1000):
    rng = np.random.default_rng(SEED)
    vals = []
    for _ in range(n):
        i = rng.integers(0, len(y), len(y))
        if 0 < y[i].sum() < len(i):
            vals.append(fn(y[i], p[i]))
    return [round(float(np.percentile(vals, 2.5)), 4), round(float(np.percentile(vals, 97.5)), 4)]


def sens_spec(y, p, thr):
    y, pred = np.asarray(y), np.asarray(p) >= thr
    return float((pred & (y == 1)).sum() / max(1, (y == 1).sum())), float((~pred & (y == 0)).sum() / max(1, (y == 0).sum()))


if SPLIT_MODE == "grouped":
    split = grouped_split(sorted({c for t in TASKS.values() for c in t["pos"] + t["neg"]}))
metrics = {"model_version": MODEL_VERSION, "backbone": "torchvision mobilenet_v3_small, ImageNet-1k weights (BSD-3-Clause torchvision)",
           "split": {"grouped": "near-duplicate-grouped 2-fold (256-bit dHash, Hamming<=20 union-find per class; whole groups per fold): fold 0 train/val (85/15), fold 1 held-out test",
                     "official": "official HyperKvasir 2_fold_split.csv: fold 0 train/val (85/15), fold 1 held-out test (frame-level; near-duplicates leak across folds)"}[SPLIT_MODE],
           "split_mode": SPLIT_MODE, "seed": SEED, "tasks": {}}
verdicts = {}
all_hash = {}
for task, spec in TASKS.items():
    t0 = time.time()
    items = {0: [], 1: []}
    for y, classes in ((1, spec["pos"]), (0, spec["neg"])):
        for c in classes:
            for p in files(c):
                items[split[os.path.basename(p)]].append((p, y, c))
    random.shuffle(items[0])
    nval = int(0.15 * len(items[0]))
    val, train, test = items[0][:nval], items[0][nval:], items[1]
    # near-duplicate leakage: test images within Hamming 4 of any train/val image
    for p, _, _ in items[0] + items[1]:
        if p not in all_hash:
            all_hash[p] = dhash(p)
    tr_h = np.array([all_hash[p] for p, _, _ in items[0]], dtype=np.uint64)
    near = []
    for p, _, _ in test:
        h = np.uint64(all_hash[p])
        x = np.bitwise_xor(tr_h, h)
        d = np.array([bin(int(v)).count("1") for v in x])
        near.append(bool((d <= 4).any()))
    near = np.array(near)

    m = make_model().to(DEV)
    pos = sum(y for _, y, _ in train)
    lossf = nn.BCEWithLogitsLoss(pos_weight=torch.tensor((len(train) - pos) / max(1, pos), device=DEV))
    opt = torch.optim.AdamW(m.parameters(), lr=3e-4, weight_decay=1e-4)
    EPOCHS = 8
    sched = torch.optim.lr_scheduler.CosineAnnealingLR(opt, EPOCHS)
    dl = torch.utils.data.DataLoader(DS([(p, y) for p, y, _ in train], TF_TRAIN), batch_size=32, shuffle=True, num_workers=0,
                                     generator=torch.Generator().manual_seed(SEED))
    best_auc, best_state = -1, None
    for ep in range(EPOCHS):
        m.train()
        for x, y in dl:
            opt.zero_grad()
            loss = lossf(m(x.to(DEV)).squeeze(1), y.to(DEV))
            loss.backward(); opt.step()
        sched.step()
        zv = logits(m, [(p, y) for p, y, _ in val])
        a = auc(np.array([y for _, y, _ in val]), zv)
        print(f"{task} epoch {ep} val AUC {a:.4f}", flush=True)
        if a > best_auc:
            best_auc, best_state = a, {k: v.detach().cpu().clone() for k, v in m.state_dict().items()}
    m.load_state_dict(best_state)
    yv = np.array([y for _, y, _ in val]); zv = logits(m, [(p, y) for p, y, _ in val])
    T = fit_temperature(zv, yv)
    pv = 1 / (1 + np.exp(-zv / T))
    # operating point on val: landmark -> specificity >= 0.95 ("caecal photo present" must rarely be said of a non-caecal photo);
    # prep -> specificity >= 0.95 for "adequate" (an inadequate prep must rarely be read as adequate)
    thr = 0.5
    for t in np.linspace(0.01, 0.99, 99):
        if sens_spec(yv, pv, t)[1] >= 0.95:
            thr = float(t); break
    yt = np.array([y for _, y, _ in test]); zt = logits(m, [(p, y) for p, y, _ in test])
    pt = 1 / (1 + np.exp(-zt / T))
    se, sp = sens_spec(yt, pt, thr)
    r = {"n_train": len(train), "n_val": len(val), "n_test": len(test), "test_pos": int(yt.sum()), "classes": spec,
         "temperature": round(T, 4), "threshold": round(thr, 3), "operating_point_rule": "first threshold with val specificity >= 0.95",
         "test_auc": round(auc(yt, pt), 4), "test_auc_ci95": boot(auc, yt, pt),
         "test_sensitivity": round(se, 4), "test_sensitivity_ci95": boot(lambda a, b: sens_spec(a, b, thr)[0], yt, pt),
         "test_specificity": round(sp, 4), "test_specificity_ci95": boot(lambda a, b: sens_spec(a, b, thr)[1], yt, pt),
         "test_ece": round(ece(yt, pt), 4), "test_ece_ci95": boot(ece, yt, pt),
         "near_duplicate_test_images": int(near.sum()),
         "test_auc_without_near_duplicates": round(auc(yt[~near], pt[~near]), 4) if (~near).sum() and 0 < yt[~near].sum() < (~near).sum() else None,
         "train_seconds": round(time.time() - t0, 1)}
    metrics["tasks"][task] = r
    print(task, json.dumps(r), flush=True)
    torch.save({"state_dict": best_state, "temperature": T, "threshold": thr, "task": task, "model_version": MODEL_VERSION},
               os.path.join(OUT, f"{task}.pt"))
    m.eval().cpu()
    torch.onnx.export(m, torch.zeros(1, 3, 224, 224), os.path.join(OUT, f"{task}.onnx"), input_names=["image"], output_names=["logit"],
                      dynamic_axes={"image": {0: "n"}, "logit": {0: "n"}}, opset_version=17, dynamo=False)
    m.to(DEV)
    for (p, y, c), prob in zip(test, pt):
        v = verdicts.setdefault(sha(p), {"file": os.path.basename(p), "hk_class": c, "fold": 1})
        v[task] = {"p": round(float(prob), 4), "label": int(y), "positive": bool(prob >= thr)}

# every cached test image gets both readers' outputs (a caecal photo also shows prep; a prep photo may show the caecum)
mods = {}
for task in TASKS:
    ck = torch.load(os.path.join(OUT, f"{task}.pt"), map_location="cpu")
    mm = make_model(); mm.load_state_dict(ck["state_dict"]); mm.to(DEV)
    mods[task] = (mm, ck["temperature"], ck["threshold"])
paths = {}
for task, spec in TASKS.items():
    for c in spec["pos"] + spec["neg"]:
        for p in files(c):
            if split[os.path.basename(p)] == 1:
                paths[sha(p)] = p
shas = sorted(paths)
for task, (mm, T, thr) in mods.items():
    z = logits(mm, [(paths[s], 0) for s in shas])
    for s, zz in zip(shas, z):
        v = verdicts[s]
        if task not in v:
            prob = 1 / (1 + np.exp(-zz / T))
            v[task] = {"p": round(float(prob), 4), "label": None, "positive": bool(prob >= thr)}
metrics["onnx_bytes"] = {t: os.path.getsize(os.path.join(OUT, f"{t}.onnx")) for t in TASKS}
metrics["weights_sha256"] = {f"{t}.onnx": hashlib.sha256(open(os.path.join(OUT, f"{t}.onnx"), "rb").read()).hexdigest() for t in TASKS}
json.dump(metrics, open(os.path.join(OUT, "metrics.json"), "w"), indent=1)
json.dump({"model_version": MODEL_VERSION, "source": "HyperKvasir fold-1 test images; verdicts only, no pixels",
           "thresholds": {t: metrics["tasks"][t]["threshold"] for t in TASKS}, "images": verdicts},
          open(os.path.join(OUT, "verdicts.json"), "w"), indent=0, sort_keys=True)
print("done", len(verdicts), "cached verdicts")
