import { readFileSync, readdirSync } from "node:fs";
import { createServer, type IncomingHttpHeaders, type Server } from "node:http";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Fixtures mode: run against a dev server without WASL_API_URL.
 * Live mode: set REVIEW_LIVE_MOCK=1 and start the dev server with WASL_API_URL=http://127.0.0.1:3293;
 * this spec serves that address with a mock of the Go API that records every POST.
 */
const LIVE = !!process.env.REVIEW_LIVE_MOCK;
const MOCK_PORT = 3293;

const holdId = fixtures.fixtures.review_hold.id;
const desk = `/app/review/${encodeURIComponent(holdId)}`;
const LINK_ITEM = "rv-f0715223bce81123";
const EVIDENCE_ITEM = "rv-25ead56223c7bef0";

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

const rail = (page: Page) => page.getByRole("complementary", { name: "Your signature" });
const prepRow = (page: Page) => page.locator('[data-check="Prep graded"]');
const signedNodes = (page: Page) => rail(page).locator('li[data-kind="human"]');

test.describe("review, fixtures mode", () => {
  test.skip(LIVE, "fixtures mode only");

  test("the queue is rebuilt from the review_hold case and says so", async ({ page }) => {
    await page.goto("/app/review");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Review");
    await expect(page.getByText("Simulated queue: rebuilt from the review_hold case")).toBeVisible();
    await expect(page.getByText(/The sample run has no open review items/)).toContainText("2 review items");
    const row = page.getByRole("link", { name: /Mariam Al Shamsi/ });
    await expect(row).toContainText("Identity · Bowel prep");
    await expect(row).toContainText("NOO-dr-391");
    await page.screenshot({ path: "test-results/screens/review-queue.png", fullPage: true });
    await noSeriousAxe(page);
    await row.click();
    await expect(page).toHaveURL(/\/app\/review\/ahs-mushrif(%2F|\/)ServiceRequest(%2F|\/)AHS-sr-1474$/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Mariam Al Shamsi");
  });

  test("the desk shows both records, both readers and the quote", async ({ page }) => {
    await page.goto(desk);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Mariam Al Shamsi");
    await expect(page.getByText("Case 1 of 1 waiting")).toBeVisible();
    await expect(page.getByText("Held: the report has no date of birth, and neither reader found a bowel prep grade.")).toBeVisible();
    await expect(page.getByText("not on report")).toBeVisible();
    await expect(page.locator("mark")).toHaveText("advanced, under direct vision, to the cecum");
    await expect(prepRow(page)).toContainText("not stated");
    await expect(prepRow(page)).toContainText("reviewers decide");
    await expect(page.locator('[data-check="Caecum reached"]')).toContainText("agreed");

    const why = page.getByRole("button", { name: "Why it was held" });
    await why.click();
    await expect(page.getByRole("button", { name: "Hide why" })).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("img", { name: /Link score 1\.000/ })).toBeVisible();
    await expect(page.getByText(/second identifier \(exact non-placeholder DOB\) or human confirmation/)).toBeVisible();
    await page.screenshot({ path: "test-results/screens/review-desk.png", fullPage: true });
    await noSeriousAxe(page);
  });

  test("identity first, then prep; Close the case shows two signatures and is not saved", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    await expect(r.getByText("Answer both questions to sign.")).toBeVisible();
    await expect(r.getByRole("radiogroup", { name: /Bowel prep/ })).toHaveCount(0);
    await expect(signedNodes(page)).toHaveCount(0);
    await expect(r.locator('li[data-kind="current"]')).toContainText("your signature");

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await expect(r.getByRole("radio", { name: "Same patient", exact: true })).toHaveAttribute("aria-checked", "true");
    await expect(r.getByText("Answer the bowel prep question to sign.")).toBeVisible();
    await expect(r.getByRole("button", { name: "Close the case" })).toHaveCount(0);

    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("button", { name: "Close the case" }).click();

    const result = r.getByRole("status");
    await expect(result).toContainText("Closed with two signatures");
    await expect(result.getByTestId("signature-squares").locator("span")).toHaveCount(2);
    await expect(result.getByText("Demo: not saved")).toBeVisible();
    await expect(signedNodes(page)).toHaveCount(2);
    await expect(signedNodes(page).nth(0)).toContainText("same patient (as reviewer)");
    await expect(signedNodes(page).nth(1)).toContainText("prep adequate (as clinician)");
    await expect(prepRow(page)).toContainText("adequate, signed");
    await expect(result.getByRole("link", { name: "Back to Review" })).toHaveAttribute("href", "/app/review");
    await expect(result.getByText("That was the last case waiting.")).toBeVisible();
    await page.screenshot({ path: "test-results/screens/review-closed.png", fullPage: true });
    await noSeriousAxe(page);

    await r.getByRole("button", { name: "Undo" }).click();
    await expect(r.getByText("Answer both questions to sign.")).toBeVisible();
    await expect(signedNodes(page)).toHaveCount(0);
    await expect(prepRow(page)).toContainText("reviewers decide");
  });

  test("the other three answers name what they sign", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Not adequate" }).click();
    await r.getByRole("button", { name: "Sign: prep not adequate" }).click();
    await expect(r.getByRole("status")).toContainText("Not closed: repeat colonoscopy");
    await expect(prepRow(page)).toContainText("not adequate, signed");
    await r.getByRole("button", { name: "Undo" }).click();

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Ask the unit" }).click();
    await r.getByRole("button", { name: "Sign and ask the unit to grade the prep" }).click();
    await expect(r.getByRole("status")).toContainText("Asked the unit");
    await expect(r.getByRole("status").getByTestId("signature-squares").locator("span")).toHaveCount(1);
    await expect(r.locator('li[data-kind="current"]')).toContainText("Noor endoscopy");
    await r.getByRole("button", { name: "Undo" }).click();

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("radio", { name: "Not the same patient" }).click();
    await expect(r.getByRole("radiogroup", { name: /Bowel prep/ })).toHaveCount(0);
    await r.getByRole("button", { name: "Sign: not the same patient" }).click();
    await expect(r.getByRole("status")).toContainText("Link rejected");
    await expect(prepRow(page)).toContainText("not applied");
    await expect(signedNodes(page)).toHaveCount(1);
  });
});

