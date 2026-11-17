import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { DICTS, LANGS, PATIENT_DEFAULT_LANG, PATIENT_LANGS, patientApproved, REVIEW_LANGS, TRANSLATION_REVIEW } from "@/components/patient/i18n";
import { parsePhoneQuery, phoneHref, STEPS } from "@/components/patient/journey";
import { translationReviewEnabled } from "@/components/patient/translation-review";

const UNREVIEWED_HEADER = "Needs medical translation review";
const dictSource = (lang: string) => readFileSync(join(__dirname, "..", "components", "patient", "i18n", `${lang}.ts`), "utf8");

describe("patient translation review gate (#133)", () => {
  it("serves patients only the source copy or a documented qualified review", () => {
    expect(PATIENT_LANGS).toEqual(["en"]);
    for (const lang of PATIENT_LANGS) expect(["source", "reviewed"]).toContain(TRANSLATION_REVIEW[lang].status);
    for (const lang of REVIEW_LANGS) expect(TRANSLATION_REVIEW[lang].status).toBe("unreviewed");
    expect([...PATIENT_LANGS, ...REVIEW_LANGS].sort()).toEqual([...LANGS].sort());
  });

  it("falls back to a patient-approved language, not Arabic", () => {
    expect(PATIENT_DEFAULT_LANG).toBe("en");
    expect(patientApproved(PATIENT_DEFAULT_LANG)).toBe(true);
    expect(patientApproved("ar")).toBe(false);
  });

  it("keeps the review record and the dictionary's own warning header in agreement", () => {
    for (const lang of LANGS) {
      const marked = dictSource(lang).includes(UNREVIEWED_HEADER);
      // A draft can only be re-enabled by recording its review AND removing its warning, never one alone.
      expect(marked, `${lang}: header vs TRANSLATION_REVIEW`).toBe(!patientApproved(lang));
    }
  });

  it("replaces every unreviewed, unknown or missing lang on patient routes, for every step", () => {
    for (const lang of [...REVIEW_LANGS, "fr", "", undefined, ["ar", "en"]]) {
      const query = parsePhoneQuery({ lang }, "patient");
      expect(query).toEqual({ lang: "en", surface: "patient" });
      for (const step of STEPS) expect(phoneHref(step, query)).toBe(`/patient/${step}?lang=en`);
    }
    expect(parsePhoneQuery({ lang: "en" })).toEqual({ lang: "en", surface: "patient" });
  });

  it("keeps the preview on its own path and to unreviewed drafts only", () => {
    expect(parsePhoneQuery({ lang: "ur" }, "review")).toEqual({ lang: "ur", surface: "review" });
    expect(parsePhoneQuery({ lang: "en" }, "review").lang).toBe(REVIEW_LANGS[0]);
    expect(phoneHref("check", { lang: "ar", surface: "review" })).toBe("/translation-review/check?lang=ar");
  });

  it("opens the preview only on an explicit opt-in", () => {
    expect(translationReviewEnabled({})).toBe(false);
    expect(translationReviewEnabled({ WASL_TRANSLATION_REVIEW_PREVIEW: "true" })).toBe(false);
    expect(translationReviewEnabled({ WASL_TRANSLATION_REVIEW_PREVIEW: "1" })).toBe(true);
  });
});

describe("refusal wording (#157)", () => {
  const item = DICTS.en.notice.rightsItems.join(" ");

  it("keeps the right to decline and to question decisions", () => {
    expect(item).toContain("You can say no to follow-up or question any decision.");
  });

  it("does not promise that declining leaves care unchanged", () => {
    expect(item).not.toMatch(/care will not change|nothing will change|will not affect your care|no effect on your care/i);
  });

  it("assures respect and other care, and sends consequences to a clinician", () => {
    expect(item).toContain("you will still be treated with respect and can still get other care you need");
    expect(item).toContain("Talk to your doctor about what saying no could mean for you and what other choices you have.");
  });

  it("leaves the recommended colonoscopy step intact", () => {
    expect(DICTS.en.notice.next).toContain("book a colonoscopy");
  });

  it("flags each draft's stale refusal line until it is retranslated and reviewed", () => {
    for (const lang of REVIEW_LANGS) {
      const review = TRANSLATION_REVIEW[lang];
      expect(review.status === "unreviewed" && review.note).toContain("#157");
    }
  });
});
