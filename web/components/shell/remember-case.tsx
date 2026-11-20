"use client";

import { useEffect } from "react";

/** Cookie the server reads at /app/audit: the last case opened in this browser, so Audit trail opens its audit. */
export const LAST_CASE_COOKIE = "wasl-last-case";

/** Remembers the last case opened, so the sidebar's Audit trail link (/app/audit) opens that case's audit. */
export function RememberCase({ id }: { id: string }) {
  useEffect(() => {
    document.cookie = `${LAST_CASE_COOKIE}=${encodeURIComponent(id)}; path=/app; max-age=86400; samesite=lax`;
  }, [id]);
  return null;
}
