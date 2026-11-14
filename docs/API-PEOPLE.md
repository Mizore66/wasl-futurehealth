# Person journey API

**Built:** authenticated, read-only cross-programme projection over a completed synthetic run. This API does not dispatch messages, schedule care, change consent, close cases, or make clinical recommendations.

## Identity boundary

Canonical `report7` materialisation writes `identity_clusters.json` atomically with mode `0600`, using **`wasl-identity-clusters/2`**. The reader also accepts the original **active-only `/1`** contract with its original closed wire shape; `/1` does not silently accept `/2` fields. Regenerate temporal artifacts from the unchanged immutable canonical source, not guessed rows or edited signed decisions.

`/2` contains only:

- the policy and SHA-256 hashes of the exact `fhir_log.ndjson` and `out_log.ndjson`;
- unchanged final trusted `LinkerCore.Clusters()` membership;
- a complete, sorted `catalog` for every decision-bearing actual OUT case, including inactive roots and stored surveillance children;
- each row's `case_id`, source-validated `lifecycle: active|retained`, explicit person `participation: current|reference_only`, signed canonical `parent_case_id` where applicable, and **separate `current` and `history` association slots**.

A resolved association has `status: resolved`, the canonical registration, provider-qualified `request_id`, accepted `request_version`, normalized `request_sha256`, `request_seq`, and the accepted explicit-subject `binding_seq` (which can be carried through a proven patientless revision). An unavailable association has only `status: unavailable` and a bounded reason: `inactive_request`, `conflicting_request`, `unbound_request`, `unknown_programme`, `unproven_binding`, or (only for a validated detached active child) `detached_child`. It contains no fake registration or selected conflicted body. Every history entry additionally binds `decision_id`, same-case `decision_sig`, the replay-captured `facts_seq`, and `transition` (the actual triggering accepted request association). Child entries also bind `origin_decision_id` and `parent_decision_id` to the producing clinical decision and its signed immediate parent. For retirement, `association` remains the child's last clinical-origin association; `transition` and `facts_seq` retain the real later cause, even when its subject differs. Neither the tombstone nor its trigger is rewritten.

Replay captures evidence at the canonical facts/output boundary before later inputs overwrite or retire it. Source verification invokes the **existing canonical request fold** without scoring or union operations; it does not introduce a second revision policy. A same-version conflict invalidates that version's apparent first-arrival historical winner in either delivery order. Previously accepted, distinct lower-version context can remain a historical reference. A new explicit subject can establish current root work without taking the former subject's history. A stored child does not inherit the final root's subject: its producing accepted context and the contexts of its signed immediate-parent lineage must all belong to the final selected root's **final trusted canonical cluster**. Equivalent registrations within that cluster are valid continuity, not different people. Without that continuity, the child is positively validated `reference_only`, with its actual latest OUT lifecycle still `active` when applicable. It contributes no current person cases, obligations, outreach flags or actions for either person; its proven subject retains non-actionable references. This does not change clinical eligibility, stored decisions or `Final()`. Historical membership never activates care.

Clusters and registrations are sorted and nonoverlapping. Every actual case and every historical decision must be present exactly once; omission is corruption, not unavailability. Source provenance, lifecycle, canonical parent, current active binding, signatures, and both exact-log hashes are checked. The exporter independently compares **all** retained OUT decisions with replay, not just active `Final()`. Names, birth dates, Emirates IDs, truth IDs, candidate pairs, scores and review answers are never written. Person membership never uses display fixtures, raw-last-arrival request maps, near links, API-overlay reviews, or a singleton fallback. The active linker index and clinical eligibility reducers remain active-only.

The opaque ID is:

```text
"person_" + sha256("wasl-person-v1\0" + sorted registrations joined by "\0")
```

It therefore does not expose the union-find root and does not change merely because the linker chooses another root. Registration values are served only as deterministic `reg_<64 hex>` pseudonyms. These unkeyed pseudonyms support equality joins; they are not semantic anonymity.

Person routes return `503 person_projection_unavailable` for a missing, malformed, overlapping, stale-hash, unknown-case, or incomplete artifact. They also refuse a nonzero `live_vs_replay_final_mismatch`. No partial journey is served. Existing case routes do not require this new artifact and continue to support older runs.

## Routes and authorization

| Route | Result |
|---|---|
| `GET /api/people?case_id=<url-encoded case ID>` | Journey for the trusted cluster containing the case |
| `GET /api/people/{person_id}` | Journey by opaque ID |

