import { dayToDate } from "./day";

/** Business days the referrer has to act on a returned referral: assumptions mirrored from the NHS England standard, not DoH requirements. */
const BUSINESS_DAYS = { urgent: 2, routine: 5 } as const;

/** UAE private-sector public holidays in the simulated year; keep in step with `uaeHolidays2026` in pipeline/handoff.go. */
const UAE_HOLIDAYS_2026 = new Set([
  "2026-01-01",
  "2026-05-26",
  "2026-05-27",
  "2026-05-28",
  "2026-05-29",
  "2026-06-15",
  "2026-08-28",
  "2026-12-01",
  "2026-12-02",
  "2026-12-03",
]);

/** `n` UAE business days after API day `day` (Monday to Friday, holidays skipped); pipeline `addBD`. */
export function addBusinessDays(day: number, n: number): number {
  let t = day;
  while (n > 0) {
    t++;
    const d = dayToDate(t);
    const weekday = d.getUTCDay();
    if (weekday !== 0 && weekday !== 6 && !UAE_HOLIDAYS_2026.has(d.toISOString().slice(0, 10))) n--;
  }
  return t;
}

/** Pipeline `handoffUrgent`: a FIT+ or any non-cervical referral is suspected cancer; cervical is urgent by grade. */
export function handoffUrgent(programme: string, priority: string): boolean {
  return programme !== "cervical" || priority === "urgent";
}

/** The day the referrer must act by after a return decided on `decidedDay`; pipeline `handoffClock`. */
export function handoffActBy(decidedDay: number, urgent: boolean): number {
  return addBusinessDays(decidedDay, urgent ? BUSINESS_DAYS.urgent : BUSINESS_DAYS.routine);
}
