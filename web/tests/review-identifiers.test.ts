// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewDesk } from "@/components/review/review-desk";
import { reviewItemViews, reviewView } from "@/components/review/case-view";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

vi.mock("@/app/app/review/[id]/actions", () => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

type Field = { state: string; value: string };
const f = (state: string, value = ""): Field => ({ state, value });
const REFERRAL = "ahs-mushrif|AHS-MRN001006";
const REPORT = "skmc-endo|SKM-MRN000409";
const record = (registration: string, o: Partial<Record<"name" | "dob" | "sex" | "eid", Field>>) => ({
  registration,
  name: o.name ?? f("present", "Synthetic Name"),
  dob: o.dob ?? f("present", "1980-03-04"),
  sex: o.sex ?? f("present", "F"),
  eid: o.eid ?? f("missing"),
});
const withProjection = (identity_review: unknown) => CaseDetail.parse({ ...hold, identity_review });
const entry = (a: ReturnType<typeof record>, b: ReturnType<typeof record>, comparison: Record<string, string>, failures: string[], met = false) => [
  {
    item: "rv-636161dc42837e08",
    pair: [REFERRAL, REPORT],
    available: true,
    records: [a, b],
    comparison: Object.entries(comparison).map(([field, verdict]) => ({ field, verdict })),
    guard: { met, failures },
  },
];
const row = (detail: CaseDetail, field: string) => reviewView(detail).identity.find((r) => r.field === field)!;

describe("second-identifier comparison shows the recorded identifiers", () => {
  const view = reviewView(CaseDetail.parse(hold));

  it("renders both review_hold records' supplied fields instead of status placeholders", () => {
    const cells = view.identity.flatMap((r) => [r.referral, r.report]);
    expect(cells.join("|")).not.toMatch(/on record|not confirmed|not on report/);
    expect(view.identity.map((r) => r.field)).toEqual(["Name", "Date of birth", "Sex", "Emirates ID", "Record"]);
    expect(view.identity.find((r) => r.field === "Date of birth")).toMatchObject({ referral: "1965-03-14", report: "not recorded", verdict: "missing" });
    expect(view.identity.find((r) => r.field === "Sex")).toMatchObject({ referral: "Female", report: "unknown", verdict: "unknown" });
    expect(view.identity.find((r) => r.field === "Emirates ID")).toMatchObject({ referral: "784*********123", report: "784*********123", verdict: "match" });
    expect(view.identityAvailable).toBe(true);
    expect(view.identityGuard).toEqual(["Date of birth is not recorded on one or both records.", "Sex is unknown on one or both records, so a matching Emirates ID does not confirm the match."]);
  });

  it("flags a missing date of birth on the side that lacks it", () => {
    const d = withProjection(entry(record(REFERRAL, {}), record(REPORT, { dob: f("missing") }), { dob: "missing" }, ["dob_missing"]));
    expect(row(d, "Date of birth")).toMatchObject({ referral: "1980-03-04", report: "not recorded", verdict: "missing" });
    expect(reviewView(d).identityGuard).toEqual(["Date of birth is not recorded on one or both records."]);
  });

  it("shows equal placeholder dates as placeholders, not as a confirming match", () => {
    const p = f("placeholder", "1975-01-01");
    const d = withProjection(entry(record(REFERRAL, { dob: p }), record(REPORT, { dob: p }), { dob: "placeholder" }, ["dob_placeholder"]));
    expect(row(d, "Date of birth")).toMatchObject({ referral: "1975-01-01 (placeholder)", report: "1975-01-01 (placeholder)", verdict: "placeholder" });
    expect(reviewView(d).identityGuard).toEqual(["A date of birth is a placeholder (1 January or 1 July), so it does not confirm the match."]);
  });

  it("shows differing real dates of birth", () => {
    const d = withProjection(entry(record(REFERRAL, { dob: f("present", "1990-05-06") }), record(REPORT, { dob: f("present", "1990-06-05") }), { dob: "differ" }, ["dob_differ"]));
    expect(row(d, "Date of birth")).toMatchObject({ referral: "1990-05-06", report: "1990-06-05", verdict: "differ" });
    expect(reviewView(d).identityGuard).toEqual(["The dates of birth differ."]);
  });

  it("shows unknown and differing sex as recorded", () => {
    const unknown = withProjection(entry(record(REFERRAL, {}), record(REPORT, { sex: f("unknown") }), { sex: "unknown" }, ["sex_unknown"]));
    expect(row(unknown, "Sex")).toMatchObject({ referral: "Female", report: "unknown", verdict: "unknown" });
    const differ = withProjection(entry(record(REFERRAL, {}), record(REPORT, { sex: f("present", "M") }), { sex: "differ" }, ["sex_differ"]));
    expect(row(differ, "Sex")).toMatchObject({ referral: "Female", report: "Male", verdict: "differ" });
    expect(reviewView(differ).identityGuard).toEqual(["The recorded sexes differ."]);
  });

  it("shows an Emirates ID conflict with both masked values", () => {
    const d = withProjection(
      entry(record(REFERRAL, { eid: f("present", "784*********123") }), record(REPORT, { eid: f("present", "784*********987") }), { eid: "differ" }, ["eid_differ"]),
    );
    expect(row(d, "Emirates ID")).toMatchObject({ referral: "784*********123", report: "784*********987", verdict: "differ" });
    expect(reviewView(d).identityGuard).toEqual(["The Emirates IDs differ."]);
  });

  it("says so when the projection is absent or unavailable, and invents no values", () => {
    const absent = reviewView(CaseDetail.parse({ ...hold, identity_review: undefined }));
    const unavailable = reviewView(withProjection([{ item: "rv-636161dc42837e08", pair: [REFERRAL, REPORT], available: false, reason: "records_not_found" }]));
    for (const v of [absent, unavailable]) {
      expect(v.identityAvailable).toBe(false);
      expect(v.identity.map((r) => r.field)).toEqual(["Name", "Record"]);
      expect(v.identityGuard).toEqual([]);
      expect(v.identitySummary).toMatch(/not available/);
      expect(v.identity.flatMap((r) => [r.referral, r.report]).join("|")).not.toMatch(/on record|not confirmed|not recorded|unknown/);
    }
  });
});

describe("the comparison is on screen before an identity answer", () => {
  let root: Root | undefined;
  let container: HTMLDivElement | undefined;
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-11-13T10:00:00Z"));
  });
  afterEach(async () => {
    vi.useRealTimers();
    if (root) await act(async () => root!.unmount());
    container?.remove();
    root = undefined;
  });
  const linkState = (slot: "r1" | "r2") =>
    ReviewState.parse({
      item: "rv-636161dc42837e08",
      question: "link",
      state: slot === "r1" ? "awaiting_r1" : "awaiting_r2",
      head: "",
      next_read: slot,
      may_submit: slot === "r1",
      claim_head: "c".repeat(64),
      claim: slot === "r1" ? { owner: "Rev-A", role: "reviewer", read: slot, claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" } : null,
      claim_actions: { claim: false, renew: slot === "r1", release: slot === "r1", takeover: false },
      reads: [{ read: "r1", submitted: slot === "r2" }],
      disagreement: false,
      canonical_unchanged: true,
    });
  async function mount(detail: CaseDetail, slot: "r1" | "r2") {
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
    const next = { href: "/app/review", label: "Back to Review", last: true };
    await act(async () =>
      root!.render(createElement(ReviewDesk, { view: reviewView(detail), itemViews: reviewItemViews(detail), mode: "live", next, states: [linkState(slot)] })),
    );
    return container;
  }

  it("shows the recorded values and the guard failure inside the item's context while the answer form is open", async () => {
    const c = await mount(CaseDetail.parse(hold), "r1");
    const ctx = c.querySelector<HTMLElement>('[data-context-item="rv-636161dc42837e08"]')!;
    expect(ctx.textContent).toContain("1965-03-14");
    expect(ctx.textContent).toContain("784*********123");
    expect(ctx.textContent).toContain("Date of birth is not recorded on one or both records.");
    expect(ctx.textContent).not.toMatch(/on record|not confirmed/);
    expect(c.querySelector('section[data-question="link"] input[type="radio"]')).not.toBeNull();
  });

  it("keeps the first answer hidden from the second reader and still shows the comparison", async () => {
    const c = await mount(CaseDetail.parse(hold), "r2");
    const card = c.querySelector<HTMLElement>('section[data-question="link"]')!;
    expect(card.textContent).toContain("first answer stays hidden");
    expect(card.querySelector('input[type="radio"]')).toBeNull();
    expect(c.querySelector('[data-context-item="rv-636161dc42837e08"]')!.textContent).toContain("1965-03-14");
  });

  it("says the comparison is unavailable instead of showing placeholders", async () => {
    const c = await mount(CaseDetail.parse({ ...hold, identity_review: undefined }), "r1");
    const ctx = c.querySelector<HTMLElement>('[data-context-item="rv-636161dc42837e08"]')!;
    expect(ctx.textContent).toMatch(/identifier comparison is not available/);
    expect(ctx.textContent).not.toMatch(/on record|not confirmed/);
  });
});
