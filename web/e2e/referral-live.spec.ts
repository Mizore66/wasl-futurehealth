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
  WASL_API_REFERRER_DR_TOKEN: "synthetic-referrer-eye-00001",
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

/*
 * #154/#155: a canonical DR packet the eye clinic returned (the engine's packet, not a unit answer through the API), from
 * the synthetic controls pipeline/referral_handoff_fixture_test.go serves. The mock projects the referrer's answers onto
 * api_overlay.referral_handoff as pipeline/api_referrer.go does and leaves `current.handoff` untouched.
 */
const handoffControls = JSON.parse(readFileSync(path.join(process.cwd(), "data", "referral-handoff-synthetic.json"), "utf8")) as { case: Entry; current: Entry }[];
const drControl = handoffControls.find((x) => (x.case.id as string).endsWith("PHC-sr-9203"))!;
const drId = drControl.case.id as string;
const drPacket = (drControl.current.handoff as Entry).referral_id as string;
let dr: { head: string; events: Entry[]; ack: Entry | null; resolved: Entry | null };

function reset() {
  cases = Object.fromEntries(ids.map((id, i) => [id, { head: hex(String(i + 1)), events: [], referral: null, ack: null, resolved: null }]));
  dr = { head: hex("d"), events: [], ack: null, resolved: null };
  failNext = null;
  posts = 0;
}
reset();

function drSummary() {
  const ov = drControl.case.api_overlay as Entry;
  const h: Entry = { ...(ov.referral_handoff as Entry) };
  if (dr.ack) h.acknowledgement = dr.ack;
  if (dr.resolved) Object.assign(h, { referrer_response: { ...dr.resolved, supplied_info: undefined }, acted_at: 120, status: "referrer_acted" });
  return { ...drControl.case, api_overlay: { ...ov, head: dr.head, actions: dr.events.length, referral_handoff: h } };
}

function drResponse(body: Entry) {
  if (body.expected_head !== dr.head) return { status: 409, body: { meta, error: "stale expected_head: the case changed since it was read", conflict: "head", current_head: dr.head } };
  if (body.referral_id !== drId || body.packet !== drPacket) return { status: 400, body: { meta, error: "packet must be this case's returned referral packet" } };
  if (dr.resolved || (body.response === "acknowledge" && dr.ack)) return { status: 409, body: { meta, error: "the referrer has already answered", conflict: "handoff_state" } };
  const requested = (drControl.current.handoff as Entry).info_requested as string[];
  const given = (body.supplied_info ?? {}) as Record<string, string>;
  if (body.response === "supply_info" && (Object.keys(given).length !== requested.length || requested.some((k) => !given[k]?.trim()))) return { status: 400, body: { meta, error: "supplied_info must answer every requested item" } };
  const at = new Date(Date.parse("2026-11-16T10:00:00Z") + ++posts * 1000).toISOString();
  const data: Entry = { referral_id: drId, packet: drPacket, response: body.response, provider: "phc-retina", run_day: 120, ...(body.response === "supply_info" && { supplied_info: given }) };
  const event: Entry = { type: "coordinator_action", kind: "referrer_response", case: drId, data, role: "clinician", actor: "Dr Khalid Al Shamsi", action: "respond_referral", at, head: dr.head, seq: dr.events.length + 1, mode: "demo" };
  event.hash = createHash("sha256").update(JSON.stringify(event)).digest("hex");
  dr.events.push(event);
  dr.head = event.hash as string;
  const entry = { response: body.response, at, actor: event.actor, run_day: 120, event: event.hash };
  if (body.response === "acknowledge") dr.ack = entry;
  else dr.resolved = entry;
  return { status: 201, body: { meta, event, api_overlay: { ...drSummary().api_overlay, contact_outcomes: [], notes: [] }, note: "recorded in the review ledger" } };
}

