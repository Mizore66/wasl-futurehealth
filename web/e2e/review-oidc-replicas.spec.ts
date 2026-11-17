import { readFileSync } from "node:fs";
import { spawn, type ChildProcess } from "node:child_process";
import { createServer, type Server } from "node:http";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";
import { startSyntheticOidcProvider, type SyntheticOidcProvider } from "../tests/support/synthetic-oidc-provider";
import { natsServerBinary, startNatsServer, type TestNatsServer } from "../tests/support/nats-server";

/*
 * Two `next start` replicas of one build share OIDC session storage on a loopback JetStream server. The browser
 * starts sign-in on replica A; the synthetic provider returns to replica B's callback. Cookies are scoped to the host,
 * not the port, so the one browser holds one session for both replicas, as it would behind a load balancer.
 * Synthetic data only: a loopback OIDC provider, a minimal read-only mock API and synthetic subjects.
 */
const MOCK_PORT = 3413;
const A_PORT = 3411;
const B_PORT = 3412;
const holdId = fixtures.fixtures.review_hold.id;
const deskOn = (port: number) => `http://127.0.0.1:${port}/app/review/${encodeURIComponent(holdId)}`;
const TOKEN = "synthetic-replica-r1-token-0001";
/* Shared server credentials the app needs for its non-review pages; never mapped to an OIDC subject. */
const SHARED = {
  WASL_API_WEB_REVIEWER_TOKEN: "synthetic-replica-web-0000001",
  WASL_API_CLINICIAN_TOKEN: "synthetic-replica-clinician-01",
  WASL_API_AUDITOR_TOKEN: "synthetic-replica-auditor-0001",
  WASL_API_COORDINATOR_TOKEN: "synthetic-replica-coord-000001",
};
const dir = path.join(process.cwd(), "data", "api");
const read = (file: string) => JSON.parse(readFileSync(path.join(dir, file), "utf8"));

const binary = natsServerBinary();

