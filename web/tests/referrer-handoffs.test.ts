// @vitest-environment happy-dom
import { createElement, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReferrerHandoffs } from "@/components/worklist/referrer-handoffs";
import type { ReturnedPacket } from "@/components/worklist/returned-packets";
import type { RespondResult } from "@/app/app/referrals/actions";

const refresh = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: () => undefined, refresh }) }));
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const packet: ReturnedPacket = {
  id: "case-1",
  name: "Mariam",
  short: "sr-1",
  head: "h3",
  packet: "ret-1",
  unit: "SKMC endoscopy",
  unitId: "skmc-endo",
  reason: "Blood thinner plan missing",
  infoRequested: ["anticoagulant_plan"],
  others: [{ id: "noor-endo", label: "Noor endoscopy" }],
  actBy: 112,
  status: "coordinator",
  acknowledged: false,
  answer: null,
};

let host: HTMLDivElement;
let root: Root;
const answer = vi.fn<(id: string, head: string, packet: string, response: unknown) => Promise<RespondResult>>();
const render = (packets: ReturnedPacket[], live = true) => act(() => root.render(createElement(ReferrerHandoffs, { packets, answer: live ? answer : undefined })));
const button = (name: string) => [...host.querySelectorAll("button")].find((b) => b.textContent?.trim() === name)!;
const radio = (label: string) => [...host.querySelectorAll("label")].find((l) => l.textContent?.startsWith(label))!.querySelector("input")!;
const type = (el: HTMLTextAreaElement, value: string) => {
  Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(el, value);
  el.dispatchEvent(new Event("input", { bubbles: true }));
};

beforeEach(() => {
  answer.mockReset();
  refresh.mockReset();
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

describe("ReferrerHandoffs", () => {
  it("acknowledges the exact packet without resolving it", async () => {
    answer.mockResolvedValue({ ok: true });
    render([packet]);
    await act(async () => button("Acknowledge the returned referral").click());
    expect(answer).toHaveBeenCalledWith("case-1", "h3", "ret-1", { response: "acknowledge" });
    expect(host.querySelector('[role="status"]')?.textContent).toContain("still waits on your answer");
    expect(refresh).toHaveBeenCalledOnce();
  });

  it("refuses an incomplete answer, then sends the complete one", async () => {
    answer.mockResolvedValue({ ok: true });
    render([packet]);
    await act(async () => radio("Send the information requested").click());
    await act(async () => button("Send answer").click());
    expect(host.querySelector('[role="alert"]')?.textContent).toBe("Fill in: Blood thinner plan.");
    expect(answer).not.toHaveBeenCalled();
    await act(async () => type(host.querySelector("textarea")!, "Hold apixaban 48 hours"));
    await act(async () => button("Send answer").click());
    expect(answer).toHaveBeenCalledWith("case-1", "h3", "ret-1", { response: "supply_info", supplied_info: { anticoagulant_plan: "Hold apixaban 48 hours" } });
  });

  it("shows a refusal and records nothing locally", async () => {
    answer.mockResolvedValue({ ok: false, status: 403, error: "x" });
    render([packet]);
    await act(async () => radio("Re-refer to the same unit").click());
    await act(async () => button("Send answer").click());
    expect(answer).toHaveBeenCalledWith("case-1", "h3", "ret-1", { response: "re_refer", destination: "skmc-endo" });
    expect(host.querySelector('[role="alert"]')?.textContent).toContain("not this patient's referring GP");
    expect(refresh).not.toHaveBeenCalled();
  });

  it("shows a resolved packet as answered with no controls, and offers no controls without the live API", () => {
    render([{ ...packet, acknowledged: true, status: "referrer_acted", answer: "Close the referral. Reason: done" }]);
    expect(host.textContent).toContain("Close the referral. Reason: done");
    expect(button("Send answer")).toBeUndefined();
    render([packet], false);
    expect(button("Send answer")).toBeUndefined();
    expect(host.textContent).toContain("need the live synthetic API");
  });
});
