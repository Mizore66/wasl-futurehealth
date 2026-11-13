# Wasl API: frontend contract (stage 5)

The backend serves a **materialised run** (the output of `scripts/demo.sh`) over a small read API. Everything it returns comes from the pipeline's own logs: decisions, FHIR/HL7 log, review queue, sign-offs, anchors and report. Nothing is invented client-side. **All data is synthetic.**

## Run it

```bash
scripts/demo.sh            # once: builds and runs the pipeline, writes out/run/v7strict-cached-fhir/ (~5 min)
scripts/demo.sh serve      # API on http://127.0.0.1:8088   (WASL_API_ADDR to change)
```

**No backend at all?** Static responses for every endpoint are in [`api-samples/`](api-samples/). They were produced against a real synthetic run, so you can build the UI against them. The committed files are legacy public synthetic fixtures; newly generated snapshots use only redacted auditor exports. See [`SECURITY.md`](SECURITY.md) for that intentional exception.

Every `/api` route requires a bearer token. `scripts/demo.sh` supplies loopback-only development tokens and the required `WASL_SYNTHETIC_DATA_ONLY=1` acknowledgment. For example:

```bash
curl -H 'Authorization: Bearer wasl-demo-reviewer-1-token' http://127.0.0.1:8088/api/fixtures
curl -H 'Authorization: Bearer wasl-demo-auditor-token' http://127.0.0.1:8088/api/cases/<id>/audit
```

A direct `wsi7 serve` requires `WASL_API_TOKENS` entries in `token:role:actor` form (comma-separated). Missing/invalid credentials receive 401; a valid principal without the required read action receives 403. Client-supplied `X-Wasl-Role` and `X-Wasl-Actor` headers are not authentication and are overwritten from the token. These static demo tokens are not a production identity system.

CORS is disabled by default. Set `WASL_API_ORIGINS` to an exact comma-separated trusted-origin allowlist if a browser client needs it; wildcard origins and self-asserted identity headers are not allowed.

## Envelope

Every response includes `meta`:

```json
{"meta": {"api": "wasl-api/1", "data_mode": "simulated", "run": "v7strict-cached-fhir",
          "policy": "v7strict+cal=true+guard=true+tau=0.98", "reader_model": "gemma-4-26b/cached",
          "loaded_at": "…", "note": "All patients, providers and outcomes are synthetic …"}}
```

Lists are always `[]`, never `null`.

## Endpoints

Read authorization is least-privilege: coordinator/clinician/reviewer tokens have `ActReadQueue` for dashboard, fixtures, lists and raw case detail; auditor tokens have `ActReadAudit` for redacted audit and export and cannot read the raw queue. `/healthz` is the sole unauthenticated operational route. Raw quotes and review notes are clinical-by-design and limited to queue-authorized detail; registration keys in detail, audit and export copies are replaced by deterministic `reg_…` pseudonyms that preserve equality joins. These unkeyed, linkable pseudonyms are not semantic anonymity. Audit responses use the redacted chain.

