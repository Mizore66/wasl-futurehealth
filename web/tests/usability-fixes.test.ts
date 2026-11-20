import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { relativeDayHint, simulatedRunDate, staffDueTime, staffLocalTime } from "@/lib/domain/staff-time";
import { personaFromOidc, personaFromToken } from "@/lib/auth/session-persona";
import { shellPersona, usesReviewSession } from "@/components/shell/persona";
import { referrerAnswerLabel } from "@/components/worklist/referrals";
import { packetHeading, ReferrerHandoffs } from "@/components/worklist/referrer-handoffs";
import type { ReturnedPacket } from "@/components/worklist/returned-packets";
import { ALL_GLOSSARY_KEYS, GLOSSARY } from "@/lib/domain/glossary";
import { GlossaryPopover } from "@/components/wasl/glossary-popover";
import { PipelineHeading, SNAPSHOT_NOTE } from "@/components/pipeline/heading";
import { hasCredential } from "@/lib/api/source";
import { reviewerPersona } from "@/lib/api/reviewer-session";

vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: () => {}, push: () => {} }), usePathname: () => "/app/pipeline/matching" }));
vi.mock("next/headers", () => ({ cookies: async () => ({ get: () => undefined }) }));

afterEach(() => {
  vi.unstubAllEnvs();
});

describe("N2/N4 one staff date format (Asia/Dubai, relative hint)", () => {
  const now = new Date("2026-11-18T12:00:00Z"); // Wed 18 Nov, 16:00 in Abu Dhabi

  it("renders due and lease times in Dubai local time with a day-level hint", () => {
    expect(staffLocalTime("2026-11-19T16:02:00Z")).toBe("Thu 19 Nov, 20:02 Dubai time");
    expect(staffDueTime("2026-11-19T16:02:00Z", now)).toBe("Thu 19 Nov, 20:02 Dubai time (tomorrow)");
    expect(staffDueTime("2026-11-20T08:00:00Z", now)).toBe("Fri 20 Nov, 12:00 Dubai time (in 2 days)");
    expect(staffDueTime("2026-11-18T13:00:00Z", now)).toBe("Wed 18 Nov, 17:00 Dubai time (today)");
    expect(staffDueTime("2026-11-15T08:00:00Z", now)).toBe("Sun 15 Nov, 12:00 Dubai time (3 days ago)");
  });

  it("counts calendar days in Abu Dhabi, not UTC", () => {
    // 21:00 UTC is already 01:00 the next day in Abu Dhabi.
    expect(relativeDayHint(new Date("2026-11-18T21:00:00Z"), now)).toBe("tomorrow");
    expect(relativeDayHint(new Date("2026-11-17T19:59:00Z"), now)).toBe("yesterday");
  });

  it("passes an unparseable value through rather than inventing a time", () => {
    expect(staffDueTime("not a time", now)).toBe("not a time");
  });

  it("labels the run clock as a simulated date", () => {
    expect(simulatedRunDate(95.4)).toBe("run day 95, 4 Aug (simulated date)");
  });
});

describe("N4 / clinical safety: the shell shows who the review session acts as", () => {
  const session = { actor: "smoke-reviewer-1", role: "Reviewer", method: "demo" as const };

  it("names the API-bound actor when this server holds the token map", () => {
    const env = { WASL_API_TOKENS: "tok-aaaaaaaaaaaaaaaa:reviewer:smoke-reviewer-1,tok-bbbbbbbbbbbbbbbb:senior_reviewer:smoke-reviewer-3" };
    expect(personaFromToken("tok-aaaaaaaaaaaaaaaa", env)).toEqual(session);
    expect(personaFromToken("tok-bbbbbbbbbbbbbbbb", env)).toEqual({ actor: "smoke-reviewer-3", role: "Senior reviewer", method: "demo" });
  });

  it("falls back to the matching credential, then to an unnamed pasted token, never echoing the token", () => {
    const env = { WASL_API_REVIEWER_2_TOKEN: "second-reader-token-1234" };
    expect(personaFromToken("second-reader-token-1234", env).actor).toBe("Second reviewer credential");
    const unknown = personaFromToken("pasted-token-9999999999", env);
    expect(unknown.actor).toBe("Pasted reviewer token");
    expect(JSON.stringify(unknown)).not.toContain("pasted-token");
  });

  it("names an OIDC subject with its mapped credential", () => {
    expect(personaFromOidc("synthetic-r2", "reviewer_2")).toEqual({ actor: "synthetic-r2", role: "Second reviewer · OIDC", method: "oidc" });
  });

  it("shows the session on the pages that act with it and labels every other persona 'Viewing as'", () => {
    expect(usesReviewSession("/app/review/AHS-sr-692")).toBe(true);
    expect(usesReviewSession("/app/patients/AHS-sr-692/audit")).toBe(true);
    expect(usesReviewSession("/app/patients/AHS-sr-692")).toBe(false);
    expect(usesReviewSession("/app/today")).toBe(false);

    const desk = shellPersona("/app/review/AHS-sr-692", session);
    expect(desk).toMatchObject({ name: "smoke-reviewer-1", role: "Reviewer", source: "signed-in", initials: "SR" });
    expect(desk.name).not.toBe("Dr Omar Al Kaabi");

    expect(shellPersona("/app/review/AHS-sr-692", null)).toMatchObject({ name: "Dr Omar Al Kaabi", source: "viewing-as" });
    expect(shellPersona("/app/today", session)).toMatchObject({ name: "Hessa Al Ali", source: "viewing-as" });
    expect(shellPersona("/app/referrals/returned", null)).toMatchObject({ name: "Dr Layla Haddad", source: "viewing-as" });
  });

  it("has no session in fixtures mode", async () => {
    vi.stubEnv("WASL_API_URL", "");
    expect(await reviewerPersona()).toBeNull();
  });
});

