import { cn } from "@/lib/utils";
import { DICTS, type Lang } from "./i18n";
import { CLINIC_PHONE } from "./journey";

/**
 * Geist by its bare family name, not `var(--font-geist)`: that variable carries "Geist Fallback", a local
 * Arial alias whose Arabic glyphs would win over Plex Arabic and Nastaliq.
 */
export const PHONE_FONT: Record<Lang, string> = {
  ar: "Geist, var(--font-plex-arabic), sans-serif",
  en: "var(--font-geist), sans-serif",
  fil: "var(--font-geist), sans-serif",
  ur: "Geist, var(--font-nastaliq), serif",
};

/** Urdu needs Nastaliq's taller lines; Arabic script needs more leading than Latin. */
export function phoneType(lang: Lang) {
  const ur = lang === "ur";
  const rtl = DICTS[lang].dir === "rtl";
  return {
    h1: cn("m-0 mb-3 font-semibold", ur ? "text-[22px] leading-[2]" : "text-[24px] leading-[1.4]"),
    body: cn("m-0", ur ? "text-[16px] leading-[2.3]" : rtl ? "text-[17px] leading-[1.8]" : "text-[17px] leading-[1.55]"),
  };
}

export function PhoneNumber() {
  return (
    <bdi dir="ltr" className="tabular-nums">
      {CLINIC_PHONE.display}
    </bdi>
  );
}

export function CallBar({ label, quiet = false }: { label: string; quiet?: boolean }) {
  return (
    <div className="sticky bottom-0 border-t border-line bg-phone-ground px-5 pt-3 pb-6">
      <a
        href={`tel:${CLINIC_PHONE.tel}`}
        className={cn(
          "flex min-h-14 w-full items-center justify-center gap-2.5 rounded-control text-[17px] font-medium no-underline",
          quiet ? "bg-active text-ink hover:text-ink" : "bg-cta text-white hover:text-white",
        )}
      >
        <svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2" />
        </svg>
        <span>{label}</span>
        <PhoneNumber />
      </a>
    </div>
  );
}
