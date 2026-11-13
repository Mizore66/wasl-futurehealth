import { readFileSync } from "node:fs";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { recordBooking } from "@/app/app/patients/[id]/booking-actions";
import { persistedLedger } from "@/components/worklist/ledger-trail";
import { BOOKING_NEEDED, DECLINED_RESERVED_NEXT, DECLINED_STATUS, recordNodes, workItem } from "@/components/worklist/model";
import { Booking, BookingSummary, CaseDetail, type ApiOverlay } from "@/lib/api/schema";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));

const load = (name: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", name), "utf8"));
const baseDetail = CaseDetail.parse(load("ledger_case_detail.json"));
const h = (char: string) => char.repeat(64);
const at = "2026-11-13T10:00:00Z";
const meta = baseDetail.meta;
const slot = { slot_id: "sim:colonoscopy:201:1", session_id: 201, unit: "AD-1", day: 20, procedure: "colonoscopy", deadline: 31 };

function reservation(status: "unreserved" | "reserved" | "cancelled", updatedAt: string | null = at) {
  return BookingSummary.parse({
    status, mode: "simulated", revision: status === "unreserved" ? 0 : 1,
    reservation_id: status === "unreserved" ? null : h("a"), reservation_version: status === "unreserved" ? null : h("b"),
    slot: status === "unreserved" ? null : slot, updated_at: updatedAt, updated_event: status === "unreserved" ? null : h("b"),
  });
}

function overlay(booking: BookingSummary, agreementAt?: string): ApiOverlay {
  return {
    head: h("c"), actions: 1, owner: "someone", last_action_at: at, ledger_status: "ok", no_answer_streak: 0,
    booking,
    ...(agreementAt ? { last_appointment_agreement: { at: agreementAt, event: h("d") }, last_contact: { at: agreementAt, event: h("d"), outcome: "appointment_agreed", channel: "phone" } } : {}),
  };
}

const workCase = { ...baseDetail.case, owner: "someone", state: "no_show" as const, dropoff: "no_show" as const, closed: false };

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("Today simulated booking precedence", () => {
  it("an active reservation wins over no-show, agreement and snooze", () => {
    const ov = overlay(reservation("reserved"), "2026-11-13T09:00:00Z");
    ov.snooze = { until: "2026-11-20T00:00:00Z", reason: "Ordinary snooze", at: "2026-11-13T11:00:00Z" };
    const item = workItem({ ...workCase, api_overlay: ov }, 40, true, Date.parse("2026-11-13T12:00:00Z"));
    expect([item.view, item.status, item.snoozed]).toEqual(["waiting", "Simulated slot reserved · Day 20", true]);
  });

  it("a cancellation after the latest agreement goes to rebook; an older agreement leaves booking needed", () => {
    const cancelled = workItem({ ...workCase, api_overlay: overlay(reservation("cancelled"), "2026-11-13T09:00:00Z") }, 40, true);
    expect([cancelled.view, cancelled.status]).toEqual(["call", "Simulated reservation cancelled · rebook"]);
    const older = reservation("cancelled", "2026-11-13T08:00:00Z");
    const agreed = workItem({ ...workCase, api_overlay: overlay(older, "2026-11-13T09:00:00Z") }, 40, true);
    expect([agreed.view, agreed.status]).toEqual(["waiting", BOOKING_NEEDED]);
  });

  const withDecline = (booking: BookingSummary, declinedAt: string, agreementAt?: string) => {
    const ov = overlay(booking, agreementAt);
    ov.last_contact = { at: declinedAt, event: h("e"), outcome: "declined", channel: "phone" };
    return { ...workCase, api_overlay: ov };
  };

  it("a decline after the reservation stays visible with the active reservation warning", () => {
    const item = workItem(withDecline(reservation("reserved"), "2026-11-13T10:00:00.000000001Z"), 40, true);
    expect([item.view, item.status, item.next]).toEqual(["waiting", DECLINED_STATUS, DECLINED_RESERVED_NEXT]);
    expect(item.reserved).toBe(true);
  });

  it("a reservation newer than, or at the same instant as, the decline keeps the reservation workflow", () => {
    for (const declinedAt of ["2026-11-13T09:00:00Z", "2026-11-13T10:00:00.000Z"]) {
      const item = workItem(withDecline(reservation("reserved"), declinedAt), 40, true);
      expect(item.status).toBe("Simulated slot reserved · Day 20");
      expect(item.next).toBe("Manage the simulated reservation");
    }
  });

  it("a decline after agreement, reservation and cancellation shows the decline, not rebook", () => {
    const item = workItem(withDecline(reservation("cancelled"), "2026-11-13T10:30:00Z", "2026-11-13T09:00:00Z"), 40, true);
    expect([item.view, item.status]).toEqual(["waiting", DECLINED_STATUS]);
    expect(item.reserved).toBe(false);
  });

  it("a cancellation after the decline goes to rebook", () => {
    const item = workItem(withDecline(reservation("cancelled"), "2026-11-13T09:30:00Z", "2026-11-13T09:00:00Z"), 40, true);
    expect([item.view, item.status]).toEqual(["call", "Simulated reservation cancelled · rebook"]);
  });

  it("agreement without a reservation keeps the existing booking-needed state", () => {
    const item = workItem({ ...workCase, api_overlay: overlay(reservation("unreserved"), "2026-11-13T09:00:00Z") }, 40, true);
    expect([item.view, item.status]).toEqual(["waiting", BOOKING_NEEDED]);
  });
});

describe("booking events in the human history", () => {
  const coordinatorEvent = (kind: "booking_accept" | "booking_reschedule" | "booking_cancel", hash: string, data: Record<string, unknown>, time: string) => ({
    ledger: "coordinator" as const, type: "coordinator_action" as const, kind, case: baseDetail.case.id,
    role: "coordinator", action: "coordinate_case", actor: "coordinator-bound", at: time, mode: "demo", prev: h("0"), hash,
    head: h("1"), seq: 1, data,
  });
  const accept = coordinatorEvent("booking_accept", h("a"), { slot_id: slot.slot_id }, "2026-11-13T09:00:00Z");
  const reschedule = coordinatorEvent("booking_reschedule", h("b"), {
    reservation_id: h("a"), slot_id: "sim:colonoscopy:202:1", reason: "Provider list changed",
  }, "2026-11-13T10:00:00Z");
  const cancel = coordinatorEvent("booking_cancel", h("c"), { reservation_id: h("a"), reason: "Patient asked to cancel" }, "2026-11-13T11:00:00Z");
  const detail = { ...baseDetail, api_reviews: [accept, reschedule, cancel] };

  it("keeps actor, reason, old/new slot and event hash in ledger order", () => {
    const entries = persistedLedger(detail).entries;
    expect(entries.map((entry) => entry.hash)).toEqual([h("a"), h("b"), h("c")]);
    expect(entries[0].what).toContain("Simulated slot reserved · sim:colonoscopy:201:1");
    expect(entries[1].what).toBe("Simulated reservation moved · sim:colonoscopy:201:1 → sim:colonoscopy:202:1: “Provider list changed”");
    expect(entries[2].what).toContain("Simulated reservation cancelled · sim:colonoscopy:202:1");
    expect(entries.every((entry) => entry.who === "coordinator-bound")).toBe(true);
  });

  it("adds booking to the human chain without injecting canonical source events", () => {
    const nodes = recordNodes(detail, []);
    const bookings = nodes.filter((node) => node.id.startsWith("booking-"));
    expect(bookings.map((node) => node.hash)).toEqual([h("a"), h("b"), h("c")]);
    expect(bookings[1].when).toBe("2026-11-13 10:00 UTC");
    expect(detail.timeline.some((event) => event.type === "booking_reschedule")).toBe(false);
  });
});

describe("booking server action read-back", () => {
  const booking = Booking.parse({
    status: "suggested", suggestion_version: h("2"), calendar_version: h("3"), suggested_slot_id: slot.slot_id,
    reservable: true,
    eligible_slots: [{
      ...slot, region: "AD", within_deadline: true, endoscopist_female: false, arabic: true, after_hours: false,
      prep_send_day: 15, predicted_no_show: 0.14, nurse_call: false, cross_region: false, version: h("4"), availability: "available",
    }],
  });
  const detail = CaseDetail.parse({
    ...baseDetail,
    case: { ...baseDetail.case, api_overlay: overlay(reservation("unreserved")) },
    api_overlay: overlay(reservation("unreserved")), review_ledger: { status: "ok", events_applied: 1 }, booking,
  });
  const action = { action: "booking_accept" as const, expected_suggestion_version: h("2"), slot_id: slot.slot_id, expected_slot_version: h("4") };

  beforeEach(() => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "coordinator-secret-1234567890");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "reader-secret-1234567890");
  });

  it("a conflict re-reads and returns the latest server projection", async () => {
    const latest = CaseDetail.parse({ ...detail, api_overlay: { ...detail.api_overlay, head: h("f") } });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ meta, error: "slot changed", conflict: "slot" }), { status: 409 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(latest), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const result = await recordBooking(detail.case.id, h("c"), action);
    expect(result.ok).toBe(false);
    expect(!result.ok && result.projection?.overlay.head).toBe(h("f"));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[1][0]).toContain(`/api/cases/${encodeURIComponent(detail.case.id)}`);
  });

  it("an unreadable 201 followed by failed read-back is saved-but-unknown, not a retryable refusal", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response("not json", { status: 201 }))
      .mockResolvedValueOnce(new Response("down", { status: 503 }));
    vi.stubGlobal("fetch", fetchMock);
    const result = await recordBooking(detail.case.id, h("c"), action);
    expect(result.ok).toBe(true);
    expect(result.ok && result.response).toBeNull();
    expect(result.ok && result.projection).toBeNull();
    expect(result.ok && result.warning).toContain("was saved");
  });

  it("missing coordinator credentials send nothing", async () => {
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await recordBooking(detail.case.id, h("c"), action);
    expect(result.ok).toBe(false);
    expect(!result.ok && result.status).toBe(401);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
