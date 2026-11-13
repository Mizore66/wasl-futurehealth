import { describe, expect, it } from "vitest";
import { DICTS, LANGS, PATIENT_LANGS } from "@/components/patient/i18n";
import { parsePhoneQuery, phoneHref, SLOTS, slotLabel } from "@/components/patient/journey";

describe("phone query", () => {
  it("defaults to a reviewed language with consent unknown", () => {
    expect(parsePhoneQuery({})).toEqual({ lang: "en", consent: "unknown" });
  });

  it("rejects an unknown language instead of rendering a missing dictionary", () => {
    expect(parsePhoneQuery({ lang: "fr" }).lang).toBe("en");
  });

  it("never selects a dictionary awaiting medical translation review", () => {
    expect(PATIENT_LANGS).toEqual(["en"]);
    for (const lang of LANGS.filter((l) => !PATIENT_LANGS.includes(l as never))) {
      expect(parsePhoneQuery({ lang }).lang).toBe("en");
      expect(parsePhoneQuery({ lang: [lang, "en"] }).lang).toBe("en");
      expect(phoneHref("book", { lang: parsePhoneQuery({ lang }).lang, consent: "given" })).toBe("/patient/book?lang=en&consent=yes");
    }
  });

  it("round-trips language and consent through the step links", () => {
    for (const lang of PATIENT_LANGS) {
      for (const consent of ["given", "declined", "unknown"] as const) {
        const href = phoneHref("book", { lang, consent });
        const search = Object.fromEntries(new URL(href, "http://x").searchParams);
        expect(parsePhoneQuery(search)).toEqual({ lang, consent });
      }
    }
  });
});

describe("booking slots", () => {
  it("are the canonical October offers with the right weekdays", () => {
    const t = DICTS.en.book;
    expect(SLOTS.map((s) => slotLabel(s, t))).toEqual([
      { weekday: "Friday", date: "9", time: "9:30", period: "am" },
      { weekday: "Thursday", date: "8", time: "11:00", period: "am" },
      { weekday: "Monday", date: "12", time: "1:00", period: "pm" },
    ]);
  });
});
