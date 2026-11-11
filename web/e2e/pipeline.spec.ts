import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import replay from "../data/replay.json";
import evidenceAudit from "../data/api/case_evidence_closure_audit.json";

// The tamper demo rewrites the readers' step of the closed fixture; its hash is the one the server sealed (first 12 hex digits).
const READERS_HASH = evidenceAudit.chain.find((e) => e.type === "evidence")!.hash.slice(0, 12);

const LINE = "Replaying the recorded showcase run · synthetic patients · names and faces are generated · real report texts";
const SHOTS = "test-results/screens/pipeline";

// A missing name or case must fail within seconds, not after the 2-minute test budget: actions (click, fill) get the same 20s as expect.
test.describe.configure({ timeout: 60_000 });
test.use({ actionTimeout: 20_000, navigationTimeout: 30_000 });

const asNumber = (s: string) => Number(s.replace(/[^\d]/g, ""));

/** Call with the replay paused: axe needs real timers. */
async function expectNoSeriousAxe(page: Page) {
  await page.clock.resume();
  // Endless status pulses never finish; wait only for one-shot fades and slides.
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running" || a.effect?.getTiming().iterations === Infinity));
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

async function totals(page: Page) {
  const read = async (id: string) => Number(await page.getByTestId(id).getAttribute("data-value"));
  return { decisions: await read("total-decisions"), chains: await read("total-chains") };
}

/** Timers fire only on clock.runFor, so a slow machine cannot drift the replay between assertions. */
async function freezeClock(page: Page) {
  await page.clock.install({ time: new Date("2026-10-31T09:00:00Z") });
  await page.clock.pauseAt(new Date("2026-10-31T09:00:01Z"));
}

async function position(page: Page) {
  return asNumber((await page.getByTestId("replay-position").innerText()).split(" of ")[0]);
}

test("pipeline: pause, step, play and tamper, totals within the run", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline");
  await expect(page.locator("h1")).toHaveText("Pipeline");
  await expect(page.getByText(LINE)).toBeVisible();
  await expect(page.locator("[data-honesty]").first()).toBeVisible();
  await expect(page.locator("[data-stage]")).toHaveCount(8);

  await page.clock.runFor(500);
  await expect(page.locator('[data-stage="received"] [data-state="unknown"]').first()).toBeAttached();
  await expect(page.locator('[data-stage="closed"] [data-state="known"]').first()).toBeAttached();
  await page.screenshot({ path: `${SHOTS}-rest.png`, fullPage: true });
  await page.getByRole("button", { name: "Pause" }).click();
  await expect(page.getByTestId("replay-position")).toContainText("paused");

  const before = await position(page);
  await page.clock.runFor(3000);
  expect(await position(page), "paused: no ticks").toBe(before);

  const flow = page.getByRole("list", { name: /^Case flow/ });
  await expect(flow.getByTestId("chip-name").first()).toBeVisible();
  await expect(flow).not.toContainText(/Case \d/);
  await expect(page.getByRole("list", { name: "Decisions, newest first" })).not.toContainText(/Case \d/);

  const arriving = page.locator('[data-stage="received"] [data-case]').first();
  const id = (await arriving.getAttribute("data-case"))!;
  const person = replay.cases.find((c) => c.id === id)!.person;
  const chip = page.locator(`[data-case="${id}"]`);
  await expect(chip.getByTestId("chip-name"), "unmatched: the report's own spelling").toHaveText(person.reportName);
  await expect(chip.locator("[data-state]")).toHaveAttribute("data-state", "unknown");
  for (let i = 0; i < 12 && (await chip.locator("[data-state]").getAttribute("data-state")) === "unknown"; i++) {
    await page.getByRole("button", { name: "Step", exact: true }).click();
  }
  await expect(chip.locator("[data-state]"), "matched: the \"?\" resolves").not.toHaveAttribute("data-state", "unknown");
  await expect(chip.getByTestId("chip-name")).toHaveText(person.name);
  const resolved = await position(page);

  for (let i = 0; i < 12; i++) await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect.poll(() => position(page)).toBe(resolved + 12);
  const stepped = await totals(page);
  expect(stepped.decisions).toBeLessThanOrEqual(replay.totals.outputs);
  expect(stepped.chains).toBeLessThanOrEqual(replay.totals.chainsVerified);

  await page.getByRole("button", { name: "Play" }).click();
  await page.clock.runFor(2500);
  await expect.poll(() => position(page)).toBe(resolved + 14);

  // The server switch-off is designed, not built: no control, a visible label, and the recorded run's figures.
  await expect(page.getByRole("button", { name: /switch .*server/i })).toHaveCount(0);
  await expect(page.getByTestId("server-result")).toContainText("live failover is not built");
  await expect(page.getByTestId("server-result")).toContainText("0 of");
  await expect(page.locator('[data-honesty="designed"]')).toHaveText("Designed, not built");
  const moving = await position(page);
  await page.clock.runFor(2000);
  await expect.poll(() => position(page)).toBe(moving + 2);

  await page.getByRole("button", { name: "Tamper with a step" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Changed record detected at step 2" })).toBeVisible();
  await expect(page.locator('[aria-label="Sealed chain"] [data-broken]')).toHaveCount(2);
  await expect(page.getByText(new RegExp(`no longer matches ${READERS_HASH}`)).first()).toBeVisible();

  await page.getByRole("button", { name: "Pause" }).click();
  await page.clock.runFor(1000);
  await page.screenshot({ path: `${SHOTS}-mid-run.png`, fullPage: true });
  await page.getByRole("button", { name: "Play" }).click();

  for (let i = 0; i < 5; i++) {
    const t = await totals(page);
    expect(t.decisions).toBeLessThanOrEqual(replay.totals.outputs);
    expect(t.chains).toBeLessThanOrEqual(replay.totals.chainsVerified);
    await page.clock.runFor(3000);
  }

  await page.getByRole("button", { name: "Undo the change" }).click();
  await expect(page.getByText("Records intact")).toBeVisible();
  await page.getByRole("button", { name: "Pause" }).click();
  await page.clock.runFor(1000);
  await expectNoSeriousAxe(page);
});

test("matching: the score lands against the 0.99 line, runner-ups labelled illustrative", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline/matching");
  await expect(page.locator("h1")).toHaveText("Identity matching");
  await expect(page.getByText(LINE)).toBeVisible();
  await page.clock.runFor(800);
  await page.getByRole("button", { name: "Pause" }).click();
  await expect(page.getByTestId("match-p")).toHaveText("0.989");
  await expect(page.getByText("auto-close line 0.99")).toBeVisible();
  await expect(page.getByText(/illustrative/i).first()).toBeVisible();
  await page.screenshot({ path: `${SHOTS}-matching-rest.png`, fullPage: true });

  await page.getByRole("button", { name: /^Aisha/ }).click();
  await expect(page.getByTestId("match-p")).toHaveText("—");
  const reportFace = page.getByRole("img", { name: /Not yet confirmed as the referral's patient|Same patient as the referral/ });
  await expect(reportFace).toHaveAttribute("data-state", "unknown");
  for (let i = 0; i < 9; i++) await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.getByTestId("match-p")).toHaveText("1.000");
  await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Matched on the Emirates ID" })).toBeVisible();
  await expect(reportFace, "held for a second identifier: the face stays \"?\"").toHaveAttribute("data-state", "unknown");

  await page.getByRole("button", { name: "How it decides" }).click();
  await expect(page.getByText(/R@10 0\.932/)).toBeVisible();
  await page.getByRole("button", { name: "Play" }).click();
  await page.clock.runFor(6 * 900);
  await page.screenshot({ path: `${SHOTS}-matching-mid-run.png`, fullPage: true });
  await page.getByRole("button", { name: "Pause" }).click();
  await page.clock.runFor(1000);
  await expectNoSeriousAxe(page);
});

test("reading: real report, recorded Gemma answer and latency, played 5x faster", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline/reading");
  await expect(page.locator("h1")).toHaveText("Report reading");
  await expect(page.getByText(/played 5× faster/).first()).toBeVisible();
  await page.clock.runFor(800);
  await page.getByRole("button", { name: "Pause" }).click();
  await expect(page.getByTestId("ai-timer")).toHaveText("7.5 s recorded");
  await expect(page.getByTestId("gate-pill")).toHaveText("Passed");
  await expect(page.getByTestId("report-text")).toContainText("which was identified by the presence of the appendiceal orifice and ileocecal valve");
  await expect(page.getByTestId("report-text")).toContainText("bowel preparation was good.");
  await expect(page.getByText("Fatima Al Zaabi's report SKM-dr-191")).toBeVisible();
  await page.screenshot({ path: `${SHOTS}-reading-rest.png`, fullPage: true });

  await page.getByRole("button", { name: "Report u38" }).click();
  await expect(page.getByTestId("gate-pill")).toHaveText("Waiting");
  for (let i = 0; i < 3; i++) await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.getByTestId("ai-timer")).toHaveText("reading · 9.0 s");
  await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.getByTestId("ai-timer")).toHaveText("10.1 s recorded");
  await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.locator('[data-field="caecum"]')).toContainText("disagree");
  await expect(page.locator('[data-field="tissue"]')).toContainText("not compared");
  await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.getByTestId("gate-pill")).toHaveText("Two reviewers");
  await page.screenshot({ path: `${SHOTS}-reading-u38.png`, fullPage: true });

  await page.getByRole("button", { name: "Report u6" }).click();
  await page.getByRole("button", { name: "Play" }).click();
  await page.clock.runFor(3 * 900);
  await expect(page.getByTestId("ai-timer")).toHaveText("reading · 9.0 s");
  await page.screenshot({ path: `${SHOTS}-reading-mid-run.png`, fullPage: true });
  await page.getByRole("button", { name: "Pause" }).click();
  await page.clock.runFor(1000);
  await expectNoSeriousAxe(page);
});

