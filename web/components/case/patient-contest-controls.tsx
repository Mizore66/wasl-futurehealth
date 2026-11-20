"use client";

import { useRef, useState, useTransition, type FormEvent } from "react";
import { signOutForGovernance } from "@/lib/api/governance-actions";
import { resolvePatientContest } from "@/lib/api/patient-contest-actions";
import { SESSION_REJECTED, setSignedOutNotice } from "@/components/review/signed-out-notice";

/**
 * Live-only resolution of one open patient contest. There is no credential picker and no preselected outcome: the server
 * action acts as the signed-in token, and the API refuses any actor who is not a senior clinician or who signed the case.
 */
export function PatientContestControls({ caseId, contest, excluded }: { caseId: string; contest: string; excluded: string[] }) {
  const [pending, startTransition] = useTransition();
  const [result, setResult] = useState<{ ok: boolean; message: string }>();
  const form = useRef<HTMLFormElement>(null);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const outcome = data.get("outcome");
    const reason = String(data.get("reason") ?? "");
    startTransition(async () => {
      const response = await resolvePatientContest({ caseId, contest, outcome, reason });
      if (!response.ok && response.signIn) {
        setResult({ ok: false, message: SESSION_REJECTED });
        setSignedOutNotice(caseId, SESSION_REJECTED);
        await signOutForGovernance(caseId);
        return;
      }
      setResult({
        ok: response.ok,
        message: response.ok ? (response.hash ? `${response.message} Event ${response.hash.slice(0, 12)}.` : response.message) : `Not saved: ${response.error}`,
      });
      if (response.ok) form.current?.reset();
    });
  }

  return (
    <details className="mt-3 rounded-control bg-ground px-3 py-2">
      <summary className="min-h-8 cursor-pointer font-medium">Resolve patient contest</summary>
      <form ref={form} onSubmit={submit} className="mt-2 flex flex-col gap-3 border-t border-rule pt-3" aria-label="Resolve patient contest">
        <p className="m-0 text-small text-muted">
          You act as your signed-in credential, which must be a senior clinician who did not sign this case
          {excluded.length > 0 ? ` (not ${excluded.join(", ")})` : ""}. The API refuses anyone else. Prior evidence and signatures are not rewritten.
        </p>
        <label className="flex flex-col gap-1 text-small font-medium">
          Outcome
          <select name="outcome" required defaultValue="" disabled={pending} className="min-h-11 rounded-control border border-control bg-surface px-3 font-normal">
            <option value="">Choose an outcome</option>
            <option value="decision_upheld">Uphold the decision and release the closure hold</option>
            <option value="decision_revised">Revise the decision and keep closure held until it changes</option>
          </select>
        </label>
        <label className="flex flex-col gap-1 text-small font-medium">
          Reason for your decision (required)
          <textarea name="reason" required maxLength={500} disabled={pending} className="min-h-20 rounded-control border border-control bg-surface px-3 py-2 font-normal" />
          <span className="font-normal text-muted">Recorded with your sign-in, the outcome and the time. Staff see it. The patient sees the outcome.</span>
        </label>
        <button type="submit" disabled={pending} className="min-h-11 rounded-control bg-cta px-4 font-medium text-white hover:bg-ink disabled:opacity-60">
          {pending ? "Appending…" : "Record contest outcome"}
        </button>
        {result && (
          <p role={result.ok ? "status" : "alert"} className={result.ok ? "m-0 text-small text-positive" : "m-0 text-small text-negative"}>
            {result.message}
          </p>
        )}
      </form>
    </details>
  );
}
