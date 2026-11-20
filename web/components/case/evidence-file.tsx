import Link from "next/link";
import { CalendarCheck, ClipboardList, Clock, FileText, History, Languages, Send, ShieldCheck, Stethoscope, UserRound } from "lucide-react";
import { Avatar } from "@/components/avatar/avatar";
import { RememberCase } from "@/components/shell/remember-case";
import { PageHeader } from "@/components/shell/page-header";
import { BookingCard, ConsentCard, IdentitiesCard, LedgerCard, PatientJourneySection, ProgrammeFindingCard, ReadersCard, ReasonCard, SignoffsCard } from "@/components/case/sections";
import { RecoveryCard } from "@/components/case/recovery-card";
import { DispositionCard } from "@/components/disposition/disposition-card";
import { dispositionContext } from "@/components/disposition/model";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { SealedChain } from "@/components/wasl/sealed-chain";
import { caseDeadlineDays, isNoAction } from "@/components/worklist/days";
import { LedgerInvalidBanner } from "@/components/worklist/ledger-banner";
import { closedHeadline, closedInDays, dueColumnWords, headerStatusWords, ownerLabel, PROGRAMME_NAMES, providerLabel, recordNodes, statusWords } from "@/components/worklist/model";
import { ApiStatusError, apiMode, getAudit, getCase } from "@/lib/api/source";
import type { CaseDetail } from "@/lib/api/schema";
import { resolveReviewer } from "@/lib/api/reviewer-session";
import { reviewAuthMode } from "@/lib/auth/oidc-config";
import { patientContestReview as contestSlots, type ContestSession } from "@/components/case/patient-contest-review";
import { OutcomeHandoffSection } from "@/components/case/outcome-handoff-review";
import { effectiveOwner } from "@/lib/api/owner";
import { getPersonByCase } from "@/lib/api/person-source";
import { formatDayYear } from "@/lib/domain/day";
import { languageWords } from "@/lib/domain/language";
import { caseHref, shortId } from "@/lib/domain/ids";
import { allQuestionsResolved, caseStage, resolvedItems } from "@/lib/domain/stage";
import { personForLoop, showsSimulatedName } from "@/lib/people";
import { referralSeed } from "@/lib/people/identity";
import { cn } from "@/lib/utils";
import { closureReason, entryLabel, eventWords } from "@/components/case/words";
import { chainCheck } from "@/components/case/audit-view";
import { StageMarks } from "@/components/case/stage-marks";
import { SourceSwitch } from "./source-switch";

type Evidence = CaseDetail["evidence"][number];

/*
 * The full record of one case laid out as the Evidence File: the source documents (latest report first, then the
 * referral), the next step, the activity with the Audit trail clock, then every section of the record. Both
 * /app/patients/[id] and an open folder in the Referral calendar render this, so the two paths show the same record.
 */

/**
 * A patient contest is resolved only by the viewer's own signed-in credential (#167). The session is read only when the
 * live case has an open contest; the shared web credential that renders this page never resolves one.
 */
async function contestSession(detail: CaseDetail): Promise<ContestSession> {
  const live = apiMode() === "live";
  const open = (detail.patient_requests ?? []).some((row) => row.kind === "contest_submitted" && row.status === "open");
  if (!live || !open) return { live, signedIn: false };
  const mode = reviewAuthMode();
  const reviewer = await resolveReviewer();
  return reviewer.ok ? { live, signedIn: true, mode: reviewer.method, expiresAt: reviewer.expiresAt } : { live, signedIn: false, mode };
}

export type CaseRecord = {
  detail: CaseDetail;
  audit: Awaited<ReturnType<typeof getAudit>>;
  personJourney: Awaited<ReturnType<typeof getPersonByCase>> | null;
};

/** The case, its audit and the optional cross-programme link; null when the API has no such case. */
export async function loadCaseRecord(id: string): Promise<CaseRecord | null> {
  const optionalPersonJourney = getPersonByCase(id).catch((error: unknown) => {
    /* Older live runs have no identity projection; only the optional cross-programme link is unavailable then. */
    if (error instanceof ApiStatusError && error.status === 503) return null;
    throw error;
  });
  const [detail, audit, personJourney] = await Promise.all([getCase(id), getAudit(id), optionalPersonJourney]);
  return detail ? { detail, audit, personJourney } : null;
}