test("pipeline: a chip opens that person's profile, pauses the replay, and Escape resumes it", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline");
  await page.clock.runFor(500);

  const chip = page.locator('[data-stage="closed"] a[data-case]').first();
  const id = (await chip.getAttribute("data-case"))!;
  const person = replay.cases.find((c) => c.id === id)!.person;
  await chip.focus();
  await expect(chip).toBeFocused();
  await chip.click();

  const sheet = page.getByRole("dialog");
  await expect(sheet.getByRole("heading", { name: person.name })).toBeVisible();
  await expect(sheet.getByTestId("report-name")).toHaveText(person.reportName);
  await expect(sheet.locator('[data-honesty="replay"]')).toHaveText(/Replay of a recorded run · (generated|demo cast) name/);
  await expect(sheet.getByRole("list", { name: "Steps in this run" }).getByRole("listitem").first()).toContainText("Received");
  await expect(page).toHaveURL((url) => url.searchParams.get("person") === id);

  const held = await position(page);
  await page.clock.runFor(3000);
  expect(await position(page), "an open profile holds the replay").toBe(held);
  await page.screenshot({ path: `${SHOTS}-profile.png` });
  await page.clock.resume();
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running" || a.effect?.getTiming().iterations === Infinity));
  const axe = await new AxeBuilder({ page }).include('[role="dialog"]').analyze();
  expect(axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical").map((v) => v.id)).toEqual([]);
  expect(await position(page), "still held on real timers").toBe(held);

  await page.keyboard.press("Escape");
  await expect(sheet).toHaveCount(0);
  await expect(page).toHaveURL(/\/app\/pipeline$/);
  await expect.poll(() => position(page), "closing resumes a playing replay").toBeGreaterThan(held);

  await page.getByRole("button", { name: "Pause" }).click();
  const paused = await position(page);
  await page.locator("a[data-case]").first().click();
  await expect(sheet).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(sheet).toHaveCount(0);
  await page.waitForTimeout(2500);
  expect(await position(page), "closing leaves a paused replay paused").toBe(paused);
});

