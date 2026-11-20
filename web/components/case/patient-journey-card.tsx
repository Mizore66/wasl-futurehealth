import { HONESTY_PLAIN, HonestyLabel } from "@/components/wasl/honesty-label";
import type { ReactNode } from "react";
import type { CaseDetail, PatientJourneyEvent } from "@/lib/api/schema";
import { staffDueTime } from "@/lib/domain/staff-time";
import { roleWords } from "@/lib/auth/session-persona";
import { atWords } from "./words";

const KIND: Record<PatientJourneyEvent["kind"], string> = {
  teach_back_completed: "Teach-back completed",
  teach_back_answered: "Teach-back not yet understood",
  contact_permission_changed: "Contact permission changed",
  slot_selected: "Preferred simulated slot selected",
  preparation_changed: "Preparation progress changed",
};

const TEACH_BACK: Record<NonNullable<CaseDetail["api_patient"]>["teach_back"]["status"], string> = {
  pending: "Pending",
  completed: "Completed",
  clarify: "Clarification needed",
  callback: "Nurse callback needed",
};

function eventDetail(event: PatientJourneyEvent): string {
  switch (event.kind) {
    case "teach_back_completed":
      return "Understood the colonoscopy next step";
    case "teach_back_answered":
      return event.data.answer_id === "not_sure" ? "Answered: not sure" : "Answered: another blood test";
    case "contact_permission_changed":
      return event.data.permission === "opted_out" ? "Stopped text message permission" : event.data.permission === "granted" ? "Granted text message permission" : "Declined text message permission";
    case "slot_selected":
      return event.data.slot_id ?? "Offered slot";
    case "preparation_changed":
      return `${event.data.item_id === "escort_home" ? "Escort home" : "Clear fluids and preparation"}: ${event.data.completed ? "completed" : "not completed"}`;
  }
}

