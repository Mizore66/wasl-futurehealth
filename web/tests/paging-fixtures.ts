/** SYNTHETIC UNIT DATA ONLY: render/state tests, not API captures, signed history or clinical evidence.
 * Never copy these rows into committed API samples. No inherited contact, overlay or patient authority.
 */
import { ActiveCaseSummary, CaseDetail, CaseList, type Dropoff } from "@/lib/api/schema";
import { workItem } from "@/components/worklist/model";
import { referralFrom } from "@/components/worklist/referrals";
import listJson from "./fixtures/ledger_case_list.json";
import holdJson from "@/data/api/case_review_hold.json";

export const WALL = Date.parse("2026-11-20T12:00:00Z");
const base = ActiveCaseSummary.parse(CaseList.parse(listJson).cases[0]);
const cleanBase = Object.fromEntries(Object.entries(base).filter(([k]) => !["api_overlay", "contact_state", "api_patient"].includes(k)));
const OWNER = "Practitioner/ahs-mushrif-nurse-1";
export const syntheticId = (i: number) => `ahs-mushrif/ServiceRequest/AHS-sr-${90000 + i}`;

export function syntheticCaseSummaries(count: number, opts: { dropoffs?: Dropoff[]; mixed?: boolean } = {}) {
  return Array.from({ length: count }, (_, i) => ActiveCaseSummary.parse({
    ...cleanBase, id: syntheticId(i), provider: "ahs-mushrif", programme: "crc", state: "open", consent: "granted", review: "none",
    closed: false, owner: opts.mixed && i % 3 === 2 ? "" : OWNER,
    dropoff: opts.mixed && i % 3 === 1 ? "" : (opts.dropoffs ?? ["not_booked"])[i % (opts.dropoffs?.length ?? 1)],
    // Whole-day ties deliberately cross page boundaries, preserving input order within each tie.
    referral_day: 1 + i * 0.01,
  }));
}
export const syntheticWorkItems = (count: number, now = 120) => syntheticCaseSummaries(count, { mixed: true }).map((c) => workItem(c, now, true, WALL));
/** Explicit single-tab variant: all 1201 are reachable in To call, unlike the mixed 401/400/400 factory. */
export const allCallWorkItems = (count: number, now = 120) => syntheticCaseSummaries(count).map((c) => workItem(c, now, true, WALL));
export const syntheticReferrals = (count: number, now = 120) => syntheticCaseSummaries(count, {
  dropoffs: ["referral_unanswered", "not_booked", "inadequate", "referral_rejected", "in_progress", ""],
}).map((c) => {
  const r = referralFrom(c, now, true);
  if (!r) throw new Error(`Synthetic referral unexpectedly absent: ${c.id}`);
  return r;
});
/** Explicit single-tab variant: every referral is new and reachable in Needs you. */
export const allNewReferrals = (count: number, now = 120) => syntheticCaseSummaries(count, { dropoffs: ["referral_unanswered"] }).map((c) => {
  const r = referralFrom(c, now, true);
  if (!r) throw new Error(`Synthetic referral unexpectedly absent: ${c.id}`);
  return r;
});
export function syntheticReviewDetails(count: number) {
  const hold = CaseDetail.parse(holdJson);
  return Array.from({ length: count }, (_, i) => ({ ...hold, case: { ...hold.case, id: syntheticId(i), referral_day: 1 + i * 0.01 } }));
}
