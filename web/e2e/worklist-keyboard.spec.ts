import { expect as baseExpect, test, type Locator, type Page } from "@playwright/test";

const expect = baseExpect.configure({ timeout: 20_000 });
// A missing name or case must fail within seconds, not after the 2-minute test budget: actions (click, fill) get the same 20s as expect.
test.describe.configure({ timeout: 60_000 });
test.use({ actionTimeout: 20_000, navigationTimeout: 30_000 });

const shot = (page: Page, name: string) => page.screenshot({ path: `test-results/screens/kbd-${name}.png`, fullPage: true });

const PHONE = { width: 390, height: 844 };

/** Focus is on the element and it is not the body. */
const focused = (page: Page, loc: Locator) => expect(loc).toBeFocused();

/** The next Tab press lands somewhere inside `region`, not back at the site navigation. */
async function tabStaysIn(page: Page, region: Locator) {
  await page.keyboard.press("Tab");
  const inside = await region.evaluate((el) => el.contains(document.activeElement) && document.activeElement !== document.body);
  expect(inside).toBe(true);
}

/**
 * Undo is the last control in the referral panel, so the next Tab has nowhere further to go in the page;
 * what matters is that the order around it is the workflow's, not site navigation.
 */
async function shiftTabStaysIn(page: Page, region: Locator) {
  await page.keyboard.press("Shift+Tab");
  const inside = await region.evaluate((el) => el.contains(document.activeElement) && document.activeElement !== document.body);
  expect(inside).toBe(true);
  await page.keyboard.press("Tab");
}

/** WCAG contrast of the placeholder against the first opaque background behind the input, from computed colours. */
async function placeholderContrast(input: Locator) {
  return input.evaluate((el) => {
    const parse = (c: string) => {
      const m = c.match(/rgba?\(([^)]+)\)|color\(srgb ([^)]+)\)/);
      if (!m) throw new Error(`cannot parse ${c}`);
      let v = (m[1] ?? m[2]).split(/[ ,/]+/).filter(Boolean).map(Number);
      if (m[2]) v = [v[0] * 255, v[1] * 255, v[2] * 255, v[3] ?? 1];
      return { r: v[0], g: v[1], b: v[2], a: v[3] ?? 1 };
    };
    const over = (top: ReturnType<typeof parse>, under: ReturnType<typeof parse>) => ({
      r: top.r * top.a + under.r * (1 - top.a),
      g: top.g * top.a + under.g * (1 - top.a),
      b: top.b * top.a + under.b * (1 - top.a),
      a: 1,
    });
    let bg = { r: 255, g: 255, b: 255, a: 1 };
    const stack: Element[] = [];
    for (let n: Element | null = el; n; n = n.parentElement) stack.push(n);
    for (const n of stack.reverse()) {
      const c = parse(getComputedStyle(n).backgroundColor);
      if (c.a > 0) bg = over(c, bg);
    }
    const ph = getComputedStyle(el, "::placeholder");
    const fg = over(parse(ph.color), bg);
    const op = parseFloat(ph.opacity || "1");
    const shown = op < 1 ? over({ ...fg, a: op }, bg) : fg;
    const lum = (c: { r: number; g: number; b: number }) => {
      const f = (x: number) => ((x /= 255) <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4);
      return 0.2126 * f(c.r) + 0.7152 * f(c.g) + 0.0722 * f(c.b);
    };
    const [a, b] = [lum(shown), lum(bg)].sort((x, y) => y - x);
    return (a + 0.05) / (b + 0.05);
  });
}

/** The whole search control shows a 2px+ outline or ring when its input is focused. */
async function controlRing(input: Locator) {
  return input.evaluate((el) => {
    const box = el.closest("label")!;
    const s = getComputedStyle(box);
    return { width: parseFloat(s.outlineWidth), style: s.outlineStyle, color: s.outlineColor, border: s.borderColor };
  });
}

/* ---------- Today ---------- */

const TODAY_TABS = ["To call", "Waiting", "No owner"];

