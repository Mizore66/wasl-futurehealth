import AxeBuilder from "@axe-core/playwright";
import { expect as baseExpect, test, type Page } from "@playwright/test";

// Six dev servers share this machine during phase 2; first compiles of a route can be slow.
const expect = baseExpect.configure({ timeout: 20_000 });
// A missing name or case must fail within seconds, not after the 2-minute test budget: actions (click, fill) get the same 20s as expect.
test.describe.configure({ timeout: 60_000 });
test.use({ actionTimeout: 20_000, navigationTimeout: 30_000 });

const shot = (page: Page, name: string) => page.screenshot({ path: `test-results/screens/worklist-${name}.png`, fullPage: true });

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

const aisha = /\/app\/patients\/ahs-mushrif(%2F|\/)ServiceRequest(%2F|\/)AHS-sr-1011$/;
const fatima = `/app/patients/${encodeURIComponent("ahs-mushrif/ServiceRequest/AHS-sr-1001")}`;

test.describe("Today", () => {
  const sheet = (page: Page) => page.getByTestId("today-sheet");
  const open = (page: Page) => page.getByRole("region", { name: "Selected case" });
  const rows = (page: Page) => sheet(page).locator("button[data-case-row]");
  const row = (page: Page, id: string) => sheet(page).locator(`button[data-case-row="${id}"]`);
  const queueLink = (page: Page, name: RegExp) => page.getByRole("navigation", { name: "Main", exact: true }).getByRole("link", { name });

  test("opens To call on the most urgent case: the task is the heading, one primary action, contact shown", async ({ page }) => {
    await page.goto("/app/today");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Today");
    await expect(page.getByText(/Colorectal screening · Al Mushrif · run day 120/)).toBeVisible();
    await expect(queueLink(page, /^To call/)).toHaveAttribute("aria-current", "page");
    await expect(queueLink(page, /^To call/)).toContainText("8");
    await expect(queueLink(page, /^Waiting/)).toContainText("6");
    await expect(queueLink(page, /^No owner/)).toContainText("2");
    await expect(sheet(page).getByRole("heading", { level: 2, name: /^To call/ })).toBeVisible();
    await expect(rows(page)).toHaveCount(8);
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1002");
    await expect(open(page).getByRole("heading", { level: 2 })).toHaveText("Receipt outstanding");
    await expect(open(page).getByText("Notified, hasn't confirmed. Call to confirm they received the result.")).toBeVisible();
    await expect(open(page).getByTestId("lead-days")).toHaveAttribute("data-value", "108");
    await expect(open(page).getByTestId("contact-status")).toHaveText("Contact permitted");
    await expect(open(page).locator("[data-variant=cta]")).toHaveCount(1);
    await expect(open(page).locator("[data-variant=cta]")).toHaveText("Log call · AHS-sr-1002");
    await expect(page.locator("[data-honesty=illustrative]")).toHaveText("Call log: kept in this browser (no live API)");
    await shot(page, "today");
    await noSeriousAxe(page);
  });

  test("the case-details icon shows owner, contact permission and ids; the folder is unavailable without a record", async ({ page }) => {
    await page.goto("/app/today");
    const details = open(page).getByRole("button", { name: "Case details" });
    await expect(details).toHaveAttribute("aria-expanded", "false");
    await expect(open(page).getByTestId("case-facts")).toBeHidden();
    await details.click();
    await expect(details).toHaveAttribute("aria-expanded", "true");
    const facts = open(page).getByTestId("case-facts");
    await expect(facts).toContainText("OwnerNurse 6");
    await expect(facts).toContainText("Contact permissionGranted");
    await expect(facts).toContainText("CaseAHS-sr-1002");
    await expect(facts).toContainText("Referred12 May");
    await expect(open(page).getByRole("button", { name: "Full record not available for this case" })).toBeDisabled();
  });

  test("logging No answer keeps the case open, offers Undo and the next case", async ({ page }) => {
    await page.goto("/app/today");
    await open(page).getByRole("button", { name: "No answer" }).click();
    const status = open(page).getByRole("status");
    await expect(status).toContainText("Logged: No answer · AHS-sr-1002 moved to the end · Undo");
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1002");
    await expect(page.getByRole("region", { name: "Case history" }).getByText("Called, no answer")).toBeVisible();
    await expect(row(page, "AHS-sr-1002")).toContainText("No answer ×1 · retry tomorrow");
    await shot(page, "today-logged");

    await status.getByRole("button", { name: "Undo" }).click();
    await expect(open(page).getByRole("status")).toHaveCount(0);
    await expect(page.getByText("Called, no answer")).toHaveCount(0);
    await expect(open(page).getByRole("group", { name: "After the call" })).toBeVisible();

    await open(page).getByRole("button", { name: "No answer" }).click();
    await open(page).getByRole("button", { name: "Next case: AHS-sr-1023" }).click();
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1023");
  });

  test("Log call, then Reached offers Appointment agreed and Call back on…", async ({ page }) => {
    // The wall clock is on another date than the run clock (late August), one step before a UTC midnight: the
    // call-back days are the dates of the wall-clock `until` that is saved, never the run's days (issue #37).
    await page.clock.setFixedTime(new Date("2026-11-10T23:59:30Z"));
    await page.goto("/app/today");
    await open(page).getByRole("button", { name: "Log call · AHS-sr-1002" }).click();
    await expect(open(page).getByRole("button", { name: "Logging call · AHS-sr-1002" })).toBeVisible();
    await expect(open(page).getByRole("group", { name: "How did it go?" })).toBeVisible();

    await open(page).getByRole("button", { name: "Reached" }).click();
    const next = open(page).getByRole("group", { name: "What next?" });
    await expect(next.getByRole("button")).toHaveText(["Appointment agreed", "Call back on…", "Done elsewhere…", "Declined"]);
    await expect(open(page).getByRole("button", { name: /^Next case/ })).toHaveCount(0);

    await next.getByRole("button", { name: "Call back on…" }).click();
    const days = open(page).getByRole("group", { name: "Call back on" });
    await expect(days.getByRole("button")).toHaveText(["Wed 11 Nov", "Thu 12 Nov", "Fri 13 Nov"]);
    await days.getByRole("button", { name: "Wed 11 Nov" }).click();
    await expect(open(page).getByRole("status")).toContainText("Logged: Call back Wed 11 Nov");
    await expect(open(page).getByRole("button", { name: /^Next case/ })).toBeVisible();
    await shot(page, "today-callback");
  });

  // user-flows.html "Coordinate and verify a callback", Today steps, with the fixture case at the top of To call.
  test("Log call → Reached → Appointment agreed moves the case to Waiting; the sidebar counts follow", async ({ page }) => {
    await page.goto("/app/today");
    await open(page).getByRole("button", { name: "Log call · AHS-sr-1002" }).click();
    await open(page).getByRole("button", { name: "Reached" }).click();
    await open(page).getByRole("button", { name: "Appointment agreed" }).click();
    // The case stays open with the outcome; "Open Waiting" follows it to its new queue.
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1002");
    await expect(open(page).getByRole("status")).toContainText("Logged: Appointment agreed · AHS-sr-1002 moved to Waiting");
    await expect(queueLink(page, /^To call/)).toContainText("7");
    await expect(queueLink(page, /^Waiting/)).toContainText("7");
    await open(page).getByRole("button", { name: "Open Waiting" }).click();
    await expect(page).toHaveURL(/queue=waiting/);
    await expect(queueLink(page, /^Waiting/)).toHaveAttribute("aria-current", "page");
    await expect(sheet(page).getByRole("heading", { level: 2, name: /^Waiting/ })).toBeVisible();
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1002");
    await expect(row(page, "AHS-sr-1002")).toContainText("Booking needed");
    await expect(row(page, "AHS-sr-1002")).toHaveAttribute("aria-expanded", "true");
    const history = page.getByRole("region", { name: "Case history" });
    await expect(history.getByRole("button", { name: /^Referral issued/ })).toBeVisible();
    await history.getByRole("button", { name: /^Appointment agreed/ }).click();
    await expect(history.getByText("Recorded by This browser")).toBeVisible();

    // The sidebar's queue links are soft navigations: the logged action survives switching queues.
    await queueLink(page, /^To call/).click();
    await expect(page).toHaveURL(/queue=call/);
    await expect(rows(page)).toHaveCount(7);
    await queueLink(page, /^Waiting/).click();
    await expect(row(page, "AHS-sr-1002")).toContainText("Booking needed");
  });

  // user-flows.html "Take ownership with contact on hold": no fixture case is both unowned and on hold, so the
  // unowned fixture case plays the role; the app's rule moves a taken case to To call, not Waiting.
  test("No owner: taking a case makes it yours and moves it to To call", async ({ page }) => {
    await page.goto("/app/today?queue=none");
    await expect(sheet(page).getByRole("heading", { level: 2, name: /^No owner/ })).toBeVisible();
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1017");
    await expect(open(page).getByTestId("contact-status")).toHaveText("Contact permitted");
    await open(page).getByRole("button", { name: "Take this patient" }).click();
    await expect(open(page).getByRole("status")).toContainText("Logged: Yours now · AHS-sr-1017 moved to To call");
    await expect(queueLink(page, /^No owner/)).toContainText("1");
    await expect(queueLink(page, /^To call/)).toContainText("9");
    await open(page).getByRole("button", { name: "Case details" }).click();
    await expect(open(page).getByTestId("case-facts")).toContainText("OwnerHessa Al Ali");
  });

  // Issue #36: nothing is booked and no dialer is connected, so no step of the workflow may say so.
  for (const width of [1440, 390]) {
    test(`no step says Book, Booked or On the call (${width}px)`, async ({ page }) => {
      await page.setViewportSize({ width, height: 900 });
      await page.goto("/app/today");
      const seen: string[] = [];
      const look = async () => {
        seen.push(await page.locator("main").innerText());
        const labels = await page.getByRole("button").evaluateAll((buttons) => buttons.map((b) => b.getAttribute("aria-label") ?? b.textContent ?? ""));
        seen.push(...labels);
      };
      await look();
      await open(page).getByRole("button", { name: "Log call · AHS-sr-1002" }).click();
      await expect(open(page).getByRole("button", { name: "Logging call · AHS-sr-1002" })).toBeVisible();
      await look();
      await open(page).getByRole("button", { name: "Reached" }).click();
      await look();
      await open(page).getByRole("button", { name: "Appointment agreed" }).click();
      await look();
      await open(page).getByRole("button", { name: "Open Waiting" }).click();
      await look();
      const text = seen.join("\n");
      expect(text).toContain("Booking needed · date to confirm");
      expect(text).not.toMatch(/\bBook(ed)?\b/);
      expect(text).not.toContain("On the call");
    });
  }

  // user-flows.html "Search without acting on the wrong case".
  test("search by id selects that row; no match clears the case actions; clearing restores the list", async ({ page }) => {
    await page.goto("/app/today");
    await page.locator("body").press("/");
    const search = page.getByRole("searchbox", { name: "Find a case or task" });
    await expect(search).toBeFocused();
    await search.fill("1002");
    await expect(rows(page)).toHaveCount(1);
    await expect(row(page, "AHS-sr-1002")).toHaveAttribute("aria-expanded", "true");
    await expect(open(page).getByRole("heading", { level: 2 })).toHaveText("Receipt outstanding");
    await search.fill("99999");
    await expect(rows(page)).toHaveCount(0);
    await expect(open(page)).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Log call/ })).toHaveCount(0);
    await expect(sheet(page).getByText("No case matches this search.")).toBeVisible();
    await search.fill("");
    await expect(rows(page)).toHaveCount(8);
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1002");
  });

  test("the folder icon opens the full record and Audit trail its audit", async ({ page }) => {
    await page.goto("/app/today?queue=waiting");
    await row(page, "AHS-sr-1011").click();
    await expect(open(page).getByTestId("case-who")).toContainText("Aisha Al Mansoori");
    await expect(page.getByRole("region", { name: "Case history" }).getByRole("link", { name: "Audit trail" })).toHaveAttribute("href", /AHS-sr-1011\/audit$/);
    await open(page).getByRole("link", { name: "Open full record" }).click();
    await expect(page).toHaveURL(aisha);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Aisha Al Mansoori");
  });

  test("phone: the selected case opens inline in its row; tapping the row folds it", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/app/today");
    const first = row(page, "AHS-sr-1002");
    await expect(first).toHaveAttribute("aria-expanded", "true");
    const [rowBox, caseBox, nextBox] = await Promise.all([first.boundingBox(), open(page).boundingBox(), row(page, "AHS-sr-1023").boundingBox()]);
    expect(caseBox!.y).toBeGreaterThan(rowBox!.y);
    expect(nextBox!.y).toBeGreaterThan(caseBox!.y + caseBox!.height - 1);
    await expect(open(page).getByRole("button", { name: "Log call · AHS-sr-1002" })).toBeVisible();
    const activity = page.getByRole("button", { name: "Activity" });
    await expect(activity).toHaveAttribute("aria-expanded", "false");
    await activity.click();
    await expect(page.getByRole("region", { name: "Case history" }).getByText("Referral issued")).toBeVisible();
    await shot(page, "today-phone");
    await noSeriousAxe(page);
    await first.click();
    await expect(first).toHaveAttribute("aria-expanded", "false");
    await expect(open(page)).toBeHidden();
    await row(page, "AHS-sr-1023").click();
    await expect(open(page)).toHaveAttribute("data-case", "AHS-sr-1023");
    await expect(open(page)).toBeVisible();
  });
});

