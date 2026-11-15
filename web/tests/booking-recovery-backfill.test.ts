// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BookingControls } from "@/components/case/booking-controls";
import { recordBooking, type BookingProjection } from "@/app/app/patients/[id]/booking-actions";
import { ActiveCaseSummary, Booking, BookingSummary, CaseDetail, type ApiOverlay } from "@/lib/api/schema";

vi.mock("@/app/app/patients/[id]/booking-actions", () => ({ recordBooking: vi.fn() }));
vi.mock("next/link", () => ({ default: (props: Record<string, unknown>) => createElement("a", { href: String(props.href) }, props.children as never) }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const baseDetail = CaseDetail.parse(JSON.parse(readFileSync(path.join(__dirname, "fixtures", "ledger_case_detail.json"), "utf8")));
const h = (char: string) => char.repeat(64);
const slot = { slot_id: "sim:colonoscopy:201:1", session_id: 201, unit: "AD-1", day: 20, procedure: "colonoscopy", deadline: 31 };
const candidate = (slotId: string, version: string) => ({
  ...slot, slot_id: slotId, region: "AD", within_deadline: true, endoscopist_female: false, arabic: true, after_hours: false,
  prep_send_day: 15, predicted_no_show: 0.14, nurse_call: false, cross_region: false, version, availability: "available",
});
const booking = Booking.parse({
  status: "suggested", suggestion_version: h("2"), calendar_version: h("3"), suggested_slot_id: slot.slot_id, reservable: true,
  eligible_slots: [candidate(slot.slot_id, h("4")), candidate("sim:colonoscopy:202:1", h("5"))],
});
const backfill = { slot_id: "rel_aaaaaaaaaaaaaaaaaaaaaaaa", status: "simulated_backfill_confirmed" as const, day: 40, event: h("e"), outreach_status: "none" as const };
const reserved = BookingSummary.parse({
  status: "reserved", mode: "simulated", revision: 1, reservation_id: h("a"), reservation_version: h("b"), slot, updated_at: "2026-11-13T09:30:00Z", updated_event: h("b"),
});

function projection({ recovery, active }: { recovery: boolean; active: boolean }): BookingProjection {
  const overlay: ApiOverlay = { head: h("c"), actions: 1, owner: "", last_action_at: "", ledger_status: "ok", no_answer_streak: 0, ...(active ? { booking: reserved } : {}) };
  const c = {
    ...ActiveCaseSummary.parse(baseDetail.case), state: "open" as const, consent: "granted" as const, closed: false, api_overlay: overlay,
    ...(recovery ? { recovery: { source_releases: [], backfill } } : {}),
  };
  return { booking, overlay, case: c } as unknown as BookingProjection;
}

let container: HTMLDivElement | undefined;
let root: Root | undefined;

async function mount(initial: BookingProjection) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(BookingControls, { caseId: "case-1", initial, live: true })));
}
const button = (label: string) => [...container!.querySelectorAll<HTMLButtonElement>("button")].find((b) => b.textContent?.trim() === label)!;

afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  container?.remove();
  root = undefined;
  container = undefined;
  vi.clearAllMocks();
});

describe("booking controls for a case with a confirmed recovery backfill", () => {
  it("disables Accept and Choose, explains the recovery slot and links to its projection", async () => {
    await mount(projection({ recovery: true, active: false }));
    expect(button("Accept simulated slot").disabled).toBe(true);
    expect(button("Choose another simulated slot").disabled).toBe(true);
    expect(container!.textContent).toContain("A simulated recovery slot is already confirmed for this case");
    expect(container!.querySelector("a")?.getAttribute("href")).toBe(`/app/recovery?slot=${backfill.slot_id}`);
    await act(async () => button("Accept simulated slot").click());
    await act(async () => button("Choose another simulated slot").click());
    expect(document.body.querySelector("form")).toBeNull();
    expect(recordBooking).not.toHaveBeenCalled();
  });

  it("disables Reschedule but keeps Cancel for an existing ordinary reservation", async () => {
    await mount(projection({ recovery: true, active: true }));
    expect(button("Reschedule simulated slot").disabled).toBe(true);
    expect(button("Cancel simulated reservation").disabled).toBe(false);
  });

  it("leaves a comparable case without a backfill enabled", async () => {
    await mount(projection({ recovery: false, active: false }));
    expect(button("Accept simulated slot").disabled).toBe(false);
    expect(button("Choose another simulated slot").disabled).toBe(false);
    expect(container!.textContent).not.toContain("recovery slot");
  });

  it("leaves Reschedule enabled for a reservation without a backfill", async () => {
    await mount(projection({ recovery: false, active: true }));
    expect(button("Reschedule simulated slot").disabled).toBe(false);
  });
});
