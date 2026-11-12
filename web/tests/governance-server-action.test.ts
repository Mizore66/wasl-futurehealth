import { renderToStaticMarkup } from "react-dom/server";
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { refresh, revalidatePath } from "next/cache";
import { appendGovernanceAction, signInForGovernance, signOutForGovernance } from "@/lib/api/governance-actions";
import { reviewerToken, signInReviewer, signOutReviewer } from "@/lib/api/reviewer-session";
import { postContestResolutionWithToken, postContestWithToken, postUndoWithToken } from "@/lib/api/source";
import { GovernanceControls } from "@/components/case/governance-controls";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn(), refresh: vi.fn() }));
vi.mock("@/lib/api/reviewer-session", () => ({ reviewerToken: vi.fn(), signInReviewer: vi.fn(), signOutReviewer: vi.fn() }));
vi.mock("@/lib/api/source", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/source")>();
  return { ...actual, postUndoWithToken: vi.fn(), postContestWithToken: vi.fn(), postContestResolutionWithToken: vi.fn() };
});

/** The signed-in person's own token, held server-side in the reviewer-session cookie. */
const SESSION = "signed-in-clinician-two-token";
const posts = () => [postUndoWithToken, postContestWithToken, postContestResolutionWithToken].map((fn) => vi.mocked(fn));
const created = (hash: string) => ({ ok: true, status: 201, response: { event: { hash } } }) as never;
/*
 * Every server-held credential `source.ts` reads (`WASL_API_${Role | "web_reviewer"}_TOKEN`), plus the numbered tokens the
 * removed credential picker used. They are all present, so a fallback such as `reviewerToken() ?? process.env...` would
 * find a token and append: the no-session tests below must still see no request.
 */
