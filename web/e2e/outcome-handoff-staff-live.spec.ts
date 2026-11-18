import { createHash } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";

/*
 * Staff approve and release of the post-closure outcome hand-off, in the browser, against a stateful synthetic mock of
 * the Go API and an isolated Next server. The mock mirrors pipeline/api_outcome_handoff.go staffHandoffError: identity
 * only from the bearer token, approval for `clinician` only and naming the current decision and the approval seen,
 * release for `coordinator` only and naming the current approved version, with the gate taken from consent. The real
 * handler and its ledger replay are covered by pipeline/api_outcome_handoff_test.go. Synthetic cases only.
 */
const MOCK_PORT = Number(process.env.HANDOFF_MOCK_PORT ?? 3395);
const NEXT_PORT = Number(process.env.HANDOFF_NEXT_PORT ?? 3396);
const NEXT_URL = `http://127.0.0.1:${NEXT_PORT}`;
/* Server-held role credentials. Only the coordinator one may be used here, and only for release. */
const SHARED = {
  WASL_API_REVIEWER_TOKEN: "synthetic-shared-reviewer-0002",
  WASL_API_WEB_REVIEWER_TOKEN: "synthetic-web-reviewer-00002",
  WASL_API_CLINICIAN_TOKEN: "synthetic-shared-clinician-0002",
  WASL_API_AUDITOR_TOKEN: "synthetic-shared-auditor-00002",
  WASL_API_COORDINATOR_TOKEN: "synthetic-shared-coord-000002",
};
/* Each person's own assigned token, typed into the sign-in form; the Next server never holds them. */
const PEOPLE: Record<string, { role: string; actor: string }> = {
  "handoff-clinician-0000001": { role: "clinician", actor: "dr-handoff" },
  "handoff-coordinator-00001": { role: "coordinator", actor: "nurse-handoff" },
  [SHARED.WASL_API_COORDINATOR_TOKEN]: { role: "coordinator", actor: "coordinator-shared" },
};
const CLINICIAN = "handoff-clinician-0000001";
const COORD_PERSON = "handoff-coordinator-00001";

const dir = path.join(process.cwd(), "data", "api");
const base = JSON.parse(readFileSync(path.join(dir, "case_review_hold.json"), "utf8"));
const baseAudit = JSON.parse(readFileSync(path.join(dir, "case_review_hold_audit.json"), "utf8"));
const meta = base.meta;
const sha = (s: string) => createHash("sha256").update(s).digest("hex");

const CASE = "ahs-mushrif/ServiceRequest/AHS-sr-9112";
const DECISION = sha("synthetic-closed-decision");
const STEPS = ["routine_screening", "surveillance", "specialist_follow_up", "discuss_with_doctor"];
type Row = { kind: string; version: string; role: string; actor: string; at: string; event: string; gate?: string; next_step?: string };
let history: Row[];
let posts: { token: string; body: Record<string, unknown> }[];

function reset() {
  history = [];
  posts = [];
}

const approval = () => history.filter((r) => r.kind === "content_approved").at(-1);
const releaseOf = (v: string) => history.filter((r) => r.kind === "release_recorded" && r.version === v).at(-1);

function handoff() {
  const a = approval();
  const release = a && releaseOf(a.version);
  const status = !a ? "pending_approval" : !release ? "pending_release" : release.gate;
  return {
    status, delivery: "simulated", consent: "granted", responsible_referrer: "referrer/ahs-mushrif", referrer_status: "pending",
    draft: { decision: DECISION, programme: "crc", outcome: "closed", safety_net: "safety_net_v1" }, next_steps: STEPS,
    ...(a && {
      version: a.version, approved_at: a.at, approved_by: a.actor, patient_informed: false,
      content: { outcome: "follow_up_complete", next_step: a.next_step, safety_net: "safety_net_v1", decision: DECISION, programme: "crc" },
    }),
    ...(release && { release: { gate: release.gate, at: release.at, actor: release.actor, event: release.event } }),
    history: history.map((row) => Object.fromEntries(Object.entries(row).filter(([key]) => key !== "next_step"))),
  };
}

function summary() {
  return { ...base.case, id: CASE, provider: "ahs-mushrif", programme: "crc", state: "closed", dropoff: "", escalations: [], owner: "nurse-synthetic", closed: true, review: "none", overdue: false };
}

function detail() {
  return {
    ...base, case: summary(), current: { ...base.current, loop: CASE, state: "closed", sig: DECISION, chain: [] },
    decisions: (base.decisions as Record<string, unknown>[]).map((d) => ({ ...d, loop: CASE, state: "closed" })),
    review_items: [], signoffs: [], evidence: [], holds: [], api_reviews: [], api_review_states: [],
    review_ledger: { status: "ok", events_applied: history.length }, outcome_handoff: handoff(),
  };
}

