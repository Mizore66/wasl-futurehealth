import type { Operations } from "@/lib/api/schema";

/** The entire operations panel succeeds or becomes unavailable together; no earlier values are retained. */
export type OperationsState =
  | { kind: "snapshot"; operations: Operations; takenAt: string }
  | { kind: "live"; operations: Operations; fetchedAt: string }
  | { kind: "unavailable"; reason: string };

export async function loadOperations(
  get: () => Promise<Operations>,
  mode: "live" | "fixtures",
  now: () => Date = () => new Date(),
): Promise<OperationsState> {
  try {
    const operations = await get();
    return mode === "live"
      ? { kind: "live", operations, fetchedAt: now().toISOString() }
      : { kind: "snapshot", operations, takenAt: operations.meta.loaded_at };
  } catch (e) {
    return { kind: "unavailable", reason: e instanceof Error ? e.message : "unknown error" };
  }
}

/** Only a recorded completed-run numerator/denominator may reach this helper. */
export function completedRunRate(numerator: number, denominator: number): string {
  if (denominator === 0) return "Unmeasured";
  return `${Number(((numerator / denominator) * 100).toFixed(2))}%`;
}

export const DEPENDENCY_ORDER = ["materialised_run", "nats", "reader", "ingest_adapter", "review_ledger"] as const;

export const DEPENDENCY_GLOSS: Record<(typeof DEPENDENCY_ORDER)[number], { label: string; note: string }> = {
  materialised_run: { label: "Recorded run", note: "The saved run the read API serves from." },
  nats: { label: "Health endpoint NATS dial", note: "Only the existing health endpoint's dependency string; the authenticated JetStream probe below is separate." },
  reader: { label: "Report reader", note: "The canonical reader's answers are cached, so reading does not call a model while you browse." },
  ingest_adapter: { label: "Ingest adapter", note: "The hospital-message format this run was loaded through." },
  review_ledger: { label: "Review ledger", note: "Whether the materialised API review ledger replayed without a validation error." },
};

/** Concise text for the screen-reader status region after the user presses Step. */
export function stepAnnouncement(view: { step: number; steps: number; feed: { step: number; who: { name: string }; text: string }[] }): string {
  const newest = view.feed[0];
  const where = `Step ${view.step} of ${view.steps}.`;
  return newest ? `${where} Newest decision: ${newest.who.name}, ${newest.text}.` : `${where} No decision recorded yet.`;
}
