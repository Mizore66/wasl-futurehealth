import { describe, expect, it } from "vitest";
import { DICTS, LANGS } from "@/components/patient/i18n";
import { parsePhoneQuery, phoneHref, SLOTS, slotLabel } from "@/components/patient/journey";

describe("phone query", () => {
  it("defaults to Arabic with consent unknown", () => {
    expect(parsePhoneQuery({})).toEqual({ lang: "ar", consent: "unknown" });
  });

  it("rejects an unknown language instead of rendering a missing dictionary", () => {
    expect(parsePhoneQuery({ lang: "fr" }).lang).toBe("ar");
  });

  it("round-trips language and consent through the step links", () => {
    for (const lang of LANGS) {
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
