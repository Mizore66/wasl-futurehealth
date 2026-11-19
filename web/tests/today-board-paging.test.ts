// @vitest-environment happy-dom
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { TodayBoard } from "@/components/worklist/today-board";
import { byUrgency } from "@/components/worklist/days";
import type { WorkItem } from "@/components/worklist/model";
import { allCallWorkItems, syntheticCaseSummaries, syntheticWorkItems } from "./paging-fixtures";

const { recordAction } = vi.hoisted(() => ({ recordAction: vi.fn() }));
vi.mock("@/app/app/today/actions", () => ({ recordAction }));
const { published } = vi.hoisted(() => ({ published: [] as unknown[] }));
vi.mock("@/components/shell/today-counts", async (load) => ({
  ...(await load<typeof import("@/components/shell/today-counts")>()),
  publishTodayCounts: (c: unknown) => published.push(c),
}));
vi.mock("@number-flow/react", () => ({ default: ({ value }: { value: number }) => String(value) }));
// State/DOM tests, not browser animation evidence. Leave the actual controls and focus effects intact.
vi.mock("motion/react", async () => {
  const { createElement } = await import("react");
  return { useReducedMotion: () => true, AnimatePresence: ({ children }: { children: unknown }) => children,
    motion: { div: (props: Record<string, unknown>) => {
      const dom = { ...props };
      for (const k of ["initial", "animate", "exit", "transition"]) delete dom[k];
      return createElement("div", dom);
    } },
  };
});
let root: ReturnType<typeof createRoot> | null = null;
let host: HTMLDivElement;
afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  root = null; vi.unstubAllGlobals(); vi.restoreAllMocks(); vi.resetAllMocks(); document.body.innerHTML = "";
});
let props: Parameters<typeof TodayBoard>[0];
async function mount(items = allCallWorkItems(1201), live = false) {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  published.length = 0;
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  props = { items, now: 120, aside: null, live, undoMs: 0 };
  await act(async () => root!.render(createElement(TodayBoard, props)));
}
/** The sidebar's queue links change `?queue=`; here the server-rendered prop stands in for the URL. */
async function queue(q: "call" | "waiting" | "none") {
  props = { ...props, queue: q };
  await act(async () => root!.render(createElement(TodayBoard, props)));
}
const counts = () => published.at(-1);
const list = () => host.querySelector<HTMLElement>('[data-testid="today-sheet"]')!;
const open = () => host.querySelector<HTMLElement>('section[aria-label="Selected case"]')!;
const rows = () => Array.from(list().querySelectorAll<HTMLButtonElement>("button[data-case-row]"));
const names = () => rows().map((r) => r.dataset.caseRow!);
const heading = () => open().dataset.case!;
/** The selected case with its Activity panel, which sits below it in its own section. */
const caseText = () => `${open().textContent}${host.querySelector('section[aria-label="Case history"]')?.textContent ?? ""}`;
const status = () => list().querySelector('[data-testid="page-status"]')?.textContent;
function button(text: string, scope: ParentNode = host) {
  const found = Array.from(scope.querySelectorAll<HTMLButtonElement>("button")).find((b) => {
    const label = b.cloneNode(true) as HTMLElement;
    label.querySelectorAll("[aria-hidden]").forEach((node) => node.remove());
    return label.textContent?.trim() === text;
  });
  expect(found, `button ${text}`).toBeDefined(); return found!;
}
const click = async (text: string, scope: ParentNode = host) => act(async () => button(text, scope).click());
const pager = (text: string) => click(text, list().querySelector("nav")!);
const assertMounted = () => expect(names()).toContain(heading());
async function search(value: string) {
  const input = list().querySelector<HTMLInputElement>('input[aria-label="Search"], input[type="search"]')!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
const pending = <T,>() => { let resolve!: (value: T) => void; const promise = new Promise<T>((r) => { resolve = r; }); return { promise, resolve }; };
const ok = (body: unknown) => ({ ok: true, json: async () => body });

it("mounts at most 25 of an explicitly all-Call 1201 while retaining complete counts", async () => {
  await mount();
  expect(rows()).toHaveLength(25);
  expect(counts()).toEqual({ call: 1201, waiting: 0, none: 0 });
  expect(list().textContent).toContain("1201 to call");
  expect(status()).toBe("Page 1 of 49, showing 1 to 25 of 1201.");
  assertMounted();
});

it("traverses all 1201 mounted rows exactly once in urgency/stable-tie order with 48 Next activations and end focus", async () => {
  const items = allCallWorkItems(1201); await mount(items);
  const seen = [...names()];
  for (let page = 2; page <= 49; page++) {
    await pager("Next");
    expect(rows()).toHaveLength(page === 49 ? 1 : 25);
    seen.push(...names()); assertMounted();
    expect(document.activeElement?.getAttribute("data-focus")).toBe(page === 49 ? "pager-prev" : "pager-next");
  }
  expect(seen).toEqual([...items].sort(byUrgency).map((x) => x.name));
  expect(new Set(seen).size).toBe(1201);
  await pager("Previous"); expect(status()).toContain("Page 48 of 49");
  await pager("First"); expect(names()[0]).toBe(items[0].name);
  expect(document.activeElement?.getAttribute("data-focus")).toBe("pager-next");
  await pager("Next"); await pager("Previous");
  expect(document.activeElement?.getAttribute("data-focus")).toBe("pager-next");
  await pager("Last"); expect(names()).toEqual([items[1200].name]);
  expect(document.activeElement?.getAttribute("data-focus")).toBe("pager-prev");
  expect(list().querySelectorAll('[role="status"]')).toHaveLength(1);
  expect(open().querySelector('[data-testid="page-status"]')).toBeNull();
}, 30_000); // 48 page renders of a 1201-row board; the 5 s default timed out on a loaded CI runner at 5.05 s.

it("mixed 401/400/400 tab badges and resets use full data, never the 1201 traversal claim", async () => {
  await mount(syntheticWorkItems(1201));
  expect(counts()).toEqual({ call: 401, waiting: 400, none: 400 });
  await pager("Last"); expect(status()).toContain("Page 17 of 17");
  await queue("waiting"); expect(status()).toBe("Page 1 of 16, showing 1 to 25 of 400."); assertMounted();
  await pager("Next"); await queue("none"); expect(status()).toContain("Page 1 of 16"); assertMounted();
  await queue("call"); expect(status()).toContain("Page 1 of 17");
});

it("follows a snoozed row to its new last page and Undo back, with no false removed notice; Next patient crosses pages", async () => {
  const items = allCallWorkItems(1201); await mount(items);
  const original = heading();
  await click("Snooze to tomorrow");
  expect(heading()).toBe(original); expect(names()).toEqual([original]);
  expect(status()).toContain("Page 49 of 49"); assertMounted();
  expect(host.querySelector('[data-testid="removed-notice"]')).toBeNull();
  await click("Undo", open()); expect(status()).toContain("Page 1 of 49"); assertMounted();
  await click("Snooze to tomorrow");
  const next = Array.from(open().querySelectorAll("button")).find((b) => b.textContent?.startsWith("Next case:"))!;
  await act(async () => next.click());
  expect(status()).toContain("Page 1 of 49"); expect(heading()).toBe(items[1].name); assertMounted();
  expect(open().textContent).not.toContain("Logged: Snoozed");
});

it("final-page hand-off clamps to 48, keeps the acted patient open with Undo and a link to Waiting, and restores the acted identity", async () => {
  const items = allCallWorkItems(1201); await mount(items); await pager("Last");
  await click("Hand to Dr Al Kaabi");
  expect(rows()).toHaveLength(25); expect(status()).toBe("Page 48 of 48, showing 1176 to 1200 of 1200.");
  // N1: the moved patient stays open until the user moves on; the list itself no longer holds them.
  expect(heading()).toBe(items[1200].name); expect(names()).not.toContain(items[1200].name);
  expect(open().textContent).toContain("moved to Waiting"); expect(button("Open Waiting", open())).toBeTruthy();
  expect(host.querySelector('[data-testid="removed-notice"]')).toBeNull();
  await click("Undo", open()); expect(heading()).toBe(items[1200].name); expect(status()).toContain("Page 49 of 49"); assertMounted();
  await click("Hand to Dr Al Kaabi");
  const next = Array.from(open().querySelectorAll("button")).find((b) => b.textContent?.startsWith("Next case:"))!;
  await act(async () => next.click());
  expect(heading()).toBe(items[1175].name); assertMounted();
});

it("search retains the selected matching row, otherwise resets to one; empty results remove pager/status", async () => {
  const items = allCallWorkItems(1201); await mount(items); await pager("Last");
  await search(items[1200].short); expect(heading()).toBe(items[1200].name); expect(rows()).toHaveLength(1); assertMounted();
  await search("90017"); expect(names()).toEqual([items[17].name]); assertMounted();
  // Search retains sel, not the temporary fallback row: clearing restores the still-selected last identity.
  await search(""); expect(status()).toContain("Page 49 of 49"); expect(heading()).toBe(items[1200].name); assertMounted();
  // Explicitly selecting the narrowed match updates sel; clearing now follows that id on page one.
  await search("90017"); await act(async () => rows()[0].click());
  await search(""); expect(status()).toContain("Page 1 of 49"); expect(heading()).toBe(items[17].name); assertMounted();
  await search("not-a-synthetic-case"); expect(rows()).toHaveLength(0); expect(list().textContent).toContain("No case matches");
  // An unmatched search leaves no case actions to press.
  expect(open()).toBeNull(); expect(host.textContent).not.toContain("Log call");
  expect(list().querySelector("nav")).toBeNull(); expect(status()).toBeUndefined();
});

it("does not transfer drafts or Undo between identities, while committed history survives page/search/tab round trips", async () => {
  const items = allCallWorkItems(1201); await mount(items);
  await click("Log call · AHS-sr-90000");
  await pager("Next"); expect(open().textContent).not.toContain("Logging call");
  await pager("First"); expect(open().textContent).not.toContain("Logging call");
  await click("Reached"); expect(open().textContent).toContain("What next?");
  await pager("Next"); expect(open().textContent).not.toContain("What next?"); expect(open().textContent).not.toContain("Undo");
  await pager("First"); expect(caseText()).toContain("Reached on the phone");
  // Committed awaitingNext belongs to A, not a leaked in-progress draft. Appointment removes A from To call.
  await click("Appointment agreed"); expect(heading()).toBe(items[0].name); expect(names()).not.toContain(items[0].name);
  expect(open().textContent).toContain("moved to Waiting");
  await pager("Next"); expect(open().textContent).not.toContain("Undo"); expect(host.querySelector('[data-testid="removed-notice"]')).toBeNull();
  expect(counts()).toEqual({ call: 1200, waiting: 1, none: 0 });
  await queue("waiting"); expect(heading()).toBe(items[0].name); expect(caseText()).toContain("Appointment agreed");
  await queue("call"); await search(items[1].short); expect(heading()).toBe(items[1].name); expect(open().textContent).not.toContain("Appointment agreed");
  await search(""); await queue("waiting"); expect(caseText()).toContain("Reached on the phone");
});

it("phone: the open row folds and unfolds its inline case; paging adds no matchMedia calls", async () => {
  const media = vi.fn().mockReturnValue({ matches: false }); vi.stubGlobal("matchMedia", media);
  await mount();
  const first = rows()[0];
  expect(first.getAttribute("aria-expanded")).toBe("true");
  expect(first.getAttribute("aria-controls")).toBe(open().id);
  await act(async () => first.click());
  expect(first.getAttribute("aria-expanded")).toBe("false"); expect(open().className).toContain("max-lg:hidden");
  await act(async () => first.click());
  expect(first.getAttribute("aria-expanded")).toBe("true"); expect(open().className).not.toContain("max-lg:hidden");
  media.mockClear();
  await pager("Next"); expect(media).not.toHaveBeenCalled(); assertMounted();
});

it("live page changes read exactly the mounted open id, abort the previous request and preserve refreshed contact projection", async () => {
  const items = allCallWorkItems(1201);
  const first = pending<ReturnType<typeof ok>>(); const second = pending<ReturnType<typeof ok>>();
  const fetchMock = vi.fn().mockReturnValue(new Promise(() => {})).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  vi.stubGlobal("fetch", fetchMock); await mount(items, true);
  expect(fetchMock).toHaveBeenCalledTimes(1);
  expect(fetchMock.mock.calls[0][0]).toBe(`/app/today/ledger?id=${encodeURIComponent(items[0].id)}`);
  await pager("Next"); expect(fetchMock).toHaveBeenCalledTimes(2);
  expect(fetchMock.mock.calls[0][1].signal.aborted).toBe(true);
  expect(fetchMock.mock.calls[1][0]).toBe(`/app/today/ledger?id=${encodeURIComponent(items[25].id)}`);
  const summary = { ...syntheticCaseSummaries(26)[25], contact_state: { status: "opted_out", contact_blocked: true },
    api_overlay: { head: "synthetic-refresh-head-25", actions: 0, owner: "", last_action_at: "", ledger_status: "ok", no_answer_streak: 0 } };
  await act(async () => second.resolve(ok({ status: "ok", detail: "", applied: 0, entries: [], bookingPanel: { booking: { status: "not_generated" }, overlay: summary.api_overlay, case: summary } })));
  expect(open().textContent).toContain("Contact on hold");
  await pager("First"); expect(fetchMock).toHaveBeenCalledTimes(3);
  await act(async () => first.resolve(ok({ status: "ok", detail: "", applied: 0, entries: [{ what: "ABORTED-CONCEALED", when: "Saved", who: "Synthetic" }] })));
  expect(host.textContent).not.toContain("ABORTED-CONCEALED");
  await pager("Next"); expect(fetchMock).toHaveBeenCalledTimes(4);
  expect(open().textContent).toContain("Contact on hold"); assertMounted();
});

it("T-TODAY-LIVE-INFLIGHT invokes recordAction before paging, then follows that acted row on genuine completion", async () => {
  const items: WorkItem[] = allCallWorkItems(1201);
  // F2: explicit SYNTHETIC per-test head, AFTER actual workItem; not a capture or production authority.
  items[0] = { ...items[0], head: "synthetic-head-0" };
  const saved = pending<{ ok: true; head: string; hash: string; actor: string }>();
  recordAction.mockReturnValue(saved.promise);
  vi.stubGlobal("fetch", vi.fn().mockReturnValue(new Promise(() => {})));
  await mount(items, true); await click("Snooze to tomorrow");
  expect(recordAction).toHaveBeenCalledExactlyOnceWith(items[0].id, "synthetic-head-0", expect.objectContaining({ action: "snooze" }));
  await pager("Next"); expect(heading()).toBe(items[25].name); expect(open().textContent).not.toContain("Logged: Snoozed");
  await act(async () => saved.resolve({ ok: true, head: "synthetic-head-0-after", hash: "synthetic-write-hash", actor: "Synthetic coordinator" }));
  expect(heading()).toBe(items[0].name); expect(status()).toContain("Page 49 of 49"); assertMounted();
  expect(open().textContent).toContain("saved to the case ledger"); expect(open().textContent).not.toContain("Undo");
  expect(host.querySelector('[data-testid="removed-notice"]')).toBeNull();
});

it.each([
  { count: 1201, action: "Snooze to tomorrow", kind: "snooze", page: "Page 49 of 49" },
  { count: 26, action: "Hand to Dr Al Kaabi", kind: "assign_owner", page: undefined },
])("a delayed $kind follows the action and focuses its outcome, including a disappearing pager ($count rows)", async ({ count, action, kind, page }) => {
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: false }));
  // SYNTHETIC per-row heads added after workItem, not production authority or API captures.
  const items = allCallWorkItems(count).map((item, i) => ({ ...item, head: `synthetic-phone-head-${i}` }));
  const saved = pending<{ ok: true; head: string; hash: string; actor: string }>();
  recordAction.mockReturnValue(saved.promise);
  vi.stubGlobal("fetch", vi.fn().mockReturnValue(new Promise(() => {})));
  await mount(items, true); await click(action);
  expect(recordAction).toHaveBeenCalledExactlyOnceWith(items[0].id, items[0].head, expect.objectContaining({ action: kind }));
  await pager("Next");
  const focusedPager = button(count === 26 ? "Previous" : "Next", list().querySelector("nav")!);
  expect(document.activeElement).toBe(focusedPager);
  expect(heading()).toBe(items[25].name);
  await act(async () => saved.resolve({ ok: true, head: "synthetic-phone-saved-head", hash: "synthetic-phone-write-hash", actor: "Synthetic coordinator" }));
  expect(heading()).toBe(items[0].name);
  // A hand-off moves the patient to Waiting but keeps them open (N1); a snooze keeps them in To call.
  if (kind === "snooze") assertMounted(); else expect(names()).not.toContain(heading());
  if (page) expect(status()).toBe(`${page}, showing 1201 to 1201 of 1201.`);
  else { expect(status()).toBeUndefined(); expect(list().querySelector("nav")).toBeNull(); }
  expect(focusedPager.isConnected).toBe(false);
  expect(document.activeElement).toBe(open().querySelector('[data-focus="undo"]'));
  expect(open().textContent).toContain("saved to the case ledger");
  expect(host.querySelector('[data-testid="removed-notice"]')).toBeNull();
  if (kind !== "snooze") expect(open().textContent).toContain("moved to Waiting");
});

