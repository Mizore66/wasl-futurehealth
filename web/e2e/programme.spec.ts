import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";
import { axe, unsourced } from "./numbers";

const stallCard = (page: Page) => page.locator('section[aria-labelledby="stall-h"]');
const kpiCard = (page: Page) => page.locator('section[aria-labelledby="kpi-h"]');
const progCard = (page: Page) => page.locator('section[aria-labelledby="prog-h"]');
const capCard = (page: Page) => page.locator('section[aria-labelledby="cap-h"]');
const SOURCE_CARDS = ["kpi-h", "prog-h", "stall-h", "cap-h"] as const;

test.describe("Programme", () => {
  test("leads with reported vs measured, bars against the target and the truth line", async ({ page }) => {
    await page.goto("/app");
    await expect(page.locator("h1")).toHaveText("Programme");
    await expect(page.locator("[data-honesty]:visible").first()).toBeVisible();
    await expect(page.getByTestId("kpi-eid")).toContainText("11.3%");
    await expect(page.getByTestId("kpi-wasl")).toContainText("30.7%");
    await expect(page.getByRole("group", { name: "On a 0 to 100% scale: reported 11.3%, measured by Wasl 30.7%, true 31.9%, target 90%." })).toBeVisible();
    await expect(page.getByTestId("kpi-truth")).toHaveText("Within 1.2 points of the true rate (31.9%) in testing");
    await expect(page.getByText("1 in 6 on your chase list is already done.")).toBeVisible();
    await page.screenshot({ path: "test-results/screens/programme.png", fullPage: true });
    await axe(page);
  });

  test("every Source disclosure opens and closes", async ({ page }) => {
    await page.goto("/app");
    for (const [card, note] of [
      ["kpi-h", "The target is the DoH 31-day standard."],
      ["prog-h", "Still open is the programme"],
      ["stall-h", "Simulated run of 2,100 cases, colorectal and cervical together (run v7strict-cached-fhir-lung-dr-breast-ldct-ifhas-drnext-lungsurv-xreflink)"],
      ["cap-h", "The optimiser is greedy"],
    ] as const) {
      const section = page.locator(`section[aria-labelledby="${card}"]`);
      const button = section.getByRole("button", { name: "Source" });
      await expect(section.getByText(note)).toBeHidden();
      await button.click();
      await expect(section.getByRole("button", { name: "Hide source" })).toHaveAttribute("aria-expanded", "true");
      await expect(section.getByText(note)).toBeVisible();
      if (card === "stall-h") await page.screenshot({ path: "test-results/screens/programme-source.png", fullPage: true });
      await section.getByRole("button", { name: "Hide source" }).click();
      await expect(section.getByText(note)).toBeHidden();
    }
  });

  test("each Source note keeps aria-controls resolvable and is out of reach while closed", async ({ page }) => {
    await page.goto("/app");
    for (const card of SOURCE_CARDS) {
      const section = page.locator(`section[aria-labelledby="${card}"]`);
      const button = section.getByRole("button", { name: "Source" });
      const note = page.locator(`[id="${await button.getAttribute("aria-controls")}"]`);
      await expect(note).toHaveCount(1);
      await expect(note).toHaveAttribute("inert", "");
      await expect(note).toBeHidden();
      await button.click();
      await expect(note).not.toHaveAttribute("inert");
      await expect(note).toBeVisible();
      await section.getByRole("button", { name: "Hide source" }).click();
      // Inert from the first frame of closing, so focus and screen readers never land in a collapsing note.
      await expect(note).toHaveAttribute("inert", "");
      await expect(note).toBeHidden();
      await expect(section.getByRole("button", { name: "Source" })).toBeFocused();
    }
  });

  /** React has attached its handlers to the Source buttons, so a scripted click is handled. */
  async function hydrated(page: Page) {
    await page.waitForFunction(() =>
      [...document.querySelectorAll("main button[aria-controls]")].every((b) => Object.keys(b).some((k) => k.startsWith("__reactProps"))),
    );
  }

  /** Clicks a card's Source button and records the card's height on every frame for 400ms. */
  async function heightsWhileOpening(page: Page, card: string): Promise<number[]> {
    return page.evaluate(
      (card) =>
        new Promise<number[]>((done) => {
          const section = document.querySelector<HTMLElement>(`section[aria-labelledby="${card}"]`)!;
          const heights = [section.getBoundingClientRect().height];
          const start = performance.now();
          section.querySelector<HTMLButtonElement>("button[aria-controls]")!.click();
          const tick = () => {
            heights.push(section.getBoundingClientRect().height);
            if (performance.now() - start < 400) requestAnimationFrame(tick);
            else done(heights);
          };
          requestAnimationFrame(tick);
        }),
      card,
    );
  }

  test("Source notes open over several frames, not in one step", async ({ page }) => {
    await page.goto("/app");
    await hydrated(page);
    for (const card of SOURCE_CARDS) {
      const heights = await heightsWhileOpening(page, card);
      const growth = heights.at(-1)! - heights[0];
      const steps = heights.slice(1).map((h, i) => h - heights[i]);
      const between = heights.filter((h) => h > heights[0] + 1 && h < heights.at(-1)! - 1);
      expect(growth, `${card} grows`).toBeGreaterThan(40);
      expect(between.length, `${card} passes through in-between heights`).toBeGreaterThanOrEqual(2);
      expect(Math.max(...steps), `${card} largest single-frame step`).toBeLessThan(growth * 0.7);
    }
  });

  test("with reduced motion the Source note opens at once", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/app");
    await hydrated(page);
    for (const card of SOURCE_CARDS) {
      const heights = await heightsWhileOpening(page, card);
      const growth = heights.at(-1)! - heights[0];
      expect(growth, `${card} grows`).toBeGreaterThan(40);
      // One step, from closed to open, within the frame or two React and Motion take to commit.
      expect(heights.filter((h) => h !== heights[0] && h !== heights.at(-1)), `${card} has no in-between heights`).toEqual([]);
      expect(heights.indexOf(heights.at(-1)!), `${card} frames until full height`).toBeLessThanOrEqual(2);
    }
  });

  test("the top three stalls each link to the filtered patient list", async ({ page }) => {
    await page.goto("/app");
    const rows = stallCard(page).locator("ol a");
    await expect(rows).toHaveCount(3);
    const expected = [
      ["No acknowledgement", "370", "no_acknowledgement"],
      ["Not booked", "241", "not_booked"],
      ["Never notified", "196", "never_notified"],
    ];
    for (const [i, [label, count, dropoff]] of expected.entries()) {
      await expect(rows.nth(i)).toContainText(label);
      await expect(rows.nth(i)).toContainText(count);
      await expect(rows.nth(i)).toHaveAttribute("href", `/app/patients?dropoff=${dropoff}`);
    }
    await rows.first().click();
    await expect(page).toHaveURL(/\/app\/patients\?dropoff=no_acknowledgement$/);
  });

  test("Open today's list and See proof go where the board says", async ({ page }) => {
    await page.goto("/app");
    await expect(page.getByLabel("Safety")).toContainText("0 false closures in 3,035 tested.");
    await page.getByRole("link", { name: "See proof" }).click();
    await expect(page).toHaveURL(/\/app\/results$/);
    await page.goto("/app");
    await page.getByRole("link", { name: "Open today's list" }).click();
    await expect(page).toHaveURL(/\/app\/today$/);
  });

  test("every number on the screen is in results.json or the dashboard", async ({ page }) => {
    await page.goto("/app");
    expect(await unsourced(page)).toEqual([]);
  });
});