test.describe("Patients", () => {
  test("filters from the query string, as Programme's stall links send them", async ({ page }) => {
    await page.goto("/app/patients?dropoff=never_notified&overdue=true");
    await expect(page.getByTestId("showing")).toHaveText("7 cases: never notified · overdue");
    await expect(page.locator("tbody tr")).toHaveCount(7);
    await expect(page.locator("[data-honesty]").first()).toBeVisible();
    await shot(page, "patients-filtered");
    await noSeriousAxe(page);

    await page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Closed" }).click();
    await expect(page).toHaveURL(/closed=true/);
    await expect(page.getByTestId("showing")).toContainText("cases: closed");
    await expect(page.locator("tbody tr").first()).toContainText("Closed day");
  });

  test("search by case id leads to the record", async ({ page }) => {
    await page.goto("/app/patients");
    await expect(page.getByTestId("showing")).toHaveText("177 cases: all cases");
    await shot(page, "patients");
    await page.getByRole("searchbox", { name: "Search case id" }).fill("1001");
    await page.getByRole("button", { name: "Search" }).click();
    await expect(page).toHaveURL(/q=1001/);
    await page.getByRole("link", { name: "Fatima Al Zaabi" }).click();
    await expect(page).toHaveURL(fatima);
  });

  test("the avatar is part of the record link", async ({ page }) => {
    await page.goto("/app/patients?q=1001");
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
    await expect(page.getByText("“the cecum, which was identified by the presence of the appendiceal orifice and ileocecal valve”")).toBeVisible();
    await expect(page.getByTestId("chain-verified")).toContainText("Verified · 3 of 3 intact");
    await expect(page.getByText("Message owner")).toHaveCount(0);

    await page.getByText("3 sealed entries").click();
    await expect(page.getByRole("list", { name: "Entry hashes" }).getByRole("listitem")).toHaveText([/Report matched/, /Readers' result/, /Closed/]);
    await shot(page, "record-fatima");
    await noSeriousAxe(page);
  });

  test("Aisha: held, the caecum was reached but no prep grade is stated", async ({ page }) => {
    await page.goto(`/app/patients/${encodeURIComponent("ahs-mushrif/ServiceRequest/AHS-sr-1011")}`);
    await expect(page.getByText("Safety gate: held").first()).toBeVisible();
    await expect(page.getByText("“until it reached the cecum”")).toBeVisible();
    await expect(page.getByRole("region", { name: "DoH closure checklist" }).getByRole("listitem")).toHaveText([/Caecum reached\s*Yes/, /Prep graded\s*Not stated/, /Histology, if tissue taken\s*Not needed: no tissue taken/]);
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
  test("opens on Needs you; the incomplete report shows Review and an unanswered referral shows Accept", async ({ page }) => {
    await page.goto("/app/referrals");
    await expect(page.getByRole("tab", { name: "Needs you 4" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("tab", { name: "All 21" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Accepted 16" })).toBeVisible();
    await expect(page.getByTestId("finding")).toHaveText("Aisha Al Mansoori's report needs your review.");
    await expect(page.getByRole("button", { name: "Review the referral for Aisha Al Mansoori" })).toBeVisible();
    await expect(page.getByRole("button", { name: /^Accept referral for/ })).toHaveCount(3);
    await expect(page.locator("[data-honesty=designed]")).toHaveText("Referral inbox: designed, not built");
    await shot(page, "referrals");
    await noSeriousAxe(page);
  });

  test("inline Accept, Undo, conditions and Return", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    // A row-level Accept opens the referral and asks once more; nothing is accepted until Confirm.
    await page.getByRole("button", { name: "Accept referral for AHS-sr-9001" }).click();
    await expect(pane.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-9001");
    const confirm = page.getByRole("button", { name: "Confirm accepting the referral for AHS-sr-9001" });
    await expect(confirm).toBeFocused();
    await expect(page.getByRole("tab", { name: "Needs you 4" })).toBeVisible();
    await page.getByRole("button", { name: "Do not accept the referral for AHS-sr-9001 yet" }).click();
    await expect(pane.getByText("Accepted. Your unit books her now.")).toHaveCount(0);
    await expect(page.getByRole("tab", { name: "Needs you 4" })).toBeVisible();
    await page.getByRole("button", { name: "Accept referral for AHS-sr-9001" }).click();
    await confirm.click();
    await expect(pane.getByRole("status")).toContainText("Accepted. Your unit books her now.");
    await expect(page.getByRole("tab", { name: "Needs you 3" })).toBeVisible();
    await pane.getByRole("button", { name: "Undo" }).click();
    await expect(page.getByRole("tab", { name: "Needs you 4" })).toBeVisible();

    await page.getByRole("button", { name: /^AHS-sr-9002/ }).click();
    await expect(pane.getByTestId("flags")).toHaveCount(0);
    await pane.getByRole("button", { name: "Accept with conditions" }).click();
    await pane.getByRole("button", { name: "Arabic interpreter on the day" }).click();
    await pane.getByRole("button", { name: "Female endoscopist" }).click();
    await pane.getByRole("button", { name: "Accept with 2 conditions" }).click();
    await expect(pane.getByRole("status")).toContainText("Accepted with conditions. Arabic interpreter on the day; Female endoscopist.");
  });

  test("Return to GP clocks the GP from the return day and Undo clears it", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await page.getByRole("button", { name: /^AHS-sr-9003/ }).click();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByText("Choose a reason to return it.")).toBeVisible();
    await expect(pane.getByText("who must act by 1 Sep.")).toBeVisible();
    await pane.getByRole("button", { name: "Wrong unit" }).click();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByRole("status")).toContainText("Returned to the Al Mushrif GP. Reason: wrong unit. Act by 1 Sep.");
    const chain = pane.getByRole("list", { name: "Sealed chain" });
    await expect(chain.getByRole("listitem").last()).toContainText("Act on the return");
    await expect(chain.getByRole("listitem").last()).toContainText("by 1 Sep");
    await expect(page.getByRole("tab", { name: "Waiting on GP 2" })).toBeVisible();
    await shot(page, "referrals-returned");
    await pane.getByRole("button", { name: "Undo" }).click();
    await expect(page.getByRole("tab", { name: "Waiting on GP 1" })).toBeVisible();
    await expect(chain).not.toContainText("by 1 Sep");
  });

  test("an existing return without a hand-off says its deadline is unavailable", async ({ page }) => {
    await page.goto("/app/referrals");
    await page.getByRole("tab", { name: "Waiting on GP 1" }).click();
    const pane = page.getByRole("region", { name: "Open referral" });
    await expect(pane.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-9004");
    const last = pane.getByRole("list", { name: "Sealed chain" }).getByRole("listitem").last();
    await expect(last).toContainText("Act on the return");
    await expect(last).toContainText("deadline unavailable");
    await expect(last).not.toContainText(/by d/);
  });

  for (const [grade, words] of [[0, "Not seen"], [1, "Partly seen"], [2, "Mostly seen"], [3, "All seen"]] as const) {
    test(`prep grade ${grade} stays held, retains history across search, and undoes`, async ({ page }) => {
      await page.goto("/app/referrals");
      const pane = page.getByRole("region", { name: "Open referral" });
      await page.getByRole("button", { name: "Review the referral for Aisha Al Mansoori" }).click();
      await pane.getByRole("group", { name: "Boston grade" }).getByRole("button", { name: new RegExp(`^${grade}\\s*${words}$`) }).click();
      await expect(pane.getByRole("status")).toContainText(`Graded ${grade}, ${words.toLowerCase()}. The case stays held until the report is complete.`);
      await expect(pane.getByRole("button", { name: "Undo" })).toBeFocused();
      await expect(page.getByRole("tab", { name: "Needs you 4" })).toBeVisible();
      await expect(page.getByRole("tab", { name: "Accepted 16" })).toBeVisible();
      const chain = pane.getByRole("list", { name: "Sealed chain" });
      await expect(chain).toContainText(`Graded the prep ${grade}`);
      await expect(chain.getByRole("listitem").last()).toContainText("Complete the report");
      await expect(chain).not.toContainText("Book and do the colonoscopy");
      await search(page, "1011");
      await expect(pane.getByRole("status")).toContainText(`Graded ${grade},`);
      await search(page, "no-such-case");
      await expect(pane.getByRole("heading", { name: "Nothing to open" })).toBeVisible();
      await search(page, "");
      await page.getByRole("button", { name: /^Aisha Al Mansoori/ }).click();
      await expect(chain).toContainText(`Graded the prep ${grade}`);
      await expect(page.getByRole("tab", { name: "Needs you 4" })).toBeVisible();
      await expect(page.getByRole("tab", { name: "Accepted 16" })).toBeVisible();
      await pane.getByRole("button", { name: "Undo" }).click();
      await expect(pane.getByRole("heading", { level: 2 })).toBeFocused();
      await expect(chain).not.toContainText("Graded the prep");
      await expect(pane.getByRole("group", { name: "Boston grade" })).toBeVisible();
      await expect(page.getByRole("tab", { name: "Needs you 4" })).toBeVisible();
      await expect(page.getByRole("tab", { name: "Accepted 16" })).toBeVisible();
    });
  }

  test("a referral's avatar opens the patient record", async ({ page }) => {
    await page.goto("/app/referrals");
    // A referral with no record shows a plain avatar: AHS-sr-1010 is booked, and only the three demo patients have a record.
    await page.getByRole("tab", { name: "All 21" }).click();
    await expect(page.getByRole("button", { name: /^AHS-sr-1010/ })).toBeVisible();
    await expect(page.getByRole("link", { name: "Open AHS-sr-1010's record" })).toHaveCount(0);
    await page.getByRole("button", { name: "Review the referral for Aisha Al Mansoori" }).click();
    await expect(page.getByRole("region", { name: "Open referral" }).getByRole("link", { name: "Open Aisha Al Mansoori's record" })).toBeVisible();
    await shot(page, "referrals-avatar-links");
    await noSeriousAxe(page);
    await page.getByRole("region", { name: "Referral list" }).getByRole("link", { name: "Open Aisha Al Mansoori's record" }).click();
    await expect(page).toHaveURL(aisha);
  });

  /** A row-level Accept opens the referral and asks once more; Confirm commits it. */
  async function acceptFromRow(page: Page, id: string) {
    await page.getByRole("button", { name: `Accept referral for ${id}` }).click();
    await page.getByRole("button", { name: `Confirm accepting the referral for ${id}` }).click();
  }

  async function search(page: Page, text: string) {
    const box = page.getByRole("searchbox", { name: "Search case id" });
    await box.fill(text);
    await box.press("Enter");
    await expect(page).toHaveURL(text ? `/app/referrals?q=${text}` : /\/app\/referrals$/);
    await expect(box).toBeFocused();
  }

  test("searching and clearing keep an Accept, its history and Undo", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await acceptFromRow(page, "AHS-sr-9001");
    await expect(page.getByRole("tab", { name: "Accepted 17" })).toBeVisible();
    await search(page, "9001");
    await expect(pane.getByRole("status")).toContainText("Accepted. Your unit books her now.");
    await expect(pane.getByRole("list", { name: "Sealed chain" })).toContainText("Accepted the referral");
    await search(page, "");
    await expect(page.getByRole("tab", { name: "Accepted 17" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Needs you 3" })).toBeVisible();
    await page.getByRole("tab", { name: "All 21" }).click();
    await page.getByRole("button", { name: /^AHS-sr-9001/ }).click();
    await pane.getByRole("button", { name: "Undo" }).click();
    await expect(page.getByRole("tab", { name: "Needs you 4" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Accepted 16" })).toBeVisible();
  });

  test("a search that hides several decisions gives every one back when cleared", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await acceptFromRow(page, "AHS-sr-9001");
    await page.getByRole("button", { name: /^AHS-sr-9002/ }).click();
    await pane.getByRole("button", { name: "Accept with conditions" }).click();
    await pane.getByRole("button", { name: "Female endoscopist" }).click();
    await pane.getByRole("button", { name: "Accept with 1 condition" }).click();
    await page.getByRole("button", { name: /^AHS-sr-9003/ }).click();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await pane.getByRole("button", { name: "Wrong unit" }).click();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await search(page, "9004");
    await expect(page.getByRole("tab", { name: "All 1" })).toBeVisible();
    await search(page, "");
    await expect(page.getByRole("tab", { name: "Accepted 18" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Waiting on GP 2" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Needs you 1" })).toBeVisible();
    await page.getByRole("tab", { name: "All 21" }).click();
    await page.getByRole("button", { name: /^AHS-sr-9002/ }).click();
    await expect(pane.getByRole("status")).toContainText("Accepted with conditions. Female endoscopist.");
    await page.getByRole("button", { name: /^AHS-sr-9003/ }).click();
    await expect(pane.getByRole("status")).toContainText("Reason: wrong unit. Act by 1 Sep.");
    await pane.getByRole("button", { name: "Undo" }).click();
    await expect(page.getByRole("tab", { name: "Waiting on GP 1" })).toBeVisible();
  });

  test("a recorded prep grade survives searching for the case and clearing", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await page.getByRole("button", { name: "Review the referral for Aisha Al Mansoori" }).click();
    await pane.getByRole("group", { name: "Boston grade" }).getByRole("button", { name: /^2\s*Mostly seen$/ }).click();
    await search(page, "1011");
    await expect(pane.getByRole("status")).toContainText("Graded 2, mostly seen.");
    await expect(pane.getByRole("list", { name: "Sealed chain" })).toContainText("Graded the prep 2");
    await search(page, "");
    await page.getByRole("tab", { name: "All 21" }).click();
    await page.getByRole("button", { name: /^Aisha Al Mansoori/ }).click();
    await expect(pane.getByRole("status")).toContainText("Graded 2, mostly seen.");
    await pane.getByRole("button", { name: "Undo" }).click();
    await expect(pane.getByRole("group", { name: "Boston grade" })).toBeVisible();
  });

  test("an unconfirmed return reason stays with its referral when a search shows another", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await page.getByRole("button", { name: /^AHS-sr-9003/ }).click();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await pane.getByRole("button", { name: "Wrong unit" }).click();
    await search(page, "9002");
    await expect(pane.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-9002");
    await expect(pane.getByRole("heading", { name: "Why return this referral?" })).toHaveCount(0);
    await expect(pane.getByRole("button", { name: "Accept referral", exact: true })).toBeVisible();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByText("Choose a reason to return it.")).toBeVisible();
    await expect(pane.getByRole("button", { name: "Wrong unit" })).toHaveAttribute("aria-pressed", "false");
    await search(page, "");
    await expect(pane.getByRole("button", { name: "Wrong unit", pressed: true })).toHaveCount(0);
  });

  test("unconfirmed conditions are not carried across a zero-result search to another referral", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await page.getByRole("button", { name: /^AHS-sr-9003/ }).click();
    await pane.getByRole("button", { name: "Accept with conditions" }).click();
    await pane.getByRole("button", { name: "Female endoscopist" }).click();
    await search(page, "no-such-case");
    await expect(page.getByRole("tab", { name: "All 0" })).toBeVisible();
    await search(page, "9002");
    await expect(pane.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-9002");
    await expect(pane.getByRole("heading", { name: "Accept on these conditions" })).toHaveCount(0);
    await pane.getByRole("button", { name: "Accept with conditions" }).click();
    await expect(pane.getByRole("button", { name: "Accept with no conditions" })).toBeVisible();
    await expect(pane.getByRole("button", { name: "Female endoscopist", pressed: true })).toHaveCount(0);
  });

  test("clearing a search drops an unconfirmed draft but keeps committed decisions", async ({ page }) => {
    await page.goto("/app/referrals");
    const pane = page.getByRole("region", { name: "Open referral" });
    await acceptFromRow(page, "AHS-sr-9001");
    await page.getByRole("button", { name: /^AHS-sr-9002/ }).click();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await pane.getByRole("button", { name: "Wrong unit" }).click();
    await search(page, "9003");
    await search(page, "");
    await expect(pane.getByRole("heading", { name: "Why return this referral?" })).toHaveCount(0);
    await expect(page.getByRole("tab", { name: "Accepted 17" })).toBeVisible();
    await page.getByRole("tab", { name: "All 21" }).click();
    await page.getByRole("button", { name: /^AHS-sr-9001/ }).click();
    await expect(pane.getByRole("status")).toContainText("Accepted. Your unit books her now.");
  });

  test("search is wired to q", async ({ page }) => {
    await page.goto("/app/referrals");
    await page.getByRole("searchbox", { name: "Search case id" }).fill("1011");
    await page.getByRole("searchbox", { name: "Search case id" }).press("Enter");
    await expect(page).toHaveURL(/\/app\/referrals\?q=1011$/);
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
    await page.goto("/app/patients?q=1011");
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

  test("Today: Call is in the first screen; the open case unfolds inline in the queue", async ({ page }) => {
    await page.goto("/app/today");
    await noSideScroll(page);
    const call = page.getByRole("button", { name: "Log call · AHS-sr-1002" });
    const box = (await call.boundingBox())!;
    expect(box.y + box.height).toBeLessThanOrEqual(PHONE.height);
    await targetsAre44(page);
    await shot(page, "today-390");
    await noSeriousAxe(page);

    // Folding the open row leaves the queue alone on the screen.
    await page.locator('button[data-case-row="AHS-sr-1002"]').click();
    await expect(page.getByRole("region", { name: "Selected case" })).toBeHidden();
    await noSideScroll(page);
    await targetsAre44(page);
    await shot(page, "today-list-390");
    await noSeriousAxe(page);

    await page.locator('button[data-case-row="AHS-sr-1023"]').click();
    await expect(page.getByRole("region", { name: "Selected case" })).toHaveAttribute("data-case", "AHS-sr-1023");
    await expect(page.getByRole("button", { name: "Log call · AHS-sr-1023" })).toBeVisible();
  });

  test("Today: after the call, the outcome buttons stay beside Call", async ({ page }) => {
    await page.clock.setFixedTime(new Date("2026-11-10T09:00:00Z"));
    await page.goto("/app/today");
    await page.getByRole("button", { name: "Log call · AHS-sr-1002" }).click();
    await page.getByRole("button", { name: "Reached" }).click();
    await page.getByRole("button", { name: "Call back on…" }).click();
    await noSideScroll(page);
    await targetsAre44(page);
    await page.getByRole("button", { name: "Wed 11 Nov" }).click();
    await targetsAre44(page);
    await shot(page, "today-logged-390");
  });

  test("Referrals: stacked, the grade controls near the top, 44px targets", async ({ page }) => {
    await page.goto("/app/referrals");
    await noSideScroll(page);
    await targetsAre44(page);
    await page.getByRole("button", { name: /All referrals/ }).click();
    await page.getByRole("button", { name: "Review the referral for Aisha Al Mansoori" }).click();
    const grade = page.getByRole("region", { name: "Open referral" }).getByRole("group", { name: "Boston grade" });
    const box = (await grade.boundingBox())!;
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
    await page.getByRole("button", { name: "Log call · AHS-sr-1002" }).focus();
    const outline = await page.evaluate(() => {
      const s = getComputedStyle(document.activeElement!);
      return { w: s.outlineWidth, style: s.outlineStyle };
    });
    // The staff shell draws a 3px ring; anything solid and at least 2px is visible focus.
    expect(outline.style).toBe("solid");
    expect(parseFloat(outline.w)).toBeGreaterThanOrEqual(2);
  });
});

test.describe("Patients: paging and filters", () => {
  test("25 per page; Next and Previous keep the filters", async ({ page }) => {
    await page.goto("/app/patients");
    await expect(page.getByTestId("range")).toHaveText("Showing 1–25 of 177");
    await expect(page.locator("tbody tr")).toHaveCount(25);
    await expect(page.getByRole("link", { name: "Previous" })).toHaveCount(0);
    await page.getByRole("navigation", { name: "Pages" }).getByRole("link", { name: "Next" }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page.getByTestId("range")).toHaveText("Showing 26–50 of 177");

    await page.goto("/app/patients?programme=crc&page=2");
    await expect(page.getByTestId("showing")).toContainText("colorectal screening");
    await page.getByRole("link", { name: "Previous" }).click();
    await expect(page).toHaveURL(/programme=crc/);
    await expect(page).not.toHaveURL(/page=/);
  });

  test("a page past the end lands on the last page", async ({ page }) => {
    await page.goto("/app/patients?page=99");
    await expect(page).toHaveURL(/page=8$/);
    await expect(page.getByTestId("range")).toHaveText("Showing 176–177 of 177");
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
