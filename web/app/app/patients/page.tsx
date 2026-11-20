import Link from "next/link";
import { redirect } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { PageHeader } from "@/components/shell/page-header";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { loadCaseRecord, renderEvidenceFile } from "@/components/case/evidence-file";
import { LedgerInvalidBanner, listLedgerInvalid } from "@/components/worklist/ledger-banner";
import { facetValueLabel, facets as buildFacets, programmesInView } from "@/components/patients/facets";
import { clearRefineHref, NativeRefine, refineMenus } from "@/components/patients/refine";
import { FolderTab, RememberFiles } from "@/components/patients/back-to-files";
import { CaseFiles, ReferralCalendar, type Row } from "@/components/patients/referral-calendar";
import {
  byDate,
  calendarHref,
  chooseMonth,
  monthsWith,
  parseCalendarView,
  plural,
  referralDate,
  scopeLabel,
  searchFields,
  searchMatches,
  type CalendarView,
} from "@/components/patients/calendar";
import { filtersHref, PAGE_SIZE, parseCaseFilters, parsePage, toCaseQuery, UNOWNED, type CaseFilters } from "@/components/worklist/filters";
import { DROPOFF_WORDS, PROGRAMME_NAMES, workItem } from "@/components/worklist/model";
import { ldctChildren, openObligation } from "@/components/worklist/obligation";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { apiMode, getCases, matches, type CaseQuery } from "@/lib/api/source";
import type { CaseSummary } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { plain } from "@/lib/domain/terms";
import { cn } from "@/lib/utils";
import { FilterMenu } from "@/components/ui/filters/filter-menu";
import { FilterSheet } from "@/components/ui/filters/filter-sheet";
import { SearchField } from "@/components/ui/filters/search-field";
import { ClearAll, ResultCount, SegmentedFilter, Toolbar } from "@/components/ui/filters/toolbar";
import "@/components/patients/patients.css";

export const metadata = { title: "Patients" };

const QUICK: { label: string; f: CaseFilters }[] = [
  { label: "All", f: {} },
  { label: "Overdue", f: { overdue: true } },
  { label: "Never notified", f: { dropoff: "never_notified" } },
  { label: "Not booked", f: { dropoff: "not_booked" } },
  { label: "No-show", f: { dropoff: "no_show" } },
  { label: "Closed", f: { closed: true } },
];

function describe(f: CaseFilters): string {
  const parts = [
    f.programme && PROGRAMME_NAMES[f.programme].toLowerCase(),
    f.provider && `provider ${facetValueLabel("provider", f.provider)}`,
    f.consent && `consent ${facetValueLabel("consent", f.consent).toLowerCase()}`,
    f.review && facetValueLabel("review", f.review).toLowerCase(),
    f.owner && (f.owner === UNOWNED ? "no owner" : `owner ${facetValueLabel("owner", f.owner)}`),
    f.state && plain(f.state).toLowerCase(),
    f.closed === true && "closed",
    f.closed === false && "not closed",
    f.dropoff && DROPOFF_WORDS[f.dropoff].status.toLowerCase(),
    f.overdue === true && "overdue",
    f.overdue === false && "not overdue",
    f.q && `matching “${f.q}”`,
  ].filter(Boolean);
  return parts.length ? parts.join(" · ") : "all cases";
}

const same = (a: CaseFilters, b: CaseFilters) => filtersHref("", a) === filtersHref("", b);

/** The quick chips own state, closed, dropoff and overdue. */
const quickOnly = (f: CaseFilters): CaseFilters => ({ state: f.state, closed: f.closed, dropoff: f.dropoff, overdue: f.overdue });

/** Chips change the view but keep every other filter: the refine menus and the search. */
const chipFilters = (f: CaseFilters, keep: CaseFilters): CaseFilters => ({
  ...f,
  programme: keep.programme,
  provider: keep.provider,
  consent: keep.consent,
  review: keep.review,
  owner: keep.owner,
  q: keep.q,
});

