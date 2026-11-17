import { createHash } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";

/*
 * Issue #167: the staff workflow to resolve a verified patient contest, in the browser, against a stateful synthetic
 * mock of the Go API and an isolated Next server. The mock mirrors pipeline/api_patient_request.go casePatientContest:
 * identity only from the bearer token, `resolve_patient_contest` for senior clinicians only, every original reviewer
 * of the case refused, a required reason, and a body of exactly {contest, outcome, reason}. The real handler, its
 * ledger replay and the engine hold are covered by pipeline/api_patient_contest_round2_test.go,
 * pipeline/api_patient_request_test.go and pipeline/patient_contest_feed_test.go. Synthetic cases only.
 */
const MOCK_PORT = Number(process.env.CONTEST_MOCK_PORT ?? 3393);
const NEXT_PORT = Number(process.env.CONTEST_NEXT_PORT ?? 3394);
const NEXT_URL = `http://127.0.0.1:${NEXT_PORT}`;
/* Server-held role credentials the Next server has for other screens. The contest review must never use one. */
const SHARED = {
  WASL_API_REVIEWER_TOKEN: "synthetic-shared-reviewer-0001",
  WASL_API_WEB_REVIEWER_TOKEN: "synthetic-web-reviewer-00001",
  WASL_API_CLINICIAN_TOKEN: "synthetic-shared-clinician-0001",
  WASL_API_AUDITOR_TOKEN: "synthetic-shared-auditor-00001",
  WASL_API_COORDINATOR_TOKEN: "synthetic-shared-coord-000001",
};
/* Each person's own assigned token, typed into the sign-in form; the Next server never holds them. */
const PEOPLE: Record<string, { role: string; actor: string }> = {
  "contest-senior-original-01": { role: "senior_clinician", actor: "dr-original" },
  "contest-senior-independent": { role: "senior_clinician", actor: "dr-independent" },
  "contest-clinician-00000001": { role: "clinician", actor: "dr-clinician" },
};
const ORIGINAL = "contest-senior-original-01";
const INDEPENDENT = "contest-senior-independent";
const CLINICIAN = "contest-clinician-00000001";

const dir = path.join(process.cwd(), "data", "api");
const base = JSON.parse(readFileSync(path.join(dir, "case_review_hold.json"), "utf8"));
const baseAudit = JSON.parse(readFileSync(path.join(dir, "case_review_hold_audit.json"), "utf8"));
const meta = base.meta;
const sha = (s: string) => createHash("sha256").update(s).digest("hex");

const CASE = "ahs-mushrif/ServiceRequest/AHS-sr-9167";
const RECEIPT = sha("synthetic-contest-receipt");
const DECISION = sha("synthetic-questioned-decision");
type Contest = {
  receipt: string;
  kind: "contest_submitted";
  decision: string;
  submitted_at: string;
  reason: string;
  requester: string;
  excluded_reviewers: string[];
  owner: string;
  due_at: string;
  status: "open" | "resolved";
  outcome?: string;
  resolved_at?: string;
  resolved_by?: string;
  resolution_reason?: string;
};
let contest: Contest;
let events: Record<string, unknown>[];
let posts: { token: string; keys: string[] }[];

function reset() {
  contest = {
    receipt: RECEIPT, kind: "contest_submitted", decision: DECISION, submitted_at: "2026-10-01T08:00:00Z",
    reason: "I do not understand why my check is finished (synthetic).", requester: "patient-link/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    excluded_reviewers: ["dr-original"], owner: "senior_clinician", due_at: "2026-10-08T08:00:00Z", status: "open",
  };
  events = [];
  posts = [];
}

function staffRow() {
  const open = contest.status === "open";
  return { ...contest, overdue: open ? Date.now() >= Date.parse(contest.due_at) : undefined, closure_held: open || contest.outcome === "decision_revised" };
}

function summary() {
  const held = staffRow().closure_held;
  return {
    ...base.case, id: CASE, provider: "ahs-mushrif", programme: "crc", state: "closed", dropoff: "", escalations: held ? ["patient_contest_review"] : [],
    owner: "nurse-synthetic", closed: !held, review: "none", overdue: false, ...(held ? { closure_hold: "patient_contest", canonical_closed: true } : {}),
    patient_tasks: [{ kind: "patient_contest", id: RECEIPT, owner: "senior_clinician", status: contest.status, escalation: "patient_contest_review", due_at: contest.due_at, closure_held: held }],
  };
}

function detail() {
  return {
    ...base, case: summary(), current: { ...base.current, loop: CASE, state: "closed", sig: DECISION, chain: [] },
    decisions: (base.decisions as Record<string, unknown>[]).map((d) => ({ ...d, loop: CASE, state: "closed" })),
    review_items: [], signoffs: [], evidence: [], holds: [], api_reviews: events, api_review_states: [],
    review_ledger: { status: "ok", events_applied: events.length }, patient_requests: [staffRow()],
  };
}