There is deliberately no person-list route. The browser flow calls `/app/people?case_id=...`, then redirects to `/app/people/{person_id}`. The Next.js server uses only its server-held `web_reviewer` bearer credential. It neither exposes that credential nor accepts browser role/actor headers.

These routes require `ActReadQueue`. Missing/invalid credentials receive `401`; authenticated principals without that permission, including auditors, receive `403`. Non-GET methods receive `405` and `Allow: GET`. Missing/unsupported query parameters and malformed person IDs receive `400`; unknown cases or people receive `404`.

## Response

Legacy active-only responses keep `wasl-person-journey/1` and the ordinary API `meta` envelope:

- `person`: opaque ID, anchor case, maximum canonical decision day (`as_of_day`), sorted cases and programmes, pseudonymous registrations, and canonical-artifact provenance including its SHA-256;
- `cases`: programme/case ordered projections containing only `case`, cluster-confirmed `identities` and `timeline`, trusted `links`, current `holds` from the latest canonical decision, `outreach`, and decision history including consent transitions, outreach and signatures; near-miss links and their associated records are excluded;
- per-case `contact`: canonical permission, separate demo-overlay status, combined contact-blocked display, and `canonical_unchanged: true`; contact is blocked unless canonical permission is `granted`, or when the overlay records denial/opt-out;
- `obligations`, `outreach_flags`, and one `next_action`.

The person projection excludes evidence, current full decisions, review items, sign-offs, API review events/states, coordinator notes, claims, audit history, review ledger data and review hashes. Full evidence and independently blinded review remain on the existing case routes. Responses are deep-copied and registration-pseudonymized; the raw identity artifact is never served.

### Retained `/2` response

A `/2` artifact produces **`wasl-person-journey/2`**. `cases` contains only active current participation with `lifecycle: active` and `history_availability: reference_only`. Current detail is latest-only and is omitted when that old signed output cannot be associated with the current subject; it is never copied from another subject. Contact is blocked when that subject context is unavailable. Full original decisions and source timelines remain on the existing authorized case/audit routes.

`retained_references` is separate from `cases`, obligations, duplicate/overlap action inputs and next-action ranking. Each reference has `case_id`, optional canonical parent, case lifecycle, `availability: reference_only`, `history_access: authorized_case_audit`, `actionable: false`, and only the programmes/decision references proven for **this as-of subject**. Outreach summaries preserve their actual `released` and hold status with `participation: historical`; they do not expose additional downstream registration aliases. The original complete outreach object stays unchanged in authorized case history. These summaries are not dispatch/delivery evidence and are not newly cancelled messages.

A retained-only person can have `cases: []`, no current obligations/flags, `next_action.kind: none`, and proven history references. `person.case_ids` covers current cases and scoped references; an old case can be a non-actionable reference for its former subject while its current association belongs to a different person. No old subject's history is relabelled as the new subject's journey. Unknown/unselected programme is not inserted into `person.programmes`.

A reference-only case lookup with no unique proven historical subject returns **409**, schema **`wasl-person-association-unavailable/2`**, with `case_id`, the truthful OUT `lifecycle: active|retained`, bounded `association: {status: unavailable, reason}`, `history_access: authorized_case_audit`, and `actionable: false`. It has **no person**. Multiple historical subjects use `multiple_historical_subjects`; missing binding, conflict and unknown programme retain their bounded source reasons. This response is case-local: unrelated valid people still resolve. A malformed active mapping, missing active **or inactive** catalog row, forged provenance, bad parent, stale hash or malformed artifact still fails closed with the existing person-only 503, never a partial supposedly complete journey.

Case list/detail history uses a separate retained lifecycle variant: `state: inactive`, `inactive: true`, `lifecycle: retained`, `actionable: false`, request lifecycle metadata, and `programme_availability: selected|unavailable`. Only this non-actionable variant permits an empty current programme. Original active programme/state enums are unchanged. The original tombstone, signatures and review history are not rewritten.

### Materialized counts and administrative rights

`report7.materialized_catalog` names its own `wasl-materialized-catalog/1` counts: `retained_cases` (all actual OUT case IDs), `active_cases` (eligible latest non-inactive decisions), and `retained_live_replay_mismatch` (must be zero after the complete retained comparison). Preflight checks these against actual OUT independently. Native `report7.loops` and `loops_by_programme` remain **clinical truth/evaluation denominators**, checked for internal consistency, not replaced by retained/active catalog counts. The API dashboard keeps its active-case meaning and health keeps its materialized-case meaning. Legacy declaration checks remain supported.

