import { todayQueueCounts } from "@/components/worklist/today-items";

/**
 * Today's queue counts for the sidebar. The sidebar asks for them from the browser, and never on pages that act with a
 * reviewer's own session (the review desk and audit), so those pages make no shared-credential case reads.
 *
 * Every staff page asks for them, and each answer reads every case, so one answer is shared for a few seconds per
 * server instance. The mounted Today board publishes its own live counts, so a stale number never outlives a visit
 * to Today; elsewhere it is at most this old.
 */
const FRESH_MS = 15_000;
let memo: { at: number; counts: Promise<Awaited<ReturnType<typeof todayQueueCounts>>> } | null = null;

export async function GET() {
  if (!memo || Date.now() - memo.at > FRESH_MS) {
    const counts = todayQueueCounts();
    memo = { at: Date.now(), counts };
    counts.catch(() => {
      if (memo?.counts === counts) memo = null;
    });
  }
  const counts = await memo.counts;
  return Response.json(counts, { headers: { "cache-control": "no-store" } });
}
