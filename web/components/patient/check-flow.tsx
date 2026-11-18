"use client";

import { useEffect, useRef, useState } from "react";
import type { PatientJourney, PatientTeachBackCallback } from "@/lib/api/patient-schema";
import { cn } from "@/lib/utils";
import { type Lang, type PatientDict } from "./i18n";
import { usePatientDict } from "./i18n/context";
import type { PhoneQuery } from "./journey";
import { CallBar, PhoneNumber, phoneType } from "./parts";
import { PatientStatus } from "./patient-status";
import { GatedNote, scriptLang } from "./programme-copy";
import { patientScript, type PatientScript } from "./programme";
import { usePatientJourney } from "./use-patient-journey";

type CheckStep = "question" | "explain" | "consent" | "done";
const PROGRESS: Record<CheckStep, 0 | 1 | 2> = { question: 0, explain: 0, consent: 1, done: 2 };
const SECONDARY = "h-[52px] w-full cursor-pointer rounded-control border-0 bg-active text-[17px] font-medium text-ink disabled:cursor-wait disabled:opacity-60";

function initialStep(journey: PatientJourney): CheckStep {
  if (journey.teach_back.status === "pending") return "question";
  if (journey.teach_back.status !== "completed") return "explain";
  return journey.contact_permission.status === "unknown" ? "consent" : "done";
}

function dueDate(iso: string, lang: Lang): string {
  return new Intl.DateTimeFormat(lang, { timeZone: "Asia/Dubai", weekday: "long", day: "numeric", month: "long" }).format(new Date(iso));
}

/** The durable receipt for a failed teach-back: who calls, by when, and whether that call is recorded. */
function CallbackReceipt({ callback, c, lang, script }: { callback: PatientTeachBackCallback; c: PatientDict["check"]; lang: Lang; script: PatientScript }) {
  const type = phoneType(lang);
  return (
    <div role="status" className="mt-4 flex flex-col gap-2 rounded-card border border-line bg-surface p-5">
      {callback.status === "resolved" ? (
        <p className={type.body}>{c.callbackResolved}</p>
      ) : (
        <>
          <p className={type.body} {...scriptLang(script)}>{script.check.callbackOpen.replace("{date}", dueDate(callback.due_at, script.lang))}</p>
          {callback.overdue && <p className={type.body}>{c.callbackLate} <PhoneNumber /></p>}
        </>
      )}
      <p className="m-0 text-[15px] text-muted">{c.callbackRef} <span dir="ltr">{callback.id.slice(0, 8)}</span></p>
    </div>
  );
}

