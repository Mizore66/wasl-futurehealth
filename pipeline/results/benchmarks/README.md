# Out-of-generator benchmarks: public data the team did not make

Status: **built**. The adapters are code in `pipeline/` and the numbers below come from running them; the data are
public and were not generated, chosen or edited by the team. These are the only Wasl results here that do not come from
Wasl's own synthetic generator.

`summary.json` holds the headline items in the `benchmarks[]` shape of `web/data/evidence.json` (fractions, 0 to 1).
Per-run detail is in `febrl/` and `names/`. Intervals are Wilson 95% for proportions and Hanley-McNeil 95% for ROC-AUC.

## 1. FEBRL4 record linkage, through Wasl's linker

FEBRL4: 5,000 original and 5,000 corrupted duplicate records of synthetic Australian people, one duplicate per original
(known 1:1 mapping), generated with Febrl and shipped in the `recordlinkage` Python package.

| Run | Fields used | Precision | Recall | F1 | False-match rate | Missed-match rate |
|---|---|---|---|---|---|---|
| **Wasl, `names_dob`** (auto-link, final clusters) | given name, surname, DOB | **0.979** [0.974, 0.983] | **0.866** [0.857, 0.876] | **0.919** | 0.013 [0.011, 0.017] | 0.134 [0.124, 0.143] |
| Wasl, `names_dob`, auto-link or human review (score ≥ 0.2, direct pairs) | same | 0.977 [0.972, 0.981] | 0.902 [0.893, 0.910] | 0.938 | 0.017 [0.014, 0.021] | 0.098 [0.090, 0.107] |
| Wasl, `names_dob_ssid` (adapted, see below) | + soc_sec_id in the ID slot | 1.000 [0.999, 1.000] | 0.912 [0.904, 0.920] | 0.954 | 0.000 [0.000, 0.001] | 0.088 [0.080, 0.096] |
| recordlinkage, published recipe | given name, surname, address, suburb, postcode | 0.943 [0.935, 0.949] | 0.808 [0.797, 0.819] | 0.870 | 0.045 [0.039, 0.051] | 0.192 [0.181, 0.203] |
| recordlinkage, same recipe on Wasl's fields | given name, surname, DOB | 0.980 [0.975, 0.984] | 0.752 [0.740, 0.764] | 0.851 | 0.016 [0.013, 0.019] | 0.248 [0.236, 0.260] |

Definitions. Precision = correct predicted A-B pairs / predicted pairs. Recall = correct predicted pairs / 5,000 true
pairs. False-match rate = share of the 5,000 B records predicted to match at least one wrong A record. Missed-match
rate = share of B records whose true A record is not predicted. "Final clusters" scores every cross-file pair that
ends in one union-find cluster, so transitive merges count; `febrl/wasl_*.json` also has `auto_link_direct` (scored
pairs at or above 0.7, without transitivity: precision 0.988, recall 0.867).

Reading it.
- With the same three fields, Wasl's unchanged linker recovers more true pairs than the same-fields recordlinkage
  rule (recall 0.866 vs 0.752, non-overlapping intervals) at the same precision (0.979 vs 0.980).
- It also beats the published baseline, which uses address fields Wasl does not have (F1 0.919 vs 0.870). That
  baseline is a simple sum-of-similarities rule, not the state of the art on FEBRL4; the claim is "competitive with a
  standard open-source baseline on data we did not make", not "best on FEBRL4".
- 179 true pairs (3.6%) never reach the scorer: they are not in the top-10 cosine neighbours and share no skeleton
  rule key (`blocking_recall` 0.964). With the human-review band, recall rises to 0.902.
- The 0.7 union threshold was not re-tuned for FEBRL. An oracle sweep over thresholds (`oracle_threshold_sweep`, for
  information only, it uses the answer key) peaks at F1 0.938 near 0.2, so on this data 0.7 trades recall for
  precision, as it is meant to.
