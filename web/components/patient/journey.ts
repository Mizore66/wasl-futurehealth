import { DEFAULT_PATIENT_LANG, PATIENT_LANGS, type Lang, type PatientDict } from "./i18n";

/**
 * Fatima's three phones (canonical facts, xcritique-consistency.md). Designed, not served by the API:
 * the API has no notice, teach-back or booking endpoint, so the slots and phone number live here.
 */
export const STEPS = ["notice", "check", "book"] as const;
export type Step = (typeof STEPS)[number];

export const CLINIC_PHONE = { display: "02 555 0142", tel: "+97125550142" } as const;

/** Booking offers, all October 2026. The first is the proposed slot. */
export const SLOTS = [
  { date: 9, time: "09:30" },
  { date: 8, time: "11:00" },
  { date: 12, time: "13:00" },
] as const;
export type Slot = (typeof SLOTS)[number];

/** Whether Fatima allowed SMS reminders. `unknown` until she answers on the check phone. */
export type Consent = "given" | "declined" | "unknown";

export interface PhoneQuery {
  lang: Lang;
  consent: Consent;
}

type Search = Record<string, string | string[] | undefined>;

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

export function isStep(v: string): v is Step {
  return (STEPS as readonly string[]).includes(v);
}

export function parsePhoneQuery(search: Search): PhoneQuery {
  const lang = first(search.lang);
  const consent = first(search.consent);
  return {
    lang: (PATIENT_LANGS as readonly string[]).includes(lang ?? "") ? (lang as Lang) : DEFAULT_PATIENT_LANG,
    consent: consent === "yes" ? "given" : consent === "no" ? "declined" : "unknown",
  };
}

/** The link to a phone that carries the language and consent forward. */
export function phoneHref(step: Step, { lang, consent }: PhoneQuery): string {
  const q = new URLSearchParams({ lang });
  if (consent !== "unknown") q.set("consent", consent === "given" ? "yes" : "no");
  return `/patient/${step}?${q}`;
}

export interface SlotLabel {
  weekday: string;
  date: string;
  time: string;
  period: string;
}

export function slotLabel(slot: Slot, t: PatientDict["book"]): SlotLabel {
  const weekday = new Date(Date.UTC(2026, 9, slot.date)).getUTCDay();
  const [h, m] = slot.time.split(":").map(Number);
  return {
    weekday: t.weekdays[weekday],
    date: String(slot.date),
    time: `${h % 12 || 12}:${String(m).padStart(2, "0")}`,
    period: h < 12 ? t.am : t.pm,
  };
}
