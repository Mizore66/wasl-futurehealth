# Blind labelling guide: lung screening (LDCT) and IFHAS cardiometabolic follow-up

You label short synthetic clinical texts. This guide is everything you need. **Do not read any software that processes these texts, any other labeller's file, or any earlier answer key.** Label only from the text in front of you and the rules below. All texts are synthetic; none describes a real person.

If a rule here is silent or the text is unclear, use `indeterminate` (where allowed). Never guess to fill a gap. A wrong confident label is worse than `indeterminate`.

Your labels are **model-labelled or human-labelled and not clinician-verified**. They will be compared with a second independent labeller; only items where you both agree get a key.

## 1. What you receive and return

You receive a corpus file (`fresh2-corpus/v1`): items with `id`, `kind`, `programme`, `text`, and for `ifhas-screen` items also a `coded` string. You return one JSON file (`fresh2-labels/v1`):

```json
{
  "schema": "fresh2-labels/v1",
  "corpus_sha256": "<copied from your template, do not change>",
  "labeller": {
    "id": "<unique short id, differs from the other labeller>",
    "kind": "model",
    "model": "<exact model name>", "model_family": "<vendor/family>", "prompt_sha256": "<sha256 of the exact prompt text you were given>",
    "run_date": "YYYY-MM-DD",
    "guide_sha256": "<copied from your template>",
    "session_isolated": true,
    "saw_reader_implementation": false,
    "saw_other_labels": false,
    "clinician_verified": false
  },
  "labels": [
    {"id": "<item id>", "label": "<exact label from section 2>"},
    {"id": "<ifhas-screen item id>", "label": "<panel label>", "coded_label": "<coded label or unavailable>"}
  ]
}
```

For a human labeller use `"kind": "human"` and `"name"` instead of the three model fields.

Declarations are binding. Set `session_isolated` to true only if you labelled in a fresh session with no memory of other labelling work on this corpus. Set `saw_reader_implementation` and `saw_other_labels` to false only if that is true; if either is not true the file is rejected, which is the correct outcome. `clinician_verified` is always `false`.

Exactness matters. Labels are compared as exact strings: case, spacing and order must match section 2. `lr1`, `LR 1`, `Lung-RADS 1` and `wk_DNA` are all rejected. Return every item exactly once, with no extra fields.

## 2. Kinds, allowed labels and rules

Four kinds appear. Use only the labels listed for the item's `kind`.

### 2.1 `ldct-screen`: one low-dose CT lung-screening report

Allowed labels (exactly these eight): `LR0`, `LR1`, `LR2`, `LR3`, `LR4A`, `LR4B`, `LR4X`, `indeterminate`.

Label the exam's **overall Lung-RADS v2022 category for the exam described in this text**:

- If the report states a final or overall category for this exam, that is the label. The `S` modifier ("2S") is ignored.
- If it gives no final category, the label is the category of the most concerning finding, in the order `1 < 2 < 3 < 4A < 4B < 4X`.
- If it gives only assessment words and no number: negative = 1; benign appearance or behaviour = 2; probably benign = 3; suspicious = 4A; very suspicious = 4B; very suspicious with additional features such as lymphadenopathy or pleural findings = 4X.
- `LR0` means the exam is incomplete or cannot be assessed without comparison or further imaging (for example motion artefact, obscuring atelectasis, prior exam requested and awaited, suspected infection with a 1 to 3 month repeat). It is the label when the report says the category is 0.
- Ignore any category belonging to a **previous or comparison exam** and any category that the text says "would" apply under a hypothetical. Ignore a category in a referral form that the report overrides.
- A category given to one nodule alone is not the overall category unless it is the most concerning finding and no overall is stated.
- Do not recompute categories from nodule sizes, and do not apply your own clinical judgement beyond the text. Use `indeterminate` if the text gives no category and no usable assessment words, or states contradictory overall categories.

### 2.2 `ldct-workup`: one note about the diagnostic work-up after a suspicious screening result

Allowed labels (exactly these six): `wk_dna`, `wk_incomplete`, `wk_treatment`, `wk_benign`, `wk_surveillance`, `indeterminate`.

