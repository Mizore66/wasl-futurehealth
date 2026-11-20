import type { RecoveryCandidate, RecoveryList, RecoverySlot } from "@/lib/api/recovery-schema";

/** Candidates per page; a slot can rank more than a thousand, which the browser cannot render usefully at once. */
export const CANDIDATE_PAGE = 25;

export type SlotStatus = RecoveryList["slots"][number]["status"];
export type SlotFilter = SlotStatus | "all";
export const SLOT_FILTERS: readonly SlotFilter[] = ["open", "filled", "stale", "all"];
export type CandidateSet = "eligible" | "excluded";

/**
 * Everything on the board a reload must keep. `slot` is read on the server, which fetches that slot's detail; the
 * rest is client state mirrored into the URL. Null means not chosen, and the board derives a default.
 */
export type RecoveryView = {
  slot: string | null;
  filter: SlotFilter | null;
  page: number | null;
  case: string | null;
  set: CandidateSet | null;
  cpage: number | null;
};

export const NO_VIEW: RecoveryView = { slot: null, filter: null, page: null, case: null, set: null, cpage: null };

const one = (value: string | string[] | undefined) => (typeof value === "string" && value !== "" ? value : null);
const pageNumber = (value: string | string[] | undefined) => {
  const n = Number(one(value));
  return Number.isInteger(n) && n > 0 ? n : null;
};

export function parseRecoveryView(query: Record<string, string | string[] | undefined>): RecoveryView {
  const filter = one(query.status);
  const set = one(query.set);
  return {
    slot: one(query.slot),
    filter: SLOT_FILTERS.find((f) => f === filter) ?? null,
    page: pageNumber(query.page),
    case: one(query.case),
    set: set === "eligible" || set === "excluded" ? set : null,
    cpage: pageNumber(query.cpage),
  };
}

export function recoveryHref(view: RecoveryView): string {
  const params = new URLSearchParams();
  if (view.slot) params.set("slot", view.slot);
  if (view.filter) params.set("status", view.filter);
  if (view.page) params.set("page", String(view.page));
  if (view.case) params.set("case", view.case);
  if (view.set) params.set("set", view.set);
  if (view.cpage) params.set("cpage", String(view.cpage));
  const query = params.toString();
  return query ? `/app/recovery?${query}` : "/app/recovery";
}

type ClientParam = "status" | "page" | "case" | "set" | "cpage";

/** Mirrors client state into the address bar without a server round trip; Next keeps its router in step. */
export function replaceRecoveryParams(patch: Partial<Record<ClientParam, string | number | null>>) {
  const url = new URL(window.location.href);
  for (const [key, value] of Object.entries(patch)) {
    if (value === null || value === undefined) url.searchParams.delete(key);
    else url.searchParams.set(key, String(value));
  }
  window.history.replaceState(null, "", url);
}

/**
 * Scrolls a panel (positioned, so it is the row's offsetParent) until the row shows below its sticky heading.
 * Not scrollIntoView: below lg the panels are part of the page, and that would scroll the whole page too.
 */
export function revealIn(panel: HTMLElement | null, selector: string, headroom = 0) {
  const row = panel?.querySelector<HTMLElement>(selector);
  if (!panel || !row) return;
  const top = row.offsetTop - headroom;
  if (top < panel.scrollTop || row.offsetTop + row.offsetHeight > panel.scrollTop + panel.clientHeight) panel.scrollTop = top;
}

/** The released slots in day order (then unit and id), so the list reads chronologically. */
export function slotsByDay<T extends Pick<RecoveryList["slots"][number], "day" | "slot_id">>(slots: readonly T[]): T[] {
  return [...slots].sort((a, b) => a.day - b.day || a.slot_id.localeCompare(b.slot_id));
}

/** A slot list row: the summary, the `id` the pager keys on, and the days from the run's day to the slot. */
export type SlotRow = RecoveryList["slots"][number] & { id: string; lead: number };

export function slotRows(list: Pick<RecoveryList, "slots" | "as_of_day">): SlotRow[] {
  return slotsByDay(list.slots).map((slot) => ({ ...slot, id: slot.slot_id, lead: Math.floor(slot.day) - Math.floor(list.as_of_day) }));
}

const days = (n: number) => `${n} ${n === 1 ? "day" : "days"}`;

export function leadWords(lead: number): string {
  if (lead === 0) return "Today";
  return lead > 0 ? `In ${days(lead)}` : `${days(-lead)} ago`;
}

/** The slot day against a candidate's due day. */
export function gapWords(slotDay: number, due: number): { text: string; past: boolean } {
  const gap = Math.floor(due) - Math.floor(slotDay);
  return { text: gap === 0 ? "On the due day" : gap > 0 ? `${days(gap)} before due` : `${days(-gap)} past due`, past: gap < 0 };
}

/**
 * A slot's candidates as the board shows them: the assigned case (if any) pinned on its own, then everyone else as
 * ranked eligible and excluded sets in API order.
 */
export function candidateSets(slot: Pick<RecoverySlot, "candidates" | "backfill">) {
  const assignedId = slot.backfill?.case_id;
  const assigned = assignedId ? (slot.candidates.find((c) => c.case_id === assignedId) ?? null) : null;
  const others = slot.candidates.filter((c) => c.case_id !== assignedId).map((c) => ({ ...c, id: c.case_id }));
  return { assigned, eligible: others.filter((c) => c.eligible), excluded: others.filter((c) => !c.eligible) };
}

export const words = (value: string) => value.replaceAll("_", " ");
export const sentence = (value: string) => (value ? value[0].toUpperCase() + value.slice(1) : value);

/**
 * The candidate the slot is measured against until the coordinator picks another: the case the slot is assigned to
 * once a backfill is confirmed, otherwise the top-ranked eligible case. Null when nobody is eligible or assigned.
 */
export function focalCandidate(slot: Pick<RecoverySlot, "candidates" | "backfill">): RecoveryCandidate | null {
  if (slot.backfill) return slot.candidates.find((c) => c.case_id === slot.backfill?.case_id) ?? null;
  return slot.candidates.find((c) => c.eligible) ?? null;
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
 * case lacks contact permission or a confirmed second identifier. The slot's outreach record belongs to the assigned
 * case only, so pass it for that case and `null` for anyone else.
 */
export function outreachState(candidate: RecoveryCandidate | null, slotOutreach: RecoverySlot["outreach"] | null): { tone: OutreachTone; title: string } {
  if (!candidate || !candidate.outreach.allowed) return { tone: "held", title: "Outreach held" };
  if (slotOutreach?.status === "recorded_not_dispatched") return { tone: "recorded", title: "Outreach recorded, not sent" };
  if (slotOutreach?.status === "cancelled_consent") return { tone: "cancelled", title: "Outreach cancelled" };
  return { tone: "allowed", title: "Outreach allowed" };
}

/** `colonoscopy` → `Colonoscopy`. */
export const procedureLabel = (procedure: string) => sentence(words(procedure));

/** The no-show projection basis read as a release reason. */
export const releaseReason = (basis: string) => (basis.startsWith("no_show") ? "Missed appointment" : sentence(words(basis)));
