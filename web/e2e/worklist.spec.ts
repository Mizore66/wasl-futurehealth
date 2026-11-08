import AxeBuilder from "@axe-core/playwright";
import { expect as baseExpect, test, type Page } from "@playwright/test";

// Six dev servers share this machine during phase 2; first compiles of a route can be slow.
const expect = baseExpect.configure({ timeout: 20_000 });
test.describe.configure({ timeout: 120_000 });

const shot = (page: Page, name: string) => page.screenshot({ path: `test-results/screens/worklist-${name}.png`, fullPage: true });

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

const aisha = /\/app\/patients\/ahs-mushrif(%2F|\/)ServiceRequest(%2F|\/)AHS-sr-1009$/;
const fatima = `/app/patients/${encodeURIComponent("ahs-mushrif/ServiceRequest/AHS-sr-1037")}`;

test.describe("Today", () => {
  test("finds the most urgent patient and opens them", async ({ page }) => {
    await page.goto("/app/today");
    await expect(page.getByTestId("finding")).toHaveText("AHS-sr-1004 is 82 days over 31: not booked. 11 more to call.");
    await expect(page.getByRole("tab", { name: "To call 12" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tab", { name: "Waiting 6" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "No owner 3" })).toBeVisible();
    const open = page.getByRole("region", { name: "Open patient" });
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1004");
    await expect(open.getByTestId("lead-days")).toHaveAttribute("data-value", "113");
    await expect(page.locator("[data-honesty=designed]")).toHaveText("Call log: designed, not built");
    await shot(page, "today");
    await noSeriousAxe(page);
  });

  test("logging No answer keeps the patient open, offers Undo and the next patient", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "No answer" }).click();
    const status = open.getByRole("status");
    await expect(status).toContainText("Logged: No answer · AHS-sr-1004 moved to the end · Undo");
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1004");
    await expect(open.getByText("Called, no answer")).toBeVisible();
    await expect(page.getByRole("button", { name: /AHS-sr-1004/ }).first()).toContainText("No answer ×1 · retry tomorrow");
    await shot(page, "today-logged");

    await status.getByRole("button", { name: "Undo" }).click();
    await expect(open.getByRole("status")).toHaveCount(0);
    await expect(open.getByText("Called, no answer")).toHaveCount(0);
    await expect(open.getByRole("group", { name: "After the call" })).toBeVisible();

    await open.getByRole("button", { name: "No answer" }).click();
    await open.getByRole("button", { name: "Next patient: AHS-sr-1011" }).click();
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1011");
  });

  test("Call, then Reached offers Book and Call back on…", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "Call AHS-sr-1004" }).click();
    await expect(open.getByRole("button", { name: "On the call · AHS-sr-1004" })).toBeVisible();
    await expect(open.getByRole("group", { name: "How did it go?" })).toBeVisible();

    await open.getByRole("button", { name: "Reached" }).click();
    const next = open.getByRole("group", { name: "What next?" });
    await expect(next.getByRole("button")).toHaveText(["Book", "Call back on…", "Declined"]);
    await expect(open.getByRole("button", { name: /^Next patient/ })).toHaveCount(0);

    await next.getByRole("button", { name: "Call back on…" }).click();
    const days = open.getByRole("group", { name: "Call back on" });
    await expect(days.getByRole("button")).toHaveText(["Sun 30 Aug", "Mon 31 Aug", "Tue 1 Sept"]);
    await days.getByRole("button", { name: "Sun 30 Aug" }).click();
    await expect(open.getByRole("status")).toContainText("Logged: Call back Sun 30 Aug");
    await expect(open.getByRole("button", { name: /^Next patient/ })).toBeVisible();
    await shot(page, "today-callback");
  });

  test("Book moves the patient to Waiting; Take moves an unowned one to To call", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "Reached" }).click();
    await open.getByRole("button", { name: "Book" }).click();
    await expect(open.getByRole("status")).toContainText("Logged: Booked · AHS-sr-1004 moved to Waiting");
    await expect(page.getByRole("tab", { name: "To call 11" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Waiting 7" })).toBeVisible();

    await page.getByRole("tab", { name: "No owner 3" }).click();
    await open.getByRole("button", { name: "Take this patient" }).click();
    await expect(page.getByRole("tab", { name: "To call 12" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "No owner 2" })).toBeVisible();
  });

  test("search narrows the list; Waiting opens Aisha's record", async ({ page }) => {
    await page.goto("/app/today");
    await page.getByRole("searchbox").fill("1014");
    await expect(page.getByRole("region", { name: "Your list" }).getByRole("button", { pressed: true })).toContainText("AHS-sr-1014");
    await page.getByRole("searchbox").fill("");
    await page.getByRole("tab", { name: "Waiting 6" }).click();
    await page.getByRole("button", { name: /Aisha Al Mansoori/ }).click();
    await page.getByRole("link", { name: "Open record" }).click();
    await expect(page).toHaveURL(/AHS-sr-1009$/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Aisha Al Mansoori");
  });

  test("avatars open the record where there is one; elsewhere they select the row", async ({ page }) => {
    await page.goto("/app/today");
    const list = page.getByRole("region", { name: "Your list" });
    await expect(page.getByRole("link", { name: "Open AHS-sr-1011's record" })).toHaveCount(0);
    const row = list.getByRole("button", { name: /AHS-sr-1011/ });
    await row.locator("..").click({ position: { x: 22, y: 24 } });
    await expect(row).toHaveAttribute("aria-pressed", "true");

    await page.getByRole("tab", { name: "Waiting 6" }).click();
    await list.getByRole("button", { name: /Aisha Al Mansoori/ }).click();
    await expect(page.getByRole("region", { name: "Open patient" }).getByRole("link", { name: "Open Aisha Al Mansoori's record" })).toBeVisible();
    await shot(page, "today-avatar-links");
    await noSeriousAxe(page);
    await list.getByRole("link", { name: "Open Aisha Al Mansoori's record" }).click();
    await expect(page).toHaveURL(aisha);
  });
});

