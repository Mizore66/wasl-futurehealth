// @vitest-environment happy-dom
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CaseList, CaseSummary, CurrentDecision, Fixtures } from "@/lib/api/schema";
import { handoffOf, referralFrom, type Referral } from "@/components/worklist/referrals";
import { formatDay } from "@/lib/domain/day";
import synthetic from "@/data/referral-inbox-synthetic-v2.json";
import provenance from "@/data/referral-inbox-provenance-v2.json";
import { loadSample } from "./samples";
import { mountInbox, type Harness } from "./referral-inbox-harness";

vi.mock("next/navigation", async () => (await import("./referral-nav-mock")).navigationMock);
// Match fixtures-mode source acquisition: named fixtures first, then the CRC sample, deduped by case ID.
const baseCases = [...new Map([
  ...Object.values(Fixtures.parse(loadSample("fixtures.json")).fixtures),
  ...CaseList.parse(loadSample("cases_crc_page1.json")).cases,
].filter((c): c is CaseSummary => c !== null && c.programme === "crc" && c.provider === "ahs-mushrif").map((c) => [c.id, c])).values()];
const BASE = baseCases.map((c) => referralFrom(c, 120, true)).filter((r): r is Referral => r !== null);
const EXTRA = synthetic.map((c) => referralFrom(CaseSummary.parse(c), 120, false)!).filter(Boolean);
const ALL = [...BASE, ...EXTRA];
const positive = EXTRA.find((r) => r.id.endsWith("9001"))!;
const aisha = BASE.find((r) => r.id.endsWith("1011"))!;
const held = referralFrom(CaseSummary.parse(provenance.held_control), 120, false)!;
let h: Harness;
let now: number;
let media: ReturnType<typeof vi.fn>;

function render(referrals = ALL, q = "") {
  h.render({ referrals, window: 2, now, q });
}
const pane = () => h.pane()!;
const content = () => h.paneText();
const click = (name: string | RegExp, scope?: ParentNode) => h.click(name, scope);
const button = (name: string | RegExp, scope?: ParentNode) => h.button(name, scope);
/** The old saved views, read from the full inbox's status filter: Needs you = needs + report, Accepted = accepted + closed. */
const counts = (needs: number, accepted: number, total = ALL.length) => {
  expect(h.count("needs") + h.count("report")).toBe(needs);
  expect(h.count("accepted") + (h.count("closed") || 0)).toBe(accepted);
  expect(h.count()).toBe(total);
};
const open = (r: Referral) => h.open(r.id);
function grade(n: number) {
  const group = pane().querySelector('[aria-label="Boston grade"]')!;
  const b = [...group.querySelectorAll<HTMLButtonElement>("button")].find((b) => b.querySelector("span")!.textContent === String(n));
  expect(b).toBeDefined();
  act(() => b!.click());
}
async function undo() {
  click("Undo", pane());
  await h.settled(() => !pane().querySelector('[data-focus="undo"]'));
  expect(document.activeElement).toBe(pane().querySelector('[data-focus="heading"]'));
}
const search = (q: string, source = ALL) => h.search(q, source);