type Posted = { path: string; headers: IncomingHttpHeaders; body: Record<string, unknown> };

test.describe("review, live mode against a mock API", () => {
  test.skip(!LIVE, "set REVIEW_LIVE_MOCK=1 with the dev server pointed at the mock");
  test.describe.configure({ mode: "serial" });

  const dir = path.join(process.cwd(), "data", "api");
  const read = (f: string) => JSON.parse(readFileSync(path.join(dir, f), "utf8"));
  const cases = readdirSync(dir)
    .filter((f) => f.startsWith("case_"))
    .map((f) => ({ f, json: read(f) }));
  const meta = read("fixtures.json").meta;

  const posted: Posted[] = [];
  let refuse: string | undefined;
  let server: Server;

  test.beforeAll(async () => {
    server = createServer((req, res) => {
      const url = new URL(req.url ?? "/", "http://mock");
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(JSON.stringify(body));
      };
      const p = decodeURIComponent(url.pathname);
      if (req.method === "POST" && p.endsWith("/review")) {
        let raw = "";
        req.on("data", (c) => (raw += c));
        req.on("end", () => {
          const body = JSON.parse(raw);
          posted.push({ path: p, headers: req.headers, body });
          const role = req.headers["x-wasl-role"];
          if (role === refuse) return send(403, { meta, error: `rbac: role "${role}" may not sign this` });
          const review = { case: holdId, item: body.item, decision: body.decision, note: body.note, role, actor: req.headers["x-wasl-actor"], action: "signoff", at: "2026-10-31T10:00:00Z", mode: "api", prev: "genesis", hash: `hash-${posted.length}-0123456789abcdef` };
          send(201, { meta, review });
        });
        return;
      }
      if (p === "/api/dashboard") return send(200, read("dashboard.json"));
      if (p === "/api/fixtures") return send(200, read("fixtures.json"));
      if (p === "/api/cases") return send(200, { meta, cases: [], total: 0, limit: 50, offset: 0 });
      const audit = p.endsWith("/audit");
      const id = p.replace(/^\/api\/cases\//, "").replace(/\/audit$/, "");
      const hit = cases.find(({ f, json }) => f.endsWith("_audit.json") === audit && (audit ? json.case : json.case?.id) === id);
      return hit ? send(200, hit.json) : send(404, { meta, error: "not found" });
    });
    await new Promise<void>((resolve) => server.listen(MOCK_PORT, "127.0.0.1", resolve));
  });

  test.afterAll(async () => {
    await new Promise((resolve) => server.close(resolve));
  });

  test.beforeEach(() => {
    posted.length = 0;
    refuse = undefined;
  });

  test("Close the case posts identity as reviewer, then prep as clinician, and both are saved", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("button", { name: "Close the case" }).click();

    const result = r.getByRole("status");
    await expect(result).toContainText("Closed with two signatures");
    await expect(result).toContainText("Saved: 2 events appended to the review log");
    await expect(result.getByText("Demo: not saved")).toHaveCount(0);
    await expect(r.getByRole("button", { name: "Undo" })).toHaveCount(0);

    expect(posted.map((x) => [x.path, x.headers["x-wasl-role"], x.headers["x-wasl-actor"], x.body])).toEqual([
      [`/api/cases/${holdId}/review`, "reviewer", "Dr Omar Al Kaabi", { item: LINK_ITEM, decision: "same", note: "" }],
      [`/api/cases/${holdId}/review`, "clinician", "Dr Omar Al Kaabi", { item: EVIDENCE_ITEM, decision: "adequate", note: "" }],
    ]);
    await page.screenshot({ path: "test-results/screens/review-live-saved.png", fullPage: true });
  });

  test("a 403 for the clinician role reads as a sentence, and the retry sends only what was refused", async ({ page }) => {
    refuse = "clinician";
    await page.goto(desk);
    const r = rail(page);
    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("button", { name: "Close the case" }).click();

    await expect(r.getByRole("alert")).toHaveText(
      "The identity answer was saved. Not saved: the clinician role is not allowed to sign the bowel prep answer.",
    );
    await expect(r.getByRole("status")).toHaveCount(0);
    await expect(signedNodes(page)).toHaveCount(1);
    expect(posted.map((x) => x.headers["x-wasl-role"])).toEqual(["reviewer", "clinician"]);
    await page.screenshot({ path: "test-results/screens/review-live-403.png", fullPage: true });
    await noSeriousAxe(page);

    refuse = undefined;
    await r.getByRole("button", { name: "Close the case" }).click();
    await expect(r.getByRole("status")).toContainText("Closed with two signatures");
    expect(posted.map((x) => x.body.item)).toEqual([LINK_ITEM, EVIDENCE_ITEM, EVIDENCE_ITEM]);
  });
});
