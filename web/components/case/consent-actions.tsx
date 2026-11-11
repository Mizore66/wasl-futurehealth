"use client";

import { useId, useState, useTransition } from "react";
import { recordConsent } from "@/app/app/patients/[id]/actions";
import { Button } from "@/components/ui/button";
import type { ApiContact, ConsentEventName } from "@/lib/api/schema";
import { atWords, roleWords } from "./words";

const STATUS = {
  none: "No demo contact permission recorded",
  granted: "Demo contact permission granted",
  denied: "Demo contact permission denied",
  opted_out: "Demo contact opt-out recorded",
} as const;

const EVENT_WORDS: Record<ConsentEventName, string> = {
  consent_granted: "Consent granted",
  consent_denied: "Consent denied",
  consent_opted_out: "Opted out",
};

const ACTIONS: { event: ConsentEventName; label: string; confirmation: string; confirm: string }[] = [
  {
    event: "consent_granted",
    label: "Record consent granted",
    confirmation: "Releases the demo consent hold. No message is sent and the canonical simulation does not change.",
    confirm: "Confirm consent granted",
  },
  {
    event: "consent_denied",
    label: "Record consent denied",
    confirmation: "Cancels queued demo outreach. No message is sent and the canonical simulation does not change.",
    confirm: "Confirm consent denied",
  },
  {
    event: "consent_opted_out",
    label: "Record opt-out",
    confirmation:
      "Cancels queued demo outreach and blocks a later grant or denial in this demo flow; only another opt-out can follow. No message is sent and the canonical simulation does not change.",
    confirm: "Confirm opt-out",
  },
];

function OutreachState({ contact }: { contact: ApiContact }) {
  const pending = contact.overlay.demo_pending_consent;
  return (
    <div className="mt-3" role="status" aria-live="polite">
      <p className="m-0 font-medium">
        {pending === "released" ? "Demo outreach released" : pending === "cancelled" ? "Demo outreach cancelled" : "Demo consent hold unchanged"}
      </p>
      <p className="m-0 mt-1">
        {pending === "released"
          ? "The demo consent hold is released. No message was sent."
          : pending === "cancelled"
            ? "Queued demo outreach is cancelled. No message was sent."
            : "No release or cancellation has been recorded in this demo overlay."}
      </p>
    </div>
  );
}

