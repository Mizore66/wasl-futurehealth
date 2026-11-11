"use client";

import { useId, useState, useTransition } from "react";
import { recordConsent } from "@/app/app/patients/[id]/actions";
import { Button } from "@/components/ui/button";
import type { ConsentEvent, ConsentOverlay } from "@/lib/api/schema";
import { formatUtcInstant } from "@/lib/domain/day";
import { CONSENT_ACTIONS, DEMO_OVERLAY_WORDS, availability, failureView, type FailureView } from "./consent-model";

type Saved = { label: string; actor: string; at: string };

/**
 * Grant, deny and opt-out for the consent demo overlay (issue #42). Each asks for confirmation, then a server action
 * appends the event with the coordinator credential; the page re-reads the case on success. Refusals stay in place.
 * Without the live API the buttons are disabled and say why: nothing is kept in the browser as if it were saved.
 */
export function ConsentActions({
  caseId,
  head,
  live,
  status,
}: {
  caseId: string;
  head: string | null;
  live: boolean;
  status: ConsentOverlay["status"] | null;
}) {
  const id = useId();
  const [confirming, setConfirming] = useState<ConsentEvent | null>(null);
  const [saved, setSaved] = useState<Saved | null>(null);
  const [failure, setFailure] = useState<FailureView | null>(null);
  const [saving, startSaving] = useTransition();

  const avail = CONSENT_ACTIONS.map((a) => ({ action: a, ...availability(a.event, { live, head, status }) }));
  const reasons = [...new Set(avail.filter((a) => !a.enabled && a.reason).map((a) => a.reason!))];
  const pending = CONSENT_ACTIONS.find((a) => a.event === confirming);

  function submit(event: ConsentEvent, label: string) {
    if (!head) return;
    setFailure(null);
    setSaved(null);
    startSaving(async () => {
      try {
        const res = await recordConsent(caseId, head, event);
        if (res.ok) {
          setSaved({ label, actor: res.actor, at: res.at });
          setConfirming(null);
        } else {
          setFailure(failureView(res));
          setConfirming(null);
        }
      } catch {
        setFailure({ title: "Nothing was recorded", detail: "The API could not be reached or answered unexpectedly.", reload: false });
        setConfirming(null);
      }
    });
  }

  return (
    <div className="mt-3" data-testid="consent-actions">
      <h4 id={`${id}-h`} className="m-0 text-body font-medium">
        Record what the patient said
      </h4>
      <div role="group" aria-labelledby={`${id}-h`} className="mt-2 flex flex-wrap gap-2">
        {avail.map(({ action, enabled }) => (
          <Button
            key={action.event}
            variant="chip"
            size="touch"
            className={confirming === action.event ? "border-ink" : "border-control"}
            disabled={!enabled || saving}
            aria-expanded={confirming === action.event}
            aria-controls={`${id}-confirm`}
            onClick={() => {
              setFailure(null);
              setSaved(null);
              setConfirming(action.event);
            }}
          >
            {action.label}
          </Button>
        ))}
      </div>
      {reasons.map((r) => (
        <p key={r} className="m-0 mt-2 text-small text-muted" data-testid="consent-disabled-reason">
          {r}
        </p>
      ))}

      <div id={`${id}-confirm`}>
        {pending && (
          <div role="group" aria-labelledby={`${id}-confirm-h`} className="mt-3 rounded-control border border-line p-3">
            <p id={`${id}-confirm-h`} className="m-0 font-medium">
              {pending.confirmTitle}
            </p>
            <p className="m-0 mt-1">{pending.confirmBody}</p>
            <p className="m-0 mt-1 text-small text-muted">{DEMO_OVERLAY_WORDS}</p>
            <div className="mt-3 flex flex-wrap gap-2">
              <Button variant="cta" size="touch" disabled={saving} aria-busy={saving} onClick={() => submit(pending.event, pending.label)}>
                {saving ? "Recording…" : pending.confirmLabel}
              </Button>
              <Button variant="quiet" size="touch" disabled={saving} onClick={() => setConfirming(null)}>
                Cancel
              </Button>
            </div>
          </div>
        )}
      </div>

      <div role="status" className="empty:hidden">
        {saved && (
          <p className="m-0 mt-3" data-testid="consent-saved">
            Recorded on the case ledger: {saved.label} · {saved.actor} · {formatUtcInstant(saved.at)}. Demo overlay only: nothing was sent.
          </p>
        )}
      </div>
      {failure && (
        <div role="alert" className="mt-3 rounded-control border-2 border-warning p-3" data-testid="consent-refused">
          <p className="m-0 font-medium">{failure.title}</p>
          <p className="m-0 mt-1">{failure.detail}</p>
          {failure.reload && (
            <Button
              variant="quiet"
              size="touch"
              className="mt-2"
              onClick={() => window.location.reload()}
            >
              Reload the case
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
