import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import type { PatientJourney, PatientMutation } from "@/lib/api/patient-schema";

const MOCK = "http://127.0.0.1:31987";
let token = "";
let expiresAt = "";
test.describe.configure({ mode: "serial" });
test.beforeEach(async () => {
  token = randomUUID();
  const response = await fetch(`${MOCK}/__mock/links`, { method: "POST", body: JSON.stringify({ token }) });
  expiresAt = (await response.json()).expires_at;
});
test.use({ viewport: { width: 390, height: 844 } });

async function control(path: string, mode?: string) {
  const response = await fetch(`${MOCK}${path}`, { method: "POST", body: JSON.stringify({ token, mode }) });
  if (!response.ok) throw new Error(`patient mock control failed: ${response.status}`);
}
async function bodies(): Promise<PatientMutation[]> {
  return (await fetch(`${MOCK}/__mock/requests?token=${token}`)).json();
}
async function journey(): Promise<PatientJourney> {
  return (await (await fetch(`${MOCK}/__mock/state?token=${token}`)).json()).journey;
}

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

async function enter(page: Page) {
  await page.goto(`/patient/access?link=${token}`);
  await expect(page).toHaveURL(/\/patient\/notice$/);
  await expect(page.locator("h1")).toBeVisible();
}

async function open(page: Page, step: string, lang = "en") {
  await enter(page);
  await page.goto(`/patient/${step}?lang=${lang}`);
}

function patientCookie(page: Page) {
  return page.context().cookies().then((cookies) => cookies.find((cookie) => cookie.name === "__Host-wasl_patient_link"));
}

for (const step of STEPS) {
  for (const { lang, dir } of LANGS) {
    test(`/patient/${step}?lang=${lang} uses a live projection, ${dir}, axe clean`, async ({ page }) => {
      await open(page, step, lang);
      await expect(page.locator(`div[lang="${lang}"]`)).toHaveAttribute("dir", dir);
      await expect(page.locator("h1")).toHaveCount(1);
      await expect(page.locator("h1")).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
      await expect(page.locator("[data-honesty=built]")).toHaveText("Synthetic · server saved");
      await expect(page.locator("[data-honesty=designed]")).toHaveCount(0);
      const nav = page.getByRole("navigation", { name: "Demo steps" });
      for (const s of STEPS) {
        await expect(nav.locator(`a[href^="/patient/${s}?lang=${lang}"]`)).toHaveCount(1);
        await expect(nav.locator(`a[href^="/patient/${s}"]`)).not.toHaveAttribute("href", /consent/);
      }
      await page.evaluate(() => document.fonts.ready);
      await page.screenshot({ path: `test-results/screens/patient_${step}_${lang}.png` });
      await expectNoSeriousAxe(page);
    });
  }
}

test("access verifies the link, sets a bounded host cookie and strips the token", async ({ page }) => {
  await enter(page);
  expect(page.url()).not.toContain(token);
  const cookie = await patientCookie(page);
  expect(cookie).toMatchObject({ httpOnly: true, secure: true, sameSite: "Lax", path: "/", value: token });
  expect(cookie?.expires).toBeCloseTo(Date.parse(expiresAt) / 1000, 0);
  const response = await page.goto("/patient/book?lang=en");
  expect(response?.headers()["cache-control"]).toBe("no-store");
  expect(response?.headers()["referrer-policy"]).toBe("no-referrer");
});

test("invalid and expired access return no patient data and clear the cookie", async ({ page }) => {
  await page.goto("/patient/access?link=forged");
  await expect(page.locator('p[role="alert"]')).toContainText("هذا الرابط غير صالح");
  expect(await patientCookie(page)).toBeUndefined();
  expect(page.url()).not.toContain("forged");
  await expect(page.getByText("Hello Fatima")).toHaveCount(0);

  await enter(page);
  await control("/__mock/expire");
  await page.goto(`/patient/access?link=${token}`);
  await expect(page.locator('p[role="alert"]')).toContainText("انتهت صلاحية هذا الرابط");
  expect(await patientCookie(page)).toBeUndefined();
  await expect(page.getByText("Your colonoscopy appointment")).toHaveCount(0);
});

test("Arabic is the default language", async ({ page }) => {
  await open(page, "notice", "ar");
  await page.goto("/patient/notice");
  await expect(page.locator('div[lang="ar"]')).toHaveAttribute("dir", "rtl");
  await expect(page.locator("h1")).toHaveText("نتيجة فحصكِ تحتاج فحصًا آخر");
});

