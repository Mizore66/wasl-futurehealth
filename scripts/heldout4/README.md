# Held-out corpus 4 - synthetic Abu Dhabi colonoscopy reports

Author: fourth independent author (Claude agent, Anthropic). Written without reading any other corpus or any system code.

| | |
|---|---|
| Files | `gen_heldout4.py` (generator), `reports_heldout4.json` (300 reports + answer key) |
| Size | 300 reports, all texts unique |
| Determinism | Python 3 stdlib only, `random.Random(4049)`, fixed corpus date 2026-10-29, no wall clock, no network, no LLM calls. `python3 gen_heldout4.py` rewrites `reports_heldout4.json` byte-identically (SHA-256 of the file is stable across runs). |
| Self-check | The generator refuses to write output unless every check below passes. `python3 gen_heldout4.py --stats` prints all counts. |
| Content | Entirely fictional: day-surgery and hospital endoscopy units, clinicians, patients, MRNs and lab numbers are invented. Patient names are generic Emirati / expatriate names, a quarter reduced to initials. |

## How it works

1. **Hidden reality.** Each report first gets a truth label (exactly 120 adequate / 90 awaiting_histology / 90 inadequate, shuffled). The inadequate ones are split into caecum-only failure (38), prep-only failure (37) and both (15, abandoned because of the preparation). From that the generator fixes four hidden booleans: caecum reached, prep adequate, tissue taken (any polypectomy or any biopsy at this colonoscopy), histology result back.
2. **Documentation plan.** Independent weighted choices decide *how* each item is written: stated, unstated, hedged, abbreviated, implied by an anatomical landmark, written in Arabic, written wrongly and corrected by an addendum, etc. Lesion and biopsy sites are restricted to segments the scope actually reached. Impression and recommendations are written from what the body says (before any addendum), as real reports are.
3. **Rendering.** One of nine layouts (below), British or US spelling (US is a conversion of the British text, US reports also use US date order), optional Arabic content, optional scan/OCR noise.
4. **Answer key.** `key_fields` are derived from the documentation plan - what a careful clinician can read from the final text - never from keyword matching. `key_state` / `key_reason` are then computed from `key_fields` by the fixed rules. `truth` is computed from the hidden reality.

### Record format

```json
{"uid": "q001", "sha": "<sha256 of text>", "text": "...", "truth": "adequate|awaiting_histology|inadequate",
 "key_fields": {"caecum": "yes|no|not_stated", "prep": "adequate|inadequate|not_stated",
                "polyps": "yes|no", "tissue_taken": "yes|no", "histology": "yes|no"},
 "key_state": "closed|not_closed|needs_human", "key_reason": "...", "tags": ["fmt:...", ...]}
```

Key rules, applied in order: caecum `no` -> not_closed/incomplete_exam; prep `inadequate` -> not_closed/inadequate_prep; caecum or prep `not_stated` -> needs_human/missing_documentation; tissue `yes` and histology `no` -> not_closed/awaiting_histology; otherwise closed/criteria_met.

## Distributions

| truth | n | | key_state / key_reason | n |
|---|---|---|---|---|
| adequate | 120 | | closed / criteria_met | 83 |
| awaiting_histology | 90 | | not_closed / awaiting_histology | 56 |
| inadequate | 90 | | not_closed / incomplete_exam | 46 |
| | | | not_closed / inadequate_prep | 37 |
| | | | needs_human / missing_documentation | 78 |

key_fields: caecum yes 206 / no 46 / not_stated 48; prep adequate 186 / inadequate 49 / not_stated 65; tissue yes 186 / no 114.
Truth x key_state: adequate->closed 83, adequate->needs_human 37, awaiting->not_closed 56, awaiting->needs_human 34, inadequate->not_closed 83, inadequate->needs_human 7. Every needs_human report hides either a fine procedure or (10 reports, tag `hidden:undocumented_failure`) a real failure. A small number of reports use fixed quotas (forced documentation modes) so that thin families - segmental prep, Boston segment failure, corrections towards failure, tissue corrections, post-resection extent - are not left to chance.

