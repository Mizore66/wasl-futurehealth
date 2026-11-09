import { afterEach, describe, expect, it, vi } from "vitest";
import { getAudit, getHealthz, postReview } from "@/lib/api/source";

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
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
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

  it("binds writes to a role token and does not send self-asserted identity headers", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
    const fetchMock = vi.fn().mockResolvedValue(response(400, { error: "invalid synthetic review" }));
    vi.stubGlobal("fetch", fetchMock);

    const result = await postReview("case-1", { item: "i1", decision: "same", note: "" }, { role: "reviewer", actor: "attacker-selected" });
    expect(result).toEqual({ ok: false, status: 400, error: "invalid synthetic review" });
    const headers = fetchMock.mock.calls[0][1].headers;
    expect(headers.get("authorization")).toBe("Bearer reviewer-secret-1234567890");
    expect(headers.has("x-wasl-role")).toBe(false);
    expect(headers.has("x-wasl-actor")).toBe(false);
  });
});
