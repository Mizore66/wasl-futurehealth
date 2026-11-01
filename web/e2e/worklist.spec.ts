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

const fatima = `/app/patients/${encodeURIComponent("ahs-mushrif/ServiceRequest/AHS-sr-1007")}`;

test.describe("Today", () => {
  test("finds the most urgent patient and opens them", async ({ page }) => {
    await page.goto("/app/today");
    await expect(page.getByTestId("finding")).toHaveText("AHS-sr-1020 is 88 days over 31: not booked. 9 more to call.");
    await expect(page.getByRole("tab", { name: "To call 10" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tab", { name: "Waiting 5" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "No owner 1" })).toBeVisible();
    const open = page.getByRole("region", { name: "Open patient" });
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1020");
    await expect(open.getByTestId("lead-days")).toHaveAttribute("data-value", "119");
    await expect(page.locator("[data-honesty=designed]")).toHaveText("Call log: designed, not built");
    await shot(page, "today");
    await noSeriousAxe(page);
  });

  test("logging No answer keeps the patient open, offers Undo and the next patient", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "No answer" }).click();
    const status = open.getByRole("status");
    await expect(status).toContainText("Logged: No answer · AHS-sr-1020 moved to the end · Undo");
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1020");
    await expect(open.getByText("Called, no answer")).toBeVisible();
    await expect(page.getByRole("button", { name: /AHS-sr-1020/ }).first()).toContainText("No answer ×1 · retry tomorrow");
    await shot(page, "today-logged");

    await status.getByRole("button", { name: "Undo" }).click();
    await expect(open.getByRole("status")).toHaveCount(0);
    await expect(open.getByText("Called, no answer")).toHaveCount(0);
    await expect(open.getByRole("group", { name: "After the call" })).toBeVisible();

    await open.getByRole("button", { name: "No answer" }).click();
    await open.getByRole("button", { name: "Next patient: AHS-sr-1014" }).click();
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1014");
  });

  test("Call, then Reached offers Book and Call back on…", async ({ page }) => {
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "Call AHS-sr-1020" }).click();
    await expect(open.getByRole("button", { name: "On the call · AHS-sr-1020" })).toBeVisible();
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
    await expect(open.getByRole("status")).toContainText("Logged: Booked · AHS-sr-1020 moved to Waiting");
    await expect(page.getByRole("tab", { name: "To call 9" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Waiting 6" })).toBeVisible();

    await page.getByRole("tab", { name: "No owner 1" }).click();
    await open.getByRole("button", { name: "Take this patient" }).click();
    await expect(page.getByRole("tab", { name: "To call 10" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "No owner 0" })).toBeVisible();
  });

  test("search narrows the list; Waiting opens Aisha's record", async ({ page }) => {
    await page.goto("/app/today");
    await page.getByRole("searchbox").fill("1014");
    await expect(page.getByRole("region", { name: "Your list" }).getByRole("button", { pressed: true })).toContainText("AHS-sr-1014");
    await page.getByRole("searchbox").fill("");
    await page.getByRole("tab", { name: "Waiting 5" }).click();
    await page.getByRole("button", { name: /Aisha Al Mansoori/ }).click();
    await page.getByRole("link", { name: "Open record" }).click();
    await expect(page).toHaveURL(/AHS-sr-1021$/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Aisha Al Mansoori");
  });
});

test.describe("Patients", () => {
  test("filters from the query string, as Programme's stall links send them", async ({ page }) => {
    await page.goto("/app/patients?dropoff=never_notified&overdue=true");
    await expect(page.getByTestId("showing")).toHaveText("3 cases: never notified · past 31 days");
    await expect(page.locator("tbody tr")).toHaveCount(3);
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
    await expect(page.getByTestId("showing")).toHaveText("26 cases: all cases");
    await shot(page, "patients");
    await page.getByRole("searchbox", { name: "Search case id" }).fill("1007");
    await page.getByRole("button", { name: "Search" }).click();
    await expect(page).toHaveURL(/q=1007/);
    await page.getByRole("link", { name: "Fatima Al Zaabi" }).click();
    await expect(page).toHaveURL(fatima);
  });
});

test.describe("Patient record", () => {
  test("Fatima: two names bridged, evidence quoted, chain verified", async ({ page }) => {
    await page.goto(fatima);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Fatima Al Zaabi");
    await expect(page.getByText("Closed day 26")).toBeVisible();
    await expect(page.getByTestId("closed-in")).toHaveText("Closed in 26 days, 5 inside the 31-day standard");
    const chain = page.getByRole("list", { name: "Sealed chain" });
    await expect(chain.getByRole("listitem").last()).toContainText("Closed · with evidence, dated to the colonoscopy");
    await expect(page.getByText("فاطمة الزعابي").first()).toBeVisible();
    await expect(page.getByText("Fatima Alzaabi")).toBeVisible();
    await expect(page.getByText("“The preparation was excellent and all surfaces were well seen”")).toBeVisible();
    await expect(page.getByTestId("chain-verified")).toContainText("Verified · 3 of 3 intact");
    await expect(page.getByText("Message owner")).toHaveCount(0);

    await page.getByText("3 sealed entries").click();
    await expect(page.getByRole("list", { name: "Entry hashes" }).getByRole("listitem")).toHaveText([/Report matched/, /Readers' result/, /Closed/]);
    await shot(page, "record-fatima");
    await noSeriousAxe(page);
  });

  test("Aisha: held, readers' fields not stated", async ({ page }) => {
    await page.goto(`/app/patients/${encodeURIComponent("ahs-mushrif/ServiceRequest/AHS-sr-1021")}`);
    await expect(page.getByText("Safety gate: held").first()).toBeVisible();
    await expect(page.getByText("The report states none of these in words.")).toBeVisible();
    await shot(page, "record-aisha");
  });
});

test.describe("Referrals", () => {
  test("opens on Needs you; flagged rows show Review, clean rows Accept", async ({ page }) => {
    await page.goto("/app/referrals");
    await expect(page.getByRole("tab", { name: "Needs you 5" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByTestId("finding")).toHaveText("AHS-sr-1020 has waited 119 days, 112 past the unit's 7. AHS-sr-1023's report needs your review.");
    await expect(page.getByRole("button", { name: "Review the referral for AHS-sr-1000" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Accept referral for AHS-sr-1000" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Accept referral for AHS-sr-1005" })).toBeVisible();
    await expect(page.locator("[data-honesty=designed]")).toHaveText("Referral inbox: designed, not built");
    await shot(page, "referrals");
    await noSeriousAxe(page);
  });

  test("inline Accept, Undo, conditions and Return", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await page.getByRole("button", { name: "Accept referral for AHS-sr-1005" }).click();
    await expect(pane.getByRole("status")).toContainText("Accepted. Your unit books her now.");
    await expect(page.getByRole("tab", { name: "Needs you 4" })).toBeVisible();
    await pane.getByRole("button", { name: "Undo" }).click();
    await expect(page.getByRole("tab", { name: "Needs you 5" })).toBeVisible();

    await page.getByRole("button", { name: "Review the referral for AHS-sr-1000" }).click();
    await expect(pane.getByTestId("flags")).toHaveText("Check first: No owner at the clinic");
    await pane.getByRole("button", { name: "Accept with conditions" }).click();
    await pane.getByRole("button", { name: "Arabic interpreter on the day" }).click();
    await pane.getByRole("button", { name: "Female endoscopist" }).click();
    await pane.getByRole("button", { name: "Accept with 2 conditions" }).click();
    await expect(pane.getByRole("status")).toContainText("Accepted with conditions. Arabic interpreter on the day; Female endoscopist.");

    await page.getByRole("button", { name: /^AHS-sr-1020/ }).click();
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
    await pane.getByRole("button", { name: "Boston grade 2, mostly seen" }).click();
    await expect(pane.getByRole("status")).toContainText("Graded 2, mostly seen. The case stays held until the report is complete.");
  });

  test("search is wired to q", async ({ page }) => {
    await page.goto("/app/referrals");
    await page.getByRole("searchbox", { name: "Search case id" }).fill("1021");
    await page.getByRole("searchbox", { name: "Search case id" }).press("Enter");
    await expect(page).toHaveURL(/\/app\/referrals\?q=1021$/);
    await expect(page.getByRole("tab", { name: "All 1" })).toBeVisible();
    await expect(page.getByRole("region", { name: "Open referral" }).getByRole("heading", { level: 2 })).toHaveText("Aisha Al Mansoori");
  });
});