/** Cases the calendar reads at most: 20 API pages of 500. Past that the view says it is partial. */
const CALENDAR_CAP = 10_000;

/** Every case matching the filters, page by page (the API serves up to 500 at a time). */
async function allCases(query: CaseQuery): Promise<{ cases: CaseSummary[]; total: number }> {
  const first = await getCases({ ...query, limit: 500, offset: 0 });
  const cases = [...first.cases];
  const want = Math.min(first.total, CALENDAR_CAP);
  const offsets: number[] = [];
  for (let o = cases.length; o < want && first.cases.length > 0; o += 500) offsets.push(o);
  const rest = await Promise.all(offsets.map((offset) => getCases({ ...query, limit: 500, offset })));
  for (const page of rest) cases.push(...page.cases);
  return { cases, total: first.total };
}

export default async function Page({ searchParams }: PageProps<"/app/patients">) {
  const sp = await searchParams;
  const filters = parseCaseFilters(sp);
  const view = parseCalendarView(sp);
  const page = parsePage(sp);
  // The search reads the task and owner words as well as the id, so it runs here over the filtered cases.
  const { q, ...apiFilters } = filters;
  // The unfiltered set (up to the API's 500 a page) gives the choices and their counts.
  const [matched, everything, now] = await Promise.all([allCases(toCaseQuery(apiFilters)), getCases({ limit: 500 }), runClock()]);
  const children = ldctChildren(everything.cases);
  const toRow = (c: CaseSummary): Row => ({ c, it: workItem(c, now, hasRecord(c)), ob: openObligation(c, now, children), date: referralDate(c) });
  const found = (r: Row) => searchMatches(q, searchFields(r.c, r.it));
  const rows = matched.cases.map(toRow).filter(found);
  const searchedAll = q ? everything.cases.filter((c) => found(toRow(c))) : everything.cases;
  const choices = buildFacets(searchedAll, apiFilters);
  const menus = refineMenus(choices, filters);
  // Each status tab counts what choosing it would show: the refine filters and the search stay applied.
  const segments = QUICK.map(({ label, f }) => {
    const query = toCaseQuery(chipFilters(f, { ...apiFilters, q: undefined }));
    return {
      label,
      href: filtersHref("/app/patients", chipFilters(f, filters)),
      current: same(f, quickOnly(filters)),
      count: searchedAll.filter((c) => matches(c, query)).length,
    };
  });
  const inView = programmesInView(searchedAll, apiFilters);
  const partialCounts = everything.total > everything.cases.length;
  const partialCalendar = matched.total > matched.cases.length;
  const sample = apiMode() === "fixtures";
  // Replay failure is server-wide, so any case the API listed saying so makes every status here a partial view.
  const ledgerPartial = listLedgerInvalid(rows.map((r) => r.it)) || everything.cases.some((c) => c.api_overlay?.ledger_status === "invalid");

  const dates = byDate(rows, (r) => r.date);
  const months = monthsWith(dates.keys());

  /* An open folder: the Evidence File takes the page, under a tab back to the files it came from. */
  if (view.open) {
    const openRow = rows.find((r) => r.c.id === view.open);
    const files = view.files ?? (openRow ? openRow.date : undefined);
    const back = calendarHref(filters, { files, page: view.page ?? (page > 1 ? page : undefined) });
    const record = openRow && !openRow.it.hasRecord ? null : await loadCaseRecord(view.open);
    if (record)
      return (
        <div className="pf-folder-open">
          <RememberFiles open={view.open} files={back} />
          {await renderEvidenceFile({ record, back: <FolderTab href={back} /> })}
        </div>
      );
    return (
      <div className="pf-folder-open pf-record-in-folder">
        <FolderTab href={back} />
        <section className="rs-card" aria-label="Record not available">
          {/* Too short to clip the Run metadata drop-down, so the card does not clip and the band rounds its own corner. */}
          <PageHeader
            attached
            className="rounded-se-[6px]!"
            title={openRow?.it.name ?? shortId(view.open)}
            sub={openRow?.it.named ? shortId(view.open) : undefined}
          />
          <div className="p-4 md:p-pad">
            <p className="m-0">{openRow ? "This demo has a full record for only some cases. This case has its summary only." : "No case with this id is in view."}</p>
            {openRow && (
              <dl className="rs-referral-facts">
                <div>
                  <dt>Status</dt>
                  <dd>{openRow.it.status}</dd>
                </div>
                <div>
                  <dt>Next</dt>
                  <dd>{openRow.it.next}</dd>
                </div>
                <div>
                  <dt>Owner</dt>
                  <dd>{openRow.it.owner}</dd>
                </div>
                <div>
                  <dt>Referred</dt>
                  <dd>{formatDay(openRow.c.referral_day)}</dd>
                </div>
              </dl>
            )}
          </div>
        </section>
      </div>
    );
  }

  const scope = view.files;
  const inScope = scope ? rows.filter((r) => r.date.startsWith(scope)).sort((a, b) => a.date.localeCompare(b.date) || a.c.id.localeCompare(b.c.id, "en", { numeric: true })) : [];
  const pages = Math.max(1, Math.ceil(inScope.length / PAGE_SIZE));
  if (scope && page > pages) redirect(calendarHref(filters, { files: scope, page: pages }));
  const shown = inScope.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);
  const from = shown.length === 0 ? 0 : (page - 1) * PAGE_SIZE + 1;
  const to = (page - 1) * PAGE_SIZE + shown.length;
  const month = chooseMonth(months, view);
  const filesView: CalendarView = { files: scope, page: page > 1 ? page : undefined };
  const chip = "inline-flex min-h-11 items-center rounded-chip border px-3 text-body no-underline";
  const searchLabel = scope ? "Filter these files" : "Find a case, task or owner";

  return (
    <>
      {ledgerPartial && <LedgerInvalidBanner what="this list" className="mb-3" />}

      <section className="pf-directory" aria-labelledby="pf-view-heading">
        <PageHeader
          attached
          className="pf-hero"
          title="Patients"
          sub={
            <ResultCount data-testid="showing" count={rows.length} one="case">
              : {describe(filters)}
            </ResultCount>
          }
          aside={<HonestyLabel kind="simulated">Simulated names and cases</HonestyLabel>}
        />
        <Toolbar label="Filter patients" className="pf-filters">
          <SegmentedFilter label="Filters" items={segments} />
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <form action="/app/patients" role="search" className="min-w-0 basis-full sm:flex-1 sm:basis-64">
              {filters.programme && <input type="hidden" name="programme" value={filters.programme} />}
              {filters.provider && <input type="hidden" name="provider" value={filters.provider} />}
              {filters.consent && <input type="hidden" name="consent" value={filters.consent} />}
              {filters.review && <input type="hidden" name="review" value={filters.review} />}
              {filters.owner && <input type="hidden" name="owner" value={filters.owner} />}
              {filters.state && <input type="hidden" name="state" value={filters.state} />}
              {filters.dropoff && <input type="hidden" name="dropoff" value={filters.dropoff} />}
              {filters.overdue !== undefined && <input type="hidden" name="overdue" value={String(filters.overdue)} />}
              {filters.closed !== undefined && <input type="hidden" name="closed" value={String(filters.closed)} />}
              {scope && <input type="hidden" name="files" value={scope} />}
              <SearchField
                id="pf-find"
                name="q"
                label={searchLabel}
                defaultValue={q}
                clearHref={calendarHref({ ...filters, q: undefined }, { files: scope })}
                submitLabel="Search"
              />
            </form>
            <div className="flex min-w-0 items-center gap-2 max-sm:w-full max-sm:overflow-x-auto sm:flex-wrap">
              <FilterSheet filters={menus} clearHref={clearRefineHref(filters)} total={rows.length} one="case" className="sm:hidden" />
              {menus.map((m) => (
                <FilterMenu key={m.label} {...m} className={m.value ? undefined : "max-sm:hidden"} />
              ))}
              <ClearAll filters={menus} href={clearRefineHref(filters)} />
            </div>
          </div>
          <NativeRefine facets={choices} filters={filters} />
        </Toolbar>

        <header className="pf-toolbar">
          <div>
            {scope && (
              <Link href={calendarHref(filters, { month: scope.slice(0, 7) })} className="pf-back">
                <ArrowLeft aria-hidden="true" />
                Calendar
              </Link>
            )}
            <h2 id="pf-view-heading">{scope ? "Case files" : "Referral calendar"}</h2>
          </div>
        </header>

        <div id="pf-results">
          {rows.length === 0 ? (
            <div className="pf-empty">
              <p className="m-0">
                {q ? "No matching cases." : `No cases match${Object.keys(filters).length ? " these filters" : ""}. Try removing one.`}
              </p>
              {q && (
                <Link href={filtersHref("/app/patients", { ...filters, q: undefined })} className="pf-link">
                  Clear search
                </Link>
              )}
            </div>
          ) : scope ? (
            <>
              <div className="pf-files-head">
                <h3>{scopeLabel(scope)}</h3>
                <small>{plural(inScope.length, "file")}</small>
              </div>
              {inScope.length === 0 ? (
                <p className="pf-empty m-0">No referrals were received {scope.length === 10 ? "on this date" : "in this month"}{q ? " that match the search" : ""}.</p>
              ) : (
                <CaseFiles rows={shown} view={filesView} filters={filters} />
              )}
              {inScope.length > PAGE_SIZE && (
                <nav aria-label="Pages" className="pf-pager">
                  <p className="m-0 text-muted" data-testid="range">
                    Showing {from}–{to} of {inScope.length}
                  </p>
                  <div className="flex gap-2">
                    {page > 1 && (
                      <Link href={calendarHref(filters, { files: scope, page: page - 1 })} rel="prev" className={cn(chip, "border-control text-ink hover:bg-ground")}>
                        Previous
                      </Link>
                    )}
                    {page < pages && (
                      <Link href={calendarHref(filters, { files: scope, page: page + 1 })} rel="next" className={cn(chip, "border-control text-ink hover:bg-ground")}>
                        Next
                      </Link>
                    )}
                  </div>
                </nav>
              )}
            </>
          ) : (
            month && <ReferralCalendar month={month} months={months} byDate={dates} filters={filters} />
          )}
        </div>
      </section>

      <footer className="pf-notes">
        <p className="m-0" data-testid="programmes-in-view">
          {inView.length === 0
            ? "No programme in view."
            : `${inView.length === 1 ? "Programme" : "Programmes"} in view: ${inView.map((p) => `${p.name} (${p.count})`).join(", ")}.`}
        </p>
        <p className="m-0" data-testid="count-basis">
          {sample
            ? `Counts are over the published sample of ${everything.total} cases (simulated), not the whole run.`
            : partialCounts
              ? `Filter counts cover the first ${everything.cases.length} of ${everything.total} cases.`
              : `Filter counts cover all ${everything.total} cases in this run.`}
        </p>
        {partialCalendar && <p className="m-0">The calendar shows the first {matched.cases.length} of {matched.total} referrals. Add a filter to see the rest.</p>}
        {rows.some((r) => !r.it.hasRecord) && <p className="m-0">In this demo only some cases have a full record. The rest open as a summary.</p>}
        <p className="m-0">
          Run day {Math.floor(now)}, {formatDay(now)}. Dates are when the referral was received (UTC).
        </p>
      </footer>
    </>
  );
}