## Counts per tag

**Layouts (`fmt:`, 9 kinds):** free_text 50, headed (three heading styles) 45, key_value (EMR export, three separators) 29, table (markdown or fixed-width grid) 30, form (checkboxes) 35, dictation (voice transcription, half with spoken punctuation) 34, hl7 (ORU^R01 segments with HL7 escapes) 16, letter (to the referrer) 34, bilingual (English/Arabic headings) 27.

**Spelling:** uk 185, us 115.

**Prep (`prep:`):** verbal 114, bbps_segments 55, bbps_total 15, ungraded 61 (nothing written, regimen only, patient/nurse compliance only, or only non-prep scores), descriptive 15, segmental_verbal 10, arabic_verbal 6, corrected 24. `prep:boston` (any Boston score anywhere, nine notations) 90.

**Extent (`extent:`):** stated 135, unstated 33, ti 21, hedged 15, anastomosis 9, arabic 4, correction 21, abbrev 16, benign_neg 13, self_correction 10, template_contra 11, recovered 12.

**Tissue (`tissue:`):** polypectomy 96, biopsy (non-polyp: random/microscopic-colitis, IBD surveillance, proctitis, ulcer, stricture, tumour, specimen pots) 105, none 114, negated (explicit statement that no tissue was taken) 94.

**Histology (`hist:`):** inline 42, addendum 31, pending 50, pending_prior 16, request 27, none 104, na 30.

**Other:** lang:arabic 69, noise:ocr 43, hidden:undocumented_failure 10. Reports with at least one `hard:*` tag: 274 (91.3%).

### Required minimums (all met)

| requirement | minimum | count |
|---|---|---|
| distinct layouts | 6 | 9 |
| reports with Arabic text | 30 | 69 |
| scan/OCR noise | 25 | 43 |
| ungraded prep | 40 | 61 |
| unstated extent | 25 | 33 |
| Boston scores, varied formats | 30 | 90 |
| non-polyp biopsies | 25 | 105 |
| negated tissue statements | 20 | 94 |
| addendum/correction changing caecum, prep or tissue | 10 | 75 reports (plus 23 histology addenda) |
| references to prior procedures | 10 | 52 |

Corrections run in both directions: extent corrected to reached 12 / to not reached 9; prep corrected to adequate 17 / to inadequate 6; tissue corrected to taken 24 / to not taken 7 (a report can carry more than one).

## Hard-case families (`hard:*`, count)

Required families: `addendum_correction` 75, `addendum_histology` 23, `prior_procedure` 52 (prior incomplete exam, prior poor prep, prior histology, prior biopsy result, text carried forward from an old report), `negated_tissue` 94, `ocr_noise` 43.

Invented families:

