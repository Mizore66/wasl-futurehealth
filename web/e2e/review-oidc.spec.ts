import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { spawn, type ChildProcess } from "node:child_process";
import { createServer, type Server } from "node:http";
import { DatabaseSync } from "node:sqlite";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Browser, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";
import { startSyntheticOidcProvider, type SyntheticOidcProvider } from "../tests/support/synthetic-oidc-provider";

/*
 * Independent double review against a stateful synthetic mock of the Go API (pipeline/review_protocol.go shapes:
 * concealed:true on a hidden first read, one-way digest heads that never equal an event hash, resolution_basis
 * "agreement" / "senior_resolution", and both review_state and api_review_states on a 201).
 *
 * Self-contained: the spec starts a loopback synthetic OIDC provider, a mock API and its own `next start` (after
 * `next build`). Subjects map only to configured synthetic tokens. Chromium project only, so other projects never
 * fight over the ports.
 */
const MOCK_PORT = 3397;
const NEXT_PORT = 3398;
const NEXT_URL = `http://127.0.0.1:${NEXT_PORT}`;
const SHARED = {
  WASL_API_REVIEWER_TOKEN: "synthetic-shared-reviewer-0001",
  WASL_API_WEB_REVIEWER_TOKEN: "synthetic-web-reviewer-00001",
  WASL_API_CLINICIAN_TOKEN: "synthetic-shared-clinician-0001",
  WASL_API_AUDITOR_TOKEN: "synthetic-shared-auditor-00001",
  WASL_API_COORDINATOR_TOKEN: "synthetic-shared-coord-000001",
};
const holdId = fixtures.fixtures.review_hold.id;
const desk = `${NEXT_URL}/app/review/${encodeURIComponent(holdId)}`;
const LINK = "rv-636161dc42837e08";
const EVIDENCE = "rv-d55f7ccf9f12c409";

type Kind = "r1" | "r2" | "senior" | "web";
type Reader = Exclude<Kind, "web">;
type Read = { read: "r1" | "r2" | "r3"; actor: string; decision: string; hash: string; role: string; at: string; prev: string };
type Principal = { actor: string; kind: Kind };

const dir = path.join(process.cwd(), "data", "api");
const read = (f: string) => JSON.parse(readFileSync(path.join(dir, f), "utf8"));
const meta = read("fixtures.json").meta;

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

