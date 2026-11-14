import { spawn, type ChildProcess } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync, rmSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import { expect, test, type Browser, type Page } from "@playwright/test";
import { CaseDetail, ReviewClaimRequest, ReviewRequest, ReviewState, type ApiReview, type ReviewClaimEvent } from "../lib/api/schema";
import { fixtureApplication, purposeFixture, PURPOSES, type Purpose } from "./review-purpose-fixtures";

/**
 * Issue #111: actual routes and hydrated ReviewDesk, not helper/SSR assertions. The standard chromium project discovers
 * every test below without REVIEW_LIVE_MOCK, an external API, role headers, production overrides, or skipped cases.
 * Like review-independent.spec.ts, this uses next start over the standard CI build and a local synthetic bearer API.
 * The fixture-only server has an isolated fs manifest; the live mock never falls back to it on an API/auth error.
 * This is browser/UI acceptance with synthetic inputs, not execution of the Go closure engine or clinical validation.
 */
const MOCK_PORT = 3497;
const PREVIEW_URL = "http://127.0.0.1:3498";
const LIVE_URL = "http://127.0.0.1:3499";
const SHARED = { WASL_API_WEB_REVIEWER_TOKEN: "synthetic-purpose-shared-web", WASL_API_AUDITOR_TOKEN: "synthetic-purpose-shared-audit" };
const CLINICIANS = ["synthetic-purpose-clinician-one", "synthetic-purpose-clinician-two"] as const;
const ANSWERS = ["confirm", "disagree"] as const;
type Answer = (typeof ANSWERS)[number];
type Principal = { actor: string; role: "clinician" | "web_reviewer" | "auditor" };
type SavedRead = { read: "r1" | "r2"; actor: string; decision: string; hash: string; event: ApiReview };
type Scenario = {
  canonical: CaseDetail;
  reads: SavedRead[];
  events: (ApiReview | ReviewClaimEvent)[];
  claim: ReviewState["claim"];
  claimHead: string;
  gets: number;
};
const key = (purpose: Purpose, answer: Answer) => `${purpose}:${answer}`;
const href = (base: string, detail: CaseDetail) => `${base}/app/review/${encodeURIComponent(detail.case.id)}`;
const rail = (page: Page) => page.getByRole("complementary", { name: "Your answers" });
const card = (page: Page) => rail(page).locator('section[data-question="evidence"]');
const summary = (page: Page) => page.locator("main > header").first();
const digest = (value: unknown) => createHash("sha256").update(JSON.stringify(value)).digest("hex");
const fixtures = PURPOSES.map((purpose) => purposeFixture(purpose));
const scenarios = new Map<string, Scenario>();
for (const purpose of PURPOSES) for (const answer of ANSWERS) scenarios.set(key(purpose, answer), { canonical: purposeFixture(purpose, answer), reads: [], events: [], claim: null, claimHead: "", gets: 0 });
const principals = new Map<string, Principal>([
  ...CLINICIANS.map((token, i): [string, Principal] => [token, { actor: `synthetic-clinician-${i + 1}`, role: "clinician" }]),
  [SHARED.WASL_API_WEB_REVIEWER_TOKEN, { actor: "synthetic-shared-web", role: "web_reviewer" }],
  [SHARED.WASL_API_AUDITOR_TOKEN, { actor: "synthetic-shared-audit", role: "auditor" }],
]);

