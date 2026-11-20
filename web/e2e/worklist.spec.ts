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
    await shot(page, "today");
    await noSeriousAxe(page);
  });

  test("Patient details groups contact, timeline and case facts; the case file is unavailable without a record", async ({ page }) => {
    await page.goto("/app/today");
    const details = open(page).getByRole("button", { name: "Patient details" });
    await expect(details).toHaveAttribute("aria-expanded", "false");
    await expect(open(page).getByTestId("case-facts")).toBeHidden();
    await expect(open(page).getByTestId("lead-days")).toBeVisible();
    await details.click();
    await expect(details).toHaveAttribute("aria-expanded", "true");
    const facts = open(page).getByTestId("case-facts");
    await expect(facts.getByRole("heading", { level: 3 })).toHaveText(["Contact", "Timeline", "Case"]);
    await expect(facts).toContainText("Next stepCall to confirm they received the result");
    await expect(facts).toContainText("PermissionGranted");
    await expect(facts).toContainText("OwnerNurse 6");
    await expect(facts).toContainText("Referred12 May108 days ago");
    await expect(facts).toContainText("DeadlineDay 31 · 12 Jun77 days over 31");
    // Open, the clock lives in the Timeline group only.
    await expect(open(page).getByTestId("lead-days")).toHaveCount(0);
    await expect(open(page).getByRole("button", { name: "No case file" })).toBeDisabled();
    await noSeriousAxe(page);
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
    await expect(history.getByText("Recorded by This browser").filter({ visible: true })).toBeVisible();

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
    await open(page).getByRole("button", { name: "Patient details" }).click();
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

  test("Open case file opens the full record and Audit trail its audit", async ({ page }) => {
    await page.goto("/app/today?queue=waiting");
    await row(page, "AHS-sr-1011").click();
    await expect(open(page).getByTestId("case-who")).toContainText("Aisha Al Mansoori");
    await expect(page.getByRole("region", { name: "Case history" }).getByRole("link", { name: "Audit trail" })).toHaveAttribute("href", /AHS-sr-1011\/audit$/);
    await open(page).getByRole("link", { name: "Open case file" }).click();
    await expect(page).toHaveURL(aisha);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Aisha Al Mansoori");
  });

  test("Records: a case with held evidence shows its held count, the held report with its reason, and source events in Activity", async ({ page }) => {
    await page.goto("/app/today?queue=waiting");
    await row(page, "AHS-sr-1011").click();
    const history = page.getByRole("region", { name: "Case history" });
    const records = history.getByRole("tab", { name: /^Records/ });
    await expect(history.getByRole("tab", { name: "Activity" })).toHaveAttribute("aria-selected", "true");
    await expect(records).toHaveAttribute("aria-selected", "false");
    await expect(records.getByTestId("records-held")).toHaveText("1 held");
    // Activity gains the source timeline in the Audit page's words.
    await expect(history.getByRole("tabpanel", { name: "Activity" })).toContainText("Report issued");
    await history.getByTestId("earlier-activity").getByText(/^Earlier activity/).click();
    await expect(history.getByTestId("earlier-activity")).toContainText("Referral issued");
    // Arrow keys move between the tabs.
    await history.getByRole("tab", { name: "Activity" }).focus();
    await page.keyboard.press("ArrowRight");
    await expect(records).toBeFocused();
    await expect(records).toHaveAttribute("aria-selected", "true");
    const panel = history.getByRole("tabpanel", { name: /^Records/ });
    const held = panel.locator("[data-testid=source-record][data-held]");
    await expect(held).toHaveCount(1);
    await expect(held).toContainText("Report, colonoscopy");
    await expect(held).toContainText("Held");
    await expect(held).toContainText("Noor endoscopy · 10 Jul 2026");
    await expect(held.getByTestId("held-reason")).toHaveText("The report does not show the exam was complete, so the case stays open.");
    await expect(panel.locator("[data-testid=source-record]:not([data-held])").first()).not.toContainText("Held");
    await expect(history.getByRole("link", { name: "Full case record" })).toHaveAttribute("href", /AHS-sr-1011$/);
    await noSeriousAxe(page);
    // A case this demo has no detail record for says so, with no held count, and names why its links are missing.
    await row(page, "AHS-sr-1022").click();
    await expect(records.getByTestId("records-held")).toHaveCount(0);
    await expect(panel.getByTestId("records-missing")).toHaveText("No records to show: this demo has records for a few sample patients only.");
    await expect(history.getByTestId("record-unavailable")).toHaveText("Full case record: not available in this demo");
    await history.getByRole("tab", { name: "Activity" }).click();
    await expect(history.getByTestId("audit-unavailable")).toHaveText("Audit trail: not available in this demo");
    await expect(open(page).getByRole("button", { name: "No case file" })).toHaveAttribute("title", /sample patients only/);
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
    await expect(page.getByRole("region", { name: "Referrals by date received" })).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: /^All\b/ })).not.toHaveAttribute("aria-current", "true");
    await shot(page, "patients-filtered");
    await noSeriousAxe(page);

    await page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Closed" }).click();
    await expect(page).toHaveURL(/closed=true/);
    await expect(page.getByTestId("showing")).toContainText("cases: closed");
    await page.getByRole("link", { name: /^Month’s files for / }).click();
    await expect(page.locator("[data-pf-case]").first()).toContainText("Closed day");
  });

  test("search by case id leads to the record through its date and folder", async ({ page }) => {
    await page.goto("/app/patients");
    await expect(page.getByTestId("showing")).toHaveText("177 cases: all cases");
    await shot(page, "patients");
    await page.getByRole("searchbox", { name: "Find a case, task or owner" }).fill("AHS-sr-1001");
    await page.getByRole("button", { name: "Search", exact: true }).click();
    await expect(page).toHaveURL(/q=AHS-sr-1001/);
    await page.getByRole("link", { name: /^23 May 2026, 1 referral/ }).click();
    await page.locator("[data-pf-case]", { hasText: "Fatima Al Zaabi" }).click();
    await expect(page).toHaveURL(/open=ahs-mushrif%2FServiceRequest%2FAHS-sr-1001/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Fatima Al Zaabi");
  });
});

