import { readFileSync } from "node:fs";
import path from "node:path";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CaseDetail, CaseList, CaseState, CaseSummary, Programme } from "@/lib/api/schema";
import { PersonAssociationUnavailable, PersonJourney } from "@/lib/api/person-schema";
import { Journey, PersonAssociationUnavailableView } from "@/components/person/journey";
import { statusWords } from "@/components/worklist/model";
import { caseStage } from "@/lib/domain/stage";

// New fixtures are actual canonical exporter -> emitted partitioned OUT ->
// verified API -> authenticated HTTP output. TestRetainedCatalogWebContract
// checks them byte-for-byte; no replacement identity artifact is hand-authored.
function fixture(name: string): unknown {
  return JSON.parse(readFileSync(path.join(process.cwd(), "tests/fixtures/retained", `${name}.json`), "utf8"));
}

describe("versioned retained lifecycle HTTP contract", () => {
  for (const scenario of ["retired", "conflict"]) {
    it(`parses real ${scenario} case, list and active person responses without widening active enums`, () => {
      const detail = CaseDetail.parse(fixture(`${scenario}-case`));
      expect(detail.case.lifecycle).toBe("retained");
      expect(detail.case.state).toBe("inactive");
      expect(detail.case.closed).toBe(false);
      expect(caseStage(detail).key).toBe("retained");
      expect(statusWords(detail.case).next).toContain("no current care action");
      expect(CaseList.parse(fixture(`${scenario}-list`)).cases).toHaveLength(2);
      const person = PersonJourney.parse(fixture(`${scenario}-active-person`));
      expect(person.schema).toBe("wasl-person-journey/2");
      expect(person.cases).toHaveLength(1);
      expect(person.cases[0].case_id).toMatch(/\/b$/);
      expect(person.obligations.every((item) => item.case_id.endsWith("/b"))).toBe(true);
      expect(Programme.safeParse("").success).toBe(false);
      expect(CaseState.safeParse("inactive").success).toBe(false);
      expect(CaseSummary.safeParse({ ...detail.case, lifecycle: "active", inactive: false }).success).toBe(false);
      expect(CaseSummary.safeParse({ ...detail.case, state: "open", lifecycle: "active", inactive: false, programme: "" }).success).toBe(false);
      if (scenario === "conflict") {
        expect(detail.case.programme).toBe("");
        expect(detail.case.programme_availability).toBe("unavailable");
      }
    });

    it(`keeps lifecycle metadata and original decision hashes in the ${scenario} redacted export`, () => {
      const raw = fixture(`${scenario}-case`) as { decisions: { sig: string }[] };
      const exported = fixture(`${scenario}-export`) as {
        redacted: boolean;
        detail: { case: unknown; decisions: { sig: string }[] };
        audit: { chain: { hash: string }[] };
      };
      expect(exported.redacted).toBe(true);
      const summary = CaseSummary.parse(exported.detail.case);
      expect(summary.lifecycle).toBe("retained");
      expect(summary.actionable).toBe(false);
      expect(exported.detail.decisions.map((decision) => decision.sig)).toEqual(raw.decisions.map((decision) => decision.sig));
      expect(exported.audit.chain.every((block) => /^[0-9a-f]{64}$/.test(block.hash))).toBe(true);
    });
  }

  it("keeps genuinely stored active children as history-only after reassociation", () => {
    const current = PersonJourney.parse(fixture("children-reassociation-current"));
    const history = PersonJourney.parse(fixture("children-reassociation-history"));
    if (current.schema !== "wasl-person-journey/2" || history.schema !== "wasl-person-journey/2") throw new Error("wrong contract version");
    expect(current.cases.map((item) => item.case_id)).toEqual(["ahs-mushrif/ServiceRequest/a"]);
    expect(current.obligations.every((item) => !item.case_id.includes("#surv"))).toBe(true);
    expect(current.retained_references.every((item) => !item.case_id.includes("#surv"))).toBe(true);
    expect(history.cases).toEqual([]);
    expect(history.obligations).toEqual([]);
    expect(history.outreach_flags).toEqual([]);
    expect(history.next_action.kind).toBe("none");
    const children = history.retained_references.filter((item) => item.case_id.includes("#surv"));
    expect(children.map((item) => item.case_id)).toEqual(["ahs-mushrif/ServiceRequest/a#surv", "ahs-mushrif/ServiceRequest/a#surv2", "ahs-mushrif/ServiceRequest/a#surv3"]);
    expect(children.every((item) => item.lifecycle === "active" && !item.actionable && item.availability === "reference_only")).toBe(true);
    const html = renderToStaticMarkup(React.createElement(Journey, { journey: history }));
    expect(html).toContain("Historical association");
    expect(html).not.toContain("Retired case");
  });

  it("does not turn changed-subject child retirement cause into origin ownership", () => {
    const trigger = PersonJourney.parse(fixture("children-retire_changed_subject-current"));
    const origin = PersonJourney.parse(fixture("children-retire_changed_subject-history"));
    if (trigger.schema !== "wasl-person-journey/2" || origin.schema !== "wasl-person-journey/2") throw new Error("wrong contract version");
    expect(trigger.cases).toEqual([]);
    expect(trigger.retained_references.map((item) => item.case_id)).toEqual(["ahs-mushrif/ServiceRequest/a"]);
    const children = origin.retained_references.filter((item) => item.case_id.includes("#surv"));
    expect(children).toHaveLength(3);
    expect(children.every((item) => item.lifecycle === "retained" && !item.actionable && item.decisions.some((decision) => decision.inactive && decision.facts_seq === 7))).toBe(true);
    expect(origin.cases).toEqual([]);
    expect(origin.obligations).toEqual([]);
    expect(origin.next_action.kind).toBe("none");
  });

  it("keeps final trusted-cluster aliases and their emitted children current", () => {
    const journey = PersonJourney.parse(fixture("children-same-cluster"));
    expect(journey.cases).toHaveLength(4);
    expect(journey.cases.filter((item) => item.case_id.includes("#surv"))).toHaveLength(3);
    expect(journey.cases.every((item) => item.lifecycle === "active")).toBe(true);
  });

  it("renders retained-only proven history separately from current cases and obligations", () => {
    const journey = PersonJourney.parse(fixture("retired-historical-person"));
    expect(journey.schema).toBe("wasl-person-journey/2");
    if (journey.schema !== "wasl-person-journey/2") throw new Error("wrong contract version");
    expect(journey.cases).toEqual([]);
    expect(journey.obligations).toEqual([]);
    expect(journey.next_action.kind).toBe("none");
    expect(journey.retained_references).toHaveLength(1);
    expect(journey.retained_references[0].actionable).toBe(false);
    const html = renderToStaticMarkup(React.createElement(Journey, { journey }));
    expect(html).toContain("Non-actionable history references");
    expect(html).toContain("Retired case");
    expect(html).not.toContain("Record a new clinical");
  });

  it("parses and renders bounded case-local unavailability without fabricating a person", () => {
    const result = PersonAssociationUnavailable.parse(fixture("conflict-historical-person"));
    expect(result.association.reason).toBe("conflicting_request");
    expect("person" in result).toBe(false);
    expect(result.actionable).toBe(false);
    expect(PersonJourney.safeParse(result).success).toBe(false);
    const html = renderToStaticMarkup(React.createElement(PersonAssociationUnavailableView, { result }));
    expect(html).toContain("Historical association unavailable");
    expect(html).toContain("no current care or outreach action");
    expect(PersonAssociationUnavailable.safeParse({ ...result, association: { status: "unavailable", reason: "anything", registration: "invented" } }).success).toBe(false);
  });
});
