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

const open = (page: Page) => page.getByRole("region", { name: "Selected case" });
const caseRow = (page: Page, id: string) => page.locator(`button[data-case-row="${id}"]`);
const searchbox = (page: Page) => page.getByRole("searchbox", { name: "Find a case or task" });

test.describe("Today: queue keyboard access", () => {
  test("the sidebar's queue links switch queues; / focuses the search; Enter on a row opens its case", async ({ page }) => {
    await page.goto("/app/today");
    const nav = page.getByRole("navigation", { name: "Main", exact: true });
    await nav.getByRole("link", { name: /^Waiting/ }).focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/queue=waiting/);
    await expect(page.getByRole("heading", { level: 2, name: /^Waiting/ })).toBeVisible();
    await expect(caseRow(page, "AHS-sr-1011")).toBeVisible();

    // Click outside any field. Not main's corner: the sticky top bar can cover it once the page has scrolled.
    await page.getByRole("heading", { level: 1 }).click();
    await page.keyboard.press("/");
    await focused(page, searchbox(page));
    // Typing a slash in the field types it; it does not jump anywhere.
    await page.keyboard.type("/");
    await expect(searchbox(page)).toHaveValue("/");
    await searchbox(page).fill("");

    await caseRow(page, "AHS-sr-1011").focus();
    await page.keyboard.press("Enter");
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1011");
    await expect(caseRow(page, "AHS-sr-1011")).toHaveAttribute("aria-expanded", "true");
    // The open case follows its row in the reading order: Tab goes into the case's own controls.
    await page.keyboard.press("Tab");
    await expect(open(page).getByRole("button", { name: "Case details" })).toBeFocused();
  });
});

test.describe("Today: stale patient actions (issue #12)", () => {
  const actions = (page: Page) => page.getByRole("button", { name: /^(Log call|Logging call|Snooze to tomorrow|Hand to Dr Al Kaabi|Take this patient)/ });

  test("Hand-off keeps the case open with a way on to the next; a no-match search leaves no case actions, only Undo", async ({ page }) => {
    await page.goto("/app/today");
    await open(page).getByRole("button", { name: "Hand to Dr Al Kaabi" }).click();
    // The handed-off case stays open (it is now in Waiting), with Undo, a way to its queue and to the next case.
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1002");
    await expect(open(page).getByRole("status")).toContainText("Logged: Handed to Dr Al Kaabi · AHS-sr-1002 moved to Waiting");
    await expect(open(page).getByTestId("removed-notice")).toHaveCount(0);
    await focused(page, open(page).getByRole("button", { name: "Undo" }));
    await expect(open(page).getByTestId("open-destination")).toHaveText("Open Waiting");
    await expect(caseRow(page, "AHS-sr-1002")).toHaveCount(0);
    await expect(open(page).getByRole("button", { name: "Next case: AHS-sr-1023" })).toBeVisible();
    await shot(page, "today-handoff");

    // A search that excludes the held case lets go of it: the way back is all that is left.
    await searchbox(page).fill("zzzz-no-match");
    await expect(page.getByText("No case matches this search.")).toBeVisible();
    await expect(page.getByText("0 to call")).toBeVisible();
    await expect(actions(page)).toHaveCount(0);
    await expect(open(page)).toHaveCount(0);
    await shot(page, "today-empty-search");

    // The only live control left is the way back.
    await page.getByTestId("removed-notice").getByRole("button", { name: "Undo" }).click();
    await searchbox(page).fill("");
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1002");
    await expect(open(page).getByRole("button", { name: "Hand to Dr Al Kaabi" })).toBeVisible();
  });

  test("Snooze, then a search that excludes the snoozed case, never leaves its actions", async ({ page }) => {
    await page.goto("/app/today");
    await open(page).getByRole("button", { name: "Snooze to tomorrow" }).click();
    await expect(open(page).getByRole("status")).toContainText("Logged: Snoozed to tomorrow · AHS-sr-1002 moved to the end");
    await focused(page, open(page).getByRole("button", { name: "Undo" }));

    await searchbox(page).fill("1023");
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1023");
    await expect(open(page).getByRole("button", { name: "Snooze to tomorrow" })).toHaveCount(1);
    await expect(page.getByRole("button", { name: "Log call · AHS-sr-1002" })).toHaveCount(0);

    await searchbox(page).fill("zzzz-no-match");
    await expect(actions(page)).toHaveCount(0);
    await expect(page.getByText("No case matches this search.")).toBeVisible();
  });

  test("No answer, then a queue change, never leaves the case's actions outside the results", async ({ page }) => {
    await page.goto("/app/today");
    await open(page).getByRole("button", { name: "No answer" }).click();
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1002");

    await searchbox(page).fill("zzzz-no-match");
    await expect(actions(page)).toHaveCount(0);
    await searchbox(page).fill("");
    await page.getByRole("navigation", { name: "Main", exact: true }).getByRole("link", { name: /^Waiting/ }).click();
    await expect(page).toHaveURL(/queue=waiting/);
    await expect(open(page)).not.toHaveAttribute("data-case", "AHS-sr-1002");
    await expect(page.getByRole("button", { name: "Log call · AHS-sr-1002" })).toHaveCount(0);
    await searchbox(page).fill("1002");
    await expect(actions(page)).toHaveCount(0);
    await expect(page.getByText("No case matches this search.")).toBeVisible();
  });
});

