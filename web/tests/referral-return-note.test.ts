// @vitest-environment happy-dom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CaseSummary, ReferralHandoff, ReferralResponse, ReferralResponseRequest, RETURN_NOTE_MAX } from "@/lib/api/schema";
import { referralFrom, referralStatus, withSessionReturn, type Referral } from "@/components/worklist/referrals";
import { needsDetail, returnedPacket, sessionPacket } from "@/components/worklist/returned-packets";
import { SessionReferrerHandoffs } from "@/components/worklist/referrer-handoffs";
import { acknowledgeSessionReturn, readSessionReturns, removeSessionReturn, saveSessionReturn, type SessionReturn } from "@/components/worklist/session-returns";
import type { RespondResult } from "@/app/app/referrals/actions";
import synthetic from "@/data/referral-inbox-synthetic-v2.json";
import { mountInbox, type Harness } from "./referral-inbox-harness";

vi.mock("next/navigation", async () => (await import("./referral-nav-mock")).navigationMock);

/* Gap 1: the unit's free-text note to the GP on a return. Gap 2: the fixtures-mode return and GP acknowledgement kept in
   this browser. The API's own rules are pinned in pipeline/api_referral_note_test.go. */

const NOW = 120;
const NOTE = "Please attach the FIT report.";
const ALL: Referral[] = synthetic.map((c) => referralFrom(CaseSummary.parse(c), NOW, false)).filter((r): r is Referral => r !== null);
const fresh = ALL.find((r) => r.state === "new")!;

const type = (el: HTMLTextAreaElement, value: string) => {
  act(() => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  });
};
const noteBox = (h: Harness) => h.pane()!.querySelector<HTMLTextAreaElement>('textarea[name="note"]')!;
const submit = (h: Harness) => h.click("Return to GP");
const flush = () => act(async () => void (await new Promise((r) => setTimeout(r, 0))));

describe("return note: request and projection shapes", () => {
  it("the request takes an optional bounded note beside a required reason", () => {
    expect(ReferralResponseRequest.safeParse({ response: "return", reason: "needs_clinic_visit", note: NOTE }).success).toBe(true);
    expect(ReferralResponseRequest.safeParse({ response: "return", reason: "needs_clinic_visit" }).success).toBe(true);
    expect(ReferralResponseRequest.safeParse({ response: "return", note: NOTE }).success).toBe(false);
    expect(ReferralResponseRequest.safeParse({ response: "return", reason: "needs_clinic_visit", note: "x".repeat(RETURN_NOTE_MAX + 1) }).success).toBe(false);
    expect(ReferralResponseRequest.safeParse({ response: "return", reason: "needs_clinic_visit", note: "   " }).success).toBe(false);
  });

  it("old projections without a note still parse; a noted one carries it or its list marker", () => {
    const base = { response: "return", reason: "wrong_unit", unit: "skmc-endo", run_day: 110, at: "2026-11-16T10:00:00Z", actor: "Dr Layla Haddad", event: "e1" };
    expect(ReferralResponse.parse(base).note).toBeUndefined();
    expect(ReferralResponse.parse({ ...base, note: NOTE }).note).toBe(NOTE);
    expect(ReferralResponse.parse({ ...base, has_note: true }).has_note).toBe(true);
  });

  it("the GP's packet takes the note from the case detail and asks for the detail when the list only marks one", () => {
    const listed = CaseSummary.parse(synthetic.find((c) => (c as { api_overlay?: { referral_response?: unknown } }).api_overlay?.referral_response === undefined)!);
    const handoff = ReferralHandoff.parse({
      referral_id: listed.id, source: "unit_return", reason_code: "needs_more_info", responsible_clinician: "referring PCP, ahs-mushrif", decided_by: "skmc-endo",
      decided_at: 110, act_by: 112, medical_director_day: 116, status: "awaiting_referrer", event: "ret-1", has_note: true,
    });
    const response = ReferralResponse.parse({ response: "return", reason: "needs_clinic_visit", unit: "skmc-endo", run_day: 110, at: "t", actor: "a", event: "ret-1", has_note: true });
    const list: CaseSummary = { ...listed, closed: false, api_overlay: { ...listed.api_overlay!, referral_response: response, referral_handoff: handoff } };
    expect(needsDetail(list)).toBe(true);
    const fromList = returnedPacket(list)!;
    expect(fromList.hasNote).toBe(true);
    expect(fromList.note).toBeNull();
    const detail = { ...list, api_overlay: { ...list.api_overlay!, referral_response: { ...response, note: NOTE, has_note: undefined }, referral_handoff: { ...handoff, note: NOTE, has_note: undefined } } };
    expect(returnedPacket(detail)!.note).toBe(NOTE);
    // A unit return without a note needs no detail read.
    expect(needsDetail({ ...list, api_overlay: { ...list.api_overlay!, referral_handoff: { ...handoff, has_note: undefined } } })).toBe(false);
  });
});

