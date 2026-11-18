import { mkdtempSync, readdirSync, readFileSync, rmSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { startSyntheticOidcProvider, type SyntheticOidcProvider } from "./support/synthetic-oidc-provider";

const { jar } = vi.hoisted(() => ({ jar: { get: vi.fn(), set: vi.fn(), delete: vi.fn() } }));
vi.mock("next/headers", () => ({ cookies: async () => jar }));
vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: () => {}, push: () => {} }), notFound: () => { throw new Error("NEXT_NOT_FOUND"); } }));

import { GET as start } from "@/app/app/auth/oidc/start/route";
import { GET as callback } from "@/app/app/auth/oidc/callback/route";
import { OIDC_SESSION_SAME_SITE, sessionEndedMessage } from "@/components/review/auth";
import { ReviewerLogin, ReviewerSessionStatus } from "@/components/review/reviewer-login";
import { resolveReviewer } from "@/lib/api/reviewer-session";
import { logOidcFailure, safeReturnTo } from "@/lib/auth/oidc";
import { personaFromOidc } from "@/lib/auth/session-persona";

const APP = "http://127.0.0.1:3299";
const WEB = process.cwd();
const read = (...parts: string[]) => readFileSync(path.join(WEB, ...parts), "utf8");

let provider: SyntheticOidcProvider;
let directory: string;
let warn: ReturnType<typeof vi.spyOn>;

beforeEach(async () => {
  provider = await startSyntheticOidcProvider();
  directory = mkdtempSync(path.join(tmpdir(), "wasl-oidc-fixes-"));
  const variables = {
    WASL_REVIEW_AUTH_MODE: "oidc",
    WASL_OIDC_ISSUER: provider.issuer,
    WASL_OIDC_CLIENT_ID: provider.clientId,
    WASL_OIDC_CLIENT_SECRET: provider.clientSecret,
    WASL_OIDC_REDIRECT_URI: provider.redirectUri,
    WASL_OIDC_SUBJECT_MAP_JSON: '{"synthetic-r1":"reviewer"}',
    WASL_OIDC_SESSION_DB: path.join(directory, "sessions.sqlite"),
    WASL_OIDC_INSECURE_LOOPBACK: "1",
    WASL_SYNTHETIC_DATA_ONLY: "1",
    WASL_API_REVIEWER_TOKEN: "synthetic-server-r1-token-0001",
  };
  for (const [name, value] of Object.entries(variables)) vi.stubEnv(name, value);
  warn = vi.spyOn(console, "warn").mockImplementation(() => {});
});

afterEach(async () => {
  await provider.close();
  rmSync(directory, { recursive: true, force: true });
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
  jar.get.mockReset();
});

const startUrl = (query: string) => new Request(`${APP}/app/auth/oidc/start?${query}`);
const toProvider = (response: Response) => {
  expect(response.status).toBe(303);
  expect(response.headers.get("location")).toMatch(new RegExp(`^${provider.issuer}/`));
  expect(response.headers.getSetCookie()).toHaveLength(1);
};
const refused = (response: Response) => {
  expect(response.status).toBe(303);
  expect(response.headers.get("location")).toContain("oidc_error=1");
  expect(response.headers.get("set-cookie")).toBeNull();
};
const warnings = () => warn.mock.calls.map((call: unknown[]) => String(call[0]));

describe("fix 1: Sign in with OIDC is a full-page navigation and tolerates Next's _rsc", () => {
  it("renders a plain anchor to the start route, not a client-router Link", () => {
    expect(read("components", "review", "reviewer-login.tsx")).not.toContain("next/link");
    const html = renderToStaticMarkup(createElement(ReviewerLogin, { caseId: "case/1", mode: "oidc" }));
    expect(html).toContain('href="/app/auth/oidc/start?returnTo=%2Fapp%2Freview%2Fcase%252F1"');
    expect(html).toMatch(/<a [^>]*href="\/app\/auth\/oidc\/start[^"]*"[^>]*>Sign in with OIDC<\/a>/);
  });

  it("accepts the start URL with the _rsc parameter the client router adds", async () => {
    toProvider(await start(startUrl("returnTo=%2Fapp%2Freview%2Fcase-1&_rsc=1ld0r")));
    toProvider(await start(startUrl("returnTo=%2Fapp%2Freview%2Fcase-1&_rsc=")));
    toProvider(await start(startUrl("_rsc=abc123")));
  });

  it("keeps refusing everything else: a repeated or malformed _rsc and any other parameter beside it", async () => {
    for (const query of [
      "returnTo=%2Fapp%2Freview%2Fx&_rsc=a&_rsc=b",
      "returnTo=%2Fapp%2Freview%2Fx&_rsc=%2Fevil",
      `returnTo=%2Fapp%2Freview%2Fx&_rsc=${"a".repeat(65)}`,
      "returnTo=%2Fapp%2Freview%2Fx&_rsc=a&role=reviewer",
      "returnTo=%2Fapp%2Freview%2Fx&_rsc=a&actor=x",
      "returnTo=%2Fapp%2Freview%2Fx&_rsc=a&extra=1",
      "returnTo=https%3A%2F%2Fevil.example%2F&_rsc=a",
    ]) refused(await start(startUrl(query)));
    expect(provider.tokenRequests).toBe(0);
  });
});

