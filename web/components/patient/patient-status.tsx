"use client";

import type { PatientDict } from "./i18n";
import type { PatientSaveStatus } from "./use-patient-journey";

export function PatientStatus({ t, status, canRetry, onRetry }: { t: PatientDict["common"]; status: PatientSaveStatus; canRetry: boolean; onRetry: () => void }) {
  if (!status) return null;
  const text =
    status === "saving"
      ? t.saving
      : status === "saved"
        ? t.saved
        : status === "conflict"
          ? t.conflict
          : status === "invalid"
            ? t.invalidLink
            : status === "expired"
              ? t.expiredLink
              : t.saveFailed;
  return (
    <div role="status" aria-live="polite" className="mt-4 text-[15px] leading-[1.7] text-muted" data-testid={`patient-${status}`}>
      <p className="m-0">{text}</p>
      {status === "failed" && canRetry && (
        <button type="button" onClick={onRetry} className="mt-2 min-h-11 cursor-pointer rounded-control border border-control bg-surface px-4 text-[16px] font-medium text-ink">
          {t.retry}
        </button>
      )}
    </div>
  );
}