test.describe("Patients", () => {
  test("filters from the query string, as Programme's stall links send them", async ({ page }) => {
    await page.goto("/app/patients?dropoff=never_notified&overdue=true");
    await expect(page.getByTestId("showing")).toHaveText("6 cases: never notified · overdue");
    await expect(page.locator("tbody tr")).toHaveCount(6);
    await expect(page.locator("[data-honesty]").first()).toBeVisible();
    await shot(page, "patients-filtered");
    await noSeriousAxe(page);

    await page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Closed" }).click();
    await expect(page).toHaveURL(/state=closed/);
    await expect(page.getByTestId("showing")).toContainText("cases: closed");
    await expect(page.locator("tbody tr").first()).toContainText("Closed day");
  });

  test("search by case id leads to the record", async ({ page }) => {
    await page.goto("/app/patients");
    await expect(page.getByTestId("showing")).toHaveText("178 cases: all cases");
    await shot(page, "patients");
    await page.getByRole("searchbox", { name: "Search case id" }).fill("1037");
    await page.getByRole("button", { name: "Search" }).click();
    await expect(page).toHaveURL(/q=1037/);
    await page.getByRole("link", { name: "Fatima Al Zaabi" }).click();
    await expect(page).toHaveURL(fatima);
  });

  test("the avatar is part of the record link", async ({ page }) => {
    await page.goto("/app/patients?q=1037");
    await page.getByRole("link", { name: "Fatima Al Zaabi" }).locator(".wav-root").click();
    await expect(page).toHaveURL(fatima);
  });
});

