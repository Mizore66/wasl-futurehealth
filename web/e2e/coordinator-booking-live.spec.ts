import { createHash } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

/* Stateful synthetic API + isolated Next server. The real transition validator is covered in pipeline/api_booking_test.go. */
const MOCK_PORT = Number(process.env.BOOKING_MOCK_PORT ?? 3387);
const NEXT_PORT = Number(process.env.BOOKING_NEXT_PORT ?? 3388);
const NEXT_URL = `http://127.0.0.1:${NEXT_PORT}`;
const SHARED = {
  WASL_API_REVIEWER_TOKEN: "synthetic-shared-reviewer-0001",
  WASL_API_WEB_REVIEWER_TOKEN: "synthetic-web-reviewer-00001",
  WASL_API_CLINICIAN_TOKEN: "synthetic-shared-clinician-0001",
  WASL_API_AUDITOR_TOKEN: "synthetic-shared-auditor-00001",
  WASL_API_COORDINATOR_TOKEN: "synthetic-shared-coord-000001",
};
const dir = path.join(process.cwd(), "data", "api");
const base = JSON.parse(readFileSync(path.join(dir, "case_review_hold.json"), "utf8"));
const baseAudit = JSON.parse(readFileSync(path.join(dir, "case_review_hold_audit.json"), "utf8"));
const meta = base.meta;
const id = base.case.id as string;
const caseURL = `${NEXT_URL}/app/patients/${encodeURIComponent(id)}`;
const hex = (c: string) => c.repeat(64);
const slot = (session: number, day: number) => ({
  slot_id: `sim:colonoscopy:${session}:1`, session_id: session, unit: `AD-${session - 200}`, day, region: "AD", deadline: 31,
  within_deadline: day <= 31, endoscopist_female: false, arabic: true, after_hours: false, prep_send_day: day - 5,
  predicted_no_show: 0.1396, nurse_call: false, cross_region: false,
});
const candidates = [slot(201, 20), slot(202, 25), slot(203, 28)];

type Event = {
  kind: string;
  hash: string;
  at: string;
  data: Record<string, unknown>;
  [key: string]: unknown;
};
type ReservationSlot = { slot_id: string; session_id: number; unit: string; day: number; procedure: string; deadline: number };
type Reservation = {
  status: string;
  mode: string;
  revision: number;
  reservation_id: string | null;
  reservation_version: string | null;
  slot: ReservationSlot | null;
  updated_at: string | null;
  updated_event: string | null;
};
type State = {
  head: string;
  consent: string;
  canonicalState: string;
  events: Event[];
  reservation: Reservation;
  versions: Record<string, string>;
  occupied: Record<string, string>;
};
let state: State;

function reset() {
  state = {
    head: hex("0"), consent: "granted", canonicalState: "open", events: [], versions: {}, occupied: {},
    reservation: {
      status: "unreserved", mode: "simulated", revision: 0, reservation_id: null, reservation_version: null,
      slot: null, updated_at: null, updated_event: null,
    },
  };
  for (const candidate of candidates) state.versions[candidate.slot_id] = hex("3");
}
reset();

function overlay(full: boolean) {
  const baseOverlay: Record<string, unknown> = {
    head: state.head, actions: state.events.length, owner: "someone", last_action_at: state.events.at(-1)?.at ?? "",
    ledger_status: "ok", no_answer_streak: 0, booking: state.reservation,
  };
  if (full) Object.assign(baseOverlay, { contact_outcomes: [], notes: [], canonical_owner: "" });
  return baseOverlay;
}

function booking() {
  return {
    status: "suggested", data_mode: "simulated", decision_support_only: true, procedure: "colonoscopy", requested_day: 5,
    region: "AD", state_at_run_end: state.canonicalState, suggestion_version: hex("2"), calendar_version: hex("3"),
    suggested_slot_id: candidates[0].slot_id, reservable: true,
    booking: { ...candidates[0], overbooked: false, booked_on: 5 },
    constraints: { female_endoscopist: "none", arabic_only: false, region: "AD", travel_ok: false },
    eligible_slots: candidates.map((candidate) => ({
      ...candidate, version: state.versions[candidate.slot_id], availability: state.occupied[candidate.slot_id] ? "reserved" : "available",
    })),
  };
}

function detail() {
  const caseSummary = {
    ...base.case, id, provider: "ahs-mushrif", programme: "crc", state: state.canonicalState, dropoff: state.canonicalState === "no_show" ? "no_show" : "not_booked", owner: "someone",
    consent: state.consent, closed: false, review: "none", api_overlay: overlay(false),
  };
  return {
    ...base, case: caseSummary, current: { ...base.current, state: state.canonicalState, now: 40, sig: hex("0") },
    review_items: [], signoffs: [], api_reviews: state.events, api_review_states: [], review_ledger: { status: "ok", events_applied: state.events.length },
    api_overlay: overlay(true), booking: booking(),
  };
}

