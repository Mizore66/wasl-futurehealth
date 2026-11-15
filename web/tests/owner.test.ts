import { afterEach, describe, expect, it, vi } from "vitest";
import { CaseList } from "@/lib/api/schema";
import { effectiveOwner } from "@/lib/api/owner";
import { getCases, matches } from "@/lib/api/source";
import { facets } from "@/components/patients/facets";
import { toCaseQuery } from "@/components/worklist/filters";
import { ownerLabel } from "@/components/worklist/model";
import { loadSample } from "./samples";

const base = CaseList.parse(loadSample("cases_crc_page1.json")).cases[0];
const overlay = (owner: string) => ({ head: "h", actions: 1, owner, last_action_at: "", ledger_status: "ok" as const, no_answer_streak: 0 });

describe("effectiveOwner (record header)", () => {
  it("names the saved owner on a case that had none", () => {
    const c = { ...base, owner: "" };
    expect(ownerLabel(effectiveOwner(c, overlay("Hessa Al Ali")))).toBe("Hessa Al Ali");
  });

  it("prefers the saved owner over a different canonical owner", () => {
    const c = { ...base, owner: "Practitioner/ahs-mushrif-nurse-5" };
    expect(effectiveOwner(c, overlay("Dr Al Kaabi"))).toBe("Dr Al Kaabi");
  });

  it("falls back to the canonical owner without an overlay or without an assigned owner", () => {
    const c = { ...base, owner: "Practitioner/ahs-mushrif-nurse-5" };
    expect(effectiveOwner(c, undefined)).toBe(c.owner);
    expect(effectiveOwner(c, overlay(""))).toBe(c.owner);
  });
});

describe("Patients filters and facets follow the saved owner", () => {
  const unowned = { ...base, id: "a", owner: "", api_overlay: overlay("Hessa Al Ali") };
  const reassigned = { ...base, id: "b", owner: "Practitioner/ahs-mushrif-nurse-5", api_overlay: overlay("Dr Al Kaabi") };
  const plain = { ...base, id: "c", owner: "Practitioner/ahs-mushrif-nurse-5", api_overlay: undefined };
  const still = { ...base, id: "d", owner: "", api_overlay: overlay("") };
  const all = [unowned, reassigned, plain, still];

  it("matches by the effective owner", () => {
    const ids = (q: Parameters<typeof matches>[1]) => all.filter((c) => matches(c, q)).map((c) => c.id);
    expect(ids(toCaseQuery({ owner: "none" }))).toEqual(["d"]);
    expect(ids(toCaseQuery({ owner: "Hessa Al Ali" }))).toEqual(["a"]);
    expect(ids(toCaseQuery({ owner: "Practitioner/ahs-mushrif-nurse-5" }))).toEqual(["c"]);
  });

  it("counts owner facets by the effective owner", () => {
    const o = facets(all, {}).owner;
    expect(Object.fromEntries(o.map((x) => [x.value, x.count]))).toEqual({
      "Hessa Al Ali": 1,
      "Dr Al Kaabi": 1,
      "Practitioner/ahs-mushrif-nurse-5": 1,
      none: 1,
    });
  });
});

describe("live getCases", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("asks the API for unowned cases so filtering happens before pagination", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "web-reviewer-secret-1234567890");
    const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ meta, cases: [], total: 0, limit: 25, offset: 50 }), { status: 200, headers: { "content-type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const list = await getCases({ unowned: true, limit: 25, offset: 50 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const url = new URL((fetchMock.mock.calls[0] as unknown as [string])[0]);
    expect(Object.fromEntries(url.searchParams)).toEqual({ unowned: "true", limit: "25", offset: "50" });
    expect([list.limit, list.offset]).toEqual([25, 50]);
  });
});
