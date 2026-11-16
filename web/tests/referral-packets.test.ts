// @vitest-environment happy-dom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CaseSummary, CurrentDecision } from "@/lib/api/schema";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { packetReferral, referralView, viewScope, type Referral } from "@/components/worklist/referrals";
import { formatDay } from "@/lib/domain/day";
import handoffs from "@/data/referral-handoff-synthetic.json";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push, refresh: vi.fn() }) }));

const control = (suffix: string) => {
  const x = handoffs.find((h) => h.case.id.endsWith(suffix))!;
  return { case: CaseSummary.parse(x.case), current: CurrentDecision.parse(x.current) };
};
const project = (suffix: string) => {
  const x = control(suffix);
  return packetReferral(x.case, x.current, 120, false)!;
};
const LUNG = project("RAD-sr-9101");
const DR = ["9201", "9202", "9203", "9204", "9205"].map((n) => project(`PHC-sr-${n}`));
const CRC_COPY = /FIT|Day 31|SKMC|colonoscopy|endoscopy|\bGP\b/i;

let host: HTMLDivElement;
let root: Root;
function render(referrals: Referral[], scope: string) {
  act(() => root.render(createElement(ReferralInbox, { referrals, window: 7, now: 120, q: "", aside: null, scope, referrer: "referrer" })));
}
const pane = () => host.querySelector<HTMLElement>('section[aria-label="Open referral"]')!;
const text = () => host.textContent ?? "";
function button(name: string | RegExp, scope: ParentNode = host) {
  const found = [...scope.querySelectorAll<HTMLButtonElement>("button")].filter((b) => {
    const label = (b.getAttribute("aria-label") ?? b.textContent ?? "").trim();
    return typeof name === "string" ? label === name : name.test(label);
  });
  expect(found, `button ${String(name)}`).toHaveLength(1);
  return found[0];
}
const click = (name: string | RegExp, scope: ParentNode = host) => act(() => button(name, scope).click());
const detail = (k: string) => [...pane().querySelectorAll("dt")].find((dt) => dt.textContent === k)?.nextElementSibling?.textContent;
const current = () => pane().querySelector('[aria-live="polite"]')!.textContent ?? "";
function show(r: Referral, all = [r]) {
  render(all, "programme=x&provider=y");
  click(/^All \d+$/);
  click(new RegExp(`^${r.name}`));
}

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
  Object.defineProperty(Element.prototype, "animate", { value: undefined, configurable: true, writable: true });
  push.mockReset();
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