function conflict(error: string, kind: string, extra: Record<string, unknown> = {}) {
  return { meta, error, conflict: kind, current_head: state.head, ...extra };
}

function append(body: Record<string, unknown>) {
  const kind = body.action as string;
  if (body.expected_head !== state.head) return { status: 409, body: conflict("stale case head", "head") };
  if (kind !== "booking_cancel" && body.expected_suggestion_version !== hex("2")) {
    return { status: 409, body: conflict("suggestion changed", "suggestion", { current_suggestion_version: hex("2") }) };
  }
  if (kind !== "booking_cancel" && state.consent !== "granted") return { status: 409, body: conflict("consent denied", "consent", { consent: state.consent }) };
  const active = state.reservation.status === "reserved";
  if ((kind === "booking_accept" || kind === "booking_choose_slot") && active) return { status: 409, body: conflict("already reserved", "state", { booking: state.reservation }) };
  if ((kind === "booking_reschedule" || kind === "booking_cancel") && !active) return { status: 409, body: conflict("not reserved", "state", { booking: state.reservation }) };
  if ((kind === "booking_reschedule" || kind === "booking_cancel") && body.expected_reservation_version !== state.reservation.reservation_version) {
    return { status: 409, body: conflict("reservation changed", "reservation", { current_reservation_version: state.reservation.reservation_version }) };
  }
  if (kind === "booking_accept" && body.slot_id !== candidates[0].slot_id) return { status: 409, body: conflict("not suggested", "state") };
  const targetID = body.slot_id as string | undefined;
  if (kind !== "booking_cancel") {
    if (!targetID || !state.versions[targetID]) return { status: 400, body: { meta, error: "unknown slot" } };
    if (state.versions[targetID] !== body.expected_slot_version || state.occupied[targetID]) {
      return { status: 409, body: conflict("slot occupied or stale", "slot", {
        slot_id: targetID, current_slot_version: state.versions[targetID], availability: state.occupied[targetID] ? "reserved" : "available",
      }) };
    }
  }
  if (kind !== "booking_accept" && (typeof body.reason !== "string" || !body.reason.trim())) return { status: 400, body: { meta, error: "reason required" } };

  const sequence = state.events.length + 1;
  const at = new Date(Date.parse("2026-11-13T10:00:00Z") + sequence * 1000).toISOString();
  const previous = state.events.at(-1)?.hash ?? "genesis";
  const data = { ...body };
  delete data.action;
  delete data.expected_head;
  const event: Event = {
    type: "coordinator_action", kind, case: id, data, role: "coordinator", actor: "mock-coordinator", action: "coordinate_case",
    at, head: body.expected_head, seq: sequence, mode: "demo", prev: previous, hash: "",
  };
  event.hash = createHash("sha256").update(JSON.stringify(event)).digest("hex");
  if (state.reservation.slot?.slot_id) {
    const old = state.reservation.slot.slot_id as string;
    delete state.occupied[old];
    state.versions[old] = event.hash;
  }
  if (targetID) {
    state.occupied[targetID] = id;
    state.versions[targetID] = event.hash;
  }
  state.events.push(event);
  state.head = event.hash;
  const selected = candidates.find((candidate) => candidate.slot_id === targetID);
  state.reservation = {
    ...state.reservation, status: kind === "booking_cancel" ? "cancelled" : "reserved", revision: state.reservation.revision + 1,
    reservation_id: kind === "booking_reschedule" || kind === "booking_cancel" ? state.reservation.reservation_id : event.hash,
    reservation_version: event.hash, updated_at: at, updated_event: event.hash,
    ...(selected ? { slot: { slot_id: selected.slot_id, session_id: selected.session_id, unit: selected.unit, day: selected.day, procedure: "colonoscopy", deadline: selected.deadline } } : {}),
  };
  return { status: 201, body: { meta, event, api_overlay: overlay(true), booking: booking(), note: "simulated only" } };
}

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

