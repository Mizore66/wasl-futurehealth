import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Browser, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Independent double review against a stateful synthetic mock of the Go API.
 *
 * Start `next start` yourself with WASL_API_URL=http://127.0.0.1:3297 and the four role tokens
 * (WASL_API_REVIEWER_TOKEN, WASL_API_CLINICIAN_TOKEN, WASL_API_AUDITOR_TOKEN, WASL_API_COORDINATOR_TOKEN), then run this
 * spec with REVIEW_INDEPENDENT_MOCK=1, PLAYWRIGHT_BASE_URL pointing at that server, the same four tokens, and
 * REVIEW_R1_TOKEN, REVIEW_R2_TOKEN, REVIEW_SENIOR_TOKEN (each 16+ characters, all distinct). The mock binds every token to
 * an actor like the real API, blinds r1 from other actors until r2, and records each GET so the spec can prove the
 * auditor history is never requested while an initial read is concealed. It is a mock: the Go handler is pinned by its own tests.
 */
const ON = !!process.env.REVIEW_INDEPENDENT_MOCK;
const MOCK_PORT = 3297;
const holdId = fixtures.fixtures.review_hold.id;
const desk = `/app/review/${encodeURIComponent(holdId)}`;
const LINK = "rv-636161dc42837e08";
const EVIDENCE = "rv-d55f7ccf9f12c409";

type Kind = "r1" | "r2" | "senior";
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