test("pipeline: Fatima's profile, opened from its link, leads to her patient record", async ({ page }) => {
  const fatima = replay.cases.find((c) => c.person.name === "Fatima Al Zaabi")!;
  await page.goto(`/app/pipeline?person=${fatima.id}`);
  const sheet = page.getByRole("dialog");
  await expect(sheet.getByRole("heading", { name: "Fatima Al Zaabi" })).toBeVisible();
  await expect(sheet.locator('[data-honesty="replay"]')).toHaveText("Replay of a recorded run · demo cast name");
  await sheet.getByRole("link", { name: "Open full record" }).click();
  await expect(page).toHaveURL(/\/app\/patients\/ahs-mushrif%2FServiceRequest%2FAHS-sr-1001$/);
  await expect(page.locator("h1")).toContainText("Fatima");
});

test.describe("on a 390px touch screen", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

  test("pipeline: the profile's close control is a 44px target that overlaps nothing and works by keyboard and tap", async ({ page }) => {
    const fatima = replay.cases.find((c) => c.person.name === "Fatima Al Zaabi")!;
    await page.goto(`/app/pipeline?person=${fatima.id}`);
    const sheet = page.getByRole("dialog");
    const close = sheet.getByRole("button", { name: "Close" });
    await expect(sheet.getByRole("heading", { name: "Fatima Al Zaabi" })).toBeVisible();
    await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running" || a.effect?.getTiming().iterations === Infinity));

    const box = (await close.boundingBox())!;
    expect(box.width).toBeGreaterThanOrEqual(44);
    expect(box.height).toBeGreaterThanOrEqual(44);
    const hits = await close.evaluate((mine) => {
      const a = mine.getBoundingClientRect();
      return [...mine.closest('[role="dialog"]')!.querySelectorAll('a, button, input, [tabindex]:not([tabindex="-1"])')]
        .filter((el) => el !== mine)
        .map((el) => ({ el, r: el.getBoundingClientRect() }))
        .filter(({ r }) => r.width > 0 && r.height > 0 && r.left < a.right && a.left < r.right && r.top < a.bottom && a.top < r.bottom)
        .map(({ el }) => el.outerHTML.slice(0, 80));
    });
    expect(hits).toEqual([]);
    // The 16px icon stays where it was, 16px in from the sheet's top-right corner.
    const sheetBox = (await sheet.boundingBox())!;
    const icon = (await close.locator("svg").boundingBox())!;
    expect(Math.round(sheetBox.x + sheetBox.width - (icon.x + icon.width))).toBe(16);
    expect(Math.round(icon.y - sheetBox.y)).toBe(16);
    expect(Math.round(icon.width)).toBe(16);

    await close.focus();
    await expect(close).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(sheet).toHaveCount(0);

    await page.goto(`/app/pipeline?person=${fatima.id}`);
    await expect(sheet.getByRole("heading", { name: "Fatima Al Zaabi" })).toBeVisible();
    await close.tap();
    await expect(sheet).toHaveCount(0);
  });
});

