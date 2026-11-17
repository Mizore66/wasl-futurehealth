import { createHash } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

/*
 * #168 / #154 browser journeys against a stateful synthetic API and an isolated Next server in live mode. The mock mirrors
 * the referral-response and referrer-response contracts (pipeline/api_referral.go, pipeline/api_referrer.go); the real
 * validators, RBAC and replay are covered by pipeline/api_referral_test.go and pipeline/api_referrer_test.go.
 */
const MOCK_PORT = Number(process.env.REFERRAL_MOCK_PORT ?? 3391);
const NEXT_PORT = Number(process.env.REFERRAL_NEXT_PORT ?? 3392);
const NEXT_URL = `http://127.0.0.1:${NEXT_PORT}`;
const SHARED = {
  WASL_API_REVIEWER_TOKEN: "synthetic-shared-reviewer-0001",
  WASL_API_WEB_REVIEWER_TOKEN: "synthetic-web-reviewer-00001",
  WASL_API_CLINICIAN_TOKEN: "synthetic-unit-clinician-0001",
  WASL_API_REFERRER_TOKEN: "synthetic-referrer-gp-000001",
  WASL_API_AUDITOR_TOKEN: "synthetic-shared-auditor-00001",
  WASL_API_COORDINATOR_TOKEN: "synthetic-shared-coord-000001",
};
const fixtures = path.join(process.cwd(), "tests", "fixtures");
const base = JSON.parse(readFileSync(path.join(fixtures, "ledger_referral_accepted_detail.json"), "utf8"));
const dashboard = JSON.parse(readFileSync(path.join(process.cwd(), "data", "api", "dashboard.json"), "utf8"));
const meta = base.meta;
const ids = ["770", "771", "772"].map((n) => `ahs-mushrif/ServiceRequest/AHS-sr-${n}`);
const hex = (c: string) => c.repeat(64);
const RETURN_REASONS: Record<string, string> = { needs_clinic_visit: "needs_more_info", anticoagulant_plan_missing: "needs_more_info", wrong_unit: "wrong_provider", not_fit_for_procedure: "other" };

type Entry = Record<string, unknown>;
type CaseState = { head: string; events: Entry[]; referral: Entry | null; ack: Entry | null; resolved: Entry | null };
let cases: Record<string, CaseState>;
let failNext: number | null;
let posts: number;

function reset() {
  cases = Object.fromEntries(ids.map((id, i) => [id, { head: hex(String(i + 1)), events: [], referral: null, ack: null, resolved: null }]));
  failNext = null;
  posts = 0;
}
reset();

function handoff(id: string) {
  const c = cases[id];
  const r = c.referral;
  if (!r || r.response !== "return") return undefined;
  const h: Entry = {
    referral_id: id, decision: "reject", reason: r.reason, reason_code: RETURN_REASONS[r.reason as string], info_requested: r.requested_info,
    clinical_priority: "urgent", responsible_clinician: "referring PCP, ahs-mushrif", decided_by: "skmc-endo", decided_at: 110, act_by: 112,
    medical_director_day: 116, status: "awaiting_referrer", event: r.event, destinations: ["skmc-endo", "noor-endo"],
  };
  if (c.ack) h.acknowledgement = c.ack;
  if (c.resolved) Object.assign(h, { referrer_response: c.resolved, acted_at: 110, status: "referrer_acted" });
  return h;
}

function overlay(id: string) {
  const c = cases[id];
  const o: Entry = { ...base.case.api_overlay, head: c.head, actions: c.events.length, last_action_at: (c.events.at(-1)?.at as string) ?? "" };
  delete o.referral_response;
  if (c.referral) o.referral_response = c.referral;
  const h = handoff(id);
  if (h) o.referral_handoff = h;
  return o;
}

function summary(id: string) {
  return { ...base.case, id, api_overlay: overlay(id) };
}

function detail(id: string) {
  const o = overlay(id);
  return {
    ...base, case: summary(id), current: { ...base.current, loop: id }, api_reviews: cases[id].events,
    review_ledger: { status: "ok", events_applied: cases[id].events.length }, api_overlay: { ...o, contact_outcomes: [], notes: [], canonical_owner: "" },
  };
}

