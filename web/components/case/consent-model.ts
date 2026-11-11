import type { ConsentEvent, ConsentHistoryEvent, ConsentOverlay } from "@/lib/api/schema";

/*
 * What the consent actions on the case page offer and say (issue #42). Pure, so the server card, the client actions and
 * the tests share one source. The rules mirror pipeline/api_consent.go; the API stays the authority and its refusals
 * are shown as they come.
 */

/** Said wherever the overlay appears: it is a demo view, not a change to the simulation. */
export const DEMO_OVERLAY_WORDS =
  "Demo overlay: recorded on the case ledger, it never changes the simulation's consent, outreach or decisions, and no message is sent.";

export type ConsentAction = { event: ConsentEvent; label: string; confirmTitle: string; confirmBody: string; confirmLabel: string };

export const CONSENT_ACTIONS: readonly ConsentAction[] = [
  {
    event: "consent_granted",
    label: "Consent granted",
    confirmTitle: "Record that the patient allowed contact?",
    confirmBody:
      "Only record this if the patient gave permission to be contacted about screening follow-up. The demo then shows the held message as released.",
    confirmLabel: "Record consent granted",
  },
  {
    event: "consent_denied",
    label: "Consent denied",
    confirmTitle: "Record that the patient declined contact?",
    confirmBody:
      "The demo then shows queued outreach as cancelled and refuses a contact confirmation. A later grant can still be recorded.",
    confirmLabel: "Record consent denied",
  },
  {
    event: "consent_opted_out",
    label: "Opted out",
    confirmTitle: "Record that the patient opted out of contact?",
    confirmBody:
      "The demo then shows queued outreach as cancelled and refuses a contact confirmation. After an opt-out the API refuses a later grant here: that needs a new, separately recorded consent flow.",
    confirmLabel: "Record opt-out",
  },
];

export type Availability = { enabled: boolean; reason?: string };

/** Whether each action can be sent now. Without the live API or a head to check against, nothing is offered as working. */
export function availability(event: ConsentEvent, ctx: { live: boolean; head: string | null; status: ConsentOverlay["status"] | null }): Availability {
  if (!ctx.live) return { enabled: false, reason: "Needs the live API: fixtures mode has no ledger to record consent on." };
  if (!ctx.head) return { enabled: false, reason: "The case has no decision signature to check the write against." };
  if (ctx.status === "opted_out" && event !== "consent_opted_out") {
    return { enabled: false, reason: "The patient opted out: a later grant or denial needs a new, separately recorded consent flow." };
  }
  return { enabled: true };
}

export type FailureView = { title: string; detail: string; reload: boolean };

/** A refusal shown in place. A 409 with a current head is a stale page: offer a reload rather than a retry. */
export function failureView(f: { status: number; error: string; currentHead?: string }): FailureView {
  switch (f.status) {
    case 409:
      return f.currentHead
        ? { title: "This case changed since the page loaded", detail: "Nothing was recorded. Reload to see the current case, then decide again.", reload: true }
        : { title: "The API refused this consent event", detail: `Nothing was recorded. ${f.error}`, reload: false };
    case 400:
      return { title: "The API rejected the request", detail: `Nothing was recorded. ${f.error}`, reload: false };
    case 403:
      return { title: "This role may not record consent", detail: `Nothing was recorded. ${f.error}`, reload: false };
    case 404:
      return { title: "The API does not know this case", detail: "Nothing was recorded. The run may have been reloaded.", reload: true };
    case 503:
      return { title: "No live API", detail: f.error, reload: false };
    default:
      return { title: "Nothing was recorded", detail: f.error, reload: false };
  }
}

const EVENT_WORDS: Record<ConsentEvent, string> = {
  consent_granted: "Consent granted",
  consent_denied: "Consent denied",
  consent_opted_out: "Opted out",
};
const EFFECT_WORDS: Record<ConsentEvent, string> = {
  consent_granted: "demo hold released",
  consent_denied: "demo outreach cancelled",
  consent_opted_out: "demo outreach cancelled",
};

export type HistoryRow = { key: string; what: string; effect: string; actor: string; role: string; at: string; note: string };

/** The overlay's events, oldest first, exactly as the ledger holds them: nothing merged, reordered or dropped. */
export function historyRows(history: ConsentHistoryEvent[]): HistoryRow[] {
  return history.map((e) => ({ key: e.hash, what: EVENT_WORDS[e.decision], effect: EFFECT_WORDS[e.decision], actor: e.actor, role: e.role, at: e.at, note: e.note }));
}

const STATUS_WORDS: Record<ConsentOverlay["status"], string> = {
  none: "No demo consent event yet",
  granted: "Consent granted",
  denied: "Consent denied",
  opted_out: "Opted out",
};
const PENDING_WORDS: Record<ConsentOverlay["demo_pending_consent"], string> = {
  "": "nothing held or released by the demo yet",
  released: "demo hold released",
  cancelled: "demo outreach cancelled",
};

/** One line for the overlay's current state. */
export function overlayWords(o: ConsentOverlay): string {
  return `${STATUS_WORDS[o.status]}: ${PENDING_WORDS[o.demo_pending_consent]}${o.contact_blocked ? "; contact confirmation blocked" : ""}.`;
}
