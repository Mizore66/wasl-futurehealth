"use client";

import { useEffect, useState } from "react";
import {
  PatientOutcomeResponse,
  PatientVerification,
  type PatientOutcomeAck,
  type PatientOutcomeHandoff,
  type PatientOutcomeVerify,
} from "@/lib/api/patient-schema";
import { DICTS, type Lang } from "./i18n";
import type { PatientDict } from "./i18n/types";
import { OUTCOME_GATE_COPY, outcomeGateCopy } from "./outcome-copy";
import { phoneType } from "./parts";
import { GatedNote } from "./programme-copy";

type Code = { code: string; issued: string | null; phase: "idle" | "sending" | "failed" };

/** The single status line for a hand-off whose content is not shown. */
export function outcomeWithheldLine(status: PatientOutcomeHandoff["status"], t: PatientDict["outcome"]): string | null {
  switch (status) {
    case "not_eligible":
      return t.notReady;
    case "pending_approval":
      return t.pending;
    case "pending_release":
      return OUTCOME_GATE_COPY.pendingRelease;
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

async function postOutcome(body: PatientOutcomeAck | PatientOutcomeVerify): Promise<PatientOutcomeHandoff | null> {
  const res = await fetch("/patient/outcome-handoff", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) }).catch(() => null);
  const parsed = PatientOutcomeResponse.safeParse(res?.ok ? await res.json().catch(() => null) : null);
  return parsed.success ? parsed.data.handoff : null;
}

export function OutcomeHandoff({ lang }: { lang: Lang }) {
  const t = DICTS[lang].outcome;
  const r = DICTS[lang].rights;
  const common = DICTS[lang].common;
  const gate = outcomeGateCopy(lang);
  const gateLang = gate.gated ? ({ lang: "en", dir: "ltr" } as const) : {};
  const type = phoneType(lang);
  const [handoff, setHandoff] = useState<PatientOutcomeHandoff | null | undefined>(undefined);
  const [code, setCode] = useState<Code>({ code: "", issued: null, phase: "idle" });
  const [acking, setAcking] = useState<"idle" | "sending" | "failed">("idle");

  useEffect(() => {
    void loadOutcome().then(setHandoff);
  }, []);

  const issue = async () => {
    const res = await fetch("/patient/verification", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }).catch(() => null);
    const parsed = PatientVerification.safeParse(res?.ok ? await res.json().catch(() => null) : null);
    setCode((c) => (parsed.success ? { ...c, issued: parsed.data.code, phase: "idle" } : { ...c, phase: "failed" }));
  };

  const verify = async () => {
    setCode((c) => ({ ...c, phase: "sending" }));
    const next = await postOutcome({ otp: code.code.trim() });
    if (!next?.content) return setCode((c) => ({ ...c, phase: "failed" }));
    setHandoff(next);
    setCode({ code: "", issued: null, phase: "idle" });
  };

  const acknowledge = async () => {
    if (!handoff?.version) return;
    setAcking("sending");
    const next = await postOutcome({ version: handoff.version, otp: "" });
    if (!next) return setAcking("failed");
    setHandoff(next);
    setAcking("idle");
  };

  if (handoff === undefined) return <main className="flex-1 px-5 pt-5 pb-8" aria-busy="true" />;
  const withheld = handoff ? outcomeWithheldLine(handoff.status, t) : common.unavailable;
  const releasable = handoff?.status === "released" || handoff?.status === "acknowledged";
  const button = "min-h-11 w-full cursor-pointer rounded-control border border-line bg-surface px-4 text-[16px] font-medium text-ink disabled:cursor-not-allowed disabled:opacity-50";
  const cta = "min-h-12 w-full cursor-pointer rounded-control border-0 bg-cta text-[16px] font-medium text-white disabled:cursor-not-allowed disabled:opacity-50";
  // The release and one-time-code wording is English until translation review (#133); say so where it shows.
  const gateShown = handoff?.status === "pending_release" || (withheld === null && releasable && !handoff?.content);

  return (
    <main className="flex-1 px-5 pt-5 pb-8">
      <h1 className={type.h1}>{t.title}</h1>
      {gateShown && gate.gated && (
        <div className="mt-2">
          <GatedNote script={{ gated: gate.gated }} />
        </div>
      )}
      {withheld !== null || !releasable ? (
        <p role="status" className={type.body} {...(handoff?.status === "pending_release" ? gateLang : {})}>
          {withheld ?? t.notReady}
        </p>
      ) : !handoff?.content ? (
        <div className="mt-2 flex flex-col gap-3" aria-live="polite">
          <p className={type.body} {...gateLang}>
            {gate.text.verifyIntro}
          </p>
          {code.issued === null ? (
            <button type="button" className={button} onClick={() => void issue()}>
              {r.getCode}
            </button>
          ) : (
            <form
              aria-label={gate.text.show}
              className="flex flex-col gap-3"
              onSubmit={(e) => {
                e.preventDefault();
                void verify();
              }}
            >
              <p role="status" className="m-0 text-[14px] leading-[1.7] text-muted">
                {r.codeShown.replace("{code}", code.issued)}
              </p>
              <label className="flex flex-col gap-1.5 text-[15px]">
                {r.codeLabel}
                <input
                  value={code.code}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  dir="ltr"
                  onChange={(e) => setCode((c) => ({ ...c, code: e.target.value.replace(/\D/g, "") }))}
                  className="min-h-11 rounded-control border border-line bg-surface px-3 text-[18px] tabular-nums"
                />
              </label>
              <button type="submit" disabled={code.phase === "sending" || code.code.length !== 6} className={cta} {...gateLang}>
                {code.phase === "sending" ? r.sending : gate.text.show}
              </button>
            </form>
          )}
          {code.phase === "failed" && (
            <p role="alert" className="m-0 text-[14px] text-ink">
              {r.failed}
            </p>
          )}
        </div>
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
            ) : (
              <button type="button" disabled={acking === "sending"} aria-busy={acking === "sending"} className={cta} onClick={() => void acknowledge()}>
                {acking === "sending" ? r.sending : t.acknowledge}
              </button>
            )}
            {acking === "failed" && (
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