test.describe("Patient record", () => {
  test("Fatima: two names bridged, evidence quoted, chain verified", async ({ page }) => {
    await page.goto(fatima);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Fatima Al Zaabi");
    await expect(page.getByText("Closed day 27")).toBeVisible();
    await expect(page.getByTestId("closed-in")).toHaveText("Closed in 27 days, 4 inside the 31-day standard");
    const chain = page.getByRole("list", { name: "Decision history" });
    await expect(chain.getByRole("listitem").last()).toContainText("Closed · with evidence, dated to the colonoscopy");
    await expect(page.getByText("فاطمة الزعابي").first()).toBeVisible();
    await expect(page.getByText("Fatima Alzaabi")).toBeVisible();
    await expect(page.getByText("“the cecum, which was identified by the presence of the appendiceal orifice and ileocecal valve”")).toBeVisible();
    await expect(page.getByTestId("chain-verified")).toContainText("Unchanged · 3 of 3 entries checked");
    await expect(page.getByText("Message owner")).toHaveCount(0);

    await page.getByText("3 recorded entries").click();
    await expect(page.getByRole("list", { name: "Recorded entries" }).getByRole("listitem")).toHaveText([/Report matched/, /Readers' result/, /Closed/]);
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
    await expect(page.getByTestId("chain-verified")).toContainText("Unchanged · 5 of 5 entries checked");
    await expect(page.getByRole("list", { name: "Decision history" }).getByRole("listitem").last()).toContainText("Closed · with evidence, dated to the eye-clinic exam");
    await shot(page, "record-dr");
    await noSeriousAxe(page);
  });
});

test.describe("Referrals", () => {
  const pane = (page: Page) => page.getByRole("region", { name: "Open referral" });
  const counts = async (page: Page, status: string) => {
    await page.goto(`/app/referrals?view=inbox&status=${status}`);
    return page.getByTestId("page-range").textContent();
  };

  test("opens on the referral that needs a response: two parties, the arrow, status, id and the unit's actions", async ({ page }) => {
    await page.goto("/app/referrals");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Colonoscopy referral");
    await expect(page.getByText("Positive FIT", { exact: true })).toBeVisible();
    const route = page.getByRole("group", { name: "Hand-off from Al Mushrif GP to SKMC endoscopy" });
    await expect(route.getByRole("heading", { level: 2 })).toHaveText(["Al Mushrif GP", "SKMC endoscopy"]);
    await expect(page.getByTestId("handoff-arrow")).toHaveAttribute("data-direction", "forward");
    const unit = page.getByRole("region", { name: "SKMC endoscopy, current" });
    await expect(unit).toContainText("Needs response");
    await expect(unit).toContainText("AHS-sr-9003");
    await expect(unit.getByRole("button", { name: "Accept referral" })).toBeVisible();
    await expect(unit.getByRole("button", { name: "Return with reason" })).toBeVisible();
    await expect(page.getByRole("region", { name: "Referral inbox" }).getByRole("listitem")).toHaveCount(3);
    await expect(page.getByRole("link", { name: "View all 21 referrals" })).toBeVisible();
    await expect(page.getByRole("list", { name: "Hand-off activity" })).toContainText("Referral sent");
    await shot(page, "referrals");
    await noSeriousAxe(page);
  });

  test("the full inbox counts each status from the fixture referrals", async ({ page }) => {
    await page.goto("/app/referrals?view=inbox");
    await expect(page.getByTestId("page-range")).toHaveText("1–10 of 21 referrals");
    expect(await counts(page, "needs")).toBe("3 referrals");
    expect(await counts(page, "report")).toBe("1 referral");
    expect(await counts(page, "waiting")).toBe("1 referral");
    expect(await counts(page, "accepted")).toBe("6 referrals");
    expect(await counts(page, "closed")).toBe("10 referrals");
    await noSeriousAxe(page);
  });

  test("Return a referral (user flow): page, search 9001, empty reason blocked, reason sent, arrow reversed, back to the same inbox", async ({ page }) => {
    await page.goto("/app/referrals");
    await page.getByRole("link", { name: "View all 21 referrals" }).click();
    await expect(page).toHaveURL(/view=inbox/);
    await page.getByRole("navigation", { name: "Inbox pages" }).getByRole("link", { name: "Next page" }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page.getByTestId("page-range")).toHaveText("11–20 of 21 referrals");
    const box = page.getByRole("searchbox", { name: "Search case id" });
    await box.fill("9001");
    await box.press("Enter");
    await expect(page).toHaveURL("/app/referrals?q=9001&view=inbox");
    await page.getByRole("link", { name: "AHS-sr-9001" }).click();
    await expect(page).toHaveURL(/q=9001&from=inbox&case=/);

    await pane(page).getByRole("button", { name: "Return with reason" }).click();
    await pane(page).getByRole("button", { name: "Return to GP" }).click();
    await expect(pane(page).getByRole("alert")).toHaveText("Choose a reason before you return it. Nothing was sent.");
    await expect(page.getByTestId("handoff-arrow")).toHaveAttribute("data-direction", "forward");
    await pane(page).getByRole("radio", { name: "Needs a clinic visit first" }).check();
    await pane(page).getByRole("button", { name: "Return to GP" }).click();

    await expect(page.getByTestId("handoff-arrow")).toHaveAttribute("data-direction", "back");
    await expect(page.getByRole("group", { name: "Hand-off from SKMC endoscopy to Al Mushrif GP" })).toBeVisible();
    const gp = page.getByRole("region", { name: "Al Mushrif GP, current" });
    await expect(gp).toContainText("Waiting on GP");
    await expect(gp).toContainText("Booking held");
    await expect(gp.getByRole("status")).toContainText("Returned to the Al Mushrif GP. Reason: needs a clinic visit first.");
    await expect(gp.getByRole("link", { name: "Open the GP view" })).toBeVisible();
    await shot(page, "referrals-returned");

    await page.getByRole("link", { name: "Back to inbox" }).click();
    await expect(page).toHaveURL(/q=9001&view=inbox&case=/);
    await expect(page.getByRole("searchbox", { name: "Search case id" })).toHaveValue("9001");
    const row = page.getByRole("row", { name: /AHS-sr-9001/ });
    await expect(row).toContainText("Waiting on GP");
    await expect(row.getByRole("link", { name: "AHS-sr-9001" })).toBeFocused();
    await noSeriousAxe(page);
  });

  test("Accept, Undo and conditions on the open referral", async ({ page }) => {
    await page.goto(`/app/referrals?case=${encodeURIComponent("ahs-mushrif/ServiceRequest/AHS-sr-9001")}`);
    await pane(page).getByRole("button", { name: "Accept referral" }).click();
    await expect(pane(page).getByRole("status")).toContainText("Accepted. Your unit books her now.");
    await expect(page.getByRole("region", { name: "SKMC endoscopy, current" })).toContainText("Accepted");
    await pane(page).getByRole("button", { name: "Undo" }).click();
    await expect(pane(page).getByRole("button", { name: "Accept referral" })).toBeVisible();

    await page.getByRole("region", { name: "Referral inbox" }).getByRole("link", { name: /^AHS-sr-9002/ }).click();
    await expect(pane(page).getByTestId("flags")).toHaveCount(0);
    await pane(page).getByRole("button", { name: "Accept with conditions" }).click();
    await pane(page).getByRole("button", { name: "Arabic interpreter on the day" }).click();
    await pane(page).getByRole("button", { name: "Female endoscopist" }).click();
    await pane(page).getByRole("button", { name: "Accept with 2 conditions" }).click();
    await expect(pane(page).getByRole("status")).toContainText("Accepted with conditions. Arabic interpreter on the day, Female endoscopist.");
  });

  test("Return to GP clocks the GP from the return day and Undo clears it", async ({ page }) => {
    await page.goto("/app/referrals");
    await pane(page).getByRole("button", { name: "Return with reason" }).click();
    await expect(pane(page).getByText("who must act by 1 Sep.")).toBeVisible();
    await pane(page).getByRole("radio", { name: "Wrong unit" }).check();
    await pane(page).getByRole("button", { name: "Return to GP" }).click();
    await expect(pane(page).getByRole("status")).toContainText("Returned to the Al Mushrif GP. Reason: wrong unit. Act by 1 Sep.");
    const activity = page.getByRole("list", { name: "Hand-off activity" });
    await expect(activity.getByRole("listitem").last()).toContainText("Now: Act on the return");
    await expect(activity.getByRole("listitem").last()).toContainText("by 1 Sep");
    await pane(page).getByRole("button", { name: "Undo" }).click();
    await expect(activity).not.toContainText("by 1 Sep");
    await expect(page.getByTestId("handoff-arrow")).toHaveAttribute("data-direction", "forward");
  });

  test("an existing return without a hand-off says its deadline is unavailable", async ({ page }) => {
    await page.goto(`/app/referrals?case=${encodeURIComponent("ahs-mushrif/ServiceRequest/AHS-sr-9004")}`);
    await expect(page.getByTestId("handoff-arrow")).toHaveAttribute("data-direction", "back");
    const last = page.getByRole("list", { name: "Hand-off activity" }).getByRole("listitem").last();
    await expect(last).toContainText("Act on the return");
    await expect(last).toContainText("deadline unavailable");
  });

  for (const [grade, words] of [[0, "Not seen"], [1, "Partly seen"], [2, "Mostly seen"], [3, "All seen"]] as const) {
    test(`prep grade ${grade} stays held, survives the inbox round trip and undoes`, async ({ page }) => {
      await page.goto("/app/referrals?status=report");
      await pane(page).getByRole("group", { name: "Boston grade" }).getByRole("button", { name: new RegExp(`^${grade}\\s*${words}$`) }).click();
      await expect(pane(page).getByRole("status")).toContainText(`Graded ${grade}, ${words.toLowerCase()}. The case stays held until the report is complete.`);
      await expect(pane(page).getByRole("button", { name: "Undo" })).toBeFocused();
      const activity = page.getByRole("list", { name: "Hand-off activity" });
      await expect(activity).toContainText(`Graded the prep ${grade}`);
      await expect(activity.getByRole("listitem").last()).toContainText("Complete the report");
      await page.getByRole("link", { name: /^View all/ }).click();
      await expect(page.getByRole("row", { name: /AHS-sr-1011/ })).toContainText("Report incomplete");
      await page.getByRole("link", { name: "AHS-sr-1011" }).click();
      await expect(activity).toContainText(`Graded the prep ${grade}`);
      await pane(page).getByRole("button", { name: "Undo" }).click();
      await expect(activity).not.toContainText("Graded the prep");
      await expect(pane(page).getByRole("group", { name: "Boston grade" })).toBeVisible();
    });
  }

  test("a referral with a record links to it from the header", async ({ page }) => {
    await page.goto("/app/referrals?status=report");
    await expect(page.getByText("Aisha Al Mansoori", { exact: true })).toBeVisible();
    await pane(page).getByRole("link", { name: "Open Aisha Al Mansoori's record" }).click();
    await expect(page).toHaveURL(aisha);
  });

  test("search is wired to q and keeps decisions made before it", async ({ page }) => {
    await page.goto(`/app/referrals?case=${encodeURIComponent("ahs-mushrif/ServiceRequest/AHS-sr-9001")}`);
    await pane(page).getByRole("button", { name: "Accept referral" }).click();
    await page.getByRole("link", { name: /^View all/ }).click();
    const box = page.getByRole("searchbox", { name: "Search case id" });
    await box.fill("1011");
    await box.press("Enter");
    await expect(page).toHaveURL(/\/app\/referrals\?q=1011&view=inbox$/);
    await expect(page.getByTestId("page-range")).toHaveText("1 referral");
    await expect(box).toBeFocused();
    await box.fill("9001");
    await box.press("Enter");
    await expect(page.getByRole("row", { name: /AHS-sr-9001/ })).toContainText("Accepted");
    await box.fill("");
    await box.press("Enter");
    await expect(page.getByTestId("page-range")).toHaveText("1–10 of 21 referrals");
    await box.fill("no-such-case");
    await box.press("Enter");
    await expect(page.getByRole("status").filter({ hasText: "No matching referrals" })).toBeVisible();
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

  test("Patients: calendar and case files, no side scroll, 44px targets", async ({ page }) => {
    await page.goto("/app/patients");
    await noSideScroll(page);
    await targetsAre44(page);
    await page.goto("/app/patients?q=AHS-sr-1011");
    // On a phone each date shows a count; the tile opens that date's case files.
    await page.getByRole("link", { name: /^27 May 2026, 1 referral/ }).click();
    const folder = page.locator("[data-pf-case]").first();
    await expect(folder).toContainText("Aisha Al Mansoori");
    await expect(folder).toContainText("Due");
    await expect(folder).toContainText("Nurse 3");
    const box = (await folder.boundingBox())!;
    expect(box.x + box.width).toBeLessThanOrEqual(PHONE.width);
    await noSideScroll(page);
    await targetsAre44(page);
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

  test("Referrals: stacked hand-off, the unit's actions near the top, 44px targets", async ({ page }) => {
    await page.goto("/app/referrals");
    await noSideScroll(page);
    await targetsAre44(page);
    const accept = page.getByRole("button", { name: "Accept referral" });
    const box = (await accept.boundingBox())!;
    expect(box.y + box.height).toBeLessThanOrEqual(PHONE.height);
    await shot(page, "referrals-390");
    await noSeriousAxe(page);

    await page.getByRole("link", { name: /^View all/ }).click();
    await noSideScroll(page);
    await targetsAre44(page);
    await shot(page, "referrals-list-390");
    await noSeriousAxe(page);

    await page.getByRole("link", { name: "AHS-sr-1011" }).click();
    await expect(page.getByRole("region", { name: "Open referral" }).getByRole("group", { name: "Boston grade" })).toBeVisible();
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
  test("a month's case files show 25 a page; Next and Previous keep the filters", async ({ page }) => {
    await page.goto("/app/patients?files=2026-05");
    await expect(page.getByTestId("range")).toHaveText("Showing 1–25 of 39");
    await expect(page.locator("[data-pf-case]")).toHaveCount(25);
    await expect(page.getByRole("link", { name: "Previous page" })).toHaveCount(0);
    await page.getByRole("navigation", { name: "Pages" }).getByRole("link", { name: "Next page" }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page.getByTestId("range")).toHaveText("Showing 26–39 of 39");

    await page.goto("/app/patients?programme=crc&files=2026-05&page=2");
    await expect(page.getByTestId("showing")).toContainText("colorectal screening");
    await expect(page.getByTestId("range")).toHaveCount(0);
  });

  test("a page past the end lands on the last page", async ({ page }) => {
    await page.goto("/app/patients?files=2026-05&page=99");
    await expect(page).toHaveURL(/page=2$/);
    await expect(page.getByTestId("range")).toHaveText("Showing 26–39 of 39");
  });

  test("an old list link still works: the calendar opens with its filters", async ({ page }) => {
    await page.goto("/app/patients?programme=crc&page=2");
    await expect(page.getByTestId("showing")).toContainText("colorectal screening");
    await expect(page.getByRole("region", { name: "Referrals by date received" })).toBeVisible();
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
