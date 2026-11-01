# Wasl API: frontend contract (stage 5)

The backend serves a **materialised run** (the output of `scripts/demo.sh`) over a small read API. Everything it returns comes from the pipeline's own logs: decisions, FHIR/HL7 log, review queue, sign-offs, anchors and report. Nothing is invented client-side. **All data is synthetic.**

## Run it

```bash
scripts/demo.sh            # once: builds and runs the pipeline, writes out/run/v7strict-cached-fhir/ (~5 min)
scripts/demo.sh serve      # API on http://127.0.0.1:8088   (WASL_API_ADDR to change)
```

**No backend at all?** Static responses for every endpoint are in [`api-samples/`](api-samples/). They were produced by `scripts/demo.sh snapshot` against a real run, so you can build the UI against them.

CORS is open (`*`) for local development.

## Envelope

Every response includes `meta`:

```json
{"meta": {"api": "wasl-api/1", "data_mode": "simulated", "run": "v7strict-cached-fhir",
          "policy": "v7strict+cal=true+guard=true+tau=0.98", "reader_model": "gemma-4-26b/cached",
          "loaded_at": "…", "note": "All patients, providers and outcomes are synthetic …"}}
```

Lists are always `[]`, never `null`.

## Endpoints

| Method | Path | Returns |
|---|---|---|
| GET | `/healthz` | `status`, case count, `dependencies` (materialised_run, nats, reader, ingest_adapter) |
| GET | `/api/dashboard` | KPI and SLA, `states`, `dropoff`, `review` counts, `by_programme`, per-provider and per-origin tables (small-cell suppressed), headline `metrics` (31-day KPI, closure P/R, consent invariant, forged, provenance, exactly-once), `fixtures`, `capacity` (simulated booking: condition adequate, misallocated or short_overall, slots per case by region, pooling_relevant, summary, `levers`: pathway and prep-risk counts; `inputs.optimiser` appears only when the booking stage ran with a non-default optimiser or stress test, e.g. {optimiser: lookahead, lookahead_days, gamma, forecast, method}; absent = the default myopic engine) |
| GET | `/api/fixtures` | The demo cases (summaries): three CRC cases, plus `dr_exam_closure` when the run has DR loops. `review_hold` falls back to a closed, signed loop whose outreach was stopped by the consent hold or the second-identifier guard when no loop had consent held then granted |
| GET | `/api/cases` | Filterable case summaries. Query: `programme` (crc\|cervical\|lung_nodule\|diabetic_retinopathy\|breast), `state`, `dropoff`, `owner`, `provider`, `consent`, `review` (none\|open\|signed), `overdue` (true\|false), `fixture=1`, `q` (id substring), `limit` (≤500, default 100), `offset` |
| GET | `/api/cases/{id}` | Full case: `case`, `identities` (linked registrations), `timeline` (every linked FHIR/HL7 event, no free text), `links`, `evidence` (both readers, per-field values, quote, model), `holds` (why it didn't close), `outreach`, `decisions` (history, with `consent_log` and that decision's `outreach`), `review_items`, `signoffs`, `api_reviews`, `current`, `booking` (simulated suggestion and capacity condition; `status`: suggested, no_slot_within_horizon, or not_applicable; from `wasl-booking/2` also `pathway` {route: direct\|clinic, reasons, checklist_notes}, `prep_risk` {score, band: low\|high, factors, suggested_intervention, requires_clinician_order: true} and synthetic `app9` fields; `target_days` (14 or 31) only in the `--crc-urgent-share` stress test) |
| GET | `/api/cases/{id}/audit` | `chain` plus `chain_verified`; `decision_signatures` (stored vs recomputed); `anchors`; review `history` |
| POST | `/api/cases/{id}/review` | Demo review action (below) |

`{id}` is the loop id, e.g. `ahs-mushrif/ServiceRequest/AHS-sr-1102`. You can pass the slashes as-is or URL-encode them.

### Case summary

```json
{"id": "ahs-mushrif/ServiceRequest/AHS-sr-1102", "programme": "crc", "state": "closed", "dropoff": "",
 "owner": "Practitioner/…", "provider": "ahs-mushrif", "referral_day": 5.91, "deadline_day": 31, "overdue": false,
 "priority": "", "consent": "granted", "review": "none", "closed": true, "closed_day": 30.5178,
 "fixture": "evidence_closure", "decisions": 11}
```

