import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

test.use({ viewport: { width: 390, height: 844 } });

const LANGS = [
  { lang: "ar", dir: "rtl" },
  { lang: "en", dir: "ltr" },
  { lang: "ur", dir: "rtl" },
  { lang: "fil", dir: "ltr" },
] as const;
const STEPS = ["notice", "check", "book"] as const;

async function expectNoSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

for (const step of STEPS) {
  for (const { lang, dir } of LANGS) {
    test(`/patient/${step}?lang=${lang} renders ${dir}, labelled designed, axe clean`, async ({ page }) => {
      const res = await page.goto(`/patient/${step}?lang=${lang}`);
      expect(res?.status()).toBe(200);
      await expect(page.locator(`div[lang="${lang}"]`)).toHaveAttribute("dir", dir);
      await expect(page.locator("h1")).toHaveCount(1);
      await expect(page.locator("h1")).toBeVisible();
      await expect(page.locator("[data-honesty=designed]")).toHaveText("Designed, not built · no API");
      const nav = page.getByRole("navigation", { name: "Demo steps" });
      for (const s of STEPS) await expect(nav.locator(`a[href^="/patient/${s}?lang=${lang}"]`)).toHaveCount(1);
      await page.evaluate(() => document.fonts.ready);
      await page.screenshot({ path: `test-results/screens/patient_${step}_${lang}.png` });
      await expectNoSeriousAxe(page);
    });
  }
}

test("Arabic is the default language", async ({ page }) => {
  await page.goto("/patient/notice");
  await expect(page.locator('div[lang="ar"]')).toHaveAttribute("dir", "rtl");
  await expect(page.locator("h1")).toHaveText("نتيجة فحصكِ تحتاج فحصًا آخر");
});