/** The Go handler's rules (pipeline/api_patient_request.go casePatientContest, resolvePatientContestError). */
function resolve(token: string, body: Record<string, unknown>): [number, unknown] {
  const who = PEOPLE[token];
  if (!who) return [401, { error: "authentication required" }];
  posts.push({ token, keys: Object.keys(body).sort() });
  const allowed = new Set(["contest", "outcome", "reason"]);
  if (Object.keys(body).some((k) => !allowed.has(k))) return [400, { meta, error: "body must be {contest, outcome, reason}" }];
  if (typeof body.reason !== "string" || body.reason.trim() === "") return [400, { meta, error: "reason is required" }];
  if (who.role !== "senior_clinician") return [403, { meta, error: `role ${who.role} may not resolve_patient_contest` }];
  if (body.outcome !== "decision_upheld" && body.outcome !== "decision_revised") return [400, { meta, error: "outcome must be decision_upheld|decision_revised" }];
  if (body.contest !== contest.receipt) return [404, { meta, error: "no patient contest with that receipt on this case" }];
  if (contest.status !== "open") return [409, { meta, error: "contest is already resolved" }];
  if (contest.excluded_reviewers.includes(who.actor)) return [403, { meta, error: "the original decision reviewer may not resolve this contest" }];
  const at = new Date().toISOString();
  const event = { type: "patient_request", kind: "contest_resolved", case: CASE, contest: contest.receipt, outcome: body.outcome, reason: body.reason, role: who.role, actor: who.actor, action: "resolve_patient_contest", at, mode: "demo" };
  const hashed = { ...event, hash: sha(JSON.stringify(event)) };
  events.push(hashed);
  Object.assign(contest, { status: "resolved", outcome: body.outcome, resolved_at: at, resolved_by: who.actor, resolution_reason: body.reason });
  return [201, { meta, event: hashed, patient_requests: [staffRow()], note: "resolution appended; canonical decisions and signatures are unchanged" }];
}

async function signIn(page: Page, token: string) {
  await page.getByLabel("Review token").fill(token);
  await page.getByRole("button", { name: /Sign in/ }).click();
}

const caseURL = `${NEXT_URL}/app/patients/${encodeURIComponent(CASE)}`;

async function review(page: Page, outcome: RegExp, reason: string) {
  const form = page.getByRole("form", { name: `Review contest ${RECEIPT.slice(0, 8)}` });
  await form.getByRole("radio", { name: outcome }).check();
  await form.getByLabel("Reason for your decision (required)").fill(reason);
  await form.getByRole("button", { name: "Record contest review" }).click();
  return form;
}

