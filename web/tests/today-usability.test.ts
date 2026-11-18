// @vitest-environment happy-dom
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LIVE_UNDO_MS, TodayBoard } from "@/components/worklist/today-board";
import { workItem, type WorkItem } from "@/components/worklist/model";
import { focusWorklist, SkipLink } from "@/components/shell/skip-link";
import { CaseList, type TeachBackCallback } from "@/lib/api/schema";
import { allCallWorkItems } from "./paging-fixtures";
import { loadSample } from "./samples";

/* Usability audit §3 (docs/USABILITY-AUDIT.md): N1 closed loop, N5 delayed commit, N6 callback link, N7 skip link. */

const { recordAction, pathname } = vi.hoisted(() => ({ recordAction: vi.fn(), pathname: { current: "/app/today" } }));
vi.mock("@/app/app/today/actions", () => ({ recordAction }));
vi.mock("next/navigation", () => ({ usePathname: () => pathname.current }));
vi.mock("@number-flow/react", () => ({ default: ({ value }: { value: number }) => String(value) }));
vi.mock("next/link", async () => {
  const { createElement } = await import("react");
  return { default: (props: Record<string, unknown>) => createElement("a", { href: String(props.href), className: props.className }, props.children as never) };
});
vi.mock("motion/react", async () => {
  const { createElement } = await import("react");
  return {
    useReducedMotion: () => true,
    AnimatePresence: ({ children }: { children: unknown }) => children,
    motion: {
      div: (props: Record<string, unknown>) => {
        const dom = { ...props };
        for (const k of ["initial", "animate", "exit", "transition"]) delete dom[k];
        return createElement("div", dom);
      },
    },
  };
});

let root: Root | null = null;
let host: HTMLDivElement;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, json: async () => ({ status: "ok", detail: "", applied: 0, entries: [] }) }));
  recordAction.mockResolvedValue({ ok: true, head: "synthetic-saved-head", hash: "synthetic-saved-hash-0000", actor: "Synthetic coordinator" });
  pathname.current = "/app/today";
});
afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  root = null;
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.resetAllMocks();
  document.body.innerHTML = "";
});

async function mount(items: WorkItem[], live = false) {
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  await act(async () => root!.render(createElement(TodayBoard, { items, now: 120, aside: null, live })));
}
const list = () => host.querySelector<HTMLElement>('section[aria-label="Your list"]')!;
const open = () => host.querySelector<HTMLElement>('section[aria-label="Open patient"]')!;
const heading = () => open().querySelector("h2")!.textContent!.trim();
const names = () => Array.from(list().querySelectorAll('[role="tabpanel"] > div > button .font-semibold')).map((n) => n.textContent);
function button(text: string, scope: ParentNode = host) {
  const found = Array.from(scope.querySelectorAll<HTMLButtonElement>("button")).find((b) => b.textContent?.trim().startsWith(text));
  expect(found, `button ${text}`).toBeDefined();
  return found!;
}
const click = async (text: string, scope: ParentNode = host) => act(async () => button(text, scope).click());
const withHeads = (n: number) => allCallWorkItems(n).map((item, i) => ({ ...item, head: `synthetic-usability-head-${i}` }));

describe("N1: an outcome that moves the patient keeps them open", () => {
  it("Appointment agreed keeps the patient in the pane and the toast opens the Waiting tab", async () => {
    const items = allCallWorkItems(3);
    await mount(items);
    await click("Log call");
    await click("Reached");
    await click("Appointment agreed");
    expect(heading()).toBe(items[0].name);
    expect(names()).not.toContain(items[0].name);
    expect(open().textContent).toContain("moved to Waiting");
    expect(host.querySelector('[data-testid="removed-notice"]')).toBeNull();
    await click("Open Waiting", open());
    expect(list().querySelector('[role="tab"][aria-selected="true"]')!.textContent).toMatch(/^Waiting/);
    expect(heading()).toBe(items[0].name);
    expect(list().querySelector('button[aria-pressed="true"] .font-semibold')!.textContent).toBe(items[0].name);
    expect(host.querySelector('[data-testid="open-destination"]')).toBeNull();
  });

  it("moving on (another row) lets go of the held patient", async () => {
    const items = allCallWorkItems(3);
    await mount(items);
    await click("Hand to Dr Al Kaabi");
    expect(heading()).toBe(items[0].name);
    await act(async () => Array.from(list().querySelectorAll<HTMLButtonElement>('[role="tabpanel"] > div > button'))[1].click());
    expect(heading()).toBe(items[2].name);
    expect(host.querySelector('[data-testid="open-destination"]')).toBeNull();
  });
});

