// @vitest-environment happy-dom
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { allNewReferrals, syntheticReferrals } from "./paging-fixtures";
vi.mock("motion/react", async () => {
  const { createElement } = await import("react");
  return { useReducedMotion: () => true, AnimatePresence: ({ children }: { children: unknown }) => children,
    motion: { div: (props: Record<string, unknown>) => {
      const dom = { ...props }; for (const k of ["initial", "animate", "exit", "transition"]) delete dom[k];
      return createElement("div", dom);
    } },
  };
});
let root: ReturnType<typeof createRoot> | null = null; let host: HTMLDivElement;
afterEach(async () => { if (root) await act(async () => root!.unmount()); root = null; vi.unstubAllGlobals(); vi.restoreAllMocks(); document.body.innerHTML = ""; });
async function mount(referrals = allNewReferrals(1201)) {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
  await act(async () => root!.render(createElement(ReferralInbox, { referrals, window: 7, actBy: "Day 125", q: "", aside: null })));
}
const list = () => host.querySelector<HTMLElement>('section[aria-label="Referral list"]')!;
const open = () => host.querySelector<HTMLElement>('section[aria-label="Open referral"]')!;
const names = () => Array.from(list().querySelectorAll('[role="tabpanel"] > div > button')).map((r) => r.querySelector(".font-semibold")!.textContent!);
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

it("all-new 1201 traversal reaches all rows exactly once in stable waiting order with complete counts and focus", async () => {
  const referrals = allNewReferrals(1201); await mount(referrals);
  expect(names()).toHaveLength(25); expect(list().textContent).toContain("Needs you 1201All 1201");
  expect(list().textContent).toContain("1201 referrals · 1201 need you");
  expect(open().textContent).toContain("All referrals · 1201");
  const seen = [...names()];
  for (let page = 2; page <= 49; page++) {
    await pager("Next"); expect(names()).toHaveLength(page === 49 ? 1 : 25);
    seen.push(...names()); expect(names()).toContain(heading());
    expect(document.activeElement?.getAttribute("data-focus")).toBe(page === 49 ? "pager-prev" : "pager-next");
  }
  expect(seen).toEqual([...referrals].sort((a, b) => b.waiting - a.waiting).map((r) => r.name));
  expect(new Set(seen).size).toBe(1201);
  await pager("Previous"); expect(status()).toContain("Page 48 of 49");
  await pager("First"); expect(status()).toContain("Page 1 of 49"); expect(document.activeElement?.getAttribute("data-focus")).toBe("pager-next");
  await pager("Last"); expect(names()).toEqual([referrals[1200].name]);
  expect(list().querySelectorAll('[role="status"]')).toHaveLength(1);
  expect(open().querySelector('[data-testid="page-status"]')).toBeNull();
});

