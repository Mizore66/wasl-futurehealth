import type { Healthz } from "@/lib/api/schema";

/** What the health panel can say. `snapshot` is the published file, `live` a fetch made when this page rendered. */
export type HealthState =
  | { kind: "snapshot"; health: Healthz; takenAt: string }
  | { kind: "live"; health: Healthz; fetchedAt: string }
  | { kind: "unavailable"; reason: string };

/** Load `/healthz` through `get`. Any failure becomes an explicit unavailable state; nothing is guessed. */
export async function loadHealth(get: () => Promise<Healthz>, mode: "live" | "fixtures", now: () => Date = () => new Date()): Promise<HealthState> {
  try {
    const health = await get();
    return mode === "live"
      ? { kind: "live", health, fetchedAt: now().toISOString() }
      : { kind: "snapshot", health, takenAt: health.meta.loaded_at };
  } catch (e) {
    return { kind: "unavailable", reason: e instanceof Error ? e.message : "unknown error" };
  }
}

/** The four dependencies the backend reports, with what each value means. */
export const DEPENDENCY_ORDER = ["materialised_run", "nats", "reader", "ingest_adapter"] as const;

export const DEPENDENCY_GLOSS: Record<(typeof DEPENDENCY_ORDER)[number], { label: string; note: string }> = {
  materialised_run: { label: "Recorded run", note: "The saved run the read API serves from." },
  nats: { label: "Message bus (NATS)", note: "Not required for the saved-run read API; it matters only when a run is processed live." },
  reader: { label: "Report reader", note: "The canonical reader's answers are cached, so reading does not call a model while you browse." },
  ingest_adapter: { label: "Ingest adapter", note: "The hospital-message format this run was loaded through." },
};

/** Concise text for the screen-reader status region after the user presses Step. */
export function stepAnnouncement(view: { step: number; steps: number; feed: { step: number; who: { name: string }; text: string }[] }): string {
  const newest = view.feed[0];
  const where = `Step ${view.step} of ${view.steps}.`;
  return newest ? `${where} Newest decision: ${newest.who.name}, ${newest.text}.` : `${where} No decision recorded yet.`;
}
