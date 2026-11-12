import { formatWeekdayDay } from "@/lib/domain/day";

/** Weekday, day and month in UTC (the timezone the API stores `until` in), from the app's month table: "Thu 3 Sep". */
export const WEEKDAY = { format: formatWeekdayDay };

/**
 * A wall-clock RFC 3339 time `days` after `nowMs`, to the second. The API checks a snooze `until` against its own wall
 * clock, not the simulation's run clock, so this is never derived from the run day.
 */
export function fromNow(days: number, nowMs: number = Date.now()): string {
  return new Date(nowMs + days * 86_400_000).toISOString().replace(/\.\d{3}Z$/, "Z");
}

export type CallbackChoice = { label: string; action: { action: "snooze"; until: string; reason: string } };

/**
 * One "Call back on…" choice. `until` is computed once; the visible label (weekday, day, month) is formatted from that
 * same `until`, and the same `until` is what is saved. The label can therefore never name a different day from the one
 * the ledger records (issue #37).
 */
export function callbackChoice(days: number, nowMs: number = Date.now()): CallbackChoice {
  const until = fromNow(days, nowMs);
  const label = WEEKDAY.format(new Date(until));
  return { label, action: { action: "snooze", until, reason: `Call back ${label}` } };
}
