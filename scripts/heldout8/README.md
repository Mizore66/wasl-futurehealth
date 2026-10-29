# Dataset card: heldout8

**Author:** independent author (GPT-6.1 Sol, OpenAI)  
**Date:** 2026-10-29  
**Purpose:** synthetic held-out evaluation of colonoscopy report reading, including documentation uncertainty, tissue acquisition, pathology attribution and examination completion.

## Generation and use

Run:

    python3 gen_heldout8.py

The script writes `reports_heldout8.json` beside itself. It uses Python 3's standard library, a private PRNG seeded with **8081**, fixed generation schedules and no external inputs. It performs no file reads, network access, model calls or clock access. Output is UTF-8 with fixed LF line endings, sorted JSON object keys and a final newline. Repeated runs produce byte-identical output in the same Python environment.

The corpus contains **300 unique texts**, identified by `v001`–`v300`. Each `sha` is SHA-256 of the exact UTF-8 report text, including its final newline.

Units, encounters and patients are fictional. Locations describe Abu Dhabi hospital and day-surgery settings; the corpus is not derived from patient records. Six layouts, alternative clinical phrasings, UK/US spelling, Arabic discharge text and benign scanning artefacts are combined deterministically.

Assertions check tag counts, uniqueness, hashes, reality-derived labels and documentation-derived classification.

## Answer-key model

Two deliberately different targets are supplied:

- **`truth`** follows private procedural reality: failure to reach the caecum or inadequate preparation takes precedence over pending histology.
- **`key_fields`, `key_state`, `key_reason`** follow what the report documents, including final corrections. Missing documentation is not silently completed from private reality.

Private reality is maintained during generation but is not exported as an additional report field. `truth` and the keys are evaluation labels, not inputs for the report reader.

| Reality label | Count |
|---|---:|
| adequate | 120 |
| awaiting_histology | 90 |
| inadequate | 90 |

| Documentation state / reason | Count |
|---|---:|
| closed / criteria_met | 86 |
| needs_human / missing_documentation | 79 |
| not_closed / incomplete_exam | 25 |
| not_closed / inadequate_prep | 45 |
| not_closed / awaiting_histology | 65 |

Thus `not_closed` totals **135**.

## Counts per tag

Tags overlap except within their format, spelling, preparation and tissue categories.

| Tag | Count |
|---|---:|
| fmt:structured | 50 |
| fmt:letter | 50 |
| fmt:theatre_log | 50 |
| fmt:narrative | 50 |
| fmt:discharge | 50 |
| fmt:worksheet | 50 |
| spelling:uk | 150 |
| spelling:us | 150 |
| prep:ungraded | 50 |
| prep:bbps | 100 |
| prep:verbal | 150 |
| tissue:none | 83 |
| tissue:biopsy_only | 95 |
| tissue:polypectomy | 122 |
| doc:extent_unstated | 45 |
| lang:arabic | 38 |
| scan:ocr_noise | 34 |
| hidden:undocumented_failure | 20 |
| hard:amended_extent | 15 |
| hard:historical_results | 15 |
| hard:conditional_future_sampling | 15 |
| hard:preprinted_orders | 15 |
| hard:ileum_not_completion | 15 |
| hard:withdrawal_not_extent | 15 |
| hard:lavage_not_final_grade | 15 |
| hard:lesion_not_specimen | 15 |
| hard:biopsy_without_polypectomy | 15 |
| hard:scoped_tissue_negation | 15 |
| hard:jar_count_not_lesion_count | 15 |
| hard:pathology_workflow_status | 15 |
| hard:landmark_completion | 15 |
| hard:separate_upper_endoscopy | 15 |
| hard:referral_not_performance | 15 |
| hard:quality_word_scope | 15 |
| hard:external_specimen | 15 |
| hard:radiology_not_extent | 15 |
| hard:staged_treatment | 15 |
| hard:other_site_histology | 15 |
| hard:bbps_segment_minimum | 18 |

Every report has a primary `hard:*` family: **300/300, or 100%**. This is an intentionally enriched challenge set, not a prevalence sample.

