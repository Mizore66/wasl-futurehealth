import { REVIEW_LANGS } from "./i18n";

/**
 * Entry condition for the internal translation-review preview (#133): a deployment must opt in explicitly with
 * WASL_TRANSLATION_REVIEW_PREVIEW=1, and there must be an unreviewed draft to review. Otherwise the route 404s.
 */
export function translationReviewEnabled(env: Record<string, string | undefined> = process.env): boolean {
  return env.WASL_TRANSLATION_REVIEW_PREVIEW === "1" && REVIEW_LANGS.length > 0;
}
