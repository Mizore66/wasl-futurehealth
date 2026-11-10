import { afterEach, describe, expect, it, vi } from "vitest";
import { CaseList } from "@/lib/api/schema";
import { getAudit, getDashboard, postAction, postReview } from "@/lib/api/source";
import { HANDOFF_OWNER, workItem } from "@/components/worklist/model";
import { loadSample } from "./samples";

const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

const TOKENS = {
  WASL_API_REVIEWER_TOKEN: "reviewer-secret-1234567890",
  WASL_API_CLINICIAN_TOKEN: "clinician-secret-1234567890",
  WASL_API_AUDITOR_TOKEN: "auditor-secret-1234567890",
  WASL_API_COORDINATOR_TOKEN: "coordinator-secret-1234567890",
};
const live = (url = "http://127.0.0.1:8088") => {
  vi.stubEnv("WASL_API_URL", url);
  for (const [k, v] of Object.entries(TOKENS)) vi.stubEnv(k, v);
};
/** The overlay a 201 carries: the API returns the full form, with the actor it bound to the coordinator token. */
const created = (actor = "hosted-coordinator") =>
  json(201, {
    meta,
    event: { type: "coordinator_action", kind: "contact_outcome", hash: "h2", at: "2026-11-10T10:00:00Z", actor, role: "coordinator" },
    api_overlay: { head: "h2", actions: 1, owner: "", last_action_at: "", ledger_status: "ok", contact_outcomes: [], notes: [], canonical_owner: "" },
  });

describe("live API per-role tokens", () => {
  it("reads with the reviewer token and never sends a legacy shared token", async () => {
    live("https://api.example.test/");
    vi.stubEnv("WASL_API_TOKEN", "legacy-shared-token-0123456789");
    const fetchMock = vi.fn().mockResolvedValue(json(500, {}));
    vi.stubGlobal("fetch", fetchMock);
    await expect(getDashboard()).rejects.toThrow("HTTP 500");
    expect(fetchMock.mock.calls[0][0]).toBe("https://api.example.test/api/dashboard");
    expect(new Headers(fetchMock.mock.calls[0][1].headers).get("authorization")).toBe(`Bearer ${TOKENS.WASL_API_REVIEWER_TOKEN}`);
  });

  it("refuses to make a live request without the role's token", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(postAction("c", { action: "note", text: "x" }, "h")).rejects.toThrow("WASL_API_COORDINATOR_TOKEN is required");
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("postAction", () => {
  it("posts with the server-side coordinator token, sends no identity headers, and returns the API's actor", async () => {
    live();
    const fetchMock = vi.fn().mockResolvedValue(created());
    vi.stubGlobal("fetch", fetchMock);

    const res = await postAction("case/1", { action: "contact_outcome", outcome: "reached", channel: "phone" }, "h1");
    expect(res.ok && [res.response.api_overlay.head, res.response.event.actor]).toEqual(["h2", "hosted-coordinator"]);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8088/api/cases/case%2F1/actions");
    const headers = new Headers(init.headers);
    expect(headers.get("authorization")).toBe(`Bearer ${TOKENS.WASL_API_COORDINATOR_TOKEN}`);
    expect(headers.has("x-wasl-role")).toBe(false);
    expect(headers.has("x-wasl-actor")).toBe(false);
    expect(JSON.parse(init.body)).toEqual({ action: "contact_outcome", outcome: "reached", channel: "phone", expected_head: "h1" });
  });

  it("each role's call carries only that role's token", async () => {
    live();
    const fetchMock = vi.fn().mockResolvedValueOnce(created()).mockResolvedValueOnce(json(400, { meta, error: "bad" })).mockResolvedValueOnce(json(404, {}));
    vi.stubGlobal("fetch", fetchMock);
    await postAction("c", { action: "note", text: "x" }, "h");
    await postReview("c", { item: "i", decision: "adequate", note: "" }, { role: "clinician", actor: "ignored" });
    await getAudit("c");
    const auth = fetchMock.mock.calls.map(([, init]) => new Headers(init.headers).get("authorization"));
    expect(auth).toEqual([
      `Bearer ${TOKENS.WASL_API_COORDINATOR_TOKEN}`,
      `Bearer ${TOKENS.WASL_API_CLINICIAN_TOKEN}`,
      `Bearer ${TOKENS.WASL_API_AUDITOR_TOKEN}`,
    ]);
  });

  it("returns a 403 from a token whose role may not coordinate instead of throwing", async () => {
    live();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(403, { meta, error: "rbac: role reviewer may not coordinate_case" })));
    const res = await postAction("c", { action: "assign_owner", owner: "x" }, "h");
    expect(res).toEqual({ ok: false, status: 403, error: "rbac: role reviewer may not coordinate_case", conflict: undefined, currentHead: undefined });
  });

  it("returns a 409 consent refusal instead of throwing", async () => {
    live();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(409, { meta, error: "contact permission not granted", conflict: "consent" })));
    const res = await postAction("c", { action: "contact_outcome", outcome: "no_answer", channel: "phone" }, "h");
    expect(res).toEqual({ ok: false, status: 409, error: "contact permission not granted", conflict: "consent", currentHead: undefined });
  });

  it("rejects a malformed action before any request", async () => {
    live();
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    // @ts-expect-error: not an API outcome
    const res = await postAction("c", { action: "contact_outcome", outcome: "texted", channel: "phone" }, "h");
    expect(res.ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("refuses in fixtures mode", async () => {
    vi.stubEnv("WASL_API_URL", "");
    await expect(postAction("c", { action: "note", text: "x" }, "h")).rejects.toThrow("needs WASL_API_URL");
  });
});

describe("workItem with a ledger overlay", () => {
  const c = CaseList.parse(loadSample("cases_crc_page1.json")).cases.find((x) => x.id.endsWith("sr-1003"))!;
  const NOW = Date.parse("2026-11-10T00:00:00Z");

  it("has no head or ledger fields without an overlay", () => {
    const it = workItem(c, 120, false, NOW);
    expect(it.head).toBeUndefined();
    expect(it.view).toBe("call");
  });

  it("takes the head, a running snooze and a hand-off owner from the overlay", () => {
    const overlay = { head: "h9", actions: 3, owner: HANDOFF_OWNER, last_action_at: "", ledger_status: "ok" as const, no_answer_streak: 0, snooze: { until: "2026-11-11T00:00:00Z", reason: "Call back Tue" } };
    const it = workItem({ ...c, api_overlay: overlay }, 120, false, NOW);
    expect([it.head, it.ledgerActions, it.snoozed, it.status, it.owner, it.view]).toEqual(["h9", 3, true, "Call back Tue", HANDOFF_OWNER, "waiting"]);
  });

  it("ignores a snooze that has run out", () => {
    const overlay = { head: "h9", actions: 1, owner: "", last_action_at: "", ledger_status: "ok" as const, no_answer_streak: 0, snooze: { until: "2026-11-01T00:00:00Z" } };
    expect(workItem({ ...c, api_overlay: overlay }, 120, false, NOW).snoozed).toBe(false);
  });
});