/** Teach-back and contact permission advance only after their durable patient events are confirmed. */
export function CheckFlow({ query, initial }: { query: PhoneQuery; initial: PatientJourney }) {
  const t = usePatientDict(query.lang);
  const c = t.check;
  const type = phoneType(query.lang);
  const { journey, busy, status, save, retry, canRetry } = usePatientJourney(initial);
  const script = patientScript(journey, query.lang, t);
  const s = script.check;
  const copy = scriptLang(script);
  const [step, setStep] = useState<CheckStep>(() => initialStep(initial));
  const heading = useRef<HTMLHeadingElement>(null);
  const moved = useRef(false);

  useEffect(() => {
    if (moved.current) heading.current?.focus();
    moved.current = true;
  }, [step]);

  async function retrySave() {
    const current = await retry();
    if (current) setStep(initialStep(current));
  }

  async function choose(answer_id: string) {
    const data = { question_id: journey.teach_back.question_id, answer_id };
    const right = answer_id === journey.next_step;
    const saved = await save(right ? { kind: "teach_back_completed", data } : { kind: "teach_back_answered", data });
    if (saved) setStep(right ? "consent" : "explain");
  }

  async function answer(permission: "granted" | "denied") {
    const saved = await save({ kind: "contact_permission_changed", data: { permission } });
    if (saved) setStep("done");
  }

  async function stopMessages() {
    await save({ kind: "contact_permission_changed", data: { permission: "opted_out" } });
  }

  const at = PROGRESS[step];
  const h1 = cn(type.h1, "outline-none");
  const contact = journey.contact_permission.status;

  return (
    <>
      <main className="flex-1 px-5 pt-4 pb-4" aria-busy={busy}>
        <div role="img" aria-label={c.stepFmt.replace("{n}", String(at + 1)).replace("{name}", c.steps[at])} className="mb-5 flex items-center gap-1.5">
          {c.steps.map((s, i) => (
            <span key={s} className={cn("block h-2 rounded-full", i === at ? "w-5 bg-hue" : i < at ? "w-2 bg-ink-soft" : "w-2 bg-control")} />
          ))}
        </div>

        <GatedNote script={script} />
        {step === "question" && (
          <div>
            <p className="m-0 mb-1.5 text-[16px] text-muted">{c.intro}</p>
            <h1 ref={heading} tabIndex={-1} className={h1} {...copy}>{s.question}</h1>
            <div role="group" aria-label={c.groupLabel} className="mt-3 flex flex-col gap-3">
              {s.answers.map((o, i) => (
                <button
                  key={o.id}
                  type="button"
                  disabled={busy}
                  onClick={() => void choose(o.id)}
                  className="flex min-h-[76px] w-full cursor-pointer items-center gap-3.5 rounded-card border border-control bg-surface px-4 py-3 text-start text-ink disabled:cursor-wait disabled:opacity-60"
                >
                  <span aria-hidden="true" className="flex size-9 flex-none items-center justify-center rounded-full bg-raised text-[17px] font-semibold text-ink-soft">{c.letters[i]}</span>
                  <span className={type.body} {...copy}>{o.text}</span>
                </button>
              ))}
            </div>
            <p className="m-0 mt-4 text-[15px] leading-[1.8] text-muted">{c.notTest}</p>
          </div>
        )}

        {step === "explain" && (
          <div>
            <h1 ref={heading} tabIndex={-1} className={h1}>{c.explainTitle}</h1>
            <div className="flex flex-col gap-3 rounded-card border border-line bg-surface p-5" {...copy}>
              {s.explain.map((line, i) => <p key={line} className={cn(type.body, i === 0 && "font-medium")}>{line}</p>)}
            </div>
            {journey.teach_back.callback && <CallbackReceipt callback={journey.teach_back.callback} c={c} lang={query.lang} script={script} />}
            <button type="button" onClick={() => setStep("question")} className={cn(SECONDARY, "mt-4")}>{c.retry}</button>
          </div>
        )}

        {step === "consent" && (
          <div>
            <p className="m-0 mb-1.5 flex items-center gap-1.5 text-[16px] text-positive">
              <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 12l5 5 9-10" /></svg>
              <span>{c.correct}</span>
            </p>
            <h1 ref={heading} tabIndex={-1} className={h1}>{c.consentTitle}</h1>
            <p className={type.body}>{c.consentBody}</p>
            <div className="mt-6 flex flex-col gap-3">
              <button type="button" disabled={busy} onClick={() => void answer("granted")} className="h-[52px] w-full cursor-pointer rounded-control border-0 bg-cta text-[17px] font-medium text-white disabled:cursor-wait disabled:opacity-60">{c.allow}</button>
              <button type="button" disabled={busy} onClick={() => void answer("denied")} className={SECONDARY}>{c.decline}</button>
            </div>
          </div>
        )}

        {step === "done" && (
          <div>
            <span aria-hidden="true" className="mb-4 flex size-14 items-center justify-center rounded-full bg-pill-positive-bg text-pill-positive-ink">
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M5 12l5 5 9-10" /></svg>
            </span>
            <h1 ref={heading} tabIndex={-1} className={h1} {...copy}>{s.thanks}</h1>
            <div className="flex flex-col gap-3">
              <p className={type.body} {...copy}>{s.willCall} <PhoneNumber /></p>
              <p className={cn(type.body, "text-muted")}>{contact === "granted" ? c.smsYes : contact === "opted_out" ? t.common.stopped : c.smsNo}</p>
              {contact === "granted" && (
                <button type="button" disabled={busy} onClick={() => void stopMessages()} className={SECONDARY}>{t.common.stopMessages}</button>
              )}
            </div>
          </div>
        )}
        <PatientStatus t={t.common} status={status} canRetry={canRetry} onRetry={() => void retrySave()} />
      </main>
      <CallBar label={t.call} quiet={step === "consent"} />
    </>
  );
}
