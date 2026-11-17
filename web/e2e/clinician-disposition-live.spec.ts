import { createHash } from "node:crypto";
import { spawn, type ChildProcess } from "node:child_process";
import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";

/*
 * Issues #180 and #115: the clinician disposition workflow in the browser, against a stateful synthetic mock of the Go
 * API and an isolated Next server. The mock mirrors pipeline/api.go caseAction for these two kinds: identity only from
 * the bearer token, `record_clinician_disposition` for clinician roles only, expected_head checks, and the overlay
 * projections. The real handler, its validation and ledger replay are covered by pipeline/api_clinician_disposition*_test.go
 * and pipeline/api_disposition*_test.go. Synthetic cases only.
 */
const MOCK_PORT = Number(process.env.DISPOSITION_MOCK_PORT ?? 3391);
const NEXT_PORT = Number(process.env.DISPOSITION_NEXT_PORT ?? 3392);
const NEXT_URL = `http://127.0.0.1:${NEXT_PORT}`;
const SHARED = {
  WASL_API_REVIEWER_TOKEN: "synthetic-shared-reviewer-0001",
  WASL_API_WEB_REVIEWER_TOKEN: "synthetic-web-reviewer-00001",
  WASL_API_CLINICIAN_TOKEN: "synthetic-shared-clinician-0001",
  WASL_API_AUDITOR_TOKEN: "synthetic-shared-auditor-00001",
  WASL_API_COORDINATOR_TOKEN: "synthetic-shared-coord-000001",
};
/* Each person's own assigned token, typed into the sign-in form; the Next server never holds them. */
const PEOPLE: Record<string, { role: string; actor: string }> = {
  "disposition-clinician-0001": { role: "clinician", actor: "dr-synthetic-clinician" },
  "disposition-reviewer-00001": { role: "reviewer", actor: "reviewer-synthetic" },
};
const CLINICIAN = "disposition-clinician-0001";
const REVIEWER = "disposition-reviewer-00001";

const dir = path.join(process.cwd(), "data", "api");
const base = JSON.parse(readFileSync(path.join(dir, "case_review_hold.json"), "utf8"));
const baseAudit = JSON.parse(readFileSync(path.join(dir, "case_review_hold_audit.json"), "utf8"));
const meta = base.meta;
const hex = (c: string) => c.repeat(64);

type Case = {
  id: string;
  programme: string;
  provider: string;
  state: string;
  dropoff: string;
  escalations: string[];
  head: string;
  events: Record<string, unknown>[];
  contacts: Record<string, unknown>[];
  plan?: Record<string, unknown>;
  decline?: Record<string, unknown>;
};
const DR = "phc-retina/ServiceRequest/PHC-sr-9001";
const LUNG = "rad-imaging/ServiceRequest/RAD-sr-9002";
const CRC = "ahs-mushrif/ServiceRequest/AHS-sr-9003";
let cases: Map<string, Case>;

function reset() {
  cases = new Map(
    [
      { id: DR, programme: "diabetic_retinopathy", provider: "phc-retina", state: "not_assessable", dropoff: "not_assessable", escalations: ["clinician_decision_no_fundal_view"] },
      { id: LUNG, programme: "lung_nodule", provider: "rad-imaging", state: "out_of_scope", dropoff: "out_of_scope", escalations: ["clinician_decision_outside_fleischner"] },
      { id: CRC, programme: "crc", provider: "ahs-mushrif", state: "open", dropoff: "not_booked", escalations: [] },
    ].map((c, i) => [c.id, { ...c, head: hex(String(i + 1)), events: [], contacts: [] }]),
  );
  /* The coordinator already recorded the patient's decline on the CRC case (Today, issue #115). */
  const crc = cases.get(CRC)!;
  appendEvent(crc, "contact_outcome", { outcome: "declined", channel: "phone" }, "coordinator", "mock-coordinator");
}

function appendEvent(c: Case, kind: string, data: Record<string, unknown>, role: string, actor: string) {
  const at = new Date().toISOString();
  const action = kind.endsWith("disposition") ? "record_clinician_disposition" : "coordinate_case";
  const event: Record<string, unknown> = { type: "coordinator_action", kind, case: c.id, data, role, actor, action, at, head: c.head, seq: c.events.length + 1, mode: "demo", prev: c.events.at(-1)?.hash ?? "genesis" };
  event.hash = createHash("sha256").update(JSON.stringify(event)).digest("hex");
  c.events.push(event);
  c.head = event.hash as string;
  const entry = { at, event: event.hash, actor, ...data };
  if (kind === "contact_outcome") c.contacts.push(entry);
  if (kind === "clinician_disposition") c.plan = entry;
  if (kind === "decline_disposition") c.decline = { ...entry, role };
  return event;
}

