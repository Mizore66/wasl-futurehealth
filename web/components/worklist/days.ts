/** The DoH colonoscopy standard: days from referral. */
export const STANDARD_DAYS = 31;

/** Over-31 first, then the longest wait. Snoozed rows sink. */
export function byUrgency(a: { days: number; snoozed?: boolean }, b: { days: number; snoozed?: boolean }): number {
  return (
    Number(!!a.snoozed) - Number(!!b.snoozed) ||
    Number(b.days > STANDARD_DAYS) - Number(a.days > STANDARD_DAYS) ||
    b.days - a.days
  );
}
