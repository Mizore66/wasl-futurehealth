// @vitest-environment happy-dom
import { createElement, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CaseSummary } from "@/lib/api/schema";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { referralFrom, type Referral } from "@/components/worklist/referrals";
import type { RespondResult } from "@/app/app/referrals/actions";
import synthetic from "@/data/referral-inbox-synthetic.json";

const refresh = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: () => undefined, refresh }) }));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const NOW = 120;
const open = CaseSummary.parse(synthetic[0]);
const HEAD = open.api_overlay!.head;
const fresh = () => referralFrom(open, NOW, false)!;

let host: HTMLDivElement;
let root: Root;
const respond = vi.fn<(id: string, head: string, response: unknown) => Promise<RespondResult>>();

function render(referrals: Referral[]) {
  act(() => root.render(createElement(ReferralInbox, { referrals, window: 2, now: NOW, q: "", aside: null, respond })));
}
const pane = () => host.querySelector<HTMLElement>('section[aria-label="Open referral"]')!;
const button = (name: string) => {
  const found = [...pane().querySelectorAll<HTMLButtonElement>("button")].filter((b) => (b.textContent ?? "").trim() === name);
  expect(found, `button ${name}`).toHaveLength(1);
  return found[0];
};
const click = async (el: HTMLElement) => act(async () => el.click());

beforeEach(() => {
  Object.defineProperty(Element.prototype, "animate", { value: undefined, configurable: true, writable: true });
  respond.mockReset();
  refresh.mockReset();
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

describe("live referral answers", () => {
  it("sends Accept bound to the case head and claims it only after the write succeeds, with no local Undo", async () => {
    respond.mockResolvedValue({ ok: true });
    render([fresh()]);
    await click(button("Accept referral"));
    expect(respond).toHaveBeenCalledExactlyOnceWith(open.id, HEAD, { response: "accept" });
    expect(refresh).toHaveBeenCalledOnce();
    expect(pane().textContent).toContain("Saved to the case ledger");
    expect([...pane().querySelectorAll("button")].some((b) => b.textContent === "Undo")).toBe(false);
  });

  it("sends structured condition codes, not their labels", async () => {
    respond.mockResolvedValue({ ok: true });
    render([fresh()]);
    await click(button("Accept with conditions"));
    await click(button("Arabic interpreter on the day"));
    await click(button("Kidney function before booking"));
    await click(button("Accept with 2 conditions"));
    expect(respond).toHaveBeenCalledExactlyOnceWith(open.id, HEAD, { response: "accept_with_conditions", conditions: ["arabic_interpreter", "kidney_function_check"] });
  });

  it("sends a return with its reason code", async () => {
    respond.mockResolvedValue({ ok: true });
    render([fresh()]);
    await click(button("Return to GP"));
    await click(button("Blood thinner plan missing"));
    await click(button("Return to GP"));
    expect(respond).toHaveBeenCalledExactlyOnceWith(open.id, HEAD, { response: "return", reason: "anticoagulant_plan_missing" });
  });

  it("leaves the referral unanswered and says why when the write is refused", async () => {
    respond.mockResolvedValue({ ok: false, status: 409, error: "stale head", conflict: "head" });
    render([fresh()]);
    await click(button("Accept referral"));
    expect(refresh).not.toHaveBeenCalled();
    expect(pane().textContent).toContain("Not saved");
    expect(button("Accept referral")).toBeTruthy();
    expect(pane().textContent).not.toContain("Saved to the case ledger");
  });

  it("sends nothing for a referral the API ledger does not hold", async () => {
    render([{ ...fresh(), head: null }]);
    await click(button("Accept referral"));
    expect(respond).not.toHaveBeenCalled();
    expect(pane().textContent).toContain("Not saved");
  });
});

describe("persisted referral answers", () => {
  const answered = (response: Record<string, unknown>, handoff?: Record<string, unknown>) =>
    CaseSummary.parse({
      ...synthetic[0],
      api_overlay: {
        ...synthetic[0].api_overlay,
        referral_response: { unit: "skmc-endoscopy", run_day: NOW, at: "2026-11-16T00:00:00Z", actor: "clinician-1", event: "e1", ...response },
        ...(handoff && { referral_handoff: handoff }),
      },
    });

  it("places an accepted answer in Accepted with its structured conditions, after a reload", () => {
    const r = referralFrom(answered({ response: "accept_with_conditions", conditions: ["female_endoscopist", "hold_anticoagulants"] }), NOW, false)!;
    expect(r.state).toBe("accepted");
    expect(r.answer).toBe("Accepted with conditions: Female endoscopist; Hold blood thinners as advised");
  });

  it("places a return in Waiting on GP with the projected hand-off clock", async () => {
    const handoff = { referral_id: open.id, reason_code: "wrong_unit", responsible_clinician: "gp-1", decided_by: "clinician-1", decided_at: NOW, act_by: 125, medical_director_day: 130, status: "awaiting_referrer" };
    const r = referralFrom(answered({ response: "return", reason: "wrong_unit" }, handoff), NOW, false)!;
    expect(r.state).toBe("returned");
    expect(r.handoff).toEqual({ status: "awaiting_referrer", actBy: 125 });
    render([r]);
    await click([...host.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find((b) => b.textContent?.startsWith("Waiting on GP"))!);
    expect(pane().textContent).toContain("Returned to the GP: wrong unit");
  });
});
