// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TodayBoard } from "@/components/worklist/today-board";
import { DECLINED_CANCELLED_NEXT, DECLINED_RESERVED_NEXT, DECLINED_STATUS, workItem } from "@/components/worklist/model";
import { recordBooking, type BookingProjection } from "@/app/app/patients/[id]/booking-actions";
import { Booking, BookingSummary, CaseDetail, type ApiOverlay } from "@/lib/api/schema";

/*
 * A genuine canonical no_show case can agree, reserve, cancel and then record a NEWER decline. The immediate Today
 * click and a reload of the same ledger must both say the GP signs off; neither may still say to rebook. The
 * synthetic overlays below are unit inputs shaped like the served overlay, not new captures.
 */
const actions = vi.hoisted(() => ({ recordAction: vi.fn() }));
vi.mock("@/app/app/today/actions", () => ({ recordAction: actions.recordAction }));
vi.mock("@/app/app/patients/[id]/booking-actions", () => ({ recordBooking: vi.fn() }));
vi.mock("@number-flow/react", () => ({ default: ({ value }: { value: number }) => createElement("span", null, String(value)) }));
vi.mock("motion/react", () => ({
  motion: { div: ({ role, className, children }: { role?: string; className?: string; children?: React.ReactNode }) => createElement("div", { role, className }, children) },
  AnimatePresence: ({ children }: { children?: React.ReactNode }) => children,
  useReducedMotion: () => true,
}));
vi.mock("next/link", () => ({ default: (props: Record<string, unknown>) => createElement("a", { href: String(props.href) }, props.children as never) }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const baseDetail = CaseDetail.parse(JSON.parse(readFileSync(path.join(__dirname, "fixtures", "ledger_case_detail.json"), "utf8")));
const h = (char: string) => char.repeat(64);
const slot = { slot_id: "sim:colonoscopy:201:1", session_id: 201, unit: "AD-1", day: 20, procedure: "colonoscopy", deadline: 31 };
const AGREED = "2026-11-13T09:00:00Z";
const RESERVED = "2026-11-13T09:30:00Z";
const CANCELLED = "2026-11-13T10:00:00.000000005Z";
const DECLINED_LATER = "2026-11-13T10:30:00Z";
const workCase = { ...baseDetail.case, owner: "someone", state: "no_show" as const, dropoff: "no_show" as const, closed: false };

const booking = (status: "reserved" | "cancelled", updatedAt: string) =>
  BookingSummary.parse({
    status, mode: "simulated", revision: 1, reservation_id: h("a"), reservation_version: h("b"), slot, updated_at: updatedAt, updated_event: h("b"),
  });

function overlay(b: BookingSummary, decline?: string, head = h("c")): ApiOverlay {
  return {
    head, actions: 3, owner: "someone", last_action_at: AGREED, ledger_status: "ok", no_answer_streak: 0, booking: b,
    last_appointment_agreement: { at: AGREED, event: h("d") },
    last_contact: decline
      ? { at: decline, event: h("e"), outcome: "declined", channel: "phone" }
      : { at: AGREED, event: h("d"), outcome: "appointment_agreed", channel: "phone" },
  };
}

const item = (ov: ApiOverlay) => workItem({ ...workCase, api_overlay: ov }, 40, true, Date.parse("2026-11-14T00:00:00Z"));

let container: HTMLDivElement | undefined;
let root: Root | undefined;

const click = async (label: string, selector = "button") => {
  const el = [...container!.querySelectorAll<HTMLElement>(selector)].find((candidate) => candidate.textContent?.trim().startsWith(label));
  expect(el, label).toBeDefined();
  await act(async () => el!.click());
};
const nextField = () => [...container!.querySelectorAll("dt")].find((dt) => dt.textContent === "Next")!.nextElementSibling!.textContent;
const count = (text: string) => container!.textContent!.split(text).length - 1;

async function declineOnToday(loaded: WorkItemLike) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(TodayBoard, { items: [loaded], now: 40, aside: null, live: true })));
  if (loaded.view === "waiting") await click("Waiting", "[role=tab]");
  await click("Log call");
  await click("Reached");
  await click("Declined");
  await click("Waiting", "[role=tab]");
  await click(loaded.name, "button[aria-pressed]");
}
type WorkItemLike = ReturnType<typeof item>;