test.describe("Patient record", () => {
  test("Fatima: two names bridged, evidence quoted, chain verified", async ({ page }) => {
    await page.goto(fatima);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Fatima Al Zaabi");
    await expect(page.getByText("Closed day 27")).toBeVisible();
    await expect(page.getByTestId("closed-in")).toHaveText("Closed in 27 days, 4 inside the 31-day standard");
    const chain = page.getByRole("list", { name: "Sealed chain" });
    await expect(chain.getByRole("listitem").last()).toContainText("Closed · with evidence, dated to the colonoscopy");
    await expect(page.getByText("فاطمة الزعابي").first()).toBeVisible();
    await expect(page.getByText("Fatima Alzaabi")).toBeVisible();
    await expect(page.getByText("“advanced carefully through the colon into the cecum and then through the ileocecal valve into the terminal ileum”")).toBeVisible();
    await expect(page.getByTestId("chain-verified")).toContainText("Verified · 3 of 3 intact");
    await expect(page.getByText("Message owner")).toHaveCount(0);

    await page.getByText("3 sealed entries").click();
    await expect(page.getByRole("list", { name: "Entry hashes" }).getByRole("listitem")).toHaveText([/Report matched/, /Readers' result/, /Closed/]);
    await shot(page, "record-fatima");
    await noSeriousAxe(page);
  });

  test("Aisha: held, the scope stopped short and the histology is missing", async ({ page }) => {
    await page.goto(`/app/patients/${encodeURIComponent("ahs-mushrif/ServiceRequest/AHS-sr-1009")}`);
    await expect(page.getByText("Safety gate: held").first()).toBeVisible();
    await expect(page.getByText("“the procedure was aborted at the level of the hepatic flexure”")).toBeVisible();
    await expect(page.getByRole("region", { name: "DoH closure checklist" }).getByRole("listitem")).toHaveText([/Caecum reached\s*No/, /Prep graded\s*Not stated/, /Histology, if tissue taken\s*Missing/]);
    await shot(page, "record-aisha");
  });

  test("a DR case reads each reader's label instead of the CRC checklist", async ({ page }) => {
    await page.goto(`/app/patients/${encodeURIComponent("phc-retina/ServiceRequest/PHC-sr-1029")}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("PHC-sr-1029");
    await expect(page.getByTestId("closed-in")).toHaveText("Closed in 47 days, 44 inside the 91-day deadline");
    const readers = page.getByRole("region", { name: "Readers' result" });
    await expect(readers.getByRole("listitem")).toHaveText([/Reader A\s*examined followup/, /Reader B\s*examined followup/]);
    await expect(readers).toContainText("Fundus examined after dilation");
    await expect(page.getByRole("region", { name: "DoH closure checklist" })).toHaveCount(0);
    await expect(page.getByTestId("chain-verified")).toContainText("Verified · 5 of 5 intact");
    await expect(page.getByRole("list", { name: "Sealed chain" }).getByRole("listitem").last()).toContainText("Closed · with evidence, dated to the eye-clinic exam");
    await shot(page, "record-dr");
    await noSeriousAxe(page);
  });
});

test.describe("Referrals", () => {
  test("opens on Needs you; flagged rows show Review, clean rows Accept", async ({ page }) => {
    await page.goto("/app/referrals");
    await expect(page.getByRole("tab", { name: "Needs you 5" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByTestId("finding")).toHaveText("AHS-sr-1004 has waited 113 days, 106 past the unit's 7. AHS-sr-1018's report needs your review.");
    await expect(page.getByRole("button", { name: "Review the referral for AHS-sr-1018" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Accept referral for AHS-sr-1018" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Accept referral for AHS-sr-1004" })).toBeVisible();
    await expect(page.locator("[data-honesty=designed]")).toHaveText("Referral inbox: designed, not built");
    await shot(page, "referrals");
    await noSeriousAxe(page);
  });

  test("inline Accept, Undo, conditions and Return", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await page.getByRole("button", { name: "Accept referral for AHS-sr-1004" }).click();
    await expect(pane.getByRole("status")).toContainText("Accepted. Your unit books her now.");
    await expect(page.getByRole("tab", { name: "Needs you 4" })).toBeVisible();
    await pane.getByRole("button", { name: "Undo" }).click();
    await expect(page.getByRole("tab", { name: "Needs you 5" })).toBeVisible();

    await page.getByRole("button", { name: /^AHS-sr-1011/ }).click();
    await expect(pane.getByTestId("flags")).toHaveCount(0);
    await pane.getByRole("button", { name: "Accept with conditions" }).click();
    await pane.getByRole("button", { name: "Arabic interpreter on the day" }).click();
    await pane.getByRole("button", { name: "Female endoscopist" }).click();
    await pane.getByRole("button", { name: "Accept with 2 conditions" }).click();
    await expect(pane.getByRole("status")).toContainText("Accepted with conditions. Arabic interpreter on the day; Female endoscopist.");

    await page.getByRole("button", { name: /^AHS-sr-1014/ }).click();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByText("Choose a reason to return it.")).toBeVisible();
    await pane.getByRole("button", { name: "Wrong unit" }).click();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByRole("status")).toContainText("Returned to the Al Mushrif GP. Reason: wrong unit. Act by 1 Sept.");
    await expect(page.getByRole("tab", { name: "Waiting on GP 1" })).toBeVisible();
    await shot(page, "referrals-returned");
  });

  test("the incomplete report shows grade chips", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await page.getByRole("button", { name: "Review the referral for Aisha Al Mansoori" }).click();
    await pane.getByRole("group", { name: "Boston grade" }).getByRole("button", { name: /^2\s*Mostly seen$/ }).click();
    await expect(pane.getByRole("status")).toContainText("Graded 2, mostly seen. The case stays held until the report is complete.");
  });

  test("a referral's avatar opens the patient record", async ({ page }) => {
    await page.goto("/app/referrals");
    await expect(page.getByRole("link", { name: "Open AHS-sr-1004's record" })).toHaveCount(0);
    await page.getByRole("button", { name: "Review the referral for Aisha Al Mansoori" }).click();
    await expect(page.getByRole("region", { name: "Open referral" }).getByRole("link", { name: "Open Aisha Al Mansoori's record" })).toBeVisible();
    await shot(page, "referrals-avatar-links");
    await noSeriousAxe(page);
    await page.getByRole("region", { name: "Referral list" }).getByRole("link", { name: "Open Aisha Al Mansoori's record" }).click();
    await expect(page).toHaveURL(aisha);
  });

  test("search is wired to q", async ({ page }) => {
    await page.goto("/app/referrals");
    await page.getByRole("searchbox", { name: "Search case id" }).fill("1009");
    await page.getByRole("searchbox", { name: "Search case id" }).press("Enter");
    await expect(page).toHaveURL(/\/app\/referrals\?q=1009$/);
    await expect(page.getByRole("tab", { name: "All 1" })).toBeVisible();
    await expect(page.getByRole("region", { name: "Open referral" }).getByRole("heading", { level: 2 })).toHaveText("Aisha Al Mansoori");
  });
});

/* ---------- Phone width (issue #4) ---------- */

const PHONE = { width: 390, height: 844 };

async function noSideScroll(page: Page) {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(PHONE.width);
}

/**
 * Every visible link, button, summary and input in `main` is at least 44px tall.
 * No exemptions: inline text links inside paragraphs were moved onto their own line.
 */
async function targetsAre44(page: Page) {
  const small = await page.locator("main").evaluate((main) =>
    [...main.querySelectorAll<HTMLElement>("a[href], button, summary, input:not([type=hidden])")]
      .map((el) => ({ el, r: el.getBoundingClientRect() }))
      .filter(({ el, r }) => r.width > 1 && r.height > 1 && getComputedStyle(el).visibility !== "hidden")
      .filter(({ r }) => r.height < 43.5)
      .map(({ el, r }) => `${el.tagName.toLowerCase()} "${(el.getAttribute("aria-label") ?? el.textContent ?? "").trim().slice(0, 40)}" ${Math.round(r.height)}px`),
  );
  expect(small).toEqual([]);
}

test.describe("Phone width, 390px", () => {
  test.use({ viewport: PHONE });

  test("Patients: cards, no side scroll, 44px targets", async ({ page }) => {
    await page.goto("/app/patients");
    await noSideScroll(page);
    await targetsAre44(page);
    await expect(page.locator("tbody tr")).toHaveCount(25);
    await page.goto("/app/patients?q=1009");
    const card = page.locator("tbody tr").first();
    await expect(card).toContainText("Aisha Al Mansoori");
    await expect(card).toContainText("Referred");
    await expect(card).toContainText("Due");
    await expect(card).toContainText("Owner");
    for (const word of ["Referred", "Due", "Owner"]) {
      const box = (await card.getByText(word).first().boundingBox())!;
      expect(box.x + box.width).toBeLessThanOrEqual(PHONE.width);
    }
    await shot(page, "patients-card-390");
    await page.goto("/app/patients");
    await shot(page, "patients-390");
    await noSeriousAxe(page);
  });

  test("Today: Call is in the first screen; the list takes its turn", async ({ page }) => {
    await page.goto("/app/today");
    await noSideScroll(page);
    const call = page.getByRole("button", { name: "Call AHS-sr-1004" });
    const box = (await call.boundingBox())!;
    expect(box.y + box.height).toBeLessThanOrEqual(PHONE.height);
    await targetsAre44(page);
    await shot(page, "today-390");
    await noSeriousAxe(page);

    await page.getByRole("button", { name: /Your list/ }).click();
    await expect(page.getByRole("region", { name: "Your list" })).toBeVisible();
    await expect(page.getByRole("region", { name: "Open patient" })).toBeHidden();
    await noSideScroll(page);
    await targetsAre44(page);
    await shot(page, "today-list-390");
    await noSeriousAxe(page);

    await page.getByRole("button", { name: /AHS-sr-1011/ }).click();
    await expect(page.getByRole("heading", { level: 2, name: "AHS-sr-1011" })).toBeFocused();
    await expect(page.getByRole("button", { name: "Call AHS-sr-1011" })).toBeVisible();
  });

  test("Today: after the call, the outcome buttons stay beside Call", async ({ page }) => {
    await page.goto("/app/today");
    await page.getByRole("button", { name: "Call AHS-sr-1004" }).click();
    await page.getByRole("button", { name: "Reached" }).click();
    await page.getByRole("button", { name: "Call back on…" }).click();
    await noSideScroll(page);
    await targetsAre44(page);
    await page.getByRole("button", { name: "Sun 30 Aug" }).click();
    await targetsAre44(page);
    await shot(page, "today-logged-390");
  });

  test("Referrals: stacked, Accept near the top, 44px targets", async ({ page }) => {
    await page.goto("/app/referrals");
    await noSideScroll(page);
    await targetsAre44(page);
    const accept = page.getByRole("region", { name: "Open referral" }).getByRole("button", { name: "Accept referral" });
    const box = (await accept.boundingBox())!;
    expect(box.y + box.height).toBeLessThanOrEqual(PHONE.height);
    await shot(page, "referrals-390");
    await noSeriousAxe(page);

    await page.getByRole("button", { name: /All referrals/ }).click();
    await expect(page.getByRole("region", { name: "Referral list" })).toBeVisible();
    await noSideScroll(page);
    await targetsAre44(page);
    await shot(page, "referrals-list-390");
    await noSeriousAxe(page);

    await page.getByRole("button", { name: "Review the referral for Aisha Al Mansoori" }).click();
    await expect(page.getByRole("heading", { level: 2, name: "Aisha Al Mansoori" })).toBeFocused();
    await noSideScroll(page);
    await targetsAre44(page);
    await shot(page, "referrals-grade-390");
  });

  test("keyboard focus is visible and nothing animates under reduced motion", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/app/today");
    const moving = await page.locator("main").evaluate((main) =>
      [...main.querySelectorAll<HTMLElement>("*")].filter((el) => getComputedStyle(el).animationName !== "none" && parseFloat(getComputedStyle(el).animationDuration) > 0).length,
    );
    expect(moving).toBe(0);
    await page.getByRole("button", { name: "Call AHS-sr-1004" }).focus();
    const outline = await page.evaluate(() => {
      const s = getComputedStyle(document.activeElement!);
      return { w: s.outlineWidth, style: s.outlineStyle };
    });
    expect(outline).toEqual({ w: "2px", style: "solid" });
  });
});

test.describe("Patients: paging and filters", () => {
  test("25 per page; Next and Previous keep the filters", async ({ page }) => {
    await page.goto("/app/patients");
    await expect(page.getByTestId("range")).toHaveText("Showing 1–25 of 178");
    await expect(page.locator("tbody tr")).toHaveCount(25);
    await expect(page.getByRole("link", { name: "Previous" })).toHaveCount(0);
    await page.getByRole("navigation", { name: "Pages" }).getByRole("link", { name: "Next" }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page.getByTestId("range")).toHaveText("Showing 26–50 of 178");

    await page.goto("/app/patients?programme=crc&page=2");
    await expect(page.getByTestId("showing")).toContainText("colorectal screening");
    await page.getByRole("link", { name: "Previous" }).click();
    await expect(page).toHaveURL(/programme=crc/);
    await expect(page).not.toHaveURL(/page=/);
  });

  test("a page past the end lands on the last page", async ({ page }) => {
    await page.goto("/app/patients?page=99");
    await expect(page).toHaveURL(/page=8$/);
    await expect(page.getByTestId("range")).toHaveText("Showing 176–178 of 178");
  });

  test("programme filter is its own param and survives the chips", async ({ page }) => {
    await page.goto("/app/patients?programme=crc");
    await page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Overdue" }).click();
    await expect(page).toHaveURL(/programme=crc/);
    await expect(page).toHaveURL(/overdue=true/);
  });
});

test.describe("Wide screens", () => {
  for (const route of ["/app/patients", "/app/today", "/app/referrals"]) {
    test(`${route} has no serious accessibility violations at 1440px`, async ({ page }) => {
      await page.goto(route);
      await noSeriousAxe(page);
      await shot(page, `${route.split("/").pop()}-1440`);
    });
  }
});
