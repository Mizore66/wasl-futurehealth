import type { WorkItem } from "./model";

/*
 * Today's casebook words: each queue row and the open case lead with the task, not the patient, and the open case says
 * the next action in one sentence. Everything here is derived from the work item; nothing is added to it.
 */

type TaskFields = Pick<WorkItem, "view" | "status" | "next" | "communicationStep" | "teachBackCallback" | "retrieval">;

/** The task a case is waiting on, as a short title. */
export function taskTitle(r: TaskFields): string {
  if (r.teachBackCallback) return "Teach-back callback";
  if (r.retrieval) return "Record retrieval";
  if (r.view !== "waiting" && r.communicationStep === "notify") return "Initial notice due";
  if (r.view !== "waiting" && r.communicationStep === "receipt") return "Receipt outstanding";
  return r.status.split(" · ")[0];
}

const sentence = (s: string) => {
  const t = s.trim();
  if (!t) return "";
  return `${t[0].toUpperCase()}${t.slice(1)}${/[.!?]$/.test(t) ? "" : "."}`;
};

/** One or two sentences under the title: where the case stands (unless the title already says it) and the next action. */
export function taskSentence(r: TaskFields): string {
  const title = taskTitle(r);
  const status = r.status === title ? "" : sentence(r.status.replaceAll(" · ", ", "));
  return [status, sentence(r.next.replaceAll(" · ", ", "))].filter(Boolean).join(" ");
}

export type ContactWords = { label: string; tone: "ok" | "warn" };

/**
 * The contact restriction shown beside Log call, before any tab is opened. A recorded denial or opt-out holds contact
 * (and Log call); otherwise the canonical permission is named as the case list serves it.
 */
export function contactWords(r: Pick<WorkItem, "contactBlocked" | "consent">): ContactWords {
  if (r.contactBlocked) return { label: "Contact on hold", tone: "warn" };
  if (r.consent === "granted") return { label: "Contact permitted", tone: "ok" };
  if (r.consent === "unknown") return { label: "Contact permission unknown", tone: "warn" };
  return { label: "Texts not permitted", tone: "warn" };
}

/** Contact permission in the case details. */
export const CONSENT_WORDS: Record<WorkItem["consent"], string> = {
  granted: "Granted",
  denied: "Declined",
  opted_out: "Opted out",
  unknown: "Unknown",
};
