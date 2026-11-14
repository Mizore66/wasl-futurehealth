import Link from "next/link";
import type { PersonAssociationUnavailable, PersonCase, PersonJourney } from "@/lib/api/person-schema";
import { caseHref, shortId } from "@/lib/domain/ids";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { actionLabels, lifecycleLabel, obligationsFor, personDisplay, programmeLabels, reasonLabel } from "./journey-model";

const linkClass = "inline-flex min-h-11 items-center rounded-control px-2 text-ink underline underline-offset-4 hover:bg-active";
const sectionClass = "rounded-card border border-line bg-surface p-4 md:p-pad";

function Day({ value, label = "Day" }: { value: number; label?: string }) {
  return <span className="tabular-nums">{label} {value}</span>;
}

function CaseLink({ caseID, children }: { caseID: string; children?: React.ReactNode }) {
  return <Link className={linkClass} href={caseHref("/app/patients", caseID)}>{children ?? `Open ${shortId(caseID)} case`}</Link>;
}

export function PersonAssociationUnavailableView({ result }: { result: PersonAssociationUnavailable }) {
  const reasons = {
    conflicting_request: "Conflicting versions do not establish a selected subject.",
    unbound_request: "No accepted patient registration was bound to this request.",
    unknown_programme: "The source request did not establish a supported programme context.",
    unproven_binding: "The retained source does not prove a unique subject binding.",
    multiple_historical_subjects: "This case has more than one proven historical subject, so it has no single person destination.",
  };
  return <section aria-labelledby="association-unavailable">
    <h1 id="association-unavailable" className="text-title font-semibold">Historical association unavailable</h1>
    <p>{reasons[result.association.reason]}</p>
    <p>The retained case remains available for authorized case and audit reads. It creates no current care or outreach action.</p>
    <CaseLink caseID={result.case_id}>Open retained case history</CaseLink>
  </section>;
}

function Obligations({ journey, caseID }: { journey: PersonJourney; caseID: string }) {
  const obligations = obligationsFor(journey, caseID);
  if (obligations.length === 0) return <p>No open or scheduled obligations are recorded for this case.</p>;
  return (
    <ul className="m-0 list-disc space-y-2 ps-5">
      {obligations.map((obligation) => (
        <li key={`${obligation.case_id}-${obligation.source_field}-${obligation.due_day}`}>
          <strong>{obligation.status.replaceAll("_", " ")}</strong> · <Day value={obligation.due_day} label="Due day" />
          {obligation.window_start_day !== undefined && <> · window opens day {obligation.window_start_day}</>}
          <div className="text-small text-muted">Source: {obligation.source_field}</div>
          {obligation.basis && <div className="text-small text-muted">{obligation.basis}</div>}
        </li>
      ))}
    </ul>
  );
}

