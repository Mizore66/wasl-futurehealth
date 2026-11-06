import Link from "next/link";
import { redirect } from "next/navigation";
import { Avatar } from "@/components/avatar/avatar";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { StatusMark, type Status } from "@/components/wasl/status-mark";
import { STANDARD_DAYS } from "@/components/worklist/days";
import { filtersHref, PAGE_SIZE, parseCaseFilters, parsePage, type CaseFilters } from "@/components/worklist/filters";
import { closedInDays, DROPOFF_WORDS, PROGRAMME_NAMES, workItem } from "@/components/worklist/model";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { getCases } from "@/lib/api/source";
import type { CaseSummary } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { caseHref } from "@/lib/domain/ids";
import { plain } from "@/lib/domain/terms";
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
    f.programme && PROGRAMME_NAMES[f.programme].toLowerCase(),
    f.state && plain(f.state).toLowerCase(),
    f.dropoff && DROPOFF_WORDS[f.dropoff].status.toLowerCase(),
    f.overdue === true && "overdue",
    f.overdue === false && "not overdue",
    f.q && `matching “${f.q}”`,
  ].filter(Boolean);
  return parts.length ? parts.join(" · ") : "all cases";
}

const same = (a: CaseFilters, b: CaseFilters) => filtersHref("", a) === filtersHref("", b);

/** Chips change the view but keep the programme. */
const chipFilters = (f: CaseFilters, keep: CaseFilters): CaseFilters => ({ ...f, programme: keep.programme, q: keep.q });

/** Days from referral to the programme's deadline: 31 for CRC, the rule's own deadline for the other programmes. */
const dueIn = (c: CaseSummary) => c.deadline_day || STANDARD_DAYS;

function mark(c: CaseSummary): Status {
  if (c.closed) return "done";
  if (c.state === "inadequate" || c.review === "open") return "held";
  return "queued";
}

