import type { CaseSummary, HandoffStatus, ReferralInfo, ReferrerResponseCode } from "@/lib/api/schema";
import type { ReferrerAnswer } from "@/app/app/referrals/actions";
import { formatDay } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { personForLoop } from "@/lib/people";
import { providerLabel } from "./model";
import { INFO_LABELS, REASON_LABELS } from "./referrals";

/** A resolving answer: everything but acknowledge, which leaves the packet waiting on the GP. */
export type Disposition = Exclude<ReferrerResponseCode, "acknowledge">;
export const DISPOSITION_LABELS: Record<Disposition, string> = {
  supply_info: "Send the information requested",
  re_refer: "Re-refer to the same unit",
  redirect: "Refer to another unit",
  manage_in_primary_care: "Manage in primary care",
  close: "Close the referral",
};

/** A referral the receiving unit returned to its GP, as the responsible GP answers it from the case ledger. */
export type ReturnedPacket = {
  id: string;
  name: string;
  short: string;
  /** The case's `api_overlay.head`, which the answer must name. */
  head: string;
  /** The ledger event of the unit's return: the packet the answer is bound to. */
  packet: string;
  /** The unit that returned it, in words, and the unit id a re-referral goes back to. */
  unit: string;
  unitId: string;
  reason: string;
  infoRequested: ReferralInfo[];
  /** Other units offering the procedure, for a redirect. */
  others: { id: string; label: string }[];
  actBy: number;
  status: HandoffStatus;
  acknowledged: boolean;
  /** The GP's resolving answer in words, or null while the hand-off waits on the GP. */
  answer: string | null;
};

export function returnedPacket(c: CaseSummary): ReturnedPacket | null {
  const ov = c.api_overlay;
  const h = ov?.referral_handoff;
  if (!ov || !h?.event || c.closed) return null;
  const r = h.referrer_response;
  return {
    id: c.id,
    name: personForLoop(c.id)?.display_name ?? shortId(c.id),
    short: shortId(c.id),
    head: ov.head,
    packet: h.event,
    unit: providerLabel(h.decided_by),
    unitId: h.decided_by,
    reason: ov.referral_response?.reason ? REASON_LABELS[ov.referral_response.reason] : h.reason_code,
    infoRequested: h.info_requested,
    others: h.destinations.filter((d) => d !== h.decided_by).map((id) => ({ id, label: providerLabel(id) })),
    actBy: h.act_by,
    status: h.status,
    acknowledged: !!h.acknowledgement,
    answer: r ? `${DISPOSITION_LABELS[r.response]}${r.destination ? `: ${providerLabel(r.destination)}` : ""}${r.reason ? `. Reason: ${r.reason}` : ""}. Answered ${formatDay(r.run_day)}` : null,
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
