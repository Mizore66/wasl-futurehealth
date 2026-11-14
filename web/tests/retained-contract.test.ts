import { readFileSync } from "node:fs";
import path from "node:path";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ApiOverlay, BookingSummary, CaseDetail, CaseList, CaseState, CaseSummary, Programme } from "@/lib/api/schema";
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

describe("booking summary read normalization", () => {
  const unreserved = {
    status: "unreserved", mode: "simulated", revision: 0,
    reservation_id: null, reservation_version: null, updated_at: null, updated_event: null,
  };
  const slot = Object.freeze({ slot_id: "sim:colonoscopy:201:1", session_id: 201, unit: "AD-1", day: 20, procedure: "colonoscopy", deadline: 31 });

  it("restores only an absent unreserved slot without mutating the input or inventing metadata", () => {
    const raw = Object.freeze({ ...unreserved });
    const before = JSON.stringify(raw);
    expect(BookingSummary.parse(raw)).toEqual({ ...unreserved, slot: null });
    expect(raw).not.toHaveProperty("slot");
    expect(JSON.stringify(raw)).toBe(before);
  });

  it.each(["unreserved", "reserved", "cancelled"])("preserves explicit null and object slots for %s", (status) => {
    for (const explicitSlot of [null, slot]) {
      const raw = Object.freeze({ ...unreserved, status, slot: explicitSlot });
      expect(BookingSummary.parse(raw)).toEqual(raw);
      expect(raw.slot).toBe(explicitSlot);
    }
  });

  it.each(["reserved", "cancelled", "unknown"])("rejects an absent slot for %s", (status) => {
    expect(BookingSummary.safeParse({ ...unreserved, status }).success).toBe(false);
  });

  it.each(["unreserved", "reserved", "cancelled"])("rejects malformed or explicitly undefined slots for %s", (status) => {
    for (const malformed of [undefined, "slot", false, 0, [], {}, { ...slot, day: "20" }, { ...slot, slot_id: 201 }]) {
      expect(BookingSummary.safeParse({ ...unreserved, status, slot: malformed }).success).toBe(false);
    }
    for (const field of Object.keys(slot)) {
      const incomplete: Record<string, unknown> = { ...slot };
      delete incomplete[field];
      expect(BookingSummary.safeParse({ ...unreserved, status, slot: incomplete }).success).toBe(false);
    }
  });

  it("rejects malformed or missing status even with an explicit null slot", () => {
    for (const status of [undefined, null, "", "unknown", 0, false]) {
      expect(BookingSummary.safeParse({ ...unreserved, status, slot: null }).success).toBe(false);
    }
    const raw: Record<string, unknown> = { ...unreserved, slot: null };
    delete raw.status;
    expect(BookingSummary.safeParse(raw).success).toBe(false);
  });

  it.each(["mode", "revision", "reservation_id", "reservation_version", "updated_at", "updated_event"])("still requires %s when the unreserved slot is omitted", (field) => {
    const raw: Record<string, unknown> = { ...unreserved };
    delete raw[field];
    expect(BookingSummary.safeParse(raw).success).toBe(false);
  });

  it("does not repair invalid metadata or non-object summaries", () => {
    for (const invalid of [
      { ...unreserved, mode: "live" }, { ...unreserved, revision: -1 }, { ...unreserved, revision: 0.5 },
      { ...unreserved, reservation_id: 0 }, { ...unreserved, reservation_version: 0 },
      { ...unreserved, updated_at: 0 }, { ...unreserved, updated_event: 0 },
      null, undefined, [], "unreserved",
    ]) {
      expect(BookingSummary.safeParse(invalid).success).toBe(false);
    }
  });
});

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
        detail: { case: unknown; api_overlay: unknown; decisions: { sig: string }[] };
        audit: { chain: { hash: string }[] };
      };
      const before = JSON.stringify(exported);
      expect(exported.redacted).toBe(true);
      expect(raw).toHaveProperty("api_overlay.booking.slot", null);
      expect(raw).toHaveProperty("case.api_overlay.booking.slot", null);
      expect(exported.detail.case).toHaveProperty("api_overlay.booking.status", "unreserved");
      expect(exported.detail.case).not.toHaveProperty("api_overlay.booking.slot");
      expect(exported.detail.api_overlay).toHaveProperty("booking.status", "unreserved");
      expect(exported.detail.api_overlay).not.toHaveProperty("booking.slot");
      const summary = CaseSummary.parse(exported.detail.case);
      const overlay = ApiOverlay.parse(exported.detail.api_overlay);
      const live = CaseDetail.parse(raw);
      expect(summary.lifecycle).toBe("retained");
      expect(summary.actionable).toBe(false);
      expect(summary.api_overlay?.booking?.slot).toBeNull();
      expect(overlay.booking).toEqual(summary.api_overlay?.booking);
      expect(overlay.booking).toEqual(live.api_overlay?.booking);
      expect(summary.api_overlay?.booking).toEqual(live.case.api_overlay?.booking);
      expect(exported.detail.decisions.map((decision) => decision.sig)).toEqual(raw.decisions.map((decision) => decision.sig));
      expect(exported.audit.chain.every((block) => /^[0-9a-f]{64}$/.test(block.hash))).toBe(true);
      expect(JSON.stringify(exported)).toBe(before);
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
