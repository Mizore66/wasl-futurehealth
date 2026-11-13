import { ar } from "./ar";
import { en } from "./en";
import { fil } from "./fil";
import { ur } from "./ur";
import { LANGS, type Lang, type PatientDict } from "./types";

export { LANGS, type Lang, type PatientDict };

export const DICTS: Record<Lang, PatientDict> = { ar, en, ur, fil };
