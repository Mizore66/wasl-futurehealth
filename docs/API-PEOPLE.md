# Person journey API

**Built:** authenticated, read-only cross-programme projection over a completed synthetic run. This API does not dispatch messages, schedule care, change consent, close cases, or make clinical recommendations.

## Identity boundary

`report7` materialisation writes `identity_clusters.json` atomically with mode `0600`. Its schema is `wasl-identity-clusters/1`. It contains only:

- the policy and SHA-256 hashes of the exact `fhir_log.ndjson` and `out_log.ndjson`;
- final trusted `LinkerCore.Clusters()` membership;
- each canonical referral loop's registration.

Clusters, registrations and cases are sorted. Surveillance children inherit `baseLoop(caseID)` membership. Names, birth dates, Emirates IDs, truth IDs, candidate pairs, scores and review answers are never written. Person membership never uses the synthetic display-name fixtures, case `link_decision` blocks, API-overlay reviews, or a singleton fallback.

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

The top-level schema is `wasl-person-journey/1`, with the ordinary API `meta` envelope and:

- `person`: opaque ID, anchor case, maximum canonical decision day (`as_of_day`), sorted cases and programmes, pseudonymous registrations, and canonical-artifact provenance including its SHA-256;
- `cases`: programme/case ordered projections containing only `case`, cluster-confirmed `identities` and `timeline`, trusted `links`, current `holds` from the latest canonical decision, `outreach`, and decision history including consent transitions, outreach and signatures; near-miss links and their associated records are excluded;
- per-case `contact`: canonical permission, separate demo-overlay status, combined contact-blocked display, and `canonical_unchanged: true`; contact is blocked unless canonical permission is `granted`, or when the overlay records denial/opt-out;
- `obligations`, `outreach_flags`, and one `next_action`.

The person projection excludes evidence, current full decisions, review items, sign-offs, API review events/states, coordinator notes, claims, audit history, review ledger data and review hashes. Full evidence and independently blinded review remain on the existing case routes. Responses are deep-copied and registration-pseudonymized; the raw identity artifact is never served.

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
