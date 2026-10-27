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
| GET | `/api/dashboard` | KPI and SLA, `states`, `dropoff`, `review` counts, `by_programme`, per-provider and per-origin tables (small-cell suppressed), headline `metrics` (31-day KPI, closure P/R, consent invariant, forged, provenance, exactly-once), `fixtures`, `capacity` (simulated booking: condition adequate, misallocated or short_overall, slots per case by region, pooling_relevant, summary, `levers`: pathway and prep-risk counts) |
| GET | `/api/fixtures` | The three demo cases (summaries) |
| GET | `/api/cases` | Filterable case summaries. Query: `programme` (crc\|cervical), `state`, `dropoff`, `owner`, `provider`, `consent`, `review` (none\|open\|signed), `overdue` (true\|false), `fixture=1`, `q` (id substring), `limit` (≤500, default 100), `offset` |
| GET | `/api/cases/{id}` | Full case: `case`, `identities` (linked registrations), `timeline` (every linked FHIR/HL7 event, no free text), `links`, `evidence` (both readers, per-field values, quote, model), `holds` (why it didn't close), `outreach`, `decisions` (history, with `consent_log`), `review_items`, `signoffs`, `api_reviews`, `current`, `booking` (simulated suggestion and capacity condition; `status`: suggested, no_slot_within_horizon, or not_applicable; from `wasl-booking/2` also `pathway` {route: direct\|clinic, reasons, checklist_notes}, `prep_risk` {score, band: low\|high, factors, suggested_intervention, requires_clinician_order: true} and synthetic `app9` fields) |
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

### POST review (demo mode)

**Headers:** `X-Wasl-Role` (`coordinator` | `clinician` | `reviewer` | `auditor`) and `X-Wasl-Actor` (who is signing).

**Body:** `{"item": "<review item id from review_items>", "decision": "same|different|adequate|inadequate|confirm_contact", "note": "…"}`

**RBAC:**
- `same`/`different` need a role allowed to sign links (reviewer).
- `adequate`/`inadequate` need one allowed to sign evidence (clinician).
- `confirm_contact` needs one allowed to record consent (coordinator, clinician).

**Response codes:**
- 201 with the appended event;
- 403 for a role that isn't allowed;
- 404 for an item that wasn't raised for this case;
- 400 for a malformed body or missing actor.

**Append-only:** every action is appended to `<run>/api_reviews.ndjson` as a hash-chained event (`prev` → `hash`). The run's decisions are never modified. Review history shows in `api_reviews` and in `/audit` → `history`.

## The three demo cases

| Fixture | What it shows | Key fields |
|---|---|---|
| `evidence_closure` | FIT+ referral, Arabic-script identity, colonoscopy at another provider under a different record, trusted link, both readers agree with a verbatim quote, consent granted, closed, chain verified | `identities` (2 providers), `evidence[0].agree=true`, `label=adequate`, `quote`; `/audit` → `chain_verified=true` |
| `false_closure_trap` | A procedure exists but the documentation is incomplete, so Wasl **holds** instead of closing | `case.closed=false`, `state=inadequate`; `holds[0]` gives the report and rule |
| `review_hold` | Uncertain link and reader disagreement both went to double review (2 reads, signed); outreach was held while consent was unknown, released after the grant; then closed | `review_items`, `signoffs[].reads=[r1,r2]`, `decisions[].consent_log`: held → released |

Fixtures are chosen deterministically from the run (the first matching CRC case by id), so the same run always gives the same three cases.
