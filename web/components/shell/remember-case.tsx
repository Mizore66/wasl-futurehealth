"use client";

import { useEffect } from "react";

export const LAST_CASE_KEY = "wasl:last-case";

/** Remembers the last case opened, so the sidebar's Audit trail link opens that case's audit. */
export function RememberCase({ id }: { id: string }) {
  useEffect(() => {
    try {
      sessionStorage.setItem(LAST_CASE_KEY, id);
      window.dispatchEvent(new Event("wasl:last-case"));
    } catch {}
  }, [id]);
  return null;
}
