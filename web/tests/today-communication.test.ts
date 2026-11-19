// @vitest-environment happy-dom
/** SYNTHETIC UNIT DATA ONLY (issues #197 and #114): render/state tests, not API captures or clinical evidence. */
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CommunicationForm } from "@/components/worklist/communication-form";
import { persistedLedger } from "@/components/worklist/ledger-trail";
import { DROPOFF_WORDS, workItem, type WorkItem } from "@/components/worklist/model";
import { TodayBoard } from "@/components/worklist/today-board";
import { ActiveCaseSummary, CaseAction, CaseDetail, type ApiOverlay, type Dropoff, type Programme } from "@/lib/api/schema";
import detailJson from "./fixtures/ledger_case_detail.json";
import { syntheticCaseSummaries, WALL } from "./paging-fixtures";

const { recordAction } = vi.hoisted(() => ({ recordAction: vi.fn() }));
const { published } = vi.hoisted(() => ({ published: [] as unknown[] }));
vi.mock("@/components/shell/today-counts", async (load) => ({
  ...(await load<typeof import("@/components/shell/today-counts")>()),
  publishTodayCounts: (c: unknown) => published.push(c),
}));
vi.mock("@/app/app/today/actions", () => ({ recordAction }));
vi.mock("@number-flow/react", () => ({ default: ({ value }: { value: number }) => String(value) }));
vi.mock("motion/react", async () => {
  const { createElement } = await import("react");
  return {
    useReducedMotion: () => true, AnimatePresence: ({ children }: { children: unknown }) => children,
    motion: { div: (props: Record<string, unknown>) => {
      const dom = { ...props };
      for (const k of ["initial", "animate", "exit", "transition"]) delete dom[k];
      return createElement("div", dom);
    } },
  };
});

const TEMPLATE = "fitfu-notify-v2/ar";
const overlay = (ov: Partial<ApiOverlay> = {}): ApiOverlay => ({ head: "h0", actions: 0, owner: "", last_action_at: "", ledger_status: "ok", no_answer_streak: 0, ...ov });
function summary(dropoff: Dropoff, extra: Record<string, unknown> = {}, programme: Programme = "crc") {
  const [base] = syntheticCaseSummaries(1);
  return ActiveCaseSummary.parse({ ...base, programme, dropoff, api_overlay: overlay(), ...extra });
}
const item = (dropoff: Dropoff, extra: Record<string, unknown> = {}, programme: Programme = "crc") => workItem(summary(dropoff, extra, programme), 40, true, WALL);

