// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TodayBoard } from "@/components/worklist/today-board";
import { DECLINED_CANCELLED_NEXT, DECLINED_RESERVED_NEXT, DECLINED_STATUS, workItem } from "@/components/worklist/model";
import { recordBooking, type BookingProjection } from "@/app/app/patients/[id]/booking-actions";
import { ActiveCaseSummary, Booking, BookingSummary, CaseDetail, type ApiOverlay } from "@/lib/api/schema";

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
const workCase = { ...ActiveCaseSummary.parse(baseDetail.case), owner: "someone", state: "no_show" as const, dropoff: "no_show" as const, closed: false };

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

/** The board's props as last rendered: a queue switch re-renders them with the new `queue`, as `?queue=` does. */
let boardProps: Parameters<typeof TodayBoard>[0] | undefined;
const QUEUES = { "To call": "call", Waiting: "waiting", "No owner": "none" } as const;
const click = async (label: string, selector = "button") => {
  if (selector === "[role=tab]") {
    boardProps = { ...boardProps!, queue: QUEUES[label as keyof typeof QUEUES] };
    await act(async () => root!.render(createElement(TodayBoard, boardProps!)));
    return;
  }
  // A queue row leads with its task; it is found by the case it names.
  const row = selector === "button[data-case-row]";
  const el = [...container!.querySelectorAll<HTMLElement>(selector)].find((candidate) => (row ? candidate.textContent?.includes(label) : candidate.textContent?.trim().startsWith(label)));
  expect(el, label).toBeDefined();
  await act(async () => el!.click());
};
const nextField = () => [...container!.querySelectorAll("dt")].find((dt) => dt.textContent === "Next")!.nextElementSibling!.textContent;
const count = (text: string) => container!.textContent!.split(text).length - 1;

