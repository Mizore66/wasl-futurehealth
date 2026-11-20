import { HonestyLabel } from "@/components/wasl/honesty-label";
import { apiMode } from "@/lib/api/source";
import type { ReviewQueue } from "./queue";

/**
 * Where the queue came from. A rebuilt queue is a stand-in, but against the live API its answers are real POSTs
 * appended to a case the server already shows as signed, so the label says that rather than "simulated".
 */
export function queueWords(source: ReviewQueue["source"]): { kind: "built" | "simulated"; text: string } | null {
  if (source === "incomplete") return { kind: "built", text: "Incomplete. Some cases could not be loaded" };
  // The normal case says nothing: open review items from the API need no label.
  if (source === "open") return null;
  return apiMode() === "live"
    ? { kind: "simulated", text: "Practice queue. Answers are saved" }
    : { kind: "simulated", text: "Simulated queue, a practice case" };
}

/** Plain muted words for the page header's subtitle line, not a chip over the header art. */
export function QueueLabel({ source }: { source: ReviewQueue["source"] }) {
  const words = queueWords(source);
  if (!words) return null;
  return (
    <HonestyLabel plain kind={words.kind} className="text-muted">
      {words.text}
    </HonestyLabel>
  );
}
