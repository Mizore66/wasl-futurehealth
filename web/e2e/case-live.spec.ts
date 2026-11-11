import { readFileSync } from "node:fs";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";

/*
 * Case detail against a mock of the Go API.
 * Issue #44: the snapshot in data/api has no lung-nodule or breast case detail, so this spec serves real summaries from
 * the committed programme pages inside the served dr_exam_closure detail shape, as pipeline/api.go caseDetail does (the
 * summary's finding object in `case` and in `current`).
 * Issue #42: the consent demo overlay. The mock follows pipeline/api_consent.go: identity only from the bearer token,
 * coordinator or clinician may record, `expected_head` must equal the case's decision signature (`current.sig`), a grant
 * or denial after an opt-out is refused, the overlay is derived from the appended events, and detail splices
 * `api_contact`. Its response shapes match tests/fixtures/consent_*.json (real handler output).
 *
 * Start `next start` yourself with WASL_API_URL=http://127.0.0.1:3295 and the four role tokens (WASL_API_REVIEWER_TOKEN,
 * WASL_API_CLINICIAN_TOKEN, WASL_API_AUDITOR_TOKEN, WASL_API_COORDINATOR_TOKEN), then run this spec with
 * CASE_LIVE_MOCK=1, PLAYWRIGHT_BASE_URL pointing at that server and the same four tokens. (A Playwright-started server
 * never becomes ready: its /app check needs the mock, which starts with the tests.) It is a mock: the real handler's
 * shapes are pinned by the Go contract tests.
 */
const LIVE = !!process.env.CASE_LIVE_MOCK;
const MOCK_PORT = 3295;

type Json = Record<string, unknown>;
const dir = path.join(process.cwd(), "data", "api");
const read = (f: string) => JSON.parse(readFileSync(path.join(dir, f), "utf8"));

const YEAR = new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
const day = (d: number) => YEAR.format(new Date(Date.UTC(2026, 4, 1) + Math.round(d * 86_400_000)));
const caseHref = (id: string) => `/app/patients/${encodeURIComponent(id)}`;

function value(page: Page, card: string, label: string) {
  return page
    .getByRole("region", { name: card })
    .locator("dt", { hasText: new RegExp(`^${label}$`) })
    .locator("xpath=following-sibling::dd[1]");
}

async function noSideScroll(page: Page, width: number) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
}

