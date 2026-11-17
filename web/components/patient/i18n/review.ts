import { LANGS, type Lang } from "./types";

/**
 * Medical translation review status of each phone dictionary (#133).
 *
 * Patient routes serve a dictionary only when it is the clinical source copy or has a documented qualified
 * medical-translation review. A UI test is not evidence of translation quality: re-enabling a draft means
 * replacing its entry with a `reviewed` record (who, when, where the review is filed) and removing the
 * "Needs medical translation review" header from its file. Never bypass `PATIENT_LANGS` instead.
 */
export type TranslationReview =
  | { status: "source"; note: string }
  | { status: "unreviewed"; note: string }
  | { status: "reviewed"; reviewer: string; reviewedOn: string; record: string };

/** Copy whose meaning changed after the drafts were written; each draft must be retranslated from it before review. */
const STALE_SINCE_157 =
  "notice.rightsItems[2] still carries the withdrawn unchanged-care promise (#157) and must be retranslated from the revised English.";

export const TRANSLATION_REVIEW: Record<Lang, TranslationReview> = {
  en: {
    status: "source",
    note: "English is the source copy the translations are made from. Its revised refusal wording (#157) still needs clinical sign-off; see docs/FRONTEND-PLAN.md.",
  },
  ar: { status: "unreviewed", note: `Drafted for the design; no qualified medical translation review. ${STALE_SINCE_157}` },
  ur: { status: "unreviewed", note: `Drafted for the design; no qualified medical translation review. ${STALE_SINCE_157}` },
  fil: { status: "unreviewed", note: `Drafted for the design; no qualified medical translation review. ${STALE_SINCE_157}` },
};

export function patientApproved(lang: Lang): boolean {
  const review = TRANSLATION_REVIEW[lang];
  return review.status === "source" || review.status === "reviewed";
}

/** Languages patient routes may serve, in selector order. */
export const PATIENT_LANGS: readonly Lang[] = LANGS.filter(patientApproved);
/** Draft languages, served only by the internal translation-review preview. */
export const REVIEW_LANGS: readonly Lang[] = LANGS.filter((lang) => !patientApproved(lang));
/** The patient fallback for a missing, unknown or unreviewed `lang`. */
export const PATIENT_DEFAULT_LANG: Lang = "en";

if (!PATIENT_LANGS.includes(PATIENT_DEFAULT_LANG)) {
  throw new Error(`patient default language ${PATIENT_DEFAULT_LANG} is not patient-approved`);
}
