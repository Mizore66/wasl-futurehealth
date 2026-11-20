// @vitest-environment happy-dom

import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RecoveryBoard } from "@/components/recovery/recovery-board";
import { cancelBackfill } from "@/app/app/recovery/actions";
import { RecoveryDetail, RecoveryList } from "@/lib/api/recovery-schema";

const refresh = vi.fn();
vi.mock("@/app/app/recovery/actions", () => ({ cancelBackfill: vi.fn(), confirmBackfill: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh }), usePathname: () => "/app/recovery" }));
vi.mock("next/link", () => ({ default: (props: Record<string, unknown>) => createElement("a", { href: String(props.href) }, props.children as never) }));

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const read = (name: string) => JSON.parse(readFileSync(path.join(__dirname, "..", "data", "api", name), "utf8"));
const list = RecoveryList.parse(read("recovery_slots.json"));
const base = RecoveryDetail.parse(read("recovery_slot.json"));
const target = base.slot.candidates[0];
const filled = RecoveryDetail.parse({
  ...base,
  slot: {
    ...base.slot, status: "filled", filled_case: target.case_id, head: "filled-slot-head",
    backfill: { case_id: target.case_id, event: "confirm-hash", status: "simulated_backfill_confirmed" },
    history: [{ hash: "confirm-hash", at: "2026-11-13T00:00:00Z", type: "waitlist_recovery", kind: "confirm_backfill", case: target.case_id }],
  },
});

let container: HTMLDivElement | undefined;
let root: Root | undefined;

async function mount(readOnly: boolean) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(RecoveryBoard, { list, detail: filled, readOnly })));
}

const button = (label: string) => [...document.querySelectorAll("button")].find((node) => node.textContent === label);

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  document.body.innerHTML = "";
  vi.mocked(cancelBackfill).mockReset();
  refresh.mockReset();
});

describe("recovery board backfill cancellation", () => {
  it("cancels the confirmed fill with a required reason and the current slot and case heads", async () => {
    vi.mocked(cancelBackfill).mockResolvedValue({
      ok: true, status: 201,
      response: { event: { hash: "cancel-hash", at: "2026-11-14T00:00:00Z", type: "waitlist_recovery" }, case_head: "cancel-hash", canonical_unchanged: true, dispatched: false, duplicate: false },
    });
    await mount(false);
    await act(async () => button("Slot source")!.click());
    expect(document.querySelector("[data-testid=recovery-slot-history]")?.textContent).toContain("confirm backfill");
    await act(async () => button("Cancel backfill")!.click());
    const submit = button("Cancel simulated backfill")!;
    expect(submit.disabled).toBe(true);
    const textarea = document.querySelector("textarea")!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(textarea, "  Patient cannot attend  ");
      textarea.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => button("Cancel simulated backfill")!.click());
    expect(cancelBackfill).toHaveBeenCalledTimes(1);
    const [slotId, input] = vi.mocked(cancelBackfill).mock.calls[0];
    expect(slotId).toBe(filled.slot.slot_id);
    expect(input).toMatchObject({ case_id: target.case_id, expected_slot_head: "filled-slot-head", expected_case_head: target.case_head, reason: "Patient cannot attend" });
    expect(document.body.textContent).toContain("synthetic seat is released");
    expect(refresh).toHaveBeenCalled();
  });

  it("offers no cancellation in read-only fixture mode", async () => {
    await mount(true);
    expect(button("Cancel backfill")).toBeUndefined();
  });
});
