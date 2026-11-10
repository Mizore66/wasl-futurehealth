import { readFileSync, readdirSync } from "node:fs";
import { createServer, type IncomingHttpHeaders, type Server } from "node:http";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Fixtures mode: run against a dev server without WASL_API_URL.
 * Live mode: start `next start` yourself with WASL_API_URL=http://127.0.0.1:3293 and the four role tokens
 * (WASL_API_REVIEWER_TOKEN, WASL_API_CLINICIAN_TOKEN, WASL_API_AUDITOR_TOKEN, WASL_API_COORDINATOR_TOKEN), then run
 * this spec with REVIEW_LIVE_MOCK=1, PLAYWRIGHT_BASE_URL pointing at that server and the same four tokens. (A
 * Playwright-started server never becomes ready: its /app check needs the mock, which starts with the tests.)
 * This spec serves 127.0.0.1:3293 with a mock of the Go API that binds each token to a role and actor, like the real
 * API, and records every POST.
 */
const LIVE = !!process.env.REVIEW_LIVE_MOCK;
const MOCK_PORT = 3293;

const holdId = fixtures.fixtures.review_hold.id;
const desk = `/app/review/${encodeURIComponent(holdId)}`;
const settledDesk = `/app/review/${encodeURIComponent(fixtures.fixtures.evidence_closure.id)}`;
const LINK_ITEM = "rv-636161dc42837e08";
const EVIDENCE_ITEM = "rv-d55f7ccf9f12c409";

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

const rail = (page: Page) => page.getByRole("complementary", { name: "Your answers" });
const prepRow = (page: Page) => page.locator('[data-check="Prep graded"]');
/** The one evidence item covers every check the readers could not settle: bowel prep and tissue taken. */
const tissueRow = (page: Page) => page.locator('[data-check="Tissue taken"]');
const disputedRows = (page: Page) => [prepRow(page), tissueRow(page)];
const signedNodes = (page: Page) => rail(page).locator('li[data-kind="human"]');
const previewNodes = (page: Page) => rail(page).locator('li[data-kind="current"]');

