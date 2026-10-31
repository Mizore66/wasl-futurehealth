import { describe, expect, it } from "vitest";
import { CaseDetail } from "@/lib/api/schema";
import { dayToDate, formatDay } from "@/lib/domain/day";
import { caseHref, caseIdFromParam } from "@/lib/domain/ids";
import { caseStage } from "@/lib/domain/stage";
import { loadSample } from "./samples";

const load = (fixture: string): CaseDetail => CaseDetail.parse(loadSample(`case_${fixture}.json`));

describe("dayToDate", () => {
  it("day 0 is the cohort epoch, 1 May 2026", () => {
    expect(dayToDate(0).toISOString()).toBe("2026-05-01T00:00:00.000Z");
  });

  it("keeps the fraction of a day", () => {
    expect(dayToDate(30.5).toISOString()).toBe("2026-05-31T12:00:00.000Z");
  });

  it("formats the evidence-closure fixture's closing day", () => {
    const c = load("evidence_closure");
    expect(formatDay(c.case.closed_day)).toBe("27 Jun");
  });
});

describe("case ids in routes", () => {
  it("round-trips a loop id through a route segment", () => {
    const id = "ahs-mushrif/ServiceRequest/AHS-sr-1007";
    const segment = caseHref("/app/patients", id).split("/").at(-1)!;
    expect(caseIdFromParam(segment)).toBe(id);
    expect(caseIdFromParam(decodeURIComponent(segment))).toBe(id);
  });
});

describe("caseStage", () => {
  it("evidence closure is recorded, closed automatically", () => {
    const s = caseStage(load("evidence_closure"));
    expect(s).toMatchObject({ key: "recorded", step: 8, technical: "closed · auto" });
  });

  it("the false-closure trap is held at the gate with its rule", () => {
    const s = caseStage(load("false_closure_trap"));
    expect(s.key).toBe("gate_held");
    expect(s.step).toBe(6);
    expect(s.technical).toContain("inadequate_evidence");
  });

  it("the review hold is recorded after human sign-off", () => {
    expect(caseStage(load("review_hold"))).toMatchObject({ key: "recorded", technical: "closed · human_signoff" });
  });

  it("before sign-off, the review hold waits for two reviewers", () => {
    const c = load("review_hold");
    const pending = { ...c, case: { ...c.case, closed: false }, signoffs: [] };
    expect(caseStage(pending)).toMatchObject({ key: "waiting_for_reviewers", step: 7 });
  });

  it("an open link review with no report yet needs a second identifier", () => {
    const c = load("review_hold");
    const pending = {
      ...c,
      case: { ...c.case, closed: false },
      signoffs: [],
      evidence: [],
      review_items: c.review_items.filter((r) => r.kinds.includes("link")),
    };
    expect(caseStage(pending)).toMatchObject({ key: "needs_second_identifier", step: 3 });
  });

  it("outreach held while consent is unknown", () => {
    const c = load("review_hold");
    const first = c.decisions[0];
    const pending = { ...c, case: { ...c.case, closed: false }, review_items: [], decisions: [first] };
    expect(caseStage(pending)).toMatchObject({ key: "message_held_for_consent", step: 7 });
  });

  it("readers disagree when a report is read but no gate outcome exists", () => {
    const c = load("review_hold");
    const pending = { ...c, case: { ...c.case, closed: false }, review_items: [], decisions: [] };
    expect(caseStage(pending)).toMatchObject({ key: "readers_disagree", step: 5 });
  });

  it("a linked case with no report yet is matched; with no link it is received", () => {
    const c = load("evidence_closure");
    const open = { ...c, case: { ...c.case, closed: false } };
    expect(caseStage({ ...open, evidence: [] }).key).toBe("matched");
    expect(caseStage({ ...open, evidence: [], links: [] }).key).toBe("received");
  });
});