Use only what the text says about **this episode**. Ignore earlier years, other people and negated statements.

Apply the first rule that fits, in this order:

1. `wk_dna`: the person did not attend (did not attend, DNA, failed to attend, no-show) the appointment the note is about.
2. `wk_incomplete`: the person came but no work-up was completed (declined, postponed, cancelled, unwell), **or** a tissue result is awaited or preliminary, **or** the outcome is still pending (decision not yet made, discussion still to come, test booked or to follow).
3. `wk_treatment`: referred for cancer treatment (surgery, oncology, radiotherapy, stereotactic radiotherapy) **and** the tissue result is malignant **and** the note records that the case was discussed at the multidisciplinary team (MDT) or states an MDT decision. If any of the three is missing, do not use this label.
4. `wk_benign`: the person returns to annual screening **and** the tissue is not malignant (benign or non-malignant result) **or** the note records an MDT decision for the return. A bare "return to annual screening" with neither benign tissue nor an MDT decision is not enough.
5. `wk_surveillance`: an interval or repeat CT is planned (a stated plan, for example "repeat CT in 3 months", is a decision, not "pending") and any tissue result is not malignant.
6. `indeterminate`: none of the above fits, the note is too short to decide, or it is internally contradictory (for example malignant tissue with a plan to return to annual screening; surgical referral with a benign biopsy and no MDT).

### 2.3 `ifhas-followup`: one clinic note after abnormal results at an adult health screen

Allowed labels: `dna`, `indeterminate`, or a composite of one to four **domain=status** parts joined by `;`, always in this fixed domain order: `bp`, `glyc`, `lipid`, `weight`. No spaces, no trailing `;`, no repeated domain. Examples: `bp=managed`, `bp=pending;glyc=managed`, `bp=managed;glyc=pending;lipid=managed;weight=not_confirmed`.

Domains: `bp` blood pressure or hypertension; `glyc` glucose, HbA1c, diabetes or prediabetes; `lipid` LDL, triglycerides, cholesterol, statins; `weight` BMI, obesity, weight programmes.

Statuses:

- `managed`: the abnormality is confirmed at or for this visit **and** a management plan is started or arranged now (medicine started, lifestyle or weight programme enrolled, dietitian booked, specialist referral sent).
- `not_confirmed`: the repeat or confirmatory result is normal or below the diagnostic threshold, or the earlier value was an error or white-coat effect, so the person returns to routine screening. Lifestyle advice alone for prediabetes is `not_confirmed`.
- `pending`: a repeat or confirmatory test is ordered or awaited, readings are still being collected, the abnormality is confirmed but no plan has been made, or the person declined or deferred.

Rules:

- Include a domain only if the note addresses it **for this visit**. Past values ("last year"), other people (relatives, spouses), history in the header and negated statements ("no history of diabetes") do not count as addressing a domain.
- `dna` if the person did not attend; then nothing else is labelled even if old values are quoted.
- `indeterminate` if the person attended but no domain is addressed, or the note is contradictory.
- A domain mentioned with no decision at all is `pending`.

### 2.4 `ifhas-screen`: one printed results panel plus its coded results

Each item has a printed panel in `text` and a `coded` string. You return **two** labels: `label` from the panel only, and `coded_label` from the `coded` string only. Do not use one to fill in the other.

Both use the same four-cell format, domains in this fixed order, exactly four parts: `bp=<c>;glyc=<c>;lipid=<c>;weight=<c>`. Example: `bp=H;glyc=D;lipid=N;weight=O`.

Cells:

