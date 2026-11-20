import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { jar, getCaseWithToken } = vi.hoisted(() => ({
  jar: { get: vi.fn(), set: vi.fn(), delete: vi.fn() },
  getCaseWithToken: vi.fn(),
}));
vi.mock("next/headers", () => ({ cookies: async () => jar }));
vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: () => {}, push: () => {} }), notFound: () => { throw new Error("NEXT_NOT_FOUND"); } }));
vi.mock("@/lib/api/source", async (original) => ({
  ...(await original<typeof import("@/lib/api/source")>()),
  apiMode: () => "live",
  getCaseWithToken,
}));

import { DEMO_PERSONA_COOKIE, DEMO_PERSONAS, REVIEW_COOKIE } from "@/components/review/auth";
import { ReviewerLogin, ReviewerSessionStatus } from "@/components/review/reviewer-login";
import { GovernanceSignIn } from "@/components/case/governance-controls";
import { resolveReviewer, reviewerPersona, signInAsDemoPersona, signInReviewer } from "@/lib/api/reviewer-session";

const TOKENS = {
  WASL_API_REVIEWER_TOKEN: "synthetic-persona-r1-token-0001",
  WASL_API_REVIEWER_2_TOKEN: "synthetic-persona-r2-token-0002",
  WASL_API_CLINICIAN_TOKEN: "synthetic-unit-clinician-token-01",
};
const cookie = (values: Record<string, string>) => (name: string) => (name in values ? { name, value: values[name] } : undefined);

beforeEach(() => {
  vi.stubEnv("WASL_REVIEW_AUTH_MODE", "persona");
  for (const [name, value] of Object.entries(TOKENS)) vi.stubEnv(name, value);
  getCaseWithToken.mockResolvedValue({ ok: true, detail: {} });
});

afterEach(() => {
  vi.unstubAllEnvs();
  jar.get.mockReset();
  jar.set.mockReset();
  jar.delete.mockReset();
  getCaseWithToken.mockReset();
});

describe("open-demo personas", () => {
  it("signs in by persona name: the cookie holds the persona, never a token", async () => {
    expect(await signInAsDemoPersona("case-1", "reviewer_2")).toEqual({ ok: true });
    expect(getCaseWithToken).toHaveBeenCalledWith("case-1", TOKENS.WASL_API_REVIEWER_2_TOKEN);
    const [name, value, options] = jar.set.mock.calls[0];
    expect([name, value]).toEqual([DEMO_PERSONA_COOKIE, "reviewer_2"]);
    expect(options).toMatchObject({ httpOnly: true, secure: true, sameSite: "strict", path: "/app" });
    expect(JSON.stringify(jar.set.mock.calls)).not.toContain("synthetic-persona");
  });

  it("acts with the picked persona's own server-side token and names the persona", async () => {
    jar.get.mockImplementation(cookie({ [DEMO_PERSONA_COOKIE]: "reviewer" }));
    expect(await resolveReviewer()).toEqual({ ok: true, token: TOKENS.WASL_API_REVIEWER_TOKEN, method: "persona", credential: "reviewer" });
    expect(await reviewerPersona()).toEqual({ actor: "Reviewer A", role: "Reviewer · demo persona", method: "persona" });
  });

  it("refuses unknown, unconfigured and shared credentials, and never falls back to a pasted token", async () => {
    for (const credential of ["web_reviewer", "clinician", "reviewer_3", "nobody"]) {
      expect(await signInAsDemoPersona("case-1", credential)).toMatchObject({ ok: false });
    }
    expect(jar.set).not.toHaveBeenCalled();
    jar.get.mockImplementation(cookie({ [DEMO_PERSONA_COOKIE]: "clinician", [REVIEW_COOKIE]: TOKENS.WASL_API_REVIEWER_TOKEN }));
    expect(await resolveReviewer()).toEqual({ ok: false, reason: "anonymous" });
    expect(await signInReviewer("case-1", TOKENS.WASL_API_REVIEWER_TOKEN)).toMatchObject({ ok: false });
  });

  it("is off unless WASL_REVIEW_AUTH_MODE=persona", async () => {
    vi.stubEnv("WASL_REVIEW_AUTH_MODE", "demo");
    expect(await signInAsDemoPersona("case-1", "reviewer")).toMatchObject({ ok: false });
    jar.get.mockImplementation(cookie({ [DEMO_PERSONA_COOKIE]: "reviewer" }));
    expect(await resolveReviewer()).toEqual({ ok: false, reason: "anonymous" });
  });

  it("offers personas, not a token field, and labels a demo sign-off", () => {
    for (const html of [
      renderToStaticMarkup(createElement(ReviewerLogin, { caseId: "case-1", mode: "persona" })),
      renderToStaticMarkup(createElement(GovernanceSignIn, { caseId: "case-1", mode: "persona" })),
    ]) {
      expect(html).not.toMatch(/type="password"|[Tt]oken/);
      for (const persona of DEMO_PERSONAS) expect(html).toContain(`Act as ${persona.name}`);
      expect(html).toContain("two-person rule still holds");
    }
    const status = renderToStaticMarkup(
      createElement(ReviewerSessionStatus, { session: { method: "persona" }, persona: { actor: "Reviewer B", role: "Reviewer · demo persona", method: "persona" } }),
    );
    expect(status).toContain("Demo sign-off as");
    expect(status).toContain("Reviewer B");
  });
});
