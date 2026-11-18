import { formatDay } from "./day";

/**
 * One staff format for wall-clock due and lease times: Abu Dhabi local time with a relative hint,
 * "Thu 19 Nov, 20:02 Dubai time (tomorrow)". Asia/Dubai is UTC+4 all year (no daylight saving), so the offset is
 * applied directly rather than through Intl, whose month names differ between ICU builds (see lib/domain/day.ts).
 */
export const STAFF_TIME_ZONE = "Asia/Dubai";
const OFFSET_MS = 4 * 60 * 60 * 1000;
const DAY_MS = 86_400_000;
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

const pad = (n: number) => String(n).padStart(2, "0");
const localDay = (ms: number) => Math.floor((ms + OFFSET_MS) / DAY_MS);

/** Whole Abu Dhabi calendar days from `now` to `at`, in words. Day granularity keeps server and browser renders equal. */
export function relativeDayHint(at: Date, now: Date): string {
  const days = localDay(at.getTime()) - localDay(now.getTime());
  if (days === 0) return "today";
  if (days === 1) return "tomorrow";
  if (days === -1) return "yesterday";
  return days > 0 ? `in ${days} days` : `${-days} days ago`;
}

/** "Thu 19 Nov, 20:02 Dubai time" for an ISO instant; an unparseable value is returned as it came. */
export function staffLocalTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const l = new Date(d.getTime() + OFFSET_MS);
  return `${WEEKDAYS[l.getUTCDay()]} ${l.getUTCDate()} ${MONTHS[l.getUTCMonth()]}, ${pad(l.getUTCHours())}:${pad(l.getUTCMinutes())} Dubai time`;
}

/** A due or lease time for staff screens: local time plus the relative hint, "Thu 19 Nov, 20:02 Dubai time (tomorrow)". */
export function staffDueTime(iso: string, now: Date = new Date()): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return `${staffLocalTime(iso)} (${relativeDayHint(d, now)})`;
}

/** The simulation's run clock, labelled so it is never read as today's date: "run day 95, 4 Aug (simulated date)". */
export function simulatedRunDate(day: number): string {
  return `run day ${Math.floor(day)}, ${formatDay(day)} (simulated date)`;
}
