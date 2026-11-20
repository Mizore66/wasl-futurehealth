import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { chainCheck, VerifiedCard } from "@/components/case/audit-view";
import { ReasonCard } from "@/components/case/sections";
import { StageMarks } from "@/components/case/stage-marks";
import { afterOutcome } from "@/components/case/teach-back-callback-card";
import { openReviewReasons } from "@/components/case/words";
import type { IdentityRow } from "@/components/review/case-view";
import { scriptHint } from "@/components/review/identity-bridge";
import { claimAnnouncement } from "@/components/review/review-desk";
import { Audit, CaseDetail, type TeachBackCallback } from "@/lib/api/schema";
import { showsSimulatedName } from "@/lib/people";
import { loadSample } from "./samples";

/* docs/USABILITY-AUDIT.md sections 3-4: the case record, audit view and review desk findings. */

const text = (html: string) => html.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();

describe("N1: teach-back callback after a recorded outcome", () => {
  const open: TeachBackCallback = { id: "cb-1", source: "patient_link", state: "misunderstood", owner: "nurse", status: "open", escalation: "nurse callback", due_at: "2026-11-19T16:02:00Z", overdue: true };
  const other: TeachBackCallback = { ...open, id: "cb-2" };

  it("uses the API's post-append list when the 201 response carries one", () => {
    const returned = [{ ...open, status: "resolved" as const, resolved_by: "coordinator-1" }, other];
    expect(afterOutcome([open, other], "cb-1", "understood_on_call", returned)).toBe(returned);
  });

  it("marks only the recorded callback resolved when the response has no list", () => {
    const next = afterOutcome([open, other], "cb-1", "understood_on_call");
    expect(next.map((cb) => [cb.id, cb.status, cb.overdue])).toEqual([
      ["cb-1", "resolved", false],
      ["cb-2", "open", true],
    ]);
  });

  it("leaves a callback open after 'another explanation'", () => {
    expect(afterOutcome([open], "cb-1", "another_explanation")[0].status).toBe("open");
  });
});

describe("N1: chain check with no sealed entries", () => {
  const audit = Audit.parse(loadSample("case_evidence_closure_audit.json"));
  const empty: Audit = { ...audit, chain: [], chain_length: 0, chain_verified: true };

  it("is neutral, not a positive '0 of 0 intact'", () => {
    expect(chainCheck(empty)).toEqual({ tone: "muted", shape: "dash", label: "Nothing recorded yet" });
    const html = text(renderToStaticMarkup(VerifiedCard({ audit: empty })));
    expect(html).toContain("Nothing recorded yet");
    expect(html).not.toContain("0 of 0 intact");
    expect(html).not.toContain("Every entry was checked");
  });

  it("still reads verified for a sealed chain and broken for a broken one", () => {
    expect(chainCheck(audit).tone).toBe("positive");
    expect(chainCheck(audit).label).toBe(`Unchanged · ${audit.chain.length} of ${audit.chain_length} entries checked`);
    expect(chainCheck({ ...empty, chain_verified: false }).label).toBe("Record altered");
  });
});

describe("N4: stage and overdue are two marks", () => {
  it("keeps the stage neutral and adds a separate Overdue mark", () => {
    const html = renderToStaticMarkup(StageMarks({ stage: "Received", overdue: true }));
    const pills = [...html.matchAll(/<span class="([^"]*)">(?:<svg[^]*?<\/svg>)?([^<]+)<\/span>/g)].map((m) => [m[2], m[1].includes("negative")]);
    expect(pills).toEqual([
      ["Received", false],
      ["Overdue", true],
    ]);
  });

  it("shows only the stage when the case is on time", () => {
    expect(text(renderToStaticMarkup(StageMarks({ stage: "Received", overdue: false })))).toBe("Received");
  });
});

describe("N8: why the case is still open sits above the timeline", () => {
  const held = CaseDetail.parse(loadSample("case_review_hold.json"));
  const open: CaseDetail = { ...held, case: { ...held.case, closed: false }, signoffs: [], api_reviews: [], api_review_states: [] } as CaseDetail;

  it("names the identity-in-review reason in the Why card", () => {
    expect(openReviewReasons(open)[0]).toBe("Identity in review: no second identifier is confirmed for the two records.");
    expect(text(renderToStaticMarkup(ReasonCard({ detail: open })))).toContain("Identity in review: no second identifier is confirmed");
    expect(openReviewReasons(held)).toEqual([]);
  });

  it("renders the Why card before the step-by-step chain on the case page", () => {
    // The case page and an open folder in the Referral calendar both render the shared Evidence File.
    const page = readFileSync(path.join(__dirname, "..", "components", "case", "evidence-file.tsx"), "utf8");
    const why = page.indexOf("<ReasonCard detail={detail} />");
    expect(why).toBeGreaterThan(-1);
    expect(page.indexOf("<ReasonCard", why + 1)).toBe(-1);
    expect(why).toBeLessThan(page.indexOf("<SealedChain"));
    // In the Evidence File itself, the reason sits in the next step, above the activity timeline.
    expect(page.indexOf('data-testid="why-summary"')).toBeGreaterThan(-1);
    expect(page.indexOf('data-testid="why-summary"')).toBeLessThan(page.indexOf('aria-label="Timeline"'));
  });
});

describe("Review desk: lease announcements", () => {
  it("uses the operation's past tense", () => {
    expect(claimAnnouncement("link", "claim")).toBe("Identity question claimed. The page is up to date.");
    expect(claimAnnouncement("evidence", "takeover")).toBe("Evidence question taken over. The page is up to date.");
    expect(claimAnnouncement("link", "release")).not.toMatch(/claim claim|release recorded/);
  });
});

describe("Identity comparison across scripts", () => {
  const row = (referral: string, report: string, verdict: IdentityRow["verdict"] = "differ"): IdentityRow => ({ field: "Name", referral, report, verdict });

  it("hints at transliteration for an Arabic-script vs Latin-script name that differs", () => {
    expect(scriptHint(row("فاطمة الزعابي", "Fatima Al Zaabi"))).toBe("different scripts; compare transliteration");
    expect(scriptHint(row("Fatima Al Zaabi", "فاطمة الزعابي"))).toBe("different scripts; compare transliteration");
  });

  it("adds nothing for same-script names, matches, or other fields", () => {
    expect(scriptHint(row("Fatima Al Zaabi", "Mariam Al Zaabi"))).toBeUndefined();
    expect(scriptHint(row("فاطمة الزعابي", "Fatima Al Zaabi", "match"))).toBeUndefined();
    expect(scriptHint({ ...row("فاطمة", "Fatima"), field: "Record" })).toBeUndefined();
  });
});

describe("N2: Simulated names chip", () => {
  it("is shown for a fixture loop with a cast name and hidden for a live case without one", () => {
    expect(showsSimulatedName("ahs-mushrif/ServiceRequest/AHS-sr-1001")).toBe(true);
    expect(showsSimulatedName("ahs-mushrif/ServiceRequest/AHS-sr-1046")).toBe(false);
  });
});
