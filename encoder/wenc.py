"""Shared code for the Wasl Arabic<->Latin name encoder: normalisers (ports of pipeline/text.go),
the dual-tower PyTorch model (mirrors pipeline/model.go Embed), JSON import/export, and the
synthetic registration noise (port of pipeline/gen.go noisyLatin/noisyArabic, used only as augmentation).
"""
import json
import random
import re
import unicodedata

import numpy as np
import torch
import torch.nn as nn
import torch.nn.functional as F

# ---------------- normalisation (pipeline/text.go) ----------------
URDU = {'پ': 'ب', 'چ': 'ج', 'ژ': 'ز', 'ڤ': 'ف', 'گ': 'غ', 'ٹ': 'ت', 'ڈ': 'د', 'ڑ': 'ر', 'ں': 'ن', 'ہ': 'ه',
        'ۀ': 'ه', 'ۃ': 'ه', 'ے': 'ي', 'ۓ': 'ي', 'ڭ': 'غ', 'ڨ': 'ق', 'ݣ': 'غ'}
AR_MAP = {'أ': 'ا', 'إ': 'ا', 'آ': 'ا', 'ٱ': 'ا', 'ى': 'ي', 'ة': 'ه', 'ؤ': 'و', 'ئ': 'ي', 'ی': 'ي', 'ک': 'ك', 'ھ': 'ه'}
AR_PREF = {"عبد": "عبد", "ابو": "ابو"}
LAT_PREF = {"abd": "abdal", "abdel": "abdal", "abdul": "abdal", "abdal": "abdal", "abdol": "abdal", "abu": "abu", "abou": "abu"}


def is_ar(s):
    return any(0x0600 <= ord(c) <= 0x06FF for c in s)


def join_prefix(toks, pref):
    out, carry = [], ""
    for t in toks:
        if t in pref:
            carry += pref[t]
            continue
        out.append(carry + t)
        carry = ""
    if carry:
        out.append(carry)
    return out


def norm_ar(s, urdu_fix=True):
    b = []
    for c in s:
        if urdu_fix and c in URDU:
            c = URDU[c]
        o = ord(c)
        if 0x064B <= o <= 0x0652 or o == 0x0670 or o == 0x0640:
            continue
        c = AR_MAP.get(c, c)
        o = ord(c)
        if not (0x0621 <= o <= 0x064A or c == ' '):
            c = ' '
        b.append(c)
    return " ".join(join_prefix("".join(b).split(), AR_PREF))


def norm_lat(s):
    s = unicodedata.normalize("NFKD", s)
    b = []
    for c in s:
        if unicodedata.category(c) == "Mn":
            continue
        c = c.lower() if len(c.lower()) == 1 else c
        if c in "'`ʿʾ":
            continue
        if not ('a' <= c <= 'z' or c == ' '):
            c = ' '
        b.append(c)
    return " ".join(join_prefix("".join(b).split(), LAT_PREF))


def norm_name(s):
    return norm_ar(s) if is_ar(s) else norm_lat(s)


# ---------------- model (pipeline/model.go) ----------------
class Tower(nn.Module):
    def __init__(self, vocab, L, DM, NH, FF, NL, OUT, dropout=0.1):
        super().__init__()
        self.emb = nn.Embedding(vocab, DM)
        self.pos = nn.Embedding(L, DM)
        self.layers = nn.ModuleList([nn.TransformerEncoderLayer(DM, NH, FF, dropout, activation="gelu",
                                                                batch_first=True, norm_first=True) for _ in range(NL)])
        self.ln = nn.LayerNorm(DM)
        self.proj = nn.Linear(DM, OUT)

    def forward(self, ids):  # ids: (B, T), 0 = pad
        pad = ids == 0
        h = self.emb(ids) + self.pos(torch.arange(ids.shape[1], device=ids.device))[None]
        for lay in self.layers:
            h = lay(h, src_key_padding_mask=pad)
        h = self.ln(h)
        m = (~pad).float()[..., None]
        pooled = (h * m).sum(1) / m.sum(1)
        return F.normalize(self.proj(pooled), dim=-1)


