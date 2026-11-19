// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CaseSummary } from "@/lib/api/schema";
import { referralFrom, type Referral } from "@/components/worklist/referrals";
import synthetic from "@/data/referral-inbox-synthetic-v2.json";
import { mountInbox, type Harness } from "./referral-inbox-harness";

vi.mock("next/navigation", async () => (await import("./referral-nav-mock")).navigationMock);

const NOW = 120;
const ALL: Referral[] = synthetic.map((c) => referralFrom(CaseSummary.parse(c), NOW, false)).filter((r): r is Referral => r !== null);
const id = (n: string) => ALL.find((r) => r.id.endsWith(`AHS-sr-${n}`))!.id;
const only = (n: string) => ALL.filter((r) => r.id.endsWith(`AHS-sr-${n}`));

let h: Harness;
const render = (referrals: Referral[], q: string) => h.render({ referrals, window: 2, now: NOW, q });
const checked = () => [...h.host.querySelectorAll<HTMLInputElement>('input[type="radio"]:checked')].map((i) => i.value);
const pressed = () => [...(h.pane()?.querySelectorAll('button[aria-pressed="true"]') ?? [])].map((b) => b.textContent);

beforeEach(() => {
  h = mountInbox();
});
afterEach(() => h.unmount());

describe("referral inbox: an unconfirmed draft belongs to the referral it was started on", () => {
  it("drops a return reason when a search shows another referral, in one mounted inbox", () => {
    render(ALL, "");
    expect(h.openId()).toBe(id("9003"));
    const title = h.host.querySelector("h1")!;
    h.click("Return with reason");
    h.radio("Wrong unit");
    expect(checked()).toEqual(["wrong_unit"]);

    h.search("9002", only("9002"));
    expect(h.openId()).toBe(id("9002"));
    expect(h.paneText()).not.toContain("Reason for return");
    expect(h.button("Accept referral")).toBeTruthy();
    h.click("Return with reason");
    expect(checked()).toEqual([]);

    h.search("", ALL);
    expect(checked()).toEqual([]);
    expect(h.host.querySelector("h1")).toBe(title);
  });

  it("drops conditions across a zero-result intermediary to a different referral", () => {
    render(ALL, "");
    h.click("Accept with conditions");
    h.click("Female endoscopist");
    expect(pressed()).toEqual(["Female endoscopist"]);

    h.search("no-such-case", []);
    expect(h.text()).toContain("Nothing to open");
    h.search("9002", only("9002"));
    expect(h.openId()).toBe(id("9002"));
    expect(h.paneText()).not.toContain("Accept on these conditions");
    h.click("Accept with conditions");
    expect(h.button("Accept with no conditions")).toBeTruthy();
    expect(pressed()).toEqual([]);
  });

  it("clearing a search drops the draft but keeps a committed decision and its Undo", async () => {
    render(ALL, "");
    const title = h.host.querySelector("h1")!;
    h.open(id("9001"));
    h.click("Accept referral");
    h.open(id("9002"));
    h.click("Return with reason");
    h.radio("Wrong unit");

    h.search("9003", only("9003"));
    h.search("", ALL);
    expect(h.paneText()).not.toContain("Reason for return");
    expect(h.host.querySelector("h1")).toBe(title);

    h.open(id("9001"));
    expect(h.pane()!.querySelector('[role="status"]')!.textContent).toContain("Accepted. Your unit books her now.");
    h.click("Undo");
    await h.settled(() => !h.paneText().includes("Accepted. Your unit books her now."));
    expect(h.button("Accept referral")).toBeTruthy();
  });

  it("keeps the draft when the same referral stays displayed through a search", () => {
    render(ALL, "");
    h.click("Return with reason");
    h.radio("Wrong unit");
    h.search("9003", only("9003"));
    expect(h.openId()).toBe(id("9003"));
    expect(h.paneText()).toContain("Reason for return");
    expect(checked()).toEqual(["wrong_unit"]);
  });
});

describe("referral inbox: a return needs a reason", () => {
  it("blocks Return to GP with no reason, says why and sends nothing; a reason returns it and reverses the hand-off", () => {
    render(ALL, "");
    const arrow = () => h.host.querySelector('[data-testid="handoff-arrow"]')!.getAttribute("data-direction");
    expect(arrow()).toBe("forward");
    h.click("Return with reason");
    h.click("Return to GP");
    const alert = h.pane()!.querySelector('[role="alert"]')!;
    expect(alert.textContent).toBe("Choose a reason before you return it. Nothing was sent.");
    expect(h.pane()!.querySelector("fieldset")!.getAttribute("aria-describedby")).toBe(alert.id);
    expect(h.paneText()).not.toContain("Returned to the");
    expect(arrow()).toBe("forward");

    h.radio("Needs a clinic visit first");
    expect(h.pane()!.querySelector('[role="alert"]')).toBeNull();
    h.click("Return to GP");
    expect(arrow()).toBe("back");
    expect(h.paneText()).toContain("Booking held");
    expect(h.paneText()).toContain("Reason: needs a clinic visit first.");
    expect(h.host.querySelector('section[aria-label="Al Mushrif GP, current"]')).not.toBeNull();
    expect(h.count("waiting")).toBe(2);
  });

  it("offers Accept only on the open referral, never on an inbox row", () => {
    render(ALL, "");
    h.go("/app/referrals?view=inbox");
    expect([...h.host.querySelectorAll("tbody button")]).toHaveLength(0);
    expect(h.rows()).toContain("AHS-sr-9001");
  });
});
