import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { persistedLedger } from "@/components/worklist/ledger-trail";
import { DECLINED_CANCELLED_NEXT, DECLINED_RESERVED_NEXT, DECLINED_STATUS, workItem } from "@/components/worklist/model";
import { ActiveCaseSummary, CaseDetail, CaseList, type ApiOverlay } from "@/lib/api/schema";

/* Issue #115: a clinician disposition answers the coordinator's persisted decline and resolves "GP to sign". */
const load = (f: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", f), "utf8"));
const base = ActiveCaseSummary.parse(CaseList.parse(load("ledger_case_list.json")).cases[0]);
const NOW = Date.parse("2026-11-20T12:00:00Z");
const decline = { at: "2026-11-20T09:00:00Z", event: "d".repeat(64), outcome: "declined", channel: "phone" };
const item = (ov: Partial<ApiOverlay>) =>
  workItem(
    { ...base, owner: "someone", dropoff: "not_booked", api_overlay: overlay({ last_contact: decline, ...ov }) },
    40,
    true,
    NOW,
  );
function overlay(ov: Partial<ApiOverlay>): ApiOverlay {
  return { head: "h", actions: 2, owner: "", last_action_at: "", ledger_status: "ok", no_answer_streak: 0, ...ov };
}
const followUp = { at: "2026-11-20T10:00:00Z", event: "e".repeat(64), role: "clinician", decline_event: decline.event, plan: "follow_up" as const, reason: "patient_preference", follow_up_owner: "Dr Al Kaabi", next_review: "2027-02-20" };

describe("clinician disposition after a decline", () => {
  it("a decline alone still waits for the GP", () => {
    expect([item({}).view, item({}).status]).toEqual(["waiting", DECLINED_STATUS]);
  });

  it("a follow-up disposition of that decline replaces the GP prompt with the owned review", () => {
    const it = item({ disposition: followUp });
    expect(it.view).toBe("waiting");
    expect(it.status).not.toContain("GP to sign");
    expect(it.status).toBe("Declined · GP review 2027-02-20");
    expect(it.next).toBe("Dr Al Kaabi reviews on 2027-02-20");
  });

  it("a closed_declined disposition shows a non-procedure end state, never a completed procedure", () => {
    const it = item({ disposition: { ...followUp, plan: "closed_declined", follow_up_owner: undefined, next_review: undefined } });
    expect(it.status).toBe("Declined · closed without the procedure");
    expect(it.status).not.toMatch(/done|complete/i);
    expect(it.next).not.toContain("GP to sign");
  });

  it("a disposition of an earlier decline does not answer a newer one", () => {
    const it = item({ disposition: { ...followUp, decline_event: "c".repeat(64) } });
    expect(it.status).toBe(DECLINED_STATUS);
  });

  it("drops the GP prompt from the reservation hand-off once signed", () => {
    const booking = (status: "reserved" | "cancelled") => ({ status, updated_at: "2026-11-20T08:00:00Z", slot: null }) as unknown as ApiOverlay["booking"];
    expect(item({ booking: booking("reserved") }).next).toBe(DECLINED_RESERVED_NEXT);
    expect(item({ booking: booking("reserved"), disposition: followUp }).next).toBe("Cancel the simulated reservation to release the slot");
    expect(item({ booking: booking("cancelled") }).next).toBe(DECLINED_CANCELLED_NEXT);
    expect(item({ booking: booking("cancelled"), disposition: followUp }).next).toBe("Dr Al Kaabi reviews on 2027-02-20");
  });

  it("the case trail names the disposition, its clinician role and the plan", () => {
    const detail = CaseDetail.parse(load("ledger_case_detail.json"));
    const last = detail.api_reviews.at(-1)!;
    const event = {
      ...last, ledger: undefined, type: "coordinator_action", kind: "clinician_disposition", role: "clinician", action: "record_disposition",
      actor: "dr-hamdan", hash: "f".repeat(64), prev: last.hash, seq: 99,
      data: { reason: "patient_preference", counselling: "c", safety_net: "s", plan: "follow_up", follow_up_owner: "Dr Al Kaabi", next_review: "2027-02-20", decline_event: decline.event, fit_reoffer: "offered_declined" },
    };
    const raw = load("ledger_case_detail.json");
    raw.api_reviews = [...raw.api_reviews, event];
    const entries = persistedLedger(CaseDetail.parse(raw)).entries;
    expect(entries.at(-1)).toMatchObject({ who: "dr-hamdan (clinician)", what: "GP disposition of the decline · Dr Al Kaabi reviews on 2027-02-20" });
  });
});