describe("N1 and issue #12: a search that excludes the held patient lets go of them", () => {
  it("leaves no patient actions, only the way back, and holds them again once the search is cleared", async () => {
    const items = allCallWorkItems(3);
    await mount(items);
    await click("Hand to Dr Al Kaabi");
    const search = host.querySelector<HTMLInputElement>('input[type="search"]')!;
    const type = (text: string) =>
      act(async () => {
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(search, text);
        search.dispatchEvent(new Event("input", { bubbles: true }));
      });
    await type("zzzz-no-match");
    expect(heading()).toBe("Nobody to open");
    // Only the phone-width "Your list" toggle and Undo are left: nothing that acts on the patient.
    expect(Array.from(open().querySelectorAll("button")).map((b) => b.textContent?.trim())).toEqual(["‹Your list", "Undo"]);
    expect(host.querySelector('[data-testid="removed-notice"]')!.textContent).toContain("moved to Waiting");
    await type("");
    expect(heading()).toBe(items[0].name);
    expect(host.querySelector('[data-testid="open-destination"]')).not.toBeNull();
  });
});

describe("N5: live Snooze, Hand to and Declined wait with Undo before the POST", () => {
  it("Undo inside the window sends nothing, ever", async () => {
    vi.useFakeTimers();
    const items = withHeads(2);
    await mount(items, true);
    await click("Hand to Dr Al Kaabi");
    const notice = host.querySelector<HTMLElement>('[data-testid="pending-write"]')!;
    expect(notice.textContent).toContain(`Handed to Dr Al Kaabi · ${items[0].name} · saves to the case ledger in 5 seconds`);
    expect(recordAction).not.toHaveBeenCalled();
    expect(button("Snooze to tomorrow").disabled).toBe(true);
    await act(async () => vi.advanceTimersByTime(LIVE_UNDO_MS - 1));
    expect(recordAction).not.toHaveBeenCalled();
    await click("Undo", notice);
    await act(async () => vi.advanceTimersByTime(LIVE_UNDO_MS * 3));
    expect(recordAction).not.toHaveBeenCalled();
    expect(host.querySelector('[data-testid="pending-write"]')).toBeNull();
    expect(button("Snooze to tomorrow").disabled).toBe(false);
    expect(heading()).toBe(items[0].name);
    expect(open().textContent).not.toContain("With Dr Al Kaabi");
  });

  it("the write is sent once when the window ends, bound to the row's head", async () => {
    vi.useFakeTimers();
    const items = withHeads(2);
    await mount(items, true);
    await click("Snooze to tomorrow");
    expect(recordAction).not.toHaveBeenCalled();
    await act(async () => vi.advanceTimersByTime(LIVE_UNDO_MS));
    expect(recordAction).toHaveBeenCalledExactlyOnceWith(items[0].id, items[0].head, expect.objectContaining({ action: "snooze" }));
    expect(host.querySelector('[data-testid="pending-write"]')).toBeNull();
    expect(open().textContent).toContain("saved to the case ledger");
  });

  it("Declined is held too; Save now sends at once and only once", async () => {
    vi.useFakeTimers();
    const items = withHeads(2);
    await mount(items, true);
    await click("Log call");
    await click("Reached");
    expect(recordAction).toHaveBeenCalledTimes(1);
    await click("Declined");
    expect(recordAction).toHaveBeenCalledTimes(1);
    await click("Save now", host.querySelector<HTMLElement>('[data-testid="pending-write"]')!);
    expect(recordAction).toHaveBeenCalledTimes(2);
    expect(recordAction.mock.calls[1][2]).toEqual({ action: "contact_outcome", outcome: "declined", channel: "phone" });
    await act(async () => vi.advanceTimersByTime(LIVE_UNDO_MS * 2));
    expect(recordAction).toHaveBeenCalledTimes(2);
  });

  it("leaving the page inside the window sends the held write instead of dropping it", async () => {
    vi.useFakeTimers();
    const items = withHeads(1);
    await mount(items, true);
    await click("Hand to Dr Al Kaabi");
    await act(async () => root!.unmount());
    root = null;
    expect(recordAction).toHaveBeenCalledExactlyOnceWith(items[0].id, items[0].head, { action: "assign_owner", owner: "Dr Al Kaabi" });
  });

  it("fixtures mode applies at once and keeps its Undo after the fact", async () => {
    const items = allCallWorkItems(2);
    await mount(items);
    await click("Hand to Dr Al Kaabi");
    expect(host.querySelector('[data-testid="pending-write"]')).toBeNull();
    expect(open().textContent).toContain("Handed to Dr Al Kaabi");
    expect(button("Undo", open())).toBeTruthy();
    expect(recordAction).not.toHaveBeenCalled();
  });
});