- **States:** `open`, `booked`, `procedure_done`, `awaiting_histology`, `inadequate`, `no_show`, `referral_rejected`, `closed`.
- **Drop-off:** `never_notified`, `no_acknowledgement`, `not_booked`, `referral_rejected`, `referral_unanswered`, `no_show`, `inadequate`, `awaiting_histology`, `in_progress`, or `""` when closed.
- **Consent:** `unknown`, `granted`, `denied`, `opted_out`.
- **Days:** fractional days since 1 May 2026, the cohort epoch.

### Programme `lung_nodule` (second loop type: incidental pulmonary nodules, Fleischner 2017)

**Status:** built; served only from runs generated with `WASL_LUNG=N` (synthetic cohort, simulated adherence). Rule, states and metrics: [`pipeline/RESULTS.md`](../pipeline/RESULTS.md), "Second loop type".

- **States:** `open`, `no_followup_needed` (index CT in a Fleischner cell with no routine follow-up), `closed` (trusted follow-up CT, both readers agree stable/resolved), `escalated` (growing or solid component ≥ 6 mm → specialist/MDT; end state), `not_assessable` (repeat CT needed), `out_of_scope` (age < 35, known primary cancer or immunosuppressed; clinician decides).
- **Drop-off:** `no_followup_ct`, `unlinked`, `escalated`, `not_assessable`, `out_of_scope`, or `""` when resolved.
- `closed` is `true` for `closed` and `no_followup_needed`. `deadline_day` is the end of the Fleischner window in days after `referral_day` (the index CT date). `referral_day` can be negative: lung index CTs predate the observation window (look-back cohort).
- The summary carries a `lung` object (also in `current.lung` on the case detail):

```json
"lung": {"finding_report": "RAD-ct-1", "finding_label": "ground_glass/<6/multiple", "risk": "high", "risk_recorded": true,
  "rec": {"action": "ct", "win_lo_days": 91, "win_hi_days": 183, "next_step": "if stable, consider CT at 2 and 4 years",
          "cell": "subsolid/ground_glass/multiple/<6"},
  "window_start_day": -37.03, "window_end_day": 54.97, "scope": "in_scope"}
```

  After a follow-up CT also `outcome` (`stable|resolved|growing|suspicious_solid|not_assessable`), `followup_report`, `within_window`, `followup_ct_seen`. `rec.action` is `none|ct|ct_or_pet_or_tissue`; `rec.optional` marks Fleischner's "optional CT at 12 months".
- **Evidence** entries for lung reports carry `reader_a_regex` / `reader_b` labels and `reader_a_fields` / `reader_b_fields` (type, axes in mm, count, solid component, prior axes, quote). The chain adds `recommendation` and, for escalations, `escalation_decision` blocks.

- **Surveillance CT** (runs with `WASL_LUNGSURV=1`, flag `-lungsurv`): a PET/CT low-uptake closure of a > 8 mm nodule opens a child loop `<loop>#surv`. The parent stays `closed` and both cases carry `lung.surveillance`:

```json
"surveillance": {"loop": "rad-imaging/ServiceRequest/RAD-sr-12#surv", "reason": "pet_low", "from_report": "RAD-pet-3",
  "from_day": 20.4, "window_start_day": 80.4, "due_day": 140.4, "state": "due",
  "basis": "BTS 2015 (Callister et al., Thorax 2015;70 Suppl 2): Herder < 10% after PET/CT -> CT surveillance, first CT at about 3 months; window 60-120 days is Wasl policy"}
```

  `state` is `due|overdue|done|escalated`; `ct_report` and `ct_day` appear once a surveillance CT is read. The parent's `closure_decision` block adds `closed_into: "surveillance_ct_due"`. Escalation `overdue_lung_surveillance_ct`; outreach kinds `lung_surveillance_due`, `lung_surveillance_recall`.

