// @vitest-environment happy-dom
import { createElement, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReferrerHandoffs } from "@/components/worklist/referrer-handoffs";
import { returnedPacket, type ReturnedPacket } from "@/components/worklist/returned-packets";
import { CaseSummary, CurrentDecision } from "@/lib/api/schema";
import handoffs from "@/data/referral-handoff-synthetic.json";
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
  source: "unit_return",
  unit: "SKMC endoscopy",
  unitId: "skmc-endo",
  reason: "Blood thinner plan missing",
  recommendation: null,
  advice: null,
  urgent: true,
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
    expect(host.querySelector('[role="alert"]')?.textContent).toContain("not this patient's referring clinician");
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

  it("answers a canonical lung work-up packet: same-unit re-referral only, no CRC copy", async () => {
    const x = handoffs.find((h) => h.case.id.endsWith("RAD-sr-9101"))!;
    const lung = returnedPacket(CaseSummary.parse(x.case), CurrentDecision.parse(x.current))!;
    expect(lung).toMatchObject({ source: "canonical", unitId: "mafraq-rad", urgent: true, others: [], status: "awaiting_referrer", answer: null });
    answer.mockResolvedValue({ ok: true });
    act(() => root.render(createElement(ReferrerHandoffs, { packets: [lung], answer, referrer: "referrer" })));
    expect(host.textContent).toContain("PET/CT or tissue sampling");
    expect(host.textContent).toContain("Urgent");
    expect(host.textContent).not.toMatch(/FIT|Day 31|SKMC|colonoscopy|endoscopy|\bGP\b/);
    expect([...host.querySelectorAll("label")].some((l) => l.textContent?.startsWith("Refer to another unit"))).toBe(false);
    expect([...host.querySelectorAll("label")].some((l) => l.textContent?.startsWith("Send the information requested"))).toBe(false);
    await act(async () => radio("Re-refer to the same unit").click());
    await act(async () => button("Send answer").click());
    expect(answer).toHaveBeenCalledWith(x.case.id, lung.head, lung.packet, { response: "re_refer", destination: "mafraq-rad" });
  });

  it("asks for every item a canonical DR packet requested before sending", async () => {
    const x = handoffs.find((h) => h.case.id.endsWith("PHC-sr-9203"))!;
    const dr = returnedPacket(CaseSummary.parse(x.case), CurrentDecision.parse(x.current))!;
    expect(dr).toMatchObject({ source: "canonical", unitId: "eye-hosp", status: "medical_director", infoRequested: ["reason_for_referral", "relevant_clinical_information", "expected_outcome"] });
    act(() => root.render(createElement(ReferrerHandoffs, { packets: [dr], answer, referrer: "referrer" })));
    expect(host.textContent).toContain("Escalated to the medical director");
    await act(async () => radio("Send the information requested").click());
    await act(async () => button("Send answer").click());
    expect(host.querySelector('[role="alert"]')?.textContent).toBe("Fill in: Reason for referral, Relevant clinical information, Expected outcome.");
    expect(answer).not.toHaveBeenCalled();
  });
});
