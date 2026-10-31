/** API days are fractional days since the cohort epoch, 1 May 2026 (UTC). */
export const COHORT_EPOCH_MS = Date.UTC(2026, 4, 1);
const DAY_MS = 86_400_000;

export function dayToDate(day: number): Date {
  return new Date(COHORT_EPOCH_MS + Math.round(day * DAY_MS));
}

const SHORT = new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "short", timeZone: "UTC" });

/** "14 Sep" style label for an API day. */
export function formatDay(day: number): string {
  return SHORT.format(dayToDate(day));
}
