import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { SqliteSessionStore } from "@/lib/auth/oidc-store";

const { store, revalidateMock, notFoundMock } = vi.hoisted(() => ({
  store: { get: vi.fn(), set: vi.fn(), delete: vi.fn() },
  revalidateMock: vi.fn(),
  notFoundMock: vi.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
}));
vi.mock("next/headers", () => ({ cookies: async () => store }));
vi.mock("next/cache", () => ({ revalidatePath: revalidateMock }));
vi.mock("next/navigation", () => ({ notFound: notFoundMock }));

import { getReviewCase, resolveReviewer, signInReviewer, signOutReviewer } from "@/lib/api/reviewer-session";

const TOKEN = "reviewer-one-token-1234567890";
const oidcStores: SqliteSessionStore[] = [];
const oidcDirectories: string[] = [];
const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-12T00:00:00Z", note: "synthetic" };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// A minimal detail: CaseDetail is a loose object, but required keys are checked, so borrow a real sample.
import { loadSample } from "./samples";
const detailBody = () => loadSample("case_evidence_closure.json");

beforeEach(() => {
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  vi.stubEnv("WASL_API_REVIEWER_TOKEN", "shared-reviewer-secret-123456");
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  store.get.mockReset();
  store.set.mockReset();
  store.delete.mockReset();
  revalidateMock.mockReset();
  while (oidcStores.length) oidcStores.pop()!.close();
  while (oidcDirectories.length) rmSync(oidcDirectories.pop()!, { recursive: true, force: true });
});

describe("signInReviewer", () => {
  it.each(["", "short", "has a space in the token 1234", "x".repeat(4097)])("rejects malformed token %#", async (token) => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(await signInReviewer("c1", token)).toEqual({ ok: false, error: expect.any(String) });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(store.set).not.toHaveBeenCalled();
  });

  it("verifies the token on this case, then sets an HttpOnly Secure SameSite=Strict review-path cookie", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json(200, detailBody()));
    vi.stubGlobal("fetch", fetchMock);
    const log = vi.spyOn(console, "log");
    const err = vi.spyOn(console, "error");
    const result = await signInReviewer("c1", TOKEN);
    expect(result).toEqual({ ok: true });
    expect(JSON.stringify(result)).not.toContain(TOKEN);
    expect(fetchMock.mock.calls[0][1].headers.get("authorization")).toBe(`Bearer ${TOKEN}`);
    expect(store.set).toHaveBeenCalledWith("wasl_review_token", TOKEN, {
      httpOnly: true,
      secure: true,
      sameSite: "strict",
      path: "/app",
      maxAge: 8 * 60 * 60,
    });
    expect(JSON.stringify([log.mock.calls, err.mock.calls])).not.toContain(TOKEN);
  });

  it.each([401, 403, 404])("returns a generic refusal and sets no cookie on %i", async (status) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(status, { error: "nope", meta })));
    const result = await signInReviewer("c1", TOKEN);
    expect(result).toEqual({ ok: false, error: "Sign-in failed. Check the token and try again." });
    expect(store.set).not.toHaveBeenCalled();
  });

  it("refuses when the API is unreachable, without leaking the token", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error(`connect failed for ${TOKEN}`)));
    const result = await signInReviewer("c1", TOKEN);
    expect(result.ok).toBe(false);
    expect(JSON.stringify(result)).not.toContain(TOKEN);
  });
});

describe("signOutReviewer", () => {
  it("deletes the review-path cookie and revalidates the case", async () => {
    await signOutReviewer("c1");
    expect(store.delete).toHaveBeenCalledWith({ name: "wasl_review_token", path: "/app" });
    expect(revalidateMock).toHaveBeenCalledWith("/app/review/c1");
  });
});

describe("getReviewCase", () => {
  it("is sign-in-required without a cookie and never uses the shared reviewer token", async () => {
    store.get.mockReturnValue(undefined);
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(await getReviewCase("c1")).toEqual({ detail: null, authenticated: false });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("reads with the cookie's raw bearer token and no identity headers", async () => {
    store.get.mockReturnValue({ value: TOKEN });
    const fetchMock = vi.fn().mockResolvedValue(json(200, detailBody()));
    vi.stubGlobal("fetch", fetchMock);
    const result = await getReviewCase("c1");
    expect(result.authenticated).toBe(true);
    const headers = fetchMock.mock.calls[0][1].headers as Headers;
    expect(headers.get("authorization")).toBe(`Bearer ${TOKEN}`);
    expect(headers.has("x-wasl-role")).toBe(false);
    expect(headers.has("x-wasl-actor")).toBe(false);
    expect(JSON.stringify(result)).not.toContain(TOKEN);
  });

  it("treats a rejected credential as signed out", async () => {
    store.get.mockReturnValue({ value: TOKEN });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(401, { error: "bad", meta })));
    expect(await getReviewCase("c1")).toEqual({ detail: null, authenticated: false });
  });

  it("ignores a malformed cookie value", async () => {
    store.get.mockReturnValue({ value: "short" });
    expect(await getReviewCase("c1")).toEqual({ detail: null, authenticated: false });
  });
});

