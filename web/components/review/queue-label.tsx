import { HONESTY_PLAIN, HonestyLabel } from "@/components/wasl/honesty-label";
import { apiMode } from "@/lib/api/source";
import type { ReviewQueue } from "./queue";

/**
 * Where the queue came from. A rebuilt queue is a stand-in, but against the live API its answers are real POSTs
 * appended to a case the server already shows as signed, so the label says that rather than "simulated".
 */
export function QueueLabel({ source }: { source: ReviewQueue["source"] }) {
  if (source === "incomplete") return <HonestyLabel kind="built" className={HONESTY_PLAIN}>Incomplete. Some open review items could not be loaded.</HonestyLabel>;
  if (source === "open") return <HonestyLabel kind="built" className={HONESTY_PLAIN}>Open review items from the API.</HonestyLabel>;
  return apiMode() === "live" ? (
    <HonestyLabel kind="simulated" className={HONESTY_PLAIN}>Practice queue on an already-signed demo case. Answers are saved.</HonestyLabel>
  ) : (
    <HonestyLabel kind="simulated" className={HONESTY_PLAIN}>Simulated queue rebuilt from a held demo case.</HonestyLabel>
  );
}
