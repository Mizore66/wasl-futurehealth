import { describe, expect, it } from "vitest";
import { Audit, CaseDetail, CaseList } from "@/lib/api/schema";
import { facets, programmesInView } from "@/components/patients/facets";
import { parseCaseFilters, parsePage, filtersHref, toCaseQuery } from "@/components/worklist/filters";
import { matches } from "@/lib/api/source";
import { closureChecks, elapsedDays, recordNodes, workItem } from "@/components/worklist/model";
import { referralFrom } from "@/components/worklist/referrals";
import { loadSample } from "./samples";

const detail = (f: string) => CaseDetail.parse(loadSample(`case_${f}.json`));
const audit = (f: string) => Audit.parse(loadSample(`case_${f}_audit.json`));
const page = CaseList.parse(loadSample("cases_crc_page1.json"));
const RUN_DAY = Math.max(...["evidence_closure", "false_closure_trap", "review_hold"].map((f) => detail(f).current.now as number));

describe("run clock", () => {
  it("is the latest current.now across the fixtures", () => {
    expect(RUN_DAY).toBe(120);
  });

  it("agrees with the API's overdue flag on every open case", () => {
    const open = page.cases.filter((c) => !c.closed);
    expect(open.length).toBeGreaterThan(10);
    for (const c of open) expect([c.id, elapsedDays(c, RUN_DAY) > 31]).toEqual([c.id, c.overdue]);
  });
});

describe("Today's tabs", () => {
  const byShort = (s: string) => page.cases.find((c) => c.id.endsWith(s))!;
  it("puts an unowned case under No owner, an incomplete report under Waiting, an unbooked one under To call", () => {
    expect(workItem(byShort("sr-1000"), RUN_DAY, false).view).toBe("none");
    expect(workItem(byShort("sr-1011"), RUN_DAY, true).view).toBe("waiting");
    expect(workItem(byShort("sr-1003"), RUN_DAY, false).view).toBe("call");
  });

  it("names only the simulated cast and falls back to the short id", () => {
    expect(workItem(byShort("sr-1011"), RUN_DAY, true).name).toBe("Aisha Al Mansoori");
    expect(workItem(byShort("sr-1003"), RUN_DAY, false).name).toBe("AHS-sr-1003");
  });
});

describe("patient record", () => {
  it("ends Fatima's record closed on day 27, sealed with the audit's closure hash", () => {
    const d = detail("evidence_closure");
    const a = audit("evidence_closure");
    const nodes = recordNodes(d, a.chain);
    const last = nodes.at(-1)!;
    expect(last).toMatchObject({ kind: "current", who: "Closed", when: "day 27 · 20 Jun" });
    expect(last.hash).toBe(a.chain.find((e) => e.type === "closure_decision")!.hash);
    const link = nodes.find((n) => n.who === "Wasl")!;
    expect(link.what).toContain("by name and details, p 0.989 before review, confirmed in review");
    expect(link.hash).toBe(a.chain.find((e) => e.type === "link_decision")!.hash);
  });

  it("ends an open case at its current stage, not a closure", () => {
    const nodes = recordNodes(detail("false_closure_trap"), audit("false_closure_trap").chain);
    expect(nodes.at(-1)).toMatchObject({ who: "Now", what: "Safety gate: held" });
    expect(nodes.some((n) => n.who === "Closed")).toBe(false);
  });

  it("reads the checklist from the readers' fields", () => {
    expect(closureChecks(detail("evidence_closure").evidence[0]).map((k) => k.result)).toEqual(["Yes", "Adequate", "Not needed: no tissue taken"]);
    expect(closureChecks(detail("false_closure_trap").evidence[0]).map((k) => k.result)).toEqual(["Yes", "Not stated", "Not needed: no tissue taken"]);
  });
});

