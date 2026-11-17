"use client";

import { useId, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { resolvePatientContest } from "@/app/app/patients/[id]/patient-contest/actions";
import { OUTCOME_WORDS, type ContestOutcome } from "./model";

const AREA = "mt-1 min-h-24 w-full rounded-control border border-control bg-surface px-3 py-2 text-ink";
const SUBMIT = "min-h-11 self-start rounded-control bg-cta px-4 font-medium text-white hover:bg-ink aria-disabled:opacity-60";

/**
 * Records an independent reviewer's outcome and reason for one open patient contest. Identity is never a field: the
 * server action uses the signed-in credential only, and the API refuses every original reviewer of the case.
 */
export function ContestReviewForm({ caseId, contest, open }: { caseId: string; contest: string; open: boolean }) {
  const id = useId();
  const router = useRouter();
  const [pending, start] = useTransition();
  const [error, setError] = useState<string>();
  const [saved, setSaved] = useState<string>();

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const outcome = String(form.get("outcome") ?? "") as ContestOutcome;
    const reason = String(form.get("reason") ?? "").trim();
    if (!outcome || !reason) {
      setError("Choose an outcome and give the reason for your decision.");
      return;
    }
    start(async () => {
      const res = await resolvePatientContest({ caseId, resolution: { contest, outcome, reason } });
      if (!res.ok) {
        setError(res.error);
        setSaved(undefined);
        return;
      }
      setError(undefined);
      setSaved(res.message);
      router.refresh();
    });
  }

  if (!open) {
    /* Same component instance after the refresh, so the confirmation stays announced once the row shows the outcome. */
    return saved ? (
      <p role="status" className="m-0 mt-2 rounded-control bg-pill-positive-bg px-3 py-2 text-pill-positive-ink" data-testid="contest-saved">
        {saved}
      </p>
    ) : null;
  }
  return (
    <form onSubmit={submit} className="mt-3 flex flex-col gap-3" aria-label={`Review contest ${contest.slice(0, 8)}`}>
      <fieldset className="m-0 border-0 p-0">
        <legend className="font-medium">Outcome (required)</legend>
        {(Object.keys(OUTCOME_WORDS) as ContestOutcome[]).map((value) => (
          <label key={value} className="mt-1 flex min-h-11 items-center gap-2">
            <input type="radio" name="outcome" value={value} required />
            {OUTCOME_WORDS[value]}
          </label>
        ))}
      </fieldset>
      <div>
        <label htmlFor={`${id}-reason`} className="block font-medium">
          Reason for your decision (required)
        </label>
        <p className="m-0 text-small text-muted">Recorded with your sign-in, the outcome and the time. The patient sees the outcome, not this note.</p>
        <textarea id={`${id}-reason`} name="reason" required maxLength={500} className={AREA} />
      </div>
      {error && (
        <p role="alert" className="m-0 rounded-control bg-pill-negative-bg px-3 py-2 text-pill-negative-ink" data-testid="contest-error">
          {error}
        </p>
      )}
      {saved && (
        <p role="status" className="m-0 rounded-control bg-pill-positive-bg px-3 py-2 text-pill-positive-ink" data-testid="contest-saved">
          {saved}
        </p>
      )}
      <button type="submit" aria-disabled={pending} disabled={pending} className={SUBMIT}>
        Record contest review
      </button>
    </form>
  );
}