beforeEach(() => {
  actions.recordAction.mockResolvedValue({ ok: true, head: h("f"), hash: h("9"), at: DECLINED_LATER, actor: "coordinator-bound" });
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ status: "ok", detail: "", applied: 3, entries: [] }), { status: 200 })));
});

afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  container?.remove();
  root = undefined;
  container = undefined;
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("a decline newer than the reservation cancellation", () => {
  const cancelled = () => overlay(booking("cancelled", CANCELLED));

  it("the loaded cancelled row is a rebook before the decline (control)", () => {
    const before = item(cancelled());
    expect([before.view, before.next]).toEqual(["call", "Choose another simulated slot"]);
  });

  it("the immediate Today click asks for GP sign-off, not rebooking, and releases nothing", async () => {
    await declineOnToday(item(cancelled()));
    expect(actions.recordAction.mock.calls.map((call) => call[2])).toEqual([
      { action: "contact_outcome", outcome: "reached", channel: "phone" },
      { action: "contact_outcome", outcome: "declined", channel: "phone" },
    ]);
    expect(nextField()).toBe(DECLINED_CANCELLED_NEXT);
    expect(count(DECLINED_CANCELLED_NEXT)).toBeGreaterThanOrEqual(2); // Next field and the trail's current node
    expect(container!.textContent).toContain(DECLINED_STATUS);
    expect(container!.textContent).not.toContain("Choose another simulated slot");
    expect(container!.textContent).not.toContain("Arrange a new date");
  });

  it("a reload of the same ledger gives the same next action", () => {
    const reloaded = item(overlay(booking("cancelled", CANCELLED), DECLINED_LATER));
    expect([reloaded.view, reloaded.status, reloaded.next]).toEqual(["waiting", DECLINED_STATUS, DECLINED_CANCELLED_NEXT]);
    expect(reloaded.next).not.toMatch(/rebook|another simulated slot|Arrange a new date/i);
    expect(reloaded.reserved).toBe(false);
  });

  it("a cancellation newer than, or at the same instant as, the decline still means rebook", () => {
    for (const declinedAt of ["2026-11-13T10:00:00.000000004Z", "2026-11-13T10:00:00.000000005Z"]) {
      const reloaded = item(overlay(booking("cancelled", CANCELLED), declinedAt));
      expect([reloaded.view, reloaded.status, reloaded.next]).toEqual(["call", "Simulated reservation cancelled · rebook", "Choose another simulated slot"]);
    }
  });

  it("a decline one nanosecond after the cancellation is the newer one", () => {
    const reloaded = item(overlay(booking("cancelled", CANCELLED), "2026-11-13T10:00:00.000000006Z"));
    expect(reloaded.next).toBe(DECLINED_CANCELLED_NEXT);
  });
});

describe("a decline while the reservation is active", () => {
  it("still warns to cancel explicitly and keeps the slot, on Today and after a reload", async () => {
    const active = item(overlay(booking("reserved", RESERVED)));
    expect(active.reserved).toBe(true);
    await declineOnToday(active);
    expect(nextField()).toBe(DECLINED_RESERVED_NEXT);
    expect(actions.recordAction).toHaveBeenCalledTimes(2);
    expect(actions.recordAction.mock.calls.every((call) => call[2].action === "contact_outcome")).toBe(true);
    const reloaded = item(overlay(booking("reserved", RESERVED), DECLINED_LATER));
    expect([reloaded.status, reloaded.next, reloaded.reserved]).toEqual([DECLINED_STATUS, DECLINED_RESERVED_NEXT, true]);
  });

  it("a reservation newer than the decline keeps the reservation workflow", () => {
    const reloaded = item(overlay(booking("reserved", DECLINED_LATER), AGREED));
    expect(reloaded.next).toBe("Manage the simulated reservation");
  });
});