function projection(scenario: Scenario, who: Principal): ReviewState {
  const { reads, claim } = scenario;
  const next = reads.length === 0 ? "r1" : reads.length === 1 ? "r2" : null;
  const eligible = who.role === "clinician" && next !== null && reads.every((read) => read.actor !== who.actor);
  const owner = claim?.owner === who.actor;
  return ReviewState.parse({
    item: scenario.canonical.review_items[0].item, question: "evidence",
    state: next === "r1" ? "awaiting_r1" : next === "r2" ? "awaiting_r2" : "resolved_agreed",
    head: reads.length ? digest(reads.map((read) => read.hash)) : "", next_read: next,
    may_submit: Boolean(eligible && claim && owner), claim_head: scenario.claimHead, claim,
    claim_actions: { claim: eligible && !claim, renew: Boolean(eligible && owner), release: Boolean(eligible && owner), takeover: false },
    reads: [...reads.map((read) => ({ read: read.read, submitted: true, ...(reads.length > 1 || read.actor === who.actor ? { decision: read.decision, hash: read.hash } : { concealed: true }) })), ...(next ? [{ read: next, submitted: false }] : [])],
    disagreement: false, final_decision: next ? "" : reads[0].decision, resolution_basis: next ? "" : "agreement", canonical_unchanged: true,
  });
}

function liveDetail(scenario: Scenario, who: Principal): CaseDetail {
  const hidden = scenario.reads.length === 1 && scenario.reads[0].actor !== who.actor ? scenario.reads[0].hash : undefined;
  return CaseDetail.parse({ ...scenario.canonical, api_reviews: scenario.events.filter((event) => event.hash !== hidden), api_review_states: [projection(scenario, who)] });
}

let mock: Server | undefined;
let fixtureRoot: string | undefined;
const children: ChildProcess[] = [];
const logs: string[] = [];
async function startNext(base: string, cwd: string, live: boolean) {
  const child = spawn(path.join(process.cwd(), "node_modules", ".bin", "next"), ["start", "-p", new URL(base).port, "-H", "127.0.0.1"], {
    cwd, env: { ...process.env, ...SHARED, WASL_REVIEW_AUTH_MODE: "demo", WASL_API_URL: live ? `http://127.0.0.1:${MOCK_PORT}` : "" }, stdio: ["ignore", "pipe", "pipe"],
  });
  children.push(child);
  child.stdout?.on("data", (chunk) => { logs.push(String(chunk)); });
  child.stderr?.on("data", (chunk) => { logs.push(String(chunk)); });
  for (let i = 0; i < 120; i++) {
    if (child.exitCode !== null) throw new Error(`Purpose browser server exited ${child.exitCode}: ${logs.join("")}`);
    if (await fetch(`${base}/app/review`).then((response) => response.ok, () => false)) return;
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`Purpose browser server not ready: ${logs.join("")}`);
}

