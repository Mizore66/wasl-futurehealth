import type { PatientOffer } from "@/lib/api/patient-schema";
import { PATIENT_DEFAULT_LANG, PATIENT_LANGS, REVIEW_LANGS, type Lang, type PatientDict } from "./i18n";

export const STEPS = ["notice", "check", "book", "outcome"] as const;
export type Step = (typeof STEPS)[number];

export const CLINIC_PHONE = { display: "02 555 0142", tel: "+97125550142" } as const;

/**
 * `patient` is the patient-facing journey and serves only patient-approved dictionaries; `review` is the internal
 * translation-review preview of unreviewed drafts (#133). The route decides the surface, never a URL parameter.
 */
export type Surface = "patient" | "review";

/** Only language belongs in patient URLs. Consent and every other journey value come from the server. */
export interface PhoneQuery {
  lang: Lang;
  surface: Surface;
}

const BASE: Record<Surface, string> = { patient: "/patient", review: "/translation-review" };

export function surfaceLangs(surface: Surface): readonly Lang[] {
  return surface === "patient" ? PATIENT_LANGS : REVIEW_LANGS;
}

type Search = Record<string, string | string[] | undefined>;
const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export function isStep(v: string): v is Step {
  return (STEPS as readonly string[]).includes(v);
}

/** A missing, unknown or (on the patient surface) unreviewed language falls back to the surface's default. */
export function parsePhoneQuery(search: Search, surface: Surface = "patient"): PhoneQuery {
  const allowed = surfaceLangs(surface);
  const lang = first(search.lang);
  const fallback = surface === "patient" ? PATIENT_DEFAULT_LANG : (allowed[0] ?? PATIENT_DEFAULT_LANG);
  return { lang: (allowed as readonly string[]).includes(lang ?? "") ? (lang as Lang) : fallback, surface };
}

export function phoneHref(step: Step, { lang, surface }: PhoneQuery): string {
  return `${BASE[surface]}/${step}?${new URLSearchParams({ lang })}`;
}

export interface SlotLabel {
  weekday: string;
  date: string;
  month: string;
  time: string;
  period: string;
}

/** Format the offer's own unit-local wall time; never substitute a browser timezone or a hard-coded slot list. */
export function slotLabel(slot: PatientOffer, t: PatientDict["book"], lang: Lang): SlotLabel {
  const match = /:(\d{4})-(\d{2})-(\d{2}):(\d{2}):(\d{2})$/.exec(slot.slot_id);
  if (!match) return { weekday: "", date: slot.starts_at, month: "", time: "", period: "" };
  const [, y, month, day, hour, minute] = match;
  const utc = new Date(Date.UTC(Number(y), Number(month) - 1, Number(day)));
  const weekday = utc.getUTCDay();
  const h = Number(hour);
  return {
    weekday: t.weekdays[weekday],
    date: String(Number(day)),
    month: month === "10" ? t.october : new Intl.DateTimeFormat(lang, { month: "long", timeZone: "UTC" }).format(utc),
    time: `${h % 12 || 12}:${minute}`,
    period: h < 12 ? t.am : t.pm,
  };
}