- `names_dob_ssid`: every one of the 439 misses is a pair whose soc_sec_id FEBRL corrupted. A valid-looking but
  different ID is a hard veto in Wasl (cannot-link), so these people are never auto-linked, and every link made is
  correct. That is Wasl's designed behaviour for Emirates IDs, and it is also its main recall risk when the ID itself is
  mistyped into another valid number. The recordlinkage baselines do not use soc_sec_id.

### What ran unchanged, what was adapted

Unchanged (same code and files as the runtime and the frozen evaluation; model hashes in `febrl/wasl_*.json`, equal
to `pipeline/results/final/manifest.json`): the name encoder `models/encoder_weights.json`; registration identity
(`registrationIdentity`: Latin form, tokens, consonant skeleton, Luhn check); candidate generation
(`LinkerCore.candidates`: exact top-10 cosine, skeleton rule keys capped at 300, ID index); the 30 pair features; the
LightGBM scorer `models/lgb_stream.json`; the union threshold 0.7 and review floor 0.2; the merge rules (cannot-links,
cluster guard, ID cluster clash) in `LinkerCore.newRecord`/`Link`. The isotonic calibrators (`iso*.json`) are not in
the union decision (they calibrate closure confidence), so they do not affect these numbers.

Adapted (field mapping only, `pipeline/febrl.go`):
- name = given_name + " " + surname; date_of_birth `YYYYMMDD` → `YYYY-MM-DD` (otherwise missing: 94 A and 199 B records).
- sex missing on every record (FEBRL has none). With both sides empty the sex-agreement feature reads "agree".
- soc_sec_id: in `names_dob` it is unused, because Wasl's ID features and rules accept only a 15-digit, 784-prefixed,
  Luhn-valid Emirates ID and the feature is empty for anything else. In `names_dob_ssid` it is wrapped into a synthetic
  number of that shape (`7840000` + 7 digits + Luhn digit) so the runtime ID rules act on it.
- address, suburb, postcode, state: unused (Wasl has no address fields).
- arrival order: all of A, then all of B; exact top-10 search (the frozen evaluation used the HNSW index, which
  approximates it).

## 2. Arabic-Latin name pairs, name encoder alone

Data: `Sayedyousef/arabic-name-pairs` (Hugging Face, revision `12991aaf`), 22,797 English → Arabic single-token
names. Normalised with the runtime normalisers: 22,672 unique pairs. Overlap with the encoder's training sources
(NETransliteration `wd_arabic`, the mined Wikidata labels/aliases, and `data/persons3.json` canonical names and aliases,
all splits) removed by exact match after normalisation: 183 pairs dropped, 22,489 kept.

| Tier | Pairs | ROC-AUC, hard negatives | ROC-AUC, random negatives | Top-1 Latin→Arabic | Top-10 Latin→Arabic | Top-1 Arabic→Latin |
|---|---|---|---|---|---|---|
| Pair overlap removed | 22,489 | 0.9947 [0.9940, 0.9954] | 0.9972 | 0.869 [0.864, 0.873] (gallery 12,493) | 0.985 [0.983, 0.986] | 0.885 [0.879, 0.890] (gallery 22,486) |
| Names never seen as a word in training | 1,440 | 0.9978 [0.9961, 0.9995] | 0.9997 | 0.948 [0.935, 0.958] (gallery 519) | 0.998 | 0.973 [0.955, 0.984] (gallery 1,440) |

- Hard negative: for each pair, another Arabic name from the set with the same first letter and length within one
  letter (seeded random choice). In 99.7% [99.66, 99.79] of pairs the true name scores above its hard negative.
- Threshold: the runtime has no standalone encoder threshold (the cosine feeds top-10 blocking and is one of the scorer's
  30 features). A cosine threshold was therefore chosen for best F1 on a random 20% of pairs (0.735) and reported on the
  other 80%, one hard negative per positive (50% prevalence): precision 0.981 [0.978, 0.982], recall 0.986 [0.984,
  0.988], hard-negative false-match rate 0.020 [0.018, 0.022].
