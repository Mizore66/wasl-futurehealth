"use client";

import { useLinkStatus } from "next/link";

/**
 * Render inside a sidebar <Link>: while that link's navigation is pending, a thin bar runs along the top of the window
 * and the link dims, so a click is acknowledged at once even when the server takes a moment. (A route-level
 * loading.js would also do this, but it makes not-found pages stream with status 200.)
 */
export function LinkPending() {
  const { pending } = useLinkStatus();
  if (!pending) return null;
  return (
    <>
      <span aria-hidden className="pointer-events-none absolute inset-0 rounded-[inherit] bg-current opacity-10" />
      <span aria-hidden className="wasl-nav-progress pointer-events-none fixed inset-x-0 top-0 z-50 h-[3px] bg-jade-500" />
    </>
  );
}