describe("fix 2: the session cookie reaches the first page after a cross-site IdP redirect", () => {
  it("issues a SameSite=Lax session cookie and documents why it keeps CSRF protection", async () => {
    expect(OIDC_SESSION_SAME_SITE).toBe("lax");
    const begun = await start(startUrl("returnTo=%2Fapp%2Freview%2Fcase-1"));
    const authorized = await fetch(begun.headers.get("location")!, { redirect: "manual" });
    const response = await callback(new Request(authorized.headers.get("location")!, { headers: { cookie: begun.headers.getSetCookie()[0].split(";", 1)[0] } }));
    expect(response.status).toBe(303);
    const session = response.headers.getSetCookie().find((value) => value.startsWith("__Secure-wasl_review_session="))!;
    expect(session.toLowerCase()).toContain("samesite=lax");
    expect(session).toContain("HttpOnly");
    expect(session).toContain("Secure");
    expect(session).toContain("Path=/app");
    expect(read("components", "review", "auth.ts")).toMatch(/CSRF/);
    expect(read("app", "app", "auth", "oidc", "callback", "route.ts")).toContain("OIDC_SESSION_SAME_SITE");
  });

  it("relies on no state-changing GET under /app: every /app route handler exports GET only", () => {
    const handlers: string[] = [];
    const walk = (dir: string) => {
      for (const name of readdirSync(dir)) {
        const full = path.join(dir, name);
        if (statSync(full).isDirectory()) walk(full);
        else if (name === "route.ts") handlers.push(full);
      }
    };
    walk(path.join(WEB, "app", "app"));
    expect(handlers.length).toBeGreaterThan(0);
    for (const file of handlers) expect(readFileSync(file, "utf8"), file).not.toMatch(/export\s+(async\s+)?(function|const)\s+(POST|PUT|PATCH|DELETE)\b/);
  });
});

describe("fix 3: an ended session explains itself above the sign-in card", () => {
  it.each([
    ["expired", /expired/],
    ["revoked", /ended/],
    ["unmapped", /access has changed/],
  ] as const)("shows a %s message before the card", (reason, words) => {
    const html = renderToStaticMarkup(createElement(ReviewerLogin, { caseId: "c", mode: "oidc", reason }));
    expect(html).toContain('data-testid="session-ended-notice"');
    expect(html).toMatch(words);
    expect(html.indexOf("session-ended-notice")).toBeLessThan(html.indexOf('data-testid="reviewer-login"'));
  });

  it("shows nothing for an anonymous visitor or an unknown reason", () => {
    expect(renderToStaticMarkup(createElement(ReviewerLogin, { caseId: "c", mode: "oidc" }))).not.toContain("session-ended-notice");
    expect(sessionEndedMessage("anonymous")).toBeUndefined();
    expect(sessionEndedMessage("toString")).toBeUndefined();
  });

  it("the review and disposition pages pass the computed reason through", () => {
    for (const page of [read("app", "app", "review", "[id]", "page.tsx"), read("app", "app", "patients", "[id]", "disposition", "page.tsx")]) {
      expect(page).toContain('reason={viewer.reason === "anonymous" ? undefined : viewer.reason}');
    }
  });
});