describe("communication steps on Today (#197)", () => {
  it("a never-notified case owes a notification with the API's approved wording; receipt is worded as receipt", () => {
    const n = item("never_notified", { notification_template: TEMPLATE });
    expect(n).toMatchObject({ communicationStep: "notify", notificationTemplate: TEMPLATE, next: "Call to tell them the result" });
    const r = item("no_acknowledgement");
    expect(r.communicationStep).toBe("receipt");
    expect(r.notificationTemplate).toBeUndefined();
    expect(DROPOFF_WORDS.no_acknowledgement.next).toBe("Call to confirm they received the result");
    expect(DROPOFF_WORDS.no_acknowledgement.next).not.toMatch(/understood|understand/i);
  });

  it("a Reached outcome leaves the notification owed; only an explicit record changes it", () => {
    const reached = { at: "2026-11-19T09:00:00Z", event: "c".repeat(64), outcome: "reached", channel: "phone" };
    const it = item("never_notified", { notification_template: TEMPLATE, api_overlay: overlay({ last_contact: reached, actions: 1 }) });
    expect(it.communicationStep).toBe("notify");
    expect(it.status).toBe(`Reached · ${DROPOFF_WORDS.never_notified.next}`);
  });

  it("a recorded notification shows the receipt obligation the API served, and receipt never claims understanding", () => {
    const fact = { at: "2026-11-19T09:00:00Z", event: "n".repeat(64), occurred_at: "2026-11-19T08:55:00Z", channel: "phone" as const, template: TEMPLATE };
    const notified = item("no_acknowledgement", { canonical_dropoff: "never_notified", api_overlay: overlay({ communication: { canonical_dropoff: "never_notified", obligation: "no_acknowledgement", understanding: "none", notification: fact } }) });
    expect(notified).toMatchObject({ communicationStep: "receipt", status: "Result told · receipt not confirmed yet" });
    const received = item("referral_unanswered", { api_overlay: overlay({ communication: { canonical_dropoff: "never_notified", obligation: "referral_unanswered", understanding: "misunderstood", notification: fact, receipt: { ...fact, event: "r".repeat(64), notification_event: fact.event } } }) });
    expect(received.communicationStep).toBeUndefined();
    expect(received.status).toBe("Receipt confirmed · understanding not checked");
    expect(received.next).toBe(DROPOFF_WORDS.referral_unanswered.next);
  });

  it("the action schema accepts only staff-assisted channels and two-identifier checks", () => {
    const at = "2026-11-19T08:55:00Z";
    expect(CaseAction.safeParse({ action: "initial_notification", occurred_at: at, channel: "phone", template: TEMPLATE, identity_check: "name_and_dob" }).success).toBe(true);
    expect(CaseAction.safeParse({ action: "receipt_acknowledgement", occurred_at: at, channel: "in_person", identity_check: "name_and_mrn" }).success).toBe(true);
    expect(CaseAction.safeParse({ action: "initial_notification", occurred_at: at, channel: "sms", template: TEMPLATE, identity_check: "name_and_dob" }).success).toBe(false);
    expect(CaseAction.safeParse({ action: "receipt_acknowledgement", occurred_at: at, channel: "phone", identity_check: "name_only" }).success).toBe(false);
  });

  it("notification, receipt and understanding have distinct labels; no wording means no notification button", () => {
    const notify = renderToStaticMarkup(createElement(CommunicationForm, { step: "notify", template: TEMPLATE, disabled: false, onRecord: () => {} }));
    const receipt = renderToStaticMarkup(createElement(CommunicationForm, { step: "receipt", disabled: false, onRecord: () => {} }));
    expect(notify).toContain("Record result notification");
    expect(notify).toContain(TEMPLATE);
    expect(receipt).toContain("Record receipt acknowledged");
    expect(receipt).toContain("not a check that they understood");
    expect(receipt).not.toContain("Record result notification");
    const missing = renderToStaticMarkup(createElement(CommunicationForm, { step: "notify", disabled: false, onRecord: () => {} }));
    expect(missing).not.toContain("<button");
  });

  it("the case trail words each recorded fact apart", () => {
    const raw = structuredClone(detailJson) as Record<string, unknown> & { api_reviews: Record<string, unknown>[] };
    const tail = raw.api_reviews.at(-1)!;
    const ev = (kind: string, data: Record<string, string>, hash: string) => ({ ...tail, type: "coordinator_action", kind, data, hash, at: "2026-11-21T09:00:00Z", actor: "nurse-1" });
    raw.api_reviews = [...raw.api_reviews,
      ev("initial_notification", { occurred_at: "2026-11-21T08:50:00Z", channel: "phone", template: TEMPLATE, identity_check: "name_and_dob" }, "n".repeat(64)),
      ev("receipt_acknowledgement", { occurred_at: "2026-11-21T08:55:00Z", channel: "phone", identity_check: "name_and_dob", notification_event: "n".repeat(64) }, "r".repeat(64))];
    const words = persistedLedger(CaseDetail.parse(raw)).entries.map((e) => e.what);
    expect(words.some((w) => w.startsWith("Result notification recorded (phone)") && w.includes(TEMPLATE))).toBe(true);
    expect(words.some((w) => w.startsWith("Receipt acknowledged (phone)") && w.includes("understanding not checked"))).toBe(true);
  });
});

let root: ReturnType<typeof createRoot> | null = null;
let host: HTMLDivElement;
afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  root = null; vi.unstubAllGlobals(); vi.restoreAllMocks(); vi.resetAllMocks(); document.body.innerHTML = "";
});
async function mount(items: WorkItem[], live = true) {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  // The selected case's ledger read never answers: these tests are about the write path.
  vi.stubGlobal("fetch", vi.fn(() => new Promise(() => {})));
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => root!.render(createElement(TodayBoard, { items, now: 40, aside: null, live, undoMs: 0 })));
}
const open = () => host.querySelector<HTMLElement>('section[aria-label="Selected case"]')!;
const button = (text: string) => Array.from(host.querySelectorAll<HTMLButtonElement>("button")).find((b) => b.textContent?.trim() === text);
async function choose(label: string, value: string) {
  const select = Array.from(open().querySelectorAll("label")).find((l) => l.textContent?.startsWith(label))!.querySelector("select")!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!.call(select, value);
    select.dispatchEvent(new Event("change", { bubbles: true }));
  });
}
const saved = (head: string, obligation?: string) => ({ ok: true, head, hash: head.padEnd(64, "0"), at: "2026-11-20T12:00:00Z", actor: "coordinator-1", owner: "", ...(obligation ? { obligation } : {}) });

