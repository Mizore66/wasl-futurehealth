import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { startSyntheticOidcProvider, type SyntheticOidcProvider } from "./support/synthetic-oidc-provider";
import { natsServerBinary, startNatsServer, type TestNatsServer } from "./support/nats-server";

/*
 * Two web replicas in one test process: each replica is a separate module graph (vi.resetModules), so each has its own
 * session-store singleton and NATS connection. Nothing is shared between them except the JetStream buckets.
 */
const { jar, revalidateMock } = vi.hoisted(() => ({
  jar: { value: undefined as string | undefined, get: vi.fn(), set: vi.fn(), delete: vi.fn() },
  revalidateMock: vi.fn(),
}));
vi.mock("next/headers", () => ({ cookies: async () => jar }));
vi.mock("next/cache", () => ({ revalidatePath: revalidateMock }));
vi.mock("next/navigation", () => ({ notFound: () => { throw new Error("NEXT_NOT_FOUND"); } }));

type Replica = {
  start: (request: Request) => Promise<Response>;
  callback: (request: Request) => Promise<Response>;
  resolveReviewer: typeof import("@/lib/api/reviewer-session").resolveReviewer;
  signOutReviewer: typeof import("@/lib/api/reviewer-session").signOutReviewer;
  close: () => Promise<void>;
};

async function loadReplica(): Promise<Replica> {
  vi.resetModules();
  const [{ GET: start }, { GET: callback }, session, store] = await Promise.all([
    import("@/app/app/auth/oidc/start/route"),
    import("@/app/app/auth/oidc/callback/route"),
    import("@/lib/api/reviewer-session"),
    import("@/lib/auth/oidc-store"),
  ]);
  return { start, callback, resolveReviewer: session.resolveReviewer, signOutReviewer: session.signOutReviewer, close: store.closeDefaultSessionStore };
}

const APP = "http://127.0.0.1:3298";
const binary = natsServerBinary();
let nats: TestNatsServer;
let provider: SyntheticOidcProvider;
let replicas: Replica[] = [];

