"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import { DICTS } from "./i18n";
import { phoneHref, type Consent, type PhoneQuery } from "./journey";
import { CallBar, PhoneNumber, phoneType } from "./parts";

type CheckStep = "question" | "explain" | "consent" | "done";

const PROGRESS: Record<CheckStep, 0 | 1 | 2> = { question: 0, explain: 0, consent: 1, done: 2 };

const SECONDARY = "h-[52px] w-full cursor-pointer rounded-control border-0 bg-active text-[17px] font-medium text-ink";

/** The teach-back: one question, a re-explanation on a wrong answer, then SMS consent. */
export function CheckFlow({ query }: { query: PhoneQuery }) {
  const t = DICTS[query.lang];
  const c = t.check;
  const type = phoneType(query.lang);
  const router = useRouter();
  const [step, setStep] = useState<CheckStep>("question");
  const [consent, setConsent] = useState<Consent>(query.consent);
  const heading = useRef<HTMLHeadingElement>(null);
  const moved = useRef(false);

  useEffect(() => {
    if (moved.current) heading.current?.focus();
    moved.current = true;
  }, [step]);

  function answer(given: boolean) {
    const next: Consent = given ? "given" : "declined";
    setConsent(next);
    setStep("done");
    router.replace(phoneHref("check", { ...query, consent: next }), { scroll: false });
  }

  const at = PROGRESS[step];
  const h1 = cn(type.h1, "outline-none");

  return (
    <>
      <main className="flex-1 px-5 pt-4 pb-4">
        <div
          role="img"
          aria-label={c.stepFmt.replace("{n}", String(at + 1)).replace("{name}", c.steps[at])}
          className="mb-5 flex items-center gap-1.5"
        >
          {c.steps.map((s, i) => (
            <span
              key={s}
              className={cn("block h-2 rounded-full", i === at ? "w-5 bg-hue" : i < at ? "w-2 bg-ink-soft" : "w-2 bg-control")}
            />
          ))}
        </div>

        {step === "question" && (
          <div>
            <p className="m-0 mb-1.5 text-[16px] text-muted">{c.intro}</p>
            <h1 ref={heading} tabIndex={-1} className={h1}>
              {c.question}
            </h1>
            <div role="group" aria-label={c.groupLabel} className="mt-3 flex flex-col gap-3">
              {[
                { text: c.wrong, right: false },
                { text: c.right, right: true },
              ].map((o, i) => (
                <button
                  key={o.text}
                  type="button"
                  onClick={() => setStep(o.right ? "consent" : "explain")}
                  className="flex min-h-[76px] w-full cursor-pointer items-center gap-3.5 rounded-card border border-control bg-surface px-4 py-3 text-start text-ink"
                >
                  <span aria-hidden="true" className="flex size-9 flex-none items-center justify-center rounded-full bg-raised text-[17px] font-semibold text-ink-soft">
                    {c.letters[i]}
                  </span>
                  <span className={type.body}>{o.text}</span>
                </button>
              ))}
            </div>
            <p className="m-0 mt-4 text-[15px] leading-[1.8] text-muted">{c.notTest}</p>
          </div>
        )}

        {step === "explain" && (
          <div>
            <h1 ref={heading} tabIndex={-1} className={h1}>
              {c.explainTitle}
            </h1>
            <div className="flex flex-col gap-3 rounded-card border border-line bg-surface p-5">
              {c.explain.map((line, i) => (
                <p key={line} className={cn(type.body, i === 0 && "font-medium")}>
                  {line}
                </p>
              ))}
            </div>
            <button type="button" onClick={() => setStep("question")} className={cn(SECONDARY, "mt-4")}>
              {c.retry}
            </button>
          </div>
        )}

        {step === "consent" && (
          <div>
            <p className="m-0 mb-1.5 flex items-center gap-1.5 text-[16px] text-positive">
              <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M5 12l5 5 9-10" />
              </svg>
              <span>{c.correct}</span>
            </p>
            <h1 ref={heading} tabIndex={-1} className={h1}>
              {c.consentTitle}
            </h1>
            <p className={type.body}>{c.consentBody}</p>
            <div className="mt-6 flex flex-col gap-3">
              <button type="button" onClick={() => answer(true)} className="h-[52px] w-full cursor-pointer rounded-control border-0 bg-cta text-[17px] font-medium text-white">
                {c.allow}
              </button>
              <button type="button" onClick={() => answer(false)} className={SECONDARY}>
                {c.decline}
              </button>
            </div>
          </div>
        )}

        {step === "done" && (
          <div>
            <span aria-hidden="true" className="mb-4 flex size-14 items-center justify-center rounded-full bg-pill-positive-bg text-pill-positive-ink">
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M5 12l5 5 9-10" />
              </svg>
            </span>
            <h1 ref={heading} tabIndex={-1} className={h1}>
              {c.thanks}
            </h1>
            <div className="flex flex-col gap-3">
              <p className={type.body}>
                {c.willCall} {c.alsoCall} <PhoneNumber />
              </p>
              <p className={cn(type.body, "text-muted")}>{consent === "given" ? c.smsYes : c.smsNo}</p>
            </div>
          </div>
        )}
      </main>
      <CallBar label={t.call} quiet={step === "consent"} />
    </>
  );
}