test.describe("coordinator simulated booking, live mock", () => {
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
      if (route === "/mock/consent" && req.method === "POST") {
        state.consent = JSON.parse(raw).consent;
        return send(200, { consent: state.consent });
      }
      if (route === "/mock/state") return send(200, state);
      if (route === "/mock/no-show" && req.method === "POST") {
        state.canonicalState = "no_show";
        return send(200, { state: state.canonicalState });
      }
      if (route === "/api/fixtures") return send(200, { meta, fixtures: { evidence_closure: detail().case, false_closure_trap: detail().case, review_hold: detail().case } });
      if (route === "/api/cases") return send(200, { meta, cases: [detail().case], total: 1, limit: 500, offset: 0 });
      if (route === `/api/cases/${id}/audit`) return send(200, { ...baseAudit, case: id, history: state.events });
      if (route === `/api/cases/${id}/actions` && req.method === "POST") {
        if (req.headers.authorization !== `Bearer ${SHARED.WASL_API_COORDINATOR_TOKEN}`) return send(401, { error: "authentication required" });
        const result = append(JSON.parse(raw));
        return send(result.status, result.body);
      }
      if (route === `/api/cases/${id}`) return send(200, detail());
      return send(404, { meta, error: "not found", route });
    });
    await new Promise<void>((resolve) => api.listen(MOCK_PORT, "127.0.0.1", resolve));
    next = spawn(path.join(process.cwd(), "node_modules", ".bin", "next"), ["start", "-p", String(NEXT_PORT)], {
      env: { ...process.env, ...SHARED, WASL_API_URL: `http://127.0.0.1:${MOCK_PORT}` }, stdio: "ignore",
    });
    for (let i = 0; i < 120; i++) {
      if (await fetch(`${NEXT_URL}/app/today`).then((r) => r.ok, () => false)) return;
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
    throw new Error("booking Next server did not become ready");
  });
  test.afterAll(async () => {
    next?.kill();
    if (api) await new Promise((resolve) => api.close(resolve));
  });
  test.beforeEach(() => reset());

  test("accept, reschedule, cancel and rebook survive reload with complete simulated history", async ({ page }) => {
    await page.goto(caseURL);
    const card = page.getByRole("region", { name: "Booking" });
    await card.getByRole("button", { name: "Accept simulated slot", exact: true }).click();
    await expect(page.getByRole("dialog")).toContainText("AD-1 · Day 20");
    await expect(page.getByRole("dialog")).toContainText("No provider scheduling system or patient is contacted");
    await page.getByRole("button", { name: "Save simulated reservation", exact: true }).click();
    await expect(card).toContainText("Simulated reservation confirmed");
    await expect(card).toContainText("AD-1 · Day 20");
    await page.reload();
    await expect(card).toContainText("Simulated reservation confirmed");
    await card.getByRole("button", { name: "Reschedule simulated slot", exact: true }).click();
    await page.getByRole("radio", { name: /AD-2 · Day 25/ }).check();
    await page.getByLabel("Reason (required)").fill("Provider list changed");
    await page.getByRole("button", { name: "Save simulated reservation", exact: true }).click();
    await expect(card).toContainText("AD-2 · Day 25");
    await card.getByRole("button", { name: "Cancel simulated reservation", exact: true }).click();
    await expect(page.getByRole("dialog")).toContainText("only releases the synthetic reservation");
    await page.getByLabel("Reason (required)").fill("Patient asked to cancel");
    await page.getByRole("button", { name: "Release synthetic reservation", exact: true }).click();
    await expect(card).toContainText("Simulated reservation cancelled · rebook");
    await card.getByRole("button", { name: "Accept simulated slot", exact: true }).click();
    await page.getByRole("button", { name: "Save simulated reservation", exact: true }).click();
    await expect(card).toContainText("Simulated reservation confirmed");
    await page.reload();
    const ledger = page.getByRole("region", { name: "Coordinator actions" });
    await expect(ledger).toContainText("Simulated reservation moved · sim:colonoscopy:201:1 → sim:colonoscopy:202:1");
    await expect(ledger).toContainText("Provider list changed");
    await expect(ledger).toContainText("Patient asked to cancel");
    await expect(card).not.toContainText(/provider booked|appointment confirmed/i);
    const mock = await (await page.request.get(`http://127.0.0.1:${MOCK_PORT}/mock/state`)).json();
    expect(mock.events.map((event: Event) => event.kind)).toEqual(["booking_accept", "booking_reschedule", "booking_cancel", "booking_accept"]);
  });

  test("choose another slot and Today accept update placement and trail", async ({ page }) => {
    await page.goto(caseURL);
    const card = page.getByRole("region", { name: "Booking" });
    await card.getByRole("button", { name: "Choose another simulated slot", exact: true }).click();
    await page.getByRole("radio", { name: /AD-3 · Day 28/ }).check();
    await page.getByLabel("Reason (required)").fill("Patient requested a later date");
    await page.getByRole("button", { name: "Save simulated reservation", exact: true }).click();
    await expect(card).toContainText("AD-3 · Day 28");

    await page.request.post(`http://127.0.0.1:${MOCK_PORT}/mock/state`); // readback route remains available
    reset();
    await page.goto(`${NEXT_URL}/app/today`);
    const open = page.getByRole("region", { name: "Open patient" });
    await expect(open.getByTestId("booking-controls")).toBeVisible();
    await open.getByRole("button", { name: "Accept simulated slot", exact: true }).click();
    await page.getByRole("button", { name: "Save simulated reservation", exact: true }).click();
    await page.getByRole("tab", { name: /Waiting/ }).click();
    await expect(page.getByRole("region", { name: "Your list" })).toContainText("Simulated slot reserved · Day 20");
    await expect(page.getByRole("region", { name: "Open patient" })).toContainText("Simulated reservation confirmed");
    await expect(page.getByRole("region", { name: "Open patient" })).toContainText("Simulated slot reserved · sim:colonoscopy:201:1");
    await page.reload();
    await page.waitForLoadState("networkidle");
    await page.getByRole("tab", { name: /Waiting/ }).click();
    await expect(page.getByRole("region", { name: "Your list" })).toContainText("Simulated slot reserved · Day 20");
  });

  test("a synthetic no-show can be rebooked without changing canonical state", async ({ page }) => {
    await page.request.post(`http://127.0.0.1:${MOCK_PORT}/mock/no-show`);
    await page.goto(caseURL);
    const card = page.getByRole("region", { name: "Booking" });
    await expect(card).toContainText("No simulated reservation");
    await card.getByRole("button", { name: "Accept simulated slot", exact: true }).click();
    await page.getByRole("button", { name: "Save simulated reservation", exact: true }).click();
    await expect(card).toContainText("Simulated reservation confirmed");
    await page.reload();
    await expect(card).toContainText("Simulated reservation confirmed");
    const mock = await (await page.request.get(`http://127.0.0.1:${MOCK_PORT}/mock/state`)).json();
    expect(mock.canonicalState).toBe("no_show");
    expect(mock.events.map((event: Event) => event.kind)).toEqual(["booking_accept"]);
  });

  test("two tabs produce one success and one refreshed conflict; denied consent cannot write", async ({ browser, page }) => {
    const second = await browser.newPage();
    await Promise.all([page.goto(caseURL), second.goto(caseURL)]);
    for (const tab of [page, second]) {
      await tab.getByRole("region", { name: "Booking" }).getByRole("button", { name: "Accept simulated slot", exact: true }).click();
    }
    await Promise.all([
      page.getByRole("button", { name: "Save simulated reservation", exact: true }).click(),
      second.getByRole("button", { name: "Save simulated reservation", exact: true }).click(),
    ]);
    const mock = await (await page.request.get(`http://127.0.0.1:${MOCK_PORT}/mock/state`)).json();
    expect(mock.events).toHaveLength(1);
    await expect.poll(async () => [
      ...(await page.getByTestId("booking-notice").allTextContents()),
      ...(await second.getByTestId("booking-notice").allTextContents()),
    ].join(" ")).toContain("latest server state");
    await page.reload();
    await expect(page.getByRole("region", { name: "Booking" })).toContainText("Simulated reservation confirmed");
    await second.close();

    reset();
    await page.request.post(`http://127.0.0.1:${MOCK_PORT}/mock/consent`, { data: { consent: "denied" } });
    await page.goto(caseURL);
    await expect(page.getByRole("region", { name: "Booking" }).getByRole("button", { name: "Accept simulated slot", exact: true })).toBeDisabled();
    const denied = await page.request.post(`http://127.0.0.1:${MOCK_PORT}/api/cases/${encodeURIComponent(id)}/actions`, {
      headers: { authorization: `Bearer ${SHARED.WASL_API_COORDINATOR_TOKEN}` },
      data: { action: "booking_accept", expected_head: hex("0"), expected_suggestion_version: hex("2"), slot_id: candidates[0].slot_id, expected_slot_version: hex("3") },
    });
    expect(denied.status()).toBe(409);
  });

  test("keyboard, 390px layout and axe remain usable", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(caseURL);
    const accept = page.getByRole("region", { name: "Booking" }).getByRole("button", { name: "Choose another simulated slot", exact: true });
    await accept.focus();
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await page.keyboard.press("Tab");
    await page.keyboard.press("Tab");
    expect(await dialog.evaluate((node) => node.contains(document.activeElement))).toBe(true);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await noSeriousAxe(page);
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(accept).toBeFocused();
  });
});
