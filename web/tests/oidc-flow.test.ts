import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GET as start } from "@/app/app/auth/oidc/start/route";
import { GET as callback } from "@/app/app/auth/oidc/callback/route";
import { startSyntheticOidcProvider, type SyntheticOidcProvider } from "./support/synthetic-oidc-provider";

const APP = "http://127.0.0.1:3298";
let provider: SyntheticOidcProvider;
let directory: string;

beforeEach(async () => {
  provider = await startSyntheticOidcProvider();
  directory = mkdtempSync(path.join(tmpdir(), "wasl-oidc-flow-"));
  const variables = {
    WASL_REVIEW_AUTH_MODE: "oidc",
    WASL_OIDC_ISSUER: provider.issuer,
    WASL_OIDC_CLIENT_ID: provider.clientId,
    WASL_OIDC_CLIENT_SECRET: provider.clientSecret,
    WASL_OIDC_REDIRECT_URI: provider.redirectUri,
    WASL_OIDC_SUBJECT_MAP_JSON: '{"synthetic-r1":"reviewer","synthetic-r2":"reviewer_2","synthetic-senior":"reviewer_3"}',
    WASL_OIDC_SESSION_DB: path.join(directory, "sessions.sqlite"),
    WASL_OIDC_INSECURE_LOOPBACK: "1",
    WASL_SYNTHETIC_DATA_ONLY: "1",
    WASL_API_REVIEWER_TOKEN: "synthetic-server-r1-token-0001",
    WASL_API_REVIEWER_2_TOKEN: "synthetic-server-r2-token-0002",
    WASL_API_REVIEWER_3_TOKEN: "synthetic-server-senior-token-03",
  };
  for (const [name, value] of Object.entries(variables)) vi.stubEnv(name, value);
});

afterEach(async () => {
  await provider.close();
  rmSync(directory, { recursive: true, force: true });
  vi.useRealTimers();
  vi.unstubAllEnvs();
});

async function begin(returnTo = "/app/review/case-1") {
  const response = await start(new Request(`${APP}/app/auth/oidc/start?returnTo=${encodeURIComponent(returnTo)}`));
  expect(response.status).toBe(303);
  expect(response.headers.get("cache-control")).toBe("no-store");
  const authorize = response.headers.get("location");
  expect(authorize).toBeTruthy();
  return authorize!;
}

async function authorize(authorizationUrl: string) {
  const response = await fetch(authorizationUrl, { redirect: "manual" });
  expect(response.status).toBe(302);
  return response.headers.get("location")!;
}

async function complete(authorizationUrl?: string) {
  const callbackUrl = await authorize(authorizationUrl ?? (await begin()));
  return { response: await callback(new Request(callbackUrl)), callbackUrl };
}

function expectRefusal(response: Response) {
  expect(response.status).toBe(303);
  expect(response.headers.get("location")).toContain("oidc_error=1");
  expect(response.headers.get("set-cookie")).toBeNull();
  expect(response.headers.get("cache-control")).toBe("no-store");
}

