import type { CaseSummary } from "./schema";

/** The owner a reader sees: the coordinator-overlay owner when one was saved, otherwise the canonical owner. */
export function effectiveOwner(c: Pick<CaseSummary, "owner" | "api_overlay">, overlay: { owner: string } | undefined = c.api_overlay): string {
  return overlay?.owner || c.owner;
}
