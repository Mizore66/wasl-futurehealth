# Wasl tech-depth upgrades: feasibility (2026-10-23)

## 1. Image evidence reader
**HyperKvasir** (CC BY 4.0; counts from the official image-labels.csv; sizes from the OSF folders):

| Class | Images | Size |
|---|---|---|
| cecum | 1,009 | 235 MB |
| ileum | **9** | 2 MB |
| retroflex-rectum | 391 | 84 MB |
| bbps-0-1 | 646 | 120 MB |
| bbps-2-3 | 1,148 | 185 MB |

- Labelled zip 3.9 GB. Each class is a separate OSF folder (osf.io/mh9sj), about 625 MB for all 5. Nothing was downloaded.
- Ileum is unusable; BBPS is binary only.

**Nerthus:**
- 250 MB, 21 videos, 5,525 frames, left colon only, BBPS 0–3.
- No licence posted ("free for research/education" per a Simula thesis); a ScopeGuide overlay is a shortcut risk.

**Others:** GastroVision (cecum F1 0.23); the Montreal AO/ICV set is private.

**Published accuracy:**
- HyperKvasir baseline: micro-F1 0.91, macro-F1 0.62.
- Taha 2023, single centre: ICV AUC 0.98, AO AUC 0.95. Both landmarks were found in only 18/21 patients.
- ENDOANGEL BBPS: 91.9% on images (internal).
- ViENDO: 91.7% on external data.
- Nerthus "99–100%" results reflect frame leakage.

**Verdict:** feasible as a third reader that can only hold, never close.
- A prep classifier on report photos could turn "ungraded" into "image suggests adequate" and send the case to a one-click endoscopist grade. That recovers part of the 0.766→0.273 drop while keeping the DoH rule.
- Missing caecal photos could be flagged.
- Domain shift: Norwegian, Olympus, single-centre video frames vs UAE compressed stills from other scopes; local recalibration is mandatory.

## 2. Conformal risk control for the gate
- **CRC** (Angelopoulos et al., ICLR 2024) bounds only expected loss: too weak for safety.
- **Learn-then-Test** (Angelopoulos, Bates et al., AoAS 2025) is the right tool. It gives P(false-closure rate ≤ α) ≥ 1−δ by testing each threshold with exact binomial or Hoeffding–Bentkus p-values, using fixed-sequence testing from strict to loose.
- **What changes:** isotonic regression stays as the score. The hand-set p≥0.99, the 0.989 cap and the Clopper–Pearson bound are replaced by an LTT-chosen threshold, with one run per risk (wrong patient, false adequate) and δ split by Bonferroni.
- **Gain:** error-controlled threshold selection, answering "rules chosen after seeing eval errors".
- **Calibration n for α = 0.001, δ = 0.05** (computed here):

| Errors observed | Calibration loops needed |
|---|---|
| 0 | 2,995 |
| 1 | 4,742 |
| 2 | 6,294 |


- The existing 0/8,422 would qualify if held out from rule design. Per-origin guarantees need ~3k per stratum; synthetic-only until real data exists.
- **Prior work:** no CRC/LTT application to record linkage was found. The nearest is conformal link prediction with FDR control (Marandon 2024). This would be novel.

## 3. Where an optimiser beats greedy
- Patrick–Puterman–Queyranne (OR 2008): multi-priority ADP. Berg–Denton (C&OR 2014): two-stage MIP for endoscopy, 13–24% no-shows.
- EJOR 2019 multi-provider model: 12% cost gain, 16% in a live trial. Gains grow with more providers and variability.
- Overbooking (2017): big gains only when capacity is below demand. POMS 2023 kept a heuristic. No colonoscopy-vs-FCFS figure found.

**Implication:** Wasl ties because capacity (~1.5 slots per FIT+) meets demand. Sweep 0.8–1.5 slots with scarce female lists and per-patient no-shows; expect ~10–20% wins only when scarce, and report the crossover.

## 4. Encoder
- **Symphonym** (arXiv 2601.06932): phonetic teacher, then student distillation, then hard negatives.
  - R@1 86.7% vs Levenshtein 81.5% (Hebrew–Arabic).
  - Cross-script R@1 0.959 vs 0.064.
  - Toponyms only; weights on Hugging Face.
- Byte-level 4M InfoNCE model with ANCE hard negatives (blog, not peer-reviewed).
- Nothing published beats a matcher ensemble on Arabic↔Latin *person* names.
- **Data:**

| Dataset | Size | Licence |
|---|---|---|
| JRC-Names | 307k entities + 333k variants; ~19.5k Arabic persons | "freely available" (verify) |
| ANETAC | 79,924 EN–AR pairs | unclear |
| TRANSLIT | 1.6M entities | not checked |

  ANERcorp is NER, not transliteration pairs.
- **Upgrade:** retrain on real JRC-Names and ANETAC person pairs, with hard negatives mined from the encoder's own index (same-surname collisions). Test on held-out real variants.
  - Fixes "trained on the eval's noise model" and keeps the 2.3 MB Go footprint.