function Pill({ tone, children }: { tone: "positive" | "neutral" | "negative"; children: React.ReactNode }) {
  return (
    <span
      className={cn(
        "inline-flex h-[22px] items-center whitespace-nowrap rounded-chip px-2 text-small font-medium",
        tone === "positive" && "bg-pill-positive-bg text-pill-positive-ink",
        tone === "neutral" && "bg-pill-neutral-bg text-pill-neutral-ink",
        tone === "negative" && "bg-pill-negative-bg text-pill-negative-ink",
      )}
    >
      {children}
    </span>
  );
}

const BUTTON = "inline-flex min-h-11 items-center justify-center rounded-control px-4 font-medium no-underline";
const QUIET = "border border-control bg-surface text-ink hover:bg-active hover:text-ink";
const SECTIONS = [
  ["why", "Why"],
  ["people", "Records"],
  ["consent", "Consent"],
  ["patient-journey", "Patient self-service"],
  ["signoffs", "Sign-offs"],
  ["ledger", "Coordinator actions"],
  ["booking", "Booking"],
  ["recovery", "Recovery"],
  ["record-audit-summary", "Audit"],
] as const;

const EVENT_ICON: Record<string, typeof Clock> = {
  ServiceRequest: Send,
  Consent: ShieldCheck,
  Encounter: Stethoscope,
  Appointment: CalendarCheck,
  DiagnosticReport: FileText,
  Procedure: ClipboardList,
};

type Source = { label: string; doc: React.ReactNode };

/** Each report the readers read (latest first) and the referral itself, as the documents the switch moves between. */
function sources(detail: CaseDetail): Source[] {
  const c = detail.case;
  const reports = [...detail.evidence].reverse();
  const outside = (ev: Evidence) => {
    const unit = detail.timeline.find((e) => e.id === ev.report)?.provider ?? ev.report.split("/")[0];
    return unit !== c.provider;
  };
  const base = reports.map((ev) => (outside(ev) ? "Outside report" : "Report"));
  const labels = base.map((b, i) => (base.filter((x) => x === b).length > 1 ? `${b} ${shortId(reports[i].report)}` : b));
  const out: Source[] = reports.map((ev, i) => {
    const event = detail.timeline.find((e) => e.id === ev.report);
    return {
      label: labels[i],
      doc: (
        <article className="rs-document" aria-label={`${labels[i]} ${shortId(ev.report)}`}>
          <div className="record-doc-meta">
            <span>{shortId(ev.report)}</span>
            <Pill tone={ev.agree ? "positive" : "negative"}>{ev.agree ? "Readers agree" : "Readers differ"}</Pill>
          </div>
          <h2>{event ? eventWords(event, c.id) : "Report"}</h2>
          <small>
            {providerLabel(event?.provider ?? ev.report.split("/")[0])}
            {event && ` · ${formatDayYear(event.day)}`}
          </small>
          <ReadersCard
            detail={detail}
            ev={ev}
            id={i === 0 ? "readers" : `readers-${shortId(ev.report)}`}
            title={i === 0 ? undefined : `Supporting reading: report ${shortId(ev.report)}`}
          />
        </article>
      ),
    };
  });
  const due = caseDeadlineDays(c);
  out.push({
    label: "Referral",
    doc: (
      <article className="rs-document" aria-label={`Referral ${shortId(c.id)}`}>
        <div className="record-doc-meta">
          <span>{shortId(c.id)}</span>
          {c.priority && <Pill tone="neutral">{c.priority}</Pill>}
        </div>
        <h2>Referral</h2>
        <small>
          {providerLabel(c.provider)} · {formatDayYear(c.referral_day)}
        </small>
        <dl className="rs-referral-facts">
          <div>
            <dt>Programme</dt>
            <dd>{c.programme ? PROGRAMME_NAMES[c.programme] : "Not recorded"}</dd>
          </div>
          <div>
            <dt>Referred by</dt>
            <dd>{providerLabel(c.provider)}</dd>
          </div>
          <div>
            <dt>{isNoAction(c) ? "Next screen" : "Due"}</dt>
            <dd>
              {isNoAction(c) ? dueColumnWords(c) : formatDayYear(c.referral_day + due)}
              {!isNoAction(c) && ` (${due} days)`}
            </dd>
          </div>
        </dl>
      </article>
    ),
  });
  return out;
}

