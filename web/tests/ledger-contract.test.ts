import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HistoryCard } from "@/components/case/audit-view";
import { LedgerCard, SignoffsCard } from "@/components/case/sections";
import { runClock } from "@/components/worklist/run-clock";
import { workItem } from "@/components/worklist/model";
import { Audit, CaseDetail, CaseList, LedgerEvent } from "@/lib/api/schema";
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

  it("renders compensation status and links between original and resolving events without deanonymising audit history", () => {
    const audit = Audit.parse(rawAudit);
    const undo = rawAudit.history.find((event: { kind?: string }) => event.kind === "undo");
    const governance = {
      caseId: rawAudit.case,
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
    expect(html).toContain("Reviewer 3");
    expect(html).toMatch(/<textarea[^>]*required/);
  });

  it("offers undo and contest only for an eligible live review, never for fixture history", () => {
    const review = rawAudit.history.find((event: { decision?: string }) => event.decision === "different");
    const audit = Audit.parse({ ...rawAudit, history: [review] });
    const governance = {
      caseId: rawAudit.case,
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

  it("an actual review still needs every review field", () => {
    const parsed = LedgerEvent.parse({ ...rawDetail.api_reviews[2], note: undefined });
    expect(parsed.ledger).toBe("unknown");
  });
});

describe("one Today action does not break pages that read every fixture's case detail", () => {
  it("runClock and the worklist read a detail whose ledger holds coordinator events", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
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
