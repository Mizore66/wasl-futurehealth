"use client";

import { useEffect, useState } from "react";
import {
  PatientRightsList,
  PatientRightsReceipt,
  PatientVerification,
  type PatientExplanation,
  type PatientRightsRequest,
  type PatientRightsSubmission,
} from "@/lib/api/patient-schema";
import { DICTS, type Lang } from "./i18n";
import type { PatientDict } from "./i18n/types";
import { phoneType } from "./parts";

type Kind = PatientRightsSubmission["kind"];
type Draft = { kind: Kind; requestId: string; reason: string; code: string; issued: string | null; phase: "editing" | "sending" | "failed" };

const LOCALE: Record<Lang, string> = { ar: "ar-AE", en: "en-AE", ur: "ur-PK", fil: "fil-PH" };

export function dueWords(iso: string, lang: Lang): string {
  return new Intl.DateTimeFormat(LOCALE[lang], { weekday: "long", day: "numeric", month: "long", timeZone: "Asia/Dubai" }).format(new Date(iso));
}

/** The one status sentence a patient sees for a request; matches the staff card's states. */
export function requestStatusLine(row: PatientRightsRequest, t: PatientDict["rights"], lang: Lang): string {
  if (row.kind === "explanation_requested") return t.answered;
  if (row.status === "resolved") return row.outcome === "decision_revised" ? t.revised : t.upheld;
  const due = row.due_at ? dueWords(row.due_at, lang) : "";
  return (row.overdue ? t.overdue : t.pending).replace("{due}", due);
}

async function loadRequests(): Promise<PatientRightsList | null> {
  const res = await fetch("/patient/requests", { cache: "no-store" }).catch(() => null);
  const parsed = PatientRightsList.safeParse(res?.ok ? await res.json().catch(() => null) : null);
  return parsed.success ? parsed.data : null;
}

