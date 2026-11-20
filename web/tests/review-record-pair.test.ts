// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewDesk, dueTitle, shownReads } from "@/components/review/review-desk";
import { reviewItemViews, reviewView, type IdentityRow } from "@/components/review/case-view";
import { pairChecks, pairState } from "@/components/review/pair-checks";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

const actions = vi.hoisted(() => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn() }));
vi.mock("@/app/app/review/[id]/actions", () => actions);
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const row = (referral: string, report: string, verdict: IdentityRow["verdict"] = "unavailable", field = "Date of birth"): IdentityRow => ({ field, referral, report, verdict });

describe("pair states come from the data, not from fixed rows", () => {
  it("keeps the API's comparison verdict when it gave one", () => {
    expect(pairState(row("1965-03-14", "1965-03-14", "differ"))).toBe("differ");
    expect(pairState(row("a", "b", "match"))).toBe("match");
    expect(pairState(row("x", "not recorded", "missing"))).toBe("missing");
    expect(pairState(row("x", "y", "placeholder"))).toBe("unclear");
  });

  it("compares values the API did not compare", () => {
    expect(pairState(row("1965-03-14", "1965-03-14"))).toBe("match");
    expect(pairState(row(" Mariam  Al Shamsi", "mariam al shamsi", "unavailable", "Name"))).toBe("match");
    expect(pairState(row("1965-03-14", "1965-04-14"))).toBe("differ");
    expect(pairState(row("1965-03-14", "not recorded"))).toBe("missing");
    expect(pairState(row("", "1965-03-14"))).toBe("missing");
  });

  it("never calls two scripts of one name a difference", () => {
    expect(pairState(row("مريم الشامسي", "Mariam Al Shamsi", "unavailable", "Name"))).toBe("unclear");
  });

  it("lists every identifier but the record ids in the checks", () => {
    const checks = pairChecks([row("a", "a", "match", "Name"), row("1", "2"), row("AHS-1", "SKM-2", "separate", "Record")]);
    expect(checks).toEqual([{ field: "Name", state: "match" }, { field: "Date of birth", state: "differ" }]);
  });
});

const ITEM = "rv-636161dc42837e08";
const linkState = (o: { slot: "r1" | "r2"; claimed: boolean; own?: boolean }) =>
  ReviewState.parse({
    item: ITEM,
    question: "link",
    state: o.slot === "r1" ? "awaiting_r1" : "awaiting_r2",
    head: "",
    next_read: o.slot,
    may_submit: o.claimed,
    claim_head: o.claimed ? "c".repeat(64) : "",
    claim: o.claimed ? { owner: "Rev-A", role: "reviewer", read: o.slot, claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" } : null,
    claim_actions: { claim: !o.claimed && !o.own, renew: o.claimed, release: o.claimed, takeover: false },
    reads: o.slot === "r1" ? [{ read: "r1", submitted: false }] : [{ read: "r1", submitted: true, ...(o.own ? { decision: "different", hash: "abcdef0123456789" } : { concealed: true }) }, { read: "r2", submitted: false }],
    disagreement: false,
    canonical_unchanged: true,
  });

describe("the record pair desk", () => {
  let root: Root | undefined;
  let container: HTMLDivElement;
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-11-13T10:00:00Z"));
  });
  afterEach(async () => {
    vi.useRealTimers();
    if (root) await act(async () => root!.unmount());
    container?.remove();
    root = undefined;
    actions.submitReviewRead.mockReset();
  });
  async function mount(state: ReviewState) {
    const detail = CaseDetail.parse(hold);
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
    const next = { href: "/app/review", label: "Back to Review", last: true };
    await act(async () => root!.render(createElement(ReviewDesk, { view: reviewView(detail), itemViews: reviewItemViews(detail), mode: "live", next, states: [state] })));
    return container.querySelector<HTMLElement>('section[data-question="link"]')!;
  }

  it("locks the identifiers before a claim and offers the claim", async () => {
    const card = await mount(linkState({ slot: "r1", claimed: false }));
    expect(card.textContent).toContain("First read due");
    expect(card.textContent).toContain("Claim to see the patient details.");
    expect([...card.querySelectorAll("button")].map((b) => b.textContent)).toContain("Claim first read");
    expect(container.textContent).not.toContain("1965-03-14");
    expect(card.querySelector('input[type="radio"]')).toBeNull();
  });

  it("shows both records and the checks once claimed, and blocks a submit without a choice", async () => {
    const card = await mount(linkState({ slot: "r1", claimed: true }));
    expect(card.querySelectorAll("[data-record]")).toHaveLength(2);
    expect(card.textContent).toContain("1965-03-14");
    expect(card.querySelector('[data-testid="pair-checks"]')).not.toBeNull();
    const submit = [...card.querySelectorAll("button")].find((b) => b.textContent === "Submit first read")!;
    await act(async () => submit.click());
    expect(card.querySelector('[role="alert"]')!.textContent).toBe("Choose an answer before you submit.");
    expect(actions.submitReviewRead).not.toHaveBeenCalled();
    await act(async () => card.querySelector<HTMLInputElement>('input[value="different"]')!.click());
    expect(card.querySelector('[data-testid="blocked-link"]')).toBeNull();
  });

  it("keeps the first reader's own answer hidden after submit and shows that the second read is due", async () => {
    const state = linkState({ slot: "r2", claimed: false, own: true });
    expect(dueTitle(state)).toBe("Second read due");
    expect(shownReads(state).every((r) => r.decision === undefined && r.hash === undefined)).toBe(true);
    const card = await mount(state);
    expect(card.textContent).toContain("Second read due");
    expect(card.textContent).toContain("Your first read is in");
    expect(card.textContent).not.toContain("Not the same patient");
    expect(card.textContent).not.toContain("abcdef012345");
    expect(container.textContent).not.toContain("1965-03-14");
  });
});
