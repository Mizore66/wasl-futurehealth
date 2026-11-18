// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TeachBackCallback } from "@/lib/api/schema";

const actions = vi.hoisted(() => ({ recordTeachBackCallback: vi.fn() }));
vi.mock("@/app/app/patients/[id]/actions", () => actions);

const { TeachBackCallbackCard } = await import("@/components/case/teach-back-callback-card");

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

let container: HTMLDivElement | undefined;
let root: Root | undefined;

afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  container?.remove();
  root = undefined;
  container = undefined;
  vi.clearAllMocks();
});

const open: TeachBackCallback = { id: "cb-1", source: "patient_link", state: "misunderstood", owner: "nurse", status: "open", escalation: "nurse callback", due_at: "2026-11-19T16:02:00Z" };

/* N1: after "understood on the call" the item shows resolved and its buttons go, before the page revalidates. */
describe("TeachBackCallbackCard after a recorded outcome", () => {
  it("shows the 201 response's resolved callback without waiting for revalidation", async () => {
    actions.recordTeachBackCallback.mockResolvedValue({ ok: true, callbacks: [{ ...open, status: "resolved", resolved_by: "coordinator-1" }] });
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
    await act(async () => root!.render(createElement(TeachBackCallbackCard, { caseId: "c1", callbacks: [open], live: true })));
    expect(container.textContent).toContain("Open · nurse callback due");

    const understood = [...container.querySelectorAll("button")].find((b) => b.textContent === "Record: understood on the call")!;
    await act(async () => understood.click());

    expect(actions.recordTeachBackCallback).toHaveBeenCalledWith({ caseId: "c1", callback: "cb-1", outcome: "understood_on_call" });
    expect(container.textContent).toContain("Resolved by coordinator-1");
    expect(container.textContent).not.toContain("Open · nurse callback due");
    expect(container.querySelectorAll("button")).toHaveLength(0);
    expect(container.textContent).toContain("Callback resolved.");
  });
});
