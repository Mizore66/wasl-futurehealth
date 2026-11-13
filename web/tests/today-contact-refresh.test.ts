// @vitest-environment happy-dom
import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { TodayBoard } from "@/components/worklist/today-board";
import { workItem } from "@/components/worklist/model";
import { CaseList, CaseSummary } from "@/lib/api/schema";

const { recordAction } = vi.hoisted(() => ({ recordAction: vi.fn() }));
vi.mock("@/app/app/today/actions", () => ({ recordAction }));
vi.mock("@number-flow/react", () => ({ default: ({ value }: { value: number }) => String(value) }));
// Exercise state/fetch behavior without happy-dom's incomplete native animation cancellation.
vi.mock("motion/react", async () => {
  const { createElement } = await import("react");
  return { useReducedMotion: () => true, AnimatePresence: ({ children }: { children: unknown }) => children,
    motion: { div: (props: Record<string, unknown>) => {
      const dom = { ...props };
      delete dom.initial; delete dom.animate; delete dom.transition;
      return createElement("div", dom);
    } },
  };
});
const load = (file: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", file), "utf8"));
const base = CaseList.parse(load("ledger_case_list.json")).cases[0];
const NOW = Date.parse("2026-11-20T12:00:00Z");
const initial: CaseSummary = { ...base, owner: "someone", dropoff: "not_booked", consent: "granted", api_patient: undefined,
  contact_state: { status: "none", contact_blocked: false },
  api_overlay: { head: "page-head", actions: 0, owner: "", last_action_at: "", ledger_status: "ok", no_answer_streak: 0 },
};
const blocked = { ...initial, contact_state: { status: "opted_out" as const, contact_blocked: true } };
const response = (summary: CaseSummary, status = "ok") => ({ status, detail: "", applied: 1, entries: [],
  bookingPanel: { booking: { status: "not_generated" }, overlay: summary.api_overlay, case: summary },
});
const ok = (body: unknown) => ({ ok: true, json: async () => body });
const deferred = () => { let resolve!: (value: ReturnType<typeof ok>) => void; const promise = new Promise<ReturnType<typeof ok>>((r) => { resolve = r; }); return { promise, resolve }; };
let root: ReturnType<typeof createRoot> | null = null;
let host: HTMLDivElement;
afterEach(async () => { if (root) await act(async () => root!.unmount()); root = null; vi.unstubAllGlobals(); vi.clearAllMocks(); document.body.innerHTML = ""; });
const button = (text: string) => Array.from(host.querySelectorAll("button")).find((b) => b.textContent?.includes(text))!;
const mount = async (items = [initial]) => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => root!.render(createElement(TodayBoard, { items: items.map((c) => workItem(c, 40, true, NOW)), now: 40, live: true, aside: null })));
};
const click = async (text: string) => act(async () => button(text).click());

it("applies refreshed opt-out immediately, keeping contact history as history", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(ok({ ...response(blocked), entries: [{ what: "Earlier permitted contact", when: "Saved", who: "Coordinator", hash: "earlier" }] })));
  await mount();
  expect(button("Log call").disabled).toBe(true);
  expect(host.textContent).not.toContain("Allowed");
  expect(host.textContent).not.toContain("After the call");
  expect(host.textContent).toContain("Contact blocked");
  expect(host.textContent).toContain("Earlier permitted contact");
});

for (const phase of ["live", "restarted"]) {
  it(`${phase}: a refreshed staff regrant restores UI despite historical patient denial`, async () => {
    const granted = CaseSummary.parse({ ...base, ...load(`contact_staff_regrant_${phase}.json`), review: base.review, owner: "someone", dropoff: "not_booked" });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(ok(response(granted))));
    await mount([{ ...granted, contact_state: { status: "denied", contact_blocked: true } }]);
    expect(button("Log call").disabled).toBe(false);
    expect(host.textContent).toContain("Allowed");
    expect(host.textContent).toContain("After the call");
    expect(host.textContent).not.toContain("Stopped by patient");
  });
}

