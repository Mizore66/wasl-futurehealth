import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import type { PatientJourney, PatientMutation } from "@/lib/api/patient-schema";
import { DICTS } from "@/components/patient/i18n";

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
  // Chromium derives expiry from its receipt clock: CI measured 1.004s after the whole-second Expires value.
  // Compare cookie Unix seconds on both sides, allowing only that one-second serialization boundary.
  const expectedExpiry = Math.floor(Date.parse(expiresAt) / 1000);
  const actualExpiry = Math.floor(cookie?.expires ?? 0);
  expect(Math.abs(actualExpiry - expectedExpiry)).toBeLessThanOrEqual(1);
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

const SHORT_VIEWPORTS = [
  { name: "short landscape", width: 844, height: 390, closeBy: "keyboard" },
  { name: "high zoom", width: 390, height: 280, closeBy: "pointer" },
] as const;

async function expectSheetReachable(page: Page, dialog: Locator, parts: Locator[], width: number, height: number) {
  await dialog.evaluate((el) => Promise.all(el.getAnimations().map((a) => a.finished)));
  const box = await dialog.boundingBox();
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.y + box!.height).toBeLessThanOrEqual(height + 0.5);
  for (const part of parts) {
    await part.scrollIntoViewIfNeeded();
    const partBox = await part.boundingBox();
    expect(partBox!.y).toBeGreaterThanOrEqual(box!.y - 0.5);
    expect(partBox!.y + partBox!.height).toBeLessThanOrEqual(box!.y + box!.height + 0.5);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  expect(await dialog.evaluate((el) => el.scrollWidth - el.clientWidth)).toBe(0);
}

for (const viewport of SHORT_VIEWPORTS) {
  for (const { lang } of LANGS) {
    test(`rights and language sheets stay reachable at ${viewport.name}, ${lang}`, async ({ page }) => {
      const t = DICTS[lang];
      await open(page, "notice", lang);
      await page.setViewportSize({ width: viewport.width, height: viewport.height });
      const phoneWidth = await page.locator(`div[lang="${lang}"]`).evaluate((el) => el.getBoundingClientRect().width);

      const trigger = page.getByRole("button", { name: t.notice.rights });
      await trigger.click();
      const rights = page.getByRole("dialog", { name: t.notice.rights });
      const close = rights.getByRole("button", { name: t.close });
      await expectSheetReachable(
        page,
        rights,
        [rights.getByRole("heading", { name: t.notice.rights }), ...(await rights.getByRole("listitem").all()), rights.getByText(t.notice.rightsNote), close],
        viewport.width,
        viewport.height,
      );
      await expect(rights.getByRole("listitem")).toHaveCount(3);
      await page.keyboard.press("Tab");
      expect(await rights.evaluate((el) => el.contains(document.activeElement))).toBe(true);
      if (viewport.closeBy === "keyboard") {
        await close.focus();
        await page.keyboard.press("Enter");
      } else {
        await close.click();
      }
      await expect(rights).toHaveCount(0);
      await expect(trigger).toBeFocused();

      await page.getByRole("button", { name: `${t.langTitle}: ${t.langName}` }).click();
      const sheet = page.getByRole("dialog", { name: t.langTitle });
      await expectSheetReachable(page, sheet, [...(await sheet.getByRole("link").all()), sheet.getByRole("button", { name: t.close })], viewport.width, viewport.height);
      await expect(sheet.getByRole("link")).toHaveCount(LANGS.length);
      await sheet.getByRole("button", { name: t.close }).click();
      await expect(sheet).toHaveCount(0);
      expect(await page.locator(`div[lang="${lang}"]`).evaluate((el) => el.getBoundingClientRect().width)).toBe(phoneWidth);
    });
  }
}

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

test("a conflict refresh replaces the confirmed slot changed in another tab", async ({ page }) => {
  await open(page, "book");
  await page.getByRole("button", { name: "Save this preferred time" }).click();
  await expect(page.getByTestId("slot-date")).toHaveText("Friday 9 October");
  await expect(page.getByText("Your preferred time is saved for coordinator review — simulated only; no appointment is reserved.")).toBeVisible();

  const otherTab = await page.context().newPage();
  await open(otherTab, "book");
  await otherTab.getByRole("button", { name: "Change appointment" }).click();
  await otherTab.getByRole("group", { name: "Other times" }).getByRole("button").nth(1).click();
  await expect(otherTab.getByTestId("slot-date")).toHaveText("Monday 12 October");
  await otherTab.getByRole("button", { name: "Save this preferred time" }).click();
  await expect(otherTab.getByText("Your preferred time is saved for coordinator review — simulated only; no appointment is reserved.")).toBeVisible();

  await page.getByRole("checkbox").first().click();
  await expect(page.getByTestId("patient-conflict")).toContainText("Review the latest details");
  await expect(page.getByText("Your preferred time is saved for coordinator review — simulated only; no appointment is reserved.")).toBeVisible();
  await expect(page.getByTestId("slot-date")).toHaveText("Monday 12 October");
  await expect(page.getByTestId("slot-time")).toHaveText("at 1:00 pm");
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

for (const { lang } of LANGS) {
  test(`rights: verified contest shows a receipt and pending review after reload (${lang})`, async ({ page }) => {
    const t = DICTS[lang];
    await open(page, "notice", lang);
    await page.getByRole("button", { name: t.notice.rights }).click();
    const sheet = page.getByRole("dialog", { name: t.notice.rights });
    await sheet.getByRole("button", { name: t.rights.contest }).click();
    await sheet.getByRole("textbox", { name: t.rights.reasonLabel }).fill("This is not my record");
    await sheet.getByRole("button", { name: t.rights.getCode }).click();
    const status = await sheet.getByRole("status").innerText();
    const code = status.match(/\d{6}/)![0];
    await sheet.getByRole("textbox", { name: t.rights.codeLabel }).fill(code);
    await sheet.getByRole("button", { name: t.rights.submit }).click();
    const list = sheet.getByRole("region", { name: t.rights.listHead });
    await expect(list).toContainText(t.rights.held);
    await expect(list).toContainText(t.rights.receipt);
    await expectNoSeriousAxe(page);

    await page.reload();
    await page.getByRole("button", { name: t.notice.rights }).click();
    const reopened = page.getByRole("dialog", { name: t.notice.rights }).getByRole("region", { name: t.rights.listHead });
    await expect(reopened).toContainText(t.rights.pending.split("{due}")[0].trim());
  });
}

test("rights: an explanation request returns this case's provenance; a wrong code records nothing", async ({ page }) => {
  await open(page, "notice", "en");
  await page.getByRole("button", { name: "Your rights" }).click();
  const sheet = page.getByRole("dialog", { name: "Your rights" });
  await sheet.getByRole("button", { name: "Ask for a clearer explanation" }).click();
  await sheet.getByRole("button", { name: "Get a one-time code" }).click();
  await sheet.getByRole("textbox", { name: "One-time code" }).fill("000000");
  await sheet.getByRole("button", { name: "Send request" }).click();
  await expect(sheet.getByRole("alert")).toHaveText(DICTS.en.rights.failed);
  const code = (await sheet.getByRole("status").innerText()).match(/\d{6}/)![0];
  await sheet.getByRole("textbox", { name: "One-time code" }).fill(code);
  await sheet.getByRole("button", { name: "Send request" }).click();
  const explanation = sheet.getByRole("region", { name: "How this decision was made" });
  await expect(explanation).toContainText("ahs/DiagnosticReport/COLO-1");
  await expect(sheet.getByRole("region", { name: "Your requests" })).toContainText("Explanation ready.");
});
