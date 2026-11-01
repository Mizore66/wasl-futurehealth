import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { ReviewQueue } from "./queue";

export function QueueLabel({ source }: { source: ReviewQueue["source"] }) {
  return source === "open" ? (
    <HonestyLabel kind="built">Open review items from the API</HonestyLabel>
  ) : (
    <HonestyLabel kind="simulated">Simulated queue: rebuilt from the review_hold case</HonestyLabel>
  );
}