test("pipeline: a generated person's profile says there is no record", async ({ page }) => {
  const generated = replay.cases.find((c) => c.person.source === "generated")!;
  await page.goto(`/app/pipeline?person=${generated.id}`);
  const sheet = page.getByRole("dialog");
  await expect(sheet.locator('[data-honesty="replay"]')).toHaveText("Replay of a recorded run · generated name");
  await expect(sheet.getByRole("link", { name: "Open full record" })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(page).toHaveURL(/\/app\/pipeline$/);
});

test("matching and reading: the demo patient opens her record", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline/matching");
  await page.clock.runFor(800);
  await page.getByRole("button", { name: "Pause" }).click();
  await page.getByRole("link", { name: /The referral/ }).click();
  await expect(page).toHaveURL(/\/app\/patients\/ahs-mushrif%2FServiceRequest%2FAHS-sr-1001$/);

  await page.goto("/app/pipeline/reading");
  await page.clock.runFor(800);
  await page.getByRole("link", { name: "Fatima Al Zaabi's report SKM-dr-191" }).click();
  await expect(page).toHaveURL(/\/app\/patients\/ahs-mushrif%2FServiceRequest%2FAHS-sr-1001$/);
});

const RAW = /closure_decision|needs_human|not_closed|link_decision|review_hold|evidence_closure|false_closure_trap|model_version|path_p_cal/;
const PIPELINE_ROUTES = ["/app/pipeline", "/app/pipeline/matching", "/app/pipeline/reading"] as const;

