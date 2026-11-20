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
    confirmation: "Lifts the demo consent hold. No message is sent.",
    confirm: "Confirm consent granted",
  },
  {
    event: "consent_denied",
    label: "Record consent denied",
    confirmation: "Cancels queued demo messages. No message is sent.",
    confirm: "Confirm consent denied",
  },
  {
    event: "consent_opted_out",
    label: "Record opt-out",
    confirmation:
      "Cancels queued demo messages and blocks any later grant or denial. Only another opt-out can follow. No message is sent.",
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
            : "No release or cancellation recorded yet."}
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
                  ? "Not saved: this case could not be found. Reload the case."
                  : stale
                    ? `Not saved: ${result.error}. Reload and review the latest case before retrying.`
                    : refused
                      ? "Not saved: the patient has opted out of contact, so consent cannot be granted or denied. Only another opt-out can be recorded. Reload to see the opt-out in the history."
                      : `Not saved: ${result.error}`;
          setMessage({ kind: "error", text });
          return;
        }
        if (result.contact) {
          setSaved(result.contact);
          setUpdatedHead(result.expectedHead || "");
          setMessage({
            kind: "success",
            text: "Consent recorded. The case record is unchanged and no message was sent.",
          });
        } else {
          setUpdatedHead("");
          setMessage({ kind: "warning", text: result.warning ?? "Consent saved. Reload the page before doing anything else." });
        }
      } catch {
        setMessage({ kind: "error", text: "We couldn't confirm the save. Reload the page before trying again." });
      }
    });
  }

  if (!current) return <p className="m-0 mt-3 text-muted">No demo contact permission is recorded for this case.</p>;

  return (
    <div className="mt-4">
      <h3 className="m-0 text-body font-medium">Demo contact permission</h3>
      <p className="m-0 mt-2">{STATUS[current.overlay.status]}</p>
      <OutreachState contact={current} />
      <p className="m-0 mt-2 text-small text-muted">
        This does not change the case record. No message is sent.
      </p>

      {live && canWrite && head ? (
        <div className="mt-3 flex flex-col gap-2" aria-label="Record demo contact permission">
          {optedOut && (
            <p className="m-0 text-small text-muted">
              The patient has opted out of contact. Consent can no longer be granted or denied here; only another opt-out can be
              recorded.
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
            ? "Consent can't be recorded here. This view is read-only."
            : !canWrite
              ? "Consent can't be recorded: this case's record failed its tamper check."
              : updatedHead === ""
                ? "Reload the page before recording another change."
                : "Consent can't be recorded for this case right now. Reload the page."}
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
        <p className="m-0 mt-2 text-muted">No demo consent changes recorded.</p>
      ) : (
        <ol className="m-0 mt-1 list-none p-0" aria-label="Demo consent history">
          {current.history.map((event) => {
            const patient = "type" in event && event.type === "patient_journey";
            const decision = event.decision;
            const actor = "actor" in event && typeof event.actor === "string" ? event.actor : "Actor not recorded";
            const role = "role" in event && typeof event.role === "string" ? event.role : "";
            return (
              <li key={event.hash} className="border-t border-rule py-3">
                <p className="m-0 font-medium">
                  {patient
                    ? `Patient ${decision === "consent_opted_out" ? "opted out of" : decision === "consent_granted" ? "granted" : decision === "consent_denied" ? "denied" : "changed"} contact permission`
                    : decision
                      ? (EVENT_WORDS[decision as keyof typeof EVENT_WORDS] ?? "Contact permission changed")
                      : "Contact permission changed"}
                </p>
                <p className="m-0 text-small text-muted">
                  {atWords(event.at)} · {patient ? "Patient self-service" : `${actor} · ${roleWords(role)}`}
                </p>
                {"note" in event && typeof event.note === "string" && event.note && <p className="m-0 mt-1">&ldquo;{event.note}&rdquo;</p>}
              </li>
            );
          })}
        </ol>
      )}
    </div>
  );
}
