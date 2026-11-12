import { cn } from "@/lib/utils";
import type { WorkItem } from "./model";

/**
 * `ledger_status: invalid` is server-wide (pipeline/api.go overlayViewLocked, from `ledgerErr`): set when replay stopped
 * at an invalid entry, when an append failed and left the ledger file in an unknown state, or when the ledger changed
 * under another writer. Every case's overlay may then disagree with the ledger, so when any listed case reports it, no
 * status, owner or position on the list is authoritative, not only the open patient's.
 */
export function listLedgerInvalid(items: Pick<WorkItem, "ledgerStatus">[]): boolean {
  return items.some((i) => i.ledgerStatus === "invalid");
}

/**
 * Board-level warning for an untrustworthy ledger, whatever set `ledgerErr`. `what` names what the reader is looking at
 * ("this board", "this list"). This is the one alert; per-patient detail under it is not announced separately.
 */
export function LedgerInvalidBanner({ what, className }: { what: string; className?: string }) {
  return (
    <div role="alert" data-testid="ledger-invalid-board" className={cn("rounded-card border-2 border-warning bg-surface p-3", className)}>
      <p className="m-0 font-medium text-warning">Case ledger not trustworthy: nothing on {what} is authoritative</p>
      <p className="m-0 mt-1">
        The API reports its shared case ledger as not trustworthy, so no status, owner or position on {what} can be relied on, and the API
        refuses new actions until an operator restores it.
      </p>
    </div>
  );
}