/* Like pipeline/api.go overlayViewLocked: the unit's note to the GP is detail only; the list says has_note. */
function handoff(id: string, full: boolean) {
  const c = cases[id];
  const r = c.referral;
  if (!r || r.response !== "return") return undefined;
  const h: Entry = {
    referral_id: id, decision: "reject", reason: r.reason, reason_code: RETURN_REASONS[r.reason as string], info_requested: r.requested_info,
    clinical_priority: "urgent", responsible_clinician: "referring PCP, ahs-mushrif", decided_by: "skmc-endo", decided_at: 110, act_by: 112,
    medical_director_day: 116, status: "awaiting_referrer", event: r.event, destinations: ["skmc-endo", "noor-endo"],
  };
  if (r.note) Object.assign(h, full ? { note: r.note } : { has_note: true });
  if (c.ack) h.acknowledgement = c.ack;
  if (c.resolved) Object.assign(h, { referrer_response: c.resolved, acted_at: 110, status: "referrer_acted" });
  return h;
}

function overlay(id: string, full = false) {
  const c = cases[id];
  const o: Entry = { ...base.case.api_overlay, head: c.head, actions: c.events.length, last_action_at: (c.events.at(-1)?.at as string) ?? "" };
  delete o.referral_response;
  if (c.referral) {
    const { note, ...rest } = c.referral;
    o.referral_response = note && !full ? { ...rest, has_note: true } : c.referral;
  }
  const h = handoff(id, full);
  if (h) o.referral_handoff = h;
  return o;
}

function summary(id: string) {
  return { ...base.case, id, api_overlay: overlay(id) };
}