describe("N6 and the closed loop: a failed teach-back callback", () => {
  const open_: TeachBackCallback = {
    id: "a1b2c3d4e5f60718", source: "patient_link", state: "misunderstood", owner: "coordinator", status: "open",
    escalation: "teachback_misunderstood_callback", opened_at: "2026-10-01T08:00:00Z", due_at: "2026-10-05T08:00:00Z", overdue: false,
  };
  const row = CaseList.parse(loadSample("cases_crc_page1.json")).cases.find((c) => !c.closed && c.owner)!;
  const item = () => workItem({ ...row, teach_back_callback: open_ }, 120, true);

  it("carries the status and the patient's receipt reference on the work item", () => {
    // The due time uses the shared staff formatter (Dubai time plus a relative hint that depends on today).
    expect(item().teachBackCallback).toEqual({ status: expect.stringMatching(/^Teach-back failed · nurse callback due Mon 5 Oct, 12:00 Dubai time \(.+\)$/), ref: "a1b2c3d4" });
    expect(workItem({ ...row, teach_back_callback: { ...open_, status: "resolved" } }, 120, true).teachBackCallback).toBeUndefined();
  });

  it("links the Today pane straight to the case page's callback card", async () => {
    await mount([item()]);
    const box = open().querySelector('[data-testid="teach-back-callback"]')!;
    const link = box.querySelector("a")!;
    expect(link.getAttribute("href")).toBe(`/app/patients/${encodeURIComponent(row.id)}#teach-back-callbacks`);
    expect(link.textContent).toBe("record the consultation outcome on the case page");
    expect(box.textContent).toContain("patient ref a1b2c3d4");
  });

  it("keeps the callback status and patient ref on the row in every tab while the task is open", async () => {
    const it_ = item();
    await mount([it_]);
    expect(list().querySelector('[data-testid="row-callback"]')!.textContent).toBe("Patient ref a1b2c3d4");
    await click("Hand to Dr Al Kaabi");
    await click("Open Waiting", open());
    const line = list().querySelector('[data-testid="row-callback"]')!.textContent;
    expect(line).toMatch(/^Teach-back failed · nurse callback due Mon 5 Oct, 12:00 Dubai time \(.+\) · Patient ref a1b2c3d4$/);
  });
});

describe("N7: skip to worklist (WCAG 2.4.1)", () => {
  it("renders a second skip link on Today and Referrals only", async () => {
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
    await act(async () => root!.render(createElement(SkipLink)));
    expect(Array.from(host.querySelectorAll("a")).map((a) => [a.textContent, a.getAttribute("href")])).toEqual([
      ["Skip to main content", "#main"],
      ["Skip to worklist", "#worklist"],
    ]);
    pathname.current = "/app/patients";
    await act(async () => root!.render(createElement(SkipLink, { key: "other" })));
    expect(Array.from(host.querySelectorAll("a")).map((a) => a.textContent)).toEqual(["Skip to main content"]);
  });

  it("lands on the first worklist row in one step", async () => {
    const items = allCallWorkItems(3);
    await mount(items);
    expect(focusWorklist()).toBe(true);
    expect(document.activeElement).toBe(list().querySelector('[role="tabpanel"] button[aria-pressed]'));
    expect(document.activeElement!.textContent).toContain(items[0].name);
  });

  it("falls back to the list itself when the view is empty", async () => {
    await mount([]);
    expect(focusWorklist()).toBe(true);
    expect(document.activeElement).toBe(list());
  });
});