describe("return form", () => {
  let h: Harness;
  const respond = vi.fn<(id: string, head: string, response: unknown) => Promise<RespondResult>>();
  beforeEach(() => {
    respond.mockReset();
    h = mountInbox();
  });
  afterEach(() => h.unmount());

  it("live: an empty submission stays blocked even with a note; the note is sent trimmed beside the reason", async () => {
    respond.mockResolvedValue({ ok: true });
    h.render({ referrals: [fresh], window: 2, now: NOW, q: "", respond });
    h.click("Return with reason");
    type(noteBox(h), NOTE);
    submit(h);
    expect(h.paneText()).toContain("Choose a reason before you return it. Nothing was sent.");
    expect(respond).not.toHaveBeenCalled();
    h.radio("Needs a clinic visit first");
    type(noteBox(h), `  ${NOTE}\n`);
    submit(h);
    await flush();
    expect(respond).toHaveBeenCalledExactlyOnceWith(fresh.id, fresh.head, { response: "return", reason: "needs_clinic_visit", note: NOTE });
  });

  it("live: a blank note is not sent", async () => {
    respond.mockResolvedValue({ ok: true });
    h.render({ referrals: [fresh], window: 2, now: NOW, q: "", respond });
    h.click("Return with reason");
    h.radio("Wrong unit");
    type(noteBox(h), "   ");
    submit(h);
    await flush();
    expect(respond).toHaveBeenCalledExactlyOnceWith(fresh.id, fresh.head, { response: "return", reason: "wrong_unit" });
  });

  it("the note box is bounded and says how much is left", () => {
    h.render({ referrals: [fresh], window: 2, now: NOW, q: "" });
    h.click("Return with reason");
    const box = noteBox(h);
    expect(box.maxLength).toBe(RETURN_NOTE_MAX);
    type(box, NOTE);
    expect(h.paneText()).toContain(`${RETURN_NOTE_MAX - NOTE.length} of ${RETURN_NOTE_MAX} characters left`);
    expect(h.host.querySelector(`#${box.getAttribute("aria-describedby")}`)).toBeTruthy();
  });

  it("fixtures: the return and its note are kept in this browser, and the GP's acknowledgement shows in the inbox", async () => {
    h.render({ referrals: ALL, window: 2, now: NOW, q: "" });
    h.open(fresh.id);
    h.click("Return with reason");
    submit(h);
    expect(readSessionReturns()[fresh.id]).toBeUndefined();
    h.radio("Needs a clinic visit first");
    type(noteBox(h), NOTE);
    submit(h);
    const kept = readSessionReturns()[fresh.id];
    expect(kept).toMatchObject({ reason: "needs_clinic_visit", note: NOTE, short: fresh.short });
    expect(kept.acknowledgedAt).toBeUndefined();
    expect(h.paneText()).toContain(`Note to the GP: ${NOTE}`);
    expect(h.paneText()).toContain("Kept in this browser only");
    expect(h.paneText()).toContain("Waiting on GP");

    // The GP acknowledges on their view (same browser); the inbox follows the stored session.
    act(() => acknowledgeSessionReturn(fresh.id));
    expect(h.paneText()).toContain("Acknowledged by GP");
    expect(h.paneText()).toContain("The GP acknowledged the return");
    expect(h.paneText()).toContain("Kept in this browser");

    // A fresh inbox (navigating back) still shows it from the session store.
    h.unmount();
    h = mountInbox();
    saveSessionReturn({ ...kept, acknowledgedAt: "2026-11-20T08:00:00Z" }); // mountInbox clears storage; restore the session
    h.render({ referrals: ALL, window: 2, now: NOW, q: "" });
    h.open(fresh.id);
    expect(h.paneText()).toContain("Acknowledged by GP");
    expect(h.paneText()).toContain(`Note to the GP: ${NOTE}`);
    expect(h.count("acknowledged")).toBe(1);
  });

  it("fixtures: Undo drops the kept return", async () => {
    h.render({ referrals: ALL, window: 2, now: NOW, q: "" });
    h.open(fresh.id);
    h.click("Return with reason");
    h.radio("Wrong unit");
    submit(h);
    expect(readSessionReturns()[fresh.id]).toBeTruthy();
    h.click("Undo");
    expect(readSessionReturns()[fresh.id]).toBeUndefined();
    await h.settled(() => h.paneText().includes("Accept"));
  });

  it("live mode never reads the browser store", () => {
    saveSessionReturn({ id: fresh.id, name: fresh.name, short: fresh.short, clinic: fresh.clinic, unit: fresh.unit, reason: "wrong_unit", urgent: false, actBy: 125 });
    h.render({ referrals: [fresh], window: 2, now: NOW, q: "", respond });
    expect(h.paneText()).toContain("Needs response");
    expect(h.paneText()).not.toContain("Kept in this browser");
  });
});