test.describe("Programme on a 390px touch screen", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

  test("Source buttons have 44px hit areas that overlap no other control, and nothing scrolls sideways", async ({ page }) => {
    await page.goto("/app");
    for (const card of SOURCE_CARDS) {
      const section = page.locator(`section[aria-labelledby="${card}"]`);
      for (const name of ["Source", "Hide source"]) {
        const button = section.getByRole("button", { name });
        await expect(button).toBeVisible();
        const box = (await button.boundingBox())!;
        expect(box.width, `${card} ${name} width`).toBeGreaterThanOrEqual(44);
        expect(box.height, `${card} ${name} height`).toBeGreaterThanOrEqual(44);
        const hits = await button.evaluate((mine) => {
          const a = mine.getBoundingClientRect();
          return [...document.querySelectorAll('a, button, input, select, [tabindex]:not([tabindex="-1"])')]
            .filter((el) => el !== mine)
            .map((el) => ({ el, r: el.getBoundingClientRect() }))
            .filter(({ r }) => r.width > 0 && r.height > 0 && r.left < a.right && a.left < r.right && r.top < a.bottom && a.top < r.bottom)
            .map(({ el }) => el.outerHTML.slice(0, 80));
        });
        expect(hits, `${card} ${name} overlaps`).toEqual([]);
        await button.tap();
        await expect(section.getByRole("button", { name: name === "Source" ? "Hide source" : "Source" })).toBeVisible();
        if (name === "Source") {
          expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), "no sideways scroll with the note open").toBe(true);
          if (card === "kpi-h") await page.screenshot({ path: "test-results/screens/programme-source-390.png", fullPage: true });
        }
      }
    }
  });
});

