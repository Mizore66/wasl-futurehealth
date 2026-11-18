import { en } from "./en";
import { LANGS, type Lang, type PatientDict } from "./types";

export { LANGS, type Lang, type PatientDict };
export { approvedWith, PATIENT_DEFAULT_LANG, PATIENT_LANGS, patientApproved, REVIEW_LANGS, SOURCE_SIGN_OFF, TRANSLATION_REVIEW, type SourceSignOff, type TranslationReview } from "./review";
export { en as EN_DICT };

/**
 * The dictionaries a render may use, keyed by language. Patient routes carry only the approved source (English); the
 * internal translation-review route adds the unreviewed drafts (`./drafts`, imported only by server code, so the draft
 * text never enters a patient page's client bundle). A language missing from the set falls back to English.
 */
export type DictSet = Partial<Record<Lang, PatientDict>>;

export const PATIENT_DICTS: DictSet = { en };

export function dictFor(dicts: DictSet, lang: Lang): PatientDict {
  return dicts[lang] ?? en;
}

/** Text direction per language, without loading a dictionary. */
export const LANG_DIR: Record<Lang, "rtl" | "ltr"> = { ar: "rtl", ur: "rtl", en: "ltr", fil: "ltr" };