// One worker owns these ports; scenarios are independent, so a failed case must not skip the other acceptance cases.
test.describe.configure({ mode: "default" });
test.beforeAll(async () => {
  test.setTimeout(120_000);
  mock = createServer(async (req, res) => {
    const send = (status: number, value: unknown) => { res.writeHead(status, { "content-type": "application/json" }); res.end(JSON.stringify(value)); };
    const route = decodeURIComponent(new URL(req.url ?? "/", "http://synthetic").pathname);
    const token = req.headers.authorization?.replace(/^Bearer /, "") ?? "";
    const who = principals.get(token);
    const meta = fixtures[0].meta;
    if (!who) return send(401, { meta, error: "synthetic bearer authentication required" });
    if (req.method === "GET" && route === "/api/cases") return send(200, { meta, cases: [...scenarios.values()].map((scenario) => scenario.canonical.case), total: scenarios.size, limit: 50, offset: 0 });
    if (req.method === "GET" && route === "/api/fixtures") return send(200, { meta, fixtures: { evidence_closure: fixtures[0].case, false_closure_trap: fixtures[0].case, review_hold: fixtures[1].case } });
    if (req.method === "GET" && route === "/api/dashboard") return send(200, JSON.parse(readFileSync(path.join(process.cwd(), "data/api/dashboard.json"), "utf8")));
    const scenario = [...scenarios.values()].find((entry) => route === `/api/cases/${entry.canonical.case.id}` || route.startsWith(`/api/cases/${entry.canonical.case.id}/`));
    if (!scenario) return send(404, { meta, error: "unknown synthetic route" });
    if (req.method === "GET" && route === `/api/cases/${scenario.canonical.case.id}`) { scenario.gets++; return send(200, liveDetail(scenario, who)); }
    // Do not fabricate a verified audit or governance history for this UI-only fixture. Existing suites cover those paths.
    if (req.method === "GET") return send(404, { meta, error: "not provided by purpose-only synthetic mock" });
    if (who.role !== "clinician") return send(403, { meta, error: "token is not bound to a clinician" });
    let raw = "";
    for await (const chunk of req) raw += String(chunk);
    let body: unknown;
    try { body = JSON.parse(raw); } catch { return send(400, { meta, error: "invalid JSON" }); }
    const state = projection(scenario, who);
    const item = scenario.canonical.review_items[0].item;
    const at = new Date().toISOString();
    const prev = scenario.events.at(-1)?.hash ?? "genesis";
    if (route.endsWith("/review/claim")) {
      const parsed = ReviewClaimRequest.safeParse(body);
      if (!parsed.success || parsed.data.item !== item || parsed.data.question !== "evidence") return send(400, { meta, error: "invalid synthetic claim" });
      const input = parsed.data;
      if (!state.next_read || !state.claim_actions?.[input.operation]) return send(403, { meta, error: "not eligible for this claim" });
      if (input.expected_claim_head !== scenario.claimHead) return send(409, { meta, error: "stale claim head", conflict: "claim_head", current_claim_head: scenario.claimHead });
      const expires = new Date(Date.now() + 300_000).toISOString();
      const event: ReviewClaimEvent = { type: "review_claim", kind: input.operation, case: scenario.canonical.case.id, item, question: "evidence", read: state.next_read, actor: who.actor, role: who.role, action: "signoff_evidence", at, claimed_at: at, heartbeat_at: at, expires_at: expires, expected_claim_head: scenario.claimHead, mode: "demo", prev, hash: digest({ prev, input, actor: who.actor, at }) };
      scenario.events.push(event);
      scenario.claimHead = event.hash;
      scenario.claim = input.operation === "release" ? null : { owner: who.actor, role: who.role, read: state.next_read, claimed_at: at, heartbeat_at: at, expires_at: expires, status: "active" };
      const fresh = projection(scenario, who);
      return send(201, { meta, claim_event: event, review_state: fresh, api_review_states: [fresh], note: "Synthetic demo claim appended" });
    }
    if (route.endsWith("/review")) {
      const parsed = ReviewRequest.safeParse(body);
      if (!parsed.success || parsed.data.item !== item || !["adequate", "inadequate"].includes(parsed.data.decision)) return send(400, { meta, error: "invalid evidence answer" });
      const input = parsed.data;
      if (!state.may_submit || !scenario.claim || scenario.claim.owner !== who.actor) return send(403, { meta, error: "claim belongs to a different actor or actor already answered" });
      if (input.read !== state.next_read || input.expected_head !== state.head || input.claim_head !== scenario.claimHead) return send(409, { meta, error: "stale review/claim head", conflict: "review_state", current_head: state.head });
      if (Date.parse(scenario.claim.expires_at) <= Date.now()) return send(409, { meta, error: "claim expired", conflict: "claim_expired" });
      // This purpose suite only needs two matching independent answers. Senior/disagreement protocol stays in the existing suite.
      if (scenario.reads[0] && scenario.reads[0].decision !== input.decision) return send(400, { meta, error: "use review-independent.spec.ts for senior protocol scenarios" });
      const read = state.next_read as "r1" | "r2";
      const event: ApiReview = { case: scenario.canonical.case.id, item, decision: input.decision, note: input.note, actor: who.actor, role: who.role, action: "signoff_evidence", at, mode: "demo", prev, hash: digest({ prev, input, actor: who.actor, at }), protocol: "wasl-independent-review/1", question: "evidence", read, claim_head: scenario.claimHead };
      scenario.events.push(event);
      scenario.reads.push({ read, actor: who.actor, decision: input.decision, hash: event.hash, event });
      scenario.claim = null;
      const fresh = projection(scenario, who);
      // In particular, no POST changes scenario.canonical, even when both readers answered.
      return send(201, { meta, review: event, review_state: fresh, api_review_states: [fresh], note: "Synthetic demo answer appended; canonical case unchanged" });
    }
    return send(404, { meta, error: "unsupported synthetic operation" });
  });
  await new Promise<void>((resolve, reject) => { mock!.once("error", reject); mock!.listen(MOCK_PORT, "127.0.0.1", resolve); });
  fixtureRoot = fixtureApplication(fixtures);
  await startNext(PREVIEW_URL, fixtureRoot, false);
  await startNext(LIVE_URL, process.cwd(), true);
});
test.afterAll(async () => {
  await Promise.all(children.map(async (child) => {
    if (child.exitCode !== null || child.signalCode !== null) return;
    const exited = new Promise<void>((resolve) => child.once("exit", () => resolve()));
    child.kill();
    await exited;
  }));
  if (mock) await new Promise<void>((resolve, reject) => mock!.close((error) => error ? reject(error) : resolve()));
  if (fixtureRoot) rmSync(fixtureRoot, { recursive: true, force: true });
});