Retirement does not prohibit every coordinator append. Existing authorized historical/admin notes, consent denial/withdrawal/opt-out under the existing rights/head rules, and ownership/version-checked cancellation of an existing simulated reservation remain permitted. None activates care or dispatches. New claims/clinical reviews, new booking acquisition/rescheduling, outreach and actionable obligations remain ineligible. No blanket append/history guard is added. The separately documented coordinator overlay re-materialization fail-closed contract in `API.md` is unchanged; this read-model upgrade does not repair or bypass an invalid ledger.

## Obligations

Obligations copy existing canonical clocks; they do not create intervals or a “due soon” threshold.

| Programme | Sources |
|---|---|
| CRC / cervical | Unresolved referral day + canonical deadline |
| Lung nodule | Open `Lung.WindowEnd`; tissue due while awaiting tissue; due/overdue `Lung.Surveillance` entries and their `Next` chain |
| Diabetic retinopathy | Open `DR.DeadlineDay`; unseen `DR.Next.DueDay`, including resolved-loop recall |
| Breast | Open `Breast.DeadlineDay`; treatment referral due day as `informational` only |
| LDCT | Annual `LDCT.DueDay` as scheduled; unresolved action-loop due day and distinct hard deadline separately |
| IFHAS | Open `Ifhas.DeadlineDay` |

Statuses are `open`, `scheduled`, `overdue`, or `informational`. Overdue uses canonical flags/states. Surveillance projections are deduplicated by child loop, round and due day. Ordering is due day, fixed programme order, case ID, then kind.

## Outreach review

History is folded by `(case_id, event, kind)` in decision order. Each intent retains its first decision, latest decision and signature, latest unchanged canonical outreach object, and lifecycle:

- `released`: a release was recorded, **not** that a provider dispatched or delivered a message;
- `held`: a canonical guard is holding the intent;
- `withheld`: consent or another canonical rule withholds it; not actionable;
- `recorded`: a non-released intent without a suppression code.

A `duplicate` group has the same `(kind,event)` on at least two distinct cases. One `overlap` group contains current intents spanning at least two programmes. Repetition across decisions of one case does not itself create a duplicate. At least two `released`, `held`, or `recorded` intents make a group actionable. Flags never delete, rewrite, cancel or suppress history, and there are no send/dismiss controls.

## Next coordinator action

The rule identifier is `wasl-person-next-action/1`. Candidates are:

1. Each actionable outreach flag, deterministically anchored to a member case.
2. Unresolved cases with canonical coordinator-actionable drop-off: `never_notified`, `no_acknowledgement`, `not_booked`, `referral_rejected`, `referral_unanswered`, `no_show`, `no_followup_ct`, `no_eye_exam`, `ungradable_no_repeat`, `technical_repeat_not_done`, `incomplete_scan_not_completed`, `followup_ldct_not_done`, or `no_followup_documented`.
3. Unseen overdue canonical DR next-due recall.

Clinical holds, unlinked records, evidence waits and specialist decisions are not coordinator actions. Patient-contact follow-up without canonical `granted` permission becomes `review_contact_permission`, never a recommendation to contact. Provider-only rejected/unanswered referral follow-up remains administrative. The separate overlay never overrides canonical consent for ranking.

Candidates sort by:

1. canonical `priority == "urgent"` first;
2. canonical overdue first;
3. earliest canonical hard deadline, with missing deadlines last;
4. `review_duplicate_outreach`, `review_contact_permission`, then `programme_follow_up`;
5. case ID;
6. outreach flag ID.

Flag urgency, overdue and deadline aggregate conservatively from member cases: any urgent, any overdue, earliest deadline. Reason codes explain the choice in machine-readable form; the UI translates them to plain language and displays priority, deadline, source case and rule. If there is no candidate, `kind` is `none`, priority is `routine`, and the sole reason is `no_open_coordinator_action`.

## Snapshots

`scripts/demo.sh snapshot` fetches one person journey through the queue-authorized server credential and projects it through `scripts/redacted_snapshot.py person`. The default-deny person allowlist rejects raw registrations, canaries and unknown structures, excludes clinical review details, and writes `docs/api-samples/person_journey.json`. `web/scripts/sync-samples.mjs` copies that public synthetic fixture without modification.
