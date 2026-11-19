// @vitest-environment happy-dom
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CaseSummary } from "@/lib/api/schema";
import { referralFrom, type Referral } from "@/components/worklist/referrals";
import type { RespondResult } from "@/app/app/referrals/actions";
import synthetic from "@/data/referral-inbox-synthetic.json";
import { mountInbox, refresh, type Harness } from "./referral-inbox-harness";

vi.mock("next/navigation", async () => (await import("./referral-nav-mock")).navigationMock);

const NOW = 120;
const open = CaseSummary.parse(synthetic[0]);
const HEAD = open.api_overlay!.head;
const fresh = () => referralFrom(open, NOW, false)!;

let h: Harness;
const respond = vi.fn<(id: string, head: string, response: unknown) => Promise<RespondResult>>();
const render = (referrals: Referral[]) => h.render({ referrals, window: 2, now: NOW, q: "", respond });
const flush = () => act(async () => void (await new Promise((r) => setTimeout(r, 0))));

beforeEach(() => {
  respond.mockReset();
  h = mountInbox();
});
afterEach(() => h.unmount());

describe("live referral answers", () => {
  it("sends Accept bound to the case head and claims it only after the write succeeds, with no local Undo", async () => {
    respond.mockResolvedValue({ ok: true });
    render([fresh()]);
    h.click("Accept referral");
    await flush();
    expect(respond).toHaveBeenCalledExactlyOnceWith(open.id, HEAD, { response: "accept" });
    expect(refresh).toHaveBeenCalledOnce();
    expect(h.paneText()).toContain("Saved to the case ledger");
    expect([...h.pane()!.querySelectorAll("button")].some((b) => b.textContent === "Undo")).toBe(false);
  });

  it("sends structured condition codes, not their labels", async () => {
    respond.mockResolvedValue({ ok: true });
    render([fresh()]);
    h.click("Accept with conditions");
    h.click("Arabic interpreter on the day");
    h.click("Kidney function before booking");
    h.click("Accept with 2 conditions");
    await flush();
    expect(respond).toHaveBeenCalledExactlyOnceWith(open.id, HEAD, { response: "accept_with_conditions", conditions: ["arabic_interpreter", "kidney_function_check"] });
  });

  it("sends a return with its reason code, and nothing without one", async () => {
    respond.mockResolvedValue({ ok: true });
    render([fresh()]);
    h.click("Return with reason");
    h.click("Return to GP");
    expect(respond).not.toHaveBeenCalled();
    h.radio("Blood thinner plan missing");
    h.click("Return to GP");
    await flush();
    expect(respond).toHaveBeenCalledExactlyOnceWith(open.id, HEAD, { response: "return", reason: "anticoagulant_plan_missing" });
  });

  it("leaves the referral unanswered and says why when the write is refused", async () => {
    respond.mockResolvedValue({ ok: false, status: 409, error: "stale head", conflict: "head" });
    render([fresh()]);
    h.click("Accept referral");
    await flush();
    expect(refresh).not.toHaveBeenCalled();
    expect(h.paneText()).toContain("Not saved");
    expect(h.button("Accept referral")).toBeTruthy();
    expect(h.paneText()).not.toContain("Saved to the case ledger");
  });

  it("sends nothing for a referral the API ledger does not hold", async () => {
    render([{ ...fresh(), head: null }]);
    h.click("Accept referral");
    await flush();
    expect(respond).not.toHaveBeenCalled();
    expect(h.paneText()).toContain("Not saved");
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
  const handoff = { referral_id: open.id, reason_code: "wrong_unit", responsible_clinician: "gp-1", decided_by: "clinician-1", decided_at: NOW, act_by: 125, medical_director_day: 130, status: "awaiting_referrer" };

  it("places an accepted answer in Accepted with its structured conditions, after a reload", () => {
    const r = referralFrom(answered({ response: "accept_with_conditions", conditions: ["female_endoscopist", "hold_anticoagulants"] }), NOW, false)!;
    expect(r.state).toBe("accepted");
    expect(r.answer).toBe("Accepted with conditions: Female endoscopist; Hold blood thinners as advised");
  });

  it("places a return in Waiting on GP with the projected hand-off clock, the arrow pointing back to the GP", () => {
    const r = referralFrom(answered({ response: "return", reason: "wrong_unit" }, handoff), NOW, false)!;
    expect(r.state).toBe("returned");
    expect(r.handoff).toEqual({ status: "awaiting_referrer", actBy: 125 });
    expect(r.acknowledged).toBe(false);
    render([r]);
    expect(h.count("waiting")).toBe(1);
    expect(h.paneText()).toContain("Returned to the GP: wrong unit");
    expect(h.paneText()).toContain("Booking held");
    expect(h.host.querySelector('[data-testid="handoff-arrow"]')!.getAttribute("data-direction")).toBe("back");
  });

  it("shows the GP's acknowledgement once the ledger holds it: the status, the GP party and the activity", () => {
    const ack = { response: "acknowledge", at: "2026-11-16T01:00:00Z", actor: "gp-1", run_day: NOW, event: "e2" };
    const r = referralFrom(answered({ response: "return", reason: "wrong_unit" }, { ...handoff, acknowledgement: ack }), NOW, false)!;
    expect(r.acknowledged).toBe(true);
    render([r]);
    expect(h.count("acknowledged")).toBe(1);
    expect(h.count("waiting")).toBe(0);
    expect(h.paneText()).toContain("Acknowledged by GP");
    expect(h.paneText()).toContain("The GP acknowledged the return");
    // Acknowledgement alone leaves the hand-off waiting on the GP.
    expect(h.paneText()).toContain("Act on the return");
    h.go("/app/referrals?view=inbox");
    expect(h.host.querySelector("tbody")!.textContent).toContain("Acknowledged by GP");
  });

  it("links the GP view with a way back to the exact inbox state", () => {
    const r = referralFrom(answered({ response: "return", reason: "wrong_unit" }, handoff), NOW, false)!;
    render([r]);
    h.go(`/app/referrals?status=waiting&from=inbox&case=${encodeURIComponent(r.id)}`);
    const gp = [...h.host.querySelectorAll<HTMLAnchorElement>("a")].find((a) => a.textContent?.startsWith("Open the GP view"))!;
    const target = new URL(gp.href, "http://x");
    expect(target.pathname).toBe("/app/referrals/returned");
    expect(target.searchParams.get("case")).toBe(r.id);
    expect(target.searchParams.get("back")).toBe(`/app/referrals?status=waiting&view=inbox&case=${encodeURIComponent(r.id)}`);
  });
});