async function declineOnToday(loaded: WorkItemLike) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(TodayBoard, (boardProps = { items: [loaded], now: 40, aside: null, live: true, undoMs: 0 }))));
  if (loaded.view === "waiting") await click("Waiting", "[role=tab]");
  await click("Log call");
  await click("Reached");
  await click("Declined");
  await click("Waiting", "[role=tab]");
  await click(loaded.name, "button[data-case-row]");
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
 *   selection        validated new-head bookingPanel  -> same row projection as booking save, preserving case-local history
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
    await act(async () => root!.render(createElement(TodayBoard, (boardProps = { items, now: 40, aside: null, live: true, undoMs: 0 }))));
  }

  async function declineSelected(tab: "To call" | "Waiting", name: string) {
    await click(tab, "[role=tab]");
    await click(name, "button[data-case-row]");
    await click("Log call");
    await click("Reached");
    await click("Declined");
    await click("Waiting", "[role=tab]");
    await click(name, "button[data-case-row]");
  }

  beforeEach(() => {
    server.clear();
    // The selection refresh after a successful contact write reads that write, not a contradictory pre-write packet.
    actions.recordAction.mockImplementation(async (id: string, _head: string, action: { outcome: "reached" | "declined" }) => {
      const current = server.get(id)!;
      const head = action.outcome === "reached" ? h("f") : h("9");
      server.set(id, { ...current, head, last_contact: { at: DECLINED_LATER, event: head, outcome: action.outcome, channel: "phone" } });
      return { ok: true, head, hash: head, at: DECLINED_LATER, actor: "coordinator-bound" };
    });
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

  // A coordinator in this mounted board first writes a local booking override. A different coordinator then changes
  // the booking. These synthetic packets exercise the real selection reader and action callbacks, not API captures.
  const EXTERNAL_HEAD = h("5");
  const REACHED_HEAD = h("6");
  const DECLINED_HEAD = h("7");
  const packet = (id: string, ov: ApiOverlay, status = "ok") => ({
    status, detail: "", applied: 4, entries: [], bookingPanel: projectionOf(id, ov),
  });
  const selectedName = () => container!.querySelector<HTMLElement>("section[data-case]")?.dataset.case;
  async function selectA() {
    // An accepted refresh may move A between tabs; that must not make the test act on the fallback case B.
    for (const tab of ["To call", "Waiting", "To call", "Waiting"]) {
      await click(tab, "[role=tab]");
      const row = [...container!.querySelectorAll<HTMLButtonElement>("button[data-case-row]")].find((b) => b.textContent?.includes("EXP-sr-1"));
      if (row) {
        await act(async () => row.click());
        if (selectedName() === "EXP-sr-1") return;
      }
    }
    throw new Error("case A was not selectable after refresh");
  }

  for (const externalStatus of ["reserved", "cancelled"] as const) {
    it(`local ${externalStatus === "reserved" ? "cancel" : "rebook"} then external ${externalStatus}: refreshed current-head Reached/Declined agrees with reload`, async () => {
      const locallyCancelled = externalStatus === "reserved";
      server.set(ID_A, overlay(booking(locallyCancelled ? "reserved" : "cancelled", locallyCancelled ? RESERVED : CANCELLED)));
      server.set(ID_B, overlay(booking("cancelled", CANCELLED)));
      await mount([itemOf(ID_A, server.get(ID_A)!), itemOf(ID_B, server.get(ID_B)!)]);
      if (locallyCancelled) await click("Waiting", "[role=tab]");
      await bookingSave(locallyCancelled ? "Cancel simulated reservation" : "Choose another simulated slot");
      expect(recordBooking).toHaveBeenCalledTimes(1);
      expect(vi.mocked(recordBooking).mock.calls[0].slice(0, 2)).toEqual([ID_A, h("c")]);
      await selectA();
      const localNext = nextField();
      expect(localNext).toBe(locallyCancelled ? "Choose another simulated slot" : "Manage the simulated reservation");

      // Start a packet for A, leave it unresolved, then change A externally and return. The abandoned read ignores
      // abort deliberately so the production abort/revision fences, rather than the fetch mock, must discard it.
      await click("To call", "[role=tab]");
      await click("EXP-sr-2", "button[data-case-row]");
      let resolveOld!: (value: Response) => void;
      const oldRead = new Promise<Response>((resolve) => { resolveOld = resolve; });
      vi.mocked(fetch).mockReturnValueOnce(oldRead);
      await click(locallyCancelled ? "To call" : "Waiting", "[role=tab]");
      if (locallyCancelled) await click("EXP-sr-1", "button[data-case-row]");
      const oldSignal = vi.mocked(fetch).mock.calls.at(-1)![1]!.signal!;
      const oldOverlay = server.get(ID_A)!;
      const external = overlay(booking(externalStatus, REBOOKED), undefined, EXTERNAL_HEAD);
      server.set(ID_A, external);
      await click("To call", "[role=tab]");
      await click("EXP-sr-2", "button[data-case-row]");
      await selectA();
      expect(oldSignal.aborted).toBe(true);
      await act(async () => resolveOld(new Response(JSON.stringify({ ...packet(ID_A, oldOverlay),
        entries: [{ what: "ABORTED-BOOKING-PACKET", when: "Saved", who: "Synthetic" }],
      }), { status: 200 })));
      const refreshedNext = nextField();
      const refreshedTrailCount = count(itemOf(ID_A, external).next);
      expect(container!.textContent).not.toContain("ABORTED-BOOKING-PACKET");

      // Unlike a canned successful response, this mock refuses stale expected heads and persists the actual contact
      // outcomes into the next ledger read. Reached and Declined must both succeed on successive CURRENT heads.
      actions.recordAction.mockImplementation(async (id: string, expectedHead: string, action: { outcome: "reached" | "declined" }) => {
        const current = server.get(id)!;
        if (current.head !== expectedHead) return { ok: false, status: 409, conflict: "head", error: "stale head" };
        const head = action.outcome === "reached" ? REACHED_HEAD : DECLINED_HEAD;
        server.set(id, { ...current, head, last_contact: { at: DECLINED_LATER, event: head, outcome: action.outcome, channel: "phone" } });
        return { ok: true, head, hash: head, at: DECLINED_LATER, actor: "coordinator-bound" };
      });
      await click("Log call");
      await click("Reached");
      await click("Declined");
      await selectA();
      expect(actions.recordAction.mock.calls.map((call) => call.slice(0, 3))).toEqual([
        [ID_A, EXTERNAL_HEAD, { action: "contact_outcome", outcome: "reached", channel: "phone" }],
        [ID_A, REACHED_HEAD, { action: "contact_outcome", outcome: "declined", channel: "phone" }],
      ]);
      expect(server.get(ID_A)!.head).toBe(DECLINED_HEAD);
      expect(container!.querySelector('[data-testid="action-refused"]')).toBeNull();
      const reloaded = itemOf(ID_A, server.get(ID_A)!);
      const expectedNext = locallyCancelled ? DECLINED_RESERVED_NEXT : DECLINED_CANCELLED_NEXT;
      expect(nextField()).toBe(expectedNext);
      expect(count(expectedNext)).toBeGreaterThanOrEqual(2); // Next and current trail agree, not just the controls.
      expect([reloaded.next, reloaded.reserved, reloaded.cancelled]).toEqual([expectedNext, locallyCancelled, !locallyCancelled]);
      expect(refreshedNext).toBe(itemOf(ID_A, external).next);
      expect(refreshedTrailCount).toBeGreaterThanOrEqual(2);
      expect(recordBooking).toHaveBeenCalledTimes(1); // Declining never cancels or acquires capacity.
      expect(server.get(ID_A)!.booking!.status).toBe(externalStatus);
      expect(server.get(ID_B)!.head).toBe(h("c"));
      await click("To call", "[role=tab]");
      await click("EXP-sr-2", "button[data-case-row]");
      expect(nextField()).toBe("Choose another simulated slot");
      expect(container!.textContent).not.toContain(expectedNext);
    });
  }

  for (const invalidPart of ["ledger", "overlay"] as const) {
    it(`an invalid ${invalidPart} prefix cannot replace a local cancellation or authorize a current-head decline`, async () => {
      server.set(ID_A, overlay(booking("reserved", RESERVED)));
      server.set(ID_B, overlay(booking("cancelled", CANCELLED)));
      await mount([itemOf(ID_A, server.get(ID_A)!), itemOf(ID_B, server.get(ID_B)!)]);
      await click("Waiting", "[role=tab]");
      await bookingSave("Cancel simulated reservation");
      await selectA();
      await click("EXP-sr-2", "button[data-case-row]");
      const invalidOverlay = { ...overlay(booking("reserved", REBOOKED), undefined, EXTERNAL_HEAD),
        ledger_status: invalidPart === "overlay" ? "invalid" as const : "ok" as const };
      vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(packet(ID_A, invalidOverlay, invalidPart === "ledger" ? "invalid" : "ok")), { status: 200 }));
      await click("EXP-sr-1", "button[data-case-row]");
      expect(nextField()).toBe("Choose another simulated slot");
      if (invalidPart === "ledger") {
        expect((docButton("Log call") as HTMLButtonElement).disabled).toBe(true);
        expect(container!.textContent).toContain("not trustworthy");
      } else {
        // The envelope is ok but its prefix is not authoritative: the last trusted row head must still be used.
        actions.recordAction.mockResolvedValue({ ok: false, status: 409, conflict: "head", error: "stale head" });
        await click("Log call");
        await click("Reached");
        expect(actions.recordAction.mock.calls[0][1]).toBe(NEW_HEAD);
        expect(container!.textContent).toContain("Not saved:");
      }
      expect(recordBooking).toHaveBeenCalledTimes(1);
    });
  }

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
    await click("EXP-sr-1", "button[data-case-row]");
    expect(nextField()).toBe("Manage the simulated reservation");
  });
});