test.describe("review, fixtures mode", () => {
  test.skip(LIVE, "fixtures mode only");

  test("the queue is rebuilt from the review_hold case and says so", async ({ page }) => {
    await page.goto("/app/review");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Review");
    await expect(page.getByText("Simulated queue: rebuilt from a held demo case")).toBeVisible();
    await expect(page.getByText(/The sample run has no open review items/)).toContainText("2 review items");
    const row = page.getByRole("link", { name: /^Mariam Al Shamsi/ });
    await expect(row).toContainText("Identity · Colonoscopy report");
    await expect(row).toContainText("SKM-dr-217");
    await page.screenshot({ path: "test-results/screens/review-queue.png", fullPage: true });
    await noSeriousAxe(page);
    await row.click();
    await expect(page).toHaveURL(/\/app\/review\/ahs-mushrif(%2F|\/)ServiceRequest(%2F|\/)AHS-sr-1079$/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Mariam Al Shamsi");
  });

  test("the patient's avatar opens her record from the queue and the desk", async ({ page }) => {
    const record = /\/app\/patients\/ahs-mushrif(%2F|\/)ServiceRequest(%2F|\/)AHS-sr-1079$/;
    await page.goto("/app/review");
    await page.getByRole("link", { name: "Open Mariam Al Shamsi's record" }).click();
    await expect(page).toHaveURL(record);
    await page.goto(desk);
    await page.getByRole("link", { name: "Open Mariam Al Shamsi's record" }).click();
    await expect(page).toHaveURL(record);
  });

  test("the desk shows both records, both readers and the quote", async ({ page }) => {
    await page.goto(desk);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Mariam Al Shamsi");
    await expect(page.getByText("Case 1 of 1 waiting")).toBeVisible();
    await expect(page.getByText("Held: no second identifier is confirmed for the two records, and the two readers could not settle the bowel prep grade and whether tissue was taken.")).toBeVisible();
    await expect(page.getByText("not confirmed", { exact: true })).toHaveCount(1);
    await expect(page.getByText("Date of birth")).toHaveCount(0);
    await expect(page.locator("mark")).toHaveText("the level of the cecum");
    await expect(prepRow(page)).toContainText("not stated");
    await expect(prepRow(page)).toContainText("reviewers decide");
    // Tissue: the rules reader found none, the second reader found it. Histology agrees (none).
    await expect(tissueRow(page)).toContainText("none");
    await expect(tissueRow(page)).toContainText("yes");
    await expect(tissueRow(page)).toContainText("reviewers decide");
    await expect(page.locator('[data-check="Histology back"]')).toContainText("agreed, none");
    await expect(page.locator('[data-check="Caecum reached"]')).toContainText("agreed");

    const why = page.getByRole("button", { name: "Why it was held" });
    await why.click();
    await expect(page.getByRole("button", { name: "Hide why" })).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("img", { name: /Link score 1\.000/ })).toBeVisible();
    await expect(page.getByText(/second identifier \(exact non-placeholder DOB, or a matching Emirates ID with agreeing name, sex and DOB\) or human confirmation/)).toBeVisible();
    await page.screenshot({ path: "test-results/screens/review-desk.png", fullPage: true });
    await noSeriousAxe(page);
  });

  test("identity first, then the report; Close the case previews the outcome and says nothing was saved", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    await expect(r.getByText("Answer both questions to sign.")).toBeVisible();
    await expect(r.getByRole("group", { name: /colonoscopy report/ })).toHaveCount(0);
    await expect(signedNodes(page)).toHaveCount(0);
    await expect(r.locator('li[data-kind="current"]')).toContainText("your answers");
    await expect(r).toContainText("nothing here is appended or saved");
    const bridge = page.getByRole("region", { name: "Is this the same person?" });
    await expect(bridge.getByRole("img", { name: "Not yet confirmed as this patient" })).toHaveAttribute("data-state", "unknown");

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await expect(bridge.getByRole("img", { name: "Same patient as Mariam Al Shamsi" })).toHaveAttribute("data-state", "resolving");
    await expect(r.getByRole("radio", { name: "Same patient", exact: true })).toBeChecked();
    await expect(r.getByText("Answer the colonoscopy report question to sign.")).toBeVisible();
    // The one evidence question names every check it covers.
    const q2 = r.getByRole("group", { name: "2. Is the colonoscopy report adequate?" });
    await expect(q2).toContainText("neither reader found the bowel prep grade");
    await expect(q2).toContainText("the readers differ on whether tissue was taken: rules reader none, second reader yes");
    await expect(q2).toContainText("One answer covers all of them");
    await expect(r.getByRole("button", { name: /Close the case/ })).toHaveCount(0);

    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("button", { name: "Preview: Close the case" }).click();

    const result = r.getByRole("status");
    await expect(result).toContainText("Would close the case");
    await expect(result).not.toContainText("Closed with");
    await expect(result.getByTestId("signature-squares")).toHaveCount(0);
    await expect(result.getByText("Reconstruction: nothing was saved")).toBeVisible();
    await expect(result).toContainText("No event was appended and no hash exists");
    await expect(signedNodes(page)).toHaveCount(0);
    await expect(previewNodes(page).nth(0)).toContainText("preview, not saved: same patient (as reviewer)");
    await expect(previewNodes(page).nth(1)).toContainText("preview, not saved: report adequate (as clinician)");
    for (const row of disputedRows(page)) await expect(row).toContainText("adequate (preview, not saved)");
    await expect(result.getByRole("link", { name: "Back to Review" })).toHaveAttribute("href", "/app/review");
    await expect(result.getByText("That was the last case waiting.")).toBeVisible();
    await page.screenshot({ path: "test-results/screens/review-closed.png", fullPage: true });
    await noSeriousAxe(page);

    await r.getByRole("button", { name: "Undo" }).click();
    await expect(r.getByText("Answer both questions to sign.")).toBeVisible();
    await expect(previewNodes(page)).toHaveCount(1);
    for (const row of disputedRows(page)) await expect(row).toContainText("reviewers decide");
  });

  test("keyboard only: arrows choose, the second question is announced, Close the case moves focus on", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    const active = () =>
      page.evaluate(() => {
        const el = document.activeElement as HTMLElement | null;
        return { tag: el?.tagName, text: (el?.textContent ?? "").trim(), label: el?.closest("label")?.textContent?.trim() ?? "" };
      });
    // Each group is named by its visible question.
    await expect(r.getByRole("group", { name: "1. Is it the same patient?" })).toBeVisible();

    await r.getByRole("radio", { name: "Same patient", exact: true }).focus();
    expect((await active()).label).toBe("Same patient");
    await page.keyboard.press("ArrowDown");
    expect((await active()).label).toBe("Not the same patient");
    await expect(r.getByRole("radio", { name: "Not the same patient" })).toBeChecked();
    await page.keyboard.press("ArrowUp");
    await expect(r.getByRole("radio", { name: "Same patient", exact: true })).toBeChecked();

    // The second question is announced in a polite live region, and the next Tab lands in it.
    await expect(r.locator("[aria-live=polite]")).toHaveText(/Question 2 of 2 is now showing/);
    await expect(r.getByRole("group", { name: "2. Is the colonoscopy report adequate?" })).toBeVisible();
    await page.keyboard.press("Tab");
    expect((await active()).tag).toBe("INPUT");
    expect((await active()).label).toBe("Adequate");
    await page.keyboard.press("Space");
    await expect(r.getByRole("radio", { name: "Adequate", exact: true })).toBeChecked();
    await page.keyboard.press("Tab");
    expect((await active()).text).toBe("Preview: Close the case");

    await page.keyboard.press("Enter");
    const heading = r.getByRole("heading", { name: "Would close the case" });
    await expect(heading).toBeVisible();
    await expect(heading).toBeFocused();
    await page.keyboard.press("Tab");
    expect((await active()).text).toBe("Back to Review");
    await page.keyboard.press("Tab");
    expect((await active()).text).toBe("Undo");
    await page.keyboard.press("Enter");
    // Undo brings the questions back and focus goes to the first of them.
    await expect(r.getByRole("radio", { name: "Same patient", exact: true })).toBeFocused();
  });

  test("the other three answers name what they sign", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Not adequate" }).click();
    await r.getByRole("button", { name: "Preview: Sign: report not adequate" }).click();
    await expect(r.getByRole("status")).toContainText("Would not close: repeat colonoscopy");
    for (const row of disputedRows(page)) await expect(row).toContainText("not adequate (preview, not saved)");
    await r.getByRole("button", { name: "Undo" }).click();

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Ask the unit" }).click();
    await r.getByRole("button", { name: "Preview: Sign and ask the unit to complete the report" }).click();
    await expect(r.getByRole("status")).toContainText("Would ask the unit");
    await expect(r.locator('li[data-kind="current"]').last()).toContainText("SKMC endoscopy");
    await r.getByRole("button", { name: "Undo" }).click();

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("radio", { name: "Not the same patient" }).click();
    await expect(r.getByRole("group", { name: /colonoscopy report/ })).toHaveCount(0);
    await r.getByRole("button", { name: "Preview: Sign: not the same patient" }).click();
    await expect(r.getByRole("status")).toContainText("Would reject the link");
    for (const row of disputedRows(page)) await expect(row).toContainText("not applied");
    await expect(signedNodes(page)).toHaveCount(0);
  });
});