test.describe("Today: tabs", () => {
  test("one tab in the Tab order; arrows, Home and End move and select; the panel is the result list", async ({ page }) => {
    await page.goto("/app/today");
    const tabs = page.getByRole("tablist", { name: "Saved views" }).getByRole("tab");
    await expect(tabs).toHaveCount(3);
    expect(await tabs.evaluateAll((els) => els.map((e) => e.getAttribute("aria-controls")))).toEqual(Array(3).fill("today-panel"));
    expect(await tabs.evaluateAll((els) => els.map((e) => e.tabIndex))).toEqual([0, -1, -1]);
    const panel = page.getByRole("tabpanel");
    await expect(panel).toHaveAttribute("aria-labelledby", "today-tab-call");

    await tabs.nth(0).focus();
    await page.keyboard.press("ArrowRight");
    await focused(page, tabs.nth(1));
    await expect(tabs.nth(1)).toHaveAttribute("aria-selected", "true");
    await expect(panel).toHaveAttribute("aria-labelledby", "today-tab-waiting");
    await expect(page.getByRole("button", { name: /Aisha Al Mansoori/ }).first()).toBeVisible();
    expect(await tabs.evaluateAll((els) => els.map((e) => e.tabIndex))).toEqual([-1, 0, -1]);

    await page.keyboard.press("End");
    await focused(page, tabs.nth(2));
    await page.keyboard.press("ArrowRight");
    await focused(page, tabs.nth(0));
    await page.keyboard.press("ArrowLeft");
    await focused(page, tabs.nth(2));
    await page.keyboard.press("Home");
    await focused(page, tabs.nth(0));
    await expect(tabs.nth(0)).toHaveAttribute("aria-selected", "true");
    expect(await tabs.evaluateAll((els) => els.map((e) => e.textContent?.replace(/\s*\d+$/, "")))).toEqual(TODAY_TABS);

    // Tab leaves the tablist in one press: into the search field, not the next tab.
    await page.keyboard.press("Tab");
    await focused(page, page.getByRole("searchbox"));
    await page.keyboard.press("Shift+Tab");
    await focused(page, tabs.nth(0));
  });
});

test.describe("Today: stale patient actions (issue #12)", () => {
  const actions = (page: Page) => {
    const open = page.getByRole("region", { name: "Open patient" });
    return open.getByRole("button", { name: /^(Call |On the call|Snooze to tomorrow|Hand to Dr Al Kaabi|Take this patient)/ });
  };

  test("Hand-off advances to the next patient; a no-match search leaves no patient actions, only Undo", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "Hand to Dr Al Kaabi" }).click();
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1023");
    await expect(open.getByRole("button", { name: "Call AHS-sr-1023" })).toBeVisible();
    await expect(open.getByRole("button", { name: "Call AHS-sr-1002" })).toHaveCount(0);
    const notice = open.getByTestId("removed-notice");
    await expect(notice).toContainText("Logged: Handed to Dr Al Kaabi · AHS-sr-1002 moved to Waiting");
    await focused(page, notice.getByRole("button", { name: "Undo" }));
    await shot(page, "today-handoff");

    await page.getByRole("searchbox").fill("zzzz-no-match");
    await expect(page.getByText("Nobody left in this view.")).toBeVisible();
    await expect(page.getByText("0 to call")).toBeVisible();
    await expect(actions(page)).toHaveCount(0);
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("Nobody to open");
    await shot(page, "today-empty-search");

    // The only live control left is the way back.
    await open.getByRole("button", { name: "Undo" }).click();
    await page.getByRole("searchbox").fill("");
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1002");
    await expect(open.getByRole("button", { name: "Hand to Dr Al Kaabi" })).toBeVisible();
  });

  test("Snooze, then a search that excludes the snoozed patient, never leaves their actions", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "Snooze to tomorrow" }).click();
    await expect(open.getByRole("status")).toContainText("Logged: Snoozed to tomorrow · AHS-sr-1002 moved to the end");
    await focused(page, open.getByRole("button", { name: "Undo" }));

    await page.getByRole("searchbox").fill("1023");
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1023");
    await expect(open.getByRole("button", { name: "Snooze to tomorrow" })).toHaveCount(1);
    await expect(open.getByRole("button", { name: "Call AHS-sr-1002" })).toHaveCount(0);

    await page.getByRole("searchbox").fill("zzzz-no-match");
    await expect(actions(page)).toHaveCount(0);
    await expect(page.getByText("Nobody left in this view.")).toBeVisible();
  });

  test("No answer, then a filter change, never leaves the patient's actions outside the results", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "No answer" }).click();
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1002");

    await page.getByRole("searchbox").fill("zzzz-no-match");
    await expect(actions(page)).toHaveCount(0);
    await page.getByRole("searchbox").fill("");
    await page.getByRole("tab", { name: /^Waiting/ }).click();
    await expect(open.getByRole("heading", { level: 2 })).not.toHaveText("AHS-sr-1002");
    await expect(open.getByRole("button", { name: "Call AHS-sr-1002" })).toHaveCount(0);
    await page.getByRole("searchbox").fill("1002");
    await expect(actions(page)).toHaveCount(0);
    await expect(page.getByText("Nobody left in this view.")).toBeVisible();
  });
});

