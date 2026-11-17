"use client";

import { useEffect, useId, useState } from "react";
import {
  PatientElsewhereReceipt,
  PatientElsewhereView,
  type PatientElsewhereReport,
  type PatientElsewhereSubmission,
} from "@/lib/api/patient-elsewhere-schema";
import { ELSEWHERE_COPY, type ElsewhereCopy } from "./elsewhere-copy";
import type { Lang } from "./i18n";

/*
 * The patient's "already done elsewhere" report (issue #177). It records a report for the care team to check and shows
 * where that check stands. It never says the screening is complete: only a retrieved report that passes the normal
 * identity-link, two-reader and human-review gates can close the case.
 */

const LOCALE: Record<Lang, string> = { ar: "ar-AE", en: "en-AE", ur: "ur-PK", fil: "fil-PH" };

const dayWords = (day: string, lang: Lang) =>
  new Intl.DateTimeFormat(LOCALE[lang], { day: "numeric", month: "long", timeZone: "UTC" }).format(new Date(`${day}T00:00:00Z`));

/** The one status sentence the patient sees for a report, from the server's retrieval status. */
export function elsewhereStatusLine(r: PatientElsewhereReport, t: ElsewhereCopy, lang: Lang): string {
  const fill = (s: string) => s.replace("{provider}", r.provider).replace("{due}", dayWords(r.retrieval_due, lang));
  if (r.status === "resumed" && r.outcome) return fill(t.resumed[r.outcome]);
  if (r.retrieval_status === "evidence_closed") return t.evidenceClosed;
  return fill(r.retrieval_status === "overdue" ? t.overdue : t.pending);
}

async function load(): Promise<PatientElsewhereView | null> {
  const res = await fetch("/patient/completed-elsewhere", { cache: "no-store" }).catch(() => null);
  const parsed = PatientElsewhereView.safeParse(res?.ok ? await res.json().catch(() => null) : null);
  return parsed.success ? parsed.data : null;
}

type Draft = { requestId: string; phase: "editing" | "sending" | "failed" };

export function ElsewhereReport({ lang, procedure }: { lang: Lang; procedure: string }) {
  const t = ELSEWHERE_COPY[lang];
  const id = useId();
  const [view, setView] = useState<PatientElsewhereView | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  useEffect(() => {
    void load().then((loaded) => loaded && setView(loaded));
  }, []);

  const submit = async (form: HTMLFormElement) => {
    if (!draft) return;
    const f = new FormData(form);
    const text = (k: string) => String(f.get(k) ?? "").trim();
    const body: PatientElsewhereSubmission = {
      request_id: draft.requestId,
      procedure: text("procedure"),
      performed: text("performed"),
      provider: text("provider"),
      ...(text("report_ref") ? { report_ref: text("report_ref") } : {}),
    };
    setDraft({ ...draft, phase: "sending" });
    const res = await fetch("/patient/completed-elsewhere", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) }).catch(() => null);
    const parsed = PatientElsewhereReceipt.safeParse(res?.ok ? await res.json().catch(() => null) : null);
    if (!parsed.success) {
      // Keep the same request id: a retry of a report that did reach the server returns its receipt, not a second one.
      setDraft({ ...draft, phase: "failed" });
      return;
    }
    setView(parsed.data.completed_elsewhere);
    setDraft(null);
  };

  const report = view?.report;
  const input = "min-h-11 rounded-control border border-line bg-surface px-3 text-[16px]";
  const button = "min-h-11 w-full cursor-pointer rounded-control border border-line bg-surface px-4 text-[16px] font-medium text-ink disabled:cursor-not-allowed disabled:opacity-50";
  const thisMonth = new Date().toISOString().slice(0, 7);

  return (
    <section aria-labelledby={`${id}-head`} className="flex flex-col gap-3">
      <h2 id={`${id}-head`} className="sr-only">
        {t.title}
      </h2>
      {report && (
        <div role="status" data-testid="elsewhere-status" className="rounded-card border border-line bg-surface p-4">
          <p className="m-0 text-[16px] leading-[1.7]">{elsewhereStatusLine(report, t, lang)}</p>
          {report.reported_by === "care_team" && <p className="m-0 mt-1 text-[14px] text-muted">{t.byCareTeam}</p>}
          <p className="m-0 mt-1 text-[14px] text-muted">{t.notResult}</p>
          <p className="m-0 mt-1 text-[13px] text-muted">
            {t.receipt}{" "}
            <bdi dir="ltr" className="tabular-nums">
              {report.receipt.slice(0, 8)}
            </bdi>
          </p>
        </div>
      )}
      {view?.can_report && !draft && (
        <button type="button" className={button} onClick={() => setDraft({ requestId: crypto.randomUUID(), phase: "editing" })}>
          {t.open}
        </button>
      )}
      {draft && (
        <form
          aria-labelledby={`${id}-form`}
          className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit(e.currentTarget);
          }}
        >
          <h3 id={`${id}-form`} className="m-0 text-[16px] font-medium">
            {t.title}
          </h3>
          <p className="m-0 text-[15px] leading-[1.7] text-muted">{t.intro}</p>
          <label className="flex flex-col gap-1.5 text-[15px]">
            {t.procedure}
            <input name="procedure" required maxLength={64} defaultValue={procedure} autoComplete="off" className={input} />
          </label>
          <label className="flex flex-col gap-1.5 text-[15px]">
            {t.performed}
            <input name="performed" type="month" required max={thisMonth} dir="ltr" aria-describedby={`${id}-when`} className={input} />
            <span id={`${id}-when`} className="text-[14px] text-muted">
              {t.performedHint}
            </span>
          </label>
          <label className="flex flex-col gap-1.5 text-[15px]">
            {t.provider}
            <input name="provider" required maxLength={120} autoComplete="off" className={input} />
          </label>
          <label className="flex flex-col gap-1.5 text-[15px]">
            {`${t.reportRef} (${t.optional})`}
            <input name="report_ref" maxLength={64} autoComplete="off" dir="ltr" className={input} />
          </label>
          <p className="m-0 text-[14px] text-muted">{t.notResult}</p>
          <button
            type="submit"
            disabled={draft.phase === "sending"}
            className="min-h-12 w-full cursor-pointer rounded-control border-0 bg-cta text-[16px] font-medium text-white disabled:cursor-not-allowed disabled:opacity-50"
          >
            {draft.phase === "sending" ? t.sending : t.submit}
          </button>
          <button type="button" className={button} onClick={() => setDraft(null)}>
            {t.cancel}
          </button>
          {draft.phase === "failed" && (
            <p role="alert" className="m-0 text-[14px] text-ink">
              {t.failed}
            </p>
          )}
        </form>
      )}
    </section>
  );
}
