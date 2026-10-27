# Name encoder: retraining on real name-variant pairs

The Arabic↔Latin person-name encoder (`models/encoder_weights.json`, loaded by `pipeline/model.go`) was
originally trained on canonical persons3 pairs plus the same synthetic noise model that the evaluation cohorts use.
This folder retrains it on **real** name variants and measures it on a **held-out real-variant test** as well as on
the synthetic protocol. Results: [`RESULTS.md`](RESULTS.md).

## Files

| File | What |
|---|---|
| `wenc.py` | Normalisers (ports of `pipeline/text.go`), the PyTorch dual-tower model (mirrors `pipeline/model.go` Embed), JSON import/export, synthetic noise (port of `gen.go` v1) |
| `parity.py` | Go ↔ PyTorch check for a weights file: cosine > 0.9999 on 55 names, normalisers identical on 5,055 names |
| `mine_wikidata.py` | Mines real labels/aliases for the persons3 QIDs from Wikidata |
| `data.py` | Per-person variant sets, NETransliteration loading with leakage removal, the real-variant and synthetic test sets |
| `eval.py` | R@1/R@10 on the real-variant test and the synthetic protocol (plus same-script retrieval) |
| `train.py` | InfoNCE training with own-index hard negatives; checkpoint selection on the dev split |
| `fit_scorer.py` | Refits the LightGBM pair scorer, `iso` and the stratified `iso_v7` calibrators on cosines from a given encoder |
| `run_all.sh` | The whole pipeline, end to end |

Go side (`pipeline/encdev.go`): `wsi encembed` (Go embeddings for parity), `wsi encnoise` (synthetic protocol pairs,
`gen.go` noise), `wsi featdump` (pair-scorer rows produced exactly like the runtime linker's candidate generation).
`-modelset X` (or `WASL_MODELSET=X scripts/demo.sh ...`) runs the alternative chain
`models/{encoder_X,lgb_stream_X,iso_X,iso_v7_X}.json`; without it the default files are used.

## Data provenance and licences

| Source | Use | Licence |
|---|---|---|
| `data/persons3.json` (Wikidata persons, existing) | person splits; canonical `ar`/`en` and `als` | CC0 (Wikidata) |
| **Wikidata** labels and aliases, fetched for the persons3 QIDs via `wbgetentities` (Latin script: en, fr, de, es, it, nl, tr, id, ms; Arabic script: ar, arz) | real variants; train split for training, dev for selection, eval for the test | **CC0 1.0** |
| **NETransliteration** (Merhav & Ash, COLING 2018), `data/wd_arabic`, 145,186 English↔Arabic Wikidata name pairs, from `github.com/steveash/NETransliteration-COLING2018` | extra training pairs | data **CC0 1.0** (Wikidata-derived); code MIT |

Not used: JRC-Names (its licence forbids derivative works) and ANETAC (no licence).

Neither raw dataset is committed. They are downloaded into a scratch directory by `run_all.sh`.

**Split hygiene.**
- Persons keep their persons3 split everywhere. Only train-split persons feed training; dev picks the checkpoint; cal fits the calibrators; eval is the test.
- NETransliteration is Wikidata-derived, so it can contain held-out people. Any pair whose English or Arabic side, once normalised, equals **any** name string of a dev, cal or eval person (canonical name, persons3 alias or mined variant) is dropped. That removed 5,944 pairs.
- Leading honorifics (Dr, Sheikh, الدكتور, الشيخ, …) are stripped from mined variants. Tokens that are also given names (Amir, Sultan, Sayed, Haji) are kept.

## Test sets

- **Real-variant test** (`data.real_test`, eval split). Queries are real variants that differ from the person's canonical name after normalisation, at most 3 per person and direction:
  - Latin queries are Wikidata Latin labels/aliases and persons3 aliases; the gallery is the canonical Arabic names of all eval persons.
  - Arabic queries are Wikidata ar/arz labels/aliases; the gallery is the canonical Latin names of all eval persons.
  - Hits are counted on normalised strings, so homonyms in the gallery count as one entry.
- **Synthetic protocol** (`wsi encnoise`, eval split, seed 7). Each person contributes one noisy Arabic and one noisy Latin registration, using the `gen.go` noise: v1 is the training-time model, v2 the independent held-out model. The report averages the two retrieval directions.
- **Same-script.** Two independent v1-noised Latin (or Arabic) registrations per person, retrieved against each other. The linker also scores same-script pairs.
