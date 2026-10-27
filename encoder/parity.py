"""Check that the PyTorch model reproduces the Go encoder (pipeline/model.go) for a weights file.

Usage: python encoder/parity.py <encoder.json> <wsi7 binary> <work dir> [n_names=50] [n_norm=5000]
Embeds n_names names from data/persons3.json (Arabic + Latin + aliases, including Urdu-script and accented
names) with both implementations, and checks normaliser equality on n_norm more names.
Pass criterion: min cosine > 0.9999 and every normalised string identical.
"""
import json
import os
import random
import subprocess
import sys

import numpy as np

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import wenc  # noqa: E402


def sample_names(n, seed):
    ps = json.load(open(os.path.join(os.path.dirname(__file__), "..", "data", "persons3.json")))
    r = random.Random(seed)
    pool = []
    for p in ps:
        pool += [p["ar"], p["en"]] + list(p["als"] or [])
    pool = [x for x in pool if x]
    hard = [x for x in pool if any(c in x for c in "پچگےہںٹ") or any(ord(c) > 0x7f and not wenc.is_ar(c) for c in x)]
    k = n // 3
    out = r.sample(hard, k) + r.sample(pool, n - k)
    out += ["O'Brien al-Sayyid", "ABDUL RAHMAN bin Khalid", "عبد الله بن عبد العزيز آل سعود", "Abu   Bakr", "José María Ñúñez"]
    return out


def main(enc_path, wsi, work, n=50, n_norm=5000):
    os.makedirs(work, exist_ok=True)
    names = sample_names(n + n_norm, 11)
    inp = os.path.join(work, "parity_in.txt")
    outp = os.path.join(work, "parity_go.json")
    with open(inp, "w") as f:
        f.write("\n".join(x.replace("\n", " ") for x in names) + "\n")
    subprocess.run([wsi, "encembed", "-encfile", enc_path, "-in", inp, "-out", outp], check=True)
    go = json.load(open(outp))
    assert len(go) == len(names), (len(go), len(names))
    bad = [(g["name"], g["norm"], wenc.norm_name(g["name"])) for g in go if g["norm"] != wenc.norm_name(g["name"])]
    m = wenc.load_json(enc_path, dropout=0.0).eval()
    sel = list(range(n)) + list(range(len(names) - 5, len(names)))
    pe = m.embed_raw([go[i]["name"] for i in sel])
    ge = np.array([go[i]["emb"] for i in sel], np.float32)
    cos = (pe * ge).sum(1) / np.linalg.norm(pe, axis=1) / np.linalg.norm(ge, axis=1)
    res = {"n_embed": len(sel), "min_cos": float(cos.min()), "mean_cos": float(cos.mean()),
           "n_norm": len(names), "norm_mismatch": len(bad), "examples": bad[:5]}
    print(json.dumps(res, ensure_ascii=False, indent=1))
    ok = cos.min() > 0.9999 and not bad
    print("PARITY", "OK" if ok else "FAIL")
    return ok


if __name__ == "__main__":
    a = sys.argv
    ok = main(a[1], a[2], a[3], int(a[4]) if len(a) > 4 else 50, int(a[5]) if len(a) > 5 else 5000)
    sys.exit(0 if ok else 1)