test.describe("Today: focus through the call workflow (issue #14)", () => {
  test("Reached moves focus to the next group; Book lands on Undo and Tab stays in the workflow", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "Call AHS-sr-1002" }).focus();
    await page.keyboard.press("Enter");
    await expect(open.getByRole("group", { name: "How did it go?" })).toBeVisible();
    await open.getByRole("button", { name: "Reached" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, open.getByRole("heading", { name: "What next?" }));
    await tabStaysIn(page, open);
    await focused(page, open.getByRole("button", { name: "Book" }));

    await page.keyboard.press("Enter");
    await expect(open.getByTestId("removed-notice")).toContainText("Logged: Booked");
    await focused(page, open.getByTestId("removed-notice").getByRole("button", { name: "Undo" }));
    await tabStaysIn(page, open);
  });

  test("No answer lands on Undo; Tab continues in the patient panel; Undo returns to the patient heading", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "No answer" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, open.getByRole("button", { name: "Undo" }));
    await tabStaysIn(page, open);
    await focused(page, open.getByRole("button", { name: /^Next patient/ }));
    await page.keyboard.press("Shift+Tab");
    await focused(page, open.getByRole("button", { name: "Undo" }));
    await page.keyboard.press("Enter");
    await focused(page, open.getByRole("heading", { level: 2, name: "AHS-sr-1002" }));
    await tabStaysIn(page, open);
  });

  test("Call back on… moves focus to its group; picking a day lands on Undo", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "Reached" }).focus();
    await page.keyboard.press("Enter");
    await open.getByRole("button", { name: "Call back on…" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, open.getByRole("heading", { name: "Call back on" }));
    await page.keyboard.press("Tab");
    await page.keyboard.press("Enter");
    await focused(page, open.getByRole("button", { name: "Undo" }));
  });

  test("Next patient hands focus to the next patient's heading", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "No answer" }).click();
    await open.getByRole("button", { name: /^Next patient/ }).focus();
    await page.keyboard.press("Enter");
    await focused(page, open.getByRole("heading", { level: 2, name: "AHS-sr-1023" }));
  });
});

for (const [label, viewport] of [
  ["desktop", { width: 1440, height: 900 }],
  ["phone", PHONE],
] as const) {
  test.describe(`Today: search control at ${label} width`, () => {
    test.use({ viewport });

    test("focus ring surrounds the whole control; placeholder reaches 4.5:1 in every state", async ({ page }) => {
      await page.goto("/app/today");
      if (viewport.width < 1024) await page.getByRole("button", { name: /Your list/ }).click();
      const input = page.getByRole("searchbox", { name: "Search" });
      await expect(input).toHaveAttribute("placeholder", "Name or case id");

      expect(await placeholderContrast(input)).toBeGreaterThanOrEqual(4.5);
      const idle = await controlRing(input);
      expect(idle.width === 0 || idle.style === "none").toBe(true);

      await input.focus();
      await focused(page, input);
      const on = await controlRing(input);
      expect(on.style).toBe("solid");
      expect(on.width).toBeGreaterThanOrEqual(2);
      expect(await placeholderContrast(input)).toBeGreaterThanOrEqual(4.5);
      const box = await input.evaluate((el) => {
        const l = el.closest("label")!.getBoundingClientRect();
        const i = el.getBoundingClientRect();
        return { wider: l.width > i.width + 20, taller: l.height >= i.height };
      });
      expect(box).toEqual({ wider: true, taller: true });
      await page.getByRole("searchbox").hover();
      expect(await placeholderContrast(input)).toBeGreaterThanOrEqual(4.5);
      await shot(page, `today-search-focus-${viewport.width}`);

      // Typing and clearing keeps the ring and the focus.
      await page.keyboard.type("1023");
      await focused(page, input);
      await page.keyboard.press("Control+a");
      await page.keyboard.press("Backspace");
      await focused(page, input);
      expect(await placeholderContrast(input)).toBeGreaterThanOrEqual(4.5);
    });
  });
}