test("raw backend ids are hidden by default and revealed by a Technical detail disclosure", async ({ page }) => {
  for (const route of PIPELINE_ROUTES) {
    await freezeClock(page);
    await page.goto(route);
    await page.clock.runFor(800);
    await page.getByRole("button", { name: "Pause" }).click();
    const main = page.locator("main");
    expect(await main.innerText(), `${route}: no raw id in the visible text`).not.toMatch(RAW);
    const details = page.getByTestId("technical-detail");
    expect(await details.count(), `${route}: at least one disclosure`).toBeGreaterThan(0);
    for (const d of await details.all()) await d.locator("summary").click();
    expect(await main.innerText(), `${route}: the disclosure reveals the system's labels`).toMatch(/closure_decision|needs_human|link_decision|model_version|path_p_cal|caecum_reached/);
  }
});

test("pipeline: the flow's plain sublabels replace the system names", async ({ page }) => {
  await page.goto("/app/pipeline");
  await expect(page.locator('[data-stage="closed"]')).toContainText("Closed with evidence");
  await expect(page.locator('[data-stage="review"]')).toContainText("Waiting for two reviewers");
  await expect(page.locator('[data-stage="open"]')).toContainText("Kept open");
  await expect(page.locator('[data-stage="closed"] summary, [data-stage="closed"] code')).toHaveCount(0);
});

test("every control is a replay control, a real toggle or labelled; none is an unlabelled simulation", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline");
  const names = (await page.locator("main button:visible").evaluateAll((els) => els.map((e) => (e.textContent ?? "").trim()))).filter(Boolean);
  expect(new Set(names.filter((n) => !/^[A-Z]/.test(n) || /^(Play|Pause|Step|Tamper with a step|Undo the change)$/.test(n)))).toEqual(
    new Set(["Pause", "Step", "Tamper with a step"]),
  );
  await expect(page.getByRole("group", { name: "Replay controls" })).toBeVisible();
  await expect(page.locator('main [data-honesty="simulated"]')).toHaveText("Simulated edit");
});

for (const route of PIPELINE_ROUTES) {
  test(`${route}: replay controls are at least 44px and show a visible focus ring`, async ({ page }) => {
    await page.goto(route);
    const controls = page.getByRole("group", { name: "Replay controls" }).getByRole("button");
    for (const b of await controls.all()) {
      const box = (await b.boundingBox())!;
      expect(box.height, await b.innerText()).toBeGreaterThanOrEqual(44);
      expect(box.width).toBeGreaterThanOrEqual(44);
    }
    await page.getByRole("button", { name: /^(Play|Pause)$/ }).focus();
    await page.keyboard.press("Shift+Tab");
    await page.keyboard.press("Tab");
    const ring = await page.evaluate(() => {
      const cs = getComputedStyle(document.activeElement!);
      return { style: cs.outlineStyle, width: cs.outlineWidth };
    });
    expect(ring.style).not.toBe("none");
    expect(parseFloat(ring.width)).toBeGreaterThanOrEqual(2);
  });

  test(`${route}: with reduced motion nothing autoplays`, async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await freezeClock(page);
    await page.goto(route);
    await expect(page.getByTestId("reduced-motion-note")).toBeVisible();
    await expect(page.getByTestId("replay-position")).toContainText("paused");
    const before = await page.getByTestId("replay-position").innerText();
    await page.clock.runFor(5000);
    expect(await page.getByTestId("replay-position").innerText(), "no ticks without a press").toBe(before);
    await page.getByRole("button", { name: "Step", exact: true }).click();
    expect(await page.getByTestId("replay-position").innerText(), "Step still moves it").not.toBe(before);
    // Nothing animates on its own: no running looping animation anywhere on the page.
    await page.clock.resume();
    const looping = await page.evaluate(() => document.getAnimations().filter((a) => a.playState === "running" && a.effect?.getTiming().iterations === Infinity).length);
    expect(looping).toBe(0);
  });

  test(`${route}: fits a 390px phone and has no serious axe findings`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await freezeClock(page);
    await page.goto(route);
    await page.clock.runFor(800);
    await page.getByRole("button", { name: "Pause" }).click();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await page.screenshot({ path: `${SHOTS}-${route.split("/").at(-1)}-390.png`, fullPage: true });
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.screenshot({ path: `${SHOTS}-${route.split("/").at(-1)}-1440.png`, fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await expectNoSeriousAxe(page);
  });
}