- Top-10 is the linker's candidate depth: 98.5% of Latin queries have their Arabic name among the 10 nearest of 12,493.
- The "never seen" tier has a much smaller gallery, so its retrieval numbers are easier and not comparable with the
  full tier; its AUC is the cleaner check that the encoder generalises to unseen names.

## Reproduce

```bash
scripts/bench_febrl.sh   # throwaway venv in $BENCH_TMP (default /tmp/wasl-bench) with recordlinkage==0.16; copies and
                         # sha256-checks FEBRL4; builds bin/wsi7; runs both Wasl configs and both baselines; rebuilds summary.json
scripts/bench_names.sh   # downloads the pinned name-pair file into $BENCH_TMP; needs the encoder's training sources in
                         # $ENC_RAW (default ~/wasl-ml-data/encoder/raw: wd_arabic is downloaded if absent;
                         # wikidata_variants.jsonl comes from encoder/mine_wikidata.py); runs bin/wsi7 namepairs
```

Single commands: `bin/wsi7 febrl -a dataset4a.csv -b dataset4b.csv -wasl models -config names_dob|names_dob_ssid -out X.json`;
`bin/wsi7 namepairs -pairs dict_FINAL.json -encfile models/encoder_weights.json -netrans wd_arabic -variants
wikidata_variants.jsonl -persons data/persons3.json -seed 1 -out X.json`; `python3 -I scripts/bench_summary.py
pipeline/results/benchmarks`. Both Wasl runs are deterministic (re-runs gave identical files). Tests:
`go test -run 'Febrl|NamePairs' ./pipeline/...` (inline fixtures, no downloads).

## Sources and licences

| Data | Source | Licence | sha256 |
|---|---|---|---|
| FEBRL4 `dataset4a.csv`, `dataset4b.csv` | Febrl (P. Christen, ANU), as shipped in `recordlinkage` 0.16 (https://github.com/J535D165/recordlinkage, `recordlinkage.datasets.load_febrl4`) | recordlinkage: BSD-3-Clause; Febrl 0.4.x: ANU Open Source Licence 1.3 or GPL-2.0-or-later | `07c7cb3f…` / `2eed76c9…` |
| arabic-name-pairs `dict_FINAL.json` | https://huggingface.co/datasets/Sayedyousef/arabic-name-pairs | CC-BY-4.0 (dataset card) | `b35b2de4…` |
| Baseline numbers | preclink 1.2.0 README benchmark table, https://pypi.org/project/preclink/1.2.0/; recipe in https://github.com/finite-sample/preclink/blob/main/examples/benchmark_febrl.py (`benchmark_recordlinkage`) | MIT | reproduced exactly here: 0.9428 / 0.808 / 0.870 |

No raw data is committed; only the scripts, the adapter code, its tests and these results. Full hashes are in the
result JSONs and the two scripts.

## Limits

- **No public benchmark combines Arabic-script names, an Emirates-ID-style national number and DOB.** FEBRL has no
  Arabic names, no cross-script pairs, no sex and no Emirates ID; the name-pair set has names only. Neither tests the
  Emirates-ID path as it runs in the UAE or the full linker on Arabic names. The frozen synthetic evaluation
  (`pipeline/results/final/`) remains the only end-to-end measure of that.
- FEBRL names are Anglo-Australian Latin strings, so the cross-script encoder is exercised only within Latin script.
- FEBRL's corruption model is Febrl's, not Wasl's (that is the point), but it is still synthetic data.
- The name-pair set is single tokens (Wasl links full names), was merged by its author from JRC-Names, machine
  translation and an LLM, and is mostly written in Modern Standard Arabic spelling. JRC-Names' own licence forbids
  derivative works; the dataset card states CC-BY-4.0. It is used here for evaluation only and not redistributed.
- Exact-pair dedup cannot remove token-level exposure: common names such as "mohammed" occur inside many training
  full names. The "never seen" tier removes that, at the cost of a small gallery.
- The hard negatives (same first letter, similar length) are moderate, not adversarial; the retrieval numbers, where
  every other name in the gallery competes, are the stricter test.