test("notice: switch language, read rights, say who calls whom", async ({ page }) => {
  await open(page, "notice", "ar");
  await page.getByRole("button", { name: /اللغة/ }).click();
  const sheet = page.getByRole("dialog", { name: "اللغة" });
  await expect(sheet.getByRole("link")).toHaveCount(4);
  await expect(sheet.getByRole("link", { name: "العربية" })).toHaveAttribute("aria-current", "true");
  await sheet.getByRole("link", { name: "English" }).click();
  await expect(page).toHaveURL(/\/patient\/notice\?lang=en$/);
  await expect(page.locator("h1")).toHaveText("Your test result needs one more check");
  const card = page.locator("section", { has: page.getByRole("heading", { name: "Next step" }) });
  await expect(card).toContainText("Nurse Hessa will call you within 2 working days");
  await expect(card).toContainText("You can also call her: 02 555 0142");
  const rightsRow = page.getByRole("button", { name: "Your rights" });
  await rightsRow.click();
  await expect(page.getByRole("dialog", { name: "Your rights" }).getByRole("listitem")).toHaveCount(3);
  await expectNoSeriousAxe(page);
});

test("teach-back and grant persist exact events; opt-out survives reload", async ({ page }) => {
  await open(page, "check");
  const initialHead = (await journey()).head;
  await page.getByRole("button", { name: /Another blood test/ }).click();
  await expect(page.locator("h1")).toHaveText("Let us explain it another way");
  expect(await bodies()).toHaveLength(0);
  await page.getByRole("button", { name: "Ask me again" }).click();
  await page.getByRole("button", { name: /small camera/ }).click();
  await expect(page.locator("h1")).toHaveText("May we send you reminders by SMS?");
  expect((await bodies())[0]).toMatchObject({
    kind: "teach_back_completed",
    expected_head: initialHead,
    data: { question_id: "crc-next-step-v1", answer_id: "colonoscopy" },
  });
  await page.getByRole("button", { name: "Allow messages" }).click();
  await expect(page.locator("h1")).toHaveText("Thank you, Fatima");
  expect((await bodies())[1]).toMatchObject({ kind: "contact_permission_changed", data: { permission: "granted" } });
  await expect(page.getByText("Your permission for SMS reminders is recorded. No SMS has been sent.")).toBeVisible();
  await page.reload();
  await expect(page.locator("h1")).toHaveText("Thank you, Fatima");
  await page.getByRole("button", { name: "Stop text messages" }).click();
  await expect(page.getByText("Text message permission is stopped. No SMS has been sent.")).toBeVisible();
  expect((await bodies())[2]).toMatchObject({ kind: "contact_permission_changed", data: { permission: "opted_out" } });
  await page.reload();
  await expect(page.getByRole("button", { name: "Stop text messages" })).toHaveCount(0);
  await expect(page.getByText("Text message permission is stopped. No SMS has been sent.")).toBeVisible();
});

test("declining SMS is saved without promising a message", async ({ page }) => {
  await open(page, "check");
  await page.getByRole("button", { name: /small camera/ }).click();
  await page.getByRole("button", { name: "Not now" }).click();
  await expect(page.getByText(/We will not send you messages/)).toBeVisible();
  await page.reload();
  await expect(page.locator("h1")).toHaveText("Thank you, Fatima");
  await expect(page.getByText(/We will not send you messages/)).toBeVisible();
});