test.describe("case detail, live mode against a mock API", () => {
  test.skip(!LIVE, "set CASE_LIVE_MOCK=1 with the server pointed at the mock");
  test.describe.configure({ mode: "serial" });

  const fx = read("fixtures.json");
  const meta = fx.meta;
  const drFixture: Json = read("case_dr_exam_closure.json");
  const pages = ["cases_lung_nodule_page1.json", "cases_diabetic_retinopathy_page1.json", "cases_breast_page1.json"].map(read);
  const byId = new Map<string, Json>();
  for (const c of [...Object.values(fx.fixtures), ...pages.flatMap((p) => p.cases)] as Json[]) if (!byId.has(c.id as string)) byId.set(c.id as string, c);
  const FINDING = ["lung", "dr", "breast", "ldct", "ifhas"] as const;

  /* Consent overlay state (pipeline/api_consent.go overlayLocked). `rematerialised` cases have a new decision signature. */
  type ConsentEv = Json & { decision: string; hash: string };
  const STATUS: Record<string, string> = { consent_granted: "granted", consent_denied: "denied", consent_opted_out: "opted_out" };
  const consent = new Map<string, ConsentEv[]>();
  const rematerialised = new Set<string>();
  let refuseWith: { status: number; error: string } | null = null;
  let prev = "genesis";
  let n = 0;
  const sigFor = (id: string) => `${id === (drFixture.case as Json).id ? (drFixture.current as Json).sig : `sig-${id}`}${rematerialised.has(id) ? "-v2" : ""}`;
  function overlayOf(id: string): Json {
    const h = consent.get(id) ?? [];
    const status = h.length ? STATUS[h.at(-1)!.decision] : "none";
    return {
      status,
      demo_pending_consent: status === "granted" ? "released" : status === "none" ? "" : "cancelled",
      events: h.length,
      contact_blocked: status === "denied" || status === "opted_out",
      mode: "demo",
      canonical_consent: byId.get(id)?.consent ?? "unknown",
      canonical_unchanged: true,
      last_event_hash: h.at(-1)?.hash ?? "",
      note: "demo overlay derived from the review ledger; canonical case.consent and the decision's outreach are not changed and no message is sent",
    };
  }
  function appendConsent(id: string, decision: string, actor: string, role: string): ConsentEv {
    const ev: ConsentEv = { case: id, item: "", decision, note: "", role, actor, action: "record_consent", at: new Date().toISOString(), head_signed: sigFor(id), mode: "demo", prev, hash: `${(++n).toString(16).padStart(8, "0")}${"cd".repeat(28)}` };
    prev = ev.hash;
    consent.set(id, [...(consent.get(id) ?? []), ev]);
    return ev;
  }

  /** A served summary inside the served detail shape; only the finding card reads the programme fields. */
  function detailFor(id: string): Json | null {
    const contact = { api_reviews: consent.get(id) ?? [], api_contact: { overlay: overlayOf(id), history: consent.get(id) ?? [] } };
    if (id === (drFixture.case as Json).id) return { ...drFixture, current: { ...(drFixture.current as Json), sig: sigFor(id) }, ...contact };
    const summary = byId.get(id);
    if (!summary) return null;
    const current: Json = { ...(drFixture.current as Json), id: `dec-${id}`, loop: id, state: summary.state, chain: [], sig: sigFor(id) };
    for (const k of FINDING) delete current[k];
    for (const k of FINDING) if (summary[k]) current[k] = summary[k];
    return { ...drFixture, case: summary, current, identities: [], timeline: [], links: [], evidence: [], holds: [], outreach: [], decisions: [], review_items: [], signoffs: [], ...contact };
  }

  const principals = new Map<string, { role: string; actor: string }>();
  let server: Server;

  function postConsent(req: IncomingMessage, id: string, who: { role: string; actor: string }, send: (status: number, body: unknown) => void) {
    let raw = "";
    req.on("data", (c) => (raw += c));
    req.on("end", () => {
      if (refuseWith) return send(refuseWith.status, { meta, error: refuseWith.error });
      if (!byId.has(id)) return send(404, { meta, error: "unknown case", id });
      if (who.role !== "coordinator" && who.role !== "clinician") return send(403, { meta, error: `rbac: role "${who.role}" may not record_consent`, role: who.role, action: "record_consent" });
      const { event, expected_head } = JSON.parse(raw);
      if (!STATUS[event]) return send(400, { meta, error: "event must be consent_granted|consent_denied|consent_opted_out" });
      if (expected_head !== sigFor(id)) return send(409, { meta, error: "stale expected_head: the case has moved on; reload and retry", current_head: sigFor(id) });
      if (overlayOf(id).status === "opted_out" && event !== "consent_opted_out") {
        return send(409, { meta, error: "patient has opted out; a later grant needs a new, separately recorded consent flow", api_contact: overlayOf(id) });
      }
      const ev = appendConsent(id, event, who.actor, who.role);
      send(201, { meta, review: ev, overlay: overlayOf(id), note: "appended to the review ledger; demo overlay only. Canonical consent and outreach are unchanged and nothing was sent." });
    });
  }

  function handle(req: IncomingMessage, res: ServerResponse) {
    const url = new URL(req.url ?? "/", "http://mock");
    const send = (status: number, body: unknown) => {
      res.writeHead(status, { "content-type": "application/json" });
      res.end(JSON.stringify(body));
    };
    const p = decodeURIComponent(url.pathname);
    if (p === "/healthz") return send(200, read("healthz.json"));
    const [scheme, token] = (req.headers.authorization ?? "").split(" ");
    const who = scheme === "Bearer" && token ? principals.get(token) : undefined;
    if (!who) return send(401, { error: "authentication required" });
    if (p.endsWith("/consent")) {
      const id = p.replace(/^\/api\/cases\//, "").replace(/\/consent$/, "");
      if (req.method === "POST") return postConsent(req, id, who, send);
      if (!byId.has(id)) return send(404, { meta, error: "unknown case", id });
      return send(200, { meta, case: id, overlay: overlayOf(id), history: consent.get(id) ?? [] });
    }
    if (p === "/api/dashboard") return send(200, read("dashboard.json"));
    if (p === "/api/fixtures") return send(200, fx);
    if (p === "/api/cases") {
      const cases = [...byId.values()];
      return send(200, { meta, cases, total: cases.length, limit: 500, offset: 0 });
    }
    if (p.endsWith("/audit")) return send(404, { meta, error: "unknown case" });
    const detail = detailFor(p.replace(/^\/api\/cases\//, ""));
    return detail ? send(200, detail) : send(404, { meta, error: "unknown case" });
  }

  test.beforeAll(async () => {
    for (const role of ["reviewer", "clinician", "auditor", "coordinator"]) {
      const name = `WASL_API_${role.toUpperCase()}_TOKEN`;
      const token = process.env[name]?.trim();
      if (!token) throw new Error(`${name} must be set for the live mock, with the same value the server sends`);
      principals.set(token, { role, actor: `mock-${role}` });
    }
    server = createServer(handle);
    await new Promise<void>((resolve) => server.listen(MOCK_PORT, "127.0.0.1", resolve));
  });

  test.afterAll(async () => {
    await new Promise((resolve) => server.close(resolve));
  });

  test.beforeEach(() => {
    consent.clear();
    rematerialised.clear();
    refuseWith = null;
  });

  test.describe("consent demo overlay (issue #42)", () => {
    /* Canonical consent "denied", so an overlay grant visibly leaves the simulation's consent as it was. */
    const id = "bsu-screen/ServiceRequest/BSU-sr-13";
    const overlayRegion = (page: Page) => page.getByRole("region", { name: "Demo consent overlay" });
    const events = (page: Page) => overlayRegion(page).getByRole("list", { name: "Demo consent events" }).getByRole("listitem");

    async function record(page: Page, label: string, confirm: string) {
      const region = overlayRegion(page);
      await region.getByRole("button", { name: label, exact: true }).click();
      await expect(region.getByText(DEMO_WORDS)).toHaveCount(2); // section note and the confirmation
      await region.getByRole("button", { name: confirm, exact: true }).click();
    }
    const DEMO_WORDS = "Demo overlay: recorded on the case ledger, it never changes the simulation's consent, outreach or decisions, and no message is sent.";

    test("grant releases the held demo message, appends one event by the API's actor, and leaves canonical consent alone", async ({ page }) => {
      await page.goto(caseHref(id));
      const region = overlayRegion(page);
      await expect(region.getByTestId("consent-overlay-now")).toHaveText("No demo consent event yet: nothing held or released by the demo yet.");
      await expect(region.getByRole("list", { name: "Demo consent events" })).toHaveCount(0);

      await region.getByRole("button", { name: "Consent granted", exact: true }).click();
      await expect(region).toContainText("Record that the patient allowed contact?");
      await region.getByRole("button", { name: "Cancel" }).click();
      await expect(region).not.toContainText("Record that the patient allowed contact?");
      expect(consent.size).toBe(0);

      await record(page, "Consent granted", "Record consent granted");
      await expect(region.getByTestId("consent-saved")).toContainText("Recorded on the case ledger: Consent granted · mock-coordinator");
      await expect(region.getByTestId("consent-overlay-now")).toHaveText("Consent granted: demo hold released.");
      await expect(events(page)).toHaveCount(1);
      await expect(events(page).first()).toContainText("Consent granted · demo hold released");
      expect(consent.get(id)!.map((e) => [e.decision, e.actor, e.role, e.head_signed])).toEqual([["consent_granted", "mock-coordinator", "coordinator", sigFor(id)]]);

      // Canonical consent and its history are unchanged; the event is not listed as a sign-off.
      const card = page.getByRole("region", { name: "Consent and messages" });
      await expect(card).toContainText("Now: Declined text messages");
      await expect(region).toContainText("Simulation consent unchanged: declined text messages.");
      await expect(page.getByRole("region", { name: "Human sign-offs" })).toContainText("No human sign-offs recorded for this case.");

      await page.reload();
      await expect(events(page)).toHaveCount(1);
      await expect(overlayRegion(page).getByTestId("consent-overlay-now")).toHaveText("Consent granted: demo hold released.");
    });

    test("denial and opt-out cancel queued demo outreach; a repeated opt-out appends; history stays append-only", async ({ page }) => {
      await page.goto(caseHref(id));
      const region = overlayRegion(page);
      await record(page, "Consent denied", "Record consent denied");
      await expect(region.getByTestId("consent-overlay-now")).toHaveText("Consent denied: demo outreach cancelled; contact confirmation blocked.");
      await record(page, "Consent granted", "Record consent granted");
      await expect(region.getByTestId("consent-overlay-now")).toHaveText("Consent granted: demo hold released.");
      await record(page, "Opted out", "Record opt-out");
      await expect(region.getByTestId("consent-overlay-now")).toHaveText("Opted out: demo outreach cancelled; contact confirmation blocked.");
      await record(page, "Opted out", "Record opt-out");
      await expect(events(page)).toHaveCount(4);
      await expect(events(page)).toHaveText([/Consent denied · demo outreach cancelled/, /Consent granted · demo hold released/, /Opted out · demo outreach cancelled/, /Opted out · demo outreach cancelled/]);
      expect(consent.get(id)!.map((e) => e.decision)).toEqual(["consent_denied", "consent_granted", "consent_opted_out", "consent_opted_out"]);

      // After an opt-out, grant and denial are not offered, as the API refuses them.
      await expect(region.getByRole("button", { name: "Consent granted", exact: true })).toBeDisabled();
      await expect(region.getByRole("button", { name: "Consent denied", exact: true })).toBeDisabled();
      await expect(region.getByTestId("consent-disabled-reason")).toContainText("The patient opted out");
      await page.screenshot({ path: "test-results/screens/case-live-consent-1440.png", fullPage: true });
    });

    test("a stale head shows a conflict, records nothing, and a reload lets the write through", async ({ page }) => {
      await page.goto(caseHref(id));
      rematerialised.add(id); // the run was re-materialised after the page loaded: new decision signature
      await record(page, "Consent granted", "Record consent granted");
      const alert = overlayRegion(page).getByTestId("consent-refused");
      await expect(alert).toHaveAttribute("role", "alert");
      await expect(alert).toContainText("This case changed since the page loaded");
      expect(consent.size).toBe(0);
      await alert.getByRole("button", { name: "Reload the case" }).click();
      await expect(overlayRegion(page).getByTestId("consent-refused")).toHaveCount(0);
      await record(page, "Consent granted", "Record consent granted");
      await expect(overlayRegion(page).getByTestId("consent-overlay-now")).toHaveText("Consent granted: demo hold released.");
      expect(consent.get(id)!.map((e) => e.head_signed)).toEqual([sigFor(id)]);
    });

    test("an opt-out recorded elsewhere after the page loaded: the API's refusal is shown in place", async ({ page }) => {
      await page.goto(caseHref(id));
      appendConsent(id, "consent_opted_out", "another-coordinator", "coordinator");
      await record(page, "Consent granted", "Record consent granted");
      const alert = overlayRegion(page).getByTestId("consent-refused");
      await expect(alert).toContainText("The API refused this consent event");
      await expect(alert).toContainText("patient has opted out");
      await expect(alert.getByRole("button", { name: "Reload the case" })).toHaveCount(0);
      expect(consent.get(id)!.map((e) => e.decision)).toEqual(["consent_opted_out"]);
    });

    for (const [status, error, title] of [
      [400, "event must be consent_granted|consent_denied|consent_opted_out", "The API rejected the request"],
      [403, 'rbac: role "reviewer" may not record_consent', "This role may not record consent"],
      [404, "unknown case", "The API does not know this case"],
    ] as const) {
      test(`a ${status} is shown in place and records nothing`, async ({ page }) => {
        await page.goto(caseHref(id));
        refuseWith = { status, error };
        await record(page, "Consent denied", "Record consent denied");
        const alert = overlayRegion(page).getByTestId("consent-refused");
        await expect(alert).toContainText(title);
        if (status !== 404) await expect(alert).toContainText(error);
        expect(consent.size).toBe(0);
      });
    }

    test("at 390px the overlay and its confirmation fit, with 44px targets", async ({ page }) => {
      await page.setViewportSize({ width: 390, height: 844 });
      appendConsent(id, "consent_denied", "mock-coordinator", "coordinator");
      await page.goto(caseHref(id));
      const region = overlayRegion(page);
      await region.getByRole("button", { name: "Opted out", exact: true }).click();
      await expect(region.getByRole("button", { name: "Record opt-out" })).toBeVisible();
      await noSideScroll(page, 390);
      for (const b of await region.getByRole("button").all()) {
        const box = (await b.boundingBox())!;
        expect(box.height).toBeGreaterThanOrEqual(44);
      }
      await page.screenshot({ path: "test-results/screens/case-live-consent-390.png", fullPage: true });
    });
  });

  for (const width of [1440, 390] as const) {
    test.describe(`clinical findings at ${width}px (issue #44)`, () => {
      test.use({ viewport: { width, height: width === 390 ? 844 : 900 } });

      test("lung nodule: Fleischner category and risk window, open and closed", async ({ page }) => {
        const card = "Incidental lung nodule (Fleischner)";
        for (const id of ["rad-imaging/ServiceRequest/RAD-sr-1", "rad-imaging/ServiceRequest/RAD-sr-101"]) {
          const lung = byId.get(id)!.lung as Json & { rec: Json };
          expect((await page.goto(caseHref(id)))?.status()).toBe(200);
          await expect(value(page, card, "Nodule")).toHaveText(lung.finding_label as string);
          await expect(value(page, card, "Fleischner category")).toHaveText(lung.rec.cell as string);
          await expect(value(page, card, "Patient risk")).toHaveText(lung.risk === "high" ? "High" : "Low");
          await expect(value(page, card, "Risk window")).toHaveText(
            `${lung.rec.win_lo_days} to ${lung.rec.win_hi_days} days, ${day(lung.window_start_day as number)} to ${day(lung.window_end_day as number)}`,
          );
          await expect(value(page, card, "Follow-up outcome")).toHaveText(lung.outcome === "stable" ? "Stable" : "Pending: no follow-up CT read yet");
          await noSideScroll(page, width);
          await page.screenshot({ path: `test-results/screens/case-live-lung-${id.split("-").pop()}-${width}.png`, fullPage: true });
        }
      });

      test("diabetic retinopathy: grade, urgency and next due, pending and closed", async ({ page }) => {
        const card = "Diabetic retinopathy screening";
        const pending = byId.get("phc-retina/ServiceRequest/PHC-sr-1")!.dr as Json & { rec: Json };
        await page.goto(caseHref("phc-retina/ServiceRequest/PHC-sr-1"));
        await expect(value(page, card, "Grade")).toHaveText(`${pending.screen_label} (coded result matched the text)`);
        await expect(value(page, card, "Urgency")).toHaveText("Urgent");
        await expect(value(page, card, "Outcome")).toHaveText("Pending");
        await expect(value(page, card, "Next due")).toHaveText("Set once the loop resolves");
        await noSideScroll(page, width);

        const closed = (drFixture.case as Json).dr as Json & { next_due: Json };
        await page.goto(caseHref((drFixture.case as Json).id as string));
        await expect(value(page, card, "Outcome")).toHaveText("Examined, follow-up");
        await expect(value(page, card, "Next due")).toHaveText(`Clinic review ${day(closed.next_due.due_day as number)} (note review interval)`);
        await noSideScroll(page, width);
      });

      test("breast: BI-RADS, recall recommendation and outcome, recall pending and routine", async ({ page }) => {
        const card = "Breast screening (BI-RADS)";
        const recall = byId.get("bsu-screen/ServiceRequest/BSU-sr-15")!.breast as Json & { rec: Json };
        await page.goto(caseHref("bsu-screen/ServiceRequest/BSU-sr-15"));
        await expect(value(page, card, "BI-RADS category")).toHaveText(`${recall.screen_label} (coded assessment matched the text)`);
        await expect(value(page, card, "Recall recommendation")).toHaveText(`Recall for assessment, ${recall.rec.urgency}`);
        await expect(value(page, card, "Deadline")).toHaveText(`${recall.rec.deadline_days} days, due ${day(recall.deadline_day as number)}`);
        await expect(value(page, card, "Assessment document")).toHaveText("Pending: none read yet");
        await expect(value(page, card, "Outcome")).toHaveText("Pending");
        await noSideScroll(page, width);
        await page.screenshot({ path: `test-results/screens/case-live-breast-recall-${width}.png`, fullPage: true });

        await page.goto(caseHref("bsu-screen/ServiceRequest/BSU-sr-1"));
        await expect(value(page, card, "Recall recommendation")).toHaveText("Routine screening, no recall");
        await expect(value(page, card, "Outcome")).toHaveText("None: no recall");
        await noSideScroll(page, width);
      });
    });
  }
});
