import { HonestyLabel } from "@/components/wasl/honesty-label";
import { apiMode } from "@/lib/api/source";
import type { ReviewQueue } from "./queue";

/**
 * Where the queue came from. A rebuilt queue is a stand-in, but against the live API its answers are real POSTs
 * appended to a case the server already shows as signed, so the label says that rather than "simulated".
 */
export function QueueLabel({ source }: { source: ReviewQueue["source"] }) {
  if (source === "open") return <HonestyLabel kind="built">Open review items from the API</HonestyLabel>;
  return apiMode() === "live" ? (
    <HonestyLabel kind="simulated">Practice queue: an already-signed demo case, answers are saved</HonestyLabel>
  ) : (
    <HonestyLabel kind="simulated">Simulated queue: rebuilt from a held demo case</HonestyLabel>
  );
}
