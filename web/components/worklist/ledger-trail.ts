import { atWords, outcomeWords } from "@/components/case/words";
import type { CaseDetail } from "@/lib/api/schema";

/**
 * One entry of Today's contact trail. Session records and persisted ledger records share this shape. `who` is the
 * actor the API recorded (live) or the browser that holds the entry (fixtures); `hash` is the ledger event, when saved.
 */
export type TrailEntry = { what: string; when: string; who: string; hash?: string };

/**
 * What the selected case's detail overlay says (issue #47): every persisted contact outcome and note as trail entries,
 * oldest first, and whether the ledger replayed completely. `invalid` means replay stopped at an invalid ledger line:
 * the entries are only the valid prefix and the case's state on Today is not authoritative; the API refuses writes.
 */
export type PersistedLedger = {
  status: "ok" | "invalid";
  /** The API's own explanation of the failure (`review_ledger.detail`), shown as given. */
  detail: string;
  /** Ledger events the API applied before stopping (all of them when `ok`). */
  applied: number | null;
  entries: TrailEntry[];
};

const saved = (at: string, event: string) => `Saved · ${atWords(at)}${event ? ` · ${event.slice(0, 8)}` : ""}`;
const quoted = (s: string | undefined) => (s ? `: “${s}”` : "");

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
  ];
  timed.sort((a, b) => (a.at < b.at ? -1 : a.at > b.at ? 1 : 0));
  return {
    status,
    detail: status === "invalid" ? (rl?.detail ?? "") : "",
    applied: rl?.events_applied ?? null,
    entries: timed.map((e) => ({ what: e.what, when: e.when, who: e.who, ...(e.hash ? { hash: e.hash } : {}) })),
  };
}
