import { ar } from "./ar";
import { en } from "./en";
import { fil } from "./fil";
import { ur } from "./ur";
import type { Lang, PatientDict } from "./types";

/**
 * Every dictionary, unreviewed drafts included. Never import this from a client component or anything a patient route's
 * client bundle loads: server code reaches it through `./drafts` (server-only); tests and e2e may import it directly.
 */
export const ALL_DICTS: Record<Lang, PatientDict> = { ar, en, ur, fil };