test.describe("OIDC sessions shared by two web replicas", () => {
  test.beforeEach(({}, testInfo) => test.skip(testInfo.project.name !== "chromium", "owns fixed ports: Chromium only"));
  test.skip(!binary, "needs bin/nats-server or WASL_NATS_SERVER");
  test.describe.configure({ mode: "serial" });

  let nats: TestNatsServer;
  let provider: SyntheticOidcProvider;
  let api: Server;
  const replicas = new Map<number, ChildProcess>();
  let caseReads = 0;

  async function startReplica(port: number) {
    const child = spawn(path.join(process.cwd(), "node_modules", ".bin", "next"), ["start", "-p", String(port)], {
      env: {
        ...process.env,
        ...SHARED,
        WASL_API_URL: `http://127.0.0.1:${MOCK_PORT}`,
        WASL_API_REVIEWER_TOKEN: TOKEN,
        WASL_REVIEW_AUTH_MODE: "oidc",
        WASL_OIDC_ISSUER: provider.issuer,
        WASL_OIDC_CLIENT_ID: provider.clientId,
        WASL_OIDC_CLIENT_SECRET: provider.clientSecret,
        WASL_OIDC_REDIRECT_URI: provider.redirectUri,
        WASL_OIDC_SUBJECT_MAP_JSON: '{"synthetic-r1":"reviewer"}',
        WASL_OIDC_SESSION_STORE: "nats",
        WASL_OIDC_SESSION_DB: "",
        WASL_OIDC_NATS_URL: nats.url,
        WASL_OIDC_NATS_REPLICAS: "1",
        WASL_OIDC_INSECURE_LOOPBACK: "1",
        WASL_SYNTHETIC_DATA_ONLY: "1",
      },
      stdio: "ignore",
    });
    replicas.set(port, child);
    for (let i = 0; i < 120; i++) {
      if (await fetch(`http://127.0.0.1:${port}/app`).then((r) => r.ok, () => false)) return;
      await new Promise((r) => setTimeout(r, 500));
    }
    throw new Error(`next start on ${port} did not become ready`);
  }
  async function stopReplica(port: number) {
    const child = replicas.get(port);
    replicas.delete(port);
    if (!child || child.exitCode !== null) return;
    const exited = new Promise((resolve) => child.once("exit", resolve));
    child.kill();
    /* `next start` drains open connections on SIGTERM (server.close() without closeAllConnections() outside dev),
     * so a client keep-alive can delay exit indefinitely on a loaded runner. Bound the wait, then force the stop. */
    const timedOut = await Promise.race([exited.then(() => false), new Promise((r) => setTimeout(() => r(true), 10_000))]);
    if (timedOut) {
      child.kill("SIGKILL");
      await exited;
    }
  }

  test.beforeAll(async ({}, testInfo) => {
    if (testInfo.project.name !== "chromium" || !binary) return;
    test.setTimeout(180_000);
    nats = await startNatsServer(binary);
    provider = await startSyntheticOidcProvider({ redirectUri: `http://127.0.0.1:${B_PORT}/app/auth/oidc/callback` });
    const meta = read("fixtures.json").meta;
    api = createServer((req, res) => {
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(JSON.stringify(body));
      };
      const pathname = decodeURIComponent(new URL(req.url ?? "/", "http://mock").pathname);
      const bearer = (req.headers.authorization ?? "").replace(/^Bearer /, "");
      if (pathname.startsWith("/api/") && ![TOKEN, ...Object.values(SHARED)].includes(bearer)) return send(401, { error: "authentication required" });
      if (pathname === "/api/fixtures") return send(200, read("fixtures.json"));
      if (pathname === "/api/dashboard") return send(200, read("dashboard.json"));
      if (pathname === "/api/cases") return send(200, { meta, cases: [], total: 0, limit: 50, offset: 0 });
      if (pathname.endsWith("/audit")) return send(200, read("case_review_hold_audit.json"));
      if (pathname === `/api/cases/${holdId}`) {
        if (bearer !== TOKEN) return send(403, { meta, error: "reviewer credential required" });
        caseReads += 1;
        const detail = read("case_review_hold.json");
        return send(200, { ...detail, case: { ...detail.case, closed: false, review: "open" }, signoffs: [], api_reviews: [], api_review_states: [] });
      }
      return send(404, { meta, error: "not found" });
    });
    await new Promise<void>((resolve) => api.listen(MOCK_PORT, "127.0.0.1", resolve));
    await Promise.all([startReplica(A_PORT), startReplica(B_PORT)]);
  });

  test.afterAll(async () => {
    await Promise.all([...replicas.keys()].map(stopReplica));
    if (api) await new Promise((resolve) => api.close(resolve));
    if (provider) await provider.close();
    if (nats) await nats.close();
  });

  const signedIn = async (page: Page) => expect(page.getByTestId("reviewer-session")).toContainText("Signed in with OIDC");
  const signedOut = async (page: Page) => expect(page.getByRole("heading", { name: "Sign in to review" })).toBeVisible();

  test("sign-in started on one replica completes once on the other, survives a replica restart and logs out on both", async ({ browser }) => {
    test.setTimeout(180_000);
    const context = await browser.newContext();
    const page = await context.newPage();
    let callbackUrl = "";
    page.on("request", (request) => {
      if (request.url().startsWith(`http://127.0.0.1:${B_PORT}/app/auth/oidc/callback`)) callbackUrl = request.url();
    });

    await page.goto(deskOn(A_PORT));
    await signedOut(page);
    await page.getByRole("link", { name: "Sign in with OIDC" }).click();
    await page.waitForURL(deskOn(B_PORT));
    await signedIn(page);
    expect(callbackUrl).toContain("state=");
    expect(await page.content()).not.toContain(TOKEN);
    expect(await page.content()).not.toContain(provider.clientSecret);
    expect(provider.tokenRequests).toBe(1);

    /* The same session cookie is valid on replica A. */
    await page.goto(deskOn(A_PORT));
    await signedIn(page);

    /* Replaying the consumed callback is refused on both replicas, without another token exchange. */
    for (const port of [B_PORT, A_PORT]) {
      const replay = await context.request.get(callbackUrl.replace(`:${B_PORT}/`, `:${port}/`), { maxRedirects: 0 });
      expect(replay.status()).toBe(303);
      expect(replay.headers().location).toContain("oidc_error=1");
    }
    expect(provider.tokenRequests).toBe(1);

    /* A restarted replica still honours the session held in NATS. Leave replica A first so the browser holds no
     * keep-alive connection that its graceful shutdown would wait for. */
    await page.goto("about:blank");
    await stopReplica(A_PORT);
    await startReplica(A_PORT);
    await page.goto(deskOn(A_PORT));
    await signedIn(page);

    /* Logout on B revokes the session server-side; the old cookie value is refused on A as well. */
    const session = (await context.cookies()).find((cookie) => cookie.name === "__Secure-wasl_review_session")!;
    await page.goto(deskOn(B_PORT));
    await page.getByRole("button", { name: "Sign out" }).click();
    await signedOut(page);
    await context.addCookies([{ name: session.name, value: session.value, domain: "127.0.0.1", path: "/app", httpOnly: true, secure: true, sameSite: "Strict" }]);
    const before = caseReads;
    await page.goto(deskOn(A_PORT));
    await signedOut(page);
    expect(caseReads).toBe(before);
    await context.close();
  });
});
