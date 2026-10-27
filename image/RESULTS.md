# Image evidence reader (third reader, hold-only): results

> **Competition use: written permission received from the HyperKvasir authors (SimulaMet), October 2026**, for this hackathon submission (non-commercial). See [DATA.md](DATA.md).
> Everything below was trained and evaluated locally, for research and evaluation.

**Status:** built (code, training, Go wiring, tests) and evaluated on HyperKvasir held-out images. The pipeline wiring is exercised on a **simulated** cohort whose reports carry synthetic photo references.

## What it does
There are two small CNNs: torchvision MobileNetV3-small with ImageNet weights, fine-tuned for 8 epochs, 6.1 MB as ONNX each.
- **landmark:** is a photo filed under the "caecum" capture station really the caecum? DoH CRC Programme Specifications App. 11 §2.2: "Auditable photo documentation of colonoscopy completion must be available".
- **prep:** does a mucosal-view photo show adequate cleansing (BBPS 2–3) or inadequate (BBPS 0–1)?

The reader can **only hold or route**:
- **No closures:** it never creates two-reader agreement and never produces an "adequate" label. This is tested in `pipeline/image_test.go`, over every label × agreement × field × image-verdict combination.
- **Prep ungraded in the report, photos look adequate:** routes to a `prep_grade_one_click` review item. The endoscopist confirms a grade with the photo shown, and the loop still closes only on the human sign-off.
- **Prep ungraded, photos look inadequate:** routes to an `image_prep_inadequate` review item (full review).
- **Text says prep adequate, photos say inadequate:** **hold** (`image_prep_conflict`); the label goes from adequate to disagree, then to review.
- **Report documents caecum reached, but no photo is verified as caecal:** audit flag `missing_caecal_photo`.
  - It is not a closure blocker. The closure rule already requires completion documented in the report.
  - The Specifications audit photo evidence at unit level: App. 2 "CIR … with video recorded and photographic evidence".
  - Making it a blocker is a DoH policy choice we don't make.

Every evidence provenance block stamps the image model version, its verdict (`caecal_photo`, `landmark_p`, `prep`, `prep_p`), the photo hashes, and the label before the image reader.

**Modes:**
- `-image cached` (default when on) reads `models/image_verdicts.json`, keyed by sha256 with no pixels, so runs are deterministic with no model.
- `-image live` calls the `image/serve.py` sidecar and writes `<run>/image_ledger.jsonl`. Audit and report replay that ledger.
- `-image off` is the default, so existing results are unchanged.
- Sidecar vs cache: max |Δp| = 0.0001 over 40 images.

## Held-out image metrics (fold 1, near-duplicate-grouped split; 95% bootstrap CIs, 1,000 resamples)

The operating point is the first threshold with validation specificity ≥0.95. Probabilities are temperature-scaled on validation.

| | Landmark (caecum vs other lower-GI views) | Prep (BBPS 2–3 vs 0–1) |
|---|---|---|
| Train / test images | 1,240 / 1,413 (498 caecum) | 709 / 961 (606 adequate) |
| AUC | **0.998** (0.995–0.999) | **0.991** (0.987–0.995) |
| Sensitivity | 0.994 (0.986–1.000) | 0.998 (0.995–1.000) |
| Specificity | 0.960 (0.946–0.972) | **0.834** (0.797–0.870) |
| ECE | 0.011 (0.008–0.020) | 0.061 (0.049–0.074) |
| Threshold | 0.12 | 0.25 |
| AUC without test images near-duplicate to training | 0.998 | 0.993 |

**Leakage.** On HyperKvasir's official frame-level split, prep specificity is 0.938 and its AUC rises to 0.9999 once near-duplicates are excluded (`metrics_official_split.json`). That gain is near-duplicate video frames shared across folds. The grouped split is the honest number. Prep specificity is the weak point: about 1 in 6 inadequate-prep views reads as adequate. That's why an "adequate" photo can only *suggest* a one-click human grade and never closes a loop.

## Integrated pipeline run (simulated cohort, seed 2026, v7strict, cached text reader, FHIR, chaos on)

The same cohort was run with photo references attached (`-imgphotos`; photos only add `DiagnosticReport.media` and leave every other resource byte-identical) and without the image reader. The photo rates are assumptions [A], chosen to exercise every path:
- **Caecal photo:** 85% of caecum-documented reports attach a true caecal photo, 5% a mis-filed non-caecal one, and 10% none.
- **Mucosal views:** 90% of reports attach 2.
- **Prep grade of the photos:** follows the text grade; for ungraded reports a latent grade, 85% adequate.

| | Image off | **Image cached** |
|---|---|---|
| CRC false closures | 0/571 | **0/571** |
| CRC recall (with review) | 0.961 | 0.961 |
| Cervical false closures | 0/217 | 0/217 |
| Wrong-patient messages / messages without consent | 0 / 0 | 0 / 0 |
| Tamper detected | 17/17 | 17/17 |
| Evidence review items | 521 | 522 |
| of which `prep_grade_one_click` (in queue) | – | 321 |
| Evidence reviewer-h per 1k loops | 11.3 | **6.1** [A] |
| `missing_caecal_photo` flags (reports) | – | 152 |
| Labels downgraded by image (adequate → hold) | – | 2 (`image_prep_conflict`) |
| Labels upgraded by image | – | **0** (invariant) |

**Reading the photos against their synthetic truth:**
- **Caecal photos:** 979/984 true caecal photos were verified, and 80/85 mis-filed "caecal" photos were caught.
- **Mucosal pairs:** 154/162 inadequate-prep pairs were read inadequate; 1,058/1,061 adequate pairs were read adequate.

**What the numbers mean:**
- **The hold-only invariant held** in the integrated run: 0 labels upgraded, 0 false closures, and recall unchanged.
- **The reviewer-hours gain is an assumption, not a measurement.** A one-click prep grade with the photo shown is assumed to take a 20-second median, against a 90-second full evidence read (`OneClickMedS`). The simulated reviewer error rate (4%) is unchanged.
- **The flag mostly does what it should:** 152 flags, most of them reports whose only caecal photo was missing or mis-filed. A few come from the reader failing on a true caecal photo (5/984).
- **Run-to-run variance:** outreach-intent and second-identifier counts vary between repeated runs of the *same* cohort, image off included (812 vs 833 intents across two image-off runs; the committed baseline has 925). They depend on chaos timing, so they aren't attributed to the image reader. Closure and safety metrics are identical.

## Limits (state them)
- **Domain shift:** HyperKvasir is Norwegian (Bærum Hospital), Olympus/Pentax video frames. UAE units file compressed stills from other scopes and processors. **Local recalibration on UAE photos is mandatory** before any pilot use; the thresholds here are not transferable.
- **The photos are synthetic attachments.** Real reports may carry no photos, photos in other stations, or PDFs; we have no UAE photo documentation rates.
- **No live UAE images were read,** and no clinician validated the labels beyond HyperKvasir's own annotations.
- **BBPS is binary here** (0–1 vs 2–3, as labelled in HyperKvasir). It is not the per-segment 0–9 score the DoH audit uses.
- **Permission:** granted for this hackathon submission only. A pilot or commercial use needs a new request to SimulaMet.

## Reproduce (local research/evaluation only)
```bash
image/download.sh /tmp/hk                          # ~770 MB, six classes + labels + official split
python image/train.py /tmp/hk /tmp/hk/model grouped # ~3 min on an M2 Pro (MPS); writes metrics + verdicts
python image/serve.py /tmp/hk/model /tmp/hk/img     # optional live sidecar on :8792
WASL_IMAGE=cached scripts/demo.sh                   # integrated run with photo references
```
