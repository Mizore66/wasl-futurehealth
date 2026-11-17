import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { randomUUID } from "node:crypto";
import { ELSEWHERE_COPY } from "@/components/patient/elsewhere-copy";

// Issue #177: a patient reports the procedure was done elsewhere. Against the patient mock API (e2e/patient-mock-api.ts),
// which mirrors pipeline/api_patient_elsewhere.go; the Go contract itself is tested in api_patient_elsewhere_test.go.
const MOCK = `http://127.0.0.1:${process.env.PATIENT_MOCK_PORT ?? 31987}`;
let token = "";
test.describe.configure({ mode: "serial" });
test.use({ viewport: { width: 390, height: 844 } });
test.beforeEach(async () => {
  token = randomUUID();
  await fetch(`${MOCK}/__mock/links`, { method: "POST", body: JSON.stringify({ token }) });
});

async function bodies(): Promise<Record<string, unknown>[]> {
  return (await fetch(`${MOCK}/__mock/requests?token=${token}`)).json();
}

async function enter(page: Page, lang = "en") {
  await page.goto(`/patient/access?link=${token}`);
  await expect(page).toHaveURL(/\/patient\/notice$/);
  await page.goto(`/patient/notice?lang=${lang}`);
}

for (const lang of ["en", "ar"] as const) {
  test(`completed elsewhere: an accessible structured report, a receipt that survives reload, no closure claim (${lang})`, async ({ page }) => {
    const t = ELSEWHERE_COPY[lang];
    await enter(page, lang);
    await page.getByRole("button", { name: t.open }).click();
    const form = page.getByRole("form", { name: t.title });
    await expect(form.getByRole("textbox", { name: t.procedure })).toHaveValue("colonoscopy");
    await form.getByLabel(t.performed).fill("2026-08");
    await form.getByRole("textbox", { name: t.provider }).fill("Synthetic Private Hospital");
    await form.getByRole("textbox", { name: `${t.reportRef} (${t.optional})` }).fill("SYN-RPT-9");
    const axe = await new AxeBuilder({ page }).include("form").analyze();
    expect(axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical").map((v) => v.id)).toEqual([]);
    await form.getByRole("button", { name: t.submit }).click();

    const status = page.getByTestId("elsewhere-status");
    await expect(status).toContainText(t.pending.split("{provider}")[0].trim());
    await expect(status).toContainText("Synthetic Private Hospital");
    await expect(status).toContainText(t.notResult);
    await expect(page.getByRole("button", { name: t.open })).toHaveCount(0);
    const sent = await bodies();
    expect(sent).toHaveLength(1);
    expect(Object.keys(sent[0]).sort()).toEqual(["performed", "procedure", "provider", "report_ref", "request_id"]);
    expect(sent[0]).toMatchObject({ procedure: "colonoscopy", performed: "2026-08", provider: "Synthetic Private Hospital", report_ref: "SYN-RPT-9" });

    await page.reload();
    await expect(page.getByTestId("elsewhere-status")).toContainText("Synthetic Private Hospital");
    await expect(page.getByRole("button", { name: t.open })).toHaveCount(0);
  });
}

test("completed elsewhere: a not-found retrieval resumes follow-up and the patient can report again", async ({ page }) => {
  const t = ELSEWHERE_COPY.en;
  await enter(page);
  await page.getByRole("button", { name: t.open }).click();
  const form = page.getByRole("form", { name: t.title });
  await form.getByLabel(t.performed).fill("2026-07");
  await form.getByRole("textbox", { name: t.provider }).fill("Synthetic Day Clinic");
  await form.getByRole("button", { name: t.submit }).click();
  await expect(page.getByTestId("elsewhere-status")).toContainText("Synthetic Day Clinic");

  await fetch(`${MOCK}/__mock/elsewhere-outcome`, { method: "POST", body: JSON.stringify({ token, mode: "not_found" }) });
  await page.reload();
  await expect(page.getByTestId("elsewhere-status")).toHaveText(new RegExp(t.resumed.not_found.replace("{provider}", "Synthetic Day Clinic")));
  await expect(page.getByRole("button", { name: t.open })).toBeVisible();
});

test("completed elsewhere: a required field missing sends nothing", async ({ page }) => {
  const t = ELSEWHERE_COPY.en;
  await enter(page);
  await page.getByRole("button", { name: t.open }).click();
  const form = page.getByRole("form", { name: t.title });
  await form.getByLabel(t.performed).fill("2026-07");
  await form.getByRole("button", { name: t.submit }).click();
  await expect(form.getByRole("textbox", { name: t.provider })).toBeFocused();
  expect(await bodies()).toHaveLength(0);
});
