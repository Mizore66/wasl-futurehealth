// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TodayBoard } from "@/components/worklist/today-board";
import { DECLINED_CANCELLED_NEXT, DECLINED_RESERVED_NEXT, DECLINED_STATUS, workItem } from "@/components/worklist/model";
import { BookingSummary, CaseDetail, type ApiOverlay } from "@/lib/api/schema";

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

function overlay(b: BookingSummary, decline?: string): ApiOverlay {
  return {
    head: h("c"), actions: 3, owner: "someone", last_action_at: AGREED, ledger_status: "ok", no_answer_streak: 0, booking: b,
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