type Posted = { path: string; headers: IncomingHttpHeaders; role: string; actor: string; body: Record<string, unknown> };

/** The one signer every role token is bound to in this mock: two answers from one person, under two roles. */
const SIGNER = "Dr Omar Al Kaabi";

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
  /** What the mock API has appended: served back as the audit history, like the real log. */
  const history: Record<string, unknown>[] = [];
  let refuse: string | undefined;
  let canonicalClosed = false;
  let settledEvidence: "disagreeing" | "missing" | undefined;
  let server: Server;

  /*
   * Like the Go API (pipeline/api_security.go), the mock takes role and actor only from the bearer token, never from
   * request headers. The tokens are the ones the Next.js server sends (lib/api/source.ts reads WASL_API_<ROLE>_TOKEN).
   */
  const principals = new Map<string, { role: string; actor: string }>();

  test.beforeAll(async () => {
    for (const role of ["reviewer", "clinician", "auditor", "coordinator"]) {
      const name = `WASL_API_${role.toUpperCase()}_TOKEN`;
      const token = process.env[name]?.trim();
      if (!token) throw new Error(`${name} must be set for the live mock, with the same value the server sends`);
      principals.set(token, { role, actor: SIGNER });
    }
    server = createServer((req, res) => {
      const url = new URL(req.url ?? "/", "http://mock");
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(JSON.stringify(body));
      };
      const p = decodeURIComponent(url.pathname);
      const [scheme, token] = (req.headers.authorization ?? "").split(" ");
      const principal = scheme === "Bearer" && token ? principals.get(token) : undefined;
      if (p.startsWith("/api/") && !principal) return send(401, { error: "authentication required" });
      // Only the auditor reads /audit, and the auditor reads nothing else (apiReadAction in pipeline/api_security.go).
      if (p.startsWith("/api/") && (principal?.role === "auditor") !== p.endsWith("/audit")) {
        return send(403, { error: "read permission denied" });
      }
      if (req.method === "POST" && p.endsWith("/review")) {
        let raw = "";
        req.on("data", (c) => (raw += c));
        req.on("end", () => {
          const body = JSON.parse(raw);
          const { role, actor } = principal!;
          posted.push({ path: p, headers: req.headers, role, actor, body });
          if (role === refuse) return send(403, { meta, error: `rbac: role "${role}" may not sign this` });
          const review = { case: holdId, item: body.item, decision: body.decision, note: body.note, role, actor, action: "signoff", at: "2026-10-31T10:00:00Z", mode: "api", prev: "genesis", hash: `hash-${posted.length}-0123456789abcdef` };
          // The auditor's redacted audit history never carries actor or note (exportReviewEventAllow in pipeline/api_export.go).
          history.push(Object.fromEntries(Object.entries(review).filter(([k]) => k !== "actor" && k !== "note")));
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
      if (hit && audit) return send(200, { ...hit.json, history: [...hit.json.history, ...history] });
      if (hit && settledEvidence && hit.json.case.id === fixtures.fixtures.evidence_closure.id) {
        const evidence = settledEvidence === "missing" ? [] : hit.json.evidence.map((e: { agree: boolean }) => ({ ...e, agree: false }));
        return send(200, { ...hit.json, review_items: [], signoffs: [], api_reviews: [], evidence });
      }
      if (hit && hit.json.case.id === holdId) {
        return send(200, { ...hit.json, case: { ...hit.json.case, closed: canonicalClosed } });
      }
      return hit ? send(200, hit.json) : send(404, { meta, error: "not found" });
    });
    await new Promise<void>((resolve) => server.listen(MOCK_PORT, "127.0.0.1", resolve));
  });

  test.afterAll(async () => {
    await new Promise((resolve) => server.close(resolve));
  });

  test.beforeEach(() => {
    posted.length = 0;
    history.length = 0;
    refuse = undefined;
    canonicalClosed = false;
    settledEvidence = undefined;
  });

  test("a 201 with case.closed=false records demo answers without claiming closure", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    const history = page.getByRole("list", { name: "Review history" });
    await expect(page.getByText("Nobody has signed this case through the API yet.")).toBeVisible();
    await expect(history).toHaveCount(0);
    await expect(r).toContainText("not independent double review");

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("button", { name: "Close the case" }).click();

    const result = r.getByRole("status");
    await expect(result).toContainText("Demo answers appended; case open");
    await expect(result).toContainText("Current case data still shows the case as open; closure is not confirmed.");
    await expect(result).not.toContainText("Closed: both answers recorded");
    await expect(result).not.toContainText("case closes");
    await expect(r.getByRole("heading", { name: /Closed/ })).toHaveCount(0);
    await expect(result).toContainText("Saved: 2 events appended to the review log, last hash hash-2-01234");
    await expect(result.getByRole("list", { name: "Appended events" }).getByRole("listitem")).toHaveText([
      /Reviewer · Dr Omar Al Kaabi · hash-1-01234/,
      /Clinician · Dr Omar Al Kaabi · hash-2-01234/,
    ]);
    await expect(result).toContainText("Both answers came from Dr Omar Al Kaabi");
    await expect(result).toContainText("not two independent reviewers");
    await expect(result).not.toContainText("two signatures");
    await expect(result.getByText("Reconstruction: nothing was saved")).toHaveCount(0);
    await expect(result.getByText("That was the last case waiting.")).toHaveCount(0);
    await expect(r.getByRole("button", { name: "Undo" })).toHaveCount(0);
    await expect(signedNodes(page)).toHaveCount(2);
    await expect(signedNodes(page).nth(0)).toContainText("Dr Omar Al Kaabi · same patient (as reviewer)");
    await expect(signedNodes(page).nth(1)).toContainText("Dr Omar Al Kaabi · report adequate (as clinician)");
    await expect(r.locator('li[data-kind="current"]').last()).not.toContainText("Closed");

    // The history on the page was read again from the log, with no reload.
    await expect(history.getByRole("listitem")).toHaveCount(2);
    await expect(history.getByRole("listitem").nth(0)).toContainText("Same patient");
    await expect(history.getByRole("listitem").nth(0)).toContainText("Reviewer");
    await expect(history.getByRole("listitem").nth(1)).toContainText("Clinician");
    // The auditor's history is redacted: no actor is shown, and no placeholder for one.
    await expect(history).not.toContainText(SIGNER);
    await expect(history).not.toContainText("undefined");
    await expect(history).not.toContainText("Unnamed");

    // Focus moved to the confirmation, not <body>.
    await expect(r.getByRole("heading", { name: /Demo answers appended; case open/ })).toBeFocused();

    expect(posted.map((x) => [x.path, x.role, x.actor, x.body])).toEqual([
      [`/api/cases/${holdId}/review`, "reviewer", SIGNER, { item: LINK_ITEM, decision: "same", note: "" }],
      [`/api/cases/${holdId}/review`, "clinician", SIGNER, { item: EVIDENCE_ITEM, decision: "adequate", note: "" }],
    ]);
    // Identity comes from the token alone: the server never asserts a role or actor header.
    expect(posted.map((x) => [x.headers["x-wasl-role"], x.headers["x-wasl-actor"]])).toEqual([
      [undefined, undefined],
      [undefined, undefined],
    ]);
    await page.screenshot({ path: "test-results/screens/review-live-saved.png", fullPage: true });
    await noSeriousAxe(page);
  });

  test("a 201 can confirm closure only after refreshed canonical data is closed", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    canonicalClosed = true;
    await r.getByRole("button", { name: "Close the case" }).click();

    const result = r.getByRole("status");
    await expect(result).toContainText("Closed in refreshed case data");
    await expect(result).toContainText("Refreshed case data confirms closure.");
    await expect(result).not.toContainText("case open");
    await expect(result.getByRole("list", { name: "Appended events" }).getByRole("listitem")).toHaveCount(2);
  });

  for (const evidence of ["disagreeing", "missing"] as const) {
    test(`a settled unsigned case with ${evidence} evidence does not infer reader agreement`, async ({ page }) => {
      settledEvidence = evidence;
      await page.goto(settledDesk);
      const settled = page.getByRole("region", { name: "This case is not waiting for a signature" });
      await expect(settled).toContainText("No review item was raised for this case. Open the case page to see the recorded evidence and outcomes.");
      await expect(settled).not.toContainText("two readers agreed");
      await expect(settled).not.toContainText("no person was needed");
    });
  }

  test("a 201 for an inadequate report does not claim rebooking while case data is unchanged", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Not adequate" }).click();
    await r.getByRole("button", { name: "Sign: report not adequate" }).click();

    const result = r.getByRole("status");
    await expect(result).toContainText("Demo answers appended");
    await expect(result).toContainText("No repeat booking is confirmed.");
    await expect(result).not.toContainText("goes back to Today");
    await expect(result).not.toContainText("to book a repeat colonoscopy");
    await expect(r.locator('li[data-kind="current"]').last()).not.toContainText("Her coordinator");
  });

  test("a 201 for ask-the-unit records identity without claiming a notification", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Ask the unit" }).click();
    await r.getByRole("button", { name: "Sign and ask the unit to complete the report" }).click();

    const result = r.getByRole("status");
    await expect(result).toContainText("Demo identity answer appended");
    await expect(result).toContainText("No unit notification was sent.");
    await expect(result).not.toContainText("Asked the unit");
    for (const row of disputedRows(page)) await expect(row).not.toContainText("asked the unit");
    await expect(signedNodes(page)).toHaveCount(1);
    await expect(r.locator('li[data-kind="current"]').last()).not.toContainText("to complete the colonoscopy report");
  });

  test("a 201 for not-the-same does not claim link undo while case data is unchanged", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    await r.getByRole("radio", { name: "Not the same patient" }).click();
    await r.getByRole("button", { name: "Sign: not the same patient" }).click();

    const result = r.getByRole("status");
    await expect(result).toContainText("Demo identity answer appended");
    await expect(result).toContainText("No link or unit change is confirmed.");
    await expect(result).not.toContainText("Link rejected");
    await expect(r).not.toContainText("Link undone");
    await expect(r).not.toContainText("report returned unmatched");
    await expect(signedNodes(page)).toHaveCount(1);
  });

  test("a 403 for the clinician role is partial: one entry saved, no closure, the retry sends only what was refused", async ({ page }) => {
    refuse = "clinician";
    await page.goto(desk);
    const r = rail(page);
    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("button", { name: "Close the case" }).click();

    await expect(r.getByRole("alert")).toHaveText(
      "The identity answer was saved. Not saved: the clinician role is not allowed to sign the report answer.",
    );
    await expect(r.getByRole("status")).toHaveCount(0);
    await expect(r).not.toContainText("Closed: both answers recorded");
    await expect(signedNodes(page)).toHaveCount(1);
    await expect(page.getByRole("list", { name: "Review history" }).getByRole("listitem")).toHaveCount(1);
    expect(posted.map((x) => x.role)).toEqual(["reviewer", "clinician"]);
    await page.screenshot({ path: "test-results/screens/review-live-403.png", fullPage: true });
    await noSeriousAxe(page);

    refuse = undefined;
    await r.getByRole("button", { name: "Close the case" }).click();
    await expect(r.getByRole("status")).toContainText("Demo answers appended; case open");
    expect(posted.map((x) => x.body.item)).toEqual([LINK_ITEM, EVIDENCE_ITEM, EVIDENCE_ITEM]);
    await expect(page.getByRole("list", { name: "Review history" }).getByRole("listitem")).toHaveCount(2);
  });

  test("a 403 for the reviewer role saves nothing and never reads as closed", async ({ page }) => {
    refuse = "reviewer";
    await page.goto(desk);
    const r = rail(page);
    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("button", { name: "Close the case" }).click();

    await expect(r.getByRole("alert")).toHaveText("Not saved: the reviewer role is not allowed to sign the identity answer.");
    await expect(r.getByRole("status")).toHaveCount(0);
    await expect(signedNodes(page)).toHaveCount(0);
    expect(posted.map((x) => x.role)).toEqual(["reviewer"]);
    // The failed save leaves focus on the button, not <body>.
    await expect(r.getByRole("button", { name: "Close the case" })).toBeFocused();
  });

  test("keyboard only: Close the case moves focus to the confirmation and Tab reaches the next link", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    await r.getByRole("radio", { name: "Same patient", exact: true }).focus();
    await page.keyboard.press("Space");
    await page.keyboard.press("Tab");
    await page.keyboard.press("Space");
    await page.keyboard.press("Tab");
    await expect(r.getByRole("button", { name: "Close the case" })).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(r.getByRole("status")).toContainText("Demo answers appended; case open");
    await expect(r.getByRole("heading", { name: /Demo answers appended; case open/ })).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(r.getByRole("link", { name: "Back to Review" })).toBeFocused();
  });
});
