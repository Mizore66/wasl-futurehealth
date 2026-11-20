// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { focalCandidate, outreachState, windowGeometry } from "@/components/recovery/candidates";
import { RecoveryBoard } from "@/components/recovery/recovery-board";
import { confirmBackfill } from "@/app/app/recovery/actions";
import { RecoveryDetail, RecoveryList } from "@/lib/api/recovery-schema";

const refresh = vi.fn();
vi.mock("@/app/app/recovery/actions", () => ({ cancelBackfill: vi.fn(), confirmBackfill: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }), usePathname: () => "/app/recovery" }));
vi.mock("next/link", () => ({ default: (props: Record<string, unknown>) => createElement("a", { href: String(props.href) }, props.children as never) }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const read = (name: string) => JSON.parse(readFileSync(path.join(__dirname, "..", "data", "api", name), "utf8"));
const list = RecoveryList.parse(read("recovery_slots.json"));
const open = RecoveryDetail.parse(read("recovery_slot.json"));
const top = open.slot.candidates[0];
const assigned = RecoveryDetail.parse({
  ...open,
  slot: { ...open.slot, status: "filled", filled_case: top.case_id, head: "filled-head", backfill: { case_id: top.case_id, event: "confirm-hash", status: "simulated_backfill_confirmed" } },
});

let container: HTMLDivElement | undefined;
let root: Root | undefined;
async function mount(detail: RecoveryDetail, readOnly: boolean) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(RecoveryBoard, { list, detail, readOnly })));
}
const button = (label: string) => [...document.querySelectorAll("button")].find((node) => node.textContent?.trim() === label);
const mark = (name: string) => document.querySelector(`[data-mark=${name}]`)?.textContent;

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  document.body.innerHTML = "";
  vi.mocked(confirmBackfill).mockReset();
});

describe("recovery time window geometry", () => {
  it("puts the slot between the ends with the gap to the due day", () => {
    const g = windowGeometry(12, 40, 47);
    expect([g.start, g.end, g.gap, g.gapText]).toEqual([0, 100, 7, "7 days"]);
    expect(g.slot).toBeCloseTo(80);
  });

  it("keeps the slot clear of the end markers when days are close or far apart", () => {
    expect(windowGeometry(39, 40, 40).slot).toBe(94);
    expect(windowGeometry(40, 40, 400).slot).toBe(6);
    expect(windowGeometry(40, 40, 40)).toMatchObject({ slot: 50, gapText: "Due that day" });
  });

  it("moves a slot after the due day to the end and leaves the due label room", () => {
    const g = windowGeometry(12, 40, 13);
    expect(g.slot).toBe(100);
    expect(g.end).toBe(35);
    expect(g.gapText).toBe("27 days past due");
  });
});

describe("recovery assignment and outreach stay distinct", () => {
  it("measures the open slot against the top-ranked case and holds its outreach", async () => {
    expect(focalCandidate(open.slot)?.case_id).toBe(top.case_id);
    expect(outreachState(top, open.slot.outreach)).toEqual({ tone: "held", title: "Outreach held" });
    await mount(open, false);
    expect(mark("requested")).toBe("RequestedDay 12");
    expect(mark("slot")).toBe("SlotDay 40");
    expect(mark("due")).toBe("DueDay 47");
    expect(document.querySelector("[data-testid=recovery-gap]")?.textContent).toContain("7 days");
    const hold = document.querySelector("[data-testid=recovery-outreach]");
    expect(hold?.textContent).toContain("Contact permission");
    expect(hold?.textContent).toContain("Unknown");
    expect(hold?.textContent).toContain("Unconfirmed");
    expect(document.querySelector("[data-eligible=false] [data-testid=excluded-by]")?.textContent).toBe("This patient released the slot");
  });

  it("confirms the top case through the existing backfill action without an outreach intent", async () => {
    vi.mocked(confirmBackfill).mockResolvedValue({
      ok: true, status: 201,
      response: { event: { hash: "confirm-hash", at: "2026-11-13T00:00:00Z", type: "waitlist_recovery" }, case_head: "confirm-hash", canonical_unchanged: true, dispatched: false, duplicate: false },
    });
    await mount(open, false);
    await act(async () => button("Confirm backfill")!.click());
    const outreachBox = document.querySelector<HTMLInputElement>("input[type=checkbox]")!;
    expect(outreachBox.disabled).toBe(true);
    await act(async () => button("Confirm simulated backfill")!.click());
    expect(confirmBackfill).toHaveBeenCalledWith(open.slot.slot_id, expect.objectContaining({ case_id: top.case_id, expected_slot_head: open.slot.head, expected_case_head: top.case_head, outreach_intent: false }));
    expect(refresh).toHaveBeenCalled();
  });

  it("marks the slot Assigned after confirmation while outreach stays held", async () => {
    await mount(assigned, false);
    expect(mark("slot")).toBe("AssignedDay 40");
    expect(document.querySelector("[data-testid=recovery-slot-backfill]")?.textContent).toContain("Backfill confirmed");
    expect(button("Confirm backfill")).toBeUndefined();
    const hold = document.querySelector("[data-testid=recovery-outreach]");
    expect(hold?.getAttribute("data-tone")).toBe("held");
    expect(hold?.textContent).toContain("Outreach held");
  });

  it("disables the backfill action in read-only fixture mode", async () => {
    await mount(open, true);
    expect(button("Confirm backfill")?.disabled).toBe(true);
    expect(document.body.textContent).toContain("This demo is read-only");
  });
});