function ProgrammeCase({ journey, item, index }: { journey: PersonJourney; item: PersonCase; index: number }) {
  const { detail, contact } = item;
  const trustedLinks = detail.links.filter((link) => !link.near_miss);
  const prefix = `case-${index}`;
  return (
    <article aria-labelledby={`${prefix}-heading`} className="border-t border-rule pt-5 first:border-t-0 first:pt-0">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 id={`${prefix}-heading`} className="m-0 text-section font-medium">{shortId(item.case_id)}</h3>
          <p className="m-0 mt-1 text-muted">State: {detail.case.state.replaceAll("_", " ")} · referral day {detail.case.referral_day}</p>
          {item.parent_case_id && <p className="m-0 text-small text-muted">Surveillance child of {shortId(item.parent_case_id)}</p>}
        </div>
        <CaseLink caseID={item.case_id}>Open full case and evidence</CaseLink>
      </div>

      <div className="mt-4 grid gap-5 lg:grid-cols-2">
        <section aria-labelledby={`${prefix}-consent`}>
          <h4 id={`${prefix}-consent`} className="m-0 mb-2 text-body font-semibold">Programme contact permission</h4>
          <ul className="m-0 list-disc ps-5">
            <li>Canonical permission: <strong>{contact.canonical.replaceAll("_", " ")}</strong></li>
            <li>Demo overlay: {contact.overlay_status.replaceAll("_", " ")}</li>
            <li>Contact blocked: {contact.contact_blocked ? "yes" : "no"}</li>
            <li>Canonical decision unchanged by the overlay</li>
          </ul>
        </section>
        <section aria-labelledby={`${prefix}-holds`}>
          <h4 id={`${prefix}-holds`} className="m-0 mb-2 text-body font-semibold">Current case holds</h4>
          {detail.holds.length ? (
            <ul className="m-0 list-disc ps-5">
              {detail.holds.map((hold, holdIndex) => <li key={`${hold.loop}-${hold.rule}-${holdIndex}`}>{hold.reason?.replaceAll("_", " ") ?? hold.rule} · {hold.state.replaceAll("_", " ")}</li>)}
            </ul>
          ) : <p>No holds are recorded in the case projection.</p>}
        </section>
      </div>

      <section aria-labelledby={`${prefix}-obligations`} className="mt-5">
        <h4 id={`${prefix}-obligations`} className="m-0 mb-2 text-body font-semibold">Obligations</h4>
        <Obligations journey={journey} caseID={item.case_id} />
      </section>

      <section aria-labelledby={`${prefix}-outreach`} className="mt-5">
        <h4 id={`${prefix}-outreach`} className="m-0 mb-2 text-body font-semibold">Latest outreach intents</h4>
        {detail.outreach.length ? (
          <ul className="m-0 list-disc space-y-2 ps-5">
            {detail.outreach.map((outreach, outreachIndex) => (
              <li key={`${outreach.kind}-${outreach.ev}-${outreachIndex}`}>
                {outreach.kind.replaceAll("_", " ")} · {outreach.released ? "release recorded" : outreach.suppress ? `suppressed: ${outreach.suppress.replaceAll("_", " ")}` : "recorded"}
                <span className="block text-small text-muted">Released means recorded for release, never confirmed dispatch or delivery.</span>
              </li>
            ))}
          </ul>
        ) : <p>No outreach intent is recorded in the latest decision.</p>}
      </section>

      <section aria-labelledby={`${prefix}-timeline`} className="mt-5">
        <h4 id={`${prefix}-timeline`} className="m-0 mb-2 text-body font-semibold">Timeline and raw event hashes</h4>
        {detail.timeline.length ? (
          <ol className="m-0 list-none space-y-3 p-0">
            {detail.timeline.map((event) => (
              <li key={`${event.seq}-${event.id}`} className="border-l-2 border-control ps-3">
                <Day value={event.day} /> · {event.type.replaceAll("_", " ")} · {event.status?.replaceAll("_", " ") ?? "status not recorded"}
                <div className="text-small text-muted">{event.provider} · {shortId(event.id)}</div>
                {event.raw_sha && <div className="text-small">Raw event hash <code>{event.raw_sha}</code></div>}
              </li>
            ))}
          </ol>
        ) : <p>No timeline events are available.</p>}
      </section>

      <section aria-labelledby={`${prefix}-identity`} className="mt-5">
        <h4 id={`${prefix}-identity`} className="m-0 mb-2 text-body font-semibold">Identity provenance and trusted links</h4>
        <ul className="m-0 list-disc space-y-1 ps-5">
          {detail.identities.map((identity) => <li key={`${identity.role}-${identity.registration}`}>{identity.role} · {identity.provider} · <code className="break-all">{identity.registration}</code></li>)}
        </ul>
        {trustedLinks.length ? (
          <ul className="m-0 mt-2 list-disc space-y-1 ps-5">
            {trustedLinks.map((link, linkIndex) => <li key={`${link.downstream_event}-${linkIndex}`}>Trusted case link: {shortId(link.downstream_event)} · {link.hops} hop{link.hops === 1 ? "" : "s"}</li>)}
          </ul>
        ) : <p className="text-small text-muted">No additional case link blocks are shown; cluster membership comes only from the canonical identity artifact.</p>}
      </section>

      <section aria-labelledby={`${prefix}-decisions`} className="mt-5">
        <h4 id={`${prefix}-decisions`} className="m-0 mb-2 text-body font-semibold">Decision history and signatures</h4>
        <ol className="m-0 list-none space-y-3 p-0">
          {detail.decisions.map((decision) => (
            <li key={decision.decision_id}>
              {decision.state.replaceAll("_", " ")} · {decision.overdue ? "overdue" : "not overdue"}
              <div className="text-small text-muted">{decision.decision_id}</div>
              <div className="text-small">Decision signature <code className="break-all">{decision.sig}</code></div>
              {decision.consent_log.length > 0 && (
                <ul className="m-0 mt-1 list-disc ps-5 text-small">
                  {decision.consent_log.map((transition, transitionIndex) => (
                    <li key={`${transition.ev}-${transition.kind}-${transitionIndex}`}>Consent transition to {transition.to}; canonical contact {transition.contact ?? "unknown"}</li>
                  ))}
                </ul>
              )}
              {decision.outreach.length > 0 && (
                <ul className="m-0 mt-1 list-disc ps-5 text-small">
                  {decision.outreach.map((outreach, outreachIndex) => (
                    <li key={`${outreach.kind}-${outreach.ev}-${outreachIndex}`}>{outreach.kind.replaceAll("_", " ")} · {outreach.released ? "release recorded (not delivered)" : outreach.suppress ? `held: ${outreach.suppress.replaceAll("_", " ")}` : "intent recorded"}</li>
                  ))}
                </ul>
              )}
            </li>
          ))}
        </ol>
      </section>
    </article>
  );
}

