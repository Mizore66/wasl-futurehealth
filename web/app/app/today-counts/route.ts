import { todayQueueCounts } from "@/components/worklist/today-items";

/**
 * Today's queue counts for the sidebar. The sidebar asks for them from the browser, and never on pages that act with a
 * reviewer's own session (the review desk and audit), so those pages make no shared-credential case reads.
 */
export async function GET() {
  const counts = await todayQueueCounts();
  return Response.json(counts, { headers: { "cache-control": "no-store" } });
}