test.describe("independent double review with synthetic OIDC", () => {
  test.beforeEach(({}, testInfo) => test.skip(testInfo.project.name !== "chromium", "owns fixed ports: Chromium only"));
  test.describe.configure({ mode: "serial" });

  const tokens = {
    r1: "synthetic-r1-token-aaaaaaaa",
    r2: "synthetic-r2-token-bbbbbbbb",
    senior: "synthetic-senior-token-cccc",
  };
  const principals = new Map<string, Principal>();
  const questions: Record<string, { question: "link" | "evidence"; reads: Read[] }> = {
    [LINK]: { question: "link", reads: [] },
    [EVIDENCE]: { question: "evidence", reads: [] },
  };
  let server: Server;
  let provider: SyntheticOidcProvider;
  let sessionDirectory = "";
  let sessionDb = "";
  let nextServer: ChildProcess | undefined;
  /** Mock switches for the interaction tests. */
  const salt = 0;
  let brokenBodyOnce = false;
  let failGetOnce = false;
  let auditRequests = 0;
  let reviewGets = 0;
  let reviewerReviewGets = 0;
  const posts: { token: string; headers: Record<string, unknown>; body: Record<string, unknown> }[] = [];

  const hash = (item: string, slot: string) => `${slot}${item.slice(3, 11)}abcdef0123456789`;
  /** The opaque head a write must present: "" initially, then a digest that never equals an event hash. */
  const headOf = (item: string) => (questions[item].reads.length === 0 ? "" : `digest-${salt}-${questions[item].reads.length}-${item.slice(3, 9)}`);

  function projection(p: Principal) {
    return Object.entries(questions).map(([item, q]) => {
      const [r1, r2, r3] = q.reads;
      const shown = (r: Read | undefined, mine: boolean) => ({ read: r!.read, submitted: true, ...(mine ? { decision: r!.decision, hash: r!.hash } : { concealed: true }) });
      if (!r1) return { item, question: q.question, state: "awaiting_r1", head: headOf(item), next_read: "r1", may_submit: p.kind !== "senior", reads: [{ read: "r1", submitted: false }], disagreement: false, final_decision: "", resolution_basis: "", canonical_unchanged: true };
      if (!r2)
        return {
          item, question: q.question, state: "awaiting_r2", head: headOf(item), next_read: "r2", may_submit: p.kind !== "senior" && p.actor !== r1.actor,
          reads: [shown(r1, p.actor === r1.actor), { read: "r2", submitted: false }], disagreement: false, canonical_unchanged: true,
        };
      const disagree = r1.decision !== r2.decision;
      const both = [shown(r1, true), shown(r2, true)];
      if (!r3 && !disagree)
        return { item, question: q.question, state: "resolved_agreed", head: headOf(item), next_read: null, may_submit: false, reads: both, disagreement: false, final_decision: r1.decision, resolution_basis: "agreement", canonical_unchanged: true };
      if (!r3)
        return { item, question: q.question, state: "awaiting_r3", head: headOf(item), next_read: "r3", may_submit: p.kind === "senior", reads: [...both, { read: "r3", submitted: false }], disagreement: true, final_decision: "", resolution_basis: "", canonical_unchanged: true };
      return { item, question: q.question, state: "resolved_senior", head: headOf(item), next_read: null, may_submit: false, reads: [...both, shown(r3, true)], disagreement: true, final_decision: r3.decision, resolution_basis: "senior_resolution", canonical_unchanged: true };
    });
  }

  const event = (item: string, r: Read) => ({
    case: holdId, item, decision: r.decision, note: "", role: r.role, actor: r.actor, action: questions[item].question === "link" ? "signoff_link" : "signoff_evidence",
    at: r.at, mode: "api", prev: r.prev, hash: r.hash, protocol: "wasl-independent-review/1", read: r.read, question: questions[item].question,
  });
  /** Viewer-facing ledger: another actor's r1 is omitted until r2 exists. */
  const ledger = (p: Principal) =>
    Object.entries(questions).flatMap(([item, q]) => q.reads.filter((r) => !(r.read === "r1" && q.reads.length === 1 && r.actor !== p.actor)).map((r) => event(item, r)));

  test.beforeAll(async ({}, testInfo) => {
    if (testInfo.project.name !== "chromium") return;
    for (const [kind, token] of Object.entries(tokens)) {
      if (token.length < 16) throw new Error(`REVIEW_${kind.toUpperCase()}_TOKEN must be set (16+ characters)`);
      principals.set(token, { kind: kind as Kind, actor: `synthetic-${kind}` });
    }
    // Shared server credentials are bound to their own non-reader actors, never to r1/r2/senior.
    for (const [name, token] of Object.entries(SHARED)) principals.set(token, { kind: "web", actor: `shared-${name.toLowerCase()}` });
    provider = await startSyntheticOidcProvider({ redirectUri: `${NEXT_URL}/app/auth/oidc/callback` });
    sessionDirectory = mkdtempSync(path.join(tmpdir(), "wasl-oidc-e2e-"));
    sessionDb = path.join(sessionDirectory, "sessions.sqlite");
    server = createServer((req, res) => {
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(JSON.stringify(body));
      };
      const p = decodeURIComponent(new URL(req.url ?? "/", "http://mock").pathname);
      const [scheme, token] = (req.headers.authorization ?? "").split(" ");
      const principal = scheme === "Bearer" ? principals.get(token) : undefined;
      if (p.startsWith("/api/") && !principal) return send(401, { error: "authentication required" });
      const who = principal!;
      if (req.method === "POST" && p.endsWith("/review")) {
        let raw = "";
        req.on("data", (c) => (raw += c));
        req.on("end", () => {
          const body = JSON.parse(raw);
          posts.push({ token, headers: req.headers, body });
          const q = questions[body.item];
          if (!q) return send(404, { meta, error: "unknown item" });
          const slot = q.reads.length === 0 ? "r1" : q.reads.length === 1 ? "r2" : q.reads.length === 2 ? "r3" : undefined;
          const current = headOf(body.item);
          if (!slot || body.read !== slot || body.expected_head !== current) return send(409, { meta, error: "stale head or wrong slot", current_head: current });
          const disagree = q.reads.length === 2 && q.reads[0].decision !== q.reads[1].decision;
          if ((slot === "r3") !== (who.kind === "senior") || (slot === "r3" && !disagree)) return send(403, { meta, error: "not authorised for this slot" });
          if (slot === "r2" && q.reads[0].actor === who.actor) return send(403, { meta, error: "same actor" });
          const r: Read = { read: slot, actor: who.actor, decision: body.decision, hash: hash(body.item, slot), role: who.kind === "senior" ? "senior_reviewer" : "reviewer", at: "2026-11-12T10:00:00Z", prev: q.reads.at(-1)?.hash || "genesis" };
          q.reads.push(r);
          if (brokenBodyOnce) {
            // Committed, but the 201 body is unreadable; the next read of the case fails once as well.
            brokenBodyOnce = false;
            failGetOnce = true;
            res.writeHead(201, { "content-type": "application/json" });
            return res.end("not json");
          }
          const states = projection(who);
          send(201, { meta, review: event(body.item, r), review_state: states.find((m) => m.item === body.item), api_review_states: states });
        });
        return;
      }
      if (p === "/api/fixtures") return send(200, read("fixtures.json"));
      if (p === "/api/dashboard") return send(200, read("dashboard.json"));
      if (p === "/api/cases") return send(200, { meta, cases: [], total: 0, limit: 50, offset: 0 });
      if (p.endsWith("/undo")) return send(404, { meta, error: "not found" });
      if (p.endsWith("/audit")) {
        auditRequests += 1;
        const audit = read("case_review_hold_audit.json");
        // The auditor's redacted history: decisions are present, actor and note are not. It would leak a concealed r1.
        const history = Object.entries(questions).flatMap(([item, q]) => q.reads.map((r) => { const { actor: _a, note: _n, ...rest } = event(item, r); void _a; void _n; return rest; }));
        return send(200, { ...audit, history });
      }
      if (p === `/api/cases/${holdId}`) {
        reviewGets += 1;
        if (failGetOnce) {
          failGetOnce = false;
          return send(500, { meta, error: "temporary" });
        }
        if (["r1", "r2", "senior"].some((k) => tokens[k as Reader] === token)) reviewerReviewGets += 1;
        const detail = read("case_review_hold.json");
        return send(200, { ...detail, case: { ...detail.case, closed: false, review: "open" }, signoffs: [], api_reviews: ledger(who), api_review_states: projection(who) });
      }
      return send(404, { meta, error: "not found" });
    });
    await new Promise<void>((resolve) => server.listen(MOCK_PORT, "127.0.0.1", resolve));
    nextServer = spawn(path.join(process.cwd(), "node_modules", ".bin", "next"), ["start", "-p", String(NEXT_PORT)], {
      env: {
        ...process.env,
        ...SHARED,
        WASL_API_URL: `http://127.0.0.1:${MOCK_PORT}`,
        WASL_API_REVIEWER_TOKEN: tokens.r1,
        WASL_API_REVIEWER_2_TOKEN: tokens.r2,
        WASL_API_REVIEWER_3_TOKEN: tokens.senior,
        WASL_REVIEW_AUTH_MODE: "oidc",
        WASL_OIDC_ISSUER: provider.issuer,
        WASL_OIDC_CLIENT_ID: provider.clientId,
        WASL_OIDC_CLIENT_SECRET: provider.clientSecret,
        WASL_OIDC_REDIRECT_URI: provider.redirectUri,
        WASL_OIDC_SUBJECT_MAP_JSON: '{"synthetic-r1":"reviewer","synthetic-r2":"reviewer_2","synthetic-senior":"reviewer_3"}',
        WASL_OIDC_SESSION_DB: sessionDb,
        WASL_OIDC_INSECURE_LOOPBACK: "1",
        WASL_SYNTHETIC_DATA_ONLY: "1",
      },
      stdio: "ignore",
    });
    for (let i = 0; i < 120; i++) {
      if (await fetch(`${NEXT_URL}/app`).then((r) => r.ok, () => false)) return;
      await new Promise((r) => setTimeout(r, 500));
    }
    throw new Error("next start did not become ready");
  });

  test.afterAll(async () => {
    nextServer?.kill();
    if (server) await new Promise((resolve) => server.close(resolve));
    if (provider) await provider.close();
    if (sessionDirectory) rmSync(sessionDirectory, { recursive: true, force: true });
  });

  async function viewer(browser: Browser, kind: Reader) {
    const context = await browser.newContext();
    const page = await context.newPage();
    provider.subject = `synthetic-${kind}`;
    await page.goto(desk);
    await expect(page.getByRole("link", { name: "Sign in with OIDC" })).toBeVisible();
    await expect(page.getByLabel("Review token")).toHaveCount(0);
    await page.getByRole("link", { name: "Sign in with OIDC" }).click();
    await expect(page.getByTestId("question-cards")).toBeVisible();
    await expect(page.getByTestId("reviewer-session")).toContainText("Signed in with OIDC");
    await expect(page.getByTestId("reviewer-session")).toContainText("Session expires at");
    expect(await page.content()).not.toContain(tokens[kind]);
    expect(await page.content()).not.toContain(provider.clientSecret);
    return { context, page };
  }
  const rail = (page: Page) => page.getByRole("complementary", { name: "Your answers" });
  const card = (page: Page, question: "link" | "evidence") => rail(page).locator(`section[data-question="${question}"]`);

  test("an unauthenticated viewer gets OIDC only, no credential choice and no case data", async ({ page }) => {
    await page.goto(desk);
    await expect(page.getByRole("heading", { name: "Sign in to review" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Sign in with OIDC" })).toBeVisible();
    await expect(page.getByLabel("Review token")).toHaveCount(0);
    await expect(page.getByRole("radio")).toHaveCount(0);
    await expect(page.getByText("Mariam Al Shamsi")).toHaveCount(0);
    await expect(page.getByTestId("question-cards")).toHaveCount(0);
    const before = reviewerReviewGets;
    await page.goto(`${NEXT_URL}/app/auth/oidc/callback?code=forged&state=forged&actor=synthetic-r2&role=reviewer`);
    await expect(page.getByTestId("signed-out-notice")).toContainText("could not be completed");
    await expect(page.getByTestId("question-cards")).toHaveCount(0);
    expect(reviewerReviewGets).toBe(before);
    await noSeriousAxe(page);
  });

  test("r1 submits both questions, reloads, and stays waiting", async ({ browser }) => {
    const { context, page } = await viewer(browser, "r1");
    await expect(card(page, "link")).toContainText("First read due");
    await expect(card(page, "evidence")).toContainText("First read due");
    await expect(rail(page).getByRole("button", { name: "Sign out" })).toBeVisible();

    await card(page, "link").getByRole("radio", { name: "Same patient", exact: true }).click();
    await card(page, "link").getByRole("button", { name: "Submit first read" }).click();
    await expect(card(page, "link")).toContainText("Your first read is in");
    await expect(card(page, "link")).toContainText("First read: Same patient");
    // Only that question changed; the evidence draft area is untouched.
    await expect(card(page, "evidence")).toContainText("First read due");

    await card(page, "evidence").getByRole("radio", { name: "Not adequate", exact: true }).click();
    await card(page, "evidence").getByRole("button", { name: "Submit first read" }).click();
    await expect(card(page, "evidence")).toContainText("Your first read is in");

    await page.reload();
    await expect(card(page, "link")).toContainText("First read: Same patient");
    await expect(card(page, "evidence")).toContainText("First read: Report not adequate");
    await expect(card(page, "evidence")).not.toContainText("Submit second read");
    await expect(rail(page).getByTestId("progress")).toContainText("0 of 2 questions resolved");
    await expect(page.getByTestId("history-withheld")).toBeVisible();
    await noSeriousAxe(page);
    expect(posts.map((x) => [x.body.item, x.body.read, x.body.expected_head])).toEqual([[LINK, "r1", ""], [EVIDENCE, "r1", ""]]);
    // The credential is the bearer token only: no identity header, no role in the body.
    for (const post of posts) {
      expect(post.headers["x-wasl-role"]).toBeUndefined();
      expect(post.headers["x-wasl-actor"]).toBeUndefined();
      expect(Object.keys(post.body).sort()).toEqual(["decision", "expected_head", "item", "note", "read"]);
    }
    await context.close();
  });

  test("r2 cannot see r1 in page content or embedded history, then disagrees on evidence", async ({ browser }) => {
    const auditBefore = auditRequests;
    const { context, page } = await viewer(browser, "r2");
    for (const q of ["link", "evidence"] as const) {
      await expect(card(page, q)).toContainText("Second read due");
      await expect(card(page, q)).toContainText("the answer stays hidden until the second read");
    }
    const html = await page.content();
    expect(html).not.toContain(hash(LINK, "r1"));
    expect(html).not.toContain(hash(EVIDENCE, "r1"));
    const rsc = await context.request.get(desk, { headers: { RSC: "1" } });
    expect(rsc.status()).toBe(200);
    const rscBody = await rsc.text();
    expect(rscBody).not.toContain(hash(LINK, "r1"));
    expect(rscBody).not.toContain(hash(EVIDENCE, "r1"));
    expect(rscBody).not.toContain('"read":"r1","submitted":true,"decision"');
    await expect(page.locator('[data-read="r1"]')).toHaveCount(2);
    await expect(page.getByRole("list", { name: "Review history" })).toHaveCount(0);
    await expect(page.getByTestId("history-withheld")).toBeVisible();
    expect(auditRequests).toBe(auditBefore);

    await card(page, "link").getByRole("radio", { name: "Same patient", exact: true }).click();
    await card(page, "link").getByRole("button", { name: "Submit second read" }).click();
    await expect(card(page, "link")).toContainText("Resolved: both reads agree");
    await expect(card(page, "link")).toContainText("No senior form is needed");
    await expect(card(page, "link").getByRole("button", { name: /Submit/ })).toHaveCount(0);

    await card(page, "evidence").getByRole("radio", { name: "Adequate", exact: true }).click();
    await card(page, "evidence").getByRole("button", { name: "Submit second read" }).click();
    await expect(card(page, "evidence")).toContainText("First read: Report not adequate");
    await expect(card(page, "evidence")).toContainText("Second read: Report adequate");
    await expect(card(page, "evidence")).toContainText("waiting for a senior");
    await expect(card(page, "evidence").getByRole("button", { name: "Submit senior resolution" })).toHaveCount(0);

    await page.reload();
    await expect(card(page, "link")).toContainText("Resolved: both reads agree");
    await expect(card(page, "evidence")).toContainText("Disagreement");
    await expect(rail(page).getByTestId("progress")).toContainText("1 of 2 questions resolved");
    // Everything is revealed now, so the audit history may render.
    await expect(page.getByRole("list", { name: "Review history" })).toBeVisible();
    await expect(page.getByTestId("history-withheld")).toHaveCount(0);
    await noSeriousAxe(page);
    await context.close();
  });

  test("a reviewer who already agreed sees a resolved question with no submit control", async ({ browser }) => {
    const { context, page } = await viewer(browser, "r2");
    await expect(card(page, "link")).toContainText("Resolved: both reads agree");
    await expect(card(page, "link").getByRole("button", { name: /Submit/ })).toHaveCount(0);
    await context.close();
  });

  test("only the senior sees the r3 form, for the disagreeing question only", async ({ browser }) => {
    const { context, page } = await viewer(browser, "senior");
    await expect(card(page, "link").getByRole("button", { name: /Submit/ })).toHaveCount(0);
    await expect(card(page, "evidence")).toContainText("senior resolution due");
    await expect(card(page, "evidence")).toContainText("First read: Report not adequate");
    await expect(card(page, "evidence")).toContainText("Second read: Report adequate");

    await card(page, "evidence").getByRole("radio", { name: "Not adequate", exact: true }).click();
    await card(page, "evidence").getByRole("button", { name: "Submit senior resolution" }).click();
    await expect(card(page, "evidence")).toContainText("Resolved by a senior");
    await expect(card(page, "evidence")).toContainText("Senior resolution: Report not adequate");

    await page.reload();
    await expect(rail(page).getByTestId("progress")).toContainText("2 of 2 questions resolved");
    for (const slot of ["r1", "r2", "r3"]) await expect(card(page, "evidence").locator(`[data-read="${slot}"]`)).toHaveCount(1);
    const history = page.getByRole("list", { name: "Review history" });
    await expect(history).toBeVisible();
    await expect(history).toContainText("Senior resolution");
    await expect(history).not.toContainText("synthetic-");
    await expect(page.getByRole("link", { name: "Back to Review" })).toBeVisible();
    await page.screenshot({ path: "test-results/screens/review-oidc-resolved.png", fullPage: true });
    await noSeriousAxe(page);

    /* One sign-out on the page: the desk's. The governance history card does not repeat it. */
    await expect(page.getByRole("button", { name: "Sign out" })).toHaveCount(1);
    await rail(page).getByRole("button", { name: "Sign out" }).click();
    await expect(page.getByRole("heading", { name: "Sign in to review" })).toBeVisible();
    await context.close();
  });
  test("an anonymous audit page withholds protocol history", async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();
    await page.goto(`${NEXT_URL}/app/patients/${encodeURIComponent(holdId)}/audit`);
    await expect(page.getByTestId("history-withheld")).toContainText("signed-in reviewer");
    await expect(page.getByRole("list", { name: "Review history" })).toHaveCount(0);
    expect(await page.content()).not.toContain(hash(LINK, "r1"));
    await noSeriousAxe(page);
    await context.close();
  });

  test("a 409 is shown after the state and next read change under the reviewer", async ({ browser }) => {
    for (const q of Object.values(questions)) q.reads.length = 0;
    const { context, page } = await viewer(browser, "r1");
    await expect(card(page, "link")).toContainText("First read due");
    // Another actor lands a first read after this page rendered: next_read moves from r1 to r2.
    questions[LINK].reads.push({ read: "r1", actor: "synthetic-r2", decision: "different", hash: hash(LINK, "r1"), role: "reviewer", at: "2026-11-12T10:00:00Z", prev: "genesis" });
    await card(page, "link").getByRole("radio", { name: "Same patient", exact: true }).click();
    await card(page, "link").getByRole("button", { name: "Submit first read" }).click();
    await expect(card(page, "link").getByRole("alert")).toContainText("the review changed while you were answering");
    await expect(card(page, "link")).toContainText("Second read due");
    await expect(page.getByTestId("read-announcement")).toContainText("changed on the server");
    // No success claim, and no write reached the ledger.
    expect(questions[LINK].reads).toHaveLength(1);
    await noSeriousAxe(page);
    await context.close();
  });

  test("a committed write with an unreadable response closes the stale form and announces it", async ({ browser }) => {
    for (const q of Object.values(questions)) q.reads.length = 0;
    const { context, page } = await viewer(browser, "r1");
    brokenBodyOnce = true;
    await card(page, "link").getByRole("radio", { name: "Same patient", exact: true }).click();
    await card(page, "link").getByRole("button", { name: "Submit first read" }).click();
    // Either the action's re-read failed (form locked) or the revalidated page already shows the new state; never a live stale form.
    await expect(page.getByText(/Do not submit again/)).toBeVisible();
    await expect(card(page, "link").getByRole("button", { name: /Submit/ })).toHaveCount(0);
    await expect(card(page, "link").getByRole("radio")).toHaveCount(0);
    await expect(page.getByText(/Do not submit again/)).toBeVisible();
    await expect(page.getByTestId("read-announcement")).toContainText("was recorded");
    expect(questions[LINK].reads).toHaveLength(1);
    await expect(card(page, "link")).toContainText("Your first read is in");
    await page.reload();
    await expect(card(page, "link")).toContainText("First read: Same patient");
    await expect(page.getByTestId("committed-lock")).toHaveCount(0);
    await noSeriousAxe(page);
    await context.close();
  });

  test("shared pages read as a non-reader and never show a concealed r1, and the cookie reaches the audit page", async ({ browser }) => {
    for (const q of Object.values(questions)) q.reads.length = 0;
    const patient = `${NEXT_URL}/app/patients/${encodeURIComponent(holdId)}`;
    const audit = `${patient}/audit`;
    // r1 submits; the cookie (path /app) must also reach the audit page.
    const r1 = await viewer(browser, "r1");
    await card(r1.page, "link").getByRole("radio", { name: "Same patient", exact: true }).click();
    await card(r1.page, "link").getByRole("button", { name: "Submit first read" }).click();
    await expect(card(r1.page, "link")).toContainText("Your first read is in");
    await r1.context.close();

    // Anonymous visitors and r2 both open the shared pages while r1 is concealed.
    const r2 = await viewer(browser, "r2");
    for (const url of [patient, audit]) {
      await r2.page.goto(url);
      const html = await r2.page.content();
      expect(html).not.toContain(hash(LINK, "r1"));
      expect(html).not.toContain("synthetic-r1");
      await expect(r2.page.getByRole("list", { name: "Review history" })).toHaveCount(0);
    }
    await expect(r2.page.getByTestId("history-withheld")).toBeVisible();
    await r2.context.close();
    const anon = await browser.newContext();
    const anonPage = await anon.newPage();
    await anonPage.goto(patient);
    expect(await anonPage.content()).not.toContain("synthetic-r1");
    await anon.close();

    // Resolve everything, then the signed-in cookie shows the resolved r1/r2/r3 history on the audit page.
    const second = await viewer(browser, "r2");
    await card(second.page, "link").getByRole("radio", { name: "Not the same patient" }).click();
    await card(second.page, "link").getByRole("button", { name: "Submit second read" }).click();
    await expect(card(second.page, "link")).toContainText("waiting for a senior");
    await second.context.close();
    const sr = await viewer(browser, "senior");
    await card(sr.page, "link").getByRole("radio", { name: "Same patient", exact: true }).click();
    await card(sr.page, "link").getByRole("button", { name: "Submit senior resolution" }).click();
    await expect(card(sr.page, "link")).toContainText("Resolved by a senior");
    await sr.page.goto(audit);
    const history = sr.page.getByRole("list", { name: "Review history" });
    await expect(history).toBeVisible();
    for (const label of ["First read", "Second read", "Senior resolution"]) await expect(history).toContainText(label);
    await expect(history).not.toContainText("synthetic-");
    await noSeriousAxe(sr.page);
    await sr.context.close();
  });

  test("logout revokes the old cookie, reauthentication rotates it, and server expiry is authoritative", async ({ browser }) => {
    const { context, page } = await viewer(browser, "r2");
    const original = (await context.cookies()).find((cookie) => cookie.name === "__Secure-wasl_review_session");
    expect(original?.httpOnly).toBe(true);
    expect(original?.secure).toBe(true);
    expect(original?.sameSite).toBe("Strict");
    expect(original?.path).toBe("/app");
    expect(original?.value).toMatch(/^[A-Za-z0-9_-]{43}$/);
    await rail(page).getByRole("button", { name: "Sign out" }).click();
    await expect(page.getByRole("heading", { name: "Sign in to review" })).toBeVisible();
    expect((await context.cookies()).some((cookie) => cookie.name === "__Secure-wasl_review_session")).toBe(false);

    await context.addCookies([{ name: "__Secure-wasl_review_session", value: original!.value, domain: "127.0.0.1", path: "/app", httpOnly: true, secure: true, sameSite: "Strict" }]);
    let before = reviewGets;
    await page.goto(desk);
    await expect(page.getByRole("heading", { name: "Sign in to review" })).toBeVisible();
    expect(reviewGets).toBe(before);

    await page.getByRole("link", { name: "Sign in with OIDC" }).click();
    await expect(page.getByTestId("question-cards")).toBeVisible();
    const rotated = (await context.cookies()).find((cookie) => cookie.name === "__Secure-wasl_review_session");
    expect(rotated?.value).not.toBe(original!.value);

    const db = new DatabaseSync(sessionDb);
    db.prepare("UPDATE review_session SET expires_at = 0 WHERE session_id_hash = (SELECT session_id_hash FROM review_session WHERE revoked_at IS NULL ORDER BY created_at DESC LIMIT 1)").run();
    db.close();
    before = reviewGets;
    await page.goto(desk);
    await expect(page.getByRole("heading", { name: "Sign in to review" })).toBeVisible();
    await expect(page.getByTestId("question-cards")).toHaveCount(0);
    expect(reviewGets).toBe(before);
    await context.close();
  });
});
