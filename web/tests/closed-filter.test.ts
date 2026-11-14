import { describe, expect, it } from "vitest";
import { filtersHref, parseCaseFilters, toCaseQuery } from "@/components/worklist/filters";
import { CaseList, type CaseSummary } from "@/lib/api/schema";
import { matches } from "@/lib/api/source";
import { loadSample } from "./samples";

const PROGRAMMES = ["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"] as const;
const byProgramme = (p: string): CaseSummary[] => CaseList.parse(loadSample(`cases_${p}_page1.json`)).cases;
const NO_ACTION = ["no_followup_needed", "no_referral_needed", "no_recall_needed"];

describe("closed=true filter", () => {
  it("round-trips through parsing and links, composed with the other filters", () => {
    const f = { closed: true as const, programme: "breast" as const, provider: "bsu-screen", consent: "granted" as const, review: "open" as const, owner: "none", q: "sr-1" };
    const href = filtersHref("/app/patients", f);
    expect(href).toContain("closed=true");
    expect(parseCaseFilters(Object.fromEntries(new URL(href, "http://x").searchParams))).toEqual(f);
    expect(toCaseQuery(f)).toMatchObject({ closed: true, programme: "breast", unowned: true });
  });

  it("ignores a malformed value and keeps state=closed as its own filter", () => {
    expect(parseCaseFilters({ closed: "maybe" })).toEqual({});
    expect(parseCaseFilters({ state: "closed" })).toEqual({ state: "closed" });
    expect(filtersHref("/app/patients", { state: "closed" })).toBe("/app/patients?state=closed");
  });

  it("matches on the closed projection, not the state", () => {
    for (const p of PROGRAMMES) {
      const cases = byProgramme(p);
      const hits = cases.filter((c) => matches(c, { closed: true }));
      expect(hits.map((c) => c.id)).toEqual(cases.filter((c) => c.closed).map((c) => c.id));
      expect(cases.filter((c) => matches(c, { closed: false })).every((c) => !c.closed)).toBe(true);
    }
  });

  it("includes every documented no-action resolution that state=closed omits", () => {
    for (const p of ["lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"]) {
      const cases = byProgramme(p);
      const noAction = cases.filter((c) => c.closed && NO_ACTION.includes(c.state));
      expect(noAction.length, p).toBeGreaterThan(0);
      for (const c of noAction) {
        expect(matches(c, { closed: true }), c.id).toBe(true);
        expect(matches(c, { state: "closed" }), c.id).toBe(false);
      }
    }
  });

  it("state=closed still selects only the canonical closed state", () => {
    const cases = PROGRAMMES.flatMap(byProgramme);
    for (const c of cases.filter((x) => matches(x, { state: "closed" }))) expect(c.state).toBe("closed");
  });
});