function overlay(c: Case, full: boolean) {
  const last = c.contacts.at(-1);
  const o: Record<string, unknown> = {
    head: c.head, actions: c.events.length, owner: "nurse-synthetic", last_action_at: c.events.at(-1)?.at ?? "", ledger_status: "ok", no_answer_streak: 0,
    ...(last ? { last_contact: { at: last.at, event: last.event, outcome: last.outcome, channel: last.channel } } : {}),
  };
  if (c.plan) {
    o.clinician_disposition = full ? c.plan : { at: c.plan.at, event: c.plan.event, hold: c.plan.hold, plan: c.plan.plan, owner: c.plan.owner, review_date: c.plan.review_date };
    if (full) o.clinician_dispositions = [c.plan];
  }
  if (c.decline) {
    const summary = Object.fromEntries(Object.entries(c.decline).filter(([k]) => !["counselling", "safety_net", "actor"].includes(k)));
    o.decline_disposition = full ? c.decline : summary;
  }
  if (full) Object.assign(o, { contact_outcomes: c.contacts, notes: [], canonical_owner: "" });
  return o;
}

function summary(c: Case) {
  return {
    ...base.case, id: c.id, provider: c.provider, programme: c.programme, state: c.state, dropoff: c.dropoff, escalations: c.escalations,
    owner: "nurse-synthetic", closed: false, review: "none", overdue: false, api_overlay: overlay(c, false),
  };
}

function detail(c: Case) {
  return {
    ...base, case: summary(c), current: { ...base.current, loop: c.id, state: c.state, sig: hex("0"), chain: [] },
    decisions: (base.decisions as Record<string, unknown>[]).map((d) => ({ ...d, state: c.state, dropoff: c.dropoff, escalations: c.escalations })),
    review_items: [], signoffs: [], evidence: [], holds: [], api_reviews: c.events, api_review_states: [],
    review_ledger: { status: "ok", events_applied: c.events.length }, api_overlay: overlay(c, true),
  };
}

const HOLD_PLANS = ["repeat_diagnostic", "alternative_diagnostic", "specialist_pathway", "primary_care_pathway"];

/** The Go handler's rules for the two disposition kinds (pipeline/api.go caseAction, validateActionData). */
function act(c: Case, token: string, body: Record<string, unknown>): [number, unknown] {
  const who = PEOPLE[token] ?? (token === SHARED.WASL_API_COORDINATOR_TOKEN ? { role: "coordinator", actor: "mock-coordinator" } : undefined);
  if (!who) return [401, { error: "authentication required" }];
  const { action, expected_head: head, ...data } = body as { action: string; expected_head: string } & Record<string, unknown>;
  if (action !== "clinician_disposition" && action !== "decline_disposition") return [400, { meta, error: "unsupported in this mock" }];
  if (who.role !== "clinician" && who.role !== "senior_clinician") return [403, { meta, error: `role ${who.role} may not record_clinician_disposition` }];
  if ("hold" in data || "decline_event" in data) return [400, { meta, error: "set by the server" }];
  if (action === "clinician_disposition") {
    if (!c.escalations.some((e) => e.startsWith("clinician_decision_"))) return [409, { meta, error: "no clinician hold", conflict: "hold" }];
    if (!["reason", "details", "owner", "review_date"].every((k) => typeof data[k] === "string" && data[k] !== "") || !HOLD_PLANS.includes(data.plan as string)) return [400, { meta, error: "incomplete plan" }];
    data.hold = c.escalations[0];
  } else {
    const last = c.contacts.at(-1);
    if (last?.outcome !== "declined" || c.decline?.decline_event === last.event) return [409, { meta, error: "no pending decline", conflict: "decline" }];
    if (!["reason", "counselling", "safety_net", "plan"].every((k) => typeof data[k] === "string" && data[k] !== "")) return [400, { meta, error: "incomplete disposition" }];
    if (c.programme === "crc" && !data.fit_reoffer) return [400, { meta, error: "fit_reoffer is required for a CRC decline" }];
    data.decline_event = last.event;
  }
  if (head !== c.head) return [409, { meta, error: "stale expected_head", conflict: "head", current_head: c.head }];
  const event = appendEvent(c, action, data, who.role, who.actor);
  return [201, { meta, event, api_overlay: overlay(c, true) }];
}

async function signIn(page: Page, token: string) {
  await page.getByLabel("Review token").fill(token);
  await page.getByRole("button", { name: /Sign in/ }).click();
}

