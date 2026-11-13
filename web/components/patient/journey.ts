import type { PatientOffer } from "@/lib/api/patient-schema";
import { LANGS, type Lang, type PatientDict } from "./i18n";

export const STEPS = ["notice", "check", "book"] as const;
export type Step = (typeof STEPS)[number];

export const CLINIC_PHONE = { display: "02 555 0142", tel: "+97125550142" } as const;

/** Only language belongs in patient URLs. Consent and every other journey value come from the server. */
export interface PhoneQuery {
  lang: Lang;
}

type Search = Record<string, string | string[] | undefined>;
const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export function isStep(v: string): v is Step {
  return (STEPS as readonly string[]).includes(v);
}

export function parsePhoneQuery(search: Search): PhoneQuery {
  const lang = first(search.lang);
  return { lang: (LANGS as readonly string[]).includes(lang ?? "") ? (lang as Lang) : "ar" };
}

export function phoneHref(step: Step, { lang }: PhoneQuery): string {
  return `/patient/${step}?${new URLSearchParams({ lang })}`;
}

export interface SlotLabel {
  weekday: string;
  date: string;
  time: string;
  period: string;
}

/** Format the offer's own unit-local wall time; never substitute a browser timezone or a hard-coded slot list. */
export function slotLabel(slot: PatientOffer, t: PatientDict["book"]): SlotLabel {
  const match = /:(\d{4})-(\d{2})-(\d{2}):(\d{2}):(\d{2})$/.exec(slot.slot_id);
  if (!match) return { weekday: "", date: slot.starts_at, time: "", period: "" };
  const [, y, month, day, hour, minute] = match;
  const weekday = new Date(Date.UTC(Number(y), Number(month) - 1, Number(day))).getUTCDay();
  const h = Number(hour);
  return {
    weekday: t.weekdays[weekday],
    date: String(Number(day)),
    time: `${h % 12 || 12}:${minute}`,
    period: h < 12 ? t.am : t.pm,
  };
}