it("applies the refreshed head with permission, never the stale page-load head", async () => {
  const fresh = { ...initial, api_overlay: { ...initial.api_overlay!, head: "refreshed-head" } };
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(ok(response(fresh))));
  recordAction.mockResolvedValue({ ok: false, status: 409, conflict: "head", error: "stale" });
  await mount(); await click("Reached");
  expect(recordAction).toHaveBeenCalledWith(initial.id, "refreshed-head", expect.objectContaining({ action: "contact_outcome" }));
});

it("failed or malformed reads cannot turn blocked permission into a successful grant or disclose trail entries", async () => {
  for (const answer of [
    { ok: false, status: 403, json: async () => ({ error: "read refused" }) },
    ok({ ...response(initial), bookingPanel: { ...response(initial).bookingPanel, case: { ...initial, contact_state: { status: "granted", contact_blocked: "false" } } }, entries: [{ what: "MALFORMED-CONCEALED", when: "Saved", who: "Hidden" }] }),
    ok(response({ ...initial, id: "wrong-case" })),
    ok({ ...response(initial), bookingPanel: { ...response(initial).bookingPanel, overlay: { ...initial.api_overlay, head: "mismatched-head" } } }),
  ]) {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(answer));
    await mount([blocked]);
    expect(button("Log call").disabled).toBe(true);
    expect(host.textContent).not.toContain("Allowed");
    expect(host.textContent).not.toContain("MALFORMED-CONCEALED");
    expect(host.textContent).toContain("Could not read the case ledger");
    await act(async () => root!.unmount()); root = null; host.remove();
  }
});

it("an invalid prefix cannot authorize regrant", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(ok(response(initial, "invalid"))));
  await mount([blocked]);
  expect(button("Log call").disabled).toBe(true);
  expect(host.textContent).not.toContain("Allowed");
  expect(host.textContent).toContain("not trustworthy");
});

it("aborted selection reads cannot change another case, reveal stale history, or override a newer retry", async () => {
  const first = deferred(); const second = deferred(); const third = deferred();
  const other = { ...initial, id: "ahs/ServiceRequest/OTHER", referral_day: initial.referral_day + 1 };
  const fetchMock = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise).mockReturnValueOnce(third.promise);
  vi.stubGlobal("fetch", fetchMock); await mount([initial, other]);
  const selectOther = Array.from(host.querySelectorAll<HTMLButtonElement>('button[aria-pressed]')).find((b) => b.textContent?.includes("OTHER"))!;
  await act(async () => selectOther.click());
  expect(fetchMock.mock.calls[0][1].signal.aborted).toBe(true);
  await act(async () => second.resolve(ok(response({ ...other, contact_state: blocked.contact_state }))));
  expect(button("Log call").disabled).toBe(true);
  await act(async () => first.resolve(ok({ ...response(initial), entries: [{ what: "STALE-CONCEALED", when: "Saved", who: "Hidden" }] })));
  expect(button("Log call").disabled).toBe(true);
  expect(host.textContent).not.toContain("STALE-CONCEALED");
  const firstRow = Array.from(host.querySelectorAll<HTMLButtonElement>('button[aria-pressed]')).find((b) => b.textContent?.includes(workItem(initial, 40, true, NOW).name))!;
  await act(async () => firstRow.click());
  await act(async () => third.resolve(ok(response(blocked))));
  expect(button("Log call").disabled).toBe(true);
});

it("a read begun before a successful local write cannot roll its head or contact permission backward", async () => {
  const stale = deferred(); const latest = deferred();
  vi.stubGlobal("fetch", vi.fn().mockImplementation(() => new Promise(() => {})).mockReturnValueOnce(stale.promise).mockReturnValueOnce(latest.promise));
  recordAction.mockResolvedValue({ ok: true, head: "written-head", hash: "write-hash", actor: "Coordinator" });
  await mount(); await click("Reached");
  await act(async () => stale.resolve(ok(response(blocked))));
  expect(button("Log call").disabled).toBe(false);
  await act(async () => latest.resolve(ok(response({ ...initial, api_overlay: { ...initial.api_overlay!, head: "written-head" } }))));
  await click("Snooze to tomorrow");
  expect(recordAction.mock.calls[1][1]).toBe("written-head");
});
