import Link from "next/link";
import { notFound } from "next/navigation";
import { Avatar } from "@/components/avatar/avatar";
import { BookingCard, ConsentCard, IdentitiesCard, LedgerCard, ProgrammeFindingCard, ReadersCard, ReasonCard, SignoffsCard, TimelineCard } from "@/components/case/sections";
import { DoubleReviewReadsCard } from "@/components/review/double-review/reads-card";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { SealedChain } from "@/components/wasl/sealed-chain";
import { caseDeadlineDays } from "@/components/worklist/days";
import { closedInDays, ownerLabel, recordNodes, statusWords } from "@/components/worklist/model";
import { getAudit, getCase } from "@/lib/api/source";
import { caseHref, caseIdFromParam, shortId } from "@/lib/domain/ids";
import { caseStage } from "@/lib/domain/stage";
import { personForLoop } from "@/lib/people";
import { referralSeed } from "@/lib/people/identity";
import { cn } from "@/lib/utils";
import { entryLabel } from "@/components/case/words";

export async function generateMetadata({ params }: PageProps<"/app/patients/[id]">) {
  const id = caseIdFromParam((await params).id);
  return { title: personForLoop(id)?.display_name ?? shortId(id) };
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
const SECTIONS = [
  ["why", "Why"],
  ["people", "Records"],
  ["readers", "Readers"],
  ["consent", "Consent"],
  ["signoffs", "Sign-offs"],
  ["ledger", "Coordinator actions"],
  ["timeline", "Timeline"],
  ["booking", "Booking"],
] as const;

export default async function Page({ params }: PageProps<"/app/patients/[id]">) {
  const id = caseIdFromParam((await params).id);
  const [detail, audit] = await Promise.all([getCase(id), getAudit(id)]);
  if (!detail) notFound();
  const c = detail.case;
  const person = personForLoop(c.id);
  const first = person?.display_name.split(" ")[0];
  const nodes = recordNodes(detail, audit?.chain ?? []);
  const closedIn = c.closed ? closedInDays(c) : null;
  /** CRC runs to the DoH 31-day standard; annual LDCT instead shows its 365-day screening interval. */
  const due = caseDeadlineDays(c);
  const dueWords = c.programme === "crc" ? `${due}-day standard` : c.ldct?.rec?.action === "annual" ? `${due}-day screening interval` : `${due}-day deadline`;
  const waiting = c.review === "open" || detail.review_items.some((r) => !detail.signoffs.some((s) => s.item === r.item));
  const [latest, ...earlier] = [...detail.evidence].reverse();

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-gap">
        <div className="flex min-w-0 items-center gap-4">
          <Avatar person={referralSeed(c.id)} size={56} label="" />
          <div className="min-w-0">
            <div className="flex flex-wrap items-baseline gap-x-3">
              <h1 className="m-0 text-title font-semibold">{person?.display_name ?? shortId(c.id)}</h1>
              {person && (
                <span lang="ar" dir="rtl" className="text-section text-muted">
                  {person.display_name_ar}
                </span>
              )}
              <span className="text-small text-muted">
                {person && `${person.age} · `}
                {shortId(c.id)}
              </span>
            </div>
            <p className="m-0 mt-1 flex flex-wrap items-center gap-2">
              {closedIn !== null ? (
                <Pill tone={closedIn <= due ? "positive" : "neutral"}>Closed day {closedIn}</Pill>
              ) : (
                <Pill tone={c.overdue ? "negative" : "neutral"}>{caseStage(detail).label}</Pill>
              )}
              <span className="text-muted">
                {statusWords(c).status === "Closed" ? "closed with evidence" : statusWords(c).status.toLowerCase()} · owner {ownerLabel(c.owner)}
              </span>
            </p>
          </div>
        </div>
        <HonestyLabel kind="simulated">Simulated names</HonestyLabel>
      </header>

      <div className="mt-4 flex flex-wrap items-center gap-3" data-testid="case-actions">
        {detail.review_items.length > 0 && (
          <Link
            href={caseHref("/app/review", c.id)}
            className={cn(
              BUTTON,
              waiting ? "bg-cta text-white hover:bg-ink hover:text-white" : "border border-control bg-surface text-ink hover:bg-active hover:text-ink",
            )}
          >
            {c.programme !== "crc" ? "Review programme evidence" : waiting ? "Open in Review" : "Review history"}
          </Link>
        )}
        <Link
          href={`${caseHref("/app/patients", c.id)}/audit`}
          className={cn(
            BUTTON,
            detail.review_items.length > 0
              ? "border border-control bg-surface text-ink hover:bg-active hover:text-ink"
              : "bg-cta text-white hover:bg-ink hover:text-white",
          )}
        >
          Audit trail
        </Link>
        {audit && (
          <span className="text-muted" data-testid="audit-teaser">
            {audit.chain_verified ? "Chain verified by the server" : "Chain broken"}
          </span>
        )}
      </div>

      <nav aria-label="On this page" className="mt-3">
        <ul className="m-0 flex list-none flex-wrap gap-2 p-0">
          {SECTIONS.map(([anchor, label]) => (
            <li key={anchor}>
              <a
                href={`#${anchor}`}
                className="inline-flex min-h-11 items-center rounded-control border border-line bg-surface px-3 text-ink no-underline hover:bg-active hover:text-ink"
              >
                {label}
              </a>
            </li>
          ))}
        </ul>
      </nav>

      <div className="mt-6 flex flex-wrap items-stretch gap-gap">
        <section aria-labelledby="chain-h" className="min-w-0 flex-[999_1_560px] self-start rounded-card border border-line bg-surface p-4 md:p-pad">
          <div className="flex flex-wrap items-baseline justify-between gap-gap">
            <h2 id="chain-h" className="m-0 text-section font-medium">
              {first ? "Her case, step by step" : "The case, step by step"}
            </h2>
            <span className="text-small text-muted" data-testid="closed-in">
              {closedIn !== null
                ? closedIn <= due
                  ? `Closed in ${closedIn} days, ${due - closedIn} inside the ${dueWords}`
                  : `Closed in ${closedIn} days, ${closedIn - due} past the ${dueWords}`
                : statusWords(c).next}
            </span>
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
          <ReasonCard detail={detail} />
          <ProgrammeFindingCard detail={detail} />
          <IdentitiesCard detail={detail} />
          {latest && <ReadersCard detail={detail} ev={latest} id="readers" />}

          {audit && (
            <section aria-labelledby="audit-h" className="rounded-card border border-line bg-surface px-4 py-2 md:px-pad">
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
                {audit.chain_verified ? (
                  <Pill tone="positive">
                    Verified · {audit.chain.length} of {audit.chain_length} intact
                  </Pill>
                ) : (
                  <Pill tone="negative">Chain broken</Pill>
                )}
                <span className="text-small text-muted">The server recomputed each hash from its contents and the entry before it.</span>
              </p>
              <p className="m-0 pb-3">
                <Link href={`${caseHref("/app/patients", c.id)}/audit`} className="inline-flex min-h-11 items-center">
                  Open the full audit trail
                </Link>
              </p>
            </section>
          )}
        </div>
      </div>

      <div className="mt-gap grid items-start gap-gap lg:grid-cols-2">
        <div className="flex min-w-0 flex-col gap-gap">
          {earlier.map((ev) => (
            <ReadersCard key={ev.report} detail={detail} ev={ev} id={`readers-${shortId(ev.report)}`} title={`Earlier reading: report ${shortId(ev.report)}`} />
          ))}
          <ConsentCard detail={detail} />
          <SignoffsCard detail={detail} />
          <DoubleReviewReadsCard detail={detail} />
          <LedgerCard detail={detail} />
        </div>
        <div className="flex min-w-0 flex-col gap-gap">
          <BookingCard detail={detail} />
          <TimelineCard detail={detail} />
        </div>
      </div>
    </>
  );
}
