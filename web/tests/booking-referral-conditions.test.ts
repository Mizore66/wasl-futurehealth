import { readFileSync } from "node:fs";
import path from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { BookingControls } from "@/components/case/booking-controls";
import type { BookingProjection } from "@/app/app/patients/[id]/booking-actions";
import { ActiveCaseSummary, Booking, CaseDetail, type ApiOverlay } from "@/lib/api/schema";

vi.mock("@/app/app/patients/[id]/booking-actions", () => ({ recordBooking: vi.fn() }));

// #168 acceptance test 3: a conditional referral acceptance reaches booking readiness as structured requirements.
const h = (char: string) => char.repeat(64);
const detail = CaseDetail.parse(JSON.parse(readFileSync(path.join(__dirname, "fixtures", "ledger_case_detail.json"), "utf8")));
const workCase = { ...ActiveCaseSummary.parse(detail.case), consent: "granted" as const, state: "open" as const, closed: false };
const slot = (session: number, female: boolean, met?: boolean) => ({
  slot_id: `sim:colonoscopy:${session}:1`, session_id: session, unit: `AD-${session}`, day: 20, deadline: 31, region: "AD", within_deadline: true,
  endoscopist_female: female, arabic: true, after_hours: false, prep_send_day: 15, predicted_no_show: 0.14, nurse_call: false, cross_region: false,
  version: h("4"), availability: "available",
  ...(met === undefined ? {} : { referral_conditions_met: met, ...(met ? {} : { unmet_conditions: ["female_endoscopist"] }) }),
});
const requirements = [
  { code: "female_endoscopist", kind: "clinician_preference", check: "slot" },
  { code: "hold_anticoagulants", kind: "medication", check: "before_procedure" },
];

function render(raw: Record<string, unknown>) {
  const booking = Booking.parse({ status: "suggested", suggestion_version: h("2"), calendar_version: h("3"), ...raw });
  const ov = { head: h("c"), actions: 1, owner: "someone", ledger_status: "ok", no_answer_streak: 0 } as ApiOverlay;
  const initial = { booking, overlay: ov, case: { ...workCase, api_overlay: ov } } as unknown as BookingProjection;
  const html = renderToStaticMarkup(createElement(BookingControls, { caseId: workCase.id, initial, live: true }));
  const button = (label: string) => {
    const m = new RegExp(`<button[^>]*>${label}</button>`).exec(html);
    expect(m, `${label} rendered`).not.toBeNull();
    return / disabled=""/.test(m![0]) ? "disabled" : "enabled";
  };
  return { html, button };
}

describe("booking controls after a conditional referral acceptance", () => {
  it("lists each condition and whether Wasl checks it on the slot", () => {
    const { html } = render({
      suggested_slot_id: "sim:colonoscopy:201:1", reservable: true, eligible_slots: [slot(201, true, true)],
      readiness: { ready: true, requirements },
    });
    expect(html).toContain("Receiving unit&#x27;s acceptance conditions");
    expect(html).toContain("Female endoscopist<span class=\"text-muted\"> · checked against the chosen slot</span>");
    expect(html).toContain("Hold blood thinners as advised<span class=\"text-muted\"> · arrange before the procedure; Wasl cannot check this</span>");
  });

  it("refuses the suggestion that misses a slot condition and offers only slots that meet it", () => {
    const { html, button } = render({
      suggested_slot_id: "sim:colonoscopy:201:1", reservable: false, reservation_explanation: "suggestion_misses_referral_conditions",
      eligible_slots: [slot(201, false, false), slot(202, true, true)], readiness: { ready: true, requirements },
    });
    expect(html).toContain("The suggested slot does not meet the receiving unit&#x27;s acceptance conditions.");
    expect(button("Accept simulated slot")).toBe("disabled");
    expect(button("Choose another simulated slot")).toBe("enabled");
  });

  it("offers no slot when none meets the conditions", () => {
    const { button } = render({
      suggested_slot_id: "sim:colonoscopy:201:1", reservable: false, reservation_explanation: "suggestion_misses_referral_conditions",
      eligible_slots: [slot(201, false, false)], readiness: { ready: true, requirements },
    });
    expect(button("Accept simulated slot")).toBe("disabled");
    expect(button("Choose another simulated slot")).toBe("disabled");
  });

  it("is unchanged without a conditional acceptance", () => {
    const { html, button } = render({ suggested_slot_id: "sim:colonoscopy:201:1", reservable: true, eligible_slots: [slot(201, false)], readiness: { ready: true } });
    expect(html).not.toContain("acceptance conditions");
    expect(button("Accept simulated slot")).toBe("enabled");
  });
});
