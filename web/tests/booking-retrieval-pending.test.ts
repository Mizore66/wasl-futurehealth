import { readFileSync } from "node:fs";
import path from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { BookingControls } from "@/components/case/booking-controls";
import type { BookingProjection } from "@/app/app/patients/[id]/booking-actions";
import { ActiveCaseSummary, Booking, BookingSummary, CaseDetail, type ApiOverlay } from "@/lib/api/schema";

vi.mock("@/app/app/patients/[id]/booking-actions", () => ({ recordBooking: vi.fn() }));

const h = (char: string) => char.repeat(64);
const detail = CaseDetail.parse(JSON.parse(readFileSync(path.join(__dirname, "fixtures", "ledger_case_detail.json"), "utf8")));
const workCase = { ...ActiveCaseSummary.parse(detail.case), consent: "granted" as const, state: "open" as const, closed: false };
const slot = {
  slot_id: "sim:colonoscopy:201:1", session_id: 201, unit: "AD-1", day: 20, procedure: "colonoscopy", deadline: 31, region: "AD", within_deadline: true,
  endoscopist_female: false, arabic: true, after_hours: false, prep_send_day: 15, predicted_no_show: 0.14, nurse_call: false, cross_region: false,
  version: h("4"), availability: "available",
};
const booking = Booking.parse({ status: "suggested", suggestion_version: h("2"), calendar_version: h("3"), suggested_slot_id: slot.slot_id, reservable: true, eligible_slots: [slot] });
const reserved = BookingSummary.parse({
  status: "reserved", mode: "simulated", revision: 1, reservation_id: h("a"), reservation_version: h("b"), slot, updated_at: "2026-11-13T09:30:00Z", updated_event: h("b"),
});
const pending = {
  status: "pending", at: "2026-11-13T10:00:00Z", procedure: "colonoscopy", performed: "2026-11-01", provider: "Other hospital", source: "patient",
  retrieval_owner: "Nurse 5", retrieval_due: "2026-11-20",
};

function render(overlay: Partial<ApiOverlay>, retrievalPending = false) {
  const ov = { head: h("c"), actions: 1, owner: "someone", ledger_status: "ok", no_answer_streak: 0, ...overlay } as ApiOverlay;
  const initial = { booking, overlay: ov, case: { ...workCase, api_overlay: ov } } as unknown as BookingProjection;
  const html = renderToStaticMarkup(createElement(BookingControls, { caseId: workCase.id, initial, live: true, retrievalPending }));
  const button = (label: string) => {
    const m = new RegExp(`<button[^>]*>${label}</button>`).exec(html);
    expect(m, `${label} rendered`).not.toBeNull();
    return / disabled=""/.test(m![0]) ? "disabled" : "enabled";
  };
  return { html, button };
}

describe("booking controls while a completed-elsewhere retrieval is pending", () => {
  it("offers booking when no retrieval is pending", () => {
    const { button } = render({});
    expect(button("Accept simulated slot")).toBe("enabled");
    expect(button("Choose another simulated slot")).toBe("enabled");
  });

  it("refuses accept and choose when the served overlay has a pending retrieval, and says why", () => {
    const { html, button } = render({ reconciliation: pending } as Partial<ApiOverlay>);
    expect(button("Accept simulated slot")).toBe("disabled");
    expect(button("Choose another simulated slot")).toBe("disabled");
    expect(html).toContain("record retrieval for a procedure done elsewhere is pending");
  });

  it("refuses acquisition when Today recorded the retrieval after the panel was read", () => {
    const { button } = render({}, true);
    expect(button("Accept simulated slot")).toBe("disabled");
    expect(button("Choose another simulated slot")).toBe("disabled");
  });

  it("still lets the coordinator cancel an existing reservation but not reschedule it", () => {
    const { button } = render({ booking: reserved }, true);
    expect(button("Cancel simulated reservation")).toBe("enabled");
    expect(button("Reschedule simulated slot")).toBe("disabled");
  });
});
