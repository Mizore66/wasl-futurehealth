import { describe, expect, it } from "vitest";
import { languageWords } from "@/lib/domain/language";
import { CaseSummary } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

describe("preferred language", () => {
  it("names Arabic and English, keeps other tags, and says when nothing is recorded", () => {
    expect(languageWords("ar")).toBe("Arabic");
    expect(languageWords("en")).toBe("English");
    expect(languageWords("en-GB")).toBe("English");
    expect(languageWords("ur")).toBe("ur");
    expect(languageWords(undefined)).toBe("Not recorded");
    expect(languageWords("")).toBe("Not recorded");
  });

  it("is optional on the case summary", () => {
    const base = { ...hold.case, language: undefined };
    expect(CaseSummary.safeParse(base).success).toBe(true);
    const withLang = CaseSummary.parse({ ...base, language: "ar" });
    expect(withLang.language).toBe("ar");
  });
});