There are 217 tissue-taking procedures, including 95 non-polyp-biopsy-only procedures. All 83 inspection-only procedures explicitly negate tissue acquisition. Fifteen reports contain signed corrections changing the documented insertion endpoint in either direction. Historical-procedure, external-specimen and other-episode pathology references exceed the requested minimum.

## Hard-case families

- **amended_extent:** a signed final correction reverses the original insertion entry.
- **historical_results:** prior incomplete examinations and old pathology do not describe the current procedure.
- **conditional_future_sampling:** contingent future biopsy plans are not completed interventions.
- **preprinted_orders:** consent options and booking checklists do not establish performance.
- **ileum_not_completion:** lack of terminal ileal intubation does not itself mean incomplete colonoscopy.
- **withdrawal_not_extent:** where withdrawal narration or photography starts is not maximum insertion.
- **lavage_not_final_grade:** washing and suction do not themselves determine final preparation quality.
- **lesion_not_specimen:** a seen but untouched lesion generates neither tissue nor histology.
- **biopsy_without_polypectomy:** non-polyp mucosal sampling still counts as tissue acquisition.
- **scoped_tissue_negation:** a site-specific negative must not negate sampling at another site.
- **jar_count_not_lesion_count:** specimen-container counts are not counts of visible or removed lesions.
- **pathology_workflow_status:** accession and courier status are not diagnoses; stale pending flags may be superseded.
- **landmark_completion:** documented appendiceal and ileocaecal landmarks can establish completion without a literal completion tick.
- **separate_upper_endoscopy:** same-day gastric or duodenal biopsies are outside this colonoscopy's tissue episode.
- **referral_not_performance:** a requested biopsy or possible polypectomy is not a performed intervention.
- **quality_word_scope:** “adequate” sedation and “poor” tolerance do not grade bowel cleansing.
- **external_specimen:** delivery of tissue from another attendance is not sampling during this procedure.
- **radiology_not_extent:** a caecal finding on CT does not establish colonoscope insertion extent.
- **staged_treatment:** a planned later resection is not treatment performed today.
- **other_site_histology:** an unrelated organ's pathology cannot close the current colonic tissue episode.
- **bbps_segment_minimum:** a Boston total of six remains inadequate when one segment scores one.

## Keying decisions to review

1. **Legacy `polyps` field:** it means tissue taken during this colonoscopy, not visible polyps. It is always identical to `tissue_taken`. Non-polyp biopsy therefore gives `"yes"`; an untouched lesion does not.

2. **Current-procedure attribution:** `histology: "yes"` requires an actual diagnostic result for this colonoscopy's tissue. Old pathology, unrelated specimens, consent, future plans and laboratory workflow alone do not qualify. Current reported benign or nonspecific diagnoses do qualify.

3. **Extent:** explicit failure is `"no"`; blank or undocumented extent is `"not_stated"`. Anatomical landmark documentation can establish `"yes"`. Final signed corrections override the original insertion entry.

4. **Preparation:** prescribed agents, patient compliance and lavage alone leave quality `"not_stated"`. Explicit excellent, good, fair or adequate grades are adequate. Poor or inadequate grades are inadequate. Boston adequacy requires every segment to score at least two; all scores are explicitly final, post-cleaning scores.

5. **Ordered closure rules:** explicit incomplete extent precedes inadequate prep; either precedes missing documentation; missing documentation precedes pending histology. Consequently a tissue-taking procedure with undocumented extent is `needs_human`, not merely `awaiting_histology`.

6. **Hidden failures:** 20 reports omit documentation of a real failure. Other omissions mostly conceal satisfactory reality. Reviewers must not recover omitted facts from generation schedules or from the answer labels.

7. **Noise:** scanning artefacts affect headers and margins, not the decisive clinical statements. Arabic text supplies recovery/discharge context rather than contradictory completion or pathology claims.

## Limitations and evaluation cautions

This is a template-driven synthetic challenge corpus, not a clinical prevalence estimate or a validation of clinical safety. Some contextual qualifications are more explicit than routine hospital documentation. Feature schedules and repeated phrase pools are deliberate and inspectable; they can introduce artificial correlations.

For held-out use, expose only report text and opaque identifiers to the evaluated reader. Keep the generator, answer fields, tags and this card out of its inference context. Do not train on these reports or treat performance here as a substitute for evaluation on independently governed real-world documentation.
