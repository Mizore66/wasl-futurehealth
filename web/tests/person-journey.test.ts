import { readdir, readFile } from "node:fs/promises";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import CasePage from "@/app/app/patients/[id]/page";
import { Journey } from "@/components/person/journey";
import { lifecycleLabel, reasonLabel } from "@/components/person/journey-model";
import { getPerson, getPersonByCase } from "@/lib/api/person-source";
import { PersonJourney } from "@/lib/api/person-schema";

async function sample(): Promise<PersonJourney> {
  return PersonJourney.parse(JSON.parse(await readFile("data/api/person_journey.json", "utf8")));
}

/** Separate test data for the legitimate no-action state: the earlier single-programme CRC journey (synthetic). */
async function noActionSample(): Promise<PersonJourney> {
  return PersonJourney.parse(JSON.parse(await readFile("tests/fixtures/person-journey-no-action.json", "utf8")));
}

/** The case detail and audit files that the published sample holds for a case id (fixtures mode). */
async function caseFiles(id: string): Promise<[string, string]> {
  for (const file of (await readdir("data/api")).filter((f) => /^case_[a-z_]+\.json$/.test(f) && !f.endsWith("_audit.json"))) {
    const detail = await readFile(`data/api/${file}`, "utf8");
    if (JSON.parse(detail).case.id === id) return [detail, await readFile(`data/api/${file.replace(/\.json$/, "_audit.json")}`, "utf8")];
  }
  throw new Error(`no published case detail for ${id}`);
}

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("person journey contract", () => {
  it("publishes an opaque, pseudonymous, nonclinical journey", async () => {
    const journey = await sample();
    expect(journey.schema).toMatch(/^wasl-person-journey\/[12]$/);
    expect(journey.person.id).toMatch(/^person_[0-9a-f]{64}$/);
    expect(journey.person.registrations.every((r) => r.registration.startsWith("reg_"))).toBe(true);
    expect(journey.cases[0].detail).not.toHaveProperty("evidence");
    expect(JSON.stringify(journey)).not.toMatch(/review_items|api_reviews|concealed-r1|provider\|MRN/);
    expect(journey.next_action.rule).toBe("wasl-person-next-action/1");
  });

  it("publishes an actionable cross-programme journey whose references resolve inside it (#161)", async () => {
    const journey = await sample();
    const published = PersonJourney.parse(JSON.parse(await readFile("../docs/api-samples/person_journey.json", "utf8")));
    expect(published).toEqual(journey);
    const members = new Set(journey.person.case_ids);
    expect(journey.person.identity_provenance.source).toBe("canonical_linker_clusters");
    expect(new Set(journey.person.programmes).size).toBeGreaterThanOrEqual(3);
    expect(new Set(journey.cases.map((c) => c.detail.case.programme))).toEqual(new Set(journey.person.programmes));
    expect(journey.cases.every((c) => members.has(c.case_id))).toBe(true);
    expect(journey.person.anchor_case_id).toBe(journey.person.case_ids[0]);
    expect(journey.obligations.some((o) => (o.status === "open" || o.status === "overdue") && members.has(o.case_id))).toBe(true);
    const actionable = journey.outreach_flags.filter((f) => f.actionable);
    expect(actionable.length).toBeGreaterThan(0);
    for (const flag of actionable) expect(flag.intents.every((i) => members.has(i.case_id))).toBe(true);
    expect(journey.next_action.kind).not.toBe("none");
    expect(members.has(journey.next_action.case_id ?? "")).toBe(true);
    if (journey.next_action.outreach_flag_id) expect(journey.outreach_flags.map((f) => f.id)).toContain(journey.next_action.outreach_flag_id);
  });

  it("uses the server-held web reviewer token and URL-encodes case membership lookup", async () => {
    const journey = await sample();
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "person-web-reviewer-secret-123456");
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(journey), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    await getPersonByCase(journey.person.anchor_case_id);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe(`http://127.0.0.1:8088/api/people?case_id=${encodeURIComponent(journey.person.anchor_case_id)}`);
    expect(init.headers.get("authorization")).toBe("Bearer person-web-reviewer-secret-123456");
    expect(init.headers.has("x-wasl-role")).toBe(false);
    expect(init.headers.has("x-wasl-actor")).toBe(false);
    expect(init.cache).toBe("no-store");
  });

  it("keeps the case page available when the live identity projection is unavailable", async () => {
    const journey = await sample();
    const id = journey.person.anchor_case_id;
    const [detail, audit] = await caseFiles(id);
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "person-web-reviewer-secret-123456");
    vi.stubEnv("WASL_API_AUDITOR_TOKEN", "person-auditor-secret-123456");
    const fetchMock = vi.fn(async (input: string) => {
      const url = new URL(input);
      if (url.pathname === "/api/people") return new Response(JSON.stringify({ error: "identity projection is stale" }), { status: 503 });
      if (url.pathname === `/api/cases/${encodeURIComponent(id)}`) return new Response(detail, { status: 200 });
      if (url.pathname === `/api/cases/${encodeURIComponent(id)}/audit`) return new Response(audit, { status: 200 });
      throw new Error(`unexpected live request: ${input}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const page = await CasePage({ params: Promise.resolve({ id: encodeURIComponent(id) }), searchParams: Promise.resolve({}) });
    const html = renderToStaticMarkup(page);
    expect(html).toContain('data-testid="case-actions"');
    expect(html).toContain("Audit trail");
    // This published case has an empty audit chain: the teaser is neutral, not "verified" (USABILITY-AUDIT N1).
    expect(html).toContain("No sealed entries yet");
    expect(html).not.toContain("Chain verified by the server");
    expect(html).not.toContain("0 of 0 intact");
    expect(html).not.toContain("View all programmes");
  });

  it("does not substitute the fixture for an unknown person or case", async () => {
    vi.stubEnv("WASL_API_URL", "");
    await expect(getPerson("person_0000000000000000000000000000000000000000000000000000000000000000")).resolves.toBeNull();
    await expect(getPersonByCase("unknown/ServiceRequest/case")).resolves.toBeNull();
  });

  it("rejects raw registration identifiers at the API boundary", async () => {
    const journey = await sample();
    journey.cases[0].referral_registration = "provider|MRN0001";
    expect(() => PersonJourney.parse(journey)).toThrow();
  });
});

describe("person journey presentation", () => {
  it("renders provenance, consent, timelines, signatures and no write controls", async () => {
    for (const journey of [await sample(), await noActionSample()]) {
      const html = renderToStaticMarkup(React.createElement(Journey, { journey }));
      expect(html).toContain("Trusted identity links only");
      expect(html).toContain("Programme contact permission");
      expect(html).toContain("Timeline and raw event hashes");
      expect(html).toContain("Decision history and signatures");
      expect(html).toContain("wasl-person-next-action/1");
      expect(html).not.toContain("Suppress outreach");
      expect(html).not.toContain("Send message");
      expect(html).not.toContain("Dismiss duplicate");
      const ids = [...html.matchAll(/\sid="([^"]+)"/g)].map((match) => match[1]);
      expect(new Set(ids).size).toBe(ids.length);
    }
  });

  it("keeps the legitimate no-action state on its own test data", async () => {
    const journey = await noActionSample();
    expect(journey.next_action.kind).toBe("none");
    const html = renderToStaticMarkup(React.createElement(Journey, { journey }));
    expect(html).toContain("No open coordinator action");
  });

  it("never labels a near-miss link as trusted", async () => {
    const journey = await noActionSample(); // its case carries a trusted link to extend
    const trusted = journey.cases[0].detail.links[0];
    journey.cases[0].detail.links.push({ ...trusted, downstream_event: "provider/DiagnosticReport/untrusted-namesake", near_miss: true });
    const html = renderToStaticMarkup(React.createElement(Journey, { journey }));
    expect(html).toContain("Trusted case link");
    expect(html).not.toContain("untrusted-namesake");
    expect(html).not.toContain("near miss");
  });

  it("describes duplicate review without calling release delivery", async () => {
    const journey = await sample();
    journey.outreach_flags = [{
      id: `outreach_${"1".repeat(64)}`,
      match: "duplicate",
      actionable: true,
      intents: ["crc", "cervical"].map((programme, index) => ({
        case_id: `provider/ServiceRequest/${index}`,
        programme: programme as "crc" | "cervical",
        first_decision_id: `first-${index}`,
        latest_decision_id: `latest-${index}`,
        latest_decision_sig: "2".repeat(64),
        lifecycle: "released",
        outreach: { kind: "appointment_reminder", ev: "event", basis: "human", p_cal: 1, released: true, suppress: "", guard: "" },
      })),
    }];
    const html = renderToStaticMarkup(React.createElement(Journey, { journey }));
    expect(html).toContain("Duplicate intent");
    expect(html).toContain("Release recorded. This is not evidence of dispatch or delivery");
    expect(reasonLabel("consent_not_granted")).toContain("not granted");
    expect(lifecycleLabel("withheld")).toContain("not actionable");
  });

  it("labels an intent omitted by the latest decision as resolved history", async () => {
    const journey = await sample();
    journey.outreach_flags = [{
      id: `outreach_${"3".repeat(64)}`,
      match: "duplicate",
      actionable: false,
      intents: ["crc", "cervical"].map((programme, index) => ({
        case_id: `provider/ServiceRequest/${index}`,
        programme: programme as "crc" | "cervical",
        first_decision_id: `booked-${index}`,
        latest_decision_id: `booked-${index}`,
        latest_decision_sig: "4".repeat(64),
        lifecycle: "released",
        participation: "resolved",
        outreach: { kind: "appointment_reminder", ev: "event", basis: "human", p_cal: 1, released: true, suppress: "", guard: "" },
      })),
    }];
    const parsed = PersonJourney.parse(journey);
    const html = renderToStaticMarkup(React.createElement(Journey, { journey: parsed }));
    expect(html).toContain("not actionable");
    expect(html).toContain("Not in the latest decision");
  });
});