function append(id: string, kind: string, actor: string, data: Entry) {
  const c = cases[id];
  const at = new Date(Date.parse("2026-11-16T10:00:00Z") + ++posts * 1000).toISOString();
  const event: Entry = { type: "coordinator_action", kind, case: id, data, role: "clinician", actor, action: "respond_referral", at, head: c.head, seq: c.events.length + 1, mode: "demo", prev: c.events.at(-1)?.hash ?? "genesis" };
  event.hash = createHash("sha256").update(JSON.stringify(event)).digest("hex");
  c.events.push(event);
  c.head = event.hash as string;
  return { event, entry: { ...data, at, actor, event: event.hash } };
}

const conflict = (id: string, error: string, kind: string) => ({ status: 409, body: { meta, error, conflict: kind, current_head: cases[id].head } });

function referralResponse(id: string, body: Entry) {
  const c = cases[id];
  if (body.expected_head !== c.head) return conflict(id, "stale expected_head: the case changed since it was read", "head");
  if (c.referral) return conflict(id, "the receiving unit has already answered this referral", "referral_state");
  if (body.referral_id !== id || !["accept", "accept_with_conditions", "return"].includes(body.response as string)) return { status: 400, body: { meta, error: "malformed referral response" } };
  if (body.response === "return" && !RETURN_REASONS[body.reason as string]) return { status: 400, body: { meta, error: "return needs a supported reason" } };
  const data: Entry = { referral_id: id, response: body.response, conditions: [...((body.conditions as string[]) ?? [])].sort(), requested_info: [...((body.requested_info as string[]) ?? [])].sort() };
  if (body.response === "return") data.reason = body.reason;
  const { event, entry } = append(id, "referral_response", "Dr Layla Haddad", { ...data, unit: "skmc-endo", run_day: 110 });
  c.referral = entry;
  return { status: 201, body: { meta, event, api_overlay: overlay(id), note: "recorded in the review ledger" } };
}

function referrerResponse(id: string, body: Entry) {
  const c = cases[id];
  if (body.expected_head !== c.head) return conflict(id, "stale expected_head: the case changed since it was read", "head");
  if (!c.referral || c.referral.response !== "return" || body.packet !== c.referral.event) return { status: 400, body: { meta, error: "packet must be this case's returned referral packet" } };
  if (c.resolved || (body.response === "acknowledge" && c.ack)) return conflict(id, "the referrer has already answered", "handoff_state");
  const data: Entry = { referral_id: id, packet: body.packet, response: body.response };
  if (body.response === "supply_info") {
    const given = (body.supplied_info ?? {}) as Record<string, string>;
    const requested = c.referral.requested_info as string[];
    if (requested.length === 0 || Object.keys(given).length !== requested.length || requested.some((k) => !given[k]?.trim())) return { status: 400, body: { meta, error: "supplied_info must answer every requested item" } };
    data.supplied_info = given;
  }
  const { event, entry } = append(id, "referrer_response", "Dr Huda Al Mansoori", { ...data, provider: "ahs-mushrif", run_day: 110 });
  if (body.response === "acknowledge") c.ack = entry;
  else c.resolved = entry;
  return { status: 201, body: { meta, event, api_overlay: overlay(id), note: "recorded in the review ledger" } };
}

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