describe("OIDC reviewer sessions", () => {
  const MAPPED_TOKEN = "mapped-r1-server-token-123456";
  function configure(options: { subject?: string; credential?: "reviewer" | "reviewer_2"; now?: number } = {}) {
    const directory = mkdtempSync(path.join(tmpdir(), "wasl-reviewer-oidc-"));
    oidcDirectories.push(directory);
    const file = path.join(directory, "session.sqlite");
    const values = {
      WASL_REVIEW_AUTH_MODE: "oidc",
      WASL_OIDC_ISSUER: "https://id.example.test",
      WASL_OIDC_CLIENT_ID: "review-client",
      WASL_OIDC_CLIENT_SECRET: "server-only-client-secret",
      WASL_OIDC_REDIRECT_URI: "https://wasl.example.test/app/auth/oidc/callback",
      WASL_OIDC_SUBJECT_MAP_JSON: '{"synthetic-r1":"reviewer","synthetic-r2":"reviewer_2"}',
      WASL_OIDC_SESSION_DB: file,
      WASL_API_REVIEWER_TOKEN: MAPPED_TOKEN,
      WASL_API_REVIEWER_2_TOKEN: "mapped-r2-server-token-123456",
    };
    for (const [name, value] of Object.entries(values)) vi.stubEnv(name, value);
    const sessions = new SqliteSessionStore(file);
    oidcStores.push(sessions);
    const { sessionId } = sessions.createSession({ issuer: "https://id.example.test", subject: options.subject ?? "synthetic-r1", credential: options.credential ?? "reviewer", now: options.now });
    store.get.mockImplementation((name: string) => name === "__Secure-wasl_review_session" ? { value: sessionId } : undefined);
    return { sessions, sessionId };
  }

  it("maps the verified stored subject to only its current configured bearer token", async () => {
    configure();
    const fetchMock = vi.fn().mockResolvedValue(json(200, detailBody()));
    vi.stubGlobal("fetch", fetchMock);
    const result = await getReviewCase("c1");
    expect(result).toMatchObject({ authenticated: true, session: { method: "oidc", expiresAt: expect.any(String) } });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][1].headers.get("authorization")).toBe(`Bearer ${MAPPED_TOKEN}`);
    expect(JSON.stringify(result)).not.toContain(MAPPED_TOKEN);
    expect(JSON.stringify(result)).not.toContain("synthetic-r1");
  });

  it("does not call the API for an unmapped subject or changed credential mapping", async () => {
    const first = configure({ subject: "synthetic-unmapped" });
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(await getReviewCase("c1")).toEqual({ detail: null, authenticated: false, reason: "unmapped" });
    first.sessions.close();
    oidcStores.pop();

    configure({ subject: "synthetic-r1", credential: "reviewer" });
    vi.stubEnv("WASL_OIDC_SUBJECT_MAP_JSON", '{"synthetic-r1":"reviewer_2","synthetic-r2":"reviewer"}');
    expect(await resolveReviewer()).toEqual({ ok: false, reason: "unmapped" });
    expect(await getReviewCase("c1")).toEqual({ detail: null, authenticated: false, reason: "unmapped" });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("withholds case data for revoked, expired, malformed or unknown sessions", async () => {
    const { sessions, sessionId } = configure();
    sessions.revokeSession(sessionId);
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(await getReviewCase("c1")).toEqual({ detail: null, authenticated: false, reason: "revoked" });
    sessions.close();
    oidcStores.pop();

    configure({ now: Date.now() - 9 * 60 * 60 * 1000 });
    expect(await getReviewCase("c1")).toEqual({ detail: null, authenticated: false, reason: "expired" });
    store.get.mockReturnValue({ value: "malformed" });
    expect(await getReviewCase("c1")).toEqual({ detail: null, authenticated: false });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("revokes the server-side session on logout and refuses an old replayed cookie", async () => {
    const { sessions, sessionId } = configure();
    await signOutReviewer("c1");
    expect(sessions.resolveSession(sessionId)).toEqual({ ok: false, reason: "revoked" });
    expect(store.set).toHaveBeenCalledWith("__Secure-wasl_review_session", "", { httpOnly: true, secure: true, sameSite: "strict", path: "/app", maxAge: 0 });
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(await getReviewCase("c1")).toEqual({ detail: null, authenticated: false, reason: "revoked" });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("disables demo token entry in OIDC mode", async () => {
    configure();
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect((await signInReviewer("c1", TOKEN)).ok).toBe(false);
    expect(store.set).not.toHaveBeenCalled();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