/** Patient overlay only: this is not the canonical FHIR timeline, consent decision or booking record. */
export function PatientJourneyCard({ detail }: { detail: CaseDetail }) {
  const patient = detail.api_patient;
  if (!patient) return null;
  const events = detail.api_reviews.filter((event): event is Extract<CaseDetail["api_reviews"][number], { ledger: "patient" }> => event.ledger === "patient");
  return (
    <section id="patient-journey" aria-labelledby="patient-journey-h" className="min-w-0 scroll-mt-4 rounded-card border border-line bg-surface p-4 md:p-pad">
      <h2 id="patient-journey-h" className="m-0 text-section font-medium">Patient self-service timeline</h2>
      <p className="m-0 mt-1 text-small text-muted">
        <HonestyLabel kind="simulated" className={HONESTY_PLAIN}>Synthetic patient link.</HonestyLabel> Server-confirmed patient activity. No message was sent and no appointment is reserved.
      </p>
      <dl className="m-0 mt-4 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2">
        <dt className="text-muted">Teach-back</dt><dd className="m-0">{TEACH_BACK[patient.teach_back.status]}{patient.teach_back.attempts ? ` · ${patient.teach_back.attempts} attempt${patient.teach_back.attempts === 1 ? "" : "s"}` : ""}</dd>
        <dt className="text-muted">Text permission</dt><dd className="m-0">{patient.contact_permission.status === "opted_out" ? "Stopped" : patient.contact_permission.status}</dd>
        <dt className="text-muted">Preferred time</dt><dd className="m-0 [overflow-wrap:anywhere]">{patient.slot_selection.status === "selected" ? `${patient.slot_selection.slot_id}, coordinator review needed` : "Not selected"}</dd>
        <dt className="text-muted">Preparation</dt><dd className="m-0">{patient.preparation.completed} of {patient.preparation.total} complete</dd>
      </dl>
      <h3 className="m-0 mt-5 text-body font-medium">Append-only patient events</h3>
      {events.length === 0 ? (
        <p className="m-0 mt-2 text-muted">No patient events recorded.</p>
      ) : (
        <ol className="m-0 mt-2 list-none p-0" aria-label="Patient self-service events">
          {events.map((event) => (
            <li key={event.hash} className="border-t border-rule py-3">
              <p className="m-0 font-medium">{KIND[event.kind]}</p>
              <p className="m-0 mt-1">{eventDetail(event)}</p>
              <p className="m-0 mt-1 text-small text-muted">Patient link · {atWords(event.at)} · event {event.hash.slice(0, 8)}</p>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

export function requestState(row: NonNullable<CaseDetail["patient_requests"]>[number]): string {
  if (row.kind === "explanation_requested") return "Explanation returned to the patient";
  if (row.status === "resolved") {
    const by = row.resolved_by ? ` by ${row.resolved_by}` : "";
    const at = row.resolved_at ? ` ${atWords(row.resolved_at)}` : "";
    return `Resolved: ${row.outcome === "decision_revised" ? "decision to be revised" : "decision upheld"}${by}${at}`;
  }
  const due = row.due_at ? ` review ${row.overdue ? "overdue since" : "due"} ${staffDueTime(row.due_at)}` : " review";
  return `${row.overdue ? "Overdue" : "Pending"} · ${roleWords(row.owner ?? "senior_clinician").toLowerCase()}${due}`;
}

/**
 * Live resolution slots for open patient contests, built server-side by `patient-contest-review.tsx` from the viewer's
 * own reviewer session. `identity` is the sign-in prompt or the signed-in identity; `controls` exists only when someone
 * is signed in. Snapshots pass `live: false` and get no controls. The shared web credential never resolves a contest.
 */
export type PatientContestReview = { live: boolean; identity?: ReactNode; controls?: (receipt: string, excluded: string[]) => ReactNode };

/** Patient-submitted explanation requests and contests; distinct from staff undo/contest compensation events. */
export function PatientRequestsCard({ detail, review }: { detail: CaseDetail; review?: PatientContestReview }) {
  const requests = detail.patient_requests ?? [];
  if (requests.length === 0) return null;
  const anyOpen = requests.some((row) => row.kind === "contest_submitted" && row.status === "open");
  return (
    <section id="patient-requests" aria-labelledby="patient-requests-h" className="min-w-0 scroll-mt-4 rounded-card border border-line bg-surface p-4 md:p-pad">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id="patient-requests-h" className="m-0 text-section font-medium">Patient explanation and contest requests</h2>
        <HonestyLabel kind="simulated" className={HONESTY_PLAIN}>The patient&apos;s one-time code is simulated.</HonestyLabel>
      </div>
      {requests.some((row) => row.closure_held) && <p className="m-0 mt-1 text-small">Closure is held. An unresolved patient contest is open on the current decision.</p>}
      {anyOpen && review?.live && review.identity}
      <ol className="m-0 mt-2 list-none p-0" aria-label="Patient requests">
        {requests.map((row) => (
          <li key={row.receipt} className="border-t border-rule py-3">
            <p className="m-0 font-medium">{row.kind === "contest_submitted" ? "Contest" : "Explanation request"}</p>
            <p className="m-0 mt-1">{requestState(row)}</p>
            {row.kind === "contest_submitted" && row.reason && <p className="m-0 mt-1 [overflow-wrap:anywhere]">Patient&apos;s reason: {row.reason}</p>}
            <p className="m-0 mt-1 text-small text-muted">
              Submitted {atWords(row.submitted_at)} · receipt {row.receipt.slice(0, 8)} · questioned decision {row.decision.slice(0, 8)}
            </p>
            {row.resolution_reason && <p className="m-0 mt-1 [overflow-wrap:anywhere]">Reviewer&apos;s reason: {row.resolution_reason}</p>}
            {row.kind === "contest_submitted" && row.status === "open" && review?.live && review.controls?.(row.receipt, row.excluded_reviewers ?? [])}
          </li>
        ))}
      </ol>
      {anyOpen && !review?.live && <p className="m-0 mt-2 text-small text-muted">Resolving a contest needs the live API and a signed-in senior clinician. This snapshot is read-only.</p>}
    </section>
  );
}
