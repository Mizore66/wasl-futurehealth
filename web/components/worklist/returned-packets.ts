import type { CaseSummary, CurrentDecision, HandoffStatus, ReferralInfo, ReferrerResponseCode } from "@/lib/api/schema";
import type { ReferrerAnswer } from "@/app/app/referrals/actions";
import { shortId } from "@/lib/domain/ids";
import { personForLoop } from "@/lib/people";
import { INFO_LABELS, REASON_LABELS, REFERRER_ANSWER_LABELS, packetReasonWords, placeName, referrerAnswerWords } from "./referrals";

/** A resolving answer: everything but acknowledge, which leaves the packet waiting on the GP. */
export type Disposition = Exclude<ReferrerResponseCode, "acknowledge">;
export const DISPOSITION_LABELS: Record<Disposition, string> = REFERRER_ANSWER_LABELS;

/** A referral the receiving unit returned to its referrer, as the responsible referrer answers it from the case ledger. */
export type ReturnedPacket = {
  id: string;
  name: string;
  short: string;
  /** The case's `api_overlay.head`, which the answer must name. */
  head: string;
  /** The packet the answer is bound to: the unit return's ledger event, or the canonical rejected-referral event. */
  packet: string;
  /** unit_return: the receiving unit answered through the API; canonical: the engine's packet on the latest decision. */
  source: "unit_return" | "canonical";
  /** The unit that returned it, in words, and the unit id a re-referral goes back to. */
  unit: string;
  unitId: string;
  reason: string;
  /** What the loop still needs and the unit's advice, from the canonical packet; null when not read or not in it. */
  recommendation: string | null;
  advice: string | null;
  urgent: boolean;
  infoRequested: ReferralInfo[];
  /** Other units offering the procedure, for a redirect. Empty for lung work-up and DR: one unit each in the network. */
  others: { id: string; label: string }[];
  actBy: number;
  status: HandoffStatus;
  acknowledged: boolean;
  /** The referrer's resolving answer in words, or null while the hand-off waits on the referrer. */
  answer: string | null;
};

/**
 * The returned packet of an open case, from its `api_overlay.referral_handoff` projection. `current` (the case detail's
 * canonical decision) adds the recommendation and advice of a canonical packet; it is never needed for the binding.
 */
export function returnedPacket(c: CaseSummary, current: CurrentDecision | null = null): ReturnedPacket | null {
  const ov = c.api_overlay;
  const h = ov?.referral_handoff;
  if (!ov || !h?.event || c.closed) return null;
  const r = h.referrer_response;
  const source = h.source ?? "unit_return";
  const canonical = source === "canonical" && current?.handoff?.referral_id === h.event ? current.handoff : null;
  return {
    id: c.id,
    name: personForLoop(c.id)?.display_name ?? shortId(c.id),
    short: shortId(c.id),
    head: ov.head,
    packet: h.event,
    source,
    unit: placeName(h.decided_by),
    unitId: h.decided_by,
    reason: source === "unit_return" && ov.referral_response?.reason ? REASON_LABELS[ov.referral_response.reason] : packetReasonWords(h.reason_code),
    recommendation: canonical?.recommendation ?? null,
    advice: canonical?.advice_to_referrer ?? null,
    urgent: h.clinical_priority === "urgent",
    infoRequested: h.info_requested,
    others: h.destinations.filter((d) => d !== h.decided_by).map((id) => ({ id, label: placeName(id) })),
    actBy: h.act_by,
    status: h.status,
    acknowledged: !!h.acknowledgement,
    answer: r ? referrerAnswerWords(r) : h.status === "referrer_acted" ? "The canonical packet already records the referrer's action" : null,
  };
}

export type Draft = { response: Disposition | null; info: Partial<Record<ReferralInfo, string>>; destination: string; reason: string };
export const EMPTY_DRAFT: Draft = { response: null, info: {}, destination: "", reason: "" };

/** The request a draft makes, or what it still lacks. Mirrors the API's rules so an incomplete answer is never sent. */
export function referrerRequest(p: ReturnedPacket, d: Draft): { ok: true; request: ReferrerAnswer } | { ok: false; missing: string } {
  if (d.response === null) return { ok: false, missing: "Choose an answer." };
  switch (d.response) {
    case "supply_info": {
      if (!p.infoRequested.length) return { ok: false, missing: "The unit asked for no information." };
      const blank = p.infoRequested.filter((i) => !d.info[i]?.trim());
      if (blank.length) return { ok: false, missing: `Fill in: ${blank.map((i) => INFO_LABELS[i]).join(", ")}.` };
      return { ok: true, request: { response: "supply_info", supplied_info: Object.fromEntries(p.infoRequested.map((i) => [i, d.info[i]!.trim()])) } };
    }
    case "re_refer":
      return { ok: true, request: { response: "re_refer", destination: p.unitId } };
    case "redirect":
      return p.others.some((o) => o.id === d.destination) ? { ok: true, request: { response: "redirect", destination: d.destination } } : { ok: false, missing: "Choose the unit to refer to." };
    case "manage_in_primary_care":
    case "close":
      return d.reason.trim() ? { ok: true, request: { response: d.response, reason: d.reason.trim() } } : { ok: false, missing: "Give the reason." };
  }
}
