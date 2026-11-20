import AxeBuilder from "@axe-core/playwright";
import { expect as baseExpect, test } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Fixtures mode: a clinician asks a specialist from the case page. The consult shows on the case's activity timeline with
 * a named owner and a reply-by date; once the reply is recorded it shows on Today until read. Kept in this browser's
 * sessionStorage (components/case/consult-store.ts).
 */
const expect = baseExpect.configure({ timeout: 20_000 });
test.describe.configure({ timeout: 60_000 });

const ID = fixtures.fixtures.false_closure_trap.id;
const QUESTION = "Is a repeat colonoscopy needed within 6 weeks?";

test("Ask a specialist: the consult appears on the case timeline, and its reply on Today", async ({ page }) => {
  await page.goto(`/app/patients/${encodeURIComponent(ID)}`);
  await page.getByTestId("ask-specialist").click();
  const dialog = page.getByRole("dialog", { name: "Ask a specialist" });
  await expect(dialog).toContainText("Dr Layla Haddad replies by");

  // An empty question is refused.
  await dialog.getByRole("button", { name: "Send question" }).click();
  await expect(dialog.getByRole("alert")).toHaveText("Write your question.");

  await dialog.getByLabel("Specialty").selectOption("pathology");
  await expect(dialog).toContainText("Dr Omar Al Kaabi replies by");
  await dialog.getByLabel("Question").fill(QUESTION);
  await dialog.getByRole("button", { name: "Send question" }).click();
  await expect(dialog).toBeHidden();

  const timeline = page.getByRole("list", { name: "Timeline" });
  const row = timeline.getByTestId("consult-row");
  await expect(row).toHaveCount(1);
  await expect(row).toContainText("Asked Pathology");
  await expect(row).toContainText(QUESTION);
  await expect(row).toContainText("Waiting for reply");
  await expect(row).toContainText(/Dr Omar Al Kaabi · reply by \d{1,2} [A-Z][a-z]{2} \d{4}/);

  const axe = await new AxeBuilder({ page }).analyze();
  expect(axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical").map((v) => v.id)).toEqual([]);

  // Waiting consults stay off Today; an answered one shows until it is read.
  await page.goto("/app/today");
  await expect(page.getByTestId("today-sheet")).toBeVisible();
  await expect(page.getByTestId("consults-due")).toHaveCount(0);

  await page.goto(`/app/patients/${encodeURIComponent(ID)}`);
  await row.getByLabel("Specialist's reply").fill("Yes, book within 6 weeks.");
  await row.getByRole("button", { name: "Record reply" }).click();
  await expect(row).toContainText("Answered");
  await expect(row.getByTestId("consult-reply")).toContainText("Yes, book within 6 weeks.");

  await page.goto("/app/today");
  const due = page.getByTestId("consults-due");
  await expect(due).toContainText("Answered");
  await expect(due).toContainText("Pathology · Dr Omar Al Kaabi · reply to read");
});
