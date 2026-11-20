"use client";

import { useState, useTransition, type FormEvent } from "react";
import { signOutForGovernance } from "@/lib/api/governance-actions";
import { approveOutcomeHandoff, releaseOutcomeHandoff, type OutcomeHandoffActionResult } from "@/lib/api/outcome-handoff-actions";
import { SESSION_REJECTED, setSignedOutNotice } from "@/components/review/signed-out-notice";
import { NEXT_STEP_WORDS } from "./outcome-handoff-words";

type Result = { ok: boolean; message: string };

function Outcome({ result }: { result?: Result }) {
  if (!result) return null;
  return (
    <p role={result.ok ? "status" : "alert"} className={result.ok ? "m-0 text-small text-positive" : "m-0 text-small text-negative"}>
      {result.message}
    </p>
  );
}

const shown = (r: OutcomeHandoffActionResult): Result =>
  r.ok ? { ok: true, message: r.hash ? `${r.message} Event ${r.hash.slice(0, 12)}.` : r.message } : { ok: false, message: r.error };

/**
 * Clinician approval of the current decision's patient content. No credential picker and no preselected next step: the
 * server action acts as the signed-in token and the API allows only a clinician.
 */
export function OutcomeHandoffApproveForm({
  caseId,
  decision,
  version,
  nextSteps,
  current,
}: {
  caseId: string;
  decision: string;
  /** The approval shown on the page ("" for none). */
  version: string;
  nextSteps: string[];
  current?: string;
}) {
  const [pending, startTransition] = useTransition();
  const [result, setResult] = useState<Result>();

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextStep = new FormData(event.currentTarget).get("next_step");
    startTransition(async () => {
      const response = await approveOutcomeHandoff({ caseId, decision, nextStep, version });
      if (!response.ok && response.signIn) {
        setResult({ ok: false, message: SESSION_REJECTED });
        setSignedOutNotice(caseId, SESSION_REJECTED);
        await signOutForGovernance(caseId);
        return;
      }
      setResult(shown(response));
    });
  }

  return (
    <details className="mt-3 rounded-control bg-ground px-3 py-2" data-testid="outcome-handoff-approve">
      <summary className="min-h-8 cursor-pointer font-medium">{version ? "Approve revised content" : "Approve content"}</summary>
      <form onSubmit={submit} className="mt-2 flex flex-col gap-3 border-t border-rule pt-3" aria-label="Approve outcome hand-off content">
        <p className="m-0 text-small text-muted">
          Only a clinician can approve. You approve the current decision, its end state, the next step you choose and the standard
          safety-net notice. A new approval creates a new version, and earlier acknowledgements stay in the history.
        </p>
        <label className="flex flex-col gap-1 text-small font-medium">
          Next step for the patient
          <select name="next_step" required defaultValue="" disabled={pending} className="min-h-11 rounded-control border border-control bg-surface px-3 font-normal">
            <option value="">Choose a next step</option>
            {nextSteps.map((step) => (
              <option key={step} value={step}>
                {NEXT_STEP_WORDS[step] ?? step}
                {step === current ? " (currently approved)" : ""}
              </option>
            ))}
          </select>
        </label>
        <button type="submit" disabled={pending} className="min-h-11 rounded-control bg-cta px-4 font-medium text-white hover:bg-ink disabled:opacity-60">
          {pending ? "Recording…" : "Record approval"}
        </button>
        <Outcome result={result} />
      </form>
    </details>
  );
}

/** The coordinator's simulated release of the approved version. The server checks consent and records the gate it found. */
export function OutcomeHandoffReleaseButton({ caseId, version, again }: { caseId: string; version: string; again: boolean }) {
  const [pending, startTransition] = useTransition();
  const [result, setResult] = useState<Result>();

  function release() {
    startTransition(async () => setResult(shown(await releaseOutcomeHandoff({ caseId, version }))));
  }

  return (
    <div className="mt-3 flex flex-col gap-2 rounded-control bg-ground px-3 py-3" data-testid="outcome-handoff-release">
      <p className="m-0 text-small text-muted">
        Recorded as the coordinator. Consent is checked now: if granted, the patient link shows the result; if unknown, it is held;
        if denied or withdrawn, it is not shown. No message is sent.
      </p>
      <button
        type="button"
        onClick={release}
        disabled={pending}
        className="min-h-11 self-start rounded-control border border-control bg-surface px-4 font-medium text-ink hover:bg-active disabled:opacity-60"
      >
        {pending ? "Recording…" : again ? "Record release again (re-check consent)" : "Record simulated release"}
      </button>
      <Outcome result={result} />
    </div>
  );
}
