# reports_heldout7 — synthetic colonoscopy report-reading corpus

## Dataset card

**Author:** independent author (Kimi K3.1 Pro, Moonshot AI)  
**Generator:** `gen_heldout7.py`  
**Output:** `reports_heldout7.json`  
**Size:** 300 unique synthetic reports  
**Provenance:** entirely synthetic; no patient records, network access, model calls, clock reads, or input-file reads are used.

Run:

`python3 gen_heldout7.py`

The script uses only the Python 3 standard library and `random.Random(7079)`. It writes UTF-8 JSON beside the script. Generation order, whitespace, tag ordering, report identifiers, hashes, and JSON formatting are deterministic; repeated runs produce byte-identical files. Each `sha` is SHA-256 of the exact UTF-8 `text`.

The reports imitate documentation from Abu Dhabi day-surgery and hospital endoscopy units. Facilities, clinicians, identifiers, and clinical vignettes are synthetic. There are eight layouts, UK and US spelling, bilingual Arabic/English reports, scan/OCR artefacts, dictated notes, discharge summaries, compact records, and correction addenda.

## Hidden truth versus text key

The hidden reality contains caecal completion, preparation adequacy, current-procedure tissue acquisition, and availability of the current specimen's histology. `truth` is calculated solely from that reality:

- **120 adequate** (40%)
- **90 awaiting_histology** (30%)
- **90 inadequate** (30%)

`key_fields`, by contrast, describe what a careful clinician can establish from the final text. Thus an undocumented failure can have `truth: inadequate` while its text key is `needs_human`. The 12 `hidden:undocumented_failure` cases are deliberate; most unstated extents or preparation grades conceal satisfactory facts.

Key-state precedence is exactly:

1. Caecum `no` → `not_closed / incomplete_exam`.
2. Preparation `inadequate` → `not_closed / inadequate_prep`.
3. Caecum or preparation `not_stated` → `needs_human / missing_documentation`.
4. Current tissue `yes` and current histology result `no` → `not_closed / awaiting_histology`.
5. Otherwise → `closed / criteria_met`.

The generator asserts these relationships, the truth distribution, uniqueness, hashes, required tag families, and all stated minimum quotas.

## Exact tag counts

Tags can overlap, so counts do not sum to 300 except within their respective exclusive format, spelling, preparation, and primary tissue groups.

| Tag | Count |
|---|---:|
| fmt:structured | 38 |
| fmt:narrative | 38 |
| fmt:impression_first | 38 |
| fmt:tabulated | 38 |
| fmt:discharge | 37 |
| fmt:dictated | 37 |
| fmt:compact | 37 |
| fmt:clinical_letter | 37 |
| fmt:arabic_bilingual | 33 |
| fmt:ocr_scan | 25 |
| fmt:addendum | 16 |
| spelling:uk | 150 |
| spelling:us | 150 |
| prep:ungraded | 50 |
| prep:boston | 48 |
| prep:verbal | 202 |
| tissue:none | 83 |
| tissue:polypectomy | 125 |
| tissue:biopsy | 70 |
| tissue:polyp_not_removed | 12 |
| tissue:planned_not_done | 10 |
| hard:addendum_correction | 16 |
| hard:missing_documentation | 87 |
| hard:bbps_total_trap | 12 |
| hard:prep_rescue | 12 |
| hard:prep_segment_overrides_overall | 10 |
| hard:extent_by_landmarks | 18 |
| hard:ti_implies_caecum | 12 |
| hard:withdrawal_phrase | 12 |
| hard:impression_only_failure | 12 |
| hard:non_polyp_biopsy | 36 |
| hard:no_polyps_but_biopsied | 12 |
| hard:specimen_no_result | 8 |
| hard:histology_request_not_result | 14 |
| hard:negated_tissue | 28 |
| hard:polyp_seen_not_removed | 12 |
| hard:planned_not_done | 10 |
| hard:prior_procedure | 24 |
| hard:prior_histology_not_current | 8 |
| hard:prior_incomplete_not_current | 8 |
| hard:ocr_noise | 25 |
| hidden:undocumented_failure | 12 |

Additional corpus counts useful to reviewers:

- Unstated extent: **37**.
- Unstated/ungraded preparation: **50**.
- Boston scores: **48**, in six layouts/formats.
- Non-polyp biopsy procedures: **70**; 36 carry the dedicated hard-family tag.
- Explicit negated-tissue hard family: **28**; other no-specimen wordings also occur.
- Final addenda/corrections: **16**, including corrections in both directions.
- Prior-procedure references: **24**.
- Arabic text: **33** reports.
- OCR/scan noise: **25** reports.
- More than 35% of reports carry at least one `hard:*` tag; overlaps are intentional.

## Invented hard-case families

Beyond the requested addendum, prior-procedure, biopsy, negation, OCR, and missing-documentation cases, the generator includes these clinician-versus-keyword traps:

- **BBPS total trap:** totals of 6 or 7 still count as inadequate when a segment scores 0 or 1.
- **Rescue preparation:** an initially poor view becomes adequate after washing; the final assessment controls.
- **Segment overrides overall wording:** “fair” overall does not rescue an explicitly poor right colon.
- **Landmark-only extent:** appendiceal orifice and ileocaecal-valve documentation establishes caecal extent.
- **Terminal ileum implies caecum:** terminal ileal intubation establishes completion without relying on a stock completion sentence.
- **Withdrawal phrasing:** withdrawal from the caecal pole documents that the caecum was reached.
- **Impression-only failure:** the body defers to the impression, where incomplete extent is recorded.
- **Polyp seen but not removed:** a visual polyp without excision or biopsy means no current tissue.
- **Planned sampling not done:** considered, discussed, or abandoned biopsy is not tissue acquisition.
- **No polyps but biopsied:** “no polyps” does not negate random or targeted mucosal biopsies.
- **Pathology request is not a result:** a request number or pending laboratory field leaves histology unresulted.
- **Specimen lost after retrieval:** tissue was taken even though no histology result can be quoted.
- **Prior histology is historical:** old adenoma or polyp results do not result the current specimen.
- **Prior incomplete examination is historical:** an earlier failed attempt does not determine current extent.
- **OCR with protected clinical content:** noise affects headers, identifiers, spacing, and scan artefacts rather than silently reversing clinical facts.

## Keying decisions for reviewers

- `polyps` is intentionally procedure-action based, as required: it equals `tissue_taken`. A non-polyp biopsy therefore makes both fields `yes`; a polyp merely seen and left in situ makes both `no`.
- Histology `yes` requires a result for tissue from **this** procedure. Pending text, a request, a lost specimen, and prior histology are all `no`.
- A current pathology addendum can change histology from `no` to `yes`; the latest explicit correction wins over earlier text.
- Boston adequacy requires every segment to score at least 2. A total of 6 or more is not sufficient by itself.
- Verbal excellent, good, fair, and adequate are adequate. Poor and inadequate are inadequate. An explicit poorly prepared segment overrides a reassuring overall adjective.
- Caecal landmarks, terminal ileal intubation, and withdrawal from the caecal pole support caecum `yes`.
- A blank or explicitly unrecorded preparation/extent field is `not_stated`, not an invitation to infer the hidden reality.
- The Arabic summary restates the current key fields. In corrected reports it follows the correction and therefore reflects the final statement.
- OCR noise is intentionally non-reversing; reports remain clinically readable.
- `truth` should not be used to repair `key_fields`, and report order, identifiers, and tags are not intended as prediction features.