describe("patients filters from the query string", () => {
  it("keeps known values and drops unknown ones", () => {
    expect(parseCaseFilters({ dropoff: "never_notified", overdue: "true", state: "nonsense", q: " 1021 " })).toEqual({
      dropoff: "never_notified",
      overdue: true,
      q: "1021",
    });
    expect(parseCaseFilters({ overdue: "yes", dropoff: "" })).toEqual({});
  });

  it("takes programme and page, and leaves page 1 out of links", () => {
    expect(parseCaseFilters({ programme: "crc" })).toEqual({ programme: "crc" });
    expect(parseCaseFilters({ programme: "nonsense" })).toEqual({});
    expect(parsePage({ page: "3" })).toBe(3);
    expect(parsePage({ page: "0" })).toBe(1);
    expect(parsePage({ page: "x" })).toBe(1);
    expect(filtersHref("/app/patients", { programme: "crc", overdue: true }, 2)).toBe("/app/patients?programme=crc&overdue=true&page=2");
    expect(filtersHref("/app/patients", { programme: "crc" }, 1)).toBe("/app/patients?programme=crc");
  });

  it("round-trips through a link", () => {
    const f = { state: "open" as const, overdue: false };
    const href = filtersHref("/app/patients", f);
    expect(href).toBe("/app/patients?state=open&overdue=false");
    expect(parseCaseFilters(Object.fromEntries(new URL(href, "http://x").searchParams))).toEqual(f);
  });
});

describe("patients refine filters", () => {
  const all = ["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast"].flatMap((p) => CaseList.parse(loadSample(`cases_${p}_page1.json`)).cases);

  it("parses, drops unknown enum values, and round-trips through a link", () => {
    const f = { programme: "cervical" as const, provider: "ahs-mushrif", consent: "denied" as const, review: "signed" as const, owner: "none", overdue: true };
    const href = filtersHref("/app/patients", f);
    expect(href).toBe("/app/patients?programme=cervical&overdue=true&provider=ahs-mushrif&consent=denied&review=signed&owner=none");
    expect(parseCaseFilters(Object.fromEntries(new URL(href, "http://x").searchParams))).toEqual(f);
    expect(parseCaseFilters({ consent: "maybe", review: "later" })).toEqual({});
  });

  it("owner=none asks for unowned cases only", () => {
    expect(toCaseQuery({ owner: "none" })).toEqual({ unowned: true });
    expect(toCaseQuery({ owner: "Practitioner/x", q: "1" })).toEqual({ owner: "Practitioner/x", q: "1" });
    const unowned = all.filter((c) => matches(c, toCaseQuery({ owner: "none" })));
    expect(unowned.length).toBeGreaterThan(0);
    expect(unowned.every((c) => c.owner === "")).toBe(true);
  });

  it("each control counts what choosing it would show, with the other filters applied", () => {
    const f = { programme: "crc" as const };
    const fx = facets(all, f);
    const crc = all.filter((c) => c.programme === "crc");
    // Owner options partition the programme's cases.
    expect(fx.owner.reduce((n, o) => n + o.count, 0)).toBe(crc.length);
    for (const o of fx.provider) expect(o.count).toBe(crc.filter((c) => c.provider === o.value).length);
    // The programme control ignores its own filter, so the other programmes stay listed with their totals.
    expect(fx.programme.find((o) => o.value === "breast")?.count).toBe(all.filter((c) => c.programme === "breast").length);
    expect(programmesInView(all, f).map((p) => p.programme)).toEqual(["crc"]);
    expect(programmesInView(all, {}).length).toBe(5);
  });

  it("keeps a chosen value listed at zero rather than dropping the filter", () => {
    expect(facets(all, { provider: "no-such-unit" }).provider.find((o) => o.value === "no-such-unit")?.count).toBe(0);
  });
});

describe("referral inbox", () => {
  const byShort = (s: string) => page.cases.find((c) => c.id.endsWith(s))!;
  it("flags an incomplete report for review and leaves the clinic's own drop-offs out", () => {
    expect(referralFrom(byShort("sr-1011"), RUN_DAY, true)).toMatchObject({ state: "grade", flags: ["Report incomplete"] });
    expect(referralFrom(byShort("sr-1003"), RUN_DAY, false)).toBeNull();
  });

  it("turns a missing owner into a flag, so the row shows Review instead of Accept", () => {
    expect(referralFrom(byShort("sr-1004"), RUN_DAY, false)!.flags).toEqual(["No owner at the clinic"]);
    expect(referralFrom(byShort("sr-1010"), RUN_DAY, false)!.flags).toEqual([]);
  });
});