const caseURL = (id: string) => `${NEXT_URL}/app/patients/${encodeURIComponent(id)}`;

test.describe("clinician disposition, live mock", () => {
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
      if (!Object.values(SHARED).includes(token) && !PEOPLE[token]) return send(401, { error: "authentication required" });
      if (route === "/api/fixtures") return send(200, { meta, fixtures: { evidence_closure: summary(cases.get(CRC)!), false_closure_trap: summary(cases.get(CRC)!), review_hold: summary(cases.get(CRC)!) } });
      if (route === "/api/cases") return send(200, { meta, cases: [...cases.values()].map(summary), total: cases.size, limit: 500, offset: 0 });
      if (route.startsWith("/api/people")) return send(503, { meta, error: "no identity projection in this mock" });
      for (const c of cases.values()) {
        if (route === `/api/cases/${c.id}/audit`) return send(200, { ...baseAudit, case: c.id, history: c.events });
        if (route === `/api/cases/${c.id}/actions` && req.method === "POST") {
          const [status, body] = act(c, token, JSON.parse(raw));
          return send(status, body);
        }
        if (route === `/api/cases/${c.id}`) return send(200, detail(c));
      }
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
    throw new Error("disposition Next server did not become ready");
  });
  test.afterAll(async () => {
    next?.kill();
    if (api) await new Promise((resolve) => api.close(resolve));
  });

  for (const [name, id, holdWords] of [
    ["diabetic eye no fundal view", DR, "No fundal view"],
    ["lung nodule outside Fleischner", LUNG, "Outside Fleischner"],
  ] as const) {
    test(`${name}: a clinician records a plan without a review item; it persists and never claims completed care`, async ({ page }) => {
      /* The case offers the workflow although no review item is raised. */
      await page.goto(caseURL(id));
      const card = page.getByRole("region", { name: "Clinician disposition" });
      await expect(card).toContainText(holdWords);
      await expect(card).toContainText("Waiting on a clinician: no plan recorded yet");
      await expect(page.getByRole("link", { name: /Review programme evidence|Open in Review/ })).toHaveCount(0);
      await card.getByRole("link", { name: "Record clinician disposition" }).click();
      await expect(page).toHaveURL(new RegExp(`/disposition$`));

      /* A reviewer may sign in but the API refuses the write; nothing is recorded. */
      await signIn(page, REVIEWER);
      const form = page.getByRole("form", { name: "Plan for the clinician hold" });
      await form.getByLabel("Reason for the decision (required)").fill("No fundal view on two attempts (synthetic)");
      await form.getByLabel("Plan (required)").selectOption("alternative_diagnostic");
      await form.getByLabel("Plan details (required)").fill("Slit-lamp examination at the eye clinic (synthetic)");
      await form.getByLabel("Responsible owner (required)").fill("Dr Synthetic");
      await form.getByLabel("Review date (required)").fill("2026-12-01");
      await form.getByRole("button", { name: "Save clinician plan" }).click();
      await expect(form.getByTestId("disposition-error")).toContainText("Only a clinician or senior clinician");
      expect(cases.get(id)!.events).toHaveLength(0);

      /* Signed in as a clinician, by direct navigation. */
      await page.context().clearCookies();
      await page.goto(`${caseURL(id)}/disposition`);
      await signIn(page, CLINICIAN);
      const form2 = page.getByRole("form", { name: "Plan for the clinician hold" });
      await form2.getByLabel("Reason for the decision (required)").fill("No fundal view on two attempts (synthetic)");
      await form2.getByLabel("Plan (required)").selectOption("alternative_diagnostic");
      await form2.getByLabel("Plan details (required)").fill("Slit-lamp examination at the eye clinic (synthetic)");
      await form2.getByLabel("Responsible owner (required)").fill("Dr Synthetic");
      await form2.getByLabel("Review date (required)").fill("2026-12-01");
      await form2.getByRole("button", { name: "Save clinician plan" }).click();
      await expect(form2.getByTestId("disposition-saved")).toContainText("The hold stays open until qualifying evidence arrives");
      /* The hold persists, so a later revised plan stays possible; the card shows the one recorded. */
      await expect(page.getByTestId("disposition-hold")).toContainText("Plan recorded, not completed care: A different diagnostic test");
      expect(cases.get(id)!.events.map((e) => [e.kind, e.role, e.actor])).toEqual([["clinician_disposition", "clinician", "dr-synthetic-clinician"]]);

      /* After reload, the case shows the clinician, rationale, plan, owner and date; the case stays open. */
      await page.goto(caseURL(id));
      await page.reload();
      const saved = page.getByRole("region", { name: "Clinician disposition" });
      for (const text of ["dr-synthetic-clinician", "No fundal view on two attempts (synthetic)", "A different diagnostic test", "Slit-lamp examination", "Dr Synthetic", "2026-12-01", "not completed care"]) {
        await expect(saved).toContainText(text);
      }
      await expect(saved).not.toContainText("Waiting on a clinician");
      await expect(page.getByTestId("closed-in")).toContainText("Dr Synthetic to review by 2026-12-01");
      await expect(page.locator("main header")).toContainText("clinician plan recorded");
      await expect(page.locator("main")).not.toContainText(/procedure done|loop closed|Closed day/i);
    });
  }

  test("CRC decline: the GP's informed-refusal disposition resolves 'GP to sign' on Today and the case", async ({ page }) => {
    const todayRow = () => page.getByRole("region", { name: "Your list" }).getByRole("button", { name: /AHS-sr-9003/ });
    await page.goto(`${NEXT_URL}/app/today`);
    await page.getByRole("tab", { name: /^Waiting/ }).click();
    await expect(todayRow()).toContainText("Declined · GP to sign");

    await page.goto(caseURL(CRC));
    const card = page.getByRole("region", { name: "Clinician disposition" });
    await expect(card).toContainText("GP to sign the informed refusal");
    await card.getByRole("link", { name: "Record clinician disposition" }).click();
    await signIn(page, CLINICIAN);
    const form = page.getByRole("form", { name: "Informed-refusal disposition" });
    await form.getByLabel("Refusal reason (required)").selectOption("procedure_anxiety");
    await form.getByLabel("Counselling given (required)").fill("Explained the risk of a missed cancer and the FIT alternative (synthetic)");
    /* The safety net is left blank: the browser refuses before anything is sent. */
    await form.getByLabel("FIT re-offer (required for bowel screening)").selectOption("offered_declined");
    await form.getByLabel("Non-procedure end state: informed refusal, no procedure").check();
    await form.getByRole("button", { name: "Save informed-refusal disposition" }).click();
    expect(cases.get(CRC)!.events).toHaveLength(1);
    await form.getByLabel("Safety-net advice (required)").fill("Return with bleeding or a change in bowel habit (synthetic)");
    await form.getByRole("button", { name: "Save informed-refusal disposition" }).click();
    await expect(page.getByTestId("disposition-none")).toBeVisible();
    const written = cases.get(CRC)!.events.at(-1)!;
    expect([written.kind, written.role, (written.data as Record<string, unknown>).decline_event]).toEqual(["decline_disposition", "clinician", cases.get(CRC)!.events[0].hash]);

    await page.goto(caseURL(CRC));
    await page.reload();
    const saved = page.getByRole("region", { name: "Clinician disposition" });
    for (const text of ["dr-synthetic-clinician (clinician)", `event ${String(cases.get(CRC)!.events[0].hash).slice(0, 12)}`, "Anxiety about the procedure", "FIT re-offered and declined", "no procedure was done"]) {
      await expect(saved).toContainText(text);
    }
    await expect(saved).not.toContainText("GP to sign");

    await page.goto(`${NEXT_URL}/app/today`);
    await page.reload();
    await page.getByRole("tab", { name: /^Waiting/ }).click();
    await expect(todayRow()).toContainText("Declined · informed refusal recorded, no procedure");
    await expect(todayRow()).not.toContainText("GP to sign");
    await expect(todayRow()).not.toContainText(/closed|completed/i);
  });

  test("a stale head is refused and nothing is appended", async ({ page }) => {
    reset();
    await page.goto(`${caseURL(DR)}/disposition`);
    await signIn(page, CLINICIAN);
    const form = page.getByRole("form", { name: "Plan for the clinician hold" });
    await expect(form).toBeVisible();
    /* Another write lands after this page read the head. */
    appendEvent(cases.get(DR)!, "note", { text: "concurrent synthetic write" }, "coordinator", "mock-coordinator");
    await form.getByLabel("Reason for the decision (required)").fill("Synthetic reason");
    await form.getByLabel("Plan (required)").selectOption("specialist_pathway");
    await form.getByLabel("Plan details (required)").fill("Synthetic details");
    await form.getByLabel("Responsible owner (required)").fill("Dr Synthetic");
    await form.getByLabel("Review date (required)").fill("2026-12-01");
    await form.getByRole("button", { name: "Save clinician plan" }).click();
    await expect(form.getByTestId("disposition-error")).toContainText("The case changed since you opened it");
    expect(cases.get(DR)!.events.map((e) => e.kind)).toEqual(["note"]);
    expect(cases.get(DR)!.plan).toBeUndefined();
  });
});