/* ---------- Referrals ---------- */

test.describe("Referrals: tabs", () => {
  test("one tab in the Tab order; arrows, Home and End move and select; the panel is the result list", async ({ page }) => {
    await page.goto("/app/referrals");
    const tabs = page.getByRole("tablist", { name: "Saved views" }).getByRole("tab");
    await expect(tabs).toHaveCount(4);
    expect(await tabs.evaluateAll((els) => els.map((e) => e.getAttribute("aria-controls")))).toEqual(Array(4).fill("referrals-panel"));
    expect(await tabs.evaluateAll((els) => els.map((e) => e.tabIndex))).toEqual([0, -1, -1, -1]);
    const panel = page.getByRole("tabpanel");
    await expect(panel).toHaveAttribute("aria-labelledby", "referrals-tab-needs");

    await tabs.nth(0).focus();
    await page.keyboard.press("ArrowRight");
    await focused(page, tabs.nth(1));
    await expect(tabs.nth(1)).toHaveAttribute("aria-selected", "true");
    await expect(panel).toHaveAttribute("aria-labelledby", "referrals-tab-all");
    await page.keyboard.press("End");
    await focused(page, tabs.nth(3));
    await expect(tabs.nth(3)).toHaveAttribute("aria-selected", "true");
    await page.keyboard.press("ArrowRight");
    await focused(page, tabs.nth(0));
    await page.keyboard.press("ArrowLeft");
    await focused(page, tabs.nth(3));
    await page.keyboard.press("Home");
    await focused(page, tabs.nth(0));
    expect(await tabs.evaluateAll((els) => els.map((e) => e.tabIndex))).toEqual([0, -1, -1, -1]);
    await page.keyboard.press("Tab");
    await focused(page, page.getByRole("searchbox"));
  });
});

