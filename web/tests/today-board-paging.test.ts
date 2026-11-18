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
async function mount(items = allCallWorkItems(1201), live = false) {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => root!.render(createElement(TodayBoard, { items, now: 120, aside: null, live, undoMs: 0 })));
}
const list = () => host.querySelector<HTMLElement>('section[aria-label="Your list"]')!;
const open = () => host.querySelector<HTMLElement>('section[aria-label="Open patient"]')!;
const rows = () => Array.from(list().querySelectorAll<HTMLButtonElement>('[role="tabpanel"] > div > button'));
const names = () => rows().map((r) => r.querySelector(".font-semibold")!.textContent!);
const heading = () => open().querySelector("h2")!.textContent!.trim();
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
  expect(list().textContent).toContain("To call 1201");
  expect(list().textContent).toContain("1201 to call");
  expect(open().textContent).toContain("Your list · 1201");
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
});

it("mixed 401/400/400 tab badges and resets use full data, never the 1201 traversal claim", async () => {
  await mount(syntheticWorkItems(1201));
  expect(list().textContent).toContain("To call 401Waiting 400No owner 400");
  await pager("Last"); expect(status()).toContain("Page 17 of 17");
  await click("Waiting 400"); expect(status()).toBe("Page 1 of 16, showing 1 to 25 of 400."); assertMounted();
  await pager("Next"); await click("No owner 400"); expect(status()).toContain("Page 1 of 16"); assertMounted();
  await click("To call 401"); expect(status()).toContain("Page 1 of 17");
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
  const next = Array.from(open().querySelectorAll("button")).find((b) => b.textContent?.startsWith("Next patient:"))!;
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
  const next = Array.from(open().querySelectorAll("button")).find((b) => b.textContent?.startsWith("Next patient:"))!;
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
  await search("not-a-synthetic-case"); expect(rows()).toHaveLength(0); expect(list().textContent).toContain("Nobody left");
  expect(list().querySelector("nav")).toBeNull(); expect(status()).toBeUndefined();
});

it("does not transfer drafts or Undo between identities, while committed history survives page/search/tab round trips", async () => {
  const items = allCallWorkItems(1201); await mount(items);
  await click("Log call · AHS-sr-90000");
  await pager("Next"); expect(open().textContent).not.toContain("Logging call");
  await pager("First"); expect(open().textContent).not.toContain("Logging call");
  await click("Reached"); expect(open().textContent).toContain("What next?");
  await pager("Next"); expect(open().textContent).not.toContain("What next?"); expect(open().textContent).not.toContain("Undo");
  await pager("First"); expect(open().textContent).toContain("Reached on the phone");
  // Committed awaitingNext belongs to A, not a leaked in-progress draft. Appointment removes A from To call.
  await click("Appointment agreed"); expect(heading()).toBe(items[0].name); expect(names()).not.toContain(items[0].name);
  expect(open().textContent).toContain("moved to Waiting");
  await pager("Next"); expect(open().textContent).not.toContain("Undo"); expect(host.querySelector('[data-testid="removed-notice"]')).toBeNull();
  await click("Waiting 1"); expect(heading()).toBe(items[0].name); expect(open().textContent).toContain("Appointment agreed");
  await click("To call 1200"); await search(items[1].short); expect(heading()).toBe(items[1].name); expect(open().textContent).not.toContain("Appointment agreed");
  await search(""); await click("Waiting 1"); expect(open().textContent).toContain("Reached on the phone");
});

