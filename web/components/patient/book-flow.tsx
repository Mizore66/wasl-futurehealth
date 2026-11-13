"use client";

import { useState } from "react";
import type { PatientJourney, PatientOffers } from "@/lib/api/patient-schema";
import { cn } from "@/lib/utils";
import { DICTS } from "./i18n";
import { slotLabel, type PhoneQuery } from "./journey";
import { CallBar } from "./parts";
import { PatientStatus } from "./patient-status";
import { usePatientJourney } from "./use-patient-journey";

type Mode = "proposed" | "choosing" | "confirmed";
const PREP_IDS = ["clear_fluids_and_prep", "escort_home"] as const;

/** Slot choices are preferences for coordinator review, never reservations. */
export function BookFlow({ query, initial, offers }: { query: PhoneQuery; initial: PatientJourney; offers: PatientOffers }) {
  const t = DICTS[query.lang];
  const b = t.book;
  const ur = query.lang === "ur";
  const { journey, busy, status, save, retry, canRetry } = usePatientJourney(initial);
  const savedIndex = offers.slots.findIndex((slot) => slot.slot_id === initial.slot_selection.slot_id);
  const [mode, setMode] = useState<Mode>(initial.slot_selection.status === "selected" ? "confirmed" : "proposed");
  const [current, setCurrent] = useState(savedIndex >= 0 ? savedIndex : 0);

  async function retrySave() {
    const currentJourney = await retry();
    if (currentJourney?.slot_selection.status !== "selected") return;
    const index = offers.slots.findIndex((offer) => offer.slot_id === currentJourney.slot_selection.slot_id);
    if (index >= 0) setCurrent(index);
    setMode("confirmed");
  }

  const slots = offers.slots.map((slot) => slotLabel(slot, b));
  const slot = slots[current];
  const confirmed = mode === "confirmed" && journey.slot_selection.status === "selected";
  const contact = journey.contact_permission.status;

  async function confirmSlot() {
    const offer = offers.slots[current];
    if (!offer) return;
    const saved = await save({ kind: "slot_selected", data: { slot_id: offer.slot_id, offers_version: offers.version } });
    if (saved) setMode("confirmed");
  }

  async function togglePreparation(index: number) {
    const item_id = PREP_IDS[index];
    const completed = !journey.preparation.items.find((item) => item.item_id === item_id)?.completed;
    await save({ kind: "preparation_changed", data: { item_id, completed } });
  }

  if (!slot) {
    return (
      <main className="flex-1 px-5 pt-4 pb-4">
        <h1 className="m-0 text-[20px] font-semibold">{b.title}</h1>
        <p role="status" className="mt-4 text-muted">{t.common.unavailable}</p>
      </main>
    );
  }

  return (
    <>
      <main className="flex flex-1 flex-col gap-3 px-5 pt-2 pb-4" aria-busy={busy}>
        <section className="rounded-card border border-line bg-surface p-5">
          <h1 className={cn("m-0 mb-2 text-[20px] font-semibold", ur ? "leading-[2]" : "leading-[1.45]")}>{b.title}</h1>
          <p className="m-0 mb-1 flex items-center gap-2" aria-live="polite">
            <span aria-hidden="true" className={cn("size-2.5 flex-none", confirmed ? "rounded-[2px] bg-positive" : "rounded-full bg-hue")} />
            <span className={cn("text-[15px] font-medium", confirmed ? "text-positive" : "text-hue")}>{confirmed ? b.confirmed : b.proposed}</span>
          </p>
          <p data-testid="slot-date" className="m-0 flex flex-wrap gap-x-1.5 text-[24px] leading-[1.5] font-semibold tabular-nums">
            <span>{slot.weekday}</span> <bdi dir="ltr">{slot.date}</bdi> <span>{b.october}</span>
          </p>
          <p data-testid="slot-time" className="m-0 flex flex-wrap gap-x-1.5 text-[17px] leading-[1.7] tabular-nums">
            <span>{b.at}</span> <bdi dir="ltr">{slot.time}</bdi> <span>{slot.period}</span>
          </p>

          <div className="mt-4 flex items-center gap-3">
            <div role="img" aria-label={b.mapLabel} className="relative size-16 flex-none overflow-hidden rounded-control bg-raised">
              <span aria-hidden="true" className="absolute top-[22px] right-0 left-0 h-1.5 bg-surface" />
              <span aria-hidden="true" className="absolute top-0 bottom-0 left-[38px] w-1.5 bg-surface" />
              <svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="var(--color-ink)" className="absolute top-3 left-5"><path d="M12 2a7 7 0 0 0-7 7c0 5 7 13 7 13s7-8 7-13a7 7 0 0 0-7-7zm0 9.5A2.5 2.5 0 1 1 12 6.5a2.5 2.5 0 0 1 0 5z" /></svg>
            </div>
            <div>
              <div className="text-[16px] leading-[1.6] font-medium">{b.place} <bdi dir="ltr" className="font-normal text-muted">(SKMC)</bdi></div>
              <div className="text-[14px] leading-[1.6] text-muted">{b.address}</div>
            </div>
          </div>

          {mode === "proposed" && (
            <div className="mt-5 flex gap-3">
              <button type="button" disabled={busy} onClick={() => void confirmSlot()} className="min-h-[52px] flex-2 cursor-pointer rounded-control border-0 bg-cta text-[17px] font-medium text-white disabled:cursor-wait disabled:opacity-60">{b.confirm}</button>
              <button type="button" disabled={busy} onClick={() => setMode("choosing")} className="min-h-[52px] flex-1 cursor-pointer rounded-control border-0 bg-active text-[17px] font-medium text-ink disabled:cursor-wait disabled:opacity-60">{b.another}</button>
            </div>
          )}

          {mode === "choosing" && (
            <div>
              <p className="m-0 mt-5 mb-2 text-[15px] text-muted">{b.pick}</p>
              <div role="group" aria-label={b.othersLabel} className="flex flex-col gap-2">
                {slots.map((s, i) =>
                  i === current ? null : (
                    <button
                      key={offers.slots[i].slot_id}
                      type="button"
                      disabled={busy}
                      onClick={() => { setCurrent(i); setMode("proposed"); }}
                      className="flex min-h-12 w-full cursor-pointer items-center gap-1.5 rounded-control border border-control bg-surface px-4 text-[16px] text-ink tabular-nums disabled:cursor-wait disabled:opacity-60"
                    >
                      <span>{s.weekday}</span> <bdi dir="ltr">{s.date}</bdi> <span>{b.october}{b.sep}</span> <bdi dir="ltr">{s.time}</bdi> <span>{s.period}</span>
                    </button>
                  ),
                )}
              </div>
            </div>
          )}

          {confirmed && (
            <div className="m-0 mt-5 text-[15px] leading-[1.8] text-muted">
              <p className="m-0">{contact === "granted" ? b.smsYes : contact === "opted_out" ? t.common.stopped : b.smsNo}</p>
              <button type="button" disabled={busy} onClick={() => setMode("choosing")} className="mt-2 inline min-h-11 cursor-pointer border-0 bg-transparent p-0 text-[15px] text-hue underline">{b.change}</button>
            </div>
          )}
          <PatientStatus t={t.common} status={status} canRetry={canRetry} onRetry={() => void retrySave()} />
        </section>

        <section aria-labelledby="prep-head" className="rounded-card border border-line bg-surface p-5">
          <h2 id="prep-head" className="m-0 mb-3 text-[18px] font-semibold">{b.prepHead}</h2>
          <p role="note" className="m-0 mb-2 flex items-start gap-2.5 rounded-control bg-pill-warning-bg p-3 text-[16px] leading-[1.75] font-medium text-pill-warning-ink">
            <svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" className="mt-1 flex-none"><path d="M12 3l9.5 17h-19z" /><path d="M12 10v4" /><path d="M12 17.5v.01" /></svg>
            <span>{b.warn}</span>
          </p>
          <div className="flex flex-col gap-1">
            {b.prep.map((text, i) => {
              const on = journey.preparation.items.find((item) => item.item_id === PREP_IDS[i])?.completed ?? false;
              return (
                <button
                  key={PREP_IDS[i]}
                  type="button"
                  role="checkbox"
                  aria-checked={on}
                  disabled={busy}
                  onClick={() => void togglePreparation(i)}
                  className="flex min-h-[52px] w-full cursor-pointer items-start gap-3 rounded-control border-0 bg-surface p-2.5 text-start disabled:cursor-wait disabled:opacity-60"
                >
                  <span aria-hidden="true" className={cn("mt-1 flex size-[22px] flex-none items-center justify-center rounded-chip", on ? "bg-cta" : "border-[1.5px] border-muted bg-surface")}>
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke={on ? "var(--color-surface)" : "transparent"} strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="M5 12l5 5 9-10" /></svg>
                  </span>
                  <span className={cn("text-[16px]", ur ? "leading-[2.1]" : "leading-[1.75]", on ? "text-muted" : "text-ink")}>{text}</span>
                </button>
              );
            })}
          </div>
        </section>
      </main>
      <CallBar label={t.call} quiet />
    </>
  );
}
