import { readFileSync } from "node:fs";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";

/*
 * Case detail against a mock of the Go API (issue #44). The snapshot in data/api has no lung-nodule or breast case
 * detail, so this spec serves real summaries from the committed programme pages inside the served dr_exam_closure detail
 * shape, as pipeline/api.go caseDetail does (the summary's finding object in `case` and in `current`).
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

// The app's three-letter months ("Sep", not Node ICU's en-GB "Sept"), as lib/domain/day.ts formatDayYear writes them.
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const day = (d: number) => {
  const t = new Date(Date.UTC(2026, 4, 1) + Math.round(d * 86_400_000));
  return `${t.getUTCDate()} ${MONTHS[t.getUTCMonth()]} ${t.getUTCFullYear()}`;
};
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

  /** A served summary inside the served detail shape; only the finding card reads the programme fields. */
  function detailFor(id: string): Json | null {
    if (id === (drFixture.case as Json).id) return drFixture;
    const summary = byId.get(id);
    if (!summary) return null;
    const current: Json = { ...(drFixture.current as Json), id: `dec-${id}`, loop: id, state: summary.state, chain: [], sig: `sig-${id}` };
    for (const k of FINDING) delete current[k];
    for (const k of FINDING) if (summary[k]) current[k] = summary[k];
    return { ...drFixture, case: summary, current, identities: [], timeline: [], links: [], evidence: [], holds: [], outreach: [], decisions: [], review_items: [], signoffs: [], api_reviews: [] };
  }

  const principals = new Map<string, { role: string; actor: string }>();
  let server: Server;

  function handle(req: IncomingMessage, res: ServerResponse) {
    const url = new URL(req.url ?? "/", "http://mock");
    const send = (status: number, body: unknown) => {
      res.writeHead(status, { "content-type": "application/json" });
      res.end(JSON.stringify(body));
    };
    const p = decodeURIComponent(url.pathname);
    if (p === "/healthz") return send(200, read("healthz.json"));
    const [scheme, token] = (req.headers.authorization ?? "").split(" ");
    if (!(scheme === "Bearer" && token && principals.has(token))) return send(401, { error: "authentication required" });
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

        const assumed = byId.get("rad-imaging/ServiceRequest/RAD-sr-111")!.lung as Json & { rec: Json };
        expect(assumed.risk_recorded).toBe(false);
        await page.goto(caseHref("rad-imaging/ServiceRequest/RAD-sr-111"));
        await expect(value(page, card, "Patient risk")).toHaveText("High (assumed: risk not recorded)");
        await expect(value(page, card, "Risk window")).toHaveText(
          `${assumed.rec.win_lo_days} to ${assumed.rec.win_hi_days} days, ${day(assumed.window_start_day as number)} to ${day(assumed.window_end_day as number)} (assumes high risk)`,
        );
        await expect(page.getByRole("region", { name: card }).getByRole("note")).toContainText("rest on that assumption");
        await noSideScroll(page, width);
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
        const routine = byId.get("bsu-screen/ServiceRequest/BSU-sr-1")!.breast as Json & { rec: Json };
        await expect(value(page, card, "Recall recommendation")).toHaveText("Routine screening, no recall");
        await expect(value(page, card, "Next screen")).toHaveText(
          `${routine.rec.deadline_days} days, ${day(routine.deadline_day as number)} (routine interval, no overdue clock)`,
        );
        await expect(value(page, card, "Outcome")).toHaveText("None: no recall");
        await noSideScroll(page, width);
      });
    });
  }
});
