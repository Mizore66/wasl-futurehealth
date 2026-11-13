import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { CaseDetail, PatientJourneyEvent } from "@/lib/api/schema";
import { atWords } from "./words";

const KIND: Record<PatientJourneyEvent["kind"], string> = {
  teach_back_completed: "Teach-back completed",
  contact_permission_changed: "Contact permission changed",
  slot_selected: "Preferred simulated slot selected",
  preparation_changed: "Preparation progress changed",
};

function eventDetail(event: PatientJourneyEvent): string {
  switch (event.kind) {
    case "teach_back_completed":
      return "Understood the colonoscopy next step";
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
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id="patient-journey-h" className="m-0 text-section font-medium">Patient self-service timeline</h2>
        <HonestyLabel kind="simulated">Synthetic patient link</HonestyLabel>
      </div>
      <p className="m-0 mt-1 text-small text-muted">Server-confirmed patient activity. No message was sent and no appointment is reserved.</p>
      <dl className="m-0 mt-4 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2">
        <dt className="text-muted">Teach-back</dt><dd className="m-0">{patient.teach_back.status === "completed" ? "Completed" : "Pending"}</dd>
        <dt className="text-muted">Text permission</dt><dd className="m-0">{patient.contact_permission.status === "opted_out" ? "Stopped" : patient.contact_permission.status}</dd>
        <dt className="text-muted">Preferred time</dt><dd className="m-0 [overflow-wrap:anywhere]">{patient.slot_selection.status === "selected" ? `${patient.slot_selection.slot_id} · coordinator review needed` : "Not selected"}</dd>
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
