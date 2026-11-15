"use client";

import { useEffect, useState } from "react";
import { PatientOutcomeResponse, PatientVerification, type PatientOutcomeAck, type PatientOutcomeHandoff } from "@/lib/api/patient-schema";
import { DICTS, type Lang } from "./i18n";
import type { PatientDict } from "./i18n/types";
import { phoneType } from "./parts";

type Ack = { code: string; issued: string | null; phase: "idle" | "sending" | "failed" };

/** The single status line for a hand-off whose content is not shown. */
export function outcomeWithheldLine(status: PatientOutcomeHandoff["status"], t: PatientDict["outcome"]): string | null {
  switch (status) {
    case "not_eligible":
      return t.notReady;
    case "pending_approval":
      return t.pending;
    case "held_consent":
      return t.held;
    case "suppressed":
      return t.suppressed;
    default:
      return null;
  }
}

async function loadOutcome(): Promise<PatientOutcomeHandoff | null> {
  const res = await fetch("/patient/outcome-handoff", { cache: "no-store" }).catch(() => null);
  const parsed = PatientOutcomeResponse.safeParse(res?.ok ? await res.json().catch(() => null) : null);
  return parsed.success ? parsed.data.handoff : null;
}

export function OutcomeHandoff({ lang }: { lang: Lang }) {
  const t = DICTS[lang].outcome;
  const r = DICTS[lang].rights;
  const common = DICTS[lang].common;
  const type = phoneType(lang);
  const [handoff, setHandoff] = useState<PatientOutcomeHandoff | null | undefined>(undefined);
  const [ack, setAck] = useState<Ack>({ code: "", issued: null, phase: "idle" });

  useEffect(() => {
    void loadOutcome().then(setHandoff);
  }, []);

  const issue = async () => {
    const res = await fetch("/patient/verification", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }).catch(() => null);
    const parsed = PatientVerification.safeParse(res?.ok ? await res.json().catch(() => null) : null);
    setAck((a) => (parsed.success ? { ...a, issued: parsed.data.code, phase: "idle" } : { ...a, phase: "failed" }));
  };

  const acknowledge = async () => {
    if (!handoff?.version) return;
    setAck((a) => ({ ...a, phase: "sending" }));
    const body: PatientOutcomeAck = { version: handoff.version, otp: ack.code.trim() };
    const res = await fetch("/patient/outcome-handoff", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) }).catch(() => null);
    const parsed = PatientOutcomeResponse.safeParse(res?.ok ? await res.json().catch(() => null) : null);
    if (!parsed.success) return setAck((a) => ({ ...a, phase: "failed" }));
    setHandoff(parsed.data.handoff);
  };

  if (handoff === undefined) return <main className="flex-1 px-5 pt-5 pb-8" aria-busy="true" />;
  const withheld = handoff ? outcomeWithheldLine(handoff.status, t) : common.unavailable;
  const button = "min-h-11 w-full cursor-pointer rounded-control border border-line bg-surface px-4 text-[16px] font-medium text-ink disabled:cursor-not-allowed disabled:opacity-50";

  return (
    <main className="flex-1 px-5 pt-5 pb-8">
      <h1 className={type.h1}>{t.title}</h1>
      {withheld !== null || !handoff?.content ? (
        <p role="status" className={type.body}>
          {withheld ?? t.notReady}
        </p>
      ) : (
        <>
          <p className={type.body}>{t.complete}</p>
          <section aria-labelledby="outcome-next" className="mt-4">
            <h2 id="outcome-next" className="m-0 mb-1 text-[17px] font-medium">
              {t.nextHead}
            </h2>
            <p className={type.body}>{t.next[handoff.content.next_step]}</p>
          </section>
          <section aria-labelledby="outcome-safety" className="mt-4 rounded-card border border-line p-4">
            <h2 id="outcome-safety" className="m-0 mb-1 text-[17px] font-medium">
              {t.safetyHead}
            </h2>
            <p className={type.body}>{t.safetyNet}</p>
          </section>
          <div className="mt-5 flex flex-col gap-3" aria-live="polite">
            {handoff.status === "acknowledged" ? (
              <p role="status" className="m-0 text-[16px] font-medium">
                {t.acknowledged}
              </p>
            ) : ack.issued === null ? (
              <button type="button" className={button} onClick={() => void issue()}>
                {r.getCode}
              </button>
            ) : (
              <form
                aria-label={t.acknowledge}
                className="flex flex-col gap-3"
                onSubmit={(e) => {
                  e.preventDefault();
                  void acknowledge();
                }}
              >
                <p role="status" className="m-0 text-[14px] leading-[1.7] text-muted">
                  {r.codeShown.replace("{code}", ack.issued)}
                </p>
                <label className="flex flex-col gap-1.5 text-[15px]">
                  {r.codeLabel}
                  <input
                    value={ack.code}
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={6}
                    dir="ltr"
                    onChange={(e) => setAck((a) => ({ ...a, code: e.target.value.replace(/\D/g, "") }))}
                    className="min-h-11 rounded-control border border-line bg-surface px-3 text-[18px] tabular-nums"
                  />
                </label>
                <button
                  type="submit"
                  disabled={ack.phase === "sending" || ack.code.length !== 6}
                  className="min-h-12 w-full cursor-pointer rounded-control border-0 bg-cta text-[16px] font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
                >
                  {ack.phase === "sending" ? r.sending : t.acknowledge}
                </button>
              </form>
            )}
            {ack.phase === "failed" && (
              <p role="alert" className="m-0 text-[14px] text-ink">
                {r.failed}
              </p>
            )}
          </div>
        </>
      )}
      <p className="m-0 mt-4 text-[13px] text-muted">{t.simulated}</p>
    </main>
  );
}
