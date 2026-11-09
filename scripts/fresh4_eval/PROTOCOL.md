# Fresh4 pre-registered independent evaluation protocol

**Frozen before corpus generation: 2026-11-09. Base commit before all work: `91cff71626e38a509395bef3c031d1d1f82a2773`.**

Fresh4 has two distinct evaluations: (1) the unchanged LDCT/IFHAS readers on a new independently authored synthetic corpus, with blind double labelling and disagreements-only adjudication; and (2) the unchanged default integrated pipeline on five previously unused seeds. All data and reviewers are simulated. Model labels and adjudication are not clinician verification. No result is clinical-effectiveness evidence.

## 1. Seed and change history searched first

Searched `git log --all` for fresh corpora, frozen evaluations, seed registrations and r1agent10; read `scripts/pairfeat_compare.py`, `scripts/pairfeat_experiment.sh`, `pipeline/RESULTS.md`, `scripts/final_eval.sh`, and fresh2/fresh3 freeze artifacts.

- Final integrated seeds 11, 23, 37, 41 and 59 are already used and are not reused.
- r1agent10 development seeds are 987, 991 and 993. Its validation seeds **997, 1027, 1037, 1043 and 1049 remain untouched**. Its recorded outcome remains **development passed; stress lost five closures against the unguarded comparator; validation did not run**. Fresh4 does not run, resurrect or reinterpret r1agent10 validation.
- The failed r1agent13 experiment's registered validation block **1103, 1109, 1117, 1121 and 1123** was documented unused at the base commit. Fresh4 freezes that block for a new evaluation of the **default pipeline only**. This is not r1agent13 validation, agent replay, a pairfeat9 run, or a candidate comparison. No r1agent flag, alternative modelset, demographic guard, r1review or image option is enabled.
- Fresh2 and fresh3 reader code/corpus/label/cache/scoring history was reviewed. Fresh4 uses the current shipped readers at the new reader-code freeze commit; it does not alter fresh2/fresh3 artifacts or claim their errors were fixed.

No rules, reader prompts, models, thresholds, candidate gates or integrated defaults may be tuned after any corpus, label, cache or result is seen. Harness defects discovered before scoring may only be fixed by a newly committed pre-score amendment with a reason; outcome-dependent amendments are prohibited.

## 2. Corpus and adversarial coverage

Exactly 100 new synthetic English clinical documents, all fictitious and without real identifiers: 50 LDCT and 50 IFHAS. Canonical order and kind blocks are mandatory:

| IDs | kind | required adversarial coverage within the block |
|---|---|---|
| f4l-001..025 | ldct-screen | 001..005 current overall category versus prior/hypothetical; 006..010 multiple/per-nodule categories; 011..015 assessment words; 016..020 incomplete, contradictory or insufficient text; 021..025 addenda, S modifiers and referral-override wording |
| f4l-026..050 | ldct-workup | 026..030 attendance/DNA; 031..035 declined, postponed, pending tissue or pending decision; 036..040 malignant treatment with missing/present MDT or tissue requirements; 041..045 benign/annual/resolved/bare-annual cases; 046..050 surveillance, wrong-document and contradictory cases |
| f4i-001..025 | ifhas-followup | 001..005 DNA; 006..010 managed single/multi-domain; 011..015 pending/awaiting/declined; 016..020 not-confirmed, including prediabetes lifestyle advice; 021..025 historical-only, no-domain, wrong-document or contradictory notes |
| f4i-026..050 | ifhas-screen | 026..030 thresholds/severity; 031..035 missing inputs; 036..040 unit/code mismatch and implausible values; 041..045 duplicates, fasting-versus-random wording and inert no-coded placeholder; 046..050 previous results, ignored codes and panel/coded mismatch |

Each five-item sub-block must contain meaningfully varied documents, not near-duplicates. The corpus carries no labels, answer key, scenario tags or item metadata. The coverage mapping is not shown to blind labellers. Normalized text must not duplicate any prior `data/ldct`, `data/ifhas`, or `data/fresh*` corpus text.

A fresh Agent context authors the complete content and returns JSON without writing files. It may read only the frozen labelling guide for clinical/rule semantics; it must not read reader implementation, old corpora, labels, scores, caches or this coverage-to-answer mapping beyond the author prompt. The coordinator validates, canonicalizes only if formatting is required without changing content, writes and commits the returned corpus, author provenance, exact prompt and key-free labeller input before any labelling or scoring.

## 3. Blind labelling and agreement

`labeller-input.json` contains only schema and item fields (`id`, `kind`, `programme`, `text`, and `coded` only for IFHAS screening). It has no provenance, author metadata, key, tags or scores. Its manifest binds exact hashes.

Two **separate fresh Agent contexts**, A and B, each receive only:

1. `scripts/fresh2_eval/LABELLING-GUIDE.md`;
2. `data/fresh4/labeller-input.json`;
3. their own metadata and return-schema instructions in the prompt.

Neither may read the canonical corpus, author prompt/provenance, coverage map, reader code, cache, prior labels, the other labeller, adjudication or scores. Each returns complete JSON for all 100 items. The coordinator, not the Agent, writes separate `labels/labeller_a.json` and `labels/labeller_b.json`. Invalid/incomplete output is rejected; any retry is a new fresh context and fully returned, never a coordinator patch.

Report direct agreement per kind and field: observed agreement and Cohen kappa (undefined with a note when degenerate), plus exact item agreement. Same-family labellers produce an independence warning. Directly agreed items form the initial key. Every disagreement is initially unadjudicated.

## 4. Disagreements-only adjudication before scores

A third fresh Agent context receives the guide and `adjudication-input.json`, which contains only disagreements, their texts/coded values and the two anonymous label sets. It never receives agreed items, reader outputs, caches, scores, gates or integrated results. It returns one decision per disagreement:

- `adjudicated` with a guide-valid key, a text-specific rationale, and `low|moderate|high` uncertainty; or
- `unresolved` with a null key, rationale and uncertainty when the guide/text does not support a defensible resolution.

The coordinator validates and writes the returned file, derives `adjudicated-key.json`, and records uncertainty. No adjudication is silently forced. Direct-agreement and adjudicated strata are scored separately and pooled; unresolved items are excluded from accuracy denominators and reported for reader routing behaviour only. Labels, raw agreement, adjudication input/output, adjudicated key and post-adjudication counts are committed before cache construction or scoring.

## 5. Frozen reader execution and denominators

After the labels commit, key-free reader inputs are exported. Reader B is the unchanged local **Gemma 3 4B IT Q4_K_M** with shipped LDCT/IFHAS prompts, schema, temperature 0 and seed 1, called once per unique text by `wsi7 ldctcache`/`ifhascache`. Cache-build output is retained as provenance; any key-free diagnostic counts it prints are not evaluation scores. The two caches and a freeze manifest are committed before the official reader run.

The official reader run is exactly one `TestFresh2Reader` replay through `fresh4.py score`, after freeze checks. It reads no labels/key and makes no model call. Raw A/B output for every item is published. The results directory is created exclusively once.

Denominators:

- total corpus 100; raw completeness denominator 100;
- direct-agreement, adjudicated and pooled scoreable counts reported separately;
- per-kind denominator is that stratum's scoreable items;
- unresolved items have no accuracy denominator;
- A correct, B correct, B unavailable, readers agree, agreed-and-right, agreed-but-wrong, and review routing are counts, not hidden percentages;
- Wilson 95% intervals accompany pooled per-kind A-correct, B-correct and agreement proportions;
- closing-direction errors use pooled scoreable items as denominator and retain per-item IDs.

Closing direction follows the frozen fresh2 analysis definition: LDCT work-up `wk_treatment`/`wk_benign` contrary to key; IFHAS follow-up managed/not-confirmed where the key is pending, DNA, indeterminate or lacks that domain. Surveillance and all other wrong states are holding-direction.

### Reader gates

- **R1 complete raw output:** exactly 100 valid unique corpus rows; otherwise fail. If the reader cannot run, R1/R2 are `not_run`, never pass.
- **R2 zero closing-direction agreed-but-wrong answers:** numerator must be 0 over the pooled scoreable denominator. Any nonzero count fails and is published. No accuracy, agreement or availability threshold is a hidden selection gate.

## 6. Frozen integrated evaluation

Run the unchanged `scripts/final_eval.sh` defaults for policy, thresholds, models, cached readers, cohort sizes, chaos and reviewer stress, with seven programmes enabled (`WASL_LDCT=300 WASL_IFHAS=800`) and both v1/v2 noise models. No r1agent10/r1agent13 candidate code path is selected.

Exact command from repository root after the reader-score commit:

```bash
WASL_FINAL_DIR="$PWD/data/fresh4/results/integrated" \
WASL_FINAL_WORK="$PWD/out/fresh4-integrated" \
WASL_LDCT=300 WASL_IFHAS=800 WASL_JOBS=3 WASL_PORT_BASE=7000 \
scripts/final_eval.sh 1103 1109 1117 1121 1123
```

The expected default cohort settings are CRC 2040, cervical 816, incidental lung 1088, DR 1200, breast 70, LDCT 300, IFHAS 800, all 8,871 held-out persons; FHIR source; v7strict; cached second readers; chaos on; reviewer error 0.04; double review merge+close; EID guard off; lung PET surveillance and DR next-due on; wrong-EID/wrong-ref and extended surveillance off. The generated manifest, not this prose, is authoritative for actual settings and hashes.

Raw per-lane `report7.json`, `tamper.json`, `summary.txt`, `origin_null.json`, `subgroups.json`, aggregate `summary.json`/`SUMMARY.md`, manifest and gate JSON are published. A failed lane or aggregation is retained/reported, not hidden or rerun to improve results. Infrastructure retry before any metric is read requires a disclosed amendment and preserves failed logs.

### Integrated gates (per noise model)

- **I0:** all 10 lanes (5 seeds × 2 noise models) complete with required artifacts and aggregation. Partial evidence cannot pass.
- **I1:** zero listed false closures for CRC, cervical, incidental lung, DR and DR referral, breast and breast recall, LDCT and LDCT action, IFHAS and IFHAS follow-up. Numerator/denominator are pooled false closures/predicted closures for each metric.
- **I2:** zero wrong-patient releases over outreach intents and zero releases without granted consent over releases.
- **I3:** zero duplicate/lost outputs over expected outputs; zero ingest losses/duplicates over generated resources; zero final-state mismatch loops.
- **I4:** zero forged events with effect over forged events; 85/85 tamper attacks detected per noise model; zero baseline tamper false alarms per noise model.

Recall, precision, reviewer burden, subgroup burden, throughput, KPI and reader agreement are descriptive with the aggregate's t intervals where applicable. Zero-event safety bounds use the aggregate's Clopper–Pearson calculation. There is no post-hoc recall or workload selection gate.

## 7. Commit order, honesty and analysis

Strict commit order: **protocol/harness → corpus/blind input → labels/raw agreement/adjudication → reader caches/freeze manifest → reader raw output/scores/gates → integrated raw output/scores/gates**. Every stage carries the required Claude Code attribution. No push.

The final report lists every gate pass/fail/not-run, all raw artifact paths, exact commands and unresolved findings. Failures remain failures. No model, prompt, rule, threshold, corpus item, label or adjudication is changed in response to reader or integrated outcomes. The r1agent10 status remains exactly **validation did not run**.
