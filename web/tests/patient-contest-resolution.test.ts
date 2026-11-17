import { renderToStaticMarkup } from "react-dom/server";
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { revalidatePath } from "next/cache";
import { resolvePatientContest } from "@/lib/api/patient-contest-actions";
import { reviewerToken } from "@/lib/api/reviewer-session";
import { postPatientContestResolutionWithToken } from "@/lib/api/source";
import { PatientRequestsCard, requestState } from "@/components/case/patient-journey-card";
import { PatientContestControls } from "@/components/case/patient-contest-controls";
import { patientContestReview, type ContestSession } from "@/components/case/patient-contest-review";
import { CaseDetail } from "@/lib/api/schema";
import { loadSample } from "./samples";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn(), refresh: vi.fn() }));
vi.mock("@/lib/api/reviewer-session", () => ({ reviewerToken: vi.fn(), signInReviewer: vi.fn(), signOutReviewer: vi.fn() }));
vi.mock("@/lib/api/source", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/source")>();
  return { ...actual, postPatientContestResolutionWithToken: vi.fn(actual.postPatientContestResolutionWithToken) };
});

/** The signed-in senior clinician's own token, held server-side in the reviewer-session cookie. */
const SESSION = "signed-in-senior-clinician-token";
const fetchMock = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  /* Server-held credentials exist; none of them may be used to resolve a patient contest. */
  vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "server-web-reviewer-token-1234567890");
  vi.stubEnv("WASL_API_CLINICIAN_TOKEN", "server-clinician-token-1234567890");
  vi.stubEnv("WASL_API_CLINICIAN_3_TOKEN", "server-clinician-three-token-1234567890");
  vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "server-coordinator-token-1234567890");
  vi.stubGlobal("fetch", fetchMock);
  vi.mocked(reviewerToken).mockResolvedValue(SESSION);
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

