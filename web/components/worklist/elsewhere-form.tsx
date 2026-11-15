"use client";

import { useId } from "react";
import { Button } from "@/components/ui/button";
import type { CaseAction } from "@/lib/api/schema";

export type ElsewhereAction = Extract<CaseAction, { action: "completed_elsewhere" }>;

const SOURCES: [ElsewhereAction["source"], string][] = [
  ["patient", "The patient"],
  ["carer", "A carer"],
  ["other_provider", "The other provider"],
];
/** Default retrieval window from the day the report is recorded. */
export const RETRIEVAL_DAYS = 14;
const INPUT = "min-h-11 w-full rounded-control border border-control bg-surface px-3 text-ink";

/**
 * What the patient (or someone for them) said was done elsewhere, and who retrieves the record by when. Recording it
 * suspends chase and booking until a retrieval outcome; it never closes the case.
 */
export function ElsewhereForm({ owner, procedure, nowMs, disabled, onRecord, onCancel }: {
  owner: string; procedure: string; nowMs: number; disabled: boolean;
  onRecord: (action: ElsewhereAction) => void; onCancel: () => void;
}) {
  const id = useId();
  const due = new Date(nowMs + RETRIEVAL_DAYS * 86_400_000).toISOString().slice(0, 10);
  const field = (name: string, label: string, input: React.ReactNode) => (
    <div className="flex flex-col gap-1">
      <label htmlFor={`${id}-${name}`} className="text-small text-muted">{label}</label>
      {input}
    </div>
  );
  return (
    <form
      aria-label="Done elsewhere"
      className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2"
      onSubmit={(e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        const text = (k: string) => String(f.get(k) ?? "").trim();
        onRecord({
          action: "completed_elsewhere",
          procedure: text("procedure"),
          performed: text("performed"),
          provider: text("provider"),
          source: text("source") as ElsewhereAction["source"],
          ...(text("report_ref") ? { report_ref: text("report_ref") } : {}),
          retrieval_owner: text("retrieval_owner"),
          retrieval_due: text("retrieval_due"),
        });
      }}
    >
      {field("procedure", "Procedure", <input id={`${id}-procedure`} name="procedure" required maxLength={64} defaultValue={procedure} autoComplete="off" className={INPUT} />)}
      {field("performed", "Roughly when (month)", <input id={`${id}-performed`} name="performed" type="month" required max={new Date(nowMs).toISOString().slice(0, 7)} className={INPUT} />)}
      {field("provider", "Where", <input id={`${id}-provider`} name="provider" required maxLength={120} autoComplete="off" className={INPUT} />)}
      {field("source", "Reported by", (
        <select id={`${id}-source`} name="source" required defaultValue="patient" className={INPUT}>
          {SOURCES.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select>
      ))}
      {field("report_ref", "Report reference, if known", <input id={`${id}-report_ref`} name="report_ref" maxLength={64} autoComplete="off" className={INPUT} />)}
      {field("retrieval_owner", "Retrieves the record", <input id={`${id}-retrieval_owner`} name="retrieval_owner" required maxLength={64} defaultValue={owner} autoComplete="off" className={INPUT} />)}
      {field("retrieval_due", "Record due by", <input id={`${id}-retrieval_due`} name="retrieval_due" type="date" required defaultValue={due} className={INPUT} />)}
      <p className="m-0 self-end text-small text-muted sm:col-span-2">Calls and booking pause until the record is retrieved. This does not close the case.</p>
      <div className="flex flex-wrap gap-2 sm:col-span-2">
        <Button type="submit" variant="cta" size="touch" disabled={disabled}>Record and request the report</Button>
        <Button type="button" variant="quiet" size="touch" onClick={onCancel}>Cancel</Button>
      </div>
    </form>
  );
}