/**
 * The record header, the Evidence File and every section of the record. `back` is the folder's Back to files tab.
 * Awaited by the page (not rendered as an async child), so the whole record renders in one pass.
 */
export async function renderEvidenceFile({ record, back }: { record: CaseRecord; back?: React.ReactNode }): Promise<React.JSX.Element> {
  const { detail, audit, personJourney } = record;
  const c = detail.case;
  const contestReview = contestSlots(c.id, await contestSession(detail));
  const person = personForLoop(c.id);
  const first = person?.display_name.split(" ")[0];
  const nodes = recordNodes(detail, audit?.chain ?? []);
  const closedIn = c.closed ? closedInDays(c) : null;
  /** CRC runs to the DoH 31-day standard; annual LDCT instead shows its 365-day screening interval. */
  const due = caseDeadlineDays(c);
  /* Independent review: only a terminal state on every question clears waiting, even while canonical review is still open. */
  const waiting = allQuestionsResolved(detail)
    ? false
    : c.review === "open" || detail.review_items.some((r) => !detail.signoffs.some((s) => s.item === r.item) && !resolvedItems(detail).has(r.item));
  /* An invalid replay keeps the valid prefix (so the saved owner still shows) but that owner is not authoritative. */
  const ledgerInvalid = detail.review_ledger?.status === "invalid" || detail.api_overlay?.ledger_status === "invalid";
  const chain = audit ? chainCheck(audit) : undefined;
  const owner = ownerLabel(effectiveOwner(c, detail.api_overlay));
  const auditHref = `${caseHref("/app/patients", c.id)}/audit`;
  const docs = sources(detail);
  const events = [...detail.timeline].sort((a, b) => (a.arrival_day ?? a.day) - (b.arrival_day ?? b.day) || (a.seq ?? 0) - (b.seq ?? 0));
  const words = statusWords(c);

  return (
    <div className={cn("pf-record", back && "pf-record-in-folder")}>
      <RememberCase id={c.id} />
      {ledgerInvalid && <LedgerInvalidBanner what="this record" className="mb-3" />}
      {back}

      <section className="rs-card live-record" aria-label={`Evidence file for ${shortId(c.id)}`}>
        {/* The card does not clip (the Run metadata drop-down hangs over it), so the band rounds the corners the folder tab leaves. */}
        <PageHeader
          attached
          run
          className={back ? "rounded-se-[6px]!" : "rounded-t-[6px]!"}
          title={person?.display_name ?? shortId(c.id)}
          titleAside={
            <>
              <Avatar person={referralSeed(c.id)} size={40} label="" />
              {person && (
                <span lang="ar" dir="rtl" className="text-section text-muted">
                  {person.display_name_ar}
                </span>
              )}
            </>
          }
          sub={
            <>
              <span className="block">
                {person && (
                  <>
                    <span className="tabular-nums">{shortId(c.id)}</span> · {person.age} ·{" "}
                  </>
                )}
                {providerLabel(c.provider)}
                {c.programme && ` · ${PROGRAMME_NAMES[c.programme]}`}
              </span>
              <span className="mt-2 flex flex-wrap items-center gap-2">
                {closedIn !== null ? (
                  <Pill tone={closedIn <= due && !isNoAction(c) ? "positive" : "neutral"}>Closed day {closedIn}</Pill>
                ) : (
                  <StageMarks stage={caseStage(detail).label} overdue={c.overdue} />
                )}
                <span className="text-ink-soft">
                  {headerStatusWords(c)} · owner {owner}
                </span>
              </span>
            </>
          }
          aside={showsSimulatedName(c.id) ? <HonestyLabel kind="simulated">Simulated names</HonestyLabel> : undefined}
        />
        <div className="record-work">
          <div className="rs-browser">
            <SourceSwitch labels={docs.map((d) => d.label)}>{docs.map((d) => d.doc)}</SourceSwitch>
          </div>
          <section className="rs-evidence-actions" aria-labelledby="next-step-h">
            <span className="rs-loop">{c.closed ? "Screening loop closed" : "Screening loop open"}</span>
            <h2 id="next-step-h" data-testid="closed-in">
              {closedIn !== null ? closedHeadline(c) : words.next}
            </h2>
            {/* Why the case is (still) open comes before the activity, so a "done" step is never read as closure. */}
            <p className="consult-prompt" data-testid="why-summary">
              <a href="#why">{c.crc ? "Screening completion is not care completion" : closureReason(detail).title}</a>
            </p>
            <dl className="record-call-facts">
              <div title="Owner">
                <dt>
                  <UserRound aria-hidden="true" />
                  <span className="sr-only">Owner</span>
                </dt>
                <dd>{owner}</dd>
              </div>
              <div title="Referred">
                <dt>
                  <Send aria-hidden="true" />
                  <span className="sr-only">Referred</span>
                </dt>
                <dd>{formatDayYear(c.referral_day)}</dd>
              </div>
              <div title="Preferred language" data-testid="preferred-language">
                <dt>
                  <Languages aria-hidden="true" />
                  <span className="sr-only">Preferred language</span>
                </dt>
                <dd>{languageWords(c.language)}</dd>
              </div>
            </dl>
            <div className="rs-actions" data-testid="case-actions">
              {detail.review_items.length > 0 && (
                <Link href={caseHref("/app/review", c.id)} className={cn(BUTTON, waiting ? "bg-cta text-white hover:bg-ink hover:text-white" : QUIET)}>
                  {c.programme !== "crc" ? "Review programme evidence" : waiting ? "Open in Review" : "Review history"}
                </Link>
              )}
              {personJourney && personJourney.schema !== "wasl-person-association-unavailable/2" && (
                <Link href={`/app/people?case_id=${encodeURIComponent(c.id)}`} className={cn(BUTTON, QUIET)}>
                  View all programmes
                </Link>
              )}
            </div>
            {audit && (
              <p className="m-0 mt-2 text-small text-muted" data-testid="audit-teaser">
                {!audit.chain_verified ? "Chain broken" : audit.chain_length === 0 ? "No sealed entries yet" : "Chain verified by the server"}
              </p>
            )}
          </section>
        </div>
        <div className="record-history">
          <details className="record-log" id="timeline" open>
            <summary>
              Activity <small>{events.length}</small>
            </summary>
            <ol className="record-events" aria-label="Timeline">
              {events.map((e) => {
                const Icon = EVENT_ICON[e.type] ?? Clock;
                return (
                  <li key={`${e.id}@${e.seq}`} className="record-event-row">
                    <span className="record-event-icon">
                      <Icon aria-hidden="true" />
                    </span>
                    <span className="min-w-0">
                      <strong>{eventWords(e, c.id)}</strong>
                      <span className="block text-small text-muted">
                        {providerLabel(e.provider)} · <span className="break-all">{shortId(e.id)}</span>
                      </span>
                    </span>
                    <small className="tabular-nums">{formatDayYear(e.day)}</small>
                  </li>
                );
              })}
            </ol>
          </details>
          <Link href={auditHref} className="record-audit" aria-label="Audit trail" title="Audit trail">
            <History aria-hidden="true" />
          </Link>
        </div>
      </section>

      <nav aria-label="On this page" className="mt-6">
        <h2 className="staff-display m-0 text-[clamp(22px,2vw,28px)] font-medium">The full record</h2>
        <ul className="m-0 mt-2 flex list-none flex-wrap gap-2 p-0">
          {SECTIONS.map(([anchor, label]) => (
            <li key={anchor}>
              <a href={`#${anchor}`} className="inline-flex min-h-11 items-center rounded-control border border-line bg-surface px-3 text-ink no-underline hover:bg-active hover:text-ink">
                {label}
              </a>
            </li>
          ))}
        </ul>
      </nav>

      {/* Why the case is (still) open comes before the step-by-step chain, so a "done" step is never read as closure. */}
      <div className="mt-gap" data-testid="why-above-timeline">
        <ReasonCard detail={detail} />
      </div>

      <div className="mt-gap flex flex-wrap items-stretch gap-gap">
        <section aria-labelledby="chain-h" className="min-w-0 flex-[999_1_560px] self-start rounded-card border border-line bg-surface p-4 md:p-pad">
          <div className="flex flex-wrap items-baseline justify-between gap-gap">
            <h2 id="chain-h" className="m-0 text-section font-medium">
              {first ? "Her case, step by step" : "The case, step by step"}
            </h2>
            <span className="text-small text-muted">{closedIn !== null ? "Closed" : words.status}</span>
          </div>
          <SealedChain nodes={nodes} className="mt-4" />
          <div aria-label="Key" className="mt-4 flex flex-wrap gap-4 text-small text-muted">
            <span className="inline-flex items-center gap-1.5">
              <span aria-hidden className="size-2.5 rounded-full border-2 border-ink" />
              Automatic
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span aria-hidden className="size-2.5 bg-ink" />
              Signed by a person
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span aria-hidden className="size-2.5 rounded-full bg-hue" />
              {c.closed ? "Closed" : "Now"}
            </span>
          </div>
        </section>

        <div className="flex min-w-0 flex-[1_1_360px] flex-col gap-gap">
          <DispositionCard ctx={dispositionContext(detail)} />
          <ProgrammeFindingCard detail={detail} />
          <IdentitiesCard detail={detail} />
          {audit && (
            <section id="record-audit-summary" aria-labelledby="audit-h" className="scroll-mt-4 rounded-card border border-line bg-surface px-4 py-2 md:px-pad">
              <details>
                <summary className="flex min-h-12 cursor-pointer list-none items-center gap-2.5">
                  <h2 id="audit-h" className="m-0 flex-1 text-section font-medium">
                    Audit
                  </h2>
                  <span className="text-small text-muted">{audit.chain_length} sealed entries</span>
                </summary>
                <ol aria-label="Entry hashes" className="m-0 list-none p-0 pb-2">
                  {audit.chain.map((e, i) => (
                    <li key={e.hash} className="flex min-h-7 items-center gap-2 text-small">
                      <span className="w-4 text-right tabular-nums text-muted">{i + 1}</span>
                      <span className="min-w-0 flex-1 truncate">{entryLabel(e.type)}</span>
                      <span className="font-mono text-muted">{e.hash.slice(0, 8)}</span>
                    </li>
                  ))}
                </ol>
              </details>
              <p className="m-0 flex flex-wrap items-center gap-2 pb-3" role="status" data-testid="chain-verified">
                <Pill tone={chain?.tone === "muted" ? "neutral" : (chain?.tone ?? "neutral")}>{chain?.label}</Pill>
                <span className="text-small text-muted">The server recomputed each hash from its contents and the entry before it.</span>
              </p>
              <p className="m-0 pb-3">
                <Link href={auditHref} className="inline-flex min-h-11 items-center">
                  Open the full audit trail
                </Link>
              </p>
            </section>
          )}
        </div>
      </div>

      <div className="mt-gap grid items-start gap-gap lg:grid-cols-2">
        <div className="flex min-w-0 flex-col gap-gap">
          {!!detail.evidence_history?.length && (
            <section aria-label="Historical evidence" className="rounded-card border border-line bg-surface p-4 md:p-pad" data-testid="evidence-history">
              <details>
                <summary className="min-h-11 cursor-pointer py-2 font-medium">Historical evidence</summary>
                <p>These reports are preserved for audit, not the current clinical checklist. Original decisions and signatures are unchanged.</p>
                <ul className="m-0 space-y-3 pl-5">
                  {detail.evidence_history.map((ev) => (
                    <li key={ev.block_hash}>
                      <span className="font-medium">{ev.history === "rejected_registration" ? "Rejected registration" : "Superseded evidence"}</span>
                      <span className="block text-small">Report {shortId(ev.report)} · recorded result: {ev.label}</span>
                      <span className="block text-small text-muted">Decision {shortId(ev.decision_id)} · signature {ev.decision_sig.slice(0, 12)}</span>
                    </li>
                  ))}
                </ul>
              </details>
            </section>
          )}
          <ConsentCard detail={detail} />
          <PatientJourneySection detail={detail} contestReview={contestReview} />
          <OutcomeHandoffSection detail={detail} />
          <SignoffsCard detail={detail} />
          <LedgerCard detail={detail} />
        </div>
        <div className="flex min-w-0 flex-col gap-gap">
          <BookingCard detail={detail} />
          <RecoveryCard detail={detail} />
        </div>
      </div>
    </div>
  );
}
