import { readFile } from "node:fs/promises";
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

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("person journey contract", () => {
  it("publishes an opaque, pseudonymous, nonclinical journey", async () => {
    const journey = await sample();
    expect(journey.schema).toBe("wasl-person-journey/1");
    expect(journey.person.id).toMatch(/^person_[0-9a-f]{64}$/);
    expect(journey.person.registrations.every((r) => r.registration.startsWith("reg_"))).toBe(true);
    expect(journey.cases[0].detail).not.toHaveProperty("evidence");
    expect(JSON.stringify(journey)).not.toMatch(/review_items|api_reviews|concealed-r1|provider\|MRN/);
    expect(journey.next_action.rule).toBe("wasl-person-next-action/1");
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
    const [detail, audit] = await Promise.all([
      readFile("data/api/case_evidence_closure.json", "utf8"),
      readFile("data/api/case_evidence_closure_audit.json", "utf8"),
    ]);
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
    expect(html).toContain("Chain verified by the server");
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
    const journey = await sample();
    const html = renderToStaticMarkup(React.createElement(Journey, { journey }));
    expect(html).toContain("Trusted identity links only");
    expect(html).toContain("Programme contact permission");
    expect(html).toContain("Timeline and raw event hashes");
    expect(html).toContain("Decision history and signatures");
    expect(html).toContain("wasl-person-next-action/1");
    expect(html).toContain("No open coordinator action");
    expect(html).not.toContain("Suppress outreach");
    expect(html).not.toContain("Send message");
    expect(html).not.toContain("Dismiss duplicate");
    const ids = [...html.matchAll(/\sid="([^"]+)"/g)].map((match) => match[1]);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("never labels a near-miss link as trusted", async () => {
    const journey = await sample();
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
    expect(html).toContain("Release recorded — not evidence of dispatch or delivery");
    expect(reasonLabel("consent_not_granted")).toContain("not granted");
    expect(lifecycleLabel("withheld")).toContain("not actionable");
  });
});