export function RightsRequests({ lang }: { lang: Lang }) {
  const t = DICTS[lang].rights;
  const type = phoneType(lang);
  const [list, setList] = useState<PatientRightsList | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [explanation, setExplanation] = useState<PatientExplanation | null>(null);

  const refresh = () => loadRequests().then((loaded) => loaded && setList(loaded));
  useEffect(() => {
    void loadRequests().then((loaded) => loaded && setList(loaded));
  }, []);

  const start = (kind: Kind) => setDraft({ kind, requestId: crypto.randomUUID(), reason: "", code: "", issued: null, phase: "editing" });
  const update = (patch: Partial<Draft>) => setDraft((d) => (d ? { ...d, ...patch } : d));

  const issue = async () => {
    const res = await fetch("/patient/verification", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }).catch(() => null);
    const parsed = PatientVerification.safeParse(res?.ok ? await res.json().catch(() => null) : null);
    update(parsed.success ? { issued: parsed.data.code, phase: "editing" } : { phase: "failed" });
  };

  const submit = async () => {
    if (!draft || !list) return;
    update({ phase: "sending" });
    const body: PatientRightsSubmission = { kind: draft.kind, request_id: draft.requestId, otp: draft.code.trim(), decision: list.current_decision, reason: draft.reason.trim() };
    const res = await fetch("/patient/requests", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) }).catch(() => null);
    const parsed = PatientRightsReceipt.safeParse(res?.ok ? await res.json().catch(() => null) : null);
    if (!parsed.success) {
      update({ phase: "failed" });
      return;
    }
    setExplanation(parsed.data.explanation ?? null);
    setDraft(null);
    await refresh();
  };

  const reasonMissing = draft?.kind === "contest_submitted" && draft.reason.trim() === "";
  const button = "min-h-11 w-full cursor-pointer rounded-control border border-line bg-surface px-4 text-[16px] font-medium text-ink disabled:cursor-not-allowed disabled:opacity-50";

  return (
    <div className="mt-5 flex flex-col gap-3">
      {!draft && (
        <>
          <button type="button" className={button} disabled={!list} onClick={() => start("explanation_requested")}>
            {t.explain}
          </button>
          <button type="button" className={button} disabled={!list} onClick={() => start("contest_submitted")}>
            {t.contest}
          </button>
        </>
      )}

      {draft && (
        <form
          aria-label={draft.kind === "contest_submitted" ? t.contest : t.explain}
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <label className="flex flex-col gap-1.5 text-[15px]">
            {t.reasonLabel}
            <textarea
              value={draft.reason}
              maxLength={500}
              rows={3}
              required={draft.kind === "contest_submitted"}
              aria-describedby={reasonMissing ? "rights-reason-required" : undefined}
              onChange={(e) => update({ reason: e.target.value })}
              className="rounded-control border border-line bg-surface p-3 text-[16px]"
            />
          </label>
          {reasonMissing && (
            <p id="rights-reason-required" className="m-0 text-[14px] text-muted">
              {t.reasonRequired}
            </p>
          )}
          {draft.issued === null ? (
            <button type="button" className={button} onClick={() => void issue()}>
              {t.getCode}
            </button>
          ) : (
            <>
              <p role="status" className="m-0 text-[14px] leading-[1.7] text-muted">
                {t.codeShown.replace("{code}", draft.issued)}
              </p>
              <label className="flex flex-col gap-1.5 text-[15px]">
                {t.codeLabel}
                <input
                  value={draft.code}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  dir="ltr"
                  onChange={(e) => update({ code: e.target.value.replace(/\D/g, "") })}
                  className="min-h-11 rounded-control border border-line bg-surface px-3 text-[18px] tabular-nums"
                />
              </label>
              <button
                type="submit"
                disabled={draft.phase === "sending" || draft.code.length !== 6 || reasonMissing}
                className="min-h-12 w-full cursor-pointer rounded-control border-0 bg-cta text-[16px] font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
              >
                {draft.phase === "sending" ? t.sending : t.submit}
              </button>
            </>
          )}
          {draft.phase === "failed" && (
            <p role="alert" className="m-0 text-[14px] text-ink">
              {t.failed}
            </p>
          )}
        </form>
      )}

      {explanation && (
        <section aria-labelledby="rights-explanation" className="rounded-card border border-line p-4">
          <h3 id="rights-explanation" className="m-0 mb-2 text-[16px] font-medium">
            {t.explanationHead}
          </h3>
          <dl className="m-0 grid gap-1 text-[15px]">
            <dt className="text-muted">{t.matched}</dt>
            <dd className="m-0">{explanation.matched_records.map((r) => r.provider).join(", ") || t.none}</dd>
            <dt className="text-muted">{t.report}</dt>
            <dd className="m-0 [overflow-wrap:anywhere]">{explanation.closing_report || t.none}</dd>
            <dt className="text-muted">{t.state}</dt>
            <dd className="m-0">{explanation.state}</dd>
          </dl>
          {explanation.closure_held && <p className="m-0 mt-2 text-[14px] text-muted">{t.held}</p>}
        </section>
      )}

      {list && list.requests.length > 0 && (
        <section aria-labelledby="rights-list">
          <h3 id="rights-list" className="m-0 mb-2 text-[16px] font-medium">
            {t.listHead}
          </h3>
          <ul className="m-0 flex list-none flex-col gap-3 p-0">
            {list.requests.map((row) => (
              <li key={row.receipt} className="border-t border-line pt-3">
                <p className={type.body}>{row.kind === "contest_submitted" ? t.contest : t.explain}</p>
                <p className="m-0 text-[15px]">{requestStatusLine(row, t, lang)}</p>
                {row.closure_held && <p className="m-0 text-[14px] text-muted">{t.held}</p>}
                <p className="m-0 text-[13px] text-muted">
                  {t.receipt}{" "}
                  <bdi dir="ltr" className="tabular-nums">
                    {row.receipt.slice(0, 8)}
                  </bdi>
                </p>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
