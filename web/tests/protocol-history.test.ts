import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Audit, ReviewState } from "@/lib/api/schema";

const m = vi.hoisted(() => ({ getReviewCase: vi.fn(), getAudit: vi.fn(), getUndoProjection: vi.fn(), getCase: vi.fn() }));
vi.mock("@/lib/api/reviewer-session", () => ({ getReviewCase: m.getReviewCase }));
vi.mock("@/lib/api/source", () => ({ apiMode: () => "live", getAudit: m.getAudit, getUndoProjection: m.getUndoProjection, getCase: m.getCase }));

import { gatedHistory } from "@/lib/api/protocol-history";

const state = (s: Partial<ReviewState>): ReviewState =>
  ({ item: "rv-1", question: "link", state: "awaiting_r1", head: "", next_read: "r1", may_submit: true, reads: [], disagreement: false, canonical_unchanged: true, ...s }) as ReviewState;
const awaitingR1 = state({});
const concealed = state({ state: "awaiting_r2", next_read: "r2", reads: [{ read: "r1", submitted: true, concealed: true } as never, { read: "r2", submitted: false }] });
const resolved = state({ state: "resolved_agreed", next_read: null, may_submit: false, final_decision: "same", reads: [{ read: "r1", submitted: true, decision: "same", hash: "a" }, { read: "r2", submitted: true, decision: "same", hash: "b" }] });
const viewer = (states: ReviewState[]) => ({ authenticated: true as const, detail: { api_review_states: states } }) as never;
const signedOut = { authenticated: false as const, detail: null };

const protocolEvent = { case: "c", item: "rv-1", decision: "same", role: "reviewer", action: "signoff_link", at: "t", mode: "api", prev: "p", hash: "h1", protocol: "wasl-independent-review/1", read: "r1", question: "link", ledger: "review" };
const legacyEvent = { ...protocolEvent, protocol: undefined, read: undefined, question: undefined };
const audit = (history: unknown[]) => ({ case: "c", history }) as unknown as Audit;

beforeEach(() => {
  vi.resetAllMocks();
  m.getUndoProjection.mockResolvedValue({ events: [] });
});

describe("gatedHistory", () => {
  it("does not request history while an initial read is concealed (review page)", async () => {
    const out = await gatedHistory("c", { before: viewer([concealed]) });
    expect(out).toEqual({ status: "concealed", audit: null, projection: null, signedIn: true });
    expect(m.getAudit).not.toHaveBeenCalled();
  });

  it("race: r1 lands while the audit fetch is in flight, so history and projection are dropped", async () => {
    let release!: (a: Audit) => void;
    m.getAudit.mockReturnValue(new Promise<Audit>((r) => (release = r)));
    m.getReviewCase.mockResolvedValue(viewer([concealed])); // the re-read after the fetch
    const pending = gatedHistory("c", { before: viewer([awaitingR1]) });
    await Promise.resolve();
    release(audit([protocolEvent]));
    const out = await pending;
    expect(out.status).toBe("concealed");
    expect(out.audit?.history).toEqual([]);
    expect(out.projection).toBeNull();
    expect(JSON.stringify(out)).not.toContain("h1");
  });

  it("returns history when the re-read is still unconcealed", async () => {
    m.getAudit.mockResolvedValue(audit([protocolEvent]));
    m.getReviewCase.mockResolvedValue(viewer([resolved]));
    const out = await gatedHistory("c", { before: viewer([resolved]) });
    expect(out.status).toBe("ok");
    expect(out.audit?.history).toHaveLength(1);
    expect(out.projection).not.toBeNull();
  });
});

describe("gatedHistory for the audit route", () => {
  it("without a reviewer cookie, protocol events are withheld", async () => {
    m.getAudit.mockResolvedValue(audit([protocolEvent]));
    m.getReviewCase.mockResolvedValue(signedOut);
    const out = await gatedHistory("c", { needAudit: true });
    expect(out.status).toBe("signin");
    expect(out.audit?.history).toEqual([]);
    expect(out.projection).toBeNull();
  });

  it("with any concealed state, protocol events are withheld even for a signed-in reviewer", async () => {
    m.getAudit.mockResolvedValue(audit([protocolEvent]));
    m.getReviewCase.mockResolvedValue(viewer([concealed]));
    const out = await gatedHistory("c", { needAudit: true });
    expect(out.status).toBe("concealed");
    expect(out.audit?.history).toEqual([]);
  });

  it("legacy history is unchanged with no cookie, and reports signed out so governance shows a sign-in prompt", async () => {
    m.getAudit.mockResolvedValue(audit([legacyEvent]));
    m.getReviewCase.mockResolvedValue(signedOut);
    const out = await gatedHistory("c", { needAudit: true });
    expect(out.status).toBe("ok");
    expect(out.audit?.history).toHaveLength(1);
    expect(out.signedIn).toBe(false);
  });

  it("reports signed in only from the re-read after the fetch (a token rejected meanwhile is signed out)", async () => {
    m.getAudit.mockResolvedValue(audit([legacyEvent]));
    m.getReviewCase.mockResolvedValueOnce(viewer([])).mockResolvedValueOnce(signedOut);
    expect((await gatedHistory("c", { needAudit: true })).signedIn).toBe(false);
    m.getReviewCase.mockResolvedValue(viewer([]));
    expect((await gatedHistory("c", { needAudit: true })).signedIn).toBe(true);
  });

  it("closes the race on the audit route too", async () => {
    m.getAudit.mockResolvedValue(audit([protocolEvent]));
    m.getReviewCase.mockResolvedValueOnce(viewer([awaitingR1])).mockResolvedValueOnce(viewer([concealed]));
    const out = await gatedHistory("c", { needAudit: true });
    expect(out.status).toBe("concealed");
    expect(out.audit?.history).toEqual([]);
  });
});

describe("route wiring", () => {
  it("both routes go through gatedHistory and neither calls getAudit directly", async () => {
    const { readFileSync } = await import("node:fs");
    const path = await import("node:path");
    for (const rel of [["app", "app", "review", "[id]", "page.tsx"], ["app", "app", "patients", "[id]", "audit", "page.tsx"]]) {
      const src = readFileSync(path.join(__dirname, "..", ...rel), "utf8");
      expect(src).toContain("gatedHistory");
      expect(src).not.toMatch(/\bgetAudit\b|\bgetUndoProjection\b/);
    }
  });
});