test("slot preference and preparation changes persist both true and false", async ({ page }) => {
  await open(page, "book");
  await expect(page.getByTestId("slot-date")).toHaveText("Friday 9 October");
  await page.getByRole("button", { name: "Another time" }).click();
  const others = page.getByRole("group", { name: "Other times" }).getByRole("button");
  await others.nth(1).click();
  await expect(page.getByTestId("slot-date")).toHaveText("Monday 12 October");
  await page.getByRole("button", { name: "Save this preferred time" }).click();
  await expect(page.getByText("Your preferred time is saved for coordinator review — simulated only; no appointment is reserved.")).toBeVisible();
  await expect(page.getByText("Your appointment is confirmed")).toHaveCount(0);
  expect((await bodies())[0]).toMatchObject({
    kind: "slot_selected",
    data: { slot_id: "skmc-endo:2026-10-12:13:00", offers_version: (await journey()).slot_selection.offers_version },
  });
  const boxes = page.getByRole("checkbox");
  await boxes.first().click();
  await expect(boxes.first()).toHaveAttribute("aria-checked", "true");
  await page.reload();
  await expect(page.getByTestId("slot-date")).toHaveText("Monday 12 October");
  await expect(page.getByRole("checkbox").first()).toHaveAttribute("aria-checked", "true");
  await page.getByRole("checkbox").first().click();
  await expect(page.getByRole("checkbox").first()).toHaveAttribute("aria-checked", "false");
  expect((await bodies())[2]).toMatchObject({ kind: "preparation_changed", data: { item_id: "clear_fluids_and_prep", completed: false } });
  await page.reload();
  await expect(page.getByRole("checkbox").first()).toHaveAttribute("aria-checked", "false");
});

test("network retry preserves request_id and never shows an unsaved advance", async ({ page }) => {
  await open(page, "check");
  await control("/__mock/fail-next", "network");
  await page.getByRole("button", { name: /small camera/ }).click();
  await expect(page.getByTestId("patient-failed")).toBeVisible();
  await expect(page.locator("h1")).not.toHaveText("May we send you reminders by SMS?");
  const first = (await bodies())[0] as { request_id: string };
  await page.getByRole("button", { name: "Try again" }).click();
  await expect.poll(async () => (await bodies()).length).toBe(2);
  expect(((await bodies())[1] as { request_id: string }).request_id).toBe(first.request_id);
  await expect(page.locator("h1")).toHaveText("May we send you reminders by SMS?");
});

test("server failure, stale head and expiry are announced without a saved state", async ({ page }) => {
  await open(page, "check");
  await control("/__mock/fail-next", "500");
  await page.getByRole("button", { name: /small camera/ }).click();
  await expect(page.getByTestId("patient-failed")).toBeVisible();
  await expect(page.getByRole("heading", { name: "What is the next step after your test result?" })).toBeVisible();
  await expectNoSeriousAxe(page);
  await page.getByRole("button", { name: "Try again" }).click();
  await expect(page.locator("h1")).toHaveText("May we send you reminders by SMS?");

  await control("/__mock/stale");
  await page.getByRole("button", { name: "Allow messages" }).click();
  await expect(page.getByTestId("patient-conflict")).toContainText("Review the latest details");
  await expect(page.locator("h1")).toHaveText("May we send you reminders by SMS?");
  await expect(page.getByText("Saved.")).toHaveCount(0);

  await control("/__mock/expire");
  await page.getByRole("button", { name: "Allow messages" }).click();
  await expect(page.getByTestId("patient-expired")).toContainText("This patient link has expired");
  await expect(page.locator("h1")).toHaveText("May we send you reminders by SMS?");
});

test("an unreadable 201 is refetched before success is shown", async ({ page }) => {
  await open(page, "check");
  await control("/__mock/fail-next", "empty201");
  await page.getByRole("button", { name: /small camera/ }).click();
  await expect(page.locator("h1")).toHaveText("May we send you reminders by SMS?");
  await expect(page.getByTestId("patient-saved")).toBeVisible();
  expect(await bodies()).toHaveLength(1);
  expect((await journey()).teach_back.status).toBe("completed");
});

test("controls expose busy state and keep keyboard focus movement", async ({ page }) => {
  await page.route("**/patient/journey", async (route) => {
    if (route.request().method() === "POST") await new Promise((resolve) => setTimeout(resolve, 350));
    await route.continue();
  });
  await open(page, "check");
  const right = page.getByRole("button", { name: /small camera/ });
  await right.focus();
  await right.press("Enter");
  await expect(page.locator("main")).toHaveAttribute("aria-busy", "true");
  await expect(page.getByRole("button", { name: /small camera/ })).toBeDisabled();
  await expect(page.locator("h1")).toHaveText("May we send you reminders by SMS?");
  await expect(page.locator("h1")).toBeFocused();
});

test("language persists across the three phones", async ({ page }) => {
  await open(page, "notice", "ur");
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
    await open(page, "notice", "ar");
    const box = await page.locator("main").boundingBox();
    expect(box && Math.round(box.x + box.width / 2)).toBe(720);
    expect(box?.width).toBeLessThanOrEqual(390);
  });
});
