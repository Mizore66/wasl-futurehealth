import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import type { PatientJourney, PatientMutation } from "@/lib/api/patient-schema";
import { DICTS, PATIENT_LANGS, REVIEW_LANGS, type Lang } from "@/components/patient/i18n";
import { OUTCOME_GATE_COPY } from "@/components/patient/outcome-copy";

const MOCK = `http://127.0.0.1:${process.env.PATIENT_MOCK_PORT ?? 31987}`;
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

type Surface = "patient" | "review";
const BASE: Record<Surface, string> = { patient: "/patient", review: "/translation-review" };
/**
 * Patient routes serve only patient-approved dictionaries (#133). Unreviewed drafts keep their layout, RTL and
 * accessibility coverage on the internal translation-review preview, never on a patient route.
 */
const PHONES: { lang: Lang; dir: "rtl" | "ltr"; surface: Surface }[] = [
  ...PATIENT_LANGS.map((lang) => ({ lang, dir: DICTS[lang].dir, surface: "patient" as const })),
  ...REVIEW_LANGS.map((lang) => ({ lang, dir: DICTS[lang].dir, surface: "review" as const })),
];
const STEPS = ["notice", "check", "book"] as const;
const UNREVIEWED_WARNING = "It has not been reviewed by a qualified medical translator and is not approved for patients.";

/** Medical strings that differ from English in each draft: teach-back, consent, medication and preparation. */
function draftStrings(lang: Lang): string[] {
  const pick = (t: (typeof DICTS)[Lang]) => [t.notice.title, t.notice.rightsItems[2], t.check.question, t.check.right, t.check.consentBody, t.book.warn, ...t.book.prep, t.langName];
  const en = pick(DICTS.en);
  return pick(DICTS[lang]).filter((s, i) => s !== en[i]);
}

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

async function open(page: Page, step: string, lang = "en", surface: Surface = "patient") {
  await enter(page);
  await page.goto(`${BASE[surface]}/${step}?lang=${lang}`);
}

function patientCookie(page: Page) {
  return page.context().cookies().then((cookies) => cookies.find((cookie) => cookie.name === "__Host-wasl_patient_link"));
}