beforeEach(() => {
  media = vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() });
  vi.stubGlobal("matchMedia", media);
  h = mountInbox();
  now = 120;
});
afterEach(() => { h.unmount(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("#119 held prep is independent of genuine acceptance", () => {
  it.each([0, 1, 2, 3])("Aisha grade %i stays Needs you, survives search, regrades and undoes", async (n) => {
    render(); counts(4, 16);
    expect(h.text()).toContain("Decisions stay in this inbox session only");
    expect(h.text()).toContain("these are not medical-policy or DoH requirements");
    open(aisha); grade(n);
    expect(content()).toContain(`Graded ${n},`);
    expect(content()).toContain(`Graded the prep ${n}`);
    expect(content()).toContain("Complete the report");
    expect(content()).not.toContain("Book and do the procedure");
    expect(document.activeElement).toBe(button("Undo", pane()));
    counts(4, 16);
    search("1011"); expect(content()).toContain(`Graded ${n},`);
    search("no-such-case"); expect(h.text()).toContain("Nothing to open");
    search(""); open(aisha); counts(4, 16);
    expect(content()).toContain(`Graded the prep ${n}`);
    grade((n + 1) % 4);
    expect(content()).toContain(`Graded the prep ${n}`);
    expect(content()).toContain(`Graded the prep ${(n + 1) % 4}`);
    counts(4, 16);
    await undo(); counts(4, 16);
    expect(content()).toContain("Review the incomplete report");
    expect(content()).not.toContain("Graded the prep");
    expect(pane().querySelector('[aria-label="Boston grade"]')).not.toBeNull();
    grade(n); counts(4, 16);
  });

  it("uses genuine signed producer outputs for the positive/held contrast, not closed=false DTO edits", async () => {
    const current = CurrentDecision.parse(provenance.currents[positive.id as keyof typeof provenance.currents]);
    const heldCurrent = CurrentDecision.parse(provenance.currents[held.id as keyof typeof provenance.currents]);
    expect(current.sig).toMatch(/^[a-f0-9]{64}$/); expect(current.state).toBe("open");
    expect(heldCurrent.sig).toMatch(/^[a-f0-9]{64}$/); expect(heldCurrent.state).toBe("inadequate");
    expect(positive).toMatchObject({ state: "new", flags: [] });
    expect(held).toMatchObject({ state: "grade", flags: ["Report incomplete"] });
    render([positive, held]); counts(2, 0, 2);
    click("Accept referral", pane());
    expect(content()).toContain("Accepted the referral");
    counts(1, 1, 2); open(positive);
    await undo(); counts(2, 0, 2);
    open(held); grade(2); counts(2, 0, 2);
    expect(content()).toContain("Complete the report");
    expect(content()).not.toContain("Book and do the procedure");
    await undo(); counts(2, 0, 2);
  });
});

describe("#147 completed decisions remain case-bound through soft search props", () => {
  it.each(["accept", "conditions", "return"] as const)("retains %s outcome/history/counts/Undo across same/other/empty/clear searches", async (action) => {
    // The decisions live in the mounted inbox; a soft search (and every move between views) keeps it mounted.
    render(); open(positive);
    if (action === "accept") click("Accept referral", pane());
    if (action === "conditions") { click("Accept with conditions", pane()); click("Female endoscopist", pane()); click("Accept with 1 condition", pane()); }
    if (action === "return") { click("Return with reason", pane()); h.radio("Wrong unit"); click("Return to GP", pane()); }
    const outcome = pane().querySelector('[role="status"]')!.textContent;
    const history = action === "return" ? "Returned to the GP: wrong unit" : action === "conditions" ? "Accepted with 1 condition" : "Accepted the referral";
    expect(content()).toContain(history);
    counts(3, action === "return" ? 16 : 17);
    search("9001"); expect(content()).toContain(outcome); expect(content()).toContain(history);
    search("9002"); expect(content()).not.toContain(history);
    search("no-such-case"); search(""); open(positive);
    expect(content()).toContain(outcome); expect(content()).toContain(history);
    counts(3, action === "return" ? 16 : 17);
    await undo(); counts(4, 16);
    expect(button("Accept referral", pane())).toBeTruthy(); expect(content()).not.toContain(history);
  });

  it("restores all four hidden decisions and independently undoes them on desktop and phone", async () => {
    media.mockReturnValue({ matches: false });
    render(); open(positive); click("Accept referral", pane());
    const conditional = EXTRA.find((r) => r.id.endsWith("9002"))!;
    const returned = EXTRA.find((r) => r.id.endsWith("9003"))!;
    open(conditional); click("Accept with conditions", pane()); click("Arabic interpreter on the day", pane()); click("Accept with 1 condition", pane());
    open(returned); click("Return with reason", pane()); h.radio("Wrong unit"); click("Return to GP", pane());
    open(aisha); grade(3); counts(1, 18);
    search("no-such-case"); search(""); counts(1, 18);
    for (const r of [aisha, returned, conditional, positive]) {
      open(r); expect(button("Undo", pane())).toBeTruthy(); await undo();
      // Back in the full inbox, the referral just undone is the marked row and takes focus.
      h.go(`/app/referrals?view=inbox&case=${encodeURIComponent(r.id)}`);
      expect(h.host.querySelector('tbody a[aria-current="true"]')!.textContent).toBe(r.short);
    }
    counts(4, 16);
  });
});

describe("#140 per-case canonical packets and stored session deadlines", () => {
  it.each([
    ["awaiting_referrer", 121, "Act on the return", "by"],
    ["coordinator", 103, "Escalated to the care coordinator", "was due"],
    ["medical_director", 97, "Escalated to the medical director", "was due"],
    ["referrer_acted", 99, "The GP has acted", "answer was due by"],
  ] as const)("renders canonical %s without reanchoring", (status, actBy, what, when) => {
    const original = CaseSummary.parse(synthetic[3]);
    const current = CurrentDecision.parse({ ...provenance.currents[original.id as keyof typeof provenance.currents], handoff: { status, act_by: actBy, decided_at: actBy - 4, clinical_priority: "urgent" } });
    const r = referralFrom(original, 120, false, handoffOf(current))!;
    render([r]);
    expect(content()).toContain(what); expect(content()).toContain(`${when} ${formatDay(actBy)}`);
    now = 150; render([r]); expect(content()).toContain(`${when} ${formatDay(actBy)}`);
    expect(content()).not.toContain(formatDay(153));
  });

  it("renders two different canonical dates at the same run day and the missing-packet state", () => {
    const a = { ...EXTRA[3], handoff: { status: "coordinator" as const, actBy: 103 } };
    const b = { ...a, id: "second-return", name: "Second return", handoff: { status: "medical_director" as const, actBy: 97 } };
    render([a, b]); open(a); expect(content()).toContain(`was due ${formatDay(103)}`);
    open(b); expect(content()).toContain(`was due ${formatDay(97)}`);
    h.go("/app/referrals");
    render([EXTRA[3]]); expect(content()).toContain("deadline unavailable");
  });

  it.each([[0, true, 4], [0, false, 7], [24, true, 32]] as const)("return day %i urgent=%s uses one assumed business-day deadline %i everywhere", async (day, urgent, due) => {
    now = day; const r = { ...positive, urgent }; render([r]);
    click("Return with reason", pane()); expect(content()).toContain(`must act by ${formatDay(due)}`);
    h.radio("Wrong unit"); click("Return to GP", pane());
    expect(content()).toContain(`Act by ${formatDay(due)}`); expect(content()).toContain(`by ${formatDay(due)}`);
    now = day + 10; render([r]); expect(content()).toContain(`by ${formatDay(due)}`);
    await undo(); expect(button("Accept referral", pane())).toBeTruthy();
    expect(content()).not.toContain(`Act by ${formatDay(due)}`);
  });
});
