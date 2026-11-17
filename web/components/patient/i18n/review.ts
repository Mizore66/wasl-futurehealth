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

/**
 * English source copy whose meaning changed and still needs clinical sign-off (#157). A translation of copy whose
 * source meaning is unsigned cannot be patient-approved, whatever its own review says: record the sign-off here
 * (who, when, where it is filed) before any draft's `reviewed` entry can take effect.
 */
export type SourceSignOff =
  | { status: "pending"; copy: string; note: string }
  | { status: "signed_off"; copy: string; clinician: string; signedOn: string; record: string };

export const SOURCE_SIGN_OFF: readonly SourceSignOff[] = [
  {
    status: "pending",
    copy: "notice.rightsItems[2]",
    note: "Revised refusal wording (#157): may decline and question; respectful treatment and other care; consequences and alternatives with the doctor. Its meaning needs clinical review.",
  },
];

/** Each draft's refusal line was redrafted from the revised English by the development team, not a qualified translator. */
const REDRAFTED_157 =
  "notice.rightsItems[2] was redrafted from the revised English refusal wording (#157) without qualified translator review; it needs that review after the English meaning is clinically signed off.";

export const TRANSLATION_REVIEW: Record<Lang, TranslationReview> = {
  en: {
    status: "source",
    note: "English is the source copy the translations are made from. Its revised refusal wording (#157) still needs clinical sign-off; see docs/FRONTEND-PLAN.md.",
  },
  ar: { status: "unreviewed", note: `Drafted for the design; no qualified medical translation review. ${REDRAFTED_157}` },
  ur: { status: "unreviewed", note: `Drafted for the design; no qualified medical translation review. ${REDRAFTED_157}` },
  fil: { status: "unreviewed", note: `Drafted for the design; no qualified medical translation review. ${REDRAFTED_157}` },
};

/**
 * The English source is served (it is the corrected copy). A translation is served only with a documented qualified
 * review AND once every changed source meaning it translates is clinically signed off.
 */
export function approvedWith(review: TranslationReview, signOffs: readonly SourceSignOff[]): boolean {
  if (review.status === "source") return true;
  return review.status === "reviewed" && signOffs.every((s) => s.status === "signed_off");
}

export function patientApproved(lang: Lang): boolean {
  return approvedWith(TRANSLATION_REVIEW[lang], SOURCE_SIGN_OFF);
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