/** Current server-derived demo contact state, its append-only history, and coordinator-only event controls. */
export function ConsentActions({
  caseId,
  expectedHead,
  contact,
  live,
  canWrite,
}: {
  caseId: string;
  expectedHead?: string;
  contact?: ApiContact;
  live: boolean;
  canWrite: boolean;
}) {
  const id = useId();
  const [saved, setSaved] = useState<ApiContact | null>(null);
  const [updatedHead, setUpdatedHead] = useState<string>();
  const [notes, setNotes] = useState<Partial<Record<ConsentEventName, string>>>({});
  const [message, setMessage] = useState<{ kind: "error" | "success" | "warning"; text: string } | null>(null);
  /** Set when the API refused an event because the patient had already opted out. */
  const [refusedOptOut, setRefusedOptOut] = useState(false);
  const [pending, startTransition] = useTransition();
  const current = saved ?? contact;
  const head = updatedHead ?? expectedHead;
  /* After an opt-out the API refuses everything except a repeat opt-out (pipeline/api_consent.go). */
  const optedOut = refusedOptOut || current?.overlay.status === "opted_out";
  const offered = optedOut ? ACTIONS.filter((action) => action.event === "consent_opted_out") : ACTIONS;

  function append(event: ConsentEventName) {
    if (pending || !live || !canWrite || !head) return;
    setMessage(null);
    startTransition(async () => {
      try {
        const result = await recordConsent(caseId, head, event, notes[event] ?? "");
        if (!result.ok) {
          /* A stale head carries current_head; the opt-out refusal does not and leaves the head valid. */
          const stale = result.status === 409 && !!result.currentHead;
          const refused = result.status === 409 && !result.currentHead;
          if (stale) setUpdatedHead("");
          if (refused) setRefusedOptOut(true);
          const text =
            result.status === 400
              ? `Not saved: ${result.error}`
              : result.status === 403
                ? "Not saved: the coordinator role is not authorised to record consent."
                : result.status === 404
                  ? "Not saved: the API does not know this case. Reload the case."
                  : stale
                    ? `Not saved: ${result.error}. Reload and review the latest case before retrying.`
                    : refused
                      ? "Not saved: the patient has opted out of contact in this demo flow, so consent cannot be granted or denied. Only another opt-out can be recorded. Reload to see the opt-out in the history."
                      : `Not saved: ${result.error}`;
          setMessage({ kind: "error", text });
          return;
        }
        if (result.contact) {
          setSaved(result.contact);
          setUpdatedHead(result.expectedHead || "");
          setMessage({
            kind: "success",
            text: "Consent event appended to the demo ledger. Canonical case data is unchanged and no message was sent.",
          });
        } else {
          setUpdatedHead("");
          setMessage({ kind: "warning", text: result.warning ?? "Consent event saved. Reload before another action." });
        }
      } catch {
        setMessage({ kind: "error", text: "The API did not answer. Reload before retrying; no new event is shown here." });
      }
    });
  }

  if (!current) return <p className="m-0 mt-3 text-muted">No demo contact overlay was included in this case snapshot.</p>;

  return (
    <div className="mt-4">
      <h3 className="m-0 text-body font-medium">Demo contact overlay</h3>
      <p className="m-0 mt-2">{STATUS[current.overlay.status]}</p>
      <OutreachState contact={current} />
      <p className="m-0 mt-2 text-small text-muted">
        Canonical consent and outreach are unchanged. No message is sent and the canonical simulation does not change.
      </p>

      {live && canWrite && head ? (
        <div className="mt-3 flex flex-col gap-2" aria-label="Record demo contact permission">
          {optedOut && (
            <p className="m-0 text-small text-muted">
              The patient has opted out of contact in this demo flow. Consent can no longer be granted or denied here; only another
              opt-out can be recorded.
            </p>
          )}
          {offered.map((action) => (
            <details key={action.event} className="rounded-control border border-control px-3 py-1">
              <summary className="min-h-11 cursor-pointer py-2 font-medium">{action.label}</summary>
              <p className="m-0 mt-1">{action.confirmation}</p>
              <label htmlFor={`${id}-${action.event}`} className="mt-3 block text-small font-medium">
                Coordinator note (optional)
              </label>
              <textarea
                id={`${id}-${action.event}`}
                value={notes[action.event] ?? ""}
                maxLength={500}
                rows={2}
                disabled={pending}
                onChange={(event) => setNotes((previous) => ({ ...previous, [action.event]: event.target.value }))}
                className="mt-1 w-full rounded-control border border-control bg-surface px-3 py-2 text-ink"
              />
              <Button type="button" variant="quiet" size="touch" disabled={pending} onClick={() => append(action.event)} className="mt-2 mb-2">
                {pending ? "Recording…" : action.confirm}
              </Button>
            </details>
          ))}
        </div>
      ) : (
        <p className="m-0 mt-3 text-small text-muted">
          {!live
            ? "Recording consent needs the live API. This snapshot is read-only."
            : !canWrite
              ? "Consent actions are unavailable because the case ledger did not validate."
              : updatedHead === ""
                ? "Consent actions are paused. Reload the latest case before recording another event."
                : "Consent actions are unavailable because the case head signature is missing."}
        </p>
      )}
      <div className="min-h-6" aria-live="polite">
        {message && (
          <p role={message.kind === "error" ? "alert" : "status"} className={message.kind === "error" ? "m-0 mt-3 text-negative" : "m-0 mt-3"}>
            {message.text}
          </p>
        )}
      </div>

      <h4 className="m-0 mt-4 text-body font-medium">Demo consent history</h4>
      {current.history.length === 0 ? (
        <p className="m-0 mt-2 text-muted">No demo consent events recorded.</p>
      ) : (
        <ol className="m-0 mt-1 list-none p-0" aria-label="Demo consent history">
          {current.history.map((event) => (
            <li key={event.hash} className="border-t border-rule py-3">
              <p className="m-0 font-medium">{EVENT_WORDS[event.decision]}</p>
              <p className="m-0 text-small text-muted">
                {atWords(event.at)} · {event.actor} · {roleWords(event.role)}
              </p>
              {event.note && <p className="m-0 mt-1">&ldquo;{event.note}&rdquo;</p>}
              <code className="mt-1 block break-all text-small text-muted">{event.hash}</code>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