describe("#155 lung and diabetic-eye returned referrals", () => {
  it("shows the lung work-up packet with its urgent PET/CT recommendation, referrer and rejecting radiology unit, and no CRC copy", () => {
    show(LUNG);
    expect(detail("Priority")).toBe("Urgent");
    expect(detail("Recommendation")).toBe("PET/CT or tissue sampling for a solid nodule > 8 mm (Fleischner 2017)");
    expect(detail("Responsible clinician")).toBe("Referring clinician, Outpatient imaging centre");
    expect(detail("Returned by")).toBe("Radiology department, regional hospital");
    expect(detail("Reason")).toBe("Not the patient's network");
    expect(text()).not.toMatch(CRC_COPY);
    expect(button(/^Waiting on referrer \d+$/)).toBeTruthy();
  });

  it("matches the DR packet: eye-clinic recommendation, reason, requested information, no-alternative advice and priority", () => {
    const packet = control("PHC-sr-9201").current.handoff!;
    show(DR[0]);
    expect(detail("Recommendation")).toBe(packet.recommendation);
    expect(detail("Reason")).toBe("Needs more information");
    expect(detail("Information requested")).toBe("Reason for referral; Relevant clinical information; Expected outcome");
    expect(detail("Advice to the referrer")).toBe("Referral returned incomplete. Resend with the information requested.");
    expect(detail("Other unit")).toBe("None in the simulated network");
    expect(detail("Responsible clinician")).toBe("Referring clinician, Primary care eye screening");
    expect(detail("Priority")).toBe("Urgent");
    expect(detail("Act by")).toBe(formatDay(packet.act_by));
    expect(DR[3].details).toContainEqual(["Priority", "Routine"]);
    expect(text()).not.toMatch(CRC_COPY);
  });

  it.each([
    [0, "awaiting_referrer", "Referring clinician, Primary care eye screening", "Act on the return", true],
    [1, "coordinator", "Care coordinator", "Escalated to the care coordinator", true],
    [2, "medical_director", "Medical director", "Escalated to the medical director", true],
    [4, "referrer_acted", "Referring clinician, Primary care eye screening", "The referrer has acted", false],
  ] as const)("follows the packet's %s owner and escalation target", (i, status, owner, what, actionable) => {
    const r = DR[i];
    expect(r.handoff?.status).toBe(status);
    show(r);
    expect(current()).toContain(`${owner} · ${what}`);
    expect(current()).toContain(`${r.unit} · Returned the referral`);
    expect([...pane().querySelectorAll("button")].some((b) => b.textContent?.startsWith("Re-refer"))).toBe(actionable);
    if (!actionable) expect(current()).not.toContain("Act on the return");
  });

  it("records a session-only same-unit re-referral, keeps the canonical packet distinct, and Undo restores the exact view", async () => {
    show(DR[0], DR);
    const before = pane().innerHTML;
    const row = () => [...host.querySelectorAll('section[aria-label="Referral list"] button[aria-pressed]')].map((b) => b.textContent);
    const rows = row();
    click("Re-refer to Eye clinic, regional hospital", pane());
    expect(pane().textContent).toContain("recorded in this session only. The canonical packet is unchanged and nothing is sent.");
    expect(current()).toContain("Re-referred to Eye clinic, regional hospital (this session)");
    expect(current()).toContain("The referrer has acted");
    const step = [...pane().querySelectorAll("li")].find((li) => li.textContent?.includes("Re-referred to Eye clinic"))!.textContent;
    expect(step).toContain(DR[0].referrer);
    expect(step).not.toContain("Dr Layla Haddad");
    expect(detail("Status")).toBe("Awaiting the referrer");
    click("Undo", pane());
    await act(async () => void (await new Promise((resolve) => setTimeout(resolve, 200))));
    expect(pane().innerHTML).toBe(before);
    expect(row()).toEqual(rows);
  });

  it("keeps the programme/provider selection on search", () => {
    render(DR, "programme=diabetic_retinopathy&provider=phc-retina");
    expect(text()).toContain("Nothing is saved to the clinical record, booked or sent to the referrer.");
    expect(text()).not.toContain("${referrer}");
    const field = host.querySelector<HTMLInputElement>('input[name="q"]')!;
    expect([...field.form!.querySelectorAll<HTMLInputElement>('input[type="hidden"]')].map((i) => [i.name, i.value])).toEqual([
      ["programme", "diabetic_retinopathy"],
      ["provider", "phc-retina"],
    ]);
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(field, " 9201 ");
      field.dispatchEvent(new Event("input", { bubbles: true }));
    });
    act(() => field.form!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));
    expect(push).toHaveBeenLastCalledWith("/app/referrals?programme=diabetic_retinopathy&provider=phc-retina&q=9201");
  });

  it("names a missing packet field as unavailable, never CRC content, and has no row without a packet", () => {
    const x = control("PHC-sr-9201");
    const bare = CurrentDecision.parse({ ...x.current, handoff: { clinical_priority: "urgent", decided_at: 119, act_by: 123, status: "awaiting_referrer" } });
    const r = packetReferral(x.case, bare, 120, false)!;
    for (const k of ["Recommendation", "Returned by", "Reason", "Advice to the referrer", "Responsible clinician", "Other unit"]) {
      expect(r.details).toContainEqual([k, "Not in the packet"]);
    }
    expect(packetReferral(x.case, CurrentDecision.parse({ ...x.current, handoff: undefined }), 120, false)).toBeNull();
    expect(packetReferral(x.case, null, 120, false)).toBeNull();
  });

  it("addresses views by programme and provider with CRC as the default", () => {
    expect(referralView()?.programme).toBe("crc");
    expect(referralView("lung_nodule", "rad-imaging")?.unit).toBe("Radiology department, regional hospital");
    expect(referralView("lung_nodule", "ahs-mushrif")).toBeNull();
    expect(viewScope()).toBe("");
    expect(viewScope("lung_nodule", "rad-imaging")).toBe("programme=lung_nodule&provider=rad-imaging");
  });
});