describe("resolvePatientContest acts only as the signed-in senior clinician (#167)", () => {
  it("posts the receipt and chosen outcome with the session token and revalidates the case", async () => {
    fetchMock.mockResolvedValue(json(201, { event: { hash: "resolution-hash-0123456789" }, note: "resolution appended" }));
    const result = await resolvePatientContest({ caseId: "LOOP/1", contest: "receipt-abc", outcome: "decision_upheld", reason: "Matched report verified independently (synthetic)." });
    expect(result).toMatchObject({ ok: true, hash: "resolution-hash-0123456789" });
    expect(result.ok && result.message).toContain("signed-in credential");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8088/api/cases/LOOP%2F1/patient-contest");
    expect(new Headers(init.headers).get("authorization")).toBe(`Bearer ${SESSION}`);
    expect(JSON.parse(init.body)).toEqual({ contest: "receipt-abc", outcome: "decision_upheld", reason: "Matched report verified independently (synthetic)." });
    expect(revalidatePath).toHaveBeenCalledWith("/app/patients/LOOP%2F1");
    expect(revalidatePath).toHaveBeenCalledWith("/app/patients");
  });

  it.each([
    ["credential", { credential: "clinician_3" }],
    ["token", { token: "someone-elses-token-123456" }],
    ["role", { role: "senior_clinician" }],
    ["actor", { actor: "clinician-three" }],
  ])("refuses a client-supplied %s and sends nothing", async (_name, identity) => {
    const result = await resolvePatientContest({ caseId: "c1", contest: "r1", outcome: "decision_revised", reason: "Matched report verified independently (synthetic).", ...identity });
    expect(result.ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it.each([undefined, "", "upheld", "closed"])("has no default outcome: %j is refused before any request", async (outcome) => {
    const result = await resolvePatientContest({ caseId: "c1", contest: "r1", outcome, reason: "Matched report verified independently (synthetic)." });
    expect(result.ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it.each([undefined, "", "   "])("requires the reviewer's reason: %j is refused before any request", async (reason) => {
    const result = await resolvePatientContest({ caseId: "c1", contest: "r1", outcome: "decision_upheld", reason });
    expect(result.ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("without a session sends nothing and asks for a sign-in, although server tokens are configured", async () => {
    vi.mocked(reviewerToken).mockResolvedValue(undefined);
    const result = await resolvePatientContest({ caseId: "c1", contest: "r1", outcome: "decision_upheld", reason: "Matched report verified independently (synthetic)." });
    expect(result).toMatchObject({ ok: false, signIn: true });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("surfaces the API's refusal of an original reviewer without claiming a resolution", async () => {
    fetchMock.mockResolvedValue(json(403, { error: "the original decision reviewer may not resolve this contest" }));
    const result = await resolvePatientContest({ caseId: "c1", contest: "r1", outcome: "decision_upheld", reason: "Matched report verified independently (synthetic)." });
    expect(result).toEqual({ ok: false, error: "the original decision reviewer may not resolve this contest" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("treats a 401 as signed out", async () => {
    fetchMock.mockResolvedValue(json(401, { error: "authentication required" }));
    expect(await resolvePatientContest({ caseId: "c1", contest: "r1", outcome: "decision_upheld", reason: "Matched report verified independently (synthetic)." })).toMatchObject({ ok: false, signIn: true });
  });

  it("refuses in fixtures mode without reading the session", async () => {
    vi.stubEnv("WASL_API_URL", "");
    expect((await resolvePatientContest({ caseId: "c1", contest: "r1", outcome: "decision_upheld", reason: "Matched report verified independently (synthetic)." })).ok).toBe(false);
    expect(reviewerToken).not.toHaveBeenCalled();
    expect(postPatientContestResolutionWithToken).not.toHaveBeenCalled();
  });
});

const base = CaseDetail.parse(loadSample("case_evidence_closure.json"));
const contest = {
  receipt: "c0ffee0123456789",
  kind: "contest_submitted" as const,
  decision: "d3c1510n5ig",
  submitted_at: "2026-10-01T08:00:00Z",
  reason: "The report was for a different visit",
  status: "open" as const,
  owner: "senior_clinician",
  due_at: "2026-10-08T08:00:00Z",
  overdue: false,
  closure_held: true,
  requester: "patient-link-1",
  excluded_reviewers: ["reviewer-one"],
};
const detail = (rows: unknown[]) => CaseDetail.parse({ ...base, patient_requests: rows });
const html = (rows: unknown[], session: ContestSession) =>
  renderToStaticMarkup(createElement(PatientRequestsCard, { detail: detail(rows), review: patientContestReview(base.case.id, session) }));

describe("staff patient-contest card", () => {
  it("states pending, overdue and resolved in the same terms as the patient", () => {
    expect(requestState(contest)).toBe("Pending · senior_clinician review due 2026-10-08 08:00 UTC");
    expect(requestState({ ...contest, overdue: true })).toBe("Overdue · senior_clinician review overdue since 2026-10-08 08:00 UTC");
    expect(requestState({ ...contest, status: "resolved", outcome: "decision_upheld", resolved_by: "clinician-three", resolved_at: "2026-10-02T09:00:00Z" })).toBe(
      "Resolved: decision upheld by clinician-three 2026-10-02 09:00 UTC",
    );
  });

  it("shows the patient's reason and the hold, and offers no control on a snapshot", () => {
    const out = html([contest], { live: false, signedIn: false });
    expect(out).toContain("The report was for a different visit");
    expect(out).toContain("Closure is held");
    expect(out).toContain("This snapshot is read-only");
    expect(out).not.toContain("Resolve patient contest");
  });

  it("asks for a personal sign-in instead of showing controls when nobody is signed in", () => {
    const out = html([contest], { live: true, signedIn: false, mode: "demo" });
    expect(out).toContain('data-testid="governance-signin"');
    expect(out).not.toContain("Record contest outcome");
  });

  it("shows controls to a signed-in viewer, only on open contests", () => {
    const resolved = { ...contest, receipt: "fedcba9876543210", status: "resolved", outcome: "decision_revised", closure_held: false };
    const out = html([contest, resolved], { live: true, signedIn: true, mode: "demo" });
    expect(out.match(/Record contest outcome/g)).toHaveLength(1);
    expect(out).toContain("not reviewer-one");
  });

  it("offers no credential picker and no default outcome", () => {
    const out = renderToStaticMarkup(createElement(PatientContestControls, { caseId: "c1", contest: "r1", excluded: [] }));
    expect(out).not.toContain('name="credential"');
    const select = out.match(/<select[^>]*name="outcome"[^>]*>([\s\S]*?)<\/select>/);
    expect(select?.[0]).toMatch(/required/);
    const options = [...(select?.[1] ?? "").matchAll(/<option([^>]*)>/g)].map((m) => m[1]);
    expect(options.filter((o) => o.includes("selected"))).toEqual([expect.stringContaining('value=""')]);
  });

  it("asks for a required reason and shows the recorded reason to staff after resolution", () => {
    const out = renderToStaticMarkup(createElement(PatientContestControls, { caseId: "c1", contest: "r1", excluded: [] }));
    expect(out).toMatch(/<textarea[^>]*name="reason"[^>]*required/);
    const resolved = { ...contest, status: "resolved", outcome: "decision_upheld", resolution_reason: "Matched report verified (synthetic)." };
    expect(html([resolved], { live: true, signedIn: true, mode: "demo" })).toContain("Reviewer&#x27;s reason: Matched report verified (synthetic).");
  });
});