test("notice: switch language, read rights, say who calls whom", async ({ page }) => {
  await page.goto("/patient/notice");
  await page.getByRole("button", { name: /اللغة/ }).click();
  const sheet = page.getByRole("dialog", { name: "اللغة" });
  await expect(sheet.getByRole("link")).toHaveCount(4);
  await expect(sheet.getByRole("link", { name: "العربية" })).toHaveAttribute("aria-current", "true");
  await page.screenshot({ path: "test-results/screens/patient_notice_ar_language.png" });
  await sheet.getByRole("link", { name: "English" }).click();
  await expect(page).toHaveURL(/\/patient\/notice\?lang=en$/);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.locator("h1")).toHaveText("Your test result needs one more check");

  const card = page.locator("section", { has: page.getByRole("heading", { name: "Next step" }) });
  await expect(card).toContainText("Nurse Hessa will call you within 2 working days");
  await expect(card).toContainText("You can also call her: 02 555 0142");
  await expect(page.getByRole("link", { name: /Call the nurse/ })).toHaveAttribute("href", "tel:+97125550142");

  const rightsRow = page.getByRole("button", { name: "Your rights" });
  await expect(rightsRow).toHaveCSS("border-top-width", "0px");
  await rightsRow.click();
  const rights = page.getByRole("dialog", { name: "Your rights" });
  await expect(rights.getByRole("listitem")).toHaveCount(3);
  await page.screenshot({ path: "test-results/screens/patient_notice_en_rights.png" });
  await expectNoSeriousAxe(page);
  await rights.getByRole("button", { name: "Close" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("check: wrong answer re-explains, right answer asks consent, consent carries to booking", async ({ page }) => {
  await page.goto("/patient/check?lang=en");
  await expect(page.getByText("This is not an exam.")).toBeVisible();
  await expect(page.getByRole("img", { name: "Step 1 of 3: Question" })).toBeVisible();

  await page.getByRole("button", { name: /Another blood test/ }).click();
  await expect(page.locator("h1")).toHaveText("Let us explain it another way");
  await expect(page.getByText("The fault is in our explanation, not in you.")).toBeVisible();
  await page.screenshot({ path: "test-results/screens/patient_check_en_explain.png" });
  await page.getByRole("button", { name: "Ask me again" }).click();

  await page.getByRole("button", { name: /small camera/ }).click();
  await expect(page.locator("h1")).toHaveText("May we send you reminders by SMS?");
  await expect(page.getByRole("img", { name: "Step 2 of 3: Messages" })).toBeVisible();
  await page.screenshot({ path: "test-results/screens/patient_check_en_consent.png" });
  await page.getByRole("button", { name: "Allow messages" }).click();

  await expect(page).toHaveURL(/consent=yes/);
  await expect(page.locator("h1")).toHaveText("Thank you, Fatima");
  await expect(page.getByText("Nurse Hessa Al Ali will call you within 2 working days")).toContainText("You can also call her: 02 555 0142");
  await expect(page.getByText("We will also send you an SMS reminder before the appointment.")).toBeVisible();
  await page.screenshot({ path: "test-results/screens/patient_check_en_done.png" });

  await page.getByRole("navigation", { name: "Demo steps" }).getByRole("link", { name: "3 Book" }).click();
  await expect(page).toHaveURL(/\/patient\/book\?lang=en&consent=yes$/);
  await page.getByRole("button", { name: "Confirm this appointment" }).click();
  await expect(page.getByText("We will send you an SMS reminder 2 days before.")).toBeVisible();
});

test("check: declining SMS means Hessa calls, and no SMS is promised on the booking", async ({ page }) => {
  await page.goto("/patient/check?lang=en");
  await page.getByRole("button", { name: /small camera/ }).click();
  await page.getByRole("button", { name: "Not now" }).click();
  await expect(page).toHaveURL(/consent=no/);
  await expect(page.getByText("We will not send you messages.")).toBeVisible();
  await page.getByRole("navigation", { name: "Demo steps" }).getByRole("link", { name: "3 Book" }).click();
  await page.getByRole("button", { name: "Confirm this appointment" }).click();
  await expect(page.getByText("Hessa will call you before the appointment to remind you.")).toBeVisible();
  await expect(page.getByText(/SMS/)).toHaveCount(0);
});

test("check: Arabic says this is not an exam", async ({ page }) => {
  await page.goto("/patient/check");
  await expect(page.getByText("هذا ليس امتحانًا.")).toBeVisible();
  await expect(page.getByText("تُحاسَبين")).toHaveCount(0);
});

test("book: October slots, pick another, confirm, prep list", async ({ page }) => {
  await page.goto("/patient/book?lang=en");
  await expect(page.getByTestId("slot-date")).toHaveText("Friday 9 October");
  await expect(page.getByTestId("slot-time")).toHaveText("at 9:30 am");

  await page.getByRole("button", { name: "Another time" }).click();
  const others = page.getByRole("group", { name: "Other times" }).getByRole("button");
  await expect(others).toHaveText(["Thursday 8 October, 11:00 am", "Monday 12 October, 1:00 pm"]);
  await page.screenshot({ path: "test-results/screens/patient_book_en_choosing.png" });
  await others.nth(1).click();
  await expect(page.getByTestId("slot-date")).toHaveText("Monday 12 October");
  await expect(page.getByTestId("slot-time")).toHaveText("at 1:00 pm");

  await page.getByRole("button", { name: "Confirm this appointment" }).click();
  await expect(page.getByText("Your appointment is confirmed")).toBeVisible();
  await expect(page.getByText("Hessa will call you before the appointment to remind you.")).toBeVisible();
  await page.screenshot({ path: "test-results/screens/patient_book_en_confirmed.png" });
  await page.getByRole("button", { name: "Change appointment" }).click();
  await expect(page.getByRole("group", { name: "Other times" }).getByRole("button")).toHaveCount(2);

  const warning = page.getByRole("note");
  await expect(warning).toHaveText("If you take diabetes medicine or blood thinners, call Hessa before your appointment.");
  const boxes = page.getByRole("checkbox");
  await expect(boxes).toHaveCount(2);
  await expect(boxes.filter({ hasText: "diabetes" })).toHaveCount(0);
  await boxes.first().click();
  await expect(boxes.first()).toHaveAttribute("aria-checked", "true");
});

test("language persists across the three phones", async ({ page }) => {
  await page.goto("/patient/notice?lang=ur");
  const nav = page.getByRole("navigation", { name: "Demo steps" });
  await nav.getByRole("link", { name: "2 Check" }).click();
  await expect(page).toHaveURL(/\/patient\/check\?lang=ur$/);
  await expect(page.locator('div[lang="ur"]')).toHaveAttribute("dir", "rtl");
  await nav.getByRole("link", { name: "3 Book" }).click();
  await expect(page).toHaveURL(/\/patient\/book\?lang=ur$/);
  await expect(page.getByTestId("slot-date")).toContainText("جمعہ");
});

test.describe("desktop", () => {
  test.use({ viewport: { width: 1440, height: 900 } });
  test("centres a phone-width column", async ({ page }) => {
    await page.goto("/patient/notice");
    const box = await page.locator("main").boundingBox();
    expect(box && Math.round(box.x + box.width / 2)).toBe(720);
    expect(box?.width).toBeLessThanOrEqual(390);
    await page.screenshot({ path: "test-results/screens/patient_notice_ar_desktop.png" });
  });
});
