"use client";

import { useRef, useState, useTransition, type FormEvent } from "react";
import { appendGovernanceAction } from "@/lib/api/governance-actions";
import type { GovernanceCredential } from "@/lib/api/source";

export type GovernanceOperation = "undo" | "contest" | "resolve";

const OPERATION_WORDS = {
  undo: "Append compensation",
  contest: "Contest review",
  resolve: "Resolve contest",
} as const;

/** Live-only controls. Fixture history never receives this component, so a preview cannot look persisted. */
export function GovernanceControls({
  caseId,
  target,
  operation,
  credentials,
}: {
  caseId: string;
  target: string;
  operation: GovernanceOperation;
  credentials: GovernanceCredential[];
}) {
  const [pending, startTransition] = useTransition();
  const [result, setResult] = useState<{ ok: boolean; message: string }>();
  const form = useRef<HTMLFormElement>(null);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    startTransition(async () => {
      const response = await appendGovernanceAction({
        caseId,
        target,
        operation,
        resolution: data.get("resolution") || undefined,
        reason: data.get("reason"),
        credential: data.get("credential"),
      });
      setResult({ ok: response.ok, message: response.ok ? `${response.message} Event ${response.hash.slice(0, 12)}.` : response.error });
      if (response.ok) form.current?.reset();
    });
  }

  return (
    <details className="mt-3 rounded-control bg-ground px-3 py-2">
      <summary className="min-h-8 cursor-pointer font-medium">{OPERATION_WORDS[operation]}</summary>
      <form ref={form} onSubmit={submit} className="mt-2 flex flex-col gap-3 border-t border-rule pt-3">
        <p className="m-0 text-small text-muted">
          {operation === "resolve"
            ? "Use a different authorised actor from both the original reviewer and the contester. The API rejects same-actor resolution."
            : "Original event remains unchanged: this appends to the review ledger and does not change the canonical decision."}
        </p>
        <label className="flex flex-col gap-1 text-small font-medium">
          Authorised credential
          <select name="credential" required disabled={pending} className="min-h-11 rounded-control border border-control bg-surface px-3 font-normal">
            <option value="">Choose a credential</option>
            {credentials.map((credential) => (
              <option key={credential} value={credential}>
                {credential === "reviewer" ? "Reviewer 1" : credential === "reviewer_2" ? "Reviewer 2" : credential === "reviewer_3" ? "Reviewer 3" : credential === "clinician" ? "Clinician" : "Coordinator"}
              </option>
            ))}
          </select>
        </label>
        {operation === "resolve" && (
          <label className="flex flex-col gap-1 text-small font-medium">
            Resolution
            <select name="resolution" required disabled={pending} className="min-h-11 rounded-control border border-control bg-surface px-3 font-normal">
              <option value="upheld">Uphold contest — compensate the original review</option>
              <option value="rejected">Reject contest — return the original review to active</option>
            </select>
          </label>
        )}
        <label className="flex flex-col gap-1 text-small font-medium">
          Reason
          <textarea
            name="reason"
            required
            minLength={1}
            maxLength={1000}
            rows={3}
            disabled={pending}
            className="resize-y rounded-control border border-control bg-surface px-3 py-2 font-normal"
          />
        </label>
        <button type="submit" disabled={pending} className="min-h-11 rounded-control bg-cta px-4 font-medium text-white hover:bg-ink disabled:opacity-60">
          {pending ? "Appending…" : OPERATION_WORDS[operation]}
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
