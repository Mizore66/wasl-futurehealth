import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import CasePage from "@/app/app/patients/[id]/page";
import { CaseDetail } from "@/lib/api/schema";

/*
 * Issues #104/#125. The record header names the saved owner. When the shared ledger replay is invalid, the API keeps the
 * valid prefix (so the saved owner stays visible) but that owner is not authoritative: the record must say so.
 * fixtures/ledger_partial_case_detail.json is real Go handler output (a replay that stopped at line 5); the owner below is
 * the one a valid prefix event would have saved.
 */
const load = (f: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", f), "utf8"));
const full = load("ledger_case_detail.json");
const partial = load("ledger_partial_case_detail.json");
const audit = readFileSync(path.join(__dirname, "fixtures", "ledger_audit.json"), "utf8");
const id: string = partial.case.id;
const SAVED = "Hessa Al Ali";
const BANNER = 'data-testid="ledger-invalid-board"';

const withOwner = (detail: typeof partial, over: Record<string, unknown> = {}) => ({
  ...detail,
  case: { ...detail.case, owner: "" },
  api_overlay: { ...detail.api_overlay, owner: SAVED, ...over },
});

async function renderRecord(detail: unknown): Promise<string> {
  CaseDetail.parse(detail);
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "record-web-reviewer-secret-123456");
  vi.stubEnv("WASL_API_AUDITOR_TOKEN", "record-auditor-secret-123456");
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string) => {
      const url = new URL(input);
      if (url.pathname === "/api/people") return new Response(JSON.stringify({ error: "identity projection is stale" }), { status: 503 });
      if (url.pathname === `/api/cases/${encodeURIComponent(id)}`) return new Response(JSON.stringify(detail), { status: 200 });
      if (url.pathname === `/api/cases/${encodeURIComponent(id)}/audit`) return new Response(audit, { status: 200 });
      throw new Error(`unexpected live request: ${input}`);
    }),
  );
  const page = await CasePage({ params: Promise.resolve({ id: encodeURIComponent(id) }), searchParams: Promise.resolve({}) });
  return renderToStaticMarkup(page);
}

beforeEach(() => {
  expect(partial.review_ledger.status).toBe("invalid");
  expect(partial.api_overlay.ledger_status).toBe("invalid");
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("record header saved owner under an invalid ledger", () => {
  it("replay prefix: keeps the saved owner visible and qualifies it as not authoritative", async () => {
    const html = await renderRecord(withOwner(partial));
    expect(html).toContain(`owner ${SAVED}`);
    expect(html).toContain(BANNER);
    expect(html).toContain("nothing on this record is authoritative");
  });

  it("warns when only review_ledger.status is invalid", async () => {
    const html = await renderRecord(withOwner(partial, { ledger_status: "ok" }));
    expect(html).toContain(`owner ${SAVED}`);
    expect(html).toContain(BANNER);
  });

  it("warns when only api_overlay.ledger_status is invalid", async () => {
    const detail = { ...withOwner(partial), review_ledger: { status: "ok", events_applied: 4 } };
    const html = await renderRecord(detail);
    expect(html).toContain(`owner ${SAVED}`);
    expect(html).toContain(BANNER);
  });

  it("still warns when the overlay is invalid and there is no review_ledger block", async () => {
    const { review_ledger: _dropped, ...rest } = withOwner(partial);
    void _dropped;
    const html = await renderRecord(rest);
    expect(html).toContain(BANNER);
  });

  it("control: a complete ledger shows its owner with no warning", async () => {
    const html = await renderRecord(full);
    expect(full.api_overlay.ledger_status).toBe("ok");
    expect(html).toContain(`owner ${full.api_overlay.owner}`);
    expect(html).not.toContain(BANNER);
  });

  it("control: a saved owner under a valid ledger shows no warning", async () => {
    const html = await renderRecord({ ...withOwner(full), case: { ...full.case, owner: "" }, api_overlay: { ...full.api_overlay, owner: SAVED } });
    expect(html).toContain(`owner ${SAVED}`);
    expect(html).not.toContain(BANNER);
  });

  it("control: no overlay and no review_ledger (older snapshot) falls back to the canonical owner with no warning", async () => {
    const { api_overlay: _o, review_ledger: _r, ...older } = full;
    void _o;
    void _r;
    const html = await renderRecord({ ...older, api_reviews: [], case: { ...full.case, owner: "Practitioner/ahs-mushrif-nurse-5" } });
    expect(html).toContain("owner Nurse 5");
    expect(html).not.toContain(BANNER);
  });
});
