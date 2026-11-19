// @vitest-environment happy-dom
import { readFileSync } from "node:fs";
import path from "node:path";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { TodayBoard } from "@/components/worklist/today-board";
import { todayRecords } from "@/components/worklist/today-records";
import { CaseDetail } from "@/lib/api/schema";
import { allCallWorkItems } from "./paging-fixtures";

/* Today's Activity | Records tabs: one lazy read of the selected case, in fixtures mode its own route, live the ledger read. */
vi.mock("@/app/app/today/actions", () => ({ recordAction: vi.fn() }));
vi.mock("@/components/shell/today-counts", async (load) => ({
  ...(await load<typeof import("@/components/shell/today-counts")>()),
  publishTodayCounts: () => {},
}));
vi.mock("@number-flow/react", () => ({ default: ({ value }: { value: number }) => String(value) }));
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

const trap = CaseDetail.parse(JSON.parse(readFileSync(path.join(__dirname, "..", "data", "api", "case_false_closure_trap.json"), "utf8")));
const records = JSON.parse(JSON.stringify(todayRecords(trap)));
const ok = (body: unknown) => ({ ok: true, status: 200, json: async () => body });
const failed = (status: number, body: unknown) => ({ ok: false, status, json: async () => body });

let root: ReturnType<typeof createRoot> | null = null;
let host: HTMLDivElement;
afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  root = null; vi.unstubAllGlobals(); vi.restoreAllMocks(); document.body.innerHTML = "";
});
const items = allCallWorkItems(3);
async function mount(live = false) {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => root!.render(createElement(TodayBoard, { items, now: 120, aside: null, live, undoMs: 0 })));
}
const history = () => host.querySelector<HTMLElement>('section[aria-label="Case history"]')!;
const tab = (name: string) => Array.from(history().querySelectorAll<HTMLButtonElement>("[role=tab]")).find((t) => t.textContent!.startsWith(name))!;
const panel = (name: "activity" | "records") => history().querySelector<HTMLElement>(`#today-panel-${name}`)!;
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 0)); });

it("fixtures mode: reads the selected case's records once, shows the held count, the held reason and source events", async () => {
  const fetchMock = vi.fn().mockResolvedValue(ok(records));
  vi.stubGlobal("fetch", fetchMock);
  await mount();
  await flush();
  expect(fetchMock).toHaveBeenCalledTimes(1);
  expect(fetchMock.mock.calls[0][0]).toBe(`/app/today/records?id=${encodeURIComponent(items[0].id)}`);
  expect(tab("Records").textContent).toBe("Records1 held");
  expect(tab("Activity").getAttribute("aria-selected")).toBe("true");
  expect(panel("activity").textContent).toContain("Report issued");
  expect(panel("records").hidden).toBe(true);
  await act(async () => tab("Records").click());
  expect(tab("Records").getAttribute("aria-selected")).toBe("true");
  expect(panel("records").hidden).toBe(false);
  const held = panel("records").querySelector("[data-held]")!;
  expect(held.textContent).toContain("Report, colonoscopy");
  expect(held.textContent).toContain("The report does not show the exam was complete, so the case stays open.");
  // Another case reads its own; coming back reuses the first read.
  await act(async () => host.querySelector<HTMLButtonElement>(`button[data-case-row="${items[1].short}"]`)!.click());
  await flush();
  await act(async () => host.querySelector<HTMLButtonElement>(`button[data-case-row="${items[0].short}"]`)!.click());
  await flush();
  expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([items[0].id, items[1].id].map((id) => `/app/today/records?id=${encodeURIComponent(id)}`));
});

it("fixtures mode: no held count without a hold, a missing record says so, a failure says why and Retry reads again", async () => {
  const fetchMock = vi.fn()
    .mockResolvedValueOnce(failed(502, { error: "the API did not answer" }))
    .mockResolvedValueOnce(ok({ sources: [], caseHeld: [], events: [] }));
  vi.stubGlobal("fetch", fetchMock);
  await mount();
  await flush();
  expect(history().querySelector('[data-testid="records-held"]')).toBeNull();
  expect(panel("activity").textContent).toContain("Source events not shown: the API did not answer");
  const retry = Array.from(panel("activity").querySelectorAll("button")).find((b) => b.textContent === "Retry")!;
  await act(async () => retry.click());
  await flush();
  expect(fetchMock).toHaveBeenCalledTimes(2);
  await act(async () => tab("Records").click());
  expect(panel("records").textContent).toBe("No source records on this case yet.");

  fetchMock.mockResolvedValueOnce(failed(404, { error: "this demo has source records only for the showcase cases", missing: true }));
  await act(async () => host.querySelector<HTMLButtonElement>(`button[data-case-row="${items[1].short}"]`)!.click());
  await flush();
  expect(panel("records").textContent).toBe("No source records to show: this demo has source records only for the showcase cases.");
  expect(panel("activity").textContent).not.toContain("Source events not shown");
});

it("live: the selected case's one ledger read carries its records; no separate records read", async () => {
  const fetchMock = vi.fn().mockResolvedValue(ok({ status: "ok", detail: "", applied: 0, entries: [], records }));
  vi.stubGlobal("fetch", fetchMock);
  await mount(true);
  await flush();
  expect(fetchMock.mock.calls.map((c) => c[0])).toEqual([`/app/today/ledger?id=${encodeURIComponent(items[0].id)}`]);
  expect(tab("Records").textContent).toBe("Records1 held");
  expect(panel("activity").textContent).toContain("Report issued");
});