async function initialPurpose(page: Page, purpose: Purpose) {
  const text = purpose === "shadow_confirm"
    ? "Suggested automatic closure to confirm. The case is open. Confirming permits closure; a different answer is followed instead."
    : "Post-closure audit of an automatic closure. The case is closed. Agreeing leaves it closed; disagreeing reopens it.";
  await expect(summary(page)).toContainText(text);
  await expect(summary(page)).not.toContainText(/Held:|could not settle|readers disagree|Nothing closes without/i);
  await expect(page.getByText(/Nothing closes without/i)).toHaveCount(0);
  // Agreeing concrete reader fields, not just agree=true over the old disputed-field fixture.
  for (const field of ["Caecum reached", "Prep graded", "Tissue taken", "Histology back"]) {
    await expect(page.locator(`[data-check="${field}"]`)).toBeVisible();
    await expect(page.locator(`[data-check="${field}"]`)).not.toContainText(/reviewers decide|not stated/);
  }
  await expect(page.locator('[data-check="Prep graded"]')).toContainText("agreed");
  await expect(rail(page).getByRole("radio", { name: "Same patient", exact: true })).toHaveCount(0);
}

async function viewer(browser: Browser, scenario: Scenario, index: 0 | 1) {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(href(LIVE_URL, scenario.canonical));
  await expect(page.getByRole("heading", { name: "Sign in to review" })).toBeVisible();
  await expect(page.getByTestId("question-cards")).toHaveCount(0);
  await page.getByLabel("Review token").fill(CLINICIANS[index]);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByTestId("question-cards")).toBeVisible();
  return { context, page };
}

test("pilot-purpose queue shows distinct labels, instructions and canonical open/closed states", async ({ page }) => {
  await page.goto(`${PREVIEW_URL}/app/review`);
  await expect(page.getByText(/Nothing closes without/i)).toHaveCount(0);
  for (const [i, purpose] of PURPOSES.entries()) {
    const row = page.locator(`a[href="/app/review/${encodeURIComponent(fixtures[i].case.id)}"]`).filter({ hasText: purpose === "shadow_confirm" ? "Suggested closure" : "Post-closure audit" });
    await expect(row).toHaveCount(1);
    await expect(row).toContainText(purpose === "shadow_confirm" ? "The case is open" : "The case is closed");
    await row.click();
    await initialPurpose(page, purpose);
    await page.goto(`${PREVIEW_URL}/app/review`);
  }
});

