# Post-closure outcome hand-off API (issue #112)

**Status: built, simulated.** Synthetic data only. Nothing is dispatched: no SMS, WhatsApp, letter, referral or portal
message is sent, and no canonical closure, decision, outreach intent, recall or appointment is created or changed. The
code is `pipeline/api_outcome_handoff.go`; tests are `pipeline/api_outcome_handoff_test.go`, `web/tests/outcome-handoff.test.ts`,
`web/tests/outcome-handoff-staff.test.ts`, the `outcome:` tests in `web/e2e/patient.spec.ts` and
`web/e2e/outcome-handoff-staff-live.spec.ts`.

## Model

A case is **eligible** when its current canonical decision is closed, belongs to one of the seven patient programmes,
and no open patient contest holds it. Everything is an `outcome_handoff` event on the hash-chained review ledger
(`api_reviews.ndjson`), in this order:

| `kind` | Who | What it records |
|---|---|---|
| `content_approved` | `clinician` (`approve_outcome_handoff`) | The exact content version: canonical decision signature, programme, outcome state, a `next_step` chosen from the closed list `routine_screening\|surveillance\|specialist_follow_up\|discuss_with_doctor`, and the fixed safety-net notice `safety_net_v1`. `version` is the SHA-256 of that content. Wasl never derives a next step from outcome codes. |
| `release_recorded` | `coordinator` (`dispatch_outreach`) | A simulated release of one approved version, with the `gate` the server found from consent at that moment: `released` (a simulated hand-off intent; consent granted), `held_consent` (consent unknown) or `suppressed` (denied or opted out). |
| `referrer_acknowledged` | `referrer`, only the case's responsible referrer `referrer/<case provider>` (`acknowledge_outcome_handoff`); or the `clinician` whose actor `WASL_REFERRING_CLINICIANS` assigns to the case's provider (the credential that answers its returned referrals), recorded with `acting_for: "referrer/<case provider>"` | That referrer acknowledged that version. It never marks the patient informed. |
| `patient_acknowledged` | the case-bound patient link (`patient_link`), after a simulated one-time code | The patient read and acknowledged that version. |

Every acknowledgement and release names the approved `version`, so all parties refer to the same immutable content. A
new approval (a revised next step) creates a new version; acknowledgements of the old one stay in history.

### Patient status

`not_eligible` → `pending_approval` → `pending_release` → `held_consent` | `suppressed` | `released` → `acknowledged`.

- No approval: `pending_approval`. Approval but no release record: `pending_release`.
- A `held_consent` or `suppressed` release record stays in force until a new release finds consent granted. A
  withdrawal before release therefore records a hold or suppression, never a release, and both records remain.
- After a `released` record, current consent is still applied on every read: a later denial or opt-out shows
  `suppressed` and hides the content again (also after the patient acknowledged).
- Content (`outcome: follow_up_complete`, `next_step`, `safety_net`) is shown to the patient only while the status is
  `released`/`acknowledged` **and** the link has verified its one-time code (the second identifier) within the last 15
  minutes. The patient view never includes the decision signature, programme or outcome code.

## Staff: `GET|POST /api/cases/{id}/outcome-handoff`

Staff bearer token. Read: `coordinator`, `clinician` and the responsible `referrer` (another referrer gets 403 and no
content). Referrer tokens cannot read the worklist. Other roles (for example `senior_clinician`) get 403
`read permission denied: …` naming which roles act on the hand-off; read and write share this route gate.

`GET` returns `{handoff}`: `status`, `delivery: "simulated"`, `consent`, `responsible_referrer`, `referrer_status`
(`pending|acknowledged`), `patient_informed` (true only after the patient's own acknowledgement), `draft` (current
decision, programme, outcome and safety net), `next_steps`, and once approved `version`, `approved_at`, `approved_by`,
`content`, `release` (`gate`, `at`, `actor`, `event`) and `history[]` (`kind`, `gate` on releases, `version`, `role`,
`actor`, `at`, `event`).

`POST` bodies (8 KiB limit, unknown fields rejected):

