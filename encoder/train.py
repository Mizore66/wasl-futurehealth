"""Retrain the Wasl Arabic<->Latin name encoder on REAL name-variant pairs.

Training items (train split only; dev selects the checkpoint, cal/eval are never touched):
  - persons3 train persons: every real Arabic label (persons3 + Wikidata ar/arz) x every real Latin label/alias
    (persons3 en/als + Wikidata en/fr/de/es/it/nl/tr/id/ms). Each epoch samples one Arabic and one Latin variant per person.
  - NETransliteration (Merhav & Ash 2018, CC0) English<->Arabic Wikidata name pairs, minus anything matching a
    dev/cal/eval person (see data.load_netrans).
  - Synthetic registration noise (gen.go v1 model) applied on top as augmentation with probability --noise.
Loss: symmetric InfoNCE across scripts + same-script InfoNCE (two independently sampled/noised variants), learnable
temperature, false negatives (identical normalised strings across items) masked.
Hard negatives: from epoch --hard-from, a fraction --hard of batches is built from neighbourhoods in the model's own
index (cross-script kNN over canonical names), with same-surname neighbours placed first.

Usage: python encoder/train.py --variants V.jsonl --netrans wd_arabic --wsi bin/wsi7 --work DIR --out DIR [--init enc.json]
"""
import argparse
import json
import math
import os
import random
import sys
import time

import numpy as np
import torch
import torch.nn.functional as F

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import data  # noqa: E402
import eval as ev  # noqa: E402
import wenc  # noqa: E402

CFG = {"L": 40, "DM": 128, "NH": 4, "FF": 256, "NL": 2, "OUT": 64,
       "AR_CH": " ءآأؤإئابةتثجحخدذرزسشصضطظعغػؼؽؾؿـفقكلمنهوىي", "LAT_CH": " abcdefghijklmnopqrstuvwxyz"}


class Items:
    """item = (list of raw Arabic variants, list of raw Latin variants, surname key)."""

    def __init__(self, people, netrans, use_wd=True, use_nt=True):
        self.ar, self.lat, self.src = [], [], []
        if use_wd:
            for p in sorted(people.values(), key=lambda x: x["qid"]):
                if p["split"] == "train":
                    self.ar.append(p["ar"])
                    self.lat.append(p["lat"])
                    self.src.append("wd")
        if use_nt:
            for en, ar in netrans:
                self.ar.append([ar])
                self.lat.append([en])
                self.src.append("nt")
        self.sur = []
        for lv in self.lat:
            t = wenc.norm_lat(lv[0]).split()
            self.sur.append(t[-1] if t else "")
        self.n = len(self.ar)


def sample_side(variants, r, noise_p, ar):
    v = r.choice(variants)
    if r.random() < noise_p:
        v = wenc.noisy_arabic(v, r) if ar else wenc.noisy_latin(v, r)
    n = wenc.norm_ar(v) if ar else wenc.norm_lat(v)
    return n if n else (wenc.norm_ar(variants[0]) if ar else wenc.norm_lat(variants[0]))


def info_nce(a, b, scale, mask_a, mask_b):
    """a, b: (B, D) normalised; mask_*: (B, B) bool, True where an off-diagonal pair is a false negative."""
    s = scale * a @ b.T
    fn = mask_a | mask_b
    fn.fill_diagonal_(False)
    s = s.masked_fill(fn, -1e4)
    y = torch.arange(len(a), device=a.device)
    return (F.cross_entropy(s, y) + F.cross_entropy(s.T, y)) / 2


def eq_mask(strs, dev):
    ids = {}
    v = torch.tensor([ids.setdefault(s, len(ids)) for s in strs], device=dev)
    return v[:, None] == v[None, :]


def hard_batches(model, items, bs, group, r, dev, k=32):
    """Neighbourhood batches from the model's own index: Arabic canonical of item i vs Latin canonical of all items."""
    model.eval()
    A = torch.tensor(model.encode([wenc.norm_ar(x[0]) for x in items.ar], True, dev, 4096), device=dev)
    Lm = torch.tensor(model.encode([wenc.norm_lat(x[0]) for x in items.lat], False, dev, 4096), device=dev)
    nbr = np.zeros((items.n, k), np.int64)
    for i in range(0, items.n, 4096):
        s = A[i:i + 4096] @ Lm.T
        s[torch.arange(s.shape[0]), torch.arange(i, i + s.shape[0])] = -2
        nbr[i:i + 4096] = s.topk(k, dim=1).indices.cpu().numpy()
    model.train()
    order = list(range(items.n))
    r.shuffle(order)
    used = np.zeros(items.n, bool)
    groups = []
    for i in order:
        if used[i]:
            continue
        cand = [j for j in nbr[i] if not used[j]]
        cand.sort(key=lambda j: items.sur[j] != items.sur[i])  # same-surname collisions first (stable on similarity)
        g = [i] + cand[:group - 1]
        for j in g:
            used[j] = True
        groups.append(g)
    r.shuffle(groups)
    batches, cur = [], []
    for g in groups:
        cur += g
        if len(cur) >= bs:
            batches.append(cur[:bs])
            cur = []
    return batches