| Method | Path | Returns |
|---|---|---|
| GET | `/healthz` | `status`, case count, `dependencies` (materialised_run, nats, reader, ingest_adapter) |
| GET | `/api/operations` | Authenticated `wasl-operations/1` point-in-time API/NATS/durable-queue observation, completed synthetic-run metrics, explicit current-API unmeasured states, and latest completed bundle metadata age |
| GET | `/api/dashboard` | KPI and SLA, `states`, `dropoff`, `review` counts, `by_programme`, per-provider and per-origin tables (small-cell suppressed), headline `metrics` (31-day KPI, closure P/R, consent invariant, forged, provenance, exactly-once), `fixtures`, `capacity` (simulated booking: condition adequate, misallocated or short_overall, slots per case by region, pooling_relevant, summary, `levers`: pathway and prep-risk counts; `inputs.optimiser` appears only when the booking stage ran with a non-default optimiser or stress test, e.g. {optimiser: lookahead, lookahead_days, gamma, forecast, method}; absent = the default myopic engine) |
| GET | `/api/fixtures` | The demo cases (summaries): three CRC cases, plus `dr_exam_closure` when the run has DR loops. `review_hold` falls back to a closed, signed loop whose outreach was stopped by the consent hold or the second-identifier guard when no loop had consent held then granted |
| GET | `/api/cases` | Filterable case summaries. Query: `programme` (crc\|cervical\|lung_nodule\|diabetic_retinopathy\|breast\|lung_ldct), `state`, `dropoff`, `owner`, `provider`, `consent`, `review` (none\|open\|signed), `overdue` (true\|false), `fixture=1`, `q` (id substring), `limit` (≤500, default 100), `offset` |
| GET | `/api/cases/{id}` | Full case: `case`, `identities` (linked registration pseudonyms), `timeline` (every linked FHIR/HL7 event, no free text), `links`, `evidence` (both readers, per-field values, quote, model), `holds` (why it didn't close), `outreach`, `decisions` (history, with `consent_log` and that decision's `outreach`), `review_items`, `signoffs`, `api_reviews`, `current`, `agent_investigations`, `agent_provenance` (see "Cached agent investigations"), `booking` (simulated suggestion and capacity condition; `status`: suggested, no_slot_within_horizon, or not_applicable; from `wasl-booking/2` also `pathway` {route: direct\|clinic, reasons, checklist_notes}, `prep_risk` {score, band: low\|high, factors, suggested_intervention, requires_clinician_order: true} and synthetic `app9` fields; `target_days` (14 or 31) only in the `--crc-urgent-share` stress test) |
| GET | `/api/cases/{id}/audit` | Redacted `chain` plus `chain_verified`; `decision_signatures` (stored vs recomputed); `anchors`; redacted review `history` |
| POST | `/api/cases/{id}/review` | Demo review action (below) |
| POST | `/api/cases/{id}/review/claim` | Append an independent-review claim, renew, release or takeover event (below) |
| POST | `/api/cases/{id}/actions` | Coordinator case action: assign owner, snooze, contact outcome, note (below) |
| GET | `/api/agentshadow/status` | Read-only agent-shadow lifecycle/progress/caps, `available`, `status`, `clean_success`, optional `health.ts`; `done` is not success ([details](AGENT-SHADOW.md#status-api)) |
| POST | `/api/cases/{id}/consent` | Demo consent event (`consent_granted`, `consent_denied`, `consent_opted_out`) plus distinct `api_contact`/`demo_pending_consent` overlays; canonical consent/outreach unchanged ([details](API-CONSENT.md)) |
| POST | `/api/cases/{id}/undo` | Compensating undo/contest event with target, independent reviewer and preserved history ([details](API-UNDO.md)) |
| GET | `/api/cases/{id}/export` | Auditor-only, internal-audit `wasl-case-export/1` with redacted detail/audit, fixed metadata, deterministic `content_sha256` and restart-stable `comparison_sha256`; not de-identified or approved for external disclosure ([details](API-EXPORT.md)) |
| GET | `/api/recovery/slots` | `wasl-recovery/1` synthetic released-capacity summaries; optional `programme=crc\|cervical` and `status=open\|filled\|stale` filters |
| GET | `/api/recovery/slots/{slot_id}` | No-show release provenance, future synthetic-session capabilities, ranked eligible and excluded candidates with checks, outreach gates and viewer-filtered history |
| POST | `/api/recovery/slots/{slot_id}/confirm` | Coordinator-only atomic simulated backfill with slot/case heads and a 16–80 character idempotency key; never books a provider slot or dispatches a message |

`{id}` is the loop id, e.g. `ahs-mushrif/ServiceRequest/AHS-sr-1102`. You can pass the slashes as-is or URL-encode them.

### No-show recovery and waitlist backfill

`scripts/demo.sh booking` runs `booking/recovery.py` immediately after the existing booking generator and writes `<run>/recovery.json` (`wasl-recovery-input/1`). The generator considers only canonical CRC/cervical no-show cases with a latest trusted Appointment status of `noshow`. A released slot is a **simulated released-capacity projection** over a future synthetic calendar, never the missed provider Appointment or a real cancellation. Existing suggestions remain unheld; no overbooking headroom is consumed.

The list returns `status: not_generated` when the input is absent and `invalid` when it is malformed. Confirmation returns 503 in either state. Recovery events replay from the ordinary `api_reviews.ndjson` hash chain; missing/changed input, altered source fingerprints, broken heads, invalid events or truncated history fail closed. Confirmations use strict JSON (maximum 8 KiB) with `case_id`, `expected_slot_head`, `expected_case_head`, `request_id`, `outreach_intent` and an optional note of at most 500 characters. Identical retries return 200 with `duplicate: true`; reusing a key for another payload returns 409 `request_id_reused`.

Candidates are ordered by urgent canonical priority, absolute deadline, requested day and case ID. Every excluded candidate remains visible with all failed hard checks. Deadline lateness and language/after-hours/soft-female mismatches are warnings, not new clinical exclusions. Consent does not change tracking eligibility, but an outreach intent requires effective consent `granted` and current canonical same-provider second-identifier evidence. An accepted intent is only `recorded_not_dispatched`; later denial or opt-out changes it to `cancelled_consent` without erasing the simulated backfill or history. No recovery path calls `DispatchC`, changes canonical clinical decisions or creates a provider reservation.

### Synthetic operational readiness

`GET /api/operations` requires the same bearer authentication and `ActReadQueue` permission as other shared reads; an auditor receives 403. Only GET is allowed (other methods return 405). `/healthz` remains anonymous and its response is unchanged. Both handlers use one health-body implementation, so the nested `api.status` and `api.dependencies` cannot diverge from `/healthz`.

The top-level response contains `schema: "wasl-operations/1"`, the ordinary `meta`, and `observed_at`. `observed_at` is the API/NATS/queue/backup observation time in RFC3339 format with subsecond precision when present; `meta.loaded_at` remains the separate freshness timestamp for the loaded completed-run data. The sections are:

- `api`: `status`, `source: "/healthz"`, and the five dependency strings `materialised_run`, `nats`, `reader`, `ingest_adapter`, `review_ledger`. The dependency `nats` string retains the existing health-endpoint meaning; it is not a JetStream readiness result.
- `nats`: `status: reachable|unreachable`, `source: "nats_jetstream_consumer_info"`, and `reason: "jetstream_probe_failed"` when unreachable. This is one bounded authenticated JetStream probe, not cluster-quorum proof.
- `queue`: `status: available|partial|unavailable`, the same source, `coverage: "known_durable_consumers"`, optional `reason: nats_unreachable|consumer_info_unavailable`, and `consumers`. Only `FHIRIN/notifications`, `FHIR/prep`, and `OUT/reviewer` are covered; ordered ephemeral consumers are not continuously measured. An available consumer has non-negative integer `num_pending` and `num_ack_pending`. An unavailable consumer has `reason: consumer_not_found|consumer_info_failed` and no counts. Partial data preserves other measured consumers. If NATS is unreachable, `consumers` is `[]`; failed observations never become zero backlog or completed-run backlog.
- `run`: `status: available|unmeasured`, `scope: "completed_synthetic_run"`, `source: "report7.json"`, and, only when fully recorded, `latency_ms` (`n`, `p50`, `p90`, `p99`, `max`) and `recovery` (`generated_resources`, `fhir_msgs`, `expected_outputs`, `live_outputs`, `ingest_lost`, `ingest_duplicates`, `lost_outputs`, `duplicate_outputs`). Otherwise it has `reason: "run_metrics_not_recorded"` and no zero-filled metrics. These are accumulated synthetic-run observations, not current API latency/error rates or an SLO.
- `api_measurements`: current `latency` and `error_rate` are explicitly `unmeasured`, with `api_request_latency_not_recorded` and `api_request_outcomes_not_recorded`; the API does not infer them from `run`.
- `backup`: `source: "recovery_bundle_manifest"` and one of three states. `unmeasured` means no manifest was configured (`backup_manifest_not_configured`). `unavailable` uses `backup_manifest_unreadable`, `backup_manifest_invalid`, `backup_created_at_invalid`, or `backup_created_at_in_future`, without `age_seconds`. `available` includes `bundle_schema` (`wasl-file-backup-v1` or `wasl-file-backup-v2-aesgcm`), `created_at`, non-negative integer `age_seconds`, and `verification_status: "not_verified_by_api"`.

Set `WASL_BACKUP_MANIFEST` to one explicit `manifest.json` path to enable the backup observation. The production-shaped Compose API sets it to `/backup/bundle/manifest.json` and mounts the recovery service's external `wasl-backup` volume read-only at `/backup`. Input is bounded at 64 KiB; the required v1/v2 manifest fields and directory/payload-file records must be structurally valid, and `created_at` must be valid RFC3339, not later than `observed_at`; age is computed server-side. The response exposes only schema, creation time, age, and stable reason codes—not paths, key IDs, wrapped keys, payloads, hosts, credentials, or internal errors.

**The backup value is the latest structurally valid bundle manifest's age only.** The API does not authenticate the manifest, read recovery keys, open or decrypt payload files, perform a restore, or verify bundle integrity. It does not claim a verified backup, successful restore, stale threshold, or RPO. See [RECOVERY.md](RECOVERY.md).

The default-deny snapshot projector accepts this same contract for `docs/api-samples/operations.json`; unknown/internal fields are discarded rather than published. The pipeline page labels published snapshots separately from live checks and replaces the whole operational panel with an unavailable state on fetch or schema failure.

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
- **Rejected work-up referral message** (flag `-rejmsg`, `WASL_REJMSG=1`, default off): when the PET/CT or tissue-sampling unit finally rejects the referral (hand-off packet), the decision's `outreach` contains only kind `lung_referral_rejected` (template `lnfu-rejected-v2/en|ar`), replacing contradictory due/overdue outreach for that referral. It is sent to the imaging centre's own registration only with granted consent (unknown is held; denied/opted-out is cancelled, regardless of the legacy consent-hold switch). The packet's `patient_informed` is `true` once it is released.

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
- **Outreach kinds:** `dr_repeat_photo`, `dr_clinic_due` (14 days before the deadline), `dr_overdue_recall`, `dr_rebook`; with `-drrecallmsg` also `dr_next_recall` (below); with `-rejmsg` also `dr_referral_rejected` (template `drsfu-rejected-v2/en|ar`) when the eye clinic finally rejects the referral. The rejection message replaces contradictory due/overdue/rebooking outreach for that referral, and the hand-off packet's `patient_informed` is `true` once it is released. Sent by phone, then letter, in the screening clinic's name. The new `-drrecallmsg` and `-rejmsg` messages always require granted consent (unknown is held; denied/opted-out is cancelled), even if the legacy consent-hold switch is disabled.

- **Next due** (runs with `WASL_DRNEXT=1`, flag `-drnext`): a resolved loop's `dr` object adds `next_due` (omitted otherwise):

```json
"next_due": {"kind": "clinic_review", "from_day": -15.45, "interval_days": 183, "due_day": 167.55, "basis": "note_review_interval"}
```

  `kind` is `rescreen|clinic_review`; `basis` is `doh2024dr_annual_rescreen`, `discharged_to_annual_screening`, `note_review_interval`, `assumed_followup_91d` or `assumed_treated_28d`. `seen`, `seen_day` and `seen_report` mark the first later read screen or attended clinic visit; `recall_overdue: true` (with escalation `dr_recall_overdue`) when none came by `due_day`. The loop stays resolved.
  - With `-drrecallmsg` (`WASL_DRRECALLMSG=1`, default off), a loop with `recall_overdue: true` also gets outreach kind `dr_next_recall` (template `drsfu-recall-v2/en|ar`), sent to the screening registration only with granted consent; the loop stays resolved.
  - With `-drseenlink` (`WASL_DRSEENLINK=1`, default off), a later eye-clinic note on an untrusted identity link (not read as did-not-attend), when no trusted later visit exists, raises an ordinary link review item (`review_items`, one question per registration pair); it counts as `seen` only after a human "same". The loop's `pending` stays empty (it is resolved); the question is in the review queue.

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

### Programme `lung_ldct` (fifth loop type: lung cancer screening LDCT to the Lung-RADS next step, rule `acr2022lungrads`)

**Status:** built; served only from runs generated with `WASL_LDCT=N` (synthetic cohort, simulated attendance; readers measured on their own dev corpus only). Design: [`docs/research/ldct-loop-design.md`](research/ldct-loop-design.md). Not the incidental-nodule programme `lung_nodule` (Fleischner): different providers, report kinds and labels. The loop is one screening LDCT at a lung cancer screening centre (ids such as `lcs-centre/ServiceRequest/LCS-sr-12`). Completion and follow-up LDCTs sit on the same record; 4B/4X work-up documents sit at a thoracic diagnostic unit under another record and need a trusted identity link.

- **Category** (Lung-RADS v2022; a stated final category wins, else the most concerning finding, 1 < 2 < 3 < 4A < 4B < 4X): `LR0`, `LR1`, `LR2`, `LR3`, `LR4A`, `LR4B`, `LR4X`. `acr2022lungrads` turns it into an action: `LR1`/`LR2` → `annual` (next LDCT due at +365, no clock); `LR0` → `complete_ldct` (91 days; a later categorised report on the same record replaces it); `LR3` → `followup_ldct` (due 183, overdue after 213); `LR4A` → `followup_ldct` (due 91, overdue after 121, `suspicious`); `LR4B`/`LR4X` → `workup` (30 days, `urgent`). The clocks are Wasl assumptions (design doc A1–A3).
- **States:** `no_recall_needed` (Lung-RADS 1/2, resolved), `open` (waiting for the completion, follow-up LDCT or work-up outcome, or held for review), `closed` (3/4A: an independent follow-up LDCT read with a category, excluding addenda and same-day rereads; 4B/4X: a work-up document on a trusted link read as an outcome), `no_show` (work-up not attended).
- **Drop-off:** `incomplete_scan_not_completed`, `followup_ldct_not_done`, `unlinked`, `no_workup_outcome`, `no_show`, or `""` when resolved.
- `closed` is `true` for `closed` and `no_recall_needed`. `deadline_day` on the summary is the rule deadline in days (0 for annual); the absolute days are `ldct.due_day` and `ldct.deadline_day`. `referral_day` can be negative (look-back cohort).
- The summary carries an `ldct` object (also in `current.ldct`):

```json
"ldct": {"scan_report": "LCS-ct-41", "scan_label": "LR4A", "coded_lungrads": true,
  "rec": {"action": "followup_ldct", "due_days": 91, "deadline_days": 121, "urgency": "suspicious", "cell": "LR4A",
          "next_step": "LDCT in 3 months; PET/CT may be used if a solid component >= 8 mm"},
  "scan_day": -150.2, "due_day": -59.2, "deadline_day": -29.2,
  "followup_report": "LCS-ct-77", "followup_label": "LR3", "followup_next_step": "LDCT in 6 months",
  "outcome": "followup_done", "within_deadline": true}
```

  `outcome` is `followup_done` (3/4A) or a work-up label: `wk_treatment` (needs malignant tissue and a documented MDT discussion in the same document, DoH 7.6.1), `wk_benign` (annual screening plus benign tissue or an MDT decision; a bare annual phrase is indeterminate), `wk_surveillance` (MDT surveillance CT plan), `wk_incomplete` or `wk_dna`; work-up loops add `workup_document`. `coded_lungrads: true` means a coded Lung-RADS value (placeholder system `urn:wsi:lung-rads`) matched reader A's reading of the text; this is not independent full-report validation and does not prove the category is correct.
- **Evidence** `reader_a_fields` / `reader_b_fields`: LDCT report `lungrads_overall`, `lungrads_worst`; work-up `attended`, `workup` (`done|not_done`), `mdt` (`discussed|not_discussed`), `tissue` (`malignant|benign|pending|none`), `outcome` (`treatment|annual|surveillance|pending`); both `label` and a verbatim `quote`. `reader_model` is `coded_lungrads` on the coded path.
- **Hold reasons:** `scan_report_not_received`, `scan_readers_disagree`, `scan_label_unknown`, `scan_label_wrong_kind`, `awaiting_completion`, `completion_readers_disagree`, `awaiting_followup_ldct`, `followup_readers_disagree`, `followup_incomplete`, `followup_addendum_not_interval_ldct`, `followup_too_early`, `followup_label_unknown`, `awaiting_workup`, `workup_readers_disagree`, `workup_label_wrong_kind`, `workup_identity_link_unconfirmed`, `workup_incomplete_or_mdt_pending`, `did_not_attend_rebook`.
- **Escalations:** `overdue_ldct_followup`, `overdue_ldct_workup`, `rebook_after_dna`, `urgent_ldct_workup_open`, `followup_needs_tracking` (the follow-up scan itself needs action; the centre opens a new tracking request for it), `surveillance_after_workup`.
- **Outreach kinds:** `ldct_followup_due` (14 days before the deadline), `ldct_followup_overdue`, `ldct_workup_rebook`, `ldct_workup_overdue`, under the same consent hold.
- **Review labels** (clinician role): `LR0`, `LR1`, `LR2`, `LR3`, `LR4A`, `LR4B`, `LR4X`, `wk_treatment`, `wk_benign`, `wk_surveillance`, `wk_incomplete`, `wk_dna`.

### Programme `ifhas` (IFHAS abnormal cardiometabolic results to documented follow-up, rule `doh2025ifhas`)

**Status:** built; served only from runs generated with `WASL_IFHAS=N` (synthetic cohort, simulated attendance and results). `GET /api/cases?programme=ifhas` filters to it. Design: [`docs/research/ifhas-loop-design.md`](research/ifhas-loop-design.md). The loop is one IFHAS screening visit at the screening facility; follow-up notes usually sit at the receiving family / internal medicine clinic under another record and need a trusted identity link.

- **Screening label:** `bp=<N|H|S>;glyc=<N|P|D|S>;lipid=<N|H>;weight=<N|O>` (keys sorted). `doh2025ifhas` turns it into an action: no abnormal domain (`N`, or glycaemia `P` prediabetes) → `routine` (rescreen in 3 years, 1095 days); otherwise `follow_up` with one clock per abnormal domain: 10 days for a severe cell (`S`, `urgency: urgent`), 59 days otherwise (`urgency: routine`). `deadline_days` is the earliest clock.
- **Follow-up document label:** `dna`, or `<domain>=<managed|not_confirmed|pending>;…` over the domains the note addresses.
- **States:** `no_followup_needed` (resolved), `open` (waiting, some domains pending, or held for review), `closed` (every abnormal domain `managed` or `not_confirmed` by documents on a trusted link that both readers or a clinician read), `no_show`.
- **Drop-off:** `unlinked`, `no_followup_documented`, `no_show`, or `""` when resolved. `closed` is `true` for `closed` and `no_followup_needed`; `priority` is `urgent` when a cell is severe.
- The summary carries an `ifhas` object (also in `current.ifhas`):

```json
"ifhas": {"screen_report": "IFH-dr-12", "screen_label": "bp=H;glyc=D;lipid=N;weight=N", "coded_results_agree": true,
  "rec": {"action": "follow_up", "deadline_days": 59, "urgency": "routine", "cell": "bp=H;glyc=D;lipid=N;weight=N",
          "abnormal_domains": ["bp", "glyc"], "domain_deadline_days": {"bp": 59, "glyc": 59}},
  "screen_day": -40.2, "deadline_day": 18.8, "domain_status": {"bp": "managed", "glyc": "not_confirmed"},
  "resolved_day": {"bp": 2.1, "glyc": 2.1}, "followup_documents": ["IFU-dr-31"], "outcome": "bp=managed;glyc=not_confirmed",
  "within_deadline": true, "next_step": "…"}
```

  `domain_status` has one entry per abnormal domain (`""` until a document addresses it). `coded_results_agree: true` means the printed results panel and the coded LOINC results gave the same label. `rec.prediabetes: true` marks glycaemia `P` (lifestyle advice and a yearly HbA1c, recorded only).
- **Hold reasons:** `screening_results_not_received`, `screening_readers_disagree`, `screening_label_unknown`, `awaiting_followup`, `followup_readers_disagree`, `followup_label_unknown`, `followup_identity_link_unconfirmed`, `followup_incomplete_domains_pending`, `did_not_attend_rebook`.
- **Escalations:** `overdue_ifhas_followup`, `urgent_ifhas_result_open`, `rebook_after_dna`.
- **Outreach kinds:** `ifhas_followup_due`, `ifhas_overdue_followup`, `ifhas_rebook`, under the same consent hold.
- **Review labels** (clinician role): a screening label as above, a follow-up label as above (domain order free), or `dna`.

### POST review (demo mode)

**Identity:** the role (`coordinator` | `clinician` | `reviewer` | `senior_reviewer` | `senior_clinician` | `auditor`) and the actor (who is signing) come from the bearer token's `WASL_API_TOKENS` entry. `X-Wasl-Role`/`X-Wasl-Actor` sent by a client are overwritten.

**Body:** `{"item": "<review item id from review_items>", "decision": "same|different|adequate|inadequate|confirm_contact", "note": "…"}`

**RBAC:**
- `same`/`different` need a role allowed to sign links (reviewer).
- `adequate`/`inadequate` need one allowed to sign evidence (clinician). On `lung_nodule` cases the evidence decision is a lung label instead: `stable`, `resolved`, `growing`, `suspicious_solid`, `not_assessable`, or an index-CT label such as `solid/6-8/single` (same clinician role).
- On `diabetic_retinopathy` cases the evidence decision can be a DR label (same clinician role):
  - a screening grade: `R0M0`, `R1M0`, `R2M1`, `R3SM0`, `R3AM1`, `R1M0/urgent`, `U`;
  - a clinic outcome: `examined_treated`, `examined_followup`, `examined_discharged`, `not_examined`, `dna`, `not_assessable`.
  Anything else (for example `R9M0`, `R2M2`, or a lung label) is rejected with 400.
- On `lung_ldct` cases the evidence decision can be an LDCT label (same clinician role): `LR0`–`LR4X` or a work-up label `wk_treatment`, `wk_benign`, `wk_surveillance`, `wk_incomplete`, `wk_dna`. A label of another programme (for example `B4`) is rejected with 400.
- `confirm_contact` needs one allowed to record consent (coordinator, clinician).

**Response codes:**
- 201 with the appended event;
- 403 for a role that isn't allowed;
- 404 for an item that wasn't raised for this case;
- 400 for a malformed body or missing actor.

**Append-only:** every action is appended to `<run>/api_reviews.ndjson` as a hash-chained event (`prev` → `hash`). The run's decisions are never modified. Review history shows in `api_reviews` and in `/audit` → `history`.

### POST review: independent double review (`wasl-independent-review/1`)

**Status: built for the synthetic demo; verified by Go handler tests and the real-HTTP smoke (`scripts/api_smoke.py`, `independent_review_smoke`) only where that smoke has actually been run.** It does not change the pipeline, canonical case state, `out_log.ndjson`, `fhir_log.ndjson` or `reviewer_decisions.ndjson`; every response carries `canonical_unchanged: true`.

A body with `read` (`r1` | `r2` | `r3`) uses the protocol; a body without it is the legacy append-only event above. A legacy event never advances protocol state and stays labelled as not independently double-reviewed. Protocol bodies also need `expected_head` (a string; empty for the first read) and `claim_head` (the active 64-lowercase-hex lease head). `read` is a workflow slot, not a role. The server derives `question` (`link` or `evidence`) from the raised item and decision; a combined-kind item has one independent state machine per question. Only the verified owner of an active lease for that exact `(item, question, read)` can submit; a successful read consumes the lease.

| State | Accepted write | Result |
|---|---|---|
| `awaiting_r1` | ordinary `reviewer` (link) or `clinician` (evidence) submits `r1` at the empty head | `awaiting_r2` |
| `awaiting_r2` | a different actor (trimmed, case-insensitive) submits `r2` at the current head | `resolved_agreed` if the answers match, else `awaiting_r3` |
| `awaiting_r3` | a `senior_reviewer` (link) or `senior_clinician` (evidence), distinct from both initial actors, submits `r3` at the current head | `resolved_senior` |
| `resolved_agreed`, `resolved_senior` | nothing | 409 |

An agreement never opens `r3`. A disagreement alone opens it. `r3` records `resolution_reads` (the `r1` and `r2` event hashes). An item with a canonical final sign-off accepts no protocol write.

**Status codes:** 400 malformed input (unknown `read`, missing `expected_head`, decision that does not answer the question); 401 no valid token; 403 role not allowed for the slot or discipline, or the same normalised actor already read the question (two tokens bound to one actor are not independent); 404 unknown case or item; 409 wrong state, duplicate slot, premature `r3`, stale head or terminal state; 500 ledger append failure (memory is left unchanged). Validation and append happen under the server mutex, so two writers at one head get one 201 and one 409.

**Response:** 201 with `review` (the event, with additive fields `protocol`, `read`, `question`, `expected_head`, `claim_head`, `resolution_reads` on `r3`) and `review_state`. Case detail adds `api_review_states`, sorted by item then question: `item`, `question`, `state`, `head`, `next_read`, `may_submit`, `reads`, `disagreement`, `final_decision`, `resolution_basis`, `canonical_unchanged`, plus `claim_head`, `claim` and viewer-specific `claim_actions`. `claim` is `null` when unclaimed, released, consumed, frozen or terminal. Otherwise it carries `owner`, `role`, `read`, `claimed_at`, `heartbeat_at`, `expires_at` and `status` (`active` or `expired`). `claim_head` remains the last claim-event head even when `claim` is null. `claim_actions` has booleans for `claim`, `renew`, `release` and `takeover`; clients render those rather than inferring permission. `may_submit` is true only for the eligible owner of an unexpired active lease.

**Blinding:** while a question is `awaiting_r2`, any actor other than the `r1` author receives no `r1` decision, note, event hash or `api_reviews` entry, in detail or in POST responses. They see only that a first read was submitted. After `r2` both initial answers are visible. Auditor history is actor- and note-redacted but keeps `protocol`, `read`, `question`, `expected_head`, `claim_head`, `resolution_reads`, `prev` and `hash`; the web review page suppresses the embedded auditor history while any initial answer is concealed. Claim events receive the same viewer filtering, so their `prev` cannot disclose a concealed r1 hash.

**Senior authority:** `senior_reviewer` and `senior_clinician` hold the sign and queue-read capabilities of their discipline, and only they may submit `r3`; only the ordinary roles may submit `r1`/`r2`. In `scripts/demo.sh` the third reviewer and clinician tokens are the seniors. No header, browser-selected role or actor label grants authority.

**Mixed ledgers:** legacy, protocol and claim events share `api_reviews.ndjson` and its hash chain. Start-up replay applies the same transition and lease validators as live writes, so a chain-valid but impossible history fails closed (`review_ledger: invalid`, writes 500). Old protocol events without `claim_head` replay only while that question has no earlier claim history; after its first claim event every protocol read must carry valid fencing. Expiry during replay compares each event's `at` with the stored expiry, never the restart wall clock.

**Undo and contest:** targeting a protocol read or a `review_claim` event returns 409 in this version (dependent invalidation is not designed). Legacy compensation is unchanged.

**Web sign-in (cookie scope):** the review page uses a reviewer-supplied assigned API token. A Next.js server action verifies it with an authenticated API request, then stores it in an `HttpOnly; Secure; SameSite=Strict` cookie (`wasl_review_token`) scoped to the review path. Review reads and writes send that exact credential; the browser never supplies actor or role headers. Sign-out clears the cookie. This is the synthetic-demo path: the Basic-auth production-shaped package (`WASL_API_USERS_FILE`) serves the protocol through the API, but its passwords are not bearer tokens and do not work in the web form. Fixture mode requests no credential and persists nothing.

**Smoke coverage:** the real-HTTP smoke checks claim, competing-claim refusal, renew, release, re-claim and fenced r1/r2/r3 reads in hash-chain order, plus mid-state blinding, actor/note-redacted claim metadata and unchanged canonical data. It does not sleep for expiry; fake-clock Go tests cover the exact expiry boundary, takeover and lease races. Same-actor-via-two-tokens and concurrent duplicate writes are covered by Go handler tests, not the smoke.

**Limits:** tokens establish configured actor identities, not proof that separate humans (or real clinicians) participated. The ledger is a hash chain, not WORM storage. There is no OIDC, professional identity proof or live Malaffi. Canonical state is never rewritten by interactive review. Deployment must use HTTPS.

### POST review claims: five-minute leases

`POST /api/cases/{id}/review/claim` requires the assigned bearer token and a strict JSON body. Unknown, duplicate, oversized or trailing fields are rejected. Other methods return 405 with `Allow: POST`.

```json
{"item":"rv-123","question":"link","operation":"claim","expected_claim_head":""}
```

`question` is `link` or `evidence`; `operation` is `claim`, `renew`, `release` or `takeover`. `expected_claim_head` is empty before the question's first claim event and otherwise equals the current `api_review_states[].claim_head`. The server derives the current `read`, role and immutable actor from protocol state and the verified token. Callers cannot select a read, owner, role, actor or expiry.

Every success is **201**, because it appends a `type:"review_claim"` event to the shared hash chain. The response has `claim_event`, `review_state`, `api_review_states` and `note:"claim event appended; canonical decisions and signatures are unchanged"`. Claim events retain `kind`, `case`, `item`, `question`, `read`, `role`, `actor`, `action`, `at`, `claimed_at`, `heartbeat_at`, `expires_at`, `expected_claim_head`, `mode`, `prev` and `hash`.

| Operation | Rule | `claimed_at` | `heartbeat_at` | `expires_at` |
|---|---|---|---|---|
| `claim` | Acquire an unclaimed, released or consumed slot. An expired active lease requires takeover instead. | event `at` | event `at` | `at + 5m` |
| `renew` | Active owner only, strictly before expiry. | preserved | event `at` | `at + 5m` |
| `release` | Active owner only, strictly before expiry. | preserved | preserved | preserved |
| `takeover` | Any otherwise eligible viewer, including the former owner, only at or after expiry. Independence rules still apply. | event `at` | event `at` | `at + 5m` |

The server lease constant is **5 minutes**. A lease is active only when its latest event is not release/consumption and `now < expires_at`; at exactly `expires_at` it is expired. Expiry is lazy: no events are deleted and no background mutation occurs. The Next.js desk renews each active owned lease every **60 seconds** with the latest head, never overlaps a renewal for one question, and stops on release, consumption, expiry, ownership change, sign-out or unmount. A 401/403/409 stops that heartbeat; the UI never automatically takes over.

`expected_claim_head` fences claim operations only; `expected_head` fences protocol state only. A read sends both `expected_head` and `claim_head`. Its event timestamp must be before expiry and its verified actor/role must match the owner. Append and lease consumption happen in the same server critical section. Release and read consumption leave `claim_head` unchanged until the next claim operation.

Claim owner and expiry are visible in queue-authorized case projections. They are demo overlay metadata, not canonical ownership or clinical sign-offs. Sign-out does not silently release a lease: explicitly release it or let it expire safely.

Failures retain the normal envelope and, for conflicts, include `conflict`, `current_head`, `current_claim_head`, `review_state` and viewer-aware `api_review_states`. `conflict` is `claim_active`, `claim_required`, `claim_expired`, `claim_head`, `review_state` or the existing `head`.

| Condition | Status / conflict |
|---|---|
| Malformed body, missing/malformed head, unknown operation | 400 |
| Missing/invalid bearer | 401 |
| Wrong discipline/slot role, independence violation, non-owner renew/release/read | 403 |
| Unknown case/item or question not raised | 404 |
| Claim or takeover while unexpired | 409 `claim_active` |
| Claim instead of takeover after expiry; renew/release/read after expiry | 409 `claim_expired` |
| Read with no active lease | 409 `claim_required` |
| Stale `expected_claim_head` or read `claim_head` | 409 `claim_head` |
| Terminal/frozen/wrong read state | 409 `review_state` or existing `head` |
| Ledger validation or append failure | Existing 500/503 behaviour |

### POST actions (coordinator, demo mode)

Workflow A coordinator actions. Overlay metadata only: they never change a decision, the `out_log`, a signature, consent or the canonical `owner`, and they never dispatch a message.

**Identity:** a bearer token bound to the `coordinator` role (the only role holding the narrow `coordinate_case` permission); the event records that token's actor (1 to 64 characters). The web app sends `WASL_API_COORDINATOR_TOKEN` from its server and no identity headers.

**Body:** one JSON object, at most 8 KiB, no unknown or cross-action fields, no trailing data. `action` and `expected_head` are always required; `expected_head` is the case's current `api_overlay.head` (from `GET /api/cases/{id}` or the case summary).

| `action` | Fields | Bounds |
|---|---|---|
| `assign_owner` | `owner` | 1 to 64 characters |
| `snooze` | `until`, `reason` (optional) | RFC 3339, in the future and within 365 days; reason up to 200 characters |
| `contact_outcome` | `outcome`, `channel`, `note` (optional) | outcome `reached\|no_answer\|voicemail\|wrong_number\|declined\|appointment_agreed`; channel `phone\|sms\|email\|in_person`; note up to 1000 characters. Requires the case's `consent` to be `granted`. Records what happened; it does not send anything |
| `note` | `text` | 1 to 1000 characters, newlines allowed |

Text must be valid UTF-8, without surrounding whitespace or control characters.

**Response codes:**
- 201 with `event` (the ledger event) and the updated `api_overlay`;
- 400 for a bad body, field, bound or missing actor or `expected_head`;
- 403 for any role without `coordinate_case`;
- 404 for an unknown case;
- 405 for a method other than POST on `/actions` (with `Allow: POST`);
- 409 for a stale `expected_head` (`conflict: "head"`, with `current_head`) or a `contact_outcome` on a case whose consent is not granted (`conflict: "consent"`);
- 500 when the ledger cannot be appended (nothing is recorded) or the ledger failed validation at start-up (all ledger writes are refused until it is repaired).

**Concurrency:** the head of a case is its last decision signature until its first coordinator event, then the hash of its latest coordinator event. Two writers holding the same head: exactly one gets 201, the other 409 and must re-read. Different cases never conflict. The event is appended and synced to the ledger first; memory is updated only after that succeeds.

**`api_overlay`** (on the case summary and the detail): `head`, `actions` (count), `owner`, `snooze` (latest), `last_action_at`, `last_contact` (the latest contact outcome: `at`, `event`, `outcome`, `channel`; no actor or note; absent before the first one), `no_answer_streak` (consecutive `no_answer` outcomes ending the contact log), `ledger_status` (`ok` or `invalid`; when `invalid` the overlay may be partial, so the case list says so too, not only the detail); the detail also has `contact_outcomes`, `notes` and `canonical_owner`. `case.owner` stays the pipeline's value.

**Ledger and restart:** coordinator events share `<run>/api_reviews.ndjson` and its hash chain with review events (`type: "coordinator_action"`, with `kind`, `data`, `head`, per-case `seq`). On start-up the whole ledger is replayed and validated: JSON, chain, hash, case, role permission, head and sequence continuity, action fields (the same validation as the write path), consent for contact outcomes, and a complete final line. The first failure stops the replay, sets `healthz` `dependencies.review_ledger` to `invalid`, and makes `/review` and `/actions` return 500. The read endpoints keep working. Nothing is repaired or skipped silently.

**Fail-closed replay and re-materialised runs:** every coordinator event records the case head it followed. If the run is re-materialised so that a case's final decision (and so its head), its contact permission, or the case itself changes, the old events no longer fit: replay stops at the first such line, `review_ledger` becomes `invalid`, and all ledger writes (`/actions` and `/review`) return 500. Events before the failing line stay applied, so `api_overlay` can be partial; the case detail carries `review_ledger` (`status` ok or invalid, `detail`, `events_applied`) so a reader is never shown a partial overlay unannounced. Nothing is dropped or repaired automatically: an operator must archive or fix `api_reviews.ndjson` (or start a new run directory). A write whose outcome on disk is unknown (sync, truncate or close failed) also marks the ledger invalid until restart and replay; a clean write failure that was rolled back does not.

**Input rules:** JSON objects with a repeated key are rejected (request bodies and ledger lines); owner, note, reason and actor text may not contain control characters or Unicode format (Cf) characters such as zero-width space, BOM, soft hyphen or bidi overrides/isolates/marks. The one exception is ZWNJ (U+200C) and ZWJ (U+200D) directly after an Arabic-script letter, which Persian, Urdu and Kurdish names need; a ledger `seq` must be an exact integer.

## The demo cases

| Fixture | What it shows | Key fields |
|---|---|---|
| `evidence_closure` | FIT+ referral, Arabic-script identity, colonoscopy at another provider under a different record, trusted link, both readers agree with a verbatim quote, consent granted, closed, chain verified | `identities` (2 providers), `evidence[0].agree=true`, `label=adequate`, `quote`; `/audit` → `chain_verified=true` |
| `false_closure_trap` | A procedure exists but the documentation is incomplete, so Wasl **holds** instead of closing | `case.closed=false`, `state=inadequate`; `holds[0]` gives the report and rule |
| `review_hold` | Uncertain link and reader disagreement both went to double review (2 reads, signed); outreach was held while consent was unknown, released after the grant; then closed | `review_items`, `signoffs[].reads=[r1,r2]`, `decisions[].consent_log`: held → released, or (when no loop in the run had consent held then granted) a closed, signed loop whose `decisions[].outreach[].suppress` shows a message stopped by the consent hold or the second-identifier guard |
| `dr_exam_closure` | **Only in runs with DR loops.** A referable retinal screen (R2, R3S, R3A or M1) closed by an eye-clinic exam at another provider within the deadline. Both text readers agree on the screen and on the clinic note (no coded grade), the link is cross-provider and trusted, consent is granted, and no human was needed at any point (no review item, sign-off, override or earlier disagreement) | `case.dr` (`screen_label`, `rec`, `outcome`, `examined=true`, `within_deadline=true`); `evidence` (2 entries, `agree=true`, `reader_a_fields`/`reader_b_fields` with quotes); `links[0].hops≥1`; `/audit` → `chain_verified=true` |
| `agent_review` | **Only in cached agent runs (experimental).** A closed CRC case with consent granted whose link question was answered with a cited agent `same` recommendation; the human sign-off read `r1` plus `agent`, the final human `link` is `same` and `wrong=false`. The agent never signs. It never takes a case that already carries another fixture tag | `agent_investigations[0]` (`label=same`, `citations`, `human`); `agent_provenance.cached=true` |

Fixtures are chosen deterministically from the run (the first matching case by id: CRC for the first three, `diabetic_retinopathy` for `dr_exam_closure`), so the same run always gives the same cases. The three CRC keys and their selection rules are unchanged; a run without DR loops has no `dr_exam_closure` key. `agent_review` is picked after the others, only from cases without a fixture tag, so existing ids and single tags never move.

### Cached agent investigations (experimental, read-only)

`GET /api/cases/{id}` carries two extra fields. They add no endpoint and trigger no replay or model call.

- `agent_investigations`: `[]` for an ordinary run. Otherwise one entry per cached evidence-investigator recommendation that a human sign-off actually consumed: `{item, pair, label (same|different|uncertain), citations: [{tool, arg, field, value}], human: {link, reads, role}}`. `human` is the existing sign-off context (for example `reads=[r1, agent]`); the agent never signs and `signoffs` is unchanged.
- `agent_provenance`: `{cached: false, experimental: false}` for an ordinary run; otherwise `{cached: true, experimental: true, model, prompt_ver, schema_ver}` taken from the nested replay `provenance`. No paths, raw model messages or reasoning, salt, cache or other replay fields are exposed.

The run supplies `agent_recs.ndjson` and `agent_replay.json`: for a served run directory named `main` they sit one level up (`../`), otherwise next to the run files. Both must exist, otherwise the run is ordinary. A recommendation is attached only when its review item id is in the case's `out_log` queue for that loop, the unordered pair matches the item, the label equals the sign-off's `agent_read`, and the sign-off read `agent`. Twin questions on the same pair with another item id get nothing. Any malformed or mismatched input (bad NDJSON line, invalid record, malformed citations or provenance, a consumed recommendation that fails the join) yields no agent projection at all. `valid` in the recommendations is generation-time validation, not an API or audit verification, and nothing here is a clinical effectiveness claim.
