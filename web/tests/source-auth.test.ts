import { afterEach, describe, expect, it, vi } from "vitest";
import { getAudit, getHealthz, getOperations, postReview } from "@/lib/api/source";
import { loadSample } from "./samples";

const health = {
  status: "ok",
  cases: 0,
  dependencies: { materialised_run: "loaded", nats: "not_required_for_read_api", reader: "cached", ingest_adapter: "fhir" },
  meta: { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-09T00:00:00Z", note: "synthetic" },
};

function response(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
}

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("live API authentication", () => {
  it("uses separate server-side reviewer and auditor bearer tokens", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "assigned-reader-secret-1234567890");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "reviewer-secret-1234567890");
    vi.stubEnv("WASL_API_AUDITOR_TOKEN", "auditor-secret-1234567890");
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response(200, health))
      .mockResolvedValueOnce(response(404, { error: "missing" }));
    vi.stubGlobal("fetch", fetchMock);

    await getHealthz();
    await getAudit("missing");
    expect(fetchMock.mock.calls[0][1].headers.get("authorization")).toBe("Bearer reviewer-secret-1234567890");
    expect(fetchMock.mock.calls[1][1].headers.get("authorization")).toBe("Bearer auditor-secret-1234567890");
  });

  it("gets operations with the shared web-reviewer bearer, no-store and an abort signal", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "web-reviewer-secret-1234567890");
    const fetchMock = vi.fn().mockResolvedValue(response(200, loadSample("operations.json")));
    vi.stubGlobal("fetch", fetchMock);

    const operations = await getOperations();
    expect(operations.schema).toBe("wasl-operations/1");
    expect(fetchMock.mock.calls[0][0]).toBe("http://127.0.0.1:8088/api/operations");
    const init = fetchMock.mock.calls[0][1];
    expect(init.headers.get("authorization")).toBe("Bearer web-reviewer-secret-1234567890");
    expect(init.cache).toBe("no-store");
    expect(init.signal).toBeInstanceOf(AbortSignal);
  });

  it("keeps the operations request alive beyond the backend's three-second probe budget", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "web-reviewer-secret-1234567890");
    const fetchMock = vi.fn().mockImplementation((_url: string, init: RequestInit) => new Promise((resolve, reject) => {
      const timer = setTimeout(() => resolve(response(200, loadSample("operations.json"))), 3250);
      init.signal?.addEventListener("abort", () => {
        clearTimeout(timer);
        reject(init.signal?.reason);
      });
    }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getOperations()).resolves.toMatchObject({ schema: "wasl-operations/1" });
  });

  it("reads the published operations snapshot in fixtures mode", async () => {
    vi.stubEnv("WASL_API_URL", "");
    vi.stubGlobal("fetch", vi.fn());
    const operations = await getOperations();
    expect(operations.backup).toMatchObject({ status: "unmeasured", reason: "backup_manifest_not_configured" });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("binds writes to a role token and does not send self-asserted identity headers", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
    const fetchMock = vi.fn().mockResolvedValue(response(400, { error: "invalid synthetic review", meta: health.meta }));
    vi.stubGlobal("fetch", fetchMock);

    const result = await postReview("case-1", { item: "i1", decision: "same", note: "" }, { role: "reviewer", actor: "attacker-selected" });
    expect(result).toEqual({ ok: false, status: 400, error: "invalid synthetic review" });
    const headers = fetchMock.mock.calls[0][1].headers;
    expect(headers.get("authorization")).toBe("Bearer reviewer-secret-1234567890");
    expect(headers.has("x-wasl-role")).toBe(false);
    expect(headers.has("x-wasl-actor")).toBe(false);
  });
});

describe("raw reviewer-token helpers", () => {
  it("send only the supplied bearer on GET and POST and leave role-token helpers unchanged", async () => {
    const { getCaseWithToken, postReviewWithToken } = await import("@/lib/api/source");
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    const fetchMock = vi.fn().mockResolvedValue(response(409, { error: "stale", current_head: "h", meta: health.meta }));
    vi.stubGlobal("fetch", fetchMock);
    const post = await postReviewWithToken("c1", { item: "i", decision: "same", note: "", read: "r1", expected_head: "" }, "raw-token-1234567890");
    expect(post).toEqual({ ok: false, status: 409, error: "stale", currentHead: "h" });
    fetchMock.mockResolvedValue(response(403, { error: "no", meta: health.meta }));
    expect(await getCaseWithToken("c1", "raw-token-1234567890")).toEqual({ ok: false, status: 403 });
    for (const call of fetchMock.mock.calls) {
      expect(call[1].headers.get("authorization")).toBe("Bearer raw-token-1234567890");
      expect(call[1].headers.has("x-wasl-role")).toBe(false);
      expect(call[1].headers.has("x-wasl-actor")).toBe(false);
    }
  });
});