- **Later surveillance CTs** (runs with `WASL_LUNGSURV2=1` and `WASL_LUNGSURV=1`, flag `-lungsurv2`; default off): a stable first surveillance CT opens `<loop>#surv2` (CT at about 1 year, window 300-420 days from the PET/CT) and a stable `#surv2` opens `<loop>#surv3` (about 2 years, 670-790 days); BTS 2015 recommends CT at 3 months and 1 year, and 2 years in total when growth is measured by 2D diameters (the windows are Wasl policy). A CT more than 30 days before a later round's window cannot decide that round; a CT inside the 30-day tolerance can decide it but has `within_window: false` because both window bounds are required (the tolerance is Wasl policy). A resolved nodule or growth ends the chain. Each later child carries `lung.surveillance` with `"round": 2|3` and `from_report` = the previous round's CT; its first chain block (`surveillance_opened`) names the previous child as `parent` with its `parent_head`. The surveillance object of each round gains `next` (the next round's object) once it opens, so the parent case shows the whole chain: `lung.surveillance.next.next`. Same states, escalation and outreach kinds as the first CT.
- **Stable-CT surveillance child** (runs with `WASL_LUNGSURVSTABLE=1`, flag `-lungsurvstable`; default off): the existing child `<loop>#surv` opened by a stable follow-up CT whose Fleischner cell recommends a further CT now also carries `lung.surveillance` with `"reason": "fleischner_stable"` (window from the Fleischner cell, `from_day` = the day it counts from; the parent case does not carry it). An untrusted link or reader disagreement on a later CT raises a review item on the parent loop and the child holds (`hold_decision`, `pending` lists the items); outreach `lung_surveillance_due` / `lung_surveillance_recall`; escalation `overdue_lung_nodule_window` as before. Without the flag the child is unchanged (no `lung.surveillance`).

### Programme `diabetic_retinopathy` (third loop type: retinal screening to eye-clinic exam, rule `doh2024dr`)

**Status:** built; served only from runs generated with `WASL_DR=N` (synthetic cohort, simulated attendance). Rule, readers and metrics: [`pipeline/RESULTS.md`](../pipeline/RESULTS.md), "Third loop type". The loop is the tracking ServiceRequest of one retinal screen at a primary-care diabetes clinic (ids such as `phc-retina/ServiceRequest/PHC-sr-118`); the eye-clinic note usually sits at another provider under another record and needs a trusted identity link.

- **Screening grade** (worse eye): `R{0|1|2|3A|3S}M{0|1}`, optional `/urgent` (the grader asks for an urgent referral), or `U` (ungradable). `doh2024dr` turns it into an action:
  - `R0`/`R1` without maculopathy → `rescreen` (12 months, no overdue clock);
  - `U` → `repeat_photo` within 28 days;
  - `R3A` or `/urgent` → `refer`, urgent, 42 days;
  - `R2`, `R3S` or `M1` → `refer`, routine, 91 days.
- **States:**
  - `no_referral_needed`: the grade is not referable; resolved (rescreen in 12 months recorded).
  - `open`: no decision yet. Covers an ungradable screen waiting for its repeat photo (28-day clock; a later graded screen on the same registration decides the loop), a referral waiting for the eye-clinic exam, a clinic visit where the retina was not examined, and anything held for review (unread or disagreeing reports, unconfirmed identity link).
  - `closed`: a referable grade, then an eye-clinic note on a trusted link that both readers (or a clinician) read as a retinal exam with an outcome.
  - `no_show`: did not attend the eye clinic (rebook outreach).
  - `not_assessable`: no fundal view (clinician decides).
- **Drop-off:** `no_eye_exam`, `unlinked`, `ungradable_no_repeat`, `no_show`, `not_assessable`, or `""` when resolved.
- `closed` is `true` for `closed` and `no_referral_needed`. `deadline_day` on the summary is the rule's deadline in days (28, 42, 91, or 365 for a rescreen); the absolute due day is `dr.deadline_day`. `overdue` is set when a referral is still `open` or `no_show`, or a repeat photo is still `open`, after `dr.deadline_day`. `referral_day` can be negative (look-back cohort).
- The summary carries a `dr` object (also in `current.dr` on the case detail):

```json
"dr": {"screen_report": "PHC-dr-123", "screen_label": "R3SM0",
  "rec": {"action": "refer", "deadline_days": 91, "urgency": "routine", "cell": "R3SM0"},
  "grade_day": -49.64, "deadline_day": 41.36,
  "clinic_note": "EYE-dr-7", "outcome": "examined_followup", "examined": true, "within_deadline": true}
```

  - `rec.action` is `refer|rescreen|repeat_photo`; `rec.urgency` is `urgent|routine` (referrals only).
  - `coded_grade: true` means the grade came from a coded result (grading software) that matched reader A's reading of the summary text.
  - `clinic_note`, `outcome`, `examined` and `within_deadline` appear once an eye-clinic note has been read. `outcome` is `examined_treated`, `examined_followup`, `examined_discharged`, `not_examined`, `dna` or `not_assessable`.
  - Before a grade is decided, only `grade_day` (the referral day) is present.
- **Evidence** entries (one for the screening report, one per eye-clinic note read) carry:
  - `reader_a_regex` / `reader_b` labels, `agree`, `label` (`disagree` when the readers differ), `human_override`, `reader_model` (`coded_grade` for a coded screening grade), `rule`;
  - `reader_a_fields` / `reader_b_fields`. Screening: `r_right`, `r_left` (`0|1|2|3A|3S|U`), `m_right`, `m_left` (`0|1`), `urgent`. Clinic: `attended` (`yes|no`), `fundus` (`examined|not_examined|no_view`), `outcome` (`treated|followup|discharged|none`). Both: `label` and a verbatim `quote`.
- The chain adds a `recommendation` block (`rule: doh2024dr`, `cell`, `action`, `deadline_days`, `urgency`, `grade_day`, `source`). A `closure_decision` closes it: for `no_referral_needed` with `next_step`; for an exam with `outcome`, `deadline_day` and `within_deadline`. Otherwise a `hold_decision` gives the `reason`:
  - `screening_report_not_received`, `screening_readers_disagree`, `screening_label_unknown`;
  - `ungradable_images` (with `due_day`);
  - `awaiting_eye_clinic_exam`, `retina_not_examined`, `clinic_note_readers_disagree`, `clinic_note_identity_link_unconfirmed`;
  - `did_not_attend_rebook`, `no_fundal_view_clinician_decides`.
- **Escalations** (`decisions[].escalations`): `overdue_dr_referral`, `repeat_photo_overdue`, `urgent_dr_referral_open`, `rebook_after_dna`, `clinician_decision_no_fundal_view`.
- **Outreach kinds:** `dr_repeat_photo`, `dr_clinic_due` (14 days before the deadline), `dr_overdue_recall`, `dr_rebook`. Sent by phone, then letter, in the screening clinic's name, under the same consent hold as the other programmes.

- **Next due** (runs with `WASL_DRNEXT=1`, flag `-drnext`): a resolved loop's `dr` object adds `next_due` (omitted otherwise):

```json
"next_due": {"kind": "clinic_review", "from_day": -15.45, "interval_days": 183, "due_day": 167.55, "basis": "note_review_interval"}
```

  `kind` is `rescreen|clinic_review`; `basis` is `doh2024dr_annual_rescreen`, `discharged_to_annual_screening`, `note_review_interval`, `assumed_followup_91d` or `assumed_treated_28d`. `seen`, `seen_day` and `seen_report` mark the first later read screen or attended clinic visit; `recall_overdue: true` (with escalation `dr_recall_overdue`) when none came by `due_day`. The loop stays resolved.

### Programme `breast` (fourth loop type: screening mammogram recall to assessment outcome, rule `doh2025breast`)

**Status:** built; served only from runs generated with `WASL_BREAST=N` (synthetic cohort, simulated attendance). Rule, readers and metrics: [`pipeline/RESULTS.md`](../pipeline/RESULTS.md), "Fourth loop type". The loop is one screening mammogram at the breast screening unit; the assessment document usually sits at the assessment unit under another record and needs a trusted identity link.

- **Screening category** (worse breast, ACR BI-RADS order 1 < 2 < 3 < 0 < 4 < 5): `B0`–`B5`, or `TR` (technically inadequate). `doh2025breast` turns it into an action: `B1`/`B2` → `routine` (2 years); `B0`/`B3` → `recall`, 14 days; `B4`/`B5` → `recall`, 21 days (`urgency` suspicious/urgent); `TR` → `repeat_mammogram`, 21 days.
- **States:** `no_recall_needed` (resolved), `open` (waiting for the assessment outcome or the repeat, or held for review), `closed` (an assessment document on a trusted link that both readers or a clinician read as an outcome), `no_show`.
- **Drop-off:** `technical_repeat_not_done`, `unlinked`, `no_assessment_outcome`, `no_show`, or `""` when resolved.
- `closed` is `true` for `closed` and `no_recall_needed`. `referral_day` can be negative (look-back cohort).
- The summary carries a `breast` object (also in `current.breast`):

```json
"breast": {"screen_report": "BSU-br-107", "screen_label": "B0", "coded_birads": true,
  "rec": {"action": "recall", "deadline_days": 14, "urgency": "routine", "cell": "B0"},
  "screen_day": -101.82, "deadline_day": -87.82, "assessment_document": "BAU-br-8", "outcome": "return_routine"}
```

  `outcome` is `return_routine`, `early_recall` (flagged for audit), `to_treatment` (needs a malignant needle biopsy in the same document; adds `treatment_referral_due_day`, recorded only), `incomplete` or `dna`. `within_deadline` appears once an outcome is read. `coded_birads: true` means a coded final assessment (CPT II 3340F–3345F) matched reader A's reading of the text.
- **Hold reasons:** `screening_report_not_received`, `screening_readers_disagree`, `screening_label_unknown`, `technically_inadequate`, `awaiting_assessment`, `assessment_readers_disagree`, `assessment_identity_link_unconfirmed`, `assessment_incomplete_or_mdt_pending`, `did_not_attend_rebook`.
- **Escalations:** `overdue_breast_recall`, `technical_repeat_overdue`, `rebook_after_dna`, `urgent_breast_recall_open`, `early_recall_audit`.
- **Outreach kinds:** `breast_assessment_due`, `breast_overdue_recall`, `breast_rebook`, `breast_repeat_mammogram`, under the same consent hold.
- **Review labels** (clinician role): `B0`–`B5`, `TR`, `return_routine`, `early_recall`, `to_treatment`, `incomplete`, `dna`.

### POST review (demo mode)

**Headers:** `X-Wasl-Role` (`coordinator` | `clinician` | `reviewer` | `auditor`) and `X-Wasl-Actor` (who is signing).

**Body:** `{"item": "<review item id from review_items>", "decision": "same|different|adequate|inadequate|confirm_contact", "note": "…"}`

**RBAC:**
- `same`/`different` need a role allowed to sign links (reviewer).
- `adequate`/`inadequate` need one allowed to sign evidence (clinician). On `lung_nodule` cases the evidence decision is a lung label instead: `stable`, `resolved`, `growing`, `suspicious_solid`, `not_assessable`, or an index-CT label such as `solid/6-8/single` (same clinician role).
- On `diabetic_retinopathy` cases the evidence decision can be a DR label (same clinician role):
  - a screening grade: `R0M0`, `R1M0`, `R2M1`, `R3SM0`, `R3AM1`, `R1M0/urgent`, `U`;
  - a clinic outcome: `examined_treated`, `examined_followup`, `examined_discharged`, `not_examined`, `dna`, `not_assessable`.
  Anything else (for example `R9M0`, `R2M2`, or a lung label) is rejected with 400.
- `confirm_contact` needs one allowed to record consent (coordinator, clinician).

**Response codes:**
- 201 with the appended event;
- 403 for a role that isn't allowed;
- 404 for an item that wasn't raised for this case;
- 400 for a malformed body or missing actor.

**Append-only:** every action is appended to `<run>/api_reviews.ndjson` as a hash-chained event (`prev` → `hash`). The run's decisions are never modified. Review history shows in `api_reviews` and in `/audit` → `history`.

## The demo cases

| Fixture | What it shows | Key fields |
|---|---|---|
| `evidence_closure` | FIT+ referral, Arabic-script identity, colonoscopy at another provider under a different record, trusted link, both readers agree with a verbatim quote, consent granted, closed, chain verified | `identities` (2 providers), `evidence[0].agree=true`, `label=adequate`, `quote`; `/audit` → `chain_verified=true` |
| `false_closure_trap` | A procedure exists but the documentation is incomplete, so Wasl **holds** instead of closing | `case.closed=false`, `state=inadequate`; `holds[0]` gives the report and rule |
| `review_hold` | Uncertain link and reader disagreement both went to double review (2 reads, signed); outreach was held while consent was unknown, released after the grant; then closed | `review_items`, `signoffs[].reads=[r1,r2]`, `decisions[].consent_log`: held → released, or (when no loop in the run had consent held then granted) a closed, signed loop whose `decisions[].outreach[].suppress` shows a message stopped by the consent hold or the second-identifier guard |
| `dr_exam_closure` | **Only in runs with DR loops.** A referable retinal screen (R2, R3S, R3A or M1) closed by an eye-clinic exam at another provider within the deadline. Both text readers agree on the screen and on the clinic note (no coded grade), the link is cross-provider and trusted, consent is granted, and no human was needed at any point (no review item, sign-off, override or earlier disagreement) | `case.dr` (`screen_label`, `rec`, `outcome`, `examined=true`, `within_deadline=true`); `evidence` (2 entries, `agree=true`, `reader_a_fields`/`reader_b_fields` with quotes); `links[0].hops≥1`; `/audit` → `chain_verified=true` |

Fixtures are chosen deterministically from the run (the first matching case by id: CRC for the first three, `diabetic_retinopathy` for `dr_exam_closure`), so the same run always gives the same cases. The three CRC keys and their selection rules are unchanged; a run without DR loops has no `dr_exam_closure` key.