/** The Go handler's rules (pipeline/api_outcome_handoff.go staffHandoffError) for approval and release. */
function write(token: string, body: Record<string, unknown>): [number, unknown] {
  const who = PEOPLE[token];
  if (!who) return [401, { error: "authentication required" }];
  posts.push({ token, body });
  const at = new Date().toISOString();
  const append = (row: Omit<Row, "event" | "at" | "role" | "actor">) => {
    const event = sha(JSON.stringify({ ...row, at, n: history.length }));
    history.push({ ...row, role: who.role, actor: who.actor, at, event });
    return [201, { meta, event: { hash: event }, replayed: false, handoff: handoff() }] as [number, unknown];
  };
  if (body.kind === "content_approved") {
    if (who.role !== "clinician") return [403, { meta, error: `role ${who.role} may not approve_outcome_handoff`, handoff: handoff() }];
    if (!STEPS.includes(String(body.next_step))) return [400, { meta, error: "next_step must be one of the patient-safe next steps" }];
    if (body.decision !== DECISION) return [409, { meta, error: "decision is not this case's current decision; review the current outcome" }];
    const version = sha(`${DECISION}:${body.next_step}`);
    const current = approval()?.version ?? "";
    if (current === version) return [200, { meta, event: null, replayed: true, handoff: handoff() }];
    if (body.version !== current) return [409, { meta, error: "the outcome hand-off changed since it was loaded; review the current approval" }];
    return append({ kind: "content_approved", version, next_step: String(body.next_step) });
  }
  if (body.kind === "release_recorded") {
    if (who.role !== "coordinator") return [403, { meta, error: `role ${who.role} may not dispatch_outreach` }];
    const a = approval();
    if (!a) return [409, { meta, error: "the outcome hand-off is pending clinician approval; nothing can be released" }];
    if (body.version !== a.version) return [409, { meta, error: "version is not the current approved content" }];
    if (releaseOf(a.version)?.gate === "released") return [200, { meta, event: null, replayed: true, handoff: handoff() }];
    return append({ kind: "release_recorded", version: a.version, gate: "released" });
  }
  return [400, { meta, error: "kind must be content_approved|referrer_acknowledged|release_recorded" }];
}

async function signIn(page: Page, token: string) {
  const box = page.getByTestId("governance-signin");
  await box.getByLabel("Your token").fill(token);
  await box.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByTestId("governance-signin")).toHaveCount(0);
}

const caseURL = `${NEXT_URL}/app/patients/${encodeURIComponent(CASE)}`;
const card = (page: Page) => page.getByRole("region", { name: "Outcome hand-off", exact: true });

async function approve(page: Page, step: string) {
  await card(page).getByText(/^Approve (revised )?content$/).click();
  const form = page.getByRole("form", { name: "Approve outcome hand-off content" });
  await form.getByRole("combobox", { name: "Next step for the patient" }).selectOption(step);
  await form.getByRole("button", { name: "Record approval" }).click();
  return form;
}

test.describe("outcome hand-off staff steps, live mock", () => {
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
      if (route === `/api/cases/${CASE}/outcome-handoff` && req.method === "POST") {
        const [status, body] = write(token, JSON.parse(raw));
        return send(status, body);
      }
      if (!Object.values(SHARED).includes(token) && !PEOPLE[token]) return send(401, { error: "authentication required" });
      if (route === "/api/fixtures") return send(200, { meta, fixtures: { evidence_closure: summary(), false_closure_trap: summary(), review_hold: summary() } });
      if (route === "/api/cases") return send(200, { meta, cases: [summary()], total: 1, limit: 500, offset: 0 });
      if (route.startsWith("/api/people")) return send(503, { meta, error: "no identity projection in this mock" });
      if (route === `/api/cases/${CASE}/audit`) return send(200, { ...baseAudit, case: CASE, history: [] });
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
    throw new Error("outcome hand-off Next server did not become ready");
  });
  test.afterAll(async () => {
    next?.kill();
    if (api) await new Promise((resolve) => api.close(resolve));
  });

  test("a signed-in clinician approves, a coordinator refusal shows the server's words, and release uses the coordinator credential", async ({ page }) => {
    await page.goto(caseURL);
    await expect(card(page)).toContainText("Pending clinician approval");
    /* Anonymous: a personal sign-in, no approval form, no release before an approval. */
    await expect(card(page).getByTestId("governance-signin")).toBeVisible();
    await expect(page.getByRole("form", { name: "Approve outcome hand-off content" })).toHaveCount(0);
    await expect(card(page).getByTestId("outcome-handoff-release")).toHaveCount(0);

    /* A coordinator may sign in, but the API refuses the approval and the page shows why. */
    await signIn(page, COORD_PERSON);
    const form = await approve(page, "surveillance");
    await expect(form.getByRole("alert")).toContainText("may not approve_outcome_handoff");
    expect(history).toHaveLength(0);

    /* The clinician approves as their own token. */
    await page.context().clearCookies();
    await page.goto(caseURL);
    await signIn(page, CLINICIAN);
    await approve(page, "surveillance");
    await expect.poll(() => history.length).toBe(1);
    expect(history[0]).toMatchObject({ kind: "content_approved", role: "clinician", actor: "dr-handoff", next_step: "surveillance" });

    await page.reload();
    await expect(card(page)).toContainText("Approved, not yet released");
    await expect(card(page)).toContainText("approved by dr-handoff");
    await card(page).getByRole("button", { name: "Record simulated release" }).click();
    await expect.poll(() => history.length).toBe(2);
    expect(history[1]).toMatchObject({ kind: "release_recorded", gate: "released", role: "coordinator", actor: "coordinator-shared" });

    await page.reload();
    await expect(card(page)).toContainText("Released to the patient link, not yet acknowledged");
    await expect(card(page)).toContainText("Responsible referrer (referrer/ahs-mushrif)");

    /* Identity came only from tokens: approval from the person's own, release from the server's coordinator credential. */
    expect(posts.map((p) => [p.token, p.body.kind])).toEqual([
      [COORD_PERSON, "content_approved"],
      [CLINICIAN, "content_approved"],
      [SHARED.WASL_API_COORDINATOR_TOKEN, "release_recorded"],
    ]);
    for (const p of posts) expect(Object.keys(p.body).filter((k) => ["role", "actor", "credential", "token"].includes(k))).toEqual([]);
  });
});
