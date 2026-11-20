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
      <p className="m-0 font-medium text-warning">Case history needs checking. Details on {what} may be out of date.</p>
      <p className="m-0 mt-1">
        Statuses, owners and order on {what} may be wrong. Saving is paused until support fixes the case history.
      </p>
    </div>
  );
}