test.describe("patient contest review, live mock", () => {
  test.beforeEach(({}, testInfo) => test.skip(testInfo.project.name !== "chromium", "owns fixed ports: Chromium only"));
  test.describe.configure({ mode: "serial" });
  let api: Server;
  let next: ChildProcess | undefined;

  test.beforeAll(async () => {
    test.setTimeout(90_000);
    reset();
    api = createServer(async (req, res) => {
      const raw = await new Promise<string>((resolve) => {
        let value = "";
        req.on("data", (chunk) => (value += chunk));
        req.on("end", () => resolve(value));
      });
      const route = decodeURIComponent(new URL(req.url ?? "/", "http://mock").pathname);
      const token = (req.headers.authorization ?? "").replace(/^Bearer /, "");
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(JSON.stringify(body));
      };
      if (route === `/api/cases/${CASE}/patient-contest` && req.method === "POST") {
        if (!PEOPLE[token]) {
          posts.push({ token, keys: [] });
          return send(401, { error: "authentication required" });
        }
        const [status, body] = resolve(token, JSON.parse(raw));
        return send(status, body);
      }
      if (!Object.values(SHARED).includes(token) && !PEOPLE[token]) return send(401, { error: "authentication required" });
      if (route === "/api/fixtures") return send(200, { meta, fixtures: { evidence_closure: summary(), false_closure_trap: summary(), review_hold: summary() } });
      if (route === "/api/cases") return send(200, { meta, cases: [summary()], total: 1, limit: 500, offset: 0 });
      if (route.startsWith("/api/people")) return send(503, { meta, error: "no identity projection in this mock" });
      if (route === `/api/cases/${CASE}/audit`) return send(200, { ...baseAudit, case: CASE, history: events });
      if (route === `/api/cases/${CASE}`) return send(200, detail());
      return send(404, { meta, error: "not found", route });
    });
    await new Promise<void>((resolve) => api.listen(MOCK_PORT, "127.0.0.1", resolve));
    next = spawn(path.join(process.cwd(), "node_modules", ".bin", "next"), ["start", "-p", String(NEXT_PORT)], {
      env: { ...process.env, ...SHARED, WASL_API_URL: `http://127.0.0.1:${MOCK_PORT}`, WASL_REVIEW_AUTH_MODE: "demo" }, stdio: "ignore",
    });
    for (let i = 0; i < 120; i++) {
      if (await fetch(`${NEXT_URL}/app/today`).then((r) => r.ok, () => false)) return;
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
    throw new Error("contest review Next server did not become ready");
  });
  test.afterAll(async () => {
    next?.kill();
    if (api) await new Promise((resolve) => api.close(resolve));
  });

  test("an independent senior clinician records the outcome and reason; refused reviewers append nothing", async ({ page }) => {
    /* The case shows the open contest and links to the review. */
    await page.goto(caseURL);
    const card = page.getByRole("region", { name: "Patient explanation and contest requests" });
    await expect(card).toContainText("Closure is held");
    await card.getByRole("link", { name: "Review the patient contest" }).click();
    await expect(page).toHaveURL(/\/patient-contest$/);

    /* Anonymous: the page asks for a sign-in and offers no form; no server-held credential stands in. */
    await expect(page.getByRole("heading", { name: "Patient contest review" })).toBeVisible();
    await expect(page.getByRole("form", { name: /Review contest/ })).toHaveCount(0);
    expect(posts).toHaveLength(0);

    /* A clinician may sign in to read, but the API refuses the write. */
    await signIn(page, CLINICIAN);
    let form = await review(page, /Uphold the decision/, "Synthetic attempt by a clinician");
    await expect(form.getByTestId("contest-error")).toContainText("Only a senior clinician who did not sign this case");
    expect(events).toHaveLength(0);

    /* The original reviewer is a senior clinician, and is still refused. */
    await page.context().clearCookies();
    await page.goto(`${caseURL}/patient-contest`);
    await signIn(page, ORIGINAL);
    await expect(page.getByTestId(`contest-${RECEIPT.slice(0, 8)}`)).toContainText("May not resolve (signed this case): dr-original");
    form = await review(page, /Uphold the decision/, "I stand by my own decision (synthetic)");
    await expect(form.getByTestId("contest-error")).toContainText("the original decision reviewer may not resolve this contest");
    expect(events).toHaveLength(0);

    /* The independent senior clinician records the outcome and reason. */
    await page.context().clearCookies();
    await page.goto(`${caseURL}/patient-contest`);
    await signIn(page, INDEPENDENT);
    const reason = "Matched colonoscopy report and closure verified independently (synthetic).";
    await review(page, /Uphold the decision/, reason);
    await expect(page.getByTestId("contest-saved")).toContainText("decision upheld");
    const row = page.getByTestId(`contest-${RECEIPT.slice(0, 8)}`);
    await expect(row.getByTestId("contest-status")).toContainText("Resolved: decision upheld by dr-independent");
    await expect(row.getByTestId("contest-resolution-reason")).toContainText(reason);
    await expect(page.getByRole("form", { name: /Review contest/ })).toHaveCount(0);

    /* One event, identity from the signed-in token only, a body of exactly contest/outcome/reason. */
    expect(events.map((e) => [e.kind, e.role, e.actor, e.outcome, e.reason])).toEqual([["contest_resolved", "senior_clinician", "dr-independent", "decision_upheld", reason]]);
    for (const p of posts) {
      expect(Object.values(SHARED)).not.toContain(p.token);
      expect(p.keys).toEqual(["contest", "outcome", "reason"]);
    }
    expect(posts.map((p) => p.token)).toEqual([CLINICIAN, ORIGINAL, INDEPENDENT]);

    /* After reload, the case and the review page show the same resolved state. */
    await page.goto(caseURL);
    await page.reload();
    const saved = page.getByRole("region", { name: "Patient explanation and contest requests" });
    await expect(saved).toContainText("Resolved: decision upheld by dr-independent");
    await expect(saved).toContainText(reason);
    await expect(saved).not.toContainText("Closure is held");
    await expect(saved.getByRole("link", { name: "View patient contest reviews" })).toBeVisible();
    await page.goto(`${caseURL}/patient-contest`);
    await expect(page.getByTestId(`contest-${RECEIPT.slice(0, 8)}`).getByTestId("contest-status")).toContainText("Resolved: decision upheld by dr-independent");
  });

  test("a revised outcome keeps closure held", async ({ page }) => {
    reset();
    await page.goto(`${caseURL}/patient-contest`);
    await signIn(page, INDEPENDENT);
    await review(page, /Revise the decision/, "The closing report belongs to another episode (synthetic).");
    await expect(page.getByTestId("contest-saved")).toContainText("Closure stays held");
    const row = page.getByTestId(`contest-${RECEIPT.slice(0, 8)}`);
    await expect(row).toContainText("Resolved: decision to be revised by dr-independent");
    await expect(row).toContainText("Closure stays held until the questioned decision is replaced");
    await expect(page.locator("main header")).toContainText("automatic closure held by a patient contest");
    expect(events).toHaveLength(1);
  });
});
