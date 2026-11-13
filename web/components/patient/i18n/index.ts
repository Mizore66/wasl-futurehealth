import { ar } from "./ar";
import { en } from "./en";
import { fil } from "./fil";
import { ur } from "./ur";
import { LANGS, type Lang, type PatientDict } from "./types";

export { LANGS, type Lang, type PatientDict };

export const DICTS: Record<Lang, PatientDict> = { ar, en, ur, fil };

/** A locale is served to patients only once qualified medical translation review is recorded here. */
const MEDICAL_TRANSLATION_REVIEWED: Record<Lang, boolean> = { en: true, ar: false, ur: false, fil: false };
export const PATIENT_LANGS: readonly Lang[] = LANGS.filter((lang) => MEDICAL_TRANSLATION_REVIEWED[lang]);
export const DEFAULT_PATIENT_LANG: Lang = "en";