test("pipeline: Step announces the new step and decision once per press; autoplay stays silent (#16)", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline");
  const announcer = page.getByTestId("step-announcer");
  await expect(announcer).toHaveAttribute("role", "status");
  await expect(announcer).toHaveAttribute("aria-live", "polite");
  await page.clock.runFor(4000);
  await expect(announcer, "autoplay does not announce").toHaveText("");

  await page.getByRole("button", { name: "Pause" }).click();
  await expect(announcer, "Pause does not announce").toHaveText("");
  const before = await position(page);
  const step = page.getByRole("button", { name: "Step", exact: true });
  await step.click();
  const after = await position(page);
  expect(after).toBe(before + 1);
  await expect(announcer).toContainText(`Step ${after} of`);
  await expect(announcer).toContainText("Newest decision:");
  const newest = await page.getByRole("list", { name: "Decisions, newest first" }).locator("li").first().locator("div.truncate").first().innerText();
  await expect(announcer).toContainText(newest);
  await expect(announcer.locator("span"), "one message, not the feed").toHaveCount(1);
  const first = await announcer.innerText();
  await expect(step, "focus stays on Step").toBeFocused();

  await step.click();
  await expect(announcer).not.toHaveText(first);
  await expect(announcer).toContainText(`Step ${after + 1} of`);

  await page.getByRole("button", { name: "Play" }).click();
  const spoken = await announcer.innerText();
  await page.clock.runFor(3000);
  await expect(announcer, "playing again does not announce").toHaveText(spoken);
});

test("pipeline: backend health sits apart from the replay and says it is a snapshot (#11)", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline");
  const panel = page.getByTestId("health-panel");
  await expect(panel.getByRole("heading", { name: "Backend health" })).toBeVisible();
  await expect(panel.getByTestId("health-status")).toHaveText("ok");
  await expect(panel.getByTestId("health-cases")).toHaveText("5,280");
  await expect(panel.getByTestId("dep-materialised_run")).toHaveText("loaded");
  await expect(panel.getByTestId("dep-nats")).toHaveText("not_required_for_read_api");
  await expect(panel.getByTestId("dep-reader")).toHaveText("cached");
  await expect(panel.getByTestId("dep-ingest_adapter")).toHaveText("fhir");
  await expect(panel.getByTestId("health-source")).toContainText("published snapshot taken 6 Nov 2026, 13:56 UTC");
  await expect(panel.getByTestId("health-source")).toContainText("not a live check");
  await expect(panel).toContainText("not required to read a saved run");
  await expect(panel.getByTestId("health-unavailable")).toHaveCount(0);
  await expect(page.locator("main")).not.toContainText(/servers? on/i);
  await expect(page.getByTestId("server-result")).toContainText("not the state of any server now");
  await expect(page.locator('[data-honesty="replay"]').first()).toBeVisible();
});

test("pipeline: health panel fits 390px and keeps 44px targets", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await freezeClock(page);
  await page.goto("/app/pipeline");
  await page.getByRole("button", { name: "Pause" }).click();
  const panel = page.getByTestId("health-panel");
  await panel.scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  for (const el of await panel.locator("summary").all()) expect((await el.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  for (const name of ["Play", "Step"]) {
    const b = page.getByRole("button", { name, exact: true });
    expect((await b.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  await page.screenshot({ path: "test-results/screens/pipe2-pipeline-390.png", fullPage: true });
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.screenshot({ path: "test-results/screens/pipe2-pipeline-1440.png", fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await expectNoSeriousAxe(page);
});
