import { readFileSync } from "node:fs";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  getUndoProjection,
  postContest,
  postContestResolution,
  postUndo,
} from "@/lib/api/source";
import { Audit } from "@/lib/api/schema";

const meta = {
  api: "wasl-api/1",
  data_mode: "simulated",
  run: "r",
  policy: "p",
  reader_model: "m",
  loaded_at: "2026-11-10T00:00:00Z",
  note: "synthetic",
};
const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const originalHash = "a".repeat(64);
const contestHash = "b".repeat(64);
const resolutionHash = "c".repeat(64);
const projection = {
  case: "case/1",
  case_status: "compensated",
  canonical_state: "open",
  note: "review-ledger projection only",
  events: [
    { hash: originalHash, kind: "review", actor: "reviewer-one", action: "signoff_link", target: "", status: "compensated", at: "2026-11-10T09:00:00Z" },
    { hash: contestHash, kind: "contest", actor: "reviewer-two", action: "signoff_link", target: originalHash, status: "upheld", at: "2026-11-10T09:01:00Z" },
    { hash: resolutionHash, kind: "contest_resolution", actor: "reviewer-three", action: "signoff_link", target: contestHash, status: "applied", at: "2026-11-10T09:02:00Z" },
  ],
};
const compensationEvent = {
  kind: "contest_resolution",
  case: "case/1",
  target: contestHash,
  target_actor: "reviewer-one",
  target_action: "signoff_link",
  action: "signoff_link",
  action_head: contestHash,
  head_signed: "decision-signature",
  role: "reviewer",
  actor: "reviewer-three",
  note: "Independent review agrees the contest is correct",
  resolution: "upheld",
  at: "2026-11-10T09:02:00Z",
  mode: "demo",
  prev: contestHash,
  hash: resolutionHash,
};
const created = () => json(201, {
  meta,
  event: compensationEvent,
  canonical_reopened: false,
  auto_closure: false,
  canonical_state: "open",
  projection,
  note: "contest resolved in the review ledger only",
});

const TOKENS = {
  WASL_API_REVIEWER_TOKEN: "reviewer-one-token-123456",
  WASL_API_WEB_REVIEWER_TOKEN: "web-reviewer-token-123456",
  WASL_API_REVIEWER_2_TOKEN: "reviewer-two-token-123456",
  WASL_API_REVIEWER_3_TOKEN: "reviewer-three-token-12345",
  WASL_API_CLINICIAN_TOKEN: "clinician-token-1234567890",
  WASL_API_COORDINATOR_TOKEN: "coordinator-token-1234567890",
};
function live() {
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  for (const [key, value] of Object.entries(TOKENS)) vi.stubEnv(key, value);
}

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("governance API clients", () => {
  it("undo appends a compensation with a required reason and the selected role-bound credential", async () => {
    live();
    const fetchMock = vi.fn().mockResolvedValue(created());
    vi.stubGlobal("fetch", fetchMock);

    const result = await postUndo("case/1", { target: originalHash, note: "The identity match was incorrect" }, "reviewer_2");
    expect(result.ok).toBe(true);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8088/api/cases/case%2F1/undo");
    expect(JSON.parse(init.body)).toEqual({ target: originalHash, contest: false, note: "The identity match was incorrect" });
    expect(new Headers(init.headers).get("authorization")).toBe(`Bearer ${TOKENS.WASL_API_REVIEWER_2_TOKEN}`);
    expect(new Headers(init.headers).has("x-wasl-actor")).toBe(false);
    expect(new Headers(init.headers).has("x-wasl-role")).toBe(false);
  });

  it("contest appends a visible contest event rather than changing the original review", async () => {
    live();
    const fetchMock = vi.fn().mockResolvedValue(created());
    vi.stubGlobal("fetch", fetchMock);

    const result = await postContest("case/1", { target: originalHash, note: "A second reviewer should check this link" }, "reviewer_2");
    expect(result.ok).toBe(true);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      target: originalHash,
      contest: true,
      note: "A second reviewer should check this link",
    });
  });

  it("returns the API's same-actor resolution refusal", async () => {
    live();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(403, { meta, error: "actor must differ from the actor(s) of the event being undone, contested or resolved" })));

    const result = await postContestResolution(
      "case/1",
      { target: contestHash, resolution: "upheld", note: "Attempted self-resolution" },
      "reviewer_2",
    );
    expect(result).toEqual({ ok: false, status: 403, error: "actor must differ from the actor(s) of the event being undone, contested or resolved" });
  });

  it("independent resolution succeeds and preserves the original event in the verified chain", async () => {
    live();
    const fetchMock = vi.fn().mockResolvedValue(created());
    vi.stubGlobal("fetch", fetchMock);

    const result = await postContestResolution(
      "case/1",
      { target: contestHash, resolution: "upheld", note: "Independent review agrees the contest is correct" },
      "reviewer_3",
    );
    expect(result.ok && result.response.event.hash).toBe(resolutionHash);
    expect(result.ok && result.response.projection.events[0]).toMatchObject({ hash: originalHash, status: "compensated" });
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      target: contestHash,
      resolution: "upheld",
      note: "Independent review agrees the contest is correct",
    });

    const audit = Audit.parse(JSON.parse(readFileSync(path.join(__dirname, "fixtures", "ledger_audit.json"), "utf8")));
    expect(audit.chain_verified).toBe(true);
    expect(audit.history.filter((event) => event.ledger === "review")).toHaveLength(3);
    for (const event of audit.history) {
      if (event.ledger === "compensation") expect(audit.history.some((original) => original.hash === event.target)).toBe(true);
    }
  });

  it("rejects an empty reason before any request", async () => {
    live();
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await postUndo("case/1", { target: originalHash, note: "   " }, "reviewer_2");
    expect(result.ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("reads the live status projection and refuses fixture-only persistence", async () => {
    live();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(200, { meta, ...projection })));
    await expect(getUndoProjection("case/1")).resolves.toMatchObject({ case_status: "compensated", canonical_state: "open" });

    vi.stubEnv("WASL_API_URL", "");
    await expect(postContest("case/1", { target: originalHash, note: "Contest this" }, "reviewer_2")).rejects.toThrow(
      "fixtures mode cannot append governance events",
    );
  });
});
