import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { CaseDetail } from "@/lib/api/schema";
import { dayToDate, formatDay } from "@/lib/domain/day";
import { caseHref, caseIdFromParam } from "@/lib/domain/ids";
import { caseStage } from "@/lib/domain/stage";
import { ProgrammeFindingCard, ReadersCard } from "@/components/case/sections";
import { programmeDetail, type NewProgramme } from "./programme-cases";
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
    expect(formatDay(c.case.closed_day)).toBe("20 Jun");
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
  it("the DR exam closure is recorded, closed automatically", () => {
    expect(caseStage(load("dr_exam_closure"))).toMatchObject({ key: "recorded", step: 8, technical: "closed · auto" });
  });

  it("evidence closure is recorded after two reviewers confirmed the link", () => {
    expect(caseStage(load("evidence_closure"))).toMatchObject({ key: "recorded", step: 8, technical: "closed · human_signoff" });
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
    const held = { ...c.decisions[0], consent_log: [{ kind: "appointment_reminder", to: "held", contact: "unknown" }] };
    const pending = { ...c, case: { ...c.case, closed: false }, review_items: [], decisions: [held] };
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

describe("programme-specific case evidence", () => {
  it.each(["lung_ldct", "ifhas"] as const)("renders the %s finding, recommendation and outcome", (programme) => {
    const detail = programmeDetail(programme);
    const html = renderToStaticMarkup(ProgrammeFindingCard({ detail }));
    if (programme === "lung_ldct") {
      expect(html).toContain("Lung-RADS category");
      expect(html).toContain("LR4X");
      expect(html).toContain("diagnostic chest CT");
      expect(html).toContain("Wk benign");
    } else {
      expect(html).toContain("IFHAS screening results");
      expect(html).toContain("bp=H;glyc=N;lipid=N;weight=N");
      expect(html).toContain("Blood pressure");
      expect(html).toContain("Managed");
      expect(html).toContain("IFU-dr-45");
    }
    expect(html).not.toContain("colonoscopy");
    expect(html).not.toContain("Bowel prep");
  });

  it.each(["lung_ldct", "ifhas"] as const)("renders %s reader evidence without the CRC checklist", (programme: NewProgramme) => {
    const detail = programmeDetail(programme);
    const html = renderToStaticMarkup(ReadersCard({ detail, ev: detail.evidence[0], id: `readers-${programme}` }));
    expect(html).toContain("Reader A");
    expect(html).toContain("Reader B");
    expect(html).not.toContain("DoH closure checklist");
    expect(html).not.toContain("Caecum reached");
    expect(html).not.toContain("Bowel prep");
    if (programme === "lung_ldct") {
      expect(html).toContain("Lung-RADS overall");
      expect(html).toContain("Lung-RADS worst");
      expect(html).toContain("Lung-RADS 4X; suspicious pulmonary nodule.");
    } else {
      expect(html).toContain("Reader note");
      expect(html).toContain("doh2025ifhas printed panel");
      expect(html).toContain("coded LOINC/UCUM results");
    }
  });
});

describe("plain", () => {
  it("names the chain and fixture identifiers in plain words and falls back to readable words", async () => {
    const { plain } = await import("@/lib/domain/terms");
    expect(plain("closure_decision")).toBe("Closed with evidence");
    expect(plain("review_hold")).toBe("Held for human review");
    expect(plain("awaiting_tissue")).toBe("Awaiting tissue");
  });
});
