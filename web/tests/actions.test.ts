import { afterEach, describe, expect, it, vi } from "vitest";
import { CaseList } from "@/lib/api/schema";
import { getDashboard, postAction } from "@/lib/api/source";
import { HANDOFF_OWNER, workItem } from "@/components/worklist/model";
import { loadSample } from "./samples";

const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("live API token", () => {
  it("sends WASL_API_TOKEN as a bearer token when set, and nothing when unset", async () => {
    vi.stubEnv("WASL_API_URL", "https://api.example.test/");
    const fetchMock = vi.fn().mockResolvedValue(json(500, {}));
    vi.stubGlobal("fetch", fetchMock);

    vi.stubEnv("WASL_API_TOKEN", "");
    await expect(getDashboard()).rejects.toThrow("HTTP 500");
    expect(fetchMock.mock.calls[0][0]).toBe("https://api.example.test/api/dashboard");
    expect(new Headers(fetchMock.mock.calls[0][1].headers).has("authorization")).toBe(false);

    vi.stubEnv("WASL_API_TOKEN", "  tok-0123456789abcdef01234567 ");
    await expect(getDashboard()).rejects.toThrow("HTTP 500");
    expect(new Headers(fetchMock.mock.calls[1][1].headers).get("authorization")).toBe("Bearer tok-0123456789abcdef01234567");
  });
});

describe("postAction", () => {
  it("posts a coordinator action with the expected head and returns the new head", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_TOKEN", "tok-0123456789abcdef01234567");
    const fetchMock = vi.fn().mockResolvedValue(
      json(201, { meta, event: { hash: "h2", at: "2026-11-10T10:00:00Z" }, api_overlay: { head: "h2", actions: 1, owner: "", last_action_at: "", ledger_status: "ok" } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await postAction("case/1", { action: "contact_outcome", outcome: "reached", channel: "phone" }, "h1", "Hessa Al Ali");
    expect(res.ok && res.response.api_overlay.head).toBe("h2");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8088/api/cases/case%2F1/actions");
    const headers = new Headers(init.headers);
    expect(headers.get("x-wasl-role")).toBe("coordinator");
    expect(headers.get("x-wasl-actor")).toBe("Hessa Al Ali");
    expect(headers.get("authorization")).toBe("Bearer tok-0123456789abcdef01234567");
    expect(JSON.parse(init.body)).toEqual({ action: "contact_outcome", outcome: "reached", channel: "phone", expected_head: "h1" });
  });

  it("returns a 409 consent refusal instead of throwing", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(409, { meta, error: "contact permission not granted", conflict: "consent" })));
    const res = await postAction("c", { action: "contact_outcome", outcome: "no_answer", channel: "phone" }, "h", "a");
    expect(res).toEqual({ ok: false, status: 409, error: "contact permission not granted", conflict: "consent", currentHead: undefined });
  });

  it("rejects a malformed action before any request", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    // @ts-expect-error: not an API outcome
    const res = await postAction("c", { action: "contact_outcome", outcome: "texted", channel: "phone" }, "h", "a");
    expect(res.ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("refuses in fixtures mode", async () => {
    vi.stubEnv("WASL_API_URL", "");
    await expect(postAction("c", { action: "note", text: "x" }, "h", "a")).rejects.toThrow("needs WASL_API_URL");
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
    const overlay = { head: "h9", actions: 3, owner: HANDOFF_OWNER, last_action_at: "", ledger_status: "ok" as const, snooze: { until: "2026-11-11T00:00:00Z", reason: "Call back Tue" } };
    const it = workItem({ ...c, api_overlay: overlay }, 120, false, NOW);
    expect([it.head, it.ledgerActions, it.snoozed, it.status, it.owner, it.view]).toEqual(["h9", 3, true, "Call back Tue", HANDOFF_OWNER, "waiting"]);
  });

  it("ignores a snooze that has run out", () => {
    const overlay = { head: "h9", actions: 1, owner: "", last_action_at: "", ledger_status: "ok" as const, snooze: { until: "2026-11-01T00:00:00Z" } };
    expect(workItem({ ...c, api_overlay: overlay }, 120, false, NOW).snoozed).toBe(false);
  });
});