| tag | n | what a careful reader does differently |
|---|---|---|
| bbps_total_only | 15 | Only a Boston total. Total 8-9 forces every segment >= 2 (adequate); total <= 5 forces a segment < 2 (inadequate); total 6-7 cannot be resolved (not_stated). |
| bbps_segment_fail | 11 | Total >= 6 but one segment 0-1, e.g. 3+3+1=7: inadequate. |
| bbps_nonstandard_order | 6 | Boston written left-first (`L3 T2 R2`). |
| template_default_contradicted | 11 | Auto-populated "Caecum: YES (template default)" contradicted by the narrative saying the scope stopped: caecum no. |
| bidirectional_session | 14 | OGD + colonoscopy in one document; gastric/duodenal biopsies and their histology do not count for the colonoscopy. |
| prep_agent_only | 9 | Prep line names the regimen and volume (4 L PEG, 2 sachets) but no quality: not_stated. |
| prep_compliance_not_quality | 15 | "Prep taken - YES", "clear watery output" are compliance, not endoscopic quality: not_stated. |
| score_lookalike | 12 | No prep grade, but comfort score 2/5, ASA 2, pain 1/10, withdrawal time: not Boston scores. |
| hedged_extent | 15 | "Probably reached the caecum", "?caecum", landmarks not confirmed: not_stated (caecal intubation needs landmarks). |
| ti_implies_caecum | 21 | Terminal ileum intubated, caecum never named: caecum yes. |
| post_resection_extent | 9 | After right hemicolectomy the ileocolic anastomosis is the end point: complete examination, keyed caecum yes. |
| recovered_extent | 12 | Initial failure at a flexure, then caecum reached after pressure/re-insertion: yes. |
| benign_negation | 13 | "No difficulty reaching the caecum", "not difficult": success despite negation words. |
| abbreviation_heavy | 16 | CI/AO/ICV/TI, Ix, CSP, EMR, Bx, NAD. |
| dictation_self_correction | 10 | "...reached the hepatic flexure, scratch that, correction, ...caecum": the later words win. |
| arabic_key_statement | 15 | The extent, prep or tissue statement exists only in Arabic. |
| descriptive_prep | 15 | Prep described (clear fluid, all mucosa seen / solid stool not cleared) without a grade word: keyed from the description. |
| segmental_prep | 10 | Good left, poor right: inadequate overall. |
| polyp_not_resected | 15 | Polyp/LST seen but left in situ or tattooed for later EMR: tissue no. |
| planned_not_done | 9 | Biopsies planned but deferred (antiplatelets, INR): tissue no. |
| specimen_label_only | 18 | Tissue evident only from "Pot A - rectum; Pot B - sigmoid": tissue yes. |
| request_not_result | 27 | Histopathology request form / "received in laboratory" is not a result: histology no. |
| blank_form_field | 13 | Empty checkboxes or empty export fields: not_stated, not "no". |

## Keying decisions

- **This procedure only.** History, carried-forward text, prior histology and OGD specimens never set caecum, prep, tissue or histology.
- **Final statement wins.** Addenda and dictation self-corrections are keyed per the corrected statement, whatever the body, impression or plan said.
- **Caecum yes** requires a positive statement, the terminal ileum, or (post right hemicolectomy) the ileocolic anastomosis. Hedged or unconfirmed landmarks are `not_stated`. Lesions in the caecum are only placed in reports that state caecal intubation, so locations never leak extent.
- **Prep.** Verbal excellent/good/fair/adequate (including Aronchick) = adequate; poor/inadequate = inadequate. Boston: adequate only if every segment >= 2; totals handled as in `bbps_total_only`. When the procedure was abandoned because of the preparation, unreached segments score 0 and the body always states the prep as inadequate. Regimen, compliance, patient report and non-prep scores are `not_stated`.
- **Tissue** = any polypectomy or any biopsy at this colonoscopy (`polyps` and `tissue_taken` always equal). Polyps seen but not removed, deferred biopsies, OGD biopsies and prior biopsies do not count.
- **Histology yes** only when a result for this colonoscopy's tissue appears in the text (inline or addendum). "Pending", "to follow", request forms and lab-receipt status are `no`; reports without tissue are always `no`.
- **OCR noise** substitutes characters inside words (rn/m, l/1, o/0, e/c, ...), adds stray punctuation, fax/scan banners and broken lines, but never deletes words and never touches digits, Arabic text or negation words, so the key remains readable.
- **Truth vs key.** Truth follows hidden reality; the key follows the text. They differ only where the text leaves caecum or prep unstated or ambiguous (`needs_human`).

## Checks performed by the generator

Before writing, the generator verifies for every report: sha256 matches text; text unique; key_state/key_reason follow the rules from key_fields; truth follows reality; stated key fields never contradict reality; polyps == tissue_taken; histology yes only with tissue; required `fmt:`, `spelling:`, `prep:`, `tissue:` tags present; all minimums above; at least 35% of reports carry a `hard:*` tag. Running it twice gives byte-identical output.
