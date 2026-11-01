import Link from "next/link";
import { Avatar } from "@/components/avatar/avatar";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { StatusMark, type Status } from "@/components/wasl/status-mark";
import { STANDARD_DAYS } from "@/components/worklist/days";
import { filtersHref, parseCaseFilters, type CaseFilters } from "@/components/worklist/filters";
import { closedInDays, DROPOFF_WORDS, workItem } from "@/components/worklist/model";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { getCases } from "@/lib/api/source";
import type { CaseSummary } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { caseHref } from "@/lib/domain/ids";
import { cn } from "@/lib/utils";

export const metadata = { title: "Patients" };

const QUICK: { label: string; f: CaseFilters }[] = [
  { label: "All", f: {} },
  { label: "Overdue", f: { overdue: true } },
  { label: "Never notified", f: { dropoff: "never_notified" } },
  { label: "Not booked", f: { dropoff: "not_booked" } },
  { label: "No-show", f: { dropoff: "no_show" } },
  { label: "Closed", f: { state: "closed" } },
];

function describe(f: CaseFilters): string {
  const parts = [
    f.state && f.state.replaceAll("_", " "),
    f.dropoff && DROPOFF_WORDS[f.dropoff].status.toLowerCase(),
    f.overdue === true && "overdue",
    f.overdue === false && "not overdue",
    f.q && `matching “${f.q}”`,
  ].filter(Boolean);
  return parts.length ? parts.join(" · ") : "all cases";
}

const same = (a: CaseFilters, b: CaseFilters) => filtersHref("", a) === filtersHref("", b);

/** Days from referral to the programme's deadline: 31 for CRC, the rule's own deadline for the other programmes. */
const dueIn = (c: CaseSummary) => c.deadline_day || STANDARD_DAYS;

function mark(c: CaseSummary): Status {
  if (c.closed) return "done";
  if (c.state === "inadequate" || c.review === "open") return "held";
  return "queued";
}

export default async function Page({ searchParams }: PageProps<"/app/patients">) {
  const filters = parseCaseFilters(await searchParams);
  const [list, now] = await Promise.all([getCases({ ...filters, limit: 200 }), runClock()]);
  const rows = list.cases.map((c) => ({ c, it: workItem(c, now, hasRecord(c)) }));

  return (
    <>
      <header className="mb-6 flex flex-wrap items-end justify-between gap-6">
        <div>
          <h1 className="m-0 text-title font-semibold">Patients</h1>
          <p className="m-0 mt-1 text-muted" data-testid="showing">
            {list.total} {list.total === 1 ? "case" : "cases"}: {describe(filters)}
            {list.total > list.cases.length && ` · first ${list.cases.length} shown`}
          </p>
        </div>
        <div className="flex flex-col items-end gap-1.5">
          <span className="text-small text-muted">
            Run day {Math.floor(now)}, {formatDay(now)}
          </span>
          <HonestyLabel kind="simulated">Simulated names and cases</HonestyLabel>
        </div>
      </header>

      <section aria-label="Cases" className="rounded-card border border-line bg-surface">
        <div className="flex flex-wrap items-center gap-gap border-b border-line px-4 py-3">
          <nav aria-label="Filters" className="flex flex-wrap gap-2">
            {QUICK.map(({ label, f }) => {
              const on = same(f, { ...filters, q: undefined });
              return (
                <Link
                  key={label}
                  href={filtersHref("/app/patients", { ...f, q: filters.q })}
                  aria-current={on ? "true" : undefined}
                  className={cn(
                    "inline-flex h-8 items-center rounded-chip border px-2.5 text-body no-underline",
                    on ? "border-ink bg-active text-ink" : "border-control text-ink hover:bg-ground",
                  )}
                >
                  {label}
                </Link>
              );
            })}
          </nav>
          <form action="/app/patients" className="ml-auto flex items-center gap-2">
            {filters.state && <input type="hidden" name="state" value={filters.state} />}
            {filters.dropoff && <input type="hidden" name="dropoff" value={filters.dropoff} />}
            {filters.overdue !== undefined && <input type="hidden" name="overdue" value={String(filters.overdue)} />}
            <label className="flex h-control items-center gap-2 rounded-control border border-control px-2.5">
              <span className="sr-only">Search case id</span>
              <input type="search" name="q" defaultValue={filters.q} placeholder="Case id" className="w-40 border-0 bg-transparent text-body outline-none" />
            </label>
            <button className="h-control cursor-pointer rounded-control bg-active px-3.5 text-body text-ink hover:bg-control">Search</button>
          </form>
        </div>

        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="text-small text-muted">
              <th scope="col" className="px-4 py-2 font-normal">Patient</th>
              <th scope="col" className="px-4 py-2 font-normal">Status</th>
              <th scope="col" className="px-4 py-2 font-normal">Referred</th>
              <th scope="col" className="px-4 py-2 font-normal">Due</th>
              <th scope="col" className="px-4 py-2 font-normal">Owner</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({ c, it }) => (
              <tr key={c.id} className="border-t border-rule">
                <td className="px-4 py-2.5">
                  <span className="flex items-center gap-2.5">
                    <Avatar person={it.seed} size={24} label="" />
                    {it.hasRecord ? (
                      <Link href={caseHref("/app/patients", c.id)} className="font-medium no-underline">
                        {it.name}
                      </Link>
                    ) : (
                      <span className="font-medium">{it.name}</span>
                    )}
                    {it.nameAr && (
                      <span lang="ar" dir="rtl" className="text-muted">
                        {it.nameAr}
                      </span>
                    )}
                    {it.named && <span className="font-mono text-small text-muted">{it.short}</span>}
                  </span>
                </td>
                <td className="px-4 py-2.5">
                  <StatusMark status={mark(c)} label={c.closed ? `Closed day ${closedInDays(c)}` : it.status} />
                </td>
                <td className="px-4 py-2.5 text-muted">{formatDay(c.referral_day)}</td>
                <td className={cn("px-4 py-2.5", !c.closed && c.overdue ? "text-negative" : "text-muted")}>
                  {formatDay(c.referral_day + dueIn(c))}
                  {!c.closed && c.overdue && ` · ${it.days - dueIn(c)} over`}
                </td>
                <td className="px-4 py-2.5 text-muted">{it.owner}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {rows.length === 0 && <p className="m-0 px-4 py-6 text-muted">No cases match. Try another filter.</p>}
      </section>
    </>
  );
}
