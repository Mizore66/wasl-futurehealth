// @vitest-environment happy-dom
import { createElement, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CaseSummary } from "@/lib/api/schema";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { referralFrom, type Referral } from "@/components/worklist/referrals";
import synthetic from "@/data/referral-inbox-synthetic.json";

// Only the router is replaced (the inbox needs an app-router context); the inbox and its state are real.
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: () => undefined }) }));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const NOW = 120;
const ALL: Referral[] = synthetic
  .map((c) => referralFrom(CaseSummary.parse(c), NOW, false))
  .filter((r): r is Referral => r !== null);
const only = (n: string) => ALL.filter((r) => r.id.endsWith(`AHS-sr-${n}`));

let host: HTMLDivElement;
let root: Root;

function render(referrals: Referral[], q: string) {
  act(() => {
    root.render(createElement(ReferralInbox, { referrals, window: 2, now: NOW, q, aside: null }));
  });
}
const pane = () => host.querySelector<HTMLElement>('section[aria-label="Open referral"]')!;
const button = (scope: ParentNode, name: string | RegExp, exact = true) => {
  const found = [...scope.querySelectorAll<HTMLButtonElement>("button")].filter((b) => {
    const label = (b.getAttribute("aria-label") ?? b.textContent ?? "").trim();
    return typeof name === "string" ? (exact ? label === name : label.includes(name)) : name.test(label);
  });
  expect(found, `button ${String(name)}`).toHaveLength(1);
  return found[0];
};
const click = (el: HTMLElement) => act(() => el.click());
const heading = () => pane().querySelector("h2")!.textContent;
const hasText = (scope: ParentNode, text: string) => (scope.textContent ?? "").includes(text);
/** Condition-based wait inside act: lets AnimatePresence finish an exit (a real, short transition) without a fixed sleep. */
async function settled(cond: () => boolean, ms = 2000) {
  const end = Date.now() + ms;
  while (!cond() && Date.now() < end) await act(async () => void (await new Promise((r) => setTimeout(r, 20))));
  expect(cond()).toBe(true);
}
const pressed = () => [...pane().querySelectorAll('button[aria-pressed="true"]')].map((b) => b.textContent);

beforeEach(() => {
  // happy-dom cancels Web Animations on unmount with an unhandled AbortError; motion falls back to its JS driver without them.
  Object.defineProperty(Element.prototype, "animate", { value: undefined, configurable: true, writable: true });
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
});

describe("referral inbox: an unconfirmed draft belongs to the referral it was started on", () => {
  it("drops a return reason when a search shows another referral, in one mounted inbox", () => {
    render(ALL, "");
    expect(heading()).toBe("AHS-sr-9003");
    const inbox = host.querySelector("h1")!;
    click(button(pane(), "Return to GP"));
    click(button(pane(), "Wrong unit"));
    expect(pressed()).toEqual(["Wrong unit"]);

    render(only("9002"), "9002");
    expect(heading()).toBe("AHS-sr-9002");
    expect(hasText(pane(), "Why return this referral?")).toBe(false);
    expect(button(pane(), "Accept referral")).toBeTruthy();
    click(button(pane(), "Return to GP"));
    expect(hasText(pane(), "Choose a reason to return it.")).toBe(true);
    expect(pressed()).toEqual([]);

    render(ALL, "");
    expect(pressed()).toEqual([]);
    expect(host.querySelector("h1")).toBe(inbox);
  });

  it("drops conditions across a zero-result intermediary to a different referral", () => {
    render(ALL, "");
    click(button(pane(), "Accept with conditions"));
    click(button(pane(), "Female endoscopist"));
    expect(pressed()).toEqual(["Female endoscopist"]);

    render([], "no-such-case");
    expect(hasText(host, "Nothing to open")).toBe(true);
    render(only("9002"), "9002");
    expect(heading()).toBe("AHS-sr-9002");
    expect(hasText(pane(), "Accept on these conditions")).toBe(false);
    click(button(pane(), "Accept with conditions"));
    expect(button(pane(), "Accept with no conditions")).toBeTruthy();
    expect(pressed()).toEqual([]);
  });

  it("clearing a search drops the draft but keeps a committed decision and its Undo", async () => {
    render(ALL, "");
    const inbox = host.querySelector("h1")!;
    click(button(host, "Accept referral for AHS-sr-9001"));
    click(button(host, /^AHS-sr-9002/, false));
    click(button(pane(), "Return to GP"));
    click(button(pane(), "Wrong unit"));

    render(only("9003"), "9003");
    render(ALL, "");
    expect(hasText(pane(), "Why return this referral?")).toBe(false);
    expect(host.querySelector("h1")).toBe(inbox);

    click(button(host, /^All \d+$/));
    click(button(host, /^AHS-sr-9001/, false));
    expect(heading()).toBe("AHS-sr-9001");
    expect(pane().querySelector('[role="status"]')!.textContent).toContain("Accepted. Your unit books her now.");
    click(button(pane(), "Undo"));
    await settled(() => !hasText(pane(), "Accepted. Your unit books her now."));
    expect(button(pane(), "Accept referral")).toBeTruthy();
  });

  it("keeps the draft when the same referral stays displayed through a search", () => {
    render(ALL, "");
    click(button(pane(), "Return to GP"));
    click(button(pane(), "Wrong unit"));
    render(only("9003"), "9003");
    expect(heading()).toBe("AHS-sr-9003");
    expect(hasText(pane(), "Why return this referral?")).toBe(true);
    expect(pressed()).toEqual(["Wrong unit"]);
  });
});
