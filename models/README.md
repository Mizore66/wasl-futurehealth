# Models

| File | What |
|---|---|
| `encoder_weights.json` | Self-trained Arabic↔Latin character dual-tower name encoder (2.3 MB float weights, serialised as JSON), retrained on real Wikidata + NETransliteration name-variant pairs (`encoder/`, results in `encoder/RESULTS.md`). Loaded by `pipeline/model.go`. |
| `lgb_stream.json` | LightGBM pair scorer (link probability), refit on the new encoder's cosines (`encoder/fit_scorer.py`) |
| `iso.json` | Isotonic calibrator for the pair scorer (cal split) |
| `iso_v7.json` | v7 stratified conservative calibrator (corroborated / uncorroborated, Clopper-Pearson plateau lower bounds, cap 0.989) |
| `encoder_v3.json`, `lgb_stream_v3.json`, `iso_v3.json`, `iso_v7_v3.json` | The previous chain (encoder trained on canonical pairs + synthetic noise). Select it with `-modelset v3` / `WASL_MODELSET=v3` to reproduce earlier results (`pipeline/RESULTS.md`, stage-7 final evaluation). |
| `gemma_cache.json` | Cached Gemma report verdicts. The merged run is deterministic without an LLM server. |
| `parse_cache.json` | Cached Gemma teach-back and e-consult parses |
| `image_verdicts.json` | Image-reader scores (caecal landmark, BBPS prep) for 2,374 HyperKvasir held-out test images, keyed by sha256; no pixels. Weights are not distributed (see `image/DATA.md`). Used by `-image cached` and by `gen -imgphotos`. |