test.describe("Referrals: focus through the decision workflow (issue #14)", () => {
  // FIXME(data): the run's published CRC sample (data/api/cases_crc_page1.json, 25 cases) holds no referral the unit has not answered
  // (no referral_unanswered or not_booked drop-off), so the Accept / Accept with conditions / Return to GP controls are never shown.
  // The controls are designed, not built; this test comes back when a sample carries such a referral.
  test.fixme("Accept referral lands on Undo; Tab continues in the panel; Undo returns to the heading", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await pane.getByRole("button", { name: "Accept referral" }).focus();
    await page.keyboard.press("Enter");
    await expect(pane.getByRole("status")).toContainText("Accepted. Your unit books her now.");
    await focused(page, pane.getByRole("button", { name: "Undo" }));
    await shiftTabStaysIn(page, page.locator("main"));
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("heading", { level: 2, name: "AHS-sr-1004" }));
    await tabStaysIn(page, pane);
  });

  // FIXME(data): the run's published CRC sample (data/api/cases_crc_page1.json, 25 cases) holds no referral the unit has not answered
  // (no referral_unanswered or not_booked drop-off), so the Accept / Accept with conditions / Return to GP controls are never shown.
  // The controls are designed, not built; this test comes back when a sample carries such a referral.
  test.fixme("the inline Accept button in the list hands focus to Undo", async ({ page }) => {
    await page.goto("/app/referrals");
    await page.getByRole("button", { name: "Accept referral for AHS-sr-1004" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, page.getByRole("region", { name: "Open referral" }).getByRole("button", { name: "Undo" }));
  });

  // FIXME(data): the run's published CRC sample (data/api/cases_crc_page1.json, 25 cases) holds no referral the unit has not answered
  // (no referral_unanswered or not_booked drop-off), so the Accept / Accept with conditions / Return to GP controls are never shown.
  // The controls are designed, not built; this test comes back when a sample carries such a referral.
  test.fixme("Accept with conditions: focus goes to the group heading, then the first choice; Cancel returns to the opener", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await pane.getByRole("button", { name: "Accept with conditions" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("heading", { name: "Accept on these conditions" }));
    await expect(pane.getByRole("group", { name: "Conditions" })).toBeVisible();
    await page.keyboard.press("Tab");
    await focused(page, pane.getByRole("button", { name: "Hold blood thinners as advised" }));
    await page.keyboard.press("Space");
    await expect(pane.getByRole("button", { name: "Hold blood thinners as advised" })).toHaveAttribute("aria-pressed", "true");
    await focused(page, pane.getByRole("button", { name: "Hold blood thinners as advised" }));
    await shot(page, "referrals-conditions-focus");

    await pane.getByRole("button", { name: "Cancel" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("button", { name: "Accept with conditions" }));
    await tabStaysIn(page, pane);

    await pane.getByRole("button", { name: "Accept with conditions" }).focus();
    await page.keyboard.press("Enter");
    await pane.getByRole("button", { name: "Female endoscopist" }).focus();
    await page.keyboard.press("Enter");
    await pane.getByRole("button", { name: "Accept with 1 condition" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("button", { name: "Undo" }));
  });

  // FIXME(data): the run's published CRC sample (data/api/cases_crc_page1.json, 25 cases) holds no referral the unit has not answered
  // (no referral_unanswered or not_booked drop-off), so the Accept / Accept with conditions / Return to GP controls are never shown.
  // The controls are designed, not built; this test comes back when a sample carries such a referral.
  test.fixme("Return to GP: focus goes to the reason heading, then the first reason; sending lands on Undo", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await pane.getByRole("button", { name: "Return to GP" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("heading", { name: "Why return this referral?" }));
    await page.keyboard.press("Tab");
    await focused(page, pane.getByRole("button", { name: "Needs a clinic visit first" }));
    await page.keyboard.press("Space");
    await pane.getByRole("button", { name: "Return to GP" }).focus();
    await page.keyboard.press("Enter");
    await expect(pane.getByRole("status")).toContainText("Returned to the Al Mushrif GP. Reason: needs a clinic visit first.");
    await focused(page, pane.getByRole("button", { name: "Undo" }));
    await shiftTabStaysIn(page, page.locator("main"));
  });

  // FIXME(data): the run's published CRC sample (data/api/cases_crc_page1.json, 25 cases) holds no referral the unit has not answered
  // (no referral_unanswered or not_booked drop-off), so the Accept / Accept with conditions / Return to GP controls are never shown.
  // The controls are designed, not built; this test comes back when a sample carries such a referral.
  test.fixme("Cancel from the return reasons goes back to Return to GP", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await pane.getByRole("button", { name: "Cancel" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("button", { name: "Return to GP" }));
  });

  test("grading the prep lands on Undo", async ({ page }) => {
    await page.goto("/app/referrals");
    await page.getByRole("button", { name: "Review the referral for Aisha Al Mansoori" }).click();
    const pane = page.getByRole("region", { name: "Open referral" });
    await pane.getByRole("group", { name: "Boston grade" }).getByRole("button", { name: /^2\s*Mostly seen$/ }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("button", { name: "Undo" }));
  });
});

for (const [label, viewport] of [
  ["desktop", { width: 1440, height: 900 }],
  ["phone", PHONE],
] as const) {
  test.describe(`Referrals: search control at ${label} width`, () => {
    test.use({ viewport });

    test("focus ring surrounds the whole control; placeholder reaches 4.5:1; focus survives Enter", async ({ page }) => {
      await page.goto("/app/referrals");
      if (viewport.width < 1024) await page.getByRole("button", { name: /All referrals/ }).click();
      const input = page.getByRole("searchbox", { name: "Search case id" });
      await expect(input).toHaveAttribute("placeholder", "Case id, then Enter");

      expect(await placeholderContrast(input)).toBeGreaterThanOrEqual(4.5);
      await input.focus();
      await focused(page, input);
      const on = await controlRing(input);
      expect(on.style).toBe("solid");
      expect(on.width).toBeGreaterThanOrEqual(2);
      expect(await placeholderContrast(input)).toBeGreaterThanOrEqual(4.5);
      await shot(page, `referrals-search-focus-${viewport.width}`);

      await page.keyboard.type("1011");
      await page.keyboard.press("Enter");
      await expect(page).toHaveURL(/\/app\/referrals\?q=1011$/);
      if (viewport.width < 1024) await page.getByRole("button", { name: /All referrals/ }).click();
      await expect(page.getByRole("searchbox", { name: "Search case id" })).toHaveValue("1011");
      // The page reloads for the query. On desktop the search field takes focus back; on a phone the list pane is
      // closed after a reload (the open referral shows first), so there is nothing visible to focus.
      if (viewport.width >= 1024) await focused(page, page.getByRole("searchbox", { name: "Search case id" }));
    });
  });
}
