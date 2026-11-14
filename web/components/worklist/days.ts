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

/** Over-31 first, then the longest wait. Snoozed rows sink. */
export function byUrgency(a: { days: number; snoozed?: boolean }, b: { days: number; snoozed?: boolean }): number {
  return (
    Number(!!a.snoozed) - Number(!!b.snoozed) ||
    Number(b.days > STANDARD_DAYS) - Number(a.days > STANDARD_DAYS) ||
    b.days - a.days
  );
}