describe("Today board records notification then receipt (#197, live)", () => {
  it("posts each action with the served wording and identity check, then moves to the next obligation", async () => {
    await mount([item("never_notified", { notification_template: TEMPLATE, owner: "Practitioner/ahs-mushrif-nurse-1" })]);
    expect(button("Record result notification")?.disabled).toBe(true); // no identity check chosen yet
    await choose("Identity confirmed with", "name_and_dob");
    recordAction.mockResolvedValueOnce(saved("h1", "no_acknowledgement"));
    await act(async () => button("Record result notification")!.click());
    const [, head, action] = recordAction.mock.calls[0];
    expect(head).toBe("h0");
    expect(action).toMatchObject({ action: "initial_notification", channel: "phone", template: TEMPLATE, identity_check: "name_and_dob" });
    expect(host.textContent).toContain("Result told · receipt not confirmed yet");
    expect(button("Record result notification")).toBeUndefined();

    await choose("Identity confirmed with", "name_and_emirates_id");
    recordAction.mockResolvedValueOnce(saved("h2", "referral_unanswered"));
    await act(async () => button("Record receipt acknowledged")!.click());
    expect(recordAction.mock.calls[1][1]).toBe("h1");
    expect(recordAction.mock.calls[1][2]).toMatchObject({ action: "receipt_acknowledgement", identity_check: "name_and_emirates_id" });
    expect(open().textContent).toContain(DROPOFF_WORDS.referral_unanswered.next);
    expect(host.textContent).toContain("Receipt confirmed · understanding not checked");
    expect(button("Record receipt acknowledged")).toBeUndefined();
    expect(open().textContent).not.toMatch(/understood the/i);
  });

  it("other actions on a notified case keep their own move even when the API echoes the obligation", async () => {
    const fact = { at: "2026-11-19T09:00:00Z", event: "n".repeat(64), occurred_at: "2026-11-19T08:55:00Z", channel: "phone" as const, template: TEMPLATE };
    const notified = item("no_acknowledgement", { canonical_dropoff: "never_notified", owner: "Practitioner/ahs-mushrif-nurse-1", api_overlay: overlay({ communication: { canonical_dropoff: "never_notified", obligation: "no_acknowledgement", understanding: "none", notification: fact } }) });
    await mount([notified]);
    expect(published.at(-1)).toEqual({ call: 1, waiting: 0, none: 0 });
    // The server action still carries the obligation for any action on a case with a recorded fact; the board must ignore it.
    recordAction.mockResolvedValueOnce({ ...saved("h1", "no_acknowledgement"), owner: "Dr Al Kaabi" });
    await act(async () => button("Hand to Dr Al Kaabi")!.click());
    expect(recordAction.mock.calls[0][2].action).toBe("assign_owner");
    expect(published.at(-1)).toEqual({ call: 0, waiting: 1, none: 0 });
  });

  it("a refused record says why and changes nothing", async () => {
    await mount([item("never_notified", { notification_template: TEMPLATE, owner: "Practitioner/ahs-mushrif-nurse-1" })]);
    await choose("Identity confirmed with", "name_and_dob");
    recordAction.mockResolvedValueOnce({ ok: false, status: 409, error: "the case has an unconfirmed second identifier; resolve identity before recording a communication", conflict: "identity" });
    await act(async () => button("Record result notification")!.click());
    expect(open().querySelector('[data-testid="action-refused"]')?.textContent).toContain("unconfirmed second identifier");
    expect(button("Record result notification")).toBeDefined();
  });
});

describe("Today queue for every programme (#114)", () => {
  const owner = { owner: "Practitioner/ahs-mushrif-nurse-1" };
  it("names each row's programme and provider only when the queue mixes programmes", async () => {
    await mount([item("not_booked", owner), item("technical_repeat_not_done", { ...owner, id: "corniche-wh/ServiceRequest/BR-1", provider: "corniche-wh" }, "breast")], false);
    const labels = Array.from(host.querySelectorAll('[data-testid="row-programme"]')).map((n) => n.textContent);
    expect(labels).toEqual(expect.arrayContaining(["Colorectal screening · Al Mushrif", "Breast screening · corniche-wh"]));
    await act(async () => root!.unmount()); root = null;
    await mount([item("not_booked", owner)], false);
    expect(host.querySelector('[data-testid="row-programme"]')).toBeNull();
  });

  it("persists a contact outcome, owner change and snooze on a non-CRC case through the same ledger action", async () => {
    const breast = item("technical_repeat_not_done", { ...owner, id: "corniche-wh/ServiceRequest/BR-2", provider: "corniche-wh" }, "breast");
    expect(breast.next).toBe("Arrange the repeat mammogram");
    await mount([breast]);
    expect(open().textContent).toContain("Breast screening");
    recordAction.mockResolvedValueOnce(saved("h1"));
    await act(async () => button("Log call · " + breast.name.split(" ")[0])?.click() ?? button("Log call")?.click());
    await act(async () => button("No answer")!.click());
    recordAction.mockResolvedValueOnce(saved("h2"));
    await act(async () => button("Snooze to tomorrow")!.click());
    recordAction.mockResolvedValueOnce({ ...saved("h3"), owner: "Dr Al Kaabi" });
    await act(async () => button("Hand to Dr Al Kaabi")!.click());
    const sent = recordAction.mock.calls.map(([id, head, action]) => [id, head, action.action]);
    expect(sent).toEqual([
      ["corniche-wh/ServiceRequest/BR-2", "h0", "contact_outcome"],
      ["corniche-wh/ServiceRequest/BR-2", "h1", "snooze"],
      ["corniche-wh/ServiceRequest/BR-2", "h2", "assign_owner"],
    ]);
  });
});