it("accept/return remain open off-tab with identity-bound Undo, while paging resets mode/picked and preserves committed state", async () => {
  const referrals = syntheticReferrals(1201); await mount(referrals);
  expect(list().textContent).toContain("All 1201");
  const need = referrals.filter((r) => r.state === "new" || r.state === "grade").length;
  expect(list().textContent).toContain(`Needs you ${need}`);
  await pager("Next"); const acted = heading();
  await click("Accept with conditions", open()); await click("Arabic interpreter on the day", open());
  await click("Accept with 1 condition", open());
  expect(heading()).toBe(acted); expect(names()).not.toContain(acted);
  expect(status()).toContain("Page 2"); expect(open().textContent).toContain("Undo");
  expect(list().textContent).toContain(`${need - 1} need you`);
  await click("Undo", open()); expect(heading()).toBe(acted); expect(names()).toContain(acted); expect(status()).toContain("Page 2");
  await click("Return to GP", open()); await click("Wrong unit", open()); await click("Return to GP", open());
  expect(heading()).toBe(acted); expect(names()).not.toContain(acted); expect(open().textContent).toContain("Undo");
  await click("Undo", open()); expect(names()).toContain(acted);
  await click("Accept referral", open()); await pager("Next");
  expect(heading()).not.toBe(acted); expect(open().textContent).not.toContain("Accepted with conditions");
  await click("Accept with conditions", open()); await click("Female endoscopist", open());
  await pager("Previous"); expect(open().querySelector('[role="group"][aria-label="Conditions"]')).toBeNull();
  await pager("Next"); await click("Accept with conditions", open());
  expect(open().querySelectorAll('[aria-label="Conditions"] button[aria-pressed="true"]')).toHaveLength(0);
  await click("Accepted " + (referrals.filter((r) => r.state === "accepted" || r.state === "done").length + 1));
  expect(status()).toContain("Page 1");
  // The accepted identity's rank/wait position, not a page-one assumption, determines its page.
  for (let i = 0; !names().includes(acted) && i < 48; i++) await pager("Next");
  const acceptedPage = status()!.match(/^Page \d+/)![0];
  const actedRow = Array.from(list().querySelectorAll<HTMLButtonElement>('[role="tabpanel"] > div > button')).find((b) => b.textContent?.includes(acted))!;
  expect(actedRow).toBeDefined(); await act(async () => actedRow.click());
  expect(heading()).toBe(acted); expect(open().textContent).toContain("Accepted. Your unit books her now.");
  await click("Undo", open()); expect(heading()).toBe(acted); expect(names()).not.toContain(acted);
  // Undo in the Accepted tab also keeps the now-new referral open off-tab.
  expect(status()).toContain(acceptedPage);
  await click(`Needs you ${need}`); expect(status()).toContain("Page 1"); expect(names()[0]).toBe(heading());
});

it("final-page acceptance clamps without replacing the legitimate off-tab open referral, and Undo follows it back", async () => {
  const referrals = allNewReferrals(1201); await mount(referrals); await pager("Last");
  await click("Accept referral", open()); expect(names()).toHaveLength(25);
  expect(status()).toBe("Page 48 of 48, showing 1176 to 1200 of 1200.");
  expect(heading()).toBe(referrals[1200].name); expect(names()).not.toContain(heading());
  await click("Undo", open()); expect(status()).toContain("Page 49 of 49"); expect(names()).toContain(heading());
});

it("phone list paging never opens detail or calls matchMedia; tab change resets page", async () => {
  const media = vi.fn().mockReturnValue({ matches: false }); vi.stubGlobal("matchMedia", media);
  await mount(); await click("All referrals · 1201"); media.mockClear(); await pager("Next");
  expect(list().classList.contains("flex")).toBe(true); expect(list().classList.contains("hidden")).toBe(false);
  expect(open().classList.contains("hidden")).toBe(true); expect(media).not.toHaveBeenCalled();
  await click("All 1201"); expect(status()).toContain("Page 1");
});

it("mixed tabs retain full rank/wait order, badges, and clear uncommitted picks/mode on page and tab changes", async () => {
  const referrals = syntheticReferrals(1201); await mount(referrals);
  const needs = referrals.filter((x) => x.state === "new" || x.state === "grade").length;
  const accepted = referrals.filter((x) => x.state === "accepted" || x.state === "done").length;
  expect(list().textContent).toContain(`Needs you ${needs}`); expect(list().textContent).toContain(`Accepted ${accepted}`);
  await pager("Next"); await click("All 1201"); expect(status()).toContain("Page 1 of 49");
  const rank = { new: 0, grade: 1, returned: 2, accepted: 3, done: 4 };
  const ordered = [...referrals].sort((a, b) => rank[a.state] - rank[b.state] || b.waiting - a.waiting);
  const seen = [...names()];
  for (let page = 2; page <= 49; page++) { await pager("Next"); seen.push(...names()); }
  expect(seen).toEqual(ordered.map((r) => r.name));
  await click(`Needs you ${needs}`); expect(status()).toContain("Page 1");
  await click("Accept with conditions", open()); await click("Arabic interpreter on the day", open());
  expect(button("Arabic interpreter on the day").getAttribute("aria-pressed")).toBe("true");
  await pager("Next"); await pager("First"); await click("Accept with conditions", open());
  expect(button("Arabic interpreter on the day").getAttribute("aria-pressed")).toBe("false");
  await click(`Accepted ${accepted}`); expect(status()).toContain("Page 1"); expect(open().textContent).not.toContain("Accept on these conditions");
});
