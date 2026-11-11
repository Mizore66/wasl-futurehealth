"use client";

import { useId, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { submitDoubleRead, type DoubleReadOutcome } from "@/app/app/review/[id]/double-review-actions";
import type { ReadName } from "@/lib/api/schema";
import { cn } from "@/lib/utils";
import { refusalWords, type Choice } from "./model";

/**
 * One read for one item, sent with this seat's server-held credential. The page shows only what the API has recorded:
 * after a 201 it re-reads the round from the API, so the confirmation is the API's own account of who gave the read.
 */
export function DoubleReadForm({
  caseId,
  item,
  question,
  seat,
  head,
  choices,
  senior,
  recordedAs,
  legend,
}: {
  caseId: string;
  item: string;
  question: "link" | "evidence";
  seat: ReadName;
  head: string;
  choices: Choice[];
  senior: boolean;
  /** The actor the API bound to this seat's credential (GET /double-review `viewer`). */
  recordedAs: string;
  legend: string;
}) {
  const [pick, setPick] = useState<string>();
  const [result, setResult] = useState<DoubleReadOutcome>();
  const [saving, start] = useTransition();
  const router = useRouter();
  const id = useId();

  function send() {
    if (!pick || saving) return;
    start(async () => {
      const res = await submitDoubleRead({ caseId, item, question, seat, decision: pick, expectedHead: head });
      setResult(res);
      if (res.ok || res.conflict === "head") router.refresh();
    });
  }

  return (
    <fieldset className="m-0 mt-3 min-w-0 border-0 p-0" aria-describedby={`${id}-who`} data-testid={`double-read-form-${item}`}>
      <legend className="p-0 text-body font-medium">{legend}</legend>
      <p id={`${id}-who`} className="m-0 mt-0.5 text-small text-muted">
        {senior
          ? `Senior read, recorded as ${recordedAs}. Both initial answers are shown above.`
          : `Your ${seat} answer, recorded as ${recordedAs}. The other initial answer stays hidden until yours is in.`}
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        {choices.map((c) => (
          <label
            key={c.value}
            className={cn(
              "relative flex min-h-11 min-w-[7rem] flex-1 cursor-pointer items-center justify-center rounded-chip px-3 py-1 text-center text-ink has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-hue",
              pick === c.value ? "border-2 border-ink bg-active font-medium" : "border border-control bg-surface hover:bg-active",
            )}
          >
            <input
              type="radio"
              name={`${id}-answer`}
              value={c.value}
              checked={pick === c.value}
              disabled={saving}
              onChange={() => setPick(c.value)}
              className="absolute -inset-0.5 m-0 size-[calc(100%+4px)] cursor-pointer opacity-0"
            />
            {c.label}
          </label>
        ))}
      </div>
      <button
        type="button"
        onClick={send}
        aria-disabled={!pick || saving}
        className="mt-3 min-h-11 rounded-control bg-cta px-4 font-medium text-white hover:bg-ink aria-disabled:opacity-60"
      >
        {saving ? "Saving…" : senior ? "Record the senior read" : `Record ${seat}`}
      </button>
      {result && !result.ok && (
        <p role="alert" className="m-0 mt-2 rounded-control bg-pill-negative-bg px-3 py-2 text-pill-negative-ink">
          {refusalWords(result)}
        </p>
      )}
      {result?.ok && (
        <p role="status" className="m-0 mt-2 text-small text-muted">
          Saved: {result.recorded.read} recorded by {result.recorded.actor} ({result.recorded.role}), hash{" "}
          <span className="font-mono">{result.recorded.hash.slice(0, 12)}</span>.
        </p>
      )}
    </fieldset>
  );
}
