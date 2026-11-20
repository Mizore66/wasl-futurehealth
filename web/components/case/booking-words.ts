import type { EligibleBookingSlot } from "@/lib/api/schema";

/** A run day as a whole day number ("Day 31"), never a long decimal. */
export function dayNumber(day: number | undefined): string {
  return day === undefined || !Number.isFinite(day) ? "not recorded" : String(Math.floor(day));
}

/**
 * Why no booking button is offered when the case is otherwise eligible: every catalogue slot has elapsed on the run clock,
 * or the suggested slot itself is past or taken. Empty when a slot can be reserved (the buttons then explain themselves).
 */
export function slotUnavailableWords(
  candidates: Pick<EligibleBookingSlot, "slot_id" | "availability">[],
  suggestedSlotId: string | undefined,
  reservable: boolean | undefined,
): string {
  if (candidates.length === 0) return "";
  const available = candidates.filter((s) => s.availability === "available");
  if (available.length === 0) {
    return candidates.every((s) => s.availability === "elapsed")
      ? "Every eligible slot has already passed, so none can be booked. Arrange a date with the unit directly."
      : "No eligible slot is free: the remaining slots are past or already reserved.";
  }
  const suggested = candidates.find((s) => s.slot_id === suggestedSlotId);
  if (suggested && (suggested.availability !== "available" || reservable === false)) {
    return suggested.availability === "elapsed"
      ? "The suggested slot is already past, so it cannot be accepted. Choose another eligible slot."
      : "The suggested slot cannot be reserved now. Choose another eligible slot.";
  }
  return "";
}