def run(a):
    dev = wenc.device()
    torch.manual_seed(a.seed)
    r = random.Random(a.seed)
    people = data.load_people(a.variants)
    ntlog = {}
    netrans = data.load_netrans(a.netrans, people, ntlog) if a.use_nt else []
    items = Items(people, netrans, a.use_wd, a.use_nt)
    print(json.dumps({"items": items.n, "wd": items.src.count("wd"), "nt": items.src.count("nt"), "netrans": ntlog}), flush=True)
    os.makedirs(a.out, exist_ok=True)
    json.dump({"args": vars(a), "netrans": ntlog, "items": items.n}, open(os.path.join(a.out, "meta.json"), "w"), indent=1)
    model = wenc.load_json(a.init, a.dropout) if a.init else wenc.DualEncoder(CFG, a.dropout)
    model.to(dev).train()
    opt = torch.optim.AdamW(model.parameters(), lr=a.lr, weight_decay=a.wd)
    steps_per = math.ceil(items.n / a.bs)
    total = steps_per * a.epochs
    warm = steps_per
    sched = torch.optim.lr_scheduler.LambdaLR(opt, lambda s: min(1, (s + 1) / warm) * 0.5 * (1 + math.cos(math.pi * min(1, s / total))))
    best, log = -1, []
    for ep in range(a.epochs):
        t0 = time.time()
        order = list(range(items.n))
        r.shuffle(order)
        rand_b = [order[i:i + a.bs] for i in range(0, items.n, a.bs)]
        batches = rand_b
        if a.hard > 0 and ep >= a.hard_from:
            hb = hard_batches(model, items, a.bs, a.group, r, dev)
            nh = int(len(rand_b) * a.hard)
            batches = rand_b[:len(rand_b) - nh] + hb[:nh]
            r.shuffle(batches)
        tot, nb = 0.0, 0
        for b in batches:
            if len(b) < 8:
                continue
            ar1 = [sample_side(items.ar[i], r, a.noise, True) for i in b]
            la1 = [sample_side(items.lat[i], r, a.noise, False) for i in b]
            ea = model.ar(model.batch(ar1, True, dev))
            el = model.lat(model.batch(la1, False, dev))
            scale = model.logit_scale.exp().clamp(max=100)
            ma, ml = eq_mask(ar1, dev), eq_mask(la1, dev)
            loss = info_nce(ea, el, scale, ma, ml)
            if a.same > 0:
                ar2 = [sample_side(items.ar[i], r, max(a.noise, 0.5), True) for i in b]
                la2 = [sample_side(items.lat[i], r, max(a.noise, 0.5), False) for i in b]
                ea2 = model.ar(model.batch(ar2, True, dev))
                el2 = model.lat(model.batch(la2, False, dev))
                loss = loss + a.same * 0.5 * (info_nce(ea, ea2, scale, ma, eq_mask(ar2, dev)) +
                                              info_nce(el, el2, scale, ml, eq_mask(la2, dev)))
            opt.zero_grad()
            loss.backward()
            torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
            opt.step()
            sched.step()
            tot += loss.item()
            nb += 1
        rec = {"epoch": ep, "loss": tot / max(nb, 1), "sec": round(time.time() - t0, 1), "scale": float(model.logit_scale.exp())}
        if (ep + 1) % a.eval_every == 0 or ep == a.epochs - 1:
            model.eval()
            res = ev.evaluate(model, people, "dev", a.wsi, a.work, dev)
            model.train()
            score = res["real"]["R@1"] + res["syn_v1"]["R@1"] + 0.5 * (res["same_lat"]["R@1"] + res["same_ar"]["R@1"])
            rec |= {"dev_real_R1": res["real"]["R@1"], "dev_real_R10": res["real"]["R@10"], "dev_syn_v1_R1": res["syn_v1"]["R@1"],
                    "dev_syn_v1_R10": res["syn_v1"]["R@10"], "dev_syn_v2_R1": res["syn_v2"]["R@1"],
                    "dev_same_lat_R1": res["same_lat"]["R@1"], "dev_same_ar_R1": res["same_ar"]["R@1"], "score": score}
            if score > best:
                best = score
                m2 = wenc.DualEncoder(CFG, 0.0)
                m2.load_state_dict({k: v.detach().cpu() for k, v in model.state_dict().items()})
                wenc.save_json(m2, os.path.join(a.out, "best.json"))
                rec["saved"] = True
        log.append(rec)
        print(json.dumps(rec), flush=True)
        json.dump(log, open(os.path.join(a.out, "log.json"), "w"), indent=1)
    return best


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--variants", required=True)
    ap.add_argument("--netrans", required=True)
    ap.add_argument("--wsi", required=True)
    ap.add_argument("--work", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--init", default="")
    ap.add_argument("--epochs", type=int, default=40)
    ap.add_argument("--bs", type=int, default=512)
    ap.add_argument("--lr", type=float, default=5e-4)
    ap.add_argument("--wd", type=float, default=0.01)
    ap.add_argument("--dropout", type=float, default=0.1)
    ap.add_argument("--noise", type=float, default=0.5, help="P(apply synthetic registration noise) per side")
    ap.add_argument("--same", type=float, default=0.5, help="weight of the same-script InfoNCE terms")
    ap.add_argument("--hard", type=float, default=0.5, help="fraction of batches built from hard-negative neighbourhoods")
    ap.add_argument("--hard-from", type=int, default=3)
    ap.add_argument("--group", type=int, default=8)
    ap.add_argument("--eval-every", type=int, default=2)
    ap.add_argument("--seed", type=int, default=1)
    ap.add_argument("--no-wd", dest="use_wd", action="store_false")
    ap.add_argument("--no-nt", dest="use_nt", action="store_false")
    run(ap.parse_args())


if __name__ == "__main__":
    main()