| domain | cell | meaning |
|---|---|---|
| `bp` | `N` | systolic below 140 and diastolic below 90 |
| `bp` | `H` | systolic 140 to 179 or diastolic 90 to 109 (either one) |
| `bp` | `S` | systolic 180 or above, or diastolic 110 or above |
| `glyc` | `N` | HbA1c below 5.7% (39 mmol/mol) and random glucose below 200 mg/dL (11.1 mmol/L) |
| `glyc` | `P` | HbA1c 5.7 to 6.4% (39 to 47 mmol/mol), glucose below 200 mg/dL |
| `glyc` | `D` | HbA1c 6.5 to 10.0% inclusive (48 to 86 mmol/mol) or random glucose 200 to below 300 mg/dL (11.1 to below 16.7 mmol/L) |
| `glyc` | `S` | HbA1c above 10.0% (above 86 mmol/mol) or random glucose 300 mg/dL (16.7 mmol/L) or above |
| `lipid` | `N` | LDL cholesterol below 190 mg/dL (4.9 mmol/L) and triglycerides below 500 mg/dL (5.6 mmol/L) |
| `lipid` | `H` | LDL cholesterol 190 mg/dL (4.9 mmol/L) or above, or triglycerides 500 mg/dL (5.6 mmol/L) or above |
| `weight` | `N` | BMI below 30 kg/m2 |
| `weight` | `O` | BMI 30 kg/m2 or above |
| any | `X` | the inputs for this domain are missing, unreadable, or implausible |

Apply the highest-severity matching cell for a domain (for `glyc`, check `S`, then `D`, then `P`, then `N`). `glyc` needs both an HbA1c and a random glucose; `bp` needs both systolic and diastolic; `lipid` needs both LDL and triglycerides; `weight` needs BMI. If any required input is missing the domain is `X`. Total cholesterol and HDL never change a cell. Use the reported unit; the mmol/L and mmol/mol figures above are the published equivalents, so a conversion never moves a value across a boundary.

Ignore any line of the panel that is explicitly an **earlier visit** ("previous visit", "last year", a past date).

`coded` format: parts joined by `;`, each `code|value|unit`. Codes you may meet:

| code | measure | expected unit |
|---|---|---|
| 4548-4 | HbA1c | % |
| 2345-7 | random glucose | mg/dL |
| 14749-6 | random glucose | mmol/L |
| 8480-6 | systolic BP | mm[Hg] |
| 8462-4 | diastolic BP | mm[Hg] |
| 13457-7, 18262-6, 2089-1 | LDL cholesterol | mg/dL |
| 22748-8 | LDL cholesterol | mmol/L |
| 2571-8 | triglycerides | mg/dL |
| 14927-8 | triglycerides | mmol/L |
| 39156-5 | BMI | kg/m2 |

Other codes are ignored. Compute `coded_label` with the same cell table from the coded values only.

Set `coded_label` to the single word `unavailable` (instead of a four-cell label) if the coded string cannot be trusted as a whole, that is if any of these holds: a part is malformed; a known code carries a unit different from the expected unit above; a value is physically implausible for its measure (for example HbA1c outside 3 to 20 %, glucose outside 20 to 1000 mg/dL, systolic outside 60 to 300, diastolic outside 30 to 200, LDL outside 10 to 500 mg/dL, triglycerides outside 10 to 5000 mg/dL, BMI outside 10 to 90); or the same measure appears twice with values more than 0.5 apart. `unavailable` is allowed only for `coded_label`, never for `label`. Domains with missing inputs inside an otherwise trusted coded string are `X` as in the table, not `unavailable`.

The same plausibility ranges apply when reading the panel: an implausible or unreadable panel value makes that domain `X` in `label`.

## 3. Procedure

1. Copy your template; do not edit `corpus_sha256` or `guide_sha256`.
2. Label items one at a time from the item's own text. Do not look ahead for patterns in the set; do not balance the label distribution.
3. Re-check every composite label for domain order and exact spelling.
4. Return the file as is. The coordinator validates it; a rejected file is re-labelled from scratch, not patched.
5. Do not discuss items with the other labeller. Disagreements are expected and are kept as `unadjudicated`; you will not be asked to resolve them, and no one guesses a key for them.

## 4. What your labels mean and do not mean

- They are an independent, rule-based reading of synthetic text.
- They are **not clinician-verified** and are not clinical truth.
- Two labellers agreeing means the item is *scoreable against an agreed key*, not that the label is clinically right.
- Results computed from them describe agreement of software with these labels on synthetic text, and say nothing about real patients or clinical effectiveness.
