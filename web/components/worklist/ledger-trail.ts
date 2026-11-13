import { atWords, outcomeWords } from "@/components/case/words";
import type { CaseDetail } from "@/lib/api/schema";
import { compareInstants } from "@/lib/domain/instant";

/**
 * One entry of Today's contact trail. Session records and persisted ledger records share this shape. `who` is the
 * actor the API recorded (live) or the browser that holds the entry (fixtures); `hash` is the ledger event, when saved.
 */
export type TrailEntry = { what: string; when: string; who: string; hash?: string };

/**
 * What the selected case's detail overlay says (issue #47): every persisted contact outcome and note as trail entries,
 * oldest first, and whether the API trusts its ledger. `invalid` (Go `ledgerErr`: replay stopped at an invalid line, an
 * append left the file in an unknown state, or another writer changed it) means the entries may not match the ledger
 * and the case's state on Today is not authoritative; the API refuses writes.
 */
export type PersistedLedger = {
  status: "ok" | "invalid";
  /** The API's own explanation of the failure (`review_ledger.detail`), shown as given. */
  detail: string;
  /** Ledger events the API applied before stopping (all of them when `ok`). */
  applied: number | null;
  entries: TrailEntry[];
  patient?: CaseDetail["api_patient"];
};

const saved = (at: string, event: string) => `Saved · ${atWords(at)}${event ? ` · ${event.slice(0, 8)}` : ""}`;
const quoted = (s: string | undefined) => (s ? `: “${s}”` : "");

function patientActivity(event: Extract<CaseDetail["api_reviews"][number], { ledger: "patient" }>): string {
  switch (event.kind) {
    case "teach_back_completed":
      return "Patient completed teach-back";
    case "contact_permission_changed":
      return `Patient contact permission: ${event.data.permission ?? "changed"}`;
    case "slot_selected":
      return "Patient selected a simulated slot · coordinator review needed";
    case "preparation_changed":
      return `Patient preparation ${event.data.item_id === "escort_home" ? "escort home" : "clear fluids and prep"}: ${event.data.completed ? "complete" : "not complete"}`;
  }
}

export function persistedLedger(detail: CaseDetail): PersistedLedger {
  const ov = detail.api_overlay;
  const rl = detail.review_ledger;
  const status = rl?.status === "invalid" || ov?.ledger_status === "invalid" ? "invalid" : "ok";
  const timed: (TrailEntry & { at: string })[] = [
    ...(ov?.contact_outcomes ?? []).map((o) => ({
      at: o.at,
      what: `${outcomeWords(o.outcome, o.channel)}${quoted(o.note)}`,
      when: saved(o.at, o.event),
      who: o.actor || "Actor not recorded",
      hash: o.event || undefined,
    })),
    ...(ov?.notes ?? []).map((n) => ({
      at: n.at,
      what: `Note${quoted(n.text)}`,
      when: saved(n.at, n.event),
      who: n.actor || "Actor not recorded",
      hash: n.event || undefined,
    })),
    ...detail.api_reviews.filter((event): event is Extract<CaseDetail["api_reviews"][number], { ledger: "patient" }> => event.ledger === "patient").map((event) => ({
      at: event.at,
      what: patientActivity(event),
      when: saved(event.at, event.hash),
      who: "Patient self-service",
      hash: event.hash,
    })),
  ];
  timed.sort((a, b) => compareInstants(a.at, b.at));
  return {
    status,
    detail: status === "invalid" ? (rl?.detail ?? "") : "",
    applied: rl?.events_applied ?? null,
    entries: timed.map((e) => ({ what: e.what, when: e.when, who: e.who, ...(e.hash ? { hash: e.hash } : {}) })),
    ...(detail.api_patient ? { patient: detail.api_patient } : {}),
  };
}
