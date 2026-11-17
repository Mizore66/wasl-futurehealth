import type { Lang } from "./i18n";

/**
 * Outcome hand-off wording added for the release and second-identifier gates (#112). English only until a qualified
 * translation review (#133): other languages show it with lang="en". Existing reviewed outcome copy stays in ./i18n.
 */
export const OUTCOME_GATE_COPY = {
  pendingRelease: "Your doctor approved this message. The care team has not released it to this link yet. Please check again later.",
  verifyIntro: "To see this message, first confirm it is you with a one-time code.",
  show: "Show my message",
} as const;

export function outcomeGateCopy(lang: Lang): { text: typeof OUTCOME_GATE_COPY; gated: boolean } {
  return { text: OUTCOME_GATE_COPY, gated: lang !== "en" };
}
