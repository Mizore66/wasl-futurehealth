import { ar } from "./ar";
import { en } from "./en";
import { fil } from "./fil";
import { ur } from "./ur";
import { LANGS, type Lang, type PatientDict } from "./types";

export { LANGS, type Lang, type PatientDict };
export { PATIENT_DEFAULT_LANG, PATIENT_LANGS, patientApproved, REVIEW_LANGS, TRANSLATION_REVIEW, type TranslationReview } from "./review";

export const DICTS: Record<Lang, PatientDict> = { ar, en, ur, fil };
