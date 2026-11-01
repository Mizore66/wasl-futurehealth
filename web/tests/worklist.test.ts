import { describe, expect, it } from "vitest";
import { Audit, CaseDetail, CaseList } from "@/lib/api/schema";
import { parseCaseFilters, filtersHref } from "@/components/worklist/filters";
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
    expect(workItem(byShort("sr-1009"), RUN_DAY, true).view).toBe("waiting");
    expect(workItem(byShort("sr-1005"), RUN_DAY, false).view).toBe("call");
  });

  it("names only the simulated cast and falls back to the short id", () => {
    expect(workItem(byShort("sr-1009"), RUN_DAY, true).name).toBe("Aisha Al Mansoori");
    expect(workItem(byShort("sr-1005"), RUN_DAY, false).name).toBe("AHS-sr-1005");
  });
});

describe("patient record", () => {
  it("ends Fatima's record closed on day 27, sealed with the audit's closure hash", () => {
    const d = detail("evidence_closure");
    const a = audit("evidence_closure");
    const nodes = recordNodes(d, a.chain);
    const last = nodes.at(-1)!;
    expect(last).toMatchObject({ kind: "current", who: "Closed", when: "day 27 · 22 Jun" });
    expect(last.hash).toBe(a.chain.find((e) => e.type === "closure_decision")!.hash);
    const link = nodes.find((n) => n.who === "Wasl")!;
    expect(link.what).toContain("by name and details, p 0.949 before review, confirmed in review");
    expect(link.hash).toBe(a.chain.find((e) => e.type === "link_decision")!.hash);
  });

  it("ends an open case at its current stage, not a closure", () => {
    const nodes = recordNodes(detail("false_closure_trap"), audit("false_closure_trap").chain);
    expect(nodes.at(-1)).toMatchObject({ who: "Now", what: "Safety gate: held" });
    expect(nodes.some((n) => n.who === "Closed")).toBe(false);
  });

  it("reads the checklist from the readers' fields", () => {
    expect(closureChecks(detail("evidence_closure").evidence[0]).map((k) => k.result)).toEqual(["Yes", "Adequate", "Not needed: no tissue taken"]);
    expect(closureChecks(detail("false_closure_trap").evidence[0]).map((k) => k.result)).toEqual(["No", "Not stated", "Missing"]);
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

  it("round-trips through a link", () => {
    const f = { state: "open" as const, overdue: false };
    const href = filtersHref("/app/patients", f);
    expect(href).toBe("/app/patients?state=open&overdue=false");
    expect(parseCaseFilters(Object.fromEntries(new URL(href, "http://x").searchParams))).toEqual(f);
  });
});

describe("referral inbox", () => {
  const byShort = (s: string) => page.cases.find((c) => c.id.endsWith(s))!;
  it("flags an incomplete report for review and leaves the clinic's own drop-offs out", () => {
    expect(referralFrom(byShort("sr-1009"), RUN_DAY, true)).toMatchObject({ state: "grade", flags: ["Report incomplete"] });
    expect(referralFrom(byShort("sr-1005"), RUN_DAY, false)).toBeNull();
  });

  it("turns a missing owner into a flag, so the row shows Review instead of Accept", () => {
    expect(referralFrom(byShort("sr-1022"), RUN_DAY, false)!.flags).toEqual(["No owner at the clinic"]);
    expect(referralFrom(byShort("sr-1004"), RUN_DAY, false)!.flags).toEqual([]);
  });
});