describe("N9: the returned-referrals header follows credential availability", () => {
  it("reads the same variable the save path does", () => {
    expect(hasCredential("referrer", {})).toBe(false);
    expect(hasCredential("referrer", { WASL_API_REFERRER_TOKEN: "  " })).toBe(false);
    expect(hasCredential("referrer", { WASL_API_REFERRER_TOKEN: "referrer-token-1234" })).toBe(true);
    expect(hasCredential("referrer_lung", { WASL_API_REFERRER_TOKEN: "referrer-token-1234" })).toBe(false);
  });

  it("never claims answers are saved when the credential is missing", () => {
    expect(referrerAnswerLabel(true, true)).toEqual({ kind: "built", text: "Referrer answers: built, saved to the simulated case record" });
    const missing = referrerAnswerLabel(true, false);
    expect(missing.kind).toBe("designed");
    expect(missing.text).not.toMatch(/saved to/);
    expect(missing.text).toMatch(/not set up on this site/);
    expect(referrerAnswerLabel(false, false).text).toBe("Referrer answers: not available in this demo");
  });
});

const packet = (over: Partial<ReturnedPacket> = {}): ReturnedPacket => ({
  id: "synthetic/AHS-sr-676",
  name: "AHS-sr-676",
  short: "AHS-sr-676",
  head: "a".repeat(64),
  packet: "evt-1",
  source: "unit_return",
  unit: "SKMC endoscopy",
  unitId: "skmc-endo",
  reason: "Needs more information",
  recommendation: null,
  advice: null,
  urgent: false,
  infoRequested: [],
  others: [{ id: "noor-endo", label: "Noor endoscopy" }],
  actBy: 120,
  status: "awaiting_referrer",
  acknowledged: false,
  answer: null,
  ...over,
});
const answer = async () => ({ ok: true as const });

describe("WCAG 1.3.1 and 2.5.8 on returned-referral cards", () => {
  it("names a case once when it has no synthetic name, and separates name and case ID otherwise", () => {
    expect(packetHeading(packet())).toEqual({ name: "AHS-sr-676", caseId: null });
    expect(packetHeading(packet({ name: "Vilma Santos" }))).toEqual({ name: "Vilma Santos", caseId: "AHS-sr-676" });

    const unnamed = renderToStaticMarkup(createElement(ReferrerHandoffs, { packets: [packet()], answer: answer as never }));
    const h2 = unnamed.match(/<h2[^>]*>(.*?)<\/h2>/)![1].replace(/<[^>]+>/g, "");
    expect(h2).toBe("AHS-sr-676");

    const named = renderToStaticMarkup(createElement(ReferrerHandoffs, { packets: [packet({ name: "Vilma Santos" })], answer: answer as never }));
    expect(named.match(/<h2[^>]*>(.*?)<\/h2>/)![1].replace(/<[^>]+>/g, "")).toBe("Vilma Santos, case AHS-sr-676");
  });

  it("gives every answer radio a 24 px box inside a 44 px label row", () => {
    const html = renderToStaticMarkup(createElement(ReferrerHandoffs, { packets: [packet()], answer: answer as never }));
    const radios = html.match(/<input[^>]*type="radio"[^>]*>/g) ?? [];
    expect(radios.length).toBeGreaterThan(0);
    for (const r of radios) expect(r).toMatch(/\bsize-6\b/);
    expect(html).toMatch(/<label class="flex min-h-11[^"]*"><input[^>]*type="radio"/);
  });
});

describe("Live-mode pipeline matching falls back to a labelled replay", () => {
  it("states that the published snapshot is shown", () => {
    const html = renderToStaticMarkup(createElement(PipelineHeading, { title: "Identity matching", current: "/app/pipeline/matching", note: SNAPSHOT_NOTE }));
    expect(html).toContain("Replay of a recorded run");
    expect(html).toContain("shows the published demo instead");
    expect(renderToStaticMarkup(createElement(PipelineHeading, { title: "Identity matching", current: "/app/pipeline/matching" }))).not.toContain(
      "pipeline-snapshot-note",
    );
  });
});

describe("N10 glossary", () => {
  it("explains every term the staff screens use", () => {
    expect(ALL_GLOSSARY_KEYS).toEqual(["reads", "canonical", "overlay", "anchor", "comparison_hash"]);
    for (const k of ALL_GLOSSARY_KEYS) {
      expect(GLOSSARY[k].term.length).toBeGreaterThan(0);
      expect(GLOSSARY[k].meaning.length).toBeGreaterThan(40);
    }
    expect(GLOSSARY.reads.term).toBe("r1, r2, r3");
  });

  it("renders a real, collapsed button that opens a dialog", () => {
    const html = renderToStaticMarkup(createElement(GlossaryPopover, { terms: ["reads", "canonical"] }));
    const button = html.match(/<button[^>]*data-testid="glossary-trigger"[^>]*>/)![0];
    expect(button).toContain('type="button"');
    expect(button).toContain('aria-expanded="false"');
    expect(button).toContain('aria-haspopup="dialog"');
    expect(button).toContain("min-h-11");
    expect(html).toContain("Terms used here");
  });
});
