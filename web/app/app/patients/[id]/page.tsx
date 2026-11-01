import { notFound } from "next/navigation";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { SealedChain } from "@/components/wasl/sealed-chain";
import { STANDARD_DAYS } from "@/components/worklist/days";
import { closedInDays, closureChecks, linkScore, ownerLabel, providerLabel, readerLabels, recordNodes, statusWords } from "@/components/worklist/model";
import { getAudit, getCase } from "@/lib/api/source";
import { caseIdFromParam, shortId } from "@/lib/domain/ids";
import { caseStage } from "@/lib/domain/stage";
import { personFor, personForLoop } from "@/lib/people";
import { cn } from "@/lib/utils";

export async function generateMetadata({ params }: PageProps<"/app/patients/[id]">) {
  const id = caseIdFromParam((await params).id);
  return { title: personForLoop(id)?.display_name ?? shortId(id) };
}

const CHAIN_WORDS: Record<string, string> = {
  link_decision: "Report matched",
  evidence: "Readers' result",
  hold_decision: "Held by the gate",
  human_signoff: "Signed by a person",
  closure_decision: "Closed",
};

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

export default async function Page({ params }: PageProps<"/app/patients/[id]">) {
  const id = caseIdFromParam((await params).id);
  const [detail, audit] = await Promise.all([getCase(id), getAudit(id)]);
  if (!detail) notFound();
  const c = detail.case;
  const person = personForLoop(c.id);
  const first = person?.display_name.split(" ")[0];
  const nodes = recordNodes(detail, audit?.chain ?? []);
  const ev = detail.evidence.at(-1);
  /** CRC and cervical evidence carries the four checklist fields; the other programmes carry one label per reader. */
  const read = ev && (ev.fields ? { checks: closureChecks(ev), quote: ev.quote } : readerLabels(ev));
  const link = detail.links[0];
  const closedIn = c.closed ? closedInDays(c) : null;
  /** CRC runs to the DoH 31-day standard; the other programmes' rules carry their own deadline in days. */
  const due = c.programme === "crc" ? STANDARD_DAYS : c.deadline_day || STANDARD_DAYS;
  const dueWords = c.programme === "crc" ? `${due}-day standard` : `${due}-day deadline`;

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-gap">
        <div className="min-w-0">
          <div className="flex flex-wrap items-baseline gap-3">
            <h1 className="m-0 text-title font-semibold">{person?.display_name ?? shortId(c.id)}</h1>
            {person && (
              <span lang="ar" dir="rtl" className="text-section text-muted">
                {person.display_name_ar}
              </span>
            )}
            <span className="text-small text-muted">
              {person && `${person.age} · `}
              <span className="font-mono">{shortId(c.id)}</span>
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
        <HonestyLabel kind="simulated">Simulated names</HonestyLabel>
      </header>

      <div className="mt-6 flex flex-wrap items-stretch gap-gap">
        <section aria-labelledby="chain-h" className="min-w-0 flex-[999_1_560px] self-start rounded-card border border-line bg-surface p-pad">
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
          <section aria-labelledby="id-h" className="rounded-card border border-line bg-surface p-pad">
            <h2 id="id-h" className="m-0 text-section font-medium">
              One patient, two records
            </h2>
            <ul className="m-0 mt-3 list-none p-0">
              {detail.identities.map((idn) => {
                const p = personFor(idn.registration);
                return (
                  <li key={idn.registration} className="flex items-baseline justify-between gap-3 border-t border-rule py-2.5 first:border-t-0">
                    <span className="min-w-0">
                      <span className="block text-small text-muted">{providerLabel(idn.provider)}</span>
                      {p ? (
                        <span lang={p.recorded_script === "arabic" ? "ar" : undefined} dir={p.recorded_script === "arabic" ? "rtl" : undefined} className="font-medium">
                          {p.recorded_name}
                        </span>
                      ) : (
                        <span className="text-muted">Name not on file</span>
                      )}
                    </span>
                    <span className="flex-none font-mono text-small text-muted">{idn.registration.split("|")[1]}</span>
                  </li>
                );
              })}
            </ul>
            {link && (
              <p className="m-0 mt-2 text-small text-muted">
                {link.weakest_edge.basis === "eid"
                  ? "Joined by Emirates ID."
                  : `Joined by name and details, no Emirates ID. Match probability ${linkScore(detail, link).toFixed(3)}, auto-close line ${link.tau}${link.weakest_edge.basis === "human" ? ", confirmed in review" : ""}.`}
              </p>
            )}
          </section>

          {ev && read && (
            <section aria-labelledby="doh-h" className="rounded-card border border-line bg-surface p-pad">
              <h2 id="doh-h" className="m-0 text-section font-medium">
                {ev.fields ? "DoH closure checklist" : "Readers' result"}
              </h2>
              <p className="m-0 mt-0.5 text-small text-muted">Two readers read report {shortId(ev.report)}: {ev.agree ? "they agree" : "they disagree"}.</p>
              <ul className="m-0 mt-3 list-none p-0">
                {read.checks.map((k) => (
                  <li key={k.name} className="flex items-center justify-between gap-3 border-t border-rule py-2.5">
                    <span>{k.name}</span>
                    <span className="text-right text-muted">
                      {k.result}
                      {k.differ && <span className="block text-small text-warning">Readers differ</span>}
                    </span>
                  </li>
                ))}
              </ul>
              {ev.human_override && <p className="m-0 mt-2 text-small text-muted">A clinician signed the evidence {ev.human_override}.</p>}
              <blockquote className="m-0 mt-2 rounded-control bg-raised px-4 py-3">
                <p className="m-0">{read.quote ? `“${read.quote}”` : "The report states none of these in words."}</p>
                <p className="m-0 mt-1 text-small text-muted">
                  {providerLabel(detail.identities.find((i) => i.role === "downstream")?.provider ?? "")} report <span className="font-mono">{shortId(ev.report)}</span>
                </p>
              </blockquote>
            </section>
          )}

          {audit && (
            <section aria-labelledby="audit-h" className="rounded-card border border-line bg-surface px-pad py-2">
              <details>
                <summary className="flex h-12 cursor-pointer list-none items-center gap-2.5">
                  <h2 id="audit-h" className="m-0 flex-1 text-section font-medium">
                    Audit
                  </h2>
                  <span className="text-small text-muted">{audit.chain_length} sealed entries</span>
                </summary>
                <ol aria-label="Entry hashes" className="m-0 list-none p-0 pb-2">
                  {audit.chain.map((e, i) => (
                    <li key={e.hash} className="flex h-7 items-center gap-2 text-small">
                      <span className="w-4 text-right text-muted">{i + 1}</span>
                      <span className="min-w-0 flex-1 truncate">{CHAIN_WORDS[e.type] ?? e.type}</span>
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
            </section>
          )}
        </div>
      </div>
    </>
  );
}