describe("fix 4: OIDC failures log a category and nothing sensitive", () => {
  it("logs the start route's refusal category", async () => {
    vi.stubEnv("WASL_REVIEW_AUTH_MODE", "demo");
    refused(await start(startUrl("returnTo=%2Fapp%2Freview%2Fx")));
    vi.stubEnv("WASL_REVIEW_AUTH_MODE", "oidc");
    provider.overrides.discoveryIssuer = `${provider.issuer}/different`;
    refused(await start(startUrl("returnTo=%2Fapp%2Freview%2Fx")));
    expect(warnings()).toEqual([
      "[wasl-oidc] start refused: category=disabled",
      expect.stringMatching(/^\[wasl-oidc\] start refused: category=discovery error=\w+( code=[A-Z_]+)?$/),
    ]);
  });

  it("logs an unmapped subject at the callback without the subject, code, state or cookie", async () => {
    provider.subject = "synthetic-unmapped";
    const begun = await start(startUrl("returnTo=%2Fapp%2Freview%2Fcase-1"));
    const cookie = begun.headers.getSetCookie()[0].split(";", 1)[0];
    const callbackUrl = (await fetch(begun.headers.get("location")!, { redirect: "manual" })).headers.get("location")!;
    refused(await callback(new Request(callbackUrl, { headers: { cookie } })));
    expect(warnings()).toEqual(["[wasl-oidc] callback refused: category=unmapped_subject"]);
    const url = new URL(callbackUrl);
    for (const secret of ["synthetic-unmapped", url.searchParams.get("code")!, url.searchParams.get("state")!, cookie.split("=")[1]]) {
      expect(warnings().join("\n")).not.toContain(secret);
    }
  });

  it("logs a callback without its correlation cookie and a provider error", async () => {
    refused(await callback(new Request(`${APP}/app/auth/oidc/callback?code=abc&state=def`)));
    refused(await callback(new Request(`${APP}/app/auth/oidc/callback?error=access_denied`)));
    expect(warnings()).toEqual(["[wasl-oidc] callback refused: category=no_correlation", "[wasl-oidc] callback refused: category=provider_error"]);
  });

  it("logs an unreadable session configuration, naming the variable but never its value", async () => {
    vi.stubEnv("WASL_OIDC_SESSION_STORE", "redis");
    jar.get.mockReturnValue({ value: "A".repeat(43) });
    expect(await resolveReviewer()).toEqual({ ok: false, reason: "anonymous" });
    expect(warnings()).toEqual(['[wasl-oidc] session refused: category=session_resolve error=OidcConfigurationError detail="WASL_OIDC_SESSION_STORE must be sqlite or nats"']);
    expect(warnings().join("\n")).not.toContain("A".repeat(43));
    expect(warnings().join("\n")).not.toContain(provider.clientSecret);
  });

  it("drops library messages and malformed codes", () => {
    logOidcFailure("callback", "token_exchange", Object.assign(new Error("id_token eyJhbGciOi.secret.sig"), { code: "OAUTH_JWT_CLAIM_COMPARISON_FAILED" }));
    logOidcFailure("callback", "token_exchange", Object.assign(new Error("x"), { code: "has spaces and eyJ" }));
    expect(warnings()).toEqual([
      "[wasl-oidc] callback refused: category=token_exchange error=Error code=OAUTH_JWT_CLAIM_COMPARISON_FAILED",
      "[wasl-oidc] callback refused: category=token_exchange error=Error",
    ]);
  });
});

describe("fix 5: who you are, and returnTo stays under /app", () => {
  it("names the OIDC subject and its mapped credential after sign-in", () => {
    const html = renderToStaticMarkup(
      createElement(ReviewerSessionStatus, { session: { method: "oidc", expiresAt: "2026-11-18T12:00:00.000Z" }, persona: personaFromOidc("synthetic-r1", "reviewer_2") }),
    );
    expect(html).toContain("Signed in with OIDC as");
    expect(html).toContain("synthetic-r1");
    expect(html).toContain("Second reviewer · OIDC");
    expect(html).toContain("Session expires at");
  });

  it("falls back to the plain line without a persona, and never shows an OIDC persona for a demo session", () => {
    const plain = renderToStaticMarkup(createElement(ReviewerSessionStatus, { session: { method: "oidc", expiresAt: "x" } }));
    expect(plain).toContain("Signed in with OIDC.");
    const demo = renderToStaticMarkup(createElement(ReviewerSessionStatus, { session: { method: "demo" }, persona: personaFromOidc("s", "reviewer") }));
    expect(demo).toContain("Signed in with a demo token.");
    expect(demo).not.toContain(">s<");
  });

  it("both pages pass reviewerPersona() to the session line", () => {
    for (const page of [read("app", "app", "review", "[id]", "page.tsx"), read("app", "app", "patients", "[id]", "disposition", "page.tsx")]) {
      expect(page).toContain('persona={viewer.session.method === "oidc" ? await reviewerPersona() : null}');
    }
  });

  it("normalises dot segments before checking the /app prefix", () => {
    expect(safeReturnTo("/app/../../x", APP)).toBe("/app/review");
    expect(safeReturnTo("/app/../x", APP)).toBe("/app/review");
    expect(safeReturnTo("/app/review/../patients/c", APP)).toBe("/app/patients/c");
  });

  it("documents the OIDC variables for deployment and the shared tokens the web server still needs", () => {
    const deploy = read("..", "docs", "DEPLOY.md");
    for (const name of ["WASL_REVIEW_AUTH_MODE", "WASL_OIDC_ISSUER", "WASL_OIDC_CLIENT_ID", "WASL_OIDC_CLIENT_SECRET", "WASL_OIDC_REDIRECT_URI", "WASL_OIDC_SUBJECT_MAP_JSON"]) {
      expect(deploy).toContain(`\`${name}\``);
    }
    const readme = read("README.md");
    const example = readme.slice(readme.indexOf("WASL_REVIEW_AUTH_MODE=oidc \\"), readme.indexOf("pnpm dev", readme.indexOf("WASL_REVIEW_AUTH_MODE=oidc \\")));
    expect(example).toContain("WASL_API_WEB_REVIEWER_TOKEN=");
    expect(example).toContain("WASL_API_AUDITOR_TOKEN=");
  });
});
