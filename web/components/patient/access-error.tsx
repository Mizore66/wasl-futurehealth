import { DICTS } from "./i18n";
import type { PhoneQuery } from "./journey";
import { CallBar } from "./parts";
import { PhoneFrame } from "./phone-frame";

export type PatientAccessFailure = "invalid" | "expired" | "unavailable";

export function patientAccessFailure(value: string | string[] | undefined): PatientAccessFailure | null {
  const reason = Array.isArray(value) ? value[0] : value;
  return reason === "invalid" || reason === "expired" || reason === "unavailable" ? reason : null;
}

/** An access failure exposes no journey data and does not require a patient cookie. */
export function PatientAccessError({ query, reason }: { query: PhoneQuery; reason: PatientAccessFailure }) {
  const t = DICTS[query.lang];
  return (
    <PhoneFrame step="notice" query={query}>
      <main className="flex-1 px-5 pt-4 pb-4">
        <h1 className="m-0 text-[24px] leading-[1.5] font-semibold">{t.common.unavailableTitle}</h1>
        <p role="alert" className="mt-4 text-[17px] leading-[1.8]">
          {reason === "invalid" ? t.common.invalidLink : reason === "expired" ? t.common.expiredLink : t.common.unavailable}
        </p>
      </main>
      <CallBar label={t.call} quiet />
    </PhoneFrame>
  );
}
