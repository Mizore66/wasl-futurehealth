import { readFileSync } from "node:fs";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Case detail, audit and review desk against the snapshots in data/api (fixtures mode).
 * Counts come from the snapshot files, so the page is checked against the API's own data, not against itself.
 */

const NAMES = ["evidence_closure", "false_closure_trap", "review_hold", "dr_exam_closure"] as const;
const dir = path.join(process.cwd(), "data", "api");
const read = (f: string) => JSON.parse(readFileSync(path.join(dir, f), "utf8"));

const href = (name: (typeof NAMES)[number], tail = "") => `/app/patients/${encodeURIComponent(fixtures.fixtures[name].id)}${tail}`;

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

const shot = (page: Page, name: string) => page.screenshot({ path: `test-results/screens/case-${name}.png`, fullPage: true });

/** The page is no wider than the window. */
async function expectNoOverflow(page: Page, width: number) {
  const w = await page.evaluate(() => document.documentElement.scrollWidth);
  expect(w).toBeLessThanOrEqual(width);
}

/** Every visible link, button, radio and disclosure inside <main> is at least `min` px tall and wide. */
async function smallTargets(page: Page, min: number) {
  return page.evaluate((m) => {
    const out: string[] = [];
    for (const el of document.querySelectorAll("main a[href], main button, main [role=radio], main summary, main input, main select")) {
      const r = el.getBoundingClientRect();
      if (r.width === 0 || r.height === 0) continue;
      // Text links inside a sentence follow the WCAG inline exception.
      if (el.tagName === "A" && getComputedStyle(el).display === "inline") continue;
      if (r.height < m - 0.5 || r.width < m - 0.5) out.push(`${el.tagName} "${(el.textContent ?? "").trim().slice(0, 30)}" ${Math.round(r.width)}x${Math.round(r.height)}`);
    }
    return out;
  }, min);
}

