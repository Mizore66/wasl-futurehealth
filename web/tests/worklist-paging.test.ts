import { describe, expect, it } from "vitest";
import { ActiveCaseSummary } from "@/lib/api/schema";
import { PAGE_SIZE } from "@/components/worklist/filters";
import { paginate, pageOfId, pageWith, rangeText, pageStatusText, pagerTargets, pagerFocusAfter, type PagerControl } from "@/components/worklist/paging";
import { personForLoop } from "@/lib/people";
import { allCallWorkItems, allNewReferrals, syntheticCaseSummaries, syntheticReferrals, syntheticWorkItems } from "./paging-fixtures";

// Execute this self-test FIRST, independently of all pagination tests.
describe("T-FACTORY", () => {
  it("proves 1201 valid active unique clean summaries and actual 401/400/400 workItem classification", () => {
    const summaries = syntheticCaseSummaries(1201, { mixed: true });
    expect(new Set(summaries.map((c) => c.id)).size).toBe(1201);
    for (const c of summaries) {
      expect(ActiveCaseSummary.safeParse(c).success).toBe(true);
      expect(c.closed).toBe(false);
      expect(c.lifecycle).not.toBe("retained");
      expect(c.provider).toBe("ahs-mushrif");
      expect(c.programme).toBe("crc");
      for (const k of ["api_overlay", "contact_state", "api_patient"]) expect(Object.hasOwn(c, k)).toBe(false);
      expect(personForLoop(c.id)).toBeUndefined();
    }
    const items = syntheticWorkItems(1201);
    expect(["call", "waiting", "none"].map((view) => items.filter((x) => x.view === view).length)).toEqual([401, 400, 400]);
    for (const x of items) {
      expect(x.name).toBe(x.short);
      expect(x.named).toBe(false);
      expect(x.head).toBeUndefined();
      expect(x.contactBlocked).toBeFalsy();
      expect(x.patient).toBeUndefined();
      expect(x.ledgerStatus).toBeUndefined();
    }
    const traversal = allCallWorkItems(1201);
    expect(traversal).toHaveLength(1201);
    expect(traversal.every((x) => x.view === "call")).toBe(true);
    expect(traversal[24].days).toBe(traversal[25].days);
    const referrals = syntheticReferrals(1201);
    expect(referrals).toHaveLength(1201);
    expect(new Set(referrals.map((x) => x.state))).toEqual(new Set(["new", "grade", "returned", "accepted", "done"]));
    expect(new Set(referrals.map((x) => x.id)).size).toBe(1201);
    expect(allNewReferrals(1201).filter((x) => x.state === "new")).toHaveLength(1201);
  });
});


describe("pure paging over the complete ordered input", () => {
  const rows = Object.freeze(Array.from({ length: 1201 }, (_, i) => ({ id: String(i) })));
  it("bounds empty, exact and partial pages without mutating or reordering input", () => {
    expect(paginate([], 9)).toEqual({ rows: [], page: 1, pages: 1, total: 0, from: 0, to: 0, size: PAGE_SIZE });
    for (const count of [25, 26, 50, 1200, 1201]) {
      const input = Object.freeze(rows.slice(0, count));
      const pages = Math.ceil(count / 25);
      const seen = Array.from({ length: pages }, (_, i) => paginate(input, i + 1).rows).flat();
      expect(seen).toEqual(input); expect(new Set(seen).size).toBe(count);
      expect(paginate(input, 999).page).toBe(pages);
      expect(paginate(input, pages).to).toBe(count);
    }
    expect(paginate(rows, 49)).toMatchObject({ rows: [rows[1200]], page: 49, pages: 49, from: 1201, to: 1201 });
    for (const invalid of [NaN, 0, -3, 2.5, Infinity]) expect(paginate(rows, invalid).page).toBe(1);
    expect(rows.map((x) => x.id)).toEqual(Array.from({ length: 1201 }, (_, i) => String(i)));
  });
  it("finds selected ids without treating missing ids as page zero or resetting a valid fallback", () => {
    for (const [id, page] of [["0", 1], ["25", 2], ["1200", 49]] as const) {
      expect(pageOfId(rows, id)).toBe(page); expect(pageWith(rows, id, 10)).toBe(page);
    }
    for (const id of [null, undefined, "absent"]) expect(pageOfId(rows, id)).toBeNull();
    expect(pageWith(rows, "absent", 3)).toBe(3); expect(pageWith(rows, null, 999)).toBe(49);
  });
  it("announces ranges truthfully for complete and incomplete inputs", () => {
    const p = paginate(rows, 2);
    expect(rangeText(p)).toBe("Showing 26–50 of 1201"); expect(rangeText(p, false)).toBe("Showing 26–50");
    expect(pageStatusText(p)).toBe("Page 2 of 49, showing 26 to 50 of 1201.");
    expect(pageStatusText(p, false)).toBe("Page 2, showing 26 to 50.");
  });
  it("omits end controls and preserves focus on every surviving control or its opposite", () => {
    const controls: PagerControl[] = ["first", "prev", "next", "last"];
    const cases = [
      { p: paginate(rows, 1), targets: [null, null, 2, 49], focus: ["next", "next", "next", "last"] },
      { p: paginate(rows, 2), targets: [1, 1, 3, 49], focus: controls },
      { p: paginate(rows, 49), targets: [1, 48, null, null], focus: ["first", "prev", "prev", "prev"] },
      { p: paginate([], 1), targets: [null, null, null, null], focus: [null, null, null, null] },
    ];
    for (const { p, targets, focus } of cases) {
      expect(controls.map((k) => pagerTargets(p)[k])).toEqual(targets);
      expect(controls.map((k) => pagerFocusAfter(k, p))).toEqual(focus);
    }
  });
  it("rejects invalid sizes consistently", () => {
    for (const size of [0, -1, 1.5, Infinity, NaN]) {
      expect(() => paginate(rows, 1, size)).toThrow(RangeError);
      expect(() => pageOfId(rows, null, size)).toThrow(RangeError);
      expect(() => pageWith(rows, null, 1, size)).toThrow(RangeError);
    }
    expect(paginate(rows, 1).rows).toHaveLength(PAGE_SIZE);
  });
});
