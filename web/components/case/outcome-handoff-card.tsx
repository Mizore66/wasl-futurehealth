import type { ReactNode } from "react";
import { HONESTY_PLAIN, HonestyLabel } from "@/components/wasl/honesty-label";
import type { OutcomeHandoff } from "@/lib/api/schema";
import { NEXT_STEP_WORDS } from "./outcome-handoff-words";
import { atWords } from "./words";

/**
 * Staff steps, built on the server (`outcomeHandoffActions`) and passed in so that rendering the card never loads the
 * server actions. `live: false` means fixtures: nothing can be appended.
 */
export type OutcomeHandoffActions = { live: false } | { live: true; identity?: ReactNode; approve?: ReactNode; release?: ReactNode };

const PATIENT: Record<OutcomeHandoff["status"], string> = {
  not_eligible: "Not eligible. The case is not closed, or a patient contest holds it open.",
  pending_approval: "Pending clinician approval of the outcome, next step and safety-net content",
  pending_release: "Approved, not yet released. A coordinator records the simulated release, which checks consent.",
  held_consent: "Approved, held. Contact permission was unknown at release. A new release is needed once it is granted.",
  suppressed: "Approved, suppressed. The patient denied or withdrew contact permission.",
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
export function OutcomeHandoffCard({ handoff, actions }: { handoff: OutcomeHandoff; actions?: OutcomeHandoffActions }) {
  return (
    <section id="outcome-handoff" aria-labelledby="outcome-handoff-h" className="min-w-0 scroll-mt-4 rounded-card border border-line bg-surface p-4 md:p-pad">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id="outcome-handoff-h" className="m-0 text-section font-medium">Outcome hand-off</h2>
        <HonestyLabel kind="simulated" className={HONESTY_PLAIN}>No message is sent.</HonestyLabel>
      </div>
      <dl className="m-0 mt-2 grid gap-1">
        <dt className="text-small text-muted">Patient</dt>
        <dd className="m-0">{PATIENT[handoff.status]}</dd>
        <dt className="text-small text-muted">Responsible referrer ({handoff.responsible_referrer})</dt>
        <dd className="m-0">{handoff.referrer_status === "acknowledged" ? "Acknowledged (does not mark the patient informed)" : "Pending"}</dd>
        {handoff.patient_informed !== undefined && (
          <>
            <dt className="text-small text-muted">Patient informed</dt>
            <dd className="m-0">{handoff.patient_informed ? "Yes, the patient acknowledged this version" : "No"}</dd>
          </>
        )}
        <dt className="text-small text-muted">Contact permission now</dt>
        <dd className="m-0">{handoff.consent}</dd>
        {handoff.draft && (
          <>
            <dt className="text-small text-muted">Current decision</dt>
            <dd className="m-0">
              {handoff.draft.programme} · {handoff.draft.outcome.replaceAll("_", " ")} · signature {handoff.draft.decision.slice(0, 12)}
            </dd>
          </>
        )}
        {handoff.content && (
          <>
            <dt className="text-small text-muted">Approved next step</dt>
            <dd className="m-0">
              {NEXT_STEP_WORDS[handoff.content.next_step] ?? handoff.content.next_step.replaceAll("_", " ")}
              {handoff.approved_by ? ` · approved by ${handoff.approved_by}` : ""}
              {handoff.approved_at ? ` · ${atWords(handoff.approved_at)}` : ""}
              {handoff.version ? ` · version ${handoff.version.slice(0, 8)}` : ""}
            </dd>
          </>
        )}
        {handoff.release && (
          <>
            <dt className="text-small text-muted">Latest release record</dt>
            <dd className="m-0">
              {GATE[handoff.release.gate]} · {handoff.release.actor} · {atWords(handoff.release.at)}
            </dd>
          </>
        )}
      </dl>
      {actions && <StaffSteps handoff={handoff} actions={actions} />}
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

/** Approve (clinician, signed in) and release (coordinator credential). Referrer acknowledgement stays read-only here. */
function StaffSteps({ handoff, actions }: { handoff: OutcomeHandoff; actions: OutcomeHandoffActions }) {
  let body: ReactNode;
  if (handoff.status === "not_eligible") {
    body = <p className="m-0 text-small text-muted">No staff step. Only a closed case with no open patient contest can have an outcome hand-off.</p>;
  } else if (!actions.live) {
    body = <p className="m-0 text-small text-muted">Approval and release need the live API. Fixture data cannot record them.</p>;
  } else {
    body = (
      <>
        {actions.identity}
        {actions.approve}
        {actions.release ?? (
          <p className="m-0 mt-3 text-small text-muted">Release is available once a clinician has approved the current content.</p>
        )}
      </>
    );
  }
  return (
    <div className="mt-3 border-t border-rule pt-3" data-testid="outcome-handoff-steps">
      <h3 className="m-0 text-body font-medium">Staff steps</h3>
      {body}
    </div>
  );
}