test.describe("referral answers, live mock", () => {
  test.beforeEach(({}, testInfo) => test.skip(testInfo.project.name !== "chromium", "owns fixed ports: Chromium only"));
  test.describe.configure({ mode: "serial" });
  let api: Server;
  let next: ChildProcess | undefined;

  test.beforeAll(async () => {
    test.setTimeout(90_000);
    api = createServer(async (req, res) => {
      const raw = await new Promise<string>((resolve) => {
        let value = "";
        req.on("data", (chunk) => (value += chunk));
        req.on("end", () => resolve(value));
      });
      const url = new URL(req.url ?? "/", "http://mock");
      const route = decodeURIComponent(url.pathname);
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(JSON.stringify(body));
      };
      if (route === "/mock/fail-next" && req.method === "POST") {
        failNext = JSON.parse(raw).status;
        return send(200, { failNext });
      }
      if (route === "/mock/state") return send(200, cases);
      if (route === "/api/dashboard") return send(200, dashboard);
      if (route === "/api/fixtures") return send(200, { meta, fixtures: { evidence_closure: summary(ids[0]), false_closure_trap: summary(ids[1]), review_hold: summary(ids[2]) } });
      if (route === "/api/cases") {
        const q = url.searchParams.get("q") ?? "";
        const prog = url.searchParams.get("programme");
        const list = prog && prog !== "crc" ? [] : ids.filter((id) => id.includes(q)).map(summary);
        return send(200, { meta, cases: list, total: list.length, limit: 500, offset: 0 });
      }
      const m = /^\/api\/cases\/(.+?)(\/referral-response|\/referrer-response)?$/.exec(route);
      const id = m?.[1];
      if (!id || !cases[id]) return send(404, { meta, error: "not found", route });
      if (!m[2]) return send(200, detail(id));
      if (req.method !== "POST") return send(405, { meta, error: "method" });
      const token = m[2] === "/referral-response" ? SHARED.WASL_API_CLINICIAN_TOKEN : SHARED.WASL_API_REFERRER_TOKEN;
      if (req.headers.authorization !== `Bearer ${token}`) return send(403, { meta, error: "this credential may not answer here" });
      if (failNext !== null) {
        const status = failNext;
        failNext = null;
        return send(status, { meta, error: status === 409 ? "stale expected_head: the case changed since it was read" : "could not append to the review ledger; nothing was recorded", conflict: status === 409 ? "head" : undefined });
      }
      const result = m[2] === "/referral-response" ? referralResponse(id, JSON.parse(raw)) : referrerResponse(id, JSON.parse(raw));
      return send(result.status, result.body);
    });
    await new Promise<void>((resolve) => api.listen(MOCK_PORT, "127.0.0.1", resolve));
    next = spawn(path.join(process.cwd(), "node_modules", ".bin", "next"), ["start", "-p", String(NEXT_PORT)], {
      env: { ...process.env, ...SHARED, WASL_API_URL: `http://127.0.0.1:${MOCK_PORT}` }, stdio: "ignore",
    });
    for (let i = 0; i < 120; i++) {
      if (await fetch(`${NEXT_URL}/app/referrals`).then((r) => r.ok, () => false)) return;
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
    throw new Error("referral Next server did not become ready");
  });
  test.afterAll(async () => {
    next?.kill();
    if (api) await new Promise((resolve) => api.close(resolve));
  });
  test.beforeEach(() => reset());

  const open = async (page: Page, n: string) => {
    await page.goto(`${NEXT_URL}/app/referrals?q=${n}`);
    await expect(page.locator("[data-honesty=built]")).toHaveText("Referral answers: built, saved to the synthetic case ledger");
    return page.getByRole("region", { name: "Open referral" });
  };
  const ledger = async () => (await fetch(`http://127.0.0.1:${MOCK_PORT}/mock/state`).then((r) => r.json())) as Record<string, CaseState>;

  test("Accept is confirmed only after the append succeeds and survives reload", async ({ page }) => {
    let pane = await open(page, "770");
    await fetch(`http://127.0.0.1:${MOCK_PORT}/mock/fail-next`, { method: "POST", body: JSON.stringify({ status: 500 }) });
    await pane.getByRole("button", { name: "Accept referral", exact: true }).click();
    await expect(pane.getByRole("alert")).toContainText("Not saved");
    await expect(pane.getByRole("button", { name: "Accept referral", exact: true })).toBeVisible();
    expect((await ledger())[ids[0]].events).toHaveLength(0);

    await pane.getByRole("button", { name: "Accept referral", exact: true }).click();
    await expect(pane.getByRole("status").first()).toContainText("Saved to the case ledger");
    await expect(pane.getByRole("button", { name: "Undo" })).toHaveCount(0);
    expect((await ledger())[ids[0]].referral?.response).toBe("accept");

    pane = await open(page, "770");
    await expect(pane.getByRole("button", { name: "Accept referral", exact: true })).toHaveCount(0);
    await expect(page.getByRole("tab", { name: /^Accepted 1/ })).toBeVisible();
    await noSeriousAxe(page);
  });

  test("conditional Accept persists each structured condition", async ({ page }) => {
    let pane = await open(page, "771");
    await pane.getByRole("button", { name: "Accept with conditions" }).click();
    await expect(pane.getByRole("heading", { name: "Accept on these conditions" })).toBeFocused();
    await pane.getByRole("button", { name: "Arabic interpreter on the day" }).click();
    await pane.getByRole("button", { name: "Female endoscopist" }).click();
    await pane.getByRole("button", { name: "Accept with 2 conditions" }).click();
    await expect(pane.getByRole("status").first()).toContainText("Saved to the case ledger");
    expect((await ledger())[ids[1]].referral?.conditions).toEqual(["arabic_interpreter", "female_endoscopist"]);
    pane = await open(page, "771");
    await page.getByRole("tab", { name: /^Accepted 1/ }).click();
    await expect(pane).toContainText("Arabic interpreter");
    await expect(pane).toContainText("Female endoscopist");
  });

  test("a stale Return is refused without changing the inbox; a fresh one hands the packet to the GP, who answers it", async ({ page }) => {
    let pane = await open(page, "772");
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await pane.getByRole("button", { name: "Blood thinner plan missing" }).click();
    await fetch(`http://127.0.0.1:${MOCK_PORT}/mock/fail-next`, { method: "POST", body: JSON.stringify({ status: 409 }) });
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByRole("alert")).toContainText("Not saved");
    expect((await ledger())[ids[2]].events).toHaveLength(0);

    pane = await open(page, "772");
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await pane.getByRole("button", { name: "Blood thinner plan missing" }).click();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByRole("status").first()).toContainText("Saved to the case ledger");
    pane = await open(page, "772");
    await page.getByRole("tab", { name: /^Waiting on GP 1/ }).click();
    await expect(pane).toContainText("Returned to the GP: blood thinner plan missing");
    await expect(pane).toContainText("Act on the return");
    await expect(pane.getByRole("button", { name: "Accept referral", exact: true })).toHaveCount(0);

    // The responsible referrer's view of the same packet.
    await page.goto(`${NEXT_URL}/app/referrals/returned`);
    await expect(page.locator("[data-honesty=built]")).toContainText("GP answers: built");
    const card = page.getByRole("article");
    await expect(card).toContainText("Returned bySKMC endoscopy");
    await expect(card).toContainText("ReasonBlood thinner plan missing");
    await card.getByRole("button", { name: "Acknowledge the returned referral" }).click();
    await expect(card.getByRole("status")).toContainText("Acknowledged");
    await page.reload();
    await expect(page.getByRole("article")).toContainText("AcknowledgedYes");
    // Acknowledgement alone leaves the hand-off waiting on the GP.
    await expect(page.getByRole("article")).toContainText("Act on the return");
    await page.getByRole("radio", { name: "Manage in primary care" }).check();
    await page.getByRole("button", { name: "Send answer" }).click();
    await expect(page.getByRole("article").getByRole("alert")).toBeVisible();
    expect((await ledger())[ids[2]].resolved).toBeNull();
    await page.getByRole("article").getByRole("textbox").fill("Anticoagulation reviewed at the clinic; repeat FIT in 3 months");
    await page.getByRole("button", { name: "Send answer" }).click();
    await expect(page.getByRole("article").getByRole("status").first()).toContainText(/no longer waits on you|Nothing else is needed/);
    await page.reload();
    await expect(page.getByRole("article").getByRole("status")).toContainText("Nothing else is needed from you");
    expect((await ledger())[ids[2]].events).toHaveLength(3);

    pane = await open(page, "772");
    await expect(page.getByRole("tab", { name: /^Waiting on GP 0/ })).toBeVisible();
    await noSeriousAxe(page);
  });
});
