"use client";

import { useState, useTransition } from "react";
import { recordTeachBackCallback } from "@/app/app/patients/[id]/actions";
import { Button } from "@/components/ui/button";
import { HONESTY_PLAIN, HonestyLabel } from "@/components/wasl/honesty-label";
import type { TeachBackCallback } from "@/lib/api/schema";
import { staffDueTime } from "@/lib/domain/staff-time";
import { atWords } from "./words";

const STATE: Record<TeachBackCallback["state"], string> = {
  misunderstood: "Misunderstood the next step",
  needs_human: "Asked for a person",
  unparsed: "Reply could not be read",
};

export function callbackStatus(cb: TeachBackCallback): string {
  if (cb.status === "resolved") return `Resolved${cb.resolved_by ? ` by ${cb.resolved_by}` : ""}${cb.resolved_at ? ` ${atWords(cb.resolved_at)}` : ""}`;
  if (!cb.due_at) return `Open · ${cb.owner} callback, no due date`;
  return `${cb.overdue ? "Overdue" : "Open"} · ${cb.owner} callback due ${staffDueTime(cb.due_at)}`;
}

/**
 * The callbacks to show right after a recorded outcome, before revalidation re-renders the page: the API's own
 * post-append list when it sent one, otherwise the recorded item marked resolved ("understood" is the only outcome
 * that resolves a callback; "another explanation" leaves it open).
 */
export function afterOutcome(
  callbacks: TeachBackCallback[],
  callback: string,
  outcome: "understood_on_call" | "another_explanation",
  returned?: TeachBackCallback[],
): TeachBackCallback[] {
  if (returned) return returned;
  if (outcome !== "understood_on_call") return callbacks;
  return callbacks.map((cb) => (cb.id === callback ? { ...cb, status: "resolved" as const, overdue: false } : cb));
}

/** Failed teach-back callbacks. Only a recorded consultation outcome resolves one; a generic call log does not. */
export function TeachBackCallbackCard({ caseId, callbacks, live }: { caseId: string; callbacks: TeachBackCallback[]; live: boolean }) {
  const [pending, startTransition] = useTransition();
  const [message, setMessage] = useState<{ error: boolean; text: string } | null>(null);
  /* An optimistic list holds only while the page still renders the props it was derived from; revalidation replaces them. */
  const [recorded, setRecorded] = useState<{ base: TeachBackCallback[]; list: TeachBackCallback[] } | null>(null);
  const shown = recorded && recorded.base === callbacks ? recorded.list : callbacks;
  if (shown.length === 0) return null;

  function record(callback: string, outcome: "understood_on_call" | "another_explanation") {
    setMessage(null);
    startTransition(async () => {
      const result = await recordTeachBackCallback({ caseId, callback, outcome }).catch(() => null);
      if (!result) return setMessage({ error: true, text: "We couldn't confirm the save. Reload the page before trying again." });
      if (!result.ok) return setMessage({ error: true, text: `Not saved: ${result.error}` });
      setRecorded({ base: callbacks, list: afterOutcome(shown, callback, outcome, result.callbacks) });
      setMessage({
        error: false,
        text: outcome === "understood_on_call" ? "Callback resolved. The case decision is unchanged." : "Another explanation recorded. The callback stays open.",
      });
    });
  }

  return (
    <section id="teach-back-callbacks" aria-labelledby="teach-back-callbacks-h" className="min-w-0 scroll-mt-4 rounded-card border border-line bg-surface p-4 md:p-pad">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id="teach-back-callbacks-h" className="m-0 text-section font-medium">Teach-back nurse callbacks</h2>
        <HonestyLabel kind="simulated" className={HONESTY_PLAIN}>No call is placed.</HonestyLabel>
      </div>
      <ol className="m-0 mt-2 list-none p-0" aria-label="Teach-back callbacks">
        {shown.map((cb) => (
          <li key={cb.id} className="border-t border-rule py-3">
            <p className="m-0 font-medium">{STATE[cb.state]}</p>
            <p className="m-0 mt-1">{callbackStatus(cb)}</p>
            <p className="m-0 mt-1 text-small text-muted">
              {cb.source === "engine" ? "Raised automatically" : `Patient link${cb.opened_at ? ` · opened ${atWords(cb.opened_at)}` : ""}`} · {cb.escalation}
            </p>
            {(cb.consultations ?? []).map((k) => (
              <p key={k.event} className="m-0 mt-1 text-small">
                {k.outcome === "understood_on_call" ? "Understood on the call" : "Another explanation needed"} · {k.actor} ({k.role}) · {atWords(k.at)}
              </p>
            ))}
            {cb.status === "open" && live && (
              <div className="mt-2 flex flex-wrap gap-2">
                <Button type="button" variant="quiet" size="touch" disabled={pending} onClick={() => record(cb.id, "understood_on_call")}>
                  Record: understood on the call
                </Button>
                <Button type="button" variant="quiet" size="touch" disabled={pending} onClick={() => record(cb.id, "another_explanation")}>
                  Record: another explanation needed
                </Button>
              </div>
            )}
          </li>
        ))}
      </ol>
      {!live && <p className="m-0 mt-2 text-small text-muted">Call outcomes can&apos;t be recorded here. This view is read-only.</p>}
      <div className="min-h-6" aria-live="polite">
        {message && <p role={message.error ? "alert" : "status"} className={message.error ? "m-0 mt-2 text-negative" : "m-0 mt-2"}>{message.text}</p>}
      </div>
    </section>
  );
}
