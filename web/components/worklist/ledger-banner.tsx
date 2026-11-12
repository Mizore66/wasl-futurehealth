import { cn } from "@/lib/utils";
import type { WorkItem } from "./model";

/**
 * `ledger_status: invalid` is server-wide (pipeline/api.go overlayViewLocked): one failed replay leaves every case's
 * overlay as a prefix of the ledger. So when any listed case reports it, no status, owner or position on the list is
 * authoritative, not only the open patient's.
 */
export function listLedgerInvalid(items: Pick<WorkItem, "ledgerStatus">[]): boolean {
  return items.some((i) => i.ledgerStatus === "invalid");
}

/** Board-level warning for a partial ledger replay. `what` names what the reader is looking at ("this board", "this list"). */
export function LedgerInvalidBanner({ what, className }: { what: string; className?: string }) {
  return (
    <div role="alert" data-testid="ledger-invalid-board" className={cn("rounded-card border-2 border-warning bg-surface p-3", className)}>
      <p className="m-0 font-medium text-warning">Case ledger incomplete: nothing on {what} is authoritative</p>
      <p className="m-0 mt-1">
        The API stopped replaying the shared case ledger at an invalid entry. Every status, owner and position on {what} reflects only
        the actions before it, and the API refuses new actions until an operator repairs the ledger.
      </p>
    </div>
  );
}