test.describe("Today: focus through the call workflow (issue #14)", () => {
  test("Reached moves focus to the next group; Appointment agreed lands on Undo and Tab stays in the workflow", async ({ page }) => {
    await page.goto("/app/today");
    await open(page).getByRole("button", { name: "Log call · AHS-sr-1002" }).focus();
    await page.keyboard.press("Enter");
    await expect(open(page).getByRole("group", { name: "How did it go?" })).toBeVisible();
    await open(page).getByRole("button", { name: "Reached" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, open(page).getByRole("heading", { name: "What next?" }));
    await tabStaysIn(page, open(page));
    await focused(page, open(page).getByRole("button", { name: "Appointment agreed" }));

    await page.keyboard.press("Enter");
    // The case moves to Waiting but stays open, so the outcome is seen where it was recorded.
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1002");
    await expect(open(page).getByRole("status")).toContainText("Logged: Appointment agreed · AHS-sr-1002 moved to Waiting");
    await focused(page, open(page).getByRole("button", { name: "Undo" }));
    await tabStaysIn(page, open(page));
    await focused(page, open(page).getByTestId("open-destination"));
  });

  test("No answer lands on Undo; Tab continues in the case; Undo returns to the task heading", async ({ page }) => {
    await page.goto("/app/today");
    await open(page).getByRole("button", { name: "No answer" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, open(page).getByRole("button", { name: "Undo" }));
    await tabStaysIn(page, open(page));
    await focused(page, open(page).getByRole("button", { name: /^Next case/ }));
    await page.keyboard.press("Shift+Tab");
    await focused(page, open(page).getByRole("button", { name: "Undo" }));
    await page.keyboard.press("Enter");
    await focused(page, open(page).getByRole("heading", { level: 2, name: "Receipt outstanding" }));
    await tabStaysIn(page, open(page));
  });

  test("Call back on… moves focus to its group; picking a day lands on Undo", async ({ page }) => {
    await page.goto("/app/today");
    await open(page).getByRole("button", { name: "Reached" }).focus();
    await page.keyboard.press("Enter");
    await open(page).getByRole("button", { name: "Call back on…" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, open(page).getByRole("heading", { name: "Call back on" }));
    await page.keyboard.press("Tab");
    await page.keyboard.press("Enter");
    await focused(page, open(page).getByRole("button", { name: "Undo" }));
  });

  test("Next case hands focus to the next case's task heading", async ({ page }) => {
    await page.goto("/app/today");
    await open(page).getByRole("button", { name: "No answer" }).click();
    await open(page).getByRole("button", { name: /^Next case/ }).focus();
    await page.keyboard.press("Enter");
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1023");
    await focused(page, open(page).getByRole("heading", { level: 2, name: "Receipt outstanding" }));
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
      const input = searchbox(page);
      await expect(input).toHaveAttribute("placeholder", "Find a case or task");

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
      await input.hover();
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

test.describe("Referrals: the inbox by keyboard", () => {
  test("rows are links in the Tab order; a row opens the referral; Back to inbox returns focus to that row", async ({ page }) => {
    await page.goto("/app/referrals?view=inbox");
    const first = page.getByRole("link", { name: "AHS-sr-9003" });
    await first.focus();
    await page.keyboard.press("Tab");
    await focused(page, page.getByRole("link", { name: "AHS-sr-9002" }));
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/from=inbox&case=/);
    await focused(page, page.getByRole("heading", { level: 1 }).locator("[data-focus=heading]"));
    await page.getByRole("link", { name: "Back to inbox" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, page.getByRole("link", { name: "AHS-sr-9002" }));
  });
});

test.describe("Referrals: focus through the decision workflow (issue #14)", () => {
  test("Accept referral lands on Undo; Undo returns to the heading", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await pane.getByRole("button", { name: "Accept referral" }).focus();
    await page.keyboard.press("Enter");
    await expect(pane.getByRole("status")).toContainText("Accepted. Your unit books her now.");
    await focused(page, pane.getByRole("button", { name: "Undo" }));
    await shiftTabStaysIn(page, page.locator("main"));
    await pane.getByRole("button", { name: "Undo" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.locator("[data-focus=heading]"));
  });

  test("Accept with conditions: focus goes to the group heading, then the first choice; Cancel returns to the opener", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await pane.getByRole("button", { name: "Accept with conditions" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("heading", { name: "Accept on these conditions" }));
    await page.keyboard.press("Tab");
    await focused(page, pane.getByRole("button", { name: "Hold blood thinners as advised" }));
    await page.keyboard.press("Space");
    await expect(pane.getByRole("button", { name: "Hold blood thinners as advised" })).toHaveAttribute("aria-pressed", "true");
    await shot(page, "referrals-conditions-focus");
    await pane.getByRole("button", { name: "Cancel" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("button", { name: "Accept with conditions" }));
    await page.keyboard.press("Enter");
    await pane.getByRole("button", { name: "Female endoscopist" }).focus();
    await page.keyboard.press("Enter");
    await pane.getByRole("button", { name: "Accept with 1 condition" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("button", { name: "Undo" }));
  });

  test("Return: focus goes to the reason group, then the first reason; sending lands on Undo; Cancel goes back to Return", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await pane.getByRole("button", { name: "Return with reason" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByText("Reason for return", { exact: true }));
    await page.keyboard.press("Tab");
    await focused(page, pane.getByRole("radio", { name: "Needs a clinic visit first" }));
    await page.keyboard.press("Space");
    await pane.getByRole("button", { name: "Return to GP" }).focus();
    await page.keyboard.press("Enter");
    await expect(pane.getByRole("status")).toContainText("Returned to the Al Mushrif GP. Reason: needs a clinic visit first.");
    await focused(page, pane.getByRole("button", { name: "Undo" }));
    await page.keyboard.press("Enter");
    await pane.getByRole("button", { name: "Return with reason" }).click();
    await pane.getByRole("button", { name: "Cancel" }).focus();
    await page.keyboard.press("Enter");
    await focused(page, pane.getByRole("button", { name: "Return with reason" }));
  });

  test("grading the prep lands on Undo", async ({ page }) => {
    await page.goto("/app/referrals?status=report");
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
      await page.goto("/app/referrals?view=inbox");
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
      await expect(page).toHaveURL(/\/app\/referrals\?q=1011&view=inbox$/);
      await expect(page.getByRole("searchbox", { name: "Search case id" })).toHaveValue("1011");
      // The query is a client navigation, so the field keeps focus at every width.
      await focused(page, page.getByRole("searchbox", { name: "Search case id" }));
    });
  });
}
