# Image reader data: HyperKvasir

> **Competition use is pending written permission from SimulaMet. The weights are not distributed.**
> HyperKvasir is licensed CC BY 4.0. The dataset's Terms of Use (GitHub README) also say: "released fully open for research and educational purposes. The use of the dataset for purposes such as competitions and commercial purposes needs prior written permission."
> A hackathon may count as a competition, and we have not asked for or received that permission.
>
> **What this repository commits:** code, download and training scripts, metric results, sha256 hashes, and `models/image_verdicts.json`. That file holds per-image *scores* keyed by sha256, plus the dataset class label, and **no pixels**.
>
> **What it does not commit:** HyperKvasir images, pixel-derived caches, and trained weights.

**Source.** Borgli H., Thambawita V., Smedsrud P.H. et al. *HyperKvasir, a comprehensive multi-class image and video dataset for gastrointestinal endoscopy.* Scientific Data 7, 283 (2020). doi:10.1038/s41597-020-00622-y. Data: https://osf.io/mh9sj/ · https://datasets.simula.no/hyper-kvasir/

**Classes used** (labelled images, lower GI tract):

| Class | Images | Role | OSF folder id |
|---|---|---|---|
| anatomical-landmarks/cecum | 1,009 | landmark positive | 5dfb5b530236b8000c746a8e |
| anatomical-landmarks/retroflex-rectum | 391 | landmark negative | 5dfb5bb70236b8000c746b70 |
| pathological-findings/polyps | 1,028 | landmark negative | 5dfb5ba6fad251000c6a3034 |
| pathological-findings/ulcerative-colitis-grade-2 | 443 | landmark negative | 5dfb5c740236b8000c746d21 |
| quality-of-mucosal-views/bbps-2-3 | 1,148 | prep positive (adequate) | 5dfb5a9cfad251000c6a2e8d |
| quality-of-mucosal-views/bbps-0-1 | 646 | prep negative (inadequate) | 5dfb5a890236b8000c7468b9 |

**Why these landmark negatives:** they are the other colon views a photo filed under "caecum" could be in practice: a rectal retroflexion, a polyp close-up, or inflamed mucosa. Ileum (9 images) is too small to use. Upper-GI landmarks (pylorus, z-line) were left out because a lower-GI report would not contain them, so they would make the task artificially easy.

**Download.** Run `image/download.sh <dir>`, about 770 MB. Per-class zips come from `https://files.osf.io/v1/resources/mh9sj/providers/osfstorage/<folder id>/?zip=`; HEAD returns 501, but GET works. Labels come from `https://osf.io/download/67y5h/`, and the official split from `https://raw.githubusercontent.com/simula/hyper-kvasir/master/official_splits/2_fold_split.csv`.

| File (downloaded 2026-10-27) | sha256 |
|---|---|
| bbps-0-1.zip | cb6d5e4f5d6ede615d001d8538faa9411676999399445c0f48c51b5b27e28644 |
| bbps-2-3.zip | a525ca89353014970f77ad0062075b7e8651eb5767a76c5daf970fdaf233babc |
| cecum.zip | 274bc523baf45950a7cd8c4dbf531cb7212d4ff2663a4a52c0cf616f21823b21 |
| polyps.zip | a3ada408b16f8a96a5800b9932746343f84afc4d2ef98a799c6bbb1c7cab5cbd |
| retroflex-rectum.zip | f4c34fe7ec429d67dc5e20906afba9412e66c12e281e3a4b94acde4a59b7765e |
| ulcerative-colitis-grade-2.zip | 022a7336d2ada96edd6672358310725d9acf2419b2b7816a26df543b848ddd49 |
| image-labels.csv | a4a6054c00129814b6261c8b3e60305c14a4ff4b34ad988c3bdafe204bfa487c |
| 2_fold_split.csv | e9bc264da8dd4ce5ebd6654080eb9ec8a00fd28c3d03a75a2c80b1eb5a096c06 |

**Leakage control.** HyperKvasir images are frames from endoscopy videos, and the dataset publishes no patient or video ids. The official 2-fold split is frame-level, so near-identical consecutive frames land in both folds. In BBPS 2-3, 72% of the official fold-1 images are within 256-bit dHash Hamming distance 20 of a fold-0 image. So the primary split is near-duplicate-grouped (`image/train.py grouped`):
- **Grouping:** within each class, a 256-bit dHash with union-find links images at Hamming ≤20.
- **Fold assignment:** whole groups go to one fold, by sha256 of the group key.
- **Group counts:** BBPS 2-3 forms 266 groups from 1,148 images, BBPS 0-1 362 from 646, and caecum 1,006 from 1,009.
- **Official-split check:** metrics on the official split are kept for comparison (`image/metrics_official_split.json`).

**Backbone.** torchvision `mobilenet_v3_small`, initialised from ImageNet-1k weights. The torchvision code is BSD-3-Clause; the ImageNet-derived weights carry ImageNet's research terms. Another reason we don't distribute weights.

**Weights hashes.** Locally trained ONNX (6.1 MB each): see `weights_sha256` in `image/metrics.json` and `models/image_verdicts.json`. Anyone with permission can reproduce them with `image/train.py`.