| Body | Rules |
|---|---|
| `{kind: "content_approved", decision, next_step, version}` | Clinician only. `decision` must be the case's current decision signature; the case must be eligible. `version` is the approval the clinician saw (`""` for none): a stale one returns 409 instead of replacing a newer approval. The same content again returns 200 `replayed: true`. |
| `{kind: "release_recorded", version}` | Coordinator only. 409 without an approval or for a version that is not the current approval. The same gate as the latest release for that version returns 200 `replayed: true` and writes nothing; a changed consent writes a new record. |
| `{kind: "referrer_acknowledged", version}` | The responsible referrer only. 409 without an approval or for a stale version; a repeat returns 200 `replayed: true`. |

New records return 201 with the stored `event`. Refusals (400/403/409) write nothing.

## Staff web workflow

**Built (simulated), minimal UI.** The case page (`/app/patients/[id]`) renders `OutcomeHandoffCard`
(`web/components/case/outcome-handoff-card.tsx`) from `outcome_handoff` on the case detail: patient status, responsible
referrer status (read-only; the web never records a referrer acknowledgement), patient informed, current consent, the
current decision (`draft`), the approved next step with approver, time and version, the latest release record and the
history. Its staff steps come from `web/components/case/outcome-handoff-review.tsx` and call the server actions in
`web/lib/api/outcome-handoff-actions.ts`:

| Step | Shown when | Credential |
|---|---|---|
| Approve content (`content_approved`) | live API, eligible case, viewer signed in | The viewer's own reviewer-session token (`reviewerToken()`), like a disposition or a patient-contest resolution. No server-held credential is used; the API allows only `clinician`. A signed-out viewer gets the personal sign-in instead. The form sends the case's current `draft.decision`, the next step the clinician picks from `next_steps` (no default) and the approval `version` shown on the page (`""` for none). |
| Record simulated release (`release_recorded`) | live API, eligible case, an approved `version` exists | The server-side coordinator credential (`WASL_API_COORDINATOR_TOKEN`), like every other coordinator action on the case page. The server records the consent gate; a repeat after a hold or suppression re-checks consent. |

Both actions refuse any client-supplied `credential`, `token`, `role` or `actor` and send nothing, and show a refusal
with the server's own `error` text (for example a 403 for a non-clinician session or a 409 for a stale approval). A
`replayed: true` answer is reported as "nothing new was recorded". Fixtures mode and ineligible cases show no
controls. Tests: `web/tests/outcome-handoff-staff.test.ts` (credential use, identity refusal, refusals, card states) and
`web/e2e/outcome-handoff-staff-live.spec.ts` (browser, against a stateful mock of this handler, not the real Go API).

## Patient: `GET|POST /api/patient/outcome`

Patient-link bearer token (proxied by Next.js at `/patient/outcome-handoff` from the `__Host-wasl_patient_link`
cookie). Invalid links return 401, expired links 410, both without content. The case is the link's case; no case,
decision or role is taken from the request.

- `GET` returns `{handoff}` with `status`, `delivery`, `verified` (on `released`/`acknowledged`) and, only when
  verified, `version`, `approved_at`, `content` and `acknowledged_at`.
- `POST {otp}` verifies a code issued by `POST /api/patient/verification` (simulated: the code is returned, not sent).
  It writes nothing. 409 when nothing is releasable now; 403 for a wrong or expired code.
- `POST {version, otp}` acknowledges the released version. `otp` may be empty while the link's verification is
  current. Repeats return 200 `replayed: true`; a held, suppressed, unreleased or stale version returns 409.

## Replay

`replayOutcomeHandoff` rejects unknown fields, unknown cases, versions that do not hash to the named decision and next
step, approvals by roles without `approve_outcome_handoff`, releases by roles without `dispatch_outreach` or with an
unknown gate, a release repeating the previous gate for the same version, referrer acknowledgements by anyone but the
responsible referrer, malformed patient acknowledgement identities, any acknowledgement or release of unapproved content,
and duplicate acknowledgements. Replay validates the history as written; it does not re-judge recorded gates against
today's consent. Ledgers written before release records existed still replay: their patient acknowledgements remain
valid history, and an approval that no patient acknowledged waits as `pending_release` until a coordinator records a
release (`TestOutcomeHandoffLegacyLedgerWithoutReleaseRecordStillReplays`).

## Not built

- A visual design for the staff steps: the case-page controls are a functional minimum.
- A web control for the referrer acknowledgement: it remains an API call with the responsible referrer's token; the case
  page shows its state read-only.
- Any real delivery channel. Clinical outcome details are never placed in unauthenticated SMS/WhatsApp content.
