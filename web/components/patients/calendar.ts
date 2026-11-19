import type { CaseSummary } from "@/lib/api/schema";
import { dayToDate } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { filtersHref, type CaseFilters } from "@/components/worklist/filters";

/*
 * The Referral calendar on /app/patients: referrals grouped by the UTC date they were received, one month at a time.
 * Everything the calendar shows (month, chosen date, open folder, search) lives in the URL, so Back, a reload and a
 * shared link all land on the same view.
 */

/** `?files=` is a date (YYYY-MM-DD) or a whole month (YYYY-MM); `?month=` is the month the grid shows. */
export type CalendarView = { month?: string; files?: string; open?: string; page?: number };

const MONTH = /^\d{4}-(0[1-9]|1[0-2])$/;
const DATE = /^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$/;
const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
const SHORT = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
export const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

type SearchParams = Record<string, string | string[] | undefined>;
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

/** Malformed values are dropped, so a stale or hand-edited link shows the calendar rather than an error. */
export function parseCalendarView(sp: SearchParams): CalendarView {
  const month = one(sp.month);
  const files = one(sp.files);
  const open = one(sp.open)?.trim();
  return {
    ...(month && MONTH.test(month) && { month }),
    ...(files && (MONTH.test(files) || DATE.test(files)) && { files }),
    ...(open && { open }),
  };
}

/** The date a referral was received, as YYYY-MM-DD in UTC (the same instant `formatDay` writes). */
export const referralDate = (c: Pick<CaseSummary, "referral_day">) => dayToDate(c.referral_day).toISOString().slice(0, 10);

/** "May 2026" */
export function monthLabel(month: string): string {
  const [y, m] = month.split("-").map(Number);
  return `${MONTHS[m - 1]} ${y}`;
}

/** "2 May 2026" */
export function dateLabel(date: string): string {
  const [y, m, d] = date.split("-").map(Number);
  return `${d} ${SHORT[m - 1]} ${y}`;
}

/** The heading of a files scope: a date or a month. */
export const scopeLabel = (scope: string) => (scope.length === 10 ? dateLabel(scope) : monthLabel(scope));

export const plural = (n: number, unit: string) => `${n} ${unit}${n === 1 ? "" : "s"}`;

/** The calendar and files links: the filters first (as /app/patients writes them), then the calendar state. */
export function calendarHref(f: CaseFilters, v: CalendarView = {}): string {
  const base = filtersHref("/app/patients", f);
  const params = new URLSearchParams(base.includes("?") ? base.slice(base.indexOf("?") + 1) : "");
  if (v.month) params.set("month", v.month);
  if (v.files) params.set("files", v.files);
  if (v.open) params.set("open", v.open);
  if (v.page && v.page > 1) params.set("page", String(v.page));
  const s = params.toString();
  return s ? `/app/patients?${s}` : "/app/patients";
}

/**
 * The search box ("Find a case, task or owner"): every word must appear in the case id, the task or status words, the
 * owner or the patient's (simulated) name. Case-insensitive; an id search such as `1001` still finds AHS-sr-1001.
 */
export function searchMatches(q: string | undefined, fields: (string | undefined)[]): boolean {
  const words = (q ?? "").toLowerCase().split(/\s+/).filter(Boolean);
  if (words.length === 0) return true;
  const hay = fields.filter(Boolean).join(" ").toLowerCase();
  return words.every((w) => hay.includes(w));
}

export const searchFields = (c: CaseSummary, words: { status: string; next: string; owner: string; name: string; nameAr?: string }) => [
  c.id,
  shortId(c.id),
  words.status,
  words.next,
  words.owner,
  words.name,
  words.nameAr,
];

export type DayGroup<T> = { date: string; items: T[] };

/** Items grouped by referral date, dates in order; within a date, the input order is kept. */
export function byDate<T>(items: T[], dateOf: (t: T) => string): Map<string, T[]> {
  const out = new Map<string, T[]>();
  for (const t of items) {
    const d = dateOf(t);
    const list = out.get(d);
    if (list) list.push(t);
    else out.set(d, [t]);
  }
  return out;
}

/** The months that hold at least one referral, oldest first. */
export const monthsWith = (dates: Iterable<string>) => [...new Set([...dates].map((d) => d.slice(0, 7)))].sort();

/**
 * Which month the grid shows: the one asked for when it holds referrals, else the month of an open files scope, else
 * the latest month that holds any (so a search jumps to where its matches are). Undefined when nothing matches.
 */
export function chooseMonth(months: string[], v: CalendarView): string | undefined {
  if (v.month && months.includes(v.month)) return v.month;
  if (v.files && months.includes(v.files.slice(0, 7))) return v.files.slice(0, 7);
  return months.at(-1);
}

/** Monday-first cells for a month: `null` pads the first and last weeks. */
export function monthCells(month: string): (string | null)[] {
  const [y, m] = month.split("-").map(Number);
  const first = new Date(Date.UTC(y, m - 1, 1));
  const length = new Date(Date.UTC(y, m, 0)).getUTCDate();
  const offset = (first.getUTCDay() + 6) % 7;
  const cells: (string | null)[] = [];
  for (let n = 0; n < Math.ceil((offset + length) / 7) * 7; n++) {
    const day = n - offset + 1;
    cells.push(day < 1 || day > length ? null : `${month}-${String(day).padStart(2, "0")}`);
  }
  return cells;
}
