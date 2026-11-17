# Patient requests and staff tasks API (#179, #167)

Status labels follow the repository rule: **built** means implemented and covered by a Go test, **simulated** means synthetic stand-ins (no message, call or dispatch), **designed** means not yet built.

Code: `pipeline/patient_task.go` (shared task mechanism), `pipeline/api_teachback.go` (failed teach-back), `pipeline/api_patient_request.go` (explanation and contest), `pipeline/patient_contest_hold.go` (engine hold). Tests: `pipeline/patient_task_test.go`, `pipeline/api_teachback_test.go`, `pipeline/teachback_engine_test.go`, `pipeline/api_patient_request_test.go`, `pipeline/patient_contest_hold_test.go`.

## One mechanism

A verified, case-bound patient request (a synthetic patient link, see `API.md` "Synthetic patient self-service") becomes owned staff work in the same way for both request types. **Built.**

1. The patient action is an append-only event on the hash-chained `api_reviews.ndjson`. Its event hash is the receipt and the task identity. Retries with the same `request_id` replay the receipt and append nothing.
2. The event opens a `staffTask`: an owner role, an opening instant and a due date counted in working days (Monday to Friday, Gulf Standard Time). An open task is `overdue` from the due instant onwards.
3. Only a later signed staff event, from an RBAC-authorized credential, resolves the task. It records the actor, role, timestamp and outcome. Canonical decisions, escalations and signatures are never rewritten.
4. Patient views show `status`, `owner`, `due_at`, `overdue` and `resolved_at`, but never who resolved the task. Staff views add `resolved_by` and the consultation or review detail.

| Request | Opening event | Owner | Due | Resolved by | Served escalation while open |
|---|---|---|---|---|---|
| Failed teach-back | `patient_journey` `teach_back_answered` with a `misunderstood` or `needs_human` answer | `coordinator` (nurse navigator) | 2 working days | `teachback_callback` event with `understood_on_call` (`coordinator` or `clinician`) | `teachback_<state>_callback` |
| Patient contest | `patient_request` `contest_submitted` after a one-time code | `senior_clinician`, excluding every original reviewer of the case | 5 working days | `patient_request` `contest_resolved` with `decision_upheld` or `decision_revised` (a `senior_clinician` who did not sign the case) | `patient_contest_review` |

An explanation request (`explanation_requested`) is answered immediately with the case's provenance and opens no staff task.

## Case summary fields

`GET /api/cases` and `/api/cases/{id}` summaries carry these optional fields, which are absent when the case has no patient request. **Built.**

- `patient_tasks`: every patient-request task on the case, open tasks first (earliest due first), then resolved. Each row has `kind` (`teach_back_callback` or `patient_contest`), `id` (the receipt, or `engine/teachback_<state>_callback` for an engine-raised callback), `owner`, `status` (`open` or `resolved`), `escalation`, optional `opened_at`, `due_at`, `overdue` (open tasks only), `resolved_at` and `resolved_by`. Teach-back rows add `source` (`patient_link` or `engine`). Contest rows add `closure_held` and, once resolved, `outcome`.
- `escalations`: present with `patient_tasks`. It is the canonical list, without teach-back callback escalations that a coordinator resolved, plus open patient callbacks and `patient_contest_review` while a contest is open.
- `teach_back_callback`: the first open callback, otherwise the most recent one. Unchanged.
- `closed: false`, `canonical_closed` and `closure_hold: "patient_contest"` while a patient contest holds closure. Unchanged.

## Failed teach-back (#179)

- `POST /api/patient/journey` `{"kind": "teach_back_answered", "expected_head", "request_id", "data": {"question_id", "answer_id"}}` records one attempt. `answer_id` must be one the server offered. The correct answer uses `teach_back_completed`. Answers map to the existing parser states (`teachBackAnswers`). The response's `journey.teach_back` carries `understanding`, `attempts`, `status` (`completed`, `clarify` or `callback`) and the patient `callback` receipt. A second failed attempt while a callback is open opens no second task. Viewing the screen infers nothing. **Built.**
- `POST /api/cases/{id}/teach-back-callback` `{"callback": id, "outcome": "understood_on_call|another_explanation"}` appends a consultation. `understood_on_call` resolves the task and clears the matching served `teachback_<state>_callback` escalation. `another_explanation` records the consultation and keeps the task open. Generic contact outcomes are refused. **Built.**
- Engine: a `teachback-reply` parsed `partial` raises `teachback_clarify`. `misunderstood`, `needs_human` and `unparsed` raise `teachback_<state>_callback` until a completed `callback` communication exists. A prompt alone raises nothing (`TestEngineTeachBackStatesRaiseOnlyTheirOwnEscalation`). **Built.** No call is placed (**simulated**).

## Explanation and contest (#167)

- `POST /api/patient/verification` issues a simulated six-digit one-time code bound to the link, valid for 10 minutes and five attempts. The code is returned in the response and nothing is sent. **Simulated.**
- `GET /api/patient/requests` lists the patient's receipts and statuses and the current decision signature. `POST` `{"kind": "explanation_requested|contest_submitted", "request_id", "otp", "decision", "reason"}` requires a valid code. A contest requires a reason. An explanation response includes `explanation` with the decision, state, closing report, closed day and matched records (provider and role only). Forged, expired, wrong-case and unverified requests fail without case data and append nothing. **Built.**
- `POST /api/cases/{id}/patient-contest` `{"contest": receipt, "outcome": "decision_upheld|decision_revised"}`, for `senior_clinician` only. The original reviewers recorded on the contest cannot resolve it. **Built.**
- Served hold: while a contest is open, or resolved `decision_revised` while the questioned decision is still current, the API serves the case as not closed (`closure_hold`). The outcome hand-off and dashboard follow the same rule. **Built.**
- Engine hold: `LoopFacts.patient_contests` (`receipt`, `decision`, `outcome`) makes every programme's closure engine emit `contested` instead of an automatic closure, with a `patient_contest_hold` chain block and the `patient_contest_review` escalation. `decision_upheld` releases the hold, and the released decision is byte-identical to the questioned one. `decision_revised` holds while the engine would still emit the questioned decision. Without contests, facts and decisions are byte-identical to before. **Built** (`TestEngineHoldsAutomaticClosureWhileAPatientContestIsUnresolved`). The linker does not yet fill `patient_contests` from the API ledger, so in a served run the API hold is what applies. Feeding those events to the linker is **designed**.
- This workflow is separate from the staff undo/contest in [API-UNDO.md](API-UNDO.md). A staff contest opens no patient task and holds nothing (`TestStaffContestIsNotAPatientRequestAndHoldsNothing`).
