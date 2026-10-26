# Models

| File | What |
|---|---|
| `encoder_weights.json` | Self-trained Arabic↔Latin character dual-tower name encoder (2.3 MB float weights, serialised as JSON). Loaded by `pipeline/model.go`. |
| `lgb_stream.json` | LightGBM pair scorer (link probability) |
| `iso.json` | Isotonic calibrator for the pair scorer |
| `gemma_cache.json` | Cached Gemma report verdicts. The merged run is deterministic without an LLM server. |
| `parse_cache.json` | Cached Gemma teach-back and e-consult parses |