test.describe("Programme charts", () => {
  test("the toggle switches the 31-day rates and one visible line says why they differ", async ({ page }) => {
    await page.goto("/app");
    const card = kpiCard(page);
    const demo = card.getByRole("button", { name: "This demo run" });
    const mean = card.getByRole("button", { name: "Five-seed held-out mean" });
    await expect(demo).toHaveAttribute("aria-pressed", "true");
    await expect(mean).toHaveAttribute("aria-pressed", "false");
    await expect(page.getByTestId("kpi-why")).toBeVisible();
    await expect(page.getByTestId("kpi-why")).toContainText("one run on training-time noise");
    await expect(page.getByTestId("kpi-why")).toContainText("mean averages 5 seeds on held-out noise");
    await page.screenshot({ path: "test-results/screens/programme-demo.png", fullPage: true });

    await mean.click();
    await expect(mean).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByTestId("kpi-eid")).toContainText("12.2%");
    await expect(page.getByTestId("kpi-wasl")).toContainText("29.8%");
    await expect(page.getByTestId("kpi-truth")).toContainText("Within 3.1 points of the true rate (32.9%)");
    await expect(page.getByTestId("kpi-why")).toBeVisible();
    await expect(page.getByRole("group", { name: /reported 12.2%, measured by Wasl 29.8%, true 32.9%, target 90%/ })).toBeVisible();
    await page.screenshot({ path: "test-results/screens/programme-mean.png", fullPage: true });
    expect(await unsourced(page)).toEqual([]);

    await demo.click();
    await expect(page.getByTestId("kpi-eid")).toContainText("11.3%");
  });

  test("keyboard focus on a bar reveals its tooltip, and Escape closes it", async ({ page }) => {
    await page.goto("/app");
    const bar = kpiCard(page).getByRole("button", { name: /Measured by Wasl/ }).first();
    await bar.focus();
    const tip = kpiCard(page).getByRole("tooltip").filter({ hasText: "Measured by Wasl: 30.7%" });
    await expect(tip).toBeVisible();
    await expect(tip).toContainText("408 of 1,331 eligible referrals");
    await page.keyboard.press("Escape");
    await expect(tip).toBeHidden();
    await bar.hover();
    await expect(tip).toBeVisible();
  });

  test("by programme: seven rows with human names, counts and a link to the filtered list", async ({ page }) => {
    await page.goto("/app");
    const rows = progCard(page).locator("ul a");
    await expect(rows).toHaveCount(7);
    const keys = ["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"];
    for (const [i, name] of ["Colorectal", "Cervical", "Lung nodule", "Diabetic retinopathy", "Breast", "Lung screening (LDCT)", "IFHAS cardiometabolic"].entries()) {
      await expect(rows.nth(i)).toContainText(name);
      await expect(rows.nth(i)).toHaveAttribute("href", `/app/patients?programme=${keys[i]}`);
    }
    await expect(rows.first()).toContainText("473 closed · 869 overdue · 158 still open");
    await rows.nth(1).focus();
    await expect(progCard(page).getByRole("tooltip").filter({ hasText: "205 closed, 267 overdue, 128 still open, of 600 cases" })).toBeVisible();
    await rows.first().click();
    await expect(page).toHaveURL(/\/app\/patients\?programme=crc$/);
  });

  test("Show all stalls opens every sourced stall by pathway, and closes again", async ({ page }) => {
    await page.goto("/app");
    const card = stallCard(page);
    const button = card.getByRole("button", { name: "Show all stalls" });
    await expect(button).toHaveAttribute("aria-expanded", "false");
    await button.click();
    const open = card.getByRole("button", { name: "Show only the top 3" });
    await expect(open).toHaveAttribute("aria-expanded", "true");
    for (const h of ["Colorectal and cervical", "Lung nodule", "Diabetic retinopathy", "Breast", "Lung cancer screening (LDCT)", "IFHAS cardiometabolic follow-up"]) await expect(card.getByRole("heading", { name: h })).toBeVisible();
    await expect(card.locator("ol a")).toHaveCount(20);
    const lung = card.getByRole("link", { name: /No follow-up CT booked/ });
    await expect(lung).toContainText("498");
    await expect(lung).toHaveAttribute("href", "/app/patients?dropoff=no_followup_ct");
    await expect(card.getByRole("link", { name: /Awaiting tissue sampling/ })).toHaveAttribute("href", "/app/patients?dropoff=awaiting_tissue");
    await expect(card.getByText("<5")).toHaveCount(0);
    await page.screenshot({ path: "test-results/screens/programme-stalls-open.png", fullPage: true });
    expect(await unsourced(page)).toEqual([]);
    await open.click();
    await expect(card.locator("ol a")).toHaveCount(3);
    await expect(card.getByRole("button", { name: "Show all stalls" })).toHaveAttribute("aria-expanded", "false");
  });

  test("capacity: simulated label, summary, regions, and the measured pooling result with its caveat", async ({ page }) => {
    await page.goto("/app");
    const card = capCard(page);
    await expect(card.locator('[data-honesty="simulated"]')).toHaveText("Simulated decision support");
    await expect(page.getByTestId("capacity-condition")).toContainText("Misallocated");
    await expect(card).toContainText("network pooling (cross-site booking) recovers them");
    const summary = page.getByTestId("capacity-summary");
    await expect(summary).toContainText("97.3%");
    await expect(summary).toContainText("17 days");
    await expect(summary).toContainText("85");
    await expect(summary).toContainText("102");
    await expect(card.getByText("Pooling only moves existing slots between regions. It cannot create slots, so it does not help when the whole network is short.")).toBeVisible();
    await expect(page.getByTestId("pooling-result")).toContainText("21.5% within 31 days without cross-site booking, 41.7% with network pooling");
    await expect(card.getByRole("figure")).toHaveCount(2);
    await page.screenshot({ path: "test-results/screens/programme-capacity.png", fullPage: true });
  });

  test("every chart is a figure with a caption and a Show as table toggle with the same numbers", async ({ page }) => {
    await page.goto("/app");
    const figures = page.getByRole("figure");
    await expect(figures).toHaveCount(5);
    for (let i = 0; i < 5; i++) await expect(figures.nth(i).locator("figcaption")).toBeVisible();

    const kpi = kpiCard(page).getByRole("button", { name: "Show as table" });
    await expect(kpi).toHaveAttribute("aria-pressed", "false");
    await kpi.click();
    await expect(kpi).toHaveAttribute("aria-pressed", "true");
    await expect(kpiCard(page).getByRole("table")).toContainText("30.7%");
    await expect(kpiCard(page).getByRole("row", { name: /True rate/ })).toContainText("424 of 1,331 eligible referrals");

    await progCard(page).getByRole("button", { name: "Show as table" }).click();
    const table = progCard(page).getByRole("table");
    await expect(table.getByRole("row", { name: /Lung nodule/ })).toContainText("905");
    await expect(table.getByRole("row", { name: /Lung nodule/ })).toContainText("441");
    await expect(table.getByRole("columnheader")).toHaveText(["Programme", "Cases", "Closed", "Overdue", "Still open"]);

    await stallCard(page).getByRole("button", { name: "Show as table" }).click();
    await expect(stallCard(page).getByRole("row", { name: /No follow-up CT booked/ })).toContainText("498");

    await capCard(page).getByRole("button", { name: "Show as table" }).first().click();
    await expect(capCard(page).getByRole("row", { name: /Al Dhafra/ })).toContainText("96");
    await capCard(page).getByRole("button", { name: "Show as table" }).nth(1).click();
    await expect(capCard(page).getByRole("row", { name: /Misallocated, network pooling.*1\.2/ })).toContainText("41.7%");
    await page.screenshot({ path: "test-results/screens/programme-tables.png", fullPage: true });
    expect(await unsourced(page)).toEqual([]);
    await axe(page);
  });

  test("every chart mark can be reached by keyboard", async ({ page }) => {
    await page.goto("/app");
    const marks = page.locator("main figure").getByRole("button").or(page.locator("main figure a"));
    expect(await marks.count()).toBeGreaterThan(15);
    for (const el of await marks.all()) await expect(el).not.toHaveAttribute("tabindex", "-1");
  });

  test("phone width: no horizontal overflow, targets at least 44px, no serious axe violations", async ({ browser }) => {
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true });
    const page = await context.newPage();
    await page.goto("/app");
    const overflow = () => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(await overflow()).toBeLessThanOrEqual(0);
    await page.screenshot({ path: "test-results/screens/programme-390.png", fullPage: true });
    await kpiCard(page).getByRole("button", { name: "Five-seed held-out mean" }).click();
    await page.screenshot({ path: "test-results/screens/programme-390-mean.png", fullPage: true });
    await stallCard(page).getByRole("button", { name: "Show all stalls" }).click();
    expect(await overflow()).toBeLessThanOrEqual(0);
    for (const b of await page.locator("main figure").getByRole("button").all()) {
      expect((await b.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(43.5);
    }
    for (const b of await kpiCard(page).getByRole("button", { name: /This demo run|Five-seed/ }).all()) {
      expect((await b.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(43.5);
    }
    for (const t of await page.getByRole("button", { name: "Show as table" }).all()) await t.click();
    expect(await overflow()).toBeLessThanOrEqual(0);
    await page.screenshot({ path: "test-results/screens/programme-390-tables.png", fullPage: true });
    await axe(page);
    await context.close();
  });
});

/* Issue #45: provider links into filtered Patients lists, and the queue-lever counts. */
const providerBlock = (page: Page) => page.getByTestId("provider-links");
const SHOTS = "/tmp/claude-0/-home-user-wasl-futurehealth/309dfc1c-29a9-5a64-83c9-6308e8ff1d9e/scratchpad/review-64";

type SampleCase = { id: string; provider: string; state: string; dropoff: string };

/** The cases fixtures mode serves to Patients: every programme's sample page plus the demo fixtures (lib/api/source.ts). */
function sampleCases(): SampleCase[] {
  const dir = path.join(process.cwd(), "data", "api");
  const pages = readdirSync(dir).filter((f) => /^cases_[a-z_]+_page1\.json$/.test(f));
  const fx = JSON.parse(readFileSync(path.join(dir, "fixtures.json"), "utf8")) as { fixtures: Record<string, SampleCase> };
  const all = [...Object.values(fx.fixtures), ...pages.flatMap((f) => (JSON.parse(readFileSync(path.join(dir, f), "utf8")) as { cases: SampleCase[] }).cases)];
  return [...new Map(all.map((c) => [c.id, c])).values()];
}

const sampleCount = (f: Partial<SampleCase>) => sampleCases().filter((c) => (Object.keys(f) as (keyof SampleCase)[]).every((k) => c[k] === f[k])).length;
const casesWord = (n: number) => `${n} ${n === 1 ? "case" : "cases"}`;

/** Opens the provider disclosure from the keyboard. */
async function openProviders(page: Page) {
  const summary = providerBlock(page).locator("summary");
  await summary.focus();
  await page.keyboard.press("Enter");
  await expect(providerBlock(page).locator("details")).toHaveAttribute("open", "");
}

test.describe("Programme: provider links", () => {
  test("one collapsed line; opened, one line per provider whose links carry their number and denominator", async ({ page }) => {
    await page.goto("/app");
    const block = providerBlock(page);
    await expect(block.getByRole("heading", { name: "By provider" })).toBeVisible();
    await expect(block.locator('[data-honesty="simulated"]')).toHaveText("Simulated cohort");
    await expect(block.locator("summary")).toHaveText("Closure and largest stall for 7 providers, linked to the patient list");
    await expect(block.getByRole("link")).toHaveCount(0);

    await openProviders(page);
    await expect(block.getByRole("listitem")).toHaveCount(7);
    const rad = block.getByTestId("provider-rad-imaging");
    for (const shown of ["rad-imaging, 905 cases", "168 of 905 closed", "largest stall: No follow-up CT booked, 498 of 905"]) await expect(rad).toContainText(shown);
    await expect(rad.getByRole("link", { name: "rad-imaging, 905 cases (simulated)" })).toHaveAttribute("href", "/app/patients?provider=rad-imaging");
    await expect(rad.getByRole("link", { name: "rad-imaging: 168 of 905 closed (simulated)" })).toHaveAttribute("href", "/app/patients?state=closed&provider=rad-imaging");
    await expect(rad.getByRole("link", { name: "rad-imaging: largest stall: No follow-up CT booked, 498 of 905 (simulated)" })).toHaveAttribute(
      "href",
      "/app/patients?dropoff=no_followup_ct&provider=rad-imaging",
    );

    // Every pattern link's accessible name holds its number out of the provider's cases, and says it is simulated.
    for (const l of await block.getByRole("link").all()) {
      const href = (await l.getAttribute("href"))!;
      await expect(l).toHaveAccessibleName(/[?&](state|dropoff)=/.test(href) ? / \d[\d,]* of \d[\d,]*( closed)? \(simulated\)$/ : /, \d[\d,]* cases \(simulated\)$/);
      await expect(l).not.toContainText("fewer than");
    }

    // Suppressed small cells: no number, no link.
    const breast = block.getByTestId("provider-bsu-screen");
    await expect(breast).toContainText("fewer than 5 of 70 closed");
    await expect(breast).toContainText("largest stall: each one fewer than 5 of 70");
    await expect(breast.getByRole("link")).toHaveCount(1);
    await expect(block).not.toContainText("<5 of");

    // No origin breakdown and no unlinkable timing or escalation patterns.
    await expect(block).not.toContainText("Notified within");
    await expect(block).not.toContainText("escalation");
    await expect(block).toContainText("Origin is not broken down here");
    expect(await unsourced(page)).toEqual([]);
  });

  test("by keyboard, a stall link lands on Patients filtered to that provider and stall", async ({ page }) => {
    await page.goto("/app");
    await openProviders(page);
    const link = providerBlock(page).getByRole("link", { name: /^rad-imaging: largest stall: No follow-up CT booked, 498 of 905/ });
    await link.focus();
    await expect(link).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/app\/patients\?dropoff=no_followup_ct&provider=rad-imaging$/);
    // Fixtures mode serves a published sample, not the whole run, so the list holds the sample's matching cases, not 498.
    const n = sampleCount({ provider: "rad-imaging", dropoff: "no_followup_ct" });
    expect(n).toBeGreaterThan(0);
    await expect(page.getByTestId("showing")).toHaveText(`${casesWord(n)}: provider rad-imaging · no follow-up ct booked`);
    await expect(page.getByTestId("active-filters")).toContainText("Provider: rad-imaging");
  });

  test("by keyboard, a closure link lands on Patients filtered to that provider's closed cases", async ({ page }) => {
    await page.goto("/app");
    await openProviders(page);
    const link = providerBlock(page).getByRole("link", { name: "ahs-mushrif: 523 of 1,606 closed (simulated)" });
    await link.focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/\/app\/patients\?state=closed&provider=ahs-mushrif$/);
    const n = sampleCount({ provider: "ahs-mushrif", state: "closed" });
    expect(n).toBeGreaterThan(0);
    await expect(page.getByTestId("showing")).toHaveText(`${casesWord(n)}: provider Al Mushrif · closed`);
    await expect(page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Closed" })).toHaveAttribute("aria-current", "true");
  });

  test("queue levers: aggregate direct-pathway and prep-risk counts over the open cases needing a slot, as plain text", async ({ page }) => {
    await page.goto("/app");
    const levers = page.getByTestId("capacity-levers");
    await expect(levers).toContainText("683 of 815 suggested direct to colonoscopy");
    await expect(levers).toContainText("238 of 815 at high risk of failed bowel prep");
    await expect(levers).toContainText("Counted over the 815 open colorectal cases needing a colonoscopy slot.");
    await expect(levers).toContainText("needs a clinician’s order");
    await expect(levers.getByRole("link")).toHaveCount(0);
  });

  for (const width of [390, 1440]) {
    test(`at ${width}px the block stays short, opens without sideways scroll and keeps an even rhythm`, async ({ browser }) => {
      const context = await browser.newContext({ viewport: { width, height: 900 }, hasTouch: width < 768 });
      const page = await context.newPage();
      await page.goto("/app");
      const block = providerBlock(page);
      await block.scrollIntoViewIfNeeded();
      const closed = (await block.boundingBox())!.height;
      expect(closed, "collapsed block height").toBeLessThan(400);
      await block.screenshot({ path: `${SHOTS}/after-${width}-closed.png` });
      await openProviders(page);
      const open = (await block.boundingBox())!.height;
      if (width >= 1024) expect(open, "opened block height at desktop width").toBeLessThan(400);
      test.info().annotations.push({ type: "heights", description: `${width}px: closed ${closed.toFixed(0)}px, open ${open.toFixed(0)}px` });
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
      // Linked and plain items share one height, 44px on touch widths.
      const heights = await block.locator("[data-item]").evaluateAll((els) =>
        els.map((el) => Math.round(el.getBoundingClientRect().height)),
      );
      expect(heights.length).toBeGreaterThan(14);
      if (width < 768) for (const h of heights) expect(h, "touch item height").toBeGreaterThanOrEqual(44);
      else expect(Math.max(...heights) - Math.min(...heights), "desktop items share one line height").toBeLessThanOrEqual(1);
      for (const l of await block.getByRole("link").all()) {
        const box = (await l.boundingBox())!;
        expect(box.x).toBeGreaterThanOrEqual(0);
        expect(box.x + box.width).toBeLessThanOrEqual(width);
      }
      await block.screenshot({ path: `${SHOTS}/after-${width}-open.png` });
      await page.screenshot({ path: `${SHOTS}/after-${width}-page.png`, fullPage: true });
      await context.close();
    });
  }
});
