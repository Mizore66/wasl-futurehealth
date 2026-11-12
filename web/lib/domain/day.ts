/** API days are fractional days since the cohort epoch, 1 May 2026 (UTC). */
export const COHORT_EPOCH_MS = Date.UTC(2026, 4, 1);
const DAY_MS = 86_400_000;

export function dayToDate(day: number): Date {
  return new Date(COHORT_EPOCH_MS + Math.round(day * DAY_MS));
}

/**
 * The app's one month table. formatDay, formatDayYear and formatUtcInstant all use it rather than Intl, because Node's ICU
 * formats en-GB September as "Sept" and the app writes "Sep" everywhere.
 */
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/** "14 Sep" style label for an API day. */
export function formatDay(day: number): string {
  const d = dayToDate(day);
  return `${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]}`;
}

/** "14 Sep 2027": for windows and due dates that can fall in another year. */
export function formatDayYear(day: number): string {
  const d = dayToDate(day);
  return `${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]} ${d.getUTCFullYear()}`;
}

/**
 * "31 Oct 2026, 23:47 UTC" from an ISO instant; the raw string if it does not parse. Always UTC, so the server render and
 * the browser agree whatever the viewer's time zone.
 */
export function formatUtcInstant(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const hh = String(d.getUTCHours()).padStart(2, "0");
  const mm = String(d.getUTCMinutes()).padStart(2, "0");
  return `${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]} ${d.getUTCFullYear()}, ${hh}:${mm} UTC`;
}