function detail(id: string) {
  const o = overlay(id, true);
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
  const note = body.note;
  if (note !== undefined && (body.response !== "return" || typeof note !== "string" || note !== note.trim() || !note || [...note].length > 500)) return { status: 400, body: { meta, error: "note must be 1 to 500 characters on a return" } };
  const data: Entry = { referral_id: id, response: body.response, conditions: [...((body.conditions as string[]) ?? [])].sort(), requested_info: [...((body.requested_info as string[]) ?? [])].sort() };
  if (body.response === "return") data.reason = body.reason;
  if (note) data.note = note;
  const { event, entry } = append(id, "referral_response", "Dr Layla Haddad", { ...data, unit: "skmc-endo", run_day: 110 });
  c.referral = entry;
  return { status: 201, body: { meta, event, api_overlay: overlay(id, true), note: "recorded in the review ledger" } };
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
      if (route === "/mock/dr") return send(200, dr);
      if (route === "/api/cases" && url.searchParams.get("programme") === "diabetic_retinopathy") {
        const list = url.searchParams.get("provider") === "phc-retina" && drId.includes(url.searchParams.get("q") ?? "") ? [drSummary()] : [];
        return send(200, { meta, cases: list, total: list.length, limit: 500, offset: 0 });
      }
      if (route === `/api/cases/${drId}`) return send(200, { ...base, case: drSummary(), current: drControl.current, api_reviews: dr.events, api_overlay: { ...drSummary().api_overlay, contact_outcomes: [], notes: [] } });
      if (route === `/api/cases/${drId}/referrer-response` && req.method === "POST") {
        if (req.headers.authorization !== `Bearer ${SHARED.WASL_API_REFERRER_DR_TOKEN}`) return send(403, { meta, error: "only the case's responsible referring clinician may answer this hand-off" });
        const result = drResponse(JSON.parse(raw));
        return send(result.status, result.body);
      }
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
    await expect(page.locator("[data-honesty=built]")).toHaveText("Referral answers: built, saved to the simulated case record");
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
    await expect(pane.getByRole("status").first()).toContainText("Saved to the case record");
    await expect(pane.getByRole("button", { name: "Undo" })).toHaveCount(0);
    expect((await ledger())[ids[0]].referral?.response).toBe("accept");

    pane = await open(page, "770");
    await expect(pane.getByRole("button", { name: "Accept referral", exact: true })).toHaveCount(0);
    await expect(page.getByRole("region", { name: "SKMC endoscopy, current" })).toContainText("Accepted");
    await noSeriousAxe(page);
  });

  test("conditional Accept persists each structured condition", async ({ page }) => {
    let pane = await open(page, "771");
    await pane.getByRole("button", { name: "Accept with conditions" }).click();
    await expect(pane.getByRole("heading", { name: "Accept on these conditions" })).toBeFocused();
    await pane.getByRole("button", { name: "Arabic interpreter on the day" }).click();
    await pane.getByRole("button", { name: "Female endoscopist" }).click();
    await pane.getByRole("button", { name: "Accept with 2 conditions" }).click();
    await expect(pane.getByRole("status").first()).toContainText("Saved to the case record");
    expect((await ledger())[ids[1]].referral?.conditions).toEqual(["arabic_interpreter", "female_endoscopist"]);
    pane = await open(page, "771");
    await expect(pane).toContainText("Arabic interpreter");
    await expect(pane).toContainText("Female endoscopist");
  });

  test("a stale Return is refused without changing the inbox; a fresh one hands the packet to the GP, who answers it", async ({ page }) => {
    let pane = await open(page, "772");
    await pane.getByRole("button", { name: "Return with reason" }).click();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByRole("alert")).toHaveText("Choose a reason before you return it. Nothing was sent.");
    expect((await ledger())[ids[2]].events).toHaveLength(0);
    await pane.getByRole("radio", { name: "Blood thinner plan missing" }).check();
    await fetch(`http://127.0.0.1:${MOCK_PORT}/mock/fail-next`, { method: "POST", body: JSON.stringify({ status: 409 }) });
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByRole("alert")).toContainText("Not saved");
    expect((await ledger())[ids[2]].events).toHaveLength(0);

    // The user flow: from the full inbox, searched, into the referral and back.
    await page.goto(`${NEXT_URL}/app/referrals?view=inbox`);
    const box = page.getByRole("searchbox", { name: "Search case id" });
    await box.fill("772");
    await box.press("Enter");
    await expect(page).toHaveURL(/q=772&view=inbox$/);
    await expect(page.getByTestId("page-range")).toHaveText("1 referral");
    await page.getByRole("link", { name: "AHS-sr-772" }).click();
    pane = page.getByRole("region", { name: "Open referral" });
    await pane.getByRole("button", { name: "Return with reason" }).click();
    await pane.getByRole("radio", { name: "Blood thinner plan missing" }).check();
    await pane.getByRole("textbox", { name: /Note to the GP/ }).fill("Please attach the FIT report.");
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByRole("status").first()).toContainText("Saved to the case record");
    expect((await ledger())[ids[2]].referral?.note).toBe("Please attach the FIT report.");
    await expect(page.getByTestId("handoff-arrow")).toHaveAttribute("data-direction", "back");
    const gp = page.getByRole("region", { name: /GP, current$/ });
    await expect(gp).toContainText("Waiting on GP");
    await expect(gp).toContainText("Booking held");
    await expect(gp).toContainText("Returned to the GP: blood thinner plan missing");
    // Read from the case detail: the list carries only has_note.
    await expect(gp.getByTestId("return-note")).toHaveText("Note to the GP: Please attach the FIT report.");
    await expect(gp).not.toContainText("Kept in this browser");
    await expect(pane).toContainText("Act on the return");
    await expect(pane.getByRole("button", { name: "Accept referral", exact: true })).toHaveCount(0);

    // The responsible referrer's view of the same packet, opened from the hand-off.
    await gp.getByRole("link", { name: "Open the GP view" }).click();
    await expect(page).toHaveURL(/\/app\/referrals\/returned\?case=/);
    await expect(page.locator("[data-honesty=built]")).toContainText("Referrer answers: built");
    const card = page.getByRole("article");
    await expect(card).toContainText("Returned bySKMC endoscopy");
    await expect(card).toContainText("ReasonBlood thinner plan missing");
    await expect(card).toContainText("Note from the unitPlease attach the FIT report.");
    await card.getByRole("button", { name: "Acknowledge the returned referral" }).click();
    await expect(card.getByRole("status")).toContainText("Acknowledged");
    await page.reload();
    await expect(page.getByRole("article")).toContainText("AcknowledgedYes");
    // Acknowledgement alone leaves the hand-off waiting on the GP.
    await expect(page.getByRole("article")).toContainText("Act on the return");
    // Back to the same inbox state: the search is kept and the row shows the acknowledgement.
    await page.getByRole("link", { name: "Back to inbox" }).click();
    await expect(page).toHaveURL(/q=772&view=inbox&case=/);
    await expect(page.getByRole("searchbox", { name: "Search case id" })).toHaveValue("772");
    await expect(page.getByRole("row", { name: /AHS-sr-772/ })).toContainText("Acknowledged by GP");
    await noSeriousAxe(page);
    await page.goto(`${NEXT_URL}/app/referrals/returned`);
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
    await expect(page.getByRole("region", { name: /GP, current$/ })).toContainText("The GP has acted");
    await noSeriousAxe(page);
  });

  test("the eye-screening referrer answers a canonical DR packet; the inbox stops escalating it", async ({ page }) => {
    const scope = "programme=diabetic_retinopathy&provider=phc-retina";
    await page.goto(`${NEXT_URL}/app/referrals/returned?${scope}`);
    await expect(page.getByRole("link", { name: "Diabetic eye" })).toHaveAttribute("aria-current", "true");
    const card = page.getByRole("article");
    await expect(card).toContainText("dilated eye examination at an eye clinic");
    await expect(card).toContainText("Returned byEye clinic, regional hospital");
    await expect(card).toContainText("Escalated to the medical director");
    await expect(card).not.toContainText(/FIT|Day 31|SKMC|colonoscopy|endoscopy/);
    await expect(card.getByRole("radio", { name: "Refer to another unit" })).toHaveCount(0);

    await card.getByRole("button", { name: "Acknowledge the returned referral" }).click();
    await expect(card.getByRole("status")).toContainText("Acknowledged");
    await page.reload();
    await expect(page.getByRole("article")).toContainText("AcknowledgedYes");
    await expect(page.getByRole("article")).toContainText("Escalated to the medical director");

    await page.getByRole("radio", { name: "Send the information requested" }).check();
    await page.getByRole("article").getByRole("textbox").first().fill("R3A, new vessels on both eyes");
    await page.getByRole("button", { name: "Send answer" }).click();
    await expect(page.getByRole("article").getByRole("alert")).toContainText("Fill in: Relevant clinical information, Expected outcome.");
    expect((await fetch(`http://127.0.0.1:${MOCK_PORT}/mock/dr`).then((r) => r.json())).resolved).toBeNull();
    const boxes = page.getByRole("article").getByRole("textbox");
    await boxes.nth(1).fill("HbA1c 9.1%, on insulin");
    await boxes.nth(2).fill("Eye-clinic assessment and treatment plan");
    await page.getByRole("button", { name: "Send answer" }).click();
    await expect(page.getByRole("article").getByRole("status").first()).toContainText(/no longer waits on you|Nothing else is needed/);
    await page.reload();
    await expect(page.getByRole("article").getByRole("status")).toContainText("Send the information requested");
    expect((await fetch(`http://127.0.0.1:${MOCK_PORT}/mock/dr`).then((r) => r.json())).events).toHaveLength(2);

    // The receiving unit's view of the same packet: the referrer has acted, the canonical packet stays as it was.
    await page.goto(`${NEXT_URL}/app/referrals?${scope}`);
    await expect(page.getByRole("region", { name: /current$/ })).toContainText("The referrer has acted");
    const pane = page.getByRole("region", { name: "Open referral" });
    await expect(pane).toContainText("The referrer has acted");
    await expect(pane).toContainText("Send the information requested. Answered");
    await noSeriousAxe(page);
  });
});
