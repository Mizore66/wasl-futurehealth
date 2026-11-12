import { readFileSync } from "node:fs";
import { spawn, type ChildProcess } from "node:child_process";
import { createServer, type Server } from "node:http";
import { createHash } from "node:crypto";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Browser, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Independent double review against a stateful synthetic mock of the Go API (pipeline/review_protocol.go shapes:
 * concealed:true on a hidden first read, one-way digest heads that never equal an event hash, resolution_basis
 * "agreement" / "senior_resolution", and both review_state and api_review_states on a 201).
 *
 * Self-contained: the spec starts its own mock API and its own `next start` (needs a prior `next build`, as the
 * ordinary Playwright command already does) with fixed synthetic tokens. Chromium project only, so other projects
 * never fight over the ports.
 */
const MOCK_PORT = 3297;
const NEXT_PORT = 3298;
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
type Lease = { actor: string; kind: Kind; read: "r1" | "r2" | "r3"; claimedAt: number; heartbeatAt: number; expiresAt: number; head: string; active: boolean };

const dir = path.join(process.cwd(), "data", "api");
const read = (f: string) => JSON.parse(readFileSync(path.join(dir, f), "utf8"));
const meta = read("fixtures.json").meta;

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

test.describe("independent double review, live mock", () => {
  test.beforeEach(({}, testInfo) => test.skip(testInfo.project.name !== "chromium", "owns fixed ports: Chromium only"));
  test.describe.configure({ mode: "serial" });

  const tokens = {
    r1: "synthetic-r1-token-aaaaaaaa",
    r2: "synthetic-r2-token-bbbbbbbb",
    senior: "synthetic-senior-token-cccc",
  };
  const principals = new Map<string, Principal>();
  const questions: Record<string, { question: "link" | "evidence"; reads: Read[]; claim?: Lease; claimEvents: Record<string, unknown>[] }> = {
    [LINK]: { question: "link", reads: [], claimEvents: [] },
    [EVIDENCE]: { question: "evidence", reads: [], claimEvents: [] },
  };
  let server: Server;
  let nextServer: ChildProcess | undefined;
  /** Mock switches for the interaction tests. */
  const salt = 0;
  let brokenBodyOnce = false;
  let failGetOnce = false;
  let auditRequests = 0;
  let sharedReviewGets = 0;
  let mockNow = Date.parse("2026-11-13T10:00:00Z");
  let claimSequence = 0;
  const posts: { token: string; headers: Record<string, unknown>; body: Record<string, unknown> }[] = [];

  const hash = (item: string, slot: string) => `${slot}${item.slice(3, 11)}abcdef0123456789`;
  /** The opaque head a write must present: "" initially, then a digest that never equals an event hash. */
  const headOf = (item: string) => (questions[item].reads.length === 0 ? "" : `digest-${salt}-${questions[item].reads.length}-${item.slice(3, 9)}`);

  function slotOf(item: string): "r1" | "r2" | "r3" | undefined {
    const reads = questions[item].reads;
    if (reads.length === 0) return "r1";
    if (reads.length === 1) return "r2";
    if (reads.length === 2 && reads[0].decision !== reads[1].decision) return "r3";
    return undefined;
  }
  function eligible(p: Principal, item: string, slot: "r1" | "r2" | "r3" | null | undefined): boolean {
    if (!slot || p.kind === "web") return false;
    if (slot === "r3") return p.kind === "senior";
    if (p.kind === "senior") return false;
    return slot !== "r2" || questions[item].reads[0].actor !== p.actor;
  }
  function baseProjection(p: Principal) {
    return Object.entries(questions).map(([item, q]) => {
      const [r1, r2, r3] = q.reads;
      const shown = (r: Read | undefined, mine: boolean) => ({ read: r!.read, submitted: true, ...(mine ? { decision: r!.decision, hash: r!.hash } : { concealed: true }) });
      if (!r1) return { item, question: q.question, state: "awaiting_r1", head: headOf(item), next_read: "r1" as const, may_submit: false, reads: [{ read: "r1", submitted: false }], disagreement: false, final_decision: "", resolution_basis: "", canonical_unchanged: true };
      if (!r2)
        return {
          item, question: q.question, state: "awaiting_r2", head: headOf(item), next_read: "r2" as const, may_submit: false,
          reads: [shown(r1, p.actor === r1.actor), { read: "r2", submitted: false }], disagreement: false, canonical_unchanged: true,
        };
      const disagree = r1.decision !== r2.decision;
      const both = [shown(r1, true), shown(r2, true)];
      if (!r3 && !disagree)
        return { item, question: q.question, state: "resolved_agreed", head: headOf(item), next_read: null, may_submit: false, reads: both, disagreement: false, final_decision: r1.decision, resolution_basis: "agreement", canonical_unchanged: true };
      if (!r3)
        return { item, question: q.question, state: "awaiting_r3", head: headOf(item), next_read: "r3" as const, may_submit: false, reads: [...both, { read: "r3", submitted: false }], disagreement: true, final_decision: "", resolution_basis: "", canonical_unchanged: true };
      return { item, question: q.question, state: "resolved_senior", head: headOf(item), next_read: null, may_submit: false, reads: [...both, shown(r3, true)], disagreement: true, final_decision: r3.decision, resolution_basis: "senior_resolution", canonical_unchanged: true };
    });
  }
  function projection(p: Principal) {
    return baseProjection(p).map((state) => {
      const q = questions[state.item];
      const lease = q.claim?.active ? q.claim : undefined;
      const expired = lease ? mockNow >= lease.expiresAt : false;
      const owner = lease?.actor === p.actor;
      const allowed = eligible(p, state.item, state.next_read);
      const claim = lease ? {
        owner: lease.actor, role: lease.kind === "senior" ? "senior_reviewer" : "reviewer", read: lease.read,
        claimed_at: new Date(lease.claimedAt).toISOString(), heartbeat_at: new Date(lease.heartbeatAt).toISOString(), expires_at: new Date(lease.expiresAt).toISOString(), status: expired ? "expired" : "active",
      } : null;
      return {
        ...state,
        may_submit: Boolean(lease && !expired && owner && allowed && lease.read === state.next_read),
        claim_head: q.claim?.head ?? "", claim,
        claim_actions: {
          claim: Boolean(!lease && allowed), takeover: Boolean(lease && expired && allowed),
          renew: Boolean(lease && !expired && owner && allowed), release: Boolean(lease && !expired && owner && allowed),
        },
      };
    });
  }

  const event = (item: string, r: Read) => ({
    case: holdId, item, decision: r.decision, note: "", role: r.role, actor: r.actor, action: questions[item].question === "link" ? "signoff_link" : "signoff_evidence",
    at: r.at, mode: "api", prev: r.prev, hash: r.hash, protocol: "wasl-independent-review/1", read: r.read, question: questions[item].question, claim_head: questions[item].claim?.head ?? "",
  });
  /** Viewer-facing ledger: another actor's r1 is omitted until r2 exists. */
  const ledger = (p: Principal) =>
    Object.entries(questions).flatMap(([item, q]) => [
      ...q.claimEvents,
      ...q.reads.filter((r) => !(r.read === "r1" && q.reads.length === 1 && r.actor !== p.actor)).map((r) => event(item, r)),
    ]);

  test.beforeAll(async ({}, testInfo) => {
    if (testInfo.project.name !== "chromium") return;
    for (const [kind, token] of Object.entries(tokens)) {
      if (token.length < 16) throw new Error(`REVIEW_${kind.toUpperCase()}_TOKEN must be set (16+ characters)`);
      principals.set(token, { kind: kind as Kind, actor: `synthetic-${kind}` });
    }
    // Shared server credentials are bound to their own non-reader actors, never to r1/r2/senior.
    for (const [name, token] of Object.entries(SHARED)) principals.set(token, { kind: "web", actor: `shared-${name.toLowerCase()}` });
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
      if (req.method === "POST" && p === "/mock/advance") {
        let raw = "";
        req.on("data", (c) => (raw += c));
        req.on("end", () => {
          mockNow += Number(JSON.parse(raw).ms);
          send(200, { now: new Date(mockNow).toISOString() });
        });
        return;
      }
      if (req.method === "POST" && p.endsWith("/review/claim")) {
        let raw = "";
        req.on("data", (c) => (raw += c));
        req.on("end", () => {
          const body = JSON.parse(raw);
          const q = questions[body.item];
          if (!q || q.question !== body.question) return send(404, { meta, error: "unknown question" });
          const slot = slotOf(body.item);
          if (!eligible(who, body.item, slot)) return send(403, { meta, error: "not eligible for current read" });
          const expected = q.claim?.head ?? "";
          if (body.expected_claim_head !== expected) return send(409, { meta, error: "stale claim head", conflict: "claim_head", current_claim_head: expected });
          const old = q.claim?.active ? q.claim : undefined;
          const expired = old ? mockNow >= old.expiresAt : false;
          if (body.operation === "claim" && old) return send(409, { meta, error: expired ? "takeover required" : "claim active", conflict: expired ? "claim_expired" : "claim_active" });
          if (body.operation === "takeover" && (!old || !expired)) return send(409, { meta, error: "claim is not expired", conflict: old ? "claim_active" : "claim_required" });
          if ((body.operation === "renew" || body.operation === "release") && !old) return send(409, { meta, error: "no active claim", conflict: "claim_required" });
          if ((body.operation === "renew" || body.operation === "release") && old?.actor !== who.actor) return send(403, { meta, error: "not claim owner" });
          if ((body.operation === "renew" || body.operation === "release") && expired) return send(409, { meta, error: "claim expired", conflict: "claim_expired" });
          if (!slot) return send(409, { meta, error: "question terminal", conflict: "review_state" });
          const at = mockNow;
          const previous = old!;
          const head = createHash("sha256").update(`${body.item}:${++claimSequence}`).digest("hex");
          const lease: Lease = {
            actor: who.actor, kind: who.kind, read: slot,
            claimedAt: body.operation === "renew" || body.operation === "release" ? previous.claimedAt : at,
            heartbeatAt: body.operation === "release" ? previous.heartbeatAt : at,
            expiresAt: body.operation === "release" ? previous.expiresAt : at + 300_000,
            head, active: body.operation !== "release",
          };
          q.claim = lease;
          const claimEvent = {
            type: "review_claim", kind: body.operation, case: holdId, item: body.item, question: body.question, read: slot,
            role: who.kind === "senior" ? "senior_reviewer" : "reviewer", actor: who.actor, action: body.question === "link" ? "signoff_link" : "signoff_evidence",
            at: new Date(at).toISOString(), claimed_at: new Date(lease.claimedAt).toISOString(), heartbeat_at: new Date(lease.heartbeatAt).toISOString(),
            expires_at: new Date(lease.expiresAt).toISOString(), expected_claim_head: expected, mode: "demo", prev: "genesis", hash: `claim-event-${claimSequence}`,
          };
          q.claimEvents.push(claimEvent);
          const states = projection(who);
          send(201, { meta, claim_event: claimEvent, review_state: states.find((m) => m.item === body.item), api_review_states: states, note: "claim appended" });
        });
        return;
      }
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
          const lease = q.claim?.active ? q.claim : undefined;
          if (!lease) return send(409, { meta, error: "claim required", conflict: "claim_required" });
          if (mockNow >= lease.expiresAt) return send(409, { meta, error: "claim expired", conflict: "claim_expired" });
          if (body.claim_head !== lease.head) return send(409, { meta, error: "stale claim", conflict: "claim_head", current_claim_head: lease.head });
          if (lease.actor !== who.actor || lease.read !== slot) return send(403, { meta, error: "not claim owner" });
          const r: Read = { read: slot, actor: who.actor, decision: body.decision, hash: hash(body.item, slot), role: who.kind === "senior" ? "senior_reviewer" : "reviewer", at: "2026-11-12T10:00:00Z", prev: q.reads.at(-1)?.hash || "genesis" };
          q.reads.push(r);
          q.claim!.active = false;
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
      if (p === "/api/cases") {
        const detail = read("case_review_hold.json");
        return send(200, { meta, cases: [{ ...detail.case, closed: false, review: "open" }], total: 1, limit: 50, offset: 0 });
      }
      if (p.endsWith("/undo")) return send(404, { meta, error: "not found" });
      if (p.endsWith("/audit")) {
        auditRequests += 1;
        const audit = read("case_review_hold_audit.json");
        // The auditor's redacted history: decisions are present, actor and note are not. It would leak a concealed r1.
        const history = Object.entries(questions).flatMap(([item, q]) => [
          ...q.claimEvents.map((ev) => Object.fromEntries(Object.entries(ev).filter(([key]) => key !== "actor" && key !== "note"))),
          ...q.reads.map((r) => { const { actor: _a, note: _n, ...rest } = event(item, r); void _a; void _n; return rest; }),
        ]);
        return send(200, { ...audit, history });
      }
      if (p === `/api/cases/${holdId}`) {
        if (failGetOnce) {
          failGetOnce = false;
          return send(500, { meta, error: "temporary" });
        }
        if (!["r1", "r2", "senior"].some((k) => tokens[k as Reader] === token)) sharedReviewGets += 1;
        const detail = read("case_review_hold.json");
        return send(200, { ...detail, case: { ...detail.case, closed: false, review: "open" }, signoffs: [], api_reviews: ledger(who), api_review_states: projection(who) });
      }
      return send(404, { meta, error: "not found" });
    });
    await new Promise<void>((resolve) => server.listen(MOCK_PORT, "127.0.0.1", resolve));
    nextServer = spawn(path.join(process.cwd(), "node_modules", ".bin", "next"), ["start", "-p", String(NEXT_PORT)], {
      env: { ...process.env, ...SHARED, WASL_API_URL: `http://127.0.0.1:${MOCK_PORT}` },
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
  });

  async function viewer(browser: Browser, kind: Reader) {
    const context = await browser.newContext();
    const page = await context.newPage();
    await page.goto(desk);
    await page.getByLabel("Review token").fill(tokens[kind]);
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByTestId("question-cards")).toBeVisible();
    return { context, page };
  }
  const rail = (page: Page) => page.getByRole("complementary", { name: "Your answers" });
  const card = (page: Page, question: "link" | "evidence") => rail(page).locator(`section[data-question="${question}"]`);
  async function acquire(page: Page, question: "link" | "evidence", slot: "first" | "second" | "senior" = "first") {
    const label = slot === "senior" ? "Claim senior resolution" : `Claim ${slot} read`;
    await card(page, question).getByRole("button", { name: label, exact: true }).click();
    await expect(card(page, question)).toContainText("Claimed by");
  }

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

  test("two actors see one owner, renew, release, expire and take over without a stale write", async ({ browser }) => {
    const a = await viewer(browser, "r1");
    const b = await viewer(browser, "r2");
    await acquire(a.page, "link");
    await b.page.reload();
    await expect(card(b.page, "link")).toContainText("Claimed by synthetic-r1 until");
    await expect(card(b.page, "link").getByRole("radio")).toHaveCount(0);
    await expect(card(b.page, "link").getByRole("button", { name: "Claim first read" })).toHaveCount(0);

    await a.page.goto(`${NEXT_URL}/app/review`);
    const queueLink = a.page.locator(`[data-queue-question="${LINK}:link"]`);
    await expect(queueLink).toContainText("Claimed by synthetic-r1 until");
    await expect(queueLink.getByRole("link", { name: "Renew or release" })).toHaveAttribute("href", `/app/review/${encodeURIComponent(holdId)}#review-question-${LINK}-link`);
    await a.page.goto(desk);
    await card(a.page, "link").getByRole("button", { name: "Renew claim" }).click();
    await card(a.page, "link").getByRole("button", { name: "Release claim" }).click();
    await b.page.reload();
    await acquire(b.page, "link");
    const bHead = questions[LINK].claim!.head;
    await b.page.request.post(`http://127.0.0.1:${MOCK_PORT}/mock/advance`, { data: { ms: 300_000 } });
    const expiredWrite = await a.page.request.post(`http://127.0.0.1:${MOCK_PORT}/api/cases/${encodeURIComponent(holdId)}/review`, {
      headers: { authorization: `Bearer ${tokens.r1}`, "content-type": "application/json" },
      data: { item: LINK, decision: "same", note: "", read: "r1", expected_head: "", claim_head: bHead },
    });
    expect(expiredWrite.status()).toBe(409);
    await b.page.reload();
    await expect(card(b.page, "link")).toContainText("expired at");
    await card(b.page, "link").getByRole("button", { name: "Take over claim" }).click();
    const takenHead = questions[LINK].claim!.head;
    const staleWrite = await a.page.request.post(`http://127.0.0.1:${MOCK_PORT}/api/cases/${encodeURIComponent(holdId)}/review`, {
      headers: { authorization: `Bearer ${tokens.r1}`, "content-type": "application/json" },
      data: { item: LINK, decision: "same", note: "", read: "r1", expected_head: "", claim_head: takenHead },
    });
    expect([403, 409]).toContain(staleWrite.status());
    await card(b.page, "link").getByRole("button", { name: "Release claim" }).click();
    await noSeriousAxe(b.page);
    await a.context.close();
    await b.context.close();
  });

  test("r1 submits both questions, reloads, and stays waiting", async ({ browser }) => {
    const { context, page } = await viewer(browser, "r1");
    await expect(card(page, "link")).toContainText("First read due");
    await expect(card(page, "evidence")).toContainText("First read due");
    await expect(rail(page).getByRole("button", { name: "Sign out" })).toBeVisible();

    await acquire(page, "link");
    await card(page, "link").getByRole("radio", { name: "Same patient", exact: true }).click();
    await card(page, "link").getByRole("button", { name: "Submit first read" }).click();
    await expect(card(page, "link")).toContainText("Your first read is in");
    await expect(card(page, "link")).toContainText("First read: Same patient");
    // Only that question changed; the evidence draft area is untouched.
    await expect(card(page, "evidence")).toContainText("First read due");

    await acquire(page, "evidence");
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
    expect(posts.filter((x) => "decision" in x.body).slice(-2).map((x) => [x.body.item, x.body.read, x.body.expected_head])).toEqual([[LINK, "r1", ""], [EVIDENCE, "r1", ""]]);
    // The credential is the bearer token only: no identity header, no role in the body.
    for (const post of posts.filter((x) => "decision" in x.body)) {
      expect(post.headers["x-wasl-role"]).toBeUndefined();
      expect(post.headers["x-wasl-actor"]).toBeUndefined();
      expect(Object.keys(post.body).sort()).toEqual(["claim_head", "decision", "expected_head", "item", "note", "read"]);
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

    await acquire(page, "link", "second");
    await card(page, "link").getByRole("radio", { name: "Same patient", exact: true }).click();
    await card(page, "link").getByRole("button", { name: "Submit second read" }).click();
    await expect(card(page, "link")).toContainText("Resolved: both reads agree");
    await expect(card(page, "link")).toContainText("No senior form is needed");
    await expect(card(page, "link").getByRole("button", { name: /Submit/ })).toHaveCount(0);

    await acquire(page, "evidence", "second");
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

    await acquire(page, "evidence", "senior");
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

    /* One sign-out on the page: the desk's. The governance history card does not repeat it. */
    await expect(page.getByRole("button", { name: "Sign out" })).toHaveCount(1);
    await rail(page).getByRole("button", { name: "Sign out" }).click();
    await expect(page.getByRole("heading", { name: "Sign in to review" })).toBeVisible();
    await context.close();
    expect(sharedReviewGets).toBe(0);
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
    for (const q of Object.values(questions)) { q.reads.length = 0; q.claim = undefined; q.claimEvents.length = 0; }
    const { context, page } = await viewer(browser, "r1");
    await expect(card(page, "link")).toContainText("First read due");
    await acquire(page, "link");
    // Another actor lands a first read after this page rendered: next_read moves from r1 to r2 and consumes its lease.
    questions[LINK].reads.push({ read: "r1", actor: "synthetic-r2", decision: "different", hash: hash(LINK, "r1"), role: "reviewer", at: "2026-11-12T10:00:00Z", prev: "genesis" });
    questions[LINK].claim = undefined;
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
    for (const q of Object.values(questions)) { q.reads.length = 0; q.claim = undefined; q.claimEvents.length = 0; }
    const { context, page } = await viewer(browser, "r1");
    await acquire(page, "link");
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
    for (const q of Object.values(questions)) { q.reads.length = 0; q.claim = undefined; q.claimEvents.length = 0; }
    const patient = `${NEXT_URL}/app/patients/${encodeURIComponent(holdId)}`;
    const audit = `${patient}/audit`;
    // r1 submits; the cookie (path /app) must also reach the audit page.
    const r1 = await viewer(browser, "r1");
    await acquire(r1.page, "link");
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
    await acquire(second.page, "link", "second");
    await card(second.page, "link").getByRole("radio", { name: "Not the same patient" }).click();
    await card(second.page, "link").getByRole("button", { name: "Submit second read" }).click();
    await expect(card(second.page, "link")).toContainText("waiting for a senior");
    await second.context.close();
    const sr = await viewer(browser, "senior");
    await acquire(sr.page, "link", "senior");
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
});
