import Link from "next/link";
import { ChevronLeft, ChevronRight, Folder } from "lucide-react";
import type { CaseFilters } from "@/components/worklist/filters";
import { isActionable, obligationWords, recallDueWords, type Obligation } from "@/components/worklist/obligation";
import { closedInDays, dueColumnWords, type WorkItem } from "@/components/worklist/model";
import type { CaseSummary } from "@/lib/api/schema";
import { shortId } from "@/lib/domain/ids";
import { cn } from "@/lib/utils";
import { calendarHref, dateLabel, monthCells, monthLabel, plural, WEEKDAYS, type CalendarView } from "./calendar";

export type Row = { c: CaseSummary; it: WorkItem; ob: Obligation | null; date: string };

/** The folder's lane: what the coordinator does with it, in the Today words. */
function lane(r: Row): { tone: "call" | "waiting" | "none" | "closed"; words: string } {
  if (r.c.closed && !isActionable(r.ob)) return { tone: "closed", words: "Closed" };
  if (r.it.view === "none") return { tone: "none", words: "No owner" };
  if (r.it.view === "waiting") return { tone: "waiting", words: "Waiting" };
  return { tone: "call", words: "To call" };
}

/** A folder's title: the task, or how the case closed. */
const task = (r: Row) => (r.c.closed ? `Closed day ${closedInDays(r.c)}` : r.it.status);

/**
 * One month of referrals by date received. A date shows its first referral and how many more, never every case, so
 * a busy day stays one tile; choosing it opens that date's case files.
 */
export function ReferralCalendar({
  month,
  months,
  byDate,
  filters,
}: {
  month: string;
  months: string[];
  byDate: Map<string, Row[]>;
  filters: CaseFilters;
}) {
  const pos = months.indexOf(month);
  const total = [...byDate].reduce((n, [d, rows]) => (d.startsWith(month) ? n + rows.length : n), 0);
  const step = (to: string | undefined, dir: "prev" | "next") => {
    const label = dir === "prev" ? "Previous referral month" : "Next referral month";
    const Icon = dir === "prev" ? ChevronLeft : ChevronRight;
    return to ? (
      <Link href={calendarHref(filters, { month: to })} className="pf-step" aria-label={`${label}: ${monthLabel(to)}`} title={label}>
        <Icon aria-hidden="true" />
      </Link>
    ) : (
      <span className="pf-step" aria-hidden="true" data-disabled>
        <Icon />
      </span>
    );
  };
  return (
    <section className="pf-calendar" aria-label="Referrals by date received">
      <header className="pf-cal-head">
        <div className="pf-month-nav">
          {step(months[pos - 1], "prev")}
          <div className="pf-month-label">
            <h2 id="pf-month-heading">{monthLabel(month)}</h2>
            <small className="pf-phone-total">{plural(total, "referral")}</small>
          </div>
          {step(months[pos + 1], "next")}
        </div>
        <div className="pf-month-files">
          <small data-testid="month-total">{plural(total, "referral")}</small>
          <Link href={calendarHref(filters, { files: month })} className="pf-link">
            <Folder aria-hidden="true" />
            Month’s files<span className="sr-only"> for {monthLabel(month)}</span>
          </Link>
        </div>
      </header>
      <div className="pf-week" aria-hidden="true">
        {WEEKDAYS.map((d) => (
          <span key={d}>{d}</span>
        ))}
      </div>
      <ol className="pf-grid">
        {monthCells(month).map((date, i) => {
          if (!date) return <li key={`pad-${i}`} className="pf-pad" aria-hidden="true" />;
          const rows = byDate.get(date) ?? [];
          const day = Number(date.slice(8));
          if (rows.length === 0)
            return (
              <li key={date}>
                <time dateTime={date}>{day}</time>
              </li>
            );
          const [lead] = rows;
          return (
            <li key={date} className="pf-occupied">
              <Link href={calendarHref(filters, { files: date })} className="pf-day" data-date={date}>
                <span className="sr-only">
                  {dateLabel(date)}, {plural(rows.length, "referral")}. View case files
                </span>
                <time dateTime={date} aria-hidden="true">
                  {day}
                </time>
                <span className="pf-entry" aria-hidden="true">
                  <span className="pf-entry-id">{shortId(lead.c.id)}</span>
                  {task(lead)}
                </span>
                {rows.length > 1 && (
                  <small className="pf-overflow" aria-hidden="true">
                    +{rows.length - 1} more
                  </small>
                )}
                <span className="pf-day-count" aria-hidden="true">
                  {rows.length}
                </span>
              </Link>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

/** The case files of a date or a month: one folder per referral. Opening a folder keeps the scope, search and page. */
export function CaseFiles({ rows, view, filters }: { rows: Row[]; view: CalendarView; filters: CaseFilters }) {
  return (
    <ul className="pf-files" aria-label="Case files">
      {rows.map((r) => {
        const l = lane(r);
        return (
          <li key={r.c.id}>
            <Link href={calendarHref(filters, { ...view, open: r.c.id })} className="pf-file" data-pf-case={r.c.id}>
              <span className="pf-file-id">
                {shortId(r.c.id)}
                {r.it.named && <span className="pf-file-name"> · {r.it.name}</span>}
              </span>
              <ChevronRight className="pf-file-caret" aria-hidden="true" />
              <strong>{task(r)}</strong>
              {r.ob && (
                <span className="pf-file-note" data-testid="obligation">
                  {r.ob.kind === "ldct_tracking" && r.ob.child
                    ? `Follow-up tracked on ${shortId(r.ob.child)}`
                    : `${obligationWords(r.ob).status.replace(/^Closed · /, "")} · ${obligationWords(r.ob).next}`}
                </span>
              )}
              <span className="pf-meta">
                <span>{r.it.owner}</span>
                <span className={cn("pf-due", !r.c.closed && r.c.overdue && "text-negative")}>
                  {r.ob?.kind === "dr_recall" ? recallDueWords(r.ob) : `Due ${dueColumnWords(r.c)}`}
                </span>
                <span className="pf-status" data-tone={l.tone}>
                  <i aria-hidden="true" />
                  {l.words}
                </span>
              </span>
            </Link>
          </li>
        );
      })}
    </ul>
  );
}
