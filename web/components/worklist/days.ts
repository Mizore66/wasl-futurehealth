import type { CaseSummary } from "@/lib/api/schema";

/** The DoH colonoscopy standard: days from referral. */
export const STANDARD_DAYS = 31;

/** The programme's rule interval; annual LDCT has no overdue clock but still has a 365-day next-screen interval. */
export function caseDeadlineDays(c: CaseSummary): number {
  return c.deadline_day || c.ldct?.rec?.due_days || c.ifhas?.rec?.deadline_days || STANDARD_DAYS;
}

/** Over-31 first, then the longest wait. Snoozed rows sink. */
export function byUrgency(a: { days: number; snoozed?: boolean }, b: { days: number; snoozed?: boolean }): number {
  return (
    Number(!!a.snoozed) - Number(!!b.snoozed) ||
    Number(b.days > STANDARD_DAYS) - Number(a.days > STANDARD_DAYS) ||
    b.days - a.days
  );
}
