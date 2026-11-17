import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { OutcomeHandoff } from "@/lib/api/schema";
import { atWords } from "./words";

const PATIENT: Record<OutcomeHandoff["status"], string> = {
  not_eligible: "Not eligible: the case is not closed, or a patient contest holds it open",
  pending_approval: "Pending clinician approval of the outcome, next step and safety-net content",
  pending_release: "Approved, not yet released: a coordinator records the simulated release, which checks consent",
  held_consent: "Approved, held: contact permission was unknown at release; a new release is needed once it is granted",
  suppressed: "Approved, suppressed: the patient denied or withdrew contact permission",
  released: "Released to the patient link, not yet acknowledged",
  acknowledged: "Acknowledged by the patient",
};

const EVENT: Record<OutcomeHandoff["history"][number]["kind"], string> = {
  content_approved: "Content approved",
  release_recorded: "Simulated release recorded",
  referrer_acknowledged: "Referrer acknowledged",
  patient_acknowledged: "Patient acknowledged",
};

const GATE: Record<NonNullable<OutcomeHandoff["release"]>["gate"], string> = {
  released: "released to the patient link",
  held_consent: "held: consent unknown",
  suppressed: "suppressed: consent denied or withdrawn",
};

/** Post-closure outcome hand-off. Referrer and patient acknowledgements are separate; neither changes the canonical decision. */
export function OutcomeHandoffCard({ handoff }: { handoff: OutcomeHandoff }) {
  return (
    <section id="outcome-handoff" aria-labelledby="outcome-handoff-h" className="min-w-0 scroll-mt-4 rounded-card border border-line bg-surface p-4 md:p-pad">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id="outcome-handoff-h" className="m-0 text-section font-medium">Outcome hand-off</h2>
        <HonestyLabel kind="simulated">No message is sent</HonestyLabel>
      </div>
      <dl className="m-0 mt-2 grid gap-1">
        <dt className="text-small text-muted">Patient</dt>
        <dd className="m-0">{PATIENT[handoff.status]}</dd>
        <dt className="text-small text-muted">Responsible referrer ({handoff.responsible_referrer})</dt>
        <dd className="m-0">{handoff.referrer_status === "acknowledged" ? "Acknowledged (does not mark the patient informed)" : "Pending"}</dd>
        {handoff.patient_informed !== undefined && (
          <>
            <dt className="text-small text-muted">Patient informed</dt>
            <dd className="m-0">{handoff.patient_informed ? "Yes: the patient acknowledged this version" : "No"}</dd>
          </>
        )}
        {handoff.content && (
          <>
            <dt className="text-small text-muted">Approved next step</dt>
            <dd className="m-0">{handoff.content.next_step.replaceAll("_", " ")}</dd>
          </>
        )}
      </dl>
      {handoff.history.length > 0 && (
        <ol className="m-0 mt-2 list-none p-0" aria-label="Outcome hand-off history">
          {handoff.history.map((h) => (
            <li key={h.event} className="border-t border-rule py-2 text-small">
              {EVENT[h.kind]}
              {h.gate ? ` (${GATE[h.gate]})` : ""} · {h.actor} ({h.role}) · {atWords(h.at)} · version {h.version.slice(0, 8)}
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
