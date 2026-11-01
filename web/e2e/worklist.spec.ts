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
    await expect(page.getByTestId("showing")).toHaveText("128 cases: all cases");
    await shot(page, "patients");
    await page.getByRole("searchbox", { name: "Search case id" }).fill("1037");
    await page.getByRole("button", { name: "Search" }).click();
    await expect(page).toHaveURL(/q=1037/);
    await page.getByRole("link", { name: "Fatima Al Zaabi" }).click();
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
    await pane.getByRole("button", { name: "Boston grade 2, mostly seen" }).click();
    await expect(pane.getByRole("status")).toContainText("Graded 2, mostly seen. The case stays held until the report is complete.");
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