class DualEncoder(nn.Module):
    def __init__(self, cfg, dropout=0.1):
        super().__init__()
        self.cfg = dict(cfg)
        c = cfg
        self.ar = Tower(len(c["AR_CH"]) + 2, c["L"], c["DM"], c["NH"], c["FF"], c["NL"], c["OUT"], dropout)
        self.lat = Tower(len(c["LAT_CH"]) + 2, c["L"], c["DM"], c["NH"], c["FF"], c["NL"], c["OUT"], dropout)
        self.logit_scale = nn.Parameter(torch.tensor(float(np.log(20.0))))
        self.av = {ch: i + 2 for i, ch in enumerate(c["AR_CH"])}
        self.lv = {ch: i + 2 for i, ch in enumerate(c["LAT_CH"])}

    def ids(self, normed, ar):
        """normed: already-normalised string -> token ids (start token 1, unknown chars skipped, cap L)."""
        v = self.av if ar else self.lv
        out = [1]
        for ch in normed:
            if ch in v and len(out) < self.cfg["L"]:
                out.append(v[ch])
        return out

    def batch(self, names, ar, device):
        seqs = [self.ids(n, ar) for n in names]
        T = min(self.cfg["L"], -(-max(len(s) for s in seqs) // 8) * 8)  # few distinct shapes (MPS graph cache)
        x = torch.zeros(len(seqs), T, dtype=torch.long)
        for i, s in enumerate(seqs):
            x[i, :len(s)] = torch.tensor(s)
        return x.to(device)

    def encode(self, normed, ar, device="cpu", bs=1024):
        """Embed already-normalised names of one script; returns float32 numpy (N, OUT)."""
        tw = self.ar if ar else self.lat
        out = []
        with torch.no_grad():
            for i in range(0, len(normed), bs):
                out.append(tw(self.batch(normed[i:i + bs], ar, device)).float().cpu().numpy())
        return np.concatenate(out) if out else np.zeros((0, self.cfg["OUT"]), np.float32)

    def embed_raw(self, names, device="cpu"):
        """Embed raw names (any script), routing like Go Embed."""
        res = np.zeros((len(names), self.cfg["OUT"]), np.float32)
        ai = [i for i, n in enumerate(names) if is_ar(n)]
        li = [i for i, n in enumerate(names) if not is_ar(n)]
        if ai:
            res[ai] = self.encode([norm_ar(names[i]) for i in ai], True, device)
        if li:
            res[li] = self.encode([norm_lat(names[i]) for i in li], False, device)
        return res


def load_json(path, dropout=0.1):
    j = json.load(open(path))
    m = DualEncoder(j["cfg"], dropout)
    sd = {k: torch.tensor(np.array(v, dtype=np.float32)) for k, v in j["w"].items()}
    missing, unexpected = m.load_state_dict(sd, strict=False)
    assert not unexpected, unexpected
    assert not missing, missing
    return m


def save_json(m, path):
    w = {}
    for k, v in m.state_dict().items():
        a = v.detach().float().cpu().numpy()
        w[k] = np.round(a.astype(np.float64), 7).tolist() if a.ndim else round(float(a), 7)
    json.dump({"cfg": m.cfg, "w": w}, open(path, "w"), separators=(",", ":"))


# ---------------- synthetic noise (port of pipeline/gen.go v1) ----------------
KEYB = "qwertyuiopasdfghjklzxcvbnm"
_AL_DASH = re.compile(r"\b[Aa]l-")


def typo(s, r):
    if len(s) < 3:
        return s
    i = r.randrange(len(s))
    op = r.random()
    if op < .3:
        return s[:i] + r.choice(KEYB) + s[i + 1:]
    if op < .55:
        return s[:i] + s[i + 1:]
    if op < .8:
        return s[:i] + r.choice("aeiouy") + s[i:]
    if i < len(s) - 1:
        return s[:i] + s[i + 1] + s[i] + s[i + 2:]
    return s


def drop_middle(toks, r):
    if len(toks) >= 3 and r.random() < .2:
        k = 1 + r.randrange(len(toks) - 2)
        toks = toks[:k] + toks[k + 1:]
    return toks


def noisy_latin(name, r):
    s = " ".join(drop_middle(name.split(), r))
    x = r.random()
    if x < .25:
        s = _AL_DASH.sub("Al ", s)
    elif x < .4:
        s = _AL_DASH.sub("Al", s)
    elif x < .5:
        s = _AL_DASH.sub("", s)
    if r.random() < .25:
        s = typo(s, r)
    if r.random() < .1:
        s = s.upper()
    return s


def noisy_arabic(name, r):
    s = " ".join(drop_middle(name.split(), r))
    if r.random() < .3:
        s = s.replace("أ", "ا").replace("إ", "ا").replace("آ", "ا")
    if r.random() < .2:
        s = s.replace("ة", "ه")
    if r.random() < .2:
        s = s.replace("عبد ال", "عبدال")
    if r.random() < .08 and len(s) > 4:
        i = r.randrange(len(s))
        s = s[:i] + s[i + 1:]
    return s


def device():
    return "mps" if torch.backends.mps.is_available() else "cpu"