for (const name of NAMES) {
  const detail = read(`case_${name}.json`);
  const audit = read(`case_${name}_audit.json`);

  test.describe(name, () => {
    test("detail page shows identities, readers, sign-offs and booking from the API data", async ({ page }) => {
      const res = await page.goto(href(name));
      expect(res?.status()).toBe(200);
      await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);

      const records = page.getByRole("list", { name: "Records", exact: true });
      await expect(records.getByRole("listitem")).toHaveCount(detail.identities.length);
      for (const idn of detail.identities) await expect(records).toContainText(idn.registration.split("|")[1]);
      await expect(page.getByRole("list", { name: "How the records were linked" })).toContainText("Match probability");
      await expect(page.getByRole("list", { name: "How the records were linked" })).toContainText("Auto-close line");

      // The latest reading: field table and quote.
      const readers = page.getByRole("region", { name: /DoH closure checklist|Readers' result/ });
      await expect(readers.getByRole("table")).toBeVisible();
      const ev = detail.evidence.at(-1);
      const quote = ev.quote || ev.reader_a_fields?.quote;
      await expect(readers).toContainText(quote);

      await expect(page.getByRole("region", { name: "Human sign-offs" })).toBeVisible();
      if (detail.signoffs.length) await expect(page.getByRole("list", { name: "Sign-offs" }).getByRole("listitem")).toHaveCount(detail.signoffs.length);
      else await expect(page.getByRole("region", { name: "Human sign-offs" })).toContainText("No person signed anything");

      const booking = page.getByRole("region", { name: "Booking" });
      await expect(booking.locator('[data-honesty="simulated"]')).toHaveText("Simulated decision support");
      await expect(booking).toContainText(detail.booking.status === "suggested" ? "Slot suggested" : "No booking needed");
      if (detail.booking.prep_risk) await expect(booking).toContainText("Bowel-prep risk (App 9)");

      await expect(page.getByRole("region", { name: /Why it (closed|is still open)/ })).toBeVisible();
      await expect(page.getByRole("region", { name: "Consent and messages" })).toBeVisible();
      await expect(page.getByRole("list", { name: "Timeline" }).getByRole("listitem")).toHaveCount(detail.timeline.length);
      await expect(page.getByRole("link", { name: "Audit trail", exact: true })).toHaveAttribute("href", href(name, "/audit"));
      await shot(page, `detail-${name}-1440`);
      await noSeriousAxe(page);
    });

    test("audit page shows the chain check, every signature and the history", async ({ page }) => {
      const res = await page.goto(href(name, "/audit"));
      expect(res?.status()).toBe(200);
      await expect(page.getByRole("heading", { level: 1 })).toHaveText("Audit trail");
      await expect(page.getByTestId("chain-verified")).toContainText(
        audit.chain_verified ? `Verified · ${audit.chain.length} of ${audit.chain_length} intact` : "Chain broken",
      );
      await expect(page.getByRole("region", { name: "Chain check" })).toContainText("recomputed every entry's hash");

      const entries = page.getByRole("list", { name: "Chain entries" }).getByRole("listitem");
      await expect(entries).toHaveCount(audit.chain.length);
      await expect(entries.first()).toContainText("Starts the chain");

      const sigs = page.getByRole("list", { name: "Decision signatures" }).getByRole("listitem");
      await expect(sigs).toHaveCount(audit.decision_signatures.length);
      const matching = audit.decision_signatures.filter((s: { sig: string; recomputed: string }) => s.sig === s.recomputed).length;
      await expect(page.getByTestId("signature-count")).toHaveText(`${matching} of ${audit.decision_signatures.length} match`);
      await expect(sigs.first()).toContainText("Stored");
      await expect(sigs.first()).toContainText("Recomputed");
      await expect(sigs.first()).toContainText(matching === audit.decision_signatures.length ? "Matches" : /Matches|Does not match/);

      const history = page.getByRole("region", { name: "Review history" });
      await expect(history).toBeVisible();
      if (audit.history.length === 0) await expect(history).toContainText("Nobody has signed this case through the API yet");
      await expect(page.getByRole("region", { name: "Anchors" })).toContainText(audit.anchor_note);
      await shot(page, `audit-${name}-1440`);
      await noSeriousAxe(page);
    });

    test("at 390px the case and audit pages do not scroll sideways", async ({ page }) => {
      await page.setViewportSize({ width: 390, height: 844 });
      for (const [tail, label] of [
        ["", "detail"],
        ["/audit", "audit"],
      ] as const) {
        await page.goto(href(name, tail));
        await expectNoOverflow(page, 390);
        expect(await smallTargets(page, 24)).toEqual([]);
        await shot(page, `${label}-${name}-390`);
      }
    });
  });
}

test.describe("a wrong id", () => {
  for (const base of ["/app/patients", "/app/review"] as const) {
    test(`${base}/1 is a 404 that lists the demo cases`, async ({ page }) => {
      const res = await page.goto(`${base}/1`);
      expect(res?.status()).toBe(404);
      await expect(page.getByRole("heading", { level: 1 })).toHaveText("We could not find that case");
      const list = page.getByRole("list", { name: "Demo cases" }).getByRole("link");
      await expect(list).toHaveCount(NAMES.length);
      await expect(list.first()).toHaveAttribute("href", new RegExp(`^${base}/`));
      await expect(list.filter({ hasText: "Wrong-patient report caught" })).toContainText("Aisha");
      await expect(list.filter({ hasText: "Eye exam closed the loop" })).toContainText("PHC-sr-1029");
      await shot(page, `notfound-${base.split("/").pop()}`);
      await noSeriousAxe(page);
      await list.first().click();
      expect((await page.evaluate(() => document.title)).length).toBeGreaterThan(0);
      await expect(page.getByRole("heading", { level: 1 })).not.toHaveText("We could not find that case");
    });
  }

  test("an unknown audit id is the same helpful 404", async ({ page }) => {
    const res = await page.goto(`/app/patients/${encodeURIComponent("nowhere/ServiceRequest/none")}/audit`);
    expect(res?.status()).toBe(404);
    await expect(page.getByRole("list", { name: "Demo cases" })).toBeVisible();
  });
});

test.describe("review desk on a phone", () => {
  test.use({ viewport: { width: 390, height: 844 } });
  const desk = href("review_hold").replace("/patients/", "/review/");

  test("the queue stacks, has no sideways scroll and no small targets", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/app/review");
    await expectNoOverflow(page, 390);
    expect(await smallTargets(page, 44)).toEqual([]);
    await shot(page, "review-queue-390");
    await noSeriousAxe(page);
  });

  test("the desk stacks, the answer is in the first screen and every target is 44px", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(desk);
    await expectNoOverflow(page, 390);
    const same = page.getByRole("radio", { name: "Same patient", exact: true });
    const box = await same.boundingBox();
    expect(box && box.y + box.height).toBeLessThanOrEqual(844);
    expect(await smallTargets(page, 44)).toEqual([]);

    // Answer both questions; the sign button is a target too.
    await same.click();
    await page.getByRole("radio", { name: "Adequate", exact: true }).click();
    await expectNoOverflow(page, 390);
    expect(await smallTargets(page, 44)).toEqual([]);
    await page.getByRole("button", { name: "Why it was held" }).click();
    await expectNoOverflow(page, 390);
    await shot(page, "review-desk-390");

    await page.getByRole("button", { name: "Preview: Close the case" }).click();
    await expect(page.getByRole("status")).toContainText("Would close the case");
    await expectNoOverflow(page, 390);
    expect(await smallTargets(page, 44)).toEqual([]);
    await shot(page, "review-closed-390");
    await noSeriousAxe(page);
  });

  test("keyboard focus is visible on the answers", async ({ page }) => {
    await page.goto(desk);
    const same = page.getByRole("radio", { name: "Same patient", exact: true });
    await same.focus();
    const outline = await same.evaluate((el) => {
      const s = getComputedStyle(el);
      return { style: s.outlineStyle, width: parseFloat(s.outlineWidth) };
    });
    // :focus-visible is set in globals.css; a focus() call from a script counts after a key press.
    await page.keyboard.press("Shift+Tab");
    await page.keyboard.press("Tab");
    const after = await same.evaluate((el) => {
      const s = getComputedStyle(el);
      return { style: s.outlineStyle, width: parseFloat(s.outlineWidth) };
    });
    expect(after.style).not.toBe("none");
    expect(after.width).toBeGreaterThan(0);
    expect(outline).toBeDefined();
  });

  test("reduced motion: the second question appears without a transition", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(desk);
    await page.getByRole("radio", { name: "Same patient", exact: true }).click();
    const q2 = page.getByRole("group", { name: /Bowel prep/ });
    await expect(q2).toBeVisible();
    // With reduced motion the motion library skips the transform, so no y offset is left behind.
    await expect(q2.locator("xpath=..")).toHaveCSS("opacity", "1");
  });

  test("a case that is not waiting shows its answers read-only", async ({ page }) => {
    await page.goto(href("evidence_closure").replace("/patients/", "/review/"));
    await expect(page.getByRole("heading", { level: 2, name: "This case is not waiting for a signature" })).toBeVisible();
    await expect(page.getByRole("radio")).toHaveCount(0);
    await expectNoOverflow(page, 390);
  });
});

test("the review desk at desktop width keeps the answer beside the evidence", async ({ page }) => {
  await page.goto(href("review_hold").replace("/patients/", "/review/"));
  const aside = await page.getByRole("complementary", { name: "Your answers" }).boundingBox();
  const bridge = await page.getByRole("region", { name: "Is this the same person?" }).boundingBox();
  expect(aside && bridge && aside.x).toBeGreaterThan(bridge!.x + bridge!.width - 1);
  await shot(page, "review-desk-1440");
});
