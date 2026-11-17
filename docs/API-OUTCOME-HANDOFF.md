# Post-closure outcome hand-off API (issue #112)

**Status: built, simulated.** Synthetic data only. Nothing is dispatched: no SMS, WhatsApp, letter, referral or portal
message is sent, and no canonical closure, decision, outreach intent, recall or appointment is created or changed. The
code is `pipeline/api_outcome_handoff.go`; tests are `pipeline/api_outcome_handoff_test.go`, `web/tests/outcome-handoff.test.ts`
and the `outcome:` tests in `web/e2e/patient.spec.ts`.

## Model

A case is **eligible** when its current canonical decision is closed, belongs to one of the seven patient programmes,
and no open patient contest holds it. Everything is an `outcome_handoff` event on the hash-chained review ledger
(`api_reviews.ndjson`), in this order:

| `kind` | Who | What it records |
|---|---|---|
| `content_approved` | `clinician` (`approve_outcome_handoff`) | The exact content version: canonical decision signature, programme, outcome state, a `next_step` chosen from the closed list `routine_screening\|surveillance\|specialist_follow_up\|discuss_with_doctor`, and the fixed safety-net notice `safety_net_v1`. `version` is the SHA-256 of that content. Wasl never derives a next step from outcome codes. |
| `release_recorded` | `coordinator` (`dispatch_outreach`) | A simulated release of one approved version, with the `gate` the server found from consent at that moment: `released` (a simulated hand-off intent; consent granted), `held_consent` (consent unknown) or `suppressed` (denied or opted out). |
| `referrer_acknowledged` | `referrer`, only the case's responsible referrer `referrer/<case provider>` (`acknowledge_outcome_handoff`) | That referrer acknowledged that version. It never marks the patient informed. |
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
content). Referrer tokens cannot read the worklist.

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

- A staff UI for approving and releasing: the case record shows the hand-off state and history (read-only); approval
  and release are API calls.
- Any real delivery channel. Clinical outcome details are never placed in unauthenticated SMS/WhatsApp content.