describe("OIDC authorization-code flow", () => {
  it("uses discovery/JWKS, code, state, nonce and S256 PKCE and sets only an opaque strict cookie", async () => {
    const authorizationUrl = await begin();
    const parameters = new URL(authorizationUrl).searchParams;
    expect([...parameters.keys()].sort()).toEqual(["client_id", "code_challenge", "code_challenge_method", "nonce", "redirect_uri", "response_type", "scope", "state"].sort());
    expect(parameters.get("code_challenge_method")).toBe("S256");
    expect(parameters.get("scope")).toBe("openid");
    expect(parameters.get("state")!.length).toBeGreaterThanOrEqual(43);
    expect(parameters.get("nonce")!.length).toBeGreaterThanOrEqual(43);
    expect(authorizationUrl).not.toContain(provider.clientSecret);

    const { response, callbackUrl } = await complete(authorizationUrl);
    expect(response.status).toBe(303);
    expect(response.headers.get("location")).toBe(`${APP}/app/review/case-1`);
    const cookie = response.headers.get("set-cookie")!;
    expect(cookie).toMatch(/^__Secure-wasl_review_session=[A-Za-z0-9_-]{43};/);
    expect(cookie).toContain("Path=/app");
    expect(cookie).toContain("HttpOnly");
    expect(cookie).toContain("Secure");
    expect(cookie.toLowerCase()).toContain("samesite=strict");
    expect(Number(cookie.match(/Max-Age=(\d+)/)?.[1])).toBeGreaterThanOrEqual(28790);
    expect(Number(cookie.match(/Max-Age=(\d+)/)?.[1])).toBeLessThanOrEqual(28800);
    expect(cookie.toLowerCase()).not.toContain("domain=");
    for (const secret of [provider.clientSecret, "synthetic-server-r1-token-0001", "code_verifier", "access_token", "id_token"]) {
      expect(callbackUrl).not.toContain(secret);
      expect([...response.headers].toString()).not.toContain(secret);
    }
  });

  it.each(["issuer", "audience", "azp", "nonce", "tamperToken", "expiry"] as const)("refuses an invalid ID-token %s", async (override) => {
    const authorizationUrl = await begin();
    if (override === "issuer") provider.overrides.issuer = `${provider.issuer}/wrong`;
    if (override === "audience") provider.overrides.audience = "wrong-client";
    if (override === "azp") provider.overrides.azp = "wrong-client";
    if (override === "expiry") provider.overrides.expiresIn = -300;
    if (override === "nonce") provider.overrides.nonce = "wrong-nonce";
    if (override === "tamperToken") provider.overrides.tamperToken = true;
    const { response } = await complete(authorizationUrl);
    expectRefusal(response);
    expect(provider.tokenRequests).toBe(1);
  });

  it("refuses a token signed by an unknown key", async () => {
    const authorizationUrl = await begin();
    provider.overrides.signingKey = "unknown";
    expectRefusal((await complete(authorizationUrl)).response);
  });

  it("refuses a verified but unmapped subject", async () => {
    const authorizationUrl = await begin();
    provider.subject = "synthetic-unmapped";
    expectRefusal((await complete(authorizationUrl)).response);
  });

  it("refuses wrong state before calling the token endpoint", async () => {
    const callbackUrl = new URL(await authorize(await begin()));
    callbackUrl.searchParams.set("state", "x".repeat(43));
    expectRefusal(await callback(new Request(callbackUrl)));
    expect(provider.tokenRequests).toBe(0);
  });

  it("refuses expired state before discovery or token exchange", async () => {
    const callbackUrl = await authorize(await begin());
    vi.useFakeTimers();
    vi.setSystemTime(Date.now() + 11 * 60 * 1000);
    expectRefusal(await callback(new Request(callbackUrl)));
    expect(provider.tokenRequests).toBe(0);
  });

  it("cannot replay a consumed callback", async () => {
    const first = await complete();
    expect(first.response.headers.get("set-cookie")).toBeTruthy();
    const requests = provider.tokenRequests;
    expectRefusal(await callback(new Request(first.callbackUrl)));
    expect(provider.tokenRequests).toBe(requests);
  });

  it("refuses identity-bearing callback and start parameters", async () => {
    const authorizationUrl = await begin();
    const callbackUrl = new URL(await authorize(authorizationUrl));
    callbackUrl.searchParams.set("actor", "synthetic-r2");
    expectRefusal(await callback(new Request(callbackUrl)));
    expect(provider.tokenRequests).toBe(0);
    expectRefusal(await start(new Request(`${APP}/app/auth/oidc/start?returnTo=%2Fapp%2Freview%2Fx&role=reviewer`)));
  });

  it("refuses discovery issuer mismatch", async () => {
    provider.overrides.discoveryIssuer = `${provider.issuer}/different`;
    expectRefusal(await start(new Request(`${APP}/app/auth/oidc/start`)));
    expect(provider.tokenRequests).toBe(0);
  });

  it("rejects external and protocol-relative return targets", async () => {
    for (const target of ["https://evil.example/path", "//evil.example/app/review/x", "/app/review/x?actor=other"]) {
      const response = await start(new Request(`${APP}/app/auth/oidc/start?returnTo=${encodeURIComponent(target)}`));
      expectRefusal(response);
    }
  });

  it("is unavailable in demo mode", async () => {
    vi.stubEnv("WASL_REVIEW_AUTH_MODE", "demo");
    expectRefusal(await start(new Request(`${APP}/app/auth/oidc/start`)));
  });

  it("synthetic provider rejects a wrong PKCE verifier", async () => {
    const callbackUrl = new URL(await authorize(await begin()));
    const response = await fetch(`${provider.issuer}/token`, {
      method: "POST",
      headers: { authorization: `Basic ${Buffer.from(`${provider.clientId}:${provider.clientSecret}`).toString("base64")}`, "content-type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ grant_type: "authorization_code", code: callbackUrl.searchParams.get("code")!, redirect_uri: provider.redirectUri, code_verifier: "wrong-verifier-wrong-verifier-wrong-verifier" }),
    });
    expect(response.status).toBe(400);
    expectRefusal(await callback(new Request(callbackUrl)));
    expect(provider.tokenRequests).toBe(2);
  });
});