/*
 * Booking transition table (each row is one place a displayed case's booking flags and next/trail are produced):
 *   initial/reload   workItem(case)                    -> view,status,next,head,snoozed,noAnswer,reserved,cancelled  (one overlay)
 *   booking save,    BookingControls.install(readback) -> bookingProjection(workItem(projection.case))               (one projection)
 *   cancel, rebook,  409 conflict re-read              -> same path; before this repair reserved/cancelled stayed from page load
 *   selection        ledger read bookingPanel          -> controls only; row flags come from the loaded item or the last patch
 *   Declined patch   p.reserved / p.cancelled          -> next text; must read the flags of the latest projection
 * These tests drive the production callbacks (BookingControls -> recordBooking -> bookingProjection -> log), not a parallel model.
 */
describe("booking flags follow the latest successful booking projection", () => {
  const REBOOKED = "2026-11-13T10:20:00Z";
  const CANCELLED_LATER = "2026-11-13T10:15:00Z";
  const NEW_HEAD = h("1");
  const ID_A = "exp-prov/ServiceRequest/EXP-sr-1";
  const ID_B = "exp-prov/ServiceRequest/EXP-sr-2";
  const panelBooking = Booking.parse({
    status: "suggested", suggestion_version: h("2"), calendar_version: h("3"), suggested_slot_id: slot.slot_id, reservable: true,
    eligible_slots: [{
      ...slot, region: "AD", within_deadline: true, endoscopist_female: false, arabic: true, after_hours: false,
      prep_send_day: 15, predicted_no_show: 0.14, nurse_call: false, cross_region: false, version: h("4"), availability: "available",
    }],
  });
  const server = new Map<string, ApiOverlay>();
  const caseOf = (id: string, ov: ApiOverlay) => ({ ...workCase, id, api_overlay: ov });
  const projectionOf = (id: string, ov: ApiOverlay) => ({ booking: panelBooking, overlay: ov, case: caseOf(id, ov) }) as unknown as BookingProjection;
  const itemOf = (id: string, ov: ApiOverlay) => workItem(caseOf(id, ov), 40, true, Date.parse("2026-11-14T00:00:00Z"));

  const settle = async (done: () => boolean) => {
    for (let i = 0; i < 100 && !done(); i++) await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });
    expect(done()).toBe(true);
  };
  const docButton = (label: string) => [...document.body.querySelectorAll<HTMLElement>("button")].find((b) => b.textContent?.trim().startsWith(label));
  const type = (el: HTMLInputElement, text: string) => {
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(el), "value")!.set!.call(el, text);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  };

  /** Open the booking dialog from the mounted controls, fill the reason and submit the real form. */
  async function bookingSave(openLabel: string) {
    await settle(() => !!docButton(openLabel));
    await click(openLabel);
    await settle(() => !!document.body.querySelector("input[name=reason]"));
    await act(async () => type(document.body.querySelector<HTMLInputElement>("input[name=reason]")!, "Coordinator action"));
    const form = document.body.querySelector("form")!;
    await act(async () => { form.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })); });
  }

  async function mount(items: WorkItemLike[]) {
    vi.stubGlobal("fetch", vi.fn(async (url: string) => {
      const id = new URL(url, "http://localhost").searchParams.get("id")!;
      const ov = server.get(id)!;
      return new Response(JSON.stringify({ status: "ok", detail: "", applied: 3, entries: [], bookingPanel: projectionOf(id, ov) }), { status: 200 });
    }));
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
    await act(async () => root!.render(createElement(TodayBoard, { items, now: 40, aside: null, live: true })));
  }

  async function declineSelected(tab: "To call" | "Waiting", name: string) {
    await click(tab, "[role=tab]");
    await click(name, "button[aria-pressed]");
    await click("Log call");
    await click("Reached");
    await click("Declined");
    await click("Waiting", "[role=tab]");
    await click(name, "button[aria-pressed]");
  }

  beforeEach(() => {
    server.clear();
    vi.mocked(recordBooking).mockImplementation(async (id, _head, action) => {
      const next = action.action === "booking_cancel"
        ? overlay(booking("cancelled", CANCELLED_LATER), undefined, NEW_HEAD)
        : overlay(booking("reserved", REBOOKED), undefined, NEW_HEAD);
      server.set(id, next);
      return { ok: true, response: null, projection: projectionOf(id, next) };
    });
  });

  it("loaded cancelled, rebooked through the controls, then Declined keeps the active-reservation cancel warning", async () => {
    server.set(ID_A, overlay(booking("cancelled", CANCELLED)));
    await mount([itemOf(ID_A, server.get(ID_A)!)]);
    await bookingSave("Choose another simulated slot");
    expect(recordBooking).toHaveBeenCalledTimes(1);
    expect(vi.mocked(recordBooking).mock.calls[0][0]).toBe(ID_A);
    expect(vi.mocked(recordBooking).mock.calls[0][1]).toBe(h("c"));
    expect(vi.mocked(recordBooking).mock.calls[0][2]).toMatchObject({ action: "booking_choose_slot", expected_suggestion_version: h("2"), slot_id: slot.slot_id, expected_slot_version: h("4") });

    await declineSelected("Waiting", "EXP-sr-1");
    const calls = actions.recordAction.mock.calls;
    expect(calls.map((call) => call[2].outcome)).toEqual(["reached", "declined"]);
    expect(calls[0][1]).toBe(NEW_HEAD); // the next action is fenced by the head of the same projection that set the flags
    expect(nextField()).toBe(DECLINED_RESERVED_NEXT);
    expect(container!.textContent).toContain(DECLINED_STATUS);
    const reloaded = itemOf(ID_A, overlay(booking("reserved", REBOOKED), DECLINED_LATER, NEW_HEAD));
    expect([reloaded.status, reloaded.next, reloaded.reserved, reloaded.cancelled]).toEqual([DECLINED_STATUS, DECLINED_RESERVED_NEXT, true, false]);
  });

  it("loaded reserved, cancelled through the controls, then a later Declined asks only for GP sign-off", async () => {
    server.set(ID_A, overlay(booking("reserved", RESERVED)));
    await mount([itemOf(ID_A, server.get(ID_A)!)]);
    await click("Waiting", "[role=tab]");
    await bookingSave("Cancel simulated reservation");
    expect(vi.mocked(recordBooking).mock.calls[0][2]).toMatchObject({ action: "booking_cancel", reservation_id: h("a"), expected_reservation_version: h("b") });

    await declineSelected("To call", "EXP-sr-1");
    expect(actions.recordAction.mock.calls[0][1]).toBe(NEW_HEAD);
    expect(nextField()).toBe(DECLINED_CANCELLED_NEXT);
    expect(container!.textContent).not.toContain(DECLINED_RESERVED_NEXT);
    const reloaded = itemOf(ID_A, overlay(booking("cancelled", CANCELLED_LATER), DECLINED_LATER, NEW_HEAD));
    expect([reloaded.status, reloaded.next, reloaded.reserved, reloaded.cancelled]).toEqual([DECLINED_STATUS, DECLINED_CANCELLED_NEXT, false, true]);
  });

  it("a rebook of one case does not change what Declined says for another case", async () => {
    server.set(ID_A, overlay(booking("cancelled", CANCELLED)));
    server.set(ID_B, overlay(booking("cancelled", CANCELLED)));
    await mount([itemOf(ID_A, server.get(ID_A)!), itemOf(ID_B, server.get(ID_B)!)]);
    await bookingSave("Choose another simulated slot");
    expect(vi.mocked(recordBooking).mock.calls[0][0]).toBe(ID_A);

    await declineSelected("To call", "EXP-sr-2");
    expect(actions.recordAction.mock.calls[0][0]).toBe(ID_B);
    expect(actions.recordAction.mock.calls[0][1]).toBe(h("c")); // B keeps its own head, not A's refreshed one
    expect(nextField()).toBe(DECLINED_CANCELLED_NEXT);
    await click("EXP-sr-1", "button[aria-pressed]");
    expect(nextField()).toBe("Manage the simulated reservation");
  });
});