for (const step of STEPS) {
  for (const { lang, dir, surface } of PHONES) {
    test(`${BASE[surface]}/${step}?lang=${lang} uses a live projection, ${dir}, axe clean`, async ({ page }) => {
      await open(page, step, lang, surface);
      await expect(page.locator(`div[lang="${lang}"]`)).toHaveAttribute("dir", dir);
      await expect(page.getByTestId("unreviewed-translation")).toHaveCount(surface === "review" ? 1 : 0);
      if (surface === "review") await expect(page.getByTestId("unreviewed-translation")).toContainText(UNREVIEWED_WARNING);
      await expect(page.locator("h1")).toHaveCount(1);
      await expect(page.locator("h1")).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
      await expect(page.locator("[data-honesty=built]")).toHaveText("Synthetic · server saved");
      await expect(page.locator("[data-honesty=designed]")).toHaveCount(0);
      const nav = page.getByRole("navigation", { name: "Demo steps" });
      for (const s of STEPS) {
        await expect(nav.locator(`a[href^="${BASE[surface]}/${s}?lang=${lang}"]`)).toHaveCount(1);
        await expect(nav.locator(`a[href^="${BASE[surface]}/${s}"]`)).not.toHaveAttribute("href", /consent/);
      }
      await page.evaluate(() => document.fonts.ready);
      await page.screenshot({ path: `test-results/screens/${surface}_${step}_${lang}.png` });
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
  await expect(page.locator('p[role="alert"]')).toContainText(DICTS.en.common.invalidLink);
  expect(await patientCookie(page)).toBeUndefined();
  expect(page.url()).not.toContain("forged");
  await expect(page.getByText("Hello Fatima")).toHaveCount(0);

  await enter(page);
  await control("/__mock/expire");
  await page.goto(`/patient/access?link=${token}`);
  await expect(page.locator('p[role="alert"]')).toContainText(DICTS.en.common.expiredLink);
  expect(await patientCookie(page)).toBeUndefined();
  await expect(page.getByText("Your colonoscopy appointment")).toHaveCount(0);
});

test("a reviewed language, not unreviewed Arabic, is the patient default", async ({ page }) => {
  await enter(page);
  await page.goto("/patient/notice");
  await expect(page.locator('div[lang="en"]')).toHaveAttribute("dir", "ltr");
  await expect(page.locator("h1")).toHaveText(DICTS.en.notice.title);
  await expect(page.locator('div[lang="ar"]')).toHaveCount(0);
});

// #133: a direct, missing or invalid `lang` and every propagated step link stay on a patient-approved dictionary.
for (const lang of [...REVIEW_LANGS, "xx", ""]) {
  for (const step of STEPS) {
    test(`/patient/${step}?lang=${lang || "(empty)"} serves no unreviewed dictionary`, async ({ page }) => {
      await open(page, step, lang);
      await expect(page.locator('div[lang="en"]')).toHaveAttribute("dir", "ltr");
      await expect(page.getByTestId("unreviewed-translation")).toHaveCount(0);
      const nav = page.getByRole("navigation", { name: "Demo steps" });
      for (const s of STEPS) await expect(nav.locator(`a[href="/patient/${s}?lang=en"]`)).toHaveCount(1);
      await page.getByRole("button", { name: `${DICTS.en.langTitle}: ${DICTS.en.langName}` }).click();
      const sheet = page.getByRole("dialog", { name: DICTS.en.langTitle });
      await expect(sheet.getByRole("link")).toHaveCount(PATIENT_LANGS.length);
      for (const draft of REVIEW_LANGS) await expect(sheet.getByRole("link", { name: DICTS[draft].langName })).toHaveCount(0);
      await sheet.getByRole("button", { name: DICTS.en.close }).click();
      if (step === "notice") await page.getByRole("button", { name: DICTS.en.notice.rights }).click();
      const body = await page.locator("body").innerText();
      for (const draft of REVIEW_LANGS) for (const s of draftStrings(draft)) expect(body, `${draft} copy on a patient route`).not.toContain(s);
    });
  }
}

test("unreviewed drafts never reach the rendered patient journey, through teach-back, consent and preparation", async ({ page }) => {
  for (const draft of REVIEW_LANGS) {
    await page.context().clearCookies();
    token = randomUUID();
    await fetch(`${MOCK}/__mock/links`, { method: "POST", body: JSON.stringify({ token }) });
    await open(page, "check", draft);
    await page.getByRole("button", { name: DICTS.en.check.wrong }).click();
    await page.getByRole("button", { name: "Ask me again" }).click();
    await page.getByRole("button", { name: DICTS.en.check.right }).click();
    await expect(page.locator("h1")).toHaveText(DICTS.en.check.consentTitle);
    let body = await page.locator("body").innerText();
    for (const s of draftStrings(draft)) expect(body).not.toContain(s);
    await page.goto(`/patient/book?lang=${draft}`);
    await expect(page.getByRole("heading", { name: DICTS.en.book.prepHead })).toBeVisible();
    body = await page.locator("body").innerText();
    for (const s of draftStrings(draft)) expect(body).not.toContain(s);
    await expect(page.getByText(DICTS.en.book.warn)).toBeVisible();
  }
});

test("translation review preview: separate path, unreviewed warning on every step, drafts only", async ({ page }) => {
  for (const lang of REVIEW_LANGS) {
    for (const step of [...STEPS, "outcome"]) {
      await open(page, step, lang, "review");
      await expect(page).toHaveURL(new RegExp(`/translation-review/${step}\\?lang=${lang}$`));
      await expect(page.getByTestId("unreviewed-translation")).toContainText("Not for patients.");
      await expect(page.getByTestId("unreviewed-translation")).toContainText(UNREVIEWED_WARNING);
    }
  }
  // English is patient copy, never a preview of itself; the preview sheet lists only drafts.
  await page.goto("/translation-review/notice?lang=en");
  await expect(page.locator(`div[lang="${REVIEW_LANGS[0]}"]`)).toBeVisible();
  const t = DICTS[REVIEW_LANGS[0]];
  await page.getByRole("button", { name: `${t.langTitle}: ${t.langName}` }).click();
  const sheet = page.getByRole("dialog", { name: t.langTitle });
  await expect(sheet.getByRole("link")).toHaveCount(REVIEW_LANGS.length);
  await expect(sheet.getByRole("link", { name: DICTS.en.langName })).toHaveCount(0);
  for (const link of await sheet.getByRole("link").all()) await expect(link).toHaveAttribute("href", /^\/translation-review\//);
  // #157: even the drafts no longer carry the withdrawn unchanged-care promise.
  for (const lang of REVIEW_LANGS) {
    await open(page, "notice", lang, "review");
    await page.getByRole("button", { name: DICTS[lang].notice.rights }).click();
    const rights = page.getByRole("dialog", { name: DICTS[lang].notice.rights });
    await expect(rights.getByRole("listitem").nth(2)).toHaveText(DICTS[lang].notice.rightsItems[2]);
    await expect(rights).not.toContainText(/ولن يؤثر ذلك في رعايتكِ|اس سے آپ کی دیکھ بھال پر اثر نہیں پڑے گا|Hindi magbabago ang pangangalaga/);
  }
  const response = await page.goto("/translation-review/notice?lang=ar&error=invalid");
  expect(response?.headers()["cache-control"]).toBe("no-store");
  expect(response?.headers()["x-robots-tag"]).toBe("noindex, nofollow");
  await expect(page.getByTestId("unreviewed-translation")).toContainText(UNREVIEWED_WARNING);
});

test("notice: read rights, say who calls whom", async ({ page }) => {
  await open(page, "notice", "en");
  await page.getByRole("button", { name: "Language: English" }).click();
  const sheet = page.getByRole("dialog", { name: "Language" });
  await expect(sheet.getByRole("link")).toHaveCount(PATIENT_LANGS.length);
  await expect(sheet.getByRole("link", { name: "English" })).toHaveAttribute("aria-current", "true");
  await sheet.getByRole("link", { name: "English" }).click();
  await expect(page).toHaveURL(/\/patient\/notice\?lang=en$/);
  await expect(page.locator("h1")).toHaveText("Your test result needs one more check");
  const card = page.locator("section", { has: page.getByRole("heading", { name: "Next step" }) });
  await expect(card).toContainText("Nurse Hessa will call you within 2 working days");
  await expect(card).toContainText("book a colonoscopy");
  await expect(card).toContainText("You can also call her: 02 555 0142");
  const rightsRow = page.getByRole("button", { name: "Your rights" });
  await rightsRow.click();
  const rights = page.getByRole("dialog", { name: "Your rights" });
  await expect(rights.getByRole("listitem")).toHaveCount(3);
  // #157: the right to decline stays explicit; declining is never promised to leave care unchanged.
  await expect(rights).toContainText("You can say no to follow-up or question any decision.");
  await expect(rights).toContainText("you will still be treated with respect and can still get other care you need");
  await expect(rights).toContainText("Talk to your doctor about what saying no could mean for you and what other choices you have.");
  await expect(rights).not.toContainText(/care will not change/i);
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
  for (const { lang, surface } of PHONES) {
    test(`rights and language sheets stay reachable at ${viewport.name}, ${lang}`, async ({ page }) => {
      const t = DICTS[lang];
      await open(page, "notice", lang, surface);
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
      await expect(sheet.getByRole("link")).toHaveCount(surface === "patient" ? PATIENT_LANGS.length : REVIEW_LANGS.length);
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
  expect(await bodies()).toEqual([expect.objectContaining({ kind: "teach_back_answered", expected_head: initialHead, data: { question_id: "crc-next-step-v1", answer_id: "blood_test" } })]);
  const receipt = page.getByRole("status").filter({ hasText: /Nurse Hessa will call you by/ });
  await expect(receipt).toContainText("This is a demo: no call is placed.");
  const callback = (await journey()).teach_back.callback!;
  await expect(receipt).toContainText(`Reference ${callback.id.slice(0, 8)}`);
  await page.reload();
  await expect(page.locator("h1")).toHaveText("Let us explain it another way");
  await expect(receipt).toContainText(`Reference ${callback.id.slice(0, 8)}`);
  await page.getByRole("button", { name: "Ask me again" }).click();
  await page.getByRole("button", { name: "I am not sure" }).click();
  await expect(receipt).toContainText(`Reference ${callback.id.slice(0, 8)}`);
  expect((await journey()).teach_back).toMatchObject({ status: "callback", understanding: "needs_human", attempts: 2, callback: { id: callback.id, status: "open" } });
  await page.getByRole("button", { name: "Ask me again" }).click();
  await page.getByRole("button", { name: /small camera/ }).click();
  await expect(page.locator("h1")).toHaveText("May we send you reminders by SMS?");
  expect((await bodies())[2]).toMatchObject({ kind: "teach_back_completed", data: { question_id: "crc-next-step-v1", answer_id: "colonoscopy" } });
  await page.getByRole("button", { name: "Allow messages" }).click();
  await expect(page.locator("h1")).toHaveText("Thank you, Fatima");
  expect((await bodies())[3]).toMatchObject({ kind: "contact_permission_changed", data: { permission: "granted" } });
  await expect(page.getByText("Your permission for SMS reminders is recorded. No SMS has been sent.")).toBeVisible();
  await page.reload();
  await expect(page.locator("h1")).toHaveText("Thank you, Fatima");
  await page.getByRole("button", { name: "Stop text messages" }).click();
  await expect(page.getByText("Text message permission is stopped. No SMS has been sent.")).toBeVisible();
  expect((await bodies())[4]).toMatchObject({ kind: "contact_permission_changed", data: { permission: "opted_out" } });
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

test("language persists across the three phones on the review preview", async ({ page }) => {
  await open(page, "notice", "ur", "review");
  const nav = page.getByRole("navigation", { name: "Demo steps" });
  await nav.getByRole("link", { name: "2 Check" }).click();
  await expect(page).toHaveURL(/\/translation-review\/check\?lang=ur$/);
  await expect(page.locator('div[lang="ur"]')).toHaveAttribute("dir", "rtl");
  await expect(page.getByTestId("unreviewed-translation")).toBeVisible();
  await nav.getByRole("link", { name: "3 Book" }).click();
  await expect(page).toHaveURL(/\/translation-review\/book\?lang=ur$/);
  await expect(page.getByTestId("slot-date")).toContainText("جمعہ");
  await expect(page.getByTestId("unreviewed-translation")).toBeVisible();
});

test.describe("desktop", () => {
  test.use({ viewport: { width: 1440, height: 900 } });
  test("centres a phone-width column", async ({ page }) => {
    await open(page, "notice", "en");
    const box = await page.locator("main").boundingBox();
    expect(box && Math.round(box.x + box.width / 2)).toBe(720);
    expect(box?.width).toBeLessThanOrEqual(390);
  });
});

for (const { lang, surface } of PHONES) {
  test(`rights: verified contest shows a receipt and pending review after reload (${lang})`, async ({ page }) => {
    const t = DICTS[lang];
    await open(page, "notice", lang, surface);
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

for (const { lang, dir, surface } of PHONES) {
  test(`outcome: approved content, next step and safety net are acknowledged once (${lang})`, async ({ page }) => {
    const t = DICTS[lang];
    await control("/__mock/handoff", "pending_approval");
    await open(page, "outcome", lang, surface);
    await expect(page.locator(`div[lang="${lang}"]`)).toHaveAttribute("dir", dir);
    await expect(page.getByRole("status")).toHaveText(t.outcome.pending);
    await expect(page.getByText(t.outcome.safetyNet)).toHaveCount(0);

    await control("/__mock/handoff", "pending_release");
    await page.reload();
    await expect(page.getByRole("status")).toHaveText(OUTCOME_GATE_COPY.pendingRelease);
    if (lang !== "en") await expect(page.getByRole("status")).toHaveAttribute("lang", "en");
    await expect(page.getByText(t.outcome.safetyNet)).toHaveCount(0);

    await control("/__mock/handoff", "released");
    await page.reload();
    await expect(page.locator("h1")).toHaveText(t.outcome.title);
    // The second identifier comes first: no outcome, next step or safety net before the one-time code is verified.
    await expect(page.getByText(OUTCOME_GATE_COPY.verifyIntro)).toBeVisible();
    await expect(page.getByRole("region", { name: t.outcome.safetyHead })).toHaveCount(0);
    await expect(page.getByText(t.outcome.simulated)).toBeVisible();
    await page.getByRole("button", { name: t.rights.getCode }).click();
    const code = (await page.getByRole("status").innerText()).match(/\d{6}/)![0];
    const show = page.getByRole("button", { name: OUTCOME_GATE_COPY.show });
    await expect(show).toBeDisabled();
    await page.getByRole("textbox", { name: t.rights.codeLabel }).fill(code);
    await show.click();
    await expect(page.getByRole("region", { name: t.outcome.nextHead })).toContainText(t.outcome.next.routine_screening);
    await expect(page.getByRole("region", { name: t.outcome.safetyHead })).toContainText(t.outcome.safetyNet);
    await page.getByRole("button", { name: t.outcome.acknowledge }).click();
    await expect(page.getByRole("status")).toHaveText(t.outcome.acknowledged);
    await expectNoSeriousAxe(page);

    await page.reload();
    await expect(page.getByRole("status")).toHaveText(t.outcome.acknowledged);
  });
}

test("outcome: a wrong one-time code shows nothing and is announced", async ({ page }) => {
  await control("/__mock/handoff", "released");
  await open(page, "outcome", "en");
  await page.getByRole("button", { name: DICTS.en.rights.getCode }).click();
  await page.getByRole("textbox", { name: DICTS.en.rights.codeLabel }).fill("000000");
  await page.getByRole("button", { name: OUTCOME_GATE_COPY.show }).click();
  await expect(page.locator("main").getByRole("alert")).toHaveText(DICTS.en.rights.failed);
  await expect(page.getByRole("region", { name: DICTS.en.outcome.safetyHead })).toHaveCount(0);
  await expect(page.getByRole("button", { name: DICTS.en.outcome.acknowledge })).toHaveCount(0);
});

test("outcome: held and suppressed consent show no content", async ({ page }) => {
  for (const [mode, line] of [["held_consent", DICTS.en.outcome.held], ["suppressed", DICTS.en.outcome.suppressed]] as const) {
    await control("/__mock/handoff", mode);
    await open(page, "outcome", "en");
    await expect(page.getByRole("status")).toHaveText(line);
    await expect(page.getByRole("region", { name: DICTS.en.outcome.safetyHead })).toHaveCount(0);
    await expect(page.getByRole("button", { name: DICTS.en.rights.getCode })).toHaveCount(0);
  }
});

test("a breast case keeps its own programme through steps, languages and forged parameters", async ({ page }) => {
  token = randomUUID();
  await fetch(`${MOCK}/__mock/links`, { method: "POST", body: JSON.stringify({ token, programme: "breast" }) });
  await enter(page);
  const crcWords = /colonoscopy|bowel|SKMC|Endoscopy|Fatima|Hessa|October/i;
  for (const [lang, surface] of [["en", "patient"], ["ar", "review"]] as const) {
    for (const step of STEPS) {
      await page.goto(`${BASE[surface]}/${step}?lang=${lang}&programme=crc&case=ahs/ServiceRequest/OTHER&next_step=colonoscopy`);
      await expect(page.locator("main")).toContainText(/breast clinic|What is the next step after your result\?/);
      await expect(page.locator("main")).not.toContainText(crcWords);
      const nav = page.getByRole("navigation", { name: "Demo steps" });
      await expect(nav.locator("a").first()).toHaveAttribute("href", `${BASE[surface]}/notice?lang=${lang}`);
    }
  }
  await expect(page.getByText("until a reviewed translation is ready")).toBeVisible();
  await page.goto("/patient/book?lang=en");
  await expect(page.getByTestId("no-offer")).toContainText("The care coordinator will contact you");
  await expect(page.getByTestId("slot-date")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Save this preferred time" })).toHaveCount(0);
  await expect(page.getByText("Before your appointment")).toHaveCount(0);

  // #157: the refusal right is programme-neutral and never promises unchanged care.
  await page.goto("/patient/notice?lang=en");
  await page.getByRole("button", { name: "Your rights" }).click();
  const rights = page.getByRole("dialog", { name: "Your rights" });
  await expect(rights).toContainText("You can say no to follow-up or question any decision.");
  await expect(rights).toContainText("Talk to your doctor about what saying no could mean for you");
  await expect(rights).not.toContainText(/care will not change|colonoscopy/i);
  await page.keyboard.press("Escape");

  await page.goto("/patient/check?lang=en");
  await page.getByRole("button", { name: /assessment appointment at the breast clinic/ }).click();
  await expect(page.locator("h1")).toHaveText("May we send you reminders by SMS?");
  expect(await bodies()).toEqual([expect.objectContaining({ kind: "teach_back_completed", data: { question_id: "breast-next-step-v1", answer_id: "recall" } })]);
  await expectNoSeriousAxe(page);
});

test("a cervical case books from its own simulated colposcopy slots, with no CRC slot or preparation", async ({ page }) => {
  token = randomUUID();
  await fetch(`${MOCK}/__mock/links`, { method: "POST", body: JSON.stringify({ token, programme: "cervical" }) });
  await enter(page);
  await page.goto("/patient/book?lang=en&programme=crc&procedure=colonoscopy");
  await expect(page.locator("h1")).toHaveText("Your colposcopy appointment");
  await expect(page.getByTestId("slot-date")).toHaveText(/Thursday\s+25\s+June/);
  await expect(page.getByTestId("slot-time")).toHaveCount(0);
  await expect(page.getByTestId("slot-place")).toContainText("Abu Dhabi city region");
  await expect(page.locator("main")).not.toContainText(/colonoscopy|bowel|SKMC|Endoscopy|Hessa|October|Before your appointment/i);

  await page.getByRole("button", { name: "Another time" }).click();
  await page.getByRole("button", { name: /1\s+July/ }).click();
  await page.getByRole("button", { name: "Save this preferred time" }).click();
  await expect(page.getByText(/saved for coordinator review/)).toBeVisible();
  await expect(page.getByTestId("slot-place")).toContainText("Al Dhafra region");
  expect((await bodies()).at(-1)).toEqual(expect.objectContaining({ kind: "slot_selected", data: expect.objectContaining({ slot_id: "sim:colposcopy:5:1" }) }));

  // Public requests cannot enable an unreviewed translation.
  await page.goto("/patient/book?lang=ar");
  await expect(page.locator('div[lang="en"]')).toHaveAttribute("dir", "ltr");
  await expect(page.locator('div[lang="ar"]')).toHaveCount(0);
  await expect(page.locator("h1")).toHaveText("Your colposcopy appointment");
  await expect(page.getByTestId("slot-date")).toBeVisible();
  await expect(page.getByTestId("slot-date")).toHaveText(/Wednesday\s+1\s+July/);
  await expect(page.getByTestId("slot-place")).toContainText("Al Dhafra region");
  await expect(page.locator("main")).not.toContainText(/colonoscopy|SKMC/i);
  await expectNoSeriousAxe(page);

  // Retain Arabic/RTL and explicit English-fallback coverage on the internal preview.
  await page.goto("/translation-review/book?lang=ar");
  await expect(page.locator('div[lang="ar"]')).toHaveAttribute("dir", "rtl");
  await expect(page.getByText(UNREVIEWED_WARNING)).toBeVisible();
  await expect(page.locator("h1")).toHaveAttribute("lang", "en");
  await expect(page.getByText("until a reviewed translation is ready")).toBeVisible();
  await expect(page.locator("h1")).toHaveAttribute("dir", "ltr");
  await expect(page.getByTestId("slot-date")).toBeVisible();
  await expect(page.getByTestId("slot-place")).toContainText("Al Dhafra region");
  await expect(page.locator("main")).not.toContainText(/colonoscopy|bowel|SKMC|Before your appointment/i);
  await expectNoSeriousAxe(page);
});