for (const purpose of PURPOSES) for (const answer of ANSWERS) {
  const copy = purpose === "shadow_confirm"
    ? answer === "confirm"
      ? { button: "Confirm the suggested closure", heading: "Would confirm the suggested closure", explanation: "Confirming the automatic suggestion permits closure of the case." }
      : { button: "Sign: do not confirm closure", heading: "Would not confirm the suggested closure", explanation: "The suggestion is not confirmed, so the case follows your answer instead of closing automatically." }
    : answer === "confirm"
      ? { button: "Sign: audit agrees", heading: "Would leave the case closed", explanation: "Agreeing with the audit means the case stays closed." }
      : { button: "Sign: audit disagrees", heading: "Would reopen the case", explanation: "Disagreeing with the audit reopens the case." };
  test(`${purpose} / ${answer}: browser preview has purpose-specific outcome, no saved or canonical transition claim`, async ({ page }) => {
    const detail = fixtures[PURPOSES.indexOf(purpose)];
    await page.goto(href(PREVIEW_URL, detail));
    await initialPurpose(page, purpose);
    const posts: string[] = [];
    page.on("request", (request) => { if (request.method() === "POST") posts.push(request.url()); });
    const choice = rail(page).getByRole("radio", { name: answer === "confirm" ? "Adequate" : "Not adequate", exact: true });
    // Exercise native keyboard input and the production preview/focus behavior, not application-state injection.
    await choice.focus();
    await choice.press("Space");
    await expect(choice).toBeChecked();
    await rail(page).getByRole("button", { name: `Preview: ${copy.button}`, exact: true }).click();
    const heading = rail(page).getByRole("heading", { name: copy.heading, exact: true });
    await expect(heading).toBeVisible();
    await expect(heading).toBeFocused();
    const result = rail(page).getByRole("status");
    await expect(result).toContainText(copy.explanation);
    await expect(result).toContainText("Reconstruction: nothing was saved");
    await expect(result).toContainText("No event was appended and no hash exists");
    await expect(result).not.toContainText(/Would close the case|Closed with|Refreshed case data confirms/);
    await expect(rail(page).locator('li[data-kind="human"]')).toHaveCount(0);
    await initialPurpose(page, purpose);
    await rail(page).getByRole("button", { name: "Undo", exact: true }).click();
    await expect(rail(page).getByRole("radio", { name: "Adequate", exact: true })).toBeFocused();
    await page.reload();
    await initialPurpose(page, purpose);
    await expect(rail(page).getByRole("radio", { checked: true })).toHaveCount(0);
    expect(posts).toEqual([]);
  });

  test(`${purpose} / ${answer}: demo answers acknowledged, canonical unchanged until refreshed canonical result`, async ({ browser }) => {
    const scenario = scenarios.get(key(purpose, answer))!;
    const original = JSON.stringify(scenario.canonical);
    const a = await viewer(browser, scenario, 0);
    const b = await viewer(browser, scenario, 1);
    try {
      await initialPurpose(a.page, purpose);
      await card(a.page).getByRole("button", { name: "Claim first read", exact: true }).click();
      await expect(card(a.page)).toContainText("Claimed by synthetic-clinician-1");
      await card(a.page).getByRole("radio", { name: answer === "confirm" ? "Adequate" : "Not adequate", exact: true }).click();
      await card(a.page).getByRole("button", { name: "Submit first read", exact: true }).click();
      await expect(card(a.page)).toHaveAttribute("data-state", "awaiting_r2");
      await expect(card(a.page).getByRole("button", { name: "Claim second read", exact: true })).toHaveCount(0);
      await expect(a.page.getByTestId("read-announcement")).toContainText("Your first read for the evidence question was recorded.");
      await expect(card(a.page)).toContainText(answer === "confirm" ? "Report adequate" : "Report not adequate");
      await initialPurpose(a.page, purpose);
      expect(JSON.stringify(scenario.canonical)).toBe(original);
      const firstRead = scenario.reads[0];
      expect(firstRead.actor).toBe("synthetic-clinician-1");
      expect(firstRead.event.role).toBe("clinician");

      await b.page.reload();
      await expect(card(b.page)).toContainText("the answer stays hidden until the second read");
      await expect(card(b.page)).not.toContainText(firstRead.hash.slice(0, 12));
      await expect(card(b.page)).not.toContainText(/First read: Report (not )?adequate/);
      await expect(b.page.getByTestId("history-withheld")).toBeVisible();
      await card(b.page).getByRole("button", { name: "Claim second read", exact: true }).click();
      await card(b.page).getByRole("radio", { name: answer === "confirm" ? "Adequate" : "Not adequate", exact: true }).click();
      await card(b.page).getByRole("button", { name: "Submit second read", exact: true }).click();
      await expect(card(b.page)).toHaveAttribute("data-state", "resolved_agreed");
      await expect(b.page.getByTestId("final-evidence")).toContainText(answer === "confirm" ? "Report adequate" : "Report not adequate");
      await expect(card(b.page)).toContainText("Case data is unchanged: this is a demo overlay, not a canonical decision.");
      await expect(b.page.getByTestId("progress")).toContainText("Every question is resolved");
      await initialPurpose(b.page, purpose);
      expect(JSON.stringify(scenario.canonical)).toBe(original);
      expect(scenario.reads.map((read) => [read.actor, read.decision])).toEqual([
        ["synthetic-clinician-1", answer === "confirm" ? "adequate" : "inadequate"],
        ["synthetic-clinician-2", answer === "confirm" ? "adequate" : "inadequate"],
      ]);
      const savedEvents = JSON.stringify(scenario.events);
      const getsBefore = scenario.gets;
      await b.page.reload();
      await initialPurpose(b.page, purpose);
      await expect(card(b.page)).toHaveAttribute("data-state", "resolved_agreed");
      expect(scenario.gets).toBeGreaterThan(getsBefore);
      expect(JSON.stringify(scenario.events)).toBe(savedEvents);
      expect(JSON.stringify(scenario.canonical)).toBe(original);

      // Only an independently supplied canonical GET result may change the displayed current state. This is a synthetic
      // result replacement in the test API, not a POST side effect, injected React state, or proof the Go engine ran.
      if ((purpose === "shadow_confirm" && answer === "confirm") || (purpose === "auto_audit" && answer === "disagree")) {
        const closed = purpose === "shadow_confirm";
        scenario.canonical = CaseDetail.parse({ ...scenario.canonical, case: { ...scenario.canonical.case, closed, state: closed ? "closed" : "procedure_done", closed_day: closed ? 32 : 0 }, current: { ...scenario.canonical.current, state: closed ? "closed" : "procedure_done" } });
        const getsBeforeRefresh = scenario.gets;
        await b.page.reload();
        await expect(summary(b.page)).toContainText(`The case is ${closed ? "closed" : "open"}.`);
        expect(scenario.gets).toBeGreaterThan(getsBeforeRefresh);
        expect(JSON.stringify(scenario.events)).toBe(savedEvents);
        await expect(card(b.page)).toHaveAttribute("data-state", "resolved_agreed");
      }
    } finally { await a.context.close(); await b.context.close(); }
  });
}

test("purpose mock refuses unauthenticated or shared-role forged claims without appending events", async ({ request }) => {
  const scenario = scenarios.get(key("shadow_confirm", "disagree"))!;
  const before = JSON.stringify(scenario.events);
  const url = `http://127.0.0.1:${MOCK_PORT}/api/cases/${encodeURIComponent(scenario.canonical.case.id)}/review/claim`;
  const data = { item: scenario.canonical.review_items[0].item, question: "evidence", operation: "claim", expected_claim_head: scenario.claimHead };
  const anonymous = await request.post(url, { data, headers: { "X-Wasl-Role": "clinician", "X-Wasl-Actor": "synthetic-clinician-1" } });
  expect(anonymous.status()).toBe(401);
  const shared = await request.post(url, { data, headers: { authorization: `Bearer ${SHARED.WASL_API_WEB_REVIEWER_TOKEN}`, "X-Wasl-Role": "clinician", "X-Wasl-Actor": "synthetic-clinician-1" } });
  expect(shared.status()).toBe(403);
  expect(JSON.stringify(scenario.events)).toBe(before);
});