const SERVER_TOKENS = {
  WASL_API_REVIEWER_TOKEN: "server-reviewer-token-1234567890",
  WASL_API_WEB_REVIEWER_TOKEN: "server-web-reviewer-token-1234567890",
  WASL_API_CLINICIAN_TOKEN: "server-clinician-token-1234567890",
  WASL_API_COORDINATOR_TOKEN: "server-coordinator-token-1234567890",
  WASL_API_AUDITOR_TOKEN: "server-auditor-token-1234567890",
  WASL_API_REVIEWER_2_TOKEN: "server-reviewer-two-token-1234567890",
  WASL_API_REVIEWER_3_TOKEN: "server-reviewer-three-token-1234567890",
  WASL_API_CLINICIAN_2_TOKEN: "server-clinician-two-token-1234567890",
  WASL_API_CLINICIAN_3_TOKEN: "server-clinician-three-token-1234567890",
};
const fetchMock = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  for (const [name, value] of Object.entries(SERVER_TOKENS)) vi.stubEnv(name, value);
  vi.stubGlobal("fetch", fetchMock);
  vi.mocked(reviewerToken).mockResolvedValue(SESSION);
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("appendGovernanceAction acts only as the signed-in identity", () => {
  it("resolves with the session token and revalidates the original and resolving history", async () => {
    vi.mocked(postContestResolutionWithToken).mockResolvedValue(created("resolution-hash"));
    const result = await appendGovernanceAction({
      caseId: "case/1",
      target: "contest-hash",
      operation: "resolve",
      resolution: "upheld",
      reason: "Independent clinician checked the evidence",
    });
    expect(result).toMatchObject({ ok: true, hash: "resolution-hash" });
    expect(result.ok && result.message).toContain("signed-in credential");
    expect(postContestResolutionWithToken).toHaveBeenCalledWith(
      "case/1",
      { target: "contest-hash", resolution: "upheld", note: "Independent clinician checked the evidence" },
      SESSION,
    );
    expect(revalidatePath).toHaveBeenCalledWith("/app/patients/case%2F1/audit");
    expect(revalidatePath).toHaveBeenCalledWith("/app/review/case%2F1");
  });

  it.each(["undo", "contest"] as const)("%s uses the session token", async (operation) => {
    const post = operation === "undo" ? postUndoWithToken : postContestWithToken;
    vi.mocked(post).mockResolvedValue(created(`${operation}-hash`));
    const result = await appendGovernanceAction({ caseId: "c1", target: "review-hash", operation, reason: "Needs a second look" });
    expect(result.ok).toBe(true);
    expect(post).toHaveBeenCalledWith("c1", { target: "review-hash", note: "Needs a second look" }, SESSION);
  });

  it.each([
    ["credential", { credential: "clinician_2" }],
    ["credential (empty)", { credential: "" }],
    ["token", { token: "someone-elses-token-123456" }],
    ["role", { role: "senior_clinician" }],
    ["actor", { actor: "clinician-three" }],
  ])("rejects a client-supplied %s and appends nothing", async (_name, identity) => {
    const result = await appendGovernanceAction({
      caseId: "c1",
      target: "contest-hash",
      operation: "resolve",
      resolution: "rejected",
      reason: "Trying to choose who acts",
      ...identity,
    });
    expect(result).toEqual({ ok: false, error: "The page cannot choose who acts. Governance uses only the signed-in credential." });
    for (const post of posts()) expect(post).not.toHaveBeenCalled();
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("rejects any other unexpected field rather than passing it on", async () => {
    const result = await appendGovernanceAction({ caseId: "c1", target: "h", operation: "undo", reason: "r", as: "reviewer_3" });
    expect(result.ok).toBe(false);
    for (const post of posts()) expect(post).not.toHaveBeenCalled();
  });

  it.each(["undo", "contest", "resolve"] as const)(
    "without a session, %s appends nothing and asks for a sign-in, although server-held tokens are configured",
    async (operation) => {
      vi.mocked(reviewerToken).mockResolvedValue(undefined);
      for (const name of Object.keys(SERVER_TOKENS)) expect(process.env[name]).toBeTruthy();
      const result = await appendGovernanceAction({
        caseId: "c1",
        target: "review-hash",
        operation,
        ...(operation === "resolve" ? { resolution: "upheld" } : {}),
        reason: "No session",
      });
      expect(result).toMatchObject({ ok: false, signIn: true });
      for (const post of posts()) expect(post).not.toHaveBeenCalled();
      expect(fetchMock).not.toHaveBeenCalled();
      expect(revalidatePath).not.toHaveBeenCalled();
    },
  );

  it("treats an API-rejected session token as signed out", async () => {
    vi.mocked(postUndoWithToken).mockResolvedValue({ ok: false, status: 401, error: "authentication required" });
    const result = await appendGovernanceAction({ caseId: "c1", target: "review-hash", operation: "undo", reason: "Undo" });
    expect(result).toMatchObject({ ok: false, signIn: true });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("surfaces a same-actor refusal without claiming an event was appended", async () => {
    vi.mocked(postContestResolutionWithToken).mockResolvedValue({ ok: false, status: 403, error: "actor must differ" });
    const result = await appendGovernanceAction({
      caseId: "case/1",
      target: "contest-hash",
      operation: "resolve",
      resolution: "upheld",
      reason: "Attempted self-resolution",
    });
    expect(result).toEqual({ ok: false, error: "actor must differ" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it.each([undefined, "", "uphold", "default"])("a resolution has no default: %j is refused before any request", async (resolution) => {
    const result = await appendGovernanceAction({ caseId: "c1", target: "contest-hash", operation: "resolve", resolution, reason: "Resolve" });
    expect(result.ok).toBe(false);
    for (const post of posts()) expect(post).not.toHaveBeenCalled();
  });

  it("requires a reason at the server-action boundary", async () => {
    const result = await appendGovernanceAction({ caseId: "case/1", target: "review-hash", operation: "undo", reason: " " });
    expect(result.ok).toBe(false);
    expect(postUndoWithToken).not.toHaveBeenCalled();
  });

  it("refuses in fixtures mode without reading the session", async () => {
    vi.stubEnv("WASL_API_URL", "");
    const result = await appendGovernanceAction({ caseId: "c1", target: "review-hash", operation: "undo", reason: "Undo" });
    expect(result.ok).toBe(false);
    expect(reviewerToken).not.toHaveBeenCalled();
  });
});

describe("governance sign-in", () => {
  it("uses the reviewer-session sign-in and refreshes only on success", async () => {
    vi.mocked(signInReviewer).mockResolvedValue({ ok: false, error: "Sign-in failed. Check the token and try again." });
    expect(await signInForGovernance("c1", "bad-token-1234567890")).toEqual({ ok: false, error: "Sign-in failed. Check the token and try again." });
    expect(refresh).not.toHaveBeenCalled();

    vi.mocked(signInReviewer).mockResolvedValue({ ok: true });
    expect(await signInForGovernance("c1", SESSION)).toEqual({ ok: true });
    expect(signInReviewer).toHaveBeenLastCalledWith("c1", SESSION);
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("signs out through the reviewer session", async () => {
    await signOutForGovernance("c1");
    expect(signOutReviewer).toHaveBeenCalledWith("c1");
    expect(refresh).toHaveBeenCalled();
  });
});

describe("GovernanceControls", () => {
  it("offers no credential picker and no default resolution", () => {
    const html = renderToStaticMarkup(createElement(GovernanceControls, { caseId: "c1", target: "contest-hash", operation: "resolve" }));
    expect(html).not.toContain('name="credential"');
    expect(html).not.toMatch(/Reviewer \d|Clinician \d|Coordinator/);
    const select = html.match(/<select[^>]*name="resolution"[^>]*>([\s\S]*?)<\/select>/);
    expect(select?.[0]).toMatch(/required/);
    const options = [...(select?.[1] ?? "").matchAll(/<option([^>]*)>/g)].map((m) => m[1]);
    expect(options[0]).toContain('value=""');
    expect(options.filter((o) => o.includes("selected"))).toEqual([expect.stringContaining('value=""')]);
  });
});
