/** Words for a recorded preferred language (BCP-47 code from Patient.communication); "Not recorded" when absent. */
const LANGUAGE_WORDS: Record<string, string> = { ar: "Arabic", en: "English" };

export function languageWords(code: string | undefined): string {
  if (!code) return "Not recorded";
  const primary = code.split("-")[0].toLowerCase();
  return LANGUAGE_WORDS[primary] ?? code;
}