export function Journey({ journey }: { journey: PersonJourney }) {
  const display = personDisplay(journey);
  const action = journey.next_action;
  const sections = journey.person.programmes.map((programme, index) => ({
    programme,
    anchor: `programme-${index}`,
    cases: journey.cases.filter((item) => item.detail.case.programme === programme),
  }));
  return (
    <div className="max-w-6xl pb-10">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="m-0 text-title font-semibold">{display.name}</h1>
          {display.arabic && <p lang="ar" dir="rtl" className="m-0 mt-1 text-section text-muted">{display.arabic}</p>}
          <p className="m-0 mt-1 text-muted">{journey.cases.length} linked case{journey.cases.length === 1 ? "" : "s"} · projection through day {journey.person.as_of_day}</p>
        </div>
        {display.synthetic && <HonestyLabel kind="simulated">Simulated names</HonestyLabel>}
      </header>

      <section aria-labelledby="identity-provenance" className={`${sectionClass} mt-5`}>
        <h2 id="identity-provenance" className="m-0 text-section font-semibold">Trusted identity links only</h2>
        <p>Names are presentation fixtures only. They never establish membership. Uncertain, rejected and cannot-link pairs remain separate.</p>
        <p className="m-0 text-small text-muted">Source: canonical linker clusters · artifact {journey.person.identity_provenance.artifact_schema} · policy {journey.person.identity_provenance.policy}</p>
        <p className="text-small">Artifact SHA-256 <code className="break-all">{journey.person.identity_provenance.artifact_sha256}</code></p>
        <h3 className="m-0 mt-3 text-body font-semibold">Trusted cluster registrations</h3>
        <ul className="m-0 mt-1 list-disc ps-5">
          {journey.person.registrations.map((registration) => <li key={registration.registration}><code className="break-all">{registration.registration}</code> · {registration.providers.join(", ")}</li>)}
        </ul>
      </section>

      <nav aria-label="Person journey sections" className="mt-4">
        <ul className="m-0 flex list-none flex-wrap gap-2 p-0">
          <li><a className={linkClass} href="#next-action">Next action</a></li>
          <li><a className={linkClass} href="#outreach-review">Outreach review</a></li>
          <li><a className={linkClass} href="#upcoming-obligations">Obligations</a></li>
          {sections.map((section) => <li key={section.anchor}><a className={linkClass} href={`#${section.anchor}`}>{programmeLabels[section.programme]}</a></li>)}
        </ul>
      </nav>

      <section aria-labelledby="next-action" className={`${sectionClass} mt-6`} data-testid="next-action">
        <h2 id="next-action" className="m-0 text-section font-semibold">{actionLabels[action.kind]}</h2>
        {(action.priority || action.overdue !== undefined || action.deadline_day !== undefined) && <p><strong>{action.priority}</strong> · {action.overdue ? "overdue" : "not marked overdue"}{action.deadline_day !== undefined && <> · deadline day {action.deadline_day}</>}</p>}
        <ul className="m-0 list-disc ps-5">
          {action.reason_codes.map((code) => <li key={code}>{reasonLabel(code)}</li>)}
        </ul>
        {action.case_id && <CaseLink caseID={action.case_id}>Open source case {shortId(action.case_id)}</CaseLink>}
        {action.outreach_flag_id && <a className={linkClass} href={`#flag-${action.outreach_flag_id}`}>Open the related outreach review group</a>}
        <p className="mb-0 text-small text-muted">Rule {action.rule}. Coordinator review only; this is not a clinical recommendation.</p>
      </section>

      <section aria-labelledby="outreach-review" className={`${sectionClass} mt-gap`}>
        <h2 id="outreach-review" className="m-0 text-section font-semibold">Duplicate and overlapping outreach</h2>
        <p>Review flags preserve the full decision history. Nothing here suppresses, cancels or sends a message.</p>
        {journey.outreach_flags.length ? (
          <ul className="m-0 list-none space-y-4 p-0">
            {journey.outreach_flags.map((flag) => (
              <li key={flag.id} id={`flag-${flag.id}`} className="scroll-mt-24 border-t border-rule pt-3">
                <h3 className="m-0 text-body font-semibold">{flag.match === "duplicate" ? "Duplicate intent" : "Cross-programme overlap"} · {flag.actionable ? "review actionable" : "not actionable"}</h3>
                <p className="text-small text-muted">Review group <code>{flag.id.slice(0, 20)}</code></p>
                <ul className="m-0 list-disc space-y-2 ps-5">
                  {flag.intents.map((intent) => (
                    <li key={`${intent.case_id}-${intent.outreach.kind}-${intent.outreach.ev}`}>
                      <CaseLink caseID={intent.case_id}>{programmeLabels[intent.programme]}: {shortId(intent.case_id)}</CaseLink>
                      <div>{intent.outreach.kind.replaceAll("_", " ")} · {lifecycleLabel(intent.lifecycle)}</div>
                      <div className="text-small text-muted">Latest decision {intent.latest_decision_id}</div>
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ul>
        ) : <p>No duplicate or cross-programme outreach group is present in the current projection.</p>}
      </section>

      <section aria-labelledby="upcoming-obligations" className={`${sectionClass} mt-gap`}>
        <h2 id="upcoming-obligations" className="m-0 text-section font-semibold">Upcoming obligations by due day</h2>
        {journey.obligations.length ? (
          <ol className="m-0 list-decimal space-y-2 ps-5">
            {journey.obligations.map((obligation, index) => (
              <li key={`${obligation.case_id}-${obligation.source_field}-${obligation.due_day}-${index}`}>
                <Day value={obligation.due_day} label="Due day" /> · {programmeLabels[obligation.programme]} · <strong>{obligation.status}</strong> · <CaseLink caseID={obligation.case_id}>{shortId(obligation.case_id)}</CaseLink>
              </li>
            ))}
          </ol>
        ) : <p>No open or scheduled obligations are recorded.</p>}
      </section>

      {journey.schema === "wasl-person-journey/2" && <section aria-labelledby="retained-references" className={`${sectionClass} mt-gap`}>
        <h2 id="retained-references" className="text-section font-semibold">Non-actionable history references</h2>
        <p>Only proven as-of associations appear here. References are not current care participation; full original history remains in the authorized case and audit views.</p>
        {journey.retained_references.map((reference) => <article key={reference.case_id} className="border-t border-rule pt-3">
          <h3 className="text-body font-semibold"><CaseLink caseID={reference.case_id}>{shortId(reference.case_id)}</CaseLink> · {reference.lifecycle === "retained" ? "Retired case" : "Historical association"}</h3>
          <p>{reference.programmes.map((programme) => programmeLabels[programme]).join(", ") || "Historical programme unavailable"}. History reference only; no current action.</p>
          <details><summary className="cursor-pointer py-3">Source decisions and recorded outreach</summary>
            <ul>{reference.decisions.map((decision) => <li key={decision.decision_id}>{decision.decision_id} · <code>{decision.sig}</code></li>)}</ul>
            <ul>{reference.outreach_intents.map((intent) => <li key={`${intent.case_id}-${intent.outreach.ev}-${intent.outreach.kind}`}>
              {intent.outreach.kind.replaceAll("_", " ")} · {lifecycleLabel(intent.lifecycle)} · historical, not actionable
            </li>)}</ul>
          </details>
        </article>)}
      </section>}

      <div className="mt-6 space-y-gap">
        {sections.map((section) => (
          <section key={section.anchor} id={section.anchor} aria-labelledby={`${section.anchor}-heading`} className={sectionClass}>
            <h2 id={`${section.anchor}-heading`} className="m-0 mb-4 text-section font-semibold">{programmeLabels[section.programme]}</h2>
            <div className="space-y-5">
              {section.cases.map((item) => <ProgrammeCase key={item.case_id} journey={journey} item={item} index={journey.cases.findIndex((candidate) => candidate.case_id === item.case_id)} />)}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}