it("a delayed hand-off of the only row keeps the case open with its outcome focused", async () => {
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: false }));
  const items = allCallWorkItems(1).map((item) => ({ ...item, head: "synthetic-empty-head" }));
  const saved = pending<{ ok: true; head: string; hash: string; actor: string }>();
  recordAction.mockReturnValue(saved.promise);
  vi.stubGlobal("fetch", vi.fn().mockReturnValue(new Promise(() => {})));
  await mount(items, true); await click("Hand to Dr Al Kaabi");
  expect(recordAction).toHaveBeenCalledExactlyOnceWith(items[0].id, items[0].head, expect.objectContaining({ action: "assign_owner" }));
  await act(async () => saved.resolve({ ok: true, head: "synthetic-empty-saved-head", hash: "synthetic-empty-write-hash", actor: "Synthetic coordinator" }));
  expect(rows()).toHaveLength(0); expect(list().querySelector("nav")).toBeNull();
  expect(document.activeElement).toBe(open().querySelector('[data-focus="undo"]'));
  expect(heading()).toBe(items[0].name); expect(open().textContent).toContain("moved to Waiting");
});

it("a delayed action retains its detail focus after paging away", async () => {
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: true }));
  const items = allCallWorkItems(26).map((item, i) => ({ ...item, head: `synthetic-focus-head-${i}` }));
  const saved = pending<{ ok: true; head: string; hash: string; actor: string }>();
  recordAction.mockReturnValue(saved.promise);
  vi.stubGlobal("fetch", vi.fn().mockReturnValue(new Promise(() => {})));
  await mount(items, true); await click("Snooze to tomorrow");
  expect(recordAction).toHaveBeenCalledExactlyOnceWith(items[0].id, items[0].head, expect.objectContaining({ action: "snooze" }));
  await pager("Next");
  await act(async () => saved.resolve({ ok: true, head: "synthetic-focus-saved-head", hash: "synthetic-focus-write-hash", actor: "Synthetic coordinator" }));
  expect(heading()).toBe(items[0].name); expect(status()).toContain("Page 2 of 2"); assertMounted();
  expect(document.activeElement).toBe(open().querySelector('[data-focus="undo"]'));
});

it("publishes the board's live counts for the sidebar and clears them on unmount", async () => {
  await mount(syntheticWorkItems(9));
  expect(counts()).toEqual({ call: 3, waiting: 3, none: 3 });
  await click("Hand to Dr Al Kaabi");
  expect(counts()).toEqual({ call: 2, waiting: 4, none: 3 });
  await act(async () => root!.unmount()); root = null;
  expect(counts()).toBeNull();
});

it("unmounted synthetic invalid row still warns for the whole board", async () => {
  const items = allCallWorkItems(1201);
  // F2: explicit SYNTHETIC status only on the unmounted last row, AFTER actual workItem.
  items[1200] = { ...items[1200], ledgerStatus: "invalid" };
  vi.stubGlobal("fetch", vi.fn().mockReturnValue(new Promise(() => {})));
  await mount(items, true); expect(rows()).toHaveLength(25);
  expect(names()).not.toContain(items[1200].name);
  expect(host.querySelector('[role="alert"]')?.textContent).toContain("not trustworthy");
  expect(button("Snooze to tomorrow").disabled).toBe(true);
});
