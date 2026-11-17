import { ar } from "./ar";
import { en } from "./en";
import { fil } from "./fil";
import { ur } from "./ur";
import { LANGS, type Lang, type PatientDict } from "./types";

export { LANGS, type Lang, type PatientDict };
export { approvedWith, PATIENT_DEFAULT_LANG, PATIENT_LANGS, patientApproved, REVIEW_LANGS, SOURCE_SIGN_OFF, TRANSLATION_REVIEW, type SourceSignOff, type TranslationReview } from "./review";

export const DICTS: Record<Lang, PatientDict> = { ar, en, ur, fil };
