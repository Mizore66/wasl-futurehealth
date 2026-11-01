"use client";

import NumberFlow, { type Format } from "@number-flow/react";
import { useState } from "react";
import { cn } from "@/lib/utils";
import { LTT_TIERS, OPERATING_TAU } from "./ltt";

const valueClass = "m-0 mt-1 block text-[clamp(1.75rem,1rem_+_1.5vw,2.5rem)] font-semibold leading-[1.1] tracking-[-0.02em]";

/** Pick a closure tier; the six calibration figures for it tick into place. */
export function TauPicker() {
  const [tau, setTau] = useState(OPERATING_TAU);
  const t = LTT_TIERS.find((r) => r.tau === tau)!;

  const n3: Format = { minimumFractionDigits: 3, maximumFractionDigits: 3 };
  const n1: Format = { minimumFractionDigits: 1, maximumFractionDigits: 1 };
  const cells: { label: string; value: number | string; format?: Format }[] = [
    { label: "False closures, with review", value: t.falseClosures },
    { label: "Wrong-patient message loops", value: t.wrongPatient },
    { label: "Certified at α 0.001", value: t.certified ? "Yes" : "No" },
    { label: "Recall, with review", value: t.recall, format: n3 },
    { label: "Recall, no review", value: t.recallNoReview, format: n3 },
    { label: "Review items per 100 loops", value: t.reviewPer100, format: n1 },
  ];

  return (
    <>
      <div role="group" aria-label="Closure tier" className="mt-6 flex flex-wrap gap-2">
        {LTT_TIERS.map((r) => (
          <button
            key={r.tau}
            type="button"
            aria-pressed={r.tau === tau}
            onClick={() => setTau(r.tau)}
            className={cn(
              "min-h-11 cursor-pointer rounded border-0 px-3.5 py-2.5 text-[16px] tabular-nums",
              r.tau === tau ? "bg-(--ink) font-semibold text-(--paper)" : "bg-white font-medium text-[#0a0a0a] hover:bg-(--paper)",
            )}
          >
            τ {r.tau}
          </button>
        ))}
      </div>
      <div aria-live="polite" className="mt-5 rounded bg-white px-[clamp(1.25rem,2.5vw,2rem)] py-7" data-testid="tau-panel">
        <div className="text-[20px] font-semibold tracking-[-0.01em]">
          τ {t.tau}. {t.role}
        </div>
        <dl className="m-0 mt-5 grid grid-cols-2 gap-x-6 gap-y-5 sm:grid-cols-3">
          {cells.map((c) => (
            <div key={c.label}>
              <dt className="text-[14px] text-[#5f6368]">{c.label}</dt>
              <dd className={valueClass} data-cell={c.label}>
                {typeof c.value === "string" ? (
                  c.value
                ) : (
                  <>
                    <span className="sr-only" data-value>
                      {c.value.toLocaleString("en-US", c.format)}
                    </span>
                    <span aria-hidden>
                      <NumberFlow value={c.value} format={c.format} />
                    </span>
                  </>
                )}
              </dd>
            </div>
          ))}
        </dl>
        <p className="m-0 mt-5 text-[16px] leading-[1.55]">{t.note}</p>
      </div>
    </>
  );
}
