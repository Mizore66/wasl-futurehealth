import type { RecoveryCandidate, RecoveryList, RecoverySlot } from "@/lib/api/recovery-schema";

/** Candidates shown per page; a slot can rank more than a thousand, which the browser cannot render usefully at once. */
export const CANDIDATE_PAGE = 25;

/** The released slots in day order (then unit and id), so the list reads chronologically. */
export function slotsByDay<T extends Pick<RecoveryList["slots"][number], "day" | "slot_id">>(slots: readonly T[]): T[] {
  return [...slots].sort((a, b) => a.day - b.day || a.slot_id.localeCompare(b.slot_id));
}

/**
 * Eligible candidates in rank order and excluded ones after them, each cut to what the coordinator asked to see.
 * `hiddenEligible` and `hiddenExcluded` say how many more each list holds.
 */
export function candidatePages(candidates: readonly RecoveryCandidate[], eligibleShown: number, excludedShown: number) {
  const eligible = candidates.filter((c) => c.eligible);
  const excluded = candidates.filter((c) => !c.eligible);
  return {
    eligible: eligible.slice(0, eligibleShown),
    excluded: excluded.slice(0, excludedShown),
    hiddenEligible: Math.max(0, eligible.length - eligibleShown),
    hiddenExcluded: Math.max(0, excluded.length - excludedShown),
    totalExcluded: excluded.length,
  };
}

export const words = (value: string) => value.replaceAll("_", " ");
export const sentence = (value: string) => (value ? value[0].toUpperCase() + value.slice(1) : value);

/**
 * The candidate the time window describes: the case the slot is assigned to once a backfill is confirmed,
 * otherwise the top-ranked eligible case. Null when nobody is eligible and nothing is assigned.
 */
export function focalCandidate(slot: Pick<RecoverySlot, "candidates" | "backfill">): RecoveryCandidate | null {
  if (slot.backfill) return slot.candidates.find((c) => c.case_id === slot.backfill?.case_id) ?? null;
  return slot.candidates.find((c) => c.eligible) ?? null;
}

export type Align = "start" | "center" | "end";
const align = (position: number): Align => (position < 20 ? "start" : position > 80 ? "end" : "center");
const clamp = (value: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, value));

/**
 * Positions (percent of the axis) for the requested day, the slot and the deadline. Requested and Due sit at the
 * two ends so their labels below the axis never meet; the slot label sits above the axis and is kept clear of the
 * end markers. A slot after the deadline moves to the right end and the deadline keeps room for its own label.
 */
export function windowGeometry(requested: number, slotDay: number, due: number) {
  const gap = due - slotDay;
  const start = 0;
  let end = 100;
  let slot: number;
  if (slotDay > due) {
    slot = 100;
    end = clamp(((due - requested) / Math.max(1, slotDay - requested)) * 100, 35, 82);
  } else if (due === requested) {
    slot = 50;
  } else {
    slot = clamp(((slotDay - requested) / (due - requested)) * 100, 6, 94);
  }
  return {
    start,
    slot,
    end,
    gap,
    slotAlign: align(slot),
    endAlign: slotDay > due ? align(end) : ("end" as Align),
    gapText: gap === 0 ? "Due that day" : gap > 0 ? `${gap} ${gap === 1 ? "day" : "days"}` : `${-gap} ${gap === -1 ? "day" : "days"} past due`,
  };
}

/** Passing checks a coordinator reads as matches, in display order. Other passes stay in the full check list. */
const MATCHES: Record<string, string> = {
  programme_match: "Programme match",
  procedure_match: "Procedure match",
  region_match: "Region match",
  arabic_preference: "Arabic available",
  after_hours_preference: "After hours available",
  female_preference: "Female endoscopist available",
};

const WARNINGS: Record<string, string> = {
  arabic_not_available: "Arabic not available",
  after_hours_not_available: "After hours not available",
  female_endoscopist_not_available: "Female endoscopist not available",
  past_target: "After the due day",
};

const EXCLUSIONS: Record<string, string> = {
  source_case_for_own_release: "Source case for this release",
  returned_referral_unresolved: "Returned referral unresolved",
  case_already_backfilled: "Already backfilled",
  case_closed_inactive_or_held: "Case closed, inactive or held",
  case_has_active_coordinator_reservation: "Has an active coordinator booking",
  slot_stale_or_past: "Slot stale or past",
  insufficient_prep_lead: "Not enough preparation time",
  before_repeat_window: "Before the repeat window",
  region_mismatch_without_travel: "Different region, travel not agreed",
  female_endoscopist_required: "Needs a female endoscopist",
  ramadan_conflict: "Falls in Ramadan",
  protected_female_list_outside_release_window: "Protected female list",
  non_regular_or_exhausted_capacity: "No regular capacity left",
  procedure_mismatch: "Different procedure",
  same_programme: "Different programme",
  no_simulated_capacity: "Programme has no simulated capacity",
};

export function candidateMatches(candidate: RecoveryCandidate) {
  const passed = new Set(candidate.checks.filter((c) => c.result === "pass").map((c) => c.code));
  const matches = Object.keys(MATCHES).filter((code) => passed.has(code)).map((code) => MATCHES[code]);
  const warnings = candidate.checks.filter((c) => c.result === "warning").map((c) => WARNINGS[c.reason_code] ?? sentence(words(c.reason_code)));
  return { matches, warnings };
}

/** Why an excluded candidate cannot take the slot, from its failed hard checks. */
export function exclusionReasons(candidate: RecoveryCandidate): string[] {
  const failed = candidate.checks.filter((c) => c.result === "fail");
  return failed.length ? failed.map((c) => EXCLUSIONS[c.reason_code] ?? sentence(words(c.reason_code))) : ["A failed check"];
}

export type OutreachTone = "held" | "recorded" | "cancelled" | "allowed";

/**
 * Outreach is decided apart from assignment: a confirmed backfill assigns the slot, and outreach stays held while the
 * assigned (or top) case lacks contact permission or a confirmed second identifier.
 */
export function outreachState(candidate: RecoveryCandidate | null, slotOutreach: RecoverySlot["outreach"]): { tone: OutreachTone; title: string } {
  if (!candidate || !candidate.outreach.allowed) return { tone: "held", title: "Outreach held" };
  if (slotOutreach.status === "recorded_not_dispatched") return { tone: "recorded", title: "Outreach recorded, not sent" };
  if (slotOutreach.status === "cancelled_consent") return { tone: "cancelled", title: "Outreach cancelled" };
  return { tone: "allowed", title: "Outreach allowed" };
}

/** `colonoscopy` → `Colonoscopy`. */
export const procedureLabel = (procedure: string) => sentence(words(procedure));

/** The no-show projection basis read as a release reason. */
export const releaseReason = (basis: string) => (basis.startsWith("no_show") ? "Missed appointment" : sentence(words(basis)));
