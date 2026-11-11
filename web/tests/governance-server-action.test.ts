import { beforeEach, describe, expect, it, vi } from "vitest";
import { revalidatePath } from "next/cache";
import { appendGovernanceAction } from "@/lib/api/governance-actions";
import { postContestResolution, postUndo } from "@/lib/api/source";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/api/source", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/source")>();
  return { ...actual, postUndo: vi.fn(), postContest: vi.fn(), postContestResolution: vi.fn() };
});

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
});

describe("appendGovernanceAction", () => {
  it("uses the selected server-side credential and revalidates the original and resolving history", async () => {
    vi.mocked(postContestResolution).mockResolvedValue({
      ok: true,
      status: 201,
      response: { event: { hash: "resolution-hash" } },
    } as never);

    const result = await appendGovernanceAction({
      caseId: "case/1",
      target: "contest-hash",
      operation: "resolve",
      resolution: "upheld",
      reason: "Independent reviewer checked the evidence",
      credential: "reviewer_3",
    });
    expect(result).toEqual({ ok: true, hash: "resolution-hash", message: "Contest resolution appended. The canonical decision is unchanged." });
    expect(postContestResolution).toHaveBeenCalledWith(
      "case/1",
      { target: "contest-hash", resolution: "upheld", note: "Independent reviewer checked the evidence" },
      "reviewer_3",
    );
    expect(revalidatePath).toHaveBeenCalledWith("/app/patients/case%2F1/audit");
    expect(revalidatePath).toHaveBeenCalledWith("/app/review/case%2F1");
  });

  it("surfaces a same-actor refusal without claiming an event was appended", async () => {
    vi.mocked(postContestResolution).mockResolvedValue({ ok: false, status: 403, error: "actor must differ" });
    const result = await appendGovernanceAction({
      caseId: "case/1",
      target: "contest-hash",
      operation: "resolve",
      resolution: "upheld",
      reason: "Attempted self-resolution",
      credential: "reviewer_2",
    });
    expect(result).toEqual({ ok: false, error: "actor must differ" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("requires a reason at the server-action boundary", async () => {
    const result = await appendGovernanceAction({
      caseId: "case/1",
      target: "review-hash",
      operation: "undo",
      reason: " ",
      credential: "reviewer_2",
    });
    expect(result.ok).toBe(false);
    expect(postUndo).not.toHaveBeenCalled();
  });
});
