import { describe, expect, it } from "vitest";
import { dueColumnWords } from "@/components/worklist/model";
import { CaseList, type CaseSummary } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { loadSample } from "./samples";

const cases = (page: string): CaseSummary[] => CaseList.parse(loadSample(`cases_${page}_page1.json`)).cases;
const byId = (page: string, id: string): CaseSummary => {
  const found = cases(page).find((c) => c.id === id);
  if (!found) throw new Error(`no ${id} in ${page}`);
  return found;
};

describe("no-action Patients due column uses the served next-screen date", () => {
  it("LDCT LCS-sr-119: the screen came after the referral, so the served due_day is shown, not referral + 365", () => {
    const c = byId("lung_ldct", "lcs-centre/ServiceRequest/LCS-sr-119");
    expect(c.state).toBe("no_recall_needed");
    expect(c.ldct!.due_day).toBeCloseTo(251.78078703703704, 9);
    expect(c.referral_day + 365).toBeCloseTo(168.5666, 4);
    expect(formatDay(c.ldct!.due_day!)).not.toBe(formatDay(c.referral_day + 365));
    expect(dueColumnWords(c)).toBe(`Next screen ${formatDay(c.ldct!.due_day!)}`);
  });

  it("DR PHC-sr-10: the served next_due.due_day is shown, not referral + 365", () => {
    const c = byId("diabetic_retinopathy", "phc-retina/ServiceRequest/PHC-sr-10");
    expect(c.state).toBe("no_referral_needed");
    expect(c.dr!.next_due!.due_day).toBeCloseTo(311.4144444444444, 9);
    expect(formatDay(c.dr!.next_due!.due_day)).not.toBe(formatDay(c.referral_day + 365));
    expect(dueColumnWords(c)).toBe(`Next screen ${formatDay(c.dr!.next_due!.due_day)}`);
  });

  it("breast and IFHAS show their served absolute next-screen dates", () => {
    const breast = cases("breast").find((c) => c.closed && c.state === "no_recall_needed" && c.breast!.screen_day !== c.referral_day)!;
    expect(dueColumnWords(breast)).toBe(`Next screen ${formatDay(breast.breast!.deadline_day!)}`);
    const ifhas = cases("ifhas").find((c) => c.closed && c.state === "no_followup_needed")!;
    expect(dueColumnWords(ifhas)).toBe(`Next screen ${formatDay(ifhas.ifhas!.deadline_day!)}`);
  });

  it("control: when the screen and referral share an anchor the served date equals the interval date", () => {
    const c = byId("lung_ldct", "lcs-centre/ServiceRequest/LCS-sr-1");
    expect(c.ldct!.scan_day).toBeCloseTo(c.referral_day, 2);
    expect(dueColumnWords(c)).toBe(`Next screen ${formatDay(c.referral_day + 365)}`);
    expect(dueColumnWords(c)).toBe(`Next screen ${formatDay(c.ldct!.due_day!)}`);
  });

  it("an absent served date reads as unavailable and never falls back to referral + interval", () => {
    const ldct = byId("lung_ldct", "lcs-centre/ServiceRequest/LCS-sr-119");
    const noDue = CaseList.parse({ ...(loadSample("cases_lung_ldct_page1.json") as object), cases: [{ ...ldct, ldct: { ...ldct.ldct, due_day: undefined } }] }).cases[0];
    expect(noDue.ldct!.due_day).toBeUndefined();
    expect(dueColumnWords(noDue)).toBe("Next screen date unavailable");
    expect(dueColumnWords(noDue)).not.toContain(formatDay(ldct.referral_day + 365));
    const dr = byId("diabetic_retinopathy", "phc-retina/ServiceRequest/PHC-sr-10");
    const noDr = CaseList.parse({ ...(loadSample("cases_diabetic_retinopathy_page1.json") as object), cases: [{ ...dr, dr: { ...dr.dr, next_due: undefined, deadline_day: undefined } }] }).cases[0];
    expect(dueColumnWords(noDr)).toBe("Next screen date unavailable");
  });

  it("a no-action case with no routine interval still says nothing is due", () => {
    expect(dueColumnWords(byId("lung_nodule", "rad-imaging/ServiceRequest/RAD-sr-10"))).toBe("None due");
  });

  it("procedural dates are unchanged: referral day plus the programme deadline", () => {
    const procedural = cases("crc").find((c) => c.closed && c.state === "closed")!;
    expect(procedural).toBeDefined();
    expect(dueColumnWords(procedural)).toBe(formatDay(procedural.referral_day + (procedural.deadline_day || 31)));
  });
});
