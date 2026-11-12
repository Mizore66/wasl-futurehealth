import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HistoryCard } from "@/components/case/audit-view";
import { LedgerCard, SignoffsCard } from "@/components/case/sections";
import { runClock } from "@/components/worklist/run-clock";
import { workItem } from "@/components/worklist/model";
import { Audit, AuditLedgerEvent, CaseDetail, CaseList, LedgerEvent } from "@/lib/api/schema";
import { caseStage } from "@/lib/domain/stage";
import { loadSample } from "./samples";

/*
 * fixtures/ledger_*.json are GET /api/cases/{id}, GET /api/cases and GET /api/cases/{id}/audit as the Go API serves
 * them after a consent event, two link reviews, every coordinator action kind, an undo, a contest and a contest
 * resolution were appended through the authenticated handler with role-bound tokens (pipeline/api_ledger_contract_test.go,
 * WASL_WRITE_LEDGER_FIXTURES=1). They are real handler output, not hand-written JSON.
 */
const load = (f: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", f), "utf8"));
const rawDetail = load("ledger_case_detail.json");
const rawList = load("ledger_case_list.json");
const rawAudit = load("ledger_audit.json");

/** The ledger order the Go test appends, as `ledger:kind-or-decision`. */
const ORDER = [
  "review:consent_granted",
  "coordinator:assign_owner",
  "review:same",
  "coordinator:contact_outcome",
  "coordinator:contact_outcome",
  "review:different",
  "coordinator:snooze",
  "coordinator:note",
  "compensation:undo",
  "compensation:contest",
  "compensation:contest_resolution",
  "coordinator:contact_outcome",
];
const order = (events: { ledger: string; kind?: unknown; decision?: unknown }[]) =>
  events.map((e) => `${e.ledger}:${String(e.ledger === "review" ? e.decision : e.kind)}`);

function renderGovernanceHistory(events: Record<string, unknown>[], statuses: [string, string][], signedIn = true, hostSignOut = false) {
  const audit = Audit.parse({ ...rawAudit, history: events });
  const byHash = new Map(events.map((event) => [event.hash, event]));
  const governance = {
    caseId: rawAudit.case,
    signedIn,
    hostSignOut,
    projection: {
      ...rawAudit.meta,
      case: rawAudit.case,
      case_status: "contested",
      canonical_state: rawDetail.case.state,
      note: "projection only",
      events: statuses.map(([hash, status]) => {
        const event = byHash.get(hash)!;
        return { hash, kind: event.kind ?? "review", actor: "projection-actor", action: event.action, target: event.target ?? "", status, at: event.at };
      }),
    },
  };
  return renderToStaticMarkup(HistoryCard({ audit, programme: "crc", governance } as never));
}

function reviewFor(action: "record_consent" | "signoff_evidence") {
  return {
    ...rawAudit.history.find((event: { decision?: string }) => event.decision === "same"),
    hash: action === "record_consent" ? "d".repeat(64) : "e".repeat(64),
    action,
    decision: action === "record_consent" ? "consent_granted" : "adequate",
  };
}

function contestFor(review: Record<string, unknown>) {
  return {
    ...rawAudit.history.find((event: { kind?: string }) => event.kind === "contest"),
    hash: "f".repeat(64),
    target: review.hash,
    target_action: review.action,
    action: review.action,
  };
}

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("shared ledger contract (real Go handler output)", () => {
  it("case detail parses every event shape and keeps each one's real type/kind", () => {
    const r = CaseDetail.safeParse(rawDetail);
    expect(r.error?.issues ?? []).toEqual([]);
    expect(order(r.data!.api_reviews)).toEqual(ORDER);
    for (const e of r.data!.api_reviews) {
      if (e.ledger === "coordinator") {
        expect([e.type, e.role, e.actor]).toEqual(["coordinator_action", "coordinator", "coordinator-bound"]);
      }
      expect(e.ledger).not.toBe("unknown");
    }
  });

  it("the redacted audit history parses with no actor or note on any event", () => {
    const r = Audit.safeParse(rawAudit);
    expect(r.error?.issues ?? []).toEqual([]);
    expect(order(r.data!.history)).toEqual(ORDER);
    for (const e of rawAudit.history) {
      expect(e).not.toHaveProperty("actor");
      expect(e).not.toHaveProperty("note");
    }
  });

  it("the case list parses with the overlay the actions left", () => {
    const r = CaseList.safeParse(rawList);
    expect(r.error?.issues ?? []).toEqual([]);
    const ov = r.data!.cases[0].api_overlay!;
    expect([ov.actions, ov.owner, ov.ledger_status]).toEqual([6, "coordinator-bound", "ok"]);
  });

  it("case pages render each event by its kind and the actor the API recorded", () => {
    const detail = CaseDetail.parse(rawDetail);
    const signoffs = renderToStaticMarkup(SignoffsCard({ detail }));
    expect(signoffs).toContain("Undid an earlier sign-off");
    expect(signoffs).toContain("Contested an earlier sign-off");
    expect(signoffs).toContain("Contest resolved: rejected");
    expect(signoffs).toContain("reviewer-two");
    expect(signoffs).not.toContain("coordinator_action");

    const ledger = renderToStaticMarkup(LedgerCard({ detail }));
    expect(ledger).toContain("Owner set to coordinator-bound");
    expect(ledger).toContain("Contact logged: Appointment agreed (phone)");
    expect(ledger).toContain("Contact logged: Declined the procedure (phone)");
    expect(ledger).toContain("coordinator_action/contact_outcome");
    expect(ledger).toContain("coordinator_action/snooze");
    expect(ledger).toContain("synthetic coordinator note");
    expect(ledger).toContain("coordinator-bound");
    expect(ledger).not.toContain("self-asserted");
    expect(ledger.match(/<li /g)).toHaveLength(6);
  });

  it("the audit view names coordinator and compensation events without an actor", () => {
    const audit = Audit.parse(rawAudit);
    const html = renderToStaticMarkup(HistoryCard({ audit, programme: "crc" }));
    expect(html).toContain("Contact logged: Reached (phone)");
    expect(html).toContain("Owner changed"); // the redacted audit does not carry the owner name
    expect(html).toContain("contest_resolution");
    expect(html).not.toContain("coordinator-bound");
    expect(html.match(/<li /g)).toHaveLength(ORDER.length);
  });

  it("coordinator and compensation events never count as answers to review items", () => {
    const detail = CaseDetail.parse(rawDetail);
    expect(() => caseStage(detail)).not.toThrow();
  });

  it("tags claim history separately from clinical answers and accepts actor-redacted audit claims", () => {
    const claim = {
      type: "review_claim", kind: "renew", case: rawDetail.case.id, item: "rv-1", question: "link", read: "r1", role: "reviewer", actor: "Rev-A",
      action: "signoff_link", at: "2026-11-13T10:01:00Z", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:01:00Z",
      expires_at: "2026-11-13T10:06:00Z", expected_claim_head: "a".repeat(64), mode: "demo", prev: "p", hash: "h-claim",
    };
    const detail = CaseDetail.parse({ ...rawDetail, api_reviews: [claim] });
    expect(detail.api_reviews[0]).toMatchObject({ ledger: "claim", kind: "renew", actor: "Rev-A" });
    const redacted = Object.fromEntries(Object.entries(claim).filter(([key]) => key !== "actor"));
    const audit = Audit.parse({ ...rawAudit, history: [redacted] });
    expect(audit.history[0]).toMatchObject({ ledger: "claim", kind: "renew" });
    expect(audit.history[0]).not.toHaveProperty("actor");
    expect(detail.api_reviews[0]).not.toHaveProperty("decision");
    expect(() => caseStage(detail)).not.toThrow();
  });

  it("renders compensation status and links between original and resolving events without deanonymising audit history", () => {
    const audit = Audit.parse(rawAudit);
    const undo = rawAudit.history.find((event: { kind?: string }) => event.kind === "undo");
    const governance = {
      caseId: rawAudit.case,
      signedIn: true,
      projection: {
        ...rawAudit.meta,
        case: rawAudit.case,
        case_status: "compensated",
        canonical_state: rawDetail.case.state,
        note: "projection only",
        events: rawAudit.history.map((event: { hash: string; kind?: string; action: string; target?: string; at: string }) => ({
          hash: event.hash,
          kind: event.kind ?? "review",
          actor: "redacted-projection-actor",
          action: event.action,
          target: event.target ?? "",
          status: event.hash === undo.target ? "compensated" : event.kind === "contest" ? "rejected" : event.kind === "undo" || event.kind === "contest_resolution" ? "applied" : event.kind ? "" : "active",
          at: event.at,
        })),
      },
    };
    const html = renderToStaticMarkup(HistoryCard({ audit, programme: "crc", governance } as never));
    expect(html).toContain("Review status: compensated");
    expect(html).toContain("Contest status: rejected");
    expect(html).toContain(`id="review-event-${undo.target}"`);
    expect(html).toContain(`href="#review-event-${undo.target}"`);
    expect(html).toContain("Resolving event");
    expect(html).not.toContain("redacted-projection-actor");
  });

  it("shows an open contest and its independent-resolution form with a required reason", () => {
    const review = rawAudit.history.find((event: { hash: string }) => event.hash === contestHashForFixture());
    const contest = rawAudit.history.find((event: { kind?: string }) => event.kind === "contest");
    const audit = Audit.parse({ ...rawAudit, history: [review, contest] });
    const governance = {
      caseId: rawAudit.case,
      signedIn: true,
      projection: {
        ...rawAudit.meta,
        case: rawAudit.case,
        case_status: "contested",
        canonical_state: rawDetail.case.state,
        note: "projection only",
        events: [
          { hash: review.hash, kind: "review", actor: "reviewer-one", action: review.action, target: "", status: "contested", at: review.at },
          { hash: contest.hash, kind: "contest", actor: "reviewer-two", action: contest.action, target: review.hash, status: "open", at: contest.at },
        ],
      },
    };
    const html = renderToStaticMarkup(HistoryCard({ audit, programme: "crc", governance } as never));
    expect(html).toContain("Contest status: open");
    expect(html).toContain("Resolve contest");
    expect(html).toContain("different authorised actor");
    expect(html).toContain("signed-in credential");
    expect(html).not.toContain('name="credential"');
    expect(html).toContain('<option value="" selected="">Choose a resolution</option>');
    expect(html).toMatch(/<select[^>]*name="resolution"[^>]*required/);
    expect(html).toMatch(/<textarea[^>]*required/);
  });

  it("offers undo and contest only for an eligible live review, never for fixture history", () => {
    const review = rawAudit.history.find((event: { decision?: string }) => event.decision === "different");
    const audit = Audit.parse({ ...rawAudit, history: [review] });
    const governance = {
      caseId: rawAudit.case,
      signedIn: true,
      projection: {
        ...rawAudit.meta,
        case: rawAudit.case,
        case_status: "active",
        canonical_state: rawDetail.case.state,
        note: "projection only",
        events: [{ hash: review.hash, kind: "review", actor: "reviewer-one", action: review.action, target: "", status: "active", at: review.at }],
      },
    };
    const liveHtml = renderToStaticMarkup(HistoryCard({ audit, programme: "crc", governance } as never));
    expect(liveHtml).toContain("Append compensation");
    expect(liveHtml).toContain("Contest review");
    expect(liveHtml).toContain("Original event remains unchanged");
    const fixtureHtml = renderToStaticMarkup(HistoryCard({ audit, programme: "crc" }));
    expect(fixtureHtml).not.toContain("Append compensation");
    expect(fixtureHtml).not.toContain("Contest review");
  });

  it("offers a consent contest's resolution to the signed-in actor, with no credential choice", () => {
    const review = reviewFor("record_consent");
    const contest = contestFor(review);
    const html = renderGovernanceHistory([review, contest], [[review.hash, "contested"], [contest.hash, "open"]]);
    expect(html).toContain("Resolve contest");
    expect(html).not.toMatch(/name="credential"|Coordinator<\/option>|Clinician \d/);
  });

  it("offers undo and contest controls for an active evidence review and says who acts", () => {
    const review = reviewFor("signoff_evidence");
    const html = renderGovernanceHistory([review], [[review.hash, "active"]]);
    expect(html).toContain("Append compensation");
    expect(html).toContain("Contest review");
    expect(html).toContain('data-testid="governance-identity"');
    expect(html).toContain("act as the token you signed in with");
    expect(html).toContain("signs in separately");
    expect(html).toContain("distinct token the operator issued");
    expect(html).toMatch(/<button[^>]*>Sign out<\/button>/);
    expect(html).not.toContain('name="credential"');
  });

  it("does not repeat the host page's sign-out (the review desk shows its own)", () => {
    const review = reviewFor("signoff_evidence");
    const html = renderGovernanceHistory([review], [[review.hash, "active"]], true, true);
    expect(html).toContain('data-testid="governance-identity"');
    expect(html).toContain("Contest review");
    expect(html).not.toMatch(/<button[^>]*>Sign out<\/button>/);
  });

  it.each([
    ["an active review", "signoff_evidence", false],
    ["an open contest", "record_consent", true],
  ] as const)("signed out, %s shows a sign-in prompt instead of any control", (_label, action, contested) => {
    const review = reviewFor(action);
    const events = contested ? [review, contestFor(review)] : [review];
    const statuses: [string, string][] = contested ? [[review.hash, "contested"], [contestFor(review).hash, "open"]] : [[review.hash, "active"]];
    const html = renderGovernanceHistory(events, statuses, false);
    expect(html).toContain('data-testid="governance-signin"');
    expect(html).toContain("Sign in to undo, contest or resolve");
    expect(html).toContain('type="password"');
    expect(html).toContain("signs in separately");
    expect(html).not.toContain('data-testid="governance-identity"');
    for (const control of ["Append compensation", "Contest review", "Resolve contest", 'name="reason"', 'name="resolution"', 'name="credential"']) {
      expect(html).not.toContain(control);
    }
  });

  it("shows no sign-in prompt when nothing can be undone, contested or resolved", () => {
    const review = reviewFor("signoff_evidence");
    const html = renderGovernanceHistory([review], [[review.hash, "compensated"]], false);
    expect(html).not.toContain('data-testid="governance-signin"');
  });
});

function contestHashForFixture(): string {
  return rawAudit.history.find((event: { kind?: string }) => event.kind === "contest").target;
}

describe("unknown or malformed ledger events", () => {
  it("are kept as explicit unknown entries instead of failing the whole case", () => {
    const future = { type: "coordinator_action", kind: "callback_booked", case: "c", hash: "h-future", at: "2026-11-10T00:00:00Z", actor: "a", role: "coordinator" };
    const malformedReview = { case: "c", item: "", decision: "same", role: "reviewer", actor: "r", action: "signoff_link", at: "x", mode: "demo", prev: "p", hash: "h-bad" }; // no note
    const r = CaseDetail.safeParse({ ...rawDetail, api_reviews: [...rawDetail.api_reviews, future, malformedReview, 42] });
    expect(r.error?.issues ?? []).toEqual([]);
    const tail = r.data!.api_reviews.slice(-3);
    expect(tail.map((e) => e.ledger)).toEqual(["unknown", "unknown", "unknown"]);
    expect(tail[0]).toMatchObject({ label: "coordinator_action/callback_booked", hash: "h-future" });
    const html = renderToStaticMarkup(LedgerCard({ detail: r.data! }));
    expect(html).toContain("Unrecognised ledger event (coordinator_action/callback_booked)");
  });

  it("an event of an unrecognised type or kind is never read as a review, even with every review field", () => {
    // Real Go review events carry neither `type` nor `kind` (fixture rows 0 and 2).
    const review = rawDetail.api_reviews[2];
    const auditReview = rawAudit.history[2];
    expect([review.type, review.kind, auditReview.type, auditReview.kind]).toEqual([undefined, undefined, undefined, undefined]);
    expect(LedgerEvent.parse(review).ledger).toBe("review");
    expect(AuditLedgerEvent.parse(auditReview).ledger).toBe("review");

    const futureType = { type: "escalation", kind: "raised" };
    const futureKind = { kind: "retract" };
    for (const extra of [futureType, futureKind, { type: "" }, { kind: null }]) {
      expect(LedgerEvent.parse({ ...review, ...extra }).ledger).toBe("unknown");
      expect(AuditLedgerEvent.parse({ ...auditReview, ...extra }).ledger).toBe("unknown");
    }
    expect(AuditLedgerEvent.parse({ ...auditReview, ...futureType })).toMatchObject({ ledger: "unknown", label: "escalation/raised" });
    // Go's evKind treats an empty kind as a review, so the frontend does too.
    expect(LedgerEvent.parse({ ...review, kind: "" }).ledger).toBe("review");

    // A whole audit with one such event still parses, and shows it as unrecognised rather than as a review.
    const audit = Audit.parse({ ...rawAudit, history: [...rawAudit.history, { ...auditReview, ...futureKind, hash: "h-retract" }] });
    expect(audit.history.at(-1)).toMatchObject({ ledger: "unknown", hash: "h-retract", label: "retract" });
    expect(audit.history.filter((e) => e.ledger === "review")).toHaveLength(rawAudit.history.filter((e: { type?: unknown; kind?: unknown }) => !e.type && !e.kind).length);
  });

  it("an actual review still needs every review field", () => {
    const parsed = LedgerEvent.parse({ ...rawDetail.api_reviews[2], note: undefined });
    expect(parsed.ledger).toBe("unknown");
  });
});

describe("one Today action does not break pages that read every fixture's case detail", () => {
  it("runClock and the worklist read a detail whose ledger holds coordinator events", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "reviewer-secret-1234567890");
    const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => (url.endsWith("/api/fixtures") ? json(loadSample("fixtures.json")) : json(rawDetail))),
    );
    await expect(runClock()).resolves.toBe(rawDetail.current.now);
    const c = CaseList.parse(rawList).cases[0];
    expect(() => workItem(c, 40, true)).not.toThrow();
  });
});