test.describe("independent double review, live mock", () => {
  test.skip(!ON, "set REVIEW_INDEPENDENT_MOCK=1 with the server pointed at the mock");
  test.describe.configure({ mode: "serial" });

  const tokens = {
    r1: process.env.REVIEW_R1_TOKEN ?? "",
    r2: process.env.REVIEW_R2_TOKEN ?? "",
    senior: process.env.REVIEW_SENIOR_TOKEN ?? "",
  };
  const principals = new Map<string, Principal>();
  const questions: Record<string, { question: "link" | "evidence"; reads: Read[] }> = {
    [LINK]: { question: "link", reads: [] },
    [EVIDENCE]: { question: "evidence", reads: [] },
  };
  let server: Server;
  let auditRequests = 0;
  let sharedReviewGets = 0;
  const posts: { token: string; headers: Record<string, unknown>; body: Record<string, unknown> }[] = [];

  const hash = (item: string, slot: string) => `${slot}${item.slice(3, 11)}abcdef0123456789`;

  function projection(p: Principal) {
    return Object.entries(questions).map(([item, q]) => {
      const [r1, r2, r3] = q.reads;
      const shown = (r: Read | undefined, mine: boolean) => ({ read: r!.read, submitted: true, ...(mine ? { decision: r!.decision, hash: r!.hash } : {}) });
      if (!r1) return { item, question: q.question, state: "awaiting_r1", head: "", next_read: "r1", may_submit: p.kind !== "senior", reads: [{ read: "r1", submitted: false }], disagreement: false, canonical_unchanged: true };
      if (!r2)
        return {
          item, question: q.question, state: "awaiting_r2", head: r1.hash, next_read: "r2", may_submit: p.kind !== "senior" && p.actor !== r1.actor,
          reads: [shown(r1, p.actor === r1.actor), { read: "r2", submitted: false }], disagreement: false, canonical_unchanged: true,
        };
      const disagree = r1.decision !== r2.decision;
      const both = [shown(r1, true), shown(r2, true)];
      if (!r3 && !disagree)
        return { item, question: q.question, state: "resolved_agreed", head: r2.hash, next_read: null, may_submit: false, reads: both, disagreement: false, final_decision: r1.decision, resolution_basis: "agreement", canonical_unchanged: true };
      if (!r3)
        return { item, question: q.question, state: "awaiting_r3", head: r2.hash, next_read: "r3", may_submit: p.kind === "senior", reads: [...both, { read: "r3", submitted: false }], disagreement: true, canonical_unchanged: true };
      return { item, question: q.question, state: "resolved_senior", head: r3.hash, next_read: null, may_submit: false, reads: [...both, shown(r3, true)], disagreement: true, final_decision: r3.decision, resolution_basis: "senior", canonical_unchanged: true };
    });
  }

  const event = (item: string, r: Read) => ({
    case: holdId, item, decision: r.decision, note: "", role: r.role, actor: r.actor, action: questions[item].question === "link" ? "signoff_link" : "signoff_evidence",
    at: r.at, mode: "api", prev: r.prev, hash: r.hash, protocol: "wasl-independent-review/1", read: r.read, question: questions[item].question,
  });
  /** Viewer-facing ledger: another actor's r1 is omitted until r2 exists. */
  const ledger = (p: Principal) =>
    Object.entries(questions).flatMap(([item, q]) => q.reads.filter((r) => !(r.read === "r1" && q.reads.length === 1 && r.actor !== p.actor)).map((r) => event(item, r)));

  test.beforeAll(async () => {
    for (const [kind, token] of Object.entries(tokens)) {
      if (token.length < 16) throw new Error(`REVIEW_${kind.toUpperCase()}_TOKEN must be set (16+ characters)`);
      principals.set(token, { kind: kind as Kind, actor: `synthetic-${kind}` });
    }
    for (const role of ["REVIEWER", "CLINICIAN", "AUDITOR", "COORDINATOR"]) {
      const token = process.env[`WASL_API_${role}_TOKEN`]?.trim();
      if (!token) throw new Error(`WASL_API_${role}_TOKEN must be set, with the value the server sends`);
      principals.set(token, { kind: "r1", actor: `shared-${role.toLowerCase()}` });
    }
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
          const head = q.reads.at(-1)?.hash ?? "";
          if (!slot || body.read !== slot || body.expected_head !== head) return send(409, { meta, error: "stale head or wrong slot", current_head: head });
          const disagree = q.reads.length === 2 && q.reads[0].decision !== q.reads[1].decision;
          if ((slot === "r3") !== (who.kind === "senior") || (slot === "r3" && !disagree)) return send(403, { meta, error: "not authorised for this slot" });
          if (slot === "r2" && q.reads[0].actor === who.actor) return send(403, { meta, error: "same actor" });
          const r: Read = { read: slot, actor: who.actor, decision: body.decision, hash: hash(body.item, slot), role: who.kind === "senior" ? "senior_reviewer" : "reviewer", at: "2026-11-12T10:00:00Z", prev: head || "genesis" };
          q.reads.push(r);
          send(201, { meta, review: event(body.item, r), api_review_states: projection(who) });
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
        if (!["r1", "r2", "senior"].some((k) => tokens[k as Kind] === token)) sharedReviewGets += 1;
        const detail = read("case_review_hold.json");
        return send(200, { ...detail, case: { ...detail.case, closed: false, review: "open" }, signoffs: [], api_reviews: ledger(who), api_review_states: projection(who) });
      }
      return send(404, { meta, error: "not found" });
    });
    await new Promise<void>((resolve) => server.listen(MOCK_PORT, "127.0.0.1", resolve));
  });

  test.afterAll(async () => {
    await new Promise((resolve) => server.close(resolve));
  });

  async function viewer(browser: Browser, kind: Kind) {
    const context = await browser.newContext({ baseURL: process.env.PLAYWRIGHT_BASE_URL });
    const page = await context.newPage();
    await page.goto(desk);
    await page.getByLabel("Review token").fill(tokens[kind]);
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByTestId("question-cards")).toBeVisible();
    return { context, page };
  }
  const rail = (page: Page) => page.getByRole("complementary", { name: "Your answers" });
  const card = (page: Page, question: "link" | "evidence") => rail(page).locator(`section[data-question="${question}"]`);

  test("an unauthenticated viewer gets a token form, no credential choice and no case data", async ({ page }) => {
    await page.goto(desk);
    await expect(page.getByRole("heading", { name: "Sign in to review" })).toBeVisible();
    await expect(page.getByLabel("Review token")).toBeVisible();
    await expect(page.getByRole("radio")).toHaveCount(0);
    await expect(page.getByText("Mariam Al Shamsi")).toHaveCount(0);
    await expect(page.getByTestId("question-cards")).toHaveCount(0);
    await page.getByLabel("Review token").fill("short");
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByRole("alert").filter({ hasText: "Sign-in failed" })).toBeVisible();
    void 0;
    await noSeriousAxe(page);
    expect(sharedReviewGets).toBe(0);
  });

  test("r1 submits both questions, reloads, and stays waiting", async ({ browser }) => {
    const { context, page } = await viewer(browser, "r1");
    await expect(card(page, "link")).toContainText("First read due");
    await expect(card(page, "evidence")).toContainText("First read due");
    await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();

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
    await page.screenshot({ path: "test-results/screens/review-independent-resolved.png", fullPage: true });
    await noSeriousAxe(page);

    await page.getByRole("button", { name: "Sign out" }).click();
    await expect(page.getByRole("heading", { name: "Sign in to review" })).toBeVisible();
    await context.close();
    expect(sharedReviewGets).toBe(0);
  });
});