it("phone list pane stays the list after paging and adds no matchMedia calls", async () => {
  const media = vi.fn().mockReturnValue({ matches: false }); vi.stubGlobal("matchMedia", media);
  await mount(); await click("Your list · 1201"); media.mockClear();
  await pager("Next"); expect(list().classList.contains("flex")).toBe(true); expect(list().classList.contains("hidden")).toBe(false);
  expect(open().classList.contains("hidden")).toBe(true); expect(media).not.toHaveBeenCalled();
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
  expect(open().textContent).toContain("Contact blocked");
  await pager("First"); expect(fetchMock).toHaveBeenCalledTimes(3);
  await act(async () => first.resolve(ok({ status: "ok", detail: "", applied: 0, entries: [{ what: "ABORTED-CONCEALED", when: "Saved", who: "Synthetic" }] })));
  expect(host.textContent).not.toContain("ABORTED-CONCEALED");
  await pager("Next"); expect(fetchMock).toHaveBeenCalledTimes(4);
  expect(open().textContent).toContain("Contact blocked"); assertMounted();
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
  { count: 1201, action: "Snooze to tomorrow", kind: "snooze", page: "Page 49 of 49", selected: 0 },
  { count: 26, action: "Hand to Dr Al Kaabi", kind: "assign_owner", page: undefined, selected: 0 },
])("phone delayed $kind follows the action but focuses the visible row, including a disappearing pager ($count rows)", async ({ count, action, kind, page, selected }) => {
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: false }));
  // SYNTHETIC per-row heads added after workItem, not production authority or API captures.
  const items = allCallWorkItems(count).map((item, i) => ({ ...item, head: `synthetic-phone-head-${i}` }));
  const saved = pending<{ ok: true; head: string; hash: string; actor: string }>();
  recordAction.mockReturnValue(saved.promise);
  vi.stubGlobal("fetch", vi.fn().mockReturnValue(new Promise(() => {})));
  await mount(items, true); await click(action);
  expect(recordAction).toHaveBeenCalledExactlyOnceWith(items[0].id, items[0].head, expect.objectContaining({ action: kind }));
  await click(`Your list · ${count}`); await pager("Next");
  const focusedPager = button(count === 26 ? "Previous" : "Next", list().querySelector("nav")!);
  expect(document.activeElement).toBe(focusedPager);
  expect(heading()).toBe(items[25].name);
  await act(async () => saved.resolve({ ok: true, head: "synthetic-phone-saved-head", hash: "synthetic-phone-write-hash", actor: "Synthetic coordinator" }));
  expect(heading()).toBe(items[selected].name);
  // A hand-off moves the patient to Waiting but keeps them open (N1); a snooze keeps them in To call.
  if (kind === "snooze") assertMounted(); else expect(names()).not.toContain(heading());
  if (page) expect(status()).toBe(`${page}, showing 1201 to 1201 of 1201.`);
  else { expect(status()).toBeUndefined(); expect(list().querySelector("nav")).toBeNull(); }
  expect(focusedPager.isConnected).toBe(false);
  // happy-dom has no responsive CSS: assert production pane classes and intentional list focus,
  // not browser display:none behavior, layout, or axe evidence.
  expect(list().classList.contains("hidden")).toBe(false);
  expect(list().classList.contains("flex")).toBe(true);
  expect(open().classList.contains("hidden")).toBe(true);
  expect(document.activeElement).toBe(
    kind === "snooze" ? rows().find((row) => row.getAttribute("aria-pressed") === "true") : list().querySelector('[role="tab"][aria-selected="true"]'),
  );
  expect(list().contains(document.activeElement)).toBe(true);
  expect(open().contains(document.activeElement)).toBe(false);
  expect(document.activeElement).not.toBe(document.body);
  expect(open().textContent).toContain("saved to the case ledger");
  expect(host.querySelector('[data-testid="removed-notice"]')).toBeNull();
  if (kind !== "snooze") expect(open().textContent).toContain("moved to Waiting");
});

it("phone delayed hand-off of the only row focuses the surviving selected list tab", async () => {
  vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({ matches: false }));
  const items = allCallWorkItems(1).map((item) => ({ ...item, head: "synthetic-empty-head" }));
  const saved = pending<{ ok: true; head: string; hash: string; actor: string }>();
  recordAction.mockReturnValue(saved.promise);
  vi.stubGlobal("fetch", vi.fn().mockReturnValue(new Promise(() => {})));
  await mount(items, true); await click("Hand to Dr Al Kaabi");
  expect(recordAction).toHaveBeenCalledExactlyOnceWith(items[0].id, items[0].head, expect.objectContaining({ action: "assign_owner" }));
  await click("Your list · 1"); expect(document.activeElement).toBe(rows()[0]);
  await act(async () => saved.resolve({ ok: true, head: "synthetic-empty-saved-head", hash: "synthetic-empty-write-hash", actor: "Synthetic coordinator" }));
  expect(rows()).toHaveLength(0); expect(list().querySelector("nav")).toBeNull();
  expect(list().classList.contains("hidden")).toBe(false); expect(open().classList.contains("hidden")).toBe(true);
  expect(document.activeElement).toBe(list().querySelector('[role="tab"][aria-selected="true"]'));
  expect(document.activeElement).not.toBe(document.body);
  expect(heading()).toBe(items[0].name); expect(open().textContent).toContain("moved to Waiting");
});

it.each(["phone detail", "desktop list after resize"])("delayed action retains its normal detail focus on %s", async (pane) => {
  const media = vi.fn().mockReturnValue({ matches: false }); vi.stubGlobal("matchMedia", media);
  const items = allCallWorkItems(26).map((item, i) => ({ ...item, head: `synthetic-focus-head-${i}` }));
  const saved = pending<{ ok: true; head: string; hash: string; actor: string }>();
  recordAction.mockReturnValue(saved.promise);
  vi.stubGlobal("fetch", vi.fn().mockReturnValue(new Promise(() => {})));
  await mount(items, true); await click("Snooze to tomorrow");
  expect(recordAction).toHaveBeenCalledExactlyOnceWith(items[0].id, items[0].head, expect.objectContaining({ action: "snooze" }));
  if (pane === "desktop list after resize") {
    await click("Your list · 26"); await pager("Next");
    // The viewport at completion, not at action start, decides whether detail is hidden.
    media.mockReturnValue({ matches: true });
  } else await pager("Next");
  await act(async () => saved.resolve({ ok: true, head: "synthetic-focus-saved-head", hash: "synthetic-focus-write-hash", actor: "Synthetic coordinator" }));
  expect(heading()).toBe(items[0].name); expect(status()).toContain("Page 2 of 2"); assertMounted();
  expect(document.activeElement).toBe(open().querySelector('[data-focus="undo"]'));
  expect(open().classList.contains("hidden")).toBe(pane === "desktop list after resize");
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
