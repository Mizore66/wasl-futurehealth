"use client";

import { useSearchParams } from "next/navigation";

/** The queue being viewed, as a hidden field: the sidebar switches queue in the browser, so read the live URL. */
export function QueueField({ initial }: { initial?: string }) {
  const q = useSearchParams()?.get("queue") ?? initial;
  return q ? <input type="hidden" name="queue" value={q} /> : null;
}
