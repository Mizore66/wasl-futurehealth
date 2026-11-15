import type { CaseSummary } from "@/lib/api/schema";

/** The DoH colonoscopy standard: days from referral. */
export const STANDARD_DAYS = 31;

/** The programme's rule interval; annual LDCT has no overdue clock but still has a 365-day next-screen interval. */
export function caseDeadlineDays(c: CaseSummary): number {
  return intervalDays(c) || STANDARD_DAYS;
}

const intervalDays = (c: CaseSummary) => c.deadline_day || c.ldct?.rec?.due_days || c.ifhas?.rec?.deadline_days;

/** Terminal states where the screening finding itself needs no procedure, so there is no procedure clock. */
const NO_ACTION_STATES: ReadonlySet<string> = new Set(["no_followup_needed", "no_referral_needed", "no_recall_needed"]);
export const isNoAction = (c: CaseSummary) => NO_ACTION_STATES.has(c.state);

/** Days to the next routine screen for a no-action resolution, or null when the rule sets no interval. */
export function nextScreenDays(c: CaseSummary): number | null {
  return isNoAction(c) ? intervalDays(c) || null : null;
}

/**
 * Days from referral the case's overdue clock allows, or null when its rule sets only a routine interval. Mirrors the
 * API: an annual LDCT or a routine IFHAS result is never overdue.
 */
export function overdueClockDays(c: CaseSummary): number | null {
  if (c.ldct?.rec?.action === "annual" || c.ifhas?.rec?.action === "routine") return null;
  return caseDeadlineDays(c);
}

type Clocked = { days: number; deadline: number | null };

const daysLeft = (r: Clocked) => (r.deadline === null ? Infinity : r.deadline - r.days);

export function clockWords(r: Clocked): { over: boolean; short: string; long: string } {
  if (r.deadline === null) return { over: false, short: "routine interval", long: "Routine interval · no overdue clock" };
  const left = r.deadline - r.days;
  return left < 0
    ? { over: true, short: `${-left} over`, long: `${-left} days over ${r.deadline}` }
    : { over: false, short: `${left} left`, long: `${left} days left of ${r.deadline}` };
}

/** Overdue first, then fewest days left on the case's own clock, routine intervals last. Snoozed rows sink. */
export function byUrgency(a: Clocked & { snoozed?: boolean }, b: Clocked & { snoozed?: boolean }): number {
  return Number(!!a.snoozed) - Number(!!b.snoozed) || daysLeft(a) - daysLeft(b) || b.days - a.days;
}