describe.skipIf(!binary)("OIDC sign-in across two web replicas sharing NATS session storage", () => {
  beforeAll(async () => {
    nats = await startNatsServer(binary!);
  });
  afterAll(async () => {
    await nats?.close();
  });

  beforeEach(async () => {
    provider = await startSyntheticOidcProvider();
    const variables = {
      WASL_REVIEW_AUTH_MODE: "oidc",
      WASL_OIDC_ISSUER: provider.issuer,
      WASL_OIDC_CLIENT_ID: provider.clientId,
      WASL_OIDC_CLIENT_SECRET: provider.clientSecret,
      WASL_OIDC_REDIRECT_URI: provider.redirectUri,
      WASL_OIDC_SUBJECT_MAP_JSON: '{"synthetic-r1":"reviewer","synthetic-r2":"reviewer_2"}',
      WASL_OIDC_SESSION_STORE: "nats",
      WASL_OIDC_NATS_URL: nats.url,
      WASL_OIDC_NATS_REPLICAS: "1",
      WASL_OIDC_INSECURE_LOOPBACK: "1",
      WASL_SYNTHETIC_DATA_ONLY: "1",
      WASL_API_REVIEWER_TOKEN: "synthetic-server-r1-token-0001",
      WASL_API_REVIEWER_2_TOKEN: "synthetic-server-r2-token-0002",
    };
    for (const [name, value] of Object.entries(variables)) vi.stubEnv(name, value);
    vi.stubEnv("WASL_OIDC_SESSION_DB", "");
    jar.value = undefined;
    jar.get.mockImplementation((name: string) => (name === "__Secure-wasl_review_session" && jar.value ? { value: jar.value } : undefined));
    replicas = [await loadReplica(), await loadReplica()];
  });

  afterEach(async () => {
    for (const replica of replicas) await replica.close();
    await provider.close();
    vi.useRealTimers();
    vi.unstubAllEnvs();
    jar.get.mockReset();
    jar.set.mockReset();
    jar.delete.mockReset();
  });

  async function begin(replica: Replica) {
    const response = await replica.start(new Request(`${APP}/app/auth/oidc/start?returnTo=${encodeURIComponent("/app/review/case-1")}`));
    expect(response.status).toBe(303);
    const cookie = response.headers.getSetCookie()[0].split(";", 1)[0];
    const authorized = await fetch(response.headers.get("location")!, { redirect: "manual" });
    expect(authorized.status).toBe(302);
    return { cookie, callbackUrl: authorized.headers.get("location")! };
  }
  const sessionCookie = (response: Response) =>
    response.headers.getSetCookie().find((value) => value.startsWith("__Secure-wasl_review_session=") && !value.startsWith("__Secure-wasl_review_session=;"));
  const refused = (response: Response) => response.status === 303 && response.headers.get("location")!.includes("oidc_error=1") && response.headers.get("set-cookie") === null;

  it("completes a login started on one replica on the other exactly once, and refuses replay on both", async () => {
    const [a, b] = replicas;
    const login = await begin(a);
    const completed = await b.callback(new Request(login.callbackUrl, { headers: { cookie: login.cookie } }));
    expect(completed.status).toBe(303);
    expect(completed.headers.get("location")).toBe(`${APP}/app/review/case-1`);
    const cookie = sessionCookie(completed)!;
    expect(cookie).toMatch(/^__Secure-wasl_review_session=[A-Za-z0-9_-]{43};/);
    expect(cookie.toLowerCase()).toContain("samesite=strict");
    expect(cookie).toContain("HttpOnly");
    expect(cookie).toContain("Secure");
    for (const secret of [provider.clientSecret, "synthetic-server-r1-token-0001", "access_token", "id_token"]) {
      expect([...completed.headers].toString()).not.toContain(secret);
    }
    expect(provider.tokenRequests).toBe(1);

    expect(refused(await a.callback(new Request(login.callbackUrl, { headers: { cookie: login.cookie } })))).toBe(true);
    expect(refused(await b.callback(new Request(login.callbackUrl, { headers: { cookie: login.cookie } })))).toBe(true);
    expect(provider.tokenRequests).toBe(1);
  });

  it("lets only one of two concurrent callbacks on different replicas create a session", async () => {
    const [a, b] = replicas;
    const login = await begin(a);
    const responses = await Promise.all([a, b, a, b].map((replica) => replica.callback(new Request(login.callbackUrl, { headers: { cookie: login.cookie } }))));
    expect(responses.filter(sessionCookie)).toHaveLength(1);
    expect(responses.filter(refused)).toHaveLength(3);
    expect(provider.tokenRequests).toBe(1);
  });

  it("refuses a callback from a different browser on the other replica", async () => {
    const [a, b] = replicas;
    const login = await begin(a);
    const other = await begin(b);
    expect(refused(await b.callback(new Request(login.callbackUrl, { headers: { cookie: other.cookie } })))).toBe(true);
    expect(provider.tokenRequests).toBe(0);
  });

  it("maps the session on either replica, revokes it on both at logout, and keeps it across a replica restart", async () => {
    const [a, b] = replicas;
    provider.subject = "synthetic-r2";
    const login = await begin(a);
    jar.value = sessionCookie(await a.callback(new Request(login.callbackUrl, { headers: { cookie: login.cookie } })))!.split(";", 1)[0].split("=")[1];

    expect(await b.resolveReviewer()).toMatchObject({ ok: true, token: "synthetic-server-r2-token-0002", method: "oidc" });
    /* A restarted replica reconnects and still sees the valid session. */
    await b.close();
    expect(await b.resolveReviewer()).toMatchObject({ ok: true, token: "synthetic-server-r2-token-0002" });

    await b.signOutReviewer("case-1");
    expect(await a.resolveReviewer()).toEqual({ ok: false, reason: "revoked" });
    expect(await b.resolveReviewer()).toEqual({ ok: false, reason: "revoked" });
  });

  it("expires the session on both replicas at the fixed eight-hour limit", async () => {
    const [a, b] = replicas;
    const login = await begin(a);
    jar.value = sessionCookie(await a.callback(new Request(login.callbackUrl, { headers: { cookie: login.cookie } })))!.split(";", 1)[0].split("=")[1];
    expect((await b.resolveReviewer()).ok).toBe(true);
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(Date.now() + 8 * 60 * 60 * 1000 + 1);
    expect(await a.resolveReviewer()).toEqual({ ok: false, reason: "expired" });
    expect(await b.resolveReviewer()).toEqual({ ok: false, reason: "expired" });
  });
});
