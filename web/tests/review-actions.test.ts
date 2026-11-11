import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { signReview } from "@/app/app/review/[id]/actions";
import type { Signature } from "@/components/review/decision";

const { revalidateMock } = vi.hoisted(() => ({ revalidateMock: vi.fn() }));
vi.mock("next/cache", () => ({ revalidatePath: revalidateMock }));

const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

function created(caseId: string, signature: Signature, hash = "hash-1") {
  const role = signature.decision === "same" || signature.decision === "different" ? "reviewer" : "clinician";
  return json(201, {
    meta,
    review: {
      case: caseId,
      item: signature.item,
      decision: signature.decision,
      note: "",
      role,
      actor: "bound-api-actor",
      action: role === "reviewer" ? "sign_link" : "sign_evidence",
      at: "2026-11-11T08:00:00Z",
      mode: "demo",
      prev: "genesis",
      hash,
    },
  });
}

beforeEach(() => {
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
  vi.stubEnv("WASL_API_CLINICIAN_TOKEN", "clinician-secret-1234567890");
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  revalidateMock.mockReset();
});

describe("signReview programme evidence", () => {
  it.each([
    ["crc/evidence-only", { item: "rv-crc-evidence", decision: "adequate" }],
    ["lung_nodule", { item: "rv-lung", decision: "pet_low" }],
    ["diabetic_retinopathy", { item: "rv-dr", decision: "R3AM1/urgent" }],
    ["breast", { item: "rv-breast", decision: "B4" }],
    ["lung_ldct", { item: "rv-ldct", decision: "wk_benign" }],
    ["ifhas", { item: "rv-ifhas", decision: "bp=managed;glyc=not_confirmed" }],
  ] as const)("appends a 201 for %s and refreshes the accurate history", async (caseId, signature) => {
    const fetchMock = vi.fn().mockResolvedValue(created(caseId, signature));
    vi.stubGlobal("fetch", fetchMock);

    const result = await signReview(caseId, [signature]);
    expect(result).toEqual({
      saved: [{ item: signature.item, hash: "hash-1", role: "clinician", actor: "bound-api-actor", at: "2026-11-11T08:00:00Z" }],
    });
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe(`http://127.0.0.1:8088/api/cases/${encodeURIComponent(caseId)}/review`);
    expect(new Headers(init.headers).get("authorization")).toBe("Bearer clinician-secret-1234567890");
    expect(JSON.parse(init.body)).toEqual({ ...signature, note: "" });
    expect(revalidateMock).toHaveBeenCalledTimes(4);
    expect(revalidateMock).toHaveBeenCalledWith("/app/patients/[id]", "page");
    expect(revalidateMock).toHaveBeenCalledWith("/app/patients/[id]/audit", "page");
    expect(revalidateMock).toHaveBeenCalledWith(`/app/review/${encodeURIComponent(caseId)}`);
    expect(revalidateMock).toHaveBeenCalledWith("/app/review");
  });

  it("appends a link-only CRC signature as reviewer", async () => {
    const signature: Signature = { item: "rv-link-only", decision: "same" };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(created("crc/link-only", signature)));
    const result = await signReview("crc/link-only", [signature]);
    expect(result.saved).toEqual([{ item: signature.item, hash: "hash-1", role: "reviewer", actor: "bound-api-actor", at: "2026-11-11T08:00:00Z" }]);
  });

  it("returns the API's unsupported-decision 400 without refreshing history", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json(400, { meta, error: "decision is not supported for lung_nodule" }));
    vi.stubGlobal("fetch", fetchMock);
    const signature = { item: "rv-lung", decision: "B4" } as Signature;
    const result = await signReview("lung_nodule", [signature]);
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(result.saved).toEqual([]);
    expect(result.failed).toEqual({ signature, failure: { status: 400, error: "decision is not supported for lung_nodule" } });
    expect(revalidateMock).not.toHaveBeenCalled();
  });

  it("rejects a malformed evidence decision before any request", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await signReview("dr", [{ item: "rv-dr", decision: "R9M0" } as Signature]);
    expect(result.failed?.failure.status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
