// @vitest-environment happy-dom
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CaseSummary, CurrentDecision } from "@/lib/api/schema";
import { mountInbox, push, type Harness } from "./referral-inbox-harness";
import { packetReferral, referralView, viewScope, type Referral } from "@/components/worklist/referrals";
import { formatDay } from "@/lib/domain/day";
import handoffs from "@/data/referral-handoff-synthetic.json";

vi.mock("next/navigation", async () => (await import("./referral-nav-mock")).navigationMock);

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

let h: Harness;
function render(referrals: Referral[], scope: string) {
  h.render({ referrals, window: 7, now: 120, q: "", scope, referrer: "referrer" });
}
const pane = () => h.pane()!;
const text = () => h.text();
const click = (name: string | RegExp, scope?: ParentNode) => h.click(name, scope);
const detail = (k: string) => [...pane().querySelectorAll("#rf-packet dt")].find((dt) => dt.textContent === k)?.nextElementSibling?.textContent;
/** The hand-off activity, from the referral to where it waits now. */
const current = () => pane().querySelector("#rf-activity")!.textContent ?? "";
function show(r: Referral, all = [r]) {
  render(all, "programme=x&provider=y");
  h.go(`/app/referrals?programme=x&provider=y&case=${encodeURIComponent(r.id)}`);
  expect(h.openId()).toBe(r.id);
}

beforeEach(() => {
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() }));
  h = mountInbox();
});
afterEach(() => {
  h.unmount();
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
    expect(h.count("waiting")).toBe(1);
    expect(pane().textContent).toContain("Waiting on referrer");
    expect(h.host.querySelector('[data-testid="handoff-arrow"]')!.getAttribute("data-direction")).toBe("back");
  });

  it("matches the DR packet: eye-clinic recommendation, reason, requested information, no-alternative advice and priority", () => {
    const packet = control("PHC-sr-9201").current.handoff!;
    show(DR[0]);
    expect(detail("Recommendation")).toBe(packet.recommendation);
    expect(detail("Reason")).toBe("Needs more information");
    expect(detail("Information requested")).toBe("Reason for referral; Relevant clinical information; Expected outcome");
    expect(detail("Advice to the referrer")).toBe("Referral returned incomplete. Resend with the information requested.");
    expect(detail("Other unit")).toBe("None suggested");
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
    expect(current()).toContain(`Now: ${what}${owner}`);
    expect(current()).toContain(`Returned the referral: ${r.details.find(([k]) => k === "Reason")![1].toLowerCase()}${r.unit}`);
    expect([...pane().querySelectorAll("button")].some((b) => b.textContent?.startsWith("Re-refer"))).toBe(actionable);
    if (!actionable) expect(current()).not.toContain("Act on the return");
  });

  it("records a session-only same-unit re-referral, keeps the canonical packet distinct, and Undo restores the exact view", async () => {
    show(DR[0], DR);
    const before = pane().innerHTML;
    const row = () => [...h.host.querySelectorAll('section[aria-label="Referral inbox"] li a')].map((a) => a.textContent);
    const rows = row();
    click("Re-refer to Eye clinic, regional hospital", pane());
    expect(pane().textContent).toContain("recorded on this page only. Nothing is saved or sent.");
    expect(current()).toContain("Re-referred to Eye clinic, regional hospital (this page only)");
    expect(current()).toContain("The referrer has acted");
    const step = [...pane().querySelectorAll("#rf-activity li")].find((li) => li.textContent?.includes("Re-referred to Eye clinic"))!.textContent;
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
    h.go("/app/referrals?programme=diabetic_retinopathy&provider=phc-retina&view=inbox");
    expect(text()).toContain("Nothing is saved to the clinical record, booked or sent to the referrer.");
    expect(text()).not.toContain("${referrer}");
    const field = h.host.querySelector<HTMLInputElement>('input[name="q"]')!;
    expect([...field.form!.querySelectorAll<HTMLInputElement>('input[type="hidden"]')].map((i) => [i.name, i.value])).toEqual([
      ["programme", "diabetic_retinopathy"],
      ["provider", "phc-retina"],
      ["view", "inbox"],
    ]);
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(field, " 9201 ");
      field.dispatchEvent(new Event("input", { bubbles: true }));
    });
    act(() => field.form!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));
    expect(push).toHaveBeenLastCalledWith("/app/referrals?programme=diabetic_retinopathy&provider=phc-retina&q=9201&view=inbox");
  });

  it("names a missing packet field as unavailable, never CRC content, and has no row without a packet", () => {
    const x = control("PHC-sr-9201");
    const bare = CurrentDecision.parse({ ...x.current, handoff: { clinical_priority: "urgent", decided_at: 119, act_by: 123, status: "awaiting_referrer" } });
    const r = packetReferral(x.case, bare, 120, false)!;
    for (const k of ["Recommendation", "Returned by", "Reason", "Advice to the referrer", "Responsible clinician", "Other unit"]) {
      expect(r.details).toContainEqual([k, "Not recorded"]);
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

  it("follows the referrer's ledger answer to the canonical packet without rewriting it (#154)", () => {
    const x = control("PHC-sr-9203");
    const h = x.case.api_overlay!.referral_handoff!;
    expect(h).toMatchObject({ source: "canonical", event: x.current.handoff!.referral_id, status: "medical_director" });
    const answered = CaseSummary.parse({
      ...x.case,
      api_overlay: {
        ...x.case.api_overlay,
        referral_handoff: { ...h, status: "referrer_acted", acted_at: 120, referrer_response: { response: "re_refer", destination: "eye-hosp", at: "2026-11-17T10:00:00Z", actor: "Dr K", run_day: 120, event: "e1" } },
      },
    });
    const r = packetReferral(answered, x.current, 120, false)!;
    expect(r.handoff?.status).toBe("referrer_acted");
    expect(r.details).toContainEqual(["Referrer's answer", expect.stringMatching(/^Re-refer to the same unit: Eye clinic, regional hospital\. Answered/)]);
    expect(x.current.handoff!.status).toBe("medical_director");
    // An answer bound to another packet is not this packet's answer.
    const other = CaseSummary.parse({ ...answered, api_overlay: { ...answered.api_overlay, referral_handoff: { ...answered.api_overlay!.referral_handoff!, event: "other-packet" } } });
    expect(packetReferral(other, x.current, 120, false)!.handoff?.status).toBe("medical_director");
  });
});