describe("session returns: the GP view in this browser", () => {
  const kept: SessionReturn = { id: "case-9", name: "Mariam Al Hosani", short: "AHS-sr-9", clinic: "Al Mushrif", unit: "SKMC endoscopy", reason: "needs_clinic_visit", note: NOTE, urgent: true, actBy: 122 };
  let host: HTMLDivElement;
  let root: Root;
  beforeEach(() => {
    sessionStorage.clear();
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
  });
  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  it("maps a kept return onto the inbox and the GP's packet", () => {
    const r = withSessionReturn(fresh, kept.id === fresh.id ? kept : { ...kept, id: fresh.id });
    expect(r).toMatchObject({ state: "returned", kept: "browser", note: NOTE, acknowledged: false, answerWhen: "Kept in this browser" });
    expect(referralStatus(r).label).toBe("Waiting on GP");
    expect(referralStatus(withSessionReturn(fresh, { ...kept, id: fresh.id, acknowledgedAt: "t" })).label).toBe("Acknowledged by GP");
    expect(sessionPacket(kept)).toMatchObject({ reason: "Needs a clinic visit first", note: NOTE, kept: "browser", acknowledged: false, status: "awaiting_referrer" });
  });

  it("lists this browser's returns with reason and note, and acknowledges only in this browser", () => {
    act(() => root.render(createElement(SessionReferrerHandoffs, { referrer: "GP" })));
    expect(host.textContent).toContain("No returns in this browser yet");
    act(() => saveSessionReturn(kept));
    const text = () => host.textContent ?? "";
    expect(text()).toContain("Kept in this browser only");
    expect(text()).toContain("ReasonNeeds a clinic visit first");
    expect(text()).toContain(`Note from the unit${NOTE}`);
    expect(text()).toContain("AcknowledgedNot yet");
    const ack = [...host.querySelectorAll("button")].find((b) => b.textContent === "Acknowledge the returned referral")!;
    act(() => ack.click());
    expect(readSessionReturns()[kept.id].acknowledgedAt).toBeTruthy();
    expect(text()).toContain("AcknowledgedYes");
    expect(text()).toContain("Acknowledged in this browser");
    expect([...host.querySelectorAll("button")].some((b) => b.textContent === "Acknowledge the returned referral")).toBe(false);
    // Nothing else can be answered here: no disposition controls.
    expect(host.querySelector('input[type="radio"]')).toBeNull();
    act(() => removeSessionReturn(kept.id));
    expect(text()).toContain("No returns in this browser yet");
  });

  it("ignores a corrupt store", () => {
    sessionStorage.setItem("wasl.referrals.session-returns.v1", "{not json");
    expect(readSessionReturns()).toEqual({});
    sessionStorage.setItem("wasl.referrals.session-returns.v1", JSON.stringify({ a: { id: "b", reason: "wrong_unit", actBy: 1 } }));
    expect(readSessionReturns()).toEqual({});
  });
});