export default async function Page({ searchParams }: PageProps<"/app/patients">) {
  const sp = await searchParams;
  const filters = parseCaseFilters(sp);
  const page = parsePage(sp);
  const [list, now] = await Promise.all([getCases({ ...filters, limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE }), runClock()]);
  const pages = Math.max(1, Math.ceil(list.total / PAGE_SIZE));
  if (page > pages) redirect(filtersHref("/app/patients", filters, pages));
  const rows = list.cases.map((c) => ({ c, it: workItem(c, now, hasRecord(c)) }));
  const from = list.total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1;
  const to = (page - 1) * PAGE_SIZE + rows.length;
  const chip = "inline-flex min-h-11 items-center rounded-chip border px-3 text-body no-underline";

  return (
    <>
      <header className="mb-6 flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
        <div>
          <h1 className="m-0 text-title font-semibold">Patients</h1>
          <p className="m-0 mt-1 text-muted" data-testid="showing">
            {list.total} {list.total === 1 ? "case" : "cases"}: {describe(filters)}
          </p>
          {rows.some((r) => !r.it.hasRecord) && <p className="m-0 mt-1 text-muted">In this demo only some cases have a full record. Those open when you tap them.</p>}
        </div>
        <div className="flex flex-col gap-1.5 sm:items-end">
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
              const on = same(f, { ...filters, programme: undefined, q: undefined });
              return (
                <Link
                  key={label}
                  href={filtersHref("/app/patients", chipFilters(f, filters))}
                  aria-current={on ? "true" : undefined}
                  className={cn(chip, on ? "border-ink bg-active text-ink" : "border-control text-ink hover:bg-ground")}
                >
                  {label}
                </Link>
              );
            })}
            {filters.programme && (
              <Link href={filtersHref("/app/patients", { ...filters, programme: undefined })} className={cn(chip, "border-control text-ink hover:bg-ground")}>
                {PROGRAMME_NAMES[filters.programme]} · show all programmes
              </Link>
            )}
          </nav>
          <form action="/app/patients" className="flex w-full items-center gap-2 md:ml-auto md:w-auto">
            {filters.programme && <input type="hidden" name="programme" value={filters.programme} />}
            {filters.state && <input type="hidden" name="state" value={filters.state} />}
            {filters.dropoff && <input type="hidden" name="dropoff" value={filters.dropoff} />}
            {filters.overdue !== undefined && <input type="hidden" name="overdue" value={String(filters.overdue)} />}
            <label className="flex min-w-0 flex-1 items-center gap-2 rounded-control border border-control px-2.5 md:flex-none">
              <span className="sr-only">Search case id</span>
              <input type="search" name="q" defaultValue={filters.q} placeholder="Case id" className="w-full min-w-0 h-11 border-0 bg-transparent text-body outline-none md:w-40" />
            </label>
            <button className="h-11 cursor-pointer rounded-control bg-active px-4 text-body text-ink hover:bg-control">Search</button>
          </form>
        </div>

        <table className="w-full border-collapse text-left max-md:block">
          <thead className="max-md:sr-only">
            <tr className="text-small text-muted">
              <th scope="col" className="px-4 py-2 font-normal">Patient</th>
              <th scope="col" className="px-4 py-2 font-normal">Status</th>
              <th scope="col" className="px-4 py-2 font-normal">Referred</th>
              <th scope="col" className="px-4 py-2 font-normal">Due</th>
              <th scope="col" className="px-4 py-2 font-normal">Owner</th>
            </tr>
          </thead>
          <tbody className="max-md:block">
            {rows.map(({ c, it }) => (
              <tr key={c.id} className={cn("relative border-t border-rule max-md:flex max-md:flex-col max-md:gap-1 max-md:px-4 max-md:py-3", it.hasRecord && "hover:bg-ground")}>
                <td className="px-4 py-2.5 max-md:block max-md:p-0">
                  <span className="flex min-h-11 flex-wrap items-center gap-x-2.5 gap-y-1">
                    {it.hasRecord ? (
                      <Link
                        href={caseHref("/app/patients", c.id)}
                        className="flex min-h-11 items-center gap-2.5 font-medium no-underline after:absolute after:inset-0 after:content-['']"
                      >
                        <Avatar person={it.seed} size={24} label="" />
                        {it.name}
                      </Link>
                    ) : (
                      <>
                        <Avatar person={it.seed} size={24} label="" />
                        <span className="font-medium">{it.name}</span>
                      </>
                    )}
                    {it.nameAr && (
                      <span lang="ar" dir="rtl" className="text-muted">
                        {it.nameAr}
                      </span>
                    )}
                    {it.named && <span className="font-mono text-small text-muted">{it.short}</span>}
                  </span>
                </td>
                <td className="px-4 py-2.5 max-md:block max-md:p-0">
                  <StatusMark status={mark(c)} label={c.closed ? `Closed day ${closedInDays(c)}` : it.status} className="max-md:text-body" />
                </td>
                <td className="px-4 py-2.5 text-muted max-md:block max-md:p-0">
                  <span className="md:hidden">Referred </span>
                  {formatDay(c.referral_day)}
                </td>
                <td className={cn("px-4 py-2.5 max-md:block max-md:p-0", !c.closed && c.overdue ? "text-negative" : "text-muted")}>
                  <span className="md:hidden">Due </span>
                  {formatDay(c.referral_day + dueIn(c))}
                  {!c.closed && c.overdue && ` · ${it.days - dueIn(c)} over`}
                </td>
                <td className="px-4 py-2.5 text-muted max-md:block max-md:p-0">
                  <span className="md:hidden">Owner </span>
                  {it.owner}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {rows.length === 0 && <p className="m-0 px-4 py-6 text-muted">No cases match. Try another filter.</p>}

        <nav aria-label="Pages" className="flex flex-wrap items-center justify-between gap-gap border-t border-line px-4 py-3">
          <p className="m-0 text-muted" data-testid="range">
            Showing {from}–{to} of {list.total}
          </p>
          <div className="flex gap-2">
            {page > 1 && (
              <Link href={filtersHref("/app/patients", filters, page - 1)} rel="prev" className={cn(chip, "border-control text-ink hover:bg-ground")}>
                Previous
              </Link>
            )}
            {page < pages && (
              <Link href={filtersHref("/app/patients", filters, page + 1)} rel="next" className={cn(chip, "border-control text-ink hover:bg-ground")}>
                Next
              </Link>
            )}
          </div>
        </nav>
      </section>
    </>
  );
}
